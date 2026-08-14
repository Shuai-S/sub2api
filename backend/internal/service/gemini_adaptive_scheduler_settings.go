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
	GeminiAdaptiveSchedulerModeShadow  = "shadow"
	GeminiAdaptiveSchedulerModeEnforce = "enforce"
)

const (
	geminiAdaptiveSchedulerSettingPrefix = "gemini_adaptive_scheduler_"

	SettingKeyGeminiAdaptiveSchedulerEnabled                    = geminiAdaptiveSchedulerSettingPrefix + "enabled"
	SettingKeyGeminiAdaptiveSchedulerMode                       = geminiAdaptiveSchedulerSettingPrefix + "mode"
	SettingKeyGeminiAdaptiveSchedulerTopK                       = geminiAdaptiveSchedulerSettingPrefix + "top_k"
	SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature         = geminiAdaptiveSchedulerSettingPrefix + "softmax_temperature"
	SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty  = geminiAdaptiveSchedulerSettingPrefix + "consecutive_failure_penalty"
	SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha            = geminiAdaptiveSchedulerSettingPrefix + "success_ema_alpha"
	SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha            = geminiAdaptiveSchedulerSettingPrefix + "latency_ema_alpha"
	SettingKeyGeminiAdaptiveSchedulerWeightReliability          = geminiAdaptiveSchedulerSettingPrefix + "weight_reliability"
	SettingKeyGeminiAdaptiveSchedulerWeightCapacity             = geminiAdaptiveSchedulerSettingPrefix + "weight_capacity"
	SettingKeyGeminiAdaptiveSchedulerWeightLatency              = geminiAdaptiveSchedulerSettingPrefix + "weight_latency"
	SettingKeyGeminiAdaptiveSchedulerWeightCost                 = geminiAdaptiveSchedulerSettingPrefix + "weight_cost"
	SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold = geminiAdaptiveSchedulerSettingPrefix + "capacity_probe_load_threshold"
	SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft           = geminiAdaptiveSchedulerSettingPrefix + "shrink_factor_soft"
	SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds      = geminiAdaptiveSchedulerSettingPrefix + "learning_window_seconds"
	SettingKeyGeminiAdaptiveSchedulerCooldownSeconds            = geminiAdaptiveSchedulerSettingPrefix + "cooldown_seconds"
	SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds         = geminiAdaptiveSchedulerSettingPrefix + "cooldown_max_seconds"
	SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold    = geminiAdaptiveSchedulerSettingPrefix + "account_failure_threshold"
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled       = geminiAdaptiveSchedulerSettingPrefix + "diagnostic_log_enabled"
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate    = geminiAdaptiveSchedulerSettingPrefix + "diagnostic_log_sample_rate"
	SettingKeyGeminiAdaptiveSchedulerExplorationRate            = geminiAdaptiveSchedulerSettingPrefix + "exploration_rate"
	SettingKeyGeminiAdaptiveSchedulerLearningMinHealthSamples   = geminiAdaptiveSchedulerSettingPrefix + "learning_min_health_samples"
	SettingKeyGeminiAdaptiveSchedulerHighErrorMinSamples        = geminiAdaptiveSchedulerSettingPrefix + "high_error_min_samples"
	SettingKeyGeminiAdaptiveSchedulerHighErrorMaxSamples        = geminiAdaptiveSchedulerSettingPrefix + "high_error_max_samples"
	SettingKeyGeminiAdaptiveSchedulerHighErrorEnterRate         = geminiAdaptiveSchedulerSettingPrefix + "high_error_enter_rate"
	SettingKeyGeminiAdaptiveSchedulerHighErrorExitRate          = geminiAdaptiveSchedulerSettingPrefix + "high_error_exit_rate"
	SettingKeyGeminiAdaptiveSchedulerCapacityRecoverySamples    = geminiAdaptiveSchedulerSettingPrefix + "capacity_recovery_samples"
	SettingKeyGeminiAdaptiveSchedulerCapacityGrowthFactor       = geminiAdaptiveSchedulerSettingPrefix + "capacity_growth_factor"
	SettingKeyGeminiAdaptiveSchedulerQuotaProbeIntervalSeconds  = geminiAdaptiveSchedulerSettingPrefix + "quota_probe_interval_seconds"

	geminiAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	geminiAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
)

type GeminiAdaptiveSchedulerSettings struct {
	GeminiAdaptiveSchedulerEnabled                    bool    `json:"gemini_adaptive_scheduler_enabled"`
	GeminiAdaptiveSchedulerMode                       string  `json:"gemini_adaptive_scheduler_mode"`
	GeminiAdaptiveSchedulerTopK                       int     `json:"gemini_adaptive_scheduler_top_k"`
	GeminiAdaptiveSchedulerSoftmaxTemperature         float64 `json:"gemini_adaptive_scheduler_softmax_temperature"`
	GeminiAdaptiveSchedulerConsecutiveFailurePenalty  float64 `json:"gemini_adaptive_scheduler_consecutive_failure_penalty"`
	GeminiAdaptiveSchedulerSuccessEMAAlpha            float64 `json:"gemini_adaptive_scheduler_success_ema_alpha"`
	GeminiAdaptiveSchedulerLatencyEMAAlpha            float64 `json:"gemini_adaptive_scheduler_latency_ema_alpha"`
	GeminiAdaptiveSchedulerWeightReliability          float64 `json:"gemini_adaptive_scheduler_weight_reliability"`
	GeminiAdaptiveSchedulerWeightCapacity             float64 `json:"gemini_adaptive_scheduler_weight_capacity"`
	GeminiAdaptiveSchedulerWeightLatency              float64 `json:"gemini_adaptive_scheduler_weight_latency"`
	GeminiAdaptiveSchedulerWeightCost                 float64 `json:"gemini_adaptive_scheduler_weight_cost"`
	GeminiAdaptiveSchedulerCapacityProbeLoadThreshold float64 `json:"gemini_adaptive_scheduler_capacity_probe_load_threshold"`
	GeminiAdaptiveSchedulerShrinkFactorSoft           float64 `json:"gemini_adaptive_scheduler_shrink_factor_soft"`
	GeminiAdaptiveSchedulerLearningWindowSeconds      int     `json:"gemini_adaptive_scheduler_learning_window_seconds"`
	GeminiAdaptiveSchedulerCooldownSeconds            int     `json:"gemini_adaptive_scheduler_cooldown_seconds"`
	GeminiAdaptiveSchedulerCooldownMaxSeconds         int     `json:"gemini_adaptive_scheduler_cooldown_max_seconds"`
	GeminiAdaptiveSchedulerAccountFailureThreshold    int     `json:"gemini_adaptive_scheduler_account_failure_threshold"`
	GeminiAdaptiveSchedulerDiagnosticLogEnabled       bool    `json:"gemini_adaptive_scheduler_diagnostic_log_enabled"`
	GeminiAdaptiveSchedulerDiagnosticLogSampleRate    float64 `json:"gemini_adaptive_scheduler_diagnostic_log_sample_rate"`
	GeminiAdaptiveSchedulerExplorationRate            float64 `json:"gemini_adaptive_scheduler_exploration_rate"`
	GeminiAdaptiveSchedulerLearningMinHealthSamples   int     `json:"gemini_adaptive_scheduler_learning_min_health_samples"`
	GeminiAdaptiveSchedulerHighErrorMinSamples        int     `json:"gemini_adaptive_scheduler_high_error_min_samples"`
	GeminiAdaptiveSchedulerHighErrorMaxSamples        int     `json:"gemini_adaptive_scheduler_high_error_max_samples"`
	GeminiAdaptiveSchedulerHighErrorEnterRate         float64 `json:"gemini_adaptive_scheduler_high_error_enter_rate"`
	GeminiAdaptiveSchedulerHighErrorExitRate          float64 `json:"gemini_adaptive_scheduler_high_error_exit_rate"`
	GeminiAdaptiveSchedulerCapacityRecoverySamples    int     `json:"gemini_adaptive_scheduler_capacity_recovery_samples"`
	GeminiAdaptiveSchedulerCapacityGrowthFactor       float64 `json:"gemini_adaptive_scheduler_capacity_growth_factor"`
	GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds  int     `json:"gemini_adaptive_scheduler_quota_probe_interval_seconds"`
}

type cachedGeminiAdaptiveSchedulerSettings struct {
	settings  GeminiAdaptiveSchedulerSettings
	expiresAt int64
}

var geminiAdaptiveSchedulerSettingCache atomic.Value // *cachedGeminiAdaptiveSchedulerSettings
var geminiAdaptiveSchedulerSettingSF singleflight.Group
var geminiAdaptiveSchedulerSettingGeneration atomic.Uint64

func DefaultGeminiAdaptiveSchedulerSettings() GeminiAdaptiveSchedulerSettings {
	return GeminiAdaptiveSchedulerSettings{
		GeminiAdaptiveSchedulerEnabled:                    false,
		GeminiAdaptiveSchedulerMode:                       GeminiAdaptiveSchedulerModeShadow,
		GeminiAdaptiveSchedulerTopK:                       8,
		GeminiAdaptiveSchedulerSoftmaxTemperature:         0.35,
		GeminiAdaptiveSchedulerConsecutiveFailurePenalty:  0.25,
		GeminiAdaptiveSchedulerSuccessEMAAlpha:            0.05,
		GeminiAdaptiveSchedulerLatencyEMAAlpha:            0.05,
		GeminiAdaptiveSchedulerWeightReliability:          0.50,
		GeminiAdaptiveSchedulerWeightCapacity:             0.20,
		GeminiAdaptiveSchedulerWeightLatency:              0.15,
		GeminiAdaptiveSchedulerWeightCost:                 0.15,
		GeminiAdaptiveSchedulerCapacityProbeLoadThreshold: 0.80,
		GeminiAdaptiveSchedulerShrinkFactorSoft:           0.90,
		GeminiAdaptiveSchedulerLearningWindowSeconds:      1200,
		GeminiAdaptiveSchedulerCooldownSeconds:            60,
		GeminiAdaptiveSchedulerCooldownMaxSeconds:         600,
		GeminiAdaptiveSchedulerAccountFailureThreshold:    3,
		GeminiAdaptiveSchedulerDiagnosticLogEnabled:       false,
		GeminiAdaptiveSchedulerDiagnosticLogSampleRate:    0.05,
		GeminiAdaptiveSchedulerExplorationRate:            0.02,
		GeminiAdaptiveSchedulerLearningMinHealthSamples:   30,
		GeminiAdaptiveSchedulerHighErrorMinSamples:        10,
		GeminiAdaptiveSchedulerHighErrorMaxSamples:        100,
		GeminiAdaptiveSchedulerHighErrorEnterRate:         0.25,
		GeminiAdaptiveSchedulerHighErrorExitRate:          0.15,
		GeminiAdaptiveSchedulerCapacityRecoverySamples:    30,
		GeminiAdaptiveSchedulerCapacityGrowthFactor:       1.15,
		GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds:  300,
	}
}

func NormalizeGeminiAdaptiveSchedulerSettings(settings GeminiAdaptiveSchedulerSettings) GeminiAdaptiveSchedulerSettings {
	defaults := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = normalizeGeminiAdaptiveSchedulerMode(settings.GeminiAdaptiveSchedulerMode)
	settings.GeminiAdaptiveSchedulerTopK = clampInt(settings.GeminiAdaptiveSchedulerTopK, 1, 100, defaults.GeminiAdaptiveSchedulerTopK)
	settings.GeminiAdaptiveSchedulerSoftmaxTemperature = clampFloat(settings.GeminiAdaptiveSchedulerSoftmaxTemperature, 0.01, 10, defaults.GeminiAdaptiveSchedulerSoftmaxTemperature)
	settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty = nonNegativeFinite(settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.GeminiAdaptiveSchedulerSuccessEMAAlpha = clampFloat(settings.GeminiAdaptiveSchedulerSuccessEMAAlpha, 0, 1, defaults.GeminiAdaptiveSchedulerSuccessEMAAlpha)
	settings.GeminiAdaptiveSchedulerLatencyEMAAlpha = clampFloat(settings.GeminiAdaptiveSchedulerLatencyEMAAlpha, 0, 1, defaults.GeminiAdaptiveSchedulerLatencyEMAAlpha)
	settings.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold = clampFloat(settings.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold, 0, 1, defaults.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.GeminiAdaptiveSchedulerShrinkFactorSoft = clampFloat(settings.GeminiAdaptiveSchedulerShrinkFactorSoft, 0.01, 1, defaults.GeminiAdaptiveSchedulerShrinkFactorSoft)
	settings.GeminiAdaptiveSchedulerLearningWindowSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerLearningWindowSeconds, 1, defaults.GeminiAdaptiveSchedulerLearningWindowSeconds)
	settings.GeminiAdaptiveSchedulerCooldownSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerCooldownSeconds, 0, defaults.GeminiAdaptiveSchedulerCooldownSeconds)
	settings.GeminiAdaptiveSchedulerCooldownMaxSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerCooldownMaxSeconds, settings.GeminiAdaptiveSchedulerCooldownSeconds, defaults.GeminiAdaptiveSchedulerCooldownMaxSeconds)
	settings.GeminiAdaptiveSchedulerAccountFailureThreshold = clampInt(settings.GeminiAdaptiveSchedulerAccountFailureThreshold, 1, 100, defaults.GeminiAdaptiveSchedulerAccountFailureThreshold)
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = clampFloat(settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate, 0, 1, defaults.GeminiAdaptiveSchedulerDiagnosticLogSampleRate)
	settings.GeminiAdaptiveSchedulerExplorationRate = clampFloat(settings.GeminiAdaptiveSchedulerExplorationRate, 0, 1, defaults.GeminiAdaptiveSchedulerExplorationRate)
	settings.GeminiAdaptiveSchedulerLearningMinHealthSamples = clampIntMin(settings.GeminiAdaptiveSchedulerLearningMinHealthSamples, 1, defaults.GeminiAdaptiveSchedulerLearningMinHealthSamples)
	settings.GeminiAdaptiveSchedulerHighErrorMinSamples = clampIntMin(settings.GeminiAdaptiveSchedulerHighErrorMinSamples, 1, defaults.GeminiAdaptiveSchedulerHighErrorMinSamples)
	settings.GeminiAdaptiveSchedulerHighErrorMaxSamples = clampIntMin(settings.GeminiAdaptiveSchedulerHighErrorMaxSamples, settings.GeminiAdaptiveSchedulerHighErrorMinSamples, defaults.GeminiAdaptiveSchedulerHighErrorMaxSamples)
	settings.GeminiAdaptiveSchedulerHighErrorEnterRate = clampFloat(settings.GeminiAdaptiveSchedulerHighErrorEnterRate, 0, 1, defaults.GeminiAdaptiveSchedulerHighErrorEnterRate)
	settings.GeminiAdaptiveSchedulerHighErrorExitRate = clampFloat(settings.GeminiAdaptiveSchedulerHighErrorExitRate, 0, settings.GeminiAdaptiveSchedulerHighErrorEnterRate, defaults.GeminiAdaptiveSchedulerHighErrorExitRate)
	settings.GeminiAdaptiveSchedulerCapacityRecoverySamples = clampIntMin(settings.GeminiAdaptiveSchedulerCapacityRecoverySamples, 1, defaults.GeminiAdaptiveSchedulerCapacityRecoverySamples)
	settings.GeminiAdaptiveSchedulerCapacityGrowthFactor = clampFloat(settings.GeminiAdaptiveSchedulerCapacityGrowthFactor, 1.01, 10, defaults.GeminiAdaptiveSchedulerCapacityGrowthFactor)
	settings.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds, 1, defaults.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds)

	weights := []*float64{
		&settings.GeminiAdaptiveSchedulerWeightReliability,
		&settings.GeminiAdaptiveSchedulerWeightCapacity,
		&settings.GeminiAdaptiveSchedulerWeightLatency,
		&settings.GeminiAdaptiveSchedulerWeightCost,
	}
	weightSum := 0.0
	for _, weight := range weights {
		*weight = nonNegativeFinite(*weight)
		weightSum += *weight
	}
	if weightSum <= 0 {
		settings.GeminiAdaptiveSchedulerWeightReliability = defaults.GeminiAdaptiveSchedulerWeightReliability
		settings.GeminiAdaptiveSchedulerWeightCapacity = defaults.GeminiAdaptiveSchedulerWeightCapacity
		settings.GeminiAdaptiveSchedulerWeightLatency = defaults.GeminiAdaptiveSchedulerWeightLatency
		settings.GeminiAdaptiveSchedulerWeightCost = defaults.GeminiAdaptiveSchedulerWeightCost
	}
	return settings
}

func normalizeGeminiAdaptiveSchedulerMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case GeminiAdaptiveSchedulerModeEnforce:
		return GeminiAdaptiveSchedulerModeEnforce
	default:
		return GeminiAdaptiveSchedulerModeShadow
	}
}

var geminiAdaptiveSchedulerSettingKeys = []string{
	SettingKeyGeminiAdaptiveSchedulerEnabled,
	SettingKeyGeminiAdaptiveSchedulerMode,
	SettingKeyGeminiAdaptiveSchedulerTopK,
	SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature,
	SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty,
	SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha,
	SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha,
	SettingKeyGeminiAdaptiveSchedulerWeightReliability,
	SettingKeyGeminiAdaptiveSchedulerWeightCapacity,
	SettingKeyGeminiAdaptiveSchedulerWeightLatency,
	SettingKeyGeminiAdaptiveSchedulerWeightCost,
	SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold,
	SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft,
	SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds,
	SettingKeyGeminiAdaptiveSchedulerCooldownSeconds,
	SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds,
	SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold,
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled,
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate,
	SettingKeyGeminiAdaptiveSchedulerExplorationRate,
	SettingKeyGeminiAdaptiveSchedulerLearningMinHealthSamples,
	SettingKeyGeminiAdaptiveSchedulerHighErrorMinSamples,
	SettingKeyGeminiAdaptiveSchedulerHighErrorMaxSamples,
	SettingKeyGeminiAdaptiveSchedulerHighErrorEnterRate,
	SettingKeyGeminiAdaptiveSchedulerHighErrorExitRate,
	SettingKeyGeminiAdaptiveSchedulerCapacityRecoverySamples,
	SettingKeyGeminiAdaptiveSchedulerCapacityGrowthFactor,
	SettingKeyGeminiAdaptiveSchedulerQuotaProbeIntervalSeconds,
}

func parseGeminiAdaptiveSchedulerSettings(values map[string]string) GeminiAdaptiveSchedulerSettings {
	s := DefaultGeminiAdaptiveSchedulerSettings()
	s.GeminiAdaptiveSchedulerEnabled = parseBoolSetting(values, SettingKeyGeminiAdaptiveSchedulerEnabled, s.GeminiAdaptiveSchedulerEnabled)
	s.GeminiAdaptiveSchedulerMode = firstNonEmpty(values[SettingKeyGeminiAdaptiveSchedulerMode], s.GeminiAdaptiveSchedulerMode)
	s.GeminiAdaptiveSchedulerTopK = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerTopK, s.GeminiAdaptiveSchedulerTopK)
	s.GeminiAdaptiveSchedulerSoftmaxTemperature = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature, s.GeminiAdaptiveSchedulerSoftmaxTemperature)
	s.GeminiAdaptiveSchedulerConsecutiveFailurePenalty = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty, s.GeminiAdaptiveSchedulerConsecutiveFailurePenalty)
	s.GeminiAdaptiveSchedulerSuccessEMAAlpha = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha, s.GeminiAdaptiveSchedulerSuccessEMAAlpha)
	s.GeminiAdaptiveSchedulerLatencyEMAAlpha = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha, s.GeminiAdaptiveSchedulerLatencyEMAAlpha)
	s.GeminiAdaptiveSchedulerWeightReliability = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightReliability, s.GeminiAdaptiveSchedulerWeightReliability)
	s.GeminiAdaptiveSchedulerWeightCapacity = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightCapacity, s.GeminiAdaptiveSchedulerWeightCapacity)
	s.GeminiAdaptiveSchedulerWeightLatency = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightLatency, s.GeminiAdaptiveSchedulerWeightLatency)
	s.GeminiAdaptiveSchedulerWeightCost = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightCost, s.GeminiAdaptiveSchedulerWeightCost)
	s.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold, s.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold)
	s.GeminiAdaptiveSchedulerShrinkFactorSoft = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft, s.GeminiAdaptiveSchedulerShrinkFactorSoft)
	s.GeminiAdaptiveSchedulerLearningWindowSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds, s.GeminiAdaptiveSchedulerLearningWindowSeconds)
	s.GeminiAdaptiveSchedulerCooldownSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCooldownSeconds, s.GeminiAdaptiveSchedulerCooldownSeconds)
	s.GeminiAdaptiveSchedulerCooldownMaxSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds, s.GeminiAdaptiveSchedulerCooldownMaxSeconds)
	s.GeminiAdaptiveSchedulerAccountFailureThreshold = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold, s.GeminiAdaptiveSchedulerAccountFailureThreshold)
	s.GeminiAdaptiveSchedulerDiagnosticLogEnabled = parseBoolSetting(values, SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled, s.GeminiAdaptiveSchedulerDiagnosticLogEnabled)
	s.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate, s.GeminiAdaptiveSchedulerDiagnosticLogSampleRate)
	s.GeminiAdaptiveSchedulerExplorationRate = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerExplorationRate, s.GeminiAdaptiveSchedulerExplorationRate)
	s.GeminiAdaptiveSchedulerLearningMinHealthSamples = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerLearningMinHealthSamples, s.GeminiAdaptiveSchedulerLearningMinHealthSamples)
	s.GeminiAdaptiveSchedulerHighErrorMinSamples = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerHighErrorMinSamples, s.GeminiAdaptiveSchedulerHighErrorMinSamples)
	s.GeminiAdaptiveSchedulerHighErrorMaxSamples = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerHighErrorMaxSamples, s.GeminiAdaptiveSchedulerHighErrorMaxSamples)
	s.GeminiAdaptiveSchedulerHighErrorEnterRate = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerHighErrorEnterRate, s.GeminiAdaptiveSchedulerHighErrorEnterRate)
	s.GeminiAdaptiveSchedulerHighErrorExitRate = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerHighErrorExitRate, s.GeminiAdaptiveSchedulerHighErrorExitRate)
	s.GeminiAdaptiveSchedulerCapacityRecoverySamples = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacityRecoverySamples, s.GeminiAdaptiveSchedulerCapacityRecoverySamples)
	s.GeminiAdaptiveSchedulerCapacityGrowthFactor = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacityGrowthFactor, s.GeminiAdaptiveSchedulerCapacityGrowthFactor)
	s.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerQuotaProbeIntervalSeconds, s.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds)
	return NormalizeGeminiAdaptiveSchedulerSettings(s)
}

func geminiAdaptiveSchedulerSettingsToMap(s GeminiAdaptiveSchedulerSettings) map[string]string {
	s = NormalizeGeminiAdaptiveSchedulerSettings(s)
	return map[string]string{
		SettingKeyGeminiAdaptiveSchedulerEnabled:                    strconv.FormatBool(s.GeminiAdaptiveSchedulerEnabled),
		SettingKeyGeminiAdaptiveSchedulerMode:                       s.GeminiAdaptiveSchedulerMode,
		SettingKeyGeminiAdaptiveSchedulerTopK:                       strconv.Itoa(s.GeminiAdaptiveSchedulerTopK),
		SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature:         formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerSoftmaxTemperature),
		SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty:  formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerConsecutiveFailurePenalty),
		SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha:            formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerSuccessEMAAlpha),
		SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha:            formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerLatencyEMAAlpha),
		SettingKeyGeminiAdaptiveSchedulerWeightReliability:          formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightReliability),
		SettingKeyGeminiAdaptiveSchedulerWeightCapacity:             formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightCapacity),
		SettingKeyGeminiAdaptiveSchedulerWeightLatency:              formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightLatency),
		SettingKeyGeminiAdaptiveSchedulerWeightCost:                 formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightCost),
		SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold: formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold),
		SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft:           formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerShrinkFactorSoft),
		SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds:      strconv.Itoa(s.GeminiAdaptiveSchedulerLearningWindowSeconds),
		SettingKeyGeminiAdaptiveSchedulerCooldownSeconds:            strconv.Itoa(s.GeminiAdaptiveSchedulerCooldownSeconds),
		SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds:         strconv.Itoa(s.GeminiAdaptiveSchedulerCooldownMaxSeconds),
		SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold:    strconv.Itoa(s.GeminiAdaptiveSchedulerAccountFailureThreshold),
		SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled:       strconv.FormatBool(s.GeminiAdaptiveSchedulerDiagnosticLogEnabled),
		SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate:    formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerDiagnosticLogSampleRate),
		SettingKeyGeminiAdaptiveSchedulerExplorationRate:            formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerExplorationRate),
		SettingKeyGeminiAdaptiveSchedulerLearningMinHealthSamples:   strconv.Itoa(s.GeminiAdaptiveSchedulerLearningMinHealthSamples),
		SettingKeyGeminiAdaptiveSchedulerHighErrorMinSamples:        strconv.Itoa(s.GeminiAdaptiveSchedulerHighErrorMinSamples),
		SettingKeyGeminiAdaptiveSchedulerHighErrorMaxSamples:        strconv.Itoa(s.GeminiAdaptiveSchedulerHighErrorMaxSamples),
		SettingKeyGeminiAdaptiveSchedulerHighErrorEnterRate:         formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerHighErrorEnterRate),
		SettingKeyGeminiAdaptiveSchedulerHighErrorExitRate:          formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerHighErrorExitRate),
		SettingKeyGeminiAdaptiveSchedulerCapacityRecoverySamples:    strconv.Itoa(s.GeminiAdaptiveSchedulerCapacityRecoverySamples),
		SettingKeyGeminiAdaptiveSchedulerCapacityGrowthFactor:       formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerCapacityGrowthFactor),
		SettingKeyGeminiAdaptiveSchedulerQuotaProbeIntervalSeconds:  strconv.Itoa(s.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds),
	}
}

func (s *SettingService) GetGeminiAdaptiveSchedulerSettings(ctx context.Context) (GeminiAdaptiveSchedulerSettings, error) {
	defaults := DefaultGeminiAdaptiveSchedulerSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	if cached, _ := geminiAdaptiveSchedulerSettingCache.Load().(*cachedGeminiAdaptiveSchedulerSettings); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.settings, nil
	}
	generation := geminiAdaptiveSchedulerSettingGeneration.Load()
	value, err, _ := geminiAdaptiveSchedulerSettingSF.Do("settings", func() (any, error) {
		if cached, _ := geminiAdaptiveSchedulerSettingCache.Load().(*cachedGeminiAdaptiveSchedulerSettings); cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached.settings, nil
		}
		dbCtx, cancel := context.WithTimeout(ctx, geminiAdaptiveSchedulerSettingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, geminiAdaptiveSchedulerSettingKeys)
		if err != nil {
			return defaults, err
		}
		settings := parseGeminiAdaptiveSchedulerSettings(values)
		if geminiAdaptiveSchedulerSettingGeneration.Load() == generation {
			geminiAdaptiveSchedulerSettingCache.Store(&cachedGeminiAdaptiveSchedulerSettings{settings: settings, expiresAt: time.Now().Add(geminiAdaptiveSchedulerSettingCacheTTL).UnixNano()})
		}
		return settings, nil
	})
	if err != nil {
		return defaults, err
	}
	settings, ok := value.(GeminiAdaptiveSchedulerSettings)
	if !ok {
		return defaults, fmt.Errorf("unexpected Gemini adaptive scheduler settings type %T", value)
	}
	return settings, nil
}

func refreshGeminiAdaptiveSchedulerSettingCache(settings GeminiAdaptiveSchedulerSettings) {
	settings = NormalizeGeminiAdaptiveSchedulerSettings(settings)
	geminiAdaptiveSchedulerSettingGeneration.Add(1)
	geminiAdaptiveSchedulerSettingSF.Forget("settings")
	geminiAdaptiveSchedulerSettingCache.Store(&cachedGeminiAdaptiveSchedulerSettings{settings: settings, expiresAt: time.Now().Add(geminiAdaptiveSchedulerSettingCacheTTL).UnixNano()})
}
