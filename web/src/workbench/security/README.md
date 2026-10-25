# 安全策略维护入口

- `views/SecurityPolicyView.vue` 只装配 Settings 工作面、路由会话与安全状态展示。
- `useSecurityPolicy.ts` 唯一持有只读事实、请求防重、取消与会话代；路由变化、撤权、失败和卸载都不能重放旧安全状态。
- `api/securityPolicy.ts` 严格校验非秘密白名单、实际启动时间、固定安全模式及执行开关依赖；没有写接口。
- `internal/securitypolicy` 持有不可变启动事实及内存密钥标识可用性；`internal/controlplane/security_policy.go` 在读取前后复验 ACTIVE 系统管理员。
- 根密钥材料、文件路径及摘要不进入端口；密钥版本别名不等于密钥指纹。审计保留未配置且没有自动清理，普通日志期限不能套用到审计。
- 首批不提供密钥维护、敏感命令原文、保护降级或审计清理；后续修改须有对应契约和消费者。
