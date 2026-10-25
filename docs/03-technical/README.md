# 技术契约与验证

- [普通任务日志保留设置](log-retention-settings-contract.md)：实际策略来源、权限、原子保存及后续批次生效边界。
- [调度策略](scheduling-policy-contract.md)：并发上限、心跳/启动/领取检查时间、独立候选顺序、原子领取、可靠释放及权限边界。
- [安全策略只读状态](security-policy-contract.md)：实际启动开关、密钥版本别名、固定保护与未配置审计期限的安全投影。
- [工具与兼容只读状态](tool-compatibility-contract.md)：内置工具目标、按模块/配置选择的目录、历史重放和节点环境边界。

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
| 已认证主体权限、角色范围、原子保存与即时撤权 | [权限配置首批契约](access-control-contract.md) · [前端维护入口](../../web/src/workbench/access-control/README.md) |
| 固定工具启动、进程和恢复 | [工具隔离启动](tool-launch-isolation-contract.md) |
| 日志、脱敏、游标与证据 | [日志与执行证据](log-collection-evidence-contract.md) |
| API、SQLite、迁移与备份 | [API 与 SQLite](api-sqlite-data-contract.md) · [OpenAPI](../../contracts/README.md) · [迁移](../../migrations/README.md) |
| 首页只读快照、UTC 终态分桶与任务中心下钻 | [API 契约 §5.1.1](api-sqlite-data-contract.md#511-首页只读运行快照2026-10-23) · [首页维护入口](../../web/src/workbench/overview/README.md) |
| Export 领域模型与兼容 | [通用导出契约](export-general-contract.md) |
| 普通导入草稿、文件与检查 | [普通导入技术契约](normal-import-contract.md)，独立 CSV/CUT/SQL 配置及 G2/真实执行边界；[Windows MySQL 真实 E2E](evidence/normal-import-mysql-e2e-2026-10-19.md)、[失败整改与待复验状态](evidence/normal-import-fixes-2026-10-20.md) |
| 旁路导入草稿、单表文件与独立检查 | [旁路导入技术契约](direct-load-contract.md)，本地五步配置、九项预检及独立执行门禁；现役能力查任务地图 |
| 开发和真实执行准入 | [准入与真实验证门禁](development-readiness-closure.md)，含首条切片与 Windows 验收操作 |

<a id="evidence"></a>

## 按问题查验证记录

以下均为历史证据；请同时核对日期、环境、测试范围与未覆盖项。真实执行授权和 G3/G4、EX-V1/EX-V2 门禁不因文档合并而改变。

| 要核对的证据 | 记录 |
|---|---|
| 首页四指标、唯一趋势、节点/任务/异常与权限、迟到隔离、响应式 | [首页本地与合成验收](evidence/home-overview-2026-10-23.md) |
| 工具版本、包摘要与帮助参数 | [4.3.5 工具包基线](evidence/tool-package-baseline.md) |
| 单机直连登记、租户映射、真实 API 导入与日志隔离 | [单机修复验收](evidence/direct-load-standalone-repair-2026-10-22.md)，仅已实证的 Windows/MySQL/4.4.2.3 组合 |
| 旁路提交防重、替换入口和具体结构诊断 | [三项缺陷修复验收](evidence/direct-load-defect-fixes-2026-10-22.md)，新双源 14/14 API、2 个真实任务与单一任务/目标取证通过 |
| Oracle 直连固定预检修复与真实 API 导入 | [Oracle 修复验收](evidence/direct-load-oracle-repair-2026-10-22.md)，12/12 API、7 个真实任务及完整行集/DDL 核对通过；仅精确组合及当前账号 Schema |
| Oracle 修复前基础连接、文件发现及九项预检阻断 | [Oracle API 验证](evidence/direct-load-oracle-api-2026-10-22.md)，5 PASS / 1 BLOCKED，零导入任务 |
| 官方安全文件兼容与错误私钥 | [安全材料验证](evidence/secure-gen-compatibility-spike-2026-07-21.md) |
| 直接 Java 入口、路径与退出码 | [隔离启动验证](evidence/direct-java-launch-spike-2026-07-21.md) |
| 初始切片 P0 结果及阻断 | [P0 执行记录](evidence/first-vertical-slice-p0-2026-07-21.md) |
| SQLite、日志脱敏、JDBC、假 Agent 和最小前端 | [隔离组件验证](evidence/component-validation.md) |
| Windows CSV、权限负例、argv、日志和中断恢复、POS | [Windows 受控验证](evidence/windows-validation.md) |
| 剩余参数两批实测、存储预检查和结果复用 | [Export 增量验证](evidence/export-increment-validation.md) |
| 第五步自动预检查、独立检查与依赖阻断、读取恢复及提交门禁 | [第五步重构验证](evidence/export-precheck-review-2026-10-13.md) |
| MySQL 本地 172 项真实 E2E、失败组合、生命周期、参数追踪与夹具清理 | [2026-10-14 实测结果](evidence/export-mysql-e2e-2026-10-14.md) · [用例矩阵](evidence/export-mysql-e2e-cases.md) · [参数追踪](evidence/export-mysql-parameter-coverage.md) |
| 任务中心列表重构（阶段 1）：冻结展示事实、服务端筛选与行资格、合成回归 | [任务中心阶段 1 记录](evidence/task-center-phase1-2026-10-14.md) |
| 旁路导入六步、草稿/命令、单表文件、专用探针/执行器、派生及真实门控 | [旁路开发证据](evidence/direct-load-development-2026-10-19.md) |
| 解释 DEV/F 阶段或旧桌面组件实现的来由 | [工程历史](evidence/engineering-history.md)，按需追溯 |

日常修改只回写受影响契约、任务地图和对应证据。验证结果不能从“文档有定义”“代码存在”或“构建通过”外推。

- [日志中心任务查询契约](log-center-task-query-contract.md)：授权快照、同源窗口与脱敏下载。
