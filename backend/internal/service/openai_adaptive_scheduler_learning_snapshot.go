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

	OpenAIAdaptiveLearningStatusDisabled         = "disabled"
	OpenAIAdaptiveLearningStatusUnavailable      = "unavailable"
	OpenAIAdaptiveLearningStatusCooldown         = "cooldown"
	OpenAIAdaptiveLearningStatusHalfOpen         = "half_open"
	OpenAIAdaptiveLearningStatusCircuitHalfOpen  = "circuit_half_open"
	OpenAIAdaptiveLearningStatusCapacityRecovery = "capacity_recovery"
	OpenAIAdaptiveLearningStatusHighError        = "high_error"
	OpenAIAdaptiveLearningStatusSaturated        = "saturated"
	OpenAIAdaptiveLearningStatusLearning         = "learning"
	OpenAIAdaptiveLearningStatusUnlearned        = "unlearned"
	OpenAIAdaptiveLearningStatusHealthy          = "healthy"
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

func (f *OpenAIAdaptiveSchedulerLearningFilter) IsTopNMode() bool {
	return f != nil && f.TopN > 0
}

type OpenAIAdaptiveSchedulerLearningSettingsSnapshot struct {
	DiagnosticLogEnabled      bool    `json:"diagnostic_log_enabled"`
	DiagnosticLogSampleRate   float64 `json:"diagnostic_log_sample_rate"`
	TopK                      int     `json:"top_k"`
	ExplorationRate           float64 `json:"exploration_rate"`
	SoftmaxTemperature        float64 `json:"softmax_temperature"`
	ConsecutiveFailurePenalty float64 `json:"consecutive_failure_penalty"`
	LearningWindowSeconds     int     `json:"learning_window_seconds"`
	LearningMinHealthSamples  int     `json:"learning_min_health_samples"`
	SuccessEMAAlpha           float64 `json:"success_ema_alpha"`
	TTFTEMAAlpha              float64 `json:"ttft_ema_alpha"`
	HealthFailureThreshold    int     `json:"health_failure_threshold"`
	CooldownSeconds           int     `json:"cooldown_seconds"`
	CooldownMaxSeconds        int     `json:"cooldown_max_seconds"`
	HighErrorMinSamples       int     `json:"high_error_min_samples"`
	HighErrorMaxSamples       int     `json:"high_error_max_samples"`
	HighErrorEnterRate        float64 `json:"high_error_enter_rate"`
	HighErrorExitRate         float64 `json:"high_error_exit_rate"`
	CapacityShrinkFactor      float64 `json:"capacity_shrink_factor"`
	CapacityGrowthFactor      float64 `json:"capacity_growth_factor"`
	CapacityRecoverySamples   int     `json:"capacity_recovery_samples"`
	CapacityRecoveryLoad      float64 `json:"capacity_recovery_load"`
	QuotaProbeIntervalSeconds int     `json:"quota_probe_interval_seconds"`
	WeightReliability         float64 `json:"weight_reliability"`
	WeightCapacity            float64 `json:"weight_capacity"`
	WeightTTFT                float64 `json:"weight_ttft"`
	WeightCost                float64 `json:"weight_cost"`
}

type OpenAIAdaptiveSchedulerLearningSummary struct {
	TrackedAccounts          int `json:"tracked_accounts"`
	UnlearnedAccounts        int `json:"unlearned_accounts"`
	LearningAccounts         int `json:"learning_accounts"`
	HealthyAccounts          int `json:"healthy_accounts"`
	HighErrorAccounts        int `json:"high_error_accounts"`
	CooldownAccounts         int `json:"cooldown_accounts"`
	HalfOpenAccounts         int `json:"half_open_accounts"`
	CircuitHalfOpenAccounts  int `json:"circuit_half_open_accounts"`
	CapacityRecoveryAccounts int `json:"capacity_recovery_accounts"`
	SaturatedAccounts        int `json:"saturated_accounts"`
	UnavailableAccounts      int `json:"unavailable_accounts"`
	LearnedAccounts          int `json:"learned_accounts"`
	NotApplicableAccounts    int `json:"not_applicable_accounts"`
	QuotaLimitedAccounts     int `json:"quota_limited_accounts"`
}

type OpenAIAdaptiveSchedulerAccountLearningSnapshot struct {
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
	CircuitHalfOpen    bool     `json:"circuit_half_open"`
	CapacityRecovery   bool     `json:"capacity_recovery"`

	SchedulerScore float64 `json:"scheduler_score"`
	SuccessScore   float64 `json:"success_score"`
	CostScore      float64 `json:"cost_score"`
	CapacityScore  float64 `json:"capacity_score"`
	LatencyScore   float64 `json:"latency_score"`

	SuccessEMA  float64 `json:"success_ema"`
	TTFTEMA     float64 `json:"ttft_ema"`
	TTFTSamples int64   `json:"ttft_samples"`

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
	var stateStore *adaptiveStateStore
	if s != nil && s.openAIGatewayService != nil {
		cfg = s.openAIGatewayService.openAIAdaptiveSchedulerSettings(ctx)
		stateStore = s.openAIGatewayService.openAIAdaptiveSchedulerCoreStateStoreForSnapshot()
	}
	realtimeEnabled := s.IsRealtimeMonitoringEnabled(ctx)

	accounts, err := s.listAllAccountsForOps(ctx, PlatformOpenAI, filter.GroupID)
	if err != nil {
		return nil, err
	}
	accounts = filterOpenAIAdaptiveLearningAccountsByGroup(accounts, filter.GroupID)
	accounts = filterOpenAIAdaptiveLearningSchedulableAccounts(accounts)

	now := time.Now()
	coreStates := make(map[int64]adaptiveAccountState, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		coreState := newAdaptiveAccountState(acc.ID, acc.Concurrency, now)
		if stateStore != nil {
			snapshot := stateStore.snapshot(acc.ID, acc.Concurrency, now, openAIAdaptiveCoreSettings(cfg))
			coreState = &snapshot
		}
		coreStates[acc.ID] = *coreState
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: coreState.EffectiveCapacity,
		})
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if realtimeEnabled {
		loadMap = s.getOpenAIAdaptiveLearningLoadMapBestEffort(ctx, loadReq)
	}

	rows := make([]OpenAIAdaptiveSchedulerAccountLearningSnapshot, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		loadInfo := loadMap[acc.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: acc.ID}
		}
		row := buildOpenAIAdaptiveCoreLearningAccountSnapshot(acc, coreStates[acc.ID], loadInfo, now, openAIAdaptiveCoreSettings(cfg))
		rows = append(rows, row)
	}
	applyOpenAIAdaptiveCoreScores(rows, accounts, coreStates, loadMap, now, openAIAdaptiveCoreSettings(cfg))
	rows = filterOpenAIAdaptiveLearningRowsByDualStatus(rows, filter.LearningStatus, filter.RuntimeStatus)
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
	filter.LearningStatus = strings.ToLower(strings.TrimSpace(filter.LearningStatus))
	filter.RuntimeStatus = strings.ToLower(strings.TrimSpace(filter.RuntimeStatus))
}

func buildOpenAIAdaptiveCoreLearningAccountSnapshot(account *Account, state adaptiveAccountState, load *AccountLoadInfo, now time.Time, settings adaptiveCoreSettings) OpenAIAdaptiveSchedulerAccountLearningSnapshot {
	if account == nil {
		return OpenAIAdaptiveSchedulerAccountLearningSnapshot{}
	}
	if load == nil {
		load = &AccountLoadInfo{AccountID: account.ID}
	}
	learning, samples := adaptiveLearningState(state, account.IsOAuth(), now, settings)
	accountCallable := account.IsSchedulableAt(now)
	runtimeState := state
	accountRateLimited := account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt)
	if accountRateLimited {
		// Account-level 429 state is a durable scheduling hard gate and may have
		// been written by a different instance (or before adaptive state restore).
		// Merge it into this read-only view so the panel never calls the account
		// healthy while the real scheduler excludes it.
		runtimeState.QuotaLimited = true
		runtimeState.QuotaResetAt = *account.RateLimitResetAt
		runtimeState.QuotaNextProbeAt = *account.RateLimitResetAt
	}
	runtimeStatus, flags, reasonCode, reason := adaptiveRuntimeState(runtimeState, accountCallable, load.CurrentConcurrency, now)
	if accountRateLimited {
		reasonCode = "account_rate_limited"
		reason = ""
	}
	row := OpenAIAdaptiveSchedulerAccountLearningSnapshot{
		AccountID:                 account.ID,
		AccountName:               account.Name,
		Platform:                  account.Platform,
		Type:                      account.Type,
		AccountStatus:             account.Status,
		Schedulable:               accountCallable,
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
		CircuitHalfOpen:           containsAdaptiveRuntimeFlag(flags, adaptiveRuntimeCircuitHalfOpen),
		CapacityRecovery:          containsAdaptiveRuntimeFlag(flags, adaptiveRuntimeCapacityRecovery),
		SuccessEMA:                state.SuccessEMA,
		TTFTEMA:                   state.TTFTEMA,
		TTFTSamples:               state.TTFTSamples,
		TotalSamples:              int64(samples),
		ConsecutiveFailure:        state.ConsecutiveFailures,
		LastSuccessAt:             timePtrIfNotZero(state.LastSuccessAt),
		LastFailureAt:             timePtrIfNotZero(state.LastFailureAt),
		CooldownUntil:             timePtrIfNotZero(state.CircuitOpenUntil),
		CircuitOpenCount:          state.CircuitOpenCount,
		CapacityCooldownUntil:     timePtrIfNotZero(state.CapacityCooldownUntil),
		CapacityRecoverySuccesses: state.CapacityRecoverySuccesses,
		QuotaLimited:              runtimeState.QuotaLimited,
		QuotaResetAt:              timePtrIfNotZero(runtimeState.QuotaResetAt),
		QuotaNextProbeAt:          timePtrIfNotZero(runtimeState.QuotaNextProbeAt),
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

func applyOpenAIAdaptiveCoreScores(rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot, accounts []Account, states map[int64]adaptiveAccountState, loads map[int64]*AccountLoadInfo, now time.Time, settings adaptiveCoreSettings) {
	inputs := make([]adaptiveScoreCandidate, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		load := loads[account.ID]
		if load == nil {
			load = &AccountLoadInfo{AccountID: account.ID}
		}
		inputs = append(inputs, adaptiveScoreCandidate{AccountID: account.ID, OAuth: account.IsOAuth(), Cost: account.BillingRateMultiplier(), CurrentConcurrency: load.CurrentConcurrency, State: states[account.ID]})
	}
	scores := scoreAdaptiveCandidates(inputs, now, settings)
	byID := make(map[int64]adaptiveScoreCandidate, len(scores))
	for _, score := range scores {
		byID[score.AccountID] = score
	}
	for i := range rows {
		score := byID[rows[i].AccountID]
		rows[i].SchedulerScore = score.Score
		rows[i].SuccessScore = score.ReliabilityScore
		rows[i].CostScore = score.CostScore
		rows[i].CapacityScore = score.CapacityScore
		rows[i].LatencyScore = score.TTFTScore
	}
}

func filterOpenAIAdaptiveLearningRowsByDualStatus(rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot, learningStatus, runtimeStatus string) []OpenAIAdaptiveSchedulerAccountLearningSnapshot {
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

func (s *OpenAIGatewayService) openAIAdaptiveSchedulerCoreStateStoreForSnapshot() *adaptiveStateStore {
	if s == nil {
		return nil
	}
	if s.openaiAdaptiveCore != nil {
		return s.openaiAdaptiveCore
	}
	s.openaiSchedulerMu.Lock()
	defer s.openaiSchedulerMu.Unlock()
	scheduler, _ := s.openaiScheduler.(*adaptiveOpenAIAccountScheduler)
	if scheduler == nil {
		return nil
	}
	return scheduler.core
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
	case "account", "status", "capacity", "load", "score", "samples", "error", "latency", "last_event":
		return strings.TrimSpace(value)
	case "default", "":
		return ""
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
		if cmp := compareFloat64(1-left.SuccessEMA, 1-right.SuccessEMA); cmp != 0 {
			return cmp
		}
	case "latency":
		if cmp := compareFloat64(left.TTFTEMA, right.TTFTEMA); cmp != 0 {
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
		if left.SuccessEMA != right.SuccessEMA {
			return compareFloat64(left.SuccessEMA, right.SuccessEMA)
		}
		if left.SchedulerScore != right.SchedulerScore {
			return compareFloat64(left.SchedulerScore, right.SchedulerScore)
		}
	}
	return compareInt64(left.AccountID, right.AccountID)
}

func openAIAdaptiveLearningLastEventTime(row OpenAIAdaptiveSchedulerAccountLearningSnapshot) time.Time {
	var latest time.Time
	for _, candidate := range []*time.Time{
		row.LastSuccessAt,
		row.LastFailureAt,
		row.CooldownUntil,
		row.CapacityCooldownUntil,
		row.QuotaResetAt,
		row.QuotaNextProbeAt,
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
	case string(adaptiveRuntimeQuotaLimited):
		return 1
	case OpenAIAdaptiveLearningStatusHalfOpen:
		return 2
	case OpenAIAdaptiveLearningStatusCircuitHalfOpen:
		return 2
	case OpenAIAdaptiveLearningStatusCapacityRecovery:
		return 3
	case OpenAIAdaptiveLearningStatusHighError:
		return 4
	case OpenAIAdaptiveLearningStatusSaturated:
		return 5
	case OpenAIAdaptiveLearningStatusUnavailable:
		return 6
	case OpenAIAdaptiveLearningStatusLearning:
		return 7
	case OpenAIAdaptiveLearningStatusUnlearned:
		return 8
	case OpenAIAdaptiveLearningStatusDisabled:
		return 9
	default:
		return 9
	}
}

func summarizeOpenAIAdaptiveLearningRows(rows []OpenAIAdaptiveSchedulerAccountLearningSnapshot) OpenAIAdaptiveSchedulerLearningSummary {
	var summary OpenAIAdaptiveSchedulerLearningSummary
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
		case OpenAIAdaptiveLearningStatusUnavailable:
			summary.UnavailableAccounts++
		case OpenAIAdaptiveLearningStatusCooldown:
			summary.CooldownAccounts++
		case OpenAIAdaptiveLearningStatusHalfOpen:
			summary.HalfOpenAccounts++
		case OpenAIAdaptiveLearningStatusCircuitHalfOpen:
			summary.CircuitHalfOpenAccounts++
			summary.HalfOpenAccounts++
		case OpenAIAdaptiveLearningStatusCapacityRecovery:
			summary.CapacityRecoveryAccounts++
			summary.HalfOpenAccounts++
		case string(adaptiveRuntimeQuotaLimited):
			summary.QuotaLimitedAccounts++
		case OpenAIAdaptiveLearningStatusHighError:
			summary.HighErrorAccounts++
		case OpenAIAdaptiveLearningStatusSaturated:
			summary.SaturatedAccounts++
		case OpenAIAdaptiveLearningStatusHealthy:
			summary.HealthyAccounts++
		}
		if row.CircuitHalfOpen && row.RuntimeStatus != OpenAIAdaptiveLearningStatusCircuitHalfOpen {
			summary.CircuitHalfOpenAccounts++
			summary.HalfOpenAccounts++
		}
		if row.CapacityRecovery && row.RuntimeStatus != OpenAIAdaptiveLearningStatusCapacityRecovery {
			summary.CapacityRecoveryAccounts++
			summary.HalfOpenAccounts++
		}
	}
	return summary
}

func openAIAdaptiveLearningSettingsSnapshot(cfg OpenAIAdaptiveSchedulerSettings) OpenAIAdaptiveSchedulerLearningSettingsSnapshot {
	return OpenAIAdaptiveSchedulerLearningSettingsSnapshot{
		DiagnosticLogEnabled:      cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled,
		DiagnosticLogSampleRate:   cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate,
		TopK:                      cfg.OpenAIAdaptiveSchedulerTopK,
		ExplorationRate:           cfg.OpenAIAdaptiveSchedulerExplorationRate,
		SoftmaxTemperature:        cfg.OpenAIAdaptiveSchedulerSoftmaxTemperature,
		ConsecutiveFailurePenalty: cfg.OpenAIAdaptiveSchedulerConsecutiveFailurePenalty,
		LearningWindowSeconds:     cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds,
		LearningMinHealthSamples:  cfg.OpenAIAdaptiveSchedulerLearningMinHealthSamples,
		SuccessEMAAlpha:           cfg.OpenAIAdaptiveSchedulerSuccessEMAAlpha,
		TTFTEMAAlpha:              cfg.OpenAIAdaptiveSchedulerTTFTEMAAlpha,
		HealthFailureThreshold:    cfg.OpenAIAdaptiveSchedulerHealthFailureThreshold,
		CooldownSeconds:           cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds,
		CooldownMaxSeconds:        cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds,
		HighErrorMinSamples:       cfg.OpenAIAdaptiveSchedulerHighErrorMinSamples,
		HighErrorMaxSamples:       cfg.OpenAIAdaptiveSchedulerHighErrorMaxSamples,
		HighErrorEnterRate:        cfg.OpenAIAdaptiveSchedulerHighErrorEnterRate,
		HighErrorExitRate:         cfg.OpenAIAdaptiveSchedulerHighErrorExitRate,
		CapacityShrinkFactor:      cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft,
		CapacityGrowthFactor:      cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor,
		CapacityRecoverySamples:   cfg.OpenAIAdaptiveSchedulerCapacityRecoverySamples,
		CapacityRecoveryLoad:      cfg.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold,
		QuotaProbeIntervalSeconds: cfg.OpenAIAdaptiveSchedulerQuotaProbeIntervalSeconds,
		WeightReliability:         cfg.OpenAIAdaptiveSchedulerWeightSuccess,
		WeightCapacity:            cfg.OpenAIAdaptiveSchedulerWeightCapacity,
		WeightTTFT:                cfg.OpenAIAdaptiveSchedulerWeightLatency,
		WeightCost:                cfg.OpenAIAdaptiveSchedulerWeightCost,
	}
}

func timePtrIfNotZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
