# Diagnostic Detail Pattern

Diagnostic / Detail 按 **Result / Failure Fact → Reason → Evidence** 组织。稳定对象身份和状态先于失败或结果事实；原因紧随其后；证据层承载不可变配置、脱敏命令、日志、操作记录和输出事实。

## 层级

1. Object identity：任务、节点、数据源或凭据的稳定身份。
2. Current result：成功、失败、取消、运行中或待核对。
3. Reason：失败原因、错误码、阻断域、可恢复性。
4. Evidence：配置快照、命令、日志、操作记录、输出事实。
5. Recovery：可执行的下一步或下钻入口。

## 视觉

- Result / Failure 使用紧凑 semantic alert surface。
- Reason 的标题、文字和 error code 高于普通 metadata。
- Command 和 Log 使用 technical evidence surface、等宽字体、内部横滚或可换行策略。
- Operation History 使用时间顺序与轻量轨迹，不伪造调用链。
- 不 Card 化所有证据分区，不使用 Wizard skeleton。

## 长值与脱敏

- 命令默认脱敏，只读，可复制脱敏版本。
- 日志保留来源、时间、级别、完整性和脱敏事实。
- 长错误、路径、对象名、Job ID 使用 `overflow-wrap: anywhere` 或可访问完整值入口。
