# EX-I8 结果、失败恢复与复用（2026-08-14）

> 验证状态：合成验证通过（Go/前端全部门禁）。真实 `dump.ckpt` 续跑取证、结果清单行数/校验和解析与模板复用未完成，归 EX-V1/后续补充切片。
> 安全说明：本文不记录真实端点、身份、密码、密钥、完整命令或工具输出；示例值均为合成数据。

## 1. 交付内容（8-A/8-B/8-C 三段）

### 8-A 结果与失败事实

- Agent 成功与失败路径都上报受控结果事实：文件数/字节数/受限相对路径清单（≤100 项、≤512 字符、禁止绝对前缀/回退段/控制字符）+ `dump.ckpt` 存在性。失败路径在 `TOOL_TERMINAL_OBSERVED(FAILED)` 之后作为迟到事实上报，控制面按同一 Agent/租约/连续序号在接受失败终态已释放租约时接收。
- 控制面把 `RESULT_FACTS_OBSERVED` 合并为 `task_executions.result_summary_json`（`result/fileCount/totalBytes/files/checkpointPresent/observedAt`），任务详情 `/execution` 投影 `resultSummary` 安全摘要；前端新增“执行结果”区与失败操作区。结果清单不含内容、行数或校验和（未取证，不伪造）。

### 8-B 派生任务与两类派生草稿

- 迁移 0018：`tasks.derivation_kind`（REBUILD_FROM_CONFIG/RERUN_FROM_SCRATCH/CHECKPOINT_RESUME）+ `export_drafts.source_task_id/source_derivation`（`parent_task_id` 沿用 0014）。
- `POST /tasks/{id}:rebuild-draft`：从失败任务冻结快照重建可编辑 v6 草稿，不复制凭据明文/预检查/风险确认/日志；REBUILD_FROM_CONFIG 可改参，RERUN_FROM_SCRATCH 提交时服务端强制 configFingerprint 与来源任务一致（422 RERUN_CONFIGURATION_CHANGED）。
- 提交派生草稿时写入 `parent_task_id/derivation_kind`；任务概览投影来源关系（parentTaskId/derivationKind）。

### 8-C 检查点继续

- `POST /tasks/{id}:resume-checkpoint`：资格 = 失败终态 + 结果摘要确认 dump.ckpt 存在 + 原预检查 SUCCEEDED/COMPLETE；新任务继承原快照并追加官方 `--retry`（服务端固定构造，不新增参数元数据版本），不重新预检查。
- 继续任务的领取执行复验数据源/凭据/节点/Agent 事实版本，豁免预检查 TTL（检查点内含继续语义）；不满足条件返回稳定门禁错误（CHECKPOINT_RESUME_UNAVAILABLE / CHECKPOINT_PRECHECK_UNAVAILABLE）。
- 前端失败任务操作区：基于原配置新建 / 从头重新执行 / 条件化从检查点继续；向导支持派生草稿加载与表单回填（REBUILD 可编辑，RERUN 直接进入预检查步骤）。

## 2. 验证记录

```powershell
go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...
./scripts/check-secrets.ps1
git diff --check
gofmt -l cmd contracts internal migrations

Set-Location web
npm run lint
npm run typecheck
npm run test
npm run build
```

结果：Go 全量测试/vet（31 包）、密钥扫描、gofmt、差异检查与前端 lint/typecheck/18 文件 136 项测试/生产构建全部通过。

新增合成正负例覆盖：结果/检查点事件负载边界、失败终态后迟到事实接受、结果摘要合并与授权投影、派生草稿重建（正例/非失败/未知任务/快照版本）、从头执行指纹不可变、检查点继续（正例/无检查点/链式继续拒绝/幂等重放）、任务概览派生关系、OpenAPI 派生面与结果摘要安全投影、前端派生客户端与解析。

## 3. 边界与未完成

- 真实 `dump.ckpt` 续跑取证（失败中断制造保存点后继续）归 EX-V1；当前为合成验证。
- 结果清单只含相对路径与字节数；行数、校验和与格式特征未解析。
- 获准范围内的模板复用（从成功任务保存模板/模板创建草稿）为下一补充切片。
- 派生操作仅对失败任务提供；成功任务的“保存为模板”随模板复用切片实现。
