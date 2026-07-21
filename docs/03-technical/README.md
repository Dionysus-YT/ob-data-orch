# 技术设计入口

> 文档状态：工程启动架构、首条切片范围与技术路线已确认
> 当前阶段：首条切片最小实现契约设计与 P0 补齐
> 更新日期：2026-07-21

## 1. 目录用途

本目录承接已经确认的产品与低保真基线，将其转化为可验证、可实现和可追踪的工程约束。技术设计保持轻量，只建立首条纵向切片真正需要的架构、边界、门禁和验收，不提前设计全部模块的接口和数据表。

## 2. 当前文档

- [技术架构首版](architecture.md)：控制面、执行 Agent、参数元数据、命令生成、任务状态、日志、权限和凭据的职责边界。
- [首条纵向切片](first-vertical-slice.md)：数据源管理到基础 CSV 导出的最小开发范围、P0 证据和验收条件。
- [技术路线与部署选型基线](technology-stack.md)：已确认的 Go、Vue 3、SQLite、Windows AMD64，以及麒麟 V10 SP1/V11 ARM64、V10 SP3 C86 三个国产 Linux 目标和轻量任务通道，记录 TS-R01～TS-R14。
- [参数元数据与确定性命令生成契约](parameter-command-contract.md)：首条切片的值状态、最小参数集合、稳定顺序、指纹、跨平台路径和秘密槽位边界；PC-R01～PC-R15 已确认，契约测试待执行。
- [Agent 协议与任务状态最小契约](agent-task-state-contract.md)：首条切片的机器认证、心跳、长轮询、租约、幂等事件、进程证据、状态投影和失联恢复；AS-R01～AS-R16 已确认，契约测试待执行。
- [凭据、权限与安全最小契约](credential-access-security-contract.md)：数据源密码、Agent 机器凭据、跨平台根密钥、短时槽位解析、权限、审计和 OBDUMPER 进程暴露边界；CS-R01～CS-R18 已确认。
- [OBDUMPER 跨平台隔离启动入口契约](tool-launch-isolation-contract.md)：直接 Java 受控入口、版本化启动配置、execution 私有目录、最小环境、进程恢复和清理边界；TL-R01～TL-R18 已确认。
- [日志采集、双层脱敏与执行证据最小契约](log-collection-evidence-contract.md)：定义日志来源、记录重组、双层脱敏、分段存储、游标、缺口和下载边界；LG-R01～LG-R20 已确认。
- [开发准入清单](development-entry-checklist.md)：版本控制、产品基线、验证、技术设计、安全和可追踪条件的状态。
- [本地工具包证据](evidence/tool-package-baseline.md)：4.3.5 压缩包摘要、版本、109/103 参数集合及 Windows 启动环境发现。
- [安全文件兼容性验证](evidence/secure-gen-compatibility-spike-2026-07-21.md)：平台内存生成的 PKCS#8/RSA 材料被 Windows 4.3.5 解密器与 OBDUMPER 读取、错误私钥负例、ACL 和三目标构建证据；Linux 运行与清理仍阻断。
- [直接 Java 隔离启动验证](evidence/direct-java-launch-spike-2026-07-21.md)：Windows 结构化 argv、任务级安全配置、含空格路径、最小环境、直接父子进程和真实退出码证据；Linux 与成功路径待验证。
- [日志流重组与双层脱敏验证](evidence/log-stream-redaction-spike-2026-07-21.md)：本地合成验证跨读取块秘密、UTF-8 重组、重复批次、冲突批次和超长记录缺口规则。
- [首条纵向切片 P0 执行记录](evidence/first-vertical-slice-p0-2026-07-21.md)：连接、只读查询、Windows OBDUMPER 运行、脱敏和 VS-P0-02～VS-P0-12 状态。
- [SQLite 跨平台最小技术验证](evidence/sqlite-cross-platform-spike-2026-07-21.md)：纯 Go SQLite 候选的 Windows AMD64、Linux AMD64/ARM64 构建证据和 Windows 事务/备份运行结果。

## 3. 阶段结论

- 产品规则、字段规则和已规划静态低保真已经形成基线；
- 项目已经初始化独立 Git 工作区，官方工具压缩包仅作为本地验证输入，不纳入版本控制；
- 可以开展技术设计、验证工具和测试夹具准备；
- TD-001～TD-008 与“直连 OBServer 单表 CSV 导出”首条切片范围已经确认；
- TS-R01～TS-R14、PC-R01～PC-R15、AS-R01～AS-R16、CS-R01～CS-R18、TL-R01～TL-R18 和 LG-R01～LG-R20 已确认；参数及 Agent 契约测试、安全路线剩余跨平台门禁、API 和数据模型最小契约仍有阻断，因此暂不进入业务代码实现；
- 当前测试端点疑似 ODP；Windows 实际参数采用用户确认的完整盘符绝对路径，早期 `file://null` 结果保留为正式 Agent 环境回归项；
- 不要求等待全部 83 个 P0 用例完成，采用“切片相关 P0 通过后开放对应实现”的增量准入方式。

## 4. 维护规则

- 技术设计不得把“待官方参数映射确认”改写成已支持；
- 每项实现能力必须能追踪到产品规则、字段、参数映射和验证证据；
- 验证失败或环境阻断时收缩、隐藏或阻断对应能力，不在代码中猜测兼容行为；
- 密码、密钥、Token、sys 凭据和完整敏感命令不得进入普通配置、日志、测试快照或 Git；
- 首条切片评审通过后再补充实现级 API、数据模型和任务协议，避免在证据不足时过早固化。
