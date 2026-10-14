# 执行节点注册/编辑重构 · 2026-10-10

## 结果与范围

注册与编辑从独立大表单收敛为节点列表中的右侧 Ant Drawer，复用数据源定版的宽度、控件、浅灰分组、反馈与底栏。原有表单路由保留为列表子路由，筛选与自动刷新间隔保留；新建成功仍进入节点详情进行独立 Agent 关联。未新增协议字段、API、注册协议、依赖、全局 Token 或 Shell。

表单唯一所有者仍为 `useNodeForm.ts`：管理配置、修订、dirty 指纹、校验、保存结果与 GiB/字节转换。空预算、清空数字控件及零值保持自动策略；编辑原有预算继续准确回填。页面只协调抽屉、未保存导航、错误定位、焦点与列表事务。

关键行为：

- 未保存关闭、离开或切换节点先确认，默认焦点为“继续编辑”，取消保留草稿。保存期间拒绝关闭与导航，方法入口防重。
- 抽屉保存期间复用列表原 mutation 边界，使旧读取失效并跳过周期请求；结束和卸载成对释放。编辑成功重新读取列表，打开抽屉本身不会重置或停止列表刷新。
- 401/403/404 清除表单配置与提交资格，列表授权撤销同样使当前表单失效。授权重新读取后也不接受撤销前的迟到写响应。
- 汇总链接定位实际字段，预算错误先展开唯一可选区。帮助与错误关联输入；目录 FormItem 绑定 `allowedRootsText`，避免有效目录仍出现 Ant 必填错误。
- 注册入口关闭返回原触发器；编辑菜单项关闭后不再可见，打开抽屉前把焦点交给对应行操作按钮，关闭后返回该按钮。背景 inert 恢复有独立断言。
- 一次性注册材料继续只属于详情页的 `useNodeEnrollment.ts`，未进入表单、列表、路由或持久化。

设计与维护入口已同步：[节点 P1](../../../design-system/pages/execution-nodes.md#注册与编辑抽屉)、[节点设计 §8](../../02-design/execution-node-module.md#8-注册与编辑节点)、[节点维护索引](../../../web/src/workbench/nodes/README.md)。

## 合成验证

本节记录首次抽屉重构的验证；同日后续的操作系统与 CPU 架构分类结果见文末追加记录。

所有浏览器请求由合成路由响应接管，Vite 使用隔离端口 15174；没有 API 代理或真实 Agent、数据源、存储和工具操作。浏览器运行仅在子进程环境清除不兼容代理，不修改系统配置。

| 检查 | 本轮结果 |
| --- | --- |
| `npm run test -- src/workbench/nodes` | 53/53 通过 |
| `npm run test` | 当前工作区全量 294/294 通过 |
| `npm run typecheck` | 通过 |
| `npm run lint` | 通过，最终焦点修改后复验通过 |
| `npm run build` | 通过，最终焦点修改后复验通过 |
| `npm run audit:business` | 正反测试 11/11，0 violations；6 个既有 REVIEW，最终复验通过 |
| 注册、原节点页面/生命周期、节点资源 Chrome 回归 | 最终完整 22/22 通过 |
| `scripts/check-secrets.ps1` | 通过 |
| `git diff --check` | 通过 |

注册新增 7 项浏览器场景：[execution-node-registration.spec.ts](../../../web/tests/execution-node-registration.spec.ts)，覆盖筛选保留/双入口焦点、未保存关闭与同实例切换、错误定位、预算 DTO、新建后独立签发、服务端预算错误、写期间刷新/导航、授权撤销，以及 1920/1440/1280/640/320 和 200% 缩放。原生命周期回归继续覆盖迟到读取与模式切换；旧写响应、卸载和授权撤销后的回写负例由节点单测覆盖。资源回归继续验证旧预算回填/清空、额度、资源、刷新和删除。

最终浏览器命令为 `npm run test:browser -- tests/execution-node-registration.spec.ts tests/execution-node-page.spec.ts tests/execution-node-lifecycle.spec.ts tests/execution-node-resources.spec.ts`。截图位于忽略目录 `artifacts/frontend-platform/` 的注册测试结果中；320px 与 200% 使用视口截图，避免将固定抽屉外的文档高度误当作可滚动界面。

首轮发现并修正目录 FormItem 绑定与编辑关闭焦点问题；旧测试的模糊字段定位、缺少 `code` 的错误夹具，以及依赖“编辑会卸载列表/保存期允许离开”的断言已按现行行为更新。没有移除迟到响应、计时器清理、未保存或权限负例。

## 未通过的既有门禁与实际边界

- `npm run tokens:check` 未通过：Canonical Token CSS 未同步。本轮未修改 `tokens.ts`、`tokens.css` 或生成脚本。
- `npm run audit:platform` 未通过：既有未引用选择器、Legacy 样式，以及 `NodeExecutionQuota.vue` 的动态类/自研 Primitive 信号。输出涉及 shell、导出 SQL 编辑器、数据源与额度组件；没有本次注册表单的违规。保留审计结果，不扩展本轮到全局样式治理。
- 构建仍有既有大 chunk 提示，未调整打包策略。

本轮为本地前端实现与合成验证；未安装或重启控制面/Agent 二进制，未运行真实 Agent 首次注册、环境检查、官方工具或用户数据库操作，不提升 G3/G4 或真实能力准入。未提交、推送或创建 PR。

## 追加：操作系统与 CPU 架构分类

按用户确认，注册与编辑把原“目标平台”拆为“操作系统”和“CPU 架构”：操作系统显示 Windows、Linux；CPU 架构按 lscpu 命名显示 x86_64、aarch64。列表复用“操作系统 · CPU 架构”，详情登记配置分别展示两项。

`nodePlatformRules.ts` 维护唯一组合映射；`useNodeForm.ts` 的两个 computed 投影唯一 `form.platform`，没有第二份可编辑配置。Windows 拒绝 aarch64，Linux aarch64 切换为 Windows 时收敛到 x86_64；读取、保存、授权失效和卸载时分类写入口保持阻断。路径输入不随分类切换转换。DTO、安装包目标和机器协议仍使用原三个枚举，Linux 分类命名不扩大三个指定麒麟环境的正式支持边界。

| 检查 | 追加结果 |
| --- | --- |
| 节点单测 | 58/58，通过三种组合回填与原值保存、非法组合、读取/保存锁、授权失效和卸载负例 |
| 注册 Chrome 回归 | 原 7 项通过；新增两种 Linux 分类的 DTO、详情名称、Windows aarch64 禁用及 Agent 包匹配，修正定位后 2/2 通过 |
| 类型检查、相关文件 Lint、前端构建 | 通过；构建仍有既有大 chunk 提示 |
| 业务架构审计 | 正反测试 11/11，0 violations，6 个既有 REVIEW |
| 秘密扫描与差异检查 | 通过 |

浏览器初轮暴露 Ant 只读输入被选中值覆盖及两个下载入口导致定位不唯一；改用键盘展开选择器，并在“完成 Agent 关联”区域校验包链接。类型检查发现的详情显示函数导入遗漏已修复。新增组合验证没有改变 UI 或安装包匹配断言。原 7 项覆盖 320～1920px 和 200% 缩放；截图与初轮复核材料在忽略目录 `artifacts/node-platform-naming/`，最终分类回归在 `artifacts/node-platform-naming-combinations/`。

本次追加仅为本地前端分类、命名与合成验证；没有实际运行 lscpu、Agent 注册、环境检查、数据库或官方工具，也没有替换运行二进制或提升真实能力准入。
