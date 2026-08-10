<template>
  <Captchala
    v-if="serverToken"
    :key="renderKey"
    ref="captchaRef"
    :app-key="appKey"
    :server-token="serverToken"
    :action="activeAction"
    :lang="language"
    product="popup"
    @ready="handleReady"
    @success="handleSuccess"
    @error="handleError"
    @close="handleClose"
  />
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { Captchala, type CaptchalaResult } from '@captcha-la/vue'
import { useI18n } from 'vue-i18n'
import { issueCaptchaLaChallenge, type CaptchaLaAction } from '@/api/auth'

const props = defineProps<{
  appKey: string
  action: CaptchaLaAction
}>()

const emit = defineEmits<{
  verify: [token: string]
  error: []
  close: []
}>()

const { locale } = useI18n()
const language = computed(() => (locale.value.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'))
const captchaRef = ref<InstanceType<typeof Captchala> | null>(null)
const serverToken = ref('')
const renderKey = ref(0)
const activeAction = ref<CaptchaLaAction>(props.action)

let pending: {
  resolve: (token: string | null) => void
  reject: (error: unknown) => void
} | null = null
let pendingPromise: Promise<string | null> | null = null
let readyResolve: (() => void) | null = null
let readyReject: ((error: unknown) => void) | null = null
let readyTimer: ReturnType<typeof setTimeout> | null = null

function clearReadyWait(): void {
  if (readyTimer) clearTimeout(readyTimer)
  readyTimer = null
  readyResolve = null
  readyReject = null
}

function waitUntilReady(): Promise<void> {
  clearReadyWait()
  return new Promise((resolve, reject) => {
    readyResolve = resolve
    readyReject = reject
    readyTimer = setTimeout(() => {
      const rejectReady = readyReject
      clearReadyWait()
      rejectReady?.(new Error('CaptchaLa SDK initialization timed out'))
    }, 12_000)
  })
}

function handleReady(): void {
  const resolve = readyResolve
  clearReadyWait()
  resolve?.()
}

function settle(value: string | null, error?: unknown): void {
  const current = pending
  pending = null
  if (!current) return
  if (error !== undefined) current.reject(error)
  else current.resolve(value)
}

function handleSuccess(result: CaptchalaResult): void {
  const token = result.token?.trim() || ''
  // Some deployed CaptchaLa CDN SDK versions omit `action` from the
  // success payload and only return `{ token, challengeId }`. The backend
  // validates the server-owned action, so treat an omitted action as valid
  // while still rejecting an explicitly mismatched action.
  const resultAction = typeof result.action === 'string' ? result.action.trim() : ''
  if (!token.startsWith('pt_') || (resultAction && resultAction !== activeAction.value)) {
    handleError(new Error('CaptchaLa returned an invalid proof'))
    return
  }
  emit('verify', token)
  settle(token)
}

function handleError(error: unknown): void {
  emit('error')
  settle(null, error instanceof Error ? error : new Error('CaptchaLa verification failed'))
}

function handleClose(): void {
  emit('close')
  settle(null)
}

async function startVerification(actionOverride?: CaptchaLaAction): Promise<string | null> {
  activeAction.value = actionOverride || props.action
  const challenge = await issueCaptchaLaChallenge(activeAction.value)
  serverToken.value = challenge.server_token
  renderKey.value += 1
  const ready = waitUntilReady()
  await nextTick()

  return new Promise<string | null>((resolve, reject) => {
    pending = { resolve, reject }
    void ready
      .then(() => captchaRef.value?.verify())
      .catch((error) => handleError(error))
  })
}

function verify(actionOverride?: CaptchaLaAction): Promise<string | null> {
  if (pendingPromise) return pendingPromise

  let currentPromise: Promise<string | null>
  currentPromise = startVerification(actionOverride).finally(() => {
    if (pendingPromise === currentPromise) pendingPromise = null
  })
  pendingPromise = currentPromise
  return currentPromise
}

function reset(): void {
  clearReadyWait()
  captchaRef.value?.destroy()
  settle(null)
  serverToken.value = ''
}

onBeforeUnmount(reset)
defineExpose({ verify, reset })
</script>
