# OB Data Orch · Frontend UI Design & Implementation Contract

> 状态：**P0 Visual / Implementation Contract**
>
> 版本：4.0 · 2026-09-17
>
> 适用范围：OB Data Orch 前端视觉语言、token、Ant Theme、Product CSS Variables、组件语义与页面实现约束。

## 1. 定位与阅读顺序

`DESIGN.md` 回答“产品整体长什么样、为什么这样设计、怎么落到 Ant Design Vue 与 Product CSS”。`MASTER.md` 只负责 Product IA、Global Shell、Page Archetype、Navigation 和重大结构；Pattern 文件负责复用 UI 的实现规则；Page Spec 只记录页面或 archetype 的收窄规则。

开发任何前端页面前按以下顺序阅读：

```text
design-system/MASTER.md
design-system/DESIGN.md
design-system/patterns/<relevant>.md
design-system/pages/<page>.md（存在时）
适用 P3 产品 / API / 安全 / 验证契约
```

实现权威链路固定为：

```text
Design Direction
  ↓
DESIGN.md
  ↓
Design Tokens
  ↓
Ant Design Vue Theme Tokens
  + Product Layout CSS Variables
  ↓
Patterns / Reference Pages
  ↓
Feature Pages
```

## 2. UI/UX Pro Max Mapping

采用：

- **Minimalism & Swiss Style**：clean、functional、grid-based、precise hierarchy、restrained color、strong typography hierarchy。
- **Data-Dense Dashboard**：只补充中高信息密度、Table scanability、runtime visibility、diagnostic information 和 comparison。
- **Adobe Spectrum principle**：Precise、Dense、Legible、Strong focus states、Professional。
- **B2B Service palette**：neutral first，蓝色只承担交互和重要状态锚点。
- **Chinese Simplified typography**：中文 UI 优先，技术 identifier 使用等宽字体。

修改：

- 不照搬 UI/UX Pro Max 的组件、模板、marketing composition 或英文品牌字体。
- 不把 Pro Max 的 `Primary` 字段机械映射为大面积蓝色背景；`#0B6BFF` 只作为 Action / Link / Focus / Selected / Processing。
- 不引入 Spectrum CSS 或组件库；Generic UI 继续由 Ant Design Vue 负责。
- 不做 ultra-dense dashboard；本产品密度为 medium-high，中文正文必须保持可读。

拒绝：

- Glassmorphism、Neumorphism、Aurora、Glow、AI 蓝紫渐变、Bento everywhere、Card wall、Dark IDE skin、Gold banking theme、Huge hero typography、decorative motion。
- 传统 Ant Demo 后台的默认拼装感。
- 营销型 SaaS Dashboard 与银行品牌门户视觉。

## 3. Final Design Profile

| 项目 | 结论 |
| --- | --- |
| Product Type | Enterprise Database Operations Workspace |
| Primary Style | Minimalism & Swiss Style |
| Secondary Style | Data-Dense Dashboard |
| Professional Principle | Precise / Dense / Legible |
| Color | OCP-inspired light blue-gray operational canvas + restrained enterprise blue |
| Typography | Chinese Simplified |
| UI Font | `"Noto Sans SC", "PingFang SC", "Microsoft YaHei", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif` |
| Technical Font | `"JetBrains Mono", "Cascadia Mono", Consolas, "SFMono-Regular", ui-monospace, monospace` |
| Component System | Ant Design Vue |
| Icons | `@ant-design/icons-vue`；操作图标采用 Outlined，导航按已确认 OCP 风格优先 Filled，无对应实心图标时保留 Outlined |
| Visual Mode | Light-first |
| Density | Medium-high |
| Motion | Low |
| Decoration | Very low |
| Qualities | Precise, Calm, Professional, Technical, Dense but legible, Trustworthy |

## 4. Color Contract

核心语义：

| Semantic | Value | Usage |
| --- | --- | --- |
| Text Strong | `#132039` | 页面标题、主要事实、重要标签 |
| Foreground | `#132039` | 表单与 Ant 基础正文 |
| Text Secondary | `#5F6B7A` | 说明、metadata、低权重事实 |
| Page BG | `#F3F6FC` | 全局页面底色 |
| Surface | `#FFFFFF` | 工作面、Drawer、弹层 |
| Muted Surface | `#F8FAFD` | 辅助面、轻量背景 |
| Border | `#E5EAF2` | hairline、表格线、分区线 |
| Action Primary | `#0B6BFF` | Primary action、link、selected、focus、processing |
| Destructive | `#DC2626` | danger action、error semantic |

蓝色只用于 Primary Action、Selected、Link、Focus、Processing 和重要交互状态。禁止大面积蓝色背景、蓝色 card wall、全部 icon 蓝色、蓝紫渐变、glow 和 aurora。

Success / Warning / Error 继续通过 Ant semantic token 表达；业务页面不得建立第二套状态色，也不得把环境、资格和连接结果混成同一个 badge。

## 5. Typography Contract

中文 UI 使用 sans，技术 identifier 使用 mono。不得为了“技术感”把普通中文说明、整张表格或整块 Drawer 改成 mono。

推荐尺度：

| Role | Size / Line / Weight |
| --- | --- |
| Page Title | `24 / 32 / 600` |
| Section Title | `16 / 24 / 600` |
| Subsection | `14 / 22 / 500` |
| Body | `14 / 22 / 400` |
| Control | `14 / 20-22 / 400` |
| Table | `13-14 / 20-22` |
| Metadata | `12 / 18-20` |

中文默认 `letter-spacing: normal`。禁止为中文标题、按钮、表格或导航使用 `-0.02em`、`-0.04em` 等拉丁字形负字距。

技术等宽字体只用于 SQL、IP、Endpoint、Tenant、Cluster、SCN、Job ID、Plan UID、文件路径、命令、日志字段和错误证据。

## 6. Density, Surface and Motion

默认密度：

| Role | Value |
| --- | --- |
| Page Padding | `20-24px`，当前产品壳使用 `24px` |
| Section Gap | `16 / 24 / 32px` |
| Control Height | `36px` |
| Table Header | `36px` |
| Table Row | `40-44px`，当前基线 `42px` |
| Body | `14px` |
| Metadata | `12px` |

页面层级优先使用 Page Background、Surface、Spacing、Typography 和 Hairline Border。Card 只用于真正独立的信息单元；Table 不默认包大型 Card；普通页面分区不使用阴影。阴影只允许真实浮层：Drawer、Modal、Dropdown、Popover、Tooltip。

Radius：control `4-6px`，surface `6-8px`。禁止 `16px+` 圆角、pill everywhere 和装饰性大圆角。

Motion：Low Motion。优先 Ant 原生 motion；允许 Drawer、Collapse、Dropdown、Button wave 和 180-240ms 的 opacity / border / background 反馈。禁止 hover scale、全页 fade、staggered card、floating animation、parallax 和 glow pulse。

## 7. Ant Ownership

Ant Design Vue 是唯一 Generic UI Component System。以下状态由 Ant 原生负责：

```text
hover / focus / focus-visible / active / loading / disabled /
validation / open / motion / wave
```

禁止通过 Feature CSS 重绘 `OrchButton hover`、`OrchInput focus`、`OrchForm error` 等第二套基础状态。页面如需调整 Ant 组件视觉，优先使用：

```text
ConfigProvider
Ant Global Token
Ant Component Token
```

长期 `.ant-*` selector 只能作为已登记例外：Ant Vue 4.2.6 暂无 token API 的表格几何、Form 标签/帮助布局、确认弹层 footer、产品布局宽度和 reduced-motion 覆盖。升级 Ant 后优先删除例外。

## 8. Product CSS Variables

CSS Variables 只负责 Ant 不应知道的产品布局、密度与证据结构：

```text
--ob-product-shell-navigation-width
--ob-product-shell-header-height
--ob-product-shell-workspace-padding
--ob-product-task-rail-width
--ob-product-inspector-width
--ob-component-table-row-height
--ob-component-table-header-height
```

禁止新增：

```text
--orch-button-hover
--orch-input-error
--orch-form-focus
--orch-select-open
```

Design Token taxonomy 见 [tokens/README.md](tokens/README.md)。实现唯一值源为 [tokens.ts](../web/src/platform/tokens.ts)。

## 9. Component Semantics

组件按用户意图选择，而不是按外观选择。基础映射：

| Intent | Component |
| --- | --- |
| 固定值单选 | Select / Radio |
| 多选 | Checkbox / Select multiple |
| 大量候选值搜索选择 | Searchable Select / AutoComplete |
| 命令集合 | Dropdown Menu |
| 行级更多操作 | More Dropdown |
| 同级内容切换 | Tabs |
| 线性任务流程 | Steps / Product Task Step Rail |
| 保持当前页面上下文的编辑 | Drawer |
| 必须阻断当前流程的确认 | Modal |
| 简短危险确认 | Popconfirm / Product Danger Confirm |
| 短状态 | Badge |
| 分类属性 | Tag |
| 需要解释的异常 | Alert |
| 执行进度 | Progress |

详细规则见 [component-semantics.md](patterns/component-semantics.md)。

## 10. Action Hierarchy

一个操作上下文通常只有一个 Primary Action。Primary、Default / Secondary、Text / Tertiary、Danger、Icon Action 必须可区分。

示例：

- Page Toolbar：`新建数据源` 是 Primary，`刷新` 是 Utility。
- Drawer Footer：`保存` 是 Primary，`取消` 是 Default / Text。
- Save 与 Test 是独立业务动作，不自动合并成 `Save & Test`，除非 P3 明确授权 Composite Action。

详细规则见 [action-hierarchy.md](patterns/action-hierarchy.md)。

## 11. Long Technical Token Rule

长技术值必须可缩、可换行、可查看完整值。覆盖 Endpoint、Tenant、Cluster、URL、NFS Path、Backup Path、SQL ID、Plan UID、Job ID、Object Name、Error Message。

实现要求：

```css
min-inline-size: 0;
overflow-wrap: anywhere;
```

使用 ellipsis 时必须提供完整值访问路径：Tooltip、Popover、Copy 或 Detail。禁止长值撑爆 Grid，禁止 flex child 因缺少 `min-width: 0` 而无法 shrink。

## 12. Accessibility and Feedback

Accessibility 是 P0：

- visible focus、keyboard navigation、contrast、semantic label、ARIA、form association、Drawer / Modal focus safety。
- 状态不能只靠 red / green / yellow，必须有 text、icon 或 label 辅助表达。
- icon-only action 必须有 accessible name。

异步反馈：

- Click 后立即有状态变化。
- Async 有 loading。
- Loading 阶段按需要防止重复提交。
- Success 有明确确认。
- Failure 有可行动解释。
- Retryable 提供恢复动作。

## 13. Reference Pages

四个 Reference Page 继续作为视觉和 pattern 验证对象：

| Reference | Archetype |
| --- | --- |
| Overview | Overview |
| Data Sources | Management / Table |
| Export Builder | Task Builder |
| Task Detail | Diagnostic / Detail |

新页面必须先选择最接近的 Reference / Archetype，再使用对应 pattern。禁止直接照 Ant Demo 页面开发。
