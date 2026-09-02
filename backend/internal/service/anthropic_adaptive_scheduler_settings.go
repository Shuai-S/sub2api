package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	AnthropicAdaptiveSchedulerModeShadow  = "shadow"
	AnthropicAdaptiveSchedulerModeEnforce = "enforce"
)

const (
	anthropicAdaptiveSchedulerSettingPrefix = "anthropic_adaptive_scheduler_"

	SettingKeyAnthropicAdaptiveSchedulerEnabled                    = anthropicAdaptiveSchedulerSettingPrefix + "enabled"
	SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled       = anthropicAdaptiveSchedulerSettingPrefix + "diagnostic_log_enabled"
	SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate    = anthropicAdaptiveSchedulerSettingPrefix + "diagnostic_log_sample_rate"
	SettingKeyAnthropicAdaptiveSchedulerMode                       = anthropicAdaptiveSchedulerSettingPrefix + "mode"
	SettingKeyAnthropicAdaptiveSchedulerTopK                       = anthropicAdaptiveSchedulerSettingPrefix + "top_k"
	SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature         = anthropicAdaptiveSchedulerSettingPrefix + "softmax_temperature"
	SettingKeyAnthropicAdaptiveSchedulerWeightReliability          = anthropicAdaptiveSchedulerSettingPrefix + "weight_reliability"
	SettingKeyAnthropicAdaptiveSchedulerWeightCapacity             = anthropicAdaptiveSchedulerSettingPrefix + "weight_capacity"
	SettingKeyAnthropicAdaptiveSchedulerWeightLatency              = anthropicAdaptiveSchedulerSettingPrefix + "weight_latency"
	SettingKeyAnthropicAdaptiveSchedulerWeightCache                = anthropicAdaptiveSchedulerSettingPrefix + "weight_cache"
	SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty  = anthropicAdaptiveSchedulerSettingPrefix + "consecutive_failure_penalty"
	SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha            = anthropicAdaptiveSchedulerSettingPrefix + "success_ema_alpha"
	SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha            = anthropicAdaptiveSchedulerSettingPrefix + "latency_ema_alpha"
	SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = anthropicAdaptiveSchedulerSettingPrefix + "capacity_probe_load_threshold"
	SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds      = anthropicAdaptiveSchedulerSettingPrefix + "learning_window_seconds"
	SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds            = anthropicAdaptiveSchedulerSettingPrefix + "cooldown_seconds"
	SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft           = anthropicAdaptiveSchedulerSettingPrefix + "shrink_factor_soft"
	SettingKeyAnthropicAdaptiveSchedulerExplorationRate            = anthropicAdaptiveSchedulerSettingPrefix + "exploration_rate"
	SettingKeyAnthropicAdaptiveSchedulerWeightCost                 = anthropicAdaptiveSchedulerSettingPrefix + "weight_cost"
	SettingKeyAnthropicAdaptiveSchedulerLearningMinHealthSamples   = anthropicAdaptiveSchedulerSettingPrefix + "learning_min_health_samples"
	SettingKeyAnthropicAdaptiveSchedulerHealthFailureThreshold     = anthropicAdaptiveSchedulerSettingPrefix + "health_failure_threshold"
	SettingKeyAnthropicAdaptiveSchedulerCooldownMaxSeconds         = anthropicAdaptiveSchedulerSettingPrefix + "cooldown_max_seconds"
	SettingKeyAnthropicAdaptiveSchedulerHighErrorMinSamples        = anthropicAdaptiveSchedulerSettingPrefix + "high_error_min_samples"
	SettingKeyAnthropicAdaptiveSchedulerHighErrorMaxSamples        = anthropicAdaptiveSchedulerSettingPrefix + "high_error_max_samples"
	SettingKeyAnthropicAdaptiveSchedulerHighErrorEnterRate         = anthropicAdaptiveSchedulerSettingPrefix + "high_error_enter_rate"
	SettingKeyAnthropicAdaptiveSchedulerHighErrorExitRate          = anthropicAdaptiveSchedulerSettingPrefix + "high_error_exit_rate"
	SettingKeyAnthropicAdaptiveSchedulerCapacityRecoverySamples    = anthropicAdaptiveSchedulerSettingPrefix + "capacity_recovery_samples"
	SettingKeyAnthropicAdaptiveSchedulerCapacityGrowthFactor       = anthropicAdaptiveSchedulerSettingPrefix + "capacity_growth_factor"
	SettingKeyAnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds  = anthropicAdaptiveSchedulerSettingPrefix + "quota_probe_interval_seconds"

	anthropicAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	anthropicAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
)

type AnthropicAdaptiveSchedulerSettings struct {
	AnthropicAdaptiveSchedulerEnabled                    bool    `json:"anthropic_adaptive_scheduler_enabled"`
	AnthropicAdaptiveSchedulerDiagnosticLogEnabled       bool    `json:"anthropic_adaptive_scheduler_diagnostic_log_enabled"`
	AnthropicAdaptiveSchedulerDiagnosticLogSampleRate    float64 `json:"anthropic_adaptive_scheduler_diagnostic_log_sample_rate"`
	AnthropicAdaptiveSchedulerMode                       string  `json:"anthropic_adaptive_scheduler_mode"`
	AnthropicAdaptiveSchedulerTopK                       int     `json:"anthropic_adaptive_scheduler_top_k"`
	AnthropicAdaptiveSchedulerSoftmaxTemperature         float64 `json:"anthropic_adaptive_scheduler_softmax_temperature"`
	AnthropicAdaptiveSchedulerWeightReliability          float64 `json:"anthropic_adaptive_scheduler_weight_reliability"`
	AnthropicAdaptiveSchedulerWeightCapacity             float64 `json:"anthropic_adaptive_scheduler_weight_capacity"`
	AnthropicAdaptiveSchedulerWeightLatency              float64 `json:"anthropic_adaptive_scheduler_weight_latency"`
	AnthropicAdaptiveSchedulerWeightCache                float64 `json:"anthropic_adaptive_scheduler_weight_cache"`
	AnthropicAdaptiveSchedulerConsecutiveFailurePenalty  float64 `json:"anthropic_adaptive_scheduler_consecutive_failure_penalty"`
	AnthropicAdaptiveSchedulerSuccessEMAAlpha            float64 `json:"anthropic_adaptive_scheduler_success_ema_alpha"`
	AnthropicAdaptiveSchedulerLatencyEMAAlpha            float64 `json:"anthropic_adaptive_scheduler_latency_ema_alpha"`
	AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold float64 `json:"anthropic_adaptive_scheduler_capacity_probe_load_threshold"`
	AnthropicAdaptiveSchedulerLearningWindowSeconds      int     `json:"anthropic_adaptive_scheduler_learning_window_seconds"`
	AnthropicAdaptiveSchedulerCooldownSeconds            int     `json:"anthropic_adaptive_scheduler_cooldown_seconds"`
	AnthropicAdaptiveSchedulerShrinkFactorSoft           float64 `json:"anthropic_adaptive_scheduler_shrink_factor_soft"`
	AnthropicAdaptiveSchedulerExplorationRate            float64 `json:"anthropic_adaptive_scheduler_exploration_rate"`
	AnthropicAdaptiveSchedulerWeightCost                 float64 `json:"anthropic_adaptive_scheduler_weight_cost"`
	AnthropicAdaptiveSchedulerLearningMinHealthSamples   int     `json:"anthropic_adaptive_scheduler_learning_min_health_samples"`
	AnthropicAdaptiveSchedulerHealthFailureThreshold     int     `json:"anthropic_adaptive_scheduler_health_failure_threshold"`
	AnthropicAdaptiveSchedulerCooldownMaxSeconds         int     `json:"anthropic_adaptive_scheduler_cooldown_max_seconds"`
	AnthropicAdaptiveSchedulerHighErrorMinSamples        int     `json:"anthropic_adaptive_scheduler_high_error_min_samples"`
	AnthropicAdaptiveSchedulerHighErrorMaxSamples        int     `json:"anthropic_adaptive_scheduler_high_error_max_samples"`
	AnthropicAdaptiveSchedulerHighErrorEnterRate         float64 `json:"anthropic_adaptive_scheduler_high_error_enter_rate"`
	AnthropicAdaptiveSchedulerHighErrorExitRate          float64 `json:"anthropic_adaptive_scheduler_high_error_exit_rate"`
	AnthropicAdaptiveSchedulerCapacityRecoverySamples    int     `json:"anthropic_adaptive_scheduler_capacity_recovery_samples"`
	AnthropicAdaptiveSchedulerCapacityGrowthFactor       float64 `json:"anthropic_adaptive_scheduler_capacity_growth_factor"`
	AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds  int     `json:"anthropic_adaptive_scheduler_quota_probe_interval_seconds"`
}

type cachedAnthropicAdaptiveSchedulerSettings struct {
	settings  AnthropicAdaptiveSchedulerSettings
	expiresAt int64
}

var anthropicAdaptiveSchedulerSettingCache atomic.Value // *cachedAnthropicAdaptiveSchedulerSettings
var anthropicAdaptiveSchedulerSettingSF singleflight.Group
var anthropicAdaptiveSchedulerSettingGeneration atomic.Uint64

func DefaultAnthropicAdaptiveSchedulerSettings() AnthropicAdaptiveSchedulerSettings {
	return AnthropicAdaptiveSchedulerSettings{
		AnthropicAdaptiveSchedulerEnabled:                    false,
		AnthropicAdaptiveSchedulerDiagnosticLogEnabled:       false,
		AnthropicAdaptiveSchedulerDiagnosticLogSampleRate:    0.05,
		AnthropicAdaptiveSchedulerMode:                       AnthropicAdaptiveSchedulerModeShadow,
		AnthropicAdaptiveSchedulerTopK:                       8,
		AnthropicAdaptiveSchedulerSoftmaxTemperature:         0.35,
		AnthropicAdaptiveSchedulerWeightReliability:          0.50,
		AnthropicAdaptiveSchedulerWeightCapacity:             0.20,
		AnthropicAdaptiveSchedulerWeightLatency:              0.15,
		AnthropicAdaptiveSchedulerWeightCache:                0,
		AnthropicAdaptiveSchedulerConsecutiveFailurePenalty:  0.25,
		AnthropicAdaptiveSchedulerSuccessEMAAlpha:            0.05,
		AnthropicAdaptiveSchedulerLatencyEMAAlpha:            0.05,
		AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold: 0.80,
		AnthropicAdaptiveSchedulerLearningWindowSeconds:      1200,
		AnthropicAdaptiveSchedulerCooldownSeconds:            60,
		AnthropicAdaptiveSchedulerShrinkFactorSoft:           0.90,
		AnthropicAdaptiveSchedulerExplorationRate:            0.02,
		AnthropicAdaptiveSchedulerWeightCost:                 0.15,
		AnthropicAdaptiveSchedulerLearningMinHealthSamples:   30,
		AnthropicAdaptiveSchedulerHealthFailureThreshold:     3,
		AnthropicAdaptiveSchedulerCooldownMaxSeconds:         600,
		AnthropicAdaptiveSchedulerHighErrorMinSamples:        10,
		AnthropicAdaptiveSchedulerHighErrorMaxSamples:        100,
		AnthropicAdaptiveSchedulerHighErrorEnterRate:         0.25,
		AnthropicAdaptiveSchedulerHighErrorExitRate:          0.15,
		AnthropicAdaptiveSchedulerCapacityRecoverySamples:    30,
		AnthropicAdaptiveSchedulerCapacityGrowthFactor:       1.15,
		AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds:  300,
	}
}

func NormalizeAnthropicAdaptiveSchedulerSettings(settings AnthropicAdaptiveSchedulerSettings) AnthropicAdaptiveSchedulerSettings {
	defaults := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = clampFloat(settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate, 0, 1, defaults.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	settings.AnthropicAdaptiveSchedulerTopK = clampInt(settings.AnthropicAdaptiveSchedulerTopK, 1, 100, defaults.AnthropicAdaptiveSchedulerTopK)
	settings.AnthropicAdaptiveSchedulerSoftmaxTemperature = clampFloat(settings.AnthropicAdaptiveSchedulerSoftmaxTemperature, 0.01, 10, defaults.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha = clampFloat(settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha, 0, 1, defaults.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
	settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha = clampFloat(settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha, 0, 1, defaults.AnthropicAdaptiveSchedulerLatencyEMAAlpha)
	settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = clampFloat(settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold, 0, 1, defaults.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.AnthropicAdaptiveSchedulerLearningWindowSeconds = clampIntMin(settings.AnthropicAdaptiveSchedulerLearningWindowSeconds, 1, defaults.AnthropicAdaptiveSchedulerLearningWindowSeconds)
	settings.AnthropicAdaptiveSchedulerCooldownSeconds = clampIntMin(settings.AnthropicAdaptiveSchedulerCooldownSeconds, 0, defaults.AnthropicAdaptiveSchedulerCooldownSeconds)
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = clampFloat(settings.AnthropicAdaptiveSchedulerShrinkFactorSoft, 0.01, 1, defaults.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	settings.AnthropicAdaptiveSchedulerExplorationRate = clampFloat(settings.AnthropicAdaptiveSchedulerExplorationRate, 0, 1, defaults.AnthropicAdaptiveSchedulerExplorationRate)
	settings.AnthropicAdaptiveSchedulerWeightCost = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightCost)
	settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples = clampIntMin(settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples, 1, defaults.AnthropicAdaptiveSchedulerLearningMinHealthSamples)
	settings.AnthropicAdaptiveSchedulerHealthFailureThreshold = clampIntMin(settings.AnthropicAdaptiveSchedulerHealthFailureThreshold, 1, defaults.AnthropicAdaptiveSchedulerHealthFailureThreshold)
	settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds = clampIntMin(settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds, settings.AnthropicAdaptiveSchedulerCooldownSeconds, defaults.AnthropicAdaptiveSchedulerCooldownMaxSeconds)
	settings.AnthropicAdaptiveSchedulerHighErrorMinSamples = clampIntMin(settings.AnthropicAdaptiveSchedulerHighErrorMinSamples, 1, defaults.AnthropicAdaptiveSchedulerHighErrorMinSamples)
	settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples = clampIntMin(settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples, settings.AnthropicAdaptiveSchedulerHighErrorMinSamples, defaults.AnthropicAdaptiveSchedulerHighErrorMaxSamples)
	settings.AnthropicAdaptiveSchedulerHighErrorEnterRate = clampFloat(settings.AnthropicAdaptiveSchedulerHighErrorEnterRate, 0, 1, defaults.AnthropicAdaptiveSchedulerHighErrorEnterRate)
	settings.AnthropicAdaptiveSchedulerHighErrorExitRate = clampFloat(settings.AnthropicAdaptiveSchedulerHighErrorExitRate, 0, settings.AnthropicAdaptiveSchedulerHighErrorEnterRate, defaults.AnthropicAdaptiveSchedulerHighErrorExitRate)
	settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples = clampIntMin(settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples, 1, defaults.AnthropicAdaptiveSchedulerCapacityRecoverySamples)
	settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor = clampFloat(settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor, 1.01, 10, defaults.AnthropicAdaptiveSchedulerCapacityGrowthFactor)
	settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds = clampIntMin(settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds, 1, defaults.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds)
	settings.AnthropicAdaptiveSchedulerWeightReliability = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightReliability)
	settings.AnthropicAdaptiveSchedulerWeightCapacity = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightCapacity)
	settings.AnthropicAdaptiveSchedulerWeightLatency = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightLatency)
	settings.AnthropicAdaptiveSchedulerWeightCache = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightCache)
	weightSum := settings.AnthropicAdaptiveSchedulerWeightReliability +
		settings.AnthropicAdaptiveSchedulerWeightCapacity +
		settings.AnthropicAdaptiveSchedulerWeightLatency +
		settings.AnthropicAdaptiveSchedulerWeightCost +
		settings.AnthropicAdaptiveSchedulerWeightCache
	if weightSum <= 0 {
		settings.AnthropicAdaptiveSchedulerWeightReliability = defaults.AnthropicAdaptiveSchedulerWeightReliability
		settings.AnthropicAdaptiveSchedulerWeightCapacity = defaults.AnthropicAdaptiveSchedulerWeightCapacity
		settings.AnthropicAdaptiveSchedulerWeightLatency = defaults.AnthropicAdaptiveSchedulerWeightLatency
		settings.AnthropicAdaptiveSchedulerWeightCost = defaults.AnthropicAdaptiveSchedulerWeightCost
	}
	return settings
}

func normalizeAnthropicAdaptiveSchedulerMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AnthropicAdaptiveSchedulerModeEnforce:
		return AnthropicAdaptiveSchedulerModeEnforce
	case AnthropicAdaptiveSchedulerModeShadow:
		return AnthropicAdaptiveSchedulerModeShadow
	default:
		return AnthropicAdaptiveSchedulerModeShadow
	}
}

func parseAnthropicAdaptiveSchedulerSettings(values map[string]string) AnthropicAdaptiveSchedulerSettings {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerEnabled = parseBoolSetting(values, SettingKeyAnthropicAdaptiveSchedulerEnabled, settings.AnthropicAdaptiveSchedulerEnabled)
	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = parseBoolSetting(values, SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled, settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled)
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate, settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate)
	settings.AnthropicAdaptiveSchedulerMode = firstNonEmpty(values[SettingKeyAnthropicAdaptiveSchedulerMode], settings.AnthropicAdaptiveSchedulerMode)
	settings.AnthropicAdaptiveSchedulerTopK = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerTopK, settings.AnthropicAdaptiveSchedulerTopK)
	settings.AnthropicAdaptiveSchedulerSoftmaxTemperature = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature, settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	settings.AnthropicAdaptiveSchedulerWeightReliability = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightReliability, settings.AnthropicAdaptiveSchedulerWeightReliability)
	settings.AnthropicAdaptiveSchedulerWeightCapacity = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightCapacity, settings.AnthropicAdaptiveSchedulerWeightCapacity)
	settings.AnthropicAdaptiveSchedulerWeightLatency = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightLatency, settings.AnthropicAdaptiveSchedulerWeightLatency)
	settings.AnthropicAdaptiveSchedulerWeightCache = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightCache, settings.AnthropicAdaptiveSchedulerWeightCache)
	settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty, settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha, settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
	settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha, settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha)
	settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold, settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.AnthropicAdaptiveSchedulerLearningWindowSeconds = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds, settings.AnthropicAdaptiveSchedulerLearningWindowSeconds)
	settings.AnthropicAdaptiveSchedulerCooldownSeconds = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds, settings.AnthropicAdaptiveSchedulerCooldownSeconds)
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft, settings.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	settings.AnthropicAdaptiveSchedulerExplorationRate = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerExplorationRate, settings.AnthropicAdaptiveSchedulerExplorationRate)
	settings.AnthropicAdaptiveSchedulerWeightCost = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightCost, settings.AnthropicAdaptiveSchedulerWeightCost)
	settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerLearningMinHealthSamples, settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples)
	settings.AnthropicAdaptiveSchedulerHealthFailureThreshold = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerHealthFailureThreshold, settings.AnthropicAdaptiveSchedulerHealthFailureThreshold)
	settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerCooldownMaxSeconds, settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds)
	settings.AnthropicAdaptiveSchedulerHighErrorMinSamples = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerHighErrorMinSamples, settings.AnthropicAdaptiveSchedulerHighErrorMinSamples)
	settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerHighErrorMaxSamples, settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples)
	settings.AnthropicAdaptiveSchedulerHighErrorEnterRate = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerHighErrorEnterRate, settings.AnthropicAdaptiveSchedulerHighErrorEnterRate)
	settings.AnthropicAdaptiveSchedulerHighErrorExitRate = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerHighErrorExitRate, settings.AnthropicAdaptiveSchedulerHighErrorExitRate)
	settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacityRecoverySamples, settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples)
	settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacityGrowthFactor, settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor)
	settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds, settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds)
	return NormalizeAnthropicAdaptiveSchedulerSettings(settings)
}

func anthropicAdaptiveSchedulerSettingsToMap(settings AnthropicAdaptiveSchedulerSettings) map[string]string {
	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)
	return map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled:                    strconv.FormatBool(settings.AnthropicAdaptiveSchedulerEnabled),
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled:       strconv.FormatBool(settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled),
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate:    formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate),
		SettingKeyAnthropicAdaptiveSchedulerMode:                       settings.AnthropicAdaptiveSchedulerMode,
		SettingKeyAnthropicAdaptiveSchedulerTopK:                       strconv.Itoa(settings.AnthropicAdaptiveSchedulerTopK),
		SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature:         formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerSoftmaxTemperature),
		SettingKeyAnthropicAdaptiveSchedulerWeightReliability:          formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightReliability),
		SettingKeyAnthropicAdaptiveSchedulerWeightCapacity:             formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightCapacity),
		SettingKeyAnthropicAdaptiveSchedulerWeightLatency:              formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightLatency),
		SettingKeyAnthropicAdaptiveSchedulerWeightCache:                formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightCache),
		SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty:  formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty),
		SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha:            formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha),
		SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha:            formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha),
		SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold: formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold),
		SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds:      strconv.Itoa(settings.AnthropicAdaptiveSchedulerLearningWindowSeconds),
		SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds:            strconv.Itoa(settings.AnthropicAdaptiveSchedulerCooldownSeconds),
		SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft:           formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerShrinkFactorSoft),
		SettingKeyAnthropicAdaptiveSchedulerExplorationRate:            formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerExplorationRate),
		SettingKeyAnthropicAdaptiveSchedulerWeightCost:                 formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightCost),
		SettingKeyAnthropicAdaptiveSchedulerLearningMinHealthSamples:   strconv.Itoa(settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples),
		SettingKeyAnthropicAdaptiveSchedulerHealthFailureThreshold:     strconv.Itoa(settings.AnthropicAdaptiveSchedulerHealthFailureThreshold),
		SettingKeyAnthropicAdaptiveSchedulerCooldownMaxSeconds:         strconv.Itoa(settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds),
		SettingKeyAnthropicAdaptiveSchedulerHighErrorMinSamples:        strconv.Itoa(settings.AnthropicAdaptiveSchedulerHighErrorMinSamples),
		SettingKeyAnthropicAdaptiveSchedulerHighErrorMaxSamples:        strconv.Itoa(settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples),
		SettingKeyAnthropicAdaptiveSchedulerHighErrorEnterRate:         formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerHighErrorEnterRate),
		SettingKeyAnthropicAdaptiveSchedulerHighErrorExitRate:          formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerHighErrorExitRate),
		SettingKeyAnthropicAdaptiveSchedulerCapacityRecoverySamples:    strconv.Itoa(settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples),
		SettingKeyAnthropicAdaptiveSchedulerCapacityGrowthFactor:       formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor),
		SettingKeyAnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds:  strconv.Itoa(settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds),
	}
}

func (s *SettingService) GetAnthropicAdaptiveSchedulerSettings(ctx context.Context) (AnthropicAdaptiveSchedulerSettings, error) {
	defaults := DefaultAnthropicAdaptiveSchedulerSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	if cached, _ := anthropicAdaptiveSchedulerSettingCache.Load().(*cachedAnthropicAdaptiveSchedulerSettings); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.settings, nil
	}

	generation := anthropicAdaptiveSchedulerSettingGeneration.Load()
	value, err, _ := anthropicAdaptiveSchedulerSettingSF.Do("settings", func() (any, error) {
		if cached, _ := anthropicAdaptiveSchedulerSettingCache.Load().(*cachedAnthropicAdaptiveSchedulerSettings); cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached.settings, nil
		}
		dbCtx, cancel := context.WithTimeout(ctx, anthropicAdaptiveSchedulerSettingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyAnthropicAdaptiveSchedulerEnabled,
			SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled,
			SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate,
			SettingKeyAnthropicAdaptiveSchedulerMode,
			SettingKeyAnthropicAdaptiveSchedulerTopK,
			SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature,
			SettingKeyAnthropicAdaptiveSchedulerWeightReliability,
			SettingKeyAnthropicAdaptiveSchedulerWeightCapacity,
			SettingKeyAnthropicAdaptiveSchedulerWeightLatency,
			SettingKeyAnthropicAdaptiveSchedulerWeightCache,
			SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty,
			SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha,
			SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha,
			SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold,
			SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds,
			SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds,
			SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft,
			SettingKeyAnthropicAdaptiveSchedulerExplorationRate,
			SettingKeyAnthropicAdaptiveSchedulerWeightCost,
			SettingKeyAnthropicAdaptiveSchedulerLearningMinHealthSamples,
			SettingKeyAnthropicAdaptiveSchedulerHealthFailureThreshold,
			SettingKeyAnthropicAdaptiveSchedulerCooldownMaxSeconds,
			SettingKeyAnthropicAdaptiveSchedulerHighErrorMinSamples,
			SettingKeyAnthropicAdaptiveSchedulerHighErrorMaxSamples,
			SettingKeyAnthropicAdaptiveSchedulerHighErrorEnterRate,
			SettingKeyAnthropicAdaptiveSchedulerHighErrorExitRate,
			SettingKeyAnthropicAdaptiveSchedulerCapacityRecoverySamples,
			SettingKeyAnthropicAdaptiveSchedulerCapacityGrowthFactor,
			SettingKeyAnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds,
		})
		if err != nil {
			return defaults, err
		}
		settings := parseAnthropicAdaptiveSchedulerSettings(values)
		if anthropicAdaptiveSchedulerSettingGeneration.Load() == generation {
			anthropicAdaptiveSchedulerSettingCache.Store(&cachedAnthropicAdaptiveSchedulerSettings{
				settings:  settings,
				expiresAt: time.Now().Add(anthropicAdaptiveSchedulerSettingCacheTTL).UnixNano(),
			})
		}
		return settings, nil
	})
	if err != nil {
		return defaults, err
	}
	settings, ok := value.(AnthropicAdaptiveSchedulerSettings)
	if !ok {
		return defaults, fmt.Errorf("unexpected Anthropic adaptive scheduler settings type %T", value)
	}
	return settings, nil
}

func refreshAnthropicAdaptiveSchedulerSettingCache(settings AnthropicAdaptiveSchedulerSettings) {
	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)
	anthropicAdaptiveSchedulerSettingGeneration.Add(1)
	anthropicAdaptiveSchedulerSettingSF.Forget("settings")
	anthropicAdaptiveSchedulerSettingCache.Store(&cachedAnthropicAdaptiveSchedulerSettings{
		settings:  settings,
		expiresAt: time.Now().Add(anthropicAdaptiveSchedulerSettingCacheTTL).UnixNano(),
	})
}
