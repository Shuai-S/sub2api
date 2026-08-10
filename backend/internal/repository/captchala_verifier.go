package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	captchaLaAPIBaseURL       = "https://apiv1.captcha.la"
	captchaLaIssuePath        = "/v1/server/challenge/issue"
	captchaLaValidatePath     = "/v1/validate"
	captchaLaMaxResponseBytes = 1 << 20
)

type captchaLaVerifier struct {
	httpClient *http.Client
	baseURL    string
}

func NewCaptchaLaVerifier() service.CaptchaLaVerifier {
	client, err := httpclient.GetClient(httpclient.Options{
		Timeout:            10 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &captchaLaVerifier{httpClient: client, baseURL: captchaLaAPIBaseURL}
}

type captchaLaAPIResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type captchaLaIssueData struct {
	ServerToken string `json:"server_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type captchaLaValidateData struct {
	Valid       bool   `json:"valid"`
	Action      string `json:"action"`
	UID         string `json:"uid"`
	ChallengeID string `json:"challenge_id"`
	CaptchaArgs struct {
		RiskScore int `json:"risk_score"`
	} `json:"captcha_args"`
}

func (v *captchaLaVerifier) IssueServerToken(ctx context.Context, credentials service.CaptchaLaCredentials, action string, ttl, maxUses int, bindIP string) (*service.CaptchaLaIssueResult, error) {
	values := url.Values{}
	values.Set("action", action)
	values.Set("ttl", fmt.Sprintf("%d", ttl))
	values.Set("max_uses", fmt.Sprintf("%d", maxUses))
	if strings.TrimSpace(bindIP) != "" {
		values.Set("bind_ip", strings.TrimSpace(bindIP))
	}
	return v.issue(ctx, credentials, values.Encode())
}

func (v *captchaLaVerifier) issue(ctx context.Context, credentials service.CaptchaLaCredentials, body string) (*service.CaptchaLaIssueResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+captchaLaIssuePath, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create captchala issue request: %w", err)
	}
	req.Header.Set("X-App-Key", credentials.AppKey)
	req.Header.Set("X-App-Secret", credentials.AppSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response captchaLaAPIResponse
	if err := v.doJSON(req, &response); err != nil {
		return nil, err
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("captchala issue rejected: %s", apiErrorMessage(response))
	}
	var data captchaLaIssueData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, fmt.Errorf("decode captchala issue data: %w", err)
	}
	return &service.CaptchaLaIssueResult{Token: data.ServerToken, ExpiresIn: data.ExpiresIn}, nil
}

func (v *captchaLaVerifier) ValidateToken(ctx context.Context, credentials service.CaptchaLaCredentials, token, clientIP string) (*service.CaptchaLaValidateResult, error) {
	payload := map[string]any{"pass_token": token, "keep_token": false}
	if strings.TrimSpace(clientIP) != "" {
		payload["client_ip"] = strings.TrimSpace(clientIP)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode captchala validation request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+captchaLaValidatePath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create captchala validation request: %w", err)
	}
	req.Header.Set("X-App-Key", credentials.AppKey)
	req.Header.Set("X-App-Secret", credentials.AppSecret)
	req.Header.Set("Content-Type", "application/json")
	var response captchaLaAPIResponse
	if err := v.doJSON(req, &response); err != nil {
		return nil, err
	}
	if response.Code != 0 {
		return nil, fmt.Errorf("captchala validation rejected: %s", apiErrorMessage(response))
	}
	var data captchaLaValidateData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, fmt.Errorf("decode captchala validation data: %w", err)
	}
	return &service.CaptchaLaValidateResult{
		Valid:       data.Valid,
		Action:      data.Action,
		UID:         data.UID,
		ChallengeID: data.ChallengeID,
		RiskScore:   data.CaptchaArgs.RiskScore,
	}, nil
}

func (v *captchaLaVerifier) doJSON(req *http.Request, target *captchaLaAPIResponse) error {
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send captchala request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(io.LimitReader(resp.Body, captchaLaMaxResponseBytes)).Decode(target); err != nil {
		return fmt.Errorf("decode captchala response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("captchala http status %d: %s", resp.StatusCode, apiErrorMessage(*target))
	}
	return nil
}

func apiErrorMessage(response captchaLaAPIResponse) string {
	if strings.TrimSpace(response.Msg) != "" {
		return response.Msg
	}
	return "unknown error"
}
