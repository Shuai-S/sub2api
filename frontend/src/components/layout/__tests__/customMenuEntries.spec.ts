import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(dir, '../AppHeader.vue'), 'utf8')
const sidebarSource = readFileSync(resolve(dir, '../AppSidebar.vue'), 'utf8')
const customPageSource = readFileSync(resolve(dir, '../../../views/user/CustomPageView.vue'), 'utf8')

describe('custom menu placements', () => {
  it('renders the custom header region before the built-in announcement entry', () => {
    expect(headerSource.indexOf('data-testid="custom-header-menu"')).toBeGreaterThan(-1)
    expect(headerSource.indexOf('data-testid="custom-header-menu"')).toBeLessThan(
      headerSource.indexOf('<AnnouncementBell'),
    )
    expect(headerSource).toContain("item.placement === 'header'")
  })

  it('keeps legacy sidebar items embedded and supports safe new-tab anchors', () => {
    expect(sidebarSource).toContain("return item.placement || 'sidebar'")
    expect(sidebarSource).toContain("item.open_mode === 'new_tab'")
    expect(sidebarSource).toContain("rel: 'noopener noreferrer'")
    expect(sidebarSource).toContain('v-bind="navLinkProps(item)"')
    expect(sidebarSource).toContain("item.externalUrl ? 'a' : RouterLink")
  })

  it('does not resolve header entries as custom pages', () => {
    expect(customPageSource).toContain("(item.placement || 'sidebar') === 'sidebar'")
  })
})
