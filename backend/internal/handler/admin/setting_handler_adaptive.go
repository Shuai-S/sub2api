package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

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
