# OBLOADER V4.3.5 旁路导入参数映射基线

> 文档状态：103 个长参数已逐项建档；旁路格式与共享参数适用性待官方确认/受控验证  
> 核验日期：2026-07-17  
> 发布包：`ob-loader-dumper-4.3.5-RELEASE.zip`  
> 发布包 SHA-256：`C1A5D5EE053106803263015F00EC7F0B94B73EA4015A2E53CF1AFFF598983493`  
> 实际程序版本：`4.3.5-RELEASE`

## 1. 核查结论

- 实际 `obloader --help` 的 **103 个唯一长参数**全部进入本表，集合与普通导入基线一致，旁路处置逐项重判，零缺失。
- 旁路不是只有 `--direct`、`--rpc-port`、`--parallel` 三个参数；连接、数据库、单表、文件、格式、解析、并发和风险行为共同构成可执行命令。
- 当前 V4.3.5 旁路专题页明确了 Direct Load 核心参数、连接场景、版本、单表、结构、提交和失败边界；当前官方旁路操作示例明确使用 CSV、external-data、truncate、column-separator、thread。
- CSV 有 V4.3.5 通用旁路示例；OceanBase Cloud Direct Load 官方页另明确列出 SQL、Parquet 和 ORC。后三者的证据边界为云 ODP，不自动扩展到私有 ODP/OBServer。
- CUT、POS、Avro 具有普通导入官方语义，但未找到可与 `--direct` 组合的官方证据。
- `--retry`、`--max-errors`、`--max-discards` 官方明确不适用于旁路；DDL、MIX、all 和定义对象与单表数据旁路边界不符。
- 发布包 Direct Load 配置值与官网示例值冲突，平台必须展示执行节点实际配置，不能硬编码官网示例。

## 2. 来源

- **S1**：[OBLOADER V4.3.5 旁路导入](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997310)：Direct Load 核心事实来源。
- **S2**：[OBLOADER V4.3.5 命令行选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997308)：参数含义、格式、错误处理与 direct 模式条件。
- **S3**：[使用 OBLOADER 旁路导入数据](https://www.oceanbase.com/docs/common-oceanbase-database-cn-1000000001573614)：官方 CSV 旁路命令示例和连接说明。
- **S4**：[OBLOADER & OBDUMPER V4.3.0 发布说明](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002055811)：压缩、Avro、storage-uri 调整。
- **S5**：[导数工具 V4.2.8 发布说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997334)：三段式用户和 tenant/cluster 兼容信息。
- **S6**：[OceanBase Cloud Direct load](https://en.oceanbase.com/docs/common-oceanbase-cloud-10000000001867623)：云 ODP 旁路示例及 CSV、SQL、Parquet、ORC 格式证据。
- **H**：项目发布包实际 `obloader 4.3.5-RELEASE --help`。
- **C**：项目发布包实际 `conf/session.config.json`。

## 3. 旁路适用状态

| 状态 | 含义 | 产品处置 |
|---|---|---|
| 旁路已确认 | 专题页、命令行页或官方旁路示例有明确证据 | 可配置或按场景派生 |
| 明确不适用 | 官方明确不适用于旁路，或与单表数据边界冲突 | 不进入新建旁路任务 |
| 待官方参数映射确认 | 参数的一般 OBLOADER 含义明确，但 Direct Load 适用性没有充分证据 | 保留元数据，验证前不写成确定能力 |
| 废弃/兼容 | 新参数已有替代或仅为旧连接方式 | 历史识别或场景派生 |
| CLI 元参数 | 不属于任务配置 | 不进入表单 |

分类计数：旁路已确认 24、明确不适用 31、待官方参数映射确认 44、废弃/兼容 2、CLI 元参数 2，合计 103。

## 4. 103 个参数逐项映射

| 参数 | 分类 | 官方含义/旁路核查结论 | 旁路产品处置 | 来源 |
|---|---|---|---|---|
| `--all` | 明确不适用 | 导入全部对象/表数据；旁路官方明确禁止多表。 | 不显示、不生成 | S1/S2 |
| `--auto-column-mapping` | 待官方参数映射确认 | CSV/ORC/Parquet 按列名自动映射；当前旁路专题未说明是否支持。 | 保留映射，验证前不开放 | S2 |
| `--avro` | 待官方参数映射确认 | 导入 Avro；普通导入支持 Primary/Logical Types，不支持 Complex Types；旁路适用性未确认。 | 格式暂不开放 | S2/S4 |
| `--batch` | 待官方参数映射确认 | 普通导入每批事务记录数；旁路按表整体提交，是否影响 Direct Load 未说明。 | 验证前不开放 | S2 |
| `--block-size` | 待官方参数映射确认 | 普通导入大文件逻辑切分阈值；Direct Load 是否共用未说明。 | 验证前不开放 | S2 |
| `--buffer-size` | 明确不适用 | help-only 的 ring buffer capacity，无当前官网语义。 | 不显示、不生成 | H |
| `--character-set` | 待官方参数映射确认 | 创建数据库连接使用的字符集；旁路连接适用性合理但专题未逐项说明。 | 专家映射保留，验证前标记 | S2 |
| `--cluster` | 旁路已确认 | 私有 OceanBase + ODP 场景集群名；OBServer 直连不使用。 | 数据源按连接场景派生 | S1 |
| `--column-delimiter` | 废弃/兼容 | CSV 包围符的过时别名，与 column-quote 同义。 | 不显示，历史识别 | S2 |
| `--column-quote` | 待官方参数映射确认 | CSV 字符串包围符；旁路 CSV 是否完整支持该解析项未单独说明。 | CSV 专家映射保留 | S2 |
| `--column-separator` | 旁路已确认 | CSV 列分隔符；官方旁路命令示例使用。 | CSV 高级配置 | S2/S3 |
| `--column-splitter` | 待官方参数映射确认 | CUT 列分隔字符串；CUT 的旁路适用性未确认。 | 格式未确认前不开放 | S2 |
| `--compat-mode` | 明确不适用 | 用于兼容导入 MySQL 表定义；旁路只导入表数据。 | 不显示、不生成 | S2 |
| `--compress` | 待官方参数映射确认 | 读取经 OBDUMPER 压缩的 CSV/CUT/POS/SQL；Direct Load 适用性未说明。 | 验证前不开放 | S2/S4 |
| `--compression-algo` | 待官方参数映射确认 | 压缩输入算法 zstd/zlib/gzip/snappy。 | 依赖 compress，验证前不开放 | S2/S4 |
| `--compression-level` | 待官方参数映射确认 | 压缩等级；zstd/zlib 有范围，gzip/snappy 不支持。 | 依赖 compress，验证前不开放 | S2/S4 |
| `--csv` | 旁路已确认 | 导入 CSV 数据；当前官方旁路示例明确使用。 | 当前已确认格式 | S2/S3 |
| `--ctl-path` | 待官方参数映射确认 | 控制文件目录；POS 必需，其他解析/预处理可用；旁路适用性未确认。 | 验证前不开放 | S2 |
| `--cut` | 待官方参数映射确认 | 导入 CUT 数据；旁路专题无格式矩阵。 | 格式暂不开放 | S2 |
| `--database` | 旁路已确认 | 目标数据库/Schema，官方旁路示例必备。 | 普通模式，必填 | S2/S3 |
| `--date-format` | 待官方参数映射确认 | MySQL DATE/DATETIME 原数据格式，与 default-date 配合；旁路适用性未确认。 | 验证前不开放 | S2 |
| `--ddl` | 明确不适用 | 导入对象定义；旁路是单表数据写入。 | 不显示、不生成 | S1/S2 |
| `--default-date` | 待官方参数映射确认 | MySQL DATE/DATETIME 解析失败替代值；旁路适用性未确认。 | 验证前不开放 | S2 |
| `--delete-from-table` | 待官方参数映射确认 | 普通导入前 DELETE 目标表数据；旁路专题未确认适用性。 | 验证前不开放 | S2 |
| `--direct` | 旁路已确认 | 开启 Direct Load；根据目标表是否为空自动采用全量或增量旁路模式。 | 必选、系统固定生成 | S1/S2/H |
| `--empty-string` | 待官方参数映射确认 | 指定值按空字符串处理；旁路 CSV 适用性未单独说明。 | CSV 映射保留，验证前标记 | S2 |
| `--enable-hidden-pk` | 明确不适用 | 当前官方资料确认的是 OBDUMPER 导出侧隐藏主键语义。 | 不显示、不生成 | H |
| `--escape-character` | 待官方参数映射确认 | CSV/CUT 转义字符；旁路适用性未单独说明。 | 格式映射保留，验证前标记 | S2 |
| `--exclude-column-names` | 待官方参数映射确认 | 排除指定列；与控制文件有约束；旁路列映射适用性未确认。 | 验证前不开放 | S2 |
| `--exclude-data-types` | 待官方参数映射确认 | 跳过指定数据类型；不能绕过旁路不支持结构限制。 | 验证前不开放 | S2 |
| `--exclude-table` | 明确不适用 | 排除多表范围中的表；旁路只允许一个明确目标表。 | 不显示、不生成 | S1/S2 |
| `--external-data` | 旁路已确认 | 第三方文件模式；官方旁路 CSV 示例使用，精确 MANIFEST 行为待实测。 | 高级配置 | S3/H |
| `--file-encoding` | 待官方参数映射确认 | 输入文件编码，区别于数据库字符集；Direct Load 适用性未逐项说明。 | 文件高级映射保留 | S2 |
| `--file-path` | 旁路已确认 | 输入文件/目录路径；官方旁路命令必备。 | 普通模式，必填 | S2/S3 |
| `--file-regular-expression` | 待官方参数映射确认 | 用正则筛选输入文件；同表分片场景合理，但旁路专题未说明。 | 高级映射保留，验证前标记 | S2 |
| `--file-suffix` | 待官方参数映射确认 | 自定义输入文件后缀；旁路专题未逐项说明。 | CSV 文件匹配映射保留 | S2 |
| `--function` | 明确不适用 | 导入函数定义。 | 不显示、不生成 | S2 |
| `--help` | CLI 元参数 | 显示命令行帮助。 | 不进入任务配置 | S2/H |
| `--host` | 旁路已确认 | ODP 或 OBServer 地址；旁路需同时建立 SQL/RPC 目标。 | 数据源快照 | S1/S2/S3 |
| `--ignore-escape` | 待官方参数映射确认 | CUT 忽略转义；CUT 旁路适用性未确认。 | 格式未确认前不开放 | S2 |
| `--ignore-unhex` | 明确不适用 | 普通导入可忽略二进制字符串解码，但旁路当前存在 BIT/二进制类型限制，不能用它绕过。 | 不显示、不生成 | S1/S2 |
| `--include-column-names` | 待官方参数映射确认 | 指定目标列顺序；旁路列映射适用性未确认。 | 验证前不开放 | S2 |
| `--line-separator` | 待官方参数映射确认 | CSV/CUT/POS/SQL 行分隔符；旁路适用性未逐项说明。 | CSV 映射保留，验证前标记 | S2 |
| `--log-path` | 待官方参数映射确认 | OBLOADER 运行日志目录；通用运行能力，旁路专题未单列。 | 执行节点/高级配置，待验证 | S2 |
| `--logical-database` | 明确不适用 | ODP Sharding 逻辑库标识；旁路专题只确认 ODP 连接，不确认逻辑库。 | 不显示、不生成 | S1/S2 |
| `--max-discards` | 明确不适用 | 普通导入单表重复数据上限；官方明确不适用于旁路。 | 不显示、不生成 | S2 |
| `--max-errors` | 明确不适用 | 普通导入单表写入错误上限；官方明确不适用于旁路。 | 不显示、不生成 | S2 |
| `--max-tps` | 待官方参数映射确认 | 普通导入限速；Direct Load 是否生效未说明。 | 验证前不开放 | S2 |
| `--max-wait-timeout` | 待官方参数映射确认 | 普通导入等待服务端合并时长；Direct Load 是否生效未说明。 | 验证前不开放 | S2 |
| `--mem` | 待官方参数映射确认 | JVM 内存大小，属于客户端进程资源；旁路精确影响未说明。 | 节点高级映射保留 | H |
| `--mix` | 明确不适用 | DDL/DML 混合文本解析；不符合单表数据 Direct Load 边界。 | 不显示、不生成 | S1/S2 |
| `--nls-date-format` | 待官方参数映射确认 | Oracle 会话日期格式；旁路数据解析适用性未确认。 | 验证前不开放 | S2 |
| `--nls-timestamp-format` | 待官方参数映射确认 | Oracle 会话时间戳格式；旁路适用性未确认。 | 验证前不开放 | S2 |
| `--nls-timestamp-tz-format` | 待官方参数映射确认 | Oracle 带时区时间戳格式；旁路适用性未确认。 | 验证前不开放 | S2 |
| `--no-sys` | 明确不适用 | 专题页注明只用于 OceanBase V4.0.0 之前，而旁路支持版本均更高。 | 当前版本不生成 | S1 |
| `--null-string` | 待官方参数映射确认 | 指定值按 NULL 处理；旁路 CSV 适用性未逐项说明。 | CSV 映射保留，验证前标记 | S2 |
| `--obj-user` | 明确不适用 | 导入用户定义。 | 不显示、不生成 | S2 |
| `--orc` | 旁路已确认 | 导入 ORC；OceanBase Cloud 官方页明确可用于 Direct Load。 | 云 ODP 开放；私有路径待官方参数映射确认 | S2/S6 |
| `--package` | 明确不适用 | 导入程序包定义。 | 不显示、不生成 | S2 |
| `--package-body` | 明确不适用 | 导入程序包体定义。 | 不显示、不生成 | S2 |
| `--par` | 旁路已确认 | 导入 Parquet；OceanBase Cloud 官方页明确可用于 Direct Load。 | 云 ODP 开放；私有路径待官方参数映射确认 | S2/S6 |
| `--parallel` | 旁路已确认 | OBServer 写入与排序工作线程数，默认 1，建议结合租户 CPU 并与 thread 一致。 | 高级配置 | S1/S2/H |
| `--password` | 旁路已确认 | 数据库密码。 | 数据源快照，脱敏 | S2/S3 |
| `--pause` | 待官方参数映射确认 | 普通导入停导内存水位阈值；Direct Load 是否生效未说明。 | 验证前不开放 | S2 |
| `--port` | 旁路已确认 | SQL 端口，不等于 RPC 端口。 | 数据源快照 | S1/S2/S3 |
| `--pos` | 待官方参数映射确认 | 导入定长 POS，需要控制文件；旁路专题无格式矩阵。 | 格式暂不开放 | S2 |
| `--procedure` | 明确不适用 | 导入存储过程定义。 | 不显示、不生成 | S2 |
| `--public-cloud` | 旁路已确认 | 云数据库 OceanBase + ODP 场景必需标识。 | 数据源场景派生 | S1/S2 |
| `--public-synonym` | 明确不适用 | 仅在 help Usage 出现且属于定义对象，官网无旁路语义。 | 不显示、不生成 | H |
| `--replace-data` | 旁路已确认 | 全量模式可用；增量模式仅无索引或 LOB 时可用；不能解决唯一索引冲突。 | 专家配置，条件显示 | S1/S2 |
| `--replace-object` | 明确不适用 | 替换 DDL/MIX 中已有对象定义。 | 不显示、不生成 | S2 |
| `--retry` | 明确不适用 | 从 load.ckpt 继续普通导入；官方明确旁路不支持重试/断点续传。 | 不显示、不生成 | S1/S2 |
| `--role` | 明确不适用 | 导入角色定义。 | 不显示、不生成 | S2 |
| `--rpc-port` | 旁路已确认 | Direct Load RPC 端口，云 ODP 常见 3307、私有 ODP 2885、OBServer 2882；必须实际校验。 | 普通模式，必填 | S1/S2/S3/H |
| `--rw` | 待官方参数映射确认 | 普通导入文件解析线程比例；Direct Load 是否共用未说明。 | 验证前不开放 | S2 |
| `--sequence` | 明确不适用 | 导入序列定义。 | 不显示、不生成 | S2 |
| `--server` | 明确不适用 | help-only 的 remote load/dump，官网无部署语义。 | 不显示、不生成 | H |
| `--session-config` | 旁路已确认 | 指定连接配置文件；该文件同时承载 direct_path_load 节点配置。 | 执行节点派生/专家摘要 | S2/C |
| `--skip-footer` | 待官方参数映射确认 | CUT 跳过末行；CUT 旁路适用性未确认。 | 格式未确认前不开放 | S2 |
| `--skip-header` | 待官方参数映射确认 | CSV/CUT 跳过首行；旁路 CSV 适用性未逐项说明。 | CSV 映射保留，验证前标记 | S2 |
| `--slow` | 待官方参数映射确认 | 普通导入慢导内存水位阈值；Direct Load 是否生效未说明。 | 验证前不开放 | S2 |
| `--source-type` | 待官方参数映射确认 | Hive 路径列解析，支持 ORC/Parquet；旁路适用性未确认。 | 验证前不开放 | S2 |
| `--sql` | 旁路已确认 | 导入 INSERT SQL 数据文件；OceanBase Cloud 官方页明确可用于 Direct Load。 | 云 ODP 开放；私有路径待官方参数映射确认 | S2/S6 |
| `--storage-uri` | 废弃/兼容 | 旧远程存储参数，V4.3.0 起整合到 file-path。 | 不显示，历史识别 | S4/H |
| `--strict` | 待官方参数映射确认 | 脏数据是否影响普通导入退出码；旁路错误语义未说明。 | 验证前不开放 | S2 |
| `--synonym` | 明确不适用 | 导入同义词定义。 | 不显示、不生成 | S2 |
| `--sys-password` | 明确不适用 | 只用于 OceanBase V4.0.0 之前，与旁路支持版本无交集。 | 当前版本不生成 | S1 |
| `--sys-user` | 明确不适用 | 只用于 OceanBase V4.0.0 之前，与旁路支持版本无交集。 | 当前版本不生成 | S1 |
| `--table` | 旁路已确认 | 唯一目标表；旁路不允许多表或星号。 | 普通模式，单值必填 | S1/S2/S3 |
| `--table-group` | 明确不适用 | 导入表组定义。 | 不显示、不生成 | S2 |
| `--tenant` | 旁路已确认 | 云 ODP、私有 ODP 场景按专题页要求派生；OBServer 直连不使用独立 tenant。 | 数据源场景派生 | S1 |
| `--thread` | 旁路已确认 | 客户端到服务端连接池规模；官方建议与 parallel 一致。 | 高级配置 | S1/S2/S3 |
| `--tmp-path` | 待官方参数映射确认 | 本地缓冲数据与日志目录；远程存储 Direct Load 行为未确认。 | 验证前不开放 | H |
| `--trail-delimiter` | 待官方参数映射确认 | CUT 行尾分隔符；CUT 旁路适用性未确认。 | 格式未确认前不开放 | S2 |
| `--trigger` | 明确不适用 | 导入触发器定义。 | 不显示、不生成 | S2 |
| `--truncate-table` | 旁路已确认 | 导入前 Truncate 目标表；官方旁路命令示例使用，属于高风险行为。 | 专家配置，二次确认 | S2/S3 |
| `--type` | 明确不适用 | 导入类型定义。 | 不显示、不生成 | S2 |
| `--type-body` | 明确不适用 | 导入类型体定义。 | 不显示、不生成 | S2 |
| `--user` | 旁路已确认 | 数据库用户名；Oracle 兼容模式用户名需大写，连接场景决定 tenant/cluster 表达。 | 数据源快照 | S1/S2 |
| `--version` | CLI 元参数 | 显示工具版本。 | 节点预检查使用，不进入任务配置 | S2/H |
| `--view` | 明确不适用 | 导入视图定义。 | 不显示、不生成 | S2 |
| `--with-trim` | 待官方参数映射确认 | 删除输入数据左右空格；旁路 CSV 适用性未逐项说明。 | CSV 映射保留，验证前标记 | S2 |
| `--yes` | 旁路已确认 | 跳过 truncate/delete 的 CLI 交互确认；旁路已确认 truncate 时由平台风险确认后派生。 | 不作独立开关，按需派生 | S2/S3 |

## 5. Direct Load 节点配置

| 配置键 | 官网专题示例值 | 发布包实际值 | 产品处置 | 状态 |
|---|---:|---:|---|---|
| `direct_path_load.task_timeout_ms` | `25920000` | `2147483647` | 节点只读配置，展示实际生效值 | 来源冲突，待验证 |
| `direct_path_load.heartbeat_timeout_ms` | `60000000` | `60000` | 节点只读配置，展示实际生效值 | 来源冲突，待验证 |
| `direct_path_load.heartbeat_interval_ms` | `30000000` | `10000` | 节点只读配置，展示实际生效值 | 来源冲突，待验证 |

这些配置不是单任务命令参数。任务快照记录执行节点、配置文件指纹、实际值和读取时间；V1.0 不在任务向导中编辑。

## 6. 旁路命令最小结构

当前证据足以支持的最小结构为：

```text
obloader
  <连接地址、SQL 端口、用户和密码>
  --rpc-port <RPC端口>
  --database <目标数据库>
  --table <唯一目标表>
  --csv
  --file-path <输入文件或目录>
  --direct
  [--thread <客户端连接池>]
  [--parallel <服务端并行度>]
```

场景派生参数、高风险参数和 CSV 解析参数只有在字段活动、约束通过且风险确认有效时进入最终命令。

## 7. 覆盖验收

- 参数名称集合必须与实际 `obloader --help` 的 103 个唯一长参数完全相等。
- 每个参数必须有旁路已确认、明确不适用、待官方参数映射确认、废弃/兼容或 CLI 元参数状态。
- 三个 Direct Load 专属参数必须与连接、文件、单表、格式和风险参数一起形成完整命令，不能做成孤立“高级开关”。
- 普通导入的有效参数不得未经证据自动复制为旁路有效参数。
- 发布包或官方资料变化时，先更新参数事实和节点配置事实，再更新字段和产品建议。
