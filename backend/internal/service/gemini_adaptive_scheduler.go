package service

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"
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
		slog.Warn("gemini_adaptive_settings_read_failed", "error", err)
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

func (s *GatewayService) geminiAdaptiveOrder(ctx context.Context, mode string, settings GeminiAdaptiveSchedulerSettings, requestedModel string, candidates []accountWithLoad, quota map[int64]GeminiAdaptiveQuotaSnapshot) ([]accountWithLoad, map[int64]int, *GeminiAdaptiveDecision) {
	if mode == "" || s == nil || s.geminiAdaptiveScheduler == nil || len(candidates) == 0 {
		return candidates, nil, nil
	}
	hint := geminiAdaptiveHintFromContext(ctx)
	inputs := make([]GeminiAdaptiveCandidateInput, 0, len(candidates))
	baseline := make([]int64, 0, len(candidates))
	for _, item := range candidates {
		if item.account == nil {
			continue
		}
		inputs = append(inputs, GeminiAdaptiveCandidateInput{Account: item.account, Load: item.loadInfo, Quota: quota[item.account.ID]})
		baseline = append(baseline, item.account.ID)
	}
	decision, err := s.geminiAdaptiveScheduler.BuildOrder(GeminiAdaptiveScheduleRequest{
		RequestedModel: requestedModel,
		Stream:         hint.Stream,
		Action:         hint.Action,
		Candidates:     inputs,
		BaselineOrder:  baseline,
		Settings:       &settings,
	})
	if err != nil || len(decision.Order) == 0 {
		s.geminiAdaptiveScheduler.fallbackTotal.Add(1)
		if err != nil {
			decision.FallbackReason = "build_order_error"
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

func (s *GatewayService) logGeminiAdaptiveShadowDecision(decision *GeminiAdaptiveDecision, baselineAccountID int64, scope string, stickyWouldMigrate bool, settings GeminiAdaptiveSchedulerSettings) {
	if decision == nil || s == nil || s.geminiAdaptiveScheduler == nil {
		return
	}
	diverged := decision.SelectedAccountID > 0 && baselineAccountID > 0 && decision.SelectedAccountID != baselineAccountID
	if diverged {
		s.geminiAdaptiveScheduler.shadowDivergeTotal.Add(1)
	}
	if !settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled || (!diverged && rand.Float64() > settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate) {
		return
	}
	slog.Info("gemini_adaptive_shadow_decision",
		"baseline_account_id", baselineAccountID,
		"adaptive_account_id", decision.SelectedAccountID,
		"shadow_diverged", diverged,
		"sticky_would_migrate", stickyWouldMigrate,
		"scope", scope,
		"candidate_count", decision.CandidateCount,
		"top_k", decision.TopK,
		"fallback_reason", decision.FallbackReason,
	)
}

func (s *GatewayService) markGeminiAdaptiveStickyHit(mode string) {
	if mode != "" && s != nil && s.geminiAdaptiveScheduler != nil {
		s.geminiAdaptiveScheduler.stickyHitTotal.Add(1)
	}
}
