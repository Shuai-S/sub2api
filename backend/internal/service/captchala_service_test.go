//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type captchaLaVerifierStub struct {
	issueResult    *CaptchaLaIssueResult
	validateResult *CaptchaLaValidateResult
	err            error
	issueCalls     int
	validateCalls  int
	credentials    CaptchaLaCredentials
	action         string
	token          string
	clientIP       string
	ttl            int
	maxUses        int
}

func (s *captchaLaVerifierStub) IssueServerToken(_ context.Context, credentials CaptchaLaCredentials, action string, ttl, maxUses int, bindIP string) (*CaptchaLaIssueResult, error) {
	s.issueCalls++
	s.credentials = credentials
	s.action = action
	s.ttl = ttl
	s.maxUses = maxUses
	s.clientIP = bindIP
	return s.issueResult, s.err
}

func (s *captchaLaVerifierStub) ValidateToken(_ context.Context, credentials CaptchaLaCredentials, token, clientIP string) (*CaptchaLaValidateResult, error) {
	s.validateCalls++
	s.credentials = credentials
	s.token = token
	s.clientIP = clientIP
	return s.validateResult, s.err
}

func enabledCaptchaLaConfig() CaptchaLaConfig {
	return CaptchaLaConfig{Enabled: true, AppKey: "app-key", AppSecret: "app-secret"}
}

func TestCaptchaLaServiceIssuesBoundSingleUseServerToken(t *testing.T) {
	verifier := &captchaLaVerifierStub{
		issueResult: &CaptchaLaIssueResult{Token: "sct_test", ExpiresIn: 300},
	}
	svc := NewCaptchaLaService(nil, verifier)

	result, err := svc.IssueServerTokenWithConfig(
		context.Background(),
		enabledCaptchaLaConfig(),
		CaptchaLaActionLogin,
		"203.0.113.10",
	)

	require.NoError(t, err)
	require.Equal(t, "sct_test", result.Token)
	require.Equal(t, 1, verifier.issueCalls)
	require.Equal(t, CaptchaLaCredentials{AppKey: "app-key", AppSecret: "app-secret"}, verifier.credentials)
	require.Equal(t, CaptchaLaActionLogin, verifier.action)
	require.Equal(t, captchaLaServerTokenTTL, verifier.ttl)
	require.Equal(t, captchaLaServerTokenMaxUses, verifier.maxUses)
	require.Equal(t, "203.0.113.10", verifier.clientIP)
}

func TestCaptchaLaServiceRejectsUnknownActionBeforeIssue(t *testing.T) {
	verifier := &captchaLaVerifierStub{}
	svc := NewCaptchaLaService(nil, verifier)

	_, err := svc.IssueServerTokenWithConfig(context.Background(), enabledCaptchaLaConfig(), "admin_delete", "")

	require.ErrorIs(t, err, ErrCaptchaLaActionMismatch)
	require.Zero(t, verifier.issueCalls)
}

func TestCaptchaLaServiceValidatesPassTokenAndAction(t *testing.T) {
	verifier := &captchaLaVerifierStub{
		validateResult: &CaptchaLaValidateResult{Valid: true, Action: CaptchaLaActionForgotPassword},
	}
	svc := NewCaptchaLaService(nil, verifier)

	err := svc.VerifyTokenWithConfig(
		context.Background(),
		enabledCaptchaLaConfig(),
		"pt_test",
		CaptchaLaActionForgotPassword,
		"203.0.113.10",
	)

	require.NoError(t, err)
	require.Equal(t, 1, verifier.validateCalls)
	require.Equal(t, "pt_test", verifier.token)
	require.Equal(t, "203.0.113.10", verifier.clientIP)
}

func TestCaptchaLaServiceRejectsActionMismatch(t *testing.T) {
	verifier := &captchaLaVerifierStub{
		validateResult: &CaptchaLaValidateResult{Valid: true, Action: CaptchaLaActionRegister},
	}
	svc := NewCaptchaLaService(nil, verifier)

	err := svc.VerifyTokenWithConfig(
		context.Background(),
		enabledCaptchaLaConfig(),
		"pt_test",
		CaptchaLaActionLogin,
		"",
	)

	require.ErrorIs(t, err, ErrCaptchaLaActionMismatch)
}

func TestCaptchaLaServiceRejectsNonPassTokenWithoutNetworkCall(t *testing.T) {
	verifier := &captchaLaVerifierStub{}
	svc := NewCaptchaLaService(nil, verifier)

	err := svc.VerifyTokenWithConfig(
		context.Background(),
		enabledCaptchaLaConfig(),
		"ct_client_only",
		CaptchaLaActionLogin,
		"",
	)

	require.ErrorIs(t, err, ErrCaptchaLaVerificationFailed)
	require.Zero(t, verifier.validateCalls)
}

func TestCaptchaLaServiceFailsClosedOnVerifierError(t *testing.T) {
	verifier := &captchaLaVerifierStub{err: errors.New("network unavailable")}
	svc := NewCaptchaLaService(nil, verifier)

	err := svc.VerifyTokenWithConfig(
		context.Background(),
		enabledCaptchaLaConfig(),
		"pt_test",
		CaptchaLaActionLogin,
		"",
	)

	require.ErrorIs(t, err, ErrCaptchaLaVerificationFailed)
}
