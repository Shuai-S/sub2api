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
	require.Equal(t, AnthropicAdaptiveSchedulerModeShadow, settings.AnthropicAdaptiveSchedulerMode)
	require.Equal(t, AnthropicAdaptiveSchedulerModeShadow, normalizeAnthropicAdaptiveSchedulerMode("invalid"))
}

func TestAnthropicAdaptiveSettingsParseAndSerialize(t *testing.T) {
	settings := parseAnthropicAdaptiveSchedulerSettings(map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled:                     "true",
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
	require.Equal(t, AnthropicAdaptiveSchedulerModeEnforce, settings.AnthropicAdaptiveSchedulerMode)
	require.Equal(t, 4, settings.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.2, settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, 2, settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep)
	require.Equal(t, 12, settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink)
	require.Equal(t, 3, settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier)
	serialized := anthropicAdaptiveSchedulerSettingsToMap(settings)
	require.Len(t, serialized, 25)
	require.Equal(t, "true", serialized[SettingKeyAnthropicAdaptiveSchedulerEnabled])
	require.Equal(t, "enforce", serialized[SettingKeyAnthropicAdaptiveSchedulerMode])
	require.Equal(t, "4", serialized[SettingKeyAnthropicAdaptiveSchedulerTopK])
	require.Equal(t, "0.2", serialized[SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature])
}

func TestNormalizeAnthropicAdaptiveSettingsRejectsInvalidValues(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerTopK = 0
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = 0.4
	settings.AnthropicAdaptiveSchedulerShrinkFactorHard = 0.8
	settings.AnthropicAdaptiveSchedulerWeightReliability = 0
	settings.AnthropicAdaptiveSchedulerWeightCapacity = 0
	settings.AnthropicAdaptiveSchedulerWeightLatency = 0
	settings.AnthropicAdaptiveSchedulerWeightExploration = 0

	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)

	require.Equal(t, 8, settings.AnthropicAdaptiveSchedulerTopK)
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
