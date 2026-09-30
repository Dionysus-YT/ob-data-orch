# Export Builder Page Rule

> 状态：**PAGE SPEC NOT YET READY**
>
> 本文件尚无 P1 覆盖规则。导出 Builder 在进入单页 High-Fidelity READY 前，完全遵循 [P0 MASTER](../MASTER.md)、[DESIGN](../DESIGN.md)、[Task Builder Pattern](../patterns/task-builder.md) 与 P3 Export Canonical、参数映射、命令及预检查契约。

按 DEC-049，导出步骤 schema 为五步，内容与对象位于同一工作页；Task Builder Framework 是 P0。当前不能在此处把某一步的内容布局固化为全部步骤的通用骨架。

2026-09-30 用户确认导出进度栏置于当前工作区顶部，使用 Ant `Steps` 的水平布局；窄屏允许横向滚动。数据源、数据库和导出对象仍为当前步骤的业务内容，不放入进度栏。
