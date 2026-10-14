# 执行节点详情重构与合成验证

日期：2026-10-12。依据：用户确认“运行概览为默认、资源与执行额度并排、任务统计紧凑、环境与配置独立 Tab”的方案并授权执行。现有工作区包含此前节点注册、预算和全平台反馈改动，本轮在其上追加，不回退或提交其他改动。

> 本文前段记录同日各次实施与验证，不代表最新可见布局。采集链继续保留，现役展示与平台路径修正以末节“核心详情精简与平台路径修正”为准。

## 已完成的页面范围

- 稳定对象头：返回节点列表、名称、操作系统/架构、节点 ID、刷新、编辑和一个当前主操作。
- 四类状态共用 `nodeCategoryStates`；检查中覆盖旧环境结论，Agent 关联与心跳归同一类别。接收资格仍由服务端投影，首因 Alert 与其余原因展开不代替任务预检查。
- 默认运行概览：复用资源进度条及额度圆环，磁盘选择最高使用率的同份容量采样；额度不由槽位、主机百分比或任务数量反推。本人/全部任务统计使用服务端 scope，查看任务保留节点及账号范围。
- 环境与配置：登记配置、Agent 上报、预算策略和逐目录空间。预算策略整体缺失时保持未提供，完整策略中的空值/零值显示自动策略；默认 JVM 堆只使用返回值。目录容量不重复累加，长名称和路径可换行。
- 下载入口归关联指引或 Agent 分区，待关联注册码入口唯一。环境检查入口说明当前事务会禁用节点，不接收新任务，通过后仍需单独启用；未增加操作或资格。
- 手动刷新由 `useNodeDetail` 防重并维护原位完成反馈和上次成功读取时间。失败保留事实与成功时间；401/403/404、路由切换、卸载和迟到响应不能恢复旧事实或反馈。Agent 采样时间与页面读取时间分开。
- Tab 不建立第二份详情、表单或请求，对象或授权失效回到概览；方向键及 Home/End 只处理当前 Tab，不影响资源提示或注册码弹层。

P1 与模块事实已同步：[节点页面规则](../../../design-system/pages/execution-nodes.md#节点详情运行概览与环境配置)、[节点业务设计](../../02-design/execution-node-module.md#10-节点详情)、[节点维护索引](../../../web/src/workbench/nodes/README.md)。P0 Shell、Archetype、Token、API、Agent 和 SQLite 契约未修改。

## 唯一所有者与实现入口

| 入口 | 本次职责 |
| --- | --- |
| `web/src/views/ExecutionNodeDetailView.vue` | 对象头、状态/原因、Tab、键盘、导航和原注册码弹层焦点协调 |
| `web/src/workbench/nodes/useNodeDetail.ts` | 唯一详情、修订写入、读取代、刷新时间与反馈；沿用原会话及一次延迟复读 |
| `web/src/workbench/nodes/NodeRuntimeOverview.vue` | 只读资源、额度和任务汇总；不持有副本或发起请求 |
| `web/src/workbench/nodes/NodeEnvironmentConfiguration.vue` | 只读配置、上报、预算及目录空间；不推断未返回事实 |
| `web/src/workbench/nodes/NodeResourceUsage.vue` | 增加详情展开显示，列表保留紧凑模式；数值与颜色口径未改 |
| `web/tests/execution-node-detail.spec.ts` | 首轮七项真实页面合成验证，去重后增加平台差异与目录配置负例至八项；同时消费现有 API 解析与路由 |
| `web/src/workbench/nodes/nodeLifecycle.test.ts` | 刷新防重、成功时间、失败/授权/卸载及迟到反馈负例 |

## 实际检查

| 检查 | 结果 |
| --- | --- |
| 节点 Vitest | 首轮 60/60，去重与上报平台格式化负例追加后 64/64 通过 |
| Chrome 合成浏览器回归 | 首轮 38/38，键盘细化 2/2；去重后的八项详情与两项注册分类定向复验 10/10 通过 |
| TypeScript | `typecheck` 与最终 `build` 内的 `vue-tsc -b` 通过 |
| ESLint | 全前端检查通过；键盘与截图断言补充后新测试文件定向检查通过 |
| Vite 生产构建 | 通过；保留原有大包体提示，未扩展为包拆分重构 |
| 业务架构 | 审计测试 11/11，0 violations、6 个既有 REVIEW；本次入口与两个只读分区没有新增跨层或状态所有权信号 |
| 秘密扫描 | 通过，仅合成材料 |
| 差异检查 | `git diff --check` 通过 |
| 截图与布局 | 实际查看 1440 概览/配置、离线/刷新失败及 640 长值截图；1920×1080、1440×1024、1280×720、640、320 两个 Tab 均无整页横滚，200% CSS 缩放下复验 |

关键交互：默认概览 → 资源提示 → 键盘切换配置 → 预算/目录查看 → 返回概览，不补读第二份详情；环境检查 → 检查中 → 返回通过但已禁用 → 显式启用；A → B → A 复用实例 → 旧响应返回 → 当前节点保持有效；刷新权限撤销 → 清空配置/时间 → 新授权读取回到概览。原注册码签发、关闭清理、焦点恢复和复制反馈回归通过。

浏览器使用仓库锁定的 Playwright 与已安装 Chrome，地址 `http://127.0.0.1:15174`，隔离 Vite 无 API 代理，接口均由合成路由响应提供。会话未提供 Browser 插件的 `browser` Skill，采用现有测试入口。首次启动的回环健康探针遇到环境代理的 `socks5:` 协议不受支持，后续只在测试子进程为回环地址设置 `NO_PROXY`，未改全局代理。

首次两项默认概览测试将产品标题误写成固定标题，已改为真实路由标题；后续发现当前 Ant Tabs 可见项仅处理 Enter/Space，补齐本页方向键及 Home/End 后保留键盘断言复验。截图使用真实浏览器输出，保存在本次 Codex 的仓库外工作目录，未提交截图、Trace 或临时报告。

复现入口（在 `web/` 中）：

```powershell
npm run test -- src/workbench/nodes
npm run lint
npm run build
npm run audit:business
$env:NO_PROXY = '127.0.0.1,localhost,::1'
npm run test:browser -- tests/execution-node-detail.spec.ts tests/execution-node-page.spec.ts tests/execution-node-lifecycle.spec.ts tests/execution-node-registration.spec.ts tests/execution-node-resources.spec.ts tests/platform-feedback.spec.ts
```

## 去重与采集点覆盖复核

> 本节保留上一轮核对时的事实。用户随后明确要求补齐缺项并删除目录区块；现役结果以文末“诊断链补齐与目录区块删除”为准。

用户进一步要求：环境与配置不重复运行概览已有内容，并核对是否涵盖全部采集点。本次核对为源码与合同复核，没有执行真实采集。CodeGraph MCP/CLI 不可用，使用 rg 和源文件逐层对照，未重建或更改索引。

去掉配置区重复的平台分类、Agent 采样时间和任务槽位；实际上报的平台与登记相同时省略，差异或未知保留。目录白名单与采样容量合并为一份明细，配置目录只出现一次，缺空间样本仍显示已登记路径和未知；仅匹配当前 allowedRoots，旧/额外目录样本不进入页面。系统预留、堆外预算和默认堆仍是独立策略，不混同概览圆环的任务分配预留。

| 事实或采集点 | 当前来源与接口 | 页面覆盖及限制 |
| --- | --- | --- |
| CPU 使用率、内存使用率 | `agenttelemetry.Collector.Sample` → `agentwire.EnvironmentFacts` → `agentFacts` | 概览进度条与百分比；首次或失败采样保持未知 |
| 物理总内存、可用内存 | 同次 physicalMemory 采样，控制面成对安全投影 | 概览内存提示展示同采样的已用/总量，可用量作为 total − used 表达，不用百分比或额度反推 |
| 各配置目录总量/可用量 | 各平台 dataRootUsage → rootDigest 匹配当前配置 → `dataRootUsages` | 概览最高使用率采样 + 配置逐目录已用/总量；不漏配置、不混旧目录、不累加同盘 |
| 操作系统类型、CPU 架构 | Agent 主循环上报，控制面校验平台一致性 | 对象头显示登记分类；上报分类不一致时配置补充实际值。不是操作系统发行版/版本采集 |
| Agent 版本、bootId | 主循环版本与本次进程启动身份 → `agentFacts` | 配置显示版本和启动标识摘要；bootId 不推算启动时间 |
| 事实采样时间、最近心跳、Worker 上界/活动数量 | ObservedAt、心跳接收及技术 Worker 统计 → `agentFacts`/节点摘要 | 概览及稳定状态区；不在配置重复。Worker 上界不是 JVM 并发承载保证 |
| 环境检查稳定状态/结果码 | `agentenvironment.Worker` → 控制面保存 → 环境状态/阻断原因 | 状态和原因已表达；当前只返回固定布局核验结论，不包含逐项原始检查输出 |
| 配置摘要、目录摘要、Agent/协议标识和 factsRevision | 已上报/保存，用于一致性、身份和事实绑定 | 浏览器详情未逐字段投影，页面使用其派生状态，不显示为普通资源采集项 |
| 环境检查 checkId、请求/完成时间、检查事实版本 | `ExecutionNodeEnvironmentCheck` 与 SQLite 已保存 | 当前浏览器详情响应没有这些字段；不能拿采样/修改时间代替 |
| 执行额度、预算策略、默认堆、本人/全部任务统计 | 控制面配置、执行分配与完整授权任务聚合 | 页面已表达；这些是控制面配置/计算，不是 Agent 物理资源采集 |

结论：当前公开的节点资源与 Agent 业务摘要已经表达，但不能声称全部内部采集/检查字段或原合同的完整遥测清单都已覆盖。`agent-task-state-contract.md` §7.1 的本次启动时间、活动 execution/lease/epoch/最后事件序号，以及本地待上传事件数量/上传结果摘要尚未进入当前固定心跳。它们在任务事件/日志通道中的已有能力不能外推为节点心跳已提供。

固定节点环境检查的 `ToolRuntimeValidator.Validate` 只验证 Java、Connector/J、OBDUMPER 的受控文件布局，不启动 Java/OBDUMPER读取实际版本；仅将 PASSED/FAILED 与稳定码上报。因此实际 Java/OBLOADER/OBDUMPER 版本、逐项检查证据、工作/日志目录和 Direct Load 配置事实尚未在节点详情采集链实现；登记的工具/Java 路径不代表这些事实已经取得。主机名/IP、CPU 型号/核数等整机库存也不在当前节点采集范围，本次不扩大采集或协议。

主要复核入口：`cmd/agent/main.go` 的主循环、`internal/agenttelemetry/collector.go` 和各平台采样、`internal/agentwire/identity.go` 的 EnvironmentFacts、`internal/agentenvironment/worker.go`、`internal/agentlocalpreflight/probe.go`、`internal/store/types.go`、`internal/controlplane/server.go` 的节点投影，以及 `web/src/api/browser.ts` 的详情模型。

## 首轮验收边界（后续增量见下节）

本轮为本地页面完成及合成验证通过。未替换或重启控制面/Agent 二进制，未进行真实注册、环境检查、资源采集、数据库/存储探测或官方工具运行，也未进行 Linux 原生验证。

当前详情响应不含逐项环境检查证据、实际工具/Java 版本和检查完成时间，页面没有用配置、示例或资源时间补造。节点事件、审计记录和完整日志不在本轮接口范围，未预建空区。环境请求仍使用原来的单次 2500ms 复读，晚于该次复读的完成可手动刷新，不声明持续轮询或推送。

没有执行整体 `verify`，未解决任务地图中既有 Token/平台样式门禁阻断，不提升真实功能或 G3/G4 准入。截图和 CSS 缩放检查不等同原生 200% 系统缩放或完整无障碍认证。


## 诊断链补齐与目录区块删除

> 本节为上一轮事实；用户随后要求把尚未采集的信息全部实现，现役结果见末节“完整节点采集与展现”。

用户明确要求完成“已记录但未下发”和“尚未接入采集链”两组缺项，并删除“导出数据目录与空间”。本轮完成 Agent → 严格心跳 → SQLite 当前事实 → 控制面授权投影 → 浏览器解析 → 详情展示的本地实现；未替换运行中的服务。

| 缺项 | 当前实现与页面位置 | 可信边界 |
| --- | --- | --- |
| 检查标识、状态/结果码、请求/完成时间及检查绑定版本 | 已有 ExecutionNodeEnvironmentCheck 进入详情 environmentCheck；配置页“环境检查记录” | 记录旧通过不代替当前运行环境状态，时间不拿心跳或修改时间补造 |
| Agent/协议标识、factsRevision、两侧配置摘要 | 详情 agentDiagnostics；配置页“Agent 上报事实” | 配置摘要为非秘密一致性事实，列表不增加诊断明细 |
| 本次进程启动时间 | cmd Agent 进程初始化 → 心跳 startedAt → facts_json → 详情 | 不从 bootId/最近心跳反推；旧 Agent 缺项保持未知 |
| Java/OBLOADER/OBDUMPER 安装版本 | agenttelemetry 固定 release/JAR Manifest/Maven 元数据 → runtimeVersions → 详情“安装版本” | 不执行进程、不猜文件/目录名；固定主类关联，缺失、冲突、超限或配置不匹配保持未知。不是运行输出版本认证 |
| 活动 executionId/leaseId/epoch/本地最后事件序号 | 真实 Worker 唯一活动状态 → Agent 装配聚合 → 心跳 → 授权详情 → 概览“活动执行与租约” | 同时匹配数据库当前节点/Agent/租约和任务读取范围。ALL 须可信全局角色与能力；不泄露隐藏项标识/数量，不从空列表证明进程已终止 |
| 本地待确认上传统计与最近结果 | 现有唯一共享日志 Queue 的批次、缺口、同步文件字节及本次进程最近确认/失败结果 → 心跳 → 概览“日志上传队列” | 已脱敏日志队列，非业务事件持久队列；无正文/来源/执行标识/路径/原始错误。fsync 后更新结果，重启积压保留但最近结果 UNKNOWN |
| 详情执行额度和授权任务聚合 | 复用已有预算与任务统计读取能力 | 原详情 HTTP 未接入这两份事实，本轮已连接；失败不填零，额度读取失败同时关闭新任务资格 |

稳定的启动时间、安装版本变动推进事实版本；动态资源、活动序号及队列变化不使检查/预检查失效。新字段在现有 agents.facts_json 保存，沿用幂等和重启恢复，无迁移、新表或无限心跳历史。新增字段被旧控制面拒绝时不清机器身份、不自动重新注册、不降级继续领取。

目录区块与明细已从详情删除；节点登记/编辑、Agent 固定空间采样和概览磁盘最高使用率保留，不删除本机目录或数据。环境配置不重复概览的资源、采样、槽位和相同平台。页面仍只有 useNodeDetail 一份事实，没有新增轮询或状态副本。

实际检查：

- Go 全包 `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` 通过；最后额度失败关闭、稳定时间和时间负例修改后，nodefacts/store/controlplane/cmd Agent 复验通过。
- 同范围 `go vet` 通过；`CGO_ENABLED=0 go build -trimpath ./cmd/...` 本机隔离构建通过，没有安装/替换服务。
- 节点/API 前端单测 140/140 通过；typecheck、全量 lint、Vite build 通过。构建保留既有大块提示，没有新增依赖。
- `audit:business` 正反测试 11/11、0 violations；browser.ts 的规模 REVIEW 已复核：新增内容只负责严格 DTO 解析，不持有业务状态，不反向依赖 workbench；其余既有 REVIEW 未扩成重构。
- 合成 Chrome 详情 9/9 和注册回归 9/9 通过；首轮详情失败来自已删除目录的旧测试断言，修正后通过。检查键盘、迟到/授权失效、缺项与真实零、活动链接、队列状态，以及 1920/1440/1280、640/320 和 CSS 200% 缩放。最终检查标识/完整概览截图另复验一个目标场景。
- Secret scan 和 git diff --check 通过。Browser plugin not available，按 frontend-testing-debugging 使用仓库 Playwright；临时代理绕过只作用于测试子进程。

预览证据位于本轮外部 visualization 目录，最终 overview-telemetry.png 与 configuration-telemetry.png 已人工检查；没有把测试截图或工具输出写入仓库。未进行真实节点版本采集、真实官方工具运行、数据库/存储操作、服务升级或 Linux 原生验证；安装元数据不保证具体包一定具备可识别版本声明，缺项明确展示未知。未实现任意逐项原始检查输出、节点事件/审计/完整日志视图或 Direct Load 库存，不扩展项目范围。未运行整体 verify，既有 Token/平台样式门禁阻断未提升。

## 完整节点采集与展现

同日用户追加“未采集的信息全部实现采集展现”。本节替代前文的缺项结论；范围为现有执行节点契约的本机事实、固定环境检查和既有记录展示，不实现导入业务、远程运维或完整日志中心。原工作区改动保留，未提交或安装服务。

| 本次补齐 | 唯一来源及链路 | 页面与可信边界 |
| --- | --- | --- |
| 主机名、接口地址、原生 OS 名称/版本 | HostSnapshot 与 Windows 原生 API/注册表、Linux 固定系统文件 → 严格心跳 → facts_json | 配置“机器与系统事实”；地址只显示 /24 或 /64 网段摘要，不做 DNS/网络探测 |
| CPU 型号、逻辑 CPU、物理核数 | 本机原生拓扑/固定 cpuinfo；物理值不可取得时省略 | 与使用率分开；逻辑 CPU 为 Agent 可见数量，不推算物理核数、并发或预算 |
| Java/OBLOADER/OBDUMPER 实际版本与可用性 | 当前用户检查许可 → 固定版本进程 → 完整回执 → 原子保存 | 与安装元数据分开；共用规则限定 Java 8 与工具 4.3.5，自报 PASSED 不得扩大范围 |
| 工具/Connector/J、工作/日志基础目录访问 | LocalInspector 固定十项；目录用私有临时文件核对基础读写并清理 | 逐项显示已取得的存在/可读/可写/可执行事实、路径和稳定码；不扫描任务目录或导出数据根 |
| Direct Load 安全配置 | 固定 conf/session.config.json，64 KiB 上限、重复键拒绝，未知值不外传 | 三个非负毫秒值、真实零、未知键存在标志及安全投影指纹；没有密码/未知键名/整文件指纹 |
| 检查来源与完整证据 | 用户/自动请求审计绑定 checkId 和许可，回执绑定当前配置/事实版本 | 上次清单有明确 Alert；重启/稳定事实变更清除，迟到、漂移、不同回执和越权许可拒绝 |
| 近期任务 | 复用 TaskListQuery 的节点、账号、可信全局读取范围，最多五项 | 概览只返回类型、状态、时间和授权链接，不附数据库/表/SQL/输出内容 |
| 已保存节点事件及操作 | 复用 audit_events 节点动作白名单，事件/操作各五项，来源另按 checkId 查询 | 概览事件、配置操作；无账号或审计差异，大量编辑不挤掉事件和检查来源 |

Direct Load 三个键和单位按 [OceanBase 导数工具 V4.3.5 配置说明](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997310) 核对：task_timeout_ms、heartbeat_timeout_ms、heartbeat_interval_ms。不沿用其他版本的微秒键、默认值或扩展键。工具 `Version:` 解析依据已有[4.3.5 包身份取证](tool-package-baseline.md#2-工具身份)，本轮未重新运行真实包。

实现所有者：nodefacts 为有界模型/校验；agenttelemetry/host*.go 为原生事实；agentenvironment/inspection.go 为固定检查与安全配置；agentexec/version_probe.go 为受控版本进程；store/node_inspection_audit.go 为当前请求许可；store 完成事务保存清单；store/node_activity.go 读取既有记录；controlplane/node_diagnostics.go 负责权限/网段投影。前端 nodeRuntimeFacts.ts 只解析传输，useNodeDetail 唯一持有事实和串行复读，两个分区及 NodeInspectionDetails 只消费事实。没有新依赖、表或迁移。

版本进程使用私有临时目录、最小固定环境、16/64 MiB 堆、每进程 8 秒/总检查 20 秒、16 KiB 输出上限和平台进程树监管。只接受三个固定种类，不经过 Shell、解析秘密或连接数据库；原始输出有界解析后清零。自动检查没有版本启动许可，必需版本未取得时保持未知/失败关闭。页面先 2500ms 延迟读取，在线且 pending 时串行继续，最多两分钟；完成、离线、读取失败或会话失效停止，自动读取不发布手动刷新成功反馈。

本轮实际检查：

- Go 全包测试通过；最终权限、来源窗口和版本边界变更后，受影响包全量/定向复验通过。编译 Go 测试程序充当假 Java/工具，覆盖三种查询、超时、超限、冲突/非法输出、非法选择器和临时目录清理，未运行真实 Java/官方工具。
- go vet 通过；CGO_ENABLED=0、trimpath 的 Windows AMD64 与 Linux AMD64/ARM64 隔离构建通过，交叉构建不证明麒麟原生可用。
- 节点/API Vitest 146/146；覆盖慢检查串行完成、两分钟上限、离线/切换/卸载、版本边界、脱敏和真实零。typecheck、全前端 Lint、Vite build 通过，保留既有大块提示，不扩成包拆分。
- 完整详情 Chrome 10/10，最终检查流程和完整新增事实 2/2 复验通过。覆盖全部十项、系统/CPU/网段、实际版本/Direct Load、近期记录/链接、旧 Agent 缺项、上次清单、窄屏与 CSS 200% 缩放。外部 all-facts 配置截图已人工查看，最终截图保存在本次 visualization 目录，不提交图片/Trace。
- 业务审计正反测试 11/11、0 violations、7 REVIEW。新增 nodes/tasks 依赖仅复用无状态 taskStateLabel；API 为严格传输校验，不持有业务状态或反向依赖。其余既有 REVIEW 不扩成无关重构。
- 秘密扫描和 git diff --check 通过。Browser 插件不可用，沿用锁定 Playwright/Chrome；NO_PROXY 只作用于测试子进程的回环地址。

完成边界：本地采集、协议、持久化、授权 API、页面和合成验证完成；未安装/升级控制面或 Agent，未进行真实节点完整采集链、真实工具/数据库/存储验证或 Linux 原生运行。旧 Agent 缺字段保持未知，新采集须协调升级两端并由当前用户检查取得实际版本。节点事件仅覆盖既有持久化记录，当前在线仍由心跳投影，不声称采集历史上下线。未执行整体 verify，既有 Token/平台样式门禁和真实阶段准入未提升。

## 核心详情精简与平台路径修正

用户后续明确取消折叠，要求环境检查进入概览、日志上传队列进入配置，并只保留重点直观指标。本节覆盖前文页面展示范围，不撤销此前授权的采集链。

- 返回执行节点与唯一下载 Agent 入口集中在对象头右上角，刷新、编辑及现有唯一主操作同组。技术节点 ID 从详情隐藏。
- 运行概览保留 CPU/内存/磁盘、执行额度、授权任务统计和任务中心入口，完整十项“环境检查”直接可见。只展示名称、结果、可选标识、异常说明和检查时间；上次清单提醒继续保留，路径/访问布尔值/稳定码不进入 DOM。
- 环境与配置采用两列执行环境与工具/内存，底部整行日志上传队列。只保留主机/系统/核数/Agent 版本与启动、实际或明确标注来源的工具版本、安装根/Java 路径、默认内存/两项预留，以及上传结果/日志份数/缓存占用/时间。已在概览展示的资源、采样、槽位和相同平台不重复；缺项与真实零分开，物理核数缺失时明确显示逻辑核心。
- 移除活动执行与租约、近期任务、最近节点事件、Direct Load 参数摘要、环境检查记录、导出目录与空间及最近操作。技术身份/协议/哈希/事实修订/检查绑定、网段和 CPU 型号隐藏。两个 Tab 和关联材料/阻断原因均没有折叠入口，登记抽屉的原预算行为不在本次详情范围。
- 底层 API、采集、授权、租约和日志队列唯一所有者继续保留；只读分区不复制事实或补读请求。没有新依赖、数据表、迁移、全局 token 或 Shell 改动。

只读核对本机 4.3.5 包：bin 中有 Linux 启动文件，Windows 的两项 `.bat` 在 bin/windows。发现新固定检查错误查找 bin/*.bat，而原任务预检查已使用 bin/windows；现统一调用 `agentlocalpreflight.ToolLauncherPath`，两种工具均从安装根按平台派生。拒绝非法平台/工具/相对根，错误层级夹具明确缺文件。表单提示仍要求 bin/lib/conf 安装根，不能把 Windows 脚本目录当根；未调用真实 Java、OBLOADER 或 OBDUMPER。

实际验证：

| 检查 | 结果与范围 |
| --- | --- |
| Go 测试 | `go test ./internal/agentlocalpreflight ./internal/agentenvironment ./cmd/agent` 通过；覆盖两工具/三平台、非法输入、错层及固定检查负例 |
| Go 静态检查 | 同三包 `go vet` 通过 |
| 隔离构建 | CGO_ENABLED=0、trimpath 的 Windows cmd 全包，以及 Linux AMD64/ARM64 Agent 构建通过；本轮产生的临时 ELF 已删除 |
| 前端单元 | 节点与 nodeDiagnostics/nodeRuntimeFacts 82/82 通过 |
| Chrome 合成回归 | 详情和注册共 19/19 通过；取消残余折叠后复验，覆盖 1920/1440/1280/640/320、200% CSS、未知/零、数据存在时隐藏、默认 Tab、键盘、离线、写入、A→B→A 与授权撤销 |
| 类型、代码风格与生产构建 | typecheck、ESLint、Vite build 通过；保留既有大块提示 |
| 业务架构 | 正反测试 11/11、0 violations、6 个既有 REVIEW；只读 UI 无新增状态所有者，移除概览任务行后不再依赖 tasks 展示规则 |
| 秘密与差异检查 | `scripts/check-secrets.ps1` 和 `git diff --check` 通过；既有换行格式提示不影响检查结果 |

人工查看最终 1440 概览与配置截图，布局、指标和检查位置符合最新要求；合成截图位于本轮仓库外 visualization/node-detail/final-core，未提交图片/Trace。浏览器沿用仓库锁定 Playwright/Chrome，回环 NO_PROXY 只作用于测试子进程。

交付边界：本地源码、页面与合成验证完成，未提交、推送或替换运行中的控制面/Agent；新采集与路径修正需使用当前源码构建的两端。未做真实节点采集、真实工具/数据库/存储或 Linux 原生验证，交叉构建不提升支持结论；未运行整体 verify，不提升既有阶段门禁。

## 人工环境检查被心跳覆盖的修正

用户报告“工具运行时暂无法核验”。只读核对当前控制面 SQLite 检查记录、类型化审计和逐项回执，确认人工请求带版本查询许可，下一次心跳却将它替换为不带许可的自动请求。Java、OBDUMPER、OBLOADER 均为 UNKNOWN / VERSION_PROBE_NOT_REQUESTED；其余七项通过。工具目录仍是安装根，Windows bin/windows 的两项启动文件存在，平台布局已通过。本次未执行真实 Java/官方工具或发起新的环境检查。

根因是人工请求把 environment_check_facts_revision 清空，自动续接把空值当作不同机器事实，即使环境不变也更换 checkId。先在既有检查测试补入实际“人工请求 → 心跳 → 自动续接核对 → 领取”的顺序，测试明确失败于人工检查被替换；原测试绕过了心跳续接，未覆盖该场景。

修正 RequestExecutionNodeEnvironmentCheck 的同一事务，绑定当前 ACTIVE Agent 事实版本，使同版本心跳保留原检查标识与服务端许可。完成回执增加已绑定版本核对；新增重启负例覆盖自动续接前的新旧回执均拒绝，随后重新排队的自动检查不继承人工许可。配置漂移、账号许可、幂等回执及自动检查失败关闭仍沿用原边界，无新 API、依赖、表或迁移，不改真实数据。

实际验证入口为 internal/store/node_inspection_test.go 的人工领取与事实漂移场景，以及 store/controlplane/agentenvironment/cmd Agent 的既有回归。运行实例的修复须由当前源码重建控制面后生效；此次没有替换二进制、重启服务、重新注册或改写用户检查记录。网页通过前仍保持禁用，通过后需显式启用。

最终检查：`go test ./internal/store ./internal/controlplane ./internal/agentenvironment ./cmd/agent` 全部通过；同范围 `go vet`、`CGO_ENABLED=0 go build -trimpath ./cmd/...`、秘密扫描和定向 `git diff --check` 通过。首次新增领取顺序回归在修改实现前确实失败，修正后通过；重启漂移、自动不继承许可及已有过期/伪造回执负例同样通过。构建使用隔离路径规则，没有安装到当前服务位置，也未执行真实版本查询。

## 环境检查报错解释优化

按用户要求，把笼统“工具运行时暂无法核验”改为检查项、原因与处理建议。`nodeInspectionPresentation.ts` 是唯一只读文案规则：未通过与未确认分开；文件缺失、无法读写/执行、版本不兼容、未执行查询、超时和缺少有效版本输出分别解释。已取得的不兼容版本可与 Java 8 / 工具 4.3.5 基线对照；通过项不增加建议，原始码、路径、访问布尔值、哈希及身份字段仍隐藏。

顶部只使用当前异常检查且 checkId 匹配的完整清单，点名未通过/没有结果的必需项。可选项不进入阻断摘要，受影响可选行明确“不影响当前导出环境检查”。上次清单标明不能代表本次结果；空明细区分正在等待、失败未返回和旧检查未提供，等待提示允许刷新查看，不无条件承诺自动更新。解释不重算服务端接收资格，不增加检查、请求或状态副本，不修改 API、Agent、数据库或检查行为。

本轮验证：

- 节点及 nodeDiagnostics/nodeRuntimeFacts 单元测试 87/87；新增行为负例覆盖权限建议、实际/期望版本、未执行与超时区别、可选项不阻断、旧清单/过期事实不冒充当前原因、未知码不回显及通过项不制造异常。
- 详情与节点页合成 Chrome 14/14；新增两项覆盖原因/建议、版本未查询、超时、无有效输出、旧结果、失败空明细与可选项影响。新测试首轮因可选标题附带“（可选）”而精确定位失败，核对实际 DOM 后改为完整可见标题，复验全部通过，没有改产品来迎合断言。等待提示最终更正后，现有核心场景定向复验 1/1 通过。
- typecheck、全前端 Lint、Vite build 通过；保留既有大包提示。业务架构正反测试 11/11、0 violations、6 个既有 REVIEW。展示规则不拥有会话/事实，不新增跨层依赖；列表保留一参数通用解释，详情可传当前清单上下文。
- 秘密扫描与定向差异检查通过。使用仓库锁定 Playwright/Chrome；Browser plugin not available，NO_PROXY 只作用于回环测试子进程，无 API 代理或真实数据库/工具操作。

人工查看最终错误与未确认两类 1440 局部截图；桌面、640 窄屏和 200% CSS 无横向溢出，原因/建议可读，框架与 console/pageerror 无相关错误。截图位于本轮仓库外 visualization/node-detail/inspection-errors，交付预览为 inspection-errors-final.png 与 inspection-unconfirmed-final.png，不提交图片、Trace 或测试输出。

交付为本地前端与合成验证；没有重启/替换控制面或 Agent、发起真实检查、运行真实工具或提升既有门禁。此前人工检查被心跳覆盖的运行修复仍以控制面采用上一节代码为前提，优化文案不代替更新后真实检查结果。

## 两平台注解帮助补齐

按用户要求补齐两种平台的帮助，帮助中不列具体发行版名称或系统版本。`nodePlatformHelp` 是唯一只读帮助规则，注册/编辑从 `form.platform` 派生，详情从当前节点的平台派生，不增加平台或路径状态副本。

- 字段帮助覆盖架构选择、完整工具安装根目录、实际启动文件层级、Java 8 完整路径、导出目录格式和 Agent 运行账号权限。两种平台切换同步更新说明与占位示例，保留已填路径；Linux Java 与安装帮助同步当前 x86_64/aarch64 分类。
- 首次关联区域和一次性注册码弹层分别说明管理员启动 `启动.cmd`、root/sudo 启动 `start.sh`，沿用包内固定入口与菜单。下载映射、平台协议、注册码签发/清理和节点资格保持原行为；帮助不扩大正式支持矩阵。
- 架构字段用 Form.Item 分组关联可访问描述，避免当前 Ant Select 将描述属性仅留在外层容器。浏览器同时验证帮助描述和实际 combobox 的包含关系，未修改依赖或通过 DOM 写入补丁。
- 运维手册新增[构建与系统兼容性说明](../deployment-operations.md#agent-构建与系统兼容性)：按 Go 官方最低要求解释内核基线与发行版主版本的区别，安装入口另需 systemd；Java/工具和目标平台验证仍单独判断。未把 Go 通用最低要求作为项目正式认证。

本轮实际检查：

| 检查 | 结果 |
| --- | --- |
| 节点 Vitest | `npm run test -- src/workbench/nodes`，71/71 通过 |
| Chrome 合成页面 | 最终 `execution-node-registration.spec.ts` + `execution-node-page.spec.ts`，12/12 通过；覆盖两种 Linux 架构、平台帮助切换/已填值保留/无发行版名称、下载目标、独立材料签发与清理，以及抽屉原生命周期 |
| 响应式与人工查看 | 1920/1440/1280/640/320 与 200% CSS 检查通过；核对两套 1440 帮助截图及 640/320 抽屉，正文可滚动、底栏可达、长说明可换行 |
| 前端静态与构建 | typecheck、受影响源码/浏览器测试 ESLint、最终 Vite build 通过；保留既有大 chunk 提示 |
| 业务架构 | 审计正反测试 11/11，0 violations、6 个既有 REVIEW；帮助只读派生，不新增事实/会话所有者、请求或跨层依赖 |
| 秘密与差异 | 秘密扫描及差异空白检查通过 |

浏览器首轮识别描述未传入内部输入，改为字段分组后按标签与 combobox 角色定位，保留可访问描述断言；另一次 Trace 确认本地资源请求遭遇 `ERR_NETWORK_CHANGED`，后续完整复验通过。没有增加重试、放宽断言或修改产品来掩盖加载失败。使用锁定 Playwright/Chrome，Browser plugin not available；测试只运行隔离 Vite 并接管合成 API，不触及开发控制面或真实节点。

最终截图在本轮仓库外 visualization/node-detail/platform-help-complete，两个平台预览为 node-help-windows.png / node-help-linux.png；图片、Trace 和测试输出不提交。页面与文档为本地完成，未安装/替换二进制、进行真实注册/工具/数据库操作或目标机器原生验证，既有准入与整体门禁不提升。
