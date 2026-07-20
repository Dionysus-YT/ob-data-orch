# OBLOADER V4.3.5 普通导入参数映射基线

> 文档状态：103 个长参数已逐项建档；help-only 与历史兼容项待受控确认  
> 核验日期：2026-07-17  
> 发布包：`ob-loader-dumper-4.3.5-RELEASE.zip`  
> 发布包 SHA-256：`C1A5D5EE053106803263015F00EC7F0B94B73EA4015A2E53CF1AFFF598983493`  
> 实际程序版本：`4.3.5-RELEASE`

## 1. 核查结论

- 实际运行 `obloader --help` 共检出 **103 个唯一长参数**，下表逐项给出含义、分类、普通导入处置和来源，名称覆盖无遗漏。
- 参数出现在共享帮助中不等于适用于普通导入。`--direct`、`--rpc-port`、`--parallel` 是旁路导入专属；`--enable-hidden-pk` 的官方资料确认的是导出用途。
- `--storage-uri` 已被 `--file-path` 统一承载，`--column-delimiter` 是 `--column-quote` 的过时别名；两者只用于历史命令识别。
- `--buffer-size`、`--server`、`--public-synonym` 当前只有本地 help 字样，缺少 V4.3.5 官方普通导入语义，不进入新建任务。
- 所有经核验的普通导入有效参数均进入基础、格式、高级、专家、派生或失败任务操作，不以简化界面为由删减能力。

## 2. 来源

- **S1**：[OBLOADER V4.3.5 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997308)：当前版本主要事实来源。
- **S2**：[OBLOADER V4.3.5 命令行应用示例](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997306)：自动列映射与 Hive 路径示例。
- **S3**：[OBLOADER & OBDUMPER V4.3.0 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002055811)：压缩、Avro 与 `--storage-uri` 调整。
- **S4**：[OBLOADER & OBDUMPER V4.3.2 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002265549)：Hive 来源和 JVM 内存等能力。
- **S5**：[导数工具 V4.2.8 发布说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997334)：三段式用户及 tenant/cluster 兼容别名。
- **S6**：[OBLOADER V4.2.7 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000000381203)：`--external-data` 历史官方语义。
- **H**：项目发布包中实际执行 `obloader 4.3.5-RELEASE --help`；用于核对二进制声明，不单独构成产品能力确认。

## 3. 分类说明

| 分类 | 说明 |
|---|---|
| 连接与会话 | 数据源、安全配置或专家会话参数 |
| 文件与匹配 | 输入路径、编码、后缀、正则、控制文件和第三方文件 |
| 内容与格式 | DDL、MIX 与七种数据格式 |
| 对象范围 | 全部、表及定义对象 |
| 列与解析 | 列映射、列名单、类型排除及文本解析 |
| 错误与数据行为 | 错误上限、严格退出、替换和清空/删除 |
| 性能与资源 | 线程、批次、限速、阈值、内存和检查点 |
| 旁路专属 | 普通导入不得生成 |
| 废弃/兼容 | 只读识别历史命令，新建任务不用 |
| 未确认/非普通导入 | 共享 help 存在，但普通导入官方语义不足或属于导出侧 |
| CLI 元参数 | 不属于任务配置 |

分类计数：对象范围 16、列与解析 19、内容与格式 9、性能与资源 10、连接与会话 16、文件与匹配 12、错误与数据行为 8、旁路专属 3、废弃/兼容 4、未确认/非普通导入 4、CLI 元参数 2，合计 103。

## 4. 103 个参数逐项映射

| 参数 | 分类 | 官方含义/核查结论 | 普通导入产品处置 | 来源 |
|---|---|---|---|---|
| `--all` | 对象范围 | 导入全部已支持对象定义和对应格式表数据；与任意具体对象参数互斥。 | 普通模式，范围单选 | S1 |
| `--auto-column-mapping` | 列与解析 | 按源文件列名与目标表列名自动对应，允许列数不同；支持 CSV、ORC、Parquet，不能与控制文件同用。 | 高级配置，条件显示 | S1/S2 |
| `--avro` | 内容与格式 | 导入 Apache Avro；支持 Primary/Logical Types，不支持 Complex Types。 | 格式选择 | S1/S3 |
| `--batch` | 性能与资源 | 每批事务记录数，文档默认 200；V4.2.0 起可按 JVM 内存自适应默认值。 | 高级配置 | S1 |
| `--block-size` | 性能与资源 | 大文件逻辑切分阈值，默认单位 MB，默认 64，不产生额外物理子文件。 | 高级配置 | S1 |
| `--buffer-size` | 未确认/非普通导入 | 本地 help 仅说明 ring buffer capacity，官网未给出单位、范围和普通导入适用条件。 | 不展示，待官方确认 | H |
| `--character-set` | 连接与会话 | 创建数据库连接使用的字符集，覆盖会话配置的 JDBC 字符编码。 | 专家配置/数据源派生 | S1 |
| `--cluster` | 废弃/兼容 | 旧式独立集群参数；V4.2.8 起官方优先使用 `<user>@<tenant>#<cluster>`，保留兼容。 | 数据源派生；新任务不单独编辑 | S5/H |
| `--column-delimiter` | 废弃/兼容 | CSV 字符串定界符，是 `--column-quote` 的过时同义参数，不能同时指定。 | 不展示，历史识别 | S1 |
| `--column-quote` | 列与解析 | CSV 字符串包围符，仅支持单字符，默认英文单引号。 | 高级配置，仅 CSV | S1 |
| `--column-separator` | 列与解析 | CSV 列分隔符，默认逗号；V4.3.5 起支持多字符。 | 高级配置，仅 CSV | S1 |
| `--column-splitter` | 列与解析 | CUT 列分隔字符串，区别于 CSV 分隔符。 | 高级配置，仅 CUT | S1 |
| `--compat-mode` | 连接与会话 | 兼容性导入 MySQL 5.6/5.7/8.0 表定义并转换为 OceanBase MySQL 同义语句。 | 专家配置，仅 DDL/MIX 适用场景 | S1 |
| `--compress` | 文件与匹配 | 标识输入是经 OBDUMPER 压缩导出的文件，支持 CSV、CUT、POS、SQL。 | 高级配置，条件显示 | S1/S3 |
| `--compression-algo` | 文件与匹配 | 输入文件压缩算法：zstd、zlib、gzip、snappy，默认 zstd。 | 启用压缩后必填/继承默认 | S1/S3 |
| `--compression-level` | 文件与匹配 | OBDUMPER 压缩等级；zstd 1～22，zlib -1～9，gzip/snappy 不支持。 | 条件配置 | S1/S3 |
| `--csv` | 内容与格式 | 导入标准 CSV 数据文件，官方推荐格式。 | 格式选择 | S1 |
| `--ctl-path` | 文件与匹配 | 控制文件所在目录；POS 必须使用，也可承载官方字段映射/预处理。 | 专家配置；POS 必填 | S1 |
| `--cut` | 内容与格式 | 导入字符串分隔的 CUT 数据文件。 | 格式选择 | S1 |
| `--database` | 连接与会话 | 目标数据库/Schema。 | 普通模式，必填 | S1 |
| `--date-format` | 列与解析 | MySQL DATE/DATETIME 原数据格式，默认 `yyyy-MM-dd HH:mm:ss`，与 `--default-date` 配合。 | 专家配置，条件显示 | S1 |
| `--ddl` | 内容与格式 | 导入对象定义文件；OceanBase V4.2.1 起支持并行导入 DDL。 | 内容选择 | S1 |
| `--default-date` | 列与解析 | MySQL DATE/DATETIME 解析失败时使用的替代值，无默认值。 | 专家配置，依赖 date-format | S1 |
| `--delete-from-table` | 错误与数据行为 | 导入数据前 DELETE 目标表；与 all/`table '*'` 组合可能删除所有匹配表，即使路径中没有对应文件。 | 专家配置，强风险二次确认 | S1 |
| `--direct` | 旁路专属 | 开启旁路导入，与 RPC 端口和服务端并行度搭配。 | 普通导入不显示、不生成 | S1/H |
| `--empty-string` | 列与解析 | 指定值按空字符串处理，默认 `\E`。 | 高级配置，适用格式显示 | S1 |
| `--enable-hidden-pk` | 未确认/非普通导入 | 共享 help 说明使用隐藏主键；当前官方资料确认的是 OBDUMPER 导出加速语义，未确认普通导入能力。 | 不展示、不生成 | H |
| `--escape-character` | 列与解析 | CSV/CUT 转义字符，仅支持单字符；CSV 默认 null，CUT 默认反斜杠。 | 高级配置，仅 CSV/CUT | S1 |
| `--exclude-column-names` | 列与解析 | 排除指定目标列；列名大小写需一致，不能与控制文件同时生效。 | 专家配置 | S1 |
| `--exclude-data-types` | 列与解析 | 跳过导入指定数据类型的数据。 | 专家配置 | S1 |
| `--exclude-table` | 对象范围 | 排除指定表定义和数据，支持官方模糊匹配表达式。 | 高级配置 | S1 |
| `--external-data` | 文件与匹配 | 历史官方语义为第三方文件模式，跳过 OBDUMPER MANIFEST 校验；4.3.5 当前页未列入选项表。 | 高级配置，标注待实测 | S6/H |
| `--file-encoding` | 文件与匹配 | 读取输入文件使用的编码，区别于数据库连接字符集，默认 UTF-8。 | 高级配置 | S1 |
| `--file-path` | 文件与匹配 | 输入数据目录、单文件绝对路径或官方支持的存储 URI；必选。 | 普通模式，必填 | S1/S3 |
| `--file-regular-expression` | 文件与匹配 | 用正则表达式选择输入文件；V4.3.2 起支持。 | 高级配置，带匹配预览 | S1 |
| `--file-suffix` | 文件与匹配 | 自定义输入文件后缀；CSV/SQL/CUT/POS 有官方默认后缀。 | 基础/高级配置 | S1 |
| `--function` | 对象范围 | 导入函数定义，不导入数据。 | 对象选择 | S1 |
| `--help` | CLI 元参数 | 显示命令行帮助。 | 不进入任务配置 | S1/H |
| `--host` | 连接与会话 | ODP 或 OceanBase 物理节点地址。 | 数据源快照，脱离表单编辑 | S1 |
| `--ignore-escape` | 列与解析 | 导入 CUT 时忽略字符转义，默认不忽略。 | 专家配置，仅 CUT | S1 |
| `--ignore-unhex` | 列与解析 | 不解码十六进制字符串，仅二进制类型且 MySQL 兼容模式生效。 | 专家配置，条件显示 | S1 |
| `--include-column-names` | 列与解析 | 只向指定列按给定顺序导入数据，列名大小写需一致。 | 专家配置 | S1 |
| `--line-separator` | 列与解析 | CSV、CUT、POS、SQL 的行分隔符；可选 `\r`、`\n`、`\r\n`。 | 高级配置，条件显示 | S1 |
| `--log-path` | 文件与匹配 | 运行日志输出目录；未设置时默认输出到 `--file-path` 目录。 | 高级配置 | S1 |
| `--logical-database` | 连接与会话 | 标识连接 ODP Sharding 逻辑库进行导入。 | 数据源派生/专家摘要 | S1 |
| `--max-discards` | 错误与数据行为 | 单表允许的重复数据最大值，默认 -1 表示忽略重复继续；不适用于旁路导入。 | 高级配置 | S1 |
| `--max-errors` | 错误与数据行为 | 单表数据库写入错误最大值，可为 0、-1 或正整数，默认 1000；不适用于旁路导入。 | 高级配置 | S1 |
| `--max-tps` | 性能与资源 | 限制最大导入 TPS，默认单位行/秒。 | 高级配置 | S1 |
| `--max-wait-timeout` | 性能与资源 | OceanBase 合并期间最大等待时间，默认单位小时，默认 3。 | 高级配置 | S1 |
| `--mem` | 性能与资源 | JVM 内存大小；V4.3.2 发布说明给出默认 4G。 | 执行节点/高级配置 | S4/H |
| `--mix` | 内容与格式 | 导入 DDL 与 DML 混合文本；兼容性强但全量读入内存、性能低，不适合大文件。 | 独立内容模式，风险提示 | S1 |
| `--nls-date-format` | 连接与会话 | 设置 Oracle 会话 `nls_date_format`。 | 专家配置，仅 Oracle | S1 |
| `--nls-timestamp-format` | 连接与会话 | 设置 Oracle 会话 `nls_timestamp_format`。 | 专家配置，仅 Oracle | S1 |
| `--nls-timestamp-tz-format` | 连接与会话 | 设置 Oracle 会话 `nls_timestamp_tz_format`。 | 专家配置，仅 Oracle | S1 |
| `--no-sys` | 连接与会话 | 私有部署无法提供 sys 租户密码时使用。 | 数据源/安全配置派生 | S1 |
| `--null-string` | 列与解析 | 指定值按 NULL 处理，默认 `\N`。 | 高级配置，适用格式显示 | S1 |
| `--obj-user` | 对象范围 | 导入用户定义；有权限要求并默认忽略系统用户。 | 对象选择，权限预检查 | S1 |
| `--orc` | 内容与格式 | 导入 Apache ORC 列式文件。 | 格式选择 | S1 |
| `--package` | 对象范围 | 导入程序包定义，Oracle 兼容模式。 | 对象选择 | S1 |
| `--package-body` | 对象范围 | 导入程序包体定义，Oracle 兼容模式。 | 对象选择 | S1 |
| `--par` | 内容与格式 | 导入 Apache Parquet 列式文件。 | 格式选择 | S1 |
| `--parallel` | 旁路专属 | 旁路导入服务端加载并行度，与 `--direct`、`--rpc-port` 搭配。 | 普通导入不显示、不生成 | S1/H |
| `--password` | 连接与会话 | 数据库密码。 | 数据源快照，脱敏 | S1 |
| `--pause` | 性能与资源 | 进入停导模式的 OceanBase 内存水位阈值，默认 0.85。 | 高级配置 | S1 |
| `--port` | 连接与会话 | ODP 或 OceanBase 物理节点 SQL 端口。 | 数据源快照 | S1 |
| `--pos` | 内容与格式 | 导入定长字节 POS 文件，必须与控制文件配合。 | 格式选择 | S1 |
| `--procedure` | 对象范围 | 导入存储过程定义，不导入数据。 | 对象选择 | S1 |
| `--public-cloud` | 连接与会话 | 标识云数据库 OceanBase 环境；云上旁路还需 tenant，但普通导入仍按数据源环境派生。 | 数据源快照 | S1 |
| `--public-synonym` | 未确认/非普通导入 | 仅在本地 Usage 中接受公共同义词清单，当前 V4.3.5 官网未给出普通导入语义。 | 不展示，待官方确认 | H |
| `--replace-data` | 错误与数据行为 | 替换重复数据，仅适用于有主键或含非空列唯一键的表；性能低于空表导入。 | 专家配置，高风险 | S1 |
| `--replace-object` | 错误与数据行为 | DDL/MIX 导入时强制替换已存在对象；表/同义词可能先删后建。 | 专家配置，强风险 | S1 |
| `--retry` | 性能与资源 | 从 `--file-path` 下 `load.ckpt` 最近保存点继续；不适用于旁路导入。 | 失败任务操作 | S1 |
| `--role` | 对象范围 | 导入角色定义，仅 Oracle 兼容模式，并有权限和默认忽略角色规则。 | 对象选择 | S1 |
| `--rpc-port` | 旁路专属 | 旁路导入连接 OBServer/ODP 的 RPC 端口。 | 普通导入不显示、不生成 | S1/H |
| `--rw` | 性能与资源 | 文件解析线程占总线程数的比例，默认 1；解析线程数为 `thread × rw`。 | 高级配置 | S1 |
| `--sequence` | 对象范围 | 导入序列定义，仅 Oracle 兼容模式。 | 对象选择 | S1 |
| `--server` | 未确认/非普通导入 | 本地 help 仅说明 remote load/dump in server，官网没有部署、权限和路径语义。 | 不展示，待官方确认 | H |
| `--session-config` | 连接与会话 | 指定连接配置文件；工具目录已有默认配置，通常无需显式指定。 | 执行节点/专家配置 | S1 |
| `--skip-footer` | 列与解析 | 导入 CUT 时跳过最后一行。 | 高级配置，仅 CUT | S1 |
| `--skip-header` | 列与解析 | 跳过 CSV/CUT 第一行；CUT 自 V3.3.0 起支持。 | 高级配置，仅 CSV/CUT | S1 |
| `--slow` | 性能与资源 | 进入慢导模式的 OceanBase 内存水位阈值，默认 0.75。 | 高级配置 | S1 |
| `--source-type` | 文件与匹配 | 当前只支持 `hive`；解析路径中的 `key=value` 为分区列，支持 ORC/Parquet。 | 高级配置，条件显示 | S1/S2/S4 |
| `--sql` | 内容与格式 | 导入 INSERT SQL 数据文件；V4.3.1 起支持 Batch Insert 文本。 | 格式选择 | S1 |
| `--storage-uri` | 废弃/兼容 | 旧式远程存储参数；V4.3.0 起存储语义整合到 `--file-path`。 | 不展示，历史识别 | S3/H |
| `--strict` | 错误与数据行为 | 脏数据是否影响进程退出状态，默认 true；false 时坏/丢弃记录不改变最终退出码。 | 高级配置 | S1 |
| `--synonym` | 对象范围 | 导入同义词定义，仅 Oracle 兼容模式。 | 对象选择 | S1 |
| `--sys-password` | 连接与会话 | sys 租户特定用户密码。 | 安全配置派生，脱敏 | S1 |
| `--sys-user` | 连接与会话 | sys 租户用户名。 | 安全配置派生 | S1 |
| `--table` | 对象范围 | 导入表定义或表数据；支持多个名称、`*` 和大小写精确表达。 | 对象选择 | S1 |
| `--table-group` | 对象范围 | 导入表组定义，不导入数据。 | 对象选择 | S1 |
| `--tenant` | 废弃/兼容 | 旧式独立租户参数；V4.2.8 起优先合并到三段式 user，保留兼容。 | 数据源派生；新任务不单独编辑 | S5/H |
| `--thread` | 性能与资源 | 写入并发；默认 CPU×2，CPU 大于 16 时默认上限 32；V4.2.1 起也用于并行 DDL。 | 高级配置 | S1 |
| `--tmp-path` | 文件与匹配 | 远程存储等场景的本地缓冲数据和日志目录。 | 执行节点/高级配置 | S1/H |
| `--trail-delimiter` | 列与解析 | 标识 CUT 数据行以列分隔符结束，解析时去除行尾最后一个分隔符。 | 高级配置，仅 CUT | S1 |
| `--trigger` | 对象范围 | 导入触发器定义，不导入数据。 | 对象选择 | S1 |
| `--truncate-table` | 错误与数据行为 | 导入数据前 Truncate 目标表；官方不推荐自动使用。 | 专家配置，强风险二次确认 | S1 |
| `--type` | 对象范围 | 导入类型定义，仅 Oracle 兼容模式。 | 对象选择 | S1 |
| `--type-body` | 对象范围 | 导入类型体定义，仅 Oracle 兼容模式。 | 对象选择 | S1 |
| `--user` | 连接与会话 | 数据库用户名；官方推荐 `<user>@<tenant>#<cluster>` 三段式。 | 数据源快照 | S1/S5 |
| `--version` | CLI 元参数 | 显示工具版本。 | 不进入任务配置 | S1/H |
| `--view` | 对象范围 | 导入视图定义，不导入数据。 | 对象选择 | S1 |
| `--with-trim` | 列与解析 | 删除输入数据左右空格。 | 高级配置，仅适用文本格式 | S1 |
| `--yes` | 错误与数据行为 | 跳过 truncate/delete 的工具交互确认。 | 平台二次确认后按需派生，不作独立开关 | S1 |

## 5. 普通导入能力边界汇总

### 可直接配置

- 内容与格式、对象范围、输入与匹配、列解析、错误处理、性能和会话参数。
- 高风险项仍是官方能力，只是进入专家配置和二次确认，不因风险被删除。

### 派生但必须可见

- `--host`、`--port`、`--user`、`--password`、`--database` 及环境相关连接参数。
- `--sys-user`、`--sys-password`、`--session-config`、`--tmp-path` 可由安全配置或执行节点派生。
- `--yes` 只能在平台已经完成对应风险确认后，为非交互执行派生。

### 不进入新建普通导入

| 类型 | 参数 |
|---|---|
| 旁路导入专属 | `--direct`、`--rpc-port`、`--parallel` |
| CLI 元参数 | `--help`、`--version` |
| 废弃别名 | `--storage-uri`、`--column-delimiter` |
| 旧连接兼容 | `--tenant`、`--cluster`（仍由数据源快照兼容派生，不单独编辑） |
| 当前语义不足 | `--buffer-size`、`--server`、`--public-synonym` |
| 官方仅确认导出侧 | `--enable-hidden-pk` |

## 6. 覆盖验收

- 基线中的参数名称集合必须与实际 `obloader --help` 提取的 103 个唯一长参数完全相等。
- 新增或升级 OBLOADER 版本时，先重跑 help 差异，再更新官方来源、分类、字段规则和验证计划。
- “工具接受参数”不能单独作为普通导入能力确认；至少需要官方语义或受控验证证据。
- 页面字段数量可以少于参数数量，因为连接和安全参数可派生，但最终命令和任务快照不得丢失活动参数。
