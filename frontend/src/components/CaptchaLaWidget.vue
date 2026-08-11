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
    :on-server-token-expired="refreshExpiredServerToken"
    @ready="handleReady"
    @success="handleSuccess"
    @error="handleError"
    @close="handleClose"
  />
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { Captchala, loadCaptchalaSDK, type CaptchalaResult } from '@captcha-la/vue'
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
const serverTokenExpiresAt = ref(0)
const renderKey = ref(0)
const activeAction = ref<CaptchaLaAction>(props.action)
let loadedAction: CaptchaLaAction | null = null
let sdkReady = false

let pending: {
  resolve: (token: string | null) => void
  reject: (error: unknown) => void
} | null = null
let pendingPromise: Promise<string | null> | null = null
let readyResolve: (() => void) | null = null
let readyReject: ((error: unknown) => void) | null = null
let readyTimer: ReturnType<typeof setTimeout> | null = null
let preloadPromise: Promise<void> | null = null
let preloadEpoch = 0
const serverTokenRefreshSkewMs = 10_000

function clearReadyWait(): void {
  if (readyTimer) clearTimeout(readyTimer)
  readyTimer = null
  readyResolve = null
  readyReject = null
}

function waitUntilReady(): Promise<void> {
  if (sdkReady) return Promise.resolve()
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
  sdkReady = true
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
  const normalizedError = error instanceof Error ? error : new Error('CaptchaLa verification failed')
  const rejectReady = readyReject
  clearReadyWait()
  rejectReady?.(normalizedError)
  emit('error')
  settle(null, normalizedError)
}

function handleClose(): void {
  emit('close')
  settle(null)
}

// Fetch the server-owned token and mount the SDK before the user submits the
// form. CaptchaLa only creates a challenge when verify() is explicitly called.
function hasUsableServerToken(action: CaptchaLaAction): boolean {
  return (
    Boolean(serverToken.value) &&
    loadedAction === action &&
    Date.now() < serverTokenExpiresAt.value - serverTokenRefreshSkewMs
  )
}

async function preload(actionOverride?: CaptchaLaAction): Promise<void> {
  const action = actionOverride || props.action
  if (hasUsableServerToken(action)) return

  if (preloadPromise) {
    await preloadPromise
    if (hasUsableServerToken(action)) return
    return preload(action)
  }

  const epoch = preloadEpoch
  const currentPromise = (async () => {
    activeAction.value = action
    const challenge = await issueCaptchaLaChallenge(action)
    if (epoch !== preloadEpoch) return

    const shouldRemount = !serverToken.value || loadedAction !== action
    sdkReady = false
    loadedAction = action
    serverToken.value = challenge.server_token
    serverTokenExpiresAt.value = Date.now() + Math.max(0, challenge.expires_in) * 1000
    if (shouldRemount) renderKey.value += 1
    await nextTick()
  })()
  preloadPromise = currentPromise

  try {
    await currentPromise
  } finally {
    if (preloadPromise === currentPromise) preloadPromise = null
  }
}

async function refreshExpiredServerToken(): Promise<string | null> {
  try {
    const challenge = await issueCaptchaLaChallenge(activeAction.value)
    serverTokenExpiresAt.value = Date.now() + Math.max(0, challenge.expires_in) * 1000
    return challenge.server_token || null
  } catch {
    return null
  }
}

async function startVerification(actionOverride?: CaptchaLaAction): Promise<string | null> {
  await preload(actionOverride)
  await waitUntilReady()

  return new Promise<string | null>((resolve, reject) => {
    pending = { resolve, reject }
    try {
      if (!captchaRef.value) {
        handleError(new Error('CaptchaLa SDK is not ready'))
        return
      }
      captchaRef.value.verify()
    } catch (error) {
      handleError(error)
    }
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

function reset(warm = true): void {
  preloadEpoch += 1
  clearReadyWait()
  captchaRef.value?.destroy()
  settle(null)
  sdkReady = false
  loadedAction = null
  serverToken.value = ''
  serverTokenExpiresAt.value = 0
  // A failed login consumes the pass token. Keep the next attempt warm without
  // reopening the popup; a fresh server token is ready in the background.
  if (warm && typeof window !== 'undefined') {
    void preload().catch(() => undefined)
  }
}

onMounted(() => {
  // Download the small loader in the background and issue the login token as
  // soon as the component is present. Neither action opens a challenge.
  void loadCaptchalaSDK().catch(() => undefined)
  void preload().catch(() => undefined)
})

onBeforeUnmount(() => reset(false))
defineExpose({ verify, preload, reset })
</script>
