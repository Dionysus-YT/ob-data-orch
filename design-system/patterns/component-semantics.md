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
| 线性任务流程 | `WizardFrame` 内的 Ant `Steps` |
| 保持当前页面上下文的编辑 | `Drawer` |
| 必须阻断当前流程的确认 | `Modal` |
| 简短危险确认 | `Popconfirm` 或 `OrchDangerConfirm` |
| 短状态 | `Badge` |
| 主动操作的短时结果 | `Message` |
| 分类属性 | `Tag` |
| 持续说明、风险与需要解释的异常 | `Alert` |
| 执行进度 | `Progress` |

禁止：

- `Select` 用于执行命令。
- `Dropdown` 用于输入值。
- `Tag` 代替所有状态。
- `Drawer` 代替完整页面。
- `Modal` 承担复杂长期编辑。
- 自建基础 Button/Input/Form/Table 状态。

操作成功、持续说明与字段错误的选择及生命周期统一遵守 [Status and Feedback Pattern](status-feedback.md)。

Product Component 允许存在的条件：

- 它组合 Ant 原生组件。
- 它表达领域行为，例如服务端资格、脏表单保护、游标分页、证据层级、危险确认。
- 它不重新绘制 Ant 的 hover、focus、disabled、validation、open、motion。

### 已登记的领域图形例外

`workbench/nodes/NodeExecutionQuota.vue` 的 SVG 环形分段表达已用、预留、可用额度。下方原生 `button[type="button"]` 仅为只读额度提供键盘聚焦和 Ant Tooltip 触发，不执行命令、修改值或承担通用 Button；保留语义名称和可见焦点。该文件不因此豁免其他原生输入、表格或动作按钮，新增点击行为须重新选择 Ant 组件。

额度分段的 `quota-used`、`quota-reserved`、`quota-free` 来自 `nodeResourcePresentation.ts` 的领域键，模板动态生成类名；Monaco 查找区的 `find-widget`、`visible`、`replaceToggled` 由编辑器运行时生成。平台审计按限定文件识别这些引用，不能仅因模板中无完整字符串判定它们已失效。
