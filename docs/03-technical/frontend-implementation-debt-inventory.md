# Frontend Implementation Debt Inventory

> 状态：**Inventory only · 2026-09-02**
>
> 本清单不授权删除、重写或改变现有前端行为。它记录 P4 实现与 [P0 MASTER](../../design-system/MASTER.md) 的待核对项；处理顺序必须是引用分析 → 替代实现 → `vue-tsc` → build → 页面验证 → 删除 dead code。

| 区域 | 当前 P4 事实 | 与 P0 的待核对项 | 后续处理门槛 |
|---|---|---|---|
| `web/src/style.css` | 同时存在 `--color-*`、`--surface-*`、旧 34px control / 52px table row 和大量 raw hex | Token 命名、候选值及密度与 P0 36px control / 40px dense row 未统一；旧样式含 card-heavy 与页面级例外 | 逐页替代后确认无调用，再删旧选择器 |
| `ProductShell.vue` | 已有分组导航和 216px/48px 基础结构 | Header 目前缺少 P0 所述 breadcrumb/global context；1280 collapse 尚未形成 | Shell 变更需 DCR 与 1920/1440/1280 验证 |
| `WizardFrame.vue` | 所有 Builder 共用横向六步 Stepper、固定 summary 与 Footer | 与 P0 的 compact vertical Task Rail、conditional Inspector、步骤内容可变骨架冲突 | 三类 Builder 引用与状态路径均验证后替换 |
| `WorkbenchTable.vue` 与旧 `.table-card` | Table wrapper 与遗留表格 CSS 并行 | 尚未收敛为单一 dense table surface；旧 52px row、card container 与列策略需逐页核对 | 先迁移 Data Sources 并完成键盘/窄屏验证 |
| `WorkbenchButton.vue` / `WorkbenchIconButton.vue` | 组件 token 与 raw hex/30px icon button 并存 | Focus、disabled reason、尺寸和颜色 token 尚未完全以 P0 收敛 | 组件测试与所有调用点验证后统一 |
| `DataSourceListView.vue` / `DataSourceFormView.vue` / `DataSourceEditDrawer.vue` | 现有列表含 fixture runtime 列、客户端全量过滤与 `1 / N` 分页；新建时可将保存与测试合并为一个主动作，且保留独立页侧栏布局 | 已由 P1 [数据源页面规范](../../design-system/pages/data-sources.md) 收敛为表格 + Drawer、服务端 cursor、独立 Save/Test 状态机和分离的生命周期/测试域；P3 cursor 响应字段仍待澄清 | 在 P3 分页契约澄清后逐项替换；真实 API、DEV fixture、权限/错误/键盘及 1920/1440/1280 验证均通过后再删死代码 |
| `TaskDetailView.vue` 与 detail CSS | 以多张 `.content-card` 分段呈现技术事实 | 与 P0 的 Fact → Reason → Evidence divider-led hierarchy 不一致 | Task Detail Hi-Fi 实施及日志/命令/状态回归后处理 |
| 各 View scoped CSS | 多处定义 layout、row height、drawer/table/form 样式 | 存在重复 token、局部 breakpoint 与历史布局骨架 | 每个 Archetype 精修时建立引用清单并逐项移除 |

该清单不改变任何 P3 业务规则，也不表示这些组件无用或可立即删除。
