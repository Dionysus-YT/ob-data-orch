# 日志中心任务日志维护索引

本域沿用前端平台基线和 [P1](../../../../design-system/pages/log-center.md)，不持有任务执行、节点或审计管理状态。

| 能力 | 唯一所有者 |
| --- | --- |
| 预设资格、布局和导航 | `useLogPresets.ts`、`views/LogCenterView.vue` |
| 任务路由带入与工作面 | `TaskLogWorkspace.vue` |
| 表单、已应用条件、快照游标、分页、请求代次 | `useLogQuery.ts` |
| 同源上下文和批量复制/下载事务 | `useLogQuery.ts` 的独立会话代次与 AbortController |
| 抽屉呈现与焦点 | `LogContextDrawer.vue` + 既有 `useAntDrawerDialog` |
| 纯文本高亮、来源和有界摘要 | `presentation.ts` |
| DTO 校验与固定 API | `api/logQuery.ts`；认证、CSRF 与下载传输沿用 `api/browser.ts` |
| 服务端权限/索引/审计 | `internal/store/log_query.go` |
| 服务端查询/预算/加密快照 | `internal/logquery` |

初始不宽查。输入和路由变化立即撤销旧异步展示与写响应资格；重复点击在方法入口防重。保留上次查询的条件与事实供辨认，撤权/失效游标则清空，A → B → A 不能复用旧代次。完整正文只在当前授权会话可见，不进入全局通知或持久前端状态。上下文复制先取得服务端审计结果，再向剪贴板写入；浏览器迟到下载不能保存或提示成功。

审计表单与状态独立位于 `workbench/audit-logs`，不追加到任务查询所有者。

工作面只在可信结果非空时挂载表格，首次/空结果使用 Ant Empty，首次查询使用 Skeleton，失败使用原错误 Alert；刷新保留可信旧表，不借空表做横向尺寸测量。来源标识、代次和未取得的原始时间/级别收进上下文“来源详情”，接收时间、来源、缺口及下载范围保持可见。`tests/log-query.spec.ts` 同时收集原生窗口错误，覆盖普通 `pageerror` 未覆盖的 ResizeObserver 交付异常，不屏蔽错误或改写观察器。
