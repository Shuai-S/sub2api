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
	openAIAdaptiveSchedulerDiagnosticLogEnabledKey       = openAIAdaptiveSchedulerSettingPrefix + "diagnostic_log_enabled"
	openAIAdaptiveSchedulerDiagnosticLogSampleRateKey    = openAIAdaptiveSchedulerSettingPrefix + "diagnostic_log_sample_rate"
	openAIAdaptiveSchedulerModeKey                       = openAIAdaptiveSchedulerSettingPrefix + "mode"
	openAIAdaptiveSchedulerTopKKey                       = openAIAdaptiveSchedulerSettingPrefix + "top_k"
	openAIAdaptiveSchedulerExplorationRateKey            = openAIAdaptiveSchedulerSettingPrefix + "exploration_rate"
	openAIAdaptiveSchedulerRecoveryExplorationRateKey    = openAIAdaptiveSchedulerSettingPrefix + "recovery_exploration_rate"
	openAIAdaptiveSchedulerRecoveryMaxConcurrencyKey     = openAIAdaptiveSchedulerSettingPrefix + "recovery_max_concurrency"
	openAIAdaptiveSchedulerRecoveryWarmupSuccessesKey    = openAIAdaptiveSchedulerSettingPrefix + "recovery_warmup_successes"
	openAIAdaptiveSchedulerSoftmaxTemperatureKey         = openAIAdaptiveSchedulerSettingPrefix + "softmax_temperature"
	openAIAdaptiveSchedulerCapacityGrowthFactorKey       = openAIAdaptiveSchedulerSettingPrefix + "capacity_growth_factor"
	openAIAdaptiveSchedulerCapacityProbeLoadThresholdKey = openAIAdaptiveSchedulerSettingPrefix + "capacity_probe_load_threshold"
	openAIAdaptiveSchedulerShrinkFactorSoftKey           = openAIAdaptiveSchedulerSettingPrefix + "shrink_factor_soft"
	openAIAdaptiveSchedulerLearningWindowSecondsKey      = openAIAdaptiveSchedulerSettingPrefix + "learning_window_seconds"
	openAIAdaptiveSchedulerSuccessEMAAlphaKey            = openAIAdaptiveSchedulerSettingPrefix + "success_ema_alpha"
	openAIAdaptiveSchedulerTTFTEMAAlphaKey               = openAIAdaptiveSchedulerSettingPrefix + "ttft_ema_alpha"
	openAIAdaptiveSchedulerCooldownBaseSecondsKey        = openAIAdaptiveSchedulerSettingPrefix + "cooldown_base_seconds"
	openAIAdaptiveSchedulerCooldownMaxSecondsKey         = openAIAdaptiveSchedulerSettingPrefix + "cooldown_max_seconds"
	openAIAdaptiveSchedulerWeightSuccessKey              = openAIAdaptiveSchedulerSettingPrefix + "weight_success"
	openAIAdaptiveSchedulerWeightCostKey                 = openAIAdaptiveSchedulerSettingPrefix + "weight_cost"
	openAIAdaptiveSchedulerWeightCapacityKey             = openAIAdaptiveSchedulerSettingPrefix + "weight_capacity"
	openAIAdaptiveSchedulerWeightLatencyKey              = openAIAdaptiveSchedulerSettingPrefix + "weight_latency"
	openAIAdaptiveSchedulerWeightCacheKey                = openAIAdaptiveSchedulerSettingPrefix + "weight_cache"
	openAIAdaptiveSchedulerConsecutiveFailurePenaltyKey  = openAIAdaptiveSchedulerSettingPrefix + "consecutive_failure_penalty"
	openAIAdaptiveSchedulerLearningMinHealthSamplesKey   = openAIAdaptiveSchedulerSettingPrefix + "learning_min_health_samples"
	openAIAdaptiveSchedulerHealthFailureThresholdKey     = openAIAdaptiveSchedulerSettingPrefix + "health_failure_threshold"
	openAIAdaptiveSchedulerHighErrorMinSamplesKey        = openAIAdaptiveSchedulerSettingPrefix + "high_error_min_samples"
	openAIAdaptiveSchedulerHighErrorMaxSamplesKey        = openAIAdaptiveSchedulerSettingPrefix + "high_error_max_samples"
	openAIAdaptiveSchedulerHighErrorEnterRateKey         = openAIAdaptiveSchedulerSettingPrefix + "high_error_enter_rate"
	openAIAdaptiveSchedulerHighErrorExitRateKey          = openAIAdaptiveSchedulerSettingPrefix + "high_error_exit_rate"
	openAIAdaptiveSchedulerCapacityRecoverySamplesKey    = openAIAdaptiveSchedulerSettingPrefix + "capacity_recovery_samples"
	openAIAdaptiveSchedulerQuotaProbeIntervalSecondsKey  = openAIAdaptiveSchedulerSettingPrefix + "quota_probe_interval_seconds"
)

const (
	openAIAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	openAIAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
	openAIAdaptiveSchedulerSettingsCacheKey = "openai_adaptive_scheduler_settings"
)

type OpenAIAdaptiveSchedulerSettings struct {
	OpenAIAdaptiveSchedulerEnabled                    bool    `json:"openai_adaptive_scheduler_enabled"`
	OpenAIAdaptiveSchedulerDiagnosticLogEnabled       bool    `json:"openai_adaptive_scheduler_diagnostic_log_enabled"`
	OpenAIAdaptiveSchedulerDiagnosticLogSampleRate    float64 `json:"openai_adaptive_scheduler_diagnostic_log_sample_rate"`
	OpenAIAdaptiveSchedulerMode                       string  `json:"openai_adaptive_scheduler_mode"`
	OpenAIAdaptiveSchedulerTopK                       int     `json:"openai_adaptive_scheduler_top_k"`
	OpenAIAdaptiveSchedulerExplorationRate            float64 `json:"openai_adaptive_scheduler_exploration_rate"`
	OpenAIAdaptiveSchedulerRecoveryExplorationRate    float64 `json:"openai_adaptive_scheduler_recovery_exploration_rate"`
	OpenAIAdaptiveSchedulerRecoveryMaxConcurrency     int     `json:"openai_adaptive_scheduler_recovery_max_concurrency"`
	OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses    int     `json:"openai_adaptive_scheduler_recovery_warmup_successes"`
	OpenAIAdaptiveSchedulerSoftmaxTemperature         float64 `json:"openai_adaptive_scheduler_softmax_temperature"`
	OpenAIAdaptiveSchedulerCapacityGrowthFactor       float64 `json:"openai_adaptive_scheduler_capacity_growth_factor"`
	OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold float64 `json:"openai_adaptive_scheduler_capacity_probe_load_threshold"`
	OpenAIAdaptiveSchedulerShrinkFactorSoft           float64 `json:"openai_adaptive_scheduler_shrink_factor_soft"`
	OpenAIAdaptiveSchedulerLearningWindowSeconds      int     `json:"openai_adaptive_scheduler_learning_window_seconds"`
	OpenAIAdaptiveSchedulerSuccessEMAAlpha            float64 `json:"openai_adaptive_scheduler_success_ema_alpha"`
	// Retained for API compatibility. OpenAI TTFT scheduling uses rolling-window percentiles.
	OpenAIAdaptiveSchedulerTTFTEMAAlpha              float64 `json:"openai_adaptive_scheduler_ttft_ema_alpha"`
	OpenAIAdaptiveSchedulerCooldownBaseSeconds       int     `json:"openai_adaptive_scheduler_cooldown_base_seconds"`
	OpenAIAdaptiveSchedulerCooldownMaxSeconds        int     `json:"openai_adaptive_scheduler_cooldown_max_seconds"`
	OpenAIAdaptiveSchedulerWeightSuccess             float64 `json:"openai_adaptive_scheduler_weight_success"`
	OpenAIAdaptiveSchedulerWeightCost                float64 `json:"openai_adaptive_scheduler_weight_cost"`
	OpenAIAdaptiveSchedulerWeightCapacity            float64 `json:"openai_adaptive_scheduler_weight_capacity"`
	OpenAIAdaptiveSchedulerWeightLatency             float64 `json:"openai_adaptive_scheduler_weight_latency"`
	OpenAIAdaptiveSchedulerWeightCache               float64 `json:"openai_adaptive_scheduler_weight_cache"`
	OpenAIAdaptiveSchedulerConsecutiveFailurePenalty float64 `json:"openai_adaptive_scheduler_consecutive_failure_penalty"`
	OpenAIAdaptiveSchedulerLearningMinHealthSamples  int     `json:"openai_adaptive_scheduler_learning_min_health_samples"`
	OpenAIAdaptiveSchedulerHealthFailureThreshold    int     `json:"openai_adaptive_scheduler_health_failure_threshold"`
	OpenAIAdaptiveSchedulerHighErrorMinSamples       int     `json:"openai_adaptive_scheduler_high_error_min_samples"`
	OpenAIAdaptiveSchedulerHighErrorMaxSamples       int     `json:"openai_adaptive_scheduler_high_error_max_samples"`
	OpenAIAdaptiveSchedulerHighErrorEnterRate        float64 `json:"openai_adaptive_scheduler_high_error_enter_rate"`
	OpenAIAdaptiveSchedulerHighErrorExitRate         float64 `json:"openai_adaptive_scheduler_high_error_exit_rate"`
	OpenAIAdaptiveSchedulerCapacityRecoverySamples   int     `json:"openai_adaptive_scheduler_capacity_recovery_samples"`
	OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds int     `json:"openai_adaptive_scheduler_quota_probe_interval_seconds"`
}

type cachedOpenAIAdaptiveSchedulerSetting struct {
	settings  OpenAIAdaptiveSchedulerSettings
	complete  bool
	expiresAt int64
}

var openAIAdaptiveSchedulerSettingCache atomic.Value // *cachedOpenAIAdaptiveSchedulerSetting
var openAIAdaptiveSchedulerSettingSF singleflight.Group
var openAIAdaptiveSchedulerSettingGeneration atomic.Uint64

func DefaultOpenAIAdaptiveSchedulerSettings() OpenAIAdaptiveSchedulerSettings {
	return OpenAIAdaptiveSchedulerSettings{
		OpenAIAdaptiveSchedulerEnabled:                    false,
		OpenAIAdaptiveSchedulerDiagnosticLogEnabled:       false,
		OpenAIAdaptiveSchedulerDiagnosticLogSampleRate:    0.05,
		OpenAIAdaptiveSchedulerMode:                       openAIAdaptiveSchedulerModeShadow,
		OpenAIAdaptiveSchedulerTopK:                       8,
		OpenAIAdaptiveSchedulerExplorationRate:            0.02,
		OpenAIAdaptiveSchedulerRecoveryExplorationRate:    0.01,
		OpenAIAdaptiveSchedulerRecoveryMaxConcurrency:     2,
		OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses:    3,
		OpenAIAdaptiveSchedulerSoftmaxTemperature:         0.35,
		OpenAIAdaptiveSchedulerCapacityGrowthFactor:       1.25,
		OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold: 0.80,
		OpenAIAdaptiveSchedulerShrinkFactorSoft:           0.90,
		OpenAIAdaptiveSchedulerLearningWindowSeconds:      1200,
		OpenAIAdaptiveSchedulerSuccessEMAAlpha:            0.05,
		OpenAIAdaptiveSchedulerTTFTEMAAlpha:               0.05,
		OpenAIAdaptiveSchedulerCooldownBaseSeconds:        60,
		OpenAIAdaptiveSchedulerCooldownMaxSeconds:         600,
		OpenAIAdaptiveSchedulerWeightSuccess:              0.50,
		OpenAIAdaptiveSchedulerWeightCost:                 0.15,
		OpenAIAdaptiveSchedulerWeightCapacity:             0.20,
		OpenAIAdaptiveSchedulerWeightLatency:              0.15,
		OpenAIAdaptiveSchedulerWeightCache:                0,
		OpenAIAdaptiveSchedulerConsecutiveFailurePenalty:  0.25,
		OpenAIAdaptiveSchedulerLearningMinHealthSamples:   30,
		OpenAIAdaptiveSchedulerHealthFailureThreshold:     3,
		OpenAIAdaptiveSchedulerHighErrorMinSamples:        10,
		OpenAIAdaptiveSchedulerHighErrorMaxSamples:        100,
		OpenAIAdaptiveSchedulerHighErrorEnterRate:         0.25,
		OpenAIAdaptiveSchedulerHighErrorExitRate:          0.15,
		OpenAIAdaptiveSchedulerCapacityRecoverySamples:    8,
		OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds:  300,
	}
}

func NormalizeOpenAIAdaptiveSchedulerSettings(settings OpenAIAdaptiveSchedulerSettings) OpenAIAdaptiveSchedulerSettings {
	defaults := DefaultOpenAIAdaptiveSchedulerSettings()
	settings.OpenAIAdaptiveSchedulerMode = normalizeOpenAIAdaptiveSchedulerMode(settings.OpenAIAdaptiveSchedulerMode)
	if settings.OpenAIAdaptiveSchedulerMode == "" {
		settings.OpenAIAdaptiveSchedulerMode = defaults.OpenAIAdaptiveSchedulerMode
	}
	settings.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = clampFloat(settings.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate, 0, 1, defaults.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
	settings.OpenAIAdaptiveSchedulerTopK = clampInt(settings.OpenAIAdaptiveSchedulerTopK, 1, 100, defaults.OpenAIAdaptiveSchedulerTopK)
	settings.OpenAIAdaptiveSchedulerExplorationRate = clampFloat(settings.OpenAIAdaptiveSchedulerExplorationRate, 0, 1, defaults.OpenAIAdaptiveSchedulerExplorationRate)
	settings.OpenAIAdaptiveSchedulerRecoveryExplorationRate = clampFloat(settings.OpenAIAdaptiveSchedulerRecoveryExplorationRate, 0, 1, defaults.OpenAIAdaptiveSchedulerRecoveryExplorationRate)
	settings.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency = clampIntMin(settings.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency, 1, defaults.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency)
	settings.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses = clampIntMin(settings.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses, 1, defaults.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses)
	settings.OpenAIAdaptiveSchedulerSoftmaxTemperature = clampFloat(settings.OpenAIAdaptiveSchedulerSoftmaxTemperature, 0.01, 10, defaults.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor = clampFloat(settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor, 1, 10, defaults.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold = clampFloat(settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold, 0, 1, defaults.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.OpenAIAdaptiveSchedulerShrinkFactorSoft = clampFloat(settings.OpenAIAdaptiveSchedulerShrinkFactorSoft, 0.01, 1, defaults.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	settings.OpenAIAdaptiveSchedulerLearningWindowSeconds = clampIntMin(settings.OpenAIAdaptiveSchedulerLearningWindowSeconds, 0, defaults.OpenAIAdaptiveSchedulerLearningWindowSeconds)
	settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha = clampFloat(settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha, 0, 1, defaults.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
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
	settings.OpenAIAdaptiveSchedulerWeightCache = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerWeightCache)
	settings.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty = nonNegativeFinite(settings.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.OpenAIAdaptiveSchedulerLearningMinHealthSamples = clampIntMin(settings.OpenAIAdaptiveSchedulerLearningMinHealthSamples, 1, defaults.OpenAIAdaptiveSchedulerLearningMinHealthSamples)
	settings.OpenAIAdaptiveSchedulerHealthFailureThreshold = clampIntMin(settings.OpenAIAdaptiveSchedulerHealthFailureThreshold, 1, defaults.OpenAIAdaptiveSchedulerHealthFailureThreshold)
	settings.OpenAIAdaptiveSchedulerHighErrorMinSamples = clampIntMin(settings.OpenAIAdaptiveSchedulerHighErrorMinSamples, 1, defaults.OpenAIAdaptiveSchedulerHighErrorMinSamples)
	settings.OpenAIAdaptiveSchedulerHighErrorMaxSamples = clampIntMin(settings.OpenAIAdaptiveSchedulerHighErrorMaxSamples, settings.OpenAIAdaptiveSchedulerHighErrorMinSamples, defaults.OpenAIAdaptiveSchedulerHighErrorMaxSamples)
	settings.OpenAIAdaptiveSchedulerHighErrorEnterRate = clampFloat(settings.OpenAIAdaptiveSchedulerHighErrorEnterRate, 0, 1, defaults.OpenAIAdaptiveSchedulerHighErrorEnterRate)
	settings.OpenAIAdaptiveSchedulerHighErrorExitRate = clampFloat(settings.OpenAIAdaptiveSchedulerHighErrorExitRate, 0, settings.OpenAIAdaptiveSchedulerHighErrorEnterRate, defaults.OpenAIAdaptiveSchedulerHighErrorExitRate)
	settings.OpenAIAdaptiveSchedulerCapacityRecoverySamples = clampIntMin(settings.OpenAIAdaptiveSchedulerCapacityRecoverySamples, 1, defaults.OpenAIAdaptiveSchedulerCapacityRecoverySamples)
	settings.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds = clampIntMin(settings.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds, 1, defaults.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds)
	weightSum := settings.OpenAIAdaptiveSchedulerWeightSuccess +
		settings.OpenAIAdaptiveSchedulerWeightCost +
		settings.OpenAIAdaptiveSchedulerWeightCapacity +
		settings.OpenAIAdaptiveSchedulerWeightLatency +
		settings.OpenAIAdaptiveSchedulerWeightCache
	if weightSum <= 0 {
		settings.OpenAIAdaptiveSchedulerWeightSuccess = defaults.OpenAIAdaptiveSchedulerWeightSuccess
		settings.OpenAIAdaptiveSchedulerWeightCost = defaults.OpenAIAdaptiveSchedulerWeightCost
		settings.OpenAIAdaptiveSchedulerWeightCapacity = defaults.OpenAIAdaptiveSchedulerWeightCapacity
		settings.OpenAIAdaptiveSchedulerWeightLatency = defaults.OpenAIAdaptiveSchedulerWeightLatency
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
		openAIAdaptiveSchedulerDiagnosticLogEnabledKey:       strconv.FormatBool(settings.OpenAIAdaptiveSchedulerDiagnosticLogEnabled),
		openAIAdaptiveSchedulerDiagnosticLogSampleRateKey:    formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate),
		openAIAdaptiveSchedulerModeKey:                       settings.OpenAIAdaptiveSchedulerMode,
		openAIAdaptiveSchedulerTopKKey:                       strconv.Itoa(settings.OpenAIAdaptiveSchedulerTopK),
		openAIAdaptiveSchedulerExplorationRateKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerExplorationRate),
		openAIAdaptiveSchedulerRecoveryExplorationRateKey:    formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerRecoveryExplorationRate),
		openAIAdaptiveSchedulerRecoveryMaxConcurrencyKey:     strconv.Itoa(settings.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency),
		openAIAdaptiveSchedulerRecoveryWarmupSuccessesKey:    strconv.Itoa(settings.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses),
		openAIAdaptiveSchedulerSoftmaxTemperatureKey:         formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerSoftmaxTemperature),
		openAIAdaptiveSchedulerCapacityGrowthFactorKey:       formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor),
		openAIAdaptiveSchedulerCapacityProbeLoadThresholdKey: formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold),
		openAIAdaptiveSchedulerShrinkFactorSoftKey:           formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerShrinkFactorSoft),
		openAIAdaptiveSchedulerLearningWindowSecondsKey:      strconv.Itoa(settings.OpenAIAdaptiveSchedulerLearningWindowSeconds),
		openAIAdaptiveSchedulerSuccessEMAAlphaKey:            formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha),
		openAIAdaptiveSchedulerTTFTEMAAlphaKey:               formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha),
		openAIAdaptiveSchedulerCooldownBaseSecondsKey:        strconv.Itoa(settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds),
		openAIAdaptiveSchedulerCooldownMaxSecondsKey:         strconv.Itoa(settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds),
		openAIAdaptiveSchedulerWeightSuccessKey:              formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightSuccess),
		openAIAdaptiveSchedulerWeightCostKey:                 formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightCost),
		openAIAdaptiveSchedulerWeightCapacityKey:             formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightCapacity),
		openAIAdaptiveSchedulerWeightLatencyKey:              formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightLatency),
		openAIAdaptiveSchedulerWeightCacheKey:                formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerWeightCache),
		openAIAdaptiveSchedulerConsecutiveFailurePenaltyKey:  formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty),
		openAIAdaptiveSchedulerLearningMinHealthSamplesKey:   strconv.Itoa(settings.OpenAIAdaptiveSchedulerLearningMinHealthSamples),
		openAIAdaptiveSchedulerHealthFailureThresholdKey:     strconv.Itoa(settings.OpenAIAdaptiveSchedulerHealthFailureThreshold),
		openAIAdaptiveSchedulerHighErrorMinSamplesKey:        strconv.Itoa(settings.OpenAIAdaptiveSchedulerHighErrorMinSamples),
		openAIAdaptiveSchedulerHighErrorMaxSamplesKey:        strconv.Itoa(settings.OpenAIAdaptiveSchedulerHighErrorMaxSamples),
		openAIAdaptiveSchedulerHighErrorEnterRateKey:         formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerHighErrorEnterRate),
		openAIAdaptiveSchedulerHighErrorExitRateKey:          formatOpenAIAdaptiveFloat(settings.OpenAIAdaptiveSchedulerHighErrorExitRate),
		openAIAdaptiveSchedulerCapacityRecoverySamplesKey:    strconv.Itoa(settings.OpenAIAdaptiveSchedulerCapacityRecoverySamples),
		openAIAdaptiveSchedulerQuotaProbeIntervalSecondsKey:  strconv.Itoa(settings.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds),
	}
}

func parseOpenAIAdaptiveSchedulerSettings(settings map[string]string) OpenAIAdaptiveSchedulerSettings {
	result := DefaultOpenAIAdaptiveSchedulerSettings()
	result.OpenAIAdaptiveSchedulerEnabled = parseBoolSetting(settings, openAIAdaptiveSchedulerEnabledKey, result.OpenAIAdaptiveSchedulerEnabled)
	result.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = parseBoolSetting(settings, openAIAdaptiveSchedulerDiagnosticLogEnabledKey, result.OpenAIAdaptiveSchedulerDiagnosticLogEnabled)
	result.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = parseFloatSetting(settings, openAIAdaptiveSchedulerDiagnosticLogSampleRateKey, result.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
	result.OpenAIAdaptiveSchedulerMode = firstNonEmpty(settings[openAIAdaptiveSchedulerModeKey], result.OpenAIAdaptiveSchedulerMode)
	result.OpenAIAdaptiveSchedulerTopK = parseIntSetting(settings, openAIAdaptiveSchedulerTopKKey, result.OpenAIAdaptiveSchedulerTopK)
	result.OpenAIAdaptiveSchedulerExplorationRate = parseFloatSetting(settings, openAIAdaptiveSchedulerExplorationRateKey, result.OpenAIAdaptiveSchedulerExplorationRate)
	result.OpenAIAdaptiveSchedulerRecoveryExplorationRate = parseFloatSetting(settings, openAIAdaptiveSchedulerRecoveryExplorationRateKey, result.OpenAIAdaptiveSchedulerRecoveryExplorationRate)
	result.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency = parseIntSetting(settings, openAIAdaptiveSchedulerRecoveryMaxConcurrencyKey, result.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency)
	result.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses = parseIntSetting(settings, openAIAdaptiveSchedulerRecoveryWarmupSuccessesKey, result.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses)
	result.OpenAIAdaptiveSchedulerSoftmaxTemperature = parseFloatSetting(settings, openAIAdaptiveSchedulerSoftmaxTemperatureKey, result.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	result.OpenAIAdaptiveSchedulerCapacityGrowthFactor = parseFloatSetting(settings, openAIAdaptiveSchedulerCapacityGrowthFactorKey, result.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	result.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold = parseFloatSetting(settings, openAIAdaptiveSchedulerCapacityProbeLoadThresholdKey, result.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	result.OpenAIAdaptiveSchedulerShrinkFactorSoft = parseFloatSetting(settings, openAIAdaptiveSchedulerShrinkFactorSoftKey, result.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	result.OpenAIAdaptiveSchedulerLearningWindowSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerLearningWindowSecondsKey, result.OpenAIAdaptiveSchedulerLearningWindowSeconds)
	result.OpenAIAdaptiveSchedulerSuccessEMAAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerSuccessEMAAlphaKey, result.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	result.OpenAIAdaptiveSchedulerTTFTEMAAlpha = parseFloatSetting(settings, openAIAdaptiveSchedulerTTFTEMAAlphaKey, result.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	result.OpenAIAdaptiveSchedulerCooldownBaseSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerCooldownBaseSecondsKey, result.OpenAIAdaptiveSchedulerCooldownBaseSeconds)
	result.OpenAIAdaptiveSchedulerCooldownMaxSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerCooldownMaxSecondsKey, result.OpenAIAdaptiveSchedulerCooldownMaxSeconds)
	result.OpenAIAdaptiveSchedulerWeightSuccess = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightSuccessKey, result.OpenAIAdaptiveSchedulerWeightSuccess)
	result.OpenAIAdaptiveSchedulerWeightCost = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightCostKey, result.OpenAIAdaptiveSchedulerWeightCost)
	result.OpenAIAdaptiveSchedulerWeightCapacity = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightCapacityKey, result.OpenAIAdaptiveSchedulerWeightCapacity)
	result.OpenAIAdaptiveSchedulerWeightLatency = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightLatencyKey, result.OpenAIAdaptiveSchedulerWeightLatency)
	result.OpenAIAdaptiveSchedulerWeightCache = parseFloatSetting(settings, openAIAdaptiveSchedulerWeightCacheKey, result.OpenAIAdaptiveSchedulerWeightCache)
	result.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty = parseFloatSetting(settings, openAIAdaptiveSchedulerConsecutiveFailurePenaltyKey, result.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty)
	result.OpenAIAdaptiveSchedulerLearningMinHealthSamples = parseIntSetting(settings, openAIAdaptiveSchedulerLearningMinHealthSamplesKey, result.OpenAIAdaptiveSchedulerLearningMinHealthSamples)
	result.OpenAIAdaptiveSchedulerHealthFailureThreshold = parseIntSetting(settings, openAIAdaptiveSchedulerHealthFailureThresholdKey, result.OpenAIAdaptiveSchedulerHealthFailureThreshold)
	result.OpenAIAdaptiveSchedulerHighErrorMinSamples = parseIntSetting(settings, openAIAdaptiveSchedulerHighErrorMinSamplesKey, result.OpenAIAdaptiveSchedulerHighErrorMinSamples)
	result.OpenAIAdaptiveSchedulerHighErrorMaxSamples = parseIntSetting(settings, openAIAdaptiveSchedulerHighErrorMaxSamplesKey, result.OpenAIAdaptiveSchedulerHighErrorMaxSamples)
	result.OpenAIAdaptiveSchedulerHighErrorEnterRate = parseFloatSetting(settings, openAIAdaptiveSchedulerHighErrorEnterRateKey, result.OpenAIAdaptiveSchedulerHighErrorEnterRate)
	result.OpenAIAdaptiveSchedulerHighErrorExitRate = parseFloatSetting(settings, openAIAdaptiveSchedulerHighErrorExitRateKey, result.OpenAIAdaptiveSchedulerHighErrorExitRate)
	result.OpenAIAdaptiveSchedulerCapacityRecoverySamples = parseIntSetting(settings, openAIAdaptiveSchedulerCapacityRecoverySamplesKey, result.OpenAIAdaptiveSchedulerCapacityRecoverySamples)
	result.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds = parseIntSetting(settings, openAIAdaptiveSchedulerQuotaProbeIntervalSecondsKey, result.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds)
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
		generation := openAIAdaptiveSchedulerSettingGeneration.Load()
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
		if openAIAdaptiveSchedulerSettingGeneration.Load() == generation {
			openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{
				settings:  settings,
				complete:  false,
				expiresAt: time.Now().Add(openAIAdaptiveSchedulerSettingCacheTTL).UnixNano(),
			})
		}
		return settings.OpenAIAdaptiveSchedulerEnabled, nil
	})

	if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.settings.OpenAIAdaptiveSchedulerEnabled
		}
	}
	enabled, _ := result.(bool)
	return enabled
}

func (s *OpenAIGatewayService) openAIAdaptiveSchedulerSettings(ctx context.Context) OpenAIAdaptiveSchedulerSettings {
	if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt && cached.complete && cached.settings.OpenAIAdaptiveSchedulerEnabled {
			return cached.settings
		}
	}

	result, _, _ := openAIAdaptiveSchedulerSettingSF.Do(openAIAdaptiveSchedulerSettingsCacheKey, func() (any, error) {
		generation := openAIAdaptiveSchedulerSettingGeneration.Load()
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

		if openAIAdaptiveSchedulerSettingGeneration.Load() == generation {
			openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{
				settings:  settings,
				complete:  true,
				expiresAt: time.Now().Add(openAIAdaptiveSchedulerSettingCacheTTL).UnixNano(),
			})
		}
		return settings, nil
	})

	if cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt && cached.complete {
			return NormalizeOpenAIAdaptiveSchedulerSettings(cached.settings)
		}
	}
	settings, _ := result.(OpenAIAdaptiveSchedulerSettings)
	return NormalizeOpenAIAdaptiveSchedulerSettings(settings)
}

func refreshOpenAIAdaptiveSchedulerSettingCache(settings OpenAIAdaptiveSchedulerSettings) {
	openAIAdaptiveSchedulerSettingGeneration.Add(1)
	openAIAdaptiveSchedulerSettingCache.Store(&cachedOpenAIAdaptiveSchedulerSetting{
		settings:  NormalizeOpenAIAdaptiveSchedulerSettings(settings),
		complete:  true,
		expiresAt: time.Now().Add(openAIAdaptiveSchedulerSettingCacheTTL).UnixNano(),
	})
	openAIAdaptiveSchedulerSettingSF.Forget(openAIAdaptiveSchedulerEnabledKey)
	openAIAdaptiveSchedulerSettingSF.Forget(openAIAdaptiveSchedulerSettingsCacheKey)
}

func resetOpenAIAdaptiveSchedulerSettingCacheForTest() {
	openAIAdaptiveSchedulerSettingCache = atomic.Value{}
	openAIAdaptiveSchedulerSettingSF = singleflight.Group{}
	openAIAdaptiveSchedulerSettingGeneration = atomic.Uint64{}
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

func nonNegativeFinite(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}
