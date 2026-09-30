# OB Data Orch · Global Design System Master v4

> 状态：**P0 Product IA / Shell / Archetype Source of Truth**
>
> 版本：4.0 · 2026-09-17
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
└─ 平台设置：系统设置、存储凭据、访问控制
```

Global Navigation 只负责产品模块切换。Task Step Rail、日志预设、对象筛选、设置分区和行选择均为 Contextual UI，不得升级为全局导航。

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
| Dialog / AlertDialog | 确认、冲突、未保存离开、不可逆或高风险动作 | 说明对象、影响、理由与确定动作；不承载普通编辑 |
| Fixed Action Footer | 长流程或需要持续主操作的页面 | 正文预留安全底距；不用于普通 CRUD 与详情页 |

## 5. Page Archetype System

任何页面只能选择一个主要 Archetype；可组合 Contextual UI，但不得把所有页面机械做成三栏、表格 + 表单并排或 Wizard。

| Archetype | Skeleton | Header / Toolbar | Secondary content | Drawer / Inspector / Footer | 使用场景 |
| --- | --- | --- | --- | --- | --- |
| Overview | 运营摘要面 → 单一趋势 → 节点/任务/Attention/Activity | 授权范围、freshness、Refresh utility | 有限下钻列表 | 默认无 Inspector；无固定 Footer | 首页 |
| Management / Table | Header → FilterToolbar → Dense Table → pagination/state | Create + Search/Filter/Refresh | 行级 metadata 与 eligibility | 编辑/测试 Drawer；危险 Dialog | 数据源、模板、节点、凭据 |
| Task Builder | Task header → Step Rail → current-step workspace | saved/dirty state、Save Draft、当前步骤动作 | 当前选择的派生事实 | 条件 Inspector、Summary Drawer、Fixed Footer | 导出、普通导入、旁路导入 |
| Task Operations | Header → FilterToolbar → scope bar → Task Table | Create / More、筛选与刷新 | 状态、阶段、可靠进度 | 默认无 Inspector；状态化行操作 | 任务中心 |
| Diagnostic / Detail | 稳定对象头 → 事实分区 → 证据/日志/记录 | permitted action + Back | failure/reconciliation banner | 单对象日志内联；跨对象转日志中心 | 任务/节点详情 |
| Settings | Header → compact section nav → one section workspace | Edit/Save current section | 生效值、来源、时间、差异/影响 | 高影响确认 Dialog；无 Inspector | 系统设置、访问控制 |

Reference Page 对应关系：

| Reference | Archetype | Page Spec |
| --- | --- | --- |
| Overview | Overview | [overview.md](pages/overview.md) |
| Data Sources | Management / Table | [data-sources.md](pages/data-sources.md) |
| Export Builder | Task Builder | [export-builder.md](pages/export-builder.md) |
| Task Detail | Diagnostic / Detail | [task-detail.md](pages/task-detail.md) |

新页面先选择最接近 Reference / Archetype，再套用 [DESIGN.md](DESIGN.md) 与相关 pattern。禁止直接照 Ant Demo 页面开发。

## 6. Overview Rules

Overview 回答：需要关注什么、发生了什么变化、什么失败或被阻断、Runtime 是否健康、我的任务正在发生什么。

- 使用共享运营摘要面，不形成 independent KPI card system。
- 视觉优先级：Failure / Blocked > Runtime Health > Running Tasks > Recent Activity > Trend。
- 只保留一张有明确事实口径的趋势；不做资源曲线、预测、健康分、拓扑或多图表 Dashboard。
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

## 11. Change Control

- 改动 Product Shell、Page Archetype、Task Builder、Table model、Drawer anatomy、Status semantics、风险确认或 Reference Page 身份时，必须记录 Design System Change。
- Token calibration 只能修改已确认视觉方向中的值，记录影响组件、前后值和验证结果。
- 页面实现必须先选择 Archetype，再应用 DESIGN 与 patterns；不得因实现方便重新引入统一三栏、Card-heavy 或 Wizard-all-the-things 骨架。
- 现有实现只表示 P4 事实，不能静默覆盖 P0/P1。
