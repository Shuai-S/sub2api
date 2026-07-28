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

const geminiAdaptiveDiagnosticCandidateLimit = 5

type geminiAdaptiveDiagnosticCandidate struct {
	AccountID                  int64                       `json:"account_id"`
	Platform                   string                      `json:"platform"`
	AccountType                string                      `json:"account_type"`
	Priority                   int                         `json:"priority"`
	ConfiguredCapacity         int                         `json:"configured_capacity"`
	EffectiveCapacity          int                         `json:"effective_capacity"`
	CurrentConcurrency         int                         `json:"current_concurrency"`
	WaitingCount               int                         `json:"waiting_count"`
	LoadRate                   int                         `json:"load_rate"`
	Score                      float64                     `json:"score"`
	ReliabilityScore           float64                     `json:"reliability_score"`
	QuotaScore                 float64                     `json:"quota_score"`
	CapacityScore              float64                     `json:"capacity_score"`
	LatencyScore               float64                     `json:"latency_score"`
	CostScore                  float64                     `json:"cost_score"`
	ExplorationScore           float64                     `json:"exploration_score"`
	Quota                      GeminiAdaptiveQuotaSnapshot `json:"quota"`
	PathSuccessEMA             float64                     `json:"path_success_ema"`
	ModelFamily                string                      `json:"model_family"`
	ModelSuccessEMA            float64                     `json:"model_success_ema"`
	ModelTTFTEMA               float64                     `json:"model_ttft_ema"`
	ModelLatencyEMA            float64                     `json:"model_latency_ema"`
	ModelSamples               int64                       `json:"model_samples"`
	ModelFailures              int64                       `json:"model_failures"`
	TotalSamples               int64                       `json:"total_samples"`
	RecentHealthSamples        int                         `json:"recent_health_samples"`
	RecentHealthFailures       int                         `json:"recent_health_failures"`
	RecentCapacitySamples      int                         `json:"recent_capacity_samples"`
	RecentCapacityFailures     int                         `json:"recent_capacity_failures"`
	ConsecutiveFailure         int                         `json:"consecutive_failure"`
	ConsecutiveCapacityFailure int                         `json:"consecutive_capacity_failure"`
	CooldownUntil              time.Time                   `json:"cooldown_until"`
	CooldownStatus             string                      `json:"cooldown_status"`
}

type geminiAdaptiveDecisionLog struct {
	Mode               string
	Scope              string
	Outcome            string
	RequestedModel     string
	GroupID            *int64
	SessionHash        string
	Decision           *GeminiAdaptiveDecision
	SelectedAccount    *Account
	Sticky             bool
	StickyWouldMigrate bool
	Force              bool
	Err                error
}

func (s *GatewayService) logGeminiAdaptiveDiagnosticDecision(ctx context.Context, settings GeminiAdaptiveSchedulerSettings, entry geminiAdaptiveDecisionLog) {
	hint := geminiAdaptiveHintFromContext(ctx)
	if !shouldLogGeminiAdaptiveDiagnostic(ctx, settings, entry.RequestedModel, hint.Action, entry.Force) {
		return
	}

	var plannedBaselineAccountID, baselineAccountID, adaptiveAccountID int64
	var candidateCount, inputCandidateCount, hardRejectedCount, topK int
	var buildLatencyMs int64
	var fallbackReason string
	var candidates []geminiAdaptiveDiagnosticCandidate
	if entry.Decision != nil {
		plannedBaselineAccountID = entry.Decision.BaselineAccountID
		baselineAccountID = plannedBaselineAccountID
		adaptiveAccountID = entry.Decision.SelectedAccountID
		candidateCount = entry.Decision.CandidateCount
		inputCandidateCount = entry.Decision.InputCandidateCount
		hardRejectedCount = entry.Decision.HardRejectedCount
		topK = entry.Decision.TopK
		buildLatencyMs = entry.Decision.BuildLatencyMs
		fallbackReason = entry.Decision.FallbackReason
		candidates = geminiAdaptiveDiagnosticCandidates(entry.Decision.Order, entry.RequestedModel, hint.Action, geminiAdaptiveDiagnosticCandidateLimit, time.Now())
	}

	var selectedAccountID int64
	var selectedAccountType, selectedPlatform string
	if entry.SelectedAccount != nil {
		selectedAccountID = entry.SelectedAccount.ID
		selectedAccountType = entry.SelectedAccount.Type
		selectedPlatform = entry.SelectedAccount.Platform
	}
	if selectedAccountID > 0 && (entry.Mode == GeminiAdaptiveSchedulerModeShadow || entry.Sticky) {
		baselineAccountID = selectedAccountID
	}
	diverged := baselineAccountID > 0 && adaptiveAccountID > 0 && baselineAccountID != adaptiveAccountID
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"mode", entry.Mode,
		"scope", entry.Scope,
		"outcome", entry.Outcome,
		"model", entry.RequestedModel,
		"model_family", geminiAdaptiveModelFamily(entry.RequestedModel, hint.Action),
		"action", hint.Action,
		"stream", hint.Stream,
		"group_id", derefGroupID(entry.GroupID),
		"session_sticky", entry.SessionHash != "",
		"session", shortSessionHash(entry.SessionHash),
		"sticky", entry.Sticky,
		"sticky_would_migrate", entry.StickyWouldMigrate,
		"planned_baseline_account_id", plannedBaselineAccountID,
		"baseline_account_id", baselineAccountID,
		"adaptive_account_id", adaptiveAccountID,
		"selected_account_id", selectedAccountID,
		"selected_account_type", selectedAccountType,
		"selected_platform", selectedPlatform,
		"shadow_diverged", diverged,
		"selection_matches_adaptive", selectedAccountID > 0 && selectedAccountID == adaptiveAccountID,
		"input_candidate_count", inputCandidateCount,
		"candidate_count", candidateCount,
		"native_candidate_count", candidateCount,
		"hard_rejected_count", hardRejectedCount,
		"top_k", topK,
		"build_latency_ms", buildLatencyMs,
		"fallback_reason", fallbackReason,
		"diagnostic_sample_rate", settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate,
		"candidates", candidates,
	}
	fields = append(fields, geminiAdaptiveErrorLogFields(entry.Err)...)
	event := "gemini_adaptive_scheduler_diagnostic_decision"
	if entry.Mode == GeminiAdaptiveSchedulerModeShadow {
		event = "gemini_adaptive_shadow_decision"
	}
	slog.Info(event, fields...)
}

func (s *GatewayService) logGeminiAdaptiveDiagnosticResult(
	ctx context.Context,
	settings GeminiAdaptiveSchedulerSettings,
	report GeminiAdaptiveScheduleReport,
	before geminiAdaptiveAccountState,
	after geminiAdaptiveAccountState,
	capacityIncreased bool,
	capacityDecreased bool,
	err error,
) {
	if report.Account == nil {
		return
	}
	force := capacityIncreased || capacityDecreased || (err != nil && report.TerminalReason != "request_error" && report.TerminalReason != "client_cancelled")
	if !shouldLogGeminiAdaptiveDiagnostic(ctx, settings, report.RequestedModel, report.Action, force) {
		return
	}
	family := geminiAdaptiveModelFamily(firstNonEmpty(report.MappedModel, report.RequestedModel), report.Action)
	modelState := after.ByModelFamily[family]
	accountSwitchCount, _ := AccountSwitchCountFromContext(ctx)
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"upstream_request_id", report.UpstreamRequestID,
		"account_switch_count", accountSwitchCount,
		"attempt_number", accountSwitchCount + 1,
		"account_id", report.Account.ID,
		"account_type", report.Account.Type,
		"platform", report.Account.Platform,
		"model", report.RequestedModel,
		"mapped_model", report.MappedModel,
		"model_family", family,
		"action", report.Action,
		"stream", report.Stream,
		"success", report.Success,
		"terminal_reason", report.TerminalReason,
		"path_sample", report.PathSample,
		"model_sample", report.ModelSample,
		"capacity_sample", report.CapacitySample,
		"synthetic", report.Synthetic,
		"first_token_ms", nullableIntForSlog(report.FirstTokenMs),
		"first_token_status", geminiAdaptiveFirstTokenStatus(report),
		"duration_ms", report.DurationMs,
		"configured_capacity", report.Account.Concurrency,
		"capacity_before", before.EstimatedCapacity,
		"estimated_capacity", after.EstimatedCapacity,
		"capacity_increased", capacityIncreased,
		"capacity_decreased", capacityDecreased,
		"path_success_ema", after.PathSuccessEMA,
		"model_success_ema", modelState.SuccessEMA,
		"model_ttft_ema", modelState.TTFTEMA,
		"model_latency_ema", modelState.LatencyEMA,
		"model_samples", modelState.Samples,
		"model_failures", modelState.Failures,
		"total_samples", after.TotalSamples,
		"recent_health_samples", after.RecentHealthSamples,
		"recent_health_failures", after.RecentHealthFailures,
		"recent_capacity_samples", after.RecentCapacitySamples,
		"recent_capacity_failures", after.RecentCapacityFailures,
		"consecutive_success", after.ConsecutiveSuccess,
		"consecutive_failure", after.ConsecutiveFailure,
		"consecutive_capacity_failure", after.ConsecutiveCapacityFailure,
		"cooldown_until", after.CooldownUntil,
		"cooldown_status", geminiAdaptiveCooldownStatus(after, time.Now()),
		"diagnostic_sample_rate", settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate,
	}
	fields = append(fields, geminiAdaptiveErrorLogFields(err)...)
	slog.Info("gemini_adaptive_scheduler_diagnostic_result", fields...)
}

func shouldLogGeminiAdaptiveDiagnostic(ctx context.Context, settings GeminiAdaptiveSchedulerSettings, requestedModel, action string, force bool) bool {
	if !settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled {
		return false
	}
	if force {
		return true
	}
	rate := settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate
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
		seedText = strings.TrimSpace(requestedModel) + "\x00" + strings.TrimSpace(action)
	}
	if strings.Trim(seedText, "\x00") == "" {
		seedText = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	const buckets = uint64(1_000_000)
	return float64(hashString64(seedText)%buckets)/float64(buckets) < rate
}

func geminiAdaptiveDiagnosticCandidates(candidates []GeminiAdaptiveCandidate, requestedModel, action string, limit int, now time.Time) []geminiAdaptiveDiagnosticCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
	family := geminiAdaptiveModelFamily(requestedModel, action)
	out := make([]geminiAdaptiveDiagnosticCandidate, 0, limit)
	for _, candidate := range candidates[:limit] {
		if candidate.Account == nil {
			continue
		}
		var currentConcurrency, waitingCount, loadRate int
		if candidate.Load != nil {
			currentConcurrency = candidate.Load.CurrentConcurrency
			waitingCount = candidate.Load.WaitingCount
			loadRate = candidate.Load.LoadRate
		}
		modelState := candidate.state.ByModelFamily[family]
		out = append(out, geminiAdaptiveDiagnosticCandidate{
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
			QuotaScore:                 candidate.QuotaScore,
			CapacityScore:              candidate.CapacityScore,
			LatencyScore:               candidate.LatencyScore,
			CostScore:                  candidate.CostScore,
			ExplorationScore:           candidate.ExplorationScore,
			Quota:                      candidate.Quota,
			PathSuccessEMA:             candidate.state.PathSuccessEMA,
			ModelFamily:                family,
			ModelSuccessEMA:            modelState.SuccessEMA,
			ModelTTFTEMA:               modelState.TTFTEMA,
			ModelLatencyEMA:            modelState.LatencyEMA,
			ModelSamples:               modelState.Samples,
			ModelFailures:              modelState.Failures,
			TotalSamples:               candidate.state.TotalSamples,
			RecentHealthSamples:        candidate.state.RecentHealthSamples,
			RecentHealthFailures:       candidate.state.RecentHealthFailures,
			RecentCapacitySamples:      candidate.state.RecentCapacitySamples,
			RecentCapacityFailures:     candidate.state.RecentCapacityFailures,
			ConsecutiveFailure:         candidate.state.ConsecutiveFailure,
			ConsecutiveCapacityFailure: candidate.state.ConsecutiveCapacityFailure,
			CooldownUntil:              candidate.state.CooldownUntil,
			CooldownStatus:             geminiAdaptiveCooldownStatus(candidate.state, now),
		})
	}
	return out
}

func geminiAdaptiveFirstTokenStatus(report GeminiAdaptiveScheduleReport) string {
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

func geminiAdaptiveCooldownStatus(state geminiAdaptiveAccountState, now time.Time) string {
	if state.CooldownUntil.IsZero() {
		return "none"
	}
	if state.CooldownUntil.After(now) {
		return "active"
	}
	return "expired"
}

func geminiAdaptiveErrorLogFields(err error) []any {
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
