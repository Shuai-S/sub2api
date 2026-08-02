<template>
  <Teleport to="body">
    <Transition name="custom-menu-modal">
      <div
        v-if="visible"
        class="fixed inset-0 z-[125] flex items-start justify-center overflow-y-auto bg-black/65 p-4 pt-[8vh] backdrop-blur-sm"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
      >
        <div
          ref="dialogRef"
          class="w-full max-w-[680px] overflow-hidden rounded-lg border border-gray-200 bg-white shadow-2xl outline-none dark:border-dark-700 dark:bg-dark-800"
          tabindex="-1"
          @click.stop
        >
          <div class="flex items-start gap-4 border-b border-gray-200 px-6 py-5 dark:border-dark-700">
            <span
              v-if="iconSvg"
              class="custom-menu-modal-icon flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-gray-100 p-2 text-gray-700 dark:bg-dark-700 dark:text-gray-200"
              v-html="sanitizeSvg(iconSvg)"
            ></span>
            <div class="min-w-0 flex-1 pt-1">
              <h2 :id="titleId" class="break-words text-xl font-semibold text-gray-900 dark:text-white">
                {{ dialogTitle }}
              </h2>
            </div>
            <button
              type="button"
              class="btn-ghost btn-icon flex-shrink-0"
              :aria-label="t('common.close')"
              data-testid="custom-menu-modal-close-icon"
              @click="handleClose"
            >
              <Icon name="x" size="sm" />
            </button>
          </div>

          <div class="max-h-[55vh] min-h-40 overflow-y-auto px-6 py-6">
            <div v-if="isLoading" class="space-y-3" aria-live="polite">
              <div class="h-4 w-3/4 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
              <div class="h-4 w-full animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
              <div class="h-4 w-5/6 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
              <span class="sr-only">{{ t('common.loading') }}</span>
            </div>

            <div v-else-if="hasError" class="flex min-h-28 flex-col items-center justify-center text-center">
              <Icon name="exclamationCircle" size="lg" class="text-red-500" />
              <p class="mt-3 text-sm text-gray-600 dark:text-gray-300">
                {{ t('customMenuModal.loadFailed') }}
              </p>
              <button type="button" class="btn btn-secondary btn-sm mt-4" @click="handleRetry">
                <Icon name="refresh" size="sm" />
                {{ t('customMenuModal.retry') }}
              </button>
            </div>

            <div
              v-else
              class="markdown-body prose prose-sm max-w-none dark:prose-invert"
              v-html="renderedContent"
            ></div>
          </div>

          <div class="flex justify-end border-t border-gray-200 bg-gray-50 px-6 py-4 dark:border-dark-700 dark:bg-dark-900/40">
            <button
              type="button"
              class="btn btn-primary"
              data-testid="custom-menu-modal-close"
              @click="handleClose"
            >
              {{ t('common.close') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import { useCustomMenuModalStore } from '@/stores/customMenuModal'
import { sanitizeSvg } from '@/utils/sanitize'
import type { CustomMenuItem } from '@/types'
import '@/styles/announcement-markdown.css'

type PreviewItem = Pick<
  CustomMenuItem,
  'id' | 'label' | 'icon_svg' | 'modal_title' | 'modal_content'
>

const props = withDefaults(defineProps<{
  previewItem?: PreviewItem | null
}>(), {
  previewItem: null,
})

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const store = useCustomMenuModalStore()
const dialogRef = ref<HTMLElement | null>(null)
const titleId = `custom-menu-modal-title-${Math.random().toString(36).slice(2)}`
let previousOverflow = ''
let previousFocus: HTMLElement | null = null

marked.setOptions({ breaks: true, gfm: true })

const isPreview = computed(() => props.previewItem !== null)
const visible = computed(() => isPreview.value || store.isOpen)
const iconSvg = computed(() => (
  isPreview.value ? props.previewItem?.icon_svg || '' : store.selectedItem?.icon_svg || ''
))
const dialogTitle = computed(() => {
  if (isPreview.value) {
    return props.previewItem?.modal_title?.trim() || props.previewItem?.label || ''
  }
  return store.content?.title || store.selectedItem?.label || ''
})
const sourceContent = computed(() => (
  isPreview.value ? props.previewItem?.modal_content || '' : store.content?.content || ''
))
const isLoading = computed(() => !isPreview.value && store.loading)
const hasError = computed(() => !isPreview.value && store.error)

const renderedContent = computed(() => {
  const html = DOMPurify.sanitize(marked.parse(sourceContent.value) as string)
  if (typeof document === 'undefined') return html

  const container = document.createElement('div')
  container.innerHTML = html
  container.querySelectorAll<HTMLAnchorElement>('a[href]').forEach((link) => {
    if (/^https?:\/\//i.test(link.getAttribute('href') || '')) {
      link.target = '_blank'
      link.rel = 'noopener noreferrer'
    }
  })
  return container.innerHTML
})

function handleClose() {
  if (isPreview.value) {
    emit('close')
  } else {
    store.close()
  }
}

function handleRetry() {
  store.retry()
}

function handleKeydown(event: KeyboardEvent) {
  if (!visible.value) return

  if (event.key === 'Escape') {
    handleClose()
    return
  }

  if (event.key !== 'Tab' || !dialogRef.value) return

  const focusable = Array.from(
    dialogRef.value.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute('hidden'))

  if (focusable.length === 0) {
    event.preventDefault()
    dialogRef.value.focus()
    return
  }

  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && (document.activeElement === first || document.activeElement === dialogRef.value)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(visible, async (open) => {
  if (open) {
    previousOverflow = document.body.style.overflow
    previousFocus = document.activeElement as HTMLElement | null
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', handleKeydown)
    await nextTick()
    dialogRef.value?.focus()
    return
  }

  document.body.style.overflow = previousOverflow
  document.removeEventListener('keydown', handleKeydown)
  previousFocus?.focus()
  previousFocus = null
}, { immediate: true })

onBeforeUnmount(() => {
  document.body.style.overflow = previousOverflow
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.custom-menu-modal-enter-active,
.custom-menu-modal-leave-active {
  transition: opacity 0.2s ease;
}

.custom-menu-modal-enter-from,
.custom-menu-modal-leave-to {
  opacity: 0;
}

.custom-menu-modal-enter-active > div,
.custom-menu-modal-leave-active > div {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.custom-menu-modal-enter-from > div,
.custom-menu-modal-leave-to > div {
  opacity: 0;
  transform: translateY(-8px) scale(0.98);
}

.custom-menu-modal-icon :deep(svg) {
  display: block;
  height: 100%;
  width: 100%;
}
</style>
