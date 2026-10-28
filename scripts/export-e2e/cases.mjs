// 用例只构造产品 DTO；所有命令与秘密注入仍由平台的唯一生成器和 Agent 完成。
const rows = []
const add = (id, area, title, patch = {}, expected = 'SUCCEEDED', extra = {}) => rows.push({ id, area, title, patch, expected, ...extra })
const reject = (id, title, patch) => add(id, '输入与能力边界', title, patch, 'API_REJECTED')

add('MX-001', '主链路', '单表 CSV：真实预检、任务领取、终态与完整内容')
add('MX-001-LOG', '主链路', '日志目录与输出目录分离的真实 CSV 主链路', { logPath: true })
add('MX-002', '主链路', '相同业务参数在新输出目录的内容一致', {}, 'SUCCEEDED', { baseline: 'MX-001-LOG' })
add('MX-003', '范围', '多个表的数据导出', { tables: ['rich', 'empty'] })
add('MX-004', '范围', '独立夹具库整库导出', { scope: 'ALL' })
add('MX-005', '范围', '整库排除指定表', { scope: 'ALL', excludeTables: ['empty', 'zeros', 'slow'] })
add('MX-006', '范围', '查询结果单条 SELECT 与列别名', { query: 'SELECT id, ascii_label AS label FROM {database}.rich WHERE id <= 2 ORDER BY id' })
add('MX-006-MULTILINE', '范围', '正常多行 SELECT 经领取、启动与限行导出', { query: 'SELECT\n id, ascii_label\nFROM {database}.rich\nWHERE id <= 2\nORDER BY id' })
add('MX-006-LONG', '范围', '超过普通 4096 字节参数上限的合法 SELECT', { query: `SELECT ${' '.repeat(4500)}id, ascii_label FROM {database}.rich ORDER BY id` })
add('MX-007', '范围', '查询结果条数上限', { query: 'SELECT id, ascii_label FROM {database}.rich ORDER BY id', queryLimit: 2 })
add('MX-008', '范围', '查询结果空集合', { query: 'SELECT id, ascii_label FROM {database}.rich WHERE id < 0', retainEmptyFiles: true })
add('MX-009', '范围', '查询 SQL 的执行错误由真实工具结束为失败', { query: 'SELECT missing_column FROM {database}.rich' }, 'TASK_FAILED')
add('MX-010', '范围', '指定表仅 DDL', { content: 'DDL_ONLY' }, 'SUCCEEDED', { validator: 'DDL' })
for (const format of ['CSV', 'CUT', 'SQL']) {
  add(`MX-011-${format}`, '范围', `表 DDL + ${format} 数据`, { content: 'DDL_AND_DATA', format })
}
for (const [kind, name] of [['VIEW', 'v_rich'], ['FUNCTION', 'f_identity'], ['PROCEDURE', 'p_identity'], ['SEQUENCE', 's_counter']]) {
  add(`MX-012-${kind}`, '对象类型', `${kind} 的定义导出`, { objectType: kind, object: name, content: 'DDL_ONLY' }, 'SUCCEEDED', { requires: [kind], validator: 'DDL' })
}
add('MX-013', '对象类型', '表和视图的结构组合导出', { objects: [['TABLE', 'rich'], ['VIEW', 'v_rich']], content: 'DDL_ONLY' }, 'SUCCEEDED', { requires: ['VIEW'], validator: 'DDL' })
reject('MX-014', '非表纯数据内容不得进入任务', { objectType: 'VIEW', object: 'v_rich' })
reject('MX-015', '指定范围不允许空对象', { tables: [] })
reject('MX-016', '对象类型白名单拒绝 TRIGGER', { objectType: 'TRIGGER' })
reject('MX-017', '非生产测试不创建多语句 SQL 执行任务', { query: 'SELECT 1; DELETE FROM {database}.rich' })
reject('MX-018', '非 SELECT 的结果集输入拒绝', { query: 'DELETE FROM {database}.rich' })
reject('MX-019', '结果集与显式对象互斥', { query: 'SELECT 1', forceObjects: true })

add('MX-020', 'CSV', 'NULL、空字符串、逗号、引号、反斜杠、换行、Unicode 的精确内容')
add('MX-021', 'CSV', '无列头', { csv: { skipHeader: true } })
add('MX-022', 'CSV', '自定义分隔符与包围字符', { csv: { columnSeparator: '|', columnQuote: "'" } })
add('MX-023', 'CSV', '全部字段引用模式', { csv: { columnQuoteMode: 'all' } })
add('MX-024', 'CSV', '按需引用模式', { csv: { columnQuoteMode: 'minimal' } })
add('MX-025', 'CSV', '非数字引用模式', { csv: { columnQuoteMode: 'non_numeric' } })
add('MX-025-ALL-NOT-NULL', 'CSV', '全部非空值引用模式', { csv: { columnQuoteMode: 'all_not_null' } })
add('MX-025-NONE', 'CSV', 'none 模式配合必要转义字符', { include: ['id', 'ascii_label'], csv: { columnQuoteMode: 'none', escapeCharacter: '\\' } })
add('MX-026', 'CSV', '自定义转义字符', { csv: { escapeCharacter: '\\' } })
add('MX-027', 'CSV', 'CRLF 行分隔符与字段内换行', { csv: { lineSeparator: String.raw`\r\n` } })
add('MX-028', 'CSV', '自定义 NULL 标记与空字符串区分', { csv: { nullString: '[NULL]' } })
add('MX-029', 'CSV', '去除字段首尾空白', { csv: { withTrim: true } })
add('MX-030', 'CUT', '定长分隔字符串与特殊值', { format: 'CUT', cut: { columnSplitter: '||' } })
add('MX-031', 'CUT', 'trail-delimiter 的实际方向与内容', { format: 'CUT', cut: { columnSplitter: '|', trailDelimiter: true } }, 'OBSERVED')
add('MX-032', 'CUT', '删除字段内换行', { format: 'CUT', cut: { columnSplitter: '|', removeNewline: true } })
add('MX-033', 'SQL', 'INSERT SQL 的行数、字面值与转义', { format: 'SQL' }, 'SUCCEEDED', { validator: 'SQL' })
add('MX-034', 'SQL', 'SQL 行分隔符与编码', { format: 'SQL', csv: { lineSeparator: String.raw`\r\n`, fileEncoding: 'UTF-8' } }, 'SUCCEEDED', { validator: 'SQL' })
for (const encoding of ['UTF-8', 'GBK', 'GB2312', 'GB18030', 'Big5', 'ISO-8859-1', 'UTF-16', 'UTF-32', 'US-ASCII']) {
  add(`MX-035-${encoding}`, '编码', `${encoding} 的受控 ASCII 数据与实际字节解码`, { include: ['id', 'ascii_label'], csv: { fileEncoding: encoding } })
}
for (const format of ['CSV', 'CUT', 'SQL']) {
  add(`MX-036-${format}`, '空表', `${format} 保留空文件`, { format, table: 'empty', retainEmptyFiles: true }, 'SUCCEEDED', { validator: 'EMPTY' })
  add(`MX-037-${format}`, '空表', format === 'CSV' ? `${format} 默认空表输出事实` : `${format} 默认空表无产物时失败关闭`, { format, table: 'empty' }, format === 'CSV' ? 'SUCCEEDED' : 'TASK_FAILED_NO_OUTPUT', { validator: 'EMPTY' })
}

add('MX-040', '筛选', 'where 过滤产生精确子集', { where: 'id <= 2' })
add('MX-041', '筛选', '包含列产生精确列集合', { include: ['id', 'ascii_label'] })
add('MX-042', '筛选', '排除列产生精确列集合', { exclude: ['notes'] })
add('MX-043', '筛选', '排除生成列', { table: 'generated_values', excludeVirtualColumns: true }, 'SUCCEEDED', { requires: ['GENERATED'] })
add('MX-044', '筛选', '一级分区导出精确行集合', { table: 'partitioned', partition: 'p0' }, 'SUCCEEDED', { requires: ['PARTITION'] })
add('MX-045', '筛选', '复合表仅子分区可导出', { table: 'subpartitioned', partition: 'sp00' }, 'SUCCEEDED', { requires: ['SUBPARTITION'] })
add('MX-046', '筛选', '复合表父分区预检查失败', { table: 'subpartitioned', partition: 'p0' }, 'PRECHECK_FAILED', { requires: ['SUBPARTITION'], failedCheck: 'OBJECT_ACCESS' })
add('MX-047', '筛选', '不存在的列预检失败，连接事实保留', { include: ['missing_column'] }, 'PRECHECK_FAILED', { failedCheck: 'OBJECT_ACCESS' })
add('MX-048', '筛选', '列名大小写不一致预检失败', { include: ['ASCII_LABEL'] }, 'PRECHECK_FAILED', { failedCheck: 'OBJECT_ACCESS' })
add('MX-049', '筛选', '不存在的分区预检失败', { table: 'partitioned', partition: 'missing_partition' }, 'PRECHECK_FAILED', { requires: ['PARTITION'], failedCheck: 'OBJECT_ACCESS' })
add('MX-050', '筛选', '不存在的表预检失败', { table: 'missing_table' }, 'PRECHECK_FAILED', { failedCheck: 'OBJECT_ACCESS' })
add('MX-051', '筛选', '排除数据类型', { table: 'types', excludeDataTypes: ['BLOB'] })
reject('MX-052', '包含列与排除列互斥', { include: ['id'], exclude: ['ascii_label'] })
reject('MX-053', '重复列拒绝进入固定元数据检查', { include: ['id', 'id'] })
reject('MX-054', 'where 与结果集查询互斥', { query: 'SELECT id FROM {database}.rich', where: 'id > 0' })
reject('MX-055', 'partition 与结果集查询互斥', { query: 'SELECT id FROM {database}.rich', partition: 'p0' })
reject('MX-056', '列筛选通配符拒绝', { include: ['*'] })

for (const [key, value] of [['dateValueFormat', 'yyyy/MM/dd'], ['timeValueFormat', 'HH-mm-ss'], ['datetimeValueFormat', 'yyyy/MM/dd HH:mm:ss'], ['timestampValueFormat', 'yyyy/MM/dd HH:mm:ss']]) {
  add(`MX-060-${key}`, '日期时间', `${key} 的实际序列化`, { table: 'temporal', timestampFormats: { [key]: value } }, 'SUCCEEDED', { validator: 'TEMPORAL' })
}
add('MX-061', '日期时间', '所有 MySQL 值格式组合', { table: 'temporal', timestampFormats: { dateValueFormat: 'yyyy/MM/dd', timeValueFormat: 'HH-mm-ss', datetimeValueFormat: 'yyyy/MM/dd HH:mm:ss', timestampValueFormat: 'yyyy/MM/dd HH:mm:ss' } }, 'SUCCEEDED', { validator: 'TEMPORAL' })
add('MX-061-CUT', '日期时间', 'CUT 的全部 MySQL 日期时间格式', { table: 'temporal', format: 'CUT', timestampFormats: { dateValueFormat: 'yyyy/MM/dd', timeValueFormat: 'HH-mm-ss', datetimeValueFormat: 'yyyy/MM/dd HH:mm:ss', timestampValueFormat: 'yyyy/MM/dd HH:mm:ss' } }, 'SUCCEEDED', { validator: 'TEMPORAL' })
add('MX-062', '日期时间', '保留零日期时间的实际值', { table: 'zeros', timestampFormats: { preserveZeroDatetime: true } }, 'SUCCEEDED', { requires: ['ZERO_DATETIME'], validator: 'ZERO' })
add('MX-062-CUT', '日期时间', 'CUT 保留零日期时间', { table: 'zeros', format: 'CUT', timestampFormats: { preserveZeroDatetime: true } }, 'SUCCEEDED', { requires: ['ZERO_DATETIME'], validator: 'ZERO' })
add('MX-063', '日期时间', '默认零日期行为单独取证', { table: 'zeros' }, 'OBSERVED', { requires: ['ZERO_DATETIME'], validator: 'ZERO_DEFAULT' })
for (const key of ['timestampTzValueFormat', 'timestampLtzValueFormat', 'nlsDateFormat', 'nlsTimestampFormat', 'nlsTimestampTzFormat']) {
  reject(`MX-064-${key}`, 'MySQL 拒绝 Oracle 专属日期时间参数', { timestampFormats: { [key]: 'YYYY-MM-DD' } })
}
reject('MX-065', '日期格式中的控制字符拒绝', { timestampFormats: { dateValueFormat: 'yyyy\nMM' } })
reject('MX-066', '仅 DDL 不接受日期值格式', { content: 'DDL_ONLY', timestampFormats: { dateValueFormat: 'yyyy-MM-dd' } })

add('MX-070', '读取与结构', '快照一致性读取', { snapshot: true })
add('MX-071', '读取与结构', '弱读固定预检查及真实输出', { weakRead: true })
add('MX-072', '读取与结构', '无主键表隐藏主键读取', { table: 'nopk', enableHiddenPk: true }, 'SUCCEEDED', { validator: 'ROWS', requires: ['HIDDEN_PK_READABLE'] })
add('MX-073', '读取与结构', '附加表定义信息与 sys 槽位', { content: 'DDL_ONLY', addExtraMessage: true }, 'SUCCEEDED', { requires: ['SYS'], validator: 'DDL_EXTRA' })
add('MX-073-NO-SYS', '读取与结构', '缺少 sys 凭据时附加定义信息失败关闭', { content: 'DDL_ONLY', addExtraMessage: true }, 'API_REJECTED', { requires: ['NO_SYS'] })
add('MX-074', '读取与结构', '前置 DROP 的定义输出', { content: 'DDL_ONLY', dropObject: true }, 'SUCCEEDED', { validator: 'DDL_DROP' })
add('MX-075', '读取与结构', '保留 Schema 前缀的定义', { content: 'DDL_ONLY', retainSchema: true }, 'SUCCEEDED', { validator: 'DDL_SCHEMA' })
add('MX-076', '读取与结构', '紧凑原生建表定义', { content: 'DDL_ONLY', compactSchema: true }, 'SUCCEEDED', { validator: 'DDL_COMPACT' })
for (const policy of ['PRESERVE', 'RESTART']) {
  add(`MX-077-${policy}`, '读取与结构', `序列策略 ${policy} 的定义结果`, { objectType: 'SEQUENCE', object: 's_counter', content: 'DDL_ONLY', sequencePolicy: policy }, 'SUCCEEDED', { requires: ['SEQUENCE'], validator: 'SEQUENCE' })
}
add('MX-078', '读取与结构', '指定 SCN 快照', { flashbackScn: '{scn}' }, 'SUCCEEDED', { requires: ['SCN'] })
reject('MX-079', '快照与闪回 SCN 互斥', { snapshot: true, flashbackScn: '1' })
reject('MX-080', 'MySQL 不接受 Oracle 闪回时间', { flashbackTimestamp: '2026-01-01 00:00:00' })
reject('MX-081', '结果集不接受隐藏主键读取', { query: 'SELECT id FROM {database}.rich', enableHiddenPk: true })
reject('MX-082', '普通表不得声明序列策略', { content: 'DDL_ONLY', sequencePolicy: 'RESTART' })

add('MX-090', '文件与资源', '自定义节点本地日志目录', { logPath: true })
add('MX-091', '文件与资源', '输出不生成多级目录', { noNestedDir: true })
add('MX-092', '文件与资源', '按行拆分与全部文件内容合并校验', { table: 'bulk', blockSize: '256ROW' })
add('MX-093', '文件与资源', '按 MB 拆分', { table: 'bulk', blockSize: '1MB' })
add('MX-094', '文件与资源', '进程导出总量上限允许停止且不冒充拆分', { table: 'bulk', maxFileSize: 32768 })
add('MX-095', '文件与资源', '线程、分页与 JVM 堆的实际启动', { thread: 2, pageSize: 100, jvmMemory: '512M' })
add('MX-096', '文件与资源', '每线程宏块数', { parallelMacro: 1 })
add('MX-097', '文件与资源', '静态额度不足不压掉数据库结果', { jvmMemory: '2048G' }, 'PRECHECK_FAILED', { failedCheck: 'NODE_RESOURCE' })
add('MX-098', '文件与资源', '节点根目录外的输出被阻断', { outsideRoot: true }, 'PRECHECK_FAILED', { failedCheck: 'OUTPUT_PATH' })
add('MX-099', '文件与资源', '合法输出与非法日志目录分别取证', { outsideLogRoot: true }, 'PRECHECK_FAILED', { failedCheck: 'OUTPUT_PATH' })
add('MX-100', '文件与资源', '非空目录失败，数据库结果独立通过', { nonempty: true }, 'PRECHECK_FAILED', { failedCheck: 'OUTPUT_EMPTY' })
add('MX-101', '文件与资源', '仅跳过专用目录空性', { nonempty: true, skipCheckDir: true })
reject('MX-102', '节点平台与路径形态不匹配', { invalidPlatformPath: true })
reject('MX-103', '线程非正整数拒绝', { thread: 0 })
reject('MX-104', '非法 JVM 单位拒绝', { jvmMemory: '4XYZ' })
reject('MX-105', 'MySQL 拒绝 Oracle fetch-size', { fetchSize: 10 })
reject('MX-106', '无效文件拆分单位拒绝', { blockSize: '1GB' })
for (const [algo, level] of [['zstd', 3], ['zlib', 6], ['gzip', undefined], ['snappy', undefined]]) {
  add(`MX-110-${algo}`, '压缩', `${algo} 导出及解压内容验证`, { compress: true, compressionAlgo: algo, compressionLevel: level }, 'SUCCEEDED', { validator: 'COMPRESSED' })
}
reject('MX-111', '压缩算法白名单拒绝', { compress: true, compressionAlgo: 'unsupported' })
reject('MX-112', '压缩等级越界拒绝', { compress: true, compressionAlgo: 'gzip', compressionLevel: 99 })
reject('MX-113', 'gzip 不接受压缩等级', { compress: true, compressionAlgo: 'gzip', compressionLevel: 6 })
reject('MX-114', 'CSV 分隔符只接受单字符', { csv: { columnSeparator: '||' } })
reject('MX-115', 'CSV 包围符只接受单字符', { csv: { columnQuote: "''" } })
reject('MX-116', 'CSV 转义符只接受单字节', { csv: { escapeCharacter: 'xx' } })
reject('MX-117', 'NULL 标记控制字符拒绝', { csv: { nullString: 'NULL\n' } })
reject('MX-118', 'SQL 不接受 CSV 专属参数', { format: 'SQL', csv: { skipHeader: true } })
reject('MX-118-NONE-ESCAPE', 'NONE 包围模式缺少必要转义字符拒绝', { csv: { columnQuoteMode: 'none' } })
for (const format of ['POS', 'PARQUET', 'ORC', 'AVRO']) add(`MX-119-${format}`, '能力门禁', `${format} 普通 V1 界面保持门控`, { format }, 'WORKFLOW', { workflow: 'FORMAT_GATE' })

for (const [id, title, workflow] of [
  ['MX-120', '五步页面与真实对象目录', 'UI'], ['MX-121', '修改配置使旧预检失效', 'INVALIDATION'],
  ['MX-122', '保存草稿、预检、提交各自的幂等', 'IDEMPOTENCY'], ['MX-123', '过期预检不能提交', 'EXPIRY'],
  ['MX-124', '重复点击提交不创建第二任务', 'SUBMIT_REPLAY'], ['MX-125', '排队取消与一致终态', 'CANCEL_QUEUED'],
  ['MX-126', '运行取消与受控进程终止', 'CANCEL_RUNNING'], ['MX-127', '无检查点失败任务拒绝继续', 'CHECKPOINT_NEGATIVE'],
  ['MX-128', '有真实检查点的失败任务继续', 'CHECKPOINT_POSITIVE'], ['MX-129', '按原配置新建与从头重跑', 'DERIVE'],
  ['MX-130', '成功任务保存模板再建草稿', 'TEMPLATE'], ['MX-131', '实时日志、下载与两层脱敏', 'LOGS'],
  ['MX-132', '秘密材料与进程结束后清理', 'SECRET_CLEANUP'], ['MX-133', 'Agent 断联恢复与至少一次投递', 'AGENT_RECOVERY'],
  ['MX-134', '未登录/CSRF/越权 API 失败关闭', 'AUTHORIZATION'], ['MX-135', '四档视口、键盘、完整命令和统一摘要', 'ACCESSIBILITY'],
  ['MX-136', '保存草稿刷新后读取开关与日期格式不丢失', 'DRAFT_ROUNDTRIP'],
]) add(id, '生命周期与安全', title, {}, 'WORKFLOW', { workflow })

for (const [id, title, patch] of [
  ['MX-140', '结果集条数上限非正数', { query: 'SELECT id FROM {database}.rich', queryLimit: 0 }],
  ['MX-141', '结果集 SQL 文件引用拒绝', { query: 'file:///fixture.sql' }],
  ['MX-142', '结果集不接受列筛选', { query: 'SELECT id FROM {database}.rich', include: ['id'] }],
  ['MX-143', '仅 DDL 不接受读取选项', { content: 'DDL_ONLY', weakRead: true }],
  ['MX-144', 'SQL 不接受日期值格式', { format: 'SQL', timestampFormats: { timeValueFormat: 'HH:mm:ss' } }],
  ['MX-145', '普通文本格式不接受 POS 控制目录', { controlFilePath: true }],
  ['MX-147', '分页非正整数拒绝', { pageSize: 0 }],
  ['MX-148', '文件阈值非正整数拒绝', { maxFileSize: 0 }],
  ['MX-149', '压缩算法未开启压缩时拒绝', { compressionAlgo: 'gzip' }],
]) reject(id, title, patch)
add('MX-146', '文件与资源', '现行元数据允许本地格式携带节点临时目录', { tmpPath: true })
for (const [algo, levels] of [['zstd', [1, 22]], ['zlib', [-1, 9]]]) {
  for (const level of levels) add(`MX-150-${algo}-${level}`, '压缩', `${algo} 压缩等级边界 ${level}`, { include: ['id', 'ascii_label'], compress: true, compressionAlgo: algo, compressionLevel: level }, 'SUCCEEDED', { validator: 'COMPRESSED' })
}

export const mysqlExportCases = Object.freeze(rows.map(row => Object.freeze(row)))

export function buildConfig(test, context) {
  const p = test.patch
  // 本机读文件使用 Windows 原生路径，提交给现役 OBDUMPER 接口的输入按节点已登记格式填写。
  const toolPath = value => /^[A-Za-z]:[\\/]/.test(value ?? '') ? `/${value.replaceAll('\\', '/')}` : value
  const config = {
    objectScope: { database: context.database, scopeKind: p.scope ?? (p.query ? 'QUERY_RESULT' : 'SPECIFIED'), objectTypes: [p.objectType ?? 'TABLE'], expressions: (p.tables ?? [p.object ?? p.table ?? 'rich']).map(name => ({ name, objectType: p.objectType ?? 'TABLE' })) },
    contentSelection: { contentKind: p.content ?? 'DATA_ONLY' },
    dataFormat: { formatKind: p.format ?? 'CSV' },
    outputConfig: { outputKind: 'LOCAL', filePath: toolPath(context.outputPath), ...(test.id === 'MX-001' ? {} : { logPath: toolPath(context.logPath) }) },
    performanceConfig: p.content === 'DDL_ONLY' ? {} : { jvmMemory: p.jvmMemory ?? '512M', thread: p.thread ?? 1 },
  }
  if (p.objects) { config.objectScope.expressions = p.objects.map(([objectType, name]) => ({ objectType, name })); config.objectScope.objectTypes = [...new Set(p.objects.map(([type]) => type))] }
  if (p.scope === 'ALL' || p.query && !p.forceObjects) { delete config.objectScope.objectTypes; delete config.objectScope.expressions }
  if (p.excludeTables) config.objectScope.excludeTables = p.excludeTables
  if (p.content === 'DDL_ONLY' && !p.timestampFormats) delete config.dataFormat
  else {
    if (p.csv) config.dataFormat.csvOptions = p.csv
    if (p.cut) {
      const { columnSplitter, ...options } = p.cut
      config.dataFormat.cutOptions = options
      if (columnSplitter) config.dataFormat.csvOptions = { ...config.dataFormat.csvOptions, columnSplitter }
    }
    if (p.timestampFormats) config.dataFormat.timestampFormats = p.timestampFormats
  }
  const filter = {}
  for (const key of ['where', 'partition', 'snapshot', 'weakRead', 'enableHiddenPk', 'excludeVirtualColumns', 'excludeDataTypes', 'flashbackTimestamp']) if (p[key] !== undefined) filter[key] = p[key]
  if (p.flashbackScn) filter.flashbackScn = Number(p.flashbackScn === '{scn}' ? context.scn : p.flashbackScn)
  if (p.include) filter.includeColumnNames = p.include
  if (p.exclude) filter.excludeColumnNames = p.exclude
  if (p.query) filter.querySql = p.query.replaceAll('{database}', context.database)
  if (p.query) filter.queryResultLimit = p.queryLimit ?? 10000
  if (Object.keys(filter).length) config.filterConfig = filter
  for (const key of ['retainEmptyFiles', 'noNestedDir', 'maxFileSize', 'compress', 'compressionAlgo', 'skipCheckDir']) if (p[key] !== undefined) config.outputConfig[key] = p[key]
  if (p.compressionLevel !== undefined) config.outputConfig.compressionLevel = p.compressionLevel
  if (p.logPath) config.outputConfig.logPath = toolPath(context.logPath)
  if (p.controlFilePath) config.outputConfig.controlFilePath = toolPath(context.outputPath)
  if (p.tmpPath) config.outputConfig.tmpPath = toolPath(context.outputPath)
  if (p.outsideRoot) config.outputConfig.filePath = toolPath(context.disallowedPath)
  if (p.outsideLogRoot) config.outputConfig.logPath = toolPath(context.disallowedPath)
  if (p.invalidPlatformPath) config.outputConfig.filePath = '/tmp/odo_e2e_invalid'
  for (const key of ['blockSize', 'pageSize', 'parallelMacro', 'fetchSize']) if (p[key] !== undefined) config.performanceConfig[key] = p[key]
  const ddl = {}
  for (const key of ['dropObject', 'retainSchema', 'compactSchema', 'addExtraMessage', 'sequencePolicy']) if (p[key] !== undefined) ddl[key] = p[key]
  if (Object.keys(ddl).length) config.ddlBehavior = ddl
  return config
}
