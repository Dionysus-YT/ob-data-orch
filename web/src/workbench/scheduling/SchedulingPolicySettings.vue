<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Checkbox as ACheckbox, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Form as AForm, FormItem as AFormItem, Input as AInput, InputNumber as AInputNumber, Modal as AModal, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import { useSchedulingPolicy } from './useSchedulingPolicy'

const { MessageContext, success, clear } = useOperationMessage()
const root = ref<HTMLElement>()
let focusVersion = 0
const state = useSchedulingPolicy(browserApi(), { onSaved: text => { success(text); void focusControl('[data-scheduling-edit]') }, clearNotice: clear })
const sourceNames = { PLATFORM_SETTING: '平台设置', UNCONFIGURED: '尚未配置平台上限' }
const route = useRoute()
const leaveOpen = ref(false)
let pendingLeave: ((allowed: boolean) => void) | undefined
function canLeave(): boolean | Promise<boolean> {
  if (state.saving.value || pendingLeave) return false
  if (!state.dirty.value) return true
  state.cancelReview(); leaveOpen.value = true
  return new Promise(resolve => { pendingLeave = resolve })
}
function resolveLeave(allowed: boolean) { const resolve = pendingLeave; pendingLeave = undefined; leaveOpen.value = false; if (allowed) state.cancel(); resolve?.(allowed) }
function beforeUnload(event: BeforeUnloadEvent) { if (state.dirty.value || state.saving.value) event.preventDefault() }
onBeforeRouteLeave(canLeave); onBeforeRouteUpdate(canLeave); useLogoutGuard(canLeave)
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
watch(() => route.fullPath, () => { focusVersion++; state.reset(); void state.read() }, { flush: 'sync' })
async function focusControl(selector: string) { const expected = ++focusVersion; await nextTick(); if (expected === focusVersion) root.value?.querySelector<HTMLElement>(selector)?.focus() }
function edit() { state.edit(); if (state.editing.value) void focusControl('input[aria-label="全平台最大并发"]') }
function cancel() { state.cancel(); if (!state.editing.value) void focusControl('[data-scheduling-edit]') }
function review() { state.reviewChange(); if (state.maximumError.value || state.reasonError.value) void focusControl(state.maximumError.value ? 'input[aria-label="全平台最大并发"]' : 'input[aria-label="变更原因"]') }
onScopeDispose(() => { focusVersion++; window.removeEventListener('beforeunload', beforeUnload); resolveLeave(false) })
onMounted(() => { void state.read() })
</script>

<template>
  <section ref="root" class="scheduling-policy-settings" aria-labelledby="platform-concurrency-title" :aria-busy="state.loading.value || state.saving.value">
    <MessageContext />
    <h3 id="platform-concurrency-title">全平台执行并发</h3>
    <AAlert type="info" show-icon message="上限约束后续新执行领取" description="既有执行保持运行，达到上限后新任务等待。节点 JVM 预算、Agent 承载能力和显式选节点仍须同时满足；无需重启。" />
    <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取当前策略…</div>
    <ADescriptions v-if="state.policy.value" :column="1">
      <ADescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取上限' : '当前生效上限'">{{ state.policy.value.maxConcurrentExecutions === null ? '尚未配置平台上限' : `${state.policy.value.maxConcurrentExecutions} 个执行` }}<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span></ADescriptionsItem>
      <ADescriptionsItem label="配置来源">{{ sourceNames[state.policy.value.source] }}</ADescriptionsItem>
      <ADescriptionsItem label="生效方式">后续新执行领取使用最新策略 · 无需重启</ADescriptionsItem>
      <ADescriptionsItem label="最近修改">{{ state.policy.value.modifiedAt ? `${state.policy.value.modifiedAt} · ${state.policy.value.modifiedBy}` : '尚无平台修改记录' }}</ADescriptionsItem>
      <ADescriptionsItem label="全平台占用 / 运行">{{ state.policy.value.usage.visibility === 'AVAILABLE' ? `${state.policy.value.usage.heldExecutions} / ${state.policy.value.usage.runningExecutions}` : '无全局任务读取权限，数量不返回' }}</ADescriptionsItem>
      <ADescriptionsItem label="事实采样时间 · UTC">{{ state.policy.value.usage.observedAt }}</ADescriptionsItem>
      <ADescriptionsItem label="配置版本">{{ state.policy.value.revision }}</ADescriptionsItem>
    </ADescriptions>
    <AForm v-if="state.editing.value" :model="{ maxConcurrentExecutions: state.maximum.value, reason: state.reason.value }" layout="vertical" :disabled="state.saving.value || Boolean(state.review.value)" @finish="review">
      <AFormItem label="全平台最大并发" name="maxConcurrentExecutions" :validate-status="state.maximumError.value ? 'error' : undefined" :help="state.maximumError.value || '1–1024 个执行；尚未配置时不预填猜测值。'">
        <AInputNumber :value="state.maximum.value ?? undefined" aria-label="全平台最大并发" :min="1" :max="1024" :precision="0" :step="1" @change="state.maximum.value = typeof $event === 'number' ? $event : null" />
      </AFormItem>
      <AFormItem label="变更原因" name="reason" :validate-status="state.reasonError.value ? 'error' : undefined" :help="state.reasonError.value || '1–200 字，请勿填写密码、令牌或密钥。'">
        <AInput v-model:value="state.reason.value" aria-label="变更原因" :maxlength="400" autocomplete="off" />
      </AFormItem>
      <ASpace wrap>
        <AButton type="primary" html-type="submit" :disabled="!state.changed.value" :loading="state.saving.value">核对变更</AButton>
        <AButton :disabled="state.saving.value" @click="cancel">取消编辑</AButton>
      </ASpace>
    </AForm>
    <ASpace v-else wrap>
      <AButton v-if="state.canEdit.value" data-scheduling-edit @click="edit">编辑平台并发策略</AButton>
      <AButton :loading="state.loading.value" :disabled="state.saving.value" @click="state.read">重新读取</AButton>
    </ASpace>
    <AModal :open="Boolean(state.review.value)" title="核对平台并发上限" :closable="!state.saving.value" :mask-closable="false" :keyboard="!state.saving.value" @cancel="state.cancelReview">
      <div v-if="state.review.value" class="scheduling-policy-review">
        <ADescriptions :column="1">
          <ADescriptionsItem label="上限变化">{{ state.review.value.beforeMaximum === null ? '尚未配置' : `${state.review.value.beforeMaximum} 个执行` }} → {{ state.review.value.payload.maxConcurrentExecutions }} 个执行</ADescriptionsItem>
          <ADescriptionsItem label="变更原因">{{ state.review.value.payload.reason }}</ADescriptionsItem>
        </ADescriptions>
        <AAlert :type="state.review.value.payload.confirmed ? 'warning' : 'info'" show-icon :message="state.review.value.payload.confirmed ? '首次设置或降低上限，可能使新的执行等待。' : '后续新执行领取使用提高后的上限。'" description="当前运行及待核对执行不取消、不改派；释放仍需可靠进程证据，容量恢复不代表其他领取条件已经通过。" />
        <ACheckbox v-if="state.review.value.payload.confirmed" v-model:checked="state.confirmed.value" :disabled="state.saving.value">我已了解收紧上限只阻止新增执行，不取消已有任务</ACheckbox>
      </div>
      <template #footer>
        <AButton :disabled="state.saving.value" @click="state.cancelReview">返回修改</AButton>
        <AButton type="primary" :danger="state.review.value?.payload.confirmed" :loading="state.saving.value" :disabled="Boolean(state.review.value?.payload.confirmed && !state.confirmed.value)" @click="state.save">确认保存</AButton>
      </template>
    </AModal>
    <AModal :open="leaveOpen" title="放弃未保存的调度设置？" :mask-closable="false" @cancel="resolveLeave(false)">
      <p>离开将放弃当前输入；已经发往服务端的请求不能通过离开撤回。</p>
      <template #footer><AButton @click="resolveLeave(false)">继续编辑</AButton><AButton danger type="primary" @click="resolveLeave(true)">放弃并离开</AButton></template>
    </AModal>
  </section>
</template>

<style scoped>
.scheduling-policy-settings, .scheduling-policy-review { display: grid; gap: var(--ob-foundation-space-4); min-inline-size: 0; overflow-wrap: anywhere; }
.scheduling-policy-settings h3 { margin: 0; color: var(--ob-color-form-text); font-size: var(--ob-component-control-font-size); }
</style>
