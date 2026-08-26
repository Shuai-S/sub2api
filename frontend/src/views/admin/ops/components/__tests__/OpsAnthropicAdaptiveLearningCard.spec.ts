import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsAnthropicAdaptiveLearningCard from '../OpsAnthropicAdaptiveLearningCard.vue'

const mockGetAnthropicAdaptiveLearning = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getAnthropicAdaptiveLearning: (...args: any[]) => mockGetAnthropicAdaptiveLearning(...args),
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
  generated_at: '2026-07-23T00:00:00Z',
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
    weight_reliability: 0.5,
    weight_capacity: 0.2,
    weight_ttft: 0.15,
    weight_cost: 0.15,
    consecutive_failure_penalty: 0.1,
    success_ema_alpha: 0.1,
    ttft_ema_alpha: 0.1,
    health_failure_threshold: 3,
    learning_window_seconds: 1200,
    learning_min_health_samples: 30,
    cooldown_seconds: 60,
    cooldown_max_seconds: 600,
    high_error_min_samples: 10,
    high_error_max_samples: 30,
    high_error_enter_rate: 0.25,
    high_error_exit_rate: 0.1,
    capacity_shrink_factor: 0.85,
    capacity_growth_factor: 1.15,
    capacity_recovery_samples: 30,
    capacity_recovery_load: 0.8,
    quota_probe_interval_seconds: 60,
  },
  summary: {
    tracked_accounts: 1,
    unlearned_accounts: 0,
    learning_accounts: 1,
    healthy_accounts: 0,
    high_error_accounts: 0,
    cooldown_accounts: 0,
    saturated_accounts: 0,
    unavailable_accounts: 0,
    learned_accounts: 0,
    not_applicable_accounts: 0,
    half_open_accounts: 0,
    quota_limited_accounts: 0,
  },
  accounts: [
    {
      account_id: 7,
      account_name: 'Claude primary',
      platform: 'anthropic',
      type: 'apikey',
      account_status: 'active',
      schedulable: true,
      configured_concurrency: 10,
      effective_capacity: 8,
      rate_multiplier: 0.8,
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
      cost_score: 0.5,
      success_ema: 0.95,
      ttft_ema: 180,
      ttft_samples: 12,
      total_samples: 12,
      consecutive_failure: 0,
      last_success_at: '2026-07-23T00:00:00Z',
      cooldown_remaining_sec: 0,
      circuit_open_count: 0,
      capacity_recovery_successes: 0,
      quota_limited: false,
    },
  ],
}

function mountCard(props: Record<string, any> = {}) {
  return mount(OpsAnthropicAdaptiveLearningCard, {
    props: { refreshToken: 0, ...props },
    global: {
      stubs: {
        Select: SelectStub,
        EmptyState: EmptyStateStub,
        RouterLink: defineComponent({ template: '<a><slot /></a>' }),
      },
    },
  })
}

describe('OpsAnthropicAdaptiveLearningCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads Anthropic learning data with group and learning-state filters', async () => {
    mockGetAnthropicAdaptiveLearning.mockResolvedValue(sampleResponse)

    const wrapper = mountCard({ platformFilter: 'anthropic', groupIdFilter: 7 })
    await flushPromises()

    expect(mockGetAnthropicAdaptiveLearning).toHaveBeenCalledWith(
      expect.objectContaining({ group_id: 7, top_n: 20 })
    )

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[0].vm.$emit('update:modelValue', 'learned')
    await flushPromises()

    expect(mockGetAnthropicAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ learning_status: 'learned' })
    )
  })

  it('supports pagination and renders Anthropic-specific learning fields', async () => {
    mockGetAnthropicAdaptiveLearning.mockImplementation(async (params: Record<string, any>) => ({
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

    expect(mockGetAnthropicAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, page_size: 20 })
    )
    expect(wrapper.text()).toContain('Claude primary')
    expect(wrapper.text()).toContain('admin.ops.anthropicAdaptiveLearning.rateMultiplier {"value":"0.80"}')
    expect(wrapper.text()).toContain('admin.ops.anthropicAdaptiveLearning.table.error')
    expect(wrapper.text()).toContain('5.0%')
    expect(wrapper.text()).toContain('180ms')
    expect(wrapper.text()).toContain('admin.ops.anthropicAdaptiveLearning.ttftSamples {"count":"12"}')
    expect(wrapper.text()).toContain('R 95 / C 63 / T 70 / $ 50')

    const errorButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.ops.anthropicAdaptiveLearning.table.error')
    )
    expect(errorButton).toBeDefined()
    await errorButton?.trigger('click')
    await flushPromises()

    expect(mockGetAnthropicAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort_by: 'error', sort_order: 'desc' })
    )
  })

  it('does not request data when another platform is selected', async () => {
    mockGetAnthropicAdaptiveLearning.mockResolvedValue(sampleResponse)

    const wrapper = mountCard({ platformFilter: 'openai' })
    await flushPromises()

    expect(mockGetAnthropicAdaptiveLearning).not.toHaveBeenCalled()
    expect(wrapper.html()).toBe('<!--v-if-->')
  })

  it('shows the empty state for an empty response', async () => {
    mockGetAnthropicAdaptiveLearning.mockResolvedValue({
      ...sampleResponse,
      total_accounts: 0,
      total: 0,
      returned_accounts: 0,
      accounts: [],
    })

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.find('.empty-state').exists()).toBe(true)
  })
})
