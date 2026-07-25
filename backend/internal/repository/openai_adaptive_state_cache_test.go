package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAdaptiveStateCacheSaveScanAndCleanup(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	cache, ok := NewGatewayCache(rdb).(service.AdaptiveSchedulerStateCache)
	require.True(t, ok)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	hashKey, expiryKey, err := adaptiveSchedulerStateKeys("openai")
	require.NoError(t, err)
	err = cache.SaveAdaptiveSchedulerStates(t.Context(), "openai", []service.AdaptiveSchedulerStateCacheEntry{
		{AccountID: 101, Payload: []byte(`{"account_id":101}`), ExpiresAt: now.Add(-time.Minute)},
		{AccountID: 102, Payload: []byte(`{"account_id":102}`), ExpiresAt: now.Add(12 * time.Hour)},
	}, 24*time.Hour)
	require.NoError(t, err)
	hashLen, err := rdb.HLen(t.Context(), hashKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(2), hashLen)
	require.Equal(t, 24*time.Hour, mr.TTL(hashKey))
	require.Equal(t, 24*time.Hour, mr.TTL(expiryKey))

	records, cursor, err := cache.ScanAdaptiveSchedulerStates(t.Context(), "openai", 0, 256)
	require.NoError(t, err)
	require.Zero(t, cursor)
	require.Len(t, records, 2)

	removed, err := cache.DeleteExpiredAdaptiveSchedulerStates(t.Context(), "openai", now, 256)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)
	exists, err := rdb.HExists(t.Context(), hashKey, "101").Result()
	require.NoError(t, err)
	require.False(t, exists)
	exists, err = rdb.HExists(t.Context(), hashKey, "102").Result()
	require.NoError(t, err)
	require.True(t, exists)
	_, err = rdb.ZScore(t.Context(), expiryKey, "101").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = rdb.ZScore(t.Context(), expiryKey, "102").Result()
	require.NoError(t, err)
}

func TestOpenAIAdaptiveStateCacheRefreshesAccountExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	cache := NewGatewayCache(rdb).(service.AdaptiveSchedulerStateCache)
	now := time.Now()
	hashKey, _, err := adaptiveSchedulerStateKeys("openai")
	require.NoError(t, err)

	require.NoError(t, cache.SaveAdaptiveSchedulerStates(t.Context(), "openai", []service.AdaptiveSchedulerStateCacheEntry{
		{AccountID: 201, Payload: []byte("old"), ExpiresAt: now.Add(-time.Minute)},
	}, 24*time.Hour))
	require.NoError(t, cache.SaveAdaptiveSchedulerStates(t.Context(), "openai", []service.AdaptiveSchedulerStateCacheEntry{
		{AccountID: 201, Payload: []byte("new"), ExpiresAt: now.Add(12 * time.Hour)},
	}, 24*time.Hour))

	removed, err := cache.DeleteExpiredAdaptiveSchedulerStates(context.Background(), "openai", now, 256)
	require.NoError(t, err)
	require.Zero(t, removed)
	require.Equal(t, "new", mr.HGet(hashKey, "201"))
}

func TestAdaptiveSchedulerStateCacheKeepsNamespacesIsolated(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	cache := NewGatewayCache(rdb).(service.AdaptiveSchedulerStateCache)
	now := time.Now()

	for _, namespace := range []string{"openai", "anthropic"} {
		require.NoError(t, cache.SaveAdaptiveSchedulerStates(t.Context(), namespace, []service.AdaptiveSchedulerStateCacheEntry{
			{AccountID: 301, Payload: []byte(namespace), ExpiresAt: now.Add(-time.Minute)},
		}, 24*time.Hour))
	}

	removed, err := cache.DeleteExpiredAdaptiveSchedulerStates(t.Context(), "anthropic", now, 256)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)
	openAIHashKey, _, err := adaptiveSchedulerStateKeys("openai")
	require.NoError(t, err)
	anthropicHashKey, _, err := adaptiveSchedulerStateKeys("anthropic")
	require.NoError(t, err)
	require.Equal(t, "openai", mr.HGet(openAIHashKey, "301"))
	require.False(t, mr.Exists(anthropicHashKey))
}
