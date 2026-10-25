# 普通导入 Windows MySQL 真实端到端验证

## 结论与范围

2026-10-19 用户要求“使用现有数据源进行普通导入模块，真实环境的端到端测试验证”。本轮使用现有非生产 `test-mysql`、`win-test` 节点及允许根目录内独立小型夹具，完成 **20 项：16 PASS、4 FAIL**。共提交 **8 个真实任务：6 SUCCEEDED、2 FAILED**，均已终态且无待核对 execution。任务成功不直接计为内容正确。

已验证真实五步页面、草稿保存、唯一生成器、六项预检查、受认证 Agent 领取、固定 OBLOADER 4.3.5、命令/快照绑定、日志与持久完成事件；独立查询全部目标记录与列结构，另验证幂等、失败关闭、配置派生、模板重新绑定及版本/CSRF 保护。CSV 全字段加引号、CUT、SQL、仅结构的本轮数据/结构断言通过；CSV 未加引号的空格保留断言、显式 strict 参数和重跑错误分类未通过。**不构成普通导入全量 NIV、G3/G4 或生产准入通过。**

未使用 `test-oracle`，未测试 Oracle、麒麟、对象存储、压缩、非 UTF-8、控制文件、全量高级参数、取消/活跃进程恢复、跨主体越权及 `load.ckpt` 继续；不以这些环境的已有数据源记录代替真实通过。未重跑历史大文件草稿，未访问或清理用户既有业务表和上轮夹具。

## 运行与证据

- Run：`b4dbfdfe3e3e4c99ae2388f616a86ce7`；实际执行约北京时间 20:56～21:04，内容核对、清理与健康复核至 21:08。
- 独立库：`odo_e2e_20261019_b4dbfdfe3e3e`，`odo_owner` 保存本轮 runId；目标表均为 `nivt_` 前缀。CSV 4 行，CUT/SQL/结构加数据各 2 行，加引号 CSV 4 行；失败夹具仅 1 条已存在主键。
- Windows AMD64、Java 8、OBLOADER `4.3.5-RELEASE`、固定 `oceanbase-client-2.4.14.jar`；节点环境检查 NORMAL，真实执行与普通导入独立开关已开启。
- 测试既有运行二进制，没有安装、重启、修改服务配置或升级运行 SQLite。二进制生成于本日 19:18；构建元信息为 `-trimpath=true`、`CGO_ENABLED=0`、Windows AMD64、Git `0c5953cdbf406c01ee89ac9018af90a123d67b64` 加 dirty。不能用该标识声称当前全部未提交工作树都已安装。
- Agent SHA-256：`AAC5AC460674597BAB6F235F01DC9CAEFE953DD283069488CD7800B693D8142A`；控制面 SHA-256：`59C55A1DDEEDB103F5B4AEC6F6DED85DAC23AEBA3078C7EBA6E3EA382D85600D`。
- 第一条用例由正常已登录 Edge 页面完成全部五步及确认提交；其余矩阵在同源已登录浏览器调用正常业务 API，未拦截请求、修改运行数据时间或伪造结果。输入密码不离开正常平台安全槽位；夹具辅助程序仅经匿名 stdin 交给短进程。
- [逐项安全结果 JSON](normal-import-mysql-e2e-2026-10-19-results.json)包含预检查、任务标识、计划/实际一致性、持久结果、完整内容/结构断言与差异。复现入口为 [验证脚本](../../../scripts/normal-import-e2e/README.md)。原始工具输出、数据库和真实凭据不进入仓库。

## 逐项结果

| ID | 场景 | 结果 | 实际证据 |
|---|---|---|---|
| NI-E2E-01 | 五步页面 CSV 特殊值 | FAIL | 4→4 行，工具成功，但 2 个未加引号的空格值改变 |
| NI-E2E-02 | CUT 数据 | PASS | 完整 2 行与列类型、长度、精度、可空性、主键一致 |
| NI-E2E-03 | SQL 数据 | PASS | 完整 2 行及结构一致，空白字符串保留 |
| NI-E2E-04 | 仅结构 | PASS | 固定 6 列结构与空表一致 |
| NI-E2E-05 | CSV 结构和数据 | FAIL | 结构及 2 行数量一致，1 个空白字符串变为空串 |
| NI-E2E-06 | 首条列数不匹配 | PASS | INPUT_MAPPING/IMPORT_COLUMN_COUNT_MISMATCH；提交 422；目标未写入 |
| NI-E2E-07 | 512 MiB 堆 | PASS | NODE_RESOURCE/IMPORT_JVM_HEAP_TOO_SMALL；提交 422 |
| NI-E2E-08 | 空输入目录 | PASS | INPUT_FILES 失败；提交 422 |
| NI-E2E-09 | 普通导入注入旁路字段 | PASS | 创建 400/REQUEST_INVALID；不预检、不执行 |
| NI-E2E-10 | 配置注入任务级密码字段 | PASS | 创建 400/REQUEST_INVALID；仅合成拒绝值 |
| NI-E2E-11 | 仅文件发现后提交 | PASS | INPUT_FILES 通过，但非完整用途；提交 422 |
| NI-E2E-12 | 配置改变后复用旧检查 | PASS | 修订变化；旧检查提交 422 |
| NI-E2E-13 | 全字段加引号 CSV | PASS | 全部 4 行逐值一致：NULL、空串、空白、中文/emoji、引号、逗号、反斜杠和嵌入换行 |
| NI-E2E-14 | 显式 strict=true 下重复主键 | FAIL | 预检查全通过，但生成 `--strict true`，工具在解析参数时退出 1，未到重复数据写入 |
| NI-E2E-15 | 成功任务配置派生 | PASS | 同幂等键返回同草稿，来源关系正确，导出端点读取为 404 |
| NI-E2E-16 | 成功任务从头重跑 | PASS | 422/TASK_DERIVATION_UNAVAILABLE |
| NI-E2E-17 | 普通导入模板 | PASS | 保存、类型、重新绑定、清名称正确；旧检查不能解锁提交 |
| NI-E2E-18 | CSRF 与旧修订 | PASS | 缺 CSRF 为 401/CSRF_VALIDATION_FAILED；错误修订为 412 |
| NI-E2E-19 | 失败任务重跑参数锁定 | FAIL | 内存改值被拒绝且原值/修订不变，但返回 503/NORMAL_IMPORT_SAVE_UNAVAILABLE，未正确表达配置锁定 |
| NI-E2E-20 | 继承 strict 默认值时重复主键 | PASS | 工具明确报告 Too many discard records，退出 1，任务 FAILED，原单行目标完整不变 |

所有实际提交均核对命令、冻结快照与执行摘要；矩阵提交复放返回同一任务。每个真实任务只有一个接受的 PROCESS_STARTED 和一个 RESULT_FACTS_OBSERVED，持久结果含 `normalImport=true`、`checkpointPresent=false`，不含导出文件计数/清单。

对应既有 NIV：`NIV-P0-04` 内容组合仅部分通过，`NIV-P0-06` 仅覆盖当前 CSV/CUT/SQL 切片，不提升七格式矩阵；`NIV-P1-01/02/03` 仅覆盖本轮特殊值及解析对照；`NIV-P0-21` 的错误/重复处理仅覆盖显式 strict 失败与继承默认值的单行重复主键。完整参数定版所需的重复独立执行与其余组合没有完成。

## 未通过项及边界

1. **CSV 空格保留（01、05）**：未启用 with-trim 时，未加引号的 `" "` 实际成为空串，`"  padded  "` 实际成为 `"padded"`。01 和 05 的独立任务均观察到空白变化。全字段加引号的独立 13 完整保留这些值。此结论描述本机固定工具/输入组合，不断言整个 CSV 格式损坏，也不把“4 行成功”写成无损通过；需要保留空格的输入须结合本轮已验证引用方式评估。
2. **strict 参数（14）**：脱敏计划与实际参数一致，但 `--strict true` 被工具报告 `Unmatched argument at index 23: 'true'`；平台预检不能发现该 CLI 参数表达问题。20 未显式设置 strict，执行到数据库写入并按丢弃阈值失败，不能用 20 覆盖 14。`strict=false` 未执行，不能外推其真实行为。本轮只验证和记录，未修改产品命令契约/生成器或安装服务。
3. **重跑错误分类（19）**：仓储保持参数锁定，读回原内存及原修订均不变，没有形成锁定绕过；HTTP 层把该拒绝映射为通用 503。初轮脚本没有保存拒绝状态就使用笼统断言，本轮随后以新派生草稿复验，明确保存 503 和不变事实，保留初轮记录作测试准备证据。

额外观察：当前列表/详情标题仍显示目标库摘要，用户任务名已在冻结快照保存；普通导入安全投影刻意返回空的导出型 resultSummary，因而详情仍显示“尚无结果证据”，而 SQLite 的独立普通导入事件为 VERIFIED/FAILED。此表现属于现有投影/反馈限制，不能说事件丢失，也不能借导出计数补零。工具在 Windows `/E:/...` 检查点序列化路径上打印 InvalidPathException；`load.ckpt` 继续仍不开放，该观察不证明可恢复性。

## 收尾与检查

- 完整目标行集与结构已在清理前读取；列名/类型、字符长度、Decimal 精度/小数位、可空性、主键均核对。结果 JSON 的 differences 只有本轮合成记录，不包含用户业务数据。
- 先保存内容核对和 9 个输入文件 SHA-256，再核验 `odo_owner` 与精确库名清理本轮数据库；核验绝对路径位于允许根目录且无重解析点后清理本轮输入/工具日志目录。没有清理历史目录。5 个未提交检查草稿、4 个派生草稿及 1 个模板均已清理；8 个提交任务及其执行/审计/脱敏日志保留供复核。
- 最终 HTTPS `/healthz` 为 200，Agent ONLINE，环境 NORMAL，可接收任务；running/waiting/reconciling 全为 0，activeExecutions 为 0，日志队列 pendingBatches/pendingGaps/pendingBytes 全为 0。
- 真实密码及其 Base64/URL 编码的内存泄漏审计、JavaScript/Python 脚本语法、夹具 CGO=0 构建、`scripts/check-secrets.ps1` 与 `git diff --check` 均通过。未运行全量 Go/前端合成套件，也没有以其替代上述真实内容核对。
- 本轮未修复产品代码、未提交、未推送；新增测试入口与脱敏证据，不改写已有普通导入历史成功事实。当前 4 项失败仍需处理，不能宣布模块全量可用。
