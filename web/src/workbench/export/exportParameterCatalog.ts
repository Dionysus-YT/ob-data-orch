import type { ExportContentKind, ExportDataFormatKind, ExportOutputKind, ExportScopeKind } from '@/api/browser'

// 目录只登记本轮新建适用参数；连接与秘密项仅说明受管来源，不持有或展示其值。
export interface ExportParameterEntry { readonly name: string; readonly label: string; readonly description: string; readonly step: number }
export const exportParameterCatalog: readonly ExportParameterEntry[] = [
  {"name":"add-extra-message","label":"附加表定义信息","description":"在导出的表定义中附加表组名称等信息。使用数据源已配置的 sys 凭据，并在预检查中复核权限。","step":3},
  {"name":"all","label":"整库导出","description":"选择当前数据库全部适用对象；与具体对象表达式互斥。","step":2},
  {"name":"block-size","label":"单个文件拆分阈值","description":"达到阈值后拆分为多个文件，不限制总导出量。MB 模式填正整数或整数MB，ROW 模式使用整数ROW；未设置时继承工具默认。","step":3},
  {"name":"column-quote","label":"文本识别符","description":"用于包围 CSV 字段内容的单字符，默认单引号。与包围模式共同控制字段输出，不支持多个字符。","step":3},
  {"name":"column-quote-mode","label":"包围模式","description":"控制 CSV 字段的包围规则：全部、非空、最小必要、非数字或不包围。默认 non_numeric；不包围时须确认特殊字符已正确处理。","step":3},
  {"name":"column-separator","label":"字段分隔符","description":"指定 CSV 字段之间的单字符分隔符，默认逗号。仅支持单字符；空格和制表符按选择值保留。","step":3},
  {"name":"column-splitter","label":"字段分隔符","description":"指定 CUT 字段之间的分隔字符串，当前最长 256 个字符。不同于 CSV，可配置多字符分隔；留空继承工具默认。","step":3},
  {"name":"compact-schema","label":"使用原生建表语句","description":"通过 SHOW CREATE TABLE 获取表定义，而非查询系统视图后反拼 DDL。未勾选时继承工具默认策略，不表示显式关闭。","step":3},
  {"name":"compress","label":"启用压缩","description":"压缩导出的 CSV/CUT/SQL 数据文件，以减小文件体积。启用后可指定算法与等级；关闭时清除算法和等级。","step":3},
  {"name":"compression-algo","label":"压缩算法","description":"指定导出压缩算法，可选 zstd、zlib、gzip、snappy。需启用压缩；自动选择 zstd，gzip/snappy 不支持指定压缩等级。","step":3},
  {"name":"compression-level","label":"压缩等级","description":"指定压缩算法的等级：zstd 支持 1~22，zlib 支持 -1~9。gzip/snappy 不提供等级；不填写时使用算法默认等级。","step":3},
  {"name":"csv","label":"CSV 数据格式","description":"以 CSV 格式导出表数据，按当前适用的文本参数配置。","step":3},
  {"name":"cut","label":"CUT 数据格式","description":"以 CUT 格式导出数据，支持独立的字符串分隔符。","step":3},
  {"name":"database","label":"数据库 / Schema","description":"选择目标数据库目录或填写明确名称；访问资格仍需完整预检查确认。","step":2},
  {"name":"date-value-format","label":"DATE 值格式","description":"设置 CSV/CUT 的 DATE 值格式，例如 yyyy-MM-dd。Oracle DATE 可以包含时分秒；此字段不设置 NLS 会话格式。","step":3},
  {"name":"datetime-value-format","label":"DATETIME 值格式","description":"设置 MySQL CSV/CUT 中 DATETIME 值的文本格式，不改变源字段类型；留空继承工具默认。","step":3},
  {"name":"ddl","label":"导出结构","description":"导出对象定义；包含结构与数据时还需选择一种数据格式。","step":2},
  {"name":"drop-object","label":"前置 DROP","description":"在导出的 DDL 中，先添加删除同名对象的语句，再生成创建语句。执行该 DDL 可能删除目标对象及其数据；导出过程不删除源对象。","step":3},
  {"name":"enable-hidden-pk","label":"隐藏主键加速","description":"利用隐藏主键 __pk_increment 加速无主键表的读取，不将该列加入导出文件。预检查复核版本、表结构与读取权限；4.0 之前需要官方指定的内部账号。","step":3},
  {"name":"escape-character","label":"转义字符","description":"指定 CSV/CUT 用来转义特殊字符的字符。当前仅接受单个 ASCII 字符，不允许换行或 NUL；未填写时继承该格式默认。","step":3},
  {"name":"exclude-column-names","label":"排除列","description":"不导出指定列，多个列名以逗号分隔；与包含列互斥。","step":2},
  {"name":"exclude-data-types","label":"排除数据类型","description":"不导出指定类型的列，多个类型名以逗号分隔；仅包含数据时生效。","step":2},
  {"name":"exclude-table","label":"排除表","description":"排除指定表，多个表名以逗号分隔；仅在当前对象范围支持时生效。","step":2},
  {"name":"exclude-virtual-columns","label":"排除生成列","description":"导出表数据时排除生成列的值；不删除源表列，也不改变导出的表结构定义。","step":3},
  {"name":"fetch-size","label":"游标抓取行数（Oracle）","description":"设置 JDBC 游标每次抓取的行数，仅 Oracle 兼容模式适用。","step":4},
  {"name":"file-encoding","label":"文件编码","description":"指定导出文件的字符编码，与数据库连接字符集不同。留空继承工具默认 UTF-8；所选编码须由执行环境支持。","step":3},
  {"name":"file-path","label":"导出路径","description":"执行节点上的完整绝对输出路径；原样传递，不追加目录或转换平台格式。","step":4},
  {"name":"flashback-scn","label":"闪回 SCN","description":"指定用于导出数据的 SCN 事务点，仅接受正整数。与一致性快照及自定义查询互斥；留空不指定 SCN。","step":3},
  {"name":"flashback-timestamp","label":"闪回时间点","description":"指定闪回读取的时间点，仅 Oracle 兼容模式可用。与一致性快照及自定义查询互斥；留空不指定时间点。","step":3},
  {"name":"function","label":"函数","description":"指定导出的函数定义，属于结构内容。","step":2},
  {"name":"host","label":"连接主机","description":"来自已有数据源的受管连接地址，不在任务中重复编辑。","step":1},
  {"name":"include-column-names","label":"包含列","description":"只导出指定列，多个列名以逗号分隔；与排除列互斥。","step":2},
  {"name":"line-separator","label":"换行符号","description":"设置数据文件中记录之间的行分隔符，例如 LF、CRLF 或 CR。未指定时继承平台默认；不同于删除字段内容中的换行。","step":3},
  {"name":"log-path","label":"日志路径","description":"指定工具日志目录；留空由 Agent 受控采集，填写后检查目录可写性。","step":4},
  {"name":"max-file-size","label":"导出总量上限（Byte）","description":"限制整个导出进程的输出总量，单位 Byte，达到上限后停止；不同于第三步单个文件拆分。","step":4},
  {"name":"mem","label":"JVM 内存","description":"设置运行工具的 JVM 内存，使用数字及 K/M/G/T 单位，例如 4G；留空继承默认。","step":4},
  {"name":"nls-date-format","label":"NLS DATE 格式","description":"设置 Oracle nls_date_format 会话变量，不直接指定导出 DATE 值格式。","step":3},
  {"name":"nls-timestamp-format","label":"NLS TIMESTAMP 格式","description":"设置 Oracle nls_timestamp_format 会话变量，不直接指定导出 TIMESTAMP 值格式。","step":3},
  {"name":"nls-timestamp-tz-format","label":"NLS TIMESTAMP TZ 格式","description":"设置 Oracle nls_timestamp_tz_format 会话变量，不直接指定导出 TZ 值格式。","step":3},
  {"name":"no-nested-dir","label":"扁平输出目录","description":"所有导出文件直接写入指定输出目录，不再创建子目录。","step":3},
  {"name":"null-string","label":"NULL 表示","description":"设置 CSV/CUT 中 NULL 的输出字符串，默认 \\N。NULL 与空字符串不同；显式输入的首尾空格按原值保留。","step":3},
  {"name":"page-size","label":"分页大小","description":"设置分页查询每次读取的行数；留空继承工具默认。","step":4},
  {"name":"parallel-macro","label":"每线程宏块数","description":"设置每个导出线程并行处理的宏块数；留空继承工具默认。","step":4},
  {"name":"partition","label":"分区筛选","description":"仅导出指定表的所选分区，多个分区以逗号分隔；与结果集查询互斥。","step":2},
  {"name":"password","label":"数据库凭据","description":"由短时安全上下文提供；目录不读取、展示或复制凭据。","step":1},
  {"name":"port","label":"连接端口","description":"来自已有数据源的 ODP 连接端口。","step":1},
  {"name":"preserve-zero-datetime","label":"保留零日期时间","description":"保留 MySQL DATE、DATETIME、TIMESTAMP 零值的原有表达方式。非空日期时间列读出的 NULL 导出为 0，可空列仍保留 NULL；零值不使用自定义日期时间格式。","step":3},
  {"name":"procedure","label":"存储过程","description":"指定导出的存储过程定义，属于结构内容。","step":2},
  {"name":"query-sql","step":2,"label":"结果集查询","description":"按结果集导出时输入单条 SELECT；默认最多 1000 条，语法、对象和权限由后续检查确认。"},
  {"name":"remove-newline","label":"移除回车换行","description":"删除 CUT 字段内容中的回车和换行字符，例如 \\r、\\n；不用于设置记录分隔符，不修改源表，但会改变导出文本。","step":3},
  {"name":"retain-empty-files","label":"保留空文件","description":"空表、空分区或条件筛选结果为空时仍生成文件。CSV 默认保留列名表头；不包含表头时生成完全空文件。","step":3},
  {"name":"retain-schema","label":"保留 Schema 前缀","description":"保留 schema.table 限定。MySQL 使用工具元数据重建路径，与原生建表语句互斥，可能需要系统视图读取权限。","step":3},
  {"name":"sequence","label":"序列","description":"指定导出的序列定义，属于结构内容。","step":2},
  {"name":"sequence-policy","label":"序列策略","description":"preserve 保留导出时的当前序列值；restart 使用序列原始定义的起始值。只随序列定义导出活动，不修改源序列。","step":2},
  {"name":"skip-check-dir","label":"跳过导出目录空性检查","description":"允许向已有文件的导出目录写入，可能覆盖同名文件；仍检查路径可写性与可用空间。","step":4},
  {"name":"skip-header","label":"包含列名表头","description":"勾选时保留 CSV 第一行列名；取消勾选时传入 --skip-header，跳过列名表头。","step":3},
  {"name":"snapshot","label":"一致性快照","description":"导出最近一次成功合并的基线数据，以获得全局一致性快照；不等同于当前最新数据。与闪回 SCN、闪回时间互斥。","step":3},
  {"name":"sql","label":"SQL 数据格式","description":"以官方 INSERT SQL 文件格式导出数据，不提供任意 SQL 执行入口。","step":3},
  {"name":"sys-password","label":"sys 凭据","description":"仅执行阶段的安全上下文使用，不能进入页面、草稿或命令展示。","step":1},
  {"name":"sys-user","label":"sys 用户","description":"来自已有数据源的受管 sys 连接身份，不在任务中重复输入。","step":1},
  {"name":"table","label":"表","description":"指定导出的表对象；只有表对象形成普通数据输出。","step":2},
  {"name":"thread","label":"导出线程","description":"设置并行导出线程数；留空继承工具默认，需结合节点资源和源库负载配置。","step":4},
  {"name":"time-value-format","label":"TIME 值格式","description":"设置 MySQL TIME 值的导出格式，默认 HH:mm:ss，精度继承源列定义。","step":3},
  {"name":"timestamp-ltz-value-format","label":"TIMESTAMP LTZ 值格式","description":"设置 Oracle TIMESTAMP WITH LOCAL TIME ZONE 的导出值格式。","step":3},
  {"name":"timestamp-tz-value-format","label":"TIMESTAMP TZ 值格式","description":"设置 Oracle TIMESTAMP WITH TIME ZONE 的导出值格式。","step":3},
  {"name":"timestamp-value-format","label":"TIMESTAMP 值格式","description":"设置 CSV/CUT 的 TIMESTAMP 值格式，精度仍受源列定义约束。","step":3},
  {"name":"tmp-path","label":"本地临时分块目录","description":"对象存储 Multipart 上传使用的执行节点本地目录；预检查需要明确路径以检查可写性与可用空间。","step":4},
  {"name":"trail-delimiter","label":"行尾分隔符处理","description":"控制 CUT 记录末尾的字段分隔符处理。官方 4.3.5 文档对保留或移除方向描述不一致，启用前需确认实际输出。","step":3},
  {"name":"user","label":"连接用户名","description":"来自已有数据源的连接身份，执行时由受管上下文提供。","step":1},
  {"name":"view","label":"视图","description":"指定导出的视图定义，不表示导出视图数据。","step":2},
  {"name":"weak-read","label":"备副本读取","description":"从备副本采用弱一致性读取，数据可能落后于主副本。预检查复核读取权限与副本可达性。","step":3},
  {"name":"where","label":"条件筛选","description":"按 WHERE 条件筛选指定表的数据；不填写 WHERE 关键字，与结果集查询互斥。","step":2},
  {"name":"with-trim","label":"去除首尾空格","description":"去除导出字段值首尾的空格；字段内的空格保持不变。此处理会改变导出值，不修改源表数据。","step":3},
]

export interface ExportParameterContext {
  readonly content: ExportContentKind | ''; readonly format: ExportDataFormatKind; readonly scope: ExportScopeKind
  readonly output: ExportOutputKind; readonly compressed: boolean; readonly algorithm: string; readonly mode?: string
}

const csvOnly = new Set(['skip-header', 'column-separator', 'column-quote', 'column-quote-mode'])
const cutOnly = new Set(['column-splitter', 'trail-delimiter', 'remove-newline'])
const textOnly = new Set(['escape-character', 'null-string', 'with-trim', 'date-value-format', 'time-value-format', 'datetime-value-format', 'preserve-zero-datetime'])
const ddlOnly = new Set(['drop-object', 'retain-schema', 'compact-schema', 'add-extra-message', 'sequence-policy'])
const oracleOnly = new Set(['flashback-timestamp', 'fetch-size', 'timestamp-tz-value-format', 'timestamp-ltz-value-format', 'nls-date-format', 'nls-timestamp-format', 'nls-timestamp-tz-format'])

// 适用性只提供目录说明，实际显隐、参数资格和提交门禁继续由各能力所有者决定。
export function exportParameterRows(context: ExportParameterContext) {
  return exportParameterCatalog.map(row => ({ ...row, reason: parameterReason(row, context) }))
}

function parameterReason(row: ExportParameterEntry, context: ExportParameterContext): string {
  const name = row.name, hasData = context.content !== 'DDL_ONLY', hasDdl = context.content !== 'DATA_ONLY'
  if (row.step === 1) return '由已有数据源和安全上下文提供；任务内不可编辑，目录不展示连接值或秘密。'
  if (!context.content) return '请先选择导出内容，再按格式、范围和兼容模式配置适用参数。'
  if (name === 'ddl') return hasDdl ? '随当前导出内容派生，不是独立结构执行入口。' : '当前仅导出数据，不生成对象定义。'
  if (['csv', 'cut', 'sql'].includes(name)) return hasData && context.format.toLowerCase() === name ? '当前数据格式；仍需完整预检查。' : '选择相应数据格式后活动；仅结构内容不使用数据格式。'
  if (name === 'query-sql') return context.scope === 'QUERY_RESULT' ? '当前按结果集导出；只接受单条 SELECT，并与普通对象筛选互斥。' : '选择按结果集导出后配置；不属于通用 SQL 执行能力。'
  if (name === 'tmp-path' && context.output === 'LOCAL') return '当前本地输出不使用对象存储分块目录。'
  if (name === 'skip-check-dir' && context.output !== 'LOCAL') return '仅本地输出适用；不能绕过可写性与空间检查。'
  if (ddlOnly.has(name) && !hasDdl) return '当前不包含结构，此参数不参与活动配置。'
  if (oracleOnly.has(name) && context.mode !== 'ORACLE') return '需要 Oracle 兼容模式及对应字段条件，当前不活动。'
  if (csvOnly.has(name) && (!hasData || context.format !== 'CSV')) return '仅 CSV 数据内容活动，当前格式不适用。'
  if (cutOnly.has(name) && (!hasData || context.format !== 'CUT')) return '仅 CUT 数据内容活动，当前格式不适用。'
  if (textOnly.has(name) && (!hasData || !['CSV', 'CUT'].includes(context.format))) return '仅适用的 CSV/CUT 数据内容活动；兼容模式与字段条件继续生效。'
  if (['compress', 'compression-algo', 'compression-level'].includes(name)) {
    if (!hasData) return '当前仅结构，不使用数据压缩设置。'
    if (name !== 'compress' && !context.compressed) return '启用压缩后配置，关闭压缩时不参与当前任务。'
    if (name === 'compression-level' && ['gzip', 'snappy'].includes(context.algorithm)) return 'gzip 与 snappy 不支持指定压缩等级。'
  }
  if (row.step === 2) return '按当前内容与对象范围配置；改变范围会使旧检查失效，实际资格仍需完整预检查。'
  return '在对应步骤的常用或高级设置中配置；格式、范围、兼容模式和权限条件继续生效，目录不代表功能已验证。'
}
