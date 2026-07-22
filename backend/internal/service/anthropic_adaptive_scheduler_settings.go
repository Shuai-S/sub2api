package service

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	AnthropicAdaptiveSchedulerModeShadow  = "shadow"
	AnthropicAdaptiveSchedulerModeEnforce = "enforce"
)

const (
	SettingKeyAnthropicAdaptiveSchedulerEnabled = "anthropic_adaptive_scheduler_enabled"
	SettingKeyAnthropicAdaptiveSchedulerMode    = "anthropic_adaptive_scheduler_mode"

	anthropicAdaptiveSchedulerSettingCacheTTL  = 5 * time.Second
	anthropicAdaptiveSchedulerSettingDBTimeout = 2 * time.Second
)

type AnthropicAdaptiveSchedulerSettings struct {
	AnthropicAdaptiveSchedulerEnabled bool   `json:"anthropic_adaptive_scheduler_enabled"`
	AnthropicAdaptiveSchedulerMode    string `json:"anthropic_adaptive_scheduler_mode"`
}

type cachedAnthropicAdaptiveSchedulerSettings struct {
	settings  AnthropicAdaptiveSchedulerSettings
	expiresAt int64
}

var anthropicAdaptiveSchedulerSettingCache atomic.Value // *cachedAnthropicAdaptiveSchedulerSettings
var anthropicAdaptiveSchedulerSettingSF singleflight.Group
var anthropicAdaptiveSchedulerSettingGeneration atomic.Uint64

func DefaultAnthropicAdaptiveSchedulerSettings() AnthropicAdaptiveSchedulerSettings {
	return AnthropicAdaptiveSchedulerSettings{
		AnthropicAdaptiveSchedulerEnabled: false,
		AnthropicAdaptiveSchedulerMode:    AnthropicAdaptiveSchedulerModeShadow,
	}
}

func NormalizeAnthropicAdaptiveSchedulerSettings(settings AnthropicAdaptiveSchedulerSettings) AnthropicAdaptiveSchedulerSettings {
	settings.AnthropicAdaptiveSchedulerMode = normalizeAnthropicAdaptiveSchedulerMode(settings.AnthropicAdaptiveSchedulerMode)
	return settings
}

func normalizeAnthropicAdaptiveSchedulerMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AnthropicAdaptiveSchedulerModeEnforce:
		return AnthropicAdaptiveSchedulerModeEnforce
	case AnthropicAdaptiveSchedulerModeShadow:
		return AnthropicAdaptiveSchedulerModeShadow
	default:
		return AnthropicAdaptiveSchedulerModeShadow
	}
}

func parseAnthropicAdaptiveSchedulerSettings(values map[string]string) AnthropicAdaptiveSchedulerSettings {
	settings := DefaultAnthropicAdaptiveSchedulerSettings()
	settings.AnthropicAdaptiveSchedulerEnabled = values[SettingKeyAnthropicAdaptiveSchedulerEnabled] == "true"
	if mode, ok := values[SettingKeyAnthropicAdaptiveSchedulerMode]; ok {
		settings.AnthropicAdaptiveSchedulerMode = mode
	}
	return NormalizeAnthropicAdaptiveSchedulerSettings(settings)
}

func anthropicAdaptiveSchedulerSettingsToMap(settings AnthropicAdaptiveSchedulerSettings) map[string]string {
	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)
	return map[string]string{
		SettingKeyAnthropicAdaptiveSchedulerEnabled: formatBool(settings.AnthropicAdaptiveSchedulerEnabled),
		SettingKeyAnthropicAdaptiveSchedulerMode:    settings.AnthropicAdaptiveSchedulerMode,
	}
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (s *SettingService) GetAnthropicAdaptiveSchedulerSettings(ctx context.Context) (AnthropicAdaptiveSchedulerSettings, error) {
	defaults := DefaultAnthropicAdaptiveSchedulerSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	if cached, _ := anthropicAdaptiveSchedulerSettingCache.Load().(*cachedAnthropicAdaptiveSchedulerSettings); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.settings, nil
	}

	generation := anthropicAdaptiveSchedulerSettingGeneration.Load()
	value, err, _ := anthropicAdaptiveSchedulerSettingSF.Do("settings", func() (any, error) {
		if cached, _ := anthropicAdaptiveSchedulerSettingCache.Load().(*cachedAnthropicAdaptiveSchedulerSettings); cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached.settings, nil
		}
		dbCtx, cancel := context.WithTimeout(ctx, anthropicAdaptiveSchedulerSettingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyAnthropicAdaptiveSchedulerEnabled,
			SettingKeyAnthropicAdaptiveSchedulerMode,
		})
		if err != nil {
			return defaults, err
		}
		settings := parseAnthropicAdaptiveSchedulerSettings(values)
		if anthropicAdaptiveSchedulerSettingGeneration.Load() == generation {
			anthropicAdaptiveSchedulerSettingCache.Store(&cachedAnthropicAdaptiveSchedulerSettings{
				settings:  settings,
				expiresAt: time.Now().Add(anthropicAdaptiveSchedulerSettingCacheTTL).UnixNano(),
			})
		}
		return settings, nil
	})
	if err != nil {
		return defaults, err
	}
	return value.(AnthropicAdaptiveSchedulerSettings), nil
}

func refreshAnthropicAdaptiveSchedulerSettingCache(settings AnthropicAdaptiveSchedulerSettings) {
	settings = NormalizeAnthropicAdaptiveSchedulerSettings(settings)
	anthropicAdaptiveSchedulerSettingGeneration.Add(1)
	anthropicAdaptiveSchedulerSettingSF.Forget("settings")
	anthropicAdaptiveSchedulerSettingCache.Store(&cachedAnthropicAdaptiveSchedulerSettings{
		settings:  settings,
		expiresAt: time.Now().Add(anthropicAdaptiveSchedulerSettingCacheTTL).UnixNano(),
	})
}

func resetAnthropicAdaptiveSchedulerSettingCacheForTest() {
	anthropicAdaptiveSchedulerSettingGeneration.Add(1)
	anthropicAdaptiveSchedulerSettingSF.Forget("settings")
	anthropicAdaptiveSchedulerSettingCache = atomic.Value{}
}
