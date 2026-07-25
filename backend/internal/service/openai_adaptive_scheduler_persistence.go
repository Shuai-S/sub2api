package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"
)

const (
	openAIAdaptiveStateSchemaVersion         = 1
	openAIAdaptiveStateRestoreScanSize       = 256
	openAIAdaptiveStateWriteBatchSize        = 256
	openAIAdaptiveStateLocalMergeSampleLimit = 20
)

const (
	openAIAdaptiveStateRetention = adaptiveStateRetention
	openAIAdaptiveStateHashTTL   = adaptiveStateHashTTL
)

type openAIAdaptivePersistedState struct {
	SchemaVersion  int    `json:"schema_version"`
	SourceInstance string `json:"source_instance,omitempty"`
	AccountID      int64  `json:"account_id"`
	UpdatedAtUnix  int64  `json:"updated_at"`

	EstimatedCapacity int     `json:"estimated_capacity"`
	SuccessEMA        float64 `json:"success_ema"`
	ErrorEMA          float64 `json:"error_ema"`
	LatencyEMA        float64 `json:"latency_ema"`
	TTFTEMA           float64 `json:"ttft_ema"`
	ThompsonAlpha     float64 `json:"thompson_alpha"`
	ThompsonBeta      float64 `json:"thompson_beta"`

	ConsecutiveSuccess         int   `json:"consecutive_success"`
	ConsecutiveFailure         int   `json:"consecutive_failure"`
	ConsecutiveCapacityFailure int   `json:"consecutive_capacity_failure"`
	TotalSamples               int64 `json:"total_samples"`
	RecentSamples              int   `json:"recent_samples"`
	RecentFailures             int   `json:"recent_failures"`

	LastSuccessAtUnix         int64 `json:"last_success_at,omitempty"`
	LastFailureAtUnix         int64 `json:"last_failure_at,omitempty"`
	RecentWindowStartedAtUnix int64 `json:"recent_window_started_at,omitempty"`
	LastCapacityFailureAtUnix int64 `json:"last_capacity_failure_at,omitempty"`
	CooldownUntilUnix         int64 `json:"cooldown_until,omitempty"`
}

type openAIAdaptiveDirtySnapshot struct {
	state    openAIAdaptiveAccountState
	revision uint64
}

type openAIAdaptiveStatePersistence struct {
	cache          AdaptiveSchedulerStateCache
	store          *openAIAdaptiveSchedulerStateStore
	sourceInstance string
	worker         *adaptiveStatePersistenceWorker
	now            func() time.Time
}

func newOpenAIAdaptiveStatePersistence(cache AdaptiveSchedulerStateCache, store *openAIAdaptiveSchedulerStateStore) *openAIAdaptiveStatePersistence {
	persistence := &openAIAdaptiveStatePersistence{
		cache:          cache,
		store:          store,
		sourceInstance: adaptiveStateInstanceID(),
		now:            time.Now,
	}
	persistence.worker = newAdaptiveStatePersistenceWorker(
		cache,
		adaptiveSchedulerStateNamespaceOpenAI,
		persistence.restoreOnce,
		persistence.flush,
		func() time.Time { return persistence.now() },
	)
	return persistence
}

func (p *openAIAdaptiveStatePersistence) Start() {
	if p == nil || p.store == nil {
		return
	}
	p.worker.Start()
}

func (p *openAIAdaptiveStatePersistence) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.worker.Stop(ctx)
}

func (p *openAIAdaptiveStatePersistence) restoreOnce(ctx context.Context) (restored, stale, invalid int, err error) {
	now := p.now()
	loaded := make(map[int64]openAIAdaptiveAccountState)
	var cursor uint64
	for {
		records, nextCursor, scanErr := p.cache.ScanAdaptiveSchedulerStates(ctx, adaptiveSchedulerStateNamespaceOpenAI, cursor, openAIAdaptiveStateRestoreScanSize)
		if scanErr != nil {
			return 0, 0, 0, scanErr
		}
		for _, record := range records {
			state, stateErr := decodeOpenAIAdaptivePersistedState(record.AccountID, record.Payload, now)
			if stateErr != nil {
				invalid++
				continue
			}
			if now.Sub(state.UpdatedAt) > openAIAdaptiveStateRetention {
				stale++
				continue
			}
			loaded[record.AccountID] = state
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	for _, state := range loaded {
		if p.store.restoreAtStartup(state, now) {
			restored++
		}
	}
	return restored, stale, invalid, nil
}

func (p *openAIAdaptiveStatePersistence) flush(ctx context.Context) error {
	now := p.now()
	dirty := p.store.dirtySnapshots(now, openAIAdaptiveStateRetention)
	for start := 0; start < len(dirty); start += openAIAdaptiveStateWriteBatchSize {
		end := min(start+openAIAdaptiveStateWriteBatchSize, len(dirty))
		batch := dirty[start:end]
		entries := make([]AdaptiveSchedulerStateCacheEntry, 0, len(batch))
		written := make([]openAIAdaptiveDirtySnapshot, 0, len(batch))
		for _, snapshot := range batch {
			payload, err := encodeOpenAIAdaptivePersistedState(snapshot.state, p.sourceInstance)
			if err != nil {
				slog.Warn("openai_adaptive_state_encode_failed", "account_id", snapshot.state.AccountID, "error", err)
				continue
			}
			entries = append(entries, AdaptiveSchedulerStateCacheEntry{
				AccountID: snapshot.state.AccountID,
				Payload:   payload,
				ExpiresAt: snapshot.state.UpdatedAt.Add(openAIAdaptiveStateRetention),
			})
			written = append(written, snapshot)
		}
		if len(entries) == 0 {
			continue
		}
		if err := p.cache.SaveAdaptiveSchedulerStates(ctx, adaptiveSchedulerStateNamespaceOpenAI, entries, openAIAdaptiveStateHashTTL); err != nil {
			return err
		}
		p.store.markPersisted(written)
	}
	return nil
}

func encodeOpenAIAdaptivePersistedState(state openAIAdaptiveAccountState, sourceInstance string) ([]byte, error) {
	if err := validateOpenAIAdaptiveAccountState(state); err != nil {
		return nil, err
	}
	persisted := openAIAdaptivePersistedState{
		SchemaVersion:              openAIAdaptiveStateSchemaVersion,
		SourceInstance:             sourceInstance,
		AccountID:                  state.AccountID,
		UpdatedAtUnix:              state.UpdatedAt.UnixMilli(),
		EstimatedCapacity:          state.EstimatedCapacity,
		SuccessEMA:                 state.SuccessEMA,
		ErrorEMA:                   state.ErrorEMA,
		LatencyEMA:                 state.LatencyEMA,
		TTFTEMA:                    state.TTFTEMA,
		ThompsonAlpha:              state.ThompsonAlpha,
		ThompsonBeta:               state.ThompsonBeta,
		ConsecutiveSuccess:         state.ConsecutiveSuccess,
		ConsecutiveFailure:         state.ConsecutiveFailure,
		ConsecutiveCapacityFailure: state.ConsecutiveCapacityFailure,
		TotalSamples:               state.TotalSamples,
		RecentSamples:              state.RecentSamples,
		RecentFailures:             state.RecentFailures,
		LastSuccessAtUnix:          unixMilliOrZero(state.LastSuccessAt),
		LastFailureAtUnix:          unixMilliOrZero(state.LastFailureAt),
		RecentWindowStartedAtUnix:  unixMilliOrZero(state.RecentWindowStartedAt),
		LastCapacityFailureAtUnix:  unixMilliOrZero(state.LastCapacityFailureAt),
		CooldownUntilUnix:          unixMilliOrZero(state.CooldownUntil),
	}
	return json.Marshal(persisted)
}

func decodeOpenAIAdaptivePersistedState(accountID int64, payload []byte, now time.Time) (openAIAdaptiveAccountState, error) {
	var persisted openAIAdaptivePersistedState
	if err := json.Unmarshal(payload, &persisted); err != nil {
		return openAIAdaptiveAccountState{}, err
	}
	if persisted.SchemaVersion != openAIAdaptiveStateSchemaVersion {
		return openAIAdaptiveAccountState{}, fmt.Errorf("unsupported schema version %d", persisted.SchemaVersion)
	}
	if accountID <= 0 || persisted.AccountID != accountID {
		return openAIAdaptiveAccountState{}, fmt.Errorf("account id mismatch: field=%d payload=%d", accountID, persisted.AccountID)
	}
	state := openAIAdaptiveAccountState{
		AccountID:                  accountID,
		EstimatedCapacity:          persisted.EstimatedCapacity,
		SuccessEMA:                 persisted.SuccessEMA,
		ErrorEMA:                   persisted.ErrorEMA,
		LatencyEMA:                 persisted.LatencyEMA,
		TTFTEMA:                    persisted.TTFTEMA,
		ThompsonAlpha:              persisted.ThompsonAlpha,
		ThompsonBeta:               persisted.ThompsonBeta,
		ConsecutiveSuccess:         persisted.ConsecutiveSuccess,
		ConsecutiveFailure:         persisted.ConsecutiveFailure,
		ConsecutiveCapacityFailure: persisted.ConsecutiveCapacityFailure,
		TotalSamples:               persisted.TotalSamples,
		RecentSamples:              persisted.RecentSamples,
		RecentFailures:             persisted.RecentFailures,
		LastSuccessAt:              timeFromUnixMilli(persisted.LastSuccessAtUnix),
		LastFailureAt:              timeFromUnixMilli(persisted.LastFailureAtUnix),
		RecentWindowStartedAt:      timeFromUnixMilli(persisted.RecentWindowStartedAtUnix),
		LastCapacityFailureAt:      timeFromUnixMilli(persisted.LastCapacityFailureAtUnix),
		CooldownUntil:              timeFromUnixMilli(persisted.CooldownUntilUnix),
		UpdatedAt:                  timeFromUnixMilli(persisted.UpdatedAtUnix),
	}
	if state.UpdatedAt.After(now.Add(5 * time.Minute)) {
		return openAIAdaptiveAccountState{}, fmt.Errorf("updated_at is too far in the future")
	}
	if !state.CooldownUntil.After(now) {
		state.CooldownUntil = time.Time{}
	}
	if err := validateOpenAIAdaptiveAccountState(state); err != nil {
		return openAIAdaptiveAccountState{}, err
	}
	return state, nil
}

func validateOpenAIAdaptiveAccountState(state openAIAdaptiveAccountState) error {
	if state.AccountID <= 0 || state.EstimatedCapacity <= 0 || state.UpdatedAt.IsZero() {
		return fmt.Errorf("invalid account identity, capacity, or updated_at")
	}
	if !finiteInRange(state.SuccessEMA, 0, 1) || !finiteInRange(state.ErrorEMA, 0, 1) {
		return fmt.Errorf("invalid success or error EMA")
	}
	if !finiteNonNegative(state.LatencyEMA) || !finiteNonNegative(state.TTFTEMA) {
		return fmt.Errorf("invalid latency EMA")
	}
	if !finitePositive(state.ThompsonAlpha) || !finitePositive(state.ThompsonBeta) {
		return fmt.Errorf("invalid Thompson parameters")
	}
	if state.ConsecutiveSuccess < 0 || state.ConsecutiveFailure < 0 || state.ConsecutiveCapacityFailure < 0 ||
		state.TotalSamples < 0 || state.RecentSamples < 0 || state.RecentFailures < 0 || state.RecentFailures > state.RecentSamples {
		return fmt.Errorf("invalid sample counters")
	}
	return nil
}

func finiteInRange(value, minValue, maxValue float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minValue && value <= maxValue
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func finitePositive(value float64) bool {
	return finiteNonNegative(value) && value > 0
}

func unixMilliOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func timeFromUnixMilli(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

func (s *openAIAdaptiveSchedulerStateStore) dirtySnapshots(now time.Time, retention time.Duration) []openAIAdaptiveDirtySnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	out := make([]openAIAdaptiveDirtySnapshot, 0, len(s.states))
	for _, state := range s.states {
		if state == nil || state.revision <= state.persistedRevision || state.UpdatedAt.IsZero() {
			continue
		}
		if retention > 0 && now.Sub(state.UpdatedAt) > retention {
			continue
		}
		out = append(out, openAIAdaptiveDirtySnapshot{state: *state, revision: state.revision})
	}
	s.mu.RUnlock()
	return out
}

func (s *openAIAdaptiveSchedulerStateStore) markPersisted(snapshots []openAIAdaptiveDirtySnapshot) {
	if s == nil || len(snapshots) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, snapshot := range snapshots {
		state := s.states[snapshot.state.AccountID]
		if state == nil {
			continue
		}
		if snapshot.revision > state.persistedRevision {
			state.persistedRevision = snapshot.revision
		}
	}
}

func (s *openAIAdaptiveSchedulerStateStore) restoreAtStartup(incoming openAIAdaptiveAccountState, now time.Time) bool {
	if s == nil || incoming.AccountID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	incoming.revision = 1
	incoming.persistedRevision = 1
	local := s.states[incoming.AccountID]
	if local == nil {
		restored := incoming
		s.states[incoming.AccountID] = &restored
		return true
	}
	localHasFailure := local.ConsecutiveFailure > 0 || local.ConsecutiveCapacityFailure > 0 ||
		local.RecentFailures > 0 || !local.LastFailureAt.IsZero() || local.CooldownUntil.After(now)
	if local.TotalSamples == 0 && !localHasFailure {
		restored := incoming
		s.states[incoming.AccountID] = &restored
		return true
	}
	if local.TotalSamples >= openAIAdaptiveStateLocalMergeSampleLimit {
		return false
	}

	merged := incoming
	merged.TotalSamples += local.TotalSamples
	merged.RecentSamples += local.RecentSamples
	merged.RecentFailures += local.RecentFailures
	merged.UpdatedAt = laterTime(incoming.UpdatedAt, local.UpdatedAt)
	merged.LastSuccessAt = laterTime(incoming.LastSuccessAt, local.LastSuccessAt)
	merged.LastFailureAt = laterTime(incoming.LastFailureAt, local.LastFailureAt)
	merged.LastCapacityFailureAt = laterTime(incoming.LastCapacityFailureAt, local.LastCapacityFailureAt)
	if localHasFailure {
		if local.EstimatedCapacity > 0 && local.EstimatedCapacity < merged.EstimatedCapacity {
			merged.EstimatedCapacity = local.EstimatedCapacity
		}
		merged.SuccessEMA = math.Min(merged.SuccessEMA, local.SuccessEMA)
		merged.ErrorEMA = math.Max(merged.ErrorEMA, local.ErrorEMA)
		merged.ConsecutiveFailure = max(merged.ConsecutiveFailure, local.ConsecutiveFailure)
		merged.ConsecutiveCapacityFailure = max(merged.ConsecutiveCapacityFailure, local.ConsecutiveCapacityFailure)
		merged.CooldownUntil = laterTime(merged.CooldownUntil, local.CooldownUntil)
	} else {
		merged.ConsecutiveSuccess += local.ConsecutiveSuccess
	}
	merged.ThompsonAlpha = math.Max(merged.ThompsonAlpha, local.ThompsonAlpha)
	merged.ThompsonBeta = math.Max(merged.ThompsonBeta, local.ThompsonBeta)
	merged.revision = local.revision + 1
	merged.persistedRevision = incoming.persistedRevision
	s.states[incoming.AccountID] = &merged
	return true
}

func laterTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func (s *OpenAIGatewayService) startOpenAIAdaptiveStatePersistence() {
	if s == nil {
		return
	}
	s.openaiAdaptivePersistenceOnce.Do(func() {
		cache, ok := s.cache.(AdaptiveSchedulerStateCache)
		if !ok || cache == nil {
			return
		}
		if s.openaiAdaptiveState == nil {
			s.openaiAdaptiveState = newOpenAIAdaptiveSchedulerStateStore()
		}
		s.openaiAdaptivePersistence = newOpenAIAdaptiveStatePersistence(cache, s.openaiAdaptiveState)
		s.openaiAdaptivePersistence.Start()
	})
}

func (s *OpenAIGatewayService) CloseOpenAIAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil || s.openaiAdaptivePersistence == nil {
		return nil
	}
	return s.openaiAdaptivePersistence.Stop(ctx)
}
