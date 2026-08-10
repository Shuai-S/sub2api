import { defineComponent, h, onMounted } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CaptchaLaWidget from '../CaptchaLaWidget.vue'

const issueChallenge = vi.fn()
const sdkVerify = vi.fn()
const sdkDestroy = vi.fn()

vi.mock('@/api/auth', () => ({
  issueCaptchaLaChallenge: (...args: unknown[]) => issueChallenge(...args)
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale: { value: 'zh-CN' } })
}))

vi.mock('@captcha-la/vue', () => ({
  Captchala: defineComponent({
    props: {
      appKey: String,
      serverToken: String,
      action: String,
      lang: String,
      product: String
    },
    emits: ['ready', 'success', 'error', 'close'],
    setup(_, { emit, expose }) {
      expose({ verify: sdkVerify, destroy: sdkDestroy })
      onMounted(() => emit('ready'))
      return () =>
        h('div', [
          h(
            'button',
            {
              'data-testid': 'success',
              onClick: () => emit('success', { token: 'pt_test', action: 'login', type: 'click' })
            },
            'success'
          ),
          h(
            'button',
            { 'data-testid': 'close', onClick: () => emit('close') },
            'close'
          )
        ])
    }
  })
}))

describe('CaptchaLaWidget', () => {
  beforeEach(() => {
    issueChallenge.mockReset()
    sdkVerify.mockReset()
    sdkDestroy.mockReset()
    issueChallenge.mockResolvedValue({ server_token: 'sct_test', expires_in: 300 })
  })

  it('issues a server token before opening the SDK and resolves a pt_ proof', async () => {
    const wrapper = mount(CaptchaLaWidget, {
      props: { appKey: 'pk_test', action: 'login' }
    })
    const vm = wrapper.vm as unknown as { verify: () => Promise<string | null> }

    const proof = vm.verify()
    await flushPromises()

    expect(issueChallenge).toHaveBeenCalledWith('login')
    expect(sdkVerify).toHaveBeenCalledOnce()
    await wrapper.get('[data-testid="success"]').trigger('click')
    await expect(proof).resolves.toBe('pt_test')
    expect(wrapper.emitted('verify')).toEqual([['pt_test']])
  })

  it('deduplicates concurrent verification requests', async () => {
    const wrapper = mount(CaptchaLaWidget, {
      props: { appKey: 'pk_test', action: 'login' }
    })
    const vm = wrapper.vm as unknown as { verify: () => Promise<string | null> }

    const first = vm.verify()
    const second = vm.verify()
    await flushPromises()

    expect(issueChallenge).toHaveBeenCalledOnce()
    await wrapper.get('[data-testid="success"]').trigger('click')
    await expect(Promise.all([first, second])).resolves.toEqual(['pt_test', 'pt_test'])
  })

  it('resolves null when the user closes the challenge', async () => {
    const wrapper = mount(CaptchaLaWidget, {
      props: { appKey: 'pk_test', action: 'login' }
    })
    const vm = wrapper.vm as unknown as { verify: () => Promise<string | null> }

    const proof = vm.verify()
    await flushPromises()
    await wrapper.get('[data-testid="close"]').trigger('click')

    await expect(proof).resolves.toBeNull()
  })
})
