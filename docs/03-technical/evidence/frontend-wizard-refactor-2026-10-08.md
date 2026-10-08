# 前端向导架构重构验收（2026-10-08）

## 结果与范围

导出五步流程已按职责迁入 `web/src/workbench/export/`，`ExportWizardView.vue` 只负责页面装配和页面请求生命周期。唯一业务表单由 `exportForm.ts` 创建；步骤通过显式类型端口引用原状态，切换步骤不创建同步副本。参数、引用列表、目录、保存/预检/提交和恢复分别具有维护入口，详见[导出维护索引](../../../web/src/workbench/export/README.md)。

参数语义、后端 API、设计 P0/P1 和既有五步顺序保持原契约。既有草稿输入、数据源资格、预检展示纯规则及测试迁入 feature，模板中心同步引用资格规则；原有 ExportAdvancedSettings、ExportFormatChoice、ExportOptionHint、SqlQueryEditor、WizardFrame 与 Ant Design Vue 继续复用。

普通导入、旁路导入本轮只纳入[统一架构规范](../frontend-platform-baseline.md#复杂向导架构与开发规范)，两入口没有业务变更，没有创建空 feature 或通用 Wizard Engine。工作区原有后端、设计及其他前端改动均保留，未提交、推送或替换运行服务。

## 行为与异步管理

| 验收项 | 实现与验证证据 |
| --- | --- |
| 跨步骤状态保持 | 单一表单和五个步骤；浏览器往返、摘要、跨对象分类及内容/格式切换；单元测试验证仍合法的值保留 |
| 数据源、数据库、节点、租户切换 | 绑定变化同步失效；目录缓存与选库范围更新，Oracle/MySQL 不适用字段清值；源修订变化刷新恢复拒绝旧数据库 |
| 参数互斥、条件显隐 | 复用 `exportDraftInput`；snapshot/flashback、结果集、列包含/排除、内容/租户/CSV/CUT/SQL 均有现有与新增用例 |
| 保存与刷新恢复 | 保存中编辑保留 dirty；普通草稿只在当前历史项保存 ID/会话指纹，刷新重读授权服务端快照；第四步恢复输出参数，第五步恢复后提交禁用；不重复 POST/PATCH |
| 预检查失效 | 表单字段集合和源修订变化立即清除预览、预检与确认；预览/预检发起中的配置变化及迟到失败不能解锁旧状态 |
| 提交防重 | 生命周期忙状态、当前修订/指纹/节点/完整性门禁及原有确认 UI；双调用只发一次提交，卸载后的结果不导航 |
| 请求竞态 | 引用列表、草稿读取、预览、预检、批量/分类目录分别使用版本边界；分类刷新结果不被迟到批量覆盖；切库后的 PENDING 不建立旧轮询 |
| 组件卸载 | 页面 AbortSignal 传入既有 API 客户端；轮询/debounce 停止并唤醒等待；回填期间卸载不发布草稿或读取预览 |

HTTP 取消不代表取消服务端已受理的操作。目录失效仅停止客户端轮询并丢弃旧响应，后端没有逐查询取消 API。普通刷新恢复最后保存快照，未保存的步骤 3/4 参数不保证恢复。步骤 2 原有同会话恢复继续保留。

## 规范更新与开发入口

| 既有入口 | 本轮补充 |
| --- | --- |
| `AGENTS.md` §7.4 | 三类向导约束、维护索引、追加功能前的所有权/职责复核及异步负例 |
| `frontend-platform-baseline.md` | 沿用 workbench 组织；区分强制规则、人工建议、自动检查和必要验收；两类导入后续准入与实现方式 |
| `docs/README.md` | 统一规范和维护索引导航 |
| `development-task-map.md` | 本轮实现事实、恢复边界、验收证据及真实能力门禁不变 |
| `export-general-contract.md` §0.2 | 普通保存恢复、派生入口区别、失效和页面取消边界；不增加后端字段 |
| ESLint / `audit:wizards` | 阻断步骤 API 值导入、feature 反向依赖 views、feature 值依赖循环及步骤/入口直接全局网络调用；保留类型导入 |
| `verify.ps1` / `verify.sh` | 接入向导架构检查；检查器有自动化负例 |
| `check-secrets.ps1` | 密码参数须有独立参数边界，避免 HTML `custom-placeholder` 误报；合成正/反例核验通过 |
| `generate-tokens.mjs` | 内容比较忽略 LF/CRLF 编码差异，继续严格拒绝实际 Token 漂移 |

未另建冲突规范或更改设计权威。文件总行数大于 400 / 脚本大于 300 只输出 REVIEW，不作为机械拆分门禁。当前四个信号为纯 DTO 校验、目录管理、草稿事务和参数联动；人工复核确认各有单一业务能力，无循环值依赖或共享表单副本，后续职责增加时重新评估。

未来普通导入在 `workbench/import/normal/`、旁路导入在 `workbench/import/direct-load/` 按已获准真实能力开发：入口装配、唯一表单、真实步骤、明确规则、异步端口和恢复策略，并补同类失败负例。只有两类导入确实验证可复用的能力才进入 `import/shared/`；跨业务 UI/composable 延续现有目录，不复制导出参数或提前建公共引擎。

## 检查结果

环境：Windows AMD64；Node 24.16.0 / npm 11.13.0，使用现有锁定依赖。没有新增运行或开发依赖。

| 检查 | 实际结果 |
| --- | --- |
| 修改前 Vitest | 19 文件 / 170 用例通过 |
| 最终 `npm test` | 20 文件 / 191 用例通过；新增 20 个状态/异步/恢复用例及 1 个 API AbortSignal 用例，原测试保留 |
| `npm run audit:wizards` | 检查器 3 项测试通过，0 架构违规，4 人工复核信号 |
| `npm run lint` | 通过 |
| `npm run typecheck` | 通过；最终生产构建也包含 vue-tsc |
| `npm run build` | 通过，仍提示大于 500 kB 的既有主包/Monaco 分块 |
| 完整 Playwright | 56 项执行：55 通过，1 旧标题断言失败；修正当前页面标题后，刷新/SQL 恢复/草稿更新/节点换绑 4 项定向复验全部通过。56 个用例均已有通过记录，未声称最后一次完整执行无失败 |
| `git diff --check` | 通过 |
| `scripts/check-secrets.ps1` | 通过；密码选项、私钥标记正例及 HTML 属性负例核验通过 |
| `npm run tokens:check` | **未通过**：既有开发/测试环境颜色与生成源有 4 项差异，未为此次重构更改全局配色 |
| `npm run audit:platform` | **未通过**：当前 28 项既有选择器/Legacy 问题；以重构前入口快照和未变平台文件对照为 30 项，本轮清除导出未引用占位样式及裸图标字号 2 项，没有新增违规 |

静态平台剩余项包含 Monaco 运行时类名的静态识别不足、平台历史树/预检选择器及 SqlQueryEditor、Shell、SourceEditor 的 4 个 Legacy 声明。它们属于既有审计阻断，不能把前端整体门禁标为通过。没有运行根目录整体 `verify` 或 Go 测试；本次没有后端实现改动，不以其他检查替代整体门禁。

## 浏览器与视觉证据

浏览器使用仓库 Playwright，独立 Vite `127.0.0.1:15174`、合成 API、系统 Chrome；200% 缩放用例沿用仓库隔离 Chromium。没有代理开发控制面、真实数据库、Agent、凭据解析或 OBDUMPER。测试子进程仅临时清空不兼容 SOCKS 的代理环境。

完整结果与失败 trace 保留在本机 `C:/Users/Dionysus/AppData/Local/Temp/ob-export-wizard-final/`；最终定向复验在 `ob-export-wizard-recheck/`；重构前截图在 `ob-export-wizard-before/`。静态审计对照在 `ob-export-refactor/platform-before.json` 与仓库忽略的 `artifacts/frontend-platform-audit.json`。这些是本机证据，不属于可提交业务产物。

人工查看第三步与第四步前后截图，布局与参数视觉保留；三张共同截图逐字节相同（对象选择区、其他选项工作面和其他选项布局）。其他截图存在 Tooltip、焦点/动画或整页截图的滚动/固定层差异，不能声称所有截图像素相同。自动断言覆盖三档视口、窄屏、原生 200% 缩放、区域滚动、控件/图标尺寸、说明交互与焦点。

完整回归中的数据源高级设置用例触发既有 `ResizeObserver loop completed with undelivered notifications` 开发服务器警告，该用例操作断言通过，但不声明开发日志零警告。静态架构检查不能证明全部动态调用、状态语义、远端事实或真实环境能力。真实功能、准入、部署与上线验证状态均未被此次前端重构提升。

## GitHub 推送前补充检查

本节记录重构验收之后的提交准备：用户明确要求包含此前未提交的全部相关源码和文档，仍排除 `ob-data/` 本地工具日志、临时产物及运行数据。因此提交范围还包含此前的导出参数、目录、结果集、探针及契约改动，不能将整次提交描述为仅前端重构。

补跑 `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...`、对应 `go vet` 和 `gofmt -l` 均通过；此前前端验收仍有效。Linux 秘密扫描同步修正密码参数边界，并在 Git Bash 下检查源码及合成正/反例。GitHub Actions 的 Web quality 增加 `audit:wizards`，使同一架构检查在推送和 PR 时执行。已有 Token/样式审计阻断不由这些检查替代，未执行真实数据库/工具验证或部署。
