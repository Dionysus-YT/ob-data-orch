# 通用导出模块技术契约

> 文档状态：Export v1 技术 Canonical（EX-D2 产出持续维护）
> 创建日期：2026-08-05
> 最近更新：2026-08-22
> 依据：[开发任务地图](development-task-map.md) 第 5.2 节、[导出模块 Canonical](../02-design/export-module.md)
> 关联契约：[API/SQLite](api-sqlite-data-contract.md)、[参数/命令](parameter-command-contract.md)、[Agent/状态](agent-task-state-contract.md)、[工具启动](tool-launch-isolation-contract.md)、[凭据/安全](credential-access-security-contract.md)、[日志/证据](log-collection-evidence-contract.md)
> 安全说明：不记录真实端点、身份、密码、密钥、完整命令或工具原始输出

## 0. 文档目的

本文是 EX-D2 完整技术契约的唯一产出。它将首条切片（单表 CSV）的六个专项契约泛化为覆盖 OBDUMPER 4.3.5 V1.0 全部导出能力的通用技术框架。首条切片契约继续作为已验证基线保留证据价值，本文在其基础上扩展，不替代或回退已确认的安全边界。

本文同时驱动以下代码同步变更：
- 参数元数据设计稿与受控运行时版本链（`internal/parammeta/drafts/`、`internal/parammeta/resources/obdumper-4.3.5-slice-v5.json` 至 `v7.json`）
- 迁移脚本（`0014_export_generalization.sql`）
- Go 类型定义（`internal/store/types.go`）
- OpenAPI schema（`contracts/openapi.json`）

### 0.1 2026-08-22 目标契约修订

本节是已确认的下一运行时版本目标，优先级高于本文中与之冲突的历史实施记录。v5～v7 参数元数据、既有任务快照和运行证据保持不可变；当前代码尚未全部对齐，开发任务地图必须持续标记该差距，不能把本节写成已经交付。

1. 新建向导顺序固定为 `SOURCE → CONTENT → OBJECT → FORMAT → OUTPUT → REVIEW`。页面字段所在步骤是产品呈现策略，不改变领域配置的稳定字段名。
2. 普通 V1 新建格式集合固定为 `CSV | CUT | SQL`。`POS | PARQUET | ORC | AVRO` 继续属于可解析的历史/门控能力，不能通过缩小枚举破坏旧草稿和任务。
3. host、port、组合用户名、密码引用、字符集、逻辑库和会话配置只能来自 `DATA_SOURCE`、`NODE` 或 `SECURITY` 派生事实；浏览器任务表单不得提交同名可编辑字段。
4. `filterConfig.querySql` 的敏感性为 `NORMAL`，使用普通导出任务授权，不要求 `CAP_SENSITIVE_COMMAND`、平台敏感开关、风险指纹或提交级二次确认。服务端仍须拒绝 `file://`，并复验它与 where、partition、flashback SCN、flashback timestamp 的互斥关系。
5. `outputConfig.retainEmptyFiles` 对 `DATA_ONLY`、`DDL_AND_DATA` 的 CSV/CUT/SQL 活动，对 `DDL_ONLY` 不活动；它不依赖 where 或 partition 才能配置。CSV 与 skipHeader 同时决定空文件是否保留表头。
6. MySQL/Oracle 不适用项由同一参数元数据投影为“可见但禁用 + 原因”；浏览器提示不是可信校验，服务端规范化仍按兼容模式、数据库版本、权限和证据状态失败关闭。
7. 每个业务步骤最多一个高级设置容器；分类只用于页面组织和错误定位，不进入命令指纹。第 1 步没有任务级高级字段，第 6 步没有可编辑字段。

---

## 1. 通用导出领域模型

### 1.1 设计原则

1. **不以页面 JSON 或命令字符串充当领域模型**：导出配置、规范化结果和提交快照各有独立的结构化类型，不等同于前端表单或 CLI argv。
2. **版本化不可变**：提交快照一旦冻结不得被外部数据回写；参数元数据版本发布后不得原地改写。
3. **安全边界不降级**：泛化不扩大 Agent 能力、不引入任意命令、通用 SQL 执行通道或任意 URI；OBDUMPER 官方 `--query-sql` 只能经类型化字段和唯一命令生成器进入固定导出信封。
4. **向后兼容**：现有 CSV 单表草稿、快照和任务在结构泛化后仍可正确读取和执行。
5. **能力切片驱动**：通过 capabilityVersion 区分不同导出能力，每个切片有独立的参数子集、预检查集和结果模型。

### 1.2 ExportConfig（通用导出配置）

替代当前 config_json 中的 CSV 单表硬编码结构（database/table/format/filePath/logPath/skipCheckDir）。

```text
ExportConfig {
  objectScope:      ObjectScope        // 对象范围（基础选项 · 功能选项 · 数据库对象类型）
  contentSelection: ContentSelection   // 导出内容
  dataFormat:       DataFormat         // 数据格式及专属参数（基础选项 · 功能选项 · 文件格式）
  outputConfig:     OutputConfig       // 输出位置与文件布局（基础选项 · 功能选项 · 存储路径）
  performanceConfig: PerformanceConfig // 高级选项 · 性能选项
  filterConfig:     FilterConfig       // 高级选项 · 功能选项 · 黑白名单筛选 / 时间戳格式
  ddlBehavior:      DDLBehavior        // 文件格式（DDL 伴生）/ 数据库对象类型 / 高级选项 · 其他选项
}
```

**ObjectScope（对象范围）**

```text
ObjectScope {
  database:      string            // 对象所在数据库（--database），全部与指定范围均必填
  scopeKind:       ALL | SPECIFIED     // 全部/指定
  objectTypes:     [ObjectType]        // 对象类型列表（表/视图/触发器/...）
  expressions:     [ObjectExpression]  // 对象表达式（可选 schema 前缀只允许缺省或与 database 一致，跨库未取证）
  excludeTables:   [string]            // 排除表表达式
}

ObjectType = TABLE | TABLE_GROUP | VIEW | TRIGGER | USER | ROLE
           | SEQUENCE | SYNONYM | TYPE | TYPE_BODY | PACKAGE
           | PACKAGE_BODY | FUNCTION | PROCEDURE

ObjectExpression {
  schema:    string?    // 可选 schema 前缀（多库模式标识）；EX-I2 只接受缺省或与 database 一致，跨库失败关闭
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

`formatKind` 枚举用于历史兼容和能力版本解析；普通 V1 新建入口只允许 CSV、CUT、SQL。旧格式不得因页面收缩而改写为未知值或丢失快照。

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
  retainEmptyFiles: bool           // CSV/CUT/SQL 数据导出可用；DDL_ONLY 不活动
  storageCredential: StorageCredentialRef?  // 对象存储凭据引用
}
```

**PerformanceConfig（高级选项 · 性能选项）**

```text
PerformanceConfig {
  thread:         int?
  pageSize:        int?
  parallelMacro:  int?
  fetchSize:      int?
  jvmMemory:      string?        // K/M/G/T
  retry:          bool           // 仅供 EX-I8 失败任务恢复派生；新建草稿携带时阻断
}
```

**FilterConfig（高级选项 · 功能选项 · 黑白名单筛选 / 时间戳格式 / 错误处理）**

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
  snapshot:            bool?     // 无值开关；与闪回参数组合在确认前阻断
  weakRead:            bool?     // 副本与权限预检查完成前 VALIDATION_GATED
}
```

**DDLBehavior（基础选项 · 功能选项 · 文件格式 DDL 伴生 / 数据库对象类型 / 高级选项 · 其他选项）**

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
| `export-odp-full-csv-v1` | 全对象 CSV（含 all/多表/多对象类型） | ENABLED（已实现，EX-I2） |
| `export-odp-ddl-v1` | 纯 DDL 导出 | ENABLED（已实现，EX-I2） |
| `export-odp-ddl-csv-v1` | DDL + CSV 数据 | ENABLED（已实现，EX-I2） |
| `export-odp-cut-v1` | CUT 格式 | ENABLED（已实现，EX-I4） |
| `export-odp-sql-v1` | Insert SQL 格式 | ENABLED（已实现，EX-I4） |
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

v5 元数据继续作为 `export-odp-single-table-csv-v1` 的冻结基线；v6 是已发布的泛化历史版本；v7 是当前泛化版本。草稿、预检查与任务均绑定精确 metadataVersion，重算时必须按该版本选择定义集，不能把 v5/v6 静默提升到 v7。

### 2.4 命令生成兼容

命令生成器按 metadataVersion 选择参数定义集，按 capabilityVersion 选择活动参数子集和命令模板。现有 CSV 单表命令生成路径不变。

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
    FormatKind string      `json:"formatKind"` // CSV | CUT | POS | SQL | PARQUET | ORC | AVRO
    CsvOptions CsvOptions  `json:"csvOptions,omitempty"`
    CutOptions      map[string]interface{} `json:"cutOptions,omitempty"`
    PosOptions      map[string]interface{} `json:"posOptions,omitempty"`
    TextOptions     map[string]interface{} `json:"textOptions,omitempty"`
    DateTimeOptions map[string]interface{} `json:"dateTimeOptions,omitempty"`
    Compression     map[string]interface{} `json:"compression,omitempty"`
}

// 偏差记录：初稿用 map[string]interface{} 表达格式专属选项；EX-I3 起 CSV 选项改为类型化 CsvOptions，
// 拒绝自由键值进入草稿与快照（与 rawInput 同因的安全收紧）。其余格式选项仍保持待实现状态。
type CsvOptions struct {
    SkipHeader      bool   `json:"skipHeader,omitempty"`
    ColumnSeparator string `json:"columnSeparator,omitempty"`
    ColumnQuote     string `json:"columnQuote,omitempty"`
    ColumnQuoteMode string `json:"columnQuoteMode,omitempty"`
    EscapeCharacter string `json:"escapeCharacter,omitempty"`
    LineSeparator   string `json:"lineSeparator,omitempty"`
    NullString      string `json:"nullString,omitempty"`
    FileEncoding    string `json:"fileEncoding,omitempty"`
    WithTrim        bool   `json:"withTrim,omitempty"`
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
    Retry         bool   `json:"retry"` // 仅保留契约兼容；新建草稿不得启用
}

type FilterConfig struct {
    QuerySql           string   `json:"querySql,omitempty"` // 普通高级参数；不使用敏感命令 capability，拒绝 file:// 并复验互斥
    Where              string   `json:"where,omitempty"`
    Partition          string   `json:"partition,omitempty"`
    IncludeColumnNames []string `json:"includeColumnNames,omitempty"`
    ExcludeColumnNames []string `json:"excludeColumnNames,omitempty"`
    ExcludeDataTypes   []string `json:"excludeDataTypes,omitempty"`
    ExcludeVirtualColumns *bool `json:"excludeVirtualColumns,omitempty"`
    EnableHiddenPk     *bool    `json:"enableHiddenPk,omitempty"`
    FlashbackScn       *int64   `json:"flashbackScn,omitempty"`
    FlashbackTimestamp string   `json:"flashbackTimestamp,omitempty"`
    Snapshot           *bool    `json:"snapshot,omitempty"`
    WeakRead           *bool    `json:"weakRead,omitempty"` // 预检查完成前携带即阻断
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

### 5.1 参数元数据运行时版本链

运行时只嵌入 `internal/parammeta/resources/` 中通过清单身份、继承链、定义数、状态和命令顺序校验的版本。`drafts/` 仅保存历史设计输入，不得由运行时加载。

- v5：18 个定义，冻结单表 CSV 基线。
- v6：59 个定义，冻结的泛化历史版本，仅用于重放已持久化 v6 草稿和任务。
- v7：在 v6 上新增 13 个定义，共 72 个定义；当前只新增启用 MySQL DATE/DATETIME 格式、分区筛选和类型排除。其余 9 个第二批定义保持 VALIDATION_GATED。

**运行时定义字段**（每个参数）：
- `supportState`：ENABLED | VALIDATION_GATED | HIDDEN | BLOCKED
- `sensitivity`：NORMAL | IDENTIFIER | SECRET
- `riskLevel`：LOW | MEDIUM | HIGH
- `officialEvidence`：支撑该定义的契约、官方资料和受控验证引用
- `capabilityVersions`：该参数适用的能力切片列表
- `emissionTarget`：ARGV | SECURITY_FILE（从 v5 overrides 提升为定义级字段）

**现行分类体系**（9 类）：

> 元数据 `category` 是命令发射顺序的技术标识（与 `order` 配合驱动 plannedArgv 排序），不是产品分类。产品与文档按 OBDUMPER 官方选项分类组织（基础选项：连接/功能/其他；高级选项：功能/性能/其他）；109 参数研究与分类表保留在 `docs/archive/export/research/`。

| 序号 | 分类标识 | 含义 |
|---|---|---|
| 1 | CONNECTION | 连接与会话 |
| 2 | DATABASE_CONNECTION | 数据库连接 |
| 3 | OBJECT_SCOPE | 对象范围 |
| 4 | CONTENT_FORMAT | 内容与数据格式 |
| 5 | FORMAT_SERIALIZATION | 文本、CSV/CUT 与已启用时间值格式 |
| 6 | OUTPUT_FILE | 输出与文件 |
| 7 | DATA_FILTER | 数据筛选、一致性与相关 DDL 辅助参数 |
| 8 | PERFORMANCE | 性能与资源 |
| 9 | COMPRESSION | 压缩参数 |

### 5.2 capabilityVersion 参数子集

每个 capabilityVersion 激活不同参数子集：

| capabilityVersion | 必须参数 | 格式参数 | 特殊参数 |
|---|---|---|---|
| `export-odp-single-table-csv-v1` | host/port/user/password/database/table | csv + csvOptions | file-path |
| `export-odp-full-csv-v1` | host/port/user/password/database | csv + csvOptions | all/table/exclude-table |
| `export-odp-ddl-v1` | host/port/user/password/database | ddl | drop-object/retain-schema |
| `export-odp-ddl-csv-v1` | host/port/user/password/database | ddl + csv | 全 DDL + CSV 参数 |
| `export-odp-cut-v1` | host/port/user/password/database | cut + cutOptions | 共享文本序列化（escape/null/with-trim/line-separator/file-encoding）、压缩、存储路径、筛选与性能参数（官方复核：不限定格式，2026-08-07 起绑定 CUT 能力） |
| `export-odp-sql-v1` | host/port/user/password/database | sql | 行分隔符/文件编码、压缩、存储路径、筛选与性能参数（官方复核：不限定格式，2026-08-07 起绑定 SQL 能力） |
| `export-odp-pos-v1` | host/port/user/password/database | pos + ctl-path | 控制文件目录（用户提供来源已接入；自动生成为后续切片）、存储路径、筛选与性能参数（2026-08-07 POS 实测定版） |
| `export-odp-parquet-v1` | host/port/user/password/database | par | 文件编码、闪回/筛选/性能/存储路径参数（官方格式表，2026-08-07）；压缩与序列化不适用 |
| `export-odp-orc-v1` | host/port/user/password/database | orc | 文件编码、闪回/筛选/性能/存储路径参数（官方格式表，2026-08-07）；压缩与序列化不适用，内存风险较高 |
| `export-odp-avro-v1` | host/port/user/password/database | avro | 文件编码、闪回/筛选/性能/存储路径参数（官方格式表，2026-08-07）；压缩与序列化不适用 |

### 5.3 命令生成泛化

当前 `generateExportDraft` 中的硬编码参数列表改为参数元数据驱动：

1. 加载草稿或任务绑定的 metadataVersion 对应定义集（v1 基础 + 逐层 overrides/additions）；仅新建泛化草稿选择当前 v7
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
| 7 | 存储层 | STORAGE_CONNECTIVITY | Agent 网络探测 | URI/凭据变化 | 框架已实现；代码默认关闭，本机 MVP/Agent 包启动器持续授权并开启 |
| 8 | 存储层 | STORAGE_AUTH | Agent 网络探测 | URI/凭据变化 | 框架已实现；真实探测默认关闭（EX-V1 授权） |
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

EX-I2 实施收敛：三个新能力已接入固定六项检查。OBJECT_ACCESS 对 SPECIFIED 范围逐对象运行冻结单对象 JDBC 探针（数据可读性是 DDL 可读性的保守超集），对 ALL 范围按数据库级可达性投影，逐对象枚举由工具运行时完成。层 3 SYS_PRIVILEGE 仅在 `--add-extra-message` 等 DDL 行为参数启用后才需要，EX-I2 未激活。

**EX-I6 存储层实施（2026-08-14）**：对象存储输出任务的预检查使用“存储形态清单”——`DATABASE_CONNECTIVITY → OBJECT_ACCESS → TOOL_ENVIRONMENT → AVAILABLE_SPACE → STORAGE_CONNECTIVITY → STORAGE_AUTH`（裁剪不适用 URI 输出的 OUTPUT_PATH/OUTPUT_EMPTY；AVAILABLE_SPACE 转向 `--tmp-path` 卷，未指定时 UNKNOWN）。本地输出保持冻结六项不变。两项存储检查由 Agent 探测：

- STORAGE_CONNECTIVITY：受控 TCP 端点可达性探测，仅在显式运行开关（`OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true`）开启时装配；Agent 二进制直接启动时默认 UNKNOWN。本机 MVP/Agent 包启动器固定开启该开关，运行启动器即视为对该 Agent 生命周期内受控 provider endpoint TCP 建连的持续授权，关闭开关或停止进程即撤销；探针不发送凭据或业务数据，该授权不扩展到 STORAGE_AUTH、工具执行或数据库操作。
- STORAGE_AUTH：凭据有效性探测接口与失败关闭默认实现已就位。预检查创建会冻结已校验的存储凭据引用；只有在数据库/对象检查和 STORAGE_CONNECTIVITY 均通过后，且探测器显式声明需要凭据时，才可经独立 `STORAGE_CREDENTIAL` 槽位短时解析该引用。默认探测器不解析凭据并返回 UNKNOWN；真实四类云厂商签名探测归 EX-V1。

任务提交门禁由结果驱动：对象存储输出要求两项存储检查均 PASSED（`STORAGE_PRECHECK_REQUIRED`），未启用探测时保持 UNKNOWN 并失败关闭。旧 `STORAGE_PRECHECK_UNAVAILABLE` 功能门禁已移除；STORAGE_AUTH 真实凭据取证仍须另行授权。

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
| STORAGE_CREDENTIAL | 对象存储凭据（OSS/S3/COS/OBS） | ENABLED（执行槽位，2026-08-14；预检查受控槽位准备已实现，真实凭据探测归 EX-V1） |

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

**EX-I8 实施收敛（2026-08-14，合成验证）**：

- Agent 在成功与失败路径都上报 `dump.ckpt` 存在性事实（失败路径作为失败终态后的迟到事实，由同一租约与连续序号接受）；控制面合并为 `result_summary_json`（`result/fileCount/totalBytes/files/checkpointPresent/observedAt`），任务详情投影受限结果摘要。进程证据另以 `process_evidence_json` 保存直接 Java 退出码、固定错误码和 planned/actual argv SHA-256 摘要比较；耗时由可信 `started_at/finished_at` 派生，输出位置按主体权限返回完整值或仅类型。
- 派生任务模型：迁移 0018 增加 `tasks.derivation_kind`（REBUILD_FROM_CONFIG/RERUN_FROM_SCRATCH/CHECKPOINT_RESUME）与 `export_drafts.source_task_id/source_derivation`；`parent_task_id` 沿用 0014。
- 基于原配置新建/从头重新执行：`POST /tasks/{id}:rebuild-draft` 从失败任务冻结快照重建可编辑 v6 草稿；从头执行提交时服务端强制 configFingerprint 与来源任务一致（否则 422 RERUN_CONFIGURATION_CHANGED）。
- 检查点继续：`POST /tasks/{id}:resume-checkpoint` 资格 = 失败终态 + 结果摘要确认 dump.ckpt 存在 + 原预检查 SUCCEEDED/COMPLETE；新任务继承原快照并追加 `--retry`（服务端固定构造，不新增参数元数据版本），领取执行时复验数据源/凭据/节点/Agent 事实版本但豁免预检查 TTL。
- 真实 `dump.ckpt` 续跑取证与结果清单的行数/校验和解析归 EX-V1；当前文件清单只含相对路径与字节数。
- Cancel 已纳入当前执行协议，但只允许受控、有界和可核验的取消意图：浏览器 `POST /api/v1/tasks/{taskId}:cancel` 要求 CSRF、幂等键和任务授权；排队任务短事务投影 `CANCELLED`，运行中任务先进入 `CANCELLING`。Agent 在同一有效租约内以固定 `OBDUMPER_EXPORT_POLL_CONTROL` 信封轮询，控制面校验冻结任务信封摘要后才返回取消事实。
- Agent 终止受控 Java 进程树并按序上报 `PROCESS_CANCELLED`、`PROCESS_EXITED` 和结果事实。只有树终止已观察且终态证据完整时才投影 `CANCELLED`；取消失败、期限到期、摘要不一致或事件缺口均失败关闭，使用固定 `CANCEL_FAILED`、`CANCEL_TIMEOUT` 或 `EXECUTION_EVIDENCE_UNAVAILABLE`，必要时保留 `reconciliation_required`。浏览器不能直接终止节点进程，也不能提交命令、路径、SQL、秘密或自由原因。

### 8.4 终态证据

明确不以包装脚本退出码单独判断成功。本地输出必须结合：

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

**对象存储便利性例外（2026-08-17）**：在 EX-V1 远端 MANIFEST/对象清单取证尚未接入 Agent 前，对象存储任务没有可枚举的本地输出目录。此阶段允许“受控 OBDUMPER 进程退出码 0 + Agent 上报固定 `TOOL_TERMINAL_OBSERVED=SUCCEEDED`”形成 `RESULT_FACTS_OBSERVED=VERIFIED`；`fileCount=0`、`totalBytes=0`、`files=[]`、`checkpointPresent=false`，不得伪造远端文件或检查点，也不得把该状态解读为远端对象逐项校验已通过。非零退出、进程/终态冲突、事件缺失仍按失败或待核对处理。EX-V1 接入远端 MANIFEST 后应移除此例外并恢复完整文件事实门禁。

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

EX-I2 交付收敛：按任务地图权威，EX-I2 一次实现 full-csv、ddl、ddl-csv 三个能力，上表 EX-I2~EX-I4 三行测试焦点已在同一切片内覆盖（对象矩阵正反例、仅 DDL 不生成数据格式参数、DDL + CSV 联合激活）；EX-I3/EX-I4 行后续仅保留 DDL 行为参数与权限层的增量工作。多库 schema 前缀（EX-F012）保持 VALIDATION_GATED，控制面对跨库表达式失败关闭。

EX-I3 交付收敛：25 个 ENABLED 参数一次启用（CSV 序列化 9、压缩 2、文件布局 3、筛选 6、资源 5）。该切片交付时日期时间、--compression-level、--where/--partition/--exclude-data-types 均保持门禁；后续切片的状态变化以任务地图和现行支持矩阵为准。带任一活动选项的单表 CSV 离开冻结 v5 路径并使用对应泛化版本；无选项单表保持字节级不变。

EX-I4 交付收敛：CUT（export-odp-cut-v1）与 Insert SQL（export-odp-sql-v1）已启用并实现。CUT 启用 --cut、--trail-delimiter、--remove-newline（高风险）及与 CSV 共享的转义字符/行分隔符/空串/编码/修剪（FORMAT_IN CSV,CUT）；SQL 启用 --sql 及行分隔符/文件编码（FORMAT_IN CSV,CUT,SQL），共享文本之外的 CSV 专属与筛选/资源参数在 CUT/SQL 能力下按 UNKNOWN_PARAMETER 失败关闭。服务端归一化按格式校验 CsvOptions/CutOptions 越界（422），DDL_AND_DATA 固定 CSV、DDL_ONLY 不得声明数据格式；POS 已按独立 `--pos` + `--ctl-path` 定版，自动生成控制文件仍保持后续门控。前端向导新增格式单选与 CUT 高级配置面板，按格式收敛请求体。契约测试覆盖正例、互斥、边界与 ORACLE 负例；生成器格式单选在元数据误配置时仍失败关闭。

### 9.3 测试约束

- 所有测试必须在不连接真实数据库、不启动 OBDUMPER 的条件下完成
- 命令生成测试验证 argv 结构和秘密槽位引用，不执行实际命令
- 预检查测试使用模拟 Agent 回执，不连接实际端点
- 安全测试覆盖全部秘密路径（响应/日志/快照/错误消息）
- 跨平台路径测试覆盖 Windows（盘符 + UNC）和 Linux 绝对路径

### 9.4 事实源分工与重复审计

同一规则在不同层出现时，必须是“输入提示 + 服务端复验”或“事实投影 + 展示”，不能形成可独立改变发布行为的第二实现。当前分工如下：

| 事实面 | 唯一权威 | 允许的下游副本 | 不允许的行为 |
|---|---|---|---|
| 官方参数与格式适用性 | `internal/parammeta/resources/` v5～v7 | 前端字段显示、领域适配器和文档引用 | 在 Vue、handler 或测试夹具中另造默认能力矩阵 |
| 导出范围/内容/格式组合 | `internal/exportdomain/selection.go`、`internal/exportdomain/storage.go` 与服务端归一化 | 前端即时校验可提前提示同一错误 | 仅依赖浏览器校验创建草稿或任务 |
| 配置快照与提交事实 | `internal/store` 的 `ExportConfig`、`SubmissionSnapshot` 和任务表 | API/OpenAPI 只做白名单投影 | 用草稿表单状态推断已冻结任务 |
| 执行命令 | `internal/commandgen` 与泛化字段适配器 | 预览和执行共用脱敏/执行形态摘要 | 前端或 handler 拼接另一份 Shell 字符串 |
| 预检查与安全上下文 | 控制面 + Agent 协议/租约 | 页面只展示状态和稳定摘要 | Agent 决定产品参数、页面绕过预检查或提交门禁 |
| 任务状态与结果证据 | 控制面 `store` 事件投影 | API/页面只展示授权投影 | 以 Agent 自报状态、父进程退出码或包装脚本单独判成功 |
| 产品、技术和验证说明 | 本目录列出的五份 Canonical 文档 | `docs/README.md` 提供索引；历史文件只保留短指针 | 以归档计划、旧字段矩阵或 HANDOFF 快照覆盖当前结论 |

---

## 10. 关联文件索引

| 文件 | 说明 |
|---|---|
| `docs/03-technical/export-general-contract.md` | 本文（通用导出技术契约） |
| `internal/parammeta/drafts/obdumper-4.3.5-slice-v6.draft.json` | 历史设计输入，不由运行时加载 |
| `internal/parammeta/resources/obdumper-4.3.5-slice-v5.json` 至 `v7.json` | 冻结基线、历史泛化重放与当前泛化参数元数据 |
| `migrations/0014_export_generalization.sql` | 通用导出领域模型迁移 |
| `internal/store/types.go` | 泛化导出配置类型 |
| `contracts/openapi.json` | 导出草稿和任务快照 schema |
