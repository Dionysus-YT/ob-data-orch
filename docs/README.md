# OB Data Orch 文档中心

本目录用于维护 OB Loader/Dumper 调度平台的产品需求、产品设计基线和后续技术设计。文档保持轻量，以能够支持评审、参数映射和后续实现为准，不扩展为大型企业文档体系。

## 当前阶段

项目已完成总体需求讨论稿整理、十一个模块的产品/字段规则确认及产品设计语义收口。全局 UI/UX 只以 [P0 MASTER](../design-system/MASTER.md) 为准；页面进入精修 READY 后才可增加 `design-system/pages/` 下的 P1 规则。模块、字段、参数、测试和技术契约均为 P3 业务事实，不得由视觉文档覆盖。首条切片架构、技术路线和最小实现契约已经确认，DR-R01～DR-R18 进一步确认 G1 工程骨架与 G2 隔离组件准入。开发顺序、阶段、门禁、页面功能接入与真实验证状态统一以[开发任务地图](03-technical/development-task-map.md)为准。任何 G2 结果均不得描述为真实导出或产品可用。

2026-08-22 已完成 Export V1 六步向导与参数分层重新定版：普通新建采用“数据源 → 内容 → 对象 → 格式 → 输出 → 确认”，只展示 CSV/CUT/SQL；`--query-sql` 改为普通高级参数，`--retain-empty-files` 适用于三种数据格式的数据内容。当前下一工作项是运行时与参数元数据 v8 对齐，目标契约不等于现有页面和服务端已经完成迁移。

2026-09-02 已完成 Frontend Design Governance Reset：旧 Design System、单页 Visual Baseline、高保真资产、截图、视觉 QA 与临时设计 spec 已从工作树移除。P3 场景资料仍保留其业务语义，但不再携带或授权历史视觉决定。

## 文档导航

### 产品基线

- [总体 PRD](01-product/PRD.md)：V0.1 需求讨论整理稿，是当前需求事实来源。
- [产品范围](01-product/product-scope.md)：V1.0 范围、非目标、设计原则和官方能力边界。
- [关键决策](01-product/decisions.md)：从 PRD 和专项评审中形成并经明确确认的产品决策记录。

### 产品设计

- [信息架构](02-design/information-architecture.md)：导航、页面职责、模块关系和跳转关系。
- [用户流程](02-design/user-flow.md)：导出、普通导入、旁路导入三条创建流程及共用任务追踪/失败处理流程。
- [全局交互规范](02-design/interaction-spec.md)：向导、校验、命令预览、任务与日志的通用交互规则。
- [P0 Global UI/UX MASTER](../design-system/MASTER.md)：唯一全局设计语言、Product Shell、Archetype、Token 与响应式入口。
- [P1 Page Rules](../design-system/pages/)：仅在页面精修 READY 后增加；当前四个代表页文件没有 P1 覆盖规则。
- [02-design 使用边界](02-design/README.md)：本目录文档是 P3 业务事实，不承担视觉规范职责。
- [数据源 UI Regression Fixture](02-design/ui-regression-fixture-data-source.md)：16 行 DEV-only 合成输入、显式启用、无真实业务副作用和长期维护规则。
- [产品设计基线收口评审](02-design/product-design-baseline-closure.md)：汇总已确认基线、官方验证门禁、低保真范围、技术设计输入和开发准入条件。
- [首页模块评审稿](02-design/home-module.md)：授权范围内的运行摘要、四项指标、单一趋势图、节点健康、我的任务和最近异常，HM-R01～HM-R20 已确认。
- [首页字段与条件矩阵](02-design/home-field-rules.md)：75 个首页快照、指标、趋势、节点、任务、异常和页面状态字段，HM-FR01～HM-FR15 已确认。
- [首页场景矩阵](02-design/home-low-fidelity.md)：P3 的快照、过期、真实空与无权限语义；不定义当前页面视觉。
- [数据源管理评审稿](02-design/data-source-management.md)：首个可评审模块，限定在连接信息管理与基础连接测试。
- [导出模块评审稿](02-design/export-module.md)：Export v1 产品与交互 Canonical，覆盖六步向导、ODC Reference Matrix、Format Matrix、发布态、取消和任务结果。
- [通用导出技术契约](03-technical/export-general-contract.md)：Export v1 的领域模型、参数/命令、Agent 信封、结果证据和取消协议。
- [Export 历史研究归档](archive/export/)：109 参数映射、87 字段矩阵、支持矩阵和早期低保真，仅用于追溯，不作为当前事实源。
- [导出参数受控验证计划](02-design/export-controlled-validation.md)：冲突参数的隔离环境、安全边界、P0/P1/P2 用例、证据和回写标准。

### Export 文档治理

Export 长期现役入口控制为五份：产品范围、架构、Export 产品/交互、Export 技术契约、Export 受控验证；[开发任务地图](03-technical/development-task-map.md)另作为全项目开发顺序和门禁的唯一权威。以下 inventory 记录每个 Export 文档的处置，不以文件存在或历史路径继续制造第二事实源。

| 文件 | 用途 | 状态 | 重复/冲突 | 唯一知识与处置 | 目标入口 |
|---|---|---|---|---|---|
| `docs/01-product/product-scope.md` | 产品边界与 V1 范围 | CANONICAL | 与旧 PRD 有历史重叠 | 保留当前范围、发布态和安全边界 | 产品范围 |
| `docs/03-technical/architecture.md` | 系统分层与控制面/Agent 边界 | CANONICAL | 与首条切片历史架构有重叠 | 保留当前架构；旧实现说明只作追溯 | 技术架构 |
| `docs/02-design/export-module.md` | Export v1 产品、UX、ODC/格式矩阵、任务结果 | CANONICAL | 吸收旧字段、低保真和支持矩阵 | 保留当前产品事实和交互规则 | Export 产品/交互 |
| `docs/03-technical/export-general-contract.md` | 领域模型、参数、命令、Agent、结果和取消 | CANONICAL | 与旧参数映射有重叠 | 保留可执行契约；旧参数研究归档 | Export 技术契约 |
| `docs/02-design/export-controlled-validation.md` | Golden Path、格式证据和 EX-V1 门禁 | CANONICAL | 与旧受控验证记录有过程性重叠 | 保留证据判定和回写规则；现场记录仍留 evidence | Export 验证 |
| `docs/03-technical/development-task-map.md` | 全局开发顺序、Workstream 和阶段门禁 | CANONICAL（流程） | 与历史 DEV/WI 记录有顺序冲突 | 以当前表格为准，历史记录只追溯 | 开发顺序 |
| `docs/03-technical/ex-d0-implementation-inventory.md` | EX-D0 基线盘点与兼容冻结 | HISTORICAL BASELINE | 已被当前 Canonical 的状态段覆盖 | 保留源码追溯和历史冻结，不作为发布声明 | 任务地图/Canonical |
| `docs/02-design/export-field-rules.md`、`export-parameter-mapping.md`、`export-v1-support-matrix.md` | 早期字段、参数与支持矩阵 | ARCHIVE REDIRECT | 内容已合并，原路径不再保留视觉基线 | 不再单独维护；现役业务事实回到 Export Canonical | Export Canonical |
| `docs/archive/export/research/` | 参数与能力研究追溯 | ARCHIVE | 不参与当前前端设计上下文 | 仅为研究追溯，不证明当前能力或视觉规则 | 无 |
| `HANDOFF.md` | 2026-08-14 交接快照 | HISTORICAL SNAPSHOT | 运行态/PID/授权声明可能过期 | 仅追溯；当前事实必须回查 Canonical 与现场 | 无 |
- [普通导入模块评审稿](02-design/normal-import-module.md)：OBLOADER 六步向导、普通/旁路边界、完整参数承载方式和已确认的 NI-R01～NI-R18 产品规则。
- [普通导入参数映射基线](02-design/normal-import-parameter-mapping.md)：实际 `--help` 的 103 个长参数含义、分类、普通导入处置和官方来源。
- [普通导入字段与条件矩阵](02-design/normal-import-field-rules.md)：91 个页面字段/操作的显示、派生、默认、清值、风险和检查失效规则。
- [普通导入参数受控验证计划](02-design/normal-import-controlled-validation.md)：主流程、文件匹配、格式、列映射、高风险行为及检查点的 P0/P1/P2 用例。
- [普通导入场景矩阵](02-design/normal-import-low-fidelity.md)：P3 的文件、格式、映射、风险、命令和检查点继续语义；不定义当前页面视觉。
- [旁路导入模块评审稿](02-design/direct-load-module.md)：与导出、普通导入同构的六步向导、参数分层和任务交互，以及已确认的 DL-R01～DL-R20 产品规则。
- [旁路导入参数映射基线](02-design/direct-load-parameter-mapping.md)：同一份 103 个 OBLOADER 长参数的旁路适用状态，以及 3 个 Direct Load 节点配置键。
- [旁路导入字段与条件矩阵](02-design/direct-load-field-rules.md)：92 个页面字段/操作的显示、派生、阻断、快照、风险和失败规则，DL-FR01～DL-FR12 已确认。
- [旁路导入参数受控验证计划](02-design/direct-load-controlled-validation.md)：连接场景、版本、格式、结构、模式、并发、节点配置及失败语义的 P0/P1/P2 用例。
- [旁路导入场景矩阵](02-design/direct-load-low-fidelity.md)：P3 的 SQL/RPC、单表、参数和失败恢复语义；不定义当前页面视觉。
- [任务中心与任务详情评审稿](02-design/task-center-module.md)：统一三类任务的列表、状态、详情、快照、命令、日志和失败操作，TC-R01～TC-R20 已确认。
- [任务中心字段与条件矩阵](02-design/task-center-field-rules.md)：85 个列表/详情字段与操作的条件、证据、脱敏和追溯规则，TC-FR01～TC-FR14 已确认。
- [任务中心与详情场景矩阵](02-design/task-center-low-fidelity.md)：P3 的状态、快照、命令、结果和失败恢复语义；不定义当前页面视觉。
- [执行节点管理评审稿](02-design/execution-node-module.md)：节点注册、四维状态、工具环境、基础资源、任务关系和调度边界，EN-R01～EN-R19 已确认。
- [执行节点字段与条件矩阵](02-design/execution-node-field-rules.md)：72 个列表、注册、环境、资源、任务与状态化操作字段，EN-FR01～EN-FR14 已确认。
- [执行节点场景矩阵](02-design/execution-node-low-fidelity.md)：P3 的节点资格、环境、维护和失联语义；不定义当前页面视觉。
- [日志中心模块评审稿](02-design/log-center-module.md)：统一日志预设、跨对象查询、同来源上下文、采集完整性、脱敏下载和权限边界，LC-R01～LC-R20 已确认。
- [日志中心字段与条件矩阵](02-design/log-center-field-rules.md)：75 个查询、结果、上下文、持续加载、下载与异常字段，LC-FR01～LC-FR15 已确认。
- [日志中心场景矩阵](02-design/log-center-low-fidelity.md)：P3 的查询、来源、上下文、缺口和脱敏下载语义；不定义当前页面视觉。
- [模板中心模块评审稿](02-design/template-center-module.md)：三类任务模板的保存边界、数据源策略、兼容状态、任务草稿创建和版本变化处理，TP-R01～TP-R20 已确认。
- [模板中心字段与条件矩阵](02-design/template-center-field-rules.md)：83 个列表、元信息、配置、兼容、操作与异常字段，TP-FR01～TP-FR15 已确认。
- [模板中心场景矩阵](02-design/template-center-low-fidelity.md)：P3 的模板、兼容性和草稿承接语义；不定义当前页面视觉。
- [系统设置模块评审稿](02-design/system-settings-module.md)：平台基础、调度、安全和工具兼容四类系统级设置及生效边界，SS-R01～SS-R20 已确认。
- [系统设置字段与条件矩阵](02-design/system-settings-field-rules.md)：80 个设置、影响、保存、权限与异常字段，SS-FR01～SS-FR16 已确认。
- [系统设置场景矩阵](02-design/system-settings-low-fidelity.md)：P3 的生效值、影响、保存和冲突语义；不定义当前页面视觉。
- [权限与安全基线专项评审稿](02-design/access-control-security-module.md)：统一五类固定角色、功能与对象范围、生产任务、专家配置、敏感命令、日志与审计边界，AC-R01～AC-R20 已确认。
- [权限配置字段与条件矩阵](02-design/access-control-security-field-rules.md)：80 个用户、角色、对象范围、高风险能力、保存与审计字段，AC-FR01～AC-FR16 已确认。
- [权限与安全场景矩阵](02-design/access-control-security-low-fidelity.md)：P3 的对象范围、高风险确认和审计语义；不定义当前页面视觉。

### 技术设计

- [技术设计入口](03-technical/README.md)：工程启动阶段、文档范围和维护门禁。
- [技术架构首版](03-technical/architecture.md)：已确认的模块化单体控制面、独立执行 Agent 和核心工程不变量基线。
- [首条纵向切片](03-technical/first-vertical-slice.md)：数据源到基础 CSV 导出的范围、VS-P0-01～VS-P0-12 和验收条件。
- [技术路线与部署选型基线](03-technical/technology-stack.md)：已确认的 Go 控制面/Agent、Vue 3、SQLite、Windows AMD64，以及两个麒麟 ARM64 和一个麒麟 C86 目标与轻量通信方案。
- [参数元数据与确定性命令生成契约](03-technical/parameter-command-contract.md)：首条切片的结构化参数、稳定命令、跨平台路径、指纹和脱敏边界；PC-R01～PC-R15 已确认。
- [Agent 协议与任务状态最小契约](03-technical/agent-task-state-contract.md)：首条切片的机器认证、租约、幂等事件、进程证据、状态投影和失联恢复；AS-R01～AS-R16 已确认。
- [凭据、权限与安全最小契约](03-technical/credential-access-security-contract.md)：数据源密码、Agent 机器凭据、跨平台根密钥、短时解析、权限、审计和工具进程暴露边界；CS-R01～CS-R18 已确认。
- [OBDUMPER 跨平台隔离启动入口契约](03-technical/tool-launch-isolation-contract.md)：直接 Java 受控入口、版本化平台配置、任务私有目录、最小环境、进程恢复和清理规则；TL-R01～TL-R18 已确认。
- [日志采集、双层脱敏与执行证据最小契约](03-technical/log-collection-evidence-contract.md)：日志来源、记录重组、双层脱敏、分段文件、SQLite 索引、游标、缺口与下载规则；LG-R01～LG-R20 已确认。
- [API 与 SQLite 数据模型最小契约](03-technical/api-sqlite-data-contract.md)：首条切片浏览器/Agent API、提交前 Agent 预检查、20 张窄表、事务、迁移和备份规则；AD-R01～AD-R20 已确认。
- [首条切片开发准入收口](03-technical/development-readiness-closure.md)：G0～G4 分层准入、P0 重新归类、Windows/麒麟门禁、外部输入和 DEV-01～DEV-09 顺序；DR-R01～DR-R18 已确认。
- [G3 Windows 真实集成准入准备](03-technical/g3-windows-entry-readiness.md)：WI-01～WI-12 的责任、脱敏证据、执行顺序与 G3 启动判定；当前只准备门禁材料。
- [开发准入清单](03-technical/development-entry-checklist.md)：版本控制、验证、架构、安全、API 和测试门禁状态。
- [本地工具包证据](03-technical/evidence/tool-package-baseline.md)：本地 4.3.5 包摘要、工具版本、帮助参数集合和 Windows 启动环境风险。
- [安全文件兼容性验证](03-technical/evidence/secure-gen-compatibility-spike-2026-07-21.md)：Windows 上的官方兼容安全材料、无密码 argv、错误私钥、ACL 和三目标构建证据，以及仍待执行的 Linux/清理门禁。
- [直接 Java 隔离启动验证](03-technical/evidence/direct-java-launch-spike-2026-07-21.md)：Windows 候选入口的结构化路径、最小环境、直接进程身份和真实退出码证据。
- [日志流重组与双层脱敏验证](03-technical/evidence/log-stream-redaction-spike-2026-07-21.md)：跨读取块秘密、UTF-8 重组、双层脱敏、批次幂等和超长记录缺口的本地合成证据。
- [API/数据模型 SQLite 约束验证](03-technical/evidence/api-data-model-sqlite-spike-2026-07-21.md)：乐观锁、不可变任务、同任务唯一领取、复合租约外键、幂等和日志批次冲突的 Windows 合成证据。
- [SQLite 跨平台最小技术验证](03-technical/evidence/sqlite-cross-platform-spike-2026-07-21.md)：Windows AMD64、Linux AMD64/ARM64 纯 Go 构建和 Windows 事务、WAL、完整性及备份证据。
- [首条纵向切片 P0 执行记录](03-technical/evidence/first-vertical-slice-p0-2026-07-21.md)：VS-P0-02～VS-P0-12 的连接、查询、导出尝试、环境阻断和安全结论。

## 文档状态

| 文档 | 状态 | 说明 |
|---|---|---|
| 总体 PRD | 待产品评审 | 保留原始需求内容，目标工具版本为 V4.3.5 |
| 产品范围 | V1.0 范围基线已收口 | 从 PRD 和现行已确认决策提炼，官方行为仍以受控验证结果为准 |
| 关键决策 | 已确认并持续维护 | 收录 PRD 与模块专项评审中经明确确认的结论 |
| 全局产品设计基础文档 | 产品设计语义基线已收口 | 信息架构、用户流程、全局交互及已规划静态低保真范围均已形成基线 |
| P0 Global UI/UX MASTER | FROZEN | `design-system/MASTER.md` 是唯一全局 UI/UX 规范；1920×1080 为主基准，1440×1024 与 1280×720 为适配验证视口 |
| P1 Page Rules | NOT YET READY | 四个代表页只建立入口，不存在页面级视觉覆盖规则 |
| P2 Representative Visual Baseline | FROZEN | 用于产品级视觉校准；不能替代 P3 业务验证或 P0/P1 规则 |
| 数据源 UI Regression Fixture | 长期保留 | 16 行 DEV-only、显式参数启用的合成输入；不写业务数据，不是自动 Screenshot Diff |
| 产品设计基线收口评审 | 历史收口结论已确认 | CL-01～CL-08 的历史统计保留；后续增量决策以 `decisions.md` 和对应模块 Canonical 为准 |
| 首页模块评审稿 | 产品、字段与低保真规则已确认 | HM-R01～HM-R20、HM-FR01～HM-FR15、HM-LF-R01～HM-LF-R18 已确认；刷新、聚合和响应式实现仍待技术设计 |
| 首页字段与条件矩阵 | 字段规则已确认 | 已覆盖 HM-F001～HM-F075；HM-FR01～HM-FR15 已确认 |
| 首页低保真基线 | 关键页面状态已确认 | 正常快照、部分失败与过期、真实空状态和无权限已覆盖；HM-LF-R01～HM-LF-R18 已确认 |
| 数据源管理评审稿 | 产品决策已确认 | DS-R01～DS-R11 已确认；当前切片固定私有 ODP，其租户/集群精确字段条件仍待官方参数映射 |
| 导出模块评审稿 | Export v1 Canonical 已重新定版 | 六步顺序、CSV/CUT/SQL 普通入口、对象下拉、单一高级设置、query-sql 普通授权、取消和任务结果以本文档与开发任务地图为准；运行时迁移仍待 CONS-05 |
| 通用导出技术契约 | Export v1 技术 Canonical 已收口 | 领域模型、命令/Agent 信封、结果证据和取消协议已同步源码与 OpenAPI；真实工具/远端证据仍按 EX-V1 |
| Export 历史研究归档 | 仅追溯 | 109 参数、87 字段、早期支持矩阵与低保真保留历史来源，不再作为当前事实源 |
| 导出参数受控验证计划 | 合成基线已完成，真实证据待授权 | 已回写现有 CSV/POS/DDL 工具证据并完成 Phase 2 合成回归；取消已有协议级负例，CUT/SQL/结构化格式及 EX-V1 产品链路证据仍需授权环境与证据映射 |
| 普通导入模块评审稿 | 产品决策已确认 | NI-R01～NI-R18 已确认；help-only 与行为边界仍待受控验证 |
| 普通导入参数映射基线 | 参数名称已核查 | 实际 help 的 103 个长参数已逐项覆盖；旁路、废弃、导出侧与 help-only 项已分类 |
| 普通导入字段与条件矩阵 | 字段规则已确认 | 已覆盖 NI-F001～NI-F091；NI-FR01～NI-FR10 已确认，待验证项状态不变 |
| 普通导入参数受控验证计划 | 待执行 | 已形成29个P0、12个P1和8个P2用例；需要非生产测试环境与授权 |
| 普通导入低保真基线 | 关键流程已确认 | 文件来源、内容与格式、对象与映射、执行风险、预检查命令和检查点继续已覆盖；NI-LF-R01～NI-LF-R08 已确认 |
| 旁路导入模块评审稿 | 产品决策已确认 | DL-R01～DL-R20 已确认；官方适用性、参数行为和取消语义仍待受控验证 |
| 旁路导入参数映射基线 | 参数名称已核查 | 103 个 help 参数零遗漏；24 项旁路已确认、44 项待旁路适用性确认，并记录 3 个节点配置冲突项 |
| 旁路导入字段与条件矩阵 | 字段规则已确认 | 已覆盖 DL-F001～DL-F092；DL-FR01～DL-FR12 已确认，待验证项状态不变 |
| 旁路导入参数受控验证计划 | 待执行 | 已形成34个P0、12个P1和10个P2用例；需要隔离 OBServer/ODP 和明确授权 |
| 旁路导入低保真基线 | 关键流程已确认 | 六步向导、SQL/RPC、版本、单表分片、格式结构、命令风险和失败从头执行已覆盖；DL-LF-R01～DL-LF-R12 已确认 |
| 任务中心与任务详情评审稿 | 产品与字段规则已确认 | TC-R01～TC-R20、TC-FR01～TC-FR14 已确认；精确工具行为边界仍待验证 |
| 任务中心字段与条件矩阵 | 字段规则已确认 | 已覆盖 TC-F001～TC-F085；TC-FR01～TC-FR14 已确认，待验证项状态不变 |
| 任务中心与任务详情全状态低保真基线 | 关键流程已确认 | 任务列表、八种状态、状态核对、快照、命令证据、取消、成功和失败处理已覆盖；TC-LF-R01～TC-LF-R14 已确认 |
| 执行节点管理评审稿 | 产品、字段与低保真规则已确认 | EN-R01～EN-R19、EN-FR01～EN-FR14、EN-LF-R01～EN-LF-R16 已确认；Agent 安全和阈值/调度技术边界仍待设计 |
| 执行节点字段与条件矩阵 | 字段规则已确认 | 已覆盖 EN-F001～EN-F072；EN-FR01～EN-FR14 已确认，技术待设计项状态不变 |
| 执行节点管理低保真基线 | 关键流程已确认 | 节点列表、注册关联、四维状态、只读环境检查、维护确认和失联任务关系已覆盖；EN-LF-R01～EN-LF-R16 已确认 |
| 日志中心模块评审稿 | 产品、字段、低保真和日志技术契约已确认 | LC-R01～LC-R20、LC-FR01～LC-FR15、LC-LF-R01～LC-LF-R18、LG-R01～LG-R20 已确认；真实并发、崩溃恢复和三个麒麟目标仍待验证 |
| 日志中心字段与条件矩阵 | 字段规则已确认 | 已覆盖 LC-F001～LC-F075；LC-FR01～LC-FR15 已确认，技术待设计项状态不变 |
| 日志中心低保真基线 | 关键流程已确认 | 首次进入、任务聚合、审计查询、同来源上下文、持续加载与缺口、脱敏下载均已覆盖；LC-LF-R01～LC-LF-R18 已确认 |
| 模板中心模块评审稿 | 产品、字段与低保真规则已确认 | TP-R01～TP-R20、TP-FR01～TP-FR15、TP-LF-R01～TP-LF-R16 已确认；技术实现边界仍待设计 |
| 模板中心字段与条件矩阵 | 字段规则已确认 | 已覆盖 TP-F001～TP-F083；TP-FR01～TP-FR15 已确认 |
| 模板中心复用链路低保真基线 | 关键流程已确认 | 模板列表、成功任务提取、模板详情/编辑、兼容核查和可编辑任务草稿承接已覆盖；TP-LF-R01～TP-LF-R16 已确认 |
| 系统设置模块评审稿 | 产品、字段与低保真规则已确认 | SS-R01～SS-R20、SS-FR01～SS-FR16、SS-LF-R01～SS-LF-R18 已确认；技术实现边界仍待设计 |
| 系统设置字段与条件矩阵 | 字段规则已确认 | 已覆盖 SS-F001～SS-F080；SS-FR01～SS-FR16 已确认 |
| 系统设置低保真基线 | 关键流程已确认 | 平台基础、保留期确认、调度、安全、工具兼容和配置冲突均已覆盖；SS-LF-R01～SS-LF-R18 已确认 |
| 权限与安全基线专项评审稿 | 产品、字段与低保真规则已确认 | AC-R01～AC-R20、AC-FR01～AC-FR16、AC-LF-R01～AC-LF-R18 已确认；技术安全实现仍待设计 |
| 权限配置字段与条件矩阵 | 字段规则已确认 | 已覆盖 AC-F001～AC-F080；AC-FR01～AC-FR16 已确认 |
| 权限与安全高风险操作低保真基线 | 关键流程已确认 | 用户列表、角色、对象范围、原子保存、生产提交和敏感命令查看已覆盖；AC-LF-R01～AC-LF-R18 已确认 |
| 技术设计与开发准入 | 首条切片最小实现契约和分层开发准入已确认 | TD、TS、PC、AS、CS、TL、LG、AD、DR 已确认；G1 已准入、G2 按组件准入、G3/G4 阻断；SQLite 核心约束 Windows 合成验证通过，私有 ODP G3 环境、三个麒麟目标、认证接入、正式迁移及既有 P0 仍待完成 |
| [开发任务地图](03-technical/development-task-map.md) | 后续开发顺序与每阶段验收 | DEV-04～DEV-09 的目标、非目标、验收、门禁与外部前置条件 |

所有未通过 OB Loader/Dumper V4.3.5 官方命令行文档与实际 `--help` 核验的参数，统一标记为“待官方参数映射确认”。

## 推荐阅读顺序

1. [产品范围](01-product/product-scope.md)
2. [总体 PRD](01-product/PRD.md)
3. [关键决策](01-product/decisions.md)
4. [信息架构](02-design/information-architecture.md)
5. [用户流程](02-design/user-flow.md)
6. [全局交互规范](02-design/interaction-spec.md)
   - 开发或评审新前端页面时，先阅读 [P0 MASTER](../design-system/MASTER.md)，再读取存在的 P1 Page Rule 与适用的 P3 Canonical/字段规则/API/安全契约；现有实现、场景矩阵和 Fixture 只能作为事实或测试输入，不能覆盖 P0/P1。
7. [产品设计基线收口评审](02-design/product-design-baseline-closure.md)
9. [首页模块评审稿](02-design/home-module.md)
10. [首页字段与条件矩阵](02-design/home-field-rules.md)
11. [首页低保真基线](02-design/home-low-fidelity.md)
12. [数据源管理评审稿](02-design/data-source-management.md)
13. [导出模块评审稿](02-design/export-module.md)
14. [通用导出技术契约](03-technical/export-general-contract.md)
15. [Export 历史研究归档](archive/export/)
16. [导出参数受控验证计划](02-design/export-controlled-validation.md)
17. [普通导入模块评审稿](02-design/normal-import-module.md)
18. [普通导入参数映射基线](02-design/normal-import-parameter-mapping.md)
19. [普通导入字段与条件矩阵](02-design/normal-import-field-rules.md)
20. [普通导入参数受控验证计划](02-design/normal-import-controlled-validation.md)
21. [普通导入低保真基线](02-design/normal-import-low-fidelity.md)
22. [旁路导入模块评审稿](02-design/direct-load-module.md)
23. [旁路导入参数映射基线](02-design/direct-load-parameter-mapping.md)
24. [旁路导入字段与条件矩阵](02-design/direct-load-field-rules.md)
25. [旁路导入参数受控验证计划](02-design/direct-load-controlled-validation.md)
26. [旁路导入低保真基线](02-design/direct-load-low-fidelity.md)
27. [任务中心与任务详情评审稿](02-design/task-center-module.md)
28. [任务中心字段与条件矩阵](02-design/task-center-field-rules.md)
29. [任务中心与任务详情全状态低保真基线](02-design/task-center-low-fidelity.md)
30. [执行节点管理评审稿](02-design/execution-node-module.md)
31. [执行节点字段与条件矩阵](02-design/execution-node-field-rules.md)
32. [执行节点管理低保真基线](02-design/execution-node-low-fidelity.md)
33. [日志中心模块评审稿](02-design/log-center-module.md)
34. [日志中心字段与条件矩阵](02-design/log-center-field-rules.md)
35. [日志中心低保真基线](02-design/log-center-low-fidelity.md)
36. [模板中心模块评审稿](02-design/template-center-module.md)
37. [模板中心字段与条件矩阵](02-design/template-center-field-rules.md)
38. [模板中心复用链路低保真基线](02-design/template-center-low-fidelity.md)
39. [系统设置模块评审稿](02-design/system-settings-module.md)
40. [系统设置字段与条件矩阵](02-design/system-settings-field-rules.md)
41. [系统设置低保真基线](02-design/system-settings-low-fidelity.md)
42. [权限与安全基线专项评审稿](02-design/access-control-security-module.md)
43. [权限配置字段与条件矩阵](02-design/access-control-security-field-rules.md)
44. [权限与安全高风险操作低保真基线](02-design/access-control-security-low-fidelity.md)
45. [技术设计入口](03-technical/README.md)
46. [技术架构首版](03-technical/architecture.md)
47. [首条纵向切片](03-technical/first-vertical-slice.md)
48. [技术路线与部署选型基线](03-technical/technology-stack.md)
49. [开发准入清单](03-technical/development-entry-checklist.md)
50. [本地工具包证据](03-technical/evidence/tool-package-baseline.md)
51. [SQLite 跨平台最小技术验证](03-technical/evidence/sqlite-cross-platform-spike-2026-07-21.md)
52. [日志采集、双层脱敏与执行证据最小契约](03-technical/log-collection-evidence-contract.md)
53. [日志流重组与双层脱敏验证](03-technical/evidence/log-stream-redaction-spike-2026-07-21.md)
54. [API 与 SQLite 数据模型最小契约](03-technical/api-sqlite-data-contract.md)
55. [API/数据模型 SQLite 约束验证](03-technical/evidence/api-data-model-sqlite-spike-2026-07-21.md)
56. [首条切片开发准入收口](03-technical/development-readiness-closure.md)

## 维护规则

- PRD 是需求事实来源；辅助文档与 PRD 冲突时，先回到 PRD 和评审记录核实。
- 已确认产品结论通过决策记录管理；未确认内容不得写成确定能力。
- 官方工具能力以 V4.3.5 官方资料及实际工具 `--help` 为准。
- 模块评审通过后应更新文档状态和版本，避免页面、接口与数据模型反复联动修改。
- 新页面实现前必须先读取前端设计基线及对应模块 Canonical，并区分公共视觉、业务模式和模块专属规则；不得把单页字段强行抽象为公共系统。
- 对根 Token、Product Shell、公共组件或已确认模式的修改必须说明不足、影响页面、迁移范围和回归视口；普通业务变更不得静默改写全局视觉规则。
- 当前 Design Baseline 不触发一次性全站迁移；存量页面只能在各自真实业务改造时逐步接入，并单独记录代码与浏览器验收状态。
- 技术设计从首条纵向切片的真实门禁出发，不预先创建全部模块的空接口和数据表文档。
