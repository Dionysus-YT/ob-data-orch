# Agent 安装包命名与重建验证（2026-10-10）

本轮按用户确认的操作系统分类和 `lscpu` CPU 架构名称重建安装包。交付名称采用 `windows-x86_64`、`linux-x86_64`、`linux-aarch64`；Go 构建参数仍使用 `windows/amd64`、`linux/amd64`、`linux/arm64`。原三个节点平台协议值、机器身份、执行开关和正式支持矩阵不变。

## 交付与命名

成品位于 Git 忽略目录 `artifacts/release-platform-naming-20261010/`，由当前工作区源码构建，未提交或推送。Go 1.26.4 构建元信息为 `vcs.revision=991498ca2df7b51d2c4c26636a5af0e3c0ab346b`、`vcs.modified=true`；这是包含工作区修改的开发构建，未创建新的正式版本号。

| 注册分类 | Agent ZIP | 分发目录 | Go 构建参数 |
|---|---|---|---|
| Windows · x86_64 | `ob-data-orch-agent-windows-x86_64.zip` | `agents/windows-x86_64/` | `GOOS=windows GOARCH=amd64` |
| Linux · x86_64 | `ob-data-orch-agent-linux-x86_64.zip` | `agents/linux-x86_64/` | `GOOS=linux GOARCH=amd64` |
| Linux · aarch64 | `ob-data-orch-agent-linux-aarch64.zip` | `agents/linux-aarch64/` | `GOOS=linux GOARCH=arm64` |

同时生成 `ob-data-orch-windows-x86_64.zip`、`ob-data-orch-linux-x86_64.zip`、`ob-data-orch-linux-aarch64.zip` 三套完整控制面包，每套均包含全部三种 Agent 分发素材。

安装包、分发目录、详情页下载链接、下载处理器、Windows 升级资源校验及当前部署文档同步采用新命名。前端下载名称从 `nodePlatformRules.ts` 的同一平台分类映射派生，页面不再单独维护构建架构别名。

构建使用 `CGO_ENABLED=0` 和 `-trimpath`。修复构建脚本缺少 UTF-8 BOM 导致 Windows PowerShell 5.1 解析失败的问题；完整 ZIP 改用标准 `ZipFile.CreateFromDirectory`，避免 `Compress-Archive` 将 Windows 反斜杠写入 ZIP 内路径。最终全部 ZIP 内部采用正斜杠，逐项读取和内容比较通过。

## 验证结果

- `go test ./internal/deployment ./cmd/control-plane ./cmd/agent` 与相同范围的 `go vet` 通过。新增下载契约测试覆盖三平台 GET/HEAD、认证保护、二进制/入口/配置/CA/权限、旧或未知名称拒绝、非读取方法拒绝，以及缺少二进制、启动入口或 CA 时失败关闭。
- Windows 升级夹具通过：原有十种服务路径、启动入口与配置保留、Agent 升级、新命名三平台控制面资源升级，以及缺少任一资源时在停服前拒绝。
- TypeScript 检查、相关 ESLint、前端生产构建通过。构建存在既有的大 chunk 提示，不影响本轮成品生成。
- 注册 Chrome 9/9 通过，三种分类均断言新下载链接；保留注册 DTO、独立签发、抽屉导航和授权失效负例。业务架构审计 11/11、零违规；六个既有人工复核信号保留。
- 新构建的 Windows 控制面使用临时数据目录、动态回环端口、合成管理员密码、合成 CA 与关闭的真实执行能力启动，健康检查通过；匿名下载拒绝，认证后按新名称实际 HEAD/GET 三套 Agent ZIP 成功，旧名称和 Windows aarch64 拒绝。未连接开发实例的数据库或执行节点。
- 实际交付的三个 Agent ZIP 随后写入当前本机控制面的公共地址和 CA 公钥；重读 ZIP、CRC、完整文件字节及执行权限均通过。配置保留 `agent-bundle-config-v1`、`data/agent-security` 及原有执行开关边界；Windows 包包含五个文件，两个 Linux 包各包含四个文件，不含机器身份、管理员校验材料、数据库或 TLS 私钥。
- 三个完整控制面 ZIP 共核对 106 个文件；Agent ZIP 共核对 13 个文件。二进制与构建输出逐字节一致，启动入口与当前源码一致；`go version -m` 确认控制面和 Agent 的三目标架构及构建参数。全部六个 ZIP 的 SHA-256 已写入成品目录 `SHA256SUMS.txt`。
- 秘密扫描（PowerShell 7）和 `git diff --check` 通过。初次 Windows PowerShell 5.1 秘密扫描解析失败，改用可用的 PowerShell 7 后通过，不将初次失败计为通过。

| Agent 包 | ZIP SHA-256 | Agent 二进制 SHA-256 |
|---|---|---|
| Windows · x86_64 | `62a3239fee9567a5b8abe99e997a78b177b0410054be827caf2fe843df83c543` | `6224ea2a9d26d0f1c32335136e32ec12f0dfd1bf52ff3e4269964350f050c680` |
| Linux · x86_64 | `e8f2ba494bd51685748718103f37854cfca5ae8202b9c23c872c759942bd5be7` | `f7824e019479b53c9c854c1c630265d26d8b21dc83e84814a9d9bcc989b5cf27` |
| Linux · aarch64 | `a4fe306f25438db9ffe202df8346037840745b1823ff49ed4c93e210b190abcf` | `aa7e35d7bb7380e3bebf44c94d9af2a31826867a59f69fc23d3394b45f561683` |

## 部署与真实验证边界

重建验证阶段完成源码命名统一、当前源码构建、成品校验与隔离下载验证，未替换或重启正在运行的开发控制面和 Agent。该阶段开发实例未部署 `agents/` 分发目录，页面下载不算已恢复；需部署完整新版控制面的分发资源，才能由网页的新下载入口提供安装包。后续清理时核验的运行二进制状态见下方记录，不能据此推定下载资源已部署。

交付 Agent 包绑定的是当前本机控制面 `https://127.0.0.1:18443` 与其 CA 公钥。跨机器部署应先使用 Agent 可达的控制面 HTTPS 地址，再从该实例下载绑定包；不能把回环地址当作远程地址，也不能仅修改配置来绕过 TLS 主机名校验。

未做真实 Agent 注册、环境探测、数据库/存储访问、官方工具运行或 Windows 系统服务安装。Linux 仅完成交叉构建、ZIP 内容与权限校验，技术栈指定的三个麒麟目标原生验证仍待完成，不提升任务地图的真实能力准入。

## 历史产物清理

2026-10-10 按用户“进行历史产物全部清理”的授权，删除已由新版成品替代的历史安装包与过期构建文件。删除前逐项确认绝对路径位于工作区、没有 Git 跟踪文件或活动进程依赖；三个历史分发目录均不含重解析点、运行数据、机器身份或 TLS 私钥。

| 已删除对象 | 文件数 |
|---|---:|
| `artifacts/release-startup-final-20260914/` | 46 |
| `artifacts/startup-fixed-20260915/` | 46 |
| `artifacts/startup-validation-20260915/` | 46 |
| 根目录 `agent` | 1 |
| 根目录 `control-plane` | 1 |
| `var/dev/agent/agent-batch-precheck-candidate.exe` | 1 |
| `artifacts/verify-deployment-package.py` | 1 |

合计删除 142 个文件，包含 9 个旧完整包 ZIP、39 个过期二进制及废弃的包验证脚本，逻辑大小为 762,513,601 字节（约 727 MiB）。删除后全部七个对象均不存在；`artifacts/` 与 `var/dev/` 中的分发及构建二进制仅保留本轮成品和正在运行的开发程序。

新版目录 `artifacts/release-platform-naming-20261010/` 的六个 ZIP 均保留，清理前后逐项 SHA-256 与 `SHA256SUMS.txt` 一致。开发数据、`agent-security/`、配置及 CA 材料保留；历史验收原始记录 `artifacts/deployment-package-validation.json`、文档证据与页面复核材料继续保留。

清理期间未停止、替换或重启服务。清理前后控制面 PID 22052、Agent PID 12296 及其二进制哈希均未变化，控制面 HTTPS 健康检查通过。此时两者二进制已分别与新版 Windows 成品一致：控制面 SHA-256 为 `f4db206e505f52ef0bc3a5dc78ac2c1255c791abb81ad5d86b62db0b5d85f643`，Agent 为 `6224ea2a9d26d0f1c32335136e32ec12f0dfd1bf52ff3e4269964350f050c680`；这记录清理时的实际状态，不将此前发生的更新归为本次清理操作。

## 开发实例下载资源恢复

同日用户反馈 Windows x86_64 包下载失败。核查发现 `var/dev/control-plane/agents/` 不存在：完整包构建已经生成三平台资源，但开发启动器此前只构建控制面自身，没有准备下载处理器读取的分发目录。按处理器规则，认证后的请求会返回 503；当前控制面二进制已使用新名称，页面链接与命名相符。

本次将下载资源构建归入 `scripts/dev-agent-packages.mjs`，由 `dev.mjs` 在控制面首次启动和每次重建时调用。三种 Agent 使用 `CGO_ENABLED=0`、`-trimpath` 和对应 Go 目标构建，启动入口一起写入本次独占暂存目录；全部完成才替换 `agents/`。失败构建、缺少启动入口或缺少二进制均不替换上一套资源。控制面源码监视补充 `cmd/agent/` 和 `scripts/distribution/`，继续遵循原有真实模式手动重建策略。

已经用同一构建函数从当前源码生成并发布当前开发实例的三平台资源，共七个文件。三套 Agent 二进制 SHA-256 分别与上方交付表一致，启动入口来自当前源码；暂存目录清除。此发布只补齐分发素材，运行控制面与 Agent 的 PID、二进制、管理员设置、Agent 配置和 CA 均保持原值，身份目录保留，控制面 HTTPS 健康检查通过。没有停止或重启当前监视器；新版启动器逻辑在下次重新运行开发入口后生效。

验证结果：

- `node --test scripts/tests/dev-settings.test.mjs scripts/tests/dev-agent-packages.test.mjs` 8/8 通过，覆盖新平台资源、成功替换、三个失败保留场景及原有执行开关与重建策略；`node --check scripts/dev.mjs` 通过。
- `go test ./internal/deployment -run TestAgentPackage -count=1` 和 `go vet ./internal/deployment` 通过。
- 使用实际部署的 `agents/` 和开发版下载处理器，在动态回环端口创建隔离 HTTPS 服务；只使用合成管理员、独立临时 CA 与关闭的真实执行能力，不接入用户数据库。正常合成登录后，三个新名称的 HEAD/GET 均返回 200，下载名与内容类型正确；ZIP 的全部文件、CRC、二进制和启动入口字节、配置绑定与 CA 均核对通过。匿名请求保持 303 登录保护，旧名称及 Windows aarch64 返回 404。临时服务和数据清除。
- 秘密扫描（PowerShell 7）与差异空白检查通过。

浏览器连接工具返回 `Codex auth token is unavailable`，本次未操作或复验用户当前 Edge 登录会话的下载记录；不能将隔离下载通过写成该浏览器已下载成功。未做真实 Agent 注册、数据库/工具操作或 Linux 原生运行，不提升相关真实能力准入。
