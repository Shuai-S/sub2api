package service

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	openAIAdaptiveLearningDefaultLimit = 50
	openAIAdaptiveLearningMaxLimit     = 500

	OpenAIAdaptiveLearningStatusDisabled    = "disabled"
	OpenAIAdaptiveLearningStatusUnavailable = "unavailable"
	OpenAIAdaptiveLearningStatusCooldown    = "cooldown"
	OpenAIAdaptiveLearningStatusHalfOpen    = "half_open"
	OpenAIAdaptiveLearningStatusHighError   = "high_error"
	OpenAIAdaptiveLearningStatusSaturated   = "saturated"
	OpenAIAdaptiveLearningStatusLearning    = "learning"
	OpenAIAdaptiveLearningStatusUnlearned   = "unlearned"
	OpenAIAdaptiveLearningStatusHealthy     = "healthy"
)

type OpenAIAdaptiveSchedulerLearningSnapshot struct {
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

	Settings OpenAIAdaptiveSchedulerLearningSettingsSnapshot  `json:"settings"`
	Summary  OpenAIAdaptiveSchedulerLearningSummary           `json:"summary"`
	Accounts []OpenAIAdaptiveSchedulerAccountLearningSnapshot `json:"accounts"`
}

type OpenAIAdaptiveSchedulerLearningFilter struct {
	GroupID   *int64
	TimeRange string
	StartTime time.Time
	EndTime   time.Time
	TopN      int
	Page      int
	PageSize  int
	Status    string
	SortBy    string
	SortOrder string
}

func (f *OpenAIAdaptiveSchedulerLearningFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type OpenAIAdaptiveSchedulerLearningSettingsSnapshot struct {
	TopK                      int     `json:"top_k"`
	AccountTypePriorityMode   string  `json:"account_type_priority_mode"`
	ExplorationRate           float64 `json:"exploration_rate"`
	SoftmaxTemperature        float64 `json:"softmax_temperature"`
	InitialCapacityFraction   float64 `json:"initial_capacity_fraction"`
	MinCapacity               int     `json:"min_capacity"`
	CapacityGrowthFactor      float64 `json:"capacity_growth_factor"`
	BurstProbeRatio           float64 `json:"burst_probe_ratio"`
	CapacityFailureThreshold  int     `json:"capacity_failure_threshold"`
	MinRecentSamplesForShrink int     `json:"min_recent_samples_for_shrink"`
	ShrinkErrorThreshold      float64 `json:"shrink_error_threshold"`
	ShrinkFactorSoft          float64 `json:"shrink_factor_soft"`
	ShrinkFactorHard          float64 `json:"shrink_factor_hard"`
	HalfOpenFailureThreshold  int     `json:"half_open_failure_threshold"`
	HalfOpenProbeCapacity     int     `json:"half_open_probe_capacity"`
	LearningWindowSeconds     int     `json:"learning_window_seconds"`
}

type OpenAIAdaptiveSchedulerLearningSummary struct {
	TrackedAccounts     int `json:"tracked_accounts"`
	UnlearnedAccounts   int `json:"unlearned_accounts"`
	LearningAccounts    int `json:"learning_accounts"`
	HealthyAccounts     int `json:"healthy_accounts"`
	HighErrorAccounts   int `json:"high_error_accounts"`
	CooldownAccounts    int `json:"cooldown_accounts"`
	HalfOpenAccounts    int `json:"half_open_accounts"`
	SaturatedAccounts   int `json:"saturated_accounts"`
	UnavailableAccounts int `json:"unavailable_accounts"`
}

type OpenAIAdaptiveSchedulerAccountLearningSnapshot struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	Platform      string `json:"platform"`
	Type          string `json:"type"`
	AccountStatus string `json:"account_status"`
	Schedulable   bool   `json:"schedulable"`
	Priority      int    `json:"priority"`

	ConfiguredConcurrency int     `json:"configured_concurrency"`
	StableCapacity        int     `json:"stable_capacity"`
	EffectiveCapacity     int     `json:"effective_capacity"`
	BurstCapacity         int     `json:"burst_capacity"`
	RateMultiplier        float64 `json:"rate_multiplier"`

	CurrentConcurrency int     `json:"current_concurrency"`
	WaitingCount       int     `json:"waiting_count"`
	LoadPercentage     float64 `json:"load_percentage"`

	SchedulerStatus string `json:"scheduler_status"`
	StatusReason    string `json:"status_reason,omitempty"`
	Learned         bool   `json:"learned"`

	SchedulerScore   float64 `json:"scheduler_score"`
	SuccessScore     float64 `json:"success_score"`
	CostScore        float64 `json:"cost_score"`
	CapacityScore    float64 `json:"capacity_score"`
	LatencyScore     float64 `json:"latency_score"`
	StabilityScore   float64 `json:"stability_score"`
	ExplorationScore float64 `json:"exploration_score"`

	SuccessEMA float64 `json:"success_ema"`
	ErrorEMA   float64 `json:"error_ema"`
	LatencyEMA float64 `json:"latency_ema"`
	TTFTEMA    float64 `json:"ttft_ema"`

	TotalSamples               int64   `json:"total_samples"`
	RecentSamples              int     `json:"recent_samples"`
	RecentFailures             int     `json:"recent_failures"`
	RecentFailureRate          float64 `json:"recent_failure_rate"`
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

func (s *OpsService) GetOpenAIAdaptiveSchedulerLearningSnapshot(
	ctx context.Context,
	filter *OpenAIAdaptiveSchedulerLearningFilter,
) (*OpenAIAdaptiveSchedulerLearningSnapshot, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil {
		filter = &OpenAIAdaptiveSchedulerLearningFilter{}
	}
	normalizeOpenAIAdaptiveLearningFilter(filter)
	limit := filter.TopN
	if !filter.IsTopNMode() {
		limit = filter.PageSize
	}

	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	var stateStore *openAIAdaptiveSchedulerStateStore
	if s != nil && s.openAIGatewayService != nil {
		cfg = s.openAIGatewayService.openAIAdaptiveSchedulerSettings(ctx)
		stateStore = s.openAIGatewayService.openAIAdaptiveSchedulerStateStoreForSnapshot()
	}
	realtimeEnabled := s.IsRealtimeMonitoringEnabled(ctx)

	accounts, err := s.listAllAccountsForOps(ctx, PlatformOpenAI, filter.GroupID)
	if err != nil {
		return nil, err
	}
	accounts = filterOpenAIAdaptiveLearningAccountsByGroup(accounts, filter.GroupID)
	accounts = filterOpenAIAdaptiveLearningSchedulableAccounts(accounts)

	now := time.Now()
	states := make(map[int64]openAIAdaptiveAccountState, len(accounts))
	stableCapacities := make(map[int64]int, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		state := defaultOpenAIAdaptiveAccountState(acc.ID, cfg)
		if stateStore != nil {
			state = stateStore.snapshot(acc.ID, cfg)
		}
		states[acc.ID] = state
		stable := stableOpenAIAdaptiveCapacity(acc, state, cfg)
		stableCapacities[acc.ID] = stable
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: stable,
		})
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if realtimeEnabled {
		loadMap = s.getOpenAIAdaptiveLearningLoadMapBestEffort(ctx, loadReq)
	}

	rows := make([]OpenAIAdaptiveSchedulerAccountLearningSnapshot, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		state := states[acc.ID]
		loadInfo := loadMap[acc.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: acc.ID}
		}
		stable := stableCapacities[acc.ID]
		effective := effectiveOpenAIAdaptiveCapacityWithLoad(acc, state, cfg, loadInfo)
		row := buildOpenAIAdaptiveLearningAccountSnapshot(acc, state, cfg, loadInfo, stable, effective, now, cfg.OpenAIAdaptiveSchedulerEnabled)
		rows = append(rows, row)
	}
	applyOpenAIAdaptiveLearningScores(rows, accounts, states, loadMap, cfg)
	rows = filterOpenAIAdaptiveLearningRowsByStatus(rows, filter.Status)
	rows = filterOpenAIAdaptiveLearningRowsByTime(rows, filter.StartTime, filter.EndTime)
	sortOpenAIAdaptiveLearningRows(rows, filter.SortBy, filter.SortOrder)

	summary := summarizeOpenAIAdaptiveLearningRows(rows)
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

	return &OpenAIAdaptiveSchedulerLearningSnapshot{
		Enabled:          cfg.OpenAIAdaptiveSchedulerEnabled,
		Mode:             cfg.OpenAIAdaptiveSchedulerMode,
		RealtimeEnabled:  realtimeEnabled,
		GeneratedAt:      now.UTC(),
		TotalAccounts:    total,
		Total:            total,
		ReturnedAccounts: len(rows),
		Limit:            limit,
		TimeRange:        filter.TimeRange,
		StartTime:        filter.StartTime.UTC(),
		EndTime:          filter.EndTime.UTC(),
		Page:             filter.Page,
		PageSize:         filter.PageSize,
		TopN:             filter.TopN,
		SortBy:           filter.SortBy,
		SortOrder:        filter.SortOrder,
		Settings:         openAIAdaptiveLearningSettingsSnapshot(cfg),
		Summary:          summary,
		Accounts:         rows,
	}, nil
}

func normalizeOpenAIAdaptiveLearningFilter(filter *OpenAIAdaptiveSchedulerLearningFilter) {
	if filter == nil {
		return
	}
	if filter.TopN > openAIAdaptiveLearningMaxLimit {
		filter.TopN = openAIAdaptiveLearningMaxLimit
	}
	if filter.TopN < 0 {
		filter.TopN = 0
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = openAIAdaptiveLearningDefaultLimit
	}
	if filter.PageSize > openAIAdaptiveLearningMaxLimit {
		filter.PageSize = openAIAdaptiveLearningMaxLimit
	}
	filter.SortBy = normalizeOpenAIAdaptiveLearningSortBy(filter.SortBy)
	filter.SortOrder = normalizeOpenAIAdaptiveLearningSortOrder(filter.SortOrder)
	filter.Status = normalizeOpenAIAdaptiveLearningStatusFilter(filter.Status)
}

func (s *OpenAIGatewayService) openAIAdaptiveSchedulerStateStoreForSnapshot() *openAIAdaptiveSchedulerStateStore {
	if s == nil {
		return nil
	}
	s.openaiSchedulerMu.Lock()
	defer s.openaiSchedulerMu.Unlock()
	scheduler, _ := s.openaiScheduler.(*adaptiveOpenAIAccountScheduler)
	if scheduler == nil {
		return nil
	}
	return scheduler.state
}

func (s *OpsService) getOpenAIAdaptiveLearningLoadMapBestEffort(
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
			log.Printf("[Ops] adaptive learning GetAccountsLoadBatch failed: %v", err)
			continue
		}
		for k, v := range part {
			out[k] = v
		}
	}
	return out
}

func filterOpenAIAdaptiveLearningAccountsByGroup(accounts []Account, groupIDFilter *int64) []Account {
	if groupIDFilter == nil || *groupIDFilter <= 0 {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		for _, group := range acc.Groups {
			if group != nil && group.ID == *groupIDFilter {
				out = append(out, acc)
				break
			}
		}
	}
	return out
}

func filterOpenAIAdaptiveLearningSchedulableAccounts(accounts []Account) []Account {
	if len(accounts) == 0 {
		return accounts
	}
	out := accounts[:0]
	for _, acc := range accounts {
		if acc.Schedulable {
			out = append(out, acc)
		}
	}
	return out
}

func buildOpenAIAdaptiveLearningAccountSnapshot(
	account *Account,
	state openAIAdaptiveAccountState,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadInfo *AccountLoadInfo,
	stableCapacity int,
	effectiveCapacity int,
	now time.Time,
	adaptiveEnabled bool,
) OpenAIAdaptiveSchedulerAccountLearningSnapshot {
	if loadInfo == nil {
		loadInfo = &AccountLoadInfo{}
	}
	recentFailureRate := 0.0
	if state.RecentSamples > 0 {
		recentFailureRate = float64(state.RecentFailures) / float64(state.RecentSamples)
	}
	burstCapacity := effectiveCapacity - stableCapacity
	if burstCapacity < 0 {
		burstCapacity = 0
	}
	status, reason := openAIAdaptiveLearningAccountStatus(account, state, cfg, loadInfo, effectiveCapacity, recentFailureRate, now, adaptiveEnabled)
	cooldownUntil := timePtrIfNotZero(state.CooldownUntil)
	cooldownRemaining := int64(0)
	if state.CooldownUntil.After(now) {
		cooldownRemaining = int64(state.CooldownUntil.Sub(now).Seconds())
		if cooldownRemaining < 1 {
			cooldownRemaining = 1
		}
	}
	loadPercentage := adaptiveLoadRate(loadInfo, effectiveCapacity)
	return OpenAIAdaptiveSchedulerAccountLearningSnapshot{
		AccountID:                  account.ID,
		AccountName:                account.Name,
		Platform:                   account.Platform,
		Type:                       account.Type,
		AccountStatus:              account.Status,
		Schedulable:                account.IsSchedulable(),
		Priority:                   account.Priority,
		ConfiguredConcurrency:      account.Concurrency,
		StableCapacity:             stableCapacity,
		EffectiveCapacity:          effectiveCapacity,
		BurstCapacity:              burstCapacity,
		RateMultiplier:             account.BillingRateMultiplier(),
		CurrentConcurrency:         loadInfo.CurrentConcurrency,
		WaitingCount:               loadInfo.WaitingCount,
		LoadPercentage:             loadPercentage,
		SchedulerStatus:            status,
		StatusReason:               reason,
		Learned:                    state.TotalSamples > 0,
		SuccessEMA:                 state.SuccessEMA,
		ErrorEMA:                   state.ErrorEMA,
		LatencyEMA:                 state.LatencyEMA,
		TTFTEMA:                    state.TTFTEMA,
		TotalSamples:               state.TotalSamples,
		RecentSamples:              state.RecentSamples,
		RecentFailures:             state.RecentFailures,
		RecentFailureRate:          recentFailureRate,
		ConsecutiveSuccess:         state.ConsecutiveSuccess,
		ConsecutiveFailure:         state.ConsecutiveFailure,
		ConsecutiveCapacityFailure: state.ConsecutiveCapacityFailure,
		LearningWindowStartedAt:    timePtrIfNotZero(state.RecentWindowStartedAt),
		LastSuccessAt:              timePtrIfNotZero(state.LastSuccessAt),
		LastFailureAt:              timePtrIfNotZero(state.LastFailureAt),
		LastCapacityFailureAt:      timePtrIfNotZero(state.LastCapacityFailureAt),
		CooldownUntil:              cooldownUntil,
		CooldownRemainingSec:       cooldownRemaining,
	}
}

func openAIAdaptiveLearningAccountStatus(
	account *Account,
	state openAIAdaptiveAccountState,
	cfg OpenAIAdaptiveSchedulerSettings,
	loadInfo *AccountLoadInfo,
	effectiveCapacity int,
	recentFailureRate float64,
	now time.Time,
	adaptiveEnabled bool,
) (string, string) {
	if !adaptiveEnabled {
		return OpenAIAdaptiveLearningStatusDisabled, "adaptive scheduler disabled"
	}
	if account == nil || !account.IsSchedulable() {
		if account != nil && account.ErrorMessage != "" {
			return OpenAIAdaptiveLearningStatusUnavailable, account.ErrorMessage
		}
		return OpenAIAdaptiveLearningStatusUnavailable, "account is not schedulable"
	}
	if state.CooldownUntil.After(now) {
		return OpenAIAdaptiveLearningStatusCooldown, "adaptive cooldown after capacity failures"
	}
	if shouldUseOpenAIAdaptiveHalfOpenProbe(state, cfg, now) {
		return OpenAIAdaptiveLearningStatusHalfOpen, "probing with half-open capacity"
	}
	if state.ErrorEMA >= cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold ||
		recentFailureRate >= cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold {
		return OpenAIAdaptiveLearningStatusHighError, "recent error signal reached shrink threshold"
	}
	if effectiveCapacity > 0 && loadInfo != nil && loadInfo.CurrentConcurrency >= effectiveCapacity {
		return OpenAIAdaptiveLearningStatusSaturated, "current concurrency reached adaptive capacity"
	}
	if state.TotalSamples == 0 {
		return OpenAIAdaptiveLearningStatusUnlearned, "no runtime samples yet"
	}
	if state.TotalSamples < int64(cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink) {
		return OpenAIAdaptiveLearningStatusLearning, "sample count below shrink confidence threshold"
	}
	return OpenAIAdaptiveLearningStatusHealthy, ""
}

func applyOpenAIAdaptiveLearningScores(
	rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot,
	accounts []Account,
	states map[int64]openAIAdaptiveAccountState,
	loadMap map[int64]*AccountLoadInfo,
	cfg OpenAIAdaptiveSchedulerSettings,
) {
	if len(rows) == 0 {
		return
	}
	rowByID := make(map[int64]*OpenAIAdaptiveSchedulerAccountLearningSnapshot, len(rows))
	for i := range rows {
		rowByID[rows[i].AccountID] = &rows[i]
	}
	candidates := make([]openAIAdaptiveCandidateScore, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		row := rowByID[account.ID]
		if row == nil || row.SchedulerStatus == OpenAIAdaptiveLearningStatusUnavailable {
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		candidates = append(candidates, openAIAdaptiveCandidateScore{
			account:           account,
			loadInfo:          loadInfo,
			state:             states[account.ID],
			effectiveCapacity: row.EffectiveCapacity,
		})
	}
	if len(candidates) == 0 {
		return
	}
	applyOpenAIAdaptiveScores(candidates, cfg)
	for _, candidate := range candidates {
		if candidate.account == nil {
			continue
		}
		row := rowByID[candidate.account.ID]
		if row == nil {
			continue
		}
		row.SchedulerScore = candidate.score
		row.SuccessScore = candidate.successScore
		row.CostScore = candidate.costScore
		row.CapacityScore = candidate.capacityScore
		row.LatencyScore = candidate.latencyScore
		row.StabilityScore = candidate.stabilityScore
		row.ExplorationScore = candidate.explorationScore
	}
}

func filterOpenAIAdaptiveLearningRowsByTime(
	rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot,
	start time.Time,
	end time.Time,
) []OpenAIAdaptiveSchedulerAccountLearningSnapshot {
	if len(rows) == 0 || start.IsZero() || end.IsZero() || !end.After(start) {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		lastEvent := openAIAdaptiveLearningLastEventTime(row)
		if lastEvent.IsZero() || (!lastEvent.Before(start) && lastEvent.Before(end.Add(time.Nanosecond))) {
			out = append(out, row)
		}
	}
	return out
}

func filterOpenAIAdaptiveLearningRowsByStatus(
	rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot,
	status string,
) []OpenAIAdaptiveSchedulerAccountLearningSnapshot {
	status = normalizeOpenAIAdaptiveLearningStatusFilter(status)
	if status == "" || len(rows) == 0 {
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

func sortOpenAIAdaptiveLearningRows(rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot, sortBy string, sortOrder string) {
	sortBy = normalizeOpenAIAdaptiveLearningSortBy(sortBy)
	sortOrder = normalizeOpenAIAdaptiveLearningSortOrder(sortOrder)
	if sortBy != "" {
		sort.SliceStable(rows, func(i, j int) bool {
			cmp := compareOpenAIAdaptiveLearningRows(rows[i], rows[j], sortBy)
			if cmp == 0 {
				return compareOpenAIAdaptiveLearningRows(rows[i], rows[j], "default") < 0
			}
			if sortOrder == "asc" {
				return cmp < 0
			}
			return cmp > 0
		})
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return compareOpenAIAdaptiveLearningRows(rows[i], rows[j], "default") < 0
	})
}

func normalizeOpenAIAdaptiveLearningSortBy(value string) string {
	switch strings.TrimSpace(value) {
	case "account", "status", "capacity", "load", "score", "samples", "error", "last_event":
		return strings.TrimSpace(value)
	case "default", "":
		return ""
	default:
		return ""
	}
}

func normalizeOpenAIAdaptiveLearningStatusFilter(value string) string {
	switch strings.TrimSpace(value) {
	case OpenAIAdaptiveLearningStatusDisabled,
		OpenAIAdaptiveLearningStatusUnavailable,
		OpenAIAdaptiveLearningStatusCooldown,
		OpenAIAdaptiveLearningStatusHalfOpen,
		OpenAIAdaptiveLearningStatusHighError,
		OpenAIAdaptiveLearningStatusSaturated,
		OpenAIAdaptiveLearningStatusLearning,
		OpenAIAdaptiveLearningStatusUnlearned,
		OpenAIAdaptiveLearningStatusHealthy:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeOpenAIAdaptiveLearningSortOrder(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "asc") {
		return "asc"
	}
	return "desc"
}

func compareOpenAIAdaptiveLearningRows(left, right OpenAIAdaptiveSchedulerAccountLearningSnapshot, sortBy string) int {
	switch sortBy {
	case "account":
		if cmp := strings.Compare(strings.ToLower(left.AccountName), strings.ToLower(right.AccountName)); cmp != 0 {
			return cmp
		}
	case "status":
		if cmp := compareInt(openAIAdaptiveLearningStatusRank(left.SchedulerStatus), openAIAdaptiveLearningStatusRank(right.SchedulerStatus)); cmp != 0 {
			return -cmp
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
		if cmp := compareFloat64(left.ErrorEMA, right.ErrorEMA); cmp != 0 {
			return cmp
		}
	case "last_event":
		if cmp := compareTime(openAIAdaptiveLearningLastEventTime(left), openAIAdaptiveLearningLastEventTime(right)); cmp != 0 {
			return cmp
		}
	default:
		leftRank := openAIAdaptiveLearningStatusRank(left.SchedulerStatus)
		rightRank := openAIAdaptiveLearningStatusRank(right.SchedulerStatus)
		if leftRank != rightRank {
			return compareInt(leftRank, rightRank)
		}
		if left.LoadPercentage != right.LoadPercentage {
			return compareFloat64(right.LoadPercentage, left.LoadPercentage)
		}
		if left.ErrorEMA != right.ErrorEMA {
			return compareFloat64(right.ErrorEMA, left.ErrorEMA)
		}
		if left.SchedulerScore != right.SchedulerScore {
			return compareFloat64(left.SchedulerScore, right.SchedulerScore)
		}
		if left.Priority != right.Priority {
			return compareInt(left.Priority, right.Priority)
		}
	}
	return compareInt64(left.AccountID, right.AccountID)
}

func openAIAdaptiveLearningLastEventTime(row OpenAIAdaptiveSchedulerAccountLearningSnapshot) time.Time {
	var latest time.Time
	for _, candidate := range []*time.Time{
		row.LastSuccessAt,
		row.LastFailureAt,
		row.LastCapacityFailureAt,
		row.CooldownUntil,
		row.LearningWindowStartedAt,
	} {
		if candidate != nil && candidate.After(latest) {
			latest = *candidate
		}
	}
	return latest
}

func compareInt(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareFloat64(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareTime(left, right time.Time) int {
	if left.Before(right) {
		return -1
	}
	if left.After(right) {
		return 1
	}
	return 0
}

func openAIAdaptiveLearningStatusRank(status string) int {
	switch status {
	case OpenAIAdaptiveLearningStatusCooldown:
		return 0
	case OpenAIAdaptiveLearningStatusHalfOpen:
		return 1
	case OpenAIAdaptiveLearningStatusHighError:
		return 2
	case OpenAIAdaptiveLearningStatusSaturated:
		return 3
	case OpenAIAdaptiveLearningStatusUnavailable:
		return 4
	case OpenAIAdaptiveLearningStatusLearning:
		return 5
	case OpenAIAdaptiveLearningStatusUnlearned:
		return 6
	case OpenAIAdaptiveLearningStatusDisabled:
		return 7
	default:
		return 8
	}
}

func summarizeOpenAIAdaptiveLearningRows(rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot) OpenAIAdaptiveSchedulerLearningSummary {
	var summary OpenAIAdaptiveSchedulerLearningSummary
	for _, row := range rows {
		if row.Learned {
			summary.TrackedAccounts++
		}
		switch row.SchedulerStatus {
		case OpenAIAdaptiveLearningStatusUnavailable:
			summary.UnavailableAccounts++
		case OpenAIAdaptiveLearningStatusCooldown:
			summary.CooldownAccounts++
		case OpenAIAdaptiveLearningStatusHalfOpen:
			summary.HalfOpenAccounts++
		case OpenAIAdaptiveLearningStatusHighError:
			summary.HighErrorAccounts++
		case OpenAIAdaptiveLearningStatusSaturated:
			summary.SaturatedAccounts++
		case OpenAIAdaptiveLearningStatusLearning:
			summary.LearningAccounts++
		case OpenAIAdaptiveLearningStatusUnlearned:
			summary.UnlearnedAccounts++
		case OpenAIAdaptiveLearningStatusHealthy:
			summary.HealthyAccounts++
		}
	}
	return summary
}

func openAIAdaptiveLearningSettingsSnapshot(cfg OpenAIAdaptiveSchedulerSettings) OpenAIAdaptiveSchedulerLearningSettingsSnapshot {
	return OpenAIAdaptiveSchedulerLearningSettingsSnapshot{
		TopK:                      cfg.OpenAIAdaptiveSchedulerTopK,
		AccountTypePriorityMode:   cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode,
		ExplorationRate:           cfg.OpenAIAdaptiveSchedulerExplorationRate,
		SoftmaxTemperature:        cfg.OpenAIAdaptiveSchedulerSoftmaxTemperature,
		InitialCapacityFraction:   cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction,
		MinCapacity:               cfg.OpenAIAdaptiveSchedulerMinCapacity,
		CapacityGrowthFactor:      cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor,
		BurstProbeRatio:           cfg.OpenAIAdaptiveSchedulerBurstProbeRatio,
		CapacityFailureThreshold:  cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold,
		MinRecentSamplesForShrink: cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink,
		ShrinkErrorThreshold:      cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold,
		ShrinkFactorSoft:          cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft,
		ShrinkFactorHard:          cfg.OpenAIAdaptiveSchedulerShrinkFactorHard,
		HalfOpenFailureThreshold:  cfg.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold,
		HalfOpenProbeCapacity:     cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity,
		LearningWindowSeconds:     cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds,
	}
}

func timePtrIfNotZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
