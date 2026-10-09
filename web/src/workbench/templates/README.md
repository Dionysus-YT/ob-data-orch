# 模板维护索引

规范沿用 [AGENTS](../../../../AGENTS.md) 与[前端基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)。模板仅复用显式非敏感配置，派生草稿仍需重新选择绑定并完成预检查。

| 修改目标 | 唯一入口 |
| --- | --- |
| 页面、列、Ant 装配、路由导航 | `views/TemplateCenterView.vue` |
| 列表事实、读取失效和共享操作锁 | `useTemplateCatalog.ts` |
| 改名/删除及取消会话 | `useTemplateActions.ts` |
| 引用加载/缓存与派生草稿 | `useTemplateDraft.ts`；两类引用原子发布、单一在途 Promise，失败不缓存半份结果 |
| 数据源资格 | 直接复用 `workbench/export/exportDataSourceEligibility.ts`，不重复实现 |
| HTTP、修订、安全响应 | `api/browser.ts` 原方法，不绕过边界 |

目录不是通用模板引擎。每类事实和编辑状态只创建一次，页面消费原 ref；读写成功、失败、finally 和导航均核验页面会话，卸载取消 HTTP。方法共享操作锁，写入使先前列表读取失效；编辑及删除取消有独立代号。`templateLifecycle.test.ts` 与 `tests/business-lifecycle.spec.ts` 覆盖关键负例；详见[第三阶段证据](../../../../docs/03-technical/evidence/frontend-business-phase3-2026-10-09.md)。
