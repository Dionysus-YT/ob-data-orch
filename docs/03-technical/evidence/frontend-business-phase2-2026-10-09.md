# 前端业务架构治理第二阶段：执行节点

日期：2026-10-09。范围：用户确认的执行节点列表、登记/编辑、详情及辅助代码。基线提交 `02f61ce`；本阶段不修改后端、存储凭据、数据源、模板及其他业务页面，不推进第三、四阶段。

## 职责迁移与实际文件清单

原页面混合读取、版本写入、校验、敏感材料和定时器。现页面只保留路由、布局、列配置、Ant 装配、导航与焦点协调；三个页面的 template/style 与基线逐项比对一致。[节点维护索引](../../../web/src/workbench/nodes/README.md)是后续修改入口。

| 原职责 | 实际修改 / 迁移文件 |
| --- | --- |
| 列表页面、读取、筛选及操作 | `web/src/views/ExecutionNodeView.vue`；新增 `web/src/workbench/nodes/useNodeList.ts`、`useNodeListActions.ts` |
| 详情页面、事实读取与操作 | `web/src/views/ExecutionNodeDetailView.vue`；新增 `web/src/workbench/nodes/useNodeDetail.ts` |
| 登记/编辑页面、表单、校验与映射 | `web/src/views/ExecutionNodeFormView.vue`；新增 `web/src/workbench/nodes/useNodeForm.ts`、`nodeFormRules.ts` |
| 材料签发、清理与复制反馈 | 新增 `web/src/workbench/nodes/useNodeEnrollment.ts` |
| 节点内部共享会话与 HTTP 取消 | 新增 `web/src/workbench/nodes/nodeSession.ts` |
| 既有展示、资格及注册编码 | 从 `web/src/views/` 迁到 `web/src/workbench/nodes/`：`executionNodePresentation.ts`、`executionNodeEnrollmentInstructions.ts`；展示文件接收原详情的比例、字节及 bootId 格式化 |
| 原有单元断言 | 上述两个同名 `.test.ts` 同步迁入，内容与基线一致 |
| 新增单元与浏览器测试 | `web/src/workbench/nodes/nodeLifecycle.test.ts`、`web/tests/execution-node-lifecycle.spec.ts`；原 `execution-node-page.spec.ts` 未改 |
| 维护与工程约束 | 新增 `web/src/workbench/nodes/README.md`、本记录；更新 `AGENTS.md`、`docs/03-technical/frontend-platform-baseline.md`、`docs/README.md` |
| 协议边界与验证状态 | 更新 `docs/03-technical/agent-task-state-contract.md` 的浏览器材料生命周期说明、`docs/03-technical/development-task-map.md` 的本阶段证据；没有改变端点、载荷或注册协议 |

## 状态与异步生命周期

- 列表事实只有 `useNodeList` 持有；操作通过 `acceptNode` 发布结果并使用同一失效机制。详情事实只有 `useNodeDetail` 持有；注册能力消费该引用，不复制节点。
- `useNodeForm` 只有一个可编辑 form；加载节点提供服务端修订与回填来源，没有第二个可编辑副本或双向 watcher。校验与服务端字段白名单只有规则文件中的实现，DTO 转换只在表单能力中实现。
- 会话同步绑定 ID，表单额外绑定新建/编辑模式。变化后使旧会话失效并 abort，重置旧事实、操作和材料，再加载新对象。A → B → A 也不能接受旧 A 响应；卸载同样失效和清理。
- 读取有独立序号，成功/catch/finally 均核验。写事务捕获 ID/revision，入口锁防重，开始时使先前读取失效，写入期间拒绝刷新。成功、失败、finally 和新建成功导航均拒绝失效会话。
- 列表与详情环境请求最多各一个 2500ms 延迟复读，替换、路由切换或卸载时清理；没有新增轮询或流。
- 临时服务刷新失败保留可信事实；详情 401/403/404 清除对象，列表 401/403 清除授权列表。沿用原 API 错误白名单及服务端接收任务资格，不从前端状态推断权限。
- 注册材料只属于当前弹层代；关闭、关联完成、路由切换和卸载清除引用。签发与复制回调分别有代号，迟到回调不能恢复材料或写入新弹层反馈。不持久化，不进入路由、日志或命令。
- HTTP 取消不能撤销已签发材料或服务端事务；清引用不等于物理擦除，也不清除用户剪贴板，保留原产品提醒。重新进入页面重新读取当前事实。

## 实际验证

Windows、锁定依赖、隔离 Vite `127.0.0.1:15174` 与系统 Chrome；所有 API、节点、材料、错误均为合成夹具。没有调用真实控制面、Agent、数据库或官方工具。

| 检查 | 实际结果 |
| --- | --- |
| `npm test` | 22 文件、236 项通过；节点 3 文件、25 项，其中新增生命周期 19 项。覆盖路由/模式变化、A → B → A、迟到读/写/错误/finally/导航、卸载、修订保持、读取与写入竞态、防重、定时器、规则及材料/复制清理 |
| `npm run lint` | 全量通过 |
| `npm run typecheck` | 通过；最终 `npm run build` 也包含全量 `vue-tsc -b`，新增测试纳入类型检查 |
| `npm run build` | 通过；保留既有 >500 kB 分块提示，未修改阈值或扩展拆包范围 |
| Playwright | 两个节点文件共 6 项通过（原 2、新增 4），无跳过、重试或弱化原断言 |
| 浏览器证据 | 真实路由同实例标记保持；详情迟到刷新/签发、编辑写入/读取迟到、新建迟到导航、环境操作与离开后计时器取消通过。原列表 640/1280/1440/1920 视口无整页横滚，新增新建页 640 视口通过；保留原菜单、权限刷新、字段校验及焦点回归 |
| 内容一致性 | 三页面 template/style、注册编码实现、两个迁入测试与基线一致；API 与路由配置未修改 |
| 节点依赖复核 | 当前新增模块相对导入均解析成功，无反向依赖 views 或节点内部值循环。是局部复核，不冒充第四阶段正式全业务门禁 |
| `npm run audit:wizards` | 3 个检查器测试通过；0 violations、4 个既有 REVIEW；仍仅覆盖向导 |
| 秘密与差异 | `scripts/check-secrets.ps1`、`git diff --check` 通过；LF/CRLF 提示不是失败 |

成功截图在本机 Temp `ob-node-governance-verified`，首轮新增浏览器失败 trace 在 `ob-node-governance-regression`。首轮 1 项单元夹具错误：初始详情误返回已关联状态，正确触发材料清理；改为待关联夹具后通过。首轮 2 项新增浏览器定位错误：弹层与文本框名称相同导致严格匹配失败、环境按钮使用错误名称；按实际角色及既有名称修正后 6 项完整复验通过，未修改产品 UI 或原断言。

## 既有失败与未覆盖风险

只读核对基线 [CI 37870783148](https://github.com/Dionysus-YT/ob-data-orch/actions/runs/37870783148)：`02f61ce` 的 Web quality、secret scan 和三个目标构建通过，Go quality 的 Unit tests 失败。失败包括 `TestRunWithContext`、`TestWorker`、`TestStorageAvailableSpaceUsesTmpPathVolume`、目录空间采集测试、`TestConfigurationRejectsDigestRollbackAndCrossPlatform`、`TestAgentEnrollmentAndHeartbeatOverTLS`；属于本阶段前的后端问题，没有混入本次修复或声称整个 CI 通过。

第一阶段已记录 `tokens:check` 的 4 个环境色漂移，以及 `audit:platform` 的 24 条未引用选择器、4 条 Legacy。本阶段未改相关 token/样式、未重复执行这两项，不把历史失败写成通过。

未执行整个 Playwright 套件、非 Chrome 浏览器、原生 200% 缩放、真实注册/环境检查/配置同步或真实权限变更。合成测试证明浏览器状态隔离和 API 调用保持，不能提升真实 Agent 准入或部署状态。存储凭据、数据源反向依赖及模板生命周期债仍留第三阶段；正式全业务检查器、正反测试与 verify/CI 推广留第四阶段。本阶段完成后停止，等待下一阶段确认。
