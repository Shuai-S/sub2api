package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsNormalizesLegacyCustomMenuItem(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{
		"custom_menu_items": []map[string]any{{
			"id": "legacy", "label": "Legacy", "icon_svg": "", "url": "https://example.com",
			"visibility": "user", "sort_order": 0,
		}},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var items []dto.CustomMenuItem
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCustomMenuItems]), &items))
	require.Equal(t, "sidebar", items[0].Placement)
	require.Equal(t, "embedded", items[0].OpenMode)
}

func TestUpdateSettingsAcceptsHeaderModalWithoutURL(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{
		"custom_menu_items": []map[string]any{{
			"id": "notice", "label": "须知", "icon_svg": "", "url": "", "visibility": "user",
			"sort_order": 0, "placement": "header", "modal_title": "使用须知", "modal_content": "# 内容",
		}},
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, repo.values[service.SettingKeyCustomMenuItems], `"placement":"header"`)
}

func TestUpdateSettingsRejectsInvalidCustomMenuModes(t *testing.T) {
	tests := []struct {
		name string
		item map[string]any
	}{
		{
			name: "header content required",
			item: map[string]any{"id": "notice", "label": "Notice", "url": "", "visibility": "user", "placement": "header", "modal_content": ""},
		},
		{
			name: "markdown cannot open in new tab",
			item: map[string]any{"id": "guide", "label": "Guide", "url": "md:guide", "visibility": "user", "placement": "sidebar", "open_mode": "new_tab"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newStepUpSwitchTestHandler(t, map[string]string{})
			rec := doUpdateSettings(t, h, map[string]any{"custom_menu_items": []map[string]any{tt.item}}, nil)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
}
