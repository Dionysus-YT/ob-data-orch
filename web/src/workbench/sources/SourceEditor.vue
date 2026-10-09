<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Skeleton, Button as AButton, Textarea as ATextarea, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Checkbox as ACheckbox, Drawer as ADrawer, Form as AForm, FormItem as AFormItem, InputPassword as AInputPassword } from 'ant-design-vue'
import { computed, nextTick, ref, useId, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { DownOutlined, RightOutlined, DatabaseOutlined, ReloadOutlined, ApiOutlined } from '@ant-design/icons-vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import ConnectionTestResult from '@/workbench/sources/ConnectionTestResult.vue'
import { validateDataSourceForm, type DataSourceFormField } from '@/workbench/sources/dataSourceFormErrors'
import { dataSourceConnectionTestDiagnostic } from '@/workbench/sources/dataSourceConnectionTestDiagnostic'
import { dataSourceConnectionTestNotice, sysCredentialVerificationNotice } from '@/workbench/sources/dataSourceConnectionTestNotice'
import { antBadgeStatus, connectionFact, editorEnvironments, formatVerifiedTime } from './sourcePresentation'
import type { SourceGateway } from './sourceGateway'
import { useSourceEditor } from './useSourceEditor'
import { product } from '@/platform/tokens'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'

const props = defineProps<{ api: SourceGateway; sourceId: string | null; initialMode: 'MYSQL' | 'ORACLE'; focusTest?: boolean; preview: boolean }>()
const emit = defineEmits<{ close: []; saved: [id: string] }>()
const { source, form, clearSys, parserInput, errors, failure, feedback, loading, saving, testing, nodes, nodeLoading, nodeFailure, nodeId, test, isNew, dirty, busy, invalidated, testBlocked, load, loadNodes, parseConnection, save, startTest } = useSourceEditor(props.api, props.sourceId, (id) => emit('saved', id), props.initialMode)
const modeLabel = computed(() => form.compatibilityMode === 'ORACLE' ? 'OceanBase Oracle' : 'OceanBase MySQL')
const nameOpen = ref(false)
const nameDraft = ref('')
const nameInput = ref<{ focus: () => void }>()
const nodeSelect = ref<{ focus: () => void; $el: HTMLElement }>()
const testExpanded = ref(Boolean(props.focusTest))
const confirmClose = ref(false)
const advancedPanels = ref<string[]>([])
const formElement = ref<{ $el: HTMLFormElement }>()
const drawerTitleId = useId()
const { heading: drawerHeading, afterOpenChange: afterDrawerOpenChange } = useAntDrawerDialog(drawerTitleId, close, () => saving.value)
void drawerHeading
let resolveNavigation: ((leave: boolean) => void) | undefined
const fact = computed(() => testing.value ? connectionFact({ lastTestStatus: 'PENDING' }) : invalidated.value ? connectionFact({ lastTestStatus: 'INVALIDATED' }) : test.value ? connectionFact({ lastTestStatus: test.value.status, lastTestedAt: test.value.completedAt }) : connectionFact(source.value ?? {}))
const diagnostic = computed(() => test.value ? dataSourceConnectionTestDiagnostic(test.value) : undefined)

function close() {
  if (saving.value) return
  if (dirty.value || testing.value) { confirmClose.value = true; return }
  emit('close')
}
function cancelClose() { confirmClose.value = false; resolveNavigation?.(false); resolveNavigation = undefined }
function acceptClose() { confirmClose.value = false; if (resolveNavigation) { resolveNavigation(true); resolveNavigation = undefined } else emit('close') }
onBeforeRouteLeave(() => {
  if (saving.value) return false
  if (!dirty.value && !testing.value) return true
  confirmClose.value = true
  return new Promise<boolean>((resolve) => { resolveNavigation = resolve })
})
async function saveForm() {
  if (busy.value) return
  if (isNew.value) {
    // 名称在最后一步填写；此处只暂缓名称校验，提交仍经过完整校验。
    const checked = validateDataSourceForm(form, true)
    delete checked.displayName
    errors.value = checked
    if (Object.keys(checked).length) { await focusFirstError(); return }
    nameDraft.value = form.displayName
    failure.value = ''
    nameOpen.value = true
    await nextTick()
    nameInput.value?.focus()
    return
  }
  const success = await save()
  if (!success && Object.keys(errors.value).length) await focusFirstError()
}
function cancelName() {
  if (saving.value) return
  nameOpen.value = false
  delete errors.value.displayName
}
async function confirmName() {
  if (busy.value) return
  form.displayName = nameDraft.value.trim()
  const success = await save()
  // 创建已完成但回读失败时也退出命名，交由现有 ID 的重新读取入口恢复。
  if (success || !isNew.value) { nameOpen.value = false; return }
  if (Object.keys(errors.value).some((key) => key !== 'displayName')) {
    nameOpen.value = false
    await focusFirstError()
  } else {
    await nextTick()
    nameInput.value?.focus()
  }
}
async function focusFirstError() {
  // sys 字段可能尚未渲染，先展开 Ant 折叠区再定位错误输入。
  if (errors.value.sysUser || errors.value.sysPassword) advancedPanels.value = ['sys']
  await nextTick()
  const field = formElement.value?.$el.querySelector<HTMLElement>('[aria-invalid="true"]')
  field?.focus({ preventScroll: true })
  field?.scrollIntoView({ block: 'nearest', behavior: 'instant' })
}
// 失焦后校验；已有错误在输入修正后复验，不能输入任意字符就移除错误。
function validateField(key: DataSourceFormField) {
  const checked = validateDataSourceForm(form, isNew.value)
  const keys: DataSourceFormField[] = key === 'sysUser' || key === 'sysPassword' ? ['sysUser', 'sysPassword'] : [key]
  for (const field of keys) {
    if (checked[field]) errors.value[field] = checked[field]
    else delete errors.value[field]
  }
}
function clearError(key: DataSourceFormField) { if (errors.value[key] || ((key === 'sysUser' || key === 'sysPassword') && errors.value.sysPassword)) validateField(key) }
function validateBlur(event: FocusEvent) {
  const target = event.target
  if (target instanceof HTMLInputElement || target instanceof HTMLSelectElement) {
    if (target.name && Object.hasOwn(form, target.name)) validateField(target.name as DataSourceFormField)
  }
}
const resultState = computed(() => {
  if (test.value?.status === 'PENDING' || test.value?.status === 'LEASED') return 'pending'
  if (test.value?.status === 'FAILED') return 'error'
  if (test.value?.status === 'EXPIRED' || test.value?.status === 'INVALIDATED' || test.value?.verificationSource === 'G2_SYNTHETIC') return 'warning'
  if (test.value?.status === 'SUCCEEDED' && test.value.verificationSource === 'AGENT_JDBC' && test.value.realConnectionVerified) return 'success'
  return 'neutral'
})
// 列表测试入口直接定位同一表单中的节点选择，等详情加载后再转移焦点。
watch(loading, async (value) => {
  if (value || !props.focusTest) return
  await nextTick()
  nodeSelect.value?.focus()
  nodeSelect.value?.$el.scrollIntoView({ block: 'center' })
}, { immediate: true })
</script>

<template>
  <ADrawer :open="true" :width="`min(${product.source.editorWidth}px, 100vw)`" :closable="!saving" :keyboard="!saving" :mask-closable="false" :push="false" root-class-name="orch-inspector" class="orch-ui orch-source-editor" :body-style="{ padding: 0, overflow: 'hidden' }" @close="close" @after-open-change="afterDrawerOpenChange">
    <template #title><h2 :id="drawerTitleId" ref="drawerHeading" class="ant-drawer-title" tabindex="-1">{{ isNew ? '新建数据源' : '编辑数据源' }}</h2></template>
    <Skeleton v-if="loading" active :paragraph="{ rows: 4 }" aria-label="正在读取配置" />
    <div v-else class="orch-editor-scroll">
      <AAlert v-if="failure" class="orch-feedback" type="error" message="操作未完成" :description="failure" show-icon><template #action><AButton v-if="!isNew && !source" @click="load">重新读取</AButton></template></AAlert>
      <AAlert v-if="feedback" class="orch-feedback" type="info" :message="feedback" show-icon role="status" />
      <AForm v-if="isNew || source" id="orch-editor-configuration" ref="formElement" layout="vertical" :model="form" :disabled="busy" aria-label="连接配置" novalidate @focusout="validateBlur" @submit.prevent="saveForm">
        <AAlert v-if="Object.keys(errors).length" class="orch-feedback" type="error" :message="`有 ${Object.keys(errors).length} 项配置需要修正`" show-icon><template #action><AButton type="text" @click="focusFirstError">定位错误字段</AButton></template></AAlert>
        <fieldset :disabled="busy" class="orch-fieldset">
          <p class="orch-source-type">数据源类型：<DatabaseOutlined class="product-icon" aria-hidden="true" /><span>{{ modeLabel }}</span></p>
          <section class="orch-source-parser">
            <AFormItem label="智能解析（可选）" html-for="source-parser-input">
              <div class="orch-parser-input"><ATextarea id="source-parser-input" v-model:value="parserInput" :rows="3" autocomplete="off" spellcheck="false" :placeholder="`粘贴 ${form.compatibilityMode === 'MYSQL' ? 'mysql' : 'obclient'} 连接串`" /><AButton :disabled="!parserInput.trim() || busy" type="text" @click="parseConnection">智能解析</AButton></div>
            </AFormItem>
          </section>
          <section class="orch-form-section">
            <h3>连接地址</h3>
            <div class="orch-form-grid orch-source-group">
              <AFormItem label="主机 IP/域名" name="host" html-for="source-host" required :validate-status="errors.host ? 'error' : undefined" :help="errors.host" class="orch-host-field"><AInput id="source-host" v-model:value="form.host" name="host" :maxlength="253" required placeholder="请输入 ODP 主机 IP 或域名" :aria-invalid="Boolean(errors.host)" @input="clearError('host')" /></AFormItem>
              <AFormItem label="端口" name="port" html-for="source-port" required :validate-status="errors.port ? 'error' : undefined" :help="errors.port"><AInput id="source-port" v-model:value.number="form.port" name="port" placeholder="请输入端口" type="number" min="1" max="65535" required :aria-invalid="Boolean(errors.port)" @input="clearError('port')" /></AFormItem>
              <AFormItem label="集群名（可选）" name="clusterName" html-for="source-cluster" :validate-status="errors.clusterName ? 'error' : undefined" :help="errors.clusterName"><AInput id="source-cluster" v-model:value="form.clusterName" name="clusterName" placeholder="可选，留空时使用 用户@租户" :maxlength="256" :aria-invalid="Boolean(errors.clusterName)" @input="clearError('clusterName')" /></AFormItem>
              <AFormItem label="租户名" name="tenantName" html-for="source-tenant" required :validate-status="errors.tenantName ? 'error' : undefined" :help="errors.tenantName"><AInput id="source-tenant" v-model:value="form.tenantName" name="tenantName" placeholder="请输入租户名" :maxlength="256" required :aria-invalid="Boolean(errors.tenantName)" @input="clearError('tenantName')" /></AFormItem>
              <AFormItem v-if="form.compatibilityMode === 'MYSQL'" label="默认数据库（可选）" name="defaultDatabase" html-for="source-default-database" :validate-status="errors.defaultDatabase ? 'error' : undefined" :help="errors.defaultDatabase" class="orch-span-all"><AInput id="source-default-database" v-model:value="form.defaultDatabase" name="defaultDatabase" placeholder="可选，用于任务默认回填" :maxlength="512" :aria-invalid="Boolean(errors.defaultDatabase)" @input="clearError('defaultDatabase')" /></AFormItem>
            </div>
          </section>
          <section class="orch-form-section">
            <h3>数据库账号</h3><div class="orch-form-grid orch-source-group">
              <AFormItem label="数据库用户名" name="username" html-for="source-username" :required="isNew" :validate-status="errors.username ? 'error' : undefined" :help="errors.username"><AInput id="source-username" v-model:value="form.username" name="username" placeholder="请输入数据库用户名" :maxlength="256" autocomplete="off" :aria-invalid="Boolean(errors.username)" @input="clearError('username')" /></AFormItem>
              <AFormItem label="数据库密码" name="password" html-for="source-password" :required="isNew" :validate-status="errors.password ? 'error' : undefined" :help="errors.password"><AInputPassword id="source-password" v-model:value="form.password" name="password" autocomplete="new-password" :maxlength="4096" :aria-invalid="Boolean(errors.password)" :placeholder="isNew ? '请输入密码' : '留空保留现有密码'" @input="clearError('password')" /></AFormItem>
            </div>
            <div class="orch-source-test-entry orch-source-group">
              <AButton type="link" class="orch-test-disclosure" :aria-expanded="testExpanded" aria-controls="source-test-details" @click="testExpanded = !testExpanded">测试连接</AButton>
              <p v-if="isNew && !testExpanded">{{ testBlocked }}</p>
              <div v-show="testExpanded" id="source-test-details" class="orch-test-details">
                <div class="orch-inline-test-controls">
                  <AFormItem label="执行节点" html-for="source-test-node" required>
                    <ASelect id="source-test-node" ref="nodeSelect" v-model:value="nodeId" :disabled="busy || nodeLoading" aria-describedby="orch-test-help">
                      <ASelectOption value="">{{ nodeLoading ? '正在加载执行节点' : '请选择执行节点' }}</ASelectOption>
                      <ASelectOption v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</ASelectOption>
                    </ASelect>
                  </AFormItem>
                  <AButton :disabled="busy || nodeLoading" type="text" aria-label="刷新可测试节点" title="刷新执行节点" class="orch-icon-action" @click="loadNodes"><template #icon><ReloadOutlined class="product-icon" :spin="nodeLoading" aria-hidden="true" /></template></AButton>
                </div>
                <AAlert v-if="nodeFailure" type="error" :message="nodeFailure" show-icon />
                <div class="orch-test-action-row">
                  <p id="orch-test-help">{{ !nodeLoading && !nodes.length ? '暂无可用执行节点，请刷新后重试。' : testBlocked || '测试已保存的配置。' }}</p>
                  <AButton :disabled="busy || Boolean(testBlocked) || nodeLoading || !nodes.length" aria-describedby="orch-test-help" :loading="testing" @click="startTest"><template #icon><ApiOutlined class="product-icon" aria-hidden="true" /></template>开始测试</AButton>
                </div>
                <div v-if="!isNew && !test && !invalidated && !testing" class="orch-inline-test-fact"><ABadge :status="antBadgeStatus(fact.tone)" :text="fact.label" :aria-label="fact.label" /></div>
                <ConnectionTestResult v-if="testing" state="pending" title="等待执行节点回写结果"><p>关闭面板不会取消已经提交的测试。</p></ConnectionTestResult>
                <ConnectionTestResult v-else-if="!isNew && invalidated" state="invalidated" title="旧测试结果不再适用"><p>连接配置或凭据发生变化，需要基于保存后的配置重新测试。</p></ConnectionTestResult>
                <ConnectionTestResult v-else-if="test" :state="resultState" :title="diagnostic?.title || (resultState === 'success' ? '已验证可连接' : fact.label)">
                  <AAlert v-if="test.verificationSource === 'G2_SYNTHETIC'" type="warning" message="合成验证结果" description="不代表真实连接成功，不能据此启用数据源。" show-icon />
                  <template v-if="diagnostic"><p class="orch-evidence-caption">{{ diagnostic.stage }}</p><p>{{ diagnostic.summary }}</p><ul><li v-for="check in diagnostic.checks" :key="check">{{ check }}</li></ul></template>
                  <p v-else>{{ dataSourceConnectionTestNotice(test) }}</p>
                  <p v-if="sysCredentialVerificationNotice(test)">{{ sysCredentialVerificationNotice(test) }}</p>
                  <ACollapse ghost><template #expandIcon="panel"><DownOutlined v-if="panel?.isActive" class="product-icon" aria-hidden="true" /><RightOutlined v-else class="product-icon" aria-hidden="true" /></template><ACollapsePanel key="verification" header="查看验证记录"><dl class="orch-evidence-facts"><div><dt>结果代码</dt><dd><code>{{ test.resultCode || '未提供' }}</code></dd></div><div><dt>验证来源</dt><dd>{{ test.verificationSource }}</dd></div><div><dt>完成时间</dt><dd>{{ formatVerifiedTime(test.completedAt) }}</dd></div></dl></ACollapsePanel></ACollapse>
                </ConnectionTestResult>
              </div>
            </div>
          </section>
          <section class="orch-form-section orch-source-metadata">
            <div class="orch-form-grid">
              <AFormItem label="环境" name="environment" html-for="source-environment" required :validate-status="errors.environment ? 'error' : undefined" :help="errors.environment">
                <ASelect id="source-environment" v-model:value="form.environment" name="environment" placeholder="默认" :aria-invalid="Boolean(errors.environment)" @change="clearError('environment')">
                  <ASelectOption value="" label="默认" disabled><span class="orch-environment-chip orch-environment-chip--default">默认</span></ASelectOption>
                  <ASelectOption v-for="environment in editorEnvironments" :key="environment.value" :value="environment.value" :label="environment.label"><span class="orch-environment-chip" :style="product.environment[environment.value]">{{ environment.label }}</span></ASelectOption>
                  <ASelectOption v-if="form.environment === 'STAGING'" value="STAGING" label="预生产"><span class="orch-environment-chip" :style="product.environment.STAGING">预生产</span></ASelectOption>
                </ASelect>
              </AFormItem>
              <AFormItem v-if="!isNew" label="数据源名称" name="displayName" html-for="source-display-name" required :validate-status="errors.displayName ? 'error' : undefined" :help="errors.displayName" class="orch-span-all"><AInput id="source-display-name" v-model:value="form.displayName" name="displayName" :maxlength="120" required :aria-invalid="Boolean(errors.displayName)" placeholder="输入便于识别的名称" @input="clearError('displayName')" /></AFormItem>
            </div>
          </section>
          <ACollapse v-model:active-key="advancedPanels" ghost>
            <template #expandIcon="panel"><DownOutlined v-if="panel?.isActive" class="product-icon" aria-hidden="true" /><RightOutlined v-else class="product-icon" aria-hidden="true" /></template>
            <ACollapsePanel key="sys" header="高级设置" force-render>
              <template #extra>sys 凭据</template><p>可选。用于读取 sys 租户视图，未配置时相关导出能力降级。</p><div class="orch-form-grid">
                <AFormItem label="sys 账号" name="sysUser" html-for="source-sys-user" :validate-status="errors.sysUser ? 'error' : undefined" :help="errors.sysUser"><AInput id="source-sys-user" v-model:value="form.sysUser" name="sysUser" :disabled="clearSys" autocomplete="off" :maxlength="256" :aria-invalid="Boolean(errors.sysUser)" @input="clearError('sysUser')" /></AFormItem>
                <AFormItem label="sys 密码" name="sysPassword" html-for="source-sys-password" :validate-status="errors.sysPassword ? 'error' : undefined" :help="errors.sysPassword"><AInput id="source-sys-password" v-model:value="form.sysPassword" name="sysPassword" :disabled="clearSys" type="password" autocomplete="new-password" :maxlength="4096" :aria-invalid="Boolean(errors.sysPassword)" @input="clearError('sysPassword')" /></AFormItem>
              </div><label v-if="source?.sysCredentialState === 'AVAILABLE'" class="orch-checkbox"><ACheckbox v-model:checked="clearSys" @change="form.sysUser = ''; form.sysPassword = ''" />清除当前 sys 凭据</label>
            </ACollapsePanel>
          </ACollapse>
        </fieldset>
      </AForm>
    </div>
    <template #footer><div class="orch-editor-actions"><AButton :disabled="saving" @click="close">取消</AButton><AButton :disabled="busy || (!isNew && !dirty) || (!isNew && !source)" type="primary" :loading="saving" @click="saveForm">确定</AButton></div></template>
  </ADrawer>
  <OrchDangerConfirm :open="nameOpen" title="填写数据源名称" confirm-label="保存" :busy="saving" @cancel="cancelName" @confirm="confirmName">
    <AForm layout="vertical" :model="form" :disabled="busy" aria-label="数据源命名" @submit.prevent="confirmName">
      <AFormItem label="数据源名称" name="displayName" html-for="source-name-draft" required :validate-status="errors.displayName ? 'error' : undefined" :help="errors.displayName">
        <AInput id="source-name-draft" ref="nameInput" v-model:value="nameDraft" :maxlength="120" :disabled="saving" :aria-invalid="Boolean(errors.displayName)" placeholder="输入便于识别的名称" @input="delete errors.displayName" />
      </AFormItem>
      <AAlert v-if="failure" type="error" :message="failure" show-icon />
    </AForm>
  </OrchDangerConfirm>
  <OrchDangerConfirm :open="confirmClose" :title="testing ? '连接测试仍在进行' : '放弃未保存的更改？'" :confirm-label="testing ? '仍然离开' : '放弃更改'" cancel-label="继续编辑" :destructive="!testing" @cancel="cancelClose" @confirm="acceptClose"><p>{{ testing ? '关闭编辑面板不会取消测试。已提交的测试会继续在所选执行节点运行。' : '当前更改尚未保存，离开后无法恢复。' }}</p></OrchDangerConfirm>
</template>

<style scoped>
.orch-source-type { display: flex; align-items: center; gap: var(--ob-foundation-space-1); margin: 0 0 var(--ob-foundation-space-4); }

.orch-parser-input { position: relative; }
.orch-parser-input textarea { min-height: 84px; padding-bottom: 36px; resize: vertical; }
.orch-parser-input > .ant-btn { position: absolute; right: 4px; bottom: 4px; }
:global(html .orch-source-editor .ant-drawer-body) { min-height: 0; }
:global(html .orch-source-editor .orch-editor-scroll) { height: 100%; }
:global(html .orch-source-editor .orch-form-section) { padding-block: var(--ob-foundation-space-4) 0; border: 0; }
.orch-form-section h3 { margin-bottom: var(--ob-foundation-space-2); font-size: var(--ob-component-control-font-size); font-weight: 500; line-height: 22px; }
.orch-form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--ob-foundation-space-3); }
.orch-source-group { padding: var(--ob-foundation-space-4) var(--ob-foundation-space-3); background: var(--ob-color-subtle); }
.orch-source-group :deep(.ant-form-item) { margin-bottom: var(--ob-foundation-space-3); }
.orch-form-section > .orch-form-grid.orch-source-group { padding-bottom: var(--ob-foundation-space-1); }
.orch-form-section > .orch-form-grid.orch-source-group + .orch-source-test-entry { padding-top: 0; }
.orch-source-test-entry { display: grid; gap: var(--ob-foundation-space-2); }
.orch-test-disclosure { justify-self: start; height: auto; padding: 0; }
.orch-test-details { display: grid; gap: var(--ob-foundation-space-2); }
.orch-source-metadata .orch-form-grid { align-items: start; }
.orch-source-metadata .orch-span-all { grid-column: 1 / -1; }
.orch-environment-chip { display: inline-flex; align-items: center; min-height: 22px; padding-inline: 7px; border-radius: 2px; font-size: 12px; line-height: 18px; }
.orch-environment-chip--default { color: var(--ob-color-form-secondary); background: #f0f0f0; }
.orch-inline-test-controls { display: grid; grid-template-columns: minmax(0, 1fr) 36px; align-items: end; gap: var(--ob-foundation-space-2); }
.orch-inline-test-controls :deep(.ant-form-item) { margin-bottom: 0; }
.orch-test-action-row { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--ob-foundation-space-2); }
.orch-test-action-row > p { min-width: 0; flex: 1; padding-top: var(--ob-foundation-space-2); }
.orch-test-action-row > .ant-btn { flex: none; }
.orch-inline-test-fact { display: flex; }
.orch-editor-actions { justify-content: flex-end; gap: var(--ob-foundation-space-2); }
.orch-source-test-entry p { margin: 0; color: var(--ob-color-form-secondary); font-size: var(--ob-component-field-helper-size); line-height: 18px; }
@media (max-width: 420px) {
  .orch-form-grid { grid-template-columns: minmax(0, 1fr); }
  .orch-source-metadata .orch-span-all { grid-column: auto; }
}
</style>
