package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newClaudeCodeUpstreamMimicryAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":                               "sk-upstream",
			credKeyClaudeCodeUpstreamMimicryEnabled: true,
		},
	}
}

func newClaudeCodeUpstreamMimicryContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("User-Agent", "third-party-client/1.0")
	req.Header.Set("anthropic-beta", "client-beta-should-not-pass")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func newClaudeCodeUpstreamMimicryService() *GatewayService {
	return &GatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}},
	}}
}

func TestAccountClaudeCodeUpstreamMimicry(t *testing.T) {
	t.Run("eligible anthropic api key account", func(t *testing.T) {
		require.True(t, newClaudeCodeUpstreamMimicryAccount().IsClaudeCodeUpstreamMimicryEnabled())
	})

	t.Run("disabled or ineligible accounts", func(t *testing.T) {
		account := newClaudeCodeUpstreamMimicryAccount()
		account.Credentials[credKeyClaudeCodeUpstreamMimicryEnabled] = false
		require.False(t, account.IsClaudeCodeUpstreamMimicryEnabled())

		account.Credentials[credKeyClaudeCodeUpstreamMimicryEnabled] = true
		account.Type = AccountTypeOAuth
		require.False(t, account.IsClaudeCodeUpstreamMimicryEnabled())
		account.Type = AccountTypeAPIKey
		account.Platform = PlatformOpenAI
		require.False(t, account.IsClaudeCodeUpstreamMimicryEnabled())
	})

	t.Run("credential value must be boolean", func(t *testing.T) {
		require.NoError(t, NormalizeClaudeCodeUpstreamMimicryCredentials(nil))
		require.NoError(t, NormalizeClaudeCodeUpstreamMimicryCredentials(map[string]any{
			credKeyClaudeCodeUpstreamMimicryEnabled: true,
		}))
		err := NormalizeClaudeCodeUpstreamMimicryCredentials(map[string]any{
			credKeyClaudeCodeUpstreamMimicryEnabled: "true",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be a boolean")
	})
}

func TestClaudeCodeUpstreamMimicryBodyAndHeadersPassValidator(t *testing.T) {
	svc := newClaudeCodeUpstreamMimicryService()
	account := newClaudeCodeUpstreamMimicryAccount()
	c := newClaudeCodeUpstreamMimicryContext()
	body := []byte(`{"model":"claude-sonnet-4-6","system":"Keep this project instruction","messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	sessionContext := &SessionContext{ClientIP: "203.0.113.9", UserAgent: "third-party-client/1.0", APIKeyID: 77}

	mimicked := svc.applyClaudeCodeUpstreamMimicryToBody(
		context.Background(), c, account, body, json.RawMessage(`"Keep this project instruction"`),
		"claude-sonnet-4-6", sessionContext,
	)

	metadataUserID := gjson.GetBytes(mimicked, "metadata.user_id").String()
	require.NotNil(t, ParseMetadataUserID(metadataUserID))
	require.Contains(t, gjson.GetBytes(mimicked, "messages.0.content.0.text").String(), "Keep this project instruction")
	require.True(t, systemHasBillingAttributionBlock(mimicked))

	req, wireBody, err := svc.buildUpstreamRequest(
		context.Background(), c, account, mimicked, "sk-upstream", "apikey",
		"claude-sonnet-4-6", false, true,
	)
	require.NoError(t, err)
	require.Equal(t, "sk-upstream", req.Header.Get("x-api-key"))
	require.Regexp(t, `^claude-cli/\d+\.\d+\.\d+`, req.Header.Get("User-Agent"))
	require.Equal(t, "cli", getHeaderRaw(req.Header, "x-app"))
	require.Equal(t, "2023-06-01", getHeaderRaw(req.Header, "anthropic-version"))
	require.Equal(t, claude.APIKeyBetaHeader, getHeaderRaw(req.Header, "anthropic-beta"))
	require.NotContains(t, getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaOAuth)
	require.NotContains(t, getHeaderRaw(req.Header, "anthropic-beta"), "client-beta-should-not-pass")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(wireBody, &payload))
	validationReq := req.Clone(req.Context())
	validationReq.Header = make(http.Header, len(req.Header))
	for key, values := range req.Header {
		for _, value := range values {
			validationReq.Header.Add(http.CanonicalHeaderKey(key), value)
		}
	}
	require.True(t, NewClaudeCodeValidator().Validate(validationReq, payload))
}

func TestClaudeCodeUpstreamMimicryMetadata(t *testing.T) {
	svc := newClaudeCodeUpstreamMimicryService()
	account := newClaudeCodeUpstreamMimicryAccount()
	c := newClaudeCodeUpstreamMimicryContext()
	sessionContext := &SessionContext{ClientIP: "203.0.113.10", UserAgent: "client/1.0", APIKeyID: 88}

	t.Run("invalid metadata is replaced with a stable value", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4-6","metadata":{"user_id":"invalid","trace":"keep"},"messages":[{"role":"user","content":"same conversation"}],"max_tokens":128}`)
		first := svc.applyClaudeCodeUpstreamMimicryToBody(context.Background(), c, account, body, nil, "claude-sonnet-4-6", sessionContext)
		second := svc.applyClaudeCodeUpstreamMimicryToBody(context.Background(), c, account, body, nil, "claude-sonnet-4-6", sessionContext)

		firstUserID := gjson.GetBytes(first, "metadata.user_id").String()
		require.Equal(t, firstUserID, gjson.GetBytes(second, "metadata.user_id").String())
		require.NotNil(t, ParseMetadataUserID(firstUserID))
		require.Equal(t, "keep", gjson.GetBytes(first, "metadata.trace").String())
	})

	t.Run("valid metadata is retained", func(t *testing.T) {
		valid := FormatMetadataUserID(strings.Repeat("a", 64), "", "11111111-1111-4111-8111-111111111111", claude.CLICurrentVersion)
		body, err := json.Marshal(map[string]any{
			"model":      "claude-sonnet-4-6",
			"metadata":   map[string]any{"user_id": valid},
			"messages":   []any{map[string]any{"role": "user", "content": "hello"}},
			"max_tokens": 128,
		})
		require.NoError(t, err)
		mimicked := svc.applyClaudeCodeUpstreamMimicryToBody(context.Background(), c, account, body, nil, "claude-sonnet-4-6", sessionContext)
		require.Equal(t, valid, gjson.GetBytes(mimicked, "metadata.user_id").String())
	})
}

func TestClaudeCodeUpstreamMimicryPreservesAPIKeyModel(t *testing.T) {
	svc := newClaudeCodeUpstreamMimicryService()
	account := newClaudeCodeUpstreamMimicryAccount()
	c := newClaudeCodeUpstreamMimicryContext()
	const model = "claude-sonnet-4-5"
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	sessionContext := &SessionContext{ClientIP: "203.0.113.12", UserAgent: "client/1.0", APIKeyID: 101}

	mimicked := svc.applyClaudeCodeUpstreamMimicryToBody(
		context.Background(), c, account, body, nil, model, sessionContext,
	)
	require.Equal(t, model, gjson.GetBytes(mimicked, "model").String())

	countTokensBody := svc.applyClaudeCodeUpstreamMimicryToCountTokensBody(
		context.Background(), c, account, body, model, sessionContext,
	)
	require.Equal(t, model, gjson.GetBytes(countTokensBody, "model").String())
}

func TestClaudeCodeUpstreamMimicryLeavesExistingClaudeCodeBodyUntouched(t *testing.T) {
	svc := newClaudeCodeUpstreamMimicryService()
	account := newClaudeCodeUpstreamMimicryAccount()
	c := newClaudeCodeUpstreamMimicryContext()
	body := []byte(`{"model":"claude-sonnet-4-6","system":"project instruction","messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	mimicked := svc.applyClaudeCodeUpstreamMimicryToBody(
		context.Background(), c, account, body, json.RawMessage(`"project instruction"`),
		"claude-sonnet-4-6", &SessionContext{ClientIP: "203.0.113.11", APIKeyID: 99},
	)
	metadataUserID := gjson.GetBytes(mimicked, "metadata.user_id").String()
	require.True(t, isClaudeCodeRequestForMimicry(context.Background(), c, mimicked, metadataUserID))

	forwardBody := mimicked
	if !isClaudeCodeRequestForMimicry(context.Background(), c, mimicked, metadataUserID) {
		forwardBody = svc.applyClaudeCodeUpstreamMimicryToBody(context.Background(), c, account, mimicked, nil, "claude-sonnet-4-6", nil)
	}
	require.Equal(t, mimicked, forwardBody)
}

func TestClaudeCodeUpstreamMimicryPassthroughBuilders(t *testing.T) {
	svc := newClaudeCodeUpstreamMimicryService()
	account := newClaudeCodeUpstreamMimicryAccount()
	c := newClaudeCodeUpstreamMimicryContext()
	body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)

	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthroughWithMimicry(
		context.Background(), c, account, body, "sk-upstream", true, false,
	)
	require.NoError(t, err)
	require.Regexp(t, `^claude-cli/`, req.Header.Get("User-Agent"))
	require.Equal(t, "cli", getHeaderRaw(req.Header, "x-app"))
	require.Equal(t, claude.APIKeyBetaHeader, getHeaderRaw(req.Header, "anthropic-beta"))

	countReq, err := svc.buildCountTokensRequestAnthropicAPIKeyPassthroughWithMimicry(
		context.Background(), c, account, body, "sk-upstream", true,
	)
	require.NoError(t, err)
	require.Regexp(t, `^claude-cli/`, countReq.Header.Get("User-Agent"))
	require.Contains(t, getHeaderRaw(countReq.Header, "anthropic-beta"), claude.BetaTokenCounting)
	require.NotContains(t, getHeaderRaw(countReq.Header, "anthropic-beta"), claude.BetaOAuth)
}
