package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) newOpenAIWSResponseFailedError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	responseHeaders http.Header,
	payload []byte,
	wroteDownstream bool,
	usage OpenAIUsage,
) error {
	message := extractOpenAISSEErrorMessage(payload)
	if message == "" {
		message = "OpenAI upstream response failed"
	}
	if hit, code, cyberMessage := detectOpenAICyberPolicy(payload); hit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code:           code,
			Message:        cyberMessage,
			Body:           truncateString(string(payload), 4096),
			UpstreamStatus: http.StatusOK,
			UpstreamInTok:  usage.InputTokens,
			UpstreamOutTok: usage.OutputTokens,
		})
	}

	upstreamRequestID := strings.TrimSpace(responseHeaders.Get("x-request-id"))
	if upstreamRequestID == "" {
		upstreamRequestID = strings.TrimSpace(responseHeaders.Get("xai-request-id"))
	}
	if !wroteDownstream && openAIStreamFailedEventShouldFailover(payload, message) {
		failoverErr := s.newOpenAIStreamFailoverError(c, account, passthrough, upstreamRequestID, payload, message)
		failoverErr.ResponseHeaders = cloneHeader(responseHeaders)
		return failoverErr
	}

	s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "http_error", payload, message)
	return fmt.Errorf("upstream response failed: %s", message)
}
