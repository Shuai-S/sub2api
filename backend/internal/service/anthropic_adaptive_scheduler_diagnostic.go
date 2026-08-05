package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const anthropicAdaptiveDiagnosticCandidateLimit = 5

type anthropicAdaptiveDiagnosticCandidate struct {
	AccountID                  int64     `json:"account_id"`
	Platform                   string    `json:"platform"`
	AccountType                string    `json:"account_type"`
	Priority                   int       `json:"priority"`
	ConfiguredCapacity         int       `json:"configured_capacity"`
	EffectiveCapacity          int       `json:"effective_capacity"`
	CurrentConcurrency         int       `json:"current_concurrency"`
	WaitingCount               int       `json:"waiting_count"`
	LoadRate                   int       `json:"load_rate"`
	Score                      float64   `json:"score"`
	ReliabilityScore           float64   `json:"reliability_score"`
	CapacityScore              float64   `json:"capacity_score"`
	LatencyScore               float64   `json:"latency_score"`
	ExplorationScore           float64   `json:"exploration_score"`
	ModelFamily                string    `json:"model_family"`
	ModelTTFTEMA               float64   `json:"model_ttft_ema"`
	ModelLatencyEMA            float64   `json:"model_latency_ema"`
	ModelLatencySamples        int64     `json:"model_latency_samples"`
	ModelSuccessEMA            float64   `json:"model_success_ema"`
	ModelHealthSamples         int64     `json:"model_health_samples"`
	ModelConsecutiveFailure    int       `json:"model_consecutive_failure"`
	SuccessEMA                 float64   `json:"success_ema"`
	TotalSamples               int64     `json:"total_samples"`
	RecentHealthSamples        int       `json:"recent_health_samples"`
	RecentHealthFailures       int       `json:"recent_health_failures"`
	RecentCapacitySamples      int       `json:"recent_capacity_samples"`
	RecentCapacityFailures     int       `json:"recent_capacity_failures"`
	ConsecutiveSuccess         int       `json:"consecutive_success"`
	ConsecutiveFailure         int       `json:"consecutive_failure"`
	ConsecutiveCapacityFailure int       `json:"consecutive_capacity_failure"`
	AccountHealthSamples       int       `json:"account_health_samples"`
	AccountHealthFailures      int       `json:"account_health_failures"`
	AccountConsecutiveFailure  int       `json:"account_consecutive_failure"`
	CooldownUntil              time.Time `json:"cooldown_until"`
	CooldownStatus             string    `json:"cooldown_status"`
	CircuitOpenUntil           time.Time `json:"circuit_open_until"`
	CircuitProbeUntil          time.Time `json:"circuit_probe_until"`
	CircuitStatus              string    `json:"circuit_status"`
}

type anthropicAdaptiveDecisionLog struct {
	Mode              string
	Scope             string
	Outcome           string
	RequestedModel    string
	Platform          string
	GroupID           *int64
	SessionHash       string
	StickyAccountID   int64
	BaselineAccountID int64
	Decision          *AnthropicAdaptiveDecision
	SelectedAccount   *Account
	Sticky            bool
	StickyWouldBypass bool
	Force             bool
	Err               error
	StartedAt         time.Time
	ExcludedCount     int
}

type anthropicAdaptiveBypassLog struct {
	Reason               string
	Mode                 string
	Scope                string
	RequestedModel       string
	Platform             string
	GroupID              *int64
	SessionHash          string
	StickyAccountID      int64
	AccountCount         int
	NativeCandidateCount int
	MixedCandidateCount  int
	LoadBatchEnabled     bool
	HasConcurrency       bool
	Force                bool
	Err                  error
	StartedAt            time.Time
}

func (s *GatewayService) logAnthropicAdaptiveDiagnosticDecision(
	ctx context.Context,
	settings AnthropicAdaptiveSchedulerSettings,
	entry anthropicAdaptiveDecisionLog,
) {
	if !shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, entry.RequestedModel, entry.Force) {
		return
	}

	var adaptiveAccountID int64
	var candidateCount, topK int
	var buildLatencyMs int64
	var fallbackReason string
	var candidates []anthropicAdaptiveDiagnosticCandidate
	if entry.Decision != nil {
		adaptiveAccountID = entry.Decision.SelectedAccountID
		candidateCount = entry.Decision.CandidateCount
		topK = entry.Decision.TopK
		buildLatencyMs = entry.Decision.BuildLatencyMs
		fallbackReason = entry.Decision.FallbackReason
		candidates = anthropicAdaptiveDiagnosticCandidates(
			entry.Decision.Order,
			entry.RequestedModel,
			anthropicAdaptiveDiagnosticCandidateLimit,
			time.Now(),
		)
	}

	var selectedAccountID int64
	var selectedAccountType, selectedPlatform, selectedCooldownStatus, selectedCircuitStatus string
	var selectedEffectiveCapacity int
	var selectedCooldownUntil, selectedCircuitOpenUntil, selectedCircuitProbeUntil time.Time
	var selectedAccountHealthSamples, selectedAccountHealthFailures, selectedAccountConsecutiveFailure int
	if entry.SelectedAccount != nil {
		selectedAccountID = entry.SelectedAccount.ID
		selectedAccountType = entry.SelectedAccount.Type
		selectedPlatform = entry.SelectedAccount.Platform
		if s != nil && s.anthropicAdaptiveScheduler != nil && s.anthropicAdaptiveScheduler.state != nil && entry.SelectedAccount.Platform == PlatformAnthropic {
			state := s.anthropicAdaptiveScheduler.state.snapshot(entry.SelectedAccount, settings)
			selectedEffectiveCapacity = s.anthropicAdaptiveScheduler.state.effectiveCapacity(entry.SelectedAccount, settings)
			selectedCooldownUntil = state.CooldownUntil
			selectedCooldownStatus = anthropicAdaptiveCooldownStatus(state, time.Now())
			selectedAccountHealthSamples = state.AccountHealthSamples
			selectedAccountHealthFailures = state.AccountHealthFailures
			selectedAccountConsecutiveFailure = state.AccountConsecutiveFailure
			selectedCircuitOpenUntil = state.CircuitOpenUntil
			selectedCircuitProbeUntil = state.CircuitProbeUntil
			selectedCircuitStatus = anthropicAdaptiveCircuitStatus(state, time.Now())
		}
	}
	if selectedCooldownStatus == "" {
		selectedCooldownStatus = "none"
	}
	baselineAccountID := entry.BaselineAccountID
	if baselineAccountID == 0 && selectedAccountID > 0 && (entry.Mode == AnthropicAdaptiveSchedulerModeShadow || entry.Sticky) {
		baselineAccountID = selectedAccountID
	}
	platform := entry.Platform
	if platform == "" {
		platform = selectedPlatform
	}
	if platform == "" {
		platform = PlatformAnthropic
	}
	latencyMs := anthropicAdaptiveElapsedMilliseconds(entry.StartedAt)
	accountSwitchCount, accountSwitchCountSource := anthropicAdaptiveAccountSwitchCount(ctx, entry.ExcludedCount)
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"mode", entry.Mode,
		"scope", entry.Scope,
		"outcome", entry.Outcome,
		"model", entry.RequestedModel,
		"model_family", anthropicAdaptiveModelFamily(entry.RequestedModel),
		"platform", platform,
		"group_id", derefGroupID(entry.GroupID),
		"session_sticky", entry.SessionHash != "",
		"session", shortSessionHash(entry.SessionHash),
		"sticky_account_id", entry.StickyAccountID,
		"sticky_selected", entry.Sticky,
		"sticky_would_bypass", entry.StickyWouldBypass,
		"baseline_account_id", baselineAccountID,
		"adaptive_account_id", adaptiveAccountID,
		"selected_account_id", selectedAccountID,
		"selected_account_type", selectedAccountType,
		"selected_platform", selectedPlatform,
		"selection_matches_adaptive", selectedAccountID > 0 && selectedAccountID == adaptiveAccountID,
		"selection_matches_sticky", selectedAccountID > 0 && selectedAccountID == entry.StickyAccountID,
		"selected_effective_capacity", selectedEffectiveCapacity,
		"selected_cooldown_until", selectedCooldownUntil,
		"selected_cooldown_status", selectedCooldownStatus,
		"selected_account_health_samples", selectedAccountHealthSamples,
		"selected_account_health_failures", selectedAccountHealthFailures,
		"selected_account_consecutive_failure", selectedAccountConsecutiveFailure,
		"selected_circuit_open_until", selectedCircuitOpenUntil,
		"selected_circuit_probe_until", selectedCircuitProbeUntil,
		"selected_circuit_status", selectedCircuitStatus,
		"candidate_count", candidateCount,
		"top_k", topK,
		"excluded_count", entry.ExcludedCount,
		"account_switch_count", accountSwitchCount,
		"account_switch_count_source", accountSwitchCountSource,
		"attempt_number", accountSwitchCount + 1,
		"build_latency_ms", buildLatencyMs,
		"latency_ms", latencyMs,
		"fallback_reason", fallbackReason,
		"diagnostic_sample_rate", settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate,
		"candidates", candidates,
	}
	fields = append(fields, anthropicAdaptiveErrorLogFields(entry.Err)...)
	slog.Info("anthropic_adaptive_scheduler_diagnostic_decision", fields...)
}

func (s *GatewayService) logAnthropicAdaptiveDiagnosticBypass(
	ctx context.Context,
	settings AnthropicAdaptiveSchedulerSettings,
	entry anthropicAdaptiveBypassLog,
) {
	settingsReadFailed := entry.Reason == "settings_read_failed"
	force := entry.Force || entry.Reason != ""
	if !settingsReadFailed && !shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, entry.RequestedModel, force) {
		return
	}
	mode := entry.Mode
	if mode == "" && settings.AnthropicAdaptiveSchedulerEnabled {
		mode = normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
	}
	accountSwitchCount, accountSwitchCountSource := anthropicAdaptiveAccountSwitchCount(ctx, 0)
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"reason", entry.Reason,
		"enabled", settings.AnthropicAdaptiveSchedulerEnabled,
		"mode", mode,
		"scope", entry.Scope,
		"model", entry.RequestedModel,
		"platform", entry.Platform,
		"group_id", derefGroupID(entry.GroupID),
		"session_sticky", entry.SessionHash != "",
		"session", shortSessionHash(entry.SessionHash),
		"sticky_account_id", entry.StickyAccountID,
		"account_count", entry.AccountCount,
		"native_candidate_count", entry.NativeCandidateCount,
		"mixed_candidate_count", entry.MixedCandidateCount,
		"load_batch_enabled", entry.LoadBatchEnabled,
		"has_concurrency_service", entry.HasConcurrency,
		"account_switch_count", accountSwitchCount,
		"account_switch_count_source", accountSwitchCountSource,
		"attempt_number", accountSwitchCount + 1,
		"latency_ms", anthropicAdaptiveElapsedMilliseconds(entry.StartedAt),
		"diagnostic_sample_rate", settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate,
	}
	fields = append(fields, anthropicAdaptiveErrorLogFields(entry.Err)...)
	if settingsReadFailed {
		slog.Warn("anthropic_adaptive_scheduler_diagnostic_bypass", fields...)
		return
	}
	slog.Info("anthropic_adaptive_scheduler_diagnostic_bypass", fields...)
}

func (s *GatewayService) logAnthropicAdaptiveDiagnosticResult(
	ctx context.Context,
	settings AnthropicAdaptiveSchedulerSettings,
	report AnthropicAdaptiveScheduleReport,
	before anthropicAdaptiveAccountState,
	after anthropicAdaptiveAccountState,
	capacityDecreased bool,
	err error,
) {
	if report.Account == nil {
		return
	}
	force := capacityDecreased || (err != nil && report.TerminalReason != "request_error" && report.TerminalReason != "client_cancelled")
	if !shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, report.RequestedModel, force) {
		return
	}
	family := anthropicAdaptiveModelFamily(report.RequestedModel)
	latency := after.LatencyByModelFamily[family]
	modelHealth := after.HealthByModelFamily[family]
	accountSwitchCount, accountSwitchCountSource := anthropicAdaptiveAccountSwitchCount(ctx, 0)
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"upstream_request_id", report.UpstreamRequestID,
		"account_switch_count", accountSwitchCount,
		"account_switch_count_source", accountSwitchCountSource,
		"attempt_number", accountSwitchCount + 1,
		"mode", normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode),
		"account_id", report.Account.ID,
		"account_type", report.Account.Type,
		"platform", report.Account.Platform,
		"model", report.RequestedModel,
		"mapped_model", report.MappedModel,
		"model_family", family,
		"stream", report.Stream,
		"success", report.Success,
		"terminal_reason", report.TerminalReason,
		"health_sample", report.HealthSample,
		"health_scope", report.HealthScope,
		"capacity_sample", report.CapacitySample,
		"synthetic", report.Synthetic,
		"first_token_ms", nullableIntForSlog(report.FirstTokenMs),
		"first_token_status", anthropicAdaptiveFirstTokenStatus(report),
		"duration_ms", report.DurationMs,
		"configured_capacity", report.Account.Concurrency,
		"capacity_before", before.EstimatedCapacity,
		"estimated_capacity", after.EstimatedCapacity,
		"capacity_decreased", capacityDecreased,
		"success_ema", after.SuccessEMA,
		"model_success_ema", modelHealth.SuccessEMA,
		"model_health_samples", modelHealth.TotalSamples,
		"model_consecutive_failure", modelHealth.ConsecutiveFailure,
		"model_ttft_ema", latency.TTFTEMA,
		"model_latency_ema", latency.LatencyEMA,
		"model_latency_samples", latency.Samples,
		"total_samples", after.TotalSamples,
		"recent_health_samples", after.RecentHealthSamples,
		"recent_health_failures", after.RecentHealthFailures,
		"recent_capacity_samples", after.RecentCapacitySamples,
		"recent_capacity_failures", after.RecentCapacityFailures,
		"consecutive_success", after.ConsecutiveSuccess,
		"consecutive_failure", after.ConsecutiveFailure,
		"consecutive_capacity_failure", after.ConsecutiveCapacityFailure,
		"account_health_samples", after.AccountHealthSamples,
		"account_health_failures", after.AccountHealthFailures,
		"account_consecutive_failure", after.AccountConsecutiveFailure,
		"cooldown_until", after.CooldownUntil,
		"cooldown_status", anthropicAdaptiveCooldownStatus(after, time.Now()),
		"circuit_open_until", after.CircuitOpenUntil,
		"circuit_probe_until", after.CircuitProbeUntil,
		"circuit_status", anthropicAdaptiveCircuitStatus(after, time.Now()),
		"diagnostic_sample_rate", settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate,
	}
	fields = append(fields, anthropicAdaptiveErrorLogFields(err)...)
	slog.Info("anthropic_adaptive_scheduler_diagnostic_result", fields...)
}

func anthropicAdaptiveAccountSwitchCount(ctx context.Context, fallback int) (int, string) {
	if value, ok := AccountSwitchCountFromContext(ctx); ok {
		return value, "context"
	}
	if fallback > 0 {
		return fallback, "excluded_ids"
	}
	return 0, "unavailable"
}

func shouldLogAnthropicAdaptiveDiagnostic(
	ctx context.Context,
	settings AnthropicAdaptiveSchedulerSettings,
	requestedModel string,
	force bool,
) bool {
	if !settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled {
		return false
	}
	if force {
		return true
	}
	rate := settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	requestID := contextStringValue(ctx, ctxkey.RequestID)
	clientRequestID := contextStringValue(ctx, ctxkey.ClientRequestID)
	seedText := requestID + "\x00" + clientRequestID
	if requestID == "" && clientRequestID == "" {
		seedText = strings.TrimSpace(requestedModel)
	}
	if strings.Trim(seedText, "\x00") == "" {
		seedText = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	const buckets = uint64(1_000_000)
	return float64(hashString64(seedText)%buckets)/float64(buckets) < rate
}

func anthropicAdaptiveDiagnosticCandidates(
	candidates []AnthropicAdaptiveCandidate,
	requestedModel string,
	limit int,
	now time.Time,
) []anthropicAdaptiveDiagnosticCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
	family := anthropicAdaptiveModelFamily(requestedModel)
	out := make([]anthropicAdaptiveDiagnosticCandidate, 0, limit)
	for _, candidate := range candidates[:limit] {
		if candidate.Account == nil {
			continue
		}
		var currentConcurrency, waitingCount, loadRate int
		if candidate.LoadInfo != nil {
			currentConcurrency = candidate.LoadInfo.CurrentConcurrency
			waitingCount = candidate.LoadInfo.WaitingCount
			loadRate = candidate.LoadInfo.LoadRate
		}
		latency := candidate.state.LatencyByModelFamily[family]
		health := candidate.state.HealthByModelFamily[family]
		out = append(out, anthropicAdaptiveDiagnosticCandidate{
			AccountID:                  candidate.Account.ID,
			Platform:                   candidate.Account.Platform,
			AccountType:                candidate.Account.Type,
			Priority:                   candidate.Account.Priority,
			ConfiguredCapacity:         candidate.Account.Concurrency,
			EffectiveCapacity:          candidate.EffectiveCapacity,
			CurrentConcurrency:         currentConcurrency,
			WaitingCount:               waitingCount,
			LoadRate:                   loadRate,
			Score:                      candidate.Score,
			ReliabilityScore:           candidate.ReliabilityScore,
			CapacityScore:              candidate.CapacityScore,
			LatencyScore:               candidate.LatencyScore,
			ExplorationScore:           candidate.ExplorationScore,
			ModelFamily:                family,
			ModelTTFTEMA:               latency.TTFTEMA,
			ModelLatencyEMA:            latency.LatencyEMA,
			ModelLatencySamples:        latency.Samples,
			ModelSuccessEMA:            health.SuccessEMA,
			ModelHealthSamples:         health.TotalSamples,
			ModelConsecutiveFailure:    health.ConsecutiveFailure,
			SuccessEMA:                 candidate.state.SuccessEMA,
			TotalSamples:               candidate.state.TotalSamples,
			RecentHealthSamples:        candidate.state.RecentHealthSamples,
			RecentHealthFailures:       candidate.state.RecentHealthFailures,
			RecentCapacitySamples:      candidate.state.RecentCapacitySamples,
			RecentCapacityFailures:     candidate.state.RecentCapacityFailures,
			ConsecutiveSuccess:         candidate.state.ConsecutiveSuccess,
			ConsecutiveFailure:         candidate.state.ConsecutiveFailure,
			ConsecutiveCapacityFailure: candidate.state.ConsecutiveCapacityFailure,
			AccountHealthSamples:       candidate.state.AccountHealthSamples,
			AccountHealthFailures:      candidate.state.AccountHealthFailures,
			AccountConsecutiveFailure:  candidate.state.AccountConsecutiveFailure,
			CooldownUntil:              candidate.state.CooldownUntil,
			CooldownStatus:             anthropicAdaptiveCooldownStatus(candidate.state, now),
			CircuitOpenUntil:           candidate.state.CircuitOpenUntil,
			CircuitProbeUntil:          candidate.state.CircuitProbeUntil,
			CircuitStatus:              anthropicAdaptiveCircuitStatus(candidate.state, now),
		})
	}
	return out
}

func anthropicAdaptiveCooldownStatus(state anthropicAdaptiveAccountState, now time.Time) string {
	if state.CooldownUntil.IsZero() {
		return "none"
	}
	if state.CooldownUntil.After(now) {
		return "active"
	}
	return "expired"
}

func anthropicAdaptiveCircuitStatus(state anthropicAdaptiveAccountState, now time.Time) string {
	if state.CircuitOpenUntil.After(now) {
		if state.CircuitProbeInFlight && state.CircuitProbeUntil.After(now) {
			return "half_open_probe_in_flight"
		}
		return "open"
	}
	if !state.CircuitOpenUntil.IsZero() {
		return "half_open_ready"
	}
	return "closed"
}

func anthropicAdaptiveFirstTokenStatus(report AnthropicAdaptiveScheduleReport) string {
	if report.FirstTokenMs != nil && *report.FirstTokenMs > 0 {
		return "recorded"
	}
	if report.FirstTokenMs != nil {
		return "zero_value"
	}
	if !report.Stream {
		return "not_applicable"
	}
	if report.Success {
		return "stream_first_token_missing"
	}
	return "stream_failed_before_first_token"
}

func anthropicAdaptiveElapsedMilliseconds(startedAt time.Time) int64 {
	if startedAt.IsZero() {
		return 0
	}
	elapsed := time.Since(startedAt).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func anthropicAdaptiveErrorLogFields(err error) []any {
	if err == nil {
		return nil
	}
	message := logredact.RedactText(truncateForLog([]byte(err.Error()), 1024))
	fields := []any{"error", message, "error_type", fmt.Sprintf("%T", err)}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		fields = append(fields,
			"upstream_status", failoverErr.StatusCode,
			"failure_stage", failoverErr.Stage,
			"failure_scope", failoverErr.Scope,
			"failure_reason", failoverErr.Reason,
			"failure_kind", failoverErr.FailureKind,
			"client_status", failoverErr.ClientStatusCode,
			"retryable_same_account", failoverErr.RetryableOnSameAccount,
			"retry_next_account", failoverErr.ShouldRetryNextAccount(),
		)
	}
	return fields
}
