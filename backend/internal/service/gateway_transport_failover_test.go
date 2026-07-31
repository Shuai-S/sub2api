package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayAnthropicTransportErrorsFailOverWithoutWritingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*GatewayService, *gin.Context, *Account) (*ForwardResult, error)
	}{
		{
			name: "messages",
			run: func(svc *GatewayService, c *gin.Context, account *Account) (*ForwardResult, error) {
				body := []byte(`{"model":"claude-sonnet-4-6","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
				parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
				require.NoError(t, err)
				return svc.Forward(context.Background(), c, account, parsed)
			},
		},
		{
			name: "responses",
			run: func(svc *GatewayService, c *gin.Context, account *Account) (*ForwardResult, error) {
				body := []byte(`{"model":"claude-sonnet-4-6","input":"hello","stream":false}`)
				return svc.ForwardAsResponses(context.Background(), c, account, body, nil)
			},
		},
		{
			name: "chat_completions",
			run: func(svc *GatewayService, c *gin.Context, account *Account) (*ForwardResult, error) {
				body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}],"stream":false}`)
				return svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)

			upstream := &anthropicHTTPUpstreamRecorder{
				err: errors.New(`Post "https://account-secret.example/v1/messages": dial tcp: lookup account-secret.example on 1.1.1.1:53: no such host`),
			}
			svc := &GatewayService{
				cfg: &config.Config{
					Security: config.SecurityConfig{
						URLAllowlist: config.URLAllowlistConfig{Enabled: false},
					},
				},
				httpUpstream: upstream,
			}
			account := newAnthropicAPIKeyAccountForTest()
			account.Extra = nil
			account.Credentials["base_url"] = "https://account-secret.example"

			result, err := tt.run(svc, c, account)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
			require.Equal(t, UpstreamFailureKindTransport, failoverErr.FailureKind)
			require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
			require.True(t, failoverErr.ShouldRetryNextAccount())
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.NotNil(t, failoverErr.HealthSample)
			require.False(t, *failoverErr.HealthSample)

			require.Equal(t, http.StatusOK, rec.Code, "service must not commit a response before handler failover")
			require.Empty(t, rec.Body.String())
			for _, sensitive := range []string{"account-secret.example", "1.1.1.1", "no such host"} {
				require.NotContains(t, err.Error(), sensitive)
				require.NotContains(t, string(failoverErr.ResponseBody), sensitive)
			}
			require.Contains(t, string(failoverErr.ResponseBody), gatewayTransportFailureClientMessage)
		})
	}
}

func TestNewGatewayTransportFailoverErrorClientCancellationDoesNotSwitchAccount(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	err := newGatewayTransportFailoverError(
		context.Background(),
		c,
		newAnthropicAPIKeyAccountForTest(),
		"https://account-secret.example/v1/messages",
		false,
		context.Canceled,
	)

	var failoverErr *UpstreamFailoverError
	require.Error(t, err)
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Body.String())
}
