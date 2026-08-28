package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAdaptiveTTFTBucketKeyUsesRequestedDimensions(t *testing.T) {
	req := OpenAIAccountScheduleRequest{
		RequestedModel:     "openai/gpt-4.1-mini-2025-04-14",
		RequiredCapability: OpenAIEndpointCapabilityResponses,
	}

	httpKey := openAIAdaptiveTTFTBucketKey(req, OpenAIUpstreamTransportHTTPSSE)
	wsKey := openAIAdaptiveTTFTBucketKey(req, OpenAIUpstreamTransportResponsesWebsocketV2)

	require.Equal(t, "gpt-4.1-mini|responses|http", httpKey)
	require.Equal(t, "gpt-4.1-mini|responses|ws", wsKey)
}

func TestOpenAIAdaptiveTTFTBucketKeySeparatesCapabilityWithoutRequestSize(t *testing.T) {
	base := OpenAIAccountScheduleRequest{RequestedModel: "gpt-5.1"}
	chat := base
	chat.RequiredCapability = OpenAIEndpointCapabilityChatCompletions
	responses := base
	responses.RequiredCapability = OpenAIEndpointCapabilityResponses

	require.Equal(t, "gpt-5.1|chat_completions|http", openAIAdaptiveTTFTBucketKey(chat, OpenAIUpstreamTransportHTTPSSE))
	require.Equal(t, "gpt-5.1|responses|http", openAIAdaptiveTTFTBucketKey(responses, OpenAIUpstreamTransportHTTPSSE))
	require.Equal(t, "gpt-5.1|responses|http", openAIAdaptiveTTFTBucketKey(base, OpenAIUpstreamTransportHTTPSSE))
}

func TestOpenAIAdaptiveTTFTModelFamilyRemovesReasoningEffort(t *testing.T) {
	require.Equal(t, "gpt-5.1", openAIAdaptiveTTFTModelFamily("gpt-5.1-high"))
}
