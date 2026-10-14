# 任务中心重构阶段 1（列表）验证记录

> 日期：2026-10-14；依据：[DEC-050](../../01-product/decisions.md#dec-050-任务中心重构范围与收窄规则)、[任务中心 P1](../../../design-system/pages/task-center.md)。
> 范围：`/tasks` 列表页及其后端读取接口。任务详情、草稿列表、日志保留清理不在本阶段。

## 改动

| 层 | 内容 |
| --- | --- |
| 迁移 | `0027_task_display_facts.sql`：不可改写、不可删除的展示事实表，提交与检查点继续同事务冻结数据源名称、环境、节点名称；旧任务按迁移时当前名称回填 |
| 仓储 | `internal/store/task_list.go`：列表、计数、候选共用 `buildTaskListPredicate`，授权谓词在前，筛选只收窄；关键字转义 LIKE 通配符；时间用 `julianday` 比较；`Operable` 独立于读取范围 |
| HTTP | `internal/controlplane/task_list_filters.go`：筛选解析失败关闭、创建人筛选授权、游标绑定全部条件、展示名派生、逐行资格投影、`GET /api/v1/tasks/filter-options` |
| 契约 | `contracts/openapi.json`：列表参数与字段、候选接口与结构；API/SQLite 契约同步 |
| 前端 | `useTaskList`、`useTaskListActions`、`taskListQuery`、`TaskListFilters.vue`、`taskStatePresentation`、`taskDerivationRoutes`；`TaskCenterView.vue` 只装配 |

## 实际检查

| 检查 | 结果 |
| --- | --- |
| `go test ./cmd/... ./contracts/... ./internal/... ./migrations/...` | 通过 |
| `go vet` 同范围、`gofmt -l` | 通过，无输出 |
| 新增 Go 用例 | 展示事实冻结/改名不覆盖/删除后保留并标注/不可改写；14 种筛选及计数一致（含通配符字面匹配、越权创建人、他人关键字不可发现）；全局只读无写资格；候选不越权；迁移回填与外键完整；参数解析 11 种失败关闭；创建人筛选 403；条件变化使游标失效；10 种行资格；展示名派生 |
| `npx vitest run` | 32 文件 401 项通过 |
| `npm run lint`、`vue-tsc`、`npm run build` | 通过 |
| `npm run audit:business` / `audit:wizards` | 0 违规；REVIEW 信号均为既有项 |
| Playwright（Chrome，合成路由） | 任务中心、节点筛选、其余页面、反馈、任务详情共 21 项通过 |
| `scripts/check-secrets.ps1`（PowerShell 7） | 通过 |
| `git diff --check` | 通过 |

## 限制与说明

- 浏览器回归在临时隔离端口 15184 运行：15174 被另一个本机会话的 Vite 服务占用，未终止该进程；临时配置已删除。首轮批量运行中节点切换用例失败 1 次，单独运行及第二轮批量运行均通过，未能复现，判断为与共享 Vite 依赖缓存并发相关的环境波动，未改动该用例断言。
- 列表派生资格沿用现有接口：基于原配置新建与从头重新执行只对失败任务开放，P1 表中成功/已取消任务的这两项未显示，已在 P1 第 9 节登记。
- 页码在路由内返回时恢复；整页刷新回到第 1 页。
- 取消入口只在 `realExecutionEnabled` 时出现；受控取消仍只有合成验证，确认框如实说明。
- 只在隔离临时 SQLite 与合成数据上验证；未迁移现役数据库、未替换本机服务、未执行真实取消、数据库或工具操作。
