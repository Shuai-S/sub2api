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
  generated_at: '2026-07-23T00:00:00Z',
  requested_model: 'sonnet',
  model_family: 'sonnet',
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
    weight_reliability: 0.5,
    weight_capacity: 0.3,
    weight_latency: 0.15,
    weight_exploration: 0.05,
    initial_reliability: 0.5,
    consecutive_failure_penalty: 0.25,
    neutral_latency_score: 0.5,
    success_ema_alpha: 0.05,
    latency_ema_alpha: 0.05,
    capacity_success_threshold: 0.97,
    capacity_probe_load_threshold: 0.8,
    capacity_failure_threshold: 3,
    min_recent_samples_for_shrink: 30,
    shrink_error_threshold: 0.25,
    learning_window_seconds: 1200,
    cooldown_seconds: 60,
    shrink_factor_soft: 0.85,
    shrink_factor_hard: 0.6,
    capacity_increase_step: 1,
    min_capacity: 1,
    hard_shrink_failure_multiplier: 2,
  },
  summary: {
    tracked_accounts: 1,
    disabled_accounts: 0,
    unlearned_accounts: 0,
    learning_accounts: 1,
    healthy_accounts: 0,
    high_error_accounts: 0,
    cooldown_accounts: 0,
    saturated_accounts: 0,
    unavailable_accounts: 0,
  },
  accounts: [
    {
      account_id: 7,
      account_name: 'Claude primary',
      platform: 'anthropic',
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
      capacity_score: 0.625,
      latency_score: 0.7,
      exploration_score: 0.2,
      success_ema: 0.95,
      model_family: 'sonnet',
      ttft_ema: 180,
      latency_ema: 950,
      latency_samples: 12,
      latency_by_model_family: [
        { model_family: 'sonnet', ttft_ema: 180, latency_ema: 950, samples: 12 },
      ],
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
      last_success_at: '2026-07-23T00:00:00Z',
      cooldown_remaining_sec: 0,
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

  it('loads Anthropic learning data with group and model-family filters', async () => {
    mockGetAnthropicAdaptiveLearning.mockResolvedValue(sampleResponse)

    const wrapper = mountCard({ platformFilter: 'anthropic', groupIdFilter: 7 })
    await flushPromises()

    expect(mockGetAnthropicAdaptiveLearning).toHaveBeenCalledWith(
      expect.objectContaining({ group_id: 7, model: 'sonnet', top_n: 20 })
    )

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[0].vm.$emit('update:modelValue', 'opus')
    await flushPromises()

    expect(mockGetAnthropicAdaptiveLearning).toHaveBeenLastCalledWith(
      expect.objectContaining({ model: 'opus' })
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
    expect(wrapper.text()).toContain('180ms')
    expect(wrapper.text()).toContain('R 95 / C 63 / L 70 / E 20')
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
