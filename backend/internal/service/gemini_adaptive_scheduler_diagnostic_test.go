package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGeminiAdaptiveDiagnosticSamplingRespectsSwitchRateAndForcedEvents(t *testing.T) {
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-123")

	require.False(t, shouldLogGeminiAdaptiveDiagnostic(ctx, settings, "gemini-2.5-pro", "generateContent", true))

	settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = 0
	require.False(t, shouldLogGeminiAdaptiveDiagnostic(ctx, settings, "gemini-2.5-pro", "generateContent", false))
	require.True(t, shouldLogGeminiAdaptiveDiagnostic(ctx, settings, "gemini-2.5-pro", "generateContent", true))

	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = 1
	require.True(t, shouldLogGeminiAdaptiveDiagnostic(ctx, settings, "gemini-2.5-pro", "generateContent", false))
}

func TestGeminiAdaptiveDiagnosticDecisionIncludesRequestAndCandidateDetails(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	now := time.Now()
	account := &Account{ID: 101, Platform: PlatformGemini, Type: AccountTypeOAuth, Priority: 2, Concurrency: 8}
	decision := &GeminiAdaptiveDecision{
		BaselineAccountID:   202,
		SelectedAccountID:   account.ID,
		InputCandidateCount: 3,
		CandidateCount:      2,
		HardRejectedCount:   1,
		TopK:                2,
		BuildLatencyMs:      4,
		Order: []GeminiAdaptiveCandidate{{
			Account: account,
			Load:    &AccountLoadInfo{CurrentConcurrency: 3, WaitingCount: 1, LoadRate: 50},
			Quota: GeminiAdaptiveQuotaSnapshot{
				Scope:         GeminiAdaptiveQuotaScope{Daily: GeminiQuotaBucketPro, Minute: GeminiQuotaBucketShared},
				DailyUsed:     20,
				DailyLimit:    100,
				DataAvailable: true,
			},
			EffectiveCapacity: 6,
			Score:             0.81,
			ReliabilityScore:  0.9,
			QuotaScore:        0.8,
			CapacityScore:     0.7,
			LatencyScore:      0.6,
			CostScore:         0.5,
			ExplorationScore:  0.4,
			state: geminiAdaptiveAccountState{
				PathSuccessEMA: 0.93,
				ByModelFamily: map[string]geminiAdaptiveModelState{
					"pro": {SuccessEMA: 0.88, TTFTEMA: 120, LatencyEMA: 900, Samples: 12, Failures: 1},
				},
				TotalSamples:           18,
				RecentHealthSamples:    10,
				RecentCapacitySamples:  8,
				RecentCapacityFailures: 1,
				CooldownUntil:          now.Add(time.Minute),
			},
		}},
	}
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = 1
	groupID := int64(9)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-123")
	ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "client-456")
	ctx = WithGeminiAdaptiveRequestHint(ctx, "generateContent", true)

	service := &GatewayService{}
	service.logGeminiAdaptiveDiagnosticDecision(ctx, settings, geminiAdaptiveDecisionLog{
		Mode:            GeminiAdaptiveSchedulerModeEnforce,
		Scope:           "load_balance",
		Outcome:         "slot_acquired",
		RequestedModel:  "gemini-2.5-pro",
		GroupID:         &groupID,
		SessionHash:     "gemini:session-hash",
		Decision:        decision,
		SelectedAccount: account,
	})

	logText := output.String()
	require.Contains(t, logText, "gemini_adaptive_scheduler_diagnostic_decision")
	require.Contains(t, logText, "request_id=request-123")
	require.Contains(t, logText, "client_request_id=client-456")
	require.Contains(t, logText, "mode=enforce")
	require.Contains(t, logText, "scope=load_balance")
	require.Contains(t, logText, "outcome=slot_acquired")
	require.Contains(t, logText, "baseline_account_id=202")
	require.Contains(t, logText, "adaptive_account_id=101")
	require.Contains(t, logText, "hard_rejected_count=1")
	require.Contains(t, logText, "candidates=")

	candidates := geminiAdaptiveDiagnosticCandidates(decision.Order, "gemini-2.5-pro", "generateContent", 5, now)
	require.Len(t, candidates, 1)
	require.Equal(t, 6, candidates[0].EffectiveCapacity)
	require.Equal(t, 0.81, candidates[0].Score)
	require.Equal(t, GeminiQuotaBucketPro, candidates[0].Quota.Scope.Daily)
	require.Equal(t, int64(12), candidates[0].ModelSamples)
	require.Equal(t, "active", candidates[0].CooldownStatus)
}

func TestGeminiAdaptiveDiagnosticResultLogsFailureWithoutSampling(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = 0
	account := &Account{ID: 303, Platform: PlatformGemini, Type: AccountTypeOAuth, Concurrency: 10}
	before := defaultGeminiAdaptiveAccountState(account, time.Now(), settings)
	before.EstimatedCapacity = 10
	after := before
	after.EstimatedCapacity = 8
	after.TotalSamples = 30
	after.RecentCapacitySamples = 30
	after.RecentCapacityFailures = 8
	after.ConsecutiveCapacityFailure = 3
	after.CooldownUntil = time.Now().Add(time.Minute)
	report := GeminiAdaptiveScheduleReport{
		Account:           account,
		RequestedModel:    "gemini-2.5-pro",
		UpstreamRequestID: "upstream-789",
		Action:            "generateContent",
		PathSample:        true,
		ModelSample:       true,
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
	service.logGeminiAdaptiveDiagnosticResult(ctx, settings, report, before, after, false, true, err)

	logText := output.String()
	require.Contains(t, logText, "gemini_adaptive_scheduler_diagnostic_result")
	require.Contains(t, logText, "request_id=request-failed")
	require.Contains(t, logText, "upstream_request_id=upstream-789")
	require.Contains(t, logText, "account_switch_count=2")
	require.Contains(t, logText, "attempt_number=3")
	require.Contains(t, logText, "terminal_reason=concurrency_limit")
	require.Contains(t, logText, "capacity_before=10")
	require.Contains(t, logText, "estimated_capacity=8")
	require.Contains(t, logText, "upstream_status=429")
	require.Contains(t, logText, "failure_scope=account")
}

func TestGeminiAdaptiveShadowDiagnosticUsesActualBaselineAndForcesDivergence(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = 0
	adaptive := &Account{ID: 101, Platform: PlatformGemini, Type: AccountTypeOAuth}
	baseline := &Account{ID: 202, Platform: PlatformGemini, Type: AccountTypeOAuth}
	decision := &GeminiAdaptiveDecision{
		BaselineAccountID: 101,
		SelectedAccountID: adaptive.ID,
		CandidateCount:    2,
		Order:             []GeminiAdaptiveCandidate{{Account: adaptive}},
	}
	service := &GatewayService{geminiAdaptiveScheduler: newGeminiAdaptiveScheduler()}

	service.logGeminiAdaptiveShadowDecision(
		context.Background(),
		decision,
		baseline,
		"gemini-2.5-pro",
		nil,
		"gemini:session",
		"load_balance",
		false,
		settings,
	)

	logText := output.String()
	require.Contains(t, logText, "gemini_adaptive_shadow_decision")
	require.Contains(t, logText, "planned_baseline_account_id=101")
	require.Contains(t, logText, "baseline_account_id=202")
	require.Contains(t, logText, "adaptive_account_id=101")
	require.Contains(t, logText, "shadow_diverged=true")
	require.Equal(t, uint64(1), service.geminiAdaptiveScheduler.SnapshotMetrics().ShadowDivergeTotal)
}

func TestGeminiAdaptiveErrorLogFieldsTruncateLongErrors(t *testing.T) {
	fields := geminiAdaptiveErrorLogFields(errors.New("access_token=secret-value " + strings.Repeat("x", 2048)))
	require.NotEmpty(t, fields)
	message, ok := fields[1].(string)
	require.True(t, ok)
	require.Less(t, len(message), 1200)
	require.Contains(t, message, "access_token=***")
	require.NotContains(t, message, "secret-value")
}

func captureGeminiAdaptiveLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})
	return &output
}
