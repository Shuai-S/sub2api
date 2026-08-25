import { readFileSync, readdirSync } from 'node:fs'
import { extname, join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

type LocaleMessages = Record<string, unknown>

const sourceRoot = join(process.cwd(), 'src')
const sourceExtensions = new Set(['.ts', '.vue'])
const translationCallPattern = /(?<![\w$])(?:\$t|t|i18n(?:\.global)?\.t)\(\s*(['"`])([^'"`$]+)\1(?=\s*[,\)])/g

function flattenKeys(node: unknown, prefix = ''): string[] {
  if (!node || typeof node !== 'object' || Array.isArray(node)) {
    return prefix ? [prefix] : []
  }

  return Object.entries(node as LocaleMessages).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key
    return value && typeof value === 'object' && !Array.isArray(value)
      ? flattenKeys(value, path)
      : [path]
  })
}

function collectSourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || path.includes(`${join('src', 'i18n', 'locales')}`)) {
        return []
      }
      return collectSourceFiles(path)
    }
    return sourceExtensions.has(extname(entry.name)) ? [path] : []
  })
}

function collectLiteralTranslationCalls(): Map<string, string[]> {
  const calls = new Map<string, string[]>()

  for (const file of collectSourceFiles(sourceRoot)) {
    const source = readFileSync(file, 'utf8')
    for (const match of source.matchAll(translationCallPattern)) {
      const key = match[2]
      const line = source.slice(0, match.index).split('\n').length
      const locations = calls.get(key) ?? []
      locations.push(`${relative(sourceRoot, file)}:${line}`)
      calls.set(key, locations)
    }
  }

  return calls
}

function collectIndirectLocaleReferences(localeRoots: Set<string>): Map<string, string[]> {
  const references = new Map<string, string[]>()
  const localeReferencePattern = /\b(?:key|titleKey|descriptionKey|labelKey|messageKey|hintKey|errorKey|successKey|fallbackKey|i18nKey|translationKey)\s*:\s*(['"`])([^'"`$]+)\1/g

  for (const file of collectSourceFiles(sourceRoot)) {
    const source = readFileSync(file, 'utf8')
    for (const match of source.matchAll(localeReferencePattern)) {
      const key = match[2]
      if (!key.includes('.') || !localeRoots.has(key.split('.')[0])) continue

      const line = source.slice(0, match.index).split('\n').length
      const locations = references.get(key) ?? []
      locations.push(`${relative(sourceRoot, file)}:${line}`)
      references.set(key, locations)
    }
  }

  return references
}

describe('locale key completeness', () => {
  const enKeys = new Set(flattenKeys(en))
  const zhKeys = new Set(flattenKeys(zh))

  it('keeps the English and Chinese locale key sets in sync', () => {
    expect({
      missingInEnglish: [...zhKeys].filter((key) => !enKeys.has(key)).sort(),
      missingInChinese: [...enKeys].filter((key) => !zhKeys.has(key)).sort()
    }).toEqual({ missingInEnglish: [], missingInChinese: [] })
  })

  it('defines every literal translation key used by application source', () => {
    const missing = [...collectLiteralTranslationCalls()]
      .flatMap(([key, locations]) => {
        const locales = [
          !enKeys.has(key) ? 'en' : '',
          !zhKeys.has(key) ? 'zh' : ''
        ].filter(Boolean)
        return locales.length > 0
          ? [`${key} (missing in ${locales.join(', ')}; used at ${locations.join(', ')})`]
          : []
      })
      .sort()

    expect(missing).toEqual([])
  })

  it('defines locale-key literals passed through application metadata and maps', () => {
    const localeRoots = new Set(Object.keys(en))
    const missing = [...collectIndirectLocaleReferences(localeRoots)]
      .flatMap(([key, locations]) => {
        const locales = [
          !enKeys.has(key) ? 'en' : '',
          !zhKeys.has(key) ? 'zh' : ''
        ].filter(Boolean)
        return locales.length > 0
          ? [`${key} (missing in ${locales.join(', ')}; used at ${locations.join(', ')})`]
          : []
      })
      .sort()

    expect(missing).toEqual([])
  })
})
