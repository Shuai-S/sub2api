package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImagesTransportFailoverPreservesProxyAttribution(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		t.Run(accountType, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2","prompt":"test"}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{err: io.EOF}}
			account := b64BackfillAccount(false)
			account.Type = accountType
			account.Credentials["access_token"] = "test-token"
			proxyID := int64(42)
			account.ProxyID = &proxyID
			account.Proxy = &Proxy{ID: proxyID, Name: "image-proxy", Protocol: "http", Host: "proxy.example.com", Port: 8080}
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)

			result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.False(t, c.Writer.Written())
			events, ok := c.MustGet(OpsUpstreamErrorsKey).([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Equal(t, &proxyID, events[0].ProxyID)
			require.Equal(t, "image-proxy", events[0].ProxyName)
			require.NotEmpty(t, events[0].UpstreamURL)
		})
	}
}

func TestImagesB64BackfillRetainsResponseValidation(t *testing.T) {
	for _, body := range []string{"", "invalid JSON", `{"data":[]}`, `{"error":{"code":"upstream_error","message":"failed"}}`} {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			resp := b64BackfillImageResponse(http.StatusOK, "application/json", []byte(body))
			defer func() { require.NoError(t, resp.Body.Close()) }()

			_, count, _, err := svc.handleOpenAIImagesNonStreamingResponse(context.Background(), resp, c, b64BackfillAccount(true), &OpenAIImagesRequest{})

			require.Error(t, err)
			require.Zero(t, count)
			require.False(t, c.Writer.Written())
			require.Empty(t, upstream.requests)
		})
	}
}
