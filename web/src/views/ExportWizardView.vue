<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { browserApi, dataSourceErrorMessage, exportDraftErrorMessage, type CommandPreview, type CompressionAlgo, type CsvQuoteMode, type DataSourceSummary, type ExecutionNodeCandidate, type ExportContentKind, type ExportDataFormatKind, type ExportDraft, type ExportObjectType, type ExportOutputKind, type ExportScopeKind, type Precheck, type PrecheckResult } from '@/api/browser'
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
const scopeKind = ref<ExportScopeKind>('SPECIFIED')
const objectType = ref<ExportObjectType>('TABLE')
const objectNames = ref<string[]>([''])
const excludeTablesText = ref('')
const contentKind = ref<ExportContentKind>('DATA_ONLY')
// EX-I4：数据格式单选；默认 CSV 保持既有行为，切换时清空不适用格式的选项。
const formatKind = ref<ExportDataFormatKind>('CSV')
const trailDelimiter = ref(false)
const removeNewline = ref(false)
// EX-I4 POS 定版后解锁：CUT 列分隔字符串（--column-splitter）。
const columnSplitter = ref('')
const filePath = ref('')
const logPath = ref('')
const skipCheckDir = ref(false)
// EX-I6 对象存储（2026-08-07）：输出类型单选与受控对象存储字段（凭据走执行槽位，不在表单收集）。
const outputKind = ref<ExportOutputKind>('LOCAL')
const storageBucket = ref('')
const storagePath = ref('')
const storageEndpoint = ref('')
const storageRegion = ref('')
const tmpPath = ref('')
// EX-I4 POS 定版（2026-08-07 实测）：控制文件目录（--ctl-path），仅 POS 格式使用。
const controlFilePath = ref('')
// EX-I3 选项状态。
const skipHeader = ref(false)
const columnSeparator = ref('')
const columnQuote = ref('')
const columnQuoteMode = ref<CsvQuoteMode | ''>('')
const escapeCharacter = ref('')
const lineSeparator = ref('')
const nullString = ref('')
const fileEncoding = ref('')
const withTrim = ref(false)
const noNestedDir = ref(false)
const maxFileSize = ref('')
const retainEmptyFiles = ref(false)
const compress = ref(false)
const compressionAlgo = ref<CompressionAlgo | ''>('')
const querySql = ref('')
const includeColumnNames = ref('')
const excludeColumnNames = ref('')
const excludeVirtualColumns = ref(false)
const flashbackScn = ref('')
const flashbackTimestamp = ref('')
const thread = ref('')
const pageSize = ref('')
const parallelMacro = ref('')
const fetchSize = ref('')
const jvmMemory = ref('')
const dataOptionsActive = computed(() => contentKind.value !== 'DDL_ONLY')
// EX-I4：序列化面板按数据格式适用；文件布局、筛选与性能选项官方不限定格式，CSV/CUT/SQL 均有效。
const hasFilterOptions = computed(() => Boolean(querySql.value.trim() || includeColumnNames.value.trim() || excludeColumnNames.value.trim() || excludeVirtualColumns.value || flashbackScn.value.trim() || flashbackTimestamp.value.trim()))
const hasPerformanceOptions = computed(() => Boolean(thread.value.trim() || pageSize.value.trim() || parallelMacro.value.trim() || fetchSize.value.trim() || jvmMemory.value.trim()))
const hasCsvOptions = computed(() => formatKind.value === 'CSV' && Boolean(skipHeader.value || withTrim.value || columnSeparator.value || columnQuote.value || columnQuoteMode.value || escapeCharacter.value || lineSeparator.value || nullString.value || fileEncoding.value))
const hasCutOptions = computed(() => formatKind.value === 'CUT' && Boolean(trailDelimiter.value || removeNewline.value || withTrim.value || escapeCharacter.value || lineSeparator.value || nullString.value || fileEncoding.value))
const hasSqlOptions = computed(() => formatKind.value === 'SQL' && Boolean(lineSeparator.value || fileEncoding.value))
const hasFileLayoutOptions = computed(() => Boolean(noNestedDir.value || maxFileSize.value || retainEmptyFiles.value))
const hasCompressionOptions = computed(() => Boolean(compress.value || compressionAlgo.value))
const optionPanelsActive = computed(() => Boolean(hasFilterOptions.value || hasPerformanceOptions.value || hasCsvOptions.value || hasCutOptions.value || hasSqlOptions.value || hasFileLayoutOptions.value || hasCompressionOptions.value))
const optionPanelsMessage = computed(() => {
  if (!dataOptionsActive.value) return ''
  if (!optionPanelsActive.value) return ''
  if (querySql.value.trim() && (flashbackScn.value.trim() || flashbackTimestamp.value.trim())) return '自定义查询与闪回参数互斥，只能选择其一。'
  if (includeColumnNames.value.trim() && excludeColumnNames.value.trim()) return '包含列与排除列互斥，只能选择其一。'
  if (compress.value && !compressionAlgo.value) return '启用压缩后请选择压缩算法。'
  return ''
})
const activeStep = computed(() => {
  const value = Number(route.query.step ?? '1')
  return Number.isInteger(value) && value >= 1 && value <= 6 ? value : 1
})

const titles = ['选择数据源', '选择导出对象', '选择导出内容', '选择数据格式', '执行与输出', '预检查与命令']
const eligibleSources = computed(() => sources.value.filter(isExportEligibleDataSource))
const selectedSource = computed(() => eligibleSources.value.find((source) => source.id === selectedDataSourceID.value))
const selectedNode = computed(() => nodes.value.find((node) => node.id === selectedNodeID.value))
const displayedDraftConfig = computed(() => currentDraft.value?.config)
const draftScopeSummary = computed(() => {
  const config = displayedDraftConfig.value
  if (!config) return ''
  const scope = config.objectScope
  if (scope.scopeKind === 'ALL') return `${scope.database} · 全部对象`
  const names = (scope.expressions ?? []).map((expression) => expression.name)
  const typeLabel = (scope.objectTypes ?? [])[0] === 'VIEW' ? '视图' : '表'
  return `${scope.database} · ${names.length} 个${typeLabel}：${names.join('、')}`
})
const draftContentLabel = computed(() => {
  const config = displayedDraftConfig.value
  if (!config) return ''
  const content = config.contentSelection.contentKind
  if (content === 'DDL_ONLY') return '仅 DDL'
  if (content === 'DDL_AND_DATA') return 'DDL + 数据 · CSV'
  // EX-I4：仅数据按实际数据格式投影。
  return config.dataFormat?.formatKind === 'CUT' ? '仅数据 · CUT' : config.dataFormat?.formatKind === 'SQL' ? '仅数据 · SQL' : '仅数据 · CSV'
})
const displayedSource = computed(() => {
  const sourceID = currentDraft.value?.dataSourceId ?? selectedDataSourceID.value
  return sources.value.find((source) => source.id === sourceID)
})
const displayedNode = computed(() => {
  const nodeID = currentDraft.value?.nodeId ?? selectedNodeID.value
  return nodes.value.find((node) => node.id === nodeID)
})
const precheckRunning = computed(() => activePrecheck.value?.status === 'PENDING' || activePrecheck.value?.status === 'LEASED' || (Boolean(precheckID.value) && !activePrecheck.value && !precheckFailure.value))
// EX-I6 门禁（2026-08-10）：对象存储输出的存储专用预检查（凭据、网络、权限、空间）尚未完成，
// 固定预检查与提交保持功能门禁阻断，不把存储 URI 伪装成本地路径进入固定预检查。
const storagePrecheckBlocked = computed(() => outputKind.value !== 'LOCAL')
const precheckRows = computed(() => fixedPrecheckChecks.map((check) => ({
  check,
  result: activePrecheck.value?.results.find((result) => result.check === check),
})))
const blockingPrecheckRows = computed(() => precheckRows.value.filter((row) => precheckResultBlocksSubmission(row.result)))
const canSubmit = computed(() => activePrecheck.value?.status === 'SUCCEEDED' && activePrecheck.value.integrityStatus === 'COMPLETE' && Boolean(currentDraft.value) && !submitting.value && !storagePrecheckBlocked.value)
const footerBaselineNote = computed(() => {
	if (activeStep.value === 6) return storagePrecheckBlocked.value ? '对象存储输出的存储专用预检查（凭据、网络、权限、空间）尚未完成，当前不能发起预检查或提交。' : (canSubmit.value ? '预检查已通过。提交后，所选 Agent 将领取已冻结的导出任务并启动 OBDUMPER。' : (currentDraft.value ? '草稿已保存；请先完成当前版本的固定预检查，提交后才会启动 OBDUMPER。' : '尚未读取草稿；请返回上一步完成固定字段并创建草稿。'))
  return '仅在固定字段完整时才创建草稿；服务端会再次校验数据源和节点授权。'
})
const objectInputMessage = computed(() => {
  if (!database.value.trim()) return '请填写默认数据库或 Schema。'
  if (scopeKind.value === 'ALL') return ''
  const names = objectNames.value.map((name) => name.trim()).filter((name) => name.length > 0)
  if (names.length === 0) return objectType.value === 'VIEW' ? '请至少填写一个视图名称。' : '请至少填写一个表名。'
  if (names.length > 100) return '对象数量不能超过 100 个。'
  if (names.some((name) => name.includes('*') || name.includes(',') || name.length > 256)) return '对象名称不能使用通配符或逗号，且不超过 256 个字符；多个对象请分行填写。'
  return ''
})
const contentInputMessage = computed(() => {
  if (objectType.value === 'VIEW' && scopeKind.value === 'SPECIFIED' && contentKind.value !== 'DDL_ONLY') return '视图只能导出对象定义（仅 DDL），不能导出数据。'
  return ''
})
const excludeTables = computed(() => excludeTablesText.value.split(',').map((name) => name.trim()).filter((name) => name.length > 0))
const draftValidation = computed(() => validateExportDraftInput({
  dataSourceId: selectedDataSourceID.value,
  nodeId: selectedNodeID.value,
  platform: selectedNode.value?.platform ?? '',
  database: database.value,
  scopeKind: scopeKind.value,
  objectType: objectType.value,
  objectNames: objectNames.value,
  excludeTables: excludeTables.value,
  contentKind: contentKind.value,
  formatKind: formatKind.value,
  trailDelimiter: trailDelimiter.value,
  removeNewline: removeNewline.value,
  columnSplitter: columnSplitter.value,
  filePath: filePath.value,
  logPath: logPath.value,
  skipCheckDir: skipCheckDir.value,
  outputKind: outputKind.value,
  storageBucket: storageBucket.value,
  storagePath: storagePath.value,
  storageEndpoint: storageEndpoint.value,
  storageRegion: storageRegion.value,
  tmpPath: tmpPath.value,
  controlFilePath: controlFilePath.value,
  skipHeader: skipHeader.value,
  columnSeparator: columnSeparator.value,
  columnQuote: columnQuote.value,
  columnQuoteMode: columnQuoteMode.value,
  escapeCharacter: escapeCharacter.value,
  lineSeparator: lineSeparator.value,
  nullString: nullString.value,
  fileEncoding: fileEncoding.value,
  withTrim: withTrim.value,
  noNestedDir: noNestedDir.value,
  maxFileSize: maxFileSize.value,
  retainEmptyFiles: retainEmptyFiles.value,
  compress: compress.value,
  compressionAlgo: compressionAlgo.value,
  querySql: querySql.value,
  includeColumnNames: includeColumnNames.value,
  excludeColumnNames: excludeColumnNames.value,
  excludeVirtualColumns: excludeVirtualColumns.value,
  flashbackScn: flashbackScn.value,
  flashbackTimestamp: flashbackTimestamp.value,
  thread: thread.value,
  pageSize: pageSize.value,
  parallelMacro: parallelMacro.value,
  fetchSize: fetchSize.value,
  jvmMemory: jvmMemory.value,
}))
const draftInput = computed(() => draftValidation.value.valid ? draftValidation.value.input : undefined)
const draftValidationMessage = computed(() => draftValidation.value.valid ? '' : draftValidation.value.message)
const canAdvance = computed(() => {
  if (activeStep.value === 1) return Boolean(selectedSource.value)
  if (activeStep.value === 2) return Boolean(selectedSource.value) && !objectInputMessage.value
  if (activeStep.value === 3) return Boolean(selectedSource.value) && !contentInputMessage.value
  if (activeStep.value === 5) return Boolean(draftInput.value) && !creatingDraft.value
  return Boolean(selectedSource.value) && activeStep.value < 5
})
const footerLabel = computed(() => {
  if (activeStep.value === 5) return creatingDraft.value ? '正在创建草稿…' : '创建导出草稿'
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
  scopeKind.value = 'SPECIFIED'
  objectType.value = 'TABLE'
  objectNames.value = ['']
  excludeTablesText.value = ''
  contentKind.value = 'DATA_ONLY'
  clearDraftState()
})

watch([database, scopeKind, objectType, objectNames, excludeTablesText, contentKind, formatKind, trailDelimiter, removeNewline, columnSplitter, selectedNodeID, filePath, logPath, skipCheckDir, outputKind, storageBucket, storagePath, storageEndpoint, storageRegion, tmpPath, controlFilePath, skipHeader, columnSeparator, columnQuote, columnQuoteMode, escapeCharacter, lineSeparator, nullString, fileEncoding, withTrim, noNestedDir, maxFileSize, retainEmptyFiles, compress, compressionAlgo, querySql, includeColumnNames, excludeColumnNames, excludeVirtualColumns, flashbackScn, flashbackTimestamp, thread, pageSize, parallelMacro, fetchSize, jvmMemory], () => {
  // 任一配置变化都会作废已创建草稿并清除服务端错误提示。
  clearDraftState()
}, { deep: true })

watch(scopeKind, (kind) => {
  if (kind !== 'ALL') return
  excludeTablesText.value = ''
})

watch(objectType, (type, previousType) => {
  if (type === 'VIEW' && contentKind.value !== 'DDL_ONLY') contentKind.value = 'DDL_ONLY'
  // 视图被强制为仅 DDL 后切回表时，对称恢复默认的仅数据内容。
  if (previousType === 'VIEW' && type === 'TABLE' && scopeKind.value === 'SPECIFIED' && contentKind.value === 'DDL_ONLY') contentKind.value = 'DATA_ONLY'
})

watch(compress, (enabled) => {
  // 取消压缩时清除算法值，避免禁用态下拉保留旧值造成校验死锁。
  if (!enabled) compressionAlgo.value = ''
})

watch(contentKind, (kind) => {
  if (kind === 'DDL_ONLY') {
    // 仅 DDL 时清除数据专属选项，避免残留值进入下一个草稿。
    skipHeader.value = false
    columnSeparator.value = ''
    columnQuote.value = ''
    columnQuoteMode.value = ''
    escapeCharacter.value = ''
    lineSeparator.value = ''
    nullString.value = ''
    fileEncoding.value = ''
    withTrim.value = false
    trailDelimiter.value = false
    removeNewline.value = false
    columnSplitter.value = ''
    controlFilePath.value = ''
    noNestedDir.value = false
    maxFileSize.value = ''
    retainEmptyFiles.value = false
    compress.value = false
    compressionAlgo.value = ''
    querySql.value = ''
    includeColumnNames.value = ''
    excludeColumnNames.value = ''
    excludeVirtualColumns.value = false
    flashbackScn.value = ''
    flashbackTimestamp.value = ''
    thread.value = ''
    pageSize.value = ''
    parallelMacro.value = ''
    fetchSize.value = ''
    jvmMemory.value = ''
  }
  // DDL + 数据只支持 CSV 数据格式：切换到该内容类型时重置非 CSV 格式，
  // 其余格式专属残留由 formatKind 监听器在离开对应格式时清理。
  if (kind === 'DDL_AND_DATA' && formatKind.value !== 'CSV') formatKind.value = 'CSV'
})

// 切换数据格式时清空不适用格式的选项，避免残留值进入下一个草稿。
watch(formatKind, (kind, previousKind) => {
  if (kind === previousKind) return
  // 离开 CSV：只清 CSV 专属序列化选项；文件布局、筛选与性能选项官方不限定格式，切换后保留。
  if (kind !== 'CSV') {
    skipHeader.value = false
    columnSeparator.value = ''
    columnQuote.value = ''
    columnQuoteMode.value = ''
  }
  // 离开 CUT：清空 CUT 专属选项（含列分隔字符串）。
  if (kind !== 'CUT') {
    trailDelimiter.value = false
    removeNewline.value = false
    columnSplitter.value = ''
  }
  // 离开 POS：清空控制文件目录（--ctl-path）。
  if (kind !== 'POS') controlFilePath.value = ''
  // SQL 只支持行分隔符与文件编码。
  if (kind === 'SQL') {
    escapeCharacter.value = ''
    nullString.value = ''
    withTrim.value = false
  }
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
    draftNotice.value = '导出草稿已创建，正在读取服务端配置快照。'
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
    draftNotice.value = '导出草稿已从服务端读取。完整命令仅隐藏密码，其余参数由控制面生成并经本地校验后展示。'
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
  // EX-I6 门禁：对象存储输出在存储专用预检查完成前不能进入只接受本地绝对路径的固定预检查。
  if (storagePrecheckBlocked.value) {
    precheckFailure.value = '对象存储输出的存储专用预检查（凭据、网络、权限、空间）尚未完成，当前不能发起固定预检查。'
    return
  }
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

function addObjectNameRow() {
  if (objectNames.value.length >= 100) return
  objectNames.value = [...objectNames.value, '']
}

function removeObjectNameRow(index: number) {
  if (objectNames.value.length <= 1) {
    objectNames.value = ['']
    return
  }
  objectNames.value = objectNames.value.filter((_, position) => position !== index)
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
      <p>OBDUMPER 4.3.5 导出向导。支持全部/指定对象、仅数据、仅 DDL 与 DDL + 数据的已验证组合；预检查通过后可显式提交执行。</p>
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
        <p>支持全部对象或指定对象；指定对象时当前只开放表与视图两种类型，不支持通配符、多库前缀或其他对象类型。</p>
        <details class="tree-node" open>
          <summary>基础选项 · 功能选项 · 数据库对象类型</summary>
          <div class="tree-body">
            <div class="tree-row">
              <span class="tree-label">默认数据库 / Schema <b>*</b></span>
              <input v-model.trim="database" class="tree-input" autocomplete="off" placeholder="从数据源默认值回填，可按任务覆盖" />
            </div>
            <div class="tree-row">
              <span class="tree-label">导出范围 <b>*</b></span>
              <label class="checkbox-label"><input v-model="scopeKind" type="radio" name="export-scope" value="ALL" />全部对象</label>
              <label class="checkbox-label"><input v-model="scopeKind" type="radio" name="export-scope" value="SPECIFIED" />指定对象</label>
            </div>
            <template v-if="scopeKind === 'SPECIFIED'">
              <div class="tree-row">
                <span class="tree-label">对象类型 <b>*</b></span>
                <label class="checkbox-label"><input v-model="objectType" type="radio" name="object-type" value="TABLE" />表</label>
                <label class="checkbox-label"><input v-model="objectType" type="radio" name="object-type" value="VIEW" />视图（仅 DDL）</label>
              </div>
              <div v-for="(_name, index) in objectNames" :key="index" class="tree-row">
                <span class="tree-label">{{ objectType === 'VIEW' ? '视图名称' : '表名' }} <b>*</b></span>
                <input v-model.trim="objectNames[index]" class="tree-input" autocomplete="off" :placeholder="objectType === 'VIEW' ? '填写一个明确视图名' : '填写一个明确表名'" />
                <button type="button" class="link-button" :disabled="objectNames.length <= 1" @click="removeObjectNameRow(index)">移除</button>
              </div>
              <div class="tree-row">
                <button type="button" class="link-button" :disabled="objectNames.length >= 100" @click="addObjectNameRow">添加对象</button>
              </div>
              <div v-if="objectType === 'TABLE'" class="tree-row">
                <span class="tree-label">排除表 <span class="muted">（专家配置，逗号分隔）</span></span>
                <input v-model.trim="excludeTablesText" class="tree-input" autocomplete="off" placeholder="例如 tmp_a,tmp_b" />
              </div>
            </template>
            <p v-else class="section-hint">全部对象将导出当前数据库内的全部对象定义与表数据；排除表不适用于全部范围。</p>
          </div>
        </details>
        <details class="tree-node" open>
          <summary>高级选项 · 功能选项 · 黑白名单筛选 <span class="tree-note">列筛选，仅在导出数据时生效</span></summary>
          <div class="tree-body">
            <p class="section-hint">列筛选在步骤 3 选择“仅 DDL”时不参与生成；与对象范围参数相互独立。</p>
            <div class="tree-row">
              <span class="tree-label">包含列 <span class="muted">（逗号分隔）</span></span>
              <input v-model.trim="includeColumnNames" class="tree-input" :disabled="Boolean(excludeColumnNames)" placeholder="例如 col_a,col_b" />
            </div>
            <div class="tree-row">
              <span class="tree-label">排除列 <span class="muted">（逗号分隔）</span></span>
              <input v-model.trim="excludeColumnNames" class="tree-input" :disabled="Boolean(includeColumnNames)" placeholder="例如 col_c" />
            </div>
            <div class="tree-row">
              <span class="tree-label">排除生成列</span>
              <label class="checkbox-label"><input v-model="excludeVirtualColumns" type="checkbox" />--exclude-virtual-columns</label>
            </div>
          </div>
        </details>
        <p v-if="objectInputMessage" class="feedback feedback-error" role="alert">{{ objectInputMessage }}</p>
        <p v-else class="section-hint">对象存在性和实际权限不在此页推断，仍由后续固定预检查确认。</p>
      </section>

      <section v-else-if="activeStep === 3" class="form-section">
        <h2>选择导出内容</h2>
        <div class="tree-row tree-row-root">
          <span class="tree-label">导出内容 <b>*</b></span>
          <label class="checkbox-label"><input v-model="contentKind" type="radio" name="content-kind" value="DATA_ONLY" :disabled="objectType === 'VIEW' && scopeKind === 'SPECIFIED'" />仅数据</label>
          <label class="checkbox-label"><input v-model="contentKind" type="radio" name="content-kind" value="DDL_ONLY" />仅 DDL</label>
          <label class="checkbox-label"><input v-model="contentKind" type="radio" name="content-kind" value="DDL_AND_DATA" :disabled="objectType === 'VIEW' && scopeKind === 'SPECIFIED'" />DDL + 数据</label>
        </div>
        <template v-if="contentKind !== 'DATA_ONLY'">
          <details class="tree-node" open>
            <summary>基础选项 · 功能选项 · 文件格式 <span class="tree-note">DDL 伴生参数</span></summary>
            <div class="tree-body">
              <div class="tree-row pending">
                <span class="tree-label">前置 DROP</span>
                <input type="checkbox" disabled />
                <span class="tree-note">--drop-object，待接入（包含 DDL 时生效）</span>
              </div>
              <div class="tree-row pending">
                <span class="tree-label">紧凑 Schema</span>
                <input type="checkbox" disabled />
                <span class="tree-note">--compact-schema，待验证</span>
              </div>
            </div>
          </details>
          <details class="tree-node">
            <summary>基础选项 · 功能选项 · 数据库对象类型 <span class="tree-note">序列策略</span></summary>
            <div class="tree-body">
              <div class="tree-row pending">
                <span class="tree-label">序列策略</span>
                <select class="tree-input tree-select" disabled><option>preserve（默认）</option><option>restart</option></select>
                <span class="tree-note">--sequence-policy，待验证</span>
              </div>
            </div>
          </details>
          <details class="tree-node">
            <summary>高级选项 · 其他选项 <span class="tree-note">DDL 专属</span></summary>
            <div class="tree-body">
              <div class="tree-row pending">
                <span class="tree-label">保留 Schema</span>
                <input type="checkbox" disabled />
                <span class="tree-note">--retain-schema，待接入（包含 DDL 时生效）</span>
              </div>
            </div>
          </details>
        </template>
        <p v-if="contentInputMessage" class="feedback feedback-error" role="alert">{{ contentInputMessage }}</p>
        <p v-else class="section-hint">仅 DDL 不生成数据格式参数；DDL + 数据同时生成对象定义与数据文件。DDL 伴生参数按官方分类归属展示。</p>
      </section>

      <section v-else-if="activeStep === 4" class="form-section">
        <h2>选择数据格式</h2>
        <template v-if="contentKind === 'DDL_ONLY'">
          <div class="fixed-field"><strong>无数据格式</strong><span>仅 DDL 导出不生成数据文件，因此不选择数据格式。</span></div>
        </template>
        <template v-else>
          <details class="tree-node" open>
            <summary>基础选项 · 功能选项 · 文件格式</summary>
            <div class="tree-body">
              <div class="tree-row">
                <span class="tree-label">数据格式 <b>*</b></span>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="CSV" />CSV</label>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="CUT" />CUT</label>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="POS" />POS</label>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="SQL" />SQL</label>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="PARQUET" />Parquet</label>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="ORC" />ORC</label>
                <label class="checkbox-label"><input v-model="formatKind" type="radio" name="data-format" value="AVRO" />Avro</label>
              </div>
              <p class="section-hint">CSV 生成逗号分隔文本，CUT 生成紧凑定界文本，POS 生成定长文本（需要控制文件定义列长度），SQL 生成 INSERT 语句，Parquet/ORC/Avro 生成列式结构化文件；均按官方 4.3.5 参数映射生成。POS 映射已于受控实测定版：独立 --pos + --ctl-path + 控制文件。</p>
              <!-- 格式专属配置：按选中格式显示对应参数行 -->
              <template v-if="formatKind === 'CSV'">
                <div class="tree-row">
                  <span class="tree-label">省略字段头</span>
                  <label class="checkbox-label"><input v-model="skipHeader" type="checkbox" />--skip-header</label>
                </div>
                <div class="tree-row">
                  <span class="tree-label">列分隔符</span>
                  <input v-model.trim="columnSeparator" class="tree-input" placeholder="默认英文逗号；支持多字符" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">列包围符</span>
                  <input v-model.trim="columnQuote" class="tree-input" placeholder="默认英文单引号" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">包围模式</span>
                  <select v-model="columnQuoteMode" class="tree-input tree-select"><option value="">继承官方默认</option><option value="all">all</option><option value="all_not_null">all_not_null</option><option value="minimal">minimal</option><option value="non_numeric">non_numeric</option><option value="none">none</option></select>
                </div>
                <div class="tree-row">
                  <span class="tree-label">转义字符</span>
                  <input v-model.trim="escapeCharacter" class="tree-input" placeholder="仅支持单字符" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">行分隔符</span>
                  <input v-model.trim="lineSeparator" class="tree-input" placeholder="按官方平台换行形式" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">NULL 替换</span>
                  <input v-model.trim="nullString" class="tree-input" placeholder="默认 \N" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">文件编码</span>
                  <input v-model.trim="fileEncoding" class="tree-input" placeholder="默认 UTF-8" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">去除左右空格</span>
                  <label class="checkbox-label"><input v-model="withTrim" type="checkbox" />--with-trim</label>
                </div>
                <p class="section-hint">包围模式含义：all 全部包围、all_not_null 非空包围、minimal 最小包围、non_numeric 非数字包围、none 不包围。</p>
              </template>
              <template v-else-if="formatKind === 'CUT'">
                <div class="tree-row">
                  <span class="tree-label">行尾追加分隔符</span>
                  <label class="checkbox-label"><input v-model="trailDelimiter" type="checkbox" />--trail-delimiter</label>
                </div>
                <div class="tree-row">
                  <span class="tree-label">列分隔字符串</span>
                  <input v-model.trim="columnSplitter" class="tree-input" placeholder="例如 |" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">转义字符</span>
                  <input v-model.trim="escapeCharacter" class="tree-input" placeholder="默认反斜杠；仅支持单字符" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">行分隔符</span>
                  <input v-model.trim="lineSeparator" class="tree-input" placeholder="按官方平台换行形式" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">NULL 替换</span>
                  <input v-model.trim="nullString" class="tree-input" placeholder="默认 \N" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">文件编码</span>
                  <input v-model.trim="fileEncoding" class="tree-input" placeholder="默认 UTF-8" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">去除左右空格</span>
                  <label class="checkbox-label"><input v-model="withTrim" type="checkbox" />--with-trim</label>
                </div>
                <p class="section-hint">CUT 列分隔字符串（--column-splitter）已随 POS 定版解锁；日期时间值格式仍未完成受控实测，暂不开放。</p>
              </template>
              <template v-else-if="formatKind === 'POS'">
                <div class="tree-row">
                  <span class="tree-label">控制文件目录 <b>*</b></span>
                  <input v-model.trim="controlFilePath" class="tree-input" placeholder="例如 /E:/workespace/ob-data-orch/tmp/controls" autocomplete="off" />
                </div>
                <p class="section-hint">POS 使用独立 --pos 并必须搭配 --ctl-path 控制文件目录：目录内为每张表提供 &lt;表名&gt;.ctrl（列名 + position(字节长度)）。目前先填写执行节点上的控制文件目录；自动生成来源将在后续版本开放。</p>
              </template>
              <template v-else-if="formatKind === 'SQL'">
                <div class="tree-row">
                  <span class="tree-label">行分隔符</span>
                  <input v-model.trim="lineSeparator" class="tree-input" placeholder="按官方平台换行形式" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">文件编码</span>
                  <input v-model.trim="fileEncoding" class="tree-input" placeholder="默认 UTF-8" />
                </div>
                <p class="section-hint">SQL 格式只支持行分隔符与文件编码；CSV/CUT 专属选项不适用。</p>
              </template>
              <template v-else-if="['PARQUET', 'ORC', 'AVRO'].includes(formatKind)">
                <p v-if="formatKind === 'ORC'" class="section-hint">ORC 格式内存占用较高，请确保执行节点资源充足。</p>
                <div class="tree-row">
                  <span class="tree-label">文件编码</span>
                  <input v-model.trim="fileEncoding" class="tree-input" placeholder="默认 UTF-8" />
                </div>
                <p class="section-hint">结构化格式按官方格式表只支持文件编码与闪回等读取层参数；压缩与序列化选项不适用。</p>
              </template>
              <!-- 闪回 SCN：官方归类为文件格式伴生参数 -->
              <div class="tree-row">
                <span class="tree-label">闪回 SCN <span class="muted">（格式伴生）</span></span>
                <input v-model.trim="flashbackScn" class="tree-input" :disabled="Boolean(querySql)" placeholder="正整数" />
              </div>
            </div>
          </details>
          <details class="tree-node" open>
            <summary>基础选项 · 功能选项 · 压缩导出 <span class="tree-note">仅可读格式</span></summary>
            <div class="tree-body">
              <div class="tree-row">
                <span class="tree-label">启用压缩</span>
                <label class="checkbox-label"><input v-model="compress" type="checkbox" />--compress</label>
              </div>
              <div class="tree-row">
                <span class="tree-label">压缩算法</span>
                <select v-model="compressionAlgo" class="tree-input tree-select" :disabled="!compress"><option value="">继承官方默认（zstd）</option><option value="zstd">zstd</option><option value="zlib">zlib</option><option value="gzip">gzip</option><option value="snappy">snappy</option></select>
              </div>
              <p class="section-hint">压缩仅适用于 CSV/CUT/POS/SQL 可读格式；Parquet/ORC/Avro 结构化格式不适用。</p>
            </div>
          </details>
        </template>
        <p class="section-hint">所有已启用格式（CSV/CUT/POS/SQL/Parquet/ORC/Avro）均可在此页选择；未完成映射定版的格式不能选择。</p>
      </section>

      <section v-else-if="activeStep === 5" class="form-section">
        <h2>执行与输出配置</h2>
        <p class="section-hint">本页按 OBDUMPER 官方选项分类组织：基础选项（存储路径）与高级选项（错误处理、时间戳格式、黑白名单筛选、性能选项）；执行节点为产品扩展。压缩导出与闪回 SCN 已在步骤 4 配置。</p>
        <details class="tree-node" open>
          <summary>基础选项 · 功能选项 · 存储路径</summary>
          <div class="tree-body">
            <div class="tree-row">
              <span class="tree-label">输出类型 <span class="muted">（产品扩展）</span></span>
              <label class="checkbox-label"><input v-model="outputKind" type="radio" name="output-kind" value="LOCAL" />本地路径</label>
              <label class="checkbox-label"><input v-model="outputKind" type="radio" name="output-kind" value="OSS" />OSS</label>
              <label class="checkbox-label"><input v-model="outputKind" type="radio" name="output-kind" value="S3" />S3</label>
              <label class="checkbox-label"><input v-model="outputKind" type="radio" name="output-kind" value="COS" />COS</label>
              <label class="checkbox-label"><input v-model="outputKind" type="radio" name="output-kind" value="OBS" />OBS</label>
            </div>
            <p class="section-hint">对象存储输出使用受控 URI（仅 Bucket/路径/Endpoint/Region）；存储凭据走任务级安全槽位，不会进入命令、日志或快照。</p>
            <details v-if="outputKind === 'LOCAL'" class="tree-node" open>
              <summary>本地路径</summary>
              <div class="tree-body">
                <p class="section-hint">导出路径和日志路径均为所选执行节点上的完整绝对路径；Windows 必须使用 /E:/exports 形式，平台不会追加子目录或转换路径格式。</p>
                <div class="tree-row">
                  <span class="tree-label">导出路径 <b>*</b></span>
                  <input v-model.trim="filePath" class="tree-input" :placeholder="outputPathPlaceholder" autocomplete="off" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">日志路径 <span class="muted">（可选）</span></span>
                  <input v-model.trim="logPath" class="tree-input" :placeholder="outputPathPlaceholder" autocomplete="off" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">扁平目录</span>
                  <label class="checkbox-label"><input v-model="noNestedDir" type="checkbox" />--no-nested-dir</label>
                </div>
                <p class="section-hint">填写日志路径时生成 `--log-path`；留空则保留 OBDUMPER 的默认日志目录行为。</p>
              </div>
            </details>
            <details v-else class="tree-node" open>
              <summary>对象存储</summary>
              <div class="tree-body">
                <p class="section-hint">对象路径以 / 开头；Endpoint 与 Region 至少填写一项。存储凭据（AccessKey/SecretKey）由任务级安全槽位提供，此处不收集。</p>
                <div class="tree-row">
                  <span class="tree-label">Bucket <b>*</b></span>
                  <input v-model.trim="storageBucket" class="tree-input" placeholder="例如 my-bucket" autocomplete="off" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">对象路径 <b>*</b></span>
                  <input v-model.trim="storagePath" class="tree-input" placeholder="例如 /exports/daily" autocomplete="off" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">Endpoint</span>
                  <input v-model.trim="storageEndpoint" class="tree-input" placeholder="例如 oss-cn-hangzhou-internal.aliyuncs.com" autocomplete="off" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">Region</span>
                  <input v-model.trim="storageRegion" class="tree-input" placeholder="例如 cn-hangzhou" autocomplete="off" />
                </div>
                <div class="tree-row">
                  <span class="tree-label">本地临时分块目录 <span class="muted">（高级，可选）</span></span>
                  <input v-model.trim="tmpPath" class="tree-input" :placeholder="outputPathPlaceholder" autocomplete="off" />
                </div>
                <p class="section-hint">对象存储 Multipart 上传使用本地临时分块目录（--tmp-path）；不填写时继承 OBDUMPER 默认行为。</p>
              </div>
            </details>
          </div>
        </details>
        <details class="tree-node" open>
          <summary>执行节点 <span class="tree-note">产品扩展</span></summary>
          <div class="tree-body">
            <p v-if="nodeLoadFailure" class="feedback feedback-error" role="alert">{{ nodeLoadFailure }} <button type="button" class="link-button" @click="loadNodeCandidates">重试</button></p>
            <div v-else-if="loadingNodes" class="placeholder-control short"><span>正在加载已授权执行节点…</span></div>
            <EmptyState v-else-if="nodes.length === 0" title="没有可选执行节点" description="当前授权范围内没有已启用节点。节点在线、工具、路径和空间事实仍需在后续预检查中确认。" />
            <div v-else class="tree-row">
              <span class="tree-label">执行节点 <b>*</b></span>
              <select v-model="selectedNodeID" class="tree-input tree-select"><option value="" disabled>请选择执行节点</option><option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</option></select>
            </div>
            <p v-if="selectedNode" class="section-hint">已选择 {{ selectedNode.displayName }}。此处只表示已授权且已启用，不代表节点在线、输出路径可写或任务已经预检查通过。</p>
          </div>
        </details>
        <details v-if="dataOptionsActive" class="tree-node">
          <summary>高级选项 · 功能选项 · 错误处理</summary>
          <div class="tree-body">
            <div v-if="outputKind === 'LOCAL'" class="tree-row">
              <span class="tree-label">跳过目录空性检查</span>
              <label class="checkbox-label"><input v-model="skipCheckDir" type="checkbox" />--skip-check-dir</label>
            </div>
            <p v-if="skipCheckDir" class="section-hint">将生成 `--skip-check-dir`。仍会检查导出路径和日志路径可写、位于允许根目录内，以及导出路径可用空间。</p>
            <div class="tree-row">
              <span class="tree-label">导出总量上限 <span class="muted">（Byte）</span></span>
              <input v-model.trim="maxFileSize" class="tree-input" placeholder="正整数，例如 1048576" />
            </div>
            <div v-if="formatKind === 'CUT'" class="tree-row">
              <span class="tree-label">删除换行 <span class="muted">（高风险）</span></span>
              <label class="checkbox-label"><input v-model="removeNewline" type="checkbox" />--remove-newline</label>
            </div>
            <p v-if="removeNewline" class="section-hint">--remove-newline 会删除导出数据内的换行并改变数据内容，请确认可接受后再开启。</p>
          </div>
        </details>
        <details v-if="dataOptionsActive" class="tree-node">
          <summary>高级选项 · 功能选项 · 时间戳格式</summary>
          <div class="tree-body">
            <div class="tree-row">
              <span class="tree-label">闪回时间点 <span class="muted">（仅 Oracle）</span></span>
              <input v-model.trim="flashbackTimestamp" class="tree-input" :disabled="Boolean(querySql)" placeholder="例如 2026-08-06 00:00:00" />
            </div>
            <div class="tree-row pending">
              <span class="tree-label">NLS / 日期时间值格式</span>
              <span class="tree-note">待验证（--nls-date-format、--date-value-format 等）</span>
            </div>
            <p class="section-hint">--flashback-scn 已在步骤 4 文件格式节点配置；与自定义查询互斥。</p>
          </div>
        </details>
        <details v-if="dataOptionsActive" class="tree-node">
          <summary>高级选项 · 功能选项 · 黑白名单筛选</summary>
          <div class="tree-body">
            <div class="tree-row">
              <span class="tree-label">自定义查询 <span class="muted">（专家受限）</span></span>
              <textarea v-model.trim="querySql" class="tree-input tree-textarea" rows="2" placeholder="例如 SELECT * FROM t WHERE id > 0" />
            </div>
            <p v-if="querySql" class="section-hint">自定义查询为受限专家能力：不提供 SQL 编辑器，服务端只把已确认文本作为 --query-sql 生成；结果不能直接导入。</p>
            <div class="tree-row">
              <span class="tree-label">保留空结果文件</span>
              <label class="checkbox-label"><input v-model="retainEmptyFiles" type="checkbox" />--retain-empty-files</label>
            </div>
            <div class="tree-row">
              <span class="tree-label">游标抓取行数 <span class="muted">（Oracle）</span></span>
              <input v-model.trim="fetchSize" class="tree-input" placeholder="继承 1000" />
            </div>
            <p class="section-hint">包含列、排除列与排除生成列已在步骤 2 配置；官方把 --fetch-size 归入列黑白名单筛选节，其语义为 Oracle 模式游标抓取行数。</p>
          </div>
        </details>
        <details v-if="dataOptionsActive" class="tree-node">
          <summary>高级选项 · 性能选项</summary>
          <div class="tree-body">
            <div class="tree-row">
              <span class="tree-label">导出线程</span>
              <input v-model.trim="thread" class="tree-input" placeholder="继承官方默认" />
            </div>
            <div class="tree-row">
              <span class="tree-label">分页大小</span>
              <input v-model.trim="pageSize" class="tree-input" placeholder="继承 1,000,000" />
            </div>
            <div class="tree-row">
              <span class="tree-label">每线程宏块数</span>
              <input v-model.trim="parallelMacro" class="tree-input" placeholder="继承 8" />
            </div>
            <div class="tree-row">
              <span class="tree-label">JVM 内存</span>
              <input v-model.trim="jvmMemory" class="tree-input" placeholder="例如 4G（K/M/G/T）" />
            </div>
          </div>
        </details>
        <p v-if="optionPanelsMessage" class="feedback feedback-error" role="alert">{{ optionPanelsMessage }}</p>
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
            <div><dt>对象与内容</dt><dd>{{ displayedDraftConfig ? `${draftScopeSummary} · ${draftContentLabel}` : '尚未读取草稿' }}</dd></div>
            <div><dt>导出、日志与节点</dt><dd>{{ displayedDraftConfig && displayedNode ? `${displayedNode.displayName} · 导出：${displayedDraftConfig.outputConfig.filePath}${displayedDraftConfig.outputConfig.logPath ? ` · 日志：${displayedDraftConfig.outputConfig.logPath}` : ''}${displayedDraftConfig.outputConfig.skipCheckDir ? ' · 已跳过目录空性检查' : ''}` : '尚未读取草稿' }}</dd></div>
          </dl>
        </section>
        <section class="precheck-list">
          <h3>预检查结果</h3>
          <p class="section-hint">预检查由已选择的 Agent 执行：确认数据源连接、当前草稿所选对象可读取（全部范围按数据库级可达性投影）、OB Loader/Dumper 与专用 Java 8 配置、导出目录及已填写日志目录可写、导出目录空性，以及至少 1 GiB 可用空间。勾选跳过选项时，仅目录空性检查会被跳过。它不会启动 OBDUMPER 或创建导出文件。</p>
          <p v-if="storagePrecheckBlocked" class="feedback feedback-notice" role="status">对象存储输出暂不能发起预检查：存储凭据、网络、权限与空间预检查尚未完成，固定预检查只接受本地绝对路径输出。</p>
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
        <div><dt>导出范围</dt><dd>{{ displayedDraftConfig ? draftScopeSummary : (database ? `${database} · ${scopeKind === 'ALL' ? '全部对象' : '指定对象'}` : '尚未配置') }}</dd></div>
        <div><dt>导出内容</dt><dd>{{ contentKind === 'DDL_ONLY' ? '仅 DDL' : contentKind === 'DDL_AND_DATA' ? 'DDL + 数据' : '仅数据' }}</dd></div>
        <div><dt>数据格式</dt><dd>{{ contentKind === 'DDL_ONLY' ? '无数据格式' : formatKind }}</dd></div>
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
          <button type="button" class="button button-secondary" :disabled="!currentDraft || startingPrecheck || precheckRunning || submitting || storagePrecheckBlocked" @click="startPrecheck">{{ footerLabel }}</button>
          <button type="button" class="button button-primary" :disabled="!canSubmit" @click="submitTask">{{ submitting ? '正在提交任务…' : '提交并启动导出' }}</button>
        </div>
      </footer>
    </template>
  </WizardFrame>
</template>
