package service

import "strings"

const (
	openAIAdaptiveTTFTTransportHTTP = "http"
	openAIAdaptiveTTFTTransportWS   = "ws"
)

func (s *adaptiveOpenAIAccountScheduler) ttftBucketKey(req OpenAIAccountScheduleRequest, account *Account) string {
	transport := req.RequiredTransport
	if transport == OpenAIUpstreamTransportResponsesWebsocketV2Ingress && s != nil && s.service != nil && account != nil {
		transport = s.service.getOpenAIWSProtocolResolver().Resolve(account).Transport
	}
	return openAIAdaptiveTTFTBucketKey(req, transport)
}

func openAIAdaptiveTTFTBucketKey(req OpenAIAccountScheduleRequest, transport OpenAIUpstreamTransport) string {
	return strings.Join([]string{
		openAIAdaptiveTTFTModelFamily(req.RequestedModel),
		openAIAdaptiveTTFTCapability(req),
		openAIAdaptiveTTFTTransportClass(transport),
	}, "|")
}

func openAIAdaptiveTTFTModelFamily(model string) string {
	normalized := strings.ToLower(lastOpenAIModelSegment(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.Join(strings.Fields(normalized), "-")
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}
	normalized = trimOpenAIAdaptiveTTFTDateSuffix(normalized)
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		if strings.HasSuffix(normalized, "-"+effort) {
			normalized = strings.TrimSuffix(normalized, "-"+effort)
			break
		}
	}
	if normalized == "" {
		return "unknown"
	}
	if len(normalized) > 96 {
		normalized = normalized[:96]
	}
	return normalized
}

func trimOpenAIAdaptiveTTFTDateSuffix(model string) string {
	parts := strings.Split(model, "-")
	if len(parts) < 4 {
		return model
	}
	year, month, day := parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
	if len(year) != 4 || len(month) != 2 || len(day) != 2 || !asciiDigits(year) || !asciiDigits(month) || !asciiDigits(day) {
		return model
	}
	return strings.Join(parts[:len(parts)-3], "-")
}

func asciiDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return value != ""
}

func openAIAdaptiveTTFTCapability(req OpenAIAccountScheduleRequest) string {
	if capability := strings.TrimSpace(string(req.RequiredCapability)); capability != "" {
		return sanitizeOpenAIAdaptiveTTFTKeyPart(capability)
	}
	if imageCapability := strings.TrimSpace(string(req.RequiredImageCapability)); imageCapability != "" {
		return "images:" + sanitizeOpenAIAdaptiveTTFTKeyPart(imageCapability)
	}
	if req.RequireCompact {
		return "responses_compact"
	}
	return string(OpenAIEndpointCapabilityResponses)
}

func openAIAdaptiveTTFTTransportClass(transport OpenAIUpstreamTransport) string {
	switch transport {
	case OpenAIUpstreamTransportResponsesWebsocket,
		OpenAIUpstreamTransportResponsesWebsocketV2,
		OpenAIUpstreamTransportResponsesWebsocketV2Ingress:
		return openAIAdaptiveTTFTTransportWS
	default:
		return openAIAdaptiveTTFTTransportHTTP
	}
}

func sanitizeOpenAIAdaptiveTTFTKeyPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "|", "_")
	if value == "" {
		return "default"
	}
	return value
}
