# 导出模块字段与条件矩阵

> 文档状态：字段规则已确认，V1.0 支持状态已标注，官方冲突项待实测
> 模块：OBDUMPER 导出
> 目标版本：4.3.5-RELEASE
> 更新日期：2026-08-05
> 产品决策依据：[DEC-014](../01-product/decisions.md#dec-014-导出模块专项产品规则)
> 参数事实依据：[导出参数映射基线](export-parameter-mapping.md)
> V1.0 支持状态依据：[V1.0 支持矩阵](export-v1-support-matrix.md)

## 1. 目的与边界

本文把已确认的 EX-R01～EX-R17 落到字段层，明确字段位置、显示条件、必填条件、默认来源、清值方式、命令映射和预检查失效范围。不定义 API、数据库表、前端组件或命令生成代码。

## 2. 字段状态与通用规则

| 状态 | 含义 | 生成命令 | 提交快照 |
|---|---|---:|---:|
| 未设置 | 沿用 OBDUMPER 4.3.5 官方默认 | 否 | 记录默认来源，精确方式待评审 |
| 已显式设置 | 用户在当前适用条件下设置 | 是 | 是 |
| 非活动 | 因内容、格式、对象或环境条件不适用 | 否 | 否；草稿期可暂存 |
| 派生 | 从数据源、节点、安全配置或其他字段计算 | 需要时 | 是，敏感值脱敏 |
| 阻断 | 无确定映射或预检查失败 | 否 | 不允许提交 |

**V1.0 支持状态**：每个字段额外标注 `supportState`（ENABLED / VALIDATION_GATED / HIDDEN / BLOCKED），定义见 [V1.0 支持矩阵](export-v1-support-matrix.md) 第 1 节。VALIDATION_GATED 的字段在页面显示但标记"待验证"，受控实测完成前不参与最终命令生成。

规则：

- 未设置时显示"官方默认"或"继承执行节点配置"，不伪装成用户输入，也不主动生成参数。
- 已知默认值可作帮助文本，例如 `--page-size` 为 1,000,000、`--mem` 为 4G。
- `--block-size` 的官网默认值存在 0 与 1024MB 冲突，不显示确定默认值。
- 格式切换造成的可逆隐藏按格式命名空间保留草稿值，但立即停止生效。
- 数据源、对象或兼容模式变化导致值必然非法时，列出受影响字段，经确认后清除。
- 提交只保存活动字段、派生字段、最终命令和必要审计信息，不保存其他格式的隐藏草稿值。
- 影响风险事实的字段变化只清除对应风险确认。

## 3. 步骤 1：数据源与连接

| ID | 字段 | 参数/来源 | 显示与必填 | 默认/派生 | 变化影响 | supportState |
|---|---|---|---|---|---|---|
| EX-F001 | 数据源 | 平台数据源 | 始终显示、必填、单选 | 无默认 | 清除对象选择，全部预检查失效 | ENABLED |
| EX-F002 | 连接摘要 | `--host`、`--port`、`--user`、`--password`、`--cluster` | 选源后只读 | 数据源快照派生，密码不回显 | 重新生成连接参数 | ENABLED |
| EX-F003 | 默认数据库 / Schema | `--database` | 始终显示、必填 | 数据源默认值可回填，任务可覆盖 | 对象、权限和命令检查失效 | ENABLED |
| EX-F004 | 兼容模式 | 数据源属性 | 只读 | 自动识别或"待校验" | 重新判定 Oracle/MySQL 专属字段 | ENABLED |
| EX-F005 | 连接环境 | `--public-cloud`、`--no-sys`、`--logical-database` | 仅适用环境只读显示 | 数据源类型与安全配置派生 | 对象、sys 权限和风险检查失效 | ENABLED |
| EX-F006 | sys 凭据状态 | `--sys-user`、`--sys-password` | 只显示可用/不可用/不需要 | 安全配置派生 | DDL 与元数据预检查失效 | ENABLED |
| EX-F007 | 连接字符集 | `--character-set` | 专家配置 | 未设置继承会话配置 | 编码检查和命令失效 | VALIDATION_GATED |
| EX-F008 | 会话配置 | `--session-config` | 节点配置只读摘要 | 执行节点派生 | 更换节点后重新派生 | ENABLED |

向导不再次编辑地址、端口、用户名、租户、集群或密码。连接环境参数是否完全派生进入 EX-FR05～EX-FR07 评审。

## 4. 步骤 2：导出对象

| ID | 字段 | 参数 | 显示与必填 | 默认 | 变化影响 | supportState |
|---|---|---|---|---|---|---|
| EX-F009 | 导出范围 | `--all` 或具体对象参数 | 始终必填；全部/指定严格单选 | 无 | 切换前确认清除另一范围 | ENABLED |
| EX-F010 | 对象类型 | table、view、sequence 等 | 指定对象时显示；至少一项 | 无 | 重算内容、兼容模式和版本条件 | ENABLED |
| EX-F011 | 对象表达式 | 对象参数值 | 指定对象时每行必填 | 无 | 对象、权限、多库检查失效 | ENABLED |
| EX-F012 | 多库摘要 | `schema.object` | 出现 Schema 前缀时自动显示 | 不提供独立开关 | 重算默认数据库和多库摘要 | VALIDATION_GATED |
| EX-F013 | 排除表 | `--exclude-table` | 全部导出或包含表时；专家配置 | 未设置 | 对象范围和命令失效 | ENABLED |
| EX-F014 | 序列能力提示 | 序列对象选择结果 | 选择序列对象时只读显示 | 提示后续专家配置存在序列策略 | 取消序列时隐藏；实际字段见 EX-F069 | VALIDATION_GATED |
| EX-F015 | 公共同义词 | `--public-synonym` | V1.0 不显示 | 无 | help-only，待确认 | HIDDEN |

对象类型与兼容模式、数据库版本的完整矩阵见 [V1.0 支持矩阵](export-v1-support-matrix.md) 第 4.1 节。

## 5. 步骤 3～4：内容与格式

| ID | 字段 | 参数 | 显示与必填 | 默认 | 变化影响 | supportState |
|---|---|---|---|---|---|---|
| EX-F016 | 导出内容 | `--ddl` 与数据格式组合 | 始终必填；仅 DDL/仅数据/DDL+数据 | 无 | 仅 DDL 时数据字段非活动 | ENABLED |
| EX-F017 | 数据格式 | `--csv`、`--cut`、POS、`--sql`、`--par`、`--orc`、`--avro` | 包含数据时必填、单选 | CSV 只作推荐，不自动选 | 按格式命名空间切换草稿值 | ENABLED（POS 待产品接入） |
| EX-F018 | POS 映射状态 | 独立 `--pos` + `--ctl-path` + `<表名>.ctrl` | 选择 POS 时展示控制文件来源（用户提供/自动生成） | 2026-08-07 实测定版 | 未提供控制文件时阻断提交 | ENABLED（待产品接入） |
| EX-F019 | DDL 行为分组 | DDL 专属参数 | 包含 DDL 时显示 | 折叠、未设置 | 仅数据时非活动 | VALIDATION_GATED |
| EX-F020 | 数据行为分组 | 文件格式、压缩导出、存储路径、时间戳格式、黑白名单筛选、错误处理与性能选项参数 | 包含数据时显示 | 未设置 | 仅 DDL 时非活动 | ENABLED |

## 6. 步骤 5：输出、筛选与性能（官方分类：基础选项 · 功能选项 · 存储路径 / 压缩导出；高级选项 · 功能选项 · 时间戳格式 / 黑白名单筛选 / 错误处理；高级选项 · 性能选项）

| ID | 字段 | 参数/来源 | 层级 | 显示与必填 | 默认/清值 | supportState |
|---|---|---|---|---|---|---|
| EX-F021 | 执行节点 | 平台调度字段 | 普通 | 始终必填 | 更换后节点相关检查失效 | ENABLED |
| EX-F022 | 输出类型 | `--file-path` 语义 | 普通 | 本地、OSS、S3、COS、OBS 单选 | 类型专属值分草稿命名空间 | ENABLED |
| EX-F023 | 导出路径 | `--file-path` | 普通 | 本地输出必填；用户填写本次任务完整节点绝对路径，平台不追加子目录 | 切到对象存储后非活动 | ENABLED |
| EX-F024 | Bucket / 路径 | `--file-path` URI | 普通 | 对象存储必填 | 按存储类型保存草稿 | VALIDATION_GATED |
| EX-F025 | Endpoint / Region | URI 参数 | 普通 | 按存储类型条件必填 | 不适用值清除 | VALIDATION_GATED |
| EX-F026 | 存储凭据 | URI 敏感参数 | 普通 | 对象存储按官方要求 | 脱敏；变化后检查与确认失效 | VALIDATION_GATED |
| EX-F027 | 日志路径 | `--log-path` | 步骤 5 | 可选；填写时必须为节点完整绝对路径并随草稿冻结 | 未设置沿用 OBDUMPER 默认行为 | ENABLED |
| EX-F028 | 扁平目录 | `--no-nested-dir` | 高级 | 可选 | 默认未设置 | ENABLED |
| EX-F029 | 控制文件目录 | `--ctl-path` | 专家 | 仅 POS 格式显示；来源二选一：用户提供节点目录 / 自动生成（预检查阶段按对象元数据生成 `<表名>.ctrl`） | 取消 POS 后非活动 | ENABLED（待产品接入） |
| EX-F030 | 对象存储临时目录 | `--tmp-path` | 高级 | 对象存储输出时显示；节点绝对路径 | 未设置继承节点临时目录 | ENABLED（2026-08-07） |
| EX-F031 | 文件拆分 | `--block-size` | 高级 | ORC/Parquet 隐藏；显式传值按 MB/ROW 生效（2026-08-07 实测） | 默认值 0/1024MB 冲突保留为低风险残余；隐藏后不生成 | VALIDATION_GATED（待产品接入） |
| EX-F032 | 导出总量上限 | `--max-file-size` | 高级 | 包含数据时可选 | 未设置不增加限制 | ENABLED |
| EX-F033 | 空结果文件 | `--retain-empty-files` | 高级 | where/partition 等场景 | 退出筛选后非活动 | ENABLED |
| EX-F034 | 跳过导出目录空性检查 | `--skip-check-dir` | 步骤 5 | 可选；勾选时生成 | 只跳过导出目录是否为空，仍检查路径、允许根目录、可写性和空间 | ENABLED |
| EX-F035 | 导出线程 | `--thread` | 高级 | 可选 | 继承官方默认；多 DDL 提示不宜超过 4 | ENABLED |
| EX-F036 | 分页大小 | `--page-size` | 高级 | 包含数据时可选 | 继承 1,000,000 | ENABLED |
| EX-F037 | 每线程宏块数 | `--parallel-macro` | 高级 | 适用模式显示，条件待确认 | 继承 8 | ENABLED |
| EX-F038 | 游标抓取行数 | `--fetch-size` | 高级 | Oracle + OceanBase JDBC | 继承 1000 | ENABLED |
| EX-F039 | JVM 内存 | `--mem` | 高级 | 可选，K/M/G/T | 继承 4G；不自动推荐 | ENABLED |
| EX-F040 | 启用压缩 | `--compress` | 高级 | CSV/CUT/POS/SQL | 关闭后算法和等级非活动 | ENABLED |
| EX-F041 | 压缩算法 | `--compression-algo` | 高级 | 启用压缩后 | 继承 zstd | ENABLED |
| EX-F042 | 压缩等级 | `--compression-level` | 高级 | zstd/zlib；gzip/snappy 隐藏 | 按算法保存草稿值 | VALIDATION_GATED |

## 7. 格式序列化（基础选项 · 功能选项 · 文件格式）

| ID | 字段 | 参数 | 显示条件 | 默认/范围 | 切换规则 | supportState |
|---|---|---|---|---|---|---|
| EX-F043 | 省略 CSV 字段头 | `--skip-header` | CSV | 继承官方行为 | 离开 CSV 后非活动 | ENABLED |
| EX-F044 | CSV 列分隔符 | `--column-separator` | CSV | 英文逗号；4.3.5 支持多字符 | 离开 CSV 后非活动 | ENABLED |
| EX-F045 | CSV 列包围符 | `--column-quote` | CSV | 英文单引号 | 不提供废弃别名 | ENABLED |
| EX-F046 | CSV 包围模式 | `--column-quote-mode` | CSV | non_numeric；五种枚举 | 离开 CSV 后非活动 | ENABLED |
| EX-F047 | 转义字符 | `--escape-character` | CSV/CUT | CSV 默认 null，CUT 默认反斜杠 | 按格式保存 | ENABLED |
| EX-F048 | CUT 列分隔字符串 | `--column-splitter` | CUT；POS 待实测 | 无统一默认 | 离开 CUT/POS 后非活动 | VALIDATION_GATED |
| EX-F049 | 行分隔符 | `--line-separator` | CSV/CUT/POS/SQL | 官方平台换行形式 | 离开适用格式后非活动 | ENABLED |
| EX-F050 | 行尾分隔符 | `--trail-delimiter` | CUT/POS | 继承官方行为 | 离开后非活动 | ENABLED |
| EX-F051 | NULL 替换 | `--null-string` | CSV/CUT | `\N` | 离开后非活动 | ENABLED |
| EX-F052 | 文件编码 | `--file-encoding` | 任意导出内容 | UTF-8 | 与连接字符集分开 | ENABLED |
| EX-F053 | 去除左右空格 | `--with-trim` | CSV/CUT | 未设置 | 离开后非活动 | ENABLED |
| EX-F054 | 删除换行 | `--remove-newline` | CUT | 未设置、高风险 | 开启需确认数据变化 | ENABLED |

## 8. 日期时间、DDL 与筛选（官方分类：时间戳格式 / 黑白名单筛选 / 错误处理；DDL 行为参数按官方归文件格式伴生或高级选项）

### 8.1 日期时间与会话（高级选项 · 功能选项 · 时间戳格式）

| ID | 字段 | 参数 | 显示条件 | 处理 | supportState |
|---|---|---|---|---|---|
| EX-F055 | DATE 值格式 | `--date-value-format` | CSV/CUT | 按兼容模式显示默认 | VALIDATION_GATED |
| EX-F056 | TIME 值格式 | `--time-value-format` | CSV/CUT + MySQL | 继承官方格式 | VALIDATION_GATED |
| EX-F057 | DATETIME 值格式 | `--datetime-value-format` | CSV/CUT + MySQL | 继承官方格式 | VALIDATION_GATED |
| EX-F058 | TIMESTAMP 值格式 | `--timestamp-value-format` | CSV/CUT | 按兼容模式继承 | VALIDATION_GATED |
| EX-F059 | TIMESTAMP TZ 值格式 | `--timestamp-tz-value-format` | CSV/CUT + Oracle | 继承官方格式 | VALIDATION_GATED |
| EX-F060 | TIMESTAMP LTZ 值格式 | `--timestamp-ltz-value-format` | CSV/CUT + Oracle | 继承官方格式 | VALIDATION_GATED |
| EX-F061 | 保留时间零值 | `--preserve-zero-datetime` | MySQL + 适用类型 | 默认未设置 | VALIDATION_GATED |
| EX-F062 | NLS DATE | `--nls-date-format` | Oracle 专家配置 | 提示它不是文件值格式 | VALIDATION_GATED |
| EX-F063 | NLS TIMESTAMP | `--nls-timestamp-format` | Oracle 专家配置 | 提示它不是文件值格式 | VALIDATION_GATED |
| EX-F064 | NLS TIMESTAMP TZ | `--nls-timestamp-tz-format` | Oracle 专家配置 | 提示它不是文件值格式 | VALIDATION_GATED |

### 8.2 DDL 行为（基础选项 · 功能选项 · 文件格式 / 数据库对象类型；高级选项 · 其他选项）

| ID | 字段 | 参数 | 显示条件 | 处理 | supportState |
|---|---|---|---|---|---|
| EX-F065 | 前置 DROP | `--drop-object` | 包含 DDL | 默认未设置；风险提示 | ENABLED |
| EX-F066 | 附加对象信息 | `--add-extra-message` | 包含 DDL | 依赖 sys 权限 | ENABLED |
| EX-F067 | 保留 Schema | `--retain-schema` | 包含 DDL | 默认未设置 | ENABLED |
| EX-F068 | 紧凑 Schema | `--compact-schema` | 包含表 DDL | 可能缺信息；待 4.3.5 实测 | VALIDATION_GATED |
| EX-F069 | 序列策略 | `--sequence-policy` | 包含序列 | 默认值只作提示；条件待实测 | VALIDATION_GATED |

### 8.3 筛选与一致性（高级选项 · 功能选项 · 黑白名单筛选 / 错误处理）

| ID | 字段 | 参数 | 显示条件 | 互斥/依赖 | supportState |
|---|---|---|---|---|---|
| EX-F070 | 自定义查询 | `--query-sql` | 包含数据 + 专家 | 与 where/partition 互斥；不提供 SQL 编辑器 | ENABLED |
| EX-F071 | 条件筛选 | `--where` | 包含数据 | 与 query SQL 互斥 | VALIDATION_GATED |
| EX-F072 | 分区筛选 | `--partition` | 包含表数据 | 与 query SQL 互斥；校验二级分区 | VALIDATION_GATED |
| EX-F073 | 包含列 | `--include-column-names` | 包含表数据 | 校验实际列名 | ENABLED |
| EX-F074 | 排除列 | `--exclude-column-names` | 包含表数据 | 与控制文件互斥 | ENABLED |
| EX-F075 | 排除数据类型 | `--exclude-data-types` | 包含表数据 | 校验兼容模式和类型 | VALIDATION_GATED |
| EX-F076 | 排除生成列 | `--exclude-virtual-columns` | 包含表数据 | 默认未设置 | ENABLED |
| EX-F077 | 使用隐藏主键 | `--enable-hidden-pk` | 无主键表且版本/权限满足 | 由预检查决定可用性 | VALIDATION_GATED |
| EX-F078 | 闪回 SCN | `--flashback-scn` | 包含数据 | 与 query SQL 互斥 | ENABLED |
| EX-F079 | 闪回时间点 | `--flashback-timestamp` | 数据 + Oracle | 与 query SQL 互斥 | ENABLED |
| EX-F080 | 一致性快照 | `--snapshot` | 包含数据 | 与其他一致性参数组合待确认 | VALIDATION_GATED |
| EX-F081 | 备副本弱读 | `--weak-read` | 环境支持 | 预检查副本和权限 | VALIDATION_GATED |

## 9. 步骤 6：确认页

| ID | 区域 | 内容 | 规则 | supportState |
|---|---|---|---|---|
| EX-F082 | 配置摘要 | 数据源、对象、内容、格式、输出、节点和参数 | 只展示活动配置 | ENABLED |
| EX-F083 | 预检查 | 阻断错误、风险警告、普通提示 | 相关事实变化后失效 | ENABLED |
| EX-F084 | 完整命令 | 实际 OBDUMPER 命令 | 只读、分类展示、敏感值脱敏 | ENABLED |
| EX-F085 | 复制命令 | 默认复制脱敏命令 | 明文需要权限、确认和审计 | ENABLED |
| EX-F086 | 风险确认 | 生产、覆盖、数据变化等 | 相关字段变化后清除 | ENABLED |
| EX-F087 | 提交执行 | 提交任务 | 无阻断、风险已确认、命令生成成功 | ENABLED |

### 9.1 字段支持状态汇总

| supportState | 字段数量 | 占比 |
|---|---:|---:|
| ENABLED | 53 | 60.9% |
| VALIDATION_GATED | 33 | 37.9% |
| HIDDEN | 1 | 1.1% |
| BLOCKED | 0 | 0% |
| **合计** | **87** | **100%** |

## 10. 命令生成与预检查失效

命令顺序（括号内为官方选项分类）：

```text
程序与连接（基础选项 · 连接选项）
→ 数据库与连接环境（基础选项 · 连接选项）
→ 对象范围（基础选项 · 功能选项 · 数据库对象类型）
→ 内容与格式（基础选项 · 功能选项 · 文件格式）
→ 格式序列化（基础选项 · 功能选项 · 文件格式）
→ DDL / 筛选 / 一致性（高级选项 · 功能选项 · 时间戳格式 / 黑白名单筛选 / 错误处理）
→ 输出与文件（基础选项 · 功能选项 · 存储路径 / 压缩导出）
→ 性能与资源（高级选项 · 性能选项）
```

只生成活动且映射确定的字段；官方默认未覆盖时不生成；派生字段不得自由编辑；非活动、导入侧、废弃和 help-only 参数不得生成。命令预览、参数快照和实际执行必须来自同一份标准化配置。

| 变化 | 失效检查 | 清除确认 |
|---|---|---|
| 数据源、数据库、连接环境 | 连接、权限、对象、版本、命令 | 生产与账号风险 |
| 对象范围或表达式 | 对象、权限、多库、命令 | 对象风险 |
| 内容或格式 | 格式、专属参数、输出、命令 | 数据与格式风险 |
| 执行节点 | 工具版本、路径、资源、会话、命令 | 节点资源风险 |
| 输出位置或凭据 | 可达、写权限、空间、目录、命令 | 覆盖与敏感信息 |
| 筛选或一致性参数 | 表达式、权限、对象、命令 | 查询与一致性风险 |
| 性能、内存、压缩、切分 | 参数范围、节点资源、命令 | 资源风险 |
| 仅折叠/展开分组 | 无 | 无 |

预检查应绑定标准化配置指纹、工具版本、执行节点和检查时间。

## 11. 不进入新建导出表单

- CLI 元参数：`--help`、`--version`；
- 导入侧：`--mix`、`--file-suffix`、`--ignore-escape`、`--parallel`；
- 废弃：`--storage-uri`、`--file-name`、`--upload-behavior`、`--distinct`、`--column-delimiter`；
- help-only：`--commit-size`、`--server`、`--public-synonym`；
- 连接别名或派生参数：独立 `--tenant` 是否生成，以及 sys、云环境、逻辑库、会话配置参数由数据源、节点和安全配置决定。

## 12. 已确认字段专项规则

EX-FR01～EX-FR10 已于 2026-07-17 全部确认。确认的是默认继承、草稿保留、派生、清值、快照和风险处理规则；明确标记待实测的工具行为仍不视为已定版。

| 编号 | 议题 | 已确认方案 |
|---|---|---|
| EX-FR01 | 官方默认 | 未显式设置时只展示"官方默认"，命令不生成参数 |
| EX-FR02 | 非活动值 | 草稿可保留，命令和提交快照排除 |
| EX-FR03 | 格式切换 | 各格式分命名空间保存草稿值 |
| EX-FR04 | 硬冲突清除 | 列出影响并经确认后清除 |
| EX-FR05 | 连接环境 | public-cloud、no-sys、sys 凭据由数据源和安全配置派生 |
| EX-FR06 | 节点配置 | session-config 只读派生，向导不上传或编辑配置文件 |
| EX-FR07 | 逻辑库 | logical-database 由数据源类型派生，并提示结果不能直接导入 |
| EX-FR08 | 任务快照 | 保存活动值、派生来源、工具版本、最终命令及默认值版本依据 |
| EX-FR09 | 风险确认 | 事实变化后只清除对应确认 |
| EX-FR10 | POS | 可选择，但实测完成前不得通过最终命令验收 |

## 13. 验收基线

1. 每个字段可追溯到官方参数或平台调度字段。
2. 隐藏、非活动和不展示参数不进入命令。
3. 格式切换可恢复草稿值，提交只包含当前格式。
4. 官方默认、派生值和用户显式值可区分。
5. 数据源与节点派生参数不能通过普通表单绕过。
6. 显示、下一步、预检查和提交校验使用同一规则。
7. 影响执行事实的变化会使相关预检查失效。
8. POS、block-size 等冲突项在实测前不写成确定能力。
9. 每个字段具有 supportState 标注，VALIDATION_GATED 字段在受控实测完成前不参与最终命令生成。
