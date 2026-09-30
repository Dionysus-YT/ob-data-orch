# Task Builder Pattern

Export、Normal Import、Direct Load 共享 Task Builder Interaction Language，但各自的步骤 schema、资格、风险、命令语义和提交门禁独立。

## 结构

```text
Task Header
  saved / dirty state, Save Draft, current step context
Task Step Rail
  导出使用顶部水平进度栏；其他向导在 1920 / 1440 使用紧凑竖向进度栏
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
