package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	coderws "github.com/coder/websocket"
)

func (s *OpenAIGatewayService) ReportOpenAIAccountAdaptiveFailureWithContext(ctx context.Context, accountID int64, err error, firstTokenMs *int) {
	s.ReportOpenAIAccountAdaptiveFailureTerminalWithContext(ctx, accountID, err, firstTokenMs, 0, false)
}

func (s *OpenAIGatewayService) ReportOpenAIAccountAdaptiveFailureTerminalWithContext(ctx context.Context, accountID int64, err error, firstTokenMs *int, durationMs int64, stream bool) {
	healthSample := openAIAdaptiveFailureHealthSample(err)
	cooldownReason := openAIAdaptiveFailureCooldownReason(err)
	s.ReportOpenAIAccountScheduleReportWithContext(ctx, OpenAIAccountScheduleReport{
		AccountID:      accountID,
		Success:        false,
		FirstTokenMs:   firstTokenMs,
		DurationMs:     durationMs,
		Stream:         stream,
		HealthSample:   healthSample,
		Cooldown:       cooldownReason != "",
		CooldownReason: cooldownReason,
		TerminalReason: classifyOpenAIAdaptiveTerminalReason(err, healthSample),
		Err:            err,
	})
}

func openAIAdaptiveFailureHealthSample(err error) bool {
	healthSample := shouldLearnOpenAIAdaptiveFailure(err)
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) && shouldIgnoreOpenAIAdaptiveFailoverError(failoverErr) {
		healthSample = false
	}
	return healthSample
}

func openAIAdaptiveFailureCooldownReason(err error) string {
	if err == nil {
		return ""
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		if reason := openAIAdaptiveFailureTextCooldownReason(upstreamFailoverErrorMessageForAdaptive(failoverErr)); reason != "" {
			return reason
		}
		switch failoverErr.StatusCode {
		case http.StatusTooManyRequests:
			return "upstream_429"
		case http.StatusBadGateway:
			return "upstream_502"
		case http.StatusServiceUnavailable:
			return "upstream_503"
		}
	}
	var wsCloseErr *OpenAIWSClientCloseError
	if errors.As(err, &wsCloseErr) {
		if wsCloseErr.StatusCode() == coderws.StatusTryAgainLater {
			if isOpenAIWSUserConcurrencyCloseReason(wsCloseErr.Reason()) {
				return ""
			}
			if reason := openAIAdaptiveFailureTextCooldownReason(wsCloseErr.Reason()); reason != "" {
				return reason
			}
			return "ws_close_try_again"
		}
		var dialErr *openAIWSDialError
		if errors.As(err, &dialErr) {
			switch dialErr.StatusCode {
			case http.StatusTooManyRequests:
				return "ws_upstream_429"
			case http.StatusBadGateway:
				return "ws_upstream_502"
			case http.StatusServiceUnavailable:
				return "ws_upstream_503"
			}
		}
	}
	var closeErr coderws.CloseError
	if errors.As(err, &closeErr) && closeErr.Code != coderws.StatusNormalClosure && closeErr.Code != coderws.StatusPolicyViolation {
		return fmt.Sprintf("ws_close_%d", int(closeErr.Code))
	}
	if errors.Is(err, errOpenAIWSConnQueueFull) {
		return "ws_connection_limit"
	}
	return openAIAdaptiveFailureTextCooldownReason(err.Error())
}

func isOpenAIWSUserConcurrencyCloseReason(reason string) bool {
	lower := strings.ToLower(strings.TrimSpace(reason))
	return strings.Contains(lower, "too many concurrent requests") ||
		strings.Contains(lower, "user concurrency")
}

func upstreamFailoverErrorMessageForAdaptive(err *UpstreamFailoverError) string {
	if err == nil {
		return ""
	}
	msg := extractUpstreamErrorMessage(err.ResponseBody)
	msg = sanitizeUpstreamErrorMessage(strings.TrimSpace(msg))
	if msg == "" && len(err.ResponseBody) > 0 {
		msg = strings.TrimSpace(string(err.ResponseBody))
	}
	return msg
}

func openAIAdaptiveFailureTextCooldownReason(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" ||
		strings.Contains(lower, "client disconnected") ||
		strings.Contains(lower, "request canceled") {
		return ""
	}
	if strings.Contains(lower, "concurrency limit") ||
		strings.Contains(lower, "timeout waiting for") && strings.Contains(lower, "concurrency slot") ||
		strings.Contains(lower, "connection limit") ||
		strings.Contains(lower, "queue full") ||
		strings.Contains(lower, "too many concurrent") ||
		strings.Contains(lower, "account is busy") ||
		strings.Contains(lower, "upstream websocket is busy") {
		return "concurrency_limit"
	}
	if strings.Contains(lower, "websocket") && strings.Contains(lower, "close") {
		return "ws_close"
	}
	return ""
}

func classifyOpenAIAdaptiveTerminalReason(err error, healthSample bool) string {
	if err == nil {
		return "failure"
	}
	if !healthSample {
		return "non_account_health_error"
	}
	return "account_health_failure"
}

func shouldLearnOpenAIAdaptiveFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var wsClientCloseErr *OpenAIWSClientCloseError
	if errors.As(err, &wsClientCloseErr) {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if lower == "" {
		return true
	}

	if strings.Contains(lower, "client disconnected") ||
		strings.Contains(lower, "after disconnect") ||
		strings.Contains(lower, "request canceled") {
		return false
	}
	if isOpenAIAdaptiveRequestPolicyFailure(lower) {
		return false
	}

	if strings.Contains(lower, "context window") ||
		strings.Contains(lower, "context length") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "max context length") ||
		strings.Contains(lower, "context_too_large") ||
		strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "array too long") ||
		strings.Contains(lower, "missing required parameter") ||
		strings.Contains(lower, "required parameter") ||
		strings.Contains(lower, "invalid_request_error") ||
		strings.Contains(lower, "invalid value") ||
		strings.Contains(lower, "invalid encrypted content") ||
		strings.Contains(lower, "function_call_output requires") ||
		strings.Contains(lower, "previous_response_id") ||
		strings.Contains(lower, "model not found") ||
		strings.Contains(lower, "bad request") {
		return false
	}

	if strings.Contains(lower, "stream usage incomplete") {
		return true
	}
	if strings.Contains(lower, "upstream response failed") {
		return true
	}

	return true
}

func shouldIgnoreOpenAIAdaptiveFailoverError(err *UpstreamFailoverError) bool {
	if err == nil {
		return true
	}
	msg := extractUpstreamErrorMessage(err.ResponseBody)
	msg = sanitizeUpstreamErrorMessage(strings.TrimSpace(msg))
	if msg == "" && len(err.ResponseBody) > 0 {
		msg = strings.TrimSpace(string(err.ResponseBody))
	}
	lower := strings.ToLower(msg)
	if err.StatusCode == http.StatusBadRequest || err.StatusCode == http.StatusNotFound || err.StatusCode == http.StatusUnprocessableEntity {
		return true
	}
	if isOpenAIContextWindowError(msg, err.ResponseBody) {
		return true
	}
	if strings.Contains(lower, "invalid_request_error") ||
		strings.Contains(lower, "array too long") ||
		strings.Contains(lower, "missing required parameter") ||
		strings.Contains(lower, "required parameter") ||
		strings.Contains(lower, "previous_response_id") ||
		strings.Contains(lower, "function_call_output requires") ||
		strings.Contains(lower, "model not found") ||
		strings.Contains(lower, "unsupported") {
		return true
	}
	if isOpenAIAdaptiveRequestPolicyFailure(lower) {
		return true
	}
	return false
}

func isOpenAIAdaptiveRequestPolicyFailure(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"cyber_policy",
		"content_policy",
		"moderation_blocked",
		"safety_error",
		"safety violation",
		"safety system",
		"high-risk cyber",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "content") && strings.Contains(lower, "policy") ||
		strings.Contains(lower, "cyber") && strings.Contains(lower, "policy") ||
		strings.Contains(lower, "moderation") && (strings.Contains(lower, "blocked") || strings.Contains(lower, "rejected")) ||
		strings.Contains(lower, "policy") && (strings.Contains(lower, "blocked") || strings.Contains(lower, "rejected") || strings.Contains(lower, "violation"))
}

func (s *OpenAIGatewayService) ShouldIgnoreOpenAIAdaptiveFailoverError(err *UpstreamFailoverError) bool {
	return shouldIgnoreOpenAIAdaptiveFailoverError(err)
}
