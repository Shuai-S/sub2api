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
	require.Equal(t, AnthropicAdaptiveSchedulerModeShadow, normalizeAnthropicAdaptiveSchedulerMode("invalid"))
}

func TestAnthropicAdaptiveSettingsParseAndSerialize(t *testing.T) {
	settings := parseAnthropicAdaptiveSchedulerSettings(map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled:                     "true",
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled:        "true",
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate:     "0.25",
		SettingKeyAnthropicAdaptiveSchedulerMode:                        "ENFORCE",
		SettingKeyAnthropicAdaptiveSchedulerTopK:                        "4",
		SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature:          "0.2",
		SettingKeyAnthropicAdaptiveSchedulerCapacityIncreaseStep:        "2",
		SettingKeyAnthropicAdaptiveSchedulerMinRecentSamplesForShrink:   "12",
		SettingKeyAnthropicAdaptiveSchedulerWeightReliability:           "0.7",
		SettingKeyAnthropicAdaptiveSchedulerWeightCapacity:              "0.2",
		SettingKeyAnthropicAdaptiveSchedulerWeightLatency:               "0.1",
		SettingKeyAnthropicAdaptiveSchedulerWeightExploration:           "0",
		SettingKeyAnthropicAdaptiveSchedulerHardShrinkFailureMultiplier: "3",
	})

	require.True(t, settings.AnthropicAdaptiveSchedulerEnabled)
	require.True(t, settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.25, settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, AnthropicAdaptiveSchedulerModeEnforce, settings.AnthropicAdaptiveSchedulerMode)
	require.Equal(t, 4, settings.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.2, settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, 2, settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep)
	require.Equal(t, 12, settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink)
	require.Equal(t, 3, settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier)
	serialized := anthropicAdaptiveSchedulerSettingsToMap(settings)
	require.Len(t, serialized, 27)
	require.Equal(t, "true", serialized[SettingKeyAnthropicAdaptiveSchedulerEnabled])
	require.Equal(t, "true", serialized[SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled])
	require.Equal(t, "0.25", serialized[SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate])
	require.Equal(t, "enforce", serialized[SettingKeyAnthropicAdaptiveSchedulerMode])
	require.Equal(t, "4", serialized[SettingKeyAnthropicAdaptiveSchedulerTopK])
	require.Equal(t, "0.2", serialized[SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature])
}

func TestNormalizeAnthropicAdaptiveSettingsRejectsInvalidValues(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerTopK = 0
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 2
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = 0.4
	settings.AnthropicAdaptiveSchedulerShrinkFactorHard = 0.8
	settings.AnthropicAdaptiveSchedulerWeightReliability = 0
	settings.AnthropicAdaptiveSchedulerWeightCapacity = 0
	settings.AnthropicAdaptiveSchedulerWeightLatency = 0
	settings.AnthropicAdaptiveSchedulerWeightExploration = 0

	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)

	require.Equal(t, 8, settings.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.05, settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, 0.4, settings.AnthropicAdaptiveSchedulerShrinkFactorHard)
	require.Equal(t, 0.5, settings.AnthropicAdaptiveSchedulerWeightReliability)
	require.Equal(t, 0.3, settings.AnthropicAdaptiveSchedulerWeightCapacity)
}

func TestAnthropicAdaptiveOrderPreservesPriorityLayers(t *testing.T) {
	candidates := []AnthropicAdaptiveCandidate{
		{Account: &Account{ID: 1, Priority: 2}, LoadInfo: &AccountLoadInfo{}, Score: 1.0},
		{Account: &Account{ID: 2, Priority: 1}, LoadInfo: &AccountLoadInfo{}, Score: 0.1},
		{Account: &Account{ID: 3, Priority: 2}, LoadInfo: &AccountLoadInfo{}, Score: 0.9},
		{Account: &Account{ID: 4, Priority: 1}, LoadInfo: &AccountLoadInfo{}, Score: 0.2},
	}

	order := buildAnthropicAdaptiveOrder(candidates, DefaultAnthropicAdaptiveSchedulerSettings())

	require.Len(t, order, len(candidates))
	require.Equal(t, 1, order[0].Account.Priority)
	require.Equal(t, 1, order[1].Account.Priority)
	require.Equal(t, 2, order[2].Account.Priority)
	require.Equal(t, 2, order[3].Account.Priority)
}

func TestAnthropicAdaptiveSoftmaxOrderIsCompleteAndUnique(t *testing.T) {
	candidates := make([]AnthropicAdaptiveCandidate, 0, 12)
	for i := 0; i < 12; i++ {
		candidates = append(candidates, AnthropicAdaptiveCandidate{
			Account:  &Account{ID: int64(i + 1), Priority: 1},
			LoadInfo: &AccountLoadInfo{},
			Score:    float64(i) / 10,
		})
	}

	order := buildAnthropicAdaptiveOrder(candidates, DefaultAnthropicAdaptiveSchedulerSettings())

	require.Len(t, order, len(candidates))
	seen := make(map[int64]struct{}, len(order))
	for _, candidate := range order {
		require.NotNil(t, candidate.Account)
		_, duplicate := seen[candidate.Account.ID]
		require.False(t, duplicate, "account %d appeared more than once", candidate.Account.ID)
		seen[candidate.Account.ID] = struct{}{}
	}
}

func TestAnthropicAdaptiveBuildOrderUsesConfiguredTopKAndScores(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerTopK = 1
	settings.AnthropicAdaptiveSchedulerWeightReliability = 1
	settings.AnthropicAdaptiveSchedulerWeightCapacity = 0
	settings.AnthropicAdaptiveSchedulerWeightLatency = 0
	settings.AnthropicAdaptiveSchedulerWeightExploration = 0

	scheduler.state.mu.Lock()
	first := scheduler.state.ensureLocked(&Account{ID: 1, Concurrency: 5}, time.Now(), settings)
	first.SuccessEMA = 0.2
	second := scheduler.state.ensureLocked(&Account{ID: 2, Concurrency: 5}, time.Now(), settings)
	second.SuccessEMA = 0.9
	scheduler.state.mu.Unlock()

	decision := scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: "claude-sonnet-4-6",
		Candidates: []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 1}},
			{account: &Account{ID: 2, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 2}},
		},
		Settings: &settings,
	})

	require.Equal(t, 1, decision.TopK)
	require.Equal(t, int64(2), decision.SelectedAccountID)
	require.Greater(t, decision.Order[0].ReliabilityScore, decision.Order[1].ReliabilityScore)
}

func TestAnthropicAdaptiveBuildOrderExcludesOpenCircuitAndAllowsHalfOpenProbe(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	now := time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerEnabled = true
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	open := &Account{ID: 1, Platform: PlatformAnthropic, Priority: 1, Concurrency: 5}
	healthy := &Account{ID: 2, Platform: PlatformAnthropic, Priority: 1, Concurrency: 5}

	scheduler.state.mu.Lock()
	state := scheduler.state.ensureLocked(open, now, settings)
	state.CircuitOpenUntil = now.Add(time.Minute)
	state.CircuitProbeInFlight = true
	state.CircuitProbeUntil = now.Add(time.Minute)
	scheduler.state.mu.Unlock()

	decision := scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: "claude-sonnet-4-6",
		Candidates: []accountWithLoad{
			{account: open, loadInfo: &AccountLoadInfo{AccountID: open.ID}},
			{account: healthy, loadInfo: &AccountLoadInfo{AccountID: healthy.ID}},
		},
		Settings: &settings,
	})
	require.Len(t, decision.Order, 1)
	require.Equal(t, healthy.ID, decision.Order[0].Account.ID)

	scheduler.state.mu.Lock()
	state.CircuitProbeInFlight = false
	state.CircuitProbeUntil = time.Time{}
	scheduler.state.mu.Unlock()
	now = now.Add(time.Minute + time.Second)
	decision = scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: "claude-sonnet-4-6",
		Candidates:     []accountWithLoad{{account: open, loadInfo: &AccountLoadInfo{AccountID: open.ID}}},
		Settings:       &settings,
	})
	require.Len(t, decision.Order, 1)
	require.Equal(t, open.ID, decision.Order[0].Account.ID)
}

func TestAnthropicAdaptiveEnforceDoesNotFallBackToOpenCircuitCandidates(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	now := time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerEnabled = true
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	account := &Account{ID: 1, Platform: PlatformAnthropic, Priority: 1, Concurrency: 5}
	scheduler.state.mu.Lock()
	state := scheduler.state.ensureLocked(account, now, settings)
	state.CircuitOpenUntil = now.Add(time.Minute)
	state.CircuitProbeInFlight = true
	state.CircuitProbeUntil = now.Add(time.Minute)
	scheduler.state.mu.Unlock()

	service := &GatewayService{anthropicAdaptiveScheduler: scheduler}
	ordered, capacities, decision := service.anthropicAdaptiveOrder(
		AnthropicAdaptiveSchedulerModeEnforce,
		settings,
		"claude-sonnet-4-6",
		[]accountWithLoad{{account: account, loadInfo: &AccountLoadInfo{AccountID: account.ID}}},
	)

	require.Empty(t, ordered)
	require.Empty(t, capacities)
	require.NotNil(t, decision)
	require.Equal(t, "all_circuits_open", decision.FallbackReason)
}

func TestAnthropicAdaptiveModelHealthOverridesAccountHealthForScoring(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerTopK = 1
	settings.AnthropicAdaptiveSchedulerWeightReliability = 1
	settings.AnthropicAdaptiveSchedulerWeightCapacity = 0
	settings.AnthropicAdaptiveSchedulerWeightLatency = 0
	settings.AnthropicAdaptiveSchedulerWeightExploration = 0
	now := time.Now()

	scheduler.state.mu.Lock()
	first := scheduler.state.ensureLocked(&Account{ID: 1, Concurrency: 5}, now, settings)
	first.SuccessEMA = 0.95
	first.HealthByModelFamily["haiku"] = anthropicAdaptiveHealthState{SuccessEMA: 0.05, TotalSamples: 20, ConsecutiveFailure: 3}
	second := scheduler.state.ensureLocked(&Account{ID: 2, Concurrency: 5}, now, settings)
	second.SuccessEMA = 0.60
	second.HealthByModelFamily["haiku"] = anthropicAdaptiveHealthState{SuccessEMA: 0.60, TotalSamples: 20}
	scheduler.state.mu.Unlock()

	decision := scheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: "claude-haiku-4-5-20251001",
		Candidates: []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 1}},
			{account: &Account{ID: 2, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 2}},
		},
		Settings: &settings,
	})
	require.Equal(t, int64(2), decision.SelectedAccountID)
	require.InDelta(t, 0.05/1.75, decision.Order[1].ReliabilityScore, 1e-9)
}

func TestAnthropicAdaptiveModelHealthDoesNotPolluteAccountHealth(t *testing.T) {
	store := newAnthropicAdaptiveStateStore()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 5}
	now := time.Now()

	store.report(AnthropicAdaptiveScheduleReport{
		Account:        account,
		RequestedModel: "claude-haiku-4-5-20251001",
		HealthSample:   true,
		HealthScope:    "model",
	}, now, settings)

	state := store.snapshot(account, settings)
	require.Equal(t, settings.AnthropicAdaptiveSchedulerInitialReliability, state.SuccessEMA)
	require.Zero(t, state.AccountHealthSamples)
	require.Zero(t, state.AccountHealthFailures)
	require.Zero(t, state.AccountConsecutiveFailure)
	require.Equal(t, int64(1), state.HealthByModelFamily["haiku"].TotalSamples)
	require.Equal(t, 1, state.HealthByModelFamily["haiku"].ConsecutiveFailure)
}

func TestAnthropicAdaptiveFailureSampleDeduplicatesSameRequestRetry(t *testing.T) {
	store := newAnthropicAdaptiveStateStore()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 5}
	now := time.Now()
	require.True(t, store.claimFailureSample(account.ID, "request-1", "claude-sonnet-4-6", now))
	store.report(AnthropicAdaptiveScheduleReport{
		Account:        account,
		RequestID:      "request-1",
		RequestedModel: "claude-sonnet-4-6",
		HealthSample:   true,
		HealthScope:    "account",
	}, now, settings)
	require.False(t, store.claimFailureSample(account.ID, "request-1", "claude-sonnet-4-6", now.Add(time.Second)))
	state := store.snapshot(account, settings)
	require.Equal(t, int64(1), state.TotalSamples)
	require.Equal(t, 1, state.AccountHealthFailures)
}

func TestAnthropicAdaptiveCapacityLearningUsesConfiguredGrowthAndShrink(t *testing.T) {
	store := newAnthropicAdaptiveStateStore()
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	now := time.Now()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep = 3
	settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold = 0.9
	settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold = 1
	settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink = 1
	settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold = 0
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = 0.5
	settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier = 10

	store.mu.Lock()
	state := store.ensureLocked(account, now, settings)
	state.EstimatedCapacity = 2
	state.SuccessEMA = 0.99
	state.ConsecutiveSuccess = 2
	store.mu.Unlock()

	stateAfterGrowth := store.observeLoad(account, &AccountLoadInfo{CurrentConcurrency: 2}, now, settings)
	require.Equal(t, 5, stateAfterGrowth.EstimatedCapacity)

	_, decreased := store.report(AnthropicAdaptiveScheduleReport{
		Account:        account,
		CapacitySample: true,
	}, now, settings)
	require.True(t, decreased)
	require.Equal(t, 2, store.effectiveCapacity(account, settings))
}

func TestAnthropicAdaptiveShadowOnlyObservesOrder(t *testing.T) {
	svc := &GatewayService{anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler()}
	input := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 1}},
		{account: &Account{ID: 2, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{AccountID: 2}},
	}

	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	actual, capacities, decision := svc.anthropicAdaptiveOrder(AnthropicAdaptiveSchedulerModeShadow, settings, "claude-sonnet-4-6", input)

	require.Equal(t, []int64{1, 2}, adaptiveAccountIDs(actual))
	require.NotNil(t, capacities)
	require.NotNil(t, decision)
	require.Len(t, decision.Order, 2)
}

func TestAnthropicAdaptiveCapacityKeepsUnlimitedConcurrency(t *testing.T) {
	scheduler := newAnthropicAdaptiveScheduler()
	svc := &GatewayService{anthropicAdaptiveScheduler: scheduler}
	unlimited := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 0}
	limited := &Account{ID: 2, Platform: PlatformAnthropic, Concurrency: 10}
	now := time.Now()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()

	scheduler.state.mu.Lock()
	state := scheduler.state.ensureLocked(limited, now, settings)
	state.EstimatedCapacity = 4
	scheduler.state.mu.Unlock()

	require.Zero(t, scheduler.state.effectiveCapacity(unlimited, settings))
	require.Zero(t, svc.anthropicAdaptiveCapacity(AnthropicAdaptiveSchedulerModeEnforce, settings, unlimited))
	require.Equal(t, limited.Concurrency, svc.anthropicAdaptiveCapacity(AnthropicAdaptiveSchedulerModeShadow, settings, limited))
	require.Equal(t, 4, svc.anthropicAdaptiveCapacity(AnthropicAdaptiveSchedulerModeEnforce, settings, limited))
}

func TestClassifyAnthropicAdaptiveResultOnlyMarksExplicitConcurrencyForCapacity(t *testing.T) {
	ctx := context.Background()
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}

	tests := []struct {
		name               string
		err                error
		wantHealthSample   bool
		wantCapacitySample bool
		wantReason         string
	}{
		{
			name:       "provider overload",
			err:        &UpstreamFailoverError{StatusCode: 529, Scope: GatewayFailureScopeAccount},
			wantReason: "provider_overloaded",
		},
		{
			name:             "generic rate limit",
			err:              &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount},
			wantHealthSample: true,
			wantReason:       "generic_rate_limit",
		},
		{
			name: "window rate limit",
			err: &UpstreamFailoverError{
				StatusCode: http.StatusTooManyRequests,
				Scope:      GatewayFailureScopeAccount,
				ResponseHeaders: http.Header{
					"Anthropic-Ratelimit-Unified-5h-Status": []string{"rejected"},
				},
			},
			wantReason: "window_rate_limit",
		},
		{
			name:       "local queue failure",
			err:        errors.New("timeout waiting for account concurrency slot"),
			wantReason: "local_queue",
		},
		{
			name: "account concurrency limit",
			err: &UpstreamFailoverError{
				StatusCode:   http.StatusTooManyRequests,
				Scope:        GatewayFailureScopeAccount,
				ResponseBody: []byte(`{"error":{"message":"Concurrency limit exceeded for account"}}`),
			},
			wantHealthSample:   true,
			wantCapacitySample: true,
			wantReason:         "concurrency_limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := classifyAnthropicAdaptiveResult(ctx, account, "claude-sonnet-4-6", nil, tt.err)
			require.Equal(t, tt.wantHealthSample, report.HealthSample)
			require.Equal(t, tt.wantCapacitySample, report.CapacitySample)
			require.Equal(t, tt.wantReason, report.TerminalReason)
		})
	}
}

func TestClassifyAnthropicAdaptiveResultHonorsHealthSampleOverride(t *testing.T) {
	falseValue := false
	trueValue := true
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}

	genericRateLimit := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, &UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		Scope:        GatewayFailureScopeAccount,
		HealthSample: &falseValue,
	})
	providerOverload := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, &UpstreamFailoverError{
		StatusCode:   529,
		Scope:        GatewayFailureScopeAccount,
		HealthSample: &trueValue,
	})

	require.False(t, genericRateLimit.HealthSample)
	require.True(t, providerOverload.HealthSample)
}

func TestClassifyAnthropicAdaptiveTransportFailoverPenalizesAccount(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	healthSample := false

	report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, &UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		Scope:             GatewayFailureScopeAccount,
		FailureKind:       UpstreamFailureKindTransport,
		NextAccountAction: NextAccountRetry,
		HealthSample:      &healthSample,
	})

	require.Equal(t, "transport_error", report.TerminalReason)
	require.True(t, report.HealthSample)
	require.False(t, report.CapacitySample)
	require.False(t, report.Success)
}

func TestClassifyAnthropicAdaptiveResultTreatsPolicyFailureAsRequestScopedBeforeHealthOverride(t *testing.T) {
	trueValue := true
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	policyBody := []byte(`{"error":{"message":"This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request. To get authorized for security work, join the Trusted Access for Cyber program."}}`)

	for _, scope := range []GatewayFailureScope{
		GatewayFailureScopeAccount,
		GatewayFailureScopeRequest,
		GatewayFailureScopeProvider,
	} {
		t.Run(string(scope), func(t *testing.T) {
			err := &UpstreamFailoverError{
				StatusCode:   http.StatusForbidden,
				Scope:        scope,
				HealthSample: &trueValue,
				ResponseBody: policyBody,
			}

			report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, err)

			require.Equal(t, "request_policy", report.TerminalReason)
			require.False(t, report.HealthSample)
			require.False(t, report.CapacitySample)
		})
	}
}

func TestClassifyAnthropicAdaptiveAnthropicRequestErrorsAreNotAccountHealth(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	for _, message := range []string{
		`upstream error: 400 message=thinking.adaptive.output_config: Extra inputs are not permitted`,
		`upstream error: 400 message=messages.8.content.4: each tool_use must have a single result`,
	} {
		report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", nil, errors.New(message))
		require.Equal(t, "request_policy", report.TerminalReason)
		require.False(t, report.HealthSample)
	}
}

func TestClassifyAnthropicAdaptiveModelStatusUsesModelHealthScope(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound} {
		report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-haiku-4-5-20251001", nil, &UpstreamFailoverError{
			StatusCode: status,
			Scope:      GatewayFailureScopeAccount,
		})
		require.Equal(t, "model_upstream_error", report.TerminalReason)
		require.True(t, report.HealthSample)
		require.Equal(t, "model", report.HealthScope)
	}
}

func TestClassifyAnthropicAdaptiveIncompleteStreamUsesModelHealthScope(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-fable-5", nil, errors.New("stream usage incomplete: missing terminal event"))
	require.Equal(t, "stream_incomplete", report.TerminalReason)
	require.True(t, report.HealthSample)
	require.Equal(t, "model", report.HealthScope)
}

func TestClassifyAnthropicAdaptiveSyntheticSuccessKeepsLearningSemantics(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	firstTokenMs := 42
	result := &ForwardResult{
		RequestID:     "synthetic-request",
		UpstreamModel: "claude-sonnet-4-6-20260101",
		Stream:        true,
		Synthetic:     true,
		FirstTokenMs:  &firstTokenMs,
		Duration:      250 * time.Millisecond,
	}

	report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", result, nil)

	require.True(t, report.Synthetic)
	require.True(t, report.Success)
	require.True(t, report.HealthSample)
	require.True(t, report.CapacitySample)
	require.Equal(t, "success", report.TerminalReason)
	require.Equal(t, "synthetic-request", report.UpstreamRequestID)
	require.Equal(t, int64(250), report.DurationMs)
}

func TestClassifyAnthropicAdaptiveFailureKeepsPartialResultMetadata(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	firstTokenMs := 42
	result := &ForwardResult{
		RequestID:     "partial-request",
		UpstreamModel: "claude-sonnet-4-6-20260101",
		Stream:        true,
		FirstTokenMs:  &firstTokenMs,
		Duration:      250 * time.Millisecond,
	}
	err := &UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
		Scope:      GatewayFailureScopeAccount,
	}

	report := classifyAnthropicAdaptiveResult(context.Background(), account, "claude-sonnet-4-6", result, err)

	require.Equal(t, "partial-request", report.UpstreamRequestID)
	require.Equal(t, "claude-sonnet-4-6-20260101", report.MappedModel)
	require.True(t, report.Stream)
	require.Equal(t, &firstTokenMs, report.FirstTokenMs)
	require.Equal(t, int64(250), report.DurationMs)
	require.Equal(t, "upstream_5xx", report.TerminalReason)
}

func TestAnthropicAdaptiveCapacityShrinksOnExplicitConcurrencyEvidence(t *testing.T) {
	store := newAnthropicAdaptiveStateStore()
	account := &Account{ID: 1, Platform: PlatformAnthropic, Concurrency: 10}
	now := time.Now()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()

	store.mu.Lock()
	state := store.ensureLocked(account, now, settings)
	state.EstimatedCapacity = 10
	state.RecentWindowStartedAt = now
	state.RecentCapacitySamples = 29
	state.RecentCapacityFailures = 7
	state.ConsecutiveCapacityFailure = 2
	store.mu.Unlock()

	_, decreased := store.report(AnthropicAdaptiveScheduleReport{
		Account:        account,
		HealthSample:   true,
		CapacitySample: true,
		TerminalReason: "concurrency_limit",
	}, now, settings)

	require.True(t, decreased)
	require.Equal(t, 8, store.effectiveCapacity(account, settings))
}

func adaptiveAccountIDs(candidates []accountWithLoad) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.account.ID)
	}
	return ids
}
