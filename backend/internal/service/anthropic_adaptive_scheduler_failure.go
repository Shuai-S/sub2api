package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
	if !report.HealthSample && !report.CapacitySample && !report.Success {
		return
	}
	_, decreased := s.anthropicAdaptiveScheduler.state.report(report, s.anthropicAdaptiveScheduler.now())
	if decreased {
		s.anthropicAdaptiveScheduler.capacityDecreaseTotal.Add(1)
	}
}

func classifyAnthropicAdaptiveResult(ctx context.Context, account *Account, requestedModel string, result *ForwardResult, err error) AnthropicAdaptiveScheduleReport {
	report := AnthropicAdaptiveScheduleReport{
		Account:        account,
		RequestedModel: requestedModel,
	}
	if err == nil {
		if result == nil || result.ClientDisconnect || ctx.Err() != nil {
			return report
		}
		report.Success = true
		report.HealthSample = true
		report.CapacitySample = account != nil && account.Concurrency > 0
		report.FirstTokenMs = result.FirstTokenMs
		report.DurationMs = result.Duration.Milliseconds()
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

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
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
			report.CapacitySample = account != nil && account.Concurrency > 0
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
	var text strings.Builder
	if failoverErr != nil {
		text.Write(failoverErr.ResponseBody)
		text.WriteByte(' ')
		text.WriteString(string(failoverErr.Reason))
	}
	if err != nil {
		text.WriteByte(' ')
		text.WriteString(err.Error())
	}
	message := strings.ToLower(text.String())
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
