# 首页维护入口

2026-11-02 按用户要求提升曲线舒适度：以本条及首页 P1 取代下述旧细线与禁止表现插值的规则。`overviewTrend.ts` 是有界连线、整数刻度/顶部留白、装饰性主系列底色和真实坐标的唯一所有者；各系列共用区间过渡，保持端点范围、零平台及总数/分项关系，不产生额外计数或查询参数。`OverviewTrendPlot.vue` 统一持有上方图例、200px 默认绘图区、放大图适配及完整 UTC 窗口；CSS 圆形标记按同一坐标定位，独立于 SVG 宽高缩放，不增加尺寸观察器。统计说明归 `OverviewTrend.vue` 页头既有 Tooltip，快照/授权/下钻事务及固定外框不变。7 项计算测试覆盖曲线边界、刻度和分项关系，浏览器验证标记对齐、常规视口完整呈现及窄屏内部滚动。

2026-10-31 曲线参照内嵌 OCP 监控图：`OverviewTrendPlot.vue` 统一持有首页/放大图的圆角细线、浅实线网格、线段式 Ant Button 图例及白底 Tooltip。系列按钮用 `aria-pressed`，隐藏态同时划线；鼠标/键盘共享唯一读数，鼠标离开恢复当前键盘桶。`overviewTrend.ts` 的真实桶坐标、计数、路径及下钻不变，既有固定外框保留；不增加插值、统计指标、全局 Token 或图表依赖。浏览器覆盖图例鼠标/键盘切换、提示唯一性及焦点读数恢复。

2026-10-31 后续按用户要求固定摘要区：趋势与需要关注各 420px、我的任务 360px，所有正文状态使用同一外框；标题与动作不进入滚动正文。`overview.css` 的 `.overview-fixed-panel` / `.overview-panel-body` 是高度与滚动的唯一样式入口，三个分区分别提供具名、可聚焦的原生正文区域，保留键盘 Home/End、鼠标纵向滚动及完整事实；任务表自身继续承接必要横向滚动。执行节点区按内容增高、API 有限列表与弹窗会话不变。这条取代下条在三个摘要区使用紧凑空态高度的说明；回归覆盖空态/长内容/失败/受限往返和 1440px、720px 内部滚动。

2026-10-31 按用户主色调与 OMS 对照整改：图集统一蓝白，四卡缩小，空态用 `.overview-empty` 收紧，页头完整 UTC 时间和今日口径保留键盘提示，旧资源值直接标记过期/未知。“查看对应任务”仅替换原分析入口文案。服务端 `internal/overview.Describe` 仍是受控摘要的唯一所有者，省略零计数并优先异常；前端不解析、删词或重新汇总摘要。静态素材蓝不替代业务状态与趋势语义色，Token 与 Shell 不变。

2026-10-28 DEC-068 增补立体插画、独立放大与授权任务下钻；新增验证入口为 `overviewTrend.test.ts`、`overviewAnalysisLifecycle.test.ts`，完整浏览器交互仍在 `tests/overview.spec.ts`。本地页面与合成验收见[首页证据](../../../../docs/03-technical/evidence/home-overview-2026-10-23.md#2026-10-28-立体指标与独立趋势弹窗dec-068)。

首页使用按 DEC-066 修订的 Overview Archetype，P0 是 `design-system/MASTER.md`、`DESIGN.md`，页面规则为 `pages/overview.md`；布局规则已确认，单页视觉定版须依实际验收。业务沿用 `docs/02-design/home-module.md`，接口与时间/授权规则见 `docs/03-technical/api-sqlite-data-contract.md` §5.1.1。

| 能力 | 唯一所有者 |
| --- | --- |
| 路由装配 | `views/HomeView.vue` |
| 快照、范围、串行读取、定时器、授权与迟到隔离 | `useOverview.ts` |
| 只读传输与严格 DTO 校验 | `api/overview.ts`，经既有 `browserApi` 安全传输 |
| 页头、整页反馈、分区顺序与重试分发 | `OverviewWorkspace.vue`；共用分区样式在 `overview.css` |
| 四项独立任务指标卡 | `OverviewMetrics.vue`；立体图集与生成说明在 `assets/`；节点接收统计只在节点区 |
| 需要关注：受控摘要、任务失败与节点不可接收事实 | `OverviewAttention.vue`；节点原因只从同一快照的可见节点补充 |
| 执行节点紧凑表格 | `OverviewNodes.vue`；原因全部以文本列出 |
| 本人重点任务 | `OverviewMyTasks.vue` |
| 唯一趋势入口、范围单选、图例与互斥弹窗会话 | `OverviewTrend.vue`；捕获图表快照，范围/失权变化关闭；页面不管理任务读取 |
| 图形、圆形标记、命中区、Tooltip、图例及 UTC 窗口呈现 | `OverviewTrendPlot.vue`；SVG 只绘制曲线，CSS 标记与同一坐标对齐，Ant Button 命中区按真实相邻点中线划分，支持键盘下钻 |
| 时间位置、有界连线、整数刻度与允许下钻的固定窗口 | `overviewTrend.ts`；仅作几何过渡，不生成计数、预测或新统计 |
| 分析筛选意图与任务读取绑定 | `useOverviewAnalysis.ts` → 既有 `tasks/useTaskList.ts`；后者仍是请求、游标、错误和迟到隔离的唯一所有者，无写操作、额外候选请求或轮询 |
| 分析窗口任务列表与详情入口 | `OverviewTrendAnalysis.vue`；关闭立即卸载，授权失效通知 `useOverview` 清全页，详情新标签页打开 |
| UTC 时间、分桶标签和同口径下钻 | `overviewPresentation.ts`；任务中心 `taskListQuery.ts` 持有下钻筛选 |

结构按 [DEC-066](../../../../docs/01-product/decisions.md#dec-066-ocp-参考首页布局)及当前页面规则：页头 → 四项指标卡 → 任务结果趋势 / 需要关注（3:2）→ 我的任务通栏 → 执行节点通栏。容器不足 1000px 时趋势与关注单列，不足 900px 时卡片两列，不足 420px 时一列；DOM 顺序不变，两张明细表不再被并列布局挤压。分区只接收快照分区与 `busy`，不持有读取状态；重试统一回到 `useOverview.refresh`。`overviewTable.ts` 仅提供三张表格的原生命名渲染端口，保留 Ant 属性和插槽，不增加表格状态或基础控件包装。

只使用既有 Ant Button、Radio、Badge、Tag、Alert、Empty、Skeleton、Table、Tooltip、Select 和 Modal。按 DEC-068 仅四卡允许生成的立体插画及既有轻阴影；不新增 Token、Ant Wrapper、图表库或 Shell。任务状态/进度、节点原因复用原纯展示规则，分析任务读取直接复用 `useTaskList`，不复制授权、DTO、分页或异步状态；首页不持有秘密，不操作任务和节点。

当前平台时区为明确的 `BUILTIN_UTC`，不是已接入系统设置。异常只覆盖已有任务终态和可见节点当前事实；节点范围不可见与未注册无法安全区分时统一收缩为受限，不输出计数。所有工程默认与限制见 API 契约，不在这里维护第二份能力状态表。

首页不重复用途标语和逐行事实来源标签；受控摘要位于“需要关注”正文首段，覆盖范围统一在该区说明。今日统计口径使用可键盘访问的 Tooltip，UTC、授权、更新时间、受限/失败及实际节点原因仍来自同一快照，不新增本地摘要或事实副本。趋势坐标文字采用普通排版，不随 SVG 的宽高缩放，SVG 继续只承担绘制。

“我的任务”仅在可信记录非空时挂载 Table，可信空结果直接使用 Empty；节点继续按契约在无可见对象时返回 RESTRICTED，不展示猜测零值。范围受限和读取失败仍各自表达，不能当成可信空结果。浏览器回归通过原生 `window.error` 跨导航收集异常，覆盖 `pageerror` 未上报的尺寸测量错误，不改写或抑制观察器。

验证：`src/api/overview.test.ts`、本目录 `overviewLifecycle.test.ts` / `overviewNavigation.test.ts`、`tests/overview.spec.ts`。浏览器使用隔离 Vite、合成 API、受控 Chrome 与现有原生缩放扩展；不触达运行控制面、真实数据源、Agent 或官方工具。
