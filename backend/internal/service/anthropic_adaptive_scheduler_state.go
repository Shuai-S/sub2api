package service

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	anthropicAdaptiveCircuitFailureThreshold = 3
	anthropicAdaptiveCircuitProbeLease       = 30 * time.Second
	anthropicAdaptiveFailureSampleRetention  = 10 * time.Minute
)

type anthropicAdaptiveLatencyState struct {
	TTFTEMA    float64 `json:"ttft_ema"`
	LatencyEMA float64 `json:"latency_ema"`
	Samples    int64   `json:"samples"`
}

type anthropicAdaptiveHealthState struct {
	SuccessEMA         float64 `json:"success_ema"`
	ConsecutiveFailure int     `json:"consecutive_failure"`
	TotalSamples       int64   `json:"total_samples"`
}

type anthropicAdaptiveAccountState struct {
	AccountID                  int64
	EstimatedCapacity          int
	SuccessEMA                 float64
	HealthByModelFamily        map[string]anthropicAdaptiveHealthState
	LatencyByModelFamily       map[string]anthropicAdaptiveLatencyState
	ConsecutiveSuccess         int
	ConsecutiveFailure         int
	ConsecutiveCapacityFailure int
	AccountHealthSamples       int
	AccountHealthFailures      int
	AccountConsecutiveFailure  int
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
	CircuitOpenUntil           time.Time
	CircuitProbeUntil          time.Time
	CircuitProbeInFlight       bool
	UpdatedAt                  time.Time

	revision          uint64
	persistedRevision uint64
}

type anthropicAdaptiveStateStore struct {
	mu                  sync.RWMutex
	accounts            map[int64]*anthropicAdaptiveAccountState
	failureSampleClaims map[string]time.Time
}

func newAnthropicAdaptiveStateStore() *anthropicAdaptiveStateStore {
	return &anthropicAdaptiveStateStore{
		accounts:            make(map[int64]*anthropicAdaptiveAccountState),
		failureSampleClaims: make(map[string]time.Time),
	}
}

func defaultAnthropicAdaptiveAccountState(account *Account, now time.Time, settings AnthropicAdaptiveSchedulerSettings) anthropicAdaptiveAccountState {
	capacity := 0
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
		if account.Concurrency > 0 {
			capacity = account.Concurrency
		}
	}
	return anthropicAdaptiveAccountState{
		AccountID:             accountID,
		EstimatedCapacity:     capacity,
		SuccessEMA:            settings.AnthropicAdaptiveSchedulerInitialReliability,
		HealthByModelFamily:   make(map[string]anthropicAdaptiveHealthState, 4),
		LatencyByModelFamily:  make(map[string]anthropicAdaptiveLatencyState, 4),
		RecentWindowStartedAt: now,
	}
}

func cloneAnthropicAdaptiveAccountState(state *anthropicAdaptiveAccountState) anthropicAdaptiveAccountState {
	if state == nil {
		return anthropicAdaptiveAccountState{}
	}
	clone := *state
	clone.HealthByModelFamily = make(map[string]anthropicAdaptiveHealthState, len(state.HealthByModelFamily))
	for key, value := range state.HealthByModelFamily {
		clone.HealthByModelFamily[key] = value
	}
	clone.LatencyByModelFamily = make(map[string]anthropicAdaptiveLatencyState, len(state.LatencyByModelFamily))
	for key, value := range state.LatencyByModelFamily {
		clone.LatencyByModelFamily[key] = value
	}
	return clone
}

func (s *anthropicAdaptiveStateStore) snapshot(account *Account, settings AnthropicAdaptiveSchedulerSettings) anthropicAdaptiveAccountState {
	if account == nil {
		return anthropicAdaptiveAccountState{}
	}
	s.mu.RLock()
	state := s.accounts[account.ID]
	snapshot := cloneAnthropicAdaptiveAccountState(state)
	s.mu.RUnlock()
	if state == nil {
		return defaultAnthropicAdaptiveAccountState(account, time.Now(), settings)
	}
	if account.Concurrency <= 0 {
		snapshot.EstimatedCapacity = 0
	} else if snapshot.EstimatedCapacity <= 0 || snapshot.EstimatedCapacity > account.Concurrency {
		snapshot.EstimatedCapacity = account.Concurrency
	}
	return snapshot
}

func (s *anthropicAdaptiveStateStore) effectiveCapacity(account *Account, settings AnthropicAdaptiveSchedulerSettings) int {
	if account == nil || account.Concurrency <= 0 {
		return 0
	}
	state := s.snapshot(account, settings)
	capacity := state.EstimatedCapacity
	if capacity <= 0 || capacity > account.Concurrency {
		capacity = account.Concurrency
	}
	return capacity
}

// claimCircuitProbe excludes an account while its circuit is open, except for
// one short-lived half-open probe. The lease prevents concurrent requests from
// stampeding a known-bad account and allows recovery if a result is lost.
func (s *anthropicAdaptiveStateStore) claimCircuitProbe(account *Account, now time.Time, settings AnthropicAdaptiveSchedulerSettings) bool {
	if s == nil || account == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(account, now, settings)
	if state.CircuitOpenUntil.IsZero() {
		return true
	}
	if state.CircuitProbeInFlight && state.CircuitProbeUntil.After(now) {
		return false
	}
	if state.CircuitOpenUntil.After(now) {
		return false
	}
	state.CircuitProbeInFlight = true
	state.CircuitProbeUntil = now.Add(anthropicAdaptiveCircuitProbeLease)
	touchAnthropicAdaptiveAccountState(state, now)
	return true
}

// claimFailureSample makes a same-account retry burst count as one health
// observation. Different accounts in the same request still produce separate
// samples, as they represent independent upstream paths.
func (s *anthropicAdaptiveStateStore) claimFailureSample(accountID int64, requestID, requestedModel string, now time.Time) bool {
	if s == nil || accountID <= 0 || strings.TrimSpace(requestID) == "" {
		return true
	}
	key := strconv.FormatInt(accountID, 10) + ":" + requestID + ":" + anthropicAdaptiveModelFamily(requestedModel)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failureSampleClaims == nil {
		s.failureSampleClaims = make(map[string]time.Time)
	}
	for existingKey, claimedAt := range s.failureSampleClaims {
		if now.Sub(claimedAt) > anthropicAdaptiveFailureSampleRetention {
			delete(s.failureSampleClaims, existingKey)
		}
	}
	if _, exists := s.failureSampleClaims[key]; exists {
		return false
	}
	s.failureSampleClaims[key] = now
	return true
}

func (s *anthropicAdaptiveStateStore) observeLoad(account *Account, load *AccountLoadInfo, now time.Time, settings AnthropicAdaptiveSchedulerSettings) anthropicAdaptiveAccountState {
	if account == nil {
		return anthropicAdaptiveAccountState{}
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
			touchAnthropicAdaptiveAccountState(state, now)
		}
	}()
	if account.Concurrency <= 0 || state.EstimatedCapacity >= account.Concurrency || state.CooldownUntil.After(now) {
		return cloneAnthropicAdaptiveAccountState(state)
	}
	loadHigh := false
	if load != nil {
		loadHigh = load.WaitingCount > 0
		if state.EstimatedCapacity > 0 {
			loadHigh = loadHigh || float64(load.CurrentConcurrency)/float64(state.EstimatedCapacity) >= settings.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold
		}
	}
	if loadHigh && state.SuccessEMA >= settings.AnthropicAdaptiveSchedulerCapacitySuccessThreshold && state.ConsecutiveSuccess >= max(1, state.EstimatedCapacity) {
		state.EstimatedCapacity += settings.AnthropicAdaptiveSchedulerCapacityIncreaseStep
		if state.EstimatedCapacity > account.Concurrency {
			state.EstimatedCapacity = account.Concurrency
		}
		state.ConsecutiveSuccess = 0
		changed = true
	}
	return cloneAnthropicAdaptiveAccountState(state)
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
	CapacitySample    bool
	HealthScope       string
	FirstTokenMs      *int
	DurationMs        int64
	TerminalReason    string
}

func (s *anthropicAdaptiveStateStore) report(report AnthropicAdaptiveScheduleReport, now time.Time, settings AnthropicAdaptiveSchedulerSettings) (capacityIncreased bool, capacityDecreased bool) {
	if report.Account == nil || (!report.HealthSample && !report.CapacitySample && !report.Success) {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(report.Account, now, settings)
	s.resetWindowLocked(state, now, settings)
	defer touchAnthropicAdaptiveAccountState(state, now)

	if report.HealthSample {
		state.TotalSamples++
		accountScoped := report.HealthScope != "model"
		if accountScoped {
			state.RecentHealthSamples++
			if report.Success {
				state.SuccessEMA = updateAnthropicAdaptiveEMA(state.SuccessEMA, 1, settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
				state.ConsecutiveSuccess++
				state.ConsecutiveFailure = 0
				state.LastSuccessAt = now
			} else {
				state.SuccessEMA = updateAnthropicAdaptiveEMA(state.SuccessEMA, 0, settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
				state.ConsecutiveSuccess = 0
				state.ConsecutiveFailure++
				state.RecentHealthFailures++
				state.LastFailureAt = now
			}
		}

		family := anthropicAdaptiveModelFamily(report.RequestedModel)
		modelHealth := state.HealthByModelFamily[family]
		if report.Success {
			modelHealth.SuccessEMA = updateAnthropicAdaptiveEMA(modelHealth.SuccessEMA, 1, settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
			modelHealth.ConsecutiveFailure = 0
		} else {
			modelHealth.SuccessEMA = updateAnthropicAdaptiveEMA(modelHealth.SuccessEMA, 0, settings.AnthropicAdaptiveSchedulerSuccessEMAAlpha)
			modelHealth.ConsecutiveFailure++
		}
		modelHealth.TotalSamples++
		state.HealthByModelFamily[family] = modelHealth

		if accountScoped {
			state.AccountHealthSamples++
			if report.Success {
				state.AccountConsecutiveFailure = 0
				state.CircuitOpenUntil = time.Time{}
				state.CircuitProbeUntil = time.Time{}
				state.CircuitProbeInFlight = false
			} else {
				state.AccountHealthFailures++
				state.AccountConsecutiveFailure++
				if state.AccountConsecutiveFailure >= anthropicAdaptiveCircuitFailureThreshold {
					cooldown := time.Duration(settings.AnthropicAdaptiveSchedulerCooldownSeconds) * time.Second
					if cooldown <= 0 {
						cooldown = time.Minute
					}
					state.CircuitOpenUntil = now.Add(cooldown)
					state.CircuitProbeUntil = time.Time{}
					state.CircuitProbeInFlight = false
				}
			}
		} else if report.Success && state.CircuitProbeInFlight {
			// A successful half-open probe closes an account circuit even when
			// the current model sample is model-scoped.
			state.CircuitOpenUntil = time.Time{}
			state.CircuitProbeUntil = time.Time{}
			state.CircuitProbeInFlight = false
		}
	}

	if report.Success {
		s.observeLatencyLocked(state, report, settings)
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
				factor := settings.AnthropicAdaptiveSchedulerShrinkFactorSoft
				if state.ConsecutiveCapacityFailure >= settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold*settings.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier {
					factor = settings.AnthropicAdaptiveSchedulerShrinkFactorHard
				}
				next := int(math.Floor(float64(state.EstimatedCapacity) * factor))
				minCapacity := min(settings.AnthropicAdaptiveSchedulerMinCapacity, report.Account.Concurrency)
				if next < minCapacity {
					next = minCapacity
				}
				if next < state.EstimatedCapacity {
					state.EstimatedCapacity = next
					state.CooldownUntil = now.Add(time.Duration(settings.AnthropicAdaptiveSchedulerCooldownSeconds) * time.Second)
					capacityDecreased = true
				}
			}
		}
	}
	return false, capacityDecreased
}

func (s *anthropicAdaptiveStateStore) ensureLocked(account *Account, now time.Time, settings AnthropicAdaptiveSchedulerSettings) *anthropicAdaptiveAccountState {
	state := s.accounts[account.ID]
	if state == nil {
		initial := defaultAnthropicAdaptiveAccountState(account, now, settings)
		state = &initial
		s.accounts[account.ID] = state
	}
	if state.HealthByModelFamily == nil {
		state.HealthByModelFamily = make(map[string]anthropicAdaptiveHealthState, 4)
	}
	if state.LatencyByModelFamily == nil {
		state.LatencyByModelFamily = make(map[string]anthropicAdaptiveLatencyState, 4)
	}
	if account.Concurrency <= 0 {
		state.EstimatedCapacity = 0
	} else if state.EstimatedCapacity <= 0 || state.EstimatedCapacity > account.Concurrency {
		state.EstimatedCapacity = account.Concurrency
	} else if minCapacity := min(settings.AnthropicAdaptiveSchedulerMinCapacity, account.Concurrency); state.EstimatedCapacity < minCapacity {
		state.EstimatedCapacity = minCapacity
	}
	return state
}

func (s *anthropicAdaptiveStateStore) resetWindowLocked(state *anthropicAdaptiveAccountState, now time.Time, settings AnthropicAdaptiveSchedulerSettings) bool {
	learningWindow := time.Duration(settings.AnthropicAdaptiveSchedulerLearningWindowSeconds) * time.Second
	if state.RecentWindowStartedAt.IsZero() || now.Sub(state.RecentWindowStartedAt) >= learningWindow {
		state.RecentWindowStartedAt = now
		state.RecentHealthSamples = 0
		state.RecentHealthFailures = 0
		state.RecentCapacitySamples = 0
		state.RecentCapacityFailures = 0
		return true
	}
	return false
}

func touchAnthropicAdaptiveAccountState(state *anthropicAdaptiveAccountState, now time.Time) {
	if state == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.UpdatedAt = now
	state.revision++
}

func (s *anthropicAdaptiveStateStore) shouldShrinkLocked(state *anthropicAdaptiveAccountState, now time.Time, settings AnthropicAdaptiveSchedulerSettings) bool {
	if state.EstimatedCapacity <= settings.AnthropicAdaptiveSchedulerMinCapacity || state.CooldownUntil.After(now) || state.ConsecutiveCapacityFailure < settings.AnthropicAdaptiveSchedulerCapacityFailureThreshold || state.RecentCapacitySamples < settings.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink {
		return false
	}
	return float64(state.RecentCapacityFailures)/float64(state.RecentCapacitySamples) >= settings.AnthropicAdaptiveSchedulerShrinkErrorThreshold
}

func (s *anthropicAdaptiveStateStore) observeLatencyLocked(state *anthropicAdaptiveAccountState, report AnthropicAdaptiveScheduleReport, settings AnthropicAdaptiveSchedulerSettings) {
	family := anthropicAdaptiveModelFamily(report.RequestedModel)
	latency := state.LatencyByModelFamily[family]
	if report.FirstTokenMs != nil && *report.FirstTokenMs >= 0 {
		latency.TTFTEMA = updateAnthropicAdaptiveEMA(latency.TTFTEMA, float64(*report.FirstTokenMs), settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha)
	}
	if report.DurationMs >= 0 {
		latency.LatencyEMA = updateAnthropicAdaptiveEMA(latency.LatencyEMA, float64(report.DurationMs), settings.AnthropicAdaptiveSchedulerLatencyEMAAlpha)
	}
	latency.Samples++
	state.LatencyByModelFamily[family] = latency
}

func updateAnthropicAdaptiveEMA(current, sample, alpha float64) float64 {
	if current <= 0 {
		return sample
	}
	return alpha*sample + (1-alpha)*current
}

func anthropicAdaptiveModelFamily(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	case strings.Contains(model, "haiku"):
		return "haiku"
	default:
		return "other"
	}
}
