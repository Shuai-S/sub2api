package service

import (
	"math"
	"math/rand/v2"
	"time"
)

type AnthropicAdaptiveCandidate struct {
	Account           *Account
	LoadInfo          *AccountLoadInfo
	EffectiveCapacity int
	Score             float64
	ReliabilityScore  float64
	CapacityScore     float64
	LatencyScore      float64
	CostScore         float64
	ExplorationScore  float64
	coreState         adaptiveAccountState
}

type AnthropicAdaptiveDecision struct {
	Order             []AnthropicAdaptiveCandidate
	CandidateCount    int
	TopK              int
	SelectedAccountID int64
	FallbackReason    string
	BuildLatencyMs    int64
}

type AnthropicAdaptiveScheduleRequest struct {
	RequestedModel string
	Candidates     []accountWithLoad
	Settings       *AnthropicAdaptiveSchedulerSettings
	NewSession     bool
}

func (s *anthropicAdaptiveScheduler) BuildOrder(req AnthropicAdaptiveScheduleRequest) (decision AnthropicAdaptiveDecision) {
	startedAt := time.Now()
	decision = AnthropicAdaptiveDecision{CandidateCount: len(req.Candidates)}
	defer func() {
		decision.BuildLatencyMs = time.Since(startedAt).Milliseconds()
	}()
	if s == nil || s.core == nil || len(req.Candidates) == 0 {
		decision.FallbackReason = "no_candidates"
		return decision
	}
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	if req.Settings != nil {
		settings = NormalizeAnthropicAdaptiveSchedulerSettings(*req.Settings)
	}
	now := s.now()
	coreSettings := anthropicAdaptiveCoreSettings(settings)
	candidates := make([]AnthropicAdaptiveCandidate, 0, len(req.Candidates))
	for _, item := range req.Candidates {
		if item.account == nil {
			continue
		}
		load := item.loadInfo
		if load == nil {
			load = &AccountLoadInfo{AccountID: item.account.ID}
		}
		state := s.core.observeLoad(item.account.ID, item.account.Concurrency, load.CurrentConcurrency, now, coreSettings)
		if !s.core.allowedForSelection(item.account.ID, item.account.Concurrency, now, coreSettings) {
			continue
		}
		if state.EffectiveCapacity > 0 && load.CurrentConcurrency >= state.EffectiveCapacity {
			continue
		}
		candidates = append(candidates, AnthropicAdaptiveCandidate{
			Account:           item.account,
			LoadInfo:          load,
			EffectiveCapacity: state.EffectiveCapacity,
			coreState:         state,
		})
	}
	if len(candidates) == 0 {
		decision.FallbackReason = "all_circuits_open"
		return decision
	}
	applyAnthropicAdaptiveScores(candidates, req.RequestedModel, now, settings)
	decision.Order = buildAnthropicAdaptiveOrder(candidates, settings, req.NewSession)
	decision.CandidateCount = len(candidates)
	decision.TopK = min(settings.AnthropicAdaptiveSchedulerTopK, len(candidates))
	if len(decision.Order) > 0 {
		decision.SelectedAccountID = decision.Order[0].Account.ID
	}
	return decision
}

func applyAnthropicAdaptiveScores(candidates []AnthropicAdaptiveCandidate, requestedModel string, now time.Time, settings AnthropicAdaptiveSchedulerSettings) {
	_ = requestedModel
	inputs := make([]adaptiveScoreCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		inputs = append(inputs, adaptiveScoreCandidate{
			AccountID:          candidate.Account.ID,
			OAuth:              candidate.Account.IsOAuth(),
			Cost:               candidate.Account.BillingRateMultiplier(),
			CurrentConcurrency: candidate.LoadInfo.CurrentConcurrency,
			State:              candidate.coreState,
		})
	}
	scored := scoreAdaptiveCandidates(inputs, now, anthropicAdaptiveCoreSettings(settings))
	byID := make(map[int64]adaptiveScoreCandidate, len(scored))
	for _, candidate := range scored {
		byID[candidate.AccountID] = candidate
	}
	for i := range candidates {
		score := byID[candidates[i].Account.ID]
		candidates[i].Score = score.Score
		candidates[i].ReliabilityScore = score.ReliabilityScore
		candidates[i].CapacityScore = score.CapacityScore
		candidates[i].LatencyScore = score.TTFTScore
		candidates[i].CostScore = score.CostScore
		if score.HealthSamples < anthropicAdaptiveCoreSettings(settings).LearningMinHealthSamples {
			candidates[i].ExplorationScore = float64(anthropicAdaptiveCoreSettings(settings).LearningMinHealthSamples - score.HealthSamples)
		}
	}
}

func buildAnthropicAdaptiveOrder(candidates []AnthropicAdaptiveCandidate, settings AnthropicAdaptiveSchedulerSettings, newSessionValue ...bool) []AnthropicAdaptiveCandidate {
	newSession := false
	if len(newSessionValue) > 0 {
		newSession = newSessionValue[0]
	}
	inputs := make([]adaptiveScoreCandidate, 0, len(candidates))
	byID := make(map[int64]AnthropicAdaptiveCandidate, len(candidates))
	for _, candidate := range candidates {
		inputs = append(inputs, adaptiveScoreCandidate{AccountID: candidate.Account.ID, OAuth: candidate.Account.IsOAuth(), Score: candidate.Score, HealthSamples: len(candidate.coreState.HealthObservations)})
		byID[candidate.Account.ID] = candidate
	}
	ordered := orderAdaptiveCandidates(inputs, newSession, settings.AnthropicAdaptiveSchedulerMode == AnthropicAdaptiveSchedulerModeShadow, time.Now(), anthropicAdaptiveCoreSettings(settings))
	result := make([]AnthropicAdaptiveCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		result = append(result, byID[candidate.AccountID])
	}
	return result
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
