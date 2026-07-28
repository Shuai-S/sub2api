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
	messages := decoded["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	require.NotContains(t, content[0].(map[string]any), "signature")
	require.Equal(t, "text-signature", content[1].(map[string]any)["signature"])
}

func TestCleanClaudeToolUseSignaturesLeavesUnparseableOrUnchangedBodiesAlone(t *testing.T) {
	invalid := []byte(`{"messages":[`)
	withoutSignature := []byte(`{"messages":[]}`)

	require.Equal(t, invalid, CleanClaudeToolUseSignatures(invalid))
	require.Equal(t, withoutSignature, CleanClaudeToolUseSignatures(withoutSignature))
}
