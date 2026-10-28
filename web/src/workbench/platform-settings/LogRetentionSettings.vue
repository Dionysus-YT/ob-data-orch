<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Checkbox as ACheckbox, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Form as AForm, FormItem as AFormItem, Input as AInput, InputNumber as AInputNumber, Modal as AModal, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import { useLogRetention } from './useLogRetention'

const { MessageContext, success, clear } = useOperationMessage()
const root = ref<HTMLElement>()
let focusVersion = 0
const state = useLogRetention(browserApi(), { onSaved: text => { success(text); void focusControl('[data-retention-edit]') }, clearNotice: clear })
const sourceNames = { PLATFORM_SETTING: '平台设置', DEPLOYMENT_CONFIG: '部署配置', BUILT_IN_DEFAULT: '内置默认' }
const route = useRoute()
const leaveOpen = ref(false)
let pendingLeave: ((allowed: boolean) => void) | undefined
// 页签切换与其他离开共用当前事务的保护；保存中不允许卸载写会话。
function canLeave(): boolean | Promise<boolean> {
  if (state.saving.value || pendingLeave) return false
  if (!state.dirty.value) return true
  state.cancelReview(); leaveOpen.value = true
  return new Promise(resolve => { pendingLeave = resolve })
}
function resolveLeave(allowed: boolean) { const resolve = pendingLeave; pendingLeave = undefined; leaveOpen.value = false; if (allowed) state.cancel(); resolve?.(allowed) }
function beforeUnload(event: BeforeUnloadEvent) { if (state.dirty.value || state.saving.value) event.preventDefault() }
onBeforeRouteLeave(canLeave); onBeforeRouteUpdate(canLeave); useLogoutGuard(canLeave)
watch(() => route.fullPath, () => { focusVersion++; state.reset(); void state.read() }, { flush: 'sync' })
async function focusControl(selector: string) { const expected = ++focusVersion; await nextTick(); if (expected === focusVersion) root.value?.querySelector<HTMLElement>(selector)?.focus() }
function edit() { state.edit(); if (state.editing.value) void focusControl('input[aria-label="普通日志保留天数"]') }
function cancel() { state.cancel(); if (!state.editing.value) void focusControl('[data-retention-edit]') }
// 展示态只读取服务端事实，控件变化仅交给当前可编辑会话。
function changeDays(value: unknown) { if (state.editing.value && !state.saving.value && !state.review.value) state.days.value = typeof value === 'number' ? value : null }
function review() { state.reviewChange(); if (state.daysError.value || state.reasonError.value) void focusControl(state.daysError.value ? 'input[aria-label="普通日志保留天数"]' : 'input[aria-label="变更原因"]') }
onScopeDispose(() => { focusVersion++; window.removeEventListener('beforeunload', beforeUnload); resolveLeave(false) })
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void state.read() })
</script>

<template>
  <section ref="root" class="log-retention-settings settings-form-block" aria-labelledby="ordinary-log-retention-title" :aria-busy="state.loading.value || state.saving.value">
    <MessageContext />
    <div class="settings-control-header">
      <h3 id="ordinary-log-retention-title">普通任务日志</h3>
      <ASpace v-if="!state.editing.value" wrap><AButton v-if="state.canEdit.value" data-retention-edit @click="edit">编辑普通日志策略</AButton><AButton type="text" :loading="state.loading.value" :disabled="state.saving.value" @click="state.read">重新读取</AButton></ASpace>
    </div>
    <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取当前策略…</div>
    <AForm v-if="state.policy.value" :model="{ retentionDays: state.editing.value ? state.days.value : state.policy.value.retentionDays, reason: state.reason.value }" layout="vertical" :disabled="state.saving.value || Boolean(state.review.value)" @finish="review">
      <AFormItem :label="!state.editing.value && (state.stale.value || state.unconfirmed.value) ? '最近读取期限（当前值待核对）' : '普通日志保留天数'" name="retentionDays" :validate-status="state.daysError.value ? 'error' : undefined" :help="state.daysError.value || '7–365 天，按任务结束时间保留；后续清理批次生效，无需重启。'" extra="已删除或已登记清理的普通日志无法恢复，不影响任务记录和审计日志。">
        <AInputNumber class="settings-number" :value="state.editing.value ? state.days.value ?? undefined : state.policy.value.retentionDays" aria-label="普通日志保留天数" addon-after="天" :readonly="!state.editing.value" :controls="state.editing.value" :min="7" :max="365" :precision="0" :step="1" @change="changeDays" />
      </AFormItem>
      <AFormItem v-if="state.editing.value" label="变更原因" name="reason" :validate-status="state.reasonError.value ? 'error' : undefined" :help="state.reasonError.value || '1–200 字，请勿填写密码、令牌或密钥。'">
        <AInput v-model:value="state.reason.value" aria-label="变更原因" placeholder="说明本次调整的原因" :maxlength="400" autocomplete="off" />
      </AFormItem>
      <ASpace v-if="state.editing.value" wrap>
        <AButton type="primary" html-type="submit" :disabled="!state.changed.value" :loading="state.saving.value">核对变更</AButton>
        <AButton :disabled="state.saving.value" @click="cancel">取消编辑</AButton>
      </ASpace>
    </AForm>
    <ACollapse v-if="state.policy.value" ghost class="settings-details">
      <ACollapsePanel key="details" header="配置详情">
        <ADescriptions :column="1" size="small">
          <ADescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取期限' : '当前生效期限'">{{ state.policy.value.retentionDays }} 天<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span></ADescriptionsItem>
          <ADescriptionsItem label="配置来源">{{ sourceNames[state.policy.value.source] }}</ADescriptionsItem>
          <ADescriptionsItem label="最近修改">{{ state.policy.value.modifiedAt ? `${state.policy.value.modifiedAt} · ${state.policy.value.modifiedBy}` : '尚无平台修改记录' }}</ADescriptionsItem>
          <ADescriptionsItem label="配置版本">{{ state.policy.value.revision }}</ADescriptionsItem>
        </ADescriptions>
      </ACollapsePanel>
    </ACollapse>
    <AModal :open="Boolean(state.review.value)" title="核对普通日志保留策略" :closable="!state.saving.value" :mask-closable="false" :keyboard="!state.saving.value" @cancel="state.cancelReview">
      <div v-if="state.review.value" class="log-retention-review">
        <ADescriptions :column="1">
          <ADescriptionsItem label="期限变化">{{ state.review.value.beforeDays }} 天 → {{ state.review.value.payload.retentionDays }} 天</ADescriptionsItem>
          <ADescriptionsItem label="变更原因">{{ state.review.value.payload.reason }}</ADescriptionsItem>
        </ADescriptions>
        <AAlert :type="state.review.value.payload.confirmed ? 'warning' : 'info'" show-icon :message="state.review.value.payload.confirmed ? '缩短期限会使更多已结束任务的普通日志进入后续清理范围。' : '后续清理批次使用延长后的期限。'" description="不修改任务记录或审计日志。已删除或已登记清理的日志无法通过修改期限恢复。" />
        <ACheckbox v-if="state.review.value.payload.confirmed" v-model:checked="state.confirmed.value" :disabled="state.saving.value">我已了解缩短期限的清理影响</ACheckbox>
      </div>
      <template #footer>
        <AButton :disabled="state.saving.value" @click="state.cancelReview">返回修改</AButton>
        <AButton type="primary" :danger="state.review.value?.payload.confirmed" :loading="state.saving.value" :disabled="Boolean(state.review.value?.payload.confirmed && !state.confirmed.value)" @click="state.save">确认保存</AButton>
      </template>
    </AModal>
    <AModal :open="leaveOpen" title="放弃未保存的普通日志设置？" :mask-closable="false" @cancel="resolveLeave(false)">
      <p>离开将放弃当前日志策略输入；已经发往服务端的请求不能通过离开撤回。</p>
      <template #footer><AButton @click="resolveLeave(false)">继续编辑</AButton><AButton danger type="primary" @click="resolveLeave(true)">放弃并离开</AButton></template>
    </AModal>
  </section>
</template>

<style scoped>
.log-retention-settings, .log-retention-review { display: grid; gap: var(--ob-foundation-space-4); min-inline-size: 0; overflow-wrap: anywhere; }
</style>
