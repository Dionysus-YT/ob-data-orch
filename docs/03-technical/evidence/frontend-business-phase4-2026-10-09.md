# 前端业务架构治理第四阶段：全前端门禁收口

日期：2026-10-09。在任务、节点及凭据/模板实际治理基础上，将有效约束补入现有 AGENTS 和前端基线，不建立新的规范体系。第三阶段代码与验证见[阶段记录](frontend-business-phase3-2026-10-09.md)。本阶段没有业务功能、视觉或后端变化。

## 实际文件与检查职责

| 文件 | 职责 |
| --- | --- |
| `web/scripts/architecture/sources.mjs` | 读取 TS/JS/Vue，解析 Vue 两类 script、导入/再导出/动态字面量/require、类型边与值边、alias/相对/index 解析及循环 |
| `web/scripts/architecture/rules.mjs` | 明确跨层和请求边界硬约束；规模、页面职责、动态依赖和公共归属人工复核信号 |
| `web/scripts/audit-business.mjs` | 组装生产源码图、输出 FAIL/REVIEW、硬违规非零退出；不将规模变成失败阈值 |
| `web/scripts/audit-business.test.mjs` | 10 项正反测试，包含真实临时目录 CLI 的退出码验证，测试后清理精确临时目录 |
| `web/scripts/audit-wizards.mjs` | 保留向导专属检查，复用上述解析/图能力；原检查器测试不变 |
| `web/package.json` | 新增 `audit:business`，先运行检查器正反测试再审计生产代码；无新增依赖及 lockfile 变化 |
| `web/eslint.config.js` | 扩展 views/workbench/composables/platform 中绕过 API 的全局网络/流对象限制；API 层与已有认证退出边界仍由架构审计精确检查 |
| `scripts/verify.ps1`、`scripts/verify.sh` | 加入 business 审计，保留原检查顺序、失败传播及现有门禁 |
| `.github/workflows/ci.yml` | Web quality 增加同一业务架构检查，不削弱现有 lint/类型/测试/构建 |
| `AGENTS.md`、前端平台基线、文档中心、任务地图 | 更新维护入口、已实施检查、人工复核要求与当前验证状态；保留未开发模块边界 |

检查器拆成源码分析、规则与入口三项真实职责；无巨型检查脚本、机械目录数量或新通用业务引擎。

## 硬约束与人工复核

硬约束覆盖所有生产模块：workbench 不依赖 views/router；公共 components/composables 不依赖具体业务、页面或路由；platform 不依赖业务/API/UI 装配；API 不依赖业务/UI/平台实现；步骤组件不创建 API 客户端。类型边也不能绕过禁止层关系，仅在合法方向允许纯类型端口。全图循环值依赖失败，纯类型循环不误判。网络客户端、直接全局 HTTP/实时流、常见全局属性/别名/解构不得绕过 API；ProductHeader 的既有 `fetch('/logout')` 是准确文件和端点例外，没有整个模块豁免。

REVIEW 覆盖公共能力单业务归属、复杂页面多事务/生命周期、非字面量动态依赖、>400 总行或 >300 脚本行。单独 REVIEW 的真实 CLI 退出 0；人工解释职责、唯一状态及行为证据，不能按行数机械拆分。反射调用、动态路径、重复状态、规则语义和权限正确性仍需源码复核及行为测试，静态通过不证明全部正确。简单页面保留单文件，普通/旁路导入等未开发模块只受规范约束。

## 当前五个规模信号的复核

| 文件 | 当前总行 | 决定及职责理由 |
| --- | --- | --- |
| `api/browser.ts` | 1977 | 按用户明确范围只审计，承载类型、白名单投影及现有 HTTP/安全边界，未整体拆分或绕过 API；不是页面状态所有者 |
| `exportDraftInput.ts` | 514 | 单一草稿 DTO 转换/纯校验能力，无页面生命周期或第二个表单所有者，保留现有规则测试 |
| `useExportCatalog.ts` | 500 | 同一目录能力的加载、缓存、失效与响应隔离；与表单/草稿事务分开，保持已有完整目录及竞态测试 |
| `useExportDraftLifecycle.ts` | 467 | 草稿恢复/保存、预检查与提交有明确生命周期和唯一结果所有者，消费既有表单而非同步副本；现有失效/提交行为测试保持 |
| `useExportParameters.ts` | 463 | 同一参数能力的适用条件、显隐、归一化与跨步清理，消费唯一表单；没有重复 DTO/页面副本 |

此表是本次源码复核结果，不是永久豁免或未来追加职责许可。后续新增独立能力仍须按入口、组件、规则、异步所有者判断归属；维护索引以业务能力指向现有实现。

## 实际验证与限制

`npm run audit:business`：10 项正反测试通过，生产源码 0 violations、5 REVIEW；涵盖跨层正反、alias/相对/barrel/index、Vue 两类 script、混合/纯类型、跨模块循环、网络别名/局部同名、精确 logout、间接组件消费者、规模及真实 CLI。初轮 globalThis 别名负例暴露内置 symbol 无声明的识别缺口，修正后通过。

`npm run audit:wizards`：原 3 项检查器测试通过，0 violations、4 个已有 export REVIEW。全量 Vitest 255 项、ESLint、TypeScript、构建通过；Chrome 19 项主回归与 4 项导出组件补验通过。秘密扫描与差异检查通过。CI 配置已接入；本地验证不能被描述为远端新 CI 已完成，推送后的实际结果单独核验。

既有 Token/平台审计失败、Go 基线 CI 失败、ResizeObserver 提示、合成与真实验证边界同第三阶段记录。未执行整个 verify、全量 Playwright 或真实业务操作，不把 REVIEW、未执行检查、目标交叉构建或合成结果冒充真实能力证明。

## 最终缺陷收口（b02c201 基线）

日期：2026-10-09。本节是第三、四阶段验收之后的定向修复，前文数字保留历史事实。基线完整 SHA 为 `b02c201abf50960b3c289f65fc016a14bee4aae4`。不改变架构、视觉、模板派生流程、其他模块或后端；原未跟踪 `ob-data/` 不进入提交。

### 实际变更文件

| 文件 | 结果 |
| --- | --- |
| `web/src/workbench/credentials/useCredentialDeletion.ts` | 监听列表授权失效，同步取消目标与确认代；拒绝授权阻断期间重新打开删除确认 |
| `web/src/workbench/credentials/credentialLifecycle.test.ts` | 新增 3 项：401/403 清目标、503 不能解除阻断、成功重读不恢复旧确认、普通故障保留会话、过期拒绝不撤销最新授权 |
| `web/src/workbench/templates/useTemplateCatalog.ts` | 目录拥有持续授权阻断；401/403 清列表并阻断共享写入口，仅成功重读解除 |
| `web/src/workbench/templates/useTemplateActions.ts` | 同步清改名标识、输入及待删除目标；拒绝旧授权对象重开操作 |
| `web/src/workbench/templates/templateLifecycle.test.ts` | 新增 3 项，覆盖两类授权失败、临时故障、过期拒绝、恢复不复活及改名/删除/派生写入口阻断 |
| `web/scripts/architecture/rules.mjs`、`web/scripts/audit-business.mjs` | 跨业务依赖输出 REVIEW；值/类型、alias/相对、再导出、动态引用沿用既有解析，不增加硬性方向禁令 |
| `web/scripts/audit-business.test.mjs` | 新增跨业务正例矩阵；真实 CLI 确认 REVIEW 退出 0，现有循环及跨层硬门禁保持 |
| `web/tests/business-lifecycle.spec.ts` | 新增 4 项 Chrome 负例，验证凭据及模板在 401/403 后关闭确认框、清输入、成功重读不恢复旧会话，不发出删除/改名请求 |
| 两模块 `README.md`、前端平台基线、凭据安全契约、本记录、任务地图 | 同步维护入口、授权失效边界、跨业务审计及当前验收证据 |

### REVIEW 决定

业务审计为 0 硬违规、6 个 REVIEW：原 5 个规模信号沿用本记录上方职责复核。新增 `templates/useTemplateDraft.ts → export/exportDataSourceEligibility.ts`：模板派生和导出使用同一数据源资格，导出纯函数为唯一规则所有者，不持有 Vue 状态、不执行 HTTP、不反向依赖模板；保持现有合理复用，不移动或复制规则。模板引用缓存与绑定仍由 `useTemplateDraft` 持有。未来跨业务新增边均会提示 REVIEW，本次无依赖豁免清单。

CodeGraph 缓存最后修改于 2026-09-16，早于本次业务拆分，未用其判断当前调用关系，也未重建索引；依据现役源码、AST 图与负例测试核验。

### 独立待办（本次未修复）

- **产品体验 TPL-UX-01：模板派生两阶段交互。** 当前按钮同时调用 `loadChoices(); createDraft(template)`，首次点击加载引用时先提示选择源与节点，选择后需要再次点击创建。单独作为产品体验优化评审；本次保留既有流程，不将其列为架构缺陷已解决。维护入口为 `TemplateCenterView.vue` 与 `useTemplateDraft.ts`；后续验收应覆盖首次进入、引用加载失败/重试、绑定选择和防重。

用户补充的四项导出历史业务审查意见单独核实登记，排除在本次提交之外；本次没有修改导出业务实现，不将历史静态意见直接判为已确认缺陷，也不宣称已解决。

### 本次检查

| 本次检查 | 实际结果 |
| --- | --- |
| 针对性生命周期单元测试 | 凭据/模板 25 项通过，新增 6 项 |
| 全量前端单元测试 | Vitest 24 文件、261 项通过 |
| 类型与 Lint | `npm run typecheck`、`npm run lint` 通过；401 测试按既有跳转修正后单独 ESLint 复验通过 |
| 业务架构审计 | 11 项检查器正反测试通过；0 硬违规、6 REVIEW，决定见上 |
| 向导架构审计 | 原 3 项检查器测试通过；0 硬违规、4 既有规模 REVIEW |
| 生产构建 | `npm run build` 通过，保留既有 >500 kB 分块提示 |
| 全量 Chrome | 75 项全部执行，74 通过、1 失败；本次新增 4 项授权负例和原凭据/模板 4 项回归全部通过 |
| 秘密与差异 | `scripts/check-secrets.ps1`、`git diff --check` 通过 |

完整浏览器失败为 `web/tests/reference-pages.spec.ts:415` 的旧控制面数据库目录兼容性断言：查找 `手动输入其他数据库 / Schema…`，当前失败态显示 `__unavailable__` 选项及独立的 `手动输入数据库` 按钮，原文字不存在。该测试与导出实现相对 `b02c201` 均无差异，本次不修改其断言或导出业务；未另行运行原基线，不把这次完整套件写成全部通过。数据源高级设置出现既有 `ResizeObserver loop completed with undelivered notifications` Vite 告警，对应用例通过，不声明浏览器日志零告警。

Browser plugin not available，采用仓库 Playwright 与隔离 Vite `127.0.0.1:15174`、系统 Chrome；新增验证路径为刷新在途已有确认/输入 → 合成 401/403 → 关闭确认与清输入 → 授权恢复仍无旧会话。401 沿用全局 `/login` 跳转并验证重新进入；403 验证同一页面成功重读。首轮因 SOCKS 代理环境不兼容而未启动，重试仅在测试子进程清空代理。首个 401 用例误设为同页刷新而超时，按既有认证行为修正测试后重跑，未修改 API 或登录规则。最终完整结果与 trace 写入本机 Temp `ob-frontend-closure-final-20261009`，初轮失败保留于 `ob-frontend-closure-20261009`；不提交测试产物。

没有替换服务二进制或执行真实数据库、存储凭据、Agent、官方工具验证；不提升任何真实准入或上线状态。未执行全仓 verify 或远端 CI，既有 Token/平台/Go 阻断不由本次局部通过替代。
