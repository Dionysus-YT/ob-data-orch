<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute } from 'vue-router'
import { Alert, Button, Checkbox, Descriptions, DescriptionsItem, Form, FormItem, Input, Modal, Select, Space, Spin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import { candidateOrderNames, useSchedulingCandidates } from './useSchedulingCandidates'

const { MessageContext, success, clear } = useOperationMessage(), root = ref<HTMLElement>(), route = useRoute()
let focusVersion = 0, pendingLeave: ((allowed: boolean) => void) | undefined
const leaveOpen = ref(false)
const state = useSchedulingCandidates(browserApi(), { onSaved: text => { success(text); void focusControl('[data-candidate-edit]') }, clearNotice: clear })
const choices = Object.entries(candidateOrderNames).map(([value, label]) => ({ value, label }))
async function focusControl(selector: string) { const expected = ++focusVersion; await nextTick(); if (expected === focusVersion) root.value?.querySelector<HTMLElement>(selector)?.focus() }
function edit() { state.edit(); if (state.editing.value) void focusControl('[role="combobox"]') }
function cancel() { state.cancel(); if (!state.editing.value) void focusControl('[data-candidate-edit]') }
function review() { state.reviewChange(); if (state.fieldError.value) void focusControl('input[aria-label="候选顺序变更原因"]') }
function canLeave(): boolean | Promise<boolean> { if (state.saving.value || pendingLeave) return false; if (!state.dirty.value) return true; state.cancelReview(); leaveOpen.value = true; return new Promise(resolve => { pendingLeave = resolve }) }
function resolveLeave(allowed: boolean) { const resolve = pendingLeave; pendingLeave = undefined; leaveOpen.value = false; if (allowed) state.cancel(); resolve?.(allowed) }
function beforeUnload(event: BeforeUnloadEvent) { if (state.dirty.value || state.saving.value) event.preventDefault() }
onBeforeRouteLeave(canLeave); onBeforeRouteUpdate(canLeave); useLogoutGuard(canLeave)
watch(() => route.fullPath, () => { focusVersion++; state.reset(); void state.read() }, { flush: 'sync' })
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void state.read() })
onScopeDispose(() => { focusVersion++; window.removeEventListener('beforeunload', beforeUnload); resolveLeave(false) })
</script>

<template>
  <section ref="root" class="scheduling-candidates" aria-labelledby="candidate-order-title" :aria-busy="state.loading.value || state.saving.value">
    <MessageContext />
    <h3 id="candidate-order-title">候选节点顺序</h3>
    <Alert type="info" show-icon message="始终由用户显式选择节点" description="顺序只在下次读取三类任务候选时生效，已有选择保持不变。在线优先依据当前心跳，不代表环境、预算或任务预检查已经通过；同组保留原顺序。" />
    <Alert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><Spin size="small" /> 正在读取候选顺序…</div>
    <Descriptions v-if="state.policy.value" :column="1">
      <DescriptionsItem :label="state.stale.value || state.unconfirmed.value ? '最近读取的候选顺序' : '当前候选顺序'">{{ candidateOrderNames[state.policy.value.order] }}<span v-if="state.stale.value || state.unconfirmed.value">（当前值待核对）</span> · {{ state.policy.value.modifiedAt ? '平台设置' : '内置基线' }}</DescriptionsItem>
      <DescriptionsItem label="节点选择方式">用户显式选择；不自动选中、提交或改派</DescriptionsItem>
      <DescriptionsItem label="候选策略最近修改">{{ state.policy.value.modifiedAt ? `${state.policy.value.modifiedAt} · ${state.policy.value.modifiedBy}` : '尚无平台修改记录' }}</DescriptionsItem>
      <DescriptionsItem label="候选策略版本">{{ state.policy.value.revision }} · 下次读取候选时生效，无需重启</DescriptionsItem>
    </Descriptions>
    <Form v-if="state.editing.value" :model="state.form" layout="vertical" :disabled="state.saving.value || Boolean(state.review.value)" @finish="review">
      <FormItem label="候选顺序" name="order"><Select v-model:value="state.form.order" aria-label="候选顺序" :options="choices" /></FormItem>
      <FormItem label="候选顺序变更原因" name="reason" :validate-status="state.fieldError.value ? 'error' : undefined" :help="state.fieldError.value || '1–200 字，请勿填写密码、令牌或密钥。'"><Input v-model:value="state.form.reason" aria-label="候选顺序变更原因" :maxlength="400" autocomplete="off" /></FormItem>
      <Space wrap><Button type="primary" html-type="submit" :disabled="!state.changed.value">核对候选顺序</Button><Button @click="cancel">取消候选编辑</Button></Space>
    </Form>
    <Space v-else wrap><Button v-if="state.canEdit.value" data-candidate-edit @click="edit">编辑候选顺序</Button><Button :loading="state.loading.value" :disabled="state.saving.value" @click="state.read">刷新候选顺序</Button></Space>
    <Modal :open="Boolean(state.review.value)" title="核对候选顺序" :mask-closable="false" :closable="!state.saving.value" :keyboard="!state.saving.value" @cancel="state.cancelReview">
      <div v-if="state.review.value" class="scheduling-candidate-review">
        <Descriptions :column="1"><DescriptionsItem label="候选顺序">{{ candidateOrderNames[state.review.value.before.order] }} → {{ candidateOrderNames[state.review.value.payload.order] }}</DescriptionsItem><DescriptionsItem label="变更原因">{{ state.review.value.payload.reason }}</DescriptionsItem></Descriptions>
        <Alert type="info" show-icon message="只改变候选顺序，保留已有选择" description="下次读取三类任务候选时应用；在线优先不保证可执行，提交与领取仍须核验权限、预检查、平台上限和节点预算。" />
        <Checkbox v-model:checked="state.confirmed.value" :disabled="state.saving.value">我已了解候选顺序变化不会自动选择或改派节点</Checkbox>
      </div>
      <template #footer><Button :disabled="state.saving.value" @click="state.cancelReview">返回修改</Button><Button type="primary" :loading="state.saving.value" :disabled="!state.confirmed.value" @click="state.save">确认保存</Button></template>
    </Modal>
    <Modal :open="leaveOpen" title="放弃未保存的候选顺序？" :mask-closable="false" @cancel="resolveLeave(false)"><p>离开将放弃当前候选顺序输入；已送达的保存请求仍须重新读取核对。</p><template #footer><Button @click="resolveLeave(false)">继续编辑</Button><Button danger type="primary" @click="resolveLeave(true)">放弃并离开</Button></template></Modal>
  </section>
</template>

<style scoped>
.scheduling-candidates, .scheduling-candidate-review { display: grid; gap: var(--ob-foundation-space-4); min-inline-size: 0; overflow-wrap: anywhere; }
.scheduling-candidates h3 { margin: 0; color: var(--ob-color-form-text); font-size: var(--ob-component-control-font-size); }
</style>
