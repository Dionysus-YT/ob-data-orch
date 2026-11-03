# 模板维护索引

2026-11-02 模板中心已按[模块重设计](../../../../docs/02-design/template-center-module.md)与[页面 P1](../../../../design-system/pages/template-center.md)接入本地代码：完整授权摘要内的筛选/分页、使用抽屉、改名弹窗与操作菜单；availableActions 分离使用与管理，缺失时只读。创建与保存保留当前会话原键，30 秒等待超时进入未知；已创建的草稿只重试导航，不再次创建。阶段与验证边界只查[任务地图](../../../../docs/03-technical/development-task-map.md#template-center-redesign)及[本轮证据](../../../../docs/03-technical/evidence/template-center-2026-11-02.md)，不据本地代码推断已安装或真实业务通过。

2026-10-19 模板中心消费 config-templates 的显式 taskType；既有导出 URL 保持导出边界。useTemplateDraft 独占选定模板、绑定与候选会话，先选择模板再创建；两类任务分别复用现有纯资格规则，普通导入读取自身授权节点列表。类型、修订、授权、绑定或卸载使迟到候选/写反馈/导航失效。改名和删除继续由 useTemplateActions 管理，不在页面持有第二份事实。

规范沿用 [AGENTS](../../../../AGENTS.md) 与[前端基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)。模板仅复用显式非敏感配置，派生草稿仍需重新选择绑定并完成预检查。

| 修改目标 | 唯一入口 |
| --- | --- |
| 页面、列、Ant 装配、路由导航 | `views/TemplateCenterView.vue`；业务弹层为 `TemplateDraftDrawer.vue`、`TemplateRenameDialog.vue`，只消费原所有者与意图 |
| 列表事实、本地浏览、读取失效和共享操作锁 | `useTemplateCatalog.ts`；401/403 阻断操作，后续临时故障不恢复权限，仅成功授权重读解除阻断 |
| 改名/删除及取消会话 | `useTemplateActions.ts`；列表授权失效同步清改名标识、输入与待删除目标；恢复授权不恢复旧会话 |
| 引用加载/缓存与派生草稿 | `useTemplateDraft.ts`；节点下拉展开强制重读，按 ID 保留或清除选择；两类引用原子发布、单一在途 Promise，失败清引用，读取中不创建草稿 |
| 数据源资格 | 直接复用 `workbench/export/exportDataSourceEligibility.ts`，不重复实现 |
| 名称与未决结果规则 | `templateRules.ts`；任务保存真实复用相同字节与未知结果规则 |
| 有界写入等待 | `templateWrites.ts`；保存与创建共用，不持有业务输入或自动重试，超时/卸载清理计时器 |
| HTTP、修订、安全投影与请求键 | `api/browser.ts`；现役保存/创建方法显式接收会话原键，动作只读投影不代替授权 |

目录不是通用模板引擎。每类事实和编辑状态只创建一次，页面消费原 ref；读写成功、失败、finally 和导航均核验页面会话，卸载取消 HTTP。方法共享操作锁，写入使先前列表读取失效；编辑及删除取消有独立代号。`templateLifecycle.test.ts` 与 `tests/business-lifecycle.spec.ts` 覆盖关键负例；详见[第三阶段证据](../../../../docs/03-technical/evidence/frontend-business-phase3-2026-10-09.md)。

2026-10-27 检查整改后，列表标题、刷新、错误与表格复用管理列表的单一工作面；空态只说明从成功任务保存模板，所需绑定由创建区的现有控件表达。改名、删除、候选读取和草稿派生的所有者及失效边界不变。
