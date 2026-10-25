# 日志中心任务日志首批验收（2026-10-25）

结论：任务日志首批已完成页面、隔离功能验证和本机安装恢复。时间/精确任务/实际通道/脱敏关键字查询、固定快照分页、同来源上下文、脱敏复制及两种下载范围形成闭环。其他预设和完整日志中心不据此标记完成；不提升 CONS-05、G3/G4 或目标平台真实验证结论。

## 范围和职责

- [任务查询契约](../log-center-task-query-contract.md)明确 TASK_READ、AEAD 快照、水位、预算、复制/下载审计和失败关闭；[P1](../../../design-system/pages/log-center.md)沿用 Management / Table 与已定版数据源视觉，未改 Shell、全局 Token 或 Archetype。
- 领域 `internal/logquery` 管会话、预算和输出；SQLite 索引/授权/审计在 `internal/store/log_query.go`；受控正文沿用 `internal/logstream` 摘要验证和二次脱敏。HTTP 只处理传输，页面只装配，本域状态、抽屉和写事务进入 `workbench/log-query`。
- 0040 只新增不可重用批次顺序索引，旧日志正文/任务/历史迁移不改，清理外键级联。已提交水位固定后，晚到批次也不能插入旧页；游标绑定主体、权限、筛选及累计预算。
- 无可靠来源时间和原始级别时明确未取得；不从 stderr、stdout 或文本推断。GAP/TRUNCATED、已登记过期、未采集、无匹配和读取损坏分别处理，不宣称采集完整。

## 已通过的检查

| 检查 | 当前结果 |
| --- | --- |
| Go 完整回归 | `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` 通过；SQLite 包最终 354.045 秒；最后 HTTP 非法 URL 编码和注释收尾后，领域/HTTP/实际登录集成定向复验通过 |
| Go 静态检查 | 全范围 `go vet` 通过，最后涉及的 controlplane/logquery/integration/migrate 定向复验通过 |
| 新增领域/索引负例 | 授权先于正文、跨主体/不存在统一拒绝、运维范围、撤权指纹、固定水位/返回旧页、纳秒键、50000 条/64 MiB 扫描、100000 条下载拒绝、同源窗口、审计失败、损坏/过期及封存后读取通过 |
| 真实认证的隔离集成 | 临时 SQLite、合成密码、实际 Browser 会话和 CSRF：匿名拒绝、登录读取、CSRF 拒绝/下载、审计、同会话撤权通过；不接触现役口令 |
| 前端单测 | 69 文件 / 714 项通过，包含 API 严格投影、A → B → A、迟到 success/error/finally、抽屉关闭、复制/下载失效和失败重试 |
| 前端工程门禁 | TypeScript/构建、Lint、业务架构、平台架构、Token 检查通过；业务 0 violations / 21 REVIEW，平台 0 violations / 126 REVIEW / 7 runtime references；没有为本批豁免规则或扩大重构 |
| 浏览器最终验收 | 18/18：日志中心 14 项（1920/1440/1280、八种状态、两种下载、失败/撤权、原生 200%），三档四代表页 3 项，任务详情联动 1 项；重复轮次不累计 |
| 目视复核 | 查询、同源抽屉、下载确认、1280 和原生 200% 查询/上下文已查看截图；无整页横滚，表格局部横滚、键盘/ESC/回焦/分页可达 |
| 安全与差异 | `scripts/check-secrets.ps1`、`git diff --check` 通过；未提交、推送或创建 PR |

浏览器插件不可用，采用仓库 Playwright 和无 API 代理的隔离 Vite 15174、合成路由数据。最初运行被本机代理协议阻断，清除该测试进程代理后运行；没有改全局代理。首轮跨主机长暂停期间 Go store 测试被 11 分钟保护终止，不能算通过；最终完整重跑通过。页面刷新失败后的相对时间问题、表单提交绑定和测试夹具只读类型已修正并复验。

Vite 开发运行保留一次性 `ResizeObserver loop completed with undelivered notifications` 诊断，最终交互用例和页面错误断言通过；未用全局错误抑制或修改依赖遮蔽。构建仍有既有分包体积提示。上述诊断不描述为全平台无警告。

## 本机安装和数据保留

先核对无活动执行，经 SQLite Backup API 在忽略的 `var/backups/control-plane/pre-log-center-20261025-093225/` 创建一致快照；该目录仅当前 Windows 身份与 SYSTEM 可访问。数据库、身份材料及私有校验指纹没有进入仓库或公开证据目录。

通过原监视器按原参数分别正常停止、删除旧成品、`CGO_ENABLED=0 go build -trimpath` 并恢复：控制面仍为 `dev.ps1 -RealExecution`，Agent 仍为 `dev.ps1 -Agent -RealExecution`。控制面停止期没有运行旧进程；恢复后 TLS 健康 200。两者 PID 分别为 35356 / 11624，Agent 新二进制等于新 Windows 下载包；三个 Linux/Windows 包的构建不冒充目标系统原生验证。

| 安装后事实 | 核验结果 |
| --- | --- |
| SQLite | 40 项迁移 / 42 张 STRICT 表；每项源码校验和匹配；integrity `ok`，外键问题 0，查询水位索引与批次数匹配 |
| 业务与授权 | tasks 240、executions 240、agents 3、nodes 1、subjects 1、sources 10、drafts 544；原业务数量和审计 ID、Agent 逻辑凭据绑定、角色/范围、配置/CA/根密钥指纹全部保留 |
| 审计增量 | 5165 → 5167；仅恢复服务沿用既有环境检查持续授权产生的 `EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED` / `…COMPLETED`，未新增真实数据任务 |
| 运行开关 | 控制面与 Agent 原四个执行/探针开关均保留 true，未重置身份或恢复旧撤权 |
| 心跳与执行 | 同一 ACTIVE Agent 恢复心跳，采样年龄约 4 秒；活动执行 0，历史待核对仍为 1，未修改其结论 |
| 访问边界 | 健康 200、匿名日志 API 401、匿名页面 303 → `/login`、登录页 200；未重置或读取现役口令以绕过认证 |

安装的控制面 SHA256：`36d9940cc21fd7350b92ba7f5d5815c9b3591b3422dd4d7390325a022c86062a`；Agent SHA256：`2fbfa36a4e7ddb0c7c3f9edb0008ddaaa1e9e1056556e899b98e96ceff147509`。均核对 trimpath/CGO 关闭，控制面包含新任务日志入口。

## 证据与剩余边界

外部复核目录：`C:/Users/Dionysus/.codex/visualizations/2026/10/25/ob-data-orch-log-center/`。`go-test-final.log`、`go-targeted-final.log`、`go-vet*.log`、`web-test-final.log`、`web-build-final.log`、工程门禁日志及 `browser-verified/` 为最终证据；失败/调试轮次保留供复核。`runtime-before.json` / `runtime-after.json` 为脱敏安装事实，`runtime-audit-delta.json` 只记录新增动作类型。源文件和证据摘要由 `source-manifest.json` / `evidence-manifest.json` 固定。

本轮源清单 1072 项（排除验收证据以免自引用），SHA256 指纹 `afefe4806628a7514b910bdfddabfe08b410a462c1c994780d056e470d788b83`，清单同时绑定已安装控制面和 Agent 的成品摘要。

现役已登录日志查询/复制/下载未做人工验收，实际认证和权限只在隔离库完成闭环；不能将匿名探针、构建或截图外推为现役账号功能验收。没有提交新的真实导入/导出、数据库或存储连接测试。Agent 恢复时仅沿用已有环境检查持续授权。

节点/Agent、调度和审计查询尚未接入；原始日志、任意文件、未脱敏下载、全文检索、告警和 SQL/命令执行入口均未新增。任务日志首批提供精确任务 ID，不引入虚构任务名称候选；单任务持续加载仍使用任务详情原链路。后续按任务地图单独准入其他来源与独立权限，不以此首批宣称完整日志中心完成。
