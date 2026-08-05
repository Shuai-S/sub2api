package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func (s *GatewayService) ReportGeminiAdaptiveResult(ctx context.Context, account *Account, requestedModel, action string, result *ForwardResult, err error) {
	if s == nil || s.geminiAdaptiveScheduler == nil || account == nil || account.Platform != PlatformGemini || s.settingService == nil {
		return
	}
	settings, settingsErr := s.settingService.GetGeminiAdaptiveSchedulerSettings(ctx)
	if settingsErr != nil {
		fields := []any{
			"request_id", contextStringValue(ctx, ctxkey.RequestID),
			"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
			"account_id", account.ID,
			"model", requestedModel,
			"action", action,
		}
		fields = append(fields, geminiAdaptiveErrorLogFields(settingsErr)...)
		slog.Warn("gemini_adaptive_result_settings_read_failed", fields...)
		return
	}
	if !settings.GeminiAdaptiveSchedulerEnabled {
		return
	}
	report := classifyGeminiAdaptiveResult(ctx, account, requestedModel, action, result, err)
	before := s.geminiAdaptiveScheduler.state.snapshot(account, settings)
	increased, decreased := s.geminiAdaptiveScheduler.state.report(report, s.geminiAdaptiveScheduler.now(), settings)
	if decreased {
		s.geminiAdaptiveScheduler.capacityDecreaseTotal.Add(1)
	}
	after := s.geminiAdaptiveScheduler.state.snapshot(account, settings)
	s.logGeminiAdaptiveDiagnosticResult(ctx, settings, report, before, after, increased, decreased, err)
}

func classifyGeminiAdaptiveResult(ctx context.Context, account *Account, requestedModel, action string, result *ForwardResult, err error) GeminiAdaptiveScheduleReport {
	hint := geminiAdaptiveHintFromContext(ctx)
	if strings.TrimSpace(action) == "" {
		action = hint.Action
	}
	report := GeminiAdaptiveScheduleReport{
		Account:        account,
		RequestID:      firstNonEmpty(contextStringValue(ctx, ctxkey.RequestID), contextStringValue(ctx, ctxkey.ClientRequestID)),
		RequestedModel: requestedModel,
		MappedModel:    geminiAdaptiveCanonicalModel(account, requestedModel, "", action),
		Action:         action,
		Stream:         hint.Stream,
		ctx:            ctx,
	}
	if result != nil {
		report.UpstreamRequestID = result.RequestID
		report.MappedModel = firstNonEmpty(strings.TrimSpace(result.UpstreamModel), report.MappedModel)
		report.Stream = result.Stream
		report.FirstTokenMs = result.FirstTokenMs
		report.DurationMs = result.Duration.Milliseconds()
		report.Synthetic = result.Synthetic
	}
	if report.Synthetic {
		report.TerminalReason = "synthetic"
		return report
	}
	if err == nil {
		if result == nil || result.ClientDisconnect || ctx.Err() != nil {
			report.TerminalReason = "client_cancelled"
			return report
		}
		report.Success = true
		report.PathSample = true
		report.ModelSample = true
		report.CapacitySample = account != nil && account.Concurrency > 0
		report.AccountCircuitSample = true
		report.ModelCircuitSample = true
		report.TerminalReason = "success"
		return report
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		report.TerminalReason = "client_cancelled"
		return report
	}
	if isGeminiAdaptiveLocalQueueFailure(err) {
		report.TerminalReason = "local_queue"
		return report
	}

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		if failoverErr.FailureKind == UpstreamFailureKindCapabilityMismatch || failoverErr.Scope == GatewayFailureScopeRequest {
			report.TerminalReason = "request_error"
			return report
		}
		if failoverErr.Scope == GatewayFailureScopeProvider || isGeminiAdaptiveProviderCapacityFailure(failoverErr, err) {
			report.TerminalReason = "provider_capacity"
			return report
		}
		if isGeminiAdaptiveSignatureFailure(failoverErr.ResponseBody) {
			report.TerminalReason = "signature_error"
			return report
		}
		if failoverErr.HealthSample != nil {
			report.PathSample = *failoverErr.HealthSample
		}
		hasPathOverride := failoverErr.HealthSample != nil
		if isGeminiAdaptiveConcurrencyFailure(failoverErr, err) {
			if !hasPathOverride {
				report.PathSample = true
			}
			report.ModelSample = true
			report.CapacitySample = account != nil && account.Concurrency > 0
			report.TerminalReason = "concurrency_limit"
			return report
		}
		switch failoverErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			if !hasPathOverride {
				report.PathSample = true
			}
			report.TerminalReason = "account_auth"
			report.AccountCircuitSample = true
		case http.StatusTooManyRequests:
			if isGeminiAdaptiveExplicitQuotaFailure(failoverErr.ResponseBody) {
				report.TerminalReason = "quota_rate_limit"
			} else {
				report.ModelSample = true
				report.ModelCircuitSample = true
				report.TerminalReason = "generic_rate_limit"
			}
		default:
			if failoverErr.StatusCode >= 500 {
				report.ModelSample = true
				report.ModelCircuitSample = true
				if !hasPathOverride && failoverErr.Scope == GatewayFailureScopeAccount {
					report.PathSample = true
				}
				if report.PathSample && failoverErr.Scope == GatewayFailureScopeAccount {
					report.AccountCircuitSample = true
				}
				report.TerminalReason = "upstream_5xx"
			} else {
				report.TerminalReason = "request_error"
			}
		}
		return report
	}

	message := strings.ToLower(err.Error())
	if isGeminiAdaptiveSignatureFailure([]byte(message)) || isGeminiAdaptiveRequestFailure(message) {
		report.TerminalReason = "request_error"
		return report
	}
	if strings.Contains(message, "gemini upstream error: 5") || strings.Contains(message, "upstream status 5") {
		report.ModelSample = true
		report.ModelCircuitSample = true
		report.TerminalReason = "upstream_5xx"
		return report
	}
	report.PathSample = true
	report.AccountCircuitSample = true
	report.TerminalReason = "transport_error"
	return report
}

func isGeminiAdaptiveLocalQueueFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"timeout waiting for account concurrency slot", "too many pending requests", "account wait queue full"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isGeminiAdaptiveConcurrencyFailure(failoverErr *UpstreamFailoverError, err error) bool {
	parts := make([]string, 0, 4)
	if failoverErr != nil {
		parts = append(parts, string(failoverErr.ResponseBody), string(failoverErr.Reason))
	}
	if err != nil {
		parts = append(parts, err.Error())
	}
	message := strings.ToLower(strings.Join(parts, " "))
	for _, marker := range []string{"concurrency limit exceeded", "concurrency_limit", "too many concurrent requests", "account concurrency limit"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isGeminiAdaptiveProviderCapacityFailure(failoverErr *UpstreamFailoverError, err error) bool {
	message := ""
	if failoverErr != nil {
		message = string(failoverErr.ResponseBody) + " " + string(failoverErr.Reason)
	}
	if err != nil {
		message += " " + err.Error()
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "model_capacity_exhausted") || strings.Contains(message, "model capacity exhausted")
}

func isGeminiAdaptiveExplicitQuotaFailure(body []byte) bool {
	message := strings.ToLower(string(body))
	for _, marker := range []string{"quotafailure", "quota exceeded", "quota_limit", "requests per minute", "requests per day", "rate limit", "rpd", "rpm", "tpm"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isGeminiAdaptiveSignatureFailure(body []byte) bool {
	message := strings.ToLower(string(body))
	return strings.Contains(message, "thoughtsignature") || strings.Contains(message, "thought_signature") || strings.Contains(message, "invalid signature")
}

func isGeminiAdaptiveRequestFailure(message string) bool {
	for _, marker := range []string{"missing model", "missing action", "request body is empty", "unsupported action", "invalid_argument", "context length", "context window", "model not found", "unsupported model"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
