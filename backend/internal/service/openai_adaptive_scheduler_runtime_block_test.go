package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAdaptiveFallbackPreservesRuntimeBlockUntilRecovery(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run("advanced="+strconv.FormatBool(advanced), func(t *testing.T) {
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			// The repository still exposes the account from before the cooldown
			// write, as with a failed write or a scheduling snapshot that lags.
			account := Account{ID: 22009, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
			var acquiredIDs []int64
			service := &OpenAIGatewayService{
				accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(strconv.FormatBool(advanced)),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
			}
			scheduler := newOpenAIAdaptiveTestScheduler(service)
			service.BlockAccountScheduling(&account, time.Now().Add(time.Hour), "upstream_disable")
			req := OpenAIAccountScheduleRequest{
				Platform:          PlatformOpenAI,
				RequestedModel:    "gpt-5.1",
				RequiredTransport: OpenAIUpstreamTransportAny,
			}

			selection, _, _, _, _, err := scheduler.selectByAdaptiveLoadBalance(context.Background(), req, DefaultOpenAIAdaptiveSchedulerSettings())
			if selection != nil && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.Nil(t, selection)
			require.Empty(t, acquiredIDs, "fallback must reject the blocked account before acquiring a slot")
			require.True(t, service.isOpenAIAccountRuntimeBlocked(&account))

			service.ClearAccountSchedulingBlock(account.ID)
			selection, _, _, _, _, err = scheduler.selectByAdaptiveLoadBalance(context.Background(), req, DefaultOpenAIAdaptiveSchedulerSettings())
			require.NoError(t, err)
			require.NotNil(t, selection)
			t.Cleanup(selection.ReleaseFunc)
			require.True(t, selection.Acquired)
			require.Equal(t, account.ID, selection.Account.ID)
		})
	}
}

func TestOpenAIAdaptiveDegradedFallbackSkipsBlockedAccountForHealthyAccount(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run("advanced="+strconv.FormatBool(advanced), func(t *testing.T) {
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			blocked := Account{ID: 22010, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
			healthy := blocked
			healthy.ID = 22011
			healthy.Priority = 1
			var acquiredIDs []int64
			service := &OpenAIGatewayService{
				accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{blocked, healthy}},
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(strconv.FormatBool(advanced)),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
			}
			scheduler := newOpenAIAdaptiveTestScheduler(service)
			service.BlockAccountScheduling(&blocked, time.Now().Add(time.Hour), "upstream_disable")

			selection, err := scheduler.degradedAdaptiveFallback(context.Background(), OpenAIAccountScheduleRequest{
				Platform:          PlatformOpenAI,
				RequestedModel:    "gpt-5.1",
				RequiredTransport: OpenAIUpstreamTransportAny,
			}, "runtime_unavailable", errOpenAIAdaptiveSchedulerFallback)
			require.NoError(t, err)
			require.NotNil(t, selection)
			t.Cleanup(selection.ReleaseFunc)
			require.True(t, selection.Acquired)
			require.Equal(t, healthy.ID, selection.Account.ID)
			require.NotContains(t, acquiredIDs, blocked.ID)
			require.True(t, service.isOpenAIAccountRuntimeBlocked(&blocked))
		})
	}
}
