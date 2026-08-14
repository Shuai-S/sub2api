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
	require.Equal(t, "concurrency_limit", openAIAdaptiveFailureCooldownReason(errors.New("upstream websocket is busy, please retry later")))
}

func TestOpenAIAdaptiveFailureHealthSampleOverride(t *testing.T) {
	falseValue := false
	trueValue := true

	require.False(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusBadGateway, HealthSample: &falseValue}))
	require.True(t, openAIAdaptiveFailureHealthSample(&UpstreamFailoverError{StatusCode: http.StatusBadRequest, HealthSample: &trueValue}))
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
