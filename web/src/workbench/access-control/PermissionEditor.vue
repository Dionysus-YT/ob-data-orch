<script setup lang="ts">
import { computed } from 'vue'
import { Alert as AAlert, Button as AButton, Checkbox as ACheckbox, Drawer as ADrawer, Form as AForm, FormItem as AFormItem, Input as AInput, Spin as ASpin, Tabs as ATabs, TabPane as ATabPane, Tag as ATag } from 'ant-design-vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { permissionCapabilities, permissionRoles, permissionScopeTypes, type AccessControlApi, type PermissionCapability, type PermissionRole, type PermissionScopeType } from '@/api/accessControl'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import { product } from '@/platform/tokens'
import { accountLabels, capabilityLabels, capabilityRoleAllowed, roleDescriptions, roleLabels, scopeAdditionGated, scopeLabels, scopeRoleAllowed } from './permissionRules'
import type { PermissionEditor } from './usePermissionEditor'
import PermissionScopePicker from './PermissionScopePicker.vue'

const props = defineProps<{ model: PermissionEditor; api: AccessControlApi }>()
const emit = defineEmits<{ denied: [session: number]; closed: [] }>()
const m = props.model
const { heading, afterOpenChange } = useAntDrawerDialog('permission-editor-title', m.close, () => m.saving.value, () => m.open.value)
void heading
function drawerChanged(open: boolean) { afterOpenChange(open); if (!open && !m.open.value) emit('closed') }
const blocked = computed(() => m.readOnly.value || m.loading.value || m.saving.value || Boolean(m.review.value))
const active = computed(() => m.profile.value?.subject.accountStatus === 'ACTIVE')
const ids = (type: PermissionScopeType) => m.form.scopes.filter(item => item.type === type).map(item => item.objectId)
const baseline = (type: PermissionScopeType) => m.profile.value?.assignment.scopes.filter(item => item.type === type).map(item => item.objectId) ?? []
function toggleRole(role: PermissionRole, checked: boolean) { m.setRoles(checked ? [...m.form.roles, role] : m.form.roles.filter(item => item !== role)) }
function toggleCapability(capability: PermissionCapability, checked: boolean) { m.setCapabilities(checked ? [...m.form.capabilities, capability] : m.form.capabilities.filter(item => item !== capability)) }
const changes = computed(() => {
  const diff = m.diff.value
  if (!diff) return []
  return [
    ...diff.addedRoles.map(value => ({ key: `+${value}`, action: '新增', label: roleLabels[value] })),
    ...diff.removedRoles.map(value => ({ key: `-${value}`, action: '移除', label: roleLabels[value] })),
    ...diff.addedScopes.map(value => ({ key: `+${value.type}:${value.objectId}`, action: '新增', label: `${scopeLabels[value.type]}：${m.scopeName(value.type, value.objectId)}` })),
    ...diff.removedScopes.map(value => ({ key: `-${value.type}:${value.objectId}`, action: '移除', label: `${scopeLabels[value.type]}：${m.scopeName(value.type, value.objectId)}` })),
    ...diff.addedCapabilities.map(value => ({ key: `+${value}`, action: '新增', label: capabilityLabels[value] })),
    ...diff.removedCapabilities.map(value => ({ key: `-${value}`, action: '移除', label: capabilityLabels[value] })),
  ]
})
</script>

<template>
  <ADrawer :open="m.open.value" :width="`min(${product.inspector.width}px, 100vw)`" :mask-closable="!m.saving.value" :keyboard="!m.saving.value" root-class-name="orch-inspector" class="orch-ui permission-editor" @close="m.close" @after-open-change="drawerChanged">
    <template #title><h2 id="permission-editor-title" ref="heading" class="ant-drawer-title" tabindex="-1">{{ m.profile.value && !m.profile.value.editable ? '查看权限' : '配置权限' }}</h2></template>
    <ASpin :spinning="m.loading.value">
      <div class="permission-editor-body">
        <AAlert v-if="m.failure.value" type="error" show-icon :message="m.failure.value"><template #action><AButton :disabled="m.loading.value || m.saving.value" @click="m.refresh">重试读取</AButton></template></AAlert>
        <template v-if="m.profile.value">
          <div class="permission-subject"><div><strong>{{ m.profile.value.subject.displayName }}</strong><p>{{ m.profile.value.subject.login }}</p></div><ATag>{{ accountLabels[m.profile.value.subject.accountStatus] }}</ATag><span>版本 {{ m.profile.value.revision }}</span></div>
          <AAlert v-if="!m.profile.value.editable" type="info" show-icon message="当前登录用户的权限只读，不能在此修改自身授权。" />
          <AAlert v-else-if="!active" type="warning" show-icon message="账号未启用或状态未知，只能撤销已有授权。" />
          <AAlert v-if="m.stale.value" type="warning" show-icon message="当前资料需要重新读取。已保留草稿，刷新后重新比较，不会覆盖其他修改。" />
          <AForm layout="vertical" @submit.prevent>
            <ATabs default-active-key="roles">
              <ATabPane key="roles" tab="固定角色">
                <p class="permission-description">角色决定职责，对象范围决定作用对象。只配置角色、未指定范围时，不会取得对应对象访问权。</p>
                <div class="permission-role-list"><div v-for="role in permissionRoles" :key="role" class="permission-role"><ACheckbox :checked="m.form.roles.includes(role)" :disabled="blocked || !active && !m.profile.value.assignment.roles.includes(role)" @change="event => toggleRole(role, event.target.checked)">{{ roleLabels[role] }}</ACheckbox><p>{{ roleDescriptions[role] }}</p></div></div>
              </ATabPane>
              <ATabPane key="scopes" tab="对象范围">
                <p class="permission-description">选择具体对象；任务运维范围按数据源关联任务，不代表任务数量。生产范围还需要已有生产任务能力。</p>
                <AFormItem v-for="type in permissionScopeTypes" :key="type" :label="scopeLabels[type]" :html-for="`permission-scope-${type}`" :extra="scopeAdditionGated(type) ? '本批只保留或撤销旧记录，尚未开放新增。' : !scopeRoleAllowed(m.form.roles, type) ? '需先选择对应职责角色。' : '空范围表示当前职责尚无可访问对象。'">
                  <PermissionScopePicker :api="api" :type="type" :session="m.session.value" :value="ids(type)" :baseline="baseline(type)" :objects="m.selectedObjects.value" :disabled="blocked || !scopeRoleAllowed(m.form.roles, type)" :allow-add="active && !scopeAdditionGated(type)" :production-allowed="m.form.capabilities.includes('CAP_PRODUCTION_TASK')" @change="values => m.setScopes(type, values)" @facts="m.rememberObjects" @denied="session => emit('denied', session)" />
                </AFormItem>
              </ATabPane>
              <ATabPane key="capabilities" tab="独立能力">
                <AAlert type="info" show-icon message="审计访问可显式授予，新增时须确认影响；当前操作者自身只读。其他能力仅保留或撤销，仍受各自准入约束。" />
                <div class="permission-role-list"><div v-for="capability in permissionCapabilities" :key="capability" class="permission-role"><ACheckbox :checked="m.form.capabilities.includes(capability)" :disabled="blocked || (!active || capability !== 'CAP_AUDIT_LOG') && !m.profile.value.assignment.capabilities.includes(capability) || !capabilityRoleAllowed(m.form.roles, capability)" @change="event => toggleCapability(capability, event.target.checked)">{{ capabilityLabels[capability] }}</ACheckbox></div></div>
              </ATabPane>
            </ATabs>
            <AAlert v-if="m.configError.value" type="error" show-icon :message="m.configError.value" />
            <AFormItem v-if="m.profile.value.editable" label="变更原因" required :validate-status="m.reasonError.value ? 'error' : undefined" :help="m.reasonError.value || '填写职责和范围调整说明，不填写密码、令牌或连接材料。'">
              <AInput.TextArea v-model:value="m.form.reason" aria-label="变更原因" :disabled="blocked" :auto-size="{ minRows: 2, maxRows: 5 }" placeholder="例如：调整日常维护职责与负责的数据源" />
            </AFormItem>
          </AForm>
        </template>
        <p v-else-if="m.loading.value">正在读取权限资料…</p>
      </div>
    </ASpin>
    <template #footer><div class="permission-editor-footer"><AButton :disabled="m.saving.value" @click="m.close">关闭</AButton><AButton v-if="!m.readOnly.value" :disabled="blocked || !m.dirty.value" @click="m.restore">还原草稿</AButton><AButton v-if="!m.readOnly.value" type="primary" :disabled="blocked || !m.changed.value" @click="m.startReview">比较并保存</AButton></div></template>
  </ADrawer>
  <OrchDangerConfirm :open="Boolean(m.review.value)" title="确认权限变更" :busy="m.saving.value" confirm-label="确认保存" cancel-label="返回编辑" @confirm="m.submitReview" @cancel="m.cancelReview">
    <div class="permission-review">
      <p>目标用户：<strong>{{ m.profile.value?.subject.displayName }}</strong>（{{ m.profile.value?.subject.login }}），版本 {{ m.review.value?.revision }}</p>
      <AAlert v-if="m.diff.value?.risk" type="warning" show-icon message="新增系统管理职责，请核对目标用户和全部权限差异。" />
      <AAlert v-if="m.diff.value?.productionRisk" type="warning" show-icon message="本次新增生产对象范围，请核对职责、对象和变更原因。" />
      <ul class="permission-diff"><li v-for="change in changes" :key="change.key"><ATag :color="change.action === '新增' ? 'blue' : 'red'">{{ change.action }}</ATag><span>{{ change.label }}</span></li></ul>
      <p>保留对象范围 {{ m.diff.value?.retainedScopes.length ?? 0 }} 项；未列出的角色和能力保持不变。</p>
      <p class="permission-reason">原因：{{ m.review.value?.payload.reason }}</p>
      <p>保存后后续请求按新权限校验；未提交草稿需要重新预检查，已运行任务继续执行。</p>
    </div>
  </OrchDangerConfirm>
  <OrchDangerConfirm :open="m.discardOpen.value" title="放弃未保存的权限修改？" confirm-label="放弃修改" cancel-label="继续编辑" @confirm="m.confirmDiscard" @cancel="m.cancelDiscard"><p>当前草稿尚未保存，放弃后不会改变用户权限。</p></OrchDangerConfirm>
</template>

<style scoped>
.permission-editor-body, .permission-review { display: grid; gap: var(--ob-foundation-space-4); min-inline-size: 0; }
.permission-subject { display: flex; align-items: flex-start; flex-wrap: wrap; gap: var(--ob-foundation-space-3); overflow-wrap: anywhere; }
.permission-subject > div { flex: 1; min-inline-size: 0; }
.permission-subject p, .permission-role p { margin: var(--ob-foundation-space-1) 0 0; color: var(--ob-color-secondary); }
.permission-description { margin: 0 0 var(--ob-foundation-space-4); color: var(--ob-color-secondary); }
.permission-role-list { display: grid; gap: var(--ob-foundation-space-4); margin-block: var(--ob-foundation-space-4); }
.permission-role p { padding-inline-start: var(--ob-foundation-space-6); }
.permission-editor-footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.permission-diff { max-block-size: 40vh; overflow: auto; list-style: none; padding: 0; margin: 0; display: grid; gap: var(--ob-foundation-space-2); }
.permission-diff li { display: flex; align-items: flex-start; gap: var(--ob-foundation-space-2); overflow-wrap: anywhere; }
.permission-reason { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
