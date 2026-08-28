package service

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdaptiveScoreUsesDynamicCandidateLayerCost(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 0
	settings.WeightTTFT = 0
	settings.WeightCost = 1
	candidates := []adaptiveScoreCandidate{
		{AccountID: 1, Cost: 0.15, State: *newAdaptiveAccountState(1, 10, now)},
		{AccountID: 2, Cost: 0.25, State: *newAdaptiveAccountState(2, 10, now)},
	}

	scored := scoreAdaptiveCandidates(candidates, now, settings)

	require.InDelta(t, 1.0, scored[0].CostScore, 1e-9)
	require.InDelta(t, 0.6, scored[1].CostScore, 1e-9)
	require.InDelta(t, 1.0, scored[0].Score, 1e-9)
	require.InDelta(t, 0.6, scored[1].Score, 1e-9)
}

func TestAdaptiveScoreCalculatesCostWithinPriorityLayer(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 0
	settings.WeightTTFT = 0
	settings.WeightCost = 1
	candidates := []adaptiveScoreCandidate{
		{AccountID: 1, OAuth: true, Cost: 0.05, State: *newAdaptiveAccountState(1, 10, now)},
		{AccountID: 2, Cost: 0.20, State: *newAdaptiveAccountState(2, 10, now)},
		{AccountID: 3, Cost: 0.40, State: *newAdaptiveAccountState(3, 10, now)},
	}

	scored := scoreAdaptiveCandidates(candidates, now, settings)

	require.InDelta(t, 1.0, scored[0].CostScore, 1e-9)
	require.InDelta(t, 1.0, scored[1].CostScore, 1e-9)
	require.InDelta(t, 0.5, scored[2].CostScore, 1e-9)
}

func TestAdaptiveOrderKeepsOAuthAsTheOnlyHardPriorityLayer(t *testing.T) {
	settings := defaultAdaptiveCoreSettings()
	settings.TopK = 1
	candidates := []adaptiveScoreCandidate{
		{AccountID: 1, Score: 1},
		{AccountID: 2, OAuth: true, Score: 0.1},
		{AccountID: 3, OAuth: true, Score: 0.2},
	}

	ordered := orderAdaptiveCandidates(candidates, false, false, time.Now(), settings)

	require.Len(t, ordered, 3)
	require.Equal(t, int64(3), ordered[0].AccountID)
	require.Equal(t, int64(2), ordered[1].AccountID)
	require.Equal(t, int64(1), ordered[2].AccountID)
}

func TestAdaptiveScoreRedistributesTTFTWeightWithoutComparableSamples(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	state := newAdaptiveAccountState(1, 10, now)
	state.TTFTEMA = 100
	state.TTFTSamples = 1

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{{
		AccountID: 1,
		Cost:      1,
		State:     *state,
	}}, now, settings)

	require.Len(t, scored, 1)
	require.Zero(t, scored[0].TTFTScore)
	expected := (0.50*0.5 + 0.20 + 0.15) / (0.50 + 0.20 + 0.15)
	require.InDelta(t, expected, scored[0].Score, 1e-9)
}

func TestAdaptiveScoreRedistributesTTFTWeightWithOnlyOneValidCandidate(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 1
	settings.WeightCost = 0
	settings.WeightTTFT = 1
	withTTFT := newAdaptiveAccountState(1, 10, now)
	withTTFT.TTFTEMA = 100
	withTTFT.TTFTSamples = 10
	withoutTTFT := newAdaptiveAccountState(2, 10, now)

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: *withTTFT},
		{AccountID: 2, Cost: 1, State: *withoutTTFT},
	}, now, settings)

	require.Zero(t, scored[0].TTFTScore)
	require.Zero(t, scored[1].TTFTScore)
	require.InDelta(t, 1.0, scored[0].Score, 1e-9)
	require.InDelta(t, 1.0, scored[1].Score, 1e-9)
}

func TestAdaptiveScoreRedistributesTTFTWeightWithoutCandidateSpread(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 1
	settings.WeightCost = 0
	settings.WeightTTFT = 1
	first := newAdaptiveAccountState(1, 10, now)
	first.TTFTEMA = 200
	first.TTFTSamples = 10
	second := newAdaptiveAccountState(2, 10, now)
	second.TTFTEMA = 200
	second.TTFTSamples = 20

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: *first},
		{AccountID: 2, Cost: 1, State: *second},
	}, now, settings)

	require.Zero(t, scored[0].TTFTScore)
	require.Zero(t, scored[1].TTFTScore)
	require.InDelta(t, 1.0, scored[0].Score, 1e-9)
	require.InDelta(t, 1.0, scored[1].Score, 1e-9)
}

func TestAdaptiveScoreUsesNeutralTTFTForMissingSamplesAndConfidenceForSparseSamples(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 0
	settings.WeightCost = 0
	settings.WeightTTFT = 1
	fastSparse := newAdaptiveAccountState(1, 10, now)
	fastSparse.TTFTEMA = 100
	fastSparse.TTFTSamples = 1
	slowLearned := newAdaptiveAccountState(2, 10, now)
	slowLearned.TTFTEMA = 300
	slowLearned.TTFTSamples = int64(settings.LearningMinHealthSamples)
	missing := newAdaptiveAccountState(3, 10, now)

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: *fastSparse},
		{AccountID: 2, Cost: 1, State: *slowLearned},
		{AccountID: 3, Cost: 1, State: *missing},
	}, now, settings)

	expectedSparseScore := 0.5 + 0.5/float64(settings.LearningMinHealthSamples)
	require.InDelta(t, expectedSparseScore, scored[0].TTFTScore, 1e-9)
	require.Zero(t, scored[1].TTFTScore)
	require.InDelta(t, 0.5, scored[2].TTFTScore, 1e-9)
	require.InDelta(t, expectedSparseScore, scored[0].Score, 1e-9)
	require.Zero(t, scored[1].Score)
	require.InDelta(t, 0.5, scored[2].Score, 1e-9)
}

func TestAdaptiveTTFTWindowUsesOnlyLearningWindowSamples(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.LearningWindow = 20 * time.Minute
	store := newAdaptiveStateStore()
	bucketKey := "gpt-5.1|responses|http"

	oldValue := 60000
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "old",
		Type:               adaptiveObservationHealthSuccess,
		FirstTokenMs:       &oldValue,
		TTFTBucketKey:      bucketKey,
		WindowedTTFT:       true,
		ConfiguredCapacity: 10,
	}, now.Add(-21*time.Minute), settings)
	newValue := 4000
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "new",
		Type:               adaptiveObservationHealthSuccess,
		FirstTokenMs:       &newValue,
		TTFTBucketKey:      bucketKey,
		WindowedTTFT:       true,
		ConfiguredCapacity: 10,
	}, now, settings)

	state := store.snapshot(1, 10, now, settings)
	stats := adaptiveTTFTWindowStatsForState(state, bucketKey, now, settings)
	require.Equal(t, 1, stats.Samples)
	require.Equal(t, 4000.0, stats.P50)
	require.Equal(t, int64(1), state.TTFTSamples)
	require.InDelta(t, 4000.0, state.TTFTEMA, 1e-9)
}

func TestAdaptiveTTFTWindowCapsSamplesAndBuckets(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	state := newAdaptiveAccountState(1, 10, now)
	primaryBucket := "gpt-5.1|responses|http"
	for i := 0; i < adaptiveTTFTMaxSamplesPerBucket+25; i++ {
		observeAdaptiveTTFTWindowLocked(state, primaryBucket, i+1, now.Add(time.Duration(i)*time.Millisecond), settings)
	}

	require.Len(t, state.TTFTWindows[primaryBucket], adaptiveTTFTMaxSamplesPerBucket)
	require.Equal(t, 26, state.TTFTWindows[primaryBucket][0].Milliseconds)
	require.Equal(t, int64(adaptiveTTFTMaxSamplesPerBucket), state.TTFTSamples)

	for i := 0; i < adaptiveTTFTMaxBucketsPerAccount; i++ {
		bucket := "model-" + string(rune('a'+i)) + "|responses|http"
		observeAdaptiveTTFTWindowLocked(state, bucket, 100+i, now.Add(time.Minute+time.Duration(i)*time.Millisecond), settings)
	}

	require.Len(t, state.TTFTWindows, adaptiveTTFTMaxBucketsPerAccount)
	require.NotContains(t, state.TTFTWindows, primaryBucket)
}

func TestAdaptiveSchedulingSnapshotCarriesOnlyRequestedTTFTStats(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	requestedBucket := "gpt-5.1|responses|http"
	otherBucket := "gpt-5.1|responses|ws"
	store.mu.Lock()
	state := store.ensureLocked(1, 10, now)
	state.TTFTWindows = map[string][]adaptiveTTFTObservation{
		requestedBucket: {{At: now, Milliseconds: 100}},
		otherBucket:     {{At: now, Milliseconds: 200}},
	}
	store.mu.Unlock()

	snapshot, allowed := store.schedulingSnapshot(1, 10, 3, requestedBucket, now, settings)

	require.True(t, allowed)
	require.Equal(t, 3, snapshot.LastObservedConcurrency)
	require.Empty(t, snapshot.TTFTWindows)
	require.Empty(t, snapshot.ttftWindowStats)
	require.Empty(t, snapshot.ttftWindowSortedValues)
	require.Empty(t, snapshot.ttftSortedValues)
	require.Equal(t, requestedBucket, snapshot.schedulingTTFTBucketKey)
	require.Equal(t, adaptiveTTFTWindowStats{Samples: 1, P50: 100, P90: 100},
		adaptiveTTFTWindowStatsForState(snapshot, requestedBucket, now, settings))
	require.Empty(t, adaptiveTTFTWindowStatsForState(snapshot, otherBucket, now, settings))
}

func TestAdaptiveTTFTIncrementalCachesMatchFullRebuild(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	state := newAdaptiveAccountState(1, 10, now)
	buckets := []string{"gpt-5.4|responses|http", "gpt-5.4|responses|ws"}
	for i := 0; i < adaptiveTTFTMaxSamplesPerBucket+25; i++ {
		bucket := buckets[i%len(buckets)]
		observeAdaptiveTTFTWindowLocked(state, bucket, 100+(i*37)%4000, now.Add(time.Duration(i)*time.Millisecond), settings)
	}

	wantEMA := state.TTFTEMA
	wantSamples := state.TTFTSamples
	wantStats := make(map[string]adaptiveTTFTWindowStats, len(buckets))
	for _, bucket := range buckets {
		wantStats[bucket] = state.ttftWindowStats[bucket]
	}

	state.ttftWindowStats = nil
	state.ttftWindowSortedValues = nil
	state.ttftSortedValues = nil
	refreshAdaptiveTTFTWindowSummary(state)

	require.Equal(t, wantSamples, state.TTFTSamples)
	require.InDelta(t, wantEMA, state.TTFTEMA, 1e-9)
	for _, bucket := range buckets {
		require.Equal(t, wantStats[bucket], state.ttftWindowStats[bucket])
	}
}

func TestAdaptiveFullSnapshotOmitsDerivedSortedTTFTCaches(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	bucket := "gpt-5.4|responses|http"
	store.mu.Lock()
	state := store.ensureLocked(1, 10, now)
	observeAdaptiveTTFTWindowLocked(state, bucket, 125, now, settings)
	store.mu.Unlock()

	snapshot := store.snapshot(1, 10, now, settings)

	require.Equal(t, adaptiveTTFTWindowStats{Samples: 1, P50: 125, P90: 125}, snapshot.ttftWindowStats[bucket])
	require.Nil(t, snapshot.ttftWindowSortedValues)
	require.Nil(t, snapshot.ttftSortedValues)
	require.Equal(t, 125.0, adaptiveTTFTWindowStatsForState(snapshot, bucket, now, settings).P50)
}

func TestAdaptiveTTFTPruneSkipsSummaryRebuildWhenNothingChanges(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	state := newAdaptiveAccountState(1, 10, now)
	state.TTFTEMA = 123
	state.TTFTSamples = 7
	state.TTFTWindows = map[string][]adaptiveTTFTObservation{
		"gpt-5.1|responses|http": {{At: now, Milliseconds: 100}},
	}

	require.False(t, pruneAdaptiveTTFTWindows(state, now, settings))
	require.Equal(t, 123.0, state.TTFTEMA)
	require.Equal(t, int64(7), state.TTFTSamples)
}

func TestAdaptiveTTFTAdmissionCarriesBucketToObservation(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	bucketKey := "gpt-5.1|responses|ws"
	store.registerAdmissionWithLoadAndTTFTBucket(1, "request-1", 10, 1, 0, true, bucketKey, now, settings)
	firstToken := 4200

	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "request-1",
		Type:               adaptiveObservationHealthSuccess,
		FirstTokenMs:       &firstToken,
		WindowedTTFT:       true,
		ConfiguredCapacity: 10,
	}, now.Add(time.Second), settings)

	state := store.snapshot(1, 10, now.Add(time.Second), settings)
	stats := adaptiveTTFTWindowStatsForState(state, bucketKey, now.Add(time.Second), settings)
	require.Equal(t, 1, stats.Samples)
	require.Equal(t, 4200.0, stats.P50)
}

func TestAdaptiveAdmissionSuppliesMissingConfiguredCapacity(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.registerAdmission(1, "request-1", 10, now, settings)

	store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "request-1",
		Type:                adaptiveObservationHealthSuccess,
		ConfiguredCapacity:  0,
		ObservedConcurrency: -1,
	}, now.Add(time.Second), settings)

	store.mu.RLock()
	state := cloneAdaptiveAccountState(store.accounts[1])
	store.mu.RUnlock()
	require.Equal(t, 10, state.ConfiguredCapacity)
	require.Equal(t, 10, state.EffectiveCapacity)
}

func TestAdaptiveTTFTAdmissionSurvivesRetryableObservationUntilSuccess(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	bucketKey := "gpt-5.1|responses|ws"
	store.registerAdmissionWithLoadAndTTFTBucket(1, "request-1", 10, 1, 0, true, bucketKey, now, settings)

	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "request-1",
		Type:               adaptiveObservationProviderOverload,
		WindowedTTFT:       true,
		ConfiguredCapacity: 10,
	}, now.Add(time.Second), settings)
	firstToken := 4300
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "request-1",
		Type:               adaptiveObservationHealthSuccess,
		FirstTokenMs:       &firstToken,
		WindowedTTFT:       true,
		ConfiguredCapacity: 10,
	}, now.Add(2*time.Second), settings)

	state := store.snapshot(1, 10, now.Add(2*time.Second), settings)
	stats := adaptiveTTFTWindowStatsForState(state, bucketKey, now.Add(2*time.Second), settings)
	require.Equal(t, 1, stats.Samples)
	require.Equal(t, 4300.0, stats.P50)
	store.mu.RLock()
	require.Empty(t, store.ttftClaims)
	store.mu.RUnlock()
}

func TestAdaptiveTTFTWindowScoresP50WithSmallP90TailPenalty(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 0
	settings.WeightCost = 0
	settings.WeightTTFT = 1
	bucketKey := "gpt-5.1|responses|http"
	fast := newAdaptiveAccountState(1, 10, now)
	slow := newAdaptiveAccountState(2, 10, now)
	for i := 0; i < 50; i++ {
		fastValue := 4000
		if i == 49 {
			fastValue = 120000
		}
		observeAdaptiveTTFTWindowLocked(fast, bucketKey, fastValue, now.Add(time.Duration(i)*time.Second), settings)
		observeAdaptiveTTFTWindowLocked(slow, bucketKey, 8000, now.Add(time.Duration(i)*time.Second), settings)
	}

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: *fast, TTFTBucketKey: bucketKey},
		{AccountID: 2, Cost: 1, State: *slow, TTFTBucketKey: bucketKey},
	}, now.Add(time.Minute), settings)

	require.Equal(t, 50, scored[0].TTFTWindowSamples)
	require.Equal(t, 4000.0, scored[0].TTFTP50)
	require.Equal(t, 4000.0, scored[0].TTFTP90)
	require.Greater(t, scored[0].TTFTScore, scored[1].TTFTScore)
	require.Greater(t, scored[0].Score, scored[1].Score)
}

func TestAdaptiveTTFTWindowKeepsSparseAccountNeutral(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 0
	settings.WeightCost = 0
	settings.WeightTTFT = 1
	bucketKey := "gpt-5.1|responses|http"
	states := []*adaptiveAccountState{
		newAdaptiveAccountState(1, 10, now),
		newAdaptiveAccountState(2, 10, now),
		newAdaptiveAccountState(3, 10, now),
	}
	for i := 0; i < 4; i++ {
		observeAdaptiveTTFTWindowLocked(states[0], bucketKey, 1000, now.Add(time.Duration(i)*time.Second), settings)
	}
	for i := 0; i < 20; i++ {
		observeAdaptiveTTFTWindowLocked(states[1], bucketKey, 4000, now.Add(time.Duration(i)*time.Second), settings)
		observeAdaptiveTTFTWindowLocked(states[2], bucketKey, 8000, now.Add(time.Duration(i)*time.Second), settings)
	}

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: *states[0], TTFTBucketKey: bucketKey},
		{AccountID: 2, Cost: 1, State: *states[1], TTFTBucketKey: bucketKey},
		{AccountID: 3, Cost: 1, State: *states[2], TTFTBucketKey: bucketKey},
	}, now.Add(time.Minute), settings)

	require.Equal(t, 4, scored[0].TTFTWindowSamples)
	require.InDelta(t, 0.5, scored[0].TTFTScore, 1e-9)
	require.Greater(t, scored[1].TTFTScore, scored[2].TTFTScore)
}

func TestAdaptiveTTFTWindowDoesNotCompareDifferentBuckets(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	settings.WeightReliability = 0
	settings.WeightCapacity = 1
	settings.WeightCost = 0
	settings.WeightTTFT = 1
	firstKey := "gpt-5.1|responses|http"
	secondKey := "gpt-5.4-mini|responses|http"
	first := newAdaptiveAccountState(1, 10, now)
	second := newAdaptiveAccountState(2, 10, now)
	for i := 0; i < 20; i++ {
		observeAdaptiveTTFTWindowLocked(first, firstKey, 1000, now.Add(time.Duration(i)*time.Second), settings)
		observeAdaptiveTTFTWindowLocked(second, secondKey, 10000, now.Add(time.Duration(i)*time.Second), settings)
	}

	scored := scoreAdaptiveCandidates([]adaptiveScoreCandidate{
		{AccountID: 1, Cost: 1, State: *first, TTFTBucketKey: firstKey},
		{AccountID: 2, Cost: 1, State: *second, TTFTBucketKey: secondKey},
	}, now.Add(time.Minute), settings)

	require.Zero(t, scored[0].TTFTScore)
	require.Zero(t, scored[1].TTFTScore)
	require.Equal(t, 1.0, scored[0].Score)
	require.Equal(t, 1.0, scored[1].Score)
}

func TestAdaptiveTTFTBlendMatchesP50P90Design(t *testing.T) {
	require.InDelta(t, 5027.83, adaptiveTTFTBlend(4120, 11151), 0.01)
}

func TestAdaptiveCircuitUsesDeduplicatedFailuresAndAuthenticationFastOpen(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	failed := func(requestID string, authentication bool) {
		store.observe(adaptiveObservation{
			AccountID:          1,
			RequestID:          requestID,
			Type:               adaptiveObservationAccountFailure,
			Authentication:     authentication,
			ConfiguredCapacity: 10,
		}, now, settings)
	}

	failed("request-1", false)
	failed("request-1", false)
	failed("request-2", false)
	state := store.snapshot(1, 10, now, settings)
	require.Equal(t, 2, state.ConsecutiveFailures)
	require.True(t, state.CircuitOpenUntil.IsZero())

	failed("request-3", false)
	state = store.snapshot(1, 10, now, settings)
	require.Equal(t, 3, state.ConsecutiveFailures)
	require.Equal(t, 1, state.CircuitOpenCount)
	require.Equal(t, now.Add(time.Minute), state.CircuitOpenUntil)

	authStore := newAdaptiveStateStore()
	authStore.observe(adaptiveObservation{
		AccountID:          2,
		RequestID:          "auth-failure",
		Type:               adaptiveObservationAccountFailure,
		Authentication:     true,
		ConfiguredCapacity: 10,
	}, now, settings)
	authState := authStore.snapshot(2, 10, now, settings)
	require.Equal(t, 1, authState.ConsecutiveFailures)
	require.Equal(t, now.Add(time.Minute), authState.CircuitOpenUntil)
}

func TestAdaptiveCircuitProbeBackoffCapsAtMaximum(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	for i := 1; i <= settings.HealthFailureThreshold; i++ {
		store.observe(adaptiveObservation{
			AccountID:          1,
			RequestID:          "initial-" + string(rune('0'+i)),
			Type:               adaptiveObservationAccountFailure,
			ConfiguredCapacity: 10,
		}, now, settings)
	}

	expected := []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	probeAt := now.Add(time.Minute)
	for i, cooldown := range expected {
		probeAt = probeAt.Add(time.Second)
		requestID := "probe-" + string(rune('0'+i))
		require.True(t, store.claimHealthProbe(1, requestID, 10, probeAt, settings))
		store.registerAdmission(1, requestID, 10, probeAt, settings)
		store.observe(adaptiveObservation{
			AccountID:          1,
			RequestID:          requestID,
			Type:               adaptiveObservationAccountFailure,
			ConfiguredCapacity: 10,
		}, probeAt, settings)
		state := store.snapshot(1, 10, probeAt, settings)
		require.Equal(t, probeAt.Add(cooldown), state.CircuitOpenUntil)
		probeAt = state.CircuitOpenUntil
	}
}

func TestAdaptiveExpiredCircuitIgnoresOldInflightFailureUntilRealProbe(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "auth",
		Type:               adaptiveObservationAccountFailure,
		Authentication:     true,
		ConfiguredCapacity: 10,
	}, now, settings)
	expiredAt := now.Add(time.Minute + time.Second)
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "old-inflight",
		Type:               adaptiveObservationAccountFailure,
		ConfiguredCapacity: 10,
	}, expiredAt, settings)
	state := store.snapshot(1, 10, expiredAt, settings)
	require.Equal(t, 1, state.CircuitOpenCount)
	require.Equal(t, now.Add(time.Minute), state.CircuitOpenUntil)

	require.True(t, store.claimHealthProbe(1, "real-probe", 10, expiredAt, settings))
	store.registerAdmission(1, "real-probe", 10, expiredAt, settings)
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "real-probe",
		Type:               adaptiveObservationAccountFailure,
		ConfiguredCapacity: 10,
	}, expiredAt, settings)
	state = store.snapshot(1, 10, expiredAt, settings)
	require.Equal(t, 2, state.CircuitOpenCount)
	require.Equal(t, expiredAt.Add(2*time.Minute), state.CircuitOpenUntil)
}

func TestAdaptiveDueHealthProbesAreOldestFirstAndExcludeInflight(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	store.ensureLocked(1, 10, now).CircuitOpenUntil = now.Add(-time.Minute)
	store.ensureLocked(2, 10, now).CircuitOpenUntil = now.Add(-2 * time.Minute)
	store.ensureLocked(3, 10, now).CircuitOpenUntil = now.Add(time.Minute)
	store.mu.Unlock()

	require.Equal(t, []int64{2, 1}, store.dueHealthProbeAccountIDs(now, settings))
	require.True(t, store.claimHealthProbe(2, "probe-2", 10, now, settings))
	require.Equal(t, []int64{1}, store.dueHealthProbeAccountIDs(now, settings))
}

func TestAdaptiveHalfOpenProbeLeaseIsSingleFlight(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	store.ensureLocked(1, 10, now).CircuitOpenUntil = now.Add(-time.Second)
	store.mu.Unlock()

	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var wg sync.WaitGroup
	for range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- store.claimHealthProbe(1, "probe", 10, now, settings)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for acquired := range results {
		if acquired {
			winners++
		}
	}
	require.Equal(t, 1, winners)
}

func TestAdaptiveRequestCanClaimOnlyOneHalfOpenProbe(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	store.ensureLocked(1, 10, now).CircuitOpenUntil = now.Add(-2 * time.Second)
	store.ensureLocked(2, 10, now).CircuitOpenUntil = now.Add(-time.Second)
	store.mu.Unlock()

	require.True(t, store.claimHealthProbe(1, "one-probe-request", 10, now, settings))
	store.releaseHealthProbe(1, "one-probe-request", now)
	require.False(t, store.claimHealthProbe(2, "one-probe-request", 10, now, settings))
	require.True(t, store.claimHealthProbe(2, "next-request", 10, now, settings))

	// Healthy accounts remain eligible when the request's half-open budget is spent.
	require.True(t, store.claimHealthProbe(3, "one-probe-request", 10, now, settings))
}

func TestAdaptiveInconclusiveHalfOpenProbeSchedulesShortRetry(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	state := store.ensureLocked(1, 10, now)
	state.CircuitOpenUntil = now.Add(-time.Second)
	state.CircuitOpenCount = 4
	store.mu.Unlock()

	require.True(t, store.claimHealthProbe(1, "probe", 10, now, settings))
	store.registerAdmission(1, "probe", 10, now, settings)
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "probe",
		Type:               adaptiveObservationProviderOverload,
		ConfiguredCapacity: 10,
	}, now, settings)

	stateSnapshot := store.snapshot(1, 10, now, settings)
	require.Equal(t, now.Add(settings.HealthProbeLease), stateSnapshot.CircuitOpenUntil)
	require.Equal(t, 4, stateSnapshot.CircuitOpenCount)
	require.False(t, stateSnapshot.HealthProbeInFlight)
	require.Empty(t, store.dueHealthProbeAccountIDs(now, settings))
	require.Equal(t, []int64{1}, store.dueHealthProbeAccountIDs(now.Add(settings.HealthProbeLease), settings))
}

func TestAdaptiveExpiredCircuitRecoversFromUncorrelatedSuccess(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	state := store.ensureLocked(1, 10, now)
	state.CircuitOpenUntil = now.Add(-time.Second)
	state.CircuitOpenCount = 3
	state.ConsecutiveFailures = 7
	store.mu.Unlock()

	store.observe(adaptiveObservation{
		AccountID:          1,
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10,
	}, now, settings)

	stateSnapshot := store.snapshot(1, 10, now, settings)
	require.True(t, stateSnapshot.CircuitOpenUntil.IsZero())
	require.Zero(t, stateSnapshot.CircuitOpenCount)
	require.Zero(t, stateSnapshot.ConsecutiveFailures)
}

func TestAdaptiveQuotaOnlyClearsAfterSuccessfulQuotaProbe(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "quota-limit",
		Type:               adaptiveObservationQuotaLimit,
		ConfiguredCapacity: 10,
	}, now, settings)

	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "old-success",
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10,
	}, now.Add(time.Second), settings)
	state := store.snapshot(1, 10, now.Add(time.Second), settings)
	require.True(t, state.QuotaLimited)

	probeAt := now.Add(settings.QuotaProbeInterval)
	allowed, claimed := store.claimQuotaProbe(1, "failed-probe", 10, probeAt, settings)
	require.True(t, allowed)
	require.True(t, claimed)
	store.registerAdmission(1, "failed-probe", 10, probeAt, settings)
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "failed-probe",
		Type:               adaptiveObservationAccountFailure,
		ConfiguredCapacity: 10,
	}, probeAt, settings)
	state = store.snapshot(1, 10, probeAt, settings)
	require.True(t, state.QuotaLimited)
	require.Zero(t, state.ConsecutiveFailures)
	require.Len(t, state.HealthObservations, 1)

	successAt := probeAt.Add(settings.QuotaProbeInterval)
	allowed, claimed = store.claimQuotaProbe(1, "successful-probe", 10, successAt, settings)
	require.True(t, allowed)
	require.True(t, claimed)
	store.registerAdmission(1, "successful-probe", 10, successAt, settings)
	store.observe(adaptiveObservation{
		AccountID:          1,
		RequestID:          "successful-probe",
		Type:               adaptiveObservationHealthSuccess,
		ConfiguredCapacity: 10,
	}, successAt, settings)
	state = store.snapshot(1, 10, successAt, settings)
	require.False(t, state.QuotaLimited)
}

func TestAdaptiveCapacityOnlyShrinksForCapacityLimitAndKeepsInflight(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.observeLoad(1, 100, 100, now, settings)

	store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "provider-overload",
		Type:                adaptiveObservationProviderOverload,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now, settings)
	store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "account-failure",
		Type:                adaptiveObservationAccountFailure,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now, settings)
	require.Equal(t, 100, store.snapshot(1, 100, now, settings).EffectiveCapacity)

	_, decreased := store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "capacity-limit",
		Type:                adaptiveObservationCapacityLimit,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now, settings)
	state := store.snapshot(1, 100, now, settings)
	require.True(t, decreased)
	require.Equal(t, 90, state.EffectiveCapacity)
	require.Equal(t, 100, state.LastObservedConcurrency)
}

func TestAdaptiveCapacityGenerationRejectsOldInflightResults(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	require.Equal(t, uint64(1), store.registerAdmission(1, "shrinker", 100, now, settings))
	require.Equal(t, uint64(1), store.registerAdmission(1, "old-inflight", 100, now, settings))

	_, decreased := store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "shrinker",
		Type:                adaptiveObservationCapacityLimit,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now, settings)
	require.True(t, decreased)
	state := store.snapshot(1, 100, now, settings)
	require.Equal(t, uint64(2), state.CapacityGeneration)
	require.Equal(t, 90, state.EffectiveCapacity)

	_, decreased = store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "old-inflight",
		Type:                adaptiveObservationCapacityLimit,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now.Add(time.Second), settings)
	require.False(t, decreased)
	require.Equal(t, 90, store.snapshot(1, 100, now.Add(time.Second), settings).EffectiveCapacity)
}

func TestAdaptiveConfiguredCapacityIncreaseAppliesImmediately(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	_, decreased := store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "limit",
		Type:                adaptiveObservationCapacityLimit,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now, settings)
	require.True(t, decreased)
	require.Equal(t, 90, store.snapshot(1, 100, now, settings).EffectiveCapacity)

	state := store.snapshot(1, 200, now.Add(time.Second), settings)
	require.Equal(t, 200, state.EffectiveCapacity)
	require.False(t, state.CapacityHalfOpen)
	require.Zero(t, state.CapacityRecoverySuccesses)
}

func TestAdaptiveRuntimeSeparatesCircuitAndCapacityRecovery(t *testing.T) {
	now := time.Now()
	state := *newAdaptiveAccountState(1, 100, now)
	state.CircuitOpenUntil = now.Add(-time.Second)
	state.CapacityHalfOpen = true
	main, flags, _, _ := adaptiveRuntimeState(state, true, 1, now)
	require.Equal(t, adaptiveRuntimeCircuitHalfOpen, main)
	require.Contains(t, flags, adaptiveRuntimeCircuitHalfOpen)
	require.Contains(t, flags, adaptiveRuntimeCapacityRecovery)
}

func TestAdaptivePendingAdmissionDoesNotRecoverCapacity(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	_, decreased := store.observe(adaptiveObservation{AccountID: 1, RequestID: "limit", Type: adaptiveObservationCapacityLimit, ConfiguredCapacity: 100, ObservedConcurrency: 100}, now, settings)
	require.True(t, decreased)
	recoveryAt := now.Add(settings.CapacityCooldown)
	state := store.snapshot(1, 100, recoveryAt, settings)
	require.True(t, state.CapacityHalfOpen)
	store.registerPendingAdmission(1, "pending", 100, recoveryAt, settings)
	store.observe(adaptiveObservation{AccountID: 1, RequestID: "pending", Type: adaptiveObservationHealthSuccess, ConfiguredCapacity: 100, ObservedConcurrency: 90, CapacityGeneration: state.CapacityGeneration}, recoveryAt, settings)
	state = store.snapshot(1, 100, recoveryAt, settings)
	require.Zero(t, state.CapacityRecoverySuccesses)
	require.Equal(t, 90, state.EffectiveCapacity)
}

func TestAdaptiveCapacityRecoversAfterSuccessfulAdmissionsInCurrentGeneration(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	_, decreased := store.observe(adaptiveObservation{
		AccountID:           1,
		RequestID:           "capacity-limit",
		Type:                adaptiveObservationCapacityLimit,
		ConfiguredCapacity:  100,
		ObservedConcurrency: 100,
	}, now, settings)
	require.True(t, decreased)
	recoveryAt := now.Add(settings.CapacityCooldown)
	state := store.snapshot(1, 100, recoveryAt, settings)
	require.True(t, state.CapacityHalfOpen)

	for i := 0; i < settings.CapacityRecoverySamples; i++ {
		requestID := "recovery-" + string(rune('a'+i))
		store.registerAdmissionWithLoad(1, requestID, 100, 1, 0, true, recoveryAt.Add(time.Duration(i)*time.Millisecond), settings)
		store.observe(adaptiveObservation{
			AccountID:           1,
			RequestID:           requestID,
			Type:                adaptiveObservationHealthSuccess,
			ConfiguredCapacity:  100,
			ObservedConcurrency: -1,
			CapacityGeneration:  state.CapacityGeneration,
		}, recoveryAt.Add(time.Duration(i)*time.Millisecond), settings)
	}
	state = store.snapshot(1, 100, recoveryAt.Add(time.Second), settings)
	require.Equal(t, 100, state.EffectiveCapacity)
	require.False(t, state.CapacityHalfOpen)
}

func TestAdaptiveHighErrorUsesHysteresis(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	settings.HighErrorMinSamples = 4
	settings.HighErrorMaxSamples = 8
	settings.HighErrorEnterRate = 0.5
	settings.HighErrorExitRate = 0.25
	store := newAdaptiveStateStore()

	results := []bool{false, false, true, true}
	for i, success := range results {
		kind := adaptiveObservationAccountFailure
		if success {
			kind = adaptiveObservationHealthSuccess
		}
		store.observe(adaptiveObservation{
			AccountID:          1,
			RequestID:          "initial-" + string(rune('a'+i)),
			Type:               kind,
			ConfiguredCapacity: 10,
		}, now.Add(time.Duration(i)*time.Second), settings)
	}
	require.True(t, store.snapshot(1, 10, now.Add(4*time.Second), settings).HighError)

	store.observe(adaptiveObservation{AccountID: 1, RequestID: "success-1", Type: adaptiveObservationHealthSuccess, ConfiguredCapacity: 10}, now.Add(5*time.Second), settings)
	require.True(t, store.snapshot(1, 10, now.Add(5*time.Second), settings).HighError)
	for i := 2; i <= 4; i++ {
		store.observe(adaptiveObservation{
			AccountID:          1,
			RequestID:          "success-" + string(rune('0'+i)),
			Type:               adaptiveObservationHealthSuccess,
			ConfiguredCapacity: 10,
		}, now.Add(time.Duration(4+i)*time.Second), settings)
	}
	require.False(t, store.snapshot(1, 10, now.Add(8*time.Second), settings).HighError)
}

func TestAdaptiveOAuthLearningStatusIsNotApplicable(t *testing.T) {
	now := time.Now()
	state := *newAdaptiveAccountState(1, 10, now)
	state.HealthObservations = append(state.HealthObservations, adaptiveHealthObservation{At: now, Success: true})

	status, samples := adaptiveLearningState(state, true, now, defaultAdaptiveCoreSettings())

	require.Equal(t, adaptiveLearningNotApplicable, status)
	require.Zero(t, samples)
}

func TestAdaptiveShadowModeDoesNotExplore(t *testing.T) {
	settings := defaultAdaptiveCoreSettings()
	settings.TopK = 1
	settings.ExplorationRate = 1
	candidates := []adaptiveScoreCandidate{
		{AccountID: 1, Score: 1, HealthSamples: settings.LearningMinHealthSamples},
		{AccountID: 2, Score: 0.1, HealthSamples: 0},
	}

	shadow := orderAdaptiveCandidates(candidates, true, true, time.Now(), settings)
	enforce := orderAdaptiveCandidates(candidates, true, false, time.Now(), settings)

	require.Equal(t, int64(1), shadow[0].AccountID)
	require.Equal(t, int64(2), enforce[0].AccountID)
}

func TestAdaptiveExistingSessionDoesNotExplore(t *testing.T) {
	settings := defaultAdaptiveCoreSettings()
	settings.TopK = 1
	settings.ExplorationRate = 1
	candidates := []adaptiveScoreCandidate{
		{AccountID: 1, Score: 1, HealthSamples: settings.LearningMinHealthSamples},
		{AccountID: 2, Score: 0.1, HealthSamples: 0},
	}

	newSession := orderAdaptiveCandidates(candidates, true, false, time.Now(), settings)
	existingSession := orderAdaptiveCandidates(candidates, false, false, time.Now(), settings)

	require.Equal(t, int64(2), newSession[0].AccountID)
	require.Equal(t, int64(1), existingSession[0].AccountID)
}

func TestAdaptiveDueProbeIncludesQuotaReset(t *testing.T) {
	now := time.Now()
	settings := defaultAdaptiveCoreSettings()
	store := newAdaptiveStateStore()
	store.mu.Lock()
	state := store.ensureLocked(77, 10, now)
	state.QuotaLimited = true
	state.QuotaNextProbeAt = now.Add(-time.Minute)
	store.mu.Unlock()

	require.Equal(t, []int64{77}, store.dueHealthProbeAccountIDs(now, settings))
}
