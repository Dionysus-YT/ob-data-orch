# Overview Page Rule

> 状态：**PAGE SPEC NOT YET READY**
>
> 本文件尚无 P1 覆盖规则。首页在进入单页 High-Fidelity READY 前，完全遵循 [P0 MASTER](../MASTER.md)、[DESIGN](../DESIGN.md)、相关 pattern 与 P3 首页业务规则。

当前已冻结的是 Representative Visual Baseline，不代表本页已经完成单页精修或可在此处新增 token、布局例外、KPI 数量或操作模型。

2026-10-23 首页只读实现入口为 `web/src/workbench/overview/`，路由页只装配。工程按现有 Overview Archetype 采用共享摘要面、四个精确指标、唯一趋势、节点和有限任务/异常列表；没有新 token、Shell 或 Archetype 例外。当前实现与验收属于 P4，不因代码存在或合成浏览器通过自动将本页提升为 P1 READY；业务与时间/权限边界见 [API 契约 §5.1.1](../../docs/03-technical/api-sqlite-data-contract.md#511-首页只读运行快照2026-10-23)。

## 2026-10-23 关注优先结构（DEC-060）

上段“共享摘要面、四个精确指标”的描述由本节取代。按 [DEC-060](../../docs/01-product/decisions.md#dec-060-首页关注优先结构) 与 P0 §5/§6，本页结构为：

| 顺序 | 分区 | 收窄规则 |
| --- | --- | --- |
| 0 | 页头 | 共享 `orch-page-header`；右侧依次为授权范围、UTC、快照时间、过期/更新中 Badge、自动刷新说明，以及文本刷新按钮（刷新 / 刷新中 / 已刷新） |
| 1 | 状态带 | 一个通栏工作面五格：四项任务指标 + “不可接收节点 N / 可见 M”。数字为 Ant 链接按钮；未知、未授权、受限各显示对应文字，不以 0 代替 |
| 2 | 需要关注 | 标题行右侧为受控摘要文本；列表每行：Badge 类型、对象链接、来源与时间。无事实时用 Empty 并保留覆盖范围说明 |
| 3 | 我的任务 / 执行节点 | 宽容器 3:2 并排，两者均为 Ant Table。节点列：名称、接收结论、状态（四维）、槽位、心跳与资源；原因以文本列出 |
| 4 | 任务结果趋势 | Radio 按钮组切换范围；SVG 仅绘制（`role="img"`，降低高度）；下钻由 Collapse 内明细 Table + Button 承担 |

工作区容器不足 1000px 时第 3 行单列，顺序不变。分区沿用平面 `content-card`，不新增阴影、token、Wrapper 或组件例外。实现与验收仍属 P4，本页状态保持 PAGE SPEC NOT YET READY，证据见[首页验收记录](../../docs/03-technical/evidence/home-overview-2026-10-23.md#2026-10-23-关注优先结构重构dec-060)。
