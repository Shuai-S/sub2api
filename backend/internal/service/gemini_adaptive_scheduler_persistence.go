package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const (
	geminiAdaptiveStateSchemaVersion   = 1
	geminiAdaptiveStateRestoreScanSize = 256
	geminiAdaptiveStateWriteBatchSize  = 256
)

type geminiAdaptivePersistedState struct {
	SchemaVersion  int    `json:"schema_version"`
	SourceInstance string `json:"source_instance,omitempty"`
	AccountID      int64  `json:"account_id"`
	UpdatedAtUnix  int64  `json:"updated_at"`

	EstimatedCapacity          int                                 `json:"estimated_capacity"`
	PathSuccessEMA             float64                             `json:"path_success_ema"`
	ByModelFamily              map[string]geminiAdaptiveModelState `json:"by_model_family"`
	ConsecutiveSuccess         int                                 `json:"consecutive_success"`
	ConsecutiveFailure         int                                 `json:"consecutive_failure"`
	ConsecutiveCapacityFailure int                                 `json:"consecutive_capacity_failure"`
	TotalSamples               int64                               `json:"total_samples"`
	RecentHealthSamples        int                                 `json:"recent_health_samples"`
	RecentHealthFailures       int                                 `json:"recent_health_failures"`
	RecentCapacitySamples      int                                 `json:"recent_capacity_samples"`
	RecentCapacityFailures     int                                 `json:"recent_capacity_failures"`
	LastSuccessAtUnix          int64                               `json:"last_success_at,omitempty"`
	LastFailureAtUnix          int64                               `json:"last_failure_at,omitempty"`
	LastCapacityFailureAtUnix  int64                               `json:"last_capacity_failure_at,omitempty"`
	RecentWindowStartedAtUnix  int64                               `json:"recent_window_started_at,omitempty"`
	CooldownUntilUnix          int64                               `json:"cooldown_until,omitempty"`
}

type geminiAdaptiveStatePersistence struct {
	cache            AdaptiveSchedulerStateCache
	store            *geminiAdaptiveStateStore
	sourceInstance   string
	settingsResolver func(context.Context) GeminiAdaptiveSchedulerSettings
	worker           *adaptiveStatePersistenceWorker
	now              func() time.Time
}

func newGeminiAdaptiveStatePersistence(cache AdaptiveSchedulerStateCache, store *geminiAdaptiveStateStore, settingsResolver func(context.Context) GeminiAdaptiveSchedulerSettings) *geminiAdaptiveStatePersistence {
	persistence := &geminiAdaptiveStatePersistence{
		cache:            cache,
		store:            store,
		sourceInstance:   adaptiveStateInstanceID(),
		settingsResolver: settingsResolver,
		now:              time.Now,
	}
	persistence.worker = newAdaptiveStatePersistenceWorker(cache, adaptiveSchedulerStateNamespaceGemini, persistence.restoreOnce, persistence.flush, func() time.Time { return persistence.now() })
	return persistence
}

func (p *geminiAdaptiveStatePersistence) Start() {
	if p != nil && p.store != nil {
		p.worker.Start()
	}
}

func (p *geminiAdaptiveStatePersistence) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.worker.Stop(ctx)
}

func (p *geminiAdaptiveStatePersistence) restoreOnce(ctx context.Context) (restored, stale, invalid int, err error) {
	now := p.now()
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	if p.settingsResolver != nil {
		settings = NormalizeGeminiAdaptiveSchedulerSettings(p.settingsResolver(ctx))
	}
	loaded := make(map[int64]geminiAdaptiveAccountState)
	var cursor uint64
	for {
		records, nextCursor, scanErr := p.cache.ScanAdaptiveSchedulerStates(ctx, adaptiveSchedulerStateNamespaceGemini, cursor, geminiAdaptiveStateRestoreScanSize)
		if scanErr != nil {
			return 0, 0, 0, scanErr
		}
		for _, record := range records {
			state, stateErr := decodeGeminiAdaptivePersistedState(record.AccountID, record.Payload, now)
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

func (p *geminiAdaptiveStatePersistence) flush(ctx context.Context) error {
	now := p.now()
	dirty := p.store.dirtySnapshots(now, adaptiveStateRetention)
	for start := 0; start < len(dirty); start += geminiAdaptiveStateWriteBatchSize {
		end := min(start+geminiAdaptiveStateWriteBatchSize, len(dirty))
		batch := dirty[start:end]
		entries := make([]AdaptiveSchedulerStateCacheEntry, 0, len(batch))
		written := make([]geminiAdaptiveDirtySnapshot, 0, len(batch))
		for _, snapshot := range batch {
			payload, encodeErr := encodeGeminiAdaptivePersistedState(snapshot.state, p.sourceInstance)
			if encodeErr != nil {
				slog.Warn("gemini_adaptive_state_encode_failed", "account_id", snapshot.state.AccountID, "error", encodeErr)
				continue
			}
			entries = append(entries, AdaptiveSchedulerStateCacheEntry{AccountID: snapshot.state.AccountID, Payload: payload, ExpiresAt: snapshot.state.UpdatedAt.Add(adaptiveStateRetention)})
			written = append(written, snapshot)
		}
		if len(entries) == 0 {
			continue
		}
		if saveErr := p.cache.SaveAdaptiveSchedulerStates(ctx, adaptiveSchedulerStateNamespaceGemini, entries, adaptiveStateHashTTL); saveErr != nil {
			return saveErr
		}
		p.store.markPersisted(written)
	}
	return nil
}

func encodeGeminiAdaptivePersistedState(state geminiAdaptiveAccountState, sourceInstance string) ([]byte, error) {
	if err := validateGeminiAdaptiveAccountState(state); err != nil {
		return nil, err
	}
	persisted := geminiAdaptivePersistedState{
		SchemaVersion:              geminiAdaptiveStateSchemaVersion,
		SourceInstance:             sourceInstance,
		AccountID:                  state.AccountID,
		UpdatedAtUnix:              state.UpdatedAt.UnixMilli(),
		EstimatedCapacity:          state.EstimatedCapacity,
		PathSuccessEMA:             state.PathSuccessEMA,
		ByModelFamily:              cloneGeminiAdaptiveModelMap(state.ByModelFamily),
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

func decodeGeminiAdaptivePersistedState(accountID int64, payload []byte, now time.Time) (geminiAdaptiveAccountState, error) {
	var persisted geminiAdaptivePersistedState
	if err := json.Unmarshal(payload, &persisted); err != nil {
		return geminiAdaptiveAccountState{}, err
	}
	if persisted.SchemaVersion != geminiAdaptiveStateSchemaVersion {
		return geminiAdaptiveAccountState{}, fmt.Errorf("unsupported schema version %d", persisted.SchemaVersion)
	}
	if accountID <= 0 || persisted.AccountID != accountID {
		return geminiAdaptiveAccountState{}, fmt.Errorf("account id mismatch: field=%d payload=%d", accountID, persisted.AccountID)
	}
	state := geminiAdaptiveAccountState{
		AccountID:                  accountID,
		EstimatedCapacity:          persisted.EstimatedCapacity,
		PathSuccessEMA:             persisted.PathSuccessEMA,
		ByModelFamily:              cloneGeminiAdaptiveModelMap(persisted.ByModelFamily),
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
	if state.UpdatedAt.After(now.Add(5 * time.Minute)) {
		return geminiAdaptiveAccountState{}, fmt.Errorf("updated_at is too far in the future")
	}
	if !state.CooldownUntil.After(now) {
		state.CooldownUntil = time.Time{}
	}
	if err := validateGeminiAdaptiveAccountState(state); err != nil {
		return geminiAdaptiveAccountState{}, err
	}
	return state, nil
}

func validateGeminiAdaptiveAccountState(state geminiAdaptiveAccountState) error {
	if state.AccountID <= 0 || state.EstimatedCapacity < 0 || state.UpdatedAt.IsZero() {
		return fmt.Errorf("invalid account identity, capacity, or updated_at")
	}
	if !finiteInRange(state.PathSuccessEMA, 0, 1) {
		return fmt.Errorf("invalid path success EMA")
	}
	if state.ConsecutiveSuccess < 0 || state.ConsecutiveFailure < 0 || state.ConsecutiveCapacityFailure < 0 || state.TotalSamples < 0 || state.RecentHealthSamples < 0 || state.RecentHealthFailures < 0 || state.RecentCapacitySamples < 0 || state.RecentCapacityFailures < 0 || state.RecentHealthFailures > state.RecentHealthSamples || state.RecentCapacityFailures > state.RecentCapacitySamples {
		return fmt.Errorf("invalid sample counters")
	}
	for family, modelState := range state.ByModelFamily {
		if !isGeminiAdaptiveModelFamily(family) || modelState.Samples < 0 || modelState.Failures < 0 || modelState.Failures > modelState.Samples || !finiteInRange(modelState.SuccessEMA, 0, 1) || !finiteNonNegative(modelState.TTFTEMA) || !finiteNonNegative(modelState.LatencyEMA) {
			return fmt.Errorf("invalid model state for family %q", family)
		}
	}
	return nil
}

func isGeminiAdaptiveModelFamily(family string) bool {
	switch family {
	case "pro", "flash", "image", "embedding", "other":
		return true
	default:
		return false
	}
}

func (s *GatewayService) startGeminiAdaptiveStatePersistence() {
	if s == nil || s.geminiAdaptiveScheduler == nil {
		return
	}
	s.geminiStatePersistOnce.Do(func() {
		cache, ok := s.cache.(AdaptiveSchedulerStateCache)
		if !ok || cache == nil {
			return
		}
		resolver := func(ctx context.Context) GeminiAdaptiveSchedulerSettings {
			if s.settingService == nil {
				return DefaultGeminiAdaptiveSchedulerSettings()
			}
			settings, err := s.settingService.GetGeminiAdaptiveSchedulerSettings(ctx)
			if err != nil {
				return DefaultGeminiAdaptiveSchedulerSettings()
			}
			return settings
		}
		s.geminiStatePersistence = newGeminiAdaptiveStatePersistence(cache, s.geminiAdaptiveScheduler.state, resolver)
		s.geminiStatePersistence.Start()
	})
}

func (s *GatewayService) CloseGeminiAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil || s.geminiStatePersistence == nil {
		return nil
	}
	return s.geminiStatePersistence.Stop(ctx)
}

func (s *GatewayService) CloseAdaptiveStatePersistence(ctx context.Context) error {
	firstErr := s.CloseAnthropicAdaptiveStatePersistence(ctx)
	if err := s.CloseGeminiAdaptiveStatePersistence(ctx); firstErr == nil {
		firstErr = err
	}
	return firstErr
}
