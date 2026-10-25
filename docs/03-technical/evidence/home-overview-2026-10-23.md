# 首页只读运行概览验收 · 2026-10-23

本轮按用户“继续”完成首页运行概览的本地工程切片：四项精确指标、唯一任务趋势、授权节点、本人重点任务和有限异常，以及任务中心同口径下钻。最终 Go 回归、606 项前端单测、16 项隔离浏览器验收及八项适用视觉门槛通过。源码未提交，现役控制面、Agent 和 SQLite 未安装或升级；没有新增真实数据库、凭据或官方工具验证。

## 实现与版本

- 契约：[API 与 SQLite §5.1.1](../api-sqlite-data-contract.md#511-首页只读运行快照2026-10-23)、`contracts/openapi.json`；职责入口：[首页维护索引](../../../web/src/workbench/overview/README.md)。
- 工作树基于 `0c5953cdbf406c01ee89ac9018af90a123d67b64`，包含大量既有未提交改动；不以 HEAD 代表本次实现，也不将共享文件的全部差异归于本轮。
- 全仓工作树内容指纹（1022 个 Git 已跟踪及未忽略新文件，排除本配对记录以避免递归）：`47cddf95901c3872214410bfd246a567d1b10f17b61e61e0020414f12160c29c`。
- 本切片 32 个关键实现文件指纹：`a8e1a456ea3f5b34fa894500c7a265abc35bd5bafa7f7694284d011d574c4884`。逐文件 SHA-256、11 张最终截图 SHA-256、聚合算法与检查结果保存在[配对结果 JSON](home-overview-2026-10-23-results.json)；截图保留于已忽略的 `artifacts/home-overview-20261023/browser/`，不提交到 Git。
- 环境：Windows AMD64，Go 1.26.4，Node 24.16.0 / npm 11.13.0；普通用例 Chrome 156.0.8078.48，原生缩放用例 Chromium 153.0.8010.12。
- 浏览器使用仓库 Playwright 与隔离 Vite `127.0.0.1:15174`，业务响应全部合成，不代理运行控制面。Browser 插件不可用，按已读取的前端测试技能采用仓库现有 Playwright；只在测试进程调整代理环境，不修改用户系统代理。

## 事实与权限口径

`internal/overview` 持有时间窗口、安全 DTO、摘要与有限投影；Store 在单个短只读事务内聚合已有事实；HTTP 只装配固定入口和授权。没有新增表、迁移、任意查询、进程或执行端口。

四项指标严格使用运行、等待、今日成功、今日失败的已有状态，不将核对状态扩写为运行或成功。当前时区明确为 `UTC / BUILTIN_UTC`：今日按 UTC 自然日，24 小时为精确滚动窗口且允许首尾部分小时桶，7 天为六天前 UTC 零点至快照时刻。终态窗口均为 `[from,to)`；取消只进入终态总数。指标和图点使用任务中心的 `finishedFrom/finishedTo`，条件可见、可移除，非法窗口不读取任务，翻页游标绑定该窗口。

任务统计与任务列表复用同一对象授权谓词；“我的任务”只含当前主体创建的任务。节点先读取受控标识并授权，再将授权标识绑定到事实和预算查询，事务后再次复验授权，再计数、排序和取前五条。隐藏节点的坏事实不能影响可见分区；没有可见节点时统一返回受限，不区分隐藏对象与未注册，不附计数。

只有 READY 分区携带数据；读取失败、未知、受限和真实零值分别表达。全部分区不可用时返回整次失败，保留客户端旧快照。浏览器严格拒绝未知字段、非法计数、矛盾节点事实、非 READY 携带数据和无有效分区的快照。自然语言摘要不将缺失信息描述为正常。

## 生命周期与负例

`useOverview.ts` 是范围、整份快照、请求和定时器的唯一所有者。默认可见时串行 30 秒刷新、90 秒标过期；方法入口去重，范围切换保留带旧范围说明的完整旧快照，成功后整份替换，不拼接新旧分区。A → B → A、迟到成功/失败/finally、卸载、取消、401/403 和权限撤销均验证；授权失效清除旧事实并停止自动刷新。

单元/契约负例还覆盖 UTC 午夜、精确边界、取消计数、本人/受托/无权任务、身份停用、查询取消、局部读取失败、授权在读取期间撤销、未知与重复查询参数、隐藏节点坏数据、全部分区失败，以及进度证据和安全计数失败关闭。节点异常时间只标“本次核对时间”，不伪造状态发生时间；任务失败使用真实终态时间。

## 实际检查

| 检查 | 最终结果 |
| --- | --- |
| `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` | 通过 |
| 相同范围 `go vet` | 通过 |
| Windows AMD64 `CGO_ENABLED=0 go build -trimpath` 控制面 | 通过；独立临时输出已删除，未替换服务 |
| `npm run test` | 51 文件、606 项通过 |
| `npm run test:browser -- tests/overview.spec.ts --output=../artifacts/home-overview-20261023/browser` | 最终 16 项全部通过；该目录 `.last-run.json` 为 passed |
| `npm run build` | 类型检查和生产构建通过；保留既有 chunk 超过 500 kB 提示 |
| `npm run lint -- --quiet`、`npm run tokens:check` | 通过 |
| `npm run audit:business` | 0 硬违规、19 REVIEW |
| `npm run audit:platform` | 0 硬违规、105 REVIEW、7 个运行时引用 |
| `scripts/check-secrets.ps1`、`git -c core.safecrlf=false diff --check` | 通过 |

浏览器最终 16 项覆盖三档正常页面、指标下钻及移除、趋势图点键盘下钻、非法终态窗口、空态/局部失败/过期/拒绝/非法响应、刷新失败后撤权、长文本、范围读取期间去重、原生 200% 缩放及初次加载。原生缩放通过已有测试扩展设置真实浏览器 zoom=2，核验 devicePixelRatio 翻倍、无整页横向溢出，并用键盘激活刷新和系列开关；没有用 CSS zoom 或缩小字体替代。

三档正常首页的 pageerror 和 console error 断言为空；任务中心及空态转换仍可出现既有 Ant/Vite `ResizeObserver loop completed with undelivered notifications` 开发诊断，沿用[既有记录](task-center-phases2-3-2026-10-14.md)。这不记为开发服务零诊断。中间代理与隐藏 radio 定位导致的测试失败经修正后完整重跑，本表只将最终通过批次计为验收结果。

## 八项视觉与交互门槛

比较基准为当前 P0 Overview Archetype、数据源定版的字体/控件/工作面复用规则，以及既有任务状态、进度和 Ant 表格；首页业务结构按 Overview 表达，不套管理列表布局。本轮没有修改共享 Shell、全局 token 或共享控件视觉，因此未扩大到重跑数据源业务流程。以下分别依据源码/审计、实际浏览器交互及最终截图目视检查判定。

| P0 门槛 | 结论 | 当前证据与适用范围 |
| --- | --- | --- |
| 同类型页面遵循统一布局和对齐规则 | 通过 | 单共享运行摘要面、四项指标、唯一趋势；1920/1440 双列、1280 纵向，外壳与间距沿用 P0 |
| 同类控件使用统一尺寸、样式和状态 | 通过 | 直接使用 Ant Button / Segmented / Checkbox / Badge / Alert / Empty / Skeleton；状态具备文字，焦点可见 |
| 表格、表单、抽屉、弹窗的视觉规则一致 | 通过 | 本人任务直接使用 Ant Table 与现有任务进度；本切片没有业务表单、Drawer 或 Modal，其打开/关闭验收不适用 |
| 同类操作具有一致的动效和反馈 | 通过 | 刷新使用原位加载/成功，持续失败和过期用 Ant Alert；范围及重试去重、键盘激活通过；没有保存、危险确认、脏输入和弹层事务，相应项目不适用 |
| 不同模块之间没有明显视觉密度落差 | 通过 | 按数据源定版复用规则比对共同字体、控件与工作面，任务行复用既有表格/状态/进度；无新视觉体系 |
| 1920×1080 实际截图检查通过 | 通过 | 见下表，最终批次实际截图已打开目视检查，正常首页错误断言为空 |
| 1440×1024、1280×720 响应式检查通过 | 通过 | 两档实际截图目视检查、无整页横滚；另验证原生 200% 和长中文/长技术标识 |
| 未发现无依据的模块私有样式或重复实现 | 通过 | 只新增本模块布局与单一 SVG 趋势，不新增 token / Ant Wrapper / Wizard Engine；跨业务 REVIEW 的复核见下文 |

`audit:business` 新增的跨业务 REVIEW 对应对任务状态、`TaskProgress.vue` 和节点资格展示规则的真实只读复用。首页没有接管这些模块的读取、订阅、写事务或状态；共同能力仍由原 feature 所有，不引入反向/循环依赖。页面呈现、趋势呈现、快照生命周期、DTO 与下钻规则各有明确所有者；没有按行数机械拆分。其余既有 REVIEW 未被清零或扩大为本轮整改。

## 最终截图

以下 11 张均来自最终 16 项批次并已打开目视复核。三档截图使用对应真实 viewport，`fullPage` 仅用于保留纵向完整内容；200% 图片为原生缩放后的当前视口。

| 场景 | 截图 |
| --- | --- |
| 1920×1080 | [完整页面](../../../artifacts/home-overview-20261023/browser/overview-首页-1920×1080-显示授权事实并完成范围、系列和刷新交互-chrome/overview-1920.png) |
| 1440×1024 | [完整页面](../../../artifacts/home-overview-20261023/browser/overview-首页-1440×1024-显示授权事实并完成范围、系列和刷新交互-chrome/overview-1440.png) |
| 1280×720 | [完整页面](../../../artifacts/home-overview-20261023/browser/overview-首页-1280×720-显示授权事实并完成范围、系列和刷新交互-chrome/overview-1280.png) |
| 原生 200% | [趋势与可见键盘焦点](../../../artifacts/home-overview-20261023/browser/overview-首页原生-200-缩放下刷新和趋势键盘操作可达-chrome/overview-native-200-percent.png) |
| 初次加载 | [骨架与禁用刷新](../../../artifacts/home-overview-20261023/browser/overview-初次加载保留骨架且不展示猜测指标-chrome/overview-initial-loading.png) |
| 空态 | [真实零值与范围受限](../../../artifacts/home-overview-20261023/browser/overview-首页-empty-状态不展示伪造事实-chrome/overview-empty.png) |
| 部分失败 | [分区错误，不绘制伪零值](../../../artifacts/home-overview-20261023/browser/overview-首页-partial-状态不展示伪造事实-chrome/overview-partial.png) |
| 快照过期 | [最后可信记录与持续提示](../../../artifacts/home-overview-20261023/browser/overview-首页-stale-状态不展示伪造事实-chrome/overview-stale.png) |
| 权限拒绝 | [无旧数据](../../../artifacts/home-overview-20261023/browser/overview-首页-denied-状态不展示伪造事实-chrome/overview-denied.png) |
| 非法响应 | [拒绝展示](../../../artifacts/home-overview-20261023/browser/overview-首页-invalid-状态不展示伪造事实-chrome/overview-invalid.png) |
| 长文本 1280 | [中文与长标识](../../../artifacts/home-overview-20261023/browser/overview-长中文与长标识在窄屏不撑破页面-chrome/overview-long-1280.png) |

## 2026-10-23 布局与组件收敛修订

用户要求“重构首页，不引入未经允许的组件”，并选择布局与视觉重做、Segmented 改 Radio、SVG 下钻改用 Ant 控件。接口、快照生命周期和业务口径不变；上文为修订前记录，本节取代其中的组件与布局描述。

- 结构：`OverviewWorkspace.vue` 只负责页头、整页反馈、两列布局和重试分发；摘要、异常、节点、本人任务、趋势各自成为只接收快照分区的组件，共用样式在 `overview.css`。读取、定时器与授权仍唯一归 `useOverview.ts`。
- 布局：页头复用 `orch-page-header`，授权范围、UTC、快照时间和自动刷新在右侧，刷新为文本 Utility（刷新 / 刷新中 / 已刷新）。摘要通栏，左列“最近失败与异常 → 执行节点”，右列“任务结果趋势 → 我的任务”；容器不足 1000px 按同一顺序单列。分区沿用既有 `content-card` 平面工作面，未新增阴影。
- 组件：范围选择改用组件语义表内的 Radio 按钮组；SVG 改为 `role="img"`，不含链接或可聚焦元素；逐桶下钻改由 Ant Collapse 收纳的 Table + Button 承担，另提供整段终态窗口入口。节点原因改为全部文本，删除自建 `tabindex` 段落和 Tooltip。删除表格 `size="small"`，回到平台默认密度。没有新 token、Wrapper、图表库或组件例外。
- 合成检查（工作树含其他未提交改动，不以 HEAD 代表）：overview 单元 20 项与全量 `npm run test` 53 文件 627 项通过；`vue-tsc -b`、`npm run build`、`tokens:check` 通过；overview 范围 ESLint 通过，全仓 lint 仍有既有 `tests/access-control.spec.ts:40` 的 `no-empty-pattern`，不在本次范围；`audit:platform` 0 硬违规、无首页 REVIEW；`audit:business` 0 硬违规、19 REVIEW 不变（首页仍只读复用任务状态/进度与节点展示规则）；`git diff --check` 通过。`scripts/check-secrets.ps1` 在本机 Windows PowerShell 5.1 解析无 BOM 中文脚本时 `$pattern` 为空而报错，改用相同正则的 `git grep` 等价扫描无命中，脚本本身未修改。
- 浏览器：默认端口 15174 被其他会话占用，改用临时隔离端口 15194（临时配置已删除）；原生缩放用例改为跟随项目 `baseURL`。`tests/overview.spec.ts` 最终批次 18 项中 17 项通过，原生 200% 用例因并行会话在运行中改写首页文件触发 HMR 而超时，随后单独复跑通过。DOM 几何探针（临时，已删除）确认 1920×1080 首屏内包含摘要、异常、趋势图和节点区标题，1440/1280 保持两列且无整页横滚。
- 截图：最终批次截图在已忽略的 `artifacts/home-overview-redesign/`。本轮工具环境无法打开图片，**截图目视检查未验证**，须由用户或后续会话目视确认后才能将八项视觉门槛记为通过；在此之前首页视觉验收状态为“合成验证通过、视觉待确认”。

## 2026-10-23 关注优先结构重构（DEC-060）

用户指出上一节只是局部调整，要求按结构重构，并在三版方案中选定“A 关注优先”。本节取代上一节的布局描述；组件收敛（Radio、SVG 仅绘制、明细下钻、原因文本化）继续有效。接口、快照生命周期与业务口径不变。

- 规范：新增 [DEC-060](../../01-product/decisions.md#dec-060-首页关注优先结构)；修订首页设计 HM-R02、HM-LF-R04、§4 首屏规则；P0 MASTER Overview 骨架、§6 与 Change Control；P1 `overview.md` 记录分区收窄规则，状态仍为 PAGE SPEC NOT YET READY。
- 结构：页头 → 状态带（四项任务指标 + 不可接收节点 N / 可见 M，单一工作面）→ 需要关注（标题下为受控摘要，列表补充同一快照中可见节点的原因）→ 我的任务 / 执行节点（3:2 并排，节点改为 Ant Table）→ 降低高度的任务结果趋势（系列开关并入标题行）。`OverviewSummary.vue` 删除，新增 `OverviewStatusBand.vue`；并行会话补充的“未授权 / 受限”分区状态全部保留。
- 几何（临时探针，已删除）：1920×1080 下状态带 y=146、需要关注 y=286、我的任务 / 执行节点 y=651 均在首屏，趋势 y=1123 位于首屏下；1440、1280 保持并排，整页无横向滚动。
- 合成检查：全量单测 53 文件 627 项、`vue-tsc -b`、`npm run build`、`tokens:check`、overview 范围 ESLint、`git diff --check` 通过；`audit:platform` 0 硬违规、无首页 REVIEW；`audit:business` 0 硬违规，首页 REVIEW 4 项均为对任务状态/进度与节点展示规则的只读复用（新增一项来自“需要关注”的节点原因标签），总数 21 中另 1 项新增不属于首页；secrets 用等价 `git grep` 扫描无命中（PS 5.1 问题同上）。
- 浏览器（临时端口 15194，配置已删除）：`tests/overview.spec.ts` 20 项，新增结构顺序/节点状态格/原因补充与节点受限不出计数两项。整批 19 项通过；1920 用例在冷启动下超过 30 秒触发一次自动刷新，使“范围切换后读取 2 次”的计数断言得到 3 而失败，热启动下三档各重复 2 次共 6 次全部通过。这是用例时序对冷启动敏感，不是页面多发请求。
- 截图：最终批次截图在已忽略的 `artifacts/home-overview-redesign/`。本环境仍无法打开图片，**截图目视检查未验证**；八项视觉门槛待用户目视确认后再判定。

## 完成边界与未验证项

本工程切片完成页面、本地 API、合成回归与适用视觉验收；`design-system/pages/overview.md` 仍为 **PAGE SPEC NOT YET READY**，P4 实现不自动提升为单页 P1 定版。

当前使用明确的内置 UTC，系统设置时区尚未接入。异常仅覆盖已有任务终态与可见节点当前事实，独立调度/工具日志检索及相应入口尚未接入。没有安装新控制面或 Agent，没有升级运行 SQLite，没有执行真实网络、凭据解析、数据库或官方工具操作，也没有进行麒麟原生验证。EX-V1、NIV、DLV、G3/G4 及其他模块状态继续以任务地图和各自证据为准。
