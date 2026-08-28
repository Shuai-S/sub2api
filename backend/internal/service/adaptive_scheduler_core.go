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
	adaptiveRuntimeHealthy          adaptiveRuntimeStatus = "healthy"
	adaptiveRuntimeHighError        adaptiveRuntimeStatus = "high_error"
	adaptiveRuntimeCooldown         adaptiveRuntimeStatus = "cooldown"
	adaptiveRuntimeCircuitHalfOpen  adaptiveRuntimeStatus = "circuit_half_open"
	adaptiveRuntimeCapacityRecovery adaptiveRuntimeStatus = "capacity_recovery"
	// Kept for persisted/API compatibility with older snapshots. New runtime
	// flags use the two independent statuses above.
	adaptiveRuntimeHalfOpen     adaptiveRuntimeStatus = "half_open"
	adaptiveRuntimeQuotaLimited adaptiveRuntimeStatus = "quota_limited"
	adaptiveRuntimeSaturated    adaptiveRuntimeStatus = "saturated"
	adaptiveRuntimeUnavailable  adaptiveRuntimeStatus = "unavailable"
)

type adaptiveRecoveryStatus string

const (
	adaptiveRecoveryActive  adaptiveRecoveryStatus = "active"
	adaptiveRecoveryStale   adaptiveRecoveryStatus = "stale"
	adaptiveRecoveryProbing adaptiveRecoveryStatus = "probing"
	adaptiveRecoveryWarming adaptiveRecoveryStatus = "warming"
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
	CapacityCooldown          time.Duration
	QuotaProbeInterval        time.Duration
	RecoveryExplorationRate   float64
	RecoveryMaxConcurrency    int
	RecoveryWarmupSuccesses   int
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
		CapacityRecoveryFactor:    1.25,
		CapacityRecoverySamples:   8,
		CapacityCooldown:          60 * time.Second,
		QuotaProbeInterval:        5 * time.Minute,
		RecoveryExplorationRate:   0.01,
		RecoveryMaxConcurrency:    2,
		RecoveryWarmupSuccesses:   3,
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
	if settings.CapacityCooldown <= 0 {
		settings.CapacityCooldown = defaults.CapacityCooldown
	}
	if settings.QuotaProbeInterval <= 0 {
		settings.QuotaProbeInterval = defaults.QuotaProbeInterval
	}
	settings.RecoveryExplorationRate = clampFloat(settings.RecoveryExplorationRate, 0, 1, defaults.RecoveryExplorationRate)
	settings.RecoveryMaxConcurrency = clampIntMin(settings.RecoveryMaxConcurrency, 1, defaults.RecoveryMaxConcurrency)
	settings.RecoveryWarmupSuccesses = clampIntMin(settings.RecoveryWarmupSuccesses, 1, defaults.RecoveryWarmupSuccesses)
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

type adaptiveTTFTObservation struct {
	At           time.Time `json:"at"`
	Milliseconds int       `json:"milliseconds"`
}

const (
	adaptiveTTFTMaxSamplesPerBucket  = 512
	adaptiveTTFTMaxBucketsPerAccount = 32
)

type adaptiveAdmission struct {
	AccountID           int64
	CapacityGeneration  uint64
	TTFTBucketKey       string
	HealthProbe         bool
	QuotaProbe          bool
	RecoveryProbe       bool
	Admitted            bool
	ObservedConcurrency int
	WaitingCount        int
	ClaimedAt           time.Time
}

type adaptiveTTFTClaim struct {
	BucketKey string
	ClaimedAt time.Time
}

type adaptiveObservation struct {
	AccountID             int64
	RequestID             string
	Type                  adaptiveObservationType
	ReasonCode            string
	Reason                string
	Authentication        bool
	FirstTokenMs          *int
	TTFTBucketKey         string
	WindowedTTFT          bool
	ConfiguredCapacity    int
	ObservedConcurrency   int
	WaitingCount          int
	CapacityGeneration    uint64
	QuotaResetAt          *time.Time
	HealthProbe           bool
	RecoveryProbe         bool
	AccountHealthEligible *bool
	Synthetic             bool
}

type adaptiveAccountState struct {
	Version                   int                                  `json:"version"`
	AccountID                 int64                                `json:"account_id"`
	ConfiguredCapacity        int                                  `json:"configured_capacity"`
	EffectiveCapacity         int                                  `json:"effective_capacity"`
	SuccessEMA                float64                              `json:"success_ema"`
	TTFTEMA                   float64                              `json:"ttft_ema"`
	TTFTSamples               int64                                `json:"ttft_samples"`
	TTFTWindows               map[string][]adaptiveTTFTObservation `json:"ttft_windows,omitempty"`
	ConsecutiveFailures       int                                  `json:"consecutive_failures"`
	HealthObservations        []adaptiveHealthObservation          `json:"health_observations"`
	HighError                 bool                                 `json:"high_error"`
	CircuitOpenUntil          time.Time                            `json:"circuit_open_until,omitempty"`
	CircuitOpenCount          int                                  `json:"circuit_open_count"`
	HealthProbeInFlight       bool                                 `json:"-"`
	HealthProbeUntil          time.Time                            `json:"-"`
	HealthProbeOwner          string                               `json:"-"`
	CapacityGeneration        uint64                               `json:"capacity_generation"`
	CapacityCooldownUntil     time.Time                            `json:"capacity_cooldown_until,omitempty"`
	CapacityHalfOpen          bool                                 `json:"capacity_half_open"`
	CapacityRecoverySuccesses int                                  `json:"capacity_recovery_successes"`
	CapacityLimitedGeneration bool                                 `json:"capacity_limited_generation"`
	LastCapacityShrinkAt      time.Time                            `json:"last_capacity_shrink_at,omitempty"`
	LastObservedConcurrency   int                                  `json:"last_observed_concurrency"`
	QuotaLimited              bool                                 `json:"quota_limited"`
	QuotaResetAt              time.Time                            `json:"quota_reset_at,omitempty"`
	QuotaNextProbeAt          time.Time                            `json:"quota_next_probe_at,omitempty"`
	QuotaProbeInFlight        bool                                 `json:"-"`
	QuotaProbeOwner           string                               `json:"-"`
	LastDispatchAt            time.Time                            `json:"last_dispatch_at,omitempty"`
	LastProbeAt               time.Time                            `json:"last_probe_at,omitempty"`
	RecoveryStatus            adaptiveRecoveryStatus               `json:"recovery_status,omitempty"`
	RecoverySuccesses         int                                  `json:"recovery_successes"`
	RecoveryProbeInFlight     bool                                 `json:"-"`
	RecoveryProbeUntil        time.Time                            `json:"-"`
	RecoveryProbeOwner        string                               `json:"-"`
	LastObservationType       adaptiveObservationType              `json:"last_observation_type"`
	LastReasonCode            string                               `json:"last_reason_code"`
	LastReason                string                               `json:"last_reason"`
	LastSuccessAt             time.Time                            `json:"last_success_at,omitempty"`
	LastFailureAt             time.Time                            `json:"last_failure_at,omitempty"`
	UpdatedAt                 time.Time                            `json:"updated_at"`
	revision                  uint64
	persistedRevision         uint64
}

type adaptiveStateStore struct {
	mu            sync.RWMutex
	accounts      map[int64]*adaptiveAccountState
	failureClaims map[string]time.Time
	// Kept after an account lease is released so failover cannot spend the same request on another half-open account.
	healthProbeClaims    map[string]time.Time
	recoveryProbeClaims  map[string]time.Time
	admissions           map[string]adaptiveAdmission
	ttftClaims           map[string]adaptiveTTFTClaim
	lastTransientCleanup time.Time
}

func newAdaptiveStateStore() *adaptiveStateStore {
	return &adaptiveStateStore{
		accounts:            make(map[int64]*adaptiveAccountState),
		failureClaims:       make(map[string]time.Time),
		healthProbeClaims:   make(map[string]time.Time),
		recoveryProbeClaims: make(map[string]time.Time),
		admissions:          make(map[string]adaptiveAdmission),
		ttftClaims:          make(map[string]adaptiveTTFTClaim),
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
		RecoveryStatus:     adaptiveRecoveryActive,
		UpdatedAt:          now,
	}
}

func cloneAdaptiveAccountState(state *adaptiveAccountState) adaptiveAccountState {
	if state == nil {
		return adaptiveAccountState{}
	}
	clone := *state
	clone.HealthObservations = append([]adaptiveHealthObservation(nil), state.HealthObservations...)
	if len(state.TTFTWindows) > 0 {
		clone.TTFTWindows = make(map[string][]adaptiveTTFTObservation, len(state.TTFTWindows))
		for key, observations := range state.TTFTWindows {
			clone.TTFTWindows[key] = append([]adaptiveTTFTObservation(nil), observations...)
		}
	}
	return clone
}

func cloneAdaptiveAccountStateForScheduling(state *adaptiveAccountState, ttftBucketKey string) adaptiveAccountState {
	if state == nil {
		return adaptiveAccountState{}
	}
	clone := *state
	clone.HealthObservations = append([]adaptiveHealthObservation(nil), state.HealthObservations...)
	clone.TTFTWindows = nil
	if ttftBucketKey = strings.TrimSpace(ttftBucketKey); ttftBucketKey != "" {
		if observations := state.TTFTWindows[ttftBucketKey]; len(observations) > 0 {
			clone.TTFTWindows = map[string][]adaptiveTTFTObservation{
				ttftBucketKey: append([]adaptiveTTFTObservation(nil), observations...),
			}
		}
	}
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
	previousConfigured := state.ConfiguredCapacity
	previousEffective := state.EffectiveCapacity
	state.ConfiguredCapacity = configuredCapacity
	if configuredCapacity == 0 {
		state.EffectiveCapacity = 0
	} else if configuredCapacity > previousConfigured && state.EffectiveCapacity < configuredCapacity {
		// A configuration increase is an explicit operator signal. Do not make
		// the account wait for the sampled recovery loop to catch up.
		state.EffectiveCapacity = configuredCapacity
		state.CapacityRecoverySuccesses = 0
		state.CapacityHalfOpen = false
		state.CapacityLimitedGeneration = false
		state.CapacityCooldownUntil = time.Time{}
		state.CapacityGeneration++
	} else if state.EffectiveCapacity <= 0 || state.EffectiveCapacity > configuredCapacity {
		state.EffectiveCapacity = configuredCapacity
	}
	if previousConfigured != configuredCapacity || previousEffective != state.EffectiveCapacity {
		touchAdaptiveState(state, now)
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

func (s *adaptiveStateStore) schedulingSnapshot(
	accountID int64,
	configuredCapacity int,
	currentConcurrency int,
	ttftBucketKey string,
	now time.Time,
	settings adaptiveCoreSettings,
) (adaptiveAccountState, bool) {
	if s == nil || accountID <= 0 {
		return adaptiveAccountState{}, false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(accountID, configuredCapacity, now)
	changed := s.refreshLocked(state, now, settings)
	if currentConcurrency >= 0 && state.LastObservedConcurrency != currentConcurrency {
		state.LastObservedConcurrency = currentConcurrency
		changed = true
	}
	if changed {
		touchAdaptiveState(state, now)
	}
	return cloneAdaptiveAccountStateForScheduling(state, ttftBucketKey), adaptiveStateAllowedForSelection(state, now)
}

func (s *adaptiveStateStore) summarySnapshot(accountID int64, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) adaptiveAccountState {
	state, _ := s.schedulingSnapshot(accountID, configuredCapacity, -1, "", now, settings)
	return state
}

func (s *adaptiveStateStore) runtimeStatusWithLoad(accountID int64, configuredCapacity, currentConcurrency int, now time.Time, settings adaptiveCoreSettings) (effectiveCapacity int, allowed, circuitTracked, quotaTracked bool) {
	if s == nil || accountID <= 0 {
		return 0, false, false, false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(accountID, configuredCapacity, now)
	changed := s.refreshLocked(state, now, settings)
	if currentConcurrency >= 0 && state.LastObservedConcurrency != currentConcurrency {
		state.LastObservedConcurrency = currentConcurrency
		changed = true
	}
	if changed {
		touchAdaptiveState(state, now)
	}
	return state.EffectiveCapacity, adaptiveStateAllowedForSelection(state, now), !state.CircuitOpenUntil.IsZero(), state.QuotaLimited
}

func (s *adaptiveStateStore) runtimeStatus(accountID int64, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) (effectiveCapacity int, allowed, circuitTracked, quotaTracked bool) {
	return s.runtimeStatusWithLoad(accountID, configuredCapacity, -1, now, settings)
}

func (s *adaptiveStateStore) effectiveCapacity(accountID int64, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) int {
	effectiveCapacity, _, _, _ := s.runtimeStatus(accountID, configuredCapacity, now, settings)
	return effectiveCapacity
}

func (s *adaptiveStateStore) dueHealthProbeAccountIDs(now time.Time, settings adaptiveCoreSettings) []int64 {
	if s == nil {
		return nil
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	type dueProbe struct {
		accountID int64
		dueAt     time.Time
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupTransientsLocked(now, settings)
	due := make([]dueProbe, 0)
	for accountID, state := range s.accounts {
		if s.refreshLocked(state, now, settings) {
			touchAdaptiveState(state, now)
		}
		dueAt := time.Time{}
		if !state.CircuitOpenUntil.IsZero() && !state.CircuitOpenUntil.After(now) && !state.HealthProbeInFlight {
			dueAt = state.CircuitOpenUntil
		}
		if state.QuotaLimited && adaptiveQuotaProbeDue(state, now) && !state.QuotaProbeInFlight {
			quotaDueAt := adaptiveQuotaProbeAt(state)
			if quotaDueAt.IsZero() {
				quotaDueAt = now
			}
			if dueAt.IsZero() || (!quotaDueAt.IsZero() && quotaDueAt.Before(dueAt)) {
				dueAt = quotaDueAt
			}
		}
		if !dueAt.IsZero() {
			due = append(due, dueProbe{accountID: accountID, dueAt: dueAt})
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if !due[i].dueAt.Equal(due[j].dueAt) {
			return due[i].dueAt.Before(due[j].dueAt)
		}
		return due[i].accountID < due[j].accountID
	})
	ids := make([]int64, 0, len(due))
	for _, probe := range due {
		ids = append(ids, probe.accountID)
	}
	return ids
}

func (s *adaptiveStateStore) recoverHealth(accountID int64, now time.Time, settings adaptiveCoreSettings) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.accounts[accountID]
	if state == nil {
		return false
	}
	wasOpen := !state.CircuitOpenUntil.IsZero()
	s.observeHealthLocked(state, true, adaptiveObservation{
		AccountID:   accountID,
		HealthProbe: true,
		ReasonCode:  "successful_account_test",
	}, now, settings)
	state.LastObservationType = adaptiveObservationHealthSuccess
	state.LastReasonCode = "successful_account_test"
	state.LastReason = ""
	touchAdaptiveState(state, now)
	return wasOpen
}

func (s *adaptiveStateStore) refreshLocked(state *adaptiveAccountState, now time.Time, settings adaptiveCoreSettings) bool {
	changed := pruneAdaptiveHealthWindow(state, now, settings)
	changed = pruneAdaptiveTTFTWindows(state, now, settings) || changed
	if !state.CapacityCooldownUntil.IsZero() && !state.CapacityCooldownUntil.After(now) && state.EffectiveCapacity < state.ConfiguredCapacity && !state.CapacityHalfOpen {
		state.CapacityHalfOpen = true
		state.CapacityRecoverySuccesses = 0
		changed = true
	}
	if state.HealthProbeInFlight && !state.HealthProbeUntil.After(now) {
		state.HealthProbeInFlight = false
		state.HealthProbeUntil = time.Time{}
		state.HealthProbeOwner = ""
		changed = true
	}
	if state.QuotaLimited && adaptiveQuotaProbeDue(state, now) && state.QuotaProbeInFlight {
		state.QuotaProbeInFlight = false
		state.QuotaProbeOwner = ""
		changed = true
	}
	if state.RecoveryProbeInFlight && !state.RecoveryProbeUntil.After(now) {
		state.RecoveryProbeInFlight = false
		state.RecoveryProbeUntil = time.Time{}
		state.RecoveryProbeOwner = ""
		state.RecoveryStatus = adaptiveRecoveryStatusAfterProbe(state)
		changed = true
	}
	if !state.RecoveryProbeInFlight && state.RecoveryStatus == adaptiveRecoveryWarming &&
		!state.LastProbeAt.IsZero() && now.Sub(state.LastProbeAt) >= settings.LearningWindow {
		state.RecoveryStatus = adaptiveRecoveryStale
		state.RecoverySuccesses = 0
		changed = true
	}
	if !state.RecoveryProbeInFlight && (state.RecoveryStatus == "" || state.RecoveryStatus == adaptiveRecoveryActive) {
		status := adaptiveRecoveryActive
		if state.LastDispatchAt.IsZero() || now.Sub(state.LastDispatchAt) >= settings.LearningWindow {
			status = adaptiveRecoveryStale
		}
		if state.RecoveryStatus != status {
			state.RecoveryStatus = status
			if status == adaptiveRecoveryStale {
				state.RecoverySuccesses = 0
			}
			changed = true
		}
	}
	return changed
}

func pruneAdaptiveTTFTWindows(state *adaptiveAccountState, now time.Time, settings adaptiveCoreSettings) bool {
	if state == nil || len(state.TTFTWindows) == 0 {
		return false
	}
	cutoff := now.Add(-settings.LearningWindow)
	changed := false
	for key, observations := range state.TTFTWindows {
		first := 0
		for first < len(observations) && observations[first].At.Before(cutoff) {
			first++
		}
		if first == len(observations) {
			delete(state.TTFTWindows, key)
			changed = true
			continue
		}
		if first > 0 {
			state.TTFTWindows[key] = append([]adaptiveTTFTObservation(nil), observations[first:]...)
			changed = true
		}
		if observations = state.TTFTWindows[key]; len(observations) > adaptiveTTFTMaxSamplesPerBucket {
			state.TTFTWindows[key] = append([]adaptiveTTFTObservation(nil), observations[len(observations)-adaptiveTTFTMaxSamplesPerBucket:]...)
			changed = true
		}
	}
	for len(state.TTFTWindows) > adaptiveTTFTMaxBucketsPerAccount {
		delete(state.TTFTWindows, oldestAdaptiveTTFTBucket(state.TTFTWindows))
		changed = true
	}
	if len(state.TTFTWindows) == 0 {
		state.TTFTWindows = nil
		state.TTFTEMA = 0
		state.TTFTSamples = 0
		return true
	}
	if changed {
		refreshAdaptiveTTFTWindowSummary(state)
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
	return s.registerAdmissionWithLoad(accountID, requestID, configuredCapacity, -1, 0, true, now, settings)
}

func (s *adaptiveStateStore) registerPendingAdmission(accountID int64, requestID string, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) uint64 {
	return s.registerAdmissionWithLoad(accountID, requestID, configuredCapacity, -1, 0, false, now, settings)
}

func (s *adaptiveStateStore) registerAdmissionWithLoad(accountID int64, requestID string, configuredCapacity, observedConcurrency, waitingCount int, admitted bool, now time.Time, settings adaptiveCoreSettings) uint64 {
	return s.registerAdmissionWithLoadAndTTFTBucket(accountID, requestID, configuredCapacity, observedConcurrency, waitingCount, admitted, "", now, settings)
}

func (s *adaptiveStateStore) registerAdmissionWithLoadAndTTFTBucket(accountID int64, requestID string, configuredCapacity, observedConcurrency, waitingCount int, admitted bool, ttftBucketKey string, now time.Time, settings adaptiveCoreSettings) uint64 {
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
		transientKey := adaptiveTransientKey(accountID, requestID)
		healthProbe := state.HealthProbeInFlight && (state.HealthProbeOwner == "" || state.HealthProbeOwner == requestID)
		quotaProbe := state.QuotaProbeInFlight && (state.QuotaProbeOwner == "" || state.QuotaProbeOwner == requestID)
		recoveryProbe := state.RecoveryProbeInFlight && (state.RecoveryProbeOwner == "" || state.RecoveryProbeOwner == requestID)
		s.admissions[transientKey] = adaptiveAdmission{
			AccountID:           accountID,
			CapacityGeneration:  generation,
			TTFTBucketKey:       strings.TrimSpace(ttftBucketKey),
			HealthProbe:         healthProbe,
			QuotaProbe:          quotaProbe,
			RecoveryProbe:       recoveryProbe,
			Admitted:            admitted,
			ObservedConcurrency: observedConcurrency,
			WaitingCount:        waitingCount,
			ClaimedAt:           now,
		}
		if ttftBucketKey = strings.TrimSpace(ttftBucketKey); ttftBucketKey != "" {
			s.ttftClaims[transientKey] = adaptiveTTFTClaim{BucketKey: ttftBucketKey, ClaimedAt: now}
		}
	}
	if admitted {
		state.LastDispatchAt = now
		if !state.RecoveryProbeInFlight && state.RecoveryStatus != adaptiveRecoveryWarming {
			state.RecoveryStatus = adaptiveRecoveryActive
		}
		touchAdaptiveState(state, now)
	}
	return generation
}

func (s *adaptiveStateStore) claimRecoveryProbe(accountID int64, requestID string, configuredCapacity int, now time.Time, settings adaptiveCoreSettings) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	settings = normalizeAdaptiveCoreSettings(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupTransientsLocked(now, settings)
	state := s.ensureLocked(accountID, configuredCapacity, now)
	s.refreshLocked(state, now, settings)
	if state.RecoveryProbeInFlight || state.RecoveryStatus == adaptiveRecoveryActive || !state.CircuitOpenUntil.IsZero() || state.QuotaLimited {
		return false
	}
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		if _, claimed := s.recoveryProbeClaims[requestID]; claimed {
			return false
		}
	}
	inFlight := 0
	for _, candidateState := range s.accounts {
		if candidateState.RecoveryProbeInFlight && candidateState.RecoveryProbeUntil.After(now) {
			inFlight++
		}
	}
	if inFlight >= settings.RecoveryMaxConcurrency {
		return false
	}
	if requestID != "" {
		s.recoveryProbeClaims[requestID] = now
	}
	state.LastProbeAt = now
	state.RecoveryProbeInFlight = true
	state.RecoveryProbeUntil = now.Add(settings.HealthProbeLease)
	state.RecoveryProbeOwner = requestID
	state.RecoveryStatus = adaptiveRecoveryProbing
	touchAdaptiveState(state, now)
	return true
}

func (s *adaptiveStateStore) releaseRecoveryProbe(accountID int64, requestID string, now time.Time) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.accounts[accountID]
	if state == nil || !state.RecoveryProbeInFlight || state.RecoveryProbeOwner != strings.TrimSpace(requestID) {
		return
	}
	state.RecoveryProbeInFlight = false
	state.RecoveryProbeUntil = time.Time{}
	state.RecoveryProbeOwner = ""
	state.RecoveryStatus = adaptiveRecoveryStatusAfterProbe(state)
	touchAdaptiveState(state, now)
}

func adaptiveRecoveryStatusAfterProbe(state *adaptiveAccountState) adaptiveRecoveryStatus {
	if state != nil && state.RecoverySuccesses > 0 {
		return adaptiveRecoveryWarming
	}
	return adaptiveRecoveryStale
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
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		if _, claimed := s.healthProbeClaims[requestID]; claimed {
			return false
		}
		s.healthProbeClaims[requestID] = now
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
	_, allowed, _, _ := s.runtimeStatus(accountID, configuredCapacity, now, settings)
	return allowed
}

func adaptiveStateAllowedForSelection(state *adaptiveAccountState, now time.Time) bool {
	if state == nil {
		return false
	}
	if !state.CircuitOpenUntil.IsZero() && (state.CircuitOpenUntil.After(now) || state.HealthProbeInFlight) {
		return false
	}
	return !state.QuotaLimited || (adaptiveQuotaProbeDue(state, now) && !state.QuotaProbeInFlight)
}

func adaptiveQuotaProbeDue(state *adaptiveAccountState, now time.Time) bool {
	if state == nil || !state.QuotaLimited {
		return false
	}
	probeAt := adaptiveQuotaProbeAt(state)
	return probeAt.IsZero() || !probeAt.After(now)
}

func adaptiveQuotaProbeAt(state *adaptiveAccountState) time.Time {
	if state == nil {
		return time.Time{}
	}
	probeAt := state.QuotaNextProbeAt
	if !state.QuotaResetAt.IsZero() && (probeAt.IsZero() || state.QuotaResetAt.Before(probeAt)) {
		probeAt = state.QuotaResetAt
	}
	return probeAt
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
	recoveryProbe := false
	admitted := true
	hadAdmission := false
	transientKey := adaptiveTransientKey(observation.AccountID, observation.RequestID)
	if admission, ok := s.admissions[transientKey]; ok {
		hadAdmission = true
		if observation.CapacityGeneration == 0 {
			observation.CapacityGeneration = admission.CapacityGeneration
		}
		quotaProbe = admission.QuotaProbe
		recoveryProbe = admission.RecoveryProbe
		observation.HealthProbe = admission.HealthProbe
		observation.RecoveryProbe = admission.RecoveryProbe
		if observation.TTFTBucketKey == "" {
			observation.TTFTBucketKey = admission.TTFTBucketKey
		}
		admitted = admission.Admitted
		if observation.ObservedConcurrency < 0 && admission.ObservedConcurrency >= 0 {
			observation.ObservedConcurrency = admission.ObservedConcurrency
		}
		if observation.WaitingCount <= 0 {
			observation.WaitingCount = admission.WaitingCount
		}
		delete(s.admissions, transientKey)
	}
	if observation.TTFTBucketKey == "" {
		if claim, ok := s.ttftClaims[transientKey]; ok {
			observation.TTFTBucketKey = claim.BucketKey
		}
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
	if hadAdmission {
		state.LastDispatchAt = now
		if !recoveryProbe && state.RecoveryStatus != adaptiveRecoveryWarming {
			state.RecoveryStatus = adaptiveRecoveryActive
		}
	}

	switch observation.Type {
	case adaptiveObservationHealthSuccess:
		s.observeHealthLocked(state, true, observation, now, settings)
		delete(s.ttftClaims, transientKey)
		if quotaProbe && state.QuotaLimited {
			state.QuotaLimited = false
			state.QuotaResetAt = time.Time{}
			state.QuotaNextProbeAt = time.Time{}
			state.QuotaProbeInFlight = false
			state.QuotaProbeOwner = ""
		}
		capacityIncreased = s.observeCapacityRecoveryLocked(state, observation, admitted, now, settings)
		s.completeRecoveryProbeLocked(state, recoveryProbe, true, true, settings)
	case adaptiveObservationAccountFailure:
		if !adaptiveAccountHealthEligible(observation) {
			s.completeRecoveryProbeLocked(state, recoveryProbe, false, false, settings)
			break
		}
		if s.claimFailureLocked(observation.AccountID, observation.RequestID, now, settings) {
			s.observeHealthLocked(state, false, observation, now, settings)
		}
		s.completeRecoveryProbeLocked(state, recoveryProbe, false, true, settings)
	case adaptiveObservationCapacityLimit:
		if adaptiveAccountHealthEligible(observation) {
			if s.claimFailureLocked(observation.AccountID, observation.RequestID, now, settings) {
				s.observeHealthLocked(state, false, observation, now, settings)
			}
		}
		capacityDecreased = s.observeCapacityLimitLocked(state, observation, now, settings)
		s.completeRecoveryProbeLocked(state, recoveryProbe, false, true, settings)
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
		s.completeRecoveryProbeLocked(state, recoveryProbe, false, true, settings)
	case adaptiveObservationProviderOverload, adaptiveObservationIgnored:
		// Provider-wide and request-local failures do not count against account
		// health, but an inconclusive half-open probe must still advance the state.
		s.deferInconclusiveHealthProbeLocked(state, observation, now, settings)
		s.completeRecoveryProbeLocked(state, recoveryProbe, false, false, settings)
	}
	touchAdaptiveState(state, now)
	return capacityIncreased, capacityDecreased
}

func (s *adaptiveStateStore) completeRecoveryProbeLocked(state *adaptiveAccountState, recoveryProbe, success, conclusive bool, settings adaptiveCoreSettings) {
	if state == nil || !recoveryProbe {
		return
	}
	state.RecoveryProbeInFlight = false
	state.RecoveryProbeUntil = time.Time{}
	state.RecoveryProbeOwner = ""
	if success {
		state.RecoverySuccesses++
		if state.RecoverySuccesses >= settings.RecoveryWarmupSuccesses {
			state.RecoverySuccesses = settings.RecoveryWarmupSuccesses
			state.RecoveryStatus = adaptiveRecoveryActive
		} else {
			state.RecoveryStatus = adaptiveRecoveryWarming
		}
		return
	}
	if conclusive {
		state.RecoverySuccesses = 0
	}
	state.RecoveryStatus = adaptiveRecoveryStatusAfterProbe(state)
}

func adaptiveAccountHealthEligible(observation adaptiveObservation) bool {
	if observation.AccountHealthEligible != nil {
		return *observation.AccountHealthEligible
	}
	switch strings.ToLower(strings.TrimSpace(observation.ReasonCode)) {
	case "upstream_5xx", "generic_rate_limit", "account_health_failure", "stream_incomplete", "failure", "provider_overloaded", "provider_capacity", "model_upstream_error":
		return false
	default:
		return true
	}
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
	probeMatches := adaptiveHealthProbeMatches(state, observation)
	if success {
		state.ConsecutiveFailures = 0
		state.LastSuccessAt = now
		if state.CircuitOpenUntil.IsZero() || !state.CircuitOpenUntil.After(now) || probeMatches {
			state.CircuitOpenUntil = time.Time{}
			state.CircuitOpenCount = 0
			state.HealthProbeInFlight = false
			state.HealthProbeUntil = time.Time{}
			state.HealthProbeOwner = ""
		}
		if observation.FirstTokenMs != nil && observation.WindowedTTFT && strings.TrimSpace(observation.TTFTBucketKey) != "" {
			observeAdaptiveTTFTWindowLocked(state, observation.TTFTBucketKey, *observation.FirstTokenMs, now, settings)
		} else if observation.FirstTokenMs != nil && !observation.WindowedTTFT {
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

func adaptiveHealthProbeMatches(state *adaptiveAccountState, observation adaptiveObservation) bool {
	return observation.HealthProbe || (state.HealthProbeInFlight &&
		(state.HealthProbeOwner == "" || observation.RequestID == "" || state.HealthProbeOwner == observation.RequestID))
}

func (s *adaptiveStateStore) deferInconclusiveHealthProbeLocked(state *adaptiveAccountState, observation adaptiveObservation, now time.Time, settings adaptiveCoreSettings) {
	if state == nil || state.CircuitOpenUntil.IsZero() || state.CircuitOpenUntil.After(now) || !adaptiveHealthProbeMatches(state, observation) {
		return
	}
	state.CircuitOpenUntil = now.Add(settings.HealthProbeLease)
	state.HealthProbeInFlight = false
	state.HealthProbeUntil = time.Time{}
	state.HealthProbeOwner = ""
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

func (s *adaptiveStateStore) observeCapacityRecoveryLocked(state *adaptiveAccountState, observation adaptiveObservation, admitted bool, now time.Time, settings adaptiveCoreSettings) bool {
	if !state.CapacityHalfOpen || state.EffectiveCapacity >= state.ConfiguredCapacity || state.CapacityLimitedGeneration {
		return false
	}
	if !admitted {
		return false
	}
	if observation.CapacityGeneration != state.CapacityGeneration {
		return false
	}
	// Once the capacity cooldown has elapsed, every successful request that
	// actually acquired this account's slot is valid recovery evidence. Do not
	// require a queue or a utilization threshold: low traffic must be able to
	// restore a healthy account before the next burst arrives.
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
	for requestID, claimedAt := range s.healthProbeClaims {
		if now.Sub(claimedAt) > retention {
			delete(s.healthProbeClaims, requestID)
		}
	}
	for requestID, claimedAt := range s.recoveryProbeClaims {
		if now.Sub(claimedAt) > retention {
			delete(s.recoveryProbeClaims, requestID)
		}
	}
	for key, admission := range s.admissions {
		if now.Sub(admission.ClaimedAt) > retention {
			delete(s.admissions, key)
		}
	}
	for key, claim := range s.ttftClaims {
		if now.Sub(claim.ClaimedAt) > retention {
			delete(s.ttftClaims, key)
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
			flags = append(flags, adaptiveRuntimeCircuitHalfOpen)
		}
	}
	if state.QuotaLimited {
		flags = append(flags, adaptiveRuntimeQuotaLimited)
	}
	if state.CapacityHalfOpen {
		flags = appendUniqueAdaptiveRuntimeFlag(flags, adaptiveRuntimeCapacityRecovery)
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
	priority := []adaptiveRuntimeStatus{adaptiveRuntimeUnavailable, adaptiveRuntimeCooldown, adaptiveRuntimeCircuitHalfOpen, adaptiveRuntimeCapacityRecovery, adaptiveRuntimeQuotaLimited, adaptiveRuntimeSaturated, adaptiveRuntimeHighError, adaptiveRuntimeHealthy}
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

func containsAdaptiveRuntimeFlag(flags []adaptiveRuntimeStatus, wanted adaptiveRuntimeStatus) bool {
	for _, flag := range flags {
		if flag == wanted {
			return true
		}
	}
	return false
}

type adaptiveScoreCandidate struct {
	AccountID          int64
	OAuth              bool
	Cost               float64
	CurrentConcurrency int
	State              adaptiveAccountState
	TTFTBucketKey      string
	ReliabilityScore   float64
	CapacityScore      float64
	TTFTScore          float64
	TTFTP50            float64
	TTFTP90            float64
	TTFTWindowSamples  int
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
	windowedTTFT := false
	for i := range candidates {
		if candidates[i].Cost < 0 || math.IsNaN(candidates[i].Cost) || math.IsInf(candidates[i].Cost, 0) {
			candidates[i].Cost = 1
		}
		minimumCost = math.Min(minimumCost, candidates[i].Cost)
		if strings.TrimSpace(candidates[i].TTFTBucketKey) != "" {
			windowedTTFT = true
		} else if candidates[i].State.TTFTSamples > 0 && candidates[i].State.TTFTEMA > 0 {
			ttfts = append(ttfts, candidates[i].State.TTFTEMA)
		}
	}
	if math.IsInf(minimumCost, 1) {
		minimumCost = 1
	}
	sort.Float64s(ttfts)
	minTTFT, maxTTFT := 0.0, 0.0
	if len(ttfts) >= 2 {
		minTTFT, maxTTFT = ttfts[0], ttfts[len(ttfts)-1]
	}
	legacyTTFTEnabled := !windowedTTFT && len(ttfts) >= 2 && maxTTFT > minTTFT
	windowTTFTScores := scoreAdaptiveTTFTWindows(candidates, now, settings)
	baseWeightSum := settings.WeightReliability + settings.WeightCapacity + settings.WeightCost
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
		ttftEnabled := legacyTTFTEnabled
		if windowScore, ok := windowTTFTScores[i]; ok {
			candidate.TTFTP50 = windowScore.P50
			candidate.TTFTP90 = windowScore.P90
			candidate.TTFTWindowSamples = windowScore.Samples
			candidate.TTFTScore = windowScore.Score
			ttftEnabled = windowScore.Enabled
		} else if legacyTTFTEnabled {
			candidate.TTFTScore = 0.5
			if candidate.State.TTFTSamples > 0 && candidate.State.TTFTEMA > 0 {
				rawTTFTScore := 1 - normalizeAdaptiveValue(candidate.State.TTFTEMA, minTTFT, maxTTFT, 0.5)
				ttftConfidence := math.Min(1, float64(candidate.State.TTFTSamples)/float64(settings.LearningMinHealthSamples))
				candidate.TTFTScore = 0.5 + ttftConfidence*(rawTTFTScore-0.5)
			}
		}
		weightSum := baseWeightSum
		if ttftEnabled {
			weightSum += settings.WeightTTFT
		}
		if weightSum <= 0 {
			weightSum = 1
		}
		candidate.Score = (settings.WeightReliability*candidate.ReliabilityScore +
			settings.WeightCapacity*candidate.CapacityScore +
			settings.WeightCost*candidate.CostScore +
			settings.WeightTTFT*candidate.TTFTScore*boolFloat(ttftEnabled)) / weightSum
	}
	return candidates
}

const (
	adaptiveTTFTMinP50Samples  = 5
	adaptiveTTFTP50FullSamples = 20
	adaptiveTTFTMinP90Samples  = 20
	adaptiveTTFTP90FullSamples = 50
)

type adaptiveTTFTWindowStats struct {
	Samples int
	P50     float64
	P90     float64
}

type adaptiveTTFTWindowScore struct {
	Enabled bool
	Score   float64
	Samples int
	P50     float64
	P90     float64
}

func observeAdaptiveTTFTWindowLocked(state *adaptiveAccountState, bucketKey string, value int, now time.Time, settings adaptiveCoreSettings) {
	bucketKey = strings.TrimSpace(bucketKey)
	if state == nil || bucketKey == "" {
		return
	}
	if value < 1 {
		value = 1
	}
	if value > 120000 {
		value = 120000
	}
	pruneAdaptiveTTFTWindows(state, now, settings)
	if state.TTFTWindows == nil {
		state.TTFTWindows = make(map[string][]adaptiveTTFTObservation)
	}
	if _, exists := state.TTFTWindows[bucketKey]; !exists && len(state.TTFTWindows) >= adaptiveTTFTMaxBucketsPerAccount {
		delete(state.TTFTWindows, oldestAdaptiveTTFTBucket(state.TTFTWindows))
	}
	observations := append(state.TTFTWindows[bucketKey], adaptiveTTFTObservation{
		At:           now,
		Milliseconds: value,
	})
	if len(observations) > adaptiveTTFTMaxSamplesPerBucket {
		observations = append([]adaptiveTTFTObservation(nil), observations[len(observations)-adaptiveTTFTMaxSamplesPerBucket:]...)
	}
	state.TTFTWindows[bucketKey] = observations
	refreshAdaptiveTTFTWindowSummary(state)
}

func oldestAdaptiveTTFTBucket(windows map[string][]adaptiveTTFTObservation) string {
	oldestKey := ""
	var oldestAt time.Time
	for key, observations := range windows {
		lastAt := time.Time{}
		if len(observations) > 0 {
			lastAt = observations[len(observations)-1].At
		}
		if oldestKey == "" || lastAt.Before(oldestAt) || (lastAt.Equal(oldestAt) && key < oldestKey) {
			oldestKey = key
			oldestAt = lastAt
		}
	}
	return oldestKey
}

func refreshAdaptiveTTFTWindowSummary(state *adaptiveAccountState) {
	if state == nil || len(state.TTFTWindows) == 0 {
		return
	}
	values := make([]float64, 0)
	for _, observations := range state.TTFTWindows {
		for _, observation := range observations {
			if observation.Milliseconds > 0 {
				values = append(values, float64(observation.Milliseconds))
			}
		}
	}
	if len(values) == 0 {
		state.TTFTEMA = 0
		state.TTFTSamples = 0
		return
	}
	sort.Float64s(values)
	p50 := adaptiveTTFTPercentile(values, 0.50)
	p90 := adaptiveTTFTPercentile(values, 0.90)
	state.TTFTEMA = adaptiveTTFTBlend(p50, p90)
	state.TTFTSamples = int64(len(values))
}

func adaptiveTTFTWindowStatsForState(state adaptiveAccountState, bucketKey string, now time.Time, settings adaptiveCoreSettings) adaptiveTTFTWindowStats {
	observations := state.TTFTWindows[strings.TrimSpace(bucketKey)]
	if len(observations) == 0 {
		return adaptiveTTFTWindowStats{}
	}
	cutoff := now.Add(-settings.LearningWindow)
	values := make([]float64, 0, len(observations))
	for _, observation := range observations {
		if !observation.At.Before(cutoff) && observation.Milliseconds > 0 {
			values = append(values, float64(observation.Milliseconds))
		}
	}
	if len(values) == 0 {
		return adaptiveTTFTWindowStats{}
	}
	sort.Float64s(values)
	return adaptiveTTFTWindowStats{
		Samples: len(values),
		P50:     adaptiveTTFTPercentile(values, 0.50),
		P90:     adaptiveTTFTPercentile(values, 0.90),
	}
}

func scoreAdaptiveTTFTWindows(candidates []adaptiveScoreCandidate, now time.Time, settings adaptiveCoreSettings) map[int]adaptiveTTFTWindowScore {
	settings = normalizeAdaptiveCoreSettings(settings)
	stats := make([]adaptiveTTFTWindowStats, len(candidates))
	groups := make(map[string][]int)
	for i := range candidates {
		key := strings.TrimSpace(candidates[i].TTFTBucketKey)
		if key == "" {
			continue
		}
		stats[i] = adaptiveTTFTWindowStatsForState(candidates[i].State, key, now, settings)
		groups[key] = append(groups[key], i)
	}

	result := make(map[int]adaptiveTTFTWindowScore, len(candidates))
	for _, indexes := range groups {
		p50s := make([]float64, 0, len(indexes))
		p90s := make([]float64, 0, len(indexes))
		for _, index := range indexes {
			if stats[index].Samples >= adaptiveTTFTMinP50Samples {
				p50s = append(p50s, stats[index].P50)
			}
			if stats[index].Samples >= adaptiveTTFTMinP90Samples {
				p90s = append(p90s, stats[index].P90)
			}
		}
		if len(p50s) < 2 {
			for _, index := range indexes {
				result[index] = adaptiveTTFTWindowScore{Samples: stats[index].Samples, P50: stats[index].P50, P90: stats[index].P90}
			}
			continue
		}
		sort.Float64s(p50s)
		cohortP50 := adaptiveTTFTPercentile(p50s, 0.50)
		cohortP90 := cohortP50
		if len(p90s) >= 2 {
			sort.Float64s(p90s)
			cohortP90 = adaptiveTTFTPercentile(p90s, 0.50)
		}
		cohortEstimate := adaptiveTTFTBlend(cohortP50, cohortP90)
		estimates := make(map[int]float64, len(indexes))
		minEstimate, maxEstimate := math.Inf(1), 0.0
		for _, index := range indexes {
			adjustedP50, adjustedP90 := cohortP50, cohortP90
			if stats[index].Samples >= adaptiveTTFTMinP50Samples {
				confidence := math.Min(1, float64(stats[index].Samples)/adaptiveTTFTP50FullSamples)
				adjustedP50 = adaptiveTTFTLogShrink(stats[index].P50, cohortP50, confidence)
			}
			if stats[index].Samples >= adaptiveTTFTMinP90Samples && len(p90s) >= 2 {
				confidence := math.Min(1, float64(stats[index].Samples-adaptiveTTFTMinP90Samples)/float64(adaptiveTTFTP90FullSamples-adaptiveTTFTMinP90Samples))
				adjustedP90 = adaptiveTTFTLogShrink(stats[index].P90, cohortP90, confidence)
			}
			estimate := adaptiveTTFTBlend(adjustedP50, adjustedP90)
			estimates[index] = estimate
			minEstimate = math.Min(minEstimate, estimate)
			maxEstimate = math.Max(maxEstimate, estimate)
		}
		enabled := cohortEstimate > 0 && minEstimate > 0 && maxEstimate/minEstimate > 1.001
		for _, index := range indexes {
			score := 0.0
			if enabled {
				ratio := estimates[index] / cohortEstimate
				score = 1 / (1 + ratio*ratio)
			}
			result[index] = adaptiveTTFTWindowScore{
				Enabled: enabled,
				Score:   score,
				Samples: stats[index].Samples,
				P50:     stats[index].P50,
				P90:     stats[index].P90,
			}
		}
	}
	return result
}

func adaptiveTTFTPercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}
	position := clampFloat(percentile, 0, 1, 0.5) * float64(len(sortedValues)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sortedValues[lower]
	}
	fraction := position - float64(lower)
	return sortedValues[lower] + fraction*(sortedValues[upper]-sortedValues[lower])
}

func adaptiveTTFTLogShrink(value, cohort, confidence float64) float64 {
	if value <= 0 || cohort <= 0 {
		return cohort
	}
	confidence = clamp01(confidence)
	return math.Exp(confidence*math.Log(value) + (1-confidence)*math.Log(cohort))
}

func adaptiveTTFTBlend(p50, p90 float64) float64 {
	if p50 <= 0 {
		return 0
	}
	if p90 <= 0 {
		p90 = p50
	}
	return math.Exp(0.8*math.Log(p50) + 0.2*math.Log(p90))
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
	if success && (reason == "success" || reason == "success_no_result" || reason == "legacy_result") {
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
