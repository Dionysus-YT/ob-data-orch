# SQLite 迁移

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
