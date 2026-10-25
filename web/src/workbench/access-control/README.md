# 权限配置维护入口

入口 `views/AccessControlView.vue` 只装配本模块。设计沿用 P0、数据源定版管理列表与既有 Drawer / 确认组件；业务以[首批契约](../../../../docs/03-technical/access-control-contract.md)为准，阶段只查任务地图。

页面从“系统管理 > 权限配置”进入，与平台设置、调度策略、安全策略、工具与兼容及存储凭据为独立子项；兼容地址 `/settings/access-control`。全局分组及折叠菜单由 `components/ProductSystemNavigation.vue` 持有，不将导航展开状态或其他模块表单并入本模块。

| 能力 | 唯一所有者 | 边界 |
|---|---|---|
| 用户目录、管理资格、筛选与游标 | `usePermissionList.ts` | 每次目录读取复验；失败不把旧数据当作写资格，无权清空数量和元信息 |
| 单用户权限、草稿、比较与写事务 | `usePermissionEditor.ts` | 唯一表单，编辑代处理 A → B → A；版本冲突保留草稿并阻断覆盖；确认前不写入 |
| 候选查询 | `usePermissionCandidates.ts` | 每个范围独立绑定编辑代、类型、搜索与分页；有界加载，不合并其他用户响应 |
| 固定角色依赖、差异与说明 | `permissionRules.ts` | 纯规则，与服务端固定策略对应；最终由服务端判定 |
| 严格输入输出和浏览器传输 | `api/accessControl.ts`、`api/browser.ts` | CSRF、If-Match、幂等与取消信号复用现有安全传输，不提交操作者 |
| 列表、编辑器与范围控件 | `PermissionWorkspace.vue`、`PermissionEditor.vue`、`PermissionScopePicker.vue` | 消费事实与操作意图，不拥有第二份表单或权限缓存 |

列表刷新期间临时撤销页面写资格，正在编辑的草稿不自动覆盖。关闭、切换身份、权限拒绝、成功和卸载清除引用与提示；迟到成功、失败和 finally 不能重放到其他会话。写操作的方法入口防重；不以按钮 disabled 替代。

审计能力已接入独立日志预设，可为其他 ACTIVE 主体显式授予，新增须风险确认；自身只读。其余未接入完整消费链路的独立能力和 `NODE_OPERATE` 只保留/撤销旧记录，不允许新增。固定 admin 在真实部署中仍是唯一登录身份；测试里的多个主体不代表增加认证账号。

验证入口：`permissionLifecycle.test.ts`、`api/accessControl.test.ts`、`tests/access-control.spec.ts`。模块 CSS 仅负责角色列表、筛选换行、差异滚动与分页位置；表格、工作面、字体、控件、弹层、反馈复用平台，不复制数据源 CSS、不新增 Token。
