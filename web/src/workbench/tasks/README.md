# 任务模块维护索引

此目录管理任务详情的读取、日志和操作生命周期，以及任务列表/详情的纯展示规则。路由、页面模板、Ant 组件和 scoped 布局仍在 `views/`；不创建公共业务引擎，不复制 API 的授权、脱敏或 DTO 白名单规则。

## 修改入口与唯一所有者

| 职责 / 原位置 | 当前入口 | 所有者与依赖 |
| --- | --- | --- |
| 详情装配 / `TaskDetailView.vue` | `../../views/TaskDetailView.vue` | 读取路由 ID、装配能力、消费同一组 ref，保留原模板和样式 |
| 概览、快照、命令、执行状态、两秒核对 / 详情页脚本 | `useTaskDetail.ts` | 每个组件实例唯一事实状态；每次 ID 变化建立独立会话及 AbortController；冻结证据成功读取后不反复请求 |
| 任务绑定与失效资格 | `taskDetailSession.ts` | 只有 ID、API 端口和有效性判定；不包含事实或表单副本；会话对象区分 A → B → A |
| 日志补读、游标、去重、SSE / 详情页脚本 | `useTaskLogs.ts` | 唯一日志列表、可靠游标与日志错误；消费同一会话及执行事实，不自己读取执行状态 |
| 单流关闭与连接代隔离 / `views/taskLogStreamLifecycle.ts` | `taskLogStreamLifecycle.ts` | 每个实例一个流句柄；旧断开与记录回调不能影响新连接；支持打开过程中同步断开 |
| 新建、从头执行、检查点继续、模板保存 / 详情页脚本 | `useTaskActions.ts` | 唯一输入、忙状态和操作反馈；只在当前会话跳转，服务端仍复验资格 |
| 详情时间、状态标签与字节显示 / 详情页脚本 | `taskDetailPresentation.ts` | 纯展示转换，保留详情与列表不同文案，不制造第二套状态转换规则 |
| 失败摘要 / `views/taskFailurePresentation.ts` | `taskFailurePresentation.ts` | 只提取已经脱敏的失败日志，不推断根因 |
| 列表标签 / `views/taskListPresentation.ts` | `taskListPresentation.ts` | 保留原标签及未知状态的核对语义；`TaskCenterView.vue` 只更新导入路径，本阶段未重构分页 |

## 异步边界

- ID 同步 watcher 使旧会话立即失效，清空全部详情事实、日志、游标、操作输入与反馈，再读取新任务。HTTP 使用原 `browserApi(signal)`，不修改 CSRF、幂等或响应投影。
- 所有成功、错误、finally、导航和流回调都验证捕获的会话。流事件额外验证连接代；关闭、断开、重连或任务终态使旧事件资格失效。
- 同一会话的刷新调用与单项读取复用在途 Promise；手动刷新取消原等待计时器，刷新完成只安排一个下一轮计时器。冻结快照及命令成功后保持原缓存语义，状态和日志继续核对。
- HTTP 日志补读只能在当前游标仍等于请求发起时游标时更新它，不比较不透明游标大小。流已推进游标时，迟到 HTTP 不能把它倒退。
- 卸载释放 HTTP 等待、轮询和流。取消浏览器等待不撤销服务端已接受的写操作；过期写响应不强制导航、提示成功或清空新任务输入。
- 检查点按钮仍只消费 `FAILED + checkpointPresent`；草稿派生目标步骤仍为 1 / 5，检查点继续仍跳转新任务 ID。该显示资格不代替服务端授权。

## 测试与维护

- `taskDetailLifecycle.test.ts`：同实例 ID 切换、A → B → A、迟到成功/错误、卸载、单一刷新链、日志去重/游标/连接代、操作防重及迟到导航。
- `taskLogStreamLifecycle.test.ts`：断开重连、陈旧断开、同步断开及旧事件资格。
- `taskListPresentation.test.ts`、`taskFailurePresentation.test.ts`：迁移原测试，不删改断言。
- `../../../tests/task-detail.spec.ts`：真实路由装配，来源链接复用组件、合成事件、迟到 HTTP、终态/卸载、派生写请求头与导航、模板、两档视口。
- `../../../tests/reference-pages.spec.ts`：原四代表页、冻结证据、响应式与控制台回归。

追加功能前先判断是模板展示、纯展示规则、详情事实、日志生命周期还是写操作。不要将新的操作状态复制到页面，也不要让日志能力接管派生操作或让页面直接发送网络请求。工程规则见[前端平台基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)。
