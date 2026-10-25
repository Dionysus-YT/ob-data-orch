# 旁路导入真实验证辅助入口

仅用于本次明确授权的既有 TEST MySQL / Oracle 数据源、Windows AMD64、固定 Java 8 / Connector/J 2.4.14。不是产品 SQL/命令入口，不接受任意 SQL、主类、连接字符串或数据库来源。

- `scripts/export-e2e/fixture` 复用现有只读凭据解析，新增 `direct-probe/create/inspect/cleanup/jdbc` 固定动作。密码只在内存及子进程匿名 stdin 中短时传递，运行输出不包含原值或可恢复编码。
- `fixture.py` 只接受 `odo_e2e_YYYYMMDD_<run前12位>` 独立库及对应 `dlvt-direct-<run前12位>` 目录；创建冲突即拒绝，检查/清理核对 `odo_owner`。清理先确认库已不存在，再验证精确绝对目录后删除，不接管历史数据。
- `direct-probe` 读取当前获授权 SQL 连接的固定版本/零行系统视图，并核查同主机两个默认 RPC 端口；仅在当前 SQL 不是 2883 时检查同主机官方默认 ODP SQL 端口，拒绝后不猜测口令或连接身份。TCP 可达不证明 RPC 协议、版本或导入成功。
- `direct-jdbc` 使用当前源码的真实固定探针，校验 Java/Connector/J/探针摘要，仅读取本轮固定表的连接、权限、版本与结构。它不创建平台预检查或执行证明，成功不等于真实旁路写入通过。
- `direct-compatibility` 仅在所有权空表上运行固定主 JAR、唯一命令生成器和生产受控 Java 启动器；进程、同表提交、日志与独立目标行集共同核对。它是组件验证，不生成产品任务或预检查证明。Windows 原生依赖须使用已核验的现役 `HADOOP_HOME`，不能用缺失 DLL 的默认目录替代。
- `auth-boundary.mjs <artifacts/direct-load-e2e-YYYYMMDD-...>` 复用受信 CA 的回环 HTTPS 客户端，只验证未登录 401 和健康 200；没有会话替代、独立 Token 或身份绕过，零任务不计为功能成功。
- `pwsh -File scripts/direct-load-e2e/api-test.ps1 -RunDirectory artifacts/direct-load-e2e-YYYYMMDD-...` 使用正常 admin 登录、Secure Cookie、CSRF、幂等键与修订执行现役业务 API 测试。密码不回显，仅经匿名 stdin 传给 Node，发送和登录完成后分别清零；不得把密码放入命令行、环境、脚本或日记。
- 加 `-Standalone` 选择 `standalone-api.mjs`：先创建自有库/输入，再通过正常 API 登记独立 TEST 直连数据源、真实 Agent JDBC 测试和启用；不改写原共享数据源。覆盖九项预检查、最终确认缺失拒绝、提交幂等，以及基础 CSV、同表分片、自定义分隔符、增量、truncate、thread/parallel 的七个真实任务；五类结构阻断和旧证明失效均实际拒绝提交。`-Standalone -Smoke` 保留基础任务和全部拒绝用例，用于安装后回归。
- 加 `-Oracle` 选择 `oracle-execution-api.mjs`，与 `-Standalone` 互斥。只从既有 TEST Oracle 源取得连接输入，在当前用户 Schema 创建随机 `DLVO_<run前12位>_` 所有权表和 10 张固定测试表，使用独立 `dlvt-oracle-<run前12位>` 输入。通过正常 API 登记独立直连源、真实 Agent JDBC 验证与启用，覆盖基础 CSV、同表分片、自定义分隔符、增量、truncate、thread/parallel 七个真实任务、RAW/索引/LOB 三类结构阻断，以及旧证明失效，共 12 项；最终确认缺失拒绝和幂等重放在基础任务中核对。`-Oracle -Smoke` 保留基础任务、连接和全部拒绝用例，共 6 项。
- Oracle 固定测试类 `DirectLoadOracleFixture.java` 和 `DirectLoadOracleExecutionFixture.java` 均须用 Java 8 编译到 `artifacts/direct-load-oracle-tools/classes`，Go helper 构建到同目录 `fixture-oracle.exe`。固定动作绑定当前 Oracle 用户和完整 run 所有权，连接器摘要固定；不创建用户、授权或修改既有对象。完成后必须运行 `python scripts/direct-load-e2e/collect-oracle-execution.py <RunDirectory>`，独立核对完整行集、DBMS_METADATA DDL/列/索引/约束、持久事件与摘要、原源修订、归档及清理。此收集器还比对本次安装前的一致 SQLite 备份；新验收批次须先准备并核验其基线，不能套用旧环境备份。当前结果见[Oracle 修复验收](../../docs/03-technical/evidence/direct-load-oracle-repair-2026-10-22.md)。
- 单机批次执行后必须运行 `python scripts/direct-load-e2e/collect.py <RunDirectory> --require-private-logs --require-summary-commit`，独立比较完整目标行集、DDL、未触及表、持久事件/摘要、进程证据与精确清理。`apiPassed` 本身不是 E2E 成功；私有日志检查要求输入目录没有额外日志。固定 4.3.5 + MYSQL 直连 4.4.2.3 的实证不外推 Oracle、其他版本、F5、ODP 或未开放参数。
- 两个真实执行 Node 入口支持 `--replace-smoke`：基础任务在空表上显式开启 replace-data，保留对应结构拒绝和旧证明反例；同任务内验证原键重放、读取原任务 ID、换键 409 和异名称 409，并由独立收尾核对该源没有额外任务。此模式仅作为本轮隔离 TEST 验收，不扩大增量替换、类型或生产范围。
- API 批次需要预先创建新的 `run.json`，包含随机 32 位小写十六进制 `runId` 及 `odo_e2e_YYYYMMDD_<run前12位>` 数据库标签；目录必须使用同一 run 前缀。Oracle 标签只用于批次绑定，不创建同名数据库。`api-started.json`、`standalone-started.json` 和 `oracle-execution-started.json` 均排他创建，已有启动回执即拒绝重放。真实执行入口自建夹具，不能事先对同批次调用 create。失败批次须先核对回执与终态，不删除启动标记重试。
- API 覆盖参数目录、草稿创建/读取/修订、CSRF、幂等重放与冲突、输入反例、单表/分片/空目录/多表/缺失/越界路径发现、预检和拒绝提交。文件发现通过只证明 Agent 读取和单表清单，不证明列解析、数据库写入或旁路执行。
- 默认 `api-runner.mjs` 是历史非执行覆盖：私有 ODP 的已登记集群/租户事实缺失时，完整草稿、命令预览和完整预检查记为 `BLOCKED`，不填假集群、借用版本或强制通过。此入口不发起真实任务；单机真实执行必须使用上面的独立入口。 历史 `oracle-api.mjs` / `collect-oracle.py` 保留修复前零任务阻断取证逻辑，不是 `-Oracle` 当前执行入口或成功验收依据。
- 结束时清理本批次回执中的草稿及所有权验证通过的库/目录；检查未终态时保留资产，记录 `cleanupDeferred`，先核对终态再清理。原始回执写入忽略的 artifacts，不进入 Git。

实际批次、准备及清理状态见[早期测试记录](../../docs/03-technical/evidence/direct-load-e2e-2026-10-20.md)、[Oracle 修复验收](../../docs/03-technical/evidence/direct-load-oracle-repair-2026-10-22.md)和[单机修复验证](../../docs/03-technical/evidence/direct-load-standalone-repair-2026-10-22.md)。已清理批次仅供核对，不能重放后冒充新批次或重读目标结果。后续认证业务 API、Agent 检查与工具执行仍须正常登录和可验证的实际 ODP/OBServer 连接。
