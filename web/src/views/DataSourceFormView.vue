<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertTriangle, ChevronDown, Eye, EyeOff } from '@lucide/vue'

import { browserApi, dataSourceErrorMessage, type DataSourceConnectionTest, type DataSourceConnectionTestRequest, type DataSourceDetail, type DataSourceUpdate, type DataSourceWrite, type ExecutionNodeCandidate } from '@/api/browser'
import { parseDataSourceConnectionString } from './dataSourceConnectionString'
import { dataSourceConnectionTestNotice, sysCredentialVerificationNotice } from './dataSourceConnectionTestNotice'
import { dataSourceConnectionTestDiagnostic } from './dataSourceConnectionTestDiagnostic'
import { dataSourceFieldErrorsFromApi, type DataSourceFormErrors, type DataSourceFormField, validateDataSourceForm } from './dataSourceFormErrors'

// 支持两种承载方式（参考 OMS 数据源设计）：独立路由页面（standalone=true）或列表页右侧抽屉（standalone=false）。
// dataSourceId 为空表示新增；缺省时回退到路由参数以兼容直接访问 /data-sources/:id。
const props = withDefaults(defineProps<{ dataSourceId?: string | null; standalone?: boolean; focusTest?: boolean }>(), { dataSourceId: null, standalone: true, focusTest: false })
const emit = defineEmits<{ saved: [id: string]; cancel: []; 'dirty-change': [dirty: boolean] }>()

const api = browserApi()
const route = useRoute()
const router = useRouter()
type DataSourceForm = { -readonly [Key in keyof DataSourceWrite]: DataSourceWrite[Key] }
type MutableDataSourceUpdate = { -readonly [Key in keyof DataSourceUpdate]: DataSourceUpdate[Key] }
const activeDataSourceID = ref(props.dataSourceId ?? '')
const dataSourceID = computed(() => activeDataSourceID.value || (typeof route.params.id === 'string' ? route.params.id : ''))
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
const testAfterCreate = ref(false)
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
    if (route.query.test === '1' && !connectionTestRequest.value) {
      notice.value = nodeCandidates.value.length
        ? '请选择执行节点后开始基础连接测试。'
        : '当前没有可测试节点；连接测试不会退化为控制面直连数据库。'
    }
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
const effectiveConnectionStatusInvalidated = computed(() => connectionStatusInvalidated.value || hasUnsavedConnectionChanges.value)

watch(hasUnsavedChanges, (dirty) => emit('dirty-change', dirty), { immediate: true })

function applyConnectionString() {
  const parsed = parseDataSourceConnectionString(connectionString.value)
  if (!parsed) {
    connectionStringError.value = '连接串无法按当前 OceanBase ODP 规则解析。请使用 mysql 或 obclient 开头，并提供地址、端口和用户@租户#集群。'
    return
  }
  Object.assign(form, parsed, { password: parsed.password || form.password })
  connectionStringError.value = ''
  clearFormErrors()
  failure.value = ''
  notice.value = '连接串已解析并回填结构化字段。原始连接串仅保留在当前浏览器页面，便于调整；保存和测试请求不会提交它。'
}

async function save(runTestAfterCreate = false) {
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
      // 创建已经成功后先通知父级刷新列表，后续详情刷新失败不能再误报为“保存失败”。
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
      // 抽屉承载时保持当前上下文并切换为编辑态。
      if (props.standalone) await router.replace(`/data-sources/${createdID}`)
      if (runTestAfterCreate && selectedNodeID.value) {
        testAfterCreate.value = false
        await testConnection()
      }
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

// 连接测试结果的就近高亮投影（参考 ODC：失败在对应操作位置醒目提示，错误码与可操作说明一并展示）。
const connectionTestResultState = computed(() => {
  const result = connectionTest.value
  if (!result) return connectionTestRequest.value ? 'pending' : ''
  if (result.status === 'SUCCEEDED') return 'success'
  if (result.status === 'FAILED' || result.status === 'EXPIRED' || result.status === 'INVALIDATED' || result.status === 'UNKNOWN') return 'error'
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
  target?.scrollIntoView({ behavior: 'smooth', block: 'start' })
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

async function saveAndTest() {
  if (!selectedNodeID.value) {
    testFailure.value = '请选择一个可测试执行节点。'
    return
  }
  testAfterCreate.value = true
  try {
    await save(true)
  } finally {
    testAfterCreate.value = false
  }
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
  <div class="data-source-form-view" :class="{ 'is-drawer': !props.standalone }">
    <div class="data-source-form-scroll">
      <!-- 独立路由页面显示页面标题；右侧抽屉承载时标题由抽屉提供。 -->
      <section v-if="props.standalone" class="page-heading">
        <div>
          <h1>{{ isNew ? '新增数据源' : '编辑数据源' }}</h1>
          <p>数据源只保存任务复用的基础连接信息；私有 ODP 是当前 V1.0 的固定连接方式。</p>
        </div>
      </section>
      <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
      <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
      <section v-if="loading" class="loading-state">正在加载数据源…</section>
      <section v-else-if="loadFailed" class="empty-state">
        <h2>无法打开数据源</h2>
        <p>{{ failure }}</p>
        <button type="button" class="button button-secondary" @click="loadPage">重试</button>
      </section>
      <!-- 独立页面保持双列（表单 + 状态侧栏）；抽屉内单列全宽，避免主要内容被压缩折叠 -->
      <div v-else class="form-layout" :class="{ 'drawer-form-layout': !props.standalone }">
        <form class="form-card data-source-form-card" @submit.prevent="save()">
          <section class="data-source-form-section">
            <h2>基本信息</h2>
            <div class="form-grid basic-information-grid">
              <label class="field-label">
                <span class="field-label-text">数据源名称 <b>*</b></span>
                <input v-model.trim="form.displayName" maxlength="120" :aria-describedby="fieldErrorID('displayName')" :aria-invalid="formErrors.displayName ? 'true' : undefined" @input="clearFieldError('displayName')" />
                <span v-if="formErrors.displayName" :id="fieldErrorID('displayName')" class="field-error" role="alert">{{ formErrors.displayName }}</span>
              </label>
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
                <label class="field-label field-span">
                  <span class="field-label-text">智能解析连接串</span>
                  <input v-model="connectionString" type="text" autocomplete="off" placeholder="mysql 或 obclient 开头的 ODP 连接串" :aria-describedby="connectionStringError ? 'data-source-connection-string-error' : undefined" :aria-invalid="connectionStringError ? 'true' : undefined" @input="connectionStringError = ''" />
                  <span v-if="connectionStringError" id="data-source-connection-string-error" class="field-error" role="alert">{{ connectionStringError }}</span>
                </label>
                <button type="button" class="button button-secondary" :disabled="busy" @click="applyConnectionString">解析并填充</button>
                <p>仅解析并回填结构化字段；原始连接串只保留在当前浏览器页面，不会提交、保存或进入审计。</p>
              </div>
            </details>

            <div class="form-grid">
              <label class="field-label">
                <span class="field-label-text">租户模式 <b>*</b></span>
                <select v-model="form.compatibilityMode" :aria-describedby="fieldErrorID('compatibilityMode')" :aria-invalid="formErrors.compatibilityMode ? 'true' : undefined" @change="clearCompatibilityModeErrors">
                  <option value="MYSQL">OceanBase MySQL</option>
                  <option value="ORACLE">OceanBase Oracle</option>
                </select>
                <span v-if="formErrors.compatibilityMode" :id="fieldErrorID('compatibilityMode')" class="field-error" role="alert">{{ formErrors.compatibilityMode }}</span>
              </label>
              <label class="field-label">
                <span class="field-label-text">ODP 地址 <b>*</b></span>
                <input v-model.trim="form.host" maxlength="253" placeholder="IP、域名或 VIP" :aria-describedby="fieldErrorID('host')" :aria-invalid="formErrors.host ? 'true' : undefined" @input="clearFieldError('host')" />
                <span v-if="formErrors.host" :id="fieldErrorID('host')" class="field-error" role="alert">{{ formErrors.host }}</span>
              </label>
              <label class="field-label">
                <span class="field-label-text">SQL 端口 <b>*</b></span>
                <input v-model.number="form.port" type="number" min="1" max="65535" :aria-describedby="fieldErrorID('port')" :aria-invalid="formErrors.port ? 'true' : undefined" @input="clearFieldError('port')" />
                <span v-if="formErrors.port" :id="fieldErrorID('port')" class="field-error" role="alert">{{ formErrors.port }}</span>
              </label>
              <label class="field-label">
                <span class="field-label-text">集群名 <b>*</b></span>
                <input v-model.trim="form.clusterName" maxlength="255" :aria-describedby="fieldErrorID('clusterName')" :aria-invalid="formErrors.clusterName ? 'true' : undefined" @input="clearFieldError('clusterName')" />
                <span v-if="formErrors.clusterName" :id="fieldErrorID('clusterName')" class="field-error" role="alert">{{ formErrors.clusterName }}</span>
              </label>
              <label class="field-label">
                <span class="field-label-text">租户名 <b>*</b></span>
                <input v-model.trim="form.tenantName" maxlength="255" :aria-describedby="fieldErrorID('tenantName')" :aria-invalid="formErrors.tenantName ? 'true' : undefined" @input="clearFieldError('tenantName')" />
                <span v-if="formErrors.tenantName" :id="fieldErrorID('tenantName')" class="field-error" role="alert">{{ formErrors.tenantName }}</span>
              </label>
              <label class="field-label">
                <span class="field-label-text">用户名 <b>*</b></span>
                <input v-model.trim="form.username" maxlength="256" autocomplete="username" :aria-describedby="fieldErrorID('username')" :aria-invalid="formErrors.username ? 'true' : undefined" @input="clearFieldError('username')" />
                <span v-if="formErrors.username" :id="fieldErrorID('username')" class="field-error" role="alert">{{ formErrors.username }}</span>
              </label>
              <label v-if="form.compatibilityMode === 'MYSQL'" class="field-label field-span">
                <span class="field-label-text">默认数据库 <span class="optional">可选</span></span>
                <input v-model.trim="form.defaultDatabase" maxlength="512" :aria-describedby="fieldErrorID('defaultDatabase')" :aria-invalid="formErrors.defaultDatabase ? 'true' : undefined" @input="clearFieldError('defaultDatabase')" />
                <span v-if="formErrors.defaultDatabase" :id="fieldErrorID('defaultDatabase')" class="field-error" role="alert">{{ formErrors.defaultDatabase }}</span>
              </label>
              <label class="field-label field-span">
                <span class="field-label-text">密码 <b v-if="isNew">*</b><span v-else class="optional">留空保留现有值</span></span>
                <span class="password-control">
                  <input v-model="form.password" :type="passwordVisible ? 'text' : 'password'" maxlength="4096" autocomplete="new-password" :placeholder="isNew ? '新增时必填；页面不会回显或保存密码' : '输入新密码才会轮换凭据'" :aria-describedby="fieldErrorID('password')" :aria-invalid="formErrors.password ? 'true' : undefined" @input="clearFieldError('password')" />
                  <button type="button" class="password-visibility" :aria-label="passwordVisible ? '隐藏新密码' : '显示新密码'" :aria-pressed="passwordVisible" @click="passwordVisible = !passwordVisible">
                    <EyeOff v-if="passwordVisible" :size="16" aria-hidden="true" />
                    <Eye v-else :size="16" aria-hidden="true" />
                  </button>
                </span>
                <span v-if="formErrors.password" :id="fieldErrorID('password')" class="field-error" role="alert">{{ formErrors.password }}</span>
              </label>
              <details class="field-span sys-credential-panel">
                <summary><ChevronDown class="disclosure-icon" :size="15" aria-hidden="true" />高级设置：sys 凭据（可选）</summary>
                <p>拥有 sys 租户视图查看权限的账号（如 root）与密码，用于查询租户视图以提升导出能力；不配置时相关能力自动降级（参考 ODC 数据源高级设置）。</p>
                <div class="sys-credential-row">
                  <label class="field-label">
                    <span class="field-label-text">sys 账号</span>
                    <input v-model.trim="form.sysUser" maxlength="256" autocomplete="off" placeholder="例如 root（勿填 @sys#集群 后缀）" :disabled="clearSysCredential" :aria-describedby="fieldErrorID('sysUser')" :aria-invalid="formErrors.sysUser ? 'true' : undefined" @input="clearFieldError('sysUser')" />
                  </label>
                  <label class="field-label">
                    <span class="field-label-text">sys 密码</span>
                    <input v-model="form.sysPassword" type="password" maxlength="4096" autocomplete="new-password" placeholder="输入新密码才会设置或轮换 sys 凭据" :disabled="clearSysCredential" :aria-describedby="fieldErrorID('sysPassword')" :aria-invalid="formErrors.sysPassword ? 'true' : undefined" @input="clearFieldError('sysPassword')" />
                    <span v-if="formErrors.sysPassword" :id="fieldErrorID('sysPassword')" class="field-error" role="alert">{{ formErrors.sysPassword }}</span>
                  </label>
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
            <p>由所选执行节点发起数据源网络、认证和基础数据库连接测试；控制面不会直接连接数据库。</p>
            <div class="test-node-row">
              <label class="field-label">
                <span class="field-label-text">执行节点 <b>*</b></span>
                <select ref="testNodeSelect" v-model="selectedNodeID" :disabled="busy || nodeCandidatesLoading" aria-describedby="data-source-connection-test-node-help">
                  <option value="">{{ nodeCandidatesLoading ? '正在加载可测试节点…' : '请选择执行节点' }}</option>
                  <option v-for="candidate in nodeCandidates" :key="candidate.id" :value="candidate.id">{{ candidate.displayName }}（{{ candidate.platform }}）</option>
                </select>
              </label>
              <button type="button" class="button button-secondary" :disabled="busy || nodeCandidatesLoading" @click="loadConnectionTestNodeCandidates">刷新节点</button>
              <button type="button" class="button button-secondary" :disabled="busy || isNew || nodeCandidatesLoading || !nodeCandidates.length || !selectedNodeID || hasUnsavedConnectionChanges" @click="testConnection">{{ testBusy ? '测试进行中…' : '测试连接' }}</button>
            </div>
            <p v-if="isNew" id="data-source-connection-test-node-help" class="section-hint is-locked">可以预先选择合格节点；执行真实测试时会先保存数据源，再以已保存的配置和凭据版本发起测试。</p>
            <p v-else-if="hasUnsavedConnectionChanges" id="data-source-connection-test-node-help" class="section-hint is-warning">连接配置存在未保存更改。请先保存，保存后才能重新测试。</p>
            <p v-else id="data-source-connection-test-node-help" class="section-hint">候选节点必须已关联、在线且空闲；维护中或已归档节点不可用于测试。每次测试只针对当前选择的一个节点，不会永久绑定数据源。</p>
            <p v-if="nodeCandidatesFailure" class="field-error" role="alert">{{ nodeCandidatesFailure }}</p>
            <p v-else-if="!nodeCandidatesLoading && nodeCandidates.length === 0" class="field-error" role="status">当前没有可用于连接测试的执行节点。请确认节点已关联、在线且空闲，并且当前身份拥有节点使用权限。</p>
            <p v-if="testFailure" class="field-error" role="alert">{{ testFailure }}</p>
          </section>
          <!-- 连接测试结果按诊断阶段展示，并提供返回相关表单模块的定位入口；页面仍只使用受控证据码。 -->
          <section v-if="connectionTestRequest || connectionTest" class="test-result" :class="hasUnsavedConnectionChanges ? 'connection-test-result-invalidated' : `connection-test-result-${connectionTestResultState}`" aria-live="polite">
            <template v-if="hasUnsavedConnectionChanges">
              <strong>已有测试结果已失效</strong>
              <span>连接配置存在未保存更改。保存后才能基于新的配置重新执行真实连接测试。</span>
            </template>
            <template v-else>
              <div class="test-result-heading">
                <span class="test-result-icon" aria-hidden="true"><AlertTriangle v-if="connectionTestResultState === 'error'" :size="18" /></span>
                <div><strong>{{ connectionTestDiagnostic?.title ?? `连接测试${connectionTest ? connectionTestStatusLabel(connectionTest.status) : '排队中'}` }}</strong><span v-if="connectionTestDiagnostic">失败阶段：{{ connectionTestDiagnostic.stage }}</span></div>
              </div>
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
          </section>
        </form>
        <aside class="detail-aside">
          <section class="content-card">
            <h2>数据源状态</h2>
            <dl>
              <div><dt>连接状态</dt><dd>{{ effectiveConnectionStatusInvalidated ? '连接信息已变更，需重新测试' : source?.lastTestStatus === 'SUCCEEDED' ? '可连接' : source?.lastTestStatus === 'FAILED' ? '连接失败' : source?.lastTestStatus === 'UNKNOWN' ? '结果未知' : source?.lastTestStatus === 'EXPIRED' ? '测试已过期' : source?.lastTestStatus === 'INVALIDATED' ? '测试已失效' : source?.lastTestStatus === 'PENDING' ? '测试已请求' : '未测试' }}</dd></div>
              <div><dt>最近测试时间</dt><dd>{{ effectiveConnectionStatusInvalidated ? '已因连接信息变更失效' : source?.lastTestedAt ? new Date(source.lastTestedAt).toLocaleString() : source?.lastTestStatus ? '控制面未返回完成时间' : '尚未测试' }}</dd></div>
              <div><dt>启用状态</dt><dd>{{ source ? (source.state === 'ENABLED' ? '已启用' : '已禁用') : '保存并成功测试后可启用' }}</dd></div>
            </dl>
          </section>
          <section class="content-card">
            <h2>连接测试说明</h2>
            <p>不会检查导入/导出权限、对象权限、性能，也不会判断任务是否一定可执行。</p>
          </section>
        </aside>
      </div>
    </div>
    <footer v-if="!loading && !loadFailed" class="page-footer data-source-form-footer">
      <button type="button" class="button button-secondary" @click="props.standalone ? router.push('/data-sources') : emit('cancel')">取消</button>
      <span class="footer-grow" />
      <button v-if="isNew && selectedNodeID" type="button" class="button button-secondary" :disabled="busy" @click="save()">仅保存</button>
      <button type="button" class="button button-primary" :disabled="busy" @click="isNew && selectedNodeID ? saveAndTest() : save()">{{ saveBusy ? (testAfterCreate ? '保存并测试中…' : '保存中…') : isNew && selectedNodeID ? '保存并测试' : isNew ? '保存数据源' : '保存更改' }}</button>
    </footer>
  </div>
</template>

<style scoped>
.data-source-form-view {
  color: var(--color-text-primary);
}

.data-source-form-view.is-drawer {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
}

.data-source-form-scroll {
  min-height: 0;
}

.is-drawer .data-source-form-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 0 var(--space-6);
  scrollbar-gutter: stable;
}

.form-layout.drawer-form-layout {
  display: block;
}

.form-layout.drawer-form-layout .detail-aside {
  display: none;
}

.data-source-form-card {
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
}

.data-source-form-section {
  padding: var(--space-6) 0;
  border-bottom: 1px solid var(--color-border-default);
}

.data-source-form-section:last-of-type {
  border-bottom: 0;
}

.data-source-form-section h2 {
  margin: 0 0 var(--space-4) !important;
  padding: 0 !important;
  border: 0 !important;
  color: #252a31;
  font-size: 15px;
  font-weight: 600;
  line-height: 22px;
}

.form-grid {
  column-gap: var(--space-4);
  row-gap: var(--space-4);
}

.basic-information-grid {
  grid-template-columns: 1fr;
}

.field-label,
.field-label legend {
  color: #4a525c;
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
}

fieldset.field-label {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.field-label input,
.field-label select {
  height: var(--size-control);
  border-color: var(--color-border-strong);
  border-radius: var(--radius-control);
  color: #2b3037;
  font-family: inherit;
  font-size: 13px;
  font-weight: 400;
  line-height: 1;
}

.field-label input:focus,
.field-label select:focus {
  border-color: var(--color-primary);
  outline: 2px solid rgb(37 103 185 / 14%);
  outline-offset: 0;
}

.password-control { position: relative; display: block; }
.password-control input { padding-right: 38px; }
.password-visibility {
  position: absolute;
  top: 2px;
  right: 2px;
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 3px;
  color: var(--color-text-tertiary);
  background: transparent;
  cursor: pointer;
}
.password-visibility:hover { color: var(--color-text-primary); background: var(--color-bg-subtle); }
.password-visibility:focus-visible { outline: 2px solid rgb(37 103 185 / 30%); outline-offset: 0; }

.field-label input[aria-invalid='true'],
.field-label select[aria-invalid='true'] {
  border-color: #d94841;
}

.field-label[aria-invalid='true'] .radio-row {
  padding: 7px 9px;
  border: 1px solid #d94841;
  border-radius: 4px;
}

.radio-row {
  min-height: 36px;
  align-items: center;
  gap: 10px 16px;
}

.radio-row label {
  white-space: nowrap;
  font-weight: 400;
}

.radio-row input {
  flex: none;
  width: 16px;
  height: 16px;
  margin: 0;
}

.field-error {
  display: block;
  margin: 0;
  color: #b42318;
  font-size: 12px;
  font-weight: 400;
  line-height: 18px;
}

.fixed-field {
  margin: -2px 0 16px;
  padding: 0;
  color: #526174;
  background: transparent;
}

.fixed-field strong {
  color: #2f4057;
}

.connection-parser {
  margin: 0 0 18px;
  padding: 14px 0 0;
  border: 0;
  border-top: 1px solid #e2e7ed;
  border-radius: 0;
  background: transparent;
}

.field-label {
  align-content: start;
  gap: var(--space-1);
}

.field-label-text {
  display: inline-flex;
  align-items: baseline;
  gap: var(--space-1);
  min-width: 0;
  white-space: nowrap;
}

.field-label-text b {
  flex: none;
  color: #c53b32;
  font-weight: 600;
}

.field-label-text .optional {
  margin-left: var(--space-1);
  color: var(--color-text-tertiary);
  font-size: 12px;
  font-weight: 400;
}

.connection-parser > summary {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0;
  color: #414851;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  list-style: none;
}

.connection-parser > summary::-webkit-details-marker,
.sys-credential-panel summary::-webkit-details-marker { display: none; }
.disclosure-icon { flex: none; color: var(--color-text-tertiary); transform: rotate(-90deg); transition: transform 120ms ease; }
details[open] > summary .disclosure-icon { transform: rotate(0deg); }

.connection-parser-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 9px 12px;
  align-items: end;
  padding: 12px 0 0;
}

.connection-parser-body p {
  grid-column: 1 / -1;
  margin: 0;
  color: #6b778a;
  font-size: 12px;
  line-height: 18px;
}

.sys-credential-panel {
  margin-top: 4px;
  padding: 14px 0 0;
  border: 0;
  border-top: 1px solid #e2e7ed;
  border-radius: 0;
  background: transparent;
}

.sys-credential-panel summary {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: #414851;
  list-style: none;
}

.sys-credential-panel > p {
  color: #6b778a;
}

.checkbox-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  color: #8f332d;
  font-size: 13px;
}

.checkbox-row input {
  width: 15px;
  height: 15px;
  margin: 0;
}

.connection-test-section > p {
  margin: -7px 0 14px;
  color: #66758a;
  font-size: 12px;
  line-height: 19px;
}

.test-node-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: 10px;
  align-items: end;
}

.section-hint {
  margin: 10px 0 0 !important;
}

.section-hint.is-locked {
  padding: 8px 10px;
  border-left: 3px solid #aeb9c7;
  color: #66758a;
  background: #f6f7f9;
}

.section-hint.is-warning {
  padding: 8px 10px;
  border-left: 3px solid #d7a74e;
  color: #80551a;
  background: #fff8e9;
}

.button {
  height: var(--size-control);
  border-radius: var(--radius-control);
  font-size: 13px;
  font-weight: 500;
}

.button.button-primary {
  background: var(--color-primary);
}

.button.button-primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.button:focus-visible {
  outline: 2px solid rgb(37 103 185 / 30%);
  outline-offset: 2px;
}

.test-result {
  display: grid;
  gap: var(--space-3);
  margin: 0 0 24px;
  padding: var(--space-4);
  border: 1px solid #b9ceeb;
  border-radius: 4px;
  color: #526174;
  background: #f2f6fc;
}

.test-result strong {
  color: #245da8;
  font-size: 14px;
}

.test-result span,
.test-result p {
  margin: 0;
  color: #5d6d82;
  font-size: 12px;
  line-height: 19px;
}

.test-result.connection-test-result-error {
  border-color: #e7b8b3;
  background: #fff2f0;
}

.test-result.connection-test-result-error strong,
.test-result.connection-test-result-error .connection-test-detail {
  color: #b42318;
}

.test-result.connection-test-result-success {
  border-color: #b9dec9;
  background: #edf8f2;
}

.test-result.connection-test-result-success strong {
  color: #19734a;
}

.test-result.connection-test-result-invalidated {
  border-color: #e8d1a2;
  background: #fff7e6;
}

.test-result.connection-test-result-invalidated strong {
  color: #8a5a12;
}

.synthetic-warning {
  color: #8a5a12 !important;
}

.data-source-form-footer {
  flex: none;
}

.is-drawer .data-source-form-footer {
  position: static;
  z-index: 2;
  margin: 0;
  padding: var(--space-3) var(--space-6);
  border-top: 1px solid var(--color-border-default);
  background: var(--color-bg-surface);
}

.test-result-heading {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
}

.test-result-heading > div {
  display: grid;
  gap: 2px;
}

.test-result-icon {
  display: grid;
  width: 24px;
  height: 24px;
  flex: none;
  place-items: center;
  color: var(--color-danger);
}

.connection-test-summary {
  color: #3f4650 !important;
  font-size: 13px !important;
  line-height: 20px !important;
}

.connection-test-location {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-2);
  padding: 9px 10px;
  border: 1px solid var(--color-danger-border);
  border-radius: var(--radius-control);
  background: #fff;
}

.connection-test-location > span {
  color: var(--color-text-tertiary);
}

.connection-test-location > strong {
  color: #542925 !important;
  font-size: 13px;
}

.diagnostic-link {
  min-height: 28px;
  padding: 0 var(--space-2);
  border: 1px solid #d5a39f;
  border-radius: var(--radius-control);
  color: #8f2c25;
  background: #fff;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.diagnostic-link:hover { background: #fff6f5; }

.connection-test-checks > strong {
  display: block;
  margin-bottom: var(--space-1);
  color: #3f4650 !important;
  font-size: 13px;
}

.connection-test-checks ol {
  display: grid;
  gap: var(--space-1);
  margin: 0;
  padding-left: 22px;
  color: #525a64;
  font-size: 12px;
  line-height: 19px;
}

.connection-test-evidence {
  padding-top: var(--space-2);
  border-top: 1px solid rgb(166 44 36 / 14%);
}

.connection-test-evidence summary {
  width: fit-content;
  color: #6a4c49;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
}

.connection-test-evidence dl {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2) var(--space-4);
  margin: var(--space-2) 0 0;
}

.connection-test-evidence dl div { min-width: 0; }
.connection-test-evidence dt { color: var(--color-text-tertiary); font-size: 11px; }
.connection-test-evidence dd { margin: 2px 0 0; overflow-wrap: anywhere; color: var(--color-text-secondary); font-size: 12px; }
.connection-test-evidence code { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 11px; }

.data-source-form-section.has-connection-diagnostic {
  position: relative;
}

.data-source-form-section.has-connection-diagnostic::before {
  position: absolute;
  inset-block: var(--space-4);
  left: calc(var(--space-3) * -1);
  width: 3px;
  border-radius: 2px;
  background: var(--color-danger);
  content: "";
}

.module-diagnostic {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: calc(var(--space-2) * -1) 0 var(--space-4);
  padding: 9px 10px;
  border: 1px solid var(--color-danger-border);
  border-radius: var(--radius-control);
  color: var(--color-danger);
  background: var(--color-danger-bg);
}

.module-diagnostic > svg { flex: none; margin-top: 1px; }
.module-diagnostic > div { display: grid; gap: 2px; }
.module-diagnostic strong { font-size: 13px; }
.module-diagnostic span { color: #74504d; font-size: 12px; line-height: 18px; }

.is-drawer .data-source-form-card {
  padding-bottom: var(--space-2);
}

.is-drawer .feedback {
  margin-top: 16px;
}

.is-drawer .loading-state,
.is-drawer .empty-state {
  margin: 24px 0;
}

@media (max-width: 560px) {
  .is-drawer .data-source-form-scroll {
    padding-inline: 18px;
  }

  .form-grid,
  .sys-credential-row,
  .connection-parser-body,
  .test-node-row {
    grid-template-columns: 1fr;
  }

  .field-span,
  .connection-parser-body p {
    grid-column: auto;
  }

  .is-drawer .data-source-form-footer {
    padding-inline: 18px;
  }

  .connection-test-location,
  .connection-test-evidence dl {
    grid-template-columns: 1fr;
  }

  .diagnostic-link { width: fit-content; }
}
</style>
