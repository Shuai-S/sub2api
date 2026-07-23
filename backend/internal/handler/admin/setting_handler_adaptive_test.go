package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMergeAnthropicAdaptiveSchedulerSettingsUpdatesOnlyProvidedFields(t *testing.T) {
	previous := service.DefaultAnthropicAdaptiveSchedulerSettings()
	topK := 3
	softShrink := 0.4
	hardShrink := 0.8

	merged := mergeAnthropicAdaptiveSchedulerSettings(previous, AnthropicAdaptiveSchedulerSettingsUpdateRequest{
		AnthropicAdaptiveSchedulerTopK:             &topK,
		AnthropicAdaptiveSchedulerShrinkFactorSoft: &softShrink,
		AnthropicAdaptiveSchedulerShrinkFactorHard: &hardShrink,
	})

	require.Equal(t, 3, merged.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.4, merged.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	require.Equal(t, 0.4, merged.AnthropicAdaptiveSchedulerShrinkFactorHard)
	require.Equal(t, previous.AnthropicAdaptiveSchedulerSoftmaxTemperature, merged.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, previous.AnthropicAdaptiveSchedulerWeightReliability, merged.AnthropicAdaptiveSchedulerWeightReliability)
}
