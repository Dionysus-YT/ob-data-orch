<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Button as AButton, Checkbox as ACheckbox, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Empty as AEmpty, Form as AForm, FormItem as AFormItem, Input as AInput, List as AList, ListItem as AListItem, Modal as AModal, Radio as ARadio, RadioButton as ARadioButton, RadioGroup as ARadioGroup, Select as ASelect, SelectOptGroup as ASelectOptGroup, SelectOption as ASelectOption, Skeleton as ASkeleton, Spin as ASpin, Tag as ATag, Textarea as ATextarea } from 'ant-design-vue'
import { CaretDownOutlined, CaretRightOutlined, CodeOutlined, DeleteOutlined, EyeOutlined, FileTextOutlined, OrderedListOutlined, SearchOutlined, TableOutlined } from '@ant-design/icons-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { browserApi, dataSourceErrorMessage, exportCatalogErrorMessage, exportDraftErrorMessage, storageCredentialErrorMessage, type CommandPreview, type CompressionAlgo, type CsvQuoteMode, type DataSourceSummary, type ExecutionNodeCandidate, type ExportContentKind, type ExportDataFormatKind, type ExportDraft, type ExportObjectCatalogQuery, type ExportObjectType, type ExportOutputKind, type ExportScopeKind, type Precheck, type StorageCredentialListItem } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import WizardFrame from '@/components/WizardFrame.vue'
import { isExportEligibleDataSource } from './exportDataSourceEligibility'
import { BLOCK_SIZE_PATTERN, validateExportDraftInput } from './exportDraftInput'
import { fixedPrecheckChecks, precheckCheckLabel, precheckResultBlocksSubmission, precheckResultDetail, precheckResultLabel } from './exportPrecheckPresentation'

const api = browserApi()
const fieldPrefix = useId()
const submitCancelID = useId()
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
const draftDirty = ref(false)
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
const submitConfirmationOpen = ref(false)
let submitFocusTimer: ReturnType<typeof setTimeout> | undefined
// 保存请求期间若表单继续变化，旧响应只能更新草稿版本，不能解锁预检查或提交。
let formVersion = 0
const attemptedStep = ref(0)
const copyNotice = ref('')
const selectedDataSourceID = ref('')
const sourceKeyword = ref('')
const sourceEnvironment = ref('')
const selectedNodeID = ref('')
const database = ref('')
const databaseCatalogNames = ref<string[]>([])
const databaseCatalogKeyword = ref('')
const databaseCatalogLoading = ref(false)
const databaseCatalogLoaded = ref(false)
const databaseCatalogTruncated = ref(false)
const databaseCatalogFailure = ref('')
const manualDatabaseOpen = ref(false)
const manualDatabaseInput = ref('')
const manualDatabaseError = ref('')
let databaseCatalogEpoch = 0
let databaseCatalogTimer: ReturnType<typeof setTimeout> | undefined
let databaseCatalogLoadTimer: ReturnType<typeof setTimeout> | undefined
let databaseCatalogWaitResolve: (() => void) | undefined
const scopeKind = ref<ExportScopeKind>('SPECIFIED')
const objectType = ref<ExportObjectType>('TABLE')
const objectNames = ref<string[]>([''])
const candidateObjectNames = ref<string[]>([])
const candidateGroupExpanded = ref(true)
const selectedGroupExpanded = ref(true)
const manualCandidateNames = ref<string[]>([])
const candidateObjectInput = ref('')
const candidateEditorPanels = ref<string[]>([])
const candidateObjectKeyword = ref('')
const candidateObjectError = ref('')
const catalogKeywordTooLong = computed(() => new TextEncoder().encode(candidateObjectKeyword.value.trim()).length > 100)
const catalogLoading = ref(false)
const catalogLoaded = ref(false)
const catalogTruncated = ref(false)
const catalogFailure = ref('')
let catalogEpoch = 0
let catalogTimer: ReturnType<typeof setTimeout> | undefined
let catalogLoadTimer: ReturnType<typeof setTimeout> | undefined
let catalogWaitResolve: (() => void) | undefined
const selectedObjectKeyword = ref('')
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
// EX-I6 存储凭据槽位（2026-08-14）：对象存储输出可选绑定主体拥有的凭据引用。
// 只保存标识与当前修订；密钥永远由任务级安全槽位解析，不会进入页面状态或草稿。
const storageCredentials = ref<StorageCredentialListItem[]>([])
const loadingStorageCredentials = ref(false)
const storageCredentialLoadFailure = ref('')
const storageCredentialID = ref('')
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
// EX-I7 压缩等级（2026-08-10）：--compression-level，按所选算法分范围。
const compressionLevel = ref('')
const querySql = ref('')
// 条件筛选仅在指定表范围启用，并与自定义查询互斥。
const where = ref('')
const includeColumnNames = ref('')
const excludeColumnNames = ref('')
const excludeVirtualColumns = ref(false)
const flashbackScn = ref('')
const flashbackTimestamp = ref('')
// 一致性快照不能与任一闪回参数共同使用。
const snapshot = ref(false)
const thread = ref('')
const pageSize = ref('')
const parallelMacro = ref('')
const fetchSize = ref('')
const jvmMemory = ref('')
// EX-I7 文件拆分（2026-08-10）：--block-size（正整数 MB 或正整数+MB/ROW 后缀）。
const blockSize = ref('')
// 前置 DROP、保留 Schema 仅在 DDL 内容时生效；紧凑 Schema 还必须包含表 DDL。
// 附加对象信息仍需 sys 权限预检查，浏览器不提供启用入口。
const dropObject = ref(false)
const retainSchema = ref(false)
const compactSchema = ref(false)
const addExtraMessage = ref(false)
// 分区筛选仅指定表数据并与自定义查询互斥，类型排除仅数据内容。
// 隐藏主键仍需对象、版本与权限预检查，浏览器不提供启用入口。
const partition = ref('')
const excludeDataTypes = ref('')
const enableHiddenPk = ref(false)
// 时间格式当前只允许 MySQL CSV/CUT 数据导出的 DATE 与 DATETIME；其余字段必须保持清空。
const dateValueFormat = ref('')
const timeValueFormat = ref('')
const datetimeValueFormat = ref('')
const timestampValueFormat = ref('')
const timestampTzValueFormat = ref('')
const timestampLtzValueFormat = ref('')
const nlsDateFormat = ref('')
const nlsTimestampFormat = ref('')
const nlsTimestampTzFormat = ref('')
const dataOptionsActive = computed(() => contentKind.value !== 'DDL_ONLY')
const excludeTablesSupported = computed(() => (scopeKind.value === 'ALL' && dataOptionsActive.value) || (scopeKind.value === 'SPECIFIED' && objectType.value === 'TABLE'))
const whereSupported = computed(() => scopeKind.value === 'SPECIFIED' && objectType.value === 'TABLE')
const partitionSupported = computed(() => contentKind.value !== 'DDL_ONLY' && scopeKind.value === 'SPECIFIED' && objectType.value === 'TABLE')
const compactSchemaSupported = computed(() => contentKind.value !== 'DATA_ONLY' && (scopeKind.value === 'ALL' || objectType.value === 'TABLE'))
const objectOptionsMessage = computed(() => {
  if (!dataOptionsActive.value) return ''
  if (querySql.value.trim() && (where.value.trim() || partition.value.trim() || flashbackScn.value.trim() || flashbackTimestamp.value.trim())) return '自定义查询与条件、分区及闪回参数互斥，只能选择其一。'
  if (snapshot.value && (flashbackScn.value.trim() || flashbackTimestamp.value.trim())) return '一致性快照与闪回参数互斥，只能选择其一。'
  if (includeColumnNames.value.trim() && excludeColumnNames.value.trim()) return '包含列与排除列互斥，只能选择其一。'
  return ''
})
const formatOptionsMessage = computed(() => {
  if (dataOptionsActive.value && blockSize.value.trim() && !BLOCK_SIZE_PATTERN.test(blockSize.value.trim())) return '文件拆分必须为正整数（MB）或正整数+MB/ROW 后缀，例如 1024 或 256ROW。'
  if (compress.value && !compressionAlgo.value) return '启用压缩后请选择压缩算法。'
  return ''
})
const activeStep = computed(() => {
  const value = Number(route.query.step ?? '1')
  return Number.isInteger(value) && value >= 1 && value <= 5 ? value : 1
})

const titles = ['选择数据源', '导出内容与对象', '选择数据格式', '执行与输出', '预检查与命令']
const enteredObjectCount = computed(() => objectNames.value.filter((name) => name.trim().length > 0).length)
const selectedObjectRows = computed(() => objectNames.value.map((name, index) => ({ name: name.trim(), index })).filter((item) => item.name.length > 0))
const visibleSelectedObjectRows = computed(() => selectedObjectRows.value.filter((item) => item.name.toLocaleLowerCase().includes(selectedObjectKeyword.value.trim().toLocaleLowerCase())))
const visibleCandidateObjectNames = computed(() => candidateObjectNames.value.filter((name) => name.toLocaleLowerCase().includes(candidateObjectKeyword.value.trim().toLocaleLowerCase())))
const visibleCandidateSelectedCount = computed(() => visibleCandidateObjectNames.value.filter((name) => objectNames.value.includes(name)).length)
const objectCategories = [
  { type: 'TABLE', label: '表', icon: TableOutlined },
  { type: 'VIEW', label: '视图', icon: EyeOutlined },
  { type: 'FUNCTION', label: '函数', icon: CodeOutlined },
  { type: 'PROCEDURE', label: '存储过程', icon: FileTextOutlined },
  { type: 'SEQUENCE', label: '序列', icon: OrderedListOutlined },
] as const
const eligibleSources = computed(() => sources.value.filter(isExportEligibleDataSource))
const visibleSources = computed(() => eligibleSources.value.filter((source) =>
  (!sourceEnvironment.value || source.environment === sourceEnvironment.value)
  && (!sourceKeyword.value.trim() || source.displayName.toLocaleLowerCase().includes(sourceKeyword.value.trim().toLocaleLowerCase())),
))
const selectedSource = computed(() => eligibleSources.value.find((source) => source.id === selectedDataSourceID.value))
const selectedSourceRevision = computed(() => selectedSource.value?.revision ?? 0)
const selectedNode = computed(() => nodes.value.find((node) => node.id === selectedNodeID.value))
const databaseOptions = computed(() => [...new Set([
  ...databaseCatalogNames.value,
  selectedSource.value?.defaultDatabase ?? '',
  database.value,
].filter(Boolean))].filter((name) => name.toLocaleLowerCase().includes(databaseCatalogKeyword.value.trim().toLocaleLowerCase())))
// 仅 MySQL CSV/CUT 数据导出开放两个已验证时间格式；Oracle 的九个字段均保持关闭。
const timestampFormatsSupported = computed(() => selectedSource.value?.compatibilityMode === 'MYSQL' && contentKind.value !== 'DDL_ONLY' && (formatKind.value === 'CSV' || formatKind.value === 'CUT'))
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
// EX-I6 存储专用预检查（2026-08-14）：对象存储草稿可以发起预检查；
// 检查清单切换为存储形态，端点连通性与凭据有效性两项未授权探测保持 UNKNOWN，
// 提交按结果失败关闭（真实探测归 EX-V1）。
const storageOutput = computed(() => outputKind.value !== 'LOCAL')
// EX-I6 存储凭据槽位：只列出与当前输出类型同 provider 的凭据；不指定则依赖 Hadoop 标准配置链。
const matchingStorageCredentials = computed(() => storageCredentials.value.filter((credential) => credential.provider === outputKind.value))
const selectedStorageCredential = computed(() => storageCredentials.value.find((credential) => credential.id === storageCredentialID.value))
const precheckRows = computed(() => {
  // 预检查清单由服务端按输出类型确定：有结果时按服务端顺序展示，未执行时显示本地冻结六项。
  const checks = activePrecheck.value && activePrecheck.value.results.length > 0 ? activePrecheck.value.results.map((result) => result.check) : fixedPrecheckChecks
  return checks.map((check) => ({
    check,
    result: activePrecheck.value?.results.find((result) => result.check === check),
  }))
})
const blockingPrecheckRows = computed(() => precheckRows.value.filter((row) => precheckResultBlocksSubmission(row.result)))
// 预检查和脱敏命令必须绑定当前草稿版本、指纹与节点；旧证据不能解锁提交。
const canSubmit = computed(() => {
  const draft = currentDraft.value
  const precheck = activePrecheck.value
  return Boolean(draft && precheck && commandPreview.value
    && !draftDirty.value && !submitting.value
    && precheck.status === 'SUCCEEDED' && precheck.integrityStatus === 'COMPLETE'
    && precheck.draftId === draft.id && precheck.draftRevision === draft.revision
    && precheck.configFingerprint === draft.configFingerprint && precheck.nodeId === draft.nodeId
    && commandPreview.value.configFingerprint === draft.configFingerprint
    && blockingPrecheckRows.value.length === 0)
})
const footerBaselineNote = computed(() => {
	if (activeStep.value === 5) {
		if (storageOutput.value && blockingPrecheckRows.value.some((row) => row.check === 'STORAGE_CONNECTIVITY' || row.check === 'STORAGE_AUTH')) {
			return '对象存储输出需要存储端点连通性与凭据有效性两项预检查通过后才能提交；这两项探测尚未授权（归 EX-V1 排期），当前保持未完成。'
		}
		return canSubmit.value ? '预检查已通过。提交后，所选 Agent 将领取已冻结的导出任务并启动 OBDUMPER。' : (currentDraft.value ? '草稿已保存；请先完成当前版本的固定预检查，提交后才会启动 OBDUMPER。' : '尚未读取草稿；请返回上一步完成固定字段并创建草稿。')
	}
	if (currentStepError.value && (attemptedStep.value === activeStep.value || activeStep.value === 2)) return currentStepError.value
	return activeStep.value === 4 ? '创建草稿时服务端会再次校验数据源和节点授权。' : '填写当前步骤后继续；进入下一步前会检查当前输入。'
})
const objectInputMessage = computed(() => {
  if (!database.value.trim()) return '请选择数据库或 Schema。'
  if (scopeKind.value === 'ALL') return ''
  const names = objectNames.value.map((name) => name.trim()).filter((name) => name.length > 0)
  if (names.length === 0) return objectType.value === 'VIEW' ? '请至少选择一个视图。' : '请至少选择一个表。'
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
  compatibilityMode: selectedSource.value?.compatibilityMode ?? '',
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
  storageCredentialId: storageCredentialID.value,
  storageCredentialRevision: selectedStorageCredential.value?.currentRevision ?? 0,
  storageCredentialProvider: selectedStorageCredential.value?.provider ?? '',
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
  compressionLevel: compressionLevel.value,
  querySql: querySql.value,
  where: where.value,
  includeColumnNames: includeColumnNames.value,
  excludeColumnNames: excludeColumnNames.value,
  excludeVirtualColumns: excludeVirtualColumns.value,
  flashbackScn: flashbackScn.value,
  flashbackTimestamp: flashbackTimestamp.value,
  snapshot: snapshot.value,
  thread: thread.value,
  pageSize: pageSize.value,
  parallelMacro: parallelMacro.value,
  fetchSize: fetchSize.value,
  jvmMemory: jvmMemory.value,
  blockSize: blockSize.value,
  dropObject: dropObject.value,
  retainSchema: retainSchema.value,
  compactSchema: compactSchema.value,
  addExtraMessage: addExtraMessage.value,
  partition: partition.value,
  excludeDataTypes: excludeDataTypes.value,
  enableHiddenPk: enableHiddenPk.value,
  dateValueFormat: dateValueFormat.value,
  timeValueFormat: timeValueFormat.value,
  datetimeValueFormat: datetimeValueFormat.value,
  timestampValueFormat: timestampValueFormat.value,
  timestampTzValueFormat: timestampTzValueFormat.value,
  timestampLtzValueFormat: timestampLtzValueFormat.value,
  nlsDateFormat: nlsDateFormat.value,
  nlsTimestampFormat: nlsTimestampFormat.value,
  nlsTimestampTzFormat: nlsTimestampTzFormat.value,
}))
const ordinaryFormat = computed(() => contentKind.value === 'DDL_ONLY' || ['CSV', 'CUT', 'SQL'].includes(formatKind.value))
const draftInput = computed(() => ordinaryFormat.value && draftValidation.value.valid && !contentInputMessage.value && !objectInputMessage.value && !objectOptionsMessage.value && !formatOptionsMessage.value ? draftValidation.value.input : undefined)
const draftValidationMessage = computed(() => !ordinaryFormat.value ? '当前格式仅保留历史任务读取；普通新建入口只开放 CSV、CUT 和 SQL。' : draftValidation.value.valid ? contentInputMessage.value || objectInputMessage.value || objectOptionsMessage.value || formatOptionsMessage.value : draftValidation.value.message)
const derivedDraftBindingLocked = computed(() => currentDraft.value !== null && typeof route.query.draft === 'string' && route.query.draft === currentDraft.value.id)
const canAdvance = computed(() => {
  if (activeStep.value === 1) return Boolean(selectedSource.value)
  if (activeStep.value === 2) return Boolean(selectedSource.value && selectedNode.value) && !contentInputMessage.value && !objectInputMessage.value && !objectOptionsMessage.value
  if (activeStep.value === 3) return Boolean(selectedSource.value) && ordinaryFormat.value && !formatOptionsMessage.value
  if (activeStep.value === 4) return Boolean(draftInput.value) && !creatingDraft.value
  return Boolean(selectedSource.value) && activeStep.value < 4
})
const footerLabel = computed(() => {
  if (activeStep.value === 4) return creatingDraft.value ? '正在保存草稿…' : currentDraft.value ? '保存并进入预检查' : '创建草稿并进入预检查'
  if (activeStep.value === 5) {
    if (startingPrecheck.value) return '正在发起预检查…'
    if (precheckRunning.value) return '预检查进行中…'
    if (precheckID.value && !activePrecheck.value) return '重新读取预检查'
    return activePrecheck.value ? '重新执行预检查' : '执行预检查'
  }
  return `下一步：${titles[activeStep.value]}`
})
const currentStepError = computed(() => {
  if (activeStep.value <= 3 && !selectedSource.value) return '请选择已启用且基础连接测试成功的数据源。'
  if (activeStep.value === 2 && !selectedNode.value) return '请选择读取元数据并执行导出的节点。'
  if (activeStep.value === 2) return contentInputMessage.value || objectInputMessage.value || objectOptionsMessage.value
  if (activeStep.value === 3) return !ordinaryFormat.value ? draftValidationMessage.value : formatOptionsMessage.value
  if (activeStep.value === 4) return draftValidationMessage.value
  return ''
})
watch(activeStep, () => { attemptedStep.value = 0 })
// 弹窗打开动画会重新设置焦点；动画结束后将焦点移到安全的取消操作。
watch(submitConfirmationOpen, (open) => {
  if (submitFocusTimer) clearTimeout(submitFocusTimer)
  if (open) submitFocusTimer = setTimeout(() => document.getElementById(submitCancelID)?.focus(), 280)
})
const outputPathPlaceholder = computed(() => selectedNode.value?.platform === 'WINDOWS_AMD64'
  ? '例如 /E:/exports/daily'
  : '例如 /var/ob-data-orch/exports/daily')
let precheckPollTimer: ReturnType<typeof setTimeout> | undefined
let precheckPollResolve: (() => void) | undefined
let precheckPollVersion = 0
let hydratingDerivedDraft = false

// clearGatedParameters 清除尚未完成专用预检查的开关，避免状态切换后留下可提交残留。
function clearGatedParameters() {
  addExtraMessage.value = false
  enableHiddenPk.value = false
}

// clearTimestampFormats 清除全部时间格式，Oracle 与未知兼容模式不得保留任何待验证字段。
function clearTimestampFormats() {
  dateValueFormat.value = ''
  timeValueFormat.value = ''
  datetimeValueFormat.value = ''
  timestampValueFormat.value = ''
  timestampTzValueFormat.value = ''
  timestampLtzValueFormat.value = ''
  nlsDateFormat.value = ''
  nlsTimestampFormat.value = ''
  nlsTimestampTzFormat.value = ''
}

// clearUnverifiedTimestampFormats 清除 MySQL 已验证 DATE/DATETIME 以外的七个字段。
function clearUnverifiedTimestampFormats() {
  timeValueFormat.value = ''
  timestampValueFormat.value = ''
  timestampTzValueFormat.value = ''
  timestampLtzValueFormat.value = ''
  nlsDateFormat.value = ''
  nlsTimestampFormat.value = ''
  nlsTimestampTzFormat.value = ''
}

watch(selectedSource, (source, previousSource) => {
  if (hydratingDerivedDraft) return
  if (source?.id === previousSource?.id) return
  // 服务端不允许更新已保存草稿的数据源绑定；更换后下一次保存必须创建新草稿。
  if (currentDraft.value && source?.id !== currentDraft.value.dataSourceId) clearDraftState()
  database.value = source?.defaultDatabase ?? ''
  scopeKind.value = 'SPECIFIED'
  objectType.value = 'TABLE'
  objectNames.value = ['']
  candidateObjectNames.value = []
  manualCandidateNames.value = []
  candidateObjectInput.value = ''
  candidateEditorPanels.value = []
  candidateObjectKeyword.value = ''
  candidateObjectError.value = ''
  excludeTablesText.value = ''
  contentKind.value = 'DATA_ONLY'
  clearGatedParameters()
  clearTimestampFormats()
  invalidateDraftState()
})

watch(selectedSourceRevision, (revision, previousRevision) => {
  if (hydratingDerivedDraft || !previousRevision || revision === previousRevision) return
  objectNames.value = ['']
  candidateObjectNames.value = []
  manualCandidateNames.value = []
  candidateObjectKeyword.value = ''
  selectedObjectKeyword.value = ''
})

watch(selectedNodeID, (nodeID) => {
  if (hydratingDerivedDraft) return
  // 执行节点绑定同样不可更新；清除旧草稿避免提交旧节点的预检查证据。
  if (currentDraft.value && nodeID !== currentDraft.value.nodeId) clearDraftState()
  if (scopeKind.value === 'SPECIFIED') {
    objectNames.value = ['']
    candidateObjectNames.value = []
    manualCandidateNames.value = []
    candidateObjectKeyword.value = ''
    selectedObjectKeyword.value = ''
  }
})

watch([database, scopeKind, objectType, objectNames, excludeTablesText, contentKind, formatKind, trailDelimiter, removeNewline, columnSplitter, selectedNodeID, filePath, logPath, skipCheckDir, outputKind, storageBucket, storagePath, storageEndpoint, storageRegion, tmpPath, storageCredentialID, controlFilePath, skipHeader, columnSeparator, columnQuote, columnQuoteMode, escapeCharacter, lineSeparator, nullString, fileEncoding, withTrim, noNestedDir, maxFileSize, retainEmptyFiles, compress, compressionAlgo, compressionLevel, querySql, where, includeColumnNames, excludeColumnNames, excludeVirtualColumns, flashbackScn, flashbackTimestamp, snapshot, thread, pageSize, parallelMacro, fetchSize, jvmMemory, blockSize, dropObject, retainSchema, compactSchema, addExtraMessage, partition, excludeDataTypes, enableHiddenPk, dateValueFormat, timeValueFormat, datetimeValueFormat, timestampValueFormat, timestampTzValueFormat, timestampLtzValueFormat, nlsDateFormat, nlsTimestampFormat, nlsTimestampTzFormat], () => {
  if (hydratingDerivedDraft) return
  // 任一配置变化都会清除旧预览和预检查；已有草稿保留标识，待保存时更新。
  invalidateDraftState()
}, { deep: true })

// EX-I6 存储凭据槽位：切换输出类型时清除凭据引用，避免跨 provider 或本地输出残留绑定。
watch(outputKind, () => {
  if (hydratingDerivedDraft) return
  storageCredentialID.value = ''
})

watch(scopeKind, (kind) => {
  if (hydratingDerivedDraft) return
  clearGatedParameters()
  // 条件筛选只可随指定表发送，切到全部对象或已选视图时清除残留。
  if (kind !== 'SPECIFIED' || objectType.value !== 'TABLE') {
    where.value = ''
    partition.value = ''
  }
})

watch(objectType, (type) => {
  if (hydratingDerivedDraft) return
  clearGatedParameters()
  // 切换对象类型时，旧名称不能被重新解释成另一种对象类型。
  objectNames.value = ['']
  candidateObjectNames.value = []
  manualCandidateNames.value = []
  candidateObjectInput.value = ''
  candidateEditorPanels.value = []
  candidateObjectKeyword.value = ''
  candidateObjectError.value = ''
  selectedObjectKeyword.value = ''
  if (type === 'VIEW') {
    where.value = ''
    partition.value = ''
    if (contentKind.value !== 'DDL_ONLY') contentKind.value = 'DDL_ONLY'
  }
})

watch(database, (value, previousValue) => {
  if (hydratingDerivedDraft || value === previousValue) return
  // 候选与已选名称属于当前数据库；切库后不能把旧对象悄悄带入新范围。
  objectNames.value = ['']
  candidateObjectNames.value = []
  manualCandidateNames.value = []
  candidateObjectInput.value = ''
  candidateEditorPanels.value = []
  candidateObjectKeyword.value = ''
  candidateObjectError.value = ''
  selectedObjectKeyword.value = ''
})

watch(objectNames, (names) => {
  // 恢复历史草稿时，将已选对象并入候选区，使两栏状态保持一致。
  const missing = names.map((name) => name.trim()).filter((name) => name && !candidateObjectNames.value.includes(name))
  if (missing.length > 0) candidateObjectNames.value = [...candidateObjectNames.value, ...missing]
}, { deep: true })

watch(candidateObjectInput, () => { candidateObjectError.value = '' })

function stopCatalogQuery() {
  catalogEpoch++
  if (catalogTimer) clearTimeout(catalogTimer)
  catalogWaitResolve?.()
  catalogWaitResolve = undefined
  if (catalogLoadTimer) clearTimeout(catalogLoadTimer)
  catalogTimer = undefined
  catalogLoadTimer = undefined
  catalogLoading.value = false
  catalogLoaded.value = false
  catalogTruncated.value = false
  catalogFailure.value = ''
}

function stopDatabaseCatalogQuery() {
  databaseCatalogEpoch++
  if (databaseCatalogTimer) clearTimeout(databaseCatalogTimer)
  databaseCatalogWaitResolve?.()
  databaseCatalogWaitResolve = undefined
  if (databaseCatalogLoadTimer) clearTimeout(databaseCatalogLoadTimer)
  databaseCatalogTimer = undefined
  databaseCatalogLoadTimer = undefined
  databaseCatalogLoading.value = false
  databaseCatalogFailure.value = ''
}

async function loadDatabaseCatalog() {
  if (databaseCatalogLoading.value) return
  stopDatabaseCatalogQuery()
  const epoch = databaseCatalogEpoch
  const source = selectedSource.value
  const nodeID = selectedNodeID.value
  const keyword = databaseCatalogKeyword.value.trim()
  if (new TextEncoder().encode(keyword).length > 100) {
    databaseCatalogFailure.value = '数据库关键字最多 100 字节。'
    return
  }
  if (activeStep.value !== 2 || !source || !nodeID) return
  databaseCatalogLoading.value = true
  try {
    let query = await api.searchExportObjects(source.id, source.revision, { nodeId: nodeID, database: '', objectType: 'DATABASE', keyword })
    if (epoch !== databaseCatalogEpoch) return
    while (query.status === 'PENDING' || query.status === 'LEASED') {
      await new Promise<void>((resolve) => { databaseCatalogWaitResolve = resolve; databaseCatalogTimer = setTimeout(resolve, 2000) })
      databaseCatalogWaitResolve = undefined
      if (epoch !== databaseCatalogEpoch) return
      query = await api.getExportObjectCatalogQuery(query.id)
      if (epoch !== databaseCatalogEpoch) return
    }
    if (query.dataSourceId !== source.id || query.nodeId !== nodeID || query.database !== '' || query.objectType !== 'DATABASE' || query.keyword !== keyword) {
      databaseCatalogFailure.value = '数据库查询结果与当前条件不一致，请重新加载。'
      return
    }
    if (query.status !== 'SUCCEEDED') {
      databaseCatalogFailure.value = query.status === 'EXPIRED' ? '执行节点未及时完成数据库查询。可重新加载或手动输入。' : '执行节点未能读取数据库目录，可手动输入并由预检查确认。'
      return
    }
    databaseCatalogNames.value = [...query.objects]
    databaseCatalogLoaded.value = true
    databaseCatalogTruncated.value = query.truncated
  } catch (error) {
    if (epoch === databaseCatalogEpoch) databaseCatalogFailure.value = exportCatalogErrorMessage(error)
  } finally {
    if (epoch === databaseCatalogEpoch) databaseCatalogLoading.value = false
  }
}

function scheduleDatabaseCatalog(keyword: string) {
  databaseCatalogKeyword.value = keyword
  stopDatabaseCatalogQuery()
  databaseCatalogNames.value = []
  databaseCatalogLoaded.value = false
  databaseCatalogTruncated.value = false
  if (activeStep.value !== 2 || !selectedSource.value || !selectedNodeID.value) return
  databaseCatalogLoadTimer = setTimeout(() => { void loadDatabaseCatalog() }, 500)
}

function onDatabaseDropdownVisibleChange(open: boolean) {
  if (open && selectedNodeID.value && !databaseCatalogLoaded.value && !databaseCatalogLoading.value && !databaseCatalogFailure.value) {
    void loadDatabaseCatalog()
  }
}

function selectDatabaseOption(value: unknown) {
  if (typeof value !== 'string') return
  if (value === '__manual__') {
    manualDatabaseInput.value = database.value
    manualDatabaseError.value = ''
    manualDatabaseOpen.value = true
    return
  }
  database.value = value
}

function confirmManualDatabase() {
  const name = manualDatabaseInput.value.trim()
  if (!name || new TextEncoder().encode(name).length > 256 || /[*,\r\n\0]/.test(name)) {
    manualDatabaseError.value = '请输入不含通配符和逗号、最多 256 字节的数据库或 Schema 名称。'
    return
  }
  database.value = name
  manualDatabaseOpen.value = false
  manualDatabaseError.value = ''
}

watch([activeStep, selectedDataSourceID, selectedSourceRevision, selectedNodeID], () => {
  stopDatabaseCatalogQuery()
  databaseCatalogNames.value = []
  databaseCatalogLoaded.value = false
  databaseCatalogTruncated.value = false
  databaseCatalogKeyword.value = ''
  if (activeStep.value === 2 && selectedSource.value && selectedNodeID.value) {
    databaseCatalogLoadTimer = setTimeout(() => { void loadDatabaseCatalog() }, 500)
  }
}, { flush: 'post' })

function catalogMatches(query: ExportObjectCatalogQuery, sourceID: string, nodeID: string, schema: string, type: 'TABLE' | 'VIEW', keyword: string) {
  return query.dataSourceId === sourceID && query.nodeId === nodeID && query.database === schema && query.objectType === type && query.keyword === keyword
}

async function loadCatalog() {
  if (catalogLoading.value) return
  stopCatalogQuery()
  const epoch = catalogEpoch
  const source = selectedSource.value
  const nodeID = selectedNodeID.value
  const schema = database.value.trim()
  const type = objectType.value
  const keyword = candidateObjectKeyword.value.trim()
  if (catalogKeywordTooLong.value) { catalogFailure.value = '名称关键字最多 100 字节，请缩短后重试。'; return }
  if (activeStep.value !== 2 || scopeKind.value !== 'SPECIFIED' || !source || !nodeID || !schema) return
  catalogLoading.value = true
  try {
    let query = await api.searchExportObjects(source.id, source.revision, { nodeId: nodeID, database: schema, objectType: type, keyword })
    if (epoch !== catalogEpoch || !catalogMatches(query, source.id, nodeID, schema, type, keyword)) return
    while (query.status === 'PENDING' || query.status === 'LEASED') {
      await new Promise<void>((resolve) => { catalogWaitResolve = resolve; catalogTimer = setTimeout(resolve, 2000) })
      catalogWaitResolve = undefined
      if (epoch !== catalogEpoch) return
      query = await api.getExportObjectCatalogQuery(query.id)
      if (epoch !== catalogEpoch || !catalogMatches(query, source.id, nodeID, schema, type, keyword)) return
    }
    if (query.status !== 'SUCCEEDED') {
      catalogFailure.value = query.status === 'EXPIRED' ? '执行节点未及时领取或完成对象查询。请确认 Agent 在线后重试；也可手动添加对象。' : '节点未能读取对象元数据，请检查数据源连接与节点状态后重试。'
      return
    }
    catalogLoaded.value = true
    catalogTruncated.value = query.truncated
    const selected = objectNames.value.map((name) => name.trim()).filter(Boolean)
    manualCandidateNames.value = manualCandidateNames.value.filter((name) => !query.objects.includes(name))
    candidateObjectNames.value = [...new Set([...query.objects, ...selected])]
  } catch (error) {
    if (epoch === catalogEpoch) catalogFailure.value = exportCatalogErrorMessage(error)
  } finally {
    if (epoch === catalogEpoch) catalogLoading.value = false
  }
}

watch([activeStep, selectedDataSourceID, selectedSourceRevision, selectedNodeID, database, objectType, scopeKind], () => {
  if (hydratingDerivedDraft) return
  stopCatalogQuery()
  if (activeStep.value !== 2 || scopeKind.value !== 'SPECIFIED' || !selectedSource.value || !selectedNodeID.value || !database.value.trim()) return
  catalogLoadTimer = setTimeout(() => { void loadCatalog() }, 500)
}, { flush: 'post' })

watch(candidateObjectKeyword, () => { stopCatalogQuery() })

// 紧凑 Schema 失去表 DDL 前提时立即清值，避免隐藏残留进入草稿构造。
watch(compactSchemaSupported, (supported) => {
  if (hydratingDerivedDraft) return
  if (!supported) compactSchema.value = false
})

// 排除表失去适用范围时清值，避免折叠项隐藏后留下不可见的无效草稿参数。
watch(excludeTablesSupported, (supported) => {
  if (hydratingDerivedDraft) return
  if (!supported) excludeTablesText.value = ''
})

// 分区筛选失去指定表数据前提时立即清值；时间格式仅保留 MySQL 的两个已验证字段。
watch(partitionSupported, (supported) => {
  if (hydratingDerivedDraft) return
  if (!supported) partition.value = ''
})
watch(timestampFormatsSupported, (supported) => {
  if (hydratingDerivedDraft) return
  clearUnverifiedTimestampFormats()
  if (!supported) {
    dateValueFormat.value = ''
    datetimeValueFormat.value = ''
  }
}, { immediate: true })

watch(compress, (enabled) => {
  if (hydratingDerivedDraft) return
  // 取消压缩时清除算法与等级值，避免禁用态控件保留旧值造成校验死锁。
  if (!enabled) {
    compressionAlgo.value = ''
    compressionLevel.value = ''
  }
})

// 切换压缩算法时清除不适用或越界的等级值（gzip/snappy 不支持等级）。
watch(compressionAlgo, (algo) => {
  if (hydratingDerivedDraft) return
  if (algo === 'gzip' || algo === 'snappy') compressionLevel.value = ''
})

watch(contentKind, (kind) => {
  if (hydratingDerivedDraft) return
  clearGatedParameters()
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
    blockSize.value = ''
    maxFileSize.value = ''
    retainEmptyFiles.value = false
    compress.value = false
    compressionAlgo.value = ''
    querySql.value = ''
    where.value = ''
    includeColumnNames.value = ''
    excludeColumnNames.value = ''
    excludeVirtualColumns.value = false
    flashbackScn.value = ''
    flashbackTimestamp.value = ''
    snapshot.value = false
    thread.value = ''
    pageSize.value = ''
    parallelMacro.value = ''
    fetchSize.value = ''
    jvmMemory.value = ''
    // 仅 DDL 时清除数据专属筛选与全部时间格式。
    partition.value = ''
    excludeDataTypes.value = ''
    clearTimestampFormats()
  }
  // DDL + 数据只支持 CSV 数据格式：切换到该内容类型时重置非 CSV 格式，
  // 其余格式专属残留由 formatKind 监听器在离开对应格式时清理。
  if (kind === 'DDL_AND_DATA' && formatKind.value !== 'CSV') formatKind.value = 'CSV'
  // 仅数据内容不携带前置 DROP、保留 Schema 与紧凑 Schema，切换时清除残留。
  if (kind === 'DATA_ONLY') {
    dropObject.value = false
    retainSchema.value = false
    compactSchema.value = false
  }
})

// 切换数据格式时清空不适用格式的选项，避免残留值进入下一个草稿。
watch(formatKind, (kind, previousKind) => {
  if (hydratingDerivedDraft) return
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
  // 结构化格式不支持文件拆分参数；隐藏输入时同步清空，避免旧值进入新草稿。
  if (kind === 'PARQUET' || kind === 'ORC' || kind === 'AVRO') blockSize.value = ''
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
  void initializeWizard()
})

// initializeWizard 先加载回填所需的引用数据，再处理派生草稿，避免数据源列表迟到触发 watcher 清空来源关系。
async function initializeWizard() {
  await Promise.all([loadSources(), loadNodeCandidates(), loadStorageCredentials()])
  // EX-I8：派生草稿入口（基于原配置新建/从头重新执行）通过 ?draft=<id>&step=N 加载；
  // 从头重新执行直接进入预检查步骤（服务端在提交时强制参数不可变）。
  const derivedDraftID = typeof route.query.draft === 'string' ? route.query.draft : ''
  if (derivedDraftID) await loadDerivedDraft(derivedDraftID)
}
onBeforeUnmount(() => {
  stopPrecheckPolling()
  stopCatalogQuery()
  stopDatabaseCatalogQuery()
  if (submitFocusTimer) clearTimeout(submitFocusTimer)
})

// loadDerivedDraft 加载派生草稿：回填向导表单（可编辑），并读取命令预览与草稿状态。
async function loadDerivedDraft(draftID: string) {
  loadingDraft.value = true
  draftLoadFailure.value = ''
  try {
    const draft = await api.getExportDraft(draftID)
    if (typeof route.query.draft !== 'string' || route.query.draft !== draftID) return
    hydratingDerivedDraft = true
    try {
      populateFormFromDraft(draft)
      // Vue 默认在下一轮刷新 watcher；保持 hydration 标记直到本轮 watcher 全部跳过。
      await nextTick()
    } finally {
      hydratingDerivedDraft = false
    }
    createdDraftID.value = draftID
    currentDraft.value = draft
    draftDirty.value = false
    draftNotice.value = '已加载来源任务派生的草稿；基于原配置新建可以修改参数，从头重新执行不允许修改参数（提交时由服务端复验）。'
    await loadCommandPreview(draft)
  } catch (error) {
    draftLoadFailure.value = exportDraftErrorMessage(error, '无法读取派生草稿。')
  } finally {
    loadingDraft.value = false
  }
}

// populateFormFromDraft 把服务端冻结配置回填到向导表单（纯展示映射，不改变语义）。
function populateFormFromDraft(draft: ExportDraft) {
  const config = draft.config
  selectedDataSourceID.value = draft.dataSourceId
  selectedNodeID.value = draft.nodeId
  database.value = config.objectScope.database
  scopeKind.value = config.objectScope.scopeKind
  objectType.value = config.objectScope.objectTypes?.[0] ?? 'TABLE'
  objectNames.value = (config.objectScope.expressions ?? []).map((expression) => expression.name)
  if (objectNames.value.length === 0) objectNames.value = ['']
  candidateObjectNames.value = objectNames.value.filter(Boolean)
  manualCandidateNames.value = candidateObjectNames.value.slice()
  candidateObjectInput.value = ''
  candidateEditorPanels.value = []
  candidateObjectKeyword.value = ''
  candidateObjectError.value = ''
  selectedObjectKeyword.value = ''
  excludeTablesText.value = (config.objectScope.excludeTables ?? []).join(',')
  contentKind.value = config.contentSelection.contentKind
  formatKind.value = config.dataFormat?.formatKind ?? 'CSV'
  const output = config.outputConfig
  filePath.value = output.outputKind === 'LOCAL' ? output.filePath : ''
  logPath.value = output.logPath ?? ''
  skipCheckDir.value = output.skipCheckDir ?? false
  noNestedDir.value = output.noNestedDir ?? false
  maxFileSize.value = output.maxFileSize !== undefined ? String(output.maxFileSize) : ''
  retainEmptyFiles.value = output.retainEmptyFiles ?? false
  compress.value = output.compress ?? false
  compressionAlgo.value = output.compressionAlgo ?? ''
  compressionLevel.value = output.compressionLevel !== undefined ? String(output.compressionLevel) : ''
  controlFilePath.value = output.controlFilePath ?? ''
  tmpPath.value = output.tmpPath ?? ''
  if (output.outputKind !== 'LOCAL') {
    outputKind.value = output.outputKind
    const parsed = parseControlledStorageURI(output.filePath)
    storageBucket.value = parsed.bucket
    storagePath.value = parsed.path
    storageEndpoint.value = parsed.endpoint
    storageRegion.value = parsed.region
    storageCredentialID.value = output.storageCredential?.storageCredentialId ?? ''
  }
  const csv = config.dataFormat?.csvOptions
  if (csv) {
    skipHeader.value = csv.skipHeader ?? false
    columnSeparator.value = csv.columnSeparator ?? ''
    columnQuote.value = csv.columnQuote ?? ''
    columnQuoteMode.value = csv.columnQuoteMode ?? ''
    escapeCharacter.value = csv.escapeCharacter ?? ''
    lineSeparator.value = csv.lineSeparator ?? ''
    nullString.value = csv.nullString ?? ''
    fileEncoding.value = csv.fileEncoding ?? ''
    withTrim.value = csv.withTrim ?? false
    columnSplitter.value = csv.columnSplitter ?? ''
  }
  const cut = config.dataFormat?.cutOptions
  if (cut) {
    trailDelimiter.value = cut.trailDelimiter ?? false
    removeNewline.value = cut.removeNewline ?? false
  }
  const formats = config.dataFormat?.timestampFormats
  if (formats) {
    dateValueFormat.value = formats.dateValueFormat ?? ''
    datetimeValueFormat.value = formats.datetimeValueFormat ?? ''
  }
  const filter = config.filterConfig
  if (filter) {
    querySql.value = filter.querySql ?? ''
    where.value = filter.where ?? ''
    includeColumnNames.value = (filter.includeColumnNames ?? []).join(',')
    excludeColumnNames.value = (filter.excludeColumnNames ?? []).join(',')
    excludeVirtualColumns.value = filter.excludeVirtualColumns ?? false
    flashbackScn.value = filter.flashbackScn !== undefined ? String(filter.flashbackScn) : ''
    flashbackTimestamp.value = filter.flashbackTimestamp ?? ''
    snapshot.value = filter.snapshot ?? false
    partition.value = filter.partition ?? ''
    excludeDataTypes.value = (filter.excludeDataTypes ?? []).join(',')
  }
  const performance = config.performanceConfig
  if (performance) {
    thread.value = performance.thread !== undefined ? String(performance.thread) : ''
    pageSize.value = performance.pageSize !== undefined ? String(performance.pageSize) : ''
    parallelMacro.value = performance.parallelMacro !== undefined ? String(performance.parallelMacro) : ''
    fetchSize.value = performance.fetchSize !== undefined ? String(performance.fetchSize) : ''
    jvmMemory.value = performance.jvmMemory ?? ''
    blockSize.value = performance.blockSize ?? ''
  }
  const ddl = config.ddlBehavior
  if (ddl) {
    dropObject.value = ddl.dropObject ?? false
    retainSchema.value = ddl.retainSchema ?? false
    compactSchema.value = ddl.compactSchema ?? false
  }
}

// parseControlledStorageURI 把向导生成的受控 URI 还原为表单字段（与 buildOutputFilePath 互为逆操作）。
function parseControlledStorageURI(uri: string): { bucket: string; path: string; endpoint: string; region: string } {
  const schemeIndex = uri.indexOf('://')
  const queryIndex = uri.indexOf('?')
  const authority = uri.slice(schemeIndex + 3, queryIndex >= 0 ? queryIndex : undefined)
  const slashIndex = authority.indexOf('/')
  const bucket = slashIndex >= 0 ? authority.slice(0, slashIndex) : authority
  const path = slashIndex >= 0 ? authority.slice(slashIndex) : '/'
  let endpoint = ''
  let region = ''
  if (queryIndex >= 0) {
    for (const pair of uri.slice(queryIndex + 1).split('&')) {
      const [key, value] = pair.split('=')
      if (key === 'endpoint') endpoint = decodeURIComponent(value ?? '')
      if (key === 'region') region = decodeURIComponent(value ?? '')
    }
  }
  return { bucket, path, endpoint, region }
}

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

async function loadStorageCredentials() {
  loadingStorageCredentials.value = true
  storageCredentialLoadFailure.value = ''
  try {
    storageCredentials.value = await api.listStorageCredentials()
    // 当前选择已被删除或轮换出列表时清除引用，避免提交陈旧绑定。
    if (!storageCredentials.value.some((credential) => credential.id === storageCredentialID.value)) storageCredentialID.value = ''
  } catch (error) {
    storageCredentialLoadFailure.value = storageCredentialErrorMessage(error, '无法加载存储凭据，请稍后重试。')
  } finally {
    loadingStorageCredentials.value = false
  }
}

function moveToStep(step: number) {
  void router.replace({ query: { ...route.query, step: String(step) } })
}

function previousStep() {
  if (activeStep.value > 1) moveToStep(activeStep.value - 1)
}

function nextStep() {
  if (!canAdvance.value) {
    attemptedStep.value = activeStep.value
    return
  }
  if (activeStep.value === 4) {
    if (currentDraft.value && !draftDirty.value) {
      moveToStep(5)
      return
    }
    void createDraft()
    return
  }
  if (activeStep.value < 4 && canAdvance.value) moveToStep(activeStep.value + 1)
}

async function copyCommand() {
  if (!commandPreview.value) return
  try {
    await navigator.clipboard.writeText(commandPreview.value.command)
    copyNotice.value = '已复制脱敏命令；密码占位符不能用于直接执行。'
  } catch {
    copyNotice.value = '复制失败，请检查浏览器剪贴板权限。'
  }
}

async function createDraft(advance = true) {
  const input = draftInput.value
  if (!input || creatingDraft.value) return
  creatingDraft.value = true
  // EX-I8：已加载的派生草稿在步骤 5 保存时走更新路径，保留来源任务标记；
  // 全新草稿仍走创建路径。
  const existing = currentDraft.value
  if (existing) invalidateDraftState()
  else clearDraftState()
  const savedVersion = formVersion
  try {
    if (existing) {
      const updated = await api.updateExportDraft({ ...existing, dataSourceId: input.dataSourceId, nodeId: input.nodeId, config: input.config })
      if (updated.dataSourceId !== selectedDataSourceID.value || updated.nodeId !== selectedNodeID.value) {
        clearDraftState()
        draftNotice.value = '保存期间更换了数据源或执行节点；请为当前绑定创建新草稿。'
        return
      }
      createdDraftID.value = updated.id
      currentDraft.value = updated
      draftDirty.value = formVersion !== savedVersion
      draftNotice.value = draftDirty.value ? '保存期间配置又有变化；请再次保存当前配置。' : '草稿已保存，正在重算命令预览。'
      if (draftDirty.value) return
      if (advance) moveToStep(5)
      await loadCommandPreview(updated)
      return
    }
    const draftID = await api.createExportDraft(input)
    if (input.dataSourceId !== selectedDataSourceID.value || input.nodeId !== selectedNodeID.value) {
      clearDraftState()
      draftNotice.value = '保存期间更换了数据源或执行节点；请为当前绑定创建新草稿。'
      return
    }
    createdDraftID.value = draftID
    draftNotice.value = '导出草稿已创建，正在读取服务端配置快照。'
    await loadCreatedDraft(draftID, savedVersion)
    if (advance && currentDraft.value && !draftDirty.value) moveToStep(5)
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
  draftDirty.value = false
  draftFailure.value = ''
  draftLoadFailure.value = ''
  draftNotice.value = ''
  commandPreview.value = null
  commandPreviewFailure.value = ''
  copyNotice.value = ''
  activePrecheck.value = null
  precheckID.value = ''
  precheckFailure.value = ''
  submissionFailure.value = ''
  submitConfirmationOpen.value = false
}

// invalidateDraftState 清除旧预览与预检查，同时保留草稿标识；后续保存更新同一草稿，
// 派生草稿因此不会丢失来源关系，普通草稿也不会在每次编辑后生成孤立副本。
function invalidateDraftState() {
  formVersion += 1
  const draft = currentDraft.value
  clearDraftState()
  if (!draft) return
  createdDraftID.value = draft.id
  currentDraft.value = draft
  draftDirty.value = true
  draftNotice.value = '当前配置有未保存更改；保存后需重新生成命令预览与预检查。'
}

async function loadCreatedDraft(draftID = createdDraftID.value, savedVersion?: number) {
  if (!draftID) return
  loadingDraft.value = true
  draftLoadFailure.value = ''
  commandPreview.value = null
  commandPreviewFailure.value = ''
  try {
    const draft = await api.getExportDraft(draftID)
    if (createdDraftID.value !== draftID) return
    if (draft.dataSourceId !== selectedDataSourceID.value || draft.nodeId !== selectedNodeID.value) {
      clearDraftState()
      draftNotice.value = '读取草稿期间更换了数据源或执行节点；请为当前绑定创建新草稿。'
      return
    }
    currentDraft.value = draft
    draftDirty.value = savedVersion !== undefined && formVersion !== savedVersion
    draftNotice.value = draftDirty.value ? '保存期间配置又有变化；请再次保存当前配置。' : '导出草稿已从服务端读取。完整命令仅隐藏密码，其余参数由控制面生成并经本地校验后展示。'
    if (draftDirty.value) return
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
  copyNotice.value = ''
  commandPreviewFailure.value = ''
  try {
    const preview = await api.previewExportCommand(draft)
    if (draftDirty.value || currentDraft.value?.id !== draft.id || currentDraft.value.revision !== draft.revision) return
    commandPreview.value = preview
  } catch (error) {
    if (currentDraft.value?.id !== draft.id || currentDraft.value.revision !== draft.revision) return
    commandPreviewFailure.value = exportDraftErrorMessage(error, '无法生成命令预览，请重新读取草稿后重试。')
  } finally {
    if (currentDraft.value?.id === draft.id && currentDraft.value.revision === draft.revision) previewingCommand.value = false
  }
}

async function startPrecheck() {
  if (!currentDraft.value || draftDirty.value || !commandPreview.value || startingPrecheck.value || precheckRunning.value) return
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

function submitTask() {
	if (!canSubmit.value) return
	submissionFailure.value = ''
	submitConfirmationOpen.value = true
}

function closeSubmitConfirmation() {
	if (!submitting.value) submitConfirmationOpen.value = false
}

async function confirmSubmitTask() {
	if (!currentDraft.value || !activePrecheck.value || !canSubmit.value) return
	submitting.value = true
	submissionFailure.value = ''
	try {
		const taskID = await api.submitExportDraft(currentDraft.value, activePrecheck.value.id)
		submitConfirmationOpen.value = false
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

function addCandidateObjects() {
  const names = candidateObjectInput.value.split(/[\r\n,]+/).map((name) => name.trim()).filter(Boolean)
  if (names.length === 0) {
    candidateObjectError.value = '请输入至少一个对象名称。'
    return
  }
  if (names.some((name) => name.length > 256 || name.includes('*'))) {
    candidateObjectError.value = '对象名称不能使用通配符，且不超过 256 个字符。'
    return
  }
  const added = [...new Set(names)].filter((name) => !candidateObjectNames.value.includes(name))
  if (candidateObjectNames.value.length + added.length > 100) {
    candidateObjectError.value = '候选对象最多 100 个。'
    return
  }
  candidateObjectNames.value = [...candidateObjectNames.value, ...added]
  manualCandidateNames.value = [...manualCandidateNames.value, ...added]
  const selected = new Set(objectNames.value.map((name) => name.trim()).filter(Boolean))
  for (const name of names) selected.add(name)
  objectNames.value = [...selected]
  candidateObjectInput.value = ''
  candidateEditorPanels.value = []
  candidateObjectError.value = ''
}

function toggleCandidate(name: string) {
  const selected = new Set(objectNames.value.map((value) => value.trim()).filter(Boolean))
  if (selected.has(name)) {
    selected.delete(name)
    forgetManualCandidates([name])
  }
  else if (selected.size < 100) selected.add(name)
  else { candidateObjectError.value = '最多选择 100 个导出对象。'; return }
  objectNames.value = selected.size > 0 ? [...selected] : ['']
}

function toggleVisibleCandidates() {
  const selected = new Set(objectNames.value.map((name) => name.trim()).filter(Boolean))
  const allVisibleSelected = visibleCandidateObjectNames.value.every((name) => selected.has(name))
  for (const name of visibleCandidateObjectNames.value) {
    if (allVisibleSelected) selected.delete(name)
    else if (selected.size < 100) selected.add(name)
  }
  if (allVisibleSelected) forgetManualCandidates(visibleCandidateObjectNames.value)
  if (!allVisibleSelected && selected.size === 100) candidateObjectError.value = '最多选择 100 个导出对象。'
  objectNames.value = selected.size > 0 ? [...selected] : ['']
}

function chooseObjectCategory(type: string) {
  if (type !== 'TABLE' && type !== 'VIEW') return
  if (type === 'VIEW' && contentKind.value !== 'DDL_ONLY') return
  if (type === objectType.value) {
    candidateGroupExpanded.value = !candidateGroupExpanded.value
    return
  }
  const switchType = () => {
    objectType.value = type
    candidateGroupExpanded.value = true
    selectedGroupExpanded.value = true
  }
  if (enteredObjectCount.value === 0) {
    switchType()
    return
  }
  AModal.confirm({
    title: `切换到${type === 'VIEW' ? '视图' : '表'}？`,
    content: '当前已选对象会清空，切换后将重新读取该分类的元数据。',
    okText: '切换并清空',
    cancelText: '保留当前选择',
    onOk: switchType,
  })
}

function removeObjectNameRow(index: number) {
  forgetManualCandidates([objectNames.value[index] ?? ''])
  if (objectNames.value.length <= 1) {
    objectNames.value = ['']
    return
  }
  objectNames.value = objectNames.value.filter((_, position) => position !== index)
}

function clearObjectNameRows() {
  objectNames.value = ['']
  selectedObjectKeyword.value = ''
  forgetManualCandidates(manualCandidateNames.value)
  candidateObjectInput.value = ''
}

function forgetManualCandidates(names: readonly string[]) {
  const removed = new Set(names.filter((name) => manualCandidateNames.value.includes(name)))
  if (removed.size === 0) return
  manualCandidateNames.value = manualCandidateNames.value.filter((name) => !removed.has(name))
  candidateObjectNames.value = candidateObjectNames.value.filter((name) => !removed.has(name))
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
    <ATag :color="draftDirty ? 'warning' : createdDraftID ? 'success' : 'default'">{{ draftDirty ? '草稿有未保存更改' : createdDraftID ? '草稿已保存' : '草稿尚未创建' }}</ATag>
  </section>
  <WizardFrame kind="export" :active-step="activeStep" class="export-builder" @step-change="moveToStep">
    <template #actions><AButton v-if="activeStep === 4 && draftInput" :loading="creatingDraft" @click="createDraft(false)">保存草稿</AButton></template>
    <template #default>
      <AAlert v-if="attemptedStep === activeStep && currentStepError" class="export-step-error" type="error" show-icon :message="currentStepError" />
      <section v-if="activeStep === 1" class="form-section">
        <h2>选择已有数据源</h2>
        <p>向导只选择已启用、当前配置至少一次基础测试成功的数据源；不重复填写地址、用户名或密码。</p>
        <AForm layout="vertical" class="export-source-filter">
          <AFormItem label="按名称筛选数据源"><AInput v-model:value="sourceKeyword" aria-label="按名称筛选数据源" placeholder="输入数据源名称" allow-clear /></AFormItem>
          <AFormItem label="环境" :html-for="fieldPrefix + '-source-environment'"><ASelect :id="fieldPrefix + '-source-environment'" v-model:value="sourceEnvironment"><ASelectOption value="">全部环境</ASelectOption><ASelectOption value="DEVELOPMENT">开发</ASelectOption><ASelectOption value="TEST">测试</ASelectOption><ASelectOption value="STAGING">预生产</ASelectOption><ASelectOption value="PRODUCTION">生产</ASelectOption></ASelect></AFormItem>
        </AForm>
        <AAlert v-if="sourceLoadFailure" type="error" show-icon :message="sourceLoadFailure"><template #action><AButton type="link" @click="loadSources">重试</AButton></template></AAlert>
        <ASkeleton v-else-if="loadingSources" active :paragraph="{ rows: 3 }" aria-label="正在加载已授权数据源" />
        <EmptyState v-else-if="eligibleSources.length === 0" title="没有可选数据源" :description="sources.length === 0 ? '当前授权范围内没有数据源。请先登记数据源并完成一次成功的基础连接测试。' : '当前已授权数据源均未同时满足已启用和成功测试条件。请在数据源管理中完成受控测试并启用数据源。'" action="前往数据源管理" @action="router.push('/data-sources')" />
        <EmptyState v-else-if="visibleSources.length === 0" title="当前筛选无可选数据源" description="清除名称或环境筛选后重试。" />
        <ARadioGroup v-else v-model:value="selectedDataSourceID" class="export-source-list" aria-label="选择数据源" :disabled="derivedDraftBindingLocked">
          <ARadio v-for="source in visibleSources" :key="source.id" :value="source.id" class="export-source-choice">
            <span class="export-source-facts"><strong>{{ source.displayName }}</strong><span>{{ environmentLabel(source.environment) }} · {{ source.compatibilityMode }} · {{ source.host }}:{{ source.port }}</span><span>{{ source.clusterName || '未登记集群' }} / {{ source.tenantName }} · 基础连接测试成功：{{ lastTestLabel(source) }}</span></span>
          </ARadio>
        </ARadioGroup>
        <AAlert v-if="selectedSource && derivedDraftBindingLocked" type="info" show-icon message="派生草稿固定使用来源任务的数据源；如需更换数据源，请退出派生流程后新建草稿。" />
        <AAlert v-else-if="selectedSource" type="info" show-icon :message="`已选择 ${selectedSource.displayName}。更换数据源会清除当前对象选择；已保存草稿的数据源绑定不可更新，更换后需创建新草稿。`" />
        <p v-else-if="eligibleSources.length > 0" class="section-hint">请选择一个数据源后继续；任务级对象、权限、路径和空间检查仍将在预检查阶段执行。</p>
      </section>

      <section v-else-if="activeStep === 2" class="form-section">
        <h2>导出内容与对象</h2>
        <p>选择导出内容、数据库和对象范围。数据库与对象名称由所选执行节点读取；实际可访问性仍由预检查确认。</p>
        <AForm layout="vertical" class="export-content-form">
          <AFormItem label="导出内容" required>
            <ARadioGroup v-model:value="contentKind" class="export-content-options" button-style="solid" role="radiogroup" aria-label="导出内容"><ARadioButton value="DDL_AND_DATA" :disabled="objectType === 'VIEW' && scopeKind === 'SPECIFIED'">导出结构和数据</ARadioButton><ARadioButton value="DATA_ONLY" :disabled="objectType === 'VIEW' && scopeKind === 'SPECIFIED'">仅导出数据</ARadioButton><ARadioButton value="DDL_ONLY">仅导出结构</ARadioButton></ARadioGroup>
          </AFormItem>
        </AForm>
        <p class="section-hint">{{ contentKind === 'DDL_ONLY' ? '仅生成对象定义；数据格式和数据文件参数不参与任务。' : contentKind === 'DATA_ONLY' ? '仅导出表数据；视图等只支持结构的对象不可选。' : '同时生成对象定义与表数据；只支持结构的对象不会被标记为已导出数据。' }}</p>
        <div class="export-field-group">
          <h3>数据库与导出范围</h3>
          <AForm layout="vertical" class="export-field-body">
            <div class="export-database-grid">
              <AFormItem label="读取元数据的执行节点" :html-for="fieldPrefix + '-catalog-node'" required :help="nodeLoadFailure || '选择节点后加载当前数据源的数据库目录；执行任务时使用同一节点。'">
                <ASkeleton v-if="loadingNodes" active :paragraph="{ rows: 1 }" aria-label="正在加载已授权执行节点" />
                <ASelect v-else :id="fieldPrefix + '-catalog-node'" v-model:value="selectedNodeID" aria-label="读取元数据的执行节点" :disabled="derivedDraftBindingLocked" placeholder="请选择执行节点"><ASelectOption v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</ASelectOption></ASelect>
              </AFormItem>
              <AFormItem label="数据库 / Schema" :html-for="fieldPrefix + '-database'" required :validate-status="attemptedStep === 2 && !database.trim() ? 'error' : undefined" :help="attemptedStep === 2 && !database.trim() ? '请选择数据库或 Schema。' : `数据源：${selectedSource?.displayName ?? '尚未选择'}；可搜索已读取的数据库，或手动输入其他名称。`">
                <ASelect :id="fieldPrefix + '-database'" :value="database || undefined" aria-label="数据库 / Schema" show-search :filter-option="false" :loading="databaseCatalogLoading" placeholder="请选择数据库 / Schema" @search="scheduleDatabaseCatalog" @select="selectDatabaseOption" @dropdown-visible-change="onDatabaseDropdownVisibleChange">
                  <ASelectOption v-if="!selectedNodeID" value="__choose_node__" disabled>请先选择读取元数据的执行节点</ASelectOption>
                  <ASelectOptGroup v-if="databaseOptions.length" :label="selectedSource?.displayName ?? '当前数据源'">
                    <ASelectOption v-for="name in databaseOptions" :key="name" :value="name" :label="name">{{ name }}</ASelectOption>
                  </ASelectOptGroup>
                  <ASelectOption v-if="databaseCatalogLoading" value="__loading__" disabled>正在读取数据库目录…</ASelectOption>
                  <ASelectOption v-else-if="databaseCatalogFailure" value="__unavailable__" disabled>目录暂不可用，可重试或手动输入</ASelectOption>
                  <ASelectOption v-else-if="selectedNodeID && databaseCatalogLoaded && !databaseOptions.length" value="__empty__" disabled>没有匹配的数据库 / Schema</ASelectOption>
                  <ASelectOption value="__manual__" label="手动输入其他数据库 / Schema">手动输入其他数据库 / Schema…</ASelectOption>
                </ASelect>
              </AFormItem>
            </div>
            <AAlert v-if="databaseCatalogFailure" type="warning" show-icon :message="databaseCatalogFailure"><template #action><AButton type="link" :disabled="!selectedNodeID" @click="loadDatabaseCatalog">重试</AButton></template></AAlert>
            <p v-else-if="databaseCatalogTruncated" class="section-hint" role="status">仅显示前 100 个匹配数据库；输入关键字继续筛选。</p>
            <p v-else-if="!selectedNodeID" class="section-hint">选择执行节点后加载数据库目录；已登记的默认数据库仍可直接选择。</p>
            <AFormItem label="导出范围" required><ARadioGroup v-model:value="scopeKind" role="radiogroup" aria-label="导出范围"><ARadio value="SPECIFIED">部分导出</ARadio><ARadio value="ALL">整库导出</ARadio></ARadioGroup></AFormItem>
            <p v-if="scopeKind === 'ALL'" class="section-hint">整库导出按当前数据库范围生成 --all；指定对象区已收起。</p>
          </AForm>
        </div>
        <AModal :open="manualDatabaseOpen" title="手动输入数据库 / Schema" ok-text="使用此名称" cancel-text="取消" @ok="confirmManualDatabase" @cancel="manualDatabaseOpen = false">
          <AInput v-model:value="manualDatabaseInput" aria-label="手动输入数据库 / Schema" autocomplete="off" :maxlength="256" @press-enter="confirmManualDatabase" />
          <AAlert v-if="manualDatabaseError" type="error" show-icon :message="manualDatabaseError" />
          <p class="section-hint">目录不可用时可填写明确名称，最终访问权限由预检查确认。</p>
        </AModal>
        <div v-if="scopeKind === 'SPECIFIED'" class="export-field-group export-object-group">
          <div class="export-group-heading"><h3>导出对象</h3><span class="export-object-count" role="status">已选 {{ enteredObjectCount }} / 100 项</span></div>
          <div class="export-object-workspace">
            <section class="export-object-pane" aria-label="选择导出对象">
              <div class="export-object-pane-heading"><h4>选择对象</h4></div>
              <div class="export-object-search"><AInput v-model:value="candidateObjectKeyword" aria-label="搜索候选对象" placeholder="搜索关键字" :maxlength="100" allow-clear @press-enter="loadCatalog" /><AButton :loading="catalogLoading" :disabled="catalogLoading || !selectedNodeID || !database.trim() || catalogKeywordTooLong" aria-label="搜索或刷新对象" @click="loadCatalog"><template #icon><SearchOutlined aria-hidden="true" /></template></AButton></div>
              <AAlert v-if="catalogKeywordTooLong" type="warning" show-icon message="名称关键字最多 100 字节。" />
              <AAlert v-if="catalogFailure" type="error" show-icon :message="catalogFailure" />
              <AAlert v-if="candidateObjectError" type="warning" show-icon :message="candidateObjectError" />
              <AAlert v-else-if="catalogTruncated" type="info" show-icon message="仅显示前 100 个匹配对象；输入名称关键字继续筛选。" />
              <p v-else-if="catalogLoading" class="section-hint" role="status">正在通过执行节点读取对象元数据…</p>
              <div class="export-object-tree" aria-label="候选对象分类">
                <template v-for="category in objectCategories" :key="category.type">
                  <div class="export-object-tree-category" :class="{ 'is-active': category.type === objectType }">
                    <AButton v-if="category.type === 'TABLE' || category.type === 'VIEW'" type="text" class="export-object-expand" :disabled="category.type === 'VIEW' && contentKind !== 'DDL_ONLY'" :aria-label="`${category.type === objectType && candidateGroupExpanded ? '收起' : '展开'}${category.label}分类`" @click="chooseObjectCategory(category.type)"><template #icon><CaretDownOutlined v-if="category.type === objectType && candidateGroupExpanded" aria-hidden="true" /><CaretRightOutlined v-else aria-hidden="true" /></template></AButton>
                    <span v-else class="export-object-expand-placeholder" aria-hidden="true" />
                    <ACheckbox v-if="category.type === objectType" :checked="visibleCandidateObjectNames.length > 0 && visibleCandidateSelectedCount === visibleCandidateObjectNames.length" :indeterminate="visibleCandidateSelectedCount > 0 && visibleCandidateSelectedCount < visibleCandidateObjectNames.length" :disabled="visibleCandidateObjectNames.length === 0" :aria-label="`选择全部可见${category.label}`" @change="toggleVisibleCandidates" />
                    <ACheckbox v-else disabled :aria-label="`${category.label}分类尚不可勾选`" />
                    <component :is="category.icon" class="export-object-kind-icon" aria-hidden="true" />
                    <AButton v-if="category.type === 'TABLE' || category.type === 'VIEW'" type="link" class="export-object-category-name" :disabled="category.type === 'VIEW' && contentKind !== 'DDL_ONLY'" @click="chooseObjectCategory(category.type)">{{ category.label }}（{{ category.type === objectType ? (catalogLoaded || candidateObjectNames.length ? candidateObjectNames.length : '待加载') : '待加载' }}）</AButton>
                    <span v-else class="export-object-category-name is-disabled">{{ category.label }}（未开放）</span>
                    <span v-if="category.type === 'VIEW' && contentKind !== 'DDL_ONLY'" class="export-object-kind-note">仅结构</span>
                  </div>
                  <template v-if="category.type === objectType && candidateGroupExpanded">
                    <AEmpty v-if="candidateObjectNames.length === 0 && !catalogLoading" class="export-object-tree-empty" :description="catalogFailure ? '自动读取失败，可重试或手动添加对象' : catalogLoaded ? '当前条件没有匹配对象' : '选择执行节点后加载对象'" />
                    <AEmpty v-else-if="visibleCandidateObjectNames.length === 0" class="export-object-tree-empty" description="没有匹配的候选对象" />
                    <AList v-else size="small" class="export-object-tree-children" :data-source="visibleCandidateObjectNames" aria-label="候选对象列表">
                      <template #renderItem="{ item }"><AListItem><ACheckbox :checked="objectNames.includes(item)" @change="toggleCandidate(item)"><component :is="category.icon" class="export-object-kind-icon" aria-hidden="true" />{{ item }}</ACheckbox></AListItem></template>
                    </AList>
                  </template>
                </template>
              </div>
              <ACollapse v-model:active-key="candidateEditorPanels" class="export-candidate-editor" :bordered="false">
                <ACollapsePanel key="add" header="手动添加候选对象">
                  <AForm layout="vertical">
                    <AFormItem label="添加候选对象" :validate-status="candidateObjectError ? 'error' : undefined" :help="candidateObjectError || '每行一个名称，也可用逗号分隔；添加后自动选中。'">
                      <ATextarea v-model:value="candidateObjectInput" aria-label="添加候选对象" :rows="2" autocomplete="off" :placeholder="objectType === 'VIEW' ? '例如 view_a' : '例如 orders'" />
                    </AFormItem>
                    <AButton :disabled="candidateObjectNames.length >= 100" @click="addCandidateObjects">添加并选中</AButton>
                  </AForm>
                </ACollapsePanel>
              </ACollapse>
            </section>
            <section class="export-object-pane export-object-selected" aria-label="已选导出对象">
              <div class="export-object-pane-heading"><h4>已选 {{ enteredObjectCount }} 项</h4><AButton type="link" :disabled="enteredObjectCount === 0" @click="clearObjectNameRows">清空</AButton></div>
              <AInput v-model:value="selectedObjectKeyword" aria-label="搜索已选对象" placeholder="搜索关键字" allow-clear :disabled="enteredObjectCount === 0"><template #suffix><SearchOutlined aria-hidden="true" /></template></AInput>
              <AEmpty v-if="enteredObjectCount === 0" description="尚未选择对象" />
              <AEmpty v-else-if="visibleSelectedObjectRows.length === 0" description="没有匹配的已选对象" />
              <template v-else>
                <div class="export-object-tree-category export-object-selected-category"><AButton type="text" class="export-object-expand" :aria-label="`${selectedGroupExpanded ? '收起' : '展开'}已选${objectType === 'VIEW' ? '视图' : '表'}`" @click="selectedGroupExpanded = !selectedGroupExpanded"><template #icon><CaretDownOutlined v-if="selectedGroupExpanded" aria-hidden="true" /><CaretRightOutlined v-else aria-hidden="true" /></template></AButton><component :is="objectType === 'VIEW' ? EyeOutlined : TableOutlined" class="export-object-kind-icon" aria-hidden="true" /><span>{{ objectType === 'VIEW' ? '视图' : '表' }}（{{ enteredObjectCount }}）</span><AButton type="text" class="export-object-delete" :aria-label="`清空已选${objectType === 'VIEW' ? '视图' : '表'}`" @click="clearObjectNameRows"><template #icon><DeleteOutlined aria-hidden="true" /></template></AButton></div>
                <AList v-if="selectedGroupExpanded" size="small" class="export-object-tree-children" :data-source="visibleSelectedObjectRows" aria-label="已选对象列表">
                  <template #renderItem="{ item }"><AListItem><component :is="objectType === 'VIEW' ? EyeOutlined : TableOutlined" class="export-object-kind-icon" aria-hidden="true" /><span>{{ item.name }}</span><AButton type="text" class="export-object-delete" :aria-label="`移除已选对象 ${item.name}`" @click="removeObjectNameRow(item.index)"><template #icon><DeleteOutlined aria-hidden="true" /></template></AButton></AListItem></template>
                </AList>
              </template>
            </section>
          </div>
        </div>
        <ACollapse class="export-advanced" :bordered="false">
          <ACollapsePanel key="advanced" header="高级设置 · DDL、对象与数据筛选">
            <AForm layout="vertical" class="export-advanced-form">
              <template v-if="contentKind !== 'DATA_ONLY'">
                <h3>DDL 与对象处理</h3>
                <AFormItem><ACheckbox v-model:checked="dropObject">前置 DROP（--drop-object）</ACheckbox></AFormItem>
                <p v-if="dropObject" class="section-hint">--drop-object 会在导入侧重建对象前删除同名对象，可能造成数据丢失，请确认已了解影响。</p>
                <AFormItem v-if="compactSchemaSupported"><ACheckbox v-model:checked="compactSchema">紧凑 Schema（--compact-schema，使用 show create table 检索文本）</ACheckbox></AFormItem>
                <AFormItem label="序列策略" :html-for="fieldPrefix + '-sequence-policy'" extra="--sequence-policy 待验证，当前保持关闭。"><ASelect :id="fieldPrefix + '-sequence-policy'" default-value="preserve（默认）" disabled><ASelectOption value="preserve（默认）">preserve（默认）</ASelectOption><ASelectOption value="restart">restart</ASelectOption></ASelect></AFormItem>
                <AFormItem><ACheckbox v-model:checked="retainSchema">保留 Schema（--retain-schema，保留 schema.table 前缀）</ACheckbox></AFormItem>
                <AFormItem extra="--add-extra-message 需当前 sys 权限预检查与秘密槽位绑定，当前保持关闭。"><ACheckbox disabled>附加对象信息</ACheckbox></AFormItem>
              </template>
              <h3 v-if="dataOptionsActive || excludeTablesSupported">对象排除与数据筛选</h3>
              <AFormItem v-if="excludeTablesSupported" label="排除表（可选）" extra="多个表名以逗号分隔。"><AInput v-model:value.trim="excludeTablesText" aria-label="排除表" autocomplete="off" placeholder="例如 tmp_a,tmp_b" /></AFormItem>
              <template v-if="dataOptionsActive">
                <p class="section-hint">以下筛选仅在包含数据时生效；服务端将复核参数互斥和对象资格。</p>
                <h3>列与类型筛选</h3>
                <AFormItem label="包含列" extra="多个列名以逗号分隔。"><AInput v-model:value.trim="includeColumnNames" aria-label="包含列" :disabled="Boolean(excludeColumnNames)" placeholder="例如 col_a,col_b" /></AFormItem>
                <AFormItem label="排除列" extra="多个列名以逗号分隔。"><AInput v-model:value.trim="excludeColumnNames" aria-label="排除列" :disabled="Boolean(includeColumnNames)" placeholder="例如 col_c" /></AFormItem>
                <AFormItem><ACheckbox v-model:checked="excludeVirtualColumns">排除生成列（--exclude-virtual-columns）</ACheckbox></AFormItem>
                <AFormItem label="排除数据类型"><AInput v-model:value.trim="excludeDataTypes" aria-label="排除数据类型" placeholder="例如 BLOB,TEXT" /></AFormItem>
                <h3>数据筛选</h3>
                <AFormItem label="自定义查询"><ATextarea v-model:value.trim="querySql" aria-label="自定义查询" :rows="2" :disabled="Boolean(where || partition || flashbackScn || flashbackTimestamp)" placeholder="仅允许固定 OBDUMPER 导出参数" /></AFormItem>
                <p class="section-hint">自定义查询只作为 OBDUMPER 的 --query-sql 参数；与条件、分区和闪回参数互斥。</p>
                <AFormItem v-if="whereSupported" label="条件筛选"><AInput v-model:value.trim="where" aria-label="条件筛选" :disabled="Boolean(querySql)" placeholder="例如 id &gt; 100" /></AFormItem>
                <AFormItem v-if="partitionSupported" label="分区筛选"><AInput v-model:value.trim="partition" aria-label="分区筛选" :disabled="Boolean(querySql)" placeholder="例如 p0,p2" /></AFormItem>
                <h3>一致性</h3>
                <AFormItem label="闪回 SCN"><AInput v-model:value.trim="flashbackScn" aria-label="闪回 SCN" :disabled="Boolean(querySql) || snapshot" placeholder="正整数" /></AFormItem>
                <AFormItem label="闪回时间点"><AInput v-model:value.trim="flashbackTimestamp" aria-label="闪回时间点" :disabled="Boolean(querySql) || snapshot" placeholder="例如 2026-08-06 00:00:00" /></AFormItem>
                <p class="section-hint">隐藏主键仍需对象、版本与权限预检查，当前不提供启用入口。</p>
              </template>
            </AForm>
          </ACollapsePanel>
        </ACollapse>
        <p class="section-hint">对象存在性和实际权限不在此页推断，仍由后续固定预检查确认。</p>
      </section>

      <section v-else-if="activeStep === 3" class="form-section">
        <h2>选择数据格式</h2>
        <AAlert v-if="contentKind === 'DDL_ONLY'" type="info" show-icon message="无数据格式" description="仅 DDL 导出不生成数据文件，因此不选择数据格式。" />
        <template v-else>
          <div class="export-field-group">
            <h3>数据文件设置</h3>
            <AForm layout="vertical" class="export-field-body">
              <AFormItem label="数据格式" required><ARadioGroup v-model:value="formatKind" role="radiogroup" aria-label="数据格式"><ARadio value="CSV">CSV</ARadio><ARadio value="CUT" :disabled="contentKind === 'DDL_AND_DATA'">CUT</ARadio><ARadio value="SQL" :disabled="contentKind === 'DDL_AND_DATA'">Insert SQL</ARadio></ARadioGroup></AFormItem>
              <p class="section-hint">普通新建入口提供 CSV、CUT、Insert SQL；仅 DDL 时不生成数据格式参数。</p>
              <AAlert v-if="!ordinaryFormat" type="warning" show-icon :message="draftValidationMessage" />
              <template v-if="ordinaryFormat">
                <div class="export-format-grid">
                  <AFormItem label="文件编码"><AInput v-model:value.trim="fileEncoding" aria-label="文件编码" placeholder="默认 UTF-8" /></AFormItem>
                  <AFormItem label="文件拆分（--block-size）" extra="正整数按 MB 拆分，也可使用 256ROW；留空继承官方默认。" :validate-status="attemptedStep === 3 && blockSize.trim() && !BLOCK_SIZE_PATTERN.test(blockSize.trim()) ? 'error' : undefined"><AInput v-model:value.trim="blockSize" aria-label="文件拆分" placeholder="例如 1024 或 256ROW" /></AFormItem>
                </div>
                <AFormItem class="export-common-choice" extra="导出最近一次合并版本快照；与高级设置中的闪回参数互斥。"><ACheckbox v-model:checked="snapshot" :disabled="Boolean((flashbackScn || flashbackTimestamp) && !snapshot)">一致性快照</ACheckbox></AFormItem>
                <div class="export-format-core">
                  <h3>{{ formatKind === 'SQL' ? 'Insert SQL 设置' : `${formatKind} 设置` }}</h3>
                  <div class="export-format-grid">
                    <AFormItem v-if="formatKind === 'CSV'" label="列分隔符"><AInput v-model:value.trim="columnSeparator" aria-label="列分隔符" placeholder="默认英文逗号；支持多字符" /></AFormItem>
                    <AFormItem v-else-if="formatKind === 'CUT'" label="列分隔字符串"><AInput v-model:value.trim="columnSplitter" aria-label="列分隔字符串" placeholder="例如 |" /></AFormItem>
                    <AFormItem label="行分隔符"><AInput v-model:value.trim="lineSeparator" aria-label="行分隔符" placeholder="按官方平台换行形式" /></AFormItem>
                  </div>
                  <AFormItem v-if="formatKind === 'CSV'" class="export-common-choice"><ACheckbox :checked="!skipHeader" @update:checked="skipHeader = !$event">包含列头</ACheckbox></AFormItem>
                  <p v-if="formatKind === 'SQL'" class="section-hint">Insert SQL 格式只支持行分隔符与文件编码；CSV/CUT 专属选项不适用。</p>
                </div>
              </template>
            </AForm>
          </div>
          <ACollapse class="export-advanced" :bordered="false">
            <ACollapsePanel key="advanced" header="高级设置 · 序列化、日期时间与压缩">
              <AForm layout="vertical" class="export-advanced-form">
                <h3>序列化</h3>
                <template v-if="formatKind === 'CSV'">
                  <AFormItem label="列包围符"><AInput v-model:value.trim="columnQuote" aria-label="列包围符" placeholder="默认英文单引号" /></AFormItem>
                  <AFormItem label="包围模式" :html-for="fieldPrefix + '-quote-mode'"><ASelect :id="fieldPrefix + '-quote-mode'" v-model:value="columnQuoteMode"><ASelectOption value="">继承官方默认</ASelectOption><ASelectOption value="all">all</ASelectOption><ASelectOption value="all_not_null">all_not_null</ASelectOption><ASelectOption value="minimal">minimal</ASelectOption><ASelectOption value="non_numeric">non_numeric</ASelectOption><ASelectOption value="none">none</ASelectOption></ASelect></AFormItem>
                  <p class="section-hint">包围模式：all 全部包围、all_not_null 非空包围、minimal 最小包围、non_numeric 非数字包围、none 不包围。</p>
                </template>
                <AFormItem v-if="formatKind === 'CUT'"><ACheckbox v-model:checked="trailDelimiter">行尾追加分隔符（--trail-delimiter）</ACheckbox></AFormItem>
                <AFormItem v-if="formatKind === 'CSV' || formatKind === 'CUT'" label="转义字符"><AInput v-model:value.trim="escapeCharacter" aria-label="转义字符" placeholder="仅支持单字符" /></AFormItem>
                <AFormItem v-if="formatKind === 'CSV' || formatKind === 'CUT'" label="NULL 替换"><AInput v-model:value.trim="nullString" aria-label="NULL 替换" placeholder="默认 \N" /></AFormItem>
                <AFormItem v-if="formatKind === 'CSV' || formatKind === 'CUT'"><ACheckbox v-model:checked="withTrim">去除左右空格（--with-trim）</ACheckbox></AFormItem>
                <AFormItem v-if="formatKind === 'CUT'"><ACheckbox v-model:checked="removeNewline">删除换行（高风险，--remove-newline）</ACheckbox></AFormItem>
                <p v-if="removeNewline" class="section-hint">--remove-newline 会删除导出数据中的换行并改变内容。</p>
                <h3>日期时间</h3>
                <template v-if="timestampFormatsSupported">
                  <AFormItem label="DATETIME 值格式"><AInput v-model:value.trim="datetimeValueFormat" aria-label="DATETIME 值格式" placeholder="例如 yyyy-MM-dd HH:mm:ss" /></AFormItem>
                  <AFormItem label="DATE 值格式"><AInput v-model:value.trim="dateValueFormat" aria-label="DATE 值格式" placeholder="例如 yyyy-MM-dd" /></AFormItem>
                </template>
                <p v-else class="section-hint">仅 MySQL CSV/CUT 数据导出支持已验证的 DATE 与 DATETIME 值格式。</p>
                <h3>压缩</h3>
                <AFormItem><ACheckbox v-model:checked="compress">启用压缩（--compress）</ACheckbox></AFormItem>
                <AFormItem label="压缩算法" :html-for="fieldPrefix + '-compression-algo'"><ASelect :id="fieldPrefix + '-compression-algo'" v-model:value="compressionAlgo" :disabled="!compress"><ASelectOption value="">继承官方默认（zstd）</ASelectOption><ASelectOption value="zstd">zstd</ASelectOption><ASelectOption value="zlib">zlib</ASelectOption><ASelectOption value="gzip">gzip</ASelectOption><ASelectOption value="snappy">snappy</ASelectOption></ASelect></AFormItem>
                <AFormItem v-if="compressionAlgo !== 'gzip' && compressionAlgo !== 'snappy'" label="压缩等级"><AInput v-model:value.trim="compressionLevel" aria-label="压缩等级" :disabled="!compress" :placeholder="compressionAlgo === 'zlib' ? '例如 5（zlib 支持 -1~9）' : '例如 3（zstd 支持 1~22）'" /></AFormItem>
                <p class="section-hint">压缩等级按算法分范围：zstd 1~22、zlib -1~9；gzip/snappy 不支持指定等级。</p>
              </AForm>
            </ACollapsePanel>
          </ACollapse>
        </template>
      </section>

      <section v-else-if="activeStep === 4" class="form-section">
        <h2>执行与输出配置</h2>
        <AAlert v-if="draftNotice" type="info" show-icon :message="draftNotice" />
        <p>填写已选执行节点上的完整输出路径。路径资格、目录空性、空间和工具环境由后续预检查确认。</p>
        <AForm layout="vertical" class="export-form">
          <AFormItem label="执行节点" required :validate-status="attemptedStep === 4 && !selectedNodeID ? 'error' : undefined" :help="attemptedStep === 4 && !selectedNodeID ? '请返回导出内容与对象选择执行节点。' : '已与数据库和对象目录绑定；节点在线及工具可用性由预检查确认。'">
            <AAlert v-if="nodeLoadFailure" type="error" show-icon :message="nodeLoadFailure"><template #action><AButton type="link" @click="loadNodeCandidates">重试</AButton></template></AAlert>
            <ASkeleton v-else-if="loadingNodes" active :paragraph="{ rows: 1 }" aria-label="正在加载已授权执行节点" />
            <template v-else>
              <span>{{ selectedNode ? `${selectedNode.displayName} · ${selectedNode.platform}` : '尚未选择' }}</span>
              <AButton v-if="!derivedDraftBindingLocked" type="link" @click="moveToStep(2)">返回内容与对象修改节点</AButton>
            </template>
          </AFormItem>
          <AAlert v-if="selectedNode && derivedDraftBindingLocked" type="info" show-icon message="派生草稿固定使用来源任务的执行节点；节点状态仍由预检查确认。" />
          <template v-if="outputKind === 'LOCAL'">
            <AFormItem label="导出路径" required :validate-status="attemptedStep === 4 && !filePath.trim() ? 'error' : undefined" :help="attemptedStep === 4 && !filePath.trim() ? '请填写与节点平台匹配的完整绝对路径。' : '平台原样传递此路径，不追加目录或转换平台格式。'">
              <AInput v-model:value.trim="filePath" aria-label="导出路径" :placeholder="outputPathPlaceholder" autocomplete="off" />
            </AFormItem>
          </template>
          <AAlert v-if="outputKind !== 'LOCAL'" type="warning" show-icon message="对象存储输出仍需端点连通性与凭据有效性预检查；未取得通过证据时不能提交。" />
        </AForm>
        <ACollapse class="export-advanced" :bordered="false" :default-active-key="outputKind === 'LOCAL' ? [] : ['advanced']">
          <ACollapsePanel key="advanced" header="高级设置 · 文件布局、性能与对象存储">
            <AForm layout="vertical" class="export-advanced-form">
              <h3>文件布局与生成</h3>
              <AFormItem label="日志路径（可选）" extra="留空时继承 OBDUMPER 默认日志目录。"><AInput v-model:value.trim="logPath" aria-label="日志路径" :placeholder="outputPathPlaceholder" autocomplete="off" /></AFormItem>
              <AFormItem v-if="outputKind === 'LOCAL'"><ACheckbox v-model:checked="skipCheckDir">跳过导出目录空性检查（--skip-check-dir）</ACheckbox></AFormItem>
              <AAlert v-if="skipCheckDir && outputKind === 'LOCAL'" type="warning" show-icon message="可能覆盖同名文件；路径可写性与可用空间仍会检查。" />
              <AFormItem><ACheckbox v-model:checked="noNestedDir">扁平目录（--no-nested-dir）</ACheckbox></AFormItem>
              <AFormItem v-if="dataOptionsActive" label="导出总量上限（Byte）"><AInput v-model:value.trim="maxFileSize" aria-label="导出总量上限" placeholder="正整数，例如 1048576" /></AFormItem>
              <AFormItem v-if="dataOptionsActive"><ACheckbox v-model:checked="retainEmptyFiles">保留空结果文件（--retain-empty-files）</ACheckbox></AFormItem>
              <p class="section-hint">保留空结果文件适用于空表，以及筛选后的空分区或空结果；同时省略 CSV 表头时会生成空文件。</p>
              <h3>性能与资源</h3>
              <div v-if="dataOptionsActive" class="export-field-grid">
                <AFormItem label="导出线程"><AInput v-model:value.trim="thread" aria-label="导出线程" placeholder="继承官方默认" /></AFormItem>
                <AFormItem label="分页大小"><AInput v-model:value.trim="pageSize" aria-label="分页大小" placeholder="继承 1,000,000" /></AFormItem>
                <AFormItem label="每线程宏块数"><AInput v-model:value.trim="parallelMacro" aria-label="每线程宏块数" placeholder="继承 8" /></AFormItem>
                <AFormItem label="游标抓取行数（Oracle）"><AInput v-model:value.trim="fetchSize" aria-label="游标抓取行数" placeholder="继承 1000" /></AFormItem>
                <AFormItem label="JVM 内存"><AInput v-model:value.trim="jvmMemory" aria-label="JVM 内存" placeholder="例如 4G" /></AFormItem>
              </div>
              <h3>对象存储（当前门控）</h3>
              <AAlert type="info" show-icon message="OSS、S3、COS、OBS 按受控 URI 配置；凭据只使用安全槽位。未授权的真实探测保持未完成。" />
              <AFormItem label="输出类型">
                <ARadioGroup v-model:value="outputKind" role="radiogroup" aria-label="输出类型"><ARadio value="LOCAL">本地路径</ARadio><ARadio value="OSS">OSS</ARadio><ARadio value="S3">S3</ARadio><ARadio value="COS">COS</ARadio><ARadio value="OBS">OBS</ARadio></ARadioGroup>
              </AFormItem>
              <template v-if="outputKind !== 'LOCAL'">
                <div class="export-field-grid">
                  <AFormItem label="Bucket" required><AInput v-model:value.trim="storageBucket" aria-label="Bucket" autocomplete="off" /></AFormItem>
                  <AFormItem label="对象路径" required><AInput v-model:value.trim="storagePath" aria-label="对象路径" placeholder="/exports/daily" autocomplete="off" /></AFormItem>
                  <AFormItem label="Endpoint"><AInput v-model:value.trim="storageEndpoint" aria-label="Endpoint" autocomplete="off" /></AFormItem>
                  <AFormItem label="Region"><AInput v-model:value.trim="storageRegion" aria-label="Region" autocomplete="off" /></AFormItem>
                </div>
                <AFormItem label="存储凭据（可选）" :html-for="fieldPrefix + '-storage-credential'" extra="只保存凭据标识与修订，不在页面读取密钥。">
                  <AAlert v-if="storageCredentialLoadFailure" type="error" show-icon :message="storageCredentialLoadFailure"><template #action><AButton type="link" @click="loadStorageCredentials">重试</AButton></template></AAlert>
                  <ASkeleton v-else-if="loadingStorageCredentials" active :paragraph="{ rows: 1 }" aria-label="正在加载存储凭据" />
                  <ASelect v-else :id="fieldPrefix + '-storage-credential'" v-model:value="storageCredentialID"><ASelectOption value="">不指定（依赖执行节点 Hadoop 配置）</ASelectOption><ASelectOption v-for="credential in matchingStorageCredentials" :key="credential.id" :value="credential.id">{{ credential.displayName }} · 修订 {{ credential.currentRevision }}</ASelectOption></ASelect>
                </AFormItem>
                <AFormItem label="本地临时分块目录（可选）" extra="Multipart 上传的本地分块目录；留空则继承官方默认。"><AInput v-model:value.trim="tmpPath" aria-label="本地临时分块目录" :placeholder="outputPathPlaceholder" autocomplete="off" /></AFormItem>
              </template>
            </AForm>
          </ACollapsePanel>
        </ACollapse>
        <AAlert v-if="draftFailure" type="error" show-icon :message="draftFailure" />
      </section>

      <section v-else class="form-section">
        <h2>参数预检查与完整命令</h2>
        <AAlert v-if="draftNotice" :type="activePrecheck?.status === 'FAILED' ? 'error' : 'info'" show-icon :message="draftNotice" />
        <ASpin v-if="loadingDraft" tip="正在读取已创建草稿的服务端配置快照…"><span class="export-loading-space" /></ASpin>
        <AAlert v-if="draftLoadFailure" type="error" show-icon :message="draftLoadFailure"><template #action><AButton type="link" @click="loadCreatedDraft()">重新读取草稿</AButton></template></AAlert>
        <section class="configuration-summary">
          <h3>任务配置摘要</h3>
          <ADescriptions class="export-facts" size="small" :column="1">
            <ADescriptionsItem label="数据源">{{ displayedSource?.displayName ?? (currentDraft ? '草稿数据源当前不可用' : '尚未读取草稿') }}</ADescriptionsItem>
            <ADescriptionsItem label="对象与内容">{{ displayedDraftConfig ? `${draftScopeSummary} · ${draftContentLabel}` : '尚未读取草稿' }}</ADescriptionsItem>
            <ADescriptionsItem label="导出、日志与节点">{{ displayedDraftConfig && displayedNode ? `${displayedNode.displayName} · 导出：${displayedDraftConfig.outputConfig.filePath}${displayedDraftConfig.outputConfig.logPath ? ` · 日志：${displayedDraftConfig.outputConfig.logPath}` : ''}${displayedDraftConfig.outputConfig.skipCheckDir ? ' · 已跳过目录空性检查' : ''}` : '尚未读取草稿' }}</ADescriptionsItem>
          </ADescriptions>
        </section>
        <section class="precheck-list">
          <h3>预检查结果</h3>
          <p class="section-hint">预检查由已选择的 Agent 执行：确认数据源连接、当前草稿所选对象可读取（全部范围按数据库级可达性投影）、OB Loader/Dumper 与专用 Java 8 配置、导出目录及已填写日志目录可写、导出目录空性，以及至少 1 GiB 可用空间。对象存储输出额外检查端点连通性与凭据有效性（未授权探测保持未完成）。勾选跳过选项时，仅目录空性检查会被跳过。它不会启动 OBDUMPER 或创建导出文件。</p>
          <AAlert v-if="storageOutput" type="warning" show-icon message="对象存储的端点连通性与凭据有效性检查必须通过才能提交；未授权探测保持未完成。" />
          <AAlert v-if="precheckFailure" type="error" show-icon :message="precheckFailure" />
          <ASpin v-if="precheckRunning" tip="预检查进行中…"><span class="export-loading-space" /></ASpin>
          <p v-if="activePrecheck" role="status">当前状态：<ATag :color="activePrecheck.status === 'FAILED' ? 'error' : activePrecheck.status === 'SUCCEEDED' ? 'success' : 'processing'">{{ precheckStatusLabel(activePrecheck.status) }}</ATag></p>
          <AAlert v-if="activePrecheck?.status === 'FAILED'" type="error" show-icon message="预检查未通过" :description="`以下 ${blockingPrecheckRows.length} 项检查未通过或未完成。请修正后重新执行预检查。`" />
          <AList v-if="activePrecheck?.status === 'FAILED'" size="small" :data-source="blockingPrecheckRows" aria-label="预检查阻断原因">
            <template #renderItem="{ item: row }"><AListItem><div class="export-precheck-detail"><strong>{{ precheckCheckLabel(row.check) }}</strong><span>{{ precheckResultDetail(row.result) }}</span><code>原因码：{{ row.result?.evidenceCode }}</code></div></AListItem></template>
          </AList>
          <AList size="small" :data-source="precheckRows" aria-label="预检查结果">
            <template #renderItem="{ item: row }">
              <AListItem><div class="export-precheck-row"><ABadge :status="!row.result ? 'default' : row.result.status === 'PASSED' ? 'success' : row.result.status === 'FAILED' ? 'error' : 'default'" /><strong>{{ precheckCheckLabel(row.check) }}</strong><span><b>{{ precheckResultLabel(row.result, precheckRunning) }}</b><small v-if="precheckResultDetail(row.result)">{{ precheckResultDetail(row.result) }}</small><code v-if="precheckResultBlocksSubmission(row.result)">原因码：{{ row.result?.evidenceCode }}</code></span></div></AListItem>
            </template>
          </AList>
        </section>
        <section class="command-empty">
          <div><h3>完整命令（仅隐藏密码）</h3><AButton :disabled="!commandPreview" @click="copyCommand">复制脱敏命令</AButton></div>
          <ASpin v-if="previewingCommand" tip="控制面正在重算命令预览…"><span class="export-loading-space" /></ASpin>
          <AAlert v-else-if="commandPreviewFailure" type="error" show-icon :message="commandPreviewFailure"><template #action><AButton type="link" @click="loadCommandPreview()">重新生成命令</AButton></template></AAlert>
          <pre v-else-if="commandPreview"><code>{{ commandPreview.command }}</code></pre>
          <pre v-else><code>命令只会由控制面根据已读取的草稿快照生成；密码始终不会显示或由浏览器自行拼接。</code></pre>
          <p v-if="commandPreview">`-p ******` 仅为密码占位；实际运行从官方安全文件读取密码，不把密码放入进程参数。</p>
          <p v-if="commandPreview">草稿版本 rev-{{ currentDraft?.revision }} · 配置指纹 {{ commandPreview.configFingerprint }}</p>
          <AAlert v-if="copyNotice" type="success" show-icon :message="copyNotice" />
        </section>
        <AAlert v-if="submissionFailure" type="error" show-icon :message="submissionFailure" />
      </section>
    </template>

    <template #summary>
      <h2>配置总览</h2>
      <ADescriptions class="export-facts" size="small" :column="1">
        <ADescriptionsItem label="当前步骤">{{ titles[activeStep - 1] }}</ADescriptionsItem>
        <ADescriptionsItem label="数据源">{{ displayedSource?.displayName ?? '尚未选择' }}</ADescriptionsItem>
        <ADescriptionsItem label="导出范围">{{ displayedDraftConfig ? draftScopeSummary : (database ? `${database} · ${scopeKind === 'ALL' ? '全部对象' : '指定对象'}` : '尚未配置') }}</ADescriptionsItem>
        <ADescriptionsItem label="导出内容">{{ contentKind === 'DDL_ONLY' ? '仅 DDL' : contentKind === 'DDL_AND_DATA' ? 'DDL + 数据' : '仅数据' }}</ADescriptionsItem>
        <ADescriptionsItem label="数据格式">{{ contentKind === 'DDL_ONLY' ? '无数据格式' : formatKind }}</ADescriptionsItem>
        <ADescriptionsItem label="执行节点">{{ displayedNode?.displayName ?? '尚未选择' }}</ADescriptionsItem>
        <ADescriptionsItem label="预检查">{{ precheckStatusLabel(activePrecheck?.status) }}</ADescriptionsItem>
      </ADescriptions>
      <p class="aside-note">草稿创建和命令预览不启动 Agent、工具或数据库连接。预检查通过后，点击“提交并启动导出”才会由 Agent 领取冻结任务并启动 OBDUMPER。</p>
    </template>

    <template #footer>
      <footer class="wizard-footer">
        <AButton :disabled="activeStep === 1 || creatingDraft" @click="previousStep">上一步</AButton>
        <span class="wizard-baseline-note">{{ footerBaselineNote }}</span>
        <span class="footer-grow" />
        <AButton v-if="activeStep < 5" :loading="creatingDraft" :disabled="loadingSources || loadingNodes && activeStep === 4 || activeStep === 2 && !canAdvance" type="primary" @click="nextStep">{{ footerLabel }}</AButton>
        <div v-else class="footer-actions">
          <AButton :disabled="!currentDraft || draftDirty || !commandPreview || startingPrecheck || precheckRunning || submitting" @click="startPrecheck">{{ footerLabel }}</AButton>
          <AButton :disabled="!canSubmit" type="primary" @click="submitTask">{{ submitting ? '正在提交任务…' : '提交并启动导出' }}</AButton>
        </div>
      </footer>
    </template>
  </WizardFrame>
  <AModal :open="submitConfirmationOpen" title="确认提交导出任务" :closable="!submitting" :mask-closable="!submitting" :keyboard="!submitting" @cancel="closeSubmitConfirmation">
    <div class="export-submit-facts">
      <p>提交后配置冻结，所选 Agent 领取任务并启动 OBDUMPER；已提交任务不能原地修改参数。</p>
      <ADescriptions class="export-facts" size="small" :column="1"><ADescriptionsItem label="数据源">{{ displayedSource?.displayName ?? '当前不可用' }}</ADescriptionsItem><ADescriptionsItem label="导出范围">{{ draftScopeSummary }}</ADescriptionsItem><ADescriptionsItem label="输出位置">{{ displayedDraftConfig?.outputConfig.filePath ?? '未读取' }}</ADescriptionsItem></ADescriptions>
      <AAlert v-if="displayedSource?.environment === 'PRODUCTION' || dropObject || removeNewline" type="warning" show-icon message="请确认生产环境与高风险参数的影响。" :description="[displayedSource?.environment === 'PRODUCTION' ? '生产数据源' : '', dropObject ? 'DDL 包含前置 DROP' : '', removeNewline ? '删除导出数据中的换行' : ''].filter(Boolean).join('；')" />
      <AAlert v-if="submissionFailure" type="error" show-icon :message="submissionFailure" />
    </div>
    <template #footer><AButton :id="submitCancelID" :disabled="submitting" @click="closeSubmitConfirmation">返回检查</AButton><AButton type="primary" :loading="submitting" @click="confirmSubmitTask">确认提交并启动导出</AButton></template>
  </AModal>
</template>

<style scoped>
.export-source-filter {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ob-foundation-space-3);
  margin-block: var(--ob-foundation-space-4);
}
.export-source-filter > :first-child { flex: 1 1 240px; min-inline-size: 0; }
.export-source-filter > :last-child { inline-size: 180px; }
.export-source-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr)); gap: var(--ob-foundation-space-2); inline-size: 100%; margin-block: var(--ob-foundation-space-4); }
.export-source-choice { display: flex; align-items: flex-start; min-inline-size: 0; margin-inline-end: 0; padding: var(--ob-foundation-space-3); border: 1px solid var(--ob-color-border); border-radius: var(--ob-component-control-radius); background: var(--ob-color-surface); }
.export-source-facts { display: grid; min-inline-size: 0; gap: var(--ob-foundation-space-1); overflow-wrap: anywhere; }
.export-source-facts span { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.export-field-group { margin-block: var(--ob-foundation-space-4); }
.export-object-group { container-type: inline-size; }
.export-content-form { margin-block-start: var(--ob-foundation-space-4); }
.export-content-options { display: flex; flex-wrap: wrap; max-inline-size: 100%; }
.export-field-group h3,
.export-advanced h3 {
  margin: var(--ob-foundation-space-4) 0 var(--ob-foundation-space-2);
  color: var(--ob-color-form-text);
  font-size: var(--ob-product-typography-section-size);
  font-weight: var(--ob-product-typography-weight);
}
.export-field-group h3:first-child,
.export-advanced h3:first-child { margin-block-start: 0; }
.export-field-body { padding-block: var(--ob-foundation-space-2); }
.export-database-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); max-inline-size: 740px; gap: var(--ob-foundation-space-4); }
.export-database-grid :deep(.ant-form-item) { min-inline-size: 0; }
.export-group-heading { display: flex; align-items: baseline; justify-content: space-between; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.export-group-heading h3 { margin-block-end: 0; }
.export-object-count { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.export-object-workspace { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); margin-block-start: var(--ob-foundation-space-3); overflow: hidden; border: 1px solid var(--ob-color-border); border-radius: var(--ob-component-control-radius); }
.export-object-pane { min-inline-size: 0; min-block-size: 360px; padding: 0 var(--ob-foundation-space-3) var(--ob-foundation-space-3); }
.export-object-pane h4 { margin: 0; color: var(--ob-color-form-text); font-size: var(--ob-product-typography-section-size); font-weight: var(--ob-product-typography-weight); }
.export-object-selected { border-inline-start: 1px solid var(--ob-color-border); background: var(--ob-color-subtle); }
.export-object-pane-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--ob-foundation-space-2); min-block-size: 44px; margin-inline: calc(-1 * var(--ob-foundation-space-3)); margin-block-end: var(--ob-foundation-space-3); padding-inline: var(--ob-foundation-space-3); border-block-end: 1px solid var(--ob-color-border); }
.export-candidate-editor { margin-block-start: var(--ob-foundation-space-4); }
.export-object-search { display: flex; gap: 0; }
.export-object-search :deep(.ant-input-affix-wrapper) { flex: 1; min-inline-size: 0; }
.export-object-search :deep(.ant-btn) { border-start-start-radius: 0; border-end-start-radius: 0; }
.export-object-search :deep(.ant-input-affix-wrapper) { border-start-end-radius: 0; border-end-end-radius: 0; }
.export-candidate-editor :deep(.ant-collapse-header) { padding-inline: 0; }
.export-candidate-editor :deep(.ant-collapse-content-box) { padding-inline: 0; }
.export-object-tree { margin-block-start: var(--ob-foundation-space-3); }
.export-object-tree-category { display: flex; align-items: center; gap: var(--ob-foundation-space-2); min-block-size: 29px; min-inline-size: 0; }
.export-object-expand { flex: none; inline-size: 20px; min-inline-size: 20px; padding: 0; }
.export-object-expand-placeholder { flex: none; inline-size: 20px; }
.export-object-kind-icon { flex: none; color: var(--ob-color-secondary); }
.export-object-category-name { min-inline-size: 0; padding: 0; text-align: start; color: var(--ob-color-form-text); }
.export-object-category-name.is-disabled, .export-object-kind-note { color: var(--ob-color-secondary); }
.export-object-kind-note { margin-inline-start: auto; font-size: var(--ob-component-field-helper-size); }
.export-object-tree-children { margin-inline-start: 36px; }
.export-object-tree-children :deep(.ant-list-item) { min-block-size: 28px; padding-block: 2px; border-block-end: 0; }
.export-object-tree-children :deep(.ant-checkbox-wrapper) { display: flex; align-items: center; gap: var(--ob-foundation-space-2); }
.export-object-tree-children :deep(.ant-checkbox-wrapper > span:last-child) { display: inline-flex; align-items: center; gap: var(--ob-foundation-space-2); min-inline-size: 0; }
.export-object-tree-empty :deep(.ant-empty-description) { font-size: var(--ob-component-field-helper-size); }
.export-object-pane :deep(.ant-checkbox-wrapper) { min-inline-size: 0; overflow-wrap: anywhere; }
.export-object-pane :deep(.ant-list-item) { min-inline-size: 0; }
.export-object-selected > .ant-input-affix-wrapper { margin-block-end: var(--ob-foundation-space-3); }
.export-object-selected-category { margin-block-start: var(--ob-foundation-space-2); }
.export-object-delete { margin-inline-start: auto; padding-inline: var(--ob-foundation-space-1); color: var(--ob-color-secondary); }
.export-object-selected :deep(.ant-list-item) { display: flex; align-items: center; gap: var(--ob-foundation-space-2); }
.export-object-selected :deep(.ant-list-item > span) { min-inline-size: 0; overflow-wrap: anywhere; }
.export-form { display: grid; max-inline-size: 720px; gap: var(--ob-foundation-space-3); margin-block: var(--ob-foundation-space-4); }
.export-advanced-form { max-inline-size: 720px; margin-block: var(--ob-foundation-space-3); }
.export-format-core { margin-block-start: var(--ob-foundation-space-4); padding: var(--ob-foundation-space-4); background: var(--ob-color-subtle); border: 1px solid var(--ob-color-border); border-radius: var(--ob-component-control-radius); }
.export-format-core h3 { margin-block-start: 0; }
.export-format-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr)); gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); }
.export-format-grid :deep(.ant-form-item) { margin-block-end: 0; }
.export-field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); }
@container (max-width: 800px) { .export-database-grid, .export-object-workspace { grid-template-columns: minmax(0, 1fr); } .export-object-selected { border-inline-start: 0; border-block-start: 1px solid var(--ob-color-border); } }
.export-advanced { margin-block-start: var(--ob-foundation-space-4); }
.export-step-error { margin-block-end: var(--ob-foundation-space-4); }
.export-loading-space { display: block; min-block-size: var(--ob-foundation-space-8); }
.export-facts { min-inline-size: 0; overflow-wrap: anywhere; }
.export-precheck-row { display: grid; grid-template-columns: 16px minmax(140px, 200px) minmax(0, 1fr); align-items: start; gap: var(--ob-foundation-space-3); inline-size: 100%; min-inline-size: 0; overflow-wrap: anywhere; }
.export-precheck-row small, .export-precheck-row code { display: block; margin-block-start: var(--ob-foundation-space-1); }
.export-precheck-detail { display: grid; gap: var(--ob-foundation-space-1); min-inline-size: 0; overflow-wrap: anywhere; }
.export-builder :deep(.ant-select) { min-inline-size: 0; }
@media (max-width: 800px) {
  .export-field-grid { grid-template-columns: minmax(0, 1fr); }
  .export-precheck-row { grid-template-columns: 16px minmax(0, 1fr); }
  .export-precheck-row > :last-child { grid-column: 2; }
  .export-source-filter > :last-child { flex: 1 1 180px; }
}
</style>
