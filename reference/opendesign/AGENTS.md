# Codex 开发约束

## 目标

将已确认的 OB Data Orch 原型迁移到真实 Vue 前端。当前 Design Files 工程是预览和交付包，不是生产目标。

## 开工前必读

1. `README.md`
2. `DESIGN.md`
3. `IMPLEMENTATION.md`
4. `ACCEPTANCE.md`
5. `ob-data-orch-workbench-prototype.html`
6. `frontend-style-plan.md`（仅用于背景和开放问题）

## 事实优先级

真实 API/类型/权限契约 > 原型交互 > `DESIGN.md` 视觉规范 > 规划文档。

## 工作规则

- 如果真实仓库存在 `.codegraph/`，理解代码前先使用 `codegraph explore`。
- 开始编辑前检查目标文件 diff；保护所有已有未提交改动。
- 禁止 `git reset --hard`、`git checkout --` 或批量覆盖用户改动。
- 不在生产 Vue 中使用 `v-html` 注入原型或动态执行原型脚本。
- 复用 `browserApi()`、现有 TypeScript 类型、路由和 Workbench 组件。
- 不为页面分别复制状态摘要、进度条、刷新、空态或错误态方法。
- 不新增无后端证据的字段、指标、状态、远程日志或终端能力。
- CSS 使用 `operator-signal.css` Token 和 OKLch 派生色；不新增业务 Hex。
- 每次只迁移明确范围，完成后运行相关测试和完整质量门槛。

## 质量门槛

```powershell
cd web
npm run typecheck
npm run lint
npm run test
npm run build
```

所有 P0 验收项见 `ACCEPTANCE.md`。任何命令失败时先修复原因，不通过“跳过测试”交付。

