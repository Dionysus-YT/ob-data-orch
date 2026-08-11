# EX-I7 剩余参数第一批受控实测与定版（2026-08-11）

> 验证状态：`--compact-schema`/`--snapshot`/`--weak-read`/`--where` 定版 ENABLED；`--retry` 失败关闭语义已实测，续跑行为并入 EX-I8 验证
> 授权范围：`test` 数据源、`ob_test` 库只读导出、输出路径 `tmp/exi7-run-01`（git 忽略）
> 关联：EX-I7、EX-F028/EX-F031 相关组、EX-F052 系列（时间戳格式仍待第二批）

## 1. 授权与安全边界

- 用户当轮明确授权：使用 `test` 数据源对 `ob_test` 库执行只读导出实测（5 项组合 + 2 项补测）。
- 密码从本地 SQLite 加密信封解密（控制面同款根密钥链：`var/local-mvp-root-key.json` DPAPI 载体 + AES-GCM），只经内存与 OBDUMPER 官方 `security.properties`（RSA 材料）短时传递，不进入 argv、日志、标准输出或文档；执行后清理材料并清零内存字节。
- 输出目录与工具日志（`tmp/exi7-run-01/`）保留供复核；`tmp/` 已被 Git 忽略，不提交任何输出、日志或临时材料。

## 2. 实测环境与命令基线

- 工具：OBDUMPER 4.3.5（`var/local-mvp-obdumper-4.3.5/ob-loader-dumper-4.3.5-RELEASE`），主类 `com.oceanbase.tools.loaddump.cmd.Obdumper`，Java 8（1.8.0_491）。
- 连接：`root@test#rlc_cdpV4 @ 192.168.2.53:2883`，库 `ob_test`。
- 命令基线（模拟平台启动参数）：`-Dsecurity.configurationFile`（安全材料）+ `-Dtool.base.dir` + `-Dsession.configurationFile` + `-Ddecrypt.configurationFile` + `-Dlog4j.output` + `-Dhadoop.home.dir` + `-Dfs.defaultFS=file:///` + `-classpath <tool>/lib/*`。
- **平台差异实测发现**：OBDUMPER 4.3.5（Hadoop 3.3.6）在 Windows 上把盘符绝对路径（`E:/...`）解析为 `file://nullE:/...`（Wrong FS）；`-f`/`--log-path` 必须使用相对路径（POS 受控实测同口径）。控制面/Agent 正式任务路径传递需核对此约束（平台适配层）。

## 3. 实测结论

| 组合 | 结果 | 证据 |
|---|---|---|
| `--ddl --all`（基线） | code=0 | 8 个 `-schema.sql`（TABLE/PROCEDURE 等） |
| `--ddl --all --compact-schema` | code=0 | 8 个 DDL 文件与基线**逐字节一致**（当前库对象无差异） |
| `--csv --all --snapshot` | code=0 | 全量导出完成（frozen_version 提示见于失败轮次日志） |
| `--csv --all --weak-read` | code=0 | 全量导出完成 |
| `--csv --all`（prep）→ 中断 | 中断 | 数据量太小（全量 7 秒完成）无法在导出中制造保存点 |
| `--csv --table t_order_test --retry`（无保存点） | code=1 | 失败关闭：`File: "<out>\.dump.ckpt" is missing` |
| `--csv --table t_order_test --where "id < 100"` | code=0 | 导出 109 数据行（全表 1,000 行） |

## 4. 参数定版

| 参数 | 官方语义（4.3.5 --help/文档） | 实测结论 | v1 状态 |
|---|---|---|---|
| `--compact-schema` | `Export the schema text which are retrieved by executing 'show create table'` | 运行成功；当前库对象（无列注释/索引的对象）与完整 DDL 无差异 | **ENABLED** |
| `--snapshot` | 导出最近一次合并版本的快照数据（官方 V2.2.2 参数表） | 运行成功，正常导出 | **ENABLED** |
| `--weak-read` | `Interpret to dump from follower server`（从备库读） | 运行成功，正常导出 | **ENABLED** |
| `--retry` | `是否从最近一次的保存点继续导出`（官方 V2.2.2 参数表） | 无保存点失败关闭（明确错误）；续跑需真实中断环境 | **ENABLED**（续跑并入 EX-I8） |
| `--where` | 支持指定全局条件导出符合条件的数据（如 `--where 'age>16 and age<65'`） | 行数筛选生效（109 vs 1,000） | **ENABLED** |

## 5. 残余与未覆盖项

- `--compact-schema` 的差异效果需要包含注释、索引、分区策略复杂对象的目标库才能证伪（当前 `ob_test` 对象为紧凑形态）；语义已由官方 help 确认，接入后不影响无差异场景。
- `--retry` 的"从保存点续跑"行为：`ob_test` 数据量（全量 15,004 行、约 7 秒完成）无法在导出中稳定制造保存点；该行为并入 EX-I8 `dump.ckpt` 继续功能，在真实大数据/慢导出环境验证。
- 第二批待实测：时间戳值格式（--date-value-format 等 10 个，需含日期/时间列的表，`ob_test` 的 t_order_test 有 created_at/updated_at 可作观察对象）、`--partition`（t_hash_prune_test 是 hash 分区表，可测）、`--exclude-data-types`/`--enable-hidden-pk`（需对应表结构）、对象类型 13 个（需目标库对象存在性核对）。
- `--add-extra-message`：映射文档标 ENABLED 但 v6 元数据未接入；依赖 sys 凭据可用性（数据源 sysCredentialState=AVAILABLE 信号已就绪），接入时需验证 sys 凭据真实有效。
