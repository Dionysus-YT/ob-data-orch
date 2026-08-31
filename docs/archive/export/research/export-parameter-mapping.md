# OBDUMPER V4.3.5 导出参数映射基线

> 文档状态：历史归档；参数研究与编号仅用于追溯，当前能力状态以 `docs/02-design/export-module.md`、`docs/03-technical/export-general-contract.md` 和运行时元数据为准
> 核验日期：2026-07-17；POS 实测定版：2026-08-07
> 发布包：`ob-loader-dumper-4.3.5-RELEASE.zip`
> 发布包 SHA-256：`C1A5D5EE053106803263015F00EC7F0B94B73EA4015A2E53CF1AFFF598983493`
> 实际程序版本：`4.3.5-RELEASE`

## 1. 核查结论

- 本地 `--help` 共检出 **109 个唯一长参数**，本表逐项给出含义、分类、产品处置和来源，名称覆盖无遗漏。
- V4.3.5 官网当前命令行页面的正文或选项表覆盖绝大多数导出能力，但二进制仍暴露导入参数、废弃参数和官网未公开参数。
- 参数出现于 `obdumper --help` 不等于它属于当前导出能力；`--mix`、`--file-suffix`、`--ignore-escape`、`--parallel` 已由官网资料确认属于导入侧。
- `--storage-uri`、`--file-name`、`--upload-behavior` 已废弃；V1.0 不提供新建入口，只为历史命令识别保留元数据。
- `--commit-size`、`--server`、`--public-synonym` 仅能从本地 help 看到，官网没有足够语义，不能进入产品表单。
- POS 映射已定版：OBDUMPER 4.3.5 实际二进制支持独立 `--pos`，且必须搭配 `--ctl-path` 与 `<表名>.ctrl` 控制文件（`position(字节长度)` 定义定长列）；官网 4.3.6“CUT + 空 splitter”口径与 4.3.5 实际行为不符，以 4.3.5 实测定版为准。详见 [Windows POS 定长格式受控实测与定版](../03-technical/evidence/windows-pos-format-validation-2026-08-07.md)。

## 2. 官方资料来源

- **S1**：[OBDUMPER V4.3.5 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997299)：当前版本主要事实来源（含官方选项分类：基础选项与高级选项）。
- **S2**：[OBLOADER & OBDUMPER V4.3.0 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002055811)：Avro 及废弃存储参数。
- **S3**：[OBLOADER & OBDUMPER V4.3.2.1 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002265551)：`--sequence-policy` 与内存相关说明。
- **S4**：[OBLOADER & OBDUMPER V4.3.1 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000001865603)：`--pos` 导出修复记录。
- **S5**：[V4.2.8 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000000518406)：`--compact-schema` 等历史参数语义。
- **S6**：[V4.2.8 快速入门](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000001179955)：`--pos --ctl-path` 导出示例。
- **S7**：[V4.3.2 旁路/直接导入](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000001696357)：`--parallel` 的导入侧语义。
- **S8**：[OBLOADER V4.3.2 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000001583710)：`--file-suffix`、`--ignore-escape` 等导入参数。
- **H**：项目发布包中实际执行 `obdumper 4.3.5-RELEASE --help`；仅用于核对二进制声明，不替代官网能力说明。

## 3. 分类统计

分类按 [OBDUMPER 4.3.5 官方命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997299) 的选项分类划分：基础选项（连接选项、功能选项、其他选项）与高级选项（功能选项、性能选项、其他选项），功能选项内再按官方子类（文件格式、压缩导出、数据库对象类型、存储路径；时间戳格式、黑白名单筛选、错误处理）细分。导入侧、废弃兼容和官网未公开参数保留在同一基线中，是为了防止命令解析器暴露的名称被误认成导出能力。

| 官方分类 | 参数数 |
|---|---:|
| 基础选项 · 连接选项 | 11 |
| 基础选项 · 功能选项 · 文件格式 | 24 |
| 基础选项 · 功能选项 · 压缩导出 | 3 |
| 基础选项 · 功能选项 · 数据库对象类型 | 16 |
| 基础选项 · 功能选项 · 存储路径 | 5 |
| 基础选项 · 其他选项 | 2 |
| 高级选项 · 功能选项 · 时间戳格式 | 11 |
| 高级选项 · 功能选项 · 黑白名单筛选 | 12 |
| 高级选项 · 功能选项 · 错误处理 | 6 |
| 高级选项 · 性能选项 | 5 |
| 高级选项 · 其他选项 | 2 |
| 导入侧/未公开 | 5 |
| 废弃兼容 | 4 |
| 运行模式/未公开 | 1 |
| 数据库对象/未公开 | 1 |
| 连接与会话/未公开 | 1 |

个别参数官方正文归类与语义有偏差（如 `--fetch-size` 位于列黑白名单筛选节、`--flashback-scn` 为各数据格式伴生参数），按官方正文归位并在向导中加注说明。

## 4. 109 个参数逐项映射

> 本节提供历史参数名称、分类、官方含义、产品处置和来源的基线映射。四元属性汇总表只反映归档时点，不覆盖当前运行时状态。

| 参数 | 分类 | 官方含义/核查结论 | 产品处置 | 来源 |
|---|---|---|---|---|
| `--add-extra-message` | 高级选项 · 功能选项 · 黑白名单筛选 | 在表 DDL 中附加表组等额外信息；依赖 sys 租户权限。 | 专家配置 | S1 |
| `--all` | 基础选项 · 功能选项 · 数据库对象类型 | 导出全部已支持对象定义和表数据；与具体对象选项互斥。 | 普通模式 | S1 |
| `--avro` | 基础选项 · 功能选项 · 文件格式 | 将表数据导出为 Apache Avro 格式。 | 格式选择 | S1/S2 |
| `--block-size` | 高级选项 · 性能选项 | 按容量或行数切分输出文件；ORC/Parquet 不生效。正文默认 0，选项表写 1024MB，存在官网内冲突；2026-08-07 实测定版：显式传值时按 MB/ROW 生效，默认值在 ≤1024MB 数据下不可区分，冲突保留。 | 高级配置 | S1/实测 |
| `--character-set` | 基础选项 · 功能选项 · 文件格式 | 创建数据库连接时使用的字符集，覆盖会话配置中的 JDBC 字符编码。 | 数据源/专家配置 | S1 |
| `--cluster` | 基础选项 · 连接选项 | 指定 OceanBase 集群；指定时 host/port 表示 ODP 服务，未指定时表示物理节点。 | 数据源快照 | S1 |
| `--column-delimiter` | 基础选项 · 功能选项 · 文件格式 | CSV 列定界符，默认单引号；与 `--column-quote` 同义且不能同时使用，已过时。 | 不展示，兼容旧任务 | S1 |
| `--column-quote` | 基础选项 · 功能选项 · 文件格式 | CSV 列包围符，默认单引号；与 `--column-delimiter` 同义且不能同时使用。 | 高级配置 | S1 |
| `--column-quote-mode` | 基础选项 · 功能选项 · 文件格式 | CSV 包围模式：all、all_not_null、minimal、non_numeric、none；默认 non_numeric。 | 高级配置 | S1 |
| `--column-separator` | 基础选项 · 功能选项 · 文件格式 | CSV 列分隔符，默认英文逗号；4.3.5 起支持多字符。 | 高级配置 | S1 |
| `--column-splitter` | 基础选项 · 功能选项 · 文件格式 | CUT 列分隔字符串；官网 4.3.6 曾把 POS 描述为 CUT + 空 splitter，但 4.3.5 实测定版为独立 `--pos`，本参数保持 CUT 专属。 | 高级配置 | S1/实测 |
| `--commit-size` | 导入侧/未公开 | 仅出现在本地 OBDUMPER Usage；4.3.5 官网未给出导出含义。 | 不进入导出配置 | H |
| `--compact-schema` | 基础选项 · 功能选项 · 文件格式 | 直接使用 `SHOW CREATE TABLE` 返回结果导出表定义，可能缺失 primary zone 等信息；仅建议性能问题时使用。 | 专家配置，待 4.3.5 实测 | S5/H |
| `--compress` | 基础选项 · 功能选项 · 压缩导出 | 是否压缩 CSV、CUT、POS、SQL 等可读格式的输出。 | 高级配置 | S1 |
| `--compression-algo` | 基础选项 · 功能选项 · 压缩导出 | 压缩算法：zstd、zlib、gzip、snappy；默认 zstd。 | 高级配置 | S1 |
| `--compression-level` | 基础选项 · 功能选项 · 压缩导出 | 压缩等级；zstd 为 1～22 默认 3，zlib 为 -1～9 默认 -1，gzip/snappy 不支持等级。 | 高级配置 | S1 |
| `--csv` | 基础选项 · 功能选项 · 文件格式 | 导出标准 CSV 数据文件，官网推荐格式。 | 格式选择 | S1 |
| `--ctl-path` | 基础选项 · 功能选项 · 存储路径 | 控制文件目录；可在导出前配置大小写转换、判空等内置处理函数。 | 专家配置 | S1 |
| `--cut` | 基础选项 · 功能选项 · 文件格式 | 导出字符串分隔的 CUT 数据文件。 | 格式选择 | S1 |
| `--database` | 基础选项 · 连接选项 | 指定要导出对象定义和表数据的数据库。 | 数据源/任务快照 | S1 |
| `--date-value-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 DATE 值的导出格式，仅 CSV/CUT；MySQL 与 Oracle 默认格式不同。 | 专家配置 | S1 |
| `--datetime-value-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 MySQL 模式 DATETIME 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--ddl` | 基础选项 · 功能选项 · 文件格式 | 导出对象定义文件，不导出数据；可与数据格式组合形成 DDL+数据任务。 | 内容选择 | S1 |
| `--distinct` | 废弃兼容 | 导出表中非重复数据；官网标记已过时。 | 不展示，兼容旧任务 | S1 |
| `--drop-object` | 基础选项 · 功能选项 · 文件格式 | 导出 DDL 时在对象创建语句前增加 DROP 语句，仅与 `--ddl` 搭配。 | 专家配置 | S1 |
| `--enable-hidden-pk` | 高级选项 · 功能选项 · 黑白名单筛选 | 无主键表使用隐藏主键 `__pk_increment` 提升导出速度；旧数据库版本有特殊权限要求。 | 专家配置 | S1 |
| `--escape-character` | 基础选项 · 功能选项 · 文件格式 | CSV/CUT 转义字符；仅支持单字符，CSV 默认 null，CUT 默认反斜杠。 | 高级配置 | S1 |
| `--exclude-column-names` | 高级选项 · 功能选项 · 黑白名单筛选 | 排除指定列；区分大小写，不能与控制文件同时生效。 | 专家配置 | S1 |
| `--exclude-data-types` | 高级选项 · 功能选项 · 黑白名单筛选 | 排除指定数据类型对应的列数据。 | 专家配置 | S1 |
| `--exclude-table` | 高级选项 · 功能选项 · 黑白名单筛选 | 排除指定表的定义和数据，支持官方模糊匹配表达式。 | 专家配置 | S1 |
| `--exclude-virtual-columns` | 高级选项 · 功能选项 · 黑白名单筛选 | 不导出生成列数据；默认会导出生成列。 | 专家配置 | S1 |
| `--fetch-size` | 高级选项 · 功能选项 · 黑白名单筛选 | Oracle 模式下每次从数据库游标读取的行数，默认 1000；需要 OceanBase JDBC。 | 高级配置 | S1 |
| `--file-encoding` | 基础选项 · 功能选项 · 文件格式 | 输出文件编码，区别于数据库连接字符集；默认 UTF-8。 | 高级配置 | S1 |
| `--file-name` | 废弃兼容 | 旧版用于指定文件名；官方 4.3.0 起标记废弃，本地 help 仍保留。 | 不展示，兼容旧任务 | S2/H |
| `--file-path` | 基础选项 · 功能选项 · 存储路径 | 输出目录、单文件路径或受支持对象存储 URI；必选。 | 普通模式 | S1 |
| `--file-suffix` | 导入侧/未公开 | 自定义输入文件后缀，官方文档将其定义为 OBLOADER 文件匹配参数，不是导出参数。 | 不进入导出配置 | S8/H |
| `--flashback-scn` | 基础选项 · 功能选项 · 文件格式 | 按指定 SCN 闪回事务点导出数据；仅与数据格式搭配且不能与 `--query-sql` 同用。 | 专家配置 | S1 |
| `--flashback-timestamp` | 高级选项 · 功能选项 · 时间戳格式 | 按闪回时间点导出，仅 Oracle 模式；仅与数据格式搭配且不能与 `--query-sql` 同用。 | 专家配置 | S1 |
| `--function` | 基础选项 · 功能选项 · 数据库对象类型 | 导出函数定义；兼容模式和数据库版本条件按官网对象矩阵校验。 | 对象选择 | S1 |
| `--help` | 基础选项 · 其他选项 | 显示 OBDUMPER 命令行帮助。 | 不进入任务配置 | S1/H |
| `--host` | 基础选项 · 连接选项 | ODP 或 OceanBase 物理节点地址；4.2.6 起支持多个 IP/域名。 | 数据源快照 | S1 |
| `--ignore-escape` | 导入侧/未公开 | 导入 CUT 文件时忽略字符转义；官方将其定义为 OBLOADER 选项。 | 不进入导出配置 | S8/H |
| `--include-column-names` | 高级选项 · 功能选项 · 黑白名单筛选 | 只导出指定列名对应的数据。 | 专家配置 | S1 |
| `--line-separator` | 基础选项 · 功能选项 · 文件格式 | CSV、CUT、POS、SQL 输出行分隔符；可选值受平台换行形式约束。 | 高级配置 | S1 |
| `--log-path` | 基础选项 · 功能选项 · 存储路径 | OBDUMPER 日志目录；未指定时默认写入 `--file-path` 对应目录。 | 高级配置 | S1 |
| `--logical-database` | 基础选项 · 连接选项 | 标识连接 ODP Sharding 逻辑库；只导出随机物理分库表定义，结果不能直接导入。 | 专家配置，高风险 | S1 |
| `--max-file-size` | 高级选项 · 功能选项 · 错误处理 | 限制单进程可导出的数据总量，超过上限停止导出，单位 Byte。 | 高级配置 | S1 |
| `--mem` | 高级选项 · 性能选项 | 设置 JVM 内存大小，支持 K/M/G/T；4.3.2 起支持，默认 4G。 | 执行节点/高级配置 | S1/S3 |
| `--mix` | 导入侧/未公开 | 混合定义与数据的文件格式，官方用例用于 OBLOADER 非标准目录或多语句输入。 | 不进入导出配置 | S8/H |
| `--nls-date-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 Oracle 会话 `nls_date_format`，不等于 DATE 文件值格式。 | 专家配置 | S1 |
| `--nls-timestamp-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 Oracle 会话 `nls_timestamp_format`，不等于文件值格式。 | 专家配置 | S1 |
| `--nls-timestamp-tz-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 Oracle 会话 `nls_timestamp_tz_format`，不等于文件值格式。 | 专家配置 | S1 |
| `--no-nested-dir` | 基础选项 · 功能选项 · 存储路径 | 使用扁平输出目录，所有文件直接写入 `--file-path` 目录。 | 高级配置 | S1 |
| `--no-sys` | 基础选项 · 连接选项 | 私有部署无法提供 sys 密码时使用；会影响元数据能力和性能。 | 数据源/专家配置 | S1 |
| `--null-string` | 基础选项 · 功能选项 · 文件格式 | 将 NULL 输出为指定字符串；仅 CSV/CUT，默认 `\N`。 | 高级配置 | S1 |
| `--obj-user` | 基础选项 · 功能选项 · 数据库对象类型 | 导出用户定义；公有云等适用限制需校验。 | 对象选择 | S1 |
| `--orc` | 基础选项 · 功能选项 · 文件格式 | 导出 Apache ORC 列式数据文件；内存占用可能较高。 | 格式选择 | S1 |
| `--package` | 基础选项 · 功能选项 · 数据库对象类型 | 导出程序包定义。 | 对象选择 | S1 |
| `--package-body` | 基础选项 · 功能选项 · 数据库对象类型 | 导出程序包体定义。 | 对象选择 | S1 |
| `--page-size` | 高级选项 · 性能选项 | 每次分页查询的记录数，默认 1,000,000。 | 高级配置 | S1 |
| `--par` | 基础选项 · 功能选项 · 文件格式 | 导出 Apache Parquet 列式数据文件。 | 格式选择 | S1 |
| `--parallel` | 导入侧/未公开 | OBServer 旁路/直接导入写入和排序的并行度，不是 OBDUMPER 导出并发。 | 不进入导出配置 | S7/H |
| `--parallel-macro` | 高级选项 · 性能选项 | 每个导出线程处理的宏块数，默认 8。 | 高级配置 | S1 |
| `--partition` | 高级选项 · 功能选项 · 黑白名单筛选 | 只导出指定分区数据；不能与 `--query-sql` 同用，二级分区需指定二级分区名。 | 专家配置 | S1 |
| `--password` | 基础选项 · 连接选项 | 数据库账号密码；命令行可省略表示无密码账户。 | 数据源快照，脱敏 | S1 |
| `--port` | 基础选项 · 连接选项 | ODP 或物理节点 SQL 端口；4.2.6 起支持多个端口。 | 数据源快照 | S1 |
| `--pos` | 基础选项 · 功能选项 · 文件格式 | 固定长度数据格式。2026-08-07 受控实测定版：4.3.5 支持独立 `--pos`，必须搭配 `--ctl-path` 与 `<表名>.ctrl` 控制文件（`position(字节长度)` 定义定长列）；官网 4.3.6“CUT+空 splitter”口径与 4.3.5 行为不符。 | 格式选择（定版） | S1/S4/S6/H/实测 |
| `--preserve-zero-datetime` | 高级选项 · 功能选项 · 时间戳格式 | 保留 MySQL 模式 DATE/DATETIME/TIMESTAMP 零值的原始形式。 | 专家配置 | S1 |
| `--procedure` | 基础选项 · 功能选项 · 数据库对象类型 | 导出存储过程定义；MySQL 模式受数据库版本限制。 | 对象选择 | S1 |
| `--public-cloud` | 基础选项 · 连接选项 | 标识云数据库 OceanBase 环境，并默认启用无 sys 模式。 | 数据源快照 | S1 |
| `--public-synonym` | 数据库对象/未公开 | 本地 help Usage 接受公共同义词清单，但 4.3.5 官网未列出含义和适用条件。 | 不展示，待官方/实测确认 | H |
| `--query-sql` | 高级选项 · 功能选项 · 黑白名单筛选 | 导出自定义单条查询结果；可直接传 SQL 或 `file://` 文件，不能与 where/partition 同用。 | 专家配置 | S1 |
| `--remove-newline` | 高级选项 · 功能选项 · 错误处理 | 强制删除数据中的回车和换行，仅 CUT；会改变导出数据。 | 专家配置，高风险 | S1 |
| `--retain-empty-files` | 高级选项 · 功能选项 · 黑白名单筛选 | where 或 partition 结果为空时仍生成空文件；实际参数为复数。 | 高级配置 | S1/H |
| `--retain-schema` | 高级选项 · 其他选项 | 在表和同义词 DDL 中保留 `schema.object` 前缀。 | 专家配置 | S1 |
| `--retry` | 高级选项 · 功能选项 · 错误处理 | 从 `--file-path` 下 `dump.ckpt` 最近保存点继续导出。 | 失败任务操作 | S1 |
| `--role` | 基础选项 · 功能选项 · 数据库对象类型 | 导出角色定义；Oracle 模式及公有云限制需校验。 | 对象选择 | S1 |
| `--sequence` | 基础选项 · 功能选项 · 数据库对象类型 | 导出序列定义。 | 对象选择 | S1 |
| `--sequence-policy` | 基础选项 · 功能选项 · 数据库对象类型 | 序列导出策略：restart 或 preserve，默认 preserve。 | 专家配置 | S3/H |
| `--server` | 运行模式/未公开 | 本地 help 描述为在 server 中远程 load/dump；4.3.5 官网无部署语义。 | 不展示，待官方/实测确认 | H |
| `--session-config` | 高级选项 · 其他选项 | 指定连接配置文件；默认配置无需显式指定。 | 执行节点/专家配置 | S1 |
| `--skip-check-dir` | 高级选项 · 功能选项 · 错误处理 | 跳过输出目录非空检查，可能覆盖同名文件。 | 专家配置，高风险 | S1 |
| `--skip-header` | 基础选项 · 功能选项 · 文件格式 | 控制是否省略 CSV 第一行字段头。 | 高级配置 | S1 |
| `--snapshot` | 高级选项 · 功能选项 · 错误处理 | 导出历史版本以获得全局一致性快照；仅与数据格式搭配。 | 专家配置 | S1 |
| `--sql` | 基础选项 · 功能选项 · 文件格式 | 将每行表数据导出为一条 INSERT 语句。 | 格式选择 | S1 |
| `--storage-uri` | 废弃兼容 | 旧版对象存储 URI；官方 4.3.0 起废弃并整合到 `--file-path`。 | 不展示，兼容旧任务 | S2/H |
| `--synonym` | 基础选项 · 功能选项 · 数据库对象类型 | 导出同义词定义；官网注明暂不支持 MySQL 模式。 | 对象选择 | S1 |
| `--sys-password` | 基础选项 · 连接选项 | sys 租户特权账户密码；与 `--sys-user` 配合，OceanBase 4.0+ 通常无需指定。 | 任务预检查/安全配置 | S1 |
| `--sys-user` | 基础选项 · 连接选项 | sys 租户特权用户名，默认 root；用于读取系统元数据。 | 任务预检查/安全配置 | S1 |
| `--table` | 基础选项 · 功能选项 · 数据库对象类型 | 导出指定表定义或表数据。 | 对象选择 | S1 |
| `--table-group` | 基础选项 · 功能选项 · 数据库对象类型 | 导出表组定义。 | 对象选择 | S1 |
| `--tenant` | 连接与会话/未公开 | 本地 help 将其作为租户名；4.3.5 官网主要要求通过 `--user` 的 `user@tenant#cluster` 组合表达。 | 数据源快照，独立参数待实测 | S1/H |
| `--thread` | 高级选项 · 性能选项 | OBDUMPER 导出线程数；默认 CPU×2、上限 32，导出多个 DDL 时建议不超过 4。 | 高级配置 | S1 |
| `--time-value-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 MySQL 模式 TIME 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--timestamp-ltz-value-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 Oracle TIMESTAMP WITH LOCAL TIME ZONE 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--timestamp-tz-value-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 Oracle TIMESTAMP WITH TIME ZONE 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--timestamp-value-format` | 高级选项 · 功能选项 · 时间戳格式 | 设置 MySQL/Oracle TIMESTAMP 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--tmp-path` | 基础选项 · 功能选项 · 存储路径 | 对象存储 Multipart Upload 使用的本地临时分块及日志路径。 | 执行节点/高级配置 | S1/H |
| `--trail-delimiter` | 基础选项 · 功能选项 · 文件格式 | 控制导出数据行是否保留行尾最后一个列分隔符。 | 高级配置 | S1 |
| `--trigger` | 基础选项 · 功能选项 · 数据库对象类型 | 导出触发器定义，主要适用于 Oracle 兼容模式。 | 对象选择 | S1 |
| `--type` | 基础选项 · 功能选项 · 数据库对象类型 | 导出类型定义。 | 对象选择 | S1 |
| `--type-body` | 基础选项 · 功能选项 · 数据库对象类型 | 导出类型体定义，依赖对应类型。 | 对象选择 | S1 |
| `--upload-behavior` | 废弃兼容 | 旧版对象存储上传策略 FAST/COMPLETE；官方 4.3.0 起废弃。 | 不展示，兼容旧任务 | S2/H |
| `--user` | 基础选项 · 连接选项 | 数据库用户名；官网推荐组合格式 `user@tenant#cluster`。 | 数据源快照 | S1 |
| `--version` | 基础选项 · 其他选项 | 显示 OBDUMPER 版本。 | 节点版本核验 | S1/H |
| `--view` | 基础选项 · 功能选项 · 数据库对象类型 | 导出视图定义。 | 对象选择 | S1 |
| `--weak-read` | 高级选项 · 功能选项 · 错误处理 | 从备副本读取并导出表数据，不等于备集群。 | 专家配置 | S1 |
| `--where` | 高级选项 · 功能选项 · 黑白名单筛选 | 按条件表达式过滤数据；仅与数据格式搭配且不能与 `--query-sql` 同用。 | 专家配置 | S1 |
| `--with-trim` | 基础选项 · 功能选项 · 文件格式 | 删除导出值左右空格字符。 | 高级配置 | S1 |

## 5. 官方分类在导出向导中的落点

官方分类与向导步骤对应如下：

1. **基础选项 · 连接选项**：由数据源选择与数据源快照承载（host/port/user/password/database/cluster、sys 与云环境配置）；向导不得重复填写基础连接。
2. **基础选项 · 功能选项 · 文件格式**：步骤 4 格式单选与各格式专属配置面板（含 CSV/CUT/POS/SQL/Parquet/ORC/Avro 序列化、DDL 伴生与闪回 SCN 等伴生参数）。
3. **基础选项 · 功能选项 · 压缩导出**：步骤 5 压缩面板（仅 CSV/CUT/POS/SQL 可读格式）。
4. **基础选项 · 功能选项 · 数据库对象类型**：步骤 2 对象选择（全部对象与各对象类型选项）。
5. **基础选项 · 功能选项 · 存储路径**：步骤 5 输出路径、日志路径、控制文件目录、扁平目录与对象存储临时分块目录。
6. **高级选项 · 功能选项 · 时间戳格式**：步骤 5 时间戳格式面板（闪回时间点、NLS 会话格式与日期时间值格式）。
7. **高级选项 · 功能选项 · 黑白名单筛选**：步骤 5 筛选面板（自定义查询、表/列黑白名单、保留空文件与游标抓取行数）。
8. **高级选项 · 功能选项 · 错误处理**：步骤 5 错误处理面板（目录空性跳过、总量上限、删除换行与快照/弱读）。
9. **高级选项 · 性能选项**：步骤 5 性能面板（线程、分页、宏块、JVM 内存与文件切分）。
10. **兼容治理**：导入侧参数、废弃参数、help-only 参数不得混入可配置参数清单。

## 6. 需要补入导出模块的参数候选

- **高级配置**：`--column-quote-mode`、`--line-separator`、`--file-encoding`、`--null-string`、`--skip-header`、`--trail-delimiter`、`--compress`、`--compression-algo`、`--compression-level`、`--tmp-path`、`--mem`、`--fetch-size`、`--parallel-macro`、`--max-file-size`。
- **专家配置**：日期时间值格式、Oracle NLS 会话格式、`--drop-object`、`--add-extra-message`、`--retain-schema`、`--compact-schema`、`--sequence-policy`、列/表筛选、闪回、快照、弱读、逻辑库和高风险目录选项。
- **不进入新建表单**：`--mix`、`--file-suffix`、`--ignore-escape`、`--parallel`、`--commit-size`、`--server`、`--public-synonym`、`--storage-uri`、`--file-name`、`--upload-behavior`、`--distinct`、`--column-delimiter`。

## 7. 仍需受控实测

- ~~POS 最终生成独立 `--pos` 还是 CUT 组合~~（2026-08-07 已实测定版：独立 `--pos` + `--ctl-path` + `<表名>.ctrl`，见 [受控实测与定版](../03-technical/evidence/windows-pos-format-validation-2026-08-07.md)）。
- `--block-size` 默认值：官网正文为 0，选项表为 1024MB；实测确认显式传值按 MB/ROW 生效，默认值在 ≤1024MB 数据下不可区分，冲突保留为低风险残余。
- `--compact-schema`、`--sequence-policy` 在 4.3.5 当前页面缺少完整约束。
- help-only 的 `--server`、`--public-synonym`、`--commit-size` 是否为可用导出能力。
- `--tenant` 独立长参数与官网推荐 `user@tenant#cluster` 连接形式的实际优先级。
- 对象存储 `--tmp-path` 的空间预检查以及敏感 URI 脱敏规则。

## 8. Windows 4.3.5 输出路径验证基线

官方把 `-f/--file-path` 定义为本地磁盘绝对路径。Windows 4.3.5 的产品与实际命令基线保持一致：用户填写本次任务的完整 Windows 绝对路径，命令原样使用该格式，不拆分父目录、不转换为相对路径，也不追加子目录。

示例：

```text
--file-path "E:\workespace\ob-data-orch\tmp\p0-validation\run-06\output-test"
```

执行规则：

- 产品配置、预检查、命令预览、任务快照和实际参数均保留用户填写的规范化绝对路径；
- Windows 路径使用 `/E:/...` 形式并原样传递；不生成 `file:///` URI，也不传递 `E:\\...` 形式；
- Agent 启动前校验路径属于允许目录、可创建或可写，并防止目录穿越；
- 目标目录已非空时，未勾选跳过选项的预检查必须阻断；用户明确勾选时生成 `--skip-check-dir` 并把空性检查标记为已跳过。平台不得自动改路径或自行加入该参数。

早期自动化验证曾在部分绝对路径运行中观察到 `file://nullE:/...`，同时相对路径运行成功；用户随后确认上述完整 Windows 绝对路径格式可以被 4.3.5 正确识别并成功导出。产品基线按直接绝对路径执行，`file://null` 作为环境差异回归项保留，不据此引入相对路径适配。

## 9. V1.0 四元属性汇总

> 本节为每个参数提供归档时点的 sensitivity、riskLevel、evidenceState 和 v1State 属性；当前发布态不由本表单独决定。

### 9.1 导入侧/未公开（5 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--file-suffix` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--ignore-escape` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--mix` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--parallel` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--commit-size` | NORMAL | LOW | HELP_ONLY | HIDDEN |

### 9.2 废弃兼容（4 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--storage-uri` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--file-name` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--upload-behavior` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--distinct` | NORMAL | LOW | HELP_ONLY | HIDDEN |

### 9.3 CLI 元参数（2 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--help` | NORMAL | LOW | VERIFIED | HIDDEN |
| `--version` | NORMAL | LOW | VERIFIED | HIDDEN |

### 9.4 运行模式/未公开（1 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--server` | NORMAL | LOW | HELP_ONLY | HIDDEN |

### 9.5 数据库对象/未公开（1 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--public-synonym` | NORMAL | LOW | HELP_ONLY | HIDDEN |

### 9.6 连接与会话/未公开（1 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--tenant` | IDENTIFIER | LOW | CONFLICT_PENDING | VALIDATION_GATED |

### 9.6 基础选项 · 连接选项（11 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--host` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--port` | NORMAL | LOW | VERIFIED | ENABLED |
| `--user` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--password` | SECRET | LOW | VERIFIED | ENABLED |
| `--cluster` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--database` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--no-sys` | NORMAL | MEDIUM | VERIFIED | ENABLED |
| `--public-cloud` | NORMAL | LOW | VERIFIED | ENABLED |
| `--sys-user` | IDENTIFIER | MEDIUM | VERIFIED | ENABLED |
| `--sys-password` | SECRET | MEDIUM | VERIFIED | ENABLED |
| `--logical-database` | NORMAL | HIGH | VERIFIED | ENABLED |

### 9.7 基础选项 · 功能选项 · 文件格式（24 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--csv` | NORMAL | LOW | VERIFIED | ENABLED |
| `--cut` | NORMAL | LOW | VERIFIED | ENABLED |
| `--pos` | NORMAL | LOW | VERIFIED（2026-08-07 实测） | ENABLED（待产品接入） |
| `--sql` | NORMAL | LOW | VERIFIED | ENABLED |
| `--par` | NORMAL | LOW | VERIFIED（官方格式表，2026-08-07 接入） | ENABLED |
| `--orc` | NORMAL | MEDIUM | VERIFIED（官方格式表，2026-08-07 接入） | ENABLED |
| `--avro` | NORMAL | LOW | VERIFIED（官方格式表，2026-08-07 接入） | ENABLED |
| `--ddl` | NORMAL | LOW | VERIFIED | ENABLED |
| `--character-set` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--escape-character` | NORMAL | LOW | VERIFIED | ENABLED |
| `--file-encoding` | NORMAL | LOW | VERIFIED | ENABLED |
| `--line-separator` | NORMAL | LOW | VERIFIED | ENABLED |
| `--null-string` | NORMAL | LOW | VERIFIED | ENABLED |
| `--with-trim` | NORMAL | LOW | VERIFIED | ENABLED |
| `--skip-header` | NORMAL | LOW | VERIFIED | ENABLED |
| `--column-separator` | NORMAL | LOW | VERIFIED | ENABLED |
| `--column-quote` | NORMAL | LOW | VERIFIED | ENABLED |
| `--column-quote-mode` | NORMAL | LOW | VERIFIED | ENABLED |
| `--column-delimiter` | NORMAL | LOW | HELP_ONLY | HIDDEN |
| `--column-splitter` | NORMAL | LOW | VERIFIED（CUT 专属；POS 已实测定版为独立 `--pos`） | ENABLED（待产品接入） |
| `--trail-delimiter` | NORMAL | LOW | VERIFIED | ENABLED |
| `--drop-object` | NORMAL | HIGH | VERIFIED | ENABLED |
| `--compact-schema` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测：show create table 检索文本，当前库无差异） | ENABLED |
| `--flashback-scn` | NORMAL | MEDIUM | VERIFIED | ENABLED |

### 9.8 基础选项 · 功能选项 · 压缩导出（3 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--compress` | NORMAL | LOW | VERIFIED | ENABLED |
| `--compression-algo` | NORMAL | LOW | VERIFIED | ENABLED |
| `--compression-level` | NORMAL | LOW | OFFICIAL_ONLY | ENABLED（2026-08-10 接入，按算法分范围） |

### 9.9 基础选项 · 功能选项 · 数据库对象类型（16 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--all` | NORMAL | LOW | VERIFIED | ENABLED |
| `--table` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--table-group` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--view` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--trigger` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--obj-user` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--role` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--sequence` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--sequence-policy` | NORMAL | LOW | CONFLICT_PENDING | VALIDATION_GATED |
| `--synonym` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--type` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--type-body` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--package` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--package-body` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--function` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--procedure` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED |

### 9.10 基础选项 · 功能选项 · 存储路径（5 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--file-path` | NORMAL | LOW | VERIFIED | ENABLED |
| `--log-path` | NORMAL | LOW | VERIFIED | ENABLED |
| `--no-nested-dir` | NORMAL | LOW | VERIFIED | ENABLED |
| `--ctl-path` | NORMAL | LOW | VERIFIED（2026-08-07 实测，需 `<表名>.ctrl` 控制文件） | ENABLED（待产品接入，仅 POS 显示） |
| `--tmp-path` | NORMAL | MEDIUM | VERIFIED（jar 字节码取证，2026-08-07） | ENABLED |

### 9.11 基础选项 · 其他选项（2 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--help` | NORMAL | LOW | VERIFIED | HIDDEN |
| `--version` | NORMAL | LOW | VERIFIED | HIDDEN |

### 9.12 高级选项 · 功能选项 · 时间戳格式（11 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--date-value-format` | NORMAL | LOW | VERIFIED（2026-08-13 MySQL DATE 列格式生效） | ENABLED |
| `--time-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--datetime-value-format` | NORMAL | LOW | VERIFIED（2026-08-13 MySQL DATETIME 列格式生效） | ENABLED |
| `--timestamp-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--timestamp-tz-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--timestamp-ltz-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--preserve-zero-datetime` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--nls-date-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--nls-timestamp-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--nls-timestamp-tz-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED |
| `--flashback-timestamp` | NORMAL | MEDIUM | VERIFIED | ENABLED |

### 9.13 高级选项 · 功能选项 · 黑白名单筛选（12 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--query-sql` | NORMAL | HIGH | VERIFIED | VALIDATION_GATED |
| `--where` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测：行数筛选生效） | ENABLED |
| `--partition` | NORMAL | LOW | VERIFIED（2026-08-13 单/多 HASH 分区行数生效） | ENABLED |
| `--include-column-names` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--exclude-column-names` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--exclude-data-types` | NORMAL | LOW | VERIFIED（2026-08-13 decimal 列排除生效） | ENABLED |
| `--exclude-virtual-columns` | NORMAL | LOW | VERIFIED | ENABLED |
| `--exclude-table` | IDENTIFIER | LOW | VERIFIED | ENABLED |
| `--enable-hidden-pk` | NORMAL | MEDIUM | CONFLICT_PENDING（工具接受；隐藏主键行为与前置校验未验证） | VALIDATION_GATED |
| `--fetch-size` | NORMAL | LOW | VERIFIED | ENABLED |
| `--add-extra-message` | NORMAL | MEDIUM | CONFLICT_PENDING（工具接受；行为与 sys 权限链未验证） | VALIDATION_GATED |
| `--retain-empty-files` | NORMAL | LOW | VERIFIED | ENABLED |

### 9.14 高级选项 · 功能选项 · 错误处理（6 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--skip-check-dir` | NORMAL | MEDIUM | VERIFIED | ENABLED |
| `--max-file-size` | NORMAL | LOW | VERIFIED | ENABLED |
| `--remove-newline` | NORMAL | HIGH | VERIFIED | ENABLED |
| `--retry` | NORMAL | MEDIUM | CONFLICT_PENDING（仅无保存点失败关闭；有效续跑并入 EX-I8） | VALIDATION_GATED |
| `--snapshot` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测） | ENABLED |
| `--weak-read` | NORMAL | MEDIUM | VERIFIED（2026-08-11 工具实测：从备库读；产品预检查待实现） | VALIDATION_GATED |

### 9.15 高级选项 · 性能选项（5 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--thread` | NORMAL | LOW | VERIFIED | ENABLED |
| `--page-size` | NORMAL | LOW | VERIFIED | ENABLED |
| `--parallel-macro` | NORMAL | LOW | VERIFIED | ENABLED |
| `--mem` | NORMAL | MEDIUM | VERIFIED | ENABLED |
| `--block-size` | NORMAL | LOW | CONFLICT_PENDING（默认值残余；显式传值已实测生效） | ENABLED（2026-08-10 接入） |

### 9.16 高级选项 · 其他选项（2 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State |
|---|---|---|---|---|
| `--session-config` | NORMAL | LOW | VERIFIED | ENABLED |
| `--retain-schema` | NORMAL | LOW | VERIFIED | ENABLED |

### 9.17 V1.0 状态汇总

| v1State | 参数数量 | 占比 |
|---|---:|---:|
| ENABLED | 68 | 62.4% |
| VALIDATION_GATED | 27 | 24.8% |
| HIDDEN | 14 | 12.8% |
| BLOCKED | 0 | 0% |
| **合计** | **109** | **100%** |
