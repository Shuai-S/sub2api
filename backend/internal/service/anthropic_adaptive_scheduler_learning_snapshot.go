package service

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	anthropicAdaptiveLearningDefaultLimit = 50
	anthropicAdaptiveLearningMaxLimit     = 500

	AnthropicAdaptiveLearningStatusDisabled    = "disabled"
	AnthropicAdaptiveLearningStatusUnavailable = "unavailable"
	AnthropicAdaptiveLearningStatusCooldown    = "cooldown"
	AnthropicAdaptiveLearningStatusHighError   = "high_error"
	AnthropicAdaptiveLearningStatusSaturated   = "saturated"
	AnthropicAdaptiveLearningStatusLearning    = "learning"
	AnthropicAdaptiveLearningStatusUnlearned   = "unlearned"
	AnthropicAdaptiveLearningStatusHealthy     = "healthy"
)

type AnthropicAdaptiveSchedulerLearningSnapshot struct {
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

	Settings AnthropicAdaptiveSchedulerLearningSettingsSnapshot  `json:"settings"`
	Summary  AnthropicAdaptiveSchedulerLearningSummary           `json:"summary"`
	Accounts []AnthropicAdaptiveSchedulerAccountLearningSnapshot `json:"accounts"`
}

type AnthropicAdaptiveSchedulerLearningFilter struct {
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

func (f *AnthropicAdaptiveSchedulerLearningFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type AnthropicAdaptiveSchedulerLearningSettingsSnapshot struct {
	DiagnosticLogEnabled       bool    `json:"diagnostic_log_enabled"`
	DiagnosticLogSampleRate    float64 `json:"diagnostic_log_sample_rate"`
	TopK                       int     `json:"top_k"`
	SoftmaxTemperature         float64 `json:"softmax_temperature"`
	ExplorationRate            float64 `json:"exploration_rate"`
	ConsecutiveFailurePenalty  float64 `json:"consecutive_failure_penalty"`
	LearningWindowSeconds      int     `json:"learning_window_seconds"`
	LearningMinHealthSamples   int     `json:"learning_min_health_samples"`
	SuccessEMAAlpha            float64 `json:"success_ema_alpha"`
	LatencyEMAAlpha            float64 `json:"ttft_ema_alpha"`
	HealthFailureThreshold     int     `json:"health_failure_threshold"`
	CooldownSeconds            int     `json:"cooldown_seconds"`
	CooldownMaxSeconds         int     `json:"cooldown_max_seconds"`
	HighErrorMinSamples        int     `json:"high_error_min_samples"`
	HighErrorMaxSamples        int     `json:"high_error_max_samples"`
	HighErrorEnterRate         float64 `json:"high_error_enter_rate"`
	HighErrorExitRate          float64 `json:"high_error_exit_rate"`
	CapacityProbeLoadThreshold float64 `json:"capacity_recovery_load"`
	ShrinkFactorSoft           float64 `json:"capacity_shrink_factor"`
	CapacityGrowthFactor       float64 `json:"capacity_growth_factor"`
	CapacityRecoverySamples    int     `json:"capacity_recovery_samples"`
	QuotaProbeIntervalSeconds  int     `json:"quota_probe_interval_seconds"`
	WeightReliability          float64 `json:"weight_reliability"`
	WeightCapacity             float64 `json:"weight_capacity"`
	WeightLatency              float64 `json:"weight_ttft"`
	WeightCost                 float64 `json:"weight_cost"`
}

type AnthropicAdaptiveSchedulerLearningSummary struct {
	TrackedAccounts       int `json:"tracked_accounts"`
	DisabledAccounts      int `json:"disabled_accounts"`
	UnlearnedAccounts     int `json:"unlearned_accounts"`
	LearningAccounts      int `json:"learning_accounts"`
	HealthyAccounts       int `json:"healthy_accounts"`
	HighErrorAccounts     int `json:"high_error_accounts"`
	CooldownAccounts      int `json:"cooldown_accounts"`
	SaturatedAccounts     int `json:"saturated_accounts"`
	UnavailableAccounts   int `json:"unavailable_accounts"`
	LearnedAccounts       int `json:"learned_accounts"`
	NotApplicableAccounts int `json:"not_applicable_accounts"`
	HalfOpenAccounts      int `json:"half_open_accounts"`
	QuotaLimitedAccounts  int `json:"quota_limited_accounts"`
}

type AnthropicAdaptiveSchedulerAccountLearningSnapshot struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	Platform      string `json:"platform"`
	Type          string `json:"type"`
	AccountStatus string `json:"account_status"`
	Schedulable   bool   `json:"schedulable"`

	ConfiguredConcurrency int     `json:"configured_concurrency"`
	EffectiveCapacity     int     `json:"effective_capacity"`
	RateMultiplier        float64 `json:"rate_multiplier"`

	CurrentConcurrency int     `json:"current_concurrency"`
	WaitingCount       int     `json:"waiting_count"`
	LoadPercentage     float64 `json:"load_percentage"`

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

	SchedulerScore   float64 `json:"scheduler_score"`
	ReliabilityScore float64 `json:"reliability_score"`
	CapacityScore    float64 `json:"capacity_score"`
	LatencyScore     float64 `json:"latency_score"`
	CostScore        float64 `json:"cost_score"`

	SuccessEMA     float64 `json:"success_ema"`
	TTFTEMA        float64 `json:"ttft_ema"`
	LatencySamples int64   `json:"ttft_samples"`

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

func (s *OpsService) GetAnthropicAdaptiveSchedulerLearningSnapshot(
	ctx context.Context,
	filter *AnthropicAdaptiveSchedulerLearningFilter,
) (*AnthropicAdaptiveSchedulerLearningSnapshot, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil {
		filter = &AnthropicAdaptiveSchedulerLearningFilter{}
	}
	normalizeAnthropicAdaptiveLearningFilter(filter)
	limit := filter.TopN
	if !filter.IsTopNMode() {
		limit = filter.PageSize
	}

	cfg := DefaultAnthropicAdaptiveSchedulerSettings()
	var stateStore *adaptiveStateStore
	if s != nil && s.gatewayService != nil {
		cfg = s.gatewayService.anthropicAdaptiveSchedulerSettingsForSnapshot(ctx)
		stateStore = s.gatewayService.anthropicAdaptiveSchedulerCoreStateStoreForSnapshot()
	}
	realtimeEnabled := s.IsRealtimeMonitoringEnabled(ctx)

	accounts, err := s.listAllAccountsForOps(ctx, PlatformAnthropic, filter.GroupID)
	if err != nil {
		return nil, err
	}
	accounts = filterAnthropicAdaptiveLearningAccountsByGroup(accounts, filter.GroupID)
	accounts = filterAnthropicAdaptiveLearningSchedulableAccounts(accounts)

	now := time.Now()
	coreStates := make(map[int64]adaptiveAccountState, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		coreState := newAdaptiveAccountState(account.ID, account.Concurrency, now)
		if stateStore != nil {
			snapshot := stateStore.snapshot(account.ID, account.Concurrency, now, anthropicAdaptiveCoreSettings(cfg))
			coreState = &snapshot
		}
		coreStates[account.ID] = *coreState
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: coreState.EffectiveCapacity,
		})
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if realtimeEnabled {
		loadMap = s.getAnthropicAdaptiveLearningLoadMapBestEffort(ctx, loadReq)
	}

	rows := make([]AnthropicAdaptiveSchedulerAccountLearningSnapshot, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		row := buildAnthropicAdaptiveCoreLearningAccountSnapshot(account, coreStates[account.ID], loadInfo, now, anthropicAdaptiveCoreSettings(cfg))
		rows = append(rows, row)
	}
	applyAnthropicAdaptiveCoreScores(rows, accounts, coreStates, loadMap, now, anthropicAdaptiveCoreSettings(cfg))
	rows = filterAnthropicAdaptiveLearningRowsByDualStatus(rows, filter.LearningStatus, filter.RuntimeStatus)
	rows = filterAnthropicAdaptiveLearningRowsByTime(rows, filter.StartTime, filter.EndTime)
	sortAnthropicAdaptiveLearningRows(rows, filter.SortBy, filter.SortOrder)

	summary := summarizeAnthropicAdaptiveLearningRows(rows)
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
			end := start + filter.PageSize
			if end > len(rows) {
				end = len(rows)
			}
			rows = rows[start:end]
		}
	}

	return &AnthropicAdaptiveSchedulerLearningSnapshot{
		Enabled:          cfg.AnthropicAdaptiveSchedulerEnabled,
		Mode:             cfg.AnthropicAdaptiveSchedulerMode,
		RealtimeEnabled:  realtimeEnabled,
		GeneratedAt:      now.UTC(),
		TimeRange:        filter.TimeRange,
		StartTime:        filter.StartTime.UTC(),
		EndTime:          filter.EndTime.UTC(),
		TotalAccounts:    total,
		Total:            total,
		ReturnedAccounts: len(rows),
		Limit:            limit,
		Page:             filter.Page,
		PageSize:         filter.PageSize,
		TopN:             filter.TopN,
		SortBy:           filter.SortBy,
		SortOrder:        filter.SortOrder,
		Settings:         anthropicAdaptiveLearningSettingsSnapshot(cfg),
		Summary:          summary,
		Accounts:         rows,
	}, nil
}

func normalizeAnthropicAdaptiveLearningFilter(filter *AnthropicAdaptiveSchedulerLearningFilter) {
	if filter == nil {
		return
	}
	if filter.TopN > anthropicAdaptiveLearningMaxLimit {
		filter.TopN = anthropicAdaptiveLearningMaxLimit
	}
	if filter.TopN < 0 {
		filter.TopN = 0
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = anthropicAdaptiveLearningDefaultLimit
	}
	if filter.PageSize > anthropicAdaptiveLearningMaxLimit {
		filter.PageSize = anthropicAdaptiveLearningMaxLimit
	}
	filter.SortBy = normalizeAnthropicAdaptiveLearningSortBy(filter.SortBy)
	filter.SortOrder = normalizeAnthropicAdaptiveLearningSortOrder(filter.SortOrder)
	filter.LearningStatus = strings.ToLower(strings.TrimSpace(filter.LearningStatus))
	filter.RuntimeStatus = strings.ToLower(strings.TrimSpace(filter.RuntimeStatus))
}

func (s *GatewayService) anthropicAdaptiveSchedulerSettingsForSnapshot(ctx context.Context) AnthropicAdaptiveSchedulerSettings {
	defaults := DefaultAnthropicAdaptiveSchedulerSettings()
	if s == nil || s.settingService == nil {
		return defaults
	}
	settings, err := s.settingService.GetAnthropicAdaptiveSchedulerSettings(ctx)
	if err != nil {
		log.Printf("[Ops] Anthropic adaptive settings lookup failed: %v", err)
		return defaults
	}
	return NormalizeAnthropicAdaptiveSchedulerSettings(settings)
}

func (s *GatewayService) anthropicAdaptiveSchedulerCoreStateStoreForSnapshot() *adaptiveStateStore {
	if s == nil || s.anthropicAdaptiveScheduler == nil {
		return nil
	}
	return s.anthropicAdaptiveScheduler.core
}

func buildAnthropicAdaptiveCoreLearningAccountSnapshot(account *Account, state adaptiveAccountState, load *AccountLoadInfo, now time.Time, settings adaptiveCoreSettings) AnthropicAdaptiveSchedulerAccountLearningSnapshot {
	if account == nil {
		return AnthropicAdaptiveSchedulerAccountLearningSnapshot{}
	}
	if load == nil {
		load = &AccountLoadInfo{AccountID: account.ID}
	}
	learning, samples := adaptiveLearningState(state, account.IsOAuth(), now, settings)
	runtimeStatus, flags, reasonCode, reason := adaptiveRuntimeState(state, account.IsActive() && account.Schedulable, load.CurrentConcurrency, now)
	row := AnthropicAdaptiveSchedulerAccountLearningSnapshot{
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
		CapacityGeneration:        state.CapacityGeneration,
		CapacityHalfOpen:          state.CapacityHalfOpen,
		SuccessEMA:                state.SuccessEMA,
		TTFTEMA:                   state.TTFTEMA,
		LatencySamples:            state.TTFTSamples,
		TotalSamples:              int64(samples),
		ConsecutiveFailure:        state.ConsecutiveFailures,
		LastSuccessAt:             anthropicAdaptiveTimePtrIfNotZero(state.LastSuccessAt),
		LastFailureAt:             anthropicAdaptiveTimePtrIfNotZero(state.LastFailureAt),
		CooldownUntil:             anthropicAdaptiveTimePtrIfNotZero(state.CircuitOpenUntil),
		CircuitOpenCount:          state.CircuitOpenCount,
		CapacityCooldownUntil:     anthropicAdaptiveTimePtrIfNotZero(state.CapacityCooldownUntil),
		CapacityRecoverySuccesses: state.CapacityRecoverySuccesses,
		QuotaLimited:              state.QuotaLimited,
		QuotaResetAt:              anthropicAdaptiveTimePtrIfNotZero(state.QuotaResetAt),
		QuotaNextProbeAt:          anthropicAdaptiveTimePtrIfNotZero(state.QuotaNextProbeAt),
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

func applyAnthropicAdaptiveCoreScores(rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot, accounts []Account, states map[int64]adaptiveAccountState, loads map[int64]*AccountLoadInfo, now time.Time, settings adaptiveCoreSettings) {
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
	}
}

func filterAnthropicAdaptiveLearningRowsByDualStatus(rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot, learningStatus, runtimeStatus string) []AnthropicAdaptiveSchedulerAccountLearningSnapshot {
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

func (s *OpsService) getAnthropicAdaptiveLearningLoadMapBestEffort(
	ctx context.Context,
	accounts []AccountWithConcurrency,
) map[int64]*AccountLoadInfo {
	if s == nil || s.concurrencyService == nil || len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}
	}
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	for i := 0; i < len(accounts); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(accounts) {
			end = len(accounts)
		}
		part, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accounts[i:end])
		if err != nil {
			log.Printf("[Ops] Anthropic adaptive learning GetAccountsLoadBatch failed: %v", err)
			continue
		}
		for key, value := range part {
			out[key] = value
		}
	}
	return out
}

func filterAnthropicAdaptiveLearningAccountsByGroup(accounts []Account, groupIDFilter *int64) []Account {
	if groupIDFilter == nil || *groupIDFilter <= 0 {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		for _, group := range account.Groups {
			if group != nil && group.ID == *groupIDFilter {
				out = append(out, account)
				break
			}
		}
	}
	return out
}

func filterAnthropicAdaptiveLearningSchedulableAccounts(accounts []Account) []Account {
	if len(accounts) == 0 {
		return accounts
	}
	out := accounts[:0]
	for _, account := range accounts {
		if account.Schedulable {
			out = append(out, account)
		}
	}
	return out
}

func filterAnthropicAdaptiveLearningRowsByTime(
	rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot,
	start time.Time,
	end time.Time,
) []AnthropicAdaptiveSchedulerAccountLearningSnapshot {
	if len(rows) == 0 || start.IsZero() || end.IsZero() || !end.After(start) {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		lastEvent := anthropicAdaptiveLearningLastEventTime(row)
		if lastEvent.IsZero() || (!lastEvent.Before(start) && lastEvent.Before(end.Add(time.Nanosecond))) {
			out = append(out, row)
		}
	}
	return out
}

func sortAnthropicAdaptiveLearningRows(
	rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot,
	sortBy string,
	sortOrder string,
) {
	sortBy = normalizeAnthropicAdaptiveLearningSortBy(sortBy)
	sortOrder = normalizeAnthropicAdaptiveLearningSortOrder(sortOrder)
	if sortBy != "" {
		sort.SliceStable(rows, func(i, j int) bool {
			cmp := compareAnthropicAdaptiveLearningRows(rows[i], rows[j], sortBy)
			if cmp == 0 {
				return compareAnthropicAdaptiveLearningRows(rows[i], rows[j], "default") < 0
			}
			if sortOrder == "asc" {
				return cmp < 0
			}
			return cmp > 0
		})
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return compareAnthropicAdaptiveLearningRows(rows[i], rows[j], "default") < 0
	})
}

func normalizeAnthropicAdaptiveLearningSortBy(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "account", "status", "capacity", "load", "score", "samples", "error", "latency", "last_event":
		return value
	case "default", "":
		return ""
	default:
		return ""
	}
}

func normalizeAnthropicAdaptiveLearningSortOrder(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "asc") {
		return "asc"
	}
	return "desc"
}

func compareAnthropicAdaptiveLearningRows(
	left AnthropicAdaptiveSchedulerAccountLearningSnapshot,
	right AnthropicAdaptiveSchedulerAccountLearningSnapshot,
	sortBy string,
) int {
	switch sortBy {
	case "account":
		if cmp := strings.Compare(strings.ToLower(left.AccountName), strings.ToLower(right.AccountName)); cmp != 0 {
			return cmp
		}
	case "status":
		if cmp := compareAnthropicAdaptiveLearningInt(anthropicAdaptiveLearningStatusRank(left.SchedulerStatus), anthropicAdaptiveLearningStatusRank(right.SchedulerStatus)); cmp != 0 {
			return -cmp
		}
	case "capacity":
		if cmp := compareAnthropicAdaptiveLearningInt(left.EffectiveCapacity, right.EffectiveCapacity); cmp != 0 {
			return cmp
		}
	case "load":
		if cmp := compareAnthropicAdaptiveLearningFloat64(left.LoadPercentage, right.LoadPercentage); cmp != 0 {
			return cmp
		}
	case "score":
		if cmp := compareAnthropicAdaptiveLearningFloat64(left.SchedulerScore, right.SchedulerScore); cmp != 0 {
			return cmp
		}
	case "samples":
		if cmp := compareAnthropicAdaptiveLearningInt64(left.TotalSamples, right.TotalSamples); cmp != 0 {
			return cmp
		}
	case "error":
		if cmp := compareAnthropicAdaptiveLearningFloat64(1-left.SuccessEMA, 1-right.SuccessEMA); cmp != 0 {
			return cmp
		}
	case "latency":
		if cmp := compareAnthropicAdaptiveLearningFloat64(left.TTFTEMA, right.TTFTEMA); cmp != 0 {
			return cmp
		}
	case "last_event":
		if cmp := compareAnthropicAdaptiveLearningTime(anthropicAdaptiveLearningLastEventTime(left), anthropicAdaptiveLearningLastEventTime(right)); cmp != 0 {
			return cmp
		}
	default:
		leftRank := anthropicAdaptiveLearningStatusRank(left.SchedulerStatus)
		rightRank := anthropicAdaptiveLearningStatusRank(right.SchedulerStatus)
		if leftRank != rightRank {
			return compareAnthropicAdaptiveLearningInt(leftRank, rightRank)
		}
		if left.LoadPercentage != right.LoadPercentage {
			return compareAnthropicAdaptiveLearningFloat64(right.LoadPercentage, left.LoadPercentage)
		}
		if left.SchedulerScore != right.SchedulerScore {
			return compareAnthropicAdaptiveLearningFloat64(left.SchedulerScore, right.SchedulerScore)
		}
	}
	return compareAnthropicAdaptiveLearningInt64(left.AccountID, right.AccountID)
}

func compareAnthropicAdaptiveLearningInt(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareAnthropicAdaptiveLearningInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareAnthropicAdaptiveLearningFloat64(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareAnthropicAdaptiveLearningTime(left, right time.Time) int {
	if left.Before(right) {
		return -1
	}
	if left.After(right) {
		return 1
	}
	return 0
}

func anthropicAdaptiveLearningLastEventTime(row AnthropicAdaptiveSchedulerAccountLearningSnapshot) time.Time {
	var latest time.Time
	for _, candidate := range []*time.Time{
		row.LastSuccessAt,
		row.LastFailureAt,
		row.CooldownUntil,
	} {
		if candidate != nil && candidate.After(latest) {
			latest = *candidate
		}
	}
	return latest
}

func anthropicAdaptiveLearningStatusRank(status string) int {
	switch status {
	case AnthropicAdaptiveLearningStatusCooldown:
		return 0
	case AnthropicAdaptiveLearningStatusHighError:
		return 1
	case AnthropicAdaptiveLearningStatusSaturated:
		return 2
	case AnthropicAdaptiveLearningStatusUnavailable:
		return 3
	case AnthropicAdaptiveLearningStatusLearning:
		return 4
	case AnthropicAdaptiveLearningStatusUnlearned:
		return 5
	case AnthropicAdaptiveLearningStatusDisabled:
		return 6
	default:
		return 7
	}
}

func summarizeAnthropicAdaptiveLearningRows(
	rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot,
) AnthropicAdaptiveSchedulerLearningSummary {
	var summary AnthropicAdaptiveSchedulerLearningSummary
	for _, row := range rows {
		summary.TrackedAccounts++
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
		case AnthropicAdaptiveLearningStatusUnavailable:
			summary.UnavailableAccounts++
		case AnthropicAdaptiveLearningStatusCooldown:
			summary.CooldownAccounts++
		case string(adaptiveRuntimeHalfOpen):
			summary.HalfOpenAccounts++
		case string(adaptiveRuntimeQuotaLimited):
			summary.QuotaLimitedAccounts++
		case AnthropicAdaptiveLearningStatusHighError:
			summary.HighErrorAccounts++
		case AnthropicAdaptiveLearningStatusSaturated:
			summary.SaturatedAccounts++
		case AnthropicAdaptiveLearningStatusHealthy:
			summary.HealthyAccounts++
		}
	}
	return summary
}

func anthropicAdaptiveLearningSettingsSnapshot(
	cfg AnthropicAdaptiveSchedulerSettings,
) AnthropicAdaptiveSchedulerLearningSettingsSnapshot {
	return AnthropicAdaptiveSchedulerLearningSettingsSnapshot{
		DiagnosticLogEnabled:       cfg.AnthropicAdaptiveSchedulerDiagnosticLogEnabled,
		DiagnosticLogSampleRate:    cfg.AnthropicAdaptiveSchedulerDiagnosticLogSampleRate,
		TopK:                       cfg.AnthropicAdaptiveSchedulerTopK,
		SoftmaxTemperature:         cfg.AnthropicAdaptiveSchedulerSoftmaxTemperature,
		ExplorationRate:            cfg.AnthropicAdaptiveSchedulerExplorationRate,
		ConsecutiveFailurePenalty:  cfg.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty,
		LearningWindowSeconds:      cfg.AnthropicAdaptiveSchedulerLearningWindowSeconds,
		LearningMinHealthSamples:   cfg.AnthropicAdaptiveSchedulerLearningMinHealthSamples,
		SuccessEMAAlpha:            cfg.AnthropicAdaptiveSchedulerSuccessEMAAlpha,
		LatencyEMAAlpha:            cfg.AnthropicAdaptiveSchedulerLatencyEMAAlpha,
		HealthFailureThreshold:     cfg.AnthropicAdaptiveSchedulerHealthFailureThreshold,
		CooldownSeconds:            cfg.AnthropicAdaptiveSchedulerCooldownSeconds,
		CooldownMaxSeconds:         cfg.AnthropicAdaptiveSchedulerCooldownMaxSeconds,
		HighErrorMinSamples:        cfg.AnthropicAdaptiveSchedulerHighErrorMinSamples,
		HighErrorMaxSamples:        cfg.AnthropicAdaptiveSchedulerHighErrorMaxSamples,
		HighErrorEnterRate:         cfg.AnthropicAdaptiveSchedulerHighErrorEnterRate,
		HighErrorExitRate:          cfg.AnthropicAdaptiveSchedulerHighErrorExitRate,
		CapacityProbeLoadThreshold: cfg.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold,
		ShrinkFactorSoft:           cfg.AnthropicAdaptiveSchedulerShrinkFactorSoft,
		CapacityGrowthFactor:       cfg.AnthropicAdaptiveSchedulerCapacityGrowthFactor,
		CapacityRecoverySamples:    cfg.AnthropicAdaptiveSchedulerCapacityRecoverySamples,
		QuotaProbeIntervalSeconds:  cfg.AnthropicAdaptiveSchedulerQuotaProbeIntervalSeconds,
		WeightReliability:          cfg.AnthropicAdaptiveSchedulerWeightReliability,
		WeightCapacity:             cfg.AnthropicAdaptiveSchedulerWeightCapacity,
		WeightLatency:              cfg.AnthropicAdaptiveSchedulerWeightLatency,
		WeightCost:                 cfg.AnthropicAdaptiveSchedulerWeightCost,
	}
}

func anthropicAdaptiveTimePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
