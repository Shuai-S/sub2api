<template>
  <TurnstileWidget
    v-if="turnstileEnabled && turnstileSiteKey"
    ref="turnstileRef"
    :site-key="turnstileSiteKey"
    @verify="(token) => emit('verify', token, '')"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <TencentCaptchaGate
    v-else-if="tencentEnabled && tencentAppId"
    ref="tencentRef"
    :app-id="tencentAppId"
    :region="tencentRegion"
  />
  <AliyunCaptchaWidget
    v-else-if="aliyunEnabled && aliyunSceneId && aliyunPrefix"
    ref="aliyunRef"
    :scene-id="aliyunSceneId"
    :prefix="aliyunPrefix"
    :region="aliyunRegion === 'sgp' ? 'sgp' : 'cn'"
    @verify="(param: string) => emit('verify', param, '')"
    @expire="emit('expire')"
    @error="emit('error')"
  />
  <CaptchaLaWidget
    v-else-if="captchalaEnabled && captchalaAppKey"
    ref="captchalaRef"
    :app-key="captchalaAppKey"
    :action="captchalaAction"
    @verify="(token: string) => emit('verify', token, '')"
    @error="emit('error')"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import TencentCaptchaGate from '@/components/TencentCaptchaGate.vue'
import AliyunCaptchaWidget from '@/components/AliyunCaptchaWidget.vue'
import CaptchaLaWidget from '@/components/CaptchaLaWidget.vue'
import type { CaptchaLaAction } from '@/api/auth'

// ActionCaptchaResult 动作触发式验证的结果：腾讯 randstr 非空，
// 阿里云和 CaptchaLa 的 randstr 恒为空。
export interface ActionCaptchaResult {
  token: string
  randstr: string
}

const props = withDefaults(defineProps<{
  siteKey?: string
  turnstileEnabled: boolean
  turnstileSiteKey: string
  tencentEnabled: boolean
  tencentAppId: string
  tencentRegion?: string
  aliyunEnabled?: boolean
  aliyunSceneId?: string
  aliyunPrefix?: string
  aliyunRegion?: string
  captchalaEnabled?: boolean
  captchalaAppKey?: string
  captchalaAction?: CaptchaLaAction
}>(), {
  captchalaEnabled: false,
  captchalaAppKey: '',
  captchalaAction: 'login'
})

const emit = defineEmits<{
  verify: [tokenOrTicket: string, randstr: string]
  expire: []
  error: []
}>()

const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const tencentRef = ref<InstanceType<typeof TencentCaptchaGate> | null>(null)
const aliyunRef = ref<InstanceType<typeof AliyunCaptchaWidget> | null>(null)
const captchalaRef = ref<InstanceType<typeof CaptchaLaWidget> | null>(null)

function reset(): void {
  turnstileRef.value?.reset()
  tencentRef.value?.reset()
  aliyunRef.value?.reset()
  captchalaRef.value?.reset()
}

// verifyAction 弹出当前启用的动作触发式验证码并等待结果；
// 用户关闭弹窗返回 null，验证异常 emit('error') 并返回 null。
async function verifyAction(actionOverride?: CaptchaLaAction): Promise<ActionCaptchaResult | null> {
  if (props.tencentEnabled && props.tencentAppId) {
    try {
      const proof = (await tencentRef.value?.verify()) ?? null
      if (!proof) return null
      return { token: proof.ticket, randstr: proof.randstr }
    } catch {
      emit('error')
      return null
    }
  }
  if (props.aliyunEnabled && props.aliyunSceneId && props.aliyunPrefix) {
    try {
      const param = (await aliyunRef.value?.verify()) ?? null
      if (!param) return null
      return { token: param, randstr: '' }
    } catch {
      emit('error')
      return null
    }
  }
  if (props.captchalaEnabled && props.captchalaAppKey) {
    try {
      const token = (await captchalaRef.value?.verify(actionOverride)) ?? null
      if (!token) return null
      return { token, randstr: '' }
    } catch {
      emit('error')
      return null
    }
  }
  return null
}

defineExpose({ reset, verifyAction })
</script>
