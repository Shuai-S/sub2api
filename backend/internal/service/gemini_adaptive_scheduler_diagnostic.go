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
	AccountID                 int64                       `json:"account_id"`
	Platform                  string                      `json:"platform"`
	AccountType               string                      `json:"account_type"`
	ConfiguredCapacity        int                         `json:"configured_capacity"`
	EffectiveCapacity         int                         `json:"effective_capacity"`
	CurrentConcurrency        int                         `json:"current_concurrency"`
	WaitingCount              int                         `json:"waiting_count"`
	LoadRate                  int                         `json:"load_rate"`
	Score                     float64                     `json:"score"`
	ReliabilityScore          float64                     `json:"reliability_score"`
	CapacityScore             float64                     `json:"capacity_score"`
	TTFTScore                 float64                     `json:"ttft_score"`
	CostScore                 float64                     `json:"cost_score"`
	Quota                     GeminiAdaptiveQuotaSnapshot `json:"quota"`
	LearningStatus            string                      `json:"learning_status"`
	HealthSamples             int                         `json:"health_samples"`
	SuccessEMA                float64                     `json:"success_ema"`
	TTFTEMA                   float64                     `json:"ttft_ema"`
	TTFTSamples               int64                       `json:"ttft_samples"`
	ConsecutiveFailure        int                         `json:"consecutive_failure"`
	HighError                 bool                        `json:"high_error"`
	CircuitStatus             string                      `json:"circuit_status"`
	CircuitOpenUntil          time.Time                   `json:"circuit_open_until"`
	CircuitOpenCount          int                         `json:"circuit_open_count"`
	CapacityGeneration        uint64                      `json:"capacity_generation"`
	CapacityCooldownUntil     time.Time                   `json:"capacity_cooldown_until"`
	CapacityRecoverySuccesses int                         `json:"capacity_recovery_successes"`
	QuotaLimited              bool                        `json:"quota_limited"`
	QuotaResetAt              time.Time                   `json:"quota_reset_at"`
	QuotaNextProbeAt          time.Time                   `json:"quota_next_probe_at"`
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
	var candidateCount, inputCandidateCount, hardRejectedCount, circuitRejectedCount, halfOpenCandidateCount, topK int
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
		circuitRejectedCount = entry.Decision.CircuitRejectedCount
		halfOpenCandidateCount = entry.Decision.HalfOpenCandidateCount
		topK = entry.Decision.TopK
		buildLatencyMs = entry.Decision.BuildLatencyMs
		fallbackReason = entry.Decision.FallbackReason
		candidates = geminiAdaptiveDiagnosticCandidates(entry.Decision.Order, entry.RequestedModel, hint.Action, geminiAdaptiveDiagnosticCandidateLimit, time.Now(), geminiAdaptiveCoreSettings(settings))
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
		"circuit_rejected_count", circuitRejectedCount,
		"half_open_candidate_count", halfOpenCandidateCount,
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
	before adaptiveAccountState,
	after adaptiveAccountState,
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
		"action", report.Action,
		"stream", report.Stream,
		"success", report.Success,
		"terminal_reason", report.TerminalReason,
		"path_sample", report.PathSample,
		"synthetic", report.Synthetic,
		"first_token_ms", nullableIntForSlog(report.FirstTokenMs),
		"first_token_status", geminiAdaptiveFirstTokenStatus(report),
		"duration_ms", report.DurationMs,
		"configured_capacity", report.Account.Concurrency,
		"capacity_before", before.EffectiveCapacity,
		"effective_capacity", after.EffectiveCapacity,
		"capacity_increased", capacityIncreased,
		"capacity_decreased", capacityDecreased,
		"success_ema", after.SuccessEMA,
		"ttft_ema", after.TTFTEMA,
		"ttft_samples", after.TTFTSamples,
		"health_samples", len(after.HealthObservations),
		"consecutive_failure", after.ConsecutiveFailures,
		"high_error", after.HighError,
		"circuit_status", adaptiveDiagnosticCircuitStatus(after, time.Now()),
		"circuit_open_until", after.CircuitOpenUntil,
		"circuit_open_count", after.CircuitOpenCount,
		"capacity_generation", after.CapacityGeneration,
		"capacity_cooldown_until", after.CapacityCooldownUntil,
		"capacity_recovery_successes", after.CapacityRecoverySuccesses,
		"quota_limited", after.QuotaLimited,
		"quota_reset_at", after.QuotaResetAt,
		"quota_next_probe_at", after.QuotaNextProbeAt,
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

func geminiAdaptiveDiagnosticCandidates(candidates []GeminiAdaptiveCandidate, requestedModel, action string, limit int, now time.Time, settings adaptiveCoreSettings) []geminiAdaptiveDiagnosticCandidate {
	_, _ = requestedModel, action
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
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
		learning, healthSamples := adaptiveLearningState(candidate.coreState, candidate.Account.IsOAuth(), now, settings)
		out = append(out, geminiAdaptiveDiagnosticCandidate{
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
			Quota:                     candidate.Quota,
			LearningStatus:            string(learning),
			HealthSamples:             healthSamples,
			SuccessEMA:                candidate.coreState.SuccessEMA,
			TTFTEMA:                   candidate.coreState.TTFTEMA,
			TTFTSamples:               candidate.coreState.TTFTSamples,
			ConsecutiveFailure:        candidate.coreState.ConsecutiveFailures,
			HighError:                 candidate.coreState.HighError,
			CircuitStatus:             adaptiveDiagnosticCircuitStatus(candidate.coreState, now),
			CircuitOpenUntil:          candidate.coreState.CircuitOpenUntil,
			CircuitOpenCount:          candidate.coreState.CircuitOpenCount,
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
