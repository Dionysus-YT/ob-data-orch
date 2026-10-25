# 单机旁路直连修复与 API 验收 · 2026-10-22

**结论：已实现、安装并验证 Windows AMD64 / MySQL 单机 OBServer 4.4.2.3 / 固定 OBLOADER 4.3.5 的 CSV 旁路闭环。** 主 API 批次 14/14 PASS、7 个真实任务独立目标验证通过；日志与持久摘要修复后的最终批次 8/8 PASS、1 个真实任务严格验收通过，另 7 项 HTTPS 未登录边界通过。此结论只覆盖下述场景，不代表完整 DLV/G3/G4、所有参数、Oracle、其他新版本或 F5/ODP 环境已通过。

用户明确单机环境、正常 API 测试并授权真实执行，随后采纳修复方向。实施依据 [DEC-058](../../01-product/decisions.md#dec-058-单机旁路直连链路与数据源登记)。机器摘要见 [results.json](direct-load-standalone-repair-2026-10-22-results.json)。修复前的 [API 阻断记录](direct-load-api-2026-10-22.md)保持历史原值。

## 修复行为

- 数据源可登记、编辑和基础测试 `OBSERVER_DIRECT`；直连集群必须为空，租户仍必填。SQL 默认 2881，RPC 默认 2882，均允许实际配置；ODP 保留其默认值。仅空值或原默认值随方式切换，自定义端口与恢复值保留，不根据端口猜测连接方式。
- 旁路按所选数据源带出“通过 ODP 连接 / 直连 OBServer”。历史场景冲突须显式应用当前方式。修正前端错误别名 `OBSERVER`，与服务端 canonical 枚举一致。
- SQL 身份使用 `username@tenant`，仍独立生成工具必需的 `--tenant`；直连不生成 `--cluster`。唯一生成器、固定预检查凭据槽位、冻结 argv 与 Agent 复验保持一致。导出/普通导入仍限定既有 ODP 资格，未扩大能力。
- 加入实证的 **固定 4.3.5 + MYSQL + OBSERVER_DIRECT + 4.4.2.3** 精确版本行；其他新版本或 Oracle 同版本证据不足为 `VERSION_UNVERIFIED` / UNKNOWN，阻断提交，不解释为官方宣告不支持。
- 真实采集器正常 LOG 使用空完整性原因码，解析器兼容空值与历史 COMPLETE；未知码、缺口、错表和单独退出码仍不能判定成功。接受事件中的 `serverCommitObserved` 和安全失败码进入持久摘要，未回填旧摘要，未虚构 execution API 字段。
- 主工具采用 execution 私有固定 stdout XML；表 SDK 路径绑定私有目录，Bolt 经官方 SOFA 配置资源入口使用同一受控输出。工具会覆盖全局 `logging.path`，单独设置 JVM 路径不足。固定 XML 排他创建、复用时逐字节校验，runtime 仅用于该固定资源且位于已核验主包与 lib 后；当前工作目录不在 classpath。配置和路径不能由任务提供。

## 真实验证与历史失败

正常 `/login`、受信 CA、Secure/HttpOnly/SameSite Cookie、CSRF、幂等和修订均走产品协议。密码仅经匿名 stdin 与短时字节请求传递，未复制浏览器身份或更改已有账号权限。所有写入仅面向本轮随机所有权库与目录；原共享 TEST 数据源及凭据修订保持不变，新增 TEST 直连源在结束后归档。

| API 用例 | 主批次结果 |
| --- | --- |
| 直连源基础连接 | 真实 Agent JDBC SUCCEEDED，随后显式启用 |
| 基础 CSV | 九项预检通过，缺失最终确认拒绝，幂等重放仍为同一任务 |
| 同表分片 | 两个输入文件，完整目标行集正确 |
| 自定义分隔符 | `|` 解析结果正确 |
| 增量 | 原有行保留，两行新数据正确 |
| truncate | 原有行移除，两行新数据正确 |
| thread / parallel | 分别显式设为 2，任务与目标结果通过 |
| 五类结构反例 | BIT、二进制、虚拟列、有二级索引的 replace、LOB replace 预检失败，提交 422 `PRECHECK_REQUIRED` |
| 修改配置后的旧证明 | INVALIDATED，提交 422，不产生任务 |

BIT/二进制反例只证明当前平台受控范围的拒绝行为；4.3.5 发布说明与旧专题存在差异，不据此声称官方工具不支持这些类型。

独立收尾比较所有目标表的完整行集和 DDL，核对中文/DECIMAL、未触及表、进程退出 0、planned/actual MATCHED、指纹、唯一启动/结果事件与服务端同表提交事实；输入文件计数不作为导入结果。最终批次另严格要求持久摘要提交字段为 true、输入目录无额外日志，全部通过。

| 本地原始回执前缀 | 结果及边界 |
| --- | --- |
| `21612f54a423` | 主批次 14 PASS、7 个成功真实任务，独立验证通过；当时旧持久摘要缺提交字段，提交事件本身完整，随后修复 |
| `7ca0c4b6db2f` | 首轮脚本误要求 execution API 返回内部 resultSummary，API 用例 FAIL；实际任务 SUCCEEDED，保留原失败回执 |
| `bf40955b9681` | 8 项 API 通过，提交摘要与目标结果正确；严格日志验收 FAIL，Bolt 仍写入输入目录，随后修复 |
| `73774530ff4a` | 日志隔离通过，但 RPC 登录响应超时，真实任务 FAILED/退出1/待核对；所有目标表前后完全相同。保留失败及核对标记，根因未定 |
| `9fab01fbf58f` | 新夹具最终 8 PASS、真实任务 SUCCEEDED；严格日志与持久摘要、完整目标行集和清理均通过。成功不用于推断前次超时根因 |

上述五批共 11 个真实 API 任务：10 成功、1 失败，均已终态。失败任务的 `reconciliationRequired=true` 保留；独立目标核对与清理证据不能擅自修改产品历史状态。另有隔离组件核查：首次缺少正确 Hadoop 原生依赖环境而退出1，修正固定环境后两个组件场景提交与完整目标结果通过；组件不计为 API 任务。

原始回执位于忽略的 `artifacts/direct-load-e2e-20261022-<前缀>/`。这些已清理批次不能重放；故障回执未删除或改写为通过。

## 安装、检查与清理

控制面和 Agent 按停止、确认端口关闭、删除旧二进制、`CGO_ENABLED=0 go build -trimpath`、恢复原参数/工作目录/状态与 TLS 材料的流程安装。中断恢复后再次核对健康 200、Agent ACTIVE/心跳正常、环境 PASSED、占用0、无非终态任务。旁路探测和执行开关在两端均为 true；默认交付值仍关闭，当前本机显式授权配置已经启用。

| 检查 | 实际结果 |
| --- | --- |
| Go 回归 | `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` 通过；最后 Bolt 配置变更后受影响 agentexec/agentexecution/cmd-agent 测试通过 |
| Go 静态检查 | 最终全范围 `go vet` 通过 |
| 前端 | 13 文件 / 76 单测、类型构建、lint 通过；构建保留大 chunk 与插件耗时提示 |
| 架构 | business / wizards 均 0 violations；16 / 4 REVIEW 信号人工复核，未扩大为无关重构。来源状态由 sources 所有、旁路资格由 direct-load 所有，跨模块只复用现有展示事实 |
| 浏览器 | 17 个旁路用例及 3 个直连源用例最终通过，含三档视口、键盘/弹层和旁路原生200%；早期选择器/夹具假设错误已修正并保留失败日志 |
| 视觉复核 | 实际查看三档来源抽屉与旁路工作面；等待 Ant 下拉关闭后重新截图，修正截图捕获动画残影 |
| 身份与秘密 | 最终7项 HTTPS 边界通过；源码秘密扫描、124份 API/组件回执及服务日志内秘密核查通过 |
| 差异与脚本 | `git diff --check`、Node 入口语法及 PowerShell 解析通过 |

浏览器插件及其专属 browser skill 不可用，按已采用 frontend-testing-debugging skill 使用隔离 Vite + 常规 Playwright/Chrome；无真实 API 代理，UI 只使用合成响应。截图位于仓库外 `C:/Users/Dionysus/.codex/tmp/direct-*-repair-ui-*-20261022/`；业务端到端验证由上述真实 API 批次完成。

本轮自有库、输入目录、未提交草稿和临时组件执行目录均已清理；新增源已归档，历史任务和脱敏证据保留。安装前 SQLite 一致性备份、原始开关元数据与运行日志仅保存在忽略的 artifacts。没有提交、推送或创建 PR。

## 使用与未覆盖范围

在数据源管理登记专用“直连 OBServer”数据源，填写实际主机、SQL 端口和租户，集群留空；保存并测试/启用后，在旁路导入中选择它，场景和 RPC 默认值会带出，最终仍需九项真实检查及风险确认。原共享 ODP 源保留供原有模块使用，本轮没有把 SQL 2881 自动解释为源类型转换。

未验证 Oracle、其他 4.4 补丁/新版本、麒麟原生、真实 ODP/F5、生产负载、全部参数/取值、恢复/断点或完整56项 DLV。一次 RPC 登录超时的根因仍未确定；目前证明其失败投影正确、目标未变化和随后独立新夹具成功，不承诺环境稳定性。

## 官方依据

- [集中式版 OBLOADER](https://www.oceanbase.com/en/docs/common-oceanbase-database-standalone-1000000006338110)：单机直连 SQL/RPC 路径，默认值不能替代实测。
- [集中式版租户登录](https://www.oceanbase.com/docs/common-oceanbase-database-standalone-1000000003577291)：保留 `username@tenant` 身份。
- [4.3.5 旁路专题](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997310)与[4.3.5 发布说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000005770388)：保留精确旧矩阵及文档差异；当前新版本结论来自固定工具实际提交与目标取证。
- [SOFA 官方配置说明](https://github.com/sofastack/sofa-common-tools)及[配置资源读取实现](https://raw.githubusercontent.com/sofastack/sofa-common-tools/master/src/main/java/com/alipay/sofa/common/log/factory/AbstractLoggerSpaceFactoryBuilder.java)：结合本机固定 sofa-common-tools 1.3.11 / Bolt 1.5.10 字节码、包内 XML 与 LoadClient 的覆盖行为核验，没有修改官方工具包。

固定主 JAR SHA256：`b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06`；Connector/J 2.4.14：`3168e7e3f2f14f35738d7c695e05012bf351f623757c7baada29659d2afa2ad4`。
