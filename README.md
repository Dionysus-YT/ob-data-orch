# OB Data Orch

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台。开发顺序、阶段、门禁、页面功能接入和真实验证状态统一以[开发任务地图](docs/03-technical/development-task-map.md)为准。当前仓库尚不是可执行真实导出任务的产品版本。

## 当前可用内容

- Go 控制面最小浏览器/Agent API 适配：默认未配置身份时失败关闭，真实执行始终关闭；回环 Local MVP 使用 TLS，可完成节点声明、一次性注册码签发、Agent 实际关联、首次心跳和节点侧 `G2_SYNTHETIC` 连接测试闭环；仅在控制面与 Agent 都显式开启受控 JDBC 测试开关时，才签发一次节点侧 `AGENT_JDBC` 基础连接测试；
- Go Agent G2 机器协议适配：受保护本机关联状态、HTTPS/CA 校验、受认证心跳和固定预检查信封均已具备；默认不连接数据库、不解析真实凭据、不启动 OB Loader/Dumper；
- Vue 3 + TypeScript + Vite 产品页面基线：覆盖数据源、三类任务向导、任务/日志、模板、节点与系统设置；其中真实功能接入范围以开发计划逐项验收；
- OpenAPI 3.1 结构基线：覆盖当前已实现的浏览器与 Agent 操作，真实执行仍关闭；
- SQLite 前向迁移：`0001` 建立 20 张首条切片窄表，后续迁移补充数据源 ODP 身份、未验证数据源禁用和无引用归档数据源清理；仅在临时数据库使用合成数据验证；
- OBDUMPER 4.3.5 首条切片只读参数资源：8 个可用参数、8 个验证门禁参数；
- Windows AMD64、Linux AMD64、Linux ARM64 交叉构建；
- Go/前端测试、静态检查和基础敏感信息扫描。

`/readyz` 只表示控制面进程本身可响应。默认启动时业务 API 会因未配置浏览器身份而失败关闭；显式 `--local-mvp` 只允许回环本机的 TLS 管理面，并可验证节点、注册码、Agent 实际关联与首次心跳。默认该入口不会建立真实数据库连接；仅在当次真实连接获明确授权、控制面与 Agent 都设置 `OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=true`，且 Agent 已通过 Java/工具运行时校验时，才允许固定 JDBC 探针建立一次受控基础连接。该开关不启用节点、预检查、OBDUMPER 或任何导入导出。Agent 只会主动访问 HTTPS 控制面；HTTP、未受信证书及非回环 Local MVP 绑定都会失败关闭。交叉构建成功也不等于三个麒麟目标环境已经认证通过。

### Windows 本机 Agent 注册测试

```powershell
# 终端一：生成仅用于本机回环 TLS 的 7 天测试证书（材料位于 Git 忽略的 var/）
./scripts/new-local-mvp-tls-certificate.ps1

# 终端一：仅监听 https://127.0.0.1:8080，不接受外部连接
$env:OB_DATA_ORCH_LISTEN = '127.0.0.1:8080'
$env:OB_DATA_ORCH_TLS_CERT_FILE = "$PWD\var\local-mvp-tls\control-plane-cert.pem"
$env:OB_DATA_ORCH_TLS_KEY_FILE = "$PWD\var\local-mvp-tls\control-plane-key.pem"
go run ./cmd/control-plane --local-mvp

# 终端二：构建固定指向本机控制面的 Windows Agent 包
./scripts/build-local-mvp-agent-package.ps1 -ControlPlaneCAFile "$PWD\var\local-mvp-tls\control-plane-ca.pem"

# 终端三：Vite 代理会显式验证本机测试 CA，绝不跳过 TLS 校验
$env:NODE_EXTRA_CA_CERTS = "$PWD\var\local-mvp-tls\control-plane-ca.pem"
Set-Location web
npm run dev -- --host 127.0.0.1
```

在浏览器打开 `http://127.0.0.1:5173/nodes`，创建 Windows 节点后可在节点详情下载 Windows Agent ZIP。将 ZIP 解压到目标 Windows 机器，首次双击“首次注册并启动Agent.cmd”，粘贴页面生成的一次性注册码并按 Enter；进程会继续发送心跳。以后双击“启动Agent.cmd”，无需再次输入注册码。两个脚本都会显式启用固定 JDBC 连接测试、六项 `EXPORT_PREFLIGHT` 与受控真实执行；它只会在操作者完成预检查并在导出向导点击“提交并启动导出”后，领取该节点的一条冻结单表 CSV 任务并直接启动 OBDUMPER。密码只进入任务级官方安全文件，绝不进入启动参数或日志。该下载包固定连接本机回环 TLS，只用于控制面和 Agent 位于同一 Windows 主机的 Local MVP 测试。当前实时日志为控制面进程内投影，控制面重启后不保留；断线恢复与正式 G3 验收尚未完成。

本机 MVP 的 `G2_SYNTHETIC` 连接测试在控制面确认首次心跳后由 Agent 独立领取，最多约 2 秒开始处理，不再等待下一次 30 秒心跳。它不建立真实数据库连接。

本机 MVP 的监听地址固定为 `127.0.0.1:8080`，且必须同时配置 TLS 证书和私钥。`OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true` 仅在控制面使用 `--local-mvp` 且 Agent 为 Windows AMD64 时启用这条受控本机链路；非本机 MVP、任意命令、任意 SQL、任意路径浏览、自动重试和重新分配仍然拒绝。

### G2 Agent HTTPS 边界

非 `--local-mvp` 控制面只有同时配置绝对路径的 `OB_DATA_ORCH_TLS_CERT_FILE` 与 `OB_DATA_ORCH_TLS_KEY_FILE` 时才会使用 HTTPS；缺少任一文件会拒绝启动。Windows 本机包无需用户填写 URL、节点 IP、CA 路径或节点标识：包内固定 `https://127.0.0.1:8080` 与同目录 CA 文件，注册码仅通过标准输入短时接收。Agent 不会跟随重定向、使用代理或跳过证书和主机名校验。

这条链路已通过回环 TLS、临时 SQLite 与实际 Windows `agent.exe` 完成关联和首个心跳验证；仍不是获授权 G3 运行环境，也不是连接真实 ODP、启用数据源或启动工具的授权。

## 本地启动

要求 Go 1.26、Node.js 24 和 npm 11。

```powershell
# 控制面默认仅监听 127.0.0.1:8080，未配置浏览器身份时所有业务 API 失败关闭
go run ./cmd/control-plane

# 未关联 Agent 会失败关闭；已关联 Agent 仅发送 HTTPS 心跳，不执行工具
go run ./cmd/agent

# 前端开发服务器
Set-Location web
npm ci
npm run dev
```

## 验证

Windows：

```powershell
./scripts/verify.ps1
```

Linux：

```sh
./scripts/verify.sh
```

## 目录

```text
cmd/                  控制面和 Agent 启动入口
contracts/            OpenAPI 3.1 结构与安全边界基线
internal/             工程基础设施、迁移器和参数元数据读取器
migrations/           SQLite 前向迁移草案
web/                  最小 Vue 业务链路
scripts/              本地一致性验证
docs/                 产品、设计与技术基线
.github/workflows/    持续集成
```

页面基线、合成验证、真实功能可用与真实环境验证是不同状态。默认 Agent 启动入口只进行受认证关联、心跳和 `G2_SYNTHETIC` 基础连接测试编排，不解析数据库槽位、不启动 Java/JDBC 或 OBDUMPER；真实数据库连接、真实凭据、真实进程和 OBDUMPER 执行均须遵循任务地图规定的授权与证据门禁。
