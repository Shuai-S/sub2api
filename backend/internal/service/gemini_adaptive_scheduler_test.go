package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGeminiAdaptiveSchedulerDefaultsDisabled(t *testing.T) {
	settings := DefaultGeminiAdaptiveSchedulerSettings()

	require.False(t, settings.GeminiAdaptiveSchedulerEnabled)
	require.Equal(t, GeminiAdaptiveSchedulerModeShadow, settings.GeminiAdaptiveSchedulerMode)
	require.Equal(t, 8, settings.GeminiAdaptiveSchedulerTopK)
	require.Equal(t, 0.35, settings.GeminiAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, GeminiAdaptiveSchedulerModeShadow, normalizeGeminiAdaptiveSchedulerMode("invalid"))
}

func TestGeminiAdaptiveScoresIgnoreQuotaRemaining(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	candidates := []GeminiAdaptiveCandidate{
		{
			Account:           &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10},
			Load:              &AccountLoadInfo{AccountID: 1, CurrentConcurrency: 1},
			EffectiveCapacity: 10,
			Quota:             GeminiAdaptiveQuotaSnapshot{DailyUsed: 1, DailyLimit: 100, DataAvailable: true},
			coreState:         *newAdaptiveAccountState(1, 10, now),
		},
		{
			Account:           &Account{ID: 2, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10},
			Load:              &AccountLoadInfo{AccountID: 2, CurrentConcurrency: 1},
			EffectiveCapacity: 10,
			Quota:             GeminiAdaptiveQuotaSnapshot{DailyUsed: 99, DailyLimit: 100, DataAvailable: true},
			coreState:         *newAdaptiveAccountState(2, 10, now),
		},
	}

	applyGeminiAdaptiveScores(candidates, "gemini-2.5-pro", "generateContent", false, now, settings)

	require.InDelta(t, candidates[0].Score, candidates[1].Score, 1e-12)
}

func TestGeminiAdaptiveMixedPoolReordersNativeSlotsByScoreAndIgnoresPriority(t *testing.T) {
	scheduler := newGeminiAdaptiveScheduler()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = GeminiAdaptiveSchedulerModeEnforce
	settings.GeminiAdaptiveSchedulerTopK = 1
	settings.GeminiAdaptiveSchedulerExplorationRate = 0
	settings.GeminiAdaptiveSchedulerWeightReliability = 1
	settings.GeminiAdaptiveSchedulerWeightCapacity = 0
	settings.GeminiAdaptiveSchedulerWeightLatency = 0
	settings.GeminiAdaptiveSchedulerWeightCost = 0
	nativeLow := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 1, Concurrency: 5}
	other := &Account{ID: 2, Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Priority: 1, Concurrency: 5}
	nativeHigh := &Account{ID: 3, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 99, Concurrency: 5}

	scheduler.core.mu.Lock()
	low := scheduler.core.ensureLocked(nativeLow.ID, nativeLow.Concurrency, now)
	low.SuccessEMA = 0.1
	high := scheduler.core.ensureLocked(nativeHigh.ID, nativeHigh.Concurrency, now)
	high.SuccessEMA = 0.9
	for i := 0; i < settings.GeminiAdaptiveSchedulerLearningMinHealthSamples; i++ {
		at := now.Add(-time.Duration(i) * time.Second)
		low.HealthObservations = append(low.HealthObservations, adaptiveHealthObservation{At: at, Success: false})
		high.HealthObservations = append(high.HealthObservations, adaptiveHealthObservation{At: at, Success: true})
	}
	scheduler.core.mu.Unlock()

	decision, err := scheduler.BuildOrder(GeminiAdaptiveScheduleRequest{
		RequestedModel: "gemini-2.5-pro",
		Candidates: []GeminiAdaptiveCandidateInput{
			{Account: nativeLow},
			{Account: other},
			{Account: nativeHigh},
		},
		BaselineOrder: []int64{nativeLow.ID, other.ID, nativeHigh.ID},
		Settings:      &settings,
		NewSession:    false,
	})

	require.NoError(t, err)
	require.Equal(t, 2, decision.CandidateCount)
	require.Equal(t, 1, decision.TopK)
	require.Equal(t, []int64{nativeHigh.ID, other.ID, nativeLow.ID}, geminiAdaptiveCandidateIDs(decision.Order))
}

func TestGeminiAdaptiveBuildOrderRejectsQuotaAndOpenAccountCircuit(t *testing.T) {
	scheduler := newGeminiAdaptiveScheduler()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = GeminiAdaptiveSchedulerModeEnforce
	open := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10}
	quotaRejected := &Account{ID: 2, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10}
	healthy := &Account{ID: 3, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10}
	scheduler.core.mu.Lock()
	scheduler.core.ensureLocked(open.ID, open.Concurrency, now).CircuitOpenUntil = now.Add(time.Minute)
	scheduler.core.mu.Unlock()

	decision, err := scheduler.BuildOrder(GeminiAdaptiveScheduleRequest{
		Candidates: []GeminiAdaptiveCandidateInput{
			{Account: open},
			{Account: quotaRejected, Quota: GeminiAdaptiveQuotaSnapshot{HardRejected: true}},
			{Account: healthy},
		},
		BaselineOrder: []int64{open.ID, quotaRejected.ID, healthy.ID},
		Settings:      &settings,
	})

	require.NoError(t, err)
	require.Equal(t, 1, decision.HardRejectedCount)
	require.Equal(t, 1, decision.CircuitRejectedCount)
	require.Equal(t, []int64{healthy.ID}, geminiAdaptiveCandidateIDs(decision.Order))
}

func TestClassifyGeminiAdaptiveResultMapsExistingSignalsToCoreObservations(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10}
	tests := []struct {
		name               string
		err                error
		wantReason         string
		wantObservation    adaptiveObservationType
		wantAuthentication bool
	}{
		{name: "generic rate limit", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount}, wantReason: "generic_rate_limit", wantObservation: adaptiveObservationProviderOverload},
		{name: "quota rate limit", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount, ResponseBody: []byte(`{"error":{"message":"requests per minute quota exceeded"}}`)}, wantReason: "quota_rate_limit", wantObservation: adaptiveObservationQuotaLimit},
		{name: "provider capacity", err: &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, Scope: GatewayFailureScopeAccount, ResponseBody: []byte(`{"error":{"reason":"MODEL_CAPACITY_EXHAUSTED"}}`)}, wantReason: "provider_capacity", wantObservation: adaptiveObservationProviderOverload},
		{name: "local queue", err: errors.New("timeout waiting for account concurrency slot"), wantReason: "local_queue", wantObservation: adaptiveObservationIgnored},
		{name: "account concurrency", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount, ResponseBody: []byte(`{"error":{"message":"Concurrency limit exceeded for account"}}`)}, wantReason: "concurrency_limit", wantObservation: adaptiveObservationCapacityLimit},
		{name: "authentication", err: &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, Scope: GatewayFailureScopeAccount}, wantReason: "account_auth", wantObservation: adaptiveObservationAccountFailure, wantAuthentication: true},
		{name: "health override excluded", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway, Scope: GatewayFailureScopeAccount, HealthSample: boolPtr(false)}, wantReason: "upstream_5xx", wantObservation: adaptiveObservationProviderOverload},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := classifyGeminiAdaptiveResult(context.Background(), account, "gemini-2.5-pro", "generateContent", nil, tt.err)
			observation, authentication := geminiAdaptiveObservation(report)
			require.Equal(t, tt.wantReason, report.TerminalReason)
			require.Equal(t, tt.wantObservation, observation)
			require.Equal(t, tt.wantAuthentication, authentication)
		})
	}
}

func TestGeminiAdaptiveProbeReleasesWhenRequestContextEnds(t *testing.T) {
	scheduler := newGeminiAdaptiveScheduler()
	service := &GatewayService{geminiAdaptiveScheduler: scheduler}
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = GeminiAdaptiveSchedulerModeEnforce
	account := &Account{ID: 852, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1}
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	scheduler.core.mu.Lock()
	state := scheduler.core.ensureLocked(account.ID, account.Concurrency, now)
	state.CircuitOpenUntil = now.Add(-time.Second)
	state.CircuitOpenCount = 1
	scheduler.core.mu.Unlock()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.RequestID, "context-probe"))

	allowed, _ := service.claimGeminiAdaptiveCircuitProbe(ctx, GeminiAdaptiveSchedulerModeEnforce, settings, account, "gemini-2.5-pro")
	require.True(t, allowed)
	require.False(t, scheduler.core.allowedForSelection(account.ID, account.Concurrency, now, geminiAdaptiveCoreSettings(settings)))

	cancel()
	require.Eventually(t, func() bool {
		return scheduler.core.allowedForSelection(account.ID, account.Concurrency, now, geminiAdaptiveCoreSettings(settings))
	}, time.Second, 10*time.Millisecond)
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
	require.False(t, released)
	releaseProbe()
	require.True(t, released)
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
