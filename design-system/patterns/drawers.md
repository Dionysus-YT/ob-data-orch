# Drawers Pattern

Drawer 用于保持当前页面上下文的编辑、摘要、日志同源上下文或窄屏 Inspector 替代。使用 Ant `Drawer`。

## Anatomy

```text
Header
  fixed title, close control
Body
  independent scroll area
Footer
  optional fixed action bar
```

## 行为

- 打开后焦点进入标题或第一个安全控件。
- Tab / Shift+Tab 不逃出 Drawer。
- ESC 在无不可逆 busy 时关闭。
- 关闭后焦点返回触发控件。
- dirty、testing、permission、unsaved confirmation 属于 Product Behavior。

## 视觉

- Drawer surface 为白色。
- 普通右贴 Drawer 不使用圆角。
- 阴影只用于浮层存在感，不用于普通页面分区。
- 不重绘 mask、motion、focus state、close icon 和 button feedback。

## 禁止

- Drawer 代替完整页面。
- Drawer 内再做第二套应用壳。
- 复杂长期编辑塞入 Modal。
- 页面 CSS 覆盖 Ant Drawer 的基础交互状态。
