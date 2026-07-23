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

	SettingKeyAnthropicAdaptiveSchedulerEnabled                     = anthropicAdaptiveSchedulerSettingPrefix + "enabled"
	SettingKeyAnthropicAdaptiveSchedulerMode                        = anthropicAdaptiveSchedulerSettingPrefix + "mode"
	SettingKeyAnthropicAdaptiveSchedulerTopK                        = anthropicAdaptiveSchedulerSettingPrefix + "top_k"
	SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature          = anthropicAdaptiveSchedulerSettingPrefix + "softmax_temperature"
	SettingKeyAnthropicAdaptiveSchedulerWeightReliability           = anthropicAdaptiveSchedulerSettingPrefix + "weight_reliability"
	SettingKeyAnthropicAdaptiveSchedulerWeightCapacity              = anthropicAdaptiveSchedulerSettingPrefix + "weight_capacity"
	SettingKeyAnthropicAdaptiveSchedulerWeightLatency               = anthropicAdaptiveSchedulerSettingPrefix + "weight_latency"
	SettingKeyAnthropicAdaptiveSchedulerWeightExploration           = anthropicAdaptiveSchedulerSettingPrefix + "weight_exploration"
	SettingKeyAnthropicAdaptiveSchedulerInitialReliability          = anthropicAdaptiveSchedulerSettingPrefix + "initial_reliability"
	SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty   = anthropicAdaptiveSchedulerSettingPrefix + "consecutive_failure_penalty"
	SettingKeyAnthropicAdaptiveSchedulerNeutralLatencyScore         = anthropicAdaptiveSchedulerSettingPrefix + "neutral_latency_score"
	SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha             = anthropicAdaptiveSchedulerSettingPrefix + "success_ema_alpha"
	SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha             = anthropicAdaptiveSchedulerSettingPrefix + "latency_ema_alpha"
	SettingKeyAnthropicAdaptiveSchedulerCapacitySuccessThreshold    = anthropicAdaptiveSchedulerSettingPrefix + "capacity_success_threshold"
	SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold  = anthropicAdaptiveSchedulerSettingPrefix + "capacity_probe_load_threshold"
	SettingKeyAnthropicAdaptiveSchedulerCapacityFailureThreshold    = anthropicAdaptiveSchedulerSettingPrefix + "capacity_failure_threshold"
	SettingKeyAnthropicAdaptiveSchedulerMinRecentSamplesForShrink   = anthropicAdaptiveSchedulerSettingPrefix + "min_recent_samples_for_shrink"
	SettingKeyAnthropicAdaptiveSchedulerShrinkErrorThreshold        = anthropicAdaptiveSchedulerSettingPrefix + "shrink_error_threshold"
	SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds       = anthropicAdaptiveSchedulerSettingPrefix + "learning_window_seconds"
	SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds             = anthropicAdaptiveSchedulerSettingPrefix + "cooldown_seconds"
	SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft            = anthropicAdaptiveSchedulerSettingPrefix + "shrink_factor_soft"
	SettingKeyAnthropicAdaptiveSchedulerShrinkFactorHard            = anthropicAdaptiveSchedulerSettingPrefix + "shrink_factor_hard"
	SettingKeyAnthropicAdaptiveSchedulerCapacityIncreaseStep        = anthropicAdaptiveSchedulerSettingPrefix + "capacity_increase_step"
	SettingKeyAnthropicAdaptiveSchedulerMinCapacity                 = anthropicAdaptiveSchedulerSettingPrefix + "min_capacity"
	SettingKeyAnthropicAdaptiveSchedulerHardShrinkFailureMultiplier = anthropicAdaptiveSchedulerSettingPrefix + "hard_shrink_failure_multiplier"

	anthropicAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	anthropicAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
)

type AnthropicAdaptiveSchedulerSettings struct {
	AnthropicAdaptiveSchedulerEnabled                     bool    `json:"anthropic_adaptive_scheduler_enabled"`
	AnthropicAdaptiveSchedulerMode                        string  `json:"anthropic_adaptive_scheduler_mode"`
	AnthropicAdaptiveSchedulerTopK                        int     `json:"anthropic_adaptive_scheduler_top_k"`
	AnthropicAdaptiveSchedulerSoftmaxTemperature          float64 `json:"anthropic_adaptive_scheduler_softmax_temperature"`
	AnthropicAdaptiveSchedulerWeightReliability           float64 `json:"anthropic_adaptive_scheduler_weight_reliability"`
	AnthropicAdaptiveSchedulerWeightCapacity              float64 `json:"anthropic_adaptive_scheduler_weight_capacity"`
	AnthropicAdaptiveSchedulerWeightLatency               float64 `json:"anthropic_adaptive_scheduler_weight_latency"`
	AnthropicAdaptiveSchedulerWeightExploration           float64 `json:"anthropic_adaptive_scheduler_weight_exploration"`
	AnthropicAdaptiveSchedulerInitialReliability          float64 `json:"anthropic_adaptive_scheduler_initial_reliability"`
	AnthropicAdaptiveSchedulerConsecutiveFailurePenalty   float64 `json:"anthropic_adaptive_scheduler_consecutive_failure_penalty"`
	AnthropicAdaptiveSchedulerNeutralLatencyScore         float64 `json:"anthropic_adaptive_scheduler_neutral_latency_score"`
	AnthropicAdaptiveSchedulerSuccessEMAAlpha             float64 `json:"anthropic_adaptive_scheduler_success_ema_alpha"`
	AnthropicAdaptiveSchedulerLatencyEMAAlpha             float64 `json:"anthropic_adaptive_scheduler_latency_ema_alpha"`
	AnthropicAdaptiveSchedulerCapacitySuccessThreshold    float64 `json:"anthropic_adaptive_scheduler_capacity_success_threshold"`
	AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold  float64 `json:"anthropic_adaptive_scheduler_capacity_probe_load_threshold"`
	AnthropicAdaptiveSchedulerCapacityFailureThreshold    int     `json:"anthropic_adaptive_scheduler_capacity_failure_threshold"`
	AnthropicAdaptiveSchedulerMinRecentSamplesForShrink   int     `json:"anthropic_adaptive_scheduler_min_recent_samples_for_shrink"`
	AnthropicAdaptiveSchedulerShrinkErrorThreshold        float64 `json:"anthropic_adaptive_scheduler_shrink_error_threshold"`
	AnthropicAdaptiveSchedulerLearningWindowSeconds       int     `json:"anthropic_adaptive_scheduler_learning_window_seconds"`
	AnthropicAdaptiveSchedulerCooldownSeconds             int     `json:"anthropic_adaptive_scheduler_cooldown_seconds"`
	AnthropicAdaptiveSchedulerShrinkFactorSoft            float64 `json:"anthropic_adaptive_scheduler_shrink_factor_soft"`
	AnthropicAdaptiveSchedulerShrinkFactorHard            float64 `json:"anthropic_adaptive_scheduler_shrink_factor_hard"`
	AnthropicAdaptiveSchedulerCapacityIncreaseStep        int     `json:"anthropic_adaptive_scheduler_capacity_increase_step"`
	AnthropicAdaptiveSchedulerMinCapacity                 int     `json:"anthropic_adaptive_scheduler_min_capacity"`
	AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier int     `json:"anthropic_adaptive_scheduler_hard_shrink_failure_multiplier"`
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
		AnthropicAdaptiveSchedulerEnabled:                     false,
		AnthropicAdaptiveSchedulerMode:                        AnthropicAdaptiveSchedulerModeShadow,
		AnthropicAdaptiveSchedulerTopK:                        8,
		AnthropicAdaptiveSchedulerSoftmaxTemperature:          0.35,
		AnthropicAdaptiveSchedulerWeightReliability:           0.50,
		AnthropicAdaptiveSchedulerWeightCapacity:              0.30,
		AnthropicAdaptiveSchedulerWeightLatency:               0.15,
		AnthropicAdaptiveSchedulerWeightExploration:           0.05,
		AnthropicAdaptiveSchedulerInitialReliability:          0.50,
		AnthropicAdaptiveSchedulerConsecutiveFailurePenalty:   0.25,
		AnthropicAdaptiveSchedulerNeutralLatencyScore:         0.50,
		AnthropicAdaptiveSchedulerSuccessEMAAlpha:             0.05,
		AnthropicAdaptiveSchedulerLatencyEMAAlpha:             0.05,
		AnthropicAdaptiveSchedulerCapacitySuccessThreshold:    0.97,
		AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold:  0.80,
		AnthropicAdaptiveSchedulerCapacityFailureThreshold:    3,
		AnthropicAdaptiveSchedulerMinRecentSamplesForShrink:   30,
		AnthropicAdaptiveSchedulerShrinkErrorThreshold:        0.25,
		AnthropicAdaptiveSchedulerLearningWindowSeconds:       1200,
		AnthropicAdaptiveSchedulerCooldownSeconds:             60,
		AnthropicAdaptiveSchedulerShrinkFactorSoft:            0.85,
		AnthropicAdaptiveSchedulerShrinkFactorHard:            0.60,
		AnthropicAdaptiveSchedulerCapacityIncreaseStep:        1,
		AnthropicAdaptiveSchedulerMinCapacity:                 1,
		AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier: 2,
	}
}

func NormalizeAnthropicAdaptiveSchedulerSettings(settings AnthropicAdaptiveSchedulerSettings) AnthropicAdaptiveSchedulerSettings {
	defaults := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
	settings.AnthropicAdaptiveSchedulerTopK = clampInt(settings.AnthropicAdaptiveSchedulerTopK, 1, 100, defaults.AnthropicAdaptiveSchedulerTopK)
	settings.AnthropicAdaptiveSchedulerSoftmaxTemperature = clampFloat(settings.AnthropicAdaptiveSchedulerSoftmaxTemperature, 0.01, 10, defaults.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	settings.AnthropicAdaptiveSchedulerInitialReliability = clampFloat(settings.AnthropicAdaptiveSchedulerInitialReliability, 0, 1, defaults.AnthropicAdaptiveSchedulerInitialReliability)
	settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.AnthropicAdaptiveSchedulerNeutralLatencyScore = clampFloat(settings.AnthropicAdaptiveSchedulerNeutralLatencyScore, 0, 1, defaults.AnthropicAdaptiveSchedulerNeutralLatencyScore)
	settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha = clampFloat(settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha, 0, 1, defaults.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
	settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha = clampFloat(settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha, 0, 1, defaults.AnthropicAdaptiveSchedulerLatencyEMAAlpha)
	settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold = clampFloat(settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold, 0, 1, defaults.AnthropicAdaptiveSchedulerCapacitySuccessThreshold)
	settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = clampFloat(settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold, 0, 1, defaults.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold = clampIntMin(settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold, 1, defaults.AnthropicAdaptiveSchedulerCapacityFailureThreshold)
	settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink = clampIntMin(settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink, 1, defaults.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink)
	settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold = clampFloat(settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold, 0, 1, defaults.AnthropicAdaptiveSchedulerShrinkErrorThreshold)
	settings.AnthropicAdaptiveSchedulerLearningWindowSeconds = clampIntMin(settings.AnthropicAdaptiveSchedulerLearningWindowSeconds, 1, defaults.AnthropicAdaptiveSchedulerLearningWindowSeconds)
	settings.AnthropicAdaptiveSchedulerCooldownSeconds = clampIntMin(settings.AnthropicAdaptiveSchedulerCooldownSeconds, 0, defaults.AnthropicAdaptiveSchedulerCooldownSeconds)
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = clampFloat(settings.AnthropicAdaptiveSchedulerShrinkFactorSoft, 0.01, 1, defaults.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	settings.AnthropicAdaptiveSchedulerShrinkFactorHard = clampFloat(settings.AnthropicAdaptiveSchedulerShrinkFactorHard, 0.01, 1, defaults.AnthropicAdaptiveSchedulerShrinkFactorHard)
	if settings.AnthropicAdaptiveSchedulerShrinkFactorHard > settings.AnthropicAdaptiveSchedulerShrinkFactorSoft {
		settings.AnthropicAdaptiveSchedulerShrinkFactorHard = settings.AnthropicAdaptiveSchedulerShrinkFactorSoft
	}
	settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep = clampIntMin(settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep, 1, defaults.AnthropicAdaptiveSchedulerCapacityIncreaseStep)
	settings.AnthropicAdaptiveSchedulerMinCapacity = clampIntMin(settings.AnthropicAdaptiveSchedulerMinCapacity, 1, defaults.AnthropicAdaptiveSchedulerMinCapacity)
	settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier = clampInt(settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier, 1, 100, defaults.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier)
	settings.AnthropicAdaptiveSchedulerWeightReliability = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightReliability)
	settings.AnthropicAdaptiveSchedulerWeightCapacity = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightCapacity)
	settings.AnthropicAdaptiveSchedulerWeightLatency = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightLatency)
	settings.AnthropicAdaptiveSchedulerWeightExploration = nonNegativeFinite(settings.AnthropicAdaptiveSchedulerWeightExploration)
	weightSum := settings.AnthropicAdaptiveSchedulerWeightReliability +
		settings.AnthropicAdaptiveSchedulerWeightCapacity +
		settings.AnthropicAdaptiveSchedulerWeightLatency +
		settings.AnthropicAdaptiveSchedulerWeightExploration
	if weightSum <= 0 {
		settings.AnthropicAdaptiveSchedulerWeightReliability = defaults.AnthropicAdaptiveSchedulerWeightReliability
		settings.AnthropicAdaptiveSchedulerWeightCapacity = defaults.AnthropicAdaptiveSchedulerWeightCapacity
		settings.AnthropicAdaptiveSchedulerWeightLatency = defaults.AnthropicAdaptiveSchedulerWeightLatency
		settings.AnthropicAdaptiveSchedulerWeightExploration = defaults.AnthropicAdaptiveSchedulerWeightExploration
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
	settings.AnthropicAdaptiveSchedulerMode = firstNonEmpty(values[SettingKeyAnthropicAdaptiveSchedulerMode], settings.AnthropicAdaptiveSchedulerMode)
	settings.AnthropicAdaptiveSchedulerTopK = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerTopK, settings.AnthropicAdaptiveSchedulerTopK)
	settings.AnthropicAdaptiveSchedulerSoftmaxTemperature = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature, settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
	settings.AnthropicAdaptiveSchedulerWeightReliability = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightReliability, settings.AnthropicAdaptiveSchedulerWeightReliability)
	settings.AnthropicAdaptiveSchedulerWeightCapacity = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightCapacity, settings.AnthropicAdaptiveSchedulerWeightCapacity)
	settings.AnthropicAdaptiveSchedulerWeightLatency = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightLatency, settings.AnthropicAdaptiveSchedulerWeightLatency)
	settings.AnthropicAdaptiveSchedulerWeightExploration = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerWeightExploration, settings.AnthropicAdaptiveSchedulerWeightExploration)
	settings.AnthropicAdaptiveSchedulerInitialReliability = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerInitialReliability, settings.AnthropicAdaptiveSchedulerInitialReliability)
	settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty, settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.AnthropicAdaptiveSchedulerNeutralLatencyScore = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerNeutralLatencyScore, settings.AnthropicAdaptiveSchedulerNeutralLatencyScore)
	settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha, settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
	settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha, settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha)
	settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacitySuccessThreshold, settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold)
	settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold, settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacityFailureThreshold, settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold)
	settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerMinRecentSamplesForShrink, settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink)
	settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerShrinkErrorThreshold, settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold)
	settings.AnthropicAdaptiveSchedulerLearningWindowSeconds = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds, settings.AnthropicAdaptiveSchedulerLearningWindowSeconds)
	settings.AnthropicAdaptiveSchedulerCooldownSeconds = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds, settings.AnthropicAdaptiveSchedulerCooldownSeconds)
	settings.AnthropicAdaptiveSchedulerShrinkFactorSoft = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft, settings.AnthropicAdaptiveSchedulerShrinkFactorSoft)
	settings.AnthropicAdaptiveSchedulerShrinkFactorHard = parseFloatSetting(values, SettingKeyAnthropicAdaptiveSchedulerShrinkFactorHard, settings.AnthropicAdaptiveSchedulerShrinkFactorHard)
	settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerCapacityIncreaseStep, settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep)
	settings.AnthropicAdaptiveSchedulerMinCapacity = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerMinCapacity, settings.AnthropicAdaptiveSchedulerMinCapacity)
	settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier = parseIntSetting(values, SettingKeyAnthropicAdaptiveSchedulerHardShrinkFailureMultiplier, settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier)
	return NormalizeAnthropicAdaptiveSchedulerSettings(settings)
}

func anthropicAdaptiveSchedulerSettingsToMap(settings AnthropicAdaptiveSchedulerSettings) map[string]string {
	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)
	return map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled:                     strconv.FormatBool(settings.AnthropicAdaptiveSchedulerEnabled),
		SettingKeyAnthropicAdaptiveSchedulerMode:                        settings.AnthropicAdaptiveSchedulerMode,
		SettingKeyAnthropicAdaptiveSchedulerTopK:                        strconv.Itoa(settings.AnthropicAdaptiveSchedulerTopK),
		SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature:          formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerSoftmaxTemperature),
		SettingKeyAnthropicAdaptiveSchedulerWeightReliability:           formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightReliability),
		SettingKeyAnthropicAdaptiveSchedulerWeightCapacity:              formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightCapacity),
		SettingKeyAnthropicAdaptiveSchedulerWeightLatency:               formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightLatency),
		SettingKeyAnthropicAdaptiveSchedulerWeightExploration:           formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerWeightExploration),
		SettingKeyAnthropicAdaptiveSchedulerInitialReliability:          formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerInitialReliability),
		SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty:   formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty),
		SettingKeyAnthropicAdaptiveSchedulerNeutralLatencyScore:         formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerNeutralLatencyScore),
		SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha:             formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha),
		SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha:             formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha),
		SettingKeyAnthropicAdaptiveSchedulerCapacitySuccessThreshold:    formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold),
		SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold:  formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold),
		SettingKeyAnthropicAdaptiveSchedulerCapacityFailureThreshold:    strconv.Itoa(settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold),
		SettingKeyAnthropicAdaptiveSchedulerMinRecentSamplesForShrink:   strconv.Itoa(settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink),
		SettingKeyAnthropicAdaptiveSchedulerShrinkErrorThreshold:        formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold),
		SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds:       strconv.Itoa(settings.AnthropicAdaptiveSchedulerLearningWindowSeconds),
		SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds:             strconv.Itoa(settings.AnthropicAdaptiveSchedulerCooldownSeconds),
		SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft:            formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerShrinkFactorSoft),
		SettingKeyAnthropicAdaptiveSchedulerShrinkFactorHard:            formatOpenAIAdaptiveFloat(settings.AnthropicAdaptiveSchedulerShrinkFactorHard),
		SettingKeyAnthropicAdaptiveSchedulerCapacityIncreaseStep:        strconv.Itoa(settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep),
		SettingKeyAnthropicAdaptiveSchedulerMinCapacity:                 strconv.Itoa(settings.AnthropicAdaptiveSchedulerMinCapacity),
		SettingKeyAnthropicAdaptiveSchedulerHardShrinkFailureMultiplier: strconv.Itoa(settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier),
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
			SettingKeyAnthropicAdaptiveSchedulerMode,
			SettingKeyAnthropicAdaptiveSchedulerTopK,
			SettingKeyAnthropicAdaptiveSchedulerSoftmaxTemperature,
			SettingKeyAnthropicAdaptiveSchedulerWeightReliability,
			SettingKeyAnthropicAdaptiveSchedulerWeightCapacity,
			SettingKeyAnthropicAdaptiveSchedulerWeightLatency,
			SettingKeyAnthropicAdaptiveSchedulerWeightExploration,
			SettingKeyAnthropicAdaptiveSchedulerInitialReliability,
			SettingKeyAnthropicAdaptiveSchedulerConsecutiveFailurePenalty,
			SettingKeyAnthropicAdaptiveSchedulerNeutralLatencyScore,
			SettingKeyAnthropicAdaptiveSchedulerSuccessEMAAlpha,
			SettingKeyAnthropicAdaptiveSchedulerLatencyEMAAlpha,
			SettingKeyAnthropicAdaptiveSchedulerCapacitySuccessThreshold,
			SettingKeyAnthropicAdaptiveSchedulerCapacityProbeLoadThreshold,
			SettingKeyAnthropicAdaptiveSchedulerCapacityFailureThreshold,
			SettingKeyAnthropicAdaptiveSchedulerMinRecentSamplesForShrink,
			SettingKeyAnthropicAdaptiveSchedulerShrinkErrorThreshold,
			SettingKeyAnthropicAdaptiveSchedulerLearningWindowSeconds,
			SettingKeyAnthropicAdaptiveSchedulerCooldownSeconds,
			SettingKeyAnthropicAdaptiveSchedulerShrinkFactorSoft,
			SettingKeyAnthropicAdaptiveSchedulerShrinkFactorHard,
			SettingKeyAnthropicAdaptiveSchedulerCapacityIncreaseStep,
			SettingKeyAnthropicAdaptiveSchedulerMinCapacity,
			SettingKeyAnthropicAdaptiveSchedulerHardShrinkFailureMultiplier,
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
