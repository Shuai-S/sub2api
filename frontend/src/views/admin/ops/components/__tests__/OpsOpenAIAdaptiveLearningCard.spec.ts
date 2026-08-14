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
    diagnostic_log_enabled: false,
    diagnostic_log_sample_rate: 0.01,
    top_k: 8,
    exploration_rate: 0.02,
    softmax_temperature: 0.35,
    consecutive_failure_penalty: 0.1,
    learning_window_seconds: 1200,
    learning_min_health_samples: 30,
    success_ema_alpha: 0.1,
    ttft_ema_alpha: 0.1,
    health_failure_threshold: 3,
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
    weight_reliability: 0.5,
    weight_capacity: 0.2,
    weight_ttft: 0.15,
    weight_cost: 0.15,
  },
  summary: {
    tracked_accounts: 1,
    unlearned_accounts: 0,
    learning_accounts: 0,
    healthy_accounts: 0,
    high_error_accounts: 1,
    cooldown_accounts: 1,
    half_open_accounts: 1,
    quota_limited_accounts: 1,
    saturated_accounts: 1,
    unavailable_accounts: 0,
    learned_accounts: 1,
    not_applicable_accounts: 0,
  },
  accounts: [{
    account_id: 7,
    account_name: 'OpenAI balance probe',
    platform: 'openai',
    type: 'apikey',
    account_status: 'active',
    schedulable: true,
    configured_concurrency: 100,
    effective_capacity: 3,
    rate_multiplier: 1,
    current_concurrency: 1,
    waiting_count: 0,
    load_percentage: 33.3,
    scheduler_status: 'quota_limited',
    status_reason: 'quota exhausted',
    learned: true,
    learning_status: 'learned',
    runtime_status: 'quota_limited',
    runtime_flags: ['quota_limited'],
    runtime_reason_code: 'quota_limited',
    runtime_reason: 'quota exhausted',
    health_samples: 30,
    capacity_generation: 2,
    capacity_half_open: false,
    scheduler_score: 0.5,
    success_score: 0.9,
    cost_score: 0.5,
    capacity_score: 0.67,
    latency_score: 0.5,
    success_ema: 0.9,
    ttft_ema: 200,
    ttft_samples: 12,
    total_samples: 30,
    consecutive_failure: 0,
    cooldown_remaining_sec: 0,
    circuit_open_count: 0,
    capacity_recovery_successes: 0,
    quota_limited: true,
    quota_reset_at: '2026-08-01T01:00:00Z',
    quota_next_probe_at: '2026-08-01T00:01:00Z',
  }],
}

describe('OpsOpenAIAdaptiveLearningCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders quota limits separately from account failure streaks', async () => {
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

    expect(wrapper.text()).toContain('admin.ops.openaiAdaptiveLearning.status.quotaLimited')
    expect(wrapper.text()).toContain('quota exhausted')
    expect(wrapper.text()).not.toContain('admin.ops.openaiAdaptiveLearning.consecutiveFailures')

    const riskCard = wrapper.findAll('.grid > div').find((node) =>
      node.text().includes('admin.ops.openaiAdaptiveLearning.summary.risk')
    )
    expect(riskCard?.text()).toContain('5')
  })
})
