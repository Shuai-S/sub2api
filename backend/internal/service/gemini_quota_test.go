//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type trackingGeminiQuotaUsageRepo struct {
	UsageLogRepository
	modelStatsCalls int
}

func (r *trackingGeminiQuotaUsageRepo) GetModelStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.ModelStat, error) {
	r.modelStatsCalls++
	return nil, nil
}

func TestGeminiQuotaServiceQuotaForAccountPoolMode(t *testing.T) {
	t.Parallel()
	service := NewGeminiQuotaService(nil, nil)
	tests := []struct {
		name      string
		account   *Account
		wantOK    bool
		wantFlash int64
	}{
		{
			name: "native API key keeps configured tier quota",
			account: &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"tier_id": GeminiTierAIStudioFree,
			}},
			wantOK: true, wantFlash: 1500,
		},
		{
			name: "pool API key ignores stale native tier",
			account: &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"pool_mode": true, "tier_id": GeminiTierAIStudioFree,
			}},
			wantOK: false,
		},
		{
			name:    "non Gemini pool account remains outside Gemini quota",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
			wantOK:  false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quota, ok := service.QuotaForAccount(context.Background(), test.account)
			if ok != test.wantOK {
				t.Fatalf("QuotaForAccount() ok = %v, want %v", ok, test.wantOK)
			}
			if quota.FlashRPD != test.wantFlash {
				t.Fatalf("QuotaForAccount() FlashRPD = %d, want %d", quota.FlashRPD, test.wantFlash)
			}
		})
	}
}

func TestGeminiPoolModeBatchPrecheckIsNeutral(t *testing.T) {
	t.Parallel()
	repo := &trackingGeminiQuotaUsageRepo{}
	quotaService := NewGeminiQuotaService(nil, nil)
	rateLimitService := NewRateLimitService(nil, repo, &config.Config{}, quotaService, nil)
	account := &Account{ID: 42, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"pool_mode": true, "tier_id": GeminiTierAIStudioFree,
	}}
	allowed, snapshots, err := rateLimitService.PreCheckUsageBatchWithSnapshots(context.Background(), []*Account{account}, "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("PreCheckUsageBatchWithSnapshots() error = %v", err)
	}
	if !allowed[account.ID] {
		t.Fatal("pool account should remain allowed")
	}
	if _, exists := snapshots[account.ID]; exists {
		t.Fatal("pool account should not expose a simulated native quota snapshot")
	}
	if repo.modelStatsCalls != 0 {
		t.Fatalf("usage repository calls = %d, want 0", repo.modelStatsCalls)
	}
}

func TestNormalizeGeminiPoolCredentials(t *testing.T) {
	t.Parallel()
	original := map[string]any{"api_key": "secret", "pool_mode": true, "tier_id": GeminiTierAIStudioPaid}
	normalized := normalizeGeminiPoolCredentials(PlatformGemini, AccountTypeAPIKey, original)
	if _, exists := normalized["tier_id"]; exists {
		t.Fatal("Gemini pool credentials should not retain tier_id")
	}
	if original["tier_id"] != GeminiTierAIStudioPaid {
		t.Fatal("normalization should not mutate the request credentials map")
	}
	poolAccount := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: original}
	if tierID := poolAccount.GeminiTierID(); tierID != "" {
		t.Fatalf("GeminiTierID() = %q for pool account, want empty", tierID)
	}
	native := normalizeGeminiPoolCredentials(PlatformGemini, AccountTypeAPIKey, map[string]any{"tier_id": GeminiTierAIStudioFree})
	if native["tier_id"] != GeminiTierAIStudioFree {
		t.Fatal("native Gemini API key should retain tier_id")
	}
	nonGemini := normalizeGeminiPoolCredentials(PlatformOpenAI, AccountTypeAPIKey, original)
	if nonGemini["tier_id"] != GeminiTierAIStudioPaid {
		t.Fatal("non-Gemini pool credentials should not be changed")
	}
}

func TestAccountIsGeminiThirdPartyAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected bool
	}{
		{"nil_account", nil, false},
		{"empty_base_url_is_official", geminiQuotaAPIKeyAccount(""), false},
		{"official_base_url", geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com"), false},
		{"official_base_url_trailing_slash", geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com/"), false},
		{"official_host_over_http", geminiQuotaAPIKeyAccount("http://generativelanguage.googleapis.com"), true},
		{"official_host_with_path_prefix", geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com/proxy"), true},
		{"relay_base_url", geminiQuotaAPIKeyAccount("https://relay.example.com"), true},
		{"oauth_account", &Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://relay.example.com"}}, false},
		{"vertex_service_account", &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount}, false},
		{"non_gemini_platform", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example.com"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.IsGeminiThirdPartyAPIKey())
		})
	}
}

func TestGeminiQuotaService_QuotaForAccountSkipsThirdPartyAndVertex(t *testing.T) {
	svc := NewGeminiQuotaService(&config.Config{}, nil)
	ctx := context.Background()
	quota, ok := svc.QuotaForAccount(ctx, geminiQuotaAPIKeyAccount(""))
	require.True(t, ok)
	require.Equal(t, int64(50), quota.ProRPD)
	paid := geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com")
	paid.Credentials["tier_id"] = GeminiTierAIStudioPaid
	quota, ok = svc.QuotaForAccount(ctx, paid)
	require.True(t, ok)
	require.Equal(t, int64(-1), quota.ProRPD)
	relay := geminiQuotaAPIKeyAccount("https://relay.example.com")
	relay.Credentials["tier_id"] = GeminiTierAIStudioFree
	_, ok = svc.QuotaForAccount(ctx, relay)
	require.False(t, ok, "第三方上游不套用 AI Studio 模拟配额")
	vertex := &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"project_id": "my-project", "tier_id": "vertex"}}
	_, ok = svc.QuotaForAccount(ctx, vertex)
	require.False(t, ok, "Vertex 按量付费不套用模拟配额")
	googleOne := &Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"oauth_type": "google_one", "tier_id": GeminiTierGoogleOneFree}}
	quota, ok = svc.QuotaForAccount(ctx, googleOne)
	require.True(t, ok)
	require.Positive(t, quota.SharedRPD)
}

func geminiQuotaAPIKeyAccount(baseURL string) *Account {
	credentials := map[string]any{}
	if baseURL != "" {
		credentials["base_url"] = baseURL
	}
	return &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: credentials}
}
