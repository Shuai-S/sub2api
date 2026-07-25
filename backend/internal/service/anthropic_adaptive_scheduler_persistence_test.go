package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeAdaptiveSchedulerGatewayCache struct {
	GatewayCache
	*fakeOpenAIAdaptiveStateCache
}

func validAnthropicAdaptivePersistenceState(accountID int64, updatedAt time.Time) anthropicAdaptiveAccountState {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	state := defaultAnthropicAdaptiveAccountState(
		&Account{ID: accountID, Platform: PlatformAnthropic, Concurrency: 100},
		updatedAt,
		settings,
	)
	state.EstimatedCapacity = 40
	state.SuccessEMA = 0.98
	state.LatencyByModelFamily["sonnet"] = anthropicAdaptiveLatencyState{
		TTFTEMA:    320,
		LatencyEMA: 1100,
		Samples:    100,
	}
	state.ConsecutiveSuccess = 4
	state.TotalSamples = 102
	state.RecentHealthSamples = 12
	state.RecentHealthFailures = 1
	state.RecentCapacitySamples = 10
	state.RecentCapacityFailures = 1
	state.UpdatedAt = updatedAt
	return state
}

func mustEncodeAnthropicAdaptivePersistenceState(t *testing.T, state anthropicAdaptiveAccountState) []byte {
	t.Helper()
	payload, err := encodeAnthropicAdaptivePersistedState(state, "test-instance")
	require.NoError(t, err)
	return payload
}

func TestAnthropicAdaptiveStatePersistenceFlushesOnlyDirtyRevisions(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{}
	store := newAnthropicAdaptiveStateStore()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	account := &Account{ID: 5001, Platform: PlatformAnthropic, Concurrency: 100}
	now := time.Now()
	store.report(AnthropicAdaptiveScheduleReport{
		Account:        account,
		RequestedModel: "claude-sonnet-4-6",
		Success:        true,
		HealthSample:   true,
		CapacitySample: true,
		FirstTokenMs:   intPtr(250),
		DurationMs:     900,
	}, now, settings)

	persistence := newAnthropicAdaptiveStatePersistence(cache, store, nil)
	require.NoError(t, persistence.flush(t.Context()))

	_, saveCalls, _ := cache.counts()
	require.Equal(t, 1, saveCalls)
	require.Equal(t, []string{adaptiveSchedulerStateNamespaceAnthropic}, cache.savedNamespaces)
	require.Len(t, cache.saved, 1)
	require.Len(t, cache.saved[0], 1)
	entry := cache.saved[0][0]
	require.Equal(t, account.ID, entry.AccountID)
	require.WithinDuration(t, now.Add(adaptiveStateRetention), entry.ExpiresAt, time.Millisecond)

	restored, err := decodeAnthropicAdaptivePersistedState(account.ID, entry.Payload, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(1), restored.TotalSamples)
	require.Equal(t, 250.0, restored.LatencyByModelFamily["sonnet"].TTFTEMA)
	require.Equal(t, int64(1), restored.LatencyByModelFamily["sonnet"].Samples)

	require.NoError(t, persistence.flush(t.Context()))
	_, saveCalls, _ = cache.counts()
	require.Equal(t, 1, saveCalls, "unchanged revisions must not be written again")

	store.report(AnthropicAdaptiveScheduleReport{
		Account:      account,
		HealthSample: true,
	}, now.Add(time.Second), settings)
	require.NoError(t, persistence.flush(t.Context()))
	_, saveCalls, _ = cache.counts()
	require.Equal(t, 2, saveCalls)
}

func TestAnthropicAdaptiveStatePersistenceKeepsConcurrentRevisionDirty(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{}
	store := newAnthropicAdaptiveStateStore()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	account := &Account{ID: 5002, Platform: PlatformAnthropic, Concurrency: 100}
	store.report(AnthropicAdaptiveScheduleReport{Account: account, Success: true, HealthSample: true}, time.Now(), settings)
	cache.saveFn = func([]AdaptiveSchedulerStateCacheEntry) error {
		store.report(AnthropicAdaptiveScheduleReport{Account: account, HealthSample: true}, time.Now(), settings)
		return nil
	}

	persistence := newAnthropicAdaptiveStatePersistence(cache, store, nil)
	require.NoError(t, persistence.flush(t.Context()))
	require.Len(t, store.dirtySnapshots(time.Now(), adaptiveStateRetention), 1)

	cache.saveFn = nil
	require.NoError(t, persistence.flush(t.Context()))
	require.Empty(t, store.dirtySnapshots(time.Now(), adaptiveStateRetention))
}

func TestAnthropicAdaptiveStatePersistenceRestoresFreshSnapshotsOnly(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	fresh := validAnthropicAdaptivePersistenceState(6001, now.Add(-time.Hour))
	stale := validAnthropicAdaptivePersistenceState(6002, now.Add(-13*time.Hour))
	cache := &fakeOpenAIAdaptiveStateCache{
		scanFn: func(uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
			return []AdaptiveSchedulerStateCacheRecord{
				{AccountID: fresh.AccountID, Payload: mustEncodeAnthropicAdaptivePersistenceState(t, fresh)},
				{AccountID: stale.AccountID, Payload: mustEncodeAnthropicAdaptivePersistenceState(t, stale)},
				{AccountID: 6003, Payload: []byte("not-json")},
			}, 0, nil
		},
	}
	store := newAnthropicAdaptiveStateStore()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	persistence := newAnthropicAdaptiveStatePersistence(cache, store, func(context.Context) AnthropicAdaptiveSchedulerSettings {
		return settings
	})
	persistence.now = func() time.Time { return now }

	restored, staleCount, invalid, err := persistence.restoreOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restored)
	require.Equal(t, 1, staleCount)
	require.Equal(t, 1, invalid)
	require.Equal(t, []string{adaptiveSchedulerStateNamespaceAnthropic}, cache.scanNamespaces)
	require.NotContains(t, store.accounts, stale.AccountID)

	state := store.snapshot(&Account{ID: fresh.AccountID, Concurrency: 100}, settings)
	require.Equal(t, 40, state.EstimatedCapacity)
	require.Zero(t, state.RecentHealthSamples, "expired recent learning window must be reset")
	require.Zero(t, state.RecentCapacitySamples)
	require.Equal(t, 320.0, state.LatencyByModelFamily["sonnet"].TTFTEMA)
}

func TestAnthropicAdaptiveStatePersistenceDiscardsPartialScanOnFailure(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	state := validAnthropicAdaptivePersistenceState(7001, now.Add(-time.Hour))
	cache := &fakeOpenAIAdaptiveStateCache{
		scanFn: func(cursor uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
			if cursor == 0 {
				return []AdaptiveSchedulerStateCacheRecord{{
					AccountID: state.AccountID,
					Payload:   mustEncodeAnthropicAdaptivePersistenceState(t, state),
				}}, 1, nil
			}
			return nil, 0, errors.New("redis unavailable")
		},
	}
	store := newAnthropicAdaptiveStateStore()
	persistence := newAnthropicAdaptiveStatePersistence(cache, store, nil)
	persistence.now = func() time.Time { return now }

	_, _, _, err := persistence.restoreOnce(t.Context())
	require.ErrorContains(t, err, "redis unavailable")
	require.NotContains(t, store.accounts, state.AccountID, "a failed scan must not apply a partial restore")
}

func TestAnthropicAdaptiveStateStoreStartupMergePreservesLocalSafety(t *testing.T) {
	now := time.Now()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	incoming := validAnthropicAdaptivePersistenceState(8001, now.Add(-time.Hour))
	incoming.EstimatedCapacity = 40
	account := &Account{ID: incoming.AccountID, Platform: PlatformAnthropic, Concurrency: 100}

	t.Run("early success keeps learned capacity", func(t *testing.T) {
		store := newAnthropicAdaptiveStateStore()
		store.report(AnthropicAdaptiveScheduleReport{
			Account:      account,
			Success:      true,
			HealthSample: true,
		}, now, settings)

		require.True(t, store.restoreAtStartup(incoming, now))
		state := store.snapshot(account, settings)
		require.Equal(t, 40, state.EstimatedCapacity)
		require.Equal(t, incoming.TotalSamples+1, state.TotalSamples)
	})

	t.Run("early failure keeps lower capacity and cooldown", func(t *testing.T) {
		store := newAnthropicAdaptiveStateStore()
		local := defaultAnthropicAdaptiveAccountState(account, now, settings)
		local.EstimatedCapacity = 3
		local.TotalSamples = 1
		local.RecentHealthSamples = 1
		local.RecentHealthFailures = 1
		local.RecentCapacitySamples = 1
		local.RecentCapacityFailures = 1
		local.ConsecutiveFailure = 1
		local.ConsecutiveCapacityFailure = 1
		local.SuccessEMA = 0.5
		local.CooldownUntil = now.Add(time.Minute)
		local.LastFailureAt = now
		local.UpdatedAt = now
		local.revision = 1
		store.accounts[incoming.AccountID] = &local

		require.True(t, store.restoreAtStartup(incoming, now))
		state := store.snapshot(account, settings)
		require.Equal(t, 3, state.EstimatedCapacity)
		require.Equal(t, 0.5, state.SuccessEMA)
		require.Equal(t, local.CooldownUntil, state.CooldownUntil)
	})

	t.Run("established local learning wins", func(t *testing.T) {
		store := newAnthropicAdaptiveStateStore()
		local := validAnthropicAdaptivePersistenceState(incoming.AccountID, now)
		local.EstimatedCapacity = 7
		local.TotalSamples = anthropicAdaptiveStateLocalMergeLimit
		store.accounts[incoming.AccountID] = &local

		require.False(t, store.restoreAtStartup(incoming, now))
		require.Equal(t, 7, store.snapshot(account, settings).EstimatedCapacity)
	})
}

func TestGatewayServiceStartsAnthropicAdaptiveRestoreOnlyOnce(t *testing.T) {
	stateCache := &fakeOpenAIAdaptiveStateCache{scanStarted: make(chan struct{})}
	cache := &fakeAdaptiveSchedulerGatewayCache{fakeOpenAIAdaptiveStateCache: stateCache}
	service := &GatewayService{
		cache:                      cache,
		anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler(),
	}

	service.startAnthropicAdaptiveStatePersistence()
	service.startAnthropicAdaptiveStatePersistence()
	select {
	case <-stateCache.scanStarted:
	case <-time.After(time.Second):
		t.Fatal("startup restore did not run")
	}
	require.Eventually(t, func() bool {
		scanCalls, _, _ := stateCache.counts()
		return scanCalls == 1
	}, time.Second, 10*time.Millisecond)

	stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, service.CloseAnthropicAdaptiveStatePersistence(stopCtx))
	scanCalls, _, _ := stateCache.counts()
	require.Equal(t, 1, scanCalls)
}
