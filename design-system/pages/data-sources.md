# 数据源标准页 · Data Sources Page Specification

> Authority: **P1 — page-specific rule**
> Status: **已定版 · 后续页面视觉与组件标准**
> 定版日期：2026-09-14；依据：用户明确确认与 [DEC-047](../../docs/01-product/decisions.md#dec-047-数据源页面定版与后续页面标准)。
> 初始定版源码：`94adccdd421f54ad22fba0b087448c7ad6379880`；现行规则包含下述已确认修订，最近更新 2026-09-18，路由 `/data-sources`。
> Scope: Data Sources management/list page and its create/edit drawer.
> Read before implementation: [P0 MASTER](../MASTER.md) → this file → the P3 data-source, API, credential, validation, and lifecycle contracts. P3 business facts remain authoritative where this document refers to them.

## 1. Scope and guardrails

This is the concrete **Management / Table** archetype for data sources. Its stable composition is:

`Product Shell → 工作区标题与上下文 → 列表标题及刷新/新建 → 筛选 → 表格与反馈 → 左侧统计/右下角分页 → 编辑抽屉/确认框`

It uses the P0 design language in [DESIGN.md](../DESIGN.md): Minimalism & Swiss Style, restrained B2B neutral palette, medium-high density, one table surface, compact controls, and no permanent inspector. It must not introduce global tokens, alter the Product Shell, or make the drawer a second application shell.

The page manages configuration and exposes safely projected test/lifecycle facts. It does **not** provide direct database access, runtime administration, arbitrary command execution, SQL entry, secret readback, or task configuration.

### 1.1 后续页面复用规则

- 所有页面沿用当前 Product Shell、字体、配色、控件、菜单、状态、校验和弹层语言；从 [DESIGN](../DESIGN.md)、[tokens](../tokens/README.md) 及 `patterns/` 复用，不复制整份数据源 CSS。
- 管理列表沿用一张工作面：标题左侧、刷新/主动作右侧，下面搜索与筛选、表格，底部左侧统计、右下角分页。分页随列表布局，不固定到浏览器窗口；条数和游标能力按各模块 API，不强制所有列表每页 10 条。
- 概览、向导、详情和设置共享视觉与组件，内容结构按其 Archetype；数据源字段、类型菜单、命名步骤、连接测试和 520px 抽屉宽度属于本页规则，不机械复制。
- 后续模块调整共享样式或组件时，同步检查标准页的列表、分页、抽屉与确认框，防止回归。功能修复仍按契约和验证规则进行，新的视觉变更记录到 P0/P1；当前源码不会自动替换冻结版本。

## 2. Page header and action model

### 2.1 Header

- 顶栏只保留产品标识和全局动作；不重复面包屑或页面标题。工作区标题为 `数据源管理`，上下文为 `OceanBase / ODP`。
- The page heading is `数据源管理`. A single short description may explain that sources must be verified before use in a task; it must not repeat table facts or create a KPI strip.
- `新建数据源` 是列表标题同行右侧唯一主动作，刷新位于其旁。只向具备 P3 create/manage capability 的用户提供；展开 `OceanBase MySQL` / `OceanBase Oracle` 类型菜单，再打开固定模式抽屉。
- A page may legitimately have no primary action. `刷新` is always a utility action, never the primary business action.

### 2.2 Toolbar

筛选区位于列表标题下、表格上；顺序是搜索 → 环境 → 测试结果 → 可用状态 → 兼容模式 → 清除筛选。刷新在上方列表标题的动作区，不放进 Product Header。

- Search label: `搜索数据源`; search only documented, authorized list fields: display name, host/ODP endpoint, cluster, and tenant. Do not search secrets, raw connection strings, system credentials, hidden IDs, or records not returned by the server.
- 固定筛选包括环境、连接测试、可用状态（`已启用` / `已停用`）和兼容模式；收窄时在同一筛选区换行，冻结版本没有额外筛选弹层。
- `清除筛选` appears only when a query or filter is active. It clears all active conditions and returns focus to search.
- `刷新` retains the current query, filters, cursor position, and credible rows. It reports refresh failure in the page feedback region rather than replacing rows with an empty state.
- No bulk action is specified until a P3 contract supplies a safe bulk capability.

Production filtering, sorting, and paging must be server-scoped. Fixture-only/local filtering is permitted only inside the explicit DEV fixture and must never imply that an incomplete loaded page is the whole authorized collection.

## 3. Final table model

The table is the principal workspace, not a card grid. It uses P0 dense-row, control, divider, focus, and status tokens. The P0/P1/P2 labels in this section mean **column display priority**, not document authority.

| Priority | Column | Rendered facts and rules |
|---|---|---|
| P0 | Data source | Display name. Secondary text may show the authorized username and default database where present; do not expose passwords, password presence/length, system credentials, raw connection strings, or unreturned fields. |
| P0 | Environment | P3 environment value. It is a business context label, not a success/error status. Production may have a restrained risk treatment but must remain textually explicit. |
| P0 | Endpoint | Structured host/ODP endpoint and port. Long values truncate visually and expose the complete authorized value through an accessible tooltip/copy action. |
| P0 | Cluster / tenant | Structured cluster and tenant facts; never reconstruct these from a raw connection string. |
| P0 | Connection test | 单行圆点与短文案：未测试、测试中、测试成功、连接失败、已失效、已过期、待确认；完成时间和验证详情在抽屉查看，不在列表增加次行。 |
| P0 | Availability | `已启用` / `已停用`，独立于连接测试，不能推导当前可连接或任务资格。 |
| P0 | Actions | Per-row, server-returned lifecycle eligibility controls; see section 8. |
| P1 | Compatibility | MySQL / Oracle 事实；工作区容器宽度不超过 1120px 时按共享样式并入名称次行，不按视口宽度猜测或伪造兼容模式。 |
| P2 | No persistent column | ODP connection kind is fixed product context, and default database/username are secondary identity metadata. System credentials and test runtime facts belong in the drawer. **Runtime is not a data-source table column.** |

The initial sort order and any sortable headers are shown only when a P3/API contract declares supported server-side sort keys. A visual sort affordance without a matching server contract is prohibited. The table has an accessible caption describing its current result scope and active filters.

兼容模式、连接测试和可用状态三列的表头与内容居中，其他事实按现役表格列对齐。名称链接、地址、端口和集群沿用 UI 字体；兼容模式无底色；地址不重复 ODP 次行。环境保留中文名称及开发绿、生产红、测试橙标签；存量预生产仍以黄色标签显示。环境颜色不代表连接或任务资格。

数据源列不展示装饰图标；表头、名称与次行左对齐，不保留图标占位或缩进。环境使用 Ant `Tag` 分类标签，浅色底、对应色文字、无边框；无边框背景通过 `colorFillTertiary` 主题 token 配置。

可用状态切换仅由当前行 `Switch` 展示处理中与服务端返回的结果，不显示顶部成功消息或独立加载提示。失败仍保留明确错误原因；删除保留成功反馈与确认语义。

## 4. Status domains and lifecycle

Never collapse these domains into a single badge.

| Domain | Values / presentation | Meaning and forbidden inference |
|---|---|---|
| Lifecycle availability | `ENABLED`, `DISABLED` | Governs whether a source may be selected for new task work, subject to P3 rechecks. `DISABLED` stops new task use but does not cancel running work. |
| Connection test | no result = `未测试`; `PENDING`/`LEASED` = `测试中`; `SUCCEEDED`; `FAILED`; `UNKNOWN`; `EXPIRED`; `INVALIDATED` | Shows the latest test-run projection. A test result is not permission, performance, import/export, or task-precheck proof. |
| Verification quality | `AGENT_JDBC` with `realConnectionVerified=true`; `G2_SYNTHETIC` | Only a successful real `AGENT_JDBC` verification may satisfy the P3 prerequisite to enable/offer the source for task selection. A synthetic success is labelled explicitly as non-qualifying. |
| Environment / compatibility | P3 facts | Context only; never use the error/success palette as their primary meaning. |

列表 `SUCCEEDED` 使用绿色圆点与“测试成功”，绿色只表示最近记录成功，与“已启用”共用视觉而不合并状态。抽屉只有真实 `AGENT_JDBC` 且 `realConnectionVerified=true` 才能表达“已验证可连接”；合成结果明确标注不可作为真实资格。未知、过期、失效各自保留文案，不能冒充成功。状态均保留文字，颜色只作补充。

### 4.1 Lifecycle and test state machine

```text
Create valid configuration
  → Save
  → stored DISABLED + no test result (未测试)
  → Test on an eligible runtime node
  → PENDING → LEASED → terminal test result
  → real AGENT_JDBC SUCCEEDED
  → server grants enable eligibility
  → explicit Enable
  → ENABLED

Any connection-affecting saved change
  → test result INVALIDATED / 未测试 according to P3 projection
  → task eligibility is removed until a new qualifying test
  → existing lifecycle state is shown exactly as returned by the server; the UI must not silently predict a state transition.
```

Create always persists as `DISABLED`. Test success never auto-enables. `Enable`, `Disable`, `Archive`, and `Delete` are separate, server-authorized lifecycle actions; the frontend renders the server eligibility/reason and never substitutes one action for another.

## 5. Drawer information architecture

Create and edit both use the shared right-side Drawer. The official operating flow is table + drawer; a legacy direct route, if retained, must reuse this same drawer composition rather than grow a parallel standalone form or detail sidebar.

Drawer anatomy:

1. **Header** — `新建数据源` / `编辑数据源`, close control; omit the separate configuration-state strip.
2. **Type and parser** — fixed OceanBase mode, followed by an always-visible optional smart-parser textarea.
3. **Connection address** — host/port and cluster/tenant use two columns in one muted surface group; MySQL alone adds the full-width default database field.
4. **Database account** — username/password use a second muted surface group. A compact `测试连接` control reveals node selection and refresh on one row, then the prerequisite/status hint and separate `开始测试` action on the next row, with results below. The unsaved-source prerequisite remains visible.
5. **Metadata** — environment follows the account group; `默认` is an unselected hint, never a persisted value. New records choose 开发、生产 or 测试 in that order; existing 预生产 records remain readable and editable. Editing also shows the saved name below it. Creating requests the required name in a dialog after connection-field validation and before persistence. Project binding is outside the product contract and is omitted.
6. **Advanced settings** — one collapsed disclosure for optional sys credentials.
7. **Footer** — right-aligned `取消` and primary `确定`. Confirm saves only; it never starts a connection test.

Node selection and results remain inline in the same form when the test area is expanded; the list test action opens that area and focuses node selection. The body scrolls independently; the 520px drawer is capped at the viewport width on narrow screens. Address and account groups use a flat muted surface without nested cards. Required name/tenant/password validation remains unchanged; cluster is optional and participates in the derived ODP identity only when supplied. The naming dialog preserves the connection draft on cancellation or save failure; successful creation keeps the drawer and selected node available for a separate test. The 2026-09-28 layout reference updates grouping and order only; its project selector and direct test semantics are not part of the product contract.

### 5.1 Form grouping and conditions

| Group | Fields and behavior |
|---|---|
| Basic information | Display name (globally unique) and a required environment choice (开发 / 生产 / 测试 for new records; 预生产 remains valid for stored records). Do not add a description field until P3/API supports it. |
| Connection configuration | Fixed ODP connection kind; compatibility mode; host; port; cluster; tenant; username; password; and default database only for MySQL mode. A connection-string parser, if retained, is a local fill helper: it populates structured fields, is never stored or transmitted as an alternative connection representation, and exposes parse errors locally. |
| Credential handling | Password is required when creating. On edit, an empty password means preserve the stored secret; a non-empty value is a replacement and must never be echoed after save. System-account credentials are optional advanced paired inputs. The UI may show only P3-authorized configuration state, never secret content, length, ciphertext, or a secret-presence inference beyond the contract. |
| Connection test | Select an eligible runtime node; expose its safe readiness/compatibility facts in the control/result context. Runtime selection is test context, not source metadata. No test is available before the source is saved. |

The selected type is fixed within each create/edit drawer. Editing uses the saved record type. Parsing a connection string of the other mode rejects the entire fill and preserves the draft. MySQL alone exposes the default database. Fields rendered conditionally must be removed from submitted payloads when inapplicable.

## 6. Validation, dirty state, save, and test

### 6.1 Validation

- Validate a field after it is touched/blurred and validate all relevant fields on Save. Place a concise, programmatic error beside the field; after failed Save, provide a linked error summary at the top of the drawer.
- Enforce P3-required values and formats: display-name uniqueness, environment enum, compatibility mode, structured host, port in `1–65535`, optional cluster, tenant, username, create-time password, conditional MySQL default database, and paired system credentials.
- Client validation improves feedback only. Server validation, uniqueness, authorization, `If-Match` revision, and lifecycle eligibility are decisive. Preserve non-secret user input after a server error and map safe field errors back to fields.
- No browser or client validation may claim that the database is reachable. That fact only comes from the asynchronous P3 test run.

### 6.2 Dirty state and save

- A drawer opens clean from the server snapshot. Dirty includes every editable unsaved value; a separate connection-dirty subset covers endpoint, compatibility, cluster, tenant, username, password, and other P3 connection-affecting values.
- While connection-dirty is true, `测试连接` is disabled with persistent text explaining that testing uses a saved configuration. If only non-connection values are dirty, testing the last saved configuration remains allowed and is explicitly labelled as such.
- Closing, changing the record, or leaving the route while dirty opens a discard dialog. Default focus stays on `继续编辑`; discarding never auto-saves.
- Save uses `If-Match` for an existing record. A revision conflict keeps the draft, reports the conflict, and offers an explicit refresh/review path; it must not merge or overwrite silently.
- On successful Save, clear dirty state, clear one-time secret inputs, refresh the authoritative record, and update the test/lifecycle projection from the response. A save of connection-affecting values never preserves a stale qualifying-test impression.

### 6.3 Save/Test action semantics

`保存` and `测试连接` are distinct business actions.

- `保存` persists the configuration. On creation it produces the P3 default `DISABLED` source with no qualifying test.
- `测试连接` starts an asynchronous test of the saved revision on the selected eligible node. It is unavailable for a new/unsaved source, missing/ineligible node, or unsaved connection changes; the disabled explanation states the applicable reason.
- `保存并测试连接` is **not** part of the final page action model unless a future P3/API contract explicitly authorizes it as a composite business action and defines its two-stage failure/retry semantics. The current P4 convenience implementation is not such authority. Until that contract exists, a successful Save is followed by an intentional, separate `测试连接` action; a failed Save never starts a test.
- During `PENDING` or `LEASED`, freeze mutation controls for the tested configuration and show the selected node, submitted revision, and non-cancellable progress. Closing the drawer does not claim to cancel an accepted test; the refreshed table/result remains the source of truth.
- Terminal success displays only safe run facts: verification source, real-verification qualifier, selected node, node-facts revision, safe code, and completion time. Failure/unknown displays a safe reason, correlation/run reference where authorized, and a next step; raw driver/database diagnostics and secrets are never rendered.

## 7. Result and error states

| State | Required presentation |
|---|---|
| First-use empty | Explain that no authorized data sources exist. Show `新增数据源` only when creation is authorized. Do not fabricate dashboard guidance or cards. |
| Search empty | Preserve the query, state that it matched no authorized source, and offer `清除搜索`. |
| Filter empty | Preserve filters, state that no source meets them, and offer `清除筛选`. With both query and filters, offer `清除全部条件`. |
| Permission restricted | Do not disguise authorization denial as an empty table. Show a restrained restricted-access state, omit unavailable create/actions, and do not reveal hidden record counts, identities, or eligibility reasons. If list access exists but management is absent, show only the permitted list facts and omit the actions column when no permitted row action exists. |
| Initial backend error | Keep the page shell/header, replace the table region with a safe error state and `重试`. Do not infer the error is a connection-test failure. |
| Refresh backend error with credible rows | Keep rows and current scope visible; show a non-blocking inline error with retry and last successful refresh time. |
| Drawer load/save error | Keep non-secret draft input; display safe field/general feedback in the drawer. Authorization/revision errors receive their own recovery path. |
| Test request/poll ambiguity | Do not relabel an uncertain accepted request as `FAILED`. Show `状态待确认` with refresh/reopen guidance until the server returns a terminal projection. |

Loading preserves table geometry with non-deceptive rows/skeletons and never announces fake data as a completed result.

## 8. Row actions and confirmation

Actions are per-record and derived only from P3 server-returned permissions and lifecycle eligibility. The compact action menu may contain:

- `编辑` — opens the edit drawer when management authorization is present.
- `删除` — 依据服务端删除资格决定可用性；点击打开二次确认框，说明对象、不可恢复、凭据清理与历史保留；取消不发送请求，确认才执行。处理中禁用生命周期动作并显示进度，成功后反馈对象名称并刷新列表；失败显示原因并重读资格与版本，不得静默改为归档。

2026-09-16 用户指定行菜单仅保留“编辑”和“删除”；测试连接保留行内入口及编辑抽屉入口，窄屏通过编辑抽屉访问。可用状态使用紧凑圆角滑轨与圆形滑块，蓝色启用、灰色禁用，按用户提供的“自定义设置”截图采用 44×22 滑轨、18px 白色圆形滑块，不内嵌可见文字；保留无障碍状态、键盘焦点及服务端不可用原因。此前浏览器工具连接失败不再作为当前验收状态；2026-09-16 隔离 Playwright 验证范围与结果见[前端平台基线](../../docs/03-technical/frontend-platform-baseline.md)，不据此推断真实业务已上线。

`启用` may execute directly with immediate safe feedback only when P3 allows it. Disable, archive, discard, and other P0 high-risk actions require the shared confirmation dialog. 数据源删除按 2026-09-16 用户修订恢复二次确认。历史引用不阻止删除；等待调度、启动、运行、取消中及待核对任务阻止删除。Disabled or omitted actions must use the server reason; no client-side guessed eligibility, hover-only explanation, or action substitution is permitted.

## 9. Pagination and responsive adaptation

### 9.1 Cursor pagination

分页位于列表底部右侧，与左侧 `共 N 条` 统计同一 Footer。右侧顺序为 `10 条/页 → 上一页 → 第 N 页 → 下一页`；上下页使用具有可访问名称的图标按钮，当前页通过 live region 宣告。空间不足时按共享样式换行，不能用绝对定位遮挡内容。

管理页固定每页 10 条，通过服务端 keyword/environment/state/compatibilityMode/connectionStatus 筛选。nextCursor 为空时下一页禁用，上一页使用本次查询的已访问游标栈，首屏禁用；加载与刷新期间两按钮禁用。第 N 页由已访问游标栈长度加一得到，total 来自服务端同一授权筛选范围；不虚构总页数或任意跳页。条件变化清空游标栈，翻页保留条件，旧异步响应不得覆盖新查询。

### 9.2 Shared viewport behavior

| Viewport | Page behavior |
|---|---|
| 1920×1080 (baseline) | 完整 200px 导航、24px 工作区边距；显示列表事实、搜索和筛选，刷新及新建在列表标题同行右侧。 |
| 1440×1024 | 保留完整 200px 模块出口、24px 工作区边距；筛选按剩余空间换行，列收敛由工作区容器宽度决定。 |
| 1280×720 | 48px 图标轨、24px 工作区边距；搜索和筛选保留可达，表格内部承接必要滚动，分页与抽屉底栏可达。不能因布局使整页横向滚动。 |

列响应式遵循当前 `orch-workspace` 容器查询：不超过 1120px 时兼容模式并入名称次行、行内测试通过编辑抽屉访问；不超过 900px 时表格保留内部横滚。三档视口和键盘检查仍需在后续页面或共享组件变更时实际执行。

The main workspace is the final area to compress. Long table values use truncation/copy/accessible disclosure; responsive behavior must not hide lifecycle availability, connection-test state, or the actions required for an authorized management decision.

## 10. Accessibility requirements

- Use one page `h1`, correctly nested section headings in the drawer, visible labels for search/filter/form controls, and a table caption. Placeholder text is never the only label.
- Table headers expose sort semantics only where implemented. Row actions have the data-source name in their accessible label; menus support keyboard entry, arrow navigation, Escape, focus return, and visible focus.
- Drawer opening moves focus to its heading; focus is contained while open and returns to its invoker on close. Dirty/discard and destructive dialogs have an explicit least-destructive default.
- Associate field errors and top summary links using programmatic descriptions. Announce save/test start, terminal result, and errors through concise live regions without duplicating a result on every polling refresh.
- Status communicates text, icon/shape, and color; meet P0 contrast and focus requirements. Tooltip/copy affordances are keyboard accessible and expose untruncated authorized values safely.
- 桌面控件、表格与字号沿用 P0 第 9 节的数据源定版值；不为塞入列而缩小点击区域、降低对比度或破坏键盘操作。

## 11. 冻结实现入口与维护

初始定版源码只用于追溯；当前实现遵循本文件与 P0 的现行修订。下表为唯一实现入口，后续开发先核对实际路由与样式加载关系。

| 标准部分 | 现役实现 |
|---|---|
| 路由与页面 | [router/index.ts](../../web/src/router/index.ts) → [SourceWorkspace.vue](../../web/src/workbench/sources/SourceWorkspace.vue)；`/workbench/data-sources` 重定向到 `/data-sources` |
| 公共外壳 | [ProductShell.vue](../../web/src/components/ProductShell.vue)、[ProductHeader.vue](../../web/src/components/ProductHeader.vue) |
| 编辑、校验与测试 | [SourceEditor.vue](../../web/src/workbench/sources/SourceEditor.vue)、[useSourceEditor.ts](../../web/src/workbench/sources/useSourceEditor.ts)、[ConnectionTestResult.vue](../../web/src/components/ConnectionTestResult.vue) |
| 共享组件 | Primitive 使用 Ant Design Vue；产品语义由 [OrchSourceActions](../../web/src/workbench/sources/OrchSourceActions.vue)、[OrchOperationalTable](../../web/src/components/OrchOperationalTable.vue)、[OrchDangerConfirm](../../web/src/components/OrchDangerConfirm.vue)、[ConnectionTestResult](../../web/src/components/ConnectionTestResult.vue) 与 [useAntDrawerDialog](../../web/src/composables/useAntDrawerDialog.ts) 承载 |
| 样式 | [main.ts](../../web/src/main.ts) 顺序加载 Ant reset → `platform/tokens.css` → `archetypes.css` → `sources.css` → `shell.css` → `components.css`；唯一值源为 [tokens.ts](../../web/src/platform/tokens.ts)，迁移关系见[前端平台基线](../../docs/03-technical/frontend-platform-baseline.md) |
| 合成验证入口 | DEV 环境 `/data-sources?uiFixture=data-sources`；[样本说明](../../docs/02-design/ui-regression-fixture-data-source.md)与 [sourceGateway.ts](../../web/src/workbench/sources/sourceGateway.ts) |

已删除的旧页面、实验入口和基线图片仅通过 [Git 历史](../../docs/README.md#history) 追溯，不作为现行设计入口。

定版确认视觉与页面交互方向，不等同于业务契约、可访问性矩阵或所有状态已验收。P3 仍是业务目标；行菜单按第 8 节仅保留编辑/删除，不得恢复历史归档入口。不得由视觉定版推导真实业务已实现，也不得将实现缺口复制成其他页面的业务标准。

## 12. Governance record

- **2026-09-28 抽屉布局修订：** 按用户提供的视觉参考调整双列浅灰分组、字段顺序及紧凑测试入口；创建和编辑共享布局，保留现有项目字段边界与独立保存/测试语义。
- **2026-09-18 用户修订：** 状态切换反馈收敛到行内开关；环境使用浅底分类 Tag；移除数据源列图标和遗留缩进。清除本页重复的历史截图说明，当前规则集中在相应章节。

- **2026-09-14 定版：** 用户明确将当前数据源页面确认为后续页面标准，P0 v3.0 与 DEC-047 同步承接。旧 P1 的面包屑、列表测试时间、40px 数据行及待接入分页等描述已被现役基线替代。
- **2026-09-17 Design System Consolidation：** 本页继续作为 Management / Table Reference Page；全局视觉值、字体、色彩、密度和 Ant ownership 改由 `DESIGN.md`、`tokens/` 与 `patterns/` 承担。业务流、字段、服务端资格、Save/Test 独立语义和删除确认不变。
- **P3 contract alignment (2026-09-14):** 管理列表的 limit=10、nextCursor 与授权 total 已在 OpenAPI 和 API/SQLite 契约中对齐；运行服务升级状态见任务地图。
- 本次指定的源码版本因用户确认获得基线身份；其他现有页面及后续未批准修改仍是 P4。后续改动按 P0 变更规则记录，不能以“代码现在如此”覆盖标准或业务契约。
