# OB Data Orch Design System v0.1

> 状态：已冻结
>
> 产品定位：Database Operations Workbench / 数据库运维工作台
>
> 参考页面：[数据源管理 Reference Page](ui-reference-page-data-source.md)
>
> 回归输入：[数据源 UI Regression Fixture](ui-regression-fixture-data-source.md)
>
> 冻结日期：2026-08-13

## 1. 目的与适用边界

Design System v0.1 只固化已经在真实数据源业务页面中实现并完成 1280px、1440px、1920px 桌面验证的基础规则。它为后续页面提供最小公共视觉语言，不替代业务模块文档、字段规则、API、安全契约或任务状态契约，也不是完整 UI Framework。

后续页面开发必须先区分：

- **Direct Reuse**：v0.1 已解决的问题，直接消费现有 Token、组件或模式。
- **Extension Candidate**：现有规则无法支撑、且新页面存在真实重复需求的模式；验证后才可进入后续版本。
- **Business-specific**：仅属于当前业务模块的字段、流程或状态，不进入 Design System。

本次冻结不要求存量页面立即迁移。未迁移页面中的旧视觉规则不构成 v0.1 的第二套标准。

## 2. 设计原则

设计优先级依次为信息可读性、操作效率、状态清晰度、一致性、视觉精致度和装饰性。页面层级主要通过 Typography、Spacing、Divider、Background Hierarchy 和 Status Semantics 建立。

v0.1 明确不采用：

- Card 套 Card、大面积阴影、大圆角、渐变和玻璃拟态；
- 大面积蓝色装饰、营销型 SaaS Dashboard 或通用 AI Admin 模板语言；
- 为视觉“高级感”降低 DBA / 运维人员的信息扫描效率；
- 无业务意义的插画、图形或虚构的全局能力。

普通内容区域不使用阴影。Drawer、Dialog、Popover 等 Overlay 可以使用克制阴影表达层级。

## 3. Foundations

### 3.1 Color

以下 Token 以 [web/src/style.css](../../web/src/style.css) 的 `:root` 为实现事实：

| Token | 当前值 | 用途 |
|---|---:|---|
| `--color-bg-page` | `#f6f7f8` | 页面背景 |
| `--color-bg-surface` | `#ffffff` | 表格、抽屉、控件等主 Surface |
| `--color-bg-subtle` | `#f2f4f6` | 低权重背景、Metadata |
| `--color-border-default` | `#dfe3e8` | 普通 Divider / Border |
| `--color-border-strong` | `#c8ced6` | 控件和表格外边界 |
| `--color-text-primary` | `#20242a` | 标题、主要事实 |
| `--color-text-secondary` | `#525a64` | 正文、次要事实 |
| `--color-text-tertiary` | `#747d87` | Metadata、Helper |
| `--color-primary` | `#2567b9` | Primary Action、链接、Focus、Active Indicator |
| `--color-primary-hover` | `#1d579f` | Primary Hover |
| `--color-primary-soft` | `#eaf1f8` | 低权重主色背景；不得大面积铺设 |

状态色采用 foreground / background / border 三元结构：

| 语义 | Foreground | Background | Border |
|---|---|---|---|
| Success | `#18734a` | `#edf7f1` | `#bcdcc9` |
| Warning | `#835a16` | `#fff7e8` | `#e5ce9e` |
| Danger | `#a62c24` | `#fff1ef` | `#e5b5b0` |
| Neutral | `#5e6670` | `#f1f3f5` | `#d8dde2` |

颜色不能单独承载状态含义；必须同时提供文字，必要时再提供图形和时间 Metadata。

### 3.2 Spacing

| Token | 值 |
|---|---:|
| `--space-1` | `4px` |
| `--space-2` | `8px` |
| `--space-3` | `12px` |
| `--space-4` | `16px` |
| `--space-6` | `24px` |
| `--space-8` | `32px` |

新页面优先使用这组序列。业务布局需要例外时，应说明结构原因，不能继续产生无归属的页面级 Magic Number。

### 3.3 Radius

| Token | 值 | 用途 |
|---|---:|---|
| `--radius-control` | `4px` | Button、Input、Tag 等紧凑控件 |
| `--radius-surface` | `6px` | 必要的独立 Surface |
| `--radius-overlay` | `8px` | Dialog、Popover 等 Overlay |

### 3.4 Size

| Token | 值 | 用途 |
|---|---:|---|
| `--size-control` | `34px` | Button、Input、Select 基础高度 |
| `--size-icon` | `16px` | 普通 UI Icon |
| `--size-table-header` | `38px` | 高密度表头 |
| `--size-table-row` | `52px` | 含主要信息与 Metadata 的表格行 |
| `--size-sidebar` | `216px` | Desktop Sidebar |
| `--size-global-header` | `48px` | Global Header |
| `--size-navigation-row` | `38px` | Sidebar 菜单行 |
| `--app-drawer-width` | `680px` / `640px` | `>=1440px` / `1280–1439px` Drawer |

Drawer 在更窄窗口按可用宽度收缩，并在 `<=760px` 退化为全宽；v0.1 的正式视觉验证范围仍为 1280–1920px desktop-first。

## 4. Typography

继续使用系统字体栈：`system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei UI", sans-serif`，不额外引入 Web Font。

| 层级 | Token | 字号 / 行高 | 建议字重 |
|---|---|---:|---:|
| Page Title | `--text-page-title-*` | `22px / 30px` | Semibold |
| Drawer Title | `--text-drawer-title-*` | `18px / 26px` | Semibold |
| Section Title | `--text-section-title-*` | `15px / 22px` | Semibold |
| Body | `--text-body-*` | `13px / 20px` | Regular |
| Label / Table | `--text-label-table-*` | `13px / 20px` | Medium / Regular |
| Metadata | `--text-metadata-*` | `12px / 18px` | Regular / Medium |

正式字重只有 `--font-weight-regular: 400`、`--font-weight-medium: 500`、`--font-weight-semibold: 600`。新页面不得自行引入 650、700+ 等合成字重；存量未迁移样式除外。

## 5. Product Shell 与 Layout

- Desktop Sidebar 固定 216px；普通菜单 14px / 500，Active 14px / 600，分组标题 12px / 600，Lucide Icon 16px，菜单行 38px。
- Active 使用低饱和中性背景、主文本色和左侧 2px Primary Indicator，不使用大面积蓝色圆角选中块。
- Global Header 为 48px；没有真实全局能力时不虚构 Search、Notification 或 Environment Switch。
- 页面默认以 32px 横向 Padding 和 24px 顶部节奏组织；宽数据工作台页面可使用 `wide` 内容模式，不以营销内容的窄 `max-width` 限制表格。
- 每页原则上只有一个 Primary Action。

## 6. 可直接复用的公共组件

| 组件 | 当前职责 | v0.1 复用结论 | 边界 |
|---|---|---|---|
| `WorkbenchButton` | Primary、Secondary、Text、Danger 四级按钮 | 可直接复用 | 不替代业务权限、确认和 Loading 规则 |
| `WorkbenchIconButton` | 带可访问名称的紧凑图标按钮 | 可直接复用 | 图标来自 Lucide；业务仍需提供明确 `label` |
| `WorkbenchStatus` | Runtime / Enable 的图形、文字与 Tag 外观 | 可直接复用 | Environment 不使用运行状态圆点；状态枚举仍属业务 |
| `WorkbenchFormField` | Label、Required、Field Slot、Helper、Error | 可直接复用简单字段 | 不处理 Schema、动态表单、复合 Radio/Node Picker 或保存逻辑 |

公共组件存在并不意味着所有存量页面已经迁移。新页面应优先消费这些组件，只有真实缺口才提出扩展。

## 7. 已冻结的交互与视觉模式

### 7.1 Page Header

Page Title、简短描述与单一主操作建立首层信息结构。Breadcrumb 或 Notice 没有定位和决策价值时不显示。

### 7.2 FilterToolbar

- 不套 Card；Filter Cluster 左对齐，Refresh 等 Utility Action 与筛选组分离。
- 搜索为主要入口，Select 为辅助筛选，统一 34px Control Height。
- 当前页面即时筛选，不机械增加“查询”按钮；Reset 只在存在有效条件时出现。
- 复杂筛选只在真实需求出现后扩展 More Filters / Applied Filters。

### 7.3 High-density DataTable

- 不使用外层 Card、Shadow、纵向 Grid Line 或 Zebra Stripe。
- 使用克制的 Header Background、Horizontal Divider 和 Hover。
- 表头 38px；含主信息与 Metadata 的行高 52px，不因单个长文本扩大整表行高。
- 使用固定布局和业务权重分配列宽：主要文本列承担弹性，短字段和操作列保持紧凑；禁止所有列平均吸收宽屏剩余空间，也禁止把单列异常拉宽。
- 长文本使用 Ellipsis，并以 `title` 或等价可访问方式提供完整值；高频定位字段不能因响应式策略被过早隐藏。
- 排序入口只保留给有操作价值的字段；未激活排序图标默认降噪，在 Hover / Focus 后显现。

数据源页当前 11 列是业务实现，不是全局列模板，详见 Reference Page。

### 7.4 Status Language

- **Environment** 是业务属性，使用低权重 Tag。开发、测试、预生产、生产可有轻语义色；生产不能表现为异常。
- **Runtime Status** 是运行事实，使用状态图形 + 文字 + 必要 Metadata，例如可连接、连接失败、未测试、测试已失效。
- **Enable Status** 是配置状态，使用独立 Tag；不得与 Runtime 或 Environment 混为同一套彩色 Badge。

### 7.5 Row Actions

高频动作直接显示 1–2 个，低频动作进入 `MoreHorizontal` Menu；危险动作进入 More 并经过 Confirm。图标按钮必须有稳定点击区、Hover、Focus 和可访问名称。

### 7.6 Drawer

- Header 固定、Body 独立滚动、Footer 固定；Header / Footer 使用 Divider。
- 内容通过 Section Title、Spacing 和 Divider 分层，不使用普通内容 Card 或装饰性阴影。
- 支持 ESC、Focus Trap / Return、Scroll Lock、未保存变更确认；Footer 不能遮挡最后一个字段。
- 宽度统一使用 640 / 680px 规则，不允许页面自行定义另一套 Drawer 宽度。

当前 `DataSourceEditDrawer` 仍含数据源业务逻辑；Drawer 在 v0.1 中是已验证模式，不宣称已存在完全通用的公共 Drawer 组件。

### 7.7 ConfirmDialog

支持普通确认、危险确认和未保存变更确认。危险场景说明对象与实际影响，默认 Focus 优先落到安全操作。当前实现 `DataSourceConfirmDialog` 仍有业务耦合，因此 v0.1 冻结的是模式，不是可直接跨模块导入的通用 Dialog。

### 7.8 Button

- Primary：新增、保存、创建等当前流程唯一主操作。
- Secondary：测试、重试、上一步等辅助业务操作。
- Text：取消、Reset、低优先操作。
- Danger：只用于真正危险且已明确影响的操作。

## 8. Icon System

`@lucide/vue` 是普通 UI 的唯一 Icon System。Navigation、Button、Row Action、Search、Refresh、More、Password、Disclosure 等不得使用 Unicode、Emoji、多套 Icon Library 或页面自绘风格不一致的小 SVG。品牌 Logo 与第三方品牌 Icon 不在此限制内。

存量未迁移页面仍存在 Unicode 或 CSS 字符箭头，这是已知技术债，不代表 v0.1 允许继续使用。

## 9. Accessibility 与响应式最低要求

- 交互控件必须有可见 Focus；Icon-only Button 必须有可访问名称。
- 状态不只依赖颜色；表格排序需提供 `aria-sort`；Dialog / Drawer 需有正确角色和焦点管理。
- 1280px 下保留业务必要列，并优先通过列宽、Ellipsis 和操作收缩消除无意义横向滚动。
- 1440px 是主设计基准；1920px 利用宽屏但保持紧凑列宽节奏。
- v0.1 不把 Table 改成 Card Grid；小于桌面验证范围的完整移动体验留待真实需求。

## 10. v0.1 未纳入的能力

Wizard、Stepper、Tabs、Pagination、Timeline、Tree、Log Viewer、Code Editor、Chart、Dashboard、Command Preview、Object Selector、通用 Empty State、通用 Pagination 和移动端 Product Shell 尚未完成同等级业务验证，不进入 v0.1。存量代码中即使存在类似实现，也不能据此视为已冻结模式。

## 11. 已知实现例外与迁移债务

- 全局 `style.css` 仍保留旧页面使用的 `.button`、Card、Wizard、Tree 等样式，其中包含 5px、17px、650 等旧值；它们不属于 v0.1，新页面不得复制。
- `DataSourceConfirmDialog` 的标题与按钮高度仍有局部实现值；在第二个真实页面需要复用前，不提前扩展大型 Overlay Framework。
- Environment Tag 和部分数据源状态色仍在业务页面内定义；其语义规则已冻结，是否抽取公共组件等待第二个实际消费者。
- DataTable、FilterToolbar、Drawer、ConfirmDialog、RowActions 当前是模式，不应在文档中误报为全部已经公共组件化。

## 12. 变更与版本规则

Reference Page 后续修改必须先归类：

1. **Business Change**：只改变数据源业务字段或流程，不自动改变 Design System。
2. **Bug Fix**：恢复既有功能、交互、安全或可访问性契约，不改变冻结视觉语言。
3. **Design System Change**：改变 Token、公共组件或已冻结模式。

Design System Change 实施前必须说明当前规则为何不足、影响哪些页面、是否需要同步迁移，以及是否升级版本。新页面不能为了局部实现方便直接改 Reference Page。只有至少一个真实业务场景验证通过的新模式，才进入 v0.2 或后续版本。
