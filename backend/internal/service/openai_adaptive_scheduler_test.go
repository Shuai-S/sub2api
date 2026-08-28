package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type openAIAdaptiveRuntimeBlockingConcurrencyCache struct {
	ConcurrencyCache
	onAcquire func()
}

func (c *openAIAdaptiveRuntimeBlockingConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	if c.onAcquire != nil {
		c.onAcquire()
	}
	return false, nil
}

func (c *openAIAdaptiveRuntimeBlockingConcurrencyCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	loads := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		loads[account.ID] = &AccountLoadInfo{AccountID: account.ID}
	}
	return loads, nil
}

func newOpenAIAdaptiveTestScheduler(service *OpenAIGatewayService) *adaptiveOpenAIAccountScheduler {
	if service == nil {
		service = &OpenAIGatewayService{}
	}
	core := newAdaptiveStateStore()
	scheduler := &adaptiveOpenAIAccountScheduler{
		service:  service,
		baseline: &defaultOpenAIAccountScheduler{service: service},
		core:     core,
	}
	service.openaiAdaptiveCore = core
	return scheduler
}

func TestOpenAIAdaptiveSchedulerCostScoreUsesDynamicCandidateLayer(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	lowCost := 0.15
	highCost := 0.25
	now := time.Now()
	candidates := []openAIAdaptiveCandidateScore{
		{
			account:   &Account{ID: 1, Type: AccountTypeAPIKey, RateMultiplier: &lowCost, Concurrency: 10},
			loadInfo:  &AccountLoadInfo{},
			coreState: *newAdaptiveAccountState(1, 10, now),
		},
		{
			account:   &Account{ID: 2, Type: AccountTypeAPIKey, RateMultiplier: &highCost, Concurrency: 10},
			loadInfo:  &AccountLoadInfo{},
			coreState: *newAdaptiveAccountState(2, 10, now),
		},
	}

	applyOpenAIAdaptiveScores(candidates, cfg)

	require.InDelta(t, 1.0, candidates[0].costScore, 1e-9)
	require.InDelta(t, 0.6, candidates[1].costScore, 1e-9)
	require.Greater(t, candidates[0].score, candidates[1].score)
}

func TestOpenAIAdaptiveSelectionKeepsOAuthAsOnlyHardPriority(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	cfg.OpenAIAdaptiveSchedulerTopK = 1
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 0
	candidates := []openAIAdaptiveCandidateScore{
		{account: &Account{ID: 1, Type: AccountTypeAPIKey, Priority: 1}, score: 0.99},
		{account: &Account{ID: 2, Type: AccountTypeOAuth, Priority: 99}, score: 0.10},
		{account: &Account{ID: 3, Type: AccountTypeSetupToken, Priority: 50}, score: 0.20},
	}

	order := buildOpenAIAdaptiveSelectionOrder(candidates, OpenAIAccountScheduleRequest{RequestedModel: "gpt-5"}, cfg)

	require.Len(t, order, 3)
	require.Equal(t, int64(3), order[0].account.ID)
	require.Equal(t, int64(2), order[1].account.ID)
	require.Equal(t, int64(1), order[2].account.ID)
}

func TestOpenAIAdaptiveSchedulerExplorationDoesNotDuplicateCandidates(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	cfg.OpenAIAdaptiveSchedulerTopK = 3
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 1
	candidates := []openAIAdaptiveCandidateScore{
		{account: &Account{ID: 1, Type: AccountTypeAPIKey}, score: 0.9},
		{account: &Account{ID: 2, Type: AccountTypeAPIKey}, score: 0.8},
		{account: &Account{ID: 3, Type: AccountTypeAPIKey}, score: 0.7},
	}

	order := buildOpenAIAdaptiveSelectionOrder(candidates, OpenAIAccountScheduleRequest{SessionHash: "stable-session"}, cfg)

	require.Len(t, order, len(candidates))
	seen := make(map[int64]struct{}, len(order))
	for _, candidate := range order {
		_, duplicated := seen[candidate.account.ID]
		require.False(t, duplicated, "exploration order contains duplicate account %d", candidate.account.ID)
		seen[candidate.account.ID] = struct{}{}
	}
}

func TestOpenAIAdaptiveSchedulerExploresOnlyNewSessions(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	cfg.OpenAIAdaptiveSchedulerTopK = 1
	cfg.OpenAIAdaptiveSchedulerExplorationRate = 1
	candidates := []openAIAdaptiveCandidateScore{
		{
			account: &Account{ID: 1, Type: AccountTypeAPIKey},
			score:   1,
			coreState: adaptiveAccountState{
				HealthObservations: make([]adaptiveHealthObservation, cfg.OpenAIAdaptiveSchedulerLearningMinHealthSamples),
			},
		},
		{account: &Account{ID: 2, Type: AccountTypeAPIKey}, score: 0.1},
	}

	tests := []struct {
		name      string
		request   OpenAIAccountScheduleRequest
		wantFirst int64
	}{
		{name: "new session explores", request: OpenAIAccountScheduleRequest{}, wantFirst: 2},
		{name: "sticky session does not explore", request: OpenAIAccountScheduleRequest{StickyAccountID: 99}, wantFirst: 1},
		{name: "previous response does not explore", request: OpenAIAccountScheduleRequest{PreviousResponseID: "resp_123"}, wantFirst: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := buildOpenAIAdaptiveSelectionOrder(candidates, tt.request, cfg)
			require.Equal(t, tt.wantFirst, order[0].account.ID)
		})
	}
}

func TestOpenAIAdaptiveFailureHealthSampleSkipsUserInputErrors(t *testing.T) {
	require.False(t, openAIAdaptiveFailureHealthSample(errors.New("invalid_request_error: missing required parameter")))
	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"type":"invalid_request_error","message":"bad input"}}`),
	}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
	require.Empty(t, openAIAdaptiveFailureCooldownReason(errors.New("invalid_request_error: missing required parameter")))
	require.Equal(t, "upstream_429", openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
	require.Equal(t, "upstream_502", openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.Empty(t, openAIAdaptiveFailureCooldownReason(&UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}))
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(errors.New("upstream websocket is busy, please retry later")))
}

func TestOpenAIAdaptiveFailureHealthSampleOverride(t *testing.T) {
	falseValue := false
	trueValue := true

	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusBadGateway, HealthSample: &falseValue}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusBadRequest, HealthSample: &trueValue}))
	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, HealthSample: &trueValue}))
	require.False(t, openAIAdaptiveFailureHealthSample(&openAIUpstreamResponseFailedError{StatusCode: http.StatusServiceUnavailable}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
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

func TestOpenAIModelNotFoundCanFailOverWithoutHealthSample(t *testing.T) {
	body := []byte(`{"error":{"type":"model_not_found","message":"Model gpt-5.6-luna not found"}}`)
	service := &OpenAIGatewayService{}

	require.True(t, service.shouldFailoverOpenAIUpstreamResponse(http.StatusNotFound, "Model gpt-5.6-luna not found", body))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeAPIKey}, http.StatusNotFound, body))

	err := newOpenAIUpstreamFailoverError(http.StatusNotFound, nil, body, "Model gpt-5.6-luna not found", true)
	require.True(t, IsUpstreamCapabilityMismatch(err))
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
	require.True(t, openAIAdaptiveFailureHealthSample(errors.New("upstream response failed: server is overloaded")))
}

func TestOpenAIAdaptiveFailureCooldownDistinguishesUserAndAccountConcurrency(t *testing.T) {
	userLimitErr := NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "too many concurrent requests, please retry later", nil)
	accountLimitErr := NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is busy, please retry later", nil)

	require.Empty(t, openAIAdaptiveFailureCooldownReason(userLimitErr))
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(accountLimitErr))
}

func TestOpenAIAdaptiveUnscopedBadGatewayBecomesAccountHealthFailure(t *testing.T) {
	err := &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(`{"error":{"type":"server_error"}}`)}
	reason := classifyOpenAIAdaptiveTerminalReason(err, true)
	require.Equal(t, "upstream_5xx", reason)
	observation, _ := classifyAdaptiveTerminalReason(false, reason)
	require.Equal(t, adaptiveObservationAccountFailure, observation)
}

func TestOpenAIAdaptiveExplicitGlobalServerErrorsStayProviderScoped(t *testing.T) {
	tests := []struct {
		name string
		err  *UpstreamFailoverError
	}{
		{name: "provider scope", err: &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, Scope: GatewayFailureScopeProvider}},
		{name: "request scope", err: &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, Scope: GatewayFailureScopeRequest}},
		{name: "request transient", err: &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, Scope: GatewayFailureScopeAccount, RequestScopedTransient: true}},
		{name: "provider overload status", err: &UpstreamFailoverError{StatusCode: 529, Scope: GatewayFailureScopeAccount}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, openAIAdaptiveFailureHealthSample(tt.err))
			reason := classifyOpenAIAdaptiveTerminalReason(tt.err, true)
			require.Equal(t, "provider_overloaded", reason)
			observation, _ := classifyAdaptiveTerminalReason(false, reason)
			require.Equal(t, adaptiveObservationProviderOverload, observation)
		})
	}
}

func TestOpenAIAdaptiveAccountTransportFailureRemainsAccountScoped(t *testing.T) {
	err := &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		Scope:        GatewayFailureScopeAccount,
		FailureKind:  UpstreamFailureKindTransport,
		HealthSample: boolPtr(true),
	}
	require.Equal(t, "transport_error", classifyOpenAIAdaptiveTerminalReason(err, true))
	observation, _ := classifyAdaptiveTerminalReason(false, "transport_error")
	require.Equal(t, adaptiveObservationAccountFailure, observation)
}

func TestOpenAIAdaptiveRepeatedServiceUnavailableSwitchesWithoutAccountCircuit(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})
	account := Account{ID: 1000, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"openai:broken_account":  account.ID,
		"openai:healthy_account": 1001,
	}}
	service := &OpenAIGatewayService{cache: cache, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	err := &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte(`{"error":{"code":"upstream_unavailable","message":"Service temporarily unavailable, please retry later.","type":"service_unavailable"}}`)}
	healthSample := openAIAdaptiveFailureHealthSample(err)
	reason := classifyOpenAIAdaptiveTerminalReason(err, healthSample)

	require.True(t, service.shouldFailoverOpenAIUpstreamResponse(err.StatusCode, "", err.ResponseBody))
	require.True(t, err.ShouldRetryNextAccount())
	failoverOutcome, suppressedReason := openAIAdaptiveFailoverDecision(err, OpenAIAdaptiveFailureReportOptions{})
	require.Equal(t, OpenAIAdaptiveFailoverOutcomeEligible, failoverOutcome)
	require.Empty(t, suppressedReason)
	require.False(t, healthSample)
	require.Empty(t, openAIAdaptiveFailureCooldownReason(err))
	require.Equal(t, "provider_overloaded", reason)
	requestIDs := []string{"server-error-1", "server-error-2", "server-error-3"}
	for _, requestID := range requestIDs {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, requestID)
		scheduler.ReportScheduleResultWithContext(ctx, OpenAIAccountScheduleReport{
			AccountID:      account.ID,
			HealthSample:   healthSample,
			TerminalReason: reason,
			Err:            err,
		})
		state := scheduler.core.snapshot(account.ID, account.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg))
		require.Zero(t, state.ConsecutiveFailures)
		require.True(t, state.CircuitOpenUntil.IsZero())
	}

	state := scheduler.core.snapshot(account.ID, account.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg))
	require.Empty(t, state.HealthObservations)
	require.True(t, state.CircuitOpenUntil.IsZero())
	require.Equal(t, account.ID, cache.sessionBindings["openai:broken_account"])
	require.Equal(t, int64(1001), cache.sessionBindings["openai:healthy_account"])
	require.Zero(t, cache.accountCleanupCall[account.ID])
}

func TestOpenAIAdaptiveExistingFailureReasonsMapToCoreObservations(t *testing.T) {
	tests := []struct {
		reason             string
		want               adaptiveObservationType
		wantAuthentication bool
	}{
		{reason: "success", want: adaptiveObservationHealthSuccess},
		{reason: "account_auth", want: adaptiveObservationAccountFailure, wantAuthentication: true},
		{reason: "account_health_failure", want: adaptiveObservationAccountFailure},
		{reason: "concurrency_limit", want: adaptiveObservationCapacityLimit},
		{reason: "insufficient_balance", want: adaptiveObservationQuotaLimit},
		{reason: "quota_rate_limit", want: adaptiveObservationQuotaLimit},
		{reason: "provider_overloaded", want: adaptiveObservationProviderOverload},
		{reason: "request_policy", want: adaptiveObservationIgnored},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			observation, authentication := classifyAdaptiveTerminalReason(tt.reason == "success", tt.reason)
			require.Equal(t, tt.want, observation)
			require.Equal(t, tt.wantAuthentication, authentication)
		})
	}
}

func TestOpenAIAdaptiveConcurrencyLimitClearsStickyForFutureRequests(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})
	account := Account{ID: 1001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 100}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"openai:sticky_a": account.ID,
		"openai:sticky_b": account.ID,
		"openai:sticky_c": 1002,
	}}
	service := &OpenAIGatewayService{cache: cache, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "capacity-limit")

	scheduler.ReportScheduleResultWithContext(ctx, OpenAIAccountScheduleReport{
		AccountID:      account.ID,
		HealthSample:   true,
		TerminalReason: "concurrency_limit",
	})

	require.NotContains(t, cache.sessionBindings, "openai:sticky_a")
	require.NotContains(t, cache.sessionBindings, "openai:sticky_b")
	require.Equal(t, int64(1002), cache.sessionBindings["openai:sticky_c"])
	require.Equal(t, 1, cache.accountCleanupCall[account.ID])
	require.Equal(t, 90, scheduler.core.snapshot(account.ID, account.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg)).EffectiveCapacity)
}

func TestOpenAIAdaptiveSchedulerOpenCircuitExcludesCandidate(t *testing.T) {
	ctx := context.Background()
	groupID := int64(11001)
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	account := Account{ID: 22001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}}
	service := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	scheduler.core.mu.Lock()
	scheduler.core.ensureLocked(account.ID, account.Concurrency, time.Now()).CircuitOpenUntil = time.Now().Add(time.Minute)
	scheduler.core.mu.Unlock()

	order, candidateCount, _, err := scheduler.buildAdaptiveSelectionOrder(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, cfg)

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "runtime_unavailable=1")
	require.Zero(t, candidateCount)
	require.Empty(t, order)
}

func TestOpenAIAdaptiveSchedulerStickyPreemptsDueProbe(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})

	groupID := int64(11008)
	sticky := Account{ID: 22020, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	halfOpen := Account{ID: 22021, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:sticky": sticky.ID}}
	acquiredIDs := make([]int64, 0, 1)
	service := &OpenAIGatewayService{
		cache:              cache,
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{sticky, halfOpen}}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	scheduler.core.mu.Lock()
	halfOpenState := scheduler.core.ensureLocked(halfOpen.ID, halfOpen.Concurrency, time.Now())
	halfOpenState.CircuitOpenUntil = time.Now().Add(-time.Minute)
	halfOpenState.CircuitOpenCount = 3
	halfOpenState.ConsecutiveFailures = 20
	scheduler.core.mu.Unlock()
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "half-open-probe")
	guardSelection, _, guardErr := scheduler.selectDueHealthProbe(ctx, OpenAIAccountScheduleRequest{
		GroupID:                 &groupID,
		Platform:                PlatformOpenAI,
		GuardianParentAccountID: sticky.ID,
		RequestedModel:          "gpt-5.1",
		RequiredTransport:       OpenAIUpstreamTransportAny,
	}, cfg)
	require.NoError(t, guardErr)
	require.Nil(t, guardSelection)

	selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		SessionHash:       "sticky",
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	})

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, sticky.ID, selection.Account.ID)
	require.Equal(t, []int64{sticky.ID}, acquiredIDs)
	require.False(t, scheduler.core.snapshot(halfOpen.ID, halfOpen.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg)).HealthProbeInFlight)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.True(t, decision.StickySessionHit)
	require.Equal(t, sticky.ID, cache.sessionBindings["openai:sticky"])

	selection.ReleaseFunc()
}

func TestOpenAIAdaptiveSchedulerFailedProbeFallsBackToHealthyAccount(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerEnabled = true
	cfg.OpenAIAdaptiveSchedulerMode = openAIAdaptiveSchedulerModeEnforce
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{settings: cfg, complete: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})

	groupID := int64(11009)
	healthy := Account{ID: 22023, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	firstProbe := Account{ID: 22024, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	secondProbe := Account{ID: 22025, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{groupID}}
	acquiredIDs := make([]int64, 0, 2)
	service := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{healthy, firstProbe, secondProbe}}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	now := time.Now()
	scheduler.core.mu.Lock()
	firstState := scheduler.core.ensureLocked(firstProbe.ID, firstProbe.Concurrency, now)
	firstState.CircuitOpenUntil = now.Add(-2 * time.Minute)
	firstState.CircuitOpenCount = 2
	secondState := scheduler.core.ensureLocked(secondProbe.ID, secondProbe.Concurrency, now)
	secondState.CircuitOpenUntil = now.Add(-time.Minute)
	secondState.CircuitOpenCount = 2
	scheduler.core.mu.Unlock()

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "single-half-open-probe")
	req := OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}
	selection, _, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, firstProbe.ID, selection.Account.ID)
	selection.ReleaseFunc()
	scheduler.ReportScheduleResultWithContext(ctx, OpenAIAccountScheduleReport{
		AccountID:      firstProbe.ID,
		Success:        false,
		HealthSample:   true,
		TerminalReason: "account_auth",
	})

	req.ExcludedIDs = map[int64]struct{}{firstProbe.ID: {}}
	fallback, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, fallback)
	require.Equal(t, healthy.ID, fallback.Account.ID)
	require.False(t, decision.StickySessionHit)
	require.Equal(t, []int64{firstProbe.ID, healthy.ID}, acquiredIDs)
	require.False(t, scheduler.core.snapshot(secondProbe.ID, secondProbe.Concurrency, time.Now(), openAIAdaptiveCoreSettings(cfg)).HealthProbeInFlight)
	fallback.ReleaseFunc()
}

func TestOpenAIAccountSuccessfulTestClosesAdaptiveCircuit(t *testing.T) {
	account := &Account{ID: 22022, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 10}
	core := newAdaptiveStateStore()
	service := &OpenAIGatewayService{openaiAdaptiveCore: core}
	service.BlockAccountScheduling(account, time.Now().Add(time.Minute), "account_health_failure")
	core.mu.Lock()
	state := core.ensureLocked(account.ID, account.Concurrency, time.Now())
	state.CircuitOpenUntil = time.Now().Add(10 * time.Minute)
	state.CircuitOpenCount = 5
	state.ConsecutiveFailures = 12
	core.mu.Unlock()

	service.RecoverAccountSchedulingHealth(context.Background(), account.ID)

	recovered := core.snapshot(account.ID, account.Concurrency, time.Now(), defaultAdaptiveCoreSettings())
	require.True(t, recovered.CircuitOpenUntil.IsZero())
	require.Zero(t, recovered.CircuitOpenCount)
	require.Zero(t, recovered.ConsecutiveFailures)
	require.Equal(t, "successful_account_test", recovered.LastReasonCode)
	require.False(t, service.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAIAdaptiveSchedulerExcludesAccountLevelRateLimit(t *testing.T) {
	ctx := context.Background()
	groupID := int64(11002)
	resetAt := time.Now().Add(time.Hour)
	account := Account{
		ID:               22002,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      100,
		GroupIDs:         []int64{groupID},
		RateLimitResetAt: &resetAt,
	}
	service := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)

	order, candidateCount, _, err := scheduler.buildAdaptiveSelectionOrder(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, DefaultOpenAIAdaptiveSchedulerSettings())

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "not_schedulable=1")
	require.Zero(t, candidateCount)
	require.Empty(t, order)

	recoveredAt := time.Now().Add(-time.Second)
	account.RateLimitResetAt = &recoveredAt
	recoveredService := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	recoveredScheduler := newOpenAIAdaptiveTestScheduler(recoveredService)
	recoveredOrder, recoveredCandidateCount, _, recoveredErr := recoveredScheduler.buildAdaptiveSelectionOrder(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, DefaultOpenAIAdaptiveSchedulerSettings())

	require.NoError(t, recoveredErr)
	require.Equal(t, 1, recoveredCandidateCount)
	require.Len(t, recoveredOrder, 1)
	require.Equal(t, account.ID, recoveredOrder[0].account.ID)
}

func TestOpenAIAdaptiveSchedulerNoAvailableErrorReportsEmptyPool(t *testing.T) {
	groupID := int64(11003)
	scheduler := newOpenAIAdaptiveTestScheduler(&OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{}})

	order, candidateCount, topK, err := scheduler.buildAdaptiveSelectionOrder(context.Background(), OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, DefaultOpenAIAdaptiveSchedulerSettings())

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.EqualError(t, err, "no available OpenAI accounts supporting model: gpt-5.1 (pool=0)")
	require.Empty(t, order)
	require.Zero(t, candidateCount)
	require.Zero(t, topK)
}

func TestOpenAIAdaptiveSchedulerNoAvailableErrorAggregatesFilterReasons(t *testing.T) {
	ctx := withOpenAIQuotaAutoPauseSettings(context.Background(), OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold7d: 0.9})
	groupID := int64(11004)
	quotaPaused := Account{
		ID: 22004, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Extra: map[string]any{
			"codex_7d_used_percent":  95.0,
			"codex_7d_reset_at":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		},
	}
	modelUnsupported := Account{
		ID: 22005, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-4o": "gpt-4o"}},
	}
	excluded := Account{ID: 22006, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	scheduler := newOpenAIAdaptiveTestScheduler(&OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{quotaPaused, modelUnsupported, excluded}}})

	order, candidateCount, topK, err := scheduler.buildAdaptiveSelectionOrder(ctx, OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.4-mini",
		RequiredTransport: OpenAIUpstreamTransportAny,
		ExcludedIDs:       map[int64]struct{}{excluded.ID: {}},
	}, DefaultOpenAIAdaptiveSchedulerSettings())

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "excluded=1")
	require.Contains(t, err.Error(), "model_not_supported=1")
	require.Contains(t, err.Error(), "quota_auto_pause_7d=1")
	require.Empty(t, order)
	require.Zero(t, candidateCount)
	require.Zero(t, topK)
}

func TestOpenAIAdaptiveSchedulerNoAvailableErrorReportsCapacityFull(t *testing.T) {
	groupID := int64(11005)
	account := Account{ID: 22007, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	service := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
			account.ID: {AccountID: account.ID, CurrentConcurrency: 1},
		}}),
	}
	scheduler := newOpenAIAdaptiveTestScheduler(service)

	order, candidateCount, topK, err := scheduler.buildAdaptiveSelectionOrder(context.Background(), OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, DefaultOpenAIAdaptiveSchedulerSettings())

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "capacity_full=1")
	require.Empty(t, order)
	require.Zero(t, candidateCount)
	require.Zero(t, topK)
}

func TestOpenAIAdaptiveSchedulerNoAvailableErrorReportsSelectionAttempts(t *testing.T) {
	groupID := int64(11006)
	account := Account{ID: 22008, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	service := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}}
	cache := &openAIAdaptiveRuntimeBlockingConcurrencyCache{}
	cache.onAcquire = func() {
		service.openaiAccountRuntimeBlockUntil.Store(account.ID, time.Now().Add(time.Hour))
	}
	service.concurrencyService = NewConcurrencyService(cache)
	scheduler := newOpenAIAdaptiveTestScheduler(service)

	selection, candidateCount, topK, _, _, err := scheduler.selectByAdaptiveLoadBalance(context.Background(), OpenAIAccountScheduleRequest{
		GroupID:           &groupID,
		Platform:          PlatformOpenAI,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: OpenAIUpstreamTransportAny,
	}, DefaultOpenAIAdaptiveSchedulerSettings())

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Contains(t, err.Error(), "slot_unavailable=1")
	require.Contains(t, err.Error(), "runtime_blocked=1")
	require.Nil(t, selection)
	require.Equal(t, 1, candidateCount)
	require.Equal(t, 1, topK)
}

func TestOpenAIAdaptiveInsufficientBalanceClassificationMapsToQuota(t *testing.T) {
	for _, err := range []error{
		&UpstreamFailoverError{StatusCode: http.StatusPaymentRequired, ResponseBody: []byte(`{"error":{"message":"payment required"}}`)},
		&UpstreamFailoverError{StatusCode: http.StatusForbidden, ResponseBody: []byte(`{"error":{"code":"insufficient_balance","message":"top up"}}`)},
		&UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(`{"error":{"message":"credit balance exhausted"}}`)},
	} {
		require.True(t, isOpenAIAdaptiveInsufficientBalanceError(err))
		require.False(t, openAIAdaptiveFailureHealthSample(err))
		reason := classifyOpenAIAdaptiveTerminalReason(err, false)
		require.Equal(t, "insufficient_balance", reason)
		observation, _ := classifyAdaptiveTerminalReason(false, reason)
		require.Equal(t, adaptiveObservationQuotaLimit, observation)
	}
}

func TestOpenAIAccountInternalFailoverErrorClassification(t *testing.T) {
	tests := []struct {
		name string
		err  *UpstreamFailoverError
		want bool
	}{
		{name: "credential", err: &UpstreamFailoverError{Stage: GatewayFailureStageAccountAuth}, want: true},
		{name: "payment", err: &UpstreamFailoverError{StatusCode: http.StatusPaymentRequired}, want: true},
		{name: "quota", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseBody: []byte(`{"error":{"code":"quota_exceeded"}}`)}, want: true},
		{name: "request", err: &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(`{"error":{"type":"invalid_request_error"}}`)}, want: false},
		{name: "server", err: &UpstreamFailoverError{StatusCode: http.StatusInternalServerError}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsOpenAIAccountInternalFailoverError(tt.err))
		})
	}
}

func TestOpenAIAdaptiveDiagnosticDecisionLogsMeasuredLatency(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 1
	scheduler := &adaptiveOpenAIAccountScheduler{}
	scheduler.logEnforceDiagnosticDecision(
		context.Background(),
		OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.1"},
		cfg,
		OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerAdaptive},
		nil,
		nil,
		"selected",
		nil,
		time.Now().Add(-20*time.Millisecond),
	)

	require.Regexp(t, `latency_ms=[1-9][0-9]*`, output.String())
}

func TestOpenAIAdaptiveShadowDecisionRequiresDiagnosticsAndAlwaysLogsDivergence(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	account := Account{ID: 7101, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10}
	service := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.1", RequiredTransport: OpenAIUpstreamTransportAny}

	scheduler.logShadowDecision(context.Background(), req, cfg, &AccountSelectionResult{Account: &account})
	require.Empty(t, output.String())

	scheduler.logShadowDecision(context.Background(), req, cfg, &AccountSelectionResult{Account: &Account{ID: 7102}})
	require.Empty(t, output.String())

	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 0
	scheduler.logShadowDecision(context.Background(), req, cfg, &AccountSelectionResult{Account: &Account{ID: 7102}})
	require.Contains(t, output.String(), "openai_adaptive_shadow_decision")
	require.Contains(t, output.String(), "diverged=true")
}

func TestOpenAIAdaptiveShadowDecisionDisabledSkipsPlanConstruction(t *testing.T) {
	scheduler := &adaptiveOpenAIAccountScheduler{}
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()

	require.NotPanics(t, func() {
		scheduler.logShadowDecision(context.Background(), OpenAIAccountScheduleRequest{}, cfg, nil)
	})
}

func TestOpenAIAdaptiveSelectionPlanLoaderLoadsOnce(t *testing.T) {
	calls := 0
	want := openAIAdaptiveSelectionPlan{candidateCount: 3, topK: 2}
	load := memoizeOpenAIAdaptiveSelectionPlanLoader(func() (openAIAdaptiveSelectionPlan, error) {
		calls++
		return want, nil
	})

	for range 3 {
		got, err := load()
		require.NoError(t, err)
		require.Equal(t, want.candidateCount, got.candidateCount)
		require.Equal(t, want.topK, got.topK)
	}
	require.Equal(t, 1, calls)
}

func TestOpenAIAdaptiveDiagnosticResultIncludesFailoverContext(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	account := Account{ID: 7001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 12}
	service := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}}
	scheduler := newOpenAIAdaptiveTestScheduler(service)
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 0
	err := &UpstreamFailoverError{
		StatusCode:               http.StatusServiceUnavailable,
		ResponseBody:             []byte(`{"response":{"error":{"code":"server_overloaded","type":"server_error"}}}`),
		RetryableOnSameAccount:   true,
		RequestScopedTransient:   true,
		SafeToFailoverAfterWrite: true,
		FirstOutputGuardFailure:  true,
		Stage:                    GatewayFailureStageInference,
		Scope:                    GatewayFailureScopeProvider,
		Reason:                   GatewayFailureReason("capacity_shed"),
		NextAccountAction:        NextAccountStop,
		FailureKind:              UpstreamFailureKindTransport,
	}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-diagnostic-failure")

	scheduler.logDiagnosticResult(ctx, cfg, OpenAIAccountScheduleReport{
		AccountID:                   account.ID,
		Stream:                      true,
		TerminalReason:              "provider_overloaded",
		FailoverOutcome:             OpenAIAdaptiveFailoverOutcomeSuppressed,
		FailoverSuppressedReason:    OpenAIAdaptiveFailoverSuppressedNextAccountStopped,
		SemanticOutputStarted:       true,
		ResponseAlreadyCommunicated: true,
		AccountSwitchCount:          2,
		MaxAccountSwitches:          3,
		SameAccountRetryCount:       1,
		SameAccountRetryLimit:       2,
		Err:                         err,
	})

	logText := output.String()
	require.Contains(t, logText, "openai_adaptive_scheduler_diagnostic_result")
	require.Contains(t, logText, "request_id=request-diagnostic-failure")
	require.Contains(t, logText, "account_type=apikey")
	require.Contains(t, logText, "platform=openai")
	require.Contains(t, logText, "account_switch_count=2")
	require.Contains(t, logText, "attempt_number=3")
	require.Contains(t, logText, "max_account_switches=3")
	require.Contains(t, logText, "failover_outcome=suppressed")
	require.Contains(t, logText, "failover_suppressed_reason=next_account_stopped")
	require.Contains(t, logText, "failure_class=upstream_failover")
	require.Contains(t, logText, "upstream_status=503")
	require.Contains(t, logText, "upstream_error_code=server_overloaded")
	require.Contains(t, logText, "upstream_error_type=server_error")
	require.Contains(t, logText, "failure_stage=inference")
	require.Contains(t, logText, "failure_scope=provider")
	require.Contains(t, logText, "failure_reason=capacity_shed")
	require.Contains(t, logText, "failure_kind=transport")
	require.Contains(t, logText, "retryable_same_account=true")
	require.Contains(t, logText, "retry_next_account=false")
	require.Contains(t, logText, "request_scoped_transient=true")
	require.Contains(t, logText, "safe_to_failover_after_write=true")
	require.Contains(t, logText, "first_output_guard_failure=true")
	require.Contains(t, logText, "semantic_output_started=true")
	require.Contains(t, logText, "response_already_communicated=true")
	require.Contains(t, logText, "same_account_retry_count=1")
	require.Contains(t, logText, "same_account_retry_limit=2")
	require.Contains(t, logText, "configured_capacity=12")
	require.Contains(t, logText, "diagnostic_sample_rate=0")
}

func TestOpenAIAdaptiveDiagnosticResultStillSamplesSuccess(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	scheduler := newOpenAIAdaptiveTestScheduler(&OpenAIGatewayService{})
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 0

	scheduler.logDiagnosticResult(context.Background(), cfg, OpenAIAccountScheduleReport{Success: true})

	require.Empty(t, output.String())
}

func TestOpenAIAdaptiveFailoverDecisionDefaults(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		options    OpenAIAdaptiveFailureReportOptions
		wantResult string
		wantReason string
	}{
		{name: "plain error", err: errors.New("stream failed"), wantResult: OpenAIAdaptiveFailoverOutcomeSuppressed, wantReason: OpenAIAdaptiveFailoverSuppressedPlainError},
		{name: "non retryable request", err: &UpstreamFailoverError{StatusCode: http.StatusBadRequest}, wantResult: OpenAIAdaptiveFailoverOutcomeSuppressed, wantReason: OpenAIAdaptiveFailoverSuppressedNonRetryable},
		{name: "next account stopped", err: &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, NextAccountAction: NextAccountStop}, wantResult: OpenAIAdaptiveFailoverOutcomeSuppressed, wantReason: OpenAIAdaptiveFailoverSuppressedNextAccountStopped},
		{name: "eligible", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway}, wantResult: OpenAIAdaptiveFailoverOutcomeEligible},
		{name: "handler override", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway}, options: OpenAIAdaptiveFailureReportOptions{FailoverOutcome: OpenAIAdaptiveFailoverOutcomeSuppressed, FailoverSuppressedReason: OpenAIAdaptiveFailoverSuppressedSwitchLimit}, wantResult: OpenAIAdaptiveFailoverOutcomeSuppressed, wantReason: OpenAIAdaptiveFailoverSuppressedSwitchLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, reason := openAIAdaptiveFailoverDecision(tt.err, tt.options)
			require.Equal(t, tt.wantResult, result)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestOpenAIStreamFailoverErrorPreservesDiagnosticCodeAndType(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error","type":"server_error","message":"Internal server error"}}}`)

	err := (&OpenAIGatewayService{}).newOpenAIStreamFailoverError(
		nil,
		&Account{ID: 7002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		false,
		"upstream-request-id",
		payload,
		"Internal server error",
	)

	require.Equal(t, "server_error", err.UpstreamErrorCode)
	require.Equal(t, "server_error", err.UpstreamErrorType)
}

func TestOpenAIResponseFailedErrorPreservesAdaptiveDiagnostics(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error","type":"server_error","message":"Internal server error"}}}`)
	err := newOpenAIUpstreamResponseFailedError(payload, "Internal server error")

	require.EqualError(t, err, "upstream response failed: Internal server error")
	metadata := openAIAdaptiveFailureLogMetadataFromError(err)
	require.Equal(t, "upstream_response_failed", metadata.FailureClass)
	require.Equal(t, http.StatusBadGateway, metadata.UpstreamStatus)
	require.Equal(t, "server_error", metadata.UpstreamErrorCode)
	require.Equal(t, "server_error", metadata.UpstreamErrorType)
	require.Equal(t, string(GatewayFailureStageInference), metadata.FailureStage)
	require.False(t, metadata.RetryNextAccount)
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
