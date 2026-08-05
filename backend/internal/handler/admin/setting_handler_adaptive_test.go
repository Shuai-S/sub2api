package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMergeAnthropicAdaptiveSchedulerSettingsUpdatesOnlyProvidedFields(t *testing.T) {
	previous := service.DefaultAnthropicAdaptiveSchedulerSettings()
	diagnosticLogEnabled := true
	diagnosticLogSampleRate := 0.25
	topK := 3
	softShrink := 0.4
	hardShrink := 0.8

	merged := mergeAnthropicAdaptiveSchedulerSettings(previous, AnthropicAdaptiveSchedulerSettingsUpdateRequest{
		AnthropicAdaptiveSchedulerDiagnosticLogEnabled:    &diagnosticLogEnabled,
		AnthropicAdaptiveSchedulerDiagnosticLogSampleRate: &diagnosticLogSampleRate,
		AnthropicAdaptiveSchedulerTopK:                    &topK,
		AnthropicAdaptiveSchedulerShrinkFactorSoft:        &softShrink,
		AnthropicAdaptiveSchedulerShrinkFactorHard:        &hardShrink,
	})

	require.True(t, merged.AnthropicAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.25, merged.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, 3, merged.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.4, merged.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	require.Equal(t, 0.4, merged.AnthropicAdaptiveSchedulerShrinkFactorHard)
	require.Equal(t, previous.AnthropicAdaptiveSchedulerSoftmaxTemperature, merged.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, previous.AnthropicAdaptiveSchedulerWeightReliability, merged.AnthropicAdaptiveSchedulerWeightReliability)
}

func TestMergeGeminiAdaptiveSchedulerSettingsIncludesCircuitBreakerFields(t *testing.T) {
	previous := service.DefaultGeminiAdaptiveSchedulerSettings()
	cooldownMaxSeconds := 480
	accountFailureThreshold := 4
	modelFailureThreshold := 5
	halfOpenProbeLeaseSeconds := 90

	merged := mergeGeminiAdaptiveSchedulerSettings(previous, GeminiAdaptiveSchedulerSettingsUpdateRequest{
		GeminiAdaptiveSchedulerCooldownMaxSeconds:        &cooldownMaxSeconds,
		GeminiAdaptiveSchedulerAccountFailureThreshold:   &accountFailureThreshold,
		GeminiAdaptiveSchedulerModelFailureThreshold:     &modelFailureThreshold,
		GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds: &halfOpenProbeLeaseSeconds,
	})

	require.Equal(t, cooldownMaxSeconds, merged.GeminiAdaptiveSchedulerCooldownMaxSeconds)
	require.Equal(t, accountFailureThreshold, merged.GeminiAdaptiveSchedulerAccountFailureThreshold)
	require.Equal(t, modelFailureThreshold, merged.GeminiAdaptiveSchedulerModelFailureThreshold)
	require.Equal(t, halfOpenProbeLeaseSeconds, merged.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds)
}
