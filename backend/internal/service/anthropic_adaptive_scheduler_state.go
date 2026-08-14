package service

import "time"

func anthropicAdaptiveCoreSettings(settings AnthropicAdaptiveSchedulerSettings) adaptiveCoreSettings {
	core := defaultAdaptiveCoreSettings()
	core.Mode = normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
	core.TopK = settings.AnthropicAdaptiveSchedulerTopK
	core.SoftmaxTemperature = settings.AnthropicAdaptiveSchedulerSoftmaxTemperature
	core.ExplorationRate = settings.AnthropicAdaptiveSchedulerExplorationRate
	core.LearningWindow = time.Duration(settings.AnthropicAdaptiveSchedulerLearningWindowSeconds) * time.Second
	core.LearningMinHealthSamples = settings.AnthropicAdaptiveSchedulerLearningMinHealthSamples
	core.SuccessEMAAlpha = settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha
	core.TTFTEMAAlpha = settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha
	core.ConsecutiveFailurePenalty = settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty
	core.HealthFailureThreshold = settings.AnthropicAdaptiveSchedulerHealthFailureThreshold
	core.CircuitCooldownInitial = time.Duration(settings.AnthropicAdaptiveSchedulerCooldownSeconds) * time.Second
	core.CircuitCooldownMaximum = time.Duration(settings.AnthropicAdaptiveSchedulerCooldownMaxSeconds) * time.Second
	core.HighErrorMinSamples = settings.AnthropicAdaptiveSchedulerHighErrorMinSamples
	core.HighErrorMaxSamples = settings.AnthropicAdaptiveSchedulerHighErrorMaxSamples
	core.HighErrorEnterRate = settings.AnthropicAdaptiveSchedulerHighErrorEnterRate
	core.HighErrorExitRate = settings.AnthropicAdaptiveSchedulerHighErrorExitRate
	core.CapacityShrinkFactor = settings.AnthropicAdaptiveSchedulerShrinkFactorSoft
	core.CapacityRecoveryFactor = settings.AnthropicAdaptiveSchedulerCapacityGrowthFactor
	core.CapacityRecoverySamples = settings.AnthropicAdaptiveSchedulerCapacityRecoverySamples
	core.CapacityRecoveryLoad = settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold
	core.CapacityCooldown = time.Duration(settings.AnthropicAdaptiveSchedulerCooldownSeconds) * time.Second
	core.QuotaProbeInterval = time.Duration(settings.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds) * time.Second
	core.WeightReliability = settings.AnthropicAdaptiveSchedulerWeightReliability
	core.WeightCapacity = settings.AnthropicAdaptiveSchedulerWeightCapacity
	core.WeightTTFT = settings.AnthropicAdaptiveSchedulerWeightLatency
	core.WeightCost = settings.AnthropicAdaptiveSchedulerWeightCost
	return normalizeAdaptiveCoreSettings(core)
}

type AnthropicAdaptiveScheduleReport struct {
	Account           *Account
	RequestID         string
	RequestedModel    string
	UpstreamRequestID string
	MappedModel       string
	Stream            bool
	Synthetic         bool
	Success           bool
	HealthSample      bool
	HealthScope       string
	FirstTokenMs      *int
	DurationMs        int64
	TerminalReason    string
}
