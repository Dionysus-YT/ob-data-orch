# 任务模块维护索引

2026-10-16 普通导入执行切片：任务中心已开放普通导入创建、类型筛选与服务端控制的取消资格；新建跳转独立五步向导。旁路仍禁用，普通导入不取得导出派生/模板/检查点资格。布局不变，筛选与创建仍由原唯一所有者管理。

此目录管理任务详情的读取、日志和操作生命周期，以及任务列表/详情的纯展示规则。路由、页面模板、Ant 组件和 scoped 布局仍在 `views/`；不创建公共业务引擎，不复制 API 的授权、脱敏或 DTO 白名单规则。

## 修改入口与唯一所有者

| 职责 / 原位置 | 当前入口 | 所有者与依赖 |
| --- | --- | --- |
| 详情装配 / `TaskDetailView.vue` | `../../views/TaskDetailView.vue` | 对象头、五个 Ant Tab、query 与导航，消费同一组事实 ref |
| 概览、快照、命令、执行状态、两秒核对 / 详情页脚本 | `useTaskDetail.ts` | 每个组件实例唯一事实状态；每次 ID 变化建立独立会话及 AbortController；冻结证据成功读取后不反复请求 |
| 任务绑定与失效资格 | `taskDetailSession.ts` | 只有 ID、API 端口和有效性判定；不包含事实或表单副本；会话对象区分 A → B → A |
| 日志补读、游标、去重、SSE / 详情页脚本 | `useTaskLogs.ts` | 唯一日志列表、可靠游标与日志错误；消费同一会话及执行事实，不自己读取执行状态 |
| 单流关闭与连接代隔离 / `views/taskLogStreamLifecycle.ts` | `taskLogStreamLifecycle.ts` | 每个实例一个流句柄；旧断开与记录回调不能影响新连接；支持打开过程中同步断开 |
| 列表及详情操作、确认、模板输入 | `useTaskListActions.ts` + `TaskActionDialogs.vue` | 共用服务端资格与单一写事务；通知与导航使用不同结果类型；对象失效后不提示或导航。旧 `useTaskActions.ts` 只保留既有测试兼容，不再由页面使用 |
| 状态、时间及字节展示 | `taskStatePresentation.ts` + `taskDetailPresentation.ts` | 列表与详情共用状态/时间；后者只在页面提供字节展示，旧状态函数不再由页面使用 |
| 失败摘要 / `views/taskFailurePresentation.ts` | `taskFailurePresentation.ts` | 只提取已经脱敏的失败日志，不推断根因 |
| 列表标签、行主操作与 More 菜单顺序 | `taskListPresentation.ts` | 只渲染服务端 `availableActions`，不按状态推断资格；未知状态附核对标识 |
| 八种状态文案、环境、耗时 | `taskStatePresentation.ts` | 列表与详情共用的唯一文案来源 |
| 列表筛选、游标、自动刷新 / 原 `TaskCenterView.vue` 脚本 | `useTaskList.ts` + `taskListQuery.ts` | 路由 query 是筛选与条数唯一来源；条件签名变化使旧读取失效；游标栈按条件缓存于模块内；非终态且页面可见时每 10 秒读取；写事务期间暂停 |
| 列表行写操作 | `useTaskListActions.ts` | 一次一个写事务；确认后执行；结束无论成败复读列表；卸载后迟到响应不导航不提示 |
| 派生导航目标 | `taskDerivationRoutes.ts` | 详情与列表共用，不各自拼路由 |
| 筛选视图 | `TaskListFilters.vue` | 纯视图，只发出修改意图；关键字 300 ms 防抖；摘要移除按钮可键盘操作 |
| 详情操作记录 | `useTaskOperations.ts` | 当前任务会话的唯一分页事实；页内及跨页去重，旧响应不恢复记录或 finally |
| 草稿列表、分页与删除 | `useTaskDraftList.ts` + `TaskDraftList.vue` | 条件来自路由；只读取安全列表投影，写事务防重，旧确认/反馈/读取隔离，不持有完整配置 |
| 任务操作菜单 | `TaskActionMenu.vue` | 任务/草稿/新建动作的领域菜单；Ant 控件承担呈现，补齐方向键、Escape、焦点与迟到打开隔离；未准入导入类型固定禁用 |
| 详情纯规则 | `taskDetailRules.ts` | 主操作只从服务端资格排序；只识别明确 ERROR；历史快照缺口不补造 |

## 异步边界

- ID 同步 watcher 使旧会话立即失效，清空全部详情事实、日志、游标、操作输入与反馈，再读取新任务。HTTP 使用原 `browserApi(signal)`，不修改 CSRF、幂等或响应投影。
- 所有成功、错误、finally、导航和流回调都验证捕获的会话。流事件额外验证连接代；关闭、断开、重连或任务终态使旧事件资格失效。
- 同一会话的刷新调用与单项读取复用在途 Promise；手动刷新取消原等待计时器，刷新完成只安排一个下一轮计时器。冻结快照及命令成功后保持原缓存语义，状态和日志继续核对。
- HTTP 日志补读只能在当前游标仍等于请求发起时游标时更新它，不比较不透明游标大小。流已推进游标时，迟到 HTTP 不能把它倒退。
- 卸载释放 HTTP 等待、轮询和流。取消浏览器等待不撤销服务端已接受的写操作；过期写响应不强制导航、提示成功或清空新任务输入。
- 所有操作按钮消费服务端 `availableActions`；检查点继续仍由服务端验证失败、检查点、原绑定与未完成核对。草稿派生仍进入步骤 1 / 5，检查点继续导航新任务 ID。
- 详情非终态且可见时两秒核对，终态停止；只读授权失效清空整个会话，手动重试创建新会话。普通日志过期清空内容和游标并关闭流，区别于采集错误。

## 测试与维护

- `taskDetailLifecycle.test.ts`：同实例 ID 切换、A → B → A、迟到成功/错误、卸载、单一刷新链、日志去重/游标/连接代、操作防重及迟到导航。
- `taskLogStreamLifecycle.test.ts`：断开重连、陈旧断开、同步断开及旧事件资格。
- `taskListPresentation.test.ts`：状态文案、资格菜单、历史名称与耗时；`taskFailurePresentation.test.ts` 保持原断言。
- `taskListLifecycle.test.ts`：路由解析、条件切换迟到响应、游标栈与 A → B → A、授权失效停止刷新、终态停刷、写事务失效、行操作防重与卸载。
- `../../../tests/task-center.spec.ts`：筛选进 URL 并恢复、摘要移除、服务端资格菜单、取消确认与写请求、已删除名称、空态、三档视口。
- `../../../tests/task-detail.spec.ts`：真实路由装配，来源链接复用组件、合成事件、迟到 HTTP、终态/卸载、派生写请求头与导航、模板、两档视口。
- `../../../tests/reference-pages.spec.ts`：原四代表页、冻结证据、响应式与控制台回归。

追加功能前先判断是模板展示、纯展示规则、详情事实、日志生命周期还是写操作。不要将新的操作状态复制到页面，也不要让日志能力接管派生操作或让页面直接发送网络请求。工程规则见[前端平台基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)。
