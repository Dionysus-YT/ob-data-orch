import { ref, reactive, computed } from 'vue'
import { type ExportScopeKind, type ExportObjectType, type ExportContentKind, type ExportDataFormatKind, type ExportOutputKind, type CsvQuoteMode, type CompressionAlgo } from '@/api/browser'
import { objectCategories } from './exportPresentation'

// createExportForm 管理当前向导唯一表单状态。
export function createExportForm() {
  const selectedDataSourceID = ref('')
  const selectedNodeID = ref('')

  const database = ref('')
  const scopeKind = ref<ExportScopeKind>('SPECIFIED')

  const objectType = ref<ExportObjectType>('TABLE')
  const selectedObjectsByType = reactive<Record<ExportObjectType, string[]>>({ TABLE: [], VIEW: [], FUNCTION: [], PROCEDURE: [], SEQUENCE: [] })

  const objectNames = computed({ get: () => selectedObjectsByType[objectType.value], set: (names: string[]) => { selectedObjectsByType[objectType.value] = names.filter(Boolean) } })
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

  const splitUnit = ref<'MB' | 'ROW'>('MB')
  const withTrim = ref(false)

  const noNestedDir = ref(false)
  const maxFileSize = ref('')

  const retainEmptyFiles = ref(false)
  const compress = ref(false)

  const compressionAlgo = ref<CompressionAlgo | ''>('')

  // EX-I7 压缩等级（2026-08-10）：--compression-level，按所选算法分范围。
  const compressionLevel = ref('')
  const querySql = ref('')

  const queryResultLimit = ref('')

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
  const allSelectedObjects = computed(() => objectCategories.flatMap((category) => selectedObjectsByType[category.type].map((name, index) => ({ type: category.type, name: name.trim(), index })).filter((item) => item.name)))

  const applicableSelectedObjects = computed(() => contentKind.value === 'DATA_ONLY' ? allSelectedObjects.value.filter((item) => item.type === 'TABLE') : allSelectedObjects.value)
  const enteredObjectCount = computed(() => applicableSelectedObjects.value.length)

  function clearSelectedObjects() { for (const category of objectCategories) selectedObjectsByType[category.type] = [] }
  const hydratingDerivedDraft = ref(false)

  // 配置字段集中登记，失效监听直接消费这些引用，新增字段无需另维护一份监听清单。
  const fields = {
    storageCredentialID, selectedDataSourceID, selectedNodeID, database, scopeKind, objectType,
    selectedObjectsByType, excludeTablesText, contentKind, formatKind, trailDelimiter, removeNewline,
    columnSplitter, filePath, logPath, skipCheckDir, outputKind, storageBucket, storagePath,
    storageEndpoint, storageRegion, tmpPath, controlFilePath, skipHeader, columnSeparator, columnQuote,
    columnQuoteMode, escapeCharacter, lineSeparator, nullString, fileEncoding, withTrim, noNestedDir,
    maxFileSize, retainEmptyFiles, compress, compressionAlgo, compressionLevel, querySql, queryResultLimit,
    where, includeColumnNames, excludeColumnNames, excludeVirtualColumns, flashbackScn, flashbackTimestamp,
    snapshot, thread, pageSize, parallelMacro, fetchSize, jvmMemory, blockSize, dropObject, retainSchema,
    compactSchema, addExtraMessage, partition, excludeDataTypes, enableHiddenPk, dateValueFormat,
    timeValueFormat, datetimeValueFormat, timestampValueFormat, timestampTzValueFormat,
    timestampLtzValueFormat, nlsDateFormat, nlsTimestampFormat, nlsTimestampTzFormat,
  }
  return {
    ...fields, fields, objectNames, allSelectedObjects, applicableSelectedObjects, enteredObjectCount,
    clearSelectedObjects, hydratingDerivedDraft, splitUnit,
  }
}

export type ExportForm = ReturnType<typeof createExportForm>
