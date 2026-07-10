package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type blockingSchedulerSettingRepo struct {
	enabledStarted  chan struct{}
	enabledRelease  chan struct{}
	enabledValue    string
	allStarted      chan struct{}
	allRelease      chan struct{}
	allValues       map[string]string
	multipleStarted chan struct{}
	multipleRelease chan struct{}
	multipleValues  map[string]string
}

func waitForSchedulerSettingRelease(ctx context.Context, started, release chan struct{}) error {
	if started != nil {
		close(started)
	}
	if release == nil {
		return nil
	}
	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *blockingSchedulerSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *blockingSchedulerSettingRepo) GetValue(ctx context.Context, _ string) (string, error) {
	if err := waitForSchedulerSettingRelease(ctx, r.enabledStarted, r.enabledRelease); err != nil {
		return "", err
	}
	return r.enabledValue, nil
}

func (r *blockingSchedulerSettingRepo) Set(context.Context, string, string) error {
	return nil
}

func (r *blockingSchedulerSettingRepo) GetMultiple(ctx context.Context, _ []string) (map[string]string, error) {
	if err := waitForSchedulerSettingRelease(ctx, r.multipleStarted, r.multipleRelease); err != nil {
		return nil, err
	}
	return r.multipleValues, nil
}

func (r *blockingSchedulerSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (r *blockingSchedulerSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	if err := waitForSchedulerSettingRelease(ctx, r.allStarted, r.allRelease); err != nil {
		return nil, err
	}
	return r.allValues, nil
}

func (r *blockingSchedulerSettingRepo) Delete(context.Context, string) error {
	return nil
}

func newGatewayWithSchedulerSettingRepo(repo SettingRepository) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		rateLimitService: &RateLimitService{
			settingService: NewSettingService(repo, &config.Config{}),
		},
	}
}

func TestDefaultOpenAIAdaptiveSchedulerSettingsBalanceAvailabilityAndCost(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()

	require.False(t, cfg.OpenAIAdaptiveSchedulerEnabled)
	require.False(t, cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, openAIAdaptiveSchedulerModeEnforce, cfg.OpenAIAdaptiveSchedulerMode)
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityMixed, cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode)
	require.Equal(t, 10, cfg.OpenAIAdaptiveSchedulerTopK)
	require.Equal(t, 0.01, cfg.OpenAIAdaptiveSchedulerExplorationRate)
	require.Equal(t, 0.35, cfg.OpenAIAdaptiveSchedulerSoftmaxTemperature)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerMinCostMultiplier)
	require.True(t, cfg.OpenAIAdaptiveSchedulerThompsonEnabled)

	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerInitialCapacityFraction)
	require.Equal(t, 2, cfg.OpenAIAdaptiveSchedulerMinCapacity)
	require.Equal(t, 2, cfg.OpenAIAdaptiveSchedulerCapacityIncreaseStep)
	require.Equal(t, 1.15, cfg.OpenAIAdaptiveSchedulerCapacityGrowthFactor)
	require.Equal(t, 0.80, cfg.OpenAIAdaptiveSchedulerCapacityProbeLoadThreshold)
	require.Equal(t, 0.15, cfg.OpenAIAdaptiveSchedulerBurstProbeRatio)
	require.Equal(t, 0.95, cfg.OpenAIAdaptiveSchedulerCapacitySuccessThreshold)
	require.Equal(t, 3, cfg.OpenAIAdaptiveSchedulerCapacityFailureThreshold)
	require.Equal(t, 50, cfg.OpenAIAdaptiveSchedulerMinRecentSamplesForShrink)
	require.Equal(t, 0.35, cfg.OpenAIAdaptiveSchedulerShrinkErrorThreshold)
	require.Equal(t, 0.90, cfg.OpenAIAdaptiveSchedulerShrinkFactorSoft)
	require.Equal(t, 0.70, cfg.OpenAIAdaptiveSchedulerShrinkFactorHard)
	require.Equal(t, 1, cfg.OpenAIAdaptiveSchedulerHalfOpenFailureThreshold)
	require.Equal(t, 3, cfg.OpenAIAdaptiveSchedulerHalfOpenProbeCapacity)
	require.Equal(t, 1200, cfg.OpenAIAdaptiveSchedulerLearningWindowSeconds)

	require.Equal(t, 0.04, cfg.OpenAIAdaptiveSchedulerSuccessEMAAlpha)
	require.Equal(t, 0.06, cfg.OpenAIAdaptiveSchedulerErrorEMAAlpha)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerLatencyEMAAlpha)
	require.Equal(t, 0.05, cfg.OpenAIAdaptiveSchedulerTTFTEMAAlpha)
	require.Equal(t, 60, cfg.OpenAIAdaptiveSchedulerCooldownBaseSeconds)
	require.Equal(t, 600, cfg.OpenAIAdaptiveSchedulerCooldownMaxSeconds)

	require.Equal(t, 0.40, cfg.OpenAIAdaptiveSchedulerWeightSuccess)
	require.Equal(t, 0.25, cfg.OpenAIAdaptiveSchedulerWeightCost)
	require.Equal(t, 0.20, cfg.OpenAIAdaptiveSchedulerWeightCapacity)
	require.Equal(t, 0.10, cfg.OpenAIAdaptiveSchedulerWeightLatency)
	require.Equal(t, 0.03, cfg.OpenAIAdaptiveSchedulerWeightStability)
	require.Equal(t, 0.02, cfg.OpenAIAdaptiveSchedulerWeightExploration)
}

func TestOpenAIAdaptiveSchedulerDiagnosticSettingsRoundTrip(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogEnabled = true
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 0.25
	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst

	values := openAIAdaptiveSchedulerSettingsToMap(cfg)
	require.Equal(t, "true", values[openAIAdaptiveSchedulerDiagnosticLogEnabledKey])
	require.Equal(t, "0.25", values[openAIAdaptiveSchedulerDiagnosticLogSampleRateKey])
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst, values[openAIAdaptiveSchedulerAccountTypePriorityModeKey])

	parsed := parseOpenAIAdaptiveSchedulerSettings(values)
	require.True(t, parsed.OpenAIAdaptiveSchedulerDiagnosticLogEnabled)
	require.Equal(t, 0.25, parsed.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityOAuthFirst, parsed.OpenAIAdaptiveSchedulerAccountTypePriorityMode)
}

func TestNormalizeOpenAIAdaptiveSchedulerDiagnosticSampleRate(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate = 2

	normalized := NormalizeOpenAIAdaptiveSchedulerSettings(cfg)
	require.Equal(t, 0.05, normalized.OpenAIAdaptiveSchedulerDiagnosticLogSampleRate)
}

func TestNormalizeOpenAIAdaptiveSchedulerAccountTypePriorityMode(t *testing.T) {
	cfg := DefaultOpenAIAdaptiveSchedulerSettings()
	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = "api_key_first"
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityAPIKeyFirst, NormalizeOpenAIAdaptiveSchedulerSettings(cfg).OpenAIAdaptiveSchedulerAccountTypePriorityMode)

	cfg.OpenAIAdaptiveSchedulerAccountTypePriorityMode = "not-a-mode"
	require.Equal(t, openAIAdaptiveSchedulerAccountTypePriorityMixed, NormalizeOpenAIAdaptiveSchedulerSettings(cfg).OpenAIAdaptiveSchedulerAccountTypePriorityMode)
}

func TestOpenAIAdaptiveSchedulerEnabledRefreshWinsAgainstStaleRead(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	started := make(chan struct{})
	release := make(chan struct{})
	repo := &blockingSchedulerSettingRepo{
		enabledStarted: started,
		enabledRelease: release,
		enabledValue:   "false",
	}
	svc := newGatewayWithSchedulerSettingRepo(repo)
	resultCh := make(chan bool, 1)
	go func() {
		resultCh <- svc.isOpenAIAdaptiveSchedulerEnabled(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stale enabled read")
	}
	updated := DefaultOpenAIAdaptiveSchedulerSettings()
	updated.OpenAIAdaptiveSchedulerEnabled = true
	refreshOpenAIAdaptiveSchedulerSettingCache(updated)
	close(release)

	select {
	case enabled := <-resultCh:
		require.True(t, enabled, "in-flight stale DB read must return the refreshed setting")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for enabled read result")
	}
	cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting)
	require.True(t, ok)
	require.NotNil(t, cached)
	require.True(t, cached.settings.OpenAIAdaptiveSchedulerEnabled)
}

func TestOpenAIAdaptiveSchedulerSettingsRefreshWinsAgainstStaleRead(t *testing.T) {
	resetOpenAIAdaptiveSchedulerSettingCacheForTest()
	defer resetOpenAIAdaptiveSchedulerSettingCacheForTest()

	started := make(chan struct{})
	release := make(chan struct{})
	repo := &blockingSchedulerSettingRepo{
		allStarted: started,
		allRelease: release,
		allValues: map[string]string{
			openAIAdaptiveSchedulerEnabledKey: "true",
			openAIAdaptiveSchedulerTopKKey:    "1",
		},
	}
	svc := newGatewayWithSchedulerSettingRepo(repo)
	resultCh := make(chan OpenAIAdaptiveSchedulerSettings, 1)
	go func() {
		resultCh <- svc.openAIAdaptiveSchedulerSettings(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stale adaptive settings read")
	}
	updated := DefaultOpenAIAdaptiveSchedulerSettings()
	updated.OpenAIAdaptiveSchedulerEnabled = true
	updated.OpenAIAdaptiveSchedulerTopK = 17
	refreshOpenAIAdaptiveSchedulerSettingCache(updated)
	close(release)

	select {
	case settings := <-resultCh:
		require.True(t, settings.OpenAIAdaptiveSchedulerEnabled)
		require.Equal(t, 17, settings.OpenAIAdaptiveSchedulerTopK)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for adaptive settings result")
	}
	cached, ok := openAIAdaptiveSchedulerSettingCache.Load().(*cachedOpenAIAdaptiveSchedulerSetting)
	require.True(t, ok)
	require.NotNil(t, cached)
	require.Equal(t, 17, cached.settings.OpenAIAdaptiveSchedulerTopK)
}

func TestOpenAIAdvancedSchedulerRefreshWinsAgainstStaleRead(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	started := make(chan struct{})
	release := make(chan struct{})
	repo := &blockingSchedulerSettingRepo{
		multipleStarted: started,
		multipleRelease: release,
		multipleValues: map[string]string{
			openAIAdvancedSchedulerSettingKey:       "false",
			SettingKeyOpenAIAdvancedSchedulerLBTopK: "1",
		},
	}
	svc := newGatewayWithSchedulerSettingRepo(repo)
	resultCh := make(chan openAIAdvancedSchedulerRuntimeSettings, 1)
	go func() {
		resultCh <- svc.openAIAdvancedSchedulerRuntimeSettings(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stale advanced settings read")
	}
	refreshOpenAIAdvancedSchedulerSettingCache(openAIAdvancedSchedulerRuntimeSettings{
		enabled:        true,
		lbTopKOverride: 19,
	})
	close(release)

	select {
	case settings := <-resultCh:
		require.True(t, settings.enabled)
		require.Equal(t, 19, settings.lbTopKOverride)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for advanced settings result")
	}
	cached, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting)
	require.True(t, ok)
	require.NotNil(t, cached)
	require.True(t, cached.enabled)
	require.Equal(t, 19, cached.lbTopKOverride)
}
