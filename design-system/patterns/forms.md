# Forms Pattern

表单优先使用 Ant `Form` / `FormItem`。领域校验只提供状态、文案和字段映射，不重新绘制通用 Field wrapper。

## 必须具备

- visible label。
- required / optional 状态清楚。
- persistent helper。
- inline validation。
- field-level error。
- API field error 映射。
- loading、disabled 与 busy feedback。
- error summary 可聚焦并能定位字段。

## 验证时机

- 首次进入不报错。
- touched / blur 后验证字段。
- Continue、Precheck、Submit 或 Save 时验证相关字段。
- Server validation、authorization、revision、uniqueness 和 lifecycle eligibility 始终是决定性结果。

## 布局

- 中文 label 不依赖 placeholder。
- 复杂表单按业务分组，分组以 typography、spacing 和 hairline divider 建立层级。
- 每个 major form 最多一个 `Advanced Settings` disclosure。
- 条件字段隐藏后不得进入提交 payload。
- 长 technical value 使用 `min-inline-size: 0` 和 `overflow-wrap: anywhere`。

## 禁止

- placeholder 替代 label。
- 自建通用 `OrchField` 状态体系。
- 客户端校验声称真实数据库、对象存储或工具可达。
- 密钥、连接串、完整敏感命令进入日志、错误、测试输出或快照。
