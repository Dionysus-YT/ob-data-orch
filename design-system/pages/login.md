# Login · Page Spec

> 状态：P1 · 2026-10-28 · 依据 [DEC-067](../../docs/01-product/decisions.md#dec-067-登录页采用客户品牌标识)
>
> 实现入口：`internal/deployment/loginpage/`（`login.html`、`login.css`、生成的 `tokens.css`、`brand-logo.png`），由 `internal/deployment/login_page.go` 嵌入渲染。

登录页位于 Product Shell 之外，不属于任何 Archetype；只收窄 P0，不重定义 Shell、Token 或组件语义。

## 结构

| 区域 | 内容 | 规则 |
| --- | --- | --- |
| 品牌区（左，约 53%） | 原始标识 → 工具版本眉标 → 产品名称 → 定位 → 导出 / 普通导入 / 旁路导入 | 白色工作面；标识按原图比例，高度不超过 57px；纹样固定右下角，纯装饰 `aria-hidden` |
| 登录区（右） | 400px 卡片：标题、失败提示、账户、密码、登录、传输与会话说明 | 页面底色 + 主工作面卡片（细边、`surfaceShadow`、8px 圆角） |

## 规则

- 颜色、字体、间距、圆角只消费 `tokens.ts` 生成的 Token；`tokens.css` 由 `npm run tokens:generate` 同时写入前端与登录页，`tokens:check` 校验两处一致。
- 标识蓝 / 绿是页面私有变量，只用于纹样与眉标短线；交互、焦点使用 Primary，失败使用 Danger。
- 认证前没有 Ant Design：输入框、主按钮与失败提示按 Ant 现役视觉（40px large 控件、2px 焦点光晕、Alert error）手写，仅限本页。
- 失败反馈只在服务端响应后出现，使用 `role="alert"`；密码框标记 `aria-invalid`，输入后清除。不展示服务端原始错误。
- 渐进增强脚本只负责显示/隐藏密码、大写锁定提示与防重复提交；脚本不可用时原生表单仍可登录。提交前恢复密码遮罩。
- 视口：≥1440 全尺寸；1280×720 收紧边距与标题，卡片完整可见；<960px 或 200% 缩放时单列，品牌区收为顶部条带并隐藏能力行与纹样。
- 不增加记住密码、找回密码、注册、SSO、验证码、版权或营销文案。
