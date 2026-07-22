package service

import (
	"math"
	"strings"
	"sync"
	"time"
)

const (
	anthropicAdaptiveSuccessEMAAlpha          = 0.05
	anthropicAdaptiveLatencyEMAAlpha          = 0.05
	anthropicAdaptiveCapacitySuccessThreshold = 0.97
	anthropicAdaptiveCapacityProbeThreshold   = 0.80
	anthropicAdaptiveCapacityFailureThreshold = 3
	anthropicAdaptiveMinCapacitySamples       = 30
	anthropicAdaptiveCapacityErrorThreshold   = 0.25
	anthropicAdaptiveLearningWindow           = 20 * time.Minute
	anthropicAdaptiveCooldown                 = 60 * time.Second
	anthropicAdaptiveSoftShrinkFactor         = 0.85
	anthropicAdaptiveHardShrinkFactor         = 0.60
)

type anthropicAdaptiveLatencyState struct {
	TTFTEMA    float64
	LatencyEMA float64
	Samples    int64
}

type anthropicAdaptiveAccountState struct {
	AccountID                  int64
	EstimatedCapacity          int
	SuccessEMA                 float64
	LatencyByModelFamily       map[string]anthropicAdaptiveLatencyState
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
}

type anthropicAdaptiveStateStore struct {
	mu       sync.RWMutex
	accounts map[int64]*anthropicAdaptiveAccountState
}

func newAnthropicAdaptiveStateStore() *anthropicAdaptiveStateStore {
	return &anthropicAdaptiveStateStore{accounts: make(map[int64]*anthropicAdaptiveAccountState)}
}

func defaultAnthropicAdaptiveAccountState(account *Account, now time.Time) anthropicAdaptiveAccountState {
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
		SuccessEMA:            0.5,
		LatencyByModelFamily:  make(map[string]anthropicAdaptiveLatencyState, 4),
		RecentWindowStartedAt: now,
	}
}

func cloneAnthropicAdaptiveAccountState(state *anthropicAdaptiveAccountState) anthropicAdaptiveAccountState {
	if state == nil {
		return anthropicAdaptiveAccountState{}
	}
	clone := *state
	clone.LatencyByModelFamily = make(map[string]anthropicAdaptiveLatencyState, len(state.LatencyByModelFamily))
	for key, value := range state.LatencyByModelFamily {
		clone.LatencyByModelFamily[key] = value
	}
	return clone
}

func (s *anthropicAdaptiveStateStore) snapshot(account *Account) anthropicAdaptiveAccountState {
	if account == nil {
		return anthropicAdaptiveAccountState{}
	}
	s.mu.RLock()
	state := s.accounts[account.ID]
	snapshot := cloneAnthropicAdaptiveAccountState(state)
	s.mu.RUnlock()
	if state == nil {
		return defaultAnthropicAdaptiveAccountState(account, time.Now())
	}
	if account.Concurrency <= 0 {
		snapshot.EstimatedCapacity = 0
	} else if snapshot.EstimatedCapacity <= 0 || snapshot.EstimatedCapacity > account.Concurrency {
		snapshot.EstimatedCapacity = account.Concurrency
	}
	return snapshot
}

func (s *anthropicAdaptiveStateStore) effectiveCapacity(account *Account) int {
	if account == nil || account.Concurrency <= 0 {
		return 0
	}
	state := s.snapshot(account)
	capacity := state.EstimatedCapacity
	if capacity <= 0 || capacity > account.Concurrency {
		capacity = account.Concurrency
	}
	return capacity
}

func (s *anthropicAdaptiveStateStore) observeLoad(account *Account, load *AccountLoadInfo, now time.Time) anthropicAdaptiveAccountState {
	if account == nil {
		return anthropicAdaptiveAccountState{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(account, now)
	s.resetWindowLocked(state, now)
	if account.Concurrency <= 0 || state.EstimatedCapacity >= account.Concurrency || state.CooldownUntil.After(now) {
		return cloneAnthropicAdaptiveAccountState(state)
	}
	loadHigh := false
	if load != nil {
		loadHigh = load.WaitingCount > 0
		if state.EstimatedCapacity > 0 {
			loadHigh = loadHigh || float64(load.CurrentConcurrency)/float64(state.EstimatedCapacity) >= anthropicAdaptiveCapacityProbeThreshold
		}
	}
	if loadHigh && state.SuccessEMA >= anthropicAdaptiveCapacitySuccessThreshold && state.ConsecutiveSuccess >= max(1, state.EstimatedCapacity) {
		state.EstimatedCapacity++
		if state.EstimatedCapacity > account.Concurrency {
			state.EstimatedCapacity = account.Concurrency
		}
		state.ConsecutiveSuccess = 0
	}
	return cloneAnthropicAdaptiveAccountState(state)
}

type AnthropicAdaptiveScheduleReport struct {
	Account        *Account
	RequestedModel string
	Success        bool
	HealthSample   bool
	CapacitySample bool
	FirstTokenMs   *int
	DurationMs     int64
	TerminalReason string
}

func (s *anthropicAdaptiveStateStore) report(report AnthropicAdaptiveScheduleReport, now time.Time) (capacityIncreased bool, capacityDecreased bool) {
	if report.Account == nil {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(report.Account, now)
	s.resetWindowLocked(state, now)

	if report.HealthSample {
		state.TotalSamples++
		state.RecentHealthSamples++
		if report.Success {
			state.SuccessEMA = updateAnthropicAdaptiveEMA(state.SuccessEMA, 1, anthropicAdaptiveSuccessEMAAlpha)
			state.ConsecutiveSuccess++
			state.ConsecutiveFailure = 0
			state.LastSuccessAt = now
		} else {
			state.SuccessEMA = updateAnthropicAdaptiveEMA(state.SuccessEMA, 0, anthropicAdaptiveSuccessEMAAlpha)
			state.ConsecutiveSuccess = 0
			state.ConsecutiveFailure++
			state.RecentHealthFailures++
			state.LastFailureAt = now
		}
	}

	if report.Success {
		s.observeLatencyLocked(state, report)
	}

	if report.CapacitySample && report.Account.Concurrency > 0 {
		state.RecentCapacitySamples++
		if report.Success {
			state.ConsecutiveCapacityFailure = 0
		} else {
			state.RecentCapacityFailures++
			state.ConsecutiveCapacityFailure++
			state.LastCapacityFailureAt = now
			if s.shouldShrinkLocked(state, now) {
				factor := anthropicAdaptiveSoftShrinkFactor
				if state.ConsecutiveCapacityFailure >= anthropicAdaptiveCapacityFailureThreshold*2 {
					factor = anthropicAdaptiveHardShrinkFactor
				}
				next := int(math.Floor(float64(state.EstimatedCapacity) * factor))
				if next < 1 {
					next = 1
				}
				if next < state.EstimatedCapacity {
					state.EstimatedCapacity = next
					state.CooldownUntil = now.Add(anthropicAdaptiveCooldown)
					capacityDecreased = true
				}
			}
		}
	}
	return false, capacityDecreased
}

func (s *anthropicAdaptiveStateStore) ensureLocked(account *Account, now time.Time) *anthropicAdaptiveAccountState {
	state := s.accounts[account.ID]
	if state == nil {
		initial := defaultAnthropicAdaptiveAccountState(account, now)
		state = &initial
		s.accounts[account.ID] = state
	}
	if account.Concurrency <= 0 {
		state.EstimatedCapacity = 0
	} else if state.EstimatedCapacity <= 0 || state.EstimatedCapacity > account.Concurrency {
		state.EstimatedCapacity = account.Concurrency
	}
	return state
}

func (s *anthropicAdaptiveStateStore) resetWindowLocked(state *anthropicAdaptiveAccountState, now time.Time) {
	if state.RecentWindowStartedAt.IsZero() || now.Sub(state.RecentWindowStartedAt) >= anthropicAdaptiveLearningWindow {
		state.RecentWindowStartedAt = now
		state.RecentHealthSamples = 0
		state.RecentHealthFailures = 0
		state.RecentCapacitySamples = 0
		state.RecentCapacityFailures = 0
	}
}

func (s *anthropicAdaptiveStateStore) shouldShrinkLocked(state *anthropicAdaptiveAccountState, now time.Time) bool {
	if state.EstimatedCapacity <= 1 || state.CooldownUntil.After(now) || state.ConsecutiveCapacityFailure < anthropicAdaptiveCapacityFailureThreshold || state.RecentCapacitySamples < anthropicAdaptiveMinCapacitySamples {
		return false
	}
	return float64(state.RecentCapacityFailures)/float64(state.RecentCapacitySamples) >= anthropicAdaptiveCapacityErrorThreshold
}

func (s *anthropicAdaptiveStateStore) observeLatencyLocked(state *anthropicAdaptiveAccountState, report AnthropicAdaptiveScheduleReport) {
	family := anthropicAdaptiveModelFamily(report.RequestedModel)
	latency := state.LatencyByModelFamily[family]
	if report.FirstTokenMs != nil && *report.FirstTokenMs >= 0 {
		latency.TTFTEMA = updateAnthropicAdaptiveEMA(latency.TTFTEMA, float64(*report.FirstTokenMs), anthropicAdaptiveLatencyEMAAlpha)
	}
	if report.DurationMs >= 0 {
		latency.LatencyEMA = updateAnthropicAdaptiveEMA(latency.LatencyEMA, float64(report.DurationMs), anthropicAdaptiveLatencyEMAAlpha)
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
