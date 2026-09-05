# 02-design 文档使用边界

本目录中的模块评审稿、字段规则、参数映射、受控验证与场景矩阵属于 **P3 业务事实**：它们说明页面应支持的能力、字段、状态、权限、验证、错误恢复和脱敏边界。

它们不是全局视觉规范，不定义 Product Shell、Token、页面 Skeleton、Sidebar、Header、Drawer 外观、Table 密度或响应式布局。所有前端视觉与交互架构决策必须先遵循 [P0 MASTER](../../design-system/MASTER.md)，页面进入精修 READY 后才可读取对应的 P1 page rule。

名称中保留 `low-fidelity` 的文档仅用于追溯已确认的业务场景；其中任何截图、布局或视口说明均不再构成当前实现依据。
