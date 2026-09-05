# Data Sources Page Specification

> Authority: **P1 — page-specific rule**
> Status: **DATA SOURCES PAGE SPEC READY**
> Scope: Data Sources management/list page and its create/edit drawer.
> Read before implementation: [P0 MASTER](../MASTER.md) → this file → the P3 data-source, API, credential, validation, and lifecycle contracts. P3 business facts remain authoritative where this document refers to them.

## 1. Scope and guardrails

This is the concrete **Management / Table** archetype for data sources. Its stable composition is:

`Product Shell → Breadcrumb → Page Header → Toolbar → Feedback → Dense Table → Cursor Pagination → Edit Drawer / Confirmation Dialog`

It uses the P0 Light Technical Operations Workbench language: a content-first workspace, typography and dividers for ordinary grouping, one table surface, compact controls, and no permanent inspector. It must not introduce global tokens, alter the Product Shell, or make the drawer a second application shell.

The page manages configuration and exposes safely projected test/lifecycle facts. It does **not** provide direct database access, runtime administration, arbitrary command execution, SQL entry, secret readback, or task configuration.

## 2. Page header and action model

### 2.1 Header

- Breadcrumb is `任务配置 / 数据源管理`; it is presented by the shared shell/context pattern, not recreated as local navigation.
- The page heading is `数据源管理`. A single short description may explain that sources must be verified before use in a task; it must not repeat table facts or create a KPI strip.
- `新增数据源` is the only possible primary action. Show it only to a principal with the P3 create/manage capability. It opens the create drawer.
- A page may legitimately have no primary action. `刷新` is always a utility action, never the primary business action.

### 2.2 Toolbar

Toolbar order is: search → high-value filters → active-filter reset → refresh. It belongs below the header and above the table; it is not part of Product Header.

- Search label: `搜索数据源`; search only documented, authorized list fields: display name, host/ODP endpoint, cluster, and tenant. Do not search secrets, raw connection strings, system credentials, hidden IDs, or records not returned by the server.
- Always-available filters: environment, lifecycle availability (`已启用` / `已禁用`), and connection-test state.
- Compatibility mode is a secondary filter: visible at 1920, placed in an overflow/filter popover at narrower widths.
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
| P0 | Connection test | Test-state label plus last completion time where supplied. It is distinct from lifecycle availability. |
| P0 | Availability | `已启用` or `已禁用`; it is the management lifecycle state, not a statement that connectivity is currently valid. |
| P0 | Actions | Per-row, server-returned lifecycle eligibility controls; see section 8. |
| P1 | Compatibility | MySQL / Oracle compatibility fact. At 1440 it may become secondary identity metadata; at 1280 it is hidden from the column model, not replaced by a fabricated value. |
| P2 | No persistent column | ODP connection kind is fixed product context, and default database/username are secondary identity metadata. System credentials and test runtime facts belong in the drawer. **Runtime is not a data-source table column.** |

The initial sort order and any sortable headers are shown only when a P3/API contract declares supported server-side sort keys. A visual sort affordance without a matching server contract is prohibited. The table has an accessible caption describing its current result scope and active filters.

## 4. Status domains and lifecycle

Never collapse these domains into a single badge.

| Domain | Values / presentation | Meaning and forbidden inference |
|---|---|---|
| Lifecycle availability | `ENABLED`, `DISABLED` | Governs whether a source may be selected for new task work, subject to P3 rechecks. `DISABLED` stops new task use but does not cancel running work. |
| Connection test | no result = `未测试`; `PENDING`/`LEASED` = `测试中`; `SUCCEEDED`; `FAILED`; `UNKNOWN`; `EXPIRED`; `INVALIDATED` | Shows the latest test-run projection. A test result is not permission, performance, import/export, or task-precheck proof. |
| Verification quality | `AGENT_JDBC` with `realConnectionVerified=true`; `G2_SYNTHETIC` | Only a successful real `AGENT_JDBC` verification may satisfy the P3 prerequisite to enable/offer the source for task selection. A synthetic success is labelled explicitly as non-qualifying. |
| Environment / compatibility | P3 facts | Context only; never use the error/success palette as their primary meaning. |

`SUCCEEDED` is labelled `已验证可连接` only when the result is real `AGENT_JDBC` verification. A synthetic result stays visibly non-qualifying. `UNKNOWN`, `EXPIRED`, and `INVALIDATED` use a warning/neutral treatment with explanatory text, not a false failure or success. Status always includes text and icon/shape; color is supplementary.

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

1. **Header** — `新增数据源` or `编辑数据源`, close control, and a compact `已保存` / `未保存更改` state. For edit, only safe immutable context may be shown.
2. **Basic information** — display name and environment.
3. **Connection configuration** — structured ODP endpoint and credentials fields; see form grouping below.
4. **Advanced system credentials** — one collapsed disclosure, only when the P3 compatibility/mode permits it.
5. **Connection test** — only for an already saved source. It contains node selection, current test progress/result, and the secondary `测试连接` action.
6. **Footer** — `取消`/close secondary on the left and `保存` as the one primary action on the right. Footer actions remain limited to navigation and persistence; test controls remain in their semantic section.

The drawer body scrolls independently. Ordinary sections are separated by headings, whitespace, and dividers; do not nest cards. The test result is a bounded semantic result surface only while it contains a distinct result/failure fact. No persistent Task Summary or Inspector is allowed here.

### 5.1 Form grouping and conditions

| Group | Fields and behavior |
|---|---|
| Basic information | Display name (globally unique) and environment (`dev` / `test` / `stage` / `prod`). Do not add a description field until P3/API supports it. |
| Connection configuration | Fixed ODP connection kind; compatibility mode; host; port; cluster; tenant; username; password; and default database only for MySQL mode. A connection-string parser, if retained, is a local fill helper: it populates structured fields, is never stored or transmitted as an alternative connection representation, and exposes parse errors locally. |
| Credential handling | Password is required when creating. On edit, an empty password means preserve the stored secret; a non-empty value is a replacement and must never be echoed after save. System-account credentials are optional advanced paired inputs. The UI may show only P3-authorized configuration state, never secret content, length, ciphertext, or a secret-presence inference beyond the contract. |
| Connection test | Select an eligible runtime node; expose its safe readiness/compatibility facts in the control/result context. Runtime selection is test context, not source metadata. No test is available before the source is saved. |

Changing compatibility mode removes and clears the incompatible default-database value before persistence. Fields rendered conditionally must be removed from submitted payloads when inapplicable.

## 6. Validation, dirty state, save, and test

### 6.1 Validation

- Validate a field after it is touched/blurred and validate all relevant fields on Save. Place a concise, programmatic error beside the field; after failed Save, provide a linked error summary at the top of the drawer.
- Enforce P3-required values and formats: display-name uniqueness, environment enum, compatibility mode, structured host, port in `1–65535`, cluster, tenant, username, create-time password, conditional MySQL default database, and paired system credentials.
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
- `测试连接` — opens/focuses the Test section for a saved, authorized source; its availability follows section 6.
- `启用` — available only after a server-authorized, qualifying real test. It does not recover historical task eligibility automatically.
- `停用` — explicit confirmation states that new task use is blocked and running tasks are not cancelled.
- `归档` — available only for referenced records; confirmation explains that it preserves historical references and removes the source from new candidates.
- `删除` — available only for unreferenced records; confirmation names the record and is irreversible. A delete ineligibility response must not silently become archive.

`启用` may execute directly with immediate safe feedback only when P3 allows it. Disable, archive, delete, discard, and any other P0 high-risk action require the shared confirmation dialog. Disabled or omitted actions must use the server reason; no client-side guessed eligibility, hover-only explanation, or action substitution is permitted.

## 9. Pagination and responsive adaptation

### 9.1 Cursor pagination

Use the P3 cursor contract. The footer presents `上一页` / `下一页` only when the response supplies the respective cursor. Do not render a fabricated page number, total pages, or total count. A total is shown only when the API explicitly returns an authorized total for the current scope. Preserve query/filter state across cursor changes and return focus to the table caption after navigation.

**P3 contract clarification required before implementation:** the API documentation describes cursor pagination while the specific data-source list response schema must explicitly expose the cursor(s) required by this UI. Reconcile that P3/API detail first. Until then, do not retain current client-side `1 / N` pagination as a production substitute.

### 9.2 Shared viewport behavior

| Viewport | Page behavior |
|---|---|
| 1920×1080 (baseline) | Full P0 and P1 columns; search, primary filters, compatibility filter, and utility refresh visible. The standard edit drawer and dense table occupy the broad workspace. |
| 1440×1024 | Global shell remains visible per P0. Compatibility filter moves to overflow if needed; compatibility column becomes secondary identity metadata before any P0 column is removed. Toolbar may wrap into two compact lines without changing action hierarchy. |
| 1280×720 | Global navigation uses the P0 collapsed rail. P1 compatibility column/filter is hidden or moved to explicit overflow; all P0 decision columns remain. Search remains discoverable, filters collapse into a labelled filter control, and no whole-page horizontal scrolling is allowed. The drawer remains an overlay and preserves its footer. |

The main workspace is the final area to compress. Long table values use truncation/copy/accessible disclosure; responsive behavior must not hide lifecycle availability, connection-test state, or the actions required for an authorized management decision.

## 10. Accessibility requirements

- Use one page `h1`, correctly nested section headings in the drawer, visible labels for search/filter/form controls, and a table caption. Placeholder text is never the only label.
- Table headers expose sort semantics only where implemented. Row actions have the data-source name in their accessible label; menus support keyboard entry, arrow navigation, Escape, focus return, and visible focus.
- Drawer opening moves focus to its heading; focus is contained while open and returns to its invoker on close. Dirty/discard and destructive dialogs have an explicit least-destructive default.
- Associate field errors and top summary links using programmatic descriptions. Announce save/test start, terminal result, and errors through concise live regions without duplicating a result on every polling refresh.
- Status communicates text, icon/shape, and color; meet P0 contrast and focus requirements. Tooltip/copy affordances are keyboard accessible and expose untruncated authorized values safely.
- Dense desktop geometry retains P0 minimum 36px controls and 40px rows. Do not reduce hit targets, type contrast, or keyboard operation merely to fit a column.

## 11. Migration implementation constraints and debt retirement

This P1 specification is the acceptance target for the Data Sources migration. It does not authorize bulk deletion; each item follows the inventory sequence: reference analysis → replacement → `vue-tsc` → build → page verification → dead-code removal.

| Current P4 debt to retire in this migration | Required replacement / verification |
|---|---|
| Ten-column legacy list, including fixture-only `Runtime` and a runtime-derived database label | Adopt section 3. Remove runtime from persistent rows; select node only in the test context. Verify against real authorized API payload and DEV fixture separately. |
| Client-side full-list filtering and `1 / N` pagination | Implement server-scoped query/filter/cursor behavior only after the P3 cursor response is clarified. Do not imply completeness from one loaded page. |
| Card-like table wrapper, legacy 52px rows, raw/scoped table CSS | Use the shared P0 table surface, 40px dense rows, control/focus/status tokens, divider hierarchy, and no duplicate page token values. |
| New-record primary `保存并测试` behavior | Make `保存` the sole primary and remove the composite convenience action unless P3/API later authorizes it. Verify Save and Test state transitions independently. |
| Standalone `DataSourceFormView` detail aside / parallel layout | Reuse one table + Drawer composition; no local inspector or alternate full-page form skeleton. |
| Test status treated as a simple connection badge | Render lifecycle, test projection, and verification quality as separate domains; cover synthetic, invalidated, unknown, and real-success eligibility cases. |
| Page-specific responsive/table/drawer/control styles | Remove or replace after call-site analysis with P0 shared behavior, including 1920/1440/1280 checks, keyboard flow, and permission/error states. |

## 12. Governance record

- **GLOBAL SPEC ISSUE:** None identified. This page spec does not modify P0 Product Shell, tokens, visual language, responsive degradation order, or global action hierarchy.
- **P3 contract clarification:** cursor-response fields for data-source list pagination need explicit API/schema alignment before production pagination work. This is not a P0 defect and must not be solved by inventing a frontend protocol.
- Existing implementation is P4 fact, not design authority. Any conflict found during implementation must be classified as implementation debt, P3 business constraint, or a proposed P0 defect; only the last may initiate a MASTER change.
