# 旁路导入 API 验证与单机连接核查 · 2026-10-22

> 后续状态：用户采纳修复方向后，DEC-058 的单机链路已经实现、安装并完成有界真实 API 验收；见[后续修复记录](direct-load-standalone-repair-2026-10-22.md)。本文及其 JSON 保留修复前的零任务与阻断事实，不代表修复后的现役结论。

**当前结论：API 已实际登录并测试；全量真实旁路 E2E 仍为 BLOCKED。** 最新批次 34 项记录中 31 项 PASS、3 项 BLOCKED、0 项 FAIL；另有 7 项未登录 HTTPS 边界通过。没有提交旁路任务或启动官方 OBLOADER 写入，不能将文件发现、拒绝提交和源码回归合并称为完整导入通过。

用户授权持续有效，且本轮明确要求使用 API。正常 `/login`、受信本机 CA、Secure/HttpOnly/SameSite Cookie、CSRF、幂等键与修订均沿用产品协议；口令仅经子进程匿名 stdin 短时传递，登录后清零并注销本次会话，没有复制浏览器 Cookie、绕过登录或更换身份。

## 实际结果

机器摘要见 [results.json](direct-load-api-2026-10-22-results.json)。最新原始回执与所有权日记在忽略目录 `artifacts/direct-load-e2e-20261022-8453750f71d8/`。

| 范围 | 结果与证据边界 |
| --- | --- |
| 元数据与读取 | 103 项冻结元数据、草稿列表、创建/读取/修订通过；未完成草稿不代表可执行 |
| 安全与并发 | CSRF 缺失 401、未知对象 404、创建重放同 ID、幂等内容冲突 409、缺少修订 428、过期修订 412 |
| 输入反例 | 部署外场景、ODP 源冒充 OBServer、SQL/Parquet/ORC、RPC 类型/范围、未知字段、thread/parallel 错误值及目标表非法文本均拒绝 |
| 真实 Agent 文件发现 | 单表目录 1 文件、单文件 1 文件、同表分片 2 文件均 SUCCEEDED/PASSED，具有真实输入清单 |
| 文件发现负例 | 多表目录、空目录、缺失路径及允许根目录外路径均 FAILED，未形成可用输入清单 |
| 完整配置/命令/预检 | 3 项 BLOCKED：当前源登记为 ODP 且集群名为空，完整配置保存/预览被拒绝，九项检查未创建 |
| 提交门禁 | 未证明的提交返回 422 `DIRECT_LOAD_EXECUTION_GATED`；仅验证拒绝路径，旁路任务总数仍为 0 |
| 清理与服务 | 7 个本轮草稿均软删除；自有库和目录经所有权核验清理；15 张测试表完整行集与 DDL 前后一致；Agent ACTIVE、占用 0，无非终态任务 |

首批 `0e1515bb8dd8` 在完整草稿保存被拒绝时停止，尚未创建夹具；随后批次 `7793de53a425` 验证未完成草稿与单表文件发现并清理。最新独立批次加入完整文件负例及幂等内容冲突；没有重放已清理批次。首批测试脚本错误响应层级未兼容现役顶层 `code`，已修正，不将脚本诊断缺失判成产品错误。

## 用户补充单机模式后的结论

用户随后确认当前源为单机模式。因此，ODP 集群名缺失是**选错连接路径后的平台阻断**，不是要求用户为单机部署补建 ODP，也不是数据库不支持旁路的证据。

- [集中式版 V4.4.2 的 OBLOADER 官方说明](https://www.oceanbase.com/en/docs/common-oceanbase-database-standalone-1000000006338110)明确给出直连 OBServer 路径：SQL 默认 2881、RPC 默认 2882；`--direct` 使用 RPC，输入文件由客户端读取。默认端口仍需实际环境确认，TCP 可达不能代表旁路成功。
- [集中式版登录租户说明](https://www.oceanbase.com/docs/common-oceanbase-database-standalone-1000000003577291)明确直连账号使用 `username@tenant`；单机不等于取消租户。独立 tenant 参数和登录身份必须按固定工具行为统一派生，不能丢失租户。
- 本机已核验官方 4.3.5 主 JAR（SHA256 `b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06`）的 `DirectPathConnection$Builder.build()` 静态字节码无条件检查 tenant 非空，并传入 RPC `setLoginInfo`。此证据提示现有“直连不使用独立 tenant”映射需要复核；它不证明实际连接、写入或版本兼容成功。静态回执保留在本轮 artifacts，未运行官方导入。
- 固定辅助探测再次读到 OBServer `4.4.2.3`；当前 SQL 上 `SHOW PROXYINFO BINARY` 返回 1064、同址默认 2883 用同一现有账号返回 1045，只描述尝试的 ODP 路径，不能据此判定单机直连失败。

## 必须解决的契约与实现冲突

1. [DEC-045](../../01-product/decisions.md#dec-045-当前连接范围仅使用私有-odp)规定数据源/API 只接受 ODP；DEC-057 与旁路向导又提供 `OBSERVER_DIRECT`。现役源登记 API、连接测试秘密投影仍有 ODP 硬限制，直接把旁路 scenario 改为 OBServer 返回 404；不能靠改 SQLite、伪造源类型或借用 ODP 证明接通单机。
2. `directload.ValidateVersions` 按旧 [4.3.5 工具专题](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997310)矩阵限制直连 4.2/4.3；新集中式版 4.4.2 文档给出更宽版本说明，但同时要求以使用的导数工具发行说明为准。当前 `4.4.2.3` 组合尚未形成固定工具与真实执行证据，不据此断言官方工具不支持，也不静默放宽平台已确认矩阵。

已向用户提出范围决策：接通单机直连数据源及旁路链路，或本轮仅记录限制。执行授权不缺失；此处待决的是既有已确认产品范围的冲突。决定前只保留已验证的收缩行为，不修改原数据源、账号权限、现役二进制或旁路开关。

## 本轮检查

- `go test`：controlplane、directload、directloadfiles、agentworker、precheckcontract、store 6 包通过。
- 固定夹具 `CGO_ENABLED=0 go build -trimpath` 成功；Node 两个 API 脚本语法检查及 PowerShell AST 检查通过。
- `scripts/check-secrets.ps1`、`git diff --check` 通过；独立内存核对未发现 admin/TEST 数据源口令原文或其可恢复编码进入本轮 JSON 回执。
- 未替换控制面/Agent。两端 `realExecutionEnabled=true`，旁路 probe/execution 仍为 false；已存在的其他模块运行许可不构成旁路许可。未进行 Oracle 旁路、真实导入/取消/重跑、F5 转发或麒麟目标验证。
