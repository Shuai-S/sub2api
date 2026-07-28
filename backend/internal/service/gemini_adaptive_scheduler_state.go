package service

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const geminiAdaptiveStateLocalMergeLimit = 20

type geminiAdaptiveModelState struct {
	SuccessEMA float64 `json:"success_ema"`
	TTFTEMA    float64 `json:"ttft_ema"`
	LatencyEMA float64 `json:"latency_ema"`
	Samples    int64   `json:"samples"`
	Failures   int64   `json:"failures"`
}

type geminiAdaptiveAccountState struct {
	AccountID                  int64
	EstimatedCapacity          int
	PathSuccessEMA             float64
	ByModelFamily              map[string]geminiAdaptiveModelState
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
	mu       sync.RWMutex
	accounts map[int64]*geminiAdaptiveAccountState
}

func newGeminiAdaptiveStateStore() *geminiAdaptiveStateStore {
	return &geminiAdaptiveStateStore{accounts: make(map[int64]*geminiAdaptiveAccountState)}
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
		RecentWindowStartedAt: now,
	}
}

func cloneGeminiAdaptiveAccountState(state *geminiAdaptiveAccountState) geminiAdaptiveAccountState {
	if state == nil {
		return geminiAdaptiveAccountState{}
	}
	clone := *state
	clone.ByModelFamily = cloneGeminiAdaptiveModelMap(state.ByModelFamily)
	return clone
}

func cloneGeminiAdaptiveModelMap(in map[string]geminiAdaptiveModelState) map[string]geminiAdaptiveModelState {
	out := make(map[string]geminiAdaptiveModelState, max(5, len(in)))
	for key, value := range in {
		out[key] = value
	}
	return out
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
	Account           *Account
	RequestedModel    string
	MappedModel       string
	UpstreamRequestID string
	Stream            bool
	Action            string
	Success           bool
	PathSample        bool
	ModelSample       bool
	CapacitySample    bool
	Synthetic         bool
	FirstTokenMs      *int
	DurationMs        int64
	TerminalReason    string
	ctx               context.Context
}

func (s *geminiAdaptiveStateStore) report(report GeminiAdaptiveScheduleReport, now time.Time, settings GeminiAdaptiveSchedulerSettings) (capacityIncreased bool, capacityDecreased bool) {
	if report.Account == nil || report.Synthetic || (!report.PathSample && !report.ModelSample && !report.CapacitySample) {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(report.Account, now, settings)
	s.resetWindowLocked(state, now, settings)
	defer touchGeminiAdaptiveAccountState(state, now)
	state.TotalSamples++

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
	localHasFailure := local.ConsecutiveFailure > 0 || local.ConsecutiveCapacityFailure > 0 || local.RecentHealthFailures > 0 || local.RecentCapacityFailures > 0
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
