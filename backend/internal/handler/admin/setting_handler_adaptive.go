package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type AnthropicAdaptiveSchedulerSettingsUpdateRequest struct {
	AnthropicAdaptiveSchedulerEnabled                    *bool    `json:"anthropic_adaptive_scheduler_enabled"`
	AnthropicAdaptiveSchedulerDiagnosticLogEnabled       *bool    `json:"anthropic_adaptive_scheduler_diagnostic_log_enabled"`
	AnthropicAdaptiveSchedulerDiagnosticLogSampleRate    *float64 `json:"anthropic_adaptive_scheduler_diagnostic_log_sample_rate"`
	AnthropicAdaptiveSchedulerMode                       *string  `json:"anthropic_adaptive_scheduler_mode"`
	AnthropicAdaptiveSchedulerTopK                       *int     `json:"anthropic_adaptive_scheduler_top_k"`
	AnthropicAdaptiveSchedulerSoftmaxTemperature         *float64 `json:"anthropic_adaptive_scheduler_softmax_temperature"`
	AnthropicAdaptiveSchedulerExplorationRate            *float64 `json:"anthropic_adaptive_scheduler_exploration_rate"`
	AnthropicAdaptiveSchedulerConsecutiveFailurePenalty  *float64 `json:"anthropic_adaptive_scheduler_consecutive_failure_penalty"`
	AnthropicAdaptiveSchedulerLearningWindowSeconds      *int     `json:"anthropic_adaptive_scheduler_learning_window_seconds"`
	AnthropicAdaptiveSchedulerLearningMinHealthSamples   *int     `json:"anthropic_adaptive_scheduler_learning_min_health_samples"`
	AnthropicAdaptiveSchedulerSuccessEMAAlpha            *float64 `json:"anthropic_adaptive_scheduler_success_ema_alpha"`
	AnthropicAdaptiveSchedulerLatencyEMAAlpha            *float64 `json:"anthropic_adaptive_scheduler_latency_ema_alpha"`
	AnthropicAdaptiveSchedulerHealthFailureThreshold     *int     `json:"anthropic_adaptive_scheduler_health_failure_threshold"`
	AnthropicAdaptiveSchedulerCooldownSeconds            *int     `json:"anthropic_adaptive_scheduler_cooldown_seconds"`
	AnthropicAdaptiveSchedulerCooldownMaxSeconds         *int     `json:"anthropic_adaptive_scheduler_cooldown_max_seconds"`
	AnthropicAdaptiveSchedulerHighErrorMinSamples        *int     `json:"anthropic_adaptive_scheduler_high_error_min_samples"`
	AnthropicAdaptiveSchedulerHighErrorMaxSamples        *int     `json:"anthropic_adaptive_scheduler_high_error_max_samples"`
	AnthropicAdaptiveSchedulerHighErrorEnterRate         *float64 `json:"anthropic_adaptive_scheduler_high_error_enter_rate"`
	AnthropicAdaptiveSchedulerHighErrorExitRate          *float64 `json:"anthropic_adaptive_scheduler_high_error_exit_rate"`
	AnthropicAdaptiveSchedulerShrinkFactorSoft           *float64 `json:"anthropic_adaptive_scheduler_shrink_factor_soft"`
	AnthropicAdaptiveSchedulerCapacityGrowthFactor       *float64 `json:"anthropic_adaptive_scheduler_capacity_growth_factor"`
	AnthropicAdaptiveSchedulerCapacityRecoverySamples    *int     `json:"anthropic_adaptive_scheduler_capacity_recovery_samples"`
	AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold *float64 `json:"anthropic_adaptive_scheduler_capacity_probe_load_threshold"`
	AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds  *int     `json:"anthropic_adaptive_scheduler_quota_probe_interval_seconds"`
	AnthropicAdaptiveSchedulerWeightReliability          *float64 `json:"anthropic_adaptive_scheduler_weight_reliability"`
	AnthropicAdaptiveSchedulerWeightCapacity             *float64 `json:"anthropic_adaptive_scheduler_weight_capacity"`
	AnthropicAdaptiveSchedulerWeightLatency              *float64 `json:"anthropic_adaptive_scheduler_weight_latency"`
	AnthropicAdaptiveSchedulerWeightCost                 *float64 `json:"anthropic_adaptive_scheduler_weight_cost"`
}

func mergeAnthropicAdaptiveSchedulerSettings(previous service.AnthropicAdaptiveSchedulerSettings, req AnthropicAdaptiveSchedulerSettingsUpdateRequest) service.AnthropicAdaptiveSchedulerSettings {
	settings := previous
	if req.AnthropicAdaptiveSchedulerEnabled != nil {
		settings.AnthropicAdaptiveSchedulerEnabled = *req.AnthropicAdaptiveSchedulerEnabled
	}
	if req.AnthropicAdaptiveSchedulerDiagnosticLogEnabled != nil {
		settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = *req.AnthropicAdaptiveSchedulerDiagnosticLogEnabled
	}
	if req.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate != nil {
		settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = *req.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate
	}
	if req.AnthropicAdaptiveSchedulerMode != nil {
		settings.AnthropicAdaptiveSchedulerMode = strings.TrimSpace(*req.AnthropicAdaptiveSchedulerMode)
	}
	if req.AnthropicAdaptiveSchedulerTopK != nil {
		settings.AnthropicAdaptiveSchedulerTopK = *req.AnthropicAdaptiveSchedulerTopK
	}
	if req.AnthropicAdaptiveSchedulerSoftmaxTemperature != nil {
		settings.AnthropicAdaptiveSchedulerSoftmaxTemperature = *req.AnthropicAdaptiveSchedulerSoftmaxTemperature
	}
	if req.AnthropicAdaptiveSchedulerExplorationRate != nil {
		settings.AnthropicAdaptiveSchedulerExplorationRate = *req.AnthropicAdaptiveSchedulerExplorationRate
	}
	if req.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty != nil {
		settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty = *req.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty
	}
	if req.AnthropicAdaptiveSchedulerLearningWindowSeconds != nil {
		settings.AnthropicAdaptiveSchedulerLearningWindowSeconds = *req.AnthropicAdaptiveSchedulerLearningWindowSeconds
	}
	if req.AnthropicAdaptiveSchedulerLearningMinHealthSamples != nil {
		settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples = *req.AnthropicAdaptiveSchedulerLearningMinHealthSamples
	}
	if req.AnthropicAdaptiveSchedulerSuccessEMAAlpha != nil {
		settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha = *req.AnthropicAdaptiveSchedulerSuccessEMAAlpha
	}
	if req.AnthropicAdaptiveSchedulerLatencyEMAAlpha != nil {
		settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha = *req.AnthropicAdaptiveSchedulerLatencyEMAAlpha
	}
	if req.AnthropicAdaptiveSchedulerHealthFailureThreshold != nil {
		settings.AnthropicAdaptiveSchedulerHealthFailureThreshold = *req.AnthropicAdaptiveSchedulerHealthFailureThreshold
	}
	if req.AnthropicAdaptiveSchedulerCooldownSeconds != nil {
		settings.AnthropicAdaptiveSchedulerCooldownSeconds = *req.AnthropicAdaptiveSchedulerCooldownSeconds
	}
	if req.AnthropicAdaptiveSchedulerCooldownMaxSeconds != nil {
		settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds = *req.AnthropicAdaptiveSchedulerCooldownMaxSeconds
	}
	if req.AnthropicAdaptiveSchedulerHighErrorMinSamples != nil {
		settings.AnthropicAdaptiveSchedulerHighErrorMinSamples = *req.AnthropicAdaptiveSchedulerHighErrorMinSamples
	}
	if req.AnthropicAdaptiveSchedulerHighErrorMaxSamples != nil {
		settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples = *req.AnthropicAdaptiveSchedulerHighErrorMaxSamples
	}
	if req.AnthropicAdaptiveSchedulerHighErrorEnterRate != nil {
		settings.AnthropicAdaptiveSchedulerHighErrorEnterRate = *req.AnthropicAdaptiveSchedulerHighErrorEnterRate
	}
	if req.AnthropicAdaptiveSchedulerHighErrorExitRate != nil {
		settings.AnthropicAdaptiveSchedulerHighErrorExitRate = *req.AnthropicAdaptiveSchedulerHighErrorExitRate
	}
	if req.AnthropicAdaptiveSchedulerShrinkFactorSoft != nil {
		settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = *req.AnthropicAdaptiveSchedulerShrinkFactorSoft
	}
	if req.AnthropicAdaptiveSchedulerCapacityGrowthFactor != nil {
		settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor = *req.AnthropicAdaptiveSchedulerCapacityGrowthFactor
	}
	if req.AnthropicAdaptiveSchedulerCapacityRecoverySamples != nil {
		settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples = *req.AnthropicAdaptiveSchedulerCapacityRecoverySamples
	}
	if req.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold != nil {
		settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = *req.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold
	}
	if req.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds != nil {
		settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds = *req.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds
	}
	if req.AnthropicAdaptiveSchedulerWeightReliability != nil {
		settings.AnthropicAdaptiveSchedulerWeightReliability = *req.AnthropicAdaptiveSchedulerWeightReliability
	}
	if req.AnthropicAdaptiveSchedulerWeightCapacity != nil {
		settings.AnthropicAdaptiveSchedulerWeightCapacity = *req.AnthropicAdaptiveSchedulerWeightCapacity
	}
	if req.AnthropicAdaptiveSchedulerWeightLatency != nil {
		settings.AnthropicAdaptiveSchedulerWeightLatency = *req.AnthropicAdaptiveSchedulerWeightLatency
	}
	if req.AnthropicAdaptiveSchedulerWeightCost != nil {
		settings.AnthropicAdaptiveSchedulerWeightCost = *req.AnthropicAdaptiveSchedulerWeightCost
	}
	return service.NormalizeAnthropicAdaptiveSchedulerSettings(settings)
}

type GeminiAdaptiveSchedulerSettingsUpdateRequest struct {
	GeminiAdaptiveSchedulerEnabled                    *bool    `json:"gemini_adaptive_scheduler_enabled"`
	GeminiAdaptiveSchedulerDiagnosticLogEnabled       *bool    `json:"gemini_adaptive_scheduler_diagnostic_log_enabled"`
	GeminiAdaptiveSchedulerDiagnosticLogSampleRate    *float64 `json:"gemini_adaptive_scheduler_diagnostic_log_sample_rate"`
	GeminiAdaptiveSchedulerMode                       *string  `json:"gemini_adaptive_scheduler_mode"`
	GeminiAdaptiveSchedulerTopK                       *int     `json:"gemini_adaptive_scheduler_top_k"`
	GeminiAdaptiveSchedulerSoftmaxTemperature         *float64 `json:"gemini_adaptive_scheduler_softmax_temperature"`
	GeminiAdaptiveSchedulerExplorationRate            *float64 `json:"gemini_adaptive_scheduler_exploration_rate"`
	GeminiAdaptiveSchedulerConsecutiveFailurePenalty  *float64 `json:"gemini_adaptive_scheduler_consecutive_failure_penalty"`
	GeminiAdaptiveSchedulerLearningWindowSeconds      *int     `json:"gemini_adaptive_scheduler_learning_window_seconds"`
	GeminiAdaptiveSchedulerLearningMinHealthSamples   *int     `json:"gemini_adaptive_scheduler_learning_min_health_samples"`
	GeminiAdaptiveSchedulerSuccessEMAAlpha            *float64 `json:"gemini_adaptive_scheduler_success_ema_alpha"`
	GeminiAdaptiveSchedulerLatencyEMAAlpha            *float64 `json:"gemini_adaptive_scheduler_latency_ema_alpha"`
	GeminiAdaptiveSchedulerAccountFailureThreshold    *int     `json:"gemini_adaptive_scheduler_account_failure_threshold"`
	GeminiAdaptiveSchedulerCooldownSeconds            *int     `json:"gemini_adaptive_scheduler_cooldown_seconds"`
	GeminiAdaptiveSchedulerCooldownMaxSeconds         *int     `json:"gemini_adaptive_scheduler_cooldown_max_seconds"`
	GeminiAdaptiveSchedulerHighErrorMinSamples        *int     `json:"gemini_adaptive_scheduler_high_error_min_samples"`
	GeminiAdaptiveSchedulerHighErrorMaxSamples        *int     `json:"gemini_adaptive_scheduler_high_error_max_samples"`
	GeminiAdaptiveSchedulerHighErrorEnterRate         *float64 `json:"gemini_adaptive_scheduler_high_error_enter_rate"`
	GeminiAdaptiveSchedulerHighErrorExitRate          *float64 `json:"gemini_adaptive_scheduler_high_error_exit_rate"`
	GeminiAdaptiveSchedulerShrinkFactorSoft           *float64 `json:"gemini_adaptive_scheduler_shrink_factor_soft"`
	GeminiAdaptiveSchedulerCapacityGrowthFactor       *float64 `json:"gemini_adaptive_scheduler_capacity_growth_factor"`
	GeminiAdaptiveSchedulerCapacityRecoverySamples    *int     `json:"gemini_adaptive_scheduler_capacity_recovery_samples"`
	GeminiAdaptiveSchedulerCapacityProbeLoadThreshold *float64 `json:"gemini_adaptive_scheduler_capacity_probe_load_threshold"`
	GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds  *int     `json:"gemini_adaptive_scheduler_quota_probe_interval_seconds"`
	GeminiAdaptiveSchedulerWeightReliability          *float64 `json:"gemini_adaptive_scheduler_weight_reliability"`
	GeminiAdaptiveSchedulerWeightCapacity             *float64 `json:"gemini_adaptive_scheduler_weight_capacity"`
	GeminiAdaptiveSchedulerWeightLatency              *float64 `json:"gemini_adaptive_scheduler_weight_latency"`
	GeminiAdaptiveSchedulerWeightCost                 *float64 `json:"gemini_adaptive_scheduler_weight_cost"`
}

func mergeGeminiAdaptiveSchedulerSettings(previous service.GeminiAdaptiveSchedulerSettings, req GeminiAdaptiveSchedulerSettingsUpdateRequest) service.GeminiAdaptiveSchedulerSettings {
	settings := previous
	if req.GeminiAdaptiveSchedulerEnabled != nil {
		settings.GeminiAdaptiveSchedulerEnabled = *req.GeminiAdaptiveSchedulerEnabled
	}
	if req.GeminiAdaptiveSchedulerDiagnosticLogEnabled != nil {
		settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled = *req.GeminiAdaptiveSchedulerDiagnosticLogEnabled
	}
	if req.GeminiAdaptiveSchedulerDiagnosticLogSampleRate != nil {
		settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = *req.GeminiAdaptiveSchedulerDiagnosticLogSampleRate
	}
	if req.GeminiAdaptiveSchedulerMode != nil {
		settings.GeminiAdaptiveSchedulerMode = strings.TrimSpace(*req.GeminiAdaptiveSchedulerMode)
	}
	if req.GeminiAdaptiveSchedulerTopK != nil {
		settings.GeminiAdaptiveSchedulerTopK = *req.GeminiAdaptiveSchedulerTopK
	}
	if req.GeminiAdaptiveSchedulerSoftmaxTemperature != nil {
		settings.GeminiAdaptiveSchedulerSoftmaxTemperature = *req.GeminiAdaptiveSchedulerSoftmaxTemperature
	}
	if req.GeminiAdaptiveSchedulerExplorationRate != nil {
		settings.GeminiAdaptiveSchedulerExplorationRate = *req.GeminiAdaptiveSchedulerExplorationRate
	}
	if req.GeminiAdaptiveSchedulerConsecutiveFailurePenalty != nil {
		settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty = *req.GeminiAdaptiveSchedulerConsecutiveFailurePenalty
	}
	if req.GeminiAdaptiveSchedulerLearningWindowSeconds != nil {
		settings.GeminiAdaptiveSchedulerLearningWindowSeconds = *req.GeminiAdaptiveSchedulerLearningWindowSeconds
	}
	if req.GeminiAdaptiveSchedulerLearningMinHealthSamples != nil {
		settings.GeminiAdaptiveSchedulerLearningMinHealthSamples = *req.GeminiAdaptiveSchedulerLearningMinHealthSamples
	}
	if req.GeminiAdaptiveSchedulerSuccessEMAAlpha != nil {
		settings.GeminiAdaptiveSchedulerSuccessEMAAlpha = *req.GeminiAdaptiveSchedulerSuccessEMAAlpha
	}
	if req.GeminiAdaptiveSchedulerLatencyEMAAlpha != nil {
		settings.GeminiAdaptiveSchedulerLatencyEMAAlpha = *req.GeminiAdaptiveSchedulerLatencyEMAAlpha
	}
	if req.GeminiAdaptiveSchedulerAccountFailureThreshold != nil {
		settings.GeminiAdaptiveSchedulerAccountFailureThreshold = *req.GeminiAdaptiveSchedulerAccountFailureThreshold
	}
	if req.GeminiAdaptiveSchedulerCooldownSeconds != nil {
		settings.GeminiAdaptiveSchedulerCooldownSeconds = *req.GeminiAdaptiveSchedulerCooldownSeconds
	}
	if req.GeminiAdaptiveSchedulerCooldownMaxSeconds != nil {
		settings.GeminiAdaptiveSchedulerCooldownMaxSeconds = *req.GeminiAdaptiveSchedulerCooldownMaxSeconds
	}
	if req.GeminiAdaptiveSchedulerHighErrorMinSamples != nil {
		settings.GeminiAdaptiveSchedulerHighErrorMinSamples = *req.GeminiAdaptiveSchedulerHighErrorMinSamples
	}
	if req.GeminiAdaptiveSchedulerHighErrorMaxSamples != nil {
		settings.GeminiAdaptiveSchedulerHighErrorMaxSamples = *req.GeminiAdaptiveSchedulerHighErrorMaxSamples
	}
	if req.GeminiAdaptiveSchedulerHighErrorEnterRate != nil {
		settings.GeminiAdaptiveSchedulerHighErrorEnterRate = *req.GeminiAdaptiveSchedulerHighErrorEnterRate
	}
	if req.GeminiAdaptiveSchedulerHighErrorExitRate != nil {
		settings.GeminiAdaptiveSchedulerHighErrorExitRate = *req.GeminiAdaptiveSchedulerHighErrorExitRate
	}
	if req.GeminiAdaptiveSchedulerShrinkFactorSoft != nil {
		settings.GeminiAdaptiveSchedulerShrinkFactorSoft = *req.GeminiAdaptiveSchedulerShrinkFactorSoft
	}
	if req.GeminiAdaptiveSchedulerCapacityGrowthFactor != nil {
		settings.GeminiAdaptiveSchedulerCapacityGrowthFactor = *req.GeminiAdaptiveSchedulerCapacityGrowthFactor
	}
	if req.GeminiAdaptiveSchedulerCapacityRecoverySamples != nil {
		settings.GeminiAdaptiveSchedulerCapacityRecoverySamples = *req.GeminiAdaptiveSchedulerCapacityRecoverySamples
	}
	if req.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold != nil {
		settings.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold = *req.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold
	}
	if req.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds != nil {
		settings.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds = *req.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds
	}
	if req.GeminiAdaptiveSchedulerWeightReliability != nil {
		settings.GeminiAdaptiveSchedulerWeightReliability = *req.GeminiAdaptiveSchedulerWeightReliability
	}
	if req.GeminiAdaptiveSchedulerWeightCapacity != nil {
		settings.GeminiAdaptiveSchedulerWeightCapacity = *req.GeminiAdaptiveSchedulerWeightCapacity
	}
	if req.GeminiAdaptiveSchedulerWeightLatency != nil {
		settings.GeminiAdaptiveSchedulerWeightLatency = *req.GeminiAdaptiveSchedulerWeightLatency
	}
	if req.GeminiAdaptiveSchedulerWeightCost != nil {
		settings.GeminiAdaptiveSchedulerWeightCost = *req.GeminiAdaptiveSchedulerWeightCost
	}
	return service.NormalizeGeminiAdaptiveSchedulerSettings(settings)
}

type OpenAIAdaptiveSchedulerSettingsUpdateRequest struct {
	OpenAIAdaptiveSchedulerEnabled                    *bool    `json:"openai_adaptive_scheduler_enabled"`
	OpenAIAdaptiveSchedulerDiagnosticLogEnabled       *bool    `json:"openai_adaptive_scheduler_diagnostic_log_enabled"`
	OpenAIAdaptiveSchedulerDiagnosticLogSampleRate    *float64 `json:"openai_adaptive_scheduler_diagnostic_log_sample_rate"`
	OpenAIAdaptiveSchedulerMode                       *string  `json:"openai_adaptive_scheduler_mode"`
	OpenAIAdaptiveSchedulerTopK                       *int     `json:"openai_adaptive_scheduler_top_k"`
	OpenAIAdaptiveSchedulerSoftmaxTemperature         *float64 `json:"openai_adaptive_scheduler_softmax_temperature"`
	OpenAIAdaptiveSchedulerExplorationRate            *float64 `json:"openai_adaptive_scheduler_exploration_rate"`
	OpenAIAdaptiveSchedulerConsecutiveFailurePenalty  *float64 `json:"openai_adaptive_scheduler_consecutive_failure_penalty"`
	OpenAIAdaptiveSchedulerLearningWindowSeconds      *int     `json:"openai_adaptive_scheduler_learning_window_seconds"`
	OpenAIAdaptiveSchedulerLearningMinHealthSamples   *int     `json:"openai_adaptive_scheduler_learning_min_health_samples"`
	OpenAIAdaptiveSchedulerSuccessEMAAlpha            *float64 `json:"openai_adaptive_scheduler_success_ema_alpha"`
	OpenAIAdaptiveSchedulerTTFTEMAAlpha               *float64 `json:"openai_adaptive_scheduler_ttft_ema_alpha"`
	OpenAIAdaptiveSchedulerHealthFailureThreshold     *int     `json:"openai_adaptive_scheduler_health_failure_threshold"`
	OpenAIAdaptiveSchedulerCooldownBaseSeconds        *int     `json:"openai_adaptive_scheduler_cooldown_base_seconds"`
	OpenAIAdaptiveSchedulerCooldownMaxSeconds         *int     `json:"openai_adaptive_scheduler_cooldown_max_seconds"`
	OpenAIAdaptiveSchedulerHighErrorMinSamples        *int     `json:"openai_adaptive_scheduler_high_error_min_samples"`
	OpenAIAdaptiveSchedulerHighErrorMaxSamples        *int     `json:"openai_adaptive_scheduler_high_error_max_samples"`
	OpenAIAdaptiveSchedulerHighErrorEnterRate         *float64 `json:"openai_adaptive_scheduler_high_error_enter_rate"`
	OpenAIAdaptiveSchedulerHighErrorExitRate          *float64 `json:"openai_adaptive_scheduler_high_error_exit_rate"`
	OpenAIAdaptiveSchedulerShrinkFactorSoft           *float64 `json:"openai_adaptive_scheduler_shrink_factor_soft"`
	OpenAIAdaptiveSchedulerCapacityGrowthFactor       *float64 `json:"openai_adaptive_scheduler_capacity_growth_factor"`
	OpenAIAdaptiveSchedulerCapacityRecoverySamples    *int     `json:"openai_adaptive_scheduler_capacity_recovery_samples"`
	OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold *float64 `json:"openai_adaptive_scheduler_capacity_probe_load_threshold"`
	OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds  *int     `json:"openai_adaptive_scheduler_quota_probe_interval_seconds"`
	OpenAIAdaptiveSchedulerWeightSuccess              *float64 `json:"openai_adaptive_scheduler_weight_success"`
	OpenAIAdaptiveSchedulerWeightCapacity             *float64 `json:"openai_adaptive_scheduler_weight_capacity"`
	OpenAIAdaptiveSchedulerWeightLatency              *float64 `json:"openai_adaptive_scheduler_weight_latency"`
	OpenAIAdaptiveSchedulerWeightCost                 *float64 `json:"openai_adaptive_scheduler_weight_cost"`
}

func mergeOpenAIAdaptiveSchedulerSettings(previous service.OpenAIAdaptiveSchedulerSettings, req OpenAIAdaptiveSchedulerSettingsUpdateRequest) service.OpenAIAdaptiveSchedulerSettings {
	settings := previous
	if req.OpenAIAdaptiveSchedulerEnabled != nil {
		settings.OpenAIAdaptiveSchedulerEnabled = *req.OpenAIAdaptiveSchedulerEnabled
	}
	if req.OpenAIAdaptiveSchedulerDiagnosticLogEnabled != nil {
		settings.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = *req.OpenAIAdaptiveSchedulerDiagnosticLogEnabled
	}
	if req.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate != nil {
		settings.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = *req.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate
	}
	if req.OpenAIAdaptiveSchedulerMode != nil {
		settings.OpenAIAdaptiveSchedulerMode = strings.TrimSpace(*req.OpenAIAdaptiveSchedulerMode)
	}
	if req.OpenAIAdaptiveSchedulerTopK != nil {
		settings.OpenAIAdaptiveSchedulerTopK = *req.OpenAIAdaptiveSchedulerTopK
	}
	if req.OpenAIAdaptiveSchedulerSoftmaxTemperature != nil {
		settings.OpenAIAdaptiveSchedulerSoftmaxTemperature = *req.OpenAIAdaptiveSchedulerSoftmaxTemperature
	}
	if req.OpenAIAdaptiveSchedulerExplorationRate != nil {
		settings.OpenAIAdaptiveSchedulerExplorationRate = *req.OpenAIAdaptiveSchedulerExplorationRate
	}
	if req.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty != nil {
		settings.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty = *req.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty
	}
	if req.OpenAIAdaptiveSchedulerLearningWindowSeconds != nil {
		settings.OpenAIAdaptiveSchedulerLearningWindowSeconds = *req.OpenAIAdaptiveSchedulerLearningWindowSeconds
	}
	if req.OpenAIAdaptiveSchedulerLearningMinHealthSamples != nil {
		settings.OpenAIAdaptiveSchedulerLearningMinHealthSamples = *req.OpenAIAdaptiveSchedulerLearningMinHealthSamples
	}
	if req.OpenAIAdaptiveSchedulerSuccessEMAAlpha != nil {
		settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha = *req.OpenAIAdaptiveSchedulerSuccessEMAAlpha
	}
	if req.OpenAIAdaptiveSchedulerTTFTEMAAlpha != nil {
		settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha = *req.OpenAIAdaptiveSchedulerTTFTEMAAlpha
	}
	if req.OpenAIAdaptiveSchedulerHealthFailureThreshold != nil {
		settings.OpenAIAdaptiveSchedulerHealthFailureThreshold = *req.OpenAIAdaptiveSchedulerHealthFailureThreshold
	}
	if req.OpenAIAdaptiveSchedulerCooldownBaseSeconds != nil {
		settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds = *req.OpenAIAdaptiveSchedulerCooldownBaseSeconds
	}
	if req.OpenAIAdaptiveSchedulerCooldownMaxSeconds != nil {
		settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds = *req.OpenAIAdaptiveSchedulerCooldownMaxSeconds
	}
	if req.OpenAIAdaptiveSchedulerHighErrorMinSamples != nil {
		settings.OpenAIAdaptiveSchedulerHighErrorMinSamples = *req.OpenAIAdaptiveSchedulerHighErrorMinSamples
	}
	if req.OpenAIAdaptiveSchedulerHighErrorMaxSamples != nil {
		settings.OpenAIAdaptiveSchedulerHighErrorMaxSamples = *req.OpenAIAdaptiveSchedulerHighErrorMaxSamples
	}
	if req.OpenAIAdaptiveSchedulerHighErrorEnterRate != nil {
		settings.OpenAIAdaptiveSchedulerHighErrorEnterRate = *req.OpenAIAdaptiveSchedulerHighErrorEnterRate
	}
	if req.OpenAIAdaptiveSchedulerHighErrorExitRate != nil {
		settings.OpenAIAdaptiveSchedulerHighErrorExitRate = *req.OpenAIAdaptiveSchedulerHighErrorExitRate
	}
	if req.OpenAIAdaptiveSchedulerShrinkFactorSoft != nil {
		settings.OpenAIAdaptiveSchedulerShrinkFactorSoft = *req.OpenAIAdaptiveSchedulerShrinkFactorSoft
	}
	if req.OpenAIAdaptiveSchedulerCapacityGrowthFactor != nil {
		settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor = *req.OpenAIAdaptiveSchedulerCapacityGrowthFactor
	}
	if req.OpenAIAdaptiveSchedulerCapacityRecoverySamples != nil {
		settings.OpenAIAdaptiveSchedulerCapacityRecoverySamples = *req.OpenAIAdaptiveSchedulerCapacityRecoverySamples
	}
	if req.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold != nil {
		settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold = *req.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold
	}
	if req.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds != nil {
		settings.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds = *req.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds
	}
	if req.OpenAIAdaptiveSchedulerWeightSuccess != nil {
		settings.OpenAIAdaptiveSchedulerWeightSuccess = *req.OpenAIAdaptiveSchedulerWeightSuccess
	}
	if req.OpenAIAdaptiveSchedulerWeightCapacity != nil {
		settings.OpenAIAdaptiveSchedulerWeightCapacity = *req.OpenAIAdaptiveSchedulerWeightCapacity
	}
	if req.OpenAIAdaptiveSchedulerWeightLatency != nil {
		settings.OpenAIAdaptiveSchedulerWeightLatency = *req.OpenAIAdaptiveSchedulerWeightLatency
	}
	if req.OpenAIAdaptiveSchedulerWeightCost != nil {
		settings.OpenAIAdaptiveSchedulerWeightCost = *req.OpenAIAdaptiveSchedulerWeightCost
	}
	return service.NormalizeOpenAIAdaptiveSchedulerSettings(settings)
}
