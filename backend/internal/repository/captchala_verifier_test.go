package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCaptchaLaVerifierIssueServerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, captchaLaIssuePath, r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "app-key", r.Header.Get("X-App-Key"))
		require.Equal(t, "app-secret", r.Header.Get("X-App-Secret"))
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		require.NoError(t, r.ParseForm())
		require.Equal(t, "login", r.Form.Get("action"))
		require.Equal(t, "300", r.Form.Get("ttl"))
		require.Equal(t, "1", r.Form.Get("max_uses"))
		require.Equal(t, "203.0.113.10", r.Form.Get("bind_ip"))

		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"server_token": "sct_test", "expires_in": 300},
		})
	}))
	defer server.Close()

	verifier := &captchaLaVerifier{httpClient: server.Client(), baseURL: server.URL}
	result, err := verifier.IssueServerToken(
		context.Background(),
		service.CaptchaLaCredentials{AppKey: "app-key", AppSecret: "app-secret"},
		"login",
		300,
		1,
		"203.0.113.10",
	)

	require.NoError(t, err)
	require.Equal(t, "sct_test", result.Token)
	require.Equal(t, 300, result.ExpiresIn)
}

func TestCaptchaLaVerifierValidateToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, captchaLaValidatePath, r.URL.Path)
		require.Equal(t, "app-key", r.Header.Get("X-App-Key"))
		require.Equal(t, "app-secret", r.Header.Get("X-App-Secret"))

		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "pt_test", payload["pass_token"])
		require.Equal(t, false, payload["keep_token"])
		require.Equal(t, "203.0.113.10", payload["client_ip"])

		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"valid":        true,
				"action":       "login",
				"challenge_id": "ch_test",
				"uid":          "user-1",
				"captcha_args": map[string]any{"risk_score": 12},
			},
		})
	}))
	defer server.Close()

	verifier := &captchaLaVerifier{httpClient: server.Client(), baseURL: server.URL}
	result, err := verifier.ValidateToken(
		context.Background(),
		service.CaptchaLaCredentials{AppKey: "app-key", AppSecret: "app-secret"},
		"pt_test",
		"203.0.113.10",
	)

	require.NoError(t, err)
	require.True(t, result.Valid)
	require.Equal(t, "login", result.Action)
	require.Equal(t, "ch_test", result.ChallengeID)
	require.Equal(t, "user-1", result.UID)
	require.Equal(t, 12, result.RiskScore)
}

func TestCaptchaLaVerifierRejectsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 401, "msg": "invalid credentials"})
	}))
	defer server.Close()

	verifier := &captchaLaVerifier{httpClient: server.Client(), baseURL: server.URL}
	_, err := verifier.ValidateToken(
		context.Background(),
		service.CaptchaLaCredentials{AppKey: "bad", AppSecret: "bad"},
		"pt_test",
		"",
	)

	require.ErrorContains(t, err, "invalid credentials")
}
