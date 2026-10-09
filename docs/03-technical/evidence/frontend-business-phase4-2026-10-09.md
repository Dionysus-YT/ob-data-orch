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
