# API 与数据模型 SQLite 约束最小验证

> 验证日期：2026-07-21
> 验证环境：Windows AMD64、Go 1.26.4、modernc.org/sqlite v1.54.0
> 结论：核心关系约束候选可行；正式 20 表迁移、API 和目标麒麟运行仍待实现验证

## 1. 目的与边界

本次验证只回答 SQLite 能否直接保护以下关键不变量：

- 草稿 revision 乐观并发；
- 提交任务快照不可修改；
- 同一配置指纹允许两个独立任务；
- 同一 task 并发领取只有一个 execution；
- execution 事件必须绑定正确 lease/epoch 且序号唯一；
- 同一幂等键不能绑定不同请求摘要；
- 同一日志序号范围不能写入不同内容摘要；
- 业务事实和审计事实能够同事务回滚。

验证夹具位于 Git 忽略的 `tmp/api-data-model-spike/`。它使用合成 ID、合成密文字节和空业务 JSON，不读取真实凭据、现有 P0 数据库或 OBDUMPER 日志，不属于业务代码或正式迁移。

## 2. 验证模型

夹具只建立与约束验证有关的 12 张缩减表：

```text
data_sources
credential_revisions
export_drafts
precheck_runs
tasks
task_executions
execution_leases
execution_events
request_idempotency
audit_events
log_streams
log_batches
```

这不是正式表集。权限、节点身份、日志段和迁移表等只在正式契约中定义，不能从夹具缺失推断为不需要。

## 3. 运行配置

| 项目 | 值 |
|---|---|
| SQLite | 3.53.3 |
| journal mode | WAL |
| synchronous | FULL |
| foreign keys | 1 |
| busy timeout | 5 秒 |
| 数据库大小 | 131,072 字节 |

夹具 SHA-256：

| 文件 | SHA-256 |
|---|---|
| `main.go` | `3D5A4DC90ACFE98B457111F7952242F9CBAA6E8BE625B798A0FE1230C71D27E3` |
| `go.mod` | `369C088B03571D93176A5465CB34D08DE98E1A43F00CFDA5B94F00DFE04F3A1F` |
| `go.sum` | `66DB7CFA01866EB864E367E804B69EBAD9B716CB1C73B1D5558C081987C658BF` |
| 验证数据库 | `D8AB2A72750DD4F3570F569CE344A4193E055BB056B29EBA5F25809C60279852` |

## 4. 验证结果

| 检查 | 结果 |
|---|---|
| 业务+审计事务回滚不留数据源 | 通过 |
| 草稿正确 revision 更新一行 | 通过 |
| 草稿过期 revision 更新零行 | 通过 |
| task 快照更新被触发器拒绝 | 通过 |
| 相同 fingerprint 创建两个 task | 通过 |
| 两个并发领取同一 task | 只有一个成功 |
| 重复 execution eventSeq | 被唯一约束拒绝 |
| 错误 lease 的事件 | 被复合外键拒绝 |
| 同幂等键不同请求摘要 | 被复合主键/冲突规则拒绝 |
| 同日志首序号不同摘要 | 被唯一约束拒绝 |
| `PRAGMA integrity_check` | `ok` |

## 5. 得到的设计结论

- task 提交快照和运行状态必须分表；否则无法同时保证快照不可变和状态更新；
- `configFingerprint` 不是唯一业务键，两次明确提交相同配置必须获得不同 taskId；
- taskId 在 task_executions 上唯一即可让两个并发 Agent 只有一个领取成功；
- execution 事件复合外键必须包含 executionId、leaseId 和 leaseEpoch，只校验 executionId 不足；
- 乐观锁可以直接用带 revision 条件的 UPDATE 影响行数判断；
- 审计和业务事实在同一短事务内保存，不需要第二套消息系统；
- 约束只能拒绝冲突；API 仍需识别“完全相同重发”并返回原确认，而不是把所有唯一冲突都显示成失败。

## 6. 尚未验证

- 正式 20 表迁移和完整 API 数据形状；
- `EXPORT_PREFLIGHT` 的 Agent 领取、短租约、秘密槽位和结果回传；
- 事件写入与状态投影的完整状态机事务；
- 日志段先 fsync、再登记 SQLite 及崩溃恢复；
- 权限变更、无权枚举、身份停用和首次管理员；
- 高并发心跳/事件/查询下的 busy 行为；
- schema 升级、校验和、Online Backup/VACUUM INTO 与应用回退；
- Windows 服务和三个麒麟目标的真实运行。

AD-R01～AD-R20 已确认；本证据只证明其中核心 SQLite 约束候选在 Windows 合成环境可行，不代表正式 API/数据模型门禁、VS-P0 或业务代码开发准入已经通过。
