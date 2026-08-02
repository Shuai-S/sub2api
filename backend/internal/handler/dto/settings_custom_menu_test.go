package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUserVisibleMenuItemsStripsProtectedModalContent(t *testing.T) {
	raw := `[
		{"id":"public","label":"Public","url":"","visibility":"user","placement":"header","modal_title":"Secret title","modal_content":"Secret body"},
		{"id":"admin","label":"Admin","url":"","visibility":"admin","placement":"header","modal_title":"Admin title","modal_content":"Admin body"}
	]`

	items := ParseUserVisibleMenuItems(raw)
	require.Len(t, items, 1)
	require.Equal(t, "public", items[0].ID)
	require.Empty(t, items[0].ModalTitle)
	require.Empty(t, items[0].ModalContent)

	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "Secret")
	require.NotContains(t, string(encoded), "modal_content")
}
