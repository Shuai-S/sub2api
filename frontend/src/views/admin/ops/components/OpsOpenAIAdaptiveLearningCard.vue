<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import {
  opsAPI,
  type OpsOpenAIAdaptiveLearningAccount,
  type OpsOpenAIAdaptiveLearningResponse
} from '@/api/admin/ops'
import { formatNumber } from '@/utils/format'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()

const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOpenAIAdaptiveLearningResponse | null>(null)
let loadSeq = 0

const enabledForPlatform = computed(() => {
  const platform = String(props.platformFilter || '').trim().toLowerCase()
  return !platform || platform === 'openai'
})

const accounts = computed(() => response.value?.accounts ?? [])
const summary = computed(() => response.value?.summary ?? null)

const statusKeyMap: Record<string, string> = {
  disabled: 'admin.ops.openaiAdaptiveLearning.status.disabled',
  unavailable: 'admin.ops.openaiAdaptiveLearning.status.unavailable',
  cooldown: 'admin.ops.openaiAdaptiveLearning.status.cooldown',
  half_open: 'admin.ops.openaiAdaptiveLearning.status.halfOpen',
  high_error: 'admin.ops.openaiAdaptiveLearning.status.highError',
  saturated: 'admin.ops.openaiAdaptiveLearning.status.saturated',
  learning: 'admin.ops.openaiAdaptiveLearning.status.learning',
  unlearned: 'admin.ops.openaiAdaptiveLearning.status.unlearned',
  healthy: 'admin.ops.openaiAdaptiveLearning.status.healthy'
}

const modeKeyMap: Record<string, string> = {
  enforce: 'admin.ops.openaiAdaptiveLearning.mode.enforce',
  shadow: 'admin.ops.openaiAdaptiveLearning.mode.shadow'
}

const statusClassMap: Record<string, string> = {
  disabled: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  unavailable: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  cooldown: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  half_open: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  high_error: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  saturated: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
  learning: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  unlearned: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  healthy: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

const summaryItems = computed(() => {
  const s = summary.value
  if (!s || !response.value) return []
  return [
    {
      key: 'tracked',
      label: t('admin.ops.openaiAdaptiveLearning.summary.tracked'),
      value: `${formatNumber(s.tracked_accounts)}/${formatNumber(response.value.total_accounts)}`,
      tone: 'text-gray-900 dark:text-white'
    },
    {
      key: 'healthy',
      label: t('admin.ops.openaiAdaptiveLearning.summary.healthy'),
      value: formatNumber(s.healthy_accounts),
      tone: 'text-green-600 dark:text-green-400'
    },
    {
      key: 'risk',
      label: t('admin.ops.openaiAdaptiveLearning.summary.risk'),
      value: formatNumber(s.high_error_accounts + s.cooldown_accounts + s.half_open_accounts + s.saturated_accounts),
      tone: 'text-orange-600 dark:text-orange-400'
    },
    {
      key: 'unavailable',
      label: t('admin.ops.openaiAdaptiveLearning.summary.unavailable'),
      value: formatNumber(s.unavailable_accounts),
      tone: 'text-gray-700 dark:text-gray-300'
    }
  ]
})

const settingsItems = computed(() => {
  const settings = response.value?.settings
  if (!settings) return []
  return [
    {
      key: 'window',
      label: t('admin.ops.openaiAdaptiveLearning.settings.window'),
      value: settings.learning_window_seconds > 0
        ? formatDuration(settings.learning_window_seconds)
        : t('admin.ops.openaiAdaptiveLearning.settings.noReset')
    },
    {
      key: 'samples',
      label: t('admin.ops.openaiAdaptiveLearning.settings.minSamples'),
      value: formatNumber(settings.min_recent_samples_for_shrink)
    },
    {
      key: 'shrink',
      label: t('admin.ops.openaiAdaptiveLearning.settings.shrinkThreshold'),
      value: formatPercent(settings.shrink_error_threshold, 1)
    },
    {
      key: 'burst',
      label: t('admin.ops.openaiAdaptiveLearning.settings.burstRatio'),
      value: formatPercent(settings.burst_probe_ratio, 1)
    },
    {
      key: 'topK',
      label: t('admin.ops.openaiAdaptiveLearning.settings.topK'),
      value: formatNumber(settings.top_k)
    }
  ]
})

function buildParams() {
  return {
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined,
    limit: 50
  }
}

async function loadData() {
  if (!enabledForPlatform.value) {
    response.value = null
    return
  }
  const seq = ++loadSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await opsAPI.getOpenAIAdaptiveLearning(buildParams())
    if (seq !== loadSeq) return
    response.value = data
  } catch (err: any) {
    if (seq !== loadSeq) return
    console.error('[OpsOpenAIAdaptiveLearningCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.openaiAdaptiveLearning.failedToLoad')
  } finally {
    if (seq === loadSeq) {
      loading.value = false
    }
  }
}

watch(
  () => ({
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    refreshToken: props.refreshToken
  }),
  () => {
    void loadData()
  },
  { immediate: true }
)

function statusLabel(status: string): string {
  const key = statusKeyMap[status]
  return key ? t(key) : status
}

function modeLabel(mode?: string): string {
  const key = modeKeyMap[String(mode || '').toLowerCase()]
  return key ? t(key) : (mode || '-')
}

function statusClass(status: string): string {
  return statusClassMap[status] || statusClassMap.unavailable
}

function formatInt(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return formatNumber(Math.round(v))
}

function formatPercent(v?: number | null, digits = 0): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return `${(v * 100).toFixed(digits)}%`
}

function formatLoad(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return `${v.toFixed(v >= 10 ? 0 : 1)}%`
}

function formatScore(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return `${Math.round(v * 100)}`
}

function formatRate(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return v.toFixed(2)
}

function formatDuration(seconds?: number | null): string {
  if (typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds <= 0) return '-'
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}

function formatTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

function loadBarStyle(row: OpsOpenAIAdaptiveLearningAccount): string {
  const pct = Number.isFinite(row.load_percentage) ? Math.min(100, Math.max(0, row.load_percentage)) : 0
  return `width: ${pct}%`
}

function loadBarClass(row: OpsOpenAIAdaptiveLearningAccount): string {
  if (row.scheduler_status === 'cooldown' || row.scheduler_status === 'high_error') return 'bg-red-500'
  if (row.load_percentage >= 90 || row.scheduler_status === 'saturated') return 'bg-orange-500'
  if (row.load_percentage >= 70 || row.waiting_count > 0) return 'bg-amber-500'
  return 'bg-green-500'
}
</script>

<template>
  <section v-if="enabledForPlatform" class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-bold text-gray-900 dark:text-white">
            {{ t('admin.ops.openaiAdaptiveLearning.title') }}
          </h3>
          <span
            v-if="response"
            :class="[
              'rounded-full px-2 py-0.5 text-[11px] font-semibold',
              response.enabled
                ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
            ]"
          >
            {{ response.enabled ? modeLabel(response.mode) : t('admin.ops.openaiAdaptiveLearning.disabled') }}
          </span>
          <span
            v-if="response && !response.realtime_enabled"
            class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
          >
            {{ t('admin.ops.openaiAdaptiveLearning.realtimeOff') }}
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.openaiAdaptiveLearning.description') }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <RouterLink
          class="btn btn-secondary btn-sm"
          to="/admin/settings"
          :title="t('admin.ops.openaiAdaptiveLearning.openSettingsTitle')"
        >
          {{ t('admin.ops.openaiAdaptiveLearning.openSettings') }}
        </RouterLink>
        <button
          class="btn btn-secondary btn-sm"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="loadData"
        >
          <svg class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
      </div>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div v-if="loading && !response" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <template v-else-if="response">
      <div class="mb-4 grid grid-cols-2 gap-3 md:grid-cols-4">
        <div
          v-for="item in summaryItems"
          :key="item.key"
          class="rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-700"
        >
          <div :class="['text-lg font-bold', item.tone]">{{ item.value }}</div>
          <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ item.label }}</div>
        </div>
      </div>

      <div class="mb-4 flex flex-wrap gap-2 text-[11px] text-gray-500 dark:text-gray-400">
        <span
          v-for="item in settingsItems"
          :key="item.key"
          class="rounded-full bg-gray-100 px-2 py-1 dark:bg-dark-700"
        >
          {{ item.label }}: <span class="font-semibold text-gray-800 dark:text-gray-200">{{ item.value }}</span>
        </span>
      </div>

      <div v-if="accounts.length === 0">
        <EmptyState
          :title="t('common.noData')"
          :description="t('admin.ops.openaiAdaptiveLearning.empty')"
        />
      </div>

      <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="max-h-[460px] overflow-auto">
          <table class="min-w-full text-left text-xs">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.account') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.status') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.capacity') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.load') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.score') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.samples') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.error') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiAdaptiveLearning.table.lastEvent') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in accounts" :key="row.account_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                <td class="px-3 py-2">
                  <div class="max-w-[220px] truncate font-semibold text-gray-900 dark:text-white" :title="row.account_name">
                    {{ row.account_name || `#${row.account_id}` }}
                  </div>
                  <div class="mt-0.5 flex flex-wrap items-center gap-1 text-[11px] text-gray-500 dark:text-gray-400">
                    <span>#{{ row.account_id }}</span>
                    <span>{{ row.type || '-' }}</span>
                    <span>P{{ row.priority }}</span>
                    <span>{{ t('admin.ops.openaiAdaptiveLearning.rateMultiplier', { value: formatRate(row.rate_multiplier) }) }}</span>
                  </div>
                </td>
                <td class="px-3 py-2">
                  <span :class="['inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold', statusClass(row.scheduler_status)]">
                    {{ statusLabel(row.scheduler_status) }}
                  </span>
                  <div v-if="row.status_reason" class="mt-1 max-w-[190px] truncate text-[11px] text-gray-500 dark:text-gray-400" :title="row.status_reason">
                    {{ row.status_reason }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">
                    {{ formatInt(row.stable_capacity) }}/{{ formatInt(row.effective_capacity) }}/{{ formatInt(row.configured_concurrency) }}
                  </div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.openaiAdaptiveLearning.table.capacityHint') }}
                    <span v-if="row.burst_capacity > 0" class="text-amber-600 dark:text-amber-400">
                      +{{ formatInt(row.burst_capacity) }}
                    </span>
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="mb-1 flex items-center justify-between gap-2">
                    <span class="font-mono font-semibold text-gray-900 dark:text-white">
                      {{ formatInt(row.current_concurrency) }}/{{ formatInt(row.effective_capacity) }}
                    </span>
                    <span class="font-semibold text-gray-600 dark:text-gray-300">{{ formatLoad(row.load_percentage) }}</span>
                  </div>
                  <div class="h-1.5 w-28 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                    <div class="h-full rounded-full" :class="loadBarClass(row)" :style="loadBarStyle(row)"></div>
                  </div>
                  <div v-if="row.waiting_count > 0" class="mt-1 text-[11px] text-amber-600 dark:text-amber-400">
                    {{ t('admin.ops.openaiAdaptiveLearning.queued', { count: row.waiting_count }) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatScore(row.scheduler_score) }}</div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    S {{ formatScore(row.success_score) }} / C {{ formatScore(row.cost_score) }} / L {{ formatScore(row.capacity_score) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatInt(row.total_samples) }}</div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ formatInt(row.recent_samples) }} / {{ formatInt(row.recent_failures) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatPercent(row.error_ema, 1) }}</div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.openaiAdaptiveLearning.recentFailureRate') }} {{ formatPercent(row.recent_failure_rate, 1) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="text-[11px] text-gray-700 dark:text-gray-300" :title="formatTime(row.last_failure_at || row.last_success_at)">
                    {{ row.cooldown_remaining_sec > 0
                      ? t('admin.ops.openaiAdaptiveLearning.cooldownRemaining', { value: formatDuration(row.cooldown_remaining_sec) })
                      : formatTime(row.last_failure_at || row.last_success_at) }}
                  </div>
                  <div v-if="row.consecutive_failure > 0" class="mt-0.5 text-[11px] text-red-600 dark:text-red-400">
                    {{ t('admin.ops.openaiAdaptiveLearning.consecutiveFailures', { count: row.consecutive_failure }) }}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <p class="mt-3 text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.openaiAdaptiveLearning.scoreNote') }}
      </p>
    </template>
  </section>
</template>
