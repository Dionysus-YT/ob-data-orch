# OB Data Orch 文档中心

OB Data Orch 是 OB Loader/Dumper 4.3.5 的轻量可视化编排平台。按当前问题选入口，无需通读全部文档。

## 从哪里开始

| 想了解什么 | 先读 | 按需继续 |
|---|---|---|
| 日常改代码与热更新 | [源码开发模式](03-technical/deployment-operations.md#源码开发模式) | `scripts/dev.ps1` 与开发 Agent，稳定后才打包 |
| 产品做什么、不做什么 | [产品范围](01-product/product-scope.md) | [关键决策](01-product/decisions.md)解释已确认的取舍 |
| 当前做到哪、下一步是什么 | [开发任务地图](03-technical/development-task-map.md) | [准入与真实验证门禁](03-technical/development-readiness-closure.md) |
| 某个页面的功能、字段和异常场景 | [模块设计索引](02-design/README.md) | 每个模块一份设计；导入参数映射和验证计划按需读取 |
| 页面应该长什么样 | [P0 MASTER](../design-system/MASTER.md) | [数据源标准页](../design-system/pages/data-sources.md)及对应页面 P1 |
| 前端基础控件、Token、样式与升级 | [前端平台基线](03-technical/frontend-platform-baseline.md) | Qualification、浏览器回归和静态审计入口 |
| 复杂向导如何组织与维护 | [统一向导架构](03-technical/frontend-platform-baseline.md#复杂向导架构与开发规范) | [导出维护索引](../web/src/workbench/export/README.md)、强制约束/人工复核/自动检查；导入仍按任务地图准入 |
| 前端业务模块职责、状态与异步治理 | [前端业务架构](03-technical/frontend-platform-baseline.md#前端业务模块架构) | [任务维护索引](../web/src/workbench/tasks/README.md)、[第一阶段验收](03-technical/evidence/frontend-business-phase1-2026-10-09.md)、[节点维护索引](../web/src/workbench/nodes/README.md)、[第二阶段验收](03-technical/evidence/frontend-business-phase2-2026-10-09.md)；其余模块按阶段确认 |
| 如何安装、使用网页、启停和升级 | [安装、使用与运维手册](03-technical/deployment-operations.md) | 首次安装、Agent 注册、真实执行条件、旧部署迁移和故障定位；快速开始见 [README](../README.md) |
| 系统如何实现、接口有哪些边界 | [技术契约索引](03-technical/README.md) | 只读受影响的契约 |
| 某项能力有什么验证证据 | [验证记录索引](03-technical/README.md#evidence) | 区分合成验证、Windows 实测和正式发布 |

首次了解项目，读“产品范围 → 任务地图 → 当前相关模块”即可。字段矩阵供实现和验收时查阅，不是入门必读。

## 如何判断文档结论

- **当前阶段与顺序**只由任务地图维护，其他入口不重复列状态表。
- **业务规则**由模块设计和技术契约承载；已确认变更的原因、日期和替代关系保留在关键决策中。
- **视觉规则**以 P0/P1 为准。2026-09-14 定版的数据源页是后续页面的视觉与组件标准，见 [DEC-047](01-product/decisions.md#dec-047-数据源页面定版与后续页面标准)。
- **验证记录**只证明记录日期、环境与范围内的结果，不能代替当前运行检查、跨平台认证或发布门禁。

<a id="history"></a>

## 历史资料

原始 PRD、Export 早期研究与跳转页、重复开发准入清单、旧实现盘点和旧视觉截图已从工作树删除；现役规则由上面的入口承接。模块字段与场景、分散测试记录已合并，不再维护旧路径副本。

需要追溯删除前原文时，在仓库根目录使用 `git show 94adccdd421f54ad22fba0b087448c7ad6379880:<原仓库相对路径>`。该提交用于查历史，不代表当前功能状态。冻结参数元数据里的历史 `sourceDocuments` 原样保留，解释见 [兼容契约](03-technical/export-general-contract.md#25-ex-d0-冻结边界的承接)。

有独立验证价值的实施记录保留在 [工程历史](03-technical/evidence/engineering-history.md)；默认阅读路径不包含它。后续维护直接更新对应入口和正文，不再新增重复进度表或仅作跳转的文档。
