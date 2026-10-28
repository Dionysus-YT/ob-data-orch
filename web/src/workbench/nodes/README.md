# 执行节点维护索引

工程约束沿用 [AGENTS](../../../../AGENTS.md) 与[前端平台基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)，产品字段与行为查[执行节点设计](../../../../docs/02-design/execution-node-module.md)，注册协议查[Agent 契约](../../../../docs/03-technical/agent-task-state-contract.md)。此页是定位索引，不另建规范。

| 要修改的职责 | 唯一维护入口 | 状态或边界 |
| --- | --- | --- |
| 路由、表格列、布局、弹层焦点、页面导航 | `views/ExecutionNodeView.vue`、`ExecutionNodeFormView.vue`、`ExecutionNodeDetailView.vue` | 六列与注册/编辑 Drawer 见[节点 P1](../../../../design-system/pages/execution-nodes.md)；原有表单路由嵌套列表，保留筛选；页面只协调未保存导航与列表写事务，不持有业务表单副本 |
| 列表读取、自动刷新与已授权事实筛选 | `useNodeList.ts` | nodes、筛选、读取状态、错误与失效序号；唯一刷新间隔/定时器、默认 60 秒、取消旧代及卸载清理 |
| 列表操作与永久删除事务 | `useNodeListActions.ts` | 操作锁、删除目标、反馈；授权失效清除确认对象；通过列表能力发布新事实，删除成功不解释为归档 |
| 详情事实与基于修订的操作 | `useNodeDetail.ts` | node、读取、最近成功读取时间、刷新完成与操作状态；写入前使旧读取失效，操作期间拒绝刷新；没有第二个节点副本 |
| 详情运行概览与只读配置 | `NodeRuntimeOverview.vue`、`NodeEnvironmentConfiguration.vue` | 只消费唯一详情；概览保留资源/额度、授权任务统计和环境检查。执行环境分别显示物理核/逻辑核、内存总量，以及与当前任务数据目录匹配的磁盘容量；不累加磁盘、不补造未知。任务数据目录在现有工具配置内原样逐行显示，覆盖导出写入和普通导入读取；日志上传队列独立保留。技术字段及历史记录隐藏，不持有副本或请求，不设折叠区 |
| 固定环境检查结果展示 | `NodeInspectionDetails.vue`、`nodeInspectionPresentation.ts` | 概览只读完整十项，唯一文案规则提供状态、原因、处理建议和必需项摘要；未通过/未确认区分，可选异常说明影响，上次清单来源明确，不展示路径、原始码、访问布尔值或 Direct Load 参数 |
| 节点登记/编辑 | `useNodeForm.ts` | 唯一 form、已加载节点修订、dirty 指纹、错误、保存/授权状态与 DTO 映射；模式/ID 切换重新加载或重置，授权失效清配置并使旧写响应失效 |
| 表单规则与字段错误白名单 | `nodeFormRules.ts` | 字段标签/ID、Windows/Linux 路径规则与唯一校验；不替代服务端安全与路径检查 |
| 操作系统与 CPU 架构分类、平台帮助 | `nodePlatformRules.ts`、`useNodeForm.ts` | Windows/Linux 与 x86_64/aarch64 组合成三个原平台值；两个控件投影唯一 form.platform，Windows 拒绝 aarch64。`nodePlatformHelp` 从表单或当前详情的平台派生架构、路径、权限和首次安装帮助，不列发行版/系统版本、不转换已填路径；下载包名从同一分类映射派生，构建与部署规则见部署手册；切换、保存锁与会话有效性由表单所有者处理 |
| 一次性材料与复制反馈 | `useNodeEnrollment.ts` | 独立弹层代和复制代；关闭、关联完成、路由切换、卸载清引用；不写存储、日志、路由或命令 |
| 会话有效性、HTTP 取消 | `nodeSession.ts` | 仅节点模块内部共享的真实复用能力；每次绑定变更产生新会话代；A → B → A 不能恢复旧回调 |
| 状态展示与主操作资格 | `executionNodePresentation.ts` | `nodeCategoryStates` 独立映射管理/Agent/运行环境/任务槽位；通用接收资格不代表模块可用，资格筛选仍使用 API 可信投影 |
| 资源图表与额度圆环 | `NodeResourceUsage.vue`、`NodeExecutionQuota.vue`、`nodeResourcePresentation.ts` | 使用真实百分比/预算；未知不填零；仅节点业务组件，不创建通用 UI Wrapper |
| 磁盘快照补读与缓存 | `useNodeResourceSnapshots.ts` | 只保留采样键及最高使用率磁盘的百分比、已用/总量，不复制详情/路径/表单；同一份事实支持图形与悬停容量，三并发、旧代取消、修订与采样匹配、卸载清空 |
| 注册码编码与固定启动入口 | `executionNodeEnrollmentInstructions.ts` | 原 `obdo-r1` 协议；固定包下载、启动指令不携带材料 |
| 请求、安全、响应验证 | `api/browser.ts`、`api/nodeRuntimeFacts.ts`、`api/nodeResponseValidation.ts` | 原 API/CSRF/If-Match/幂等机制不变；采集解析保留严格 DTO 校验，响应错误统一补充区域、固定原因及处理建议，不依赖 workbench |

提示沿用 [P0 状态与反馈规范](../../../../design-system/patterns/status-feedback.md)。操作成功由各会话所有者发布，页面用 `useOperationMessage` 展示短时确认；错误持续使用 Ant Alert，字段归 Form.Item。启用只确认动作，任务预检查边界仍在详情说明。复制成功与失败分开持有，材料关闭或会话重置同时清除。

2026-10-16 响应兼容与错误说明：近期任务保留导出及普通导入的真实类型，未知业务、范围、状态和超量记录继续拒绝。`nodeResponseValidation` 只管理安全错误，不持有节点事实或请求；列表、详情、表单及操作复用原状态所有者。错误保留 `RESPONSE_INVALID`，细分主机、检查、版本、日志队列、任务、额度和注册材料等区域，并给出固定原因与建议；不回显响应值、路径、原始异常或材料。写请求送达后校验响应失败，提示先刷新核对可能已经生效的结果，不假报事务回滚。详情刷新保留可信旧事实，首次失败阻断展示，授权与会话失效规则不变。合成回归与现场验证限制见[本轮记录](../../../../docs/03-technical/evidence/execution-node-response-errors-2026-10-16.md)。

数据源、导出与模板的节点候选复用 `executionNodePresentation.nodePlatformLabel`，统一显示操作系统分类与 CPU 架构；名称与标识仍由各模块现有引用读取能力持有。该跨业务依赖只复用纯格式化规则，不共享列表缓存、表单、请求、身份或任务资格。

异步成功、catch、finally、导航和定时器都核验捕获的会话。独立刷新按读取代丢弃旧响应，写操作入口加锁并使之前读取失效；不因按钮 disabled 就假定方法不会重入。环境操作先延迟 2500ms 读取，在线且 pending 时继续串行复读，最多两分钟，完成/离线/失败/失效时停止；唯一计时器由 useNodeDetail 持有，自动读取不发布手动刷新成功。列表周期刷新仍独立，慢请求和写事务不叠加。HTTP abort 只是取消等待，已送达控制面的变更仍可能完成，重新进入页面重新读取事实。

注册材料不与详情事实同步复制。清除引用不等于 JavaScript 内存物理擦除，也不删除用户已复制的剪贴板内容；保持既有提醒与安全契约。

验证入口：`nodeLifecycle.test.ts` 包含路由复用、迟到、卸载、读取/写入竞态、防重、定时器和敏感材料负例；两个迁入的展示/注册测试保留原断言。浏览器：`tests/execution-node-page.spec.ts` 原回归，`tests/execution-node-lifecycle.spec.ts` 新增同实例场景。实际执行结果、文件清单与限制见[第二阶段验收记录](../../../../docs/03-technical/evidence/frontend-business-phase2-2026-10-09.md)。

2026-10-10 已接入真实预算与完整节点任务汇总：`useNodeForm.ts` 唯一表单拥有 GiB 配置输入，DTO 在保存边界转换为字节；没有第二份共享状态。`NodeExecutionQuota.vue` 只展示服务端额度，`ExecutionNodeView.vue` 根据服务端 scope 展示本人/全部统计并跳转按节点筛选的任务中心。未知保持“待核对”，不能填零。隔离夹具只供测试。

后端维护入口：`internal/executionbudget/` 统一堆与预算规则；`internal/store/execution_resource_budget.go` 和迁移 `0024` 负责原子分配与持久占用；`internal/store/node_task_summary.go` 完整授权聚合；`internal/controlplane/node_resource_budget.go`、`task_read_scope.go` 负责安全投影；`internal/agentexec/jvm_heap.go` 对冻结堆进行唯一替换；`cmd/agent/export_execution.go` 负责资源上界与共享日志队列装配。任务中心范围请求具备 AbortController、代次校验和卸载失效，游标不能跨节点或账号范围复用。

2026-10-10 列表操作改为编辑/永久删除：前端 `deleteExecutionNode` 使用独立 `POST /execution-nodes/{id}:delete`，旧控制面不支持时 404 阻断，不能退回旧 DELETE 自动归档。后端迁移 `0025` 以仅含 node_id 的不可变历史引用保留任务与执行证据，实际配置物理删除；无运行或待核对执行才允许撤销机器身份并删除。前端不复制节点状态、额度或敏感材料；GiB 随数值，资源仅显示图形，刷新时间统一在页面右下角。

验证入口：预算、幂等和过期释放见 `internal/store/execution_resource_budget_test.go`；全局授权与投影负例见 controlplane 对应测试；启动堆与队列并行见 agentexec/cmd Agent 测试；响应边界见 `src/api/browser.test.ts`；真实页面与迟到范围请求见 `tests/execution-node-resources.spec.ts`、`tests/task-node-filter.spec.ts`。真实工具并发尚未验证，服务未安装升级。

2026-10-12 详情默认运行概览，环境与配置使用第二个 Ant Tab。入口只协调 Tab、键盘、导航和注册码焦点；四类状态复用 `nodeCategoryStates`。两个只读分区不持有第二份节点事实；`NodeResourceUsage` 的 expanded 展示仅由详情选择，列表保留紧凑模式。`useNodeDetail` 统一刷新防重、上次成功时间和原位完成反馈；服务错误保留事实，401/403/404 和会话重置清空时间及事实。Tab 在对象或授权失效时重置；切换 Tab 不补读接口。浏览器入口 `tests/execution-node-detail.spec.ts` 覆盖范围、未知/零值、离线、检查中/启用、同实例 A → B → A、授权失效、键盘和响应式；注册分类回归按新内容入口读取配置。详见[本轮记录](../../../../docs/03-technical/evidence/execution-node-detail-refactor-2026-10-12.md)。

同日去重细化：`nodeAgentPlatformLabel` 只格式化上报平台显示，不决定支持或任务资格；平台相同时配置区省略，差异/未知保留可核对值。导出目录与空间明细已按后续指令删除，配置、采样与概览磁盘口径继续保留；完整采集及最后展示范围以本轮记录末节为准。

2026-10-10 注册/编辑改为列表中的右侧 Drawer，沿用原路由与 API。表单保存状态通过页面事件进入列表现有 mutation 边界，结束或卸载成对释放；编辑成功再读列表，创建成功仍进入详情关联。未保存确认覆盖关闭、节点/模式切换与路由离开，保存期间阻断离开；HTTP 取消仍不撤销服务端事务。表单不持有关联材料，授权失败清输入并阻断重交。新增合成浏览器入口 `tests/execution-node-registration.spec.ts`，包含焦点、筛选保留、字段错误/预算定位、DTO、写期间刷新/导航、授权撤销和响应式；原生命周期测试继续覆盖同实例读取失效，迟到写与卸载负例继续在 `nodeLifecycle.test.ts` 验证。实际结果见[本轮记录](../../../../docs/03-technical/evidence/execution-node-registration-2026-10-10.md)。


2026-10-12 诊断链与完整采集：两个只读分区不复制事实或发起请求；`NodeInspectionDetails` 只消费当前详情清单。`api/nodeRuntimeFacts.ts` 分出系统/完整检查/记录的纯传输解析，旧服务/Agent 缺项保持未知。诊断负例入口 `src/api/nodeDiagnostics.test.ts`、`nodeRuntimeFacts.test.ts`；检查有界复读、慢请求、防重、离线和失效继续由 nodeLifecycle.test.ts 验证。采集继续保留，当前可见字段按下段最新要求收敛。

后端维护入口：`internal/nodefacts/` 是严格心跳和存储复用的有界类型/校验；`agenttelemetry/versions.go` 只读固化安装元数据；`agentexecution.Worker.ActiveExecution` 是唯一活动位置所有者；`agentlogqueue.Queue.Stats` 是唯一可靠日志队列统计所有者，cmd 只组装并转发。`store/node_execution_observations.go` 核对数据库绑定和任务权限，`controlplane/node_diagnostics.go` 统一详情预算、统计及诊断投影；新增采集沿用 facts_json，不建遥测历史。版本变化使检查失效，动态队列及序号不使检查失效。合成验证与真实部署边界见本轮证据更新。

完整采集维护入口：`agenttelemetry/host*.go` 固定原生系统接口；`agentenvironment/inspection.go` 唯一十项检查与 Direct Load 安全投影；`agentexec/version_probe.go` 唯一固定版本进程边界；`store/node_inspection_audit.go` 保存每次检查许可快照，agents 持续授权与 store 完成事务共同核验清单；`store/node_activity.go` 有界读取既有节点审计。近期任务复用原 TaskListQuery 权限，不另建节点任务源。新增协议两端须协调升级，本次没有安装服务、真实工具或数据库验证。

2026-10-16 自动复验：持续授权随新关联材料或存量节点明确授权写入当前机器身份，不从旧检查追认；身份失效后拒绝旧许可结果。手动/自动复验保留管理状态，已启用节点通过后恢复新任务资格。`nodeCanRequestEnvironmentCheck` 统一主动复验入口，`useNodeDetail` 唯一持有主动写事务和当前自动检查的两分钟串行复读；路由失效停止，自动读取不发成功提示。`UNCONFIRMED` 由已有状态投影区分待授权/待核验，真实失败继续异常。见[验证及升级边界](../../../../docs/03-technical/evidence/execution-node-auto-inspection-2026-10-16.md)。

2026-10-12 最新详情精简：返回与下载统一对象头右上角；不折叠，环境检查移至概览，日志上传队列移至配置。配置以执行环境、工具与内存、上传摘要呈现；隐藏身份、哈希、租约、协议、网段、CPU 型号、原始结果及检查绑定等技术字段。移除活动执行、近期任务、节点事件、Direct Load 摘要、检查记录和最近操作，授权任务统计及底层采集继续保留。配置版本优先实际检查，旧检查与元数据回退均显式标注；不重复概览资源/槽位。浏览器入口同时验证数据存在时隐藏、缺失与真实零、Tab 边界和无折叠。

平台布局唯一入口：`internal/agentlocalpreflight.ToolLauncherPath` 只允许 OBDUMPER/OBLOADER 固定选择器；Windows 取安装根 `bin/windows/*.bat`，Linux 取 `bin/*`。固定环境检查和旧 OBDUMPER 预检查共用，不把脚本目录当安装根。路径与错层负例见 `tool_layout_test.go`、`agentenvironment/inspection_test.go`；详见[最新验证记录](../../../../docs/03-technical/evidence/execution-node-detail-refactor-2026-10-12.md#核心详情精简与平台路径修正)。

同日环境报错解释优化：`inspectionItemGuidance` 根据固定检查项/结果码生成原因与处理建议，未知码不回显、不猜成功；`inspectionBlockingSummary` 仅汇总必需项，`inspectionEmptyMessage` 区分等待、失败缺明细和旧检查缺明细。`nodeUnavailableReasonLabel` 只在当前异常清单与检查标识匹配时补充摘要，列表仍用简短安全提示；解释不改接收资格、状态机或请求。负例覆盖可选项、旧清单/过期事实、未知码及版本未查/超时区别。浏览器在真实页面用合成回执检查原因、建议、缺项、窄屏和 200% 展示；见[报错优化记录](../../../../docs/03-technical/evidence/execution-node-detail-refactor-2026-10-12.md#环境检查报错解释优化)。

2026-10-19 节点目录语义：nodeFormRules 与 nodePlatformRules 统一字段名、错误和平台帮助，编辑器/详情显示“任务数据目录”，覆盖导出写入和普通导入读取；allowedRoots 及平台路径权限均不变。页签键盘导航提取为 composables/antTabKeyboard.ts，节点和任务详情只共享无状态交互，不共享业务事实。
