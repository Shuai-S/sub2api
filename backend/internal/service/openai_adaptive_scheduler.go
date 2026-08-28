package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const openAIAccountScheduleLayerAdaptive = "adaptive"

var errOpenAIAdaptiveSchedulerFallback = errors.New("openai adaptive scheduler fallback")

const openAIAdaptiveStickyCleanupMinInterval = 30 * time.Second

const openAIAdaptiveHealthProbeScanInterval = time.Second

type adaptiveOpenAIAccountScheduler struct {
	service               *OpenAIGatewayService
	baseline              *defaultOpenAIAccountScheduler
	core                  *adaptiveStateStore
	metrics               openAIAccountSchedulerMetrics
	stickyCleanupMu       sync.Mutex
	stickyCleanupLastByID map[int64]time.Time
	healthProbeScanMu     sync.Mutex
	healthProbeScanAt     time.Time
	healthProbeDueIDs     []int64
}

type openAIAdaptiveCandidateScore struct {
	account           *Account
	loadInfo          *AccountLoadInfo
	coreState         adaptiveAccountState
	ttftBucketKey     string
	effectiveCapacity int
	score             float64
	successScore      float64
	costScore         float64
	capacityScore     float64
	latencyScore      float64
	ttftP50           float64
	ttftP90           float64
	ttftWindowSamples int
}

type openAIAdaptiveDiagnosticCandidate struct {
	AccountID                 int64     `json:"account_id"`
	AccountType               string    `json:"account_type"`
	ConfiguredCapacity        int       `json:"configured_capacity"`
	EffectiveCapacity         int       `json:"effective_capacity"`
	CurrentConcurrency        int       `json:"current_concurrency"`
	WaitingCount              int       `json:"waiting_count"`
	Score                     float64   `json:"score"`
	ReliabilityScore          float64   `json:"reliability_score"`
	CostScore                 float64   `json:"cost_score"`
	CapacityScore             float64   `json:"capacity_score"`
	TTFTScore                 float64   `json:"ttft_score"`
	TTFTBucket                string    `json:"ttft_bucket"`
	TTFTP50                   float64   `json:"ttft_p50"`
	TTFTP90                   float64   `json:"ttft_p90"`
	TTFTWindowSamples         int       `json:"ttft_window_samples"`
	LearningStatus            string    `json:"learning_status"`
	HealthSamples             int       `json:"health_samples"`
	SuccessEMA                float64   `json:"success_ema"`
	TTFTEMA                   float64   `json:"ttft_ema"`
	TTFTSamples               int64     `json:"ttft_samples"`
	ConsecutiveFailure        int       `json:"consecutive_failure"`
	HighError                 bool      `json:"high_error"`
	CircuitStatus             string    `json:"circuit_status"`
	CircuitOpenUntil          time.Time `json:"circuit_open_until"`
	CircuitOpenCount          int       `json:"circuit_open_count"`
	CapacityGeneration        uint64    `json:"capacity_generation"`
	CapacityCooldownUntil     time.Time `json:"capacity_cooldown_until"`
	CapacityRecoverySuccesses int       `json:"capacity_recovery_successes"`
	QuotaLimited              bool      `json:"quota_limited"`
	QuotaResetAt              time.Time `json:"quota_reset_at"`
	QuotaNextProbeAt          time.Time `json:"quota_next_probe_at"`
	LastDispatchAt            time.Time `json:"last_dispatch_at"`
	LastProbeAt               time.Time `json:"last_probe_at"`
	RecoveryStatus            string    `json:"recovery_status"`
	RecoverySuccesses         int       `json:"recovery_successes"`
	RecoveryProbeInFlight     bool      `json:"recovery_probe_in_flight"`
}

type openAIAdaptiveSelectionPlan struct {
	selectionOrder []openAIAdaptiveCandidateScore
	candidateCount int
	topK           int
	loadSkew       float64
	loadReq        []AccountWithConcurrency
	filtered       []*Account
	candidates     []openAIAdaptiveCandidateScore
	filterStats    openAISelectionFilterStats
}

type openAIAdaptiveSelectionPlanLoader func() (openAIAdaptiveSelectionPlan, error)

func memoizeOpenAIAdaptiveSelectionPlanLoader(load openAIAdaptiveSelectionPlanLoader) openAIAdaptiveSelectionPlanLoader {
	var plan openAIAdaptiveSelectionPlan
	var err error
	loaded := false
	return func() (openAIAdaptiveSelectionPlan, error) {
		if !loaded {
			loaded = true
			if load != nil {
				plan, err = load()
			}
		}
		return plan, err
	}
}

// Attempt statistics are kept separate from initial filter statistics because
// an account may be retried across cached-load, fresh-load, and wait-plan passes.
type openAIAdaptiveSelectionAttemptStats struct {
	reasons map[string]int
}

func (s *openAIAdaptiveSelectionAttemptStats) record(reason string) {
	if s == nil || reason == "" {
		return
	}
	if s.reasons == nil {
		s.reasons = make(map[string]int, 4)
	}
	s.reasons[reason]++
}

func (s *openAIAdaptiveSelectionAttemptStats) merge(prefix string, stats openAISelectionFilterStats) {
	if s == nil {
		return
	}
	for reason, count := range stats.reasons {
		if count <= 0 {
			continue
		}
		if s.reasons == nil {
			s.reasons = make(map[string]int, 4)
		}
		s.reasons[prefix+reason] += count
	}
}

func (s openAIAdaptiveSelectionAttemptStats) summary(outcome string) string {
	parts := make([]string, 0, len(s.reasons))
	for reason, count := range s.reasons {
		parts = append(parts, fmt.Sprintf("%s=%d", reason, count))
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return outcome
	}
	details := "attempts: " + strings.Join(parts, " ")
	if outcome != "" {
		details += ", " + outcome
	}
	return details
}

func newAdaptiveOpenAIAccountScheduler(service *OpenAIGatewayService, stats *openAIAccountRuntimeStats) OpenAIAccountScheduler {
	if stats == nil {
		stats = newOpenAIAccountRuntimeStats()
	}
	baseline := &defaultOpenAIAccountScheduler{
		service: service,
		stats:   stats,
	}
	core := newAdaptiveStateStore()
	if service != nil && service.openaiAdaptiveCore != nil {
		core = service.openaiAdaptiveCore
	}
	return &adaptiveOpenAIAccountScheduler{
		service:  service,
		baseline: baseline,
		core:     core,
	}
}

func openAIAdaptiveCoreSettings(settings OpenAIAdaptiveSchedulerSettings) adaptiveCoreSettings {
	core := defaultAdaptiveCoreSettings()
	core.Mode = normalizeOpenAIAdaptiveSchedulerMode(settings.OpenAIAdaptiveSchedulerMode)
	core.TopK = settings.OpenAIAdaptiveSchedulerTopK
	core.SoftmaxTemperature = settings.OpenAIAdaptiveSchedulerSoftmaxTemperature
	core.ExplorationRate = settings.OpenAIAdaptiveSchedulerExplorationRate
	core.LearningWindow = time.Duration(settings.OpenAIAdaptiveSchedulerLearningWindowSeconds) * time.Second
	core.SuccessEMAAlpha = settings.OpenAIAdaptiveSchedulerSuccessEMAAlpha
	core.TTFTEMAAlpha = settings.OpenAIAdaptiveSchedulerTTFTEMAAlpha
	core.ConsecutiveFailurePenalty = settings.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty
	core.HealthFailureThreshold = settings.OpenAIAdaptiveSchedulerHealthFailureThreshold
	core.LearningMinHealthSamples = settings.OpenAIAdaptiveSchedulerLearningMinHealthSamples
	core.CircuitCooldownInitial = time.Duration(settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds) * time.Second
	core.CircuitCooldownMaximum = time.Duration(settings.OpenAIAdaptiveSchedulerCooldownMaxSeconds) * time.Second
	core.HighErrorMinSamples = settings.OpenAIAdaptiveSchedulerHighErrorMinSamples
	core.HighErrorMaxSamples = settings.OpenAIAdaptiveSchedulerHighErrorMaxSamples
	core.HighErrorEnterRate = settings.OpenAIAdaptiveSchedulerHighErrorEnterRate
	core.HighErrorExitRate = settings.OpenAIAdaptiveSchedulerHighErrorExitRate
	core.CapacityShrinkFactor = settings.OpenAIAdaptiveSchedulerShrinkFactorSoft
	core.CapacityRecoveryFactor = settings.OpenAIAdaptiveSchedulerCapacityGrowthFactor
	core.CapacityRecoverySamples = settings.OpenAIAdaptiveSchedulerCapacityRecoverySamples
	core.CapacityCooldown = time.Duration(settings.OpenAIAdaptiveSchedulerCooldownBaseSeconds) * time.Second
	core.QuotaProbeInterval = time.Duration(settings.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds) * time.Second
	core.RecoveryExplorationRate = settings.OpenAIAdaptiveSchedulerRecoveryExplorationRate
	core.RecoveryMaxConcurrency = settings.OpenAIAdaptiveSchedulerRecoveryMaxConcurrency
	core.RecoveryWarmupSuccesses = settings.OpenAIAdaptiveSchedulerRecoveryWarmupSuccesses
	core.WeightReliability = settings.OpenAIAdaptiveSchedulerWeightSuccess
	core.WeightCapacity = settings.OpenAIAdaptiveSchedulerWeightCapacity
	core.WeightTTFT = settings.OpenAIAdaptiveSchedulerWeightLatency
	core.WeightCost = settings.OpenAIAdaptiveSchedulerWeightCost
	return normalizeAdaptiveCoreSettings(core)
}

func (s *adaptiveOpenAIAccountScheduler) Select(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	cfg := s.service.openAIAdaptiveSchedulerSettings(ctx)
	if !cfg.OpenAIAdaptiveSchedulerEnabled {
		return s.baseline.Select(ctx, req)
	}

	if cfg.OpenAIAdaptiveSchedulerMode != openAIAdaptiveSchedulerModeEnforce {
		selection, decision, err := s.selectCurrentBaseline(ctx, req)
		if selection != nil && selection.Account != nil {
			s.registerAdaptiveAdmission(ctx, selection.Account, selection.Acquired, s.ttftBucketKey(req, selection.Account), openAIAdaptiveCoreSettings(cfg))
		}
		s.logShadowDecision(ctx, req, cfg, selection)
		return selection, decision, err
	}

	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerAdaptive}
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		s.metrics.recordSelect(decision)
	}()

	if selection, ok, err := s.selectByPreviousResponse(ctx, req, cfg, &decision); err != nil || ok {
		outcome := "previous_response"
		if err != nil {
			outcome = "previous_response_error"
		} else if selection == nil || selection.Account == nil {
			outcome = "previous_response_empty"
		}
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, nil, outcome, err, start)
		return selection, decision, err
	}
	selection, escapedSticky, err := s.selectByAdaptiveSticky(ctx, req, cfg)
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.Layer = openAIAccountScheduleLayerSessionSticky
		decision.StickySessionHit = true
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, nil, "session_sticky", nil, start)
		return selection, decision, nil
	}
	if escapedSticky {
		req.PreserveStickyBinding = true
	}
	loadSharedPlan := memoizeOpenAIAdaptiveSelectionPlanLoader(func() (openAIAdaptiveSelectionPlan, error) {
		return s.buildAdaptiveSelectionOrderWithLoad(ctx, req, cfg, true)
	})

	probeSelection, probeCandidates, probeErr := s.selectDueHealthProbeWithPlanLoader(ctx, req, cfg, loadSharedPlan)
	if probeErr != nil {
		slog.Warn("openai_adaptive_runtime_probe_selection_failed",
			"model", req.RequestedModel,
			"error", probeErr,
		)
	} else if probeSelection != nil && probeSelection.Account != nil {
		decision.Layer = openAIAccountScheduleLayerAdaptive
		decision.CandidateCount = len(probeCandidates)
		decision.TopK = len(probeCandidates)
		decision.SelectedAccountID = probeSelection.Account.ID
		decision.SelectedAccountType = probeSelection.Account.Type
		diagnostics := openAIAdaptiveDiagnosticCandidatesForLog(ctx, req, cfg, probeCandidates)
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, probeSelection, diagnostics, "runtime_probe", nil, start)
		return probeSelection, decision, nil
	}

	recoverySelection, recoveryCandidates, recoveryErr := s.selectRecoveryExplorationWithPlanLoader(ctx, req, cfg, loadSharedPlan)
	if recoveryErr != nil {
		slog.Warn("openai_adaptive_recovery_probe_selection_failed",
			"model", req.RequestedModel,
			"error", recoveryErr,
		)
	} else if recoverySelection != nil && recoverySelection.Account != nil {
		decision.Layer = openAIAccountScheduleLayerAdaptive
		decision.CandidateCount = len(recoveryCandidates)
		decision.TopK = len(recoveryCandidates)
		decision.SelectedAccountID = recoverySelection.Account.ID
		decision.SelectedAccountType = recoverySelection.Account.Type
		diagnostics := openAIAdaptiveDiagnosticCandidatesForLog(ctx, req, cfg, recoveryCandidates)
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, recoverySelection, diagnostics, "recovery_probe", nil, start)
		return recoverySelection, decision, nil
	}

	selection, candidateCount, topK, loadSkew, diagnosticCandidates, err := s.selectByAdaptiveLoadBalanceWithPlanLoader(ctx, req, cfg, loadSharedPlan)
	decision.Layer = openAIAccountScheduleLayerAdaptive
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	if err != nil {
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, nil, diagnosticCandidates, "fallback", err, start)
		s.logDiagnosticResult(ctx, cfg, OpenAIAccountScheduleReport{
			AccountID:      0,
			Success:        false,
			HealthSample:   false,
			TerminalReason: "adaptive_selection_fallback",
			Err:            err,
		})
		slog.Warn("openai_adaptive_scheduler_fallback",
			"reason", "adaptive_select_error",
			"error", err,
			"model", req.RequestedModel,
		)
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, diagnosticCandidates, "selected", nil, start)
		return selection, decision, nil
	}
	s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, diagnosticCandidates, "empty_selection", nil, start)
	return selection, decision, nil
}

func (s *adaptiveOpenAIAccountScheduler) selectDueHealthProbe(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) (*AccountSelectionResult, []openAIAdaptiveCandidateScore, error) {
	return s.selectDueHealthProbeWithPlanLoader(ctx, req, cfg, func() (openAIAdaptiveSelectionPlan, error) {
		return s.buildAdaptiveSelectionOrderWithLoad(ctx, req, cfg, true)
	})
}

func (s *adaptiveOpenAIAccountScheduler) selectDueHealthProbeWithPlanLoader(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadPlan openAIAdaptiveSelectionPlanLoader,
) (*AccountSelectionResult, []openAIAdaptiveCandidateScore, error) {
	if s == nil || s.core == nil || strings.TrimSpace(req.PreviousResponseID) != "" || req.GuardianParentAccountID > 0 || req.StickyAccountID > 0 || req.StickyPreviousAccountID > 0 {
		return nil, nil, nil
	}
	coreSettings := openAIAdaptiveCoreSettings(cfg)
	dueIDs := s.cachedDueHealthProbeAccountIDs(time.Now(), coreSettings)
	if len(dueIDs) == 0 {
		return nil, nil, nil
	}

	plan, err := loadPlan()
	if err != nil {
		return nil, nil, nil
	}
	probeNow := time.Now()
	activeLayer := openAIAdaptiveActiveCredentialLayer(plan.candidates)
	byID := make(map[int64]openAIAdaptiveCandidateScore, len(activeLayer))
	for _, candidate := range activeLayer {
		if candidate.account != nil && ((!candidate.coreState.CircuitOpenUntil.IsZero() &&
			!candidate.coreState.CircuitOpenUntil.After(probeNow) && !candidate.coreState.HealthProbeInFlight) ||
			(candidate.coreState.QuotaLimited && adaptiveQuotaProbeDue(&candidate.coreState, probeNow) && !candidate.coreState.QuotaProbeInFlight)) {
			byID[candidate.account.ID] = candidate
		}
	}
	dueCandidates := make([]openAIAdaptiveCandidateScore, 0, len(dueIDs))
	for _, accountID := range dueIDs {
		if candidate, ok := byID[accountID]; ok {
			dueCandidates = append(dueCandidates, candidate)
		}
	}
	if len(dueCandidates) == 0 {
		return nil, nil, nil
	}

	probeReq := req
	probeReq.PreserveStickyBinding = true
	attemptStats := openAIAdaptiveSelectionAttemptStats{}
	selection, _, err := s.tryAcquireAdaptiveSelectionOrder(ctx, probeReq, cfg, dueCandidates, false, &attemptStats)
	return selection, dueCandidates, err
}

func (s *adaptiveOpenAIAccountScheduler) selectRecoveryExplorationWithPlanLoader(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadPlan openAIAdaptiveSelectionPlanLoader,
) (*AccountSelectionResult, []openAIAdaptiveCandidateScore, error) {
	if s == nil || s.core == nil || strings.TrimSpace(req.PreviousResponseID) != "" || req.GuardianParentAccountID > 0 || req.StickyAccountID > 0 || req.StickyPreviousAccountID > 0 {
		return nil, nil, nil
	}
	coreSettings := openAIAdaptiveCoreSettings(cfg)
	if coreSettings.RecoveryExplorationRate <= 0 || rand.Float64() >= coreSettings.RecoveryExplorationRate {
		return nil, nil, nil
	}
	plan, err := loadPlan()
	if err != nil {
		return nil, nil, nil
	}
	candidates := openAIAdaptiveRecoveryCandidates(plan.candidates, time.Now(), coreSettings)
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	probeReq := req
	probeReq.PreserveStickyBinding = true
	attemptStats := openAIAdaptiveSelectionAttemptStats{}
	selection, _, err := s.tryAcquireAdaptiveSelectionOrder(ctx, probeReq, cfg, candidates, true, &attemptStats)
	return selection, candidates, err
}

func openAIAdaptiveActiveCredentialLayer(candidates []openAIAdaptiveCandidateScore) []openAIAdaptiveCandidateScore {
	hasOAuth := false
	for _, candidate := range candidates {
		if candidate.account != nil && candidate.account.IsOAuth() {
			hasOAuth = true
			break
		}
	}
	active := make([]openAIAdaptiveCandidateScore, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.account == nil || candidate.account.IsOAuth() != hasOAuth {
			continue
		}
		active = append(active, candidate)
	}
	return active
}

func openAIAdaptiveRecoveryCandidates(candidates []openAIAdaptiveCandidateScore, now time.Time, settings adaptiveCoreSettings) []openAIAdaptiveCandidateScore {
	active := openAIAdaptiveActiveCredentialLayer(candidates)
	eligible := make([]openAIAdaptiveCandidateScore, 0, len(active))
	for _, candidate := range active {
		state := candidate.coreState
		if state.RecoveryProbeInFlight || !state.CircuitOpenUntil.IsZero() || state.QuotaLimited {
			continue
		}
		stale := state.LastDispatchAt.IsZero() || now.Sub(state.LastDispatchAt) >= settings.LearningWindow
		if !stale && state.RecoveryStatus != adaptiveRecoveryStale && state.RecoveryStatus != adaptiveRecoveryWarming {
			continue
		}
		eligible = append(eligible, candidate)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left, right := eligible[i], eligible[j]
		if left.coreState.LastProbeAt.IsZero() != right.coreState.LastProbeAt.IsZero() {
			return left.coreState.LastProbeAt.IsZero()
		}
		if !left.coreState.LastProbeAt.Equal(right.coreState.LastProbeAt) {
			return left.coreState.LastProbeAt.Before(right.coreState.LastProbeAt)
		}
		leftCost, rightCost := left.account.BillingRateMultiplier(), right.account.BillingRateMultiplier()
		if leftCost != rightCost {
			return leftCost < rightCost
		}
		return left.account.ID < right.account.ID
	})
	return eligible
}

func (s *adaptiveOpenAIAccountScheduler) cachedDueHealthProbeAccountIDs(now time.Time, settings adaptiveCoreSettings) []int64 {
	s.healthProbeScanMu.Lock()
	defer s.healthProbeScanMu.Unlock()
	if !s.healthProbeScanAt.IsZero() && now.Sub(s.healthProbeScanAt) < openAIAdaptiveHealthProbeScanInterval {
		return append([]int64(nil), s.healthProbeDueIDs...)
	}
	s.healthProbeScanAt = now
	s.healthProbeDueIDs = s.core.dueHealthProbeAccountIDs(now, settings)
	return append([]int64(nil), s.healthProbeDueIDs...)
}

func (s *adaptiveOpenAIAccountScheduler) selectCurrentBaseline(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	if s.service.isOpenAIAdvancedSchedulerEnabled(ctx) {
		return s.baseline.Select(ctx, req)
	}
	return s.selectLegacyLoadAware(ctx, req)
}

// registerAdaptiveAdmission records the load observed immediately after a
// successful slot acquisition. Recovery decisions can then distinguish real
// admitted traffic from a cached pre-selection snapshot.
func (s *adaptiveOpenAIAccountScheduler) registerAdaptiveAdmission(ctx context.Context, account *Account, admitted bool, ttftBucketKey string, settings adaptiveCoreSettings) {
	if account == nil {
		return
	}
	s.registerAdaptiveAdmissionWithRequest(ctx, account, openAIAdaptiveRequestID(ctx), admitted, ttftBucketKey, settings)
}

func (s *adaptiveOpenAIAccountScheduler) registerAdaptiveAdmissionWithRequest(ctx context.Context, account *Account, requestID string, admitted bool, ttftBucketKey string, settings adaptiveCoreSettings) {
	s.registerAdaptiveAdmissionWithRequestAndLoad(ctx, account, requestID, admitted, ttftBucketKey, settings, nil)
}

func (s *adaptiveOpenAIAccountScheduler) registerAdaptiveAdmissionWithRequestAndLoad(ctx context.Context, account *Account, requestID string, admitted bool, ttftBucketKey string, settings adaptiveCoreSettings, loadInfo *AccountLoadInfo) {
	if s == nil || s.core == nil || account == nil {
		return
	}
	observed, waiting := -1, 0
	if admitted && loadInfo != nil {
		observed = max(0, loadInfo.CurrentConcurrency)
		if account.Concurrency > 0 {
			observed++
		}
		waiting = max(0, loadInfo.WaitingCount)
	} else if admitted && s.service != nil && s.service.concurrencyService != nil {
		loads, err := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, []AccountWithConcurrency{{ID: account.ID, MaxConcurrency: account.Concurrency}})
		if err == nil {
			if load := loads[account.ID]; load != nil {
				observed = load.CurrentConcurrency
				waiting = load.WaitingCount
			}
		}
	}
	if admitted {
		s.core.registerAdmissionWithLoadAndTTFTBucket(account.ID, requestID, account.Concurrency, observed, waiting, true, ttftBucketKey, time.Now(), settings)
	} else {
		s.core.registerAdmissionWithLoadAndTTFTBucket(account.ID, requestID, account.Concurrency, -1, 0, false, ttftBucketKey, time.Now(), settings)
	}
}

func (s *adaptiveOpenAIAccountScheduler) selectLegacyLoadAware(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerLoadBalance}
	if req.RequiredTransport == OpenAIUpstreamTransportAny || req.RequiredTransport == OpenAIUpstreamTransportHTTPSSE {
		effectiveExcludedIDs := cloneExcludedAccountIDs(req.ExcludedIDs)
		for {
			selection, err := s.service.selectAccountWithLoadAwareness(ctx, req.GroupID, req.Platform, req.SessionHash, req.RequestedModel, effectiveExcludedIDs, req.RequireCompact, req.RequiredCapability, req.UseUpstreamTokenCost)
			if err != nil {
				return nil, decision, err
			}
			if selection == nil || selection.Account == nil {
				return selection, decision, nil
			}
			if s.service.accountSupportsOpenAIRequestCapabilities(selection.Account, req.RequiredCapability, req.RequiredImageCapability, req.RequireImageStream) {
				decision.SelectedAccountID = selection.Account.ID
				decision.SelectedAccountType = selection.Account.Type
				return selection, decision, nil
			}
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if effectiveExcludedIDs == nil {
				effectiveExcludedIDs = make(map[int64]struct{})
			}
			if _, exists := effectiveExcludedIDs[selection.Account.ID]; exists {
				return nil, decision, ErrNoAvailableAccounts
			}
			effectiveExcludedIDs[selection.Account.ID] = struct{}{}
		}
	}

	effectiveExcludedIDs := cloneExcludedAccountIDs(req.ExcludedIDs)
	for {
		selection, err := s.service.selectAccountWithLoadAwareness(ctx, req.GroupID, req.Platform, req.SessionHash, req.RequestedModel, effectiveExcludedIDs, req.RequireCompact, req.RequiredCapability, req.UseUpstreamTokenCost)
		if err != nil {
			return nil, decision, err
		}
		if selection == nil || selection.Account == nil {
			return selection, decision, nil
		}
		if s.service.isOpenAIAccountTransportCompatible(selection.Account, req.RequiredTransport) &&
			s.service.accountSupportsOpenAIRequestCapabilities(selection.Account, req.RequiredCapability, req.RequiredImageCapability, req.RequireImageStream) {
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			return selection, decision, nil
		}
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if effectiveExcludedIDs == nil {
			effectiveExcludedIDs = make(map[int64]struct{})
		}
		if _, exists := effectiveExcludedIDs[selection.Account.ID]; exists {
			return nil, decision, ErrNoAvailableAccounts
		}
		effectiveExcludedIDs[selection.Account.ID] = struct{}{}
	}
}

func (s *adaptiveOpenAIAccountScheduler) selectByPreviousResponse(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	decision *OpenAIAccountScheduleDecision,
) (*AccountSelectionResult, bool, error) {
	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID == "" || NormalizeOpenAICompatiblePlatform(req.Platform) != PlatformOpenAI {
		return nil, false, nil
	}
	selection, err := s.service.selectAccountByPreviousResponseIDForCapability(
		ctx,
		req.GroupID,
		previousResponseID,
		req.RequestedModel,
		req.ExcludedIDs,
		req.RequiredCapability,
		req.RequireCompact,
	)
	if err != nil {
		return nil, true, err
	}
	if selection != nil && selection.Account != nil {
		now := time.Now()
		_, _, circuitTracked, quotaTracked := s.core.runtimeStatus(selection.Account.ID, selection.Account.Concurrency, now, openAIAdaptiveCoreSettings(cfg))
		if circuitTracked || quotaTracked {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			return nil, false, nil
		}
		if s.service.isOpenAIAccountRuntimeBlocked(selection.Account) ||
			!s.baseline.isAccountTransportCompatible(selection.Account, req.RequiredTransport) ||
			!s.baseline.isAccountRequestCompatible(ctx, selection.Account, req) {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			return nil, false, nil
		}
		decision.Layer = openAIAccountScheduleLayerPreviousResponse
		decision.StickyPreviousHit = true
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		s.registerAdaptiveAdmission(ctx, selection.Account, selection.Acquired, s.ttftBucketKey(req, selection.Account), openAIAdaptiveCoreSettings(cfg))
		if req.SessionHash != "" {
			_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, selection.Account.ID)
		}
		return selection, true, nil
	}
	return nil, false, nil
}

func (s *adaptiveOpenAIAccountScheduler) selectByAdaptiveSticky(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) (*AccountSelectionResult, bool, error) {
	sessionHash := strings.TrimSpace(req.SessionHash)
	if sessionHash == "" || s == nil || s.service == nil || s.service.cache == nil {
		return nil, false, nil
	}
	accountID := req.StickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.service.getStickySessionAccountID(ctx, req.GroupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil, false, nil
		}
	}
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[accountID]; excluded {
			return nil, false, nil
		}
	}
	account, err := s.service.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	if shouldClearStickySession(account, req.RequestedModel) ||
		account.Platform != NormalizeOpenAICompatiblePlatform(req.Platform) ||
		!account.IsOpenAICompatible() ||
		!account.IsSchedulable() ||
		s.service.isOpenAIAccountRuntimeBlocked(account) ||
		!s.baseline.isAccountRequestCompatible(ctx, account, req) ||
		!s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.GroupID, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
	if account == nil || !openAIStickyAccountMatchesGroup(account, req.GroupID) ||
		s.service.isOpenAIAccountRuntimeBlocked(account) ||
		!s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) ||
		!s.baseline.isAccountRequestCompatible(ctx, account, req) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	coreSettings := openAIAdaptiveCoreSettings(cfg)
	now := time.Now()
	effectiveCapacity, _, circuitTracked, quotaTracked := s.core.runtimeStatus(account.ID, account.Concurrency, now, coreSettings)
	if circuitTracked || quotaTracked {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	loadInfo := &AccountLoadInfo{AccountID: account.ID}
	if s.service.concurrencyService != nil {
		if loadMap, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, []AccountWithConcurrency{{
			ID:             account.ID,
			MaxConcurrency: effectiveCapacity,
		}}); loadErr == nil && loadMap != nil {
			if info := loadMap[account.ID]; info != nil {
				loadInfo = info
			}
		}
	}
	effectiveCapacity, _, _, _ = s.core.runtimeStatusWithLoad(account.ID, account.Concurrency, loadInfo.CurrentConcurrency, now, coreSettings)
	if effectiveCapacity > 0 && loadInfo.CurrentConcurrency >= effectiveCapacity {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, account.ID, effectiveCapacity)
	if acquireErr != nil {
		return nil, false, acquireErr
	}
	if result != nil && result.Acquired {
		s.registerAdaptiveAdmissionWithRequestAndLoad(ctx, account, openAIAdaptiveRequestID(ctx), true, s.ttftBucketKey(req, account), coreSettings, loadInfo)
		_ = s.service.refreshStickySessionTTL(ctx, req.GroupID, sessionHash, s.service.openAIWSSessionStickyTTL())
		selection, selectErr := s.service.newAcquiredSelectionResult(ctx, account, result.ReleaseFunc)
		return selection, false, selectErr
	}
	_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
	return nil, false, nil
}

func (s *adaptiveOpenAIAccountScheduler) selectByAdaptiveLoadBalance(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) (*AccountSelectionResult, int, int, float64, []openAIAdaptiveDiagnosticCandidate, error) {
	return s.selectByAdaptiveLoadBalanceWithPlanLoader(ctx, req, cfg, func() (openAIAdaptiveSelectionPlan, error) {
		return s.buildAdaptiveSelectionOrderWithLoad(ctx, req, cfg, true)
	})
}

func (s *adaptiveOpenAIAccountScheduler) selectByAdaptiveLoadBalanceWithPlanLoader(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadPlan openAIAdaptiveSelectionPlanLoader,
) (*AccountSelectionResult, int, int, float64, []openAIAdaptiveDiagnosticCandidate, error) {
	if s.service.concurrencyService == nil || !s.service.schedulingConfig().LoadBatchEnabled {
		selection, fallbackErr := s.degradedAdaptiveFallback(ctx, req, "runtime_unavailable", errOpenAIAdaptiveSchedulerFallback)
		return selection, 0, 0, 0, nil, fallbackErr
	}
	plan, err := loadPlan()
	diagnosticCandidates := openAIAdaptiveDiagnosticCandidatesForLog(ctx, req, cfg, plan.selectionOrder)
	if err != nil {
		reason := dominantAdaptiveSelectionExclusion(plan.filterStats, nil)
		selection, fallbackErr := s.degradedAdaptiveFallback(ctx, req, reason, err)
		return selection, plan.candidateCount, plan.topK, plan.loadSkew, diagnosticCandidates, fallbackErr
	}
	attemptStats := openAIAdaptiveSelectionAttemptStats{}
	result, compactBlocked, acquireErr := s.tryAcquireAdaptiveSelectionOrder(ctx, req, cfg, plan.selectionOrder, false, &attemptStats)
	if acquireErr != nil {
		return nil, plan.candidateCount, plan.topK, plan.loadSkew, diagnosticCandidates, acquireErr
	}
	if result != nil {
		return result, plan.candidateCount, plan.topK, plan.loadSkew, diagnosticCandidates, nil
	}

	if s.service.concurrencyService != nil {
		if freshLoadMap, loadErr := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, plan.loadReq); loadErr == nil {
			freshFilterStats := openAISelectionFilterStats{}
			freshCandidates, freshSkew := s.buildAdaptiveCandidates(req, cfg, plan.filtered, freshLoadMap, &freshFilterStats)
			attemptStats.merge("fresh_", freshFilterStats)
			freshOrder := buildOpenAIAdaptiveSelectionOrder(freshCandidates, req, cfg)
			freshResult, freshCompactBlocked, freshAcquireErr := s.tryAcquireAdaptiveSelectionOrder(ctx, req, cfg, freshOrder, false, &attemptStats)
			if freshAcquireErr != nil {
				return nil, plan.candidateCount, plan.topK, plan.loadSkew, diagnosticCandidates, freshAcquireErr
			}
			if freshResult != nil {
				freshTopK := cfg.OpenAIAdaptiveSchedulerTopK
				if freshTopK > len(freshCandidates) {
					freshTopK = len(freshCandidates)
				}
				return freshResult, len(freshCandidates), freshTopK, freshSkew, openAIAdaptiveDiagnosticCandidatesForLog(ctx, req, cfg, freshOrder), nil
			}
			compactBlocked = compactBlocked || freshCompactBlocked
		} else {
			attemptStats.record("fresh_load_failed")
		}
	}

	cfgWait := s.service.schedulingConfig()
	for _, candidate := range plan.selectionOrder {
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil {
			attemptStats.record("wait_" + s.adaptiveFreshResolveFailureReason(ctx, candidate.account, req))
			continue
		}
		if compatible, reason := s.isAdaptiveSelectionAccountCompatible(ctx, fresh, req); !compatible {
			attemptStats.record("wait_" + reason)
			continue
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil {
			attemptStats.record("wait_db_recheck_failed")
			continue
		}
		if compatible, reason := s.isAdaptiveSelectionAccountCompatible(ctx, fresh, req); !compatible {
			attemptStats.record("wait_" + reason)
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			attemptStats.record("wait_compact_unsupported")
			continue
		}
		coreSettings := openAIAdaptiveCoreSettings(cfg)
		now := time.Now()
		requestID := openAIAdaptiveRequestID(ctx)
		effectiveCapacity := s.core.effectiveCapacity(fresh.ID, fresh.Concurrency, now, coreSettings)
		if !s.core.claimHealthProbe(fresh.ID, requestID, fresh.Concurrency, now, coreSettings) {
			attemptStats.record("wait_health_circuit")
			continue
		}
		quotaAllowed, quotaClaimed := s.core.claimQuotaProbe(fresh.ID, requestID, fresh.Concurrency, now, coreSettings)
		if !quotaAllowed {
			s.core.releaseHealthProbe(fresh.ID, requestID, time.Now())
			attemptStats.record("wait_quota_limited")
			continue
		}
		var releaseProbeOnce sync.Once
		releaseProbes := func() {
			releaseProbeOnce.Do(func() {
				releasedAt := time.Now()
				s.core.releaseHealthProbe(fresh.ID, requestID, releasedAt)
				if quotaClaimed {
					s.core.releaseQuotaProbe(fresh.ID, requestID, releasedAt)
				}
			})
		}
		stopRelease := context.AfterFunc(ctx, releaseProbes)
		selection, selectErr := s.service.newSelectionResult(ctx, fresh, false, nil, &AccountWaitPlan{
			AccountID:      fresh.ID,
			MaxConcurrency: effectiveCapacity,
			Timeout:        cfgWait.FallbackWaitTimeout,
			MaxWaiting:     cfgWait.FallbackMaxWaiting,
		})
		if selectErr != nil || selection == nil {
			_ = stopRelease()
			releaseProbes()
		} else {
			s.registerAdaptiveAdmissionWithRequest(ctx, fresh, requestID, false, candidate.ttftBucketKey, coreSettings)
		}
		return selection, plan.candidateCount, plan.topK, plan.loadSkew, diagnosticCandidates, selectErr
	}

	originalErr := noAvailableOpenAISelectionError(
		req.RequestedModel,
		compactBlocked,
		plan.filterStats.summary(attemptStats.summary("selection_order_exhausted")),
	)
	selection, fallbackErr := s.degradedAdaptiveFallback(ctx, req, dominantAdaptiveSelectionExclusion(plan.filterStats, attemptStats.reasons), originalErr)
	return selection, plan.candidateCount, plan.topK, plan.loadSkew, diagnosticCandidates, fallbackErr
}

func (s *adaptiveOpenAIAccountScheduler) degradedAdaptiveFallback(ctx context.Context, req OpenAIAccountScheduleRequest, reason string, originalErr error) (*AccountSelectionResult, error) {
	if s == nil || s.baseline == nil {
		return nil, originalErr
	}
	selection, _, fallbackErr := s.selectCurrentBaseline(ctx, req)
	if fallbackErr == nil && selection != nil && selection.Account != nil {
		slog.Warn("openai_adaptive_scheduler_degraded_fallback",
			"reason", reason,
			"model", req.RequestedModel,
			"account_id", selection.Account.ID,
			"fallback_succeeded", true,
			"original_error", errorStringOrEmpty(originalErr),
		)
		return selection, nil
	}
	slog.Warn("openai_adaptive_scheduler_degraded_fallback",
		"reason", reason,
		"model", req.RequestedModel,
		"fallback_succeeded", false,
		"original_error", errorStringOrEmpty(originalErr),
		"fallback_error", errorStringOrEmpty(fallbackErr),
	)
	return nil, originalErr
}

func dominantAdaptiveSelectionExclusion(stats openAISelectionFilterStats, attempts map[string]int) string {
	counts := make(map[string]int, len(stats.reasons)+len(attempts))
	for reason, count := range stats.reasons {
		counts[reason] += count
	}
	for reason, count := range attempts {
		if strings.HasPrefix(reason, "fresh_") || strings.HasPrefix(reason, "wait_") {
			reason = strings.TrimPrefix(strings.TrimPrefix(reason, "fresh_"), "wait_")
		}
		counts[reason] += count
	}
	best := "runtime_unavailable"
	bestCount := 0
	for reason, count := range counts {
		if count > bestCount || (count == bestCount && reason < best) {
			best, bestCount = reason, count
		}
	}
	return best
}

func errorStringOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *adaptiveOpenAIAccountScheduler) buildAdaptiveSelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) ([]openAIAdaptiveCandidateScore, int, int, error) {
	plan, err := s.buildAdaptiveSelectionOrderWithLoad(ctx, req, cfg, false)
	return plan.selectionOrder, plan.candidateCount, plan.topK, err
}

func (s *adaptiveOpenAIAccountScheduler) buildAdaptiveSelectionOrderWithLoad(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	_ bool,
) (openAIAdaptiveSelectionPlan, error) {
	plan := openAIAdaptiveSelectionPlan{}
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return plan, err
	}
	plan.filterStats.pool = len(accounts)
	if len(accounts) == 0 {
		return plan, noAvailableOpenAISelectionError(req.RequestedModel, false, plan.filterStats.summary(""))
	}

	plan.filtered = make([]*Account, 0, len(accounts))
	plan.loadReq = make([]AccountWithConcurrency, 0, len(accounts))
	coreSettings := openAIAdaptiveCoreSettings(cfg)
	now := time.Now()
	needsUpstreamCheck := s.service.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID)
	parentLookup := s.baseline.parentAccountLookupForSelection(ctx, accounts)
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				plan.filterStats.exclude("excluded")
				continue
			}
		}
		if !account.IsSchedulable() {
			plan.filterStats.exclude("not_schedulable")
			continue
		}
		if account.Platform != NormalizeOpenAICompatiblePlatform(req.Platform) || !account.IsOpenAICompatible() {
			plan.filterStats.exclude("platform_mismatch")
			continue
		}
		if s.service.isOpenAIAccountRuntimeBlocked(account) {
			plan.filterStats.exclude("runtime_blocked")
			continue
		}
		if req.RequirePrivacySet && !account.IsPrivacySet() {
			plan.filterStats.exclude("privacy_not_set")
			continue
		}
		if compatible, reason := s.baseline.isAccountRequestCompatibleReasonWithSelectionChecks(ctx, account, req, needsUpstreamCheck, parentLookup); !compatible {
			plan.filterStats.exclude(reason)
			continue
		}
		if !s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) {
			plan.filterStats.exclude("transport_incompatible")
			continue
		}
		effectiveCapacity, allowed, _, _ := s.core.runtimeStatus(account.ID, account.Concurrency, now, coreSettings)
		if !allowed {
			plan.filterStats.exclude("runtime_unavailable")
			continue
		}
		plan.filtered = append(plan.filtered, account)
		plan.loadReq = append(plan.loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: effectiveCapacity,
		})
	}
	if len(plan.filtered) == 0 {
		return plan, noAvailableOpenAISelectionError(req.RequestedModel, false, plan.filterStats.summary(""))
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, plan.loadReq); loadErr == nil {
			loadMap = batchLoad
		}
	}
	candidates, loadSkew := s.buildAdaptiveCandidates(req, cfg, plan.filtered, loadMap, &plan.filterStats)
	plan.loadSkew = loadSkew
	if req.RequireCompact && len(candidates) == 0 {
		return plan, ErrNoAvailableCompactAccounts
	}
	if len(candidates) == 0 {
		return plan, noAvailableOpenAISelectionError(req.RequestedModel, false, plan.filterStats.summary("selection_order_empty"))
	}
	plan.candidateCount = len(candidates)
	plan.candidates = candidates
	plan.topK = cfg.OpenAIAdaptiveSchedulerTopK
	if plan.topK > len(candidates) {
		plan.topK = len(candidates)
	}
	plan.selectionOrder = buildOpenAIAdaptiveSelectionOrder(candidates, req, cfg)
	if len(plan.selectionOrder) == 0 {
		return plan, noAvailableOpenAISelectionError(req.RequestedModel, false, plan.filterStats.summary("selection_order_empty"))
	}
	return plan, nil
}

func (s *adaptiveOpenAIAccountScheduler) buildAdaptiveCandidates(
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	filtered []*Account,
	loadMap map[int64]*AccountLoadInfo,
	filterStats *openAISelectionFilterStats,
) ([]openAIAdaptiveCandidateScore, float64) {
	candidates := make([]openAIAdaptiveCandidateScore, 0, len(filtered))
	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	coreSettings := openAIAdaptiveCoreSettings(cfg)
	now := time.Now()
	for _, account := range filtered {
		if req.RequireCompact && openAICompactSupportTier(account) == 0 {
			if filterStats != nil {
				filterStats.exclude("compact_unsupported")
			}
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		ttftBucketKey := s.ttftBucketKey(req, account)
		coreState, allowed := s.core.schedulingSnapshot(account.ID, account.Concurrency, loadInfo.CurrentConcurrency, ttftBucketKey, now, coreSettings)
		if !allowed {
			if filterStats != nil {
				filterStats.exclude("runtime_unavailable")
			}
			continue
		}
		effectiveCapacity := coreState.EffectiveCapacity
		if effectiveCapacity > 0 && loadInfo.CurrentConcurrency >= effectiveCapacity {
			if filterStats != nil {
				filterStats.exclude("capacity_full")
			}
			continue
		}
		loadRate := adaptiveLoadRate(loadInfo, effectiveCapacity)
		loadRateSum += loadRate
		loadRateSumSquares += loadRate * loadRate
		candidates = append(candidates, openAIAdaptiveCandidateScore{
			account:           account,
			loadInfo:          loadInfo,
			coreState:         coreState,
			ttftBucketKey:     ttftBucketKey,
			effectiveCapacity: effectiveCapacity,
		})
	}
	if len(candidates) == 0 {
		return nil, 0
	}
	applyOpenAIAdaptiveScores(candidates, cfg)
	return candidates, calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, len(candidates))
}

func applyOpenAIAdaptiveScores(candidates []openAIAdaptiveCandidateScore, cfg OpenAIAdaptiveSchedulerSettings) {
	inputs := make([]adaptiveScoreCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		inputs = append(inputs, adaptiveScoreCandidate{
			AccountID:          candidate.account.ID,
			OAuth:              candidate.account.IsOAuth(),
			Cost:               candidate.account.BillingRateMultiplier(),
			CurrentConcurrency: candidate.loadInfo.CurrentConcurrency,
			State:              candidate.coreState,
			TTFTBucketKey:      candidate.ttftBucketKey,
		})
	}
	scored := scoreAdaptiveCandidates(inputs, time.Now(), openAIAdaptiveCoreSettings(cfg))
	byID := make(map[int64]adaptiveScoreCandidate, len(scored))
	for _, candidate := range scored {
		byID[candidate.AccountID] = candidate
	}
	for i := range candidates {
		score := byID[candidates[i].account.ID]
		candidates[i].score = score.Score
		candidates[i].successScore = score.ReliabilityScore
		candidates[i].costScore = score.CostScore
		candidates[i].capacityScore = score.CapacityScore
		candidates[i].latencyScore = score.TTFTScore
		candidates[i].ttftP50 = score.TTFTP50
		candidates[i].ttftP90 = score.TTFTP90
		candidates[i].ttftWindowSamples = score.TTFTWindowSamples
	}
}

func buildOpenAIAdaptiveSelectionOrder(
	candidates []openAIAdaptiveCandidateScore,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) []openAIAdaptiveCandidateScore {
	inputs := make([]adaptiveScoreCandidate, 0, len(candidates))
	byID := make(map[int64]openAIAdaptiveCandidateScore, len(candidates))
	for _, candidate := range candidates {
		inputs = append(inputs, adaptiveScoreCandidate{AccountID: candidate.account.ID, OAuth: candidate.account.IsOAuth(), Score: candidate.score, HealthSamples: len(candidate.coreState.HealthObservations)})
		byID[candidate.account.ID] = candidate
	}
	newSession := strings.TrimSpace(req.PreviousResponseID) == "" && req.StickyAccountID <= 0
	ordered := orderAdaptiveCandidates(inputs, newSession, cfg.OpenAIAdaptiveSchedulerMode == openAIAdaptiveSchedulerModeShadow, time.Now(), openAIAdaptiveCoreSettings(cfg))
	result := make([]openAIAdaptiveCandidateScore, 0, len(ordered))
	for _, candidate := range ordered {
		result = append(result, byID[candidate.AccountID])
	}
	return result
}

func (s *adaptiveOpenAIAccountScheduler) tryAcquireAdaptiveSelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	selectionOrder []openAIAdaptiveCandidateScore,
	recoveryProbe bool,
	attemptStats *openAIAdaptiveSelectionAttemptStats,
) (*AccountSelectionResult, bool, error) {
	compactBlocked := false
	for _, candidate := range selectionOrder {
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil {
			attemptStats.record(s.adaptiveFreshResolveFailureReason(ctx, candidate.account, req))
			continue
		}
		if compatible, reason := s.isAdaptiveSelectionAccountCompatible(ctx, fresh, req); !compatible {
			attemptStats.record(reason)
			continue
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil {
			attemptStats.record("db_recheck_failed")
			continue
		}
		if compatible, reason := s.isAdaptiveSelectionAccountCompatible(ctx, fresh, req); !compatible {
			attemptStats.record(reason)
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			attemptStats.record("compact_unsupported")
			continue
		}
		coreSettings := openAIAdaptiveCoreSettings(cfg)
		now := time.Now()
		effectiveCapacity := s.core.effectiveCapacity(fresh.ID, fresh.Concurrency, now, coreSettings)
		requestID := openAIAdaptiveRequestID(ctx)
		if recoveryProbe && !s.core.claimRecoveryProbe(fresh.ID, requestID, fresh.Concurrency, now, coreSettings) {
			attemptStats.record("recovery_probe_unavailable")
			continue
		}
		if !s.core.claimHealthProbe(fresh.ID, requestID, fresh.Concurrency, now, coreSettings) {
			if recoveryProbe {
				s.core.releaseRecoveryProbe(fresh.ID, requestID, time.Now())
			}
			attemptStats.record("health_circuit")
			continue
		}
		quotaAllowed, quotaClaimed := s.core.claimQuotaProbe(fresh.ID, requestID, fresh.Concurrency, now, coreSettings)
		if !quotaAllowed {
			s.core.releaseHealthProbe(fresh.ID, requestID, time.Now())
			if recoveryProbe {
				s.core.releaseRecoveryProbe(fresh.ID, requestID, time.Now())
			}
			attemptStats.record("quota_limited")
			continue
		}
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, fresh.ID, effectiveCapacity)
		if acquireErr != nil {
			s.core.releaseHealthProbe(fresh.ID, requestID, time.Now())
			if recoveryProbe {
				s.core.releaseRecoveryProbe(fresh.ID, requestID, time.Now())
			}
			if quotaClaimed {
				s.core.releaseQuotaProbe(fresh.ID, requestID, time.Now())
			}
			return nil, compactBlocked, acquireErr
		}
		if result != nil && result.Acquired {
			s.registerAdaptiveAdmissionWithRequestAndLoad(ctx, fresh, requestID, true, candidate.ttftBucketKey, coreSettings, candidate.loadInfo)
			if req.SessionHash != "" && !req.PreserveStickyBinding {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, fresh.ID)
			}
			release := result.ReleaseFunc
			result.ReleaseFunc = func() {
				if release != nil {
					release()
				}
				if quotaClaimed {
					s.core.releaseQuotaProbe(fresh.ID, requestID, time.Now())
				}
			}
			selection, selectErr := s.service.newAcquiredSelectionResult(ctx, fresh, result.ReleaseFunc)
			if selectErr != nil {
				s.core.releaseHealthProbe(fresh.ID, requestID, time.Now())
				if recoveryProbe {
					s.core.releaseRecoveryProbe(fresh.ID, requestID, time.Now())
				}
			}
			return selection, compactBlocked, selectErr
		}
		s.core.releaseHealthProbe(fresh.ID, requestID, time.Now())
		if recoveryProbe {
			s.core.releaseRecoveryProbe(fresh.ID, requestID, time.Now())
		}
		if quotaClaimed {
			s.core.releaseQuotaProbe(fresh.ID, requestID, time.Now())
		}
		attemptStats.record("slot_unavailable")
	}
	return nil, compactBlocked, nil
}

func (s *adaptiveOpenAIAccountScheduler) isAdaptiveSelectionAccountCompatible(
	ctx context.Context,
	account *Account,
	req OpenAIAccountScheduleRequest,
) (bool, string) {
	if account == nil {
		return false, "account_nil"
	}
	if !s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) {
		return false, "transport_incompatible"
	}
	return s.baseline.isAccountRequestCompatibleReason(ctx, account, req)
}

func (s *adaptiveOpenAIAccountScheduler) adaptiveFreshResolveFailureReason(
	ctx context.Context,
	account *Account,
	req OpenAIAccountScheduleRequest,
) string {
	if compatible, reason := s.isAdaptiveSelectionAccountCompatible(ctx, account, req); !compatible {
		return reason
	}
	return "fresh_resolve_failed"
}

func (s *adaptiveOpenAIAccountScheduler) logShadowDecision(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	baseline *AccountSelectionResult,
) {
	if !cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled {
		return
	}
	selectionOrder, candidateCount, topK, err := s.buildAdaptiveSelectionOrder(ctx, req, cfg)
	if err != nil {
		slog.Debug("openai_adaptive_shadow_failed", "error", err, "model", req.RequestedModel)
		return
	}
	var adaptiveID int64
	if len(selectionOrder) > 0 && selectionOrder[0].account != nil {
		adaptiveID = selectionOrder[0].account.ID
	}
	var baselineID int64
	if baseline != nil && baseline.Account != nil {
		baselineID = baseline.Account.ID
	}
	diverged := adaptiveID > 0 && baselineID > 0 && adaptiveID != baselineID
	if !cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled || (!diverged && !shouldLogOpenAIAdaptiveDiagnostic(ctx, req, cfg)) {
		return
	}
	slog.Info("openai_adaptive_shadow_decision",
		"baseline_account_id", baselineID,
		"adaptive_account_id", adaptiveID,
		"diverged", diverged,
		"candidate_count", candidateCount,
		"top_k", topK,
		"model", req.RequestedModel,
	)
}

func (s *adaptiveOpenAIAccountScheduler) logEnforceDiagnosticDecision(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	decision OpenAIAccountScheduleDecision,
	selection *AccountSelectionResult,
	candidates []openAIAdaptiveDiagnosticCandidate,
	outcome string,
	err error,
	startedAt time.Time,
) {
	if !shouldLogOpenAIAdaptiveDiagnostic(ctx, req, cfg) {
		return
	}
	if !startedAt.IsZero() {
		decision.LatencyMs = time.Since(startedAt).Milliseconds()
	}
	selectedAccountID := decision.SelectedAccountID
	selectedAccountType := decision.SelectedAccountType
	if selection != nil && selection.Account != nil {
		selectedAccountID = selection.Account.ID
		selectedAccountType = selection.Account.Type
	}
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"outcome", outcome,
		"model", req.RequestedModel,
		"platform", req.Platform,
		"group_id", derefGroupID(req.GroupID),
		"required_transport", string(req.RequiredTransport),
		"required_capability", string(req.RequiredCapability),
		"require_compact", req.RequireCompact,
		"session_sticky", req.SessionHash != "",
		"previous_response", req.PreviousResponseID != "",
		"layer", decision.Layer,
		"selected_account_id", selectedAccountID,
		"selected_account_type", selectedAccountType,
		"candidate_count", decision.CandidateCount,
		"top_k", decision.TopK,
		"load_skew", decision.LoadSkew,
		"latency_ms", decision.LatencyMs,
		"diagnostic_sample_rate", cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate,
		"candidates", candidates,
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	slog.Info("openai_adaptive_scheduler_diagnostic_decision", fields...)
}

func (s *adaptiveOpenAIAccountScheduler) logDiagnosticResult(
	ctx context.Context,
	cfg OpenAIAdaptiveSchedulerSettings,
	report OpenAIAccountScheduleReport,
) {
	force := !report.Success && report.Err != nil
	if !cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled || (!force && !shouldLogOpenAIAdaptiveDiagnostic(ctx, OpenAIAccountScheduleRequest{
		StickyAccountID: report.AccountID,
	}, cfg)) {
		return
	}
	account := s.reportAccountSnapshot(report.AccountID)
	configuredCapacity := 0
	accountType := ""
	platform := ""
	if account != nil {
		configuredCapacity = account.Concurrency
		accountType = account.Type
		platform = account.Platform
	}
	state := s.core.summarySnapshot(report.AccountID, configuredCapacity, time.Now(), openAIAdaptiveCoreSettings(cfg))
	firstTokenStatus := openAIAdaptiveFirstTokenStatus(report)
	failure := openAIAdaptiveFailureLogMetadataFromError(report.Err)
	accountSwitchCount := report.AccountSwitchCount
	if contextSwitchCount, ok := AccountSwitchCountFromContext(ctx); ok && contextSwitchCount > accountSwitchCount {
		accountSwitchCount = contextSwitchCount
	}
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"account_id", report.AccountID,
		"account_type", accountType,
		"platform", platform,
		"account_switch_count", accountSwitchCount,
		"attempt_number", accountSwitchCount + 1,
		"max_account_switches", report.MaxAccountSwitches,
		"success", report.Success,
		"health_sample", report.HealthSample,
		"terminal_reason", report.TerminalReason,
		"failover_outcome", report.FailoverOutcome,
		"failover_suppressed_reason", report.FailoverSuppressedReason,
		"failure_class", failure.FailureClass,
		"upstream_status", failure.UpstreamStatus,
		"upstream_error_code", failure.UpstreamErrorCode,
		"upstream_error_type", failure.UpstreamErrorType,
		"failure_stage", failure.FailureStage,
		"failure_scope", failure.FailureScope,
		"failure_reason", failure.FailureReason,
		"failure_kind", failure.FailureKind,
		"retryable_same_account", failure.RetryableSameAccount,
		"retry_next_account", failure.RetryNextAccount,
		"request_scoped_transient", failure.RequestScopedTransient,
		"safe_to_failover_after_write", failure.SafeToFailoverAfterWrite,
		"first_output_guard_failure", failure.FirstOutputGuardFailure,
		"semantic_output_started", report.SemanticOutputStarted,
		"response_already_communicated", report.ResponseAlreadyCommunicated,
		"same_account_retry_count", report.SameAccountRetryCount,
		"same_account_retry_limit", report.SameAccountRetryLimit,
		"stream", report.Stream,
		"first_token_ms", nullableIntForSlog(report.FirstTokenMs),
		"first_token_status", firstTokenStatus,
		"duration_ms", report.DurationMs,
		"configured_capacity", configuredCapacity,
		"health_samples", len(state.HealthObservations),
		"success_ema", state.SuccessEMA,
		"ttft_ema", state.TTFTEMA,
		"ttft_samples", state.TTFTSamples,
		"effective_capacity", state.EffectiveCapacity,
		"consecutive_failure", state.ConsecutiveFailures,
		"high_error", state.HighError,
		"circuit_status", adaptiveDiagnosticCircuitStatus(state, time.Now()),
		"circuit_open_until", state.CircuitOpenUntil,
		"circuit_open_count", state.CircuitOpenCount,
		"capacity_generation", state.CapacityGeneration,
		"capacity_cooldown_until", state.CapacityCooldownUntil,
		"capacity_recovery_successes", state.CapacityRecoverySuccesses,
		"quota_limited", state.QuotaLimited,
		"quota_reset_at", state.QuotaResetAt,
		"quota_next_probe_at", state.QuotaNextProbeAt,
		"last_dispatch_at", state.LastDispatchAt,
		"last_probe_at", state.LastProbeAt,
		"recovery_status", state.RecoveryStatus,
		"recovery_successes", state.RecoverySuccesses,
		"recovery_probe_in_flight", state.RecoveryProbeInFlight,
		"cooldown_applied", report.Cooldown,
		"cooldown_reason", report.CooldownReason,
		"diagnostic_sample_rate", cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate,
	}
	if report.Err != nil {
		fields = append(fields, "error", report.Err.Error())
	}
	slog.Info("openai_adaptive_scheduler_diagnostic_result", fields...)
}

func shouldLogOpenAIAdaptiveDiagnostic(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) bool {
	if !cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled {
		return false
	}
	rate := cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	seed := deriveOpenAISelectionSeed(req)
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	if rid := contextStringValue(ctx, ctxkey.RequestID); rid != "" {
		seed ^= hashString64(rid)
	}
	if cid := contextStringValue(ctx, ctxkey.ClientRequestID); cid != "" {
		seed ^= hashString64(cid)
	}
	rng := newOpenAISelectionRNG(seed)
	return rng.nextFloat64() < rate
}

func openAIAdaptiveDiagnosticCandidates(
	candidates []openAIAdaptiveCandidateScore,
	limit int,
	settings adaptiveCoreSettings,
) []openAIAdaptiveDiagnosticCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
	out := make([]openAIAdaptiveDiagnosticCandidate, 0, limit)
	now := time.Now()
	for _, item := range candidates[:limit] {
		if item.account == nil {
			continue
		}
		currentConcurrency := 0
		waitingCount := 0
		if item.loadInfo != nil {
			currentConcurrency = item.loadInfo.CurrentConcurrency
			waitingCount = item.loadInfo.WaitingCount
		}
		learning, healthSamples := adaptiveLearningState(item.coreState, item.account.IsOAuth(), now, settings)
		out = append(out, openAIAdaptiveDiagnosticCandidate{
			AccountID:                 item.account.ID,
			AccountType:               item.account.Type,
			ConfiguredCapacity:        item.account.Concurrency,
			EffectiveCapacity:         item.effectiveCapacity,
			CurrentConcurrency:        currentConcurrency,
			WaitingCount:              waitingCount,
			Score:                     item.score,
			ReliabilityScore:          item.successScore,
			CostScore:                 item.costScore,
			CapacityScore:             item.capacityScore,
			TTFTScore:                 item.latencyScore,
			TTFTBucket:                item.ttftBucketKey,
			TTFTP50:                   item.ttftP50,
			TTFTP90:                   item.ttftP90,
			TTFTWindowSamples:         item.ttftWindowSamples,
			LearningStatus:            string(learning),
			HealthSamples:             healthSamples,
			SuccessEMA:                item.coreState.SuccessEMA,
			TTFTEMA:                   item.coreState.TTFTEMA,
			TTFTSamples:               item.coreState.TTFTSamples,
			ConsecutiveFailure:        item.coreState.ConsecutiveFailures,
			HighError:                 item.coreState.HighError,
			CircuitStatus:             adaptiveDiagnosticCircuitStatus(item.coreState, now),
			CircuitOpenUntil:          item.coreState.CircuitOpenUntil,
			CircuitOpenCount:          item.coreState.CircuitOpenCount,
			CapacityGeneration:        item.coreState.CapacityGeneration,
			CapacityCooldownUntil:     item.coreState.CapacityCooldownUntil,
			CapacityRecoverySuccesses: item.coreState.CapacityRecoverySuccesses,
			QuotaLimited:              item.coreState.QuotaLimited,
			QuotaResetAt:              item.coreState.QuotaResetAt,
			QuotaNextProbeAt:          item.coreState.QuotaNextProbeAt,
			LastDispatchAt:            item.coreState.LastDispatchAt,
			LastProbeAt:               item.coreState.LastProbeAt,
			RecoveryStatus:            string(item.coreState.RecoveryStatus),
			RecoverySuccesses:         item.coreState.RecoverySuccesses,
			RecoveryProbeInFlight:     item.coreState.RecoveryProbeInFlight,
		})
	}
	return out
}

func openAIAdaptiveDiagnosticCandidatesForLog(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	candidates []openAIAdaptiveCandidateScore,
) []openAIAdaptiveDiagnosticCandidate {
	if !shouldLogOpenAIAdaptiveDiagnostic(ctx, req, cfg) {
		return nil
	}
	return openAIAdaptiveDiagnosticCandidates(candidates, 5, openAIAdaptiveCoreSettings(cfg))
}

func contextStringValue(ctx context.Context, key ctxkey.Key) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(key).(string)
	return strings.TrimSpace(value)
}

func nullableIntForSlog(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func openAIAdaptiveFirstTokenStatus(report OpenAIAccountScheduleReport) string {
	if report.FirstTokenMs != nil && *report.FirstTokenMs > 0 {
		return "recorded"
	}
	if report.FirstTokenMs != nil {
		return "zero_value"
	}
	if !report.Stream {
		return "not_applicable"
	}
	if report.Success {
		return "stream_first_token_missing"
	}
	return "stream_failed_before_first_token"
}

func hashString64(value string) uint64 {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(value); i++ {
		h ^= uint64(value[i])
		h *= 1099511628211
	}
	return h
}

func (s *adaptiveOpenAIAccountScheduler) ReportResult(accountID int64, success bool, firstTokenMs *int) {
	s.ReportResultWithContext(context.Background(), accountID, success, firstTokenMs)
}

func (s *adaptiveOpenAIAccountScheduler) ReportResultWithContext(ctx context.Context, accountID int64, success bool, firstTokenMs *int) {
	s.ReportScheduleResultWithContext(ctx, OpenAIAccountScheduleReport{
		AccountID:      accountID,
		Success:        success,
		FirstTokenMs:   firstTokenMs,
		HealthSample:   true,
		TerminalReason: "legacy_result",
	})
}

func (s *adaptiveOpenAIAccountScheduler) ReportScheduleResultWithContext(ctx context.Context, report OpenAIAccountScheduleReport) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := s.service.openAIAdaptiveSchedulerSettings(ctx)
	if !cfg.OpenAIAdaptiveSchedulerEnabled {
		return
	}
	requestID := openAIAdaptiveRequestID(ctx)
	if report.HealthSample && !report.BalanceInsufficient {
		s.baseline.ReportResult(report.AccountID, report.Success, report.FirstTokenMs)
	}
	observationType, authentication := classifyAdaptiveTerminalReason(report.Success, report.TerminalReason)
	accountHealthEligible := report.TerminalReason == "account_auth" || report.TerminalReason == "transport_error" || report.TerminalReason == "upstream_5xx" || report.TerminalReason == "concurrency_limit"
	if report.BalanceInsufficient {
		observationType = adaptiveObservationQuotaLimit
	}
	if !report.HealthSample && (observationType == adaptiveObservationHealthSuccess || observationType == adaptiveObservationAccountFailure) {
		observationType = adaptiveObservationIgnored
	}
	admission, hasAdmission := s.core.admissionSnapshot(report.AccountID, requestID)
	configuredCapacity := admission.ConfiguredCapacity
	var account *Account
	if !hasAdmission || observationType == adaptiveObservationQuotaLimit {
		account = s.reportAccountSnapshot(report.AccountID)
		if account != nil {
			configuredCapacity = account.Concurrency
		}
	}
	observation := adaptiveObservation{
		AccountID:             report.AccountID,
		RequestID:             requestID,
		Type:                  observationType,
		ReasonCode:            report.TerminalReason,
		Reason:                report.CooldownReason,
		Authentication:        authentication,
		FirstTokenMs:          report.FirstTokenMs,
		WindowedTTFT:          true,
		ConfiguredCapacity:    configuredCapacity,
		ObservedConcurrency:   report.ObservedConcurrency,
		WaitingCount:          report.WaitingCount,
		AccountHealthEligible: &accountHealthEligible,
	}
	if report.ObservedConcurrency <= 0 {
		observation.ObservedConcurrency = -1
	}
	if account != nil && observationType == adaptiveObservationQuotaLimit {
		observation.QuotaResetAt = account.RateLimitResetAt
	}
	s.core.observe(observation, time.Now(), openAIAdaptiveCoreSettings(cfg))
	_, _, circuitTracked, _ := s.core.runtimeStatus(report.AccountID, configuredCapacity, time.Now(), openAIAdaptiveCoreSettings(cfg))
	if observationType == adaptiveObservationCapacityLimit || observationType == adaptiveObservationQuotaLimit || circuitTracked {
		s.clearStickySessionsForCooldown(ctx, report.AccountID, string(observationType))
	}
	s.logDiagnosticResult(ctx, cfg, report)
}

func (s *adaptiveOpenAIAccountScheduler) clearStickySessionsForCooldown(ctx context.Context, accountID int64, reason string) {
	if s == nil || s.service == nil || accountID <= 0 || !openAIAdaptiveCooldownShouldClearSticky(reason) {
		return
	}
	cleaner, ok := s.service.cache.(GatewayAccountStickyCleaner)
	if !ok || cleaner == nil {
		return
	}
	if !s.shouldRunStickyCleanup(accountID, time.Now()) {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	deleted, err := cleaner.DeleteSessionsByAccountID(cleanupCtx, accountID)
	if err != nil {
		slog.Warn("openai_adaptive_scheduler_sticky_cleanup_failed",
			"account_id", accountID,
			"cooldown_reason", reason,
			"error", err,
		)
		return
	}
	if deleted > 0 {
		slog.Info("openai_adaptive_scheduler_sticky_cleanup",
			"account_id", accountID,
			"cooldown_reason", reason,
			"deleted_sessions", deleted,
		)
	}
}

func openAIAdaptiveCooldownShouldClearSticky(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "concurrency_limit", "insufficient_balance", string(adaptiveObservationCapacityLimit), string(adaptiveObservationQuotaLimit), string(adaptiveObservationAccountFailure):
		return true
	default:
		return false
	}
}

func (s *adaptiveOpenAIAccountScheduler) shouldRunStickyCleanup(accountID int64, now time.Time) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.stickyCleanupMu.Lock()
	defer s.stickyCleanupMu.Unlock()
	if s.stickyCleanupLastByID == nil {
		s.stickyCleanupLastByID = make(map[int64]time.Time)
	}
	if last := s.stickyCleanupLastByID[accountID]; !last.IsZero() && now.Sub(last) < openAIAdaptiveStickyCleanupMinInterval {
		return false
	}
	s.stickyCleanupLastByID[accountID] = now
	return true
}

func (s *adaptiveOpenAIAccountScheduler) ReportSwitch() {
	if s == nil {
		return
	}
	s.baseline.ReportSwitch()
	s.metrics.recordSwitch()
}

func (s *adaptiveOpenAIAccountScheduler) SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	if s == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}
	selectTotal := s.metrics.selectTotal.Load()
	if selectTotal == 0 {
		return s.baseline.SnapshotMetrics()
	}
	sessionHit := s.metrics.stickySessionHitTotal.Load()
	prevHit := s.metrics.stickyPreviousHitTotal.Load()
	switchTotal := s.metrics.accountSwitchTotal.Load()
	latencyTotal := s.metrics.latencyMsTotal.Load()
	loadSkewTotal := s.metrics.loadSkewMilliTotal.Load()
	snapshot := OpenAIAccountSchedulerMetricsSnapshot{
		SelectTotal:              selectTotal,
		StickyPreviousHitTotal:   prevHit,
		StickySessionHitTotal:    sessionHit,
		LoadBalanceSelectTotal:   s.metrics.loadBalanceSelectTotal.Load(),
		AccountSwitchTotal:       switchTotal,
		SchedulerLatencyMsTotal:  latencyTotal,
		RuntimeStatsAccountCount: s.baseline.stats.size(),
	}
	if selectTotal > 0 {
		snapshot.SchedulerLatencyMsAvg = float64(latencyTotal) / float64(selectTotal)
		snapshot.StickyHitRatio = float64(prevHit+sessionHit) / float64(selectTotal)
		snapshot.AccountSwitchRate = float64(switchTotal) / float64(selectTotal)
		snapshot.LoadSkewAvg = float64(loadSkewTotal) / 1000 / float64(selectTotal)
	}
	return snapshot
}

func (s *adaptiveOpenAIAccountScheduler) reportAccountSnapshot(accountID int64) *Account {
	if s == nil || s.service == nil || accountID <= 0 {
		return nil
	}
	if s.service.schedulerSnapshot == nil && s.service.accountRepo == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	account, err := s.service.getSchedulableAccount(ctx, accountID)
	if err != nil {
		return nil
	}
	return account
}

func openAIAdaptiveRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID := contextStringValue(ctx, ctxkey.RequestID); requestID != "" {
		return requestID
	}
	return contextStringValue(ctx, ctxkey.ClientRequestID)
}

func adaptiveLoadRate(loadInfo *AccountLoadInfo, effectiveCapacity int) float64 {
	if loadInfo == nil {
		return 0
	}
	if effectiveCapacity > 0 {
		return clamp01(float64(loadInfo.CurrentConcurrency)/float64(effectiveCapacity)) * 100
	}
	return float64(loadInfo.LoadRate)
}

func normalizeAdaptiveValue(value, minValue, maxValue, fallback float64) float64 {
	if math.IsInf(minValue, 0) || math.IsInf(maxValue, 0) || maxValue <= minValue {
		return fallback
	}
	return clamp01((value - minValue) / (maxValue - minValue))
}
