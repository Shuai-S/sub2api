package service

import (
	"math"
	"math/rand/v2"
	"sort"
)

type AnthropicAdaptiveCandidate struct {
	Account           *Account
	LoadInfo          *AccountLoadInfo
	EffectiveCapacity int
	Score             float64
	ReliabilityScore  float64
	CapacityScore     float64
	LatencyScore      float64
	ExplorationScore  float64
	state             anthropicAdaptiveAccountState
}

type AnthropicAdaptiveDecision struct {
	Order             []AnthropicAdaptiveCandidate
	CandidateCount    int
	TopK              int
	SelectedAccountID int64
	FallbackReason    string
}

type AnthropicAdaptiveScheduleRequest struct {
	RequestedModel string
	Candidates     []accountWithLoad
	Settings       *AnthropicAdaptiveSchedulerSettings
}

func (s *anthropicAdaptiveScheduler) BuildOrder(req AnthropicAdaptiveScheduleRequest) AnthropicAdaptiveDecision {
	decision := AnthropicAdaptiveDecision{CandidateCount: len(req.Candidates)}
	if s == nil || s.state == nil || len(req.Candidates) == 0 {
		decision.FallbackReason = "no_candidates"
		return decision
	}
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	if req.Settings != nil {
		settings = NormalizeAnthropicAdaptiveSchedulerSettings(*req.Settings)
	}
	now := s.now()
	candidates := make([]AnthropicAdaptiveCandidate, 0, len(req.Candidates))
	for _, item := range req.Candidates {
		if item.account == nil {
			continue
		}
		load := item.loadInfo
		if load == nil {
			load = &AccountLoadInfo{AccountID: item.account.ID}
		}
		state := s.state.observeLoad(item.account, load, now, settings)
		candidates = append(candidates, AnthropicAdaptiveCandidate{
			Account:           item.account,
			LoadInfo:          load,
			EffectiveCapacity: s.state.effectiveCapacity(item.account, settings),
			state:             state,
		})
	}
	if len(candidates) == 0 {
		decision.FallbackReason = "no_candidates"
		return decision
	}
	applyAnthropicAdaptiveScores(candidates, req.RequestedModel, settings)
	decision.Order = buildAnthropicAdaptiveOrder(candidates, settings)
	decision.CandidateCount = len(candidates)
	decision.TopK = min(settings.AnthropicAdaptiveSchedulerTopK, len(candidates))
	if len(decision.Order) > 0 {
		decision.SelectedAccountID = decision.Order[0].Account.ID
	}
	return decision
}

func applyAnthropicAdaptiveScores(candidates []AnthropicAdaptiveCandidate, requestedModel string, settings AnthropicAdaptiveSchedulerSettings) {
	family := anthropicAdaptiveModelFamily(requestedModel)
	minLatency, maxLatency := math.Inf(1), math.Inf(-1)
	hasLatency := false
	latencies := make([]float64, len(candidates))
	for i := range candidates {
		latency := candidates[i].state.LatencyByModelFamily[family]
		value := latency.TTFTEMA
		if value <= 0 {
			value = latency.LatencyEMA
		}
		latencies[i] = value
		if value > 0 {
			hasLatency = true
			minLatency = math.Min(minLatency, value)
			maxLatency = math.Max(maxLatency, value)
		}
	}

	for i := range candidates {
		candidate := &candidates[i]
		candidate.ReliabilityScore = clamp01(candidate.state.SuccessEMA)
		if candidate.state.ConsecutiveFailure > 0 {
			candidate.ReliabilityScore /= 1 + settings.AnthropicAdaptiveSchedulerConsecutiveFailurePenalty*float64(candidate.state.ConsecutiveFailure)
		}
		if candidate.EffectiveCapacity <= 0 {
			candidate.CapacityScore = 1
		} else {
			remaining := candidate.EffectiveCapacity - candidate.LoadInfo.CurrentConcurrency
			candidate.CapacityScore = clamp01(float64(remaining) / float64(candidate.EffectiveCapacity))
		}
		candidate.LatencyScore = settings.AnthropicAdaptiveSchedulerNeutralLatencyScore
		if hasLatency && latencies[i] > 0 {
			candidate.LatencyScore = 1 - normalizeAdaptiveValue(latencies[i], minLatency, maxLatency, 1-settings.AnthropicAdaptiveSchedulerNeutralLatencyScore)
		}
		candidate.ExplorationScore = 1 / math.Sqrt(float64(candidate.state.TotalSamples+1))
		candidate.Score = settings.AnthropicAdaptiveSchedulerWeightReliability*candidate.ReliabilityScore +
			settings.AnthropicAdaptiveSchedulerWeightCapacity*candidate.CapacityScore +
			settings.AnthropicAdaptiveSchedulerWeightLatency*candidate.LatencyScore +
			settings.AnthropicAdaptiveSchedulerWeightExploration*candidate.ExplorationScore
	}
}

func buildAnthropicAdaptiveOrder(candidates []AnthropicAdaptiveCandidate, settings AnthropicAdaptiveSchedulerSettings) []AnthropicAdaptiveCandidate {
	priorities := make([]int, 0)
	byPriority := make(map[int][]AnthropicAdaptiveCandidate)
	for _, candidate := range candidates {
		priority := candidate.Account.Priority
		if _, ok := byPriority[priority]; !ok {
			priorities = append(priorities, priority)
		}
		byPriority[priority] = append(byPriority[priority], candidate)
	}
	sort.Ints(priorities)
	order := make([]AnthropicAdaptiveCandidate, 0, len(candidates))
	for _, priority := range priorities {
		ranked := byPriority[priority]
		sort.SliceStable(ranked, func(i, j int) bool {
			if ranked[i].Score != ranked[j].Score {
				return ranked[i].Score > ranked[j].Score
			}
			if ranked[i].LoadInfo.LoadRate != ranked[j].LoadInfo.LoadRate {
				return ranked[i].LoadInfo.LoadRate < ranked[j].LoadInfo.LoadRate
			}
			return ranked[i].Account.ID < ranked[j].Account.ID
		})
		topK := min(settings.AnthropicAdaptiveSchedulerTopK, len(ranked))
		order = appendAnthropicAdaptiveSoftmaxOrder(order, ranked[:topK], settings.AnthropicAdaptiveSchedulerSoftmaxTemperature)
		order = append(order, ranked[topK:]...)
	}
	return order
}

func appendAnthropicAdaptiveSoftmaxOrder(order, candidates []AnthropicAdaptiveCandidate, temperature float64) []AnthropicAdaptiveCandidate {
	pool := append([]AnthropicAdaptiveCandidate(nil), candidates...)
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
