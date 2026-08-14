package service

import "context"

func (s *OpenAIGatewayService) startOpenAIAdaptiveStatePersistence() {
	if s == nil {
		return
	}
	s.openaiAdaptivePersistenceOnce.Do(func() {
		cache, ok := s.cache.(AdaptiveSchedulerStateCache)
		if !ok || cache == nil {
			return
		}
		if s.openaiAdaptiveCore == nil {
			s.openaiAdaptiveCore = newAdaptiveStateStore()
		}
		s.openaiAdaptivePersistence = newAdaptiveCoreStatePersistence(cache, s.openaiAdaptiveCore, adaptiveSchedulerCoreNamespaceOpenAI)
		s.openaiAdaptivePersistence.Start()
	})
}

func (s *OpenAIGatewayService) CloseOpenAIAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil || s.openaiAdaptivePersistence == nil {
		return nil
	}
	return s.openaiAdaptivePersistence.Stop(ctx)
}
