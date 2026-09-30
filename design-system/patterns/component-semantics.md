# Component Semantics Pattern

组件按用户意图选择，而不是按外观选择。Ant Design Vue 是唯一 Generic UI Component System；Product Feature Component 只能承载领域行为、证据组织或安全边界。

| 用户意图 | 组件 |
| --- | --- |
| 固定值单选 | `Select` / `Radio` |
| 多选 | `Checkbox` / `Select multiple` |
| 大量候选值搜索选择 | Searchable `Select` / `AutoComplete` |
| 命令集合 | `Dropdown` + `Menu` |
| 行级更多操作 | More `Dropdown` |
| 同级内容切换 | `Tabs` |
| 线性任务流程 | Ant `Steps` 或 Product `OrchTaskStepRail` |
| 保持当前页面上下文的编辑 | `Drawer` |
| 必须阻断当前流程的确认 | `Modal` |
| 简短危险确认 | `Popconfirm` 或 `OrchDangerConfirm` |
| 短状态 | `Badge` |
| 分类属性 | `Tag` |
| 需要解释的异常 | `Alert` |
| 执行进度 | `Progress` |

禁止：

- `Select` 用于执行命令。
- `Dropdown` 用于输入值。
- `Tag` 代替所有状态。
- `Drawer` 代替完整页面。
- `Modal` 承担复杂长期编辑。
- 自建基础 Button/Input/Form/Table 状态。

Product Component 允许存在的条件：

- 它组合 Ant 原生组件。
- 它表达领域行为，例如服务端资格、脏表单保护、游标分页、证据层级、危险确认。
- 它不重新绘制 Ant 的 hover、focus、disabled、validation、open、motion。
