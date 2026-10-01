import type {
  CustomMenuItem,
  CustomMenuLocale,
  CustomMenuLocalizedText,
} from '@/types'

type CustomMenuLabelSource = Pick<CustomMenuItem, 'label' | 'label_i18n'>

export function resolveCustomMenuText(
  values: CustomMenuLocalizedText | undefined,
  locale: string,
  legacy = '',
): string {
  const normalizedLocale = locale === 'zh' ? 'zh' : 'en'
  const candidates = [normalizedLocale, normalizedLocale === 'en' ? 'zh' : 'en'] as const
  for (const candidate of candidates) {
    const value = values?.[candidate]?.trim()
    if (value) return value
  }
  return typeof legacy === 'string' ? legacy.trim() : ''
}

export function resolveCustomMenuLabel(item: CustomMenuLabelSource, locale: string): string {
  return resolveCustomMenuText(item.label_i18n, locale, item.label)
}

export function resolveCustomMenuModalTitle(item: CustomMenuItem, locale: string): string {
  return resolveCustomMenuText(item.modal_title_i18n, locale, item.modal_title || resolveCustomMenuLabel(item, locale))
}

export function resolveCustomMenuModalContent(item: CustomMenuItem, locale: string): string {
  return resolveCustomMenuText(item.modal_content_i18n, locale, item.modal_content || '')
}

export function normalizeCustomMenuLocale(locale: string): CustomMenuLocale {
  return locale === 'zh' ? 'zh' : 'en'
}
