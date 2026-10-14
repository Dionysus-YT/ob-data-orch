# MySQL 本地导出真实端到端验证（2026-10-14）

本轮 172 项：152 PASS、12 FAIL、4 BLOCKED、2 PARTIAL、2 OBSERVED。全部场景已有明确结果，但导出模块整体不能声明全部通过。84 个通过项实际核对完整行集或 DDL 内容；通过总数还包含接口拒绝、预检查、生命周期和界面断言。

## 授权、环境与证据边界

用户明确授权非生产 `test-mysql`、创建并清理专用夹具、本地导出及正常浏览器登录。实际源为 OceanBase 4.4.2.3 / MySQL 兼容模式 / ODP，使用 Windows AMD64、Java 8 和 OBDUMPER 4.3.5-RELEASE。只读取本轮固定合成表，不读取业务表；测试辅助程序不进入产品 JDBC 或 SQL 接口。Oracle、对象存储、历史 POS/Parquet/ORC/Avro 实际导出、Linux 及真实并发压测不在本轮。

共创建 133 个真实任务（含修复前尝试及新目录复验）：104 SUCCEEDED、25 FAILED、4 CANCELLED；结束时无未完成任务或待核对 execution。这是任务次数，不是用例通过数。102 个代表性冻结命令已核对参数名，无预期参数遗漏；79 个目录定义的范围见[参数追踪](export-mysql-parameter-coverage.md)，不能把目录数量当作全部参数真实可用。

浏览器按用户指定使用内嵌会话，通过正常 API 执行草稿、唯一生成器、七项预检查、提交及 Agent。页面实际走完五步、真实对象目录、确认提交及成功详情。CSV/CUT/SQL 完整内容由独立校验器核对 NULL、空串、引用、Unicode、换行、重复行、列、字节和清单；压缩先解压再核对。多表和整库逐表校验，DDL 多对象逐类型核对，不能用单张 rich 表或退出码替代整库成功。

## 仍失败的 12 项

| 场景 | 真实结果与影响 |
| --- | --- |
| MX-001 默认日志目录 | 七项预检通过；工具先在输出目录建立 logs，随后因输出目录非空退出 1。其余场景使用独立日志目录，以继续验证各参数；默认配置缺陷仍存在。 |
| MX-004 整库 | 原尝试超时；最终 FAILED / PROCESS_WAIT_FAILED / exit 1，12 个文件、27,541,197 B、无检查点。部分文件存在不能证明整库完成。 |
| MX-005 整库排除 | FAILED / PROCESS_WAIT_FAILED；直接进程退出码为 0，10 个文件、1,065,487 B。完整成功证据未成立，根因尚未定位；不把此例写成非零退出或整库成功。 |
| MX-037-CUT、MX-037-SQL 空表保留 | 退出 0 但结果文件数为 0；Agent 按 RESULT_VERIFICATION_FAILED 失败关闭。CSV 空集合的真实空结果文件已独立验证。 |
| MX-062、MX-062-CUT 零日期保留 | 源端 CAST AS CHAR 确认零日期确实存在，参数已发射，文件值却为 NULL 标记；任务终态成功但内容断言失败。 |
| MX-075 retain-schema | 工具终态成功，实际 DDL 缺少期望的 Schema 限定；内容断言失败。 |
| MX-110-zstd、MX-150-zstd-1、MX-150-zstd-22 | FAILED / PROCESS_WAIT_FAILED / exit 1，压缩结果为零字节，根因尚未定位；不能声明 zstd 或其等级边界可用。 |
| MX-110-snappy | exit 1；日志明确出现 UnsatisfiedLinkError，java.library.path 中缺少 snappyjava。当前包和节点组合不满足 native 依赖。 |

## 阻断、部分验证与行为观察

- MX-072：该源固定隐藏主键列零行读取返回 1054，没有可读夹具，隐藏主键加速正例 BLOCKED；其资格负例不能替代正例。
- MX-073：源未配置 sys 凭据，附加定义信息正例 BLOCKED；未配置 sys 的拒绝负例已通过。
- MX-128：本轮真实失败任务均缺少有效 dump.ckpt，检查点继续正例 BLOCKED；无检查点继续的 422 负例已通过。
- MX-133：当前真实 Worker 未提供活跃进程重新附着入口，Agent 崩溃后的监管、租约与重复投递取证 BLOCKED；本轮安装时的闲置重启不算恢复正例。
- MX-131：真实 SSE 首次 5 条、断线续读 5 条且无重复，终态 SUCCEEDED；REST 78 条、1 页、PERSISTED_DOUBLE_REDACTED。日志下载已按 DEC-050 推迟，未观察到真实多页日志，整体 PARTIAL。
- MX-134：未登录、缺少 CSRF 和随机不存在对象检查已失败关闭；缺第二个已授权身份，跨主体验证 PARTIAL。
- MX-031：实际 trail-delimiter 去掉尾部分隔符，完整行内容核对后保留 OBSERVED；不据参数名字推断方向。MX-063：未开启 preserve-zero-datetime 时的零日期转换只记录 OBSERVED。

TIME/TIMESTAMP、DATE/DATETIME 与 PRESERVE/RESTART 序列策略已有真实文件断言。序列先实际推进，PRESERVE 导出下一值 29，RESTART 导出 17。SCN 正例通过 API 按精确 int64 传输；浏览器数值解析只接受安全整数，不能从 API 结果外推完整 int64 SCN 的页面输入/恢复可用。

## 测试中修复并复验的实现

1. 多行及超过 4096 字节的合法 SELECT 在草稿、领取、信封与启动间采用统一有界校验；原始 SELECT 60 KiB、包装后 64 KiB，其余参数保持既有限制和失败关闭。真实多行/长查询已通过。
2. 空文本结果以真实 .csv/.dat/.sql 文件、受控进程和固定成功终态共同判断；零字节实际文件允许通过，零文件、仅 MANIFEST/检查点、DDL 与二进制反例继续拒绝。真实 CSV 空集合已通过。
3. CSV NONE 引用模式必须显式提供转义字符；缺失组合在接口和页面拒绝，合法组合真实通过。
4. 重复列筛选在服务端拒绝，防止预检查之后才由工具报错。
5. PENDING 七项占位补齐 NODE_RESOURCE 固定 evidenceCode，严格浏览器解码不再中断自动预检查；页面完整自动报告已复验。
6. 草稿读取 API 解码补齐读取开关、日期/NLS、压缩等级及 DDL/序列字段，避免 GET→回填→PATCH 静默清值；真实页面刷新后 rev-3、指纹及 weakRead/TIME/TIMESTAMP 参数保持。Oracle 与序列读取有合成正反例，不外推 Oracle 真实验证。

固定夹具与脚本准备错误单独纠正后使用新目录复验：DDL_ONLY 不携带数据资源参数、默认 CSV 单引号、CRLF 字面转义、NONE 显式 escape、临时目录现行绑定，以及生命周期旧修订、正确 SSE 路径和派生空目录。max-file-size 按总量停止契约断言，32 个完整源行、33,225 B 对应 32,768 B 上限；block-size 独立断言拆分。历史检查保留原结论，但变更后的绑定不匹配、旧提交为 422；新检查通过。旧任务节点事实变化与本轮派生语义分开，用当前事实的真实失败种子验证冻结指纹。

## 实际检查、秘密与清理

- Go 全包 go test 与 go vet 通过；前端针对草稿 API/DTO/向导的 152 项测试、typecheck 与改动文件 ESLint 通过。
- E2E 校验器 10 项测试通过，含缺表、少行、未知对象及多对象 DDL 反例；脚本语法检查通过。
- audit:business 为 0 硬违规、9 REVIEW；audit:wizards 为 0 硬违规、4 REVIEW。API 只拥有传输与严格解码，表单/目录/事务保持原唯一所有者；现有跨模块节点标签与模板资格复用保持既有边界，未按行数另建框架。
- Secret scan 与 git diff --check 通过（换行转换提示不算检查失败）。
- 133 个任务的公共快照、脱敏命令与持久日志，以及 520 个本轮 native 日志文件，均未匹配真实密码明文、Base64 或 URL 编码；另现场核对 1 个运行中的 Java argv。原始响应只进入匿名 stdin 比对，不写入报告。
- 133 个 execution 的 security 与 tool-raw-log 均无残留。本次模板已删除，专用库按 owner 核验后 DROP 并确认不存在，专用输出目录按绝对路径和 owner 核验后删除；保留平台任务历史及脱敏断言/摘要。
- 清理首条混合脚本被策略拒绝后，拆分为只读路径核验及无 Force 的精确目录删除，两步成功；无越界清理。
- 本机控制面健康 200、Agent 心跳恢复、节点 ENABLED / NORMAL、环境检查 PASSED，活动 execution 为 0。当前构建 Agent 与内嵌 Windows Agent 包摘要一致；Java/工具及身份/TLS/状态目录继续复用。

Agent SHA-256：`0B158DDC3200E6E9C432DA4AFDC380D1D9F8F85E27FA0CC74B3263D2FF6E6C18`。控制面 SHA-256：`EF8CA7E615ED4BA7F30676A8DB9ABBA1C0D2A606B6D790B37C7BB7ECC3477839`。本机源码已构建并安装运行，不等于正式发布或其他目标认证。

## 逐项结果

用例输入及执行断言见[172 项矩阵](export-mysql-e2e-cases.md)，可机读的安全结果见[结果 JSON](export-mysql-e2e-2026-10-14-results.json)。完整回执、文件统计与截图保留在忽略的 artifacts/mysql-export-e2e；真实输出已清理，不能再次对旧目录运行内容校验，复验须创建新的 owner 夹具和运行目录。

| 用例 | 场景 | 结果 | 实际断言/限制 |
| --- | --- | --- | --- |
| MX-001 | 单表 CSV：真实预检、任务领取、终态与完整内容 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；PROCESS_EXIT_NONZERO |
| MX-001-LOG | 日志目录与输出目录分离的真实 CSV 主链路 | PASS | 4 行；2 文件 / 265 B |
| MX-002 | 相同业务参数在新输出目录的内容一致 | PASS | 4 行；2 文件 / 265 B；2 次独立命令/内容一致 |
| MX-003 | 多个表的数据导出 | PASS | 4 行；2 文件 / 265 B；2 表 |
| MX-004 | 独立夹具库整库导出 | FAIL | TASK_TIMEOUT |
| MX-005 | 整库排除指定表 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；PROCESS_WAIT_FAILED |
| MX-006 | 查询结果单条 SELECT 与列别名 | PASS | 2 行；2 文件 / 32 B |
| MX-006-MULTILINE | 正常多行 SELECT 经领取、启动与限行导出 | PASS | 2 行；2 文件 / 38 B |
| MX-006-LONG | 超过普通 4096 字节参数上限的合法 SELECT | PASS | 4 行；2 文件 / 58 B |
| MX-007 | 查询结果条数上限 | PASS | 2 行；2 文件 / 38 B |
| MX-008 | 查询结果空集合 | PASS | 0 行；2 文件 / 0 B |
| MX-009 | 查询 SQL 的执行错误由真实工具结束为失败 | PASS | PROCESS_EXIT_NONZERO |
| MX-010 | 指定表仅 DDL | PASS | 1 文件 / 495 B |
| MX-011-CSV | 表 DDL + CSV 数据 | PASS | 4 行；3 文件 / 760 B |
| MX-011-CUT | 表 DDL + CUT 数据 | PASS | 4 行；2 文件 / 674 B |
| MX-011-SQL | 表 DDL + SQL 数据 | PASS | 4 行；2 文件 / 1090 B |
| MX-012-VIEW | VIEW 的定义导出 | PASS | 1 文件 / 208 B |
| MX-012-FUNCTION | FUNCTION 的定义导出 | PASS | 1 文件 / 135 B |
| MX-012-PROCEDURE | PROCEDURE 的定义导出 | PASS | 1 文件 / 108 B |
| MX-012-SEQUENCE | SEQUENCE 的定义导出 | PASS | 1 文件 / 107 B |
| MX-013 | 表和视图的结构组合导出 | PASS | 2 文件 / 703 B |
| MX-014 | 非表纯数据内容不得进入任务 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-015 | 指定范围不允许空对象 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-016 | 对象类型白名单拒绝 TRIGGER | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-017 | 非生产测试不创建多语句 SQL 执行任务 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-018 | 非 SELECT 的结果集输入拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-019 | 结果集与显式对象互斥 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-020 | NULL、空字符串、逗号、引号、反斜杠、换行、Unicode 的精确内容 | PASS | 4 行；2 文件 / 265 B |
| MX-021 | 无列头 | PASS | 4 行；2 文件 / 200 B |
| MX-022 | 自定义分隔符与包围字符 | PASS | 4 行；2 文件 / 265 B |
| MX-023 | 全部字段引用模式 | PASS | 4 行；2 文件 / 283 B |
| MX-024 | 按需引用模式 | PASS | 4 行；2 文件 / 227 B |
| MX-025 | 非数字引用模式 | PASS | 4 行；2 文件 / 265 B |
| MX-025-ALL-NOT-NULL | 全部非空值引用模式 | PASS | 4 行；2 文件 / 281 B |
| MX-025-NONE | none 模式配合必要转义字符 | PASS | 4 行；2 文件 / 46 B |
| MX-026 | 自定义转义字符 | PASS | 4 行；2 文件 / 270 B |
| MX-027 | CRLF 行分隔符与字段内换行 | PASS | 4 行；2 文件 / 270 B |
| MX-028 | 自定义 NULL 标记与空字符串区分 | PASS | 4 行；2 文件 / 269 B |
| MX-029 | 去除字段首尾空白 | PASS | 4 行；2 文件 / 260 B |
| MX-030 | 定长分隔字符串与特殊值 | PASS | 4 行；1 文件 / 203 B |
| MX-031 | trail-delimiter 的实际方向与内容 | OBSERVED | 4 行；1 文件 / 175 B |
| MX-032 | 删除字段内换行 | PASS | 4 行；1 文件 / 176 B |
| MX-033 | INSERT SQL 的行数、字面值与转义 | PASS | 4 行；1 文件 / 595 B |
| MX-034 | SQL 行分隔符与编码 | PASS | 4 行；1 文件 / 595 B |
| MX-035-UTF-8 | UTF-8 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-035-GBK | GBK 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-035-GB2312 | GB2312 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-035-GB18030 | GB18030 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-035-Big5 | Big5 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-035-ISO-8859-1 | ISO-8859-1 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-035-UTF-16 | UTF-16 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 118 B |
| MX-035-UTF-32 | UTF-32 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 232 B |
| MX-035-US-ASCII | US-ASCII 的受控 ASCII 数据与实际字节解码 | PASS | 4 行；2 文件 / 58 B |
| MX-036-CSV | CSV 保留空文件 | PASS | 0 行；2 文件 / 0 B |
| MX-037-CSV | CSV 默认空表输出事实 | PASS | 0 行；1 文件 / 0 B |
| MX-036-CUT | CUT 保留空文件 | PASS | 0 行；1 文件 / 0 B |
| MX-037-CUT | CUT 默认空表输出事实 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；RESULT_VERIFICATION_FAILED |
| MX-036-SQL | SQL 保留空文件 | PASS | 0 行；1 文件 / 0 B |
| MX-037-SQL | SQL 默认空表输出事实 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；RESULT_VERIFICATION_FAILED |
| MX-040 | where 过滤产生精确子集 | PASS | 2 行；2 文件 / 153 B |
| MX-041 | 包含列产生精确列集合 | PASS | 4 行；2 文件 / 58 B |
| MX-042 | 排除列产生精确列集合 | PASS | 4 行；2 文件 / 189 B |
| MX-043 | 排除生成列 | PASS | 2 行；2 文件 / 25 B |
| MX-044 | 一级分区导出精确行集合 | PASS | 2 行；2 文件 / 31 B |
| MX-045 | 复合表仅子分区可导出 | PASS | 1 行；2 文件 / 25 B |
| MX-046 | 复合表父分区预检查失败 | PASS | submit 422 |
| MX-047 | 不存在的列预检失败，连接事实保留 | PASS | submit 422 |
| MX-048 | 列名大小写不一致预检失败 | PASS | submit 422 |
| MX-049 | 不存在的分区预检失败 | PASS | submit 422 |
| MX-050 | 不存在的表预检失败 | PASS | submit 422 |
| MX-051 | 排除数据类型 | PASS | 1 行；2 文件 / 30 B |
| MX-052 | 包含列与排除列互斥 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-053 | 重复列拒绝进入固定元数据检查 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-054 | where 与结果集查询互斥 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-055 | partition 与结果集查询互斥 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-056 | 列筛选通配符拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-060-dateValueFormat | dateValueFormat 的实际序列化 | PASS | 2 行；2 文件 / 142 B |
| MX-060-timeValueFormat | timeValueFormat 的实际序列化 | PASS | 2 行；2 文件 / 142 B |
| MX-060-datetimeValueFormat | datetimeValueFormat 的实际序列化 | PASS | 2 行；2 文件 / 142 B |
| MX-060-timestampValueFormat | timestampValueFormat 的实际序列化 | PASS | 2 行；2 文件 / 142 B |
| MX-061 | 所有 MySQL 值格式组合 | PASS | 2 行；2 文件 / 142 B |
| MX-061-CUT | CUT 的全部 MySQL 日期时间格式 | PASS | 2 行；1 文件 / 80 B |
| MX-062 | 保留零日期时间的实际值 | FAIL | OUTPUT_ROW_SET |
| MX-062-CUT | CUT 保留零日期时间 | FAIL | OUTPUT_ROW_SET |
| MX-063 | 默认零日期行为单独取证 | OBSERVED | 2 文件 / 39 B |
| MX-064-timestampTzValueFormat | MySQL 拒绝 Oracle 专属日期时间参数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-064-timestampLtzValueFormat | MySQL 拒绝 Oracle 专属日期时间参数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-064-nlsDateFormat | MySQL 拒绝 Oracle 专属日期时间参数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-064-nlsTimestampFormat | MySQL 拒绝 Oracle 专属日期时间参数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-064-nlsTimestampTzFormat | MySQL 拒绝 Oracle 专属日期时间参数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-065 | 日期格式中的控制字符拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-066 | 仅 DDL 不接受日期值格式 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-070 | 快照一致性读取 | PASS | 4 行；2 文件 / 265 B |
| MX-071 | 弱读固定预检查及真实输出 | PASS | 4 行；2 文件 / 265 B |
| MX-072 | 无主键表隐藏主键读取 | BLOCKED | FIXTURE_HIDDEN_PK_READABLE_UNAVAILABLE |
| MX-073 | 附加表定义信息与 sys 槽位 | BLOCKED | FIXTURE_SYS_UNAVAILABLE |
| MX-073-NO-SYS | 缺少 sys 凭据时附加定义信息失败关闭 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-074 | 前置 DROP 的定义输出 | PASS | 1 文件 / 524 B |
| MX-075 | 保留 Schema 前缀的定义 | FAIL | DDL_SCHEMA_MISSING |
| MX-076 | 紧凑原生建表定义 | PASS | 1 文件 / 495 B |
| MX-077-PRESERVE | 序列策略 PRESERVE 的定义结果 | PASS | 1 文件 / 107 B |
| MX-077-RESTART | 序列策略 RESTART 的定义结果 | PASS | 1 文件 / 107 B |
| MX-078 | 指定 SCN 快照 | PASS | 4 行；2 文件 / 265 B |
| MX-079 | 快照与闪回 SCN 互斥 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-080 | MySQL 不接受 Oracle 闪回时间 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-081 | 结果集不接受隐藏主键读取 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-082 | 普通表不得声明序列策略 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-090 | 自定义节点本地日志目录 | PASS | 4 行；2 文件 / 265 B |
| MX-091 | 输出不生成多级目录 | PASS | 4 行；2 文件 / 265 B |
| MX-092 | 按行拆分与全部文件内容合并校验 | PASS | 1024 行；5 文件 / 1064902 B |
| MX-093 | 按 MB 拆分 | PASS | 1024 行；3 文件 / 1064848 B |
| MX-094 | 进程导出总量上限允许停止且不冒充拆分 | PASS | 32 行；2 文件 / 33225 B |
| MX-095 | 线程、分页与 JVM 堆的实际启动 | PASS | 4 行；2 文件 / 265 B |
| MX-096 | 每线程宏块数 | PASS | 4 行；2 文件 / 265 B |
| MX-097 | 静态额度不足不压掉数据库结果 | PASS | submit 422 |
| MX-098 | 节点根目录外的输出被阻断 | PASS | submit 422 |
| MX-099 | 合法输出与非法日志目录分别取证 | PASS | submit 422 |
| MX-100 | 非空目录失败，数据库结果独立通过 | PASS | submit 422 |
| MX-101 | 仅跳过专用目录空性 | PASS | 4 行；3 文件 / 285 B |
| MX-102 | 节点平台与路径形态不匹配 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-103 | 线程非正整数拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-104 | 非法 JVM 单位拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-105 | MySQL 拒绝 Oracle fetch-size | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-106 | 无效文件拆分单位拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-110-zstd | zstd 导出及解压内容验证 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；PROCESS_WAIT_FAILED |
| MX-110-zlib | zlib 导出及解压内容验证 | PASS | 4 行；2 文件 / 214 B |
| MX-110-gzip | gzip 导出及解压内容验证 | PASS | 4 行；2 文件 / 226 B |
| MX-110-snappy | snappy 导出及解压内容验证 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；PROCESS_EXIT_NONZERO |
| MX-111 | 压缩算法白名单拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-112 | 压缩等级越界拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-113 | gzip 不接受压缩等级 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-114 | CSV 分隔符只接受单字符 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-115 | CSV 包围符只接受单字符 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-116 | CSV 转义符只接受单字节 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-117 | NULL 标记控制字符拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-118 | SQL 不接受 CSV 专属参数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-118-NONE-ESCAPE | NONE 包围模式缺少必要转义字符拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-119-POS | POS 普通 V1 界面保持门控 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-119-PARQUET | PARQUET 普通 V1 界面保持门控 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-119-ORC | ORC 普通 V1 界面保持门控 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-119-AVRO | AVRO 普通 V1 界面保持门控 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-120 | 五步页面与真实对象目录 | PASS | 4 行；2 文件 / 265 B |
| MX-121 | 修改配置使旧预检失效 | PASS | submit 422 |
| MX-122 | 保存草稿、预检、提交各自的幂等 | PASS | 4 行；2 文件 / 265 B |
| MX-123 | 过期预检不能提交 | PASS | submit 422 |
| MX-124 | 重复点击提交不创建第二任务 | PASS | 4 行；2 文件 / 265 B |
| MX-125 | 排队取消与一致终态 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-126 | 运行取消与受控进程终止 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-127 | 无检查点失败任务拒绝继续 | PASS | HTTP 422 |
| MX-128 | 有真实检查点的失败任务继续 | BLOCKED | REAL_FAILED_CHECKPOINT_UNAVAILABLE |
| MX-129 | 按原配置新建与从头重跑 | PASS | 4 行；2 文件 / 265 B |
| MX-130 | 成功任务保存模板再建草稿 | PASS | 4 行；2 文件 / 265 B |
| MX-131 | 实时日志、下载与两层脱敏 | PARTIAL | LOG_DOWNLOAD_NOT_IMPLEMENTED |
| MX-132 | 秘密材料与进程结束后清理 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-133 | Agent 断联恢复与至少一次投递 | BLOCKED | ACTIVE_PROCESS_REATTACH_ENTRY_UNAVAILABLE |
| MX-134 | 未登录/CSRF/越权 API 失败关闭 | PARTIAL | SECOND_AUTHORIZED_IDENTITY_REQUIRED |
| MX-135 | 四档视口、键盘、完整命令和统一摘要 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-136 | 保存草稿刷新后读取开关与日期格式不丢失 | PASS | 该项完整断言见矩阵与结果 JSON |
| MX-140 | 结果集条数上限非正数 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-141 | 结果集 SQL 文件引用拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-142 | 结果集不接受列筛选 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-143 | 仅 DDL 不接受读取选项 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-144 | SQL 不接受日期值格式 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-145 | 普通文本格式不接受 POS 控制目录 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-147 | 分页非正整数拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-148 | 文件阈值非正整数拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-149 | 压缩算法未开启压缩时拒绝 | PASS | EXPORT_DRAFT_REJECTED；HTTP 422 |
| MX-146 | 现行元数据允许本地格式携带节点临时目录 | PASS | 4 行；2 文件 / 265 B |
| MX-150-zstd-1 | zstd 压缩等级边界 1 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；PROCESS_WAIT_FAILED |
| MX-150-zstd-22 | zstd 压缩等级边界 22 | FAIL | EXPECTED_TASK_SUCCESS_MISSING；PROCESS_WAIT_FAILED |
| MX-150-zlib--1 | zlib 压缩等级边界 -1 | PASS | 4 行；2 文件 / 60 B |
| MX-150-zlib-9 | zlib 压缩等级边界 9 | PASS | 4 行；2 文件 / 60 B |

本轮仅认证上表已经通过的具体组合；失败、阻断、部分验证与行为观察不会因页面可选或参数已发射而提升为已支持。
