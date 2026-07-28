package service

import (
	"bytes"
	"encoding/json"
)

// CleanClaudeToolUseSignatures removes account-bound signatures from Claude
// tool_use history before a Gemini sticky session moves to another account.
// The Gemini compatibility converter replaces a missing signature with its
// account-independent validator bypass value.
func CleanClaudeToolUseSignatures(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"signature"`)) {
		return body
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return body
	}
	messages, ok := request["messages"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		content, ok := message["content"].([]any)
		if !ok {
			continue
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok || block["type"] != "tool_use" {
				continue
			}
			if _, exists := block["signature"]; exists {
				delete(block, "signature")
				changed = true
			}
		}
	}
	if !changed {
		return body
	}
	cleaned, err := json.Marshal(request)
	if err != nil {
		return body
	}
	return cleaned
}
