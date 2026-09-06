package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIChatCompletionsFailoverUpstream struct {
	service.HTTPUpstream
	mu               sync.Mutex
	accountIDs       []int64
	healthyAccountID int64
	statusCode       int
}

func (u *openAIChatCompletionsFailoverUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.mu.Unlock()

	if accountID == u.healthyAccountID {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl_healthy","object":"chat.completion","created":1,"model":"gpt-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			)),
		}, nil
	}

	return &http.Response{
		StatusCode: u.statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary upstream failure"}}`)),
	}, nil
}

func (u *openAIChatCompletionsFailoverUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func TestOpenAIChatCompletionsPoolRetryExhaustionSwitchesAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name       string
		retryCount float64
		wantCalls  []int64
	}{
		{name: "no same account retries", retryCount: 0, wantCalls: []int64{9920, 9921}},
		{name: "configured same account retries", retryCount: 2, wantCalls: []int64{9920, 9920, 9920, 9921}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(4206)
			accounts := []service.Account{
				{
					ID: 9920, Name: "pool-api-key", Platform: service.PlatformOpenAI,
					Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 1,
					Credentials: map[string]any{
						"api_key":                      "sk-pool",
						"base_url":                     "https://api.example.test",
						"pool_mode":                    true,
						"pool_mode_retry_count":        tc.retryCount,
						"pool_mode_retry_status_codes": []any{float64(http.StatusForbidden)},
					},
					Extra: map[string]any{"openai_responses_supported": false},
				},
				{
					ID: 9921, Name: "healthy-api-key", Platform: service.PlatformOpenAI,
					Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 2,
					Credentials: map[string]any{
						"api_key":  "sk-healthy",
						"base_url": "https://api.example.test",
					},
					Extra: map[string]any{"openai_responses_supported": false},
				},
			}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.MaxAccountSwitches = 1

			accountRepo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
			upstream := &openAIChatCompletionsFailoverUpstream{
				healthyAccountID: 9921,
				statusCode:       http.StatusForbidden,
			}
			billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCacheSvc.Stop)
			gatewaySvc := service.NewOpenAIGatewayService(
				accountRepo,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				cfg,
				nil,
				nil,
				service.NewBillingService(cfg, nil),
				nil,
				billingCacheSvc,
				upstream,
				&service.DeferredService{},
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			)
			h := NewOpenAIGatewayHandler(
				gatewaySvc,
				service.NewConcurrencyService(nil),
				billingCacheSvc,
				service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
				nil,
				nil,
				nil,
				nil,
				cfg,
			)

			reqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":false}`)).WithContext(reqCtx)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
				ID: 1806, GroupID: &groupID,
				User:  &service.User{ID: 1706, Status: service.StatusActive},
				Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
			})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1706, Concurrency: 0})

			h.ChatCompletions(c)

			require.Equal(t, tc.wantCalls, upstream.calls())
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "chatcmpl_healthy", gjson.GetBytes(rec.Body.Bytes(), "id").String())
		})
	}
}
