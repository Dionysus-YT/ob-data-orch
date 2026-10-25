<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Checkbox as ACheckbox, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Form as AForm, FormItem as AFormItem, Input as AInput, InputNumber as AInputNumber, Modal as AModal, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogRetention } from './useLogRetention'

const { MessageContext, success, clear } = useOperationMessage()
const root = ref<HTMLElement>()
let focusVersion = 0
const state = useLogRetention(browserApi(), { onSaved: text => { success(text); void focusControl('[data-retention-edit]') }, clearNotice: clear })
const sourceNames = { PLATFORM_SETTING: '平台设置', DEPLOYMENT_CONFIG: '部署配置', BUILT_IN_DEFAULT: '内置默认' }
const route = useRoute()
watch(() => route.fullPath, () => { focusVersion++; state.reset(); void state.read() }, { flush: 'sync' })
async function focusControl(selector: string) { const expected = ++focusVersion; await nextTick(); if (expected === focusVersion) root.value?.querySelector<HTMLElement>(selector)?.focus() }
function edit() { state.edit(); if (state.editing.value) void focusControl('input[aria-label="普通日志保留天数"]') }
function cancel() { state.cancel(); if (!state.editing.value) void focusControl('[data-retention-edit]') }
function review() { state.reviewChange(); if (state.daysError.value || state.reasonError.value) void focusControl(state.daysError.value ? 'input[aria-label="普通日志保留天数"]' : 'input[aria-label="变更原因"]') }
onScopeDispose(() => { focusVersion++ })
onMounted(() => { void state.read() })
</script>

<template>
  <section ref="root" class="log-retention-settings" aria-labelledby="ordinary-log-retention-title" :aria-busy="state.loading.value || state.saving.value">
    <MessageContext />
    <h3 id="ordinary-log-retention-title">普通任务日志</h3>
    <AAlert type="info" show-icon message="按任务结束时间保留普通日志" description="仅清理满足条件的终态任务脱敏日志。保存从后续清理批次生效，无需重启；延长期限不能恢复已删除或撤回已登记清理的日志。" />
    <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取当前策略…</div>
    <ADescriptions v-if="state.policy.value" :column="1">
      <ADescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取期限' : '当前生效期限'">{{ state.policy.value.retentionDays }} 天<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span></ADescriptionsItem>
      <ADescriptionsItem label="配置来源">{{ sourceNames[state.policy.value.source] }}</ADescriptionsItem>
      <ADescriptionsItem label="生效方式">后续清理批次使用最新策略 · 无需重启</ADescriptionsItem>
      <ADescriptionsItem label="最近修改">{{ state.policy.value.modifiedAt ? `${state.policy.value.modifiedAt} · ${state.policy.value.modifiedBy}` : '尚无平台修改记录' }}</ADescriptionsItem>
      <ADescriptionsItem label="配置版本">{{ state.policy.value.revision }}</ADescriptionsItem>
    </ADescriptions>
    <AForm v-if="state.editing.value" :model="{ retentionDays: state.days.value, reason: state.reason.value }" layout="vertical" :disabled="state.saving.value || Boolean(state.review.value)" @finish="review">
      <AFormItem label="普通日志保留天数" name="retentionDays" :validate-status="state.daysError.value ? 'error' : undefined" :help="state.daysError.value || '7–365 天；未配置时采用 30 天。'">
        <AInputNumber :value="state.days.value ?? undefined" aria-label="普通日志保留天数" :min="7" :max="365" :precision="0" :step="1" @change="state.days.value = typeof $event === 'number' ? $event : null" />
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
      <AButton v-if="state.canEdit.value" data-retention-edit @click="edit">编辑普通日志策略</AButton>
      <AButton :loading="state.loading.value" :disabled="state.saving.value" @click="state.read">重新读取</AButton>
    </ASpace>
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
  </section>
</template>

<style scoped>
.log-retention-settings, .log-retention-review { display: grid; gap: var(--ob-foundation-space-4); min-inline-size: 0; overflow-wrap: anywhere; }
.log-retention-settings h3 { margin: 0; color: var(--ob-color-form-text); font-size: var(--ob-component-control-font-size); }
</style>
