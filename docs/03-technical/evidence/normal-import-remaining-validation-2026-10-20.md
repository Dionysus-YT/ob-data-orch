# 普通导入剩余场景验证（MySQL / Oracle）

2026-10-20 用户明确允许使用现有 MySQL 和 Oracle 数据源补验剩余场景。本轮通过正常 HTTPS API、真实 Windows Agent 与 OBLOADER 4.3.5 执行，保留认证、CSRF、修订、幂等、完整预检查和冻结参数校验。

**Oracle NLS 三条定义现已通过真实结果验证，当前 53/72 条定义有成功执行场景；另外 19 条仍因现役预检查能力限制而阻断，全参数功能尚未全部通过。** 本轮不是将正确拒绝记作功能成功，也不把三个小型样本外推为全取值、全部类型/权限或其他平台通过。

## 新增通过的三项

| 定义 | 固定输入格式与值 | 独立结果 |
| --- | --- | --- |
| nls-date-format | DD/MM/YYYY HH24:MI:SS；20/10/2026 12:34:56 | DATE 按固定 TO_CHAR 回读为 2026-10-20 12:34:56 |
| nls-timestamp-format | DD/MM/YYYY HH24:MI:SS.FF6；20/10/2026 12:34:56.123456 | TIMESTAMP(6) 的六位小数全部匹配 |
| nls-timestamp-tz-format | DD/MM/YYYY HH24:MI:SS.FF6 TZH:TZM；20/10/2026 12:34:56.123456 +08:00 | TIMESTAMP(6) WITH TIME ZONE 的时间、小数和 +08:00 偏移全部匹配 |

三张本轮独立表导入前均为空，导入后完整行集均恰好一行；主键、列顺序、类型与主键非空约束核对通过。三个 API 任务均 SUCCEEDED、进程退出 0、参数 MATCHED、normalImport/VERIFIED，无待核对；计划命令/快照指纹、精确 NLS 值、提交幂等、成功终态日志及各一次的 PROCESS_STARTED / RESULT_FACTS_OBSERVED 持久事件均已独立核对。

只使用现有 Oracle 用户的所属 Schema，夹具表和 owner 标记由随机 runId 派生；没有创建新用户、授予系统权限或修改其他用户对象。夹具准备和独立回读使用测试专用固定 JDBC 主类，不进入产品 JAR/API，也不接受任意 SQL/驱动/URL。真实导入任务全部走普通产品 API 和 Agent。

## 定位并修复的阻断

固定 JDBC 诊断确认：当前 Oracle 源可连接，所属 Schema 与当前用户匹配，SYS_CONTEXT 可用；SESSION_PRIVS 查询不可用，USER_SYS_PRIVS 可以读取。源用户具备通过既有权限创建本轮固定小型表的能力，未追加授权；不能把 USER_SYS_PRIVS 未见直接 CREATE TABLE 理解为没有角色权限。

原探针在核对所有者之前无条件读取 SESSION_PRIVS，使所属 Schema 的既有表写入也被投影为 IMPORT_WRITE_SCHEMA_UNAVAILABLE。修复后：

- Schema 所有权由固定元信息和当前用户核对，既有所属表操作依据所有权，不依赖系统权限视图。
- CREATE 或跨 Schema 操作才读取 USER_SYS_PRIVS 的明确直接系统授权；角色不展开，信息不足继续 UNKNOWN，不按角色名猜测权限。
- 固定 v10 帧、原因分类、普通任务授权、输入证据和执行开关不变；生产探针仍只读，不试写或执行文件 SQL。

Java 8 合成用例验证了所属表不依赖不可用权限视图、明确 CREATE TABLE、缺直接授权、跨 Schema 有/无明确 ANY 权限和无法读取权限等正反例。直接权限视图的语义参考 [OceanBase 官方权限查看说明](https://en.oceanbase.com/docs/common-oceanbase-database-10000000003455054)；本轮运行结论以固定 JDBC 实测和 API/目标结果证据为准。

## 仍阻断的 19 条

| 条数 | 定义 | 实际结果 |
| --- | --- | --- |
| 6 | ctl-path、exclude-data-types、skip-footer、compress、compression-algo、compression-level | 输入映射能力尚未实现，INPUT_MAPPING UNKNOWN；17 条拒绝提交的批次中包含这 6 条，无工具任务 |
| 11 | table-group、view、trigger、sequence、synonym、type、type-body、package、package-body、function、procedure | 复杂定义权限核算尚未实现，WRITE_ACCESS UNKNOWN / IMPORT_WRITE_DDL_UNSUPPORTED；提交拒绝，无任务 |
| 2 | obj-user、role | 同样缺少对应权限核算；仅真实预检查，不尝试创建账号/角色或提交安全对象任务 |

19 项全部完成真实预检查及冻结绑定核对；17 项实际提交返回拒绝，2 项只读预检，SQLite 按各 precheck_id 核对均零任务，全部临时草稿删除成功。输入文件只用于验证当前阻断路径，控制文件仍为占位、压缩文件为合成样本，复杂对象未经过官方工具解析，**不能据这些负例推导有效控制文件、官方压缩产物或复杂定义的正向可用性**。

因此，剩余问题已不属于“没有可用数据源”：需要补齐受控控制文件/压缩/尾行/类型映射取证以及相应复杂对象的定义识别、授权和独立结果核对能力，再作正向工具与结果测试。本轮没有为获得通过而关闭预检查、扩大任意 SQL 或调整数据源权限。

## 安装、检查与清理

工具任务全部终态后才停止旧服务并确认监听关闭；复用原开发入口和参数，删除旧二进制后以当前源码 CGO_ENABLED=0、go build -trimpath 重建控制面及 Agent，保留原配置/CA/根密钥并恢复。控制面 PID 30404、Agent PID 33400；控制面未改逻辑，重建后确定性摘要相同，Agent 内嵌探针摘要已更新。恢复后的完整节点检查 PASSED，三项真实任务由该版本执行。

- Java 8 探针构建与全套八类合成主类检查通过；受影响 jdbcprobe、agentjdbc、agentexecution、importfiles、agentwire 测试与定向 vet 通过。
- API 客户端/门禁脚本 6 项检查通过；本轮没有前端行为变更，不重复浏览器或全前端验收。
- 结果与汇总发布前分别对 MySQL、Oracle 源密码进行明文及可恢复编码布尔审计，均通过；秘密不进入 argv、环境、文件、日志或交付证据。
- 取证后按 owner 标记删除 Oracle 三张表及 owner 表，并查询确认四表不存在；删除本轮独立 MySQL 库并查询确认不存在。
- 精确输入目录在允许根目录内，核验无重解析点、保存文件摘要后删除并确认不存在；保留任务、事件和脱敏证据。
- 收尾 HTTPS 健康 200、心跳 ONLINE、完整环境 PASSED；running/waiting/reconciling、占用额度及日志队列待发批次/缺口/字节为 0。

本轮新增三个成功任务；连同前一批次累计 45 个真实工具任务，41 SUCCEEDED、4 FAILED（包括前批预期反例与已修复历史失败），全部终态。旧批次的失败、阻断和 50/72 摘要保留，不改写为本轮实测。

## 证据与复现边界

- [本轮三项 Oracle、十九项阻断及安装/清理结果](normal-import-remaining-2026-10-20-results.json)
- [当前 72 条逐参数状态](normal-import-full-parameter-validation-2026-10-20.json)
- [前一批次 50/72 摘要快照](normal-import-full-parameter-validation-2026-10-20-initial-summary.json)
- [前一批次完整验证记录](normal-import-full-parameter-validation-2026-10-20.md)

脚本见 [API 验证入口](../../../scripts/normal-import-e2e/README.md)。本轮日记位于忽略目录 artifacts/normal-import-remaining-20261020；夹具已清理，不得重放旧日记，也不能用保留的回读结果声称重新查询了目标数据库。代码未提交或推送；未验证其他平台、恢复、规模、所有格式/取值组合或生产环境，不提升完整 NIV/G3/G4。
