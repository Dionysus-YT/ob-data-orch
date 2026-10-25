# 工具与兼容维护入口

- `useToolCompatibility.ts` 唯一持有只读事实、加载/错误与请求会话；刷新撤下旧事实，路由变化与卸载立即失效，A→B→A 用会话代而非仅比较路由或 ID。
- `api/toolCompatibility.ts` 负责固定 GET 和严格投影，区分工具、三个任务模块及历史重放目录；不把支持目标解释为节点安装事实，不推导全平台活动元数据。
- `views/ToolCompatibilityView.vue` 只装配路由、读取动作和既有 Settings/Ant 展示；不持有表单、发布操作或节点事实副本。
- 工具目标及目录来自后端实际内置参数资源。导出按配置选择，v6/v8 只用于历史重放；普通导入与旁路各用自己的固定目录。不得因本页重排或选择目录而改变命令、草稿、模板或历史快照。
- 规则见[工具与兼容只读契约](../../../../docs/03-technical/tool-compatibility-contract.md)，视觉见[系统管理配置域 P1](../../../../design-system/pages/system-settings.md)。
