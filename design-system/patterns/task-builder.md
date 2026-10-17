# Task Builder Pattern

Export、Normal Import、Direct Load 共享 Task Builder Interaction Language，但各自的步骤 schema、资格、风险、命令语义和提交门禁独立。

2026-10-15 经用户确认，[DEC-051](../../docs/01-product/decisions.md#dec-051-普通导入五步向导与旁路模块解耦) 将普通导入改为“选择数据源 → 导入文件与格式 → 对象与映射 → 执行参数 → 预检查与命令”五步；随后 [DEC-052](../../docs/01-product/decisions.md#dec-052-普通导入沿用导出布局与三种文件格式) 明确普通导入沿用导出的顶部水平步骤栏、全宽工作面、摘要 Drawer 与固定底栏，文件类型仅配置 CSV、CUT、SQL。内容使用紧凑按钮单选，格式使用下拉；常用字段按导出的分组与表单几何呈现，步骤内只保留一个高级设置区。普通导入进度只表达本地输入完整度，预检查未接入时最后一步保持 0%。旁路保持独立六步，不共享导入业务状态、参数或规则。Product Shell、Archetype 和视觉 Token 不变。

[DEC-053](../../docs/01-product/decisions.md#dec-053-普通导入参数分层与适用范围收敛) 继续按导出标准组织普通导入参数：字段与选项的说明使用 Ant Tooltip，参数名和含义分行展示；提示按钮与 Checkbox/Radio 并列，点击或键盘查看不能改变业务选项。实际 CLI 参数才提供提示，平台选择与本地确认不伪装成 CLI 开关。条件不适用的字段不进入活动摘要，受管连接值不在任务级重复编辑；第四步性能和资源进入唯一高级区。

## 结构

```text
Task Header
  saved / dirty state, Save Draft, current step context
Task Step Rail
  导出与普通导入使用顶部水平进度栏；旁路在 1920 / 1440 使用紧凑竖向进度栏
Current-step Workspace
  chosen by step content
Optional Inspector / Summary Drawer
Fixed Footer
  previous + current step primary action
```

## Step State

| State | 含义 |
| --- | --- |
| Complete | 已满足且当前有效 |
| Current | 正在编辑，使用 interaction blue |
| Pending | 尚未配置或前序未满足 |
| Blocked | 被冲突、权限、证据或前置条件阻断 |

## 规则

- Global Navigation 不隐藏；Task Step Rail 只属于当前草稿。
- 所有步骤可见，当前步骤使用 `aria-current="step"`。
- Previous 不丢失有效草稿。
- Continue 先做当前 step validation。
- Save Draft 不代表可提交、已预检或命令有效。
- Precheck、Command Preview 和风险确认在相关字段变更后必须 stale。
- Submit 只在有效预检快照上可用。
- 命令预览只读、脱敏、来自控制面唯一生成器。

禁止把所有步骤做成相同 card grid，禁止让 Inspector 复刻完整表单。
