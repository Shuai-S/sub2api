package service

import (
	"testing"
	"time"

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
