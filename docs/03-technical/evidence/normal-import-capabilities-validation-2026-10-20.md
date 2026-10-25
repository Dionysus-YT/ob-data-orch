# 普通导入剩余十九项补齐与真实复验

2026-10-20，按用户“其余 19 项补齐后测试”的授权，完成实现、当前源码安装及现有 TEST MySQL/Oracle 数据源的正常 API 实测。最终 **19/19 PASS，8/8 拒绝反例 PASS**，当前 **72/72 参数定义均有成功真实代表场景**。逐项证据：[本轮结果](normal-import-capabilities-2026-10-20-results.json)、[当前覆盖](normal-import-full-parameter-validation-2026-10-20.json)、[此前 53/72 摘要](normal-import-full-parameter-validation-2026-10-20-before-capabilities.json)。

这是 Windows AMD64、OBLOADER 4.3.5、私有 ODP、本地受控文件的小型代表场景覆盖。`fullParameterRepresentativeValidationPassed=true`，`allParameterDefinitionsHaveVerifiedScenario=true`；`allValuesAndConditionsValidated` 与完整组合意义的 `fullParameterValidationPassed` 仍为 false。不把参数出现、正确拒绝、工具退出 0 或单独连接成功算作功能通过，也不提升完整 NIV/G3/G4。

## 实现及实际覆盖

| 参数 | 实际成功场景与独立核对 |
| --- | --- |
| `--ctl-path` | 逐列显式 map(2)/map(1)/map(3) 调换源列；完整目标行是 id=1、label=alpha、note=plain，列结构和主键一致；控制文件摘要进入冻结清单 |
| `--exclude-data-types` | 排除 BLOB；两字段数据导入三列表，BLOB 保持 NULL，其他字段及结构一致 |
| `--skip-footer` | CUT、显式 LF，忽略一条尾记录；目标只有预期一行 |
| `--compress`、`--compression-algo` | 两项独立 gzip/.gz 任务；解压输入与目标完整行集一致 |
| `--compression-level` | zlib、level=1、官方 `.zz` 后缀；目标完整行集一致 |
| `--table-group`、`--view` | MySQL 随机命名全局表组存在；纯常量视图存在且查询返回 1 |
| `--obj-user` | MySQL 精确随机账号、Host=0.0.0.0；只有 USAGE，无新授权，缺少冻结风险确认的提交 422 |
| `--function`、`--procedure` | MySQL 常量函数和单语句过程；独立调用分别返回 1 |
| `--role` | Oracle 精确随机角色存在且自身无系统、对象或继承角色权限；只有数据库原生授予创建者的 ADMIN OPTION 关系；缺风险确认提交 422 |
| `--sequence`、`--synonym` | Oracle SEQUENCE 类型与 VALID、增量 1/NOCACHE 元信息；同义词可读取本轮空基表 |
| `--type`、`--type-body` | Oracle NUMBER 属性类型元信息；类型体 VALID，独立构造对象并调用方法返回 1 |
| `--package`、`--package-body` | Oracle 包/包体对象与 VALID；包体过程可独立调用 |
| `--trigger` | Oracle 受限同列赋值触发器；对象 VALID、ENABLED、INSERT 事件及关联本轮基表一致 |

MySQL 最终 11 项、Oracle 8 项。现有 MySQL 账号没有可证明的 TRIGGER 直接权限，预检 `WRITE_ACCESS/UNKNOWN`，没有创建触发器任务；该参数改在已授权 Oracle 源验证，没有修改任何既有账号权限。

新增固定 JDBC v11 只传类型枚举、Kind/Name/Reference 等结构事实，不传 SQL。非表对象按具体权限及当前 Schema 依赖核对；Oracle 新连接只补充登录默认启用角色的直接系统权限，不展开非默认或嵌套角色。控制目录摘要冻结并在执行前复核；gzip/zlib 解压累计上限 32MiB、记录前缀上限 256KiB，原字节摘要、校验和、截断、尾随内容和取消均失败关闭。

## 首轮失败与修复复验

首轮十九项独立结论 **12 PASS / 7 FAIL**；随后仅对七项创建新任务复验，原始回执和失败任务保留。

- 控制文件省略 note 的 map 时，工具实际写入 alpha，而非 plain。原工具任务虽成功，独立行集判 FAIL。现在逐列要求显式 map，隐式位置保持 UNKNOWN；复验恢复精确结果。
- zlib 初始夹具用了 `.zlib`，工具发现零文件并退出 0，平台仍判 FAILED。扫描现与 4.3.5 的 `.gz/.zz/.zst/.sz` 精确后缀一致；别名不再冒充会被工具读取的输入。`.zz` 复验成功。
- MySQL 复合过程和 Oracle 类型体、包、包体按内部分号被工具拆开。MySQL 代表过程改为单语句；复合体识别要求独立行 DELIMITER 包装，Oracle 原始定义经官方工具执行后独立调用/元信息核对通过。
- MySQL 触发器权限未知保持阻断，Oracle 受限触发器补验成功。

角色夹具最初错误要求“没有角色授予关系”；Oracle 创建角色会自动授予创建者 ADMIN OPTION。核对器改为只允许这一条原生关系，并验证角色自身无权限；只重读元信息，没有重新创建角色或额外 GRANT。首轮 12/7 已采用这一核对更正，原始夹具回执仍保留。

本轮共 **25 个真实工具任务，20 SUCCEEDED / 5 FAILED，全部终态、无待核对**。加上此前批次，累计 **70 个任务：61 SUCCEEDED / 9 FAILED**；失败历史没有删除或改写。两个阶段使用同一 Agent 身份。

## 拒绝反例与验证边界

八个实际 API 反例均预检不通过、提交拒绝，SQLite 核对零任务：控制函数、损坏 gzip 校验和、算法/后缀不符、只有尾行、排除必填 INT 列、定义追加 DROP、可路由通配来源账号、无法证明触发器权限/依赖。拒绝后再次独立核对所有成功目标，结果未改变。

其中定义追加 DROP 和通配来源账号初次测试脚本误要求 INPUT_MAPPING 非通过；实际正确阻断在 WRITE_ACCESS/UNKNOWN。更正期望后仅重做两项拒绝提交检查，原来的 6 PASS/2 FAIL 测试回执保留；没有启动工具。Go 另覆盖未知控制语法、重复/缺失映射、错名/尾随 SQL、坏压缩流、尾随内容、解压超限与取消；Java 覆盖缺权限、非默认/嵌套角色、依赖缺失及旧帧拒绝。

仍不声称通过：zstd/snappy 解压映射、非 UTF-8、控制函数/生成列、任意复杂 DDL、跨 Schema、非表替换、账号秘密或授权迁移、麒麟原生、大规模及恢复/性能组合。gzip 两个参数场景不代表所有压缩算法通过，zlib=1 不代表全等级通过。

## 安装、检查与清理

控制面与 Agent 两轮均按“停止旧进程与端口 → 删除旧二进制 → CGO_ENABLED=0/trimpath 构建 → 原配置、身份材料和开关恢复”执行。最终独立输出路径的当前源码重建与两个运行二进制 SHA256 完全一致；静态 CA、根密钥和配置摘要保留。最终 HTTPS 200、近期心跳 ONLINE、全新节点环境 PASSED，任务空闲、执行额度占用和日志待传队列均为零，可接收新任务。

已通过：相关 Go 单元/契约和 vet、此前 store/controlplane 回归、Java 8 八类探针夹具、前端 API/状态 31 项及类型检查、定向 ESLint、业务/向导边界审计、API 客户端六项、秘密扫描与 diff 检查。审计仅有既有其他模块的 REVIEW 信号，本次仍由原表单与风险确认所有者持有状态，没有新增跨层依赖。

本轮独立库、随机账号/表组、Oracle 角色/类型/包/触发器等对象、输入目录和未提交草稿均已按所有权清理并确认不存在。任务、快照与脱敏证据保留；MySQL/Oracle 密码及可恢复编码布尔审计通过。收集脚本复核的是已保存的清理前目标证据，不代表清理后重新读取了目标。

复核入口：`collect-capabilities.py`、`merge-capabilities-coverage.py`；真实执行入口和精确回执约束见 [测试说明](../../../scripts/normal-import-e2e/README.md)。旧 run 已清理，禁止重放。

语义依据：[官方 4.3.5 参数](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997308)、[OceanBase 默认角色生效规则](https://en.oceanbase.com/docs/common-oceanbase-database-10000000003455740)。后缀与文件分隔符同时回查本机固定 4.3.5 JAR 并经真实执行核对，未从其他版本文档外推全部能力。
