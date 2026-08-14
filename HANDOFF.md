# OB Data Orch 交接说明

> 快照日期：2026-08-14
> 权威状态：[开发任务地图](docs/03-technical/development-task-map.md)、[开发准入收口](docs/03-technical/development-readiness-closure.md) 与对应验证证据
> 安全说明：不记录真实端点、身份、密码、密钥、完整命令、输出路径或工具原始输出。

## 当前结论

项目已具备 Windows 本机 Local MVP 的受控固定单表 CSV 导出链路，以及任务、详情和持久日志的授权读取。该能力现统一标记为 `CSV_SINGLE_TABLE_V1`，只是导出模块的已实现基线，不代表完整导出模块、G3、WI-01～WI-12、麒麟目标或生产发布通过。

开发路线已从“继续逐项完成 WI 现场验证”切换为“先完成导出模块全功能设计与技术契约，再按能力切片实现和验证”。**EX-D0**、**EX-D1**、**EX-D2**、**EX-I1 通用导出骨架**、**EX-I2 对象范围与 DDL**、**EX-I3 CSV 完整能力**、**EX-I4 CUT、SQL 与 POS**、**EX-I5 Parquet/ORC/Avro** 均已完成；**EX-I6 第一段**（对象存储受控 URI、--tmp-path、输出类型 LOCAL/OSS/S3/COS/OBS 全链路）已交付；**EX-I7 剩余参数第二批**已完成受控实测、产品接入与代码审查收口。当前任务地图仍以 **EX-I6 后续切片**为下一工程动作（存储凭据槽位、存储专用预检查；真实对象存储验证需授权，归 EX-V1 排期）。

真实连接、预检查和工具启动默认失败关闭。它们只能在回环 Local MVP、显式运行开关、登记对象、受认证 Agent、固定任务信封及当次用户授权同时满足时发生；控制面不提供任意命令、SQL、路径浏览或远程 Shell。

## 本次代码审查收口（2026-08-14）

- 审查结论：本次改动中的可操作问题已全部修复，最终复核未发现新的阻断性问题。
- 元数据兼容：新泛化草稿使用 v7；历史 v6 目录继续只用于已持久化草稿的预览、预检查与提交重放；冻结 v5 单表 CSV 保持原有 argv 与指纹语义。v7 修正 `--table` 被错误发射为 `-t` 的问题，历史 v6 不被静默改写。
- 路径边界：控制面不再从导出目录派生日志目录；空 `logPath` 保持 OBDUMPER 默认行为，显式节点路径只校验并原样传递。
- 第二批能力：仅启用已观察到行为效果的 MySQL `--date-value-format`、`--datetime-value-format`、`--partition` 与 `--exclude-data-types`。其余七个时间格式、`--enable-hidden-pk`、`--add-extra-message` 继续失败关闭；前端、OpenAPI、Go 类型、元数据、测试和文档已同步。
- 输入边界：时间格式只接受受限可见 ASCII 字符与普通空格，制表符、回车、换行及首尾空白均拒绝；分区名和类型名使用受限字符集并有数量、长度与适用范围门禁。
- 工作区收口：根目录 Vitest 缓存已删除，根 `node_modules` 已加入忽略规则；未提交密钥、数据库文件、工具输出或 `.codegraph/` 工件。

## 已核对的现役能力

- 数据源、执行节点、受认证 Agent、固定 JDBC 基础连接测试、六项 `EXPORT_PREFLIGHT`、受控 OBDUMPER 执行、状态投影与任务日志均有对应代码、契约和测试。
- 任务中心 `/tasks` 仅读取本人创建或按数据源明确授权的任务。它使用主体绑定游标分页，支持每页 10、20、50 条，并显示当前页和只按同一授权范围派生的总页数；不返回全局任务总数、配置、命令、错误原文或无权对象。
- 日志读取使用已持久化双层脱敏记录、主体/任务绑定游标和 SSE 续传；已确认的非秘密运行上下文按日志契约保留，秘密继续遮蔽。
- 任务地图保留单一真实成功路径、受控 Agent 中断后的“状态核对中”投影、终态日志补传恢复、队列与 SSE 的合成/局部现场证据；这些记录不再决定完整导出模块的设计顺序。

## 门禁与下一步

- **G3 仍未通过。** WI-03 低权限对象负例已通过；WI-04 CSV 特殊值已由两次独立正式 Agent 导出和测试负责人人工确认通过。长期不可达、跨目标环境、正式认证/备份恢复及其余 WI 收口仍待完成。
- 终态日志补传协议已完成：只接受同一 Agent、原租约 epoch、重新计算的冻结信封摘要与 `RELEASED + SUCCEEDED/FAILED` 的持久化批次/缺口；它不恢复执行、不延长租约、不改变任务终态，且错误摘要、其他 Agent 与过期租约均拒绝。相关仓储测试已通过。
- 已完成现场重启回归：此前实际终态任务留下的 1 个已 `fsync` 待确认批次，经当前源码构建的独立标准 Agent 重启补传后清至 0；账本末尾为 `RECOVERY_REPLAY_ATTEMPT`、`CONTROL_PLANE_CONFIRMED`。全程未启动工具、未重新连接数据库、未停止常驻 Agent，详见[Windows 终态日志补传重启验证](docs/03-technical/evidence/windows-terminal-log-replay-recovery-validation-2026-08-04.md)。
- 当前下一工程动作以任务地图为准：执行 EX-I6 后续切片（存储凭据槽位与存储专用预检查；真实对象存储验证需授权，归 EX-V1 排期）。EX-I7 第二批已完成产品接入与审查收口，但未完成的 TIME/TIMESTAMP/Oracle 行为、隐藏主键预检查和附加对象信息权限链不得据此解锁。对象存储草稿在存储专用预检查完成前，固定预检查与提交保持功能门禁阻断（STORAGE_PRECHECK_UNAVAILABLE），不把存储 URI 伪装成本地路径。
- WI-05 已完成第一阶段只读核对，但现场进程 argv 取证暂停；它保持未通过，并在对应能力进入 EX-V1 时继续。未取得新的真实工具启动授权前，不再次启动 OBDUMPER。
- F3 的列表、详情和日志读取是已实现的只读能力，不替代 G3 结论。筛选、跨任务日志检索、下载、取消、重试和普通/旁路导入仍不在当前切片范围内。

## 本机运行态与验证

本次审查时观察到前端 Vite 开发服务器正在运行。它占用了 Rolldown 原生模块，导致完整门禁中的 `npm ci` 无法清理该文件；未擅自停止用户进程。该运行态不是部署或生产可用声明。

最近一次本地验证包括：

```powershell
go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...
./scripts/check-secrets.ps1
git diff --check

Set-Location web
npm run lint
npm run typecheck
npm run test
npm run build
```

结果：Go 全量测试、`go vet`、Windows AMD64/Linux AMD64/Linux ARM64 无 CGO 构建、前端 lint/typecheck/17 文件 123 项测试/生产构建、密钥扫描、Go 格式与差异检查均通过。`scripts/verify.ps1` 在完成 Go 与多平台构建后，于 `npm ci` 因运行中 Vite 占用原生 DLL 而停止；随后用锁定版本恢复依赖，`package-lock.json` 哈希保持不变，并单独通过全部前端门禁。若需要一条完整 clean-install 门禁记录，应先正常停止 Vite，再重新执行 `scripts/verify.ps1`。

本轮代码审查未重新连接真实数据库、未启动 OBDUMPER，也未新增真实执行证据；第二批行为结论沿用 [2026-08-13 受控实测证据](docs/03-technical/evidence/exi7-remaining-parameters-2026-08-13.md)。不能将合成测试、交叉构建或单一成功路径写成完整现场验收。

## 工作区与协作边界

- 根目录 `AGENTS.md` 是当前项目规则真身；未发现项目内更近的规则文件。它要求保留无关改动、禁止重置覆盖、中文注释、失败关闭、双层脱敏和按门禁报告真实验证。
- CodeGraph 缓存较源码旧；不重建、不同步、不提交。定位与结论以当前源码、契约、测试和现场验证为准。
- 平台生成记忆未获写入授权，本次不写入长期记忆。
- 本地 `agent`、`agent.exe`、`control-plane`、`control-plane.exe` 和 `.playwright-cli/` 是未跟踪运行/调试工件，未纳入 Git。它们保留供复核；删除必须在收口报告后另获明确确认。
