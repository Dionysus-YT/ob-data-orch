# SQLite 迁移

> `0040_log_query_positions.sql` 为日志中心固定快照新增批次顺序索引；仅回填 batch_id，正文、历史任务和旧迁移不变。新登记与序号同事务，清理级联且水位不重用。当前完整链 40 项迁移、42 张 STRICT 表；2026-10-25 经一致备份后按本机二进制流程安装并验证现役库，身份/授权/配置和原业务数量保留。见[日志查询契约](../docs/03-technical/log-center-task-query-contract.md)。

> `0039_scheduling_candidate_order.sql` 增加独立候选顺序单例，初始 `EXISTING` 保持原顺序，`ONLINE_FIRST` 只稳定提前已确认在线候选；不改写节点、执行或用户选择。当前完整链为 39 项迁移、41 张 STRICT 表。策略、审计与回执同事务，普通/旁路候选先按当前节点权限取同快照事实及预算；隐藏坏节点隔离。仅临时 SQLite 验证，尚未迁移现役库，见[调度契约](../docs/03-technical/scheduling-policy-contract.md)。下条为时间切片时点的历史状态。

> `0038_platform_scheduling_timing.sql` 增加独立时间策略与按 Agent/任务类型的持久检查时点，初始覆盖为空，继承实际心跳基线且不伪造启动/检查默认值。新领取冻结 `task_executions.startup_deadline`，触发器禁止后续改写，历史空期限不回填；超时只附加核对、保留状态及预算。当前完整迁移链为 38 项、40 张 STRICT 表，仅临时 SQLite 验证，未迁移现役库。时间设置、审计、幂等及新领取约束见[调度契约](../docs/03-technical/scheduling-policy-contract.md)。下条迁移数量为各自时点的历史值。

> `0037_platform_scheduling_policy.sql` 增加全平台执行并发单例：初始为空上限、版本 1，保留既有节点预算/Agent 约束；首次配置后为 1–1024 的整数，修改人及时间成组。策略、审计和幂等原子保存，新领取在同一写事务读取最新策略及未可靠释放占用。当前完整迁移链为 37 项、38 张 STRICT 表；仅临时 SQLite 验证，未迁移现役库。见[调度契约](../docs/03-technical/scheduling-policy-contract.md)。下条 0036 数量为该迁移时点的历史值。

> `0036_ordinary_log_retention_policy.sql` 增加普通任务日志策略单例：初始为空覆盖、版本 1，继承既有部署配置；后续覆盖限定 7–365 天并与修改元信息成组。配置、审计和幂等由应用短事务提交。当前完整迁移链为 36 项、37 张 STRICT 表；仅临时 SQLite 验证，未迁移现役库。见[保留设置契约](../docs/03-technical/log-retention-settings-contract.md)。

> `0035_subject_permissions.sql` 仅前向增加模板范围、独立权限版本及新任务冻结授权字段。保留身份、既有范围与历史不可变任务；迁移不引导认证账号、不回填历史权限。初始管理员由可信组合根另行引导一次，重启不恢复撤权。本轮只在临时 SQLite 验证，未升级现役数据库。见[权限契约](../docs/03-technical/access-control-contract.md)。

`0034_direct_load.sql` 以独立 `OBLOADER_DIRECT_LOAD` 类型接入旁路草稿、两种固定预检查、配置安全投影指纹及冻结快照。它前向重建封闭枚举约束，保留已有行、外键、索引和触发器，不修改历史迁移，不开放旁路模板。独立执行链默认关闭，不借其他任务开关放行。只在临时 SQLite 验证，未升级现役数据库；当前能力和阻断见[旁路技术契约](../docs/03-technical/direct-load-contract.md)。

`0031_node_environment_inspection_authorization.sql` 为当前 Agent 身份与待消费注册码各增加受 CHECK 约束的固定本机复验授权布尔值。旧行默认为 0，升级不扩大授权、不改变机器凭据和历史检查；新关联或存量节点明确授权后才可自动运行受控版本查询。身份撤销/替换及节点退役使有效授权失效，关联重放不重新授权。无新表，仅隔离 SQLite 验证，不表示已升级现役运行库。

`0001_initial.sql` 是首条纵向切片的正式前向迁移草案，建立 AD-R14 基线的窄表及关键外键、唯一性、`STRICT` 和不可变约束。`0002`～`0019` 依次补充 ODP 身份、数据源验证、Agent 事实/心跳幂等、预检查租约与秘密槽位、连接测试、节点环境、执行秘密槽位、持久日志、Export 泛化、系统凭据、对象存储凭据、任务派生和预检查存储凭据绑定。

`0020_task_execution_cancellation.sql` 为 `task_executions` 增加取消请求标识、控制面请求时间、固定期限和固定原因码。取消不保存命令、路径、秘密或自由文本；排队任务可直接投影为 `CANCELLED`，已领取任务先进入 `CANCELLING`，由 Agent 在同一租约内轮询并上报进程树终止事实。

`0007_persist_precheck_leases.sql` 为 `precheck_runs` 增加节点事实版本、绑定摘要和领取 Agent 投影，并新增 `agent_precheck_receipts`。该回执表仅保存 `(agent_id, request_id)`、操作、请求摘要、预检查/租约/epoch、绑定摘要、控制面到期时间和安全状态投影；它不保存机器凭据、连接信息、路径、命令、SQL、秘密槽位或原始检查输出。服务层以回执摘要判断 claim、acknowledge、complete 的同请求重放或同 ID 异摘要冲突；租约以控制面时间过期，迟到的完成结果不得将过期预检查提升为成功。

`0008_add_precheck_secret_resolution_receipts.sql` 使当前迁移链达到 22 张窄表。它新增预检查数据库连接槽位的无秘密幂等回执，仅保存 Agent、请求摘要、预检查/租约/epoch、绑定摘要、授权或完成状态与时间；不保存主机、用户名、密码、密文、nonce、长度提示或响应正文。相同请求可以在当前有效绑定内重新从密文解析，异摘要请求失败关闭。

`0010_add_execution_node_environment_checks.sql` 为执行节点增加固定环境检查的安全投影：不透明检查标识、状态、稳定证据码、绑定的 Agent 事实版本和时间。它不新建远程控制通道，不保存 Java/工具路径、命令、数据库信息或原始错误。平台声明变更会清除旧结论；只有当前 Agent 心跳、同一事实版本的 `TOOL_RUNTIME_READY` 结论同时成立时，节点才可由节点管理员启用。

`0011_add_execution_node_runtime_configuration.sql` 补充节点管理员在注册界面登记的 OBDUMPER 工具目录与工具专用 Java 路径。它们只是目标机器配置声明；首次关联的 Agent 必须在本机复核后才可形成运行时就绪结论，配置变更会使旧结论失效。

`0012_add_execution_secret_resolution_receipts.sql` 新增正式 OBDUMPER 导出任务的数据库连接槽位无秘密回执。它只保存 Agent、请求摘要、execution/lease/epoch、任务信封摘要、授权或完成状态和时间；不保存主机、用户名、密码、密文、nonce、长度提示或响应正文。控制面只在当前租约、节点、数据源、凭据与创建者范围仍有效时授权一次短时解析，并用请求摘要拒绝异内容重放。

边界：

- 只覆盖数据源、节点/Agent、CSV 导出草稿与预检查、任务执行、审计和日志索引；
- 不包含普通导入、旁路导入、模板、定时调度或完整系统设置；
- 不保存密码明文、机器凭据原值、根密钥、未脱敏命令/日志或导出文件；
- 只提供前向迁移，不提供自动 down migration；
- 迁移应用后以 SHA-256 校验和锁定，禁止更新或删除历史记录。

当前只在临时 SQLite 和合成数据上验证。Windows/三个麒麟目标的正式升级、备份恢复和服务权限仍按 G3/G4 门禁执行。

`0030_normal_import.sql` 在历史物理草稿表增加 `draft_kind`，以独立配置承载普通导入并隔离导出读写；预检查增加固定用途与输入摘要 JSON，任务冻结增加固定类型。保持既有外键、历史迁移和默认导出语义，不新建共享导入领域或复制旁路配置。文件发现不能作为提交证明，普通导入不进入现有导出执行器。仅在临时 SQLite 验证，未升级现役数据库。

`0028_log_retention_cleanup.sql` 为普通日志段增加 `DELETING → DELETED` 清理账本，冻结内部文件键及段事实。封段仅在账本匹配的清理事务中允许删除；任务、快照、结果、操作记录和审计保留。文件删除与索引确认分离，崩溃重启可续作。仅在临时 SQLite、合成日志与临时目录验证。

`0029_export_draft_deletion.sql` 增加 `export_drafts.deleted_at` 及所有者/时间索引。领域短事务复验所有权、版本、提交和预检查租约后标记删除、失效预检查；不物理删除历史引用，不新增草稿执行状态。仅在临时 SQLite 验证。

`0025_decouple_execution_node_history.sql` 将历史节点外键改向只含 `node_id` 的不可变标识表，现存配置仍为 `execution_nodes`，永久删除配置不改写任务快照和历史证据。同一标识不能复用；创建幂等回执保留。该迁移还为 0024 的两张预算证据表补齐 STRICT，保留所有行和禁止改写/删除的触发器。迁移全程保留 `foreign_keys=ON`，提交前检查全库引用；不可变历史迁移不修改。

`0027_task_display_facts.sql` 新增不可改写、不可删除的 `task_display_facts`，在任务提交与检查点继续的同一事务冻结数据源名称、环境和执行节点名称，供任务中心在对象删除后仍显示历史名称（DEC-050）。迁移前已有任务按迁移时的当前名称回填，不保证等于当初提交时的名称；对象已删除的任务名称为空。不保存连接信息、路径、凭据或命令。只在隔离临时 SQLite 验证，未升级现役数据库。

`0026_export_sys_precheck_binding.sql` 冻结附加表定义信息预检查所需的 sys 引用与修订。CHECK 明确拒绝任一侧单独为 NULL；历史记录两列均为空，不改写历史结论。引用参与预检查摘要，轮换/撤销使当前证据失效；密码和账号不写入此表。只在隔离临时 SQLite 验证，未升级现役数据库。
