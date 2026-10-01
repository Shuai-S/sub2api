import { describe, expect, it } from 'vitest'
import { resolveCustomMenuLabel, resolveCustomMenuText } from '@/utils/customMenu'

describe('custom menu localization', () => {
  it('uses the requested language before the alternate language', () => {
    expect(resolveCustomMenuText({ zh: '中文', en: 'English' }, 'zh', 'Legacy')).toBe('中文')
    expect(resolveCustomMenuText({ zh: '中文', en: 'English' }, 'en', 'Legacy')).toBe('English')
  })

  it('falls back when the requested translation is missing', () => {
    expect(resolveCustomMenuText({ zh: '中文' }, 'en', 'Legacy')).toBe('中文')
    expect(resolveCustomMenuText(undefined, 'en', 'Legacy')).toBe('Legacy')
  })

  it('resolves labels without breaking legacy menu items', () => {
    expect(resolveCustomMenuLabel({ label: 'Legacy', label_i18n: { en: 'English' } }, 'en')).toBe('English')
    expect(resolveCustomMenuLabel({ label: 'Legacy' }, 'zh')).toBe('Legacy')
  })
})
