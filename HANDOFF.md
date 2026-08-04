# OB Data Orch 交接说明

> 快照时间：2026-07-28 15:22 +08:00
> 续接更新：2026-07-28；已完成 Windows 本机回环 TLS 的实际 Agent 注册、持续心跳、`G2_SYNTHETIC` 节点侧连接测试，以及当次明确授权的固定 `AGENT_JDBC` 实际连接成功验证
> Git 基线：`main` / `46f0165`
> 交接范围：当前未提交工作区的 G2 数据源、执行节点、受认证 Agent、固定预检查与基础连接测试工作。
> 安全说明：本文不包含任何真实端点、连接表达、用户名、密码、密钥、输出目录或联系人信息。

## 一句话结论

当前已完成的是 **G2 本机联调能力与一次受控真实连接成功**：浏览器可以登记执行节点并签发一次性注册码；Windows Agent 包固定使用回环 TLS、粘贴注册码后完成实际关联并持续发送心跳。控制面可以冻结绑定、签发租约、保留回执并回写 `G2_SYNTHETIC` 安全结果；实际常驻 Agent 已完成一条从排队到成功的回写。在当次明确授权和双端显式开关下，受认证 Agent 领取并解析唯一短时数据库槽位，完成 TCP 建连和固定 Java JDBC 基础连接探针，终态为 `SUCCEEDED / DATABASE_CONNECTED / AGENT_JDBC`，安全投影为 `realConnectionVerified=true`。此前一次 `DATABASE_TCP_UNREACHABLE` 负例仅作为瞬时失败记录；没有足够证据将其归因于路由、防火墙或监听服务。本次成功不自动启用数据源或节点，也不证明对象权限、环境检查、预检查或导出可行性；这些仍受 G3/WI-06 硬门禁阻断。

## 当前阶段

| 事实面 | 状态 | 结论 |
| --- | --- | --- |
| 代码 | `changed-and-verified` | 当前工作区含数据源、节点、Agent 协议、迁移、OpenAPI 与前端的累计未提交实现。 |
| 运行态 | `local-verified` | 回环 TLS 控制面与常驻 Windows Agent 已完成本机关联、持续心跳、合成连接测试及一次固定 JDBC 实际连接成功；不构成现场 G3 服务、节点启用、预检查通过或真实导出验证。 |
| 文档 | `pending` | 大部分相关契约和门禁文档已更新；任务地图仍有两处需要按本快照复核和同步，见“文档待办”。 |
| 规则 | `verified-current` | 根目录 `AGENTS.md` 已核对；真实连接、凭据和工具启动必须继续失败关闭。 |
| 记忆 | `not-applicable` | 本次未写入外部或平台长期记忆。 |
| 工作区 | `pending` | 工作区很脏，必须保留现场并按逻辑单元审查；未执行删除、重置或清场。 |

## 已实现：执行节点两步注册界面

- `/nodes/new` 只登记节点名称、目标平台和允许根目录；保存后直接进入“注册步骤 2 / 2”，不会再把 Agent 关联入口隐藏在普通详情操作中。
- 关联步骤默认只呈现“生成注册码 → 在目标机器启动 Agent → 粘贴注册码”。控制面 HTTPS 地址、CA 证书绝对路径、无秘密注册指令与关联标识位于按需展开的高级部署信息；注册码关闭后立即从页面状态清除。
- 节点 IP 和主机名不是浏览器可写配置。控制面不会通过 IP 主动连接节点；Agent 必须从目标机器主动连接控制面。本机首版尚未持久化地址或主机名，跨机器部署前需先确认多网卡、代理与隐私边界后才可增加只读定位事实。
- Local MVP 现固定为回环 TLS（`https://127.0.0.1:8080`），要求显式证书和私钥。Windows 包内固定该地址及同目录 CA，用户首次只需运行 `agent.exe -register`、粘贴注册码并按 Enter；进程随后保持心跳，以后重启只运行 `agent.exe`。无需填写节点 IP、地址、CA 路径或节点标识。

## 已实现：G2 基础连接测试闭环

### 产品与控制面

- 数据源编辑页要求用户每次明确选择一个候选执行节点；页面展示排队、终态、来源、证据码以及“是否为实际节点侧连接验证”。
- 控制面只做授权、冻结数据源配置/凭据 revision/节点/Agent/节点事实版本、短租约、审计和结果投影；它不直接连接数据库、启动 Java 或读取密码。
- Local MVP 已将 `ConnectionTests` 显式装配为 SQLite 存储依赖，因此“当前环境尚未配置节点侧连接测试”不再出现在该本机入口；未关联或离线节点仍以 `AGENT_CONNECTION_TEST_NODE_UNAVAILABLE` 失败关闭。
- Local MVP 重启只撤销旧版未标记的合成遗留测试事实，并为每次撤销写入独立审计请求标识；它不会清除 `AGENT_JDBC` 结果。该幂等修复避免已有审计记录阻断控制面再次启动。
- 心跳采样时间仅用于展示，不再单独递增节点 `factsRevision`；操作系统、架构、Agent 版本、容量或资源事实变化仍会使旧测试绑定失效，避免纯时间变化在 Agent 领取前导致“基础连接测试租约无效”。
- 新迁移 `0009_add_data_source_connection_test_runs.sql` 增加连接测试运行记录、Agent 请求回执和秘密槽位解析回执。它们均保存绑定与幂等事实，不保存秘密原值。
- 浏览器读取与 Agent 协议已写入 `contracts/openapi.json`，包括 `claim-next`、`acknowledge-lease`、`secret-slots:resolve` 和 `complete` 四个固定动作；未知字段、任意 SQL、命令、路径和任意连接参数均不在协议范围内。

### Agent 连接测试 Worker

- 默认 `internal/agentconnectiontest/worker.go` 只接受 `G2_SYNTHETIC` 租约，并按 `claim-next -> acknowledge -> complete` 处理一条测试。
- 只有 Agent 本地显式启用 `OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=true`、运行时已核验且控制面也明确签发 `AGENT_JDBC` 时，Worker 才解析当前租约唯一短时槽位并调用固定路径；其他来源或缺少运行器均失败关闭。
- `AGENT_JDBC` 先以 5 秒超时建立冻结主机/端口的 TCP 连接；不能解析、被拒绝、超时或其他不可达分别回写有限证据码，且不启动 Java。TCP 成功后才在私有工作区调用固定 JDBC 探针；地址、用户名、密码、异常文本和工作区内容均不回传或落盘。
- 默认 Agent 启动入口已在受认证心跳成功后最多运行一条该 Worker；默认不开启 JDBC 运行器，仍只推进 `G2_SYNTHETIC` 协议状态。

### 启用和导出准入

- 数据源只有在 `last_test_status = SUCCEEDED` 且 `last_test_source = AGENT_JDBC` 时才可启用。
- 创建导出草稿、预览后续、创建/继续预检查同样复验该双条件；`G2_SYNTHETIC` 成功不会放行。
- Local MVP 对没有受认证 Agent 的连接测试安全失败，不伪造成功，也不能形成可用于导出的数据源。

## 明确未完成和阻断

| 项目 | 当前状态 | 为什么不能继续声称完成 |
| --- | --- | --- |
| 跨机器 Agent 实际注册/关联 | 本机完成、跨机器阻断 | 完整节点仓储、浏览器身份、授权、密钥与 Agent 协议已在回环 TLS Local MVP 中组装，并通过实际 Windows `agent.exe` 验证关联和首次心跳。该包固定 `127.0.0.1`，不能跨机器使用；不能通过填写节点 IP、关闭 TLS 校验或暴露 Local MVP 绕过。 |
| 执行节点测试与启用 | Windows 本机闭环通过 | 当前常驻 Windows Agent 已在回环 TLS 下实际回传固定 `TOOL_RUNTIME_READY` 环境检查；控制面复验当前心跳、平台、容量与同一 `factsRevision` 后，节点已变为 `ENABLED / ONLINE / NORMAL / AVAILABLE / acceptsNewTasks=true`。该结论不包含数据库对象、输出路径或空间预检查，也不授权导出。 |
| `AGENT_JDBC` 基础连接测试 | 已完成一次受控成功，后续准入仍阻断 | 私有 ODP JDBC 身份已按官方 V4.3.5 连接方式和 DEC-013 收敛为控制面短时组装 `username@tenant#cluster`；仅在控制面和 Agent 双端显式开关以及当次授权下签发 `AGENT_JDBC`。2026-07-28 实际常驻 Agent 已返回 `SUCCEEDED / DATABASE_CONNECTED`，证明该节点在该时点完成固定 JDBC 基础连接；它不替代对象、工具、路径、空间、节点启用或导出验证。 |
| WI-06 固定节点预检查 | 仅 G2 合成闭环 | 固定 `EXPORT_PREFLIGHT` 的租约、回执、槽位边界和 Worker 合成验证已完成；默认 Agent 未组装 Worker，未产生节点到数据库、对象、路径或空间的真实事实。 |
| G3 Windows 真实集成 | 阻断 | 外部前置材料的脱敏确认不替代正式 Agent、槽位、JDBC、工具、脱敏、恢复和结果证据。 |
| F2 后续导出工作 | 被准入条件阻断 | 现有数据源无法因 G2 结果启用，因此不能把草稿、预检查或命令页面视为可对真实数据库工作的闭环。 |
| G4 与生产发布 | 阻断 | 三个麒麟目标、认证、恢复、运维与发布门禁均未满足。 |

真实执行继续遵循 [开发任务地图](docs/03-technical/development-task-map.md)、[开发准入收口](docs/03-technical/development-readiness-closure.md) 和 [G3 Windows 准入准备](docs/03-technical/g3-windows-entry-readiness.md)。任何一次真实连接前都需要在批准范围内保留脱敏证据；不能通过本地开关、前端状态或手工请求绕过。

## 建议续接顺序

1. 先审查并拆分当前未提交工作区，确认 G2 合成实现、契约、迁移和前端变更的归属；不要使用 `git reset --hard`、覆盖或批量删除。
2. 已完成：重新运行前端 `npm run test`、`npm run typecheck`、`npm run lint`、`npm run build`；回环 TLS 已走通“创建节点 → 签发一次性注册码 → 实际 `agent.exe -register -once` → 关联与首次心跳”。Local MVP 没有受认证 Agent 时的数据源连接测试失败关闭负例仍需按原计划保留。
3. 已完成：私有 ODP 的 JDBC 身份按分字段登记、控制面短时组装 `username@tenant#cluster`；非法/超长输入失败关闭，完整身份不持久化、不写日志，Agent 不自行拼接。
4. 已完成：受认证 Agent 的固定连接测试 Worker 已接入成功心跳后的串行循环；心跳失败不领取，长期模式隔离 Worker 错误，`--once` 明确返回错误。尚无可靠恢复队列，也未接入秘密槽位或 JDBC。
5. 已完成：基础连接诊断允许符合当前机器事实的 `DISABLED`/`ENABLED` 节点；维护、归档、离线、平台不匹配或容量占满节点不进入候选，维护/归档状态也会使现有租约失败关闭。导出候选仍只接受 `ENABLED`。
6. 已完成：本机 Agent 到已登记端点的受控固定 JDBC 基础连接成功，并已完成“固定工具运行时检查 → 节点启用”闭环。下一步仍是最终选定节点的 `EXPORT_PREFLIGHT`；心跳、JDBC 成功或该节点运行时检查均不得据此启动 OBDUMPER 或导出。
7. 只有 `AGENT_JDBC` 成功事实被控制面复验并持久化后，才评估数据源启用与 F2 的下一步；导出前仍必须由最终选定节点执行固定 `EXPORT_PREFLIGHT`。

## 验证记录

以下是本次交接前已经完成的验证记录；不要把它们扩展解释为真实环境验证。

| 验证 | 结果 |
| --- | --- |
| 连接测试相关 Go 包：`internal/store`、`internal/controlplane`、`internal/agentwire`、`internal/agentconnectiontest`、`contracts`、`internal/localmvp` | 通过 |
| 其余 Go 分组：`cmd`、迁移、`agentjdbc`、`agentpreflight`、`agentlocalpreflight`、`agentworker`、`agentexec`、`agentstate`、`commandgen`、`config`、`credential`、`identifier` 等 | 通过 |
| `go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...` | 通过 |
| `./scripts/check-secrets.ps1` | 通过 |
| `git diff --check` | 无 diff 错误；保留既有 LF/CRLF 转换警告 |
| 2026-07-28 续接验证：`go test` 覆盖 `internal/store`、`internal/controlplane`、`internal/agentwire`、`internal/agentjdbc`、`internal/agentconnectiontest`、`internal/agentworker`、`contracts` | 通过；验证两条槽位均组装 `username@tenant#cluster`，并覆盖缺字段、分隔符、空白/控制字符、无效 UTF-8、超长输入和销毁清零 |
| 2026-07-28 续接验证：上述包 `go vet`、秘密扫描、`git diff --check` | 通过；`git diff --check` 仅保留工作区既有 LF/CRLF 提示 |
| 2026-07-28 Agent 循环续接：`go test` 覆盖 `cmd/agent`、`agentconnectiontest`、`agentwire`、`controlplane`、`store`、`integration`、`contracts`、`migrations` | 通过；覆盖心跳前置、每轮最多一条 G2 测试、心跳失败不领取、长期模式错误隔离、`--once` 错误返回及 TLS 协议信封 |
| 2026-07-28 Agent 循环续接：相关包 `go vet`、秘密扫描、`git diff --check` | 通过；`git diff --check` 仅保留工作区既有 LF/CRLF 提示 |
| 2026-07-28 诊断节点资格续接：`go test` 覆盖 `internal/store`、`internal/controlplane`、`cmd/agent`、`agentconnectiontest`、`agentwire`、`contracts` | 通过；覆盖禁用节点可诊断、维护节点拒绝、维护漂移使租约失效，以及导出候选仍保持启用状态门禁 |
| 2026-07-28 诊断节点资格收口：跨包测试与 `go vet` 覆盖上述包及 `internal/integration`、`migrations`，并运行秘密扫描和 `git diff --check` | 通过；首次冻结额外覆盖心跳时效、容量和平台匹配，差异检查仅保留工作区既有 LF/CRLF 提示 |
| 前端 `test`、`typecheck`、`lint`、`build` | 2026-07-28 重新执行通过；9 个测试文件、53 个测试通过 |
| 执行节点注册本机联调 | 回环 TLS 下创建 Windows 节点、签发一次性注册码并运行实际 `agent.exe -register -once`；节点安全投影为 `ASSOCIATED / ONLINE`，含 Windows/AMD64、容量与启动摘要。注册码未打印或写入文件；未连接真实 ODP、未解析真实凭据、未运行 Java/OBDUMPER。 |
| 常驻 Agent 注册本机联调 | 回环 TLS 下运行实际 `agent.exe -register`、粘贴后按 Enter；确认进程持续运行期间节点为 `ASSOCIATED / ONLINE`。验证使用的常驻测试目录保存本机身份状态；干净交付目录未保留已注册身份状态。 |
| 节点侧连接测试本机联调 | 回环 TLS 下由实际常驻 Windows `agent.exe` 领取一条仅含合成地址/凭据的数据源测试；状态由 `PENDING` 变为 `SUCCEEDED / SYNTHETIC_OK / G2_SYNTHETIC`，且 `realConnectionVerified=false`。未解析秘密、未连接真实 ODP、未启动 Java/OBDUMPER。 |
| 受控真实连接验证 | 2026-07-28，当次明确授权下，实际常驻 Windows Agent 领取 `AGENT_JDBC` 租约并对已登记目标完成固定 TCP 与 Java JDBC 基础连接探针；终态为 `SUCCEEDED / DATABASE_CONNECTED / AGENT_JDBC`，`realConnectionVerified=true`。未记录端点、用户名、密码或异常原文；该成功不代表对象、环境、任务或导出可行性。 |
| 汇总 `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` | 运行约 124 秒后超时，无失败输出；不能记录为整包通过，已以分组回归替代 |

## 文档待办

- 已同步：`docs/03-technical/development-task-map.md` 的 F1.2 和 F2 现明确要求 `SUCCEEDED + AGENT_JDBC`，`G2_SYNTHETIC` 不能启用数据源或放行导出。
- 已同步：F1.4 已将独立 `DATA_SOURCE_CONNECTION_TEST` 的 G2 合成闭环、默认 Agent Worker 运行循环、禁用节点诊断资格，以及回环 TLS 的实际 Agent 注册和首个心跳标记完成；下一项限定为固定环境检查与节点启用，之后才评估门禁满足后的 G3/WI-06 受控验证。
- 文档更新必须保持“G2 合成验证”“真实节点连接验证”“真实导出可用”三种状态分离。

## 工作区与残留

- 创建本文件前，工作区有 46 个已跟踪修改文件和 89 个未跟踪文件；其中既有前端、控制面、Agent、契约、迁移、测试与文档改动应作为同一开发现场审查。
- 未跟踪项包含本地构建二进制 `agent`、`agent.exe`、`control-plane`、`control-plane.exe`，以及 `.playwright-cli/` 页面快照。这些不是已确认的源码交付物；本次没有删除、移动或提交它们。
- `tmp/`、`var/`、工具归档和 `.codegraph/` 是本机/忽略工件边界。CodeGraph 当前可用但索引已落后工作区（检查时报告新增 2、修改 5）；本次按源码、契约和测试回退核验，未经授权不要重建、同步或清理索引。
- 本次验证期间运行回环控制面、常驻测试 Agent 与 Vite；除一次获授权的固定 JDBC 基础连接成功外，其余测试使用合成输入。不得把 Local MVP 当作 G3 环境；交接后应按实际进程状态复核，不应把这一记录当作常驻部署声明。

## 关键入口

- `internal/store/connection.go`：连接测试冻结、租约、绑定复验、终态和启用/导出资格关联。
- `internal/controlplane/server.go`：浏览器数据源连接测试、Agent 四个固定路由和安全响应投影。
- `internal/agentwire/connection.go`：受保护 Agent 身份上的严格 HTTPS 客户端与固定槽位协议。
- `internal/agentconnectiontest/worker.go`：默认 G2 合成、显式受控 `AGENT_JDBC` 结果的串行 Worker。
- `migrations/0009_add_data_source_connection_test_runs.sql`：连接测试及 Agent 回执持久化边界。
- `web/src/views/DataSourceFormView.vue`：节点选择、轮询、来源/证据显示和字段级错误反馈。
- `web/src/views/ExecutionNodeFormView.vue`：执行节点声明配置与保存后进入 Agent 关联步骤。
- `web/src/views/ExecutionNodeDetailView.vue`：节点事实、两步注册说明与一次性关联材料内存展示。
- `web/src/views/executionNodeEnrollmentInstructions.ts`：按目标平台生成不含秘密的 Agent 关联命令和 CA 路径示例。

## 交接底线

不要把 `G2_SYNTHETIC`、心跳、关联、运行时校验或 OpenAPI 端点称为真实数据库成功连通性。控制面不得直接测试数据库；2026-07-28 的 `AGENT_JDBC` 终态证明受认证本机 Agent 在登记节点完成了固定 JDBC 基础连接。节点的 `TOOL_RUNTIME_READY` 只证明固定 Java、Connector/J 与 OBDUMPER 启动布局，并已用于本机节点启用；两类事实均不能替代最终选定节点的完整 G3/WI-06 证据、对象/路径/空间预检查或导出。
