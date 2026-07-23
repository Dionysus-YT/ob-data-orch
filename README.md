# OB Data Orch

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台。项目阶段、门禁和真实验证状态以[开发任务地图](docs/03-technical/development-task-map.md)为准；产品页面的功能接入顺序以[产品页面功能接入计划](docs/03-technical/frontend-functional-integration-plan.md)为准。当前仓库尚不是可执行真实导出任务的产品版本。

## 当前可用内容

- Go 控制面最小浏览器/Agent API 适配：默认未配置身份时失败关闭，真实执行始终关闭；
- Go Agent 空入口：不联网、不领取任务、不启动 OB Loader/Dumper；
- Vue 3 + TypeScript + Vite 产品页面基线：覆盖数据源、三类任务向导、任务/日志、模板、节点与系统设置；其中真实功能接入范围以开发计划逐项验收；
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

页面基线、合成验证、真实功能可用与真实环境验证是不同状态。默认 Agent 启动入口不联网、不启动工具；真实数据库连接、真实凭据、真实进程和 OBDUMPER 执行均须遵循任务地图规定的授权与证据门禁。
