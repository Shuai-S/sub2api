import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import CustomMenuModal from '../CustomMenuModal.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('CustomMenuModal', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    document.body.innerHTML = ''
    document.body.style.overflow = ''
  })

  it('renders a sanitized Markdown preview and hardens external links', async () => {
    const wrapper = mount(CustomMenuModal, {
      props: {
        previewItem: {
          id: 'notice',
          label: 'Button label',
          icon_svg: '<svg><path d="M1 1h2" /></svg>',
          modal_title: 'Preview title',
          modal_content: '# Heading\n\n[Docs](https://example.com)<script>window.__xss = true</script>',
        },
      },
    })
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('Preview title')
    expect(document.body.querySelector('.markdown-body h1')?.textContent).toBe('Heading')
    expect(document.body.querySelector('.markdown-body script')).toBeNull()
    const link = document.body.querySelector<HTMLAnchorElement>('.markdown-body a')
    expect(link?.target).toBe('_blank')
    expect(link?.rel).toBe('noopener noreferrer')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not close when the backdrop is clicked', async () => {
    const wrapper = mount(CustomMenuModal, {
      props: {
        previewItem: {
          id: 'notice',
          label: 'Notice',
          icon_svg: '',
          modal_content: 'Body',
        },
      },
    })
    const backdrop = document.body.querySelector<HTMLElement>('[role="dialog"]')
    backdrop?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps keyboard focus inside the dialog', async () => {
    const outsideButton = document.createElement('button')
    document.body.appendChild(outsideButton)
    const wrapper = mount(CustomMenuModal, {
      props: {
        previewItem: {
          id: 'notice',
          label: 'Notice',
          icon_svg: '',
          modal_content: '[Docs](https://example.com)',
        },
      },
    })
    await wrapper.vm.$nextTick()

    const dialog = document.body.querySelector<HTMLElement>('[role="dialog"] > div')
    const focusable = dialog?.querySelectorAll<HTMLElement>('a[href], button:not([disabled])')
    const first = focusable?.[0]
    const last = focusable?.[focusable.length - 1]

    last?.focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    expect(document.activeElement).toBe(first)

    first?.focus()
    document.dispatchEvent(new KeyboardEvent('keydown', {
      key: 'Tab',
      shiftKey: true,
      bubbles: true,
    }))
    expect(document.activeElement).toBe(last)
    expect(document.activeElement).not.toBe(outsideButton)
    wrapper.unmount()
  })
})
