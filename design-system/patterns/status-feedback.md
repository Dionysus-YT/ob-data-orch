# Status and Feedback Pattern

状态必须表达 domain，不得把生命周期、连接测试、环境、兼容模式和任务阶段混成一个 badge。

| Domain | Component |
| --- | --- |
| Short operational state | `Badge` |
| Explicit operation result | `Message` |
| Category / attribute | `Tag` |
| Problem explanation | `Alert` |
| Execution progress | `Progress` |
| Empty / no result | `Empty` 或产品空态 composition |
| Async pending | `Spin` / loading state |
| Destructive confirmation | `OrchDangerConfirm` |

规则：

- 状态不能只靠颜色，至少有 text、icon 或 label 辅助表达。
- 绿色成功只表达该 domain 的成功，不自动表示任务资格或可执行。
- Warning 可继续与否由 P3 契约决定，不由颜色推断。
- Failure 要说明对象、原因、影响、恢复动作和追踪标识。
- Loading 保留空间和可信旧数据，不用假数据伪装完成。
- Unknown、Expired、Permission restricted、Empty 和 Backend error 是不同状态。

## 全平台反馈规则

Design System Change · 2026-10-12：按用户确认统一执行节点、数据源、任务、模板、存储凭据和导出向导的提示。以下规则属于 P0，各页面沿用同一 Ant 组件与主题，不自行绘制成功条或错误框；此次只调整反馈语义与生命周期，不新增 Token、改变 Shell 或业务准入。

| 情境 | 展示与保留规则 |
| --- | --- |
| 用户主动操作已成功，例如保存、删除、启用、复制 | Ant `Message`，`success`，3 秒自动关闭；文案只确认已完成的动作。 |
| 主动操作没有更改或无需处理 | Ant `Message`，`info`；不得声称发生了保存或修改。 |
| 持续说明、待完成条件、风险、请求失败 | Ant `Alert` + `show-icon`；分别使用 `info`、`warning`、`error`，放在对应工作面或操作区。恢复前不自动消失；失败有安全、可行动的解释及适用的重试入口。 |
| 字段无效 | Ant `Form.Item` 的校验状态与帮助文本，关联实际输入；复杂表单可补充链接字段的 `Alert` 汇总。首次打开不报错。 |
| 连接测试、预检查、任务终态等领域事实 | 保留在原结果区，以 Badge、Alert 或证据结构表达；不能用短时成功提示代替结果或外推任务可执行。 |
| 行内开关、手动刷新 | 保留已有 Switch / Button 的 loading 与返回状态；不额外弹出重复成功提示。自动刷新和轮询不弹成功提示。 |

- 同一次操作只确认一次，不同时显示 Message 和成功横幅。常驻边界说明写在对应业务区域，不能塞进每次成功文案。
- 使用 `message.useMessage()`，在当前 `ConfigProvider` 下渲染上下文，继承原生配色、图标、间距与动效。每个提示上下文最多显示一条，新的结果替换旧结果；不添加全局状态、事件总线或基础控件包装。
- 成功提示采用非打断式 `role="status"` / `aria-live="polite"`，错误使用 Alert 的警示语义。布局只复用现有间距，不添加新 token 或重绘 `.ant-*` 状态。
- 操作结果由原业务会话所有者发布。重试前清除上一条，关闭、切换对象、授权失效或卸载时销毁该会话提示；迟到、重复或过期响应不得恢复旧提示。相同文案的再次成功也必须可感知。
- 提示只包含安全的对象名称、动作和修订等允许展示的事实，不包含秘密、一次性注册材料、原始诊断、未脱敏命令或 SQL 内容。
- 页面重新读取或重新挂载不重放历史成功。导航已进入新工作面时，用目标页事实与上下文承接结果，不建立跨页面成功通知队列。
