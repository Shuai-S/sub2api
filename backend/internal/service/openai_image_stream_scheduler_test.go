package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountSupportsOpenAIImagesStreamOnlyStrictFalseDisables(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		{name: "missing", want: true},
		{name: "true", extra: map[string]any{openAIImagesStreamSupportedExtraKey: true}, want: true},
		{name: "false", extra: map[string]any{openAIImagesStreamSupportedExtraKey: false}, want: false},
		{name: "string false", extra: map[string]any{openAIImagesStreamSupportedExtraKey: "false"}, want: true},
		{name: "numeric zero", extra: map[string]any{openAIImagesStreamSupportedExtraKey: 0}, want: true},
		{name: "nil", extra: map[string]any{openAIImagesStreamSupportedExtraKey: nil}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Extra: tt.extra}
			require.Equal(t, tt.want, account.SupportsOpenAIImagesStream())
		})
	}
	require.False(t, (*Account)(nil).SupportsOpenAIImagesStream())
}

func TestOpenAIAccountImageStreamRequirementByAccountType(t *testing.T) {
	oauth := &Account{Type: AccountTypeOAuth}
	apiKey := &Account{Type: AccountTypeAPIKey}

	require.True(t, openAIAccountRequiresImageStream(oauth, OpenAIImagesCapabilityBasic, false))
	require.True(t, openAIAccountRequiresImageStream(oauth, OpenAIImagesCapabilityNative, true))
	require.False(t, openAIAccountRequiresImageStream(apiKey, OpenAIImagesCapabilityBasic, false))
	require.True(t, openAIAccountRequiresImageStream(apiKey, OpenAIImagesCapabilityBasic, true))
	require.False(t, openAIAccountRequiresImageStream(oauth, "", true))
}

func TestOpenAIAccountImageStreamCooldownMarkCheckAndExpiry(t *testing.T) {
	var cooldowns openAIAccountImageStreamCooldowns
	now := time.Now()

	cooldowns.mark(101, now)
	require.True(t, cooldowns.active(101, now.Add(openAIAccountImageStreamCooldownDuration-time.Nanosecond)))
	require.False(t, cooldowns.active(101, now.Add(openAIAccountImageStreamCooldownDuration)))
	require.False(t, cooldowns.active(101, now.Add(openAIAccountImageStreamCooldownDuration+time.Minute)))
}

func TestOpenAIImageStreamCapabilityAndCooldownCompatibility(t *testing.T) {
	svc := &OpenAIGatewayService{}
	oauthUnsupported := &Account{
		ID:       201,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{openAIImagesStreamSupportedExtraKey: false},
	}
	apiKeyUnsupported := &Account{
		ID:       202,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{openAIImagesStreamSupportedExtraKey: false},
	}
	apiKeySupported := &Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.False(t, svc.accountSupportsOpenAIRequestCapabilities(oauthUnsupported, "", OpenAIImagesCapabilityBasic, false))
	require.True(t, svc.accountSupportsOpenAIRequestCapabilities(apiKeyUnsupported, "", OpenAIImagesCapabilityBasic, false))
	require.False(t, svc.accountSupportsOpenAIRequestCapabilities(apiKeyUnsupported, "", OpenAIImagesCapabilityBasic, true))
	require.True(t, svc.accountSupportsOpenAIRequestCapabilities(apiKeySupported, "", OpenAIImagesCapabilityBasic, true))

	svc.MarkOpenAIImageStreamUnsupported(apiKeySupported.ID)
	require.True(t, svc.IsOpenAIImageStreamUnsupported(apiKeySupported.ID))
	require.False(t, svc.accountSupportsOpenAIRequestCapabilities(apiKeySupported, "", OpenAIImagesCapabilityBasic, true))
	require.True(t, svc.accountSupportsOpenAIRequestCapabilities(apiKeySupported, "", OpenAIImagesCapabilityBasic, false))
}

func newOpenAIImageStreamSchedulerTestService(accounts []Account, cache GatewayCache, concurrencyCache ConcurrencyCache) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = len(accounts)
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1
	return &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}
}

func openAIImageStreamSchedulerAccounts() []Account {
	return []Account{
		{
			ID:          301,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{openAIImagesStreamSupportedExtraKey: false},
		},
		{
			ID:          302,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    10,
		},
	}
}

func openAIImageStreamScheduleRequest() OpenAIAccountScheduleRequest {
	return OpenAIAccountScheduleRequest{
		Platform:                PlatformOpenAI,
		RequiredTransport:       OpenAIUpstreamTransportHTTPSSE,
		RequiredImageCapability: OpenAIImagesCapabilityBasic,
		RequireImageStream:      true,
	}
}

func TestDefaultOpenAIAccountSchedulerFiltersImageStreamCapability(t *testing.T) {
	accounts := openAIImageStreamSchedulerAccounts()
	svc := newOpenAIImageStreamSchedulerTestService(accounts, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{})
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	selection, decision, err := scheduler.Select(context.Background(), openAIImageStreamScheduleRequest())

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(302), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestDefaultOpenAIAccountSchedulerFiltersImageStreamCooldown(t *testing.T) {
	accounts := openAIImageStreamSchedulerAccounts()
	accounts[0].Extra = nil
	svc := newOpenAIImageStreamSchedulerTestService(accounts, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{})
	svc.MarkOpenAIImageStreamUnsupported(accounts[0].ID)
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	selection, _, err := scheduler.Select(context.Background(), openAIImageStreamScheduleRequest())

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(302), selection.Account.ID)
}

func TestDefaultOpenAIAccountSchedulerStickyAndFallbackRespectImageStreamConstraint(t *testing.T) {
	accounts := openAIImageStreamSchedulerAccounts()
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"image-stream-sticky": 301}}
	concurrencyCache := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{302: false}}
	svc := newOpenAIImageStreamSchedulerTestService(accounts, cache, concurrencyCache)
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	req := openAIImageStreamScheduleRequest()
	req.SessionHash = "image-stream-sticky"
	req.StickyAccountID = 301

	selection, decision, err := scheduler.Select(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(302), selection.Account.ID)
	require.False(t, decision.StickySessionHit)
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(302), selection.WaitPlan.AccountID)
}

func TestAdaptiveOpenAIAccountSchedulerFiltersImageStreamCapability(t *testing.T) {
	accounts := openAIImageStreamSchedulerAccounts()
	svc := newOpenAIImageStreamSchedulerTestService(accounts, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{})
	scheduler := &adaptiveOpenAIAccountScheduler{
		service:  svc,
		baseline: &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()},
		state:    newOpenAIAdaptiveSchedulerStateStore(),
	}
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerTopK = len(accounts)

	order, candidateCount, _, err := scheduler.buildAdaptiveSelectionOrder(context.Background(), openAIImageStreamScheduleRequest(), cfg)

	require.NoError(t, err)
	require.Equal(t, 1, candidateCount)
	require.Len(t, order, 1)
	require.Equal(t, int64(302), order[0].account.ID)
}

func TestOpenAIImagesNativeToBasicFallbackKeepsImageStreamConstraint(t *testing.T) {
	oauth := Account{
		ID:          401,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra:       map[string]any{openAIImagesStreamSupportedExtraKey: false},
	}
	svc := newOpenAIImageStreamSchedulerTestService([]Account{oauth}, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{})
	svc.MarkOpenAIImageStreamUnsupported(oauth.ID)

	selection, _, err := svc.SelectAccountWithSchedulerForImages(
		context.Background(), nil, "", "", nil, OpenAIImagesCapabilityNative, false,
	)

	require.Error(t, err)
	require.Nil(t, selection)

	selection, _, err = svc.SelectAccountWithSchedulerForImages(
		context.Background(), nil, "", "", nil, OpenAIImagesCapabilityNative, true,
	)
	require.Error(t, err)
	require.Nil(t, selection)
}
