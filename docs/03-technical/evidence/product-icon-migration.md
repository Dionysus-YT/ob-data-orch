# Current Icon Inventory · 2026-09-17

此表在修改引用前生成，记录迁移前来源、使用上下文/语义及拟用 Ant 图标。所有替换默认 Outlined。

| Source | Current icon | Usage context / semantic meaning | Proposed Ant replacement |
| --- | --- | --- | --- |
| `web/src/components/OrchInspectorDrawer.vue` | `X` | 关闭抽屉或导航 | `CloseOutlined` |
| `web/src/components/ProductHeader.vue` | `CircleHelp` | 产品帮助 | `QuestionCircleOutlined` |
| `web/src/components/ProductHeader.vue` | `Menu` | 打开导航 | `MenuOutlined` |
| `web/src/components/ProductHeader.vue` | `UserRound` | 退出登录操作 | `LogoutOutlined` |
| `web/src/components/ProductHeader.vue` | `X` | 关闭抽屉或导航 | `CloseOutlined` |
| `web/src/components/ProductShell.vue` | `ArrowDownToLine` | 数据导入导航，不表示浏览器上传 | `ImportOutlined` |
| `web/src/components/ProductShell.vue` | `ArrowUpFromLine` | 导出任务导航 | `ExportOutlined` |
| `web/src/components/ProductShell.vue` | `Database` | 数据库与数据源 | `DatabaseOutlined` |
| `web/src/components/ProductShell.vue` | `House` | 首页导航 | `HomeOutlined` |
| `web/src/components/ProductShell.vue` | `KeyRound` | 存储凭据导航 | `KeyOutlined` |
| `web/src/components/ProductShell.vue` | `LayoutTemplate` | 模板中心导航 | `ProfileOutlined` |
| `web/src/components/ProductShell.vue` | `ListTodo` | 任务列表导航 | `UnorderedListOutlined` |
| `web/src/components/ProductShell.vue` | `ScrollText` | 日志导航 | `FileTextOutlined` |
| `web/src/components/ProductShell.vue` | `Server` | 执行节点导航 | `ClusterOutlined` |
| `web/src/components/ProductShell.vue` | `Settings` | 系统设置导航 | `SettingOutlined` |
| `web/src/components/ProductShell.vue` | `Zap` | 旁路导入导航 | `ThunderboltOutlined` |
| `web/src/views/StorageCredentialsView.vue` | `Plus` | 新增对象 | `PlusOutlined` |
| `web/src/views/StorageCredentialsView.vue` | `RefreshCw` | 刷新当前事实 | `ReloadOutlined` |
| `web/src/views/StorageCredentialsView.vue` | `RotateCw` | 轮换凭据，不是重新读取列表 | `SyncOutlined` |
| `web/src/views/StorageCredentialsView.vue` | `Trash2` | 删除对象 | `DeleteOutlined` |
| `web/src/views/TemplateCenterView.vue` | `CopyPlus` | 用模板创建新草稿，不是复制到剪贴板 | `FileAddOutlined` |
| `web/src/views/TemplateCenterView.vue` | `Pencil` | 修改模板名称 | `EditOutlined` |
| `web/src/views/TemplateCenterView.vue` | `RefreshCw` | 刷新当前事实 | `ReloadOutlined` |
| `web/src/views/TemplateCenterView.vue` | `Trash2` | 删除对象 | `DeleteOutlined` |
| `web/src/workbench/sources/OrchSourceActions.vue` | `Ellipsis` | 更多行操作 | `MoreOutlined` |
| `web/src/workbench/sources/SourceEditor.vue` | `ChevronDown` | 展开菜单或折叠区域 | `DownOutlined` |
| `web/src/workbench/sources/SourceEditor.vue` | `ChevronRight` | 下一页或收起区域指示 | `RightOutlined` |
| `web/src/workbench/sources/SourceEditor.vue` | `Database` | 数据库与数据源 | `DatabaseOutlined` |
| `web/src/workbench/sources/SourceEditor.vue` | `RefreshCw` | 刷新当前事实 | `ReloadOutlined` |
| `web/src/workbench/sources/SourceEditor.vue` | `Unplug` | 连接测试 | `ApiOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `ChevronDown` | 展开菜单或折叠区域 | `DownOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `ChevronLeft` | 上一页 | `LeftOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `ChevronRight` | 下一页或收起区域指示 | `RightOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `Database` | 数据库与数据源 | `DatabaseOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `FilterX` | 清除全部筛选 | `ClearOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `Plus` | 新增对象 | `PlusOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `RefreshCw` | 刷新当前事实 | `ReloadOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `Search` | 搜索数据源 | `SearchOutlined` |
| `web/src/workbench/sources/SourceWorkspace.vue` | `Unplug` | 连接测试 | `ApiOutlined` |

| 其他来源 | 当前实现 | 使用语义 | 处置 |
| --- | --- | --- | --- |
| ProductShell.vue `navGroups` / `<component :is="item.icon">` | Lucide 动态组件引用 | 主导航 | 映射表全部切换为显式导入的 Ant Outlined 组件 |
| sourcePresentation.ts / OrchStatus.vue | `check / unknown / none` 字符串 | 旧兼容状态属性，无动态组件渲染 | 保留状态接口；现役 Badge 自带状态圆点，不依赖 Lucide |
| AccessControlView.vue | ⌑ / ⓘ | 权限空态 / 信息提示 | LockOutlined / InfoCircleOutlined |
| DirectLoadWizardView.vue | ⚠ | 能力边界警告 | ExclamationCircleOutlined |
| LogCenterView.vue / TaskDetailView.vue | ▤ / □ | 日志空态 | FileTextOutlined |
| TaskCenterView.vue | □ | 任务列表空态 | UnorderedListOutlined |
| SystemSettingsView.vue | ⚙ | 设置空态 | SettingOutlined |
| ExportWizardView.vue + archetypes.css | `summary::before` 的 ▸ / ▾ | 导出配置树展开状态 | RightOutlined / DownOutlined；保留原 details 状态和键盘行为 |
| sources.css / SourceEditor.vue | SVG stroke、颜色选择器 | Lucide 描边与色彩 | 删除 stroke 规则；颜色通过继承及既有语义 token 表达 |
| 显式 Ant Icon import | 0 | 无 | 7.0.1 当前由 Ant Design Vue 4.2.6 间接提供，迁移时锁定直接依赖 |
| 自定义 SVG / SVG 资源 | 0 | 无产品专属图形 | 无需保留自定义图标 |

产品品牌为文本 OB Data Orch；步骤编号、必填星号、分隔符、代码/文案中的箭头不是图标，不做语义错误的替换。Ant 组件内部的默认图标不在替换范围。

尺寸规则采用两个现有 Canonical 值：inline 为 control.fontSize（14px）；button、toolbar、navigation、status 为 foundation.space.4（16px）。统一在产品组件样式设置，页面不再传 size/stroke-width；颜色继承文字/Ant danger 状态，导航沿用现有导航色 token。

## Implementation Result · 2026-09-17

A. Lucide → Ant mapping：39 个 Lucide 显式引用已按产品语义迁移到 Ant Outlined 组件。主要映射包括新增 `PlusOutlined`、刷新 `ReloadOutlined`、数据源 `DatabaseOutlined`、连接测试 `ApiOutlined`、更多 `MoreOutlined`、编辑 `EditOutlined`、删除 `DeleteOutlined`、搜索 `SearchOutlined`、清除筛选 `ClearOutlined`、分页 `LeftOutlined` / `RightOutlined`、展开 `DownOutlined` / `RightOutlined`、导航首页 `HomeOutlined`、导出 `ExportOutlined`、导入 `ImportOutlined`、旁路导入 `ThunderboltOutlined`、执行节点 `ClusterOutlined`、模板 `ProfileOutlined`、任务 `UnorderedListOutlined`、日志 `FileTextOutlined`、凭据 `KeyOutlined`、设置 `SettingOutlined`、轮换凭据 `SyncOutlined`、基于模板新建草稿 `FileAddOutlined`、退出登录 `LogoutOutlined`。

B. 删除的 Lucide imports：`OrchInspectorDrawer.vue`、`ProductHeader.vue`、`ProductShell.vue`、`StorageCredentialsView.vue`、`TemplateCenterView.vue`、`OrchSourceActions.vue`、`SourceEditor.vue`、`SourceWorkspace.vue` 的 `@lucide/vue` import 已全部删除。`web/src/**`、`web/package.json` 和 `web/package-lock.json` 中 `@lucide` / `lucide-` / Lucide stroke 依赖扫描为 0。

C. 保留的 custom icons：无自定义 SVG 保留。旧符号图标已迁移：权限 `LockOutlined` / `InfoCircleOutlined`、旁路导入警告 `ExclamationCircleOutlined`、日志/任务空态 `FileTextOutlined` / `UnorderedListOutlined`、设置 `SettingOutlined`、导出配置树 `RightOutlined` / `DownOutlined`。Ant 组件内部默认图标、产品品牌文本、步骤编号、必填星号、分隔符和代码/文案箭头保留原语义。

D. icon size/token rule：显式产品图标统一带 `product-icon`；装饰图标带 `aria-hidden="true"`，交互名称由按钮、菜单项或链接提供。inline icon 使用 14px control font size；button、toolbar、navigation、status、empty mark 使用 16px foundation space token。颜色继承当前文本、导航、Ant danger 或现有语义 token；页面不得写随机尺寸、裸色、`stroke-width` 或单图标 margin/vertical-align。

E. affected pages：Product Header、Product Shell、Inspector Drawer、数据源列表、数据源编辑 Drawer、数据源行菜单、导出向导、模板中心、存储凭据、任务中心、任务详情、日志中心、系统设置、权限配置与旁路导入。Ant Button、Menu、Collapse、Input prefix 和导航动态组件均已按 Ant Icon 入口对齐。

F. test evidence：`npm run typecheck`、`npm run lint`、Vitest 18/18 文件 152/152 测试、原完整 Playwright 23/23、图标专项 Playwright 1/1、`npm run build`、`npm run tokens:check`、`npm run audit:platform`、`npm audit`、秘密扫描和 diff 检查通过。图标专项截图位于 `artifacts/frontend-ant-icons-acceptance/icons-产品图标统一尺寸、装饰语义与菜单对齐，窄屏导航可操作-chrome/ant-product-icons.png`。

G. remaining exceptions：Ant Design Vue 组件内部默认图标不额外处理；原生 `details` 的展开状态仍由浏览器管理，但导出树的可见展开符已改为 Ant Icon；无真实数据库、真实凭据或官方工具验证。开发期仍存在既有 `ResizeObserver loop completed with undelivered notifications` 警告，生产构建仍有既有主包 500 kB 提示，均不属于图标迁移引入。
