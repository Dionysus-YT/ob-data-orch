<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { ChevronDown, Eye, EyeOff, Database, RefreshCw, Unplug } from '@lucide/vue'
import OrchButton from '../components/OrchButton.vue'
import OrchDock from '../components/OrchDock.vue'
import OrchDialog from '../components/OrchDialog.vue'
import OrchField from '../components/OrchField.vue'
import OrchStatus from '../components/OrchStatus.vue'
import ConnectionTestResult from '@/components/ConnectionTestResult.vue'
import { validateDataSourceForm, type DataSourceFormField } from '@/views/dataSourceFormErrors'
import { dataSourceConnectionTestDiagnostic } from '@/views/dataSourceConnectionTestDiagnostic'
import { dataSourceConnectionTestNotice, sysCredentialVerificationNotice } from '@/views/dataSourceConnectionTestNotice'
import { connectionFact, environments, formatVerifiedTime } from './sourcePresentation'
import type { SourceGateway } from './sourceGateway'
import { useSourceEditor } from './useSourceEditor'

const props = defineProps<{ api: SourceGateway; sourceId: string | null; initialMode: 'MYSQL' | 'ORACLE'; focusTest?: boolean; preview: boolean }>()
const emit = defineEmits<{ close: []; saved: [id: string] }>()
const { source, form, clearSys, parserInput, errors, failure, feedback, loading, saving, testing, nodes, nodeLoading, nodeFailure, nodeId, test, isNew, dirty, busy, invalidated, testBlocked, load, loadNodes, parseConnection, save, startTest } = useSourceEditor(props.api, props.sourceId, (id) => emit('saved', id), props.initialMode)
const modeLabel = computed(() => form.compatibilityMode === 'ORACLE' ? 'OceanBase Oracle' : 'OceanBase MySQL')
const nameOpen = ref(false)
const nameDraft = ref('')
const nameInput = ref<HTMLInputElement>()
const nodeSelect = ref<HTMLSelectElement>()
const passwordVisible = ref(false)
const confirmClose = ref(false)
const formElement = ref<HTMLFormElement>()
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
  await nextTick()
  const field = formElement.value?.querySelector<HTMLElement>('[aria-invalid="true"]')
  // 高级字段可能处于折叠区中，必须先展开祖先详情，才能定位可见的错误输入。
  for (let parent = field?.parentElement; parent; parent = parent.parentElement) {
    if (parent instanceof HTMLDetailsElement) parent.open = true
  }
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
  nodeSelect.value?.scrollIntoView({ block: 'center' })
}, { immediate: true })
</script>

<template>
  <OrchDock :title="isNew ? '新建数据源' : '编辑数据源'" compact hide-state :dirty="dirty" :is-new="isNew" :busy="saving" @close="close">
    <div v-if="loading" class="orch-editor-loading" role="status"><span class="orch-skeleton" /><span class="orch-skeleton" /><span class="orch-skeleton" /><span class="orch-skeleton" /><span>正在读取配置</span></div>
    <div v-else class="orch-editor-scroll">
      <div v-if="failure" class="orch-alert orch-alert--danger" role="alert"><strong>操作未完成</strong><p>{{ failure }}</p><OrchButton v-if="!isNew && !source" @click="load">重新读取</OrchButton></div>
      <div v-if="feedback" class="orch-alert orch-alert--neutral" role="status">{{ feedback }}</div>
      <form v-if="isNew || source" id="orch-editor-configuration" ref="formElement" aria-label="连接配置" novalidate @focusout="validateBlur" @submit.prevent="saveForm">
        <div v-if="Object.keys(errors).length" class="orch-alert orch-alert--danger orch-validation-summary" role="alert"><strong>有 {{ Object.keys(errors).length }} 项配置需要修正</strong><OrchButton variant="quiet" @click="focusFirstError">定位错误字段</OrchButton></div>
        <fieldset :disabled="busy" class="orch-fieldset">
          <p class="orch-source-type">数据源类型：<Database :size="16" aria-hidden="true" /><span>{{ modeLabel }}</span></p>
          <section class="orch-source-parser">
            <OrchField v-slot="field" label="智能解析（可选）">
              <div class="orch-parser-input"><textarea :id="field.id" v-model="parserInput" class="orch-input" rows="3" autocomplete="off" spellcheck="false" :aria-describedby="field.describedBy" :placeholder="`粘贴 ${form.compatibilityMode === 'MYSQL' ? 'mysql' : 'obclient'} 连接串`" /><OrchButton variant="quiet" :disabled="!parserInput.trim() || busy" @click="parseConnection">智能解析</OrchButton></div>
            </OrchField>
          </section>
          <section class="orch-form-section">
            <h3>连接地址</h3>
            <div class="orch-form-grid">
              <OrchField v-slot="field" label="主机 IP/域名" required technical :error="errors.host" class="orch-host-field"><input :id="field.id" v-model="form.host" name="host" class="orch-input" maxlength="253" required placeholder="请输入 ODP 主机 IP 或域名" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('host')" /></OrchField>
              <OrchField v-slot="field" label="端口" required technical :error="errors.port"><input :id="field.id" v-model.number="form.port" name="port" class="orch-input" placeholder="请输入端口" type="number" min="1" max="65535" required :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('port')" /></OrchField>
              <OrchField v-slot="field" label="集群名（可选）" technical :error="errors.clusterName"><input :id="field.id" v-model="form.clusterName" name="clusterName" class="orch-input" placeholder="可选，留空时使用 用户@租户" maxlength="256" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('clusterName')" /></OrchField>
              <OrchField v-slot="field" label="租户名" required technical :error="errors.tenantName"><input :id="field.id" v-model="form.tenantName" name="tenantName" class="orch-input" placeholder="请输入租户名" maxlength="256" required :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('tenantName')" /></OrchField>
              <OrchField v-if="form.compatibilityMode === 'MYSQL'" v-slot="field" label="默认数据库（可选）" technical :error="errors.defaultDatabase" class="orch-span-all"><input :id="field.id" v-model="form.defaultDatabase" name="defaultDatabase" class="orch-input" placeholder="可选，用于任务默认回填" maxlength="512" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('defaultDatabase')" /></OrchField>
            </div>
          </section>
          <section class="orch-form-section">
            <h3>数据库账号</h3><div class="orch-form-grid">
              <OrchField v-slot="field" label="数据库用户名" :required="isNew" :error="errors.username" technical><input :id="field.id" v-model="form.username" name="username" class="orch-input" placeholder="请输入数据库用户名" maxlength="256" autocomplete="off" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('username')" /></OrchField>
              <OrchField v-slot="field" label="数据库密码" :required="isNew" :error="errors.password"><div class="orch-password"><input :id="field.id" v-model="form.password" name="password" class="orch-input" :type="passwordVisible ? 'text' : 'password'" autocomplete="new-password" maxlength="4096" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" :placeholder="isNew ? '请输入密码' : '留空保留现有密码'" @input="clearError('password')" /><OrchButton variant="quiet" icon-only :label="passwordVisible ? '隐藏密码' : '显示密码'" :aria-pressed="passwordVisible" @click="passwordVisible = !passwordVisible"><EyeOff v-if="passwordVisible" :size="16" /><Eye v-else :size="16" /></OrchButton></div></OrchField>
            </div>
            <div class="orch-source-test-entry">
              <div class="orch-inline-test-controls">
                <OrchField v-slot="field" label="执行节点" required>
                  <select :id="field.id" ref="nodeSelect" v-model="nodeId" class="orch-select" :disabled="busy || nodeLoading" aria-describedby="orch-test-help">
                    <option value="">{{ nodeLoading ? '正在加载执行节点' : '请选择执行节点' }}</option>
                    <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</option>
                  </select>
                </OrchField>
                <OrchButton variant="quiet" icon-only label="刷新可测试节点" :disabled="busy || nodeLoading" @click="loadNodes"><RefreshCw :size="16" :class="{ 'orch-spin': nodeLoading }" /></OrchButton>
                <OrchButton :disabled="busy || Boolean(testBlocked) || nodeLoading || !nodes.length" :busy="testing" aria-describedby="orch-test-help" @click="startTest"><Unplug :size="16" />测试连接</OrchButton>
              </div>
              <p v-if="nodeFailure" class="orch-field-error" role="alert">{{ nodeFailure }}</p>
              <p id="orch-test-help">{{ !nodeLoading && !nodes.length ? '暂无可用执行节点，请刷新后重试。' : testBlocked || '测试已保存的配置。' }}</p>
              <div v-if="!isNew && !test && !invalidated && !testing" class="orch-inline-test-fact"><OrchStatus :label="fact.label" :tone="fact.tone" :icon="fact.icon" /></div>
              <ConnectionTestResult v-if="testing" state="pending" title="等待执行节点回写结果"><p>关闭面板不会取消已经提交的测试。</p></ConnectionTestResult>
              <ConnectionTestResult v-else-if="!isNew && invalidated" state="invalidated" title="旧测试结果不再适用"><p>连接配置或凭据发生变化，需要基于保存后的配置重新测试。</p></ConnectionTestResult>
              <ConnectionTestResult v-else-if="test" :state="resultState" :title="diagnostic?.title || (resultState === 'success' ? '已验证可连接' : fact.label)">
                <div v-if="test.verificationSource === 'G2_SYNTHETIC'" class="orch-alert orch-alert--warning"><strong>合成验证结果</strong><p>不代表真实连接成功，不能据此启用数据源。</p></div>
                <template v-if="diagnostic"><p class="orch-evidence-caption">{{ diagnostic.stage }}</p><p>{{ diagnostic.summary }}</p><ul><li v-for="check in diagnostic.checks" :key="check">{{ check }}</li></ul></template>
                <p v-else>{{ dataSourceConnectionTestNotice(test) }}</p>
                <p v-if="sysCredentialVerificationNotice(test)">{{ sysCredentialVerificationNotice(test) }}</p>
                <details class="orch-result-details"><summary>查看验证记录</summary><dl class="orch-evidence-facts"><div><dt>结果代码</dt><dd><code>{{ test.resultCode || '未提供' }}</code></dd></div><div><dt>验证来源</dt><dd>{{ test.verificationSource }}</dd></div><div><dt>完成时间</dt><dd>{{ formatVerifiedTime(test.completedAt) }}</dd></div></dl></details>
              </ConnectionTestResult>
            </div>
          </section>
          <section class="orch-source-metadata">
            <OrchField v-slot="field" label="环境" required><select :id="field.id" v-model="form.environment" name="environment" class="orch-select" required><option v-for="environment in environments" :key="environment.value" :value="environment.value">{{ environment.label }}</option></select></OrchField>
            <OrchField v-if="!isNew" v-slot="field" label="数据源名称" required class="orch-span-all" :error="errors.displayName"><input :id="field.id" v-model="form.displayName" name="displayName" class="orch-input" maxlength="120" required :aria-invalid="field.invalid" :aria-describedby="field.describedBy" placeholder="输入便于识别的名称" @input="clearError('displayName')" /></OrchField>
          </section>
          <details class="orch-disclosure orch-advanced">
            <summary><ChevronDown :size="15" />高级设置<span>sys 凭据</span></summary><p>可选。用于读取 sys 租户视图，未配置时相关导出能力降级。</p><div class="orch-form-grid">
              <OrchField v-slot="field" label="sys 账号" :error="errors.sysUser"><input :id="field.id" v-model="form.sysUser" name="sysUser" class="orch-input" :disabled="clearSys" autocomplete="off" maxlength="256" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('sysUser')" /></OrchField>
              <OrchField v-slot="field" label="sys 密码" :error="errors.sysPassword"><input :id="field.id" v-model="form.sysPassword" name="sysPassword" class="orch-input" :disabled="clearSys" type="password" autocomplete="new-password" maxlength="4096" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" @input="clearError('sysPassword')" /></OrchField>
            </div><label v-if="source?.sysCredentialState === 'AVAILABLE'" class="orch-checkbox"><input v-model="clearSys" type="checkbox" @change="form.sysUser = ''; form.sysPassword = ''" />清除当前 sys 凭据</label>
          </details>
        </fieldset>
      </form>
    </div>
    <template #footer><div class="orch-editor-actions"><OrchButton :disabled="saving" @click="close">取消</OrchButton><OrchButton variant="primary" :disabled="busy || (!isNew && !dirty) || (!isNew && !source)" :busy="saving" @click="saveForm">确定</OrchButton></div></template>
  </OrchDock>
  <OrchDialog :open="nameOpen" title="填写数据源名称" confirm-label="保存" :busy="saving" @cancel="cancelName" @confirm="confirmName">
    <form aria-label="数据源命名" @submit.prevent="confirmName">
      <OrchField v-slot="field" label="数据源名称" required :error="errors.displayName">
        <input :id="field.id" ref="nameInput" v-model="nameDraft" class="orch-input" maxlength="120" :disabled="saving" :aria-invalid="field.invalid" :aria-describedby="field.describedBy" placeholder="输入便于识别的名称" @input="delete errors.displayName" />
      </OrchField>
      <p v-if="failure" class="orch-field-error" role="alert">{{ failure }}</p>
    </form>
  </OrchDialog>
  <OrchDialog :open="confirmClose" :title="testing ? '连接测试仍在进行' : '放弃未保存的更改？'" :confirm-label="testing ? '仍然离开' : '放弃更改'" cancel-label="继续编辑" :destructive="!testing" @cancel="cancelClose" @confirm="acceptClose"><p>{{ testing ? '关闭编辑面板不会取消测试。已提交的测试会继续在所选执行节点运行。' : '当前更改尚未保存，离开后无法恢复。' }}</p></OrchDialog>
</template>

<style scoped>
:global(html .orch-ui.orch-dock.orch-source-editor) { width: 520px; max-width: 100vw; }
:global(html .orch-source-editor .orch-dock-header) { min-height: 48px; padding-block: 8px; }
:global(html .orch-source-editor .orch-dock-header h2) { font-size: 17px; line-height: 24px; }
:global(html .orch-source-editor .orch-dock-footer) { min-height: 56px; padding-block: 8px; }
.orch-source-type { display: flex; align-items: center; gap: 4px; margin: 16px 0; }
.orch-source-type svg { color: var(--color-primary); }
.orch-parser-input { position: relative; }
.orch-parser-input textarea { min-height: 96px; padding-bottom: 36px; resize: vertical; }
.orch-parser-input .orch-button { position: absolute; right: 4px; bottom: 4px; }
:global(html .orch-source-editor .orch-form-section) { padding-block: 16px 0; border: 0; }
.orch-form-section h3 { margin-bottom: 8px; font-size: 13px; line-height: 20px; }
.orch-form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.orch-form-section > .orch-form-grid { padding: 12px; background: var(--color-bg-surface); }
.orch-source-test-entry { display: grid; gap: 8px; padding: 4px 12px 12px; background: var(--color-bg-surface); }
.orch-inline-test-controls { display: grid; grid-template-columns: minmax(0, 1fr) 36px auto; align-items: end; gap: 8px; }
.orch-inline-test-fact { display: flex; }
.orch-editor-actions { justify-content: flex-end; gap: 8px; }
.orch-editor-actions :deep(.orch-button) { min-width: 72px; }
/* 确定按钮预留两侧等宽空间，加载图标出现时文字仍保持居中。 */
.orch-editor-actions :deep(.orch-button.has-progress) { min-width: 96px; padding-inline: 28px; }
.orch-editor-actions :deep(.orch-button-spinner) { right: 6px; }
.orch-source-test-entry p { margin: 0; color: var(--color-text-secondary); font-size: 12px; line-height: 18px; }
.orch-source-metadata { display: grid; gap: 16px; padding-top: 16px; }
.orch-source-metadata > :first-child { width: calc(50% - 6px); }
@media (max-width: 600px) {
  :global(html .orch-ui.orch-dock.orch-source-editor) { width: 100vw; }
  .orch-form-grid { grid-template-columns: minmax(0, 1fr); }
  .orch-source-metadata > :first-child { width: 100%; }
  .orch-inline-test-controls { grid-template-columns: minmax(0, 1fr) 36px; }
  .orch-inline-test-controls > :last-child { grid-column: 1 / -1; justify-self: start; }
}
</style>
