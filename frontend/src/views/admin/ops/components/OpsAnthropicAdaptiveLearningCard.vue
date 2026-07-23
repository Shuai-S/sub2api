<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import {
  opsAPI,
  type OpsAnthropicAdaptiveLearningAccount,
  type OpsAnthropicAdaptiveLearningResponse,
  type OpsAnthropicAdaptiveLearningSortBy,
  type OpsAnthropicAdaptiveLearningSortOrder,
  type OpsAnthropicAdaptiveLearningStatus
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
const response = ref<OpsAnthropicAdaptiveLearningResponse | null>(null)
let loadSeq = 0

type ViewMode = 'topn' | 'pagination'
type StatusFilter = '' | OpsAnthropicAdaptiveLearningStatus
type ModelFamily = 'sonnet' | 'opus' | 'haiku' | 'other'

const statusFilter = ref<StatusFilter>('')
const modelFamily = ref<ModelFamily>('sonnet')
const viewMode = ref<ViewMode>('topn')
const topN = ref<number>(20)
const page = ref<number>(1)
const pageSize = ref<number>(20)
const sortBy = ref<OpsAnthropicAdaptiveLearningSortBy>('status')
const sortOrder = ref<OpsAnthropicAdaptiveLearningSortOrder>('desc')

const enabledForPlatform = computed(() => {
  const platform = String(props.platformFilter || '').trim().toLowerCase()
  return !platform || platform === 'anthropic'
})

const accounts = computed(() => response.value?.accounts ?? [])
const summary = computed(() => response.value?.summary ?? null)
const total = computed(() => response.value?.total ?? response.value?.total_accounts ?? 0)
const totalPages = computed(() => {
  if (viewMode.value !== 'pagination') return 1
  const size = pageSize.value > 0 ? pageSize.value : 20
  return Math.max(1, Math.ceil(total.value / size))
})

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.ops.anthropicAdaptiveLearning.statusFilter.all') },
  { value: 'healthy', label: t('admin.ops.anthropicAdaptiveLearning.status.healthy') },
  { value: 'learning', label: t('admin.ops.anthropicAdaptiveLearning.status.learning') },
  { value: 'unlearned', label: t('admin.ops.anthropicAdaptiveLearning.status.unlearned') },
  { value: 'high_error', label: t('admin.ops.anthropicAdaptiveLearning.status.highError') },
  { value: 'cooldown', label: t('admin.ops.anthropicAdaptiveLearning.status.cooldown') },
  { value: 'saturated', label: t('admin.ops.anthropicAdaptiveLearning.status.saturated') },
  { value: 'unavailable', label: t('admin.ops.anthropicAdaptiveLearning.status.unavailable') },
  { value: 'disabled', label: t('admin.ops.anthropicAdaptiveLearning.status.disabled') }
])

const modelFamilyOptions = computed(() => [
  { value: 'sonnet', label: t('admin.ops.anthropicAdaptiveLearning.modelFamily.sonnet') },
  { value: 'opus', label: t('admin.ops.anthropicAdaptiveLearning.modelFamily.opus') },
  { value: 'haiku', label: t('admin.ops.anthropicAdaptiveLearning.modelFamily.haiku') },
  { value: 'other', label: t('admin.ops.anthropicAdaptiveLearning.modelFamily.other') }
])

const viewModeOptions = computed(() => [
  { value: 'topn', label: t('admin.ops.openaiTokenStats.viewModeTopN') },
  { value: 'pagination', label: t('admin.ops.openaiTokenStats.viewModePagination') }
])

const topNOptions = computed(() => [
  { value: 10, label: 'Top 10' },
  { value: 20, label: 'Top 20' },
  { value: 50, label: 'Top 50' },
  { value: 100, label: 'Top 100' }
])

const pageSizeOptions = computed(() => [
  { value: 10, label: '10' },
  { value: 20, label: '20' },
  { value: 50, label: '50' },
  { value: 100, label: '100' }
])

const statusKeyMap: Record<string, string> = {
  disabled: 'admin.ops.anthropicAdaptiveLearning.status.disabled',
  unavailable: 'admin.ops.anthropicAdaptiveLearning.status.unavailable',
  cooldown: 'admin.ops.anthropicAdaptiveLearning.status.cooldown',
  high_error: 'admin.ops.anthropicAdaptiveLearning.status.highError',
  saturated: 'admin.ops.anthropicAdaptiveLearning.status.saturated',
  learning: 'admin.ops.anthropicAdaptiveLearning.status.learning',
  unlearned: 'admin.ops.anthropicAdaptiveLearning.status.unlearned',
  healthy: 'admin.ops.anthropicAdaptiveLearning.status.healthy'
}

const modeKeyMap: Record<string, string> = {
  enforce: 'admin.ops.anthropicAdaptiveLearning.mode.enforce',
  shadow: 'admin.ops.anthropicAdaptiveLearning.mode.shadow'
}

const statusClassMap: Record<string, string> = {
  disabled: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  unavailable: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
  cooldown: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  high_error: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  saturated: 'bg-fuchsia-100 text-fuchsia-700 dark:bg-fuchsia-900/30 dark:text-fuchsia-300',
  learning: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  unlearned: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  healthy: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

const summaryItems = computed(() => {
  const current = summary.value
  if (!current || !response.value) return []
  return [
    {
      key: 'tracked',
      label: t('admin.ops.anthropicAdaptiveLearning.summary.tracked'),
      value: `${formatNumber(current.tracked_accounts)}/${formatNumber(response.value.total_accounts)}`,
      tone: 'text-gray-900 dark:text-white'
    },
    {
      key: 'healthy',
      label: t('admin.ops.anthropicAdaptiveLearning.summary.healthy'),
      value: formatNumber(current.healthy_accounts),
      tone: 'text-green-600 dark:text-green-400'
    },
    {
      key: 'risk',
      label: t('admin.ops.anthropicAdaptiveLearning.summary.risk'),
      value: formatNumber(current.high_error_accounts + current.cooldown_accounts + current.saturated_accounts),
      tone: 'text-orange-600 dark:text-orange-400'
    },
    {
      key: 'unavailable',
      label: t('admin.ops.anthropicAdaptiveLearning.summary.unavailable'),
      value: formatNumber(current.unavailable_accounts),
      tone: 'text-gray-700 dark:text-gray-300'
    }
  ]
})

const settingsItems = computed(() => {
  const settings = response.value?.settings
  if (!settings) return []
  return [
    {
      key: 'topK',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.topK'),
      value: formatNumber(settings.top_k)
    },
    {
      key: 'temperature',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.temperature'),
      value: settings.softmax_temperature.toFixed(2)
    },
    {
      key: 'weights',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.weights'),
      value: `${settings.weight_reliability}/${settings.weight_capacity}/${settings.weight_latency}/${settings.weight_exploration}`
    },
    {
      key: 'window',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.window'),
      value: formatDuration(settings.learning_window_seconds)
    },
    {
      key: 'samples',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.minSamples'),
      value: formatNumber(settings.min_recent_samples_for_shrink)
    },
    {
      key: 'failures',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.capacityFailures'),
      value: formatNumber(settings.capacity_failure_threshold)
    },
    {
      key: 'shrink',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.shrinkThreshold'),
      value: formatPercent(settings.shrink_error_threshold, 1)
    },
    {
      key: 'factors',
      label: t('admin.ops.anthropicAdaptiveLearning.settings.shrinkFactors'),
      value: `${settings.shrink_factor_soft.toFixed(2)}/${settings.shrink_factor_hard.toFixed(2)}`
    }
  ]
})

function buildParams() {
  const params: Record<string, any> = {
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined,
    model: modelFamily.value,
    status: statusFilter.value || undefined,
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
    const data = await opsAPI.getAnthropicAdaptiveLearning(buildParams())
    if (seq !== loadSeq) return
    response.value = data
    if (viewMode.value === 'pagination' && page.value > totalPages.value) {
      page.value = totalPages.value
      response.value = await opsAPI.getAnthropicAdaptiveLearning(buildParams())
    }
  } catch (err: any) {
    if (seq !== loadSeq) return
    console.error('[OpsAnthropicAdaptiveLearningCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.anthropicAdaptiveLearning.failedToLoad')
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

watch(
  () => ({
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    refreshToken: props.refreshToken,
    statusFilter: statusFilter.value,
    modelFamily: modelFamily.value,
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
      next.statusFilter !== prev.statusFilter ||
      next.modelFamily !== prev.modelFamily ||
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

function formatInt(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return formatNumber(Math.round(value))
}

function formatPercent(value?: number | null, digits = 0): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(digits)}%`
}

function formatLoad(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${value.toFixed(value >= 10 ? 0 : 1)}%`
}

function formatScore(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `${Math.round(value * 100)}`
}

function formatDuration(seconds?: number | null): string {
  if (typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds <= 0) return '-'
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  return hours < 24 ? `${hours}h` : `${Math.floor(hours / 24)}d`
}

function formatLatency(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '-'
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(value < 10000 ? 1 : 0)}s`
}

function formatTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

function latestEvent(row: OpsAnthropicAdaptiveLearningAccount): string | undefined {
  return [row.last_success_at, row.last_failure_at, row.last_capacity_failure_at]
    .filter((value): value is string => Boolean(value))
    .sort((left, right) => new Date(right).getTime() - new Date(left).getTime())[0]
}

function loadBarStyle(row: OpsAnthropicAdaptiveLearningAccount): string {
  const percentage = Number.isFinite(row.load_percentage)
    ? Math.min(100, Math.max(0, row.load_percentage))
    : 0
  return `width: ${percentage}%`
}

function loadBarClass(row: OpsAnthropicAdaptiveLearningAccount): string {
  if (row.scheduler_status === 'cooldown' || row.scheduler_status === 'high_error') return 'bg-red-500'
  if (row.load_percentage >= 90 || row.scheduler_status === 'saturated') return 'bg-orange-500'
  if (row.load_percentage >= 70 || row.waiting_count > 0) return 'bg-amber-500'
  return 'bg-green-500'
}

function onPrevPage() {
  if (viewMode.value === 'pagination' && page.value > 1) page.value -= 1
}

function onNextPage() {
  if (viewMode.value === 'pagination' && page.value < totalPages.value) page.value += 1
}

function setSort(nextSortBy: OpsAnthropicAdaptiveLearningSortBy) {
  if (sortBy.value === nextSortBy) {
    sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
    return
  }
  sortBy.value = nextSortBy
  sortOrder.value = nextSortBy === 'account' || nextSortBy === 'latency' ? 'asc' : 'desc'
}

function sortIndicator(nextSortBy: OpsAnthropicAdaptiveLearningSortBy): string {
  if (sortBy.value !== nextSortBy) return ''
  return sortOrder.value === 'desc' ? '↓' : '↑'
}
</script>

<template>
  <section v-if="enabledForPlatform" class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-bold text-gray-900 dark:text-white">
            {{ t('admin.ops.anthropicAdaptiveLearning.title') }}
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
            {{ response.enabled ? modeLabel(response.mode) : t('admin.ops.anthropicAdaptiveLearning.disabled') }}
          </span>
          <span
            v-if="response && !response.realtime_enabled"
            class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
          >
            {{ t('admin.ops.anthropicAdaptiveLearning.realtimeOff') }}
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.anthropicAdaptiveLearning.description') }}
        </p>
      </div>

      <div class="flex flex-wrap items-center justify-end gap-2">
        <div class="w-32" :title="t('admin.ops.anthropicAdaptiveLearning.modelFamily.tooltip')">
          <Select v-model="modelFamily" :options="modelFamilyOptions" />
        </div>
        <div class="w-36">
          <Select v-model="statusFilter" :options="statusFilterOptions" />
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
          :title="t('admin.ops.anthropicAdaptiveLearning.openSettingsTitle')"
        >
          {{ t('admin.ops.anthropicAdaptiveLearning.openSettings') }}
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
          :description="t('admin.ops.anthropicAdaptiveLearning.empty')"
        />
      </div>

      <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="max-h-[460px] overflow-auto">
          <table class="min-w-full text-left text-xs">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('account')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.account') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('account') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('status')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.status') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('status') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('capacity')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.capacity') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('capacity') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('load')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.load') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('load') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('score')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.score') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('score') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('samples')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.samples') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('samples') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('latency')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.latency') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('latency') }}</span>
                  </button>
                </th>
                <th class="px-3 py-2 font-semibold">
                  <button class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white" @click="setSort('last_event')">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.lastEvent') }}
                    <span class="w-3 text-[10px]">{{ sortIndicator('last_event') }}</span>
                  </button>
                </th>
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
                    {{ formatInt(row.estimated_capacity) }}/{{ formatInt(row.configured_concurrency) }}
                  </div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.anthropicAdaptiveLearning.table.capacityHint') }}
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
                    {{ t('admin.ops.anthropicAdaptiveLearning.queued', { count: row.waiting_count }) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatScore(row.scheduler_score) }}</div>
                  <div class="mt-0.5 whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
                    R {{ formatScore(row.reliability_score) }} / C {{ formatScore(row.capacity_score) }} / L {{ formatScore(row.latency_score) }} / E {{ formatScore(row.exploration_score) }}
                  </div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.anthropicAdaptiveLearning.successEma') }} {{ formatPercent(row.success_ema, 1) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatInt(row.total_samples) }}</div>
                  <div class="mt-0.5 whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
                    H {{ formatInt(row.recent_health_samples) }}/{{ formatInt(row.recent_health_failures) }}
                    · C {{ formatInt(row.recent_capacity_samples) }}/{{ formatInt(row.recent_capacity_failures) }}
                  </div>
                  <div v-if="row.consecutive_failure > 0 || row.consecutive_capacity_failure > 0" class="mt-0.5 text-[11px] text-red-600 dark:text-red-400">
                    {{ t('admin.ops.anthropicAdaptiveLearning.failureStreaks', { health: row.consecutive_failure, capacity: row.consecutive_capacity_failure }) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="whitespace-nowrap font-mono font-semibold text-gray-900 dark:text-white">
                    {{ formatLatency(row.ttft_ema) }} / {{ formatLatency(row.latency_ema) }}
                  </div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ row.model_family }} · n={{ formatInt(row.latency_samples) }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="whitespace-nowrap text-[11px] text-gray-700 dark:text-gray-300" :title="formatTime(latestEvent(row))">
                    {{ row.cooldown_remaining_sec > 0
                      ? t('admin.ops.anthropicAdaptiveLearning.cooldownRemaining', { value: formatDuration(row.cooldown_remaining_sec) })
                      : formatTime(latestEvent(row)) }}
                  </div>
                  <div v-if="row.last_capacity_failure_at" class="mt-0.5 whitespace-nowrap text-[11px] text-orange-600 dark:text-orange-400">
                    {{ t('admin.ops.anthropicAdaptiveLearning.capacityFailureRate') }} {{ formatPercent(row.recent_capacity_failure_rate, 1) }}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <p class="mt-3 text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.anthropicAdaptiveLearning.scoreNote') }}
      </p>
      <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.anthropicAdaptiveLearning.totalAccounts', { total }) }}
      </p>
    </template>
  </section>
</template>
