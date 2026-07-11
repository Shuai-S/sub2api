package service

import (
	"sync"
	"time"
)

const openAIAccountImageStreamCooldownDuration = 30 * time.Minute

type openAIAccountImageStreamCooldowns struct {
	untilByAccount sync.Map // key: int64(accountID), value: time.Time
}

func (c *openAIAccountImageStreamCooldowns) mark(accountID int64, now time.Time) {
	if c == nil || accountID <= 0 {
		return
	}
	c.untilByAccount.Store(accountID, now.Add(openAIAccountImageStreamCooldownDuration))
}

func (c *openAIAccountImageStreamCooldowns) active(accountID int64, now time.Time) bool {
	if c == nil || accountID <= 0 {
		return false
	}
	value, ok := c.untilByAccount.Load(accountID)
	if !ok {
		return false
	}
	until, ok := value.(time.Time)
	if !ok || !now.Before(until) {
		c.untilByAccount.Delete(accountID)
		return false
	}
	return true
}

// MarkOpenAIImageStreamUnsupported temporarily removes an account from
// requests that require an upstream image stream.
func (s *OpenAIGatewayService) MarkOpenAIImageStreamUnsupported(accountID int64) {
	if s == nil {
		return
	}
	s.openaiImageStreamCooldowns.mark(accountID, time.Now())
}

// IsOpenAIImageStreamUnsupported reports whether the account is currently
// excluded from requests that require an upstream image stream.
func (s *OpenAIGatewayService) IsOpenAIImageStreamUnsupported(accountID int64) bool {
	return s != nil && s.openaiImageStreamCooldowns.active(accountID, time.Now())
}

func openAIAccountRequiresImageStream(account *Account, requiredImageCapability OpenAIImagesCapability, clientStream bool) bool {
	if account == nil || requiredImageCapability == "" {
		return false
	}
	if account.IsOAuth() {
		return true
	}
	return account.Type == AccountTypeAPIKey && clientStream
}

func (s *OpenAIGatewayService) accountSupportsOpenAIRequestCapabilities(
	account *Account,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
	clientImageStream bool,
) bool {
	if !accountSupportsOpenAICapabilities(account, requiredCapability, requiredImageCapability) {
		return false
	}
	if !openAIAccountRequiresImageStream(account, requiredImageCapability, clientImageStream) {
		return true
	}
	return account.SupportsOpenAIImagesStream() && !s.IsOpenAIImageStreamUnsupported(account.ID)
}
