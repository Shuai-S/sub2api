import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsGeminiAdaptiveLearningCard from '../OpsGeminiAdaptiveLearningCard.vue'

const mockGetGeminiAdaptiveLearning = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getGeminiAdaptiveLearning: (...args: any[]) => mockGetGeminiAdaptiveLearning(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>',
})

const sampleResponse = {
  enabled: true,
  mode: 'shadow',
  realtime_enabled: true,
  generated_at: '2026-07-28T00:00:00Z',
  requested_model: 'gemini-2.5-pro',
  model_family: 'pro',
  total_accounts: 1,
  total: 1,
  returned_accounts: 1,
  limit: 20,
  top_n: 20,
  sort_by: 'status',
  sort_order: 'desc',
  settings: {
    sticky_escape_on_capacity_full: false,
    top_k: 8,
    softmax_temperature: 0.35,
    weight_reliability: 0.3,
    weight_quota: 0.25,
    weight_capacity: 0.2,
    weight_latency: 0.15,
    weight_cost: 0.05,
    weight_exploration: 0.05,
    learning_window_seconds: 1200,
  },
  metrics: {
    select_total: 12,
    shadow_diverge_total: 2,
    fallback_total: 1,
    sticky_hit_total: 8,
    sticky_migrate_total: 3,
    capacity_decrease_total: 1,
    quota_snapshot_error_total: 0,
  },
  summary: {
    tracked_accounts: 1,
    disabled_accounts: 0,
    unavailable_accounts: 0,
    quota_limited_accounts: 0,
    cooldown_accounts: 0,
    high_error_accounts: 0,
    saturated_accounts: 0,
    learning_accounts: 1,
    unlearned_accounts: 0,
    healthy_accounts: 0,
  },
  accounts: [
    {
      account_id: 7,
      account_name: 'Gemini primary',
      platform: 'gemini',
      type: 'oauth',
      account_status: 'active',
      schedulable: true,
      priority: 1,
      configured_concurrency: 10,
      estimated_capacity: 8,
      effective_capacity: 8,
      current_concurrency: 3,
      waiting_count: 0,
      load_percentage: 37.5,
      scheduler_status: 'learning',
      learned: true,
      scheduler_score: 0.78,
      reliability_score: 0.95,
      quota_score: 0.8,
      capacity_score: 0.625,
      latency_score: 0.7,
      cost_score: 0.4,
      exploration_score: 0.2,
      path_success_ema: 0.95,
      model_family: 'pro',
      model_success_ema: 0.92,
      ttft_ema: 180,
      latency_ema: 950,
      model_samples: 12,
      model_failures: 1,
      by_model_family: [],
      quota: {
        scope: { daily: 'pro', minute: 'shared' },
        daily_used: 10,
        daily_limit: 100,
        daily_reset_at: '2026-07-29T00:00:00Z',
        minute_used: 4,
        minute_limit: 20,
        minute_reset_at: '2026-07-28T00:01:00Z',
        hard_rejected: false,
        data_available: true,
      },
      total_samples: 12,
      recent_health_samples: 12,
      recent_health_failures: 1,
      recent_health_failure_rate: 1 / 12,
      recent_capacity_samples: 8,
      recent_capacity_failures: 1,
      recent_capacity_failure_rate: 0.125,
      consecutive_success: 3,
      consecutive_failure: 0,
      consecutive_capacity_failure: 0,
      last_success_at: '2026-07-28T00:00:00Z',
      cooldown_remaining_sec: 0,
    },
  ],
}

function mountCard(props: Record<string, any> = {}) {
  return mount(OpsGeminiAdaptiveLearningCard, {
    props: { refreshToken: 0, ...props },
    global: {
      stubs: {
        Select: SelectStub,
        EmptyState: EmptyStateStub,
        Icon: true,
        RouterLink: defineComponent({ template: '<a><slot /></a>' }),
      },
    },
  })
}

describe('OpsGeminiAdaptiveLearningCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads and filters Gemini learning data with quota fields', async () => {
    mockGetGeminiAdaptiveLearning.mockResolvedValue(sampleResponse)

    const wrapper = mountCard({ platformFilter: 'gemini', groupIdFilter: 7 })
    await flushPromises()

    expect(mockGetGeminiAdaptiveLearning).toHaveBeenCalledWith(
      expect.objectContaining({ group_id: 7, model: 'gemini-2.5-pro', top_n: 20 })
    )
    expect(wrapper.text()).toContain('Gemini primary')
    expect(wrapper.text()).toContain('R 95 / Q 80 / C 63')
    expect(wrapper.text()).toContain('10/100 (10%)')

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[0].vm.$emit('update:modelValue', 'quota_limited')
    await flushPromises()

    expect(mockGetGeminiAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: 'quota_limited' })
    )
  })

  it('switches to pagination and requests the next page', async () => {
    mockGetGeminiAdaptiveLearning.mockImplementation(async (params: Record<string, any>) => ({
      ...sampleResponse,
      total: 40,
      page: params.page ?? 1,
      page_size: params.page_size ?? 20,
      top_n: params.top_n,
    }))

    const wrapper = mountCard()
    await flushPromises()

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[1].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()

    expect(mockGetGeminiAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, page_size: 20 })
    )
    const nextButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.ops.openaiTokenStats.nextPage'))
    expect(nextButton).toBeDefined()
    await nextButton?.trigger('click')
    await flushPromises()

    expect(mockGetGeminiAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, page_size: 20 })
    )
  })

  it('shows the request error and clears stale response data', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    mockGetGeminiAdaptiveLearning.mockRejectedValue(new Error('network unavailable'))

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.text()).toContain('network unavailable')
    expect(wrapper.text()).not.toContain('Gemini primary')
    consoleError.mockRestore()
  })

  it('does not request data when another platform is selected', async () => {
    mockGetGeminiAdaptiveLearning.mockResolvedValue(sampleResponse)

    const wrapper = mountCard({ platformFilter: 'openai' })
    await flushPromises()

    expect(mockGetGeminiAdaptiveLearning).not.toHaveBeenCalled()
    expect(wrapper.html()).toBe('<!--v-if-->')
  })
})
