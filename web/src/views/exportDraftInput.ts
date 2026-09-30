import { isStructuredFormat } from '@/api/browser'
import type { CompressionAlgo, CsvOptions, CsvQuoteMode, CutOptions, DDLBehaviorOptions, ExportContentKind, ExportDataFormatKind, ExportDraftInput, ExportObjectType, ExportOutputKind, ExportScopeKind, FilterOptions, GeneralizedExportConfig, PerformanceOptions, StorageCredentialReference, TimestampFormatsOptions } from '@/api/browser'

// ExportDraftFormValues 是导出向导对象、内容、格式与输出步骤的页面状态。
// 校验通过后构造当前泛化草稿请求；服务端仍会按能力矩阵再次失败关闭。
export interface ExportDraftFormValues {
  readonly dataSourceId: string
  readonly nodeId: string
  readonly platform: string
  // compatibilityMode 来自已选数据源；未知值按不支持处理，防止未验证模式绕过时间格式门禁。
  readonly compatibilityMode: string
  readonly database: string
  readonly scopeKind: ExportScopeKind
  readonly objectType: ExportObjectType
  readonly objectNames: readonly string[]
  readonly excludeTables: readonly string[]
  readonly contentKind: ExportContentKind
  // EX-I4：数据格式单选；CSV/CUT/SQL 各有适用面板。
  readonly formatKind: ExportDataFormatKind
  readonly trailDelimiter: boolean
  readonly removeNewline: boolean
  // EX-I4 POS 定版后解锁：CUT 列分隔字符串（--column-splitter）。
  readonly columnSplitter: string
  readonly filePath: string
  readonly logPath: string
  readonly skipCheckDir: boolean
  // EX-I6 对象存储（2026-08-07）：输出类型与受控对象存储字段（凭据走执行槽位，不在表单收集）。
  readonly outputKind: ExportOutputKind
  readonly storageBucket: string
  readonly storagePath: string
  readonly storageEndpoint: string
  readonly storageRegion: string
  readonly tmpPath: string
  // EX-I6 存储凭据槽位（2026-08-14）：对象存储输出的凭据引用（可缺省）；
  // storageCredentialRevision 取凭据当前修订，storageCredentialProvider 用于本地一致性校验。
  readonly storageCredentialId: string
  readonly storageCredentialRevision: number
  readonly storageCredentialProvider: string
  // EX-I4 POS 定版（2026-08-07 实测）：控制文件目录（--ctl-path），仅 POS 格式必填。
  readonly controlFilePath: string
  // EX-I3 选项：CSV 序列化、压缩、文件布局、筛选与资源参数。
  readonly skipHeader: boolean
  readonly columnSeparator: string
  readonly columnQuote: string
  readonly columnQuoteMode: CsvQuoteMode | ''
  readonly escapeCharacter: string
  readonly lineSeparator: string
  readonly nullString: string
  readonly fileEncoding: string
  readonly withTrim: boolean
  readonly noNestedDir: boolean
  readonly maxFileSize: string
  readonly retainEmptyFiles: boolean
  readonly compress: boolean
  readonly compressionAlgo: CompressionAlgo | ''
  // EX-I7 压缩等级（2026-08-10）：--compression-level，按所选算法分范围。
  readonly compressionLevel: string
  readonly querySql: string
  // 条件筛选仅适用于指定表，并与 querySql 互斥。
  readonly where: string
  readonly includeColumnNames: string
  readonly excludeColumnNames: string
  readonly excludeVirtualColumns: boolean
  readonly flashbackScn: string
  readonly flashbackTimestamp: string
  // 一致性快照不能与任一闪回参数共同使用。
  readonly snapshot: boolean
  readonly thread: string
  readonly pageSize: string
  readonly parallelMacro: string
  readonly fetchSize: string
  readonly jvmMemory: string
  // EX-I7 文件拆分（2026-08-10）：--block-size，正整数（MB）或正整数+MB/ROW 后缀。
  readonly blockSize: string
  // 前置 DROP 与保留 Schema 仅在 DDL 内容时生效；紧凑 Schema 还必须包含表 DDL。
  readonly dropObject: boolean
  readonly retainSchema: boolean
  readonly compactSchema: boolean
  // --add-extra-message 仍需 sys 权限预检查，浏览器保持关闭并拒绝提交。
  readonly addExtraMessage: boolean
  // 分区筛选仅指定表数据，类型排除仅数据内容；隐藏主键仍需对象、版本与权限预检查。
  readonly partition: string
  readonly excludeDataTypes: string
  readonly enableHiddenPk: boolean
  // 时间格式当前只允许 MySQL CSV/CUT 数据导出的 DATE 与 DATETIME；其余字段保留用于契约解码。
  readonly dateValueFormat: string
  readonly timeValueFormat: string
  readonly datetimeValueFormat: string
  readonly timestampValueFormat: string
  readonly timestampTzValueFormat: string
  readonly timestampLtzValueFormat: string
  readonly nlsDateFormat: string
  readonly nlsTimestampFormat: string
  readonly nlsTimestampTzFormat: string
}

export type ExportDraftInputValidation =
  | { readonly valid: true; readonly input: ExportDraftInput }
  | { readonly valid: false; readonly message: string }

const MAX_OBJECT_EXPRESSIONS = 100
const CSV_QUOTE_MODES: readonly CsvQuoteMode[] = ['all', 'all_not_null', 'minimal', 'non_numeric', 'none']
const COMPRESSION_ALGOS: readonly CompressionAlgo[] = ['zstd', 'zlib', 'gzip', 'snappy']
const MEMORY_PATTERN = /^[1-9][0-9]*[KMGTP]?$/
// EX-I7 文件拆分（2026-08-10）：--block-size 官方表达（正整数 MB 或正整数+MB/ROW）。
export const BLOCK_SIZE_PATTERN = /^[1-9][0-9]*(MB|ROW)?$/
// 时间格式只允许 ASCII 空格与已核验的格式字符，不能把制表符或换行作为空白字符接受。
const TIMESTAMP_FORMAT_PATTERN = /^[A-Za-z0-9 \-/:.'TZ]+$/

// supportsWhere 仅在指定表范围允许条件筛选，避免把表级参数扩展到全部对象或视图。
function supportsWhere(values: ExportDraftFormValues): boolean {
  return values.scopeKind === 'SPECIFIED' && values.objectType === 'TABLE'
}

// supportsPartition 分区筛选仅允许指定表的数据内容组合（EX-F072，2026-08-13 实测定版）。
function supportsPartition(values: ExportDraftFormValues): boolean {
  return values.contentKind !== 'DDL_ONLY' && values.scopeKind === 'SPECIFIED' && values.objectType === 'TABLE'
}

// supportsTimestampFormats 仅允许 MySQL CSV/CUT 数据导出的 DATE 与 DATETIME 格式；未知与 Oracle 均失败关闭。
function supportsTimestampFormats(values: ExportDraftFormValues): boolean {
  return values.compatibilityMode === 'MYSQL' && values.contentKind !== 'DDL_ONLY' && (values.formatKind === 'CSV' || values.formatKind === 'CUT')
}

// supportsCompactSchema 只允许会包含表 DDL 的组合，纯视图导出不得携带紧凑 Schema。
function supportsCompactSchema(values: ExportDraftFormValues): boolean {
  return values.contentKind !== 'DATA_ONLY' && (values.scopeKind === 'ALL' || values.objectType === 'TABLE')
}

export function validateExportDraftInput(values: ExportDraftFormValues): ExportDraftInputValidation {
  const dataSourceId = values.dataSourceId.trim()
  const nodeId = values.nodeId.trim()
  const database = values.database.trim()
  const filePath = values.filePath.trim()
  const logPath = values.logPath.trim()
  if (!dataSourceId) return { valid: false, message: '请先选择一个完成基础连接测试的数据源。' }
  if (!database) return { valid: false, message: '请填写默认数据库或 Schema。' }
  if (!nodeId) return { valid: false, message: '请选择一个已授权的执行节点。' }
  if (values.formatKind !== 'CSV' && values.formatKind !== 'CUT' && values.formatKind !== 'POS' && values.formatKind !== 'SQL' && values.formatKind !== 'PARQUET' && values.formatKind !== 'ORC' && values.formatKind !== 'AVRO') return { valid: false, message: '请选择支持的数据格式。' }
  // DDL + 数据只支持 CSV 数据格式；页面切换内容类型时应重置非 CSV 格式，服务端同样失败关闭。
  if (values.contentKind === 'DDL_AND_DATA' && values.formatKind !== 'CSV') return { valid: false, message: 'DDL + 数据只支持 CSV 数据格式。' }
  if (values.scopeKind === 'SPECIFIED') {
    const names = values.objectNames.map((name) => name.trim()).filter((name) => name.length > 0)
    if (names.length === 0) return { valid: false, message: values.objectType === 'VIEW' ? '请至少填写一个视图名称。' : '请至少填写一个表名。' }
    if (names.length > MAX_OBJECT_EXPRESSIONS) return { valid: false, message: `对象数量不能超过 ${MAX_OBJECT_EXPRESSIONS} 个。` }
    for (const name of names) {
      if (name.includes('*') || name.includes(',') || name.length > 256) {
        return { valid: false, message: '对象名称不能使用通配符或逗号，且不超过 256 个字符；多个对象请分行填写。' }
      }
    }
    if (values.objectType === 'VIEW' && values.contentKind !== 'DDL_ONLY') {
      return { valid: false, message: '视图只能导出对象定义（仅 DDL），不能导出数据。' }
    }
  }
  if (values.scopeKind === 'ALL' && values.excludeTables.length > 0 && values.contentKind === 'DDL_ONLY') {
    return { valid: false, message: '排除表仅在导出含表数据或 DDL + 数据时使用。' }
  }
  if (values.where.trim() && !supportsWhere(values)) {
    return { valid: false, message: '条件筛选仅可用于指定表范围，全部对象或视图导出不得携带该参数。' }
  }
  if (values.compactSchema && !supportsCompactSchema(values)) {
    return { valid: false, message: '紧凑 Schema 仅可用于包含表 DDL 的全部对象或指定表导出。' }
  }
  if (values.snapshot && (values.flashbackScn.trim() || values.flashbackTimestamp.trim())) {
    return { valid: false, message: '一致性快照与闪回参数互斥，只能选择其一。' }
  }
  // EX-I6：输出类型分流——本地输出要求节点绝对路径，对象存储按受控字段拼装 URI。
  const outputKind = values.outputKind
  if (outputKind === 'LOCAL') {
    if (!isAbsolutePathForPlatform(filePath, values.platform)) return { valid: false, message: '导出路径必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。' }
    // 本地输出没有对象存储凭据语义，残留引用必须失败关闭（与服务端 422 同口径）。
    if (values.storageCredentialId.trim()) return { valid: false, message: '本地输出不能绑定对象存储凭据。' }
  } else {
    const bucket = values.storageBucket.trim()
    const storagePath = values.storagePath.trim()
    const endpoint = values.storageEndpoint.trim()
    const region = values.storageRegion.trim()
    if (!bucket || !storagePath.startsWith('/')) return { valid: false, message: '对象存储需要填写 Bucket 和以 / 开头的对象路径。' }
    if (!endpoint && !region) return { valid: false, message: '对象存储需要填写 Endpoint 或 Region。' }
    // EX-I6 安全边界：存储凭据不进 URI，走执行槽位；表单不收集密钥。
    const credentialId = values.storageCredentialId.trim()
    if (credentialId) {
      // 凭据引用必须携带正修订，且提供方与输出类型一致（服务端提交前同样复验）。
      if (values.storageCredentialProvider !== outputKind) return { valid: false, message: '所选存储凭据与输出类型不一致，请重新选择。' }
      if (!Number.isSafeInteger(values.storageCredentialRevision) || values.storageCredentialRevision < 1) return { valid: false, message: '所选存储凭据缺少有效修订，请刷新凭据列表后重新选择。' }
    }
  }
  if (logPath && !isAbsolutePathForPlatform(logPath, values.platform)) return { valid: false, message: '日志路径必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。' }
  if (values.tmpPath.trim() && !isAbsolutePathForPlatform(values.tmpPath.trim(), values.platform)) return { valid: false, message: '临时分块目录必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。' }
  // EX-I4 POS 定版：仅包含数据时 POS 格式必须提供控制文件目录（--ctl-path）且为节点绝对路径；
  // 仅 DDL 内容不校验也不发送 controlFilePath（页面切换到仅 DDL 时清除数据格式专属残留）。
  if (values.contentKind !== 'DDL_ONLY' && values.formatKind === 'POS') {
    const controlFilePath = values.controlFilePath.trim()
    if (!controlFilePath) return { valid: false, message: 'POS 格式需要提供控制文件目录（--ctl-path）。' }
    if (!isAbsolutePathForPlatform(controlFilePath, values.platform)) return { valid: false, message: '控制文件目录必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。' }
  }
  // 额外筛选的范围门禁在数据/DDL 内容块之外独立执行，避免残留值绕过内容切换。
  const partition = values.partition.trim()
  if (partition && !supportsPartition(values)) return { valid: false, message: '分区筛选仅可用于指定表的数据导出，全部对象、视图或仅 DDL 导出不得携带该参数。' }
  const excludeDataTypes = splitNameList(values.excludeDataTypes)
  if (excludeDataTypes !== undefined && values.contentKind === 'DDL_ONLY') return { valid: false, message: '排除数据类型仅在导出数据内容时生效。' }
  if (values.addExtraMessage) return { valid: false, message: '附加对象信息仍需 sys 权限预检查，当前浏览器不支持提交。' }
  if (values.enableHiddenPk) return { valid: false, message: '隐藏主键仍需对象、版本和权限预检查，当前浏览器不支持提交。' }
  const timestampFormats = [
    ['DATE 值格式', values.dateValueFormat],
    ['TIME 值格式', values.timeValueFormat],
    ['DATETIME 值格式', values.datetimeValueFormat],
    ['TIMESTAMP 值格式', values.timestampValueFormat],
    ['TIMESTAMP TZ 值格式', values.timestampTzValueFormat],
    ['TIMESTAMP LTZ 值格式', values.timestampLtzValueFormat],
    ['NLS DATE', values.nlsDateFormat],
    ['NLS TIMESTAMP', values.nlsTimestampFormat],
    ['NLS TIMESTAMP TZ', values.nlsTimestampTzFormat],
  ] as const
  if (timestampFormats.some(([, value]) => value !== '') && !supportsTimestampFormats(values)) {
    return { valid: false, message: '时间戳值格式当前仅支持 MySQL 兼容模式的 CSV/CUT 数据导出。' }
  }
  const unverifiedTimestampFormats = [
    values.timeValueFormat, values.timestampValueFormat, values.timestampTzValueFormat, values.timestampLtzValueFormat,
    values.nlsDateFormat, values.nlsTimestampFormat, values.nlsTimestampTzFormat,
  ]
  if (unverifiedTimestampFormats.some((value) => value !== '')) {
    return { valid: false, message: '当前只支持 MySQL 的 DATE 与 DATETIME 值格式，其余时间格式仍待兼容性验证。' }
  }
  if (values.contentKind !== 'DDL_ONLY') {
    // EX-I4：格式适用性决定各面板校验范围；序列化选项按格式区分，
    // 文件布局、筛选与性能选项官方不限定格式，CSV/CUT/SQL 均参与校验。
    if (values.formatKind === 'CSV' && values.columnQuoteMode !== '' && !CSV_QUOTE_MODES.includes(values.columnQuoteMode)) return { valid: false, message: 'CSV 包围模式不是受支持的枚举值。' }
    if ((values.formatKind === 'CSV' || values.formatKind === 'CUT') && values.escapeCharacter.length > 1) return { valid: false, message: '转义字符官方仅支持单字符。' }
    // 前置 DROP 与保留 Schema 仅在 DDL 内容时生效；紧凑 Schema 已在上方按表 DDL 范围单独失败关闭。
    if (values.contentKind === 'DATA_ONLY' && (values.dropObject || values.retainSchema)) return { valid: false, message: 'DDL 行为参数仅在导出 DDL 内容时生效。' }
    for (const option of serializationOptionValues(values)) {
      if (option.length > 256) return { valid: false, message: '文本序列化选项值不能超过 256 个字符。' }
    }
    if (values.compress && values.compressionAlgo === '') return { valid: false, message: '启用压缩后请选择压缩算法。' }
    if (!values.compress && values.compressionAlgo !== '') return { valid: false, message: '压缩算法需要先启用压缩。' }
    if (values.compressionAlgo !== '' && !COMPRESSION_ALGOS.includes(values.compressionAlgo)) return { valid: false, message: '压缩算法不是受支持的枚举值。' }
    // EX-I7 压缩等级：官方按算法分范围——zstd 1~22、zlib -1~9；gzip/snappy 不支持指定等级。
    const compressionLevel = values.compressionLevel.trim()
    if (compressionLevel) {
      if (!values.compress || values.compressionAlgo === '') return { valid: false, message: '压缩等级需要先启用压缩并选择算法。' }
      const level = Number(compressionLevel)
      if (values.compressionAlgo === 'zstd' && (!Number.isInteger(level) || level < 1 || level > 22)) return { valid: false, message: 'zstd 压缩等级必须为 1~22 的整数。' }
      if (values.compressionAlgo === 'zlib' && (!Number.isInteger(level) || level < -1 || level > 9)) return { valid: false, message: 'zlib 压缩等级必须为 -1~9 的整数。' }
      if (values.compressionAlgo === 'gzip' || values.compressionAlgo === 'snappy') return { valid: false, message: 'gzip/snappy 不支持指定压缩等级。' }
    }
    // EX-I5：结构化格式不支持压缩（官方只支持 CSV/CUT/POS/SQL 可读格式）。
    if (isStructuredFormat(values.formatKind) && (values.compress || values.compressionAlgo || values.compressionLevel.trim())) return { valid: false, message: '结构化格式不支持压缩，请关闭压缩选项。' }
    if (values.maxFileSize && !isPositiveInteger(values.maxFileSize)) return { valid: false, message: '导出总量上限必须是正整数（单位 Byte）。' }
    if (values.querySql.trim() && (values.flashbackScn.trim() || values.flashbackTimestamp.trim())) return { valid: false, message: '自定义查询与闪回参数互斥，只能选择其一。' }
    // 条件筛选与自定义查询互斥。
    if (values.querySql.trim() && values.where.trim()) return { valid: false, message: '自定义查询与条件筛选互斥，只能选择其一。' }
    // EX-F072 分区筛选：与自定义查询互斥；分区名只接受字母数字下划线逗号分隔（范围门禁已在块外）。
    if (partition && values.querySql.trim()) return { valid: false, message: '自定义查询与分区筛选互斥，只能选择其一。' }
    if (partition && !/^[A-Za-z0-9_]+(,[A-Za-z0-9_]+)*$/.test(partition)) return { valid: false, message: '分区名只能使用字母、数字与下划线，多个分区用逗号分隔。' }
    // EX-F075 类型排除：类型名只接受字母数字下划线（仅数据内容门禁已在块外）。
    if (excludeDataTypes !== undefined && excludeDataTypes.some((name) => !/^[A-Za-z0-9_]+$/.test(name))) return { valid: false, message: '数据类型名只能使用字母、数字与下划线，多个类型用逗号分隔。' }
    // 时间格式只允许已核验字符和 ASCII 空格，拒绝制表符、换行、分号等会改变参数边界的字符。
    for (const [label, value] of timestampFormats) {
      if (value === '') continue
      if (value.length > 256 || value.trim() !== value || !TIMESTAMP_FORMAT_PATTERN.test(value)) return { valid: false, message: `${label}包含不支持的字符，只允许时间格式符号和 ASCII 空格，且首尾不能有空白。` }
    }
    if (values.includeColumnNames.trim() && values.excludeColumnNames.trim()) return { valid: false, message: '包含列与排除列互斥，只能选择其一。' }
    for (const list of [values.includeColumnNames, values.excludeColumnNames]) {
      for (const name of list.split(',').map((item) => item.trim()).filter((item) => item.length > 0)) {
        if (name.includes('*') || name.includes(',') || name.length > 256) return { valid: false, message: '列名不能使用通配符或逗号，且不超过 256 个字符。' }
      }
    }
    if (values.flashbackScn.trim() && !isPositiveInteger(values.flashbackScn)) return { valid: false, message: '闪回 SCN 必须是正整数。' }
    if (values.jvmMemory && !MEMORY_PATTERN.test(values.jvmMemory.trim())) return { valid: false, message: 'JVM 内存必须为数字加可选 K/M/G/T 后缀，例如 4G。' }
    // EX-I7 文件拆分：--block-size 只接受正整数或正整数+MB/ROW 后缀（官方不支持 1GB/1M 等格式）。
    if (isStructuredFormat(values.formatKind) && values.blockSize.trim()) return { valid: false, message: '结构化格式不支持文件拆分参数，请清空该选项。' }
    if (values.blockSize.trim() && !BLOCK_SIZE_PATTERN.test(values.blockSize.trim())) return { valid: false, message: '文件拆分必须为正整数（MB）或正整数+MB/ROW 后缀，例如 1024 或 256ROW。' }
    for (const [label, value] of [['线程数', values.thread], ['分页大小', values.pageSize], ['每线程宏块数', values.parallelMacro], ['游标抓取行数', values.fetchSize]] as const) {
      if (value.trim() && !isPositiveInteger(value)) return { valid: false, message: `${label}必须是正整数。` }
    }
  }
  return { valid: true, input: buildExportDraftInput(values, dataSourceId, nodeId, database, filePath, logPath) }
}

// serializationOptionValues 按当前格式返回参与 256 字符长度校验的文本选项值。
function serializationOptionValues(values: ExportDraftFormValues): readonly string[] {
  if (values.formatKind === 'CUT') return [values.columnSplitter, values.escapeCharacter, values.lineSeparator, values.nullString, values.fileEncoding]
  if (values.formatKind === 'SQL') return [values.lineSeparator, values.fileEncoding]
  // EX-I5：结构化格式只按官方格式表参与文件编码长度校验。
  if (isStructuredFormat(values.formatKind)) return [values.fileEncoding]
  return [values.columnSeparator, values.columnQuote, values.lineSeparator, values.nullString, values.fileEncoding]
}

function buildExportDraftInput(values: ExportDraftFormValues, dataSourceId: string, nodeId: string, database: string, filePath: string, logPath: string): ExportDraftInput {
  const dataOptions = values.contentKind !== 'DDL_ONLY'
  const formatKind = values.formatKind
  const csvOptions = dataOptions ? buildSerializationOptions(values, formatKind) : undefined
  const cutOptions = dataOptions && formatKind === 'CUT' && (values.trailDelimiter || values.removeNewline)
    ? { trailDelimiter: values.trailDelimiter || undefined, removeNewline: values.removeNewline || undefined } as CutOptions
    : undefined
  // 文件布局、筛选与性能参数官方不限定数据格式，CSV/CUT/POS/SQL 均按草稿内容发送。
  const outputOptions = dataOptions && (values.noNestedDir || values.maxFileSize || values.retainEmptyFiles || values.compress || values.compressionAlgo)
  const filterOptions = dataOptions ? buildFilterOptions(values) : undefined
  const performanceOptions = dataOptions ? buildPerformanceOptions(values) : undefined
  const outputFilePath = buildOutputFilePath(values, filePath)
  const config: GeneralizedExportConfig = {
    objectScope: values.scopeKind === 'ALL'
      ? { database, scopeKind: 'ALL' }
      : {
          database,
          scopeKind: 'SPECIFIED',
          objectTypes: [values.objectType],
          expressions: values.objectNames.map((name) => name.trim()).filter((name) => name.length > 0).map((name) => ({ name })),
          excludeTables: values.excludeTables.length > 0 ? values.excludeTables : undefined,
        },
    contentSelection: { contentKind: values.contentKind },
    dataFormat: values.contentKind === 'DDL_ONLY' ? undefined : { formatKind, csvOptions, cutOptions, timestampFormats: buildTimestampFormats(values) },
    outputConfig: {
      outputKind: values.outputKind || 'LOCAL',
      filePath: outputFilePath, logPath: logPath || undefined, skipCheckDir: values.skipCheckDir || undefined,
      noNestedDir: outputOptions ? (values.noNestedDir || undefined) : undefined,
      maxFileSize: values.maxFileSize ? Number(values.maxFileSize) : undefined,
      retainEmptyFiles: outputOptions ? (values.retainEmptyFiles || undefined) : undefined,
      compress: outputOptions ? (values.compress || undefined) : undefined,
      compressionAlgo: (values.compressionAlgo || undefined) as CompressionAlgo | undefined,
      // EX-I7 压缩等级：显式设置时随压缩发送（校验已按算法分范围）。
      compressionLevel: values.compressionLevel.trim() ? Number(values.compressionLevel.trim()) : undefined,
      // EX-I4 POS 定版：控制文件目录仅包含数据的 POS 格式发送；仅 DDL 与其余格式不携带（服务端对非 POS 携带失败关闭）。
      controlFilePath: dataOptions && formatKind === 'POS' ? (values.controlFilePath.trim() || undefined) : undefined,
      // EX-I6 对象存储：Multipart 本地临时分块目录。
      tmpPath: values.tmpPath.trim() || undefined,
      // EX-I6 存储凭据槽位：仅对象存储输出携带引用；LOCAL 不携带（服务端对携带失败关闭）。
      storageCredential: buildStorageCredentialReference(values),
    },
    filterConfig: filterOptions,
    performanceConfig: performanceOptions,
    // DDL 行为仅在包含 DDL 的内容时发送；附加对象信息仍由前置校验关闭。
    ddlBehavior: buildDDLBehavior(values),
  }
  return { configVersion: 'v6', dataSourceId, nodeId, config }
}

// buildDDLBehavior 按内容和对象范围构造 DDL 行为，紧凑 Schema 只发送给包含表 DDL 的组合。
function buildDDLBehavior(values: ExportDraftFormValues): DDLBehaviorOptions | undefined {
  if (values.contentKind === 'DATA_ONLY') return undefined
  const compactSchema = supportsCompactSchema(values) && values.compactSchema
  if (!values.dropObject && !values.retainSchema && !compactSchema) return undefined
  return {
    dropObject: values.dropObject || undefined,
    retainSchema: values.retainSchema || undefined,
    compactSchema: compactSchema || undefined,
  }
}

// buildTimestampFormats 只构造已完成兼容性验证的 MySQL DATE 与 DATETIME 格式。
function buildTimestampFormats(values: ExportDraftFormValues): TimestampFormatsOptions | undefined {
  if (!supportsTimestampFormats(values)) return undefined
  const formats: TimestampFormatsOptions = {
    dateValueFormat: values.dateValueFormat.trim() || undefined,
    datetimeValueFormat: values.datetimeValueFormat.trim() || undefined,
  }
  return Object.values(formats).some((value) => value !== undefined) ? formats : undefined
}

// buildOutputFilePath 按输出类型构建最终 filePath：本地输出原样使用；
// 对象存储按受控字段拼装 URI（无密钥参数，凭据走执行槽位）。
function buildOutputFilePath(values: ExportDraftFormValues, filePath: string): string {
  if (values.outputKind === 'LOCAL') return filePath
  const scheme = values.outputKind.toLowerCase()
  const params: string[] = []
  if (values.storageEndpoint.trim()) params.push(`endpoint=${encodeURIComponent(values.storageEndpoint.trim())}`)
  if (values.storageRegion.trim()) params.push(`region=${encodeURIComponent(values.storageRegion.trim())}`)
  const query = params.length > 0 ? `?${params.join('&')}` : ''
  return `${scheme}://${values.storageBucket.trim()}${values.storagePath.trim()}${query}`
}

// buildStorageCredentialReference 仅对象存储输出且显式选择凭据时构造引用；
// 引用只含标识与当前修订，绝不携带密钥材料。
function buildStorageCredentialReference(values: ExportDraftFormValues): StorageCredentialReference | undefined {
  if (values.outputKind === 'LOCAL') return undefined
  const storageCredentialId = values.storageCredentialId.trim()
  if (!storageCredentialId || !Number.isSafeInteger(values.storageCredentialRevision) || values.storageCredentialRevision < 1) return undefined
  return { storageCredentialId, revision: values.storageCredentialRevision }
}

// buildSerializationOptions 按格式只发送服务端能力矩阵内适用的序列化选项：
// CSV 全量；CUT 共享文本选项；SQL 仅行分隔符与文件编码。
function buildSerializationOptions(values: ExportDraftFormValues, formatKind: ExportDataFormatKind): CsvOptions | undefined {
  if (formatKind === 'CSV') {
    return values.skipHeader || values.withTrim || values.columnSeparator || values.columnQuote || values.columnQuoteMode || values.escapeCharacter || values.lineSeparator || values.nullString || values.fileEncoding
      ? {
          skipHeader: values.skipHeader || undefined,
          columnSeparator: values.columnSeparator || undefined,
          columnQuote: values.columnQuote || undefined,
          columnQuoteMode: (values.columnQuoteMode || undefined) as CsvQuoteMode | undefined,
          escapeCharacter: values.escapeCharacter || undefined,
          lineSeparator: values.lineSeparator || undefined,
          nullString: values.nullString || undefined,
          fileEncoding: values.fileEncoding || undefined,
          withTrim: values.withTrim || undefined,
        }
      : undefined
  }
  if (formatKind === 'CUT') {
    return values.withTrim || values.escapeCharacter || values.lineSeparator || values.nullString || values.fileEncoding || values.columnSplitter
      ? {
          escapeCharacter: values.escapeCharacter || undefined,
          lineSeparator: values.lineSeparator || undefined,
          nullString: values.nullString || undefined,
          fileEncoding: values.fileEncoding || undefined,
          withTrim: values.withTrim || undefined,
          columnSplitter: values.columnSplitter || undefined,
        }
      : undefined
  }
  // EX-I5：结构化格式只发送官方格式表列出的文件编码。
  if (isStructuredFormat(formatKind)) {
    return values.fileEncoding ? { fileEncoding: values.fileEncoding || undefined } : undefined
  }
  return values.lineSeparator || values.fileEncoding
    ? { lineSeparator: values.lineSeparator || undefined, fileEncoding: values.fileEncoding || undefined }
    : undefined
}

function buildFilterOptions(values: ExportDraftFormValues): FilterOptions | undefined {
  const querySql = values.querySql.trim()
  const where = supportsWhere(values) ? values.where.trim() : ''
  const includeColumnNames = splitNameList(values.includeColumnNames)
  const excludeColumnNames = splitNameList(values.excludeColumnNames)
  const flashbackScn = values.flashbackScn.trim() ? Number(values.flashbackScn) : undefined
  const flashbackTimestamp = values.flashbackTimestamp.trim() || undefined
  // 即使调用方绕过表单校验，快照与闪回冲突时也不能把快照送入草稿。
  const snapshot = values.snapshot && flashbackScn === undefined && flashbackTimestamp === undefined ? true : undefined
  // 分区筛选只发送给指定表数据组合，类型排除只随数据内容发送。
  const partition = supportsPartition(values) ? values.partition.trim() || undefined : undefined
  const excludeDataTypes = splitNameList(values.excludeDataTypes)
  if (!querySql && !where && includeColumnNames === undefined && excludeColumnNames === undefined && !values.excludeVirtualColumns && flashbackScn === undefined && flashbackTimestamp === undefined && snapshot === undefined && partition === undefined && excludeDataTypes === undefined) {
    return undefined
  }
  return {
    querySql: querySql || undefined,
    // 条件筛选只会在指定表范围随数据筛选发送。
    where: where || undefined,
    includeColumnNames,
    excludeColumnNames,
    excludeVirtualColumns: values.excludeVirtualColumns || undefined,
    flashbackScn,
    flashbackTimestamp,
    // 一致性快照为无值开关，已在上方排除与闪回参数的组合。
    snapshot,
    partition,
    excludeDataTypes,
  }
}

function buildPerformanceOptions(values: ExportDraftFormValues): PerformanceOptions | undefined {
  const thread = values.thread.trim() ? Number(values.thread) : undefined
  const pageSize = values.pageSize.trim() ? Number(values.pageSize) : undefined
  const parallelMacro = values.parallelMacro.trim() ? Number(values.parallelMacro) : undefined
  const fetchSize = values.fetchSize.trim() ? Number(values.fetchSize) : undefined
  const jvmMemory = values.jvmMemory.trim() || undefined
  const blockSize = values.blockSize.trim() || undefined
  if (thread === undefined && pageSize === undefined && parallelMacro === undefined && fetchSize === undefined && jvmMemory === undefined && blockSize === undefined) {
    return undefined
  }
  return { thread, pageSize, parallelMacro, fetchSize, jvmMemory, blockSize }
}

function splitNameList(value: string): readonly string[] | undefined {
  const names = value.split(',').map((name) => name.trim()).filter((name) => name.length > 0)
  return names.length > 0 ? names : undefined
}

function isPositiveInteger(value: string): boolean {
  return /^[1-9][0-9]*$/.test(value.trim())
}

function isAbsolutePathForPlatform(filePath: string, platform: string): boolean {
  if (platform === 'WINDOWS_AMD64') return /^\/[A-Za-z]:\//.test(filePath) && !filePath.includes('\\') && !filePath.split('/').some((part) => part === '.' || part === '..')
  if (platform === 'LINUX_AMD64' || platform === 'LINUX_ARM64') return filePath.startsWith('/')
  return false
}
