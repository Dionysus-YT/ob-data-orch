# Tables Pattern

Table 是管理页主要事实载体。使用 Ant `Table`，Product 层只负责列模型、服务端游标、选择事实、证据呈现和必要几何适配。

## 目标

- scanability。
- column priority。
- medium-high density。
- alignment。
- numeric consistency。
- status clarity。
- action hierarchy。

## 布局规则

- 一张白色工作面承载表格；不默认包大型 Card。
- 管理表格 Header 为 `36px`，row 基线 `40-44px`；多行事实可自然增高。
- 表头和内容按事实分配宽度，不平均列宽。
- 短状态列可居中；身份、名称、路径和描述类列左对齐。
- 服务端 cursor pagination 不伪造 total page。

## 长值

Endpoint、Tenant、Cluster、URL、NFS Path、Backup Path、Object Name、Job ID 等必须保证：

```css
min-inline-size: 0;
overflow-wrap: anywhere;
```

使用 ellipsis 时必须有 Tooltip、Popover、Copy 或 Detail。

## Ant selector 例外

Ant Vue 4.2.6 暂无表头高度、行高、斑马行和分区 padding 的完整 token API，所以 `.orch-operational-table .ant-table-*` 是已登记产品几何适配。它不得扩展为 Button/Input/Form 的状态重绘。

## 向导预检查结果表

2026-10-19 按用户要求将现役共同呈现登记为规范：导出 `ExportReviewStep.vue` 与普通导入 `NormalImportReviewStep.vue` 使用原生 Ant Table，保留现役 `size="small"` 配置与 `44px` 预检查表头。这一结果表用于展示检查项、状态、检查结果与操作，信息密度不同于管理列表，不强制套用 `.orch-operational-table` 的 36px 表头。

- 两模块相同性质的表头、状态、行操作与留白保持一致；结果与原因允许多行，不能压成固定 42px 行。
- 保留各自领域检查项、提交资格、失败与未知事实；不得为了表格一致合并业务清单。
- 44px 是预检查结果表的例外，`component.table.headerHeight` 继续为管理表格的 36px，不修改其含义或强制两类表格使用同一几何。
- 数值依据 2026-10-17 浏览器实测；本次规范更新不代表重新执行三档截图或真实业务验证。
