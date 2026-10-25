# 首页维护入口

首页使用现有 Overview Archetype，P0 是 `design-system/MASTER.md`、`DESIGN.md` 与 `pages/overview.md`；后者仍未定版为单页 P1 READY。业务沿用 `docs/02-design/home-module.md`，接口与时间/授权规则见 `docs/03-technical/api-sqlite-data-contract.md` §5.1.1。

| 能力 | 唯一所有者 |
| --- | --- |
| 路由装配 | `views/HomeView.vue` |
| 快照、范围、串行读取、定时器、授权与迟到隔离 | `useOverview.ts` |
| 只读传输与严格 DTO 校验 | `api/overview.ts`，经既有 `browserApi` 安全传输 |
| 页头、整页反馈、分区顺序与重试分发 | `OverviewWorkspace.vue`；共用分区样式在 `overview.css` |
| 状态带：四项任务指标与节点不可接收数 | `OverviewStatusBand.vue` |
| 需要关注：受控摘要、任务失败与节点不可接收事实 | `OverviewAttention.vue`；节点原因只从同一快照的可见节点补充 |
| 执行节点紧凑表格 | `OverviewNodes.vue`；原因全部以文本列出 |
| 本人重点任务 | `OverviewMyTasks.vue` |
| 唯一趋势图、范围单选、系列可见性与明细下钻 | `OverviewTrend.vue`；SVG 只绘制、不可交互，下钻由 Ant Table 内的 Button 承担；系列开关不改变统计范围 |
| UTC 时间、分桶标签和同口径下钻 | `overviewPresentation.ts`；任务中心 `taskListQuery.ts` 持有下钻筛选 |

结构按 [DEC-060](../../../../docs/01-product/decisions.md#dec-060-首页关注优先结构)：页头 → 状态带 → 需要关注 → 我的任务 / 执行节点（3:2 并排）→ 任务结果趋势；工作区容器不足 1000px 时第三行单列，顺序不变。分区只接收快照分区与 `busy`，不持有读取状态；重试统一回到 `useOverview.refresh`。

只使用 P0 组件语义表内的 Ant 组件：Button、Radio（时间范围固定值单选）、Checkbox（系列）、Badge、Tag、Alert、Empty、Skeleton、Table（我的任务、执行节点、趋势明细）、Collapse（明细收纳）。不新增 token、Ant Wrapper、图表库或 Shell / Archetype 例外。任务状态、进度与节点原因直接复用原能力的纯展示规则/组件；这是对同一事实的真实复用，不持有这些模块的读取或写入状态，不引入反向依赖、跨模块事务或通用引擎。首页不持有秘密，不操作任务和节点。

当前平台时区为明确的 `BUILTIN_UTC`，不是已接入系统设置。异常只覆盖已有任务终态和可见节点当前事实；节点范围不可见与未注册无法安全区分时统一收缩为受限，不输出计数。所有工程默认与限制见 API 契约，不在这里维护第二份能力状态表。

验证：`src/api/overview.test.ts`、本目录 `overviewLifecycle.test.ts` / `overviewNavigation.test.ts`、`tests/overview.spec.ts`。浏览器使用隔离 Vite、合成 API、受控 Chrome 与现有原生缩放扩展；不触达运行控制面、真实数据源、Agent 或官方工具。
