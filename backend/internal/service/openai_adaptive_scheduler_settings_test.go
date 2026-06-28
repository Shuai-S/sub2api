package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultOpenAIAdaptiveSchedulerSettingsBalanceAvailabilityAndCost(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()

	require.False(t, cfg.OpenAIAdaptiveSchedulerEnabled)
	require.Equal(t, openAIAdaptiveSchedulerModeEnforce, cfg.OpenAIAdaptiveSchedulerMode)
	require.Equal(t, 15, cfg.OpenAIAdaptiveSchedulerTopK)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerExplorationRate)
	require.Equal(t, 0.45, cfg.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerMinCostMultiplier)
	require.True(t, cfg.OpenAIAdaptiveSchedulerThompsonEnabled)

	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction)
	require.Equal(t, 2, cfg.OpenAIAdaptiveSchedulerMinCapacity)
	require.Equal(t, 2, cfg.OpenAIAdaptiveSchedulerCapacityIncreaseStep)
	require.Equal(t, 1.20, cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	require.Equal(t, 0.75, cfg.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	require.Equal(t, 0.30, cfg.OpenAIAdaptiveSchedulerBurstProbeRatio)
	require.Equal(t, 0.95, cfg.OpenAIAdaptiveSchedulerCapacitySuccessThreshold)
	require.Equal(t, 5, cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold)
	require.Equal(t, 20, cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink)
	require.Equal(t, 0.30, cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold)
	require.Equal(t, 0.85, cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	require.Equal(t, 0.60, cfg.OpenAIAdaptiveSchedulerShrinkFactorHard)
	require.Equal(t, 3, cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity)
	require.Equal(t, 1200, cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds)

	require.Equal(t, 0.04, cfg.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	require.Equal(t, 0.06, cfg.OpenAIAdaptiveSchedulerErrorEMAAlpha)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerLatencyEMAAlpha)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	require.Equal(t, 30, cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds)
	require.Equal(t, 180, cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds)

	require.Equal(t, 0.35, cfg.OpenAIAdaptiveSchedulerWeightSuccess)
	require.Equal(t, 0.30, cfg.OpenAIAdaptiveSchedulerWeightCost)
	require.Equal(t, 0.20, cfg.OpenAIAdaptiveSchedulerWeightCapacity)
	require.Equal(t, 0.10, cfg.OpenAIAdaptiveSchedulerWeightLatency)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerWeightStability)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerWeightExploration)
}
