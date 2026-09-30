# UI Design System Consolidation Gap Audit（2026-09-17）

> 范围：设计系统文档、Canonical Token、Ant Design Vue Theme、Product Layout CSS、四个 Reference Page（Overview、Data Sources、Export Builder、Task Detail）。本审计先于结构性调整完成，用于记录本轮判断依据；它不是新的 UI 规范源。

## 审计输入

- `design-system/MASTER.md`
- `design-system/pages/data-sources.md`
- `design-system/pages/overview.md`
- `design-system/pages/export-builder.md`
- `design-system/pages/task-detail.md`
- `docs/03-technical/frontend-platform-baseline.md`
- `web/src/platform/tokens.ts`
- `web/src/platform/theme.ts`
- `web/src/platform/tokens.css`
- `web/src/platform/archetypes.css`
- `web/src/platform/components.css`
- `web/src/platform/shell.css`
- `web/src/platform/sources.css`
- 四个 Reference Page 现役入口：`HomeView.vue`、`SourceWorkspace.vue`、`ExportWizardView.vue`、`TaskDetailView.vue`

## Existing

- Ant Design Vue 已经是唯一 Generic UI Component System；基础 Button/Input/Select/Form/Drawer/Table/Badge/Alert/Collapse 等均不再由自研 Primitive 接管。
- `web/src/platform/tokens.ts` 已经是实现侧唯一视觉值源，`theme.ts` 消费同源 token，`tokens.css` 由 `npm run tokens:generate` 生成。
- Product Layout CSS 已按入口拆分为 `shell.css`、`archetypes.css`、`sources.css`、`components.css`；页面 scoped CSS 主要承担业务布局。
- 现役产品组件剩余为 Product Feature / Product Behavior：`OrchOperationalTable`、`OrchDangerConfirm`、`OrchSourceActions`、`OrchTaskStepRail`、`useAntDrawerDialog`。
- 四个 Reference Page 已存在浏览器回归：三档视口、数据源抽屉、导出步骤、任务详情证据区均有 Playwright 覆盖。
- 数据源 P1 已沉淀 Management/Table archetype、独立 Save/Test、游标分页、服务端资格和删除确认规则。

## Missing

- 缺少 `design-system/DESIGN.md`，导致视觉方向、字体、颜色、密度、Ant ownership、CSS 禁区和开发前阅读顺序分散在 MASTER 与技术基线中。
- 缺少 `design-system/tokens/` 的面向设计与实现的 token taxonomy，无法清楚区分 Foundation、Semantic、Ant Theme mapping 与 Product Layout CSS Variables。
- 缺少 `design-system/patterns/`，表格、表单、抽屉、状态反馈、动作层级、Task Builder 与 Diagnostic Detail 的可复用规则都混在 MASTER 或页面 P1 中。
- Overview、Export Builder、Task Detail 仍是 `PAGE SPEC NOT YET READY`，只能作为 Reference Page 验证对象，不能作为 P1 例外源。
- 缺少一份明确说明 UI/UX Pro Max 哪些被采用、修改和拒绝的 mapping。

## Duplicate

- MASTER 同时描述 Product IA、Shell、Page Archetype、视觉 token、组件视觉契约、页面规则和质量门禁，职责过宽。
- `frontend-platform-baseline.md` 和 MASTER 均记录 token 层级、Ant 例外与实现入口，容易把技术事实误读为设计权威。
- Data Sources P1 仍引用已删除的旧组件名作为“共享组件”，与 Ant Native Consolidation 的实现事实重复且过期。
- 技术基线、MASTER 和代码注释都记录旧 `40 / 47 / 48px` 冻结密度，和本轮目标的 `36px control / 40-44px row` 方向重复冲突。

## Conflict

- 本轮目标采用 B2B Service neutral palette：`#0F172A`、`#475569`、`#0369A1`、`#F8FAFC`、`#E2E8F0`；现役 token 仍为旧数据源冻结蓝 `#0F62FE / #006AFF` 与页面底色 `#F3F6FC`。
- 本轮要求中文 UI 字体为 `Noto Sans SC` + 系统 CJK fallback；现役 token 为 `"Microsoft YaHei UI", "Segoe UI", system-ui, sans-serif`。
- 本轮要求负字距不得用于中文；`StorageCredentialsView.vue` 存在 `letter-spacing: -.01em`。
- 本轮要求 Ant 管理 Generic Interaction State；当前 `.ant-*` selector 仍存在，但经审计主要是 Ant Vue 4.2.6 无 token API 的产品几何和布局适配，不能扩大为基础组件重绘。
- 本轮要求 DESIGN 与 MASTER 分权；现有 MASTER 将视觉 token 与产品结构绑定，后续页面开发容易从 MASTER 复制历史视觉值。

## Obsolete

- 文档中残留 `OrchField`、`OrchStatus`、`OrchInspectorDrawer`、`OrchDock`、`Workbench*`、`visual-foundation` 旧入口说明；其中多数只应作为历史迁移证据，而不是现役开发入口。
- `docs/03-technical/evidence/frontend-foundation-audit/*.json` 中的旧 Primitive、旧 CSS 和历史裸色仅供追溯，不应参与本轮现役规范搜索。
- `enterprise-*`、`style.css`、旧 Workbench Primitive 与 Lucide 迁移描述只作为历史事实保留，不再作为设计系统事实源。

## 审计结论

本轮应采取“文档分权 + token 校准 + 最小 UI 修正”的收敛路径：

1. 新建 `DESIGN.md` 作为 Frontend UI Design & Implementation Contract。
2. 将 MASTER 收敛为 Product IA、Global Shell、Page Archetype、Navigation 和重大结构。
3. 新建 `tokens/` 与 `patterns/`，将视觉 token 与重复 UI pattern 从 MASTER 中拆出。
4. 调整 `tokens.ts` 到 B2B Service inspired neutral + restrained blue，并重新生成 `tokens.css`。
5. 保留 Ant selector 例外清单，但禁止扩展为 Button/Input/Form 等第二套状态体系。
6. 四个 Reference Page 仅做验证与必要修正，不改变 API、状态机、字段、任务流程或信息架构。

## 本轮处理结果

- 已新增 `design-system/DESIGN.md`、`design-system/tokens/README.md` 与 `design-system/patterns/`。
- 已将 `design-system/MASTER.md` 收敛为 Product IA / Shell / Archetype / Navigation 权威。
- 已将 `web/src/platform/tokens.ts` 切换到 B2B Service inspired neutral + restrained blue，并重新生成 `tokens.css`。
- 已移除存储凭据页中文标题负字距。
- 已将执行节点详情页的 code 字体改为 canonical mono token。
- 已同步 `docs/03-technical/frontend-platform-baseline.md` 的 token 与 Ant selector 例外说明。
- `npm run tokens:check` 通过。
- `npm run audit:platform` 通过：1759 条 CSS 声明，Confirmed Product Decision 3，Canonical Token 977，Component Exception 779，Legacy / Duplicate / Dead 均为 0，`important` 仅 reduced-motion 3 条。
- `npm run typecheck`、`npm run lint`、Vitest 18/18 文件 152/152 测试、`npm run build` 通过。
- Playwright 完整浏览器回归 25/25 通过，证据输出到 `artifacts/ui-design-system-consolidation-browser-final/`，覆盖四个 Reference Page、三档视口、原生 200% 缩放、Drawer 焦点、权限拒绝、独立 Save/Test、长值、行菜单和错误状态。
- `npm audit` 0 漏洞，`scripts/check-secrets.ps1` 通过，`git diff --check` 退出码 0。

## 保留限制

- 浏览器回归仍使用隔离 Playwright 与合成响应，未执行真实数据库、真实凭据、真实官方工具或发布部署验证。
- Vite 开发服务器在数据源高级设置路径仍输出既有 `ResizeObserver loop completed with undelivered notifications` 警告；对应测试通过，本轮不把它声明为已修复。
- Production build 仍有既有主包超过 500 kB 提示；可后续独立评估路由分包。
