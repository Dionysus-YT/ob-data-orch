# 前端平台基线

> 状态：DONE。2026-09-16 用户授权的前端基础设施迁移已完成本地与合成验收；本文件不替代 P0/P1 冻结值，也不代表真实业务发布或部署完成。

## 权威与边界

业务、安全、权限与能力契约 → 页面 P1 → P0 → 已确认 DCR → Canonical Token → 平台实现 → Ant Design Vue 默认值。保持 Vue 3、TypeScript、Vite 和现有 API；本次不执行真实数据库、凭据探测或工具启动，也不替换运行后端。

唯一目标关系为 Product Design System → Canonical Token System → Ant Design Vue Theme → Ant Design Vue Components → Product Domain / Feature Composition → Pages。Ant Design Vue 是唯一通用 UI Component System；Feature Component 不等于 UI Component Library。领域状态、权限、游标、版本、异步测试和执行结果继续来自现有领域逻辑与服务端。

## 平台版本与升级规则

| 项目 | 实际基线 |
| --- | --- |
| Node / npm | 24.16.0 / 11.13.0 |
| Vue / Vue Router | 3.5.40 / 5.2.0 |
| TypeScript / vue-tsc | 6.0.3 / 3.3.7 |
| Vite / plugin-vue | 8.1.5 / 6.0.8 |
| Vitest | 4.1.11 |
| Ant Design Vue | 4.2.6，实际项目 Qualification 通过 |
| Playwright | 1.63.0；系统 Chrome 回归，隔离 Chromium 扩展验证原生缩放 |

核心直接依赖使用精确版本，传递依赖由 `web/package-lock.json` 固定；CI/复现使用 `npm ci`。功能修改不得顺带升级 Vue、Router、TypeScript、Vite、plugin-vue、vue-tsc、Ant Design Vue 或测试栈。升级须独立评估兼容性、变更依赖及 lockfile、通过类型/测试/生产构建、四个参考页交互与响应式回归，再更新本基线。Node 和 npm 补丁升级也记录实际验证版本。

Ant Design Vue（MIT）用于替代自研 Primitive，避免继续维护第三套基础交互；不选择 React 或继续扩建自研基础库。Playwright（Apache-2.0）只作为开发依赖提供真实浏览器验收。产物仍是平台无关的静态资源，不引入安装端 Node、CGO 或新后台服务。

本次平台安全补丁独立核验：Vitest 4.1.10 → 4.1.11，更新兼容范围内的 brace-expansion、nanoid、PostCSS 传递依赖，未使用强制或主版本升级；`npm audit` 为 0 漏洞。后续仍按上述独立升级流程执行。

## Token 与样式架构

`web/src/platform/tokens.ts` 是唯一值源，分为 Foundation（色板、字体、间距）、Semantic（界面角色）、Component（控件、表格、焦点、弹层几何）、Product（Shell、Inspector、任务轨、环境标记）。`theme.ts` 消费这些值；`tokens.css` 由 `npm run tokens:generate` 生成，禁止手改，`tokens:check` 检查漂移。2026-09-17 UI Design System Consolidation 后，基线采用 B2B neutral + restrained blue：控件 36px、表头 36px、基础行 42px；包含多行事实/禁用原因的行可自然增高。

入口顺序为 Ant reset → tokens → archetypes → sources → shell → components。`shell.css` 唯一承载 P0 外壳；`archetypes.css` 承载各页面原型与证据布局；`sources.css` 承载数据源领域事实；`components.css` 承载产品组件及已登记的 Ant 适配。页面 scoped CSS 只可处理该页面的业务布局，不重新定义基础控件、色板、字体和主题。

`audit:platform` 按运行入口图分类 ACTIVE / FIXTURE / DEAD，并逐条输出六类样式处置、来源和理由。迁移起点的 Duplicate 是全局同属性同值的候选重复；终态门禁使用同文件、选择器、媒体作用域的重复属性检测，不能直接比较两者的 Duplicate 计数。布局比例、定位、边框形状、可见性及 CSS 关键字登记为 Component Exception，不为 `display:flex` 或某页列宽制造无意义 Token。裸颜色、裸字号、未定义变量、第二个 Token 定义来源、现役旧 Primitive、重复覆盖及未登记 important 会使命令失败。

Ant selector 例外：4.2.6 未提供对应 token 的 Table 36/42px 行高与分区 padding/斑马行；Form.Item 标签和帮助布局；危险确认 footer 的 36px 次级动作；产品布局中的 Select 占宽。仅 reduced-motion 规则的三条 important 用于覆盖框架注入动画，其他样式不得堆叠 important。升级时优先用新版本 Theme API 替换适配。

## 产品组件契约

- `OrchOperationalTable`：Feature Composition，只接收调用方提供的授权事实与列模型，禁用 Ant 内置分页；排序、筛选、Cursor、总数和版本均由原领域/API 流程负责。
- Ant `Drawer` + `useAntDrawerDialog`：页面直接使用 Ant Drawer；关闭只是意图，由页面执行 dirty / busy / 异步测试守卫。`useAntDrawerDialog` 只补齐 Ant Vue 4.2.6 未转发的 ARIA、焦点约束和关闭后焦点恢复，不接管 Drawer 视觉、结构、动画或默认关闭按钮。
- `OrchDangerConfirm`：明确对象、影响与动作；安全取消初始焦点，busy 阻止退出，关闭恢复触发点；不把打开确认等同于取得服务端资格。
- Ant `FormItem`：字段 label、required、help/error、extra 和校验视觉均由 Ant FormItem 直接承担；领域校验和 API field error 只提供状态与文案，不再通过 Orch wrapper 绘制字段 UI。
- `OrchSourceActions`：数据源领域行菜单，仅编辑/删除；服务端未知资格失败关闭，原因常显；方向键打开后等待真实布局聚焦，Up/Down/Home/End 跳过禁用项，ESC 关闭后返回行触发器。此适配补齐 Ant Vue 4.2.6 MenuItem 仅处理 Enter 的边界，不改变动作授权。
- `OrchTaskStepRail` / `WizardFrame`：保留三类向导的原步骤与门禁，访问过步骤不表示配置已验证；完整摘要按需进入 Drawer。
- Ant `Badge` / `ConnectionTestResult`：短状态由领域 tone 映射到 Ant Badge；连接测试结果继续作为 Feature Component 使用 Ant Alert/Spin 投影证据，不以 Ant 的交互状态推断连接可用性或执行成功。

基础 Button、Input、Select、Radio、Checkbox、Switch、Dropdown、Tooltip、Form 直接使用 Ant Design Vue；不建立等价 Orch wrapper。尚未接入的设置分区仍只读，不借迁移接入新行为。

## 迁移状态

| DoD / 工作项 | 状态 | 证据或剩余条件 |
| --- | --- | --- |
| 现役入口定位 | DONE | `main.ts` → `App.vue` → `ProductShell` → router；数据源为 `workbench/sources/SourceWorkspace.vue`；DEV visual-foundation 单独识别，不作为标准 |
| 修改前基线 | DONE | 18 个 Vitest 文件 / 152 条测试通过；`npm run typecheck` 通过 |
| A 平台 Qualification | DONE | 精确依赖与 lockfile、实际受控输入/表格/弹层 Qualification、类型检查及生产构建通过；版本和独立升级规则见上文 |
| B Token 审计和唯一权威 | DONE | 四层唯一值源及生成漂移检查通过；冻结 40/47/48；终态 Legacy / Duplicate / Dead 声明均为 0 |
| C Primitive / 产品组件 | DONE | Ant Primitive + Orch 产品语义；逐项处置见下表，旧调用点与实现已移除 |
| D 业务契约保留 | DONE | Save/Test 独立、服务端筛选/游标、迟到响应隔离、权限拒绝、If-Match/409 删除负例、异步测试、秘密清理和参数互斥回归通过；真实执行发布态不外推 |
| E P0/P1 保留 | DONE | 仅同步实现替代关系；Shell、Archetype、Drawer 结构与冻结值未变，无批准的产品重设计 |
| F 四参考页和其余页面 | DONE | 四代表页、十个其余页面及 Token Normalization Review 完成；Overview 保持未接入快照空态，不新增业务能力 |
| G 无障碍与响应式 | DONE | 三档视口、原生 200% 缩放、Tab/Shift+Tab、焦点可见与返回、ESC、嵌套确认、reduced-motion、拒绝/只读/禁用/加载/空态/错误/长文本/溢出均有浏览器断言；行菜单含重复打开及可用动作导航 |
| H 工程与清理 | DONE | 类型、lint、152 条单元测试、21 条完整浏览器测试、生产构建、Token 漂移/静态审计、秘密扫描与 diff 检查通过；依赖审计 0 漏洞 |

页面迁移覆盖数据源、导出、首页、任务详情，以及任务中心、执行节点列表/登记/详情、模板、存储凭据、日志、系统设置、权限配置、普通导入和旁路导入。旧 `style.css`、`styles/enterprise-*`、`workbench/workbench.css`、Workbench Primitive、OrchButton/Menu/Dock/Dialog/Shell、旧 DataSource 页面及 visual-foundation 实验入口均已退出引用并删除；历史证据继续保存在 Git。

## 组件处置记录

### Token Normalization Review（四参考页后，2026-09-16）

四参考页已使用同一 Theme、Form、Table、Drawer、步骤轨和确认契约；Chrome 合成回归 5/5 通过，覆盖 1920×1080、1440×1024、1280×720、独立 Save/Test、脏表单嵌套确认、ESC 与摘要焦点返回。任务详情夹具包含有效冻结配置、脱敏命令和失败日志，避免以错误空态代替有效状态验收。任务详情复查发现结果区与操作区缺少工作面内边距，已归入实现修复，不修改 P0/P1。

唯一值源为 `web/src/platform/tokens.ts`，Ant Theme 与生成 CSS 同源；40/47/48 冻结值保留。新增 Orch 组件均承载产品约束，无等价 Button/Input/Select wrapper。Ant selector 例外局限于框架未开放 token 的表格几何、Form label 与确认 footer。审计发现的旧样式文件、旧 Primitive、重复布局声明仍为 Legacy/Dead 待清理，不能作为第二套基线。环境徽标专属色不得复用作通用链接或危险色；下一阶段清理误映射。其余页面迁移可以沿此基线继续，最终验收仍受全量清理和状态矩阵约束。

| 现役/历史组件 | 处置 | 原因与目标 |
| --- | --- | --- |
| OrchButton / WorkbenchButton / WorkbenchIconButton | replace | 无领域语义；调用点直接使用 Ant Button，不留下等价 wrapper |
| OrchMenu | replace | 通用下拉原语由 Ant Dropdown/Menu 负责；服务端资格、常显原因由调用处传入 |
| OrchDock / WorkbenchDrawer | refactor | 保留产品 Inspector/Editor 的标题、独立正文与固定 footer；底层用 Ant Drawer |
| OrchDialog / WorkbenchAlertDialog | refactor | 保留确定动作、影响、busy 和安全初始焦点的确认契约；底层用 Ant Modal |
| OrchField / WorkbenchFormField | delete | 字段视觉、错误、提示与 required marker 由 Ant Form.Item 直接负责；领域错误模型只提供状态与文案 |
| OrchStatus / WorkbenchStatus | delete | 短状态视觉由 Ant Badge 负责；领域层只保留 tone / label helper |
| WorkbenchTable | refactor | 使用 Ant Table，产品层负责列模型、游标分页和证据，不启用默认客户端分页/过滤 |
| WizardFrame | refactor | 保留三类任务各自步骤与门禁，复用产品 Task Step Rail，不由组件步骤状态推断有效性 |
| OrchShell / visual-foundation / 旧 DataSource 页面 | delete | 已在所有引用、测试与入口替代后删除；冻结依据留在 Git 和 P0/P1 |

## 已知规范差异与范围外事实

- **Documentation Inconsistency DS-01**：P0 §7 仍描述可用 Composite Save & Test，Data Sources P1 §6.3 不允许该动作。本次按用户指令及 P1 保留独立 Save/Test，不新增 Composite Action；冻结规范不据实现自行改写。
- 任务地图已有后端删除契约/运行二进制不一致记录；本次保留服务端资格复验和版本请求，隔离 UI 测试不证明真实删除规则已上线。
- Overview 当前只提供未接入快照的真实空态。迁移不编造数据、不增加未授权 API；测试夹具只能证明组件显示与交互，不能提升业务能力发布态。
- Export 的 CONS-05 尚属任务地图下一工作项。基础设施迁移保留当前业务步骤与参数语义，不借 UI 框架迁移实施未收口的运行时契约变更。

## 验证证据

迁移前：Windows AMD64，Node 24.16.0 / npm 11.13.0；`npm test` 18/18 文件、152/152 测试通过；`npm run typecheck` 退出码 0。

最终验收（2026-09-16）：`npm run test:browser -- --output=../artifacts/frontend-platform-acceptance` 完整 21/21 通过（4.2 分钟）；Vitest 18/18 文件、152/152 测试通过；typecheck、lint、tokens:check、audit:platform、production build、秘密扫描及 diff 检查全部通过；`npm audit` 0 漏洞。三档视口下四代表页同时断言控制台错误和页面异常为空，所有页面保持可读且无整页横滚；截图保存在该验收目录内。

收尾修复与失败证据：首开菜单在 display:none 阶段聚焦无效，改为实际布局后聚焦；Ant MenuItem 缺少方向键移动，由产品行菜单补齐并验证重复打开、Up/Down/Home/End/ESC。原生缩放用例的冷启动/多页/截图总预算调整为 180 秒；新增菜单夹具补齐必需的生命周期字段。中间一次 Chromium `ERR_NO_BUFFER_SPACE` 导致任务页空白，已保留失败 trace 并在修正隔离 WebSocket 配置后完整复验通过，不将该失败记为通过。最终验收无需重试。

迁移后的隔离浏览器入口是 `http://127.0.0.1:15174`，配置为 `web/vite.platform.config.ts`，不代理开发控制面。测试使用合成响应、合成秘密和 DEV fixture。Browser Skill 不可用，使用仓库 Playwright；普通回归使用系统 Chrome，原生 200% 缩放使用带固定测试扩展的独立 Chromium。

可复现检查：`npm run typecheck`、`npm run lint`、`npm test`、`npm run tokens:check`、`npm run audit:platform`、`npm run build`、`npm run test:browser`、`npm audit`，以及根目录 `scripts/check-secrets.ps1` 和 `git diff --check`。

静态终态审计输出到 `artifacts/frontend-platform-audit.json`：2016 条 CSS 声明，Confirmed Product Decision 3、Canonical Token 1119、Component Exception 894、Legacy / Duplicate / Dead 均 0；另列框架布局 API 与业务占宽的内联样式例外。三个 `!important` 仅在 reduced-motion 媒体规则内。该静态图不把动态类名当成完整证明，须结合浏览器状态与交互测试。

已知债务：生产主包 1052.74 kB（gzip 326.01 kB）触发 Vite 500 kB 提示；后续可独立评估路由分包，不阻断本次功能验收。DS-01 文档差异保持显式记录；无批准的产品设计偏离。未替换运行后端、未提交或推送 Git，未做真实数据库、凭据或官方工具验证，EX-V1/EX-V2 与原有业务准入状态不变。

## 数据源 Ant First 收敛（2026-09-17）

本次仅收敛数据源页及其共用状态/反馈组件；业务契约、服务端游标和 Product Shell 继续由原产品层负责。

| 替换清单 | 现役基础组件 |
| --- | --- |
| 连接状态圆点与文本 | `OrchStatus` → Ant `Badge` |
| 环境标签 | Ant `Tag`；Canonical 环境色经 `environmentTagTheme` / `ConfigProvider` 注入 Tag 的 `colorText`、`colorFillAlter` |
| 连接测试结果 | `ConnectionTestResult` → Ant `Alert`；pending/testing 使用 `Spin`，兼容旧状态命名 |
| 页面加载与操作忙态 | Ant `Skeleton` / `Spin` |
| 初始空态、筛选无结果 | `EmptyState` → Ant `Empty`，保留标题、说明与 Ant Button/Dropdown 操作 |
| 读取失败、权限拒绝、节点错误、校验汇总 | Ant `Alert`；成功的启停/删除操作使用带主题上下文的 `message.useMessage` |
| 高级设置、验证记录 | Ant `Collapse` / `CollapsePanel`；sys 校验错误先展开再定位；展开/收起图标统一使用 Ant Outlined，不进入读屏名称 |

保留例外：Product Shell/Header、工作面布局、服务端 Cursor Pagination、领域状态机与权限事实。分页动作继续使用 Ant Button，不转换为 Ant Pagination。表单标签、错误关联及 API 映射继续留在产品层。

删除的 Legacy CSS：`orch-status*`、`orch-env*`、`orch-skeleton`、`orch-table-loading*`、`orch-editor-loading*`、`orch-empty*` 旧空态、`orch-inline-feedback`、`orch-alert*`、`orch-validation-summary*`、`orch-disclosure*`、旧 advanced/result-details 规则及 `connection-test-result*` 视觉覆盖。新增 `orch-feedback` / `orch-empty-region` 只负责工作面间距。其他页面仍引用的 `.empty-state` / `.empty-mark` 未删除。

Ant component coverage：数据源基础控件使用 Button、Input/Textarea/InputPassword、Select、Checkbox、Switch、Form/FormItem、Table、Dropdown/Menu、Drawer、Modal、Badge、Tag、Alert、Skeleton、Spin、Empty、Collapse 和 message；主题由 ConfigProvider 提供。此页没有跨上下文通知生命周期，本次未为覆盖率人为增加 notification，测试结果仍在原编辑区持久展示。

Native Consolidation coverage：`OrchField`、`OrchInspectorDrawer`、`OrchStatus` 已删除。数据源编辑、任务摘要和 Qualification fixture 直接使用 Ant `Drawer`；数据源、执行节点和存储凭据表单直接使用 Ant `FormItem`；短状态直接使用 Ant `Badge`。`useAntDrawerDialog` 是行为/a11y composable，不是视觉 wrapper。剩余 `OrchOperationalTable`、`OrchDangerConfirm`、`OrchSourceActions`、`OrchTaskStepRail` 均按 Feature Component / Product Behavior 记录，不能扩展为通用 UI 库。

remaining non-Ant implementations：结构化事实使用普通 HTML；长事实的原生 title 提示保留；产品布局、分页游标、焦点适配与领域逻辑不是重复基础组件。其他页面的历史空态/提示/折叠实现不属于本次数据源页范围，不能据本次结果声称全站 100% Ant。

tests：最终完整 Playwright **23/23 通过（3.5 分钟）**，证据在 `artifacts/frontend-ant-final/`；覆盖三档视口、原生 200% 缩放、焦点/ESC、菜单、独立 Save/Test、验证记录与高级设置的键盘展开、隐藏错误定位、消息、环境颜色、权限拒绝、Skeleton/Empty/Alert 转换和游标竞争响应。Vitest **152/152**、typecheck、lint、tokens:check、production build、秘密扫描和 diff 检查通过。静态平台审计为 1800 条 CSS 声明，Legacy / Duplicate / Dead 均 0；这不替代动态覆盖证据。

验证限制：使用隔离 Playwright 与合成响应，未做真实数据库/凭据/工具验证、未提交或部署。首次缩放超时在独立复验及最终完整回归中通过；错误/权限旧 heading 断言按 Alert 语义更新；验证记录箭头的读屏名称已修正。最终 Vite 开发服务器仍报告 `ResizeObserver loop completed with undelivered notifications`，操作和焦点断言通过，但不能宣称开发日志零警告。主包仍有既有的 500 kB 体积提示。

## 产品图标体系收敛（2026-09-17）

产品显式图标统一使用 Ant Design Vue + `@ant-design/icons-vue`。直接依赖锁定为 `@ant-design/icons-vue@7.0.1`，与 Vue 3.5.40、TypeScript 6.0.3、Vite 8.1.5、Ant Design Vue 4.2.6 组合完成类型、单元、浏览器与生产构建验证；`@lucide/vue` 已从生产依赖和 lockfile 移除。Ant Design Vue 组件内部默认图标不需要二次包裹或替换。

图标风格遵循 `design-system/DESIGN.md`：操作图标使用 Outlined；导航按用户已确认的 OCP 风格优先 Filled，无对应实心图标时保留 Outlined。图标按产品语义选择，历史迁移表仅用于追溯，不覆盖现行设计规范。

尺寸只保留两档现有 Canonical Token：inline icon 使用 `--ob-component-control-font-size`（14px）；button、toolbar、navigation、status、empty mark 等显式产品图标使用 `--ob-foundation-space-4`（16px）。页面不得为单个图标写随机 `15px`、`17px`、`19px`、`21px`、`stroke-width` 或裸色；颜色继承文字、导航、Ant danger 或既有语义 token。Ant Button / Menu / Collapse 优先使用官方 icon slot 或 expandIcon 入口，不手写按钮内部 flex + svg + text 布局。

本次受影响页面与组件：Product Header、Product Shell、Inspector Drawer、数据源列表/编辑、数据源行菜单、导出向导配置树、模板中心、存储凭据、任务中心、任务详情、日志中心、系统设置、权限配置与旁路导入。自定义 SVG 盘点结果为 0；品牌文本、步骤编号、必填星号、分隔符和代码/文案箭头不是产品图标，保留文本语义。

验证证据：`npm run typecheck`、`npm run lint`、Vitest 18/18 文件 152/152 测试、原完整 Playwright 23/23、图标专项 Playwright 1/1、`npm run build`、`npm run tokens:check`、`npm run audit:platform`、`npm audit`、秘密扫描和 diff 检查通过。图标专项证据在 `artifacts/frontend-ant-icons-acceptance/`，完整浏览器回归证据在 `artifacts/frontend-ant-icons/`。验证仍使用隔离 Playwright 与合成响应，未做真实数据库、凭据或官方工具验证；既有 Vite `ResizeObserver loop` 开发警告与生产主包 500 kB 提示不由本次图标迁移解决。

## UI Design System Consolidation（2026-09-17）

新增 `design-system/DESIGN.md`、`design-system/tokens/README.md` 与 `design-system/patterns/`，将 MASTER 收敛为 Product IA / Shell / Archetype 权威。最终文档职责为：`MASTER.md` 管结构，`DESIGN.md` 管视觉与实现契约，`tokens/` 管 token taxonomy，`patterns/` 管复用 UI pattern，`pages/` 管页面特例。

实现 token 已切换到 B2B Service inspired neutral + restrained blue：`#0F172A` 强文本、`#020617` 前景、`#475569` 次级文本、`#F8FAFC` 页面底色、`#FFFFFF` surface、`#E2E8F0` 边界、`#0369A1` 交互蓝和 `#DC2626` destructive。UI 字体切到 `Noto Sans SC` + 系统 CJK fallback；技术字体切到 `JetBrains Mono` + 系统 mono fallback。控件、表格与动作密度同步为 36px control、36px table header、42px table row、32px icon action。

最小 UI 修正：移除存储凭据页中文标题负字距；执行节点详情的 code 字体改为 canonical mono token。未改变 API、状态机、字段、任务流程、信息架构、权限或真实验证状态。

新增审计证据见 `docs/03-technical/evidence/ui-design-system-consolidation-2026-09-17.md`。验证仍以合成数据和隔离浏览器为准；真实数据库、凭据和官方工具验证状态不因本轮 UI Design System Consolidation 改变。
