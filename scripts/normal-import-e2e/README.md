# 普通导入真实端到端验证

2026-10-20 十九项实现及复验已完成：**19/19 PASS，8/8拒绝反例PASS，现役72/72定义有成功代表场景**，以[最新证据](../../docs/03-technical/evidence/normal-import-capabilities-validation-2026-10-20.md)为准。`-Capabilities`、`-CapabilityRetry`、`-CapabilityNegatives`、`-CapabilitiesCloseout` 分别对应本轮首次执行、核对旧终态后的七项定向修复、八项拒绝检查及精确草稿/服务收口。修复器只接受本轮已观察到的固定旧值，不能作为通用重试器；固定Java/Python夹具不进入产品包、不接受SQL或任意源。回执存在时拒绝重放，当前run所有对象/输入已清理。`collect-capabilities.py`与`merge-capabilities-coverage.py`仅复核留存事实并发布摘要，不能冒充清理后重读目标。前一批次53/72及下文50/72保留历史。

2026-10-20 后续双源补验：Oracle 三项 NLS 已通过真实 API/OBLOADER 及独立目标核对，当前覆盖 **53/72**，19 条现役预检查能力限制仍阻断。详细结果以[剩余场景报告](../../docs/03-technical/evidence/normal-import-remaining-validation-2026-10-20.md)为准；下文 50/72 是前一批次。

`-Remaining` 对忽略目录 `normal-import-remaining-20261020/run.json` 的全新所有权批次运行三项 Oracle DATA_ONLY 任务及 19 项受限配置预检，遇到已有执行回执即停止。Oracle 固定夹具由 `OracleRemainingFixture.java` 提供，只接受 probe/create/inspect/cleanup 及固定字段帧，经原 Go helper 的 `import-oracle-remaining-*` 固定动作进入。只加载摘要已核验的 oceanbase-client-2.4.14.jar，不接受 SQL、任意 URL/驱动或命令；该测试主类不进入产品探针 JAR。Oracle 表须先按本轮 owner 标记创建并核对为空；MySQL 沿用独立库固定创建器，不重放前轮已清理的 run。

`collect-remaining.py` 读取已保存的独立前后行集/结构结果，并核对实际任务、参数值、持久事件和受限检查零任务；发布前分别审计 MySQL 与 Oracle 密码。`merge-remaining-coverage.py` 保留前一批次 50/72 摘要，只用独立通过结果更新当前逐参数状态。`-RemainingCloseout` 复用正常登录确认本轮草稿清理、服务健康/心跳/环境、执行额度与日志队列。当前双源夹具和输入已清理，禁止用旧日记再次导入或声称重新读取目标数据。

2026-10-20 用户指定直接 API：`api-client.mjs` 使用受信任 CA 的回环 HTTPS、正常 `POST /login`、内存 Cookie 与 CSRF；没有独立 Token、认证旁路或机器身份替代。`api-test.ps1` 隐藏输入现有 admin 密码，经匿名 stdin 交给短进程，密码不进入 argv、环境、文件或输出；不再需要浏览器页面。密码输入后自动执行 72 条定义的真实 API 草稿/预览/错误类型/清理、完整节点环境检查、23 项整改与 28 项新参数场景，最后用原 helper、`collect.py` 和 `collect-parameters.py` 独立核对完整行集、结构、冻结参数、持久事件和秘密审计。有已存在的执行批次回执时停止并要求核对，不直接重放固定数据。

```powershell
pwsh -NoProfile -File ./scripts/normal-import-e2e/api-test.ps1
```

2026-10-20 已实际完成正常 API 批次：72 条 API 契约、23 项整改、修复后 28 项参数场景、7 项修复/版本/拒绝补验与 2 项兼容版本边界通过。首轮参数 25 PASS/3 FAIL 已保留，50 条定义有通过真实场景，22 条仍受能力或 Oracle 证据限制；不声称全参数功能通过。当前结果以[验证报告](../../docs/03-technical/evidence/normal-import-full-parameter-validation-2026-10-20.md)为准。

`api-test.ps1 -Gates` 对固定 23 个受限配置执行只读真实预检：除账号/角色外还验证拒绝提交；不运行工具、不授予权限。`-Repair` 只接受已有首轮 25/3 失败状态，归档原回执并重置 owner 库内两个固定失败目标，执行显式 LF 和三个 MySQL 版本及拒绝反例，独立核对后合并三条新记录；已有修复回执时不得重放。`-Closeout` 只按本轮回执清理未提交草稿并复核服务。`collect-repair.py`、`build-final-report.py`、`collect-closeout.py` 分别固化修复、逐参数和精确清理证据，发布前执行秘密布尔审计。

本 run 的库和输入目录已经按所有权删除并确认不存在，旧日记仅供复核，不能重新提交固定场景或复用旧回执声称重读结果。新批次需先建立新的独立所有权夹具并调整固定批次路径。无凭据只读认证边界入口仍为 `node scripts/normal-import-e2e/api-runner.mjs --auth-check`；401 或健康 200 不能计入功能通过。

2026-10-19 本轮固定范围：现有 TEST / ODP / MySQL 数据源、Windows AMD64、OBLOADER 4.3.5、本地 CSV/CUT/SQL。结果见 [真实验证记录](../../docs/03-technical/evidence/normal-import-mysql-e2e-2026-10-19.md)。这些脚本不能作为生产导入或任意 SQL 入口。

- `fixture.py` 只接受固定动作，在随机独立库内写入 `odo_owner`；创建、扩展、检查、清理都由现有 `scripts/export-e2e/fixture` 短进程提供本轮授权的测试源连接。密码只经匿名 stdin 进入短生命周期进程，不输出到命令、环境或文件。复用已安装的 PyMySQL 1.2.3 测试依赖，不增加产品依赖。
- `browser.mjs` 的 `run(input)` 在已登录应用源中运行普通草稿、预检查、提交、执行、快照和日志 API；无路由拦截、假 Agent 或伪造检查。`input` 包含 `runId/database/inputRoot/sourceId/nodeId`，可用 `ids` 选择独立用例；真实五步页面创建的 CSV 任务通过 `uiTaskId` 读取。`window.__normalImportE2E` 只保存固定安全结果。
- `lifecycle.mjs` 的 `lifecycle(input)` 使用本轮 `successTaskId/failedTaskId/oldPrecheckId`，验证派生、模板、CSRF、乐观锁与重跑锁定；不提交派生任务，完成后删除本轮新建的派生草稿与模板。结果在 `window.__normalImportLifecycle`。
- `collect.py` 在夹具清理前核对目标完整行集和结构、SQLite 的固定任务/执行/事件字段及真实密码泄漏布尔审计。它不会读取身份、凭据表或输出原始工具日志。
- 失败整改复验使用新的独立 run，传入 `retest: true`；原未引用空白场景验证安全拒绝，并新增显式 trim、引用空白的结构加数据、strict=false 正常数据三个场景。原始失败记录保留，不能将阻断写成原文件无损导入。
- 全参数配置矩阵在 `internal/importdomain/testdata/normal-parameter-matrix.json`，由 `parameter_matrix_test.go` 逐项核对 72 条冻结定义、生成及执行白名单。`parameter-matrix-report.py` 只收集尚未执行的补验批次，本 run 有任务时拒绝生成准备报告，不能把本地通过升级为真实通过。
- `parameter_fixture.py` 为 28 项 MySQL 参数真实场景提供固定种子、完整行集和结构预期。通过既有 helper 的 `import-parameters-create` / `import-parameters-inspect` 进入，均须验证原 run 所有权；创建仅运行一次。`go run ./scripts/normal-import-e2e/validate-fixtures` 只读核对当前已准备文件到命令的本地链路，不连接数据库或启动工具。将本轮根目录的 `parameter-cases.json` 加入 `run(input)` 的 `parameterCases` 后，才会按普通业务 API 真正提交；高风险用例还验证缺少冻结指纹确认时拒绝。实际任务、日志、事件和独立内容核对完成之前，`CONTENT_PENDING` 不算通过。
- `collect.py <artifacts下的本轮目录名>` 支持独立证据目录；`parameter-coverage.py <结果JSON相对路径> <覆盖JSON相对路径>` 从实际冻结参数生成 72 项追踪。`HAS_REAL_SCENARIO` 仅指有通过场景，不代表该参数全取值、互斥、格式、权限和平台验证通过。

构建夹具入口：

```powershell
$env:CGO_ENABLED='0'
go build -trimpath -o artifacts/normal-import-e2e/fixture.exe ./scripts/export-e2e/fixture
node --check scripts/normal-import-e2e/browser.mjs
node --check scripts/normal-import-e2e/lifecycle.mjs
```

真实运行必须已有明确授权，只使用有效 TEST 数据源和登记允许目录。每轮生成新 `runId`（32 位小写十六进制）及 `odo_e2e_YYYYMMDD_<runId前12位>` 数据库，先保存忽略目录的所有权日记；不得复用已清理的本轮对象。夹具动作：`import-create` → `import-extend` → `import-inspect` → `import-cleanup`。当前固定文件目录前缀为 `nivt-normal-20261019-`，更换验证批次时应同步调整前缀和精确清理校验。

浏览器使用正常既有认证会话，由受支持的 CUA/CDP 开发接口加载本地脚本；不复制 cookie、密码或 CSRF 到外部。脚本中的 CSRF 只在当前应用源读取及使用。每次只有一个真实工具任务运行，超时取消本轮任务后停止批次。

`collect.py` 读取 `artifacts/normal-import-e2e/` 中的 `run.json`、三份浏览器批次回执、两份生命周期回执、`supplemental.json` 和 `content.json`，输出 `verified-results.json`。先保存独立内容核对及输入摘要，再按所有权清理本轮数据库，核实绝对路径位于允许根目录并且无重解析点后清理本轮输入/日志目录。清理后不得用旧结果声称重新读取了数据库或文件；复验须新建独立夹具。
