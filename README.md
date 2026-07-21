# OB Data Orch

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台。本仓库已完成 **DEV-02 / G2 契约与迁移基线**的本地合成验证，尚不是可执行真实导出任务的产品版本。

## 当前可用内容

- Go 控制面空入口：只提供 `/healthz`、`/readyz` 和 `/version`；
- Go Agent 空入口：不联网、不领取任务、不启动 OB Loader/Dumper；
- Vue 3 + TypeScript + Vite 路由壳：只展示工程阶段和真实执行关闭状态；
- OpenAPI 3.1 结构基线：覆盖 29 个浏览器操作和 14 个 Agent 操作，真实执行仍关闭；
- SQLite `0001` 前向迁移草案：20 张首条切片窄表及关键约束，仅在临时数据库使用合成数据验证；
- OBDUMPER 4.3.5 首条切片只读参数资源：8 个可用参数、8 个验证门禁参数；
- Windows AMD64、Linux AMD64、Linux ARM64 交叉构建；
- Go/前端测试、静态检查和基础敏感信息扫描。

`/readyz` 只表示控制面进程本身可响应。迁移器尚未接入控制面启动，Agent 尚未联网，OB Loader/Dumper 尚未接入。交叉构建成功也不等于三个麒麟目标环境已经认证通过。

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

控制面监听地址可通过 `OB_DATA_ORCH_LISTEN` 修改。`OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true` 会使控制面和 Agent 拒绝启动；这是 G1 的硬门禁，不是待配置功能。

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
web/                  最小 Vue 路由壳
scripts/              本地一致性验证
docs/                 产品、设计与技术基线
.github/workflows/    持续集成
```

当前进入 DEV-03：参数规范化/确定性命令生成、SQLite 核心仓储、凭据加密与安全目录三个独立组件均已通过纯合成契约测试；下一项是 Agent 协议与状态机。后续日志组件仍需分别实现和验收。真实数据库连接、真实凭据和 OBDUMPER 执行继续等待 G3 授权集成门禁通过。
