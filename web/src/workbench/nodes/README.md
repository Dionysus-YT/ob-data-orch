# 执行节点维护索引

工程约束沿用 [AGENTS](../../../../AGENTS.md) 与[前端平台基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)，产品字段与行为查[执行节点设计](../../../../docs/02-design/execution-node-module.md)，注册协议查[Agent 契约](../../../../docs/03-technical/agent-task-state-contract.md)。此页是定位索引，不另建规范。

| 要修改的职责 | 唯一维护入口 | 状态或边界 |
| --- | --- | --- |
| 路由、表格列、布局、弹层焦点、页面导航 | `views/ExecutionNodeView.vue`、`ExecutionNodeFormView.vue`、`ExecutionNodeDetailView.vue` | 保留原 template/style 和 Ant 组件；页面不持有业务表单副本 |
| 列表读取与已授权事实筛选 | `useNodeList.ts` | nodes、筛选、读取状态、错误与失效序号 |
| 启用、环境请求、删除/归档事务 | `useNodeListActions.ts` | 操作锁、删除目标、反馈、一个延迟复读；通过列表能力发布新事实 |
| 详情事实与基于修订的操作 | `useNodeDetail.ts` | node、读取和操作状态；写入前使旧读取失效，操作期间拒绝刷新；没有第二个节点副本 |
| 节点登记/编辑 | `useNodeForm.ts` | 唯一 form、已加载节点修订、错误、保存状态与 DTO 映射；模式/ID 切换重新加载或重置 |
| 表单规则与字段错误白名单 | `nodeFormRules.ts` | 原字段标签/ID、Windows/Linux 路径规则与唯一校验；不替代服务端安全与路径检查 |
| 一次性材料与复制反馈 | `useNodeEnrollment.ts` | 独立弹层代和复制代；关闭、关联完成、路由切换、卸载清引用；不写存储、日志、路由或命令 |
| 会话有效性、HTTP 取消 | `nodeSession.ts` | 仅节点模块内部共享的真实复用能力；每次绑定变更产生新会话代；A → B → A 不能恢复旧回调 |
| 状态展示与主操作资格 | `executionNodePresentation.ts` | 原资格实现及格式化唯一入口；接收任务资格仍使用 API 可信投影 |
| 注册码编码与固定启动入口 | `executionNodeEnrollmentInstructions.ts` | 原 `obdo-r1` 协议；固定包下载、启动指令不携带材料 |
| 请求、安全、响应验证 | `api/browser.ts` | 沿用原 API/CSRF/If-Match/幂等/错误机制，本阶段不修改 |

异步成功、catch、finally、导航和定时器都核验捕获的会话。独立刷新按读取代丢弃旧响应，写操作入口加锁并使之前读取失效；不因按钮 disabled 就假定方法不会重入。列表和详情环境操作只有一个 2500ms 延迟读取，替换或卸载时清理，不引入轮询。HTTP abort 只是取消等待，已送达控制面的变更仍可能完成，重新进入页面重新读取事实。

注册材料不与详情事实同步复制。清除引用不等于 JavaScript 内存物理擦除，也不删除用户已复制的剪贴板内容；保持既有提醒与安全契约。

验证入口：`nodeLifecycle.test.ts` 包含路由复用、迟到、卸载、读取/写入竞态、防重、定时器和敏感材料负例；两个迁入的展示/注册测试保留原断言。浏览器：`tests/execution-node-page.spec.ts` 原回归，`tests/execution-node-lifecycle.spec.ts` 新增同实例场景。实际执行结果、文件清单与限制见[第二阶段验收记录](../../../../docs/03-technical/evidence/frontend-business-phase2-2026-10-09.md)。
