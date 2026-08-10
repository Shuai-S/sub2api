//go:build unit

package handler

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthRequestsBindCaptchaProof(t *testing.T) {
	const payload = `{"email":"user@example.com","password":"secret-123","tencent_captcha_ticket":"ticket-value","tencent_captcha_randstr":"@rand-value","captcha_token":"pt_captchala-value"}`

	tests := []struct {
		name   string
		decode func([]byte) service.CaptchaProof
	}{
		{
			name: "登录",
			decode: func(raw []byte) service.CaptchaProof {
				var req LoginRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "注册",
			decode: func(raw []byte) service.CaptchaProof {
				var req RegisterRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "发送邮箱验证码",
			decode: func(raw []byte) service.CaptchaProof {
				var req SendVerifyCodeRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "忘记密码",
			decode: func(raw []byte) service.CaptchaProof {
				var req ForgotPasswordRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "OAuth启动",
			decode: func(raw []byte) service.CaptchaProof {
				var req oauthStartCaptchaRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "Passkey登录",
			decode: func(raw []byte) service.CaptchaProof {
				var req passkeyBeginLoginRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "OAuth待处理账号发送邮箱验证码",
			decode: func(raw []byte) service.CaptchaProof {
				var req sendPendingOAuthVerifyCodeRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
		{
			name: "OAuth待处理账号创建",
			decode: func(raw []byte) service.CaptchaProof {
				var req createPendingOAuthAccountRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				return captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr, req.CaptchaToken)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proof := test.decode([]byte(payload))
			require.Equal(t, "ticket-value", proof.TencentTicket)
			require.Equal(t, "@rand-value", proof.TencentRandstr)
			require.Equal(t, "pt_captchala-value", proof.CaptchaLaToken)
		})
	}
}
