//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resetAnthropicAdaptiveSchedulerSettingCacheForTest() {
	anthropicAdaptiveSchedulerSettingGeneration.Add(1)
	anthropicAdaptiveSchedulerSettingSF.Forget("settings")
	anthropicAdaptiveSchedulerSettingCache = atomic.Value{}
}

func TestAnthropicAdaptiveEnforceBypassesBusyStickyAndPreservesBinding(t *testing.T) {
	resetAnthropicAdaptiveSchedulerSettingCacheForTest()
	defer resetAnthropicAdaptiveSchedulerSettingCacheForTest()

	repo := anthropicAdaptiveTestSettingRepo(AnthropicAdaptiveSchedulerModeEnforce)
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	accountRepo := &mockAccountRepoForPlatform{accounts: accounts, accountsByID: map[int64]*Account{}}
	for i := range accountRepo.accounts {
		accountRepo.accountsByID[accountRepo.accounts[i].ID] = &accountRepo.accounts[i]
	}
	stickyCache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"sticky": 1}}
	concurrencyCache := &mockConcurrencyCache{
		acquireResults: map[int64]bool{1: false, 2: true},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 2, LoadRate: 40},
			2: {AccountID: 2, CurrentConcurrency: 0, LoadRate: 0},
		},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &GatewayService{
		accountRepo:                accountRepo,
		cache:                      stickyCache,
		cfg:                        cfg,
		concurrencyService:         NewConcurrencyService(concurrencyCache),
		settingService:             NewSettingService(repo, cfg),
		anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler(),
	}

	result, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "sticky", "claude-sonnet-4-6", nil, "", 0)

	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(2), result.Account.ID)
	require.Equal(t, int64(1), stickyCache.sessionBindings["sticky"])
}

func TestAnthropicAdaptiveShadowKeepsBusyStickyWaitBehavior(t *testing.T) {
	resetAnthropicAdaptiveSchedulerSettingCacheForTest()
	defer resetAnthropicAdaptiveSchedulerSettingCacheForTest()

	repo := anthropicAdaptiveTestSettingRepo(AnthropicAdaptiveSchedulerModeShadow)
	account := Account{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5}
	accountRepo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{1: &account},
	}
	stickyCache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"sticky": 1}}
	concurrencyCache := &mockConcurrencyCache{
		acquireResults: map[int64]bool{1: false},
		waitCounts:     map[int64]int{1: 0},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 1
	svc := &GatewayService{
		accountRepo:                accountRepo,
		cache:                      stickyCache,
		cfg:                        cfg,
		concurrencyService:         NewConcurrencyService(concurrencyCache),
		settingService:             NewSettingService(repo, cfg),
		anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler(),
	}

	result, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "sticky", "claude-sonnet-4-6", nil, "", 0)

	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.NotNil(t, result.WaitPlan)
	require.Equal(t, int64(1), result.Account.ID)
	require.Zero(t, concurrencyCache.loadBatchCalls)
}

func TestAnthropicAdaptiveModelRouteUsesFallbackWaitAfterAllSlotsBusy(t *testing.T) {
	resetAnthropicAdaptiveSchedulerSettingCacheForTest()
	defer resetAnthropicAdaptiveSchedulerSettingCacheForTest()

	groupID := int64(20)
	repo := anthropicAdaptiveTestSettingRepo(AnthropicAdaptiveSchedulerModeEnforce)
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	accountRepo := &mockAccountRepoForPlatform{accounts: accounts, accountsByID: map[int64]*Account{}}
	for i := range accountRepo.accounts {
		accountRepo.accountsByID[accountRepo.accounts[i].ID] = &accountRepo.accounts[i]
	}
	groupRepo := &mockGroupRepoForGateway{groups: map[int64]*Group{
		groupID: {
			ID:                  groupID,
			Platform:            PlatformAnthropic,
			Status:              StatusActive,
			Hydrated:            true,
			ModelRoutingEnabled: true,
			ModelRouting: map[string][]int64{
				"claude-sonnet-4-6": {1, 2},
			},
		},
	}}
	stickyCache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"sticky": 1}}
	concurrencyCache := &mockConcurrencyCache{
		acquireResults: map[int64]bool{1: false, 2: false},
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 45 * time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 2
	cfg.Gateway.Scheduling.FallbackWaitTimeout = 17 * time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 9
	svc := &GatewayService{
		accountRepo:                accountRepo,
		groupRepo:                  groupRepo,
		cache:                      stickyCache,
		cfg:                        cfg,
		concurrencyService:         NewConcurrencyService(concurrencyCache),
		settingService:             NewSettingService(repo, cfg),
		anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler(),
	}

	result, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "sticky", "claude-sonnet-4-6", nil, "", 0)

	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.NotNil(t, result.WaitPlan)
	require.Equal(t, 17*time.Second, result.WaitPlan.Timeout)
	require.Equal(t, 9, result.WaitPlan.MaxWaiting)
	require.True(t, result.PreserveStickyBinding)
	require.Equal(t, int64(1), stickyCache.sessionBindings["sticky"])
}

func anthropicAdaptiveTestSettingRepo(mode string) SettingRepository {
	return &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled: "true",
		SettingKeyAnthropicAdaptiveSchedulerMode:    mode,
	}}
}
