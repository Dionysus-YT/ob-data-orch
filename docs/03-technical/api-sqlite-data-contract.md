# 首条切片 API 与 SQLite 数据模型最小契约

> 文档状态：首条纵向切片专项契约已确认，AD-R01～AD-R20 已确认
> 适用范围：直连 OBServer 单表 CSV 导出首条切片
> 关联基线：TD-001～TD-008、TS-R01～TS-R14、PC-R01～PC-R15、AS-R01～AS-R16、CS-R01～CS-R18、TL-R01～TL-R18、LG-R01～LG-R20
> 实现状态：20 表迁移与 SQLite 核心仓储已通过本地合成测试；API、凭据、Agent 状态投影和日志文件恢复仍待后续组件
> 更新日期：2026-07-21

## 1. 目标与范围

本契约把已经确认的领域和协议语义落到两层实现边界：

1. 浏览器、控制面与 Agent 之间有哪些最小 API；
2. 哪些事实必须进入 SQLite，哪些只进入分段文件或版本化发布资源；
3. 幂等、并发、不可变快照、租约、事件和审计由什么约束保证；
4. 数据库如何初始化、迁移、备份和跨平台验证。

首条切片只覆盖：

```text
已认证用户
→ 数据源新增/编辑/测试/启停
→ 单表 CSV 导出草稿
→ 所选 Agent 任务预检查
→ 脱敏命令预览
→ 幂等提交不可变任务
→ Agent 领取并执行
→ 状态、结果和脱敏日志查看/下载
```

本轮不设计普通导入、旁路导入、模板、定时调度、取消/重试、全量系统设置 API，也不创建 ORM 实体、GraphQL、通用工作流、通用远程命令或微服务间接口。

## 2. 本轮发现的既有契约缺口

产品基线要求“提交前”完成数据库对象、执行节点、工具环境和输出路径预检查；CS-R09 又要求由所选 Agent 验证节点到数据库的真实连接。现有 AS 契约只定义已提交任务的 `ClaimExecution`，无法承载提交前的远端检查。

本契约提出一个受控补充：

- 新增固定能力 `EXPORT_PREFLIGHT`，只执行登记的连接、对象、工具、路径和空间检查；
- 预检查绑定草稿 revision、规范配置指纹、数据源 credential revision、nodeId、Agent 和短租约；
- Agent 通过独立 `ClaimPrecheck` 主动领取，不接受 Shell、任意 SQL、任意路径浏览或工具命令；
- 预检查结果有有效期，草稿、权限、数据源、凭据、节点或工具事实变化后立即失效；
- 提交事务重新核对全部绑定值，不能把过期预检查升级为任务；
- 正式执行仍使用既有 `ClaimExecution`，预检查不创建 TaskExecution，也不占用任务 eventSeq。

AD-R09 已确认，本轮同步修订 AS/CS 契约。实现不得自行选择“控制面代检”或“先提交再检查”。

## 3. API 总体约定

### 3.1 接口域

| 接口域 | 前缀 | 身份 | 用途 |
|---|---|---|---|
| 浏览器业务 API | `/api/v1` | 已认证用户会话 | 页面查询和用户操作 |
| Agent 协议 API | `/agent/v1` | Agent 机器凭据 | 心跳、预检查、执行、事件、日志和秘密槽位 |

两个接口域使用不同认证中间件、请求模型、限流和审计策略。浏览器会话不能调用 Agent API；机器凭据不能调用业务 API。

### 3.2 数据格式

- HTTPS + UTF-8 JSON；下载和 SSE 除外；
- JSON 字段使用 `camelCase`，枚举使用稳定大写标识；
- 外部 ID 使用服务端生成的随机 UUID，不暴露 SQLite rowid；
- 时间统一输出 RFC 3339 UTC，页面再按平台时区展示；
- 金额以外的计数使用 JSON 整数；可能超过 JavaScript 安全整数的字节数、行数在 API 中使用十进制字符串；
- 空值、缺失和空集合语义分开，更新接口不把“字段缺失”解释为空字符串；
- 浏览器 JSON 默认最大 1 MiB；Agent 事件/日志继续服从 AS/LG 批次上限；
- 浏览器写入遇到未知字段直接返回校验错误；Agent 关键字段按协议版本拒绝，只有明确登记为非关键的扩展字段才可忽略。

### 3.3 响应和追踪

成功响应至少携带 `requestId`；创建资源返回 `201` 和资源 ID，异步预检查返回 `202`。列表使用不透明游标，不返回无权对象的总数。

涉及数据源详情、命令、任务、日志、凭据状态或 Agent 事实的响应使用 `Cache-Control: no-store`。服务端日志只记录 requestId、路由模板、主体摘要、结果码和耗时，不记录请求/响应正文。

OpenAPI 3 文档是 API 结构契约来源；前后端类型从同一契约生成或校验，但 OpenAPI 不替代服务端业务、权限和状态测试。

## 4. 认证与授权边界

V1.0 不建设用户名密码库、组织、用户组、自定义角色或 IAM 平台。控制面只接收认证系统已经确认的稳定 `subjectId`，保存必要身份投影和授权绑定：

- 会话建立方式和首次管理员引导属于独立部署安全阻断项；本契约不默认信任任意 HTTP 用户头；
- Cookie 会话必须使用 Secure、HttpOnly、SameSite，并对写操作校验 CSRF；Token 方案不得把 Token 放入 URL；
- 每个 API 同时校验身份状态、固定角色/独立能力和对象范围；
- 任务本人访问是固定规则，数据源使用、数据源管理、节点使用和任务运维范围分开；
- 无权对象默认返回统一 `404`，避免用 `403`、数量或耗时帮助枚举；
- 权限加载失败时失败关闭，不使用前端缓存或空范围猜测放行。

## 5. 浏览器业务 API

### 5.1 会话、节点候选与最小关联

| 方法与路径 | 用途 | 关键规则 |
|---|---|---|
| `GET /api/v1/session` | 当前身份和有效能力摘要 | 不返回认证凭据；范围只给必要摘要 |
| `GET /api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT` | 选择节点候选 | 只返回有权、启用且当前可判断的节点；候选不等于提交保证 |
| `POST /api/v1/execution-nodes` | 创建首条切片节点记录 | 仅节点管理员及对象范围；需要幂等键；不安装工具或远程连接节点 |
| `GET /api/v1/execution-nodes/{nodeId}` | 节点/Agent 当前事实 | 管理状态、在线、环境、容量四维分开；不返回机器凭据 |
| `PATCH /api/v1/execution-nodes/{nodeId}` | 编辑名称、状态和受控根目录 | 需要 If-Match；运行事实不可由页面覆盖 |
| `POST /api/v1/execution-nodes/{nodeId}:create-enrollment` | 生成一次性关联材料 | 原值只在 no-store 响应显示一次；SQLite 只存摘要、过期和消费状态 |

这些节点接口只满足首个 Agent 的受控关联和任务选择，不提前实现节点组、工具远程安装、SSH、批量操作或自动调度。

### 5.2 数据源

| 方法与路径 | 用途 | 关键规则 |
|---|---|---|
| `GET /api/v1/data-sources` | 授权列表 | 游标分页；筛选和数量与对象权限一致 |
| `POST /api/v1/data-sources` | 新增 | 需要 `Idempotency-Key`；密码为仅写字段；业务与审计原子提交 |
| `GET /api/v1/data-sources/{dataSourceId}` | 详情 | 不返回密码、密文、nonce、长度或固定密码占位符 |
| `PATCH /api/v1/data-sources/{dataSourceId}` | 编辑/轮换密码 | 需要 `If-Match`；密码缺失表示不变，非空表示新 revision |
| `POST /api/v1/data-sources/{dataSourceId}:test-connection` | 基础连接测试 | 同步、有上限；不在 DB 事务中进行网络连接；结果标明控制面位置 |
| `POST /api/v1/data-sources/{dataSourceId}:disable` | 禁用 | 幂等状态操作；阻断新任务，不伪装取消运行任务 |
| `POST /api/v1/data-sources/{dataSourceId}:enable` | 启用 | 不自动恢复旧连接测试或预检查 |
| `DELETE /api/v1/data-sources/{dataSourceId}` | 删除或归档 | 无历史引用才物理删除，否则归档；响应明确实际结果 |

数据源响应只提供 `credentialStatus`、当前 revision 和最近更新时间。连接测试返回网络、认证、基础连接、直接可得信息和脱敏错误分类，不返回导入/导出权限、对象诊断或性能结论。

### 5.3 导出草稿、预检查和提交

| 方法与路径 | 用途 | 关键规则 |
|---|---|---|
| `POST /api/v1/export-drafts` | 新建草稿 | 需要 `Idempotency-Key`；只接受首条切片字段 |
| `GET /api/v1/export-drafts/{draftId}` | 读取草稿 | 返回 revision、活动/阻断字段和失效原因 |
| `PATCH /api/v1/export-drafts/{draftId}` | 更新草稿 | 必须 `If-Match`；只保存草稿值，不生成提交事实 |
| `POST /api/v1/export-drafts/{draftId}:precheck` | 发起 Agent 预检查 | `202`；绑定当前 revision/fingerprint/node/credential；需要幂等键 |
| `GET /api/v1/prechecks/{precheckId}` | 查询预检查 | 返回分域结果、执行位置、时间、完整性和过期状态 |
| `POST /api/v1/export-drafts/{draftId}:preview-command` | 重算脱敏命令 | 不解析秘密；返回有序脱敏令牌、指纹、阻断和证据版本 |
| `POST /api/v1/export-drafts/{draftId}:submit` | 提交任务 | 需要 `If-Match` 和 `Idempotency-Key`；重算并原子冻结任务/审计 |

预检查采用轮询，不为短时操作新增 WebSocket/SSE。相同配置指纹可以创建不同任务；指纹用于证明配置相同，不是业务唯一键。只有同一幂等键和相同请求摘要才返回原任务。

### 5.4 任务、执行和日志

| 方法与路径 | 用途 | 关键规则 |
|---|---|---|
| `GET /api/v1/tasks` | 授权任务列表 | 本人或授权范围；游标分页，不泄露全局总数 |
| `GET /api/v1/tasks/{taskId}` | 任务概要 | 当前状态是执行事实投影，不覆盖提交快照 |
| `GET /api/v1/tasks/{taskId}/snapshot` | 不可变配置快照 | 仅必要非敏感数据和 credential revision 引用摘要 |
| `GET /api/v1/tasks/{taskId}/command-evidence` | 计划/实际命令证据 | 始终脱敏；不调用秘密解析 |
| `GET /api/v1/tasks/{taskId}/execution` | 执行、进程和结果事实 | 不把未知证据伪装成成功/失败 |
| `GET /api/v1/tasks/{taskId}/logs` | 日志快照查询 | 复用 LG 不透明游标、来源和完整性语义 |
| `GET /api/v1/tasks/{taskId}/logs/stream` | 单活动任务 SSE | 只发送已双层脱敏且已持久化记录 |
| `POST /api/v1/tasks/{taskId}/logs:download` | 同步脱敏下载 | 复用筛选和权限；服从 100,000 条/100 MiB 上限并审计 |

首条切片没有取消、重试、重新执行、检查点继续和任务删除 API。

## 6. Agent 协议 API

所有请求使用机器凭据认证和 AS 通用信封。路径中的 ID 不能替代信封内 nodeId、agentId、bootId、requestId、租约和摘要交叉校验。

### 6.1 既有执行协议落点

| 方法与路径 | 语义 |
|---|---|
| `POST /agent/v1/enrollments:exchange` | `EnrollAgent`，单次关联材料换取绑定结果 |
| `POST /agent/v1/heartbeats` | `Heartbeat` |
| `POST /agent/v1/executions:claim` | `ClaimExecution` 长轮询 |
| `POST /agent/v1/executions/{executionId}:acknowledge-lease` | `AcknowledgeLease` |
| `POST /agent/v1/executions/{executionId}:renew-lease` | `RenewLease` |
| `POST /agent/v1/executions/{executionId}/secret-slots:resolve` | 有效租约内解析固定秘密槽位，响应 `no-store` |
| `POST /agent/v1/executions/{executionId}/events:append` | `AppendExecutionEvents` |
| `POST /agent/v1/executions/{executionId}/logs:append` | LG 日志批次或 GapNotice |
| `POST /agent/v1/executions/{executionId}:reconcile` | `ReconcileExecution` |
| `POST /agent/v1/executions/{executionId}:release` | `ReleaseTerminalExecution` |

### 6.2 新增预检查协议落点

| 方法与路径 | 语义 |
|---|---|
| `POST /agent/v1/prechecks:claim` | `ClaimPrecheck`，只领取明确选给本节点的 `EXPORT_PREFLIGHT` |
| `POST /agent/v1/prechecks/{precheckId}:acknowledge-lease` | 确认固定检查清单和短租约 |
| `POST /agent/v1/prechecks/{precheckId}/secret-slots:resolve` | 只解析该检查绑定的数据库凭据 revision |
| `POST /agent/v1/prechecks/{precheckId}:complete` | 提交结构化检查项、证据摘要和完整性；不上传任意文件 |

预检查和正式执行分别领取；Agent 同时只运行一个执行工作，预检查是否占用空闲容量由固定规则决定。控制面不主动连接 Agent，不增加入站端口。

## 7. 统一错误契约

错误响应固定为：

```json
{
  "requestId": "opaque-request-id",
  "code": "DRAFT_REVISION_CONFLICT",
  "message": "草稿已发生变化，请刷新后重试",
  "retryable": false,
  "fieldErrors": [],
  "safeDetails": {}
}
```

- `400`：JSON/协议结构错误、未知字段；
- `401`：身份或机器认证失败；
- `403`：主体已确认但平台级能力明确禁止，且不存在对象枚举风险；
- `404`：资源不存在或主体无权发现；
- `409`：状态冲突、幂等键内容冲突、批次/事件冲突；
- `412`：`If-Match` revision 已过期；
- `422`：字段、参数、预检查或提交门禁不通过；
- `429`：明确限流/容量保护；
- `503`：数据库繁忙超时、Agent/依赖暂时不可用或安全上下文无法建立。

错误不得包含 SQL、表名、堆栈、本地路径、完整用户名、密码、密文、命令原文或无权对象差异。`retryable=true` 只表示相同安全请求可以重试，不表示客户端可绕过 revision、预检查或租约。

## 8. 幂等与并发

### 8.1 浏览器操作

- 有创建副作用的 POST 要求 `Idempotency-Key`；键绑定主体、操作和规范请求摘要；
- 相同键与相同摘要返回原结果，相同键与不同摘要返回 `409 IDEMPOTENCY_CONFLICT`；
- 首版浏览器幂等记录保留至少 24 小时；任务提交记录至少保留到任务创建事实可稳定查询；
- 草稿和数据源编辑使用 `If-Match: "rev-N"`，服务端执行 `WHERE revision=N` 的原子更新；
- 状态动作以当前状态作为条件，同状态重发返回当前结果，不重复审计成功动作。

### 8.2 Agent 操作

- requestId、eventId/eventSeq、日志来源序号和批次摘要分别去重，不能只靠 HTTP 请求 ID；
- 同一任务最多一个 TaskExecution；数据库唯一约束确保并发领取只有一个成功；
- 事件必须外键绑定 executionId + leaseId + leaseEpoch，并按 executionId + eventSeq 唯一；
- 相同配置指纹允许形成不同 taskId，避免把用户明确的两次独立运行错误合并。

## 9. SQLite 数据边界

### 9.1 进入 SQLite

- 身份投影、固定授权绑定；
- 数据源非敏感连接信息和凭据密文修订；
- 节点、Agent 身份和当前运行事实；
- 草稿、预检查、不可变任务快照；
- execution、lease、事件、状态/结果投影；
- 幂等记录和审计事实；
- 日志流、段、批次、摘要、偏移和完整性索引。

### 9.2 不进入 SQLite

- 数据源密码明文、根密钥、Agent 机器凭据原值、关联材料原值；
- 未脱敏命令、未脱敏日志和工具原始文件；
- 导出 CSV 文件和 OBDUMPER 发布包；
- 每一条工具日志正文；
- 可由版本化发布资源确定的完整参数元数据正文。

参数元数据随应用以只读版本化资源发布，任务只保存精确版本和摘要。SQLite 不成为在线参数编辑器。

## 10. 首条切片最小表集

首版候选共 20 张窄表，不对应 20 个服务或模块：

| 表 | 作用 | 关键约束 |
|---|---|---|
| `schema_migrations` | 迁移版本、名称、校验和和应用时间 | version 主键；已应用校验和不可变化 |
| `auth_subjects` | 已认证身份只读投影 | 不保存密码；稳定外部 subject 唯一 |
| `subject_grants` | 五类固定角色和独立能力 | 固定枚举；不支持自定义角色/表达式 |
| `subject_object_scopes` | 数据源使用/管理、节点使用/管理、任务运维范围 | 主体+范围类型+对象唯一；无隐式全部 |
| `data_sources` | 非敏感连接、环境、状态、revision、最近测试摘要 | 名称按规范唯一；密码不在本表；引用后归档 |
| `credential_revisions` | AES-GCM 密文、nonce、keyId、AAD 元数据和状态 | credentialId+revision 唯一；绑定 dataSourceId |
| `execution_nodes` | 节点管理状态、平台、允许根目录和工具配置引用 | 节点状态与 Agent 在线状态分开 |
| `agent_enrollment_tokens` | 一次性关联材料摘要、过期和消费事实 | 只存单向摘要；单次原子消费 |
| `agents` | agentId、nodeId、凭据校验材料、boot/心跳/容量当前事实 | 一个有效 Agent 绑定一个节点；不存机器凭据原值 |
| `export_drafts` | 所有者、revision、首条切片草稿 JSON 和失效摘要 | 乐观锁；可编辑；不含密码 |
| `precheck_runs` | 草稿/指纹/节点/credential 绑定、短租约、检查项 JSON、状态和有效期 | 不创建 TaskExecution；结果只对精确绑定有效 |
| `tasks` | 提交后的不可变快照、计划命令证据和版本摘要 | 禁止更新/删除；同 fingerprint 可多任务 |
| `task_executions` | task 一对一执行、节点/Agent、状态投影、进程和结果摘要 | taskId 唯一；状态更新有 revision |
| `execution_leases` | leaseId/epoch/Agent/控制面时间和状态 | execution+epoch 唯一；保留历史租约事实 |
| `execution_events` | 追加式状态/进程/终态事件 | eventId、execution+eventSeq 唯一；复合租约外键 |
| `request_idempotency` | 主体/操作/键、请求摘要和原资源结果 | 复合主键；不保存秘密请求正文 |
| `audit_events` | 主体、动作、对象、结果和非敏感差异 | 追加式；业务成功与审计同事务 |
| `log_streams` | 来源流、epoch、期望序号、采集和完整性状态 | stream+epoch 唯一 |
| `log_segments` | 分段文件标识、偏移范围、摘要、状态和保留期 | 不保存本地绝对路径或正文；封段不可变 |
| `log_batches` | 已确认批次序号范围、摘要、段偏移和 Gap 摘要 | 同流同 firstSeq 唯一；冲突拒绝 |

已提交但尚无 `task_executions` 记录的任务派生为“等待调度”；领取成功后状态来自 `task_executions` 投影。任务状态不写回不可变 `tasks`，也不因列表查询临时修改历史快照。

数据源连接测试的最近结果直接更新 `data_sources` 的测试摘要，并用 `audit_events` 保存动作事实；V1.0 不为每次测试建立历史诊断表。Agent 心跳只更新当前事实，不保存无限心跳历史。显式日志缺口作为版本化 GAP 记录写入脱敏段，并在 `log_streams/log_batches` 保存摘要，不另建缺口表。

## 11. 字段和约束规则

- 所有外部 ID 为 `TEXT` UUID，数据库内部不得把 rowid 返回 API；
- 表使用 SQLite `STRICT`；布尔值为 0/1 并带 CHECK；枚举带 CHECK 或由版本化迁移管理；
- 时间保存规范 UTC 文本，写入前由应用解析并格式化；数据库不保存本地时区时间；
- JSON 列带 `json_valid`，写入规范 JSON；用于摘要的 JSON 必须使用 PC-R11 规范化规则；
- revision、epoch 和序号为正整数；更新必须带期望 revision；
- tasks、execution_events、audit_events 和封段后的 log_segments 是追加/不可变事实；
- 删除使用受控归档/保留清理，不使用外键级联删除历史任务、事件、审计或日志索引；
- 密文、nonce 和摘要使用 BLOB/TEXT 精确类型，不用 Base64 假装普通业务字段；
- schema 只保存受控相对文件标识，绝不保存用户可提交的控制面绝对路径。

## 12. 关键事务边界

| 操作 | 同一短事务内 | 事务外 |
|---|---|---|
| 新增/编辑数据源 | 数据源、credential revision、授权影响、审计、幂等结果 | 无数据库连接测试 |
| 基础连接测试 | 先写审计意图；测试后另一个事务更新最新结果和审计结果 | DNS/TCP/认证/数据库连接 |
| 发起预检查 | 冻结草稿 revision/fingerprint/绑定、创建 PrecheckRun、审计、幂等 | Agent 实际检查 |
| 完成预检查 | 校验短租约和绑定、保存结构化结果/有效期、审计 | 无工具进程启动 |
| 提交任务 | 重校验权限和所有绑定、重算规范配置/命令、创建不可变 task、审计、幂等结果 | 不等待 Agent |
| Agent 领取 | 原子创建唯一 TaskExecution 和 lease、投影启动中、写调度事件 | 长轮询等待不持有事务 |
| 追加执行事件 | 去重/顺序/租约校验、事件写入、状态/结果投影、确认水位 | 无文件或网络等待 |
| 日志批次 | 文件追加+fsync 后，SQLite 登记批次/段偏移/摘要 | 文件写入不占 SQLite 事务，顺序遵循 LG-R12 |
| 权限变更 | 完整角色、范围、能力差异和审计原子保存 | 身份目录查询 |

任何 TCP、数据库探测、Agent 长轮询、文件 fsync、工具运行、下载流或外部身份查询期间都不得持有 SQLite 写事务。

## 13. SQLite 运行配置

- 数据库文件只放控制面本机磁盘，禁止网络共享盘和多控制面共享；
- 每个连接启用 `foreign_keys=ON`；启动核对实际值；
- 使用 WAL、`synchronous=FULL` 和 5 秒 `busy_timeout` 首版值；不能无限重试 `SQLITE_BUSY`；
- 一个受控写连接/写队列串行短事务，另设有界只读连接；工具运行不占连接或事务；
- 启动先完成迁移、`quick_check`/必要完整性核对和日志索引恢复，再开放 API；
- 正常关闭执行 WAL checkpoint；异常退出由 SQLite WAL 和各契约恢复流程处理；
- 数据库目录、WAL/SHM、备份和日志目录使用专用服务账户权限；
- 数据库繁忙超过边界返回可追踪的 `503`，不把未提交操作声明成功。

上述配置是首版安全默认，不进入普通系统设置页面。运行证据证明需要调整时再版本化修改。

## 14. 迁移、升级和回退

1. SQL 文件按单调版本编号，内容、名称和 SHA-256 进入 `schema_migrations`；
2. 已应用迁移的校验和变化时拒绝启动，禁止修改历史迁移“修库”；
3. 每个迁移尽量单事务；SQLite 需要重建表时使用新表、复制、约束核对、替换的受控流程；
4. 应用拒绝打开比自身支持版本更高的数据库；不在运行时猜测降级；
5. 发布前先用 Online Backup API 或 `VACUUM INTO` 生成一致备份并验证可读性；不能运行中直接复制 `.db`；
6. 生产升级只前向迁移；应用回退依赖升级前数据库备份和明确兼容矩阵，不提供未经验证的自动 down migration；
7. SQLite 备份不捆绑根密钥，跨机器恢复继续服从 CS-R05；
8. Windows AMD64 和三个麒麟目标运行同一迁移集合及升级/备份恢复夹具。

## 15. 查询、索引和保留

首版只为实际访问路径建索引：

- 数据源授权列表：状态、环境、规范名称；
- 草稿：owner + updatedAt；
- 预检查：draft/revision、node、status、expiresAt；
- 任务：creator、createdAt；授权任务范围按 dataSourceId 快照摘要；
- execution：task 唯一、node/agent、state；
- lease：execution/epoch、expiresAt；
- event：execution/eventSeq；
- 日志：scope、来源、epoch、时间和序号范围；
- 审计：主体、对象、动作和时间。

不为未来筛选提前建立大量复合索引。任务、事件、审计和日志保留遵循产品/安全契约；幂等记录和已消费关联材料按独立短保留清理。清理分批短事务执行，不阻塞 Agent 心跳和事件写入。

## 16. 安全与数据最小化

- API 模型区分 `SecretInput`、`CredentialReference` 和普通字符串；
- 通用 JSON map 不允许承载密码、机器凭据或秘密槽位原值；
- 数据库、备份、迁移夹具、OpenAPI 示例和错误快照搜索不到秘密；
- credential ciphertext 仍按对象权限隔离，普通数据访问层不提供“读取密文”接口；
- 任务快照只保存 credentialId/revision，不保存连接密码或可逆命令；
- 预检查结果只保存结构化结论和脱敏证据摘要，不保存查询结果行、任意 SQL 或数据库错误原文；
- 审计事件不复制业务正文，只保存必要非敏感差异；
- 备份、迁移失败和完整性错误不得把文件路径、SQL 或密文返回浏览器。

## 17. 最小验证矩阵

1. 同一幂等键同摘要返回同一资源，不同摘要冲突；
2. 两个并发领取只能创建一个 TaskExecution；
3. 草稿旧 revision 更新为 0 行并返回 412；
4. task 快照不可修改，相同 fingerprint 可以创建两个独立任务；
5. 事件重复序号、错误 lease 或旧 epoch 均拒绝且不改变投影；
6. 业务成功与审计成功同事务，回滚不留下半个事实；
7. 数据源密码在 API 读取、SQLite 普通列、备份、日志、错误和测试输出中不存在；
8. 网络连接测试、Agent 长轮询和文件 fsync 期间没有写事务；
9. 日志相同序号不同摘要冲突，文件/SQLite 崩溃恢复符合 LG-R12；
10. 权限变化后旧游标、预检查和提交立即失效；
11. 迁移校验和变化、数据库版本过高、外键关闭和完整性失败均拒绝启动；
12. Windows AMD64、麒麟 V10 SP3 C86、V10 SP1 ARM64、V11 ARM64 完成初始化、迁移、并发领取、事件、备份和恢复。

当前已完成核心约束及仓储短事务的本地合成验证，详见[API/数据模型 SQLite 约束验证](evidence/api-data-model-sqlite-spike-2026-07-21.md)。该结果不开放浏览器 API、Agent 协议或真实任务。

## 18. 明确禁止

- 为全部 V1.0 页面一次性建立 API 和表；
- 把数据库表一一映射为公开 CRUD 接口；
- 由前端生成可信命令、状态、权限或快照；
- 使用 fingerprint 替代 taskId 或幂等键；
- 用控制面路径检查冒充 Agent 节点路径检查；
- 为预检查开放 Shell、任意 SQL、任意文件或通用远程执行；
- 在任务提交后修改原快照以“同步最新数据源”；
- 把运行状态直接写回不可变 task 表；
- 把工具日志逐行写 SQLite；
- 在网络调用、文件写入或工具运行期间持有写事务；
- 在网络共享盘使用 WAL 或让多个控制面共享数据库；
- 修改已应用迁移、自动执行未验证降级或运行时直接复制 `.db`；
- 在 API、SQLite、备份、审计或错误中保存秘密原值。

## 19. 对现有契约的影响

- 给 TD-001 模块化单体、TS-R06 REST/OpenAPI、TS-R11 显式 SQL/迁移提供实现边界；
- 把 PC 草稿、预检查、指纹和不可变快照落到 API/数据约束；
- 给 AS 的 ClaimExecution、租约、事件和状态投影确定 URL 与关系约束；
- 新增 `EXPORT_PREFLIGHT/ClaimPrecheck`，并通过 AD-R09 同步补充 AS 和 CS 的预检查秘密槽位边界；
- 落实 CS 的 credential revision、审计原子性和普通 API 无秘密原则；
- 落实 LG 的分段文件 + SQLite 索引、批次幂等和游标边界；
- 本契约确认后仍需认证接入/首次管理员、正式 SQL 迁移、OpenAPI、跨平台运行及已有 P0，不直接获得业务代码准入。

## 20. 本轮评审项

| ID | 评审项 | 建议结论 | 状态 |
|---|---|---|---|
| AD-R01 | 切片范围 | 只定义数据源、最小节点关联、单表 CSV 草稿/预检查/提交、执行、任务和日志接口/数据 | 已确认 |
| AD-R02 | 接口域 | 浏览器 `/api/v1` 与 Agent `/agent/v1` 分离认证和模型 | 已确认 |
| AD-R03 | API 格式 | HTTPS、UTF-8 JSON、UTC、随机 UUID、OpenAPI、不透明游标和 no-store | 已确认 |
| AD-R04 | 授权 | 身份、固定能力和对象范围同时校验；无权对象默认 404，失败关闭 | 已确认 |
| AD-R05 | 错误 | 使用稳定错误码和安全详情，不返回 SQL、堆栈、路径、秘密或枚举差异 | 已确认 |
| AD-R06 | 乐观并发 | 数据源/草稿使用 revision + If-Match；旧 revision 返回 412 | 已确认 |
| AD-R07 | 幂等 | 创建类 POST 绑定主体+操作+键+请求摘要；同键异内容 409 | 已确认 |
| AD-R08 | 数据源 API | 密码仅写，编辑缺失表示不变；测试不持事务且只覆盖基础连接 | 已确认 |
| AD-R09 | 远端预检查 | 新增固定 `EXPORT_PREFLIGHT/ClaimPrecheck`；绑定短租约，禁止 Shell/任意 SQL/文件 | 已确认 |
| AD-R10 | 提交 | 提交事务重算权限、绑定、规范配置和命令，冻结不可变 task；相同指纹可多任务 | 已确认 |
| AD-R11 | 任务日志 API | 概要、快照、命令、执行和日志分资源；无取消/重试/删除 API | 已确认 |
| AD-R12 | Agent API | 落实领取、租约、槽位、事件、日志、核对和释放；机器身份不能调业务 API | 已确认 |
| AD-R13 | 数据边界 | SQLite 存业务事实/索引；秘密原值、导出文件、工具包和逐行日志不入库 | 已确认 |
| AD-R14 | 最小表集 | 采用第 10 节 20 张窄表，不为全 V1.0 或每次心跳/连接测试建表 | 已确认 |
| AD-R15 | 不可变/唯一约束 | task、事件、审计、封段不可变；execution/task、eventSeq、lease 和日志批次由约束保护 | 已确认 |
| AD-R16 | 事务 | 业务与审计原子；外部 I/O、长轮询、工具运行和 fsync 不占 SQLite 写事务 | 已确认 |
| AD-R17 | SQLite 运行 | 本机单实例、WAL、FULL、foreign_keys、5 秒 busy、单写有界读 | 已确认 |
| AD-R18 | 迁移与备份 | 前向校验和 SQL 迁移；升级前一致备份；不改历史迁移、不自动 down migration | 已确认 |
| AD-R19 | 认证未决边界 | 不信任任意用户头；会话接入和首次管理员继续作为独立生产阻断 | 已确认 |
| AD-R20 | 平台门禁 | Windows 和三个麒麟目标分别验证初始化、迁移、并发、事件、备份恢复和安全 | 已确认 |
