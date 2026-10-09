# 前端业务架构治理第一阶段（2026-10-09）

## 结果与边界

已实际修复任务详情的路由复用、迟到读取/流事件、重复刷新链和过期写响应导航，按真实职责整理为 `web/src/workbench/tasks/`。任务详情模板和 scoped 样式与修改前逐字一致；列表/失败展示实现和原测试断言只迁移位置。路由、API、权限、命令/日志脱敏与 Ant/Token 保持原契约。

本阶段不修改执行节点、存储凭据、数据源、模板、首页、导入、日志、设置或权限业务代码；不改后端。任务中心只更新展示工具的导入路径。工作区原有导出、目录性能、后端及文案改动保留，本次没有提交、推送、替换服务或执行真实数据库/工具。

## 已有规范及执行缺口

| 入口 | 已有要求 / 本阶段判断 |
| --- | --- |
| 根 `AGENTS.md` | §7.2 已规定业务与生命周期不可无限放在入口，§7.3 规定 P0/P1，§7.4 明确向导唯一表单、能力所有权、失效与维护索引；全业务实施入口和维护索引不足，补充 §7.5 |
| `frontend-platform-baseline.md` | 已存在 workbench、Ant First、API、唯一状态、值依赖与 REVIEW 规范；新增全业务适用段，沿用同一权威，不重建规则体系 |
| P0 / P1 | MASTER、DESIGN、Diagnostic Detail 与数据源冻结复用规则均沿用；Task Detail P1 仍为 PAGE SPEC NOT YET READY，不以现有页面反写规范；任务中心暂无独立 P1 |
| `workbench/export/README.md` | 能力索引和失效边界可复用为索引方式；不复用导出业务参数、预检查资格或恢复语义 |
| `audit-wizards.mjs` | 已用 AST 检查向导 feature 反向依赖 views、步骤 API 值导入、循环值依赖及规模信号，3 个正反测试；不覆盖任务/节点/凭据，不等价于全前端门禁 |
| ESLint / TS / 测试 | 保留精确锁定依赖、严格 TS、Vue lint；Vitest 为 Node 环境，使用真实 Vue effectScope 验证生命周期；Playwright 用隔离 Vite 15174 与合成 API |
| verify / CI | 已接入 lint、向导检查、类型、单元与构建。全业务检查器及其 CI 推广按第四阶段收口，本阶段没有新增豁免或降低断言 |
| 任务地图 / 准入 | 本阶段为隔离前端实现与合成验证，不扩大真实 Agent、数据库、凭据或工具准入 |

## 全局审计：按严重性排序

依赖审计扫描当前 84 个非测试 TS/Vue 文件，解析静态值导入、再导出和字面量动态导入；回查源码。未发现可解析的循环值依赖。CodeGraph 索引未作为新鲜证据使用，工具不可用时沿用源码与 AST；未安装或重建索引。该检查是本阶段只读审计，不冒充第四阶段有正反测试的正式门禁。

| 严重性 | 模块 / 证据 | 影响与处置 |
| --- | --- | --- |
| 高，已修复 | `TaskDetailView.vue` 原来仅 onMounted 加载，多个回调只判断 stopped，refresh 无在途门禁 | A → B 保留 A 事实/游标；旧请求/日志/写响应可污染 B 或导航；手动刷新叠加轮询。已按独立会话、能力和连接代修复 |
| 高，阶段二 | `ExecutionNodeFormView.vue` 的 onMounted/loadNode/save；`ExecutionNodeDetailView.vue` 的 performPrimaryAction/copyEnrollmentValue；列表操作和延迟刷新 | 表单没有路由切换/卸载隔离；详情读取已有 requestSequence、注册材料已有版本与关闭清理，但生命周期操作的成功/catch/finally 和定时器没有等价隔离。保留现有协议，下一阶段按 nodes 的表单、详情事实/检查、注册材料能力治理 |
| 高，阶段三 | `StorageCredentialsView.vue` 的 submitForm/confirmDelete/refreshCredentials | 关闭、切换、成功已清空敏感表单，但没有卸载清理；异步回调和方法入口防重不足。应分清列表、编辑/轮换、删除，保留单一短时秘密输入和既有 If-Match/CSRF/幂等 |
| 中，阶段三 | `TemplateCenterView.vue` 的 refresh/loadChoices/createDraft/rename/delete | 多种读取与写事务共用页面反馈，没有卸载与迟到隔离；资格规则已复用 export，不重新实现。后续只按真实生命周期做最小拆分 |
| 中，阶段三 | `workbench/sources/` 的 7 条生产值依赖指向 `views/` | `SourceEditor` → formErrors / testDiagnostic / testNotice；`SourceWorkspace` → listFilters；`useSourceEditor` → formErrors / connectionString；`sourceGateway` → uiFixture。模块已具备列表与编辑边界，后续迁移业务工具归属并更新调用者，不推翻工作台 |
| 中，阶段四 | 现有架构自动门禁仅向导，ESLint 网络禁令仅向导入口/步骤 | 全业务反向依赖、公共组件归属与生命周期职责需正式推广；静态工具无法证明状态唯一或语义等价，仍需人工和负例测试 |
| 复核，后续必要范围 | `ConnectionTestResult` 仅 sources 调用；`ExportAdvancedSettings` / `ExportFormatChoice` / `ExportOptionHint` 仅 export；`SqlQueryEditor` 当前仅 export | 业务专属组件在 components 的归属需复核，不因已有位置即宣布跨业务复用；`SqlQueryEditor` 纯本地编辑/格式化，未发现直接网络请求。跨业务已验证的 EmptyState、DangerConfirm、OperationalTable、WizardFrame 继续复用；不新增 Wrapper |
| 保留 | TaskCenter、首页、普通导入、旁路导入、日志、系统设置、权限 | 任务中心已有单一列表/游标与 stopped 保护，当前规模及职责不需拆分；其他为简单空态/尚未接入流程，不预建 feature 空壳或新行为 |
| 保留、仅审计 | `api/browser.ts` | 同时包含 DTO、安全请求及响应白名单投影，存在多个业务的解析职责；本阶段不拆分、不绕过。新增任务端口只暴露当前 9 个方法。ProductHeader 的既有 `/logout` 请求是认证退出边界，不属于业务模块自行调用任务 API |

规模不是本轮验收条件。节点页面虽分别约 221/250/318 行，仍存在独立异步职责混杂；模板约 163 行也有生命周期缺口。向导的 4 个规模 REVIEW 仍有能力所有者，并非本轮新增失败。

## 重构前后与唯一状态证明

| 旧职责 / 入口 | 当前入口 | 状态或操作归属 |
| --- | --- | --- |
| 路由读取、装配、模板、样式 | `views/TaskDetailView.vue` | 只消费能力返回的原 ref，保留页面派生展示与 Router 协调 |
| 概览、冻结快照、命令、执行状态、刷新链 | `workbench/tasks/useTaskDetail.ts` | 唯一事实 ref；会话对象/AbortController、每资源在途 Promise、单一刷新 Promise 和计时器 |
| 日志列表、游标、错误、实时流、去重 | `workbench/tasks/useTaskLogs.ts` | 唯一日志状态；读取与流消费同一会话；消费执行 ref，不复制或自己轮询执行状态 |
| rebuild / rerun / checkpoint / template | `workbench/tasks/useTaskActions.ts` | 唯一模板输入、忙状态、操作反馈；检查点显隐只派生原执行事实，服务端复验资格 |
| 任务绑定 | `workbench/tasks/taskDetailSession.ts` | ID、9 个 API 方法的类型端口、有效性函数；没有事实/表单副本 |
| 单流管理 | `workbench/tasks/taskLogStreamLifecycle.ts` | 原监督器迁移，增加当前连接代事件资格，同步断开与关闭均失效 |
| 详情格式化 | `workbench/tasks/taskDetailPresentation.ts` | 保留原纯标签、时间与字节展示 |
| 列表标签 / 失败摘要 | `workbench/tasks/taskListPresentation.ts`、`taskFailurePresentation.ts` | 原实现及测试迁移，标签和失败提取规则不变 |

所有详情 ref 在对应能力创建一次，页面只解构引用；动作与日志只读取同一个 session / execution，没有 watcher 同步副本。共享会话是失效资格，不是第二份事实。维护入口见[任务索引](../../../web/src/workbench/tasks/README.md)。

## 实际修改文件清单

- 更新 `web/src/views/TaskDetailView.vue`；`TaskCenterView.vue` 只替换展示工具导入。
- 新增 `web/src/workbench/tasks/{taskDetailSession.ts,useTaskDetail.ts,useTaskLogs.ts,useTaskActions.ts,taskDetailPresentation.ts,taskDetailLifecycle.test.ts,README.md}`。
- 将 `web/src/views/{taskListPresentation,taskFailurePresentation,taskLogStreamLifecycle}.{ts,test.ts}` 共 6 个文件迁至 tasks；只有 stream 实现与测试追加连接代保护，原测试断言保留。
- 新增 `web/tests/task-detail.spec.ts`；原 `reference-pages.spec.ts` 本阶段没有修改。
- 更新根 `AGENTS.md`、`docs/README.md`、`frontend-platform-baseline.md`、`development-task-map.md`、`log-collection-evidence-contract.md`，新增本阶段记录。上述任务地图/日志契约采用增量追加，保留原工作区修改。

## 失效与行为验证

| 情况 | 实现与证据 |
| --- | --- |
| A → B 同实例复用 | 同步 watcher 失效旧会话，重置事实、日志/游标、输入、busy、反馈；浏览器使用来源任务链接，页头 DOM 标记保持，未通过路由 key 销毁实例掩盖问题 |
| A 请求迟到 / A → B → A | 成功、catch、finally 按捕获会话判断，不只比较 ID；单元测试分别验证初始概览、四类详情成功与失败 |
| A 日志事件迟到 / 同任务旧流迟到 | 会话资格 + 连接代双重判断；断开与关闭立即使事件资格失效，旧断开不关闭新流；保留原记录去重键 |
| 可靠游标 | 重连使用最后可靠游标，流已推进时迟到 HTTP 不回退，不对不透明游标大小做猜测 |
| 重复刷新、重试及操作 | 读取按资源复用在途 Promise；刷新只一个链/计时器；写方法入口共同 busy 守卫，派生、检查点与模板保持原 API |
| 卸载 | abort 当前 HTTP 等待，释放轮询/流；迟到读、事件和写响应不得更新状态或导航；单元与浏览器计数证据 |
| 既有操作语义 | 新建 → `/exports/new?draft=…&step=1`；从头执行 → step=5；检查点继续 → 新任务 ID；模板裁剪名称并在成功后清空输入；既有 CSRF/幂等请求头有浏览器断言 |
| 展示 / 安全 | 页面 template/style 与 HEAD 比较相同；原纯展示文件/测试比较相同；冻结证据和脱敏命令仍经原 API 投影，没有持久化秘密或新网络通道 |

## 实际检查

| 检查 | 结果 |
| --- | --- |
| 修改前 Vitest / ESLint | 20 个文件、192 项通过；lint 通过 |
| 修改后全量 Vitest | 21 个文件、217 项通过；其中任务模块 4 个文件、31 项通过 |
| ESLint / TypeScript | 全量 lint、`vue-tsc -b --noEmit` 通过 |
| 生产构建 | 通过；保留原有 >500 kB 分块提示，未调整阈值或增加拆包范围 |
| `audit:wizards` | 3 项检查器测试通过，0 violations，4 个既有 REVIEW；不能据此宣布全业务门禁通过 |
| Playwright | 系统 Chrome、隔离 `http://127.0.0.1:15174`、合成 API；新增 6 项通过，原四代表页在 1920×1080 / 1440×1024 / 1280×720 的 3 项通过，合计 9 项。浏览器插件未提供，按前端测试技能使用仓库 Playwright；未安装依赖 |
| 页面与控制台 | 详情路由/标题、非空、无 Vite 错误层、无相关 pageerror/console error、交互、DOM 状态与无整页横滚通过；截图在临时证据目录，未进入 Git |
| 源码 / 秘密 | template/style 及纯展示/原断言一致性检查通过；`check-secrets.ps1` 与 `git diff --check` 通过，LF/CRLF 提示不是失败 |
| `tokens:check`，既有失败 | 4 个 development/test 环境前景/背景 Token 漂移，`tokens.ts` / `tokens.css` 本轮未改，不生成覆盖 |
| `audit:platform`，既有失败 | 24 条无现役选择器引用、4 条 Legacy：SqlQueryEditor、Product Shell、数据源样式等既有项；本轮不更改这些文件 |

截图与成功回归保留在本机 Temp 的 `ob-task-detail-governance-regression` / `ob-task-detail-governance-evidence`。早期新测试标记放在会被 loading 分支重建的摘要 DOM，另将带 href 的 Ant Button 误按 button 定位；修正标记位置与 link 角色后通过。两项属于新增测试定位错误，没有删除或弱化原有断言。

## 已有 CI 与风险

只读确认[既有运行 37768404831](https://github.com/Dionysus-YT/ob-data-orch/actions/runs/37768404831)：提交 `026f8dc` 的 Web quality、secret scan、三个目标构建通过；Go quality 的 Unit tests 失败。失败测试涉及 `TestAgentEnrollmentAndHeartbeatOverTLS`、`TestConfigurationRejectsDigestRollbackAndCrossPlatform`、`TestRunWithContext`、`TestStorageAvailableSpaceUsesTmpPathVolume`、`TestWorker` 等后端测试，属于既有 CI 问题，本阶段未重跑或修复后端，不把本地前端通过写成整个 CI 通过。

- 仅合成 API/EventSource 验证浏览器生命周期，没有真实服务、Agent 日志、数据库、工具或检查点恢复验证。HTTP abort 不代表服务端事务已撤销。
- 未执行整个 Playwright 套件、非 Chrome 浏览器、200% 原生缩放及真实授权变更；本阶段定向三档视口与旧代表页回归通过。
- 静态审计无法证明动态计算依赖、全部业务状态和权限语义；已有平台审计失败、后端 CI 失败及后续模块生命周期问题仍保留。
- 日志保留原增长与去重语义，没有新增分页、日志容量上限、虚拟列表或结果证据规则。海量长时日志性能仍需独立评估。
- 全业务规则已在既有规范中明确，但自动门禁尚未推广；第四阶段须实现有正反测试的检查器，再接入 verify/CI。

第一阶段止于本地实现和上述合成验收，下一阶段须经用户确认后再治理执行节点。
