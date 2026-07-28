package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
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

type anthropicAdaptiveModeResolution struct {
	Mode                 string
	Settings             AnthropicAdaptiveSchedulerSettings
	BypassReason         string
	Err                  error
	NativeCandidateCount int
	MixedCandidateCount  int
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

func (s *GatewayService) anthropicAdaptiveMode(ctx context.Context, platform string, accounts []Account) anthropicAdaptiveModeResolution {
	resolution := anthropicAdaptiveModeResolution{Settings: DefaultAnthropicAdaptiveSchedulerSettings()}
	if platform != PlatformAnthropic {
		resolution.BypassReason = "non_anthropic_platform"
		return resolution
	}
	if s == nil || s.anthropicAdaptiveScheduler == nil || s.settingService == nil {
		resolution.BypassReason = "scheduler_unavailable"
		return resolution
	}
	settings, err := s.settingService.GetAnthropicAdaptiveSchedulerSettings(ctx)
	if err != nil {
		slog.Warn("anthropic_adaptive_settings_read_failed", "error", err)
		resolution.BypassReason = "settings_read_failed"
		resolution.Err = err
		return resolution
	}
	resolution.Settings = settings
	if !settings.AnthropicAdaptiveSchedulerEnabled {
		resolution.BypassReason = "disabled"
		return resolution
	}
	for i := range accounts {
		if accounts[i].Platform == PlatformAnthropic {
			resolution.NativeCandidateCount++
		} else {
			resolution.MixedCandidateCount++
		}
	}
	if resolution.NativeCandidateCount == 0 {
		resolution.BypassReason = "no_native_candidates"
		return resolution
	}
	if resolution.MixedCandidateCount > 0 {
		resolution.BypassReason = "mixed_platform_candidates"
		return resolution
	}
	resolution.Mode = normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
	return resolution
}

func (s *GatewayService) anthropicAdaptiveCapacity(mode string, settings AnthropicAdaptiveSchedulerSettings, account *Account) int {
	if mode != AnthropicAdaptiveSchedulerModeEnforce || s == nil || s.anthropicAdaptiveScheduler == nil || account == nil || account.Platform != PlatformAnthropic {
		if account == nil {
			return 0
		}
		return account.Concurrency
	}
	return s.anthropicAdaptiveScheduler.state.effectiveCapacity(account, settings)
}

func (s *GatewayService) anthropicAdaptiveOrder(mode string, settings AnthropicAdaptiveSchedulerSettings, requestedModel string, candidates []accountWithLoad) ([]accountWithLoad, map[int64]int, *AnthropicAdaptiveDecision) {
	if mode == "" || s == nil || s.anthropicAdaptiveScheduler == nil || len(candidates) == 0 {
		return candidates, nil, nil
	}
	decision := s.anthropicAdaptiveScheduler.BuildOrder(AnthropicAdaptiveScheduleRequest{
		RequestedModel: requestedModel,
		Candidates:     candidates,
		Settings:       &settings,
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

func (s *GatewayService) logAnthropicAdaptiveDecision(
	ctx context.Context,
	settings AnthropicAdaptiveSchedulerSettings,
	entry anthropicAdaptiveDecisionLog,
) {
	if entry.Mode == "" || s == nil || s.anthropicAdaptiveScheduler == nil {
		return
	}
	if entry.Mode == AnthropicAdaptiveSchedulerModeShadow && (entry.Decision != nil || entry.StickyWouldBypass) {
		baselineAccountID := entry.BaselineAccountID
		if baselineAccountID == 0 && entry.SelectedAccount != nil {
			baselineAccountID = entry.SelectedAccount.ID
		}
		var adaptiveAccountID int64
		var candidateCount, topK int
		var fallbackReason string
		if entry.Decision != nil {
			adaptiveAccountID = entry.Decision.SelectedAccountID
			candidateCount = entry.Decision.CandidateCount
			topK = entry.Decision.TopK
			fallbackReason = entry.Decision.FallbackReason
		}
		diverged := adaptiveAccountID > 0 && baselineAccountID > 0 && adaptiveAccountID != baselineAccountID
		if diverged {
			s.anthropicAdaptiveScheduler.shadowDivergeTotal.Add(1)
		}
		slog.Info("anthropic_adaptive_shadow_decision",
			"request_id", contextStringValue(ctx, ctxkey.RequestID),
			"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
			"baseline_account_id", baselineAccountID,
			"adaptive_account_id", adaptiveAccountID,
			"selected_account_id", func() int64 {
				if entry.SelectedAccount != nil {
					return entry.SelectedAccount.ID
				}
				return 0
			}(),
			"shadow_diverged", diverged,
			"sticky_would_bypass", entry.StickyWouldBypass,
			"scope", entry.Scope,
			"outcome", entry.Outcome,
			"model", entry.RequestedModel,
			"group_id", derefGroupID(entry.GroupID),
			"candidate_count", candidateCount,
			"top_k", topK,
			"fallback_reason", fallbackReason,
			"latency_ms", anthropicAdaptiveElapsedMilliseconds(entry.StartedAt),
		)
	}
	s.logAnthropicAdaptiveDiagnosticDecision(ctx, settings, entry)
}

func (s *GatewayService) markAnthropicAdaptiveStickyHit(mode string) {
	if mode != "" && s != nil && s.anthropicAdaptiveScheduler != nil {
		s.anthropicAdaptiveScheduler.stickyHitTotal.Add(1)
	}
}
