package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnthropicAdaptiveLearningSnapshotReadDoesNotCreateState(t *testing.T) {
	store := newAnthropicAdaptiveStateStore()
	account := &Account{ID: 42, Concurrency: 8}

	snapshot := store.snapshot(account, DefaultAnthropicAdaptiveSchedulerSettings())

	require.Equal(t, 8, snapshot.EstimatedCapacity)
	store.mu.RLock()
	defer store.mu.RUnlock()
	require.Empty(t, store.accounts)
}

func TestAnthropicAdaptiveLearningAccountStatuses(t *testing.T) {
	now := time.Now()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerEnabled = true
	settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink = 10
	settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold = 3
	settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold = 0.25
	account := &Account{
		ID:          1,
		Platform:    PlatformAnthropic,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
	}
	baseState := defaultAnthropicAdaptiveAccountState(account, now, settings)

	tests := []struct {
		name             string
		enabled          bool
		account          *Account
		state            anthropicAdaptiveAccountState
		load             *AccountLoadInfo
		capacityFailRate float64
		want             string
	}{
		{
			name:    "disabled",
			enabled: false,
			account: account,
			state:   baseState,
			load:    &AccountLoadInfo{},
			want:    AnthropicAdaptiveLearningStatusDisabled,
		},
		{
			name:    "unavailable",
			enabled: true,
			account: &Account{ID: 1, Status: StatusDisabled, Schedulable: true, Concurrency: 10},
			state:   baseState,
			load:    &AccountLoadInfo{},
			want:    AnthropicAdaptiveLearningStatusUnavailable,
		},
		{
			name:    "cooldown",
			enabled: true,
			account: account,
			state: func() anthropicAdaptiveAccountState {
				state := baseState
				state.CooldownUntil = now.Add(time.Minute)
				return state
			}(),
			load: &AccountLoadInfo{},
			want: AnthropicAdaptiveLearningStatusCooldown,
		},
		{
			name:    "high error",
			enabled: true,
			account: account,
			state: func() anthropicAdaptiveAccountState {
				state := baseState
				state.RecentCapacitySamples = 10
				state.RecentCapacityFailures = 4
				return state
			}(),
			load:             &AccountLoadInfo{},
			capacityFailRate: 0.4,
			want:             AnthropicAdaptiveLearningStatusHighError,
		},
		{
			name:    "saturated",
			enabled: true,
			account: account,
			state:   baseState,
			load:    &AccountLoadInfo{CurrentConcurrency: 10},
			want:    AnthropicAdaptiveLearningStatusSaturated,
		},
		{
			name:    "unlearned",
			enabled: true,
			account: account,
			state:   baseState,
			load:    &AccountLoadInfo{},
			want:    AnthropicAdaptiveLearningStatusUnlearned,
		},
		{
			name:    "learning",
			enabled: true,
			account: account,
			state: func() anthropicAdaptiveAccountState {
				state := baseState
				state.TotalSamples = 9
				return state
			}(),
			load: &AccountLoadInfo{},
			want: AnthropicAdaptiveLearningStatusLearning,
		},
		{
			name:    "healthy",
			enabled: true,
			account: account,
			state: func() anthropicAdaptiveAccountState {
				state := baseState
				state.TotalSamples = 10
				return state
			}(),
			load: &AccountLoadInfo{},
			want: AnthropicAdaptiveLearningStatusHealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := anthropicAdaptiveLearningAccountStatus(
				tt.account,
				tt.state,
				settings,
				tt.load,
				10,
				tt.capacityFailRate,
				now,
				tt.enabled,
			)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAnthropicAdaptiveLearningFilterSortAndSummary(t *testing.T) {
	rows := []AnthropicAdaptiveSchedulerAccountLearningSnapshot{
		{AccountID: 1, AccountName: "alpha", SchedulerStatus: AnthropicAdaptiveLearningStatusHealthy, Learned: true, SchedulerScore: 0.9},
		{AccountID: 2, AccountName: "beta", SchedulerStatus: AnthropicAdaptiveLearningStatusCooldown, Learned: true, SchedulerScore: 0.2},
		{AccountID: 3, AccountName: "gamma", SchedulerStatus: AnthropicAdaptiveLearningStatusUnlearned, SchedulerScore: 0.5},
	}

	summary := summarizeAnthropicAdaptiveLearningRows(rows)
	require.Equal(t, 2, summary.TrackedAccounts)
	require.Equal(t, 1, summary.HealthyAccounts)
	require.Equal(t, 1, summary.CooldownAccounts)
	require.Equal(t, 1, summary.UnlearnedAccounts)

	filtered := filterAnthropicAdaptiveLearningRowsByStatus(rows, AnthropicAdaptiveLearningStatusHealthy)
	require.Len(t, filtered, 1)
	require.Equal(t, int64(1), filtered[0].AccountID)

	rows = []AnthropicAdaptiveSchedulerAccountLearningSnapshot{
		{AccountID: 1, SchedulerScore: 0.2},
		{AccountID: 2, SchedulerScore: 0.9},
		{AccountID: 3, SchedulerScore: 0.5},
	}
	sortAnthropicAdaptiveLearningRows(rows, "score", "desc")
	require.Equal(t, []int64{2, 3, 1}, []int64{rows[0].AccountID, rows[1].AccountID, rows[2].AccountID})
}

func TestAnthropicAdaptiveLatencySnapshotsAreStableAndComplete(t *testing.T) {
	got := anthropicAdaptiveLatencySnapshots(map[string]anthropicAdaptiveLatencyState{
		"sonnet": {TTFTEMA: 120, LatencyEMA: 700, Samples: 4},
		"opus":   {TTFTEMA: 300, LatencyEMA: 1200, Samples: 2},
	})

	require.Equal(t, []AnthropicAdaptiveLatencyLearningSnapshot{
		{ModelFamily: "opus", TTFTEMA: 300, LatencyEMA: 1200, Samples: 2},
		{ModelFamily: "sonnet", TTFTEMA: 120, LatencyEMA: 700, Samples: 4},
	}, got)
}
