# Ant Design Vue Native Consolidation · 2026-09-17

本轮目标是让 Ant Design Vue 成为唯一通用 UI Component System；Orch 只保留产品领域、业务行为和 feature composition。Inventory 先于删除生成，实施后按同一清单回填结果。

## A. Inventory Result

| Component / Source | Files / references | Current responsibility | Classification | Target / result | Risk |
| --- | --- | --- | --- | --- | --- |
| `OrchField` | `SourceEditor.vue`、`ExecutionNodeFormView.vue`、`StorageCredentialsView.vue` | label、required、help/error、aria、字段 spacing | DELETE | 已删除；全部改为 Ant `FormItem`，错误使用 `validate-status` / `help`，说明使用 `extra` | 字段错误定位与业务校验必须保留 |
| `OrchInspectorDrawer` | `SourceEditor.vue`、`WizardFrame.vue`、`Qualification.vue` | Ant Drawer wrapper、标题/正文/footer、关闭按钮、焦点补齐 | DELETE / BEHAVIOR | 已删除；页面直接使用 Ant `Drawer`。dirty/testing guard 仍在 `SourceEditor` 行为函数中 | Drawer close、ESC、mask、footer、scroll 需浏览器验证 |
| `OrchStatus` | `SourceEditor.vue`、`SourceWorkspace.vue` | tone 到 Ant `Badge` 的视觉映射 | DELETE / DOMAIN | 已删除；`sourcePresentation.ts` 保留 domain tone，页面直接用 Ant `Badge` | 成功测试记录不得被提升为真实连接资格 |
| `OrchOperationalTable` | 数据源、任务、模板、凭据、节点、权限列表 | 服务端事实页表格 composition：禁用客户端分页、rowKey、selected row、empty slot、横向滚动 | FEATURE | 保留。底层仍是 Ant `Table`；不是 Button/Input 等 primitive wrapper | Ant Table hover/selection 存在 P0 冻结几何覆盖 |
| `OrchDangerConfirm` | 删除、放弃更改、命名保存等确认 | 高风险确认语义、安全初始焦点、busy 关闭保护、嵌套弹层焦点栈 | BEHAVIOR / FEATURE | 保留。底层 Ant `Modal` + `Button`；产品行为不可散落回页面 | Ant Modal footer 尺寸有 P0 例外 |
| `OrchSourceActions` | 数据源行菜单 | 行操作、服务端资格 disabled reason、键盘打开与方向键补齐 | FEATURE / BEHAVIOR | 保留。底层 Ant `Dropdown` / `Menu` / `MenuItem` | Ant Vue 4.2.6 MenuItem 方向键边界需产品补齐 |
| `OrchTaskStepRail` | `WizardFrame.vue` | 任务步骤轨，表达 workflow state | FEATURE | 保留。不是通用 Tabs/Steps primitive；后续如使用 Ant Steps 需独立设计验证 | 不得改变导入导出步骤语义 |
| `ConnectionTestResult` | `SourceEditor.vue` | 连接测试结果 feature component | DOMAIN | 保留。内部 Ant `Alert` / `Spin` | 真实连接验证语义不能由 Alert 类型推断 |
| `EmptyState` | 数据源及部分空态 | 空态文案 + Ant `Empty` composition | FEATURE | 保留。内部 Ant `Empty` / `Button` | 不是通用 Empty wrapper，不接管 Ant state |

硬性 0 引用已达成：`OrchButton`、`OrchInput`、`OrchSelect`、`OrchField`、`OrchDrawer`、`OrchTable`、`OrchAlert`、`OrchCheckbox`、`OrchCollapse`、`OrchEmpty`、`OrchInspectorDrawer`、`OrchStatus`。

## B. Architecture Result

当前边界为：Design System / MASTER → Ant `ConfigProvider` + `platformTheme` → Ant Design Vue components → Product Domain / Feature Composition → Feature Pages。

`Ant Design Vue is the sole generic UI component system.` Feature Component 不等于 UI Component Library；允许保留的组件必须表达业务事实、权限、状态机、服务端分页、确认语义、任务步骤或连接测试结果。

## C. Deleted Legacy Layer

已删除：

- `web/src/workbench/components/OrchField.vue`
- `web/src/components/OrchInspectorDrawer.vue`
- `web/src/workbench/components/OrchStatus.vue`
- `.orch-field*` 与 `.orch-field-error` 样式
- `OrchInspectorDrawer` 相关 heading/body/footer/overlay icon wrapper 样式

已迁移：

- 数据源编辑 Drawer、任务摘要 Drawer、Qualification fixture → Ant `Drawer`
- 数据源、执行节点、存储凭据表单字段 → Ant `FormItem`
- 数据源列表/编辑连接状态 → Ant `Badge`

## D. Ant Native Feedback

恢复方向：

- Input / Password / Textarea：字段红框、help、required、disabled、focus 由 Ant Input + Ant FormItem 控制。
- Select：字段 error、focus、open、disabled 由 Ant Select + Ant FormItem 控制。
- Button：loading、disabled、danger、primary、focus、wave 保持 Ant Button。
- Drawer：结构、header、footer、mask、close button、keyboard、motion、portal 交回 Ant Drawer。
- Collapse：继续使用 Ant Collapse + Ant icon，保留原生 motion 和键盘行为。
- Badge：连接测试短状态交给 Ant Badge，domain 只提供 tone / label。

## E. Product Behavior Preservation

保留且未下沉为 UI wrapper：

- Save / Test 独立语义
- Dirty Guard / Testing Guard / route leave guard
- 连接测试提交、结果呈现和 invalidated 状态
- 服务端权限、删除资格、If-Match 版本和错误回读
- Cursor Pagination，不启用 Ant Pagination
- 数据源状态机、任务状态、导入导出字段语义
- 表单业务校验规则和 API field error 映射

## F. Validation Evidence

已执行并通过：

- `npm run typecheck`
- `npm run lint`
- Vitest 18/18 文件、152/152 测试
- Playwright 24/24 浏览器回归，证据目录：`artifacts/frontend-ant-native-consolidation-final/`
- `npm run build`，生产构建通过；仍有既有主包 500 kB 提示
- `npm run tokens:check`
- `npm run audit:platform`：77 个文件、57 个 active、1761 条 CSS 声明、3 个 reduced-motion `!important`、Legacy / Duplicate / Dead 均为 0
- `npm audit`：0 vulnerabilities
- `scripts/check-secrets.ps1`
- `git diff --check`

中间失败与修复：首次 Playwright 发现 Ant Vue 4.2.6 Drawer 未暴露 dialog role/name，且常驻 Drawer 无法恢复触发按钮焦点；已用 `useAntDrawerDialog` 行为层补齐。取消 `OrchField` 后存储凭据和数据源字段 label 未自动绑定输入；已在 Ant `FormItem` 上补 `html-for`，保留 Ant 原生校验视觉。

## G. Exceptions

| Remaining item | Why retained | Long-term design or debt |
| --- | --- | --- |
| `OrchOperationalTable` | 服务端事实表格 composition，封装禁用 Ant 客户端分页、selected row、empty slot、横向滚动和授权页事实；不是输入/按钮类 primitive | 长期 Feature Composition；若 Ant Table 暴露更多 token，可减少 CSS 几何覆盖 |
| `OrchDangerConfirm` | 高风险确认的产品行为，包含安全初始焦点、busy close guard、嵌套弹层焦点栈；底层仍是 Ant Modal | 长期 Product Behavior |
| `OrchSourceActions` | 数据源行菜单的服务端资格、禁用原因和键盘补齐；底层仍是 Ant Dropdown/Menu | 长期 Feature Component，Ant Vue 4.2.6 方向键边界解除后可简化 |
| `OrchTaskStepRail` | 任务构建 workflow 状态，不是通用 Tabs/Steps wrapper；当前 P0 规定步骤轨视觉 | 长期 Feature Component，迁 Ant Steps 需 DCR |
| `useAntDrawerDialog` | Ant Vue 4.2.6 Drawer 未转发 P0 所需 ARIA，且需要关闭后恢复业务触发点；该 composable 只补 behavior/a11y，不绘制 Drawer | 长期兼容层；升级 Ant 后以浏览器回归判断能否删除 |
| `.orch-operational-table .ant-table-*` | Ant Vue 4.2.6 缺少 P0 表头高度、行高、斑马行、selected row 与 short fact padding token | 登记的 Component Exception；升级 Ant 后优先收敛到 component token |
| `.orch-confirmation .ant-modal-footer .ant-btn` | P0 确认框 footer 36px 动作高度 | 登记的 Component Exception |
| reduced-motion `!important` | 覆盖框架注入动画以满足用户系统偏好 | 长期无障碍规则 |

## H. DCR-2026-09-29 · 导出 Builder 的 Ant 步骤轨与反馈

本次用户明确要求导出页面全部采用 Ant 组件并补齐动态反馈。设计变更仅作用于 Export Builder：`WizardFrame` 在 `kind="export"` 时使用 Ant `Steps`，普通导入和旁路导入继续使用 `OrchTaskStepRail`。六步 schema、Task Builder 工作区与固定操作区不变。

已访问步骤可点击返回；当前和未来步骤不可点击，避免绕过逐步校验。已访问只表示曾进入，Ant 步骤状态保持 `wait`，不使用 `finish` 虚构完成资格；当前步骤保持 `aria-current="step"`。字段使用 Ant `Form` / `FormItem`，数据源使用 `RadioGroup`，任务事实和预检查使用 `Descriptions` / `List`，异步等待使用 Ant `Skeleton` / `Spin`。展开、加载、按钮反馈使用 Ant 原生 motion，不添加装饰动画；系统减少动态效果偏好继续由现有全局规则处理。

验证：`npm run build`、导出相关 ESLint、`git diff --check` 通过；合成 Playwright 11/11 通过，覆盖三档视口、200% 缩放、步骤返回、参数显隐、草稿复用、预检查与提交确认。未运行真实导出或真实探测。
