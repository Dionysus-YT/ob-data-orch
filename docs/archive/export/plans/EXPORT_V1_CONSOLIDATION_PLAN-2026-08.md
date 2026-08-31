# Export v1 收敛重构与文档治理计划

> 文档状态：已完成并归档的临时实施计划，不是长期 Canonical Doc
> 形成日期：2026-08-18
> 适用范围：Export v1 Consolidation 第一阶段，包含分析、定向实施与验证排序
> 处置规则：本轮收敛完成后，将长期有效结论合并到 Canonical Docs；本文件随后删除，只有在需要保留关键决策演进时才归档
> 安全边界：本次未连接数据库、未解析真实凭据、未启动 OBDUMPER、未创建真实任务；已按计划对 Export 领域代码做小范围职责提取和 query-sql 失败关闭，未改变 API、SQLite、历史快照或 Agent 契约

> 当前执行状态（2026-08-19）：Phase 0～6 已完成；query-sql 门禁、结果证据、排队/运行中/超时取消、进程树平台适配、OpenAPI/前端投影和 Canonical 文档治理均已接入并通过定向合成回归。组合根仍默认关闭真实执行和 query-sql，高风险能力继续失败关闭。真实授权器接入、真实工具进程树终止、远端存储认证/MANIFEST 和 EX-V1 逐格式验证仍待授权环境，不能由本计划或合成门禁推出。

> 验证记录（2026-08-19）：已通过 `npm audit fix --package-lock-only` 将 `web/package-lock.json` 的 `nanoid` 更新至 3.3.18、`postcss` 更新至 8.5.26，并同步 `brace-expansion` 至 5.0.9；干净 `npm ci --ignore-scripts --no-audit` 后 `npm audit --omit=dev` 为 0，前端测试、类型检查、lint 和构建均通过。

## 1. Executive Summary

### 1.1 结论

| 问题 | 结论 |
|---|---|
| 当前方向是否正确 | **基本正确但需要收敛** |
| 当前是否需要重构 | **收敛型重构** |
| 是否需要结构性重构 | 否。核心链路和关键安全边界成立，只需要对少数职责过载点做定向拆分 |
| 是否需要推倒重写 | 否。重写会破坏已经真实运行过的命令、执行、日志、结果和历史兼容语义 |
| 当前首要目标 | 冻结真实基线，定义 Export v1 Scope/DoD，消除多份事实源，再补最小 v1 缺口 |

当前核心技术路线已经成立：浏览器只提交结构化配置，控制面重新校验并调用唯一命令生成器，任务提交时冻结配置与命令，Agent 领取固定信封并直接启动受控 OBDUMPER，控制面依据事件、日志和结果事实形成任务历史。该链路已经有真实 CSV 端到端证据；用户同时确认其他格式也已经跑通，本计划接受这一事实，不重新质疑主链是否可行。

问题不在“能否工作”，而在“是否已形成可正式承诺、可准确解释、可持续维护的 v1”。当前答案是否定的，主要原因是产品范围、实施状态、真实证据和历史设计被写入多份文档，代码中的规则也分散在前端、控制面、参数元数据和命令生成器之间，缺少清楚的权威层级。

### 1.2 问题排序

| 排名 | 类别 | 判断 |
|---:|---|---|
| 1 | 产品定义 | 已实现能力很多，但“哪些正式属于 Export v1”尚未冻结；实现存在不代表产品承诺 |
| 2 | 文档债务 | 多份现役文档互相冲突，历史基线没有退出正常开发上下文，已经影响事实判断 |
| 3 | 开发组织 | 长期按 EX-D/EX-I/WI 切片推进，完成结论留在任务地图、交接和证据文档中，没有及时回收到产品/技术基线 |
| 4 | 架构 | 总体架构正确，但导出归一化、能力选择和字段到命令映射过度集中在 `internal/controlplane/server.go` |
| 5 | 代码质量 | 前后端规则、枚举、路径和默认语义重复；`CsvOptions` 承载跨格式文本配置，命名与职责已经失真 |
| 6 | 测试 | 自动化覆盖较强，但缺少一个固定的 Export v1 Golden Path 套件，以及“用户确认的其他格式”到具体格式/任务/输出证据的正式映射 |
| 7 | UX | 当前六步向导可以完成真实工作；主要问题是对象选择、信息密度、格式配置组织和任务详情证据不足，不需要重做整体体验 |

### 1.3 必须立即承认的契约冲突

当前 `--query-sql` 已进入前端表单、`ExportConfig.FilterConfig`、控制面归一化和命令生成测试；本轮已补齐 `CAP_SENSITIVE_COMMAND`、任务范围授权、提交级风险指纹确认、输入变化失效和同一事务审计。真实组合根尚未提供授权器，因此默认运行时仍失败关闭。

因此：

- `--query-sql` 不能按当前实现直接列为 Export v1 正式能力。
- 在真实授权器和 EX-V1 证据接入前，仍按失败关闭处理；不能因为测试注入授权器可以生成命令就视为产品正式可用。
- 这不是否定主链，而是一个必须在 v1 冻结前解决的局部安全缺口。

### 1.4 安全方向：风险相称，不以流程复杂度代替安全

本轮新增一项产品与工程要求：**安全管控必须与可证明的风险相称，不得把生产级门禁无差别施加到合成、离线、无秘密、无真实副作用的开发验证上。** 安全目标是阻止明确的越权、秘密泄露、任意执行、数据破坏和错误生产操作，而不是最大化确认次数、审批层级或默认阻断数量。

以下硬边界保持不变，不能以“提升开发效率”为由绕过：

- 浏览器身份与 Agent 机器身份隔离，认证、授权、CSRF 或安全上下文不可用时失败关闭。
- 秘密不进入命令行、日志、审计、错误、响应、任务快照、SQLite、Git 或测试输出。
- 不提供远程 Shell、任意命令、任意文件浏览、SQL 编辑器或不受控 SQL；`--query-sql` 仍是受限专家能力。
- 未经明确授权，不连接真实数据库、不解析真实凭据、不启动真实工具、不操作用户数据或生产环境。
- 命令生成器、Agent 固定信封、租约/幂等、日志脱敏和结果证据的安全不变量不得降级。

风险分级采用以下最小模型；后续 Canonical Docs 可调整名称，但不得改变边界：

| 等级 | 典型场景 | 默认管控 | 开发效率要求 |
|---|---|---|---|
| S0 合成离线 | 版本化 fixture、假 Agent、假工具、临时目录，无网络、秘密或用户数据 | 自动化测试与静态检查 | 默认允许，不需要人工确认或外部依赖 |
| S1 本地无副作用 | 结构化配置校验、脱敏命令预览、历史 fixture 重放，不建立真实连接或启动工具 | 本地身份/功能开关和边界负例 | 同一操作不重复确认；失败必须给出稳定错误码和修复动作 |
| S2 受控非生产真实验证 | 已登记节点、真实工具或非生产数据源，需要真实凭据/网络 | 明确列出对象、节点、动作和期限的范围授权，执行前预检查并保留证据 | 授权范围内不为每个内部步骤重复索要确认；变更范围或到期后重新授权 |
| S3 敏感专家能力 | `--query-sql` 等可能扩大数据读取或命令表达面的能力 | 专项 capability、提交级二次确认、审计、不可变风险指纹 | 编辑和普通预览阶段不叠加无意义弹窗；最终提交确认一次，相关字段变化即失效 |
| S4 生产/破坏性 | 生产数据、删除、覆盖、不可逆或高影响操作 | 独立明确授权、最小权限、二次确认、回滚/恢复方案和完整证据 | 不以进度为由简化；未满足条件时阻断 |

每个新增安全控制都必须说明：保护的资产、要阻止的具体威胁、执行层、失败方式、开发/用户成本、自动化验证方式，以及何时可以合并或移除。多个控制若只是重复阻止同一威胁，应收敛为一个权威门禁；只有面对不同失效模式时，才保留可解释的纵深防御。

确认与授权应绑定不可变风险指纹，至少覆盖数据源/凭据修订、执行节点、输出目标、敏感参数和动作类型。指纹未变且授权仍在有效期内时复用既有确认；任一安全相关输入变化、授权撤销或过期后立即失效。现有 `STORAGE_CONNECTIVITY` 启动开关所表达的运行期持续授权可作为这种“有界授权”的参考，但不得外推到凭据探测、工具执行或数据库操作。

## 2. Current Export Status

### 2.1 已确认事实

1. 已有数据源可以被导出向导选择，并由节点侧固定 JDBC 探针验证连接。
2. 页面可以生成结构化导出草稿，覆盖内容、范围、对象、格式、输出、筛选和性能配置。
3. 服务端可以生成真实 OBDUMPER 命令，密码不进入 argv。
4. Agent 可以在 Windows AMD64 Local MVP 边界内直接启动真实 OBDUMPER。
5. CSV 已形成可追溯的真实端到端证据，包括特殊值和 Windows 路径场景。
6. 用户确认其他格式也已经跑通；仓库当前没有把这一结论逐格式映射到所有运行记录。
7. 任务状态、持久化脱敏日志、结果文件摘要、任务中心和任务详情已经进入链路。
8. 基于失败任务重建草稿、从头重新执行、检查点继续资格判断和成功任务保存模板已经实现。

### 2.2 不能外推的结论

- 真实 Export 链路可用，不等于 G3、三个麒麟目标、生产发布或全部格式组合已经通过。
- 某一格式成功，不等于该格式的全部对象范围、压缩、筛选、编码和高级参数组合均已正式支持。
- 命令生成测试通过，不等于真实输出已验证。
- OBDUMPER 退出码 0 不单独构成成功；本地输出仍需结合受控进程、工具终态和文件事实。
- 对象存储输出在远端 MANIFEST 接入前不能声称远端对象逐项核验通过。

### 2.3 当前总体状态

| 维度 | 状态 | 说明 |
|---|---|---|
| 主 Golden Path | STABLE | 已有真实成功链路，应转为回归基线 |
| 多格式产品入口 | WORKING | 七种格式已进入 UI、领域类型、OpenAPI、命令生成和自动化测试 |
| 多格式真实证据治理 | NEEDS_CONSOLIDATION | 用户确认已跑通其他格式，但仓库未逐格式绑定任务、命令、输出和历史证据 |
| Export v1 产品范围 | NEEDS_CONSOLIDATION | 设计范围、实现范围和发布范围仍混写 |
| 核心执行架构 | STABLE | 控制面、Agent、固定信封、直接 Java、日志与状态边界正确 |
| 领域/规则组织 | NEEDS_REFACTOR | 控制面 handler 文件承担了过多导出领域职责 |
| 结果与历史 | WORKING | 已增加退出码、可信时间派生耗时、固定错误码/稳定摘要、按权限裁剪的输出位置和 planned/actual argv 摘要；Cancel 与完整非敏感配置回看仍待后续切片 |
| 文档体系 | NEEDS_REFACTOR | 多份文档内容过期或互相冲突，无法作为少量可靠入口 |

## 3. Current Capability Map

以下按真实用户流程，而不是代码目录，描述当前能力。

| 用户环节 | 当前实现 | 状态 | Export v1 判断 |
|---|---|---|---|
| 数据源 | 选择已有、已启用且有成功连接测试事实的数据源 | STABLE | 保留；数据源编辑继续留在独立模块 |
| 导出内容 | `DDL_ONLY`、`DATA_ONLY`、`DDL_AND_DATA` | WORKING | 保留；当前 DDL + 数据只允许 CSV，必须在产品文档明确 |
| 导出范围 | `ALL` 或 `SPECIFIED` | WORKING | 保留；跨库前缀继续门禁 |
| 导出对象 | 指定 TABLE/VIEW，多对象文本输入；ALL 范围；排除表 | PARTIAL | v1 可保留当前能力，但对象搜索、类型过滤、批量选择和大量对象性能未实现 |
| 数据格式 | CSV/CUT/SQL/POS/PARQUET/ORC/AVRO 单选 | WORKING | 进入正式支持与实验支持分级，不再以 CSV 为唯一中心 |
| 文件配置 | 本地绝对路径、可选日志路径、目录策略、拆分、压缩、POS 控制目录 | WORKING | 保留已验证项；默认值必须明确区分“平台默认”和“继承 OBDUMPER” |
| 对象存储 | OSS/S3/COS/OBS 受控 URI、凭据引用、tmp 路径、预检查框架 | PARTIAL | 不进入正式 v1 可用声明，直到真实认证探测和端到端取证完成 |
| 高级配置 | 筛选、一致性、性能、DDL 行为、部分时间格式 | NEEDS_CONSOLIDATION | 只保留已验证且有业务语义的参数；未验证项继续失败关闭 |
| 参数校验 | 前端即时校验、控制面归一化、命令生成器元数据校验、Agent 事实检查 | DUPLICATED | 保留分层防御，但统一规则归属和契约测试，消除同一语义的手工复制 |
| 执行前确认 | 摘要、预检查、脱敏命令和提交门禁 | WORKING | 保留；补齐风险确认与 actual/planned 关系表达 |
| 命令预览 | 服务端调用同一生成入口产生脱敏命令和指纹 | STABLE | 必须保留，作为正式产品能力 |
| 任务创建 | 冻结快照、planned argv、脱敏命令、凭据引用和预检查绑定 | STABLE | 必须保留，不得由重构改变历史语义 |
| Queue | `WAITING_SCHEDULE`、固定节点领取和租约 | WORKING | v1 保留；当前不是通用调度器 |
| Execution Host | 草稿绑定节点，预检查核验节点和 Agent 事实 | WORKING | 任务详情需展示更清楚的安全主机摘要 |
| OBDUMPER | Agent 直接 Java 启动固定主类，秘密经安全文件 | STABLE | 必须保留；不引入 Shell 或任意命令 |
| Start/Running | 租约、进程事件和状态投影 | STABLE | 保留 |
| Success/Failed | 进程、工具终态和结果事实共同投影 | WORKING | 已补退出码、耗时、固定错误摘要、输出位置权限投影和 planned/actual 一致性；真实工具证据仍归 EX-V1 |
| Cancel | 状态枚举存在 `CANCELLING/CANCELLED`，没有浏览器取消 API/操作 | NOT_IMPLEMENTED | 当前明确不宣称可用；若重新列入 v1 DoD，必须另行实现受控进程树取消、幂等和终态证据，不能只保留枚举冒充能力 |
| 状态 | 任务状态与 `reconciliationRequired` | STABLE | 持久任务状态为唯一产品状态，UI 只做标签投影 |
| 日志 | stdout/stderr 经 Agent 第一层和控制面第二层脱敏，持久化并支持 SSE 续传 | STABLE | “raw log”定义为保留来源/顺序的强制脱敏日志，绝不提供未脱敏原文 |
| 结果 | 文件数、总字节、受限相对路径、检查点和观察时间 | WORKING | 已补安全输出位置、耗时、退出码、固定错误摘要和 planned/actual 摘要；行数/校验和不是当前 v1 必需 |
| Run History | 任务中心、任务详情、快照、命令、执行、日志和结果分投影读取 | WORKING | 统一称为任务历史/Export Run；无需先新建独立 `ExportRun` 表 |
| Re-run | 基于原配置新建、从头重新执行、检查点继续、模板复用 | WORKING | 保留；检查点真实续跑仍需证据，模板不复制凭据/节点/预检查/风险确认 |

## 4. Supported Format Matrix

### 4.1 证据口径

本矩阵接受“CSV 和其他格式已经真实跑通”的用户确认。当前治理缺口是仓库没有把“其他格式”逐项绑定到 CUT、SQL、POS、Parquet、ORC、Avro 的任务 ID、命令指纹、输出摘要和历史页面证据。因此以下分类区分：

- **真实能力事实**：用户已确认实际运行。
- **仓库可追溯证据**：当前 Git 中是否能逐格式定位。
- **v1 正式状态**：是否已经具备长期维护所需的边界、回归和证据。

Phase 1 应优先整理已有运行记录，不以重新运行来“证明”已确认事实。

### 4.2 格式矩阵

| Format | UI / Config | Command | Automated Tests | Real Execution Evidence | Output / History | Current Status | v1 建议 |
|---|---|---|---|---|---|---|---|
| CSV | 已接入，含完整 CSV 序列化配置 | `--csv`，v5/v6/v7 兼容 | 前端、控制面、生成器、Agent、契约覆盖广 | Git 中有完整 Windows 平台链路、特殊值和路径证据 | 已进入状态、日志、文件摘要和历史 | STABLE | **正式支持** |
| CUT | 已接入，共享文本参数 + CUT 专属项 | `--cut` | 有正例、互斥、边界和快照测试 | 用户确认其他格式已跑通；Git 中缺少 CUT 全链路逐项映射 | 代码路径已具备 | WORKING | 候选正式支持；补现有证据映射后冻结 |
| SQL | 已接入，文本编码/行分隔等 | `--sql` | 有生成与控制面全流程测试 | 用户确认其他格式已跑通；Git 中缺少 SQL 全链路逐项映射 | 代码路径已具备 | WORKING | 候选正式支持；补现有证据映射后冻结 |
| POS | 已接入，要求 `controlFilePath` | `--pos --ctl-path` | 有生成、控制面、前端正负例 | Git 中有 OBDUMPER 4.3.5 直接实测：6 表、900 行严格定长；平台全链路证据未单独归档 | 代码路径已具备 | WORKING | 候选正式支持；v1 只承诺“用户提供控制文件”，不承诺自动生成 |
| PARQUET | 已接入，结构化格式限制 | `--par` | 有生成、控制面、前端正负例 | 用户确认其他格式已跑通；任务地图仍写“真实格式输出归 EX-V1”，缺逐格式仓库证据 | 代码路径已具备 | PARTIAL | **实验支持**；若现有运行记录可映射则直接提升 |
| ORC | 已接入，显示内存风险，禁用压缩/拆分 | `--orc` | 有生成、控制面、前端正负例 | 用户确认其他格式已跑通；缺逐格式仓库证据 | 代码路径已具备 | PARTIAL | **实验支持**；补内存/输出回归后决定正式支持 |
| AVRO | 已接入，结构化格式限制 | `--avro` | 有生成、控制面、前端正负例 | 用户确认其他格式已跑通；缺逐格式仓库证据 | 代码路径已具备 | PARTIAL | **实验支持**；补现有证据映射后决定正式支持 |

### 4.3 内容组合边界

| Content | Current | v1 口径 |
|---|---|---|
| DATA_ONLY | 七种数据格式均有实现路径 | 按上表分正式/实验支持 |
| DDL_ONLY | 无数据格式，生成 `--ddl` | 正式支持候选；需要纳入 Golden Path |
| DDL_AND_DATA | 当前控制面和前端只允许 CSV | v1 明确写成“DDL + CSV 数据”，不能沿用支持矩阵中暗示其他格式可组合的旧描述 |

### 4.4 输出位置边界

| Output | Current Status | v1 口径 |
|---|---|---|
| LOCAL | STABLE | 正式支持，节点绝对路径原样传递 |
| OSS/S3/COS/OBS | PARTIAL | 配置、凭据槽位和预检查框架已实现，但真实凭据认证及远端结果取证未完成；不作为 v1 正式可用能力声明 |

## 5. Golden Path Baseline

Golden Path 的角色从“开发目标”改为“回归基线”。

### 5.1 基线定义

```text
Existing Authorized DataSource
→ Select Content / Scope / Objects
→ Select Supported Format
→ Configure Local Output and Execution Host
→ Server-side Validation
→ Command Preview
→ Agent Precheck
→ Submit Frozen Task
→ Agent Starts OBDUMPER
→ Process / Logs / State
→ Result Facts
→ Task History
→ Re-run or Rebuild
```

### 5.2 固定回归场景

| ID | 场景 | 目的 |
|---|---|---|
| GP-01 | 指定单表 CSV，本地输出 | 保留最早真实路径和 v5/v6/v7 兼容语义 |
| GP-02 | 指定多表 CSV，本地输出 | 覆盖通用对象范围与文件清单 |
| GP-03 | DDL_ONLY，TABLE 或 VIEW | 覆盖内容模式、无数据格式和 DDL 结果 |
| GP-04 | 每个 v1 正式格式至少一个 DATA_ONLY 场景 | 防止多格式入口退化为只有命令测试 |
| GP-05 | 预检查失败，不创建任务、不启动工具 | 保护失败关闭与无副作用边界 |
| GP-06 | 工具失败，页面显示状态、脱敏日志和错误摘要 | 保护失败可诊断性 |
| GP-07 | 历史任务“基于原配置新建” | 验证非敏感配置可恢复、凭据/预检查不复制 |
| GP-08 | 历史任务“从头重新执行” | 验证配置指纹不被修改 |
| GP-09 | Agent/控制面重启与日志续传 | 保护至少一次投递、重复和恢复语义 |

### 5.3 每次基线需要保存的非敏感事实

- 目标格式、内容模式、对象范围和输出类型。
- config schema version、metadata version、capability version。
- config fingerprint、planned command fingerprint。
- 任务 ID、执行 ID、节点 ID 的不透明标识。
- 进程终态、工具终态、结果摘要、日志完整性状态。
- 文件数量、总字节数、受限相对路径；不保存业务内容。
- 真实测试授权、环境和证据位置；不保存端点或秘密。

## 6. Current Architecture

### 6.1 当前真实链路

```text
ExportWizardView
  → ExportDraftInput / ExportDraft
  → ExportConfig
  → controlplane normalizeExportConfigV6
  → controlplane generateExportDraft
  → commandgen.Generate
      ├─ redacted command preview
      ├─ planned argv
      ├─ secret slots
      └─ config fingerprint
  → precheck / submit
  → immutable TaskSubmission
  → SQLite task + execution state
  → authenticated Agent lease
  → agentexecution.Worker
  → direct Java OBDUMPER process
  → events + double-redacted logs + result facts
  → task overview / snapshot / command / execution / logs
  → rebuild / rerun / checkpoint resume / template
```

### 6.2 架构判断

| 层 | 判断 |
|---|---|
| Vue 向导 | 只构造结构化输入，不拼 CLI，方向正确 |
| 控制面 | 权限、归一化、能力选择、预检查和冻结职责正确；实现位置过度集中 |
| ExportConfig | 已经是通用配置核心，不应再创建并行 `ExportJobConfig` |
| commandgen | 确定性、元数据驱动、双输出和秘密槽位设计正确，是必须保留的核心 |
| Task / Execution | 不可变提交与租约状态机方向正确；当前一任务对应一次执行，可充当 ExportRun |
| Agent | 固定信封、主动出站、直接 Java、无 Shell、只上报事实，方向正确 |
| Logs | 可靠队列、双层脱敏、持久化和 SSE 续传正确 |
| Result | 已有最小事实模型，但产品投影不完整 |

### 6.3 目标职责，而非目标命名

```text
ExportDraft aggregate (DataSource + Node + ExportConfig)
        ↓
Export domain validation and capability selection
        ↓
CommandBuilder (existing commandgen)
        ↓
DumperCommand result (argv + redacted view + fingerprint + secret slots)
        ↓
TaskSubmission / TaskExecution
        ↓
Execution Host / Agent / OBDUMPER
        ↓
Execution facts + logs + result summary
        ↓
Task history and derivation
```

不要求为了匹配图中的名称重命名现有类型。职责稳定比名称统一更重要。

## 7. ODC Reference Analysis

参考来源：[OceanBase ODC V4.5.0“导出结构和数据”](https://www.oceanbase.com/docs/common-odc-1000000006663517)，页面更新时间 2026-08-03。

ODC 只作为 UX 和信息架构参考。OBDUMPER 4.3.5 仍是本项目能力边界。

### 7.1 ODC Reference Matrix

| 维度 | ODC V4.5 | Current | Export v1 取舍 |
|---|---|---|---|
| 导出内容 | 结构+数据、仅数据、仅结构 | 已有同等三类内容 | 保留当前语义 |
| 导出范围 | 部分/整库；部分支持对象搜索选择 | ALL/SPECIFIED；指定对象主要手工输入 | 借鉴搜索、已选对象和类型过滤，不复制 ODC 页面结构 |
| 格式 | CSV、SQL | 七种 OBDUMPER 格式 | 保留多格式差异化，按正式/实验分级 |
| 编码 | 多种编码显式选择 | fileEncoding 可配置，默认多以占位提示 | 只列 OBDUMPER 已验证值；未选择表示继承工具默认 |
| 文件拆分 | 单文件上限 | `blockSize`、`maxFileSize` | 用用户语义解释“单文件拆分”和“总量上限”，避免直接堆 CLI 名称 |
| CSV 设置 | 表头、空串、分隔符、识别符、换行 | 配置更完整，含 quote mode、escape、NULL、trim | 保留专业能力，默认折叠 |
| SQL 设置 | 批量 COMMIT 数 | 当前未实现对应正式能力 | 不因 ODC 存在而新增，受 OBDUMPER 4.3.5 与 v1 需求约束 |
| 一致性 | 全局快照 | `snapshot`、flashback 等 | 只开放已验证组合，明确互斥 |
| DDL 设置 | 合并 SQL、Create 前 Drop | 当前有 drop/retain/compact 等部分能力 | 保留已验证项；不复制 ODC 的“合并文件”能力 |
| 执行方式 | 手动、立即、定时 | 提交后立即进入固定队列 | v1 保持立即/队列执行；定时任务 Not Now |
| 配置复用 | 保留当前配置、克隆 | 模板、基于原配置新建、从头重跑 | 当前项目能力更适合审计与复现，继续保留 |
| 任务详情 | 基本信息、任务流程、日志 | 多个安全投影、持久日志和结果摘要 | 合并为更清楚的信息层级，不降低安全投影 |
| 日志 | 全部/告警，查找、下载、复制 | 当前任务日志、SSE、持久化；缺告警筛选/下载入口 | v1 优先错误摘要、来源筛选和受控下载，不建设日志分析平台 |
| 结果 | 下载导出包 | 节点输出路径、文件摘要，页面不下载业务结果 | 保持执行节点输出管理，不引入浏览器大文件下载 |
| 限制 | Web 导出压缩前 2GB、最多 5 并发、14 天保留 | 依赖 OBDUMPER 和执行节点，适合更大数据 | 这是本项目差异化，不复制 ODC 限制 |

### 7.2 可借鉴内容

- 先回答“导什么”，再进入格式和文件设置。
- 对象选择提供搜索、类型和已选集合，而不是要求用户记住表达式。
- 常用设置与高级设置分层。
- 任务详情把基本信息、执行过程、日志和结果分区。
- 克隆/保留配置是用户可理解的复用入口。

### 7.3 不应复制的内容

- ODC 仅 CSV/SQL 的格式边界。
- 浏览器下载大结果包。
- 审批、定时和工单体系。
- ODC 自身 2GB/5 并发/14 天限制。
- ODC 页面布局或字段命名本身。

## 8. Export v1 Scope

### 8.1 Product Goal

Export v1 的正式产品目标是：

> 用户无需掌握 OBDUMPER CLI，即可通过向导安全完成一次数据库导出；同时，获授权 DBA 能核对非敏感冻结配置、实际执行的 OBDUMPER 参数证据、执行节点、强制脱敏的来源日志、退出码、输出结果、运行历史和重新执行关系。

普通用户需要明确回答“从哪里导、导什么、怎么导、导到哪里、是否成功、失败原因和结果在哪里”；DBA 需要在不突破秘密与路径授权边界的前提下核对 Config、CLI、Execution Host、Logs、Exit Code、Output、History 和 Re-run。

### 8.2 In Scope

- 选择已有、授权、启用且具有成功连接测试事实的数据源。
- 选择执行节点，并在提交前验证节点、Agent、工具、路径和空间事实。
- 内容：仅结构、仅数据、结构+CSV 数据。
- 范围：整库或指定对象。
- 对象：TABLE；VIEW 仅 DDL；多对象与排除表。
- 本地绝对路径输出。
- 正式格式：CSV；以及在 Phase 1 完成证据映射后被提升的 CUT/SQL/POS。
- 实验格式：尚未满足正式证据门槛的 Parquet/ORC/Avro。
- 已验证且确有业务语义的序列化、压缩、文件布局、筛选、一致性、性能和 DDL 参数。
- 服务端权威校验、命令预览、预检查、任务创建、队列、执行、状态、日志、结果和历史。
- 基于原配置新建、从头重新执行、成功任务保存模板。
- Windows AMD64 当前已验证运行边界。

### 8.3 Partial Scope

- 对象选择器：v1 最低可接受手工多对象输入，但应补搜索/类型过滤/已选对象；大规模对象浏览若需要数据库元数据枚举，必须单独评估权限和性能。
- 结果：当前文件摘要可用；输出位置、耗时、退出码、错误摘要和实际命令一致性证据需补齐。
- Cancel：当前未实现，但本任务要求纳入 DoD；需要定向实现而不是只展示状态。
- 检查点继续：资格判断和任务创建已实现，真实 `dump.ckpt` 续跑仍缺取证。
- POS：v1 只承诺用户提供控制文件目录，自动生成控制文件为 Future。
- 结构化格式：按实验支持进入，直到每种格式有完整证据映射和回归。

### 8.4 Out of Scope

- 对象存储正式可用声明，直到真实认证探测和远端结果取证完成。
- 未验证对象类型和跨库 `schema.object` 能力。
- 任意 SQL、SQL 编辑器、SQL 文件浏览或 `file://` 查询输入。
- 未具备 `CAP_SENSITIVE_COMMAND`、二次确认和审计的 `--query-sql`。
- 定时任务、审批流、浏览器大文件下载。
- 自动性能推荐、耗时预测、容量规划和全库对象扫描。
- 普通导入、旁路导入或为其预建的通用数据迁移框架。
- 三个麒麟目标、生产发布和通用跨数据库能力的无证据声明。

### 8.5 Future

- POS 控制文件自动生成。
- 对象存储真实认证与远端 MANIFEST/对象清单核验。
- `dump.ckpt` 真实继续。
- 其他对象类型和跨库导出。
- 经完整权限、确认、审计后开放的受限 `--query-sql`。
- 麒麟目标原生认证。

## 9. Export v1 DoD

Export v1 只有在下列 MUST 条件全部满足时才能形成正式基线。实验格式可按独立矩阵例外，不得模糊整体状态。

### 9.1 Product / DataSource / Content

| DoD | Current | Gap |
|---|---|---|
| 用户能选择已有授权数据源 | 已满足 | 无 |
| 数据源连接事实可验证且绑定当前修订/节点 | 已满足 | 纳入 Golden Path |
| 数据源兼容模式驱动格式/字段适用性 | 部分满足 | 将规则归属收敛到服务端领域校验 |
| 内容、范围、对象和格式边界在单一文档明确 | 未满足 | 文档收敛 |

### 9.2 Validation

| DoD | Current | Gap |
|---|---|---|
| required/range/dependency/conflict 服务端失败关闭 | 基本满足 | 从 handler 提取领域校验，并建立规则表测试 |
| 数据源能力和兼容模式校验 | 部分满足 | 明确已支持模式，未知值失败关闭 |
| 输出路径语法与节点实际事实分别校验 | 已满足 | 消除重复文案/规则漂移 |
| 执行节点在线、工具、路径、空间事实 | 已满足 | 纳入回归 |
| 高风险能力具有权限、二次确认、失效和审计 | 部分满足 | `query-sql` 的平台开关、授权、风险指纹、失效和事务审计已完成；其他高风险参数仍逐项定版 |

### 9.3 Command

```text
ExportDraft aggregate
        ↓
ExportConfig
        ↓
Authoritative Validator / Capability Selection
        ↓
existing commandgen.Generate
        ↓
planned argv + redacted command + fingerprint + secret slots
```

MUST：

- 前端不得拼 OBDUMPER 命令。
- 预览、预检查和提交必须使用同一配置、元数据版本和生成器。
- 提交必须冻结生成结果，Agent 执行冻结 argv，不重新解释用户表单。
- 页面必须能证明预览指纹与提交指纹一致。
- 密码和存储密钥不得进入 argv、预览、日志、错误、快照或 Git。

### 9.4 Execution

| DoD | Current | Gap |
|---|---|---|
| Task Create | 已满足 | 回归 |
| Queue / Lease | 已满足 | 回归重复领取、过期租约和重启 |
| Start / Running | 已满足 | 回归 |
| Success / Failed | 已满足 | 补退出码与稳定错误摘要投影 |
| Cancel | 未实现 | 设计固定取消请求、Agent 进程树终止、幂等、超时、检查点和结果语义；真实验证后才开放 |

### 9.5 Logs

- stdout 和 stderr 来源可区分或至少保留稳定来源事实。
- “raw log”指按原顺序保留的**已强制脱敏**工具日志，不允许未脱敏原文进入控制面。
- 日志重连、重复批次、缺口、终态补传和控制面重启可回归。
- 失败任务有稳定 error summary；没有足够证据时明确“原因待核对”，不推测。
- 下载若进入 v1，只允许下载已脱敏日志并复用同一授权与审计语义。

### 9.6 Result

MUST 展示：

- 状态。
- 授权范围内的安全输出位置摘要。
- 输出文件数、总字节数和受限相对路径。
- 开始、结束和可派生耗时。
- OBDUMPER 进程退出码或明确“不可用”。
- 稳定错误摘要。
- 结果核验语义及是否需要状态核对。

不把行数、校验和、远端对象清单列为本地 Export v1 的硬性 DoD，除非现有真实证据表明确支持。

### 9.7 History / Re-run

- 历史详情可以还原一次执行的非敏感配置、内容、对象、格式、输出摘要、执行节点、命令、状态、日志和结果。
- `Task + TaskExecution + immutable snapshot` 是当前 Export Run 事实，不新增重复 `ExportRun` 模型。
- 基于原配置新建可修改配置，但重新选择/复验凭据、节点、预检查和风险确认。
- 从头重新执行保持原配置指纹。
- 检查点继续只在 Agent 确认兼容检查点时提供。

### 9.8 Security Usability

| DoD | Current | Gap |
|---|---|---|
| S0/S1 开发路径不依赖真实凭据、真实网络、真实工具或人工审批即可完成 | 部分满足 | 固化假 Agent/假工具/临时目录 fixture，并提供一条可重复运行的安全开发路径 |
| 每项安全控制都有威胁、资产、执行层、成本和自动化负例 | 未形成统一要求 | 建立轻量控制清单；没有具体威胁或仅重复同层校验的控制不得新增 |
| 同一不可变风险指纹只确认一次，安全相关输入变化后确认自动失效 | 部分能力各自实现 | 统一风险指纹、有效期、撤销和失效语义，避免逐页面或逐内部步骤重复确认 |
| 安全阻断提供稳定错误码、缺失条件和可执行修复动作 | 部分满足 | 禁止只有“无权限”“预检查失败”的泛化提示 |
| `--query-sql` 只对具备 capability 的用户开放，并在最终提交时进行一次绑定当前指纹的二次确认 | 已满足（默认关闭） | 服务端和向导均要求当前风险指纹；真实授权器接入前继续失败关闭，普通编辑/预览不重复弹窗 |
| 硬边界由契约测试、秘密扫描和负例持续验证 | 基本满足 | 将身份隔离、秘密不落盘/不入 argv、任意命令拒绝和未授权真实执行纳入固定回归 |

这里的“开发友好”只缩减重复流程，不降低执行层约束。前端隐藏、跳过弹窗或本地开发开关都不能替代服务端授权、命令生成器安全复验和 Agent 固定执行边界。

## 10. Current Domain Model

### 10.1 现有模型映射

| 产品概念 | 当前类型/存储 | 判断 |
|---|---|---|
| ExportJobConfig | `ExportDraft` + `ExportConfig` + dataSourceId/nodeId | 已存在同等聚合，不新建平行模型 |
| Source | `ExportDraft.DataSourceID` + 数据源快照/凭据修订 | 正确地与非敏感配置分离 |
| Content | `ContentSelection` | 保留 |
| Objects | `ObjectScope` / `ObjectExpression` | 保留；支持矩阵需要收缩到当前真实对象类型 |
| Format | `DataFormat` | 保留并收敛格式子配置语义 |
| Output | `OutputConfig` | 保留；路径、文件布局、压缩和存储凭据引用当前混合，需要清楚分组 |
| Performance | `PerformanceConfig` | 保留 |
| Advanced | `FilterConfig` + `DDLBehavior` | 保留；高风险门禁需补齐 |
| Command | `commandgen.Result` | 保留，是唯一命令事实 |
| ExecutionTask | `TaskSubmission` / `TaskExecution` | 保留 |
| ExportResult | `ExecutionResultSummary` | 保留并扩展产品投影，不先新建大而全 Manifest 模型 |
| ExportRun | `tasks` + `task_executions` + events/logs/result | 已存在，不新增重复实体 |

### 10.2 领域模型主要问题

1. `ExportConfig` 是正确核心，但 dataSourceId/nodeId 位于草稿外层。产品文档应把 `ExportDraft` 视为聚合根，而不是强行把所有绑定塞入 `ExportConfig`。
2. `DataFormat.CsvOptions` 实际承载 CUT/SQL/结构化格式共用的文本配置，且包含 CUT 的 `ColumnSplitter`。这会让 CSV 继续成为整个格式模型的语义中心。
3. `configVersion = v6`、`metadataVersion = v7` 是两个不同版本轴，但文档和前端注释容易把它们混为一个“当前版本”。
4. `normalizedExportDraft` 是控制面内部的大型扁平模型，承担规范化、能力选择和命令字段装配，和 HTTP handler 放在同一 7248 行文件中。
5. `storedDraftConfigV6`、结构化列和 `ConfigJSON` 同时持久化，属于兼容与查询投影设计，但必须明确哪个是权威、如何做一致性复验。

### 10.3 收敛原则

- 保留 `ExportConfig` 名称和已持久化 v5/v6/v7 兼容语义。
- 不新增第二套 `ExportJobConfig`。
- 将“草稿聚合校验、能力选择、规范化配置、字段映射”移到明确的 Export domain/service 边界。
- 对格式配置优先形成清楚的内部语义：CSV 专属、CUT 专属、POS 专属、共享文本、结构化格式限制。
- 若要修改持久化 JSON，必须发布新 config schema version 并提供旧版本适配；不能静默改写历史草稿和任务。

## 11. Duplicate Truth Sources

| 主题 | 当前重复位置 | 风险 | 目标权威 |
|---|---|---|---|
| 格式枚举 | Go 类型、控制面分支、OpenAPI、TS 类型、前端校验、参数元数据、文档 | 新格式或状态变更容易漏改 | `ExportConfig`/OpenAPI 契约 + 参数元数据；前端通过契约/生成或 parity tests 对齐 |
| 格式适用性 | 前端 `exportDraftInput.ts`、控制面 `normalizeExportConfigV6`、commandgen activation、文档矩阵 | 同一组合出现不同判断 | 服务端 Export domain 负责产品语义；commandgen 负责发射安全复验 |
| 默认值 | 页面 placeholder、前端状态、参数元数据、设计文档、测试 fixture | “官方默认”“平台默认”“示例值”混淆 | 产品显式默认由 Export domain 定义；未设置统一表示继承 OBDUMPER；官方默认证据留在参数元数据/测试文档 |
| 路径规则 | 前端 regex、控制面 outputpath、commandgen outputpath、Agent allowed roots | Windows 表达和错误文案漂移 | 共享测试向量 + 各层职责：前端提示、控制面语法、Agent 事实 |
| 对象规则 | Wizard 即时提示、前端构造、控制面、参数元数据、文档 | VIEW/ALL/DDL 组合易不一致 | 服务端领域校验；前端为投影 |
| 状态 | OpenAPI enum、store 状态机、UI label、过程事件、文档低保真 | 枚举存在会被误认为操作已实现 | 持久任务状态机为权威；事件为证据；UI 只映射；文档区分状态和操作 |
| 结果 | `ExecutionResultSummary`、技术契约中的大 Manifest、任务详情文案、证据文档 | 文档声称行数/校验和但代码没有 | 当前结果类型与 Agent 事实为权威，Future 字段不得写成现状 |
| 历史/重跑 | task derivation、模板、低保真、任务地图、HANDOFF | 操作边界和完成状态不一致 | Task derivation contract + Export canonical doc |
| 参数支持状态 | parameter mapping、field rules、support matrix、metadata v5-v7、task map | ENABLED/VALIDATION_GATED 状态冲突 | 运行时参数元数据 + Export v1 support table；历史研究归档 |

## 12. Validation Assessment

### 12.1 应保留的分层校验

重复不等于所有层都应删除。以下层次具有不同职责：

| 层 | 合理职责 | 不应承担 |
|---|---|---|
| 前端 | 即时反馈、条件显示、避免无效请求 | 最终授权、最终能力判断、秘密或主机事实 |
| Export domain | required/range/dependency/conflict、内容/对象/格式/输出语义、能力选择 | 真实节点文件/进程/数据库事实 |
| commandgen | 元数据版本、活动参数、互斥、顺序、argv、脱敏、秘密槽位和指纹 | 产品 UI 流程或调度策略 |
| Agent precheck | 数据库连接、对象访问、工具、路径、空间、存储端点等节点事实 | 产品参数选择和最终任务状态 |

### 12.2 当前评价

- **优点**：服务端和 commandgen 均失败关闭；前端隐藏不能绕过服务端；Agent 不信任控制面无法证明的节点事实。
- **问题**：格式、压缩、时间格式、对象类型、路径表达和查询互斥在多个语言层手工复制。
- **问题**：`server.go` 中的错误多为内部英文字符串，再由通用 422 投影，缺少稳定字段级错误码，前端只能重复规则以获得可用文案。
- **问题**：高风险确认没有形成统一模型。`remove-newline`、`skip-check-dir` 有提示，但 `query-sql` 缺能力与二次确认门禁。

### 12.3 收敛方案

1. 建立服务端 `ExportValidator`/等价领域边界，输出规范化配置、能力版本和稳定 violation code。
2. commandgen 继续执行独立发射层复验，不能因为领域校验存在而削弱。
3. 前端保留轻量即时校验，但通过共享枚举、OpenAPI 生成或契约 parity tests 防止漂移。
4. 建立表驱动规则测试：每条 v1 规则至少覆盖服务端正例、负例和 commandgen 发射结果。
5. 将高风险能力统一为“所需 capability、确认版本、确认摘要、失效字段、审计动作”。

## 13. Command Builder Assessment

### 13.1 结论

`internal/commandgen` 是当前最成熟、最不应重写的部分。

### 13.2 必须保留

- 元数据版本化和历史资源不可变。
- capabilityVersion 激活参数子集。
- 结构化 `FieldInput`，不接受 Shell 字符串。
- 确定性参数顺序。
- planned argv 与 redacted command 同源产生。
- 密码只进入 secret slot/security file，不进入 argv。
- config fingerprint 和 token evidence。
- v5/v6 历史重放与 v7 当前资源分离。
- 未知、重复、非活动、门禁参数失败关闭。

### 13.3 当前缺口

- 字段到 `FieldInput` 的映射已收敛到 `internal/exportdomain.BuildGeneralizedFields`；控制面 `generateExportDraft` 只保留连接字段前缀、版本/能力事实和生成器调用。
- 预览和提交虽然都重走同一生成函数，但产品层尚未展示“预览指纹 = 提交指纹 = Agent 执行信封指纹”的证据链。
- 架构文档要求计划命令与实际命令证据分离；页面当前只展示 planned redacted command，没有退出码和 actual equality evidence。

### 13.4 目标

保留 commandgen，已实现一个薄的 Export adapter：

```text
Normalized ExportConfig
→ []commandgen.FieldInput
→ commandgen.Generate
→ DumperCommand result
```

adapter 不拼字符串，不拥有元数据规则，不读取秘密。

## 14. Execution Assessment

### 14.1 必须保留的实现

- 控制面不直接操作节点文件或进程。
- Agent 主动出站领取任务。
- 固定任务类型、租约、epoch、binding digest 和幂等回执。
- 直接 Java 启动固定 OBDUMPER 主类，不经过 Shell。
- execution 私有目录、安全配置和最小环境。
- 日志第一层脱敏后才能进入队列。
- 租约续期、重复/乱序事件、旧 epoch 拒绝和重启核对。
- 本地结果扫描只回报受限事实。

### 14.2 当前缺口

| Gap | 影响 | v1 处理 |
|---|---|---|
| Cancel 无 API/Agent 语义 | 状态枚举与实际操作不一致 | 定向实现并验证，或从 v1 DoD/界面/声明中明确移除；本计划建议实现 |
| actual exit code 未进入浏览器执行投影 | DBA 无法直接核对进程结果 | 增加安全数值投影 |
| planned/actual 命令关系不可见 | 不能直观看到 Agent 是否执行冻结命令 | 投影信封/命令指纹一致性，不暴露敏感 JVM argv |
| Windows 真实运行已验证，麒麟未认证 | 容易外推平台支持 | v1 发布声明按平台分级 |
| 对象存储结果便利性例外 | 可能被误读为远端文件已核验 | 页面和文档必须明确“进程/终态验证，不含远端对象清单” |

## 15. Result / History Assessment

### 15.1 当前已有

- Task overview：类型、状态、节点、来源关系和时间。
- Snapshot：数据源、节点、对象摘要、格式、版本和指纹的安全投影。
- Command evidence：planned redacted command。
- Execution：状态、开始/结束时间、reconciliationRequired、结果摘要。
- Logs：持久化双层脱敏日志和 SSE 续传。
- Result summary：VERIFIED/FAILED、文件数、总字节、相对文件清单、checkpointPresent、observedAt。
- History actions：重建草稿、从头重跑、检查点继续、保存模板。

### 15.2 不足

- 任务详情硬编码“单表 CSV 导出任务”，与多格式事实冲突。
- 页面不能查看完整的非敏感冻结配置，只能看到最小摘要。
- 输出位置已经按主体裁剪：任务创建者可读取完整路径/受控 URI，其他任务操作授权者只读取 LOCAL/OSS/S3/COS/OBS 类型。
- 任务详情已投影受控进程退出码、由开始/结束时间派生的耗时、固定错误码和稳定中文摘要。
- 失败摘要优先使用控制面持久化的固定错误码；历史任务或证据缺失时才回退到已脱敏日志，并明确保持未知。
- `stageEvidence` 和 `progressEvidence` 明确 UNAVAILABLE，这是正确的保守行为，但文档仍有完整阶段/进度设计容易造成误读。
- 技术契约中的行数、对象数、校验和和通用 MANIFEST 是目标设计，不是当前实现。

### 15.3 v1 收敛判断

- 不新增独立 `ExportRun` 表。当前一条 task 对应一次 execution，已经满足运行记录主键需求。
- 扩展安全读取投影，而不是复制数据模型。
- 错误摘要由控制面根据固定事件/退出码形成结构化字段，并保留“不可确定”状态；不把原始工具错误写入结果投影。
- 输出位置按权限返回：创建者可看完整或受控值，其他授权读者继续隐藏具体路径；不能用错误差异泄露无权路径。
- planned/actual 只返回 argv SHA-256 摘要比较结果；摘要不替代命令正文，也不允许 Agent 以不一致证据形成成功终态。
- elapsed time 从可靠 startedAt/finishedAt 派生即可，不必持久化重复字段。

## 16. UX Assessment

### 16.1 六步还是五步

当前六步：

```text
数据源 → 对象 → 内容 → 格式 → 执行与输出 → 预检查与命令
```

用户建议的五步：

```text
数据源 → 内容与对象 → 导出设置 → 性能与高级 → 确认与执行
```

结论：**不机械改成五步**。当前六步与已经验证的页面和代码一致，且“对象”和“内容”各有独立条件规则。更合理的收敛方式是：

- 保留六步导航，减少步骤 5 的过载。
- 在步骤 2/3 的页面信息层级上把“导什么”连贯表达。
- 将步骤 5 内的常用文件设置和高级参数明显分层。
- 等对象选择器和格式配置稳定后，再用可用性测试决定是否合并步骤。

### 16.2 各步骤判断

| Step | Current | v1 调整 |
|---|---|---|
| 1 数据源 | 资格过滤正确，但展示信息有限 | 展示数据库类型、连接方式、租户/集群、数据库/Schema、连接状态；不暴露密码或过早出现 Dump 参数 |
| 2 对象 | ALL/SPECIFIED + 手工行输入 | 补搜索、类型过滤、已选对象和批量移除；全选仅在权限与性能可证明时提供 |
| 3 内容 | 三选一和 DDL 行为 | 保留，明确 DDL+数据仅 CSV |
| 4 格式 | 七格式卡片，配置分散在后续步骤 | 每种格式展示适用范围、正式/实验状态和专属设置；不再用 CSV 术语承载共用文本配置 |
| 5 执行与输出 | 输出、节点、序列化、筛选、性能、DDL 高级项集中 | 首屏只放输出和节点；高级组默认折叠，按业务语义组织 |
| 6 确认 | 摘要、预检查、命令和提交 | 补风险确认、命令/提交指纹一致性、实验格式标识和真实结果预期 |
| Task Detail | 安全投影完整但信息不足 | 修正“单表 CSV”标签，补非敏感配置、执行节点摘要、退出码、耗时、输出与错误摘要 |

### 16.3 不做的 UX 工作

- 不重做视觉系统。
- 不复制 ODC 页面。
- 不为全部 109 个 OBDUMPER 参数做表单。
- 不显示虚假进度或 ETA。
- 不把任务详情变成日志分析平台。

## 17. Keep / Consolidate / Refactor / Remove

### 17.1 KEEP

| 代码/能力 | 原因 |
|---|---|
| `internal/commandgen/` | 唯一、确定性、版本化命令权威，安全边界正确 |
| `internal/parammeta/` 及 v1-v7 历史资源 | 历史重放和参数支持状态的关键事实；旧资源不可改写 |
| `internal/store` 的不可变任务、草稿 revision、租约、事件和结果摘要 | 已形成并发、幂等和恢复语义 |
| `internal/agentexecution/`、`internal/agentexec/` | 固定进程启动和执行事实链路已经真实验证 |
| `internal/agentwire/`、`internal/agentstate/` | 机器身份、租约和重放边界 |
| `internal/logstream/`、`internal/agentlogqueue/` | 双层脱敏、可靠日志与重启恢复 |
| 凭据加密、secret slot、安全文件 | 密码不入 argv/日志的核心保障 |
| precheck 固定检查和失败关闭 | 防止无效或越权执行 |
| v5/v6/v7 与 snapshot v1/v2 兼容逻辑 | 保护历史任务和草稿 |
| Task derivation、checkpoint 资格和模板去凭据逻辑 | 已有历史复用价值，不应重写 |
| OpenAPI 安全投影和浏览器二次泄露检查 | 防止服务端回归时前端直接展示秘密 |

### 17.2 CONSOLIDATE

| 目标 | 内容 |
|---|---|
| 格式支持事实 | 将 UI、Go、OpenAPI、参数元数据和文档的格式/组合状态对齐到一张 v1 矩阵 |
| 默认值语义 | 统一“平台默认 / 用户显式 / 继承 OBDUMPER / UI 示例” |
| 前后端校验 | 服务端输出稳定 violation code；前端只做投影和即时反馈 |
| 路径规则 | 建立 Windows/Linux 共享测试向量，保留 Agent 真实检查 |
| 状态语义 | 区分状态枚举、可用操作和底层过程事件 |
| 结果/历史 | 在现有 task 投影上补字段，不复制 Run 模型 |
| 文档 | 收敛为 Export 产品、技术、测试三份专项 Canonical Doc |

### 17.3 REFACTOR

只有下列位置达到“Consolidate 不足”的门槛：

1. **控制面 Export 领域逻辑拆分**：已完成第一阶段定向提取。`internal/exportdomain` 负责选择归一化、能力/显示格式和 `ExportConfig → commandgen.FieldInput` 纯适配；控制面保留版本、连接/凭据/输出/节点事实和生成器生命周期，不改变 API、存储和命令输出。后续只在出现新的职责过载证据时继续拆分。
2. **格式配置语义**：`CsvOptions` 被多格式复用并包含 CUT 字段。需要建立清楚的内部格式配置边界；若改持久化结构，必须以新 config version 兼容迁移，不能原地改 v6。
3. **高风险能力门禁**：`query-sql` 已接入能力、二次确认、风险指纹失效和事务审计；默认部署仍失败关闭，待真实授权器与现场证据后再启用。
4. **Cancel 状态机**：若作为 v1 DoD，必须实现控制面请求、Agent 取消、进程树终止、重复请求、租约、检查点和终态证据，不能靠现有枚举补 UI。
5. **任务结果证据**：退出码、错误摘要、输出位置权限投影和 actual/planned 一致性需要结构化契约，单纯修改页面文案不够。

### 17.4 REMOVE

当前分析阶段**没有任何生产代码被证明可以立即删除**。以下仅是候选：

- 任务详情中的“单表 CSV”硬编码应替换，不是删除任务详情。
- OpenAPI 中 `POST /export-config-templates` 标记 `NOT_IMPLEMENTED` 的手工模板入口，应在 v1 Scope 决定后选择实现或从 v1 契约表面移除，不能长期占位。
- 旧元数据版本、旧迁移、历史 snapshot decoder、真实验证 fixture 不得删除。
- mock/fixture 只有在 reachability、测试替代和唯一知识检查通过后才可删除；本轮没有足够证据批准删除。

## 18. Documentation Inventory

### 18.1 核心与入口文档

| File | Purpose | Current / Historical | Duplicate With | Conflict With | Unique Knowledge | Recommended Action | Target Document |
|---|---|---|---|---|---|---|---|
| `AGENTS.md` | 项目治理、安全和验证边界 | Current | 部分安全契约 | 无，以其为高优先级规则 | 不可越权与真实执行门禁 | CANONICAL | 保持治理入口 |
| `README.md` | 仓库入口和当前能力摘要 | Current but stale | HANDOFF、任务地图 | 迁移仅到 0013、单表 CSV、日志内存等描述已过期 | 快速启动入口 | CANONICAL / REWRITE | 仓库入口，只保留简明当前状态和权威链接 |
| `HANDOFF.md` | 历史交接与本机运行快照 | Mixed / Historical | 任务地图、证据、README | 文件内部“模板待做/已完成”“单表 CSV/多格式”并存 | 2026-08 本机演进和运行上下文 | ARCHIVE after merge | `docs/archive/export/handoffs/` |
| `docs/README.md` | 文档导航和状态表 | Current but stale | 各模块状态 | 多处仍写待实现/待执行 | 全项目导航 | CANONICAL / REWRITE | 只做导航，不复制状态详情 |
| `docs/01-product/product-scope.md` | 全局产品范围 | Current but stale | PRD、export module | 仍称当前切片仅单表 CSV、尚未进入业务开发 | 全局产品定位与非目标 | CANONICAL / UPDATE | Product canonical |
| `docs/01-product/decisions.md` | 已确认决策日志 | Current | 各专项规则 | 可能与后续实现状态脱节 | 决策来源与编号 | CANONICAL / UPDATE | 决策记录，不作为现状手册 |

### 18.2 Export 专项文档

| File | Purpose | Current / Historical | Duplicate With | Conflict With | Unique Knowledge | Recommended Action | Target Document |
|---|---|---|---|---|---|---|---|
| `docs/02-design/export-module.md` | 产品范围、六步向导、规则和决策 | Current design, mixed status | field rules、support matrix、low fidelity | 设计能力多于当前正式能力；部分高风险门禁未实现 | EX-R01~R17 产品决策 | CANONICAL / REWRITE | Export product canonical |
| `docs/02-design/export-low-fidelity.md` | 页面状态与交互草图 | Historical design input | export module、core flow | 与当前真实页面结构和能力状态不完全一致 | 页面异常态和交互意图 | MERGE | `export-module.md`，合并后删除源文件 |
| `docs/02-design/export-field-rules.md` | 87 字段显示/条件/支持状态 | Mixed | parameter mapping、support matrix、代码校验 | 部分字段状态已被代码和实测改变 | 字段级条件和编号 | MERGE | 产品语义进 `export-module.md`；机器规则进 `export-general-contract.md` |
| `docs/02-design/export-parameter-mapping.md` | 109 参数研究与产品处置 | Historical research + partial current | metadata、field rules、general contract | 文档支持状态可能落后于 v7 runtime metadata | 全量参数来源与研究过程 | ARCHIVE after extracting v1 subset | `docs/archive/export/research/` |
| `docs/02-design/export-v1-support-matrix.md` | 能力和格式矩阵 | Mixed / stale | module、field rules、task map | POS、结构化格式、DDL+数据、MANIFEST 等多处与代码不一致 | 早期 v1 组合设计 | MERGE | 产品矩阵进 `export-module.md`；验证矩阵进 testing canonical |
| `docs/02-design/export-controlled-validation.md` | 原始受控验证计划 | Current target, stale status | evidence docs、task map | 顶部仍称尚未执行，实际多项已完成 | 安全验证方法与用例 ID | CANONICAL / REWRITE | Export testing canonical |
| `docs/02-design/core-flow-low-fidelity.md` | 全项目核心流程低保真 | Cross-cutting | export low fidelity | 当前 Export 页面已超出早期静态基线 | 跨模块跳转基线 | KEEP / UPDATE LINKS | 全局 UX 文档，不作为 Export 现状入口 |
| `docs/02-design/user-flow.md`、`interaction-spec.md` | 全局流程与交互规范 | Cross-cutting | 各模块文档 | 可能仍是设计期口径 | 跨模块一致交互 | KEEP | 全局设计 canonical |

### 18.3 技术与契约文档

| File | Purpose | Current / Historical | Duplicate With | Conflict With | Unique Knowledge | Recommended Action | Target Document |
|---|---|---|---|---|---|---|---|
| `docs/03-technical/architecture.md` | 全局架构与不变量 | Current but slice-oriented | 各专项技术契约 | 首条切片限制与当前实现范围不同 | 模块化单体 + Agent 总体边界 | CANONICAL / UPDATE | Architecture canonical |
| `docs/03-technical/technology-stack.md` | 技术栈和平台 | Current | architecture、readiness | 局部阶段状态会过期 | 正式平台范围 | CANONICAL / TRIM STATUS | 技术栈 canonical |
| `docs/03-technical/export-general-contract.md` | 通用导出技术契约 | Current design + implementation chronicle | parameter contract、task map、support matrix | 大 Manifest/结果模型和部分 payload 状态超前于实现 | 通用配置、能力版本、命令、预检查、结果设计 | CANONICAL / REWRITE | Export technical canonical |
| `docs/03-technical/ex-d0-implementation-inventory.md` | 2026-08-05 单表 CSV 盘点 | Historical | 本计划、task map | 大量“未实现”项已经完成 | 兼容边界和演进起点 | ARCHIVE | `docs/archive/export/audits/` |
| `docs/03-technical/development-task-map.md` | 项目唯一开发顺序 | Current + large history | HANDOFF、证据、专项文档 | 顶部 I1-I8 完成，但第 2 节仍称当前代码是 CSV_SINGLE_TABLE_V1 | 阶段门禁和历史实施记录 | CANONICAL PROJECT PLAN / COLLAPSE | 保留当前 workstreams；历史段移 archive |
| `docs/03-technical/development-readiness-closure.md` | G0-G4 门禁 | Current but historical detail heavy | task map、G3 docs | 当前状态分散 | 发布门禁 | KEEP / UPDATE | 全局 readiness canonical |
| `docs/03-technical/parameter-command-contract.md` | 参数和命令不变量 | Current, slice-oriented | export general contract | v4/v5 基线描述可能过期 | 确定性、路径、指纹、秘密槽位原则 | KEEP / UPDATE | Cross-cutting command contract |
| `docs/03-technical/agent-task-state-contract.md` | Agent、租约、状态机 | Current | API/SQLite、task map | 部分历史阶段描述 | 幂等、乱序、重启契约 | KEEP | Cross-cutting canonical |
| `credential-access-security-contract.md` | 凭据和权限 | Current | AGENTS、export general | query-sql 当前代码门禁缺口 | 秘密生命周期和授权边界 | KEEP / UPDATE GAP | Cross-cutting canonical |
| `tool-launch-isolation-contract.md` | 进程启动隔离 | Current | Agent contract | 无主要方向冲突 | 固定 Java 与进程边界 | KEEP | Cross-cutting canonical |
| `log-collection-evidence-contract.md` | 日志与证据 | Current | task docs | “raw log”需统一为已脱敏来源日志 | 双层脱敏、Gap、游标 | KEEP | Cross-cutting canonical |
| `api-sqlite-data-contract.md` | API/SQLite | Current but growing | migrations、OpenAPI | 早期表数量/首切片描述过期 | 事务、迁移和数据边界 | KEEP / UPDATE | Cross-cutting canonical |
| `first-vertical-slice.md` | 初始单表 CSV 切片 | Historical | EX-D0、task map | 不再是当前产品范围 | 第一条链路的形成背景 | ARCHIVE | `docs/archive/export/first-slice/` |
| `g3-windows-entry-readiness.md` | Windows G3 准入 | Current gate + history | task map、evidence | 需要与 Export v1 分格式基线重新对齐 | WI 门禁 | KEEP / REWRITE INDEX | Readiness canonical |
| `docs/03-technical/README.md` | 技术文档导航 | Current but stale | docs README | 仍称 DEV-07 下一步、真实执行未开放 | 技术入口 | CANONICAL / REWRITE | 只做导航 |

### 18.4 代码邻近 README

| File | Purpose | Current / Historical | Duplicate / Conflict | Unique Knowledge | Recommended Action | Target |
|---|---|---|---|---|---|---|
| `contracts/README.md` | OpenAPI 状态摘要 | Stale | 仍称 29 浏览器 + 14 Agent、G2 only、无重试入口 | API 契约入口 | UPDATE | 与实际 OpenAPI operation 数和实施状态一致 |
| `migrations/README.md` | 迁移说明 | Stale | 只描述到 0012/22 表，边界仍称无模板/通用导出 | 迁移安全规则 | UPDATE | 覆盖 0013~0019，不复制全部 SQL |
| `internal/parammeta/resources/README.md` | 参数资源版本说明 | Stale | 仍称 v4 当前，实际当前 v7 且 v5/v6 需重放 | 资源不可变规则 | UPDATE | 明确 v1-v7 用途和当前选择规则 |

### 18.5 Export 证据文档

| File | Purpose | Current / Historical | Unique Knowledge | Recommended Action | Target Document |
|---|---|---|---|---|---|
| `windows-authorized-real-export-2026-08-04.md` | CSV 真实平台链路 | Historical evidence | 首次受控真实成功 | ARCHIVE + summary merge | testing canonical + `archive/export/evidence/` |
| `windows-csv-special-values-validation-2026-08-05.md` | CSV 特殊值 | Historical evidence | NULL/空串/逗号/引号/非 ASCII/换行 | ARCHIVE + regression extraction | testing canonical |
| `windows-pos-format-validation-2026-08-07.md` | POS 定版 | Historical evidence | 4.3.5 独立 `--pos`、控制文件、定长结果 | ARCHIVE + supported behavior extraction | testing canonical |
| `exi7-remaining-parameters-2026-08-11.md` | 高级参数第一批 | Historical evidence | 受控行为观察 | ARCHIVE + supported subset extraction | testing canonical |
| `exi7-remaining-parameters-2026-08-13.md` | 高级参数第二批 | Historical evidence | DATE/DATETIME/partition/type 行为与 gated 项 | ARCHIVE + supported subset extraction | testing canonical |
| `exi6-storage-precheck-framework-2026-08-14.md` | 存储预检查框架 | Historical implementation evidence | 存储门禁形成过程 | ARCHIVE | export technical/testing canonical |
| `exi8-derivation-result-recovery-2026-08-14.md` | 结果、派生、恢复 | Historical implementation evidence | 派生与 checkpoint 边界 | ARCHIVE | export technical/testing canonical |
| `windows-low-privilege-object-access-validation-2026-08-05.md` | 权限负例 | Historical evidence | 连接成功但对象访问失败关闭 | ARCHIVE + regression extraction | testing canonical |
| `windows-agent-interruption-recovery-validation-2026-08-04.md` | Agent 中断 | Historical evidence | 受控进程与核对状态 | ARCHIVE + regression extraction | testing canonical |
| `windows-terminal-log-replay-recovery-validation-2026-08-04.md` | 终态日志补传 | Historical evidence | 终态重放边界 | ARCHIVE + regression extraction | testing canonical |
| `windows-wi05-command-argv-validation-2026-08-05.md` | argv 取证 | Historical incomplete evidence | 未完成项和原因 | ARCHIVE，保留未决状态 | testing canonical risks |
| `first-vertical-slice-p0-2026-07-21.md` | 最初 P0 记录 | Historical | 早期环境发现与失败 | ARCHIVE | first-slice archive |
| tool/SQLite/credential/log/Java spikes | Cross-cutting evidence | Historical but reusable | 跨 Export/Import 的安全与平台事实 | KEEP in technical evidence; do not move solely for Export cleanup | Cross-cutting contracts |

### 18.6 tasks/、fixtures 和运行残留

- 仓库不存在 `tasks/` 目录。任务状态主要沉淀在 `development-task-map.md`、HANDOFF 和 evidence 文件中，这是任务与产品事实混写的根因之一。
- `tmp/` 下存在历史真实/受控输出和日志，但它们是忽略的本机运行材料，不是稳定测试 fixture，也不能提交为回归输入。
- 当前前端测试以合成对象和表单 fixture 为主；缺少跨层、版本化的 Export v1 fixture 套件。
- `.codegraph/` 是本机索引，不进入交付。

### 18.7 Export 文档为什么会变多

根因不是“团队喜欢写文档”，而是缺少完成后的知识回收机制：

1. **任务驱动产生新文档**：EX-D、EX-I、WI、spike 和现场验证通常各自创建文档，但任务完成后没有把长期结论回收到产品、技术和测试 Canonical Docs。
2. **阶段方案没有退出机制**：单表 CSV、通用骨架、完整设计和当前实现同时留在正常导航中，历史基线没有明确标成 Historical 或移入 archive。
3. **同一主题按视角重复设计**：module、field rules、parameter mapping、support matrix、low fidelity 和 general contract 都包含格式、参数、默认和状态，但没有唯一 owner。
4. **验证过程与支持结论混在一起**：每次真实验证独立成文是合理的取证方式，但缺少一张长期维护的 Supported Behavior / Regression / Known Limitations 汇总表。
5. **任务地图承担了实施历史数据库职责**：当前计划、完成记录、现场证据和旧开发顺序都写在一个文件中，导致它持续增长并复制其他文档。
6. **入口文档复制状态而不是链接事实源**：README、docs indexes、HANDOFF、邻近 README 都维护自己的“当前状态”，更新不同步后形成冲突。
7. **没有 post-task closeout 门禁**：任务完成时没有强制执行“更新 Canonical → 归档证据 → 删除临时计划 → 修复入口链接”。

治理方案必须修复上述流程，而不仅是本次移动文件。后续普通 Feature/Bug 默认更新 Canonical Doc、代码、测试和 Git 历史；只有需要长期独立维护或取证的主题才创建新文档。

## 19. Documentation Conflicts

| Conflict | Evidence | Resolution |
|---|---|---|
| 当前能力仍被描述为单表 CSV | README、product-scope、technical README、task map 第 2 节、HANDOFF 开头 | 统一改为“已实现通用 Export；正式 v1 格式按矩阵分级” |
| 迁移范围错误 | README 写 0001~0013；实际已有 0019 | 更新 README/migrations README，不在入口复制每个迁移细节 |
| 参数元数据当前版本错误 | resources README 写 v4 当前；实际新草稿使用 v7，v5/v6保留重放 | 明确三个版本轴和选择规则 |
| API 状态错误 | contracts README 写 G2 only、29+14 操作、无重试；实际有任务派生/模板/真实链路 | 由 OpenAPI tests 统计并更新摘要 |
| 日志持久性错误 | README 仍称控制面内存日志、重启不保留；当前已持久化并有重放证据 | 更新入口，保留历史证据到 archive |
| Export 实施状态自相矛盾 | task map 顶部 I1-I8 完成，第 2 节仍冻结 CSV_SINGLE_TABLE_V1 | 将第 2 节标成历史或移 archive，只保留当前状态 |
| HANDOFF 内部状态冲突 | 前部称模板下一步，后部称模板已交付；开头仍单表 CSV | HANDOFF 不再作为 current truth，归档 |
| 格式支持冲突 | support matrix 对 POS/结构化格式状态、DDL+数据组合与代码不一致 | 用本计划第 4 节矩阵替换后合并到 export canonical |
| 结果模型冲突 | export-general-contract 描述 rowCount/checksum/objectCount/通用 MANIFEST；当前类型明确禁止这些字段 | Canonical 技术文档区分 Current / Future，禁止混表 |
| 验证状态冲突 | export-controlled-validation 顶部称尚未执行，多项 evidence 已完成 | 重写为回归基线和未决矩阵 |
| Cancel 能力冲突 | OpenAPI/UI enum 有 CANCELLING/CANCELLED，设计文档有取消页面，但无取消端点 | 状态枚举不等于能力；v1 要么实现，要么明确移除操作声明 |
| query-sql 安全冲突 | EX-R07/AGENTS 要求能力+二次确认，历史实现曾直接允许生成 | 已补齐门禁契约；真实授权器未接入前继续失败关闭 |
| Wizard 步数口径 | 当前实现六步，用户建议评估五步，部分文档按不同分组 | 以当前六步为基线，不机械重做；Canonical 记录原因 |
| Golden Path 角色冲突 | 旧文档把它当待开发目标 | 改为稳定回归基线 |

## 20. Canonical / Merge / Archive / Delete

### 20.1 CANONICAL

长期理解 Export 只需以下五个入口，其中 Export 专项为三份：

1. `docs/01-product/product-scope.md`：全局产品、用户、V1 非目标。
2. `docs/03-technical/architecture.md`：全局架构和不变量。
3. `docs/02-design/export-module.md`：Export v1 产品范围、用户流程、能力/格式矩阵、UX 和产品规则。
4. `docs/03-technical/export-general-contract.md`：领域模型、校验、命令、执行、结果、历史和安全契约。
5. `docs/02-design/export-controlled-validation.md`：Golden Path、format matrix 证据、fixtures、回归、真实验证门禁和已知限制。

`decisions.md`、`development-task-map.md` 和各跨领域安全契约继续存在，但不应成为理解 Export 当前能力的必读前置。

### 20.2 MERGE

| Source | Target |
|---|---|
| `export-low-fidelity.md` | `export-module.md` |
| `export-field-rules.md` 的产品条件 | `export-module.md` |
| `export-field-rules.md` 的机器校验/失效规则 | `export-general-contract.md` |
| `export-v1-support-matrix.md` 的当前能力与格式矩阵 | `export-module.md` |
| `export-v1-support-matrix.md` 的验证矩阵 | `export-controlled-validation.md` |
| `export-parameter-mapping.md` 中 v1 已启用参数子集 | `export-general-contract.md` |
| 已完成 evidence 的长期结论 | `export-controlled-validation.md` |
| HANDOFF 中仍有效的当前能力与未决风险 | README、task map、三份 Export Canonical Docs |
| 本计划长期有效结论 | 三份 Export Canonical Docs |

MERGE 完成并验证链接后，源文档删除；删除动作属于合并收口，不是无依据清扫。

### 20.3 ARCHIVE

| Source | Archive | Reason |
|---|---|---|
| `HANDOFF.md` | `docs/archive/export/handoffs/HANDOFF-2026-08.md` | 保留阶段演进和本机运行背景，但不再参与当前事实判断 |
| `ex-d0-implementation-inventory.md` | `docs/archive/export/audits/` | 保存单表 CSV 到通用 Export 的演进起点和兼容边界 |
| `first-vertical-slice.md`、对应 P0 evidence | `docs/archive/export/first-slice/` | 历史切片有价值，但不再是当前产品范围 |
| `export-parameter-mapping.md` 全量 109 参数研究 | `docs/archive/export/research/` | 具有唯一研究价值，但不应把未启用参数带入日常上下文 |
| 完成的 Windows/EX-I evidence | `docs/archive/export/evidence/` | 保留真实取证和踩坑过程，Canonical 只保留结论/回归 |
| task map 中完成的 EX-D0~EX-I8 长篇实施记录 | `docs/archive/export/implementation-history.md` | 当前任务地图只保留 Workstream 和门禁 |

### 20.4 DELETE

当前没有任何跟踪中的历史文档可在未合并/未归档前直接删除。

明确可删除项：

1. `export-low-fidelity.md`、`export-field-rules.md`、`export-v1-support-matrix.md`：仅在上述 MERGE 完成、链接更新、唯一知识核对通过后删除。此时其内容已由 Canonical 完整承接。
2. 本文件 `EXPORT_V1_CONSOLIDATION_PLAN.md`：收敛完成且所有长期结论已合并后优先删除；若需要保留重大决策演进则归档，不允许继续作为日常事实源。
3. 分析过程产生的临时抓取、生成报告和无引用中间文件：确认不含唯一证据后删除，不进入 Git。

## 21. Target Documentation Structure

```text
docs/
├─ README.md                                  # 仅导航
├─ 01-product/
│  ├─ product-scope.md                       # Product canonical
│  └─ decisions.md                           # 决策日志
├─ 02-design/
│  ├─ export-module.md                       # Export product/UX canonical
│  └─ export-controlled-validation.md        # Export testing canonical
├─ 03-technical/
│  ├─ architecture.md                        # System architecture canonical
│  ├─ export-general-contract.md             # Export technical canonical
│  ├─ development-task-map.md                # 当前 workstreams/门禁，不承载产品事实
│  └─ cross-cutting contracts...             # Agent/credential/log/API/command
└─ archive/
   └─ export/
      ├─ audits/
      ├─ evidence/
      ├─ first-slice/
      ├─ handoffs/
      ├─ implementation-history.md
      └─ research/
```

### 21.1 信息所有权

| Question | Primary Document |
|---|---|
| 产品是什么、用户是谁、整体 v1 边界 | `product-scope.md` |
| 系统为什么是控制面 + Agent | `architecture.md` |
| Export v1 支持什么、页面怎么走、格式如何分级 | `export-module.md` |
| Config/Validation/Command/Task/Execution/Result 如何工作 | `export-general-contract.md` |
| 如何验证、Golden Path、fixture、格式证据和限制 | `export-controlled-validation.md` |
| 当前下一步做什么 | `development-task-map.md` |
| 为什么曾经这样做、真实验证过程是什么 | `docs/archive/export/` |

## 22. Workstreams

### WS-01 Export Domain Consolidation

目标：冻结 `ExportDraft` 聚合和 `ExportConfig` 权威，明确格式子配置、版本轴和兼容适配。

产出：

- Export v1 Scope 与格式分级。
- 服务端 Export domain 包/边界。
- v5/v6/v7 兼容测试。
- `CsvOptions` 跨格式语义的收敛方案。

### WS-02 Validation & Command Consolidation

目标：明确校验层级，消除前后端手工漂移，同时保留 commandgen 独立安全复验。

产出：

- 稳定 violation codes。
- 表驱动规则矩阵。
- ExportConfig → FieldInput adapter。
- preview/precheck/submit 指纹一致性测试。
- S0-S4 风险分类、控制清单和有界授权/确认指纹。
- `query-sql` 提交级风险门禁或失败关闭。

### WS-03 Execution / Result / History Consolidation

目标：不改变 Agent 基础架构，补齐 v1 可诊断性和运行记录。

产出：

- exit code、elapsed time、error summary、output location 安全投影（已完成合成闭环）。
- planned/actual equality evidence（已完成摘要核对和不一致失败关闭）。
- Cancel 的正式契约、进程树终止和终态证据仍单列为未实现项，不以状态枚举冒充。
- 历史配置回看和 re-run 回归。

### WS-04 Export UX Consolidation

目标：保留当前可用向导，收敛信息层级和多格式体验。

产出：

- 数据源摘要补全。
- 对象选择器最小增强。
- 格式专属设置和正式/实验标识。
- 高级设置分组与默认折叠。
- 安全阻断的稳定原因、修复动作和授权有效状态；移除同一风险上的重复提示/确认。
- 任务详情修正与证据补全。

### WS-05 Documentation Consolidation

目标：建立三份 Export 专项 Canonical Docs，移出历史上下文。

产出：

- Canonical rewrite。
- Merge/Archive/Delete 执行记录。
- README、docs indexes、contracts/migrations/parammeta README 同步。
- task map 只保留当前 workstreams 和门禁。

### WS-06 Regression & Compatibility

目标：将真实成功链路变成自动化和受控真实回归基线。

产出：

- Golden Path suite。
- 格式矩阵证据映射。
- v5/v6/v7 replay fixtures。
- S0/S1 无人工审批开发路径和 S2-S4 边界负例。
- CSV edge cases、Windows path、失败、重启、日志和重跑回归。
- v1 release evidence summary。

## 23. Implementation Sequence

### Phase 1: Baseline Freeze

1. 从现有任务历史和 evidence 中提取每个已跑格式的任务/命令指纹/输出/历史证据。
2. 冻结本计划第 4 节格式矩阵和第 5 节 Golden Path。
3. 建立版本化合成 fixture，不复制真实端点、凭据或业务数据。
4. 盘点当前安全门禁，按 S0-S4 标注威胁、执行层、重复点和开发成本。
5. 在任何重构前运行并记录现有 Go/前端回归。

停止条件：格式证据无法映射时保留“用户确认、仓库待映射”，不擅自降级或升级正式状态。

### Phase 2: Documentation Audit

1. 对第 18 节逐文件确认 owner、unique knowledge 和 target。
2. 建立 archive 目录，但暂不批量移动。
3. 在 task map 登记少量 Workstream，而不是新增几十个 WI。

停止条件：任何待删除文档仍含唯一事实时，先合并或归档。

### Phase 3: Domain & Rule Consolidation

1. 提取 Export domain validation/capability selection。
2. 明确 `ExportDraft` 聚合、`ExportConfig` 权威和版本适配。
3. 收敛格式子配置与默认语义。
4. 建立统一风险指纹、有界授权和确认失效语义，删除仅重复同一威胁的流程门禁。
5. 为 `query-sql` 建立失败关闭或完整提交级高风险门禁。

停止条件：v5/v6 历史草稿、任务、命令或指纹回归变化。

### Phase 4: Command / Execution Consolidation

1. 提取 ExportConfig → commandgen adapter。
2. 保证 preview/precheck/submit/Agent 使用同一冻结结果。
3. 补结果投影和 actual equality evidence；Cancel 因缺少 Agent 控制协议与进程树适配，保留为后续专项，不新增假操作。
4. 不改变 Agent 固定信封和无 Shell 边界。

当前进展：第 1 项已完成；现有 `commandgen.ConfigFingerprint` 已确认覆盖工具/元数据/能力、数据源修订、节点事实、平台和归一化字段，预检查与提交会重新生成并比较该指纹。`query-sql` 已在控制面、领域适配、向导和任务提交事务中接入统一门禁；真实授权器与 EX-V1 证据仍是运行时启用前的阻断项。

停止条件：出现秘密泄露、重复执行、状态乱序或现有真实路径回归。

### Phase 5: UX Consolidation

1. 修正多格式信息和任务详情。
2. 改进对象选择与高级分组。
3. 让安全阻断可诊断，并验证同一风险在一次不可变配置上只确认一次。
4. 不机械改步数，不做视觉重写。

### Phase 6: Documentation Rewrite

1. 重写三份 Export Canonical Docs。
2. 更新项目入口和邻近 README。
3. Merge 后删除源文件。
4. 移动历史资料到 archive 并修复链接。
5. 删除本临时计划或归档重大决策部分。

### Phase 7: Full Regression

1. 全自动化门禁。
2. Golden Path 和全部正式格式回归。
3. 真实执行仅在重新取得明确授权后运行。
4. 输出 v1 release evidence summary 和残余风险。

## 24. Regression Plan

### 24.1 自动化层次

| Layer | Required Coverage |
|---|---|
| Domain | 内容×对象×格式×输出×高级规则的表驱动正负例 |
| Command | 每个正式格式的确定 argv、脱敏命令、指纹和 secret slots |
| Control Plane | create/update/preview/precheck/submit、权限、CSRF、幂等、If-Match、无权 404 |
| Store | v5/v6/v7 replay、不可变快照、派生关系、重复/乱序/重启 |
| Agent | 固定信封、租约、直接 Java、日志队列、结果扫描、取消 |
| Frontend | 表单条件、错误码展示、格式配置、任务详情、重跑 |
| Contract | OpenAPI 枚举、required fields、安全禁止字段、operation 实施状态 |

### 24.2 Golden Path Fixture

建议建立 `testdata/export/v1/` 或现有测试习惯对应目录，包含：

- 合成 DataSource/Node facts。
- 每个格式一个最小 `ExportConfig`。
- expected capability/metadata/config version。
- expected normalized config、argv tokens、redacted command 和 fingerprint。
- expected task/result/log safe projections。
- v5/v6 历史 fixture。

fixture 禁止包含真实端点、用户名、密码、密钥、完整敏感命令、用户数据库对象或真实输出内容。

### 24.3 必测边界

- CSV：NULL、空串、逗号、双引号、非 ASCII、字段内换行、header、quote、escape、line separator。
- CUT/SQL/POS：格式专属配置和共享文本配置不串格式。
- Parquet/ORC/Avro：禁止压缩/不适用序列化/资源限制。
- Windows `/E:/...`、Windows 盘符语义、Linux `/...` 原样传递。
- ALL/SPECIFIED、TABLE/VIEW、DDL_ONLY/DATA_ONLY/DDL_AND_DATA。
- query/where/partition/flashback/snapshot 冲突。
- 未验证参数、未知格式、未知对象、未知 metadata 失败关闭。
- 预检查失败不创建任务、不启动工具。
- 重复 submit、重复 claim、事件乱序、日志重复、控制面/Agent 重启。
- secret scanning 覆盖响应、SQLite、日志、错误、fixture 和测试输出。
- 结果不足时不伪造文件、行数、校验和、进度或 ETA。

### 24.4 真实回归门禁

- 本计划不授权任何新的真实连接或工具启动。
- Phase 1 优先复用已有 Run History 和 evidence。
- 后续真实回归必须单独取得授权，使用非生产合成数据、新空目录和已登记节点。
- 每个格式至少两次独立一致结果，或明确记录为何只能作为实验支持。
- 对象存储、真实凭据、dump.ckpt 和麒麟目标继续按单独门禁执行。

## 25. Not Now

- 新增格式。
- 将 OBDUMPER 109 个参数全部页面化。
- 大量新的高级/专家参数。
- 定时任务、审批流和复杂调度。
- Dashboard、首页或无关页面增强。
- UI 装饰性重做。
- 通用数据库对象浏览器。
- 浏览器结果文件下载。
- 为 Import 提前设计统一 Config/Execution/Result 超级框架。
- Plugin architecture、微服务、消息队列、工作流引擎或 ORM 引入。
- 非当前正式平台的支持声明。
- 未经授权的对象存储、真实凭据、真实工具或用户数据库验证。

## 26. Risks

| Risk | Counterexample / Impact | Mitigation |
|---|---|---|
| 过度重构破坏历史命令 | v5/v6 指纹或 argv 改变，旧任务无法解释 | 字节级/令牌级兼容 fixture；旧资源不可变 |
| 把验证分层误当重复删除 | 只保留前端或只保留 commandgen，越权组合可能绕过 | 明确各层职责，保留独立服务端和发射层复验 |
| 安全流程过严拖慢开发 | 合成 fixture、脱敏预览或纯规则测试也依赖人工审批、真实 Agent 或真实凭据 | S0/S1 默认无人工确认；提供假 Agent/假工具安全开发路径，门禁尽量自动化 |
| 以开发效率为由绕过硬边界 | 调试开关可连接真实数据库、读取真实秘密或启动任意工具 | 开发便利仅适用于无真实副作用路径；S2-S4 仍由服务端和 Agent 强制执行，不信任前端开关 |
| 授权复用范围过宽 | 一次确认被复用于其他数据源、节点、输出路径或敏感参数 | 授权绑定不可变风险指纹、动作、对象和期限；字段变化、撤销或过期立即失效 |
| query-sql 越权 | 普通用户可提交敏感专家命令 | 平台开关、独立能力/任务授权、风险指纹确认和事务审计；授权器缺失时失败关闭 |
| 格式支持过度声明 | 代码存在但真实输出或历史未映射 | 正式/实验分级；现有证据逐格式绑定 |
| 文档清理丢失证据 | 删除一次真实失败或平台踩坑记录 | unique knowledge 核对，先 merge/archive 后删除 |
| Cancel 损坏输出 | 只杀父进程、子 Java 残留，状态却显示取消成功 | 进程树终止、超时、checkpoint、结果和终态联合验证 |
| 输出位置泄露 | 授权任务读者获得不应看到的节点绝对路径 | 创建者返回完整值，其他授权读者仅返回输出类型；无权对象统一 404 |
| 错误摘要误导 | 从日志关键字推断错误根因 | 优先使用固定事件/退出码错误码；历史无结构化证据时标明未知并回退已脱敏日志 |
| 对象选择器扩大数据库访问 | 为搜索而默认全库扫描或暴露无权对象数量 | 显式授权、分页、类型范围、失败关闭；必要时保持手工输入 |
| 平台外推 | Windows 通过被写成 Linux/生产可用 | 支持声明按平台和证据分级 |
| 文档先重写、代码后变化 | 新 Canonical 立刻过期 | 文档审计先做，Canonical rewrite 放在代码职责稳定后 |
| 本计划成为新债务 | 以后继续同时维护计划和 Canonical | Phase 6 强制删除或归档本文件 |

## 27. Next Actions

如果今天重新接手项目，最合理的五步是：

1. **冻结证据基线**：只读整理现有 Run History、任务快照、命令指纹、输出摘要和 evidence，把用户确认的“其他格式已跑通”映射到具体格式；不重新启动真实任务。
2. **冻结 Export v1 Scope/DoD 与安全分级**：确认正式格式、实验格式、Cancel 是否为发布必需、对象范围和本地输出边界；盘点现有门禁并冻结 S0-S4、无审批安全开发路径和有界授权规则，同时把 `query-sql` 标为阻断，直到高风险门禁完整。
3. **先提取领域规则，再动 UI**：从 `server.go` 提取 Export 归一化/能力选择/字段映射，保留 `ExportConfig`、commandgen、任务快照和 Agent 不变，并用 v5/v6/v7 回归锁住行为。
4. **补最小 v1 运行证据缺口**：退出码、耗时、错误摘要、授权输出位置和 planned/actual 一致性已完成合成闭环；Cancel 仍需单独授权和进程树协议设计，不能用页面操作替代。
5. **最后执行文档治理和全回归**：重写三份 Export Canonical Docs，Merge/Archive/Delete 旧文档，更新所有入口，再跑 Golden Path、正式格式、CSV 边界、Windows、失败与恢复回归。

---

本计划的核心约束是：保留已经真实工作的命令、执行和证据链；把当前 `ExportDraft + ExportConfig` 收敛为单一产品配置事实；把 commandgen、Agent 和任务状态机视为稳定资产；安全控制按风险相称且优先自动化，不让合成/离线开发承担生产级流程成本；同时只对职责过载、安全缺口和正式 v1 验收缺口做定向改动。
