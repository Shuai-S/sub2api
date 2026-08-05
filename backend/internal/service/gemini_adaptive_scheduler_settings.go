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

	SettingKeyGeminiAdaptiveSchedulerEnabled                     = geminiAdaptiveSchedulerSettingPrefix + "enabled"
	SettingKeyGeminiAdaptiveSchedulerMode                        = geminiAdaptiveSchedulerSettingPrefix + "mode"
	SettingKeyGeminiAdaptiveSchedulerStickyEscapeOnCapacityFull  = geminiAdaptiveSchedulerSettingPrefix + "sticky_escape_on_capacity_full"
	SettingKeyGeminiAdaptiveSchedulerTopK                        = geminiAdaptiveSchedulerSettingPrefix + "top_k"
	SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature          = geminiAdaptiveSchedulerSettingPrefix + "softmax_temperature"
	SettingKeyGeminiAdaptiveSchedulerInitialReliability          = geminiAdaptiveSchedulerSettingPrefix + "initial_reliability"
	SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty   = geminiAdaptiveSchedulerSettingPrefix + "consecutive_failure_penalty"
	SettingKeyGeminiAdaptiveSchedulerNeutralLatencyScore         = geminiAdaptiveSchedulerSettingPrefix + "neutral_latency_score"
	SettingKeyGeminiAdaptiveSchedulerNeutralQuotaScore           = geminiAdaptiveSchedulerSettingPrefix + "neutral_quota_score"
	SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha             = geminiAdaptiveSchedulerSettingPrefix + "success_ema_alpha"
	SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha             = geminiAdaptiveSchedulerSettingPrefix + "latency_ema_alpha"
	SettingKeyGeminiAdaptiveSchedulerMinCostMultiplier           = geminiAdaptiveSchedulerSettingPrefix + "min_cost_multiplier"
	SettingKeyGeminiAdaptiveSchedulerWeightReliability           = geminiAdaptiveSchedulerSettingPrefix + "weight_reliability"
	SettingKeyGeminiAdaptiveSchedulerWeightQuota                 = geminiAdaptiveSchedulerSettingPrefix + "weight_quota"
	SettingKeyGeminiAdaptiveSchedulerWeightCapacity              = geminiAdaptiveSchedulerSettingPrefix + "weight_capacity"
	SettingKeyGeminiAdaptiveSchedulerWeightLatency               = geminiAdaptiveSchedulerSettingPrefix + "weight_latency"
	SettingKeyGeminiAdaptiveSchedulerWeightCost                  = geminiAdaptiveSchedulerSettingPrefix + "weight_cost"
	SettingKeyGeminiAdaptiveSchedulerWeightExploration           = geminiAdaptiveSchedulerSettingPrefix + "weight_exploration"
	SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold  = geminiAdaptiveSchedulerSettingPrefix + "capacity_probe_load_threshold"
	SettingKeyGeminiAdaptiveSchedulerCapacitySuccessThreshold    = geminiAdaptiveSchedulerSettingPrefix + "capacity_success_threshold"
	SettingKeyGeminiAdaptiveSchedulerCapacityIncreaseStep        = geminiAdaptiveSchedulerSettingPrefix + "capacity_increase_step"
	SettingKeyGeminiAdaptiveSchedulerMinCapacity                 = geminiAdaptiveSchedulerSettingPrefix + "min_capacity"
	SettingKeyGeminiAdaptiveSchedulerCapacityFailureThreshold    = geminiAdaptiveSchedulerSettingPrefix + "capacity_failure_threshold"
	SettingKeyGeminiAdaptiveSchedulerMinRecentSamplesForShrink   = geminiAdaptiveSchedulerSettingPrefix + "min_recent_samples_for_shrink"
	SettingKeyGeminiAdaptiveSchedulerShrinkErrorThreshold        = geminiAdaptiveSchedulerSettingPrefix + "shrink_error_threshold"
	SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft            = geminiAdaptiveSchedulerSettingPrefix + "shrink_factor_soft"
	SettingKeyGeminiAdaptiveSchedulerShrinkFactorHard            = geminiAdaptiveSchedulerSettingPrefix + "shrink_factor_hard"
	SettingKeyGeminiAdaptiveSchedulerHardShrinkFailureMultiplier = geminiAdaptiveSchedulerSettingPrefix + "hard_shrink_failure_multiplier"
	SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds       = geminiAdaptiveSchedulerSettingPrefix + "learning_window_seconds"
	SettingKeyGeminiAdaptiveSchedulerCooldownSeconds             = geminiAdaptiveSchedulerSettingPrefix + "cooldown_seconds"
	SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds          = geminiAdaptiveSchedulerSettingPrefix + "cooldown_max_seconds"
	SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold     = geminiAdaptiveSchedulerSettingPrefix + "account_failure_threshold"
	SettingKeyGeminiAdaptiveSchedulerModelFailureThreshold       = geminiAdaptiveSchedulerSettingPrefix + "model_failure_threshold"
	SettingKeyGeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds   = geminiAdaptiveSchedulerSettingPrefix + "half_open_probe_lease_seconds"
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled        = geminiAdaptiveSchedulerSettingPrefix + "diagnostic_log_enabled"
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate     = geminiAdaptiveSchedulerSettingPrefix + "diagnostic_log_sample_rate"

	geminiAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	geminiAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
)

type GeminiAdaptiveSchedulerSettings struct {
	GeminiAdaptiveSchedulerEnabled                     bool    `json:"gemini_adaptive_scheduler_enabled"`
	GeminiAdaptiveSchedulerMode                        string  `json:"gemini_adaptive_scheduler_mode"`
	GeminiAdaptiveSchedulerStickyEscapeOnCapacityFull  bool    `json:"gemini_adaptive_scheduler_sticky_escape_on_capacity_full"`
	GeminiAdaptiveSchedulerTopK                        int     `json:"gemini_adaptive_scheduler_top_k"`
	GeminiAdaptiveSchedulerSoftmaxTemperature          float64 `json:"gemini_adaptive_scheduler_softmax_temperature"`
	GeminiAdaptiveSchedulerInitialReliability          float64 `json:"gemini_adaptive_scheduler_initial_reliability"`
	GeminiAdaptiveSchedulerConsecutiveFailurePenalty   float64 `json:"gemini_adaptive_scheduler_consecutive_failure_penalty"`
	GeminiAdaptiveSchedulerNeutralLatencyScore         float64 `json:"gemini_adaptive_scheduler_neutral_latency_score"`
	GeminiAdaptiveSchedulerNeutralQuotaScore           float64 `json:"gemini_adaptive_scheduler_neutral_quota_score"`
	GeminiAdaptiveSchedulerSuccessEMAAlpha             float64 `json:"gemini_adaptive_scheduler_success_ema_alpha"`
	GeminiAdaptiveSchedulerLatencyEMAAlpha             float64 `json:"gemini_adaptive_scheduler_latency_ema_alpha"`
	GeminiAdaptiveSchedulerMinCostMultiplier           float64 `json:"gemini_adaptive_scheduler_min_cost_multiplier"`
	GeminiAdaptiveSchedulerWeightReliability           float64 `json:"gemini_adaptive_scheduler_weight_reliability"`
	GeminiAdaptiveSchedulerWeightQuota                 float64 `json:"gemini_adaptive_scheduler_weight_quota"`
	GeminiAdaptiveSchedulerWeightCapacity              float64 `json:"gemini_adaptive_scheduler_weight_capacity"`
	GeminiAdaptiveSchedulerWeightLatency               float64 `json:"gemini_adaptive_scheduler_weight_latency"`
	GeminiAdaptiveSchedulerWeightCost                  float64 `json:"gemini_adaptive_scheduler_weight_cost"`
	GeminiAdaptiveSchedulerWeightExploration           float64 `json:"gemini_adaptive_scheduler_weight_exploration"`
	GeminiAdaptiveSchedulerCapacityProbeLoadThreshold  float64 `json:"gemini_adaptive_scheduler_capacity_probe_load_threshold"`
	GeminiAdaptiveSchedulerCapacitySuccessThreshold    float64 `json:"gemini_adaptive_scheduler_capacity_success_threshold"`
	GeminiAdaptiveSchedulerCapacityIncreaseStep        int     `json:"gemini_adaptive_scheduler_capacity_increase_step"`
	GeminiAdaptiveSchedulerMinCapacity                 int     `json:"gemini_adaptive_scheduler_min_capacity"`
	GeminiAdaptiveSchedulerCapacityFailureThreshold    int     `json:"gemini_adaptive_scheduler_capacity_failure_threshold"`
	GeminiAdaptiveSchedulerMinRecentSamplesForShrink   int     `json:"gemini_adaptive_scheduler_min_recent_samples_for_shrink"`
	GeminiAdaptiveSchedulerShrinkErrorThreshold        float64 `json:"gemini_adaptive_scheduler_shrink_error_threshold"`
	GeminiAdaptiveSchedulerShrinkFactorSoft            float64 `json:"gemini_adaptive_scheduler_shrink_factor_soft"`
	GeminiAdaptiveSchedulerShrinkFactorHard            float64 `json:"gemini_adaptive_scheduler_shrink_factor_hard"`
	GeminiAdaptiveSchedulerHardShrinkFailureMultiplier int     `json:"gemini_adaptive_scheduler_hard_shrink_failure_multiplier"`
	GeminiAdaptiveSchedulerLearningWindowSeconds       int     `json:"gemini_adaptive_scheduler_learning_window_seconds"`
	GeminiAdaptiveSchedulerCooldownSeconds             int     `json:"gemini_adaptive_scheduler_cooldown_seconds"`
	GeminiAdaptiveSchedulerCooldownMaxSeconds          int     `json:"gemini_adaptive_scheduler_cooldown_max_seconds"`
	GeminiAdaptiveSchedulerAccountFailureThreshold     int     `json:"gemini_adaptive_scheduler_account_failure_threshold"`
	GeminiAdaptiveSchedulerModelFailureThreshold       int     `json:"gemini_adaptive_scheduler_model_failure_threshold"`
	GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds   int     `json:"gemini_adaptive_scheduler_half_open_probe_lease_seconds"`
	GeminiAdaptiveSchedulerDiagnosticLogEnabled        bool    `json:"gemini_adaptive_scheduler_diagnostic_log_enabled"`
	GeminiAdaptiveSchedulerDiagnosticLogSampleRate     float64 `json:"gemini_adaptive_scheduler_diagnostic_log_sample_rate"`
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
		GeminiAdaptiveSchedulerEnabled:                     false,
		GeminiAdaptiveSchedulerMode:                        GeminiAdaptiveSchedulerModeShadow,
		GeminiAdaptiveSchedulerStickyEscapeOnCapacityFull:  false,
		GeminiAdaptiveSchedulerTopK:                        4,
		GeminiAdaptiveSchedulerSoftmaxTemperature:          0.12,
		GeminiAdaptiveSchedulerInitialReliability:          0.50,
		GeminiAdaptiveSchedulerConsecutiveFailurePenalty:   0.25,
		GeminiAdaptiveSchedulerNeutralLatencyScore:         0.50,
		GeminiAdaptiveSchedulerNeutralQuotaScore:           0.50,
		GeminiAdaptiveSchedulerSuccessEMAAlpha:             0.05,
		GeminiAdaptiveSchedulerLatencyEMAAlpha:             0.05,
		GeminiAdaptiveSchedulerMinCostMultiplier:           0.03,
		GeminiAdaptiveSchedulerWeightReliability:           0.55,
		GeminiAdaptiveSchedulerWeightQuota:                 0.20,
		GeminiAdaptiveSchedulerWeightCapacity:              0.10,
		GeminiAdaptiveSchedulerWeightLatency:               0.10,
		GeminiAdaptiveSchedulerWeightCost:                  0.03,
		GeminiAdaptiveSchedulerWeightExploration:           0.02,
		GeminiAdaptiveSchedulerCapacityProbeLoadThreshold:  0.80,
		GeminiAdaptiveSchedulerCapacitySuccessThreshold:    0.97,
		GeminiAdaptiveSchedulerCapacityIncreaseStep:        1,
		GeminiAdaptiveSchedulerMinCapacity:                 1,
		GeminiAdaptiveSchedulerCapacityFailureThreshold:    3,
		GeminiAdaptiveSchedulerMinRecentSamplesForShrink:   30,
		GeminiAdaptiveSchedulerShrinkErrorThreshold:        0.25,
		GeminiAdaptiveSchedulerShrinkFactorSoft:            0.85,
		GeminiAdaptiveSchedulerShrinkFactorHard:            0.60,
		GeminiAdaptiveSchedulerHardShrinkFailureMultiplier: 2,
		GeminiAdaptiveSchedulerLearningWindowSeconds:       1200,
		GeminiAdaptiveSchedulerCooldownSeconds:             60,
		GeminiAdaptiveSchedulerCooldownMaxSeconds:          600,
		GeminiAdaptiveSchedulerAccountFailureThreshold:     3,
		GeminiAdaptiveSchedulerModelFailureThreshold:       3,
		GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds:   600,
		GeminiAdaptiveSchedulerDiagnosticLogEnabled:        false,
		GeminiAdaptiveSchedulerDiagnosticLogSampleRate:     0.05,
	}
}

func NormalizeGeminiAdaptiveSchedulerSettings(settings GeminiAdaptiveSchedulerSettings) GeminiAdaptiveSchedulerSettings {
	defaults := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerMode = normalizeGeminiAdaptiveSchedulerMode(settings.GeminiAdaptiveSchedulerMode)
	settings.GeminiAdaptiveSchedulerTopK = clampInt(settings.GeminiAdaptiveSchedulerTopK, 1, 100, defaults.GeminiAdaptiveSchedulerTopK)
	settings.GeminiAdaptiveSchedulerSoftmaxTemperature = clampFloat(settings.GeminiAdaptiveSchedulerSoftmaxTemperature, 0.01, 10, defaults.GeminiAdaptiveSchedulerSoftmaxTemperature)
	settings.GeminiAdaptiveSchedulerInitialReliability = clampFloat(settings.GeminiAdaptiveSchedulerInitialReliability, 0, 1, defaults.GeminiAdaptiveSchedulerInitialReliability)
	settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty = nonNegativeFinite(settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty)
	settings.GeminiAdaptiveSchedulerNeutralLatencyScore = clampFloat(settings.GeminiAdaptiveSchedulerNeutralLatencyScore, 0, 1, defaults.GeminiAdaptiveSchedulerNeutralLatencyScore)
	settings.GeminiAdaptiveSchedulerNeutralQuotaScore = clampFloat(settings.GeminiAdaptiveSchedulerNeutralQuotaScore, 0, 1, defaults.GeminiAdaptiveSchedulerNeutralQuotaScore)
	settings.GeminiAdaptiveSchedulerSuccessEMAAlpha = clampFloat(settings.GeminiAdaptiveSchedulerSuccessEMAAlpha, 0, 1, defaults.GeminiAdaptiveSchedulerSuccessEMAAlpha)
	settings.GeminiAdaptiveSchedulerLatencyEMAAlpha = clampFloat(settings.GeminiAdaptiveSchedulerLatencyEMAAlpha, 0, 1, defaults.GeminiAdaptiveSchedulerLatencyEMAAlpha)
	settings.GeminiAdaptiveSchedulerMinCostMultiplier = clampFloat(settings.GeminiAdaptiveSchedulerMinCostMultiplier, 0.0001, 1, defaults.GeminiAdaptiveSchedulerMinCostMultiplier)
	settings.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold = clampFloat(settings.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold, 0, 1, defaults.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold)
	settings.GeminiAdaptiveSchedulerCapacitySuccessThreshold = clampFloat(settings.GeminiAdaptiveSchedulerCapacitySuccessThreshold, 0, 1, defaults.GeminiAdaptiveSchedulerCapacitySuccessThreshold)
	settings.GeminiAdaptiveSchedulerCapacityIncreaseStep = clampIntMin(settings.GeminiAdaptiveSchedulerCapacityIncreaseStep, 1, defaults.GeminiAdaptiveSchedulerCapacityIncreaseStep)
	settings.GeminiAdaptiveSchedulerMinCapacity = clampIntMin(settings.GeminiAdaptiveSchedulerMinCapacity, 1, defaults.GeminiAdaptiveSchedulerMinCapacity)
	settings.GeminiAdaptiveSchedulerCapacityFailureThreshold = clampIntMin(settings.GeminiAdaptiveSchedulerCapacityFailureThreshold, 1, defaults.GeminiAdaptiveSchedulerCapacityFailureThreshold)
	settings.GeminiAdaptiveSchedulerMinRecentSamplesForShrink = clampIntMin(settings.GeminiAdaptiveSchedulerMinRecentSamplesForShrink, 1, defaults.GeminiAdaptiveSchedulerMinRecentSamplesForShrink)
	settings.GeminiAdaptiveSchedulerShrinkErrorThreshold = clampFloat(settings.GeminiAdaptiveSchedulerShrinkErrorThreshold, 0, 1, defaults.GeminiAdaptiveSchedulerShrinkErrorThreshold)
	settings.GeminiAdaptiveSchedulerShrinkFactorSoft = clampFloat(settings.GeminiAdaptiveSchedulerShrinkFactorSoft, 0.01, 1, defaults.GeminiAdaptiveSchedulerShrinkFactorSoft)
	settings.GeminiAdaptiveSchedulerShrinkFactorHard = clampFloat(settings.GeminiAdaptiveSchedulerShrinkFactorHard, 0.01, 1, defaults.GeminiAdaptiveSchedulerShrinkFactorHard)
	if settings.GeminiAdaptiveSchedulerShrinkFactorHard > settings.GeminiAdaptiveSchedulerShrinkFactorSoft {
		settings.GeminiAdaptiveSchedulerShrinkFactorHard = settings.GeminiAdaptiveSchedulerShrinkFactorSoft
	}
	settings.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier = clampInt(settings.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier, 1, 100, defaults.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier)
	settings.GeminiAdaptiveSchedulerLearningWindowSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerLearningWindowSeconds, 1, defaults.GeminiAdaptiveSchedulerLearningWindowSeconds)
	settings.GeminiAdaptiveSchedulerCooldownSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerCooldownSeconds, 0, defaults.GeminiAdaptiveSchedulerCooldownSeconds)
	settings.GeminiAdaptiveSchedulerCooldownMaxSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerCooldownMaxSeconds, settings.GeminiAdaptiveSchedulerCooldownSeconds, defaults.GeminiAdaptiveSchedulerCooldownMaxSeconds)
	settings.GeminiAdaptiveSchedulerAccountFailureThreshold = clampInt(settings.GeminiAdaptiveSchedulerAccountFailureThreshold, 1, 100, defaults.GeminiAdaptiveSchedulerAccountFailureThreshold)
	settings.GeminiAdaptiveSchedulerModelFailureThreshold = clampInt(settings.GeminiAdaptiveSchedulerModelFailureThreshold, 1, 100, defaults.GeminiAdaptiveSchedulerModelFailureThreshold)
	settings.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds = clampIntMin(settings.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds, 1, defaults.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds)
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = clampFloat(settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate, 0, 1, defaults.GeminiAdaptiveSchedulerDiagnosticLogSampleRate)

	weights := []*float64{
		&settings.GeminiAdaptiveSchedulerWeightReliability,
		&settings.GeminiAdaptiveSchedulerWeightQuota,
		&settings.GeminiAdaptiveSchedulerWeightCapacity,
		&settings.GeminiAdaptiveSchedulerWeightLatency,
		&settings.GeminiAdaptiveSchedulerWeightCost,
		&settings.GeminiAdaptiveSchedulerWeightExploration,
	}
	weightSum := 0.0
	for _, weight := range weights {
		*weight = nonNegativeFinite(*weight)
		weightSum += *weight
	}
	if weightSum <= 0 {
		settings.GeminiAdaptiveSchedulerWeightReliability = defaults.GeminiAdaptiveSchedulerWeightReliability
		settings.GeminiAdaptiveSchedulerWeightQuota = defaults.GeminiAdaptiveSchedulerWeightQuota
		settings.GeminiAdaptiveSchedulerWeightCapacity = defaults.GeminiAdaptiveSchedulerWeightCapacity
		settings.GeminiAdaptiveSchedulerWeightLatency = defaults.GeminiAdaptiveSchedulerWeightLatency
		settings.GeminiAdaptiveSchedulerWeightCost = defaults.GeminiAdaptiveSchedulerWeightCost
		settings.GeminiAdaptiveSchedulerWeightExploration = defaults.GeminiAdaptiveSchedulerWeightExploration
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
	SettingKeyGeminiAdaptiveSchedulerStickyEscapeOnCapacityFull,
	SettingKeyGeminiAdaptiveSchedulerTopK,
	SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature,
	SettingKeyGeminiAdaptiveSchedulerInitialReliability,
	SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty,
	SettingKeyGeminiAdaptiveSchedulerNeutralLatencyScore,
	SettingKeyGeminiAdaptiveSchedulerNeutralQuotaScore,
	SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha,
	SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha,
	SettingKeyGeminiAdaptiveSchedulerMinCostMultiplier,
	SettingKeyGeminiAdaptiveSchedulerWeightReliability,
	SettingKeyGeminiAdaptiveSchedulerWeightQuota,
	SettingKeyGeminiAdaptiveSchedulerWeightCapacity,
	SettingKeyGeminiAdaptiveSchedulerWeightLatency,
	SettingKeyGeminiAdaptiveSchedulerWeightCost,
	SettingKeyGeminiAdaptiveSchedulerWeightExploration,
	SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold,
	SettingKeyGeminiAdaptiveSchedulerCapacitySuccessThreshold,
	SettingKeyGeminiAdaptiveSchedulerCapacityIncreaseStep,
	SettingKeyGeminiAdaptiveSchedulerMinCapacity,
	SettingKeyGeminiAdaptiveSchedulerCapacityFailureThreshold,
	SettingKeyGeminiAdaptiveSchedulerMinRecentSamplesForShrink,
	SettingKeyGeminiAdaptiveSchedulerShrinkErrorThreshold,
	SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft,
	SettingKeyGeminiAdaptiveSchedulerShrinkFactorHard,
	SettingKeyGeminiAdaptiveSchedulerHardShrinkFailureMultiplier,
	SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds,
	SettingKeyGeminiAdaptiveSchedulerCooldownSeconds,
	SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds,
	SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold,
	SettingKeyGeminiAdaptiveSchedulerModelFailureThreshold,
	SettingKeyGeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds,
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled,
	SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate,
}

func parseGeminiAdaptiveSchedulerSettings(values map[string]string) GeminiAdaptiveSchedulerSettings {
	s := DefaultGeminiAdaptiveSchedulerSettings()
	s.GeminiAdaptiveSchedulerEnabled = parseBoolSetting(values, SettingKeyGeminiAdaptiveSchedulerEnabled, s.GeminiAdaptiveSchedulerEnabled)
	s.GeminiAdaptiveSchedulerMode = firstNonEmpty(values[SettingKeyGeminiAdaptiveSchedulerMode], s.GeminiAdaptiveSchedulerMode)
	s.GeminiAdaptiveSchedulerStickyEscapeOnCapacityFull = parseBoolSetting(values, SettingKeyGeminiAdaptiveSchedulerStickyEscapeOnCapacityFull, s.GeminiAdaptiveSchedulerStickyEscapeOnCapacityFull)
	s.GeminiAdaptiveSchedulerTopK = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerTopK, s.GeminiAdaptiveSchedulerTopK)
	s.GeminiAdaptiveSchedulerSoftmaxTemperature = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature, s.GeminiAdaptiveSchedulerSoftmaxTemperature)
	s.GeminiAdaptiveSchedulerInitialReliability = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerInitialReliability, s.GeminiAdaptiveSchedulerInitialReliability)
	s.GeminiAdaptiveSchedulerConsecutiveFailurePenalty = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty, s.GeminiAdaptiveSchedulerConsecutiveFailurePenalty)
	s.GeminiAdaptiveSchedulerNeutralLatencyScore = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerNeutralLatencyScore, s.GeminiAdaptiveSchedulerNeutralLatencyScore)
	s.GeminiAdaptiveSchedulerNeutralQuotaScore = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerNeutralQuotaScore, s.GeminiAdaptiveSchedulerNeutralQuotaScore)
	s.GeminiAdaptiveSchedulerSuccessEMAAlpha = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha, s.GeminiAdaptiveSchedulerSuccessEMAAlpha)
	s.GeminiAdaptiveSchedulerLatencyEMAAlpha = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha, s.GeminiAdaptiveSchedulerLatencyEMAAlpha)
	s.GeminiAdaptiveSchedulerMinCostMultiplier = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerMinCostMultiplier, s.GeminiAdaptiveSchedulerMinCostMultiplier)
	s.GeminiAdaptiveSchedulerWeightReliability = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightReliability, s.GeminiAdaptiveSchedulerWeightReliability)
	s.GeminiAdaptiveSchedulerWeightQuota = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightQuota, s.GeminiAdaptiveSchedulerWeightQuota)
	s.GeminiAdaptiveSchedulerWeightCapacity = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightCapacity, s.GeminiAdaptiveSchedulerWeightCapacity)
	s.GeminiAdaptiveSchedulerWeightLatency = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightLatency, s.GeminiAdaptiveSchedulerWeightLatency)
	s.GeminiAdaptiveSchedulerWeightCost = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightCost, s.GeminiAdaptiveSchedulerWeightCost)
	s.GeminiAdaptiveSchedulerWeightExploration = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerWeightExploration, s.GeminiAdaptiveSchedulerWeightExploration)
	s.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold, s.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold)
	s.GeminiAdaptiveSchedulerCapacitySuccessThreshold = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacitySuccessThreshold, s.GeminiAdaptiveSchedulerCapacitySuccessThreshold)
	s.GeminiAdaptiveSchedulerCapacityIncreaseStep = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacityIncreaseStep, s.GeminiAdaptiveSchedulerCapacityIncreaseStep)
	s.GeminiAdaptiveSchedulerMinCapacity = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerMinCapacity, s.GeminiAdaptiveSchedulerMinCapacity)
	s.GeminiAdaptiveSchedulerCapacityFailureThreshold = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCapacityFailureThreshold, s.GeminiAdaptiveSchedulerCapacityFailureThreshold)
	s.GeminiAdaptiveSchedulerMinRecentSamplesForShrink = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerMinRecentSamplesForShrink, s.GeminiAdaptiveSchedulerMinRecentSamplesForShrink)
	s.GeminiAdaptiveSchedulerShrinkErrorThreshold = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerShrinkErrorThreshold, s.GeminiAdaptiveSchedulerShrinkErrorThreshold)
	s.GeminiAdaptiveSchedulerShrinkFactorSoft = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft, s.GeminiAdaptiveSchedulerShrinkFactorSoft)
	s.GeminiAdaptiveSchedulerShrinkFactorHard = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerShrinkFactorHard, s.GeminiAdaptiveSchedulerShrinkFactorHard)
	s.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerHardShrinkFailureMultiplier, s.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier)
	s.GeminiAdaptiveSchedulerLearningWindowSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds, s.GeminiAdaptiveSchedulerLearningWindowSeconds)
	s.GeminiAdaptiveSchedulerCooldownSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCooldownSeconds, s.GeminiAdaptiveSchedulerCooldownSeconds)
	s.GeminiAdaptiveSchedulerCooldownMaxSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds, s.GeminiAdaptiveSchedulerCooldownMaxSeconds)
	s.GeminiAdaptiveSchedulerAccountFailureThreshold = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold, s.GeminiAdaptiveSchedulerAccountFailureThreshold)
	s.GeminiAdaptiveSchedulerModelFailureThreshold = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerModelFailureThreshold, s.GeminiAdaptiveSchedulerModelFailureThreshold)
	s.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds = parseIntSetting(values, SettingKeyGeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds, s.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds)
	s.GeminiAdaptiveSchedulerDiagnosticLogEnabled = parseBoolSetting(values, SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled, s.GeminiAdaptiveSchedulerDiagnosticLogEnabled)
	s.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = parseFloatSetting(values, SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate, s.GeminiAdaptiveSchedulerDiagnosticLogSampleRate)
	return NormalizeGeminiAdaptiveSchedulerSettings(s)
}

func geminiAdaptiveSchedulerSettingsToMap(s GeminiAdaptiveSchedulerSettings) map[string]string {
	s = NormalizeGeminiAdaptiveSchedulerSettings(s)
	return map[string]string{
		SettingKeyGeminiAdaptiveSchedulerEnabled:                     strconv.FormatBool(s.GeminiAdaptiveSchedulerEnabled),
		SettingKeyGeminiAdaptiveSchedulerMode:                        s.GeminiAdaptiveSchedulerMode,
		SettingKeyGeminiAdaptiveSchedulerStickyEscapeOnCapacityFull:  strconv.FormatBool(s.GeminiAdaptiveSchedulerStickyEscapeOnCapacityFull),
		SettingKeyGeminiAdaptiveSchedulerTopK:                        strconv.Itoa(s.GeminiAdaptiveSchedulerTopK),
		SettingKeyGeminiAdaptiveSchedulerSoftmaxTemperature:          formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerSoftmaxTemperature),
		SettingKeyGeminiAdaptiveSchedulerInitialReliability:          formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerInitialReliability),
		SettingKeyGeminiAdaptiveSchedulerConsecutiveFailurePenalty:   formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerConsecutiveFailurePenalty),
		SettingKeyGeminiAdaptiveSchedulerNeutralLatencyScore:         formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerNeutralLatencyScore),
		SettingKeyGeminiAdaptiveSchedulerNeutralQuotaScore:           formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerNeutralQuotaScore),
		SettingKeyGeminiAdaptiveSchedulerSuccessEMAAlpha:             formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerSuccessEMAAlpha),
		SettingKeyGeminiAdaptiveSchedulerLatencyEMAAlpha:             formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerLatencyEMAAlpha),
		SettingKeyGeminiAdaptiveSchedulerMinCostMultiplier:           formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerMinCostMultiplier),
		SettingKeyGeminiAdaptiveSchedulerWeightReliability:           formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightReliability),
		SettingKeyGeminiAdaptiveSchedulerWeightQuota:                 formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightQuota),
		SettingKeyGeminiAdaptiveSchedulerWeightCapacity:              formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightCapacity),
		SettingKeyGeminiAdaptiveSchedulerWeightLatency:               formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightLatency),
		SettingKeyGeminiAdaptiveSchedulerWeightCost:                  formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightCost),
		SettingKeyGeminiAdaptiveSchedulerWeightExploration:           formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerWeightExploration),
		SettingKeyGeminiAdaptiveSchedulerCapacityProbeLoadThreshold:  formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold),
		SettingKeyGeminiAdaptiveSchedulerCapacitySuccessThreshold:    formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerCapacitySuccessThreshold),
		SettingKeyGeminiAdaptiveSchedulerCapacityIncreaseStep:        strconv.Itoa(s.GeminiAdaptiveSchedulerCapacityIncreaseStep),
		SettingKeyGeminiAdaptiveSchedulerMinCapacity:                 strconv.Itoa(s.GeminiAdaptiveSchedulerMinCapacity),
		SettingKeyGeminiAdaptiveSchedulerCapacityFailureThreshold:    strconv.Itoa(s.GeminiAdaptiveSchedulerCapacityFailureThreshold),
		SettingKeyGeminiAdaptiveSchedulerMinRecentSamplesForShrink:   strconv.Itoa(s.GeminiAdaptiveSchedulerMinRecentSamplesForShrink),
		SettingKeyGeminiAdaptiveSchedulerShrinkErrorThreshold:        formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerShrinkErrorThreshold),
		SettingKeyGeminiAdaptiveSchedulerShrinkFactorSoft:            formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerShrinkFactorSoft),
		SettingKeyGeminiAdaptiveSchedulerShrinkFactorHard:            formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerShrinkFactorHard),
		SettingKeyGeminiAdaptiveSchedulerHardShrinkFailureMultiplier: strconv.Itoa(s.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier),
		SettingKeyGeminiAdaptiveSchedulerLearningWindowSeconds:       strconv.Itoa(s.GeminiAdaptiveSchedulerLearningWindowSeconds),
		SettingKeyGeminiAdaptiveSchedulerCooldownSeconds:             strconv.Itoa(s.GeminiAdaptiveSchedulerCooldownSeconds),
		SettingKeyGeminiAdaptiveSchedulerCooldownMaxSeconds:          strconv.Itoa(s.GeminiAdaptiveSchedulerCooldownMaxSeconds),
		SettingKeyGeminiAdaptiveSchedulerAccountFailureThreshold:     strconv.Itoa(s.GeminiAdaptiveSchedulerAccountFailureThreshold),
		SettingKeyGeminiAdaptiveSchedulerModelFailureThreshold:       strconv.Itoa(s.GeminiAdaptiveSchedulerModelFailureThreshold),
		SettingKeyGeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds:   strconv.Itoa(s.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds),
		SettingKeyGeminiAdaptiveSchedulerDiagnosticLogEnabled:        strconv.FormatBool(s.GeminiAdaptiveSchedulerDiagnosticLogEnabled),
		SettingKeyGeminiAdaptiveSchedulerDiagnosticLogSampleRate:     formatOpenAIAdaptiveFloat(s.GeminiAdaptiveSchedulerDiagnosticLogSampleRate),
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
