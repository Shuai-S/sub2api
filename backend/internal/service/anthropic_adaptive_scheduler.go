package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

type AnthropicAdaptiveMetricsSnapshot struct {
	SelectTotal           uint64
	ShadowDivergeTotal    uint64
	FallbackTotal         uint64
	StickyHitTotal        uint64
	CapacityDecreaseTotal uint64
}

type anthropicAdaptiveScheduler struct {
	state                 *anthropicAdaptiveStateStore
	now                   func() time.Time
	selectTotal           atomic.Uint64
	shadowDivergeTotal    atomic.Uint64
	fallbackTotal         atomic.Uint64
	stickyHitTotal        atomic.Uint64
	capacityDecreaseTotal atomic.Uint64
}

func newAnthropicAdaptiveScheduler() *anthropicAdaptiveScheduler {
	return &anthropicAdaptiveScheduler{
		state: newAnthropicAdaptiveStateStore(),
		now:   time.Now,
	}
}

func (s *anthropicAdaptiveScheduler) SnapshotMetrics() AnthropicAdaptiveMetricsSnapshot {
	if s == nil {
		return AnthropicAdaptiveMetricsSnapshot{}
	}
	return AnthropicAdaptiveMetricsSnapshot{
		SelectTotal:           s.selectTotal.Load(),
		ShadowDivergeTotal:    s.shadowDivergeTotal.Load(),
		FallbackTotal:         s.fallbackTotal.Load(),
		StickyHitTotal:        s.stickyHitTotal.Load(),
		CapacityDecreaseTotal: s.capacityDecreaseTotal.Load(),
	}
}

func (s *GatewayService) anthropicAdaptiveMode(ctx context.Context, platform string, accounts []Account) string {
	if s == nil || s.anthropicAdaptiveScheduler == nil || s.settingService == nil || platform != PlatformAnthropic || len(accounts) == 0 {
		return ""
	}
	for i := range accounts {
		if accounts[i].Platform != PlatformAnthropic {
			return ""
		}
	}
	settings, err := s.settingService.GetAnthropicAdaptiveSchedulerSettings(ctx)
	if err != nil {
		slog.Warn("anthropic_adaptive_settings_read_failed", "error", err)
		return ""
	}
	if !settings.AnthropicAdaptiveSchedulerEnabled {
		return ""
	}
	return normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
}

func (s *GatewayService) anthropicAdaptiveCapacity(mode string, account *Account) int {
	if mode != AnthropicAdaptiveSchedulerModeEnforce || s == nil || s.anthropicAdaptiveScheduler == nil || account == nil || account.Platform != PlatformAnthropic {
		if account == nil {
			return 0
		}
		return account.Concurrency
	}
	return s.anthropicAdaptiveScheduler.state.effectiveCapacity(account)
}

func (s *GatewayService) anthropicAdaptiveOrder(mode, requestedModel string, candidates []accountWithLoad) ([]accountWithLoad, map[int64]int, *AnthropicAdaptiveDecision) {
	if mode == "" || s == nil || s.anthropicAdaptiveScheduler == nil || len(candidates) == 0 {
		return candidates, nil, nil
	}
	decision := s.anthropicAdaptiveScheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: requestedModel,
		Candidates:     candidates,
	})
	if len(decision.Order) == 0 {
		s.anthropicAdaptiveScheduler.fallbackTotal.Add(1)
		return candidates, nil, &decision
	}
	capacities := make(map[int64]int, len(decision.Order))
	ordered := make([]accountWithLoad, 0, len(decision.Order))
	for _, candidate := range decision.Order {
		capacities[candidate.Account.ID] = candidate.EffectiveCapacity
		ordered = append(ordered, accountWithLoad{account: candidate.Account, loadInfo: candidate.LoadInfo})
	}
	if mode == AnthropicAdaptiveSchedulerModeEnforce {
		s.anthropicAdaptiveScheduler.selectTotal.Add(1)
		return ordered, capacities, &decision
	}
	return candidates, capacities, &decision
}

func (s *GatewayService) logAnthropicAdaptiveShadowDecision(decision *AnthropicAdaptiveDecision, baselineAccountID int64, scope string, stickyWouldBypass bool) {
	if decision == nil || s == nil || s.anthropicAdaptiveScheduler == nil {
		return
	}
	diverged := decision.SelectedAccountID > 0 && baselineAccountID > 0 && decision.SelectedAccountID != baselineAccountID
	if diverged {
		s.anthropicAdaptiveScheduler.shadowDivergeTotal.Add(1)
	}
	slog.Info("anthropic_adaptive_shadow_decision",
		"baseline_account_id", baselineAccountID,
		"adaptive_account_id", decision.SelectedAccountID,
		"shadow_diverged", diverged,
		"sticky_would_bypass", stickyWouldBypass,
		"scope", scope,
		"candidate_count", decision.CandidateCount,
		"top_k", decision.TopK,
		"fallback_reason", decision.FallbackReason,
	)
}

func (s *GatewayService) logAnthropicAdaptiveStickyWouldBypass(accountID int64, scope string) {
	if s == nil || s.anthropicAdaptiveScheduler == nil {
		return
	}
	slog.Info("anthropic_adaptive_shadow_decision",
		"baseline_account_id", accountID,
		"adaptive_account_id", int64(0),
		"shadow_diverged", false,
		"sticky_would_bypass", true,
		"scope", scope,
		"candidate_count", 0,
		"top_k", 0,
	)
}

func (s *GatewayService) markAnthropicAdaptiveStickyHit(mode string) {
	if mode != "" && s != nil && s.anthropicAdaptiveScheduler != nil {
		s.anthropicAdaptiveScheduler.stickyHitTotal.Add(1)
	}
}
