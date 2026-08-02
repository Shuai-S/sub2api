import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../SettingsView.vue'),
  'utf8',
)

describe('custom menu settings', () => {
  it('separates sidebar and header configuration while preserving one data source', () => {
    expect(source).toContain('sidebarMenuEntries')
    expect(source).toContain('headerMenuEntries')
    expect(source).toContain("addMenuItem('sidebar')")
    expect(source).toContain("addMenuItem('header')")
  })

  it('configures external opening and unsaved modal previews', () => {
    expect(source).toContain('v-model="entry.item.open_mode"')
    expect(source).toContain('value="new_tab"')
    expect(source).toContain(':preview-item="previewMenuItem"')
    expect(source).toContain('v-model="entry.item.modal_content"')
  })
})
