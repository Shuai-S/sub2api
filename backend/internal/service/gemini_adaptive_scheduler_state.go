package service

import (
	"context"
	"strings"
	"time"
)

const (
	geminiAdaptiveCircuitStatusClosed   = "closed"
	geminiAdaptiveCircuitStatusHalfOpen = "half_open"
)

func geminiAdaptiveCoreSettings(settings GeminiAdaptiveSchedulerSettings) adaptiveCoreSettings {
	core := defaultAdaptiveCoreSettings()
	core.Mode = normalizeGeminiAdaptiveSchedulerMode(settings.GeminiAdaptiveSchedulerMode)
	core.TopK = settings.GeminiAdaptiveSchedulerTopK
	core.SoftmaxTemperature = settings.GeminiAdaptiveSchedulerSoftmaxTemperature
	core.ExplorationRate = settings.GeminiAdaptiveSchedulerExplorationRate
	core.LearningWindow = time.Duration(settings.GeminiAdaptiveSchedulerLearningWindowSeconds) * time.Second
	core.LearningMinHealthSamples = settings.GeminiAdaptiveSchedulerLearningMinHealthSamples
	core.SuccessEMAAlpha = settings.GeminiAdaptiveSchedulerSuccessEMAAlpha
	core.TTFTEMAAlpha = settings.GeminiAdaptiveSchedulerLatencyEMAAlpha
	core.ConsecutiveFailurePenalty = settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty
	core.HealthFailureThreshold = settings.GeminiAdaptiveSchedulerAccountFailureThreshold
	core.CircuitCooldownInitial = time.Duration(settings.GeminiAdaptiveSchedulerCooldownSeconds) * time.Second
	core.CircuitCooldownMaximum = time.Duration(settings.GeminiAdaptiveSchedulerCooldownMaxSeconds) * time.Second
	core.HighErrorMinSamples = settings.GeminiAdaptiveSchedulerHighErrorMinSamples
	core.HighErrorMaxSamples = settings.GeminiAdaptiveSchedulerHighErrorMaxSamples
	core.HighErrorEnterRate = settings.GeminiAdaptiveSchedulerHighErrorEnterRate
	core.HighErrorExitRate = settings.GeminiAdaptiveSchedulerHighErrorExitRate
	core.CapacityShrinkFactor = settings.GeminiAdaptiveSchedulerShrinkFactorSoft
	core.CapacityRecoveryFactor = settings.GeminiAdaptiveSchedulerCapacityGrowthFactor
	core.CapacityRecoverySamples = settings.GeminiAdaptiveSchedulerCapacityRecoverySamples
	core.CapacityCooldown = time.Duration(settings.GeminiAdaptiveSchedulerCooldownSeconds) * time.Second
	core.QuotaProbeInterval = time.Duration(settings.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds) * time.Second
	core.WeightReliability = settings.GeminiAdaptiveSchedulerWeightReliability
	core.WeightCapacity = settings.GeminiAdaptiveSchedulerWeightCapacity
	core.WeightTTFT = settings.GeminiAdaptiveSchedulerWeightLatency
	core.WeightCost = settings.GeminiAdaptiveSchedulerWeightCost
	core.WeightCache = settings.GeminiAdaptiveSchedulerWeightCache
	return normalizeAdaptiveCoreSettings(core)
}

type GeminiAdaptiveScheduleReport struct {
	Account             *Account
	RequestID           string
	RequestedModel      string
	MappedModel         string
	UpstreamRequestID   string
	Stream              bool
	Action              string
	Success             bool
	PathSample          bool
	Synthetic           bool
	FirstTokenMs        *int
	DurationMs          int64
	TerminalReason      string
	CacheInputTokens    int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ctx                 context.Context
}

func geminiAdaptiveCanonicalModel(account *Account, requestedModel, mappedModel, action string) string {
	_ = action
	model := strings.TrimSpace(mappedModel)
	if model == "" && account != nil {
		model = strings.TrimSpace(account.GetMappedModel(requestedModel))
	}
	if model == "" {
		model = strings.TrimSpace(requestedModel)
	}
	if model == "" {
		model = "unknown"
	}
	return strings.ToLower(model)
}
