# 整体审查缺陷修复 · 2026-10-28

## 范围与基线

依据用户在整体审查后提出的“将排查出的问题进行修复”，修复基线 `4e8cb4034b474ac0cfe9ed9c63c8ae285963eeb0` 中确认的 4 项 P1、4 项 P2，并独立处理前端依赖审计告警。修复使用合成协议、临时 SQLite/文件、隔离端口和测试二进制；未连接真实数据库/存储、解析真实凭据、启动官方 Java 工具、安装或重启现役服务。

原审查证据保留在忽略目录 `artifacts/review-20261028/`；本轮可复核日志在 `artifacts/review-fixes-20261028/`。原本未跟踪的 `.claude/` 和 `.tmp-login-preview/` 保留，不纳入本次修改。

## 修复及负例

| 编号 | 问题与修复 | 验证边界 |
| --- | --- | --- |
| R1 / P1 | 旁路目录摘要按 Git 的 LF 字节冻结，加载只规范化 CRLF；103 项参数及元数据版本不变 | Git 原始 blob 和 CRLF 均可加载；内容、额外空白、孤立 CR 仍拒绝；路径夹具按测试主机平台构造 |
| R2 / P1 | 普通导入/导出保留原提交意图和幂等键；服务端串行事务限制同一草稿修订只冻结一次；当前授权内恢复旧回执，草稿/检查投影唯一 `submittedTaskId` | 并发、换键、重新预检不能重复冻结；证明过期不妨碍原键恢复；撤权后拒绝；历史多任务不添加唯一约束或猜测结果；导航失败、刷新、响应丢失与迟到响应有负例 |
| R3 / P1 | 导出独立观察固定成功终态，再结合退出码和所选格式的结果文件验证；远端文件事实 API 改为 `null` | 只有错误/检查点且退出 0 不能成功；匹配文本格式的合法空数据保持支持；对象存储文件数/字节/列表不再由 0/空列表冒充已核验；本地数值与两类导入的完成结论保留 |
| R4 / P1 | 准备结束、Java 版本检查后及工具进程创建前复验租约截止时间 | 槽位准备和版本探测耗尽租约时不得启动；不以控制面事后拒绝事件代替启动保护 |
| R5 / P2 | 精确回收私有 Hadoop 凭据目录，消费清理失败并通过持久护栏阻断后续领取 | 覆盖旧版无护栏残留、失败/重启/并发；网络补传不能延长明文秘密生命周期；未知进程或清理失败保留待核对状态 |
| R6 / P2 | 日志采集只密封并持久入队，由独立发送协程串行上传；导入按请求期限确认有限尾部快照，导出最佳努力收尾，未确认批次保留 | 慢网不阻塞后续本地入队；容量满记录缺口并排空管道；超过 2 秒的正常尾部仍可成功，失败/取消及最后回执后取消不误报成功；续租覆盖最后事件，等待上传前秘密已清除 |
| R7 / P2 | HTTP 返回增量续页游标，详情先读完固定快照分页，再进入增量；相同批次按记录 offset 继续读取 | 同批 201 条日志完整可达；后页失败可恢复；失权、卸载及 A→B→A 隔离迟到结果 |
| R8 / P2 | 存储凭据轮换在当前授权检查后先查原键回执，再执行新写入版本检查 | 原 If-Match/请求丢失回执后可重放；重复不生成修订或审计；不同内容拒绝；后续轮换不改写旧回执；审计失败回滚 |

## 前端依赖修复

Vue 3.5.44、Vite 8.1.6、Vitest 4.1.12 均为原版本分支补丁；Monaco 保持 0.57.0，仅对其 DOMPurify 使用 3.4.17 的局部 override。传递依赖 `postcss-selector-parser` 和 `source-map-js` 更新至兼容安全版本，所有结果由 lockfile 固定。避免采用 `npm audit fix --force` 提议的 Monaco 降级。

安全补丁依据包括 [DOMPurify 公告](https://github.com/advisories/GHSA-cg2g-xh64-v4q7)、[Vite/launch-editor 公告](https://github.com/advisories/GHSA-698v-4r3x-3vcv)及在线 npm 审计。MIT、MPL-2.0 OR Apache-2.0、BSD-3-Clause 许可证范围与跨平台影响记入[前端平台基线](../frontend-platform-baseline.md)。版本匹配告警不等同于已证明项目存在可利用攻击路径。

Node v24.16.0、npm 11.13.0；一次串行 `npm ci --no-audit` 成功安装 252 个包，未并行重装。在线 `npm audit` 为 **0**，原审查为 9 项。前端业务测试使用安装完成后的锁定依赖。

## 本轮验证

| 检查 | 实际结果与日志 |
| --- | --- |
| Windows Go 全量 | `go test -count=1 -p=2 -timeout=20m ./cmd/... ./contracts/... ./internal/... ./migrations/...`，51 个有测试包通过；后续平台夹具另经两个 cmd 包和 TLS 集成定向复验，日志收尾修正后三个受影响包完整复验通过，见 `go-windows.log`、`go-windows-command-final.log`、`integration-agent-platform-windows.log`、`go-windows-execution-final.log` |
| Linux 实际执行 | Windows Go 工具链构建 Linux AMD64 测试二进制，通过 `go-wsl-exec.ps1` 在 Ubuntu-22.04 WSL 实际运行；初轮 48 包通过，2 个 cmd 包及 integration 包有失败；平台夹具修复后对应 3 包完整复验通过，合计 51 包，日志收尾修正后三个受影响包再次完整通过。见 `go-linux.log`、`go-linux-command-final.log`、`go-linux-integration-final.log`、`go-linux-execution-final.log` |
| Git LF 资源 | `go test -count=1 -overlay artifacts/review-20261028/catalog-overlay.json ./internal/parammeta ./internal/localmvp` 通过，见 `git-blob-catalog.log`；运行库与启动组装均使用 Git 原始资源字节 |
| Go 静态与构建 | 最终全量 `go vet`、`gofmt` 通过；Windows AMD64、Linux AMD64/ARM64 的控制面/Agent 与迁移测试二进制重新构建通过，`CGO_ENABLED=0`，服务使用 `-trimpath`；见 `go-vet-final.log`、`gofmt.log`、`go-build-final.log`，最终成品摘要在 `build-final-sha256.json`，初轮记录也保留 |
| 前端门禁 | 最终 Lint、向导/业务审计、typecheck、73 文件 / 769 单测、生产构建全部通过，见 `frontend-final.log`；向导 4 项、业务 24 项 REVIEW 不等于硬违规，职责仍在既有提交/日志能力所有者 |
| UI 静态与 Token | `audit:platform` 0 硬违规、123 REVIEW；`tokens:check` 通过，见 `frontend-platform.log`；本轮没有新增 CSS、全局 token 或第二套反馈组件 |
| 浏览器 | 首批 75 场景中 71 通过；修复后 18 项定向全部通过，包含原 4 项失败、两项新增结果边界及任务详情全文件；合计 **77 个不同场景最终通过**，不重复累加复验，见 `browser.log`、`browser-final-retry.log` |
| 依赖与秘密 | 在线 `npm audit` 0；秘密扫描和差异检查通过，见 `npm-audit.json`、`secrets-final.log`、`diff-check-final.log` |

跨栈复核曾发现新增 `submittedTaskId` 在未提交时返回空串，与前端非空标识约束冲突；修正后 HTTP 层已验证未提交时省略字段，且旧预检查不能携带当前其他修订的任务结果。实际 HTTP 回归、随机信封轮换重放与结果空值投影证据位于 `artifacts/review-20261028/backend-{targeted-fix,receipt-contract,fixture-fix,result-projection}-tests.log`。

首轮 75 个浏览器场景为 71 PASS / 4 FAIL（24.6 分钟），发现两个此前未覆盖的步骤 2 阻断反馈缺口，以及 MySQL `retain-schema`/`compact-schema` 和对象存储说明的旧断言。已使用现有反馈展示明确阻断原因，保留禁用行为；按已确认 MySQL 互斥契约核验 `compactSchema:false`，对象存储改验必填临时目录及已有未知证据禁止提交用例，不恢复已删除的冗余说明。最终定向 18/18（3.7 分钟）通过。中间一次复验在 Vite 启动 60 秒内未就绪，零用例执行；随后串行启动通过，原日志 `browser-final.log` 保留，未更改等待阈值。

Linux 额外失败涉及原有 Windows 固定路径/心跳夹具，修复没有跳过测试或放宽平台校验。控制面首次健康启动检查达到限时；补充提前退出诊断后，独立启动/重启测试及 cmd 包完整复验均通过，未改变生产启动逻辑或扩大等待时间。早期独立 store 全包曾在 5 分钟限时内未完成，不算通过；主审无缓存全量中 Windows store 用时 517.179 秒、Linux store 566.406 秒，均通过。

收尾源码复核另发现异步化初版误把 2 秒作为所有任务的日志总收尾期限，正常导入尾部积压可能被误判失败。最终改为普通/旁路确认本 execution 的有限快照，逐次沿用协议期限；仅导出保留最佳努力时间窗口，所有未确认批次继续持久保存。原 30 秒续租覆盖到最后状态事件，秘密仍先于网络收尾清除。新增慢尾、stderr 未确认、失败/取消、最后回执后取消、其他 execution 隔离、续租与秘密清除测试通过；没有减少“导入日志全部确认才成功”的要求。最终 `agentexecution`、`agentexec`、`credential` 三包完整复验在 Windows 分别为 21.780 / 7.831 / 1.970 秒，在 Linux 为 37.592 / 26.220 / 1.571 秒，全部通过；随后三目标构建与全量 vet 均复验通过。

构建与递归临时清理合并的初始命令被自动策略拒绝，未执行；之后改为固定隔离目录构建、记录成品摘要、逐个精确删除本轮生成文件，全部成功。未覆盖或停止任何服务；隔离浏览器监听已关闭，构建临时成品已清理。

## 交付与真实门禁

远端结果摘要的三项 `null` 与严格客户端解析须前后端同批交付；旧客户端不能解析新远端 `null`，新客户端也不会将旧远端 `0/[]` 响应当作可信文件事实。历史数据兼容在控制面投影完成，不回写数据库；普通/旁路导入继续保留其完成结论。本轮未安装、部署或重启服务。

本轮只更新源码、契约和合成验证证据。Ubuntu WSL 的 Linux 测试只能证明对应合成路径，不替代麒麟目标原生验证；交叉构建也不提升平台支持。原 EX-V1/NIV/DLV/G3/G4 与发布门禁的未完成范围保持原状态。当前运行服务仍是既有二进制，本轮修复没有安装上线。

遗留执行私有目录或清理失败会使新领取失败关闭，需要按既有恢复/清理流程核对；脱敏日志补传本身不延长明文凭据生命周期。生产构建仍有既有的 500 kB 分包体积提示，本次未扩展为无关分包重构。未提交/推送修复或重跑 GitHub CI；远端仍为审查基线，不能把本地合成通过描述为已上线。
