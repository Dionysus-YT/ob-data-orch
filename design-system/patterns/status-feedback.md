# Status and Feedback Pattern

状态必须表达 domain，不得把生命周期、连接测试、环境、兼容模式和任务阶段混成一个 badge。

| Domain | Component |
| --- | --- |
| Short operational state | `Badge` |
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
