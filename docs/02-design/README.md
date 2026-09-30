# 产品与模块设计

本目录定义功能、字段、状态、权限、错误恢复和验收场景，属于 P3 业务规则。页面外观遵循 [P0 MASTER](../../design-system/MASTER.md) 与对应 P1；所有前端任务同时读取 [数据源标准页](../../design-system/pages/data-sources.md) 的复用规则。

## 按模块阅读

模块方案、字段矩阵和业务场景已合为同一文档。先读方案，做实现或验收时再跳到附录；原字段 ID 和规则编号保留。

| 模块 | 设计入口 | 按需查阅 |
|---|---|---|
| 首页 | [首页设计](home-module.md) | [字段](home-module.md#field-rules) · [场景](home-module.md#business-scenarios) |
| 数据源 | [数据源管理](data-source-management.md) | [标准页](../../design-system/pages/data-sources.md) · [合成验证夹具](ui-regression-fixture-data-source.md) |
| 导出 | [导出设计](export-module.md) | [技术契约](../03-technical/export-general-contract.md) · [受控验证](export-controlled-validation.md) |
| 普通导入 | [普通导入设计](normal-import-module.md) | [字段](normal-import-module.md#field-rules) · [场景](normal-import-module.md#business-scenarios) · [参数映射](normal-import-parameter-mapping.md) · [验证计划](normal-import-controlled-validation.md) |
| 旁路导入 | [旁路导入设计](direct-load-module.md) | [字段](direct-load-module.md#field-rules) · [场景](direct-load-module.md#business-scenarios) · [参数映射](direct-load-parameter-mapping.md) · [验证计划](direct-load-controlled-validation.md) |
| 任务与详情 | [任务中心设计](task-center-module.md) | [字段](task-center-module.md#field-rules) · [场景](task-center-module.md#business-scenarios) |
| 执行节点 | [执行节点设计](execution-node-module.md) | [字段](execution-node-module.md#field-rules) · [场景](execution-node-module.md#business-scenarios) |
| 日志中心 | [日志中心设计](log-center-module.md) | [字段](log-center-module.md#field-rules) · [场景](log-center-module.md#business-scenarios) |
| 模板中心 | [模板中心设计](template-center-module.md) | [字段](template-center-module.md#field-rules) · [场景](template-center-module.md#business-scenarios) |
| 系统设置 | [系统设置设计](system-settings-module.md) | [字段](system-settings-module.md#field-rules) · [场景](system-settings-module.md#business-scenarios) |
| 权限与安全 | [权限与安全设计](access-control-security-module.md) | [字段](access-control-security-module.md#field-rules) · [场景](access-control-security-module.md#business-scenarios) |

## 跨模块规则

- [信息架构](information-architecture.md)：导航、页面职责和模块关系。
- [用户流程](user-flow.md)：三类任务创建、追踪与失败处理。
- [通用交互](interaction-spec.md)：校验、命令预览、任务和日志的共用语义。
- [关键决策](../01-product/decisions.md)：已确认取舍及替代关系；产品设计语义收口见 DEC-034。

附录中的原“低保真”内容只保留业务场景语义，不授权恢复旧截图、布局、尺寸或视觉规则。产品规则已确认不等于对应功能已经实现，当前阶段查 [任务地图](../03-technical/development-task-map.md)。
