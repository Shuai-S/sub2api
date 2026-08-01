package service

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	geminiAdaptiveLearningDefaultLimit = 50
	geminiAdaptiveLearningMaxLimit     = 500

	GeminiAdaptiveLearningStatusDisabled     = "disabled"
	GeminiAdaptiveLearningStatusUnavailable  = "unavailable"
	GeminiAdaptiveLearningStatusQuotaLimited = "quota_limited"
	GeminiAdaptiveLearningStatusCooldown     = "cooldown"
	GeminiAdaptiveLearningStatusHighError    = "high_error"
	GeminiAdaptiveLearningStatusSaturated    = "saturated"
	GeminiAdaptiveLearningStatusLearning     = "learning"
	GeminiAdaptiveLearningStatusUnlearned    = "unlearned"
	GeminiAdaptiveLearningStatusHealthy      = "healthy"
)

type GeminiAdaptiveSchedulerLearningSnapshot struct {
	Enabled         bool      `json:"enabled"`
	Mode            string    `json:"mode"`
	RealtimeEnabled bool      `json:"realtime_enabled"`
	GeneratedAt     time.Time `json:"generated_at"`
	RequestedModel  string    `json:"requested_model,omitempty"`
	ModelFamily     string    `json:"model_family"`
	TimeRange       string    `json:"time_range,omitempty"`
	StartTime       time.Time `json:"start_time,omitempty"`
	EndTime         time.Time `json:"end_time,omitempty"`

	TotalAccounts    int    `json:"total_accounts"`
	Total            int    `json:"total"`
	ReturnedAccounts int    `json:"returned_accounts"`
	Limit            int    `json:"limit"`
	Page             int    `json:"page,omitempty"`
	PageSize         int    `json:"page_size,omitempty"`
	TopN             int    `json:"top_n,omitempty"`
	SortBy           string `json:"sort_by,omitempty"`
	SortOrder        string `json:"sort_order,omitempty"`

	Settings GeminiAdaptiveSchedulerLearningSettingsSnapshot  `json:"settings"`
	Metrics  GeminiAdaptiveMetricsSnapshot                    `json:"metrics"`
	Summary  GeminiAdaptiveSchedulerLearningSummary           `json:"summary"`
	Accounts []GeminiAdaptiveSchedulerAccountLearningSnapshot `json:"accounts"`
}

type GeminiAdaptiveSchedulerLearningFilter struct {
	GroupID        *int64
	RequestedModel string
	TimeRange      string
	StartTime      time.Time
	EndTime        time.Time
	TopN           int
	Page           int
	PageSize       int
	Status         string
	SortBy         string
	SortOrder      string
}

func (f *GeminiAdaptiveSchedulerLearningFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type GeminiAdaptiveSchedulerLearningSettingsSnapshot struct {
	StickyEscapeOnCapacityFull bool    `json:"sticky_escape_on_capacity_full"`
	TopK                       int     `json:"top_k"`
	SoftmaxTemperature         float64 `json:"softmax_temperature"`
	WeightReliability          float64 `json:"weight_reliability"`
	WeightQuota                float64 `json:"weight_quota"`
	WeightCapacity             float64 `json:"weight_capacity"`
	WeightLatency              float64 `json:"weight_latency"`
	WeightCost                 float64 `json:"weight_cost"`
	WeightExploration          float64 `json:"weight_exploration"`
	InitialReliability         float64 `json:"initial_reliability"`
	NeutralLatencyScore        float64 `json:"neutral_latency_score"`
	NeutralQuotaScore          float64 `json:"neutral_quota_score"`
	CapacityFailureThreshold   int     `json:"capacity_failure_threshold"`
	MinRecentSamplesForShrink  int     `json:"min_recent_samples_for_shrink"`
	ShrinkErrorThreshold       float64 `json:"shrink_error_threshold"`
	LearningWindowSeconds      int     `json:"learning_window_seconds"`
	CooldownSeconds            int     `json:"cooldown_seconds"`
	CapacityIncreaseStep       int     `json:"capacity_increase_step"`
	MinCapacity                int     `json:"min_capacity"`
	DiagnosticLogEnabled       bool    `json:"diagnostic_log_enabled"`
	DiagnosticLogSampleRate    float64 `json:"diagnostic_log_sample_rate"`
}

type GeminiAdaptiveSchedulerLearningSummary struct {
	TrackedAccounts      int `json:"tracked_accounts"`
	DisabledAccounts     int `json:"disabled_accounts"`
	UnavailableAccounts  int `json:"unavailable_accounts"`
	QuotaLimitedAccounts int `json:"quota_limited_accounts"`
	CooldownAccounts     int `json:"cooldown_accounts"`
	HighErrorAccounts    int `json:"high_error_accounts"`
	SaturatedAccounts    int `json:"saturated_accounts"`
	LearningAccounts     int `json:"learning_accounts"`
	UnlearnedAccounts    int `json:"unlearned_accounts"`
	HealthyAccounts      int `json:"healthy_accounts"`
}

type GeminiAdaptiveModelLearningSnapshot struct {
	ModelFamily string  `json:"model_family"`
	SuccessEMA  float64 `json:"success_ema"`
	TTFTEMA     float64 `json:"ttft_ema"`
	LatencyEMA  float64 `json:"latency_ema"`
	Samples     int64   `json:"samples"`
	Failures    int64   `json:"failures"`
}

type GeminiAdaptiveSchedulerAccountLearningSnapshot struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	Platform      string `json:"platform"`
	Type          string `json:"type"`
	AccountStatus string `json:"account_status"`
	Schedulable   bool   `json:"schedulable"`
	Priority      int    `json:"priority"`

	ConfiguredConcurrency int     `json:"configured_concurrency"`
	EstimatedCapacity     int     `json:"estimated_capacity"`
	EffectiveCapacity     int     `json:"effective_capacity"`
	RateMultiplier        float64 `json:"rate_multiplier"`
	CurrentConcurrency    int     `json:"current_concurrency"`
	WaitingCount          int     `json:"waiting_count"`
	LoadPercentage        float64 `json:"load_percentage"`

	SchedulerStatus string `json:"scheduler_status"`
	StatusReason    string `json:"status_reason,omitempty"`
	Learned         bool   `json:"learned"`

	SchedulerScore   float64 `json:"scheduler_score"`
	ReliabilityScore float64 `json:"reliability_score"`
	QuotaScore       float64 `json:"quota_score"`
	CapacityScore    float64 `json:"capacity_score"`
	LatencyScore     float64 `json:"latency_score"`
	CostScore        float64 `json:"cost_score"`
	ExplorationScore float64 `json:"exploration_score"`

	PathSuccessEMA  float64                               `json:"path_success_ema"`
	ModelFamily     string                                `json:"model_family"`
	ModelSuccessEMA float64                               `json:"model_success_ema"`
	TTFTEMA         float64                               `json:"ttft_ema"`
	LatencyEMA      float64                               `json:"latency_ema"`
	ModelSamples    int64                                 `json:"model_samples"`
	ModelFailures   int64                                 `json:"model_failures"`
	ByModelFamily   []GeminiAdaptiveModelLearningSnapshot `json:"by_model_family"`

	Quota GeminiAdaptiveQuotaSnapshot `json:"quota"`

	TotalSamples               int64   `json:"total_samples"`
	RecentHealthSamples        int     `json:"recent_health_samples"`
	RecentHealthFailures       int     `json:"recent_health_failures"`
	RecentHealthFailureRate    float64 `json:"recent_health_failure_rate"`
	RecentCapacitySamples      int     `json:"recent_capacity_samples"`
	RecentCapacityFailures     int     `json:"recent_capacity_failures"`
	RecentCapacityFailureRate  float64 `json:"recent_capacity_failure_rate"`
	ConsecutiveSuccess         int     `json:"consecutive_success"`
	ConsecutiveFailure         int     `json:"consecutive_failure"`
	ConsecutiveCapacityFailure int     `json:"consecutive_capacity_failure"`

	LearningWindowStartedAt *time.Time `json:"learning_window_started_at,omitempty"`
	LastSuccessAt           *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt           *time.Time `json:"last_failure_at,omitempty"`
	LastCapacityFailureAt   *time.Time `json:"last_capacity_failure_at,omitempty"`
	CooldownUntil           *time.Time `json:"cooldown_until,omitempty"`
	CooldownRemainingSec    int64      `json:"cooldown_remaining_sec"`
}

func (s *OpsService) GetGeminiAdaptiveSchedulerLearningSnapshot(ctx context.Context, filter *GeminiAdaptiveSchedulerLearningFilter) (*GeminiAdaptiveSchedulerLearningSnapshot, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil {
		filter = &GeminiAdaptiveSchedulerLearningFilter{}
	}
	normalizeGeminiAdaptiveLearningFilter(filter)
	limit := filter.TopN
	if !filter.IsTopNMode() {
		limit = filter.PageSize
	}

	cfg := DefaultGeminiAdaptiveSchedulerSettings()
	var stateStore *geminiAdaptiveStateStore
	metrics := GeminiAdaptiveMetricsSnapshot{}
	if s != nil && s.gatewayService != nil {
		cfg = s.gatewayService.geminiAdaptiveSchedulerSettingsForSnapshot(ctx)
		stateStore = s.gatewayService.geminiAdaptiveSchedulerStateStoreForSnapshot()
		if s.gatewayService.geminiAdaptiveScheduler != nil {
			metrics = s.gatewayService.geminiAdaptiveScheduler.SnapshotMetrics()
		}
	}
	realtimeEnabled := s.IsRealtimeMonitoringEnabled(ctx)
	accounts, err := s.listAllAccountsForOps(ctx, PlatformGemini, filter.GroupID)
	if err != nil {
		return nil, err
	}
	accounts = filterGeminiAdaptiveLearningAccountsByGroup(accounts, filter.GroupID)
	accounts = filterGeminiAdaptiveLearningSchedulableAccounts(accounts)

	now := time.Now()
	states := make(map[int64]geminiAdaptiveAccountState, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	accountPtrs := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		state := defaultGeminiAdaptiveAccountState(account, now, cfg)
		if stateStore != nil {
			state = stateStore.snapshot(account, cfg)
		}
		states[account.ID] = state
		loadReq = append(loadReq, AccountWithConcurrency{ID: account.ID, MaxConcurrency: normalizedGeminiAdaptiveCapacity(account, state)})
		accountPtrs = append(accountPtrs, account)
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if realtimeEnabled {
		loadMap = s.getGeminiAdaptiveLearningLoadMapBestEffort(ctx, loadReq)
	}
	quotaMap := map[int64]GeminiAdaptiveQuotaSnapshot{}
	if filter.RequestedModel != "" && s != nil && s.gatewayService != nil && s.gatewayService.rateLimitService != nil {
		_, snapshots, quotaErr := s.gatewayService.rateLimitService.PreCheckUsageBatchWithSnapshots(ctx, accountPtrs, filter.RequestedModel)
		if quotaErr != nil {
			log.Printf("[Ops] Gemini adaptive quota snapshot failed: %v", quotaErr)
		} else {
			quotaMap = snapshots
		}
	}

	rows := make([]GeminiAdaptiveSchedulerAccountLearningSnapshot, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		rows = append(rows, buildGeminiAdaptiveLearningAccountSnapshot(account, states[account.ID], cfg, loadInfo, quotaMap[account.ID], filter.RequestedModel, now))
	}
	applyGeminiAdaptiveLearningScores(rows, accounts, states, loadMap, quotaMap, filter.RequestedModel, cfg, now)
	rows = filterGeminiAdaptiveLearningRowsByStatus(rows, filter.Status)
	rows = filterGeminiAdaptiveLearningRowsByTime(rows, filter.StartTime, filter.EndTime)
	sortGeminiAdaptiveLearningRows(rows, filter.SortBy, filter.SortOrder)

	summary := summarizeGeminiAdaptiveLearningRows(rows)
	total := len(rows)
	if filter.IsTopNMode() {
		if len(rows) > filter.TopN {
			rows = rows[:filter.TopN]
		}
	} else {
		start := (filter.Page - 1) * filter.PageSize
		if start >= len(rows) {
			rows = nil
		} else {
			end := min(start+filter.PageSize, len(rows))
			rows = rows[start:end]
		}
	}

	return &GeminiAdaptiveSchedulerLearningSnapshot{
		Enabled: cfg.GeminiAdaptiveSchedulerEnabled, Mode: cfg.GeminiAdaptiveSchedulerMode,
		RealtimeEnabled: realtimeEnabled, GeneratedAt: now.UTC(), RequestedModel: filter.RequestedModel,
		ModelFamily: geminiAdaptiveModelFamily(filter.RequestedModel, "generateContent"), TimeRange: filter.TimeRange,
		StartTime: filter.StartTime.UTC(), EndTime: filter.EndTime.UTC(), TotalAccounts: total, Total: total,
		ReturnedAccounts: len(rows), Limit: limit, Page: filter.Page, PageSize: filter.PageSize, TopN: filter.TopN,
		SortBy: filter.SortBy, SortOrder: filter.SortOrder, Settings: geminiAdaptiveLearningSettingsSnapshot(cfg),
		Metrics: metrics, Summary: summary, Accounts: rows,
	}, nil
}

func (s *GatewayService) geminiAdaptiveSchedulerSettingsForSnapshot(ctx context.Context) GeminiAdaptiveSchedulerSettings {
	defaults := DefaultGeminiAdaptiveSchedulerSettings()
	if s == nil || s.settingService == nil {
		return defaults
	}
	settings, err := s.settingService.GetGeminiAdaptiveSchedulerSettings(ctx)
	if err != nil {
		log.Printf("[Ops] Gemini adaptive settings lookup failed: %v", err)
		return defaults
	}
	return NormalizeGeminiAdaptiveSchedulerSettings(settings)
}

func (s *GatewayService) geminiAdaptiveSchedulerStateStoreForSnapshot() *geminiAdaptiveStateStore {
	if s == nil || s.geminiAdaptiveScheduler == nil {
		return nil
	}
	return s.geminiAdaptiveScheduler.state
}

func (s *OpsService) getGeminiAdaptiveLearningLoadMapBestEffort(ctx context.Context, accounts []AccountWithConcurrency) map[int64]*AccountLoadInfo {
	if s == nil || s.concurrencyService == nil || len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}
	}
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	for i := 0; i < len(accounts); i += opsConcurrencyBatchChunkSize {
		end := min(i+opsConcurrencyBatchChunkSize, len(accounts))
		part, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accounts[i:end])
		if err != nil {
			log.Printf("[Ops] Gemini adaptive learning GetAccountsLoadBatch failed: %v", err)
			continue
		}
		for key, value := range part {
			out[key] = value
		}
	}
	return out
}

func filterGeminiAdaptiveLearningAccountsByGroup(accounts []Account, groupID *int64) []Account {
	if groupID == nil || *groupID <= 0 {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		for _, group := range account.Groups {
			if group != nil && group.ID == *groupID {
				out = append(out, account)
				break
			}
		}
	}
	return out
}

func filterGeminiAdaptiveLearningSchedulableAccounts(accounts []Account) []Account {
	out := accounts[:0]
	for _, account := range accounts {
		if account.Schedulable {
			out = append(out, account)
		}
	}
	return out
}

func normalizedGeminiAdaptiveCapacity(account *Account, state geminiAdaptiveAccountState) int {
	if account == nil || account.Concurrency <= 0 {
		return 0
	}
	capacity := state.EstimatedCapacity
	if capacity <= 0 || capacity > account.Concurrency {
		capacity = account.Concurrency
	}
	return capacity
}

func buildGeminiAdaptiveLearningAccountSnapshot(account *Account, state geminiAdaptiveAccountState, cfg GeminiAdaptiveSchedulerSettings, load *AccountLoadInfo, quota GeminiAdaptiveQuotaSnapshot, requestedModel string, now time.Time) GeminiAdaptiveSchedulerAccountLearningSnapshot {
	if load == nil {
		load = &AccountLoadInfo{}
	}
	capacity := normalizedGeminiAdaptiveCapacity(account, state)
	healthFailureRate := adaptiveFailureRate(state.RecentHealthFailures, state.RecentHealthSamples)
	capacityFailureRate := adaptiveFailureRate(state.RecentCapacityFailures, state.RecentCapacitySamples)
	status, reason := geminiAdaptiveLearningAccountStatus(account, state, cfg, load, quota, capacity, capacityFailureRate, now)
	cooldownRemaining := int64(0)
	if state.CooldownUntil.After(now) {
		cooldownRemaining = int64(state.CooldownUntil.Sub(now).Seconds())
		if cooldownRemaining < 1 {
			cooldownRemaining = 1
		}
	}
	family := geminiAdaptiveModelFamily(requestedModel, "generateContent")
	modelState := state.ByModelFamily[family]
	return GeminiAdaptiveSchedulerAccountLearningSnapshot{
		AccountID: account.ID, AccountName: account.Name, Platform: account.Platform, Type: account.Type,
		AccountStatus: account.Status, Schedulable: account.IsSchedulable(), Priority: account.Priority,
		ConfiguredConcurrency: account.Concurrency, EstimatedCapacity: capacity, EffectiveCapacity: capacity,
		RateMultiplier:     account.BillingRateMultiplier(),
		CurrentConcurrency: load.CurrentConcurrency, WaitingCount: load.WaitingCount, LoadPercentage: adaptiveLoadRate(load, capacity),
		SchedulerStatus: status, StatusReason: reason, Learned: state.TotalSamples > 0,
		PathSuccessEMA: state.PathSuccessEMA, ModelFamily: family, ModelSuccessEMA: modelState.SuccessEMA,
		TTFTEMA: modelState.TTFTEMA, LatencyEMA: modelState.LatencyEMA, ModelSamples: modelState.Samples,
		ModelFailures: modelState.Failures, ByModelFamily: geminiAdaptiveModelLearningSnapshots(state.ByModelFamily), Quota: quota,
		TotalSamples: state.TotalSamples, RecentHealthSamples: state.RecentHealthSamples,
		RecentHealthFailures: state.RecentHealthFailures, RecentHealthFailureRate: healthFailureRate,
		RecentCapacitySamples: state.RecentCapacitySamples, RecentCapacityFailures: state.RecentCapacityFailures,
		RecentCapacityFailureRate: capacityFailureRate, ConsecutiveSuccess: state.ConsecutiveSuccess,
		ConsecutiveFailure: state.ConsecutiveFailure, ConsecutiveCapacityFailure: state.ConsecutiveCapacityFailure,
		LearningWindowStartedAt: geminiAdaptiveTimePtr(state.RecentWindowStartedAt), LastSuccessAt: geminiAdaptiveTimePtr(state.LastSuccessAt),
		LastFailureAt: geminiAdaptiveTimePtr(state.LastFailureAt), LastCapacityFailureAt: geminiAdaptiveTimePtr(state.LastCapacityFailureAt),
		CooldownUntil: geminiAdaptiveTimePtr(state.CooldownUntil), CooldownRemainingSec: cooldownRemaining,
	}
}

func geminiAdaptiveLearningAccountStatus(account *Account, state geminiAdaptiveAccountState, cfg GeminiAdaptiveSchedulerSettings, load *AccountLoadInfo, quota GeminiAdaptiveQuotaSnapshot, capacity int, capacityFailureRate float64, now time.Time) (string, string) {
	if !cfg.GeminiAdaptiveSchedulerEnabled {
		return GeminiAdaptiveLearningStatusDisabled, "adaptive scheduler disabled"
	}
	if account == nil || !account.IsSchedulable() {
		if account != nil && account.ErrorMessage != "" {
			return GeminiAdaptiveLearningStatusUnavailable, account.ErrorMessage
		}
		return GeminiAdaptiveLearningStatusUnavailable, "account is not schedulable"
	}
	if quota.HardRejected {
		return GeminiAdaptiveLearningStatusQuotaLimited, "local Gemini quota precheck rejected the account"
	}
	if state.CooldownUntil.After(now) {
		return GeminiAdaptiveLearningStatusCooldown, "adaptive cooldown after concurrency failures"
	}
	if (state.RecentCapacitySamples > 0 && capacityFailureRate >= cfg.GeminiAdaptiveSchedulerShrinkErrorThreshold) || state.ConsecutiveCapacityFailure >= cfg.GeminiAdaptiveSchedulerCapacityFailureThreshold {
		return GeminiAdaptiveLearningStatusHighError, "concurrency failure signal reached shrink threshold"
	}
	if capacity > 0 && load != nil && (load.CurrentConcurrency >= capacity || load.WaitingCount > 0) {
		return GeminiAdaptiveLearningStatusSaturated, "current load reached adaptive capacity"
	}
	if state.TotalSamples == 0 {
		return GeminiAdaptiveLearningStatusUnlearned, "no runtime samples yet"
	}
	if state.TotalSamples < int64(cfg.GeminiAdaptiveSchedulerMinRecentSamplesForShrink) {
		return GeminiAdaptiveLearningStatusLearning, "sample count below shrink confidence threshold"
	}
	return GeminiAdaptiveLearningStatusHealthy, ""
}

func applyGeminiAdaptiveLearningScores(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot, accounts []Account, states map[int64]geminiAdaptiveAccountState, loads map[int64]*AccountLoadInfo, quotas map[int64]GeminiAdaptiveQuotaSnapshot, requestedModel string, cfg GeminiAdaptiveSchedulerSettings, now time.Time) {
	rowByID := make(map[int64]*GeminiAdaptiveSchedulerAccountLearningSnapshot, len(rows))
	for i := range rows {
		rowByID[rows[i].AccountID] = &rows[i]
	}
	candidates := make([]GeminiAdaptiveCandidate, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		row := rowByID[account.ID]
		if row == nil || row.SchedulerStatus == GeminiAdaptiveLearningStatusUnavailable {
			continue
		}
		load := loads[account.ID]
		if load == nil {
			load = &AccountLoadInfo{AccountID: account.ID}
		}
		candidates = append(candidates, GeminiAdaptiveCandidate{Account: account, Load: load, Quota: quotas[account.ID], EffectiveCapacity: row.EffectiveCapacity, state: states[account.ID]})
	}
	applyGeminiAdaptiveScores(candidates, requestedModel, "generateContent", false, now, cfg)
	for _, candidate := range candidates {
		row := rowByID[candidate.Account.ID]
		row.SchedulerScore, row.ReliabilityScore = candidate.Score, candidate.ReliabilityScore
		row.QuotaScore, row.CapacityScore = candidate.QuotaScore, candidate.CapacityScore
		row.LatencyScore, row.CostScore, row.ExplorationScore = candidate.LatencyScore, candidate.CostScore, candidate.ExplorationScore
	}
}

func geminiAdaptiveModelLearningSnapshots(states map[string]geminiAdaptiveModelState) []GeminiAdaptiveModelLearningSnapshot {
	out := make([]GeminiAdaptiveModelLearningSnapshot, 0, len(states))
	for family, state := range states {
		out = append(out, GeminiAdaptiveModelLearningSnapshot{ModelFamily: family, SuccessEMA: state.SuccessEMA, TTFTEMA: state.TTFTEMA, LatencyEMA: state.LatencyEMA, Samples: state.Samples, Failures: state.Failures})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModelFamily < out[j].ModelFamily })
	return out
}

func geminiAdaptiveTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func normalizeGeminiAdaptiveLearningFilter(filter *GeminiAdaptiveSchedulerLearningFilter) {
	filter.TopN = min(max(filter.TopN, 0), geminiAdaptiveLearningMaxLimit)
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = geminiAdaptiveLearningDefaultLimit
	}
	filter.PageSize = min(filter.PageSize, geminiAdaptiveLearningMaxLimit)
	filter.RequestedModel = strings.TrimSpace(filter.RequestedModel)
	filter.SortBy = normalizeGeminiAdaptiveLearningSortBy(filter.SortBy)
	filter.SortOrder = normalizeGeminiAdaptiveLearningSortOrder(filter.SortOrder)
	filter.Status = normalizeGeminiAdaptiveLearningStatusFilter(filter.Status)
}

func filterGeminiAdaptiveLearningRowsByStatus(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot, status string) []GeminiAdaptiveSchedulerAccountLearningSnapshot {
	status = normalizeGeminiAdaptiveLearningStatusFilter(status)
	if status == "" {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		if row.SchedulerStatus == status {
			out = append(out, row)
		}
	}
	return out
}

func filterGeminiAdaptiveLearningRowsByTime(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot, start, end time.Time) []GeminiAdaptiveSchedulerAccountLearningSnapshot {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		last := geminiAdaptiveLearningLastEventTime(row)
		if last.IsZero() || (!last.Before(start) && !last.After(end)) {
			out = append(out, row)
		}
	}
	return out
}

func sortGeminiAdaptiveLearningRows(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot, sortBy, sortOrder string) {
	sortBy, sortOrder = normalizeGeminiAdaptiveLearningSortBy(sortBy), normalizeGeminiAdaptiveLearningSortOrder(sortOrder)
	sort.SliceStable(rows, func(i, j int) bool {
		cmp := compareGeminiAdaptiveLearningRows(rows[i], rows[j], sortBy)
		if cmp == 0 {
			cmp = compareGeminiAdaptiveLearningRows(rows[i], rows[j], "default")
		}
		if sortBy != "" && sortOrder == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
}

func normalizeGeminiAdaptiveLearningSortBy(value string) string {
	switch strings.TrimSpace(value) {
	case "account", "status", "capacity", "load", "score", "samples", "error", "latency", "quota", "last_event":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeGeminiAdaptiveLearningSortOrder(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "asc") {
		return "asc"
	}
	return "desc"
}

func normalizeGeminiAdaptiveLearningStatusFilter(value string) string {
	switch strings.TrimSpace(value) {
	case GeminiAdaptiveLearningStatusDisabled, GeminiAdaptiveLearningStatusUnavailable, GeminiAdaptiveLearningStatusQuotaLimited,
		GeminiAdaptiveLearningStatusCooldown, GeminiAdaptiveLearningStatusHighError, GeminiAdaptiveLearningStatusSaturated,
		GeminiAdaptiveLearningStatusLearning, GeminiAdaptiveLearningStatusUnlearned, GeminiAdaptiveLearningStatusHealthy:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func compareGeminiAdaptiveLearningRows(left, right GeminiAdaptiveSchedulerAccountLearningSnapshot, sortBy string) int {
	switch sortBy {
	case "account":
		if cmp := strings.Compare(strings.ToLower(left.AccountName), strings.ToLower(right.AccountName)); cmp != 0 {
			return cmp
		}
	case "status":
		if cmp := compareInt(geminiAdaptiveLearningStatusRank(left.SchedulerStatus), geminiAdaptiveLearningStatusRank(right.SchedulerStatus)); cmp != 0 {
			return cmp
		}
	case "capacity":
		if cmp := compareInt(left.EffectiveCapacity, right.EffectiveCapacity); cmp != 0 {
			return cmp
		}
	case "load":
		if cmp := compareFloat64(left.LoadPercentage, right.LoadPercentage); cmp != 0 {
			return cmp
		}
	case "score":
		if cmp := compareFloat64(left.SchedulerScore, right.SchedulerScore); cmp != 0 {
			return cmp
		}
	case "samples":
		if cmp := compareInt64(left.TotalSamples, right.TotalSamples); cmp != 0 {
			return cmp
		}
	case "error":
		if cmp := compareFloat64(left.RecentHealthFailureRate, right.RecentHealthFailureRate); cmp != 0 {
			return cmp
		}
	case "latency":
		if cmp := compareFloat64(left.LatencyEMA, right.LatencyEMA); cmp != 0 {
			return cmp
		}
	case "quota":
		if cmp := compareFloat64(left.QuotaScore, right.QuotaScore); cmp != 0 {
			return cmp
		}
	case "last_event":
		if cmp := compareTime(geminiAdaptiveLearningLastEventTime(left), geminiAdaptiveLearningLastEventTime(right)); cmp != 0 {
			return cmp
		}
	default:
		if cmp := compareInt(geminiAdaptiveLearningStatusRank(left.SchedulerStatus), geminiAdaptiveLearningStatusRank(right.SchedulerStatus)); cmp != 0 {
			return cmp
		}
		if cmp := compareInt(left.Priority, right.Priority); cmp != 0 {
			return cmp
		}
		if cmp := compareFloat64(right.SchedulerScore, left.SchedulerScore); cmp != 0 {
			return cmp
		}
	}
	return compareInt64(left.AccountID, right.AccountID)
}

func geminiAdaptiveLearningLastEventTime(row GeminiAdaptiveSchedulerAccountLearningSnapshot) time.Time {
	values := []*time.Time{row.LastSuccessAt, row.LastFailureAt, row.LastCapacityFailureAt, row.CooldownUntil}
	latest := time.Time{}
	for _, value := range values {
		if value != nil && value.After(latest) {
			latest = *value
		}
	}
	return latest
}

func geminiAdaptiveLearningStatusRank(status string) int {
	switch status {
	case GeminiAdaptiveLearningStatusCooldown:
		return 0
	case GeminiAdaptiveLearningStatusHighError:
		return 1
	case GeminiAdaptiveLearningStatusQuotaLimited:
		return 2
	case GeminiAdaptiveLearningStatusSaturated:
		return 3
	case GeminiAdaptiveLearningStatusUnavailable:
		return 4
	case GeminiAdaptiveLearningStatusLearning:
		return 5
	case GeminiAdaptiveLearningStatusUnlearned:
		return 6
	case GeminiAdaptiveLearningStatusDisabled:
		return 7
	case GeminiAdaptiveLearningStatusHealthy:
		return 8
	default:
		return 9
	}
}

func summarizeGeminiAdaptiveLearningRows(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot) GeminiAdaptiveSchedulerLearningSummary {
	summary := GeminiAdaptiveSchedulerLearningSummary{TrackedAccounts: len(rows)}
	for _, row := range rows {
		switch row.SchedulerStatus {
		case GeminiAdaptiveLearningStatusDisabled:
			summary.DisabledAccounts++
		case GeminiAdaptiveLearningStatusUnavailable:
			summary.UnavailableAccounts++
		case GeminiAdaptiveLearningStatusQuotaLimited:
			summary.QuotaLimitedAccounts++
		case GeminiAdaptiveLearningStatusCooldown:
			summary.CooldownAccounts++
		case GeminiAdaptiveLearningStatusHighError:
			summary.HighErrorAccounts++
		case GeminiAdaptiveLearningStatusSaturated:
			summary.SaturatedAccounts++
		case GeminiAdaptiveLearningStatusLearning:
			summary.LearningAccounts++
		case GeminiAdaptiveLearningStatusUnlearned:
			summary.UnlearnedAccounts++
		case GeminiAdaptiveLearningStatusHealthy:
			summary.HealthyAccounts++
		}
	}
	return summary
}

func geminiAdaptiveLearningSettingsSnapshot(cfg GeminiAdaptiveSchedulerSettings) GeminiAdaptiveSchedulerLearningSettingsSnapshot {
	return GeminiAdaptiveSchedulerLearningSettingsSnapshot{
		StickyEscapeOnCapacityFull: cfg.GeminiAdaptiveSchedulerStickyEscapeOnCapacityFull, TopK: cfg.GeminiAdaptiveSchedulerTopK,
		SoftmaxTemperature: cfg.GeminiAdaptiveSchedulerSoftmaxTemperature, WeightReliability: cfg.GeminiAdaptiveSchedulerWeightReliability,
		WeightQuota: cfg.GeminiAdaptiveSchedulerWeightQuota, WeightCapacity: cfg.GeminiAdaptiveSchedulerWeightCapacity,
		WeightLatency: cfg.GeminiAdaptiveSchedulerWeightLatency, WeightCost: cfg.GeminiAdaptiveSchedulerWeightCost,
		WeightExploration: cfg.GeminiAdaptiveSchedulerWeightExploration, InitialReliability: cfg.GeminiAdaptiveSchedulerInitialReliability,
		NeutralLatencyScore: cfg.GeminiAdaptiveSchedulerNeutralLatencyScore, NeutralQuotaScore: cfg.GeminiAdaptiveSchedulerNeutralQuotaScore,
		CapacityFailureThreshold:  cfg.GeminiAdaptiveSchedulerCapacityFailureThreshold,
		MinRecentSamplesForShrink: cfg.GeminiAdaptiveSchedulerMinRecentSamplesForShrink,
		ShrinkErrorThreshold:      cfg.GeminiAdaptiveSchedulerShrinkErrorThreshold, LearningWindowSeconds: cfg.GeminiAdaptiveSchedulerLearningWindowSeconds,
		CooldownSeconds: cfg.GeminiAdaptiveSchedulerCooldownSeconds, CapacityIncreaseStep: cfg.GeminiAdaptiveSchedulerCapacityIncreaseStep,
		MinCapacity: cfg.GeminiAdaptiveSchedulerMinCapacity, DiagnosticLogEnabled: cfg.GeminiAdaptiveSchedulerDiagnosticLogEnabled,
		DiagnosticLogSampleRate: cfg.GeminiAdaptiveSchedulerDiagnosticLogSampleRate,
	}
}
