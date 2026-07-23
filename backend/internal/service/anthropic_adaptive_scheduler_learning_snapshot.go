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

	Settings AnthropicAdaptiveSchedulerLearningSettingsSnapshot  `json:"settings"`
	Summary  AnthropicAdaptiveSchedulerLearningSummary           `json:"summary"`
	Accounts []AnthropicAdaptiveSchedulerAccountLearningSnapshot `json:"accounts"`
}

type AnthropicAdaptiveSchedulerLearningFilter struct {
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

func (f *AnthropicAdaptiveSchedulerLearningFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type AnthropicAdaptiveSchedulerLearningSettingsSnapshot struct {
	TopK                        int     `json:"top_k"`
	SoftmaxTemperature          float64 `json:"softmax_temperature"`
	WeightReliability           float64 `json:"weight_reliability"`
	WeightCapacity              float64 `json:"weight_capacity"`
	WeightLatency               float64 `json:"weight_latency"`
	WeightExploration           float64 `json:"weight_exploration"`
	InitialReliability          float64 `json:"initial_reliability"`
	ConsecutiveFailurePenalty   float64 `json:"consecutive_failure_penalty"`
	NeutralLatencyScore         float64 `json:"neutral_latency_score"`
	SuccessEMAAlpha             float64 `json:"success_ema_alpha"`
	LatencyEMAAlpha             float64 `json:"latency_ema_alpha"`
	CapacitySuccessThreshold    float64 `json:"capacity_success_threshold"`
	CapacityProbeLoadThreshold  float64 `json:"capacity_probe_load_threshold"`
	CapacityFailureThreshold    int     `json:"capacity_failure_threshold"`
	MinRecentSamplesForShrink   int     `json:"min_recent_samples_for_shrink"`
	ShrinkErrorThreshold        float64 `json:"shrink_error_threshold"`
	LearningWindowSeconds       int     `json:"learning_window_seconds"`
	CooldownSeconds             int     `json:"cooldown_seconds"`
	ShrinkFactorSoft            float64 `json:"shrink_factor_soft"`
	ShrinkFactorHard            float64 `json:"shrink_factor_hard"`
	CapacityIncreaseStep        int     `json:"capacity_increase_step"`
	MinCapacity                 int     `json:"min_capacity"`
	HardShrinkFailureMultiplier int     `json:"hard_shrink_failure_multiplier"`
}

type AnthropicAdaptiveSchedulerLearningSummary struct {
	TrackedAccounts     int `json:"tracked_accounts"`
	DisabledAccounts    int `json:"disabled_accounts"`
	UnlearnedAccounts   int `json:"unlearned_accounts"`
	LearningAccounts    int `json:"learning_accounts"`
	HealthyAccounts     int `json:"healthy_accounts"`
	HighErrorAccounts   int `json:"high_error_accounts"`
	CooldownAccounts    int `json:"cooldown_accounts"`
	SaturatedAccounts   int `json:"saturated_accounts"`
	UnavailableAccounts int `json:"unavailable_accounts"`
}

type AnthropicAdaptiveLatencyLearningSnapshot struct {
	ModelFamily string  `json:"model_family"`
	TTFTEMA     float64 `json:"ttft_ema"`
	LatencyEMA  float64 `json:"latency_ema"`
	Samples     int64   `json:"samples"`
}

type AnthropicAdaptiveSchedulerAccountLearningSnapshot struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	Platform      string `json:"platform"`
	Type          string `json:"type"`
	AccountStatus string `json:"account_status"`
	Schedulable   bool   `json:"schedulable"`
	Priority      int    `json:"priority"`

	ConfiguredConcurrency int `json:"configured_concurrency"`
	EstimatedCapacity     int `json:"estimated_capacity"`
	EffectiveCapacity     int `json:"effective_capacity"`

	CurrentConcurrency int     `json:"current_concurrency"`
	WaitingCount       int     `json:"waiting_count"`
	LoadPercentage     float64 `json:"load_percentage"`

	SchedulerStatus string `json:"scheduler_status"`
	StatusReason    string `json:"status_reason,omitempty"`
	Learned         bool   `json:"learned"`

	SchedulerScore   float64 `json:"scheduler_score"`
	ReliabilityScore float64 `json:"reliability_score"`
	CapacityScore    float64 `json:"capacity_score"`
	LatencyScore     float64 `json:"latency_score"`
	ExplorationScore float64 `json:"exploration_score"`

	SuccessEMA     float64 `json:"success_ema"`
	ModelFamily    string  `json:"model_family"`
	TTFTEMA        float64 `json:"ttft_ema"`
	LatencyEMA     float64 `json:"latency_ema"`
	LatencySamples int64   `json:"latency_samples"`

	LatencyByModelFamily []AnthropicAdaptiveLatencyLearningSnapshot `json:"latency_by_model_family"`

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
	var stateStore *anthropicAdaptiveStateStore
	if s != nil && s.gatewayService != nil {
		cfg = s.gatewayService.anthropicAdaptiveSchedulerSettingsForSnapshot(ctx)
		stateStore = s.gatewayService.anthropicAdaptiveSchedulerStateStoreForSnapshot()
	}
	realtimeEnabled := s.IsRealtimeMonitoringEnabled(ctx)

	accounts, err := s.listAllAccountsForOps(ctx, PlatformAnthropic, filter.GroupID)
	if err != nil {
		return nil, err
	}
	accounts = filterAnthropicAdaptiveLearningAccountsByGroup(accounts, filter.GroupID)
	accounts = filterAnthropicAdaptiveLearningSchedulableAccounts(accounts)

	now := time.Now()
	states := make(map[int64]anthropicAdaptiveAccountState, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		state := defaultAnthropicAdaptiveAccountState(account, now, cfg)
		if stateStore != nil {
			state = stateStore.snapshot(account, cfg)
		}
		states[account.ID] = state
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: normalizedAnthropicAdaptiveCapacity(account, state),
		})
	}

	loadMap := map[int64]*AccountLoadInfo{}
	if realtimeEnabled {
		loadMap = s.getAnthropicAdaptiveLearningLoadMapBestEffort(ctx, loadReq)
	}

	rows := make([]AnthropicAdaptiveSchedulerAccountLearningSnapshot, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		state := states[account.ID]
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		rows = append(rows, buildAnthropicAdaptiveLearningAccountSnapshot(
			account,
			state,
			cfg,
			loadInfo,
			filter.RequestedModel,
			now,
			cfg.AnthropicAdaptiveSchedulerEnabled,
		))
	}
	applyAnthropicAdaptiveLearningScores(rows, accounts, states, loadMap, filter.RequestedModel, cfg)
	rows = filterAnthropicAdaptiveLearningRowsByStatus(rows, filter.Status)
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
		RequestedModel:   filter.RequestedModel,
		ModelFamily:      anthropicAdaptiveModelFamily(filter.RequestedModel),
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
	filter.RequestedModel = strings.TrimSpace(filter.RequestedModel)
	filter.SortBy = normalizeAnthropicAdaptiveLearningSortBy(filter.SortBy)
	filter.SortOrder = normalizeAnthropicAdaptiveLearningSortOrder(filter.SortOrder)
	filter.Status = normalizeAnthropicAdaptiveLearningStatusFilter(filter.Status)
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

func (s *GatewayService) anthropicAdaptiveSchedulerStateStoreForSnapshot() *anthropicAdaptiveStateStore {
	if s == nil || s.anthropicAdaptiveScheduler == nil {
		return nil
	}
	return s.anthropicAdaptiveScheduler.state
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

func normalizedAnthropicAdaptiveCapacity(account *Account, state anthropicAdaptiveAccountState) int {
	if account == nil || account.Concurrency <= 0 {
		return 0
	}
	capacity := state.EstimatedCapacity
	if capacity <= 0 || capacity > account.Concurrency {
		capacity = account.Concurrency
	}
	return capacity
}

func buildAnthropicAdaptiveLearningAccountSnapshot(
	account *Account,
	state anthropicAdaptiveAccountState,
	cfg AnthropicAdaptiveSchedulerSettings,
	loadInfo *AccountLoadInfo,
	requestedModel string,
	now time.Time,
	adaptiveEnabled bool,
) AnthropicAdaptiveSchedulerAccountLearningSnapshot {
	if loadInfo == nil {
		loadInfo = &AccountLoadInfo{}
	}
	effectiveCapacity := normalizedAnthropicAdaptiveCapacity(account, state)
	healthFailureRate := adaptiveFailureRate(state.RecentHealthFailures, state.RecentHealthSamples)
	capacityFailureRate := adaptiveFailureRate(state.RecentCapacityFailures, state.RecentCapacitySamples)
	status, reason := anthropicAdaptiveLearningAccountStatus(
		account,
		state,
		cfg,
		loadInfo,
		effectiveCapacity,
		capacityFailureRate,
		now,
		adaptiveEnabled,
	)
	cooldownRemaining := int64(0)
	if state.CooldownUntil.After(now) {
		cooldownRemaining = int64(state.CooldownUntil.Sub(now).Seconds())
		if cooldownRemaining < 1 {
			cooldownRemaining = 1
		}
	}
	family := anthropicAdaptiveModelFamily(requestedModel)
	latency := state.LatencyByModelFamily[family]
	return AnthropicAdaptiveSchedulerAccountLearningSnapshot{
		AccountID:                  account.ID,
		AccountName:                account.Name,
		Platform:                   account.Platform,
		Type:                       account.Type,
		AccountStatus:              account.Status,
		Schedulable:                account.IsSchedulable(),
		Priority:                   account.Priority,
		ConfiguredConcurrency:      account.Concurrency,
		EstimatedCapacity:          effectiveCapacity,
		EffectiveCapacity:          effectiveCapacity,
		CurrentConcurrency:         loadInfo.CurrentConcurrency,
		WaitingCount:               loadInfo.WaitingCount,
		LoadPercentage:             adaptiveLoadRate(loadInfo, effectiveCapacity),
		SchedulerStatus:            status,
		StatusReason:               reason,
		Learned:                    state.TotalSamples > 0,
		SuccessEMA:                 state.SuccessEMA,
		ModelFamily:                family,
		TTFTEMA:                    latency.TTFTEMA,
		LatencyEMA:                 latency.LatencyEMA,
		LatencySamples:             latency.Samples,
		LatencyByModelFamily:       anthropicAdaptiveLatencySnapshots(state.LatencyByModelFamily),
		TotalSamples:               state.TotalSamples,
		RecentHealthSamples:        state.RecentHealthSamples,
		RecentHealthFailures:       state.RecentHealthFailures,
		RecentHealthFailureRate:    healthFailureRate,
		RecentCapacitySamples:      state.RecentCapacitySamples,
		RecentCapacityFailures:     state.RecentCapacityFailures,
		RecentCapacityFailureRate:  capacityFailureRate,
		ConsecutiveSuccess:         state.ConsecutiveSuccess,
		ConsecutiveFailure:         state.ConsecutiveFailure,
		ConsecutiveCapacityFailure: state.ConsecutiveCapacityFailure,
		LearningWindowStartedAt:    anthropicAdaptiveTimePtrIfNotZero(state.RecentWindowStartedAt),
		LastSuccessAt:              anthropicAdaptiveTimePtrIfNotZero(state.LastSuccessAt),
		LastFailureAt:              anthropicAdaptiveTimePtrIfNotZero(state.LastFailureAt),
		LastCapacityFailureAt:      anthropicAdaptiveTimePtrIfNotZero(state.LastCapacityFailureAt),
		CooldownUntil:              anthropicAdaptiveTimePtrIfNotZero(state.CooldownUntil),
		CooldownRemainingSec:       cooldownRemaining,
	}
}

func adaptiveFailureRate(failures int, samples int) float64 {
	if samples <= 0 {
		return 0
	}
	return float64(failures) / float64(samples)
}

func anthropicAdaptiveLatencySnapshots(
	latencies map[string]anthropicAdaptiveLatencyState,
) []AnthropicAdaptiveLatencyLearningSnapshot {
	if len(latencies) == 0 {
		return []AnthropicAdaptiveLatencyLearningSnapshot{}
	}
	out := make([]AnthropicAdaptiveLatencyLearningSnapshot, 0, len(latencies))
	for family, latency := range latencies {
		out = append(out, AnthropicAdaptiveLatencyLearningSnapshot{
			ModelFamily: family,
			TTFTEMA:     latency.TTFTEMA,
			LatencyEMA:  latency.LatencyEMA,
			Samples:     latency.Samples,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModelFamily < out[j].ModelFamily
	})
	return out
}

func anthropicAdaptiveLearningAccountStatus(
	account *Account,
	state anthropicAdaptiveAccountState,
	cfg AnthropicAdaptiveSchedulerSettings,
	loadInfo *AccountLoadInfo,
	effectiveCapacity int,
	capacityFailureRate float64,
	now time.Time,
	adaptiveEnabled bool,
) (string, string) {
	if !adaptiveEnabled {
		return AnthropicAdaptiveLearningStatusDisabled, "adaptive scheduler disabled"
	}
	if account == nil || !account.IsSchedulable() {
		if account != nil && account.ErrorMessage != "" {
			return AnthropicAdaptiveLearningStatusUnavailable, account.ErrorMessage
		}
		return AnthropicAdaptiveLearningStatusUnavailable, "account is not schedulable"
	}
	if state.CooldownUntil.After(now) {
		return AnthropicAdaptiveLearningStatusCooldown, "adaptive cooldown after capacity failures"
	}
	if (state.RecentCapacitySamples > 0 && capacityFailureRate >= cfg.AnthropicAdaptiveSchedulerShrinkErrorThreshold) ||
		state.ConsecutiveCapacityFailure >= cfg.AnthropicAdaptiveSchedulerCapacityFailureThreshold {
		return AnthropicAdaptiveLearningStatusHighError, "capacity failure signal reached shrink threshold"
	}
	if effectiveCapacity > 0 && loadInfo != nil &&
		(loadInfo.CurrentConcurrency >= effectiveCapacity || loadInfo.WaitingCount > 0) {
		return AnthropicAdaptiveLearningStatusSaturated, "current load reached adaptive capacity"
	}
	if state.TotalSamples == 0 {
		return AnthropicAdaptiveLearningStatusUnlearned, "no runtime samples yet"
	}
	if state.TotalSamples < int64(cfg.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink) {
		return AnthropicAdaptiveLearningStatusLearning, "sample count below shrink confidence threshold"
	}
	return AnthropicAdaptiveLearningStatusHealthy, ""
}

func applyAnthropicAdaptiveLearningScores(
	rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot,
	accounts []Account,
	states map[int64]anthropicAdaptiveAccountState,
	loadMap map[int64]*AccountLoadInfo,
	requestedModel string,
	cfg AnthropicAdaptiveSchedulerSettings,
) {
	if len(rows) == 0 {
		return
	}
	rowByID := make(map[int64]*AnthropicAdaptiveSchedulerAccountLearningSnapshot, len(rows))
	for i := range rows {
		rowByID[rows[i].AccountID] = &rows[i]
	}
	candidates := make([]AnthropicAdaptiveCandidate, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		row := rowByID[account.ID]
		if row == nil || row.SchedulerStatus == AnthropicAdaptiveLearningStatusUnavailable {
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		candidates = append(candidates, AnthropicAdaptiveCandidate{
			Account:           account,
			LoadInfo:          loadInfo,
			EffectiveCapacity: row.EffectiveCapacity,
			state:             states[account.ID],
		})
	}
	if len(candidates) == 0 {
		return
	}
	applyAnthropicAdaptiveScores(candidates, requestedModel, cfg)
	for _, candidate := range candidates {
		if candidate.Account == nil {
			continue
		}
		row := rowByID[candidate.Account.ID]
		if row == nil {
			continue
		}
		row.SchedulerScore = candidate.Score
		row.ReliabilityScore = candidate.ReliabilityScore
		row.CapacityScore = candidate.CapacityScore
		row.LatencyScore = candidate.LatencyScore
		row.ExplorationScore = candidate.ExplorationScore
	}
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

func filterAnthropicAdaptiveLearningRowsByStatus(
	rows []AnthropicAdaptiveSchedulerAccountLearningSnapshot,
	status string,
) []AnthropicAdaptiveSchedulerAccountLearningSnapshot {
	status = normalizeAnthropicAdaptiveLearningStatusFilter(status)
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

func normalizeAnthropicAdaptiveLearningStatusFilter(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case AnthropicAdaptiveLearningStatusDisabled,
		AnthropicAdaptiveLearningStatusUnavailable,
		AnthropicAdaptiveLearningStatusCooldown,
		AnthropicAdaptiveLearningStatusHighError,
		AnthropicAdaptiveLearningStatusSaturated,
		AnthropicAdaptiveLearningStatusLearning,
		AnthropicAdaptiveLearningStatusUnlearned,
		AnthropicAdaptiveLearningStatusHealthy:
		return value
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
		if cmp := compareAnthropicAdaptiveLearningFloat64(left.RecentCapacityFailureRate, right.RecentCapacityFailureRate); cmp != 0 {
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
		if left.RecentCapacityFailureRate != right.RecentCapacityFailureRate {
			return compareAnthropicAdaptiveLearningFloat64(right.RecentCapacityFailureRate, left.RecentCapacityFailureRate)
		}
		if left.SchedulerScore != right.SchedulerScore {
			return compareAnthropicAdaptiveLearningFloat64(left.SchedulerScore, right.SchedulerScore)
		}
		if left.Priority != right.Priority {
			return compareAnthropicAdaptiveLearningInt(left.Priority, right.Priority)
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
		if row.Learned {
			summary.TrackedAccounts++
		}
		switch row.SchedulerStatus {
		case AnthropicAdaptiveLearningStatusDisabled:
			summary.DisabledAccounts++
		case AnthropicAdaptiveLearningStatusUnavailable:
			summary.UnavailableAccounts++
		case AnthropicAdaptiveLearningStatusCooldown:
			summary.CooldownAccounts++
		case AnthropicAdaptiveLearningStatusHighError:
			summary.HighErrorAccounts++
		case AnthropicAdaptiveLearningStatusSaturated:
			summary.SaturatedAccounts++
		case AnthropicAdaptiveLearningStatusLearning:
			summary.LearningAccounts++
		case AnthropicAdaptiveLearningStatusUnlearned:
			summary.UnlearnedAccounts++
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
		TopK:                        cfg.AnthropicAdaptiveSchedulerTopK,
		SoftmaxTemperature:          cfg.AnthropicAdaptiveSchedulerSoftmaxTemperature,
		WeightReliability:           cfg.AnthropicAdaptiveSchedulerWeightReliability,
		WeightCapacity:              cfg.AnthropicAdaptiveSchedulerWeightCapacity,
		WeightLatency:               cfg.AnthropicAdaptiveSchedulerWeightLatency,
		WeightExploration:           cfg.AnthropicAdaptiveSchedulerWeightExploration,
		InitialReliability:          cfg.AnthropicAdaptiveSchedulerInitialReliability,
		ConsecutiveFailurePenalty:   cfg.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty,
		NeutralLatencyScore:         cfg.AnthropicAdaptiveSchedulerNeutralLatencyScore,
		SuccessEMAAlpha:             cfg.AnthropicAdaptiveSchedulerSuccessEMAAlpha,
		LatencyEMAAlpha:             cfg.AnthropicAdaptiveSchedulerLatencyEMAAlpha,
		CapacitySuccessThreshold:    cfg.AnthropicAdaptiveSchedulerCapacitySuccessThreshold,
		CapacityProbeLoadThreshold:  cfg.AnthropicAdaptiveSchedulerCapacityProbeLoadThreshold,
		CapacityFailureThreshold:    cfg.AnthropicAdaptiveSchedulerCapacityFailureThreshold,
		MinRecentSamplesForShrink:   cfg.AnthropicAdaptiveSchedulerMinRecentSamplesForShrink,
		ShrinkErrorThreshold:        cfg.AnthropicAdaptiveSchedulerShrinkErrorThreshold,
		LearningWindowSeconds:       cfg.AnthropicAdaptiveSchedulerLearningWindowSeconds,
		CooldownSeconds:             cfg.AnthropicAdaptiveSchedulerCooldownSeconds,
		ShrinkFactorSoft:            cfg.AnthropicAdaptiveSchedulerShrinkFactorSoft,
		ShrinkFactorHard:            cfg.AnthropicAdaptiveSchedulerShrinkFactorHard,
		CapacityIncreaseStep:        cfg.AnthropicAdaptiveSchedulerCapacityIncreaseStep,
		MinCapacity:                 cfg.AnthropicAdaptiveSchedulerMinCapacity,
		HardShrinkFailureMultiplier: cfg.AnthropicAdaptiveSchedulerHardShrinkFailureMultiplier,
	}
}

func anthropicAdaptiveTimePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
