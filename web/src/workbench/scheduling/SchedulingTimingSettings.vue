<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Checkbox as ACheckbox, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Form as AForm, FormItem as AFormItem, Input as AInput, InputNumber as AInputNumber, Modal as AModal, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import { useSchedulingTiming } from './useSchedulingTiming'

const root = ref<HTMLElement>(), route = useRoute()
const { MessageContext, success, clear } = useOperationMessage()
let focusVersion = 0
const state = useSchedulingTiming(browserApi(), { onSaved: text => { success(text); void focusControl('[data-timing-edit]') }, clearNotice: clear })
const leaveOpen = ref(false)
let pendingLeave: ((allowed: boolean) => void) | undefined
function canLeave(): boolean | Promise<boolean> { if (state.saving.value || pendingLeave) return false; if (!state.dirty.value) return true; state.cancelReview(); leaveOpen.value = true; return new Promise(resolve => { pendingLeave = resolve }) }
function resolveLeave(allowed: boolean) { const resolve = pendingLeave; pendingLeave = undefined; leaveOpen.value = false; if (allowed) state.cancel(); resolve?.(allowed) }
function beforeUnload(event: BeforeUnloadEvent) { if (state.dirty.value || state.saving.value) event.preventDefault() }
onBeforeRouteLeave(canLeave); onBeforeRouteUpdate(canLeave); useLogoutGuard(canLeave)
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
watch(() => route.fullPath, () => { focusVersion++; state.reset(); void state.read() }, { flush: 'sync' })
async function focusControl(selector: string) { const expected = ++focusVersion; await nextTick(); if (expected === focusVersion) root.value?.querySelector<HTMLElement>(selector)?.focus() }
function edit() { state.edit(); if (state.editing.value) void focusControl('input[aria-label="节点心跳超时"]') }
function cancel() { state.cancel(); if (!state.editing.value) void focusControl('[data-timing-edit]') }
// 只读表单从策略派生值，编辑会话才接收数值，保存与确认期间冻结输入。
function changeNumber(field: 'heartbeatTimeoutSeconds' | 'startupTimeoutSeconds' | 'pollingIntervalSeconds', value: unknown) {
  if (state.editing.value && !state.saving.value && !state.review.value) state.form[field] = typeof value === 'number' ? value : null
}
function review() { state.reviewChange(); const field = (['heartbeatTimeoutSeconds', 'startupTimeoutSeconds', 'pollingIntervalSeconds', 'reason'] as const).find(name => state.errors[name]); if (field) void focusControl(`[data-timing-field="${field}"] input`) }
function seconds(value: number | null, empty: string) { return value === null ? empty : `${value} 秒` }
onScopeDispose(() => { focusVersion++; window.removeEventListener('beforeunload', beforeUnload); resolveLeave(false) })
onMounted(() => { void state.read() })
</script>

<template>
  <section ref="root" class="scheduling-timing-settings settings-form-block" aria-labelledby="scheduling-time-policy-title" :aria-busy="state.loading.value || state.saving.value">
    <MessageContext />
    <div class="settings-control-header">
      <h3 id="scheduling-time-policy-title">超时与检查间隔</h3>
      <ASpace v-if="!state.editing.value" wrap><AButton v-if="state.canEdit.value" data-timing-edit @click="edit">编辑时间策略</AButton><AButton type="text" :loading="state.loading.value" :disabled="state.saving.value" @click="state.read">刷新时间策略</AButton></ASpace>
    </div>
    <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取调度时间策略…</div>
    <AForm v-if="state.policy.value" :model="state.form" layout="vertical" :disabled="state.saving.value || Boolean(state.review.value)" @finish="review">
      <div class="settings-field-grid">
        <AFormItem data-timing-field="heartbeatTimeoutSeconds" :label="!state.editing.value && (state.stale.value || state.unconfirmed.value) ? '最近读取的心跳超时（待核对）' : '节点心跳超时'" name="heartbeatTimeoutSeconds" :validate-status="state.errors.heartbeatTimeoutSeconds ? 'error' : undefined" :help="state.errors.heartbeatTimeoutSeconds || (state.editing.value ? '60–3600 秒，用于当前离线判定。' : undefined)" :extra="state.editing.value ? '应大于 Agent 的实际心跳间隔。' : undefined">
          <AInputNumber class="settings-number" :value="state.editing.value ? state.form.heartbeatTimeoutSeconds ?? undefined : state.policy.value.heartbeatTimeoutSeconds" aria-label="节点心跳超时" addon-after="秒" :readonly="!state.editing.value" :controls="state.editing.value" :min="60" :max="3600" :precision="0" :step="1" @change="changeNumber('heartbeatTimeoutSeconds', $event)" />
        </AFormItem>
        <AFormItem data-timing-field="startupTimeoutSeconds" :label="!state.editing.value && (state.stale.value || state.unconfirmed.value) ? '最近读取的启动期限（待核对）' : '任务启动超时'" name="startupTimeoutSeconds" :validate-status="state.errors.startupTimeoutSeconds ? 'error' : undefined" :help="state.errors.startupTimeoutSeconds || (state.editing.value ? '15–3600 秒，仅影响后续新领取。' : undefined)" :extra="state.editing.value ? '未配置时可留空；保存后不能清空重置。' : state.policy.value.startupTimeoutSeconds === null ? '未配置额外启动期限' : undefined">
          <AInputNumber class="settings-number" :value="state.editing.value ? state.form.startupTimeoutSeconds ?? undefined : state.policy.value.startupTimeoutSeconds ?? undefined" aria-label="任务启动超时" placeholder="未配置" addon-after="秒" :readonly="!state.editing.value" :controls="state.editing.value" :min="15" :max="3600" :precision="0" :step="1" @change="changeNumber('startupTimeoutSeconds', $event)" />
        </AFormItem>
        <AFormItem data-timing-field="pollingIntervalSeconds" :label="!state.editing.value && (state.stale.value || state.unconfirmed.value) ? '最近读取的检查间隔（待核对）' : '领取检查间隔'" name="pollingIntervalSeconds" :validate-status="state.errors.pollingIntervalSeconds ? 'error' : undefined" :help="state.errors.pollingIntervalSeconds || (state.editing.value ? '1–60 秒，仅限制控制面的检查。' : undefined)" :extra="state.editing.value ? '未配置时可留空；保存后不能清空，不改变 Agent 网络周期。' : state.policy.value.pollingIntervalSeconds === null ? '未配置额外检查间隔' : undefined">
          <AInputNumber class="settings-number" :value="state.editing.value ? state.form.pollingIntervalSeconds ?? undefined : state.policy.value.pollingIntervalSeconds ?? undefined" aria-label="领取检查间隔" placeholder="未配置" addon-after="秒" :readonly="!state.editing.value" :controls="state.editing.value" :min="1" :max="60" :precision="0" :step="1" @change="changeNumber('pollingIntervalSeconds', $event)" />
        </AFormItem>
      </div>
      <AFormItem v-if="state.editing.value" data-timing-field="reason" label="时间策略变更原因" name="reason" :validate-status="state.errors.reason ? 'error' : undefined" :help="state.errors.reason || '1–200 字，请勿填写密码、令牌或密钥。'">
        <AInput v-model:value="state.form.reason" aria-label="时间策略变更原因" placeholder="说明本次调整的原因" :maxlength="400" autocomplete="off" />
      </AFormItem>
      <ASpace v-if="state.editing.value" wrap><AButton type="primary" html-type="submit" :disabled="!state.changed.value" :loading="state.saving.value">核对时间变更</AButton><AButton :disabled="state.saving.value" @click="cancel">取消时间编辑</AButton></ASpace>
    </AForm>
    <ACollapse v-if="state.policy.value" ghost class="settings-details">
      <ACollapsePanel key="details" header="配置详情">
        <ADescriptions :column="1" size="small">
          <ADescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取的心跳超时' : '节点心跳超时'">{{ seconds(state.policy.value.heartbeatTimeoutSeconds, '') }}<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span> · {{ state.policy.value.heartbeatTimeoutSource === 'CONTROL_PLANE' ? '控制面启动配置' : '平台设置' }}</ADescriptionsItem>
          <ADescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取的启动期限' : '任务启动期限'">{{ seconds(state.policy.value.startupTimeoutSeconds, '未配置额外启动期限') }}<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span></ADescriptionsItem>
          <ADescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取的检查间隔' : '领取检查间隔'">{{ seconds(state.policy.value.pollingIntervalSeconds, '未配置额外检查间隔') }}<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span></ADescriptionsItem>
          <ADescriptionsItem label="时间策略最近修改">{{ state.policy.value.modifiedAt ? `${state.policy.value.modifiedAt} · ${state.policy.value.modifiedBy}` : '尚无平台修改记录' }}</ADescriptionsItem>
          <ADescriptionsItem label="时间策略版本">{{ state.policy.value.revision }} · 无需重启</ADescriptionsItem>
          <ADescriptionsItem label="判定边界">超时或失联只附加待核对，不合成失败、不取消任务或释放预算，也不延长执行租约；Agent 网络周期保持本机规则。</ADescriptionsItem>
        </ADescriptions>
      </ACollapsePanel>
    </ACollapse>
    <AModal :open="Boolean(state.review.value)" title="核对调度时间策略" :closable="!state.saving.value" :mask-closable="false" :keyboard="!state.saving.value" @cancel="state.cancelReview">
      <div v-if="state.review.value" class="scheduling-timing-review">
        <ADescriptions :column="1">
          <ADescriptionsItem label="心跳超时">{{ seconds(state.review.value.before.heartbeatTimeoutSeconds, '') }} → {{ seconds(state.review.value.payload.heartbeatTimeoutSeconds, '') }}</ADescriptionsItem>
          <ADescriptionsItem label="启动期限">{{ seconds(state.review.value.before.startupTimeoutSeconds, '未配置') }} → {{ seconds(state.review.value.payload.startupTimeoutSeconds, '未配置') }}</ADescriptionsItem>
          <ADescriptionsItem label="检查间隔">{{ seconds(state.review.value.before.pollingIntervalSeconds, '未配置') }} → {{ seconds(state.review.value.payload.pollingIntervalSeconds, '未配置') }}</ADescriptionsItem>
          <ADescriptionsItem label="变更原因">{{ state.review.value.payload.reason }}</ADescriptionsItem>
        </ADescriptions>
        <AAlert :type="state.review.value.payload.confirmed ? 'warning' : 'info'" show-icon message="各项按独立生效范围执行" description="心跳阈值用于下一次判定；启动期限只冻结到新领取；检查间隔用于下一次队列检查。既有执行不取消，超时待核对仍占用预算，Agent 网络周期不变。" />
        <ACheckbox v-if="state.review.value.payload.confirmed" v-model:checked="state.confirmed.value" :disabled="state.saving.value">我已了解离线判定、启动核对及检查频率的影响</ACheckbox>
      </div>
      <template #footer><AButton :disabled="state.saving.value" @click="state.cancelReview">返回修改</AButton><AButton type="primary" :danger="state.review.value?.payload.confirmed" :loading="state.saving.value" :disabled="Boolean(state.review.value?.payload.confirmed && !state.confirmed.value)" @click="state.save">确认保存</AButton></template>
    </AModal>
    <AModal :open="leaveOpen" title="放弃未保存的时间设置？" :mask-closable="false" @cancel="resolveLeave(false)">
      <p>离开将放弃当前时间设置输入；已经发往服务端的请求不能通过离开撤回。</p>
      <template #footer><AButton @click="resolveLeave(false)">继续编辑</AButton><AButton danger type="primary" @click="resolveLeave(true)">放弃并离开</AButton></template>
    </AModal>
  </section>
</template>

<style scoped>
.scheduling-timing-settings, .scheduling-timing-review { display: grid; gap: var(--ob-foundation-space-4); min-inline-size: 0; overflow-wrap: anywhere; }
</style>
