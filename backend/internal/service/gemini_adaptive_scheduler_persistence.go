package service

import "context"

func (s *GatewayService) startGeminiAdaptiveStatePersistence() {
	if s == nil || s.geminiAdaptiveScheduler == nil {
		return
	}
	s.geminiStatePersistOnce.Do(func() {
		cache, ok := s.cache.(AdaptiveSchedulerStateCache)
		if !ok || cache == nil {
			return
		}
		s.geminiStatePersistence = newAdaptiveCoreStatePersistence(cache, s.geminiAdaptiveScheduler.core, adaptiveSchedulerCoreNamespaceGemini)
		s.geminiStatePersistence.Start()
	})
}

func (s *GatewayService) CloseGeminiAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil || s.geminiStatePersistence == nil {
		return nil
	}
	return s.geminiStatePersistence.Stop(ctx)
}
