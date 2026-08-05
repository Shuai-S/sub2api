package service

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	geminiAdaptiveStateLocalMergeLimit     = 20
	geminiAdaptiveFailureSampleRetention   = 10 * time.Minute
	geminiAdaptiveAuthFailureThreshold     = 1
	geminiAdaptiveCircuitScopeAccount      = "account"
	geminiAdaptiveCircuitScopeModel        = "model"
	geminiAdaptiveCircuitStatusClosed      = "closed"
	geminiAdaptiveCircuitStatusOpen        = "open"
	geminiAdaptiveCircuitStatusHalfOpen    = "half_open"
	geminiAdaptiveCircuitStatusProbeActive = "probe_in_flight"
)

type geminiAdaptiveModelState struct {
	SuccessEMA float64 `json:"success_ema"`
	TTFTEMA    float64 `json:"ttft_ema"`
	LatencyEMA float64 `json:"latency_ema"`
	Samples    int64   `json:"samples"`
	Failures   int64   `json:"failures"`
}

type geminiAdaptiveCircuitState struct {
	ConsecutiveFailure int       `json:"consecutive_failure"`
	OpenUntil          time.Time `json:"open_until,omitempty"`
	ProbeUntil         time.Time `json:"-"`
	ProbeInFlight      bool      `json:"-"`
	ProbeOwner         string    `json:"-"`
}

type geminiAdaptiveAccountState struct {
	AccountID                  int64
	EstimatedCapacity          int
	PathSuccessEMA             float64
	ByModelFamily              map[string]geminiAdaptiveModelState
	AccountCircuit             geminiAdaptiveCircuitState
	ModelCircuits              map[string]geminiAdaptiveCircuitState
	ConsecutiveSuccess         int
	ConsecutiveFailure         int
	ConsecutiveCapacityFailure int
	TotalSamples               int64
	RecentHealthSamples        int
	RecentHealthFailures       int
	RecentCapacitySamples      int
	RecentCapacityFailures     int
	LastSuccessAt              time.Time
	LastFailureAt              time.Time
	LastCapacityFailureAt      time.Time
	RecentWindowStartedAt      time.Time
	CooldownUntil              time.Time
	UpdatedAt                  time.Time

	revision          uint64
	persistedRevision uint64
}

type geminiAdaptiveStateStore struct {
	mu                       sync.RWMutex
	accounts                 map[int64]*geminiAdaptiveAccountState
	failureSampleClaims      map[string]time.Time
	lastFailureSampleCleanup time.Time
}

func newGeminiAdaptiveStateStore() *geminiAdaptiveStateStore {
	return &geminiAdaptiveStateStore{
		accounts:            make(map[int64]*geminiAdaptiveAccountState),
		failureSampleClaims: make(map[string]time.Time),
	}
}

func defaultGeminiAdaptiveAccountState(account *Account, now time.Time, settings GeminiAdaptiveSchedulerSettings) geminiAdaptiveAccountState {
	accountID := int64(0)
	capacity := 0
	if account != nil {
		accountID = account.ID
		if account.Concurrency > 0 {
			capacity = account.Concurrency
		}
	}
	return geminiAdaptiveAccountState{
		AccountID:             accountID,
		EstimatedCapacity:     capacity,
		PathSuccessEMA:        settings.GeminiAdaptiveSchedulerInitialReliability,
		ByModelFamily:         make(map[string]geminiAdaptiveModelState, 5),
		ModelCircuits:         make(map[string]geminiAdaptiveCircuitState),
		RecentWindowStartedAt: now,
	}
}

func cloneGeminiAdaptiveAccountState(state *geminiAdaptiveAccountState) geminiAdaptiveAccountState {
	if state == nil {
		return geminiAdaptiveAccountState{}
	}
	clone := *state
	clone.ByModelFamily = cloneGeminiAdaptiveModelMap(state.ByModelFamily)
	clone.ModelCircuits = cloneGeminiAdaptiveCircuitMap(state.ModelCircuits)
	return clone
}

func cloneGeminiAdaptiveModelMap(in map[string]geminiAdaptiveModelState) map[string]geminiAdaptiveModelState {
	out := make(map[string]geminiAdaptiveModelState, max(5, len(in)))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneGeminiAdaptiveCircuitMap(in map[string]geminiAdaptiveCircuitState) map[string]geminiAdaptiveCircuitState {
	out := make(map[string]geminiAdaptiveCircuitState, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

type geminiAdaptiveCircuitEligibility struct {
	Allowed   bool
	HalfOpen  bool
	Scope     string
	Status    string
	OpenUntil time.Time
}

func (s *geminiAdaptiveStateStore) circuitEligibility(account *Account, requestedModel, action string, now time.Time, settings GeminiAdaptiveSchedulerSettings) geminiAdaptiveCircuitEligibility {
	if s == nil || account == nil {
		return geminiAdaptiveCircuitEligibility{Allowed: false}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(account, now, settings)
	modelKey := geminiAdaptiveCanonicalModel(account, requestedModel, "", action)
	accountAllowed, accountHalfOpen, accountStatus := geminiAdaptiveCircuitAllows(state.AccountCircuit, now)
	modelCircuit := state.ModelCircuits[modelKey]
	modelAllowed, modelHalfOpen, modelStatus := geminiAdaptiveCircuitAllows(modelCircuit, now)
	if !accountAllowed {
		return geminiAdaptiveCircuitEligibility{Scope: geminiAdaptiveCircuitScopeAccount, Status: accountStatus, OpenUntil: state.AccountCircuit.OpenUntil}
	}
	if !modelAllowed {
		return geminiAdaptiveCircuitEligibility{Scope: geminiAdaptiveCircuitScopeModel, Status: modelStatus, OpenUntil: modelCircuit.OpenUntil}
	}
	if accountHalfOpen {
		return geminiAdaptiveCircuitEligibility{Allowed: true, HalfOpen: true, Scope: geminiAdaptiveCircuitScopeAccount, Status: accountStatus, OpenUntil: state.AccountCircuit.OpenUntil}
	}
	if modelHalfOpen {
		return geminiAdaptiveCircuitEligibility{Allowed: true, HalfOpen: true, Scope: geminiAdaptiveCircuitScopeModel, Status: modelStatus, OpenUntil: modelCircuit.OpenUntil}
	}
	return geminiAdaptiveCircuitEligibility{Allowed: true, Status: geminiAdaptiveCircuitStatusClosed}
}

func geminiAdaptiveCircuitAllows(circuit geminiAdaptiveCircuitState, now time.Time) (allowed, halfOpen bool, status string) {
	if circuit.OpenUntil.IsZero() {
		return true, false, geminiAdaptiveCircuitStatusClosed
	}
	if circuit.OpenUntil.After(now) {
		return false, false, geminiAdaptiveCircuitStatusOpen
	}
	if circuit.ProbeInFlight && circuit.ProbeUntil.After(now) {
		return false, false, geminiAdaptiveCircuitStatusProbeActive
	}
	return true, true, geminiAdaptiveCircuitStatusHalfOpen
}

func (s *geminiAdaptiveStateStore) claimCircuitProbe(account *Account, requestedModel, action, owner string, now time.Time, settings GeminiAdaptiveSchedulerSettings) (allowed, claimed bool) {
	if s == nil || account == nil {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(account, now, settings)
	modelKey := geminiAdaptiveCanonicalModel(account, requestedModel, "", action)
	modelCircuit := state.ModelCircuits[modelKey]
	accountAllowed, accountHalfOpen, _ := geminiAdaptiveCircuitAllows(state.AccountCircuit, now)
	modelAllowed, modelHalfOpen, _ := geminiAdaptiveCircuitAllows(modelCircuit, now)
	if !accountAllowed || !modelAllowed {
		return false, false
	}
	if !accountHalfOpen && !modelHalfOpen {
		return true, false
	}
	lease := time.Duration(settings.GeminiAdaptiveSchedulerHalfOpenProbeLeaseSeconds) * time.Second
	if accountHalfOpen {
		claimGeminiAdaptiveCircuitProbe(&state.AccountCircuit, owner, now, lease)
	}
	if modelHalfOpen {
		claimGeminiAdaptiveCircuitProbe(&modelCircuit, owner, now, lease)
		storeGeminiAdaptiveModelCircuit(state, modelKey, modelCircuit)
	}
	touchGeminiAdaptiveAccountState(state, now)
	return true, true
}

func claimGeminiAdaptiveCircuitProbe(circuit *geminiAdaptiveCircuitState, owner string, now time.Time, lease time.Duration) {
	if circuit == nil {
		return
	}
	circuit.ProbeInFlight = true
	circuit.ProbeOwner = owner
	circuit.ProbeUntil = now.Add(lease)
}

func (s *geminiAdaptiveStateStore) releaseCircuitProbe(account *Account, requestedModel, action, owner string, now time.Time, settings GeminiAdaptiveSchedulerSettings) {
	if s == nil || account == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(account, now, settings)
	changed := releaseGeminiAdaptiveCircuitProbe(&state.AccountCircuit, owner)
	modelKey := geminiAdaptiveCanonicalModel(account, requestedModel, "", action)
	modelCircuit := state.ModelCircuits[modelKey]
	if releaseGeminiAdaptiveCircuitProbe(&modelCircuit, owner) {
		storeGeminiAdaptiveModelCircuit(state, modelKey, modelCircuit)
		changed = true
	}
	if changed {
		touchGeminiAdaptiveAccountState(state, now)
	}
}

func releaseGeminiAdaptiveCircuitProbe(circuit *geminiAdaptiveCircuitState, owner string) bool {
	if circuit == nil || !circuit.ProbeInFlight || (circuit.ProbeOwner != "" && circuit.ProbeOwner != owner) {
		return false
	}
	circuit.ProbeInFlight = false
	circuit.ProbeUntil = time.Time{}
	circuit.ProbeOwner = ""
	return true
}

func storeGeminiAdaptiveModelCircuit(state *geminiAdaptiveAccountState, modelKey string, circuit geminiAdaptiveCircuitState) {
	if state == nil || modelKey == "" {
		return
	}
	if circuit.ConsecutiveFailure == 0 && circuit.OpenUntil.IsZero() && !circuit.ProbeInFlight {
		delete(state.ModelCircuits, modelKey)
		return
	}
	state.ModelCircuits[modelKey] = circuit
}

func (s *geminiAdaptiveStateStore) snapshot(account *Account, settings GeminiAdaptiveSchedulerSettings) geminiAdaptiveAccountState {
	if account == nil {
		return geminiAdaptiveAccountState{}
	}
	s.mu.RLock()
	state := s.accounts[account.ID]
	snapshot := cloneGeminiAdaptiveAccountState(state)
	s.mu.RUnlock()
	if state == nil {
		return defaultGeminiAdaptiveAccountState(account, time.Now(), settings)
	}
	if account.Concurrency <= 0 {
		snapshot.EstimatedCapacity = 0
	} else if snapshot.EstimatedCapacity <= 0 || snapshot.EstimatedCapacity > account.Concurrency {
		snapshot.EstimatedCapacity = account.Concurrency
	}
	return snapshot
}

func (s *geminiAdaptiveStateStore) effectiveCapacity(account *Account, settings GeminiAdaptiveSchedulerSettings) int {
	if account == nil || account.Concurrency <= 0 {
		return 0
	}
	capacity := s.snapshot(account, settings).EstimatedCapacity
	if capacity <= 0 || capacity > account.Concurrency {
		capacity = account.Concurrency
	}
	return capacity
}

func (s *geminiAdaptiveStateStore) observeLoad(ctx context.Context, account *Account, load *AccountLoadInfo, now time.Time, settings GeminiAdaptiveSchedulerSettings) geminiAdaptiveAccountState {
	if account == nil {
		return geminiAdaptiveAccountState{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.accounts[account.ID]
	previousCapacity := 0
	if previous != nil {
		previousCapacity = previous.EstimatedCapacity
	}
	state := s.ensureLocked(account, now, settings)
	changed := previous != nil && previousCapacity != state.EstimatedCapacity
	changed = s.resetWindowLocked(state, now, settings) || changed
	defer func() {
		if changed {
			touchGeminiAdaptiveAccountState(state, now)
		}
	}()
	if account.Concurrency <= 0 || state.EstimatedCapacity >= account.Concurrency || state.CooldownUntil.After(now) {
		return cloneGeminiAdaptiveAccountState(state)
	}
	loadHigh := false
	if load != nil {
		loadHigh = load.WaitingCount > 0
		if state.EstimatedCapacity > 0 {
			loadHigh = loadHigh || float64(load.CurrentConcurrency)/float64(state.EstimatedCapacity) >= settings.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold
		}
	}
	if loadHigh && state.PathSuccessEMA >= settings.GeminiAdaptiveSchedulerCapacitySuccessThreshold && state.RecentCapacityFailures == 0 && state.ConsecutiveSuccess >= max(1, state.EstimatedCapacity) {
		previousEstimatedCapacity := state.EstimatedCapacity
		state.EstimatedCapacity = min(account.Concurrency, state.EstimatedCapacity+settings.GeminiAdaptiveSchedulerCapacityIncreaseStep)
		state.ConsecutiveSuccess = 0
		changed = true
		currentConcurrency, waitingCount, loadRate := 0, 0, 0
		if load != nil {
			currentConcurrency = load.CurrentConcurrency
			waitingCount = load.WaitingCount
			loadRate = load.LoadRate
		}
		slog.Info("gemini_adaptive_scheduler_capacity_changed",
			"request_id", contextStringValue(ctx, ctxkey.RequestID),
			"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
			"account_id", account.ID,
			"direction", "increase",
			"trigger", "capacity_probe",
			"configured_capacity", account.Concurrency,
			"previous_capacity", previousEstimatedCapacity,
			"estimated_capacity", state.EstimatedCapacity,
			"current_concurrency", currentConcurrency,
			"waiting_count", waitingCount,
			"load_rate", loadRate,
			"path_success_ema", state.PathSuccessEMA,
			"recent_capacity_samples", state.RecentCapacitySamples,
			"recent_capacity_failures", state.RecentCapacityFailures,
		)
	}
	return cloneGeminiAdaptiveAccountState(state)
}

type GeminiAdaptiveScheduleReport struct {
	Account              *Account
	RequestID            string
	RequestedModel       string
	MappedModel          string
	UpstreamRequestID    string
	Stream               bool
	Action               string
	Success              bool
	PathSample           bool
	ModelSample          bool
	CapacitySample       bool
	AccountCircuitSample bool
	ModelCircuitSample   bool
	Synthetic            bool
	FirstTokenMs         *int
	DurationMs           int64
	TerminalReason       string
	ctx                  context.Context
}

func (s *geminiAdaptiveStateStore) report(report GeminiAdaptiveScheduleReport, now time.Time, settings GeminiAdaptiveSchedulerSettings) (capacityIncreased bool, capacityDecreased bool) {
	if report.Account == nil {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(report.Account, now, settings)
	modelKey := geminiAdaptiveCanonicalModel(report.Account, report.RequestedModel, report.MappedModel, report.Action)
	modelCircuit := state.ModelCircuits[modelKey]
	if !report.Success && (report.PathSample || report.ModelSample || report.CapacitySample || report.AccountCircuitSample || report.ModelCircuitSample) &&
		!s.claimCircuitFailureSampleLocked(report.Account.ID, report.RequestID, "learning", modelKey, now) {
		report.PathSample = false
		report.ModelSample = false
		report.CapacitySample = false
		report.AccountCircuitSample = false
		report.ModelCircuitSample = false
	}
	if report.Synthetic || (!report.PathSample && !report.ModelSample && !report.CapacitySample && !report.AccountCircuitSample && !report.ModelCircuitSample) {
		changed := releaseGeminiAdaptiveCircuitProbe(&state.AccountCircuit, report.RequestID)
		if releaseGeminiAdaptiveCircuitProbe(&modelCircuit, report.RequestID) {
			storeGeminiAdaptiveModelCircuit(state, modelKey, modelCircuit)
			changed = true
		}
		if changed {
			touchGeminiAdaptiveAccountState(state, now)
		}
		return false, false
	}
	s.resetWindowLocked(state, now, settings)
	defer touchGeminiAdaptiveAccountState(state, now)
	state.TotalSamples++

	if report.AccountCircuitSample {
		threshold := settings.GeminiAdaptiveSchedulerAccountFailureThreshold
		if report.TerminalReason == "account_auth" {
			threshold = geminiAdaptiveAuthFailureThreshold
		}
		if report.Success || s.claimCircuitFailureSampleLocked(report.Account.ID, report.RequestID, geminiAdaptiveCircuitScopeAccount, modelKey, now) {
			applyGeminiAdaptiveCircuitSample(&state.AccountCircuit, report.Success, threshold, report.RequestID, now, settings)
		}
	} else {
		releaseGeminiAdaptiveCircuitProbe(&state.AccountCircuit, report.RequestID)
	}
	if report.ModelCircuitSample {
		if report.Success || s.claimCircuitFailureSampleLocked(report.Account.ID, report.RequestID, geminiAdaptiveCircuitScopeModel, modelKey, now) {
			applyGeminiAdaptiveCircuitSample(&modelCircuit, report.Success, settings.GeminiAdaptiveSchedulerModelFailureThreshold, report.RequestID, now, settings)
		}
	} else {
		releaseGeminiAdaptiveCircuitProbe(&modelCircuit, report.RequestID)
	}
	storeGeminiAdaptiveModelCircuit(state, modelKey, modelCircuit)

	if report.PathSample {
		state.RecentHealthSamples++
		if report.Success {
			state.PathSuccessEMA = updateGeminiAdaptiveEMA(state.PathSuccessEMA, 1, settings.GeminiAdaptiveSchedulerSuccessEMAAlpha)
			state.ConsecutiveSuccess++
			state.ConsecutiveFailure = 0
			state.LastSuccessAt = now
		} else {
			state.PathSuccessEMA = updateGeminiAdaptiveEMA(state.PathSuccessEMA, 0, settings.GeminiAdaptiveSchedulerSuccessEMAAlpha)
			state.ConsecutiveSuccess = 0
			state.ConsecutiveFailure++
			state.RecentHealthFailures++
			state.LastFailureAt = now
		}
	}

	if report.ModelSample {
		family := geminiAdaptiveModelFamily(firstNonEmpty(report.MappedModel, report.RequestedModel), report.Action)
		modelState := state.ByModelFamily[family]
		if modelState.Samples == 0 {
			modelState.SuccessEMA = settings.GeminiAdaptiveSchedulerInitialReliability
		}
		modelState.Samples++
		if report.Success {
			modelState.SuccessEMA = updateGeminiAdaptiveEMA(modelState.SuccessEMA, 1, settings.GeminiAdaptiveSchedulerSuccessEMAAlpha)
			if report.FirstTokenMs != nil && *report.FirstTokenMs >= 0 {
				modelState.TTFTEMA = updateGeminiAdaptiveEMA(modelState.TTFTEMA, float64(*report.FirstTokenMs), settings.GeminiAdaptiveSchedulerLatencyEMAAlpha)
			}
			if report.DurationMs >= 0 {
				modelState.LatencyEMA = updateGeminiAdaptiveEMA(modelState.LatencyEMA, float64(report.DurationMs), settings.GeminiAdaptiveSchedulerLatencyEMAAlpha)
			}
		} else {
			modelState.SuccessEMA = updateGeminiAdaptiveEMA(modelState.SuccessEMA, 0, settings.GeminiAdaptiveSchedulerSuccessEMAAlpha)
			modelState.Failures++
		}
		state.ByModelFamily[family] = modelState
	}

	if report.CapacitySample && report.Account.Concurrency > 0 {
		state.RecentCapacitySamples++
		if report.Success {
			state.ConsecutiveCapacityFailure = 0
		} else {
			state.RecentCapacityFailures++
			state.ConsecutiveCapacityFailure++
			state.LastCapacityFailureAt = now
			if s.shouldShrinkLocked(state, now, settings) {
				factor := settings.GeminiAdaptiveSchedulerShrinkFactorSoft
				if state.ConsecutiveCapacityFailure >= settings.GeminiAdaptiveSchedulerCapacityFailureThreshold*settings.GeminiAdaptiveSchedulerHardShrinkFailureMultiplier {
					factor = settings.GeminiAdaptiveSchedulerShrinkFactorHard
				}
				previousEstimatedCapacity := state.EstimatedCapacity
				next := int(math.Floor(float64(previousEstimatedCapacity) * factor))
				minCapacity := min(settings.GeminiAdaptiveSchedulerMinCapacity, report.Account.Concurrency)
				if next < minCapacity {
					next = minCapacity
				}
				if next < previousEstimatedCapacity {
					state.EstimatedCapacity = next
					state.CooldownUntil = now.Add(time.Duration(settings.GeminiAdaptiveSchedulerCooldownSeconds) * time.Second)
					capacityDecreased = true
					failureRate := float64(0)
					if state.RecentCapacitySamples > 0 {
						failureRate = float64(state.RecentCapacityFailures) / float64(state.RecentCapacitySamples)
					}
					slog.Info("gemini_adaptive_scheduler_capacity_changed",
						"request_id", contextStringValue(report.ctx, ctxkey.RequestID),
						"client_request_id", contextStringValue(report.ctx, ctxkey.ClientRequestID),
						"account_id", report.Account.ID,
						"direction", "decrease",
						"trigger", report.TerminalReason,
						"model", report.RequestedModel,
						"model_family", geminiAdaptiveModelFamily(firstNonEmpty(report.MappedModel, report.RequestedModel), report.Action),
						"configured_capacity", report.Account.Concurrency,
						"previous_capacity", previousEstimatedCapacity,
						"estimated_capacity", state.EstimatedCapacity,
						"shrink_factor", factor,
						"recent_capacity_samples", state.RecentCapacitySamples,
						"recent_capacity_failures", state.RecentCapacityFailures,
						"capacity_failure_rate", failureRate,
						"consecutive_capacity_failure", state.ConsecutiveCapacityFailure,
						"cooldown_until", state.CooldownUntil,
					)
				}
			}
		}
	}
	return false, capacityDecreased
}

func (s *geminiAdaptiveStateStore) claimCircuitFailureSampleLocked(accountID int64, requestID, scope, modelKey string, now time.Time) bool {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return true
	}
	if s.failureSampleClaims == nil {
		s.failureSampleClaims = make(map[string]time.Time)
	}
	if s.lastFailureSampleCleanup.IsZero() || now.Sub(s.lastFailureSampleCleanup) >= time.Minute {
		for key, claimedAt := range s.failureSampleClaims {
			if now.Sub(claimedAt) > geminiAdaptiveFailureSampleRetention {
				delete(s.failureSampleClaims, key)
			}
		}
		s.lastFailureSampleCleanup = now
	}
	key := strconv.FormatInt(accountID, 10) + ":" + requestID + ":" + scope + ":" + modelKey
	if _, exists := s.failureSampleClaims[key]; exists {
		return false
	}
	s.failureSampleClaims[key] = now
	return true
}

func applyGeminiAdaptiveCircuitSample(circuit *geminiAdaptiveCircuitState, success bool, threshold int, owner string, now time.Time, settings GeminiAdaptiveSchedulerSettings) {
	if circuit == nil {
		return
	}
	probeMatches := circuit.ProbeInFlight && (circuit.ProbeOwner == "" || circuit.ProbeOwner == owner)
	if success {
		if circuit.OpenUntil.IsZero() || probeMatches {
			*circuit = geminiAdaptiveCircuitState{}
		}
		return
	}
	if !circuit.OpenUntil.IsZero() && !probeMatches {
		return
	}
	circuit.ConsecutiveFailure++
	if circuit.ConsecutiveFailure < max(1, threshold) {
		return
	}
	circuit.OpenUntil = now.Add(geminiAdaptiveCircuitCooldown(circuit.ConsecutiveFailure, threshold, settings))
	circuit.ProbeUntil = time.Time{}
	circuit.ProbeInFlight = false
	circuit.ProbeOwner = ""
}

func geminiAdaptiveCircuitCooldown(failures, threshold int, settings GeminiAdaptiveSchedulerSettings) time.Duration {
	base := time.Duration(settings.GeminiAdaptiveSchedulerCooldownSeconds) * time.Second
	if base <= 0 {
		base = time.Minute
	}
	maximum := time.Duration(settings.GeminiAdaptiveSchedulerCooldownMaxSeconds) * time.Second
	if maximum < base {
		maximum = base
	}
	for extra := failures - max(1, threshold); extra > 0 && base < maximum; extra-- {
		if base > maximum/2 {
			return maximum
		}
		base *= 2
	}
	if base > maximum {
		return maximum
	}
	return base
}

func (s *geminiAdaptiveStateStore) ensureLocked(account *Account, now time.Time, settings GeminiAdaptiveSchedulerSettings) *geminiAdaptiveAccountState {
	state := s.accounts[account.ID]
	if state == nil {
		initial := defaultGeminiAdaptiveAccountState(account, now, settings)
		state = &initial
		s.accounts[account.ID] = state
	}
	if state.ByModelFamily == nil {
		state.ByModelFamily = make(map[string]geminiAdaptiveModelState, 5)
	}
	if state.ModelCircuits == nil {
		state.ModelCircuits = make(map[string]geminiAdaptiveCircuitState)
	}
	if account.Concurrency <= 0 {
		state.EstimatedCapacity = 0
	} else if state.EstimatedCapacity <= 0 || state.EstimatedCapacity > account.Concurrency {
		state.EstimatedCapacity = account.Concurrency
	} else if minCapacity := min(settings.GeminiAdaptiveSchedulerMinCapacity, account.Concurrency); state.EstimatedCapacity < minCapacity {
		state.EstimatedCapacity = minCapacity
	}
	return state
}

func (s *geminiAdaptiveStateStore) resetWindowLocked(state *geminiAdaptiveAccountState, now time.Time, settings GeminiAdaptiveSchedulerSettings) bool {
	window := time.Duration(settings.GeminiAdaptiveSchedulerLearningWindowSeconds) * time.Second
	if state.RecentWindowStartedAt.IsZero() || now.Sub(state.RecentWindowStartedAt) >= window {
		state.RecentWindowStartedAt = now
		state.RecentHealthSamples = 0
		state.RecentHealthFailures = 0
		state.RecentCapacitySamples = 0
		state.RecentCapacityFailures = 0
		return true
	}
	return false
}

func (s *geminiAdaptiveStateStore) shouldShrinkLocked(state *geminiAdaptiveAccountState, now time.Time, settings GeminiAdaptiveSchedulerSettings) bool {
	if state.EstimatedCapacity <= settings.GeminiAdaptiveSchedulerMinCapacity || state.CooldownUntil.After(now) || state.ConsecutiveCapacityFailure < settings.GeminiAdaptiveSchedulerCapacityFailureThreshold || state.RecentCapacitySamples < settings.GeminiAdaptiveSchedulerMinRecentSamplesForShrink {
		return false
	}
	return float64(state.RecentCapacityFailures)/float64(state.RecentCapacitySamples) >= settings.GeminiAdaptiveSchedulerShrinkErrorThreshold
}

func touchGeminiAdaptiveAccountState(state *geminiAdaptiveAccountState, now time.Time) {
	if state == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.UpdatedAt = now
	state.revision++
}

func updateGeminiAdaptiveEMA(current, sample, alpha float64) float64 {
	if current <= 0 {
		return sample
	}
	return alpha*sample + (1-alpha)*current
}

func geminiAdaptiveModelFamily(model, action string) string {
	value := strings.ToLower(strings.TrimSpace(model + " " + action))
	switch {
	case strings.Contains(value, "image"), strings.Contains(value, "imagen"):
		return "image"
	case strings.Contains(value, "embed"):
		return "embedding"
	case geminiModelClassFromName(model) == geminiModelFlash:
		return "flash"
	case strings.Contains(strings.ToLower(model), "pro"):
		return "pro"
	default:
		return "other"
	}
}

func geminiAdaptiveCanonicalModel(account *Account, requestedModel, mappedModel, action string) string {
	model := strings.TrimSpace(mappedModel)
	if model == "" && account != nil {
		model = strings.TrimSpace(account.GetMappedModel(requestedModel))
	}
	if model == "" {
		model = strings.TrimSpace(requestedModel)
	}
	if model == "" {
		model = "unknown:" + geminiAdaptiveModelFamily(requestedModel, action)
	}
	return strings.ToLower(model)
}

type geminiAdaptiveDirtySnapshot struct {
	state    geminiAdaptiveAccountState
	revision uint64
}

func (s *geminiAdaptiveStateStore) dirtySnapshots(now time.Time, retention time.Duration) []geminiAdaptiveDirtySnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]geminiAdaptiveDirtySnapshot, 0, len(s.accounts))
	for _, state := range s.accounts {
		if state == nil || state.revision <= state.persistedRevision || state.UpdatedAt.IsZero() || (retention > 0 && now.Sub(state.UpdatedAt) > retention) {
			continue
		}
		out = append(out, geminiAdaptiveDirtySnapshot{state: cloneGeminiAdaptiveAccountState(state), revision: state.revision})
	}
	return out
}

func (s *geminiAdaptiveStateStore) markPersisted(snapshots []geminiAdaptiveDirtySnapshot) {
	if s == nil || len(snapshots) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, snapshot := range snapshots {
		if state := s.accounts[snapshot.state.AccountID]; state != nil && snapshot.revision > state.persistedRevision {
			state.persistedRevision = snapshot.revision
		}
	}
}

func (s *geminiAdaptiveStateStore) restoreAtStartup(incoming geminiAdaptiveAccountState, now time.Time) bool {
	if s == nil || incoming.AccountID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	incoming.ByModelFamily = cloneGeminiAdaptiveModelMap(incoming.ByModelFamily)
	incoming.ModelCircuits = cloneGeminiAdaptiveCircuitMap(incoming.ModelCircuits)
	incoming.revision = 1
	incoming.persistedRevision = 1
	local := s.accounts[incoming.AccountID]
	if local == nil || local.TotalSamples == 0 {
		restored := incoming
		s.accounts[incoming.AccountID] = &restored
		return true
	}
	if local.TotalSamples >= geminiAdaptiveStateLocalMergeLimit {
		return false
	}
	merged := incoming
	merged.TotalSamples += local.TotalSamples
	merged.RecentHealthSamples += local.RecentHealthSamples
	merged.RecentHealthFailures += local.RecentHealthFailures
	merged.RecentCapacitySamples += local.RecentCapacitySamples
	merged.RecentCapacityFailures += local.RecentCapacityFailures
	merged.UpdatedAt = laterTime(incoming.UpdatedAt, local.UpdatedAt)
	merged.LastSuccessAt = laterTime(incoming.LastSuccessAt, local.LastSuccessAt)
	merged.LastFailureAt = laterTime(incoming.LastFailureAt, local.LastFailureAt)
	merged.LastCapacityFailureAt = laterTime(incoming.LastCapacityFailureAt, local.LastCapacityFailureAt)
	for family, modelState := range local.ByModelFamily {
		if current, ok := merged.ByModelFamily[family]; !ok || current.Samples == 0 {
			merged.ByModelFamily[family] = modelState
		}
	}
	for model, circuit := range local.ModelCircuits {
		merged.ModelCircuits[model] = mergeGeminiAdaptiveCircuitState(merged.ModelCircuits[model], circuit)
	}
	merged.AccountCircuit = mergeGeminiAdaptiveCircuitState(merged.AccountCircuit, local.AccountCircuit)
	localHasFailure := local.ConsecutiveFailure > 0 || local.ConsecutiveCapacityFailure > 0 || local.RecentHealthFailures > 0 || local.RecentCapacityFailures > 0 || local.AccountCircuit.ConsecutiveFailure > 0 || !local.AccountCircuit.OpenUntil.IsZero()
	if localHasFailure {
		if local.EstimatedCapacity > 0 && local.EstimatedCapacity < merged.EstimatedCapacity {
			merged.EstimatedCapacity = local.EstimatedCapacity
		}
		merged.PathSuccessEMA = math.Min(merged.PathSuccessEMA, local.PathSuccessEMA)
		merged.ConsecutiveFailure = max(merged.ConsecutiveFailure, local.ConsecutiveFailure)
		merged.ConsecutiveCapacityFailure = max(merged.ConsecutiveCapacityFailure, local.ConsecutiveCapacityFailure)
		merged.CooldownUntil = laterTime(merged.CooldownUntil, local.CooldownUntil)
	} else {
		merged.ConsecutiveSuccess += local.ConsecutiveSuccess
	}
	merged.revision = local.revision + 1
	merged.persistedRevision = incoming.persistedRevision
	s.accounts[incoming.AccountID] = &merged
	return true
}

func mergeGeminiAdaptiveCircuitState(persisted, local geminiAdaptiveCircuitState) geminiAdaptiveCircuitState {
	merged := persistedGeminiAdaptiveCircuit(persisted)
	merged.ConsecutiveFailure = max(merged.ConsecutiveFailure, local.ConsecutiveFailure)
	merged.OpenUntil = laterTime(merged.OpenUntil, local.OpenUntil)
	if local.ProbeInFlight {
		merged.ProbeInFlight = true
		merged.ProbeUntil = local.ProbeUntil
		merged.ProbeOwner = local.ProbeOwner
	}
	return merged
}
