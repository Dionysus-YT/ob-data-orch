# 全平台反馈整改验证

日期：2026-10-12。按用户“按建议整改，并写进规范”的要求，统一现有执行节点、数据源、存储凭据、模板、任务和导出向导的操作反馈。原实现把操作成功、持续边界和复制失败混在常驻提示中，部分页面另有自绘错误框；本次按反馈语义统一到原生 Ant 组件。

## 规范与实现

[P0 状态与反馈规范](../../../design-system/patterns/status-feedback.md#全平台反馈规则) 是唯一反馈规则入口；MASTER、DESIGN、组件语义、相关 P1、前端工程基线、项目 AGENTS.md 和节点/导出维护索引已同步引用。

| 情境 | 当前规则与实现 |
| --- | --- |
| 保存、删除、启用、复制等主动操作成功 | 原生 Ant Message，3 秒自动关闭，只确认已完成动作；无更改使用 info。 |
| 持续说明、风险和请求失败 | 原生 Ant Alert + show-icon，保留安全解释和适用恢复入口；复制失败不再进入成功提示。 |
| 字段错误 | Ant Form.Item 关联实际输入；IN 值转换校验同步归入字段帮助。 |
| 连接测试、预检查、任务结果 | 保留领域结果与证据；节点启用只确认启用，具体任务的检查边界仍在详情中常显。 |
| 行内状态开关、刷新与轮询 | 保留原位 loading 和事实反馈，不重复弹出成功提示。 |

`useOperationMessage` 只管理本地原生 Message 上下文，每个上下文最多一条。业务会话继续唯一持有结果，重试清除旧提示；关闭、切换、授权失效和卸载清理反馈，重新读取不重放成功。导出复制增加请求序号与预览版本校验，拒绝迟到成功和失败；提示内容不持有注册材料、密码、命令或 SQL。

连续保存的浏览器复核发现 Ant Design Vue 4.2.6 的旧关闭动画回调会清理同位置的新提示。本次在发布结果时更换当前原生上下文，并在 nextTick 后复验会话序号与卸载状态；不修改依赖或重绘组件。连续草稿保存、模板反复改名和相同解析结果均已验证。遗留 `.feedback-error`、`.feedback-notice`、凭据自绘反馈及任务失败框样式已移除。

## 实际验证

| 检查 | 结果 |
| --- | --- |
| `npm test` | 26 个文件、311 项全部通过。覆盖共享反馈清理、相同结果重发、上下文等待期间失效、卸载和迟到复制等负例。 |
| 相关隔离 Chrome 回归 | 52/52 通过。覆盖节点生命周期/注册、凭据/模板事务、数据源保存与独立测试、任务派生、导出草稿连续保存和复制、七项反馈专测，以及四代表页的三个视口。 |
| 合成解析夹具调整后的复验 | 1/1 通过；运行时输入仍为同一无密码合成连接串。 |
| `npm run typecheck`、`npm run lint`、`npm run build` | 全部通过；夹具调整后的单文件 ESLint 也通过。 |
| `npm run audit:business` | 11 项审计测试通过，0 违规、6 个人工复核信号。 |
| `npm run audit:wizards` | 3 项审计测试通过，0 违规、4 个人工复核信号。 |
| 秘密扫描（PowerShell 7）与 `git diff --check` | 通过。 |

浏览器回归可从 `web/` 重现：

```powershell
npx playwright test business-lifecycle.spec.ts execution-node-lifecycle.spec.ts execution-node-registration.spec.ts source-contracts.spec.ts task-detail.spec.ts platform-feedback.spec.ts reference-pages.spec.ts --grep 'business-lifecycle|execution-node-lifecycle|execution-node-registration|source-contracts|task-detail|platform-feedback|数据源迁移|四代表页|结果集 SQL 工具栏|导出草稿修改'
```

页面使用隔离端口 15174 与合成 API/剪贴板响应。已查看原生启用 Message、查询编辑器和任务详情截图；完整回归截图位于忽略目录 `artifacts/frontend-platform/`，夹具复验输出独立保存在 `artifacts/frontend-feedback-20261012/`。首次秘密扫描将测试源码中空 `-p` 后的引号误判为密码；夹具改为现有参数数组格式后扫描通过，没有放宽扫描规则。

架构人工复核：新增公共能力只依赖 Vue 与 Ant，不依赖业务模块。草稿、命令预览与预检查仍归 `useExportDraftLifecycle`；SQL 编辑器只持有自身编辑反馈，页面和步骤不复制领域状态。其余规模信号沿用 API、纯规则、目录与参数能力的原边界；模板对导出资格纯规则的复用有真实消费者，本次不按文件大小拆分或引入通知总线。

## 验证边界

本轮是本地前端整改与合成验证，没有新增 API、Token、依赖、Product Shell 或真实执行能力，未替换服务二进制或重启控制面/Agent，未做真实注册、数据库访问、工具启动或 Linux 原生验证，不提升真实能力准入。

生产构建仍有既有的大 chunk 提示。Token CSS 同步和平台样式审计的既有阻断见[注册重构记录](execution-node-registration-2026-10-10.md#未通过的既有门禁与实际边界)；本轮未修改 Token 或重跑该全局治理范围，不能将上述局部验证写成整体 verify 门禁通过。浏览器运行出现一次 ResizeObserver 循环告警，对应用例和最终 52 项均通过，未扩展为无关编辑器/布局重构。
