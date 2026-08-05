package service

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type GeminiAdaptiveMetricsSnapshot struct {
	SelectTotal           uint64 `json:"select_total"`
	ShadowDivergeTotal    uint64 `json:"shadow_diverge_total"`
	FallbackTotal         uint64 `json:"fallback_total"`
	StickyHitTotal        uint64 `json:"sticky_hit_total"`
	StickyMigrateTotal    uint64 `json:"sticky_migrate_total"`
	CapacityDecreaseTotal uint64 `json:"capacity_decrease_total"`
	QuotaSnapshotErrors   uint64 `json:"quota_snapshot_error_total"`
}

type geminiAdaptiveScheduler struct {
	state                 *geminiAdaptiveStateStore
	now                   func() time.Time
	selectTotal           atomic.Uint64
	shadowDivergeTotal    atomic.Uint64
	fallbackTotal         atomic.Uint64
	stickyHitTotal        atomic.Uint64
	stickyMigrateTotal    atomic.Uint64
	capacityDecreaseTotal atomic.Uint64
	quotaSnapshotErrors   atomic.Uint64
}

func newGeminiAdaptiveScheduler() *geminiAdaptiveScheduler {
	return &geminiAdaptiveScheduler{state: newGeminiAdaptiveStateStore(), now: time.Now}
}

func (s *geminiAdaptiveScheduler) SnapshotMetrics() GeminiAdaptiveMetricsSnapshot {
	if s == nil {
		return GeminiAdaptiveMetricsSnapshot{}
	}
	return GeminiAdaptiveMetricsSnapshot{
		SelectTotal:           s.selectTotal.Load(),
		ShadowDivergeTotal:    s.shadowDivergeTotal.Load(),
		FallbackTotal:         s.fallbackTotal.Load(),
		StickyHitTotal:        s.stickyHitTotal.Load(),
		StickyMigrateTotal:    s.stickyMigrateTotal.Load(),
		CapacityDecreaseTotal: s.capacityDecreaseTotal.Load(),
		QuotaSnapshotErrors:   s.quotaSnapshotErrors.Load(),
	}
}

type geminiAdaptiveRequestHint struct {
	Stream bool
	Action string
}

type geminiAdaptiveRequestHintContextKey struct{}

func WithGeminiAdaptiveRequestHint(ctx context.Context, action string, stream bool) context.Context {
	return context.WithValue(ctx, geminiAdaptiveRequestHintContextKey{}, geminiAdaptiveRequestHint{Action: action, Stream: stream})
}

func geminiAdaptiveHintFromContext(ctx context.Context) geminiAdaptiveRequestHint {
	if hint, ok := ctx.Value(geminiAdaptiveRequestHintContextKey{}).(geminiAdaptiveRequestHint); ok {
		return hint
	}
	return geminiAdaptiveRequestHint{Action: "generateContent"}
}

func (s *GatewayService) geminiAdaptiveMode(ctx context.Context, platform string, accounts []Account) (string, GeminiAdaptiveSchedulerSettings) {
	defaults := DefaultGeminiAdaptiveSchedulerSettings()
	if s == nil || s.geminiAdaptiveScheduler == nil || s.settingService == nil || platform != PlatformGemini || len(accounts) == 0 {
		return "", defaults
	}
	hasNativeGemini := false
	for i := range accounts {
		if accounts[i].Platform == PlatformGemini {
			hasNativeGemini = true
			break
		}
	}
	if !hasNativeGemini {
		return "", defaults
	}
	settings, err := s.settingService.GetGeminiAdaptiveSchedulerSettings(ctx)
	if err != nil {
		fields := []any{
			"request_id", contextStringValue(ctx, ctxkey.RequestID),
			"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
			"platform", platform,
			"account_count", len(accounts),
		}
		fields = append(fields, geminiAdaptiveErrorLogFields(err)...)
		slog.Warn("gemini_adaptive_settings_read_failed", fields...)
		return "", defaults
	}
	if !settings.GeminiAdaptiveSchedulerEnabled {
		return "", settings
	}
	return normalizeGeminiAdaptiveSchedulerMode(settings.GeminiAdaptiveSchedulerMode), settings
}

func (s *GatewayService) geminiAdaptiveCapacity(mode string, settings GeminiAdaptiveSchedulerSettings, account *Account) int {
	if account == nil {
		return 0
	}
	if mode != GeminiAdaptiveSchedulerModeEnforce || s == nil || s.geminiAdaptiveScheduler == nil || account.Platform != PlatformGemini {
		return account.Concurrency
	}
	return s.geminiAdaptiveScheduler.state.effectiveCapacity(account, settings)
}

func (s *GatewayService) geminiAdaptiveCircuitAllowed(ctx context.Context, mode string, settings GeminiAdaptiveSchedulerSettings, account *Account, requestedModel string) bool {
	if mode != GeminiAdaptiveSchedulerModeEnforce || s == nil || s.geminiAdaptiveScheduler == nil || account == nil || account.Platform != PlatformGemini {
		return true
	}
	now := s.geminiAdaptiveScheduler.now()
	hint := geminiAdaptiveHintFromContext(ctx)
	eligibility := s.geminiAdaptiveScheduler.state.circuitEligibility(account, requestedModel, hint.Action, now, settings)
	return eligibility.Allowed
}

func (s *GatewayService) claimGeminiAdaptiveCircuitProbe(ctx context.Context, mode string, settings GeminiAdaptiveSchedulerSettings, account *Account, requestedModel string) (bool, func()) {
	noop := func() {}
	if mode != GeminiAdaptiveSchedulerModeEnforce || s == nil || s.geminiAdaptiveScheduler == nil || account == nil || account.Platform != PlatformGemini {
		return true, noop
	}
	hint := geminiAdaptiveHintFromContext(ctx)
	owner := firstNonEmpty(contextStringValue(ctx, ctxkey.RequestID), contextStringValue(ctx, ctxkey.ClientRequestID))
	now := s.geminiAdaptiveScheduler.now()
	allowed, claimed := s.geminiAdaptiveScheduler.state.claimCircuitProbe(account, requestedModel, hint.Action, owner, now, settings)
	if !allowed || !claimed {
		return allowed, noop
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			s.geminiAdaptiveScheduler.state.releaseCircuitProbe(account, requestedModel, hint.Action, owner, s.geminiAdaptiveScheduler.now(), settings)
		})
	}
	stop := context.AfterFunc(ctx, release)
	return true, func() {
		_ = stop()
		release()
	}
}

func (s *GatewayService) tryAcquireGeminiAdaptiveAccountSlot(ctx context.Context, mode string, settings GeminiAdaptiveSchedulerSettings, account *Account, requestedModel string, maxConcurrency int) (*AcquireResult, func(), error) {
	allowed, releaseProbe := s.claimGeminiAdaptiveCircuitProbe(ctx, mode, settings, account, requestedModel)
	if !allowed {
		return &AcquireResult{}, func() {}, nil
	}
	result, err := s.tryAcquireAccountSlot(ctx, account.ID, maxConcurrency)
	if err != nil || result == nil || !result.Acquired {
		releaseProbe()
	}
	if result == nil {
		result = &AcquireResult{}
	}
	return result, releaseProbe, err
}

func (s *GatewayService) geminiAdaptiveOrder(ctx context.Context, mode string, settings GeminiAdaptiveSchedulerSettings, requestedModel, scope string, groupID *int64, sessionHash string, candidates []accountWithLoad, quota map[int64]GeminiAdaptiveQuotaSnapshot) ([]accountWithLoad, map[int64]int, *GeminiAdaptiveDecision) {
	if mode == "" || s == nil || s.geminiAdaptiveScheduler == nil || len(candidates) == 0 {
		return candidates, nil, nil
	}
	startedAt := time.Now()
	hint := geminiAdaptiveHintFromContext(ctx)
	inputs := make([]GeminiAdaptiveCandidateInput, 0, len(candidates))
	baseline := make([]int64, 0, len(candidates))
	inputAccountIDs := make(map[int64]struct{}, len(candidates))
	for _, item := range candidates {
		if item.account == nil {
			continue
		}
		inputs = append(inputs, GeminiAdaptiveCandidateInput{Account: item.account, Load: item.loadInfo, Quota: quota[item.account.ID]})
		baseline = append(baseline, item.account.ID)
		inputAccountIDs[item.account.ID] = struct{}{}
	}
	decision, err := s.geminiAdaptiveScheduler.BuildOrder(GeminiAdaptiveScheduleRequest{
		RequestedModel: requestedModel,
		Stream:         hint.Stream,
		Action:         hint.Action,
		Candidates:     inputs,
		BaselineOrder:  baseline,
		Settings:       &settings,
		ctx:            ctx,
	})
	for accountID, snapshot := range quota {
		if !snapshot.HardRejected {
			continue
		}
		if _, included := inputAccountIDs[accountID]; included {
			continue
		}
		decision.InputCandidateCount++
		decision.HardRejectedCount++
	}
	decision.BuildLatencyMs = time.Since(startedAt).Milliseconds()
	if err != nil || len(decision.Order) == 0 {
		s.geminiAdaptiveScheduler.fallbackTotal.Add(1)
		if err != nil {
			decision.FallbackReason = "build_order_error"
			fields := []any{
				"request_id", contextStringValue(ctx, ctxkey.RequestID),
				"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
				"mode", mode,
				"scope", scope,
				"model", requestedModel,
				"group_id", derefGroupID(groupID),
				"session", shortSessionHash(sessionHash),
			}
			fields = append(fields, geminiAdaptiveErrorLogFields(err)...)
			slog.Warn("gemini_adaptive_scheduler_fallback", fields...)
		} else if decision.FallbackReason == "" {
			decision.FallbackReason = "empty_order"
		}
		s.logGeminiAdaptiveDiagnosticDecision(ctx, settings, geminiAdaptiveDecisionLog{
			Mode:           mode,
			Scope:          scope,
			Outcome:        "fallback",
			RequestedModel: requestedModel,
			GroupID:        groupID,
			SessionHash:    sessionHash,
			Decision:       &decision,
			Force:          true,
			Err:            err,
		})
		if mode == GeminiAdaptiveSchedulerModeEnforce && decision.FallbackReason == "all_native_gemini_circuits_open" {
			return nil, nil, &decision
		}
		return candidates, nil, &decision
	}
	capacities := make(map[int64]int, len(decision.Order))
	ordered := make([]accountWithLoad, 0, len(decision.Order))
	for _, candidate := range decision.Order {
		if candidate.Account == nil {
			continue
		}
		capacity := candidate.Account.Concurrency
		if candidate.Account.Platform == PlatformGemini {
			capacity = candidate.EffectiveCapacity
		}
		capacities[candidate.Account.ID] = capacity
		ordered = append(ordered, accountWithLoad{account: candidate.Account, loadInfo: candidate.Load})
	}
	if mode == GeminiAdaptiveSchedulerModeEnforce {
		s.geminiAdaptiveScheduler.selectTotal.Add(1)
		return ordered, capacities, &decision
	}
	return candidates, capacities, &decision
}

func (s *GatewayService) logGeminiAdaptiveShadowDecision(ctx context.Context, decision *GeminiAdaptiveDecision, baselineAccount *Account, requestedModel string, groupID *int64, sessionHash, scope string, stickyWouldMigrate bool, settings GeminiAdaptiveSchedulerSettings) {
	if decision == nil || s == nil || s.geminiAdaptiveScheduler == nil {
		return
	}
	baselineAccountID := int64(0)
	if baselineAccount != nil {
		baselineAccountID = baselineAccount.ID
	}
	diverged := decision.SelectedAccountID > 0 && baselineAccountID > 0 && decision.SelectedAccountID != baselineAccountID
	if diverged {
		s.geminiAdaptiveScheduler.shadowDivergeTotal.Add(1)
	}
	s.logGeminiAdaptiveDiagnosticDecision(ctx, settings, geminiAdaptiveDecisionLog{
		Mode:               GeminiAdaptiveSchedulerModeShadow,
		Scope:              scope,
		Outcome:            "baseline_selected",
		RequestedModel:     requestedModel,
		GroupID:            groupID,
		SessionHash:        sessionHash,
		Decision:           decision,
		SelectedAccount:    baselineAccount,
		StickyWouldMigrate: stickyWouldMigrate,
		Force:              diverged,
	})
}

func (s *GatewayService) markGeminiAdaptiveStickyHit(mode string) {
	if mode != "" && s != nil && s.geminiAdaptiveScheduler != nil {
		s.geminiAdaptiveScheduler.stickyHitTotal.Add(1)
	}
}
