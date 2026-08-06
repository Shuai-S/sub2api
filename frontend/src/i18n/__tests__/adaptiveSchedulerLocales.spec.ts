import { describe, expect, it } from 'vitest'

import enSettings from '../locales/en/admin/settings'
import zhSettings from '../locales/zh/admin/settings'

const schedulerKeys = [
  'anthropicAdaptiveScheduler',
  'geminiAdaptiveScheduler',
  'openaiAdaptiveScheduler'
] as const

const geminiParameterKeys = [
  'accountFailureThreshold',
  'modelFailureThreshold',
  'cooldownMaxSeconds',
  'halfOpenProbeLeaseSeconds'
] as const

describe.each([
  ['zh', zhSettings],
  ['en', enSettings]
] as const)('%s adaptive scheduler locale messages', (locale, messages) => {
  it('defines every scheduler section used by Gateway settings', () => {
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
  })
})
