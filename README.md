# OB Data Orch

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台。本仓库当前处于 **DEV-01 / G1 工程骨架**，尚不是可执行真实导出任务的产品版本。

## 当前可用内容

- Go 控制面空入口：只提供 `/healthz`、`/readyz` 和 `/version`；
- Go Agent 空入口：不联网、不领取任务、不启动 OB Loader/Dumper；
- Vue 3 + TypeScript + Vite 路由壳：只展示工程阶段和真实执行关闭状态；
- Windows AMD64、Linux AMD64、Linux ARM64 交叉构建；
- Go/前端测试、静态检查和基础敏感信息扫描。

`/readyz` 只表示控制面进程本身可响应，不表示数据库、Agent 或 OB Loader/Dumper 已可使用。交叉构建成功也不等于三个麒麟目标环境已经认证通过。

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
internal/             当前仅含工程基础设施，不含业务模块
web/                  最小 Vue 路由壳
scripts/              本地一致性验证
docs/                 产品、设计与技术基线
.github/workflows/    持续集成
```

下一阶段是 DEV-02：OpenAPI、正式 `0001` SQLite 迁移草案和参数元数据只读资源。真实数据库连接、真实凭据和 OBDUMPER 执行仍需等待 G3 授权集成门禁通过。
