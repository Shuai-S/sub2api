package service

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	openAIAdaptiveSchedulerModeShadow  = "shadow"
	openAIAdaptiveSchedulerModeEnforce = "enforce"
)

const (
	openAIAdaptiveSchedulerSettingPrefix = "openai_adaptive_scheduler_"

	openAIAdaptiveSchedulerEnabledKey                    = openAIAdaptiveSchedulerSettingPrefix + "enabled"
	openAIAdaptiveSchedulerModeKey                       = openAIAdaptiveSchedulerSettingPrefix + "mode"
	openAIAdaptiveSchedulerTopKKey                       = openAIAdaptiveSchedulerSettingPrefix + "top_k"
	openAIAdaptiveSchedulerExplorationRateKey            = openAIAdaptiveSchedulerSettingPrefix + "exploration_rate"
	openAIAdaptiveSchedulerSoftmaxTemperatureKey         = openAIAdaptiveSchedulerSettingPrefix + "softmax_temperature"
	openAIAdaptiveSchedulerMinCostMultiplierKey          = openAIAdaptiveSchedulerSettingPrefix + "min_cost_multiplier"
	openAIAdaptiveSchedulerThompsonEnabledKey            = openAIAdaptiveSchedulerSettingPrefix + "thompson_enabled"
	openAIAdaptiveSchedulerThompsonPriorAlphaKey         = openAIAdaptiveSchedulerSettingPrefix + "thompson_prior_alpha"
	openAIAdaptiveSchedulerThompsonPriorBetaKey          = openAIAdaptiveSchedulerSettingPrefix + "thompson_prior_beta"
	openAIAdaptiveSchedulerInitialCapacityKey            = openAIAdaptiveSchedulerSettingPrefix + "initial_capacity"
	openAIAdaptiveSchedulerInitialCapacityFractionKey    = openAIAdaptiveSchedulerSettingPrefix + "initial_capacity_fraction"
	openAIAdaptiveSchedulerMinCapacityKey                = openAIAdaptiveSchedulerSettingPrefix + "min_capacity"
	openAIAdaptiveSchedulerCapacityIncreaseStepKey       = openAIAdaptiveSchedulerSettingPrefix + "capacity_increase_step"
	openAIAdaptiveSchedulerCapacityGrowthFactorKey       = openAIAdaptiveSchedulerSettingPrefix + "capacity_growth_factor"
	openAIAdaptiveSchedulerCapacityDecreaseFactorKey     = openAIAdaptiveSchedulerSettingPrefix + "capacity_decrease_factor"
	openAIAdaptiveSchedulerCapacityProbeLoadThresholdKey = openAIAdaptiveSchedulerSettingPrefix + "capacity_probe_load_threshold"
	openAIAdaptiveSchedulerBurstProbeRatioKey            = openAIAdaptiveSchedulerSettingPrefix + "burst_probe_ratio"
	openAIAdaptiveSchedulerCapacitySuccessThresholdKey   = openAIAdaptiveSchedulerSettingPrefix + "capacity_success_threshold"
	openAIAdaptiveSchedulerCapacityFailureThresholdKey   = openAIAdaptiveSchedulerSettingPrefix + "capacity_failure_threshold"
	openAIAdaptiveSchedulerMinRecentSamplesForShrinkKey  = openAIAdaptiveSchedulerSettingPrefix + "min_recent_samples_for_shrink"
	openAIAdaptiveSchedulerShrinkErrorThresholdKey       = openAIAdaptiveSchedulerSettingPrefix + "shrink_error_threshold"
	openAIAdaptiveSchedulerShrinkFactorSoftKey           = openAIAdaptiveSchedulerSettingPrefix + "shrink_factor_soft"
	openAIAdaptiveSchedulerShrinkFactorHardKey           = openAIAdaptiveSchedulerSettingPrefix + "shrink_factor_hard"
	openAIAdaptiveSchedulerHalfOpenProbeCapacityKey      = openAIAdaptiveSchedulerSettingPrefix + "half_open_probe_capacity"
	openAIAdaptiveSchedulerLearningWindowSecondsKey      = openAIAdaptiveSchedulerSettingPrefix + "learning_window_seconds"
	openAIAdaptiveSchedulerSuccessEMAAlphaKey            = openAIAdaptiveSchedulerSettingPrefix + "success_ema_alpha"
	openAIAdaptiveSchedulerErrorEMAAlphaKey              = openAIAdaptiveSchedulerSettingPrefix + "error_ema_alpha"
	openAIAdaptiveSchedulerLatencyEMAAlphaKey            = openAIAdaptiveSchedulerSettingPrefix + "latency_ema_alpha"
	openAIAdaptiveSchedulerTTFTEMAAlphaKey               = openAIAdaptiveSchedulerSettingPrefix + "ttft_ema_alpha"
	openAIAdaptiveSchedulerCooldownBaseSecondsKey        = openAIAdaptiveSchedulerSettingPrefix + "cooldown_base_seconds"
	openAIAdaptiveSchedulerCooldownMaxSecondsKey         = openAIAdaptiveSchedulerSettingPrefix + "cooldown_max_seconds"
	openAIAdaptiveSchedulerWeightSuccessKey              = openAIAdaptiveSchedulerSettingPrefix + "weight_success"
	openAIAdaptiveSchedulerWeightCostKey                 = openAIAdaptiveSchedulerSettingPrefix + "weight_cost"
	openAIAdaptiveSchedulerWeightCapacityKey             = openAIAdaptiveSchedulerSettingPrefix + "weight_capacity"
	openAIAdaptiveSchedulerWeightLatencyKey              = openAIAdaptiveSchedulerSettingPrefix + "weight_latency"
	openAIAdaptiveSchedulerWeightStabilityKey            = openAIAdaptiveSchedulerSettingPrefix + "weight_stability"
	openAIAdaptiveSchedulerWeightExplorationKey          = openAIAdaptiveSchedulerSettingPrefix + "weight_exploration"
)

const (
	openAIAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	openAIAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
)

type OpenAIAdaptiveSchedulerSettings struct {
	OpenAIAdaptiveSchedulerEnabled                    bool    `json:"openai_adaptive_scheduler_enabled"`
	OpenAIAdaptiveSchedulerMode                       string  `json:"openai_adaptive_scheduler_mode"`
	OpenAIAdaptiveSchedulerTopK                       int     `json:"openai_adaptive_scheduler_top_k"`
	OpenAIAdaptiveSchedulerExplorationRate            float64 `json:"openai_adaptive_scheduler_exploration_rate"`
	OpenAIAdaptiveSchedulerSoftmaxTemperature         float64 `json:"openai_adaptive_scheduler_softmax_temperature"`
	OpenAIAdaptiveSchedulerMinCostMultiplier          float64 `json:"openai_adaptive_scheduler_min_cost_multiplier"`
	OpenAIAdaptiveSchedulerThompsonEnabled            bool    `json:"openai_adaptive_scheduler_thompson_enabled"`
	OpenAIAdaptiveSchedulerThompsonPriorAlpha         float64 `json:"openai_adaptive_scheduler_thompson_prior_alpha"`
	OpenAIAdaptiveSchedulerThompsonPriorBeta          float64 `json:"openai_adaptive_scheduler_thompson_prior_beta"`
	OpenAIAdaptiveSchedulerInitialCapacity            int     `json:"openai_adaptive_scheduler_initial_capacity"`
	OpenAIAdaptiveSchedulerInitialCapacityFraction    float64 `json:"openai_adaptive_scheduler_initial_capacity_fraction"`
	OpenAIAdaptiveSchedulerMinCapacity                int     `json:"openai_adaptive_scheduler_min_capacity"`
	OpenAIAdaptiveSchedulerCapacityIncreaseStep       int     `json:"openai_adaptive_scheduler_capacity_increase_step"`
	OpenAIAdaptiveSchedulerCapacityGrowthFactor       float64 `json:"openai_adaptive_scheduler_capacity_growth_factor"`
	OpenAIAdaptiveSchedulerCapacityDecreaseFactor     float64 `json:"openai_adaptive_scheduler_capacity_decrease_factor"`
	OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold float64 `json:"openai_adaptive_scheduler_capacity_probe_load_threshold"`
	OpenAIAdaptiveSchedulerBurstProbeRatio            float64 `json:"openai_adaptive_scheduler_burst_probe_ratio"`
	OpenAIAdaptiveSchedulerCapacitySuccessThreshold   float64 `json:"openai_adaptive_scheduler_capacity_success_threshold"`
	OpenAIAdaptiveSchedulerCapacityFailureThreshold   int     `json:"openai_adaptive_scheduler_capacity_failure_threshold"`
	OpenAIAdaptiveSchedulerMinRecentSamplesForShrink  int     `json:"openai_adaptive_scheduler_min_recent_samples_for_shrink"`
	OpenAIAdaptiveSchedulerShrinkErrorThreshold       float64 `json:"openai_adaptive_scheduler_shrink_error_threshold"`
	OpenAIAdaptiveSchedulerShrinkFactorSoft           float64 `json:"openai_adaptive_scheduler_shrink_factor_soft"`
	OpenAIAdaptiveSchedulerShrinkFactorHard           float64 `json:"openai_adaptive_scheduler_shrink_factor_hard"`
	OpenAIAdaptiveSchedulerHalfOpenProbeCapacity      int     `json:"openai_adaptive_scheduler_half_open_probe_capacity"`
	OpenAIAdaptiveSchedulerLearningWindowSeconds      int     `json:"openai_adaptive_scheduler_learning_window_seconds"`
	OpenAIAdaptiveSchedulerSuccessEMAAlpha            float64 `json:"openai_adaptive_scheduler_success_ema_alpha"`
	OpenAIAdaptiveSchedulerErrorEMAAlpha              float64 `json:"openai_adaptive_scheduler_error_ema_alpha"`
	OpenAIAdaptiveSchedulerLatencyEMAAlpha            float64 `json:"openai_adaptive_scheduler_latency_ema_alpha"`
	OpenAIAdaptiveSchedulerTTFTEMAAlpha               float64 `json:"openai_adaptive_scheduler_ttft_ema_alpha"`
	OpenAIAdaptiveSchedulerCooldownBaseSeconds        int     `json:"openai_adaptive_scheduler_cooldown_base_seconds"`
	OpenAIAdaptiveSchedulerCooldownMaxSeconds         int     `json:"openai_adaptive_scheduler_cooldown_max_seconds"`
	OpenAIAdaptiveSchedulerWeightSuccess              float64 `json:"openai_adaptive_scheduler_weight_success"`
	OpenAIAdaptiveSchedulerWeightCost                 float64 `json:"openai_adaptive_scheduler_weight_cost"`
	OpenAIAdaptiveSchedulerWeightCapacity             float64 `json:"openai_adaptive_scheduler_weight_capacity"`
	OpenAIAdaptiveSchedulerWeightLatency              float64 `json:"openai_adaptive_scheduler_weight_latency"`
	OpenAIAdaptiveSchedulerWeightStability            float64 `json:"openai_adaptive_scheduler_weight_stability"`
	OpenAIAdaptiveSchedulerWeightExploration          float64 `json:"openai_adaptive_scheduler_weight_exploration"`
}

type cachedOpenAIAdaptiveSchedulerSetting struct {
	settings  OpenAIAdaptiveSchedulerSettings
	complete  bool
	expiresAt int64
}

var openAIAdaptiveSchedulerSettingCache atomic.Value // *cachedOpenAIAdaptiveSchedulerSetting
var openAIAdaptiveSchedulerSettingSF singleflight.Group

func DefaultOpenAIAdaptiveSchedulerSettings() OpenAIAdaptiveSchedulerSettings {
	return OpenAIAdaptiveSchedulerSettings{
		OpenAIAdaptiveSchedulerEnabled:                    false,
		OpenAIAdaptiveSchedulerMode:                       openAIAdaptiveSchedulerModeShadow,
		OpenAIAdaptiveSchedulerTopK:                       10,
		OpenAIAdaptiveSchedulerExplorationRate:            0.05,
		OpenAIAdaptiveSchedulerSoftmaxTemperature:         0.35,
		OpenAIAdaptiveSchedulerMinCostMultiplier:          0.01,
		OpenAIAdaptiveSchedulerThompsonEnabled:            true,
		OpenAIAdaptiveSchedulerThompsonPriorAlpha:         1,
		OpenAIAdaptiveSchedulerThompsonPriorBeta:          1,
		OpenAIAdaptiveSchedulerInitialCapacity:            1,
		OpenAIAdaptiveSchedulerInitialCapacityFraction:    0.10,
		OpenAIAdaptiveSchedulerMinCapacity:                1,
		OpenAIAdaptiveSchedulerCapacityIncreaseStep:       1,
		OpenAIAdaptiveSchedulerCapacityGrowthFactor:       1.25,
		OpenAIAdaptiveSchedulerCapacityDecreaseFactor:     0.6,
		OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold: 0.8,
		OpenAIAdaptiveSchedulerBurstProbeRatio:            0.20,
		OpenAIAdaptiveSchedulerCapacitySuccessThreshold:   0.98,
		OpenAIAdaptiveSchedulerCapacityFailureThreshold:   3,
		OpenAIAdaptiveSchedulerMinRecentSamplesForShrink:  10,
		OpenAIAdaptiveSchedulerShrinkErrorThreshold:       0.20,
		OpenAIAdaptiveSchedulerShrinkFactorSoft:           0.80,
		OpenAIAdaptiveSchedulerShrinkFactorHard:           0.50,
		OpenAIAdaptiveSchedulerHalfOpenProbeCapacity:      5,
		OpenAIAdaptiveSchedulerLearningWindowSeconds:      900,
		OpenAIAdaptiveSchedulerSuccessEMAAlpha:            0.05,
		OpenAIAdaptiveSchedulerErrorEMAAlpha:              0.10,
		OpenAIAdaptiveSchedulerLatencyEMAAlpha:            0.05,
		OpenAIAdaptiveSchedulerTTFTEMAAlpha:               0.05,
		OpenAIAdaptiveSchedulerCooldownBaseSeconds:        60,
		OpenAIAdaptiveSchedulerCooldownMaxSeconds:         600,
		OpenAIAdaptiveSchedulerWeightSuccess:              0.30,
		OpenAIAdaptiveSchedulerWeightCost:                 0.25,
		OpenAIAdaptiveSchedulerWeightCapacity:             0.20,
		OpenAIAdaptiveSchedulerWeightLatency:              0.15,
		OpenAIAdaptiveSchedulerWeightStability:            0.05,
		OpenAIAdaptiveSchedulerWeightExploration:          0.05,
	}
}

func NormalizeOpenAIAdaptiveSchedulerSettings(settings OpenAIAdaptiveSchedulerSettings) OpenAIAdaptiveSchedulerSettings {
	defaults := DefaultOpenAIAdaptiveSchedulerSettings()
	settings.OpenAIAdaptiveSchedulerMode = normalizeOpenAIAdaptiveSchedulerMode(settings.OpenAIAdaptiveSchedulerMode)
	if settings.OpenAIAdaptiveSchedulerMode == "" {
		settings.OpenAIAdaptiveSchedulerMode = defaults.OpenAIAdaptiveSchedulerMode
	}
	settings.OpenAIAdaptiveSchedulerTopK = clampInt(settings.OpenAIAdaptiveSchedulerTopK, 1, 100, defaults.OpenAIAdaptiveSchedulerTopK)
	settings.OpenAIAdaptiveSchedulerExplorationRate = clampFloat(settings.OpenAIAdaptiveSchedulerExplorationRate, 0, 1, defaults.OpenAIAdaptiveSchedulerExplorationRate)
	settings.OpenAIAdaptiveSchedulerSoftmaxTemperature = clampFloat(settings.OpenAIAdaptiveSchedulerSoftmaxTemperature, 0.01, 10, defaults.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	settings.OpenAIAdaptiveSchedulerMinCostMultiplier = minPositiveFloat(settings.OpenAIAdaptiveSchedulerMinCostMultiplier, defaults.OpenAIAdaptiveSchedulerMinCostMultiplier)
	settings.OpenAIAdaptiveSchedulerThompsonPriorAlpha = minPositiveFloat(settings.OpenAIAdaptiveSchedulerThompsonPriorAlpha, defaults.OpenAIAdaptiveSchedulerThompsonPriorAlpha)
	settings.OpenAIAdaptiveSchedulerThompsonPriorBeta = minPositiveFloat(settings.OpenAIAdaptiveSchedulerThompsonPriorBeta, defaults.OpenAIAdaptiveSchedulerThompsonPriorBeta)
	settings.OpenAIAdaptiveSchedulerInitialCapacity = clampIntMin(settings.OpenAIAdaptiveSchedulerInitialCapacity, 1, defaults.OpenAIAdaptiveSchedulerInitialCapacity)
	settings.OpenAIAdaptiveSchedulerInitialCapacityFraction = clampFloat(settings.OpenAIAdaptiveSchedulerInitialCapacityFraction, 0, 1, defaults.OpenAIAdaptiveSchedulerInitialCapacityFraction)
	settings.OpenAIAdaptiveSchedulerMinCapacity = clampIntMin(settings.OpenAIAdaptiveSchedulerMinCapacity, 1, defaults.OpenAIAdaptiveSchedulerMinCapacity)
	settings.OpenAIAdaptiveSchedulerCapacityIncreaseStep = clampIntMin(settings.OpenAIAdaptiveSchedulerCapacityIncreaseStep, 1, defaults.OpenAIAdaptiveSchedulerCapacityIncreaseStep)
	settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor = clampFloat(settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor, 1, 10, defaults.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	settings.OpenAIAdaptiveSchedulerCapacityDecreaseFactor = clampFloat(settings.OpenAIAdaptiveSchedulerCapacityDecreaseFactor, 0.01, 1, defaults.OpenAIAdaptiveSchedulerCapacityDecreaseFactor)
	settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold = clampFloat(settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold, 0, 1, defaults.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.OpenAIAdaptiveSchedulerBurstProbeRatio = clampFloat(settings.OpenAIAdaptiveSchedulerBurstProbeRatio, 0, 1, defaults.OpenAIAdaptiveSchedulerBurstProbeRatio)
	settings.OpenAIAdaptiveSchedulerCapacitySuccessThreshold = clampFloat(settings.OpenAIAdaptiveSchedulerCapacitySuccessThreshold, 0, 1, defaults.OpenAIAdaptiveSchedulerCapacitySuccessThreshold)
	settings.OpenAIAdaptiveSchedulerCapacityFailureThreshold = clampIntMin(settings.OpenAIAdaptiveSchedulerCapacityFailureThreshold, 1, defaults.OpenAIAdaptiveSchedulerCapacityFailureThreshold)
	settings.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink = clampIntMin(settings.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink, 1, defaults.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink)
	settings.OpenAIAdaptiveSchedulerShrinkErrorThreshold = clampFloat(settings.OpenAIAdaptiveSchedulerShrinkErrorThreshold, 0, 1, defaults.OpenAIAdaptiveSchedulerShrinkErrorThreshold)
	settings.OpenAIAdaptiveSchedulerShrinkFactorSoft = clampFloat(settings.OpenAIAdaptiveSchedulerShrinkFactorSoft, 0.01, 1, defaults.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	settings.OpenAIAdaptiveSchedulerShrinkFactorHard = clampFloat(settings.OpenAIAdaptiveSchedulerShrinkFactorHard, 0.01, 1, defaults.OpenAIAdaptiveSchedulerShrinkFactorHard)
	if settings.OpenAIAdaptiveSchedulerShrinkFactorHard > settings.OpenAIAdaptiveSchedulerShrinkFactorSoft {
		settings.OpenAIAdaptiveSchedulerShrinkFactorHard = settings.OpenAIAdaptiveSchedulerShrinkFactorSoft
	}
	settings.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity = clampIntMin(settings.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity, 1, defaults.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity)
	settings.OpenAIAdaptiveSchedulerLearningWindowSeconds = clampIntMin(settings.OpenAIAdaptiveSchedulerLearningWindowSeconds, 0, defaults.OpenAIAdaptiveSchedulerLearningWindowSeconds)
	settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha = clampFloat(settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha, 0, 1, defaults.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	settings.OpenAIAdaptiveSchedulerErrorEMAAlpha = clampFloat(settings.OpenAIAdaptiveSchedulerErrorEMAAlpha, 0, 1, defaults.OpenAIAdaptiveSchedulerErrorEMAAlpha)
	settings.OpenAIAdaptiveSchedulerLatencyEMAAlpha = clampFloat(settings.OpenAIAdaptiveSchedulerLatencyEMAAlpha, 0, 1, defaults.OpenAIAdaptiveSchedulerLatencyEMAAlpha)
	settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha = clampFloat(settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha, 0, 1, defaults.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds = clampIntMin(settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds, 0, defaults.OpenAIAdaptiveSchedulerCooldownBaseSeconds)
	settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds = clampIntMin(settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds, 0, defaults.OpenAIAdaptiveSchedulerCooldownMaxSeconds)
	if settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds > 0 &&
		settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds > settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds {
		settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds = settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds
	}
	settings.OpenAIAdaptiveSchedulerWeightSuccess = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightSuccess)
	settings.OpenAIAdaptiveSchedulerWeightCost = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightCost)
	settings.OpenAIAdaptiveSchedulerWeightCapacity = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightCapacity)
	settings.OpenAIAdaptiveSchedulerWeightLatency = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightLatency)
	settings.OpenAIAdaptiveSchedulerWeightStability = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightStability)
	settings.OpenAIAdaptiveSchedulerWeightExploration = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightExploration)
	weightSum := settings.OpenAIAdaptiveSchedulerWeightSuccess +
		settings.OpenAIAdaptiveSchedulerWeightCost +
		settings.OpenAIAdaptiveSchedulerWeightCapacity +
		settings.OpenAIAdaptiveSchedulerWeightLatency +
		settings.OpenAIAdaptiveSchedulerWeightStability +
		settings.OpenAIAdaptiveSchedulerWeightExploration
	if weightSum <= 0 {
		settings.OpenAIAdaptiveSchedulerWeightSuccess = defaults.OpenAIAdaptiveSchedulerWeightSuccess
		settings.OpenAIAdaptiveSchedulerWeightCost = defaults.OpenAIAdaptiveSchedulerWeightCost
		settings.OpenAIAdaptiveSchedulerWeightCapacity = defaults.OpenAIAdaptiveSchedulerWeightCapacity
		settings.OpenAIAdaptiveSchedulerWeightLatency = defaults.OpenAIAdaptiveSchedulerWeightLatency
		settings.OpenAIAdaptiveSchedulerWeightStability = defaults.OpenAIAdaptiveSchedulerWeightStability
		settings.OpenAIAdaptiveSchedulerWeightExploration = defaults.OpenAIAdaptiveSchedulerWeightExploration
	}
	return settings
}

func openAIAdaptiveSchedulerDefaultSettingValues() map[string]string {
	return openAIAdaptiveSchedulerSettingsToMap(DefaultOpenAIAdaptiveSchedulerSettings())
}

func openAIAdaptiveSchedulerSettingsToMap(settings OpenAIAdaptiveSchedulerSettings) map[string]string {
	settings = NormalizeOpenAIAdaptiveSchedulerSettings(settings)
	return map[string]string{
		openAIAdaptiveSchedulerEnabledKey:                    strconv.FormatBool(settings.OpenAIAdaptiveSchedulerEnabled),
		openAIAdaptiveSchedulerModeKey:                       settings.OpenAIAdaptiveSchedulerMode,
		openAIAdaptiveSchedulerTopKKey:                       strconv.Itoa(settings.OpenAIAdaptiveSchedulerTopK),
		openAIAdaptiveSchedulerExplorationRateKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerExplorationRate),
		openAIAdaptiveSchedulerSoftmaxTemperatureKey:         formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerSoftmaxTemperature),
		openAIAdaptiveSchedulerMinCostMultiplierKey:          formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerMinCostMultiplier),
		openAIAdaptiveSchedulerThompsonEnabledKey:            strconv.FormatBool(settings.OpenAIAdaptiveSchedulerThompsonEnabled),
		openAIAdaptiveSchedulerThompsonPriorAlphaKey:         formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerThompsonPriorAlpha),
		openAIAdaptiveSchedulerThompsonPriorBetaKey:          formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerThompsonPriorBeta),
		openAIAdaptiveSchedulerInitialCapacityKey:            strconv.Itoa(settings.OpenAIAdaptiveSchedulerInitialCapacity),
		openAIAdaptiveSchedulerInitialCapacityFractionKey:    formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerInitialCapacityFraction),
		openAIAdaptiveSchedulerMinCapacityKey:                strconv.Itoa(settings.OpenAIAdaptiveSchedulerMinCapacity),
		openAIAdaptiveSchedulerCapacityIncreaseStepKey:       strconv.Itoa(settings.OpenAIAdaptiveSchedulerCapacityIncreaseStep),
		openAIAdaptiveSchedulerCapacityGrowthFactorKey:       formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor),
		openAIAdaptiveSchedulerCapacityDecreaseFactorKey:     formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerCapacityDecreaseFactor),
		openAIAdaptiveSchedulerCapacityProbeLoadThresholdKey: formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold),
		openAIAdaptiveSchedulerBurstProbeRatioKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerBurstProbeRatio),
		openAIAdaptiveSchedulerCapacitySuccessThresholdKey:   formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerCapacitySuccessThreshold),
		openAIAdaptiveSchedulerCapacityFailureThresholdKey:   strconv.Itoa(settings.OpenAIAdaptiveSchedulerCapacityFailureThreshold),
		openAIAdaptiveSchedulerMinRecentSamplesForShrinkKey:  strconv.Itoa(settings.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink),
		openAIAdaptiveSchedulerShrinkErrorThresholdKey:       formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerShrinkErrorThreshold),
		openAIAdaptiveSchedulerShrinkFactorSoftKey:           formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerShrinkFactorSoft),
		openAIAdaptiveSchedulerShrinkFactorHardKey:           formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerShrinkFactorHard),
		openAIAdaptiveSchedulerHalfOpenProbeCapacityKey:      strconv.Itoa(settings.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity),
		openAIAdaptiveSchedulerLearningWindowSecondsKey:      strconv.Itoa(settings.OpenAIAdaptiveSchedulerLearningWindowSeconds),
		openAIAdaptiveSchedulerSuccessEMAAlphaKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha),
		openAIAdaptiveSchedulerErrorEMAAlphaKey:              formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerErrorEMAAlpha),
		openAIAdaptiveSchedulerLatencyEMAAlphaKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerLatencyEMAAlpha),
		openAIAdaptiveSchedulerTTFTEMAAlphaKey:               formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha),
		openAIAdaptiveSchedulerCooldownBaseSecondsKey:        strconv.Itoa(settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds),
		openAIAdaptiveSchedulerCooldownMaxSecondsKey:         strconv.Itoa(settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds),
		openAIAdaptiveSchedulerWeightSuccessKey:              formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightSuccess),
		openAIAdaptiveSchedulerWeightCostKey:                 formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightCost),
		openAIAdaptiveSchedulerWeightCapacityKey:             formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightCapacity),
		openAIAdaptiveSchedulerWeightLatencyKey:              formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightLatency),
		openAIAdaptiveSchedulerWeightStabilityKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightStability),
		openAIAdaptiveSchedulerWeightExplorationKey:          formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightExploration),
	}
}

func parseOpenAIAdaptiveSchedulerSettings(settings map[string]string) OpenAIAdaptiveSchedulerSettings {
	result := DefaultOpenAIAdaptiveSchedulerSettings()
	result.OpenAIAdaptiveSchedulerEnabled = parseBoolSetting(settings, openAIAdaptiveSchedulerEnabledKey, result.OpenAIAdaptiveSchedulerEnabled)
	result.OpenAIAdaptiveSchedulerMode = firstNonEmpty(settings[openAIAdaptiveSchedulerModeKey], result.OpenAIAdaptiveSchedulerMode)
	result.OpenAIAdaptiveSchedulerTopK = parseIntSetting(settings, openAIAdaptiveSchedulerTopKKey, result.OpenAIAdaptiveSchedulerTopK)
	result.OpenAIAdaptiveSchedulerExplorationRate = parseFloatSetting(settings, openAIAdaptiveSchedulerExplorationRateKey, result.OpenAIAdaptiveSchedulerExplorationRate)
	result.OpenAIAdaptiveSchedulerSoftmaxTemperature = parseFloatSetting(settings, openAIAdaptiveSchedulerSoftmaxTemperatureKey, result.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	result.OpenAIAdaptiveSchedulerMinCostMultiplier = parseFloatSetting(settings, openAIAdaptiveSchedulerMinCostMultiplierKey, result.OpenAIAdaptiveSchedulerMinCostMultiplier)
	result.OpenAIAdaptiveSchedulerThompsonEnabled = parseBoolSetting(settings, openAIAdaptiveSchedulerThompsonEnabledKey, result.OpenAIAdaptiveSchedulerThompsonEnabled)
	result.OpenAIAdaptiveSchedulerThompsonPriorAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerThompsonPriorAlphaKey, result.OpenAIAdaptiveSchedulerThompsonPriorAlpha)
	result.OpenAIAdaptiveSchedulerThompsonPriorBeta = parseFloatSetting(settings, openAIAdaptiveSchedulerThompsonPriorBetaKey, result.OpenAIAdaptiveSchedulerThompsonPriorBeta)
	result.OpenAIAdaptiveSchedulerInitialCapacity = parseIntSetting(settings, openAIAdaptiveSchedulerInitialCapacityKey, result.OpenAIAdaptiveSchedulerInitialCapacity)
	result.OpenAIAdaptiveSchedulerInitialCapacityFraction = parseFloatSetting(settings, openAIAdaptiveSchedulerInitialCapacityFractionKey, result.OpenAIAdaptiveSchedulerInitialCapacityFraction)
	result.OpenAIAdaptiveSchedulerMinCapacity = parseIntSetting(settings, openAIAdaptiveSchedulerMinCapacityKey, result.OpenAIAdaptiveSchedulerMinCapacity)
	result.OpenAIAdaptiveSchedulerCapacityIncreaseStep = parseIntSetting(settings, openAIAdaptiveSchedulerCapacityIncreaseStepKey, result.OpenAIAdaptiveSchedulerCapacityIncreaseStep)
	result.OpenAIAdaptiveSchedulerCapacityGrowthFactor = parseFloatSetting(settings, openAIAdaptiveSchedulerCapacityGrowthFactorKey, result.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	result.OpenAIAdaptiveSchedulerCapacityDecreaseFactor = parseFloatSetting(settings, openAIAdaptiveSchedulerCapacityDecreaseFactorKey, result.OpenAIAdaptiveSchedulerCapacityDecreaseFactor)
	result.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold = parseFloatSetting(settings, openAIAdaptiveSchedulerCapacityProbeLoadThresholdKey, result.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	result.OpenAIAdaptiveSchedulerBurstProbeRatio = parseFloatSetting(settings, openAIAdaptiveSchedulerBurstProbeRatioKey, result.OpenAIAdaptiveSchedulerBurstProbeRatio)
	result.OpenAIAdaptiveSchedulerCapacitySuccessThreshold = parseFloatSetting(settings, openAIAdaptiveSchedulerCapacitySuccessThresholdKey, result.OpenAIAdaptiveSchedulerCapacitySuccessThreshold)
	result.OpenAIAdaptiveSchedulerCapacityFailureThreshold = parseIntSetting(settings, openAIAdaptiveSchedulerCapacityFailureThresholdKey, result.OpenAIAdaptiveSchedulerCapacityFailureThreshold)
	result.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink = parseIntSetting(settings, openAIAdaptiveSchedulerMinRecentSamplesForShrinkKey, result.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink)
	result.OpenAIAdaptiveSchedulerShrinkErrorThreshold = parseFloatSetting(settings, openAIAdaptiveSchedulerShrinkErrorThresholdKey, result.OpenAIAdaptiveSchedulerShrinkErrorThreshold)
	result.OpenAIAdaptiveSchedulerShrinkFactorSoft = parseFloatSetting(settings, openAIAdaptiveSchedulerShrinkFactorSoftKey, result.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	result.OpenAIAdaptiveSchedulerShrinkFactorHard = parseFloatSetting(settings, openAIAdaptiveSchedulerShrinkFactorHardKey, result.OpenAIAdaptiveSchedulerShrinkFactorHard)
	result.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity = parseIntSetting(settings, openAIAdaptiveSchedulerHalfOpenProbeCapacityKey, result.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity)
	result.OpenAIAdaptiveSchedulerLearningWindowSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerLearningWindowSecondsKey, result.OpenAIAdaptiveSchedulerLearningWindowSeconds)
	result.OpenAIAdaptiveSchedulerSuccessEMAAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerSuccessEMAAlphaKey, result.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	result.OpenAIAdaptiveSchedulerErrorEMAAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerErrorEMAAlphaKey, result.OpenAIAdaptiveSchedulerErrorEMAAlpha)
	result.OpenAIAdaptiveSchedulerLatencyEMAAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerLatencyEMAAlphaKey, result.OpenAIAdaptiveSchedulerLatencyEMAAlpha)
	result.OpenAIAdaptiveSchedulerTTFTEMAAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerTTFTEMAAlphaKey, result.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	result.OpenAIAdaptiveSchedulerCooldownBaseSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerCooldownBaseSecondsKey, result.OpenAIAdaptiveSchedulerCooldownBaseSeconds)
	result.OpenAIAdaptiveSchedulerCooldownMaxSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerCooldownMaxSecondsKey, result.OpenAIAdaptiveSchedulerCooldownMaxSeconds)
	result.OpenAIAdaptiveSchedulerWeightSuccess = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightSuccessKey, result.OpenAIAdaptiveSchedulerWeightSuccess)
	result.OpenAIAdaptiveSchedulerWeightCost = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightCostKey, result.OpenAIAdaptiveSchedulerWeightCost)
	result.OpenAIAdaptiveSchedulerWeightCapacity = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightCapacityKey, result.OpenAIAdaptiveSchedulerWeightCapacity)
	result.OpenAIAdaptiveSchedulerWeightLatency = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightLatencyKey, result.OpenAIAdaptiveSchedulerWeightLatency)
	result.OpenAIAdaptiveSchedulerWeightStability = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightStabilityKey, result.OpenAIAdaptiveSchedulerWeightStability)
	result.OpenAIAdaptiveSchedulerWeightExploration = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightExplorationKey, result.OpenAIAdaptiveSchedulerWeightExploration)
	return NormalizeOpenAIAdaptiveSchedulerSettings(result)
}

func (s *OpenAIGatewayService) openAIAdaptiveSchedulerSettingRepo() SettingRepository {
	if s == nil || s.rateLimitService == nil || s.rateLimitService.settingService == nil {
		return nil
	}
	return s.rateLimitService.settingService.settingRepo
}

func (s *OpenAIGatewayService) isOpenAIAdaptiveSchedulerEnabled(ctx context.Context) bool {
	if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.settings.OpenAIAdaptiveSchedulerEnabled
		}
	}

	result, _, _ := openAIAdaptiveSchedulerSettingSF.Do(openAIAdaptiveSchedulerEnabledKey, func() (any, error) {
		if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.settings.OpenAIAdaptiveSchedulerEnabled, nil
			}
		}

		settings := DefaultOpenAIAdaptiveSchedulerSettings()
		if repo := s.openAIAdaptiveSchedulerSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAdaptiveSchedulerSettingDBTimeout)
			defer cancel()
			value, err := repo.GetValue(dbCtx, openAIAdaptiveSchedulerEnabledKey)
			if err == nil {
				settings.OpenAIAdaptiveSchedulerEnabled = strings.EqualFold(strings.TrimSpace(value), "true")
			}
		}
		openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{
			settings:  settings,
			complete:  false,
			expiresAt: time.Now().Add(openAIAdaptiveSchedulerSettingCacheTTL).UnixNano(),
		})
		return settings.OpenAIAdaptiveSchedulerEnabled, nil
	})

	enabled, _ := result.(bool)
	return enabled
}

func (s *OpenAIGatewayService) openAIAdaptiveSchedulerSettings(ctx context.Context) OpenAIAdaptiveSchedulerSettings {
	if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt && cached.complete && cached.settings.OpenAIAdaptiveSchedulerEnabled {
			return cached.settings
		}
	}

	result, _, _ := openAIAdaptiveSchedulerSettingSF.Do("openai_adaptive_scheduler_settings", func() (any, error) {
		if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt && cached.complete {
				return cached.settings, nil
			}
		}

		settings := DefaultOpenAIAdaptiveSchedulerSettings()
		if repo := s.openAIAdaptiveSchedulerSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAdaptiveSchedulerSettingDBTimeout)
			defer cancel()
			values, err := repo.GetAll(dbCtx)
			if err == nil {
				settings = parseOpenAIAdaptiveSchedulerSettings(values)
			}
		}

		openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{
			settings:  settings,
			complete:  true,
			expiresAt: time.Now().Add(openAIAdaptiveSchedulerSettingCacheTTL).UnixNano(),
		})
		return settings, nil
	})

	settings, _ := result.(OpenAIAdaptiveSchedulerSettings)
	return NormalizeOpenAIAdaptiveSchedulerSettings(settings)
}

func resetOpenAIAdaptiveSchedulerSettingCacheForTest() {
	openAIAdaptiveSchedulerSettingCache = atomic.Value{}
	openAIAdaptiveSchedulerSettingSF = singleflight.Group{}
}

func normalizeOpenAIAdaptiveSchedulerMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case openAIAdaptiveSchedulerModeEnforce:
		return openAIAdaptiveSchedulerModeEnforce
	case openAIAdaptiveSchedulerModeShadow:
		return openAIAdaptiveSchedulerModeShadow
	default:
		return ""
	}
}

func parseBoolSetting(settings map[string]string, key string, fallback bool) bool {
	value, ok := settings[key]
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseIntSetting(settings map[string]string, key string, fallback int) int {
	value, ok := settings[key]
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseFloatSetting(settings map[string]string, key string, fallback float64) float64 {
	value, ok := settings[key]
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	return parsed
}

func formatOpenAIAdaptiveFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func clampInt(value, minValue, maxValue, fallback int) int {
	if value < minValue || value > maxValue {
		return fallback
	}
	return value
}

func clampIntMin(value, minValue, fallback int) int {
	if value < minValue {
		return fallback
	}
	return value
}

func clampFloat(value, minValue, maxValue, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < minValue || value > maxValue {
		return fallback
	}
	return value
}

func minPositiveFloat(value, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fallback
	}
	return value
}

func nonNegativeFinite(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}
