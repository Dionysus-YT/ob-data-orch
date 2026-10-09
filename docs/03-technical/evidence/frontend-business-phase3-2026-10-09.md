# 前端业务架构治理第三阶段：凭据与必要模块

日期：2026-10-09。基线 `cf8dc89`。用户授权推进全部剩余阶段并推送；本阶段实际治理存储凭据、模板及已确认的业务归属，不改后端、API、路由、Token、布局或业务能力。规范复用 AGENTS、前端基线、P0/P1、任务地图及准入规则。

## 职责对照与实际文件

| 原职责 | 新维护入口 / 实际修改 |
| --- | --- |
| 凭据页面同时持有列表、编辑、删除和事务 | `web/src/views/StorageCredentialsView.vue` 只装配；新增 `workbench/credentials/useCredentialList.ts`、`useCredentialEditor.ts`、`useCredentialDeletion.ts` |
| 凭据纯校验及筛选放在 views | `storageCredentialList.ts` 与同名测试原样迁入 `workbench/credentials/`，保持唯一规则 |
| 模板页面同时管理目录、改名/删除、引用缓存、派生及导航 | `web/src/views/TemplateCenterView.vue` 只装配和导航；新增 `workbench/templates/useTemplateCatalog.ts`、`useTemplateActions.ts`、`useTemplateDraft.ts`；数据源资格直接复用导出规则 |
| sources 反向引用 views 工具 | 六组 `dataSourceConnectionString`、`dataSourceConnectionTestDiagnostic`、`dataSourceConnectionTestNotice`、`dataSourceFormErrors`、`dataSourceListFilters`、`dataSourceUiFixture` 的 `.ts/.test.ts` 迁入 `workbench/sources/`；规则和原断言保持，只调整导入路径 |
| 单业务组件误放公共 components | `ConnectionTestResult.vue` 迁到 `workbench/sources/`；`ExportAdvancedSettings.vue`、`ExportFormatChoice.vue`、`ExportOptionHint.vue`、`SqlQueryEditor.vue` 迁到 `workbench/export/components/`。仅 SQL 编辑器 Monaco CSS 相对路径因目录深度调整 |
| 路径调用方 | 更新 sources 的 `SourceEditor.vue`、`SourceWorkspace.vue`、`sourceGateway.ts/.test.ts`、`sourcePresentation.test.ts`、`useSourceEditor.ts`；export 的三个步骤和 `exportWizardState.test.ts`；浏览器 `fixtures/facts.ts`、`reference-pages.spec.ts`、`source-contracts.spec.ts` |
| 新增行为验证 | `credentials/credentialLifecycle.test.ts`、`templates/templateLifecycle.test.ts`、`web/tests/business-lifecycle.spec.ts` |
| 索引及事实记录 | 新增 credentials/templates/sources 的 README 和本记录；更新 export README、AGENTS、前端基线、文档中心、任务地图、凭据安全契约。数据源 P1 与 UI 夹具文档仅修正源码链接 |

[凭据索引](../../../web/src/workbench/credentials/README.md)、[模板索引](../../../web/src/workbench/templates/README.md)、[数据源索引](../../../web/src/workbench/sources/README.md)、[导出索引](../../../web/src/workbench/export/README.md)列出按修改目标定位的入口。两个页面的样式与展示结构不变，template 仅将取消/删除赋值改成对应能力的意图方法。没有通用 Wrapper、事件总线、全局状态框架或空模块。

首页、任务列表、普通导入、旁路导入、日志、设置和权限等简单/未开发页面保持原业务范围；复杂功能的未来约束已落实到 AGENTS 和既有基线。`api/browser.ts` 只审计职责，本轮无修改。

## 状态、异步与安全

- 凭据列表由 `useCredentialList` 唯一持有，创建/轮换表单只在 editor 中创建一次，删除确认由 deletion 持有；页面解构原 ref，不维护同步表单或双向 watcher。editor 的目标仅保存打开时非敏感标识及修订，防止刷新静默升级 If-Match。
- 列表每次读取有代号，成功、失败和 finally 均核验；写入先失效旧读取、持有唯一事务锁。各方法防重，刷新及其他写入不能在事务中重复创建。关闭编辑器使迟到结果失效，原事务直到结束才释放锁。
- 列表 401/403 清授权事实及编辑输入；后续临时失败保持阻断，仅成功授权读取恢复输入。普通临时失败保留可信列表及当前失败表单，沿用已有重试行为。
- AccessKey/SecretKey 仅属于当前编辑表单和请求栈；关闭、目标切换、成功、卸载清引用，请求 finally 清材料引用。秘密没有进入列表、持久化、路由、日志或测试输出。测试只使用合成材料，截图仅捕获清理后的状态。
- 模板列表只有 catalog 持有；actions 管理改名/删除会话，draft 管理两类引用及所选 ID。共享方法锁隔离写入和旧读取；取消会话令迟到反馈失效。引用单一在途 Promise、两类结果原子发布，空成功可缓存，部分失败不缓存并允许重试。
- 派生草稿捕获源/节点 ID，缺失时不请求，防重并保留原 API。创建后只有活跃页面才能导航；模板资格、重新预检查要求及安全投影保持。
- 每个页面会话持有一个 AbortController，卸载先失效再取消；迟到成功、错误、finally、导航不能回写。未新增轮询、订阅或计时器。HTTP 取消不能撤销已到服务端的事务，返回页面重读；清字符串引用不等于物理擦除。

## 实际验证

锁定依赖、Windows、隔离 Vite `127.0.0.1:15174` 与系统 Chrome。全部请求、材料、数据及错误为合成夹具，无真实 Agent、凭据探测、数据库或官方工具操作。专用浏览器插件不可用，采用仓库现有 Playwright。

| 检查 | 结果 |
| --- | --- |
| Vitest 全量 | 24 文件、255 项通过；新增生命周期 19 项（凭据 10、模板 9），既有规则断言保持 |
| ESLint / TypeScript | `npm run lint`、`npm run typecheck` 通过 |
| 构建 | `npm run build` 通过，包含全量 vue-tsc；既有 >500 kB 分块提示保留 |
| Playwright 主回归 | `business-lifecycle.spec.ts`、`remaining-pages.spec.ts`、`source-contracts.spec.ts` 共 19 项通过（新增 4）；覆盖创建/轮换/删除、CSRF/幂等/修订、秘密清理、防重、迟到刷新/导航、模板绑定及冻结数据源视觉/交互 |
| 导出迁移补验 | `reference-pages.spec.ts` 的单选/摘要、SQL 刷新恢复、本地编辑操作、高级设置显隐共 4 项通过；组件迁移未改变语义 |
| 架构门禁 | 第四阶段的 business 10 个正反测试、wizard 原 3 个测试通过；0 硬违规，5 个规模信号逐项复核 |
| 秘密及差异 | `scripts/check-secrets.ps1`、`git diff --check` 通过；不提交本地数据或测试产物 |

最终成功浏览器证据保留于本机 Temp 的 `ob-business-governance-accepted` 与 `ob-business-export-accepted`，失败调试 trace 留在其他本轮 Temp 输出目录。初轮新测试遇到迁移路径、模板夹具缺字段及 Ant Select/Button 可访问名称/键盘定位错误，修正测试夹具和定位后完整复验通过；没有删除、跳过或弱化已有断言。授权失败后临时故障的负例复核补入原生命周期测试。

## 既有失败与未覆盖风险

本轮实际重跑 `tokens:check` 仍因 4 个环境色漂移失败；`audit:platform` 仍为 24 条未引用选择器和 4 条 Legacy，相关 Token/样式未修改。数据源高级设置回归运行中 Vite 报一次 `ResizeObserver loop completed with undelivered notifications`，对应测试通过；没有据此声称浏览器零告警。

基线 [CI 37872855590](https://github.com/Dionysus-YT/ob-data-orch/actions/runs/37872855590) 的 Web quality、秘密扫描及三个目标构建通过，Go quality 的 Unit tests 失败、Vet 未执行。与此前阶段同属已有后端失败范围，本轮不修改 Go，也未执行整体 verify 或声称全仓门禁通过。

未执行完整 Playwright 套件、其他浏览器、原生 200% 缩放、真实存储创建/轮换/删除、权限变更或真实模板任务。合成验证证明前端状态与请求契约，不提升真实功能准入、部署或运行服务状态。服务端已接收的写入不能被页面关闭撤销。
