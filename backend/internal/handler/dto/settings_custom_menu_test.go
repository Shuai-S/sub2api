package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUserVisibleMenuItemsStripsProtectedModalContent(t *testing.T) {
	raw := `[
		{"id":"public","label":"Public","url":"","visibility":"user","placement":"header","modal_title":"Secret title","modal_content":"Secret body","modal_title_i18n":{"en":"Secret title"},"modal_content_i18n":{"en":"Secret body"}},
		{"id":"admin","label":"Admin","url":"","visibility":"admin","placement":"header","modal_title":"Admin title","modal_content":"Admin body"}
	]`

	items := ParseUserVisibleMenuItems(raw)
	require.Len(t, items, 1)
	require.Equal(t, "public", items[0].ID)
	require.Empty(t, items[0].ModalTitle)
	require.Empty(t, items[0].ModalContent)
	require.Empty(t, items[0].ModalTitleI18n)
	require.Empty(t, items[0].ModalContentI18n)

	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "Secret")
	require.NotContains(t, string(encoded), "modal_content")
}

func TestResolveLocalizedTextFallsBackToLegacyValue(t *testing.T) {
	require.Equal(t, "中文", ResolveLocalizedText(map[string]string{"zh": "中文"}, "en", "Legacy"))
	require.Equal(t, "中文", ResolveLocalizedText(map[string]string{"en": "English", "zh": "中文"}, "zh", "Legacy"))
	require.Equal(t, "English", ResolveLocalizedText(map[string]string{"en": "English"}, "zh", "Legacy"))
	require.Equal(t, "Legacy", ResolveLocalizedText(nil, "en", "Legacy"))
}
