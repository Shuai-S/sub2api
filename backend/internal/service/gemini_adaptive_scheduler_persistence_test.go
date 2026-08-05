package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validGeminiAdaptivePersistenceState(accountID int64, updatedAt time.Time) geminiAdaptiveAccountState {
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	state := defaultGeminiAdaptiveAccountState(
		&Account{ID: accountID, Platform: PlatformGemini, Concurrency: 100},
		updatedAt,
		settings,
	)
	state.EstimatedCapacity = 40
	state.PathSuccessEMA = 0.98
	state.ByModelFamily["pro"] = geminiAdaptiveModelState{
		SuccessEMA: 0.97,
		TTFTEMA:    320,
		LatencyEMA: 1100,
		Samples:    100,
		Failures:   3,
	}
	state.AccountCircuit = geminiAdaptiveCircuitState{ConsecutiveFailure: 2, OpenUntil: updatedAt.Add(time.Minute)}
	state.ModelCircuits["gemini-2.5-pro"] = geminiAdaptiveCircuitState{
		ConsecutiveFailure: 3,
		OpenUntil:          updatedAt.Add(2 * time.Minute),
		ProbeInFlight:      true,
		ProbeUntil:         updatedAt.Add(3 * time.Minute),
		ProbeOwner:         "local-only",
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

func mustEncodeGeminiAdaptivePersistenceState(t *testing.T, state geminiAdaptiveAccountState) []byte {
	t.Helper()
	payload, err := encodeGeminiAdaptivePersistedState(state, "test-instance")
	require.NoError(t, err)
	return payload
}

func TestGeminiAdaptiveStatePersistenceFlushesOnlyDirtyRevisions(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{}
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{ID: 5001, Platform: PlatformGemini, Concurrency: 100}
	now := time.Now()
	firstToken := 250
	store.report(GeminiAdaptiveScheduleReport{
		Account:        account,
		RequestedModel: "gemini-2.5-pro",
		Success:        true,
		PathSample:     true,
		ModelSample:    true,
		CapacitySample: true,
		FirstTokenMs:   &firstToken,
		DurationMs:     900,
	}, now, settings)

	persistence := newGeminiAdaptiveStatePersistence(cache, store, nil)
	require.NoError(t, persistence.flush(t.Context()))

	_, saveCalls, _ := cache.counts()
	require.Equal(t, 1, saveCalls)
	require.Equal(t, []string{adaptiveSchedulerStateNamespaceGemini}, cache.savedNamespaces)
	require.Len(t, cache.saved, 1)
	require.Len(t, cache.saved[0], 1)
	entry := cache.saved[0][0]
	require.Equal(t, account.ID, entry.AccountID)
	require.WithinDuration(t, now.Add(adaptiveStateRetention), entry.ExpiresAt, time.Millisecond)

	restored, err := decodeGeminiAdaptivePersistedState(account.ID, entry.Payload, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(1), restored.TotalSamples)
	require.Equal(t, 250.0, restored.ByModelFamily["pro"].TTFTEMA)
	require.Equal(t, int64(1), restored.ByModelFamily["pro"].Samples)

	require.NoError(t, persistence.flush(t.Context()))
	_, saveCalls, _ = cache.counts()
	require.Equal(t, 1, saveCalls, "unchanged revisions must not be written again")

	store.report(GeminiAdaptiveScheduleReport{Account: account, PathSample: true}, now.Add(time.Second), settings)
	require.NoError(t, persistence.flush(t.Context()))
	_, saveCalls, _ = cache.counts()
	require.Equal(t, 2, saveCalls)
}

func TestGeminiAdaptiveStatePersistenceKeepsConcurrentRevisionDirty(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{}
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{ID: 5002, Platform: PlatformGemini, Concurrency: 100}
	store.report(GeminiAdaptiveScheduleReport{Account: account, Success: true, PathSample: true}, time.Now(), settings)
	cache.saveFn = func([]AdaptiveSchedulerStateCacheEntry) error {
		store.report(GeminiAdaptiveScheduleReport{Account: account, PathSample: true}, time.Now(), settings)
		return nil
	}

	persistence := newGeminiAdaptiveStatePersistence(cache, store, nil)
	require.NoError(t, persistence.flush(t.Context()))
	require.Len(t, store.dirtySnapshots(time.Now(), adaptiveStateRetention), 1)

	cache.saveFn = nil
	require.NoError(t, persistence.flush(t.Context()))
	require.Empty(t, store.dirtySnapshots(time.Now(), adaptiveStateRetention))
}

func TestGeminiAdaptiveStatePersistenceRestoresFreshSnapshotsOnly(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	fresh := validGeminiAdaptivePersistenceState(6001, now.Add(-time.Hour))
	stale := validGeminiAdaptivePersistenceState(6002, now.Add(-13*time.Hour))
	cache := &fakeOpenAIAdaptiveStateCache{
		scanFn: func(uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
			return []AdaptiveSchedulerStateCacheRecord{
				{AccountID: fresh.AccountID, Payload: mustEncodeGeminiAdaptivePersistenceState(t, fresh)},
				{AccountID: stale.AccountID, Payload: mustEncodeGeminiAdaptivePersistenceState(t, stale)},
				{AccountID: 6003, Payload: []byte("not-json")},
			}, 0, nil
		},
	}
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	persistence := newGeminiAdaptiveStatePersistence(cache, store, func(context.Context) GeminiAdaptiveSchedulerSettings {
		return settings
	})
	persistence.now = func() time.Time { return now }

	restored, staleCount, invalid, err := persistence.restoreOnce(t.Context())

	require.NoError(t, err)
	require.Equal(t, 1, restored)
	require.Equal(t, 1, staleCount)
	require.Equal(t, 1, invalid)
	require.Equal(t, []string{adaptiveSchedulerStateNamespaceGemini}, cache.scanNamespaces)
	require.NotContains(t, store.accounts, stale.AccountID)
	state := store.snapshot(&Account{ID: fresh.AccountID, Concurrency: 100}, settings)
	require.Equal(t, 40, state.EstimatedCapacity)
	require.Zero(t, state.RecentHealthSamples, "expired recent learning window must be reset")
	require.Zero(t, state.RecentCapacitySamples)
	require.Equal(t, 320.0, state.ByModelFamily["pro"].TTFTEMA)
	require.Equal(t, 2, state.AccountCircuit.ConsecutiveFailure)
	require.Equal(t, 3, state.ModelCircuits["gemini-2.5-pro"].ConsecutiveFailure)
	require.False(t, state.ModelCircuits["gemini-2.5-pro"].ProbeInFlight, "probe ownership must remain instance-local")
}

func TestGeminiAdaptiveStatePersistenceDiscardsPartialScanOnFailure(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	state := validGeminiAdaptivePersistenceState(7001, now.Add(-time.Hour))
	cache := &fakeOpenAIAdaptiveStateCache{
		scanFn: func(cursor uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
			if cursor == 0 {
				return []AdaptiveSchedulerStateCacheRecord{{
					AccountID: state.AccountID,
					Payload:   mustEncodeGeminiAdaptivePersistenceState(t, state),
				}}, 1, nil
			}
			return nil, 0, errors.New("redis unavailable")
		},
	}
	store := newGeminiAdaptiveStateStore()
	persistence := newGeminiAdaptiveStatePersistence(cache, store, nil)
	persistence.now = func() time.Time { return now }

	_, _, _, err := persistence.restoreOnce(t.Context())

	require.ErrorContains(t, err, "redis unavailable")
	require.NotContains(t, store.accounts, state.AccountID, "a failed scan must not apply a partial restore")
}
