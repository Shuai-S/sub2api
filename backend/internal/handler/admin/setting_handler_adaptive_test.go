package admin

import (
	"encoding/json"
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
	capacityGrowth := 1.2

	merged := mergeAnthropicAdaptiveSchedulerSettings(previous, AnthropicAdaptiveSchedulerSettingsUpdateRequest{
		AnthropicAdaptiveSchedulerDiagnosticLogEnabled:    &diagnosticLogEnabled,
		AnthropicAdaptiveSchedulerDiagnosticLogSampleRate: &diagnosticLogSampleRate,
		AnthropicAdaptiveSchedulerTopK:                    &topK,
		AnthropicAdaptiveSchedulerShrinkFactorSoft:        &softShrink,
		AnthropicAdaptiveSchedulerCapacityGrowthFactor:    &capacityGrowth,
	})

	require.True(t, merged.AnthropicAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.25, merged.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, 3, merged.AnthropicAdaptiveSchedulerTopK)
	require.Equal(t, 0.4, merged.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	require.Equal(t, 1.2, merged.AnthropicAdaptiveSchedulerCapacityGrowthFactor)
	require.Equal(t, previous.AnthropicAdaptiveSchedulerSoftmaxTemperature, merged.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, previous.AnthropicAdaptiveSchedulerWeightReliability, merged.AnthropicAdaptiveSchedulerWeightReliability)
}

func TestMergeGeminiAdaptiveSchedulerSettingsIncludesAccountCircuitFields(t *testing.T) {
	previous := service.DefaultGeminiAdaptiveSchedulerSettings()
	cooldownMaxSeconds := 480
	accountFailureThreshold := 4
	quotaProbeIntervalSeconds := 180

	merged := mergeGeminiAdaptiveSchedulerSettings(previous, GeminiAdaptiveSchedulerSettingsUpdateRequest{
		GeminiAdaptiveSchedulerCooldownMaxSeconds:        &cooldownMaxSeconds,
		GeminiAdaptiveSchedulerAccountFailureThreshold:   &accountFailureThreshold,
		GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds: &quotaProbeIntervalSeconds,
	})

	require.Equal(t, cooldownMaxSeconds, merged.GeminiAdaptiveSchedulerCooldownMaxSeconds)
	require.Equal(t, accountFailureThreshold, merged.GeminiAdaptiveSchedulerAccountFailureThreshold)
	require.Equal(t, quotaProbeIntervalSeconds, merged.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds)
}

func TestAdaptiveSchedulerWeightCacheUpdateRequestBindingAndMerge(t *testing.T) {
	var req UpdateSettingsRequest
	err := json.Unmarshal([]byte(`{
		"anthropic_adaptive_scheduler_weight_cache": 0.11,
		"gemini_adaptive_scheduler_weight_cache": 0.22,
		"openai_adaptive_scheduler_weight_cache": 0.33
	}`), &req)
	require.NoError(t, err)

	require.NotNil(t, req.AnthropicAdaptiveSchedulerWeightCache)
	require.NotNil(t, req.GeminiAdaptiveSchedulerWeightCache)
	require.NotNil(t, req.OpenAIAdaptiveSchedulerWeightCache)

	anthropic := mergeAnthropicAdaptiveSchedulerSettings(
		service.DefaultAnthropicAdaptiveSchedulerSettings(),
		req.AnthropicAdaptiveSchedulerSettingsUpdateRequest,
	)
	gemini := mergeGeminiAdaptiveSchedulerSettings(
		service.DefaultGeminiAdaptiveSchedulerSettings(),
		req.GeminiAdaptiveSchedulerSettingsUpdateRequest,
	)
	openAI := mergeOpenAIAdaptiveSchedulerSettings(
		service.DefaultOpenAIAdaptiveSchedulerSettings(),
		req.OpenAIAdaptiveSchedulerSettingsUpdateRequest,
	)

	require.Equal(t, 0.11, anthropic.AnthropicAdaptiveSchedulerWeightCache)
	require.Equal(t, 0.22, gemini.GeminiAdaptiveSchedulerWeightCache)
	require.Equal(t, 0.33, openAI.OpenAIAdaptiveSchedulerWeightCache)
}

func TestAdaptiveSchedulerWeightCacheMergePreservesOmittedValues(t *testing.T) {
	anthropicPrevious := service.DefaultAnthropicAdaptiveSchedulerSettings()
	anthropicPrevious.AnthropicAdaptiveSchedulerWeightCache = 0.11
	geminiPrevious := service.DefaultGeminiAdaptiveSchedulerSettings()
	geminiPrevious.GeminiAdaptiveSchedulerWeightCache = 0.22
	openAIPrevious := service.DefaultOpenAIAdaptiveSchedulerSettings()
	openAIPrevious.OpenAIAdaptiveSchedulerWeightCache = 0.33

	anthropic := mergeAnthropicAdaptiveSchedulerSettings(anthropicPrevious, AnthropicAdaptiveSchedulerSettingsUpdateRequest{})
	gemini := mergeGeminiAdaptiveSchedulerSettings(geminiPrevious, GeminiAdaptiveSchedulerSettingsUpdateRequest{})
	openAI := mergeOpenAIAdaptiveSchedulerSettings(openAIPrevious, OpenAIAdaptiveSchedulerSettingsUpdateRequest{})

	require.Equal(t, 0.11, anthropic.AnthropicAdaptiveSchedulerWeightCache)
	require.Equal(t, 0.22, gemini.GeminiAdaptiveSchedulerWeightCache)
	require.Equal(t, 0.33, openAI.OpenAIAdaptiveSchedulerWeightCache)
}
