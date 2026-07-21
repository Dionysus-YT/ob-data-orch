# OB Data Orch 文档中心

本目录用于维护 OB Loader/Dumper 调度平台的产品需求、产品设计基线和后续技术设计。文档保持轻量，以能够支持评审、参数映射和后续实现为准，不扩展为大型企业文档体系。

## 当前阶段

项目已完成总体需求讨论稿整理、十一个模块的产品/字段规则确认、产品设计语义基线收口，以及数据源、导出公共链路、普通导入、旁路导入、任务中心全状态、模板复用、高风险权限操作、执行节点、日志中心、系统设置和首页静态低保真评审。首条切片架构、技术路线和最小实现契约已经确认，DR-R01～DR-R18 进一步确认 G1 工程骨架与 G2 隔离组件准入。DEV-01 和 DEV-02 已完成本地合成验证；DEV-03 的参数生成、SQLite 核心仓储、凭据加密与安全目录组件均已通过纯合成契约测试，当前参数元数据为兼容修订 `v2`。下一项是 Agent 协议与状态机；G3 Windows 真实集成、三个麒麟目标和生产发布继续阻断。

## 文档导航

### 产品基线

- [总体 PRD](01-product/PRD.md)：V0.1 需求讨论整理稿，是当前需求事实来源。
- [产品范围](01-product/product-scope.md)：V1.0 范围、非目标、设计原则和官方能力边界。
- [关键决策](01-product/decisions.md)：从 PRD 和专项评审中形成并经明确确认的产品决策记录。

### 产品设计

- [信息架构](02-design/information-architecture.md)：导航、页面职责、模块关系和跳转关系。
- [用户流程](02-design/user-flow.md)：导出、普通导入、旁路导入三条创建流程及共用任务追踪/失败处理流程。
- [全局交互规范](02-design/interaction-spec.md)：向导、校验、命令预览、任务与日志的通用交互规则。
- [产品设计基线收口评审](02-design/product-design-baseline-closure.md)：汇总已确认基线、官方验证门禁、低保真范围、技术设计输入和开发准入条件。
- [核心流程低保真基线](02-design/core-flow-low-fidelity.md)：已确认的数据源、导出、命令确认、任务详情与日志首条核心链路，以及 LF-R01～LF-R07。
- [首页模块评审稿](02-design/home-module.md)：授权范围内的运行摘要、四项指标、单一趋势图、节点健康、我的任务和最近异常，HM-R01～HM-R20 已确认。
- [首页字段与条件矩阵](02-design/home-field-rules.md)：75 个首页快照、指标、趋势、节点、任务、异常和页面状态字段，HM-FR01～HM-FR15 已确认。
- [首页低保真基线](02-design/home-low-fidelity.md)：已确认的正常快照、部分失败与过期、真实空状态和无权限页面，以及 HM-LF-R01～HM-LF-R18。
- [数据源管理评审稿](02-design/data-source-management.md)：首个可评审模块，限定在连接信息管理与基础连接测试。
- [导出模块评审稿](02-design/export-module.md)：OBDUMPER 六步向导、参数约束和已确认的 EX-R01～EX-R17 产品规则。
- [导出参数映射基线](02-design/export-parameter-mapping.md)：基于 OceanBase 官网与实际 `--help` 的 109 个长参数含义、分类、产品处置和来源。
- [导出字段与条件矩阵](02-design/export-field-rules.md)：87 个页面字段的显示、必填、默认继承、清值、命令和预检查失效规则。
- [导出参数受控验证计划](02-design/export-controlled-validation.md)：冲突参数的隔离环境、安全边界、P0/P1/P2 用例、证据和回写标准。
- [普通导入模块评审稿](02-design/normal-import-module.md)：OBLOADER 六步向导、普通/旁路边界、完整参数承载方式和已确认的 NI-R01～NI-R18 产品规则。
- [普通导入参数映射基线](02-design/normal-import-parameter-mapping.md)：实际 `--help` 的 103 个长参数含义、分类、普通导入处置和官方来源。
- [普通导入字段与条件矩阵](02-design/normal-import-field-rules.md)：91 个页面字段/操作的显示、派生、默认、清值、风险和检查失效规则。
- [普通导入参数受控验证计划](02-design/normal-import-controlled-validation.md)：主流程、文件匹配、格式、列映射、高风险行为及检查点的 P0/P1/P2 用例。
- [普通导入低保真基线](02-design/normal-import-low-fidelity.md)：已确认的文件、格式、映射、风险、命令和检查点继续页面，以及 NI-LF-R01～NI-LF-R08。
- [旁路导入模块评审稿](02-design/direct-load-module.md)：与导出、普通导入同构的六步向导、参数分层和任务交互，以及已确认的 DL-R01～DL-R20 产品规则。
- [旁路导入参数映射基线](02-design/direct-load-parameter-mapping.md)：同一份 103 个 OBLOADER 长参数的旁路适用状态，以及 3 个 Direct Load 节点配置键。
- [旁路导入字段与条件矩阵](02-design/direct-load-field-rules.md)：92 个页面字段/操作的显示、派生、阻断、快照、风险和失败规则，DL-FR01～DL-FR12 已确认。
- [旁路导入参数受控验证计划](02-design/direct-load-controlled-validation.md)：连接场景、版本、格式、结构、模式、并发、节点配置及失败语义的 P0/P1/P2 用例。
- [旁路导入低保真基线](02-design/direct-load-low-fidelity.md)：已确认的适用条件、SQL/RPC、单表文件、格式结构、Direct Load 参数、命令风险和失败从头执行页面，以及 DL-LF-R01～DL-LF-R12。
- [任务中心与任务详情评审稿](02-design/task-center-module.md)：统一三类任务的列表、状态、详情、快照、命令、日志和失败操作，TC-R01～TC-R20 已确认。
- [任务中心字段与条件矩阵](02-design/task-center-field-rules.md)：85 个列表/详情字段与操作的条件、证据、脱敏和追溯规则，TC-FR01～TC-FR14 已确认。
- [任务中心与任务详情全状态低保真基线](02-design/task-center-low-fidelity.md)：已确认的任务列表、八种状态、状态核对、配置快照、计划/实际命令、取消、成功和两类失败处理页面，以及 TC-LF-R01～TC-LF-R14。
- [执行节点管理评审稿](02-design/execution-node-module.md)：节点注册、四维状态、工具环境、基础资源、任务关系和调度边界，EN-R01～EN-R19 已确认。
- [执行节点字段与条件矩阵](02-design/execution-node-field-rules.md)：72 个列表、注册、环境、资源、任务与状态化操作字段，EN-FR01～EN-FR14 已确认。
- [执行节点管理低保真基线](02-design/execution-node-low-fidelity.md)：已确认的节点列表、注册关联、四维状态、只读环境检查、维护确认和失联任务页面，以及 EN-LF-R01～EN-LF-R16。
- [日志中心模块评审稿](02-design/log-center-module.md)：统一日志预设、跨对象查询、同来源上下文、采集完整性、脱敏下载和权限边界，LC-R01～LC-R20 已确认。
- [日志中心字段与条件矩阵](02-design/log-center-field-rules.md)：75 个查询、结果、上下文、持续加载、下载与异常字段，LC-FR01～LC-FR15 已确认。
- [日志中心低保真基线](02-design/log-center-low-fidelity.md)：已确认的首次进入、任务聚合、审计查询、同来源上下文、持续加载与缺口、脱敏下载页面，以及 LC-LF-R01～LC-LF-R18。
- [模板中心模块评审稿](02-design/template-center-module.md)：三类任务模板的保存边界、数据源策略、兼容状态、任务草稿创建和版本变化处理，TP-R01～TP-R20 已确认。
- [模板中心字段与条件矩阵](02-design/template-center-field-rules.md)：83 个列表、元信息、配置、兼容、操作与异常字段，TP-FR01～TP-FR15 已确认。
- [模板中心复用链路低保真基线](02-design/template-center-low-fidelity.md)：已确认的模板列表、成功任务提取、模板编辑、兼容差异和可编辑任务草稿承接页面，以及 TP-LF-R01～TP-LF-R16。
- [系统设置模块评审稿](02-design/system-settings-module.md)：平台基础、调度、安全和工具兼容四类系统级设置及生效边界，SS-R01～SS-R20 已确认。
- [系统设置字段与条件矩阵](02-design/system-settings-field-rules.md)：80 个设置、影响、保存、权限与异常字段，SS-FR01～SS-FR16 已确认。
- [系统设置低保真基线](02-design/system-settings-low-fidelity.md)：已确认的平台基础、保留期确认、调度、安全、工具兼容和配置冲突页面，以及 SS-LF-R01～SS-LF-R18。
- [权限与安全基线专项评审稿](02-design/access-control-security-module.md)：统一五类固定角色、功能与对象范围、生产任务、专家配置、敏感命令、日志与审计边界，AC-R01～AC-R20 已确认。
- [权限配置字段与条件矩阵](02-design/access-control-security-field-rules.md)：80 个用户、角色、对象范围、高风险能力、保存与审计字段，AC-FR01～AC-FR16 已确认。
- [权限与安全高风险操作低保真基线](02-design/access-control-security-low-fidelity.md)：已确认的用户权限列表、固定角色、对象范围、权限保存、生产任务提交和敏感命令受控查看页面，以及 AC-LF-R01～AC-LF-R18。

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
| 产品范围 | V1.0 范围基线已收口 | 从 PRD 和 44 项已确认决策提炼，官方行为仍以受控验证结果为准 |
| 关键决策 | 已确认并持续维护 | 收录 PRD 与模块专项评审中经明确确认的结论 |
| 全局产品设计基础文档 | 产品设计语义基线已收口 | 信息架构、用户流程、全局交互及已规划静态低保真范围均已形成基线 |
| 产品设计基线收口评审 | 收口结论已确认 | CL-01～CL-08 已确认；累计 44 项决策、205 条产品规则、820 个字段/操作、137 条条件规则和 83 个 P0 用例 |
| 核心流程低保真基线 | 首条核心链路已确认 | 数据源、导出步骤 5～6、任务详情与日志已形成静态低保真；LF-R01～LF-R07 已确认 |
| 首页模块评审稿 | 产品、字段与低保真规则已确认 | HM-R01～HM-R20、HM-FR01～HM-FR15、HM-LF-R01～HM-LF-R18 已确认；刷新、聚合和响应式实现仍待技术设计 |
| 首页字段与条件矩阵 | 字段规则已确认 | 已覆盖 HM-F001～HM-F075；HM-FR01～HM-FR15 已确认 |
| 首页低保真基线 | 关键页面状态已确认 | 正常快照、部分失败与过期、真实空状态和无权限已覆盖；HM-LF-R01～HM-LF-R18 已确认 |
| 数据源管理评审稿 | 产品决策已确认 | DS-R01～DS-R11 已确认；OBServer / ODP 精确字段条件仍待官方参数映射 |
| 导出模块评审稿 | 产品决策已确认 | EX-R01～EX-R17 已确认；POS、block-size 等官方冲突项仍待实测 |
| 导出参数映射与约束矩阵 | 进行中 | 109 个参数的名称、含义、分类、产品处置和来源已核查；冲突项及页面级条件矩阵待实测 |
| 导出字段与条件矩阵 | 字段规则已确认 | 已覆盖 EX-F001～EX-F087；EX-FR01～EX-FR10 已确认，官方冲突项待实测 |
| 导出参数受控验证计划 | 待执行 | 已形成20个P0、10个P1和5个P2用例；需要非生产测试环境与授权 |
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
| 技术设计与开发准入 | 首条切片最小实现契约和分层开发准入已确认 | TD、TS、PC、AS、CS、TL、LG、AD、DR 已确认；G1 已准入、G2 按组件准入、G3/G4 阻断；SQLite 核心约束 Windows 合成验证通过，直连环境、三个麒麟目标、认证接入、正式迁移及既有 P0 仍待完成 |

所有未通过 OB Loader/Dumper V4.3.5 官方命令行文档与实际 `--help` 核验的参数，统一标记为“待官方参数映射确认”。

## 推荐阅读顺序

1. [产品范围](01-product/product-scope.md)
2. [总体 PRD](01-product/PRD.md)
3. [关键决策](01-product/decisions.md)
4. [信息架构](02-design/information-architecture.md)
5. [用户流程](02-design/user-flow.md)
6. [全局交互规范](02-design/interaction-spec.md)
7. [产品设计基线收口评审](02-design/product-design-baseline-closure.md)
8. [核心流程低保真基线](02-design/core-flow-low-fidelity.md)
9. [首页模块评审稿](02-design/home-module.md)
10. [首页字段与条件矩阵](02-design/home-field-rules.md)
11. [首页低保真基线](02-design/home-low-fidelity.md)
12. [数据源管理评审稿](02-design/data-source-management.md)
13. [导出模块评审稿](02-design/export-module.md)
14. [导出参数映射基线](02-design/export-parameter-mapping.md)
15. [导出字段与条件矩阵](02-design/export-field-rules.md)
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
- 技术设计从首条纵向切片的真实门禁出发，不预先创建全部模块的空接口和数据表文档。
