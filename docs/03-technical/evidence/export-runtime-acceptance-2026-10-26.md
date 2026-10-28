# 导出运行时真实复验与剩余阻断（2026-10-26）

本轮选定 **20 个代表场景：19 PASS、1 FAIL**，通过项包括 16 个输出正例和 3 个拒绝反例。默认/显式日志、zstd/snappy、完整整库/排除表、零日期双可空性、空文件、显式 GBK 输出及五步 UI 均已有真实证据；`retain-schema` 未输出预期前缀，保留失败。完整 EX-V1、G3/G4 和发布门禁仍未通过。

累计实际任务 **32 次，25 SUCCEEDED、7 FAILED**，全部终态、无待核对。该次数包含修复前失败、重复回归与预期拒绝，不能作为 20 个场景的通过率分母。2026-10-14 的 172 项原始结果保持不变；本记录更新[安装及部分验证](export-runtime-real-2026-10-26.md)中的等待登录、零任务状态，不覆盖历史事实。

## 授权、范围与安装

用户已在本会话授权操作验证并提供正常登录所需凭据。仅使用既有 TEST `test-mysql` / ODP、Windows AMD64 `win-test`、OBDUMPER 4.3.5、登记的 Java `1.8.0_491` 以及两个独立 owner 夹具库。固定辅助进程只在短生命周期内解析已授权源密码并通过匿名 stdin 交给固定夹具程序，不把密码传入 argv、环境、文件或输出。没有修改业务表、增加账号权限或消费一次性审计授权。

两次本轮增量安装分别应用日志字符集和参数保护。每次先通过正常 API 确认无活动任务，再按既有 `dev.stop` 停止 Agent、控制面与监视器，确认旧进程退出及监听端口关闭，保存关闭状态的 SQLite 备份。原开发入口删除旧成品，以 `CGO_ENABLED=0 go build -trimpath` 构建控制面、Agent 及三目标下载资源，再以原参数、开关和工作目录恢复。构建期间健康连接拒绝不计成功；最终使用原 CA 独立取得 HTTPS 200。

最终只读核对：42 项迁移、43 张 STRICT 表；Agent ACTIVE、心跳新鲜、容量占用 0；32 个本轮任务均终态且 `reconciliation_required=0`，全平台无活动执行。6 个原配置/根密钥/信任文件摘要不变；机器身份、机器凭据摘要与修订、主体授权和对象范围与关闭备份一致，`audit_bootstrap_consumed` 仍为 0。已安装 Windows Agent 与下载资源摘要一致：`8da3352bbc1a23512f6ebd8429485ee0cf68f83371651e929df382f39e1b69d9`。Linux 两目标只有交叉构建证据，不代表原生验证。

## 实现与测试预期纠正

1. **默认日志与原生临时材料**：沿用[本地整改](export-runtime-closure-2026-10-26.md)的固定私有 console XML 和任务私有 `java.io.tmpdir`，不改业务 argv、不隐式开启目录空性跳过。实际默认/显式日志与三项 zstd、snappy 输出均解压后核对完整行集。
2. **整库日志编码**：首轮整库与排除表任务退出 0，却以 `PROCESS_WAIT_FAILED` 失败。已核对原生日志中的微秒符号采用 GBK 字节，严格 UTF-8 采集正确拒绝；只设置 `sun.stdout/stderr.encoding` 不足以约束 Log4j 缺省字符集。固定 JVM 增加 `file.encoding=UTF-8` 后，两项新任务成功，分别核对 12 张表 51,046 行、9 张表 1,045 行。采集器继续拒绝非法 UTF-8，不猜测转码、不吞掉末尾记录。
3. **Windows Java 8 参数保护**：含补充字符字面量的查询曾产生内容变化；不连接数据库的固定 Java 参数探针也复现补充字符被替换为 `?`，附加 `sun.jnu.encoding` 未解决。执行入口对业务参数及最终 JVM/业务 argv 按当前活动代码页做无最佳映射、无替代字符的严格往返检查；未知、非法或不可表示时在写启动意图和创建工具进程前拒绝，不改写冻结参数。实际反例为 `FAILED / START_REJECTED`，无 `startedAt`、退出码、计划实际匹配或结果摘要，无输出文件且无待核对；随后正常导出完整 4 行通过。这证明失败关闭，不代表该节点支持 emoji 字面量查询；从数据库读出的 emoji 数据仍通过完整核对。
4. **数据文件编码**：显式 `--file-encoding=GBK` 的独立 ASCII 查询输出 1 行中文，严格核对解码内容和实际 GBK 字节。日志 UTF-8 设置没有覆盖该显式参数。先前含不可表示查询字面量的内容失败保留，不算 GBK 正例通过。
5. **Snappy 检查器**：实际输出使用 Xerial framing，原检查器只按 Hadoop framing 解读造成一次错误失败。按同版本 [SnappyCodec](https://raw.githubusercontent.com/xerial/snappy-java/v1.1.10.7/src/main/java/org/xerial/snappy/SnappyCodec.java) 和 [SnappyInputStream](https://raw.githubusercontent.com/xerial/snappy-java/v1.1.10.7/src/main/java/org/xerial/snappy/SnappyInputStream.java) 核对头部及块结构，新增有界解析和损坏反例后，同一实际输出完整行集通过；初次失败摘要保留，不把重复解读算新任务。
6. **空表与零日期**：默认 CUT/SQL 无文件时保持 `RESULT_VERIFICATION_FAILED`，新任务核对退出 0、零文件/字节、失败结果和无待核对后作为拒绝反例通过。显式保留空文件则要求真实文件与零行。原零日期夹具列可空，其 NULL 输出不能套用 NOT NULL 保留预期；保留原库并读取实际可空性，再新建 NOT NULL owner 夹具验证零日期原值，两种格式的正反分支分别核对。检查器缺少可空性事实时拒绝判通过。

这些变更没有引入新产品依赖、API 枚举、迁移、自由命令、任意 SQL 探针或新用户能力；Windows 转换使用现有 `golang.org/x/sys/windows`。生成器仍是业务参数唯一权威，固定 JVM 配置归 Agent 启动层。

## 选定场景结论

| 场景 | 数量 | 结果与必要证据 |
| --- | ---: | --- |
| 默认日志 MX-001 | 1 | PASS；参数保护版本新任务，完整 4 行，2 文件/265 B |
| 显式日志 MX-001-LOG | 1 | PASS；完整 4 行，预览/冻结命令一致 |
| zstd 默认、1、22 | 3 | PASS；分别实际解压，完整 4 行 |
| snappy | 1 | PASS；Xerial v1 实际解压，完整 4 行 |
| 整库 MX-004、排除表 MX-005 | 2 | PASS；全部选择表及完整行集，51,046 / 1,045 行 |
| 默认空表 CUT/SQL | 2 | PASS 反例；零产物不能投影成功 |
| NOT NULL 零日期 CSV/CUT | 2 | PASS；新夹具两列均不可空，原零日期精确保留 |
| 保留 Schema MX-075 | 1 | **FAIL / DDL_SCHEMA_MISSING**；任务虽成功，DDL 未包含预期限定前缀 |
| 显式保留空文件 CUT/SQL | 2 | PASS；真实文件、完整零行断言 |
| 可空零日期 CSV/CUT | 2 | PASS；原夹具可空事实与 NULL 分支一致 |
| 显式 GBK 输出 | 1 | PASS；1 行中文内容和实际编码字节 |
| 五步 UI 默认日志导出 | 1 | PASS；真实对象目录、七项预检、确认、成功详情及完整 4 行 |
| 不可表示 argv | 1 | PASS 反例；进程前 START_REJECTED、零输出、无待核对 |
| 合计 | 20 | **19 PASS / 1 FAIL** |

五步 UI 沿用实际页面正常操作，没有拦截 API 或使用假 Agent；证据包括第四步默认日志说明、预检及成功详情截图。成功详情与独立文件核对一致：2 文件、265 B、4 行。任务标题及截图仅含本次自有夹具信息。

## 实际验证、清理与复核入口

| 检查 | 结果 |
| --- | --- |
| 全量 Go | 51 包、1,978 项测试含子项通过，0 失败；4 项既有测试跳过（1 GiB 可选物理检查、2 项 Windows 符号链接权限、Vite HMR 可选检查），另 4 包无测试文件 |
| vet | `go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...` 通过 |
| 执行与日志负例 | 旧缺省编码合成复现、UTF-8 修复、非法字节仍拒绝；Windows ASCII/中文/不同代码页/UTF-8/补充字符/非法字节/NUL/未知代码页及进程前拒绝通过 |
| E2E 检查器 | Node 13 项通过；Snappy 两种帧、版本/截断/越界、完整多表、空表分类和已核对可空性的零日期负例均通过；Python AST 通过 |
| 秘密与差异 | 仓库秘密扫描、`git diff --check` 通过；32 次任务公共快照、脱敏命令、全部分页日志及所属原生日志经固定辅助进程在内存核对，未发现已知源密码或其可恢复编码 |
| 安装与恢复 | 原 CA 健康 200；最新进程、同身份心跳、原配置/授权保留，Windows 安装/下载成品一致；三目标 CGO=0 构建通过 |
| 清理 | 两个夹具库精确核对 owner 后删除并确认不存在；两个所属输出根目录内 193 个文件（57,578,947 B）在保存摘要和核对原生日志后删除；辅助成品删除。备份、身份/信任、任务历史和脱敏证据保留 |

本机脱敏材料在忽略目录 `artifacts/export-runtime-20261026-7fb7dadd233d/`：`acceptance-summary.json` 是选定场景投影，七组 `*-results.json`/`*-verified.json` 保留原始回执与内容判断，`public-evidence-before-guard.json` 和 `argument-guard-public-evidence.json` 保存对应公共 API 证据，`owned-output-inventory.json` 保存清理前文件摘要，两个数据库清理回执与 `file-cleanup.json` 记录精确清理事实。清理后不能对旧路径再次运行内容收集器；复验须新建 owner 夹具和输出目录。

当前源码工作树含前序整改和用户原有改动，本轮只在必要执行层、E2E 检查器及对应契约内增量修改，没有提交、推送或创建 PR。安装指纹与源码状态材料只说明复核时快照，不作为文件归属声明。

## 剩余阻断

`retain-schema` 在本次 MySQL/ODP、4.3.5 的实际 DDL 缺少约定前缀，不能因退出 0 或文件存在算通过。本轮没有替工具改写 DDL、降低断言或据此开放能力；后续须定位固定工具行为与参数适用范围，再按既有产品契约决定修复或收缩。

Windows Java 8 命令行仍受节点代码页约束，不可表示参数现已安全拒绝，尚未实现该类字符的执行支持。没有重跑全部 172 项，也没有新增真实活跃租约恢复、检查点继续、跨主体身份、对象存储或麒麟原生证据；这些不能由当前正例或交叉构建推导为通过。
