package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestAnthropicAdaptiveDiagnosticSamplingRespectsSwitchRateAndForcedEvents(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-123")

	require.False(t, shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, "claude-sonnet-4-6", true))

	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 0
	require.False(t, shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, "claude-sonnet-4-6", false))
	require.True(t, shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, "claude-sonnet-4-6", true))

	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 1
	require.True(t, shouldLogAnthropicAdaptiveDiagnostic(ctx, settings, "claude-sonnet-4-6", false))
}

func TestAnthropicAdaptiveDiagnosticDecisionIncludesActualSelectionLatencyAndCooldown(t *testing.T) {
	output := captureAnthropicAdaptiveLogs(t)
	now := time.Now()
	account := &Account{
		ID:          101,
		Name:        "account-name-must-not-be-logged",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Priority:    2,
		Concurrency: 8,
		Credentials: map[string]any{"access_token": "secret-token-must-not-be-logged"},
	}
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 1
	state := defaultAnthropicAdaptiveAccountState(account, now, settings)
	state.EstimatedCapacity = 6
	state.SuccessEMA = 0.92
	state.TotalSamples = 18
	state.RecentHealthSamples = 10
	state.RecentCapacitySamples = 8
	state.RecentCapacityFailures = 1
	state.CooldownUntil = now.Add(time.Minute)
	state.LatencyByModelFamily["sonnet"] = anthropicAdaptiveLatencyState{
		TTFTEMA:    120,
		LatencyEMA: 900,
		Samples:    12,
	}
	scheduler := newAnthropicAdaptiveScheduler()
	scheduler.state.mu.Lock()
	scheduler.state.accounts[account.ID] = &state
	scheduler.state.mu.Unlock()
	decision := &AnthropicAdaptiveDecision{
		SelectedAccountID: account.ID,
		CandidateCount:    1,
		TopK:              1,
		BuildLatencyMs:    3,
		Order: []AnthropicAdaptiveCandidate{{
			Account:           account,
			LoadInfo:          &AccountLoadInfo{CurrentConcurrency: 3, WaitingCount: 1, LoadRate: 50},
			EffectiveCapacity: 6,
			Score:             0.81,
			ReliabilityScore:  0.9,
			CapacityScore:     0.7,
			LatencyScore:      0.6,
			ExplorationScore:  0.4,
			state:             state,
		}},
	}
	groupID := int64(9)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-123")
	ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "client-456")
	ctx = WithAccountSwitchCount(ctx, 2, false)
	service := &GatewayService{anthropicAdaptiveScheduler: scheduler}

	service.logAnthropicAdaptiveDiagnosticDecision(ctx, settings, anthropicAdaptiveDecisionLog{
		Mode:              AnthropicAdaptiveSchedulerModeEnforce,
		Scope:             "load_balance",
		Outcome:           "slot_acquired",
		RequestedModel:    "claude-sonnet-4-6",
		Platform:          PlatformAnthropic,
		GroupID:           &groupID,
		SessionHash:       "anthropic:session-hash",
		StickyAccountID:   202,
		Decision:          decision,
		SelectedAccount:   account,
		StickyWouldBypass: true,
		StartedAt:         now.Add(-30 * time.Millisecond),
		ExcludedCount:     7,
	})

	logText := output.String()
	require.Contains(t, logText, "anthropic_adaptive_scheduler_diagnostic_decision")
	require.Contains(t, logText, "request_id=request-123")
	require.Contains(t, logText, "client_request_id=client-456")
	require.Contains(t, logText, "mode=enforce")
	require.Contains(t, logText, "scope=load_balance")
	require.Contains(t, logText, "outcome=slot_acquired")
	require.Contains(t, logText, "sticky_account_id=202")
	require.Contains(t, logText, "sticky_would_bypass=true")
	require.Contains(t, logText, "adaptive_account_id=101")
	require.Contains(t, logText, "selected_account_id=101")
	require.Contains(t, logText, "selection_matches_adaptive=true")
	require.Contains(t, logText, "selected_effective_capacity=6")
	require.Contains(t, logText, "selected_cooldown_status=active")
	require.Contains(t, logText, "account_switch_count=2")
	require.Contains(t, logText, "account_switch_count_source=context")
	require.Contains(t, logText, "attempt_number=3")
	require.Contains(t, logText, "build_latency_ms=3")
	require.GreaterOrEqual(t, loggedInt64Field(t, logText, "latency_ms"), int64(20))
	require.NotContains(t, logText, "secret-token-must-not-be-logged")
	require.NotContains(t, logText, "account-name-must-not-be-logged")

	candidates := anthropicAdaptiveDiagnosticCandidates(decision.Order, "claude-sonnet-4-6", 5, now)
	require.Len(t, candidates, 1)
	require.Equal(t, 6, candidates[0].EffectiveCapacity)
	require.Equal(t, 0.81, candidates[0].Score)
	require.Equal(t, int64(12), candidates[0].ModelLatencySamples)
	require.Equal(t, "active", candidates[0].CooldownStatus)
}

func TestAnthropicAdaptiveDiagnosticDecisionReportsSwitchCountSource(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 1

	tests := []struct {
		name          string
		ctx           context.Context
		excludedCount int
		wantCount     string
		wantSource    string
		wantAttempt   string
	}{
		{
			name:        "unavailable on first attempt without handler context",
			ctx:         context.Background(),
			wantCount:   "account_switch_count=0",
			wantSource:  "account_switch_count_source=unavailable",
			wantAttempt: "attempt_number=1",
		},
		{
			name:          "excluded ids fallback",
			ctx:           context.Background(),
			excludedCount: 3,
			wantCount:     "account_switch_count=3",
			wantSource:    "account_switch_count_source=excluded_ids",
			wantAttempt:   "attempt_number=4",
		},
		{
			name:          "handler context wins over excluded ids",
			ctx:           WithAccountSwitchCount(context.Background(), 2, false),
			excludedCount: 7,
			wantCount:     "account_switch_count=2",
			wantSource:    "account_switch_count_source=context",
			wantAttempt:   "attempt_number=3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureAnthropicAdaptiveLogs(t)
			service := &GatewayService{}

			service.logAnthropicAdaptiveDiagnosticDecision(tt.ctx, settings, anthropicAdaptiveDecisionLog{
				Mode:           AnthropicAdaptiveSchedulerModeEnforce,
				Scope:          "load_balance",
				Outcome:        "slot_acquired",
				RequestedModel: "claude-sonnet-4-6",
				ExcludedCount:  tt.excludedCount,
			})

			logText := output.String()
			require.Contains(t, logText, tt.wantCount)
			require.Contains(t, logText, tt.wantSource)
			require.Contains(t, logText, tt.wantAttempt)
		})
	}
}

func TestAnthropicAdaptiveDiagnosticResultIncludesSwitchAndUpstreamDetails(t *testing.T) {
	output := captureAnthropicAdaptiveLogs(t)
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerMode = AnthropicAdaptiveSchedulerModeEnforce
	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 0
	account := &Account{ID: 303, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Concurrency: 10}
	before := defaultAnthropicAdaptiveAccountState(account, time.Now(), settings)
	before.EstimatedCapacity = 10
	after := before
	after.EstimatedCapacity = 8
	after.TotalSamples = 30
	after.RecentCapacitySamples = 30
	after.RecentCapacityFailures = 8
	after.ConsecutiveCapacityFailure = 3
	after.CooldownUntil = time.Now().Add(time.Minute)
	report := AnthropicAdaptiveScheduleReport{
		Account:           account,
		RequestedModel:    "claude-sonnet-4-6",
		UpstreamRequestID: "upstream-789",
		MappedModel:       "claude-sonnet-4-6-20260101",
		Stream:            true,
		HealthSample:      true,
		CapacitySample:    true,
		TerminalReason:    "concurrency_limit",
	}
	err := &UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
		Scope:      GatewayFailureScopeAccount,
		Reason:     "concurrency_limit",
	}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-failed")
	ctx = WithAccountSwitchCount(ctx, 2, false)

	service := &GatewayService{}
	service.logAnthropicAdaptiveDiagnosticResult(ctx, settings, report, before, after, true, err)

	logText := output.String()
	require.Contains(t, logText, "anthropic_adaptive_scheduler_diagnostic_result")
	require.Contains(t, logText, "request_id=request-failed")
	require.Contains(t, logText, "upstream_request_id=upstream-789")
	require.Contains(t, logText, "account_switch_count=2")
	require.Contains(t, logText, "account_switch_count_source=context")
	require.Contains(t, logText, "attempt_number=3")
	require.Contains(t, logText, "terminal_reason=concurrency_limit")
	require.Contains(t, logText, "capacity_before=10")
	require.Contains(t, logText, "estimated_capacity=8")
	require.Contains(t, logText, "cooldown_status=active")
	require.Contains(t, logText, "upstream_status=429")
	require.Contains(t, logText, "failure_scope=account")
}

func TestAnthropicAdaptiveMixedCandidatesProduceDiagnosticBypass(t *testing.T) {
	resetAnthropicAdaptiveDiagnosticSettingCache(t)
	settingsValues := map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled:                 "true",
		SettingKeyAnthropicAdaptiveSchedulerMode:                    AnthropicAdaptiveSchedulerModeEnforce,
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogEnabled:    "true",
		SettingKeyAnthropicAdaptiveSchedulerDiagnosticLogSampleRate: "0",
	}
	service := &GatewayService{
		settingService:             NewSettingService(&openAIAdvancedSchedulerSettingRepoStub{values: settingsValues}, nil),
		anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler(),
	}
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic},
		{ID: 2, Platform: PlatformAntigravity},
	}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "mixed-request")

	resolution := service.anthropicAdaptiveMode(ctx, PlatformAnthropic, accounts)

	require.Empty(t, resolution.Mode)
	require.Equal(t, "mixed_platform_candidates", resolution.BypassReason)
	require.Equal(t, 1, resolution.NativeCandidateCount)
	require.Equal(t, 1, resolution.MixedCandidateCount)

	output := captureAnthropicAdaptiveLogs(t)
	service.logAnthropicAdaptiveDiagnosticBypass(ctx, resolution.Settings, anthropicAdaptiveBypassLog{
		Reason:               resolution.BypassReason,
		Mode:                 resolution.Mode,
		Scope:                "selection",
		RequestedModel:       "claude-sonnet-4-6",
		Platform:             PlatformAnthropic,
		AccountCount:         len(accounts),
		NativeCandidateCount: resolution.NativeCandidateCount,
		MixedCandidateCount:  resolution.MixedCandidateCount,
		LoadBatchEnabled:     true,
		HasConcurrency:       true,
	})

	logText := output.String()
	require.Contains(t, logText, "anthropic_adaptive_scheduler_diagnostic_bypass")
	require.Contains(t, logText, "reason=mixed_platform_candidates")
	require.Contains(t, logText, "native_candidate_count=1")
	require.Contains(t, logText, "mixed_candidate_count=1")
}

func TestAnthropicAdaptiveErrorLogFieldsRedactAndTruncate(t *testing.T) {
	fields := anthropicAdaptiveErrorLogFields(errors.New("access_token=secret-value " + strings.Repeat("x", 2048)))
	require.NotEmpty(t, fields)
	message, ok := fields[1].(string)
	require.True(t, ok)
	require.Less(t, len(message), 1200)
	require.Contains(t, message, "access_token=***")
	require.NotContains(t, message, "secret-value")
}

type anthropicAdaptiveReleaseTrackingConcurrencyCache struct {
	ConcurrencyCache
	releaseCalls int
}

func (c *anthropicAdaptiveReleaseTrackingConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (c *anthropicAdaptiveReleaseTrackingConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.releaseCalls++
	return nil
}

type anthropicAdaptiveHydrationFailureCache struct {
	SchedulerCache
}

func (c *anthropicAdaptiveHydrationFailureCache) GetAccount(context.Context, int64) (*Account, error) {
	return nil, errors.New("hydrate account failed")
}

type anthropicAdaptiveHydrationFailureAccountRepo struct {
	AccountRepository
}

func (r *anthropicAdaptiveHydrationFailureAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return nil, errors.New("hydrate account fallback failed")
}

func TestAnthropicAdaptiveLegacyAcquireReleasesSlotWhenHydrationFails(t *testing.T) {
	concurrencyCache := &anthropicAdaptiveReleaseTrackingConcurrencyCache{}
	service := &GatewayService{
		concurrencyService: NewConcurrencyService(concurrencyCache),
		schedulerSnapshot: &SchedulerSnapshotService{
			cache:       &anthropicAdaptiveHydrationFailureCache{},
			accountRepo: &anthropicAdaptiveHydrationFailureAccountRepo{},
		},
	}
	account := &Account{
		ID:          404,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
	}

	selection, acquired, err := service.tryAcquireByLegacyOrder(
		context.Background(),
		[]*Account{account},
		nil,
		"",
		false,
		false,
		nil,
	)

	require.Error(t, err)
	require.Nil(t, selection)
	require.False(t, acquired)
	require.Equal(t, 1, concurrencyCache.releaseCalls)
}

func loggedInt64Field(t *testing.T, logText, field string) int64 {
	t.Helper()
	match := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(field) + `=(-?\d+)`).FindStringSubmatch(logText)
	require.Len(t, match, 2, "field %s missing from log: %s", field, logText)
	value, err := strconv.ParseInt(match[1], 10, 64)
	require.NoError(t, err)
	return value
}

func captureAnthropicAdaptiveLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})
	return &output
}

func resetAnthropicAdaptiveDiagnosticSettingCache(t *testing.T) {
	t.Helper()
	anthropicAdaptiveSchedulerSettingGeneration.Add(1)
	anthropicAdaptiveSchedulerSettingSF.Forget("settings")
	anthropicAdaptiveSchedulerSettingCache = atomic.Value{}
	t.Cleanup(func() {
		anthropicAdaptiveSchedulerSettingGeneration.Add(1)
		anthropicAdaptiveSchedulerSettingSF.Forget("settings")
		anthropicAdaptiveSchedulerSettingCache = atomic.Value{}
	})
}
