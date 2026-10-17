import type { ImportContent, ImportExecutionKey, ImportFormat, ImportObjectType } from './normalImportForm'

export const normalImportTitles = ['选择数据源', '导入文件与格式', '对象与映射', '执行参数', '预检查与命令'] as const
export const normalImportSteps = { source: 1, input: 2, objects: 3, execution: 4, review: 5 } as const

export const importContentChoices: readonly { value: ImportContent; label: string; description: string }[] = [
  { value: 'DDL_AND_DATA', label: '导入结构和数据', description: '分别导入对象定义文件和数据文件。' },
  { value: 'DATA_ONLY', label: '仅导入数据', description: '向已有目标表写入所选格式的数据。' },
  { value: 'DDL_ONLY', label: '仅导入结构', description: '只导入对象定义文件，不导入表数据。' },
]

export const importFormatChoices: readonly { value: ImportFormat; label: string; description: string }[] = [
  { value: 'CSV', label: 'CSV', description: '官方推荐；分隔符、包围符与转义可配置。' },
  { value: 'CUT', label: 'CUT', description: '按字符串分隔字段。' },
  { value: 'SQL', label: 'SQL', description: 'INSERT 数据文件；不提供 SQL 编辑入口。' },
]

export const importObjectLabels: Record<ImportObjectType, string> = {
  table: '表', 'table-group': '表组', view: '视图', trigger: '触发器', 'obj-user': '用户', role: '角色',
  sequence: '序列', synonym: '同义词', type: '类型', 'type-body': '类型体', package: '包',
  'package-body': '包体', function: '函数', procedure: '存储过程',
}

// 这里只登记普通导入的表单展示与本地校验，不参与命令生成或服务端能力判定。
export interface ImportExecutionField {
  key: ImportExecutionKey
  label: string
  option: string
  group: 'errors' | 'performance' | 'session'
  placeholder: string
  description: string
  dataOnly?: boolean
  oracleOnly?: boolean
  integerMin?: number
}

export const importExecutionFields: readonly ImportExecutionField[] = [
  { key: 'maxDiscards', label: '最大重复数据量', option: 'max-discards', group: 'errors', placeholder: '沿用官方默认 -1', dataOnly: true, integerMin: -1, description: '单表允许的重复数据量；-1 表示忽略重复并继续，0 表示不容忍重复。' },
  { key: 'maxErrors', label: '最大写入错误数', option: 'max-errors', group: 'errors', placeholder: '沿用官方默认 1000', dataOnly: true, integerMin: -1, description: '单表允许的数据库写入错误数；支持 -1、0 或正整数，留空继承默认 1000。' },
  { key: 'strict', label: '脏数据影响退出状态', option: 'strict', group: 'errors', placeholder: '沿用官方默认 true', dataOnly: true, description: '控制坏记录或丢弃记录是否影响最终退出码，不改变解析和错误事实。' },
  { key: 'thread', label: '写入线程', option: 'thread', group: 'performance', placeholder: '沿用官方 CPU×2、上限 32', integerMin: 1, description: '设置写入并发，也用于支持并行 DDL 的目标版本；实际版本与资源仍须预检查。' },
  { key: 'rw', label: '文件解析线程比例', option: 'rw', group: 'performance', placeholder: '沿用官方默认 1', dataOnly: true, description: '文件解析线程数按写入线程数乘此比例计算；仅数据导入适用，不是旁路服务端并行度。' },
  { key: 'batch', label: '每批事务记录数', option: 'batch', group: 'performance', placeholder: '沿用工具自适应默认', integerMin: 1, dataOnly: true, description: '每个写入事务的记录数；留空沿用工具默认，不限制导入总行数。' },
  { key: 'blockSize', label: '文件逻辑分块（MB）', option: 'block-size', group: 'performance', placeholder: '沿用官方默认 64 MB', integerMin: 1, dataOnly: true, description: '将已有大文件逻辑切分为读取块，单位 MB；不生成物理子文件，也不是导出文件拆分。' },
  { key: 'maxTps', label: '最大 TPS（行/秒）', option: 'max-tps', group: 'performance', placeholder: '未设置时不限速', integerMin: 1, dataOnly: true, description: '限制导入写入速率，单位行/秒；不限制文件数量或任务总导入量。' },
  { key: 'maxWaitTimeout', label: '合并等待上限（小时）', option: 'max-wait-timeout', group: 'performance', placeholder: '沿用官方默认 3 小时', integerMin: 1, dataOnly: true, description: '数据库合并导致数据导入暂停时的最长等待时间，单位小时；不是连接超时或任务总超时。' },
  { key: 'memory', label: 'JVM 内存', option: 'mem', group: 'performance', placeholder: '沿用官方默认 4G', description: '运行 OBLOADER 的 JVM 内存；使用正整数和 M/G 单位。包含数据时堆至少 700 MiB，实际可用资源由节点预检查核对。' },
  { key: 'nlsDateFormat', label: 'Oracle 日期格式', option: 'nls-date-format', group: 'session', placeholder: '沿用官方会话默认', oracleOnly: true, description: '设置 Oracle 会话 nls_date_format；仅 Oracle 兼容模式适用，不是文件输出日期格式。' },
  { key: 'nlsTimestampFormat', label: 'Oracle 时间戳格式', option: 'nls-timestamp-format', group: 'session', placeholder: '沿用官方会话默认', oracleOnly: true, description: '设置 Oracle 会话 nls_timestamp_format；仅 Oracle 兼容模式适用。' },
  { key: 'nlsTimestampTzFormat', label: 'Oracle 带时区时间戳格式', option: 'nls-timestamp-tz-format', group: 'session', placeholder: '沿用官方会话默认', oracleOnly: true, description: '设置 Oracle 会话 nls_timestamp_tz_format；仅 Oracle 兼容模式适用。' },
]

export const importPrecheckItems = [
  { check: 'DATABASE_CONNECTIVITY', name: '目标数据库连接', description: '当前租约与凭据绑定的目标数据库连接。', step: normalImportSteps.source },
  { check: 'WRITE_ACCESS', name: '对象写入权限', description: '本次写入、建表与高风险操作所需权限。', step: normalImportSteps.source },
  { check: 'TOOL_ENVIRONMENT', name: '执行工具', description: '固定 OBLOADER 4.3.5 与 Java 运行环境。', step: normalImportSteps.input },
  { check: 'NODE_RESOURCE', name: '节点资源', description: '当前节点内存事实与任务资源额度。', step: normalImportSteps.execution },
  { check: 'INPUT_FILES', name: '文件集合', description: '路径可读性、完整匹配集合与文件摘要。', step: normalImportSteps.input },
  { check: 'INPUT_MAPPING', name: '对象与列映射', description: '目标结构、控制文件及列数、列名。', step: normalImportSteps.objects },
] as const

export function importEnvironmentLabel(value: string) {
  return ({ DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' } as Record<string, string>)[value] ?? '环境待确认'
}
