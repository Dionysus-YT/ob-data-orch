import type { NormalImportForm } from './normalImportForm'
import { importExecutionFields, importObjectLabels, normalImportSteps } from './normalImportOptions'
import { importTraits } from './normalImportRules'

interface ParameterGroup { names: string; step: number; placement: 'input' | 'derived'; reason: string }
interface ParameterDetail { label: string; description: string }

// 新建任务只列本轮适用参数；完整 help 的研究基线与排除理由保留在参数映射文档。
const groups: readonly ParameterGroup[] = [
  { names: 'host port user password database no-sys sys-user sys-password character-set session-config', step: normalImportSteps.source, placement: 'derived', reason: '由受管数据源与安全上下文负责；本轮不读取秘密、生成连接参数或提供任务级覆盖。' },
  { names: 'file-path file-suffix file-encoding file-regular-expression log-path', step: normalImportSteps.input, placement: 'input', reason: '本地输入配置；文件事实尚未检查。' },
  { names: 'ddl csv sql cut', step: normalImportSteps.input, placement: 'input', reason: '导入内容独立单选，数据文件仅配置 CSV、CUT 或 SQL。' },
  { names: 'column-quote column-separator column-splitter line-separator escape-character skip-header skip-footer ignore-escape trail-delimiter with-trim null-string empty-string compress compression-algo compression-level date-format default-date', step: normalImportSteps.input, placement: 'input', reason: '格式适用时可配置；非活动格式只暂存，不参与配置。' },
  { names: 'all table table-group view trigger obj-user role sequence synonym type type-body package package-body function procedure exclude-table ctl-path auto-column-mapping include-column-names exclude-column-names exclude-data-types', step: normalImportSteps.objects, placement: 'input', reason: '按内容、格式与兼容模式显示；对象和映射均未实测。' },
  { names: 'max-discards max-errors strict thread rw batch block-size max-tps max-wait-timeout mem nls-date-format nls-timestamp-format nls-timestamp-tz-format compat-mode replace-data replace-object truncate-table delete-from-table', step: normalImportSteps.execution, placement: 'input', reason: '仅保留当前适用的执行配置；实际资源与行为仍须预检查。' },
  { names: 'yes', step: normalImportSteps.review, placement: 'derived', reason: '只能由服务端在冻结快照的等价确认后派生；本地阅读确认不生成 yes。' },
]

// 字段提示与目录共用含义，避免同一参数在不同入口出现两套解释。
const details: Record<string, ParameterDetail> = {
  host: { label: '连接地址', description: '目标数据源的 ODP 地址；由受管连接上下文提供。' },
  port: { label: 'SQL 端口', description: '目标数据源的 SQL 连接端口，不是旁路 RPC 端口。' },
  user: { label: '数据库账号', description: '目标数据源的完整数据库账号，由受管连接上下文提供。' },
  password: { label: '数据库凭据', description: '由安全凭据通道短时解析；页面、配置摘要和命令预览不得展示秘密。' },
  database: { label: '目标数据库', description: '承接本次普通导入的数据库或 Schema，不是源文件的存放目录。' },
  'no-sys': { label: '无 sys 凭据', description: '在适用连接上下文中使用不提供 sys 凭据的模式，不代表自动取得额外权限。' },
  'sys-user': { label: 'sys 用户', description: '适用连接上下文中的 sys 账号，不在任务内重复输入。' },
  'sys-password': { label: 'sys 凭据', description: '适用连接上下文中的 sys 凭据，遵守与数据库凭据相同的秘密边界。' },
  'character-set': { label: '连接字符集', description: '属于数据源的连接上下文；与输入文件编码不同，本页不提供任务级覆盖。' },
  'session-config': { label: '受管会话配置', description: '属于受管连接上下文，不接受任务内任意会话配置文件或 SQL。' },
  'file-path': { label: '文件或目录路径', description: '执行节点上的本地输入路径，原样传递；存在性、可读性与文件匹配仍须节点检查。' },
  'file-suffix': { label: '文件后缀', description: '筛选需要导入的文件后缀；留空沿用当前内容和格式的工具默认值。' },
  'file-encoding': { label: '文件编码', description: '读取输入文件时使用的编码；留空继承 UTF-8，不修改数据库连接字符集。' },
  'file-regular-expression': { label: '文件正则', description: '筛选输入目录内的文件名；填写正则不表示已经发现有效文件。' },
  'log-path': { label: '日志目录（可选）', description: '执行节点上的工具日志目录；留空沿用工具默认，不是导入数据路径。' },
  ddl: { label: '对象定义', description: '处理对象定义文件；仅对象定义不使用数据解析、列映射或数据写入参数。' },
  csv: { label: 'CSV', description: '读取 CSV 数据文件；列分隔符、字符串包围符与转义按输入文件配置。' },
  cut: { label: 'CUT', description: '读取以字符串分隔字段的 CUT 数据文件。' },
  sql: { label: 'SQL', description: '读取官方支持的 INSERT 数据文件；不提供任意 SQL 编辑或执行入口。' },
  'column-quote': { label: 'CSV 字符串包围符', description: '识别 CSV 字段的单字符包围符；留空沿用官方默认英文单引号。' },
  'column-separator': { label: 'CSV 列分隔符', description: '识别 CSV 字段边界，支持多字符；留空沿用逗号。' },
  'column-splitter': { label: 'CUT 列分隔字符串', description: '识别 CUT 字段之间的分隔字符串，仅 CUT 适用。' },
  'line-separator': { label: '行分隔符', description: '识别数据记录的行结束符；留空继承工具的平台默认值。' },
  'escape-character': { label: '转义字符', description: '识别文本字段中的转义；仅 CSV/CUT 适用，CUT 忽略转义时不生效。' },
  'skip-header': { label: '跳过首行', description: '跳过 CSV/CUT 输入文件的首行，不自动开启按列名映射。' },
  'skip-footer': { label: '跳过末行', description: '跳过 CUT 输入文件的末行，仅 CUT 适用。' },
  'ignore-escape': { label: '忽略 CUT 转义', description: '按普通字符读取 CUT 中的转义字符；启用后不应用自定义转义字符。' },
  'trail-delimiter': { label: '行尾包含分隔符', description: '按 CUT 行末带分隔符的输入约定解析记录。' },
  'with-trim': { label: '去除左右空格', description: '去除 CSV/CUT 字段两端的空格，可能改变需要保留空格的文本。' },
  'null-string': { label: 'NULL 标识', description: '将匹配的文本识别为数据库 NULL；默认标识为反斜杠 N，应与空字符串标识不同。' },
  'empty-string': { label: '空字符串标识', description: '将匹配的 CSV/CUT 文本识别为空字符串；默认标识为反斜杠 E，不等同于 NULL。' },
  compress: { label: '输入文件已压缩', description: '读取 OBDUMPER 产生的压缩数据文件；不在导入期间生成压缩输出。' },
  'compression-algo': { label: '压缩算法', description: '指定压缩输入使用的算法；支持 zstd、zlib、gzip、snappy，留空继承 zstd。' },
  'compression-level': { label: '压缩等级', description: '与压缩输入的等级匹配；zstd 为 1～22，zlib 为 -1～9，gzip/snappy 不使用等级。' },
  'date-format': { label: '原日期格式', description: '描述 MySQL DATE/DATETIME 原文本的日期格式，仅 CSV/CUT 适用。' },
  'default-date': { label: '无效日期替代值', description: '替代无法按指定格式读取的 MySQL DATE/DATETIME 值；目标列适用性仍须检查。' },
  all: { label: '全部对象', description: '导入输入文件中符合当前内容与兼容模式的全部对象，不表示扫描任意数据库对象。' },
  'exclude-table': { label: '排除表', description: '从全部对象或指定表表达式中排除匹配的表；留空不排除。' },
  'ctl-path': { label: '控制文件目录', description: '使用执行节点上指定目录的控制文件建立列映射；不与自动映射或显式列名单同时生效。' },
  'auto-column-mapping': { label: '官方自动列映射', description: '按 CSV 文件首行列名映射目标列；仅 CSV 适用，目标列与文件首行仍须检查。' },
  'include-column-names': { label: '包含列', description: '按文件列顺序指定目标列名，精确匹配大小写；本页与排除列择一配置。' },
  'exclude-column-names': { label: '排除列', description: '指定不写入的目标列；本页与包含列择一配置，不改变输入文件。' },
  'exclude-data-types': { label: '排除数据类型', description: '排除指定类型的列数据；实际类型与目标结构须由后续预检查核对。' },
  'compat-mode': { label: 'MySQL DDL 兼容导入', description: '按显式选择的源 MySQL 5.6、5.7 或 8.0 版本处理定义兼容性；仅目标为 MySQL 且包含定义时适用。' },
  'replace-data': { label: '替换重复数据', description: '替换与主键或非空唯一键冲突的数据；需要包含数据，实际影响仍须检查。' },
  'replace-object': { label: '替换已有对象定义', description: '以输入定义替换已有目标对象，可能影响现有结构；仅包含对象定义时适用。' },
  'truncate-table': { label: '清空表', description: '导入数据前清空目标表，影响原有全部数据；与删除表数据互斥。' },
  'delete-from-table': { label: '删除表数据', description: '导入数据前删除目标表数据；与清空表互斥，不接受任意 WHERE 条件。' },
  yes: { label: '非交互确认', description: '执行前等价确认只能由服务端绑定冻结快照派生，本地阅读状态不产生该参数。' },
  ...Object.fromEntries(Object.entries(importObjectLabels).map(([name, label]) => [name, { label: label + '表达式', description: `选择输入文件中匹配表达式的${label}；仅当前内容与兼容模式适用的对象参与配置。` }])),
  ...Object.fromEntries(importExecutionFields.map(field => [field.option, { label: field.label, description: field.description }])),
}

export const normalImportParameterCatalog = groups.flatMap(group => group.names.split(' ').map(name => ({ name, step: group.step, placement: group.placement, reason: group.reason, ...details[name]! })))
  .sort((a, b) => a.name.localeCompare(b.name))

export function normalImportParameterInfo(name: string) { return details[name] }

const formatConditions: Record<string, readonly string[]> = {
  csv: ['CSV'], cut: ['CUT'], sql: ['SQL'],
  'column-quote': ['CSV'], 'column-separator': ['CSV'], 'column-splitter': ['CUT'],
  'line-separator': ['CSV', 'CUT', 'SQL'], 'escape-character': ['CSV', 'CUT'],
  'skip-header': ['CSV', 'CUT'], 'skip-footer': ['CUT'], 'ignore-escape': ['CUT'], 'trail-delimiter': ['CUT'],
  'with-trim': ['CSV', 'CUT'], 'null-string': ['CSV', 'CUT', 'SQL'], 'empty-string': ['CSV', 'CUT'],
  compress: ['CSV', 'CUT', 'SQL'], 'compression-algo': ['CSV', 'CUT', 'SQL'], 'compression-level': ['CSV', 'CUT', 'SQL'],
  'date-format': ['CSV', 'CUT'], 'default-date': ['CSV', 'CUT'], 'auto-column-mapping': ['CSV'],
}

export function normalImportParameterRows(form: NormalImportForm, compatibilityMode?: string) {
  const traits = importTraits(form)
  return normalImportParameterCatalog.map(entry => {
    const execution = importExecutionFields.find(field => field.option === entry.name)
    let reason = entry.reason
    if (formatConditions[entry.name] && !formatConditions[entry.name]!.includes(traits.format)) reason = `当前非活动；适用格式：${formatConditions[entry.name]!.join(' / ')}。`
    if ((entry.name.startsWith('nls-') || ['role', 'sequence', 'synonym', 'type', 'type-body', 'package', 'package-body'].includes(entry.name)) && compatibilityMode !== 'ORACLE') reason = '当前非活动；仅适用于 Oracle 兼容模式。'
    if (['date-format', 'default-date', 'compat-mode'].includes(entry.name) && compatibilityMode !== 'MYSQL') reason = '当前非活动；仅适用于 MySQL 兼容模式。'
    if ((entry.name === 'ddl' || entry.name in importObjectLabels && entry.name !== 'table' || ['replace-object', 'compat-mode'].includes(entry.name)) && !traits.hasDdl) reason = '当前非活动；需要包含对象定义。'
    if ((execution?.dataOnly || ['replace-data', 'truncate-table', 'delete-from-table'].includes(entry.name)) && !traits.hasData) reason = '当前非活动；需要包含数据的导入内容。'
    if ((formatConditions[entry.name] || ['ctl-path', 'include-column-names', 'exclude-column-names', 'exclude-data-types'].includes(entry.name)) && !traits.structuredData) reason = '当前非活动；需要选择独立的数据格式。'
    const fields = traits.format ? form.formats[traits.format] : undefined
    if (fields && ['include-column-names', 'exclude-column-names'].includes(entry.name) && fields.mapping !== 'EXPLICIT') reason = '当前非活动；需要选择显式列名单映射。'
    if (fields && entry.name === 'ctl-path' && fields.mapping !== 'CONTROL') reason = '当前非活动；需要选择控制文件映射。'
    if (fields && ['compression-algo', 'compression-level'].includes(entry.name) && traits.compressedInput && !fields.compress) reason = '当前非活动；需要启用压缩输入。'
    if (fields && entry.name === 'compression-level' && ['gzip', 'snappy'].includes(fields.compressionAlgo)) reason = '当前算法不使用压缩等级，原等级已清除。'
    return { ...entry, placement: entry.name === 'database' ? 'input' : entry.placement, reason }
  })
}
