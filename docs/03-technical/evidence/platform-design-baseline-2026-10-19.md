# 现役前端设计规范校准 · 2026-10-19

依据用户“按现有情况更新规范”的明确要求，将导出与普通导入已共同采用的视觉规则登记到现有 P0 / Pattern / P1，并修正静态平台审计的判定范围。变更入口为 [MASTER 变更记录](../../../design-system/MASTER.md#11-change-control)、[DESIGN v4.1](../../../design-system/DESIGN.md)和[前端平台基线](../frontend-platform-baseline.md#token-与样式架构)。

本轮保留现有视觉方向、Product Shell、页面类型及业务状态。运行时代码的变更仅为从唯一源重新生成四个环境 CSS 变量；审计脚本、规范和验证用例同步更新。没有安装、替换或部署服务。

## 规范与实现的对应关系

| 原问题 | 本轮决定与依据 | 实现与规范入口 |
| --- | --- | --- |
| S02：环境 Token 源与生成 CSS 漂移 | 现役 Ant Tag 主题和环境选项直接消费 `tokens.ts`，继续采用开发绿、测试橙；运行生成器同步 CSS，保留生成漂移硬门禁 | [Token 规则](../../../design-system/tokens/README.md#环境标记)、`web/src/platform/theme.ts`、`tokens.ts` / `tokens.css` |
| S03：主工作面浅阴影与旧 P0 冲突 | 按用户当前指令登记管理、向导和设置主工作面的现有 `foundation.surfaceShadow`；内部分区保持平面，占位页空态作为兼容例外，不推广为新页面模式 | [DESIGN §6](../../../design-system/DESIGN.md#6-density-surface-and-motion)、[Drawers Pattern](../../../design-system/patterns/drawers.md#视觉)、[数据源 P1](../../../design-system/pages/data-sources.md#12-governance-record) |
| S04：预检查表头 44px 与管理表头 36px 冲突 | 两类页面承担不同阅读任务，预检查保留现役原生 Table 配置与 44px 表头；管理表格 Token 继续为 36px，多行检查结果不限制固定行高 | [Tables Pattern](../../../design-system/patterns/tables.md#向导预检查结果表)、[Task Builder Pattern](../../../design-system/patterns/task-builder.md) |
| 四条裸视觉声明 | 按文件、选择器、属性、值和空媒体作用域精确登记为 Component Exception；不建立新 Token，也不允许向其他位置复制 | [DESIGN §8](../../../design-system/DESIGN.md#8-product-css-variables)、`web/scripts/audit-platform.mjs` |
| 节点额度原生按钮 | P1 原有只读额度图形采用 SVG 与可聚焦的 Tooltip 触发器，按领域图形登记；其他原生控件、动作或事件绑定继续阻断 | [组件语义](../../../design-system/patterns/component-semantics.md#已登记的领域图形例外)、[执行节点 P1](../../../design-system/pages/execution-nodes.md) |
| Monaco / 额度动态选择器 | 限定在 SQL 编辑器、节点额度组件识别框架类名及领域动态类名，保留可追溯的运行时引用记录 | 同上组件语义规则；额度键来自 `nodeResourcePresentation.ts` |

开发和测试的四个生成变量恢复为 `#389e0d` / `#f6ffed`、`#d48806` / `#fffbe6`；源文件和现役 Ant 主题值未改。`rg -- '--ob-product-environment-' web/src` 仅命中生成声明，未发现页面直接读取这些 CSS 变量。本轮未以过期生成值改写现役主题。

## 平台审计口径与剩余项

本轮先在未修改的工作树重新执行审计，确认仍为 **59 项硬失败**，`tokens:check` 同时因四个环境变量漂移失败。更新后对原 59 项的处置如下：

| 原计数 | 新分类 | 边界 |
| --- | --- | --- |
| 54 个选择器信号中的 7 个 | 已登记运行时引用 | Monaco 查找/替换区两条选择器、额度动态分段五条选择器；按选择器计数，不是七个独立组件 |
| 其余 47 个选择器信号 | REVIEW | 静态未找到完整类名不足以证明失效；保留完整终端/JSON 清单，尚未完成运行态确认与清理 |
| 4 条视觉声明 | Component Exception | 只有登记文件、选择器、属性、原值和作用域的组合可通过；改值、跨文件或新增覆盖仍失败 |
| 1 个额度组件 Primitive 信号 | 限定领域图形例外 | 仅现役 `type="button"`、具名、无动作事件的 Tooltip 触发器；不豁免整个文件 |

47 项待复核分布如下，旧 CSS 继续作为维护候选，不升级为新页面标准，也不表示已清理：

| 文件 | 选择器条数 | 主要范围 |
| --- | --- | --- |
| `web/src/platform/archetypes.css` | 42 | 旧 link/loading、任务操作与详情卡片、tree-row、旧预检查状态/失败摘要等选择器 |
| `web/src/platform/components.css` | 1 | tree-row 内的 Ant 输入占宽适配 |
| `web/src/views/TaskCenterView.vue` | 1 | task-template-name |
| `web/src/workbench/export/exportWizard.css` | 3 | export-precheck-row 及其子元素 |

未登记裸颜色/字号、未定义 Token、第二值源、重复覆盖、未登记 important、通用原生控件与内联裸视觉值继续为硬失败。脚本先执行正反例测试；命令退出码只表达硬规则，`reviews` 和 `runtimeReferences` 独立输出。**零硬违规不能推导零维护债务、完整视觉一致或业务执行可用。**

## 本轮验证

| 检查 | 结果 |
| --- | --- |
| 修改前 `audit:platform` | 59 项失败；199 个文件、155 个现役文件、2507 条声明 |
| 修改前 `tokens:check` | 失败，四个生成环境变量漂移 |
| `npm run tokens:generate` → `npm run tokens:check` | 通过；仅四个变量值同步 |
| `npm run audit:platform` | 通过；0 硬违规、47 REVIEW、7 处运行时引用；Legacy / Duplicate / Dead 声明均为 0 |
| `audit-platform.test.mjs` | 6 组测试通过；覆盖动态引用、精确例外、改值/跨文件/媒体负例、额度按钮的动作与属性负例，以及原有硬规则 |
| `npm run lint` | 通过 |
| `npm run build` | 通过，包含 vue-tsc；保留既有 500 kB chunk 提示 |
| 文档本地引用与 `git diff --check` | 通过；11 份文档的 95 个本地文件目标存在；未将文件检查外推为所有锚点或外部链接可用 |

复现（从 `web/` 执行）：

```powershell
npm run tokens:check
npm run audit:platform
npm run lint
npm run build
```

默认审计 JSON 输出为仓库根目录下的 `artifacts/frontend-platform-audit.json`，不提交生成报告。本轮修改前后 JSON 分别保留在本机 `C:/Users/Dionysus/.codex/audit-artifacts/ob-data-orch-design-baseline-20261019/platform-before.json` 与 `platform-after.json`，未改写 2026-10-17 历史审查报告。

### 验证范围

本轮重新执行的是代码静态审计、规范检查、审计规则正反例及构建。44px 表头与共同工作面视觉依据 2026-10-17 浏览器实测及截图；本轮没有启动浏览器、重新截图或逐帧测量动效。没有重新运行完整业务单测/浏览器矩阵，没有进行真实数据库、凭据、Agent 或官方工具操作；历史验证结果不作为本轮新增通过项。
