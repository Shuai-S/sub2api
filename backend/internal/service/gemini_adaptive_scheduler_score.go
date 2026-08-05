package service

import (
	"context"
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

type GeminiAdaptiveQuotaBucket string

const (
	GeminiQuotaBucketShared    GeminiAdaptiveQuotaBucket = "shared"
	GeminiQuotaBucketPro       GeminiAdaptiveQuotaBucket = "pro"
	GeminiQuotaBucketFlash     GeminiAdaptiveQuotaBucket = "flash"
	GeminiQuotaBucketUnlimited GeminiAdaptiveQuotaBucket = "unlimited"
	GeminiQuotaBucketUnknown   GeminiAdaptiveQuotaBucket = "unknown"
)

type GeminiAdaptiveQuotaScope struct {
	Daily  GeminiAdaptiveQuotaBucket `json:"daily"`
	Minute GeminiAdaptiveQuotaBucket `json:"minute"`
}

type GeminiAdaptiveQuotaSnapshot struct {
	Scope         GeminiAdaptiveQuotaScope `json:"scope"`
	DailyUsed     int64                    `json:"daily_used"`
	DailyLimit    int64                    `json:"daily_limit"`
	DailyResetAt  time.Time                `json:"daily_reset_at"`
	MinuteUsed    int64                    `json:"minute_used"`
	MinuteLimit   int64                    `json:"minute_limit"`
	MinuteResetAt time.Time                `json:"minute_reset_at"`
	HardRejected  bool                     `json:"hard_rejected"`
	DataAvailable bool                     `json:"data_available"`
}

type GeminiAdaptiveCandidateInput struct {
	Account *Account
	Load    *AccountLoadInfo
	Quota   GeminiAdaptiveQuotaSnapshot
}

type GeminiAdaptiveCandidate struct {
	Account           *Account                    `json:"-"`
	Load              *AccountLoadInfo            `json:"-"`
	Quota             GeminiAdaptiveQuotaSnapshot `json:"quota"`
	EffectiveCapacity int                         `json:"effective_capacity"`
	Score             float64                     `json:"score"`
	ReliabilityScore  float64                     `json:"reliability_score"`
	QuotaScore        float64                     `json:"quota_score"`
	CapacityScore     float64                     `json:"capacity_score"`
	LatencyScore      float64                     `json:"latency_score"`
	CostScore         float64                     `json:"cost_score"`
	ExplorationScore  float64                     `json:"exploration_score"`
	CircuitStatus     string                      `json:"circuit_status"`
	CircuitScope      string                      `json:"circuit_scope,omitempty"`
	CircuitOpenUntil  time.Time                   `json:"circuit_open_until,omitempty"`
	state             geminiAdaptiveAccountState
}

type GeminiAdaptiveScheduleRequest struct {
	RequestedModel string
	Stream         bool
	Action         string
	Candidates     []GeminiAdaptiveCandidateInput
	BaselineOrder  []int64
	Settings       *GeminiAdaptiveSchedulerSettings
	ctx            context.Context
}

type GeminiAdaptiveDecision struct {
	Order                       []GeminiAdaptiveCandidate
	SelectedAccountID           int64
	BaselineAccountID           int64
	InputCandidateCount         int
	CandidateCount              int
	HardRejectedCount           int
	CircuitRejectedCount        int
	AccountCircuitRejectedCount int
	ModelCircuitRejectedCount   int
	HalfOpenCandidateCount      int
	TopK                        int
	BuildLatencyMs              int64
	FallbackReason              string
}

func (s *geminiAdaptiveScheduler) BuildOrder(req GeminiAdaptiveScheduleRequest) (GeminiAdaptiveDecision, error) {
	decision := GeminiAdaptiveDecision{InputCandidateCount: len(req.Candidates)}
	if s == nil || s.state == nil || len(req.Candidates) == 0 {
		decision.FallbackReason = "no_candidates"
		return decision, nil
	}
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	if req.Settings != nil {
		settings = NormalizeGeminiAdaptiveSchedulerSettings(*req.Settings)
	}
	now := s.now()
	allByID := make(map[int64]GeminiAdaptiveCandidate, len(req.Candidates))
	geminiCandidates := make([]GeminiAdaptiveCandidate, 0, len(req.Candidates))
	for _, input := range req.Candidates {
		if input.Account == nil {
			continue
		}
		if input.Quota.HardRejected {
			decision.HardRejectedCount++
			continue
		}
		load := input.Load
		if load == nil {
			load = &AccountLoadInfo{AccountID: input.Account.ID}
		}
		candidate := GeminiAdaptiveCandidate{
			Account:           input.Account,
			Load:              load,
			Quota:             input.Quota,
			EffectiveCapacity: input.Account.Concurrency,
		}
		if input.Account.Platform == PlatformGemini {
			candidate.state = s.state.observeLoad(req.ctx, input.Account, load, now, settings)
			candidate.EffectiveCapacity = s.state.effectiveCapacity(input.Account, settings)
			eligibility := s.state.circuitEligibility(input.Account, req.RequestedModel, req.Action, now, settings)
			candidate.CircuitStatus = eligibility.Status
			candidate.CircuitScope = eligibility.Scope
			candidate.CircuitOpenUntil = eligibility.OpenUntil
			if !eligibility.Allowed {
				decision.CircuitRejectedCount++
				if eligibility.Scope == geminiAdaptiveCircuitScopeAccount {
					decision.AccountCircuitRejectedCount++
				} else {
					decision.ModelCircuitRejectedCount++
				}
				continue
			}
			if eligibility.HalfOpen {
				decision.HalfOpenCandidateCount++
			}
			geminiCandidates = append(geminiCandidates, candidate)
		}
		allByID[input.Account.ID] = candidate
	}
	for _, accountID := range req.BaselineOrder {
		if _, ok := allByID[accountID]; ok {
			decision.BaselineAccountID = accountID
			break
		}
	}
	decision.CandidateCount = len(geminiCandidates)
	if len(geminiCandidates) == 0 {
		decision.FallbackReason = "no_native_gemini_candidates"
		decision.Order = candidatesInBaselineOrder(req, allByID)
		if len(decision.Order) > 0 {
			if decision.CircuitRejectedCount > 0 {
				decision.FallbackReason = "native_gemini_circuits_open_using_baseline"
			}
			decision.SelectedAccountID = decision.Order[0].Account.ID
		} else if decision.CircuitRejectedCount > 0 {
			decision.FallbackReason = "all_native_gemini_circuits_open"
		}
		return decision, nil
	}

	applyGeminiAdaptiveScores(geminiCandidates, req.RequestedModel, req.Action, req.Stream, now, settings)
	adaptiveGemini := buildGeminiAdaptiveOrder(geminiCandidates, settings)
	decision.TopK = min(settings.GeminiAdaptiveSchedulerTopK, len(geminiCandidates))
	for _, candidate := range adaptiveGemini {
		allByID[candidate.Account.ID] = candidate
	}
	decision.Order = mergeGeminiAdaptiveWithBaseline(req, allByID, adaptiveGemini)
	if len(decision.Order) > 0 {
		decision.SelectedAccountID = decision.Order[0].Account.ID
	}
	return decision, nil
}

func candidatesInBaselineOrder(req GeminiAdaptiveScheduleRequest, byID map[int64]GeminiAdaptiveCandidate) []GeminiAdaptiveCandidate {
	order := make([]GeminiAdaptiveCandidate, 0, len(byID))
	seen := make(map[int64]struct{}, len(byID))
	for _, id := range req.BaselineOrder {
		if candidate, ok := byID[id]; ok {
			order = append(order, candidate)
			seen[id] = struct{}{}
		}
	}
	for _, input := range req.Candidates {
		if input.Account == nil {
			continue
		}
		if _, ok := seen[input.Account.ID]; ok {
			continue
		}
		if candidate, ok := byID[input.Account.ID]; ok {
			order = append(order, candidate)
			seen[input.Account.ID] = struct{}{}
		}
	}
	return order
}

func mergeGeminiAdaptiveWithBaseline(req GeminiAdaptiveScheduleRequest, byID map[int64]GeminiAdaptiveCandidate, adaptive []GeminiAdaptiveCandidate) []GeminiAdaptiveCandidate {
	baseline := candidatesInBaselineOrder(req, byID)
	byPriority := make(map[int][]GeminiAdaptiveCandidate)
	for _, candidate := range adaptive {
		byPriority[candidate.Account.Priority] = append(byPriority[candidate.Account.Priority], candidate)
	}
	priorityIndex := make(map[int]int, len(byPriority))
	merged := make([]GeminiAdaptiveCandidate, 0, len(baseline))
	seen := make(map[int64]struct{}, len(baseline))
	for _, candidate := range baseline {
		if candidate.Account.Platform == PlatformGemini {
			priority := candidate.Account.Priority
			index := priorityIndex[priority]
			queue := byPriority[priority]
			if index < len(queue) {
				candidate = queue[index]
				priorityIndex[priority] = index + 1
			}
		}
		merged = append(merged, candidate)
		seen[candidate.Account.ID] = struct{}{}
	}
	for _, candidate := range adaptive {
		if _, ok := seen[candidate.Account.ID]; !ok {
			merged = append(merged, candidate)
		}
	}
	return merged
}

func applyGeminiAdaptiveScores(candidates []GeminiAdaptiveCandidate, requestedModel, action string, stream bool, now time.Time, settings GeminiAdaptiveSchedulerSettings) {
	family := geminiAdaptiveModelFamily(requestedModel, action)
	latencies := make([]float64, len(candidates))
	costValues := make([]float64, len(candidates))
	minLatency, maxLatency := math.Inf(1), math.Inf(-1)
	minCost, maxCost := math.Inf(1), math.Inf(-1)
	hasLatency := false
	for i := range candidates {
		modelState := candidates[i].state.ByModelFamily[family]
		latency := modelState.LatencyEMA
		if stream && modelState.TTFTEMA > 0 {
			latency = modelState.TTFTEMA
		}
		latencies[i] = latency
		if latency > 0 {
			hasLatency = true
			minLatency = math.Min(minLatency, latency)
			maxLatency = math.Max(maxLatency, latency)
		}
		multiplier := math.Max(candidates[i].Account.BillingRateMultiplier(), settings.GeminiAdaptiveSchedulerMinCostMultiplier)
		costValues[i] = 1 / multiplier
		minCost = math.Min(minCost, costValues[i])
		maxCost = math.Max(maxCost, costValues[i])
	}

	for i := range candidates {
		candidate := &candidates[i]
		pathScore := clamp01(candidate.state.PathSuccessEMA)
		if candidate.state.ConsecutiveFailure > 0 {
			pathScore /= 1 + settings.GeminiAdaptiveSchedulerConsecutiveFailurePenalty*float64(candidate.state.ConsecutiveFailure)
		}
		modelScore := settings.GeminiAdaptiveSchedulerInitialReliability
		if modelState := candidate.state.ByModelFamily[family]; modelState.Samples > 0 {
			modelScore = clamp01(modelState.SuccessEMA)
		}
		candidate.ReliabilityScore = 0.35*pathScore + 0.65*modelScore
		quotaForScore := candidate.Quota
		if quotaForScore.DataAvailable && quotaForScore.MinuteLimit > 0 && candidate.Load.CurrentConcurrency > 0 {
			quotaForScore.MinuteUsed += int64(candidate.Load.CurrentConcurrency)
		}
		candidate.QuotaScore = geminiAdaptiveQuotaScore(quotaForScore, now, settings.GeminiAdaptiveSchedulerNeutralQuotaScore)
		if candidate.EffectiveCapacity <= 0 {
			candidate.CapacityScore = 1
		} else {
			remaining := candidate.EffectiveCapacity - candidate.Load.CurrentConcurrency
			candidate.CapacityScore = clamp01(float64(remaining) / float64(candidate.EffectiveCapacity))
		}
		candidate.LatencyScore = settings.GeminiAdaptiveSchedulerNeutralLatencyScore
		if hasLatency && latencies[i] > 0 {
			candidate.LatencyScore = 1 - normalizeAdaptiveValue(latencies[i], minLatency, maxLatency, 1-settings.GeminiAdaptiveSchedulerNeutralLatencyScore)
		}
		candidate.CostScore = normalizeAdaptiveValue(costValues[i], minCost, maxCost, 1)
		candidate.ExplorationScore = 1 / math.Sqrt(float64(candidate.state.TotalSamples+1))
		candidate.Score = settings.GeminiAdaptiveSchedulerWeightReliability*candidate.ReliabilityScore +
			settings.GeminiAdaptiveSchedulerWeightQuota*candidate.QuotaScore +
			settings.GeminiAdaptiveSchedulerWeightCapacity*candidate.CapacityScore +
			settings.GeminiAdaptiveSchedulerWeightLatency*candidate.LatencyScore +
			settings.GeminiAdaptiveSchedulerWeightCost*candidate.CostScore +
			settings.GeminiAdaptiveSchedulerWeightExploration*candidate.ExplorationScore
	}
}

func geminiAdaptiveQuotaScore(snapshot GeminiAdaptiveQuotaSnapshot, now time.Time, neutral float64) float64 {
	if snapshot.HardRejected {
		return 0
	}
	if !snapshot.DataAvailable {
		return neutral
	}
	daily := geminiAdaptiveQuotaDimensionScore(snapshot.DailyUsed, snapshot.DailyLimit, snapshot.DailyResetAt, now, 24*time.Hour, neutral, true)
	minute := geminiAdaptiveQuotaDimensionScore(snapshot.MinuteUsed, snapshot.MinuteLimit, snapshot.MinuteResetAt, now, time.Minute, neutral, false)
	return math.Min(daily, minute)
}

func geminiAdaptiveQuotaDimensionScore(used, limit int64, resetAt, now time.Time, window time.Duration, neutral float64, pacing bool) float64 {
	if limit < 0 {
		return 1
	}
	if limit == 0 {
		return neutral
	}
	remaining := clamp01(1 - float64(used)/float64(limit))
	if !pacing || resetAt.IsZero() {
		return remaining
	}
	timeRemaining := clamp01(resetAt.Sub(now).Seconds() / window.Seconds())
	if timeRemaining < 0.05 {
		timeRemaining = 0.05
	}
	pacingScore := clamp01(remaining / timeRemaining)
	return 0.5*remaining + 0.5*pacingScore
}

func buildGeminiAdaptiveOrder(candidates []GeminiAdaptiveCandidate, settings GeminiAdaptiveSchedulerSettings) []GeminiAdaptiveCandidate {
	priorities := make([]int, 0)
	byPriority := make(map[int][]GeminiAdaptiveCandidate)
	for _, candidate := range candidates {
		priority := candidate.Account.Priority
		if _, ok := byPriority[priority]; !ok {
			priorities = append(priorities, priority)
		}
		byPriority[priority] = append(byPriority[priority], candidate)
	}
	sort.Ints(priorities)
	order := make([]GeminiAdaptiveCandidate, 0, len(candidates))
	for _, priority := range priorities {
		ranked := byPriority[priority]
		sort.SliceStable(ranked, func(i, j int) bool {
			if ranked[i].Score != ranked[j].Score {
				return ranked[i].Score > ranked[j].Score
			}
			if ranked[i].Load.LoadRate != ranked[j].Load.LoadRate {
				return ranked[i].Load.LoadRate < ranked[j].Load.LoadRate
			}
			if ranked[i].Account.LastUsedAt == nil || ranked[j].Account.LastUsedAt == nil {
				if ranked[i].Account.LastUsedAt == nil && ranked[j].Account.LastUsedAt != nil {
					return true
				}
				if ranked[i].Account.LastUsedAt != nil && ranked[j].Account.LastUsedAt == nil {
					return false
				}
			}
			return ranked[i].Account.ID < ranked[j].Account.ID
		})
		topK := min(settings.GeminiAdaptiveSchedulerTopK, len(ranked))
		order = appendGeminiAdaptiveSoftmaxOrder(order, ranked[:topK], settings.GeminiAdaptiveSchedulerSoftmaxTemperature)
		order = append(order, ranked[topK:]...)
	}
	return order
}

func appendGeminiAdaptiveSoftmaxOrder(order, candidates []GeminiAdaptiveCandidate, temperature float64) []GeminiAdaptiveCandidate {
	pool := append([]GeminiAdaptiveCandidate(nil), candidates...)
	for len(pool) > 0 {
		maxScore := pool[0].Score
		for _, candidate := range pool[1:] {
			maxScore = math.Max(maxScore, candidate.Score)
		}
		weights := make([]float64, len(pool))
		total := 0.0
		for i, candidate := range pool {
			weight := math.Exp((candidate.Score - maxScore) / temperature)
			if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
				weight = 1
			}
			weights[i] = weight
			total += weight
		}
		selected := 0
		if total > 0 {
			pick := rand.Float64() * total
			accumulated := 0.0
			for i, weight := range weights {
				accumulated += weight
				if pick <= accumulated {
					selected = i
					break
				}
			}
		}
		order = append(order, pool[selected])
		pool = append(pool[:selected], pool[selected+1:]...)
	}
	return order
}
