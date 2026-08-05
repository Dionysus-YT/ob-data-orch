<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { browserApi, dataSourceErrorMessage, exportDraftErrorMessage, type CommandPreview, type DataSourceSummary, type ExecutionNodeCandidate, type ExportDraft, type Precheck, type PrecheckResult } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import WizardFrame from '@/components/WizardFrame.vue'
import { isExportEligibleDataSource } from './exportDataSourceEligibility'
import { validateExportDraftInput } from './exportDraftInput'
import { fixedPrecheckChecks, precheckCheckLabel, precheckResultBlocksSubmission, precheckResultDetail, precheckResultLabel } from './exportPrecheckPresentation'

const api = browserApi()
const route = useRoute()
const router = useRouter()
const sources = ref<DataSourceSummary[]>([])
const nodes = ref<ExecutionNodeCandidate[]>([])
const loadingSources = ref(true)
const loadingNodes = ref(true)
const sourceLoadFailure = ref('')
const nodeLoadFailure = ref('')
const draftFailure = ref('')
const draftNotice = ref('')
const creatingDraft = ref(false)
const createdDraftID = ref('')
const currentDraft = ref<ExportDraft | null>(null)
const loadingDraft = ref(false)
const draftLoadFailure = ref('')
const commandPreview = ref<CommandPreview | null>(null)
const previewingCommand = ref(false)
const commandPreviewFailure = ref('')
const activePrecheck = ref<Precheck | null>(null)
const precheckID = ref('')
const startingPrecheck = ref(false)
const precheckFailure = ref('')
const submitting = ref(false)
const submissionFailure = ref('')
const selectedDataSourceID = ref('')
const selectedNodeID = ref('')
const database = ref('')
const table = ref('')
const filePath = ref('')
const logPath = ref('')
const skipCheckDir = ref(false)
const activeStep = computed(() => {
  const value = Number(route.query.step ?? '1')
  return Number.isInteger(value) && value >= 1 && value <= 6 ? value : 1
})

const titles = ['选择数据源', '选择导出对象', '选择导出内容', '选择数据格式', '执行与输出', '预检查与命令']
const eligibleSources = computed(() => sources.value.filter(isExportEligibleDataSource))
const selectedSource = computed(() => eligibleSources.value.find((source) => source.id === selectedDataSourceID.value))
const selectedNode = computed(() => nodes.value.find((node) => node.id === selectedNodeID.value))
const displayedDraftConfig = computed(() => currentDraft.value?.config)
const displayedSource = computed(() => {
  const sourceID = currentDraft.value?.dataSourceId ?? selectedDataSourceID.value
  return sources.value.find((source) => source.id === sourceID)
})
const displayedNode = computed(() => {
  const nodeID = currentDraft.value?.nodeId ?? selectedNodeID.value
  return nodes.value.find((node) => node.id === nodeID)
})
const precheckRunning = computed(() => activePrecheck.value?.status === 'PENDING' || activePrecheck.value?.status === 'LEASED' || (Boolean(precheckID.value) && !activePrecheck.value && !precheckFailure.value))
const precheckRows = computed(() => fixedPrecheckChecks.map((check) => ({
  check,
  result: activePrecheck.value?.results.find((result) => result.check === check),
})))
const blockingPrecheckRows = computed(() => precheckRows.value.filter((row) => precheckResultBlocksSubmission(row.result)))
const canSubmit = computed(() => activePrecheck.value?.status === 'SUCCEEDED' && activePrecheck.value.integrityStatus === 'COMPLETE' && Boolean(currentDraft.value) && !submitting.value)
const footerBaselineNote = computed(() => {
	if (activeStep.value === 6) return canSubmit.value ? '预检查已通过。提交后，所选 Agent 将领取已冻结的导出任务并启动 OBDUMPER。' : (currentDraft.value ? '草稿已保存；请先完成当前版本的固定预检查，提交后才会启动 OBDUMPER。' : '尚未读取草稿；请返回上一步完成固定字段并创建草稿。')
  return '仅在固定字段完整时才创建草稿；服务端会再次校验数据源和节点授权。'
})
const objectInputMessage = computed(() => {
  if (!database.value.trim()) return '请填写默认数据库或 Schema。'
  if (!table.value.trim() || table.value.includes('*') || table.value.includes(',')) return '首条切片只支持一个明确的表名，不能使用通配符或多个表。'
  return ''
})
const draftValidation = computed(() => validateExportDraftInput({
  dataSourceId: selectedDataSourceID.value,
  nodeId: selectedNodeID.value,
  platform: selectedNode.value?.platform ?? '',
  database: database.value,
  table: table.value,
  filePath: filePath.value,
  logPath: logPath.value,
  skipCheckDir: skipCheckDir.value,
}))
const draftInput = computed(() => draftValidation.value.valid ? draftValidation.value.input : undefined)
const draftValidationMessage = computed(() => draftValidation.value.valid ? '' : draftValidation.value.message)
const canAdvance = computed(() => {
  if (activeStep.value === 1) return Boolean(selectedSource.value)
  if (activeStep.value === 2) return Boolean(selectedSource.value) && !objectInputMessage.value
  if (activeStep.value === 5) return Boolean(draftInput.value) && !creatingDraft.value
  return Boolean(selectedSource.value) && activeStep.value < 5
})
const footerLabel = computed(() => {
  if (activeStep.value === 5) return creatingDraft.value ? '正在创建草稿…' : '创建 CSV 草稿'
  if (activeStep.value === 6) {
    if (startingPrecheck.value) return '正在发起预检查…'
    if (precheckRunning.value) return '预检查进行中…'
    if (precheckID.value && !activePrecheck.value) return '重新读取预检查'
    return activePrecheck.value ? '重新执行预检查' : '执行预检查'
  }
  return '下一步'
})
const outputPathPlaceholder = computed(() => selectedNode.value?.platform === 'WINDOWS_AMD64'
  ? '例如 /E:/exports/daily'
  : '例如 /var/ob-data-orch/exports/daily')
let precheckPollTimer: ReturnType<typeof setTimeout> | undefined
let precheckPollResolve: (() => void) | undefined
let precheckPollVersion = 0

watch(selectedSource, (source, previousSource) => {
  if (source?.id === previousSource?.id) return
  database.value = source?.defaultDatabase ?? ''
  table.value = ''
  clearDraftState()
})

watch([database, table, selectedNodeID, filePath, logPath, skipCheckDir], () => {
  if (!createdDraftID.value) return
  clearDraftState()
})

onMounted(() => {
  void loadSources()
  void loadNodeCandidates()
})
onBeforeUnmount(stopPrecheckPolling)

async function loadSources() {
  loadingSources.value = true
  sourceLoadFailure.value = ''
  try {
    sources.value = await api.listDataSources()
    if (!eligibleSources.value.some((source) => source.id === selectedDataSourceID.value)) selectedDataSourceID.value = ''
  } catch (error) {
    sourceLoadFailure.value = dataSourceErrorMessage(error, '无法加载可选数据源，请稍后重试。')
  } finally {
    loadingSources.value = false
  }
}

async function loadNodeCandidates() {
  loadingNodes.value = true
  nodeLoadFailure.value = ''
  try {
    nodes.value = await api.listExportNodeCandidates()
    if (!nodes.value.some((node) => node.id === selectedNodeID.value)) selectedNodeID.value = ''
  } catch (error) {
    nodeLoadFailure.value = exportDraftErrorMessage(error, '无法加载可选执行节点，请稍后重试。')
  } finally {
    loadingNodes.value = false
  }
}

function moveToStep(step: number) {
  void router.replace({ query: { ...route.query, step: String(step) } })
}

function previousStep() {
  if (activeStep.value > 1) moveToStep(activeStep.value - 1)
}

function nextStep() {
  if (activeStep.value === 5) {
    void createDraft()
    return
  }
  if (activeStep.value < 5 && canAdvance.value) moveToStep(activeStep.value + 1)
}

async function createDraft() {
  if (!draftInput.value) return
  creatingDraft.value = true
  clearDraftState()
  try {
    const draftID = await api.createExportDraft(draftInput.value)
    createdDraftID.value = draftID
    draftNotice.value = '固定单表 CSV 草稿已创建，正在读取服务端配置快照。'
    moveToStep(6)
    await loadCreatedDraft(draftID)
  } catch (error) {
    draftFailure.value = exportDraftErrorMessage(error, '无法创建导出草稿，请检查当前配置后重试。')
  } finally {
    creatingDraft.value = false
  }
}

function clearDraftState() {
  stopPrecheckPolling()
  createdDraftID.value = ''
  currentDraft.value = null
  draftFailure.value = ''
  draftLoadFailure.value = ''
  draftNotice.value = ''
  commandPreview.value = null
  commandPreviewFailure.value = ''
  activePrecheck.value = null
  precheckID.value = ''
  precheckFailure.value = ''
  submissionFailure.value = ''
}

async function loadCreatedDraft(draftID = createdDraftID.value) {
  if (!draftID) return
  loadingDraft.value = true
  draftLoadFailure.value = ''
  commandPreview.value = null
  commandPreviewFailure.value = ''
  try {
    const draft = await api.getExportDraft(draftID)
    if (createdDraftID.value !== draftID) return
    currentDraft.value = draft
    draftNotice.value = '固定单表 CSV 草稿已从服务端读取。完整命令仅隐藏密码，其余参数由控制面生成并经本地校验后展示。'
    await loadCommandPreview(draft)
  } catch (error) {
    if (createdDraftID.value !== draftID) return
    currentDraft.value = null
    draftLoadFailure.value = exportDraftErrorMessage(error, '无法读取刚创建的导出草稿，请重试。')
  } finally {
    if (createdDraftID.value === draftID) loadingDraft.value = false
  }
}

async function loadCommandPreview(draft = currentDraft.value) {
  if (!draft) return
  previewingCommand.value = true
  commandPreview.value = null
  commandPreviewFailure.value = ''
  try {
    const preview = await api.previewExportCommand(draft)
    if (currentDraft.value?.id !== draft.id || currentDraft.value.revision !== draft.revision) return
    commandPreview.value = preview
  } catch (error) {
    if (currentDraft.value?.id !== draft.id || currentDraft.value.revision !== draft.revision) return
    commandPreviewFailure.value = exportDraftErrorMessage(error, '无法生成命令预览，请重新读取草稿后重试。')
  } finally {
    if (currentDraft.value?.id === draft.id && currentDraft.value.revision === draft.revision) previewingCommand.value = false
  }
}

async function startPrecheck() {
  if (!currentDraft.value || startingPrecheck.value || precheckRunning.value) return
  startingPrecheck.value = true
  precheckFailure.value = ''
  try {
    if (precheckID.value && !activePrecheck.value) {
      await pollPrecheck(precheckID.value)
      return
    }
    stopPrecheckPolling()
    activePrecheck.value = null
    precheckID.value = ''
    let draft = currentDraft.value
    let draftRefreshed = false
    // Agent 重启或固定运行时变更会推进节点事实版本；先重算指纹，避免用户返回上一步手工重建同一份草稿。
    const currentPreview = await api.previewExportCommand(draft)
    if (currentPreview.configFingerprint !== draft.configFingerprint) {
      draft = await api.updateExportDraft(draft)
      currentDraft.value = draft
      commandPreview.value = null
      await loadCommandPreview(draft)
      draftRefreshed = true
    }
    const id = await api.startPrecheck(draft)
    precheckID.value = id
    draftNotice.value = draftRefreshed
      ? '执行节点运行事实已变化；已按原有字段刷新草稿并排队预检查。Agent 将只校验固定连接、对象、工具、输出路径和空间；不会启动 OBDUMPER。'
      : '预检查已排队，所选 Agent 将只校验固定连接、对象、工具、输出路径和空间；不会启动 OBDUMPER。'
    await pollPrecheck(id)
  } catch (error) {
    precheckFailure.value = exportDraftErrorMessage(error, '无法发起或读取预检查。若请求已被接受，可点击“重新读取预检查”继续查询。')
  } finally {
    startingPrecheck.value = false
  }
}

async function submitTask() {
	if (!currentDraft.value || !activePrecheck.value || !canSubmit.value) return
	submitting.value = true
	submissionFailure.value = ''
	try {
		const taskID = await api.submitExportDraft(currentDraft.value, activePrecheck.value.id)
		await router.push(`/tasks/${encodeURIComponent(taskID)}`)
	} catch (error) {
		submissionFailure.value = exportDraftErrorMessage(error, '任务未能提交；请重新读取预检查后重试。')
	} finally {
		submitting.value = false
	}
}

async function pollPrecheck(id: string) {
  const pollVersion = ++precheckPollVersion
  while (pollVersion === precheckPollVersion) {
    const result = await api.getPrecheck(id)
    if (pollVersion !== precheckPollVersion) return
    activePrecheck.value = result
    if (isTerminalPrecheckStatus(result.status)) {
      draftNotice.value = precheckNotice(result)
      return
    }
    await waitForPrecheckPoll(pollVersion)
  }
}

function waitForPrecheckPoll(pollVersion: number): Promise<void> {
  return new Promise((resolve) => {
    precheckPollResolve = () => {
      precheckPollResolve = undefined
      resolve()
    }
    precheckPollTimer = setTimeout(() => {
      precheckPollTimer = undefined
      if (pollVersion === precheckPollVersion) precheckPollResolve?.()
    }, 1000)
  })
}

function stopPrecheckPolling() {
  precheckPollVersion += 1
  if (precheckPollTimer !== undefined) {
    clearTimeout(precheckPollTimer)
    precheckPollTimer = undefined
  }
  precheckPollResolve?.()
}

function isTerminalPrecheckStatus(status: string) {
	return status === 'SUCCEEDED' || status === 'FAILED' || status === 'EXPIRED' || status === 'INVALIDATED'
}

function precheckNotice(precheck: Precheck) {
	if (precheck.status === 'SUCCEEDED') return '预检查已通过固定六项校验。现在可以提交已冻结任务；提交后 Agent 才会启动 OBDUMPER。'
	if (precheck.status === 'FAILED' && precheck.results.some((result) => result.evidenceCode === 'DATABASE_CONNECTION_UNAVAILABLE')) return '预检查未完成：Agent 未取得可验证的数据库连接结果，因此没有读取所选表的元数据。'
	if (precheck.status === 'FAILED') return '预检查未通过；请根据下方安全结果修正配置后重新创建草稿。'
	if (precheck.status === 'EXPIRED') return '预检查在 Agent 完成前已过期；请重新发起。'
	return '预检查状态已失效；请重新创建草稿后再试。'
}

function precheckDotClass(result?: PrecheckResult) {
  if (!result) return 'neutral'
  return result.status === 'PASSED' ? 'success' : result.status === 'FAILED' ? 'danger' : 'neutral'
}

function precheckStatusLabel(status?: string) {
  return {
    PENDING: '等待 Agent 领取',
    LEASED: 'Agent 正在检查',
    SUCCEEDED: '已通过',
    FAILED: '未通过',
    EXPIRED: '已过期',
    INVALIDATED: '已失效',
  }[status ?? ''] ?? '尚未执行'
}

function environmentLabel(value: string) {
  return { DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' }[value] ?? value
}

function lastTestLabel(source: DataSourceSummary) {
  return source.lastTestedAt ? new Date(source.lastTestedAt).toLocaleString() : '已成功测试'
}
</script>

<template>
  <section class="page-heading">
    <div>
      <h1>新建导出任务</h1>
      <p>OBDUMPER 4.3.5 导出向导。当前仅支持 Windows 本机 MVP 的固定单表 CSV 导出；预检查通过后可显式提交执行。</p>
    </div>
    <span class="draft-status">{{ createdDraftID ? '草稿已创建' : '草稿尚未创建' }}</span>
  </section>
  <WizardFrame kind="export" :active-step="activeStep">
    <template #default>
      <section v-if="activeStep === 1" class="form-section">
        <h2>选择已有数据源</h2>
        <p>向导只选择已启用、当前配置至少一次基础测试成功的数据源；不重复填写地址、用户名或密码。</p>
        <p v-if="sourceLoadFailure" class="feedback feedback-error" role="alert">{{ sourceLoadFailure }} <button type="button" class="link-button" @click="loadSources">重试</button></p>
        <div v-else-if="loadingSources" class="placeholder-control"><span>正在加载已授权数据源…</span></div>
        <EmptyState v-else-if="eligibleSources.length === 0" title="没有可选数据源" :description="sources.length === 0 ? '当前授权范围内没有数据源。请先登记数据源并完成一次成功的基础连接测试。' : '当前已授权数据源均未同时满足已启用和成功测试条件。请在数据源管理中完成受控测试并启用数据源。'" action="前往数据源管理" @action="router.push('/data-sources')" />
        <div v-else class="option-grid">
          <label v-for="source in eligibleSources" :key="source.id" class="option-card" :class="{ selected: selectedDataSourceID === source.id }">
            <input v-model="selectedDataSourceID" type="radio" name="data-source" :value="source.id" />
            <strong>{{ source.displayName }}</strong>
            <span>{{ environmentLabel(source.environment) }} · 私有 ODP · {{ source.host }}:{{ source.port }}</span>
            <span>基础连接测试成功：{{ lastTestLabel(source) }}</span>
          </label>
        </div>
        <p v-if="selectedSource" class="section-hint">已选择 {{ selectedSource.displayName }}。更换数据源会清除当前对象选择，并使已创建草稿不再代表当前页面配置。</p>
        <p v-else-if="eligibleSources.length > 0" class="section-hint">请选择一个数据源后继续；任务级对象、权限、路径和空间检查仍将在预检查阶段执行。</p>
      </section>

      <section v-else-if="activeStep === 2" class="form-section">
        <h2>选择导出对象</h2>
        <p>首条纵向切片只允许指定一个明确表，不提供全部对象、多表、通配符或其他对象类型。</p>
        <div class="form-line">
          <label class="field-label">默认数据库 / Schema <b>*</b><input v-model.trim="database" autocomplete="off" placeholder="从数据源默认值回填，可按任务覆盖" /></label>
        </div>
        <div class="form-line">
          <label class="field-label">表名 <b>*</b><input v-model.trim="table" autocomplete="off" placeholder="填写一个明确表名" /></label>
        </div>
        <p v-if="objectInputMessage" class="feedback feedback-error" role="alert">{{ objectInputMessage }}</p>
        <p v-else class="section-hint">对象存在性和实际权限不在此页推断，仍由后续固定预检查确认。</p>
      </section>

      <section v-else-if="activeStep === 3" class="form-section">
        <h2>选择导出内容</h2>
        <div class="fixed-field"><strong>仅数据</strong><span>首条单表 CSV 切片固定为数据导出，不开放 DDL 或 DDL + 数据组合。</span></div>
        <p class="section-hint">导出内容由固定能力版本约束；页面不会生成可编辑命令片段。</p>
      </section>

      <section v-else-if="activeStep === 4" class="form-section">
        <h2>选择数据格式</h2>
        <div class="fixed-field"><strong>CSV</strong><span>当前只使用服务端已确认的固定 CSV 命令映射；CSV 专属参数仍保持未设置。</span></div>
        <p class="section-hint">CUT、POS、SQL、列式与其他格式不属于当前草稿创建切片，不能在此页选择。</p>
      </section>

      <section v-else-if="activeStep === 5" class="form-section">
        <h2>执行与输出配置</h2>
        <div class="section-block">
          <h3>导出与日志路径</h3>
          <p>导出路径和日志路径均为所选执行节点上的完整绝对路径；Windows 必须使用 /E:/exports 形式，平台不会追加子目录或转换路径格式。</p>
          <label class="field-label wide-label">导出路径 <b>*</b><input v-model.trim="filePath" :placeholder="outputPathPlaceholder" autocomplete="off" /></label>
          <label class="field-label wide-label">日志路径 <span>（可选）</span><input v-model.trim="logPath" :placeholder="outputPathPlaceholder" autocomplete="off" /></label>
          <p class="section-hint">填写日志路径时生成 `--log-path`；留空则保留 OBDUMPER 的默认日志目录行为。</p>
          <label class="checkbox-label"><input v-model="skipCheckDir" type="checkbox" />跳过导出目录是否为空的检查</label>
          <p v-if="skipCheckDir" class="section-hint">将生成 `--skip-check-dir`。仍会检查导出路径和日志路径可写、位于允许根目录内，以及导出路径可用空间。</p>
        </div>
        <div class="section-block">
          <h3>执行节点</h3>
          <p v-if="nodeLoadFailure" class="feedback feedback-error" role="alert">{{ nodeLoadFailure }} <button type="button" class="link-button" @click="loadNodeCandidates">重试</button></p>
          <div v-else-if="loadingNodes" class="placeholder-control short"><span>正在加载已授权执行节点…</span></div>
          <EmptyState v-else-if="nodes.length === 0" title="没有可选执行节点" description="当前授权范围内没有已启用节点。节点在线、工具、路径和空间事实仍需在后续预检查中确认。" />
          <label v-else class="field-label wide-label">执行节点 <b>*</b><select v-model="selectedNodeID"><option value="" disabled>请选择执行节点</option><option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</option></select></label>
          <p v-if="selectedNode" class="section-hint">已选择 {{ selectedNode.displayName }}。此处只表示已授权且已启用，不代表节点在线、输出路径可写或任务已经预检查通过。</p>
        </div>
        <p v-if="draftValidationMessage" class="feedback feedback-error" role="alert">{{ draftValidationMessage }}</p>
        <p v-if="draftFailure" class="feedback feedback-error" role="alert">{{ draftFailure }}</p>
      </section>

      <section v-else class="form-section">
        <h2>参数预检查与完整命令</h2>
        <p v-if="draftNotice" class="feedback" :class="activePrecheck?.status === 'FAILED' ? 'feedback-error' : 'feedback-notice'" :role="activePrecheck?.status === 'FAILED' ? 'alert' : 'status'">{{ draftNotice }}</p>
        <p v-if="loadingDraft" class="section-hint" role="status">正在读取已创建草稿的服务端配置快照…</p>
        <p v-if="draftLoadFailure" class="feedback feedback-error" role="alert">{{ draftLoadFailure }} <button type="button" class="link-button" @click="loadCreatedDraft()">重新读取草稿</button></p>
        <section class="configuration-summary">
          <h3>任务配置摘要</h3>
          <dl>
            <div><dt>数据源</dt><dd>{{ displayedSource?.displayName ?? (currentDraft ? '草稿数据源当前不可用' : '尚未读取草稿') }}</dd></div>
            <div><dt>对象与内容</dt><dd>{{ displayedDraftConfig ? `${displayedDraftConfig.database}.${displayedDraftConfig.table} · 仅数据` : '尚未读取草稿' }}</dd></div>
            <div><dt>导出、日志与节点</dt><dd>{{ displayedDraftConfig && displayedNode ? `${displayedNode.displayName} · 导出：${displayedDraftConfig.filePath}${displayedDraftConfig.logPath ? ` · 日志：${displayedDraftConfig.logPath}` : ''}${displayedDraftConfig.skipCheckDir ? ' · 已跳过目录空性检查' : ''}` : '尚未读取草稿' }}</dd></div>
          </dl>
        </section>
        <section class="precheck-list">
          <h3>预检查结果</h3>
          <p class="section-hint">预检查由已选择的 Agent 执行：确认数据源连接、当前草稿所选表可读取、OB Loader/Dumper 与专用 Java 8 配置、导出目录及已填写日志目录可写、导出目录空性，以及至少 1 GiB 可用空间。勾选跳过选项时，仅目录空性检查会被跳过。它不会启动 OBDUMPER 或创建导出文件。</p>
          <p v-if="precheckFailure" class="feedback feedback-error" role="alert">{{ precheckFailure }}</p>
          <p v-if="activePrecheck" class="precheck-current-status" :class="{ 'is-failed': activePrecheck.status === 'FAILED' }" :role="activePrecheck.status === 'FAILED' ? 'alert' : 'status'">当前状态：<strong>{{ precheckStatusLabel(activePrecheck.status) }}</strong></p>
          <section v-if="activePrecheck?.status === 'FAILED'" class="precheck-failure-summary" role="alert">
            <strong>预检查未通过</strong>
            <p>以下 {{ blockingPrecheckRows.length }} 项检查未通过或未完成。请修正后重新执行预检查。</p>
            <ul>
              <li v-for="row in blockingPrecheckRows" :key="row.check">
                <strong>{{ precheckCheckLabel(row.check) }}</strong>
                <span>{{ precheckResultDetail(row.result) }}</span>
                <code>原因码：{{ row.result?.evidenceCode }}</code>
              </li>
            </ul>
          </section>
          <div v-for="row in precheckRows" :key="row.check" class="precheck-item" :class="{ 'is-failed': row.result?.status === 'FAILED' }">
            <span class="status-dot" :class="precheckDotClass(row.result)" />
            <strong>{{ precheckCheckLabel(row.check) }}</strong>
            <span><b class="precheck-result-status" :class="{ 'is-failed': row.result?.status === 'FAILED' }">{{ precheckResultLabel(row.result, precheckRunning) }}</b><small v-if="precheckResultDetail(row.result)">{{ precheckResultDetail(row.result) }}</small><code v-if="precheckResultBlocksSubmission(row.result)">原因码：{{ row.result?.evidenceCode }}</code></span>
          </div>
        </section>
        <section class="command-empty">
          <div><h3>完整命令（仅隐藏密码）</h3><button type="button" class="button button-secondary" disabled>复制命令</button></div>
          <p v-if="previewingCommand" role="status">控制面正在重算命令预览…</p>
          <p v-else-if="commandPreviewFailure" class="feedback feedback-error" role="alert">{{ commandPreviewFailure }} <button type="button" class="link-button" @click="loadCommandPreview()">重新生成命令</button></p>
          <pre v-else-if="commandPreview"><code>{{ commandPreview.command }}</code></pre>
          <pre v-else><code>命令只会由控制面根据已读取的草稿快照生成；密码始终不会显示或由浏览器自行拼接。</code></pre>
          <p v-if="commandPreview">`-p ******` 仅为密码占位；实际运行从官方安全文件读取密码，不把密码放入进程参数。</p>
          <p v-if="commandPreview">草稿版本 rev-{{ currentDraft?.revision }} · 配置指纹 {{ commandPreview.configFingerprint }}</p>
        </section>
        <p v-if="submissionFailure" class="feedback feedback-error" role="alert">{{ submissionFailure }}</p>
      </section>
    </template>

    <template #summary>
      <h2>配置总览</h2>
      <dl class="summary-definition">
        <div><dt>当前步骤</dt><dd>{{ titles[activeStep - 1] }}</dd></div>
        <div><dt>数据源</dt><dd>{{ displayedSource?.displayName ?? '尚未选择' }}</dd></div>
        <div><dt>导出范围</dt><dd>{{ displayedDraftConfig ? `${displayedDraftConfig.database}.${displayedDraftConfig.table}` : (database && table ? `${database}.${table}` : '尚未配置') }}</dd></div>
        <div><dt>导出内容</dt><dd>仅数据</dd></div>
        <div><dt>数据格式</dt><dd>CSV</dd></div>
        <div><dt>执行节点</dt><dd>{{ displayedNode?.displayName ?? '尚未选择' }}</dd></div>
        <div><dt>预检查</dt><dd>{{ precheckStatusLabel(activePrecheck?.status) }}</dd></div>
      </dl>
      <p class="aside-note">草稿创建和命令预览不启动 Agent、工具或数据库连接。预检查通过后，点击“提交并启动导出”才会由 Agent 领取冻结任务并启动 OBDUMPER。</p>
    </template>

    <template #footer>
      <footer class="wizard-footer">
        <button type="button" class="button button-secondary" :disabled="activeStep === 1 || creatingDraft" @click="previousStep">上一步</button>
        <span class="wizard-baseline-note">{{ footerBaselineNote }}</span>
        <span class="footer-grow" />
        <button v-if="activeStep < 6" type="button" class="button button-primary" :disabled="!canAdvance" @click="nextStep">{{ footerLabel }}</button>
        <div v-else class="footer-actions">
          <button type="button" class="button button-secondary" :disabled="!currentDraft || startingPrecheck || precheckRunning || submitting" @click="startPrecheck">{{ footerLabel }}</button>
          <button type="button" class="button button-primary" :disabled="!canSubmit" @click="submitTask">{{ submitting ? '正在提交任务…' : '提交并启动导出' }}</button>
        </div>
      </footer>
    </template>
  </WizardFrame>
</template>
