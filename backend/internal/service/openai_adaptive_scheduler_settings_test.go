package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultOpenAIAdaptiveSchedulerSettingsBalanceAvailabilityAndCost(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()

	require.False(t, cfg.OpenAIAdaptiveSchedulerEnabled)
	require.False(t, cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, openAIAdaptiveSchedulerModeEnforce, cfg.OpenAIAdaptiveSchedulerMode)
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityMixed, cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode)
	require.Equal(t, 10, cfg.OpenAIAdaptiveSchedulerTopK)
	require.Equal(t, 0.01, cfg.OpenAIAdaptiveSchedulerExplorationRate)
	require.Equal(t, 0.35, cfg.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerMinCostMultiplier)
	require.True(t, cfg.OpenAIAdaptiveSchedulerThompsonEnabled)

	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction)
	require.Equal(t, 2, cfg.OpenAIAdaptiveSchedulerMinCapacity)
	require.Equal(t, 2, cfg.OpenAIAdaptiveSchedulerCapacityIncreaseStep)
	require.Equal(t, 1.15, cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	require.Equal(t, 0.80, cfg.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	require.Equal(t, 0.15, cfg.OpenAIAdaptiveSchedulerBurstProbeRatio)
	require.Equal(t, 0.95, cfg.OpenAIAdaptiveSchedulerCapacitySuccessThreshold)
	require.Equal(t, 3, cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold)
	require.Equal(t, 50, cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink)
	require.Equal(t, 0.35, cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold)
	require.Equal(t, 0.90, cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	require.Equal(t, 0.70, cfg.OpenAIAdaptiveSchedulerShrinkFactorHard)
	require.Equal(t, 1, cfg.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold)
	require.Equal(t, 3, cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity)
	require.Equal(t, 1200, cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds)

	require.Equal(t, 0.04, cfg.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	require.Equal(t, 0.06, cfg.OpenAIAdaptiveSchedulerErrorEMAAlpha)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerLatencyEMAAlpha)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	require.Equal(t, 60, cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds)
	require.Equal(t, 600, cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds)

	require.Equal(t, 0.40, cfg.OpenAIAdaptiveSchedulerWeightSuccess)
	require.Equal(t, 0.25, cfg.OpenAIAdaptiveSchedulerWeightCost)
	require.Equal(t, 0.20, cfg.OpenAIAdaptiveSchedulerWeightCapacity)
	require.Equal(t, 0.10, cfg.OpenAIAdaptiveSchedulerWeightLatency)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerWeightStability)
	require.Equal(t, 0.02, cfg.OpenAIAdaptiveSchedulerWeightExploration)
}

func TestOpenAIAdaptiveSchedulerDiagnosticSettingsRoundTrip(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 0.25
	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst

	values := openAIAdaptiveSchedulerSettingsToMap(cfg)
	require.Equal(t, "true", values[openAIAdaptiveSchedulerDiagnosticLogEnabledKey])
	require.Equal(t, "0.25", values[openAIAdaptiveSchedulerDiagnosticLogSampleRateKey])
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst, values[openAIAdaptiveSchedulerAccountTypePriorityModeKey])

	parsed := parseOpenAIAdaptiveSchedulerSettings(values)
	require.True(t, parsed.OpenAIAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.25, parsed.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst, parsed.OpenAIAdaptiveSchedulerAccountTypePriorityMode)
}

func TestNormalizeOpenAIAdaptiveSchedulerDiagnosticSampleRate(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 2

	normalized := NormalizeOpenAIAdaptiveSchedulerSettings(cfg)
	require.Equal(t, 0.05, normalized.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
}

func TestNormalizeOpenAIAdaptiveSchedulerAccountTypePriorityMode(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = "api_key_first"
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityAPIKeyFirst, NormalizeOpenAIAdaptiveSchedulerSettings(cfg).OpenAIAdaptiveSchedulerAccountTypePriorityMode)

	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = "not-a-mode"
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityMixed, NormalizeOpenAIAdaptiveSchedulerSettings(cfg).OpenAIAdaptiveSchedulerAccountTypePriorityMode)
}
