import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('positions fixed tooltip with viewport coordinates while the page is scrolled', async () => {
    const originalScrollX = window.scrollX
    const originalScrollY = window.scrollY
    const originalInnerWidth = window.innerWidth

    Object.defineProperty(window, 'scrollX', { configurable: true, value: 300 })
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 2400 })
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })

    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'scrolled details',
      },
    })

    try {
      const trigger = wrapper.get('.group')
      const triggerEl = trigger.element as HTMLElement
      triggerEl.getBoundingClientRect = () => ({
        x: 300,
        y: 160,
        top: 160,
        left: 300,
        right: 320,
        bottom: 180,
        width: 20,
        height: 20,
        toJSON: () => ({}),
      } as DOMRect)

      const tooltip = getTooltipElement()
      tooltip.getBoundingClientRect = () => ({
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        right: 256,
        bottom: 80,
        width: 256,
        height: 80,
        toJSON: () => ({}),
      } as DOMRect)

      await trigger.trigger('mouseenter')
      await nextTick()
      await nextTick()

      expect(tooltip.style.top).toBe('calc(152px)')
      expect(tooltip.style.left).toBe('310px')
    } finally {
      wrapper.unmount()
      Object.defineProperty(window, 'scrollX', { configurable: true, value: originalScrollX })
      Object.defineProperty(window, 'scrollY', { configurable: true, value: originalScrollY })
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalInnerWidth })
    }
  })
})
