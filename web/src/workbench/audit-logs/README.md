# 审计日志维护入口

本域是日志中心独立审计预设，沿用 [P1](../../../../design-system/pages/log-center.md) 和[首批契约](../../../../docs/03-technical/log-center-audit-query-contract.md)。不管理权限、身份、任务或审计保留期。

| 能力 | 唯一所有者 |
| --- | --- |
| 预设资格与路由装配 | `log-query/useLogPresets.ts`、`views/LogCenterView.vue` |
| 表单、快照分页、单事件与下载会话 | `useAuditLogs.ts`，按异步事务分别绑定代次与 AbortController |
| 工作面、抽屉与安全字段标签 | `AuditLogWorkspace.vue`、`AuditDetailDrawer.vue`、`presentation.ts` |
| 严格 DTO 与浏览器安全传输 | `api/auditLogs.ts`、`api/browser.ts` |
| 服务端投影、令牌、预算 | `internal/auditquery` |
| 当前对象裁剪、追加水位与访问审计事务 | `internal/store/audit_query.go`、迁移 0041 |

仅处理固定审计字段；未知差异从不原样返回。初始不宽查，切换、输入与卸载使旧异步结果失效；撤权和读取失败撤下缓存。平台/API 不反向依赖本域，任务日志不持有审计表单或状态。

与任务预设的两个跨域引用仅复用日志中心既有 UTC 时间格式函数，不共享查询状态、API 缓存或业务规则；无反向依赖和循环值依赖。
