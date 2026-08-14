<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  opsAPI,
  type OpsGeminiAdaptiveLearningAccount,
  type OpsGeminiAdaptiveLearningParams,
  type OpsGeminiAdaptiveLearningResponse,
  type OpsGeminiAdaptiveLearningSortBy,
  type OpsGeminiAdaptiveLearningSortOrder,
  type OpsAdaptiveLearningStatus,
  type OpsAdaptiveRuntimeStatus,
  type OpsGeminiAdaptiveQuotaBucket
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

type ViewMode = 'topn' | 'pagination'
type LearningStatusFilter = '' | OpsAdaptiveLearningStatus
type RuntimeStatusFilter = '' | OpsAdaptiveRuntimeStatus

const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsGeminiAdaptiveLearningResponse | null>(null)
const learningStatusFilter = ref<LearningStatusFilter>('')
const runtimeStatusFilter = ref<RuntimeStatusFilter>('')
const viewMode = ref<ViewMode>('topn')
const topN = ref(20)
const page = ref(1)
const pageSize = ref(20)
const sortBy = ref<OpsGeminiAdaptiveLearningSortBy>('status')
const sortOrder = ref<OpsGeminiAdaptiveLearningSortOrder>('desc')
let loadSeq = 0

const enabledForPlatform = computed(() => {
  const platform = String(props.platformFilter || '').trim().toLowerCase()
  return !platform || platform === 'gemini'
})

const accounts = computed(() => response.value?.accounts ?? [])
const summary = computed(() => response.value?.summary ?? null)
const total = computed(() => response.value?.total ?? response.value?.total_accounts ?? 0)
const totalPages = computed(() => {
  if (viewMode.value !== 'pagination') return 1
  return Math.max(1, Math.ceil(total.value / Math.max(1, pageSize.value)))
})

const learningStatusFilterOptions = computed(() => [
  { value: '', label: t('admin.ops.geminiAdaptiveLearning.statusFilter.all') },
  { value: 'unlearned', label: t('admin.ops.geminiAdaptiveLearning.status.unlearned') },
  { value: 'learning', label: t('admin.ops.geminiAdaptiveLearning.status.learning') },
  { value: 'learned', label: t('admin.ops.geminiAdaptiveLearning.status.learned') },
  { value: 'not_applicable', label: t('admin.ops.geminiAdaptiveLearning.status.notApplicable') }
])

const runtimeStatusFilterOptions = computed(() => [
  { value: '', label: t('admin.ops.geminiAdaptiveLearning.runtimeFilter.all') },
  { value: 'healthy', label: t('admin.ops.geminiAdaptiveLearning.status.healthy') },
  { value: 'quota_limited', label: t('admin.ops.geminiAdaptiveLearning.status.quotaLimited') },
  { value: 'high_error', label: t('admin.ops.geminiAdaptiveLearning.status.highError') },
  { value: 'cooldown', label: t('admin.ops.geminiAdaptiveLearning.status.cooldown') },
  { value: 'half_open', label: t('admin.ops.geminiAdaptiveLearning.status.halfOpen') },
  { value: 'saturated', label: t('admin.ops.geminiAdaptiveLearning.status.saturated') },
  { value: 'unavailable', label: t('admin.ops.geminiAdaptiveLearning.status.unavailable') }
])

const viewModeOptions = computed(() => [
  { value: 'topn', label: t('admin.ops.openaiTokenStats.viewModeTopN') },
  { value: 'pagination', label: t('admin.ops.openaiTokenStats.viewModePagination') }
])

const topNOptions = [10, 20, 50, 100].map((value) => ({ value, label: `Top ${value}` }))
const pageSizeOptions = [10, 20, 50, 100].map((value) => ({ value, label: String(value) }))

const statusKeyMap: Record<string, string> = {
  disabled: 'admin.ops.geminiAdaptiveLearning.status.disabled',
  unavailable: 'admin.ops.geminiAdaptiveLearning.status.unavailable',
  quota_limited: 'admin.ops.geminiAdaptiveLearning.status.quotaLimited',
  cooldown: 'admin.ops.geminiAdaptiveLearning.status.cooldown',
  half_open: 'admin.ops.geminiAdaptiveLearning.status.halfOpen',
  high_error: 'admin.ops.geminiAdaptiveLearning.status.highError',
  saturated: 'admin.ops.geminiAdaptiveLearning.status.saturated',
  learning: 'admin.ops.geminiAdaptiveLearning.status.learning',
  unlearned: 'admin.ops.geminiAdaptiveLearning.status.unlearned',
  learned: 'admin.ops.geminiAdaptiveLearning.status.learned',
  not_applicable: 'admin.ops.geminiAdaptiveLearning.status.notApplicable',
  healthy: 'admin.ops.geminiAdaptiveLearning.status.healthy'
}

const statusClassMap: Record<string, string> = {
  healthy: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
  learning: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
  unlearned: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  saturated: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  quota_limited: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  cooldown: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  half_open: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  high_error: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  unavailable: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  disabled: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  learned: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  not_applicable: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

const summaryItems = computed(() => {
  const value = summary.value
  if (!value) return []
  return [
    {
      key: 'tracked',
      label: t('admin.ops.geminiAdaptiveLearning.summary.tracked'),
      value: value.tracked_accounts,
      tone: 'text-gray-900 dark:text-white'
    },
    {
      key: 'healthy',
      label: t('admin.ops.geminiAdaptiveLearning.summary.healthy'),
      value: value.healthy_accounts,
      tone: 'text-green-600 dark:text-green-400'
    },
    {
      key: 'constrained',
      label: t('admin.ops.geminiAdaptiveLearning.summary.constrained'),
      value: value.quota_limited_accounts + value.cooldown_accounts + value.high_error_accounts + value.saturated_accounts,
      tone: 'text-orange-600 dark:text-orange-400'
    },
    {
      key: 'unavailable',
      label: t('admin.ops.geminiAdaptiveLearning.summary.unavailable'),
      value: value.unavailable_accounts,
      tone: 'text-red-600 dark:text-red-400'
    }
  ]
})

const settingsItems = computed(() => {
  const settings = response.value?.settings
  if (!settings) return []
  return [
    { key: 'topk', label: t('admin.ops.geminiAdaptiveLearning.settings.topK'), value: settings.top_k },
    { key: 'temperature', label: t('admin.ops.geminiAdaptiveLearning.settings.temperature'), value: settings.softmax_temperature.toFixed(2) },
    {
      key: 'weights',
      label: t('admin.ops.geminiAdaptiveLearning.settings.weights'),
      value: [
        settings.weight_reliability,
        settings.weight_capacity,
        settings.weight_ttft,
        settings.weight_cost
      ].map((value) => value.toFixed(2)).join('/')
    },
    { key: 'window', label: t('admin.ops.geminiAdaptiveLearning.settings.window'), value: formatDuration(settings.learning_window_seconds) }
  ]
})

const metricItems = computed(() => {
  const metrics = response.value?.metrics
  if (!metrics) return []
  return [
    { key: 'select', label: t('admin.ops.geminiAdaptiveLearning.metrics.select'), value: metrics.select_total },
    { key: 'stickyHit', label: t('admin.ops.geminiAdaptiveLearning.metrics.stickyHit'), value: metrics.sticky_hit_total },
    { key: 'stickyMigrate', label: t('admin.ops.geminiAdaptiveLearning.metrics.stickyMigrate'), value: metrics.sticky_migrate_total },
    { key: 'fallback', label: t('admin.ops.geminiAdaptiveLearning.metrics.fallback'), value: metrics.fallback_total },
    { key: 'shrink', label: t('admin.ops.geminiAdaptiveLearning.metrics.capacityDecrease'), value: metrics.capacity_decrease_total },
    { key: 'quotaError', label: t('admin.ops.geminiAdaptiveLearning.metrics.quotaErrors'), value: metrics.quota_snapshot_error_total },
    { key: 'shadow', label: t('admin.ops.geminiAdaptiveLearning.metrics.shadowDiverge'), value: metrics.shadow_diverge_total }
  ]
})

function buildParams(): OpsGeminiAdaptiveLearningParams {
  const params: OpsGeminiAdaptiveLearningParams = {
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0
      ? props.groupIdFilter
      : undefined,
    learning_status: learningStatusFilter.value || undefined,
    runtime_status: runtimeStatusFilter.value || undefined,
    sort_by: sortBy.value,
    sort_order: sortOrder.value
  }
  if (viewMode.value === 'topn') {
    params.top_n = topN.value
  } else {
    params.page = page.value
    params.page_size = pageSize.value
  }
  return params
}

async function loadData() {
  if (!enabledForPlatform.value) {
    response.value = null
    loading.value = false
    return
  }
  const seq = ++loadSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await opsAPI.getGeminiAdaptiveLearning(buildParams())
    if (seq !== loadSeq) return
    response.value = data
    if (viewMode.value === 'pagination' && page.value > totalPages.value) {
      page.value = totalPages.value
      response.value = await opsAPI.getGeminiAdaptiveLearning(buildParams())
    }
  } catch (err: any) {
    if (seq !== loadSeq) return
    console.error('[OpsGeminiAdaptiveLearningCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.geminiAdaptiveLearning.failedToLoad')
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

watch(
  () => ({
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    refreshToken: props.refreshToken,
    learningStatus: learningStatusFilter.value,
    runtimeStatus: runtimeStatusFilter.value,
    viewMode: viewMode.value,
    topN: topN.value,
    page: page.value,
    pageSize: pageSize.value,
    sortBy: sortBy.value,
    sortOrder: sortOrder.value
  }),
  (next, prev) => {
    const filtersChanged = !prev ||
      next.platform !== prev.platform ||
      next.groupId !== prev.groupId ||
      next.learningStatus !== prev.learningStatus ||
      next.runtimeStatus !== prev.runtimeStatus ||
      next.viewMode !== prev.viewMode ||
      next.pageSize !== prev.pageSize ||
      next.sortBy !== prev.sortBy ||
      next.sortOrder !== prev.sortOrder

    if (next.viewMode === 'pagination' && filtersChanged && next.page !== 1) {
      page.value = 1
      return
    }
    void loadData()
  },
  { immediate: true }
)

function setSort(nextSortBy: OpsGeminiAdaptiveLearningSortBy) {
  if (sortBy.value === nextSortBy) {
    sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
    return
  }
  sortBy.value = nextSortBy
  sortOrder.value = nextSortBy === 'account' || nextSortBy === 'latency' ? 'asc' : 'desc'
}

function statusLabel(status: string): string {
  const key = statusKeyMap[status]
  return key ? t(key) : status
}

function statusClass(status: string): string {
  return statusClassMap[status] || statusClassMap.unavailable
}

function modeLabel(mode?: string): string {
  if (mode === 'enforce') return t('admin.ops.geminiAdaptiveLearning.mode.enforce')
  if (mode === 'shadow') return t('admin.ops.geminiAdaptiveLearning.mode.shadow')
  return mode || '-'
}

function formatInt(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return formatNumber(Math.round(value))
}

function formatScore(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return String(Math.round(value * 100))
}

function formatRate(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value.toFixed(2)
}

function formatPercent(value?: number | null, digits = 0): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(digits)}%`
}

function formatLoad(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${value.toFixed(value >= 10 ? 0 : 1)}%`
}

function formatLatency(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '-'
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(value < 10000 ? 1 : 0)}s`
}

function formatDuration(seconds?: number | null): string {
  if (typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds <= 0) return '-'
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  return hours < 24 ? `${hours}h` : `${Math.floor(hours / 24)}d`
}

function formatTime(value?: string | null): string {
  if (!value || value.startsWith('0001-01-01')) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

function latestEvent(row: OpsGeminiAdaptiveLearningAccount): string | undefined {
  return [row.last_success_at, row.last_failure_at, row.capacity_cooldown_until, row.quota_next_probe_at]
    .filter((value): value is string => Boolean(value))
    .sort((left, right) => new Date(right).getTime() - new Date(left).getTime())[0]
}

function quotaBucketLabel(bucket?: OpsGeminiAdaptiveQuotaBucket | string): string {
  const key = String(bucket || 'unknown')
  return t(`admin.ops.geminiAdaptiveLearning.quota.bucket.${key}`)
}

function formatQuotaUsage(used: number, limit: number, bucket: OpsGeminiAdaptiveQuotaBucket | string): string {
  if (bucket === 'unlimited' || limit <= 0) return t('admin.ops.geminiAdaptiveLearning.quota.unlimited')
  return `${formatInt(used)}/${formatInt(limit)} (${formatPercent(used / limit, 0)})`
}

function loadBarStyle(row: OpsGeminiAdaptiveLearningAccount): string {
  const percentage = Number.isFinite(row.load_percentage)
    ? Math.min(100, Math.max(0, row.load_percentage))
    : 0
  return `width: ${percentage}%`
}

function loadBarClass(row: OpsGeminiAdaptiveLearningAccount): string {
  if (row.runtime_status === 'cooldown' || row.runtime_status === 'high_error' || row.runtime_status === 'quota_limited') return 'bg-red-500'
  if (row.load_percentage >= 90 || row.runtime_status === 'saturated') return 'bg-orange-500'
  if (row.load_percentage >= 70 || row.waiting_count > 0) return 'bg-amber-500'
  return 'bg-green-500'
}

function onPrevPage() {
  if (viewMode.value === 'pagination' && page.value > 1) page.value -= 1
}

function onNextPage() {
  if (viewMode.value === 'pagination' && page.value < totalPages.value) page.value += 1
}
</script>

<template>
  <section v-if="enabledForPlatform" class="card p-4 md:p-5" data-testid="gemini-adaptive-learning-card">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-bold text-gray-900 dark:text-white">
            {{ t('admin.ops.geminiAdaptiveLearning.title') }}
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
            {{ response.enabled ? modeLabel(response.mode) : t('admin.ops.geminiAdaptiveLearning.disabled') }}
          </span>
          <span
            v-if="response && !response.realtime_enabled"
            class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
          >
            {{ t('admin.ops.geminiAdaptiveLearning.realtimeOff') }}
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.geminiAdaptiveLearning.description') }}
        </p>
      </div>

      <div class="flex flex-wrap items-center justify-end gap-2">
        <div class="w-36">
          <Select v-model="learningStatusFilter" :options="learningStatusFilterOptions" />
        </div>
        <div class="w-36">
          <Select v-model="runtimeStatusFilter" :options="runtimeStatusFilterOptions" />
        </div>
        <div class="w-36">
          <Select v-model="viewMode" :options="viewModeOptions" />
        </div>
        <div v-if="viewMode === 'topn'" class="w-28">
          <Select v-model="topN" :options="topNOptions" />
        </div>
        <template v-else>
          <div class="w-24">
            <Select v-model="pageSize" :options="pageSizeOptions" />
          </div>
          <button class="btn btn-secondary btn-sm" :disabled="loading || page <= 1" @click="onPrevPage">
            {{ t('admin.ops.openaiTokenStats.prevPage') }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="loading || page >= totalPages" @click="onNextPage">
            {{ t('admin.ops.openaiTokenStats.nextPage') }}
          </button>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.openaiTokenStats.pageInfo', { page, total: totalPages }) }}
          </span>
        </template>
        <RouterLink
          class="btn btn-secondary btn-sm"
          to="/admin/settings"
          :title="t('admin.ops.geminiAdaptiveLearning.openSettingsTitle')"
        >
          <Icon name="cog" size="xs" />
          <span>{{ t('admin.ops.geminiAdaptiveLearning.openSettings') }}</span>
        </RouterLink>
        <button
          class="btn btn-secondary btn-sm px-2"
          :disabled="loading"
          :title="t('common.refresh')"
          :aria-label="t('common.refresh')"
          @click="loadData"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
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
      <div class="mb-4 grid grid-cols-2 divide-x divide-gray-200 rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700 md:grid-cols-4">
        <div v-for="item in summaryItems" :key="item.key" class="min-w-0 px-3 py-2">
          <div :class="['text-lg font-bold', item.tone]">{{ formatInt(item.value) }}</div>
          <div class="truncate text-[11px] text-gray-500 dark:text-gray-400">{{ item.label }}</div>
        </div>
      </div>

      <div class="mb-3 flex flex-wrap gap-2 text-[11px] text-gray-500 dark:text-gray-400">
        <span v-for="item in settingsItems" :key="item.key" class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">
          {{ item.label }}: <span class="font-semibold text-gray-800 dark:text-gray-200">{{ item.value }}</span>
        </span>
      </div>
      <div class="mb-4 flex flex-wrap gap-x-4 gap-y-1 border-y border-gray-100 py-2 text-[11px] text-gray-500 dark:border-dark-700 dark:text-gray-400">
        <span v-for="item in metricItems" :key="item.key">
          {{ item.label }} <span class="font-mono font-semibold text-gray-800 dark:text-gray-200">{{ formatInt(item.value) }}</span>
        </span>
      </div>

      <EmptyState
        v-if="accounts.length === 0"
        :title="t('common.noData')"
        :description="t('admin.ops.geminiAdaptiveLearning.empty')"
      />

      <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="max-h-[500px] overflow-auto">
          <table class="min-w-[1280px] text-left text-xs">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th
                  v-for="column in ([
                    ['account', 'account'], ['status', 'status'], ['capacity', 'capacity'],
                    ['load', 'load'], ['score', 'score'], ['samples', 'samples'],
                    ['latency', 'latency'], ['last_event', 'lastEvent']
                  ] as const)"
                  :key="column[0]"
                  class="px-3 py-2 font-semibold"
                >
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort(column[0])">
                    {{ t(`admin.ops.geminiAdaptiveLearning.table.${column[1]}`) }}
                    <Icon
                      v-if="sortBy === column[0]"
                      :name="sortOrder === 'desc' ? 'chevronDown' : 'chevronUp'"
                      size="xs"
                    />
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  {{ t('admin.ops.geminiAdaptiveLearning.table.quota') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in accounts" :key="row.account_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                <td class="px-3 py-2">
                  <div class="max-w-[210px] truncate font-semibold text-gray-900 dark:text-white" :title="row.account_name">
                    {{ row.account_name || `#${row.account_id}` }}
                  </div>
                  <div class="mt-0.5 flex flex-wrap gap-1 text-[11px] text-gray-500 dark:text-gray-400">
                    <span>#{{ row.account_id }}</span><span>{{ row.platform }}</span><span>{{ row.type || '-' }}</span>
                    <span>{{ t('admin.ops.geminiAdaptiveLearning.rateMultiplier', { value: formatRate(row.rate_multiplier) }) }}</span>
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="flex flex-wrap gap-1">
                    <span :class="['inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold', statusClass(row.learning_status)]">
                      {{ statusLabel(row.learning_status) }}
                    </span>
                    <span :class="['inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold', statusClass(row.runtime_status)]">
                      {{ statusLabel(row.runtime_status) }}<template v-if="row.runtime_flags.length > 1"> +{{ row.runtime_flags.length - 1 }}</template>
                    </span>
                  </div>
                  <div v-if="row.runtime_reason" class="mt-1 max-w-[180px] truncate text-[11px] text-gray-500 dark:text-gray-400" :title="row.runtime_reason">
                    {{ row.runtime_reason }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">
					{{ formatInt(row.effective_capacity) }}/{{ formatInt(row.configured_concurrency) }}
                  </div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.geminiAdaptiveLearning.table.capacityHint') }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="mb-1 flex w-28 items-center justify-between gap-2">
                    <span class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatInt(row.current_concurrency) }}/{{ formatInt(row.effective_capacity) }}</span>
                    <span class="font-semibold text-gray-600 dark:text-gray-300">{{ formatLoad(row.load_percentage) }}</span>
                  </div>
                  <div class="h-1.5 w-28 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                    <div class="h-full rounded-full" :class="loadBarClass(row)" :style="loadBarStyle(row)"></div>
                  </div>
                  <div v-if="row.waiting_count > 0" class="mt-1 text-[11px] text-amber-600 dark:text-amber-400">
                    {{ t('admin.ops.geminiAdaptiveLearning.queued', { count: row.waiting_count }) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatScore(row.scheduler_score) }}</div>
                  <div class="mt-0.5 whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
                    R {{ formatScore(row.reliability_score) }} / C {{ formatScore(row.capacity_score) }} / T {{ formatScore(row.latency_score) }} / $ {{ formatScore(row.cost_score) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatInt(row.health_samples) }}</div>
                  <div class="mt-0.5 whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.geminiAdaptiveLearning.successEma') }} {{ formatPercent(row.path_success_ema, 1) }}
                  </div>
				  <div v-if="row.consecutive_failure > 0" class="mt-0.5 text-[11px] text-red-600 dark:text-red-400">
					{{ t('admin.ops.geminiAdaptiveLearning.failureStreaks', { count: row.consecutive_failure }) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="whitespace-nowrap font-mono font-semibold text-gray-900 dark:text-white">
                    {{ formatLatency(row.ttft_ema) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="whitespace-nowrap text-[11px] text-gray-700 dark:text-gray-300" :title="formatTime(latestEvent(row))">
                    {{ row.cooldown_remaining_sec > 0
                      ? t('admin.ops.geminiAdaptiveLearning.cooldownRemaining', { value: formatDuration(row.cooldown_remaining_sec) })
                      : formatTime(latestEvent(row)) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div v-if="row.quota?.data_available" class="space-y-0.5 whitespace-nowrap text-[11px]">
                    <div :class="row.quota.hard_rejected ? 'font-semibold text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-300'">
                      {{ quotaBucketLabel(row.quota.scope.daily) }} D: {{ formatQuotaUsage(row.quota.daily_used, row.quota.daily_limit, row.quota.scope.daily) }}
                    </div>
                    <div class="text-gray-500 dark:text-gray-400">
                      {{ quotaBucketLabel(row.quota.scope.minute) }} M: {{ formatQuotaUsage(row.quota.minute_used, row.quota.minute_limit, row.quota.scope.minute) }}
                    </div>
                    <div class="text-gray-400" :title="formatTime(row.quota.daily_reset_at)">
                      {{ t('admin.ops.geminiAdaptiveLearning.quota.reset') }} {{ formatTime(row.quota.minute_reset_at) }}
                    </div>
                  </div>
                  <span v-else class="text-[11px] text-gray-400">{{ t('admin.ops.geminiAdaptiveLearning.quota.unavailable') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <p class="mt-3 text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.geminiAdaptiveLearning.scoreNote') }}
      </p>
      <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.geminiAdaptiveLearning.totalAccounts', { total }) }}
      </p>
    </template>
  </section>
</template>
