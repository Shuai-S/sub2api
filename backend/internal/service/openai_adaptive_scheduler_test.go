package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAdaptiveSchedulerCostScoreUsesRateMultiplier(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	lowCost := 0.5
	highCost := 2.0
	candidates := []openAIAdaptiveCandidateScore{
		{
			account:           &Account{ID: 1, RateMultiplier: &lowCost},
			loadInfo:          &AccountLoadInfo{},
			state:             defaultOpenAIAdaptiveAccountState(1, cfg),
			effectiveCapacity: 10,
		},
		{
			account:           &Account{ID: 2, RateMultiplier: &highCost},
			loadInfo:          &AccountLoadInfo{},
			state:             defaultOpenAIAdaptiveAccountState(2, cfg),
			effectiveCapacity: 10,
		},
	}

	applyOpenAIAdaptiveScores(candidates, cfg)

	require.Greater(t, candidates[0].costScore, candidates[1].costScore)
	require.Greater(t, candidates[0].score, candidates[1].score)
}

func TestOpenAIAdaptiveFailureHealthSampleSkipsUserInputErrors(t *testing.T) {
	require.False(t, openAIAdaptiveFailureHealthSample(errors.New("invalid_request_error: missing required parameter")))
	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"type":"invalid_request_error","message":"bad input"}}`),
	}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
	}))
	require.Empty(t, openAIAdaptiveFailureCooldownReason(errors.New("invalid_request_error: missing required parameter")))
	require.Equal(t, "upstream_429", openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
	require.Equal(t, "upstream_502", openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.Equal(t, "upstream_503", openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}))
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(errors.New("upstream websocket is busy, please retry later")))
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(errors.New("timeout waiting for account concurrency slot")))
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"message":"Concurrency limit exceeded for account, please retry later"}}`),
	}))
}

func TestOpenAIAdaptiveFailureHealthSampleOverride(t *testing.T) {
	falseValue := false
	trueValue := true

	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		HealthSample: &falseValue,
	}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		HealthSample: &trueValue,
	}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}))
}

func TestOpenAIAdaptiveCapabilityMismatchCanFailOverWithoutHealthSample(t *testing.T) {
	healthSample := false
	err := &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"code":"unsupported_stream","message":"stream unsupported"}}`),
		FailureKind:  UpstreamFailureKindCapabilityMismatch,
		HealthSample: &healthSample,
	}

	require.True(t, IsUpstreamCapabilityMismatch(err))
	require.False(t, shouldIgnoreOpenAIAdaptiveFailoverError(err))
	require.False(t, openAIAdaptiveFailureHealthSample(err))
}

func TestOpenAIAdaptiveFailureSkipsRequestPolicyRejections(t *testing.T) {
	for _, message := range []string{
		"upstream response failed: cyber_policy",
		"Request blocked by content policy",
		"Your request was rejected by the safety system",
		"moderation_blocked: request rejected",
	} {
		require.False(t, openAIAdaptiveFailureHealthSample(errors.New(message)), message)
	}
	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"code":"safety_error","message":"request rejected by policy"}}`),
	}))
	require.True(t, openAIAdaptiveFailureHealthSample(errors.New("upstream response failed: server is overloaded")))
}

func TestOpenAIAdaptiveFailureCooldownDistinguishesUserAndAccountConcurrency(t *testing.T) {
	userLimitErr := NewOpenAIWSClientCloseError(
		coderws.StatusTryAgainLater,
		"too many concurrent requests, please retry later",
		nil,
	)
	accountLimitErr := NewOpenAIWSClientCloseError(
		coderws.StatusTryAgainLater,
		"account is busy, please retry later",
		nil,
	)

	require.Empty(t, openAIAdaptiveFailureCooldownReason(userLimitErr))
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(accountLimitErr))
}

func TestOpenAIAdaptiveSchedulerCooldownAppliedImmediately(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds = 30
	cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds = 60
	store := newOpenAIAdaptiveSchedulerStateStore()
	now := time.Now()

	store.applyCooldown(1001, cfg, "upstream_429", now)

	state := store.snapshot(1001, cfg)
	require.True(t, state.CooldownUntil.After(now))
	require.Equal(t, "active", openAIAdaptiveCooldownStatus(state, now))
	require.Equal(t, "expired", openAIAdaptiveCooldownStatus(state, state.CooldownUntil.Add(time.Nanosecond)))
}

func TestOpenAIAdaptiveSchedulerConcurrencyCooldownClearsStickySessions(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds = 30
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{
		settings:  cfg,
		complete:  true,
		expiresAt: time.Now().Add(time.Hour).UnixNano(),
	})

	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"openai:sticky_a": 1001,
		"openai:sticky_b": 1001,
		"openai:sticky_c": 1002,
	}}
	scheduler := &adaptiveOpenAIAccountScheduler{
		service:  &OpenAIGatewayService{cache: cache},
		baseline: &defaultOpenAIAccountScheduler{},
		state:    newOpenAIAdaptiveSchedulerStateStore(),
	}

	scheduler.ReportScheduleResultWithContext(context.Background(), OpenAIAccountScheduleReport{
		AccountID:      1001,
		Success:        false,
		HealthSample:   true,
		Cooldown:       true,
		CooldownReason: "concurrency_limit",
		TerminalReason: "account_health_failure",
		Err:            errors.New("upstream response failed: Concurrency limit exceeded for account, please retry later"),
	})

	require.NotContains(t, cache.sessionBindings, "openai:sticky_a")
	require.NotContains(t, cache.sessionBindings, "openai:sticky_b")
	require.Equal(t, int64(1002), cache.sessionBindings["openai:sticky_c"])
	require.Equal(t, 1, cache.deletedSessions["openai:sticky_a"])
	require.Equal(t, 1, cache.deletedSessions["openai:sticky_b"])
	require.Equal(t, "active", openAIAdaptiveCooldownStatus(scheduler.state.snapshot(1001, cfg), time.Now()))

	scheduler.ReportScheduleResultWithContext(context.Background(), OpenAIAccountScheduleReport{
		AccountID:      1001,
		Success:        false,
		HealthSample:   true,
		Cooldown:       true,
		CooldownReason: "concurrency_limit",
		TerminalReason: "account_health_failure",
		Err:            errors.New("upstream response failed: Concurrency limit exceeded for account, please retry later"),
	})
	require.Equal(t, 1, cache.accountCleanupCall[1001])
}

func TestOpenAIAdaptiveSchedulerActiveCooldownDoesNotExcludeCandidates(t *testing.T) {
	ctx := context.Background()
	groupID := int64(11001)
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerTopK = 1
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 0
	account := Account{
		ID:          22001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		GroupIDs:    []int64{groupID},
	}
	scheduler := &adaptiveOpenAIAccountScheduler{
		service: &OpenAIGatewayService{
			accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
			concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		},
		baseline: &defaultOpenAIAccountScheduler{},
		state:    newOpenAIAdaptiveSchedulerStateStore(),
	}
	scheduler.baseline.service = scheduler.service
	scheduler.state.applyCooldown(account.ID, cfg, "upstream_429", time.Now())

	order, candidateCount, _, err := scheduler.buildAdaptiveSelectionOrder(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, cfg)

	require.NoError(t, err)
	require.Equal(t, 1, candidateCount)
	require.Len(t, order, 1)
	require.Equal(t, account.ID, order[0].account.ID)
	require.Equal(t, "active", openAIAdaptiveCooldownStatus(order[0].state, time.Now()))
}

func TestOpenAIAdaptiveSchedulerActiveCooldownDoesNotBreakStickyHit(t *testing.T) {
	ctx := context.Background()
	groupID := int64(11002)
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	account := Account{
		ID:          22002,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		GroupIDs:    []int64{groupID},
	}
	cache := &schedulerTestGatewayCache{
		sessionBindings: map[string]int64{"openai:sticky_cooldown": account.ID},
	}
	scheduler := &adaptiveOpenAIAccountScheduler{
		service: &OpenAIGatewayService{
			accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
			cache:       cache,
			concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
				acquireResults: map[int64]bool{account.ID: true},
			}),
		},
		baseline: &defaultOpenAIAccountScheduler{},
		state:    newOpenAIAdaptiveSchedulerStateStore(),
	}
	scheduler.baseline.service = scheduler.service
	scheduler.state.applyCooldown(account.ID, cfg, "upstream_503", time.Now())

	selection, escapedSticky, err := scheduler.selectByAdaptiveSticky(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		SessionHash:       "sticky_cooldown",
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, cfg)

	require.NoError(t, err)
	require.False(t, escapedSticky)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, "active", openAIAdaptiveCooldownStatus(scheduler.state.snapshot(account.ID, cfg), time.Now()))
	require.Empty(t, cache.deletedSessions)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAdaptiveSchedulerAccountTypePriorityOrdersSelectionGroups(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerTopK = 3
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 0
	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst
	candidates := []openAIAdaptiveCandidateScore{
		{
			account:           &Account{ID: 1, Type: AccountTypeAPIKey},
			loadInfo:          &AccountLoadInfo{},
			effectiveCapacity: 10,
			score:             0.99,
		},
		{
			account:           &Account{ID: 2, Type: AccountTypeOAuth},
			loadInfo:          &AccountLoadInfo{},
			effectiveCapacity: 10,
			score:             0.10,
		},
		{
			account:           &Account{ID: 3, Type: AccountTypeSetupToken},
			loadInfo:          &AccountLoadInfo{},
			effectiveCapacity: 10,
			score:             0.20,
		},
	}

	order := buildOpenAIAdaptiveSelectionOrder(candidates, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5"}, cfg)

	require.Len(t, order, 3)
	require.True(t, order[0].account.IsOAuth())
	require.True(t, order[1].account.IsOAuth())
	require.Equal(t, AccountTypeAPIKey, order[2].account.Type)

	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = openAIAdaptiveSchedulerAccountTypePriorityAPIKeyFirst
	order = buildOpenAIAdaptiveSelectionOrder(candidates, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5"}, cfg)
	require.Equal(t, AccountTypeAPIKey, order[0].account.Type)

	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = openAIAdaptiveSchedulerAccountTypePriorityMixed
	require.True(t, isOpenAIAdaptiveCandidateBetter(candidates[0], candidates[1], cfg))
}

func TestOpenAIAdaptiveSchedulerExplorationDoesNotDuplicateTopKCandidates(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerTopK = 3
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 1
	cfg.OpenAIAdaptiveSchedulerThompsonEnabled = false
	candidates := []openAIAdaptiveCandidateScore{
		{account: &Account{ID: 1}, score: 0.9, explorationScore: 0.1},
		{account: &Account{ID: 2}, score: 0.8, explorationScore: 0.3},
		{account: &Account{ID: 3}, score: 0.7, explorationScore: 0.2},
	}

	order := buildOpenAIAdaptiveSelectionOrder(candidates, OpenAIAccountScheduleRequest{SessionHash: "stable-session"}, cfg)

	require.Len(t, order, len(candidates))
	seen := make(map[int64]struct{}, len(order))
	for _, candidate := range order {
		require.NotNil(t, candidate.account)
		_, duplicated := seen[candidate.account.ID]
		require.False(t, duplicated, "exploration order contains duplicate account %d", candidate.account.ID)
		seen[candidate.account.ID] = struct{}{}
	}
}

func TestOpenAIAdaptiveSchedulerThompsonExplorationUsesDeterministicBetaSamples(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerTopK = 2
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 1
	cfg.OpenAIAdaptiveSchedulerThompsonEnabled = true
	candidates := []openAIAdaptiveCandidateScore{
		{
			account:  &Account{ID: 1},
			loadInfo: &AccountLoadInfo{},
			score:    0.5,
			state: openAIAdaptiveAccountState{
				ThompsonAlpha: 1,
				ThompsonBeta:  1,
			},
		},
		{
			account:  &Account{ID: 2},
			loadInfo: &AccountLoadInfo{},
			score:    0.5,
			state: openAIAdaptiveAccountState{
				ThompsonAlpha: 100,
				ThompsonBeta:  100,
			},
		},
	}
	req := OpenAIAccountScheduleRequest{SessionHash: "thompson-session", RequestedModel: "gpt-5"}

	first := buildOpenAIAdaptiveSelectionOrder(candidates, req, cfg)
	second := buildOpenAIAdaptiveSelectionOrder(candidates, req, cfg)

	require.Len(t, first, 2)
	require.Len(t, second, 2)
	for i := range first {
		require.Equal(t, first[i].account.ID, second[i].account.ID)
		require.Equal(t, first[i].explorationScore, second[i].explorationScore)
		require.GreaterOrEqual(t, first[i].explorationScore, 0.0)
		require.LessOrEqual(t, first[i].explorationScore, 1.0)
	}
	require.NotEqual(t, 0.5, first[0].explorationScore, "Thompson exploration must sample instead of using the posterior mean")
}

func TestEffectiveOpenAIAdaptiveCapacityCapsConfiguredConcurrency(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	state.EstimatedCapacity = 10000

	require.Equal(t, 300, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 300}, state, cfg))
	require.Equal(t, 10000, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 0}, state, cfg))
}

func TestEffectiveOpenAIAdaptiveCapacityUsesInitialFractionAndBurstProbe(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction = 0.1
	cfg.OpenAIAdaptiveSchedulerBurstProbeRatio = 0.2
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	account := &Account{ID: 1, Concurrency: 30000}

	require.Equal(t, 3000, effectiveOpenAIAdaptiveCapacity(account, state, cfg))

	effective := effectiveOpenAIAdaptiveCapacityWithLoad(account, state, cfg, &AccountLoadInfo{
		AccountID:          1,
		CurrentConcurrency: 2500,
		WaitingCount:       1,
	})
	require.Equal(t, 3600, effective)
}

func TestOpenAIAdaptiveReportInitializesCapacityFromConfiguredConcurrency(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction = 0.05
	cfg.OpenAIAdaptiveSchedulerMinCapacity = 2
	store := newOpenAIAdaptiveSchedulerStateStore()
	account := &Account{ID: 1, Concurrency: 1000}

	store.reportWithAccount(account, account.ID, cfg, true, nil, 0)

	state := store.snapshot(account.ID, cfg)
	require.Equal(t, 50, state.EstimatedCapacity)
	require.Equal(t, 1, int(state.TotalSamples))
}

func TestOpenAIAdaptiveReportInitialCapacityFallsBackToMinimum(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction = 0.05
	cfg.OpenAIAdaptiveSchedulerMinCapacity = 2
	store := newOpenAIAdaptiveSchedulerStateStore()
	account := &Account{ID: 1, Concurrency: 10}

	store.reportWithAccount(account, account.ID, cfg, true, nil, 0)

	state := store.snapshot(account.ID, cfg)
	require.Equal(t, 2, state.EstimatedCapacity)
}

func TestEffectiveOpenAIAdaptiveCapacityUsesHalfOpenProbeAfterCooldown(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity = 5
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	state.EstimatedCapacity = 100
	state.ConsecutiveCapacityFailure = 3
	state.CooldownUntil = time.Now().Add(-time.Second)

	require.Equal(t, 5, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 300}, state, cfg))
}

func TestEffectiveOpenAIAdaptiveCapacityWaitsForHalfOpenFailureThreshold(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold = 2
	cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity = 5
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	state.EstimatedCapacity = 100
	state.ConsecutiveCapacityFailure = 1

	require.Equal(t, 100, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 300}, state, cfg))

	state.ConsecutiveCapacityFailure = 2
	require.Equal(t, 5, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 300}, state, cfg))
}

func TestOpenAIAdaptiveSchedulerAIMDDecreasesCapacityOnFailures(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerMinCapacity = 1
	cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold = 2
	cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink = 10
	cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold = 0.2
	cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft = 0.5
	cfg.OpenAIAdaptiveSchedulerShrinkFactorHard = 0.5
	cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds = 1
	store := newOpenAIAdaptiveSchedulerStateStore()
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	state.EstimatedCapacity = 100
	store.states[1] = &state

	for i := 0; i < 8; i++ {
		store.report(1, cfg, true, nil, 0)
	}

	store.report(1, cfg, false, nil, 0)
	require.Equal(t, 100, store.snapshot(1, cfg).EstimatedCapacity)

	store.report(1, cfg, false, nil, 0)
	state = store.snapshot(1, cfg)
	require.Equal(t, 50, state.EstimatedCapacity)
	require.True(t, state.CooldownUntil.After(state.LastCapacityFailureAt))
}

func TestOpenAIAdaptiveSchedulerDoesNotShrinkOnSparseFailures(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerMinCapacity = 1
	cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold = 3
	cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink = 10
	cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold = 0.2
	cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft = 0.5
	store := newOpenAIAdaptiveSchedulerStateStore()
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	state.EstimatedCapacity = 100
	store.states[1] = &state

	for i := 0; i < 3; i++ {
		store.report(1, cfg, false, nil, 0)
	}

	state = store.snapshot(1, cfg)
	require.Equal(t, 100, state.EstimatedCapacity)
	require.True(t, state.CooldownUntil.IsZero())
}

func TestOpenAIAdaptiveDiagnosticSamplingRespectsSwitchAndRate(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	req := OpenAIAccountScheduleRequest{RequestedModel: "gpt-5"}

	require.False(t, shouldLogOpenAIAdaptiveDiagnostic(t.Context(), req, cfg))

	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 0
	require.False(t, shouldLogOpenAIAdaptiveDiagnostic(t.Context(), req, cfg))

	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 1
	require.True(t, shouldLogOpenAIAdaptiveDiagnostic(t.Context(), req, cfg))
}
