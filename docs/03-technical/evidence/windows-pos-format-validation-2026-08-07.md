# Windows POS 定长格式受控实测与定版（2026-08-07）

> 验证状态：映射冲突已定版；控制文件双来源决策已确认；`--block-size` 默认值保留低风险残余
> 授权范围：`test` 数据源、`ob_test` 全量导出、跳过导出目录空性检查、输出路径 `tmp/run-01`
> 关联：EX-I4、EX-C05、EX-R03、EX-F017/EX-F029/EX-F031

## 1. 授权与安全边界

- 用户当轮明确授权：导出路径 `tmp/run-01`、导出数据库 `ob_test`、全量导出、跳过空目录检查、使用 `test` 数据源；随后确认 POS 命令映射定版、控制文件"用户提供 + 自动生成"双来源、`--block-size` 补测。
- 密码只经内存与 OBDUMPER 官方 `security.properties`（RSA 材料）短时传递，不进入 argv、日志、标准输出或文档；执行后清理临时材料并清零内存字节。
- 输出目录与工具日志（`tmp/pos-probe-run/`）保留供复核；`tmp/` 已被 Git 忽略，不提交任何输出、日志或临时材料。
- 控制文件列长度只读取 `information_schema.columns` 的列名与类型元数据，不读取任何业务数据；结果核对只统计字节数与行长分布，不输出字段内容。

## 2. 实测结论：POS 命令映射定版

| 组合 | 结果 |
|---|---|
| `--pos`（无 `--ctl-path`） | 失败关闭：`Option '--pos' should be used along with '--ctl-path'` |
| `--pos --ctl-path <控制文件目录>` | 成功：退出码 0，`--all` 全量 6 张表导出，用时约 21 秒 |

- **OBDUMPER 4.3.5 实际二进制支持独立 `--pos`**（`--help` 列出 `--pos  Interpret the fixed-length data format`，与 `--csv/--cut/--sql` 同级格式选项）；官网 4.3.6 正文"POS 命令行选项仍为 CUT + 空 `--column-splitter`"的口径与 4.3.5 实际行为不符，**4.3.5 以独立 `--pos` 为准**。
- 控制文件命名规范 `<表名>.ctrl`、格式 `lang=java ( 列名 position(字节长度), ... );` 与官方文档一致，日志确认 `Parse ctrl definition ... success`；`position(n)` 按字节长度定义，`n` 之和即每行定长字节数。

## 3. 定长输出证据（t_order_test，11 列）

- 控制文件列定义：`id position(20), order_no position(32), user_id position(20), user_name position(64), order_status position(4), order_amount position(14), product_type position(32), province_code position(16), description position(255), created_at position(19), updated_at position(19)`，position 之和 495 字节。
- 行长直方图：900 行严格 496 字节（495 内容 + 行尾 `\r`），`crlf=1000`；其余约 200 行为含字段内换行的记录被拆断（229~266 字节段）。
- **定长成立**：position 全列定义时每行固定字节；**边界事实**：POS 不转义字段内换行（与 CSV 转义行为不同），产品接入时需提示该限制。
- 当前 `ob_test` 各表行数呈 1,100 的整数倍（t_order_test 1,100 行、t_order_test5 5,500 行），与 WI-04 时期的 10,009 行不同，说明库数据此后已变化；本次忠实导出当前数据。

## 4. `--block-size` 补测结论

| 参数 | 结果 |
|---|---|
| 默认（不传） | ≤2.5MB 数据不切分（与候选默认值 0/1024MB 行为一致，无法区分） |
| `--block-size 1` | 按 1MB 阈值切分：t_order_test5 切为 3 个文件（1,074,514 × 2 + 335,972 字节）；最后一行超阈值仍完整写入（1,074,514 > 1,048,576），即按行粒度切分 |
| `--block-size 256ROW` | 按 256 行/文件切分：t_order_test 1,100 行 → 5 个文件，t_order_test5 5,500 行 → 22 个文件；末尾不足整块的文件保持独立 |

- 切分文件命名 `<表名>.<序号>.dat`，最后一个文件无序号（与官方"逻辑子文件"命名一致）。
- **残余**：`--block-size` 默认值（官网正文 0 vs 选项表 1024MB）在 ≤1024MB 数据量下不可证伪区分，保持 `CONFLICT_PENDING` 记录；V1.0 使用不受影响（显式传值时按参数生效）。

## 5. 控制文件来源决策（用户确认：双来源）

1. **用户提供**：向导提供控制文件目录输入（专家配置），任务冻结 `--ctl-path` 指向执行节点上的用户目录；控制面不接收控制文件内容，Agent 只把冻结路径作为参数。
2. **自动生成**：预检查阶段由 Agent 在 execution 私有目录按对象元数据生成 `<表名>.ctrl`（固定 `position` 字节长度规则），`--ctl-path` 指向该私有目录；控制面只传对象范围，不传 SQL 或任意文件内容。
3. 两种来源的校验、证据与失败关闭语义在 EX-I4 剩余实现中细化（生成器、归一化、预检查与结果事实）。

## 6. 未覆盖项

- `--pos` 与控制文件在正式产品通道（向导/生成器/预检查）的接入尚未实现（本次为映射定版实测）。
- 全量导出（`--all`）下自动生成控制文件需要对全部对象生成，逐表元数据查询的实现与预检查整合待实现。
- 官方 4.3.6"POS = CUT + 空 splitter"组合未在 4.3.5 上重复验证（独立 `--pos` 已定版，无必要）；`--pos` 与 `--exclude-column-names` 的互斥（控制文件包含其功能）沿用官方说明。
