# OB Data Orch · Global Design System Master v4

> 状态：**P0 Product IA / Shell / Archetype Source of Truth**
>
> 版本：4.1 · 2026-10-28
>
> 适用范围：OB Data Orch 的产品信息架构、全局外壳、页面原型、导航关系、重大结构与变更控制。

## 1. 目的、边界与权威关系

本文件只负责产品级 UI Architecture：Product IA、Global Shell、Page Archetype、Navigation、Task Builder 结构和重大产品 UI 约束。视觉方向、字体、颜色、密度、Ant Theme、Product CSS Variables 与组件语义由 [DESIGN.md](DESIGN.md) 负责；重复 UI 实现规则由 `patterns/` 负责；页面或 archetype 的特有收窄规则由 `pages/` 负责。

前端设计治理等级：

| 等级 | 载体 | 责任 |
| --- | --- | --- |
| P0 | `design-system/MASTER.md` | Product IA、Global Shell、Page Archetype、Navigation、重大结构 |
| P0 | `design-system/DESIGN.md` | 视觉方向、字体、颜色、密度、token、Ant ownership、实现禁区 |
| P0 | `design-system/patterns/*.md` | 复用 UI pattern 的实现规则 |
| P1 | `design-system/pages/*.md` | 页面或 archetype 的特有规则；只能收窄 P0，不能重定义全局视觉或 Shell |
| P3 | 产品、API、安全、状态、验证契约 | 业务、安全和事实真值；限制页面能表达和执行什么 |
| P4 | 现有前端实现 | 当前实现事实；不自动成为设计规范 |

开发页面前必须依次阅读：

```text
MASTER.md
DESIGN.md
patterns/<relevant>.md
pages/<page>.md（存在时）
适用 P3 产品 / API / 安全 / 验证契约
```

P0/P1 只约束设计决策；P3 的业务、安全和能力边界始终限制页面能表达和执行什么。P4 或历史资料与 P0/P1 冲突时，先判定它是 implementation debt、业务约束还是 MASTER / DESIGN defect；只有最后一种才可提议更新 P0。

## 2. 产品定位与安全边界

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台，面向中文用户、开发、运维和 DBA。它不是通用数据库客户端、SQL 编辑器、ETL 平台、监控大屏、远程 Shell 或文件浏览器。

页面只呈现已知、已授权、已脱敏且可追溯的事实。未知、未检查、过期、核对中、无权限和空结果不得伪装为成功或正常。

产品边界：

- 数据源独立管理；任务只选择已有、授权且符合资格的数据源。
- 页面不扩展官方工具能力，不绕过控制面命令生成器。
- 未验证能力隐藏、门控或 disabled + reason。
- 高风险、生产、删除、归档、禁用、提交和敏感命令操作需要明确影响、确定动作、二次确认与审计。
- 已提交任务不可原地改参。

## 3. Product Information Architecture

```text
OB Data Orch
├─ 运行概览：首页
├─ 任务配置：数据源管理、导出任务、普通导入、旁路导入、模板中心
├─ 运行与支持：任务中心、执行节点、日志中心
└─ 系统管理（可展开的父级导航）
   ├─ 平台设置
   │  └─ 页内分区：常规设置、调度策略、安全策略、工具与兼容
   ├─ 权限配置
   └─ 存储凭据
```

Global Navigation 只负责产品模块切换。Task Step Rail、日志预设、对象筛选、模块内字段分组和行选择均为 Contextual UI，不得升级为全局导航。

系统管理按 [DEC-064](../docs/01-product/decisions.md#dec-064-平台设置收拢与分组重设计) 组织平台设置、权限配置、存储凭据三个子项；父级只展开/收起。平台设置通过四个页内 Tabs 分别承载常规设置、调度策略、安全策略、工具与兼容，各域仍独立读取、编辑和保存，不合成全页事务。常规设置管理展示默认、任务命名及任务/普通日志保留；其他三域的业务边界不变。权限配置管理已有认证主体授权，存储凭据管理对象存储材料。完整导航使用缩进子项；48px 图标轨通过“系统管理”打开 Ant 子菜单。四个设置域均高亮平台设置，保留当前项与键盘访问。

## 4. Product Shell

### 4.1 Always-on Shell

| 区域 | 责任 | 不承担 |
| --- | --- | --- |
| Global Navigation | 稳定模块出口、分组、当前模块、权限可见性与折叠状态 | 向导进度、过滤条件、对象详情、表单摘要 |
| Product Header | 产品标识、帮助与账户等低优先全局操作；横跨页面顶部，导航位于其下 | 重复页面标题、面包屑、Search、Create、Refresh、Bulk action、页面筛选 |
| Module Context | 页面标题、范围、freshness、主要动作和 Page Toolbar | 全局导航或跨页任务状态 |
| Main Workspace | 当前页面唯一主判断与主要操作空间 | 被常驻 Inspector、重复摘要或无关卡片挤压 |

1920 和 1440 下 Global Navigation 保留完整模块出口。1280 下可折叠为 icon rail，但必须保留可访问名称、可见当前模块、tooltip 与明确模块出口。

### 4.2 Contextual UI

| Pattern | 使用条件 | 规则 |
| --- | --- | --- |
| Page Toolbar | Search、Filter、Create、Refresh、Bulk action、More | 属于页面，不放入 Product Header；Search → 高频筛选 → utilities |
| Inspector | 移除后会明显妨碍当前主要判断 | 只展示当前选择或诊断的高价值事实；不是常驻 Shell |
| Drawer | 编辑对象、连接测试、任务摘要、日志同来源上下文、窄屏 Inspector 替代 | 固定 Header、独立滚动 Body、可选固定 Footer |
| Dialog / AlertDialog | 确认、冲突、未保存离开、不可逆或高风险动作；首页图表放大与定界只读下钻 | 确认说明对象、影响、理由与动作；首页只读弹窗按 DEC-068，保持焦点、原窗口和授权，不承载普通编辑 |
| Fixed Action Footer | 长流程或需要持续主操作的页面 | 正文预留安全底距；不用于普通 CRUD 与详情页 |

## 5. Page Archetype System

任何页面只能选择一个主要 Archetype；可组合 Contextual UI，但不得把所有页面机械做成三栏、表格 + 表单并排或 Wizard。

| Archetype | Skeleton | Header / Toolbar | Secondary content | Drawer / Inspector / Footer | 使用场景 |
| --- | --- | --- | --- | --- | --- |
| Overview | 四项独立指标卡 → 单一趋势 / Attention 并列 → 我的任务、Runtime 节点通栏 | 授权范围、freshness、Refresh utility | 有限下钻列表 | 默认无 Inspector；无固定 Footer | 首页 |
| Management / Table | Header → FilterToolbar → Dense Table → pagination/state | Create + Search/Filter/Refresh | 行级 metadata 与 eligibility | 编辑/测试 Drawer；危险 Dialog | 数据源、模板、节点、凭据、权限配置 |
| Task Builder | Task header → Step Rail → current-step workspace | saved/dirty state、Save Draft、当前步骤动作 | 当前选择的派生事实 | 条件 Inspector、Summary Drawer、Fixed Footer | 导出、普通导入、旁路导入 |
| Task Operations | Header → FilterToolbar → scope bar → Task Table | Create / More、筛选与刷新 | 状态、阶段、可靠进度 | 默认无 Inspector；状态化行操作 | 任务中心 |
| Diagnostic / Detail | 稳定对象头 → 事实分区 → 证据/日志/记录 | permitted action + Back | failure/reconciliation banner | 单对象日志内联；跨对象转日志中心 | 任务/节点详情 |
| Settings | Header → one module workspace → optional field groups | 页内 Tabs 切换配置域；Edit/Save current domain，读写未接入时无保存入口 | 生效值、来源、时间、差异/影响；未接入时只说明边界 | 高影响确认 Dialog；无 Inspector | 平台设置内的常规、调度、安全、工具与兼容 |

平台设置四个配置域的职责、页签入口及读写边界见 [system-settings.md](pages/system-settings.md)。

Reference Page 对应关系：

| Reference | Archetype | Page Spec |
| --- | --- | --- |
| Overview | Overview | [overview.md](pages/overview.md) |
| Data Sources | Management / Table | [data-sources.md](pages/data-sources.md) |
| Export Builder | Task Builder | [export-builder.md](pages/export-builder.md) |
| Task Center | Task Operations | [task-center.md](pages/task-center.md) |
| Task Detail | Diagnostic / Detail | [task-detail.md](pages/task-detail.md) |

新页面先选择最接近 Reference / Archetype，再套用 [DESIGN.md](DESIGN.md) 与相关 pattern。禁止直接照 Ant Demo 页面开发。

## 6. Overview Rules

Overview 回答：需要关注什么、发生了什么变化、什么失败或被阻断、Runtime 是否健康、我的任务正在发生什么。

- 按 [DEC-066](../docs/01-product/decisions.md#dec-066-ocp-参考首页布局) 使用四张独立指标卡：运行中、等待调度、今日成功、今日失败；立体插画与轻阴影按 [DEC-068](../docs/01-product/decisions.md#dec-068-首页立体指标与独立趋势分析弹窗)。节点接收统计归节点区，不增加第五张指标卡。
- 借鉴 OCP 的“顶部概览卡、并列信息区、通栏明细”布局：趋势与 Attention 在桌面并列；我的任务、执行节点各用一个通栏工作面。容器不足 1000px 时并列区按 DOM 顺序合为单列。
- 受控摘要位于 Attention 正文顶部，失败与节点阻断事实紧随其后。首屏同时呈现指标、趋势和 Attention；过期、读取失败和权限变化仍优先通过明确反馈表达。
- 只保留一张有明确事实口径的趋势；不做资源曲线、预测、健康分、拓扑或多图表 Dashboard。
- 同一趋势可在独立 Ant Modal 放大或按时间桶下钻，两个视图互斥。分析弹窗复用现有授权任务列表、固定终态时间和分页；任务详情新标签页打开。主区仍是有限摘要，不在首页增加任务写操作、通用分析器或新的聚合接口。
- 节点、我的任务、失败与异常均为有限列表，只提供查看和下钻。
- Refresh 是 Utility Action，刷新可信快照而非发起业务操作。
- 授权范围、更新时间、部分失败、过期、无数据、无权限和未采集保持明确且不同状态。

## 7. Task Builder Framework

Export、Normal Import、Direct Load 共享同一 Task Builder Interaction Language，但各自的 Step Schema、资格、风险和命令语义独立。

- Global Navigation 始终是产品模块出口；进入 Builder 不隐藏它。
- Task Step Rail 只属于当前草稿 workflow context，不承担产品跳转。
- Step Rail 显示所有步骤；当前步骤带 `aria-current="step"`。
- 当前步骤可按决策模型使用 full-width table、single-column form、dual-column form、table + inspector、mapping workspace、object selector 或 precheck result。
- Previous 不丢失有效草稿；Continue 先做当前 step validation。
- Save Draft 是非破坏性保存，不代表可提交、已预检或命令有效。
- Precheck、Command Preview 与风险确认在字段变更后必须标记 stale。
- Submit 只在有效预检快照上可用；生产、高风险与敏感命令确认独立于 Save/Continue。

详细规则见 [task-builder.md](patterns/task-builder.md)。

## 8. Inspector、Drawer 与 Dialog

加入 Inspector 前必须回答：移除 Inspector 后，用户完成当前主要判断是否会明显变困难？若答案不是肯定，不加入。

| 适用 Inspector | 不适用 Inspector | 改用模式 |
| --- | --- | --- |
| Runtime selection、节点兼容性、路径/空间资格、Precheck diagnosis | Data Source List、Task List、Settings、ordinary Logs、完整历史任务摘要 | Main Workspace、Edit Drawer、Context Drawer、Detail 页面 |

Drawer 与 Dialog 的视觉和行为规则见 [drawers.md](patterns/drawers.md) 与 [action-hierarchy.md](patterns/action-hierarchy.md)。

## 9. Responsive Architecture

| Viewport | Global Navigation | Task Rail | Inspector | Workspace |
| --- | --- | --- | --- | --- |
| 1920 × 1080 | 完整 200px 导航 | 208px 紧凑轨道 | 当前判断需要时常驻 | 24px padding；优先给表格与多列 form |
| 1440 × 1024 | 完整 200px 模块出口 | 176px，保留编号/标签/状态 | 优先变为 Drawer | 24px padding；utilities 按需要收纳 |
| 1280 × 720 | 48px icon rail | 编号、短标签、状态；可展开 | 默认 Drawer | 24px padding；workspace 最后才收缩 |

降级顺序：

1. Inspector → Drawer。
2. Global Navigation 减少 secondary metadata。
3. Global Navigation → Collapsed Rail。
4. Task Step Rail compact。
5. Table 隐藏或合并低优先级 metadata。
6. Form dual-column → single-column。

1280 下核心操作不得依赖整页横向滚动。200% zoom 不缩小文字或隐藏风险。

## 10. Accessibility, Motion and Quality Gates

Accessibility 是 P0。所有可操作控件需要可见 focus ring；键盘顺序符合视觉和决策顺序；icon-only 控件提供 accessible name；状态不能只靠颜色。

最小验证矩阵：

```text
1920×1080
1440×1024
1280×720
200% Native Scaling
keyboard path
loading / empty / error / permission / unavailable
Drawer / Modal focus safety
long Chinese copy
long technical identifier
```

截图不等同于 DOM、键盘或无障碍验证。

### 视觉验收门槛

自 2026-10-19 起，所有含前端界面的模块在宣称开发完成前，必须通过以下八项视觉与交互验收。后续影响页面呈现、共享组件或同类交互的变更，按受影响范围重新验收；纯后端或文档变更可说明本门槛不适用，不免除其余工程与业务门禁。

- [ ] 同类型页面遵循统一布局和对齐规则
- [ ] 同类控件使用统一尺寸、样式和状态
- [ ] 表格、表单、抽屉、弹窗的视觉规则一致
- [ ] 同类操作具有一致的动效和反馈
- [ ] 不同模块之间没有明显的视觉密度落差
- [ ] 1920×1080 实际截图检查通过
- [ ] 1440×1024、1280×720 响应式检查通过
- [ ] 未发现无依据的模块私有样式或重复实现

### 验收证据与判定

- **比较基准：** 以现行 P0、对应 Pattern / P1 和数据源定版复用规则为准；选择同一 Archetype 的参考页对比。不同业务结构比较共同设计元素，允许已登记例外，不强行统一页面结构。
- **实际页面：** 启动待验收实现，用浏览器实际检查三档视口并保存代表截图；1920×1080 是主要视觉对比尺寸。截图必须对应本次待验收版本，历史截图只作比较基线，设计稿、DOM 尺寸或自动化断言不能单独替代截图目视检查。共享视觉变更同时复查数据源标准页及受影响的参考页。
- **关键状态：** 除默认页面外，检查实际存在的加载、禁用、空态、异常、字段校验、保存成功/失败、危险确认、取消/关闭和未保存输入处理；实际打开、关闭 Drawer / Modal，核对动效、反馈与焦点恢复。无对应组件或操作时说明不适用理由，不为验收新增业务能力；继续满足上方既有键盘、200% 缩放等验证矩阵。
- **记录方式：** 沿用模块现有验收记录，逐项记录结论、比较页面、实现版本、浏览器版本、视口、检查状态和截图/交互证据路径。有未提交改动时同时保留本轮差异或工作树标识，不能仅用 HEAD 代表实际验收内容。明确区分代码静态检查、浏览器实际验证和截图视觉检查。
- **样式与复用：** 对本次模块及受影响共享实现的相关审计 REVIEW 补充人工结论、依据与处置；已登记例外按限定范围验收。`tokens:check`、`audit:platform`、构建或功能测试通过，均不能代替这八项验收。
- **完成判定：** 各项使用“通过、不通过、未验证、无法验证、不适用”记录；有前端页面时三档视口检查必须实测。只有所有适用项通过，才可声明模块视觉验收通过及前端交付完成；未验证、无法验证或不通过项不得勾选。可分别报告已实现、合成验证通过和待验收范围，不把视觉验收外推为真实执行、部署或上线通过。验证沿用既有合成夹具、隔离环境和真实准入边界。

## 11. Change Control

2026-10-28 用户进一步要求四卡具有 OCP 式立体感，趋势借鉴 OCP 细线、浅网格、底部图例，放大与下钻均独立弹出，并选择弹窗直接展示对应任务列表。按 DEC-068 仅为首页四卡允许生成的透明立体插画与既有 surfaceShadow；图形仍连接真实计数，命中区使用 Ant Button。结果分析通过既有 GET 任务列表读取选中半开窗口，关闭、切换和失权隔离旧会话；取代页内 Collapse 下钻，不改变 Shell、Token、API 定义或任务写权限。

2026-10-28 按用户提供的客户标识及 [DEC-067](../docs/01-product/decisions.md#dec-067-登录页采用客户品牌标识) 重做登录页：品牌区 + 登录卡片左右分栏，标识蓝绿只用于页内纹样。登录页在 Shell 之外，规则见 [login.md](pages/login.md)；不修改 Shell、Archetype、Token 或其他页面。

2026-10-28 用户指出当前首页约束不符合要求，明确要求参考已打开的 OCP 首页同步修订设计与实现，并选择继续使用四项既有任务指标。本轮以 [DEC-066](../docs/01-product/decisions.md#dec-066-ocp-参考首页布局) 取代 DEC-060 的状态带、分区顺序及任务/节点并排规则：四卡 → 趋势 / 关注并列 → 任务、节点各自通栏。分区使用独立标题栏和细边框；只消费既有 Token、Ant 与 Shell，业务授权、快照、指标及下钻口径不变。旧 Change Control 保留为历史记录，不继续约束首页现役骨架。

2026-10-26 按用户明确要求将调度策略、安全策略、工具与兼容收进平台设置并重做界面。系统管理恢复三个子项，平台设置使用常规设置、调度策略、安全策略、工具与兼容四个原生 Ant 页签，旧深链接保持可用。设置分组采用用途说明与事实/操作两列，窄屏合并为单列；编辑及刷新紧邻所属标题，当前值先于生效说明。此项更新导航归属和 Settings 内部组织，不修改 Shell 几何、其他 Archetype、Token、权限、API 或各域独立事务；取代下条四页并列的现役入口，详见 [DEC-064](../docs/01-product/decisions.md#dec-064-平台设置收拢与分组重设计)。

2026-10-23 按用户采纳的拆分评估及 [DEC-062](../docs/01-product/decisions.md#dec-062-系统设置拆分为四个独立配置域)，系统管理新增平台设置、调度策略、安全策略、工具与兼容四个独立配置域，取代系统设置四个横向分区。Settings Archetype 使用独立工作面及可选字段分组，权限配置明确归属已有 Management / Table。当前只完成页面/导航职责拆分，配置事实未接入时保持未知且不提供修改，工具先展示受控目标基线；不增加配置 API、存储表或发布能力。六个子项沿用原 Shell 几何、Ant 控件和 Token。

2026-10-23 按用户明确要求，将原“平台设置”的平铺入口调整为“系统管理”可展开父级，下设系统设置、权限配置、存储凭据。权限配置从系统设置安全分区移出，三个模块保持独立页面；原路由兼容，折叠图标轨使用 Ant 子菜单。此项只修订全局导航层级，不改变 Shell 几何、Archetype、Token、权限或业务能力，详见 [DEC-061](../docs/01-product/decisions.md#dec-061-系统管理父子导航与模块边界)。

2026-10-23 按用户选定的 [DEC-060](../docs/01-product/decisions.md#dec-060-首页关注优先结构) 修订 Overview Archetype 骨架：通栏状态带（四项任务指标 + 节点不可接收数）→ 含受控摘要的 Attention → 我的任务与执行节点并排 → 降低高度的唯一趋势。视觉优先级规则不变，只是骨架顺序与其对齐；不修改 Shell、Token、组件语义表或其他 Archetype。

2026-10-20 按用户采纳的 [DEC-056](../docs/01-product/decisions.md#dec-056-旁路五步配置与入口集中确认)，旁路收敛为“数据源与连接、导入目标、文件与格式、执行参数、预检查与提交”五步。入口阅读确认改为首步有效继续时的集中弹窗，最终快照风险确认不变；三类模块的数据源选择复用同一筛选与单选列表布局。前四步仅表达本地配置完整度，第五步表达九项当前预检查进度；取代下条六步序号，不修改 Shell、Archetype、Token 或真实执行准入。随后按 [DEC-057](../docs/01-product/decisions.md#dec-057-旁路导入限定本地部署) 限定本地部署：只展示私有 ODP / OBServer、可编辑默认 RPC 2885 / 2882 与 CSV；活动目录为 102 项，不显示其他部署专属入口或说明。

2026-10-19 按用户旁路审查整改要求，三类向导统一使用顶部水平步骤进度栏；旁路仍为独立六步，前五步输入完整度与第六步有效检查进度分开统计，工具执行与准入不变。摘要、表内操作、事实展示及安全确认沿用现有共同规则，详见 [Task Builder Pattern](patterns/task-builder.md#2026-10-17--2026-10-19-三类向导一致性整改)。此项取代此前旁路竖向进度栏的呈现差异，不修改 Product Shell、Archetype 或 Token。

2026-10-19 退出登录交互修订：复用当前编辑会话的离开守卫，确认先于注销；注销失败保留输入与原生卸载保护，服务端确认后放行登录页跳转，避免重复提示。规则见 [Action Hierarchy Pattern](patterns/action-hierarchy.md#未保存输入与退出登录)，Shell 几何、Archetype 和 Token 不变。

2026-10-19 用户补充全模块[视觉验收门槛](#视觉验收门槛)：新增八项完成条件，要求当前版本三档浏览器检查、截图视觉对比和关键交互证据；未实测不能宣称通过。清单与判定规则仅在本文件维护，任务地图和前端平台基线仅链接到此入口。

2026-10-19 按用户“按现有情况更新规范”的要求，明确主工作面的现有浅阴影、管理表格与预检查表的不同密度，登记限定的图标/环境选项和节点额度图形例外，并区分平台审计的硬违规与选择器待复核信号。详见 [DESIGN v4.1](DESIGN.md)、[Tables Pattern](patterns/tables.md)与[规范校准记录](../docs/03-technical/evidence/platform-design-baseline-2026-10-19.md)。保持现有视觉方向、Shell、Archetype、业务状态与执行门禁；未引用 CSS 不因本次登记而成为推荐模式。

2026-10-12 全平台操作结果与持续状态的反馈调整已记录于 [Status and Feedback Pattern](patterns/status-feedback.md#全平台反馈规则)，按用户确认统一 Message / Alert / Form.Item，Shell、Archetype 与 Token 保留原权威。

2026-10-13 按 [DEC-050](../docs/01-product/decisions.md#dec-050-任务中心重构范围与收窄规则) 登记 Task Center 页面规则入口，并取代 Task Detail 占位；两份 P1 已经用户评审确认，Archetype、Shell 与 Token 不变。

2026-10-15 按用户明确修订及 [DEC-052](../docs/01-product/decisions.md#dec-052-普通导入沿用导出布局与三种文件格式)，普通导入沿用导出的顶部水平 Steps、全宽工作面、摘要和底栏；旁路维持独立流程。具体呈现与未接入门禁见 [Task Builder Pattern](patterns/task-builder.md)，不改变全局 Shell、Archetype 或 Token。

- 改动 Product Shell、Page Archetype、Task Builder、Table model、Drawer anatomy、Status semantics、风险确认或 Reference Page 身份时，必须记录 Design System Change。
- Token calibration 只能修改已确认视觉方向中的值，记录影响组件、前后值和验证结果。
- 页面实现必须先选择 Archetype，再应用 DESIGN 与 patterns；不得因实现方便重新引入统一三栏、Card-heavy 或 Wizard-all-the-things 骨架。
- 现有实现只表示 P4 事实，不能静默覆盖 P0/P1。
