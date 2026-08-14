package service

import (
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const adaptiveSchedulerStateVersion = 2

type adaptiveObservationType string

const (
	adaptiveObservationHealthSuccess    adaptiveObservationType = "health_success"
	adaptiveObservationAccountFailure   adaptiveObservationType = "account_failure"
	adaptiveObservationCapacityLimit    adaptiveObservationType = "capacity_limit"
	adaptiveObservationQuotaLimit       adaptiveObservationType = "quota_limit"
	adaptiveObservationProviderOverload adaptiveObservationType = "provider_overload"
	adaptiveObservationIgnored          adaptiveObservationType = "ignored"
)

type adaptiveLearningStatus string

const (
	adaptiveLearningUnlearned     adaptiveLearningStatus = "unlearned"
	adaptiveLearningLearning      adaptiveLearningStatus = "learning"
	adaptiveLearningLearned       adaptiveLearningStatus = "learned"
	adaptiveLearningNotApplicable adaptiveLearningStatus = "not_applicable"
)

type adaptiveRuntimeStatus string

const (
	adaptiveRuntimeHealthy      adaptiveRuntimeStatus = "healthy"
	adaptiveRuntimeHighError    adaptiveRuntimeStatus = "high_error"
	adaptiveRuntimeCooldown     adaptiveRuntimeStatus = "cooldown"
	adaptiveRuntimeHalfOpen     adaptiveRuntimeStatus = "half_open"
	adaptiveRuntimeQuotaLimited adaptiveRuntimeStatus = "quota_limited"
	adaptiveRuntimeSaturated    adaptiveRuntimeStatus = "saturated"
	adaptiveRuntimeUnavailable  adaptiveRuntimeStatus = "unavailable"
)

type adaptiveCoreSettings struct {
	Mode                      string
	TopK                      int
	SoftmaxTemperature        float64
	ExplorationRate           float64
	LearningWindow            time.Duration
	LearningMinHealthSamples  int
	SuccessEMAAlpha           float64
	TTFTEMAAlpha              float64
	ConsecutiveFailurePenalty float64
	HealthFailureThreshold    int
	CircuitCooldownInitial    time.Duration
	CircuitCooldownMaximum    time.Duration
	HealthProbeLease          time.Duration
	HighErrorMinSamples       int
	HighErrorMaxSamples       int
	HighErrorEnterRate        float64
	HighErrorExitRate         float64
	CapacityShrinkFactor      float64
	CapacityRecoveryFactor    float64
	CapacityRecoverySamples   int
	CapacityRecoveryLoad      float64
	CapacityCooldown          time.Duration
	QuotaProbeInterval        time.Duration
	WeightReliability         float64
	WeightCapacity            float64
	WeightTTFT                float64
	WeightCost                float64
}

func defaultAdaptiveCoreSettings() adaptiveCoreSettings {
	return adaptiveCoreSettings{
		Mode:                      "shadow",
		TopK:                      8,
		SoftmaxTemperature:        0.35,
		ExplorationRate:           0.02,
		LearningWindow:            20 * time.Minute,
		LearningMinHealthSamples:  30,
		SuccessEMAAlpha:           0.05,
		TTFTEMAAlpha:              0.05,
		ConsecutiveFailurePenalty: 0.25,
		HealthFailureThreshold:    3,
		CircuitCooldownInitial:    60 * time.Second,
		CircuitCooldownMaximum:    600 * time.Second,
		HealthProbeLease:          30 * time.Second,
		HighErrorMinSamples:       10,
		HighErrorMaxSamples:       100,
		HighErrorEnterRate:        0.25,
		HighErrorExitRate:         0.15,
		CapacityShrinkFactor:      0.90,
		CapacityRecoveryFactor:    1.15,
		CapacityRecoverySamples:   30,
		CapacityRecoveryLoad:      0.80,
		CapacityCooldown:          60 * time.Second,
		QuotaProbeInterval:        5 * time.Minute,
		WeightReliability:         0.50,
		WeightCapacity:            0.20,
		WeightTTFT:                0.15,
		WeightCost:                0.15,
	}
}

func normalizeAdaptiveCoreSettings(settings adaptiveCoreSettings) adaptiveCoreSettings {
	defaults := defaultAdaptiveCoreSettings()
	if settings.Mode != "enforce" {
		settings.Mode = "shadow"
	}
	settings.TopK = clampInt(settings.TopK, 1, 100, defaults.TopK)
	settings.SoftmaxTemperature = clampFloat(settings.SoftmaxTemperature, 0.01, 10, defaults.SoftmaxTemperature)
	settings.ExplorationRate = clampFloat(settings.ExplorationRate, 0, 1, defaults.ExplorationRate)
	if settings.LearningWindow <= 0 {
		settings.LearningWindow = defaults.LearningWindow
	}
	settings.LearningMinHealthSamples = clampIntMin(settings.LearningMinHealthSamples, 1, defaults.LearningMinHealthSamples)
	settings.SuccessEMAAlpha = clampFloat(settings.SuccessEMAAlpha, 0.0001, 1, defaults.SuccessEMAAlpha)
	settings.TTFTEMAAlpha = clampFloat(settings.TTFTEMAAlpha, 0.0001, 1, defaults.TTFTEMAAlpha)
	settings.ConsecutiveFailurePenalty = nonNegativeFinite(settings.ConsecutiveFailurePenalty)
	settings.HealthFailureThreshold = clampIntMin(settings.HealthFailureThreshold, 1, defaults.HealthFailureThreshold)
	if settings.CircuitCooldownInitial <= 0 {
		settings.CircuitCooldownInitial = defaults.CircuitCooldownInitial
	}
	if settings.CircuitCooldownMaximum < settings.CircuitCooldownInitial {
		settings.CircuitCooldownMaximum = settings.CircuitCooldownInitial
		if defaults.CircuitCooldownMaximum > settings.CircuitCooldownMaximum {
			settings.CircuitCooldownMaximum = defaults.CircuitCooldownMaximum
		}
	}
	if settings.HealthProbeLease <= 0 {
		settings.HealthProbeLease = defaults.HealthProbeLease
	}
	settings.HighErrorMinSamples = clampIntMin(settings.HighErrorMinSamples, 1, defaults.HighErrorMinSamples)
	settings.HighErrorMaxSamples = clampIntMin(settings.HighErrorMaxSamples, settings.HighErrorMinSamples, defaults.HighErrorMaxSamples)
	settings.HighErrorEnterRate = clampFloat(settings.HighErrorEnterRate, 0, 1, defaults.HighErrorEnterRate)
	settings.HighErrorExitRate = clampFloat(settings.HighErrorExitRate, 0, settings.HighErrorEnterRate, defaults.HighErrorExitRate)
	settings.CapacityShrinkFactor = clampFloat(settings.CapacityShrinkFactor, 0.01, 0.99, defaults.CapacityShrinkFactor)
	settings.CapacityRecoveryFactor = clampFloat(settings.CapacityRecoveryFactor, 1.01, 10, defaults.CapacityRecoveryFactor)
	settings.CapacityRecoverySamples = clampIntMin(settings.CapacityRecoverySamples, 1, defaults.CapacityRecoverySamples)
	settings.CapacityRecoveryLoad = clampFloat(settings.CapacityRecoveryLoad, 0, 1, defaults.CapacityRecoveryLoad)
	if settings.CapacityCooldown <= 0 {
		settings.CapacityCooldown = defaults.CapacityCooldown
	}
	if settings.QuotaProbeInterval <= 0 {
		settings.QuotaProbeInterval = defaults.QuotaProbeInterval
	}
	settings.WeightReliability = nonNegativeFinite(settings.WeightReliability)
	settings.WeightCapacity = nonNegativeFinite(settings.WeightCapacity)
	settings.WeightTTFT = nonNegativeFinite(settings.WeightTTFT)
	settings.WeightCost = nonNegativeFinite(settings.WeightCost)
	if settings.WeightReliability+settings.WeightCapacity+settings.WeightTTFT+settings.WeightCost <= 0 {
		settings.WeightReliability = defaults.WeightReliability
		settings.WeightCapacity = defaults.WeightCapacity
		settings.WeightTTFT = defaults.WeightTTFT
		settings.WeightCost = defaults.WeightCost
	}
	return settings
}

type adaptiveHealthObservation struct {
	At      time.Time `json:"at"`
	Success bool      `json:"success"`
}

type adaptiveAdmission struct {
	AccountID          int64
	CapacityGeneration uint64
	HealthProbe        bool
	QuotaProbe         bool
	ClaimedAt          time.Time
}

type adaptiveObservation struct {
	AccountID           int64
	RequestID           string
	Type                adaptiveObservationType
	ReasonCode          string
	Reason              string
	Authentication      bool
	FirstTokenMs        *int
	ConfiguredCapacity  int
	ObservedConcurrency int
	CapacityGeneration  uint64
	QuotaResetAt        *time.Time
	HealthProbe         bool
	Synthetic           bool
}

type adaptiveAccountState struct {
	Version                   int                         `json:"version"`
	AccountID                 int64                       `json:"account_id"`
	ConfiguredCapacity        int                         `json:"configured_capacity"`
	EffectiveCapacity         int                         `json:"effective_capacity"`
	SuccessEMA                float64                     `json:"success_ema"`
	TTFTEMA                   float64                     `json:"ttft_ema"`
	TTFTSamples               int64                       `json:"ttft_samples"`
	ConsecutiveFailures       int                         `json:"consecutive_failures"`
	HealthObservations        []adaptiveHealthObservation `json:"health_observations"`
	HighError                 bool                        `json:"high_error"`
	CircuitOpenUntil          time.Time                   `json:"circuit_open_until,omitempty"`
	CircuitOpenCount          int                         `json:"circuit_open_count"`
	HealthProbeInFlight       bool                        `json:"-"`
	HealthProbeUntil          time.Time                   `json:"-"`
	HealthProbeOwner          string                      `json:"-"`
	CapacityGeneration        uint64                      `json:"capacity_generation"`
	CapacityCooldownUntil     time.Time                   `json:"capacity_cooldown_until,omitempty"`
	CapacityHalfOpen          bool                        `json:"capacity_half_open"`
	CapacityRecoverySuccesses int                         `json:"capacity_recovery_successes"`
	CapacityLimitedGeneration bool                        `json:"capacity_limited_generation"`
	LastCapacityShrinkAt      time.Time                   `json:"last_capacity_shrink_at,omitempty"`
	LastObservedConcurrency   int                         `json:"last_observed_concurrency"`
	QuotaLimited              bool                        `json:"quota_limited"`
	QuotaResetAt              time.Time                   `json:"quota_reset_at,omitempty"`
	QuotaNextProbeAt          time.Time                   `json:"quota_next_probe_at,omitempty"`
	QuotaProbeInFlight        bool                        `json:"-"`
	QuotaProbeOwner           string                      `json:"-"`
	LastObservationType       adaptiveObservationType     `json:"last_observation_type"`
	LastReasonCode            string                      `json:"last_reason_code"`
	LastReason                string                      `json:"last_reason"`
	LastSuccessAt             time.Time                   `json:"last_success_at,omitempty"`
	LastFailureAt             time.Time                   `json:"last_failure_at,omitempty"`
	UpdatedAt                 time.Time                   `json:"updated_at"`
	revision                  uint64
	persistedRevision         uint64
}

type adaptiveStateStore struct {
	mu                   sync.RWMutex
	accounts             map[int64]*adaptiveAccountState
	failureClaims        map[string]time.Time
	admissions           map[string]adaptiveAdmission
	lastTransientCleanup time.Time
}

func newAdaptiveStateStore() *adaptiveStateStore {
	return &adaptiveStateStore{
		accounts:      make(map[int64]*adaptiveAccountState),
		failureClaims: make(map[string]time.Time),
		admissions:    make(map[string]adaptiveAdmission),
	}
}

func newAdaptiveAccountState(accountID int64, configuredCapacity int, now time.Time) *adaptiveAccountState {
	if configuredCapacity < 0 {
		configuredCapacity = 0
	}
	return &adaptiveAccountState{
		Version:            adaptiveSchedulerStateVersion,
		AccountID:          accountID,
		ConfiguredCapacity: configuredCapacity,
		EffectiveCapacity:  configuredCapacity,
		SuccessEMA:         0.5,
		CapacityGeneration: 1,
		UpdatedAt:          now,
	}
}

func cloneAdaptiveAccountState(state *adaptiveAccountState) adaptiveAccountState {
	if state == nil {
		return adaptiveAccountState{}
	}
	clone := *state
	clone.HealthObservations = append([]adaptiveHealthObservation(nil), state.HealthObservations...)
	return clone
}

func (s *adaptiveStateStore) ensureLocked(accountID int64, configuredCapacity int, now time.Time) *adaptiveAccountState {
	state := s.accounts[accountID]
	if state == nil {
		state = newAdaptiveAccountState(accountID, configuredCapacity, now)
		s.accounts[accountID] = state
	}
	if configuredCapacity < 0 {
		configuredCapacity = 0
	}
	state.ConfiguredCapacity = configuredCapacity
	if configuredCapacity == 0 {
		state.EffectiveCapacity = 0
	} else if state.EffectiveCapacity <= 0 || state.EffectiveCapacity > configuredCapacity {
		state.EffectiveCapacity = configuredCapacity
	}
	return state
}

func (s *adaptiveStateStore) snapshot(accountID int64, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) adaptiveAccountState {
	if s == nil || accountID <= 0 {
		return adaptiveAccountState{}
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(accountID, configuredCapacity, now)
	changed := s.refreshLocked(state, now, settings)
	if changed {
		touchAdaptiveState(state, now)
	}
	return cloneAdaptiveAccountState(state)
}

func (s *adaptiveStateStore) observeLoad(accountID int64, configuredCapacity, currentConcurrency int, now time.Time, settings adaptiveCoreSettings) adaptiveAccountState {
	if s == nil || accountID <= 0 {
		return adaptiveAccountState{}
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(accountID, configuredCapacity, now)
	changed := s.refreshLocked(state, now, settings)
	if currentConcurrency < 0 {
		currentConcurrency = 0
	}
	if state.LastObservedConcurrency != currentConcurrency {
		state.LastObservedConcurrency = currentConcurrency
		changed = true
	}
	if changed {
		touchAdaptiveState(state, now)
	}
	return cloneAdaptiveAccountState(state)
}

func (s *adaptiveStateStore) effectiveCapacity(accountID int64, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) int {
	return s.snapshot(accountID, configuredCapacity, now, settings).EffectiveCapacity
}

func (s *adaptiveStateStore) refreshLocked(state *adaptiveAccountState, now time.Time, settings adaptiveCoreSettings) bool {
	changed := pruneAdaptiveHealthWindow(state, now, settings)
	if !state.CapacityCooldownUntil.IsZero() && !state.CapacityCooldownUntil.After(now) && state.EffectiveCapacity < state.ConfiguredCapacity && !state.CapacityHalfOpen {
		state.CapacityHalfOpen = true
		state.CapacityRecoverySuccesses = 0
		changed = true
	}
	if state.HealthProbeInFlight && !state.HealthProbeUntil.After(now) {
		state.HealthProbeInFlight = false
		state.HealthProbeOwner = ""
		changed = true
	}
	if state.QuotaLimited && adaptiveQuotaProbeDue(state, now) && state.QuotaProbeInFlight {
		state.QuotaProbeInFlight = false
		state.QuotaProbeOwner = ""
		changed = true
	}
	return changed
}

func pruneAdaptiveHealthWindow(state *adaptiveAccountState, now time.Time, settings adaptiveCoreSettings) bool {
	cutoff := now.Add(-settings.LearningWindow)
	observations := state.HealthObservations
	first := 0
	for first < len(observations) && observations[first].At.Before(cutoff) {
		first++
	}
	changed := first > 0
	if first > 0 {
		state.HealthObservations = append([]adaptiveHealthObservation(nil), observations[first:]...)
	}
	if len(state.HealthObservations) > settings.HighErrorMaxSamples {
		state.HealthObservations = append([]adaptiveHealthObservation(nil), state.HealthObservations[len(state.HealthObservations)-settings.HighErrorMaxSamples:]...)
		changed = true
	}
	previous := state.HighError
	if len(state.HealthObservations) >= settings.HighErrorMinSamples {
		failures := 0
		for _, observation := range state.HealthObservations {
			if !observation.Success {
				failures++
			}
		}
		rate := float64(failures) / float64(len(state.HealthObservations))
		if state.HighError {
			state.HighError = rate > settings.HighErrorExitRate
		} else {
			state.HighError = rate >= settings.HighErrorEnterRate
		}
	} else {
		state.HighError = false
	}
	return changed || previous != state.HighError
}

func (s *adaptiveStateStore) registerAdmission(accountID int64, requestID string, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) uint64 {
	if s == nil || accountID <= 0 {
		return 0
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupTransientsLocked(now, settings)
	state := s.ensureLocked(accountID, configuredCapacity, now)
	s.refreshLocked(state, now, settings)
	generation := state.CapacityGeneration
	if requestID != "" {
		healthProbe := state.HealthProbeInFlight && (state.HealthProbeOwner == "" || state.HealthProbeOwner == requestID)
		quotaProbe := state.QuotaProbeInFlight && (state.QuotaProbeOwner == "" || state.QuotaProbeOwner == requestID)
		s.admissions[adaptiveTransientKey(accountID, requestID)] = adaptiveAdmission{
			AccountID:          accountID,
			CapacityGeneration: generation,
			HealthProbe:        healthProbe,
			QuotaProbe:         quotaProbe,
			ClaimedAt:          now,
		}
	}
	return generation
}

func (s *adaptiveStateStore) claimHealthProbe(accountID int64, requestID string, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupTransientsLocked(now, settings)
	state := s.ensureLocked(accountID, configuredCapacity, now)
	s.refreshLocked(state, now, settings)
	if state.CircuitOpenUntil.IsZero() {
		return true
	}
	if state.CircuitOpenUntil.After(now) || state.HealthProbeInFlight {
		return false
	}
	state.HealthProbeInFlight = true
	state.HealthProbeUntil = now.Add(settings.HealthProbeLease)
	state.HealthProbeOwner = requestID
	touchAdaptiveState(state, now)
	return true
}

func (s *adaptiveStateStore) claimQuotaProbe(accountID int64, requestID string, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) (allowed, claimed bool) {
	if s == nil || accountID <= 0 {
		return false, false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(accountID, configuredCapacity, now)
	s.refreshLocked(state, now, settings)
	if !state.QuotaLimited {
		return true, false
	}
	if !adaptiveQuotaProbeDue(state, now) || state.QuotaProbeInFlight {
		return false, false
	}
	state.QuotaProbeInFlight = true
	state.QuotaProbeOwner = requestID
	state.QuotaNextProbeAt = now.Add(settings.QuotaProbeInterval)
	touchAdaptiveState(state, now)
	return true, true
}

func (s *adaptiveStateStore) releaseQuotaProbe(accountID int64, requestID string, now time.Time) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.accounts[accountID]
	if state == nil || !state.QuotaProbeInFlight || state.QuotaProbeOwner != requestID {
		return
	}
	state.QuotaProbeInFlight = false
	state.QuotaProbeOwner = ""
	touchAdaptiveState(state, now)
}

func (s *adaptiveStateStore) releaseHealthProbe(accountID int64, requestID string, now time.Time) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.accounts[accountID]
	if state == nil || !state.HealthProbeInFlight || state.HealthProbeOwner != requestID {
		return
	}
	state.HealthProbeInFlight = false
	state.HealthProbeUntil = time.Time{}
	state.HealthProbeOwner = ""
	touchAdaptiveState(state, now)
}

func (s *adaptiveStateStore) allowedForSelection(accountID int64, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) bool {
	state := s.snapshot(accountID, configuredCapacity, now, settings)
	if !state.CircuitOpenUntil.IsZero() && (state.CircuitOpenUntil.After(now) || state.HealthProbeInFlight) {
		return false
	}
	if state.QuotaLimited && (!adaptiveQuotaProbeDue(&state, now) || state.QuotaProbeInFlight) {
		return false
	}
	return true
}

func adaptiveQuotaProbeDue(state *adaptiveAccountState, now time.Time) bool {
	if state == nil || !state.QuotaLimited {
		return false
	}
	probeAt := state.QuotaNextProbeAt
	if !state.QuotaResetAt.IsZero() && (probeAt.IsZero() || state.QuotaResetAt.Before(probeAt)) {
		probeAt = state.QuotaResetAt
	}
	return probeAt.IsZero() || !probeAt.After(now)
}

func (s *adaptiveStateStore) observe(observation adaptiveObservation, now time.Time, settings adaptiveCoreSettings) (capacityIncreased, capacityDecreased bool) {
	if s == nil || observation.AccountID <= 0 {
		return false, false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupTransientsLocked(now, settings)
	state := s.ensureLocked(observation.AccountID, observation.ConfiguredCapacity, now)
	s.refreshLocked(state, now, settings)
	quotaProbe := false
	if admission, ok := s.admissions[adaptiveTransientKey(observation.AccountID, observation.RequestID)]; ok {
		if observation.CapacityGeneration == 0 {
			observation.CapacityGeneration = admission.CapacityGeneration
		}
		quotaProbe = admission.QuotaProbe
		observation.HealthProbe = admission.HealthProbe
		delete(s.admissions, adaptiveTransientKey(observation.AccountID, observation.RequestID))
	}
	if quotaProbe && observation.Type != adaptiveObservationHealthSuccess && observation.Type != adaptiveObservationQuotaLimit {
		observation.Type = adaptiveObservationIgnored
	}
	state.LastObservationType = observation.Type
	state.LastReasonCode = observation.ReasonCode
	state.LastReason = observation.Reason
	if observation.ObservedConcurrency >= 0 {
		state.LastObservedConcurrency = observation.ObservedConcurrency
	}

	switch observation.Type {
	case adaptiveObservationHealthSuccess:
		s.observeHealthLocked(state, true, observation, now, settings)
		if quotaProbe && state.QuotaLimited {
			state.QuotaLimited = false
			state.QuotaResetAt = time.Time{}
			state.QuotaNextProbeAt = time.Time{}
			state.QuotaProbeInFlight = false
			state.QuotaProbeOwner = ""
		}
		capacityIncreased = s.observeCapacityRecoveryLocked(state, observation, now, settings)
	case adaptiveObservationAccountFailure:
		if s.claimFailureLocked(observation.AccountID, observation.RequestID, now, settings) {
			s.observeHealthLocked(state, false, observation, now, settings)
		}
	case adaptiveObservationCapacityLimit:
		capacityDecreased = s.observeCapacityLimitLocked(state, observation, now, settings)
	case adaptiveObservationQuotaLimit:
		state.QuotaLimited = true
		state.QuotaProbeInFlight = false
		state.QuotaProbeOwner = ""
		if observation.QuotaResetAt != nil && observation.QuotaResetAt.After(now) {
			state.QuotaResetAt = *observation.QuotaResetAt
			state.QuotaNextProbeAt = *observation.QuotaResetAt
		} else {
			state.QuotaResetAt = time.Time{}
			state.QuotaNextProbeAt = now.Add(settings.QuotaProbeInterval)
		}
	case adaptiveObservationProviderOverload, adaptiveObservationIgnored:
		// Provider-wide and request-local failures are diagnostic only.
	}
	touchAdaptiveState(state, now)
	return capacityIncreased, capacityDecreased
}

func (s *adaptiveStateStore) claimFailureLocked(accountID int64, requestID string, now time.Time, settings adaptiveCoreSettings) bool {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return true
	}
	key := adaptiveTransientKey(accountID, requestID)
	if _, exists := s.failureClaims[key]; exists {
		return false
	}
	s.failureClaims[key] = now
	return true
}

func (s *adaptiveStateStore) observeHealthLocked(state *adaptiveAccountState, success bool, observation adaptiveObservation, now time.Time, settings adaptiveCoreSettings) {
	state.HealthObservations = append(state.HealthObservations, adaptiveHealthObservation{At: now, Success: success})
	state.SuccessEMA = settings.SuccessEMAAlpha*boolFloat(success) + (1-settings.SuccessEMAAlpha)*state.SuccessEMA
	probeMatches := observation.HealthProbe || (state.HealthProbeInFlight && (state.HealthProbeOwner == "" || observation.RequestID == "" || state.HealthProbeOwner == observation.RequestID))
	if success {
		state.ConsecutiveFailures = 0
		state.LastSuccessAt = now
		if state.CircuitOpenUntil.IsZero() || probeMatches {
			state.CircuitOpenUntil = time.Time{}
			state.CircuitOpenCount = 0
			state.HealthProbeInFlight = false
			state.HealthProbeUntil = time.Time{}
			state.HealthProbeOwner = ""
		}
		if observation.FirstTokenMs != nil {
			value := *observation.FirstTokenMs
			if value < 1 {
				value = 1
			}
			if value > 120000 {
				value = 120000
			}
			if state.TTFTSamples == 0 {
				state.TTFTEMA = float64(value)
			} else {
				state.TTFTEMA = settings.TTFTEMAAlpha*float64(value) + (1-settings.TTFTEMAAlpha)*state.TTFTEMA
			}
			state.TTFTSamples++
		}
	} else {
		state.ConsecutiveFailures++
		state.LastFailureAt = now
		thresholdReached := state.ConsecutiveFailures >= settings.HealthFailureThreshold
		circuitActive := !state.CircuitOpenUntil.IsZero() && state.CircuitOpenUntil.After(now)
		firstOpen := state.CircuitOpenUntil.IsZero() && (observation.Authentication || thresholdReached)
		failedHalfOpenProbe := !state.CircuitOpenUntil.IsZero() && !circuitActive && probeMatches
		if firstOpen || failedHalfOpenProbe {
			state.CircuitOpenCount++
			cooldown := adaptiveCircuitCooldown(state.CircuitOpenCount, settings)
			state.CircuitOpenUntil = now.Add(cooldown)
			state.HealthProbeInFlight = false
			state.HealthProbeUntil = time.Time{}
			state.HealthProbeOwner = ""
		}
	}
	pruneAdaptiveHealthWindow(state, now, settings)
}

func adaptiveCircuitCooldown(openCount int, settings adaptiveCoreSettings) time.Duration {
	duration := settings.CircuitCooldownInitial
	for i := 1; i < openCount && duration < settings.CircuitCooldownMaximum; i++ {
		if duration > settings.CircuitCooldownMaximum/2 {
			return settings.CircuitCooldownMaximum
		}
		duration *= 2
	}
	if duration > settings.CircuitCooldownMaximum {
		return settings.CircuitCooldownMaximum
	}
	return duration
}

func (s *adaptiveStateStore) observeCapacityLimitLocked(state *adaptiveAccountState, observation adaptiveObservation, now time.Time, settings adaptiveCoreSettings) bool {
	if state.CapacityCooldownUntil.After(now) || state.EffectiveCapacity <= 1 {
		return false
	}
	if observation.CapacityGeneration != 0 && observation.CapacityGeneration != state.CapacityGeneration {
		return false
	}
	state.CapacityLimitedGeneration = true
	state.CapacityRecoverySuccesses = 0
	observed := observation.ObservedConcurrency
	if observed <= 0 {
		observed = state.LastObservedConcurrency
	}
	if observed <= 0 {
		observed = state.EffectiveCapacity
	}
	byOld := int(math.Floor(float64(state.EffectiveCapacity) * settings.CapacityShrinkFactor))
	byObserved := int(math.Floor(float64(observed) * settings.CapacityShrinkFactor))
	next := min(byOld, byObserved)
	if next < 1 {
		next = 1
	}
	if next >= state.EffectiveCapacity {
		return false
	}
	state.EffectiveCapacity = next
	state.CapacityGeneration++
	state.CapacityCooldownUntil = now.Add(settings.CapacityCooldown)
	state.CapacityHalfOpen = false
	state.CapacityLimitedGeneration = false
	state.LastCapacityShrinkAt = now
	return true
}

func (s *adaptiveStateStore) observeCapacityRecoveryLocked(state *adaptiveAccountState, observation adaptiveObservation, now time.Time, settings adaptiveCoreSettings) bool {
	if !state.CapacityHalfOpen || state.EffectiveCapacity >= state.ConfiguredCapacity || state.CapacityLimitedGeneration {
		return false
	}
	if observation.CapacityGeneration != state.CapacityGeneration {
		return false
	}
	observed := observation.ObservedConcurrency
	if observed <= 0 {
		observed = state.LastObservedConcurrency
	}
	if state.EffectiveCapacity > 0 && float64(observed)/float64(state.EffectiveCapacity) < settings.CapacityRecoveryLoad {
		return false
	}
	state.CapacityRecoverySuccesses++
	if state.CapacityRecoverySuccesses < settings.CapacityRecoverySamples {
		return false
	}
	next := int(math.Ceil(float64(state.EffectiveCapacity) * settings.CapacityRecoveryFactor))
	if next <= state.EffectiveCapacity {
		next = state.EffectiveCapacity + 1
	}
	if next > state.ConfiguredCapacity {
		next = state.ConfiguredCapacity
	}
	state.EffectiveCapacity = next
	state.CapacityRecoverySuccesses = 0
	state.CapacityHalfOpen = next < state.ConfiguredCapacity
	return true
}

func (s *adaptiveStateStore) cleanupTransientsLocked(now time.Time, settings adaptiveCoreSettings) {
	if !s.lastTransientCleanup.IsZero() && now.Sub(s.lastTransientCleanup) < time.Minute {
		return
	}
	retention := settings.LearningWindow
	if retention < 10*time.Minute {
		retention = 10 * time.Minute
	}
	for key, claimedAt := range s.failureClaims {
		if now.Sub(claimedAt) > retention {
			delete(s.failureClaims, key)
		}
	}
	for key, admission := range s.admissions {
		if now.Sub(admission.ClaimedAt) > retention {
			delete(s.admissions, key)
		}
	}
	s.lastTransientCleanup = now
}

func adaptiveTransientKey(accountID int64, requestID string) string {
	return strconv.FormatInt(accountID, 10) + ":" + strings.TrimSpace(requestID)
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func touchAdaptiveState(state *adaptiveAccountState, now time.Time) {
	state.Version = adaptiveSchedulerStateVersion
	state.UpdatedAt = now
	state.revision++
}

func adaptiveLearningState(state adaptiveAccountState, oauth bool, now time.Time, settings adaptiveCoreSettings) (adaptiveLearningStatus, int) {
	if oauth {
		return adaptiveLearningNotApplicable, 0
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	cutoff := now.Add(-settings.LearningWindow)
	count := 0
	for _, observation := range state.HealthObservations {
		if !observation.At.Before(cutoff) {
			count++
		}
	}
	if count == 0 {
		return adaptiveLearningUnlearned, 0
	}
	if count < settings.LearningMinHealthSamples {
		return adaptiveLearningLearning, count
	}
	return adaptiveLearningLearned, count
}

func adaptiveRuntimeState(state adaptiveAccountState, accountAvailable bool, currentConcurrency int, now time.Time) (adaptiveRuntimeStatus, []adaptiveRuntimeStatus, string, string) {
	flags := make([]adaptiveRuntimeStatus, 0, 5)
	if !accountAvailable {
		flags = append(flags, adaptiveRuntimeUnavailable)
	}
	if !state.CircuitOpenUntil.IsZero() {
		if state.CircuitOpenUntil.After(now) {
			flags = append(flags, adaptiveRuntimeCooldown)
		} else {
			flags = append(flags, adaptiveRuntimeHalfOpen)
		}
	}
	if state.QuotaLimited {
		flags = append(flags, adaptiveRuntimeQuotaLimited)
	}
	if state.CapacityHalfOpen {
		flags = appendUniqueAdaptiveRuntimeFlag(flags, adaptiveRuntimeHalfOpen)
	}
	if state.EffectiveCapacity > 0 && currentConcurrency >= state.EffectiveCapacity {
		flags = append(flags, adaptiveRuntimeSaturated)
	}
	if state.HighError {
		flags = append(flags, adaptiveRuntimeHighError)
	}
	if len(flags) == 0 {
		flags = append(flags, adaptiveRuntimeHealthy)
	}
	priority := []adaptiveRuntimeStatus{adaptiveRuntimeUnavailable, adaptiveRuntimeCooldown, adaptiveRuntimeHalfOpen, adaptiveRuntimeQuotaLimited, adaptiveRuntimeSaturated, adaptiveRuntimeHighError, adaptiveRuntimeHealthy}
	main := adaptiveRuntimeHealthy
	for _, candidate := range priority {
		for _, flag := range flags {
			if flag == candidate {
				main = candidate
				return main, flags, state.LastReasonCode, state.LastReason
			}
		}
	}
	return main, flags, state.LastReasonCode, state.LastReason
}

func appendUniqueAdaptiveRuntimeFlag(flags []adaptiveRuntimeStatus, flag adaptiveRuntimeStatus) []adaptiveRuntimeStatus {
	for _, existing := range flags {
		if existing == flag {
			return flags
		}
	}
	return append(flags, flag)
}

type adaptiveScoreCandidate struct {
	AccountID          int64
	OAuth              bool
	Cost               float64
	CurrentConcurrency int
	State              adaptiveAccountState
	ReliabilityScore   float64
	CapacityScore      float64
	TTFTScore          float64
	CostScore          float64
	Score              float64
	LearningStatus     adaptiveLearningStatus
	HealthSamples      int
}

func scoreAdaptiveCandidates(candidates []adaptiveScoreCandidate, now time.Time, settings adaptiveCoreSettings) []adaptiveScoreCandidate {
	settings = normalizeAdaptiveCoreSettings(settings)
	if len(candidates) == 0 {
		return nil
	}
	hasOAuth := false
	for _, candidate := range candidates {
		if candidate.OAuth {
			hasOAuth = true
			break
		}
	}
	if hasOAuth {
		oauth := make([]adaptiveScoreCandidate, 0, len(candidates))
		nonOAuth := make([]adaptiveScoreCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.OAuth {
				oauth = append(oauth, candidate)
			} else {
				nonOAuth = append(nonOAuth, candidate)
			}
		}
		oauth = scoreAdaptiveCandidateLayer(oauth, now, settings)
		nonOAuth = scoreAdaptiveCandidateLayer(nonOAuth, now, settings)
		byID := make(map[int64]adaptiveScoreCandidate, len(candidates))
		for _, candidate := range append(oauth, nonOAuth...) {
			byID[candidate.AccountID] = candidate
		}
		for i := range candidates {
			candidates[i] = byID[candidates[i].AccountID]
		}
		return candidates
	}
	return scoreAdaptiveCandidateLayer(candidates, now, settings)
}

func scoreAdaptiveCandidateLayer(candidates []adaptiveScoreCandidate, now time.Time, settings adaptiveCoreSettings) []adaptiveScoreCandidate {
	if len(candidates) == 0 {
		return nil
	}
	minimumCost := math.Inf(1)
	ttfts := make([]float64, 0, len(candidates))
	for i := range candidates {
		if candidates[i].Cost < 0 || math.IsNaN(candidates[i].Cost) || math.IsInf(candidates[i].Cost, 0) {
			candidates[i].Cost = 1
		}
		minimumCost = math.Min(minimumCost, candidates[i].Cost)
		if candidates[i].State.TTFTSamples > 0 && candidates[i].State.TTFTEMA > 0 {
			ttfts = append(ttfts, candidates[i].State.TTFTEMA)
		}
	}
	if math.IsInf(minimumCost, 1) {
		minimumCost = 1
	}
	sort.Float64s(ttfts)
	ttftEnabled := len(ttfts) >= 2
	medianTTFT := 0.0
	minTTFT, maxTTFT := 0.0, 0.0
	if ttftEnabled {
		medianTTFT = ttfts[len(ttfts)/2]
		if len(ttfts)%2 == 0 {
			medianTTFT = (ttfts[len(ttfts)/2-1] + medianTTFT) / 2
		}
		minTTFT, maxTTFT = ttfts[0], ttfts[len(ttfts)-1]
	}
	weightSum := settings.WeightReliability + settings.WeightCapacity + settings.WeightCost
	if ttftEnabled {
		weightSum += settings.WeightTTFT
	}
	if weightSum <= 0 {
		weightSum = 1
	}
	for i := range candidates {
		candidate := &candidates[i]
		candidate.LearningStatus, candidate.HealthSamples = adaptiveLearningState(candidate.State, candidate.OAuth, now, settings)
		confidence := math.Min(1, float64(candidate.HealthSamples)/float64(settings.LearningMinHealthSamples))
		candidate.ReliabilityScore = 0.5*(1-confidence) + candidate.State.SuccessEMA*confidence
		candidate.ReliabilityScore /= 1 + settings.ConsecutiveFailurePenalty*float64(candidate.State.ConsecutiveFailures)
		if candidate.State.EffectiveCapacity <= 0 {
			candidate.CapacityScore = 1
		} else {
			candidate.CapacityScore = clamp01(float64(candidate.State.EffectiveCapacity-candidate.CurrentConcurrency) / float64(candidate.State.EffectiveCapacity))
		}
		if candidate.Cost <= 0 {
			candidate.CostScore = 1
		} else if minimumCost <= 0 {
			candidate.CostScore = 0
		} else {
			candidate.CostScore = clamp01(minimumCost / candidate.Cost)
		}
		if ttftEnabled {
			ttft := candidate.State.TTFTEMA
			if candidate.State.TTFTSamples == 0 || ttft <= 0 {
				ttft = medianTTFT
			}
			candidate.TTFTScore = 1 - normalizeAdaptiveValue(ttft, minTTFT, maxTTFT, 0.5)
		}
		candidate.Score = (settings.WeightReliability*candidate.ReliabilityScore +
			settings.WeightCapacity*candidate.CapacityScore +
			settings.WeightCost*candidate.CostScore +
			settings.WeightTTFT*candidate.TTFTScore*boolFloat(ttftEnabled)) / weightSum
	}
	return candidates
}

func orderAdaptiveCandidates(candidates []adaptiveScoreCandidate, newSession bool, shadow bool, now time.Time, settings adaptiveCoreSettings) []adaptiveScoreCandidate {
	settings = normalizeAdaptiveCoreSettings(settings)
	if len(candidates) <= 1 {
		return append([]adaptiveScoreCandidate(nil), candidates...)
	}
	oauth := make([]adaptiveScoreCandidate, 0, len(candidates))
	nonOAuth := make([]adaptiveScoreCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.OAuth {
			oauth = append(oauth, candidate)
		} else {
			nonOAuth = append(nonOAuth, candidate)
		}
	}
	active := nonOAuth
	deferred := oauth
	if len(oauth) > 0 {
		active = oauth
		deferred = nonOAuth
	}
	if newSession && !shadow && len(oauth) == 0 && settings.ExplorationRate > 0 && rand.Float64() < settings.ExplorationRate {
		if index := weightedAdaptiveExplorationIndex(active, settings); index >= 0 {
			selected := active[index]
			active = append(active[:index], active[index+1:]...)
			return append([]adaptiveScoreCandidate{selected}, append(scoreSoftmaxAdaptiveOrder(active, settings), deferred...)...)
		}
	}
	return append(scoreSoftmaxAdaptiveOrder(active, settings), scoreSoftmaxAdaptiveOrder(deferred, settings)...)
}

func weightedAdaptiveExplorationIndex(candidates []adaptiveScoreCandidate, settings adaptiveCoreSettings) int {
	weights := make([]float64, len(candidates))
	total := 0.0
	for i, candidate := range candidates {
		if candidate.OAuth || candidate.HealthSamples >= settings.LearningMinHealthSamples {
			continue
		}
		weights[i] = float64(settings.LearningMinHealthSamples - candidate.HealthSamples)
		total += weights[i]
	}
	if total <= 0 {
		return -1
	}
	pick := rand.Float64() * total
	for i, weight := range weights {
		pick -= weight
		if pick <= 0 && weight > 0 {
			return i
		}
	}
	return -1
}

func scoreSoftmaxAdaptiveOrder(candidates []adaptiveScoreCandidate, settings adaptiveCoreSettings) []adaptiveScoreCandidate {
	if len(candidates) <= 1 {
		return append([]adaptiveScoreCandidate(nil), candidates...)
	}
	ranked := append([]adaptiveScoreCandidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].AccountID < ranked[j].AccountID
	})
	topK := min(settings.TopK, len(ranked))
	pool := append([]adaptiveScoreCandidate(nil), ranked[:topK]...)
	order := make([]adaptiveScoreCandidate, 0, len(ranked))
	for len(pool) > 0 {
		maxScore := pool[0].Score
		for _, candidate := range pool[1:] {
			maxScore = math.Max(maxScore, candidate.Score)
		}
		weights := make([]float64, len(pool))
		total := 0.0
		for i, candidate := range pool {
			weight := math.Exp((candidate.Score - maxScore) / settings.SoftmaxTemperature)
			if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
				weight = 1
			}
			weights[i] = weight
			total += weight
		}
		pick := rand.Float64() * total
		selected := 0
		for i, weight := range weights {
			pick -= weight
			if pick <= 0 {
				selected = i
				break
			}
		}
		order = append(order, pool[selected])
		pool = append(pool[:selected], pool[selected+1:]...)
	}
	return append(order, ranked[topK:]...)
}

func classifyAdaptiveTerminalReason(success bool, terminalReason string) (adaptiveObservationType, bool) {
	reason := strings.ToLower(strings.TrimSpace(terminalReason))
	if success && (reason == "success" || reason == "legacy_result") {
		return adaptiveObservationHealthSuccess, false
	}
	switch reason {
	case "account_auth":
		return adaptiveObservationAccountFailure, true
	case "account_health_failure", "transport_error", "upstream_5xx", "generic_rate_limit", "stream_incomplete", "failure":
		return adaptiveObservationAccountFailure, false
	case "concurrency_limit":
		return adaptiveObservationCapacityLimit, false
	case "insufficient_balance", "quota_rate_limit", "window_rate_limit":
		return adaptiveObservationQuotaLimit, false
	case "provider_capacity", "provider_overloaded", "model_upstream_error":
		return adaptiveObservationProviderOverload, false
	default:
		return adaptiveObservationIgnored, false
	}
}
