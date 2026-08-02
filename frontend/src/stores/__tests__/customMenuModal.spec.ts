import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { customMenuAPI } from '@/api'
import { useCustomMenuModalStore } from '@/stores/customMenuModal'
import type { CustomMenuItem } from '@/types'

vi.mock('@/api', () => ({
  customMenuAPI: {
    getModalContent: vi.fn(),
  },
}))

const item: CustomMenuItem = {
  id: 'notice',
  label: 'Notice',
  icon_svg: '',
  url: '',
  visibility: 'user',
  sort_order: 0,
  placement: 'header',
}

describe('useCustomMenuModalStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(customMenuAPI.getModalContent).mockReset()
  })

  it('loads protected content once and reuses the session cache', async () => {
    vi.mocked(customMenuAPI.getModalContent).mockResolvedValue({
      id: 'notice',
      title: 'Notice title',
      content: '# Body',
    })
    const store = useCustomMenuModalStore()

    store.open(item)
    await flushPromises()
    expect(store.content?.title).toBe('Notice title')

    store.close()
    store.open(item)
    await flushPromises()
    expect(customMenuAPI.getModalContent).toHaveBeenCalledTimes(1)
    expect(store.content?.content).toBe('# Body')
  })

  it('exposes an error state and retries on demand', async () => {
    vi.mocked(customMenuAPI.getModalContent)
      .mockRejectedValueOnce(new Error('failed'))
      .mockResolvedValueOnce({ id: 'notice', title: 'Recovered', content: 'OK' })
    const store = useCustomMenuModalStore()

    store.open(item)
    await flushPromises()
    expect(store.error).toBe(true)

    store.retry()
    await flushPromises()
    expect(store.error).toBe(false)
    expect(store.content?.title).toBe('Recovered')
  })
})
