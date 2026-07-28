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

func TestGatewayCacheGeminiSessionMigrationLeaseCASDeleteAndRelease(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	baseCache := NewGatewayCache(client)
	migrationCache, ok := baseCache.(service.GeminiSessionMigrationCache)
	require.True(t, ok)
	ctx := context.Background()

	require.NoError(t, baseCache.SetSessionAccountID(ctx, 11, "session", 101, time.Hour))
	acquired, err := migrationCache.TryAcquireSessionMigrationLease(ctx, 11, "session", "token-a", time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = migrationCache.TryAcquireSessionMigrationLease(ctx, 11, "session", "token-b", time.Minute)
	require.NoError(t, err)
	require.False(t, acquired, "only one migration may own a session lease")

	swapped, err := migrationCache.CompareAndSwapSessionAccountID(ctx, 11, "session", 101, 202, "token-b", time.Hour)
	require.NoError(t, err)
	require.False(t, swapped, "a non-owner cannot update the binding")
	swapped, err = migrationCache.CompareAndSwapSessionAccountID(ctx, 11, "session", 999, 202, "token-a", time.Hour)
	require.NoError(t, err)
	require.False(t, swapped, "CAS must reject a changed source binding")
	swapped, err = migrationCache.CompareAndSwapSessionAccountID(ctx, 11, "session", 101, 202, "token-a", time.Hour)
	require.NoError(t, err)
	require.True(t, swapped)
	accountID, err := baseCache.GetSessionAccountID(ctx, 11, "session")
	require.NoError(t, err)
	require.Equal(t, int64(202), accountID)

	released, err := migrationCache.ReleaseSessionMigrationLease(ctx, 11, "session", "token-b")
	require.NoError(t, err)
	require.False(t, released)
	released, err = migrationCache.ReleaseSessionMigrationLease(ctx, 11, "session", "token-a")
	require.NoError(t, err)
	require.True(t, released)

	acquired, err = migrationCache.TryAcquireSessionMigrationLease(ctx, 11, "session", "token-b", time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	deleted, err := migrationCache.CompareAndDeleteSessionAccountID(ctx, 11, "session", 101, "token-b")
	require.NoError(t, err)
	require.False(t, deleted)
	deleted, err = migrationCache.CompareAndDeleteSessionAccountID(ctx, 11, "session", 202, "token-b")
	require.NoError(t, err)
	require.True(t, deleted)
	_, err = baseCache.GetSessionAccountID(ctx, 11, "session")
	require.ErrorIs(t, err, redis.Nil)
	released, err = migrationCache.ReleaseSessionMigrationLease(ctx, 11, "session", "token-b")
	require.NoError(t, err)
	require.True(t, released)
}

func TestGatewayCacheGeminiSessionMigrationCASCreatesFirstBinding(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	baseCache := NewGatewayCache(client)
	migrationCache, ok := baseCache.(service.GeminiSessionMigrationCache)
	require.True(t, ok)
	ctx := context.Background()

	acquired, err := migrationCache.TryAcquireSessionMigrationLease(ctx, 12, "new-session", "token", time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	swapped, err := migrationCache.CompareAndSwapSessionAccountID(ctx, 12, "new-session", 0, 303, "token", time.Hour)
	require.NoError(t, err)
	require.True(t, swapped)
	accountID, err := baseCache.GetSessionAccountID(ctx, 12, "new-session")
	require.NoError(t, err)
	require.Equal(t, int64(303), accountID)
}
