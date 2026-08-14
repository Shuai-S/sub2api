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
      t: (key: string, params?: Record<string, unknown>) => params ? `${key} ${JSON.stringify(params)}` : key,
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
  total_accounts: 1,
  total: 1,
  returned_accounts: 1,
  limit: 20,
  top_n: 20,
  sort_by: 'status',
  sort_order: 'desc',
  settings: {
    top_k: 8,
    softmax_temperature: 0.35,
    exploration_rate: 0.02,
    consecutive_failure_penalty: 0.1,
    weight_reliability: 0.5,
    weight_capacity: 0.2,
    weight_ttft: 0.15,
    weight_cost: 0.15,
    learning_window_seconds: 1200,
    learning_min_health_samples: 30,
    success_ema_alpha: 0.1,
    ttft_ema_alpha: 0.1,
    cooldown_seconds: 60,
    cooldown_max_seconds: 600,
    account_failure_threshold: 3,
    high_error_min_samples: 10,
    high_error_max_samples: 30,
    high_error_enter_rate: 0.25,
    high_error_exit_rate: 0.1,
    capacity_shrink_factor: 0.85,
    capacity_growth_factor: 1.15,
    capacity_recovery_samples: 30,
    capacity_recovery_load: 0.8,
    quota_probe_interval_seconds: 60,
    diagnostic_log_enabled: false,
    diagnostic_log_sample_rate: 0.01,
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
    unavailable_accounts: 0,
    quota_limited_accounts: 0,
    cooldown_accounts: 0,
    high_error_accounts: 0,
    saturated_accounts: 0,
    learning_accounts: 1,
    unlearned_accounts: 0,
    healthy_accounts: 0,
    learned_accounts: 0,
    not_applicable_accounts: 0,
    half_open_accounts: 0,
  },
  accounts: [
    {
      account_id: 7,
      account_name: 'Gemini primary',
      platform: 'gemini',
      type: 'apikey',
      account_status: 'active',
      schedulable: true,
      configured_concurrency: 10,
      effective_capacity: 8,
      rate_multiplier: 1.25,
      current_concurrency: 3,
      waiting_count: 0,
      load_percentage: 37.5,
      scheduler_status: 'learning',
      learned: true,
      learning_status: 'learning',
      runtime_status: 'healthy',
      runtime_flags: ['healthy'],
      health_samples: 12,
      capacity_generation: 0,
      capacity_half_open: false,
      scheduler_score: 0.78,
      reliability_score: 0.95,
      capacity_score: 0.625,
      latency_score: 0.7,
      cost_score: 0.4,
      path_success_ema: 0.95,
      ttft_ema: 180,
      ttft_samples: 12,
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
      consecutive_failure: 0,
      last_success_at: '2026-07-28T00:00:00Z',
      cooldown_remaining_sec: 0,
      circuit_open_count: 0,
      capacity_recovery_successes: 0,
      quota_limited: false,
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
      expect.objectContaining({ group_id: 7, top_n: 20 })
    )
    expect(wrapper.text()).toContain('Gemini primary')
    expect(wrapper.text()).toContain('admin.ops.geminiAdaptiveLearning.rateMultiplier {"value":"1.25"}')
    expect(wrapper.text()).toContain('R 95 / C 63 / T 70 / $ 40')
    expect(wrapper.text()).toContain('10/100 (10%)')

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[1].vm.$emit('update:modelValue', 'quota_limited')
    await flushPromises()

    expect(mockGetGeminiAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ runtime_status: 'quota_limited' })
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
    await selects[2].vm.$emit('update:modelValue', 'pagination')
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
