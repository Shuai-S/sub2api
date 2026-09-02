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
	TimeRange      string
	StartTime      time.Time
	EndTime        time.Time
	TopN           int
	Page           int
	PageSize       int
	LearningStatus string
	RuntimeStatus  string
	SortBy         string
	SortOrder      string
}

func (f *GeminiAdaptiveSchedulerLearningFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type GeminiAdaptiveSchedulerLearningSettingsSnapshot struct {
	TopK                      int     `json:"top_k"`
	SoftmaxTemperature        float64 `json:"softmax_temperature"`
	ExplorationRate           float64 `json:"exploration_rate"`
	ConsecutiveFailurePenalty float64 `json:"consecutive_failure_penalty"`
	WeightReliability         float64 `json:"weight_reliability"`
	WeightCapacity            float64 `json:"weight_capacity"`
	WeightLatency             float64 `json:"weight_ttft"`
	WeightCost                float64 `json:"weight_cost"`
	WeightCache               float64 `json:"weight_cache"`
	LearningWindowSeconds     int     `json:"learning_window_seconds"`
	LearningMinHealthSamples  int     `json:"learning_min_health_samples"`
	SuccessEMAAlpha           float64 `json:"success_ema_alpha"`
	TTFTEMAAlpha              float64 `json:"ttft_ema_alpha"`
	CooldownSeconds           int     `json:"cooldown_seconds"`
	CooldownMaxSeconds        int     `json:"cooldown_max_seconds"`
	AccountFailureThreshold   int     `json:"account_failure_threshold"`
	HighErrorMinSamples       int     `json:"high_error_min_samples"`
	HighErrorMaxSamples       int     `json:"high_error_max_samples"`
	HighErrorEnterRate        float64 `json:"high_error_enter_rate"`
	HighErrorExitRate         float64 `json:"high_error_exit_rate"`
	CapacityShrinkFactor      float64 `json:"capacity_shrink_factor"`
	CapacityGrowthFactor      float64 `json:"capacity_growth_factor"`
	CapacityRecoverySamples   int     `json:"capacity_recovery_samples"`
	CapacityRecoveryLoad      float64 `json:"capacity_recovery_load"`
	QuotaProbeIntervalSeconds int     `json:"quota_probe_interval_seconds"`
	DiagnosticLogEnabled      bool    `json:"diagnostic_log_enabled"`
	DiagnosticLogSampleRate   float64 `json:"diagnostic_log_sample_rate"`
}

type GeminiAdaptiveSchedulerLearningSummary struct {
	TrackedAccounts          int `json:"tracked_accounts"`
	DisabledAccounts         int `json:"disabled_accounts"`
	UnavailableAccounts      int `json:"unavailable_accounts"`
	QuotaLimitedAccounts     int `json:"quota_limited_accounts"`
	CooldownAccounts         int `json:"cooldown_accounts"`
	HighErrorAccounts        int `json:"high_error_accounts"`
	SaturatedAccounts        int `json:"saturated_accounts"`
	LearningAccounts         int `json:"learning_accounts"`
	UnlearnedAccounts        int `json:"unlearned_accounts"`
	HealthyAccounts          int `json:"healthy_accounts"`
	LearnedAccounts          int `json:"learned_accounts"`
	NotApplicableAccounts    int `json:"not_applicable_accounts"`
	HalfOpenAccounts         int `json:"half_open_accounts"`
	CircuitHalfOpenAccounts  int `json:"circuit_half_open_accounts"`
	CapacityRecoveryAccounts int `json:"capacity_recovery_accounts"`
}

type GeminiAdaptiveSchedulerAccountLearningSnapshot struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	Platform      string `json:"platform"`
	Type          string `json:"type"`
	AccountStatus string `json:"account_status"`
	Schedulable   bool   `json:"schedulable"`

	ConfiguredConcurrency int     `json:"configured_concurrency"`
	EffectiveCapacity     int     `json:"effective_capacity"`
	RateMultiplier        float64 `json:"rate_multiplier"`
	CurrentConcurrency    int     `json:"current_concurrency"`
	WaitingCount          int     `json:"waiting_count"`
	LoadPercentage        float64 `json:"load_percentage"`

	SchedulerStatus    string   `json:"scheduler_status"`
	StatusReason       string   `json:"status_reason,omitempty"`
	Learned            bool     `json:"learned"`
	LearningStatus     string   `json:"learning_status"`
	RuntimeStatus      string   `json:"runtime_status"`
	RuntimeFlags       []string `json:"runtime_flags"`
	RuntimeReasonCode  string   `json:"runtime_reason_code,omitempty"`
	RuntimeReason      string   `json:"runtime_reason,omitempty"`
	HealthSamples      int      `json:"health_samples"`
	CapacityGeneration uint64   `json:"capacity_generation"`
	CapacityHalfOpen   bool     `json:"capacity_half_open"`
	CircuitHalfOpen    bool     `json:"circuit_half_open"`
	CapacityRecovery   bool     `json:"capacity_recovery"`

	SchedulerScore      float64 `json:"scheduler_score"`
	ReliabilityScore    float64 `json:"reliability_score"`
	CapacityScore       float64 `json:"capacity_score"`
	LatencyScore        float64 `json:"latency_score"`
	CostScore           float64 `json:"cost_score"`
	CacheScore          float64 `json:"cache_score"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
	CacheSamples        int64   `json:"cache_samples"`
	CacheInputTokens    int64   `json:"cache_input_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`

	PathSuccessEMA float64 `json:"path_success_ema"`
	TTFTEMA        float64 `json:"ttft_ema"`
	TTFTSamples    int64   `json:"ttft_samples"`

	Quota *GeminiAdaptiveQuotaSnapshot `json:"quota,omitempty"`

	TotalSamples       int64 `json:"total_samples"`
	ConsecutiveFailure int   `json:"consecutive_failure"`

	LastSuccessAt             *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt             *time.Time `json:"last_failure_at,omitempty"`
	CooldownUntil             *time.Time `json:"cooldown_until,omitempty"`
	CooldownRemainingSec      int64      `json:"cooldown_remaining_sec"`
	CircuitOpenCount          int        `json:"circuit_open_count"`
	CapacityCooldownUntil     *time.Time `json:"capacity_cooldown_until,omitempty"`
	CapacityRecoverySuccesses int        `json:"capacity_recovery_successes"`
	QuotaLimited              bool       `json:"quota_limited"`
	QuotaResetAt              *time.Time `json:"quota_reset_at,omitempty"`
	QuotaNextProbeAt          *time.Time `json:"quota_next_probe_at,omitempty"`
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
	var stateStore *adaptiveStateStore
	metrics := GeminiAdaptiveMetricsSnapshot{}
	if s != nil && s.gatewayService != nil {
		cfg = s.gatewayService.geminiAdaptiveSchedulerSettingsForSnapshot(ctx)
		stateStore = s.gatewayService.geminiAdaptiveSchedulerCoreStateStoreForSnapshot()
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
	coreStates := make(map[int64]adaptiveAccountState, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		coreState := newAdaptiveAccountState(account.ID, account.Concurrency, now)
		if stateStore != nil {
			snapshot := stateStore.snapshot(account.ID, account.Concurrency, now, geminiAdaptiveCoreSettings(cfg))
			coreState = &snapshot
		}
		coreStates[account.ID] = *coreState
		loadReq = append(loadReq, AccountWithConcurrency{ID: account.ID, MaxConcurrency: coreState.EffectiveCapacity})
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if realtimeEnabled {
		loadMap = s.getGeminiAdaptiveLearningLoadMapBestEffort(ctx, loadReq)
	}
	rows := make([]GeminiAdaptiveSchedulerAccountLearningSnapshot, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		row := buildGeminiAdaptiveCoreLearningAccountSnapshot(account, coreStates[account.ID], loadInfo, now, geminiAdaptiveCoreSettings(cfg))
		rows = append(rows, row)
	}
	applyGeminiAdaptiveCoreScores(rows, accounts, coreStates, loadMap, now, geminiAdaptiveCoreSettings(cfg))
	rows = filterGeminiAdaptiveLearningRowsByDualStatus(rows, filter.LearningStatus, filter.RuntimeStatus)
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
		RealtimeEnabled: realtimeEnabled, GeneratedAt: now.UTC(), TimeRange: filter.TimeRange,
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

func (s *GatewayService) geminiAdaptiveSchedulerCoreStateStoreForSnapshot() *adaptiveStateStore {
	if s == nil || s.geminiAdaptiveScheduler == nil {
		return nil
	}
	return s.geminiAdaptiveScheduler.core
}

func buildGeminiAdaptiveCoreLearningAccountSnapshot(account *Account, state adaptiveAccountState, load *AccountLoadInfo, now time.Time, settings adaptiveCoreSettings) GeminiAdaptiveSchedulerAccountLearningSnapshot {
	if account == nil {
		return GeminiAdaptiveSchedulerAccountLearningSnapshot{}
	}
	if load == nil {
		load = &AccountLoadInfo{AccountID: account.ID}
	}
	learning, samples := adaptiveLearningState(state, account.IsOAuth(), now, settings)
	cacheStats := adaptiveCacheStatsForState(state, now, settings)
	runtimeStatus, flags, reasonCode, reason := adaptiveRuntimeState(state, account.IsActive() && account.Schedulable, load.CurrentConcurrency, now)
	row := GeminiAdaptiveSchedulerAccountLearningSnapshot{
		AccountID:                 account.ID,
		AccountName:               account.Name,
		Platform:                  account.Platform,
		Type:                      account.Type,
		AccountStatus:             account.Status,
		Schedulable:               account.IsSchedulable(),
		ConfiguredConcurrency:     account.Concurrency,
		EffectiveCapacity:         state.EffectiveCapacity,
		RateMultiplier:            account.BillingRateMultiplier(),
		CurrentConcurrency:        load.CurrentConcurrency,
		WaitingCount:              load.WaitingCount,
		LoadPercentage:            adaptiveLoadRate(load, state.EffectiveCapacity),
		SchedulerStatus:           string(runtimeStatus),
		StatusReason:              reason,
		Learned:                   learning == adaptiveLearningLearned || learning == adaptiveLearningNotApplicable,
		LearningStatus:            string(learning),
		RuntimeStatus:             string(runtimeStatus),
		RuntimeFlags:              make([]string, 0, len(flags)),
		RuntimeReasonCode:         reasonCode,
		RuntimeReason:             reason,
		HealthSamples:             samples,
		CacheHitRate:              cacheStats.HitRate,
		CacheSamples:              cacheStats.Samples,
		CacheInputTokens:          cacheStats.InputTokens,
		CacheCreationTokens:       cacheStats.CacheCreationTokens,
		CacheReadTokens:           cacheStats.CacheReadTokens,
		CapacityGeneration:        state.CapacityGeneration,
		CapacityHalfOpen:          state.CapacityHalfOpen,
		CircuitHalfOpen:           containsAdaptiveRuntimeFlag(flags, adaptiveRuntimeCircuitHalfOpen),
		CapacityRecovery:          containsAdaptiveRuntimeFlag(flags, adaptiveRuntimeCapacityRecovery),
		PathSuccessEMA:            state.SuccessEMA,
		TTFTEMA:                   state.TTFTEMA,
		TTFTSamples:               state.TTFTSamples,
		TotalSamples:              int64(samples),
		ConsecutiveFailure:        state.ConsecutiveFailures,
		LastSuccessAt:             geminiAdaptiveTimePtr(state.LastSuccessAt),
		LastFailureAt:             geminiAdaptiveTimePtr(state.LastFailureAt),
		CooldownUntil:             geminiAdaptiveTimePtr(state.CircuitOpenUntil),
		CircuitOpenCount:          state.CircuitOpenCount,
		CapacityCooldownUntil:     geminiAdaptiveTimePtr(state.CapacityCooldownUntil),
		CapacityRecoverySuccesses: state.CapacityRecoverySuccesses,
		QuotaLimited:              state.QuotaLimited,
		QuotaResetAt:              geminiAdaptiveTimePtr(state.QuotaResetAt),
		QuotaNextProbeAt:          geminiAdaptiveTimePtr(state.QuotaNextProbeAt),
	}
	for _, flag := range flags {
		row.RuntimeFlags = append(row.RuntimeFlags, string(flag))
	}
	if state.CircuitOpenUntil.After(now) {
		row.CooldownRemainingSec = int64(state.CircuitOpenUntil.Sub(now).Seconds())
		if row.CooldownRemainingSec < 1 {
			row.CooldownRemainingSec = 1
		}
	}
	return row
}

func applyGeminiAdaptiveCoreScores(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot, accounts []Account, states map[int64]adaptiveAccountState, loads map[int64]*AccountLoadInfo, now time.Time, settings adaptiveCoreSettings) {
	inputs := make([]adaptiveScoreCandidate, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		load := loads[account.ID]
		if load == nil {
			load = &AccountLoadInfo{AccountID: account.ID}
		}
		inputs = append(inputs, adaptiveScoreCandidate{AccountID: account.ID, OAuth: account.IsOAuth(), Cost: account.BillingRateMultiplier(), CurrentConcurrency: load.CurrentConcurrency, State: states[account.ID]})
	}
	byID := make(map[int64]adaptiveScoreCandidate, len(inputs))
	for _, score := range scoreAdaptiveCandidates(inputs, now, settings) {
		byID[score.AccountID] = score
	}
	for i := range rows {
		score := byID[rows[i].AccountID]
		rows[i].SchedulerScore = score.Score
		rows[i].ReliabilityScore = score.ReliabilityScore
		rows[i].CapacityScore = score.CapacityScore
		rows[i].LatencyScore = score.TTFTScore
		rows[i].CostScore = score.CostScore
		rows[i].CacheScore = score.CacheScore
		rows[i].CacheHitRate = score.CacheHitRate
		rows[i].CacheSamples = score.CacheSamples
	}
}

func filterGeminiAdaptiveLearningRowsByDualStatus(rows []GeminiAdaptiveSchedulerAccountLearningSnapshot, learningStatus, runtimeStatus string) []GeminiAdaptiveSchedulerAccountLearningSnapshot {
	learningStatus = strings.ToLower(strings.TrimSpace(learningStatus))
	runtimeStatus = strings.ToLower(strings.TrimSpace(runtimeStatus))
	if learningStatus == "" && runtimeStatus == "" {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		if learningStatus != "" && row.LearningStatus != learningStatus {
			continue
		}
		if runtimeStatus != "" && row.RuntimeStatus != runtimeStatus {
			continue
		}
		out = append(out, row)
	}
	return out
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
	filter.SortBy = normalizeGeminiAdaptiveLearningSortBy(filter.SortBy)
	filter.SortOrder = normalizeGeminiAdaptiveLearningSortOrder(filter.SortOrder)
	filter.LearningStatus = strings.ToLower(strings.TrimSpace(filter.LearningStatus))
	filter.RuntimeStatus = strings.ToLower(strings.TrimSpace(filter.RuntimeStatus))
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
	case "account", "status", "capacity", "load", "score", "samples", "error", "latency", "cache", "last_event":
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
		if cmp := compareFloat64(1-left.PathSuccessEMA, 1-right.PathSuccessEMA); cmp != 0 {
			return cmp
		}
	case "latency":
		if cmp := compareFloat64(left.TTFTEMA, right.TTFTEMA); cmp != 0 {
			return cmp
		}
	case "cache":
		if cmp := compareFloat64(left.CacheHitRate, right.CacheHitRate); cmp != 0 {
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
		if cmp := compareFloat64(right.SchedulerScore, left.SchedulerScore); cmp != 0 {
			return cmp
		}
	}
	return compareInt64(left.AccountID, right.AccountID)
}

func geminiAdaptiveLearningLastEventTime(row GeminiAdaptiveSchedulerAccountLearningSnapshot) time.Time {
	values := []*time.Time{row.LastSuccessAt, row.LastFailureAt, row.CooldownUntil, row.CapacityCooldownUntil, row.QuotaResetAt, row.QuotaNextProbeAt}
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
		switch row.LearningStatus {
		case string(adaptiveLearningUnlearned):
			summary.UnlearnedAccounts++
		case string(adaptiveLearningLearning):
			summary.LearningAccounts++
		case string(adaptiveLearningLearned):
			summary.LearnedAccounts++
		case string(adaptiveLearningNotApplicable):
			summary.NotApplicableAccounts++
		}
		switch row.RuntimeStatus {
		case GeminiAdaptiveLearningStatusUnavailable:
			summary.UnavailableAccounts++
		case GeminiAdaptiveLearningStatusQuotaLimited:
			summary.QuotaLimitedAccounts++
		case GeminiAdaptiveLearningStatusCooldown:
			summary.CooldownAccounts++
		case string(adaptiveRuntimeHalfOpen):
			summary.HalfOpenAccounts++
		case string(adaptiveRuntimeCircuitHalfOpen):
			summary.CircuitHalfOpenAccounts++
			summary.HalfOpenAccounts++
		case string(adaptiveRuntimeCapacityRecovery):
			summary.CapacityRecoveryAccounts++
			summary.HalfOpenAccounts++
		case GeminiAdaptiveLearningStatusHighError:
			summary.HighErrorAccounts++
		case GeminiAdaptiveLearningStatusSaturated:
			summary.SaturatedAccounts++
		case GeminiAdaptiveLearningStatusHealthy:
			summary.HealthyAccounts++
		}
		if row.CircuitHalfOpen && row.RuntimeStatus != string(adaptiveRuntimeCircuitHalfOpen) {
			summary.CircuitHalfOpenAccounts++
			summary.HalfOpenAccounts++
		}
		if row.CapacityRecovery && row.RuntimeStatus != string(adaptiveRuntimeCapacityRecovery) {
			summary.CapacityRecoveryAccounts++
			summary.HalfOpenAccounts++
		}
	}
	return summary
}

func geminiAdaptiveLearningSettingsSnapshot(cfg GeminiAdaptiveSchedulerSettings) GeminiAdaptiveSchedulerLearningSettingsSnapshot {
	return GeminiAdaptiveSchedulerLearningSettingsSnapshot{
		TopK:                      cfg.GeminiAdaptiveSchedulerTopK,
		SoftmaxTemperature:        cfg.GeminiAdaptiveSchedulerSoftmaxTemperature,
		ExplorationRate:           cfg.GeminiAdaptiveSchedulerExplorationRate,
		ConsecutiveFailurePenalty: cfg.GeminiAdaptiveSchedulerConsecutiveFailurePenalty,
		WeightReliability:         cfg.GeminiAdaptiveSchedulerWeightReliability,
		WeightCapacity:            cfg.GeminiAdaptiveSchedulerWeightCapacity,
		WeightLatency:             cfg.GeminiAdaptiveSchedulerWeightLatency,
		WeightCost:                cfg.GeminiAdaptiveSchedulerWeightCost,
		WeightCache:               cfg.GeminiAdaptiveSchedulerWeightCache,
		LearningWindowSeconds:     cfg.GeminiAdaptiveSchedulerLearningWindowSeconds,
		LearningMinHealthSamples:  cfg.GeminiAdaptiveSchedulerLearningMinHealthSamples,
		SuccessEMAAlpha:           cfg.GeminiAdaptiveSchedulerSuccessEMAAlpha,
		TTFTEMAAlpha:              cfg.GeminiAdaptiveSchedulerLatencyEMAAlpha,
		CooldownSeconds:           cfg.GeminiAdaptiveSchedulerCooldownSeconds,
		CooldownMaxSeconds:        cfg.GeminiAdaptiveSchedulerCooldownMaxSeconds,
		AccountFailureThreshold:   cfg.GeminiAdaptiveSchedulerAccountFailureThreshold,
		HighErrorMinSamples:       cfg.GeminiAdaptiveSchedulerHighErrorMinSamples,
		HighErrorMaxSamples:       cfg.GeminiAdaptiveSchedulerHighErrorMaxSamples,
		HighErrorEnterRate:        cfg.GeminiAdaptiveSchedulerHighErrorEnterRate,
		HighErrorExitRate:         cfg.GeminiAdaptiveSchedulerHighErrorExitRate,
		CapacityShrinkFactor:      cfg.GeminiAdaptiveSchedulerShrinkFactorSoft,
		CapacityGrowthFactor:      cfg.GeminiAdaptiveSchedulerCapacityGrowthFactor,
		CapacityRecoverySamples:   cfg.GeminiAdaptiveSchedulerCapacityRecoverySamples,
		CapacityRecoveryLoad:      cfg.GeminiAdaptiveSchedulerCapacityProbeLoadThreshold,
		QuotaProbeIntervalSeconds: cfg.GeminiAdaptiveSchedulerQuotaProbeIntervalSeconds,
		DiagnosticLogEnabled:      cfg.GeminiAdaptiveSchedulerDiagnosticLogEnabled,
		DiagnosticLogSampleRate:   cfg.GeminiAdaptiveSchedulerDiagnosticLogSampleRate,
	}
}
