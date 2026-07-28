package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanClaudeToolUseSignaturesOnlyRemovesToolUseSignatures(t *testing.T) {
	body := []byte(`{
		"signature":"request-signature",
		"messages":[{
			"role":"assistant",
			"content":[
				{"type":"tool_use","id":"tool-1","name":"lookup","input":{},"signature":"account-bound"},
				{"type":"text","text":"done","signature":"text-signature"}
			]
		}]
	}`)

	cleaned := CleanClaudeToolUseSignatures(body)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(cleaned, &decoded))
	require.Equal(t, "request-signature", decoded["signature"])
	messages, ok := decoded["messages"].([]any)
	require.True(t, ok)
	message, ok := messages[0].(map[string]any)
	require.True(t, ok)
	content, ok := message["content"].([]any)
	require.True(t, ok)
	toolUse, ok := content[0].(map[string]any)
	require.True(t, ok)
	text, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, toolUse, "signature")
	require.Equal(t, "text-signature", text["signature"])
}

func TestCleanClaudeToolUseSignaturesLeavesUnparseableOrUnchangedBodiesAlone(t *testing.T) {
	invalid := []byte(`{"messages":[`)
	withoutSignature := []byte(`{"messages":[]}`)

	require.Equal(t, invalid, CleanClaudeToolUseSignatures(invalid))
	require.Equal(t, withoutSignature, CleanClaudeToolUseSignatures(withoutSignature))
}
