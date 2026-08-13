# 数据源 UI Regression Fixture

> 状态：长期保留
>
> 适用环境：DEV only
>
> 启用参数：`?uiFixture=data-sources`
>
> Reference Page：[数据源管理 UI Reference Page](ui-reference-page-data-source.md)
>
> 定版日期：2026-08-13

## 1. 用途

该 Fixture 为数据源 Reference Page 提供可重复的 16 行合成输入，用于人工 UI Review、浏览器响应式检查和前端回归测试。它解决真实环境只有少量数据时无法验证 Table Density、Ellipsis、Metadata、列宽、排序、状态组合和 Row Actions 的问题。

Fixture 是稳定 UI Regression 数据源，不是自动 Screenshot Diff 系统，也不能证明真实 API、真实凭据或真实连接测试可用。

## 2. 代码位置

- 数据定义与启用规则：[web/src/views/dataSourceUiFixture.ts](../../web/src/views/dataSourceUiFixture.ts)
- 规则测试：[web/src/views/dataSourceUiFixture.test.ts](../../web/src/views/dataSourceUiFixture.test.ts)
- 页面接入：[web/src/views/DataSourceListView.vue](../../web/src/views/DataSourceListView.vue)

## 3. 启用与失败关闭

只有同时满足以下条件才返回 Fixture：

1. Vite `DEV` 环境；
2. Query 中只有一个 `uiFixture` 参数；
3. 参数值精确等于 `data-sources`。

空参数、错误值、重复参数、非 DEV 环境均失败关闭。命中后返回独立数组副本，避免单次筛选或排序污染稳定基线。

开发环境访问：

```text
/data-sources?uiFixture=data-sources
```

## 4. 覆盖矩阵

16 行合成数据至少覆盖：

- 短名称、超长中文名称、超长英文名称和中英文混排；
- Reserved TEST-NET IPv4 与 `.test` 保留域名；Host 长度不超过当前 UI 约束 15 位；
- 不同 SQL 端口；1–8 位租户名；多种数据库用户名；
- MySQL / Oracle；开发 / 测试 / 预生产 / 生产；
- 可连接、连接失败、未测试、测试已失效；
- 已启用 / 已禁用；SYS 租户已配置 / 未配置；
- 名称和 ID Metadata、状态时间、操作列及五个可排序字段。

Fixture 不包含真实主机、真实租户、真实用户名、真实密码、真实连接串或真实数据源 ID。

## 5. 无真实业务副作用

Fixture 模式仅替换列表读取输入，并在页面交互入口阻断真实 CRUD / 测试动作：

- “新增数据源”不会打开可提交表单；
- “编辑”和“测试”不会读取真实数据源详情；
- More Menu 可以用于视觉验证，但启用、禁用、删除 / 归档不会调用真实 API；
- Refresh 只重新装载 Fixture；筛选和排序只在浏览器内处理；
- 不写入 SQLite，不发起数据库连接，不选择执行节点，不触发 Agent 或控制面业务动作。

该阻断是 UI Review 的防误操作边界，不是权限或安全边界。生产环境仍必须依赖正常认证、授权和服务端校验，不能把 Fixture 逻辑当作安全控制。

## 6. 回归检查清单

每次影响 Reference Page 的视觉或交互修改，至少检查：

- 1280px、1440px、1920px 下没有非预期整表横向滚动；
- 52px Row Height 在 16 行下不过松或过密；
- 长名称只截断本列，不抬高行高；完整值仍可获得；
- Host、Port、Tenant、Username 能快速纵向扫描；
- Environment、Runtime、Enable 不混用同一视觉语义；
- 状态文字、图形和时间不跳列；
- Sort Header 的默认、Hover、Focus、升序、降序状态稳定；
- `编辑 / 测试 / ···` 对齐，More Menu 不被表格裁剪；
- 新增、编辑、测试和 More 中的写操作均被 Fixture 边界阻断；
- 浏览器 Console 无新增 Warning / Error。

## 7. 自动验证

当前已有单元测试验证 16 行数量、字段覆盖、Host / Tenant 边界、DEV-only、精确参数匹配、重复参数失败关闭和返回副本。推荐命令：

```powershell
cd web
npm run test -- --run src/views/dataSourceUiFixture.test.ts
npm run typecheck
```

视觉回归仍需实际浏览器检查；当前未接入像素级 Screenshot Diff。

## 8. 长期维护规则

- 保持 16 行和覆盖矩阵稳定；不要把随机数据、当前时间或真实环境数据引入 Fixture。
- 业务字段确实变化时，同步更新 Fixture、测试、Reference Page 的列结构和覆盖说明。
- 为通过某次截图而随意改数据会破坏回归价值；变更必须说明它新增或替代了哪个边界场景。
- Fixture 只服务数据源页面；其他模块应在真实需要时建立独立、最小、DEV-only Fixture，不能把数据源字段强行复用。
