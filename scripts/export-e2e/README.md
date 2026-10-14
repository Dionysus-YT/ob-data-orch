# MySQL 本地导出端到端用例

范围为用户授权的非生产 `test-mysql` / ODP / MySQL 数据源、Windows AMD64 节点及本地输出。矩阵覆盖普通 V1 的 CSV、CUT、SQL；POS、PARQUET、ORC、AVRO 只验证普通向导门控，不启动这些历史格式。对象存储不在本轮授权范围内。

`cases.mjs` 是 172 项用例输入与预期的唯一目录，`browser-batch.mjs` 通过真实控制面完成草稿、唯一命令生成器、七项预检查、提交、Agent 领取和终态。`collect-browser.mjs` 独立核对本机文件、清单及内容，`browser-lifecycle.mjs` 覆盖幂等、失效、过期、取消、派生、模板、日志及权限。五步界面、视口和键盘通过 CUA 实际操作。缺少外部条件必须记录 BLOCKED/PARTIAL，不能算 PASS；不拦截应用 API 或使用假 Agent。

本轮实际结果与产品缺陷以 `docs/03-technical/evidence/export-mysql-e2e-2026-10-14.md` 为准。用例覆盖不等于真实通过，旧的合成证据不能替代此记录。

## 固定夹具

夹具库名为 `odo_e2e_YYYYMMDD_<runId前12位>`，`odo_owner` 保存本次所有权标识。包括特殊值表、空表、无主键表、生成列表、1024 行拆分表、50000 行取消表、日期时间/零日期表、BLOB 表、一级/复合分区，以及可创建的视图、函数、过程和序列。分区预期行集由固定查询读取实际夹具；不读取业务表。

`fixture/main.go` 只读已有 SQLite 密文和受保护根密钥，在短进程内解密该 TEST 数据源，通过匿名 stdin 交给固定 Python 夹具程序。不通过 argv、环境变量、文件或输出传递密码，不修改产品 JDBC 探针。依赖 PyMySQL 1.2.3（MIT、纯 Python）仅安装到忽略的 `artifacts/e2e-mysql-deps`；无需增加产品依赖或跨平台运行时要求。

```powershell
$env:CGO_ENABLED='0'
go build -trimpath -o artifacts/mysql-export-e2e/fixture.exe ./scripts/export-e2e/fixture
python -m pip install --target artifacts/e2e-mysql-deps -r scripts/export-e2e/requirements-fixtures.txt
node --test scripts/export-e2e/cases.test.mjs
```

构建后可运行 `fixture.exe -action catalog` 读取有效参数目录。真实 `create/augment/inspect/cleanup` 必须使用已授权部署的数据目录、独立库名与同一 runId；不得对生产源或已有数据库试运行。`create` 不使用 IF NOT EXISTS，`cleanup` 在精确核对所有权后才 DROP 本次库。

## 浏览器与执行

使用用户指定的内嵌浏览器。登录由正常页面和已有保存凭据完成，不读取密码或复制 cookie/CSRF。由 CUA 的开发者 CDP 能力在当前已登录应用源内加载 `browserBatch(input)` 或 `browserLifecycle(input)`；各请求在浏览器内读取当前 CSRF，保持正常权限和幂等检查。不得扩展到其他站点、凭据管理或任意 SQL 通道。运行期间不要导航或重载承载批次的页面；界面测试使用同浏览器的独立临时标签。

批次准备读取忽略目录中的 `run.json`（独立库名和所有权 runId）、`inspection.json`（授权源/节点引用）、固定夹具期待及 `fixture-current.json`。`prepare-browser-batch.mjs <options.json>` 的选项为 `cases` ID 列表和唯一 `suffix`；每次重新运行都使用新后缀，并在下一次准备前保存对应 `browser-contexts.json`。结果保存 `records` 后，以结果/上下文文件对运行 `collect-browser.mjs`。不要覆盖所有权日记，不跳过 TLS 警告，不保存登录 trace、storageState 或密码。

`prepare-command-coverage.mjs` 与 `browser-command-coverage.mjs` 核对实际脱敏命令参数及忽略输出目录后的业务命令摘要；`parameter-matrix.mjs` 生成 79 个目录定义的范围追踪。`audit-api-evidence.mjs` 把公共响应经匿名 stdin 交给固定 Go 辅助程序，只保存秘密核对布尔结果，未知脱敏状态的响应不落盘。`browser-evidence.mjs` 仅读取本轮任务的快照、脱敏命令与持久日志，不读取身份或凭据接口。

每次只运行一个真实导出。每个场景使用已登记允许根目录下的本次专用子目录和独立日志目录。`MX-001` 专门验证未指定日志目录的默认行为，其余场景隔离日志，避免一个默认配置缺陷阻断所有参数验证。输出目录的 Windows 原生路径只用于本机核对；请求按现役 `/E:/...` 形式填写。

SQL 非 SELECT、多语句、文件引用等负例止于草稿 API；即使服务端意外接受也不能继续预检查或提交。结果检查保留引用事实，区分 NULL 标记、字面字符串和空串，比较完整行集及重复行，核对真实清单/字节数，并解压后检查内容。不能只因退出码为 0 或文件非空就判成功。

`--max-file-size` 按已确认契约验证进程总量上限与完整的源行子集，不误作分片；`--block-size` 独立验证文件拆分与全行集。`MX-002` 比较两个独立真实任务的业务命令摘要和完整内容摘要。未通过的整库任务不能只凭其中一张表或输出字节数宣称整库完成。

## 生命周期与人工条件

- 过期预检等待真实 TTL，不能改 SQLite 时间或复用伪造报告。
- 检查点继续需要同一真实失败任务的 `dump.ckpt`；没有真实检查点时保持 BLOCKED，禁止复制或伪造文件。
- Agent 活跃租约故障测试需要真实进程重新附着入口和独占任务窗口，保留身份/TLS/状态目录并记录重启前后和重复投递证据；当前 Worker 缺少该入口，保持 BLOCKED，闲置重启不替代此项。
- 跨主体越权需要第二个已授权测试身份；管理员的随机 404、缺失 CSRF 和未登录检查不能替代它。
- 本轮源码中的任务日志下载尚未实现；REST 分页不可冒充实时流或下载通过。

输出 PASS、FAIL、BLOCKED、PARTIAL、OBSERVED 分开统计。失败不能降低断言，脚本错误修复后须使用新场景目录复验，保留旧回执作为准备过程证据。测试结束后取消本次残留任务、删除本次模板、核对 owner 后清理夹具库及精确所属的输出文件；保留脱敏结果和必要文件摘要，不保留真实秘密或原始工具输出到 Git。

内容核对必须在文件清理前完成并保留 verified-results.json。`report.mjs` 只投影固定安全字段，读取实际清理与运行证明，生成 Markdown 和结果 JSON；清理后不能对旧输出再次运行 collect-browser，重新验证需新夹具、所有权日记和输出目录。
