# 通用导出模块技术契约

> 文档状态：EX-D2 技术契约产出
> 创建日期：2026-08-05
> 依据：[开发任务地图](development-task-map.md) 第 5.2 节、[V1.0 支持矩阵](../02-design/export-v1-support-matrix.md)
> 关联契约：[API/SQLite](api-sqlite-data-contract.md)、[参数/命令](parameter-command-contract.md)、[Agent/状态](agent-task-state-contract.md)、[工具启动](tool-launch-isolation-contract.md)、[凭据/安全](credential-access-security-contract.md)、[日志/证据](log-collection-evidence-contract.md)
> 安全说明：不记录真实端点、身份、密码、密钥、完整命令或工具原始输出

## 0. 文档目的

本文是 EX-D2 完整技术契约的唯一产出。它将首条切片（单表 CSV）的六个专项契约泛化为覆盖 OBDUMPER 4.3.5 V1.0 全部导出能力的通用技术框架。首条切片契约继续作为已验证基线保留证据价值，本文在其基础上扩展，不替代或回退已确认的安全边界。

本文同时驱动以下代码同步变更：
- 参数元数据 v6 设计稿（`internal/parammeta/drafts/obdumper-4.3.5-slice-v6.draft.json`，未接入运行时，接入前需完成事实核验与加载器支持）
- 迁移脚本（`0014_export_generalization.sql`）
- Go 类型定义（`internal/store/types.go`）
- OpenAPI schema（`contracts/openapi.json`）

---

## 1. 通用导出领域模型

### 1.1 设计原则

1. **不以页面 JSON 或命令字符串充当领域模型**：导出配置、规范化结果和提交快照各有独立的结构化类型，不等同于前端表单或 CLI argv。
2. **版本化不可变**：提交快照一旦冻结不得被外部数据回写；参数元数据版本发布后不得原地改写。
3. **安全边界不降级**：泛化不扩大 Agent 能力、不引入任意命令/SQL/URI、不降低秘密处理标准。
4. **向后兼容**：现有 CSV 单表草稿、快照和任务在结构泛化后仍可正确读取和执行。
5. **能力切片驱动**：通过 capabilityVersion 区分不同导出能力，每个切片有独立的参数子集、预检查集和结果模型。

### 1.2 ExportConfig（通用导出配置）

替代当前 config_json 中的 CSV 单表硬编码结构（database/table/format/filePath/logPath/skipCheckDir）。

```text
ExportConfig {
  objectScope:      ObjectScope        // 对象范围
  contentSelection: ContentSelection   // 导出内容
  dataFormat:       DataFormat         // 数据格式及专属参数
  outputConfig:     OutputConfig       // 输出位置与文件布局
  performanceConfig: PerformanceConfig // 性能与资源
  filterConfig:     FilterConfig       // 筛选与一致性
  ddlBehavior:      DDLBehavior        // DDL 行为
}
```

**ObjectScope（对象范围）**

```text
ObjectScope {
  scopeKind:       ALL | SPECIFIED     // 全部/指定
  objectTypes:     [ObjectType]        // 对象类型列表（表/视图/触发器/...）
  expressions:     [ObjectExpression]  // 对象表达式（含可选 schema 前缀）
  excludeTables:   [string]            // 排除表表达式
}

ObjectType = TABLE | TABLE_GROUP | VIEW | TRIGGER | USER | ROLE
           | SEQUENCE | SYNONYM | TYPE | TYPE_BODY | PACKAGE
           | PACKAGE_BODY | FUNCTION | PROCEDURE

ObjectExpression {
  schema:    string?    // 可选 schema 前缀（多库模式标识）
  name:      string     // 对象名称或通配表达式
  rawInput:  string     // 控制面按 schema.name 生成的规范值；浏览器自由文本不得持久化，避免借此写入秘密或任意内容
}
```

**ContentSelection（导出内容）**

```text
ContentSelection {
  contentKind:  DDL_ONLY | DATA_ONLY | DDL_AND_DATA
}
```

**DataFormat（数据格式及专属参数）**

```text
DataFormat {
  formatKind:     CSV | CUT | POS | SQL | PARQUET | ORC | AVRO
  csvOptions:     CsvOptions?
  cutOptions:     CutOptions?
  posOptions:     PosOptions?
  textOptions:    TextSerializationOptions?
  dateTimeOptions: DateTimeOptions?
  compression:    CompressionOptions?
}
```

各 Options 结构映射到对应参数的 EX-F 字段，每个可选字段携带 `fieldState`（UNSET/EXPLICIT/DERIVED/INACTIVE/BLOCKED）。

**OutputConfig（输出配置）**

```text
OutputConfig {
  outputKind:      LOCAL | OSS | S3 | COS | OBS
  filePath:        string         // 本地绝对路径或受控 URI
  logPath:         string?
  skipCheckDir:    bool
  noNestedDir:     bool
  ctlPath:         string?        // 控制文件目录
  tmpPath:         string?        // 对象存储临时目录
  maxFileSize:     int64?         // 字节
  retainEmptyFiles: bool
  storageCredential: StorageCredentialRef?  // 对象存储凭据引用
}
```

**PerformanceConfig（性能与资源）**

```text
PerformanceConfig {
  thread:         int?
  pageSize:        int?
  parallelMacro:  int?
  fetchSize:      int?
  jvmMemory:      string?        // K/M/G/T
  retry:          bool           // 检查点继续
}
```

**FilterConfig（筛选与一致性）**

```text
FilterConfig {
  querySql:            string?   // 受限专家能力
  where:               string?
  partition:           string?
  includeColumnNames:  [string]?
  excludeColumnNames:  [string]?
  excludeDataTypes:    [string]?
  excludeVirtualColumns: bool?
  enableHiddenPk:      bool?
  flashbackScn:        int64?
  flashbackTimestamp:   string?
  snapshot:            string?
  weakRead:            bool?
}
```

**DDLBehavior（DDL 行为）**

```text
DDLBehavior {
  dropObject:        bool?
  addExtraMessage:   bool?
  retainSchema:      bool?
  compactSchema:     bool?
  sequencePolicy:    RESTART | PRESERVE?
}
```

### 1.3 NormalizedConfig（规范化配置）

草稿 ExportConfig + 派生事实经过类型转换、活动计算和校验后形成的唯一结构化结果。

```text
NormalizedConfig {
  config:             ExportConfig           // 活动字段子集
  derivedFacts:       DerivedFacts           // 数据源/节点/安全派生值
  activeParameters:   [ActiveParameter]      // 活动参数有序列表
  blockedParameters:  [BlockedParameter]     // 阻断参数及原因
  inactiveFields:     [FieldReference]       // 非活动字段
  validationErrors:   [ValidationError]      // 校验错误
}

ActiveParameter {
  definitionId:   string
  longName:       string
  value:          ParameterValue
  source:         EXPLICIT | DERIVED | DEFAULT
  emissionTarget: ARGV | SECURITY_FILE
}
```

NormalizedConfig 由命令生成器（commandgen）在规范化阶段产生，是提交快照和命令生成的唯一输入。它提升当前 `commandgen.NormalizedFields` 为独立领域类型，但保持向后兼容。

### 1.4 SubmissionSnapshot（提交快照）

泛化当前 tasks 表的 frozen_snapshot。

```text
SubmissionSnapshot {
  config:              NormalizedConfig
  toolVersion:         string
  metadataVersion:     string
  capabilityVersion:   string
  dataSourceId:        string
  dataSourceSnapshot:  DataSourceSnapshot
  nodeId:              string
  precheckId:          string
  configFingerprint:   string        // SHA-256
  plannedArgv:         [string]      // 计划参数令牌
  secretSlots:         [SecretSlot]  // 秘密槽位引用
  riskConfirmations:   [RiskConfirmation]
  submittedAt:         RFC3339
  submittedBy:         string        // subjectId
  parentTaskId:        string?       // 检查点继续
  derivedFromTaskId:   string?       // 基于原配置新建
  templateId:          string?       // 模板来源
}
```

### 1.5 CapabilityVersion 体系

从 `export-odp-single-table-csv-v1` 泛化为结构化命名：

```text
export-odp-{scope}-{format}-v{N}
```

| capabilityVersion | 含义 | V1.0 状态 |
|---|---|---|
| `export-odp-single-table-csv-v1` | 单表 CSV（已冻结基线） | ENABLED（已实现） |
| `export-odp-full-csv-v1` | 全对象 CSV（含 all/多表/多对象类型） | 设计完成，待实现 |
| `export-odp-ddl-v1` | 纯 DDL 导出 | 设计完成，待实现 |
| `export-odp-ddl-csv-v1` | DDL + CSV 数据 | 设计完成，待实现 |
| `export-odp-cut-v1` | CUT 格式 | 设计完成，待实现 |
| `export-odp-sql-v1` | Insert SQL 格式 | 设计完成，待实现 |
| `export-odp-parquet-v1` | Parquet 格式 | VALIDATION_GATED |
| `export-odp-orc-v1` | ORC 格式 | VALIDATION_GATED |
| `export-odp-avro-v1` | Avro 格式 | VALIDATION_GATED |
| `export-odp-pos-v1` | POS 定长格式 | VALIDATION_GATED |

capabilityVersion 决定：
- 哪些参数子集参与活动
- 哪些预检查项适用
- 结果模型的结构
- 命令生成模板的选择

### 1.6 DerivedTaskRelation（派生任务关系）

```text
DerivedTaskRelation {
  kind:          CHECKPOINT_RESUME | CONFIG_COPY | TEMPLATE_DERIVED
  parentTaskId:  string?           // CHECKPOINT_RESUME
  sourceTaskId:  string?           // CONFIG_COPY
  templateId:    string?           // TEMPLATE_DERIVED
  checkpointRef: string?           // dump.ckpt 路径引用
  configDiff:    ConfigDiff?       // 与原配置的差异
}
```

检查点继续约束：
- 原参数、输出路径和工具版本不变
- 使用 `--retry` 参数
- 创建新 taskId，保持原快照引用
- 不满足条件时不提供继续选项

---

## 2. 向后兼容策略

### 2.1 草稿兼容

现有 export_drafts 表的 config_json 保持 v5 结构可读。新草稿使用泛化 ExportConfig 结构，通过 `config_version` 列区分。控制面读取时按 config_version 自动适配解析器。

### 2.2 快照兼容

现有 tasks 表的 snapshot_json 保持 v1 结构可读。新快照使用泛化 SubmissionSnapshot 结构，通过 `snapshot_version` 列区分。

### 2.3 参数元数据兼容

v5 元数据继续作为 `export-odp-single-table-csv-v1` 的冻结基线。v6 扩展为全参数集，同时标记每个参数属于哪些 capabilityVersion 切片。旧任务的命令生成仍使用 v5 元数据版本。

### 2.4 命令生成兼容

令生成器按 metadataVersion 选择参数定义集，按 capabilityVersion 选择活动参数子集和命令模板。现有 CSV 单表命令生成路径不变。

---

## 3. OpenAPI 与 SQLite 契约

### 3.1 迁移脚本 0014_export_generalization.sql

```sql
-- 通用导出领域模型扩展
-- 向后兼容：v5 草稿和 v1 快照保持可读

-- 草稿表：新增结构化列 + 版本标识
ALTER TABLE export_drafts ADD COLUMN config_version TEXT NOT NULL DEFAULT 'v5';
ALTER TABLE export_drafts ADD COLUMN object_scope_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE export_drafts ADD COLUMN content_selection_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE export_drafts ADD COLUMN data_format_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE export_drafts ADD COLUMN output_config_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE export_drafts ADD COLUMN performance_config_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE export_drafts ADD COLUMN filter_config_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE export_drafts ADD COLUMN ddl_behavior_json TEXT NOT NULL DEFAULT '{}';

-- 任务表：快照泛化 + 派生关系
ALTER TABLE tasks ADD COLUMN snapshot_version TEXT NOT NULL DEFAULT 'v1';
ALTER TABLE tasks ADD COLUMN parent_task_id TEXT DEFAULT NULL
    REFERENCES tasks(task_id);
ALTER TABLE tasks ADD COLUMN derived_from_task_id TEXT DEFAULT NULL
    REFERENCES tasks(task_id);
ALTER TABLE tasks ADD COLUMN template_id TEXT DEFAULT NULL;

-- 索引：派生任务关系查询
CREATE INDEX idx_tasks_parent_task ON tasks(parent_task_id)
    WHERE parent_task_id IS NOT NULL;
CREATE INDEX idx_tasks_derived_from ON tasks(derived_from_task_id)
    WHERE derived_from_task_id IS NOT NULL;
CREATE INDEX idx_tasks_template ON tasks(template_id)
    WHERE template_id IS NOT NULL;

-- 导出配置模板表（从成功任务保存，可创建新草稿）
CREATE TABLE IF NOT EXISTS export_config_templates (
    template_id       TEXT PRIMARY KEY,
    owner_subject_id  TEXT NOT NULL,
    display_name      TEXT NOT NULL,
    capability_version TEXT NOT NULL,
    config_json       TEXT NOT NULL,
    config_fingerprint TEXT NOT NULL,
    source_task_id    TEXT,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    revision          INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE INDEX idx_templates_owner ON export_config_templates(owner_subject_id);
CREATE INDEX idx_templates_capability ON export_config_templates(capability_version);
```

### 3.2 乐观锁与幂等

- export_drafts 保持 `revision` + `If-Match` 乐观锁
- export_config_templates 同样使用 `revision` + `If-Match`
- tasks 的派生关系字段（parent_task_id / derived_from_task_id / template_id）在创建时冻结，不可更新
- 幂等键（idempotency_key）机制不变，新表模板创建同样需要幂等保护

### 3.3 对象授权规则

| 操作 | 授权条件 |
|---|---|
| 创建/更新草稿 | 数据源 + 节点在主体范围内 |
| 提交草稿 | 草稿 + 预检查 + 数据源 + 节点完整绑定 |
| 查看模板 | owner_subject_id 匹配或 TASK_OPERATE_BY_DATA_SOURCE |
| 从模板创建草稿 | 模板 owner_subject_id 匹配 |
| 检查点继续 | 原任务 creator_subject_id + 检查点兼容 |

### 3.4 OpenAPI schema 变更

**ExportDraftWrite 泛化**（向后兼容 v5）：

```json
{
  "configVersion": "v5 | v6",
  "dataSourceId": "OpaqueId",
  "nodeId": "OpaqueId",
  "config": {
    "objectScope": { "..." : "..." },
    "contentSelection": { "..." : "..." },
    "dataFormat": { "..." : "..." },
    "outputConfig": { "..." : "..." },
    "performanceConfig": { "..." : "..." },
    "filterConfig": { "..." : "..." },
    "ddlBehavior": { "..." : "..." }
  }
}
```

v5 请求继续使用 `database/table/format/filePath/logPath/skipCheckDir/csvOptions` 扁平结构。v6 请求使用嵌套 `config` 对象。控制面按 configVersion 路由解析。

**TaskSnapshotRead 泛化**：

```json
{
  "snapshotVersion": "v1 | v2",
  "type": "OBDUMPER_EXPORT",
  "dataSourceId": "OpaqueId",
  "nodeId": "OpaqueId",
  "precheckId": "OpaqueId",
  "objectSummary": "database.table 或 scope 摘要",
  "format": "CSV | CUT | SQL | DDL | ...",
  "configFingerprint": "SHA-256",
  "toolVersion": "string",
  "metadataVersion": "string",
  "capabilityVersion": "export-odp-{scope}-{format}-v{N}",
  "parentTaskId": "OpaqueId?",
  "derivedFromTaskId": "OpaqueId?",
  "templateId": "OpaqueId?"
}
```

**ExportConfigTemplate（新增）**：

```json
{
  "templateId": "OpaqueId",
  "displayName": "string",
  "capabilityVersion": "string",
  "config": "ExportConfig (v6)",
  "configFingerprint": "SHA-256",
  "sourceTaskId": "OpaqueId?",
  "revision": "integer"
}
```

---

## 4. Go 类型定义同步

### 4.1 ExportConfig 及子结构（internal/store/types.go 新增）

```go
// ExportConfig 是通用导出配置的结构化领域模型。
// 它不等同于前端表单 JSON 或 CLI argv，而是经过类型安全的独立抽象。
type ExportConfig struct {
    ObjectScope       ObjectScope       `json:"objectScope"`
    ContentSelection  ContentSelection  `json:"contentSelection"`
    DataFormat        DataFormat        `json:"dataFormat"`
    OutputConfig      OutputConfig      `json:"outputConfig"`
    PerformanceConfig PerformanceConfig `json:"performanceConfig"`
    FilterConfig      FilterConfig      `json:"filterConfig"`
    DDLBehavior       DDLBehavior       `json:"ddlBehavior"`
}

type ObjectScope struct {
    ScopeKind      string             `json:"scopeKind"`      // ALL | SPECIFIED
    ObjectTypes    []string           `json:"objectTypes"`
    Expressions    []ObjectExpression `json:"expressions"`
    ExcludeTables  []string           `json:"excludeTables"`
}

type ObjectExpression struct {
    Schema   string `json:"schema,omitempty"`
    Name     string `json:"name"`
    RawInput string `json:"rawInput"`
}

type ContentSelection struct {
    ContentKind string `json:"contentKind"` // DDL_ONLY | DATA_ONLY | DDL_AND_DATA
}

type DataFormat struct {
    FormatKind      string `json:"formatKind"` // CSV | CUT | POS | SQL | PARQUET | ORC | AVRO
    CsvOptions      map[string]interface{} `json:"csvOptions,omitempty"`
    CutOptions      map[string]interface{} `json:"cutOptions,omitempty"`
    PosOptions      map[string]interface{} `json:"posOptions,omitempty"`
    TextOptions     map[string]interface{} `json:"textOptions,omitempty"`
    DateTimeOptions map[string]interface{} `json:"dateTimeOptions,omitempty"`
    Compression     map[string]interface{} `json:"compression,omitempty"`
}

type OutputConfig struct {
    OutputKind        string `json:"outputKind"` // LOCAL | OSS | S3 | COS | OBS
    FilePath          string `json:"filePath"`
    LogPath           string `json:"logPath,omitempty"`
    SkipCheckDir      bool   `json:"skipCheckDir"`
    NoNestedDir       bool   `json:"noNestedDir"`
    CtlPath           string `json:"ctlPath,omitempty"`
    TmpPath           string `json:"tmpPath,omitempty"`
    MaxFileSize       *int64 `json:"maxFileSize,omitempty"`
    RetainEmptyFiles  bool   `json:"retainEmptyFiles"`
    StorageCredentialRef string `json:"storageCredentialRef,omitempty"`
}

type PerformanceConfig struct {
    Thread        *int   `json:"thread,omitempty"`
    PageSize      *int   `json:"pageSize,omitempty"`
    ParallelMacro *int   `json:"parallelMacro,omitempty"`
    FetchSize     *int   `json:"fetchSize,omitempty"`
    JvmMemory     string `json:"jvmMemory,omitempty"`
    Retry         bool   `json:"retry"`
}

type FilterConfig struct {
    QuerySql           string   `json:"querySql,omitempty"`
    Where              string   `json:"where,omitempty"`
    Partition          string   `json:"partition,omitempty"`
    IncludeColumnNames []string `json:"includeColumnNames,omitempty"`
    ExcludeColumnNames []string `json:"excludeColumnNames,omitempty"`
    ExcludeDataTypes   []string `json:"excludeDataTypes,omitempty"`
    ExcludeVirtualColumns *bool `json:"excludeVirtualColumns,omitempty"`
    EnableHiddenPk     *bool    `json:"enableHiddenPk,omitempty"`
    FlashbackScn       *int64   `json:"flashbackScn,omitempty"`
    FlashbackTimestamp string   `json:"flashbackTimestamp,omitempty"`
    Snapshot           string   `json:"snapshot,omitempty"`
    WeakRead           *bool    `json:"weakRead,omitempty"`
}

type DDLBehavior struct {
    DropObject      *bool  `json:"dropObject,omitempty"`
    AddExtraMessage *bool  `json:"addExtraMessage,omitempty"`
    RetainSchema    *bool  `json:"retainSchema,omitempty"`
    CompactSchema   *bool  `json:"compactSchema,omitempty"`
    SequencePolicy  string `json:"sequencePolicy,omitempty"`
}
```

### 4.2 SubmissionSnapshot 扩展（internal/store/types.go 新增）

```go
// SubmissionSnapshot 是泛化后的提交快照领域模型。
// 它替代当前 snapshot_json 中的硬编码结构，同时保持 v1 快照向后兼容。
type SubmissionSnapshot struct {
    Config             NormalizedConfig `json:"config"`
    ToolVersion        string           `json:"toolVersion"`
    MetadataVersion    string           `json:"metadataVersion"`
    CapabilityVersion  string           `json:"capabilityVersion"`
    DataSourceID       string           `json:"dataSourceId"`
    NodeID             string           `json:"nodeId"`
    PrecheckID         string           `json:"precheckId"`
    ConfigFingerprint  string           `json:"configFingerprint"`
    PlannedArgv        []string         `json:"plannedArgv"`
    SecretSlots        []SecretSlot     `json:"secretSlots"`
    RiskConfirmations  []RiskConfirmation `json:"riskConfirmations"`
    SubmittedAt        string           `json:"submittedAt"`
    SubmittedBy        string           `json:"submittedBy"`
    ParentTaskID       string           `json:"parentTaskId,omitempty"`
    DerivedFromTaskID  string           `json:"derivedFromTaskId,omitempty"`
    TemplateID         string           `json:"templateId,omitempty"`
}
```

### 4.3 ExportConfigTemplate（internal/store/types.go 新增）

```go
// ExportConfigTemplate 是从成功任务保存的导出配置模板。
// 可用于创建新草稿，复用已验证的配置结构。
type ExportConfigTemplate struct {
    TemplateID        string `json:"templateId"`
    OwnerSubjectID    string `json:"ownerSubjectId"`
    DisplayName       string `json:"displayName"`
    CapabilityVersion string `json:"capabilityVersion"`
    ConfigJSON        string `json:"configJson"`
    ConfigFingerprint string `json:"configFingerprint"`
    SourceTaskID      string `json:"sourceTaskId,omitempty"`
    Revision          int64  `json:"revision"`
    CreatedAt         time.Time `json:"createdAt"`
    UpdatedAt         time.Time `json:"updatedAt"`
}
```

---

## 5. 参数元数据泛化

### 5.1 参数元数据 v6 结构

> 状态：v6 目前是**设计稿**，位于 `internal/parammeta/drafts/`，不被运行时加载器嵌入或解析。其内容包含尚未完成事实核验的声明（如 `--sys-password` 的官方安全文件属性未取证、部分参数状态与定义数与正文矩阵存在出入）。对应能力切片（EX-I2 及以后）接入前，必须先修正这些出入、补齐加载器支持与测试，并保持失败关闭。

v6 从 v5 的 18 参数（overrides + additions）扩展为覆盖全部 63 个 ENABLED 参数的完整定义集。v6 继续使用 v1 作为基础版本，通过 override/addition 模式增量更新。

**v6 新增字段**（每个参数）：
- `v1State`：ENABLED | VALIDATION_GATED | HIDDEN | BLOCKED
- `sensitivity`：NORMAL | IDENTIFIER | SECRET
- `riskLevel`：LOW | MEDIUM | HIGH
- `evidenceState`：VERIFIED | OFFICIAL_ONLY | CONFLICT_PENDING | HELP_ONLY
- `capabilityVersions`：该参数适用的能力切片列表
- `emissionTarget`：ARGV | SECURITY_FILE（从 v5 overrides 提升为定义级字段）

**v6 分类体系**（11 类）：

| 序号 | 分类标识 | 含义 | ENABLED 参数数 |
|---|---|---|---|
| 1 | CONNECTION | 连接与会话 | 12 |
| 2 | DATABASE_CONNECTION | 数据库连接 | 1 |
| 3 | OBJECT_SCOPE | 对象范围 | 2 |
| 4 | CONTENT_FORMAT | 内容与数据格式 | 5 |
| 5 | FORMAT_SERIALIZATION | 文本/CSV/CUT 序列化 | 11 |
| 6 | DATE_TIME | 日期时间序列化 | 0（全部 VG） |
| 7 | OUTPUT_FILE | 输出与文件 | 6 |
| 8 | DATABASE_OBJECT | 数据库对象 | 1 |
| 9 | DATA_FILTER | 数据筛选与一致性 | 7 |
| 10 | DDL_BEHAVIOR | DDL 与对象处理 | 3 |
| 11 | PERFORMANCE | 性能与资源 + 压缩 | 7 |

### 5.2 capabilityVersion 参数子集

每个 capabilityVersion 激活不同参数子集：

| capabilityVersion | 必须参数 | 格式参数 | 特殊参数 |
|---|---|---|---|
| `export-odp-single-table-csv-v1` | host/port/user/password/database/table | csv + csvOptions | file-path |
| `export-odp-full-csv-v1` | host/port/user/password/database | csv + csvOptions | all/table/exclude-table |
| `export-odp-ddl-v1` | host/port/user/password/database | ddl | drop-object/retain-schema |
| `export-odp-ddl-csv-v1` | host/port/user/password/database | ddl + csv | 全 DDL + CSV 参数 |
| `export-odp-cut-v1` | host/port/user/password/database | cut + cutOptions | column-splitter |
| `export-odp-sql-v1` | host/port/user/password/database | sql | — |

### 5.3 命令生成泛化

当前 `generateExportDraft` 中的硬编码参数列表改为参数元数据驱动：

1. 加载当前 metadataVersion 对应的参数定义集（v1 基础 + v6 overrides/additions）
2. 按 capabilityVersion 筛选活动参数子集
3. 按 ExportConfig 映射每个参数的值
4. 执行 activation 条件判断、requiredWhen 校验和 conflictsWith 互斥检查
5. 按 order 排序生成 plannedArgv
6. SECRET 类型参数进入 secretSlots 而非 argv

命令生成器（commandgen）仍是唯一权威，输入从 ExportConfig 映射到 Fields，不改变现有 CSV 单表路径。

---

## 6. 预检查分层

### 6.1 分层框架

从当前固定六项扩展为 11 层预检查框架：

| 序号 | 层 | 检查标识 | 事实来源 | 失效条件 | V1.0 状态 |
|---|---|---|---|---|---|
| 1 | 连接层 | DATABASE_CONNECTIVITY | Agent JDBC 探针 | 数据源/凭据变化 | ENABLED |
| 2 | 对象层 | OBJECT_ACCESS | Agent 数据库查询 | 对象/权限变化 | ENABLED |
| 3 | 权限层 | SYS_PRIVILEGE | Agent sys 查询 | sys 凭据变化 | ENABLED（DDL 场景） |
| 4 | 格式层 | FORMAT_COMPATIBILITY | 控制面参数元数据 | 格式/参数变化 | ENABLED |
| 5 | 输出层 | OUTPUT_PATH | Agent 本机 | 路径/节点变化 | ENABLED |
| 6 | 空间层 | OUTPUT_EMPTY | Agent 本机 | 节点变化 | ENABLED |
| 7 | 存储层 | STORAGE_CONNECTIVITY | Agent 网络探测 | URI/凭据变化 | VALIDATION_GATED |
| 8 | 存储层 | STORAGE_AUTH | Agent 网络探测 | URI/凭据变化 | VALIDATION_GATED |
| 9 | 资源层 | AVAILABLE_SPACE | Agent 本机 | 节点变化 | ENABLED |
| 10 | 资源层 | NODE_RESOURCE | Agent 本机 | 节点变化 | ENABLED |
| 11 | 风险层 | RISK_CONFIRMATION | 控制面 | 风险参数变化 | ENABLED |

### 6.2 执行顺序

1. **控制面本地检查**（无秘密）：FORMAT_COMPATIBILITY → RISK_CONFIRMATION
2. **Agent 无秘密检查**：OUTPUT_PATH → OUTPUT_EMPTY → AVAILABLE_SPACE → NODE_RESOURCE
3. **Agent 秘密检查**（全部无秘密通过后）：DATABASE_CONNECTIVITY → OBJECT_ACCESS → SYS_PRIVILEGE
4. **汇总判定**：全部通过 → SUCCEEDED；任一失败 → FAILED

### 6.3 capabilityVersion 绑定

每个 capabilityVersion 声明适用的预检查层子集：

- `export-odp-single-table-csv-v1`：层 1/2/4/5/6/9/10/11（当前已实现）
- `export-odp-ddl-v1`：层 1/2/3/4/5/6/9/10/11（新增权限层）
- `export-odp-full-csv-v1`：层 1/2/4/5/6/7/8/9/10/11（新增存储层）

### 6.4 local-first 策略保持

预检查保持当前 local-first 策略不变：
- 无秘密检查先执行，不消耗数据库连接资源
- 全部无秘密检查通过后才解析秘密槽位
- 秘密材料只在短租约内存在，完成后立即销毁
- 检查点继续（CHECKPOINT_COMPATIBILITY）作为未来扩展层，V1.0 为 VALIDATION_GATED

---

## 7. Agent 信封泛化

### 7.1 信封分离设计保持

保持当前 precheckEnvelope / executionEnvelope 分离设计不变：

- **precheckEnvelope**：固定预检查专用，包含绑定、租约和检查清单
- **executionEnvelope**：正式导出专用，包含快照、计划参数和秘密槽位引用

### 7.2 payloadType 泛化

信封 payloadType 从固定枚举扩展为 capabilityVersion 驱动：

| payloadType | 含义 | 信封类型 | V1.0 状态 |
|---|---|---|---|
| EXPORT_PRECHECK | 固定预检查 | precheckEnvelope | ENABLED |
| EXPORT_EXECUTION_SINGLE_TABLE_CSV | 单表 CSV 执行 | executionEnvelope | ENABLED |
| EXPORT_EXECUTION_FULL_CSV | 全对象 CSV 执行 | executionEnvelope | 设计完成 |
| EXPORT_EXECUTION_DDL | 纯 DDL 执行 | executionEnvelope | 设计完成 |
| EXPORT_EXECUTION_DDL_CSV | DDL + CSV 执行 | executionEnvelope | 设计完成 |
| EXPORT_EXECUTION_MULTI_FORMAT | 多格式执行 | executionEnvelope | 设计完成 |
| EXPORT_CHECKPOINT_RESUME | 检查点继续 | executionEnvelope | VALIDATION_GATED |

### 7.3 信封禁止内容

信封不得包含以下内容（安全边界不降级）：
- 任意命令（Agent 只能启动预登记的 OBDUMPER 工具）
- 任意 SQL（信封不携带用户查询语句）
- 任意 URI（信封不携带网络地址）
- 控制面入站控制（信封不能反向影响控制面状态）

### 7.4 SecretSlot 扩展

从单一 DATABASE_CONNECTION 扩展为：

| SecretSlot 类型 | 用途 | V1.0 状态 |
|---|---|---|
| DATABASE_CONNECTION | 数据库密码 | ENABLED |
| SYS_DATABASE_CONNECTION | sys 密码（DDL 场景） | ENABLED（DDL 切片） |
| STORAGE_CREDENTIAL | 对象存储凭据（OSS/S3/COS/OBS） | VALIDATION_GATED |

每个 SecretSlot 保持当前生命周期：创建时加密 → 短租约解析 → 使用后销毁。

---

## 8. 结果、清单、检查点和终态

### 8.1 结果事实（ExportResult）

从当前仅核对 CSV 文件存在扩展为结构化 ExportResult：

```text
ExportResult {
    format:          string               // CSV | CUT | SQL | DDL | ...
    files:           [ExportFile]         // 文件清单
    totalRows:       int64                // 总行数
    totalBytes:      int64                // 总字节数
    objectCount:     int                  // 导出对象数
    duration:        Duration             // 执行时长
    formatFeatures:  map[string]string    // 格式特征（压缩、编码等）
}

ExportFile {
    relativePath:    string
    sizeBytes:       int64
    rowCount:        int64
    format:          string
    compression:     string?
    checksum:        string?              // SHA-256
}
```

### 8.2 清单（MANIFEST）

定义通用清单结构，每种格式有独立的文件命名规则：

```text
ExportManifest {
    manifestVersion:   string
    capabilityVersion: string
    taskId:            string
    completedAt:       RFC3339
    toolVersion:       string
    objects:           [ManifestObject]
}

ManifestObject {
    objectType:    string    // TABLE | VIEW | ...
    schema:        string
    name:          string
    files:         [string]  // 相对路径列表
    rowCount:      int64
    sizeBytes:     int64
}
```

文件命名规则：
- CSV/CUT/SQL：`{schema}_{objectType}_{name}/data_{seq}.csv`
- DDL：`{schema}_{objectType}_{name}/schema.sql`
- 压缩后缀追加：`data_{seq}.csv.gz`

### 8.3 检查点（dump.ckpt）

检查点兼容性判断条件：
- 参数集合不变（configFingerprint 相同）
- 输出路径不变
- 工具版本不变
- 检查点文件存在且可读

检查点继续任务的快照继承规则：
- 创建新 taskId，parentTaskId 指向原任务
- 继承原 SubmissionSnapshot，追加 `--retry` 参数
- 不重新执行预检查（检查点内含继续语义）
- 不满足条件时不提供继续选项

### 8.4 终态证据

明确不以包装脚本退出码单独判断成功，必须结合：

1. **受控进程退出码**：OBDUMPER 进程本身的退出码
2. **工具终态日志**：工具输出的终态证据（完成/失败/中断）
3. **结果文件/MANIFEST 事实**：文件存在且符合预期

| 终态 | 退出码 | 工具日志 | 文件事实 | 任务状态 |
|---|---|---|---|---|
| 完全成功 | 0 | COMPLETED | MANIFEST 完整 | SUCCEEDED |
| 部分成功 | 0 | COMPLETED_WITH_WARNINGS | MANIFEST 部分 | SUCCEEDED（带警告） |
| 工具失败 | 非 0 | FAILED | 不完整 | FAILED |
| 中断 | 任意 | ABORTED | dump.ckpt 存在 | INTERRUPTED（可继续） |
| 未知 | 任意 | 缺失 | 不确定 | REQUIRES_RECONCILIATION |

---

## 9. 测试策略

每个能力切片（EX-I1~EX-I8）的测试矩阵：

### 9.1 测试维度

| 维度 | 测试类型 | 描述 |
|---|---|---|
| 正例 | 合法配置 → 命令生成 → 预检查通过 → 任务完成 | 完整成功路径 |
| 权限负例 | 无权数据源/节点/对象 → 404/403 | 授权边界 |
| 非法组合 | 互斥参数/格式/对象类型冲突 → 阻断 | 参数校验 |
| 重复/乱序/重启 | 幂等键重复、事件乱序、Agent 重启 → 状态正确 | 状态机鲁棒性 |
| 秘密泄露 | 密码/密钥不出现在响应/日志/快照/错误中 | 安全边界 |
| 跨平台路径 | Windows 盘符绝对路径 / Linux 路径 → 原样传递 | 路径处理 |

### 9.2 切片与测试矩阵

| 切片 | capabilityVersion | 测试重点 |
|---|---|---|
| EX-I1 | export-odp-single-table-csv-v1 | 已实现基线，回归保护 |
| EX-I2 | export-odp-full-csv-v1 | all/多表/排除表 + 对象授权 |
| EX-I3 | export-odp-ddl-v1 | 权限层检查 + DDL 参数组合 |
| EX-I4 | export-odp-ddl-csv-v1 | DDL + CSV 参数联合激活 |
| EX-I5 | export-odp-cut-v1 | CUT 序列化 + column-splitter |
| EX-I6 | export-odp-sql-v1 | Insert SQL 格式 |
| EX-I7 | export-odp-pos-v1 | POS 定长格式 + ctl-path |
| EX-I8 | 压缩/对象存储/检查点 | 跨切片能力验证 |

### 9.3 测试约束

- 所有测试必须在不连接真实数据库、不启动 OBDUMPER 的条件下完成
- 命令生成测试验证 argv 结构和秘密槽位引用，不执行实际命令
- 预检查测试使用模拟 Agent 回执，不连接实际端点
- 安全测试覆盖全部秘密路径（响应/日志/快照/错误消息）
- 跨平台路径测试覆盖 Windows（盘符 + UNC）和 Linux 绝对路径

---

## 10. 关联文件索引

| 文件 | 说明 |
|---|---|
| `docs/03-technical/export-general-contract.md` | 本文（通用导出技术契约） |
| `internal/parammeta/drafts/obdumper-4.3.5-slice-v6.draft.json` | 参数元数据 v6 设计稿（未接入运行时） |
| `migrations/0014_export_generalization.sql` | 通用导出领域模型迁移 |
| `internal/store/types.go` | 泛化导出配置类型 |
| `contracts/openapi.json` | 导出草稿和任务快照 schema |
