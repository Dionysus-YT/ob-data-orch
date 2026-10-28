# MySQL Schema 前缀修复验收（2026-10-27）

本轮关闭了[前次真实复验](export-runtime-acceptance-2026-10-26.md)唯一剩余的 `retain-schema` 失败。**6 个新真实任务、3 个 API 拒绝反例和 1 个界面联动场景，共 10 PASS、0 FAIL**。四个保留前缀任务均核对实际 `CREATE TABLE` 的限定目标，CSV/CUT/SQL 的完整 4 行一致。原失败任务及历史 172 项结果保留；未重跑全矩阵，不提升完整 EX-V1、G3/G4 或发布门禁。

工作于 2026-10-26 开始，安装与任务执行在当晚完成，2026-10-27 完成独立内容核对、清理与文档收口。文件名保留开始日期，供已安装的不可变 v12 资源引用。

## 授权、范围与根因

沿用用户本会话“授权一切操作验证”，使用正常登录会话。范围仅为既有 TEST `test-mysql` / ODP、Windows AMD64 `win-test`、OBDUMPER 4.3.5 和登记的 Java `1.8.0_491`，以及本次独立 owner 夹具。不修改业务库、账号权限或一次性审计授予状态。短时辅助进程使用既有密文/根密钥，在内存解析源凭据并经匿名 stdin 交给固定夹具程序，秘密不进入 argv、环境、文件或输出。

固定 4.3.5 的 MySQL compact 路径使用 `SHOW CREATE TABLE`，结果没有 Schema 限定；retain-schema 只能保留已有前缀，不会在该路径补齐。对安装的原 JAR 做只读字节码核对：`ObMysqlCompactSchemaDumpTask` 使用原生建表语句，`ObMysqlSchemaDumpTask` 使用元数据重建。未修改官方 JAR、启动主类或导出的 DDL。

不连接数据库的固定参数解析探针确认：

| 输入片段 | 实际解析 |
| --- | --- |
| 未设置 compact-schema | getter 为 null，工具后续使用原生路径默认值 |
| `--compact-schema` | true |
| `--compact-schema false` | 拒绝；不能作为两令牌生成 |
| `--compact-schema=false` | false，选择元数据重建路径 |

官方 [4.3.6 参数说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000005770341)仅辅助理解 retain-schema 的意图；本轮 4.3.5 结论依据原二进制、固定解析探针和以下实际任务，不由相邻版本文档推导。

## 实现边界与历史兼容

- 新增 `obdumper-4.3.5-slice-v12-schema-boolean`，继承 v11，仍为 79 项定义；只有 compact-schema 的 flag 可声明 `explicitBoolean`。版本、定义或类型不匹配失败关闭，旧资源不改写。
- 领域规则为 MySQL retainSchema 派生 false，唯一生成器输出 `--retain-schema` 与单令牌 `--compact-schema=false`。显式原生 true 使用 `--compact-schema=true`，未配置仍省略；false 不能被折叠成未设置，参与确定性指纹。
- MySQL retain/native true 冲突、DATA_ONLY 的显式 compact 值、独立 compact=false 均在草稿入口拒绝。本轮只开放保留前缀所需的关闭分支，不新增界面无法保持的独立重建模式；Oracle retain 不派生 false。元数据重建可能需要系统视图读取权限，当前环境的正例不证明所有授权环境都可执行。
- 正常交互启用保留前缀会清除并禁用原生选项，关闭后恢复可选；回填历史冲突值时不悄悄改写，由校验阻断保存。步骤沿用 Ant Checkbox/Tooltip 和原表单反馈，无新外壳、Token 或状态副本。
- 旧草稿只读预览和冻结任务保留元数据、命令及指纹；明确 PATCH 保存新结构策略修订才选择 v12，并使旧预检失效。v7/v11 原 true/false 令牌与指纹有兼容测试，实际旧任务读取也核对三者未变。
- 权限分类按实际专家参数判断；明确普通的 v12 不自动升级权限，缺少分类仍保守回退。工具与兼容页读取同一真实目录。没有新 API、迁移、依赖、任意 JVM 参数、Shell 拼装或 DDL 后处理。

前端 `useExportParameters` 仍是交互清值所有者，`exportDraftInput` 负责纯校验/DTO，步骤只消费表单；服务端策略归 `exportdomain`，参数发射归唯一生成器。两项架构审计的规模 REVIEW 已人工复核，本次没有新增独立异步职责、复制表单或循环依赖，不为行数拆分无关模块。

## 实际场景与内容核对

六个任务均通过七项完整匹配的预检查，受控进程退出 0，终态 `SUCCEEDED`、命令 `MATCHED`、结果 `VERIFIED`，无待核对。独立文件检查在清理前完成，不以任务成功或文件名包含库名代替 DDL 内容核对。

| 场景 | 实际任务 ID / 结果 | 独立输出证据 |
| --- | --- | --- |
| RS-BASELINE，仅结构默认路径 | `bc77b6c5-04ac-4ee8-9ae4-a9b3a798d647` / PASS | v7；1 文件、495 B；不含限定目标，符合未保留前缀预期 |
| RS-NATIVE，仅结构显式原生 | `0b54a444-10bd-4e76-83da-602d1fb8198d` / PASS | v12 true；1 文件、495 B；原生定义与默认路径一致 |
| RS-RETAIN，仅结构保留前缀 | `ace33bad-d3b7-4ed1-80e8-9f926f0e006f` / PASS | v12 false；1 文件、299 B；实际 CREATE TABLE 为本次库和表的限定目标 |
| RS-RETAIN-CSV，结构加 CSV | `2cf5ec90-2ada-4678-bf01-8b62dfd0d9de` / PASS | 3 文件、564 B（含零字节 MANIFEST）；限定 DDL + 完整 4 行 |
| RS-RETAIN-CUT，结构加 CUT | `bd2a58b0-aaac-4012-b9ce-68621942a907` / PASS | 2 文件、478 B；限定 DDL + 完整 4 行 |
| RS-RETAIN-SQL，结构加 SQL | `d0c597db-3252-4992-9cc0-84f521b548fe` / PASS | 2 文件、894 B；限定 DDL + 完整 4 行 |
| RS-CONFLICT，retain + 原生 true | 422 `EXPORT_DRAFT_REJECTED` / PASS 反例 | 无预检查或工具任务 |
| RS-FALSE-SCOPE，独立 false | 422 `EXPORT_DRAFT_REJECTED` / PASS 反例 | 无预检查或工具任务 |
| RS-DATA-FALSE，DATA_ONLY + false | 422 `EXPORT_DRAFT_REJECTED` / PASS 反例 | 无预检查或工具任务 |
| 保存恢复与选项切换 UI | PASS | 恢复派生 false；关闭 retain 后原生可选，先开原生再开 retain 时原生清除且禁用 |

四个保留前缀任务的 DDL SHA-256 均为 `aff4f8497e63d49f581696ff95f0e3526cece50b363a77d892afab349cb53fe0`；三种数据格式规范化完整行集摘要均为 `c36f8748fdad61ee59b877c31f105d5a9681c019a0142dcaebb4eb06e5de8837`。校验器识别实际 CREATE TABLE 目标，忽略注释和字符串，错误库/表、仅注释出现前缀均拒绝；该检查覆盖 DDL_ONLY 和 DDL_AND_DATA。

浏览器长批次曾失去调试附着，未保存的内存批次回执不冒充完整证据。之后根据本次精确输出边界只读定位六个持久任务，通过同一正常浏览器授权的公共 API 重新获取执行、快照、命令、七项预检及全部分页日志，证据标记 `PERSISTED_API_AFTER_BROWSER_ATTACHMENT_LOSS`；三个 API 负例独立再次读取，未生成新任务。界面使用 Edge/CUA 正常操作，没有拦截 API 或假 Agent。首张交互截图处于过渡态，另存为 initial；最终采用恢复后的稳定截图及“结果已核验”任务详情截图。

## 检查、安装与清理

| 检查 | 实际结果 |
| --- | --- |
| Go 单元/契约 | 51 包、1,983 项含子项通过，0 失败；4 项既有测试跳过（1 GiB 可选物理检查、2 项 Windows 符号链接权限、Vite HMR 可选检查），另 4 包无测试文件。最终权限 SQL 分支补充断言后定向测试通过 |
| 静态与前端 | 全量 vet 通过；受影响前端 97 项单测、类型/生产构建和全量 lint 通过；audit:wizards / audit:business 均 0 违规，分别 4 / 23 项 REVIEW；构建既有大 chunk/插件提示不作为失败 |
| E2E 检查器 | Node 14 项通过，含注释/字符串伪前缀和错误 CREATE 目标负例 |
| 秘密与差异 | 仓库秘密扫描、最终差异检查通过；17 份阶段 JSON 证据和全部 34 个所属文件在短时内存核对，未发现已知源秘密或其可恢复编码 |
| 历史与目录 | 公共 API 读取确认旧任务元数据、指纹和脱敏命令未变；当前目录列出 v12；没有改写原失败结果 |
| 服务 | 原 CA 验证 HTTPS 健康 200；Agent ACTIVE、心跳新鲜、占用 0；六任务无待核对，全平台无活动任务 |
| 身份、配置、库 | 六个原配置/根密钥/信任文件摘要不变；机器身份、凭据摘要/修订、主体授权与对象范围和关闭备份一致；42 项迁移、43 张 STRICT 表，审计一次性授予未消费 |
| 清理 | 精确核对 owner 后删除本次夹具库并确认不存在；所属 34 文件、58,679 B 在全量内容/摘要核对后删除，13,413,376 B 辅助成品删除；一次性界面草稿通过正常删除事务软删除，API 404，数据库无活动行。备份、身份、配置与任务历史保留 |

安装遵守停止旧 Agent/控制面及监视器、确认端口关闭、一致备份、删除旧二进制、当前源码 `CGO_ENABLED=0 go build -trimpath`、按原参数/环境开关/工作目录恢复的流程。停服时无活动任务，备份目录为 `var/dev/control-plane/backups/retain-schema-20261026-223538`。五个已安装/下载成品的构建元信息均确认 CGO=0 和 trimpath：

| 成品 | SHA-256 |
| --- | --- |
| 控制面 Windows AMD64 | `49cac40777de588d1e89fade1ab2421a863fb3bd050908c438ff4c4eeb397454` |
| 已安装 Agent / Windows AMD64 下载资源（相同） | `785208c3b5b84549ca6eedcdffcf151f3ddcdb58fd0d9619c21159fd6fdd2f07` |
| Linux AMD64 下载资源 | `b5d8c60e90d46b99addcf677103979b470211ea2a38880680905a39192b4df52` |
| Linux ARM64 下载资源 | `d546393ade9e12eee254d4229365eaba7e92335a169885c953a12b6023527fe9` |

Linux 仅交叉构建，不代替麒麟原生验证。一次终端 curl 健康核对被 Windows 吊销状态未知中止，随后使用与现有开发启动器一致的 Node HTTPS 客户端及原 CA 取得 200，未关闭证书校验。只读收尾脚本初版误要求草稿物理删除，按既有软删除契约改为核对 `deleted_at` 与 API 404 后通过；没有为通过检查修改数据库。固定解析探针的两次命令准备错误也已保留为过程事实，不计入产品场景。

本机脱敏证据位于忽略目录 `artifacts/retain-schema-20261026-9966826231e7/`：`verified-results.json`、`task-map.json`、`public-evidence.json`、`negative-results.json`、`history-and-cleanup-readback.json`、`audit-summary.json`、`output-inventory.json`、清理回执、`services-final.json`、`health-final.json`、`installed-builds.json` 和 `go-test-summary.json`。外部 `ob-data-orch-retain-schema` 证据目录保留 Go JSONL、只读解析探针、最终截图和源码状态快照；不提交原始工具输出、数据库文件或安全材料。清理后不能对旧输出再次运行内容收集器，复验须新 owner 夹具。

工作树包含前序和用户改动，源码快照仅用于复核，不作为改动归属声明；未提交、推送或创建 PR。本轮没有新增 Oracle、跨库、其他对象、对象存储、真实活跃租约恢复、检查点继续、跨主体身份或麒麟原生证据；Windows Java 8 不可表示 argv 仍按既有规则拒绝，这些限制不因前缀正例而解除。
