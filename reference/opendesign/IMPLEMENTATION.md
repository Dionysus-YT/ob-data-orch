# OB Data Orch Codex 实现交接

状态：开发迁移说明  
基线日期：2026-08-26

## 1. 目标与边界

正式实现目标是现有项目的 `web/` 前端，而不是本目录中的原型包装工程。

- 本目录：设计交付、可运行预览和验收依据。
- 真实项目参考位置：`E:\workespace\ob-data-orch\web`。
- 当前会话只把真实项目作为只读参考；开始开发前必须确认目标分支和写权限。
- 不改后端、API、权限、安全契约和任务状态机，除非用户单独授权。

## 2. 事实来源优先级

1. 真实项目当前 API、TypeScript 类型、路由和测试；
2. `ob-data-orch-workbench-prototype.html` 的已确认交互；
3. `DESIGN.md` 的视觉与组件规范；
4. `frontend-style-plan.md` 的设计理由和开放问题。

原型中的合成数据、延迟和模拟操作不能迁入生产 API。规划文档与代码不一致时，先验证代码和接口，再更新文档。

## 3. 当前工程状态

Design Files 中的 Vue 工程已将原型实现为真实 Vue SFC：

- 使用 `App.vue`、共享状态/摘要组件和分视图 SFC 渲染产品壳与工作台；
- 使用安全本地 Fixture 演示筛选、排序、抽屉、确认、六步预检查和详情链路；
- 不使用 `v-html`、原型 HTML 读取器或动态脚本挂载；
- 不连接真实 API、凭据、数据库、Agent 或官方工具，不能替代生产前端验证。

该实现解决的是设计交付包的可运行 Vue 预览边界。正式迁移到真实项目时，仍必须保留真实路由、`browserApi()`、Workbench 组件、权限和后端契约。

真实项目已经存在：

- Vue 3、TypeScript、Vite、Vue Router、Lucide Vue；
- `ProductShell`、`WorkbenchButton`、`WorkbenchIconButton`、`WorkbenchStatus`、`WorkbenchFormField`、`EmptyState`；
- `operator-signal.css` 及部分 Operator Signal 迁移；
- 数据源、导出向导、任务、执行节点和日志的真实 API 链路。

真实项目工作树当前已有未提交改动，包含 API、ProductShell、按钮/状态组件、导出向导、任务页面和 `operator-signal.css`。Codex 必须先读取 diff，禁止 reset、checkout 或覆盖这些改动。

## 4. 路由与原型映射

| 原型区域 | 生产路由 | 当前生产页面 | 实现策略 |
|---|---|---|---|
| 运行概览 | `/` | `HomeView.vue` | 按原型补齐摘要、活动、待处理和最近任务 |
| 数据源管理 | `/data-sources` | `DataSourceListView.vue` | 保留真实列表、筛选、排序和抽屉链路 |
| 数据源新增/编辑 | `/data-sources/:id` | `DataSourceFormView.vue` / `DataSourceEditDrawer.vue` | 复用表单与连接测试，不复制原型状态机 |
| 新建导出 | `/exports/new` | `ExportWizardView.vue` | 沿用真实草稿、预览、预检查和提交 API |
| 任务中心 | `/tasks` | `TaskCenterView.vue` | 接入共享状态摘要和响应式任务列表 |
| 任务详情 | `/tasks/:id` | `TaskDetailView.vue` | 保留日志流、取消、派生和检查点资格 |
| 执行节点 | `/nodes` | `ExecutionNodeView.vue` | 全宽列表，状态分层与调度门禁一致 |
| 节点详情 | `/nodes/:id` | `ExecutionNodeDetailView.vue` | 独立详情页；概览、任务、事件、审计分区 |
| 节点注册/编辑 | `/nodes/new`、`/nodes/:id/edit` | `ExecutionNodeFormView.vue` | 保留注册、平台、目录和版本校验 |
| 日志中心 | `/logs` | `LogCenterView.vue` | 使用真实日志/事件 API，不模拟远程读取 |

普通导入、旁路导入、模板、系统设置和凭据不在本轮重设计核心范围。不得因原型侧栏出现入口而顺带重构。

## 5. 公共组件迁移

优先扩展现有组件，不建立第二套 `ui/` 系统。

| 组件 | 动作 |
|---|---|
| `ProductShell.vue` | 对齐 224px 侧栏、52px 顶栏、1600px 宽内容区和移动导航 |
| `WorkbenchButton.vue` | 统一主/次/文本/危险、Busy、禁用和焦点状态 |
| `WorkbenchStatus.vue` | 保留文字标签，支持语义圆点和状态映射 |
| `WorkbenchFormField.vue` | 继续承载 label、hint、error 和 `aria-describedby` |
| `EmptyState.vue` | 区分首次空、筛选空和加载失败，不负责请求逻辑 |
| `StatusSummary.vue` | 新增；任务、数据源、节点和日志共用 |
| `StatusProgress.vue` | 新增；统一进度计算和 ARIA |
| `RefreshButton.vue` | 新增或组合式封装；同步按钮和结果区 `aria-busy` |
| `DataTableState.vue` | 新增；统一加载、空结果、错误和重试插槽 |
| `PageHeader.vue` | 可新增；固定标题、说明和单一主操作布局 |

不要复制原型中的 `statusSummaryMarkup()` 或 `setRefreshState()` 到每个页面；应转换为 Vue 组件或 composable。

## 6. 数据与 API 规则

- 所有请求通过现有 `browserApi()`；页面不得新增裸 `fetch`。
- 复用 `browser.ts` 中的类型和错误转换函数，不复制一份“原型类型”。
- 数据源连接、启停、删除/归档以服务端结果为准。
- 导出向导继续使用现有草稿、命令预览、固定预检查和提交链路。
- 租户兼容模式敏感参数必须同时落实在字段门控、残留值清理、提交校验和命令预览四层。
- 执行节点候选必须使用后端返回的调度资格；页面可解释原因，不自行放宽门禁。
- 任务详情中的恢复、取消和检查点继续资格必须使用现有 API 和证据字段。
- 日志中心不读取任意文件系统路径，不提供终端或 Shell。

## 7. 推荐实施顺序

### 阶段 0：保护当前工作树

1. 查看 `git status` 和目标文件 diff。
2. 记录现有未提交改动的所有者和意图。
3. 只在确认的目标分支继续，不清理用户改动。

### 阶段 1：基础系统

1. 以 `operator-signal.css` 为唯一 Token 入口，对齐 `DESIGN.md` 字体与派生色。
2. 收敛 ProductShell、按钮、状态、表单和表格基础状态。
3. 建立共享状态摘要、刷新和表格状态组件。

### 阶段 2：运营入口

1. 任务中心。
2. 执行节点列表与独立详情。
3. 日志中心。
4. 验证共享组件真正复用，没有页面专用副本。

### 阶段 3：数据源

保留真实筛选、排序、抽屉、连接测试、启停和删除/归档流程，只替换视觉层级与响应式表达。

### 阶段 4：导出向导

在现有 `ExportWizardView.vue` 和 `ExportWizardFrame.vue` 上迁移参照稿结构。禁止从原型重新实现 API 状态机。

### 阶段 5：概览与任务详情

补齐首页扫描价值，统一任务详情运行中、成功、失败的事实层级。

## 8. 测试映射

- 纯映射、筛选、排序、参数校验和资格判断：Vitest 单元测试。
- 组件 Busy、错误、空态、键盘操作：组件或浏览器级测试。
- API 请求头、revision、幂等键和错误转换：保留并扩展 `browser.test.ts`。
- 导出参数：扩展 `exportDraftInput.test.ts`，覆盖 MySQL/Oracle 切换残留值。
- 任务派生与运行模式：保留 `taskRunModeLabel`、失败展示和日志生命周期测试。
- 响应式与视觉：按 `ACCEPTANCE.md` 的宽度矩阵人工/自动复核。

生产前端命令：

```powershell
cd web
npm ci
npm run typecheck
npm run lint
npm run test
npm run build
```

## 9. 完成定义

- 真实路由加载 Vue 组件，不依赖原型 HTML 注入。
- 四个中心共用状态摘要、进度和刷新状态实现。
- API、权限、安全文案和后端枚举未被视觉迁移改变。
- `ACCEPTANCE.md` 的 P0 全部通过。
- 新旧样式没有相互覆盖造成的双重视觉事实。
- 测试、类型检查、Lint 和构建全部通过。
- 更新 README 和实现文档，说明完成范围与剩余 TODO。

## 10. 开放问题

- TODO：确认真实项目的目标分支及当前未提交改动是否属于同一迁移任务。
- TODO：确认 920px 以下是否作为生产正式支持范围；当前交付按响应式 Web 验收。
- TODO：确认首页指标和节点事件的真实 API 是否已经满足原型信息结构；不足时保留诚实空态，不编造数据。
- TODO：确认导出对象多选契约。当前后端仍应以实际可提交类型为准。
