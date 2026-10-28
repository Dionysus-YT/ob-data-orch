# 导出运行时收口：默认日志与私有临时目录（2026-10-26）

本轮完成两项固定启动配置整改及隔离验证：未填写日志路径的导出改为受控 console 日志；Java 临时材料绑定当前 execution 私有 runtime。页面说明与已保存草稿摘要已同步。**这不是默认日志或压缩真实可用的验收结论**，未改写 2026-10-14 的 172 项真实结果。

## 范围与实现

用户要求继续 V1 下一步，本轮从任务地图登记的导出失败组合开始。只运行临时目录、合成子进程和隔离 Vite；未解析真实凭据、访问数据库、启动官方工具、安装二进制、迁移现役 SQLite、修改账号或现役配置。沿用非生产 `test-mysql` 和 Windows 节点真实复验的范围确认尚待用户回复，等待不视为授权。

只读检查现有 4.3.5 包发现，官方 `conf/log4j2.xml` 的 Routing 使用 `task.workspace`，只设置 `log4j.output` 不足以改写路由。包内 snappy-java 1.1.10.7 与 zstd-jni 1.5.2-5 有 Windows 原生资源，不能据此证明当前节点可加载。Snappy 对应版本的[原生加载源码](https://raw.githubusercontent.com/xerial/snappy-java/v1.1.10.7/src/main/java/org/xerial/snappy/SnappyLoader.java)读取专用 tempdir 或 `java.io.tmpdir`，支持私有临时目录设计，仍不能证明历史原生加载失败的真实根因已经消除。

- `log_configuration.go` 统一生成固定私有 console XML。默认导出使用 `export-log4j2.xml`；旁路沿用同名配置与原有 SDK 属性，配置内容不变。排他创建，已有文件必须为普通文件且逐字节一致；篡改或目录替换拒绝启动。
- `StartDirectJava` 只在 OBDUMPER 未显式提供 `--log-path` 时替换固定 JVM 配置。显式值原样传入，不改唯一生成器、业务 argv、配置指纹、冻结元数据或路径预检查，不隐式追加 `--skip-check-dir`。
- 三类直接 Java profile 各只有一个私有 `java.io.tmpdir`。原生临时材料随工作区清理，不进入结果清单、普通日志或下载。默认日志沿用登记的 stdout/stderr、双层脱敏和可靠队列，不新增任意文件采集能力。
- 向导只改第四步帮助、参数目录和已保存草稿摘要，未新增状态、布局、控件或反馈。草稿事实仍归 `useExportDraftLifecycle`，唯一表单与节点展示复用现有所有者。
- 当前 `DirectJavaProcess.Wait` 已先排空日志再等待进程，本轮未重复修改。快退慢接收测试验证现有行为，不能反向证明 MX-004/MX-005 的历史根因。

## 实际验证

| 检查 | 结果 |
| --- | --- |
| 修复前隔离复现 | 默认 Routing 与缺少私有 tmpdir 两项失败，合成退出码 62、63；显式路径和快速退出已通过 |
| 修复后执行层 | `go test ./internal/agentexec -count=1` 通过；假 Java 是 Go 测试二进制，未运行 Java/官方工具。覆盖默认/显式路径、临时材料清理、配置篡改/目录替换拒绝、257 条末尾排空与秘密脱敏 |
| 全量 Go | `CGO_ENABLED=0 go test ./cmd/... ./contracts/... ./internal/... ./migrations/... -json`：51 包、1965 项含子项通过，4 项既有跳过，无失败；相同范围 vet 通过，修改文件已 gofmt |
| 前端 | `exportDraftInput`、`exportWizardState`、`exportParameterCatalog` 三文件 97 项通过；build（含 vue-tsc）及三个修改源码 ESLint 通过；既有大 chunk 提示保留 |
| 结构审计 | `audit:wizards`：3 个规则测试通过、0 违规、4 个 REVIEW；`audit:business`：11 个规则测试通过、0 违规、23 个 REVIEW，未新增职责或依赖 |
| 页面 | 三档 1920×1080、1440×1024、1280×720 与原生 200% 的默认/显式日志、保存、摘要、Escape 返回共 4 项通过；既有单选/步骤/摘要、MYSQL/ORACLE 参数说明及窄屏交互 3 项通过 |
| 独立构建 | `CGO_ENABLED=0 go build -trimpath` 的 Windows AMD64、Linux AMD64、Linux ARM64 Agent 通过，仅说明可构建，未安装或原生验证 |
| 收尾 | 秘密扫描、`git diff --check` 通过；未提交或推送 |

4 项 Go 跳过分别是显式启用的一 GiB 队列物理上限、两个需要 Windows 符号链接权限的场景、显式启用的 Vite HMR，不计入通过。

页面使用 `frontend-testing-debugging`：Browser plugin not available，采用仓库 Playwright、独立 15174 端口、合成 API 和进程内 `NO_PROXY=127.0.0.1,localhost`。目标流程为“第四步留空/填写日志 → 保存草稿 → 摘要核对 → Escape 返回”。首轮 4 项因夹具未保存草稿便断言仅已保存配置才显示的字段失败；补齐合成保存事务后 4/4 通过，未改变产品逻辑。整页截图在滚动时使固定壳层错位，三档改录 viewport 并补验，原生 200% 用 CDP viewport 取证；重复运行不增加不同用例数，最终共 7 个不同浏览器用例通过。

八项视觉门槛按字段帮助与摘要的影响范围核对：现有 Task Builder 布局、Ant 尺寸、Form/Drawer、保存 Message 和焦点返回均沿用；三档与原生缩放实际截图可读，无整页横滚。未引入模块样式、全局 token 或重复组件；未把文字验收外推为全向导加载/权限状态重新认证。结构 REVIEW 沿用原有表单、目录、草稿及跨业务 presentation 归属，本轮没有新增异步事务或状态副本。

## 历史 12 项失败的后续归类

| 失败组合 | 本轮处理与仍缺证据 |
| --- | --- |
| MX-001 默认日志（1 项） | 固定出口已本地整改；需真实默认导出、完整行集、清单和日志完整性复验 |
| MX-004/MX-005 整库与排除（2 项） | 当前排空顺序已有修复，合成快退通过；需独立小型整库逐表复验，历史超时/等待错误不追认为已修复 |
| MX-037 CUT/SQL 空表保留（2 项） | 继续拒绝退出 0 但零结果文件，不把空目录算成功；待核对真实空文件行为 |
| MX-062 CSV/CUT 零日期（2 项） | 参数已发射但内容为 NULL 的历史失败保留，不改导出数据伪造支持 |
| MX-075 retain-schema（1 项） | 仍需真实 DDL 的 Schema 限定核对，不由退出码推导通过 |
| MX-110-zstd、MX-150-zstd-1/22（3 项） | 私有临时目录已接入；尚无真实压缩、解压及完整内容证据 |
| MX-110-snappy（1 项） | 历史 UnsatisfiedLinkError 保留；包内资源与合成写入不替代真实加载、解压和内容验证 |

先复验默认日志、zstd、snappy 三类，包含原 zstd 等级边界，再推进整库与内容差异。隐藏主键、sys 凭据、真实检查点、活跃进程恢复及跨身份的既有缺口继续按任务地图登记，本轮不额外创建未来模块。

## 本机证据与状态

截图、Playwright 原始结果、首轮夹具失败记录、Go JSONL/摘要、三目标构建摘要及本轮源码清单位于 `C:/Users/Dionysus/.codex/visualizations/2026/10/26/ob-data-orch-export-runtime-closure/`。构建成品在记录摘要后清除，没有替换现役程序；源码清单只登记本轮直接修改文件，不更写先前验收的历史指纹。

本地页面与合成验证完成；真实功能验收仍未完成，现役服务未更新，EX-V1/G3/G4 和发布门禁保持原状态。
