# 普通导入技术契约

2026-10-20 十九项补齐后，当前72/72定义已有成功真实代表场景，新增十九项19 PASS及八项拒绝反例均经独立数据库事实/持久事件核对；所有本轮夹具已清理，运行二进制与当前源码隔离重建摘要一致。固定协议更新为v11，受控语法、摘要和权限边界见下文；实测范围和历史失败见[十九项补齐证据](evidence/normal-import-capabilities-validation-2026-10-20.md)。不据代表场景声称全取值与所有组合通过。

2026-10-20 API 补验整改：`compatMysql=true` 时新增活动字段 `compatMysqlVersion`，仅接受 `mysql56/mysql57/mysql80` 并原值生成 `--compat-mode`。旧开关缺少版本可恢复为未完成草稿，但完整预览、预检查和提交必须拒绝；关闭开关或模式/内容不适用时清除版本，不改历史快照。CUT 隐式行分隔符按目标平台解析（Windows CRLF、Linux LF），仅作用于检查副本，不改命令/指纹；输入换行不符或未知平台保持 `INPUT_MAPPING/UNKNOWN/IMPORT_INPUT_MAPPING_UNSUPPORTED`。显式 `\n` 可用于 Windows 上的 LF 文件。检查仍只证明有界记录前缀，完整数据保真须独立结果核对。实际结果见[API 全参数验证](evidence/normal-import-full-parameter-validation-2026-10-20.md)。

2026-10-19 按用户授权完成现有非生产 MySQL 数据源的 Windows 本地真实 E2E，20 项为 16 PASS/4 FAIL。CUT、SQL、仅结构及全字段加引号 CSV 已核对本轮完整行集/结构；未加引号 CSV 空格保留、显式 `strict=true` 生成表达与重跑拒绝的 HTTP 分类仍有失败。范围、真实任务、运行版本与清理见[验证证据](evidence/normal-import-mysql-e2e-2026-10-19.md)。本轮只验证和记录，不修改产品代码或提升完整 NIV/其他平台/恢复准入。

2026-10-15 用户在前端重构完成后明确要求继续实现。普通导入首先进入 G2 隔离实现，范围仍为私有 ODP、本地 CSV/CUT/SQL 和独立五步向导。2026-10-16 补齐三项只读预检查、第五步自动准备和提交时可选命名，随后按用户明确授权接入 Windows 普通导入执行切片；安装与真实结果见[执行证据](evidence/normal-import-execution-2026-10-16.md)。普通导入执行资格仍由独立门禁失败关闭，不提升尚未验证的格式、平台或恢复能力。

## 领域与参数

普通导入配置版本为 `normal-import-v1`，工具为 `OBLOADER 4.3.5-RELEASE`，独立元数据为 `obloader-4.3.5-normal-v1`，能力为 `import-odp-local-v1`。领域配置不接受导出/旁路参数、任意 argv、任意 SQL、远程 URI、秘密或任务级连接覆盖。活动投影、范围/格式/兼容模式校验和字段映射由 `internal/importdomain` 唯一负责；命令仍经 `internal/commandgen` 生成，密码只以官方安全文件槽位表示。未设置值继承工具默认。

服务端 72 条定义与页面 76 项说明的差额为 sys 账号/密码、字符集与会话配置四项受管说明；它们不是普通导入任务覆盖。当前能力固定派生 `--no-sys`，不借用导出的 sys 密码。生成器自身再次拒绝内容/格式/映射冲突、无效数值与作为文本值注入的选项，调用方不能绕过领域规则传入旁路或恢复参数。

## 草稿与浏览器 API

`/api/v1/normal-import-drafts` 提供创建和所有者列表，`/{id}` 提供读取、If-Match 更新与删除；`:preview-command`、`:discover-files`、`:precheck` 和 `:submit` 使用已保存修订，`/normal-import-prechecks/{id}` 只读所有者检查。写入验证浏览器身份、CSRF、对象授权，POST 使用幂等，更新/删除使用乐观锁；所有者不匹配返回 404。配置变更使旧预检查和风险确认失效；保存响应不得覆盖新会话或保存期间的新输入。

现有 `export_drafts` 表以新增 `draft_kind` 区分平台草稿记录，历史物理表名保留以避免破坏外键与历史迁移。导出查询必须限定 `OBDUMPER_EXPORT`，普通导入查询限定 `OBLOADER_NORMAL_IMPORT`；两者不共享领域配置、资格或参数。现有预检查、任务、租约、日志和审计只复用平台生命周期。禁止以伪造导出配置承载导入。

## 文件与预检查

2026-10-20 真实失败整改：完整映射检查与现有 SHA-256 读取共享全文件流，检测 UTF-8 未压缩 CSV 中固定工具会移除的未引用首尾空白，不限于 256 KiB 首记录前缀。未显式设置 `withTrim=true` 时返回 `INPUT_MAPPING / FAILED / IMPORT_CSV_WHITESPACE_UNQUOTED`，不创建任务；提示将需要保留空白的字段正确加包围符。显式去空白仍按官方工具处理，平台不重写文件或替换导入引擎。跨块引号、双引号转义、换行及 EOF 均参与检查；不确定语法保持 UNKNOWN，文件变化仍优先报告输入失效。该修复是阻止静默损失，不表示官方工具能无损导入原先未引用的空白值。

控制面只签发绑定修订、指纹、凭据修订、节点事实与短租约的固定导入检查上下文；Agent 在允许根目录内检查本地输入，不提供任意文件浏览。符号链接、路径越界、空匹配、扫描超限、文件变化或无法判定时失败关闭。文件清单只返回受限相对路径、数量、字节数与稳定摘要，不返回文件内容。

文件用途 `NORMAL_IMPORT_FILES` 严格只有 `INPUT_FILES`；完整用途 `NORMAL_IMPORT_PREFLIGHT` 按顺序包含 `DATABASE_CONNECTIVITY / WRITE_ACCESS / TOOL_ENVIRONMENT / NODE_RESOURCE / INPUT_FILES / INPUT_MAPPING`。一次最多 1024 个匹配文件、8192 个扫描条目、16 层目录；超限拒绝而不截断。`modified` 纳秒时间用十进制字符串保留精度。扫描使用本机 `os.Root`、链接与身份复核、流式 SHA-256 和剩余租约截止时间；支持当前文件后缀及受控 Go 正则子集，不把无法解释的 Java 正则当成通过。

导入检查与导出检查独立解释输入/输出语义；目标连接、对象/列、写权限、工具、资源与风险条件不能由连接测试或路径语法代替。未提供真实 Probe 时报告 UNKNOWN 并阻断提交，合成依赖只能用于隔离测试。检查清单、文件证据和冻结绑定必须完整一致；迟到、重放、失效租约或事实漂移不能提升状态。

2026-10-16 检查修正：普通导入工具布局统一调用平台 `ToolLauncherPath`，Windows 固定为 `<toolHome>/bin/windows/obloader.bat`，Linux 固定为 `<toolHome>/bin/obloader`；不能要求错误的 Windows `bin/obloader.bat`，也不能以 OBDUMPER 替代。工具环境项仅核对本机文件。工具不通过时，数据库、写权限与映射三项均以 `PRECHECK_BLOCKED_BY_TOOL / UNKNOWN` 表示尚未执行；数据库不通过时后两项为 `PRECHECK_BLOCKED_BY_DATABASE / UNKNOWN`。专用 Probe 未启用时使用 `NORMAL_IMPORT_PROBE_NOT_CONFIGURED / UNKNOWN`，该码不能用于 PASSED。浏览器分别显示失败、前置阻断、未启用及安全原因。

包含数据时，`NODE_RESOURCE` 还须检查固定 OBLOADER 4.3.5 默认数据节流器的最低堆：300 MiB 不可用区 + 200 MiB 弹性区 + 200 MiB 保底区，共 700 MiB。依据本机受控 4.3.5 JAR 的 `MemoryThrottle` 固定实现；这是配置下限，不保证 JVM 实际可用堆或运行成功。Agent 使用控制面已冻结的 `heapBytes`，低于下限返回 `IMPORT_JVM_HEAP_TOO_SMALL / FAILED`；控制面在接收结果与新提交时独立复核，旧 Agent 的 PASSED 不能绕过。该码只用于对应的 `NODE_RESOURCE / FAILED`，不适用于导出或仅结构导入；资源事实缺失仍 UNKNOWN，节点额度不足仍为 `NODE_RESOURCE_INSUFFICIENT`，两者不互相替代。浏览器在原检查行说明最低堆并指向执行参数，不启动任务或静默更改内存。

### 固定只读数据库、写权限与映射检查

普通导入使用独立 `agentjdbc.NormalImportProbe` 和固定 Java 8 主类 `com.obdataorch.jdbcprobe.NormalImportProbe`（v11 帧，替代 v10），共享的只有受限 JDBC 运行基础。v11 在逐表事实后增加类型排除枚举及非表对象 Kind/Name/Reference，不携带定义 SQL；Go 与内嵌 JAR 配套更新，v10 不静默解释为新帧。Agent 本机 `OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true`、已验证固定 JDBC 运行时与完整普通导入租约同时满足时才装配；不开启该开关时保持 UNKNOWN，不能借此打开普通导入执行。文件发现用途不解析数据库槽位。

完整检查在文件清单建立后，复核摘要并提取每个数据分片首条记录的列数/可选列名，随后只解析一次本租约业务数据库槽位。在私有临时工作区运行固定探针；连接、权限、映射只缓存同一冻结绑定的安全结果，秘密和工作区在成功、失败与取消路径清除。协议只携带标识符、权限位和结构，不接受文件 SQL、路径、任意 URL、主类或驱动。

- MySQL 连接切入目标库；固定 `SHOW GRANTS` 核对当前账号的直接授权。按实际输入逐表计算所需权限：含数据的表检查 INSERT，替换数据增加 UPDATE/DELETE，删除清理增加 DELETE，TRUNCATE 增加 DROP；已完整识别的普通建表检查 CREATE，定义文件有同名单表 DROP 前缀或启用替换对象时保守增加 DROP。仅有 DDL 的表也进入核算，数据表与其他定义表不互相要求对方权限。已覆盖本次操作的全局、库级、表级细粒度授权均可通过，不再要求 `ALL PRIVILEGES` 字样。角色、列级授权或不能可靠解释的库名通配范围在不足以确认时保持 UNKNOWN，不误判无权；直接授权已充分时，可忽略额外无法展开的正向 GRANT，但 REVOKE 等其他未识别语句始终保持 UNKNOWN。
- Oracle 通过固定 Schema 元信息和当前用户核对所有权；所属 Schema 的既有表操作依据对象所有权，不要求读取系统授权视图。需要 CREATE 或跨 Schema 操作时，只读取 `USER_SYS_PRIVS` 中明确的当前用户直接系统授权；不依赖 OceanBase 环境未实现的 `SESSION_PRIVS`。非表声明的所属 Schema 检查还可在新建、未执行 SET ROLE 的连接中，根据 USER_ROLE_PRIVS 的 DEFAULT_ROLE=YES 与 ROLE_SYS_PRIVS 读取登录默认启用角色的直接系统权限；角色事实须完整读取后合并，不展开非默认、嵌套角色，也不据此增加跨 Schema 能力。跨 Schema 对象授权、角色或元信息不可判定时保持 UNKNOWN。两种模式均不试写、不执行文件中的 DML/DDL。所属既有表检查于 2026-10-20 剩余场景补验修复；本轮非表声明使用 v11 的结构化事实，固定原因码不变；完整结果见[剩余场景证据](evidence/normal-import-remaining-validation-2026-10-20.md)。
- 映射通过固定元信息读取目标列、生成列、空值/默认值与自增事实，比对首条记录列数、SQL/CSV 自动映射列名及普通显式名单，并检查无默认值必填列。MySQL 固定参数查询不调用 `getSearchStringEscape()`，避免非必要驱动接口导致元信息检查整体 UNKNOWN；Oracle 仍须对元信息模式转义并精确核对 Schema/表名。目标结构不可用或替换同名定义时，可使用同一冻结输入中完整识别的 CREATE TABLE 列结构。支持注释、受限常用会话设置及可选的同名单表 `DROP TABLE [IF EXISTS]` 前缀；其他库/表、多个 DROP、CREATE 后额外操作、可执行注释、超出有界前缀或不能解释的定义保持未知。外部对象引用、未知函数或复杂 DDL 不按普通 CREATE 权限放行。通过仅证明这些结构条件，不证明整文件数据、类型转换、全部 DDL 语句或最终写入成功。
- 当前支持 UTF-8 CSV、显式分隔符 CUT、可识别 INSERT 的 SQL 结构。gzip/zlib 分别只发现官方 `.gz`/`.zz` 后缀，使用标准库解码，在压缩原字节全量摘要及解压完整性核对后提取至多 256KiB 记录前缀；解码累计上限 32MiB，取消、校验和错误、截断和尾随内容均不放行。zstd/snappy、其他编码和超出能力子集的记录不作猜测，报告 `IMPORT_INPUT_MAPPING_UNSUPPORTED / UNKNOWN`。文件变化/结构读取失败为 `IMPORT_INPUT_MAPPING_UNAVAILABLE / UNKNOWN`；确定的列数、列名或必填列不符为 `IMPORT_INPUT_MAPPING_MISMATCH / FAILED`。写授权确定不足为 `IMPORT_WRITE_PERMISSION_REJECTED / FAILED`，证据不足为 `IMPORT_WRITE_PERMISSION_UNAVAILABLE / UNKNOWN`。

2026-10-20 十九项补齐实现边界（真实结果另见本轮证据）：

- CONTROL 仅允许受控根目录中的 `.ctrl`，单文件不超过 32KiB，`lang=java (...) ;` 只接受普通列、NONE 与逐列显式、完整无重复 map(n) 排列。控制目录摘要作为 `controlDigest` 参与清单 digest，执行前重新扫描，修改控制文件使旧绑定失效；函数、常量生成列、隐式源位置、重复列和缺失源位置保持 UNKNOWN。实测省略 map 的列会出现错值，不能按声明顺序推断。
- 类型排除仅接受固定枚举，通过只读目标列元信息过滤候选映射，必填列约束保留；CUT skip-footer 按实际行分隔符忽略最后一条完整记录，只有尾行或没有完整数据记录不算通过。
- 非表定义只接受单个完整声明及受限无外部副作用过程体：纯常量视图、同列赋值触发器、正向序列、同 Schema 同义词、NUMBER 属性类型、只返回整数的类型体、空过程包及常量函数/过程。权限按具体 Kind 和局部依赖只读核算；替换非表对象、跨库依赖、附加 GRANT/DROP、动态 SQL、其他未识别定义保持 UNKNOWN。复合体须有独立行 `DELIMITER /` 或 `DELIMITER $$`、结束标记及 `DELIMITER ;` 的完整包装，防止工具按内部分号拆开；仅原始定义文件交给官方工具，JDBC 不接收 SQL。
- 全局账号/角色须 SPECIFIC 精确对象名，沿用冻结配置风险确认。账号声明只识别 `CREATE USER name@'0.0.0.0';` 的无权限、不可路由来源休眠夹具，不支持包含秘密的定义、通配登录来源或授予权限；角色只识别无授权 CREATE ROLE。不能据此宣称任意用户/角色迁移可用。

2026-10-16 原因细化替代上述初版笼统结果码，旧报告仍可读取，新检查使用以下固定分类；所有拒绝/未知码均不能用于 PASSED，也不携带授权原文、对象数据或 JDBC 异常：

| 检查 | 新证据码 | 状态与含义 |
| --- | --- | --- |
| 写权限 | `IMPORT_WRITE_{INSERT,UPDATE,DELETE,CREATE,ALTER,DROP}_REQUIRED` | FAILED：完整可解释授权未覆盖的首项基本权限；不是申请完整授权 |
| 写权限 | `IMPORT_WRITE_GRANTS_UNAVAILABLE` / `IMPORT_WRITE_GRANTS_UNSUPPORTED` | UNKNOWN：授权读取失败/空结果，或角色、列级、通配授权无法充分展开 |
| 写权限 | `IMPORT_WRITE_DDL_UNSUPPORTED` / `IMPORT_WRITE_INPUT_UNAVAILABLE` / `IMPORT_WRITE_SCHEMA_UNAVAILABLE` | UNKNOWN：DDL 权限未能完整核算、输入结构事实不可用或 Oracle Schema 权限不可确认 |
| 写权限 | `IMPORT_WRITE_CLEANUP_SCOPE_UNAVAILABLE` | UNKNOWN：清理范围是全库、通配或包含无输入证据的表；不能以本地匹配文件替代全部目标表权限事实。精确指定且输入证据完整的表才沿用逐表检查 |
| 映射 | `IMPORT_TARGET_METADATA_UNAVAILABLE` / `IMPORT_TARGET_STRUCTURE_MISSING` | UNKNOWN：元信息读取失败/不完整，或没有可用目标列及同名建表结构；不据空元信息断言对象不存在 |
| 映射 | `IMPORT_INPUT_DEFINITION_UNSUPPORTED` / `IMPORT_AUTO_COLUMNS_UNSUPPORTED` / `IMPORT_INPUT_FILES_CHANGED` | UNKNOWN：定义未完整识别、自动列映射存在未支持组合，或输入变化/摘要无法复核 |
| 映射 | `IMPORT_COLUMN_COUNT_MISMATCH` / `IMPORT_COLUMN_NAME_MISMATCH` / `IMPORT_REQUIRED_COLUMN_MISSING` | FAILED：首条列数不符、列名不存在或遗漏无默认值必填列 |

权限语义核对依据：[OBLOADER 4.3.5 选项](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997308)与[OceanBase 表权限说明](https://en.oceanbase.com/docs/enterprise-oceanbase-database-en-10000000000829381)。本轮没有运行官方工具验证 DROP 前缀在各种参数组合中的执行策略，权限检查保守考虑其潜在 DROP 需求，不更改 `replace-object` 或风险确认，也不据此开放执行。

### 第五步自动准备与可选名称

进入第五步且活动配置、引用事实和草稿恢复就绪后，自动保存必要修订，依次获取唯一生成器的完整脱敏命令并发起完整预检查。页面没有“生成命令预览”按钮，保留复制与“重新预检查”。每次配置版本只自动尝试一次，失败、过期与已有检查不循环请求；配置变化撤销旧结果，A → B → A、离开步骤和卸载不能用迟到命令触发旧检查。

名称不是配置完整度或检查的必填项。提交按钮在资格满足时先打开确认弹窗，接收可选 `taskName`（去首尾空白，最多 200 个 Unicode 字符，不含控制字符）；空值由服务端生成 `普通导入-{目标库前64字符}-{格式或DDL}-{UTC YYYYMMDDTHHmmssZ}-{任务ID前8字符}`。名称进入冻结快照顶层 `taskName`，不改写已检查的 `config` 或命令指纹；幂等摘要包含规范化后的用户名称，同一提交重放返回原任务与原名称。旧草稿名称可预填，旧快照展示回退到 `config.taskName`。

## 提交、执行与恢复

2026-10-20 真实失败整改：显式 `execution.strict` 使用固定工具支持的单令牌 `--strict=true` / `--strict=false`，由唯一生成器同时产生执行与展示形态；Agent 协议与启动前冻结复验仅接受此规范写法，拒绝旧的双令牌写法、非法值和重复参数。控制面与 Agent 须同步升级；历史快照不改写，派生任务仍重新生成与预检查。重跑草稿尝试改参时，仓储继续拒绝并保持原修订；HTTP 返回不可重试的 `422 / NORMAL_IMPORT_CONFIG_LOCKED`，不再误报 503。源码/合成验证和真实复验状态另按任务地图记录，不据修复声明全参数真实通过。

**当前实现状态（2026-10-16 执行切片）**：用户明确授权启用普通导入，并指定非生产目标进行小型结构和数据验证。草稿、预检查、冻结提交与固定 OBLOADER 执行已接通；默认关闭的独立开关只有显式配置才开启，不能继承导出资格。真实安装及取证以[本轮执行证据](evidence/normal-import-execution-2026-10-16.md)为准。`load.ckpt` 资格与继续仍未实现。普通导入使用自身配置的派生和模板链已于 2026-10-19 接入，见末节；不展示导出的恢复操作。

控制面持久配置为 `normalImportExecutionEnabled`，同时要求 `realExecutionEnabled=true`。Agent 安装配置同名；非安装入口使用 `OB_DATA_ORCH_ENABLE_NORMAL_IMPORT_EXECUTION=true`，同时要求 `OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true`。开发入口 `scripts/dev.ps1 -RealExecution -NormalImport`（Agent 加 `-Agent`）显式持久化并保留后续启动；关闭任一侧不能继续领取普通导入。当前执行装配仅 Windows AMD64，不扩大正式平台验证结论。

`/agent/v1/executions:claim-next` 新增显式 `OBLOADER_NORMAL_IMPORT_CLAIM_NEXT / capability=OBLOADER_NORMAL_IMPORT`；省略能力的历史调用仍只领取导出。响应包含 `taskType` 与 `normalImport:{config,manifest}`，固定工具/元数据/能力版本必须匹配。配置、完整输入清单与可选官方 `.MANIFEST.bin` / `.MANIFEST` 辅助摘要均进入租约摘要；辅助清单只做有界摘要，不反序列化 Kryo。确认、续租、槽位、取消、日志和事件复用平台生命周期，保留其历史协议标签，不据标签推断业务类型。旧的按任务 ID 合成领取入口仍只限导出；旧 Agent 拒绝新导入信封。

Agent 在解析秘密前及启动前重新扫描，输入变化、越界、链接、过期或参数与冻结配置不符均拒绝启动；普通导入路径原样传入，不套用导出结果目录语义。唯一生成器派生清理所需 `--yes`，实际提交仍要求当前指纹的风险确认，Agent 不自行追加业务参数。普通导入不接受 sys 或存储凭据槽位。

直接 Java 启动固定为 `com.oceanbase.tools.loaddump.cmd.Obloader`，独立复刻本机 4.3.5 `bin/windows/obloader.bat` 的属性；共享的仅 JVM、安全材料、进程与平台基础，不混入导出属性或旁路能力。密码只通过 execution 私有加密配置，原始工具日志仍为短时私有材料。

管道来源为 `OBLOADER_STDOUT / OBLOADER_STDERR`、解析器 `obloader-4.3.5-v1`，物理 SQLite 来源仍为 STDOUT/STDERR。成功须受控进程退出 0、所选 schema/record 阶段完整结束、固定 `System exit 0`、无已知失败/截断/缺口且日志上传完成。缺失或矛盾证据失败关闭。结果事件使用 `{result,normalImport:true,checkpointPresent:false}` 与可选固定失败码，禁止附加导出文件计数；详情展示执行终态及日志，不把输入数或缺省 0 投影为导入行数。取消仅停止进程，不回滚已写入数据；Agent 重启不重复启动未知 execution，沿用租约核对。

提交要求当前完整预检查、当前权限/凭据/节点事实、冻结指纹与等价风险确认，原子保存不可变 `OBLOADER_NORMAL_IMPORT` 快照。Agent 只接受固定 Obloader 主类和该能力允许的非秘密参数；复用既有受认证领取、租约、一次启动、取消、双层脱敏和可靠日志队列，不开放任意命令。

冻结快照提供任务列表、概览和五组只读白名单投影，类型与版本不能伪装成导出；输入文件数量始终标注预检查事实。路径仅创建者可见，不透传原始配置、文件清单或凭据。任务中心支持普通导入创建、筛选、取消及按自身快照资格开放的派生与模板，不取得导出模板或 `dump.ckpt` 资格。

导入不得用输入文件的数量/字节数冒充导入结果。成功至少需要受控进程终态、固定 OBLOADER 成功证据与完整上传；冲突、未知或证据不完整进入失败/待核对，不声称数据库内容已逐行验证。失败继续只使用原冻结配置、同版工具和真实有效 `load.ckpt` 事实创建新任务，不复制导出的 `dump.ckpt` 规则。无法证明继续资格时保持阻断。

## 验收

G2 验证使用合成身份、假 Agent、临时 SQLite、隔离端口和人工文件夹具，覆盖跨类型隔离、未知参数、字段条件、确定性生成、无秘密、授权/CSRF/幂等/乐观锁、旧预检查与 A→B→A、路径/链接/文件证据、租约/迟到与卸载。后续执行切片已补齐日志、取消与独立结果负例；真实运行及本次不再重试的范围见[执行证据](evidence/normal-import-execution-2026-10-16.md)。恢复能力及尚未完成的原生平台矩阵仍阻断，G2 或单次真实成功不提升全量 G3/G4。

2026-10-16 自动准备/可选命名阶段的历史验证已复核 Go 全量测试/vet、Java 8 七组合成探针夹具、前端 457 项单测、17 项普通导入浏览器场景、构建/lint 与架构审计；该阶段没有安装或重启运行服务，也没有真实导入。后续运行安装、既有真实结果及中断收尾分别记录在执行证据；详细历史结果、样式/Token 既有失败和临时证据位置见[维护与验证记录](../../web/src/workbench/import/normal/README.md#验证入口)。

## 2026-10-19 草稿管理与历史配置复用

- 当前普通导入草稿进入统一“我的草稿”，按当前主体所有权与未提交状态查询；计数、关键字及游标在服务端同一范围完成。恢复和删除按真实类型进入 normal-import-drafts，不借用 export-drafts，也不在浏览器拼接分页。
- 可信成功任务可“基于原配置新建”和“保存为模板”；可信失败/取消任务还可“从头重新执行”。核对中、计划/实际摘要不符、未知类型或版本均拒绝。列表与详情只消费服务端资格，写接口与仓储事务再复验。
- 派生只提取 normal-import-v1、固定工具与元数据版本的独立非敏感配置，清除任务名称，不复制输入清单、凭据、旧检查或确认。source_task_id/source_derivation 原子写入既有草稿列并审计；新提交从草稿冻结 parent_task_id/derivation_kind，提交事务核对精确来源关系。
- 从头重跑进入第五步重新检查，参数不可修改；前端导航与保存方法均保护，仓储拒绝任何配置差异。引用不再可用时保持阻断；需要改参时从来源任务选择“基于原配置新建”。提交名称仍是独立元信息，不改变检查配置。
- 模板只从可信成功快照创建。默认清除数据源、节点、任务名称，保留显式路径与活动参数；模板本身无执行资格。0032 增加不可变 task_type，旧记录为导出，不支持旁路类型。
- 模板使用先选定对象，再按其类型读取源/节点候选；两份引用原子发布。普通导入复用现有独立资格规则和授权节点列表，不能借用导出候选。绑定变化、版本变化、授权失效与 A → B → A 撤销旧响应与导航；加载、无候选和未填写不先触发创建错误。
- 服务端按当前源模式及节点平台校验模板配置，不得静默丢弃不适用对象、转换路径或跨类型解释。使用生成新草稿，所有预检查、风险与真实执行开关重新生效。旧导出模板 URL 对普通导入返回不存在语义。
- 本轮没有 load.ckpt 采集、--retry、新参数或真实工具能力变更；页签、复制、日志和取消继续复用原会话所有者。检查与运行状态见[收口证据](evidence/task-import-reuse-2026-10-19.md)。
