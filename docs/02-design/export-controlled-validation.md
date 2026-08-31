# OBDUMPER 导出参数最小受控验证计划

> 文档状态：验证计划保留未完成用例；已有受控证据已回写为当前基线，未验证项仍保持门控
> 目标工具：OBDUMPER 4.3.5-RELEASE
> 更新日期：2026-08-22
> 关联文档：[导出模块 Canonical](export-module.md) · [通用导出技术契约](../03-technical/export-general-contract.md) · [Export 历史研究归档](../archive/export/research/)

## 1. 目标

使用隔离、可重复、可审计的最小测试，解决官网、历史文档与本地 `--help` 之间仍存在的导出参数冲突。测试只验证官方工具行为，不开发平台业务代码，不扩大产品范围。

验证优先级：

- **P0：命令定版门槛**。未完成时阻断对应格式或参数进入最终命令验收。
- **P1：高级能力门槛**。未完成时相关高级/专家字段保持隐藏或“待官方参数映射确认”。
- **P2：兼容调查**。不影响 V1.0 基础导出；默认不展示、不生成。

## 1.1 当前证据基线（2026-08-20）

本表只记录已经存在于仓库的证据，不把“代码已接入”“用户口头确认”或单次退出码 0 直接升级为正式发布证据。

| 能力/格式 | 当前状态 | 已有证据 | 仍缺少的证据或动作 |
|---|---|---|---|
| CSV 本地导出 | VERIFIED（Windows 局部链路） | [Windows 单表 CSV 成功](../03-technical/evidence/windows-authorized-real-export-2026-08-04.md)；[CSV 特殊值两次独立导出](../03-technical/evidence/windows-csv-special-values-validation-2026-08-05.md) | G3 全量门禁、WI-05 实际进程 argv 取证、跨目标认证 |
| POS 用户提供控制文件 | VERIFIED（命令映射与定长输出） | [POS 受控实测与定版](../03-technical/evidence/windows-pos-format-validation-2026-08-07.md) | 向导/生成器/预检查完整产品通道回归；自动生成控制文件实现与证据延期 |
| CUT / Insert SQL | IMPLEMENTED，EVIDENCE_PENDING | EX-I4 契约、控制面、前端和生成器正负例；当前无逐格式 Windows 真实输出证据 | 既有 Run History 映射或另行授权的逐格式真实结果 |
| Parquet / ORC / Avro | VALIDATION_GATED | EX-I5 元数据、生成器、控制面和前端合成正负例 | 逐格式真实输出、对象范围/资源限制和结果事实 |
| DDL / DDL + CSV | DDL `REAL_TOOL_VERIFIED`；DDL + CSV `IMPLEMENTED，EVIDENCE_PENDING` | [EX-I7 剩余参数记录（2026-08-11）](../03-technical/evidence/exi7-remaining-parameters-2026-08-11.md) 覆盖真实 `--ddl`、`--ddl --all --compact-schema`；[对象 DDL 记录（2026-08-13）](../03-technical/evidence/exi7-remaining-parameters-2026-08-13.md) 覆盖 procedure/user DDL 退出码 0 与文件输出；另有 EX-I2/EX-I3 契约路径 | DDL 现场记录尚未绑定 Export task/history；DDL + CSV 仍缺独立 Golden Path、失败与结果投影 |
| OSS / S3 / COS / OBS | SYNTHETIC_ONLY，提交受门禁 | [EX-I6 存储预检查框架](../03-technical/evidence/exi6-storage-precheck-framework-2026-08-14.md) | 真实 endpoint、凭据探测、四类 provider 端到端和远端结果事实 |
| `dump.ckpt` 继续 | SYNTHETIC_ONLY | [EX-I8 结果与恢复](../03-technical/evidence/exi8-derivation-result-recovery-2026-08-14.md) | 慢导出/中断产生保存点后的真实继续任务 |
| `--query-sql` | IMPLEMENTED，EVIDENCE_PENDING | 普通高级参数：NORMAL、普通任务授权、无 capability/二次确认；已覆盖直接文本、file:// 拒绝、互斥、预检查与提交回归 | 真实工具行为仍按 EX-V1 授权验证 |

“正式支持候选”表示产品范围可以继续评审，不表示当前已通过 EX-V1 或 G3。每一行的状态必须由相应证据、契约测试和发布门禁共同更新。

## 1.2 Phase 2 合成夹具与回归基线（2026-08-20）

本阶段复用仓库已有的最小合成夹具，不新建会连接真实数据库、解析真实凭据或启动官方工具的通用 fixture 层。测试中的路径、端点、账号、密码和任务标识均为合成值；涉及临时目录的用例由测试框架在隔离目录中创建并在结束后清理。

| 覆盖面 | 现有夹具/测试入口 | 保护的回归事实 |
|---|---|---|
| 导出草稿归一化、范围/内容/格式和失败关闭 | `internal/exportdomain/selection_test.go`、`internal/exportdomain/command_test.go`、`internal/controlplane/server_test.go` 中 `TestExportDraft*`；`web/src/views/exportDraftInput.test.ts`、`exportDataSourceEligibility.test.ts` | v5/v6/v7 兼容、格式互斥、对象范围、输出边界和数据源资格不被放宽 |
| 命令生成与脱敏展示 | `internal/commandgen/generator_test.go`、`internal/exportdomain/command_test.go` | 执行 argv 与脱敏预览由同一生成路径产生，秘密不进入 argv/展示；`--query-sql` 作为普通参数经规范化进入统一生成器 |
| query-sql 普通授权回归 | `internal/controlplane/server_test.go`、`internal/store/store_test.go`、`web/src/api/browser.test.ts` | 普通任务授权足够，旧确认字段仅兼容解码且不决定提交结果；直接文本、file:// 和互斥组合均有回归覆盖 |
| 结果事实、存储输出门禁和秘密生命周期 | `internal/agentexecution/worker_result_test.go`、`internal/agentexec/*_test.go` | 文件摘要、检查点、对象存储 URI 边界和秘密清理保持失败关闭 |
| 结果证据与权限投影 | `internal/store/store_test.go`、`internal/controlplane/server_test.go`、`web/src/api/browser.test.ts`、`contracts/openapi_test.go` | 退出码、时间派生耗时、固定错误码、创建者/授权读者输出位置裁剪和 planned/actual 摘要不一致失败关闭；不写入原始 argv 或错误 |
| Export 取消与结果证据 | `internal/store/store_test.go`、`internal/agentexecution/worker_test.go`、`web/src/api/browser.test.ts` | 排队/运行中取消、期限、幂等、固定错误码和浏览器投影失败关闭；真实工具树终止仍需 EX-V1 |
| API/OpenAPI 与预检查契约 | `contracts/openapi_test.go`、`internal/precheckcontract/*_test.go`、`internal/agentwire/precheck_test.go` | 浏览器/Agent 契约和预检查绑定不漂移 |
| 前端预检查与摘要投影 | `web/src/views/exportPrecheckPresentation.test.ts` | 预检查结果、风险摘要和提交前展示保持稳定 |
| query-sql 向导呈现 | `web/src/views/exportDraftInput.test.ts`、`ExportWizardView.vue` | 步骤 3 可直接编辑，冲突项即时禁用；提交不再依赖敏感确认 |

本轮实际执行的基线命令及结果：

```text
go test ./cmd/... ./contracts/... ./internal/... ./migrations/...                                                        PASS
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...                                                         PASS
./scripts/check-secrets.ps1                                                                                                  PASS
cd web; npm run test -- --run                                                                                               PASS (20 files, 140 tests)
cd web; npm run typecheck                                                                                                    PASS
cd web; npm run lint                                                                                                         PASS (--max-warnings=0)
cd web; npm run build                                                                                                        PASS
git diff --check                                                                                                             PASS
./scripts/verify-export-synthetic.ps1                                                                                        PASS (S0/S1)
```

该基线是收敛重构的回归门槛，不是正式格式支持或真实端到端证据。任何后续提取领域规则、增加高风险门禁、实现取消或调整结果投影的改动，都必须先保持这组合成回归通过，再补对应 S2-S4 或 EX-V1 受控证据。

## 1.3 Golden Path 当前覆盖

固定链路保持为：

```text
已授权数据源 → 内容/范围/对象 → 数据格式 → 本地输出/执行节点
→ 服务端校验 → 脱敏命令预览 → Agent 预检查
→ 冻结任务提交 → 固定信封执行 → 状态/脱敏日志/结果/历史
```

| Golden Path | 当前状态 | 证据或缺口 |
|---|---|---|
| GP-01 单表 CSV 本地成功 | VERIFIED | Windows 单表 CSV 成功与特殊值两次独立导出 |
| GP-02 多对象 CSV/POS | PARTIAL | POS 已有 6 表定长输出；完整向导到任务历史的多对象回归待补 |
| GP-03 DDL_ONLY / DDL_AND_DATA(CSV) | PARTIAL | DDL_ONLY 有 EX-I7 真实工具输出，但尚未形成产品任务历史证据；DDL_AND_DATA(CSV) 缺独立 Golden Path 证据 |
| GP-04 每个正式候选格式至少一次真实结果 | PARTIAL | CSV/POS 有结果；CUT/SQL 尚无逐格式真实结果，结构化格式仍门控 |
| GP-05 预检查失败不创建任务、不启动工具 | VERIFIED（局部） | [低权限对象访问验证](../03-technical/evidence/windows-low-privilege-object-access-validation-2026-08-05.md) |
| GP-06 工具失败、取消、重启和历史回看 | PARTIAL | 工具失败、排队/运行中/超时取消已有合成与局部协议证据；真实工具进程树终止、重启恢复和检查点继续仍待 EX-V1 |

取消的合成验收要求：排队任务直接 `CANCELLED`；运行中任务先 `CANCELLING`，仅在 `PROCESS_CANCELLED`、`PROCESS_EXITED` 和结果事实闭合后 `CANCELLED`；重复幂等键重放同一结果；不同摘要冲突；轮询期限到期失败关闭并标记核对。任何页面按钮、父进程退出码或 Agent 自报状态都不能单独证明取消成功。

## 1.4 运行态核对（2026-08-20）

本次使用内置浏览器和按当前源码重建的本机 Local MVP 做受控合成运行态核对：

| 面 | 结果 | 证据边界 |
|---|---|---|
| 控制面与页面代理 | `8080` 控制面和 `5173` Vite 页面均可用 | 只验证健康与页面读取，不改变任务数据 |
| 数据源页面 | 已授权数据源列表可读取，数量投影正常 | 不记录端点、用户名、凭据或业务对象 |
| Export 向导 | 内置浏览器完成合成数据源选择、对象/内容/格式/输出配置、草稿创建和第 6 步命令预览；六步信息架构可达 | 仅写入本机合成草稿；未执行预检查、未提交任务，不发起 Agent、OBDUMPER 或真实连接 |
| 执行节点 | 初始页面显示 Agent 离线；临时启动已登记 Agent 做心跳核对后，页面投影为在线/可接收（仍需任务级预检查）；核对完成后已停止该临时 Agent | 不把在线心跳外推为真实工具可用，不留下开启真实执行开关的常驻进程 |
| 真实执行 | 本次未提交任务、未解析凭据、未连接数据库、未启动 OBDUMPER | EX-V1 逐格式、对象存储和检查点证据状态不变 |

这次运行态证据只证明当前页面和只读 API 投影可用；它不替代 Agent 在线、真实工具执行或用户当次授权的 EX-V1 现场证据。

## 2. 安全边界

执行验证前必须同时满足：

1. 使用非生产 OceanBase 测试租户和专用测试账号。
2. 仅使用合成数据，不读取真实业务表、真实个人信息或生产对象。
3. 输出到独立测试目录或专用测试 Bucket，不复用历史任务目录。
4. 本地输出根目录必须是明确、可回收的测试子目录；不得使用工作区根、用户目录根或系统根目录。
5. 对象存储使用最小权限临时凭据，证据中统一脱敏。
6. 不使用 `--skip-check-dir` 覆盖已有数据；每个用例使用新目录。
7. 不验证自动授权、数据库调参、性能压测或大数据量极限。
8. P2 help-only 参数在官方或厂商未解释语义前，不对真实数据执行。
9. 清理测试对象和输出由执行者按已确认清单单独操作，不把清理命令混入验证命令。

### 2.1 S0-S4 验证分级

| 等级 | 允许的验证输入 | 是否需要人工确认 | 固定边界 |
|---|---|---|---|
| S0 | 版本化 fixture、假 Agent、假工具、临时目录 | 否 | 不连接网络、不解析秘密、不写用户数据 |
| S1 | 脱敏命令预览、规则校验、历史 fixture 重放 | 否 | 不建立真实连接、不启动工具；失败提供稳定错误码 |
| S2 | 登记节点、非生产数据源、真实工具或受控 endpoint | 一次有界授权 | 授权绑定对象、节点、动作和期限；范围内不重复确认 |
| S3 | 契约明确列入敏感命令或高影响能力清单的操作 | capability + 提交级二次确认 | 绑定不可变风险指纹；`--query-sql` 不属于本级 |
| S4 | 生产、删除、覆盖或不可逆动作 | 独立明确授权和二次确认 | 最小权限、回滚/恢复方案和完整证据；未满足即阻断 |

S0/S1 的无人工审批只适用于无真实副作用路径，不能关闭身份隔离、秘密生命周期、命令生成器或 Agent 执行边界。

## 3. 环境与测试数据基线

### 3.1 环境记录

每轮验证必须记录：

| 项目 | 必填内容 |
|---|---|
| 发布包 | 文件名、SHA-256、`obdumper --version` |
| 运行节点 | 操作系统、CPU 架构、Java 版本、节点标识 |
| OceanBase | 数据库版本、MySQL/Oracle 兼容模式、连接方式 |
| 网络路径 | 私有 ODP；对象存储类型（如适用） |
| 会话配置 | 使用的 session 配置文件版本或“官方默认” |
| 测试时间 | 开始、结束时间及时区 |
| 执行人 | 操作者和复核人 |

当前本地发布包基线：

- 版本：`4.3.5-RELEASE`；
- SHA-256：`C1A5D5EE053106803263015F00EC7F0B94B73EA4015A2E53CF1AFFF598983493`；
- help 长参数：109 个唯一名称。

### 3.2 最小合成数据集

| 测试对象 | 用途 | 最小特征 |
|---|---|---|
| vt_text | CSV/CUT/POS/压缩 | 数字、ASCII、中文、NULL、空串、分隔符、引号、换行 |
| vt_split | 文件切分 | 至少 5 行固定小记录，便于使用 ROW 阈值 |
| vt_empty | 空结果 | 空表，或使用永假 where 条件 |
| vt_partition | 分区与空文件 | 至少一个有数据分区和一个空分区 |
| vt_no_pk | 隐藏主键 | 无主键小表 |
| vt_sequence | 序列策略 | Oracle 模式测试序列，记录当前值与增量 |
| vt_ddl | DDL 差异 | 含表组、primary zone 或可观察额外属性的小表 |

本轮只定义数据特征，不在项目中创建数据库对象或测试脚本。

## 4. 证据规范

每个用例必须保存一个证据记录，至少包含：

- 用例 ID、执行状态和结论；
- 工具版本、发布包哈希、数据库版本和兼容模式；
- 脱敏后的完整命令；
- 参数来源：用户显式、数据源派生、节点派生或官方默认；
- 退出码、标准输出、标准错误和 OBDUMPER 日志；
- 输出目录树、文件数量、文件大小和必要的内容摘要；
- 对比用例需记录文件哈希或规范化内容差异；
- 实际开始/结束时间；
- 结论：通过、失败、不确定、环境阻断或需官方确认；
- 对产品文档和参数元数据的建议回写项。

敏感信息不得写入 Markdown、截图、普通日志或提交记录。

## 5. P0：命令定版用例

### 5.1 POS 映射

当前证据已覆盖独立 `--pos --ctl-path` 的命令映射和定长输出（见 [POS 受控实测与定版](../03-technical/evidence/windows-pos-format-validation-2026-08-07.md)）；EVT-P0-02 的 CUT 等价组合和自动生成控制文件实现仍不作为当前 V1 可用路径。以下用例表保留为可复核的验证计划，不把已有 POS 证据扩写成所有 POS 组合均已通过。

| ID | 验证问题 | 最小操作 | 通过标准 | 回写位置 |
|---|---|---|---|---|
| EVT-P0-01 | 独立 `--pos` 是否可导出定长文件 | vt_text + 控制文件，执行独立 `--pos` | 命令成功；列宽、中文、NULL 和换行符合控制文件；输出可重复 | 参数 `--pos`、EX-C05、EX-F018 |
| EVT-P0-02 | 官网 CUT 组合是否可导出 POS | 同一数据使用 `--cut`、空 splitter 和同一控制文件 | 命令成功且结果符合定长规则 | `--column-splitter`、EX-F048 |
| EVT-P0-03 | 两种 POS 入口是否等价 | 规范化比较前两项输出和日志 | 数据内容、列宽和异常处理等价，或明确记录差异 | EX-R03、EX-FR10 |

命令模板：

```text
obdumper <连接参数> --table vt_text --pos --ctl-path <控制文件目录> -f <新输出目录>

obdumper <连接参数> --table vt_text --cut --column-splitter ""   --ctl-path <控制文件目录> -f <新输出目录>
```

定版规则：

- 只有独立 `--pos` 通过：使用 `--pos`，CUT 组合保留为历史资料差异。
- 只有 CUT 组合通过：页面仍可叫 POS，命令生成 CUT 组合。
- 两者都通过且等价：优先采用 V4.3.5 当前二进制最直接、日志语义最清晰的入口，并记录兼容方案。
- 任一结果不稳定：V1.0 隐藏 POS 或继续阻断提交。

### 5.2 block-size 默认值与切分

| ID | 验证问题 | 最小操作 | 通过标准 | 回写位置 |
|---|---|---|---|---|
| EVT-P0-04 | 未指定时实际默认行为 | vt_split 不指定 `--block-size` | 记录是否切分及日志中的生效值 | `--block-size` 默认来源 |
| EVT-P0-05 | `0` 是否表示不切分 | 显式 `--block-size 0` | 单对象输出不按阈值切分 | EX-F031 |
| EVT-P0-06 | ROW 单位是否生效 | 显式 `--block-size 2ROW` | 5 行数据生成符合阈值的逻辑子文件 | 字段类型与校验 |
| EVT-P0-07 | MB 单位是否合法 | 小数据执行 `1MB`、`1`，仅验证解析与日志 | 两种官方写法行为一致；非法 M/GB 被拒绝 | 输入格式约束 |
| EVT-P0-08 | ORC/Parquet 是否忽略切分 | 分别指定 ORC、Parquet + 小阈值 | 命令行为与官网“不生效”一致且不误导 | EX-C11 |

在 EVT-P0-04 完成前，页面只显示“官方默认存在资料冲突”，不得预填 0 或 1024MB。

### 5.3 all 与列式格式

| ID | 验证问题 | 最小操作 | 通过标准 | 回写位置 |
|---|---|---|---|---|
| EVT-P0-09 | `--all --orc` 是否导出全部表数据 | 最小数据库执行组合 | 成功导出全部适用表数据，不误导为 DDL | 格式与 `--all` 组合 |
| EVT-P0-10 | `--all --par` 是否可用 | 同上 | 成功且对象范围正确 | 同上 |
| EVT-P0-11 | `--all --avro` 是否可用 | 同上 | 成功且对象范围正确 | 同上 |
| EVT-P0-12 | `--all --ddl` + 列式格式 | 分别组合 ORC/Parquet/Avro | DDL 与表数据均产生，或得到明确不支持证据 | 内容模型与格式约束 |

未通过的组合应在格式卡片或全部导出场景中禁用，而不是延迟到工具执行失败。

### 5.4 空文件参数

| ID | 验证问题 | 最小操作 | 通过标准 | 回写位置 |
|---|---|---|---|---|
| EVT-P0-13 | 复数参数是否在空 where 结果生效 | `--where` 永假条件 + `--retain-empty-files` | 成功生成预期空文件 | 参数拼写与 EX-F033 |
| EVT-P0-14 | 空分区是否生成文件 | 空分区 + `--retain-empty-files` | 生成空文件且非空分区结果正常 | partition 条件 |
| EVT-P0-15 | 单数拼写是否应拒绝 | 仅做参数解析验证，不执行真实导出 | 单数被明确拒绝或记录兼容行为 | 历史命令识别 |
| EVT-P0-15A | CSV 空表表头行为 | CSV 空表 + `--retain-empty-files` | 生成仅包含表头的空数据文件 | 步骤 5 说明、结果证据 |
| EVT-P0-15B | CSV 空表与 skip-header | CSV 空表 + `--retain-empty-files --skip-header` | 生成完全空文件 | 参数联动 |
| EVT-P0-15C | CUT 空表 | CUT 空表 + `--retain-empty-files` | 生成可识别的空 CUT 文件 | 格式适用性 |
| EVT-P0-15D | SQL 空表 | SQL 空表 + `--retain-empty-files` | 生成可识别的空 SQL 数据文件 | 格式适用性 |
| EVT-P0-15E | 仅结构任务携带 retain-empty-files | 只做规范化与命令测试 | 前端禁用，服务端阻断或转为不活动，命令不生成该参数 | 内容活动条件 |

产品生成器始终使用复数 `--retain-empty-files`。

### 5.5 序列、紧凑 Schema 和连接表达

| ID | 验证问题 | 最小操作 | 通过标准 | 回写位置 |
|---|---|---|---|---|
| EVT-P0-16 | sequence-policy preserve | 导出 vt_sequence DDL | DDL 保留当前序列语义，结果可解释 | `--sequence-policy`、EX-F069 |
| EVT-P0-17 | sequence-policy restart | 同一序列使用 restart | 与 preserve 产生可解释差异 | EX-R17 |
| EVT-P0-18 | compact-schema 的差异与风险 | vt_ddl 普通 DDL 与 compact 对照 | 记录缺失属性、性能和适用边界 | `--compact-schema`、EX-F068 |
| EVT-P0-19 | 独立 tenant 长参数优先级 | 对比组合 user 与独立 tenant/cluster | 连接目标一致，或明确优先级/冲突 | 连接派生规则 |
| EVT-P0-20 | logical-database 派生 | 仅在 ODP Sharding 测试环境验证 | 参数由数据源类型正确生成，结果警告符合事实 | EX-F005、EX-FR07 |

若没有 Oracle 或 ODP Sharding 测试环境，对应用例标记“环境阻断”，相关字段保持受限，不得猜测通过。

## 6. P1：高级与专家能力

| ID | 验证主题 | 重点证据 | 未通过时产品处理 |
|---|---|---|---|
| EVT-P1-01 | 对象存储 `--tmp-path` | 临时分块确实写入指定路径，空间不足错误可识别 | 隐藏自定义临时目录，继承节点配置 |
| EVT-P1-02 | 压缩算法与等级 | zstd/zlib 边界值有效；gzip/snappy 不接受等级 | 按实际算法收缩字段 |
| EVT-P1-03 | `--parallel-macro` 适用条件 | 日志或行为能证明宏块处理语义 | 字段保持隐藏 |
| EVT-P1-04 | snapshot 与闪回组合 | 互斥、优先级或组合行为有明确证据 | 只开放已验证单一入口 |
| EVT-P1-05 | 检查点 retry | 失败后生成 dump.ckpt，原参数可继续 | 不展示检查点继续 |
| EVT-P1-06 | `--query-sql` 直接文本与 file:// 边界 | 直接文本正例；file://、where/partition/flashback 冲突负例 | 直接文本按普通高级参数进入命令；file:// 和全部冲突组合在工具启动前阻断 |
| EVT-P1-07 | hidden PK | 无主键表、版本和权限条件可验证 | 隐藏 `--enable-hidden-pk` |
| EVT-P1-08 | 单文件 `--file-path` | 指定文件名时目录和日志位置符合官网 | 仅允许目录路径 |
| EVT-P1-09 | sys 权限与 extra message | 有/无 sys 权限的差异可归类 | 无权限时禁用并阻断 |
| EVT-P1-10 | 多 Host / 多端口 | 一端口与多端口的匹配规则可重复 | V1.0 收缩为单 Host/端口 |

## 7. P2：兼容调查

| ID | 参数 | 调查方式 | V1.0 默认处理 |
|---|---|---|---|
| EVT-P2-01 | `--server` | 先取得官方/厂商书面语义，再做隔离环境验证 | 不显示、不生成 |
| EVT-P2-02 | `--public-synonym` | 先确认对象类型、模式和版本，再验证 DDL | 不显示、不生成 |
| EVT-P2-03 | `--commit-size` | 先确认是否仅属 Loader 共享解析器 | 不显示、不生成 |
| EVT-P2-04 | 废弃存储参数 | 只验证历史命令识别，不验证新能力 | 不提供新建入口 |
| EVT-P2-05 | 导入侧参数出现在 obdumper help | 验证平台过滤清单，不执行导入行为 | 永不进入导出表单 |

“解析器接受”不构成产品能力确认。P2 参数只有在当前版本官方语义、适用条件和输出行为均可验证后，才允许重新进入产品评审。

## 8. 结果判定

| 状态 | 判定条件 | 产品动作 |
|---|---|---|
| 通过 | 至少两次独立执行结果一致，输出和日志均满足预期 | 更新参数事实、字段矩阵和约束状态 |
| 失败 | 工具明确拒绝，或输出与官方语义冲突且可重复 | 隐藏/禁用对应能力，记录失败证据 |
| 不确定 | 结果受环境、权限或数据影响，不能区分参数行为 | 保持待实测，不开放 |
| 环境阻断 | 缺少 Oracle、ODP、对象存储等必要环境 | 保持受限并记录所需环境 |
| 需官方确认 | help-only 或多种行为均可解释 | 不执行真实数据验证，先取得官方语义 |

同一用例只成功一次、只有退出码为 0、只看到参数被解析或只有日志无输出，都不足以判定“通过”。

## 9. 文档回写规则

验证完成后按以下顺序回写：

1. [导出模块 Canonical](export-module.md)：更新含义、默认值、适用范围、显示/清值/阻断规则、格式卡片和待实测清单。
2. [通用导出技术契约](../03-technical/export-general-contract.md)：更新机器校验、命令、Agent、结果和取消证据。
3. `docs/archive/export/research/`：只在需要追溯参数研究或字段编号时追加历史记录，不作为当前发布态事实源。
4. [关键决策](../01-product/decisions.md)：只有验证结果要求改变已确认产品规则时，才新增或修订决策。
5. [文档中心](../README.md)：更新模块与验证状态。

每项回写必须引用 EVT 用例 ID，避免把一次人工尝试写成无来源结论。

## 10. 执行前门槛

当前仍有未执行的计划用例；已有证据不能替代剩余真实验证。开始新的实测前还需要：

- 非生产 OceanBase 连接和执行授权；
- MySQL 兼容测试租户；涉及序列和时间戳时需要 Oracle 兼容测试租户；
- 允许创建最小合成测试对象的专用账号；
- 明确的执行节点与隔离输出目录；
- 如验证对象存储，提供专用 Bucket 和临时最小权限凭据；
- 确认测试数据与输出的保留、复核和清理责任人。

在这些条件满足前，文档工作可以继续，但不得把任何计划用例标记为已通过。
