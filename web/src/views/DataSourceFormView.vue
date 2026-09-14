<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { AlertTriangle, ChevronDown, Eye, EyeOff, RefreshCw, Unplug } from '@lucide/vue'

import { browserApi, dataSourceErrorMessage, type DataSourceConnectionTest, type DataSourceConnectionTestRequest, type DataSourceDetail, type DataSourceUpdate, type DataSourceWrite, type ExecutionNodeCandidate } from '@/api/browser'
import ConnectionTestResult from '@/components/ConnectionTestResult.vue'
import WorkbenchButton from '@/components/WorkbenchButton.vue'
import WorkbenchIconButton from '@/components/WorkbenchIconButton.vue'
import WorkbenchFormField from '@/components/WorkbenchFormField.vue'
import { parseDataSourceConnectionString } from './dataSourceConnectionString'
import { dataSourceConnectionTestNotice, sysCredentialVerificationNotice } from './dataSourceConnectionTestNotice'
import { dataSourceConnectionTestDiagnostic } from './dataSourceConnectionTestDiagnostic'
import { dataSourceFieldErrorsFromApi, type DataSourceFormErrors, type DataSourceFormField, validateDataSourceForm } from './dataSourceFormErrors'

// 数据源仅在管理页 Drawer 中编辑；空 ID 表示新增配置。
const props = withDefaults(defineProps<{ dataSourceId?: string | null; focusTest?: boolean }>(), { dataSourceId: null, focusTest: false })
const emit = defineEmits<{ saved: [id: string]; cancel: []; 'dirty-change': [dirty: boolean]; 'test-running-change': [running: boolean] }>()

const api = browserApi()
type DataSourceForm = { -readonly [Key in keyof DataSourceWrite]: DataSourceWrite[Key] }
type MutableDataSourceUpdate = { -readonly [Key in keyof DataSourceUpdate]: DataSourceUpdate[Key] }
const activeDataSourceID = ref(props.dataSourceId ?? '')
const dataSourceID = computed(() => activeDataSourceID.value)
const isNew = computed(() => dataSourceID.value === '' || dataSourceID.value === 'new')
const source = ref<DataSourceDetail>()
const loading = ref(!isNew.value)
const loadFailed = ref(false)
const saveBusy = ref(false)
const testBusy = ref(false)
const busy = computed(() => saveBusy.value || testBusy.value)
const failure = ref('')
const notice = ref('')
const connectionTest = ref<DataSourceConnectionTest>()
const connectionTestRequest = ref<DataSourceConnectionTestRequest>()
const connectionStatusInvalidated = ref(false)
const connectionString = ref('')
const connectionStringError = ref('')
const testFailure = ref('')
const nodeCandidates = ref<ExecutionNodeCandidate[]>([])
const nodeCandidatesLoading = ref(false)
const nodeCandidatesFailure = ref('')
const selectedNodeID = ref('')
const passwordVisible = ref(false)
const clearSysCredential = ref(false)
const savedFormSnapshot = ref('')
const testSection = ref<HTMLElement>()
const connectionSection = ref<HTMLElement>()
const testNodeSelect = ref<HTMLSelectElement>()
const form = reactive<DataSourceForm>({ displayName: '', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '', port: 2883, clusterName: '', tenantName: '', username: '', defaultDatabase: '', password: '', sysUser: '', sysPassword: '' })
const formErrors = reactive<DataSourceFormErrors>({})
let connectionTestPollTimer: ReturnType<typeof setTimeout> | undefined
let connectionTestPollResolve: (() => void) | undefined
let connectionTestPollVersion = 0

onMounted(initializePage)
onBeforeUnmount(stopConnectionTestPolling)

async function initializePage() {
  if (isNew.value) await loadConnectionTestNodeCandidates()
  await loadPage()
  markSavedSnapshot()
  if (props.focusTest && !isNew.value) {
    await nextTick()
    testSection.value?.scrollIntoView({ block: 'start' })
    testNodeSelect.value?.focus()
  }
}

async function loadPage() {
  if (isNew.value) return
  await Promise.all([loadSource(), loadConnectionTestNodeCandidates()])
}

async function loadSource() {
  if (isNew.value) return
  loading.value = true
  loadFailed.value = false
  failure.value = ''
  try {
    source.value = await api.getDataSource(dataSourceID.value)
    fillFromSource(source.value)
    connectionStatusInvalidated.value = false
    markSavedSnapshot()
  } catch (error) {
    loadFailed.value = true
    failure.value = dataSourceErrorMessage(error, '无法加载数据源。')
  } finally {
    loading.value = false
  }
}

async function loadConnectionTestNodeCandidates() {
  nodeCandidatesLoading.value = true
  nodeCandidatesFailure.value = ''
  try {
    nodeCandidates.value = await api.listDataSourceConnectionTestNodeCandidates()
  } catch (error) {
    nodeCandidates.value = []
    nodeCandidatesFailure.value = dataSourceErrorMessage(error, '可测试执行节点加载失败。')
  } finally {
    nodeCandidatesLoading.value = false
  }
}

function fillFromSource(value: DataSourceDetail) {
  clearFormErrors()
  form.displayName = value.displayName
  form.environment = value.environment as DataSourceWrite['environment']
  form.connectionKind = 'ODP'
  form.compatibilityMode = value.compatibilityMode === 'ORACLE' ? 'ORACLE' : 'MYSQL'
  form.host = value.host
  form.port = value.port
  form.clusterName = value.clusterName
  form.tenantName = value.tenantName
  form.username = value.username ?? ''
  form.defaultDatabase = value.defaultDatabase ?? ''
  form.password = ''
  passwordVisible.value = false
  form.sysUser = ''
  form.sysPassword = ''
  clearSysCredential.value = false
}

function formSnapshot() {
  return JSON.stringify({
    displayName: form.displayName,
    environment: form.environment,
    compatibilityMode: form.compatibilityMode,
    host: form.host,
    port: form.port,
    clusterName: form.clusterName,
    tenantName: form.tenantName,
    username: form.username,
    defaultDatabase: form.defaultDatabase,
    password: form.password,
    sysUser: form.sysUser,
    sysPassword: form.sysPassword,
    clearSysCredential: clearSysCredential.value,
    connectionString: connectionString.value,
  })
}

function markSavedSnapshot() {
  savedFormSnapshot.value = formSnapshot()
}

const hasUnsavedChanges = computed(() => savedFormSnapshot.value !== '' && formSnapshot() !== savedFormSnapshot.value)
const hasUnsavedConnectionChanges = computed(() => Boolean(source.value && changesConnectionInput(buildUpdate(source.value))))
const testResultInvalidated = computed(() => hasUnsavedConnectionChanges.value || connectionStatusInvalidated.value)

watch(hasUnsavedChanges, (dirty) => emit('dirty-change', dirty), { immediate: true })
watch(testBusy, (running) => emit('test-running-change', running), { immediate: true })

function applyConnectionString() {
  const parsed = parseDataSourceConnectionString(connectionString.value)
  if (!parsed) {
    connectionStringError.value = '连接串无法按当前 OceanBase ODP 规则解析。请使用 mysql 或 obclient 开头，并提供地址、端口和用户@租户，可选 #集群。'
    return
  }
  Object.assign(form, parsed, { password: parsed.password || form.password })
  connectionStringError.value = ''
  clearFormErrors()
  failure.value = ''
  notice.value = '连接串已解析并回填结构化字段。原始连接串仅保留在当前浏览器页面，便于调整；保存和测试请求不会提交它。'
}

async function save() {
  const validationErrors = validateDataSourceForm(form, isNew.value)
  if (Object.keys(validationErrors).length > 0) {
    setFormErrors(validationErrors)
    failure.value = ''
    return
  }
  clearFormErrors()
  saveBusy.value = true; failure.value = ''; notice.value = ''
  try {
    if (isNew.value) {
      const selectedNodeBeforeCreate = selectedNodeID.value
      const createdID = await api.createDataSource({ ...form, displayName: form.displayName.trim(), host: form.host.trim(), clusterName: form.clusterName.trim(), tenantName: form.tenantName.trim(), username: form.username.trim(), defaultDatabase: form.compatibilityMode === 'MYSQL' ? form.defaultDatabase || undefined : undefined, sysUser: (form.sysUser ?? '').trim() || undefined, sysPassword: form.sysPassword || undefined })
      activeDataSourceID.value = createdID
      form.password = ''
      form.sysPassword = ''
      connectionString.value = ''
      // 创建已成功后先通知列表刷新，随后再读取详情以进入可测试的已保存状态。
      emit('saved', createdID)
      try {
        source.value = await api.getDataSource(createdID)
      } catch (error) {
        loadFailed.value = true
        failure.value = dataSourceErrorMessage(error, '数据源已保存，但当前无法刷新详情。请重试加载后继续连接测试。')
        return
      }
      fillFromSource(source.value)
      connectionStatusInvalidated.value = false
      notice.value = '数据源已保存。现在可以选择执行节点进行真实连接测试。'
      markSavedSnapshot()
      await loadConnectionTestNodeCandidates()
      selectedNodeID.value = nodeCandidates.value.some((candidate) => candidate.id === selectedNodeBeforeCreate) ? selectedNodeBeforeCreate : ''
      return
    }
    if (!source.value) return
    const update = buildUpdate(source.value)
    if (Object.keys(update).length === 0) {
      connectionString.value = ''
      markSavedSnapshot()
      notice.value = '没有需要保存的变更。'
      return
    }
    const invalidatesConnectionTest = changesConnectionInput(update)
    const updated = await api.updateDataSource(source.value.id, source.value.revision, update)
    source.value = updated
    fillFromSource(updated)
    connectionStatusInvalidated.value = invalidatesConnectionTest
    connectionString.value = ''
    markSavedSnapshot()
    notice.value = invalidatesConnectionTest
      ? '数据源已保存。连接输入已变更，页面不会继续使用保存前的连接测试结论。'
      : '数据源已保存。'
    emit('saved', updated.id)
  } catch (error) {
    const fieldErrors = dataSourceFieldErrorsFromApi(error)
    if (Object.keys(fieldErrors).length > 0) {
      setFormErrors(fieldErrors)
      failure.value = ''
    } else {
      failure.value = dataSourceErrorMessage(error, '数据源保存失败。')
    }
  } finally { saveBusy.value = false }
}

function buildUpdate(current: DataSourceDetail): DataSourceUpdate {
  const update: MutableDataSourceUpdate = {}
  if (form.displayName.trim() !== current.displayName) update.displayName = form.displayName.trim()
  if (form.environment !== current.environment) update.environment = form.environment
  if (form.compatibilityMode !== current.compatibilityMode) update.compatibilityMode = form.compatibilityMode
  if (form.host.trim() !== current.host) update.host = form.host.trim()
  if (form.port !== current.port) update.port = form.port
  if (form.clusterName.trim() !== current.clusterName) update.clusterName = form.clusterName.trim()
  if (form.tenantName.trim() !== current.tenantName) update.tenantName = form.tenantName.trim()
  const defaultDatabase = form.compatibilityMode === 'MYSQL' ? form.defaultDatabase : ''
  if (defaultDatabase !== (current.defaultDatabase ?? '')) update.defaultDatabase = defaultDatabase
  if (form.username.trim() !== current.username) update.username = form.username.trim()
  if (form.password) update.password = form.password
  // 可选的 sys 凭据（参考 ODC 数据源高级设置）：显式清除发送空字段，否则成对填写才设置或轮换。
  if (clearSysCredential.value) {
    update.sysUser = ''
    update.sysPassword = ''
    return update
  }
  const sysUser = (form.sysUser ?? '').trim()
  if (sysUser || form.sysPassword) {
    if (!sysUser || !form.sysPassword) {
      // 校验层已拦截，这里只是防御性短路。
      return update
    }
    update.sysUser = sysUser
    update.sysPassword = form.sysPassword
  }
  return update
}

function changesConnectionInput(update: DataSourceUpdate) {
  return update.connectionKind !== undefined || update.compatibilityMode !== undefined || update.host !== undefined || update.port !== undefined || update.clusterName !== undefined || update.tenantName !== undefined || update.username !== undefined || update.defaultDatabase !== undefined || update.password !== undefined || update.sysUser !== undefined || update.sysPassword !== undefined
}

function toggleClearSysCredential() {
  if (!clearSysCredential.value) return
  form.sysUser = ''
  form.sysPassword = ''
  clearFieldError('sysUser')
  clearFieldError('sysPassword')
}

const connectionTestResultState = computed<'pending' | 'success' | 'warning' | 'error'>(() => {
  const result = connectionTest.value
  if (!result) return 'pending'
  if (result.status === 'SUCCEEDED') return result.realConnectionVerified ? 'success' : 'warning'
  if (result.status === 'INVALIDATED') return 'warning'
  if (result.status === 'FAILED' || result.status === 'EXPIRED' || result.status === 'UNKNOWN') return 'error'
  return 'pending'
})
const connectionTestResultDetail = computed(() => {
  const result = connectionTest.value
  if (!result) return connectionTestRequest.value ? '测试请求已排队，正在等待所选节点的 Agent 领取；页面只展示控制面回写的受控状态。' : ''
  if (!isTerminalConnectionTestStatus(result.status)) return '正在等待所选节点的 Agent 回写受控终态；页面每两秒刷新一次，不伪造进度。'
  return dataSourceConnectionTestNotice(result)
})
const connectionTestSysNotice = computed(() => (connectionTest.value ? sysCredentialVerificationNotice(connectionTest.value) : ''))
const connectionTestDiagnostic = computed(() => (connectionTest.value ? dataSourceConnectionTestDiagnostic(connectionTest.value) : undefined))

async function focusConnectionTestDiagnostic() {
  const diagnostic = connectionTestDiagnostic.value
  if (!diagnostic) return
  const target = diagnostic.target === 'execution-node' ? testSection.value : connectionSection.value
  target?.scrollIntoView({ behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth', block: 'start' })
  if (diagnostic.target === 'execution-node') testNodeSelect.value?.focus({ preventScroll: true })
  else document.querySelector<HTMLElement>('[data-connection-diagnostic-focus]')?.focus({ preventScroll: true })
}

async function testConnection() {
  if (isNew.value || !source.value) {
    notice.value = '请先保存数据源；页面只会请求所选 Agent 执行连接测试，不会直接连接数据库。'
    return
  }
  if (hasUnsavedConnectionChanges.value) {
    testFailure.value = '请先保存当前更改。基础连接测试只绑定已保存的数据源配置和凭据版本。'
    return
  }
  if (!selectedNodeID.value) {
    testFailure.value = '请选择一个可测试执行节点。'
    return
  }
  if (!nodeCandidates.value.some((candidate) => candidate.id === selectedNodeID.value)) {
    testFailure.value = '所选执行节点不再可用于连接测试，请刷新节点列表后重新选择。'
    return
  }
  stopConnectionTestPolling()
  testBusy.value = true
  failure.value = ''
  testFailure.value = ''
  notice.value = ''
  connectionTest.value = undefined
  connectionTestRequest.value = undefined
  connectionStatusInvalidated.value = false
  try {
    connectionTestRequest.value = await api.startDataSourceConnectionTest(source.value.id, source.value.revision, selectedNodeID.value)
    notice.value = '连接测试已排队，正在等待所选节点的 Agent 领取。页面只展示控制面回写的受控状态。'
    await pollConnectionTest(connectionTestRequest.value.id)
  } catch (error) {
    testFailure.value = dataSourceErrorMessage(error, '当前无法发起或读取节点侧连接测试。测试请求如已被接受，仍可能继续在所选节点排队。')
  } finally {
    testBusy.value = false
  }
}

async function pollConnectionTest(connectionTestID: string) {
  const pollVersion = ++connectionTestPollVersion
  while (pollVersion === connectionTestPollVersion) {
    const result = await api.getDataSourceConnectionTest(connectionTestID)
    if (pollVersion !== connectionTestPollVersion) return
    connectionTest.value = result
    if (isTerminalConnectionTestStatus(result.status)) {
      notice.value = ''
      // 测试结果改为在测试连接区域内就近高亮展示（参考 ODC：失败在对应操作位置醒目提示），不再只依赖页面顶部提示条。
      if (source.value && Object.keys(buildUpdate(source.value)).length === 0) {
        await refreshSourceAfterConnectionTest(source.value.id, pollVersion)
      }
      return
    }
    await waitForConnectionTestPoll(pollVersion)
  }
}

function waitForConnectionTestPoll(pollVersion: number): Promise<void> {
  return new Promise((resolve) => {
    connectionTestPollResolve = () => {
      connectionTestPollResolve = undefined
      resolve()
    }
    connectionTestPollTimer = setTimeout(() => {
      connectionTestPollTimer = undefined
      if (pollVersion === connectionTestPollVersion) connectionTestPollResolve?.()
    }, 1000)
  })
}

function stopConnectionTestPolling() {
  connectionTestPollVersion += 1
  if (connectionTestPollTimer !== undefined) {
    clearTimeout(connectionTestPollTimer)
    connectionTestPollTimer = undefined
  }
  connectionTestPollResolve?.()
}

async function refreshSourceAfterConnectionTest(dataSourceID: string, pollVersion: number) {
  try {
    const refreshed = await api.getDataSource(dataSourceID)
    if (pollVersion !== connectionTestPollVersion) return
    source.value = refreshed
    fillFromSource(refreshed)
    connectionStatusInvalidated.value = false
    markSavedSnapshot()
  } catch {
    testFailure.value = '连接测试已结束，但最新数据源状态刷新失败。请刷新页面后重新确认启用状态。'
  }
}

function isTerminalConnectionTestStatus(status: DataSourceConnectionTest['status']) {
  return status === 'SUCCEEDED' || status === 'FAILED' || status === 'UNKNOWN' || status === 'EXPIRED' || status === 'INVALIDATED'
}

function connectionTestStatusLabel(status: DataSourceConnectionTest['status']) {
  return {
    PENDING: '排队中',
    LEASED: 'Agent 已领取',
    SUCCEEDED: '测试成功',
    FAILED: '测试失败',
    UNKNOWN: '结果未知',
    EXPIRED: '测试已过期',
    INVALIDATED: '测试已失效',
  }[status]
}

function connectionTestNodeLabel() {
  const nodeID = connectionTest.value?.nodeId ?? connectionTestRequest.value?.nodeId
  return connectionTest.value?.nodeDisplayName ?? nodeCandidates.value.find((candidate) => candidate.id === nodeID)?.displayName ?? nodeID ?? '未指定'
}

function verificationSourceLabel(source: DataSourceConnectionTest['verificationSource']) {
  return source === 'G2_SYNTHETIC' ? 'G2 合成验证' : 'Agent 固定 JDBC 探针'
}

function clearFormErrors() {
  for (const field of Object.keys(formErrors) as DataSourceFormField[]) delete formErrors[field]
}

function setFormErrors(errors: DataSourceFormErrors) {
  clearFormErrors()
  Object.assign(formErrors, errors)
}

function clearFieldError(field: DataSourceFormField) {
  delete formErrors[field]
}

function clearCompatibilityModeErrors() {
  clearFieldError('compatibilityMode')
  clearFieldError('defaultDatabase')
}

function fieldErrorID(field: DataSourceFormField) {
  return formErrors[field] ? `data-source-${field}-error` : undefined
}
</script>

<template>
  <div class="data-source-form-view is-drawer">
    <div class="data-source-form-scroll">
      <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
      <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
      <section v-if="loading" class="loading-state">正在加载数据源…</section>
      <section v-else-if="loadFailed" class="empty-state">
        <h2>无法打开数据源</h2>
        <p>{{ failure }}</p>
        <WorkbenchButton @click="loadPage">重试</WorkbenchButton>
      </section>
      <div v-else class="form-layout drawer-form-layout">
        <form class="form-card data-source-form-card" @submit.prevent="save()">
          <fieldset class="form-fields" :disabled="busy">
            <section class="data-source-form-section">
              <h2>基本信息</h2>
              <div class="form-grid basic-information-grid">
                <WorkbenchFormField class="field-label" label="数据源名称" required :error="formErrors.displayName" :error-id="fieldErrorID('displayName')">
                  <input v-model.trim="form.displayName" maxlength="120" :aria-describedby="fieldErrorID('displayName')" :aria-invalid="formErrors.displayName ? 'true' : undefined" @input="clearFieldError('displayName')" />
                </WorkbenchFormField>
                <fieldset class="field-label" :aria-invalid="formErrors.environment ? 'true' : undefined">
                  <legend><span class="field-label-text">环境 <b>*</b></span></legend>
                  <div class="radio-row" :aria-describedby="fieldErrorID('environment')">
                    <label><input v-model="form.environment" type="radio" value="DEVELOPMENT" @change="clearFieldError('environment')" />开发</label>
                    <label><input v-model="form.environment" type="radio" value="TEST" @change="clearFieldError('environment')" />测试</label>
                    <label><input v-model="form.environment" type="radio" value="STAGING" @change="clearFieldError('environment')" />预生产</label>
                    <label><input v-model="form.environment" type="radio" value="PRODUCTION" @change="clearFieldError('environment')" />生产</label>
                  </div>
                  <p v-if="formErrors.environment" :id="fieldErrorID('environment')" class="field-error" role="alert">{{ formErrors.environment }}</p>
                </fieldset>
              </div>
            </section>

            <section ref="connectionSection" class="data-source-form-section" :class="connectionTestDiagnostic?.target === 'connection' ? 'has-connection-diagnostic' : undefined">
              <h2>连接信息</h2>
              <div v-if="connectionTestDiagnostic?.target === 'connection'" class="module-diagnostic" role="alert">
                <AlertTriangle :size="17" aria-hidden="true" />
                <div><strong>{{ connectionTestDiagnostic.title }}</strong><span>故障定位：{{ connectionTestDiagnostic.moduleLabel }}</span></div>
              </div>
              <p class="fixed-field">连接方式：<strong>私有 ODP</strong><span>当前版本固定，不提供切换。</span></p>
              <details class="connection-parser" :open="isNew">
                <summary><ChevronDown class="disclosure-icon" :size="15" aria-hidden="true" />使用连接串解析</summary>
                <div class="connection-parser-body">
                  <WorkbenchFormField class="field-label field-span" label="智能解析连接串" :error="connectionStringError" error-id="data-source-connection-string-error">
                    <input v-model="connectionString" type="text" autocomplete="off" placeholder="mysql 或 obclient 开头的 ODP 连接串" :aria-describedby="connectionStringError ? 'data-source-connection-string-error' : undefined" :aria-invalid="connectionStringError ? 'true' : undefined" @input="connectionStringError = ''" />
                  </WorkbenchFormField>
                  <WorkbenchButton :disabled="busy" @click="applyConnectionString">解析并填充</WorkbenchButton>
                  <p>原始连接串仅在当前页面解析，不会提交或保存。</p>
                </div>
              </details>

              <div class="form-grid">
                <WorkbenchFormField class="field-label" label="租户模式" required :error="formErrors.compatibilityMode" :error-id="fieldErrorID('compatibilityMode')">
                  <select v-model="form.compatibilityMode" :aria-describedby="fieldErrorID('compatibilityMode')" :aria-invalid="formErrors.compatibilityMode ? 'true' : undefined" @change="clearCompatibilityModeErrors">
                    <option value="MYSQL">OceanBase MySQL</option>
                    <option value="ORACLE">OceanBase Oracle</option>
                  </select>
                </WorkbenchFormField>
                <WorkbenchFormField class="field-label" label="ODP 地址" required :error="formErrors.host" :error-id="fieldErrorID('host')">
                  <input v-model.trim="form.host" maxlength="253" placeholder="IP、域名或 VIP" :aria-describedby="fieldErrorID('host')" :aria-invalid="formErrors.host ? 'true' : undefined" @input="clearFieldError('host')" />
                </WorkbenchFormField>
                <WorkbenchFormField class="field-label" label="SQL 端口" required :error="formErrors.port" :error-id="fieldErrorID('port')">
                  <input v-model.number="form.port" type="number" min="1" max="65535" :aria-describedby="fieldErrorID('port')" :aria-invalid="formErrors.port ? 'true' : undefined" @input="clearFieldError('port')" />
                </WorkbenchFormField>
                <WorkbenchFormField class="field-label" label="集群名" optional="可选" :error="formErrors.clusterName" :error-id="fieldErrorID('clusterName')">
                  <input v-model.trim="form.clusterName" maxlength="255" :aria-describedby="fieldErrorID('clusterName')" :aria-invalid="formErrors.clusterName ? 'true' : undefined" @input="clearFieldError('clusterName')" />
                </WorkbenchFormField>
                <WorkbenchFormField class="field-label" label="租户名" required :error="formErrors.tenantName" :error-id="fieldErrorID('tenantName')">
                  <input v-model.trim="form.tenantName" maxlength="255" :aria-describedby="fieldErrorID('tenantName')" :aria-invalid="formErrors.tenantName ? 'true' : undefined" @input="clearFieldError('tenantName')" />
                </WorkbenchFormField>
                <WorkbenchFormField class="field-label" label="用户名" required :error="formErrors.username" :error-id="fieldErrorID('username')">
                  <input v-model.trim="form.username" maxlength="256" autocomplete="username" :aria-describedby="fieldErrorID('username')" :aria-invalid="formErrors.username ? 'true' : undefined" @input="clearFieldError('username')" />
                </WorkbenchFormField>
                <WorkbenchFormField v-if="form.compatibilityMode === 'MYSQL'" class="field-label field-span" label="默认数据库" optional="可选" :error="formErrors.defaultDatabase" :error-id="fieldErrorID('defaultDatabase')">
                  <input v-model.trim="form.defaultDatabase" maxlength="512" :aria-describedby="fieldErrorID('defaultDatabase')" :aria-invalid="formErrors.defaultDatabase ? 'true' : undefined" @input="clearFieldError('defaultDatabase')" />
                </WorkbenchFormField>
                <WorkbenchFormField class="field-label field-span" label="密码" :required="isNew" :optional="isNew ? '' : '留空保留现有值'" :error="formErrors.password" :error-id="fieldErrorID('password')">
                  <span class="password-control">
                    <input v-model="form.password" :type="passwordVisible ? 'text' : 'password'" maxlength="4096" autocomplete="new-password" :placeholder="isNew ? '新增时必填；页面不会回显或保存密码' : '输入新密码才会轮换凭据'" :aria-describedby="fieldErrorID('password')" :aria-invalid="formErrors.password ? 'true' : undefined" @input="clearFieldError('password')" />
                    <button type="button" class="password-visibility" :aria-label="passwordVisible ? '隐藏新密码' : '显示新密码'" :aria-pressed="passwordVisible" @click="passwordVisible = !passwordVisible">
                      <EyeOff v-if="passwordVisible" :size="16" aria-hidden="true" />
                      <Eye v-else :size="16" aria-hidden="true" />
                    </button>
                  </span>
                </WorkbenchFormField>
                <details class="field-span sys-credential-panel">
                  <summary><ChevronDown class="disclosure-icon" :size="15" aria-hidden="true" />高级设置：sys 凭据（可选）</summary>
                  <p>可选。用于读取 sys 租户视图；未配置时相关导出能力自动降级。</p>
                  <div class="sys-credential-row">
                    <WorkbenchFormField class="field-label" label="sys 账号" :error="formErrors.sysUser" :error-id="fieldErrorID('sysUser')">
                      <input v-model.trim="form.sysUser" maxlength="256" autocomplete="off" placeholder="例如 root（勿填 @sys#集群 后缀）" :disabled="clearSysCredential" :aria-describedby="fieldErrorID('sysUser')" :aria-invalid="formErrors.sysUser ? 'true' : undefined" @input="clearFieldError('sysUser')" />
                    </WorkbenchFormField>
                    <WorkbenchFormField class="field-label" label="sys 密码" :error="formErrors.sysPassword" :error-id="fieldErrorID('sysPassword')">
                      <input v-model="form.sysPassword" type="password" maxlength="4096" autocomplete="new-password" placeholder="输入新密码才会设置或轮换 sys 凭据" :disabled="clearSysCredential" :aria-describedby="fieldErrorID('sysPassword')" :aria-invalid="formErrors.sysPassword ? 'true' : undefined" @input="clearFieldError('sysPassword')" />
                    </WorkbenchFormField>
                  </div>
                  <p v-if="!isNew && source" class="section-hint">当前 sys 凭据状态：{{ source.sysCredentialState === 'AVAILABLE' ? '已配置' : '未配置' }}。编辑时留空表示保持现状。</p>
                  <label v-if="!isNew && source?.sysCredentialState === 'AVAILABLE'" class="checkbox-row">
                    <input v-model="clearSysCredential" type="checkbox" @change="toggleClearSysCredential" />
                    清除当前 sys 凭据
                  </label>
                </details>
              </div>
            </section>
            <section ref="testSection" class="data-source-form-section connection-test-section" :class="connectionTestDiagnostic?.target === 'execution-node' ? 'has-connection-diagnostic' : undefined" aria-labelledby="data-source-connection-test-heading">
              <h2 id="data-source-connection-test-heading">通过执行节点测试连接</h2>
              <p>由所选执行节点验证网络、认证和基础数据库连接。</p>
              <div class="test-node-row">
                <label class="field-label">
                  <span class="field-label-text">执行节点 <b>*</b></span>
                  <select ref="testNodeSelect" v-model="selectedNodeID" :disabled="busy || nodeCandidatesLoading" aria-describedby="data-source-connection-test-node-help">
                    <option value="">{{ nodeCandidatesLoading ? '正在加载可测试节点…' : '请选择执行节点' }}</option>
                    <option v-for="candidate in nodeCandidates" :key="candidate.id" :value="candidate.id">{{ candidate.displayName }}（{{ candidate.platform }}）</option>
                  </select>
                </label>
                <WorkbenchIconButton label="刷新执行节点" :disabled="busy || nodeCandidatesLoading" @click="loadConnectionTestNodeCandidates"><RefreshCw :size="16" :class="{ 'is-spinning': nodeCandidatesLoading }" aria-hidden="true" /></WorkbenchIconButton>
                <WorkbenchButton :disabled="busy || isNew || nodeCandidatesLoading || !nodeCandidates.length || !selectedNodeID || hasUnsavedConnectionChanges" @click="testConnection"><template #icon><Unplug :size="16" aria-hidden="true" /></template>{{ testBusy ? '测试进行中…' : '测试连接' }}</WorkbenchButton>
              </div>
              <p v-if="isNew" id="data-source-connection-test-node-help" class="section-hint is-locked">请先保存数据源，再单独发起连接测试。</p>
              <p v-else-if="hasUnsavedConnectionChanges" id="data-source-connection-test-node-help" class="section-hint is-warning">连接配置存在未保存更改。请先保存，保存后才能重新测试。</p>
              <p v-else id="data-source-connection-test-node-help" class="section-hint">仅列出已关联、在线且空闲的节点；测试不会永久绑定数据源。</p>
              <p v-if="nodeCandidatesFailure" class="field-error" role="alert">{{ nodeCandidatesFailure }}</p>
              <p v-else-if="!nodeCandidatesLoading && nodeCandidates.length === 0" class="field-error" role="status">当前没有可用于连接测试的执行节点。请确认节点已关联、在线且空闲，并且当前身份拥有节点使用权限。</p>
              <p v-if="testFailure" class="field-error" role="alert">{{ testFailure }}</p>
            </section>
            <!-- 连接测试结果按诊断阶段展示，并提供返回相关表单模块的定位入口；页面仍只使用受控证据码。 -->
            <ConnectionTestResult v-if="connectionTestRequest || connectionTest" :state="testResultInvalidated ? 'invalidated' : connectionTestResultState" :title="testResultInvalidated ? '已有测试结果已失效' : connectionTestDiagnostic?.title ?? `连接测试${connectionTest ? connectionTestStatusLabel(connectionTest.status) : '排队中'}`">
              <template v-if="testResultInvalidated">
                <span>{{ hasUnsavedConnectionChanges ? '连接配置存在未保存更改。请先保存，再基于新的配置重新测试。' : '连接配置已变更，旧结果不再代表当前配置。请重新测试。' }}</span>
              </template>
              <template v-else>
                <span v-if="connectionTestDiagnostic">失败阶段：{{ connectionTestDiagnostic.stage }}</span>
                <template v-if="connectionTest">
                  <template v-if="connectionTestDiagnostic">
                    <p class="connection-test-summary">{{ connectionTestDiagnostic.summary }}</p>
                    <div class="connection-test-location"><span>异常模块</span><strong>{{ connectionTestDiagnostic.moduleLabel }}</strong><button type="button" data-connection-diagnostic-focus class="diagnostic-link" @click="focusConnectionTestDiagnostic">查看相关配置</button></div>
                    <div class="connection-test-checks"><strong>建议检查</strong><ol><li v-for="check in connectionTestDiagnostic.checks" :key="check">{{ check }}</li></ol></div>
                  </template>
                  <p v-else-if="isTerminalConnectionTestStatus(connectionTest.status)" class="connection-test-detail">{{ connectionTestResultDetail }}</p>
                  <p v-if="connectionTest.verificationSource === 'G2_SYNTHETIC'" class="synthetic-warning">G2 合成结果不代表数据源已连通，不能据此启用数据源或选择导出任务。</p>
                  <p v-if="connectionTestSysNotice" class="connection-test-sys">{{ connectionTestSysNotice }}</p>
                  <details class="connection-test-evidence">
                    <summary>技术信息</summary>
                    <dl><div><dt>执行节点</dt><dd>{{ connectionTestNodeLabel() }}</dd></div><div><dt>验证来源</dt><dd>{{ verificationSourceLabel(connectionTest.verificationSource) }}</dd></div><div v-if="connectionTest.agentId"><dt>关联 Agent</dt><dd>{{ connectionTest.agentId }}</dd></div><div v-if="connectionTest.factsRevision"><dt>节点事实版本</dt><dd>{{ connectionTest.factsRevision }}</dd></div><div v-if="connectionTest.resultCode"><dt>错误代码</dt><dd><code>{{ connectionTest.resultCode }}</code></dd></div><div v-if="connectionTest.completedAt"><dt>完成时间</dt><dd>{{ new Date(connectionTest.completedAt).toLocaleString() }}</dd></div></dl>
                  </details>
                </template>
                <span v-else>控制面已接受请求，正在等待所选节点 Agent 领取。</span>
              </template>
            </ConnectionTestResult>
          </fieldset>
        </form>
      </div>
    </div>
    <footer v-if="!loading && !loadFailed" class="page-footer data-source-form-footer">
      <WorkbenchButton :disabled="busy" @click="emit('cancel')">取消</WorkbenchButton>
      <span class="footer-grow" />
      <WorkbenchButton variant="primary" :disabled="busy" @click="save">{{ saveBusy ? '保存中…' : isNew ? '保存数据源' : '保存更改' }}</WorkbenchButton>
    </footer>
  </div>
</template>

<style scoped>
.data-source-form-view { color: var(--text-primary); }
.data-source-form-view.is-drawer { display: flex; height: 100%; min-height: 0; flex-direction: column; }
.data-source-form-scroll { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 0 24px; scrollbar-gutter: stable; scroll-padding-block: 16px; }
.form-layout.drawer-form-layout { display: block; }
.data-source-form-card { padding: 0 0 8px; border: 0; border-radius: 0; background: transparent; }
.form-fields { min-width: 0; margin: 0; padding: 0; border: 0; }
.data-source-form-section { padding: 24px 0; border-bottom: 1px solid var(--border-default); scroll-margin-block: 16px; }
.data-source-form-section:last-of-type { border-bottom: 0; }
.data-source-form-section h2 { margin: 0 0 16px; padding: 0; border: 0; color: var(--text-primary); font-size: var(--text-section-title-size); font-weight: 600; line-height: var(--text-section-title-line-height); }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.basic-information-grid { grid-template-columns: 1fr; }
.field-label { align-content: start; gap: 4px; }
.field-label legend, .field-label-text { color: var(--text-secondary); font-size: 12px; font-weight: 500; line-height: 18px; }
fieldset.field-label { min-width: 0; margin: 0; padding: 0; border: 0; }
.field-label input, .field-label select { height: var(--size-control); font-size: 13px; }
.field-label input:focus-visible, .field-label select:focus-visible { outline: 2px solid var(--border-focus); outline-offset: 2px; }
.field-label input[aria-invalid='true'], .field-label select[aria-invalid='true'] { border-color: var(--status-error); }
.field-label[aria-invalid='true'] .radio-row { outline: 1px solid var(--status-error); outline-offset: 4px; }
.field-span { grid-column: 1 / -1; }
.field-label-text { display: inline-flex; align-items: baseline; flex-wrap: wrap; gap: 4px; min-width: 0; }
.field-label-text b { color: var(--status-error); }
.password-control { position: relative; display: block; }
.password-control input { padding-right: 40px; }
.password-visibility { position: absolute; top: 2px; right: 2px; display: grid; width: 32px; height: 32px; place-items: center; padding: 0; border: 0; border-radius: 4px; color: var(--text-secondary); background: transparent; cursor: pointer; }
.password-visibility:hover { color: var(--text-primary); background: var(--surface-subtle); }
.radio-row { display: flex; flex-wrap: wrap; min-height: 32px; align-items: center; gap: 8px 24px; }
.radio-row label { gap: 8px; color: var(--text-primary); font-size: 13px; font-weight: 400; white-space: nowrap; }
.radio-row input { flex: none; width: 16px; height: 16px; margin: 0; }
.fixed-field { display: flex; flex-wrap: wrap; gap: 8px; margin: -4px 0 16px; padding: 0; color: var(--text-secondary); background: transparent; font-size: 12px; line-height: 18px; }
.fixed-field strong { color: var(--text-primary); font-weight: 500; }
.fixed-field span { color: var(--text-secondary); }
.connection-parser { margin: 0 0 16px; padding: 0 0 12px; border: 0; border-bottom: 1px solid var(--border-subtle); border-radius: 0; background: transparent; }
.connection-parser > summary, .sys-credential-panel > summary { display: flex; min-height: 32px; align-items: center; gap: 8px; color: var(--text-secondary); font-size: 13px; font-weight: 500; cursor: pointer; list-style: none; }
.connection-parser > summary::-webkit-details-marker, .sys-credential-panel > summary::-webkit-details-marker { display: none; }
.disclosure-icon { flex: none; transform: rotate(-90deg); transition: transform 120ms; }
details[open] > summary .disclosure-icon { transform: rotate(0); }
.connection-parser-body { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px 12px; align-items: end; padding: 12px 0 4px; }
.connection-parser-body .field-span { grid-column: auto; }
.connection-parser-body p { grid-column: 1 / -1; margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 18px; }
.sys-credential-panel { margin: 0; padding: 12px 0 0; border: 0; border-top: 1px solid var(--border-subtle); border-radius: 0; background: transparent; }
.sys-credential-panel > p { color: var(--text-secondary); }
.sys-credential-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; margin-top: 16px; }
.checkbox-row { display: flex; align-items: center; gap: 8px; margin-top: 12px; color: var(--text-primary); font-size: 13px; }
.checkbox-row input { width: 16px; height: 16px; margin: 0; }
.connection-test-section > p { margin: -8px 0 16px; color: var(--text-secondary); font-size: 12px; line-height: 18px; }
.test-node-row { display: grid; grid-template-columns: minmax(0, 1fr) 32px auto; gap: 8px; align-items: end; }
.test-node-row > :deep(.workbench-icon-button) { margin-bottom: 2px; }
.field-error { display: block; margin: 8px 0 0; color: var(--status-error); font-size: 12px; font-weight: 400; line-height: 18px; }
.section-hint { margin: 12px 0 0 !important; font-size: 12px; line-height: 18px; }
.section-hint.is-locked { padding: 8px 12px; border-left: 3px solid var(--border-default); background: var(--surface-subtle); }
.section-hint.is-warning { padding: 8px 12px; border-left: 3px solid var(--status-warning); color: var(--status-warning); background: var(--color-warning-bg); }
.data-source-form-footer { position: static; z-index: 2; flex: none; min-height: 64px; gap: 8px; margin: 0; padding: 12px 24px; border-top: 1px solid var(--border-default); background: var(--surface-primary); }
.connection-test-summary { margin: 0; color: var(--text-primary); font-size: 13px; line-height: 20px; }
.connection-test-location { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; padding: 8px 0; border-block: 1px solid var(--border-subtle); }
.connection-test-location > span { color: var(--text-secondary); }
.connection-test-location > strong { color: var(--text-primary); font-size: 13px; font-weight: 500; }
.diagnostic-link { min-height: 32px; padding: 0 8px; border: 0; border-radius: var(--radius-control); color: var(--interactive-primary); background: transparent; font-size: 12px; cursor: pointer; }
.diagnostic-link:hover { background: var(--interactive-selected); }
.connection-test-checks > strong { display: block; margin-bottom: 4px; color: var(--text-primary); font-size: 13px; }
.connection-test-checks ol { display: grid; gap: 4px; margin: 0; padding-left: 20px; color: var(--text-secondary); font-size: 12px; line-height: 18px; }
.connection-test-evidence { padding-top: 8px; border-top: 1px solid var(--border-subtle); }
.connection-test-evidence summary { width: fit-content; min-height: 24px; color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.connection-test-evidence dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 16px; margin: 8px 0 0; }
.connection-test-evidence dl div { min-width: 0; }
.connection-test-evidence dt { color: var(--text-secondary); font-size: 12px; }
.connection-test-evidence dd { margin: 4px 0 0; overflow-wrap: anywhere; color: var(--text-primary); font-size: 12px; }
.connection-test-evidence code { font-family: "Cascadia Mono", Consolas, ui-monospace, monospace; font-size: 12px; }
.synthetic-warning { color: var(--status-warning); }
.data-source-form-section.has-connection-diagnostic { position: relative; }
.data-source-form-section.has-connection-diagnostic::before { position: absolute; inset-block: 24px; left: -12px; width: 3px; background: var(--status-error); content: ""; }
.module-diagnostic { display: flex; align-items: flex-start; gap: 8px; margin: -8px 0 16px; padding: 12px; border-left: 3px solid var(--status-error); color: var(--status-error); background: var(--color-danger-bg); }
.module-diagnostic > svg { flex: none; margin-top: 1px; }
.module-diagnostic > div { display: grid; gap: 4px; }
.module-diagnostic strong { font-size: 13px; }
.module-diagnostic span { color: var(--text-secondary); font-size: 12px; line-height: 18px; }
.feedback { margin: 16px 0 0; padding: 12px; border: 0; border-left: 3px solid var(--status-info); border-radius: 0; color: var(--text-primary); background: var(--surface-subtle); font-size: 13px; line-height: 20px; }
.feedback-error { border-color: var(--status-error); color: var(--status-error); background: var(--color-danger-bg); }
.loading-state, .empty-state { margin: 24px 0; }
@media (max-width: 1280px) {
  .form-grid, .sys-credential-row { grid-template-columns: 1fr; }
}
@media (max-width: 560px) {
  .data-source-form-scroll { padding-inline: 16px; }
  .data-source-form-footer { padding-inline: 16px; }
  .connection-parser-body, .connection-test-location, .connection-test-evidence dl { grid-template-columns: 1fr; }
  .test-node-row { grid-template-columns: minmax(0, 1fr) 32px; }
  .test-node-row > :deep(.workbench-button) { grid-column: 1 / -1; justify-self: start; }
  .radio-row { gap: 8px 16px; }
  .connection-parser-body .field-span, .connection-parser-body p { grid-column: auto; }
}
</style>
