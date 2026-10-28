# 平台设置维护入口

- `views/PlatformSettingsView.vue`：统一标题、路由驱动的四个 Ant 页签与唯一当前域出口；其他设置域继续由原能力所有者持有事实与事务。
- `views/GeneralSettingsView.vue`：常规设置分组和普通日志装配，未开放项集中折叠说明，不展示尚未接入的校验与生效行为。
- `LogRetentionSettings.vue`：普通日志 Form、带单位只读/编辑数值控件、配置详情折叠和确认视图；控件只在当前编辑会话接收变化，复用原生 Ant 反馈，不持有第二份事实。
- `useLogRetention.ts`：唯一读取、编辑、确认、幂等重试和写事务会话，并派生未保存状态；路由变化或卸载失效旧代。视图复用路由/注销离开守卫，保存中不能切换页签。
- `api/logRetention.ts`：安全策略/回执严格解码；传输复用 `api/browser.ts`。
- 生效、权限与清理边界见 `docs/03-technical/log-retention-settings-contract.md`。普通日志事务不承载其他设置域、任务历史或审计日志保留；页签分组规则见 `design-system/pages/system-settings.md`。
