package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrCaptchaLaVerificationFailed = infraerrors.BadRequest("CAPTCHALA_VERIFICATION_FAILED", "captchala verification failed")
	ErrCaptchaLaNotConfigured      = infraerrors.ServiceUnavailable("CAPTCHALA_NOT_CONFIGURED", "captchala not configured")
	ErrCaptchaLaActionMismatch     = infraerrors.BadRequest("CAPTCHALA_ACTION_MISMATCH", "captchala action mismatch")
)

const (
	CaptchaLaActionLogin              = "login"
	CaptchaLaActionRegister           = "register"
	CaptchaLaActionSendVerifyCode     = "send_verify_code"
	CaptchaLaActionForgotPassword     = "forgot_password"
	CaptchaLaActionOAuthLogin         = "oauth_login"
	CaptchaLaActionPasskeyLogin       = "passkey_login"
	CaptchaLaActionOAuthCreateAccount = "oauth_create_account"

	captchaLaServerTokenTTL     = 300
	captchaLaServerTokenMaxUses = 1
)

// CaptchaLaCredentials are kept server-side and never exposed through public settings.
type CaptchaLaCredentials struct {
	AppKey    string
	AppSecret string
}

type CaptchaLaIssueResult struct {
	Token     string
	ExpiresIn int
}

type CaptchaLaValidateResult struct {
	Valid       bool
	Action      string
	UID         string
	ChallengeID string
	RiskScore   int
}

// CaptchaLaVerifier is the transport boundary for CaptchaLa's issue/validate APIs.
type CaptchaLaVerifier interface {
	IssueServerToken(context.Context, CaptchaLaCredentials, string, int, int, string) (*CaptchaLaIssueResult, error)
	ValidateToken(context.Context, CaptchaLaCredentials, string, string) (*CaptchaLaValidateResult, error)
}

type CaptchaLaService struct {
	settingService *SettingService
	verifier       CaptchaLaVerifier
}

func NewCaptchaLaService(settingService *SettingService, verifier CaptchaLaVerifier) *CaptchaLaService {
	return &CaptchaLaService{settingService: settingService, verifier: verifier}
}

func (s *CaptchaLaService) IssueServerToken(ctx context.Context, action, remoteIP string) (*CaptchaLaIssueResult, error) {
	if s == nil || s.settingService == nil {
		return nil, ErrCaptchaLaNotConfigured
	}
	config, err := s.settingService.GetCaptchaProviderConfig(ctx)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	if !config.CaptchaLa.Enabled {
		return nil, ErrCaptchaLaNotConfigured
	}
	return s.IssueServerTokenWithConfig(ctx, config.CaptchaLa, action, remoteIP)
}

func (s *CaptchaLaService) IssueServerTokenWithConfig(ctx context.Context, config CaptchaLaConfig, action, remoteIP string) (*CaptchaLaIssueResult, error) {
	credentials, ok := captchaLaCredentials(config)
	if !ok || s == nil || s.verifier == nil {
		return nil, ErrCaptchaLaNotConfigured
	}
	if !isCaptchaLaAction(action) {
		return nil, fmt.Errorf("%w: %s", ErrCaptchaLaActionMismatch, action)
	}
	result, err := s.verifier.IssueServerToken(ctx, credentials, action, captchaLaServerTokenTTL, captchaLaServerTokenMaxUses, strings.TrimSpace(remoteIP))
	if err != nil || result == nil || strings.TrimSpace(result.Token) == "" {
		if err != nil {
			logger.LegacyPrintf("service.captchala", "issue server token failed: %v", err)
		}
		return nil, ErrCaptchaLaVerificationFailed
	}
	return result, nil
}

func (s *CaptchaLaService) VerifyToken(ctx context.Context, token, expectedAction, remoteIP string) error {
	if s == nil || s.settingService == nil {
		return ErrCaptchaLaNotConfigured
	}
	config, err := s.settingService.GetCaptchaProviderConfig(ctx)
	if err != nil {
		return ErrServiceUnavailable
	}
	if !config.CaptchaLa.Enabled {
		return nil
	}
	return s.VerifyTokenWithConfig(ctx, config.CaptchaLa, token, expectedAction, remoteIP)
}

func (s *CaptchaLaService) VerifyTokenWithConfig(ctx context.Context, config CaptchaLaConfig, token, expectedAction, remoteIP string) error {
	credentials, ok := captchaLaCredentials(config)
	if !ok || s == nil || s.verifier == nil {
		return ErrCaptchaLaNotConfigured
	}
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "pt_") || !isCaptchaLaAction(expectedAction) {
		return ErrCaptchaLaVerificationFailed
	}
	result, err := s.verifier.ValidateToken(ctx, credentials, token, strings.TrimSpace(remoteIP))
	if err != nil || result == nil || !result.Valid {
		if err != nil {
			logger.LegacyPrintf("service.captchala", "validate token failed: %v", err)
		}
		return ErrCaptchaLaVerificationFailed
	}
	if strings.TrimSpace(result.Action) != expectedAction {
		logger.LegacyPrintf("service.captchala", "action mismatch expected=%s actual=%s", expectedAction, result.Action)
		return ErrCaptchaLaActionMismatch
	}
	return nil
}

func captchaLaCredentials(config CaptchaLaConfig) (CaptchaLaCredentials, bool) {
	credentials := CaptchaLaCredentials{
		AppKey:    strings.TrimSpace(config.AppKey),
		AppSecret: strings.TrimSpace(config.AppSecret),
	}
	return credentials, credentials.AppKey != "" && credentials.AppSecret != ""
}

func isCaptchaLaAction(action string) bool {
	switch strings.TrimSpace(action) {
	case CaptchaLaActionLogin, CaptchaLaActionRegister, CaptchaLaActionSendVerifyCode,
		CaptchaLaActionForgotPassword, CaptchaLaActionOAuthLogin, CaptchaLaActionPasskeyLogin,
		CaptchaLaActionOAuthCreateAccount:
		return true
	default:
		return false
	}
}
