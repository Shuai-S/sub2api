package service

import (
	"context"
	"errors"
	"net/http"
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

func geminiAdaptiveCandidateIDs(candidates []GeminiAdaptiveCandidate) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.Account.ID)
	}
	return ids
}
