const UPSTREAM_BILLING_PLATFORMS = new Set(['openai', 'anthropic', 'gemini', 'grok'])

export function supportsUpstreamBilling(platform: string, type: string): boolean {
  return type === 'apikey' && UPSTREAM_BILLING_PLATFORMS.has(platform)
}
