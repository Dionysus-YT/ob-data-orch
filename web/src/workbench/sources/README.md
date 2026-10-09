# 数据源维护索引

产品以[数据源 P1](../../../../design-system/pages/data-sources.md)、对应 P3 与任务地图为准；工程以 [AGENTS](../../../../AGENTS.md)、[前端基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)为准。本轮保留既有数据源工作台和状态所有权，只迁入原 views 的业务工具及专属反馈组件，修复反向依赖。

| 修改目标 | 实际入口 |
| --- | --- |
| 列表、服务端筛选/游标、行操作与页面反馈 | `SourceWorkspace.vue`、`sourcePresentation.ts`、`dataSourceListFilters.ts` |
| 编辑抽屉、唯一表单与敏感输入/异步测试生命周期 | `SourceEditor.vue`、`useSourceEditor.ts` |
| 字段校验、API 字段错误白名单 | `dataSourceFormErrors.ts` |
| 连接串解析 | `dataSourceConnectionString.ts`；保留原短时输入与清理行为 |
| 连接测试诊断与提示 | `dataSourceConnectionTestDiagnostic.ts`、`dataSourceConnectionTestNotice.ts`、`ConnectionTestResult.vue` |
| 行菜单 | `OrchSourceActions.vue`，业务专属而非通用 UI Wrapper |
| API 与 DEV 合成依赖 | `sourceGateway.ts`；普通业务经 `api/browser.ts`；`dataSourceUiFixture.ts` 仅依原 DEV 条件启用，不提升真实能力 |

六组迁入规则/夹具的原测试断言保持不变，只更新导入路径。验证沿用本模块 Vitest 和 `tests/source-contracts.spec.ts`、数据源参考页回归。不得把连接测试成功等同可启用或任务可执行，不改服务端资格、秘密输入、游标、授权和 Save/Test 分离语义。
