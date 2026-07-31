package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

const gatewayTransportFailureClientMessage = "Upstream request failed"

func newGatewayTransportFailoverError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamURL string,
	passthrough bool,
	err error,
) error {
	safeErr := "upstream transport error"
	if err != nil {
		safeErr = sanitizeUpstreamErrorMessage(err.Error())
	}
	setOpsUpstreamError(c, 0, safeErr, "")
	event := OpsUpstreamErrorEvent{
		UpstreamStatusCode: 0,
		UpstreamURL:        safeUpstreamURL(upstreamURL),
		Passthrough:        passthrough,
		Kind:               "request_error",
		Message:            safeErr,
	}
	if account != nil {
		event.Platform = account.Platform
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	appendOpsUpstreamError(c, event)

	// A canceled client request cannot succeed on another account. Keep it out
	// of the failover loop and let the handler's disconnect path terminate it.
	if errors.Is(err, context.Canceled) || (ctx != nil && ctx.Err() != nil) {
		return fmt.Errorf("upstream request failed: %s", safeErr)
	}
	accountHealthSample := false

	return &UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		ResponseBody:      []byte(`{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`),
		Scope:             GatewayFailureScopeAccount,
		FailureKind:       UpstreamFailureKindTransport,
		NextAccountAction: NextAccountRetry,
		ClientStatusCode:  http.StatusBadGateway,
		ClientMessage:     gatewayTransportFailureClientMessage,
		HealthSample:      &accountHealthSample,
	}
}
