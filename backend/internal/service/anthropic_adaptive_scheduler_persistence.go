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
	anthropicAdaptiveStateSchemaVersion   = 1
	anthropicAdaptiveStateRestoreScanSize = 256
	anthropicAdaptiveStateWriteBatchSize  = 256
	anthropicAdaptiveStateLocalMergeLimit = 20
)

type anthropicAdaptivePersistedState struct {
	SchemaVersion  int    `json:"schema_version"`
	SourceInstance string `json:"source_instance,omitempty"`
	AccountID      int64  `json:"account_id"`
	UpdatedAtUnix  int64  `json:"updated_at"`

	EstimatedCapacity    int                                      `json:"estimated_capacity"`
	SuccessEMA           float64                                  `json:"success_ema"`
	LatencyByModelFamily map[string]anthropicAdaptiveLatencyState `json:"latency_by_model_family,omitempty"`

	ConsecutiveSuccess         int   `json:"consecutive_success"`
	ConsecutiveFailure         int   `json:"consecutive_failure"`
	ConsecutiveCapacityFailure int   `json:"consecutive_capacity_failure"`
	TotalSamples               int64 `json:"total_samples"`
	RecentHealthSamples        int   `json:"recent_health_samples"`
	RecentHealthFailures       int   `json:"recent_health_failures"`
	RecentCapacitySamples      int   `json:"recent_capacity_samples"`
	RecentCapacityFailures     int   `json:"recent_capacity_failures"`

	LastSuccessAtUnix         int64 `json:"last_success_at,omitempty"`
	LastFailureAtUnix         int64 `json:"last_failure_at,omitempty"`
	LastCapacityFailureAtUnix int64 `json:"last_capacity_failure_at,omitempty"`
	RecentWindowStartedAtUnix int64 `json:"recent_window_started_at,omitempty"`
	CooldownUntilUnix         int64 `json:"cooldown_until,omitempty"`
}

type anthropicAdaptiveDirtySnapshot struct {
	state    anthropicAdaptiveAccountState
	revision uint64
}

type anthropicAdaptiveStatePersistence struct {
	cache            AdaptiveSchedulerStateCache
	store            *anthropicAdaptiveStateStore
	sourceInstance   string
	settingsResolver func(context.Context) AnthropicAdaptiveSchedulerSettings
	worker           *adaptiveStatePersistenceWorker
	now              func() time.Time
}

func newAnthropicAdaptiveStatePersistence(
	cache AdaptiveSchedulerStateCache,
	store *anthropicAdaptiveStateStore,
	settingsResolver func(context.Context) AnthropicAdaptiveSchedulerSettings,
) *anthropicAdaptiveStatePersistence {
	persistence := &anthropicAdaptiveStatePersistence{
		cache:            cache,
		store:            store,
		sourceInstance:   adaptiveStateInstanceID(),
		settingsResolver: settingsResolver,
		now:              time.Now,
	}
	persistence.worker = newAdaptiveStatePersistenceWorker(
		cache,
		adaptiveSchedulerStateNamespaceAnthropic,
		persistence.restoreOnce,
		persistence.flush,
		func() time.Time { return persistence.now() },
	)
	return persistence
}

func (p *anthropicAdaptiveStatePersistence) Start() {
	if p == nil || p.store == nil {
		return
	}
	p.worker.Start()
}

func (p *anthropicAdaptiveStatePersistence) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.worker.Stop(ctx)
}

func (p *anthropicAdaptiveStatePersistence) restoreOnce(ctx context.Context) (restored, stale, invalid int, err error) {
	now := p.now()
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	if p.settingsResolver != nil {
		settings = NormalizeAnthropicAdaptiveSchedulerSettings(p.settingsResolver(ctx))
	}
	loaded := make(map[int64]anthropicAdaptiveAccountState)
	var cursor uint64
	for {
		records, nextCursor, scanErr := p.cache.ScanAdaptiveSchedulerStates(
			ctx,
			adaptiveSchedulerStateNamespaceAnthropic,
			cursor,
			anthropicAdaptiveStateRestoreScanSize,
		)
		if scanErr != nil {
			return 0, 0, 0, scanErr
		}
		for _, record := range records {
			state, stateErr := decodeAnthropicAdaptivePersistedState(record.AccountID, record.Payload, now)
			if stateErr != nil {
				invalid++
				continue
			}
			if now.Sub(state.UpdatedAt) > adaptiveStateRetention {
				stale++
				continue
			}
			p.store.resetWindowLocked(&state, now, settings)
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

func (p *anthropicAdaptiveStatePersistence) flush(ctx context.Context) error {
	now := p.now()
	dirty := p.store.dirtySnapshots(now, adaptiveStateRetention)
	for start := 0; start < len(dirty); start += anthropicAdaptiveStateWriteBatchSize {
		end := min(start+anthropicAdaptiveStateWriteBatchSize, len(dirty))
		batch := dirty[start:end]
		entries := make([]AdaptiveSchedulerStateCacheEntry, 0, len(batch))
		written := make([]anthropicAdaptiveDirtySnapshot, 0, len(batch))
		for _, snapshot := range batch {
			payload, err := encodeAnthropicAdaptivePersistedState(snapshot.state, p.sourceInstance)
			if err != nil {
				slog.Warn("anthropic_adaptive_state_encode_failed", "account_id", snapshot.state.AccountID, "error", err)
				continue
			}
			entries = append(entries, AdaptiveSchedulerStateCacheEntry{
				AccountID: snapshot.state.AccountID,
				Payload:   payload,
				ExpiresAt: snapshot.state.UpdatedAt.Add(adaptiveStateRetention),
			})
			written = append(written, snapshot)
		}
		if len(entries) == 0 {
			continue
		}
		if err := p.cache.SaveAdaptiveSchedulerStates(
			ctx,
			adaptiveSchedulerStateNamespaceAnthropic,
			entries,
			adaptiveStateHashTTL,
		); err != nil {
			return err
		}
		p.store.markPersisted(written)
	}
	return nil
}

func encodeAnthropicAdaptivePersistedState(state anthropicAdaptiveAccountState, sourceInstance string) ([]byte, error) {
	if err := validateAnthropicAdaptiveAccountState(state); err != nil {
		return nil, err
	}
	persisted := anthropicAdaptivePersistedState{
		SchemaVersion:              anthropicAdaptiveStateSchemaVersion,
		SourceInstance:             sourceInstance,
		AccountID:                  state.AccountID,
		UpdatedAtUnix:              state.UpdatedAt.UnixMilli(),
		EstimatedCapacity:          state.EstimatedCapacity,
		SuccessEMA:                 state.SuccessEMA,
		LatencyByModelFamily:       cloneAnthropicAdaptiveLatencyMap(state.LatencyByModelFamily),
		ConsecutiveSuccess:         state.ConsecutiveSuccess,
		ConsecutiveFailure:         state.ConsecutiveFailure,
		ConsecutiveCapacityFailure: state.ConsecutiveCapacityFailure,
		TotalSamples:               state.TotalSamples,
		RecentHealthSamples:        state.RecentHealthSamples,
		RecentHealthFailures:       state.RecentHealthFailures,
		RecentCapacitySamples:      state.RecentCapacitySamples,
		RecentCapacityFailures:     state.RecentCapacityFailures,
		LastSuccessAtUnix:          unixMilliOrZero(state.LastSuccessAt),
		LastFailureAtUnix:          unixMilliOrZero(state.LastFailureAt),
		LastCapacityFailureAtUnix:  unixMilliOrZero(state.LastCapacityFailureAt),
		RecentWindowStartedAtUnix:  unixMilliOrZero(state.RecentWindowStartedAt),
		CooldownUntilUnix:          unixMilliOrZero(state.CooldownUntil),
	}
	return json.Marshal(persisted)
}

func decodeAnthropicAdaptivePersistedState(accountID int64, payload []byte, now time.Time) (anthropicAdaptiveAccountState, error) {
	var persisted anthropicAdaptivePersistedState
	if err := json.Unmarshal(payload, &persisted); err != nil {
		return anthropicAdaptiveAccountState{}, err
	}
	if persisted.SchemaVersion != anthropicAdaptiveStateSchemaVersion {
		return anthropicAdaptiveAccountState{}, fmt.Errorf("unsupported schema version %d", persisted.SchemaVersion)
	}
	if accountID <= 0 || persisted.AccountID != accountID {
		return anthropicAdaptiveAccountState{}, fmt.Errorf("account id mismatch: field=%d payload=%d", accountID, persisted.AccountID)
	}
	state := anthropicAdaptiveAccountState{
		AccountID:                  accountID,
		EstimatedCapacity:          persisted.EstimatedCapacity,
		SuccessEMA:                 persisted.SuccessEMA,
		LatencyByModelFamily:       cloneAnthropicAdaptiveLatencyMap(persisted.LatencyByModelFamily),
		ConsecutiveSuccess:         persisted.ConsecutiveSuccess,
		ConsecutiveFailure:         persisted.ConsecutiveFailure,
		ConsecutiveCapacityFailure: persisted.ConsecutiveCapacityFailure,
		TotalSamples:               persisted.TotalSamples,
		RecentHealthSamples:        persisted.RecentHealthSamples,
		RecentHealthFailures:       persisted.RecentHealthFailures,
		RecentCapacitySamples:      persisted.RecentCapacitySamples,
		RecentCapacityFailures:     persisted.RecentCapacityFailures,
		LastSuccessAt:              timeFromUnixMilli(persisted.LastSuccessAtUnix),
		LastFailureAt:              timeFromUnixMilli(persisted.LastFailureAtUnix),
		LastCapacityFailureAt:      timeFromUnixMilli(persisted.LastCapacityFailureAtUnix),
		RecentWindowStartedAt:      timeFromUnixMilli(persisted.RecentWindowStartedAtUnix),
		CooldownUntil:              timeFromUnixMilli(persisted.CooldownUntilUnix),
		UpdatedAt:                  timeFromUnixMilli(persisted.UpdatedAtUnix),
	}
	if state.LatencyByModelFamily == nil {
		state.LatencyByModelFamily = make(map[string]anthropicAdaptiveLatencyState, 4)
	}
	if state.UpdatedAt.After(now.Add(5 * time.Minute)) {
		return anthropicAdaptiveAccountState{}, fmt.Errorf("updated_at is too far in the future")
	}
	if !state.CooldownUntil.After(now) {
		state.CooldownUntil = time.Time{}
	}
	if err := validateAnthropicAdaptiveAccountState(state); err != nil {
		return anthropicAdaptiveAccountState{}, err
	}
	return state, nil
}

func validateAnthropicAdaptiveAccountState(state anthropicAdaptiveAccountState) error {
	if state.AccountID <= 0 || state.EstimatedCapacity < 0 || state.UpdatedAt.IsZero() {
		return fmt.Errorf("invalid account identity, capacity, or updated_at")
	}
	if !finiteInRange(state.SuccessEMA, 0, 1) {
		return fmt.Errorf("invalid success EMA")
	}
	if state.ConsecutiveSuccess < 0 || state.ConsecutiveFailure < 0 || state.ConsecutiveCapacityFailure < 0 ||
		state.TotalSamples < 0 || state.RecentHealthSamples < 0 || state.RecentHealthFailures < 0 ||
		state.RecentCapacitySamples < 0 || state.RecentCapacityFailures < 0 ||
		state.RecentHealthFailures > state.RecentHealthSamples || state.RecentCapacityFailures > state.RecentCapacitySamples {
		return fmt.Errorf("invalid sample counters")
	}
	for family, latency := range state.LatencyByModelFamily {
		if family == "" || len(family) > 64 || latency.Samples < 0 ||
			!finiteNonNegative(latency.TTFTEMA) || !finiteNonNegative(latency.LatencyEMA) {
			return fmt.Errorf("invalid latency state for model family %q", family)
		}
	}
	return nil
}

func cloneAnthropicAdaptiveLatencyMap(in map[string]anthropicAdaptiveLatencyState) map[string]anthropicAdaptiveLatencyState {
	if len(in) == 0 {
		return make(map[string]anthropicAdaptiveLatencyState, 4)
	}
	out := make(map[string]anthropicAdaptiveLatencyState, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s *anthropicAdaptiveStateStore) dirtySnapshots(now time.Time, retention time.Duration) []anthropicAdaptiveDirtySnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	out := make([]anthropicAdaptiveDirtySnapshot, 0, len(s.accounts))
	for _, state := range s.accounts {
		if state == nil || state.revision <= state.persistedRevision || state.UpdatedAt.IsZero() {
			continue
		}
		if retention > 0 && now.Sub(state.UpdatedAt) > retention {
			continue
		}
		out = append(out, anthropicAdaptiveDirtySnapshot{
			state:    cloneAnthropicAdaptiveAccountState(state),
			revision: state.revision,
		})
	}
	s.mu.RUnlock()
	return out
}

func (s *anthropicAdaptiveStateStore) markPersisted(snapshots []anthropicAdaptiveDirtySnapshot) {
	if s == nil || len(snapshots) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, snapshot := range snapshots {
		state := s.accounts[snapshot.state.AccountID]
		if state != nil && snapshot.revision > state.persistedRevision {
			state.persistedRevision = snapshot.revision
		}
	}
}

func (s *anthropicAdaptiveStateStore) restoreAtStartup(incoming anthropicAdaptiveAccountState, now time.Time) bool {
	if s == nil || incoming.AccountID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	incoming.LatencyByModelFamily = cloneAnthropicAdaptiveLatencyMap(incoming.LatencyByModelFamily)
	incoming.revision = 1
	incoming.persistedRevision = 1
	local := s.accounts[incoming.AccountID]
	if local == nil {
		restored := incoming
		s.accounts[incoming.AccountID] = &restored
		return true
	}
	localHasFailure := local.ConsecutiveFailure > 0 || local.ConsecutiveCapacityFailure > 0 ||
		local.RecentHealthFailures > 0 || local.RecentCapacityFailures > 0 ||
		!local.LastFailureAt.IsZero() || local.CooldownUntil.After(now)
	if local.TotalSamples == 0 && !localHasFailure {
		restored := incoming
		s.accounts[incoming.AccountID] = &restored
		return true
	}
	if local.TotalSamples >= anthropicAdaptiveStateLocalMergeLimit {
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
	for family, latency := range local.LatencyByModelFamily {
		if current, ok := merged.LatencyByModelFamily[family]; !ok || current.Samples == 0 {
			merged.LatencyByModelFamily[family] = latency
		}
	}
	if localHasFailure {
		if local.EstimatedCapacity > 0 && local.EstimatedCapacity < merged.EstimatedCapacity {
			merged.EstimatedCapacity = local.EstimatedCapacity
		}
		merged.SuccessEMA = math.Min(merged.SuccessEMA, local.SuccessEMA)
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

func (s *GatewayService) startAnthropicAdaptiveStatePersistence() {
	if s == nil || s.anthropicAdaptiveScheduler == nil {
		return
	}
	s.anthropicStatePersistOnce.Do(func() {
		cache, ok := s.cache.(AdaptiveSchedulerStateCache)
		if !ok || cache == nil {
			return
		}
		resolver := func(ctx context.Context) AnthropicAdaptiveSchedulerSettings {
			if s.settingService == nil {
				return DefaultAnthropicAdaptiveSchedulerSettings()
			}
			settings, err := s.settingService.GetAnthropicAdaptiveSchedulerSettings(ctx)
			if err != nil {
				return DefaultAnthropicAdaptiveSchedulerSettings()
			}
			return settings
		}
		s.anthropicStatePersistence = newAnthropicAdaptiveStatePersistence(cache, s.anthropicAdaptiveScheduler.state, resolver)
		s.anthropicStatePersistence.Start()
	})
}

func (s *GatewayService) CloseAnthropicAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil || s.anthropicStatePersistence == nil {
		return nil
	}
	return s.anthropicStatePersistence.Stop(ctx)
}
