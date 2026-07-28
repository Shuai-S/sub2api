package service

import (
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const credKeyClaudeCodeUpstreamMimicryEnabled = "claude_code_upstream_mimicry_enabled"

// IsClaudeCodeUpstreamMimicryEnabled reports whether an Anthropic API Key
// account should rewrite non-Claude-Code traffic before forwarding it upstream.
func (a *Account) IsClaudeCodeUpstreamMimicryEnabled() bool {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeAPIKey || a.Credentials == nil {
		return false
	}
	enabled, ok := a.Credentials[credKeyClaudeCodeUpstreamMimicryEnabled].(bool)
	return ok && enabled
}

// NormalizeClaudeCodeUpstreamMimicryCredentials validates the account-level
// switch when it is present. Eligibility is enforced at read time so generic
// credential merge paths can retain the setting without activating it.
func NormalizeClaudeCodeUpstreamMimicryCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	raw, ok := credentials[credKeyClaudeCodeUpstreamMimicryEnabled]
	if !ok || raw == nil {
		return nil
	}
	if _, isBool := raw.(bool); !isBool {
		return infraerrors.New(http.StatusBadRequest, "INVALID_CLAUDE_CODE_UPSTREAM_MIMICRY",
			"claude_code_upstream_mimicry_enabled must be a boolean")
	}
	return nil
}
