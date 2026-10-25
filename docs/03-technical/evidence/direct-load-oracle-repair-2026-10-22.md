# Oracle 旁路修复与真实 API 验收 · 2026-10-22

**结论：本次 Oracle 直连批次 12/12 API 用例通过，7/7 真实旁路任务成功，独立目标行集、完整 DDL 和持久提交事实全部核对通过。** 此结论替代[此前 Oracle 阻断批次](direct-load-oracle-api-2026-10-22.md)的当前状态；历史记录保留。本轮已经安装控制面和 Agent，不仅是源码或合成验证。安全摘要见 [results.json](direct-load-oracle-repair-2026-10-22-results.json)。

用户要求继续 Oracle 验证并修复，沿用同会话明确授予的真实 API、凭据解析、工具执行和测试资产创建/清理权限。范围限定 Windows AMD64、Java 8、Connector/J 2.4.14、固定 OBLOADER 4.3.5、OBServer 4.4.2.3、ORACLE、OBSERVER_DIRECT、当前账号 Schema 内既有单表、合成 CSV。原共享源及凭据修订未变，未修改数据库账号授权。

## 修复与官方依据

此前三项 UNKNOWN 来自平台缺口：Oracle 固定权限/结构分支尚未实现，且固定工具与此 Oracle 版本组合缺少独立实证。没有据此认定官方不支持。

- 固定 JDBC 仅在 `SYS_CONTEXT('USERENV', 'CURRENT_USER')` 与冻结 Schema 精确相等时，依据对象所有权检查既有表操作；使用绑定 OWNER/TABLE_NAME 的 `ALL_TABLES`、`ALL_TAB_COLS`、`ALL_INDEXES` 和固定空性查询读取结构。跨 Schema、角色/ANY 权限、未知或超预算元数据继续 UNKNOWN。
- 四项必要零行 SELECT 权限按官方 4.3.5 [加载说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997308)核对：`SYS.GV$OB_MEMSTORE`、`SYS.DBA_OB_MAJOR_COMPACTION`、`SYS.DBA_OB_LS_LOCATIONS`、`SYS.all_virtual_proxy_schema`；Oracle 最后一项不套用 MySQL 双下划线名称。
- Oracle 用户名和连接映射对照官方[旁路导入说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997310)。旧文档版本矩阵不能自行证明新组合；本轮先以固定官方主 JAR、唯一生成器和生产受控 Java 启动器取得独立同表提交、退出及完整结果，再增加 ORACLE/4.4.2.3 精确版本行。未放开其他新版本。
- 全表 DDL 取证使用官方 [4.4.2 DBMS_METADATA](https://docs.oceanbase.com/oceanbase-database/V4.4.2-dbms_metadata-overview-oracle-compatible-mode) 的 `GET_DDL`。此查询只属于固定测试夹具，不进入生产 JDBC 探针或产品任意 SQL 入口。

协议仍为 v11；没有新增秘密槽位、驱动、任意连接串或 SQL 入口。MySQL 探针规则保持原分支。

## 实际 API 与独立验收

正常 admin 登录使用受信 CA、Secure Cookie、CSRF、修订与幂等键；密码仅以短时字节经匿名 stdin 和 HTTPS 传递。通过 API 新建独立 Oracle 直连 TEST 源，由真实 Agent 基础 JDBC 验证后启用。未改写原源的 ODP 登记以冒充直连。

| 用例 | 实际结果 |
| --- | --- |
| SOURCE_CONNECTION_TEST | 真实 Agent JDBC SUCCEEDED |
| BASIC | 九项预检通过；缺少最终确认被拒；幂等重放返回同一任务；真实导入成功 |
| SHARDS | 同表两个 CSV 分片，真实导入成功 |
| SEPARATOR | 自定义分隔符，真实导入成功 |
| INCREMENTAL | 导入完整两行，原有第三行保留 |
| TRUNCATE | 原有第三行清空，最终完整两行匹配 |
| THREAD / PARALLEL | 两个分别配置并发参数的任务成功 |
| RAW_BLOCKED | 平台二进制受控边界阻断，提交 422 PRECHECK_REQUIRED |
| INDEXED_REPLACE_BLOCKED | 有索引的非空表替换被阻断，提交 422 PRECHECK_REQUIRED |
| LOB_REPLACE_BLOCKED | 含 LOB 的非空表替换被阻断，提交 422 PRECHECK_REQUIRED |
| CONFIG_CHANGE_INVALIDATES_PROOF | 原证明 INVALIDATED，旧证明提交被拒 |

拒绝用例通过表示平台边界正确生效，不表示这些类型或操作被官方工具普遍禁止。虚拟列、隐藏列和未知元数据等失败关闭行为另由合成 JDBC 反例验证，未声称真实 Oracle 虚拟列场景已验收。

7 个真实任务均核对：受控退出 0、实际与计划参数 MATCHED、精确目标表的官方提交成功行、完整日志、单个接受启动/结果事件、持久 `VERIFIED`、`directLoad=true`、`serverCommitObserved=true`、无 checkpoint；未伪造导出文件结果计数。独立固定夹具读取完整目标行集和 DBMS_METADATA DDL，并比较列、索引、约束；全部结构不变，3 个拒绝目标完全未变。合成数据包含中文、NUMBER 精度/小数及负数，数值比较按 Decimal 规范化，避免把 Oracle 合法尾零差异误判为数据损坏。API 回执本身不代替这些独立结果。

## 安装、检查与清理

安装前以 SQLite backup API 留存一致备份；按项目流程停止原进程、确认控制面端口关闭，删除旧二进制后以 `CGO_ENABLED=0`、`go build -trimpath` 构建安装，再保留原参数、工作目录、执行开关、身份/TLS 和有效 HADOOP_HOME 恢复服务。最终 HTTPS 健康 200、Agent ACTIVE、环境 PASSED、占用 0、心跳新鲜，未结束执行为 0，原有四项开关均保持。包摘要见机器回执。

- 全量 Go 单元/契约测试、`go vet`（含固定夹具）通过。
- Java 8 固定 JDBC 测试、Oracle SQL 白名单与权限/结构失败关闭反例通过，生产探针 JAR 已重建并装入新服务。
- 新执行夹具 Java 编译、Node 语法、PowerShell 语法、差异及秘密扫描通过；HTTPS 未登录边界 7/7 通过。
- 原始证据 43 文件的 admin 与 Oracle 原值/可恢复编码检查通过；安全摘要不收录连接端点、账号、Schema 或原始工具输出。
- 组件与 API 两个批次的所有权表和输入均已清理；API 测试源归档，未提交草稿清零。输入目录无失控运行日志，原共享源与凭据修订保持不变。

原始忽略证据：组件 `artifacts/direct-load-e2e-20261022-cd4e6ee75fff/`；真实 API `artifacts/direct-load-e2e-20261022-2534b1202e0c/`；安装/秘密审计/最终运行时 `artifacts/direct-load-oracle-repair-20261022/`。独立收集器为 `scripts/direct-load-e2e/collect-oracle-execution.py`。已清理批次不可重放；新验证入口与独立收尾步骤见[脚本说明](../../../scripts/direct-load-e2e/README.md)。

本轮解除上述精确 Oracle 组合在当前账号 Schema 内的闭环阻断。其他 Oracle 类型/NLS、全参数取值、跨 Schema、ODP/F5、其他版本、麒麟原生、取消/恢复及生产环境没有由此取得验证；不宣称完整 DLV/G3/G4 通过，也不解释此前 MySQL 批次的历史 RPC 超时原因。
