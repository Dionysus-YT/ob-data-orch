# 登录页维护索引

认证前页面由 `../login_page.go` 通过 `go:embed` 和 `html/template` 渲染。设计权威为[登录页 P1](../../../design-system/pages/login.md)，品牌决策见 [DEC-067](../../../docs/01-product/decisions.md#dec-067-登录页采用客户品牌标识)。

| 入口 | 唯一职责 |
| --- | --- |
| `../browser.go` | 真实认证、同源、限流及会话建立；不持有页面局部展示状态 |
| `../login_feedback.go` | 固定失败码、浏览器 POST→303→GET、一次性非秘密反馈 Cookie 的设置/消费；不得授予身份或保存输入 |
| `../login_page.go` | 嵌入可信素材和模板；仅接收固定反馈，不接收或回填登录输入 |
| `login.html` | 透明客户标识、LDP / 导数平台、唯一空白原生表单、固定错误反馈；渐进增强负责显隐、大写锁定、防重复、初始焦点和 pageshow 清理 |
| `login.css` | 登录页布局及认证前 Ant large 对齐状态；不引入共享控件或全局 Token |
| `tokens.css` | 从 `web/src/platform/tokens.ts` 生成；通过 `npm run tokens:generate` 更新，不手工修改 |
| `brand-logo.png` | 用户提供的原始青海农信 QHRC 标识，保留作来源比对，不作为当前页面资源 |
| `brand-logo-transparent.png` | 按用户注释去除矩形白底的透明版本，275×57 PNG；通过 `../login_page.go` 内嵌，不放大超过原图 |
| `ldo-mark.png` | 用户选定第一款 LD 字形，256×256 透明 PNG；登录正文、favicon 及登录后顶栏的唯一素材来源 |
| `orchestration.jpg` | 用户选定第二版原始场景，1254×1254 JPEG；保持文件及配色，不替换、调色或拆分前景 |

图标与场景最初使用内置 ImageGen 从定稿提取并做交付优化；原始文件保留在生成目录。当前场景 JPEG 的 SHA256 固定为 `22F819D31AE1CC3EFE9374C131D7D48D75C59F23FDB00F1BFBE12F0278BD9AD9`。用户明确只柔化斜切边缘，`login.css` 在原图上使用透明渐变蒙版；不能用重新生成的前景、覆盖色层或滤镜改变配色。不得用整页位图代替可访问表单。

客户标识透明版另以原始 PNG 为编辑目标通过内置 ImageGen 去白底，保留图案、字样与蓝绿配色；仅裁去多余透明画布并归一到原交付尺寸。用户最终要求删除三类能力和工具名称，页面不保留这些展示，也不将其写入插画。

缩放布局由 `login.css` 的两列最小尺寸和媒体查询负责：画布至少 960×560，两列至少 384/576px，始终左右排列，空间不足自然滚动。视口至少 900px 时以 `preventScroll` 初始聚焦账户；调整窗口不重置正在输入的内容。刷新/后退是新的展示会话，应清空表单，不恢复旧失败反馈。

新增/替换素材时同步检查 `../login_page.go` 的嵌入与 MIME 类型、P1 和页面截图。所有图片必须为页面内嵌资源，不能依赖登录后的网页包或外部地址。认证、同源、限流和会话规则仍由 `../browser.go` 承担，不移入页面。

登录后的顶栏由 `web/src/components/ProductHeader.vue` 实现，消费同目录 `assets/ldo-mark.png` 的逐字节交付副本，由 Vite 打包。更新本目录的 LD 源图时同步该副本并核对 SHA256；不跨 Go embed 包与 Vite 源码根目录读取素材，不改登录页的内嵌资源约束。顶栏布局与品牌规则以 P0 为准。

2026-11-02 按用户确认统一展示品牌为 LDP（Loader & Dumper Platform）；登录页、顶栏、默认和路由标题、帮助弹窗同步更名。LD 图标仍为同一素材，`ldo-mark.png` 是保留的资源文件名；认证、路径与运行标识不随展示品牌更名。登录模板随控制面编译嵌入，源码更名不代表现役二进制已更新。

顶栏变更定向检查前端构建、`audit:platform` 及数据源标准页/首页/向导/详情的品牌与导航可达性；三档桌面、窄屏和原生 200% 记录见[LDO 追加验收](../../../docs/03-technical/evidence/login-ldo-2026-10-31.md#登录后顶栏品牌追加验收)。

定向验证：`CGO_ENABLED=0 go test ./internal/deployment`、`go vet ./internal/deployment`、`web/` 下 `npm run tokens:check`。实际浏览器检查三档桌面、窄屏、原生 200%、键盘、错误与重复提交、原生表单退化；本轮记录见[LDO 验收](../../../docs/03-technical/evidence/login-ldo-2026-10-31.md)。

刷新缺陷须用隔离的真实 TLS Handler 验证 POST→GET 和 Cookie 消费，不以静态错误 HTML 或模拟 303 代替；检查刷新不新增 POST、原始客户端状态保留、限流不被刷新绕过、未知反馈不能授予身份。最新证据见[刷新与柔边验收](../../../docs/03-technical/evidence/login-refresh-2026-10-31.md)。
