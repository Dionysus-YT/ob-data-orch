# API 契约

> 2026-10-24 工具与兼容新增只读 `GET /api/v1/settings/tools`；当前 ACTIVE 系统管理员、读取前后复验、内置目录深副本与严格 Schema。导出按配置选择和历史重放分别表达，普通/旁路使用独立目录；无版本修改、节点安装结论或 SQLite 迁移，OpenAPI 当前 118 项操作。详见[工具状态契约](../docs/03-technical/tool-compatibility-contract.md)。

> 2026-10-24 安全状态新增 `GET /api/v1/settings/security` 及严格只读 Schema；当前 ACTIVE 系统管理员、前后复验、未知失败关闭和非秘密白名单共同约束。没有写接口或 SQLite 迁移，OpenAPI 当前 117 项操作；详见[安全状态契约](../docs/03-technical/security-policy-contract.md)。

> 2026-10-24 候选顺序新增独立 `GET/PATCH /api/v1/settings/scheduling/candidates` 与受控 `EXISTING/ONLINE_FIRST` Schema，沿用调度设置权限、CSRF、强版本、幂等、确认及原子审计。普通/旁路导入以 `candidateOrder=platform` 读取授权节点快照，导出候选自动应用当前顺序；管理和连接测试列表不变。始终显式选择，时间响应的 `candidateStrategy` 只表示选择方式，顺序由此独立策略维护。见[调度契约](../docs/03-technical/scheduling-policy-contract.md)。

> 2026-10-24 调度时间切片新增独立 `GET/PATCH /api/v1/settings/scheduling/timing` 与严格时间投影/写入 Schema，沿用既有调度 scope、当前系统管理员、CSRF、版本、幂等与原子审计。三个时间字段显式提供，启动/检查初始为空且配置后不能置空重置；候选策略仅返回固定显式选择。机器协议不新增网络周期字段，成功领取回执先于时间策略读取/节流重放。迁移 0038 仅临时 SQLite 验证，见[调度契约](../docs/03-technical/scheduling-policy-contract.md)。

> 2026-10-24 新增调度策略 `GET/PATCH /api/v1/settings/scheduling`、严格策略/写入/回执 Schema 与迁移 0037。当前系统管理员、CSRF、If-Match、幂等、首次设置/降低确认及原子审计共同约束；计数另需既有全局任务读取权限，响应前复验。任务执行详情仅增加可选固定等待原因，不返回全局数量或其他任务。见[调度契约](../docs/03-technical/scheduling-policy-contract.md)。

> 2026-10-23 新增普通任务日志保留策略 GET/PATCH、严格策略/写入/回执 Schema 与迁移 0036；浏览器身份、当前系统管理员、CSRF、If-Match、幂等、降低确认及原子审计共同约束。只覆盖普通日志，不新增清理触发入口，见[契约](../docs/03-technical/log-retention-settings-contract.md)。

> 2026-10-23 权限配置首批：新增 access-control 主体列表、详情/原子 PATCH 与最小候选目录，浏览器身份、CSRF、If-Match、幂等和固定权限管理职责共同约束。只管理已有认证投影，不创建账号；独立能力和 NODE_OPERATE 不开放新增。见[权限契约](../docs/03-technical/access-control-contract.md)。

2026-10-23 新增首页固定只读 `GET /api/v1/overview` 与任务列表 `finishedFrom/finishedTo` 终态时间筛选。`OverviewSnapshot` 对四项指标、唯一趋势和有限事实列表使用同一快照；节点在读取前授权标识、返回前复验，非 READY 分区不携带数据。当前明确使用内置 UTC，不把空范围、未知、失败或受限混同；没有新增执行、秘密解析或任意查询能力。详见 [API 与 SQLite 契约 §5.1.1](../docs/03-technical/api-sqlite-data-contract.md#511-首页只读运行快照2026-10-23)。

2026-10-19 旁路导入新增独立 `/direct-load-drafts`、`/direct-load-prechecks` 和 `direct-load.json` 配置边界。103 项官方研究目录、单表输入清单、九项预检查及三键安全投影指纹与普通导入隔离，旧导出接口不能访问旁路草稿；统一草稿和任务派生识别真实类型。2026-10-20 按 DEC-057 限定本地部署：`LocalConfig` 约束活动写入、预检与执行为私有 ODP / OBServer 和 CSV；`Config` 仅为历史草稿与快照保留读取兼容，页面目录投影为 102 项。旁路专用探针、提交与 Worker 分别复验独立许可、当前绑定和证明，默认关闭，不继承其他任务开关。源码及合成验证不代表真实 DLV 通过，具体实现和阻断见[旁路技术契约](../docs/03-technical/direct-load-contract.md)。

2026-10-16 节点自动复验：固定检查授权持续绑定当前 ACTIVE Agent，并逐检查保存许可快照。新关联及存量节点授权检查明确授予；旧材料不追认。手动检查保留管理状态，浏览器新增 UNCONFIRMED 和待授权原因，未知证据与真实异常分别解释；机器回执枚举不变，完整清单和当前事实绑定仍必须通过。见[复验证据](../docs/03-technical/evidence/execution-node-auto-inspection-2026-10-16.md)。

`openapi.json` 是 Export v1 当前 OpenAPI 3.1 契约与安全边界基线，覆盖已实现的浏览器/Agent 操作、结果证据投影和受控取消轮询。

2026-10-15 普通导入 G2 增量使用独立 `/normal-import-drafts`、`/normal-import-prechecks` 与 `normal-import.json` DTO。2026-10-16 补齐受本机开关与完整租约约束的固定只读连接/写权限/映射探针，以及第五步自动准备、提交可选 `taskName` 与服务端自动命名。名称独立冻结，不改变预检查配置；输入文件事实不作为结果，未知证据和独立执行门禁阻断提交。真实 OBLOADER 执行与 load.ckpt 未接入，不能把只读检查或冻结提交结构视为实际可执行能力。具体实现状态见[普通导入技术契约](../docs/03-technical/normal-import-contract.md)。

当前状态为 `G2_CONTRACT_ONLY`：

- 浏览器 `/api/v1` 与 Agent `/agent/v1` 使用不同认证域；
- 浏览器写操作显式声明 CSRF、幂等或 `If-Match` 要求；
- 数据源密码和关联材料只允许作为 `writeOnly` 输入；
- 任务提交仍受真实执行开关、节点/Agent 事实和任务地图门禁约束；取消入口只提交有界意图，不提供浏览器直接终止进程、任意命令或重试入口；
- 复杂资源响应和 Agent operation-specific payload 当前仍是结构占位；DEV-06 只使用实际控制面已实现的最小浏览器端点，未把结构占位当作可用功能。

DEV-06 已额外确认：数据源浏览器读取投影不得返回用户名、密码、密文或凭据引用；前端在保存前只取用明确的脱敏字段，并对命令与日志执行第二层展示前检查。执行节点列表和运行时会话/CSRF 注入尚未接入实际控制面，前端对缺少 CSRF 的写操作失败关闭，不以本地伪造节点或令牌代替契约。

该文件不决定身份提供方、首次管理员或故障恢复身份，这些仍是进入真实集成和发布的外部阻断项。

2026-10-20 普通导入控制映射清单增加可选 `controlDigest`（64位摘要），参与总digest并执行前复核；无控制映射的旧清单摘要保持兼容。新增JSON事实不需要SQLite历史迁移；普通导入固定JDBC私有帧为v11，仅增加类型枚举与非表对象权限事实，旧v10帧拒绝。边界与真实十九项复验见[普通导入契约](../docs/03-technical/normal-import-contract.md)。

日志中心任务预设增加四个固定操作，OpenAPI 共 122 项；AEAD 快照绑定当前授权，所有输出仅为已脱敏持久日志，详见日志查询契约。
