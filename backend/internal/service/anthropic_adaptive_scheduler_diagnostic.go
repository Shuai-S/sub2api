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
	AccountID                 int64     `json:"account_id"`
	Platform                  string    `json:"platform"`
	AccountType               string    `json:"account_type"`
	ConfiguredCapacity        int       `json:"configured_capacity"`
	EffectiveCapacity         int       `json:"effective_capacity"`
	CurrentConcurrency        int       `json:"current_concurrency"`
	WaitingCount              int       `json:"waiting_count"`
	LoadRate                  int       `json:"load_rate"`
	Score                     float64   `json:"score"`
	ReliabilityScore          float64   `json:"reliability_score"`
	CapacityScore             float64   `json:"capacity_score"`
	TTFTScore                 float64   `json:"ttft_score"`
	CostScore                 float64   `json:"cost_score"`
	LearningStatus            string    `json:"learning_status"`
	HealthSamples             int       `json:"health_samples"`
	SuccessEMA                float64   `json:"success_ema"`
	TTFTEMA                   float64   `json:"ttft_ema"`
	TTFTSamples               int64     `json:"ttft_samples"`
	ConsecutiveFailure        int       `json:"consecutive_failure"`
	HighError                 bool      `json:"high_error"`
	CircuitOpenUntil          time.Time `json:"circuit_open_until"`
	CircuitOpenCount          int       `json:"circuit_open_count"`
	CircuitProbeUntil         time.Time `json:"circuit_probe_until"`
	CircuitStatus             string    `json:"circuit_status"`
	CapacityGeneration        uint64    `json:"capacity_generation"`
	CapacityCooldownUntil     time.Time `json:"capacity_cooldown_until"`
	CapacityRecoverySuccesses int       `json:"capacity_recovery_successes"`
	QuotaLimited              bool      `json:"quota_limited"`
	QuotaResetAt              time.Time `json:"quota_reset_at"`
	QuotaNextProbeAt          time.Time `json:"quota_next_probe_at"`
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
			anthropicAdaptiveCoreSettings(settings),
		)
	}

	var selectedAccountID int64
	var selectedAccountType, selectedPlatform, selectedCircuitStatus string
	var selectedEffectiveCapacity int
	var selectedCircuitOpenUntil, selectedCircuitProbeUntil time.Time
	var selectedHealthSamples, selectedConsecutiveFailure, selectedCircuitOpenCount int
	var selectedQuotaLimited bool
	if entry.SelectedAccount != nil {
		selectedAccountID = entry.SelectedAccount.ID
		selectedAccountType = entry.SelectedAccount.Type
		selectedPlatform = entry.SelectedAccount.Platform
		if s != nil && s.anthropicAdaptiveScheduler != nil && s.anthropicAdaptiveScheduler.core != nil && entry.SelectedAccount.Platform == PlatformAnthropic {
			now := s.anthropicAdaptiveScheduler.now()
			state := s.anthropicAdaptiveScheduler.core.snapshot(entry.SelectedAccount.ID, entry.SelectedAccount.Concurrency, now, anthropicAdaptiveCoreSettings(settings))
			selectedEffectiveCapacity = state.EffectiveCapacity
			selectedHealthSamples = len(state.HealthObservations)
			selectedConsecutiveFailure = state.ConsecutiveFailures
			selectedCircuitOpenUntil = state.CircuitOpenUntil
			selectedCircuitOpenCount = state.CircuitOpenCount
			selectedCircuitProbeUntil = state.HealthProbeUntil
			selectedCircuitStatus = adaptiveDiagnosticCircuitStatus(state, now)
			selectedQuotaLimited = state.QuotaLimited
		}
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
		"selected_health_samples", selectedHealthSamples,
		"selected_consecutive_failure", selectedConsecutiveFailure,
		"selected_circuit_open_until", selectedCircuitOpenUntil,
		"selected_circuit_open_count", selectedCircuitOpenCount,
		"selected_circuit_probe_until", selectedCircuitProbeUntil,
		"selected_circuit_status", selectedCircuitStatus,
		"selected_quota_limited", selectedQuotaLimited,
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
	before adaptiveAccountState,
	after adaptiveAccountState,
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
		"stream", report.Stream,
		"success", report.Success,
		"terminal_reason", report.TerminalReason,
		"health_sample", report.HealthSample,
		"health_scope", report.HealthScope,
		"synthetic", report.Synthetic,
		"first_token_ms", nullableIntForSlog(report.FirstTokenMs),
		"first_token_status", anthropicAdaptiveFirstTokenStatus(report),
		"duration_ms", report.DurationMs,
		"configured_capacity", report.Account.Concurrency,
		"capacity_before", before.EffectiveCapacity,
		"effective_capacity", after.EffectiveCapacity,
		"capacity_decreased", capacityDecreased,
		"success_ema", after.SuccessEMA,
		"ttft_ema", after.TTFTEMA,
		"ttft_samples", after.TTFTSamples,
		"health_samples", len(after.HealthObservations),
		"consecutive_failure", after.ConsecutiveFailures,
		"high_error", after.HighError,
		"circuit_open_until", after.CircuitOpenUntil,
		"circuit_open_count", after.CircuitOpenCount,
		"circuit_probe_until", after.HealthProbeUntil,
		"circuit_status", adaptiveDiagnosticCircuitStatus(after, time.Now()),
		"capacity_generation", after.CapacityGeneration,
		"capacity_cooldown_until", after.CapacityCooldownUntil,
		"capacity_recovery_successes", after.CapacityRecoverySuccesses,
		"quota_limited", after.QuotaLimited,
		"quota_reset_at", after.QuotaResetAt,
		"quota_next_probe_at", after.QuotaNextProbeAt,
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
	settings adaptiveCoreSettings,
) []anthropicAdaptiveDiagnosticCandidate {
	_ = requestedModel
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
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
		learning, healthSamples := adaptiveLearningState(candidate.coreState, candidate.Account.IsOAuth(), now, settings)
		out = append(out, anthropicAdaptiveDiagnosticCandidate{
			AccountID:                 candidate.Account.ID,
			Platform:                  candidate.Account.Platform,
			AccountType:               candidate.Account.Type,
			ConfiguredCapacity:        candidate.Account.Concurrency,
			EffectiveCapacity:         candidate.EffectiveCapacity,
			CurrentConcurrency:        currentConcurrency,
			WaitingCount:              waitingCount,
			LoadRate:                  loadRate,
			Score:                     candidate.Score,
			ReliabilityScore:          candidate.ReliabilityScore,
			CapacityScore:             candidate.CapacityScore,
			TTFTScore:                 candidate.LatencyScore,
			CostScore:                 candidate.CostScore,
			LearningStatus:            string(learning),
			HealthSamples:             healthSamples,
			SuccessEMA:                candidate.coreState.SuccessEMA,
			TTFTEMA:                   candidate.coreState.TTFTEMA,
			TTFTSamples:               candidate.coreState.TTFTSamples,
			ConsecutiveFailure:        candidate.coreState.ConsecutiveFailures,
			HighError:                 candidate.coreState.HighError,
			CircuitOpenUntil:          candidate.coreState.CircuitOpenUntil,
			CircuitOpenCount:          candidate.coreState.CircuitOpenCount,
			CircuitProbeUntil:         candidate.coreState.HealthProbeUntil,
			CircuitStatus:             adaptiveDiagnosticCircuitStatus(candidate.coreState, now),
			CapacityGeneration:        candidate.coreState.CapacityGeneration,
			CapacityCooldownUntil:     candidate.coreState.CapacityCooldownUntil,
			CapacityRecoverySuccesses: candidate.coreState.CapacityRecoverySuccesses,
			QuotaLimited:              candidate.coreState.QuotaLimited,
			QuotaResetAt:              candidate.coreState.QuotaResetAt,
			QuotaNextProbeAt:          candidate.coreState.QuotaNextProbeAt,
		})
	}
	return out
}

func adaptiveDiagnosticCircuitStatus(state adaptiveAccountState, now time.Time) string {
	if !state.CircuitOpenUntil.IsZero() && state.HealthProbeInFlight && state.HealthProbeUntil.After(now) {
		return "half_open_probe_in_flight"
	}
	if state.CircuitOpenUntil.After(now) {
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
