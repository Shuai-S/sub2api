package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnthropicAdaptiveSchedulerDefaultsDisabled(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()

	require.False(t, settings.AnthropicAdaptiveSchedulerEnabled)
	require.False(t, settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.05, settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, AnthropicAdaptiveSchedulerModeShadow, settings.AnthropicAdaptiveSchedulerMode)
	require.Equal(t, 8, settings.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.35, settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, AnthropicAdaptiveSchedulerModeShadow, normalizeAnthropicAdaptiveSchedulerMode("invalid"))
}

func TestAnthropicAdaptiveSettingsParseAndSerialize(t *testing.T) {
	settings := parseAnthropicAdaptiveSchedulerSettings(map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled:                  "true",
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled:     "true",
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate:  "0.25",
		SettingKeyAnthropicAdaptiveSchedulerMode:                     "ENFORCE",
		SettingKeyAnthropicAdaptiveSchedulerTopK:                     "4",
		SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature:       "0.2",
		SettingKeyAnthropicAdaptiveSchedulerWeightReliability:        "0.5",
		SettingKeyAnthropicAdaptiveSchedulerWeightCapacity:           "0.2",
		SettingKeyAnthropicAdaptiveSchedulerWeightLatency:            "0.15",
		SettingKeyAnthropicAdaptiveSchedulerWeightCost:               "0.15",
		SettingKeyAnthropicAdaptiveSchedulerWeightCache:              "0.1",
		SettingKeyAnthropicAdaptiveSchedulerLearningMinHealthSamples: "40",
		SettingKeyAnthropicAdaptiveSchedulerHealthFailureThreshold:   "4",
		SettingKeyAnthropicAdaptiveSchedulerCooldownMaxSeconds:       "480",
	})

	require.True(t, settings.AnthropicAdaptiveSchedulerEnabled)
	require.True(t, settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.25, settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, AnthropicAdaptiveSchedulerModeEnforce, settings.AnthropicAdaptiveSchedulerMode)
	require.Equal(t, 4, settings.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.2, settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, 40, settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples)
	require.Equal(t, 4, settings.AnthropicAdaptiveSchedulerHealthFailureThreshold)
	require.Equal(t, 480, settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds)
	require.Equal(t, 0.1, settings.AnthropicAdaptiveSchedulerWeightCache)

	serialized := anthropicAdaptiveSchedulerSettingsToMap(settings)
	require.Len(t, serialized, 29)
	require.Equal(t, "true", serialized[SettingKeyAnthropicAdaptiveSchedulerEnabled])
	require.Equal(t, "enforce", serialized[SettingKeyAnthropicAdaptiveSchedulerMode])
	require.Equal(t, "4", serialized[SettingKeyAnthropicAdaptiveSchedulerTopK])
	require.Equal(t, "0.1", serialized[SettingKeyAnthropicAdaptiveSchedulerWeightCache])
}

func TestNormalizeAnthropicAdaptiveSettingsRestoresCoreWeights(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerTopK = 0
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 2
	settings.AnthropicAdaptiveSchedulerWeightReliability = 0
	settings.AnthropicAdaptiveSchedulerWeightCapacity = 0
	settings.AnthropicAdaptiveSchedulerWeightLatency = 0
	settings.AnthropicAdaptiveSchedulerWeightCost = 0

	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)

	require.Equal(t, 8, settings.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.05, settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, 0.5, settings.AnthropicAdaptiveSchedulerWeightReliability)
	require.Equal(t, 0.2, settings.AnthropicAdaptiveSchedulerWeightCapacity)
	require.Equal(t, 0.15, settings.AnthropicAdaptiveSchedulerWeightLatency)
	require.Equal(t, 0.15, settings.AnthropicAdaptiveSchedulerWeightCost)
}

func TestAnthropicAdaptiveBuildOrderUsesAccountScoreAndIgnoresPriority(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	settings.AnthropicAdaptiveSchedulerTopK = 1
	settings.AnthropicAdaptiveSchedulerExplorationRate = 0
	settings.AnthropicAdaptiveSchedulerWeightReliability = 1
	settings.AnthropicAdaptiveSchedulerWeightCapacity = 0
	settings.AnthropicAdaptiveSchedulerWeightLatency = 0
	settings.AnthropicAdaptiveSchedulerWeightCost = 0

	scheduler.core.mu.Lock()
	low := scheduler.core.ensureLocked(1, 5, now)
	low.SuccessEMA = 0.1
	high := scheduler.core.ensureLocked(2, 5, now)
	high.SuccessEMA = 0.9
	for i := 0; i < settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples; i++ {
		at := now.Add(-time.Duration(i) * time.Second)
		low.HealthObservations = append(low.HealthObservations, adaptiveHealthObservation{At: at, Success: false})
		high.HealthObservations = append(high.HealthObservations, adaptiveHealthObservation{At: at, Success: true})
	}
	scheduler.core.mu.Unlock()

	decision := scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: "claude-sonnet-4-6",
		Candidates: []accountWithLoad{
			{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 1}},
			{account: &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 99, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 2}},
		},
		Settings:   &settings,
		NewSession: false,
	})

	require.Equal(t, 1, decision.TopK)
	require.Equal(t, int64(2), decision.SelectedAccountID)
	require.Greater(t, decision.Order[0].ReliabilityScore, decision.Order[1].ReliabilityScore)
}

func TestAnthropicAdaptiveBuildOrderKeepsOAuthHardPriority(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	settings.AnthropicAdaptiveSchedulerTopK = 1
	settings.AnthropicAdaptiveSchedulerExplorationRate = 0

	decision := scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		Candidates: []accountWithLoad{
			{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 1}},
			{account: &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 2}},
		},
		Settings: &settings,
	})

	require.Equal(t, int64(2), decision.SelectedAccountID)
}

func TestAnthropicAdaptiveBuildOrderExcludesOpenAccountCircuit(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	open := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 5}
	healthy := &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 5}

	scheduler.core.mu.Lock()
	state := scheduler.core.ensureLocked(open.ID, open.Concurrency, now)
	state.CircuitOpenUntil = now.Add(time.Minute)
	scheduler.core.mu.Unlock()

	decision := scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		Candidates: []accountWithLoad{
			{account: open, loadInfo: &AccountLoadInfo{AccountID: open.ID}},
			{account: healthy, loadInfo: &AccountLoadInfo{AccountID: healthy.ID}},
		},
		Settings: &settings,
	})

	require.Len(t, decision.Order, 1)
	require.Equal(t, healthy.ID, decision.SelectedAccountID)
}

func TestAnthropicAdaptiveEnforceDoesNotFallbackPastAccountCircuit(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 5}
	scheduler.core.mu.Lock()
	scheduler.core.ensureLocked(account.ID, account.Concurrency, now).CircuitOpenUntil = now.Add(time.Minute)
	scheduler.core.mu.Unlock()

	service := &GatewayService{anthropicAdaptiveScheduler: scheduler}
	ordered, capacities, decision := service.anthropicAdaptiveOrder(
		AnthropicAdaptiveSchedulerModeEnforce,
		settings,
		"claude-sonnet-4-6",
		true,
		[]accountWithLoad{{account: account, loadInfo: &AccountLoadInfo{AccountID: account.ID}}},
	)

	require.Empty(t, ordered)
	require.Empty(t, capacities)
	require.NotNil(t, decision)
	require.Equal(t, "all_circuits_open", decision.FallbackReason)
}

func TestAnthropicAdaptiveShadowOnlyObservesOrder(t *testing.T) {
	service := &GatewayService{anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler()}
	input := []accountWithLoad{
		{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 1}},
		{account: &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 2}},
	}

	actual, capacities, decision := service.anthropicAdaptiveOrder(AnthropicAdaptiveSchedulerModeShadow, DefaultAnthropicAdaptiveSchedulerSettings(), "claude-sonnet-4-6", true, input)

	require.Equal(t, []int64{1, 2}, adaptiveAccountIDs(actual))
	require.NotNil(t, capacities)
	require.NotNil(t, decision)
	require.Len(t, decision.Order, 2)
}

func TestClassifyAnthropicAdaptiveResultMapsExistingSignalsToCoreObservations(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 10}
	tests := []struct {
		name               string
		err                error
		wantReason         string
		wantObservation    adaptiveObservationType
		wantAuthentication bool
	}{
		{name: "provider overload", err: &UpstreamFailoverError{StatusCode: 529, Scope: GatewayFailureScopeAccount}, wantReason: "provider_overloaded", wantObservation: adaptiveObservationProviderOverload},
		{name: "generic rate limit", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount}, wantReason: "generic_rate_limit", wantObservation: adaptiveObservationAccountFailure},
		{name: "window rate limit", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount, ResponseHeaders: http.Header{"Anthropic-Ratelimit-Unified-5h-Status": []string{"rejected"}}}, wantReason: "window_rate_limit", wantObservation: adaptiveObservationQuotaLimit},
		{name: "local queue", err: errors.New("timeout waiting for account concurrency slot"), wantReason: "local_queue", wantObservation: adaptiveObservationIgnored},
		{name: "account concurrency", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount, ResponseBody: []byte(`{"error":{"message":"Concurrency limit exceeded for account"}}`)}, wantReason: "concurrency_limit", wantObservation: adaptiveObservationCapacityLimit},
		{name: "authentication", err: &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, Scope: GatewayFailureScopeAccount}, wantReason: "account_auth", wantObservation: adaptiveObservationAccountFailure, wantAuthentication: true},
		{name: "model scoped", err: &UpstreamFailoverError{StatusCode: http.StatusNotFound, Scope: GatewayFailureScopeAccount}, wantReason: "model_upstream_error", wantObservation: adaptiveObservationProviderOverload},
		{name: "health override excluded", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway, Scope: GatewayFailureScopeAccount, HealthSample: boolPtr(false)}, wantReason: "upstream_5xx", wantObservation: adaptiveObservationIgnored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, tt.err)
			observation, authentication := anthropicAdaptiveObservation(report)
			require.Equal(t, tt.wantReason, report.TerminalReason)
			require.Equal(t, tt.wantObservation, observation)
			require.Equal(t, tt.wantAuthentication, authentication)
		})
	}
}

func TestClassifyAnthropicAdaptiveRequestPolicyIsIgnored(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 10}
	report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, errors.New("upstream error: 400 message=thinking.adaptive.output_config: Extra inputs are not permitted"))

	require.Equal(t, "request_policy", report.TerminalReason)
	require.False(t, report.HealthSample)
	observation, _ := anthropicAdaptiveObservation(report)
	require.Equal(t, adaptiveObservationIgnored, observation)
}

func TestClassifyAnthropicAdaptiveFailureKeepsPartialResultMetadata(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 10}
	firstTokenMs := 42
	result := &ForwardResult{
		RequestID:     "partial-request",
		UpstreamModel: "claude-sonnet-4-6-20260101",
		Stream:        true,
		FirstTokenMs:  &firstTokenMs,
		Duration:      250 * time.Millisecond,
	}
	err := &UpstreamFailoverError{StatusCode: http.StatusBadGateway, Scope: GatewayFailureScopeAccount}

	report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", result, err)

	require.Equal(t, "partial-request", report.UpstreamRequestID)
	require.Equal(t, "claude-sonnet-4-6-20260101", report.MappedModel)
	require.True(t, report.Stream)
	require.Equal(t, &firstTokenMs, report.FirstTokenMs)
	require.Equal(t, int64(250), report.DurationMs)
	require.Equal(t, "upstream_5xx", report.TerminalReason)
}

func TestAnthropicAdaptiveCapacityKeepsUnlimitedConcurrency(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	service := &GatewayService{anthropicAdaptiveScheduler: scheduler}
	unlimited := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 0}
	settings := DefaultAnthropicAdaptiveSchedulerSettings()

	require.Zero(t, service.anthropicAdaptiveCapacity(AnthropicAdaptiveSchedulerModeEnforce, settings, unlimited))
}

func adaptiveAccountIDs(candidates []accountWithLoad) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.account.ID)
	}
	return ids
}
