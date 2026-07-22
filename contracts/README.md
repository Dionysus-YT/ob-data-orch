# API 契约

`openapi.json` 是首条纵向切片的 OpenAPI 3.1 结构与安全边界基线，覆盖已确认的 29 个浏览器操作和 14 个 Agent 操作。

当前状态为 `G2_CONTRACT_ONLY`：

- 浏览器 `/api/v1` 与 Agent `/agent/v1` 使用不同认证域；
- 浏览器写操作显式声明 CSRF、幂等或 `If-Match` 要求；
- 数据源密码和关联材料只允许作为 `writeOnly` 输入；
- 任务提交仍标记为 `DISABLED_UNTIL_G3`，不存在浏览器直接执行、取消或重试入口；
- 复杂资源响应和 Agent operation-specific payload 当前仍是结构占位，必须在 DEV-06 前端接入前收紧为严格 schema。

该文件不决定身份提供方、首次管理员或故障恢复身份，这些仍是进入真实集成和发布的外部阻断项。
