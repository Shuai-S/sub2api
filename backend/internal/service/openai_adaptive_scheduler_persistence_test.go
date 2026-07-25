package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeOpenAIAdaptiveStateCache struct {
	mu sync.Mutex

	scanFn         func(cursor uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error)
	scanCalls      int
	scanNamespaces []string
	scanStarted    chan struct{}
	scanSignal     sync.Once

	saveCalls       int
	saved           [][]AdaptiveSchedulerStateCacheEntry
	savedTTLs       []time.Duration
	savedNamespaces []string
	saveFn          func(entries []AdaptiveSchedulerStateCacheEntry) error

	cleanupCalls      int
	cleanupNamespaces []string
}

func (c *fakeOpenAIAdaptiveStateCache) ScanAdaptiveSchedulerStates(
	_ context.Context,
	namespace string,
	cursor uint64,
	_ int64,
) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
	c.mu.Lock()
	c.scanCalls++
	c.scanNamespaces = append(c.scanNamespaces, namespace)
	fn := c.scanFn
	started := c.scanStarted
	c.mu.Unlock()
	if started != nil {
		c.scanSignal.Do(func() { close(started) })
	}
	if fn == nil {
		return nil, 0, nil
	}
	return fn(cursor)
}

func (c *fakeOpenAIAdaptiveStateCache) SaveAdaptiveSchedulerStates(
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
		return fn(copyEntries)
	}
	return nil
}

func (c *fakeOpenAIAdaptiveStateCache) DeleteExpiredAdaptiveSchedulerStates(
	_ context.Context,
	namespace string,
	_ time.Time,
	_ int64,
) (int64, error) {
	c.mu.Lock()
	c.cleanupCalls++
	c.cleanupNamespaces = append(c.cleanupNamespaces, namespace)
	c.mu.Unlock()
	return 0, nil
}

func (c *fakeOpenAIAdaptiveStateCache) counts() (scan, save, cleanup int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scanCalls, c.saveCalls, c.cleanupCalls
}

func validOpenAIAdaptivePersistenceState(accountID int64, updatedAt time.Time) openAIAdaptiveAccountState {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	state := defaultOpenAIAdaptiveAccountState(accountID, cfg)
	state.EstimatedCapacity = 40
	state.SuccessEMA = 0.98
	state.ErrorEMA = 0.02
	state.LatencyEMA = 1200
	state.TTFTEMA = 350
	state.ThompsonAlpha = 101
	state.ThompsonBeta = 3
	state.TotalSamples = 102
	state.RecentSamples = 12
	state.RecentFailures = 1
	state.UpdatedAt = updatedAt
	return state
}

func mustEncodeOpenAIAdaptivePersistenceState(t *testing.T, state openAIAdaptiveAccountState) []byte {
	t.Helper()
	payload, err := encodeOpenAIAdaptivePersistedState(state, "test-instance")
	require.NoError(t, err)
	return payload
}

func TestOpenAIAdaptiveStatePersistenceFlushesOnlyDirtyRevisions(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{}
	store := newOpenAIAdaptiveSchedulerStateStore()
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	account := &Account{ID: 1001, Concurrency: 100}
	store.reportWithAccount(account, account.ID, cfg, true, intPtr(240), 900)

	persistence := newOpenAIAdaptiveStatePersistence(cache, store)
	require.NoError(t, persistence.flush(t.Context()))

	_, saveCalls, _ := cache.counts()
	require.Equal(t, 1, saveCalls)
	require.Len(t, cache.saved, 1)
	require.Len(t, cache.saved[0], 1)
	require.Equal(t, openAIAdaptiveStateHashTTL, cache.savedTTLs[0])
	entry := cache.saved[0][0]
	require.Equal(t, account.ID, entry.AccountID)
	require.WithinDuration(t, store.snapshot(account.ID, cfg).UpdatedAt.Add(openAIAdaptiveStateRetention), entry.ExpiresAt, time.Millisecond)

	restored, err := decodeOpenAIAdaptivePersistedState(account.ID, entry.Payload, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(1), restored.TotalSamples)
	require.Equal(t, 240.0, restored.TTFTEMA)

	require.NoError(t, persistence.flush(t.Context()))
	_, saveCalls, _ = cache.counts()
	require.Equal(t, 1, saveCalls, "unchanged revisions must not be written again")

	store.reportWithAccount(account, account.ID, cfg, false, nil, 0)
	require.NoError(t, persistence.flush(t.Context()))
	_, saveCalls, _ = cache.counts()
	require.Equal(t, 2, saveCalls)
}

func TestOpenAIAdaptiveStatePersistenceKeepsConcurrentRevisionDirty(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{}
	store := newOpenAIAdaptiveSchedulerStateStore()
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	account := &Account{ID: 1002, Concurrency: 100}
	store.reportWithAccount(account, account.ID, cfg, true, nil, 0)
	cache.saveFn = func([]AdaptiveSchedulerStateCacheEntry) error {
		store.reportWithAccount(account, account.ID, cfg, false, nil, 0)
		return nil
	}

	persistence := newOpenAIAdaptiveStatePersistence(cache, store)
	require.NoError(t, persistence.flush(t.Context()))
	require.Len(t, store.dirtySnapshots(time.Now(), openAIAdaptiveStateRetention), 1)

	cache.saveFn = nil
	require.NoError(t, persistence.flush(t.Context()))
	require.Empty(t, store.dirtySnapshots(time.Now(), openAIAdaptiveStateRetention))
}

func TestOpenAIAdaptiveStatePersistenceRestoresFreshSnapshotsOnly(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	fresh := validOpenAIAdaptivePersistenceState(2001, now.Add(-time.Hour))
	stale := validOpenAIAdaptivePersistenceState(2002, now.Add(-13*time.Hour))
	cache := &fakeOpenAIAdaptiveStateCache{
		scanFn: func(uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
			return []AdaptiveSchedulerStateCacheRecord{
				{AccountID: fresh.AccountID, Payload: mustEncodeOpenAIAdaptivePersistenceState(t, fresh)},
				{AccountID: stale.AccountID, Payload: mustEncodeOpenAIAdaptivePersistenceState(t, stale)},
				{AccountID: 2003, Payload: []byte("not-json")},
			}, 0, nil
		},
	}
	store := newOpenAIAdaptiveSchedulerStateStore()
	persistence := newOpenAIAdaptiveStatePersistence(cache, store)
	persistence.now = func() time.Time { return now }

	restored, staleCount, invalid, err := persistence.restoreOnce(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restored)
	require.Equal(t, 1, staleCount)
	require.Equal(t, 1, invalid)
	require.True(t, store.has(fresh.AccountID))
	require.False(t, store.has(stale.AccountID))
	require.Equal(t, 40, store.snapshot(fresh.AccountID, DefaultOpenAIAdaptiveSchedulerSettings()).EstimatedCapacity)
}

func TestOpenAIAdaptiveStatePersistenceDiscardsPartialScanOnFailure(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	state := validOpenAIAdaptivePersistenceState(3001, now.Add(-time.Hour))
	cache := &fakeOpenAIAdaptiveStateCache{
		scanFn: func(cursor uint64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error) {
			if cursor == 0 {
				return []AdaptiveSchedulerStateCacheRecord{{AccountID: state.AccountID, Payload: mustEncodeOpenAIAdaptivePersistenceState(t, state)}}, 1, nil
			}
			return nil, 0, errors.New("redis unavailable")
		},
	}
	store := newOpenAIAdaptiveSchedulerStateStore()
	persistence := newOpenAIAdaptiveStatePersistence(cache, store)
	persistence.now = func() time.Time { return now }

	_, _, _, err := persistence.restoreOnce(t.Context())
	require.ErrorContains(t, err, "redis unavailable")
	require.False(t, store.has(state.AccountID), "a failed scan must not apply a partial restore")
}

func TestOpenAIAdaptiveStateStoreStartupMergePreservesLocalSafety(t *testing.T) {
	now := time.Now()
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	incoming := validOpenAIAdaptivePersistenceState(4001, now.Add(-time.Hour))
	incoming.EstimatedCapacity = 40

	t.Run("early success keeps learned capacity", func(t *testing.T) {
		store := newOpenAIAdaptiveSchedulerStateStore()
		account := &Account{ID: incoming.AccountID, Concurrency: 100}
		store.reportWithAccount(account, account.ID, cfg, true, nil, 0)

		require.True(t, store.restoreAtStartup(incoming, now))
		state := store.snapshot(account.ID, cfg)
		require.Equal(t, 40, state.EstimatedCapacity)
		require.Equal(t, incoming.TotalSamples+1, state.TotalSamples)
	})

	t.Run("early failure keeps lower capacity and cooldown", func(t *testing.T) {
		store := newOpenAIAdaptiveSchedulerStateStore()
		local := defaultOpenAIAdaptiveAccountState(incoming.AccountID, cfg)
		local.EstimatedCapacity = 3
		local.TotalSamples = 1
		local.RecentSamples = 1
		local.RecentFailures = 1
		local.ConsecutiveFailure = 1
		local.ConsecutiveCapacityFailure = 1
		local.ErrorEMA = 0.4
		local.SuccessEMA = 0.5
		local.CooldownUntil = now.Add(time.Minute)
		local.LastFailureAt = now
		local.UpdatedAt = now
		local.revision = 1
		store.states[incoming.AccountID] = &local

		require.True(t, store.restoreAtStartup(incoming, now))
		state := store.snapshot(incoming.AccountID, cfg)
		require.Equal(t, 3, state.EstimatedCapacity)
		require.Equal(t, 0.4, state.ErrorEMA)
		require.Equal(t, local.CooldownUntil, state.CooldownUntil)
	})

	t.Run("established local learning wins", func(t *testing.T) {
		store := newOpenAIAdaptiveSchedulerStateStore()
		local := validOpenAIAdaptivePersistenceState(incoming.AccountID, now)
		local.EstimatedCapacity = 7
		local.TotalSamples = openAIAdaptiveStateLocalMergeSampleLimit
		store.states[incoming.AccountID] = &local

		require.False(t, store.restoreAtStartup(incoming, now))
		require.Equal(t, 7, store.snapshot(incoming.AccountID, cfg).EstimatedCapacity)
	})
}

func TestOpenAIAdaptiveStatePersistenceStartsRestoreOnlyOnce(t *testing.T) {
	cache := &fakeOpenAIAdaptiveStateCache{scanStarted: make(chan struct{})}
	store := newOpenAIAdaptiveSchedulerStateStore()
	persistence := newOpenAIAdaptiveStatePersistence(cache, store)
	persistence.worker.flushInterval = time.Hour
	persistence.worker.flushJitter = 0

	persistence.Start()
	persistence.Start()
	select {
	case <-cache.scanStarted:
	case <-time.After(time.Second):
		t.Fatal("startup restore did not run")
	}
	require.Eventually(t, func() bool {
		scanCalls, _, _ := cache.counts()
		return scanCalls == 1
	}, time.Second, 10*time.Millisecond)

	stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, persistence.Stop(stopCtx))
	scanCalls, _, _ := cache.counts()
	require.Equal(t, 1, scanCalls)
}

func TestAdaptiveOpenAIAccountSchedulerUsesServiceStateStore(t *testing.T) {
	store := newOpenAIAdaptiveSchedulerStateStore()
	service := &OpenAIGatewayService{openaiAdaptiveState: store}
	scheduler, ok := newAdaptiveOpenAIAccountScheduler(service, nil).(*adaptiveOpenAIAccountScheduler)
	require.True(t, ok)
	require.Same(t, store, scheduler.state)
	require.Same(t, store, service.openAIAdaptiveSchedulerStateStoreForSnapshot())
}
