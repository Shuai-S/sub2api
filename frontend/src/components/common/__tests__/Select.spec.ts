import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const originalInnerWidth = window.innerWidth
let unmountWrapper: (() => void) | undefined

const setViewportWidth = (width: number) => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
}

const mockTriggerRect = (left: number, width: number) => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: left,
    y: 20,
    top: 20,
    right: left + width,
    bottom: 60,
    left,
    width,
    height: 40,
    toJSON: () => ({}),
  })
}

const openSelect = async () => {
  const wrapper = mount(Select, {
    props: {
      modelValue: null,
      options: [
        {
          value: 'example',
          label: 'very-long-unbroken-option-value-that-must-not-overflow',
        },
      ],
    },
  })
  unmountWrapper = () => wrapper.unmount()

  await wrapper.get('button').trigger('click')
  await nextTick()

  return document.body.querySelector<HTMLElement>('.select-dropdown-portal')
}

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  setViewportWidth(originalInnerWidth)
  vi.restoreAllMocks()
})

describe('Select dropdown viewport constraints', () => {
  it('uses the preferred readable width when space is available', async () => {
    setViewportWidth(1024)
    mockTriggerRect(20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('20px')
    expect(dropdown?.style.width).toBe('360px')
    expect(dropdown?.style.maxWidth).toBe('calc(100vw - 24px)')
  })

  it('shifts and shrinks the dropdown to fit near the right viewport edge', async () => {
    setViewportWidth(320)
    mockTriggerRect(220, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('12px')
    expect(dropdown?.style.width).toBe('296px')
    expect(dropdown?.style.maxWidth).toBe('calc(100vw - 24px)')
  })

  it('clamps a trigger left of the viewport to the safe padding', async () => {
    setViewportWidth(320)
    mockTriggerRect(-20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('12px')
    expect(dropdown?.style.width).toBe('296px')
    expect(dropdown?.style.maxWidth).toBe('calc(100vw - 24px)')
  })

  it('clamps an offscreen-right trigger while keeping the dropdown visible', async () => {
    setViewportWidth(320)
    mockTriggerRect(400, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('12px')
    expect(dropdown?.style.width).toBe('296px')
    expect(dropdown?.style.maxWidth).toBe('calc(100vw - 24px)')
  })
})
