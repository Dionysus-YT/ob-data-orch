# OB Data Orch

OB Data Orch 是 **OB Loader/Dumper 4.3.5 的轻量可视化编排平台**，用于配置数据任务、管理执行节点，以及查看任务状态、日志和结果证据。平台沿用官方工具能力，支持范围与真实验证状态以[开发任务地图](docs/03-technical/development-task-map.md)为准。

## 部署方式

一台控制面连接一台或多台 Agent：

- **控制面**提供网页、HTTPS API 和任务编排，使用本机 SQLite 保存元数据。
- **Agent**安装在执行机器，主动连接控制面；Java 8 和 OB Loader/Dumper 4.3.5 安装在这台机器。
- **浏览器**访问控制面的 HTTPS 地址。前端随包发布，无需另起 Vite 或 Node.js 服务。

运行机器不需要 Go、Node.js 或 npm。控制面与 Agent 可以位于同一机器，也可以分开部署；分开时，控制面地址必须能从 Agent 机器访问，不能使用 `localhost` 或 `127.0.0.1`。

## 日常开发：直接运行源码

在 Windows AMD64 开发机器的仓库根目录执行（Go 1.26、Node.js 24、npm 11）：

```powershell
# 首次或前端锁文件变化后安装依赖
npm --prefix web ci

# 终端一：开发控制面 + Vite，首次提示设置开发 admin 密码
.\scripts\dev.ps1 -RealExecution

# 终端二：需要节点联调时启动开发 Agent，首次粘贴网页注册码
.\scripts\dev.ps1 -Agent -RealExecution
```

浏览器访问 **`https://127.0.0.1:18443`**。首次需信任 `var/dev/control-plane/data/control-plane-ca.pem` 公钥，再以 `admin` 登录。创建 Windows 节点后，将注册码粘贴到第二个终端；开发 Agent 无需下载和安装服务。

- 修改 Vue、TypeScript、CSS：Vite 即时更新页面，无需打包。
- 修改 Go、契约 JSON 或迁移 SQL：真实模式下提示待更新，结束当前测试后在对应终端输入 `r` 并回车，完成正常停止、重编译和重启；不用打包。合成模式保留自动重启。控制面重启后需重新登录。
- `Ctrl+C` 退出对应入口。开发数据库、身份和证书保留在 `var/dev/`，不复用安装版数据，也不自动迁移旧 Local MVP。
- `-RealExecution` 在对应组件的开发配置中开启真实连接与工具执行，后续不带参数启动也保留开启状态。开发入口仍只监听回环，实际任务沿用授权、预检查和执行门禁；开发监视器不管理已安装的系统服务。

Vite 由开发入口管理，浏览器统一访问上面的 HTTPS 地址。稳定后再执行下方构建与安装包升级流程。详细说明见[源码开发模式](docs/03-technical/deployment-operations.md#源码开发模式)。

## 稳定版本：安装包启动

### 1. 准备安装包

使用与控制面机器匹配的完整安装包，解压到固定的本机磁盘目录：

| 包名 | 目标 |
|---|---|
| `ob-data-orch-windows-amd64.zip` | Windows AMD64 |
| `ob-data-orch-linux-amd64.zip` | 指定麒麟 AMD64 目标 |
| `ob-data-orch-linux-arm64.zip` | 指定麒麟 ARM64 目标 |

Linux 包仅对应[技术栈列明的三个麒麟目标](docs/03-technical/technology-stack.md)，不代表任意 Linux 发行版均受支持；目标机原生认证仍待完成。没有成品包时，按下方“从源码构建”生成。

### 2. 启动控制面

- **Windows**：右键包内 `启动.cmd`，选择“以管理员身份运行”。
- **Linux**：在解压目录执行 `chmod +x control-plane start.sh`，再执行 `sudo ./start.sh`。

选择 **1 启动**，按提示填写固定 HTTPS 地址并设置 `admin` 密码。Windows 首次安装还需当前 Windows 账户的密码，不能使用 PIN；后续维护须使用同一账户。Linux 入口创建固定专用服务账户。

启动检查通过后，在浏览器打开所填地址并登录。Windows 入口会为本机安装 CA 公钥信任；其他访问机器及 Linux 浏览器需先信任控制面 `data/control-plane-ca.pem`。详见[首次安装](docs/03-technical/deployment-operations.md#首次安装控制面)。

### 3. 关联 Agent

1. 在网页“执行节点”创建节点，按目标机器填写平台、Java、工具及允许目录等配置。
2. 从节点页面下载对应平台的 Agent 包，解压到执行机器的固定目录。不要直接使用控制面包内 `agents/` 的分发素材。
3. 运行 Agent 包内的启动入口，首次粘贴页面提供的注册码。注册码单次使用、24 小时有效；包内已包含控制面地址和 CA 公钥。
4. 回到节点页面确认关联、心跳和环境检查结果；新节点检查通过后再启用。

[Agent 安装与注册详解](docs/03-technical/deployment-operations.md#安装与关联-agent)

### 4. 使用网页

先准备可用执行节点，再在“数据源”维护连接信息。获准进行真实验证后，使用连接测试和导出向导完成参数配置、预检查、提交；随后在任务中心查看状态、日志与结果证据。

**安装包默认关闭真实执行。** 节点在线不等于数据库可连接，也不等于可以运行导出。启用条件和操作方法见[网页使用与真实执行](docs/03-technical/deployment-operations.md#网页使用与真实执行)。

## 日常启停与升级

控制面和 Agent 各自使用安装目录内的同一个入口：Windows 为 `启动.cmd`，Linux 为 `start.sh`。

| 菜单 | 用途 |
|---|---|
| 1 启动 | 首次安装服务；已安装时启动服务 |
| 2 停止 | 停止对应服务，保留数据和身份 |
| 3 升级 | 输入新版包的独立解压目录，在原安装目录完成替换 |
| 4 查看状态 | 查看对应系统服务状态；Agent 在线状态另看节点页面 |

升级保留 `data`、Agent 配置和 CA，更新程序、发布资源及启动入口。**2026-09-14 的旧入口首次升级前，需要先替换入口脚本**；旧 Local MVP 部署则属于迁移，不能直接套用菜单升级。按[升级与旧部署迁移](docs/03-technical/deployment-operations.md#升级与故障恢复)操作。

## 从源码构建

在 Windows 开发机器准备 Go 1.26、Node.js 24、npm 11，并在仓库根目录执行：

```powershell
# 首次构建或前端锁文件更新后安装依赖
npm --prefix web ci

# 构建前端及三个目标的控制面、Agent 成品包
./scripts/build-package.ps1
```

输出位于 `artifacts/release-时间/`，包含三个 ZIP 及对应目录。构建不会安装、停止或更新现有服务。完整包中的 `web/` 和 `agents/` 必须与控制面程序一起交付。

## 开发验证

| 范围 | Windows | Linux / Shell |
|---|---|---|
| 整体工程检查 | `./scripts/verify.ps1` | `./scripts/verify.sh` |
| Export 合成回归 | `./scripts/verify-export-synthetic.ps1` | `./scripts/verify-export-synthetic.sh` |
| 启动入口隔离回归 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/tests/distribution.ps1` | `bash scripts/tests/distribution.sh` |

合成回归使用临时数据和假依赖，不构成真实数据库、官方工具或目标平台运行验收。启动入口测试使用假服务操作，不安装系统服务。

当前启动改造已通过代码回归、隔离升级测试及三目标构建；**Windows 服务安装、开机自启和麒麟原生运行仍未完成验证**。具体范围见[验证记录](docs/03-technical/deployment-operations.md#2026-09-15-启动入口修复验证)，开发顺序和准入仍以任务地图为准。

## 文档与源码

- [安装、使用与运维手册](docs/03-technical/deployment-operations.md)：准备环境、注册 Agent、启停、升级和故障定位。
- [文档中心](docs/README.md)：产品范围、设计、技术契约与验证证据。
- [开发任务地图](docs/03-technical/development-task-map.md)：当前阶段、下一步和准入门禁。

| 目录 | 内容 |
|---|---|
| `cmd/` | 控制面与 Agent 的程序入口 |
| `internal/` | 业务实现、Agent 协议及基础设施 |
| `contracts/`、`migrations/` | API 契约与 SQLite 迁移 |
| `web/`、`design-system/` | Vue 前端与设计规范 |
| `scripts/` | 构建、启动器与验证脚本 |
| `docs/` | 产品、设计、技术与证据文档 |

<a id="原本机开发环境升级"></a>

旧 Local MVP 数据与身份迁移说明已集中到[使用手册](docs/03-technical/deployment-operations.md#旧-local-mvp-部署迁移)。
