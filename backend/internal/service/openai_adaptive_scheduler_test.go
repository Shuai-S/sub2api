package service

import (
	"testing"

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

func TestEffectiveOpenAIAdaptiveCapacityCapsConfiguredConcurrency(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	state := defaultOpenAIAdaptiveAccountState(1, cfg)
	state.EstimatedCapacity = 10000

	require.Equal(t, 300, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 300}, state, cfg))
	require.Equal(t, 10000, effectiveOpenAIAdaptiveCapacity(&Account{Concurrency: 0}, state, cfg))
}

func TestOpenAIAdaptiveSchedulerAIMDDecreasesCapacityOnFailures(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerInitialCapacity = 100
	cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold = 2
	cfg.OpenAIAdaptiveSchedulerCapacityDecreaseFactor = 0.5
	cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds = 1
	store := newOpenAIAdaptiveSchedulerStateStore()

	store.report(1, cfg, false, nil, 0)
	require.Equal(t, 100, store.snapshot(1, cfg).EstimatedCapacity)

	store.report(1, cfg, false, nil, 0)
	state := store.snapshot(1, cfg)
	require.Equal(t, 50, state.EstimatedCapacity)
	require.True(t, state.CooldownUntil.After(state.LastCapacityFailureAt))
}
