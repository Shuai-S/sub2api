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

func TestGeminiAdaptiveDiagnosticCandidatesUseAccountCoreState(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	account := &Account{ID: 41, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 20}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)
	state.SuccessEMA = 0.91
	state.TTFTEMA = 180
	state.TTFTSamples = 12
	state.CircuitOpenUntil = now.Add(time.Minute)
	state.CircuitOpenCount = 2
	candidate := GeminiAdaptiveCandidate{
		Account:           account,
		Load:              &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 3, WaitingCount: 1, LoadRate: 15},
		Quota:             GeminiAdaptiveQuotaSnapshot{DataAvailable: true, DailyLimit: 100, DailyUsed: 20},
		EffectiveCapacity: 18,
		Score:             0.8,
		ReliabilityScore:  0.9,
		CapacityScore:     0.75,
		LatencyScore:      0.7,
		CostScore:         0.6,
		coreState:         *state,
	}

	got := geminiAdaptiveDiagnosticCandidates([]GeminiAdaptiveCandidate{candidate}, "gemini-2.5-pro", "generateContent", 5, now, defaultAdaptiveCoreSettings())

	require.Len(t, got, 1)
	require.Equal(t, account.ID, got[0].AccountID)
	require.Equal(t, 180.0, got[0].TTFTEMA)
	require.Equal(t, int64(12), got[0].TTFTSamples)
	require.Equal(t, 2, got[0].CircuitOpenCount)
	require.Equal(t, "open", got[0].CircuitStatus)
	require.True(t, got[0].Quota.DataAvailable)
}

func TestGeminiAdaptiveDiagnosticDecisionIncludesRequestAndCandidateDetails(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	now := time.Now()
	account := &Account{ID: 101, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 8}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)
	state.SuccessEMA = 0.93
	decision := &GeminiAdaptiveDecision{
		BaselineAccountID:   202,
		SelectedAccountID:   account.ID,
		InputCandidateCount: 3,
		CandidateCount:      2,
		HardRejectedCount:   1,
		TopK:                2,
		BuildLatencyMs:      4,
		Order: []GeminiAdaptiveCandidate{{
			Account: account, Load: &AccountLoadInfo{CurrentConcurrency: 3, WaitingCount: 1, LoadRate: 50},
			EffectiveCapacity: 6, Score: 0.81, ReliabilityScore: 0.9, CapacityScore: 0.7, LatencyScore: 0.6, CostScore: 0.5,
			coreState: *state,
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
		Mode: GeminiAdaptiveSchedulerModeEnforce, Scope: "load_balance", Outcome: "slot_acquired",
		RequestedModel: "gemini-2.5-pro", GroupID: &groupID, SessionHash: "gemini:session-hash",
		Decision: decision, SelectedAccount: account,
	})

	logText := output.String()
	require.Contains(t, logText, "gemini_adaptive_scheduler_diagnostic_decision")
	require.Contains(t, logText, "request_id=request-123")
	require.Contains(t, logText, "client_request_id=client-456")
	require.Contains(t, logText, "baseline_account_id=202")
	require.Contains(t, logText, "adaptive_account_id=101")
	require.Contains(t, logText, "hard_rejected_count=1")
	require.NotContains(t, logText, "model_family")
}

func TestGeminiAdaptiveDiagnosticResultUsesAccountCoreFields(t *testing.T) {
	output := captureGeminiAdaptiveLogs(t)
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	settings.GeminiAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.GeminiAdaptiveSchedulerDiagnosticLogSampleRate = 0
	account := &Account{ID: 303, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 10}
	now := time.Now()
	before := *newAdaptiveAccountState(account.ID, account.Concurrency, now)
	after := before
	after.EffectiveCapacity = 8
	after.SuccessEMA = 0.8
	after.TTFTEMA = 210
	after.TTFTSamples = 4
	after.ConsecutiveFailures = 1
	after.CapacityGeneration = 3
	after.QuotaLimited = true
	report := GeminiAdaptiveScheduleReport{Account: account, RequestedModel: "gemini-2.5-pro", UpstreamRequestID: "upstream-789", Action: "generateContent", TerminalReason: "concurrency_limit"}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-failed")
	ctx = WithAccountSwitchCount(ctx, 2, false)

	service := &GatewayService{}
	service.logGeminiAdaptiveDiagnosticResult(ctx, settings, report, before, after, false, true, &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: GatewayFailureScopeAccount})

	logText := output.String()
	require.Contains(t, logText, "gemini_adaptive_scheduler_diagnostic_result")
	require.Contains(t, logText, "upstream_request_id=upstream-789")
	require.Contains(t, logText, "account_switch_count=2")
	require.Contains(t, logText, "effective_capacity=8")
	require.Contains(t, logText, "ttft_ema=210")
	require.Contains(t, logText, "capacity_generation=3")
	require.Contains(t, logText, "quota_limited=true")
	require.NotContains(t, logText, "model_success_ema")
}

func TestGeminiAdaptiveErrorLogFieldsTruncateLongErrors(t *testing.T) {
	fields := geminiAdaptiveErrorLogFields(errors.New("access_token=secret-value " + strings.Repeat("x", 2048)))
	require.NotEmpty(t, fields)
	message, ok := fields[1].(string)
	require.True(t, ok)
	require.Less(t, len(message), 1200)
}

func captureGeminiAdaptiveLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	return &output
}
