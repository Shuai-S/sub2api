package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
			AccountID:       1,
			SchedulerStatus: OpenAIAdaptiveLearningStatusUnlearned,
			TotalSamples:    0,
		},
		{
			AccountID:       2,
			SchedulerStatus: OpenAIAdaptiveLearningStatusLearning,
			Learned:         true,
			TotalSamples:    3,
		},
		{
			AccountID:       3,
			SchedulerStatus: OpenAIAdaptiveLearningStatusHighError,
			Learned:         true,
			TotalSamples:    8,
		},
	}

	summary := summarizeOpenAIAdaptiveLearningRows(rows)

	require.Equal(t, 2, summary.TrackedAccounts)
	require.Equal(t, 1, summary.UnlearnedAccounts)
	require.Equal(t, 1, summary.LearningAccounts)
	require.Equal(t, 1, summary.HighErrorAccounts)
}
