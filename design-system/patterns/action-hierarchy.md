# Action Hierarchy Pattern

一个操作上下文通常只有一个 Primary Action。这里的上下文可以是页面工具栏、表格行、Drawer footer、确认弹层或向导步骤，不等于全页面只能有一个 Primary。

| 层级 | 用途 | 呈现 |
| --- | --- | --- |
| Primary | 当前上下文最主要的业务动作 | Ant `Button type="primary"` |
| Default / Secondary | 保存、刷新、上一步、非破坏性辅助动作 | Ant default Button |
| Text / Tertiary | 取消、清除、返回、低权重入口 | Ant text/link Button |
| Danger | 删除、禁用、高风险不可逆动作 | Ant danger 或 `OrchDangerConfirm` |
| Icon Action | 空间受限且语义清晰的工具动作 | Ant Button icon slot + accessible name |

规则：

- `刷新` 是 Utility Action，不是 Primary Business Action。
- Save 与 Test 保持独立语义；除非 P3 明确授权 Composite Action，不合并成 `保存并测试`。
- Danger 动作先展示对象、影响、原因和确定动作，再执行。
- Disabled 必须有可读原因；不能只靠灰色或 hover-only tooltip。
- Loading 必须阻止重复提交或清楚说明可并发。
- 行级高频安全动作可直接显示，低频动作进入 More Menu。

## 未保存输入与退出登录

站内导航沿用页面已有的未保存/忙碌守卫；刷新、关闭标签页等整页卸载使用浏览器原生保护。退出登录先调用当前编辑会话的同一守卫，取消不发送注销请求，也不清除输入。确认后才提交既有注销请求；服务端确认成功或会话已失效后，直接进入登录页，避免再次弹出未保存提示。失败或结果无法确认时保留输入与原生卸载保护，用 Ant Alert 和统一确认弹层提供重试或留在当前页。成功退出不表示草稿自动保存。
