package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnthropicAdaptiveDiagnosticSamplingRespectsSwitchRateAndForcedEvents(t *testing.T) {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	require.False(t, shouldLogAnthropicAdaptiveDiagnostic(t.Context(), settings, "claude-sonnet-4", false))

	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 0
	require.False(t, shouldLogAnthropicAdaptiveDiagnostic(t.Context(), settings, "claude-sonnet-4", false))
	require.True(t, shouldLogAnthropicAdaptiveDiagnostic(t.Context(), settings, "claude-sonnet-4", true))

	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 1
	require.True(t, shouldLogAnthropicAdaptiveDiagnostic(t.Context(), settings, "claude-sonnet-4", false))
}

func TestAnthropicAdaptiveShadowDecisionRequiresDiagnosticsAndAlwaysLogsDivergence(t *testing.T) {
	output := captureAnthropicAdaptiveLogs(t)
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	service := &GatewayService{anthropicAdaptiveScheduler: newAnthropicAdaptiveScheduler()}
	selected := &Account{ID: 41, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}

	service.logAnthropicAdaptiveDecision(context.Background(), settings, anthropicAdaptiveDecisionLog{
		Mode:              AnthropicAdaptiveSchedulerModeShadow,
		RequestedModel:    "claude-sonnet-4",
		BaselineAccountID: selected.ID,
		SelectedAccount:   selected,
		Decision:          &AnthropicAdaptiveDecision{SelectedAccountID: selected.ID},
	})
	require.Empty(t, output.String())

	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 0
	service.logAnthropicAdaptiveDecision(context.Background(), settings, anthropicAdaptiveDecisionLog{
		Mode:              AnthropicAdaptiveSchedulerModeShadow,
		RequestedModel:    "claude-sonnet-4",
		BaselineAccountID: selected.ID,
		SelectedAccount:   selected,
		Decision:          &AnthropicAdaptiveDecision{SelectedAccountID: 42},
	})
	require.Contains(t, output.String(), "anthropic_adaptive_shadow_decision")
	require.Contains(t, output.String(), "shadow_diverged=true")
}

func TestAnthropicAdaptiveDiagnosticCandidatesUseAccountCoreState(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	account := &Account{ID: 41, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 20}
	state := newAdaptiveAccountState(account.ID, account.Concurrency, now)
	state.SuccessEMA = 0.91
	state.TTFTEMA = 180
	state.TTFTSamples = 12
	state.CircuitOpenUntil = now.Add(time.Minute)
	state.CircuitOpenCount = 2
	state.HealthObservations = []adaptiveHealthObservation{
		{At: now.Add(-time.Minute), Success: true},
		{At: now, Success: true},
	}
	candidate := AnthropicAdaptiveCandidate{
		Account:           account,
		LoadInfo:          &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 3, WaitingCount: 1, LoadRate: 15},
		EffectiveCapacity: 18,
		Score:             0.8,
		ReliabilityScore:  0.9,
		CapacityScore:     0.75,
		LatencyScore:      0.7,
		CostScore:         0.6,
		coreState:         *state,
	}

	defaultGot := anthropicAdaptiveDiagnosticCandidates([]AnthropicAdaptiveCandidate{candidate}, "claude-sonnet-4", 5, now, defaultAdaptiveCoreSettings())
	settings := defaultAdaptiveCoreSettings()
	settings.LearningMinHealthSamples = 2
	got := anthropicAdaptiveDiagnosticCandidates([]AnthropicAdaptiveCandidate{candidate}, "claude-sonnet-4", 5, now, settings)

	require.Len(t, defaultGot, 1)
	require.Equal(t, string(adaptiveLearningLearning), defaultGot[0].LearningStatus)
	require.Len(t, got, 1)
	require.Equal(t, account.ID, got[0].AccountID)
	require.Equal(t, 180.0, got[0].TTFTEMA)
	require.Equal(t, int64(12), got[0].TTFTSamples)
	require.Equal(t, 2, got[0].CircuitOpenCount)
	require.Equal(t, "open", got[0].CircuitStatus)
	require.Equal(t, 2, got[0].HealthSamples)
	require.Equal(t, string(adaptiveLearningLearned), got[0].LearningStatus)
}

func TestAnthropicAdaptiveDiagnosticDecisionIncludesSelectedCoreRuntime(t *testing.T) {
	output := captureAnthropicAdaptiveLogs(t)
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 1
	scheduler := newAnthropicAdaptiveScheduler()
	now := time.Now()
	scheduler.now = func() time.Time { return now }
	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 20}
	scheduler.core.mu.Lock()
	state := scheduler.core.ensureLocked(account.ID, account.Concurrency, now)
	state.CircuitOpenUntil = now.Add(time.Minute)
	state.CircuitOpenCount = 2
	state.QuotaLimited = true
	scheduler.core.mu.Unlock()
	service := &GatewayService{anthropicAdaptiveScheduler: scheduler}

	service.logAnthropicAdaptiveDiagnosticDecision(context.Background(), settings, anthropicAdaptiveDecisionLog{
		Mode:            AnthropicAdaptiveSchedulerModeEnforce,
		Scope:           "group",
		Outcome:         "selected",
		RequestedModel:  "claude-sonnet-4",
		SelectedAccount: account,
		StartedAt:       time.Now().Add(-20 * time.Millisecond),
	})

	logOutput := output.String()
	require.Contains(t, logOutput, "selected_effective_capacity=20")
	require.Contains(t, logOutput, "selected_circuit_open_count=2")
	require.Contains(t, logOutput, "selected_quota_limited=true")
	require.Regexp(t, `latency_ms=[1-9][0-9]*`, logOutput)
	require.NotContains(t, logOutput, "model_family")
}

func TestAnthropicAdaptiveDiagnosticResultUsesAccountCoreFields(t *testing.T) {
	output := captureAnthropicAdaptiveLogs(t)
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerDiagnosticLogEnabled = true
	settings.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate = 1
	account := &Account{ID: 43, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 20}
	now := time.Now()
	before := *newAdaptiveAccountState(account.ID, account.Concurrency, now)
	after := before
	after.SuccessEMA = 0.8
	after.TTFTEMA = 210
	after.TTFTSamples = 4
	after.ConsecutiveFailures = 1
	after.CapacityGeneration = 3
	after.QuotaLimited = true
	service := &GatewayService{}

	service.logAnthropicAdaptiveDiagnosticResult(context.Background(), settings, AnthropicAdaptiveScheduleReport{
		Account: account, RequestedModel: "claude-sonnet-4", TerminalReason: "quota_rate_limit",
	}, before, after, false, &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests})

	logOutput := output.String()
	require.Contains(t, logOutput, "ttft_ema=210")
	require.Contains(t, logOutput, "capacity_generation=3")
	require.Contains(t, logOutput, "quota_limited=true")
	require.NotContains(t, logOutput, "model_success_ema")
}

func TestAnthropicAdaptiveErrorLogFieldsTruncate(t *testing.T) {
	err := &UpstreamFailoverError{
		StatusCode:    http.StatusBadGateway,
		Stage:         GatewayFailureStageInference,
		Scope:         GatewayFailureScopeAccount,
		ResponseBody:  []byte(strings.Repeat("x", 2048)),
		ClientMessage: strings.Repeat("x", 2048),
	}
	fields := anthropicAdaptiveErrorLogFields(errors.New(err.Error() + strings.Repeat("x", 2048)))
	text := strings.TrimSpace(slogFieldsText(fields))

	require.LessOrEqual(t, len(text), 1400)
}

func captureAnthropicAdaptiveLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &output
}

func slogFieldsText(fields []any) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprint(field))
	}
	return strings.Join(parts, " ")
}
