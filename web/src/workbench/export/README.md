# 导出向导维护索引

此文件记录实现入口与验证，不替代 [前端工程基线](../../../../docs/03-technical/frontend-platform-baseline.md#复杂向导架构与开发规范)、[导出 P1](../../../../design-system/pages/export-builder.md) 或 [业务契约](../../../../docs/02-design/export-module.md)。五步流程、参数语义和后端 API 保持既有契约，正式能力与准入仍以任务地图为准。

## 状态所有权与调用关系

`ExportWizardView → useExportWizard → 表单 / 引用数据 / 参数 / 目录 / 草稿生命周期 / 恢复`。

入口创建当前页面 API 的 AbortController、路由与组件装配。`useExportWizard` 只组合能力、初始化和导航；每个实例创建一份表单。步骤 props 使用 `exportStepModels.ts` 的显式 Pick 端口，`toRefs` 指向组合模型中的原状态，不生成副本；步骤切换不重新创建这些能力。

| 文件 | 唯一职责 / 修改入口 |
| --- | --- |
| `../../views/ExportWizardView.vue` | 页面标题、WizardFrame、步骤/摘要/确认装配、footer 操作 |
| `useExportWizard.ts` | 五步导航、当前步骤错误/完成度、能力装配、初始化和卸载协调 |
| `exportForm.ts` | 唯一业务表单、对象选择集合与派生选择；`fields` 集合登记提交配置，供失效监听使用 |
| `useExportParameters.ts` | 内容/范围/格式/租户资格、互斥提示、跨步骤清值、拆分输入及草稿构造调用 |
| `exportDraftInput.ts` | 复用既有纯规则：校验、归一化、受控 URI 及请求 DTO 构造；不拼接 Shell |
| `useExportReferences.ts` | 授权数据源、节点、凭据引用列表；各请求独立 epoch，旧响应/卸载拒绝写回 |
| `exportDataSourceEligibility.ts` | 导出候选资格；其他模块如模板中心直接引用该业务规则 |
| `useExportObjectViewport.ts` | 对象列表可见窗口、滚动与尺寸观察；仅持有视口坐标，不拥有表单或目录副本 |
| `useExportCatalog.ts` | 数据库/五类对象目录与缓存、搜索/选择交互、轮询/debounce、绑定失效及过期响应 |
| `exportFormMapping.ts` | 冻结配置回填与受控存储 URI 解析；调用方控制 hydration，不执行网络或事务 |
| `useExportDraftLifecycle.ts` | 保存/读取、命令、预检、确认与提交；表单变更失效、版本防护及防重；只由 API 执行业务请求 |
| `useExportRecovery.ts` | 当前浏览器历史项的步骤 2 选择与已保存草稿引用；会话指纹门禁，刷新重新读服务端，不恢复旧预检 |
| `exportPrecheckPresentation.ts` | 固定检查项、证据说明与失败关闭的展示规则 |
| `exportPresentation.ts` | 格式选择文案、对象分类图标、环境和状态标签；无 HTTP 与提交状态 |
| `exportWizard.css` | 搬迁前的业务布局声明，作用域只覆盖导出工作面与确认事实；复用平台 Token |
| `exportStepModels.ts` | 步骤、摘要和确认消费的明确类型端口，不持有状态 |

步骤组件为 `steps/ExportSourceStep.vue`、`ExportObjectsStep.vue`、`ExportFormatStep.vue`、`ExportOutputStep.vue`、`ExportReviewStep.vue`。`ExportSummary.vue` 只展示事实；`ExportSubmitConfirmation.vue` 负责既有确认 UI、安全取消焦点及其计时器，提交由生命周期能力管理。现有 ExportAdvancedSettings、ExportOptionHint、ExportFormatChoice、SqlQueryEditor 继续复用，不新增通用 Wrapper。

## 常见改动路径

- 新增或修订参数：先查业务契约与准入；修改表单 `fields` → 既有 `exportDraftInput` 校验/转换 → 参数资格与清值 → 对应步骤及窄端口 → DTO/互斥/隐藏残留测试。不要只加控件。
- 目录问题：查 `useExportCatalog` 的绑定、完整对象缓存、数据库截断缓存和 per-type/batch epoch；搜索不改已选集合。数据源修订、节点或库改变后旧缓存和查询失效；步骤/内容切换可复用仍适用的缓存。
- 保存/预检/提交问题：查 `useExportDraftLifecycle`。保存期间编辑保持 dirty；不可变绑定变化清除旧草稿；预检与预览必须绑定当前修订/指纹/节点；迟到响应不能重新启用提交。
- 恢复问题：查 `useExportRecovery` 与 `exportFormMapping`。普通保存草稿按历史项/会话重新读取，派生草稿仍按 `?draft=` 入口读取并沿用绑定限制；两个入口不复制表单或 API。只在 hydration 期间跳过交互清值，刷新不恢复预检。
- 视觉问题：先核对 P0/P1，再查步骤与 `exportWizard.css`；不修改全局 Shell、Token 或导入占位页来解决导出局部问题。

## 验证入口与范围

2026-10-08 的实际结果、旧断言修订、视觉对照和仍阻断的平台检查见[本轮验收证据](../../../../docs/03-technical/evidence/frontend-wizard-refactor-2026-10-08.md)。

在 `web/` 运行 `npm test`、`npm run typecheck`、`npm run lint`、`npm run audit:wizards`、`npm run build`。浏览器使用 `npm run test:browser`，隔离地址为 `http://127.0.0.1:15174`，没有控制面代理，响应均为合成数据。检查器自动化负例位于 `scripts/audit-wizards.test.mjs`。

`exportWizardState.test.ts` 覆盖共享状态、条件清值、保存中编辑、预检发起竞态、预览乱序、提交防重、引用列表竞态、目录绑定失效/卸载与会话恢复引用；原草稿输入、资格和预检展示测试随规则模块一起迁移。`tests/reference-pages.spec.ts` 覆盖五步交互、三格式/两租户、对象目录/选择、步骤及刷新恢复、草稿更新/绑定、确认、响应式和缩放；新增检查不替代原用例。

当前规模信号：参数规则、纯 DTO 校验、目录管理和草稿事务各自仍超过审计的复核参考值。它们分别持有明确的一项业务能力，没有共享表单副本和循环值依赖；不为行数机械拆分。后续新增独立异步职责或新规则组时应重新复核端口与所有权。

HTTP 取消仅作用于页面等待；不能取消已接受的服务端操作。目录失效使用 epoch 和停止轮询，没有 API 支持的逐查询服务端取消。普通刷新恢复最后保存快照；未保存的步骤 3/4 参数不保证恢复。真实数据库、真实 Agent、凭据或 OBDUMPER 均不在此次前端验证范围。

对象目录无加载条数上限；超过 100 行仅改变 DOM 渲染窗口，搜索和全选仍使用完整集合。性能与完整性回归使用单类 1 万 / 五类合计 5 万个合成对象；探针响应预算 16 MiB，目录完成回执预算 17 MiB，超限或超时失败，不能接受 truncated=true 的对象结果。实际性能与风险见[大目录验证](../../../../docs/03-technical/evidence/export-object-catalog-2026-10-08.md)。
