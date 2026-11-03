# 登录刷新、空白输入与原图柔边 · 2026-10-31

状态：本地源码修复及隔离合成验收完成；未提交、安装或替换现役控制面/Agent。原真实页面的旧 POST 历史条目不会被源码修改追溯清除，加载新版后应从地址栏重新打开 `/login` 建立 GET 页面。

## 结果与边界

- 浏览器失败提交改为 POST→303→GET：固定失败枚举经独立、非秘密、60 秒反馈 Cookie 传到 GET，读取后删除。首次落地保留 400/401/403/429/503 及固定中文反馈；刷新为干净 GET，不重新发送口令、不重复显示错误、不增加认证尝试。非导航/API 客户端保留原失败状态与成功 303 `/` 协议。

303 后取回资源使用 GET 的语义参考 [MDN 303](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Status/303)；自动填入属性仅是用户代理提示，边界参考 [MDN autocomplete](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Attributes/autocomplete)，实际本轮结果仍以下述真实 Handler 验证为准。
- 初始及刷新不再预置 admin 或回填输入；`pageshow` 清除恢复的表单值、busy、遮罩状态，缓存恢复清除旧错误。`autocomplete=off` 是浏览器提示，不代表清除了用户密码管理器的保存内容；用户主动使用密码管理器仍受浏览器控制。
- 按用户最后限定，原 `orchestration.jpg` 及配色逐字节保留，只用 CSS 透明渐变蒙版柔化斜切边缘。不使用前景提取稿、覆盖色层、调色滤镜或整页位图。SHA256 仍为 `22F819D31AE1CC3EFE9374C131D7D48D75C59F23FDB00F1BFBE12F0278BD9AD9`，浏览器脚本逐字节断言通过。
- LDO / 导数平台、透明客户标识、400px 表单、40px 控件、最小 960×560 的左右画布及小窗口自然滚动继续保留；不恢复能力/工具文案，不修改 Shell、全局 Token、权限、真实工具或业务能力。

权威入口：[登录页 P1](../../../design-system/pages/login.md)、[安全契约](../credential-access-security-contract.md#登录失败导航与一次性反馈)、[维护索引](../../../internal/deployment/loginpage/README.md)。新 `login_feedback.go` 只拥有固定反馈及导航，认证、来源校验、12 次/分钟限流、秘密校验和正式会话仍归 `browser.go`。

## 实现及证据版本

基线 HEAD：`4e8cb4034b474ac0cfe9ed9c63c8ae285963eeb0`。验收包含本轮工作树变更，不能仅以 HEAD 代表当前实现。证据目录：

`C:/Users/Dionysus/.codex/visualizations/2026/10/30/01a18abb-900c-7c22-8299-58bb5268a333/ldo-login-refresh-2026-10-31/`

`*.before` 保存本轮前的相关源码，`implementation.patch` 保存限定实现差异，`source-manifest.json` 记录当前源码/素材/设计规范的字节数及 SHA256，`source-assets/` 保存新增反馈实现及现役静态素材。仓库其他原有改动未覆盖或提交。

111 项浏览器断言使用独立 HTTPS `httptest` 服务和真实生产 Handler；账户、密码、TLS 证书、会话、网页包及目录全部为合成夹具。常规与限流两个独立 Browser 实例仅为隔离计数，未绕过认证、来源、Cookie 或限流处理。测试未访问 `18443` 现役服务、用户真实凭据、Agent、数据库或工具。

Browser 插件未提供，使用项目已有 Playwright `1.63.0` 和 Windows Chrome `156.0.8078.74`，无新增依赖。忽略证书错误仅限隔离浏览器上下文中的夹具自签名证书；真实网络限定到夹具回环 URL。跨站表单页面由 Playwright route 在本地生成，不连接其示例域名。夹具启动时的自签名握手日志不作为现役 TLS 信任验证或失败证据。

## 实际验证

| 检查 | 结果与证据 |
| --- | --- |
| `CGO_ENABLED=0 go test ./internal/deployment -count=1` | 通过，`go-test-final.log`；临时 TLS 夹具已移除后执行 |
| `CGO_ENABLED=0 go vet ./internal/deployment` | 通过，`go-vet.log` |
| `npm run tokens:check`、秘密扫描、限定差异检查 | 通过，`tokens-check.log`、`secret-check.log`、`diff-check.log` |
| 真实 Handler 浏览器 | 最终 111 项断言通过，`results.json`、`browser-check.log`、`check.mjs`；不累计中间尝试或重复运行 |
| 保留的 HTTP 合成预览 | PRG、刷新、调整窗口保留输入及缓存 pageshow 清理通过，`public-preview-check.json`、`check-public-preview.mjs`；此服务不认证或保存输入 |

Go 增加有意义的认证反馈负例：口令错误、外源/跨站拒绝、格式错误、限流、会话容量、未知反馈、反馈不能认证浏览器/机器或代替 CSRF、刷新不增加尝试、成功撤销残留反馈。既有 JSON Accept 的成功/失败、正式 Cookie、CSRF 与机器隔离测试继续通过；失败页面不再回填转义后的用户名，而是完全不回填输入。

真实浏览器验证默认空白 → 原生必填阻断 → 错误 POST 303 `/login` → GET 401 固定反馈 → 刷新 GET 200、字段与错误清空 → 再次刷新不新增 POST。另验证正式合成登录成功、后退 GET 清输入、显隐/图标、大写锁定、busy、防重复、禁用脚本的 PRG、跨来源拒绝及未知反馈清理。方法序列仅记录 GET/POST，不保存请求体或会话 Cookie。

真实跨站表单验证正确口令仍被来源策略拒绝，反馈在跳转后的顶级 GET 立即消费且刷新无迟到错误；仅非秘密反馈 Cookie 使用 SameSite=Lax，正式会话仍为 Strict。该反馈不携带账户/秘密或授权信息。限流实例第十三次失败落地 429 / `Retry-After: 60`；刷新清除提示，下一次提交仍受限流，不能靠刷新解除窗口计数。

视口实际覆盖 1920×1080、1440×1024、1280×720、1024×768、940×548、800×600、390×844。三档宽视口无横向溢出；小窗口按已确认最小画布滚动。原生 200% 使用独立临时 Chrome profile：同 1920×1080 外窗，内容视口 1904×985 / DPR 1 → 952×492 / DPR 2，CSS zoom=1、visual scale=1，输入仍为 14px / 40px，正常态和错误操作可访问。直接读取完整浏览器表面，未使用会裁切的原生 zoom fullPage 截图。

## 视觉验收与限制

已实际目视当前 `login-1920x1080.png`、`login-1440x1024.png`、`login-1280x720.png`、`login-failure-get.png`、`login-after-refresh.png` 及原生 200% 截图。原图颜色、纹理、相机及物体相对关系保留；变化只在原斜切区域的透明过渡。默认无账户值和错误，真实失败后有反馈，刷新清空。

P0 八项适用门槛均通过：本页 P1 两列对齐不变；控件尺寸与状态一致；原生表单沿用认证前例外，无表格/抽屉/弹窗；busy、显隐、错误和减少动态效果实测；删除默认值及多余反馈而未增加密度；1920 实际截图与 1440/1280 响应式已目视；原图软蒙版使用既有透明语义，不新增 Token 或第二套控件。空态、危险确认、业务保存与 Drawer/Modal 焦点恢复不适用。

当前 `127.0.0.1:15232/login` 仅提供原生产模板导出的合成预览，POST 丢弃输入并模拟一次性失败；不能据此声称真实账户可登录。真实 Handler 的 TLS 夹具已停止，临时 Go 夹具与 Chrome profile 已清理，复核材料保留。

未安装或重启现役服务，未做真实账号登录、跨浏览器或其他操作系统验证，不提升真实执行或发布门禁。现役更新遵守项目二进制流程；在开发入口加载新 Go 版本后重新打开 GET 登录页，不能把旧 POST 历史条目的第一次重提提示当作新流程仍在重提。
