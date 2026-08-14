package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func (s *GatewayService) ReportAnthropicAdaptiveResult(ctx context.Context, account *Account, requestedModel string, result *ForwardResult, err error) {
	if s == nil || s.anthropicAdaptiveScheduler == nil || account == nil || account.Platform != PlatformAnthropic || s.settingService == nil {
		return
	}
	settings, settingsErr := s.settingService.GetAnthropicAdaptiveSchedulerSettings(ctx)
	if settingsErr != nil || !settings.AnthropicAdaptiveSchedulerEnabled {
		return
	}
	report := classifyAnthropicAdaptiveResult(ctx, account, requestedModel, result, err)
	report.RequestID = firstNonEmpty(contextStringValue(ctx, ctxkey.RequestID), contextStringValue(ctx, ctxkey.ClientRequestID))
	now := s.anthropicAdaptiveScheduler.now()
	coreSettings := anthropicAdaptiveCoreSettings(settings)
	beforeCore := s.anthropicAdaptiveScheduler.core.snapshot(account.ID, account.Concurrency, now, coreSettings)
	observationType, authentication := anthropicAdaptiveObservation(report)
	observation := adaptiveObservation{
		AccountID:           account.ID,
		RequestID:           report.RequestID,
		Type:                observationType,
		ReasonCode:          report.TerminalReason,
		Authentication:      authentication,
		FirstTokenMs:        report.FirstTokenMs,
		ConfiguredCapacity:  account.Concurrency,
		ObservedConcurrency: -1,
	}
	if observationType == adaptiveObservationQuotaLimit {
		observation.QuotaResetAt = account.RateLimitResetAt
	}
	_, decreased := s.anthropicAdaptiveScheduler.core.observe(observation, now, coreSettings)
	afterCore := s.anthropicAdaptiveScheduler.core.snapshot(account.ID, account.Concurrency, now, coreSettings)
	if decreased {
		s.anthropicAdaptiveScheduler.capacityDecreaseTotal.Add(1)
	}
	s.logAnthropicAdaptiveDiagnosticResult(ctx, settings, report, beforeCore, afterCore, decreased, err)
}

func anthropicAdaptiveObservation(report AnthropicAdaptiveScheduleReport) (adaptiveObservationType, bool) {
	observationType, authentication := classifyAdaptiveTerminalReason(report.Success, report.TerminalReason)
	if report.Synthetic {
		return adaptiveObservationIgnored, false
	}
	if report.HealthScope == "model" && observationType == adaptiveObservationAccountFailure {
		return adaptiveObservationProviderOverload, false
	}
	if !report.HealthSample && (observationType == adaptiveObservationHealthSuccess || observationType == adaptiveObservationAccountFailure) {
		return adaptiveObservationIgnored, false
	}
	return observationType, authentication
}

func classifyAnthropicAdaptiveResult(ctx context.Context, account *Account, requestedModel string, result *ForwardResult, err error) AnthropicAdaptiveScheduleReport {
	report := AnthropicAdaptiveScheduleReport{
		Account:        account,
		RequestedModel: requestedModel,
		HealthScope:    "account",
	}
	if result != nil {
		report.MappedModel = result.UpstreamModel
		report.UpstreamRequestID = result.RequestID
		report.Stream = result.Stream
		report.Synthetic = result.Synthetic
		report.FirstTokenMs = result.FirstTokenMs
		report.DurationMs = result.Duration.Milliseconds()
	}
	if err == nil {
		if ctx.Err() != nil {
			report.TerminalReason = "client_cancelled"
			return report
		}
		if result == nil {
			report.TerminalReason = "missing_result"
			return report
		}
		if result.ClientDisconnect {
			report.TerminalReason = "client_disconnect"
			return report
		}
		report.Success = true
		report.HealthSample = true
		report.TerminalReason = "success"
		return report
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		report.TerminalReason = "client_cancelled"
		return report
	}
	if isAnthropicAdaptiveLocalQueueFailure(err) {
		report.TerminalReason = "local_queue"
		return report
	}
	if strings.Contains(strings.ToLower(err.Error()), "stream usage incomplete: missing terminal event") {
		report.HealthSample = true
		report.HealthScope = "model"
		report.TerminalReason = "stream_incomplete"
		return report
	}
	if isAnthropicAdaptiveRequestPolicyFailure(nil, err) {
		report.TerminalReason = "request_policy"
		return report
	}
	if isAnthropicAdaptiveModelScopedUpstreamError(err.Error()) {
		report.HealthSample = true
		report.HealthScope = "model"
		report.TerminalReason = "model_upstream_error"
		return report
	}

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		if failoverErr.FailureKind == UpstreamFailureKindTransport {
			if failoverErr.HealthSample != nil {
				report.HealthSample = *failoverErr.HealthSample
			}
			if failoverErr.Scope == GatewayFailureScopeAccount && !report.HealthSample {
				report.HealthSample = true
			}
			report.TerminalReason = "transport_error"
			return report
		}
		// UpstreamFailoverError.Error does not include ResponseBody. Inspect the
		// structured payload before scope and health overrides so request policy
		// failures cannot be attributed to the selected account.
		if isAnthropicAdaptiveRequestPolicyFailure(failoverErr, err) {
			report.TerminalReason = "request_policy"
			return report
		}
		if failoverErr.FailureKind == UpstreamFailureKindCapabilityMismatch || failoverErr.Scope == GatewayFailureScopeRequest || failoverErr.Scope == GatewayFailureScopeProvider {
			report.TerminalReason = "non_account_failure"
			return report
		}
		if failoverErr.HealthSample != nil {
			report.HealthSample = *failoverErr.HealthSample
		}
		hasHealthSampleOverride := failoverErr.HealthSample != nil
		if failoverErr.IsCredentialFailure() && failoverErr.Scope != GatewayFailureScopeAccount {
			report.TerminalReason = "non_account_credential_failure"
			return report
		}
		statusCode := failoverErr.StatusCode
		switch {
		case isAnthropicAdaptiveConcurrencyFailure(failoverErr, err):
			if !hasHealthSampleOverride {
				report.HealthSample = true
			}
			report.TerminalReason = "concurrency_limit"
		case statusCode == http.StatusTooManyRequests:
			if isAnthropicAdaptiveWindowRateLimit(failoverErr.ResponseHeaders) {
				if !hasHealthSampleOverride {
					report.HealthSample = false
				}
				report.TerminalReason = "window_rate_limit"
			} else {
				if !hasHealthSampleOverride {
					report.HealthSample = true
				}
				report.TerminalReason = "generic_rate_limit"
			}
		case statusCode == 529:
			if !hasHealthSampleOverride {
				report.HealthSample = false
			}
			report.TerminalReason = "provider_overloaded"
		case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
			if !hasHealthSampleOverride {
				report.HealthSample = true
			}
			report.TerminalReason = "account_auth"
		case statusCode == http.StatusBadRequest || statusCode == http.StatusNotFound:
			if !hasHealthSampleOverride {
				report.HealthSample = true
			}
			report.HealthScope = "model"
			report.TerminalReason = "model_upstream_error"
		case statusCode >= 500:
			if !hasHealthSampleOverride && failoverErr.Scope != GatewayFailureScopeProvider {
				report.HealthSample = true
			}
			report.TerminalReason = "upstream_5xx"
		default:
			report.TerminalReason = "request_error"
		}
		return report
	}

	// Transport, TLS, proxy and read failures are account-path health samples.
	report.HealthSample = true
	report.TerminalReason = "transport_error"
	return report
}

func isAnthropicAdaptiveModelScopedUpstreamError(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "upstream error: 400 message=upstream request failed") ||
		strings.Contains(lower, "upstream error: 404 message=upstream request failed") ||
		strings.Contains(lower, "model not found")
}

func isAnthropicAdaptiveRequestPolicyFailure(failoverErr *UpstreamFailoverError, err error) bool {
	parts := make([]string, 0, 4)
	if failoverErr != nil {
		parts = append(parts, string(failoverErr.ResponseBody), failoverErr.ClientMessage, string(failoverErr.Reason))
	}
	if err != nil {
		parts = append(parts, err.Error())
	}
	message := strings.ToLower(strings.Join(parts, " "))
	for _, marker := range []string{
		"invalid_request_error",
		"each tool_use must have a single result",
		"thinking.adaptive.output_config",
		"extra inputs are not permitted",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return isOpenAIAdaptiveRequestPolicyFailure(message)
}

func isAnthropicAdaptiveLocalQueueFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"timeout waiting for account concurrency slot",
		"too many pending requests",
		"account wait queue full",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isAnthropicAdaptiveWindowRateLimit(headers http.Header) bool {
	if len(headers) == 0 {
		return false
	}
	for _, window := range []string{"5h", "7d", "7d_oi"} {
		prefix := "anthropic-ratelimit-unified-" + window + "-"
		if strings.TrimSpace(headers.Get(prefix+"reset")) != "" ||
			strings.EqualFold(strings.TrimSpace(headers.Get(prefix+"status")), "rejected") ||
			strings.TrimSpace(headers.Get(prefix+"surpassed-threshold")) != "" {
			return true
		}
	}
	return false
}

func isAnthropicAdaptiveConcurrencyFailure(failoverErr *UpstreamFailoverError, err error) bool {
	parts := make([]string, 0, 3)
	if failoverErr != nil {
		parts = append(parts, string(failoverErr.ResponseBody), string(failoverErr.Reason))
	}
	if err != nil {
		parts = append(parts, err.Error())
	}
	message := strings.ToLower(strings.Join(parts, " "))
	for _, marker := range []string{
		"concurrency limit exceeded",
		"concurrency_limit",
		"too many concurrent requests",
		"account concurrency limit",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
