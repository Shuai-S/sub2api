package service

import (
	"context"
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
	CapacityScore     float64                     `json:"capacity_score"`
	LatencyScore      float64                     `json:"latency_score"`
	CostScore         float64                     `json:"cost_score"`
	CircuitStatus     string                      `json:"circuit_status"`
	coreState         adaptiveAccountState
}

type GeminiAdaptiveScheduleRequest struct {
	RequestedModel string
	Stream         bool
	Action         string
	Candidates     []GeminiAdaptiveCandidateInput
	BaselineOrder  []int64
	Settings       *GeminiAdaptiveSchedulerSettings
	NewSession     bool
	ctx            context.Context
}

type GeminiAdaptiveDecision struct {
	Order                  []GeminiAdaptiveCandidate
	SelectedAccountID      int64
	BaselineAccountID      int64
	InputCandidateCount    int
	CandidateCount         int
	HardRejectedCount      int
	CircuitRejectedCount   int
	HalfOpenCandidateCount int
	TopK                   int
	BuildLatencyMs         int64
	FallbackReason         string
}

func (s *geminiAdaptiveScheduler) BuildOrder(req GeminiAdaptiveScheduleRequest) (GeminiAdaptiveDecision, error) {
	decision := GeminiAdaptiveDecision{InputCandidateCount: len(req.Candidates)}
	if s == nil || s.core == nil || len(req.Candidates) == 0 {
		decision.FallbackReason = "no_candidates"
		return decision, nil
	}
	settings := DefaultGeminiAdaptiveSchedulerSettings()
	if req.Settings != nil {
		settings = NormalizeGeminiAdaptiveSchedulerSettings(*req.Settings)
	}
	now := s.now()
	coreSettings := geminiAdaptiveCoreSettings(settings)
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
			state, allowed := s.core.schedulingSnapshot(input.Account.ID, input.Account.Concurrency, load.CurrentConcurrency, "", now, coreSettings)
			candidate.coreState = state
			candidate.EffectiveCapacity = state.EffectiveCapacity
			if !allowed {
				decision.CircuitRejectedCount++
				continue
			}
			if !state.CircuitOpenUntil.IsZero() && !state.CircuitOpenUntil.After(now) {
				decision.HalfOpenCandidateCount++
				candidate.CircuitStatus = geminiAdaptiveCircuitStatusHalfOpen
			} else {
				candidate.CircuitStatus = geminiAdaptiveCircuitStatusClosed
			}
			if state.EffectiveCapacity > 0 && load.CurrentConcurrency >= state.EffectiveCapacity {
				continue
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
	adaptiveGemini := buildGeminiAdaptiveOrder(geminiCandidates, settings, req.NewSession)
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
	merged := make([]GeminiAdaptiveCandidate, 0, len(baseline))
	seen := make(map[int64]struct{}, len(baseline))
	adaptiveIndex := 0
	for _, candidate := range baseline {
		if candidate.Account.Platform == PlatformGemini && adaptiveIndex < len(adaptive) {
			candidate = adaptive[adaptiveIndex]
			adaptiveIndex++
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
	_, _, _ = requestedModel, action, stream
	inputs := make([]adaptiveScoreCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		inputs = append(inputs, adaptiveScoreCandidate{
			AccountID:          candidate.Account.ID,
			OAuth:              candidate.Account.IsOAuth(),
			Cost:               candidate.Account.BillingRateMultiplier(),
			CurrentConcurrency: candidate.Load.CurrentConcurrency,
			State:              candidate.coreState,
		})
	}
	scored := scoreAdaptiveCandidates(inputs, now, geminiAdaptiveCoreSettings(settings))
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
	}
}

func buildGeminiAdaptiveOrder(candidates []GeminiAdaptiveCandidate, settings GeminiAdaptiveSchedulerSettings, newSessionValue ...bool) []GeminiAdaptiveCandidate {
	newSession := false
	if len(newSessionValue) > 0 {
		newSession = newSessionValue[0]
	}
	inputs := make([]adaptiveScoreCandidate, 0, len(candidates))
	byID := make(map[int64]GeminiAdaptiveCandidate, len(candidates))
	for _, candidate := range candidates {
		inputs = append(inputs, adaptiveScoreCandidate{AccountID: candidate.Account.ID, OAuth: candidate.Account.IsOAuth(), Score: candidate.Score, HealthSamples: len(candidate.coreState.HealthObservations)})
		byID[candidate.Account.ID] = candidate
	}
	ordered := orderAdaptiveCandidates(inputs, newSession, settings.GeminiAdaptiveSchedulerMode == GeminiAdaptiveSchedulerModeShadow, time.Now(), geminiAdaptiveCoreSettings(settings))
	result := make([]GeminiAdaptiveCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		result = append(result, byID[candidate.AccountID])
	}
	return result
}
