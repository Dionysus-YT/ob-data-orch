# 导出模块 V1.0 支持状态矩阵

> 文档状态：EX-D1 产品设计产出
> 创建日期：2026-08-05
> 依据：[参数映射基线](export-parameter-mapping.md)、[字段与条件矩阵](export-field-rules.md)、[导出模块设计](export-module.md)
> 安全说明：不记录真实端点、身份、密码、密钥、完整命令或工具原始输出

## 1. V1.0 支持状态定义

| 状态 | 含义 | 命令生成 | 页面展示 |
|---|---|---|---|
| ENABLED | 官方含义已核验、产品处置已确认、无冲突、可安全实现 | 是（活动且值合法时） | 按层级显示 |
| VALIDATION_GATED | 官方含义已核验但受控实测未完成或约束条件未锁定 | 否（阻断） | 显示但标记"待验证" |
| HIDDEN | 导入侧/废弃/help-only/未公开参数 | 否 | 不显示、不提供新建入口 |
| BLOCKED | 官方口径严重冲突且无法形成安全验收方案 | 否 | 不显示 |

## 2. 109 参数 V1.0 支持状态

### 2.1 导入侧/未公开（5 个）— 全部 HIDDEN

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--file-suffix` | NORMAL | LOW | HELP_ONLY | HIDDEN | OBLOADER 文件匹配参数 |
| `--ignore-escape` | NORMAL | LOW | HELP_ONLY | HIDDEN | OBLOADER CUT 转义选项 |
| `--mix` | NORMAL | LOW | HELP_ONLY | HIDDEN | OBLOADER 混合格式 |
| `--parallel` | NORMAL | LOW | HELP_ONLY | HIDDEN | OBServer 旁路导入并行度 |
| `--commit-size` | NORMAL | LOW | HELP_ONLY | HIDDEN | 仅本地 Usage 可见 |

### 2.2 废弃兼容（3 个）— 全部 HIDDEN

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--storage-uri` | NORMAL | LOW | HELP_ONLY | HIDDEN | 4.3.0 起废弃 |
| `--file-name` | NORMAL | LOW | HELP_ONLY | HIDDEN | 4.3.0 起废弃 |
| `--upload-behavior` | NORMAL | LOW | HELP_ONLY | HIDDEN | 4.3.0 起废弃 |

### 2.3 CLI 元参数（2 个）— 全部 HIDDEN

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--help` | NORMAL | LOW | VERIFIED | HIDDEN | 显示帮助 |
| `--version` | NORMAL | LOW | VERIFIED | HIDDEN | 显示版本（节点版本核验使用） |

### 2.4 运行模式/未公开（1 个）— HIDDEN

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--server` | NORMAL | LOW | HELP_ONLY | HIDDEN | 4.3.5 官网无部署语义 |

### 2.5 数据库对象/未公开（1 个）— HIDDEN

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--public-synonym` | NORMAL | LOW | HELP_ONLY | HIDDEN | 4.3.5 官网未列出含义 |

### 2.6 连接与会话/未公开（1 个）— VALIDATION_GATED

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--tenant` | IDENTIFIER | LOW | CONFLICT_PENDING | VALIDATION_GATED | 独立长参数与 user@tenant#cluster 优先级待实测 |

### 2.6 基础选项 · 连接选项（11 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--host` | IDENTIFIER | LOW | VERIFIED | ENABLED | 数据源快照派生 |
| `--port` | NORMAL | LOW | VERIFIED | ENABLED | 数据源快照派生 |
| `--user` | IDENTIFIER | LOW | VERIFIED | ENABLED | 数据源快照派生 |
| `--password` | SECRET | LOW | VERIFIED | ENABLED | 秘密槽位，不进入 argv |
| `--cluster` | IDENTIFIER | LOW | VERIFIED | ENABLED | 数据源快照派生 |
| `--database` | IDENTIFIER | LOW | VERIFIED | ENABLED | 任务显式值，数据源可回填 |
| `--no-sys` | NORMAL | MEDIUM | VERIFIED | ENABLED | 数据源类型派生，影响元数据能力 |
| `--public-cloud` | NORMAL | LOW | VERIFIED | ENABLED | 数据源类型派生 |
| `--sys-user` | IDENTIFIER | MEDIUM | VERIFIED | ENABLED | 安全配置派生，依赖 sys 权限可用性 |
| `--sys-password` | SECRET | MEDIUM | VERIFIED | ENABLED | 安全配置派生，秘密槽位 |
| `--logical-database` | NORMAL | HIGH | VERIFIED | ENABLED | 专家配置+风险确认，结果不能直接导入 |

### 2.7 基础选项 · 功能选项 · 文件格式（24 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--csv` | NORMAL | LOW | VERIFIED | ENABLED | 推荐格式 |
| `--cut` | NORMAL | LOW | VERIFIED | ENABLED | 字符串分隔格式 |
| `--pos` | NORMAL | LOW | VERIFIED（2026-08-07 实测） | ENABLED（待产品接入） | 定长格式，独立 `--pos` + `--ctl-path` + `<表名>.ctrl`，见 [受控实测与定版](../03-technical/evidence/windows-pos-format-validation-2026-08-07.md) |
| `--sql` | NORMAL | LOW | VERIFIED | ENABLED | Insert SQL 格式 |
| `--par` | NORMAL | LOW | VERIFIED（官方格式表，2026-08-07 接入） | ENABLED | Parquet 列式格式；压缩/序列化不适用，--block-size 不生效 |
| `--orc` | NORMAL | MEDIUM | VERIFIED（官方格式表，2026-08-07 接入） | ENABLED | ORC 列式格式；内存风险较高，压缩/序列化不适用 |
| `--avro` | NORMAL | LOW | VERIFIED（官方格式表，2026-08-07 接入） | ENABLED | Avro 格式；压缩/序列化不适用，--block-size 未取证 |
| `--ddl` | NORMAL | LOW | VERIFIED | ENABLED | 对象定义导出 |
| `--character-set` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 专家配置，字符集影响待验证 |
| `--escape-character` | NORMAL | LOW | VERIFIED | ENABLED | CSV/CUT 转义字符 |
| `--file-encoding` | NORMAL | LOW | VERIFIED | ENABLED | 输出文件编码 |
| `--line-separator` | NORMAL | LOW | VERIFIED | ENABLED | 行分隔符 |
| `--null-string` | NORMAL | LOW | VERIFIED | ENABLED | NULL 替换字符串 |
| `--with-trim` | NORMAL | LOW | VERIFIED | ENABLED | 去除左右空格 |
| `--skip-header` | NORMAL | LOW | VERIFIED | ENABLED | 省略 CSV 字段头 |
| `--column-separator` | NORMAL | LOW | VERIFIED | ENABLED | CSV 列分隔符 |
| `--column-quote` | NORMAL | LOW | VERIFIED | ENABLED | CSV 列包围符 |
| `--column-quote-mode` | NORMAL | LOW | VERIFIED | ENABLED | CSV 包围模式（5 种枚举） |
| `--column-delimiter` | NORMAL | LOW | HELP_ONLY | HIDDEN | 已过时，与 --column-quote 同义 |
| `--column-splitter` | NORMAL | LOW | VERIFIED（CUT 专属；POS 已实测定版为独立 `--pos`） | ENABLED（待产品接入） | CUT 分隔字符串 |
| `--trail-delimiter` | NORMAL | LOW | VERIFIED | ENABLED | 行尾分隔符 |
| `--drop-object` | NORMAL | HIGH | VERIFIED | ENABLED | 前置 DROP（高风险+二次确认） |
| `--compact-schema` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测） | ENABLED | show create table 检索文本；当前库无差异，残余说明见证据文档 |
| `--flashback-scn` | NORMAL | MEDIUM | VERIFIED | ENABLED | 闪回 SCN，官方归类为文件格式伴生参数 |

### 2.8 基础选项 · 功能选项 · 压缩导出（3 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--compress` | NORMAL | LOW | VERIFIED | ENABLED | 启用压缩 |
| `--compression-algo` | NORMAL | LOW | VERIFIED | ENABLED | 压缩算法（zstd/zlib/gzip/snappy） |
| `--compression-level` | NORMAL | LOW | OFFICIAL_ONLY | ENABLED（2026-08-10 接入） | 压缩等级，随算法分范围（zstd 1~22、zlib -1~9；gzip/snappy 不支持） |

### 2.9 基础选项 · 功能选项 · 数据库对象类型（16 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--all` | NORMAL | LOW | VERIFIED | ENABLED | 全部对象导出 |
| `--table` | IDENTIFIER | LOW | VERIFIED | ENABLED | 表，支持多名称和表达式 |
| `--table-group` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 表组，仅 DDL，版本条件待确认 |
| `--view` | IDENTIFIER | LOW | VERIFIED | ENABLED | 视图，仅 DDL |
| `--trigger` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 触发器，仅 Oracle，仅 DDL |
| `--obj-user` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 用户定义，公有云限制待确认 |
| `--role` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 角色，仅 Oracle，公有云限制 |
| `--sequence` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 序列，数据库版本条件待确认 |
| `--sequence-policy` | NORMAL | LOW | CONFLICT_PENDING | VALIDATION_GATED | 序列策略，4.3.5 约束待确认 |
| `--synonym` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 同义词，仅 Oracle |
| `--type` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 类型，仅 Oracle |
| `--type-body` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 类型体，依赖 --type |
| `--package` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 包，仅 Oracle |
| `--package-body` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 包体，仅 Oracle |
| `--function` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 函数，兼容模式条件待确认 |
| `--procedure` | IDENTIFIER | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 存储过程，兼容模式条件待确认 |

### 2.10 基础选项 · 功能选项 · 存储路径（5 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--file-path` | NORMAL | LOW | VERIFIED | ENABLED | 必填，本地路径或受控 URI |
| `--log-path` | NORMAL | LOW | VERIFIED | ENABLED | 可选日志目录 |
| `--no-nested-dir` | NORMAL | LOW | VERIFIED | ENABLED | 扁平目录 |
| `--ctl-path` | NORMAL | LOW | VERIFIED（2026-08-07 实测） | ENABLED（待产品接入） | 控制文件目录，仅 POS 显示；用户提供 / 自动生成双来源 |
| `--tmp-path` | NORMAL | MEDIUM | VERIFIED（jar 字节码取证，2026-08-07） | ENABLED | 对象存储 Multipart 本地临时分块目录；存储凭据走执行槽位（HADOOP_CONF_DIR/core-site.xml），URI 拒绝密钥参数 |

### 2.11 基础选项 · 其他选项（2 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--help` | NORMAL | LOW | VERIFIED | HIDDEN | 显示帮助 |
| `--version` | NORMAL | LOW | VERIFIED | HIDDEN | 显示版本（节点版本核验使用） |

### 2.12 高级选项 · 功能选项 · 时间戳格式（11 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--date-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | CSV/CUT，兼容模式相关 |
| `--time-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | CSV/CUT + MySQL |
| `--datetime-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | CSV/CUT + MySQL |
| `--timestamp-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | CSV/CUT |
| `--timestamp-tz-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | CSV/CUT + Oracle |
| `--timestamp-ltz-value-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | CSV/CUT + Oracle |
| `--preserve-zero-datetime` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | MySQL 专家配置 |
| `--nls-date-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | Oracle 专家配置 |
| `--nls-timestamp-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | Oracle 专家配置 |
| `--nls-timestamp-tz-format` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | Oracle 专家配置 |
| `--flashback-timestamp` | NORMAL | MEDIUM | VERIFIED | ENABLED | 闪回时间戳，仅 Oracle |

### 2.13 高级选项 · 功能选项 · 黑白名单筛选（12 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--query-sql` | NORMAL | HIGH | VERIFIED | ENABLED | 受限专家能力：CAP_SENSITIVE_COMMAND + 二次确认；纯文本输入（不提供 SQL 编辑器）；file:// 不开放；与 where/partition 互斥 |
| `--where` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测） | ENABLED | 条件筛选，行数生效（109 vs 1,000） |
| `--partition` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 分区筛选，二级分区规则待确认 |
| `--include-column-names` | IDENTIFIER | LOW | VERIFIED | ENABLED | 列筛选 |
| `--exclude-column-names` | IDENTIFIER | LOW | VERIFIED | ENABLED | 列排除，与控制文件互斥 |
| `--exclude-data-types` | NORMAL | LOW | OFFICIAL_ONLY | VALIDATION_GATED | 数据类型排除，兼容模式条件待确认 |
| `--exclude-virtual-columns` | NORMAL | LOW | VERIFIED | ENABLED | 排除生成列 |
| `--exclude-table` | IDENTIFIER | LOW | VERIFIED | ENABLED | 排除表 |
| `--enable-hidden-pk` | NORMAL | MEDIUM | OFFICIAL_ONLY | VALIDATION_GATED | 隐藏主键，版本/权限条件待确认 |
| `--fetch-size` | NORMAL | LOW | VERIFIED | ENABLED | 游标抓取行数（官方正文归列黑白名单筛选节） |
| `--add-extra-message` | NORMAL | MEDIUM | VERIFIED | ENABLED | 附加对象信息，依赖 sys 权限 |
| `--retain-empty-files` | NORMAL | LOW | VERIFIED | ENABLED | 空结果文件保留 |

### 2.14 高级选项 · 功能选项 · 错误处理（6 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--skip-check-dir` | NORMAL | MEDIUM | VERIFIED | ENABLED | 跳过导出目录空性检查；仍检查路径、允许根目录、可写性和空间 |
| `--max-file-size` | NORMAL | LOW | VERIFIED | ENABLED | 导出总量上限 |
| `--remove-newline` | NORMAL | HIGH | VERIFIED | ENABLED | 删除换行（仅 CUT，高风险+二次确认） |
| `--retry` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测） | ENABLED | 从保存点继续；无保存点失败关闭，续跑并入 EX-I8 |
| `--snapshot` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测） | ENABLED | 一致性快照导出 |
| `--weak-read` | NORMAL | MEDIUM | VERIFIED（2026-08-11 实测） | ENABLED | 从备库读（follower server） |

### 2.15 高级选项 · 性能选项（5 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--thread` | NORMAL | LOW | VERIFIED | ENABLED | 导出线程数 |
| `--page-size` | NORMAL | LOW | VERIFIED | ENABLED | 分页大小 |
| `--parallel-macro` | NORMAL | LOW | VERIFIED | ENABLED | 每线程宏块数 |
| `--mem` | NORMAL | MEDIUM | VERIFIED | ENABLED | JVM 内存（节点资源风险） |
| `--block-size` | NORMAL | LOW | CONFLICT_PENDING（默认值残余；显式传值已实测 MB/ROW 生效） | ENABLED（2026-08-10 接入） | 默认值官网冲突保留；结构化格式不适用 |

### 2.16 高级选项 · 其他选项（2 个）

| 参数 | sensitivity | riskLevel | evidenceState | v1State | 说明 |
|---|---|---|---|---|---|
| `--session-config` | NORMAL | LOW | VERIFIED | ENABLED | 节点只读派生 |
| `--retain-schema` | NORMAL | LOW | VERIFIED | ENABLED | 保留 Schema 前缀 |

### 2.17 V1.0 状态汇总

| v1State | 参数数量 | 占比 |
|---|---:|---:|
| ENABLED | 67 | 61.5% |
| VALIDATION_GATED | 28 | 25.7% |
| HIDDEN | 14 | 12.8% |
| BLOCKED | 0 | 0% |
| **合计** | **109** | **100%** |

---

## 3. 87 字段 V1.0 支持状态

### 3.1 步骤 1：数据源与连接（EX-F001~F008）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F001 | 数据源 | ENABLED | 当前已实现 |
| EX-F002 | 连接摘要 | ENABLED | 数据源快照只读派生 |
| EX-F003 | 默认数据库/Schema | ENABLED | 当前已实现 |
| EX-F004 | 兼容模式 | ENABLED | 数据源属性只读派生 |
| EX-F005 | 连接环境 | ENABLED | 数据源类型派生只读 |
| EX-F006 | sys 凭据状态 | ENABLED | 数据源可选字段（2026-08-10 实现）：账号/密码成对可选，密码加密信封存储，摘要派生可用/不可用 |
| EX-F007 | 连接字符集 | VALIDATION_GATED | 专家配置，字符集影响待验证 |
| EX-F008 | 会话配置 | ENABLED | 节点只读派生 |

### 3.2 步骤 2：导出对象（EX-F009~F015）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F009 | 导出范围 | ENABLED | 全部/指定互斥单选 |
| EX-F010 | 对象类型 | ENABLED | 当前已实现表；其余对象类型 VALIDATION_GATED |
| EX-F011 | 对象表达式 | ENABLED | 当前已实现单表；多对象表达式随对象类型开放 |
| EX-F012 | 多库摘要 | VALIDATION_GATED | schema.object 表达式待对象类型矩阵确认 |
| EX-F013 | 排除表 | ENABLED | --exclude-table 已核验 |
| EX-F014 | 序列能力提示 | VALIDATION_GATED | 依赖序列对象和 --sequence-policy 实测 |
| EX-F015 | 公共同义词 | HIDDEN | help-only，4.3.5 官网未列出 |

### 3.3 步骤 3~4：内容与格式（EX-F016~F020）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F016 | 导出内容 | ENABLED | 仅 DDL/仅数据/DDL+数据 |
| EX-F017 | 数据格式 | ENABLED | CSV 已实现；CUT/SQL ENABLED；POS 映射已定版（产品接入待实现）；Parquet/ORC/Avro VALIDATION_GATED |
| EX-F018 | POS 映射状态 | ENABLED（待产品接入） | 独立 `--pos` + `--ctl-path` + `<表名>.ctrl`，2026-08-07 实测定版 |
| EX-F019 | DDL 行为分组 | VALIDATION_GATED | 依赖 sys 权限和对象类型矩阵 |
| EX-F020 | 数据行为分组 | ENABLED | 文件格式、压缩导出、存储路径、时间戳格式、黑白名单筛选、错误处理与性能选项参数按各自 supportState 显示 |

### 3.4 步骤 5：输出、筛选与性能（EX-F021~F042；官方分类：存储路径 / 压缩导出 / 时间戳格式 / 黑白名单筛选 / 错误处理 / 性能选项）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F021 | 执行节点 | ENABLED | 当前已实现 |
| EX-F022 | 输出类型 | ENABLED | 本地 ENABLED；OSS/S3/COS/OBS VALIDATION_GATED |
| EX-F023 | 导出路径 | ENABLED | 本地路径已实现 |
| EX-F024 | Bucket/路径 | VALIDATION_GATED | 对象存储待取证 |
| EX-F025 | Endpoint/Region | VALIDATION_GATED | 对象存储待取证 |
| EX-F026 | 存储凭据 | VALIDATION_GATED | 对象存储凭据安全评审待完成 |
| EX-F027 | 日志路径 | ENABLED | 当前已实现 |
| EX-F028 | 扁平目录 | ENABLED | --no-nested-dir |
| EX-F029 | 控制文件目录 | ENABLED（待产品接入） | 仅 POS 显示；用户提供 / 自动生成双来源（2026-08-07 确认） |
| EX-F030 | 对象存储临时目录 | VALIDATION_GATED | 空间预检查待验证 |
| EX-F031 | 文件拆分 | ENABLED（2026-08-10 接入） | --block-size 显式传值已实测 MB/ROW 生效；默认值冲突保留；结构化格式不适用 |
| EX-F032 | 导出总量上限 | ENABLED | --max-file-size |
| EX-F033 | 空结果文件 | ENABLED | --retain-empty-files |
| EX-F034 | 跳过目录空性检查 | ENABLED | --skip-check-dir |
| EX-F035 | 导出线程 | ENABLED | --thread |
| EX-F036 | 分页大小 | ENABLED | --page-size |
| EX-F037 | 每线程宏块数 | ENABLED | --parallel-macro |
| EX-F038 | 游标抓取行数 | ENABLED | --fetch-size |
| EX-F039 | JVM 内存 | ENABLED | --mem，展示节点资源风险 |
| EX-F040 | 启用压缩 | ENABLED | --compress |
| EX-F041 | 压缩算法 | ENABLED | --compression-algo |
| EX-F042 | 压缩等级 | ENABLED（2026-08-10 接入） | --compression-level，按算法分范围；gzip/snappy 不支持 |

### 3.5 格式序列化（EX-F043~F054；基础选项 · 功能选项 · 文件格式）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F043 | 省略 CSV 字段头 | ENABLED | --skip-header |
| EX-F044 | CSV 列分隔符 | ENABLED | --column-separator |
| EX-F045 | CSV 列包围符 | ENABLED | --column-quote |
| EX-F046 | CSV 包围模式 | ENABLED | --column-quote-mode |
| EX-F047 | 转义字符 | ENABLED | --escape-character |
| EX-F048 | CUT 列分隔字符串 | ENABLED（待产品接入） | --column-splitter，CUT 专属（POS 已定版为独立 --pos） |
| EX-F049 | 行分隔符 | ENABLED | --line-separator |
| EX-F050 | 行尾分隔符 | ENABLED | --trail-delimiter |
| EX-F051 | NULL 替换 | ENABLED | --null-string |
| EX-F052 | 文件编码 | ENABLED | --file-encoding |
| EX-F053 | 去除左右空格 | ENABLED | --with-trim |
| EX-F054 | 删除换行 | ENABLED | --remove-newline，高风险+二次确认 |

### 3.6 日期时间（EX-F055~F064；高级选项 · 功能选项 · 时间戳格式）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F055 | DATE 值格式 | VALIDATION_GATED | 兼容模式相关 |
| EX-F056 | TIME 值格式 | VALIDATION_GATED | MySQL 专属 |
| EX-F057 | DATETIME 值格式 | VALIDATION_GATED | MySQL 专属 |
| EX-F058 | TIMESTAMP 值格式 | VALIDATION_GATED | 兼容模式相关 |
| EX-F059 | TIMESTAMP TZ 值格式 | VALIDATION_GATED | Oracle 专属 |
| EX-F060 | TIMESTAMP LTZ 值格式 | VALIDATION_GATED | Oracle 专属 |
| EX-F061 | 保留时间零值 | VALIDATION_GATED | MySQL 专家配置 |
| EX-F062 | NLS DATE | VALIDATION_GATED | Oracle 专家配置 |
| EX-F063 | NLS TIMESTAMP | VALIDATION_GATED | Oracle 专家配置 |
| EX-F064 | NLS TIMESTAMP TZ | VALIDATION_GATED | Oracle 专家配置 |

### 3.7 DDL 行为（EX-F065~F069；官方分类：文件格式伴生 / 数据库对象类型 / 高级选项 · 其他选项）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F065 | 前置 DROP | ENABLED | 已接入（2026-08-10），高风险提示；仅 DDL 内容时随 ddl/ddl-csv 能力发射 |
| EX-F066 | 附加对象信息 | ENABLED | 依赖 sys 凭据可用性，待接入 |
| EX-F067 | 保留 Schema | ENABLED | 已接入（2026-08-10），--retain-schema；仅 DDL 内容时发射 |
| EX-F068 | 紧凑 Schema | VALIDATION_GATED | 4.3.5 约束不完整 |
| EX-F069 | 序列策略 | VALIDATION_GATED | 4.3.5 约束待确认 |

### 3.8 筛选与一致性（EX-F070~F081；高级选项 · 功能选项 · 黑白名单筛选 / 时间戳格式 / 错误处理）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F070 | 自定义查询 | ENABLED | 受限专家能力：CAP_SENSITIVE_COMMAND + 二次确认；纯文本（不提供 SQL 编辑器）；file:// 不开放；与 where/partition 互斥 |
| EX-F071 | 条件筛选 | VALIDATION_GATED | 表达式验证待实测 |
| EX-F072 | 分区筛选 | VALIDATION_GATED | 二级分区规则待确认 |
| EX-F073 | 包含列 | ENABLED | --include-column-names |
| EX-F074 | 排除列 | ENABLED | --exclude-column-names |
| EX-F075 | 排除数据类型 | VALIDATION_GATED | 兼容模式条件待确认 |
| EX-F076 | 排除生成列 | ENABLED | --exclude-virtual-columns |
| EX-F077 | 使用隐藏主键 | VALIDATION_GATED | 版本/权限条件待确认 |
| EX-F078 | 闪回 SCN | ENABLED | --flashback-scn |
| EX-F079 | 闪回时间点 | ENABLED | --flashback-timestamp，仅 Oracle |
| EX-F080 | 一致性快照 | VALIDATION_GATED | 组合条件待确认 |
| EX-F081 | 备副本弱读 | VALIDATION_GATED | 环境条件待确认 |

### 3.9 步骤 6：确认页（EX-F082~F087）

| ID | 字段 | supportState | 说明 |
|---|---|---|---|
| EX-F082 | 配置摘要 | ENABLED | 当前已实现基础版 |
| EX-F083 | 预检查 | ENABLED | 六项固定检查已实现；扩展检查随能力开放 |
| EX-F084 | 完整命令 | ENABLED | 当前已实现 |
| EX-F085 | 复制命令 | ENABLED | 脱敏版本复制 |
| EX-F086 | 风险确认 | ENABLED | 当前已实现 |
| EX-F087 | 提交执行 | ENABLED | 当前已实现 |

### 3.10 字段支持状态汇总

| supportState | 字段数量 | 占比 |
|---|---:|---:|
| ENABLED | 55 | 63.2% |
| VALIDATION_GATED | 31 | 35.6% |
| HIDDEN | 1 | 1.1% |
| BLOCKED | 0 | 0% |
| **合计** | **87** | **100%** |

---

## 4. 组合矩阵

### 4.1 对象类型 x 兼容模式 x 数据库版本 x 权限

| 对象类型 | 官方参数 | MySQL | Oracle | 公有云限制 | 数据能力 | V1.0 状态 |
|---|---|---|---|---|---|---|
| 全部对象 | `--all` | 全部版本 | 全部版本 | 无 | DDL + 数据（含全部表数据） | ENABLED |
| 表 | `--table` | 全部版本 | 全部版本 | 无 | DDL + 数据 | ENABLED |
| 表组 | `--table-group` | 适用版本待确认 | 适用版本待确认 | 待确认 | 仅 DDL | VALIDATION_GATED |
| 视图 | `--view` | 全部版本 | 全部版本 | 无 | 仅 DDL | ENABLED |
| 触发器 | `--trigger` | 不适用 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 用户 | `--obj-user` | 全部版本 | 全部版本 | 不支持 | 仅 DDL | VALIDATION_GATED |
| 角色 | `--role` | 不适用 | 适用版本 | 不支持 | 仅 DDL | VALIDATION_GATED |
| 序列 | `--sequence` | 版本条件待确认 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 同义词 | `--synonym` | 不支持 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 类型 | `--type` | 不适用 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 类型体 | `--type-body` | 不适用 | 适用版本 | 无 | 仅 DDL，依赖 --type | VALIDATION_GATED |
| 包 | `--package` | 不适用 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 包体 | `--package-body` | 不适用 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 函数 | `--function` | 版本条件待确认 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |
| 存储过程 | `--procedure` | 版本条件待确认 | 适用版本 | 无 | 仅 DDL | VALIDATION_GATED |

**对象选择规则**：
- `--all` 与任意具体对象参数互斥，同时出现时阻断
- 仅 DDL 对象（表组、视图、触发器等）不能与"仅数据"组合
- `--type-body` 未选择对应 `--type` 时阻断
- 兼容模式不支持的对象类型不显示或禁用

### 4.2 内容 x 格式

| 内容 | CSV | CUT | POS | SQL | Parquet | ORC | Avro |
|---|---|---|---|---|---|---|---|
| 仅 DDL | N/A | N/A | N/A | N/A | N/A | N/A | N/A |
| 仅数据 | ENABLED | ENABLED | ENABLED（待产品接入） | ENABLED | ENABLED（2026-08-07） | ENABLED（2026-08-07） | ENABLED（2026-08-07） |
| DDL + 数据 | ENABLED | ENABLED | GATED | ENABLED | GATED | GATED | GATED |

**规则**：
- 仅 DDL 时不生成数据格式参数，跳过格式步骤
- 同一任务只允许一种数据格式
- 格式切换保留各格式草稿值（命名空间隔离），提交只保存当前格式
- POS 映射已实测定版（独立 `--pos` + `--ctl-path` + `<表名>.ctrl`，2026-08-07）；产品接入后放开格式选择，无控制文件时阻断提交

### 4.3 输出位置

| 输出位置 | 全部文本格式 | Parquet/ORC/Avro | 仅 DDL | V1.0 状态 |
|---|---|---|---|---|
| 本地路径 | 适用 | 适用 | 适用 | ENABLED |
| OSS | 适用 | 适用 | 适用 | VALIDATION_GATED |
| S3 | 适用 | 适用 | 适用 | VALIDATION_GATED |
| COS | 适用 | 适用 | 适用 | VALIDATION_GATED |
| OBS | 适用 | 适用 | 适用 | VALIDATION_GATED |

**规则**：
- 对象存储 URI 由 scheme + bucket + path 组成，不提供任意 URI 输入
- 存储访问密钥和 secret-key 按秘密处理，不进入 argv/日志/SQLite 普通字段
- `--block-size` 对 ORC/Parquet 不生效
- `--tmp-path` 仅对象存储场景

---

## 5. 预检查/结果/失败处理差异

### 5.1 按输出位置的预检查差异

| 预检查项 | 本地路径 | OSS/S3/COS/OBS |
|---|---|---|
| 路径可写 | Agent 本机目录创建/写入检查 | 网络连通 + Bucket 写权限 + Prefix 检查 |
| 目录空性 | 直接检查目录是否为空 | 检查 Bucket/path 前缀下是否已有对象 |
| 可用空间 | 最少 1 GiB | 临时目录空间 + Bucket 配额（如适用） |
| 凭据检查 | 不需要 | 存储访问密钥/secret-key 槽位完整性 |
| 网络连通 | 不需要 | Endpoint/Region 可达性 |
| 允许根目录 | Agent 本机白名单校验 | 不适用 |

### 5.2 按内容的预检查差异

| 预检查项 | 仅 DDL | 仅数据 | DDL + 数据 |
|---|---|---|---|
| 数据库连接 | 需要 | 需要 | 需要 |
| 对象访问 | DDL 元数据可读（SHOW CREATE） | 数据可读（零行 SELECT + 元数据） | 两者都需要 |
| sys 权限 | --add-extra-message 时需要 | 不需要 | --add-extra-message 时需要 |
| 对象类型校验 | 每种对象类型的兼容模式/版本检查 | 仅表和 --all 可与数据组合 | 同左 |

### 5.3 按格式的预检查差异

| 预检查项 | CSV/CUT/POS/SQL | Parquet/ORC | Avro |
|---|---|---|---|
| 序列化参数 | 分隔符/包围符/转义字符合法性 | 不适用 | 不适用 |
| 压缩 | --compress + --compression-algo + --compression-level 组合 | 不适用 | 不适用 |
| 资源评估 | 标准 | 内存风险较高（ORC） | 标准 |

### 5.4 按格式的结果和失败差异

| 格式 | 结果文件 | MANIFEST 结构 | dump.ckpt 继续 | 失败特征 |
|---|---|---|---|---|
| CSV | .csv 数据文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用 | 部分文件可能已生成 |
| CUT | .cut 数据文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用 | 部分文件可能已生成 |
| POS | .dat 定长文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用 | 部分文件可能已生成 |
| SQL | .sql 数据文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用 | 部分文件可能已生成 |
| Parquet | .parquet 文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用（待验证） | 列式文件特征 |
| ORC | .orc 文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用（待验证） | 列式文件特征，内存溢出风险 |
| Avro | .avro 文件 + MANIFEST | 文件列表 + 行数 + 字节数 | 适用（待验证） | 结构化文件特征 |
| 仅 DDL | .sql DDL 文件 + MANIFEST | 对象列表 + 类型 | 不适用 | 部分 DDL 可能已生成 |

### 5.5 失败处理操作

| 操作 | 条件 | 行为 |
|---|---|---|
| 从头重新执行 | 任何失败任务 | 使用原不可变快照创建新任务 |
| 基于原配置新建 | 任何失败或成功任务 | 复制配置创建新草稿，可修改后提交 |
| 检查点继续 | 失败任务 + dump.ckpt 存在 + 原参数/路径/工具版本不变 | 使用 --retry 创建新任务，保持原快照 |
| 检查点继续（不满足） | dump.ckpt 不存在或参数/路径/工具变化 | 不提供继续选项，仅从头或新建 |

---

## 6. 关键决策记录

### 6.1 --query-sql 受限专家能力

**决策**：ENABLED（受限专家能力）

**约束**：
- 要求 `CAP_SENSITIVE_COMMAND` 权限
- 二次风险确认（勾选或弹窗）
- 纯文本输入，不提供 SQL 编辑器、语法高亮或自动补全
- `file://` 引用不开放（避免引入文件浏览/上传能力）
- 与 `--where`、`--partition` 互斥
- 仍由命令生成器唯一拼装，调用方不得自行拼接
- 日志双层脱敏规则覆盖 SQL 文本内容

**影响**：
- AGENTS.md 第 5 节需更新：将"不得提供任意 SQL"收窄为"不提供 SQL 编辑器、SQL 文件浏览或任意 SQL 传递到非 OBDUMPER 目标；允许 OBDUMPER 官方 `--query-sql` 作为受限专家能力"
- export-module.md EX-R07 需更新
- export-field-rules.md EX-F070 需更新

### 6.2 POS 映射（已定版，待产品接入）

2026-08-07 受控实测定版：4.3.5 实际二进制支持独立 `--pos`，必须搭配 `--ctl-path` 与 `<表名>.ctrl` 控制文件（`position(字节长度)` 定义定长列）；官网 4.3.6“CUT + 空 splitter”口径与 4.3.5 行为不符，不再使用。控制文件来源“用户提供 / 自动生成”双支持已确认。详见 [Windows POS 受控实测与定版](../03-technical/evidence/windows-pos-format-validation-2026-08-07.md)。

### 6.3 --block-size（显式传值已实测，默认值残余）

`--block-size 1`（1MB）与 `--block-size 256ROW`（256 行/文件）已在 4.3.5 上实测生效（按行粒度切分，命名 `<表名>.<序号>.dat`）；官网默认值冲突（正文 0 vs 选项表 1024MB）在 ≤1024MB 数据下不可证伪区分，保留为低风险残余，V1.0 显式传值时按参数生效。

### 6.4 --sequence-policy（保持 VALIDATION_GATED）

等待 4.3.5 当前页面的约束条件确认。

### 6.5 --tenant（保持 VALIDATION_GATED）

等待独立 `--tenant` 长参数与 `user@tenant#cluster` 组合形式的优先级实测。
