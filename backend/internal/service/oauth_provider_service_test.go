package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// Regression: the authorization transaction must survive its Redis round
// trip with the PKCE challenge intact — a dropped challenge voids every
// token exchange (caught live, now pinned here).
func TestOAuthAuthorizationTransactionRoundTripKeepsPKCE(t *testing.T) {
	mr := miniredis.RunT(t)
	svc := NewOAuthProviderService(nil, redisclient.NewClient(&redisclient.Options{Addr: mr.Addr()}))
	original := OAuthAuthorizationTransaction{
		ClientID:            "dsh-sub2api-sync",
		ClientName:          "DSH Sub2API Sync",
		RedirectURI:         "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback",
		Scope:               []string{"openid", "profile"},
		State:               "st-1",
		CodeChallenge:       "challenge-value",
		CodeChallengeMethod: "S256",
	}
	created, err := svc.CreateAuthorizationTransaction(context.Background(), original)
	require.NoError(t, err)
	loaded, err := svc.GetAuthorizationTransaction(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "challenge-value", loaded.CodeChallenge)
	require.Equal(t, "S256", loaded.CodeChallengeMethod)
	require.Equal(t, "st-1", loaded.State)
	require.Equal(t, original.RedirectURI, loaded.RedirectURI)
	require.ElementsMatch(t, original.Scope, loaded.Scope)
}

func TestOAuthRedirectURIAllowsOnlyLoopbackDynamicPorts(t *testing.T) {
	client := OAuthClient{
		ClientType:   "public",
		RedirectURIs: []string{"http://127.0.0.1/oauth/callback", "http://[::1]/oauth/callback"},
	}
	require.True(t, validRedirectURI(client, "http://127.0.0.1:19387/oauth/callback"))
	require.True(t, validRedirectURI(client, "http://[::1]:41231/oauth/callback"))
	require.False(t, validRedirectURI(client, "http://0.0.0.0:19387/oauth/callback"))
	require.False(t, validRedirectURI(client, "http://127.0.0.1:19387/other"))
	require.False(t, validRedirectURI(client, "https://127.0.0.1:19387/oauth/callback"))
}

// The DSH harness callback rides the plugin route path; the loopback
// port-variable exception must honor the client's *registered* paths.
func TestOAuthRedirectURIPinsLoopbackPathToRegistration(t *testing.T) {
	client := OAuthClient{
		ClientType: "public",
		RedirectURIs: []string{
			"http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback",
			"http://[::1]/plugins/dsh-sub2api-sync/oauth/callback",
		},
	}
	require.True(t, validRedirectURI(client, "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback"))
	require.True(t, validRedirectURI(client, "http://[::1]:5123/plugins/dsh-sub2api-sync/oauth/callback"))
	// Unregistered path — even loopback — is refused.
	require.False(t, validRedirectURI(client, "http://127.0.0.1:19387/oauth/callback"))
	// Exact match still works without a port.
	require.True(t, validRedirectURI(client, "http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback"))
	// Confidential clients get no loopback exception at all.
	confidential := OAuthClient{
		ClientType:   "confidential",
		RedirectURIs: []string{"http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback"},
	}
	require.False(t, validRedirectURI(confidential, "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback"))
}

func TestOAuthPKCEVerifierUsesS256(t *testing.T) {
	verifier := "dsh-test-code-verifier"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	require.True(t, verifyPKCE(verifier, challenge, "S256"))
	require.False(t, verifyPKCE("wrong", challenge, "S256"))
	require.False(t, verifyPKCE(verifier, challenge, "plain"))
}

func TestOAuthScopeStringDeduplicatesValues(t *testing.T) {
	require.Equal(t, "openid profile groups:read", scopeString([]string{"openid", "profile", "openid", "groups:read"}))
}

func TestOAuthClientListItemMarshalsSnakeCase(t *testing.T) {
	// 管理端前端按蛇形命名读取字段；缺 json tag 会序列化成帕斯卡命名，
	// 导致 redirect_uris.join 抛错、整张管理卡片渲染失败（回归保护）。
	item := OAuthClientListItem{
		OAuthClient: OAuthClient{
			ClientID:      "dsh-sub2api-sync",
			ClientType:    "public",
			Name:          "DSH Sub2API Sync",
			RedirectURIs:  []string{"http://127.0.0.1/oauth/callback"},
			AllowedScopes: []string{"openid", "profile"},
			PKCERequired:  true,
			Status:        "active",
		},
	}
	payload, err := json.Marshal(item)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	for _, key := range []string{"client_id", "client_type", "name", "description", "logo_url", "redirect_uris", "allowed_scopes", "pkce_required", "status", "created_at", "updated_at"} {
		require.Contains(t, decoded, key)
	}
	require.NotContains(t, decoded, "ClientID")
	require.NotContains(t, decoded, "RedirectURIs")
}
