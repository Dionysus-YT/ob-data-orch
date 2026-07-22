# OB Data Orch

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台。本仓库已完成 **DEV-06 / G2 最小前端业务链路**的本地合成/隔离验证，尚不是可执行真实导出任务的产品版本。

## 当前可用内容

- Go 控制面最小浏览器/Agent API 适配：默认未配置身份时失败关闭，真实执行始终关闭；
- Go Agent 空入口：不联网、不领取任务、不启动 OB Loader/Dumper；
- Vue 3 + TypeScript + Vite 最小业务链路：数据源选择、CSV 草稿、脱敏命令、预检查、任务详情与日志安全投影；
- OpenAPI 3.1 结构基线：覆盖 29 个浏览器操作和 14 个 Agent 操作，真实执行仍关闭；
- SQLite `0001` 前向迁移草案：20 张首条切片窄表及关键约束，仅在临时数据库使用合成数据验证；
- OBDUMPER 4.3.5 首条切片只读参数资源：8 个可用参数、8 个验证门禁参数；
- Windows AMD64、Linux AMD64、Linux ARM64 交叉构建；
- Go/前端测试、静态检查和基础敏感信息扫描。

`/readyz` 只表示控制面进程本身可响应。浏览器身份/CSRF 注入和执行节点列表尚未接入实际控制面，Agent 默认入口仍不联网，OB Loader/Dumper 尚未接入。交叉构建成功也不等于三个麒麟目标环境已经认证通过。

## 本地启动

要求 Go 1.26、Node.js 24 和 npm 11。

```powershell
# 控制面，默认仅监听 127.0.0.1:8080
go run ./cmd/control-plane

# Agent 仅进入空闲等待，不会联网或执行工具
go run ./cmd/agent

# 前端开发服务器
Set-Location web
npm ci
npm run dev
```

控制面监听地址可通过 `OB_DATA_ORCH_LISTEN` 修改。`OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true` 会使控制面和 Agent 拒绝启动；这是 G2 的硬门禁，不是待配置功能。

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

DEV-06 已在 G2 收口：前端以脱敏浏览器 API 串联数据源选择、CSV 草稿、命令预览、预检查、任务详情和日志快照；默认 Agent 启动入口仍为空闲且不联网、不启动工具。该适配不包含真实认证会话、节点列表、真实数据库、真实凭据、真实进程或 OBDUMPER。真实数据库连接、真实凭据和 OBDUMPER 执行继续等待 G3 授权集成门禁通过。
