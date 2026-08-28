package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeAdaptiveStateCache struct {
	mu sync.Mutex

	scanFn          func(cursor uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error)
	scanRecords     []AdaptiveSchedulerStateCacheRecord
	scanErr         error
	scanCalls       int
	scanNamespaces  []string
	saveFn          func(namespace string, entries []AdaptiveSchedulerStateCacheEntry) error
	saveCalls       int
	saved           [][]AdaptiveSchedulerStateCacheEntry
	savedNamespaces []string
	savedTTLs       []time.Duration

	cleanupCalls int
}

func (c *fakeAdaptiveStateCache) ScanAdaptiveSchedulerStates(
	_ context.Context,
	namespace string,
	cursor uint64,
	_ int64,
) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
	c.mu.Lock()
	c.scanCalls++
	c.scanNamespaces = append(c.scanNamespaces, namespace)
	fn := c.scanFn
	records := append([]AdaptiveSchedulerStateCacheRecord(nil), c.scanRecords...)
	err := c.scanErr
	c.mu.Unlock()
	if fn != nil {
		return fn(cursor)
	}
	if cursor != 0 {
		return nil, 0, err
	}
	return records, 0, err
}

func (c *fakeAdaptiveStateCache) SaveAdaptiveSchedulerStates(
	_ context.Context,
	namespace string,
	entries []AdaptiveSchedulerStateCacheEntry,
	ttl time.Duration,
) error {
	copyEntries := make([]AdaptiveSchedulerStateCacheEntry, len(entries))
	for i, entry := range entries {
		copyEntries[i] = entry
		copyEntries[i].Payload = append([]byte(nil), entry.Payload...)
	}
	c.mu.Lock()
	c.saveCalls++
	c.savedNamespaces = append(c.savedNamespaces, namespace)
	c.saved = append(c.saved, copyEntries)
	c.savedTTLs = append(c.savedTTLs, ttl)
	fn := c.saveFn
	c.mu.Unlock()
	if fn != nil {
		return fn(namespace, copyEntries)
	}
	return nil
}

func (c *fakeAdaptiveStateCache) DeleteExpiredAdaptiveSchedulerStates(
	_ context.Context,
	_ string,
	_ time.Time,
	_ int64,
) (int64, error) {
	c.mu.Lock()
	c.cleanupCalls++
	c.mu.Unlock()
	return 0, nil
}

func (c *fakeAdaptiveStateCache) counts() (save, cleanup int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveCalls, c.cleanupCalls
}

func (c *fakeAdaptiveStateCache) scanCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scanCalls
}

func TestAdaptiveCorePersistenceFlushesOnlyDirtyV2Snapshots(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	cache := &fakeAdaptiveStateCache{}
	store := newAdaptiveStateStore()
	settings := defaultAdaptiveCoreSettings()
	store.observe(adaptiveObservation{
		AccountID:          1001,
		RequestID:          "success-1",
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 100,
		FirstTokenMs:       intPtr(240),
	}, now, settings)
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceOpenAI)
	persistence.now = func() time.Time { return now }

	require.NoError(t, persistence.flush(t.Context()))
	saveCalls, _ := cache.counts()
	require.Equal(t, 1, saveCalls)
	require.Equal(t, []string{adaptiveSchedulerCoreNamespaceOpenAI}, cache.savedNamespaces)
	require.Equal(t, adaptiveStateHashTTL, cache.savedTTLs[0])
	require.Len(t, cache.saved[0], 1)
	require.WithinDuration(t, now.Add(adaptiveStateRetention), cache.saved[0][0].ExpiresAt, time.Millisecond)

	var persisted adaptiveCorePersistedState
	require.NoError(t, json.Unmarshal(cache.saved[0][0].Payload, &persisted))
	require.Equal(t, adaptiveSchedulerStateVersion, persisted.Version)
	require.Equal(t, int64(1001), persisted.State.AccountID)
	require.Equal(t, 100, persisted.State.EffectiveCapacity)
	require.Equal(t, int64(1), persisted.State.TTFTSamples)
	require.Equal(t, 240.0, persisted.State.TTFTEMA)

	require.NoError(t, persistence.flush(t.Context()))
	saveCalls, _ = cache.counts()
	require.Equal(t, 1, saveCalls)
}

func TestAdaptiveCorePersistenceKeepsConcurrentRevisionDirty(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	cache := &fakeAdaptiveStateCache{}
	store := newAdaptiveStateStore()
	settings := defaultAdaptiveCoreSettings()
	store.observe(adaptiveObservation{AccountID: 1002, RequestID: "success", Type: adaptiveObservationHealthSuccess, ConfiguredCapacity: 10}, now, settings)
	cache.saveFn = func(_ string, _ []AdaptiveSchedulerStateCacheEntry) error {
		store.observe(adaptiveObservation{AccountID: 1002, RequestID: "failure", Type: adaptiveObservationAccountFailure, ConfiguredCapacity: 10}, now.Add(time.Second), settings)
		cache.mu.Lock()
		cache.saveFn = nil
		cache.mu.Unlock()
		return nil
	}
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceAnthropic)
	persistence.now = func() time.Time { return now.Add(2 * time.Second) }

	require.NoError(t, persistence.flush(t.Context()))
	require.Len(t, store.dirtySnapshots(now.Add(2*time.Second), adaptiveStateRetention), 1)
	require.NoError(t, persistence.flush(t.Context()))
	saveCalls, _ := cache.counts()
	require.Equal(t, 2, saveCalls)
}

func TestAdaptiveCorePersistenceRestoresValidV2SnapshotOnceAtStartup(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	state := *newAdaptiveAccountState(1003, 20, now.Add(-time.Minute))
	state.SuccessEMA = 0.91
	state.TTFTEMA = 240
	state.TTFTSamples = 4
	state.HealthObservations = []adaptiveHealthObservation{{At: now.Add(-time.Minute), Success: true}}
	cache := &fakeAdaptiveStateCache{scanRecords: []AdaptiveSchedulerStateCacheRecord{{
		AccountID: state.AccountID,
		Payload:   adaptiveCorePersistenceTestPayload(t, now, state),
	}}}
	store := newAdaptiveStateStore()
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceGemini)
	persistence.now = func() time.Time { return now }
	persistence.worker.now = persistence.now
	persistence.worker.flushInterval = time.Hour
	persistence.worker.flushJitter = 0

	persistence.Start()
	persistence.Start()
	require.Eventually(t, func() bool { return cache.scanCount() == 1 }, time.Second, time.Millisecond)
	require.NoError(t, persistence.Stop(t.Context()))

	store.mu.RLock()
	restored := cloneAdaptiveAccountState(store.accounts[state.AccountID])
	store.mu.RUnlock()
	require.Equal(t, 0.91, restored.SuccessEMA)
	require.Equal(t, 240.0, restored.TTFTEMA)
	require.Equal(t, int64(4), restored.TTFTSamples)
	require.Equal(t, uint64(1), restored.revision)
	require.Equal(t, uint64(1), restored.persistedRevision)
	require.Empty(t, store.dirtySnapshots(now, adaptiveStateRetention))
	require.Equal(t, []string{adaptiveSchedulerCoreNamespaceGemini}, cache.scanNamespaces)

	require.NoError(t, persistence.flush(t.Context()))
	require.Equal(t, 1, cache.scanCount(), "periodic persistence must not read Redis again")
}

func TestAdaptiveCorePersistenceRoundTripsWindowedTTFTSamples(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	bucketKey := "gpt-5.1|responses|http"
	firstToken := 4200
	store.observe(adaptiveObservation{
		AccountID:          1004,
		RequestID:          "windowed-success",
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10,
		FirstTokenMs:       &firstToken,
		TTFTBucketKey:      bucketKey,
		WindowedTTFT:       true,
	}, now.Add(-time.Minute), settings)
	cache := &fakeAdaptiveStateCache{}
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceOpenAI)
	persistence.now = func() time.Time { return now }

	require.NoError(t, persistence.flush(t.Context()))
	require.Len(t, cache.saved, 1)
	require.Len(t, cache.saved[0], 1)

	restoredStore := newAdaptiveStateStore()
	restoredCache := &fakeAdaptiveStateCache{scanRecords: []AdaptiveSchedulerStateCacheRecord{{
		AccountID: 1004,
		Payload:   cache.saved[0][0].Payload,
	}}}
	restoredPersistence := newAdaptiveCoreStatePersistence(restoredCache, restoredStore, adaptiveSchedulerCoreNamespaceOpenAI)
	restoredPersistence.now = func() time.Time { return now }
	restored, stale, invalid, err := restoredPersistence.restoreOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restored)
	require.Zero(t, stale)
	require.Zero(t, invalid)

	state := restoredStore.snapshot(1004, 10, now, settings)
	stats := adaptiveTTFTWindowStatsForState(state, bucketKey, now, settings)
	require.Equal(t, 1, stats.Samples)
	require.Equal(t, 4200.0, stats.P50)
}

func TestAdaptiveCorePersistenceDropsLegacyOpenAITTFTOnly(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	state := *newAdaptiveAccountState(1005, 10, now.Add(-time.Minute))
	state.SuccessEMA = 0.93
	state.TTFTEMA = 9000
	state.TTFTSamples = 5000
	cache := &fakeAdaptiveStateCache{scanRecords: []AdaptiveSchedulerStateCacheRecord{{
		AccountID: state.AccountID,
		Payload:   adaptiveCorePersistenceTestPayload(t, now, state),
	}}}
	store := newAdaptiveStateStore()
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceOpenAI)
	persistence.now = func() time.Time { return now }

	restored, stale, invalid, err := persistence.restoreOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restored)
	require.Zero(t, stale)
	require.Zero(t, invalid)
	restoredState := store.snapshot(state.AccountID, 10, now, defaultAdaptiveCoreSettings())
	require.Equal(t, 0.93, restoredState.SuccessEMA)
	require.Zero(t, restoredState.TTFTEMA)
	require.Zero(t, restoredState.TTFTSamples)
}

func TestAdaptiveCorePersistenceSkipsStaleAndInvalidSnapshots(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	valid := *newAdaptiveAccountState(1101, 10, now.Add(-time.Minute))
	stale := *newAdaptiveAccountState(1102, 10, now.Add(-adaptiveStateRetention-time.Minute))
	invalid := *newAdaptiveAccountState(1103, 10, now.Add(-time.Minute))
	invalid.SuccessEMA = 2
	cache := &fakeAdaptiveStateCache{scanRecords: []AdaptiveSchedulerStateCacheRecord{
		{AccountID: valid.AccountID, Payload: adaptiveCorePersistenceTestPayload(t, now, valid)},
		{AccountID: stale.AccountID, Payload: adaptiveCorePersistenceTestPayload(t, stale.UpdatedAt, stale)},
		{AccountID: invalid.AccountID, Payload: adaptiveCorePersistenceTestPayload(t, now, invalid)},
	}}
	store := newAdaptiveStateStore()
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceOpenAI)
	persistence.now = func() time.Time { return now }

	restored, staleCount, invalidCount, err := persistence.restoreOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restored)
	require.Equal(t, 1, staleCount)
	require.Equal(t, 1, invalidCount)
	store.mu.RLock()
	require.Contains(t, store.accounts, valid.AccountID)
	require.NotContains(t, store.accounts, stale.AccountID)
	require.NotContains(t, store.accounts, invalid.AccountID)
	store.mu.RUnlock()
}

func TestAdaptiveCorePersistenceScanFailureLeavesColdState(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	state := *newAdaptiveAccountState(1201, 10, now.Add(-time.Minute))
	cache := &fakeAdaptiveStateCache{}
	cache.scanFn = func(cursor uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
		if cursor == 0 {
			return []AdaptiveSchedulerStateCacheRecord{{AccountID: state.AccountID, Payload: adaptiveCorePersistenceTestPayload(t, now, state)}}, 1, nil
		}
		return nil, 0, context.DeadlineExceeded
	}
	store := newAdaptiveStateStore()
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceAnthropic)
	persistence.now = func() time.Time { return now }

	restored, staleCount, invalidCount, err := persistence.restoreOnce(t.Context())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, restored)
	require.Zero(t, staleCount)
	require.Zero(t, invalidCount)
	store.mu.RLock()
	require.Empty(t, store.accounts, "a partial Redis scan must fall back to a full cold start")
	store.mu.RUnlock()
}

func TestAdaptiveCorePersistenceRestoreFailureStillCheckpointsNewLocalState(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	cache := &fakeAdaptiveStateCache{scanErr: context.DeadlineExceeded}
	store := newAdaptiveStateStore()
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceGemini)
	persistence.now = func() time.Time { return now }
	persistence.worker.flushInterval = 5 * time.Millisecond
	persistence.worker.flushJitter = 0

	persistence.Start()
	require.Eventually(t, func() bool { return cache.scanCount() == 1 }, time.Second, time.Millisecond)
	store.observe(adaptiveObservation{
		AccountID:          1251,
		RequestID:          "post-cold-start",
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10,
	}, now, defaultAdaptiveCoreSettings())
	require.Eventually(t, func() bool {
		saveCalls, _ := cache.counts()
		return saveCalls == 1
	}, time.Second, time.Millisecond)
	require.NoError(t, persistence.Stop(t.Context()))
	require.Equal(t, 1, cache.scanCount(), "a failed startup restore must not be retried during runtime")
}

func TestAdaptiveCorePersistenceDoesNotOverwriteEarlyLocalActivity(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	incoming := *newAdaptiveAccountState(1301, 10, now.Add(-time.Minute))
	incoming.SuccessEMA = 0.1
	cache := &fakeAdaptiveStateCache{scanRecords: []AdaptiveSchedulerStateCacheRecord{{
		AccountID: incoming.AccountID,
		Payload:   adaptiveCorePersistenceTestPayload(t, now, incoming),
	}}}
	store := newAdaptiveStateStore()
	store.observe(adaptiveObservation{
		AccountID:          incoming.AccountID,
		RequestID:          "early-request",
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10,
	}, now, defaultAdaptiveCoreSettings())
	persistence := newAdaptiveCoreStatePersistence(cache, store, adaptiveSchedulerCoreNamespaceOpenAI)
	persistence.now = func() time.Time { return now }

	restored, staleCount, invalidCount, err := persistence.restoreOnce(t.Context())
	require.NoError(t, err)
	require.Zero(t, restored)
	require.Zero(t, staleCount)
	require.Zero(t, invalidCount)
	local := store.snapshot(incoming.AccountID, 10, now, defaultAdaptiveCoreSettings())
	require.Greater(t, local.SuccessEMA, incoming.SuccessEMA)
}

func adaptiveCorePersistenceTestPayload(t *testing.T, savedAt time.Time, state adaptiveAccountState) []byte {
	t.Helper()
	payload, err := json.Marshal(adaptiveCorePersistedState{
		Version: adaptiveSchedulerStateVersion,
		SavedAt: savedAt,
		State:   state,
	})
	require.NoError(t, err)
	return payload
}

func TestAdaptiveCorePersistenceUsesFiveMinuteFlushAndProviderV2Namespaces(t *testing.T) {
	require.Equal(t, 5*time.Minute, adaptiveStateFlushInterval)
	require.Equal(t, "openai_v2", adaptiveSchedulerCoreNamespaceOpenAI)
	require.Equal(t, "anthropic_v2", adaptiveSchedulerCoreNamespaceAnthropic)
	require.Equal(t, "gemini_v2", adaptiveSchedulerCoreNamespaceGemini)
}
