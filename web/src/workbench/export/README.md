# 导出向导维护索引

2026-10-27 两条普通向导体验补查：底栏只保留步骤动作，移除通用说明；第四步具名字段错误由既有 Form.Item 原位反馈，`useExportWizard.currentStepAlert` 仅保留无法归属字段的步骤问题，`currentStepError` 继续用于阻断、展开和焦点定位。第五步保留唯一检查总览与逐项证据，不重复自动检查机制。专家参数用例显式选择导出内容，并使用现役节点控件及预检查入口，不依赖已取消的默认选择；验证记录见[产品体验整改](../../../../docs/03-technical/evidence/product-experience-fixes-2026-10-27.md)。

2026-10-27 Schema 策略修复：`useExportParameters` 独占 MySQL retain/native 正常交互清值，hydration 保留历史冲突供校验；`exportDraftInput` 校验互斥并派生 compactSchema=false；`ExportFormatStep` 只消费唯一表单、禁用态和提示。服务端 `exportdomain/schema_strategy.go` 复验组合，唯一生成器负责 v12 等号令牌，步骤不发请求或改写 DDL。没有增加表单副本或全局状态，已有字段继续触发配置失效。97 项受影响单测、实际恢复/切换与三项 API 拒绝反例见[前缀修复验收](../../../../docs/03-technical/evidence/export-retain-schema-2026-10-26.md)；只读历史预览与冻结任务不升级。

2026-10-26 默认日志文案由 `steps/ExportOutputStep.vue`、`steps/ExportSummary.vue` 和 `exportParameterCatalog.ts` 同步：未填路径时描述 Agent 受控采集；摘要仍只以唯一已保存草稿为事实来源。实际日志配置与私有 Java 临时目录归 Agent 的固定启动配置，不在向导新增参数、隐式日志路径、目录空性跳过或第二份表单状态。真实压缩兼容仍以任务地图的目标环境证据为准。

2026-10-24 候选顺序由控制面调度策略统一提供，`useExportReferences` 仍唯一持有节点候选和绑定读取；重排不改选、不清空仍有效绑定。读取失败须撤下旧候选，保留当前绑定标识供复读恢复，不以旧顺序代替当前策略；未选节点不能因排序或单候选自动填入。负例见 `exportWizardState.test.ts` 与 `reference-pages.spec.ts` 的候选失败重试用例。

2026-10-17 跨模块一致性整改：按用户确认完成 8 项 P1 与 5 项模块内 P2，公共呈现规则登记于 Task Builder Pattern。摘要、预检查行和只读命令复用现有平台样式；来源切换确认仍归 `useExportParameters`，错误字段由 `exportDraftInput` 返回，步骤只关联 Form.Item，入口仅负责展开后的滚动与焦点。`useExportWizard` 按配置提供完成步骤；离开守卫消费未保存状态，持久化签名包含手选节点。参数目录和摘要复用原焦点工具的开关事实保护，快速关闭不得由迟到动画回调重新锁住背景。

本轮前端 40 文件 / 521 项单元测试、构建（含类型）、lint、两项架构检查通过；47 个不同仓库 Chrome 场景分批验证，另含定向交互和稳定态补拍。三档截图采纳 79 张，前后对照、SHA256 和范围台账位于外部证据目录 `ob-data-orch-ui-consistency-fix-20261017`。数据源标准页的分页、抽屉、删除取消和控件几何已回归。平台样式审计仍为原有 59 项且集合相同，环境 Token 漂移、工作面阴影和表格几何按约定保留；不据此宣称整个平台门禁通过。未提交、部署、替换现役服务或执行真实数据库与工具验证。

此文件记录实现入口与验证，不替代 [前端工程基线](../../../../docs/03-technical/frontend-platform-baseline.md#复杂向导架构与开发规范)、[导出 P1](../../../../design-system/pages/export-builder.md) 或 [业务契约](../../../../docs/02-design/export-module.md)。五步流程与参数语义沿用既有契约；第五步交互和固定阻断证据按本轮用户确认同步修订，正式能力与准入仍以任务地图为准。

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
| `useExportReferences.ts` | 授权数据源、节点、凭据引用列表；新建时不默认选择节点，只有一个候选也由用户手选；节点下拉展开重读名称与候选，同 ID 改名保留绑定、失效 ID 清选择且不自动改派；各请求独立 epoch，旧响应/卸载拒绝写回 |
| `exportDataSourceEligibility.ts` | 导出候选资格；其他模块如模板中心直接引用该业务规则 |
| `useExportObjectViewport.ts` | 对象列表可见窗口、滚动与尺寸观察；仅持有视口坐标，不拥有表单或目录副本 |
| `useExportCatalog.ts` | 数据库/五类对象目录与缓存、搜索/选择交互、轮询/debounce、绑定失效及过期响应 |
| `exportFormMapping.ts` | 冻结配置回填与受控存储 URI 解析；调用方控制 hydration，不执行网络或事务 |
| `useExportDraftPersistence.ts` | 草稿读写、修订/dirty、会话与 URL 绑定；保存期间继续编辑只接纳对应修订、不覆盖表单或重放成功；通过回调撤销旧执行准备 |
| `useExportDraftLifecycle.ts` | 组合持久化能力并持有命令、预检、确认与提交；第五步自动预检、防重/轮询/有效期及迟到防护；步骤不执行业务请求 |
| `exportParameterCatalog.ts`、`components/ExportParameterCatalog.vue` | 当前新建适用参数、受管参数说明与步骤定位；复用平台 `WizardParameterCatalog` 表现，不拥有表单或参数资格 |
| `useExportRecovery.ts` | 当前浏览器历史项的步骤 2 选择与已保存草稿引用；会话指纹门禁，刷新重新读服务端，不恢复旧预检 |
| `exportPrecheckPresentation.ts` | 按输出类型的固定项、结果分组、阻断汇总/修正入口及证据说明的纯展示规则；不持有事务或状态 |
| `exportPresentation.ts` | 格式选择文案、对象分类图标、环境和状态标签；无 HTTP 与提交状态 |
| `exportWizard.css` | 搬迁前的业务布局声明，作用域只覆盖导出工作面与确认事实；复用平台 Token |
| `exportStepModels.ts` | 步骤、摘要和确认消费的明确类型端口，不持有状态 |

操作提示沿用 [P0 状态与反馈规范](../../../../design-system/patterns/status-feedback.md)。草稿操作确认、复制成功和复制失败由生命周期分别持有，入口通过 `useOperationMessage` 展示短时成功；步骤消费持久说明与错误，预检查结论保持结果事实。复制请求与预览的会话序号共同阻断过期回调，反馈不持有命令或 SQL 内容。

步骤组件为 `steps/ExportSourceStep.vue`、`steps/ExportObjectsStep.vue`、`steps/ExportFormatStep.vue`、`steps/ExportOutputStep.vue`、`steps/ExportReviewStep.vue`。`ExportSummary.vue` 只展示事实；`ExportSubmitConfirmation.vue` 负责既有确认 UI、安全取消焦点及其计时器，提交由生命周期能力管理。ExportOptionHint、ExportFormatChoice、SqlQueryEditor 位于本模块 `components/`，继续属于导出业务。2026-10-15 按 DEC-052，已有 ExportAdvancedSettings 的呈现移入 `components/WizardAdvancedSettings.vue`，与普通导入复用同一主题、标题与开合反馈；导出各步骤仍独占字段、配置计数与业务规则，不共享导入状态，视觉和开合行为保留。

## 常见改动路径

- 新增或修订参数：先查业务契约与准入；修改表单 `fields` → 既有 `exportDraftInput` 校验/转换 → 参数资格与清值 → 对应步骤及窄端口 → DTO/互斥/隐藏残留测试。不要只加控件。
- 2026-10-13 专家参数：四项开关、九项日期格式与 sequencePolicy 沿用上述唯一所有者。`exportFormMapping` 负责回填；`useExportParameters` 清除失去内容/范围/租户/sys 资格的值；新字段已登记配置失效。预检查新增证据只由 `exportPrecheckPresentation` 展示，步骤不持有 sys 信息或拼装命令，范围和验证见[参数接入记录](../../../../docs/03-technical/evidence/export-expert-parameters-2026-10-13.md)。
- 目录问题：查 `useExportCatalog` 的绑定、完整对象缓存、数据库截断缓存和 per-type/batch epoch；搜索不改已选集合。数据源修订、节点或库改变后旧缓存和查询失效；步骤/内容切换可复用仍适用的缓存。
- 保存/读取问题：查 `useExportDraftPersistence`。源选择即可保存未完成的 EDITING 配置，补齐后同 ID 提升为 READY；无来源任务的编辑草稿可重选引用，完整/任务派生绑定保持不可变。保存期间继续编辑保持 dirty；URL 切换即使 A → B → A 也拒绝旧响应。`useExportDraftLifecycle` 独占预览/预检/提交，只有完整干净的第五步自动准备；保存本身不探测。
- 第五步重构（2026-10-13）：自动预检等待保存、回填和预览完成；每个表单版本只自动尝试一次，失败手动重试，不重复领取。PENDING/LEASED 读取失败只暂停本页轮询，“重新读取”恢复同一检查，不创建新探测。占位 UNKNOWN 不冒充完成结果，提交还核对固定清单和有效期。分组、依赖原因及修正入口见 `exportPrecheckPresentation`；步骤将现有分组派生为四列表格，不持有第二份报告。仅摘要开合属于步骤的视图状态，完整脱敏命令常驻且不可折叠。验证见[第五步记录](../../../../docs/03-technical/evidence/export-precheck-review-2026-10-13.md)。
- 恢复问题：`?draft=` 由 `useExportDraftPersistence` 读取，普通保存和派生草稿均可恢复，只有实际 `sourceTaskId` 才触发派生绑定限制。`useExportRecovery` 保留旧历史项/会话恢复兼容；`exportFormMapping` 回填前重置同一表单以清除缺省字段残留。只在 hydration 期间跳过交互清值，刷新不恢复预检。
- 2026-10-14 真实草稿恢复修复：API 层 `browser.ts` 的严格传输解码补齐 filterConfig 读取开关、timestampFormats、compressionLevel 和 ddlBehavior；`exportFormMapping` 仍是业务回填唯一所有者。`api/exportDraftRoundtrip.test.ts` 验证 GET→PATCH 不丢活动参数及非法类型/序列枚举失败关闭，真实页面刷新参数保留见[MySQL E2E 记录](../../../../docs/03-technical/evidence/export-mysql-e2e-2026-10-14.md)。新增字段必须同时检查该传输层，不能只登记表单和失效监听。
- 视觉问题：先核对 P0/P1，再查步骤与 `exportWizard.css`；不修改全局 Shell、Token 或导入占位页来解决导出局部问题。

## 验证入口与范围

2026-10-17 导出内容手动选择修订：新建、重置与更换数据源均不预选导出内容；未选择时不启用内容相关设置、对象目录或执行准备。EDITING 草稿保存并恢复真实空值，已保存的明确选择仍保留；READY 配置继续拒绝空内容。前端 508 项单元测试、受影响的 Go 单元/契约与 vet、类型、构建、lint、向导及业务架构检查通过；5 个交互场景及 7 个针对性 Chrome 回归通过（合计 11 个不同场景），覆盖空选草稿与摘要、手选后的格式配置和恢复、节点选择及旧流程。全部使用合成 API，未替换现役控制面或进行真实数据库/工具验证。

2026-10-17 节点手动选择修订：取消首次加载的单候选预选，刷新和展开不代选；有效的手动选择、同名标识改名与草稿恢复绑定继续保留。43 项状态测试、4 个合成 Chrome 场景（单节点未选/手选、未完成草稿恢复、步骤恢复及 SQL 输入刷新恢复）、TypeScript 与受影响文件 lint 通过。数据库目录回归显式选择合成节点，不再依赖默认预选；未连接真实节点或数据库。

2026-10-17 交互与草稿同步：Go 全量单元/契约及 vet、前端 507 项单元测试、类型/构建/lint、向导及业务架构检查通过。13 个合成 Chrome 场景覆盖未完成草稿及对象存储半成品恢复、同 ID 补齐、目录搜索定位/窄屏、离开取消焦点与确认、两步节点同步/改名、旧绑定新建、刷新、提交确认及现有格式/文件组织。新用例位于 `tests/export-wizard-parity.spec.ts`，普通导入参数目录原回归继续通过；Browser plugin not available，使用仓库 Playwright 与已安装 Chrome，在隔离端口拦截全部 API。架构复核确认持久化与执行准备各有唯一所有者，两个节点步骤只复用节点模块的纯平台文案。未替换现役服务、升级运行库或进行真实数据库/工具验证；源码检查不表示已部署。

2026-10-08 的实际结果、旧断言修订、视觉对照和仍阻断的平台检查见[本轮验收证据](../../../../docs/03-technical/evidence/frontend-wizard-refactor-2026-10-08.md)。

在 `web/` 运行 `npm test`、`npm run typecheck`、`npm run lint`、`npm run audit:wizards`、`npm run build`。浏览器使用 `npm run test:browser`，隔离地址为 `http://127.0.0.1:15174`，没有控制面代理，响应均为合成数据。检查器自动化负例位于 `scripts/audit-wizards.test.mjs`。

`exportWizardState.test.ts` 覆盖共享状态、条件清值、保存中编辑、预检发起竞态、预览乱序、提交防重、引用列表竞态、目录绑定失效/卸载与会话恢复引用；原草稿输入、资格和预检展示测试随规则模块一起迁移。`tests/reference-pages.spec.ts` 覆盖五步交互、三格式/两租户、对象目录/选择、步骤及刷新恢复、草稿更新/绑定、确认、响应式和缩放；新增检查不替代原用例。

当前规模信号：参数规则、纯 DTO 校验、目录管理和草稿事务各自仍超过审计的复核参考值。它们分别持有明确的一项业务能力，没有共享表单副本和循环值依赖；不为行数机械拆分。后续新增独立异步职责或新规则组时应重新复核端口与所有权。

2026-10-13 第五步整改：`ExportReviewStep` 只消费派生检查行与常驻命令，配置摘要唯一展示入口是现有 Drawer 内的 `ExportSummary`；输出、编码、压缩及文件组织读取保存快照。`exportPrecheckPresentation` 持有七项/九项展示规则、检查边界与修正入口。`useExportDraftLifecycle` 仍独占自动检查/重试/轮询和迟到防护。API 层只负责当前与历史报告解析，不反向依赖业务清单。临时分块目录进入常用区，不再计入高级设置。

HTTP 取消仅作用于页面等待；不能取消已接受的服务端操作。目录失效使用 epoch 和停止轮询，没有 API 支持的逐查询服务端取消。普通刷新恢复最后保存快照；未保存的步骤 3/4 参数不保证恢复。真实数据库、真实 Agent、凭据或 OBDUMPER 均不在此次前端验证范围。

对象目录无加载条数上限；超过 100 行仅改变 DOM 渲染窗口，搜索和全选仍使用完整集合。性能与完整性回归使用单类 1 万 / 五类合计 5 万个合成对象；探针响应预算 16 MiB，目录完成回执预算 17 MiB，超限或超时失败，不能接受 truncated=true 的对象结果。实际性能与风险见[大目录验证](../../../../docs/03-technical/evidence/export-object-catalog-2026-10-08.md)。
