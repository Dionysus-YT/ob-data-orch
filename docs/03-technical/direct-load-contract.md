# 旁路导入技术契约

> 2026-10-19：独立旁路开发实现。六步页面、草稿与任务派生、确定性命令、受认证 Agent 文件发现、专用固定只读探针、执行信封及 Worker 已接入。探针与写入分别默认关闭，不继承普通导入或导出准入。本轮仅源码、静态工具核验与合成验证；真实 DLV/G3/G4 门禁不提升，见[开发证据](evidence/direct-load-development-2026-10-19.md)。

> 2026-10-20 用户授权真实全量测试后的阶段记录：本机固定 JDBC 组件已对自有 TEST 夹具核对两次，尚未完成认证业务 API、任务提交、官方工具写入与目标数据闭环，因此整体仍未通过真实 E2E。ODP 查询缺口已在源码补齐，未替换运行二进制；端点、版本、文档冲突和逐项未执行范围见[本轮测试记录](evidence/direct-load-e2e-2026-10-20.md)。

## 2026-10-22 提交与诊断修复

同一完整旁路预检查在串行写事务内只允许冻结一个任务，非幂等内部入口也不能绕过；执行复验须携带原 taskId，不能把原任务误判为重复。浏览器为一次确认保留原任务名、证明与幂等键，结果不确定时先核对已提交任务，禁止换键或新预检冒充重试；已接受但导航失败不再次提交。GET 预检查只对其所有者返回可选 `submittedTaskId`，原任务名与原幂等键仅在服务端内部比较。原键与原请求可找回过期证明对应的已提交任务，仍须当前浏览器认证、对象写权限、管理员与独立开关；换键或异内容为 409 `DIRECT_LOAD_SUBMISSION_CONFLICT`。核对结果不是新的执行资格，不回填历史任务或修改已提交快照。

第四步允许设置 replace-data 意图，第五步按当前结构与配置重新检查；选择本身不代表结构通过。结构失败细化为不存在表、BIT、二进制、虚拟列及增量替换索引/LOB 的固定码；读取事实不足保持 UNKNOWN。新码仅限 `DIRECT_LOAD_STRUCTURE` 及对应 FAILED/UNKNOWN 状态，Agent、控制面、JSON 契约与浏览器均复核，保留历史通用码读取兼容；不投影数据库错误、SQL、表/列名或秘密。源码、现役安装与本轮实证分别记录在[三项修复验收](evidence/direct-load-defect-fixes-2026-10-22.md)。

## 领域与参数

`internal/directload` 是旁路配置、条件清值、精确版本矩阵、表结构判定及命令字段适配的唯一业务所有者。任务类型 `OBLOADER_DIRECT_LOAD`、配置 `direct-load-v1`、元数据 `obloader-4.3.5-direct-v1`、能力 `direct-load-local-v1`、工具 `4.3.5-RELEASE` 相互绑定，不接受其他业务类型或历史普通导入配置。

配置只包含已确认的本地单表数据路径、场景、RPC 端口、格式、活动参数和四项入口阅读确认。2026-10-20 按 DEC-056/057，RPC 按本地场景预填可编辑默认值：私有 ODP 2885、OBServer 2882；切换场景只更新空值或原默认端口，自定义值保留，恢复草稿不改写保存值。来源继续为用户提交并确认的填写配置 `MANUAL`，包含默认预填后确认的值，不代表来自探针或已连通，不从 SQL 端口推测。不接收密码、SQL、自由命令、版本结论、表结构事实或客户端声称的预检查结果。路径以匹配平台绝对路径原样传递，不添加目录或跨系统转换。

官方允许 ODP 与 OBServer 两类目标。固定 ODP 是大批量导入建议；F5/VIP 不据名称硬禁用，现有单主机模型须同一已登记地址分别提供 SQL 与 RPC 转发，并以真实环境验证为准。页面不新增单任务 SQL/RPC 地址覆盖，不把 SQL 测试或普通 TCP 可达性说成 RPC 协议和旁路能力通过。

本地活动场景仅 `PRIVATE_ODP`、`OBSERVER_DIRECT`，格式仅 CSV；SQL、Parquet、ORC 的本地旁路适用性尚未确认，不配置或生成。连接事实必须与当前已授权数据源一致，不能把 ODP 数据源解释为直连。当前数据源登记入口提供的候选范围不会因旁路页面而扩大；没有匹配候选时显示不可用。格式的官方适用性不等于真实组合已验证。

2026-10-22 用户采纳 DEC-058 后，登记与固定基础连接测试补齐 `OBSERVER_DIRECT`。旁路候选使用同一服务端枚举，不接受旧前端错误别名 `OBSERVER`；选择源时自动带出场景，当前源类型与旧草稿场景不一致时不能沿用旧资格。直连的 SQL 用户名为 `username@tenant`，同时生成 RPC 必需的 `--tenant`，不生成 `--cluster`；唯一生成器、冻结 argv 和 Agent 复验一致。原官方 103 项研究元数据继续冻结，本地投影修正 tenant 说明。

精确版本接受固定 4.3.5 主 JAR / MYSQL 或 ORACLE / OBServer 4.4.2.3 直连组合。Oracle 按用户继续修复指令取得独立官方工具同表提交、受控退出与完整中文/NUMBER目标结果后才加入，未借用MySQL证明；组件不等于API E2E，实际验收见[Oracle 修复记录](evidence/direct-load-oracle-repair-2026-10-22.md)。其他新版本保持 `VERSION_UNVERIFIED`，预检查 UNKNOWN，提交继续阻断。完整 LOG 在真实采集协议中使用空完整性原因码；旁路终态解析同时接受空值与历史 COMPLETE，仍拒绝未知码、缺口、截断、stderr 提交行、错表和无同表提交的单独退出。

`contracts/direct-load.json` 的 `LocalConfig` 限制活动写入、预检查上下文与执行输入；`Config` 为历史草稿和快照保留读取枚举。前端不静默转换历史场景、格式或端口，必须明确重新选择才能继续。领域保存/检查、唯一命令生成器、固定探针 Go 入口和 Agent argv/配置复验均拒绝本地范围外的场景与格式，不能凭旧目录支持状态或旧证明继续执行。

103 个官方长参数保留冻结研究目录和原摘要；页面通过本地适用投影显示 102 项，排除部署外专属项、覆盖本地 RPC/租户说明，并将三种未确认格式标为待验证。投影只收缩展示资格，不能授予能力；服务端独立复验本地范围。未确认共享参数与不适用参数不进入生成结果。唯一命令生成器同时产生确定性非秘密 argv、脱敏显示、字段来源和指纹。`--direct` 固定派生；`--yes` 仅随清空表策略派生，不能作为自由开关。`--retry`、错误阈值、DDL、all、多表、sys 凭据和远程存储均不开放。入口确认不替代最终当前指纹的风险确认。

版本与表事实的纯规则具备合成正反例；未装配固定探针时不以用户输入或源连接测试补足事实。BIT、二进制类型及虚拟生成列仍按当前产品受控范围失败关闭；2026-10-20 已发现 4.3.5 发布说明与旧专题限制冲突，不能把平台未放行表达为官方工具不支持，见本轮记录。表是否为空派生全量/增量，增量且存在索引或 LOB 时拒绝 replace-data。唯一索引冲突只明确警告，不能宣称 replace-data 可以解决。

## 草稿与浏览器接口

独立接口：

- `GET /api/v1/direct-load-parameter-metadata`
- `GET/POST /api/v1/direct-load-drafts`
- `GET/PATCH/DELETE /api/v1/direct-load-drafts/{id}`
- `POST /api/v1/direct-load-drafts/{id}:preview-command`
- `POST /api/v1/direct-load-drafts/{id}:discover-files`
- `POST /api/v1/direct-load-drafts/{id}:precheck`
- `POST /api/v1/direct-load-drafts/{id}:submit`
- `GET /api/v1/direct-load-prechecks/{id}`

浏览器身份、对象授权、无权 404、CSRF、创建幂等、修订与当前引用复验沿用平台边界。源选择后可保存不完整草稿；已填写的值仍需合法，保存不代表可执行。每次配置修订撤销旧预检查，旧响应不能恢复旧会话或覆盖新配置。

迁移 `0034` 只扩展本轮实际使用的草稿、预检查和冻结任务类型，前向重建封闭 CHECK 枚举并保留原数据、索引、触发器和外键。历史迁移不改写，不自动修改运行中的 SQLite。统一草稿列表在同一 SQL 所有权范围完成分页与计数，恢复和删除进入独立旁路接口。

任务名称是提交确认会话的必填字段，不进入活动配置指纹。HTTP 提交要求真实能力总开关、独立旁路探针及独立执行开关，同时复验当前主体、写权限、对象引用、完整输入清单、配置指纹、修订、当前九项通过证明、配置安全投影指纹和当前指纹的最终风险确认。缺任一开关返回 `DIRECT_LOAD_EXECUTION_GATED`，导出或普通导入开关不能解锁旁路。

任务配置复用沿用现役 `REBUILD_FROM_CONFIG`；从头执行使用 `RERUN_FROM_SCRATCH`，参数只读，允许相同活动配置重新保存以绑定当前引用与新检查。两者均创建新草稿、新检查、新任务及新的风险确认，不继承秘密或执行结果。不提供旁路模板、自动重试或检查点继续；导出的检查点派生入口也必须核对源任务类型。

## Agent 与输入证据

Agent 使用既有受认证主动出站预检查租约；冻结上下文增加独立 `directLoad`，与 `normalImport`、导出对象/输出/存储字段互斥。确认、完成、重放、过期、绑定和租约 epoch 均由平台复验。

`DIRECT_LOAD_FILES` 只执行 `INPUT_FILES`，不解析数据库秘密或执行数据库/RPC 检查。完整 `DIRECT_LOAD_PREFLIGHT` 固定顺序为：

1. `TOOL_ENVIRONMENT`
2. `NODE_RESOURCE`
3. `INPUT_FILES`
4. `DATABASE_CONNECTIVITY`
5. `RPC_CONNECTIVITY`
6. `DIRECT_LOAD_VERSION`
7. `WRITE_ACCESS`
8. `DIRECT_LOAD_STRUCTURE`
9. `DIRECT_LOAD_CONFIG`

结果只接受旁路专用状态/原因码；普通导入的 `IMPORT_VERIFIED`、导出成功码和缺项报告不能用于旁路。工具未就绪时数据库依赖项保留 UNKNOWN；连接未通过时权限/结构依赖项保持 UNKNOWN。独立的本机资源和文件项继续收集，不把“尚未执行”说成目标异常。

`internal/importfiles.ScanMatched` 仅共享允许根目录、链接拒绝、有界读取、取消和稳定摘要这些文件基础。旁路分类与集合复验由 `internal/directloadfiles` 所有，不构造普通导入配置，也不沿用普通导入对象过滤。目录内其他表的同格式数据文件保留在集合并阻断，不先过滤掉再宣称单表；零匹配、不可确定文件名、重复/乱序/篡改清单、总量不符与越界均失败关闭。文件数与字节数始终是输入事实，不能展示为导入结果或目标行数。

默认本机 Probe 只执行布局、资源和文件检查。独立许可后才装配 `agentjdbc.DirectLoadProbe`，以一次当前数据库槽位调用固定 Java 8 旁路入口；只加载已核验的 `oceanbase-client-2.4.14.jar`。它执行固定连接、最小权限与系统元信息读取，不接收 SQL、URL、驱动或主类。RPC 项只对当前业务 Host 和人工 RPCPort 建立有界 TCP 连接，不证明 RPC 协议、身份认证或导入成功。缓存绑定完整租约、配置、输入清单和当前引用，跨绑定、进行中或迟到结果保持未知。

OBServer 版本固定查询 `SELECT OB_VERSION() FROM DUAL`。私有 ODP 场景按官方 [SHOW 定义](https://www.oceanbase.com/docs/common-odp-doc-cn-1000000003575638) 仅在当前业务连接执行固定只读 `SHOW PROXYINFO BINARY`，严格限定单行 `binary info`、4096 字符以内的官方前缀和三/四段数字版本，只返回版本、不返回原始构建信息。查询不被当前连接允许、零/多行、未知格式或异常时保持 UNKNOWN；不请求 proxysys/sys 凭据、不使用 OBServer 版本代填、不以 TCP 连通推定版本。该读取协议仍为 v11，未新增输入字段或秘密槽位。MySQL 固定授权/结构读取沿用既有规则；Oracle 只对 `SYS_CONTEXT('USERENV', 'CURRENT_USER')` 与冻结Schema精确相同的既有表检查对象所有权，以及官方要求的四个 `SYS` 视图零行SELECT权限。跨Schema、未知身份和角色/ANY权限不展开，保持UNKNOWN。

Oracle结构由绑定OWNER/TABLE_NAME的固定 `ALL_TABLES`、`ALL_TAB_COLS`、`ALL_INDEXES` 读取，表空性采用已验证标识符双引号限定和 `ROWNUM <= 1`。Oracle第四个权限视图为 `SYS.all_virtual_proxy_schema`，不套用MySQL双下划线名称。列类型、虚拟列、隐藏列、索引唯一性/有效性或预算未知时保留空事实；RAW/LONG RAW/BLOB按平台二进制边界阻断，CLOB/NCLOB保留LOB事实，替换规则不因truncate意图覆盖真实非空状态。固定入口不接收任意SQL、角色或对象定义；未能证实某项不代表目标异常。

节点已有 `session.config.json` 诊断只允许三个数值的安全投影指纹、固定来源和观察时间；不公开整文件哈希、未知键名或原文件。当前任务的 `DIRECT_LOAD_CONFIG=PASSED` 必须携带本机新鲜投影 `directLoadConfigFingerprint`，其他状态及 FILES 不得携带。指纹随检查、不可变快照、执行输入和信封摘要冻结，解析秘密前及启动前分别重读比对，配置变化不能沿用旧证明。

## 独立执行与结果

部署 `directLoadProbeEnabled` 和 `directLoadExecutionEnabled` 缺省为 false；执行同时要求真实总开关及旁路探针开启。Agent 安装配置投影为 `OB_DATA_ORCH_ENABLE_DIRECT_LOAD_PROBE`、`OB_DATA_ORCH_ENABLE_DIRECT_LOAD_EXECUTION`，旧包不继承环境中的旁路许可。控制面停止许可后，新领取及秘密解析继续失败关闭。开启开关本身不能代替 DLV 准入与真实操作授权。

执行只接受显式 `OBLOADER_DIRECT_LOAD` 能力，普通导出/导入 Worker 不能代领。信封包含单表配置、输入清单、源兼容模式和配置安全投影指纹，复验时拒绝混合 normalImport、存储/sys 材料、额外 argv、旧工具和不匹配场景。固定主类仍为官方 Obloader；旁路类路径以已核验主 JAR 优先并移除当前目录，防止同名 class 覆盖静态核验实现。输入复核不创建结果目录或检查点。

成功解析器 `obloader-direct-4.3.5-v1` 仅适用于 SHA-256 为 `b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06` 的固定主 JAR。只读静态字节码确认：`DirectPathConnection.commit()` 在异步提交 await 且 isSuccess 成功之后才输出同表 `Load task on table "..." is committed successfully! Elapsed: ...`。成功要求该固定 stdout 行严格匹配冻结目标表、之后出现固定 `System exit 0`、受控进程实际退出 0、无输出错误、日志完整且上传完成；批次写入、通用 Load record finished、跳过提交或单独退出 0 均不足。静态核验不能替代服务端真实取证。

旁路结果只上报 `result`、`directLoad=true`、`serverCommitObserved`、`checkpointPresent=false`，失败另有固定 `errorCode`。上述提交事实和安全错误码随接受事件投影到持久摘要，不回填旧记录，也不虚构 execution API 未提供的字段。已观察提交后发生退出或证据失败时保留 `serverCommitObserved=true`，整体仍 FAILED 并要求人工核对；未观察提交也不能证明未写入。输入文件数、字节和清单不进入导入结果。旁路暂不开放运行中取消与检查点继续，不承诺停止客户端可回滚服务端提交；阶段百分比没有可靠来源时返回未知，不复用普通导入解析器。

## 五步会话与检查时序

2026-10-20 按 DEC-056 采用与导出、普通导入相同的顶部水平 Steps，旁路五步标题由自身规则唯一提供。前四步百分比仅统计当前有效本地输入，第五步只统计九项当前有效预检查的通过证据，等待、失效和过期清零；目录跳步不补齐前序配置。从头执行锁定第五步，旧最终步 `step=6` 兼容进入第五步。该百分比不是工具运行进度，不修改提交、执行、结果和真实准入契约。

五步分别为数据源与连接、导入目标、文件与格式、执行参数、预检查与提交。入口阅读确认在首步有效继续时集中展示，明确继续才记录四项现有确认字段；取消不确认、不导航、不发预检查，同一已确认草稿往返不重复弹。弹窗捕获当前配置与引用绑定，变化、路由会话切换及卸载使旧弹窗和确认失效。最终提交确认仍由 review 能力持有，不能继承入口确认。状态位于 `workbench/import/direct-load` 的唯一表单及能力所有者；页面入口只负责装配、路由和离开确认。字段、场景、源、节点、路由会话与卸载变化立即隔离旧成功、失败、finally 和导航；A → B → A 不能仅凭 ID 接受旧响应。方法入口防重，卸载清轮询和请求。

目标数据库和唯一表在第二步确定，节点、路径和格式在第三步确定；所需事实齐备后，第五步统一执行固定完整检查。前四步仅允许合法输入导航，连接/版本/文件及结构事实始终显示待检查，不承诺分段真实探针或放宽提交。文件发现要求有效节点、路径、目标表与格式，但不要求尚未产生作用的 RPC/最终确认；完整预检查仍要求所有必要配置与入口确认。此时序已由 DEC-056 收敛为现役规则，真实门禁前仍须按 DLV 验收复核。

## 完成与阻断

当前已取得 Windows AMD64 / 固定 4.3.5 / OBServer 4.4.2.3 直连的 MySQL 与 Oracle 当前账号 Schema 内代表场景实证，分别见[单机修复验收](evidence/direct-load-standalone-repair-2026-10-22.md)和[Oracle 修复验收](evidence/direct-load-oracle-repair-2026-10-22.md)。其他组合的真实准入仍需 [DLV 计划](../02-design/direct-load-controlled-validation.md) 中的非生产 OBServer/ODP 精确版本、独立 SQL/RPC 端点、最小授权、单表输入夹具、表前后事实及服务端可获得状态，且取得明确执行授权。

完整 DLV 尚未全部验收；ODP 精确版本、跨 Schema Oracle 权限、其他格式/类型/场景组合、取消阶段语义及失败后从头执行的真实证据仍有阻断。已实证范围具有工具同表提交与持久结果、完整目标行集/DDL 前后核对，不替代未覆盖的服务端状态或组合。当前数据源登记缺少匹配场景时也不伪造候选。独立探针和 Worker 的源码/合成通过不等于这些能力已验收；不提供 retry/checkpoint、旁路模板或真实取消入口。旁路类型的只读任务快照继续按原六个证据分组白名单投影，分组数不代表当前页面步骤数；路径仅创建者可见。
