# 数据源管理 UI Reference Page

> 状态：Design System v0.1 正式参考页
>
> 页面 Route：`/data-sources`
>
> Fixture Route：`/data-sources?uiFixture=data-sources`
>
> 关联规范：[Design System v0.1](design-system-v0.1.md)
>
> 冻结日期：2026-08-13

## 1. 为什么选择数据源页

数据源管理在单一业务链路中同时验证了 Product Shell、Page Header、FilterToolbar、High-density DataTable、Environment / Runtime / Enable 三类语义、Row Actions、Drawer、FormField、Connection Test、ConfirmDialog、空状态和响应式列宽。它能够代表数据库运维工作台的主要桌面交互，又没有引入 Wizard、日志流或图表等尚未成熟模式。

Reference Page 的作用是提供可运行、可测量的实现证据。它不把数据源业务结构强制复制到其他模块，也不覆盖[数据源管理产品规则](data-source-management.md)及 API / 安全契约。

## 2. 当前业务结构

列表当前为 11 列：

`数据源 | 环境 | IP/域名 | 端口 | 租户名 | 数据库用户名 | SYS 租户 | 租户模式 | 连接状态 | 启用状态 | 操作`

这是数据源业务结构，不是 Design System 的固定列数。其他页面复用的是表格 Density、Header、Divider、Metadata、Status、Action、Sort 和 Width Strategy。

当前排序只提供给数据源、环境、IP/域名、租户名、连接状态五个高价值字段。直接行操作为 `编辑 / 测试 / ···`，启用、禁用和删除 / 归档进入 More Menu。

## 3. 正式冻结范围

- 216px Sidebar、48px Global Header、宽内容工作台布局；
- Page Title、说明、单一“新增数据源”主操作；
- 无 Card 的 FilterToolbar 和即时筛选；
- 38px Header / 52px Row 的高密度表格；
- 名称 + ID、连接状态 + 时间的主信息 / Metadata 分层；
- Environment、Runtime、Enable 三种独立视觉语义；
- 业务权重列宽、Ellipsis、完整值提示和紧凑操作列；
- 640 / 680px Drawer、Section 化表单、固定 Footer、独立 Body Scroll；
- Label / Required / Helper / Error 的 FormField 节奏；
- More Menu、Danger Confirm、未保存变更确认、Focus 与 Scroll Lock。

冻结指视觉与交互基准已经成立，不表示页面 API、业务字段或安全规则永不变化。

## 4. 响应式验收基线

| Viewport | 验收重点 | 当前基线 |
|---|---|---|
| 1280px | 保留 11 列；无整表横向滚动；Filter 合理收缩；Drawer 640px | 已验证 |
| 1440px | Sidebar、Toolbar、Table、Action、Drawer 的主要设计节奏 | 主设计基准，Drawer 680px |
| 1920px | 不限制整个页面为窄容器；主要列适度吸收空间，短列与操作列紧凑 | 已验证，Drawer 680px |

验收必须使用浏览器实际渲染和 16 行 Fixture，不能只以 CSS 推断或历史截图代替。1280px 下若新增业务必要列，应先重新评估列权重；不得直接隐藏 IP/域名、端口、租户名或状态等高频定位事实。

## 5. Reference 与业务专用边界

以下内容可作为 v0.1 直接参考：

- Shell、Typography、Token、Button、Icon、FormField 和 Status 基础组件；
- Page Header、FilterToolbar、DataTable、Row Actions、Drawer 和 ConfirmDialog 模式；
- 桌面端 1280–1920px 的信息密度与响应式策略。

以下内容只属于数据源业务：

- 11 个具体列及其字段顺序；
- 数据源环境枚举、连接状态枚举、SYS 租户事实和租户模式；
- 编辑 / 测试动作、连接测试流程、节点选择与凭据规则；
- 启用、禁用、删除 / 归档的业务影响文案。

## 6. 验证入口

开发环境启动前端后访问：

```text
/data-sources?uiFixture=data-sources
```

Fixture 规则和覆盖矩阵见 [UI Regression Fixture](ui-regression-fixture-data-source.md)。真实单行数据用于确认正常 API 投影，Fixture 用于确认密度、边界文本和状态组合；两者不能互相替代。

## 7. 已知非阻断问题

- 当前表格和 Drawer 是成熟模式，但尚未抽成完全业务无关的公共容器组件。
- ConfirmDialog 仍为数据源专用实现；第二个真实消费者出现前不提前建立大型 Dialog Framework。
- 全局旧页面样式尚未迁移，不能以旧 `.button`、Card 或 Unicode Icon 反向覆盖 Reference Page。
- 小于 1280px 的桌面 / 移动体验没有达到与 1280–1920px 相同的验证等级。

## 8. 变更门禁

修改本页前必须标记 Business Change、Bug Fix 或 Design System Change。若修改 Token、公共组件、Sidebar、表格 Density、Drawer 尺寸、Status Language 或 Icon System，必须同步更新 [Design System v0.1](design-system-v0.1.md)，说明影响范围，并判断是否需要升级版本。
