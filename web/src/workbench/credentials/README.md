# 存储凭据维护索引

规范沿用 [AGENTS](../../../../AGENTS.md)、[前端基线](../../../../docs/03-technical/frontend-platform-baseline.md#前端业务模块架构)及[凭据安全契约](../../../../docs/03-technical/credential-access-security-contract.md)，不另建规则体系。

| 修改目标 | 唯一入口 |
| --- | --- |
| 页面布局、列、控件、装配 | `views/StorageCredentialsView.vue`；单张管理工作面包含标题/操作、筛选与列表，新增/轮换使用原生 Ant Drawer；模板只将取消意图交给能力，不维护表单副本；焦点复用 `useAntDrawerDialog` |
| 列表、筛选、可信事实、错误和共享写锁 | `useCredentialList.ts`；独立读取代、一个 AbortController，写入使旧读取失效，身份拒绝清空列表 |
| 创建/轮换及短时敏感输入 | `useCredentialEditor.ts`；唯一表单、编辑代、打开时目标修订；关闭/切换/成功/卸载清输入，当前失败保留重试 |
| 删除确认与版本事务 | `useCredentialDeletion.ts`；目标与确认代、方法防重、取消/卸载失效；列表 401/403 同步清目标并关闭确认，授权阻断期间不能重开 |
| 校验、provider、筛选 | `storageCredentialList.ts`；从 views 原样迁入的唯一纯规则，原 `.test.ts` 同步迁移 |
| HTTP/安全投影 | `api/browser.ts`；不修改 API、CSRF、If-Match、幂等或密钥读取边界 |

列表不保存秘密，编辑器消费原列表而非同步副本。编辑器关闭时旧事务继续持有写锁直到结束，避免迟到 finally 解除另一事务的锁；请求栈在 finally 清除材料引用。浏览器输入仍是字符串，清引用不等于物理擦除。取消浏览器等待不能撤销已到达服务端的写入，返回页面重新读取事实。

页面只保留密钥保存后不回显、轮换后的草稿绑定及删除所有版本的影响；信封、AAD 和修订加密细节以安全契约为准，不进入日常表单说明。表单和秘密生命周期仍由原编辑器唯一持有。

编辑器关闭或切换时同步清除该会话失败，重新打开不会重放旧错误；当前失败仍保留输入便于重试，成功消息仍由原列表通知发布，清除失败不清除已提交的成功回执。

页面的 `formModel` 只组合编辑器已有的 ref，Ant 字段名与领域字段一致，自动校验立即读取同一输入；不在渲染时复制秘密值或新建第二份表单。字段纠正后必须撤下旧必填错误，服务端失败仍在当前抽屉保留输入供重试。

测试：`credentialLifecycle.test.ts`、原规则测试；浏览器 `business-lifecycle.spec.ts` 与原 `remaining-pages.spec.ts`。`storage-credentials.spec.ts` 覆盖三档视口和原生 200% 的抽屉键盘、失败重试、清引用与焦点恢复；编辑失败在当前抽屉显示，不被遮罩挡住。历史验收见[第三阶段记录](../../../../docs/03-technical/evidence/frontend-business-phase3-2026-10-09.md)，本轮见[体验整改记录](../../../../docs/03-technical/evidence/product-experience-fixes-2026-10-27.md)。
