package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdaptiveCacheStatsAggregateTokensBeforeCalculatingHitRate(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	state := newAdaptiveAccountState(1, 10, now)
	observeAdaptiveCacheLocked(state, 70, 20, 10, now.Add(-time.Minute), settings)
	observeAdaptiveCacheLocked(state, 10, 10, 80, now, settings)

	stats := adaptiveCacheStatsForState(*state, now, settings)

	require.Equal(t, int64(80), stats.InputTokens)
	require.Equal(t, int64(30), stats.CacheCreationTokens)
	require.Equal(t, int64(90), stats.CacheReadTokens)
	require.Equal(t, int64(2), stats.Samples)
	require.InDelta(t, 0.45, stats.HitRate, 1e-9)
}

func TestAdaptiveCacheStatsExpireWithLearningWindow(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 30, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.LearningWindow = 20 * time.Minute
	state := newAdaptiveAccountState(1, 10, now)
	observeAdaptiveCacheLocked(state, 0, 0, 100, now.Add(-21*time.Minute), settings)
	observeAdaptiveCacheLocked(state, 80, 0, 20, now.Add(-19*time.Minute), settings)

	require.True(t, pruneAdaptiveCacheBuckets(state, now, settings))
	stats := adaptiveCacheStatsForState(*state, now, settings)
	require.Equal(t, int64(1), stats.Samples)
	require.InDelta(t, 0.2, stats.HitRate, 1e-9)
}

func TestAdaptiveCacheLearningAttributesOnlyFinalSuccessfulAccount(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.observe(adaptiveObservation{
		AccountID: 1, RequestID: "retry", Type: adaptiveObservationAccountFailure,
		ConfiguredCapacity: 10, CacheInputTokens: 10, CacheReadTokens: 90,
	}, now, settings)
	store.observe(adaptiveObservation{
		AccountID: 2, RequestID: "retry", Type: adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10, CacheInputTokens: 80, CacheReadTokens: 20,
	}, now.Add(time.Second), settings)

	failed := store.snapshot(1, 10, now.Add(time.Second), settings)
	succeeded := store.snapshot(2, 10, now.Add(time.Second), settings)
	require.Zero(t, adaptiveCacheStatsForState(failed, now.Add(time.Second), settings).Samples)
	require.Equal(t, int64(1), adaptiveCacheStatsForState(succeeded, now.Add(time.Second), settings).Samples)
}

func TestAdaptiveCacheScoreOnlyRewardsMatureAccountsAboveLayerMedian(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.LearningMinHealthSamples = 2
	settings.WeightReliability = 1
	settings.WeightCapacity = 0
	settings.WeightTTFT = 0
	settings.WeightCost = 0
	settings.WeightCache = 1

	stateWithRate := func(accountID int64, input, read int64, samples int) adaptiveAccountState {
		state := newAdaptiveAccountState(accountID, 10, now)
		for i := 0; i < samples; i++ {
			observeAdaptiveCacheLocked(state, input, 0, read, now, settings)
		}
		return *state
	}
	candidates := []adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: stateWithRate(1, 80, 20, 2)},
		{AccountID: 2, Cost: 1, State: stateWithRate(2, 50, 50, 2)},
		{AccountID: 3, Cost: 1, State: stateWithRate(3, 20, 80, 2)},
		{AccountID: 4, Cost: 1, State: stateWithRate(4, 10, 90, 1)},
	}

	scored := scoreAdaptiveCandidates(candidates, now, settings)

	require.InDelta(t, 0.5, scored[0].Score, 1e-9, "below-median accounts must not be penalized")
	require.InDelta(t, 0.5, scored[1].Score, 1e-9, "median accounts must remain neutral")
	require.Greater(t, scored[2].Score, 0.5, "above-median mature accounts should receive a bonus")
	require.InDelta(t, 0.5, scored[3].Score, 1e-9, "sparse accounts use the pool median and remain neutral")
}

func TestAdaptiveCacheScoreIsNeutralWithoutMatureCandidates(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.LearningMinHealthSamples = 2
	settings.WeightReliability = 1
	settings.WeightCapacity = 0
	settings.WeightTTFT = 0
	settings.WeightCost = 0
	settings.WeightCache = 1
	state := newAdaptiveAccountState(1, 10, now)
	observeAdaptiveCacheLocked(state, 0, 0, 100, now, settings)

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{{AccountID: 1, Cost: 1, State: *state}}, now, settings)

	require.InDelta(t, 0.5, scored[0].Score, 1e-9)
}

func TestAdaptiveCacheWeightZeroPreservesExistingScore(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.WeightCache = 0
	state := newAdaptiveAccountState(1, 10, now)
	for i := 0; i < settings.LearningMinHealthSamples; i++ {
		observeAdaptiveCacheLocked(state, 0, 0, 100, now, settings)
	}

	withoutCache := *state
	withoutCache.CacheBuckets = nil
	withScore := scoreAdaptiveCandidates([]adaptiveScoreCandidate{{AccountID: 1, Cost: 1, State: *state}}, now, settings)
	withoutScore := scoreAdaptiveCandidates([]adaptiveScoreCandidate{{AccountID: 1, Cost: 1, State: withoutCache}}, now, settings)
	require.InDelta(t, withoutScore[0].Score, withScore[0].Score, 1e-9)
}

func TestAdaptiveV2StateJSONCacheFieldCompatibility(t *testing.T) {
	type oldAdaptiveAccountState struct {
		Version            int     `json:"version"`
		AccountID          int64   `json:"account_id"`
		ConfiguredCapacity int     `json:"configured_capacity"`
		EffectiveCapacity  int     `json:"effective_capacity"`
		SuccessEMA         float64 `json:"success_ema"`
	}

	oldPayload, err := json.Marshal(oldAdaptiveAccountState{
		Version: adaptiveSchedulerStateVersion, AccountID: 7,
		ConfiguredCapacity: 10, EffectiveCapacity: 8, SuccessEMA: 0.9,
	})
	require.NoError(t, err)
	var restored adaptiveAccountState
	require.NoError(t, json.Unmarshal(oldPayload, &restored))
	require.Empty(t, restored.CacheBuckets)
	require.Equal(t, adaptiveSchedulerStateVersion, restored.Version)
	require.Equal(t, int64(7), restored.AccountID)

	restored.CacheBuckets = []adaptiveCacheBucket{{
		BucketStart: time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC),
		InputTokens: 20, CacheReadTokens: 80, Samples: 1,
	}}
	newPayload, err := json.Marshal(restored)
	require.NoError(t, err)
	var oldView oldAdaptiveAccountState
	require.NoError(t, json.Unmarshal(newPayload, &oldView))
	require.Equal(t, adaptiveSchedulerStateVersion, oldView.Version)
	require.Equal(t, int64(7), oldView.AccountID)
	require.Equal(t, 8, oldView.EffectiveCapacity)

	restored.CacheBuckets = nil
	emptyPayload, err := json.Marshal(restored)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(emptyPayload), "cache_buckets"))
}
