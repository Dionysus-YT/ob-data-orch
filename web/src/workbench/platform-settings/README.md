# 平台设置维护入口

- `LogRetentionSettings.vue`：普通日志字段和确认视图，复用原生 Ant 反馈。
- `useLogRetention.ts`：唯一读取、编辑、确认、幂等重试和写事务会话；路由变化或卸载失效旧代。
- `api/logRetention.ts`：安全策略/回执严格解码；传输复用 `api/browser.ts`。
- 生效、权限与清理边界见 `docs/03-technical/log-retention-settings-contract.md`。不承载其他设置域、任务历史或审计日志保留。
