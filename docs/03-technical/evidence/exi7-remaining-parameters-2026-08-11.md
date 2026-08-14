# EX-I7 剩余参数第一批受控实测与定版（2026-08-11）

> 验证状态：`--compact-schema`/`--snapshot`/`--where` 定版 ENABLED；`--weak-read` 已验证工具行为但缺少副本/权限预检查，`--retry` 仅验证无保存点失败语义，二者保持 VALIDATION_GATED
> 授权范围：已授权测试数据源与只读测试 Schema；真实身份、端点和对象名均已脱敏；输出位于 Git 忽略的受控临时目录
> 关联：EX-I7、EX-F028/EX-F031 相关组、EX-F052 系列（时间戳格式仍待第二批）

## 1. 授权与安全边界

- 用户当轮明确授权：使用已授权测试数据源对只读测试 Schema 执行导出实测（5 项组合 + 2 项补测）；文档只保留合成别名。
- 密码从本地 SQLite 加密信封解密（控制面同款根密钥链：`var/local-mvp-root-key.json` DPAPI 载体 + AES-GCM），只经内存与 OBDUMPER 官方 `security.properties`（RSA 材料）短时传递，不进入 argv、日志、标准输出或文档；执行后清理材料并清零内存字节。
- 输出目录与工具日志位于 `<git-ignored-output-dir>`，保留供本机复核；不提交任何输出、日志或临时材料。

## 2. 实测环境与命令基线

- 工具：OBDUMPER 4.3.5（`<verified-obdumper-4.3.5-dir>`），主类 `com.oceanbase.tools.loaddump.cmd.Obdumper`，Java 8（1.8.0_491）。
- 连接：`<synthetic-user>@<synthetic-tenant>#<synthetic-cluster> @ <private-endpoint>:<port>`，Schema `<synthetic-schema>`。
- 命令基线（模拟平台启动参数）：`-Dsecurity.configurationFile`（安全材料）+ `-Dtool.base.dir` + `-Dsession.configurationFile` + `-Ddecrypt.configurationFile` + `-Dlog4j.output` + `-Dhadoop.home.dir` + `-Dfs.defaultFS=file:///` + `-classpath <tool>/lib/*`。
- **平台差异实测发现**：OBDUMPER 4.3.5（Hadoop 3.3.6）在 Windows 上把盘符绝对路径（`<drive>:/<absolute-output-path>`）解析为错误的本地文件 URI（Wrong FS）；`-f`/`--log-path` 必须使用相对路径（POS 受控实测同口径）。控制面/Agent 正式任务路径传递需核对此约束（平台适配层）。

## 3. 实测结论

| 组合 | 结果 | 证据 |
|---|---|---|
| `--ddl --all`（基线） | code=0 | 8 个 `-schema.sql`（TABLE/PROCEDURE 等） |
| `--ddl --all --compact-schema` | code=0 | 8 个 DDL 文件与基线**逐字节一致**（当前库对象无差异） |
| `--csv --all --snapshot` | code=0 | 全量导出完成（frozen_version 提示见于失败轮次日志） |
| `--csv --all --weak-read` | code=0 | 全量导出完成 |
| `--csv --all`（prep）→ 中断 | 中断 | 数据量太小（全量 7 秒完成）无法在导出中制造保存点 |
| `--csv --table <synthetic-table> --retry`（无保存点） | code=1 | 失败关闭：`File: "<out>\.dump.ckpt" is missing` |
| `--csv --table <synthetic-table> --where "id < 100"` | code=0 | 导出 109 数据行（全表 1,000 行） |

## 4. 参数定版

| 参数 | 官方语义（4.3.5 --help/文档） | 实测结论 | v1 状态 |
|---|---|---|---|
| `--compact-schema` | `Export the schema text which are retrieved by executing 'show create table'` | 运行成功；当前库对象（无列注释/索引的对象）与完整 DDL 无差异 | **ENABLED** |
| `--snapshot` | 导出最近一次合并版本的快照数据（官方 V2.2.2 参数表） | 运行成功，正常导出 | **ENABLED** |
| `--weak-read` | `Interpret to dump from follower server`（从备库读） | 工具运行成功；产品仍缺副本与权限预检查 | **VALIDATION_GATED** |
| `--retry` | `是否从最近一次的保存点继续导出`（官方 V2.2.2 参数表） | 无保存点失败关闭；有效保存点续跑、原快照绑定仍待 EX-I8 | **VALIDATION_GATED** |
| `--where` | 支持指定全局条件导出符合条件的数据（如 `--where 'age>16 and age<65'`） | 行数筛选生效（109 vs 1,000） | **ENABLED** |

## 5. 残余与未覆盖项

- `--compact-schema` 的差异效果需要包含注释、索引、分区策略复杂对象的目标库才能证伪（当前测试对象为紧凑形态）；语义已由官方 help 确认，接入后不影响无差异场景。
- `--retry` 的"从保存点续跑"行为：测试数据量较小（全量约 1.5 万行、数秒完成），无法稳定制造保存点；该行为并入 EX-I8 `dump.ckpt` 继续功能，在受控的大数据或慢导出环境验证。
- 第二批待实测：时间戳值格式（--date-value-format 等 10 个，使用含日期/时间列的合成表）、`--partition`（使用合成分区表）、`--exclude-data-types`/`--enable-hidden-pk`（需对应合成表结构）、对象类型 13 个（需核对测试库对象存在性）。
- `--add-extra-message`：本次验证时映射文档曾标 ENABLED 但 v6 元数据未接入；后续第二批仅确认工具接受，DDL 行为、当前 sys 权限预检查与任务秘密槽位绑定仍未完成，现行状态保持 VALIDATION_GATED（见 2026-08-13 第二批证据）。
