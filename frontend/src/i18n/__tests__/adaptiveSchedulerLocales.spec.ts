import { describe, expect, it } from 'vitest'

import enSettings from '../locales/en/admin/settings'
import zhSettings from '../locales/zh/admin/settings'

const schedulerKeys = [
  'anthropicAdaptiveScheduler',
  'geminiAdaptiveScheduler',
  'openaiAdaptiveScheduler'
] as const

const geminiParameterKeys = [
  'healthFailureThreshold',
  'cooldownMaxSeconds'
] as const

const openaiRecoveryParameterKeys = [
  'recoveryExplorationRate',
  'recoveryMaxConcurrency',
  'recoveryWarmupSuccesses'
] as const

describe.each([
  ['zh', zhSettings],
  ['en', enSettings]
] as const)('%s adaptive scheduler locale messages', (locale, messages) => {
  it('defines the adaptive scheduling tab and every provider section', () => {
    expect(messages.settings.tabs.adaptive, `${locale}.tabs.adaptive is missing`).toBeTypeOf('string')
    expect(messages.settings.adaptiveScheduling.title, `${locale}.adaptiveScheduling.title is missing`).toBeTypeOf('string')
    expect(messages.settings.adaptiveScheduling.description, `${locale}.adaptiveScheduling.description is missing`).toBeTypeOf('string')

    for (const key of schedulerKeys) {
      const section = messages.settings[key]
      expect(section, `${locale}.${key} is missing`).toBeDefined()
      expect(section.title, `${locale}.${key}.title is missing`).toBeTypeOf('string')
      expect(section.description, `${locale}.${key}.description is missing`).toBeTypeOf('string')
      expect(section.mode, `${locale}.${key}.mode is missing`).toBeTypeOf('string')
      expect(section.modes?.shadow, `${locale}.${key}.modes.shadow is missing`).toBeTypeOf('string')
      expect(section.modes?.enforce, `${locale}.${key}.modes.enforce is missing`).toBeTypeOf('string')
    }

    const gemini = messages.settings.geminiAdaptiveScheduler
    for (const key of geminiParameterKeys) {
      expect(gemini.parameters[key], `${locale}.geminiAdaptiveScheduler.parameters.${key} is missing`).toBeTypeOf('string')
      expect(gemini.tooltips[key], `${locale}.geminiAdaptiveScheduler.tooltips.${key} is missing`).toBeTypeOf('string')
    }

    const openai = messages.settings.openaiAdaptiveScheduler
    for (const key of openaiRecoveryParameterKeys) {
      expect(openai.parameters[key], `${locale}.openaiAdaptiveScheduler.parameters.${key} is missing`).toBeTypeOf('string')
      expect(openai.tooltips[key], `${locale}.openaiAdaptiveScheduler.tooltips.${key} is missing`).toBeTypeOf('string')
    }
  })
})
