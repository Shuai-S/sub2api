package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeminiAdaptiveLearningSnapshotReadDoesNotCreateState(t *testing.T) {
	store := newGeminiAdaptiveStateStore()
	account := &Account{ID: 42, Concurrency: 8}

	snapshot := store.snapshot(account, DefaultGeminiAdaptiveSchedulerSettings())

	require.Equal(t, 8, snapshot.EstimatedCapacity)
	store.mu.RLock()
	defer store.mu.RUnlock()
	require.Empty(t, store.accounts)
}

func TestGeminiAdaptiveLearningAccountStatuses(t *testing.T) {
	now := time.Now()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerEnabled = true
	settings.GeminiAdaptiveSchedulerMinRecentSamplesForShrink = 10
	settings.GeminiAdaptiveSchedulerCapacityFailureThreshold = 3
	settings.GeminiAdaptiveSchedulerShrinkErrorThreshold = 0.25
	account := &Account{
		ID:          1,
		Platform:    PlatformGemini,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
	}
	baseState := defaultGeminiAdaptiveAccountState(account, now, settings)

	tests := []struct {
		name     string
		enabled  bool
		account  *Account
		state    geminiAdaptiveAccountState
		load     *AccountLoadInfo
		quota    GeminiAdaptiveQuotaSnapshot
		failRate float64
		want     string
	}{
		{name: "disabled", account: account, state: baseState, load: &AccountLoadInfo{}, want: GeminiAdaptiveLearningStatusDisabled},
		{name: "unavailable", enabled: true, account: &Account{ID: 1, Status: StatusDisabled, Schedulable: true, Concurrency: 10}, state: baseState, load: &AccountLoadInfo{}, want: GeminiAdaptiveLearningStatusUnavailable},
		{name: "quota limited", enabled: true, account: account, state: baseState, load: &AccountLoadInfo{}, quota: GeminiAdaptiveQuotaSnapshot{HardRejected: true}, want: GeminiAdaptiveLearningStatusQuotaLimited},
		{
			name: "cooldown", enabled: true, account: account,
			state: func() geminiAdaptiveAccountState {
				state := baseState
				state.CooldownUntil = now.Add(time.Minute)
				return state
			}(),
			load: &AccountLoadInfo{}, want: GeminiAdaptiveLearningStatusCooldown,
		},
		{
			name: "high error", enabled: true, account: account,
			state: func() geminiAdaptiveAccountState {
				state := baseState
				state.RecentCapacitySamples = 10
				state.RecentCapacityFailures = 4
				return state
			}(),
			load: &AccountLoadInfo{}, failRate: 0.4, want: GeminiAdaptiveLearningStatusHighError,
		},
		{name: "saturated", enabled: true, account: account, state: baseState, load: &AccountLoadInfo{CurrentConcurrency: 10}, want: GeminiAdaptiveLearningStatusSaturated},
		{name: "unlearned", enabled: true, account: account, state: baseState, load: &AccountLoadInfo{}, want: GeminiAdaptiveLearningStatusUnlearned},
		{
			name: "learning", enabled: true, account: account,
			state: func() geminiAdaptiveAccountState {
				state := baseState
				state.TotalSamples = 9
				return state
			}(),
			load: &AccountLoadInfo{}, want: GeminiAdaptiveLearningStatusLearning,
		},
		{
			name: "healthy", enabled: true, account: account,
			state: func() geminiAdaptiveAccountState {
				state := baseState
				state.TotalSamples = 10
				return state
			}(),
			load: &AccountLoadInfo{}, want: GeminiAdaptiveLearningStatusHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := settings
			cfg.GeminiAdaptiveSchedulerEnabled = tt.enabled
			got, _ := geminiAdaptiveLearningAccountStatus(tt.account, tt.state, cfg, tt.load, tt.quota, 10, tt.failRate, now)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGeminiAdaptiveLearningFilterSortAndSummary(t *testing.T) {
	rows := []GeminiAdaptiveSchedulerAccountLearningSnapshot{
		{AccountID: 1, AccountName: "alpha", SchedulerStatus: GeminiAdaptiveLearningStatusHealthy, Learned: true, SchedulerScore: 0.9},
		{AccountID: 2, AccountName: "beta", SchedulerStatus: GeminiAdaptiveLearningStatusCooldown, Learned: true, SchedulerScore: 0.2},
		{AccountID: 3, AccountName: "gamma", SchedulerStatus: GeminiAdaptiveLearningStatusQuotaLimited, SchedulerScore: 0.5},
	}

	summary := summarizeGeminiAdaptiveLearningRows(rows)
	require.Equal(t, 3, summary.TrackedAccounts)
	require.Equal(t, 1, summary.HealthyAccounts)
	require.Equal(t, 1, summary.CooldownAccounts)
	require.Equal(t, 1, summary.QuotaLimitedAccounts)

	filtered := filterGeminiAdaptiveLearningRowsByStatus(rows, GeminiAdaptiveLearningStatusHealthy)
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

func TestGeminiAdaptiveModelSnapshotsAreStableAndComplete(t *testing.T) {
	got := geminiAdaptiveModelLearningSnapshots(map[string]geminiAdaptiveModelState{
		"pro":   {SuccessEMA: 0.9, TTFTEMA: 120, LatencyEMA: 700, Samples: 4, Failures: 1},
		"flash": {SuccessEMA: 1, TTFTEMA: 80, LatencyEMA: 400, Samples: 2},
	})

	require.Equal(t, []GeminiAdaptiveModelLearningSnapshot{
		{ModelFamily: "flash", SuccessEMA: 1, TTFTEMA: 80, LatencyEMA: 400, Samples: 2},
		{ModelFamily: "pro", SuccessEMA: 0.9, TTFTEMA: 120, LatencyEMA: 700, Samples: 4, Failures: 1},
	}, got)
}
