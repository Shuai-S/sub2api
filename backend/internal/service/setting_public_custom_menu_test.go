package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterUserVisibleMenuItemsRemovesProtectedFields(t *testing.T) {
	raw := `[
		{"id":"public","label":"Public","visibility":"user","placement":"header","modal_title":"Secret title","modal_content":"Secret body"},
		{"id":"admin","label":"Admin","visibility":"admin","placement":"header","modal_content":"Admin body"}
	]`

	filtered := filterUserVisibleMenuItems(raw)
	var items []map[string]any
	require.NoError(t, json.Unmarshal(filtered, &items))
	require.Len(t, items, 1)
	require.Equal(t, "public", items[0]["id"])
	require.NotContains(t, items[0], "modal_title")
	require.NotContains(t, items[0], "modal_content")
}
