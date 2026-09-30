# 技术契约与验证

先按修改对象选契约；开发顺序和能力状态只查 [任务地图](development-task-map.md)。本页不复制接口数量、参数版本或运行进度。

## 架构与契约

| 关注点 | 权威入口 |
|---|---|
| 安装、网页使用、启停、升级与验证 | [安装、使用与运维手册](deployment-operations.md) |
| 技术栈、平台、部署 | [技术栈](technology-stack.md) · [技术架构](architecture.md) |
| 前端依赖、Token、组件和样式治理 | [前端平台基线](frontend-platform-baseline.md)；P0/P1 仍决定产品设计 |
| 参数、命令、路径与指纹 | [参数与命令契约](parameter-command-contract.md) |
| Agent、租约、幂等和状态 | [Agent 与任务状态](agent-task-state-contract.md) |
| 凭据、身份、授权和秘密 | [凭据与访问安全](credential-access-security-contract.md) |
| 固定工具启动、进程和恢复 | [工具隔离启动](tool-launch-isolation-contract.md) |
| 日志、脱敏、游标与证据 | [日志与执行证据](log-collection-evidence-contract.md) |
| API、SQLite、迁移与备份 | [API 与 SQLite](api-sqlite-data-contract.md) · [OpenAPI](../../contracts/README.md) · [迁移](../../migrations/README.md) |
| Export 领域模型与兼容 | [通用导出契约](export-general-contract.md) |
| 开发和真实执行准入 | [准入与真实验证门禁](development-readiness-closure.md)，含首条切片与 Windows 验收操作 |

<a id="evidence"></a>

## 按问题查验证记录

以下均为历史证据；请同时核对日期、环境、测试范围与未覆盖项。真实执行授权和 G3/G4、EX-V1/EX-V2 门禁不因文档合并而改变。

| 要核对的证据 | 记录 |
|---|---|
| 工具版本、包摘要与帮助参数 | [4.3.5 工具包基线](evidence/tool-package-baseline.md) |
| 官方安全文件兼容与错误私钥 | [安全材料验证](evidence/secure-gen-compatibility-spike-2026-07-21.md) |
| 直接 Java 入口、路径与退出码 | [隔离启动验证](evidence/direct-java-launch-spike-2026-07-21.md) |
| 初始切片 P0 结果及阻断 | [P0 执行记录](evidence/first-vertical-slice-p0-2026-07-21.md) |
| SQLite、日志脱敏、JDBC、假 Agent 和最小前端 | [隔离组件验证](evidence/component-validation.md) |
| Windows CSV、权限负例、argv、日志和中断恢复、POS | [Windows 受控验证](evidence/windows-validation.md) |
| 剩余参数两批实测、存储预检查和结果复用 | [Export 增量验证](evidence/export-increment-validation.md) |
| 解释 DEV/F 阶段或旧桌面组件实现的来由 | [工程历史](evidence/engineering-history.md)，按需追溯 |

日常修改只回写受影响契约、任务地图和对应证据。验证结果不能从“文档有定义”“代码存在”或“构建通过”外推。
