package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiStickyMigrationCacheStub struct {
	bindings       map[string]int64
	leases         map[string]string
	acquireResults []bool
	swapCalls      int
	deleteCalls    int
	releaseCalls   int
}

func (c *geminiStickyMigrationCacheStub) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	return c.bindings[sessionHash], nil
}

func (c *geminiStickyMigrationCacheStub) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, _ time.Duration) error {
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	c.bindings[sessionHash] = accountID
	return nil
}

func (c *geminiStickyMigrationCacheStub) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *geminiStickyMigrationCacheStub) DeleteSessionAccountID(_ context.Context, _ int64, sessionHash string) error {
	delete(c.bindings, sessionHash)
	return nil
}

func (c *geminiStickyMigrationCacheStub) SetGrokVideoPendingBilling(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

func (c *geminiStickyMigrationCacheStub) GetGrokVideoPendingBilling(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (c *geminiStickyMigrationCacheStub) ClaimGrokVideoBilled(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (c *geminiStickyMigrationCacheStub) ReleaseGrokVideoBilled(_ context.Context, _ string) error {
	return nil
}

func (c *geminiStickyMigrationCacheStub) TryAcquireSessionMigrationLease(_ context.Context, _ int64, sessionHash, token string, _ time.Duration) (bool, error) {
	acquired := true
	if len(c.acquireResults) > 0 {
		acquired = c.acquireResults[0]
		c.acquireResults = c.acquireResults[1:]
	}
	if !acquired {
		return false, nil
	}
	if c.leases == nil {
		c.leases = make(map[string]string)
	}
	if _, exists := c.leases[sessionHash]; exists {
		return false, nil
	}
	c.leases[sessionHash] = token
	return true, nil
}

func (c *geminiStickyMigrationCacheStub) CompareAndSwapSessionAccountID(_ context.Context, _ int64, sessionHash string, expectedAccountID, targetAccountID int64, token string, _ time.Duration) (bool, error) {
	c.swapCalls++
	if c.leases[sessionHash] != token || c.bindings[sessionHash] != expectedAccountID {
		return false, nil
	}
	c.bindings[sessionHash] = targetAccountID
	return true, nil
}

func (c *geminiStickyMigrationCacheStub) CompareAndDeleteSessionAccountID(_ context.Context, _ int64, sessionHash string, expectedAccountID int64, token string) (bool, error) {
	c.deleteCalls++
	if c.leases[sessionHash] != token || c.bindings[sessionHash] != expectedAccountID {
		return false, nil
	}
	delete(c.bindings, sessionHash)
	return true, nil
}

func (c *geminiStickyMigrationCacheStub) ReleaseSessionMigrationLease(_ context.Context, _ int64, sessionHash, token string) (bool, error) {
	c.releaseCalls++
	if c.leases[sessionHash] != token {
		return false, nil
	}
	delete(c.leases, sessionHash)
	return true, nil
}

func TestGeminiStickyMigrationPrepareAndCommitUsesLeaseAndCAS(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	cache := &geminiStickyMigrationCacheStub{
		bindings:       map[string]int64{"session": 101},
		acquireResults: []bool{true},
	}
	svc := &GatewayService{cache: cache, geminiAdaptiveScheduler: newGeminiAdaptiveScheduler()}
	released := false
	selection := &AccountSelectionResult{
		Account:     &Account{ID: 202, Platform: PlatformGemini},
		ReleaseFunc: func() { released = true },
	}

	prepared, err := svc.prepareGeminiStickyMigration(context.Background(), selection, nil, "session", 101, true)

	require.NoError(t, err)
	require.Same(t, selection, prepared)
	require.False(t, released)
	require.True(t, prepared.PreserveStickyBinding)
	require.NotNil(t, prepared.PendingGeminiMigration)
	require.Equal(t, int64(101), prepared.PendingGeminiMigration.SignatureSourceAccountID)
	require.Equal(t, int64(202), prepared.PendingGeminiMigration.ToAccountID)
	require.True(t, prepared.PendingGeminiMigration.DeleteSourceOnFailure)

	require.NoError(t, svc.CommitGeminiStickyMigration(context.Background(), prepared.PendingGeminiMigration))
	require.Equal(t, int64(202), cache.bindings["session"])
	require.Equal(t, 1, cache.swapCalls)
	require.Equal(t, 1, cache.releaseCalls)
	require.Equal(t, uint64(1), svc.geminiAdaptiveScheduler.SnapshotMetrics().StickyMigrateTotal)

	require.NoError(t, svc.CommitGeminiStickyMigration(context.Background(), prepared.PendingGeminiMigration))
	require.Equal(t, 1, cache.swapCalls, "a completed migration must be idempotent")
	require.Contains(t, output.String(), "gemini_adaptive_sticky_migration_prepared")
	require.Contains(t, output.String(), "gemini_adaptive_sticky_migration_committed")
	require.NotContains(t, output.String(), prepared.PendingGeminiMigration.LeaseToken)
}

func TestGeminiStickyMigrationLeaseCompetitionUsesCompletedBinding(t *testing.T) {
	cache := &geminiStickyMigrationCacheStub{
		bindings:       map[string]int64{"session": 202},
		acquireResults: []bool{false},
	}
	svc := &GatewayService{cache: cache}
	selection := &AccountSelectionResult{Account: &Account{ID: 202, Platform: PlatformGemini}}

	prepared, err := svc.prepareGeminiStickyMigration(context.Background(), selection, nil, "session", 101, false)

	require.NoError(t, err)
	require.Same(t, selection, prepared)
	require.True(t, prepared.PreserveStickyBinding)
	require.Nil(t, prepared.PendingGeminiMigration)
}

func TestGeminiStickyMigrationBindingChangeReleasesSelectionAndRetries(t *testing.T) {
	cache := &geminiStickyMigrationCacheStub{
		bindings:       map[string]int64{"session": 303},
		acquireResults: []bool{true},
	}
	svc := &GatewayService{cache: cache}
	released := false
	selection := &AccountSelectionResult{
		Account:     &Account{ID: 202, Platform: PlatformGemini},
		ReleaseFunc: func() { released = true },
	}

	prepared, err := svc.prepareGeminiStickyMigration(context.Background(), selection, nil, "session", 101, false)

	require.Nil(t, prepared)
	require.True(t, released)
	var retryErr *geminiStickyMigrationRetryError
	require.True(t, errors.As(err, &retryErr))
	require.Equal(t, int64(303), retryErr.AccountID)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestGeminiStickyMigrationAbortDeletesOnlyHardFailedSource(t *testing.T) {
	t.Run("hard source failure", func(t *testing.T) {
		cache := &geminiStickyMigrationCacheStub{
			bindings: map[string]int64{"session": 101},
			leases:   map[string]string{"session": "token"},
		}
		svc := &GatewayService{cache: cache}
		migration := &GeminiPendingStickyMigration{
			GroupID:               1,
			SessionKey:            "session",
			ExpectedAccountID:     101,
			LeaseToken:            "token",
			DeleteSourceOnFailure: true,
		}

		svc.AbortGeminiStickyMigration(context.Background(), migration)

		require.NotContains(t, cache.bindings, "session")
		require.Equal(t, 1, cache.deleteCalls)
		require.Equal(t, 1, cache.releaseCalls)
	})

	t.Run("capacity escape failure", func(t *testing.T) {
		cache := &geminiStickyMigrationCacheStub{
			bindings: map[string]int64{"session": 101},
			leases:   map[string]string{"session": "token"},
		}
		svc := &GatewayService{cache: cache}
		migration := &GeminiPendingStickyMigration{
			GroupID:           1,
			SessionKey:        "session",
			ExpectedAccountID: 101,
			LeaseToken:        "token",
		}

		svc.AbortGeminiStickyMigration(context.Background(), migration)

		require.Equal(t, int64(101), cache.bindings["session"])
		require.Zero(t, cache.deleteCalls)
		require.Equal(t, 1, cache.releaseCalls)
	})
}
