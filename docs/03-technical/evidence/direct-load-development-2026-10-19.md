# 旁路导入开发与验证 · 2026-10-19

## 实现与交付边界

本轮基于用户“继续完成旁路导入模块”的开发请求，在既有导出、普通导入之后实现独立旁路源码闭环，没有提交、推送、创建 PR、替换现役二进制、升级现役 SQLite 或修改运行开关。开始时已有大量未提交的任务/普通导入/UI 工作；这些变更保留，不回退或批量清理。项目规则、任务地图、旁路产品/参数规则、API/状态/安全/启动/日志契约与现行 P0/P1 为依据。

主要入口为 `internal/directload`、`internal/directloadfiles`、独立 `parammeta/direct` 与 `commandgen.NewDirectLoad`，浏览器/控制面/Agent 均按 `OBLOADER_DIRECT_LOAD` 隔离。迁移 `0034` 前向扩展类型与预检安全配置指纹；草稿、修订、幂等、九项证明、单表清单、当前引用和最终风险确认共同冻结任务。配置复用与参数只读从头执行使用现役派生协议，不开放旁路模板或检查点。无权对象、列表数量、任务快照路径仍遵守平台范围，路径仅创建者可见。

独立探针与执行开关默认关闭。代码包含显式旁路 claim/ack/renew/secret/event、短生命周期业务槽位、输入与三键配置指纹的秘密前/启动前复验，以及固定主类与 JAR 的独立 argv 校验。成功必须结合受控进程、匹配目标表的服务端提交标记及完整日志；普通导入 finished、写入批次、跳过提交、单独退出 0 不足。已观察提交后发生其他失败仍保留观测；已启动后的失败要求人工核对，不宣称未写入或自动回滚。

## 受控进程与结果合成检查

`internal/executionargv/direct_load_test.go` 覆盖固定旁路参数正例及冻结配置漂移、重复/未知/秘密/sys/普通导入/SQL/恢复参数、非法 RPC/线程数、确认未配对；`internal/agentexec/direct_load_test.go` 覆盖未知包不能按文件名宣称版本、预算/取消/缺失以及固定主 JAR 优先且移除当前目录。固定摘要读取核对打开句柄和前后文件身份、大小/时间，拒绝祖先链接；没有运行该 JAR。

`internal/agentexecution/direct_load_test.go` 覆盖同表提交后固定退出的正例、普通完成与批次成功不能替代提交、缺退出/倒序/错误表/错误来源或解析器/不完整或截断日志/固定失败及跳过/非零退出；合法 `failed_orders`、`failure_records` 不按裸词误判。结果负例覆盖进程等待错误、上传缺口、取消、保留已观察提交及四/五字段独立结果形状；配置投影变化、零值、缺项、未知键与文件缺失阻断启动，普通 Worker 不能确认旁路信封。测试未启动官方工具。

`internal/agentworker/direct_load_secret_test.go` 验证 FILES 无论开关均不能解析秘密、未开启专用探针不解析、当前完整数据库检查只解析一次；安装配置和 featuregate 的默认/错误/不完整许可负例验证旁路不继承其他类型开关。

最终 `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` 与 `go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...` 全部通过，`gofmt -l cmd contracts internal migrations` 无输出。Windows AMD64、Linux AMD64、Linux ARM64 均以 `CGO_ENABLED=0 go build -trimpath ./cmd/...` 构建通过。构建无安装或服务停止，不证明目标 Linux 原生运行。秘密扫描与 `git diff --check` 已通过，换行格式提示不代表差异错误。

## 尚未完成的真实验收

本轮没有真实数据库/存储/RPC 连接、真实秘密解析、官方工具启动或用户数据库操作。Java 与 TCP 正例均为合成代理/隔离回环；静态 JAR 检查只读取既有工具文件，不调用其 main。独立开关没有被实际开启，运行实例仍需另行按授权范围部署。

DLV 真实验收仍需非生产目标与明确操作授权、SQL/RPC 端点、OBServer/ODP 精确版本、最小权限、单表格式夹具及前后/服务端提交事实。当前没有充分已确认的业务账户 ODP 精确版本查询，ODP 场景版本检查保持 UNKNOWN；Oracle 权限/结构未核实也保持 UNKNOWN；现有数据源登记没有某场景候选时不伪造支持。阶段百分比及运行中取消未核验，分别保持未知/禁用。上述事实不因源码、页面或合成检查通过而提升为真实可用，DLV/G3/G4 状态仍按任务地图。

## 前端与交互验证

旁路实现为独立六步 Task Builder，入口为 `web/src/views/DirectLoadWizardView.vue`，业务能力和唯一状态所有者登记于 `web/src/workbench/import/direct-load/README.md`。表单、引用读取、草稿修订和异步复验分别持有自身状态；步骤消费同一模型，不复制普通导入规则。任务中心的新建菜单、筛选、草稿恢复/删除、任务类型和独立六组快照已接入；模板仍只接受导出和普通导入，旁路不接受检查点继续、取消或普通导入进度语义。

前五步按当前输入验证，连接、RPC、版本、单表文件、权限、结构和节点配置在第六步自动保存后统一检查。该顺序用于收缩既有设计中节点/目标表尚未填写却要求前置检查的时序冲突；页面始终显示待检查或无法确认，没有人工通过路径。提交同时要求草稿、命令和九项完整预检查允许执行、相同配置指纹和修订、证据未过期、节点三键安全配置指纹、任务名称与最终风险确认；独立执行策略默认关闭。合成开放策略只证明页面事务能正确提交，不能证明真实旁路工具或数据库可用。

实际检查结果：

- `npm test`：47 个文件、564 项通过，覆盖真实 103 参数目录、canonical `--longName`、服务端省略空值、配置污染、九项证据和秘密拒绝、场景/格式/单表/路径、保存期间编辑、防重、A → B → A、卸载迟到、默认门禁、最终名称与风险确认。旁路派生来源与锁定漂移、只读原配置重绑、统一三类草稿和旁路六组快照均有负例。检查原因码限于旁路六个专用码；缺少、污染或错误附带节点配置指纹均拒绝。
- `npm run typecheck`、全量 `npm run lint`、`npm run build` 通过；构建仍有原有大包体提示，没有新增依赖。
- `npm run audit:wizards`：0 违规、4 个既有导出 REVIEW。审计实际覆盖 `workbench/import/` 和 `DirectLoadWizardView.vue`；旁路业务没有反向依赖页面或步骤创建 API 客户端。
- `npm run audit:business`：0 违规、13 个既有 REVIEW；本轮没有新增跨层依赖或循环值依赖。
- `go test ./contracts/...`：旁路九项/六码、三键配置摘要、DATA-only 清单、派生来源/锁、独立默认关闭、开启后 201 响应及模板双类型边界均通过。
- Chrome 合成 API 共验证 8 个旁路独立场景：完整六步/保存刷新/未保存离开、1440 与 1280 工作面及键盘弹层、场景格式阻断、无权失败关闭、合成独立策略开启后的最终提交、RERUN 只读复验与 REBUILD 可编辑、原生 200% 缩放。另有普通导入独立路由与任务中心新建菜单 2 项回归通过。原生缩放验证了 Drawer 焦点约束与返回焦点、完整命令不被固定底栏遮挡、提交按钮可达及页面无横向溢出。

浏览器截图保留在独立的 `artifacts/direct-load-ui/`，后续定向 Playwright 运行使用专属 outputDir，避免清除其他任务证据。下列截图均已人工检查；主任务另行复核了 1920、1280 和原生缩放布局。

| 证据 | 文件 |
| --- | --- |
| 1920 预检查与默认门禁 | `artifacts/direct-load-ui/direct-load-1920-review.png` |
| 1440 表与格式、预检查 | `artifacts/direct-load-ui/direct-load-1440-target.png`、`direct-load-1440-review.png` |
| 1280 表与格式、预检查 | `artifacts/direct-load-ui/direct-load-1280-target.png`、`direct-load-1280-review.png` |
| 任务摘要 | `artifacts/direct-load-ui/direct-load-summary.png` |
| 原生 200% 完整命令与可达操作 | `artifacts/direct-load-ui/direct-load-native-200.png` |
| 合成开启策略后的最终名称与风险确认 | `artifacts/direct-load-ui/direct-load-submit-confirmation.png` |

本轮浏览器全部使用隔离 Vite、合成身份/端点/文件事实；没有浏览器真实工具启动、用户数据库操作、凭据解析或实际 SQL/RPC 探测。旁路阶段进度保持未知，尚未启用普通导入解析器。真实验证与部署状态不由这些合成结果推定。

## 专用只读探针与安全配置绑定

新增独立 `DirectLoadProbe` 固定 Java 入口及 v11 有界 stdin 协议，复用既有私有凭据工作区、固定 JDBC 驱动和受控探针启动基础；不调用普通导入业务探针，不接收任意 SQL、驱动、URL、主类或 RPC 协议指令。MySQL 只使用固定版本、权限与单表元信息读取，按空表、BIT、二进制、虚拟列、索引和 LOB 分别保留事实；权限不通过试写确认。手工 RPC 端口仅检查同一业务 Host 的 TCP 建连，不证明协议、认证或导入成功。

本机工具版本只来自固定官方包静态指纹；三个允许毫秒数值及观测时间来自固定 `session.config.json` 安全读取，Java 输出不能提供或覆盖这些本机事实。配置观测独立于数据库成功，完整租约、草稿、节点、凭据、根目录或输入清单变化后不复用缓存。通过配置项时才输出 `directLoadConfigFingerprint`；缺失、非法或迟到指纹使该项 `UNKNOWN`，文件发现及配置未通过报告禁止携带该字段。该安全指纹进入冻结快照及执行 envelope，Worker 启动前再读并匹配；不保存原文件、未知键值或整文件哈希。

本轮验证结果：

- Java 8 编译、内嵌 JAR 打包及构建脚本列出的 8 个 Java 合成测试入口通过；旁路使用代理 JDBC 拒绝未列明 SQL、DML 和提交，覆盖权限、未知结构、版本、协议截断/重复/越界、秘密清零等负例。内嵌探针 JAR SHA256 为 `adf44e96027c37140e2802e42600ff34f5d5acc0445d4fce274cc5bf25867098`。
- `jdbcprobe`、`agentjdbc`、`agentlocalpreflight`、`agentpreflight`、`agentwire`、`agentworker` 的定向 Go 测试与静态检查通过，包含专用端口默认关闭、一次性秘密解析、取消/并发、跨租约与配置绑定、Java 伪造本机事实、数据库失败仍保留独立本机配置、完成协议指纹及跨模块夹带指纹负例。网络夹具仅使用隔离本地 TCP 或内存连接。
- 修复持久日志缺口重读遗漏旁路解析器来源的问题；`logstream` 测试与静态检查通过，实际临时 JSONL 重读保留 stdout/stderr 身份，未知或相似解析器不能提升为 OBLOADER 来源。长任务续期路径不重新套用预检查 TTL；该结论来自当前调用链审阅，不替代真实长任务验证。

仍阻断的事实：ODP 业务槽位版本读取及 Oracle 旁路权限/结构精确契约未核验，探针保留未知；公开资料中的 OMS 系统视图不能作为本任务绑定的 OBLOADER 服务端提交证明。固定官方 JAR 的 `commitAsync → await → isSuccess` 与提交日志模板静态取证、以及 Worker 合成终态测试只证明源码判定边界，未执行真实 SQL/RPC、真实凭据解析或官方 Direct Load。真实验收仍需按 DLV 核对目标表数据、可得服务端状态和完整受控运行证据，不能由客户端日志及退出码单独提升准入。

## 控制面、仓储与租约协议合成验证

`internal/store/direct_load*_test.go`、`internal/controlplane/direct_load*_test.go` 与 `internal/agentwire/direct_load_execution_test.go` 使用临时 SQLite、合成秘密、内存事实与隔离 TLS 服务；没有读取现役数据库、应用迁移、启动官方工具或解析真实凭据。

已通过的定向检查：

- `go test ./internal/store ./internal/controlplane ./internal/agentwire -run '旁路|RenewExecutionLeaseKeepsCurrentAgentAndEpoch' -count=1`：三个包分别通过，覆盖独立 CRUD、修订、幂等、权限、固定文件发现/九项检查、提交、领取、确认、业务秘密槽位及事件投影。
- 最后补充 `go test ./internal/store -run '旁路显式|旁路提交事实|RenewExecutionLeaseKeepsCurrentAgentAndEpoch' -count=1` 通过（7.233 秒），包含续租安全修复和独立结果矩阵。
- 责任文件的 `gofmt -l` 无输出；`git diff --check` 通过。最终全量检查由主任务单独记录，不由上述定向结果替代。

失败关闭证据包括：

- 原始配置与冻结快照拒绝未知键、重复键、大小写别名及 `null`，覆盖嵌套配置、入口确认、清单及九项证明；冻结元数据、能力、检查身份、修订、指纹和 DATA-only 唯一表清单须完全匹配，不能借用普通导入或导出结果。
- 派生仅通过现役 `:rebuild-draft` 的 `REBUILD_FROM_CONFIG`/`RERUN_FROM_SCRATCH`，来源任务须处于可信终态且证明身份一致。RERUN 可对相同活动配置 PATCH 重绑当前事实，任何配置值改变均拒绝；新草稿不复制原预检查、输入清单、任务名称或风险确认。提交再次核对来源关系与本草稿的新完整证明。
- 已提交草稿不再允许编辑或删除，避免破坏历史任务的预检查证明；统一草稿列表排除已提交草稿，三类列表/六组快照保留类型和表名，非创建者看不到路径。旁路不取得导出检查点、模板或取消资格。
- `probe=false/execution=true` 的错误装配仍不能提交或领取；文件发现永不解析秘密，完整旁路预检查只在独立探针开关开启时解析业务槽位，不读取 sys 或存储秘密。缺少、非法或不匹配的三键安全投影指纹不能冻结或领取任务，该字段与配置/清单一起进入执行信封。
- 续租必须带控制面时间及当前冻结信封摘要，且当前租约尚未过期；错误摘要、错误 Agent、已过期但尚未扫描以及旧请求重放均不能续活。已领取的合法执行不会因提交前预检查有效期过去而失去续租资格。
- 旁路只接受独立四/五字段结果，不接受普通导入标记、导出文件计数或检查点。失败事实可保留已观察到的服务端提交；任何 `PROCESS_STARTED` 后失败终态均标记核对，未观察到提交不能解释为未写入。

机器预检查 OpenAPI 保留原导出与独立旁路的闭合类型分支，并将检查集与执行上下文关联约束；FILES 固定一项，PREFLIGHT 固定九项及其顺序。契约测试同时核对独立结果引用与输入清单边界，跨类型或跨用途混合响应不能满足该契约；最终 `go test ./contracts/...` 及上述全量检查通过。

本轮现役数据源与秘密槽位保持 ODP-only；三场景参数生成矩阵不表示三场景 JDBC 均已可执行。PUBLIC_CLOUD/OBSERVER_DIRECT 缺少已授权源事实、ODP 精确版本或 Oracle 结构/权限未知时继续阻断，未扩展数据源 CRUD 或推定真实支持。

## 跨模块视觉一致性专项验收

本节保留初轮审查事实；后续整改与当前复验结论见下方“跨模块整改与水平进度栏复验”。

**结论：不通过。** 用户要求按现行跨模块一致性清单验收后，本次对导出、普通导入、旁路导入进行同版本浏览器对照。此前旁路单页、功能与合成流程检查不能代替该专项验收；当前不能声明旁路前端视觉验收通过或前端交付完成。本节仅审计和登记问题，没有修复产品代码、修改设计权威、安装服务或进行真实执行。

### 基准与版本标识

- 唯一八项清单及判定规则：[MASTER 视觉验收门槛](../../../design-system/MASTER.md#视觉验收门槛)。比较依据还包括当前 DESIGN、Task Builder、Drawer、Table、Action Hierarchy、Status Feedback Pattern，以及数据源定版复用规则；实现本身不作为规范。
- 比较路由：`/exports/new`、`/imports/normal/new`、`/imports/direct/new`；从当前工作树启动隔离 Vite `127.0.0.1:15175`，全部 `/api/v1/**` 使用合成响应。独立合成执行许可仅用于展示最终确认，不启动任务、工具或数据库操作。
- HEAD 为 `0c5953cdbf406c01ee89ac9018af90a123d67b64`，但工作树包含既有未提交及未跟踪实现，因此不以 HEAD 单独代表验收版本。[源码清单](../../../artifacts/cross-module-visual-20261019/source-manifest.json)记录 255 个前端、设计和锁定配置文件的 SHA256；清单 SHA256 为 `3e54e0c896e23792ff9c90eec3fa63cce87aa75ca86a295dcf1ddc5dcaf5733b`。[哈希复核](../../../artifacts/cross-module-visual-20261019/source-verification.json)未发现内容或文件集合变化。
- 原始截图、computed style、几何、交互和请求记录分别保留于 `artifacts/cross-module-visual-20261019/screenshots/`、`metrics/`；`compare-*` 是原图等比拼接，仅便于目视对照，不替代原图。

### 八项清单结论

| 验收项 | 结论 | 实际依据 |
| --- | --- | --- |
| 同类型页面统一布局和对齐 | 不通过 | 共同摘要事实距 Drawer body 左边分别为 24 / 24 / 64px，旁路产生额外嵌套留白，见 V-02。已登记的横向/竖向步骤栏差异不计缺陷。 |
| 同类控件统一尺寸、样式和状态 | 不通过 | 表内操作导出/普通 27px、旁路 36px；末两项底栏操作间隔为 8 / 8 / 12px，见 V-04。未将这些数值外推成表行被撑高。 |
| 表格、表单、抽屉、弹窗视觉一致 | 不通过 | 预检表头三类均为 44px、目录 Drawer 均为 720px、摘要均为 640px；但摘要内部留白与事实排版分叉，见 V-02、V-03。 |
| 同类操作动效和反馈一致 | 不通过 | 旁路正常返回 FAILED、UNKNOWN 及已过期结果时总览均使用蓝色 info，未按领域结果表达阻断严重性，见 V-01。弹层、保存和取消交互的已测结果另列，不以某一子项通过覆盖总览问题。 |
| 模块间无明显密度落差 | 不通过 | 旁路摘要事实区仅 512px，导出/普通为 592px；旁路使用更小标签、单列和逐项横线，见 V-02、V-03。未以不同业务字段数量或总高度判定密度。 |
| 1920×1080 实际截图检查通过 | 不通过 | 已采集并目视三模块来源、字段、预检、摘要、目录和确认；V-01 至 V-04 在当前浏览器截图及量测中成立。 |
| 1440×1024、1280×720 响应式检查通过 | 不通过 | 两档均已实际采集并目视，V-02、V-03 的共同摘要偏差仍复现；P0 同时要求的原生 200% 矩阵另发现普通导入确认弹层初始安全返回按钮大部分处于视口下方，见 V-06。1280 / 200% 的末字段、命令滚动与焦点约束子项通过，不覆盖该可见性问题。 |
| 无无依据的模块私有样式或重复实现 | 不通过 | 旁路摘要的额外 padding 与旧事实排版未找到 P0/P1 例外依据。私有业务目录与字段布局本身不据此判违规；静态 REVIEW 的限定人工处置见下。 |

### 已确认问题与最小收敛范围

V-01 至 V-06 均判定为 implementation debt；已登记的六步导航、参数目录和业务风险差异属于业务约束。本次未确认 MASTER defect，不修改 P0/P1。

| 编号 | 优先级 | 问题、影响与证据 | 最小修复建议（本次未实施） |
| --- | --- | --- | --- |
| V-01 | P1 | `DirectLoadReviewStep.vue:16` 仅将未过期 SUCCEEDED 映射为 success，其他全部为 info。实际 FAILED 行已红色“未通过”，总览仍为蓝色“尚未全部确认”；UNKNOWN 和过期也相同，弱化阻断反馈。见 `1920-direct-state-precheck-{failed,unknown,expired}.png/json` 与导出对应状态。依据 Task Builder 的领域总览及 Status Feedback。 | 从当前领域状态和过期事实计算总览严重性，保留独立执行门禁说明，不混成请求失败或修改业务资格。 |
| V-02 | P2 | `WizardFrame.vue:51` 仅 horizontal 分支清除 Ant Drawer body padding；共享 `.orch-task-summary` 已有 24px，旁路 `directLoadWizard.css:10` 再加 16px。浏览器首事实 inset 为导出/普通 24px、旁路 64px，旁路可读事实宽度 512px，对照为 592px。见 `1920-{export,normal,direct}-summary.json`、三档摘要原图及 `compare-1920-summary-detail.png`。 | 共同摘要仅保留一层正文留白；保留旁路字段与业务分组。 |
| V-03 | P2 | `DirectLoadSummary.vue:6` 复用旧 `summary-definition`：单列、逐项横线，标签 12px/13.8px 行高；对照摘要使用分组和短事实双列，标签 13px/20px。旁路组标题 computed 为 16.38px/500，对照 16px/600。此处是共同信息呈现规则分叉，不是六步业务差异。 | 沿用现役共同摘要排版规则，取消无依据的另一套字号、分隔与短事实布局，不合并业务状态或字段模型。 |
| V-04 | P2 | 导出/普通表内操作实际 27px 高，旁路默认按钮 36px；字号均为 14px/22px。三个底栏按钮均为 36px，但导出/普通末两按钮为 8px 间距，旁路为 12px。当前首行实测导出 61.84px、旁路 60.59px，**没有证据证明按钮撑高表行**。见 `1920-*-review-disabled.json`。 | 统一相同语义的表内按钮尺寸和底栏操作分组间距，不按内容行高机械设置表格高度。 |
| V-05 | P2 | `DirectLoadConfigurationStep.vue:40,53` 直接展示环境、ISO 时间和节点平台协议值；导出/普通使用环境中文与格式化时间，节点 P1 要求“名称 · Windows/Linux · 架构”。来源截图可见 `TEST`、原始 ISO 时间；`1920-direct-node-options.png` 实际选项显示 `WINDOWS_AMD64`。 | 复用现役展示函数；只调整显示，不改变源、节点资格和真实连接事实。 |
| V-06 | P2 | 普通导入最终确认在原生 200% 下，稳定初始焦点确为“返回检查”，但按钮 y=455、height=36，CSS 视口高 464，仅顶部约 9px 可见。实际完整物理截图中按钮文字和大部分动作区在下方，须额外滚动才能完整看到安全动作。见 `normal-modal-settled.json`、`native200-normal-modal-focus-stable.png`。依据 P0 可见 focus ring、200% 与 Modal focus safety；不是焦点泄漏或无法取消。 | 在当前确认组件适应高度，或打开稳定后让安全焦点控件进入可见区；保留完整冻结事实与风险，不缩字、不隐藏风险、不绕开现有焦点栈。 |

V-03 还包含共同的既有实现债务：导出/普通摘要组标题实测行高均为 18.4px，旁路为 18.837px，均未达到 DESIGN 中 Section Title 的 24px。因此修复应依据 P0 的 16px / 24px / 600，不直接照搬对照页面现有 P4 行高；本次不修改规范来消除差异。

RPC InputNumber 实测为 90×36px；补测 `65535` 时内部 clientWidth / scrollWidth 均为 88px，没有正常端口值被截断的证据，不列验收缺陷。确认 Modal 的 520 / 520 / 552px 宽度和旁路必填名称、风险文案数量不同，不能只按像素不同判为缺陷；普通导入 200% 的实际初始按钮可见性另按 V-06 记录。滚动后 bottom=464.5 与整数 innerHeight=464 的 0.5px 边界误差不单独列缺陷。

### 合法差异及静态 REVIEW 处置

导出/普通五步水平栏、旁路六步竖向栏是 Task Builder 已登记差异。旁路独立参数目录的处置状态、异步元数据、只读节点键有真实业务依据；三类目录同为 720px Ant Drawer、搜索及列表，P1 仅明确导出/普通共用目录，不强制合并旁路模型。来源 Select 与 Radio 均为允许的控件语义，不能仅因选择形式不同判失败。旁路必填任务名称、单表整体提交和独立风险确认属于业务差异。

本次重新执行 `npm run tokens:check`、`npm run audit:platform` 通过；平台审计为 **0 硬违规、95 REVIEW**，输出保留在 `platform-audit.log`、`platform-audit.json`。95 条经只读引用复核分为：1 条混合选择器误报、66 条当前三页无引用的历史候选、19 条任务中心/详情范围外、9 条平台泛用规则范围外。关联清单 44 条为其中 1 条误报、35 条历史候选和 8 条任务详情范围外。

- `.command-empty pre, .wizard-command pre` 后半仍由三个 ReviewStep 实际消费，不能整条删除或判失效。
- 旧参数列表/折叠 8 条、旧预检 20 条、command-empty 4 条、export-precheck-row 3 条当前未匹配三页；现役为 Ant Table、`wizard-precheck-*`、Ant Collapse 和参数 Drawer。它们保留为历史候选，不宣称全站 dead，也不把旧 CSS 当推荐规范。
- 另外 31 条旧表单、选项和树规则当前三页无引用；范围外 28 条不纳入本次三向导通过结论。`.task-detail-card .summary-definition div` 不匹配本三页，也不代表旁路仍消费的基础 `summary-definition` 无用。

上述人工处置只说明引用范围，**0 硬违规和 REVIEW 归类均不等于八项视觉验收通过**。本次不删除历史样式，不修改 P0/P1 以迎合实现。

### 浏览器、状态与交互证据

主矩阵浏览器为 Chrome `156.0.8078.36`，使用 1920×1080、1440×1024、1280×720，DPR=1；原生 200% 使用 Chrome for Testing `153.0.8010.12`，同一矩阵内三模块使用相同浏览器版本。每档均覆盖三模块来源、字段、草稿未保存/保存后、预检、摘要 Drawer、参数目录和最终确认。当前有效原始 PNG 为 137 张，其中原生缩放 18 张主图和 9 张补图；页面量测 131 份。已对代表原图及三档拼图进行目视复核，差异数值另由 computed style / geometry 佐证，未用 DOM 自动断言代替目视。

| 范围 | 实际观察与限定 | 证据入口 |
| --- | --- | --- |
| 1920 主对照 | 已目视三模块六类主要工作面；共同 Shell、表头、目录外壳保留同一视觉语言，摘要和同类操作仍有 V-01 至 V-04。 | `compare-1920-{source,fields,review-disabled,summary,catalog,confirmation}.png`；[摘要原尺寸对照](../../../artifacts/cross-module-visual-20261019/compare-1920-summary-detail.png) |
| 1440 / 1280 | 1440 五组拼图、1280 对应全部十五张原图另行交叉目视；表单单列重排、摘要/目录滚动、确认文案和按钮未发现新的水平越界。已知摘要差异不因响应式可用而消失。 | `compare-{1440,1280}-*.png`；`screenshots/{1440,1280}-*-*.png` |
| 加载、空态、读取异常、无权/不可读草稿 | 实际打开合成加载 Skeleton、无候选 Empty、来源请求错误和 403 草稿不可读状态；错误可见，下一阶段或保存保持阻断。只验证展示与当前合成交互，不提升真实权限或网络验证结论。 | `1920-*-state-{loading,empty,error,unreadable}.png/json` |
| 字段错误 | 对来源/数据库/场景/RPC 等所选必填字段触发校验，页面保持当前步骤并显示错误；未穷举所有参数的每一项字段规则。 | `1920-*-state-field-error.png/json` |
| 保存成功/失败、未保存输入 | 保存成功为短时 Ant Message；合成保存失败有持久错误 Alert、输入仍在。未保存离开弹层实际打开，取消后输入保留。导出失败图最终等待 PATCH 503、红 Alert 与 loading 结束后重采。 | `*-saved.png`、`1920-*-state-{save-failed,leave-dirty}.png`；`1920-export-save-settled-run.json` |
| 预检等待、FAILED、UNKNOWN、过期 | 四态均实际打开。FAILED / UNKNOWN 总览导出和普通为 error、旁路为 info；过期导出和普通为 warning、旁路仍为 info。旁路固定的业务风险 warning 不等于领域总览，不能抵销 V-01。 | `1920-*-states-summary.json`；[失败状态对照](../../../artifacts/cross-module-visual-20261019/compare-1920-state-precheck-failed.png)、[过期对照](../../../artifacts/cross-module-visual-20261019/compare-1920-state-precheck-expired.png) |
| Drawer / Modal 键盘 | 三模块摘要、目录、最终确认共九组：实际 Tab、Shift+Tab 各八次并测首/尾边界换向，焦点均留在当前弹层；摘要/目录初始标题聚焦，最终确认初始“返回检查”，取消/关闭返还触发器。初始焦点正确不等于完整可见，普通导入 200% 保留 V-06。实际开闭同时保留 Ant transition / animation 事件。 | `metrics/{export,normal,direct}-keyboard-scroll.json`、`*-modal-settled.json`；逐页 `motionEvents` 与 `activeElement` |
| 原生 200% | 使用隔离 Chrome 扩展 `chrome.tabs.setZoom(tabId, 2)`；CSS viewport 由 1424×929 / DPR1 变为 712×464 / DPR2。采用浏览器 `captureVisibleTab` 原始图像，物理截图 1424×929，已目视三类命令、摘要、字段和确认；没有使用 CSS zoom 或只改 deviceScaleFactor 冒充原生缩放。 | `native200-*-{source,fields,fields-scrolled,review-command,summary,catalog,confirmation}.png`；`native200-*-summary.json` |
| 滚动与底栏 | 1280 和原生 200% 下，三模块各检查末个可见配置控件及页尾命令，共十二处；均与可用区相交，目标 bottom 位于 footer top 之前。配合滚动后截图目视，未发现本轮样本被固定底栏永久遮挡。 | `metrics/{export,normal,direct}-keyboard-scroll.json` 的 `scroll1280` / `scroll200`；滚动后原图 |
| 长中文 / 技术值 | 已目视目录中的长中文说明、CLI 参数、脱敏命令及 64 字符合成指纹的换行，未发现样本越界；不外推任意长度输入或所有参数组合。 | 三档目录、原生 200% 命令图及对应 entries |

采集质量修正不计产品缺陷：普通导入首批 UNKNOWN 夹具将独立检查原因码错误写为通用码，被严格响应解析拒绝；已修正临时夹具并重采普通三档主页面及状态图，最终图为六项真实领域 UNKNOWN，而非“无法读取检查结果”。首批原生缩放图发生 712×464 裁切，已废弃并以浏览器原生全幅图覆盖；不使用旧裁切图证明通过。临时采集器、重采和补测均未修改产品代码。

汇总、复现入口及实际分批记录见 [browser-audit-results.json](../../../artifacts/cross-module-visual-20261019/browser-audit-results.json) 和 [run-notes.md](../../../artifacts/cross-module-visual-20261019/run-notes.md)。临时夹具保留在 `web/node_modules/.cache/cross-module-visual-20261019/`；没有新增依赖或持久产品测试。没有宣称最终夹具一次性全量运行通过，早期定位错误、夹具修正及中止批次均有记录。完整动效专项、减少动态偏好、屏幕阅读器、长中文名称及极限长度路径未验证，不外推为全部无障碍或所有输入组合通过。

[清理核验](../../../artifacts/cross-module-visual-20261019/cleanup.json)确认本次 15175 已关闭、独立浏览器已退出、临时独立 Vite 缓存已清除；原有 15173 / PID3804 保持监听，复现夹具和证据保留。

本次视觉审计不重复运行已在上文记录的 Go、全量前端功能或构建检查，也不将历史通过结果当作本轮新执行；本轮实际静态检查为 tokens、platform 和 `git diff --check`。当前仍有 V-01 至 V-06，八项完整验收未通过；修复后应按受影响矩阵复验，再更新该结论。

## 跨模块整改与水平进度栏复验

2026-10-19 用户明确要求按上述审查记录整改，并要求旁路进度条与另外两模块一致。整改对象为独立旁路导入；同时修复审查涉及的共同摘要标题与普通导入确认弹层。仅改前端呈现、对应独立展示规则、回归测试和规范记录，不安装服务、不操作运行库、不启动官方工具或访问真实数据库。

### 实施范围

| 项目 | 当前实现与边界 |
| --- | --- |
| V-01 | `directLoadPrecheckPresentation.ts` 统一旁路行、总览和有效检查进度：失败/未知为 error，过期/失效为 warning，等待为 info；成功只表示当前预检，独立执行门禁继续单列。 |
| V-02 / V-03 | `WizardFrame` 统一摘要 body 零 padding，保留共享正文 24px；旁路复用 `wizard-summary-*` 分组、短事实双列、技术值换行，删除私有摘要 padding 与旧 definition 排版。三模块标题共同补齐 P0 的 16/24/600。 |
| V-04 | 旁路预检查表内链接采用原生 small 按钮，底栏末两操作复用 `footer-actions` 的 8px 间距，表行仍按实际内容自然增高。 |
| V-05 | 来源环境与测试时间复用 sources 展示函数；节点复用 nodes 的“名称 · Windows/Linux · 架构”，只改展示，不改标识或资格。 |
| V-06 | 普通导入最终确认复用 `orch-confirmation-body` 的视口高度限制与内部滚动，安全返回和提交位于 body 之外；沿用原生 Modal 与现有焦点栈，不缩字、不删除风险事实。 |
| 步骤进度 | 三模块共用顶部水平 Ant Steps，旁路仍六步；前五步来自有效本地输入，第六步来自固定九项当前通过证据，等待/失效/过期为 0%。已访问但无效步骤显示待完善；目录定位不授予前序完成资格，从头执行仍锁定第六步。 |

新增长参数或执行能力：无。运行中工具阶段/百分比缺少可靠旁路来源时仍保持未知，本节的步骤栏完整度不能作为执行进度。

### 复验记录

**当前结论：V-01～V-06 已修复，本次受影响的八项跨模块视觉验收通过。** 仅表示当前前端与合成验证完成，不提升真实旁路执行准入。

浏览器使用现有 Playwright 与合成 API；Browser 插件未列出，按前端测试调试 Skill 使用现有 Playwright。临时收集器复用初轮夹具，保存到仓库外的 `C:/Users/Dionysus/.codex/visualizations/2026/10/19/01a1541b-37f1-7621-9705-59932b42c0f5/direct-load-remediation/`，不覆盖初轮截图。

工作树 HEAD 仍为 `0c5953cdbf406c01ee89ac9018af90a123d67b64`，包含用户原有未提交实现。本次截图对应的 255 个源码、设计及锁定配置文件记录于该目录 `source-manifest.json`，SHA256 为 `9488fb3fdefed2e6bb70d47901bf6a166e2f847cca4c25b56cc0fb524fc85218`；`source-verification.json` 复核无文件内容变化。不能只用 HEAD 代表截图版本。

| 八项验收 | 当前结论 | 本轮证据 |
| --- | --- | --- |
| 同类型页面布局与对齐 | 通过 | 三模块统一水平步骤栏、全宽工作面和摘要；三档摘要首项 inset 均为 24px，保留业务六步/五步差异。 |
| 同类控件尺寸、样式与状态 | 通过 | 旁路表内链接 27px，底栏原生按钮 36px，末两动作 8px 间距；失败、未知与过期没有成功或普通信息伪装。 |
| 表格、表单、抽屉、弹窗 | 通过 | 三档现役表格、字段、摘要与目录实际开闭；旁路复用共同摘要排版，普通确认正文内部滚动，安全动作保持可见。 |
| 同类操作动效和反馈 | 通过 | 保存沿用短时 Message，读取/失败/风险沿用 Alert，字段错误原位；Drawer/Modal 保留 Ant 动效，实际打开、取消与焦点返回。 |
| 模块间密度 | 通过 | 共同摘要正文宽 592px，短事实双列、标签 13/20、组标题 16/24/600；不以业务字段数量强行比较总高度。 |
| 1920×1080 实际截图 | 通过 | 三模块来源、字段、预检、摘要、目录、确认及关键状态均已采集，整改涉及的代表原图完成目视对照。 |
| 1440×1024、1280×720 | 通过 | 三模块两档实际页面和截图通过；原生 200% 另复验三模块命令、摘要、目录与确认。普通确认安全动作初始 y=388.5、height=36，完整位于 CSS 高 464px 的视口内。 |
| 无无依据私有样式或重复实现 | 通过 | 删除旁路摘要私有留白，复用共享分组与按钮间距，不新增 token、控件 wrapper 或业务状态副本；下述历史/范围外 REVIEW 不冒充现役实现。 |

### 实际检查与证据入口

- 前端全量 Vitest **48 文件、570/570**；`typecheck`、全量 `lint`、最终两份变更浏览器测试的定向 ESLint 和生产 `build` 均通过。构建保留既有大 chunk 提示，未扩成本次拆包重构。
- 向导、业务与平台审计正反例分别 **3/3、11/11、6/6**；均 **0 硬违规**。Token 检查、秘密扫描与 `git diff --check` 通过。未重复运行与本次前端呈现无关的 Go/真实执行检查。
- 旁路现役 **12 个浏览器场景分批通过**：新增水平百分比、目录跳步与事实展示、失败/未知/过期总览及摘要几何，保留保存恢复、格式门禁、无权、风险确认、从头执行和原生 200%。普通导入新增小视口初始安全动作断言的两个名称确认场景通过；数据源标准页分页/编辑焦点/Save-Test 回归通过。
- 仓库外跨模块采集 **19 个场景分批通过**：三档九个主要流程、三模块关键状态、三模块原生 200%、三模块稳定确认焦点、节点名称与五位 RPC。`metrics/*-run.json`、`*-states-summary.json`、`*-modal-settled.json` 均无 UNVERIFIED、运行时错误或意外请求；三模块原生确认的安全动作完整可见、初始聚焦、取消返回，提交请求为 0。
- 主矩阵使用本机 Chrome `156.0.8078.36`，原生缩放实际版本 `153.0.8010.12`，通过 `chrome.tabs.setZoom(tabId, 2)` 从 CSS `1424×929 / DPR1` 变为 `712×464 / DPR2`；原图使用 `captureVisibleTab`，不是 CSS zoom 或单独改变 DPR。代表入口为 `screenshots/1920-{export,normal,direct}-summary.png`、`{1440,1280}-direct-{summary,review-disabled}.png`、`1920-direct-state-precheck-{failed,unknown,expired}.png`、`1920-direct-node-options.png` 和 `native200-normal-modal-focus-stable.png`；原始几何及焦点保留在同名 metrics。

采集器修正如实保留：首次浏览器启动因继承 SOCKS 代理失败，仅清除子进程代理变量重跑；两项旧竖栏/按钮角色定位更新后复验通过；沿用的节点截图脚本原来断言 `WINDOWS_AMD64`，改为断言本次目标显示“Windows · x86_64”后单项复验通过。没有将早期失败或中止批次记为通过，也没有声称最终所有夹具一次全量通过。采集器、日志与失败 trace 保留在上述仓库外目录及 `C:/Users/Dionysus/AppData/Local/Temp/ob-direct-remediation-20261019/` 供复核。

### REVIEW 人工处置与限制

本轮向导审计 4 条 REVIEW 均为既有导出能力规模信号，旁路继续由唯一表单、草稿事务、检查事务与导航组合持有状态。业务审计 16 条 REVIEW 中，旁路新增的两条跨 feature 引用只复用 sources/nodes 的无状态事实显示函数；没有导入其会话状态、HTTP 客户端或业务资格。原竖栏组件退出本次现役入口后产生“0 个业务入口”和未引用样式信号，按历史候选登记，不恢复旧布局或借此复制另一份步骤实现。

平台审计 **105 REVIEW** 继续区分硬违规与未引用选择器。与本次改动直接相关的摘要分组、单层留白、标题行高、原生按钮及共享间距均有 P0 或用户明确依据；退出使用的旧竖栏选择器和初轮已归类历史候选不在当前页面 DOM，未批量删除或宣称全站 dead。其余任务详情、列表及既有通用规则沿用初轮限定处置，不扩入旁路整改范围。报告保存在上述 `platform-audit.json`。

本轮验证覆盖当前合成夹具、三档与原生 200%，未声称穷举全部长输入、减少动态偏好、屏幕阅读器或全部业务组合。没有安装/替换控制面或 Agent、升级现役 SQLite、解析真实秘密、启动官方工具或操作真实数据库；DLV/G3/G4、ODP 精确版本及 Oracle 等真实阻断保持不变。

收尾核验见仓库外证据目录的 `cleanup.json`：15174/15175 隔离预览已关闭，本轮 Vite 缓存已清除；原有 15173 / PID3804 继续监听。复现收集器、截图、日志与来源清单保留供复核。

## 2026-10-20 五步流程与 RPC 默认填写复验

本轮按用户明确采纳的 DEC-056 实施源码变更：数据源与连接 → 导入目标 → 文件与格式 → 执行参数 → 预检查与提交。入口改为首步有效继续时集中确认，保留四项服务端阅读确认和最终快照风险确认。数据源名称/环境筛选与单选列表复用三模块同一 `wizard-source-*` 布局；候选资格、表单和生命周期仍由各模块独立持有。RPC 按场景预填可编辑默认值，切换保留自定义值、草稿恢复不改写保存值；F5/VIP 只提示同源地址的 SQL/RPC 转发和真实验证条件，不扩展支持声明。

### 环境与版本

- Windows AMD64；主矩阵 Chrome `156.0.8078.36`，原生缩放 Chromium `153.0.8010.12`。隔离 Vite 为 `http://127.0.0.1:15174`，全部 API 使用合成路由，未连接开发控制面、数据库或 RPC 目标。
- 会话未列出 Browser skill，沿用项目已配置的 Playwright 浏览器测试。三档为 1920×1080、1440×1024、1280×720；200% 由现有隔离扩展调用 `chrome.tabs.setZoom`，验证 DPR 实际翻倍，不使用 CSS zoom。
- HEAD 为 `0c5953cdbf406c01ee89ac9018af90a123d67b64`，工作树已有其他改动，HEAD 不能代表本次实现。下列 SHA256 标识最终相关工作树文件：

| 文件 | SHA256 |
| --- | --- |
| `useDirectLoadWizard.ts` | `d79ad2e31eead6d8d92e62aa640b5c2f3793fa9f4e59055ebb64a2bd7add8b36` |
| `directLoadForm.ts` | `275e2f2a0ea4cc050fbc302806d361c42d61ccbde371707b0099820ede63ec84` |
| `directLoadRules.ts` | `f6419565d159fbf10b49d40827174f688e0cf424bc37e8a857037fb39450288f` |
| `steps/DirectLoadSourceStep.vue` | `500419e88d26842384718a6fcc8230d1b34c29b1e2265109c2234e33464f3f6b` |
| `views/DirectLoadWizardView.vue` | `5b44a1e44ce6897e962b4cc6922cf06a4144fe325306f22fa17811f60e1e37ce` |
| `platform/archetypes.css` | `4ce7a182abd5592f5721a1d1c6066f85306713a86ff18c602ed65e4ce339a315` |
| `tests/direct-load.spec.ts` | `b9e95bd2338cb39f7ebccd8c2cc1d97275f37f7c642456f431c3df6e9e381ead` |

前三个文件及步骤文件位于 `web/src/workbench/import/direct-load/`；views/platform 相对 `web/src/`，tests 相对 `web/`。

### 检查与行为证据

- 全量 Vitest **48 文件、576/576**，`typecheck`、全量 `lint` 和生产 `build` 通过；最终两份测试文件调整后定向 ESLint 通过。构建保留既有大 chunk 提示，本轮未改依赖或拆包。
- `audit:business`、`audit:wizards`、`audit:platform` 及其 **11/11、3/3、6/6** 正反例通过，均 **0 硬违规**；Token、秘密扫描与 `git diff --check` 通过。
- **22 个相关浏览器场景分批通过**：旁路 15 项；导出更换来源取消回归 1 项；普通导入来源工作面和独立路由/参数定位 2 项；数据源标准页分页/编辑焦点/Save-Test 1 项；四代表页三档 3 项。验证无页面运行时错误；没有把失败、中止或未执行批次写成通过。
- 五步往返保留输入；来源名称/环境筛选、元信息与空态正确；私有 ODP 2885、云 ODP 3307、OBServer 2882 的默认填写有单元覆盖，浏览器覆盖 ODP 两场景及五位自定义端口保留。恢复空值或自定义值不隐式改写。
- 首次有效继续才打开入口弹窗；Escape/返回配置不记录四项确认，明确继续才记录。往返不重复弹；原六步最终链接兼容到第五步，缺少阅读确认时，即使合成执行策略开启也不发检查或提交。最终名称、风险、九项检查及独立执行策略仍不可绕过；只读从头执行与可编辑重建均通过。
- 预检查修正入口、103 参数定位、任务草稿提交及派生导航已同步五步。冻结快照继续保留原六个证据分组，未改协议、迁移或历史快照。

代表截图位于 `artifacts/frontend-platform/`：`direct-load-旁路-1920-五步工作面与键盘弹层-chrome/direct-load-1920-source.png`、同目录 `direct-load-1920-files.png`；`direct-load-入口确认可取消、仅明确继续时记录，筛选与RPC默认值不授予执行资格-chrome/direct-applicability-{1920,1440,1280}.png`；1440 步骤场景的 `direct-load-1440-review.png`。导出/普通导入来源页与数据源标准页截图同批保留。

最终低视口与原生缩放补验位于 `artifacts/frontend-platform-direct-load-final/`，分别为 `direct-load-旁路-1280-五步工作面与键盘弹层-chrome/direct-load-1280-source.png` 和 `direct-load-旁路原生-200-缩放下五步、摘要与门禁操作可达-chrome/direct-load-200-percent-applicability.png`。已打开检查原图，200% 弹窗的完整说明、返回配置和明确继续均在窗口内，正常截图不截掉右半弹窗。

### 视觉验收、REVIEW 与限制

本次适用的 P0 八项均通过：共同布局/对齐、同类控件、表单/表格/抽屉/弹窗、动效/反馈、跨模块密度、1920 实际截图、1440/1280 响应式、无无依据私有样式。来源列表几何只保留共享实现，入口浓缩说明不占用独立步骤，风险使用 Ant Alert/Dialog。三档截图、原生 200%、取消焦点返回、摘要焦点约束、保存恢复和门禁均有当前版本证据；低视口允许工作面正常滚动，不缩小文字或隐藏关键操作。

业务审计 16、向导 4、平台 105 条 REVIEW 仍按前节限定处置。本次新增来源步骤只复用 sources 的无状态环境/时间展示函数，不借用来源管理状态、请求或资格；唯一表单、引用列表、草稿、检查及导航确认仍分别由既有能力持有。公共样式有三个真实调用模块，未新增 Token、基础控件 Wrapper 或导入业务引擎。

早期检查如实保留：第一次浏览器启动继承 SOCKS 代理而失败，仅清除测试子进程代理后重跑。主批为 21 PASS/1 FAIL；新加的“1280 首屏整项无需滚动即可显示”断言不符合低视口允许工作面滚动的验收条件，最终补验先正常滚动定位，再断言整项完整可见且不受固定底栏遮挡，2/2 PASS，没有降低完整可见断言。首次原生 200% 的截图助手按 CSS 尺寸裁切，原图不作为通过证据；最终使用既有 CDP 可视窗口采集并重新目视检查，动作边界另有 DOM 断言。产品源码在主批与补验间未再修改。

隔离测试服务已退出，15174 无监听；本轮未停止或替换控制面/Agent、升级现役 SQLite、解析真实秘密、进行真实 SQL/RPC/F5 探测或启动官方工具。真实 DLV/G3/G4、ODP 精确版本、Oracle 及各场景环境验证仍保持原门禁；本次页面和合成验证通过不表示真实旁路可用或 F5 链路已验证。

## 2026-10-20 本地部署范围收敛复验

用户明确本地部署后，按 DEC-057 收敛旁路活动范围。页面只展示私有 ODP / OBServer，RPC 默认分别为 2885 / 2882 且可修改，格式仅 CSV；移除部署外场景、专属参数入口、端口与说明。五步、集中入口确认、最终风险确认、当前九项预检及独立默认关闭的执行门禁继续生效。

### 实现与兼容边界

- 唯一规则所有者 `directLoadRules.ts` 提供本地场景、格式、默认端口及参数目录投影；引用能力验证冻结响应后应用投影，不新增请求、状态副本或元数据版本。官方 103 项研究目录及摘要保留原值；页面排除一项部署外专属参数后显示 102 项，三种本地未确认格式标为待验证，RPC/租户仅显示本地说明。
- 服务端领域校验、唯一命令生成器、固定探针 Go 入口及 Agent argv/配置复验同步拒绝本地范围外场景和 SQL/Parquet/ORC 格式；不能以原研究目录的支持状态绕过。`LocalConfig` 约束活动写入、机器预检上下文和执行输入，`Config` 仅为草稿及快照保留读取兼容。
- 历史草稿场景、格式和端口保留原值；页面不把旧场景静默转换为私有 ODP、不把未确认格式静默改成 CSV。重新选择前导航、预检查和提交仍阻断。没有改写 SQLite 历史或任务快照。
- 既有跨业务 REVIEW 仅复用 sources/nodes 无状态展示函数；表单、引用、草稿、检查与导航仍由各自现有能力持有。目录投影是纯规则，不授予执行资格，不增加 Wrapper、Token 或通用向导引擎。

### 实际检查

| 检查 | 当前结果 |
| --- | --- |
| 定向 Vitest（旁路生命周期、严格 API、任务草稿管理及详情生命周期） | 4 文件、50/50 PASS |
| Go 单元/契约回归 | `directload`、`commandgen`、`executionargv`、`jdbcprobe`、`controlplane`、`store`、`agentwire`、`agentpreflight`、`agentexecution`、`agentworker`、`contracts` 共 11 包 PASS |
| Go 静态检查 | 上述领域、生成器、argv、探针、控制面与契约 6 包 `go vet` PASS |
| 前端构建与静态检查 | `npm run build`（含 Vue/TypeScript 检查）、全量 `lint` PASS；保留既有大 chunk 提示 |
| 向导/业务审计 | 3/3、11/11 反例 PASS；均 0 硬违规，既有 4/16 条 REVIEW 已按上述所有者边界复核 |
| 秘密与差异检查 | `check-secrets.ps1`、`git diff --check` PASS |
| 浏览器回归 | 17 个独立用例分批 PASS；三档 1920×1080、1440×1024、1280×720 与原生 200% 均覆盖 |

浏览器复用上一节隔离 Chrome/Chromium 与 Vite 15174，所有 API、数据库目标、节点和凭据均为合成夹具。实际断言场景下拉仅两项、目录不含部署外文字/参数/端口、本地 RPC 说明正确，历史非法场景/格式阻断且不静默改写，保存恢复和最终提交仍受名称、风险及九项检查约束。

首批 **16 PASS / 1 FAIL**：新增历史场景测试误用了“返回第 5 步”入口，步骤栏仅允许返回已访问步骤，因此定位超时；产品阻断行为正常。测试改为通过既有 `?step=5` 深链读取同一历史草稿，独立补验 **1/1 PASS**，断言零写入、零预检和提交禁用。产品源码在两批间未改动，失败没有记为通过。

截图位于 `artifacts/frontend-platform-direct-local/`，已打开目视核对：

- `direct-load-本地部署只提供两种连接场景，目录过滤部署外入口和说明-chrome/direct-local-scenarios.png`、同目录 `direct-local-rpc-catalog.png`；
- `direct-load-旁路-1280-五步工作面与键盘弹层-chrome/direct-load-1280-source.png`；
- `direct-load-旁路-1440-五步工作面与键盘弹层-chrome/direct-load-1440-files.png`；
- `direct-load-旁路原生-200-缩放下五步、摘要与门禁操作可达-chrome/direct-load-200-percent-applicability.png`。

200% 使用现有隔离扩展 `chrome.tabs.setZoom` 验证 DPR 翻倍，并使用 CDP 完整采集可视窗口；入口说明及两个操作均完整可见。低视口沿用工作面滚动，来源整行和固定底栏不相互遮挡。补验结果另保留在 `artifacts/frontend-platform-direct-local-recheck/`。

### 工作树标识与限制

工作树已有其他未提交修改，仍不以 HEAD 代表当前实现。以下 SHA256 标识本轮最终关键文件：

| 文件 | SHA256 |
| --- | --- |
| `web/src/workbench/import/direct-load/directLoadRules.ts` | `df7c324c8b5b11e22b53b87dc326a25ec24b932357a3e2123fcc3cc8023a136a` |
| `web/tests/direct-load.spec.ts` | `8b4b0b4383d73840290e311906e0ef9dcd12b8e8a22bf2f93c284a0b5134b018` |
| `internal/directload/config.go` | `a9cae6953f4f940bc25e17f45e97afdf140aaa83653e705be9b3120bc626ab22` |
| `internal/commandgen/direct_load.go` | `c36749a31895a6fbe3bc798770b36f33098b4465aae627dc9698eb8a1252c6e8` |
| `internal/executionargv/direct_load.go` | `5f4a593d3b49122934a3e43819d90ec5a5ab4b94ec9480aae1d591912b3fe322` |
| `contracts/direct-load.json` | `f67eaa84706ce41a9ed083f5b869474a6886288933a5bead8e3ec108e9bea56b` |

隔离预览已退出，15174 无监听。只完成本地源码、页面与合成验证，没有安装/重启控制面或 Agent、变更现役 SQLite、真实秘密解析、SQL/RPC/F5 建连或官方工具执行；真实 DLV、ODP 精确版本与 Oracle 等阻断仍保留，不宣称真实旁路功能已验证或已部署。

## 2026-10-20 连接场景名称微调

按用户要求，连接选择与任务摘要统一使用“通过 ODP 连接”“直连 OBServer”，取代重复的部署/产品名称。仍由 `directLoadScenarioLabels` 唯一提供标签，枚举、RPC 默认值、自定义端口保留及历史配置行为不变；字段规则与维护索引同步更新。

`npm run typecheck`、两份受影响源码/测试的 ESLint 和 5/5 个既有合成浏览器用例通过，覆盖两个选项、端口切换及 1920×1080、1440×1024、1280×720。已打开三档截图检查名称与现有布局，截图位于 `artifacts/frontend-platform-direct-scenario-labels/`；200% 沿用上节记录，本次未重复验证，不将文案修改外推为新的布局或真实执行验收。

当前 `directLoadRules.ts` SHA256 为 `e6c3f30b51e46ce1ef80417270ff05d2af215584403306bcb9634e9b884f1cad`，浏览器测试文件为 `dc7ddd17613aef16609ee44036e7fba751b1c277152e409c47b34e32d57c69a1`。仅本地源码与合成预览，未安装服务或执行真实数据库/工具操作。
