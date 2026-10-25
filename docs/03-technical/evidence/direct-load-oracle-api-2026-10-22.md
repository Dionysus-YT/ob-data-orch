# Oracle 旁路数据源 API 验证 · 2026-10-22

> 本页保留修复前批次事实。后续已完成固定探针、精确 Oracle 版本实证与现役安装，新批次 12/12 API、7/7 真实任务及独立结果通过，当前结论见[Oracle 修复验收](direct-load-oracle-repair-2026-10-22.md)。以下 BLOCKED 仅指本页历史批次。

**结论：Oracle 基础连接与前置 API 链路通过，旁路 E2E 仍为 BLOCKED。** 正常登录 API 的6项记录为 **5 PASS、1 BLOCKED、0 FAIL**；九项完整预检为6 PASSED、3 UNKNOWN，整体 FAILED；有效确认指纹的提交返回422 `PRECHECK_REQUIRED`。没有创建 Oracle 旁路任务或启动官方 OBLOADER，不能声称 Oracle 真实导入通过。

用户明确要求验证 Oracle 数据源，沿用本会话的 API、真实环境及测试资产创建/清理授权。验证对象是既有 TEST OceanBase Oracle 兼容模式源；真实只读 `OB_VERSION()` 为 **4.4.2.3**，当前登录用户与登记 Schema 匹配。固定工具主 JAR SHA256 为 `b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06`。机器摘要见 [results.json](direct-load-oracle-api-2026-10-22-results.json)。

## 实际测试

原源登记为 ODP、SQL端口2881且缺少集群名。验证时通过正常 API 登记独立 TEST `OBSERVER_DIRECT` 源，保留原源的身份、配置和凭据修订。登录使用受信 CA、Secure Cookie、CSRF、幂等和修订；密码通过短时字节与匿名 stdin 传递，没有绕过认证或更改账号授权。

Oracle 使用当前账号 Schema 内随机 `DLVO_<run前12位>_OWNER` 与 `..._BASIC` 两张表，所有权记录绑定完整 run；基础表为 NUMBER / VARCHAR2 / NUMBER，输入是独立目录中的合成 CSV。固定夹具不提供任意 SQL、连接串、驱动或类名入口。

| 用例 | 结果 |
| --- | --- |
| Oracle直连源基础连接 | PASS：真实 Agent JDBC SUCCEEDED / DATABASE_CONNECTED |
| 命令身份与租户 | PASS：大写用户名、username@tenant、独立tenant，无cluster；仅证明平台预览映射 |
| 单文件发现 | PASS：真实 Agent 得到1个输入文件及绑定清单 |
| 同表分片发现 | PASS：真实 Agent 得到2个输入文件及绑定清单 |
| 九项完整预检 | BLOCKED：下表三项无法确认，预检整体FAILED |
| 未证明的提交 | PASS：包含当前预检ID与配置确认指纹，仍返回422 PRECHECK_REQUIRED，无任务 |

| 预检查 | 状态 |
| --- | --- |
| TOOL_ENVIRONMENT | PASSED |
| NODE_RESOURCE | PASSED |
| INPUT_FILES | PASSED |
| DATABASE_CONNECTIVITY | PASSED |
| RPC_CONNECTIVITY | PASSED；只证明TCP可达 |
| DIRECT_LOAD_VERSION | UNKNOWN |
| WRITE_ACCESS | UNKNOWN |
| DIRECT_LOAD_STRUCTURE | UNKNOWN |
| DIRECT_LOAD_CONFIG | PASSED |

## 阻断依据

1. `internal/directload/precheck.go` 的4.4.2.3实证行只覆盖 MYSQL / OBSERVER_DIRECT，Oracle没有固定工具的实际提交与完整目标结果证据，返回VERSION_UNVERIFIED，不将未知表述为官方不支持。
2. `DirectLoadProbe.java` 在Oracle连接、Schema和版本读取后明确提前返回；写权限与表结构保持UNAVAILABLE/空事实。两项UNKNOWN来自尚未实现的固定Oracle分支，不能诊断成用户缺权限或表异常，也不能用测试夹具可建表替代旁路所需权限证明。
3. 当前提交门禁按完整证明失败关闭。没有放开Oracle版本行、伪造表事实、借用MySQL证明或绕过API启动工具。

[官方4.3.5旁路说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997310)包含Oracle兼容模式，并要求用户名大写；其专题版本表不能直接证明固定4.3.5与Oracle4.4.2.3组合。后续要完成真实导入，需取得该Oracle组合的受控工具适用证据，补齐固定只读Oracle权限/结构契约及实现，再从九项预检查和真实API提交重新验证。

## 独立核对与清理

回执位于忽略的 `artifacts/direct-load-e2e-20261022-0d1831be18cc/`。`collect-oracle.py` 独立核对：

- 目标空行集、三列定义与主键数量前后一致；两次目标读取使用固定夹具的新JDBC连接。没有解析或写入CSV数据，不能用此结果证明Oracle数据导入精度。
- 原Oracle源的修订、连接方式、兼容模式、状态、凭据修订和端口一致。
- 临时源已删除、两个草稿已清理，关联任务数量0；所有权验证后删除两张测试表并确认不存在，输入目录不存在。
- 控制面健康200，Agent ACTIVE、环境PASSED、占用0，无非终态执行；控制面与Agent二进制摘要相对上轮安装均未变化。
- 7项HTTPS未登录边界通过；18份批次JSON的Oracle密码及可恢复编码检查通过，未发现admin原文/UTF16。

隔离测试辅助代码的Go构建、测试、vet，Java8编译，Node语法、PowerShell解析和两个无网络非法输入帧反例通过；源码秘密扫描与差异检查通过。本轮没有生产代码修改、服务重启或工具包修改，未重复运行与变更无关的全前端测试。

测试启动脚本首次范围正则错误发生在登录和排他启动标记之前，没有API调用或数据库变更；修正后才启动本批次，原诊断保留在 `runner-prestart.json`，未删除启动标记重放批次。当前 `apiSemanticsPassed=true` 仅表示已验证的API行为符合契约；`functionalE2EPassed=false` 与 `ORACLE_API_E2E_BLOCKED` 是功能结论。
