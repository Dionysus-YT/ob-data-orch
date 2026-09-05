# OB Data Orch · Global Design System Master v2

> 状态：**GLOBAL VISUAL CRAFT FROZEN · P0 GLOBAL UI/UX SOURCE OF TRUTH**
>
> 版本：2.1 · 2026-09-04
>
> 适用范围：OB Data Orch——面向开发、运维和 DBA 的轻量化 Database Operations Workbench。

## 1. 目的、边界与权威关系

本文件是后续页面设计、组件设计、Visual QA 与前端重构的全局 UI Source of Truth。它沉淀已批准的产品级 UI Architecture 与 **Refined Technical Operations Workbench** Visual Foundation；不改变产品能力、API、安全模型、工具参数、发布门禁或事实状态。

前端设计治理等级：

| 等级 | 载体 | 责任 |
|---|---|---|
| P0 | `design-system/MASTER.md` | 全局 UI/UX、Product Shell、Page Archetype、Token 与响应式的唯一规范 |
| P1 | `design-system/pages/*.md` | 已进入页面精修 READY 后才存在的页面/Archetype 特有规则；仅可收窄 P0，不能重定义 Shell 或全局 Token |
| P2 | 已批准的 Representative High-Fidelity Baseline | 用于视觉校准，不以截图替代交互、无障碍或业务验证 |
| P3 | PRD、产品规则、API/安全/技术契约、验证矩阵与测试用例 | 业务、安全和事实真值；不定义新的视觉语言 |
| P4 | 现有前端实现 | 当前实现事实，不自动成为设计规范 |

P0/P1 只约束设计决策；P3 的业务、安全和能力边界始终限制页面能表达和执行什么。P4 或历史资料与 P0/P1 冲突时，先判定它是 implementation debt、业务约束还是 MASTER defect；只有最后一种才可提议修改 P0。冻结 Baseline 不得由实现静默替换；需要改变时建立 DCR。Token 校准只能由真实浏览器证据支持。

### 产品定位与安全边界

- 产品是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台，不是通用数据库客户端、SQL 编辑器、ETL 平台或监控墙。
- 页面只呈现已知、已授权、已脱敏且可追溯的事实。未知、未检查、过期、核对中、无权限和空结果绝不伪装为成功或正常。
- 平台不能通过 UI 扩展官方工具能力。未验证能力必须隐藏、门控或 disabled + reason；不能被视觉优化成已可用。
- 数据源独立管理；三类任务只选择已有、授权且符合资格的数据源，不在任务中编辑连接或凭据。
- 命令由控制面唯一生成，始终只读、默认脱敏；页面不得成为任意命令、任意 SQL、远程 Shell 或文件浏览入口。
- 高风险、生产、删除、归档、禁用、提交和敏感命令操作需要明确影响、确定动作、二次确认与审计；已提交任务不可原地改参。

## 2. 设计原则与视觉语言

1. **先判断事实，再给操作。** 资格、状态、来源、预检、原因与证据先于装饰和 CTA。
2. **高密度而可读。** 通过对齐、排版、分组、留白、分层 divider 与有意义的 surface 提高密度；不缩小正文、不堆叠 Card、不使用高对比 zebra table。
3. **按用户意图组织页面。** 看态势、管理对象、构建任务、操作任务、诊断事实、配置平台使用不同页面原型。
4. **渐进披露，不删除能力。** Required 先出现；Recommended 说明收益；Advanced/Expert 只在单一可发现入口中按权限、适用性和风险出现。
5. **一致性不等于同一骨架。** 全产品统一视觉语言、控件、状态、间距、排版、动作和反馈；不强制统一 Inspector、Drawer、Footer、Toolbar 或内容布局。
6. **操作可检查、可恢复、可追溯。** 选择、派生、校验、风险确认、预检、脱敏命令、不可变快照、日志和结果构成连续证据链。

### Refined Technical Operations Workbench

视觉基础：**Precision Grid + Refined Enterprise Surface + Technical Brand Expression**。

- Technical、precise、content-first、high-density、long-session friendly；结构化事实优先于装饰，精修但克制。
- Low Chrome 不等于 No Surface；High Density 不等于 No Breathing Room；Technical 不等于 Debug UI；Minimal 不等于 Flat；Consistency 不等于 identical skeleton。
- 页面以中性矿物底色、清晰 Workspace、克制的 surface 层级、对齐和 divider 建立稳定阅读节奏。只有独立语义边界才使用 surface：运营摘要、当前决策、失败/警告、技术证据、编辑/Drawer 工作区。
- 专业蓝只表达 interaction、selection、focus、可达 link 与必要结构锚点；状态仅使用语义色，且必须同时有 indicator/icon、文本和可用时的时间或原因事实。
- 禁止 Marketing SaaS、Bento、Hero、Glass、Aurora/渐变、Card wall、oversized rounded surface、IDE cosplay、监控大屏与消费型 Dashboard。

## 3. Product Information Architecture

```text
OB Data Orch
├─ 运行概览：首页
├─ 任务配置：数据源管理、导出任务、普通导入、旁路导入、模板中心
├─ 运行与支持：任务中心、执行节点、日志中心
└─ 平台设置：系统设置、存储凭据
```

Global Navigation 只负责产品模块切换。Task Step Rail、日志预设、对象筛选、设置分区和行选择均为 Contextual UI，不得升级为全局导航。

## 4. Product Shell

### 4.1 Always-on Product Shell

| 区域 | 责任 | 不承担 |
|---|---|---|
| Global Navigation | 稳定模块出口、分组、当前模块、权限可见性与折叠状态 | 向导进度、过滤条件、对象详情、表单摘要 |
| Product Header | 产品标识、面包屑/全局上下文、帮助与账户等低优先全局操作 | Search、Create、Refresh、Bulk action、页面筛选 |
| Module Context | 页面标题、范围、freshness、主要动作和 Page Toolbar | 全局导航或跨页任务状态 |
| Main Workspace | 当前页面唯一主判断与主要操作空间 | 被常驻 Inspector、重复摘要或无关卡片挤压 |

1920 和 1440 下 Global Navigation 必须保留。1280 下可折叠为 icon rail，但必须保留可访问名称、可见当前模块、tooltip 与明确模块出口。

### 4.2 Contextual UI

| Pattern | 使用条件 | 规则 |
|---|---|---|
| Page Toolbar | Search、Filter、Create、Refresh、Bulk action、More | 属于页面，不放入 Product Header；Search → 高频筛选 → utilities |
| Inspector | 移除后会明显妨碍当前主要判断 | 只展示当前选择/诊断的高价值事实；不是常驻 Shell |
| Drawer | 编辑对象、连接测试、完整任务摘要、日志同来源上下文、窄屏 Inspector 替代 | 固定 Header、独立滚动 Body、可选固定 Footer |
| Dialog / AlertDialog | 确认、冲突、未保存离开、不可逆或高风险动作 | 说明对象、影响、理由与确定动作；不承载普通编辑 |
| Fixed Action Footer | 长流程或需要持续主操作的页面 | 正文预留安全底距；不用于普通 CRUD 与详情页 |

### 4.3 Action model

- 一个 Action Context 最多一个 Primary，但页面或区域不要求存在 Primary。Secondary 用于 Save、Refresh、Retry、Previous；Text 用于 Cancel、Clear、Back；Danger 只在确定确认层。
- Refresh 是刷新事实快照的 Utility Action，不是 Primary Business Action；Overview 可以只提供 Refresh 与下钻入口而没有 Primary。
- loading 保留动作名、避免重复提交，并提供可读原因；disabled 不能只靠灰色或 hover tooltip。
- 生命周期资格在菜单打开前由服务端确定。Delete、Archive、Disable 等是不同且确定的动作，不能点击后再变更语义。

## 5. Page Archetype System

| Archetype | Skeleton | Header / Toolbar | Secondary content | Drawer / Inspector / Footer | 使用场景 |
|---|---|---|---|---|---|
| Overview | 共享运营摘要面 → 单一趋势 → 节点/任务/Attention/Activity | 授权范围、freshness、Refresh utility | 有限下钻列表 | 默认无 Inspector；无固定 Footer | 首页 |
| Management / Table | Header → FilterToolbar → Dense Table → pagination/state | Create + Search/Filter/Refresh | 行级 metadata 与 eligibility | 编辑/测试 Drawer；危险 Dialog | 数据源、模板、节点 |
| Task Builder | Task header → Step Rail → current-step workspace | saved/dirty state、Save Draft、当前步骤动作 | 当前选择的派生事实 | 条件 Inspector、Summary Drawer、Fixed Footer | 导出、普通导入、旁路导入 |
| Task Operations | Header → FilterToolbar → scope bar → Task Table | Create / More、筛选与刷新 | 状态、阶段、可靠进度 | 默认无 Inspector；状态化行操作 | 任务中心 |
| Diagnostic / Detail | 稳定对象头 → 事实分区 → 证据/日志/记录 | permitted action + Back | failure/reconciliation banner | 单对象日志内联；跨对象转日志中心 | 任务/节点详情 |
| Settings | Header → compact section nav → one section workspace | Edit/Save current section | 生效值、来源、时间、差异/影响 | 高影响确认 Dialog；无 Inspector | 系统设置、存储凭据 |

任何页面只能选择一个主要 Archetype；可组合 Contextual UI，但不得把所有页面机械做成三栏、表格 + 表单并排或 Wizard。

## 6. Task Builder Framework

Export、Normal Import、Direct Load 共享同一 Task Builder Interaction Language，但各自的 Step Schema、资格、风险和命令语义必须独立。

### 6.1 Global Navigation 与 Task Navigation

- Global Navigation 始终是产品模块出口；进入 Builder 不隐藏它。
- Task Step Rail 只属于当前草稿的 workflow context，不承担产品跳转。
- Step Rail 在 1920/1440 使用紧凑纵向轨道：编号、标签、状态和必要短摘要；禁止 6 个大型 Step Card。
- Task Rail 显示所有步骤；当前步骤带 `aria-current="step"`，不把六步隐藏在单一 Dropdown。
- Framework 统一的是导航、状态、动作、校验和证据链，不是每一步的内容布局。当前步骤可按决策模型使用 full-width table、single-column form、dual-column form、table + inspector、mapping workspace、object selector 或 precheck result。

### 6.2 Step state 与导航

| State | 含义与呈现 |
|---|---|
| Complete | 已满足且当前有效；编号/图标 + 文本 + 成功语义，不以颜色单独表达 |
| Current | 正在编辑；蓝色 interaction 标识、明确标题与当前字段上下文 |
| Pending | 尚未配置或前序未满足；可见但不伪装为错误 |
| Blocked | 被冲突、权限、证据或前置条件阻断；显示原因与修复入口 |

- Previous 不丢失有效草稿；Continue 先做当前 step validation，再移动到下一个可达步骤。
- Save Draft 是非破坏性保存，不代表可提交、已预检或命令有效。它固定在 Builder Header 的任务上下文动作区，紧邻 `已保存` / `有未保存更改` 状态，作为 Secondary / Utility Action；不能与 Footer 的 Continue、Precheck 或 Submit 竞争 Primary。
- 三类 Builder 都使用同一 Save Draft 位置和语义。Footer 只保留必要的 Back/Previous 与当前步骤唯一 Primary，避免形成按钮墙。
- 字段变更影响命令时，Precheck、Command Preview 与风险确认必须标记 stale 并在提交前重新生成。

### 6.3 Validation、precheck 与提交

- 字段在 blur、Continue、Precheck、Submit 验证；首次进入不报错。多错误操作提供可聚焦 Error Summary，链接 invalid field，同时保留 inline error、原因与恢复路径。
- 每个步骤或 major form 最多一个 `Advanced Settings` disclosure；内部按官方参数类别分组，不再嵌套 card/accordion。
- Inspector 仅呈现 Runtime、Compatibility、路径资格或 Precheck diagnosis 等当前判断所需事实。完整 Task Summary 按需进入 Summary Drawer。
- Precheck 按检查域展示 pending/running/passed/warning/failed、原因、影响和修复入口；failed 阻断，warning 仅按已确认规则可继续。
- Command Preview 来自控制面唯一生成器，默认脱敏、只读、内部横滚、可复制脱敏版本；不得编辑命令绕过校验。
- Submit 只在有效预检快照上可用。生产、高风险与敏感命令确认独立于 Save/Continue，确认后仍需服务端复验。
- 错误恢复保留输入和草稿，不伪造进度或状态；已提交任务通过基于原配置新建、从头执行或条件化 checkpoint continue 恢复。Direct Load 永不提供 checkpoint continue。

## 7. Inspector、Drawer 与 Dialog Rules

加入 Inspector 前必须回答：**移除 Inspector 后，用户完成当前主要判断是否会明显变困难？** 若答案不是肯定，不加入。

| 适用 Inspector | 不适用 Inspector | 改用模式 |
|---|---|---|
| Runtime selection、节点兼容性、路径/空间资格、Precheck diagnosis | Data Source List、Task List、Settings、ordinary Logs、完整历史任务摘要 | Main Workspace、Edit Drawer、Context Drawer、Detail 页面 |

- Inspector 只展示当前步骤/选择的可行动事实，不复刻完整表单或全部完成步骤。
- Data Source 新增、编辑、连接测试使用 Drawer；Task Summary 与日志同来源上下文使用 Drawer。
- Dialog 要 trap focus、支持 ESC（无不可逆进行中时）、关闭后 return focus；AlertDialog 默认焦点是安全操作。

### Data Source Save / Test Connection

- Save 与 Test Connection 是两个独立业务动作。Save 只持久化结构化配置，不发起真实连接测试；新记录保存后状态是“未测试”，不能作为新任务候选。
- Test Connection 基于已保存且当前有效的配置发起基础连接测试。表单存在未保存连接字段变更时，Test Connection disabled，并说明“请先保存连接配置后再测试”；保存后再允许测试。
- 已保存记录未填写新密码时，测试可使用已保存凭据，页面不得回显密码。关键连接字段变更会使旧测试结果失效并重置为“未测试”。
- 项目支持的 `保存并测试连接` 仅是显式 Composite Action：先成功持久化，再基于该已保存版本发起测试；它不是默认 Save 的替代，也不能掩盖中间保存失败或测试失败。测试结果独立形成连接状态和最近测试事实。

## 8. Responsive Architecture

| Viewport | Global Navigation | Task Rail | Inspector | Workspace |
|---|---|---|---|---|
| 1920 × 1080 | 完整 216px 分组导航 | 208px 紧凑轨道 | 当前判断需要时常驻 | 32px padding；优先给 dense table 与多列 form |
| 1440 × 1024 | 完整模块出口，压缩次级 metadata | 176px，保留编号/标签/状态 | 优先变为 Drawer | 24px padding；utilities 进入 More |
| 1280 × 720 | collapse 为清晰 icon rail | 编号、短标签、状态；可展开 | 默认 Drawer | 20px padding；workspace 最后才收缩 |

降级严格按此顺序执行：

1. Inspector → Drawer；
2. Global Navigation 减少 secondary metadata；
3. Global Navigation → Collapsed Rail；
4. Task Step Rail compact；
5. Table 隐藏或合并低优先级 metadata；
6. Form dual-column → single-column。

1280 下核心操作不得依赖整页横向滚动。Table 保留 identity、状态和当前判断的关键列；路径、长命令和日志文本可在自身容器横滚，并提供非 hover-only 的完整值路径。固定 Header/Footer/Drawer 必须保留内容和焦点安全边距。200% zoom 不缩小文字或隐藏风险。

## 9. Final Visual Foundation and Tokens

以下为已由 Overview、Data Sources、Export Builder 与 Task Detail 联合验证的 P0 Token。不得与旧 Token 并行使用；生产迁移只能从本表取值，页面组件不得新增 raw hex 或本地替代尺度。

### 9.1 Layout, spacing and typography

| 类别 | Token | 冻结值 / 规则 |
|---|---|---|
| Viewport | `baseline / validation` | 主基准 `1920×1080`；验证 `1440×1024`、`1280×720` |
| Structural | `global-nav / product-header / task-rail` | `216 / 48 / 208px`；1440 Task Rail `176px`；1280 按第 8 节降级 |
| Workspace | `padding-1920 / 1440 / 1280` | `32 / 24 / 20px`；Main Workspace 最后才被压缩 |
| Spacing | `space-1/2/3/4/6/8` | `4 / 8 / 12 / 16 / 24 / 32px`；不引入碎片间距 |
| Typography | `page-title` | `27/34px`，600–700；只用于页面身份和当前主任务 |
| Typography | `drawer-title / section-title` | `17/24px`、`16/24px`，600–700；与 Page Title 保持可辨层级 |
| Typography | `body / label / metadata` | `13/20px` 400；`12/18px` 600；metadata 最小 `12/18px`，不得以 10–11px 换密度 |
| Typography | `technical-evidence` | `12/18px` monospace；只用于 Command、Log、Endpoint、Version、Path、ID、Trace 与必要数值 |
| Font | `ui / mono` | UI：`"Microsoft YaHei UI", "Segoe UI", system-ui, sans-serif`；mono：`"Cascadia Mono", Consolas, ui-monospace, monospace`。普通导航、面包屑与表单不使用 monospace。 |

### 9.2 Controls, table and interaction

| 类别 | Token | 冻结值 / 规则 |
|---|---|---|
| Controls | `control-height / icon-action` | 标准 Button、Input、Select、Search 均为 `36px`；icon-only action 为 `32px`。同一 Toolbar 不混用高度。 |
| Controls | `control-radius` | `4px`；白或低饱和填充面、可见 1px border、无位移 hover。 |
| Table | `header / row-fact / row-single` | header `38px`；双行事实行 `44–45px`；单行 dense fact row `40px`。按事实层级选择，不将 44px 强制用于全部管理表。 |
| Table | `selected / hover` | Selected 使用低饱和蓝 surface + 左侧 `3px` selection rail；hover 使用极弱中性/蓝灰面，不改变布局。 |
| Table | `fact-hierarchy` | Primary fact：13px 600–700；Secondary fact：12px 次级文字；technical metadata：12px mono；status：indicator + text + 可选时间；action：末列低权重 icon/utility。 |
| Focus | `focus-ring` | `2px` interaction blue，`2px` offset；所有可操作控件一致可见。 |
| Motion | `feedback` | hover/focus/press 使用 `120–160ms` 的 opacity、border、background 或 shadow 过渡；不以 motion 表达真实状态；`prefers-reduced-motion` 下禁用非必要 transition。 |

### 9.3 Surface, divider, radius, shadow and color

| 类别 | Token | 冻结值 / 规则 |
|---|---|---|
| Surface | `page / workspace / subtle` | `#EDF1F5 / #FFFFFF / #F8FAFC`；Page 负责全局背景，Workspace 承载主要判断，Subtle 承载低权重结构。 |
| Surface | `selected / failure / warning / evidence` | `#EAF3FF / #FFF9F9 / #FFF4DF / #F2F6FA`；仅用于真实选择、失败/警告或技术证据语义，不作装饰。 |
| Surface | `drawer / overlay` | Drawer `#FCFDFF`；Overlay 使用可读 scrim，Drawer 自身是独立编辑工作区。 |
| Divider | `row / section / strong` | `#E1E7EE / #D5DFE9 / #CCD6E1`；row 用于连续事实，section 用于阅读分区，strong 用于 Shell、表头和主要 Workspace 边界。 |
| Radius | `control / semantic-surface / overlay` | `4 / 6–7 / 7–8px`；普通状态不使用 pill，禁止大圆角。 |
| Shadow | `work-surface / overlay` | 工作 Surface 仅 `0 1px 3px rgb(30 51 75 / 5%)`；Drawer/Menu 使用 `0 12px 28px rgb(29 50 76 / 18%)` 或等效有限 elevation；不制造浮层堆叠。 |
| Interaction | `primary / primary-hover / selected` | `#1767C7 / #1156A8 / #EAF3FF`；Primary 每个 Action Context 最多一个。 |
| Text | `primary / secondary / muted / disabled` | `#172433 / #52647A / #718096 / #A1ADBB`；文本对比度至少 4.5:1。 |
| Status | `success / warning / error / info / neutral` | `#177B55 / #9B6517 / #B8323C / #1767C7 / #66758A`；不得只依赖颜色或大面积状态色。 |

Brand blue 是 Product Shell、selection rail、current decision、focus 和 primary action 的共同锚点；它不是背景色、营销色或大面积装饰色。非文本边界、状态 indicator 和 focus 对比度至少 3:1。

### 9.4 OB Data Orch Visual DNA

1. **Grouped Global Navigation**：稳定分组、清晰 icon/text hierarchy、restrained selected surface 与 `3px` active rail；导航表达产品位置，不承担步骤、筛选或摘要。
2. **Structured Fact Presentation**：每个连续事实面都区分 Primary fact、Secondary fact、Technical metadata、Status 与 Action；视觉重量随用户当前判断价值下降。
3. **Semantic Surface**：只用于真实独立语义边界——Shared Operational Summary、Current Decision、Failure/Warning、Evidence、Editor/Drawer workspace。禁止将一般段落或所有区块包装为 Card。
4. **Technical Evidence**：Command、Log、Endpoint、Version、Path、ID、Trace 使用 monospace 和 Evidence surface；其他内容保持 Chinese-first UI typography。
5. **Status with evidence**：Status 必须由 indicator/icon、文本和可用时的时间/原因共同表达；不只依赖颜色，也不以大面积语义色装饰页面。

## 10. Component Visual Contract

本节只规定视觉与交互契约，不强制每个 Pattern 必须对应一个 Vue primitive。是否抽取 Input、Select、Table、Drawer、Menu 或 Alert，由生产迁移中的真实复用、可访问性和测试边界决定。

### Product Shell、navigation and actions

| Pattern | Visual / interaction contract |
|---|---|
| Global Navigation | `216px` 分组导航；分组 label 使用 12px 以下的次级层级，模块项使用正常 UI 字体；当前项为 restrained selected surface + `3px` blue rail，hover 不改变布局。1280 仅折叠呈 icon rail，不改变 taxonomy。 |
| Product Header / Breadcrumb | `48px`、Workspace 上方的清晰 strong divider；面包屑为次级正常 UI 字体，Help/Account 为低优先全局动作。不得承载 Search、Create、Refresh 或页面筛选。 |
| Primary Button | `36px`、4px radius、蓝色实面与克制 `2–4px` shadow；仅承担当前 Action Context 唯一 Primary。hover 更深，不位移。 |
| Secondary / Utility / Icon Button | Secondary 为白面 + 可见边界；Utility/Ghost 为透明面 + hover surface；icon action `32px`、有 accessible name。三者必须弱于 Primary。 |
| Link / focus / disabled | Link 仅在可达导航或下钻事实中使用 interaction blue；所有交互元素使用统一 focus ring；disabled 降低 text、surface 与 border，并保留可读原因，不能只靠灰色。 |

### Inputs, toolbar, table and status

| Pattern | Visual / interaction contract |
|---|---|
| Input / Select / Search | `36px`、4px radius、低饱和填充面、可见 border；Search 使用 16px leading icon 与保持可读的输入起点；hover 加强 border，focus 使用 P0 ring。 |
| FilterToolbar | Search → 高频筛选 → Clear/Refresh → More；作为紧凑 toolbar surface，可有 1px 边界与极弱工作阴影。1920 单行；收窄时先收 utility，不变成第二张 Card。 |
| WorkbenchTable | 一张 dense structured-fact surface：38px header band、row divider、事实层级、明确 selected/hover/disabled/empty/loading/error；禁止高对比 zebra、纵向网格和平均列宽。 |
| Table Header / Row | Header 使用 Subtle surface、强 divider、12px 600–700 label；行按 single-line 40px 或 two-line 44–45px；primary/secondary/technical/status/action 依第 9 节固定层级。仅真实可排序列提供 button 与 `aria-sort`。 |
| Status / Overflow Menu | Status 固定为 indicator/icon + text + 可选时间/reason；不同 domain 不得混写为 Active/Inactive。Overflow Menu 是小型 overlay workspace，7–8px radius、有限 shadow、keyboard accessible，菜单打开前即由服务端确定资格。 |

### Forms, feedback, drawer and evidence

| Pattern | Visual / interaction contract |
|---|---|
| Form Label / Helper | `label → required/optional → control → persistent helper → inline validation`；label 12px 600–700，helper 12px 次级文字。placeholder 不能替代 label。 |
| Alert | 只在 failure、warning、stale 或需要判断的事实使用 semantic surface；使用左侧 3–4px semantic rail、文本和事实，不用大面积状态色或装饰性图标。 |
| WorkbenchDrawer | 独立 editor workspace：固定 Header（title、optional subtitle、close）→ independent scroll Body → optional fixed Footer；Drawer 7–8px overlay radius、有限 shadow、不可与 Product Header 混合。 |
| Command Surface | 只读、脱敏、等宽、内部横滚；以 Evidence surface + blue structural rail 区分普通 metadata，计划命令与实际命令必须区分来源和事实状态。 |
| Log Table / Operation History | Log 保持高密度 structured table，保留来源、时间、级别、完整性和脱敏事实；Operation History 使用时间顺序与轻量轨迹，不伪造调用链。 |
| Empty / Loading / Error | Empty 说明范围、原因和唯一下一步；Loading 保留空间与可信旧数据；Error 说明对象、freshness、恢复动作和追踪标识。未知、过期、无权限和采集缺口不得归并为普通 error。 |

Readonly 是可复制但不可编辑的事实；Disabled 是当前不可用的动作/输入，必须有可读原因。依赖、互斥与条件显示在选择时收敛；Required、Recommended、Advanced/Expert 均不能绕过校验、权限、版本或证据门槛。

## 11. Overview Rules

Overview 回答：**需要关注什么、发生了什么变化、什么失败或被阻断、Runtime 是否健康、我的任务正在发生什么。**

- 使用 `Shared Operational Summary Surface`，而不是 independent KPI card system：3–5 个有真实业务事实、能帮助判断运行状态的关键指标共享一个语义 surface；不冻结为固定四项，也不为版式构造 card。
- 新指标优先替换既有低价值事实，而不是继续增加独立摘要；Overview 不得形成 Card Wall。
- 页面视觉优先级固定为：**Failure / Blocked > Runtime Health > Running Tasks > Recent Activity > Trend / KPI embellishment**。Recent Activity 是次级时间事实，不能与 Needs Attention 争夺主层级。
- 只保留一张有明确事实口径的任务结果趋势；不做资源曲线、预测、成功率目标、健康分、拓扑或多图表 Dashboard。
- 节点、我的任务、失败与异常均为有限列表，只提供查看/下钻；首页不提供取消、重试、命令、日志下载或编辑。
- 授权范围、更新时间、部分失败、过期、无数据、无权限和未采集保持明确且不同状态。
- `Needs Attention` 只列出当前仍需用户判断、处理或确认的 actionable issue，例如失败任务、被阻断任务、不能接收新任务的 Runtime。`Recent Activity` 只列出已发生的时间序列事实；两者可引用同一底层事件，但不得形成视觉上高度重复的消息列表。
- Refresh 是 Utility Action，刷新可信快照而非发起业务操作；Overview 不要求 Primary Business Action。

## 11.1 Diagnostic Detail Evidence Hierarchy

Diagnostic / Detail 按 **Result / Failure Fact → Reason → Evidence** 组织：稳定任务身份与状态先于失败或结果事实；原因紧随其后；不可变配置快照、脱敏命令、日志、操作记录和输出/结果事实构成证据层。

- Result / Failure 使用紧凑 semantic alert surface，突出事实而非大红卡；Reason 的标题、文字与 error code 必须明显高于普通 metadata。
- Configuration Snapshot 是只读技术事实区；Command 是独立 Evidence Surface；Logs 是高密度技术表；Operation History 与 Output Facts 为后续时间/结果证据。
- 证据可以使用标题、对齐、divider 与已批准 semantic/evidence surface 建立层级，但不 Card 化、不伪装为可编辑表单，也不使用 Wizard Skeleton。

## 12. Accessibility, Motion and Quality Gates

- 所有可操作控件有可见统一 2px focus ring；键盘顺序符合视觉和决策顺序；icon-only 控件提供 accessible name。
- Form 使用 label、`aria-invalid`、`aria-describedby`；错误摘要可聚焦并链接字段；Table sort 使用 `aria-sort`；Step Rail 使用 `aria-current="step"`。
- Drawer/Dialog 必测 open、Tab/Shift+Tab、ESC、return focus；Fixed UI 必须有 scroll padding，不遮挡最后字段或焦点。
- 不以颜色作为唯一信息；disabled/unavailable 原因不依赖 hover。动画仅用于 `120–160ms` 局部反馈，支持 `prefers-reduced-motion`，且不决定真实状态、加载完成或可点击时机。
- 验收最小矩阵：1920×1080、1440×1024、1280×720、200% zoom、键盘路径、loading/empty/error/permission/unavailable 及高风险确认路径。截图不等同于 DOM、键盘或无障碍验证。

## 13. Change Control and Delivery Boundary

- 本文件已足以约束后续模块的 Shell、页面原型、Task Builder、Inspector、Drawer、状态、tokens、响应式和反馈；不代表页面实现完成或真实功能可用。
- 改动 Product Shell、Page Archetype、Task Builder、Table model、Drawer anatomy、Status semantics、风险确认或冻结视觉源时，必须先建立 Design System Change / DCR。
- Token Calibration 只能微调已经真实验证的数值，记录视口、浏览器证据、影响组件与前后值。
- 后续页面实现必须先选择适用 Archetype，再应用本文件的通用模式；不得因实现方便重新引入统一三栏、Card-heavy 或 Wizard-all-the-things 骨架。
- 开发任一前端页面前，必须依次阅读本文件、对应 `design-system/pages/<page>.md`（存在时）及适用的 P3 产品/API/安全契约；不得从历史页面 CSS、已删除 Baseline 或旧截图反推规范。
