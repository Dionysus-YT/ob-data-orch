# 普通导入全参数 API 验证结果

> 现役更新：后续已补齐十九项并完成真实复验，当前72/72定义有成功代表场景，见[十九项补齐记录](normal-import-capabilities-validation-2026-10-20.md)和[当前汇总JSON](normal-import-full-parameter-validation-2026-10-20.json)。本文以下50/72及22项阻断为前一批次历史，不是当前整体覆盖；全取值与组合仍未全部验证。

最新补验：现有 MySQL/Oracle 双数据源的剩余场景已执行，Oracle 三项 NLS 经修复和完整结果核对通过，当前 **53/72 条有成功真实场景，19 条能力限制仍阻断**。现役逐参数 JSON 已更新，前一批次 50/72 摘要另行保留；详情见[剩余场景验证](normal-import-remaining-validation-2026-10-20.md)。

## 前一批次记录（50/72，历史快照）

2026-10-20 已按用户授权，通过现有 TEST 数据源、真实 Windows Agent、OBLOADER 4.3.5 和普通 HTTPS API 完成补验。正常登录、Cookie、CSRF、修订与幂等保护全程保留。

**全参数功能验证仍未通过。** 72 条定义的 API 契约全部通过；50 条已有通过的真实执行场景，22 条仍受当前能力或 Oracle 目标证据限制。参数出现、正确拒绝、连接成功和退出码 0 均不单独算功能通过；50 条也不表示其全部取值、格式、权限、平台或性能范围通过。

| 验证批次 | 结果 | 实际证据范围 |
| --- | --- | --- |
| 72 条参数定义 API 契约 | 72 PASS | 草稿创建、活动参数预览、错误类型拒绝、草稿清理；修复安装后再次执行 |
| 原失败整改回归 | 23 PASS | strict、CSV 保真/trim 与风险拒绝、任务/模板/派生、CSRF、修订、幂等、配置失效 |
| 新参数真实执行场景 | 首轮 25 PASS / 3 FAIL；修复后 28 PASS | 每项完整目标行集/表结构、冻结 argv/指纹、受控进程、持久事件、工具终态与脱敏日志 |
| 修复及兼容版本补充 | 7 PASS | 上述失败三项复验、MySQL 5.6/5.7 两项补充、CUT 默认/显式 CRLF 不匹配两个拒绝反例；前三项与 28 项重叠，不能相加 |
| 兼容版本 API 边界 | 2 PASS | 旧开关未选版本可编辑但预览/预检 422；非法 mysql 创建 422 |
| 受限配置真实预检查 | 23 阻断 | 21 项提交拒绝并核对零任务；账号/角色两项只读预检，不试写安全对象 |

本 run 共 **42 个真实工具任务：38 SUCCEEDED、4 FAILED，全部终态且无待核对**。4 个失败包括两个预期的重复主键反例及已修复的 trail/compat 首轮失败；ignore 首轮虽工具成功但行集不匹配，仍判 FAIL。历史结果没有被覆盖，三项首轮失败与新任务的对应关系保留于汇总 JSON。

## 修复与实际复验

- CUT 的官方默认换行来自 Java `System.lineSeparator()`。原 Windows 节点对 LF 文件的检查误用通用换行语义，导致 ignore 末字段多 LF、trail 解析多列。预检查现以目标平台默认值检查副本，未知或不匹配保持 `INPUT_MAPPING/UNKNOWN/IMPORT_INPUT_MAPPING_UNSUPPORTED`；不改冻结配置或命令。两个 LF 夹具显式选择 `lineSeparator=\n` 后完整行集和结构通过；省略或选 CRLF 均预检阻断、提交 422、零任务。
- 官方 `--compat-mode` 仅接受 `mysql56/mysql57/mysql80`，原 `mysql` 被真实工具拒绝。新增显式源版本选择 `compatMysqlVersion`，关闭/不适用即清除；缺版本不能预览、预检或执行，不猜测旧任务版本，不改历史快照。三个官方值均通过小型受控 DDL 场景，后两项还验证替换结构缺确认时拒绝。此结果不证明任意版本 DDL 转换均正确。
- 先前 strict 同令牌布尔值、未引用 CSV 空白风险阻断和重跑锁定 422 的修复已通过 23 项真实回归。未引用文件是安全拒绝，不能称为原文件无损导入。

## 尚未功能验证通过的 22 条定义

| 数量 | 参数 | 本轮真实结果与后续条件 |
| --- | --- | --- |
| 6 | ctl-path、exclude-data-types、skip-footer、compress、compression-algo、compression-level | INPUT_MAPPING UNKNOWN / IMPORT_INPUT_MAPPING_UNSUPPORTED；提交 422，无任务。须先具备契约允许的受控文件/映射取证能力，再作工具与结果验证 |
| 13 | table-group、view、trigger、obj-user、role、sequence、synonym、type、type-body、package、package-body、function、procedure | WRITE_ACCESS UNKNOWN / IMPORT_WRITE_DDL_UNSUPPORTED。11 类提交 422；obj-user/role 仅预检，均无任务。须先具备对应对象的安全授权、定义识别及独立结果核对能力 |
| 3 | nls-date-format、nls-timestamp-format、nls-timestamp-tz-format | Oracle 连接与映射检查通过，但 WRITE_ACCESS UNKNOWN / IMPORT_WRITE_SCHEMA_UNAVAILABLE，提交 422，无任务。缺少可验证的受控 Oracle schema/写授权及独立结果读取条件，未启动工具 |

另对已有 UTF-8 通过场景的 `file-encoding=GBK` 执行真实拒绝检查：映射 UNKNOWN、提交 422、零任务。该分支不能因 UTF-8 通过而算通过。23 个受限配置 = 19 条定义的能力限制 + GBK 分支 + 3 条 NLS。

本轮不关闭检查、不扩大任意 SQL/账号权限、不自动授予 Oracle 权限。麒麟原生、其他平台、压缩/非 UTF-8、复杂对象、load.ckpt 恢复、大数据量及所有参数组合仍没有本轮成功证据，不提升完整 NIV/G3/G4。

## 安装、检查与清理

所有工具终态后，按原开发入口执行停止、监听关闭、删除旧二进制、当前源码 `CGO_ENABLED=0 go build -trimpath`、恢复与验证。当前控制面 PID 2636、Agent PID 31208；原 CA、根/信任密钥、控制面及 Agent 配置摘要一致。动态身份密文随受认证配置同步改变，以旧新 42 个任务相同 agent_id、原根密钥及同节点认证心跳确认身份连续性。

恢复后完整节点检查 PASSED，factsRevision 27。收尾 HTTPS 健康 200、心跳 ONLINE、环境 PASSED；running/waiting/reconciling、预留/占用额度及日志队列待发批次/缺口/字节均为 0，节点可接收任务。

相关 Go 测试（领域、生成器、映射、JDBC、执行、控制面、仓储、协议、Worker、预检及 contracts）、定向 vet 通过。前端相关 3 文件 45 项、类型检查及定向 ESLint 通过；API 客户端/门禁脚本 6 项通过；业务与向导审计 0 违规，已有 REVIEW 均未因本次修改新增共享职责，源版本继续由原表单/参数能力所有者持有。秘密扫描与差异检查通过。此前 72 合法定义、261 负例、102 边界/条件矩阵均为合成检查，不能混作真实场景数量。

完成结果取证和秘密布尔审计后，按 owner 标记删除本 run 独立库并查询确认不存在；核验精确输入目录位于允许根目录且无重解析点，保存输入文件摘要后删除并确认不存在。本轮未提交草稿已清理，已提交任务、事件与脱敏证据保留。没有改动其他业务库/目录，没有提交或推送代码。

## 可复核证据

- [72 条逐参数状态与修复任务对应](normal-import-full-parameter-validation-2026-10-20.json)
- [API 契约及兼容版本边界](normal-import-api-contracts-2026-10-20-results.json)
- [23 项整改回归](normal-import-api-regression-2026-10-20-results.json)
- [28 项最终参数场景](normal-import-api-parameters-2026-10-20-results.json)及[首轮失败原记录](normal-import-api-initial-parameters-2026-10-20-results.json)
- [7 项定向修复与版本补充](normal-import-api-repair-2026-10-20-results.json)
- [23 项受限配置检查](normal-import-api-gates-2026-10-20-results.json)
- [服务恢复、身份连续性与精确清理](normal-import-api-closeout-2026-10-20.json)

脚本入口见 [README](../../../scripts/normal-import-e2e/README.md)。本 run 已清理，不能重放旧夹具或把保留回执当成重新读取数据库；新真实批次须先按同范围授权建立新的独立所有权夹具。原始任务日记位于忽略目录 artifacts/normal-import-retest-20261019 与 artifacts/normal-import-full-20261020。
