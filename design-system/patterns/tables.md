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
- Header 约 `36px`，row 基线 `40-44px`；多行事实可自然增高。
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
