package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const openAIAccountScheduleLayerAdaptive = "adaptive"

var errOpenAIAdaptiveSchedulerFallback = errors.New("openai adaptive scheduler fallback")

const openAIAdaptiveStickyCleanupMinInterval = 30 * time.Second

type adaptiveOpenAIAccountScheduler struct {
	service               *OpenAIGatewayService
	baseline              *defaultOpenAIAccountScheduler
	state                 *openAIAdaptiveSchedulerStateStore
	metrics               openAIAccountSchedulerMetrics
	stickyCleanupMu       sync.Mutex
	stickyCleanupLastByID map[int64]time.Time
}

type openAIAdaptiveAccountState struct {
	AccountID int64

	EstimatedCapacity int
	SuccessEMA        float64
	ErrorEMA          float64
	LatencyEMA        float64
	TTFTEMA           float64

	ThompsonAlpha float64
	ThompsonBeta  float64

	ConsecutiveSuccess         int
	ConsecutiveFailure         int
	ConsecutiveCapacityFailure int

	TotalSamples   int64
	RecentSamples  int
	RecentFailures int

	LastSuccessAt         time.Time
	LastFailureAt         time.Time
	RecentWindowStartedAt time.Time
	LastCapacityFailureAt time.Time
	CooldownUntil         time.Time
}

type openAIAdaptiveSchedulerStateStore struct {
	mu     sync.RWMutex
	states map[int64]*openAIAdaptiveAccountState
}

type openAIAdaptiveCandidateScore struct {
	account           *Account
	loadInfo          *AccountLoadInfo
	state             openAIAdaptiveAccountState
	effectiveCapacity int
	score             float64
	successScore      float64
	costScore         float64
	capacityScore     float64
	latencyScore      float64
	stabilityScore    float64
	explorationScore  float64
}

type openAIAdaptiveDiagnosticCandidate struct {
	AccountID          int64     `json:"account_id"`
	AccountType        string    `json:"account_type"`
	Priority           int       `json:"priority"`
	EffectiveCapacity  int       `json:"effective_capacity"`
	CurrentConcurrency int       `json:"current_concurrency"`
	WaitingCount       int       `json:"waiting_count"`
	Score              float64   `json:"score"`
	SuccessScore       float64   `json:"success_score"`
	CostScore          float64   `json:"cost_score"`
	CapacityScore      float64   `json:"capacity_score"`
	LatencyScore       float64   `json:"latency_score"`
	StabilityScore     float64   `json:"stability_score"`
	ExplorationScore   float64   `json:"exploration_score"`
	TotalSamples       int64     `json:"total_samples"`
	RecentSamples      int       `json:"recent_samples"`
	RecentFailures     int       `json:"recent_failures"`
	ConsecutiveFailure int       `json:"consecutive_failure"`
	CooldownUntil      time.Time `json:"cooldown_until"`
	CooldownStatus     string    `json:"cooldown_status"`
}

func newAdaptiveOpenAIAccountScheduler(service *OpenAIGatewayService, stats *openAIAccountRuntimeStats) OpenAIAccountScheduler {
	if stats == nil {
		stats = newOpenAIAccountRuntimeStats()
	}
	baseline := &defaultOpenAIAccountScheduler{
		service: service,
		stats:   stats,
	}
	return &adaptiveOpenAIAccountScheduler{
		service:  service,
		baseline: baseline,
		state:    newOpenAIAdaptiveSchedulerStateStore(),
	}
}

func newOpenAIAdaptiveSchedulerStateStore() *openAIAdaptiveSchedulerStateStore {
	return &openAIAdaptiveSchedulerStateStore{
		states: make(map[int64]*openAIAdaptiveAccountState),
	}
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
		s.logShadowDecision(ctx, req, cfg, selection)
		return selection, decision, err
	}

	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerAdaptive}
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		s.metrics.recordSelect(decision)
	}()

	if selection, ok, err := s.selectByPreviousResponse(ctx, req, &decision); err != nil || ok {
		outcome := "previous_response"
		if err != nil {
			outcome = "previous_response_error"
		} else if selection == nil || selection.Account == nil {
			outcome = "previous_response_empty"
		}
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, nil, outcome, err)
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
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, nil, "session_sticky", nil)
		return selection, decision, nil
	}
	if escapedSticky {
		req.PreserveStickyBinding = true
	}

	selection, candidateCount, topK, loadSkew, diagnosticCandidates, err := s.selectByAdaptiveLoadBalance(ctx, req, cfg)
	decision.Layer = openAIAccountScheduleLayerAdaptive
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	if err != nil {
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, nil, diagnosticCandidates, "fallback", err)
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
		return s.selectCurrentBaseline(ctx, req)
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, diagnosticCandidates, "selected", nil)
		return selection, decision, nil
	}
	s.logEnforceDiagnosticDecision(ctx, req, cfg, decision, selection, diagnosticCandidates, "empty_selection", nil)
	return selection, decision, nil
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

func (s *adaptiveOpenAIAccountScheduler) selectLegacyLoadAware(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerLoadBalance}
	if req.RequiredTransport == OpenAIUpstreamTransportAny || req.RequiredTransport == OpenAIUpstreamTransportHTTPSSE {
		effectiveExcludedIDs := cloneExcludedAccountIDs(req.ExcludedIDs)
		for {
			selection, err := s.service.selectAccountWithLoadAwareness(ctx, req.GroupID, req.Platform, req.SessionHash, req.RequestedModel, effectiveExcludedIDs, req.RequireCompact, req.RequiredCapability)
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
		selection, err := s.service.selectAccountWithLoadAwareness(ctx, req.GroupID, req.Platform, req.SessionHash, req.RequestedModel, effectiveExcludedIDs, req.RequireCompact, req.RequiredCapability)
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
	decision *OpenAIAccountScheduleDecision,
) (*AccountSelectionResult, bool, error) {
	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID == "" || normalizeOpenAICompatiblePlatform(req.Platform) != PlatformOpenAI {
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
		account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) ||
		!account.IsOpenAICompatible() ||
		!account.IsSchedulable() ||
		s.service.isOpenAIAccountRuntimeBlocked(account) ||
		!s.baseline.isAccountRequestCompatible(ctx, account, req) ||
		!s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
	if account == nil || !openAIStickyAccountMatchesGroup(account, req.GroupID) ||
		s.service.isOpenAIAccountRuntimeBlocked(account) ||
		!s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) ||
		!s.baseline.isAccountRequestCompatible(ctx, account, req) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	state := s.state.snapshot(account.ID, cfg)
	effectiveCapacity := effectiveOpenAIAdaptiveCapacity(account, state, cfg)
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
	state = s.state.observeLoad(account, cfg, loadInfo)
	effectiveCapacity = effectiveOpenAIAdaptiveCapacityWithLoad(account, state, cfg, loadInfo)
	if effectiveCapacity > 0 && loadInfo.CurrentConcurrency >= effectiveCapacity {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, account.ID, effectiveCapacity)
	if acquireErr != nil {
		return nil, false, acquireErr
	}
	if result != nil && result.Acquired {
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
	if s.service.concurrencyService == nil || !s.service.schedulingConfig().LoadBatchEnabled {
		return nil, 0, 0, 0, nil, errOpenAIAdaptiveSchedulerFallback
	}
	selectionOrder, candidateCount, topK, loadSkew, loadReq, filtered, states, err := s.buildAdaptiveSelectionOrderWithLoad(ctx, req, cfg, true)
	diagnosticCandidates := openAIAdaptiveDiagnosticCandidates(selectionOrder, 5)
	if err != nil {
		return nil, candidateCount, topK, loadSkew, diagnosticCandidates, err
	}
	result, compactBlocked, acquireErr := s.tryAcquireAdaptiveSelectionOrder(ctx, req, cfg, selectionOrder)
	if acquireErr != nil {
		return nil, candidateCount, topK, loadSkew, diagnosticCandidates, acquireErr
	}
	if result != nil {
		return result, candidateCount, topK, loadSkew, diagnosticCandidates, nil
	}

	if s.service.concurrencyService != nil {
		if freshLoadMap, loadErr := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, loadReq); loadErr == nil {
			freshCandidates, freshSkew := s.buildAdaptiveCandidates(req, cfg, filtered, states, freshLoadMap, true)
			freshOrder := buildOpenAIAdaptiveSelectionOrder(freshCandidates, req, cfg)
			freshResult, freshCompactBlocked, freshAcquireErr := s.tryAcquireAdaptiveSelectionOrder(ctx, req, cfg, freshOrder)
			if freshAcquireErr != nil {
				return nil, candidateCount, topK, loadSkew, diagnosticCandidates, freshAcquireErr
			}
			if freshResult != nil {
				freshTopK := cfg.OpenAIAdaptiveSchedulerTopK
				if freshTopK > len(freshCandidates) {
					freshTopK = len(freshCandidates)
				}
				return freshResult, len(freshCandidates), freshTopK, freshSkew, openAIAdaptiveDiagnosticCandidates(freshOrder, 5), nil
			}
			compactBlocked = compactBlocked || freshCompactBlocked
		}
	}

	cfgWait := s.service.schedulingConfig()
	for _, candidate := range selectionOrder {
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.baseline.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.baseline.isAccountRequestCompatible(ctx, fresh, req) {
			continue
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.baseline.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.baseline.isAccountRequestCompatible(ctx, fresh, req) {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			continue
		}
		effectiveCapacity := effectiveOpenAIAdaptiveCapacityWithLoad(fresh, s.state.snapshot(fresh.ID, cfg), cfg, candidate.loadInfo)
		selection, selectErr := s.service.newSelectionResult(ctx, fresh, false, nil, &AccountWaitPlan{
			AccountID:      fresh.ID,
			MaxConcurrency: effectiveCapacity,
			Timeout:        cfgWait.FallbackWaitTimeout,
			MaxWaiting:     cfgWait.FallbackMaxWaiting,
		})
		return selection, candidateCount, topK, loadSkew, diagnosticCandidates, selectErr
	}

	return nil, candidateCount, topK, loadSkew, diagnosticCandidates, noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked)
}

func (s *adaptiveOpenAIAccountScheduler) buildAdaptiveSelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) ([]openAIAdaptiveCandidateScore, int, int, error) {
	selectionOrder, candidateCount, topK, _, _, _, _, err := s.buildAdaptiveSelectionOrderWithLoad(ctx, req, cfg, false)
	return selectionOrder, candidateCount, topK, err
}

func (s *adaptiveOpenAIAccountScheduler) buildAdaptiveSelectionOrderWithLoad(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	allowSideEffects bool,
) ([]openAIAdaptiveCandidateScore, int, int, float64, []AccountWithConcurrency, []*Account, map[int64]openAIAdaptiveAccountState, error) {
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return nil, 0, 0, 0, nil, nil, nil, err
	}
	if len(accounts) == 0 {
		return nil, 0, 0, 0, nil, nil, nil, noAvailableOpenAISelectionError(req.RequestedModel, false)
	}

	var schedGroup *Group
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		schedGroup, _ = s.service.schedulerSnapshot.GetGroupByID(ctx, *req.GroupID)
	}

	filtered := make([]*Account, 0, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	states := make(map[int64]openAIAdaptiveAccountState, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				continue
			}
		}
		if !account.IsSchedulable() || account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) || !account.IsOpenAICompatible() {
			continue
		}
		if s.service.isOpenAIAccountRuntimeBlocked(account) {
			continue
		}
		if schedGroup != nil && schedGroup.RequirePrivacySet && !account.IsPrivacySet() {
			if allowSideEffects {
				s.service.BlockAccountScheduling(account, time.Time{}, "privacy_not_set")
				_ = s.service.accountRepo.SetError(ctx, account.ID,
					fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			}
			continue
		}
		if !s.baseline.isAccountRequestCompatible(ctx, account, req) || !s.baseline.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		state := s.state.snapshot(account.ID, cfg)
		effectiveCapacity := effectiveOpenAIAdaptiveCapacity(account, state, cfg)
		filtered = append(filtered, account)
		states[account.ID] = state
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: effectiveCapacity,
		})
	}
	if len(filtered) == 0 {
		return nil, 0, 0, 0, nil, nil, nil, noAvailableOpenAISelectionError(req.RequestedModel, false)
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
			loadMap = batchLoad
		}
	}
	candidates, loadSkew := s.buildAdaptiveCandidates(req, cfg, filtered, states, loadMap, allowSideEffects)
	if req.RequireCompact && len(candidates) == 0 {
		return nil, 0, 0, 0, nil, nil, nil, ErrNoAvailableCompactAccounts
	}
	if len(candidates) == 0 {
		return nil, 0, 0, loadSkew, nil, nil, nil, noAvailableOpenAISelectionError(req.RequestedModel, false)
	}
	topK := cfg.OpenAIAdaptiveSchedulerTopK
	if topK > len(candidates) {
		topK = len(candidates)
	}
	selectionOrder := buildOpenAIAdaptiveSelectionOrder(candidates, req, cfg)
	return selectionOrder, len(candidates), topK, loadSkew, loadReq, filtered, states, nil
}

func (s *adaptiveOpenAIAccountScheduler) buildAdaptiveCandidates(
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	filtered []*Account,
	states map[int64]openAIAdaptiveAccountState,
	loadMap map[int64]*AccountLoadInfo,
	allowSideEffects bool,
) ([]openAIAdaptiveCandidateScore, float64) {
	candidates := make([]openAIAdaptiveCandidateScore, 0, len(filtered))
	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	for _, account := range filtered {
		if req.RequireCompact && openAICompactSupportTier(account) == 0 {
			continue
		}
		state := states[account.ID]
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		if allowSideEffects {
			state = s.state.observeLoad(account, cfg, loadInfo)
			states[account.ID] = state
		}
		effectiveCapacity := effectiveOpenAIAdaptiveCapacityWithLoad(account, state, cfg, loadInfo)
		if effectiveCapacity > 0 && loadInfo.CurrentConcurrency >= effectiveCapacity {
			continue
		}
		loadRate := adaptiveLoadRate(loadInfo, effectiveCapacity)
		loadRateSum += loadRate
		loadRateSumSquares += loadRate * loadRate
		candidates = append(candidates, openAIAdaptiveCandidateScore{
			account:           account,
			loadInfo:          loadInfo,
			state:             state,
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
	minCost, maxCost := math.Inf(1), math.Inf(-1)
	minLatency, maxLatency := math.Inf(1), math.Inf(-1)
	hasLatency := false

	rawCost := make([]float64, len(candidates))
	for i := range candidates {
		costMultiplier := candidates[i].account.BillingRateMultiplier()
		if costMultiplier < cfg.OpenAIAdaptiveSchedulerMinCostMultiplier {
			costMultiplier = cfg.OpenAIAdaptiveSchedulerMinCostMultiplier
		}
		rawCost[i] = 1 / costMultiplier
		if rawCost[i] < minCost {
			minCost = rawCost[i]
		}
		if rawCost[i] > maxCost {
			maxCost = rawCost[i]
		}
		latency := candidates[i].state.TTFTEMA
		if latency <= 0 {
			latency = candidates[i].state.LatencyEMA
		}
		if latency > 0 {
			hasLatency = true
			if latency < minLatency {
				minLatency = latency
			}
			if latency > maxLatency {
				maxLatency = latency
			}
		}
	}

	weightSum := cfg.OpenAIAdaptiveSchedulerWeightSuccess +
		cfg.OpenAIAdaptiveSchedulerWeightCost +
		cfg.OpenAIAdaptiveSchedulerWeightCapacity +
		cfg.OpenAIAdaptiveSchedulerWeightLatency +
		cfg.OpenAIAdaptiveSchedulerWeightStability +
		cfg.OpenAIAdaptiveSchedulerWeightExploration
	if weightSum <= 0 {
		weightSum = 1
	}

	for i := range candidates {
		item := &candidates[i]
		item.successScore = clamp01(item.state.SuccessEMA)
		item.costScore = normalizeAdaptiveValue(rawCost[i], minCost, maxCost, 0.5)
		remaining := float64(item.effectiveCapacity - item.loadInfo.CurrentConcurrency)
		if item.effectiveCapacity <= 0 {
			item.capacityScore = 1
		} else {
			item.capacityScore = clamp01(remaining / float64(item.effectiveCapacity))
		}
		item.latencyScore = 0.5
		latency := item.state.TTFTEMA
		if latency <= 0 {
			latency = item.state.LatencyEMA
		}
		if hasLatency && latency > 0 {
			item.latencyScore = 1 - normalizeAdaptiveValue(latency, minLatency, maxLatency, 0.5)
		}
		item.stabilityScore = clamp01(1 - item.state.ErrorEMA)
		if item.state.ConsecutiveFailure > 0 {
			item.stabilityScore *= 1 / (1 + float64(item.state.ConsecutiveFailure)*0.25)
		}
		item.explorationScore = 1 / math.Sqrt(float64(item.state.TotalSamples+1))
		if cfg.OpenAIAdaptiveSchedulerThompsonEnabled {
			item.explorationScore = clamp01(item.state.ThompsonAlpha / (item.state.ThompsonAlpha + item.state.ThompsonBeta))
		}
		item.score = (cfg.OpenAIAdaptiveSchedulerWeightSuccess*item.successScore +
			cfg.OpenAIAdaptiveSchedulerWeightCost*item.costScore +
			cfg.OpenAIAdaptiveSchedulerWeightCapacity*item.capacityScore +
			cfg.OpenAIAdaptiveSchedulerWeightLatency*item.latencyScore +
			cfg.OpenAIAdaptiveSchedulerWeightStability*item.stabilityScore +
			cfg.OpenAIAdaptiveSchedulerWeightExploration*item.explorationScore) / weightSum
	}
}

func buildOpenAIAdaptiveSelectionOrder(
	candidates []openAIAdaptiveCandidateScore,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) []openAIAdaptiveCandidateScore {
	if len(candidates) <= 1 {
		return append([]openAIAdaptiveCandidateScore(nil), candidates...)
	}
	ranked := append([]openAIAdaptiveCandidateScore(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return isOpenAIAdaptiveCandidateBetter(ranked[i], ranked[j], cfg)
	})
	topK := cfg.OpenAIAdaptiveSchedulerTopK
	if topK <= 0 || topK > len(ranked) {
		topK = len(ranked)
	}

	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	if cfg.OpenAIAdaptiveSchedulerExplorationRate > 0 && rng.nextFloat64() < cfg.OpenAIAdaptiveSchedulerExplorationRate {
		explorePool := append([]openAIAdaptiveCandidateScore(nil), ranked[topK:]...)
		fallbackTop := ranked[:topK]
		if len(explorePool) == 0 {
			explorePool = append([]openAIAdaptiveCandidateScore(nil), ranked...)
			fallbackTop = nil
		}
		if cfg.OpenAIAdaptiveSchedulerThompsonEnabled {
			for i := range explorePool {
				explorePool[i].explorationScore = sampleOpenAIAdaptiveBeta(
					explorePool[i].state.ThompsonAlpha,
					explorePool[i].state.ThompsonBeta,
					&rng,
				)
			}
		}
		sort.SliceStable(explorePool, func(i, j int) bool {
			if explorePool[i].explorationScore != explorePool[j].explorationScore {
				return explorePool[i].explorationScore > explorePool[j].explorationScore
			}
			return isOpenAIAdaptiveCandidateBetter(explorePool[i], explorePool[j], cfg)
		})
		order := make([]openAIAdaptiveCandidateScore, 0, len(ranked))
		order = append(order, explorePool...)
		return append(order, fallbackTop...)
	}

	return buildOpenAIAdaptiveSoftmaxOrder(ranked[:topK], ranked[topK:], req, cfg)
}

func sampleOpenAIAdaptiveBeta(alpha, beta float64, rng *openAISelectionRNG) float64 {
	if rng == nil || alpha <= 0 || beta <= 0 || math.IsNaN(alpha) || math.IsNaN(beta) ||
		math.IsInf(alpha, 0) || math.IsInf(beta, 0) {
		return 0.5
	}
	x := sampleOpenAIAdaptiveGamma(alpha, rng)
	y := sampleOpenAIAdaptiveGamma(beta, rng)
	total := x + y
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return clamp01(alpha / (alpha + beta))
	}
	return clamp01(x / total)
}

func sampleOpenAIAdaptiveGamma(shape float64, rng *openAISelectionRNG) float64 {
	if shape < 1 {
		u := rng.nextFloat64()
		for u <= 0 {
			u = rng.nextFloat64()
		}
		return sampleOpenAIAdaptiveGamma(shape+1, rng) * math.Pow(u, 1/shape)
	}

	d := shape - 1.0/3.0
	c := 1 / math.Sqrt(9*d)
	for {
		x := sampleOpenAIAdaptiveStandardNormal(rng)
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.nextFloat64()
		if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

func sampleOpenAIAdaptiveStandardNormal(rng *openAISelectionRNG) float64 {
	u1 := rng.nextFloat64()
	for u1 <= 0 {
		u1 = rng.nextFloat64()
	}
	u2 := rng.nextFloat64()
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

func buildOpenAIAdaptiveSoftmaxOrder(
	top []openAIAdaptiveCandidateScore,
	rest []openAIAdaptiveCandidateScore,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
) []openAIAdaptiveCandidateScore {
	if len(top) <= 1 {
		return append(append([]openAIAdaptiveCandidateScore(nil), top...), rest...)
	}
	temperature := cfg.OpenAIAdaptiveSchedulerSoftmaxTemperature
	if temperature <= 0 {
		temperature = 0.35
	}
	order := make([]openAIAdaptiveCandidateScore, 0, len(top)+len(rest))
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	if cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode == openAIAdaptiveSchedulerAccountTypePriorityMixed {
		order = appendOpenAIAdaptiveSoftmaxPool(order, top, temperature, &rng)
		return append(order, rest...)
	}

	groups := groupOpenAIAdaptiveCandidatesByAccountTypePriority(top, cfg)
	for _, group := range groups {
		order = appendOpenAIAdaptiveSoftmaxPool(order, group, temperature, &rng)
	}
	return append(order, rest...)
}

func appendOpenAIAdaptiveSoftmaxPool(
	order []openAIAdaptiveCandidateScore,
	top []openAIAdaptiveCandidateScore,
	temperature float64,
	rng *openAISelectionRNG,
) []openAIAdaptiveCandidateScore {
	if len(top) == 0 {
		return order
	}
	if len(top) == 1 {
		return append(order, top[0])
	}
	pool := append([]openAIAdaptiveCandidateScore(nil), top...)
	for len(pool) > 0 {
		maxScore := pool[0].score
		for _, item := range pool[1:] {
			if item.score > maxScore {
				maxScore = item.score
			}
		}
		weights := make([]float64, len(pool))
		total := 0.0
		for i, item := range pool {
			weight := math.Exp((item.score - maxScore) / temperature)
			if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
				weight = 1
			}
			weights[i] = weight
			total += weight
		}
		selectedIdx := 0
		if total > 0 {
			r := rng.nextFloat64() * total
			acc := 0.0
			for i, weight := range weights {
				acc += weight
				if r <= acc {
					selectedIdx = i
					break
				}
			}
		}
		order = append(order, pool[selectedIdx])
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
	}
	return order
}

func (s *adaptiveOpenAIAccountScheduler) tryAcquireAdaptiveSelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	selectionOrder []openAIAdaptiveCandidateScore,
) (*AccountSelectionResult, bool, error) {
	compactBlocked := false
	for _, candidate := range selectionOrder {
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.baseline.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.baseline.isAccountRequestCompatible(ctx, fresh, req) {
			continue
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.baseline.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.baseline.isAccountRequestCompatible(ctx, fresh, req) {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			continue
		}
		effectiveCapacity := effectiveOpenAIAdaptiveCapacityWithLoad(fresh, s.state.snapshot(fresh.ID, cfg), cfg, candidate.loadInfo)
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, fresh.ID, effectiveCapacity)
		if acquireErr != nil {
			return nil, compactBlocked, acquireErr
		}
		if result != nil && result.Acquired {
			if req.SessionHash != "" && !req.PreserveStickyBinding {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, fresh.ID)
			}
			selection, selectErr := s.service.newAcquiredSelectionResult(ctx, fresh, result.ReleaseFunc)
			return selection, compactBlocked, selectErr
		}
	}
	return nil, compactBlocked, nil
}

func (s *adaptiveOpenAIAccountScheduler) logShadowDecision(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	cfg OpenAIAdaptiveSchedulerSettings,
	baseline *AccountSelectionResult,
) {
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
	slog.Info("openai_adaptive_shadow_decision",
		"baseline_account_id", baselineID,
		"adaptive_account_id", adaptiveID,
		"diverged", adaptiveID > 0 && baselineID > 0 && adaptiveID != baselineID,
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
) {
	if !shouldLogOpenAIAdaptiveDiagnostic(ctx, req, cfg) {
		return
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
	if !shouldLogOpenAIAdaptiveDiagnostic(ctx, OpenAIAccountScheduleRequest{
		StickyAccountID: report.AccountID,
	}, cfg) {
		return
	}
	state := s.state.snapshot(report.AccountID, cfg)
	firstTokenStatus := openAIAdaptiveFirstTokenStatus(report)
	cooldownStatus := openAIAdaptiveCooldownStatus(state, time.Now())
	fields := []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"account_id", report.AccountID,
		"success", report.Success,
		"health_sample", report.HealthSample,
		"terminal_reason", report.TerminalReason,
		"stream", report.Stream,
		"first_token_ms", nullableIntForSlog(report.FirstTokenMs),
		"first_token_status", firstTokenStatus,
		"duration_ms", report.DurationMs,
		"total_samples", state.TotalSamples,
		"recent_samples", state.RecentSamples,
		"recent_failures", state.RecentFailures,
		"success_ema", state.SuccessEMA,
		"error_ema", state.ErrorEMA,
		"ttft_ema", state.TTFTEMA,
		"latency_ema", state.LatencyEMA,
		"estimated_capacity", state.EstimatedCapacity,
		"consecutive_success", state.ConsecutiveSuccess,
		"consecutive_failure", state.ConsecutiveFailure,
		"consecutive_capacity_failure", state.ConsecutiveCapacityFailure,
		"cooldown_until", state.CooldownUntil,
		"cooldown_status", cooldownStatus,
		"cooldown_applied", report.Cooldown,
		"cooldown_reason", report.CooldownReason,
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
) []openAIAdaptiveDiagnosticCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
	out := make([]openAIAdaptiveDiagnosticCandidate, 0, limit)
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
		out = append(out, openAIAdaptiveDiagnosticCandidate{
			AccountID:          item.account.ID,
			AccountType:        item.account.Type,
			Priority:           item.account.Priority,
			EffectiveCapacity:  item.effectiveCapacity,
			CurrentConcurrency: currentConcurrency,
			WaitingCount:       waitingCount,
			Score:              item.score,
			SuccessScore:       item.successScore,
			CostScore:          item.costScore,
			CapacityScore:      item.capacityScore,
			LatencyScore:       item.latencyScore,
			StabilityScore:     item.stabilityScore,
			ExplorationScore:   item.explorationScore,
			TotalSamples:       item.state.TotalSamples,
			RecentSamples:      item.state.RecentSamples,
			RecentFailures:     item.state.RecentFailures,
			ConsecutiveFailure: item.state.ConsecutiveFailure,
			CooldownUntil:      item.state.CooldownUntil,
			CooldownStatus:     openAIAdaptiveCooldownStatus(item.state, time.Now()),
		})
	}
	return out
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

func openAIAdaptiveCooldownStatus(state openAIAdaptiveAccountState, now time.Time) string {
	if state.CooldownUntil.IsZero() {
		return "none"
	}
	if state.CooldownUntil.After(now) {
		return "active"
	}
	return "expired"
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
	if report.HealthSample {
		s.baseline.ReportResult(report.AccountID, report.Success, report.FirstTokenMs)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := s.service.openAIAdaptiveSchedulerSettings(ctx)
	if !cfg.OpenAIAdaptiveSchedulerEnabled {
		return
	}
	var account *Account
	if report.HealthSample && !s.state.has(report.AccountID) {
		account = s.reportAccountSnapshot(report.AccountID)
	}
	if report.HealthSample {
		s.state.reportWithAccount(account, report.AccountID, cfg, report.Success, report.FirstTokenMs, report.DurationMs)
	}
	if report.Cooldown {
		s.state.applyCooldown(report.AccountID, cfg, report.CooldownReason, time.Now())
		s.clearStickySessionsForCooldown(ctx, report.AccountID, report.CooldownReason)
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
	return strings.TrimSpace(reason) == "concurrency_limit"
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

func (s *openAIAdaptiveSchedulerStateStore) snapshot(accountID int64, cfg OpenAIAdaptiveSchedulerSettings) openAIAdaptiveAccountState {
	if s == nil || accountID <= 0 {
		return defaultOpenAIAdaptiveAccountState(accountID, cfg)
	}
	s.mu.RLock()
	state, ok := s.states[accountID]
	s.mu.RUnlock()
	if ok && state != nil {
		return *state
	}
	return defaultOpenAIAdaptiveAccountState(accountID, cfg)
}

func (s *openAIAdaptiveSchedulerStateStore) has(accountID int64) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	s.mu.RLock()
	_, ok := s.states[accountID]
	s.mu.RUnlock()
	return ok
}

func (s *openAIAdaptiveSchedulerStateStore) report(
	accountID int64,
	cfg OpenAIAdaptiveSchedulerSettings,
	success bool,
	firstTokenMs *int,
	durationMs int64,
) {
	s.reportWithAccount(nil, accountID, cfg, success, firstTokenMs, durationMs)
}

func (s *openAIAdaptiveSchedulerStateStore) reportWithAccount(
	account *Account,
	accountID int64,
	cfg OpenAIAdaptiveSchedulerSettings,
	success bool,
	firstTokenMs *int,
	durationMs int64,
) {
	if s == nil || accountID <= 0 {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[accountID]
	if state == nil {
		initial := defaultOpenAIAdaptiveAccountStateForAccount(account, accountID, cfg)
		state = &initial
		s.states[accountID] = state
	}
	refreshOpenAIAdaptiveLearningWindow(state, cfg, now)
	state.TotalSamples++
	state.RecentSamples++
	successSample := 0.0
	errorSample := 1.0
	if success {
		successSample = 1
		errorSample = 0
	}
	state.SuccessEMA = updateOpenAIAdaptiveEMA(state.SuccessEMA, successSample, cfg.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	state.ErrorEMA = updateOpenAIAdaptiveEMA(state.ErrorEMA, errorSample, cfg.OpenAIAdaptiveSchedulerErrorEMAAlpha)
	if firstTokenMs != nil && *firstTokenMs > 0 {
		state.TTFTEMA = updateOpenAIAdaptiveEMA(state.TTFTEMA, float64(*firstTokenMs), cfg.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	}
	if durationMs > 0 {
		state.LatencyEMA = updateOpenAIAdaptiveEMA(state.LatencyEMA, float64(durationMs), cfg.OpenAIAdaptiveSchedulerLatencyEMAAlpha)
	}
	if success {
		state.ThompsonAlpha++
		state.ConsecutiveSuccess++
		state.ConsecutiveFailure = 0
		state.ConsecutiveCapacityFailure = 0
		state.LastSuccessAt = now
		if state.SuccessEMA >= cfg.OpenAIAdaptiveSchedulerCapacitySuccessThreshold &&
			state.ConsecutiveSuccess >= state.EstimatedCapacity &&
			state.EstimatedCapacity < math.MaxInt-cfg.OpenAIAdaptiveSchedulerCapacityIncreaseStep {
			state.EstimatedCapacity = nextOpenAIAdaptiveGrowthCapacity(state.EstimatedCapacity, cfg)
			state.ConsecutiveSuccess = 0
		}
		return
	}
	state.ThompsonBeta++
	state.ConsecutiveFailure++
	state.ConsecutiveCapacityFailure++
	state.ConsecutiveSuccess = 0
	state.RecentFailures++
	state.LastFailureAt = now
	if shouldShrinkOpenAIAdaptiveCapacity(state, cfg) {
		state.EstimatedCapacity = nextOpenAIAdaptiveShrinkCapacity(state, cfg)
		state.LastCapacityFailureAt = now
		cooldown := openAIAdaptiveCooldownDurationForState(*state, cfg)
		if cooldown > 0 {
			state.CooldownUntil = now.Add(cooldown)
		}
	}
}

func (s *openAIAdaptiveSchedulerStateStore) applyCooldown(
	accountID int64,
	cfg OpenAIAdaptiveSchedulerSettings,
	reason string,
	now time.Time,
) {
	if s == nil || accountID <= 0 {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	cooldown := openAIAdaptiveCooldownDurationForState(openAIAdaptiveAccountState{}, cfg)
	if cooldown <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[accountID]
	if state == nil {
		initial := defaultOpenAIAdaptiveAccountState(accountID, cfg)
		state = &initial
		s.states[accountID] = state
	}
	cooldown = openAIAdaptiveCooldownDurationForState(*state, cfg)
	if cooldown <= 0 {
		return
	}
	until := now.Add(cooldown)
	if state.CooldownUntil.Before(until) {
		state.CooldownUntil = until
	}
	if state.LastCapacityFailureAt.Before(now) {
		state.LastCapacityFailureAt = now
	}
	slog.Info("openai_adaptive_scheduler_cooldown_applied",
		"account_id", accountID,
		"reason", reason,
		"cooldown_until", state.CooldownUntil,
		"consecutive_capacity_failure", state.ConsecutiveCapacityFailure,
	)
}

func openAIAdaptiveCooldownDurationForState(state openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings) time.Duration {
	cooldown := time.Duration(cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds) * time.Second
	if cooldown <= 0 {
		return 0
	}
	if cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds <= 0 {
		return cooldown
	}
	maxCooldown := time.Duration(cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds) * time.Second
	failuresOverThreshold := state.ConsecutiveCapacityFailure - cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold
	for i := 0; i < failuresOverThreshold && cooldown < maxCooldown; i++ {
		cooldown *= 2
		if cooldown > maxCooldown {
			return maxCooldown
		}
	}
	if cooldown > maxCooldown {
		return maxCooldown
	}
	return cooldown
}

func (s *openAIAdaptiveSchedulerStateStore) observeLoad(
	account *Account,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadInfo *AccountLoadInfo,
) openAIAdaptiveAccountState {
	if s == nil || account == nil || account.ID <= 0 {
		return defaultOpenAIAdaptiveAccountState(0, cfg)
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[account.ID]
	if state == nil {
		initial := defaultOpenAIAdaptiveAccountState(account.ID, cfg)
		state = &initial
		s.states[account.ID] = state
	}
	stableCapacity := stableOpenAIAdaptiveCapacity(account, *state, cfg)
	if state.TotalSamples == 0 && state.EstimatedCapacity < stableCapacity {
		state.EstimatedCapacity = stableCapacity
	}
	if state.CooldownUntil.After(now) ||
		state.ConsecutiveCapacityFailure > 0 ||
		!shouldProbeOpenAIAdaptiveCapacity(loadInfo, stableCapacity, cfg) ||
		state.SuccessEMA < cfg.OpenAIAdaptiveSchedulerCapacitySuccessThreshold ||
		state.ConsecutiveSuccess < cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink {
		return *state
	}
	nextCapacity := capOpenAIAdaptiveCapacity(account, nextOpenAIAdaptiveGrowthCapacity(stableCapacity, cfg))
	if nextCapacity > state.EstimatedCapacity {
		state.EstimatedCapacity = nextCapacity
		state.ConsecutiveSuccess = 0
	}
	return *state
}

func refreshOpenAIAdaptiveLearningWindow(state *openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings, now time.Time) {
	if state == nil {
		return
	}
	windowSeconds := cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds
	if windowSeconds <= 0 {
		if state.RecentWindowStartedAt.IsZero() {
			state.RecentWindowStartedAt = now
		}
		return
	}
	window := time.Duration(windowSeconds) * time.Second
	if state.RecentWindowStartedAt.IsZero() {
		state.RecentWindowStartedAt = now
		return
	}
	if now.Sub(state.RecentWindowStartedAt) >= window {
		state.RecentWindowStartedAt = now
		state.RecentSamples = 0
		state.RecentFailures = 0
	}
}

func shouldShrinkOpenAIAdaptiveCapacity(state *openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings) bool {
	if state == nil {
		return false
	}
	if state.ConsecutiveCapacityFailure < cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold {
		return false
	}
	if state.RecentSamples < cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink {
		return false
	}
	recentFailureRate := 0.0
	if state.RecentSamples > 0 {
		recentFailureRate = float64(state.RecentFailures) / float64(state.RecentSamples)
	}
	return state.ErrorEMA >= cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold ||
		recentFailureRate >= cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold
}

func nextOpenAIAdaptiveShrinkCapacity(state *openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings) int {
	if state == nil {
		return cfg.OpenAIAdaptiveSchedulerMinCapacity
	}
	factor := cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft
	recentFailureRate := 0.0
	if state.RecentSamples > 0 {
		recentFailureRate = float64(state.RecentFailures) / float64(state.RecentSamples)
	}
	hardConsecutiveThreshold := cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold * 2
	hardErrorThreshold := cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold * 2
	if hardErrorThreshold > 1 {
		hardErrorThreshold = 1
	}
	if state.ConsecutiveCapacityFailure >= hardConsecutiveThreshold ||
		state.ErrorEMA >= hardErrorThreshold ||
		recentFailureRate >= hardErrorThreshold {
		factor = cfg.OpenAIAdaptiveSchedulerShrinkFactorHard
	}
	nextCapacity := int(math.Floor(float64(state.EstimatedCapacity) * factor))
	if nextCapacity < cfg.OpenAIAdaptiveSchedulerMinCapacity {
		nextCapacity = cfg.OpenAIAdaptiveSchedulerMinCapacity
	}
	return nextCapacity
}

func defaultOpenAIAdaptiveAccountState(accountID int64, cfg OpenAIAdaptiveSchedulerSettings) openAIAdaptiveAccountState {
	return openAIAdaptiveAccountState{
		AccountID:          accountID,
		EstimatedCapacity:  cfg.OpenAIAdaptiveSchedulerMinCapacity,
		SuccessEMA:         0.5,
		ErrorEMA:           0,
		ThompsonAlpha:      cfg.OpenAIAdaptiveSchedulerThompsonPriorAlpha,
		ThompsonBeta:       cfg.OpenAIAdaptiveSchedulerThompsonPriorBeta,
		ConsecutiveSuccess: 0,
	}
}

func defaultOpenAIAdaptiveAccountStateForAccount(account *Account, accountID int64, cfg OpenAIAdaptiveSchedulerSettings) openAIAdaptiveAccountState {
	if account != nil && account.ID > 0 {
		accountID = account.ID
	}
	state := defaultOpenAIAdaptiveAccountState(accountID, cfg)
	if account != nil {
		state.EstimatedCapacity = initialOpenAIAdaptiveCapacityForAccount(account, cfg)
	}
	return state
}

func updateOpenAIAdaptiveEMA(current float64, sample float64, alpha float64) float64 {
	if alpha <= 0 {
		return current
	}
	if alpha > 1 {
		alpha = 1
	}
	if current == 0 && sample > 0 {
		return sample
	}
	return alpha*sample + (1-alpha)*current
}

func effectiveOpenAIAdaptiveCapacity(account *Account, state openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings) int {
	return effectiveOpenAIAdaptiveCapacityWithLoad(account, state, cfg, nil)
}

func effectiveOpenAIAdaptiveCapacityWithLoad(
	account *Account,
	state openAIAdaptiveAccountState,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadInfo *AccountLoadInfo,
) int {
	stable := stableOpenAIAdaptiveCapacity(account, state, cfg)
	effective := stable
	now := time.Now()
	if shouldUseOpenAIAdaptiveHalfOpenProbe(state, cfg, now) {
		if cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity < effective {
			effective = cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity
		}
		return capOpenAIAdaptiveCapacity(account, effective)
	}
	if shouldProbeOpenAIAdaptiveCapacity(loadInfo, stable, cfg) && cfg.OpenAIAdaptiveSchedulerBurstProbeRatio > 0 {
		burstCapacity := int(math.Ceil(float64(stable) * cfg.OpenAIAdaptiveSchedulerBurstProbeRatio))
		if burstCapacity < 1 {
			burstCapacity = 1
		}
		if stable <= math.MaxInt-burstCapacity {
			effective = stable + burstCapacity
		}
	}
	return capOpenAIAdaptiveCapacity(account, effective)
}

func shouldUseOpenAIAdaptiveHalfOpenProbe(state openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings, now time.Time) bool {
	threshold := cfg.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold
	if threshold <= 0 {
		threshold = 1
	}
	return state.ConsecutiveCapacityFailure >= threshold && !state.CooldownUntil.After(now)
}

func stableOpenAIAdaptiveCapacity(account *Account, state openAIAdaptiveAccountState, cfg OpenAIAdaptiveSchedulerSettings) int {
	estimated := state.EstimatedCapacity
	if estimated <= 0 {
		estimated = cfg.OpenAIAdaptiveSchedulerMinCapacity
	}
	if estimated < cfg.OpenAIAdaptiveSchedulerMinCapacity {
		estimated = cfg.OpenAIAdaptiveSchedulerMinCapacity
	}
	initial := initialOpenAIAdaptiveCapacityForAccount(account, cfg)
	if state.TotalSamples == 0 && estimated < initial {
		estimated = initial
	}
	return capOpenAIAdaptiveCapacity(account, estimated)
}

func initialOpenAIAdaptiveCapacityForAccount(account *Account, cfg OpenAIAdaptiveSchedulerSettings) int {
	initial := cfg.OpenAIAdaptiveSchedulerMinCapacity
	if account != nil && account.Concurrency > 0 && cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction > 0 {
		fractionCapacity := int(math.Ceil(float64(account.Concurrency) * cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction))
		if fractionCapacity > initial {
			initial = fractionCapacity
		}
	}
	return capOpenAIAdaptiveCapacity(account, initial)
}

func capOpenAIAdaptiveCapacity(account *Account, capacity int) int {
	if capacity <= 0 {
		return capacity
	}
	if account != nil && account.Concurrency > 0 && account.Concurrency < capacity {
		return account.Concurrency
	}
	return capacity
}

func nextOpenAIAdaptiveGrowthCapacity(current int, cfg OpenAIAdaptiveSchedulerSettings) int {
	if current < cfg.OpenAIAdaptiveSchedulerMinCapacity {
		current = cfg.OpenAIAdaptiveSchedulerMinCapacity
	}
	additive := current
	if current <= math.MaxInt-cfg.OpenAIAdaptiveSchedulerCapacityIncreaseStep {
		additive = current + cfg.OpenAIAdaptiveSchedulerCapacityIncreaseStep
	}
	multiplicative := additive
	if cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor > 1 {
		value := math.Ceil(float64(current) * cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
		if value > float64(math.MaxInt) {
			multiplicative = math.MaxInt
		} else {
			multiplicative = int(value)
		}
	}
	if multiplicative > additive {
		return multiplicative
	}
	return additive
}

func shouldProbeOpenAIAdaptiveCapacity(loadInfo *AccountLoadInfo, stableCapacity int, cfg OpenAIAdaptiveSchedulerSettings) bool {
	if loadInfo == nil {
		return cfg.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold <= 0
	}
	if loadInfo.WaitingCount > 0 {
		return true
	}
	threshold := cfg.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold * 100
	if threshold <= 0 {
		return true
	}
	return adaptiveLoadRate(loadInfo, stableCapacity) >= threshold
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

func groupOpenAIAdaptiveCandidatesByAccountTypePriority(
	candidates []openAIAdaptiveCandidateScore,
	cfg OpenAIAdaptiveSchedulerSettings,
) [][]openAIAdaptiveCandidateScore {
	groupsByRank := make(map[int][]openAIAdaptiveCandidateScore, 3)
	ranks := make([]int, 0, 3)
	for _, candidate := range candidates {
		rank := openAIAdaptiveAccountTypePriorityRank(candidate.account, cfg)
		if _, ok := groupsByRank[rank]; !ok {
			ranks = append(ranks, rank)
		}
		groupsByRank[rank] = append(groupsByRank[rank], candidate)
	}
	sort.Ints(ranks)
	groups := make([][]openAIAdaptiveCandidateScore, 0, len(ranks))
	for _, rank := range ranks {
		groups = append(groups, groupsByRank[rank])
	}
	return groups
}

func isOpenAIAdaptiveCandidateBetter(left openAIAdaptiveCandidateScore, right openAIAdaptiveCandidateScore, cfg OpenAIAdaptiveSchedulerSettings) bool {
	if leftRank, rightRank := openAIAdaptiveAccountTypePriorityRank(left.account, cfg), openAIAdaptiveAccountTypePriorityRank(right.account, cfg); leftRank != rightRank {
		return leftRank < rightRank
	}
	if left.score != right.score {
		return left.score > right.score
	}
	if left.account.Priority != right.account.Priority {
		return left.account.Priority < right.account.Priority
	}
	leftLoad := adaptiveLoadRate(left.loadInfo, left.effectiveCapacity)
	rightLoad := adaptiveLoadRate(right.loadInfo, right.effectiveCapacity)
	if leftLoad != rightLoad {
		return leftLoad < rightLoad
	}
	if left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
		return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
	}
	return left.account.ID < right.account.ID
}

func openAIAdaptiveAccountTypePriorityRank(account *Account, cfg OpenAIAdaptiveSchedulerSettings) int {
	switch cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode {
	case openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst:
		if account != nil && account.IsOAuth() {
			return 0
		}
		if account != nil && account.Type == AccountTypeAPIKey {
			return 1
		}
		return 2
	case openAIAdaptiveSchedulerAccountTypePriorityAPIKeyFirst:
		if account != nil && account.Type == AccountTypeAPIKey {
			return 0
		}
		if account != nil && account.IsOAuth() {
			return 1
		}
		return 2
	default:
		return 0
	}
}
