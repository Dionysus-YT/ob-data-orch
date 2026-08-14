# EX-I7 剩余参数第二批受控实测与定版（2026-08-13）

> 验证状态：MySQL DATE/DATETIME 两个格式参数已观察到输出效果；其余时间格式只确认工具接受，缺少对应类型列或 Oracle 行为证据；`--partition`/`--exclude-data-types` 定版 ENABLED；隐藏主键与附加对象信息仍缺产品前置校验
> 授权范围：`test` 数据源、`ob_test` 只读测试 Schema（`t_order_test` 系列 + `t_hash_prune_test` 合成分区表）；真实身份、端点和对象名均已脱敏；输出位于 Git 忽略的受控临时目录 `tmp/exi7-run-02/`
> 关联：EX-I7 第二批、EX-F055~EX-F064（时间格式）、EX-F066/EX-F072/EX-F075/EX-F077

## 1. 授权与安全边界

- 用户当轮明确授权：使用已授权 `test` 数据源对只读测试 Schema 执行第二批导出实测（时间戳格式/分区/类型排除/对象类型/add-extra-message 共 11 个组合 + 1 项基线 + 1 项补测）；文档只保留合成别名。
- 密码从本地 SQLite 加密信封解密（控制面同款根密钥链：`var/local-mvp-root-key.json` DPAPI 载体 + AES-GCM，AAD 由引用信息动态构造、与 `internal/credential.aad` 同构；数据库密码与 sys 密码使用独立 credential_id），只经内存与 OBDUMPER 官方 `security.properties`（RSA 材料）短时传递，不进入 argv、日志、标准输出或文档；执行后删除材料目录并清零内存字节。
- 实测辅助程序（`tmp/exi7-run-02/tool/main.go`）与执行脚本位于 Git 忽略目录，不提交源码；输出目录保留供本机复核，不提交任何输出、日志或临时材料。

## 2. 实测环境与命令基线

- 工具：OBDUMPER 4.3.5，主类 `com.oceanbase.tools.loaddump.cmd.Obdumper`，Java 8（1.8.0_491）。
- 连接：`<synthetic-user>@<synthetic-tenant>#<synthetic-cluster> @ <private-endpoint>:<port>`，Schema `<synthetic-schema>`（`ob_test`）。
- 命令基线（同第一批）：`-Dsecurity.configurationFile` + `-Dtool.base.dir` + `-Dsession.configurationFile=conf/session.config.json` + `-Ddecrypt.configurationFile` + `-Dlog4j.output` + `-Dhadoop.home.dir` + `-Dfs.defaultFS=file:///` + `-classpath <tool>/lib/*`；`--file-path`/`--log-path` 使用相对路径（第一批 Wrong FS 结论沿用）。
- 测试对象：`t_order_test`（11 列，含 `datetime` 列 created_at/updated_at）；`t_hash_prune_test`（4 列，含 `date` 列 trade_date，HASH 分区表 `partition by hash(user_id)`，8 个分区 p0~p7）。

## 3. 实测结论

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

## 4. 参数定版

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

## 5. 对象类型存在性核对与实测

测试库 `ob_test` 当前存在的对象类型（基线 `--ddl --all` 清单）：`TABLE`（6 张，含合成分区表）、`PROCEDURE`（1 个）、`USER`（1 个）。

| 对象类型参数 | 库中存在 | 实测 |
|---|---|---|
| `--table` / `--procedure` / `--obj-user` | 是 | 已实测（指定对象 DDL 导出成功） |
| `--view` / `--sequence` / `--synonym` / `--public-synonym` / `--trigger` / `--type` / `--type-body` / `--package` / `--package-body` / `--function` / `--role` / `--table-group` | **否**（只读测试 Schema 中不存在） | 未实测；参数存在性已由 `--help` 确认，行为待授权对象后验证 |
| 不存在的指定对象 | — | code=0 空输出（工具静默成功，不失败关闭） |

## 6. 残余与未覆盖项

- `--year-value-format` 核对结论（2026-08-13 官网核实）：**三处一致确认该参数不存在**——① OceanBase 官网命令行选项文档（社区版最新 + V4.3.1.1）时间戳格式系列只有 `--date-value-format`/`--time-value-format`/`--datetime-value-format`/`--timestamp-value-format`/`--timestamp-tz-value-format`/`--timestamp-ltz-value-format` 六项；② 4.3.5 二进制 `--help` 无该选项（实测 `Unknown option`）；③ 项目映射文档与字段规则无该条目。无需新增 BLOCKED 条目，后续接入不得引入该参数名。
- `--timestamp-value-format`/`--time-value-format` 等无对应类型列的格式效果未观察（库中无 timestamp/time/year 列）；官方 help 只能证明参数语义，不能替代行为验收，因此继续保持门禁。
- Oracle 模式参数（`--nls-*`、`--enable-hidden-pk` 的隐藏主键行为、`--timestamp-tz/ltz-value-format`）需要 Oracle 兼容模式库验证。
- `--add-extra-message` 的"额外信息"内容未在 MySQL CSV 输出中观察到；官方语义、适用格式、当前 sys 权限预检查和任务秘密槽位绑定完成前，产品不得透传。
- 13 种对象类型中 9 种测试库不存在，无法实测；不存在的指定对象"静默成功"行为已记录，产品预检查需自行核对对象存在性（不能依赖工具失败信号）。
- 第二批产品接入只开放 `--date-value-format`、`--datetime-value-format`、`--partition`、`--exclude-data-types`；其余已登记参数按上述残余继续失败关闭。
