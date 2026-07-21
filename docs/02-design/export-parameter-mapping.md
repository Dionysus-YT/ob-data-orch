# OBDUMPER V4.3.5 导出参数映射基线

> 文档状态：参数名称与官方含义已核查；官网冲突、未公开参数和行为边界待实测  
> 核验日期：2026-07-17  
> 发布包：`ob-loader-dumper-4.3.5-RELEASE.zip`  
> 发布包 SHA-256：`C1A5D5EE053106803263015F00EC7F0B94B73EA4015A2E53CF1AFFF598983493`  
> 实际程序版本：`4.3.5-RELEASE`

## 1. 核查结论

- 本地 `--help` 共检出 **109 个唯一长参数**，本表逐项给出含义、分类、产品处置和来源，名称覆盖无遗漏。
- V4.3.5 官网当前命令行页面的正文或选项表覆盖绝大多数导出能力，但二进制仍暴露导入参数、废弃参数和官网未公开参数。
- 参数出现于 `obdumper --help` 不等于它属于当前导出能力；`--mix`、`--file-suffix`、`--ignore-escape`、`--parallel` 已由官网资料确认属于导入侧。
- `--storage-uri`、`--file-name`、`--upload-behavior` 已废弃；V1.0 不提供新建入口，只为历史命令识别保留元数据。
- `--commit-size`、`--server`、`--public-synonym` 仅能从本地 help 看到，官网没有足够语义，不能进入产品表单。
- POS 存在官方口径冲突：V4.3.5 当前页写为 CUT + 空 splitter + 控制文件，旧版官方示例和本地 help 使用独立 `--pos`；提交映射必须经过受控实测。

## 2. 官方资料来源

- **S1**：[OBDUMPER V4.3.5 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997299)：当前版本主要事实来源。
- **S2**：[OBLOADER & OBDUMPER V4.3.0 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002055811)：Avro 及废弃存储参数。
- **S3**：[OBLOADER & OBDUMPER V4.3.2.1 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002265551)：`--sequence-policy` 与内存相关说明。
- **S4**：[OBLOADER & OBDUMPER V4.3.1 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000001865603)：`--pos` 导出修复记录。
- **S5**：[V4.2.8 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000000518406)：`--compact-schema` 等历史参数语义。
- **S6**：[V4.2.8 快速入门](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000001179955)：`--pos --ctl-path` 导出示例。
- **S7**：[V4.3.2 旁路/直接导入](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000001696357)：`--parallel` 的导入侧语义。
- **S8**：[OBLOADER V4.3.2 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000001583710)：`--file-suffix`、`--ignore-escape` 等导入参数。
- **H**：项目发布包中实际执行 `obdumper 4.3.5-RELEASE --help`；仅用于核对二进制声明，不替代官网能力说明。

## 3. 分类统计

| 分类 | 参数数 |
|---|---:|
| 导入侧/未公开 | 5 |
| 对象范围 | 1 |
| 废弃兼容 | 3 |
| 连接与会话 | 16 |
| 连接与会话/未公开 | 1 |
| 内容与数据格式 | 8 |
| 日期时间序列化 | 7 |
| 输出与文件 | 8 |
| 数据库对象 | 14 |
| 数据库对象/未公开 | 1 |
| 数据筛选与一致性 | 14 |
| 文本序列化 | 6 |
| 性能与资源 | 7 |
| 压缩 | 3 |
| 运行模式/未公开 | 1 |
| CLI 元参数 | 2 |
| CSV 序列化 | 5 |
| CUT/POS 序列化 | 2 |
| DDL 与对象处理 | 5 |

分类按产品配置语义划分，不照搬命令行 Usage 的排列。导入侧、废弃兼容和官网未公开参数被保留在同一基线中，是为了防止命令解析器暴露的名称被误认成导出能力。

## 4. 109 个参数逐项映射

| 参数 | 分类 | 官方含义/核查结论 | 产品处置 | 来源 |
|---|---|---|---|---|
| `--add-extra-message` | DDL 与对象处理 | 在表 DDL 中附加表组等额外信息；依赖 sys 租户权限。 | 专家配置 | S1 |
| `--all` | 对象范围 | 导出全部已支持对象定义和表数据；与具体对象选项互斥。 | 普通模式 | S1 |
| `--avro` | 内容与数据格式 | 将表数据导出为 Apache Avro 格式。 | 格式选择 | S1/S2 |
| `--block-size` | 输出与文件 | 按容量或行数切分输出文件；ORC/Parquet 不生效。正文默认 0，选项表写 1024MB，存在官网内冲突。 | 高级配置 | S1 |
| `--character-set` | 连接与会话 | 创建数据库连接时使用的字符集，覆盖会话配置中的 JDBC 字符编码。 | 数据源/专家配置 | S1 |
| `--cluster` | 连接与会话 | 指定 OceanBase 集群；指定时 host/port 表示 ODP 服务，未指定时表示物理节点。 | 数据源快照 | S1 |
| `--column-delimiter` | CSV 序列化 | CSV 列定界符，默认单引号；与 `--column-quote` 同义且不能同时使用，已过时。 | 不展示，兼容旧任务 | S1 |
| `--column-quote` | CSV 序列化 | CSV 列包围符，默认单引号；与 `--column-delimiter` 同义且不能同时使用。 | 高级配置 | S1 |
| `--column-quote-mode` | CSV 序列化 | CSV 包围模式：all、all_not_null、minimal、non_numeric、none；默认 non_numeric。 | 高级配置 | S1 |
| `--column-separator` | CSV 序列化 | CSV 列分隔符，默认英文逗号；4.3.5 起支持多字符。 | 高级配置 | S1 |
| `--column-splitter` | CUT/POS 序列化 | CUT 列分隔字符串；官网 4.3.5 的 POS 方案要求置为空字符串。 | 高级配置 | S1 |
| `--commit-size` | 导入侧/未公开 | 仅出现在本地 OBDUMPER Usage；4.3.5 官网未给出导出含义。 | 不进入导出配置 | H |
| `--compact-schema` | DDL 与对象处理 | 直接使用 `SHOW CREATE TABLE` 返回结果导出表定义，可能缺失 primary zone 等信息；仅建议性能问题时使用。 | 专家配置，待 4.3.5 实测 | S5/H |
| `--compress` | 压缩 | 是否压缩 CSV、CUT、POS、SQL 等可读格式的输出。 | 高级配置 | S1 |
| `--compression-algo` | 压缩 | 压缩算法：zstd、zlib、gzip、snappy；默认 zstd。 | 高级配置 | S1 |
| `--compression-level` | 压缩 | 压缩等级；zstd 为 1～22 默认 3，zlib 为 -1～9 默认 -1，gzip/snappy 不支持等级。 | 高级配置 | S1 |
| `--csv` | 内容与数据格式 | 导出标准 CSV 数据文件，官网推荐格式。 | 格式选择 | S1 |
| `--ctl-path` | 输出与文件 | 控制文件目录；可在导出前配置大小写转换、判空等内置处理函数。 | 专家配置 | S1 |
| `--cut` | 内容与数据格式 | 导出字符串分隔的 CUT 数据文件。 | 格式选择 | S1 |
| `--database` | 连接与会话 | 指定要导出对象定义和表数据的数据库。 | 数据源/任务快照 | S1 |
| `--date-value-format` | 日期时间序列化 | 设置 DATE 值的导出格式，仅 CSV/CUT；MySQL 与 Oracle 默认格式不同。 | 专家配置 | S1 |
| `--datetime-value-format` | 日期时间序列化 | 设置 MySQL 模式 DATETIME 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--ddl` | 内容与数据格式 | 导出对象定义文件，不导出数据；可与数据格式组合形成 DDL+数据任务。 | 内容选择 | S1 |
| `--distinct` | 数据筛选与一致性 | 导出表中非重复数据；官网标记已过时。 | 不展示，兼容旧任务 | S1 |
| `--drop-object` | DDL 与对象处理 | 导出 DDL 时在对象创建语句前增加 DROP 语句，仅与 `--ddl` 搭配。 | 专家配置 | S1 |
| `--enable-hidden-pk` | 数据筛选与一致性 | 无主键表使用隐藏主键 `__pk_increment` 提升导出速度；旧数据库版本有特殊权限要求。 | 专家配置 | S1 |
| `--escape-character` | 文本序列化 | CSV/CUT 转义字符；仅支持单字符，CSV 默认 null，CUT 默认反斜杠。 | 高级配置 | S1 |
| `--exclude-column-names` | 数据筛选与一致性 | 排除指定列；区分大小写，不能与控制文件同时生效。 | 专家配置 | S1 |
| `--exclude-data-types` | 数据筛选与一致性 | 排除指定数据类型对应的列数据。 | 专家配置 | S1 |
| `--exclude-table` | 数据筛选与一致性 | 排除指定表的定义和数据，支持官方模糊匹配表达式。 | 专家配置 | S1 |
| `--exclude-virtual-columns` | 数据筛选与一致性 | 不导出生成列数据；默认会导出生成列。 | 专家配置 | S1 |
| `--fetch-size` | 性能与资源 | Oracle 模式下每次从数据库游标读取的行数，默认 1000；需要 OceanBase JDBC。 | 高级配置 | S1 |
| `--file-encoding` | 文本序列化 | 输出文件编码，区别于数据库连接字符集；默认 UTF-8。 | 高级配置 | S1 |
| `--file-name` | 废弃兼容 | 旧版用于指定文件名；官方 4.3.0 起标记废弃，本地 help 仍保留。 | 不展示，兼容旧任务 | S2/H |
| `--file-path` | 输出与文件 | 输出目录、单文件路径或受支持对象存储 URI；必选。 | 普通模式 | S1 |
| `--file-suffix` | 导入侧/未公开 | 自定义输入文件后缀，官方文档将其定义为 OBLOADER 文件匹配参数，不是导出参数。 | 不进入导出配置 | S8/H |
| `--flashback-scn` | 数据筛选与一致性 | 按指定 SCN 闪回事务点导出数据；仅与数据格式搭配且不能与 `--query-sql` 同用。 | 专家配置 | S1 |
| `--flashback-timestamp` | 数据筛选与一致性 | 按闪回时间点导出，仅 Oracle 模式；仅与数据格式搭配且不能与 `--query-sql` 同用。 | 专家配置 | S1 |
| `--function` | 数据库对象 | 导出函数定义；兼容模式和数据库版本条件按官网对象矩阵校验。 | 对象选择 | S1 |
| `--help` | CLI 元参数 | 显示 OBDUMPER 命令行帮助。 | 不进入任务配置 | S1/H |
| `--host` | 连接与会话 | ODP 或 OceanBase 物理节点地址；4.2.6 起支持多个 IP/域名。 | 数据源快照 | S1 |
| `--ignore-escape` | 导入侧/未公开 | 导入 CUT 文件时忽略字符转义；官方将其定义为 OBLOADER 选项。 | 不进入导出配置 | S8/H |
| `--include-column-names` | 数据筛选与一致性 | 只导出指定列名对应的数据。 | 专家配置 | S1 |
| `--line-separator` | 文本序列化 | CSV、CUT、POS、SQL 输出行分隔符；可选值受平台换行形式约束。 | 高级配置 | S1 |
| `--log-path` | 输出与文件 | OBDUMPER 日志目录；未指定时默认写入 `--file-path` 对应目录。 | 高级配置 | S1 |
| `--logical-database` | 连接与会话 | 标识连接 ODP Sharding 逻辑库；只导出随机物理分库表定义，结果不能直接导入。 | 专家配置，高风险 | S1 |
| `--max-file-size` | 性能与资源 | 限制单进程可导出的数据总量，超过上限停止导出，单位 Byte。 | 高级配置 | S1 |
| `--mem` | 性能与资源 | 设置 JVM 内存大小，支持 K/M/G/T；4.3.2 起支持，默认 4G。 | 执行节点/高级配置 | S1/S3 |
| `--mix` | 导入侧/未公开 | 混合定义与数据的文件格式，官方用例用于 OBLOADER 非标准目录或多语句输入。 | 不进入导出配置 | S8/H |
| `--nls-date-format` | 连接与会话 | 设置 Oracle 会话 `nls_date_format`，不等于 DATE 文件值格式。 | 专家配置 | S1 |
| `--nls-timestamp-format` | 连接与会话 | 设置 Oracle 会话 `nls_timestamp_format`，不等于文件值格式。 | 专家配置 | S1 |
| `--nls-timestamp-tz-format` | 连接与会话 | 设置 Oracle 会话 `nls_timestamp_tz_format`，不等于文件值格式。 | 专家配置 | S1 |
| `--no-nested-dir` | 输出与文件 | 使用扁平输出目录，所有文件直接写入 `--file-path` 目录。 | 高级配置 | S1 |
| `--no-sys` | 连接与会话 | 私有部署无法提供 sys 密码时使用；会影响元数据能力和性能。 | 数据源/专家配置 | S1 |
| `--null-string` | 文本序列化 | 将 NULL 输出为指定字符串；仅 CSV/CUT，默认 `\N`。 | 高级配置 | S1 |
| `--obj-user` | 数据库对象 | 导出用户定义；公有云等适用限制需校验。 | 对象选择 | S1 |
| `--orc` | 内容与数据格式 | 导出 Apache ORC 列式数据文件；内存占用可能较高。 | 格式选择 | S1 |
| `--package` | 数据库对象 | 导出程序包定义。 | 对象选择 | S1 |
| `--package-body` | 数据库对象 | 导出程序包体定义。 | 对象选择 | S1 |
| `--page-size` | 性能与资源 | 每次分页查询的记录数，默认 1,000,000。 | 高级配置 | S1 |
| `--par` | 内容与数据格式 | 导出 Apache Parquet 列式数据文件。 | 格式选择 | S1 |
| `--parallel` | 导入侧/未公开 | OBServer 旁路/直接导入写入和排序的并行度，不是 OBDUMPER 导出并发。 | 不进入导出配置 | S7/H |
| `--parallel-macro` | 性能与资源 | 每个导出线程处理的宏块数，默认 8。 | 高级配置 | S1 |
| `--partition` | 数据筛选与一致性 | 只导出指定分区数据；不能与 `--query-sql` 同用，二级分区需指定二级分区名。 | 专家配置 | S1 |
| `--password` | 连接与会话 | 数据库账号密码；命令行可省略表示无密码账户。 | 数据源快照，脱敏 | S1 |
| `--port` | 连接与会话 | ODP 或物理节点 SQL 端口；4.2.6 起支持多个端口。 | 数据源快照 | S1 |
| `--pos` | 内容与数据格式 | 固定长度数据格式。本地 help 列出独立 `--pos`；4.3.5 官网正文写为 CUT+空 splitter+控制文件，旧版官方用例又使用 `--pos`，口径冲突。 | 格式选择，提交前待实测 | S1/S4/S6/H |
| `--preserve-zero-datetime` | 日期时间序列化 | 保留 MySQL 模式 DATE/DATETIME/TIMESTAMP 零值的原始形式。 | 专家配置 | S1 |
| `--procedure` | 数据库对象 | 导出存储过程定义；MySQL 模式受数据库版本限制。 | 对象选择 | S1 |
| `--public-cloud` | 连接与会话 | 标识云数据库 OceanBase 环境，并默认启用无 sys 模式。 | 数据源快照 | S1 |
| `--public-synonym` | 数据库对象/未公开 | 本地 help Usage 接受公共同义词清单，但 4.3.5 官网未列出含义和适用条件。 | 不展示，待官方/实测确认 | H |
| `--query-sql` | 数据筛选与一致性 | 导出自定义单条查询结果；可直接传 SQL 或 `file://` 文件，不能与 where/partition 同用。 | 专家配置 | S1 |
| `--remove-newline` | 文本序列化 | 强制删除数据中的回车和换行，仅 CUT；会改变导出数据。 | 专家配置，高风险 | S1 |
| `--retain-empty-files` | 输出与文件 | where 或 partition 结果为空时仍生成空文件；实际参数为复数。 | 高级配置 | S1/H |
| `--retain-schema` | DDL 与对象处理 | 在表和同义词 DDL 中保留 `schema.object` 前缀。 | 专家配置 | S1 |
| `--retry` | 性能与资源 | 从 `--file-path` 下 `dump.ckpt` 最近保存点继续导出。 | 失败任务操作 | S1 |
| `--role` | 数据库对象 | 导出角色定义；Oracle 模式及公有云限制需校验。 | 对象选择 | S1 |
| `--sequence` | 数据库对象 | 导出序列定义。 | 对象选择 | S1 |
| `--sequence-policy` | DDL 与对象处理 | 序列导出策略：restart 或 preserve，默认 preserve。 | 专家配置 | S3/H |
| `--server` | 运行模式/未公开 | 本地 help 描述为在 server 中远程 load/dump；4.3.5 官网无部署语义。 | 不展示，待官方/实测确认 | H |
| `--session-config` | 连接与会话 | 指定连接配置文件；默认配置无需显式指定。 | 执行节点/专家配置 | S1 |
| `--skip-check-dir` | 输出与文件 | 跳过输出目录非空检查，可能覆盖同名文件。 | 专家配置，高风险 | S1 |
| `--skip-header` | CSV 序列化 | 控制是否省略 CSV 第一行字段头。 | 高级配置 | S1 |
| `--snapshot` | 数据筛选与一致性 | 导出历史版本以获得全局一致性快照；仅与数据格式搭配。 | 专家配置 | S1 |
| `--sql` | 内容与数据格式 | 将每行表数据导出为一条 INSERT 语句。 | 格式选择 | S1 |
| `--storage-uri` | 废弃兼容 | 旧版对象存储 URI；官方 4.3.0 起废弃并整合到 `--file-path`。 | 不展示，兼容旧任务 | S2/H |
| `--synonym` | 数据库对象 | 导出同义词定义；官网注明暂不支持 MySQL 模式。 | 对象选择 | S1 |
| `--sys-password` | 连接与会话 | sys 租户特权账户密码；与 `--sys-user` 配合，OceanBase 4.0+ 通常无需指定。 | 任务预检查/安全配置 | S1 |
| `--sys-user` | 连接与会话 | sys 租户特权用户名，默认 root；用于读取系统元数据。 | 任务预检查/安全配置 | S1 |
| `--table` | 数据库对象 | 导出指定表定义或表数据。 | 对象选择 | S1 |
| `--table-group` | 数据库对象 | 导出表组定义。 | 对象选择 | S1 |
| `--tenant` | 连接与会话/未公开 | 本地 help 将其作为租户名；4.3.5 官网主要要求通过 `--user` 的 `user@tenant#cluster` 组合表达。 | 数据源快照，独立参数待实测 | S1/H |
| `--thread` | 性能与资源 | OBDUMPER 导出线程数；默认 CPU×2、上限 32，导出多个 DDL 时建议不超过 4。 | 高级配置 | S1 |
| `--time-value-format` | 日期时间序列化 | 设置 MySQL 模式 TIME 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--timestamp-ltz-value-format` | 日期时间序列化 | 设置 Oracle TIMESTAMP WITH LOCAL TIME ZONE 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--timestamp-tz-value-format` | 日期时间序列化 | 设置 Oracle TIMESTAMP WITH TIME ZONE 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--timestamp-value-format` | 日期时间序列化 | 设置 MySQL/Oracle TIMESTAMP 值导出格式，仅 CSV/CUT。 | 专家配置 | S1 |
| `--tmp-path` | 输出与文件 | 对象存储 Multipart Upload 使用的本地临时分块及日志路径。 | 执行节点/高级配置 | S1/H |
| `--trail-delimiter` | CUT/POS 序列化 | 控制导出数据行是否保留行尾最后一个列分隔符。 | 高级配置 | S1 |
| `--trigger` | 数据库对象 | 导出触发器定义，主要适用于 Oracle 兼容模式。 | 对象选择 | S1 |
| `--type` | 数据库对象 | 导出类型定义。 | 对象选择 | S1 |
| `--type-body` | 数据库对象 | 导出类型体定义，依赖对应类型。 | 对象选择 | S1 |
| `--upload-behavior` | 废弃兼容 | 旧版对象存储上传策略 FAST/COMPLETE；官方 4.3.0 起废弃。 | 不展示，兼容旧任务 | S2/H |
| `--user` | 连接与会话 | 数据库用户名；官网推荐组合格式 `user@tenant#cluster`。 | 数据源快照 | S1 |
| `--version` | CLI 元参数 | 显示 OBDUMPER 版本。 | 节点版本核验 | S1/H |
| `--view` | 数据库对象 | 导出视图定义。 | 对象选择 | S1 |
| `--weak-read` | 数据筛选与一致性 | 从备副本读取并导出表数据，不等于备集群。 | 专家配置 | S1 |
| `--where` | 数据筛选与一致性 | 按条件表达式过滤数据；仅与数据格式搭配且不能与 `--query-sql` 同用。 | 专家配置 | S1 |
| `--with-trim` | 文本序列化 | 删除导出值左右空格字符。 | 高级配置 | S1 |

## 5. 导出模块需补充的分类

现有导出设计需要明确补充以下分类，而不是继续把所有参数放入“执行参数”：

1. **连接与会话**：数据源快照、sys 安全配置、逻辑库、字符集、会话配置必须区分；向导不得重复填写基础连接。
2. **CSV、CUT/POS 与日期时间序列化**：格式专属参数应随格式和兼容模式条件显示。
3. **DDL 与对象处理**：`--drop-object`、`--add-extra-message`、`--retain-schema`、`--compact-schema`、`--sequence-policy` 单列为专家能力。
4. **数据筛选与一致性**：where、partition、query、flashback、snapshot、weak-read、列筛选和隐藏主键统一进入任务级预检查。
5. **输出与文件**：区分主输出路径、日志、控制文件、对象存储临时目录、文件切分、空文件和目录安全。
6. **性能与资源**：区分导出线程、宏块并行、分页、游标抓取、JVM 内存、总量上限和检查点恢复。
7. **兼容治理**：导入侧参数、废弃参数、help-only 参数不得混入可配置参数清单。

## 6. 需要补入导出模块的参数候选

- **高级配置**：`--column-quote-mode`、`--line-separator`、`--file-encoding`、`--null-string`、`--skip-header`、`--trail-delimiter`、`--compress`、`--compression-algo`、`--compression-level`、`--tmp-path`、`--mem`、`--fetch-size`、`--parallel-macro`、`--max-file-size`。
- **专家配置**：日期时间值格式、Oracle NLS 会话格式、`--drop-object`、`--add-extra-message`、`--retain-schema`、`--compact-schema`、`--sequence-policy`、列/表筛选、闪回、快照、弱读、逻辑库和高风险目录选项。
- **不进入新建表单**：`--mix`、`--file-suffix`、`--ignore-escape`、`--parallel`、`--commit-size`、`--server`、`--public-synonym`、`--storage-uri`、`--file-name`、`--upload-behavior`、`--distinct`、`--column-delimiter`。

## 7. 仍需受控实测

- POS 最终生成独立 `--pos` 还是 CUT 组合，以及二者对控制文件的要求。
- `--block-size` 默认值：官网正文为 0，选项表为 1024MB。
- `--compact-schema`、`--sequence-policy` 在 4.3.5 当前页面缺少完整约束。
- help-only 的 `--server`、`--public-synonym`、`--commit-size` 是否为可用导出能力。
- `--tenant` 独立长参数与官网推荐 `user@tenant#cluster` 连接形式的实际优先级。
- 对象存储 `--tmp-path` 的空间预检查以及敏感 URI 脱敏规则。

## 8. Windows 4.3.5 输出路径验证基线

官方把 `-f/--file-path` 定义为本地磁盘绝对路径。当前 Windows 4.3.5 实测中，直接传 `E:\...`、默认嵌套输出、`--no-nested-dir` 或 `file:///E:/...` 都会在写文件阶段形成 `file://nullE:/...` 错误。

已验证的兼容方式不是改变用户配置语义，而是在执行层做确定性映射：

- 产品配置、预检查和任务快照仍保存规范化绝对输出目录；
- Agent 将进程工作目录设为受控绝对父目录；
- 实际命令的 `--file-path` 使用 `/` 风格相对路径，例如 `./task-output`；
- 禁止使用 `\.\task-output`，4.3.5 实测会报 URI 非法字符；
- 命令预览和执行证据同时展示工作目录、实际参数与解析后的绝对目录；
- Agent 必须在启动前再次解析并校验目标仍位于允许的输出根目录内，防止目录穿越。
- 每次执行在绝对输出根目录下创建唯一任务子目录；不得默认复用非空目录或自动加入 `--skip-check-dir` 覆盖既有结果。

该适配已连续两次导出同一张 10,000 行表成功，两份 CSV 的行列数、字节数和 SHA-256 一致。它属于 Windows 4.3.5 执行适配，不改变官方参数定义，也不推断其他版本存在相同行为。
