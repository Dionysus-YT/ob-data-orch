# OB Data Orch 交接说明

> 快照日期：2026-08-14
> 文档状态：历史交接快照，不是当前运行态或发布状态事实源。进程 PID、服务是否运行、授权窗口和“下一步”均可能已过期；当前事实以 [开发任务地图](docs/03-technical/development-task-map.md)、[Export 产品/交互 Canonical](docs/02-design/export-module.md)、[Export 技术契约](docs/03-technical/export-general-contract.md)、[受控验证计划](docs/02-design/export-controlled-validation.md) 和现场验证为准。
> 权威状态：[开发任务地图](docs/03-technical/development-task-map.md)、[开发准入收口](docs/03-technical/development-readiness-closure.md) 与对应验证证据
> 安全说明：不记录真实端点、身份、密码、密钥、完整命令、输出路径或工具原始输出。

## 当前结论

项目已具备 Windows 本机 Local MVP 的受控固定单表 CSV 导出链路，以及任务、详情和持久日志的授权读取。该能力现统一标记为 `CSV_SINGLE_TABLE_V1`，只是导出模块的已实现基线，不代表完整导出模块、G3、WI-01～WI-12、麒麟目标或生产发布通过。

开发路线已从“继续逐项完成 WI 现场验证”切换为“先完成导出模块全功能设计与技术契约，再按能力切片实现和验证”。**EX-D0**、**EX-D1**、**EX-D2**、**EX-I1 通用导出骨架**、**EX-I2 对象范围与 DDL**、**EX-I3 CSV 完整能力**、**EX-I4 CUT、SQL 与 POS**、**EX-I5 Parquet/ORC/Avro** 均已完成；**EX-I6** 已交付三段——第一段（对象存储受控 URI、--tmp-path、输出类型 LOCAL/OSS/S3/COS/OBS 全链路）、第二段（存储凭据槽位：迁移 0017 + 控制面 CRUD/轮换/删除 + 草稿引用 + 提交绑定 + Agent 同租约槽位解析 + core-site.xml 生成与 HADOOP_CONF_DIR 短时注入）、第三段（存储专用预检查框架与提交门禁打通 + OpenAPI/前端同步，2026-08-14，详见 [EX-I6 存储预检查框架证据](docs/03-technical/evidence/export-increment-validation.md#exi6-storage)）。**EX-I7 剩余参数第二批**已完成受控实测、产品接入与代码审查收口。**EX-I8** 已交付三段（2026-08-14，合成验证）：8-A 结果与失败事实（Agent 成功/失败路径上报受控结果事实与 dump.ckpt 存在性，控制面合并 result_summary_json 并投影到任务详情）、8-B 派生任务与两类派生草稿（迁移 0018 + `POST /tasks/{id}:rebuild-draft` 基于原配置新建/从头重新执行 + 提交写入 parent/derivation）、8-C 检查点继续（`POST /tasks/{id}:resume-checkpoint` 继承原快照追加 --retry，不重新预检查），详见 [EX-I8 证据](docs/03-technical/evidence/export-increment-validation.md#exi8-recovery)。剩余工程动作以任务地图为准：**下一补充切片为 EX-I8 模板复用收口**（从成功任务保存模板/模板创建草稿，模板不复用凭据、节点、预检查或风险确认）；EX-I6 两项存储探测真实执行、四类存储端到端取证与 dump.ckpt 续跑取证需真实外网/工具授权，归 **EX-V1** 排期；对象存储任务在两项存储检查 PASSED 前保持提交阻断。

真实连接、预检查和工具启动默认失败关闭。除本机 MVP/Agent 包启动器已持续授权的受控存储 TCP 探针外，它们只能在回环 Local MVP、显式运行开关、登记对象、受认证 Agent、固定任务信封及当次用户授权同时满足时发生；控制面不提供任意命令、SQL、路径浏览或远程 Shell。

## 本次代码审查收口（2026-08-14）

- 审查结论：本次改动中的可操作问题已全部修复，最终复核未发现新的阻断性问题。
- 元数据兼容：新泛化草稿使用 v7；历史 v6 目录继续只用于已持久化草稿的预览、预检查与提交重放；冻结 v5 单表 CSV 保持原有 argv 与指纹语义。v7 修正 `--table` 被错误发射为 `-t` 的问题，历史 v6 不被静默改写。
- 路径边界：控制面不再从导出目录派生日志目录；空 `logPath` 保持 OBDUMPER 默认行为，显式节点路径只校验并原样传递。
- 第二批能力：仅启用已观察到行为效果的 MySQL `--date-value-format`、`--datetime-value-format`、`--partition` 与 `--exclude-data-types`。其余七个时间格式、`--enable-hidden-pk`、`--add-extra-message` 继续失败关闭；前端、OpenAPI、Go 类型、元数据、测试和文档已同步。
- 输入边界：时间格式只接受受限可见 ASCII 字符与普通空格，制表符、回车、换行及首尾空白均拒绝；分区名和类型名使用受限字符集并有数量、长度与适用范围门禁。
- 工作区收口：根目录 Vitest 缓存已删除，根 `node_modules` 已加入忽略规则；未提交密钥、数据库文件、工具输出或 `.codegraph/` 工件。

## 存储凭据槽位实现（EX-I6 第二段，2026-08-14，已审查收口）

- 数据与存储层：迁移 0017 新增 `storage_credentials`（主体级主表：displayName/provider/current_revision/revision）与 `storage_credential_revisions`（access-key 与 secret-key 分别加密的独立信封表，AAD 绑定 storageCredentialId）；`tasks` 增加 `storage_credential_id`/`storage_credential_revision` 列。credential 包新增 `STORAGE_ACCESS_KEY`/`STORAGE_SECRET_KEY` 秘密槽位类型。
- 控制面：`GET/POST /api/v1/storage-credentials`、`POST :rotate`、`DELETE`（幂等、If-Match 乐观锁、越权 404 不泄露存在性、三处写端点 CSRF 失败关闭、rotate 幂等键格式入口校验）；密钥明文只在请求体内存中，加密为信封后立即清零，摘要只含显示字段与密钥存在性。草稿归一化接受对象存储输出绑定的凭据引用（LOCAL 携带即 422）；提交冻结前校验引用存在、属于当前主体、provider 与输出类型一致、修订未过期。
- Agent：执行槽位解析在同一 `secret-slots:resolve` 请求内附带 `storageCredential` 段；Agent 在 execution 私有目录生成 core-site.xml（四类 provider 官方属性名 + XML 转义 + 拒绝 NUL）并通过 `HADOOP_CONF_DIR` 短时注入，密钥绝不进入 argv、日志、长期环境变量或状态文件。Agent 侧在协议解析层校验 provider 白名单、长度与禁止字节。
- 审查修复：执行解密重建 AAD 时改用两个加密信封自身的 credentialId（此前误用 storageCredentialId，解密必然失败）；创建幂等重放补回请求摘要比对（同键异内容改为冲突）；存储槽位协议校验提前到解析层。
- OpenAPI/前端已同步：`/api/v1/storage-credentials` 三路径与 schema/响应（密钥 writeOnly、provider 白名单、no-store）；存储凭据管理页（`/settings/storage-credentials`）与导出向导步骤 5 凭据绑定（provider 匹配、修订取 currentRevision）。

## 存储专用预检查框架（EX-I6 第三段，2026-08-14）

- 检查清单：对象存储输出使用存储形态清单 `DATABASE_CONNECTIVITY → OBJECT_ACCESS → TOOL_ENVIRONMENT → AVAILABLE_SPACE（--tmp-path 卷，未指定 UNKNOWN）→ STORAGE_CONNECTIVITY → STORAGE_AUTH`；本地输出保持冻结六项。local-first 不变：本机前置失败时数据库与存储检查全部 UNKNOWN 且不解析槽位。
- 上下文与协议：预检查上下文新增 `OutputKind` 与 `StorageTarget{Provider, URI, Endpoint, TmpPath}`（受控 URI 白名单解析，拒绝密钥参数）；claim 按输出类型下发检查清单；complete 在事务内按冻结草稿复核形态（混淆 → 租约拒绝）。
- 门禁打通：对象存储草稿可发起预检查；提交门禁改为结果驱动——两项存储检查均 PASSED 才可冻结任务，否则 `STORAGE_PRECHECK_REQUIRED`。旧 `STORAGE_PRECHECK_UNAVAILABLE` 硬门禁已移除。
- Agent 探测：`TCPStorageConnectivityProber` 只对受控 endpoint 单次 TCP 建连（默认 443），仅显式开关 `OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true` 时装配；本机 MVP/Agent 包启动器固定开启，运行启动器即对该进程生命周期内的受控 TCP 探针持续授权。`STORAGE_AUTH` 只保留接口与失败关闭实现（真实凭据探测归 EX-V1）；未开启连通性开关时两项保持 UNKNOWN，提交保持阻断。
- 未完成：STORAGE_AUTH 的授权真实执行、四类存储端到端验证与对象存储远端 MANIFEST 取证，均归 EX-V1。MANIFEST 接入前，受控进程退出码 0 与固定成功终态组合可投影 VERIFIED，但文件数、字节数、清单和检查点保持空值。

## 已核对的现役能力

- 数据源、执行节点、受认证 Agent、固定 JDBC 基础连接测试、六项 `EXPORT_PREFLIGHT`、受控 OBDUMPER 执行、状态投影与任务日志均有对应代码、契约和测试。
- 任务中心 `/tasks` 仅读取本人创建或按数据源明确授权的任务。它使用主体绑定游标分页，支持每页 10、20、50 条，并显示当前页和只按同一授权范围派生的总页数；不返回全局任务总数、配置、命令、错误原文或无权对象。
- 日志读取使用已持久化双层脱敏记录、主体/任务绑定游标和 SSE 续传；已确认的非秘密运行上下文按日志契约保留，秘密继续遮蔽。
- 任务地图保留单一真实成功路径、受控 Agent 中断后的“状态核对中”投影、终态日志补传恢复、队列与 SSE 的合成/局部现场证据；这些记录不再决定完整导出模块的设计顺序。

## 门禁与下一步

- **G3 仍未通过。** WI-03 低权限对象负例已通过；WI-04 CSV 特殊值已由两次独立正式 Agent 导出和测试负责人人工确认通过。长期不可达、跨目标环境、正式认证/备份恢复及其余 WI 收口仍待完成。
- 终态日志补传协议已完成：只接受同一 Agent、原租约 epoch、重新计算的冻结信封摘要与 `RELEASED + SUCCEEDED/FAILED` 的持久化批次/缺口；它不恢复执行、不延长租约、不改变任务终态，且错误摘要、其他 Agent 与过期租约均拒绝。相关仓储测试已通过。
- 已完成现场重启回归：此前实际终态任务留下的 1 个已 `fsync` 待确认批次，经当前源码构建的独立标准 Agent 重启补传后清至 0；账本末尾为 `RECOVERY_REPLAY_ATTEMPT`、`CONTROL_PLANE_CONFIRMED`。全程未启动工具、未重新连接数据库、未停止常驻 Agent，详见[Windows 终态日志补传重启验证](docs/03-technical/evidence/windows-validation.md#log-replay)。
- 当前下一工程动作以任务地图为准。**EX-I6 三段、EX-I8 四段（含模板复用）均已交付（2026-08-14）**；EX-I8 模板链路已对真实本机数据端到端验证（保存/列表/建草稿 201、旧失败任务继续失败关闭）。**EX-V1 真实取证已授权并部分执行**：Agent 已装载存储连通性探测开关（`OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true`）；剩余项需真实材料——dump.ckpt 续跑取证（需大数据量/慢导出制造保存点）、两项存储探测真实端点执行、四类存储端到端导出（需用户提供各厂商真实 endpoint 与凭据）。EX-I7 第二批已完成产品接入与审查收口，但未完成的 TIME/TIMESTAMP/Oracle 行为、隐藏主键预检查和附加对象信息权限链不得据此解锁。对象存储草稿可以发起预检查；任务提交在 STORAGE_CONNECTIVITY/STORAGE_AUTH 两项均 PASSED 前保持失败关闭（STORAGE_PRECHECK_REQUIRED），不把存储 URI 伪装成本地路径。
- WI-05 已完成第一阶段只读核对，但现场进程 argv 取证暂停；它保持未通过，并在对应能力进入 EX-V1 时继续。未取得新的真实工具启动授权前，不再次启动 OBDUMPER。
- F3 的列表、详情和日志读取是已实现的只读能力，不替代 G3 结论。筛选、跨任务日志检索、下载、取消、重试和普通/旁路导入仍不在当前切片范围内。

## 本机运行态与验证

### 本机二进制刷新（2026-08-14 晚，按 AGENTS.md 6.1 流程）

新规则已写入根 `AGENTS.md` §6.1：Agent/控制面二进制变更必须“先停旧服务 → 删旧二进制 → 构建新二进制 → 按原开关重启并验证”，旧配套脚本已由 DEC-048 的安装包单一“启动”入口替代，构建仅使用 `scripts/build-package.ps1`；当前整改与验证状态见 `docs/03-technical/deployment-operations.md`。

本次按该流程完成全量刷新（停旧 → 删旧 → 重建 → 重启 → 验证）：

- 控制面：`var\local-mvp-tls\control-plane-local-mvp.exe`（PID 29072，新构建 15:57:29，`--local-mvp`，回环 8080，HTTPS `/api/v1/data-sources` 返回 200）。
- 常驻 Agent：`var\ob-data-orch-agent-windows-amd64\agent.exe`（PID 25776，最新构建；同一启动开关 `OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST/EXPORT_PREFLIGHT/REAL_EXECUTION=true`），心跳 `ONLINE`（lastHeartbeat 16:03 UTC），身份目录 `agent-security` 与 TLS 材料未改动。
- 页面代理：vite（PID 14340，5173），`http://localhost:5173/api/v1/data-sources` 返回 200。
- 旧 Agent 运行日志已保留为 `agent.stdout.log.previous-20260814[-2]`；历史 `control-plane-local-mvp.exe~` 旧副本已删除。

运行分支的“停→删→装→重启”路径已实际演练通过；首次演练暴露的“杀进程后文件仍短暂被占用”问题已通过删除重试循环修复。

### 验证记录

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

结果：Go 全量测试、`go vet`、密钥扫描、`gofmt`、`git diff --check`、Windows AMD64/Linux AMD64/Linux ARM64 无 CGO 构建与前端 lint/typecheck/18 文件 134 项测试/生产构建全部通过。完整 `scripts/verify.ps1` 仍会因运行中 Vite 占用原生 DLL 而在 `npm ci` 停止；若需要一条完整 clean-install 门禁记录，应先正常停止 Vite，再重新执行。

本轮代码审查未重新连接真实数据库、未启动 OBDUMPER，也未新增真实执行证据；第二批行为结论沿用 [2026-08-13 受控实测证据](docs/03-technical/evidence/export-increment-validation.md#exi7-batch2)。不能将合成测试、交叉构建或单一成功路径写成完整现场验收。

## 工作区与协作边界

- 根目录 `AGENTS.md` 是当前项目规则真身；未发现项目内更近的规则文件。它要求保留无关改动、禁止重置覆盖、中文注释、失败关闭、双层脱敏和按门禁报告真实验证。
- CodeGraph 缓存较源码旧；不重建、不同步、不提交。定位与结论以当前源码、契约、测试和现场验证为准。
- 平台生成记忆未获写入授权，本次不写入长期记忆。
- 已获授权并执行：本地 `agent`、`agent.exe`、`control-plane`、`control-plane.exe` 四个被跟踪二进制已 `git rm --cached` 移出索引并加入 `.gitignore`（构建产物不再入库；本地按 AGENTS.md 6.1 删除后重建）。
- 本地 `.playwright-cli/` 仍为未跟踪运行工件。
