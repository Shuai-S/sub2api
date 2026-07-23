package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type AnthropicAdaptiveSchedulerSettingsUpdateRequest struct {
	AnthropicAdaptiveSchedulerEnabled                     *bool    `json:"anthropic_adaptive_scheduler_enabled"`
	AnthropicAdaptiveSchedulerMode                        *string  `json:"anthropic_adaptive_scheduler_mode"`
	AnthropicAdaptiveSchedulerTopK                        *int     `json:"anthropic_adaptive_scheduler_top_k"`
	AnthropicAdaptiveSchedulerSoftmaxTemperature          *float64 `json:"anthropic_adaptive_scheduler_softmax_temperature"`
	AnthropicAdaptiveSchedulerWeightReliability           *float64 `json:"anthropic_adaptive_scheduler_weight_reliability"`
	AnthropicAdaptiveSchedulerWeightCapacity              *float64 `json:"anthropic_adaptive_scheduler_weight_capacity"`
	AnthropicAdaptiveSchedulerWeightLatency               *float64 `json:"anthropic_adaptive_scheduler_weight_latency"`
	AnthropicAdaptiveSchedulerWeightExploration           *float64 `json:"anthropic_adaptive_scheduler_weight_exploration"`
	AnthropicAdaptiveSchedulerInitialReliability          *float64 `json:"anthropic_adaptive_scheduler_initial_reliability"`
	AnthropicAdaptiveSchedulerConsecutiveFailurePenalty   *float64 `json:"anthropic_adaptive_scheduler_consecutive_failure_penalty"`
	AnthropicAdaptiveSchedulerNeutralLatencyScore         *float64 `json:"anthropic_adaptive_scheduler_neutral_latency_score"`
	AnthropicAdaptiveSchedulerSuccessEMAAlpha             *float64 `json:"anthropic_adaptive_scheduler_success_ema_alpha"`
	AnthropicAdaptiveSchedulerLatencyEMAAlpha             *float64 `json:"anthropic_adaptive_scheduler_latency_ema_alpha"`
	AnthropicAdaptiveSchedulerCapacitySuccessThreshold    *float64 `json:"anthropic_adaptive_scheduler_capacity_success_threshold"`
	AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold  *float64 `json:"anthropic_adaptive_scheduler_capacity_probe_load_threshold"`
	AnthropicAdaptiveSchedulerCapacityFailureThreshold    *int     `json:"anthropic_adaptive_scheduler_capacity_failure_threshold"`
	AnthropicAdaptiveSchedulerMinRecentSamplesForShrink   *int     `json:"anthropic_adaptive_scheduler_min_recent_samples_for_shrink"`
	AnthropicAdaptiveSchedulerShrinkErrorThreshold        *float64 `json:"anthropic_adaptive_scheduler_shrink_error_threshold"`
	AnthropicAdaptiveSchedulerLearningWindowSeconds       *int     `json:"anthropic_adaptive_scheduler_learning_window_seconds"`
	AnthropicAdaptiveSchedulerCooldownSeconds             *int     `json:"anthropic_adaptive_scheduler_cooldown_seconds"`
	AnthropicAdaptiveSchedulerShrinkFactorSoft            *float64 `json:"anthropic_adaptive_scheduler_shrink_factor_soft"`
	AnthropicAdaptiveSchedulerShrinkFactorHard            *float64 `json:"anthropic_adaptive_scheduler_shrink_factor_hard"`
	AnthropicAdaptiveSchedulerCapacityIncreaseStep        *int     `json:"anthropic_adaptive_scheduler_capacity_increase_step"`
	AnthropicAdaptiveSchedulerMinCapacity                 *int     `json:"anthropic_adaptive_scheduler_min_capacity"`
	AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier *int     `json:"anthropic_adaptive_scheduler_hard_shrink_failure_multiplier"`
}

func mergeAnthropicAdaptiveSchedulerSettings(previous service.AnthropicAdaptiveSchedulerSettings, req AnthropicAdaptiveSchedulerSettingsUpdateRequest) service.AnthropicAdaptiveSchedulerSettings {
	settings := previous
	if req.AnthropicAdaptiveSchedulerEnabled != nil {
		settings.AnthropicAdaptiveSchedulerEnabled = *req.AnthropicAdaptiveSchedulerEnabled
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
	if req.AnthropicAdaptiveSchedulerWeightReliability != nil {
		settings.AnthropicAdaptiveSchedulerWeightReliability = *req.AnthropicAdaptiveSchedulerWeightReliability
	}
	if req.AnthropicAdaptiveSchedulerWeightCapacity != nil {
		settings.AnthropicAdaptiveSchedulerWeightCapacity = *req.AnthropicAdaptiveSchedulerWeightCapacity
	}
	if req.AnthropicAdaptiveSchedulerWeightLatency != nil {
		settings.AnthropicAdaptiveSchedulerWeightLatency = *req.AnthropicAdaptiveSchedulerWeightLatency
	}
	if req.AnthropicAdaptiveSchedulerWeightExploration != nil {
		settings.AnthropicAdaptiveSchedulerWeightExploration = *req.AnthropicAdaptiveSchedulerWeightExploration
	}
	if req.AnthropicAdaptiveSchedulerInitialReliability != nil {
		settings.AnthropicAdaptiveSchedulerInitialReliability = *req.AnthropicAdaptiveSchedulerInitialReliability
	}
	if req.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty != nil {
		settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty = *req.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty
	}
	if req.AnthropicAdaptiveSchedulerNeutralLatencyScore != nil {
		settings.AnthropicAdaptiveSchedulerNeutralLatencyScore = *req.AnthropicAdaptiveSchedulerNeutralLatencyScore
	}
	if req.AnthropicAdaptiveSchedulerSuccessEMAAlpha != nil {
		settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha = *req.AnthropicAdaptiveSchedulerSuccessEMAAlpha
	}
	if req.AnthropicAdaptiveSchedulerLatencyEMAAlpha != nil {
		settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha = *req.AnthropicAdaptiveSchedulerLatencyEMAAlpha
	}
	if req.AnthropicAdaptiveSchedulerCapacitySuccessThreshold != nil {
		settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold = *req.AnthropicAdaptiveSchedulerCapacitySuccessThreshold
	}
	if req.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold != nil {
		settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = *req.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold
	}
	if req.AnthropicAdaptiveSchedulerCapacityFailureThreshold != nil {
		settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold = *req.AnthropicAdaptiveSchedulerCapacityFailureThreshold
	}
	if req.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink != nil {
		settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink = *req.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink
	}
	if req.AnthropicAdaptiveSchedulerShrinkErrorThreshold != nil {
		settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold = *req.AnthropicAdaptiveSchedulerShrinkErrorThreshold
	}
	if req.AnthropicAdaptiveSchedulerLearningWindowSeconds != nil {
		settings.AnthropicAdaptiveSchedulerLearningWindowSeconds = *req.AnthropicAdaptiveSchedulerLearningWindowSeconds
	}
	if req.AnthropicAdaptiveSchedulerCooldownSeconds != nil {
		settings.AnthropicAdaptiveSchedulerCooldownSeconds = *req.AnthropicAdaptiveSchedulerCooldownSeconds
	}
	if req.AnthropicAdaptiveSchedulerShrinkFactorSoft != nil {
		settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = *req.AnthropicAdaptiveSchedulerShrinkFactorSoft
	}
	if req.AnthropicAdaptiveSchedulerShrinkFactorHard != nil {
		settings.AnthropicAdaptiveSchedulerShrinkFactorHard = *req.AnthropicAdaptiveSchedulerShrinkFactorHard
	}
	if req.AnthropicAdaptiveSchedulerCapacityIncreaseStep != nil {
		settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep = *req.AnthropicAdaptiveSchedulerCapacityIncreaseStep
	}
	if req.AnthropicAdaptiveSchedulerMinCapacity != nil {
		settings.AnthropicAdaptiveSchedulerMinCapacity = *req.AnthropicAdaptiveSchedulerMinCapacity
	}
	if req.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier != nil {
		settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier = *req.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier
	}
	return service.NormalizeAnthropicAdaptiveSchedulerSettings(settings)
}

type OpenAIAdaptiveSchedulerSettingsUpdateRequest struct {
	OpenAIAdaptiveSchedulerEnabled                    *bool    `json:"openai_adaptive_scheduler_enabled"`
	OpenAIAdaptiveSchedulerDiagnosticLogEnabled       *bool    `json:"openai_adaptive_scheduler_diagnostic_log_enabled"`
	OpenAIAdaptiveSchedulerDiagnosticLogSampleRate    *float64 `json:"openai_adaptive_scheduler_diagnostic_log_sample_rate"`
	OpenAIAdaptiveSchedulerMode                       *string  `json:"openai_adaptive_scheduler_mode"`
	OpenAIAdaptiveSchedulerAccountTypePriorityMode    *string  `json:"openai_adaptive_scheduler_account_type_priority_mode"`
	OpenAIAdaptiveSchedulerTopK                       *int     `json:"openai_adaptive_scheduler_top_k"`
	OpenAIAdaptiveSchedulerExplorationRate            *float64 `json:"openai_adaptive_scheduler_exploration_rate"`
	OpenAIAdaptiveSchedulerSoftmaxTemperature         *float64 `json:"openai_adaptive_scheduler_softmax_temperature"`
	OpenAIAdaptiveSchedulerMinCostMultiplier          *float64 `json:"openai_adaptive_scheduler_min_cost_multiplier"`
	OpenAIAdaptiveSchedulerThompsonEnabled            *bool    `json:"openai_adaptive_scheduler_thompson_enabled"`
	OpenAIAdaptiveSchedulerThompsonPriorAlpha         *float64 `json:"openai_adaptive_scheduler_thompson_prior_alpha"`
	OpenAIAdaptiveSchedulerThompsonPriorBeta          *float64 `json:"openai_adaptive_scheduler_thompson_prior_beta"`
	OpenAIAdaptiveSchedulerInitialCapacityFraction    *float64 `json:"openai_adaptive_scheduler_initial_capacity_fraction"`
	OpenAIAdaptiveSchedulerMinCapacity                *int     `json:"openai_adaptive_scheduler_min_capacity"`
	OpenAIAdaptiveSchedulerCapacityIncreaseStep       *int     `json:"openai_adaptive_scheduler_capacity_increase_step"`
	OpenAIAdaptiveSchedulerCapacityGrowthFactor       *float64 `json:"openai_adaptive_scheduler_capacity_growth_factor"`
	OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold *float64 `json:"openai_adaptive_scheduler_capacity_probe_load_threshold"`
	OpenAIAdaptiveSchedulerBurstProbeRatio            *float64 `json:"openai_adaptive_scheduler_burst_probe_ratio"`
	OpenAIAdaptiveSchedulerCapacitySuccessThreshold   *float64 `json:"openai_adaptive_scheduler_capacity_success_threshold"`
	OpenAIAdaptiveSchedulerCapacityFailureThreshold   *int     `json:"openai_adaptive_scheduler_capacity_failure_threshold"`
	OpenAIAdaptiveSchedulerMinRecentSamplesForShrink  *int     `json:"openai_adaptive_scheduler_min_recent_samples_for_shrink"`
	OpenAIAdaptiveSchedulerShrinkErrorThreshold       *float64 `json:"openai_adaptive_scheduler_shrink_error_threshold"`
	OpenAIAdaptiveSchedulerShrinkFactorSoft           *float64 `json:"openai_adaptive_scheduler_shrink_factor_soft"`
	OpenAIAdaptiveSchedulerShrinkFactorHard           *float64 `json:"openai_adaptive_scheduler_shrink_factor_hard"`
	OpenAIAdaptiveSchedulerHalfOpenFailureThreshold   *int     `json:"openai_adaptive_scheduler_half_open_failure_threshold"`
	OpenAIAdaptiveSchedulerHalfOpenProbeCapacity      *int     `json:"openai_adaptive_scheduler_half_open_probe_capacity"`
	OpenAIAdaptiveSchedulerLearningWindowSeconds      *int     `json:"openai_adaptive_scheduler_learning_window_seconds"`
	OpenAIAdaptiveSchedulerSuccessEMAAlpha            *float64 `json:"openai_adaptive_scheduler_success_ema_alpha"`
	OpenAIAdaptiveSchedulerErrorEMAAlpha              *float64 `json:"openai_adaptive_scheduler_error_ema_alpha"`
	OpenAIAdaptiveSchedulerLatencyEMAAlpha            *float64 `json:"openai_adaptive_scheduler_latency_ema_alpha"`
	OpenAIAdaptiveSchedulerTTFTEMAAlpha               *float64 `json:"openai_adaptive_scheduler_ttft_ema_alpha"`
	OpenAIAdaptiveSchedulerCooldownBaseSeconds        *int     `json:"openai_adaptive_scheduler_cooldown_base_seconds"`
	OpenAIAdaptiveSchedulerCooldownMaxSeconds         *int     `json:"openai_adaptive_scheduler_cooldown_max_seconds"`
	OpenAIAdaptiveSchedulerWeightSuccess              *float64 `json:"openai_adaptive_scheduler_weight_success"`
	OpenAIAdaptiveSchedulerWeightCost                 *float64 `json:"openai_adaptive_scheduler_weight_cost"`
	OpenAIAdaptiveSchedulerWeightCapacity             *float64 `json:"openai_adaptive_scheduler_weight_capacity"`
	OpenAIAdaptiveSchedulerWeightLatency              *float64 `json:"openai_adaptive_scheduler_weight_latency"`
	OpenAIAdaptiveSchedulerWeightStability            *float64 `json:"openai_adaptive_scheduler_weight_stability"`
	OpenAIAdaptiveSchedulerWeightExploration          *float64 `json:"openai_adaptive_scheduler_weight_exploration"`
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
	if req.OpenAIAdaptiveSchedulerAccountTypePriorityMode != nil {
		settings.OpenAIAdaptiveSchedulerAccountTypePriorityMode = strings.TrimSpace(*req.OpenAIAdaptiveSchedulerAccountTypePriorityMode)
	}
	if req.OpenAIAdaptiveSchedulerTopK != nil {
		settings.OpenAIAdaptiveSchedulerTopK = *req.OpenAIAdaptiveSchedulerTopK
	}
	if req.OpenAIAdaptiveSchedulerExplorationRate != nil {
		settings.OpenAIAdaptiveSchedulerExplorationRate = *req.OpenAIAdaptiveSchedulerExplorationRate
	}
	if req.OpenAIAdaptiveSchedulerSoftmaxTemperature != nil {
		settings.OpenAIAdaptiveSchedulerSoftmaxTemperature = *req.OpenAIAdaptiveSchedulerSoftmaxTemperature
	}
	if req.OpenAIAdaptiveSchedulerMinCostMultiplier != nil {
		settings.OpenAIAdaptiveSchedulerMinCostMultiplier = *req.OpenAIAdaptiveSchedulerMinCostMultiplier
	}
	if req.OpenAIAdaptiveSchedulerThompsonEnabled != nil {
		settings.OpenAIAdaptiveSchedulerThompsonEnabled = *req.OpenAIAdaptiveSchedulerThompsonEnabled
	}
	if req.OpenAIAdaptiveSchedulerThompsonPriorAlpha != nil {
		settings.OpenAIAdaptiveSchedulerThompsonPriorAlpha = *req.OpenAIAdaptiveSchedulerThompsonPriorAlpha
	}
	if req.OpenAIAdaptiveSchedulerThompsonPriorBeta != nil {
		settings.OpenAIAdaptiveSchedulerThompsonPriorBeta = *req.OpenAIAdaptiveSchedulerThompsonPriorBeta
	}
	if req.OpenAIAdaptiveSchedulerInitialCapacityFraction != nil {
		settings.OpenAIAdaptiveSchedulerInitialCapacityFraction = *req.OpenAIAdaptiveSchedulerInitialCapacityFraction
	}
	if req.OpenAIAdaptiveSchedulerMinCapacity != nil {
		settings.OpenAIAdaptiveSchedulerMinCapacity = *req.OpenAIAdaptiveSchedulerMinCapacity
	}
	if req.OpenAIAdaptiveSchedulerCapacityIncreaseStep != nil {
		settings.OpenAIAdaptiveSchedulerCapacityIncreaseStep = *req.OpenAIAdaptiveSchedulerCapacityIncreaseStep
	}
	if req.OpenAIAdaptiveSchedulerCapacityGrowthFactor != nil {
		settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor = *req.OpenAIAdaptiveSchedulerCapacityGrowthFactor
	}
	if req.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold != nil {
		settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold = *req.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold
	}
	if req.OpenAIAdaptiveSchedulerBurstProbeRatio != nil {
		settings.OpenAIAdaptiveSchedulerBurstProbeRatio = *req.OpenAIAdaptiveSchedulerBurstProbeRatio
	}
	if req.OpenAIAdaptiveSchedulerCapacitySuccessThreshold != nil {
		settings.OpenAIAdaptiveSchedulerCapacitySuccessThreshold = *req.OpenAIAdaptiveSchedulerCapacitySuccessThreshold
	}
	if req.OpenAIAdaptiveSchedulerCapacityFailureThreshold != nil {
		settings.OpenAIAdaptiveSchedulerCapacityFailureThreshold = *req.OpenAIAdaptiveSchedulerCapacityFailureThreshold
	}
	if req.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink != nil {
		settings.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink = *req.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink
	}
	if req.OpenAIAdaptiveSchedulerShrinkErrorThreshold != nil {
		settings.OpenAIAdaptiveSchedulerShrinkErrorThreshold = *req.OpenAIAdaptiveSchedulerShrinkErrorThreshold
	}
	if req.OpenAIAdaptiveSchedulerShrinkFactorSoft != nil {
		settings.OpenAIAdaptiveSchedulerShrinkFactorSoft = *req.OpenAIAdaptiveSchedulerShrinkFactorSoft
	}
	if req.OpenAIAdaptiveSchedulerShrinkFactorHard != nil {
		settings.OpenAIAdaptiveSchedulerShrinkFactorHard = *req.OpenAIAdaptiveSchedulerShrinkFactorHard
	}
	if req.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold != nil {
		settings.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold = *req.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold
	}
	if req.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity != nil {
		settings.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity = *req.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity
	}
	if req.OpenAIAdaptiveSchedulerLearningWindowSeconds != nil {
		settings.OpenAIAdaptiveSchedulerLearningWindowSeconds = *req.OpenAIAdaptiveSchedulerLearningWindowSeconds
	}
	if req.OpenAIAdaptiveSchedulerSuccessEMAAlpha != nil {
		settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha = *req.OpenAIAdaptiveSchedulerSuccessEMAAlpha
	}
	if req.OpenAIAdaptiveSchedulerErrorEMAAlpha != nil {
		settings.OpenAIAdaptiveSchedulerErrorEMAAlpha = *req.OpenAIAdaptiveSchedulerErrorEMAAlpha
	}
	if req.OpenAIAdaptiveSchedulerLatencyEMAAlpha != nil {
		settings.OpenAIAdaptiveSchedulerLatencyEMAAlpha = *req.OpenAIAdaptiveSchedulerLatencyEMAAlpha
	}
	if req.OpenAIAdaptiveSchedulerTTFTEMAAlpha != nil {
		settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha = *req.OpenAIAdaptiveSchedulerTTFTEMAAlpha
	}
	if req.OpenAIAdaptiveSchedulerCooldownBaseSeconds != nil {
		settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds = *req.OpenAIAdaptiveSchedulerCooldownBaseSeconds
	}
	if req.OpenAIAdaptiveSchedulerCooldownMaxSeconds != nil {
		settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds = *req.OpenAIAdaptiveSchedulerCooldownMaxSeconds
	}
	if req.OpenAIAdaptiveSchedulerWeightSuccess != nil {
		settings.OpenAIAdaptiveSchedulerWeightSuccess = *req.OpenAIAdaptiveSchedulerWeightSuccess
	}
	if req.OpenAIAdaptiveSchedulerWeightCost != nil {
		settings.OpenAIAdaptiveSchedulerWeightCost = *req.OpenAIAdaptiveSchedulerWeightCost
	}
	if req.OpenAIAdaptiveSchedulerWeightCapacity != nil {
		settings.OpenAIAdaptiveSchedulerWeightCapacity = *req.OpenAIAdaptiveSchedulerWeightCapacity
	}
	if req.OpenAIAdaptiveSchedulerWeightLatency != nil {
		settings.OpenAIAdaptiveSchedulerWeightLatency = *req.OpenAIAdaptiveSchedulerWeightLatency
	}
	if req.OpenAIAdaptiveSchedulerWeightStability != nil {
		settings.OpenAIAdaptiveSchedulerWeightStability = *req.OpenAIAdaptiveSchedulerWeightStability
	}
	if req.OpenAIAdaptiveSchedulerWeightExploration != nil {
		settings.OpenAIAdaptiveSchedulerWeightExploration = *req.OpenAIAdaptiveSchedulerWeightExploration
	}
	return service.NormalizeOpenAIAdaptiveSchedulerSettings(settings)
}
