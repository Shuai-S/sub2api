package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGeminiAdaptiveSchedulerDefaultsDisabled(t *testing.T) {
	settings := DefaultGeminiAdaptiveSchedulerSettings()

	require.False(t, settings.GeminiAdaptiveSchedulerEnabled)
	require.Equal(t, GeminiAdaptiveSchedulerModeShadow, settings.GeminiAdaptiveSchedulerMode)
	require.Equal(t, GeminiAdaptiveSchedulerModeShadow, normalizeGeminiAdaptiveSchedulerMode("invalid"))
}

func TestGeminiAdaptiveScoresUseAllSixDimensions(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	lowCost := 0.5
	highCost := 2.0
	candidates := []GeminiAdaptiveCandidate{
		{
			Account:           &Account{ID: 1, Platform: PlatformGemini, Concurrency: 10, RateMultiplier: &lowCost},
			Load:              &AccountLoadInfo{AccountID: 1, CurrentConcurrency: 1},
			EffectiveCapacity: 10,
			Quota: GeminiAdaptiveQuotaSnapshot{
				DailyUsed:     10,
				DailyLimit:    100,
				DailyResetAt:  now.Add(12 * time.Hour),
				MinuteUsed:    1,
				MinuteLimit:   100,
				MinuteResetAt: now.Add(30 * time.Second),
				DataAvailable: true,
			},
			state: geminiAdaptiveAccountState{
				PathSuccessEMA: 0.95,
				TotalSamples:   1,
				ByModelFamily: map[string]geminiAdaptiveModelState{
					"pro": {SuccessEMA: 0.9, LatencyEMA: 100, Samples: 10},
				},
			},
		},
		{
			Account:           &Account{ID: 2, Platform: PlatformGemini, Concurrency: 10, RateMultiplier: &highCost},
			Load:              &AccountLoadInfo{AccountID: 2, CurrentConcurrency: 9},
			EffectiveCapacity: 10,
			Quota: GeminiAdaptiveQuotaSnapshot{
				DailyUsed:     90,
				DailyLimit:    100,
				DailyResetAt:  now.Add(12 * time.Hour),
				MinuteUsed:    90,
				MinuteLimit:   100,
				MinuteResetAt: now.Add(30 * time.Second),
				DataAvailable: true,
			},
			state: geminiAdaptiveAccountState{
				PathSuccessEMA: 0.5,
				TotalSamples:   100,
				ByModelFamily: map[string]geminiAdaptiveModelState{
					"pro": {SuccessEMA: 0.4, LatencyEMA: 1000, Samples: 10},
				},
			},
		},
	}

	applyGeminiAdaptiveScores(candidates, "gemini-2.5-pro", "generateContent", false, now, settings)

	require.Greater(t, candidates[0].ReliabilityScore, candidates[1].ReliabilityScore)
	require.Greater(t, candidates[0].QuotaScore, candidates[1].QuotaScore)
	require.Greater(t, candidates[0].CapacityScore, candidates[1].CapacityScore)
	require.Greater(t, candidates[0].LatencyScore, candidates[1].LatencyScore)
	require.Greater(t, candidates[0].CostScore, candidates[1].CostScore)
	require.Greater(t, candidates[0].ExplorationScore, candidates[1].ExplorationScore)
	require.Greater(t, candidates[0].Score, candidates[1].Score)

	weighted := settings.GeminiAdaptiveSchedulerWeightReliability*candidates[0].ReliabilityScore +
		settings.GeminiAdaptiveSchedulerWeightQuota*candidates[0].QuotaScore +
		settings.GeminiAdaptiveSchedulerWeightCapacity*candidates[0].CapacityScore +
		settings.GeminiAdaptiveSchedulerWeightLatency*candidates[0].LatencyScore +
		settings.GeminiAdaptiveSchedulerWeightCost*candidates[0].CostScore +
		settings.GeminiAdaptiveSchedulerWeightExploration*candidates[0].ExplorationScore
	require.InDelta(t, weighted, candidates[0].Score, 1e-12)
}

func TestGeminiAdaptiveOrderPreservesPriorityAndTopKTail(t *testing.T) {
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerTopK = 1
	candidates := []GeminiAdaptiveCandidate{
		{Account: &Account{ID: 1, Priority: 2}, Load: &AccountLoadInfo{}, Score: 100},
		{Account: &Account{ID: 2, Priority: 1}, Load: &AccountLoadInfo{}, Score: 0.7},
		{Account: &Account{ID: 3, Priority: 1}, Load: &AccountLoadInfo{}, Score: 0.9},
		{Account: &Account{ID: 4, Priority: 1}, Load: &AccountLoadInfo{}, Score: 0.8},
	}

	order := buildGeminiAdaptiveOrder(candidates, settings)

	require.Equal(t, []int64{3, 4, 2, 1}, geminiAdaptiveCandidateIDs(order))
	require.Equal(t, []int{1, 1, 1, 2}, []int{
		order[0].Account.Priority,
		order[1].Account.Priority,
		order[2].Account.Priority,
		order[3].Account.Priority,
	})
}

func TestGeminiAdaptiveMixedPoolOnlyReordersNativeSlots(t *testing.T) {
	scheduler := newGeminiAdaptiveScheduler()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerTopK = 1
	settings.GeminiAdaptiveSchedulerWeightReliability = 1
	settings.GeminiAdaptiveSchedulerWeightQuota = 0
	settings.GeminiAdaptiveSchedulerWeightCapacity = 0
	settings.GeminiAdaptiveSchedulerWeightLatency = 0
	settings.GeminiAdaptiveSchedulerWeightCost = 0
	settings.GeminiAdaptiveSchedulerWeightExploration = 0
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }

	native1 := &Account{ID: 1, Platform: PlatformGemini, Priority: 1, Concurrency: 5}
	antigravity1 := &Account{ID: 2, Platform: PlatformAntigravity, Priority: 1, Concurrency: 5}
	native2 := &Account{ID: 3, Platform: PlatformGemini, Priority: 1, Concurrency: 5}
	antigravity2 := &Account{ID: 4, Platform: PlatformAntigravity, Priority: 2, Concurrency: 5}
	native3 := &Account{ID: 5, Platform: PlatformGemini, Priority: 2, Concurrency: 5}

	scheduler.state.mu.Lock()
	setReliability := func(account *Account, reliability float64) {
		state := scheduler.state.ensureLocked(account, now, settings)
		state.PathSuccessEMA = reliability
		state.ByModelFamily["pro"] = geminiAdaptiveModelState{SuccessEMA: reliability, Samples: 1}
	}
	setReliability(native1, 0.1)
	setReliability(native2, 0.9)
	setReliability(native3, 0.5)
	scheduler.state.mu.Unlock()

	decision, err := scheduler.BuildOrder(GeminiAdaptiveScheduleRequest{
		RequestedModel: "gemini-2.5-pro",
		Candidates: []GeminiAdaptiveCandidateInput{
			{Account: native1},
			{Account: antigravity1},
			{Account: native2},
			{Account: antigravity2},
			{Account: native3},
		},
		BaselineOrder: []int64{1, 2, 3, 4, 5},
		Settings:      &settings,
	})

	require.NoError(t, err)
	require.Equal(t, 3, decision.CandidateCount)
	require.Equal(t, 1, decision.TopK)
	require.Equal(t, []int64{3, 2, 1, 4, 5}, geminiAdaptiveCandidateIDs(decision.Order))
}

func TestClassifyGeminiAdaptiveResultOnlyExplicitConcurrencyShrinksCapacity(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformGemini, Concurrency: 10}
	tests := []struct {
		name               string
		err                error
		wantPathSample     bool
		wantModelSample    bool
		wantCapacitySample bool
		wantReason         string
	}{
		{
			name:            "generic rate limit",
			err:             &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount},
			wantModelSample: true,
			wantReason:      "generic_rate_limit",
		},
		{
			name: "explicit quota rate limit",
			err: &UpstreamFailoverError{
				StatusCode:   http.StatusTooManyRequests,
				Scope:        GatewayFailureScopeAccount,
				ResponseBody: []byte(`{"error":{"message":"requests per minute quota exceeded"}}`),
			},
			wantReason: "quota_rate_limit",
		},
		{
			name: "provider model capacity",
			err: &UpstreamFailoverError{
				StatusCode:   http.StatusServiceUnavailable,
				Scope:        GatewayFailureScopeAccount,
				ResponseBody: []byte(`{"error":{"reason":"MODEL_CAPACITY_EXHAUSTED"}}`),
			},
			wantReason: "provider_capacity",
		},
		{
			name:       "local queue",
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
			wantPathSample:     true,
			wantModelSample:    true,
			wantCapacitySample: true,
			wantReason:         "concurrency_limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := classifyGeminiAdaptiveResult(context.Background(), account, "gemini-2.5-pro", "generateContent", nil, tt.err)

			require.Equal(t, tt.wantPathSample, report.PathSample)
			require.Equal(t, tt.wantModelSample, report.ModelSample)
			require.Equal(t, tt.wantCapacitySample, report.CapacitySample)
			require.Equal(t, tt.wantReason, report.TerminalReason)
		})
	}
}

func TestGeminiAdaptiveCapacityShrinksAndProbesBackUp(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	store := newGeminiAdaptiveStateStore()
	account := &Account{ID: 1, Platform: PlatformGemini, Concurrency: 10}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	capacityCtx := context.WithValue(context.Background(), ctxkey.RequestID, "capacity-request")

	store.mu.Lock()
	state := store.ensureLocked(account, now, settings)
	state.EstimatedCapacity = 10
	state.RecentWindowStartedAt = now
	state.RecentCapacitySamples = 29
	state.RecentCapacityFailures = 7
	state.ConsecutiveCapacityFailure = 2
	store.mu.Unlock()

	_, decreased := store.report(GeminiAdaptiveScheduleReport{
		Account:        account,
		CapacitySample: true,
		TerminalReason: "concurrency_limit",
		ctx:            capacityCtx,
	}, now, settings)

	require.True(t, decreased)
	require.Equal(t, 8, store.effectiveCapacity(account, settings))

	probeAt := now.Add(time.Duration(settings.GeminiAdaptiveSchedulerCooldownSeconds+1) * time.Second)
	store.mu.Lock()
	state = store.ensureLocked(account, probeAt, settings)
	state.PathSuccessEMA = 0.99
	state.ConsecutiveSuccess = state.EstimatedCapacity
	state.RecentCapacityFailures = 0
	store.mu.Unlock()

	observed := store.observeLoad(capacityCtx, account, &AccountLoadInfo{
		AccountID:          account.ID,
		CurrentConcurrency: 8,
	}, probeAt, settings)

	require.Equal(t, 9, observed.EstimatedCapacity)
	require.Equal(t, 2, strings.Count(output.String(), "gemini_adaptive_scheduler_capacity_changed"))
	require.Contains(t, output.String(), "direction=decrease")
	require.Contains(t, output.String(), "direction=increase")
	require.Contains(t, output.String(), "previous_capacity=10")
	require.Contains(t, output.String(), "previous_capacity=8")
	require.Equal(t, 2, strings.Count(output.String(), "request_id=capacity-request"))
}

func TestGeminiAdaptiveModelCircuitOpensAndAllowsSingleLocalProbe(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = GeminiAdaptiveSchedulerModeEnforce
	account := &Account{ID: 81, Platform: PlatformGemini, Concurrency: 10}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	for i := 1; i <= settings.GeminiAdaptiveSchedulerModelFailureThreshold; i++ {
		store.report(GeminiAdaptiveScheduleReport{
			Account:            account,
			RequestID:          "failure-" + strconv.Itoa(i),
			RequestedModel:     "gemini-3.1-flash-lite",
			ModelSample:        true,
			ModelCircuitSample: true,
			TerminalReason:     "upstream_5xx",
		}, now.Add(time.Duration(i)*time.Second), settings)
	}

	modelKey := geminiAdaptiveCanonicalModel(account, "gemini-3.1-flash-lite", "", "generateContent")
	opened := store.snapshot(account, settings).ModelCircuits[modelKey]
	require.Equal(t, settings.GeminiAdaptiveSchedulerModelFailureThreshold, opened.ConsecutiveFailure)
	require.Equal(t, now.Add(3*time.Second+time.Minute), opened.OpenUntil)
	require.False(t, store.circuitEligibility(account, "gemini-3.1-flash-lite", "generateContent", now.Add(30*time.Second), settings).Allowed)

	probeAt := opened.OpenUntil.Add(time.Second)
	allowed, claimed := store.claimCircuitProbe(account, "gemini-3.1-flash-lite", "generateContent", "probe-1", probeAt, settings)
	require.True(t, allowed)
	require.True(t, claimed)
	allowed, claimed = store.claimCircuitProbe(account, "gemini-3.1-flash-lite", "generateContent", "probe-2", probeAt, settings)
	require.False(t, allowed)
	require.False(t, claimed)

	store.report(GeminiAdaptiveScheduleReport{
		Account:              account,
		RequestID:            "probe-1",
		RequestedModel:       "gemini-3.1-flash-lite",
		Success:              true,
		PathSample:           true,
		ModelSample:          true,
		AccountCircuitSample: true,
		ModelCircuitSample:   true,
		TerminalReason:       "success",
	}, probeAt.Add(time.Second), settings)

	closed := store.snapshot(account, settings).ModelCircuits[modelKey]
	require.Zero(t, closed.ConsecutiveFailure)
	require.True(t, closed.OpenUntil.IsZero())
	require.True(t, store.circuitEligibility(account, "gemini-3.1-flash-lite", "generateContent", probeAt.Add(time.Second), settings).Allowed)
}

func TestGeminiAdaptiveCircuitDeduplicatesSameRequestRetryBurst(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{ID: 82, Platform: PlatformGemini, Concurrency: 10}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	report := GeminiAdaptiveScheduleReport{
		Account:            account,
		RequestID:          "same-request",
		RequestedModel:     "gemini-3.1-flash-lite",
		ModelSample:        true,
		ModelCircuitSample: true,
		TerminalReason:     "upstream_5xx",
	}

	for i := 0; i < 3; i++ {
		store.report(report, now.Add(time.Duration(i)*time.Second), settings)
	}

	modelKey := geminiAdaptiveCanonicalModel(account, report.RequestedModel, "", "generateContent")
	state := store.snapshot(account, settings)
	circuit := state.ModelCircuits[modelKey]
	require.Equal(t, 1, circuit.ConsecutiveFailure)
	require.True(t, circuit.OpenUntil.IsZero())
	require.Equal(t, int64(1), state.TotalSamples)
	require.Equal(t, int64(1), state.ByModelFamily["flash"].Samples)
	require.Equal(t, int64(1), state.ByModelFamily["flash"].Failures)
}

func TestGeminiAdaptiveBuildOrderHardRejectsOpenCircuit(t *testing.T) {
	scheduler := newGeminiAdaptiveScheduler()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = GeminiAdaptiveSchedulerModeEnforce
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	failed := &Account{ID: 91, Platform: PlatformGemini, Priority: 1, Concurrency: 10}
	healthy := &Account{ID: 92, Platform: PlatformGemini, Priority: 1, Concurrency: 10}
	model := "gemini-3.1-flash-lite"
	modelKey := geminiAdaptiveCanonicalModel(failed, model, "", "generateContent")
	scheduler.state.mu.Lock()
	state := scheduler.state.ensureLocked(failed, now, settings)
	state.ModelCircuits[modelKey] = geminiAdaptiveCircuitState{
		ConsecutiveFailure: settings.GeminiAdaptiveSchedulerModelFailureThreshold,
		OpenUntil:          now.Add(time.Minute),
	}
	scheduler.state.mu.Unlock()

	decision, err := scheduler.BuildOrder(GeminiAdaptiveScheduleRequest{
		RequestedModel: model,
		Action:         "generateContent",
		Candidates: []GeminiAdaptiveCandidateInput{
			{Account: failed},
			{Account: healthy},
		},
		BaselineOrder: []int64{failed.ID, healthy.ID},
		Settings:      &settings,
	})

	require.NoError(t, err)
	require.Equal(t, 1, decision.CircuitRejectedCount)
	require.Equal(t, 1, decision.ModelCircuitRejectedCount)
	require.Equal(t, []int64{healthy.ID}, geminiAdaptiveCandidateIDs(decision.Order))
}

func TestGeminiAdaptiveAuthFailureImmediatelyOpensAccountCircuit(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{ID: 83, Platform: PlatformGemini, Concurrency: 10}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	report := classifyGeminiAdaptiveResult(
		context.WithValue(context.Background(), ctxkey.RequestID, "auth-request"),
		account,
		"gemini-3.1-flash-lite",
		"generateContent",
		nil,
		&UpstreamFailoverError{StatusCode: http.StatusUnauthorized, Scope: GatewayFailureScopeAccount},
	)

	store.report(report, now, settings)

	state := store.snapshot(account, settings)
	require.Equal(t, 1, state.AccountCircuit.ConsecutiveFailure)
	require.Equal(t, now.Add(time.Minute), state.AccountCircuit.OpenUntil)
	require.False(t, store.circuitEligibility(account, report.RequestedModel, report.Action, now.Add(time.Second), settings).Allowed)
}

func TestGeminiAdaptiveModelCircuitUsesCanonicalMappedModel(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{
		ID:          84,
		Platform:    PlatformGemini,
		Concurrency: 10,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"public-flash": "upstream-flash",
			"public-pro":   "upstream-pro",
		}},
	}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	for i := 0; i < settings.GeminiAdaptiveSchedulerModelFailureThreshold; i++ {
		store.report(GeminiAdaptiveScheduleReport{
			Account:            account,
			RequestID:          "mapped-failure-" + strconv.Itoa(i),
			RequestedModel:     "public-flash",
			ModelCircuitSample: true,
			TerminalReason:     "upstream_5xx",
		}, now.Add(time.Duration(i)*time.Second), settings)
	}

	require.False(t, store.circuitEligibility(account, "public-flash", "generateContent", now.Add(10*time.Second), settings).Allowed)
	require.True(t, store.circuitEligibility(account, "public-pro", "generateContent", now.Add(10*time.Second), settings).Allowed)
}

func TestGeminiAdaptiveUnobservedProbeReleasesLease(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{ID: 85, Platform: PlatformGemini, Concurrency: 10}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	model := "gemini-3.1-flash-lite"
	modelKey := geminiAdaptiveCanonicalModel(account, model, "", "generateContent")
	store.mu.Lock()
	state := store.ensureLocked(account, now, settings)
	state.ModelCircuits[modelKey] = geminiAdaptiveCircuitState{
		ConsecutiveFailure: settings.GeminiAdaptiveSchedulerModelFailureThreshold,
		OpenUntil:          now.Add(-time.Second),
	}
	store.mu.Unlock()

	allowed, claimed := store.claimCircuitProbe(account, model, "generateContent", "cancelled-probe", now, settings)
	require.True(t, allowed)
	require.True(t, claimed)
	store.report(GeminiAdaptiveScheduleReport{
		Account:        account,
		RequestID:      "cancelled-probe",
		RequestedModel: model,
		TerminalReason: "client_cancelled",
	}, now.Add(time.Second), settings)

	eligibility := store.circuitEligibility(account, model, "generateContent", now.Add(time.Second), settings)
	require.True(t, eligibility.Allowed)
	require.True(t, eligibility.HalfOpen)
}

func TestGeminiAdaptiveLegacyAcquireReturnsProbeRelease(t *testing.T) {
	service := &GatewayService{}
	account := &Account{ID: 851, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1}
	released := false

	selection, acquired, releaseProbe, err := service.tryAcquireByLegacyOrderWithGate(
		context.Background(),
		[]*Account{account},
		nil,
		"",
		false,
		false,
		nil,
		func(*Account) (bool, func()) {
			return true, func() { released = true }
		},
	)

	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, selection)
	require.False(t, released, "a selected half-open probe must remain claimed until the request is observed")
	releaseProbe()
	require.True(t, released)
}

func TestGeminiAdaptiveProbeReleasesWhenRequestContextEnds(t *testing.T) {
	scheduler := newGeminiAdaptiveScheduler()
	service := &GatewayService{geminiAdaptiveScheduler: scheduler}
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = GeminiAdaptiveSchedulerModeEnforce
	account := &Account{ID: 852, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	model := "gemini-3.1-flash-lite"
	modelKey := geminiAdaptiveCanonicalModel(account, model, "", "generateContent")
	scheduler.now = func() time.Time { return now }
	scheduler.state.mu.Lock()
	state := scheduler.state.ensureLocked(account, now, settings)
	state.ModelCircuits[modelKey] = geminiAdaptiveCircuitState{
		ConsecutiveFailure: settings.GeminiAdaptiveSchedulerModelFailureThreshold,
		OpenUntil:          now.Add(-time.Second),
	}
	scheduler.state.mu.Unlock()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.RequestID, "context-probe"))

	allowed, _ := service.claimGeminiAdaptiveCircuitProbe(ctx, GeminiAdaptiveSchedulerModeEnforce, settings, account, model)
	require.True(t, allowed)
	require.False(t, scheduler.state.circuitEligibility(account, model, "generateContent", now, settings).Allowed)

	cancel()
	require.Eventually(t, func() bool {
		eligibility := scheduler.state.circuitEligibility(account, model, "generateContent", now, settings)
		return eligibility.Allowed && eligibility.HalfOpen
	}, time.Second, 10*time.Millisecond)
}

func TestGeminiAdaptiveFailedProbeExtendsCooldownExponentially(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	account := &Account{ID: 86, Platform: PlatformGemini, Concurrency: 10}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	model := "gemini-3.1-flash-lite"
	modelKey := geminiAdaptiveCanonicalModel(account, model, "", "generateContent")
	store.mu.Lock()
	state := store.ensureLocked(account, now, settings)
	state.ModelCircuits[modelKey] = geminiAdaptiveCircuitState{
		ConsecutiveFailure: settings.GeminiAdaptiveSchedulerModelFailureThreshold,
		OpenUntil:          now.Add(-time.Second),
	}
	store.mu.Unlock()

	allowed, claimed := store.claimCircuitProbe(account, model, "generateContent", "failed-probe", now, settings)
	require.True(t, allowed)
	require.True(t, claimed)
	store.report(GeminiAdaptiveScheduleReport{
		Account:            account,
		RequestID:          "failed-probe",
		RequestedModel:     model,
		ModelCircuitSample: true,
		TerminalReason:     "upstream_5xx",
	}, now, settings)

	circuit := store.snapshot(account, settings).ModelCircuits[modelKey]
	require.Equal(t, settings.GeminiAdaptiveSchedulerModelFailureThreshold+1, circuit.ConsecutiveFailure)
	require.Equal(t, now.Add(2*time.Minute), circuit.OpenUntil)
}

func TestGeminiPoolModeLimitsSameAccountRetries(t *testing.T) {
	account := &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode":                    true,
			"pool_mode_retry_count":        float64(5),
			"pool_mode_retry_status_codes": []any{float64(401), float64(403), float64(429), float64(503)},
		},
	}

	require.Equal(t, 1, account.GetPoolModeRetryCount())
	require.False(t, account.IsPoolModeRetryableStatus(http.StatusUnauthorized))
	require.False(t, account.IsPoolModeRetryableStatus(http.StatusForbidden))
	require.True(t, account.IsPoolModeRetryableStatus(http.StatusTooManyRequests))
	require.True(t, account.IsPoolModeRetryableStatus(http.StatusServiceUnavailable))
}

func geminiAdaptiveCandidateIDs(candidates []GeminiAdaptiveCandidate) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.Account.ID)
	}
	return ids
}
