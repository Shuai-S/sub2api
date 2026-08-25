package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAdaptiveLearningSnapshotIncludesAccountLevelCoreState(t *testing.T) {
	rate := 0.8
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	settings := defaultAdaptiveCoreSettings()
	account := &Account{
		ID:             42,
		Name:           "openai-42",
		Platform:       PlatformOpenAI,
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

	snapshot := buildOpenAIAdaptiveCoreLearningAccountSnapshot(account, *state, load, now, settings)

	require.Equal(t, account.ID, snapshot.AccountID)
	require.InDelta(t, rate, snapshot.RateMultiplier, 1e-12)
	require.Equal(t, account.Concurrency, snapshot.EffectiveCapacity)
	require.Equal(t, string(adaptiveLearningLearned), snapshot.LearningStatus)
	require.Equal(t, string(adaptiveRuntimeHealthy), snapshot.RuntimeStatus)
	require.Equal(t, settings.LearningMinHealthSamples, snapshot.HealthSamples)
	require.Equal(t, 180.0, snapshot.TTFTEMA)
	require.Equal(t, int64(12), snapshot.TTFTSamples)
}

func TestOpenAIAdaptiveLearningSnapshotMarksOAuthNotApplicable(t *testing.T) {
	now := time.Now()
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 10}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)

	snapshot := buildOpenAIAdaptiveCoreLearningAccountSnapshot(account, *state, &AccountLoadInfo{}, now, defaultAdaptiveCoreSettings())

	require.Equal(t, string(adaptiveLearningNotApplicable), snapshot.LearningStatus)
	require.True(t, snapshot.Learned)
}

func TestOpenAIAdaptiveLearningSnapshotMarksRateLimitedOAuthUnavailableUntilRecovery(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(2 * time.Hour)
	account := &Account{
		ID:               7,
		Name:             "rate-limited-oauth",
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      100,
		RateLimitResetAt: &resetAt,
	}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)

	limited := buildOpenAIAdaptiveCoreLearningAccountSnapshot(account, *state, &AccountLoadInfo{}, now, defaultAdaptiveCoreSettings())

	require.False(t, limited.Schedulable)
	require.Equal(t, string(adaptiveLearningNotApplicable), limited.LearningStatus)
	require.Equal(t, string(adaptiveRuntimeUnavailable), limited.RuntimeStatus)
	require.Contains(t, limited.RuntimeFlags, string(adaptiveRuntimeUnavailable))
	require.Contains(t, limited.RuntimeFlags, string(adaptiveRuntimeQuotaLimited))
	require.Equal(t, "account_rate_limited", limited.RuntimeReasonCode)
	require.True(t, limited.QuotaLimited)
	require.NotNil(t, limited.QuotaResetAt)
	require.Equal(t, resetAt, *limited.QuotaResetAt)
	limitedSummary := summarizeOpenAIAdaptiveLearningRows([]OpenAIAdaptiveSchedulerAccountLearningSnapshot{limited})
	require.Zero(t, limitedSummary.HealthyAccounts)
	require.Equal(t, 1, limitedSummary.UnavailableAccounts)

	recoveredAt := resetAt.Add(time.Second)
	recovered := buildOpenAIAdaptiveCoreLearningAccountSnapshot(account, *state, &AccountLoadInfo{}, recoveredAt, defaultAdaptiveCoreSettings())

	require.True(t, recovered.Schedulable)
	require.Equal(t, string(adaptiveRuntimeHealthy), recovered.RuntimeStatus)
	require.Equal(t, []string{string(adaptiveRuntimeHealthy)}, recovered.RuntimeFlags)
	require.Empty(t, recovered.RuntimeReasonCode)
	require.False(t, recovered.QuotaLimited)
	require.Nil(t, recovered.QuotaResetAt)
	recoveredSummary := summarizeOpenAIAdaptiveLearningRows([]OpenAIAdaptiveSchedulerAccountLearningSnapshot{recovered})
	require.Equal(t, 1, recoveredSummary.HealthyAccounts)
	require.Zero(t, recoveredSummary.UnavailableAccounts)
}

func TestFilterOpenAIAdaptiveLearningSchedulableAccountsHidesSchedulingDisabled(t *testing.T) {
	accounts := []Account{
		{
			ID:          1,
			Schedulable: false,
		},
		{
			ID:          2,
			Schedulable: true,
		},
		{
			ID:          3,
			Status:      StatusDisabled,
			Schedulable: true,
		},
	}

	got := filterOpenAIAdaptiveLearningSchedulableAccounts(accounts)

	require.Len(t, got, 2)
	require.Equal(t, []int64{2, 3}, []int64{
		got[0].ID,
		got[1].ID,
	})
}

func TestOpenAIAdaptiveLearningUnlearnedRowsRemainSummarized(t *testing.T) {
	rows := []OpenAIAdaptiveSchedulerAccountLearningSnapshot{
		{
			AccountID:      1,
			LearningStatus: string(adaptiveLearningUnlearned),
			RuntimeStatus:  string(adaptiveRuntimeHealthy),
			TotalSamples:   0,
		},
		{
			AccountID:      2,
			LearningStatus: string(adaptiveLearningLearning),
			RuntimeStatus:  string(adaptiveRuntimeCooldown),
			TotalSamples:   3,
		},
		{
			AccountID:      3,
			LearningStatus: string(adaptiveLearningLearned),
			RuntimeStatus:  string(adaptiveRuntimeHighError),
			Learned:        true,
			TotalSamples:   8,
		},
	}

	summary := summarizeOpenAIAdaptiveLearningRows(rows)

	require.Equal(t, 3, summary.TrackedAccounts)
	require.Equal(t, 1, summary.UnlearnedAccounts)
	require.Equal(t, 1, summary.LearningAccounts)
	require.Equal(t, 1, summary.LearnedAccounts)
	require.Equal(t, 1, summary.CooldownAccounts)
	require.Equal(t, 1, summary.HighErrorAccounts)
}

func TestOpenAIAdaptiveLearningReportsQuotaLimited(t *testing.T) {
	summary := summarizeOpenAIAdaptiveLearningRows([]OpenAIAdaptiveSchedulerAccountLearningSnapshot{{
		AccountID: 1, LearningStatus: string(adaptiveLearningLearned), RuntimeStatus: string(adaptiveRuntimeQuotaLimited),
	}})
	require.Equal(t, 1, summary.QuotaLimitedAccounts)
}
