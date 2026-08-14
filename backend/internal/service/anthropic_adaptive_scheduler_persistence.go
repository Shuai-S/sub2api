package service

import "context"

func (s *GatewayService) startAnthropicAdaptiveStatePersistence() {
	if s == nil || s.anthropicAdaptiveScheduler == nil {
		return
	}
	s.anthropicStatePersistOnce.Do(func() {
		cache, ok := s.cache.(AdaptiveSchedulerStateCache)
		if !ok || cache == nil {
			return
		}
		s.anthropicStatePersistence = newAdaptiveCoreStatePersistence(cache, s.anthropicAdaptiveScheduler.core, adaptiveSchedulerCoreNamespaceAnthropic)
		s.anthropicStatePersistence.Start()
	})
}

func (s *GatewayService) CloseAnthropicAdaptiveStatePersistence(ctx context.Context) error {
	if s == nil || s.anthropicStatePersistence == nil {
		return nil
	}
	return s.anthropicStatePersistence.Stop(ctx)
}
