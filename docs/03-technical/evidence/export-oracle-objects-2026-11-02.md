# Oracle 导出对象选择补齐（2026-11-02）

本轮按用户提供的十类对象截图，补齐程序包、触发器、同义词、公共同义词、类型。Oracle 包含结构时可选择十类；MySQL 保持原五类，纯数据只允许表。页面、合成链路与受影响检查通过。随后按用户“进行真实导出验证”授权，在非生产 `test-oracle` 完成十类真实目录/访问检查和 7/7 官方工具导出内容核对；本机开发控制面、Agent 和 0043 迁移已更新。真实平台任务链因 Work 本地浏览器访问关闭而未验证，不把固定工具组件证据写成平台 E2E 通过。

## 实现与边界

| 新分类 | 固定参数 | 固定元数据来源 |
| --- | --- | --- |
| PACKAGE | `--package` | `ALL_OBJECTS`，绑定 OWNER，固定 OBJECT_TYPE=PACKAGE |
| TRIGGER | `--trigger` | `ALL_TRIGGERS`，绑定 OWNER |
| SYNONYM | `--synonym` | `ALL_SYNONYMS`，绑定 OWNER |
| PUBLIC_SYNONYM | `--public-synonym` | `ALL_SYNONYMS`，OWNER 为 PUBLIC/__public，绑定当前 TABLE_OWNER |
| TYPE | `--type` | `ALL_OBJECTS`，绑定 OWNER，固定 OBJECT_TYPE=TYPE |

- 公共同义词参数及范围先通过固定 4.3.5 工具 JAR 的 `ObjectTypes` 注解和 `AbstractMetadataProvider` 常量离线核对，后在本节真实验证中运行相同官方工具主类。该 JAR 的 SHA-256 为 `b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06`。[官方命令选项资料](https://docs.oceanbase.com/obloader-obdumper/V4.3.4.1-obdumper-command-line-options)用于对照通用 DDL 语义，精确 4.3.5 新参数以本次固定包检查为据。
- `internal/exportobject` 统一 Go 类型与模式边界。`exportPresentation.objectCategories` 与 `createObjectTypeMap` 统一前端标签、图标和集合初始化。表单、目录、恢复、草稿生命周期继续各有唯一所有者；步骤不新增请求或表单副本。
- Oracle ALL 成功须返回完整十类，MySQL 须五类；单类失败保持不可用，旧 Oracle Agent 缺类时提示更新，不把缺失解释为零对象。目录继续只有一次 JDBC 连接，响应预算、租约、授权及过期防护不变。
- `OracleObjectMetadata` 同时用于目录和精确名称检查；Schema、精确名称均绑定，搜索在受控名称集合上过滤。真实验证后，Oracle FUNCTION/PROCEDURE 也改用按固定 OBJECT_TYPE 过滤的 ALL_OBJECTS 查询，避免驱动精确名称的 LIKE 转义差异和过程目录混入函数；MySQL 例程继续原 JDBC 路径。产品探针只核对元数据，不读取同义词目标或执行包、类型、触发器代码。
- 新不可变 `obdumper-4.3.5-slice-v13-oracle-objects` 继承 v12，共 84 项；新增五个参数只绑定四种包含 DDL 的能力且依赖 `--ddl`。新建或明确保存完整配置时按对象选择 v13；编辑占位草稿不生成命令。历史资源、任务和只读重放不升级；包体、类型体不隐式加入。
- `0043_export_oracle_catalog_types.sql` 只增加受约束列 `catalog_oracle_object_type`，保存新单类目录请求；旧迁移和列编码不变。43 项迁移、43 张 STRICT 表先通过隔离 SQLite 验证；真实验证阶段在一致备份后，由现役开发控制面正常迁移至 43。

## 实际检查

| 检查 | 结果与范围 |
| --- | --- |
| `scripts/build-jdbc-probe.ps1 -JdkHome <Java 8 JDK>` | 通过；重新编译内嵌探针，执行合成 JDBC 测试，覆盖十类大目录、精确绑定、空/重复结果、权限异常及公共同义词范围 |
| Go 受影响包测试 | controlplane、store、agentpreflight、agentjdbc、agentconnectiontest、migrate、commandgen、parammeta、exportdomain、catalogresult、jdbcprobe、agentwire、toolcompatibility 均通过；新增 Agent/帧边界负例后单独复验 agentpreflight、agentwire、jdbcprobe 通过 |
| 迁移负例 | 首轮校验和夹具仍只带 0042，已补入 0043 后复验通过；历史校验和拒绝逻辑保留 |
| `go vet` | exportobject、catalogresult、exportdomain、jdbcprobe、parammeta、commandgen、controlplane、store、agentpreflight、agentconnectiontest、agentwire、toolcompatibility 通过 |
| 前端定向单测 | 导出和 API 首轮 7 文件/208 项通过；追加对象 API/会话负例后的最终复验 2 文件/133 项通过，不相加计数 |
| `npm run typecheck`、`npm run build` | 通过；构建仍有现有大包体积和插件耗时提示 |
| `npm run lint` | 通过 |
| `npm run audit:business`、`npm run audit:wizards` | 0 violations；既有规模和共享入口 REVIEW 仍由原能力所有者承担，本轮没有新增业务/页面反向依赖或状态副本 |
| Playwright 定向交互 | 5 项通过：Oracle 三档、旧 Agent 缺类失败反馈、既有 MySQL 五类回归 |
| `scripts/check-secrets.ps1`、`git diff --check` | 通过 |

控制面测试覆盖十类同名混选的草稿→命令预览→预检查→冻结快照；MySQL、纯数据、旧参数目录、缺少 DDL 与通配范围均拒绝。前端覆盖分类全选、程序包单类搜索、内容切换后恢复非表选择、编辑草稿保存和刷新恢复、同名私有/公共同义词独立移除、键盘 Enter 展开类型。目录迟到与切库负例由 `exportWizardState.test.ts` 复验。

## 页面证据

入口为隔离 `http://127.0.0.1:15174/exports/new`，所有 `/api/v1` 响应使用合成路由，不接触开发控制面。使用 Chrome `156.0.8078.74` 与项目 Playwright；Browser plugin 不可用（Browser plugin not available）。初次启动受本机 SOCKS 代理协议影响，只在测试进程移除代理配置后重试成功，没有修改机器代理。

三档为 1920×1080、1440×1024、1280×720。URL、非空页面、无 Vite 错误覆盖层、无页面/控制台错误、无整页横向溢出和目标交互均通过。实际截图目视检查沿用现有外壳、Ant 控件、双栏 400px 对象框与独立滚动；新增图标与原分类等尺寸，没有新 Token 或样式覆盖。小屏通过页面滚动到达对象区，分类与已选对象继续在各自框内滚动。

交互结果位于系统临时目录 `ob-data-orch-oracle-objects-20261102`；最终补拍与目视检查位于 `ob-data-orch-oracle-objects-visual-20261102`。后者每个宽度子目录均包含：

- `oracle-ten-categories.png`：对应视口实际页面。
- `oracle-object-selector.png`：对象区滚动到新增分类，显示五个新增入口及原位选择状态。

本轮只声明受影响页面和交互通过；200% 原生缩放未验证，未宣称完整 P0 发布视觉矩阵通过。没有新增 Drawer/Modal、危险操作或全局反馈状态。

## 授权真实验证与修复

范围限定为既有 `test-oracle` / TEST / ODP、Windows AMD64 节点和允许根目录中的本轮独占输出，不读取业务表。固定 Java 夹具在当前 Schema 内创建十类对象、两行表数据及独立所有权表；不创建用户、授权或接收 SQL。公共同义词创建前全局核对同名冲突，清理前同时核对所有权、目标 Schema 和本轮表名。密码只在已有辅助进程内短时解密，经匿名 stdin 交给固定 Java 夹具或进入 execution 私有的官方加密材料，未进入 argv、环境或证据。

首次真实目录已返回十类，新增五类访问均为 ACCESSIBLE；混合检查的 FUNCTION/PROCEDURE 为 NOT_ACCESSIBLE，过程目录还包含函数。修复上述固定元数据路径后，同一组自有十类全部可访问，过程目录只返回过程；不存在的五类对象组合为 NOT_ACCESSIBLE。合成测试覆盖精确名称绑定、类型过滤、零行、重复行、权限错误和 MySQL 分支保持原路径，固定 Java 探针构建及受影响四个 Go 包复验通过。

7 个工具场景均通过唯一命令生成器和生产 `StartDirectJava` 启动边界运行固定 OBDUMPER 4.3.5，逐次串行执行。组件使用辅助进程的两分钟截止点，不签发平台租约或伪造平台预检查。准备阶段曾因 Windows 路径形式、DDL 能力不接收资源参数及组件遗漏启动截止点被拒绝，均未启动工具；修正测试辅助程序后再执行，保留初次拒绝回执。没有降低产品校验。

| 场景 | 实际 DDL 对象 | 数据行 | 结果文件/字节 | 内容结论 |
| --- | ---: | ---: | ---: | --- |
| 程序包 | 1 | — | 1 / 109 | PASS |
| 触发器 | 1 | — | 1 / 147 | PASS |
| 私有同义词 | 1 | — | 1 / 79 | PASS |
| 公共同义词 | 1 | — | 1 / 86 | PASS |
| 类型 | 1 | — | 1 / 113 | PASS |
| 十类混合结构 | 10 | — | 10 / 1257 | PASS |
| 十类混合结构 + CSV | 10 | 2 | 11 / 1289 | PASS |

每项均核对受控进程退出、退出码 0、完整可靠日志和固定 `System exit 0` 终态；独立检查器解析实际 CREATE 目标和类型，忽略注释、字符串并支持官方 `DELIMITER $$` 包装。实际目标集合与冻结选择完全相等，私有/公共同义词均指向本轮表。CSV 核对完整表头、两行值、行数和重复性；不是只核对文件名、非空或工具退出码。共 25 份 DDL 和 1 份 CSV，3080 字节；另有零字节 `.MANIFEST.bin`，仅记为元数据，不计结果文件或数据证明。全部结果及本轮工具文件日志经已知密码/可恢复编码核对，未发现秘密。

真实证据保存在忽略目录 `artifacts/export-oracle-objects-real-20261102-f9dab086460a/`：

- `jdbc-before-fix-summary.json`、`jdbc-catalog-and-access.json`：首次失败诊断与修复后真实目录/访问回执。
- `fixture-component-*.json`、`verified-results.json`：受控工具观察、实际文件大小/摘要和内容核对；scope 为 FIXED_TOOL_COMPONENT，platformTasksVerified=false。
- `authorized-residual-cleanup.json`：用户另行明确批准的两个历史孤立执行目录精确清理记录，未清理其他历史对象。
- `fixture-cleanup.json`、`output-cleanup.json`：十类夹具及所有权表全部清除，本轮输出和日志共 55 个文件已在核对、扫描、保存摘要后删除。
- `before-upgrade.sqlite`、`configuration-hashes-before.json`、`runtime-final.json`：升级前一致备份、配置/身份摘要及服务恢复事实，仅本机保留，不进入 Git。

修复后的控制面与 Agent 按停止、等待端口关闭、删除旧二进制、`CGO_ENABLED=0 go build -trimpath`、恢复原参数/开关的流程安装。最终健康请求为 200，SQLite 43，Agent ACTIVE 且心跳新鲜、占用为零，节点 ENABLED / 环境 PASSED；安装 Agent 与控制面提供的 Windows Agent 包摘要相同。身份、TLS、根密钥和原执行开关摘要未变，活动 execution 为零，组件私有目录无残留。原有一条终态 FAILED 的待核对记录保留，未清除标记或外推为本轮失败。

## 版本与未验证范围

基础 HEAD 为 `4e8cb4034b474ac0cfe9ed9c63c8ae285963eeb0`，工作区包含任务开始前的其他未提交改动，本轮保留这些改动。实际实现以当前工作树和本轮新增测试为准，不能只以该 HEAD 重放。CodeGraph 索引停留在 2026-09-16，早于相关实现，因此本轮直接回查源码和契约，没有重建索引。

未提交或推送；仅更新本机开发服务，未发布到其他环境。CUA 明确返回 `Local browser use is disabled in Work settings`，开启本地浏览器访问的用户输入尚未到达；没有绕过该限制、提取会话、伪造登录或直接写 SQLite 创建任务。因此真实草稿→预检查→提交→Agent 领取→任务结果的完整平台链保持 BLOCKED，合成平台链与上述真实组件证据分开声明。未验证其他权限、版本、CUT/SQL 组合、包体/类型体、对象存储或其他操作系统；交叉平台支持和发布状态不变。夹具清理后只复核保存的回执，重跑必须创建新所有权 runId 和独立对象。
