import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsOpenAIAdaptiveLearningCard from '../OpsOpenAIAdaptiveLearningCard.vue'

const mockGetOpenAIAdaptiveLearning = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getOpenAIAdaptiveLearning: (...args: any[]) => mockGetOpenAIAdaptiveLearning(...args),
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
  props: { modelValue: { type: [String, Number], default: '' } },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const sampleResponse = {
  enabled: true,
  mode: 'enforce',
  realtime_enabled: true,
  generated_at: '2026-08-01T00:00:00Z',
  total_accounts: 1,
  total: 1,
  returned_accounts: 1,
  limit: 20,
  top_n: 20,
  settings: {
    top_k: 10,
    account_type_priority_mode: 'oauth_first',
    exploration_rate: 0.05,
    softmax_temperature: 0.35,
    initial_capacity_fraction: 0.1,
    min_capacity: 1,
    capacity_growth_factor: 1.1,
    burst_probe_ratio: 0.3,
    capacity_failure_threshold: 3,
    min_recent_samples_for_shrink: 50,
    shrink_error_threshold: 0.3,
    shrink_factor_soft: 0.8,
    shrink_factor_hard: 0.5,
    half_open_failure_threshold: 3,
    half_open_probe_capacity: 3,
    learning_window_seconds: 1200,
  },
  summary: {
    tracked_accounts: 1,
    unlearned_accounts: 0,
    learning_accounts: 0,
    healthy_accounts: 0,
    high_error_accounts: 1,
    cooldown_accounts: 1,
    half_open_accounts: 1,
    insufficient_balance_accounts: 1,
    saturated_accounts: 1,
    unavailable_accounts: 0,
  },
  accounts: [{
    account_id: 7,
    account_name: 'OpenAI balance probe',
    platform: 'openai',
    type: 'apikey',
    account_status: 'active',
    schedulable: true,
    priority: 1,
    configured_concurrency: 100,
    stable_capacity: 80,
    effective_capacity: 3,
    burst_capacity: 0,
    rate_multiplier: 1,
    current_concurrency: 1,
    waiting_count: 0,
    load_percentage: 33.3,
    scheduler_status: 'insufficient_balance',
    status_reason: 'probing account after insufficient balance',
    learned: true,
    scheduler_score: 0.5,
    success_score: 0.9,
    cost_score: 0.5,
    capacity_score: 0.67,
    latency_score: 0.5,
    stability_score: 0.9,
    exploration_score: 0.1,
    success_ema: 0.9,
    error_ema: 0.1,
    latency_ema: 1000,
    ttft_ema: 200,
    total_samples: 10,
    recent_samples: 5,
    recent_failures: 0,
    recent_failure_rate: 0,
    consecutive_success: 0,
    consecutive_failure: 9,
    consecutive_capacity_failure: 0,
    cooldown_remaining_sec: 0,
    balance_insufficient_at: '2026-08-01T00:00:00Z',
    last_balance_probe_at: '2026-08-01T00:01:00Z',
    balance_generation: 2,
  }],
}

describe('OpsOpenAIAdaptiveLearningCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders insufficient balance as a probe state without a failure streak', async () => {
    mockGetOpenAIAdaptiveLearning.mockResolvedValue(sampleResponse)
    const wrapper = mount(OpsOpenAIAdaptiveLearningCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: true,
          RouterLink: defineComponent({ template: '<a><slot /></a>' }),
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.ops.openaiAdaptiveLearning.status.insufficientBalance')
    expect(wrapper.text()).toContain('admin.ops.openaiAdaptiveLearning.balanceProbeAt')
    expect(wrapper.text()).not.toContain('admin.ops.openaiAdaptiveLearning.consecutiveFailures')

    const riskCard = wrapper.findAll('.grid > div').find((node) =>
      node.text().includes('admin.ops.openaiAdaptiveLearning.summary.risk')
    )
    expect(riskCard?.text()).toContain('5')
  })
})
