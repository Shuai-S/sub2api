package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeminiAdaptiveLearningSnapshotIncludesAccountLevelCoreState(t *testing.T) {
	rate := 0.8
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	account := &Account{
		ID:             42,
		Name:           "gemini-42",
		Platform:       PlatformGemini,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Schedulable:    true,
		Concurrency:    8,
		RateMultiplier: &rate,
	}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)
	state.SuccessEMA = 0.9
	state.TTFTEMA = 180
	state.TTFTSamples = 12
	for i := 0; i < settings.LearningMinHealthSamples; i++ {
		state.HealthObservations = append(state.HealthObservations, adaptiveHealthObservation{At: now.Add(-time.Duration(i) * time.Second), Success: true})
	}
	load := &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 2, WaitingCount: 1}

	snapshot := buildGeminiAdaptiveCoreLearningAccountSnapshot(account, *state, load, now, settings)

	require.Equal(t, account.ID, snapshot.AccountID)
	require.InDelta(t, rate, snapshot.RateMultiplier, 1e-12)
	require.Equal(t, account.Concurrency, snapshot.EffectiveCapacity)
	require.Equal(t, string(adaptiveLearningLearned), snapshot.LearningStatus)
	require.Equal(t, string(adaptiveRuntimeHealthy), snapshot.RuntimeStatus)
	require.Equal(t, settings.LearningMinHealthSamples, snapshot.HealthSamples)
	require.Equal(t, 180.0, snapshot.TTFTEMA)
	require.Equal(t, int64(12), snapshot.TTFTSamples)
	require.Nil(t, snapshot.Quota)
}

func TestGeminiAdaptiveLearningSnapshotMarksOAuthNotApplicable(t *testing.T) {
	now := time.Now()
	account := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)

	snapshot := buildGeminiAdaptiveCoreLearningAccountSnapshot(account, *state, &AccountLoadInfo{}, now, defaultAdaptiveCoreSettings())

	require.Equal(t, string(adaptiveLearningNotApplicable), snapshot.LearningStatus)
	require.True(t, snapshot.Learned)
}

func TestGeminiAdaptiveLearningFilterSortAndSummary(t *testing.T) {
	rows := []GeminiAdaptiveSchedulerAccountLearningSnapshot{
		{AccountID: 1, AccountName: "alpha", LearningStatus: string(adaptiveLearningLearned), RuntimeStatus: string(adaptiveRuntimeHealthy), SchedulerStatus: string(adaptiveRuntimeHealthy), Learned: true, SchedulerScore: 0.9},
		{AccountID: 2, AccountName: "beta", LearningStatus: string(adaptiveLearningLearned), RuntimeStatus: string(adaptiveRuntimeCooldown), SchedulerStatus: string(adaptiveRuntimeCooldown), Learned: true, SchedulerScore: 0.2},
		{AccountID: 3, AccountName: "gamma", LearningStatus: string(adaptiveLearningUnlearned), RuntimeStatus: string(adaptiveRuntimeQuotaLimited), SchedulerStatus: string(adaptiveRuntimeQuotaLimited), SchedulerScore: 0.5},
	}

	summary := summarizeGeminiAdaptiveLearningRows(rows)
	require.Equal(t, 3, summary.TrackedAccounts)
	require.Equal(t, 2, summary.LearnedAccounts)
	require.Equal(t, 1, summary.HealthyAccounts)
	require.Equal(t, 1, summary.CooldownAccounts)
	require.Equal(t, 1, summary.UnlearnedAccounts)
	require.Equal(t, 1, summary.QuotaLimitedAccounts)

	filtered := filterGeminiAdaptiveLearningRowsByDualStatus(rows, string(adaptiveLearningLearned), string(adaptiveRuntimeHealthy))
	require.Len(t, filtered, 1)
	require.Equal(t, int64(1), filtered[0].AccountID)

	rows = []GeminiAdaptiveSchedulerAccountLearningSnapshot{
		{AccountID: 1, SchedulerScore: 0.2},
		{AccountID: 2, SchedulerScore: 0.9},
		{AccountID: 3, SchedulerScore: 0.5},
	}
	sortGeminiAdaptiveLearningRows(rows, "score", "desc")
	require.Equal(t, []int64{2, 3, 1}, []int64{rows[0].AccountID, rows[1].AccountID, rows[2].AccountID})
}

func TestFilterGeminiAdaptiveLearningSchedulableAccounts(t *testing.T) {
	accounts := []Account{{ID: 1, Schedulable: false}, {ID: 2, Schedulable: true}, {ID: 3, Status: StatusDisabled, Schedulable: true}}
	got := filterGeminiAdaptiveLearningSchedulableAccounts(accounts)
	require.Equal(t, []int64{2, 3}, []int64{got[0].ID, got[1].ID})
}

func TestGeminiAdaptiveLearningLastEventIncludesQuotaReset(t *testing.T) {
	lastSuccess := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	quotaReset := lastSuccess.Add(time.Hour)

	got := geminiAdaptiveLearningLastEventTime(GeminiAdaptiveSchedulerAccountLearningSnapshot{
		LastSuccessAt: &lastSuccess,
		QuotaResetAt:  &quotaReset,
	})

	require.Equal(t, quotaReset, got)
}
