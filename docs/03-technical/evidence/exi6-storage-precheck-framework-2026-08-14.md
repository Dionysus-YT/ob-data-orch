# EX-I6 存储专用预检查框架与门禁打通（2026-08-14）

> 验证状态：合成验证通过（Go/前端全部门禁）。真实对象存储网络连接、真实凭据解析与四类存储取证未执行，归 EX-V1 排期。
> 安全说明：本文不记录真实端点、身份、密码、密钥、完整命令或工具输出；示例值均为合成数据。

## 1. 交付内容

存储专用预检查框架（EX-I6 第三段）与存储凭据槽位（第二段）的审查收口、OpenAPI/前端同步：

### 1.1 检查清单与契约

- `internal/precheckcontract`：新增 `STORAGE_CONNECTIVITY`、`STORAGE_AUTH` 两项检查与受控证据码（PASSED/FAILED/UNKNOWN 各一条专属码 + SYNTHETIC_OK）；冻结六项与 `FixedChecks()` 语义不变。
- `internal/agentpreflight`：对象存储输出的“存储形态清单”固定为 `DATABASE_CONNECTIVITY → OBJECT_ACCESS → TOOL_ENVIRONMENT → AVAILABLE_SPACE → STORAGE_CONNECTIVITY → STORAGE_AUTH`；本地输出保持冻结六项。`Run` 保持 local-first：本机前置失败时数据库与存储检查全部 UNKNOWN 且不解析槽位。`ValidateReportFor(kind, report)` 按输出类型拒绝形态混淆。

### 1.2 控制面与仓储

- 预检查上下文新增 `OutputKind` 与 `StorageTarget{Provider, URI, Endpoint, TmpPath}`：对象存储草稿从冻结 v6 配置解析（URI scheme 白名单、参数白名单 endpoint/region、拒绝密钥参数），URI 不经浏览器重写。
- `claim-next` 按输出类型下发检查清单与存储目标段；`complete` 在事务内按冻结草稿的输出类型复核结果形态（形态混淆 → 租约拒绝）。
- 提交门禁由结果驱动：对象存储输出要求两项存储检查均 PASSED，否则 `STORAGE_PRECHECK_REQUIRED`；`STORAGE_PRECHECK_UNAVAILABLE` 硬门禁已移除。

### 1.3 Agent 探测

- `internal/agentlocalpreflight`：`TCPStorageConnectivityProber` 只对受控 endpoint（host[:port]，默认 443）执行单次 TCP 建连，不携带凭据或业务数据；仅在显式运行开关 `OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true` 时装配，默认 UNKNOWN。
- `UnavailableStorageAuthProber`：凭据有效性探测的固定失败关闭实现；真实探测（四类云厂商签名协议）归 EX-V1，本切片绝不解析或发送真实凭据。
- AVAILABLE_SPACE 对对象存储输出转向 `--tmp-path` 所在卷；未指定时 UNKNOWN 失败关闭。

### 1.4 存储凭据槽位审查收口（第二段）

- 修复执行解密 AAD 信封标识：`ResolveExecutionStorageCredential` 返回两个加密信封自身的 credentialId，控制面解密以其重建 AAD（此前误用 storageCredentialId，AES-GCM 校验必然失败）。
- 修复创建幂等重放：同幂等键不同内容改为 `IDEMPOTENCY_CONFLICT`，不再静默返回旧资源。
- 三个写端点（create/rotate/delete）补齐 CSRF 失败关闭；rotate 入口校验幂等键格式（缺失不再落为仓储层 500）。
- Agent 侧短时存储凭据段在协议解析层校验 provider 白名单、长度与禁止字节；core-site.xml 生成拒绝 NUL。

### 1.5 OpenAPI 与前端

- OpenAPI：`/api/v1/storage-credentials` 三路径与 5 个 schema/4 个响应（密钥 writeOnly、provider 白名单、If-Match/幂等键声明、no-store）；`AgentPrecheckCheckSet/Results` 改为本地/存储双形态 oneOf，新增两项结果 schema 与上下文 `outputKind/storageTarget` 段。
- 前端：存储凭据管理页（列表/创建/轮换/删除，密钥只写不回显）、向导步骤 5 凭据绑定（provider 匹配、修订取 currentRevision、本地输出携带即失败关闭）、预检查展示接入两项存储检查（未授权探测显示“未完成（探测未授权）”）；对象存储草稿可发起预检查，提交按钮按结果禁用。

## 2. 验证记录

```powershell
go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...
./scripts/check-secrets.ps1
git diff --check
gofmt -l internal\ cmd\

Set-Location web
npm run lint
npm run typecheck
npm run test
npm run build
```

结果：Go 全量测试/vet、密钥扫描、gofmt、差异检查、Windows AMD64/Linux AMD64/Linux ARM64 无 CGO 构建与前端 lint/typecheck/18 文件 134 项测试/生产构建全部通过。完整 `scripts/verify.ps1` 因运行中 Vite 占用原生 DLL 会在 `npm ci` 停止，本次按等价分段门禁执行。

新增合成正负例覆盖：存储形态编排与前置失败 UNKNOWN、形态混淆拒绝、存储目标失败关闭、端点解析、连通性/凭据探测投影、tmp-path 卷空间、v6 存储上下文解析与负例、完成端点形态复核、控制面提交门禁正反例（存储检查 UNKNOWN → 422；PASSED → 201）、agentwire 存储租约解析与客户端宽松复核、OpenAPI 双形态与存储凭据面、前端草稿引用往返与凭据表单校验。

## 3. 边界与未完成

- 两项存储探测默认 UNKNOWN：真实网络连接与真实凭据解析受 AGENTS.md 授权约束，未执行；开启 TCP 连通性开关仍需当次授权（EX-V1）。
- STORAGE_AUTH 的真实探测协议（四类云厂商签名）未实现，只保留接口与失败关闭实现。
- 四类存储（OSS/S3/COS/OBS）真实导出取证、对象存储任务提交后的端到端执行未验证。
- 对象存储任务在两项存储检查通过前保持提交阻断；这不是把“未验证”伪装成“可用”。
