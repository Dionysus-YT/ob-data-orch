# MySQL 本地导出端到端测试矩阵

共 172 个独立场景。用例定义由 scripts/export-e2e/cases.mjs 驱动；此文件由 matrix.mjs 生成。真实结果单独记录，矩阵存在不表示已通过。

通用前提：用户授权的非生产 test-mysql、Windows AMD64 当前 Agent/控制面、独立 owner 夹具库和允许根目录下专用输出。CSV/CUT/SQL 参数通过真实 API→唯一生成器→七项预检→Agent→官方工具→结果验证。每个场景串行，危险 SQL 负例止于草稿接口。默认日志行为单独测试，其余参数场景使用独立日志目录。

| 用例 | 分类/场景 | 输入变化 | 执行与预期 | 条件 |
| --- | --- | --- | --- | --- |
| MX-001 | 主链路：单表 CSV：真实预检、任务领取、终态与完整内容 | {} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-001-LOG | 主链路：日志目录与输出目录分离的真实 CSV 主链路 | {"logPath":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-002 | 主链路：相同业务参数在新输出目录的内容一致 | {} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-003 | 范围：多个表的数据导出 | {"tables":["rich","empty"]} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-004 | 范围：独立夹具库整库导出 | {"scope":"ALL"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-005 | 范围：整库排除指定表 | {"scope":"ALL","excludeTables":["empty","zeros","slow"]} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-006 | 范围：查询结果单条 SELECT 与列别名 | {"query":"SELECT id, ascii_label AS label FROM {database}.rich WHERE id <= 2 ORDER BY id"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-006-MULTILINE | 范围：正常多行 SELECT 经领取、启动与限行导出 | {"query":"SELECT\n id, ascii_label\nFROM {database}.rich\nWHERE id <= 2\nORDER BY id"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-006-LONG | 范围：超过普通 4096 字节参数上限的合法 SELECT | {"query":"SELECT                                                                          …（4555 字符合法 SELECT）"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-007 | 范围：查询结果条数上限 | {"query":"SELECT id, ascii_label FROM {database}.rich ORDER BY id","queryLimit":2} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-008 | 范围：查询结果空集合 | {"query":"SELECT id, ascii_label FROM {database}.rich WHERE id < 0","retainEmptyFiles":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-009 | 范围：查询 SQL 的执行错误由真实工具结束为失败 | {"query":"SELECT missing_column FROM {database}.rich"} | 七项通过 → 真实启动 → 非零退出和指定 SQL 错误日志；不得错判成功 | 通用前提 |
| MX-010 | 范围：指定表仅 DDL | {"content":"DDL_ONLY"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-011-CSV | 范围：表 DDL + CSV 数据 | {"content":"DDL_AND_DATA","format":"CSV"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-011-CUT | 范围：表 DDL + CUT 数据 | {"content":"DDL_AND_DATA","format":"CUT"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-011-SQL | 范围：表 DDL + SQL 数据 | {"content":"DDL_AND_DATA","format":"SQL"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-012-VIEW | 对象类型：VIEW 的定义导出 | {"objectType":"VIEW","object":"v_rich","content":"DDL_ONLY"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | VIEW |
| MX-012-FUNCTION | 对象类型：FUNCTION 的定义导出 | {"objectType":"FUNCTION","object":"f_identity","content":"DDL_ONLY"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | FUNCTION |
| MX-012-PROCEDURE | 对象类型：PROCEDURE 的定义导出 | {"objectType":"PROCEDURE","object":"p_identity","content":"DDL_ONLY"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | PROCEDURE |
| MX-012-SEQUENCE | 对象类型：SEQUENCE 的定义导出 | {"objectType":"SEQUENCE","object":"s_counter","content":"DDL_ONLY"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | SEQUENCE |
| MX-013 | 对象类型：表和视图的结构组合导出 | {"objects":[["TABLE","rich"],["VIEW","v_rich"]],"content":"DDL_ONLY"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | VIEW |
| MX-014 | 输入与能力边界：非表纯数据内容不得进入任务 | {"objectType":"VIEW","object":"v_rich"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-015 | 输入与能力边界：指定范围不允许空对象 | {"tables":[]} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-016 | 输入与能力边界：对象类型白名单拒绝 TRIGGER | {"objectType":"TRIGGER"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-017 | 输入与能力边界：非生产测试不创建多语句 SQL 执行任务 | {"query":"SELECT 1; DELETE FROM {database}.rich"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-018 | 输入与能力边界：非 SELECT 的结果集输入拒绝 | {"query":"DELETE FROM {database}.rich"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-019 | 输入与能力边界：结果集与显式对象互斥 | {"query":"SELECT 1","forceObjects":true} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-020 | CSV：NULL、空字符串、逗号、引号、反斜杠、换行、Unicode 的精确内容 | {} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-021 | CSV：无列头 | {"csv":{"skipHeader":true}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-022 | CSV：自定义分隔符与包围字符 | {"csv":{"columnSeparator":"\|","columnQuote":"'"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-023 | CSV：全部字段引用模式 | {"csv":{"columnQuoteMode":"all"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-024 | CSV：按需引用模式 | {"csv":{"columnQuoteMode":"minimal"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-025 | CSV：非数字引用模式 | {"csv":{"columnQuoteMode":"non_numeric"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-025-ALL-NOT-NULL | CSV：全部非空值引用模式 | {"csv":{"columnQuoteMode":"all_not_null"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-025-NONE | CSV：none 模式配合必要转义字符 | {"include":["id","ascii_label"],"csv":{"columnQuoteMode":"none","escapeCharacter":"\\"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-026 | CSV：自定义转义字符 | {"csv":{"escapeCharacter":"\\"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-027 | CSV：CRLF 行分隔符与字段内换行 | {"csv":{"lineSeparator":"\\r\\n"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-028 | CSV：自定义 NULL 标记与空字符串区分 | {"csv":{"nullString":"[NULL]"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-029 | CSV：去除字段首尾空白 | {"csv":{"withTrim":true}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-030 | CUT：定长分隔字符串与特殊值 | {"format":"CUT","cut":{"columnSplitter":"\|\|"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-031 | CUT：trail-delimiter 的实际方向与内容 | {"format":"CUT","cut":{"columnSplitter":"\|","trailDelimiter":true}} | 记录工具终态与实际数据/文件行为；不外推为推荐组合或 PASS | 通用前提 |
| MX-032 | CUT：删除字段内换行 | {"format":"CUT","cut":{"columnSplitter":"\|","removeNewline":true}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-033 | SQL：INSERT SQL 的行数、字面值与转义 | {"format":"SQL"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-034 | SQL：SQL 行分隔符与编码 | {"format":"SQL","csv":{"lineSeparator":"\\r\\n","fileEncoding":"UTF-8"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-UTF-8 | 编码：UTF-8 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"UTF-8"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-GBK | 编码：GBK 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"GBK"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-GB2312 | 编码：GB2312 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"GB2312"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-GB18030 | 编码：GB18030 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"GB18030"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-Big5 | 编码：Big5 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"Big5"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-ISO-8859-1 | 编码：ISO-8859-1 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"ISO-8859-1"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-UTF-16 | 编码：UTF-16 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"UTF-16"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-UTF-32 | 编码：UTF-32 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"UTF-32"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-035-US-ASCII | 编码：US-ASCII 的受控 ASCII 数据与实际字节解码 | {"include":["id","ascii_label"],"csv":{"fileEncoding":"US-ASCII"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-036-CSV | 空表：CSV 保留空文件 | {"format":"CSV","table":"empty","retainEmptyFiles":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-037-CSV | 空表：CSV 默认空表输出事实 | {"format":"CSV","table":"empty"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-036-CUT | 空表：CUT 保留空文件 | {"format":"CUT","table":"empty","retainEmptyFiles":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-037-CUT | 空表：CUT 默认空表输出事实 | {"format":"CUT","table":"empty"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-036-SQL | 空表：SQL 保留空文件 | {"format":"SQL","table":"empty","retainEmptyFiles":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-037-SQL | 空表：SQL 默认空表输出事实 | {"format":"SQL","table":"empty"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-040 | 筛选：where 过滤产生精确子集 | {"where":"id <= 2"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-041 | 筛选：包含列产生精确列集合 | {"include":["id","ascii_label"]} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-042 | 筛选：排除列产生精确列集合 | {"exclude":["notes"]} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-043 | 筛选：排除生成列 | {"table":"generated_values","excludeVirtualColumns":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | GENERATED |
| MX-044 | 筛选：一级分区导出精确行集合 | {"table":"partitioned","partition":"p0"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | PARTITION |
| MX-045 | 筛选：复合表仅子分区可导出 | {"table":"subpartitioned","partition":"sp00"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | SUBPARTITION |
| MX-046 | 筛选：复合表父分区预检查失败 | {"table":"subpartitioned","partition":"p0"} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OBJECT_ACCESS 失败 | SUBPARTITION |
| MX-047 | 筛选：不存在的列预检失败，连接事实保留 | {"include":["missing_column"]} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OBJECT_ACCESS 失败 | 通用前提 |
| MX-048 | 筛选：列名大小写不一致预检失败 | {"include":["ASCII_LABEL"]} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OBJECT_ACCESS 失败 | 通用前提 |
| MX-049 | 筛选：不存在的分区预检失败 | {"table":"partitioned","partition":"missing_partition"} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OBJECT_ACCESS 失败 | PARTITION |
| MX-050 | 筛选：不存在的表预检失败 | {"table":"missing_table"} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OBJECT_ACCESS 失败 | 通用前提 |
| MX-051 | 筛选：排除数据类型 | {"table":"types","excludeDataTypes":["BLOB"]} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-052 | 输入与能力边界：包含列与排除列互斥 | {"include":["id"],"exclude":["ascii_label"]} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-053 | 输入与能力边界：重复列拒绝进入固定元数据检查 | {"include":["id","id"]} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-054 | 输入与能力边界：where 与结果集查询互斥 | {"query":"SELECT id FROM {database}.rich","where":"id > 0"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-055 | 输入与能力边界：partition 与结果集查询互斥 | {"query":"SELECT id FROM {database}.rich","partition":"p0"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-056 | 输入与能力边界：列筛选通配符拒绝 | {"include":["*"]} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-060-dateValueFormat | 日期时间：dateValueFormat 的实际序列化 | {"table":"temporal","timestampFormats":{"dateValueFormat":"yyyy/MM/dd"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-060-timeValueFormat | 日期时间：timeValueFormat 的实际序列化 | {"table":"temporal","timestampFormats":{"timeValueFormat":"HH-mm-ss"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-060-datetimeValueFormat | 日期时间：datetimeValueFormat 的实际序列化 | {"table":"temporal","timestampFormats":{"datetimeValueFormat":"yyyy/MM/dd HH:mm:ss"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-060-timestampValueFormat | 日期时间：timestampValueFormat 的实际序列化 | {"table":"temporal","timestampFormats":{"timestampValueFormat":"yyyy/MM/dd HH:mm:ss"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-061 | 日期时间：所有 MySQL 值格式组合 | {"table":"temporal","timestampFormats":{"dateValueFormat":"yyyy/MM/dd","timeValueFormat":"HH-mm-ss","datetimeValueFormat":"yyyy/MM/dd HH:mm:ss","timestampValueFormat":"yyyy/MM/dd HH:mm:ss"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-061-CUT | 日期时间：CUT 的全部 MySQL 日期时间格式 | {"table":"temporal","format":"CUT","timestampFormats":{"dateValueFormat":"yyyy/MM/dd","timeValueFormat":"HH-mm-ss","datetimeValueFormat":"yyyy/MM/dd HH:mm:ss","timestampValueFormat":"yyyy/MM/dd HH:mm:ss"}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-062 | 日期时间：保留零日期时间的实际值 | {"table":"zeros","timestampFormats":{"preserveZeroDatetime":true}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | ZERO_DATETIME |
| MX-062-CUT | 日期时间：CUT 保留零日期时间 | {"table":"zeros","format":"CUT","timestampFormats":{"preserveZeroDatetime":true}} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | ZERO_DATETIME |
| MX-063 | 日期时间：默认零日期行为单独取证 | {"table":"zeros"} | 记录工具终态与实际数据/文件行为；不外推为推荐组合或 PASS | ZERO_DATETIME |
| MX-064-timestampTzValueFormat | 输入与能力边界：MySQL 拒绝 Oracle 专属日期时间参数 | {"timestampFormats":{"timestampTzValueFormat":"YYYY-MM-DD"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-064-timestampLtzValueFormat | 输入与能力边界：MySQL 拒绝 Oracle 专属日期时间参数 | {"timestampFormats":{"timestampLtzValueFormat":"YYYY-MM-DD"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-064-nlsDateFormat | 输入与能力边界：MySQL 拒绝 Oracle 专属日期时间参数 | {"timestampFormats":{"nlsDateFormat":"YYYY-MM-DD"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-064-nlsTimestampFormat | 输入与能力边界：MySQL 拒绝 Oracle 专属日期时间参数 | {"timestampFormats":{"nlsTimestampFormat":"YYYY-MM-DD"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-064-nlsTimestampTzFormat | 输入与能力边界：MySQL 拒绝 Oracle 专属日期时间参数 | {"timestampFormats":{"nlsTimestampTzFormat":"YYYY-MM-DD"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-065 | 输入与能力边界：日期格式中的控制字符拒绝 | {"timestampFormats":{"dateValueFormat":"yyyy\nMM"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-066 | 输入与能力边界：仅 DDL 不接受日期值格式 | {"content":"DDL_ONLY","timestampFormats":{"dateValueFormat":"yyyy-MM-dd"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-070 | 读取与结构：快照一致性读取 | {"snapshot":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-071 | 读取与结构：弱读固定预检查及真实输出 | {"weakRead":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-072 | 读取与结构：无主键表隐藏主键读取 | {"table":"nopk","enableHiddenPk":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | HIDDEN_PK_READABLE |
| MX-073 | 读取与结构：附加表定义信息与 sys 槽位 | {"content":"DDL_ONLY","addExtraMessage":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | SYS |
| MX-073-NO-SYS | 读取与结构：缺少 sys 凭据时附加定义信息失败关闭 | {"content":"DDL_ONLY","addExtraMessage":true} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | NO_SYS |
| MX-074 | 读取与结构：前置 DROP 的定义输出 | {"content":"DDL_ONLY","dropObject":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-075 | 读取与结构：保留 Schema 前缀的定义 | {"content":"DDL_ONLY","retainSchema":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-076 | 读取与结构：紧凑原生建表定义 | {"content":"DDL_ONLY","compactSchema":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-077-PRESERVE | 读取与结构：序列策略 PRESERVE 的定义结果 | {"objectType":"SEQUENCE","object":"s_counter","content":"DDL_ONLY","sequencePolicy":"PRESERVE"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | SEQUENCE |
| MX-077-RESTART | 读取与结构：序列策略 RESTART 的定义结果 | {"objectType":"SEQUENCE","object":"s_counter","content":"DDL_ONLY","sequencePolicy":"RESTART"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | SEQUENCE |
| MX-078 | 读取与结构：指定 SCN 快照 | {"flashbackScn":"{scn}"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | SCN |
| MX-079 | 输入与能力边界：快照与闪回 SCN 互斥 | {"snapshot":true,"flashbackScn":"1"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-080 | 输入与能力边界：MySQL 不接受 Oracle 闪回时间 | {"flashbackTimestamp":"2026-01-01 00:00:00"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-081 | 输入与能力边界：结果集不接受隐藏主键读取 | {"query":"SELECT id FROM {database}.rich","enableHiddenPk":true} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-082 | 输入与能力边界：普通表不得声明序列策略 | {"content":"DDL_ONLY","sequencePolicy":"RESTART"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-090 | 文件与资源：自定义节点本地日志目录 | {"logPath":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-091 | 文件与资源：输出不生成多级目录 | {"noNestedDir":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-092 | 文件与资源：按行拆分与全部文件内容合并校验 | {"table":"bulk","blockSize":"256ROW"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-093 | 文件与资源：按 MB 拆分 | {"table":"bulk","blockSize":"1MB"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-094 | 文件与资源：进程导出总量上限允许停止且不冒充拆分 | {"table":"bulk","maxFileSize":32768} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-095 | 文件与资源：线程、分页与 JVM 堆的实际启动 | {"thread":2,"pageSize":100,"jvmMemory":"512M"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-096 | 文件与资源：每线程宏块数 | {"parallelMacro":1} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-097 | 文件与资源：静态额度不足不压掉数据库结果 | {"jvmMemory":"2048G"} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；NODE_RESOURCE 失败 | 通用前提 |
| MX-098 | 文件与资源：节点根目录外的输出被阻断 | {"outsideRoot":true} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OUTPUT_PATH 失败 | 通用前提 |
| MX-099 | 文件与资源：合法输出与非法日志目录分别取证 | {"outsideLogRoot":true} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OUTPUT_PATH 失败 | 通用前提 |
| MX-100 | 文件与资源：非空目录失败，数据库结果独立通过 | {"nonempty":true} | 完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝；OUTPUT_EMPTY 失败 | 通用前提 |
| MX-101 | 文件与资源：仅跳过专用目录空性 | {"nonempty":true,"skipCheckDir":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-102 | 输入与能力边界：节点平台与路径形态不匹配 | {"invalidPlatformPath":true} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-103 | 输入与能力边界：线程非正整数拒绝 | {"thread":0} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-104 | 输入与能力边界：非法 JVM 单位拒绝 | {"jvmMemory":"4XYZ"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-105 | 输入与能力边界：MySQL 拒绝 Oracle fetch-size | {"fetchSize":10} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-106 | 输入与能力边界：无效文件拆分单位拒绝 | {"blockSize":"1GB"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-110-zstd | 压缩：zstd 导出及解压内容验证 | {"compress":true,"compressionAlgo":"zstd","compressionLevel":3} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-110-zlib | 压缩：zlib 导出及解压内容验证 | {"compress":true,"compressionAlgo":"zlib","compressionLevel":6} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-110-gzip | 压缩：gzip 导出及解压内容验证 | {"compress":true,"compressionAlgo":"gzip"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-110-snappy | 压缩：snappy 导出及解压内容验证 | {"compress":true,"compressionAlgo":"snappy"} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-111 | 输入与能力边界：压缩算法白名单拒绝 | {"compress":true,"compressionAlgo":"unsupported"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-112 | 输入与能力边界：压缩等级越界拒绝 | {"compress":true,"compressionAlgo":"gzip","compressionLevel":99} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-113 | 输入与能力边界：gzip 不接受压缩等级 | {"compress":true,"compressionAlgo":"gzip","compressionLevel":6} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-114 | 输入与能力边界：CSV 分隔符只接受单字符 | {"csv":{"columnSeparator":"\|\|"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-115 | 输入与能力边界：CSV 包围符只接受单字符 | {"csv":{"columnQuote":"''"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-116 | 输入与能力边界：CSV 转义符只接受单字节 | {"csv":{"escapeCharacter":"xx"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-117 | 输入与能力边界：NULL 标记控制字符拒绝 | {"csv":{"nullString":"NULL\n"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-118 | 输入与能力边界：SQL 不接受 CSV 专属参数 | {"format":"SQL","csv":{"skipHeader":true}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-118-NONE-ESCAPE | 输入与能力边界：NONE 包围模式缺少必要转义字符拒绝 | {"csv":{"columnQuoteMode":"none"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-119-POS | 能力门禁：POS 普通 V1 界面保持门控 | {"format":"POS"} | 普通向导只提供 CSV/CUT/SQL；历史格式不作为当前新建入口，不启动门控格式 | 通用前提 |
| MX-119-PARQUET | 能力门禁：PARQUET 普通 V1 界面保持门控 | {"format":"PARQUET"} | 普通向导只提供 CSV/CUT/SQL；历史格式不作为当前新建入口，不启动门控格式 | 通用前提 |
| MX-119-ORC | 能力门禁：ORC 普通 V1 界面保持门控 | {"format":"ORC"} | 普通向导只提供 CSV/CUT/SQL；历史格式不作为当前新建入口，不启动门控格式 | 通用前提 |
| MX-119-AVRO | 能力门禁：AVRO 普通 V1 界面保持门控 | {"format":"AVRO"} | 普通向导只提供 CSV/CUT/SQL；历史格式不作为当前新建入口，不启动门控格式 | 通用前提 |
| MX-120 | 生命周期与安全：五步页面与真实对象目录 | {} | 五步正常导航、真实对象目录、保存、自动预检、确认提交、详情终态及内容 | 通用前提 |
| MX-121 | 生命周期与安全：修改配置使旧预检失效 | {} | 保存后编辑线程；修订+1/指纹变化；旧预检绑定不匹配、旧提交拒绝；新预检通过 | 通用前提 |
| MX-122 | 生命周期与安全：保存草稿、预检、提交各自的幂等 | {} | 创建/预检/提交同键同请求重放得到同 ID；同键不同载荷 409；乐观锁拒绝旧修订 | 通用前提 |
| MX-123 | 生命周期与安全：过期预检不能提交 | {} | 等待真实 validUntil，不改数据库/系统时间；过期提交 409/422 | 通用前提 |
| MX-124 | 生命周期与安全：重复点击提交不创建第二任务 | {} | 同提交幂等键重复请求只返回同一任务；终态、内容和源配置冻结一致 | 通用前提 |
| MX-125 | 生命周期与安全：排队取消与一致终态 | {} | 观察 WAITING_SCHEDULE 且未启动；取消为 CANCELLED；重复取消回执稳定 | 通用前提 |
| MX-126 | 生命周期与安全：运行取消与受控进程终止 | {} | 观察真实 RUNNING/进程证据；取消受控进程树；终态 CANCELLED、无待核对 | 通用前提 |
| MX-127 | 生命周期与安全：无检查点失败任务拒绝继续 | {} | 真实失败且无 dump.ckpt 的任务继续请求被拒绝 | 通用前提 |
| MX-128 | 生命周期与安全：有真实检查点的失败任务继续 | {} | 真实失败/检查点/原完整预检；继续任务继承快照并追加 --retry；不重复数据且父子关系正确 | 通用前提 |
| MX-129 | 生命周期与安全：按原配置新建与从头重跑 | {} | 真实失败任务派生；按原配置新建可编辑并重新预检；从头重跑保持指纹，改配置提交被拒绝 | 通用前提 |
| MX-130 | 生命周期与安全：成功任务保存模板再建草稿 | {} | 成功任务保存→列表→改名→创建草稿→更换专用输出→新预检/执行→删除本次模板 | 通用前提 |
| MX-131 | 生命周期与安全：实时日志、下载与两层脱敏 | {} | 真实运行期间 SSE；来源内序号、断线续读、REST 分页、两层脱敏；下载未实现单列阻断 | 通用前提 |
| MX-132 | 生命周期与安全：秘密材料与进程结束后清理 | {} | 核对真实密码及其可恢复编码不进入 argv/响应/快照/日志；对应 execution 安全文件与原始日志清理 | 通用前提 |
| MX-133 | 生命周期与安全：Agent 断联恢复与至少一次投递 | {} | 独占本轮任务窗口；保留身份/TLS重启 Agent；记录租约、序号、重复投递和单进程结果 | 通用前提 |
| MX-134 | 生命周期与安全：未登录/CSRF/越权 API 失败关闭 | {} | 未登录/缺失 CSRF/随机对象失败关闭；第二个已授权身份验证跨主体无权对象 404 | 通用前提 |
| MX-135 | 生命周期与安全：四档视口、键盘、完整命令和统一摘要 | {} | 1920/1440/1280/720 视口；键盘、焦点还原、无整页横向溢出、唯一摘要和常驻完整命令 | 通用前提 |
| MX-136 | 生命周期与安全：保存草稿刷新后读取开关与日期格式不丢失 | {} | 页面保存读取开关与 TIME/TIMESTAMP 格式，刷新后修订及指纹保持；命令参数仍存在 | 通用前提 |
| MX-140 | 输入与能力边界：结果集条数上限非正数 | {"query":"SELECT id FROM {database}.rich","queryLimit":0} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-141 | 输入与能力边界：结果集 SQL 文件引用拒绝 | {"query":"file:///fixture.sql"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-142 | 输入与能力边界：结果集不接受列筛选 | {"query":"SELECT id FROM {database}.rich","include":["id"]} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-143 | 输入与能力边界：仅 DDL 不接受读取选项 | {"content":"DDL_ONLY","weakRead":true} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-144 | 输入与能力边界：SQL 不接受日期值格式 | {"format":"SQL","timestampFormats":{"timeValueFormat":"HH:mm:ss"}} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-145 | 输入与能力边界：普通文本格式不接受 POS 控制目录 | {"controlFilePath":true} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-147 | 输入与能力边界：分页非正整数拒绝 | {"pageSize":0} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-148 | 输入与能力边界：文件阈值非正整数拒绝 | {"maxFileSize":0} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-149 | 输入与能力边界：压缩算法未开启压缩时拒绝 | {"compressionAlgo":"gzip"} | 创建请求得到 400/422；预检查、提交、工具执行均为零 | 通用前提 |
| MX-146 | 文件与资源：现行元数据允许本地格式携带节点临时目录 | {"tmpPath":true} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-150-zstd-1 | 压缩：zstd 压缩等级边界 1 | {"include":["id","ascii_label"],"compress":true,"compressionAlgo":"zstd","compressionLevel":1} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-150-zstd-22 | 压缩：zstd 压缩等级边界 22 | {"include":["id","ascii_label"],"compress":true,"compressionAlgo":"zstd","compressionLevel":22} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-150-zlib--1 | 压缩：zlib 压缩等级边界 -1 | {"include":["id","ascii_label"],"compress":true,"compressionAlgo":"zlib","compressionLevel":-1} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |
| MX-150-zlib-9 | 压缩：zlib 压缩等级边界 9 | {"include":["id","ascii_label"],"compress":true,"compressionAlgo":"zlib","compressionLevel":9} | 创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容 | 通用前提 |

判定：PASS 必须完成该项全部断言；FAIL 是已执行且不符合预期；BLOCKED 为缺少具体前提；PARTIAL 为只有部分断言完成；OBSERVED 只记录未定版语义。阻断和部分完成不计入通过数。真实检查点、sys 正例、跨主体身份和活跃租约恢复缺失时保留明确条件，禁止合成证据顶替。
