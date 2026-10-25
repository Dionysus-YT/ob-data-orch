import type { DataSourceSummary, ExecutionNodeSummary } from '@/api/browser'
import { importContents, importFormats, importObjectTypes, type ImportFormat, type NormalImportForm } from './normalImportForm'
import { importExecutionFields, importObjectLabels, normalImportSteps } from './normalImportOptions'

export interface ImportValidationIssue { step: number; field: string; message: string }
export interface ImportReferenceFacts { source?: DataSourceSummary; node?: ExecutionNodeSummary }

function containsControl(value: string) { return Array.from(value).some(character => character.charCodeAt(0) < 32) }

export function isNormalImportSource(source: DataSourceSummary): boolean {
  return source.state === 'ENABLED' && source.lastTestStatus === 'SUCCEEDED' && source.connectionKind === 'ODP'
    && ['MYSQL', 'ORACLE'].includes(source.compatibilityMode)
}

// 路径只做匹配平台的语法检查，原字符串保留；不推断目录存在、权限或文件集合。
export function isImportAbsolutePath(path: string, platform?: ExecutionNodeSummary['platform']): boolean {
  if (!path || containsControl(path)) return false
  if (platform === 'WINDOWS_AMD64') return /^[a-zA-Z]:[\\/]/.test(path) && !/[<>"|?*]/.test(path.slice(2)) && !path.slice(2).includes(':')
  if (platform === 'LINUX_AMD64' || platform === 'LINUX_ARM64') return path.startsWith('/')
  return false
}

export function importTraits(form: NormalImportForm) {
  const structuredData = form.content === 'DATA_ONLY' || form.content === 'DDL_AND_DATA'
  const hasData = structuredData
  const hasDdl = form.content === 'DDL_ONLY' || form.content === 'DDL_AND_DATA'
  // 页面只承载当前三种文件格式；未知或越界值不能进入活动配置。
  const format: ImportFormat | '' = structuredData && importFormats.includes(form.format as ImportFormat) ? form.format as ImportFormat : ''
  return {
    structuredData, hasData, hasDdl, format,
    text: ['CSV', 'CUT'].includes(format), compressedInput: Boolean(format), autoMapping: format === 'CSV',
  }
}

export function importAvailableObjectTypes(form: NormalImportForm, mode?: string) {
  if (!importTraits(form).hasDdl) return ['table'] as const
  const oracleOnly = ['role', 'sequence', 'synonym', 'type', 'type-body', 'package', 'package-body']
  return importObjectTypes.filter(type => mode === 'ORACLE' || !oracleOnly.includes(type))
}

// 活动配置用于本地摘要与校验；不是请求 DTO，更不是 argv 或可执行命令。
export function projectNormalImport(form: NormalImportForm, facts: ImportReferenceFacts) {
  const traits = importTraits(form)
  const fields = traits.format ? form.formats[traits.format] : undefined
  const parser: Record<string, string | boolean> = {}
  if (fields) {
    if (traits.text) {
      for (const key of ['skipHeader', 'withTrim'] as const) if (fields[key]) parser[key] = true
      for (const key of ['escapeCharacter', 'nullString', 'emptyString'] as const) if (fields[key]) parser[key] = fields[key]
    }
    if (traits.format === 'SQL' && fields.nullString) parser.nullString = fields.nullString
    if (traits.format === 'CSV') {
      if (fields.columnSeparator) parser.columnSeparator = fields.columnSeparator
      if (fields.columnQuote) parser.columnQuote = fields.columnQuote
    }
    if (traits.format === 'CUT') {
      if (fields.columnSplitter) parser.columnSplitter = fields.columnSplitter
      for (const key of ['skipFooter', 'ignoreEscape', 'trailDelimiter'] as const) if (fields[key]) parser[key] = true
      if (fields.ignoreEscape) delete parser.escapeCharacter
    }
    if (traits.compressedInput && fields.lineSeparator) parser.lineSeparator = fields.lineSeparator
    if (traits.compressedInput && fields.compress) {
      parser.compress = true
      if (fields.compressionAlgo) parser.compressionAlgo = fields.compressionAlgo
      if (['zstd', 'zlib'].includes(fields.compressionAlgo || 'zstd') && fields.compressionLevel) parser.compressionLevel = fields.compressionLevel
    }
    if (facts.source?.compatibilityMode === 'MYSQL' && traits.text && fields.dateFormat) {
      parser.dateFormat = fields.dateFormat
      if (fields.defaultDate) parser.defaultDate = fields.defaultDate
    }
  }
  // 显式列名单冲突时不擅自选边；保留原输入供修正，活动摘要不带冲突名单，校验负责阻断。
  const mapping = fields ? {
    kind: fields.mapping,
    ...(fields.mapping === 'CONTROL' ? { ctlPath: fields.ctlPath } : {}),
    ...(fields.mapping === 'EXPLICIT' && !(fields.includeColumns.trim() && fields.excludeColumns.trim()) ? { includeColumns: fields.includeColumns, excludeColumns: fields.excludeColumns } : {}),
    ...(fields.excludeDataTypes ? { excludeDataTypes: fields.excludeDataTypes } : {}),
  } : undefined
  const execution = Object.fromEntries(importExecutionFields.filter(field =>
    (!field.dataOnly || traits.hasData) && (!field.oracleOnly || facts.source?.compatibilityMode === 'ORACLE')
    && form.execution[field.key] !== '',
  ).map(field => [field.key, form.execution[field.key]]))
  const objects = form.scope === 'SPECIFIC' ? Object.fromEntries(importAvailableObjectTypes(form, facts.source?.compatibilityMode)
    .filter(type => form.objects[type].trim()).map(type => [type, form.objects[type]])) : {}
  return {
    dataSourceId: form.dataSourceId, sourceRevision: facts.source?.revision, credentialRevision: facts.source?.credentialRevision,
    compatibilityMode: facts.source?.compatibilityMode, environment: facts.source?.environment,
    nodeId: form.nodeId, nodeRevision: facts.node?.revision, platform: facts.node?.platform,
    database: form.database, filePath: form.filePath, fileSuffix: form.fileSuffix, fileEncoding: form.fileEncoding,
    fileRegex: form.fileRegex, logPath: form.logPath, content: importContents.includes(form.content) ? form.content : '', format: traits.format,
    scope: form.scope, objects, excludeTables: form.scope === 'ALL' || objects.table ? form.excludeTables : '',
    parser, mapping, execution, compatMysql: traits.hasDdl && facts.source?.compatibilityMode === 'MYSQL' && form.compatMysql,
    compatMysqlVersion: traits.hasDdl && facts.source?.compatibilityMode === 'MYSQL' && form.compatMysql ? form.compatMysqlVersion : '',
    replaceData: traits.structuredData && form.replaceData, replaceObject: traits.hasDdl && form.replaceObject,
    cleanup: traits.structuredData ? form.cleanup : 'NONE', taskName: form.taskName,
  }
}

export function validateNormalImport(form: NormalImportForm, facts: ImportReferenceFacts): ImportValidationIssue[] {
  const issues: ImportValidationIssue[] = []
  const add = (step: number, field: string, message: string) => issues.push({ step, field, message })
  const config = projectNormalImport(form, facts)
  const traits = importTraits(form)
  if (config.compatMysql && !['mysql56', 'mysql57', 'mysql80'].includes(config.compatMysqlVersion)) add(normalImportSteps.execution, 'compatMysqlVersion', '请选择源 MySQL 版本：5.6、5.7 或 8.0。')
  if (!facts.source || !isNormalImportSource(facts.source)) add(normalImportSteps.source, 'dataSourceId', '请选择已启用、基础连接测试成功的 ODP 数据源。')
  if (!form.database.trim() || containsControl(form.database)) add(normalImportSteps.source, 'database', '请填写有效的目标数据库 / Schema。')
  if (!facts.node?.acceptsNewTasks) add(normalImportSteps.input, 'nodeId', '请选择当前可接收任务的执行节点；普通导入资格仍待任务预检查。')
  if (!isImportAbsolutePath(form.filePath, facts.node?.platform)) add(normalImportSteps.input, 'filePath', '请填写与执行节点平台匹配的本地绝对路径。')
  if (form.logPath && !isImportAbsolutePath(form.logPath, facts.node?.platform)) add(normalImportSteps.input, 'logPath', '日志目录必须是执行节点上的绝对路径。')
  if (containsControl(form.fileSuffix)) add(normalImportSteps.input, 'fileSuffix', '文件后缀不能包含控制字符。')
  if (form.fileRegex.includes(String.fromCharCode(0))) add(normalImportSteps.input, 'fileRegex', '文件正则不能包含空字符；正则语义由工具预检查核对。')
  if (!importContents.includes(form.content)) add(normalImportSteps.input, 'content', '请选择导入结构和数据、仅导入数据或仅导入结构。')
  if (traits.structuredData && !traits.format) add(normalImportSteps.input, 'format', '请选择一种数据格式；当前仅支持 CSV、CUT、SQL。')
  const fields = traits.format ? form.formats[traits.format] : undefined
  if (fields) {
    for (const key of ['columnQuote', 'escapeCharacter'] as const) {
      if (config.parser[key] && Array.from(String(config.parser[key])).length !== 1) add(normalImportSteps.input, key, `${key === 'columnQuote' ? 'CSV 字符串包围符' : '转义字符'}只能是单个字符。`)
    }
    if (traits.text && (fields.nullString || '\\N') === (fields.emptyString || '\\E')) add(normalImportSteps.input, 'emptyString', 'NULL 标识和空字符串标识不能相同。')
    if (fields.compress && !['', 'zstd', 'zlib', 'gzip', 'snappy'].includes(fields.compressionAlgo)) add(normalImportSteps.input, 'compressionAlgo', '请选择 zstd、zlib、gzip 或 snappy 压缩算法。')
    if (config.parser.compress && fields.compressionLevel) {
      const min = fields.compressionAlgo === 'zlib' ? -1 : 1
      const max = fields.compressionAlgo === 'zlib' ? 9 : 22
      if (config.parser.compressionLevel && (!/^-?\d+$/.test(fields.compressionLevel) || Number(fields.compressionLevel) < min || Number(fields.compressionLevel) > max)) add(normalImportSteps.input, 'compressionLevel', `压缩等级必须是 ${min}～${max} 的整数。`)
    }
    if (facts.source?.compatibilityMode === 'MYSQL' && traits.text && fields.defaultDate && !fields.dateFormat) add(normalImportSteps.input, 'dateFormat', '设置无效日期替代值时，请显式填写原日期格式。')
    if (config.mapping?.kind === 'CONTROL' && !isImportAbsolutePath(fields.ctlPath, facts.node?.platform)) add(normalImportSteps.objects, 'ctlPath', '控制文件目录必须是执行节点上的绝对路径。')
    if (config.mapping?.kind === 'AUTO' && !traits.autoMapping) add(normalImportSteps.objects, 'mapping', '当前格式不支持官方自动列映射。')
    if (config.mapping?.kind === 'EXPLICIT' && !fields.includeColumns.trim() && !fields.excludeColumns.trim()) add(normalImportSteps.objects, 'includeColumns', '显式列名单至少填写包含列或排除列。')
    if (config.mapping?.kind === 'EXPLICIT' && fields.includeColumns.trim() && fields.excludeColumns.trim()) add(normalImportSteps.objects, 'includeColumns', '包含列和排除列不能同时配置，请保留一种。')
  }
  if (!form.scope) add(normalImportSteps.objects, 'scope', '请选择全部对象或指定对象。')
  if (form.scope === 'SPECIFIC' && Object.keys(config.objects).length === 0) add(normalImportSteps.objects, 'objects', '请至少填写一种适用对象的表达式。')
  for (const [type, expression] of Object.entries(config.objects)) if (containsControl(expression)) add(normalImportSteps.objects, 'objects', `${importObjectLabels[type as keyof typeof importObjectLabels]}表达式不能包含控制字符。`)
  for (const field of importExecutionFields) {
    const value = config.execution[field.key]
    if (!value) continue
    if (field.integerMin !== undefined && (!/^-?\d+$/.test(value) || !Number.isSafeInteger(Number(value)) || Number(value) < field.integerMin)) add(normalImportSteps.execution, field.key, `${field.label}必须是大于等于 ${field.integerMin} 的安全整数。`)
    if (field.key === 'rw' && (!/^(?:\d+(?:\.\d+)?|\.\d+)$/.test(value) || Number(value) <= 0 || Number(value) > 1)) add(normalImportSteps.execution, field.key, '文件解析线程比例必须大于 0 且不超过 1。')
    if (field.key === 'strict' && !['true', 'false'].includes(value)) add(normalImportSteps.execution, field.key, '请选择有效的脏数据退出状态。')
    if (field.key === 'memory' && !/^[1-9]\d*[mMgG]$/.test(value)) add(normalImportSteps.execution, field.key, 'JVM 内存请使用正整数和 M / G 单位，实际可用资源仍待节点检查。')
  }
  if (containsControl(form.taskName)) add(normalImportSteps.review, 'taskName', '任务名称不能包含控制字符。')
  return issues
}
