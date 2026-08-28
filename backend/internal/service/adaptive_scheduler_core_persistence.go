package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	adaptiveSchedulerCoreNamespaceOpenAI    = "openai_v2"
	adaptiveSchedulerCoreNamespaceAnthropic = "anthropic_v2"
	adaptiveSchedulerCoreNamespaceGemini    = "gemini_v2"
	adaptiveCoreRestoreScanSize             = 256
	adaptiveCoreRestoreFutureTolerance      = 5 * time.Minute
)

type adaptiveCorePersistedState struct {
	Version int                  `json:"version"`
	SavedAt time.Time            `json:"saved_at"`
	State   adaptiveAccountState `json:"state"`
}

type adaptiveCoreDirtySnapshot struct {
	state    adaptiveAccountState
	revision uint64
}

func (s *adaptiveStateStore) dirtySnapshots(now time.Time, retention time.Duration) []adaptiveCoreDirtySnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]adaptiveCoreDirtySnapshot, 0, len(s.accounts))
	for _, state := range s.accounts {
		if state == nil || state.revision <= state.persistedRevision || state.UpdatedAt.IsZero() || (retention > 0 && now.Sub(state.UpdatedAt) > retention) {
			continue
		}
		out = append(out, adaptiveCoreDirtySnapshot{state: cloneAdaptiveAccountState(state), revision: state.revision})
	}
	return out
}

func (s *adaptiveStateStore) markPersisted(snapshots []adaptiveCoreDirtySnapshot) {
	if s == nil {
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

func (s *adaptiveStateStore) restoreAtStartup(incoming adaptiveAccountState) bool {
	if s == nil || incoming.AccountID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if local := s.accounts[incoming.AccountID]; local != nil && local.revision > 0 {
		return false
	}
	for _, admission := range s.admissions {
		if admission.AccountID == incoming.AccountID {
			return false
		}
	}
	restored := cloneAdaptiveAccountState(&incoming)
	restored.HealthProbeInFlight = false
	restored.HealthProbeUntil = time.Time{}
	restored.HealthProbeOwner = ""
	restored.QuotaProbeInFlight = false
	restored.QuotaProbeOwner = ""
	restored.CapacityLimitedGeneration = false
	restored.revision = 1
	restored.persistedRevision = 1
	s.accounts[incoming.AccountID] = &restored
	return true
}

type adaptiveCoreStatePersistence struct {
	cache     AdaptiveSchedulerStateCache
	store     *adaptiveStateStore
	namespace string
	now       func() time.Time
	worker    *adaptiveStatePersistenceWorker
}

func newAdaptiveCoreStatePersistence(cache AdaptiveSchedulerStateCache, store *adaptiveStateStore, namespace string) *adaptiveCoreStatePersistence {
	persistence := &adaptiveCoreStatePersistence{cache: cache, store: store, namespace: namespace, now: time.Now}
	persistence.worker = newAdaptiveStatePersistenceWorker(cache, namespace, persistence.restoreOnce, persistence.flush, func() time.Time {
		return persistence.now()
	})
	return persistence
}

func (p *adaptiveCoreStatePersistence) Start() {
	if p != nil && p.worker != nil {
		p.worker.Start()
	}
}

func (p *adaptiveCoreStatePersistence) Stop(ctx context.Context) error {
	if p == nil || p.worker == nil {
		return nil
	}
	return p.worker.Stop(ctx)
}

func (s *GatewayService) CloseAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return errors.Join(
		s.CloseAnthropicAdaptiveStatePersistence(ctx),
		s.CloseGeminiAdaptiveStatePersistence(ctx),
	)
}

func (p *adaptiveCoreStatePersistence) restoreOnce(ctx context.Context) (restored, stale, invalid int, err error) {
	if p == nil || p.cache == nil || p.store == nil {
		return 0, 0, 0, nil
	}
	now := p.now()
	loaded := make(map[int64]adaptiveAccountState)
	var cursor uint64
	for {
		records, nextCursor, scanErr := p.cache.ScanAdaptiveSchedulerStates(ctx, p.namespace, cursor, adaptiveCoreRestoreScanSize)
		if scanErr != nil {
			return 0, 0, 0, scanErr
		}
		for _, record := range records {
			persisted, decodeErr := decodeAdaptiveCorePersistedState(record.AccountID, record.Payload, now)
			if decodeErr != nil {
				invalid++
				continue
			}
			if now.Sub(persisted.SavedAt) > adaptiveStateRetention || now.Sub(persisted.State.UpdatedAt) > adaptiveStateRetention {
				stale++
				continue
			}
			state := persisted.State
			if p.namespace == adaptiveSchedulerCoreNamespaceOpenAI && len(state.TTFTWindows) == 0 {
				// V2 snapshots created before windowed TTFT contain a lifetime EMA.
				// Preserve health and capacity learning, but do not restore that legacy
				// latency history into the OpenAI scheduler or diagnostics.
				state.TTFTEMA = 0
				state.TTFTSamples = 0
			}
			loaded[record.AccountID] = state
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	for _, state := range loaded {
		if p.store.restoreAtStartup(state) {
			restored++
		}
	}
	return restored, stale, invalid, nil
}

func decodeAdaptiveCorePersistedState(accountID int64, payload []byte, now time.Time) (adaptiveCorePersistedState, error) {
	var persisted adaptiveCorePersistedState
	if err := json.Unmarshal(payload, &persisted); err != nil {
		return adaptiveCorePersistedState{}, err
	}
	if persisted.Version != adaptiveSchedulerStateVersion {
		return adaptiveCorePersistedState{}, fmt.Errorf("unsupported adaptive state version %d", persisted.Version)
	}
	if persisted.SavedAt.IsZero() || persisted.SavedAt.After(now.Add(adaptiveCoreRestoreFutureTolerance)) {
		return adaptiveCorePersistedState{}, fmt.Errorf("invalid adaptive state saved_at")
	}
	if err := validateAdaptiveCoreRestoredState(accountID, persisted.State, now); err != nil {
		return adaptiveCorePersistedState{}, err
	}
	return persisted, nil
}

func validateAdaptiveCoreRestoredState(accountID int64, state adaptiveAccountState, now time.Time) error {
	if accountID <= 0 || state.AccountID != accountID {
		return fmt.Errorf("adaptive state account id mismatch: field=%d payload=%d", accountID, state.AccountID)
	}
	if state.Version != adaptiveSchedulerStateVersion {
		return fmt.Errorf("unsupported account state version %d", state.Version)
	}
	if state.UpdatedAt.IsZero() || state.UpdatedAt.After(now.Add(adaptiveCoreRestoreFutureTolerance)) {
		return fmt.Errorf("invalid adaptive account updated_at")
	}
	if state.ConfiguredCapacity < 0 || state.EffectiveCapacity < 0 ||
		(state.ConfiguredCapacity == 0 && state.EffectiveCapacity != 0) ||
		(state.ConfiguredCapacity > 0 && (state.EffectiveCapacity == 0 || state.EffectiveCapacity > state.ConfiguredCapacity)) {
		return fmt.Errorf("invalid adaptive account capacity")
	}
	if state.CapacityGeneration == 0 || state.CapacityRecoverySuccesses < 0 || state.LastObservedConcurrency < 0 {
		return fmt.Errorf("invalid adaptive account capacity counters")
	}
	if !finiteAdaptiveRestoreValue(state.SuccessEMA) || state.SuccessEMA < 0 || state.SuccessEMA > 1 {
		return fmt.Errorf("invalid adaptive account success EMA")
	}
	if !finiteAdaptiveRestoreValue(state.TTFTEMA) || state.TTFTEMA < 0 || state.TTFTSamples < 0 {
		return fmt.Errorf("invalid adaptive account TTFT state")
	}
	for key, observations := range state.TTFTWindows {
		if key == "" {
			return fmt.Errorf("invalid adaptive account TTFT bucket key")
		}
		var previous time.Time
		for _, observation := range observations {
			if observation.At.IsZero() || observation.At.After(now.Add(adaptiveCoreRestoreFutureTolerance)) {
				return fmt.Errorf("invalid adaptive TTFT observation time")
			}
			if !previous.IsZero() && observation.At.Before(previous) {
				return fmt.Errorf("adaptive TTFT observations are not ordered")
			}
			if observation.Milliseconds < 1 || observation.Milliseconds > 120000 {
				return fmt.Errorf("invalid adaptive TTFT observation value")
			}
			previous = observation.At
		}
	}
	if state.ConsecutiveFailures < 0 || state.CircuitOpenCount < 0 {
		return fmt.Errorf("invalid adaptive account failure counters")
	}
	if err := validateAdaptiveRestoredPastTime("last_success_at", state.LastSuccessAt, now); err != nil {
		return err
	}
	if err := validateAdaptiveRestoredPastTime("last_failure_at", state.LastFailureAt, now); err != nil {
		return err
	}
	if err := validateAdaptiveRestoredPastTime("last_capacity_shrink_at", state.LastCapacityShrinkAt, now); err != nil {
		return err
	}
	var previous time.Time
	for _, observation := range state.HealthObservations {
		if observation.At.IsZero() || observation.At.After(now.Add(adaptiveCoreRestoreFutureTolerance)) {
			return fmt.Errorf("invalid adaptive health observation time")
		}
		if !previous.IsZero() && observation.At.Before(previous) {
			return fmt.Errorf("adaptive health observations are not ordered")
		}
		previous = observation.At
	}
	switch state.LastObservationType {
	case "", adaptiveObservationHealthSuccess, adaptiveObservationAccountFailure, adaptiveObservationCapacityLimit,
		adaptiveObservationQuotaLimit, adaptiveObservationProviderOverload, adaptiveObservationIgnored:
	default:
		return fmt.Errorf("invalid adaptive observation type %q", state.LastObservationType)
	}
	return nil
}

func validateAdaptiveRestoredPastTime(name string, value, now time.Time) error {
	if !value.IsZero() && value.After(now.Add(adaptiveCoreRestoreFutureTolerance)) {
		return fmt.Errorf("invalid adaptive account %s", name)
	}
	return nil
}

func finiteAdaptiveRestoreValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (p *adaptiveCoreStatePersistence) flush(ctx context.Context) error {
	if p == nil || p.cache == nil || p.store == nil {
		return nil
	}
	now := p.now()
	snapshots := p.store.dirtySnapshots(now, adaptiveStateRetention)
	if len(snapshots) == 0 {
		return nil
	}
	entries := make([]AdaptiveSchedulerStateCacheEntry, 0, len(snapshots))
	for _, snapshot := range snapshots {
		payload, err := json.Marshal(adaptiveCorePersistedState{Version: adaptiveSchedulerStateVersion, SavedAt: now, State: snapshot.state})
		if err != nil {
			return fmt.Errorf("marshal adaptive state for account %d: %w", snapshot.state.AccountID, err)
		}
		entries = append(entries, AdaptiveSchedulerStateCacheEntry{AccountID: snapshot.state.AccountID, Payload: payload, ExpiresAt: now.Add(adaptiveStateRetention)})
	}
	if err := p.cache.SaveAdaptiveSchedulerStates(ctx, p.namespace, entries, adaptiveStateHashTTL); err != nil {
		return err
	}
	p.store.markPersisted(snapshots)
	return nil
}
