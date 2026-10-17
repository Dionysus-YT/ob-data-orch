# 执行节点自动环境复验修订 · 2026-10-16

用户在确认原因与设计评估后明确要求按建议调整。原流程在重启时使旧清单失效，自动检查却没有版本查询许可；Java/OBDUMPER 必需版本因而未知，总体以失败码阻断。现场只读记录已确认该组合，其余七项通过。本轮以用户确认修订 DEC-048 及节点、安全、机器协议、启动隔离与 API/SQLite 契约。

## 最终行为与边界

- 新关联材料明确包含持续固定本机复验授权，交换成功后绑定当前 Agent；旧身份及旧材料迁移后默认为未授权。存量节点通过「授权并检查环境」一次授予，无需重新注册。
- 授权仅包含当前受控节点配置下的十项清单与 Java / OBDUMPER / OBLOADER 三个固定版本进程。沿用固定种类、最小环境、每进程 8 秒/总计 20 秒、输出上限和进程树监管，不授予数据库、秘密槽位、数据任务、自由命令或入站控制能力。
- 每个检查独立保存绑定 checkId 与 agentId 的许可快照；领取和完成均复核 ACTIVE 机器身份与持续授权。重启重新分配标识、事实版本并采集新证据；旧清单、迟到、配置漂移、自报越权和不同回执不能覆盖当前结果。新身份关联事务清除旧检查，即使新旧身份首份事实同为版本 1 也必须重新取证；同一材料安全重放不清除新身份的检查。机器身份撤销、替换或节点退役后有效授权失效，关联重放不恢复撤销范围。
- 手动/自动检查保留 ENABLED / DISABLED / MAINTENANCE 管理意图，检查期间阻断新任务。通过后已启用节点恢复接收资格；禁用/维护节点保持原意图，首次启用仍显式完成。既有运行/核对中任务继续按自身租约与证据规则处理。
- 浏览器 environmentStatus 新增 UNCONFIRMED：缺少授权显示「待授权」，证据不足显示「待核验」，已证实必需项失败才显示「检查异常」。机器检查回执仍使用 PASSED/FAILED 与既有稳定码，未知不得变成通过，缺完整清单的布局通过回执拒绝。
- 正常节点也可主动重新检查。详情首次打开承接已排队的自动检查，串行读取最多两分钟，切换、卸载、离线或读取失败停止；不会发送检查/启用写请求或自动成功消息。

## 所有者与数据变更

持续授权由现有 store 关联/检查事务写入，0031 仅增加 agents 和 agent_enrollment_tokens 的 CHECK 布尔列，不新建表，不修改历史迁移。`node_inspection_audit.go` 保存无秘密的检查许可快照；`agentenvironment` 与既有 `FixedVersionProber` 负责固定采集；控制面状态投影和前端纯展示规则区分待授权/待核验/异常。`useNodeDetail` 继续唯一持有详情事实、主动写事务及有界复读，页面只装配与展示。

## 实际验证

2026-10-16 开始实施，2026-10-17 完成收尾。全部运行于合成数据、临时 SQLite、假工具或隔离端口；结果不提升真实工具及数据任务准入。

| 检查 | 实际结果 |
| --- | --- |
| `go test ./internal/store ./internal/migrate ./internal/controlplane ./internal/agentenvironment ./internal/agentexec ./internal/agentwire ./cmd/agent ./contracts` | 通过。身份绑定收尾后，store/controlplane/migrate/contracts 全包再次通过，store 最终 138.614 秒；OpenAPI 说明同步后 contracts 再次通过 |
| 同范围 `go vet` | 通过；身份绑定收尾后的 store/controlplane/migrate/contracts 再次通过 |
| `go test ./internal/store -run 'Test持续授权使重启后的自动完整复验通过且保留管理意图' -count=1 -timeout=90s` | 最终将夹具明确设为存量未授权身份后，三种管理状态均通过，7.514 秒；覆盖从 0 授权到重启自动复验的完整闭环 |
| `go test ./cmd/agent -run 'TestRunWithContext在本机TLS控制面完成Agent关联和心跳' -count=1` | 最终源码通过。临时 TLS 控制面、合成节点及独立身份目录完成关联与首份心跳，未连接现役控制面 |
| 前端 `npm test -- src/api/browser.test.ts src/api/nodeDiagnostics.test.ts src/api/nodeRuntimeFacts.test.ts src/workbench/nodes` | 8 个文件，197/197 通过 |
| `npm run build` 与受影响文件 ESLint | 类型检查、构建及 Lint 通过。Vite 仍提示现有较大 chunk，未因此扩展打包优化范围 |
| `npm run audit:business` | 审计自身 11 项测试通过，0 个违规，10 项 REVIEW 保持人工复核；本轮详情事实与写事务仍属于 useNodeDetail，没有新增跨层业务状态 |
| Chrome：`npm run test:browser -- tests/execution-node-detail.spec.ts tests/execution-node-page.spec.ts tests/execution-node-registration.spec.ts --output ../artifacts/node-auto-inspection-20261016/browser` | 29/29 通过。使用假 API 与隔离 Vite；覆盖自动复验承接、存量明确授权、主动复验、防重复、离线、窄屏、缩放及注册材料清理 |
| Windows AMD64、Linux AMD64/ARM64 的控制面与 Agent | `CGO_ENABLED=0 go build -trimpath` 全部通过，独立输出至 artifacts/node-auto-inspection-20261016/build，不安装到服务目录 |
| `./scripts/check-secrets.ps1`、`git diff --check` | 通过；Git 行尾提示不属于差异错误 |

新增行为负例包含：未授权不得启动固定版本查询；旧布局通过回执拒绝；授权撤销、机器撤销/替换后旧许可不能完成；失效主体授予事务回滚；新旧身份事实同号仍重新检查；关联重放不重新授权或清除新检查；重启清除旧清单并保留三种管理状态。前端验证自动复读两分钟上限、同一检查不续期、切换/卸载/离线/失败停止与迟到响应隔离。

关键截图已人工查看：`artifacts/node-auto-inspection-20261016/browser/` 下的 `inspection-authorization-required.png` 和 `automatic-inspection-restored-640.png`，分别验证桌面授权提示及窄屏自动复验恢复。

## 部署与真实验证限制

本轮未停止、替换或重启现役控制面/Agent，未升级其 SQLite，未改写机器身份或历史检查，未启动真实 Java/官方版本查询、数据库/存储探测或数据任务。服务与网页需采用本轮兼容代码并应用 0031 后生效；存量节点必须一次明确授权检查，旧版布局回执不会被当成完整通过。隔离合成验证与交叉构建不能替代目标机真实版本查询或 Linux 原生验证，既有真实执行准入不提升。
