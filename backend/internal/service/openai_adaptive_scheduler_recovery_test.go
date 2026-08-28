package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAdaptiveRecoveryCandidatesKeepOAuthHardLayer(t *testing.T) {
	now := time.Now()
	candidates := []openAIAdaptiveCandidateScore{
		{account: &Account{ID: 1, Type: AccountTypeAPIKey}, coreState: adaptiveAccountState{RecoveryStatus: adaptiveRecoveryStale}},
		{account: &Account{ID: 2, Type: AccountTypeOAuth}, coreState: adaptiveAccountState{RecoveryStatus: adaptiveRecoveryStale}},
	}

	got := openAIAdaptiveRecoveryCandidates(candidates, now, defaultAdaptiveCoreSettings())

	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].account.ID)
}

func TestOpenAIAdaptiveRecoveryCandidatesOrderByProbeThenCostAndID(t *testing.T) {
	now := time.Now()
	cheap := 0.2
	expensive := 0.8
	candidates := []openAIAdaptiveCandidateScore{
		{account: &Account{ID: 4, Type: AccountTypeAPIKey, RateMultiplier: &expensive}, coreState: adaptiveAccountState{LastProbeAt: now.Add(-time.Hour), RecoveryStatus: adaptiveRecoveryStale}},
		{account: &Account{ID: 3, Type: AccountTypeAPIKey, RateMultiplier: &expensive}, coreState: adaptiveAccountState{RecoveryStatus: adaptiveRecoveryStale}},
		{account: &Account{ID: 2, Type: AccountTypeAPIKey, RateMultiplier: &cheap}, coreState: adaptiveAccountState{RecoveryStatus: adaptiveRecoveryStale}},
		{account: &Account{ID: 1, Type: AccountTypeAPIKey, RateMultiplier: &cheap}, coreState: adaptiveAccountState{RecoveryStatus: adaptiveRecoveryStale}},
	}

	got := openAIAdaptiveRecoveryCandidates(candidates, now, defaultAdaptiveCoreSettings())

	require.Equal(t, []int64{1, 2, 3, 4}, []int64{got[0].account.ID, got[1].account.ID, got[2].account.ID, got[3].account.ID})
}

func TestAdaptiveRecoveryProbeLeaseAndWarmup(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.RecoveryMaxConcurrency = 2
	settings.RecoveryWarmupSuccesses = 3
	store := newAdaptiveStateStore()

	for accountID := int64(1); accountID <= 3; accountID++ {
		store.snapshot(accountID, 10, now, settings)
	}
	require.True(t, store.claimRecoveryProbe(1, "probe-1", 10, now, settings))
	require.False(t, store.claimRecoveryProbe(1, "probe-1-duplicate", 10, now, settings))
	require.True(t, store.claimRecoveryProbe(2, "probe-2", 10, now, settings))
	require.False(t, store.claimRecoveryProbe(3, "probe-3", 10, now, settings))
	store.releaseRecoveryProbe(2, "probe-2", now.Add(time.Second))
	require.True(t, store.claimRecoveryProbe(3, "probe-3", 10, now.Add(2*time.Second), settings))

	store.releaseRecoveryProbe(3, "probe-3", now.Add(3*time.Second))
	for sample := 1; sample <= 3; sample++ {
		requestID := "warmup-" + string(rune('0'+sample))
		if sample == 1 {
			store.releaseRecoveryProbe(1, "probe-1", now.Add(3*time.Second))
		}
		at := now.Add(time.Duration(3+sample) * time.Second)
		require.True(t, store.claimRecoveryProbe(1, requestID, 10, at, settings))
		store.registerAdmission(1, requestID, 10, at, settings)
		store.observe(adaptiveObservation{
			AccountID:          1,
			RequestID:          requestID,
			Type:               adaptiveObservationHealthSuccess,
			ConfiguredCapacity: 10,
		}, at.Add(time.Millisecond), settings)
		state := store.snapshot(1, 10, at.Add(time.Second), settings)
		if sample < 3 {
			require.Equal(t, adaptiveRecoveryWarming, state.RecoveryStatus)
		} else {
			require.Equal(t, adaptiveRecoveryActive, state.RecoveryStatus)
		}
		require.Equal(t, sample, state.RecoverySuccesses)
	}
}

func TestAdaptiveRecoveryInconclusiveResultDoesNotPenalizeHealth(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.snapshot(1, 10, now, settings)
	require.True(t, store.claimRecoveryProbe(1, "inconclusive", 10, now, settings))
	store.registerAdmission(1, "inconclusive", 10, now, settings)

	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "inconclusive",
		Type:               adaptiveObservationProviderOverload,
		ConfiguredCapacity: 10,
	}, now.Add(time.Second), settings)

	state := store.snapshot(1, 10, now.Add(2*time.Second), settings)
	require.Equal(t, adaptiveRecoveryStale, state.RecoveryStatus)
	require.Zero(t, state.RecoverySuccesses)
	require.Zero(t, state.ConsecutiveFailures)
	require.Empty(t, state.HealthObservations)
}

func TestAdaptiveRecoveryWarmupExpiresAndStartsANewCycle(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	state := store.ensureLocked(1, 10, now.Add(-2*settings.LearningWindow))
	state.LastDispatchAt = now.Add(-2 * settings.LearningWindow)
	state.LastProbeAt = now.Add(-2 * settings.LearningWindow)
	state.RecoveryStatus = adaptiveRecoveryWarming
	state.RecoverySuccesses = settings.RecoveryWarmupSuccesses - 1
	store.mu.Unlock()

	refreshed := store.snapshot(1, 10, now, settings)

	require.Equal(t, adaptiveRecoveryStale, refreshed.RecoveryStatus)
	require.Zero(t, refreshed.RecoverySuccesses)
}

func TestOpenAIAdaptiveRecoveryBypassesTopKWithinAPIKeyLayer(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	cfg.OpenAIAdaptiveSchedulerTopK = 1
	cfg.OpenAIAdaptiveSchedulerRecoveryExplorationRate = 1
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})

	groupID := int64(12001)
	active := Account{ID: 23001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	stale := Account{ID: 23002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	acquiredIDs := make([]int64, 0, 1)
	service := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{active, stale}}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	now := time.Now()
	scheduler.core.mu.Lock()
	activeState := scheduler.core.ensureLocked(active.ID, active.Concurrency, now)
	activeState.LastDispatchAt = now
	activeState.RecoveryStatus = adaptiveRecoveryActive
	staleState := scheduler.core.ensureLocked(stale.ID, stale.Concurrency, now)
	staleState.LastDispatchAt = now.Add(-2 * time.Duration(cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds) * time.Second)
	staleState.RecoveryStatus = adaptiveRecoveryStale
	scheduler.core.mu.Unlock()
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "recovery-top-k")

	selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	})

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, stale.ID, selection.Account.ID)
	require.Equal(t, []int64{stale.ID}, acquiredIDs)
	require.Equal(t, openAIAccountScheduleLayerAdaptive, decision.Layer)
	state := scheduler.core.snapshot(stale.ID, stale.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg))
	require.Equal(t, adaptiveRecoveryProbing, state.RecoveryStatus)
	require.False(t, state.LastProbeAt.IsZero())
	require.False(t, state.LastDispatchAt.IsZero())
	selection.ReleaseFunc()
}

func TestOpenAIAdaptiveRecoveryUsesAPIKeyOnlyWithoutUsableOAuth(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	cfg.OpenAIAdaptiveSchedulerRecoveryExplorationRate = 1
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})

	groupID := int64(12002)
	oauth := Account{ID: 23003, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}}
	apiKey := Account{ID: 23004, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	service := &OpenAIGatewayService{
		accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{oauth, apiKey}}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
			oauth.ID:  {AccountID: oauth.ID, CurrentConcurrency: 1},
			apiKey.ID: {AccountID: apiKey.ID},
		}}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "recovery-api-key")

	selection, _, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	})

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, apiKey.ID, selection.Account.ID)
	selection.ReleaseFunc()
}

func TestOpenAIAdaptiveRuntimeProbeDoesNotCrossOAuthLayer(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	cfg.OpenAIAdaptiveSchedulerRecoveryExplorationRate = 0
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})

	groupID := int64(12003)
	oauth := Account{ID: 23005, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	halfOpenAPIKey := Account{ID: 23006, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	acquiredIDs := make([]int64, 0, 1)
	service := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{oauth, halfOpenAPIKey}}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	scheduler.core.mu.Lock()
	state := scheduler.core.ensureLocked(halfOpenAPIKey.ID, halfOpenAPIKey.Concurrency, time.Now())
	state.CircuitOpenUntil = time.Now().Add(-time.Minute)
	state.CircuitOpenCount = 2
	scheduler.core.mu.Unlock()
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "oauth-hard-layer")

	selection, _, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	})

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, oauth.ID, selection.Account.ID)
	require.Equal(t, []int64{oauth.ID}, acquiredIDs)
	require.False(t, scheduler.core.snapshot(halfOpenAPIKey.ID, halfOpenAPIKey.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg)).HealthProbeInFlight)
	selection.ReleaseFunc()
}
