# EX-D0 当前实现盘点与能力冻结

> 文档状态：已完成盘点，已冻结兼容边界
> 盘点日期：2026-08-05
> 依据：[开发任务地图](development-task-map.md) 第 4 节 EX-D0
> 适用范围：`CSV_SINGLE_TABLE_V1` 已实现基线的全维度现状追溯
> 安全说明：不记录真实端点、身份、密码、密钥、完整命令、输出路径或工具原始输出

## 0. 文档目的

本文是 EX-D0 当前实现盘点与能力冻结的唯一产出。它形成：

1. **现状矩阵**：页面、API、OpenAPI、SQLite、参数元数据、命令生成、预检查、Agent、结果和测试共 10 个维度的逐项追溯。
2. **兼容边界冻结**：`CSV_SINGLE_TABLE_V1` 中不可因后续泛化而改变的结构和安全约束。
3. **差距清单**：当前专用结构与[任务地图](development-task-map.md)第 3 节功能全景之间的差距，按可复用、需泛化、需新增和必须保留的安全边界分类。

每项结论追溯到源码路径、契约段落或测试文件；不以历史宣称替代当前事实。

---

## 1. 现状矩阵

### 1.1 页面现状

| 页面 | 路由 | 组件 | 已实现能力 | 未实现能力 |
|---|---|---|---|---|
| 首页 | `/` | `HomeView.vue` | 导航入口 | — |
| 数据源列表 | `/data-sources` | `DataSourceListView.vue` | 列表、筛选（关键字/环境/连接方式/连接状态/启用状态）、分页 | — |
| 数据源表单 | `/data-sources/:id` | `DataSourceFormView.vue` | 创建/编辑、连接串解析、连接测试（选节点 + Agent JDBC）、启停、删除/归档 | — |
| 导出向导 | `/exports/new` | `ExportWizardView.vue` | 六步流程：单数据源选择→单库单表→固定仅数据→固定 CSV→路径+节点+可选日志路径/skip-check-dir→六项预检查+命令预览+提交 | EX-F009 对象范围（全部/指定）、EX-F010 对象类型、EX-F011 多对象表达式、EX-F016 DDL/数据组合、EX-F017 多格式、EX-F022 对象存储、EX-F025~EX-F042 高级参数 |
| 任务中心 | `/tasks` | `TaskCenterView.vue` | 授权列表、游标分页（10/20/50）、刷新 | 筛选、跨任务日志检索、下载、取消、重试 |
| 任务详情 | `/tasks/:id` | `TaskDetailView.vue` | 概览/快照/命令证据/执行事实四投影独立读取、SSE 实时日志流、游标续传 | 结果清单解析、检查点继续、派生任务 |
| 执行节点列表 | `/nodes` | `ExecutionNodeView.vue` | 列表、候选筛选 | — |
| 执行节点表单 | `/nodes/new`, `/nodes/:id/edit` | `ExecutionNodeFormView.vue` | 创建（声明配置）、编辑、关联签发、环境检查请求、启用 | — |
| 执行节点详情 | `/nodes/:id` | `ExecutionNodeDetailView.vue` | 关联/心跳/平台/容量/环境投影 | — |
| 模板中心 | `/templates` | `TemplateCenterView.vue` | 占位空态 | 全部模板能力 |
| 普通导入 | `/imports/normal/new` | `NormalImportWizardView.vue` | 占位空态 | 全部导入能力 |
| 旁路导入 | `/imports/direct/new` | `DirectLoadWizardView.vue` | 占位空态 | 全部旁路导入能力 |
| 日志中心 | `/logs` | `LogCenterView.vue` | 占位空态 | 跨任务日志检索、下载 |
| 系统设置 | `/settings` | `SystemSettingsView.vue` | 占位空态 | 全部设置能力 |
| 访问控制 | `/settings/access-control` | `AccessControlView.vue` | 占位空态 | 全部权限配置能力 |

**前端辅助逻辑**（`web/src/views/`）：
- `taskListPresentation.ts`：状态标签和分类
- `taskLogStreamLifecycle.ts`：SSE 生命周期管理
- `taskFailurePresentation.ts`：失败原因总结
- `exportPrecheckPresentation.ts`：预检查结果展示和阻断
- `exportDraftInput.ts`：草稿输入验证
- `exportDataSourceEligibility.ts`：数据源导出资格判断
- `dataSourceListFilters.ts`：数据源筛选
- `dataSourceFormErrors.ts`：表单错误处理

**源码追溯**：`web/src/router/index.ts`（路由）、`web/src/views/`（组件）、`web/src/api/browser.ts`（API 客户端，1181 行）

### 1.2 API 现状

#### 浏览器域 `/api/v1`

| 端点 | 方法 | operationId | 源码位置 |
|---|---|---|---|
| `/api/v1/session` | GET | `getSession` | `internal/controlplane/server.go` |
| `/api/v1/data-sources` | GET | `listDataSources` | `server.go` |
| `/api/v1/data-sources` | POST | `createDataSource` | `server.go` |
| `/api/v1/data-sources/{dataSourceId}` | GET | `getDataSource` | `server.go` |
| `/api/v1/data-sources/{dataSourceId}` | PATCH | `updateDataSource` | `server.go` |
| `/api/v1/data-sources/{dataSourceId}` | DELETE | `deleteDataSource` | `server.go` |
| `/api/v1/data-sources/{dataSourceId}:test-connection` | POST | `testDataSourceConnection` | `server.go` |
| `/api/v1/data-sources/{dataSourceId}:enable` | POST | `enableDataSource` | `server.go` |
| `/api/v1/data-sources/{dataSourceId}:disable` | POST | `disableDataSource` | `server.go` |
| `/api/v1/data-source-connection-tests/{connectionTestId}` | GET | `getDataSourceConnectionTest` | `server.go` |
| `/api/v1/execution-nodes` | GET | `listExecutionNodes` | `server.go` |
| `/api/v1/execution-nodes` | POST | `createExecutionNode` | `server.go` |
| `/api/v1/execution-nodes/{nodeId}` | GET | `getExecutionNode` | `server.go` |
| `/api/v1/execution-nodes/{nodeId}` | PATCH | `updateExecutionNode` | `server.go` |
| `/api/v1/execution-nodes/{nodeId}` | DELETE | `deleteOrArchiveExecutionNode` | `server.go` |
| `/api/v1/execution-nodes/{nodeId}:enrollments` | POST | `issueAgentEnrollment` | `server.go` |
| `/api/v1/execution-nodes/{nodeId}:environment-check` | POST | `requestExecutionNodeEnvironmentCheck` | `server.go` |
| `/api/v1/execution-nodes/{nodeId}:enable` | POST | `enableExecutionNode` | `server.go` |
| `/api/v1/export-drafts` | POST | `createExportDraft` | `server.go` |
| `/api/v1/export-drafts/{draftId}` | GET | `getExportDraft` | `server.go` |
| `/api/v1/export-drafts/{draftId}` | PATCH | `updateExportDraft` | `server.go` |
| `/api/v1/export-drafts/{draftId}:preview-command` | POST | `previewExportDraft` | `server.go` |
| `/api/v1/export-drafts/{draftId}:precheck` | POST | `createExportPrecheck` | `server.go` |
| `/api/v1/export-drafts/{draftId}:submit` | POST | `submitExportDraft` | `server.go` |
| `/api/v1/prechecks/{precheckId}` | GET | `getExportPrecheck` | `server.go` |
| `/api/v1/tasks` | GET | `listTasks` | `server.go` |
| `/api/v1/tasks/{taskId}` | GET | `getTask` | `server.go` |
| `/api/v1/tasks/{taskId}/snapshot` | GET | `getTaskSnapshot` | `server.go` |
| `/api/v1/tasks/{taskId}/command-evidence` | GET | `getTaskCommandEvidence` | `server.go` |
| `/api/v1/tasks/{taskId}/execution` | GET | `getTaskExecution` | `server.go` |
| `/api/v1/tasks/{taskId}/logs` | GET | `listSyntheticLogs` | `server.go` |
| `/api/v1/tasks/{taskId}/logs/stream` | GET | `streamTaskLogs` | `server.go` |

**安全约束**：所有写操作要求 CSRF token；草稿/数据源创建要求幂等键；更新要求 `If-Match` 乐观锁；密码只写不回显；日志双层脱敏。

#### Agent 域 `/agent/v1`

| 端点 | 方法 | 用途 |
|---|---|---|
| `/agent/v1/enrollments:exchange` | POST | Agent 交换一次性关联材料 |
| `/agent/v1/heartbeats` | POST | Agent 心跳上报 |
| `/agent/v1/prechecks:claim-next` | POST | 领取下一条预检查 |
| `/agent/v1/prechecks/{precheckId}:acknowledge-lease` | POST | 确认预检查租约 |
| `/agent/v1/prechecks/{precheckId}/secret-slots:resolve` | POST | 解析预检查秘密槽位 |
| `/agent/v1/prechecks/{precheckId}:complete` | POST | 完成预检查 |
| `/agent/v1/data-source-connection-tests:claim-next` | POST | 领取下一条连接测试 |
| `/agent/v1/data-source-connection-tests/{testId}:acknowledge-lease` | POST | 确认连接测试租约 |
| `/agent/v1/data-source-connection-tests/{testId}/secret-slots:resolve` | POST | 解析连接测试秘密槽位 |
| `/agent/v1/data-source-connection-tests/{testId}:complete` | POST | 完成连接测试 |
| `/agent/v1/executions:claim-next` | POST | 领取下一条执行任务 |
| `/agent/v1/executions/{executionId}:acknowledge-lease` | POST | 确认执行租约 |
| `/agent/v1/executions/{executionId}:renew-lease` | POST | 续期执行租约 |
| `/agent/v1/executions/{executionId}/secret-slots:resolve` | POST | 解析执行秘密槽位 |
| `/agent/v1/executions/{executionId}/events:append` | POST | 追加执行事件 |
| `/agent/v1/executions/{executionId}/logs:append` | POST | 追加执行日志 |
| `/agent/v1/executions/{executionId}/logs:gap` | POST | 追加日志缺口 |
| `/agent/v1/executions/{executionId}:reconcile` | POST | 核对执行状态 |
| `/agent/v1/executions/{executionId}:release` | POST | 释放终态执行 |
| `/agent/v1/execution-node-environment-checks/{checkId}:complete` | POST | 完成环境检查 |

**安全约束**：浏览器域和 Agent 域身份完全隔离；Agent 使用 Bearer 机器凭据；所有协议操作要求 leaseId + leaseEpoch 绑定；秘密槽位仅在有效租约内短时可用。

**源码追溯**：`internal/controlplane/server.go`（路由分发和 handler 实现）、`cmd/control-plane/main.go`（composition root）

### 1.3 OpenAPI 现状

| 属性 | 当前值 |
|---|---|
| 规范版本 | OpenAPI 3.1.0 |
| API 版本 | `0.1.0-dev02` |
| 开发门禁标记 | `x-development-gate: G2_CONTRACT_ONLY` |
| 真实执行标记 | `x-real-execution-enabled: false` |
| Schema 完备性 | `x-schema-completeness: STRUCTURAL_BASELINE` |
| 认证阻断 | Browser identity provider 和 recovery identity 仍为部署阻断 |
| Tags | Session、ExecutionNodes、DataSources、ExportDrafts、Tasks、AgentEnrollment、AgentHeartbeat、AgentEnvironmentChecks、AgentPrechecks、AgentConnectionTests、AgentExecutions |

OpenAPI 覆盖所有已实现端点的请求/响应结构定义。契约测试（`contracts/openapi_test.go`，1025 行）验证实现与规范一致性。

**差距**：草稿 JSON 模型仍是单表 CSV 专用；无通用导出配置 schema、无模板/导入/高级参数的 OpenAPI 定义。

**源码追溯**：`contracts/openapi.json`、`contracts/openapi_test.go`、`contracts/embed.go`

### 1.4 SQLite 现状

13 个迁移（0001~0013），共计约 28 张表（含后续迁移新增）。按领域分组：

| 领域 | 表名 | 主要字段概要 | 迁移 |
|---|---|---|---|
| 身份与授权 | `auth_subjects` | subject_id, external_subject, display_name, account_status | 0001 |
| | `subject_grants` | subject_id, grant_code（5 类角色 + 5 类能力） | 0001 |
| | `subject_object_scopes` | subject_id, scope_type, object_id | 0001 |
| 数据源 | `data_sources` | 连接信息、环境、兼容模式、状态、revision、最近测试摘要 | 0001, 0002, 0003 |
| | `credential_revisions` | credential_id, revision, AES-GCM 密文/nonce/keyId/AAD | 0001 |
| 执行节点 | `execution_nodes` | 声明配置、管理状态、平台、允许根目录、环境检查字段 | 0001, 0010, 0011 |
| | `agent_enrollment_tokens` | 一次性关联材料摘要、状态 | 0001 |
| | `agents` | agent_id, node_id, 凭据校验、心跳、容量、facts_revision | 0001, 0005, 0006 |
| 导出草稿 | `export_drafts` | owner, data_source, node, revision, tool/metadata/capability_version, config_json, fingerprint | 0001 |
| 预检查 | `precheck_runs` | draft 绑定、credential 绑定、node 绑定、租约、六项结果、有效期 | 0001 |
| | `agent_precheck_receipts` | claim/acknowledge/complete 的 Agent 请求摘要和租约状态 | 0007 |
| | `agent_precheck_secret_resolution_receipts` | 预检查秘密槽位解析记录 | 0008 |
| 连接测试 | `data_source_connection_test_runs` | 数据源绑定、节点绑定、租约、结果 | 0009 |
| | `agent_data_source_connection_test_receipts` | 连接测试 Agent 回执 | 0009 |
| | `agent_data_source_connection_test_secret_resolution_receipts` | 连接测试秘密解析 | 0009 |
| 任务与执行 | `tasks` | 不可变快照、计划命令、版本摘要 | 0001 |
| | `task_executions` | task 一对一执行、状态、reconciliation_required、结果摘要 | 0001 |
| | `execution_leases` | leaseId/epoch/Agent/状态/控制面时间 | 0001 |
| | `execution_events` | 追加式状态/进程/终态事件 | 0001 |
| | `execution_secret_resolution_receipts` | 正式执行秘密槽位解析 | 0012 |
| 日志 | `log_streams` | 来源流、epoch、采集状态 | 0001 |
| | `log_segments` | 分段文件标识、偏移范围、SHA-256 摘要、保留期、投影字段 | 0001, 0013 |
| | `log_batches` | 已确认批次范围、摘要、段偏移、策略/解析器版本 | 0001, 0013 |
| 辅助 | `schema_migrations` | 版本、名称、校验和 | 0001 |
| | `request_idempotency` | 主体/操作/幂等键绑定 | 0001 |
| | `audit_events` | 审计事件记录 | 0001 |

**关键约束**：全部表使用 `STRICT` 和 `WITHOUT ROWID`（除 `schema_migrations`）；全局启用 `foreign_keys`、WAL 模式、`FULL synchronous`、5 秒 `busy_timeout`；时间使用 RFC 3339 UTC；JSON 列带 `json_valid` 校验。

**差距**：无通用导出领域模型表（草稿/规范化配置/提交快照分表）、无模板表、无检查点/继续任务表、无派生任务关系表。

**源码追溯**：`migrations/0001_initial.sql`~`migrations/0013_add_log_batch_projection_fields.sql`、`internal/store/store.go`（~5384 行）、`internal/store/types.go`（~1096 行）、`internal/migrate/runner.go`、`docs/03-technical/api-sqlite-data-contract.md`

### 1.5 参数元数据现状

**版本化结构**：

| 版本 | capabilityVersion | 参数数量 | 变更 |
|---|---|---|---|
| v1（基础） | `export-direct-single-table-csv-v1` | 10 | 基础定义：host, port, user, password, database, table, csv, 8 个 CSV 序列化参数, file-path |
| v2~v4 | 逐步过渡 | 10 | password 的 emissionTarget 从 ARGV 调整为 SECURITY_FILE |
| v5（当前） | `export-odp-single-table-csv-v1` | 18 | 保留 v1 基础，overrides 为 host/port/user/password 设短参数名，additions 新增 log-path 和 skip-check-dir |

**当前 v5 参数清单**：

| # | 参数长名 | 短名 | 分类 | 值类型 | 来源策略 | 支持状态 | 敏感性 | 发射目标 |
|---|---|---|---|---|---|---|---|---|
| 1 | `--host` | `-h` | CONNECTION | string | DATA_SOURCE | ENABLED | IDENTIFIER | ARGV |
| 2 | `--port` | `-P` | CONNECTION | integer | DATA_SOURCE | ENABLED | NORMAL | ARGV |
| 3 | `--user` | `-u` | CONNECTION | string | DATA_SOURCE | ENABLED | IDENTIFIER | ARGV |
| 4 | password | `-p` | CONNECTION | secret-slot | SECURITY | ENABLED | SECRET | SECURITY_FILE |
| 5 | `--database` | — | DATABASE_CONNECTION | string | USER | ENABLED | IDENTIFIER | ARGV |
| 6 | `--table` | — | OBJECT_SCOPE | string | USER | ENABLED | IDENTIFIER | ARGV |
| 7 | `--csv` | — | CONTENT_FORMAT | flag | FORMAT | ENABLED | NORMAL | ARGV |
| 8 | `--skip-header` | — | FORMAT_SERIALIZATION | flag | USER | VALIDATION_GATED | NORMAL | ARGV |
| 9 | `--column-separator` | — | FORMAT_SERIALIZATION | string | USER | VALIDATION_GATED | NORMAL | ARGV |
| 10 | `--column-quote` | — | FORMAT_SERIALIZATION | string | USER | VALIDATION_GATED | NORMAL | ARGV |
| 11 | `--column-quote-mode` | — | FORMAT_SERIALIZATION | enum | USER | VALIDATION_GATED | NORMAL | ARGV |
| 12 | `--escape-character` | — | FORMAT_SERIALIZATION | string | USER | VALIDATION_GATED | NORMAL | ARGV |
| 13 | `--line-separator` | — | FORMAT_SERIALIZATION | string | USER | VALIDATION_GATED | NORMAL | ARGV |
| 14 | `--null-string` | — | FORMAT_SERIALIZATION | string | USER | VALIDATION_GATED | NORMAL | ARGV |
| 15 | `--file-encoding` | — | FORMAT_SERIALIZATION | enum | USER | VALIDATION_GATED | NORMAL | ARGV |
| 16 | `--file-path` | — | OUTPUT_FILE | path | USER | ENABLED | NORMAL | ARGV |
| 17 | `--log-path` | — | OUTPUT_FILE | path | USER | ENABLED | NORMAL | ARGV |
| 18 | `--skip-check-dir` | — | OUTPUT_FILE | flag | USER | ENABLED | NORMAL | ARGV |

**参数值状态机**：每个字段在一次规范化中落入 UNSET / EXPLICIT / DERIVED / INACTIVE / BLOCKED 之一。

**差距**：109 个长参数（见 `docs/02-design/export-parameter-mapping.md`）中仅 18 个已定义；无 DDL 行为、CUT/POS/SQL/Parquet/ORC/Avro 格式、对象存储、性能与资源、筛选与一致性、日期时间格式参数。

**源码追溯**：`internal/parammeta/catalog.go`、`internal/parammeta/resources/obdumper-4.3.5-slice-v1.json`（v1 基础）、v5 修订（代码内 revision manifest）、`docs/03-technical/parameter-command-contract.md`

### 1.6 命令生成现状

**生成流程**（`internal/commandgen/generator.go`）：

1. **请求验证**：校验 tool、toolVersion、metadataVersion、capabilityVersion、connectionKind、dataSourceFactVersion、nodeFactVersion、targetPlatform
2. **参数收集**：遍历 request.Fields，检测 UNKNOWN_PARAMETER 和 DUPLICATE_PARAMETER
3. **活动状态计算**：按 activation 规则和格式选择判定每个参数是否活动
4. **值规范化**：对活动参数执行类型转换（string/integer/boolean/enum/path/secret-slot）
5. **来源策略验证**：检查 sourcePolicy（USER/DATA_SOURCE/SECURITY/FORMAT）
6. **支持状态检查**：VALIDATION_GATED 参数被显式设置时阻断（BLOCKED）
7. **令牌生成**：秘密参数生成 SecretSlot；其他参数生成 argv 令牌
8. **脱敏展示**：跨平台渲染（`render.go`）+ 密码位置显示 `******`
9. **指纹计算**：SHA-256（规范化输入）→ configFingerprint

**输出结构**（`internal/commandgen/types.go`）：

| 字段 | 说明 |
|---|---|
| `argvTemplate` | 有序参数令牌数组，不含密码参数或密码槽位 |
| `redactedCommand` | 脱敏后的命令文本，密码显示 `******` |
| `secretSlots` | 秘密槽位 → 凭据引用、发射目标及安全属性 |
| `configFingerprint` | 规范化输入的 SHA-256 |
| `normalizedFields` | 每个参数的规范化结果 |
| `tokenEvidence` | 每个令牌的产品字段、参数、来源和 argv 位置范围 |

**差距**：仅支持 CSV 单表命令模板；无 DDL 命令、多对象表达式、多格式命令模板、`--query-sql`（保持 BLOCKED）。

**源码追溯**：`internal/commandgen/generator.go`（433 行）、`internal/commandgen/types.go`（140 行）、`internal/commandgen/render.go`

### 1.7 预检查现状

**固定六项检查**（`internal/precheckcontract/contract.go`）：

| 序号 | 检查名 | 受控证据码 | 是否需要秘密槽位 |
|---|---|---|---|
| 1 | DATABASE_CONNECTIVITY | DATABASE_CONNECTED / DATABASE_CONNECTION_FAILED / DATABASE_CONNECTION_UNAVAILABLE | 是 |
| 2 | OBJECT_ACCESS | OBJECT_ACCESSIBLE / OBJECT_NOT_ACCESSIBLE / OBJECT_ACCESS_UNAVAILABLE | 是 |
| 3 | TOOL_ENVIRONMENT | TOOL_RUNTIME_READY / TOOL_RUNTIME_INVALID / TOOL_RUNTIME_UNAVAILABLE | 否 |
| 4 | OUTPUT_PATH | OUTPUT_PATH_WRITABLE / OUTPUT_PATH_NOT_WRITABLE / OUTPUT_PATH_UNAVAILABLE | 否 |
| 5 | OUTPUT_EMPTY | OUTPUT_PATH_EMPTY / OUTPUT_EMPTY_CHECK_SKIPPED / OUTPUT_PATH_NOT_EMPTY / OUTPUT_PATH_UNAVAILABLE | 否 |
| 6 | AVAILABLE_SPACE | OUTPUT_SPACE_SUFFICIENT / OUTPUT_SPACE_INSUFFICIENT / OUTPUT_SPACE_UNAVAILABLE | 否 |

**执行顺序与依赖**（`internal/agentpreflight/preflight.go`）：
- 先执行 4 项无秘密本机检查（TOOL_ENVIRONMENT、OUTPUT_PATH、OUTPUT_EMPTY、AVAILABLE_SPACE）
- 仅当 4 项全 PASSED 时才解析数据库槽位并执行 DATABASE_CONNECTIVITY 和 OBJECT_ACCESS
- 任一前置检查 FAILED 或 UNKNOWN 时，后两项固定为 UNKNOWN 且不调用槽位解析

**差距**：无对象类型 × 兼容模式 × 数据库版本检查、无 DDL 专属预检查（sys 权限）、无对象存储网络/权限/空间检查、无高风险能力确认。

**源码追溯**：`internal/precheckcontract/contract.go`（66 行）、`internal/agentpreflight/preflight.go`（274 行）、`internal/agentlocalpreflight/probe.go`（358 行）、`internal/agentjdbc/precheck.go`（202 行）、`internal/jdbcprobe/probe.go`（458 行）

### 1.8 Agent 现状

**模块结构**：

| 模块 | 目录 | 职责 |
|---|---|---|
| 启动入口 | `cmd/agent/main.go` | 命令行解析、状态存储打开、Worker 组装、心跳循环 |
| 注册与关联 | `cmd/agent/registration.go` | 一次性关联材料交换、机器凭据保存 |
| 预检查 Worker | `cmd/agent/export_preflight.go` | EXPORT_PREFLIGHT 运行器和检查编排 |
| 连接测试 Worker | `cmd/agent/jdbc_connection.go` | AGENT_JDBC 连接测试（TCP + JDBC 探针） |
| 协议层 | `internal/agentwire/` | HTTP 客户端、预检查/连接测试/执行协议、心跳、事件和日志上报 |
| 预检查编排 | `internal/agentpreflight/` | 六项检查的纯本地编排逻辑 |
| 本地探针 | `internal/agentlocalpreflight/` | 工具环境、输出路径、目录空性、可用空间 |
| JDBC 适配 | `internal/agentjdbc/` | 数据库连接和对象访问的预检查适配 |
| JDBC 探针 | `internal/jdbcprobe/` | 固定 Java 探针（ConnectionProbe v1/v3） |
| 执行适配 | `internal/agentexec/` | 合成执行、直接 Java 启动、启动意图、清理 |
| 执行 Worker | `internal/agentexecution/` | 正式任务执行 Worker |
| 日志队列 | `internal/agentlogqueue/` | 本地可靠队列（1 GiB 上限、fsync、缺口、终态补传） |
| 状态管理 | `internal/agentstate/` | Agent 本机受保护状态（DPAPI/文件权限） |
| 环境检查 | `internal/agentenvironment/` | 固定本机运行时检查 Worker |
| 连接测试 | `internal/agentconnectiontest/` | 基础连接测试 Worker |
| 遥测 | `internal/agenttelemetry/` | 机器事实采样 |
| Worker 框架 | `internal/agentworker/` | Worker 接口和运行框架 |

**关键能力边界**：
- 直接 Java 启动（`internal/agentexec/direct_java.go`，426 行）：复刻 Windows OBDUMPER 4.3.5 的 obdumper.bat JVM 参数，直启 `com.oceanbase.tools.datax.Main` 主类
- Java 版本探测：按 Java 8 update 版本选择 CMS 或 G1 GC
- 安全配置隔离：security.configurationFile、log4j.output 和堆转储路径改为 execution 私有目录
- 进程监管：PID、开始时间、退出码、stdout/stderr 管道采集和第一层脱敏
- 可靠日志队列（`internal/agentlogqueue/queue.go`，494 行）：1 GiB 上限、fsync 批次、16 MiB 缺口元数据上限、终态补传恢复

**差距**：执行 Worker 当前通过固定信封启动，不具备通用导出参数解析能力；无检查点继续（dump.ckpt）；无完整失败恢复。

**源码追溯**：`cmd/agent/` 全部文件、`internal/agent*/` 全部模块、`docs/03-technical/agent-task-state-contract.md`、`docs/03-technical/tool-launch-isolation-contract.md`

### 1.9 结果现状

| 维度 | 当前实现 | 追溯 |
|---|---|---|
| 任务终态 | SUCCEEDED / FAILED + reconciliationRequired 标识 | `task_executions.state`、`task_executions.reconciliation_required` |
| 状态核对 | Agent bootId 变化附加"状态核对中"，保留原状态和租约 | `internal/controlplane/server.go` |
| 日志 | 双层脱敏、SSE 实时流、游标续传、.jsonl 分段封段 + SHA-256 核对、fsync 私有队列和终态补传 | `internal/logstream/`（7 文件）、`internal/agentlogqueue/` |
| 结果文件 | 仅核对 CSV 文件和清单文件存在（人工确认），无结构化解析 | 证据文件 `evidence/windows-authorized-real-export-2026-08-04.md` |
| 事件 | execution_events 追加式记录状态/进程/终态事件 | `execution_events` 表 |
| 进程证据 | process_evidence_json 存储进程启动/退出事实 | `task_executions.process_evidence_json` |

**差距**：无 dump.ckpt 检查点解析和继续任务创建；无结果清单（MANIFEST）结构化解析；无派生任务关系（基于原配置新建/从头执行/检查点继续）；无模板保存和复用。

**源码追溯**：`internal/store/types.go`（TaskSummary）、`internal/logstream/`、`migrations/0001_initial.sql`（task_executions、execution_events）

### 1.10 测试现状

#### 前端测试（14 个测试文件）

| 测试文件 | 覆盖范围 |
|---|---|
| `web/src/api/browser.test.ts`（896 行） | API 客户端请求/响应/错误 |
| `web/src/config/runtime.test.ts` | 运行时配置 |
| `web/src/views/dataSourceConnectionString.test.ts` | 连接串解析 |
| `web/src/views/dataSourceConnectionTestNotice.test.ts` | 连接测试通知 |
| `web/src/views/dataSourceDeletionNotice.test.ts` | 删除通知 |
| `web/src/views/dataSourceFormErrors.test.ts` | 表单错误 |
| `web/src/views/dataSourceListFilters.test.ts` | 列表筛选 |
| `web/src/views/executionNodeEnrollmentInstructions.test.ts` | 节点注册说明 |
| `web/src/views/exportDataSourceEligibility.test.ts` | 导出资格判断 |
| `web/src/views/exportDraftInput.test.ts` | 草稿输入 |
| `web/src/views/exportPrecheckPresentation.test.ts` | 预检查展示 |
| `web/src/views/taskFailurePresentation.test.ts` | 失败总结 |
| `web/src/views/taskListPresentation.test.ts` | 任务列表展示 |
| `web/src/views/taskLogStreamLifecycle.test.ts` | 日志流生命周期 |

#### 后端测试（25+ 个测试文件）

| 包/文件 | 覆盖范围 |
|---|---|
| `cmd/agent/main_test.go`（348 行） | Agent 启动、心跳、Worker 组装 |
| `cmd/agent/export_preflight_test.go` | 导出预检查 Worker |
| `cmd/agent/jdbc_connection_test.go`（163 行） | JDBC 连接测试 Worker |
| `cmd/agent/queue_validation_barrier_enabled_test.go` | 队列验证屏障 |
| `cmd/agent/registration_test.go` | Agent 注册 |
| `cmd/agent/restart_replay_binary_test.go`（479 行） | 重启重放二进制验证 |
| `cmd/control-plane/main_test.go` | 控制面启动 |
| `contracts/openapi_test.go`（1025 行） | OpenAPI 契约验证 |
| `internal/agentconnectiontest/worker_test.go`（251 行） | 连接测试 Worker |
| `internal/agentenvironment/worker_test.go`（72 行） | 环境检查 Worker |
| `internal/agentexec/adapter_test.go` | 合成执行适配 |
| `internal/agentexec/cleanup_test.go` | 清理逻辑 |
| `internal/agentexec/direct_java_test.go` | 直接 Java 启动 |
| `internal/agentexec/intent_test.go` | 启动意图 |
| `internal/agentexec/synthetic_test.go` | 合成执行 |
| `internal/agentexecution/worker_test.go` | 执行 Worker |
| `internal/agentexecution/worker_control_plane_fault_test.go` | 控制面故障处理 |
| `internal/agentjdbc/precheck_test.go` | JDBC 预检查 |
| `internal/agentlocalpreflight/probe_test.go` | 本地预检查探针 |
| `internal/agentlogqueue/*_test.go` | 日志队列（含 1 GiB 物理验证） |
| `internal/agentpreflight/preflight_test.go` | 预检查编排 |
| `internal/agentstate/*_test.go` | Agent 状态管理 |
| `internal/agenttelemetry/*_test.go` | 遥测 |
| `internal/agentwire/*_test.go` | 协议层 |
| `internal/commandgen/*_test.go` | 命令生成 |
| `internal/config/*_test.go` | 配置 |
| `internal/controlplane/*_test.go` | 控制面 |
| `internal/credential/*_test.go` | 凭据管理 |
| `internal/integration/*_test.go` | 集成测试（TestDEV04SyntheticSQLiteHTTPChain） |
| `internal/jdbcprobe/*_test.go` | JDBC 探针 |
| `internal/logstream/*_test.go` | 日志流 |
| `internal/migrate/*_test.go` | 迁移 |
| `internal/parammeta/*_test.go` | 参数元数据 |
| `internal/store/*_test.go` | 仓储层 |

**差距**：无多格式/多对象/DDL/对象存储/高级参数测试；无跨平台麒麟目标测试；无模板/导入相关测试。

---

## 2. CSV_SINGLE_TABLE_V1 兼容边界冻结

以下结构和安全约束已在当前实现中形成稳定事实。后续 EX-I1 通用导出骨架的泛化必须保持这些约束兼容，不得改变已有任务、草稿、快照和命令的语义。

### 2.1 草稿 JSON 结构

`export_drafts.config_json` 当前存储单表 CSV 专用配置。泛化后旧草稿仍可按原结构读取和提交。

**兼容约束**：草稿创建、读取和更新的服务端逻辑必须识别 `capabilityVersion = export-odp-single-table-csv-v1` 并按原字段集处理，不得因新增通用字段而使旧草稿校验失败。

**追溯**：`migrations/0001_initial.sql`（export_drafts 表）、`internal/store/store.go`（CreateExportDraft/GetExportDraft/UpdateDraft）

### 2.2 参数元数据版本

`metadataVersion = obdumper-4.3.5-slice-v5`、`capabilityVersion = export-odp-single-table-csv-v1` 是当前唯一活跃版本。

**兼容约束**：v5 基础资源通过 SHA-256 校验和固定，不可修改；新能力必须发布新版本，不得原地修改 v5 定义。发布后版本不可变。

**追溯**：`internal/parammeta/catalog.go`、`internal/parammeta/resources/obdumper-4.3.5-slice-v1.json`

### 2.3 命令生成输出

`Result` 结构（argvTemplate、redactedCommand、secretSlots、configFingerprint、normalizedFields、tokenEvidence）是当前命令生成的唯一权威输出。

**兼容约束**：
- argvTemplate 的令牌顺序和 secretSlots 结构（slotId、target、securityProperty、credentialReference）不可改变
- 密码始终通过 `database-password` 槽位写入安全文件属性 `oceanbase.jdbc.password`，不进入 argv
- configFingerprint 的计算方式（SHA-256 规范化输入）不可改变
- tokenEvidence 的 argvStart/argvLength 位置追踪不可改变

**追溯**：`internal/commandgen/types.go`（Result）、`internal/commandgen/generator.go`（Generate）

### 2.4 六项固定预检查

DATABASE_CONNECTIVITY、OBJECT_ACCESS、TOOL_ENVIRONMENT、OUTPUT_PATH、OUTPUT_EMPTY、AVAILABLE_SPACE 的名称、顺序、依赖关系和受控证据码（`internal/precheckcontract/contract.go` 中 `allowedEvidenceCodes`）不可改变。

**兼容约束**：新检查项必须在此契约、协议 schema 和负例测试中共同增加，不得由 Agent 自由上传。

**追溯**：`internal/precheckcontract/contract.go`（66 行）

### 2.5 任务快照

`tasks.snapshot_json` 存储提交时的不可变配置快照。`tasks.planned_argv_json` 存储计划命令令牌。`tasks.planned_command_redacted` 存储脱敏展示命令。

**兼容约束**：已提交任务的快照和命令证据不可修改；泛化后的新快照格式必须通过 capabilityVersion 区分，旧快照仍可按原结构解释。

**追溯**：`migrations/0001_initial.sql`（tasks 表）、`internal/store/types.go`（TaskSubmission）

### 2.6 Agent 执行信封

执行 Worker 通过固定信封领取任务，信封包含租约信息、工具版本、节点事实版本和秘密槽位引用。

**兼容约束**：信封结构不得引入任意命令、任意 SQL 或任意 URI；Agent 只接受固定任务类型。

**追溯**：`internal/agentexec/direct_java.go`、`internal/agentexecution/worker.go`、`docs/03-technical/tool-launch-isolation-contract.md`

### 2.7 日志双层脱敏

第一层脱敏在 Agent 本机执行（密码槽位规则）；第二层脱敏在控制面执行（日志入口规则）。秘密不进入日志正文、SQLite、审计或响应。

**兼容约束**：脱敏规则和安全投影（`log_streams`、`log_segments`、`log_batches` 结构）不可因泛化而降级；secret-slot 绑定机制（凭据引用 → 安全文件 → 进程内存）不可改变。

**追溯**：`internal/logstream/`、`docs/03-technical/log-collection-evidence-contract.md`

### 2.8 安全边界不可降级

以下安全约束在任何泛化中必须保留：

- 浏览器域 `/api/v1` 和 Agent 域 `/agent/v1` 身份完全隔离
- 密码/凭据原值不进入 argv、日志、审计、错误、响应、任务快照、SQLite 普通字段或 Git
- 命令生成器是唯一命令拼装权威，调用方不得自行拼接 Shell 字符串
- 控制面不直接操作执行节点文件或进程
- Agent 只上报事实，不决定产品参数、调度策略或最终产品状态
- 不提供 SSH、WinRM、远程 Shell、任意命令、任意 SQL、任意文件浏览能力

**追溯**：`docs/03-technical/credential-access-security-contract.md`、`AGENTS.md` 第 5 节

---

## 3. 差距清单

以下按[任务地图](development-task-map.md)第 3 节功能全景，列出当前专用结构与完整设计的差距。

### 3.1 可复用（现有结构可直接被通用模型引用）

| 能力 | 当前实现 | 复用方式 |
|---|---|---|
| 安全认证与授权 | 浏览器/Agent 双域隔离、CSRF、幂等键、乐观锁 | 通用模型直接沿用 |
| 凭据管理 | AES-GCM 加密存储、revision 管理、秘密槽位 | 通用模型直接沿用 |
| 日志系统 | 双层脱敏、SSE 流、游标续传、分段封段、SHA-256 核对、fsync 私有队列 | 通用模型直接沿用 |
| 租约协议 | leaseId + leaseEpoch 绑定、claim-next/acknowledge/complete/release | 通用模型直接沿用（预检查/连接测试/执行三类协议） |
| Agent 注册与心跳 | 一次性关联、机器凭据、bootId、factsRevision | 通用模型直接沿用 |
| 环境检查 | 固定本机运行时检查、TOOL_RUNTIME_READY | 通用模型直接沿用 |
| 幂等与审计 | request_idempotency、audit_events | 通用模型直接沿用 |
| 任务中心只读能力 | 授权列表、四投影详情、日志流 | 通用模型直接沿用 |
| 跨平台命令渲染 | Windows/Linux 转义差异处理 | 通用模型直接沿用 |

### 3.2 需泛化（现有专用结构需扩展为通用模型）

| 能力 | 当前专用结构 | 泛化方向 |
|---|---|---|
| 导出草稿 | `export_drafts.config_json` 为单表 CSV JSON | 版本化通用导出配置，按 capabilityVersion 区分 |
| 任务快照 | `tasks.snapshot_json` 为单表 CSV 快照 | 版本化通用快照，保持旧快照兼容读取 |
| 参数元数据 | v5 仅 18 个参数，仅 CSV 格式 | 新版本覆盖 DDL/多格式/对象存储/高级参数 |
| 命令生成 | 仅 CSV 单表命令模板 | 按 capabilityVersion 和格式分支生成不同命令模板 |
| 对象范围 | 单一数据库 + 单一表名 | 全部/指定对象、多对象表达式、多库 `schema.object`、排除表 |
| 导出内容 | 固定仅数据 | 仅 DDL / 仅数据 / DDL + 数据 |
| 数据格式 | 固定 CSV | CSV/CUT/POS/Insert SQL/Parquet/ORC/Avro |
| 输出位置 | 执行节点本地绝对路径 | 本地 + OSS/S3/COS/OBS |
| 预检查 | 固定六项 | 按对象类型/内容/格式/输出位置分层扩展 |
| 数据源连接 | 私有 ODP 单一连接 | 云 ODP、ODP Sharding、直连 OBServer、兼容模式差异 |

### 3.3 需新增（完整设计需要但当前不存在的能力）

| 能力 | 所属 EX-I 阶段 | 说明 |
|---|---|---|
| 对象类型矩阵 | EX-I2 | 表/表组/视图/触发器/用户/角色/序列/同义词/类型/包/函数/存储过程 × 兼容模式 × 数据库版本 |
| DDL 导出 | EX-I2 | `--ddl` 参数、DDL 行为分组、sys 凭据状态检查 |
| CSV 完整参数 | EX-I3 | EX-F043~EX-F047 序列化、日期时间、压缩、文件布局、筛选 |
| CUT 格式 | EX-I4 | CUT 专属参数和通用文本能力 |
| POS 格式 | EX-I4 | POS 映射需受控实测确认 |
| Insert SQL 格式 | EX-I4 | SQL 格式专属参数 |
| Parquet/ORC/Avro | EX-I5 | 结构化格式专属限制和资源 |
| 对象存储 | EX-I6 | OSS/S3/COS/OBS URI、存储凭据槽位、网络预检查 |
| DDL 行为 | EX-I7 | DROP、附加对象信息、Schema/序列策略 |
| 筛选与一致性 | EX-I7 | where、partition、列筛选、flashback、snapshot、weak-read |
| 性能与资源 | EX-I7 | 导出线程、分页、宏块并行、抓取行数、JVM 内存 |
| 高风险确认 | EX-I7 | 二次确认、权限校验、失效规则 |
| 结果清单解析 | EX-I8 | MANIFEST 文件结构化解析 |
| 检查点继续 | EX-I8 | dump.ckpt 解析和新的继续任务创建 |
| 派生任务 | EX-I8 | 基于原配置新建、从头执行 |
| 模板复用 | EX-I8 | 从成功任务保存模板、从模板创建草稿 |
| `--query-sql` 决策 | EX-D1 | 与"不得提供任意 SQL"安全边界的契约冲突，未解决前保持 BLOCKED |

### 3.4 必须保留的安全边界

| 安全约束 | 说明 | 追溯 |
|---|---|---|
| 身份隔离 | 浏览器域和 Agent 域完全隔离 | `AGENTS.md` 第 5 节 |
| 秘密不入存储 | 密码/凭据原值不进入 argv、日志、审计、SQLite、Git | `credential-access-security-contract.md` |
| 唯一命令权威 | 命令生成器是唯一命令拼装入口 | `parameter-command-contract.md` |
| 控制面不操作节点 | 控制面不直接操作执行节点文件或进程 | `AGENTS.md` 第 5 节 |
| Agent 只上报 | Agent 不决定产品参数或调度策略 | `agent-task-state-contract.md` |
| 固定能力约束 | 不提供任意命令/SQL/文件浏览/远程 Shell | `AGENTS.md` 第 5 节 |
| 失败关闭 | 认证/授权/CSRF/密钥/安全上下文不可用时失败关闭 | `AGENTS.md` 第 5 节 |
| 无权 404 | 无权对象不得通过状态、数量或错误差异被发现 | `AGENTS.md` 第 5 节 |
| JDBC 探针固定 | 只加载已核验 `oceanbase-client-2.4.14.jar`，固定执行连接与基础元信息读取 | `AGENTS.md` 第 5 节 |
| 平台不转换 | Windows 盘符路径和 Linux `/` 路径原样传递，不跨系统转换 | `AGENTS.md` 第 6 节 |

---

## 4. 盘点结论

### 4.1 CSV_SINGLE_TABLE_V1 已实现的完整链路

数据源选择（已启用 + JDBC 连接测试 SUCCEEDED）→ 单库单表 CSV 草稿创建 → 参数元数据 v5 命令生成 → 脱敏命令预览 → 六项固定预检查（Agent 领取/执行/上报）→ 任务提交（不可变快照 + 计划命令）→ Agent 领取执行（直接 Java 启动 OBDUMPER）→ 双层脱敏日志（SSE 实时流 + 持久化）→ 终态（SUCCEEDED/FAILED + reconciliationRequired）→ 任务中心授权列表 + 四投影详情 + 日志读取。

### 4.2 已验证的真实证据

| 验证项 | 状态 | 证据 |
|---|---|---|
| Windows 已授权单表 CSV 导出 | 已通过 | `evidence/windows-authorized-real-export-2026-08-04.md` |
| Windows Agent 中断与恢复 | 已通过 | `evidence/windows-agent-interruption-recovery-validation-2026-08-04.md` |
| Windows 终态日志补传恢复 | 已通过 | `evidence/windows-terminal-log-replay-recovery-validation-2026-08-04.md` |
| WI-03 低权限对象访问负例 | 已通过 | `evidence/windows-low-privilege-object-access-validation-2026-08-05.md` |
| WI-04 CSV 特殊值 | 已通过 | `evidence/windows-csv-special-values-validation-2026-08-05.md` |
| WI-05 命令与 argv 取证 | 未通过（暂停） | `evidence/windows-wi05-command-argv-validation-2026-08-05.md` |
| G3 完整通过 | 未通过 | WI-05 暂停、长期不可达、跨目标环境、正式认证/备份恢复待完成 |

### 4.3 后续动作

EX-D0 盘点完成后，下一步是 **EX-D1 完整产品设计**，按任务地图第 4 节顺序执行。EX-D1 的前置条件是本盘点文档的全部结论可追溯且兼容边界已冻结。
