# 数据源 UI Regression Fixture

> 状态：长期保留
>
> 适用环境：DEV only
>
> 启用参数：`?uiFixture=data-sources`
>
> 视觉与响应式验收规则：[P0 MASTER](../../design-system/MASTER.md)。本 Fixture 只提供 DEV-only 合成测试输入，不定义页面视觉。
>
> 当前说明核对日期：2026-09-14；页面定版与复用规则见 [数据源标准页](../../design-system/pages/data-sources.md)。

## 1. 用途

该 Fixture 为现役数据源标准页提供可重复的 7 行合成输入，用于人工 UI Review、浏览器响应式检查和前端回归测试，覆盖列表排版、筛选、状态组合和内存中的抽屉交互。7 行只覆盖单页，不能单独证明多页导航正确；翻页验证须另用隔离的合成数据或 API 夹具，不向真实数据源写入测试记录。

Fixture 是稳定 UI Regression 数据源，不是自动 Screenshot Diff 系统，也不能证明真实 API、真实凭据或真实连接测试可用。

## 2. 代码位置

- 数据定义与启用规则：[web/src/views/dataSourceUiFixture.ts](../../web/src/views/dataSourceUiFixture.ts)
- 规则测试：[web/src/views/dataSourceUiFixture.test.ts](../../web/src/views/dataSourceUiFixture.test.ts)
- 现役页面：[SourceWorkspace.vue](../../web/src/workbench/sources/SourceWorkspace.vue)
- 内存网关与验证：[sourceGateway.ts](../../web/src/workbench/sources/sourceGateway.ts)、[sourceGateway.test.ts](../../web/src/workbench/sources/sourceGateway.test.ts)

## 3. 启用与失败关闭

只有同时满足以下条件才返回 Fixture：

1. Vite `DEV` 环境；
2. Query 中只有一个 `uiFixture` 参数；
3. 参数值精确等于 `data-sources`。

空参数、错误值、重复参数、非 DEV 环境均不启用 Fixture。有效参数命中后，现役页面创建内存网关处理数据源操作；未命中会使用真实浏览器 API，不能把错误参数理解为隔离模式。操作前必须看到“视觉验证”提示，并只使用合成输入。

开发环境访问：

```text
/data-sources?uiFixture=data-sources
```

## 4. 覆盖矩阵

7 行固定合成数据覆盖：

- 不同英文名称、数据库用户名、集群及租户；
- Reserved TEST-NET 地址 `192.0.2.18`，固定 SQL 端口 `2883`；
- MySQL / Oracle；开发 / 测试 / 预生产 / 生产；
- 测试成功、连接失败、未测试、测试中、已失效和待确认；
- 已启用 / 已停用；sys 凭据配置摘要 AVAILABLE / UNAVAILABLE；
- 名称与次要信息、操作列和生命周期资格。当前列表不提供排序操作；过期状态、长文本和多页边界需另行补充合成输入。

Fixture 不包含真实主机、真实租户、真实用户名、真实密码、真实连接串或真实数据源 ID。

## 5. 无真实业务副作用

Fixture 模式下，现役 `SourceWorkspace` 和 `SourceEditor` 使用内存数据源网关：

- “新建数据源”、编辑、保存和生命周期操作只修改本页内存，刷新浏览器后重新初始化；页面内“刷新”读取当前内存状态。
- 测试仅选择 `Visual validation runtime` 合成节点，通过内存网关模拟进行中、失败或失效，并返回 `G2_SYNTHETIC` / `realConnectionVerified=false`。
- 搜索、筛选和分页在当前合成集合内计算；不调用真实数据源 API、不写 SQLite、不触发 Agent 或真实数据库连接。
- 旧 `DataSourceListView.vue` 的仅列表、禁用编辑模式不代表当前标准页行为。

该阻断是 UI Review 的防误操作边界，不是权限或安全边界。生产环境仍必须依赖正常认证、授权和服务端校验，不能把 Fixture 逻辑当作安全控制。

## 6. 回归检查清单

每次影响数据源管理页面的视觉或交互修改，至少检查：

- 1920×1080、1440×1024、1280×720 下没有非预期整表横向滚动；
- 表格行高、可见列和低优先级 metadata 按 [P0 MASTER](../../design-system/MASTER.md#8-responsive-architecture) 的密度与降级规则验证；
- 长名称只截断本列，不抬高行高；完整值仍可获得；
- Host、Port、Tenant、Username 能快速纵向扫描；
- 环境、连接测试与可用状态不混用业务含义；
- 短状态文字与圆点居中，完成时间和证据在抽屉查看；
- 统计在底部左侧，分页在右下角，首末页按钮正确禁用；多页另用合成夹具验证；
- 行内测试与菜单对齐，More Menu 不被表格裁剪；
- 新建、编辑、测试与 More 操作使用合成输入，确认均由内存网关承接；
- 浏览器 Console 无新增 Warning / Error。

## 7. 自动验证

当前单元测试验证 7 行固定身份、字段和状态覆盖、DEV-only、精确参数匹配、重复参数不启用和返回副本；网关测试覆盖内存交互与秘密不保留。推荐命令：

```powershell
cd web
npm run test -- src/views/dataSourceUiFixture.test.ts src/workbench/sources/sourceGateway.test.ts
npm run typecheck
```

视觉回归仍需实际浏览器检查；当前未接入像素级 Screenshot Diff。

## 8. 长期维护规则

- 保持 7 行固定输入和覆盖矩阵稳定；不要把真实环境数据引入 Fixture。内存模拟测试时间属于交互状态，不是固定列表基线。
- 业务字段确实变化时，同步更新 Fixture、测试、当前页面的列结构和覆盖说明。
- 为通过某次截图而随意改数据会破坏回归价值；变更必须说明它新增或替代了哪个边界场景。
- Fixture 只服务数据源页面；其他模块应在真实需要时建立独立、最小、DEV-only Fixture，不能把数据源字段强行复用。
