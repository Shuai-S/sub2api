package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

func (s *OpenAIGatewayService) ReportOpenAIAccountAdaptiveFailureWithContext(ctx context.Context, accountID int64, err error, firstTokenMs *int) {
	if !shouldLearnOpenAIAdaptiveFailure(err) {
		return
	}
	s.ReportOpenAIAccountScheduleResultWithContext(ctx, accountID, false, firstTokenMs)
}

func shouldLearnOpenAIAdaptiveFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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
	return false
}

func (s *OpenAIGatewayService) ShouldIgnoreOpenAIAdaptiveFailoverError(err *UpstreamFailoverError) bool {
	return shouldIgnoreOpenAIAdaptiveFailoverError(err)
}
