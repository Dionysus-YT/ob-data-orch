# Export 增量验证记录

本页按主题合并原始记录，日期、授权范围、验证结果与未覆盖项沿用各记录。历史通过不代表当前重新验证，历史授权不自动扩展到新操作；当前阶段与发布门禁见 [任务地图](../development-task-map.md)。

- [EX-I7 剩余参数第一批受控实测与定版（2026-08-11）](#exi7-batch1)
- [EX-I7 剩余参数第二批受控实测与定版（2026-08-13）](#exi7-batch2)
- [EX-I6 存储专用预检查框架与门禁打通（2026-08-14）](#exi6-storage)
- [EX-I8 结果、失败恢复与复用（2026-08-14）](#exi8-recovery)

<a id="exi7-batch1"></a>

## EX-I7 剩余参数第一批受控实测与定版（2026-08-11）

> 验证状态：`--compact-schema`/`--snapshot`/`--where` 定版 ENABLED；`--weak-read` 已验证工具行为但缺少副本/权限预检查，`--retry` 仅验证无保存点失败语义，二者保持 VALIDATION_GATED
> 授权范围：已授权测试数据源与只读测试 Schema；真实身份、端点和对象名均已脱敏；输出位于 Git 忽略的受控临时目录
> 关联：EX-I7、EX-F028/EX-F031 相关组、EX-F052 系列（时间戳格式仍待第二批）

<a id="exi7-batch1-1-授权与安全边界"></a>

### 1. 授权与安全边界

- 用户当轮明确授权：使用已授权测试数据源对只读测试 Schema 执行导出实测（5 项组合 + 2 项补测）；文档只保留合成别名。
- 密码从本地 SQLite 加密信封解密（控制面同款根密钥链：`var/local-mvp-root-key.json` DPAPI 载体 + AES-GCM），只经内存与 OBDUMPER 官方 `security.properties`（RSA 材料）短时传递，不进入 argv、日志、标准输出或文档；执行后清理材料并清零内存字节。
- 输出目录与工具日志位于 `<git-ignored-output-dir>`，保留供本机复核；不提交任何输出、日志或临时材料。

<a id="exi7-batch1-2-实测环境与命令基线"></a>

### 2. 实测环境与命令基线

- 工具：OBDUMPER 4.3.5（`<verified-obdumper-4.3.5-dir>`），主类 `com.oceanbase.tools.loaddump.cmd.Obdumper`，Java 8（1.8.0_491）。
- 连接：`<synthetic-user>@<synthetic-tenant>#<synthetic-cluster> @ <private-endpoint>:<port>`，Schema `<synthetic-schema>`。
- 命令基线（模拟平台启动参数）：`-Dsecurity.configurationFile`（安全材料）+ `-Dtool.base.dir` + `-Dsession.configurationFile` + `-Ddecrypt.configurationFile` + `-Dlog4j.output` + `-Dhadoop.home.dir` + `-Dfs.defaultFS=file:///` + `-classpath <tool>/lib/*`。
- **平台差异实测发现**：OBDUMPER 4.3.5（Hadoop 3.3.6）在 Windows 上把盘符绝对路径（`<drive>:/<absolute-output-path>`）解析为错误的本地文件 URI（Wrong FS）；`-f`/`--log-path` 必须使用相对路径（POS 受控实测同口径）。控制面/Agent 正式任务路径传递需核对此约束（平台适配层）。

<a id="exi7-batch1-3-实测结论"></a>

### 3. 实测结论

| 组合 | 结果 | 证据 |
|---|---|---|
| `--ddl --all`（基线） | code=0 | 8 个 `-schema.sql`（TABLE/PROCEDURE 等） |
| `--ddl --all --compact-schema` | code=0 | 8 个 DDL 文件与基线**逐字节一致**（当前库对象无差异） |
| `--csv --all --snapshot` | code=0 | 全量导出完成（frozen_version 提示见于失败轮次日志） |
| `--csv --all --weak-read` | code=0 | 全量导出完成 |
| `--csv --all`（prep）→ 中断 | 中断 | 数据量太小（全量 7 秒完成）无法在导出中制造保存点 |
| `--csv --table <synthetic-table> --retry`（无保存点） | code=1 | 失败关闭：`File: "<out>\.dump.ckpt" is missing` |
| `--csv --table <synthetic-table> --where "id < 100"` | code=0 | 导出 109 数据行（全表 1,000 行） |

<a id="exi7-batch1-4-参数定版"></a>

### 4. 参数定版

| 参数 | 官方语义（4.3.5 --help/文档） | 实测结论 | v1 状态 |
|---|---|---|---|
| `--compact-schema` | `Export the schema text which are retrieved by executing 'show create table'` | 运行成功；当前库对象（无列注释/索引的对象）与完整 DDL 无差异 | **ENABLED** |
| `--snapshot` | 导出最近一次合并版本的快照数据（官方 V2.2.2 参数表） | 运行成功，正常导出 | **ENABLED** |
| `--weak-read` | `Interpret to dump from follower server`（从备库读） | 工具运行成功；产品仍缺副本与权限预检查 | **VALIDATION_GATED** |
| `--retry` | `是否从最近一次的保存点继续导出`（官方 V2.2.2 参数表） | 无保存点失败关闭；有效保存点续跑、原快照绑定仍待 EX-I8 | **VALIDATION_GATED** |
| `--where` | 支持指定全局条件导出符合条件的数据（如 `--where 'age>16 and age<65'`） | 行数筛选生效（109 vs 1,000） | **ENABLED** |

<a id="exi7-batch1-5-残余与未覆盖项"></a>

### 5. 残余与未覆盖项

- `--compact-schema` 的差异效果需要包含注释、索引、分区策略复杂对象的目标库才能证伪（当前测试对象为紧凑形态）；语义已由官方 help 确认，接入后不影响无差异场景。
- `--retry` 的"从保存点续跑"行为：测试数据量较小（全量约 1.5 万行、数秒完成），无法稳定制造保存点；该行为并入 EX-I8 `dump.ckpt` 继续功能，在受控的大数据或慢导出环境验证。
- 第二批待实测：时间戳值格式（--date-value-format 等 10 个，使用含日期/时间列的合成表）、`--partition`（使用合成分区表）、`--exclude-data-types`/`--enable-hidden-pk`（需对应合成表结构）、对象类型 13 个（需核对测试库对象存在性）。
- `--add-extra-message`：本次验证时映射文档曾标 ENABLED 但 v6 元数据未接入；后续第二批仅确认工具接受，DDL 行为、当前 sys 权限预检查与任务秘密槽位绑定仍未完成，现行状态保持 VALIDATION_GATED（见 2026-08-13 第二批证据）。

<a id="exi7-batch2"></a>

## EX-I7 剩余参数第二批受控实测与定版（2026-08-13）

> 验证状态：MySQL DATE/DATETIME 两个格式参数已观察到输出效果；其余时间格式只确认工具接受，缺少对应类型列或 Oracle 行为证据；`--partition`/`--exclude-data-types` 定版 ENABLED；隐藏主键与附加对象信息仍缺产品前置校验
> 授权范围：`test` 数据源、`ob_test` 只读测试 Schema（`t_order_test` 系列 + `t_hash_prune_test` 合成分区表）；真实身份、端点和对象名均已脱敏；输出位于 Git 忽略的受控临时目录 `tmp/exi7-run-02/`
> 关联：EX-I7 第二批、EX-F055~EX-F064（时间格式）、EX-F066/EX-F072/EX-F075/EX-F077

<a id="exi7-batch2-1-授权与安全边界"></a>

### 1. 授权与安全边界

- 用户当轮明确授权：使用已授权 `test` 数据源对只读测试 Schema 执行第二批导出实测（时间戳格式/分区/类型排除/对象类型/add-extra-message 共 11 个组合 + 1 项基线 + 1 项补测）；文档只保留合成别名。
- 密码从本地 SQLite 加密信封解密（控制面同款根密钥链：`var/local-mvp-root-key.json` DPAPI 载体 + AES-GCM，AAD 由引用信息动态构造、与 `internal/credential.aad` 同构；数据库密码与 sys 密码使用独立 credential_id），只经内存与 OBDUMPER 官方 `security.properties`（RSA 材料）短时传递，不进入 argv、日志、标准输出或文档；执行后删除材料目录并清零内存字节。
- 实测辅助程序（`tmp/exi7-run-02/tool/main.go`）与执行脚本位于 Git 忽略目录，不提交源码；输出目录保留供本机复核，不提交任何输出、日志或临时材料。

<a id="exi7-batch2-2-实测环境与命令基线"></a>

### 2. 实测环境与命令基线

- 工具：OBDUMPER 4.3.5，主类 `com.oceanbase.tools.loaddump.cmd.Obdumper`，Java 8（1.8.0_491）。
- 连接：`<synthetic-user>@<synthetic-tenant>#<synthetic-cluster> @ <private-endpoint>:<port>`，Schema `<synthetic-schema>`（`ob_test`）。
- 命令基线（同第一批）：`-Dsecurity.configurationFile` + `-Dtool.base.dir` + `-Dsession.configurationFile=conf/session.config.json` + `-Ddecrypt.configurationFile` + `-Dlog4j.output` + `-Dhadoop.home.dir` + `-Dfs.defaultFS=file:///` + `-classpath <tool>/lib/*`；`--file-path`/`--log-path` 使用相对路径（第一批 Wrong FS 结论沿用）。
- 测试对象：`t_order_test`（11 列，含 `datetime` 列 created_at/updated_at）；`t_hash_prune_test`（4 列，含 `date` 列 trade_date，HASH 分区表 `partition by hash(user_id)`，8 个分区 p0~p7）。

<a id="exi7-batch2-3-实测结论"></a>

### 3. 实测结论

| 组合 | 结果 | 证据 |
|---|---|---|
| `--csv --table t_hash_prune_test`（全表基线） | code=0 | 4 条数据行 |
| `--csv --table t_order_test --datetime-value-format=yyyy/MM/dd HH:mm:ss` | code=0 | created_at 输出 `2026/08/06 16:36:25`（默认 `2026-08-06 16:36:25`，格式生效） |
| `--csv --table t_hash_prune_test --date-value-format=yyyy/MM/dd` | code=0 | trade_date 输出 `2024/02/15`（格式生效） |
| `--csv --table t_order_test --timestamp-value-format --time-value-format --nls-date-format --nls-timestamp-format --nls-timestamp-tz-format`（组合） | code=0 | 参数全部接受；表中无 timestamp/time/year 列，不影响 datetime 列输出 |
| `--csv --table t_order_test --year-value-format=yyyy` | code=1 | **`Unknown option: '--year-value-format'`**（picocli 参数表不存在该选项） |
| `--csv --table t_hash_prune_test --partition=p0` | code=0 | 1 条数据行（全表 4 行，分区过滤生效） |
| `--csv --table t_hash_prune_test --partition=p0,p2` | code=0 | 2 条数据行（多分区生效） |
| `--csv --table t_order_test --exclude-data-types=decimal` | code=0 | 表头**不含 order_amount 列**（decimal 列被排除，生效） |
| `--csv --table t_order_test --enable-hidden-pk` | code=0 | 参数接受；MySQL 模式无隐藏主键，输出与默认一致 |
| `--ddl --procedure=p_generate_order_test_data` | code=0 | PROCEDURE DDL 正常导出 |
| `--ddl --obj-user=test` | code=0 | USER DDL 正常导出 |
| `--ddl --view=nonexistent_view_x`（不存在的对象） | code=0 | **空输出、无 warn/error 日志**（工具对不存在的指定对象静默成功） |
| `--csv --table t_order_test --add-extra-message` | code=0 | 参数接受；CSV 输出与默认无可见差异；本次未触发 sys 凭据路径 |

<a id="exi7-batch2-4-参数定版"></a>

### 4. 参数定版

| 参数 | 官方语义（4.3.5 --help） | 实测结论 | v1 状态 |
|---|---|---|---|
| `--date-value-format` | `dump date type record(oboracle) by specified date format` | 格式生效（`yyyy/MM/dd`） | **ENABLED** |
| `--datetime-value-format` | `dump datetime type record by specified datetime format` | 格式生效（`yyyy/MM/dd HH:mm:ss`） | **ENABLED** |
| `--timestamp-value-format` | 指定时间戳格式导出 | 接受；无 timestamp 列环境未观察格式效果 | **VALIDATION_GATED**（工具接受不等于行为已验证） |
| `--timestamp-tz-value-format` | 带时区时间戳格式 | 接受（组合内） | **VALIDATION_GATED**（缺少 Oracle 对应类型列证据） |
| `--timestamp-ltz-value-format` | 本地时区时间戳格式 | 接受（组合内） | **VALIDATION_GATED**（缺少 Oracle 对应类型列证据） |
| `--time-value-format` | 时间格式 | 接受（组合内） | **VALIDATION_GATED**（缺少 TIME 列行为证据） |
| `--year-value-format` | —（官网文档与 4.3.5 help 均未列出，2026-08-13 核实） | **4.3.5 二进制不支持**（`Unknown option`） | **BLOCKED**（参数不存在，产品不接入） |
| `--nls-date-format` | Oracle NLS 日期格式 | 接受（组合内） | **VALIDATION_GATED**（缺少 Oracle 行为证据） |
| `--nls-timestamp-format` | Oracle NLS 时间戳格式 | 接受（组合内） | **VALIDATION_GATED**（同上） |
| `--nls-timestamp-tz-format` | Oracle NLS 时区时间戳格式 | 接受（组合内） | **VALIDATION_GATED**（同上） |
| `--partition` | `partition[,partition...]` 指定分区导出 | 单/多分区均生效（行数 1/2 vs 全表 4） | **ENABLED** |
| `--exclude-data-types` | `dataType[,dataType...]` 排除数据类型 | decimal 列被排除（表头验证） | **ENABLED** |
| `--enable-hidden-pk` | `Interpret whether to use the hidden primary key` | 接受；MySQL 无隐藏主键场景无差异 | **VALIDATION_GATED**（表结构、版本与权限预检查未完成） |
| `--add-extra-message` | boolean | 接受且不破坏导出；CSV 无可见附加内容 | **VALIDATION_GATED**（DDL 行为、当前 sys 权限预检查与秘密槽位绑定未完成） |

<a id="exi7-batch2-5-对象类型存在性核对与实测"></a>

### 5. 对象类型存在性核对与实测

测试库 `ob_test` 当前存在的对象类型（基线 `--ddl --all` 清单）：`TABLE`（6 张，含合成分区表）、`PROCEDURE`（1 个）、`USER`（1 个）。

| 对象类型参数 | 库中存在 | 实测 |
|---|---|---|
| `--table` / `--procedure` / `--obj-user` | 是 | 已实测（指定对象 DDL 导出成功） |
| `--view` / `--sequence` / `--synonym` / `--public-synonym` / `--trigger` / `--type` / `--type-body` / `--package` / `--package-body` / `--function` / `--role` / `--table-group` | **否**（只读测试 Schema 中不存在） | 未实测；参数存在性已由 `--help` 确认，行为待授权对象后验证 |
| 不存在的指定对象 | — | code=0 空输出（工具静默成功，不失败关闭） |

<a id="exi7-batch2-6-残余与未覆盖项"></a>

### 6. 残余与未覆盖项

- `--year-value-format` 核对结论（2026-08-13 官网核实）：**三处一致确认该参数不存在**——① OceanBase 官网命令行选项文档（社区版最新 + V4.3.1.1）时间戳格式系列只有 `--date-value-format`/`--time-value-format`/`--datetime-value-format`/`--timestamp-value-format`/`--timestamp-tz-value-format`/`--timestamp-ltz-value-format` 六项；② 4.3.5 二进制 `--help` 无该选项（实测 `Unknown option`）；③ 项目映射文档与字段规则无该条目。无需新增 BLOCKED 条目，后续接入不得引入该参数名。
- `--timestamp-value-format`/`--time-value-format` 等无对应类型列的格式效果未观察（库中无 timestamp/time/year 列）；官方 help 只能证明参数语义，不能替代行为验收，因此继续保持门禁。
- Oracle 模式参数（`--nls-*`、`--enable-hidden-pk` 的隐藏主键行为、`--timestamp-tz/ltz-value-format`）需要 Oracle 兼容模式库验证。
- `--add-extra-message` 的"额外信息"内容未在 MySQL CSV 输出中观察到；官方语义、适用格式、当前 sys 权限预检查和任务秘密槽位绑定完成前，产品不得透传。
- 13 种对象类型中 9 种测试库不存在，无法实测；不存在的指定对象"静默成功"行为已记录，产品预检查需自行核对对象存在性（不能依赖工具失败信号）。
- 第二批产品接入只开放 `--date-value-format`、`--datetime-value-format`、`--partition`、`--exclude-data-types`；其余已登记参数按上述残余继续失败关闭。

<a id="exi6-storage"></a>

## EX-I6 存储专用预检查框架与门禁打通（2026-08-14）

> 验证状态：合成验证通过（Go/前端全部门禁）。真实对象存储网络连接、真实凭据解析与四类存储取证未执行，归 EX-V1 排期。
> 安全说明：本文不记录真实端点、身份、密码、密钥、完整命令或工具输出；示例值均为合成数据。

<a id="exi6-storage-1-交付内容"></a>

### 1. 交付内容

存储专用预检查框架（EX-I6 第三段）与存储凭据槽位（第二段）的审查收口、OpenAPI/前端同步：

<a id="exi6-storage-11-检查清单与契约"></a>

#### 1.1 检查清单与契约

- `internal/precheckcontract`：新增 `STORAGE_CONNECTIVITY`、`STORAGE_AUTH` 两项检查与受控证据码（PASSED/FAILED/UNKNOWN 各一条专属码 + SYNTHETIC_OK）；冻结六项与 `FixedChecks()` 语义不变。
- `internal/agentpreflight`：对象存储输出的“存储形态清单”固定为 `DATABASE_CONNECTIVITY → OBJECT_ACCESS → TOOL_ENVIRONMENT → AVAILABLE_SPACE → STORAGE_CONNECTIVITY → STORAGE_AUTH`；本地输出保持冻结六项。`Run` 保持 local-first：本机前置失败时数据库与存储检查全部 UNKNOWN 且不解析槽位。`ValidateReportFor(kind, report)` 按输出类型拒绝形态混淆。

<a id="exi6-storage-12-控制面与仓储"></a>

#### 1.2 控制面与仓储

- 预检查上下文新增 `OutputKind` 与 `StorageTarget{Provider, URI, Endpoint, TmpPath}`：对象存储草稿从冻结 v6 配置解析（URI scheme 白名单、参数白名单 endpoint/region、拒绝密钥参数），URI 不经浏览器重写。
- `claim-next` 按输出类型下发检查清单与存储目标段；`complete` 在事务内按冻结草稿的输出类型复核结果形态（形态混淆 → 租约拒绝）。
- 提交门禁由结果驱动：对象存储输出要求两项存储检查均 PASSED，否则 `STORAGE_PRECHECK_REQUIRED`；`STORAGE_PRECHECK_UNAVAILABLE` 硬门禁已移除。

<a id="exi6-storage-13-agent-探测"></a>

#### 1.3 Agent 探测

- `internal/agentlocalpreflight`：`TCPStorageConnectivityProber` 只对受控 endpoint（host[:port]，默认 443）执行单次 TCP 建连，不携带凭据或业务数据；仅在显式运行开关 `OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true` 时装配，默认 UNKNOWN。
- `UnavailableStorageAuthProber`：凭据有效性探测的固定失败关闭实现；真实探测（四类云厂商签名协议）归 EX-V1，本切片绝不解析或发送真实凭据。
- AVAILABLE_SPACE 对对象存储输出转向 `--tmp-path` 所在卷；未指定时 UNKNOWN 失败关闭。

<a id="exi6-storage-14-存储凭据槽位审查收口第二段"></a>

#### 1.4 存储凭据槽位审查收口（第二段）

- 修复执行解密 AAD 信封标识：`ResolveExecutionStorageCredential` 返回两个加密信封自身的 credentialId，控制面解密以其重建 AAD（此前误用 storageCredentialId，AES-GCM 校验必然失败）。
- 修复创建幂等重放：同幂等键不同内容改为 `IDEMPOTENCY_CONFLICT`，不再静默返回旧资源。
- 三个写端点（create/rotate/delete）补齐 CSRF 失败关闭；rotate 入口校验幂等键格式（缺失不再落为仓储层 500）。
- Agent 侧短时存储凭据段在协议解析层校验 provider 白名单、长度与禁止字节；core-site.xml 生成拒绝 NUL。

<a id="exi6-storage-15-openapi-与前端"></a>

#### 1.5 OpenAPI 与前端

- OpenAPI：`/api/v1/storage-credentials` 三路径与 5 个 schema/4 个响应（密钥 writeOnly、provider 白名单、If-Match/幂等键声明、no-store）；`AgentPrecheckCheckSet/Results` 改为本地/存储双形态 oneOf，新增两项结果 schema 与上下文 `outputKind/storageTarget` 段。
- 前端：存储凭据管理页（列表/创建/轮换/删除，密钥只写不回显）、向导步骤 5 凭据绑定（provider 匹配、修订取 currentRevision、本地输出携带即失败关闭）、预检查展示接入两项存储检查（未授权探测显示“未完成（探测未授权）”）；对象存储草稿可发起预检查，提交按钮按结果禁用。

<a id="exi6-storage-2-验证记录"></a>

### 2. 验证记录

```powershell
go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...
./scripts/check-secrets.ps1
git diff --check
gofmt -l internal\ cmd\

Set-Location web
npm run lint
npm run typecheck
npm run test
npm run build
```

结果：Go 全量测试/vet、密钥扫描、gofmt、差异检查、Windows AMD64/Linux AMD64/Linux ARM64 无 CGO 构建与前端 lint/typecheck/18 文件 134 项测试/生产构建全部通过。完整 `scripts/verify.ps1` 因运行中 Vite 占用原生 DLL 会在 `npm ci` 停止，本次按等价分段门禁执行。

新增合成正负例覆盖：存储形态编排与前置失败 UNKNOWN、形态混淆拒绝、存储目标失败关闭、端点解析、连通性/凭据探测投影、tmp-path 卷空间、v6 存储上下文解析与负例、完成端点形态复核、控制面提交门禁正反例（存储检查 UNKNOWN → 422；PASSED → 201）、agentwire 存储租约解析与客户端宽松复核、OpenAPI 双形态与存储凭据面、前端草稿引用往返与凭据表单校验。

<a id="exi6-storage-3-边界与未完成"></a>

### 3. 边界与未完成

- 两项存储探测默认 UNKNOWN：真实网络连接与真实凭据解析受 AGENTS.md 授权约束，未执行；开启 TCP 连通性开关仍需当次授权（EX-V1）。
- STORAGE_AUTH 的真实探测协议（四类云厂商签名）未实现，只保留接口与失败关闭实现。
- 四类存储（OSS/S3/COS/OBS）真实导出取证、对象存储任务提交后的端到端执行未验证。
- 对象存储任务在两项存储检查通过前保持提交阻断；这不是把“未验证”伪装成“可用”。

<a id="exi8-recovery"></a>

## EX-I8 结果、失败恢复与复用（2026-08-14）

> 验证状态：合成验证通过（Go/前端全部门禁）。真实 `dump.ckpt` 续跑取证、结果清单行数/校验和解析与模板复用未完成，归 EX-V1/后续补充切片。
> 安全说明：本文不记录真实端点、身份、密码、密钥、完整命令或工具输出；示例值均为合成数据。

<a id="exi8-recovery-1-交付内容8-a8-b8-c-三段"></a>

### 1. 交付内容（8-A/8-B/8-C 三段）

<a id="exi8-recovery-8-a-结果与失败事实"></a>

#### 8-A 结果与失败事实

- Agent 成功与失败路径都上报受控结果事实：文件数/字节数/受限相对路径清单（≤100 项、≤512 字符、禁止绝对前缀/回退段/控制字符）+ `dump.ckpt` 存在性。失败路径在 `TOOL_TERMINAL_OBSERVED(FAILED)` 之后作为迟到事实上报，控制面按同一 Agent/租约/连续序号在接受失败终态已释放租约时接收。
- 控制面把 `RESULT_FACTS_OBSERVED` 合并为 `task_executions.result_summary_json`（`result/fileCount/totalBytes/files/checkpointPresent/observedAt`），任务详情 `/execution` 投影 `resultSummary` 安全摘要；前端新增“执行结果”区与失败操作区。结果清单不含内容、行数或校验和（未取证，不伪造）。

<a id="exi8-recovery-8-b-派生任务与两类派生草稿"></a>

#### 8-B 派生任务与两类派生草稿

- 迁移 0018：`tasks.derivation_kind`（REBUILD_FROM_CONFIG/RERUN_FROM_SCRATCH/CHECKPOINT_RESUME）+ `export_drafts.source_task_id/source_derivation`（`parent_task_id` 沿用 0014）。
- `POST /tasks/{id}:rebuild-draft`：从失败任务冻结快照重建可编辑 v6 草稿，不复制凭据明文/预检查/风险确认/日志；REBUILD_FROM_CONFIG 可改参，RERUN_FROM_SCRATCH 提交时服务端强制 configFingerprint 与来源任务一致（422 RERUN_CONFIGURATION_CHANGED）。
- 提交派生草稿时写入 `parent_task_id/derivation_kind`；任务概览投影来源关系（parentTaskId/derivationKind）。

<a id="exi8-recovery-8-c-检查点继续"></a>

#### 8-C 检查点继续

- `POST /tasks/{id}:resume-checkpoint`：资格 = 失败终态 + 结果摘要确认 dump.ckpt 存在 + 原预检查 SUCCEEDED/COMPLETE；新任务继承原快照并追加官方 `--retry`（服务端固定构造，不新增参数元数据版本），不重新预检查。
- 继续任务的领取执行复验数据源/凭据/节点/Agent 事实版本，豁免预检查 TTL（检查点内含继续语义）；不满足条件返回稳定门禁错误（CHECKPOINT_RESUME_UNAVAILABLE / CHECKPOINT_PRECHECK_UNAVAILABLE）。
- 前端失败任务操作区：基于原配置新建 / 从头重新执行 / 条件化从检查点继续；向导支持派生草稿加载与表单回填（REBUILD 可编辑，RERUN 直接进入预检查步骤）。

<a id="exi8-recovery-2-验证记录"></a>

### 2. 验证记录

```powershell
go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...
./scripts/check-secrets.ps1
git diff --check
gofmt -l cmd contracts internal migrations

Set-Location web
npm run lint
npm run typecheck
npm run test
npm run build
```

结果：Go 全量测试/vet（31 包）、密钥扫描、gofmt、差异检查与前端 lint/typecheck/18 文件 136 项测试/生产构建全部通过。

新增合成正负例覆盖：结果/检查点事件负载边界、失败终态后迟到事实接受、结果摘要合并与授权投影、派生草稿重建（正例/非失败/未知任务/快照版本）、从头执行指纹不可变、检查点继续（正例/无检查点/链式继续拒绝/幂等重放）、任务概览派生关系、OpenAPI 派生面与结果摘要安全投影、前端派生客户端与解析。

<a id="exi8-recovery-3-边界与未完成"></a>

### 3. 边界与未完成

- 真实 `dump.ckpt` 续跑取证（失败中断制造保存点后继续）归 EX-V1；当前为合成验证。
- 结果清单只含相对路径与字节数；行数、校验和与格式特征未解析。
- 获准范围内的模板复用（从成功任务保存模板/模板创建草稿）为下一补充切片。
- 派生操作仅对失败任务提供；成功任务的“保存为模板”随模板复用切片实现。
