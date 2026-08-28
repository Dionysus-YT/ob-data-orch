# OB Data Orch 前端设计交付包

本目录同时包含高保真原型、可运行预览包装和 Codex 开发交接文档。它用于说明已确认的视觉与交互，不是 `ob-data-orch` 的生产前端副本。

## 先读这些文件

| 文件 | 用途 |
|---|---|
| `DESIGN.md` | 视觉 Token、组件、状态、响应式与可访问性规范 |
| `IMPLEMENTATION.md` | 真实路由、组件、API 和分阶段迁移映射 |
| `ACCEPTANCE.md` | Codex 开发的 P0/P1 验收门槛 |
| `AGENTS.md` | Codex 在本项目中的执行约束 |
| `ob-data-orch-workbench-prototype.html` | 当前已确认的交互与视觉实现 |
| `frontend-style-plan.md` | 设计背景、范围和仍开放的问题 |
| `assets/README.md` | 参考截图与生产素材边界 |

事实优先级：真实项目 API/类型/权限契约 > 原型交互 > `DESIGN.md` > 规划文档。

## 当前 Vue 工程的用途

当前 Vue 3 + TypeScript + Vite 工程是原型的可运行 Vue 实现。它使用真实 SFC、组合式状态和本地安全 Fixture 来复现产品壳、概览、数据源、六步导出向导、任务、执行节点和日志交互。

该工程不连接真实 API、数据库、凭据、Agent 或工具执行，只用于设计评审和本地交互验证。正式开发目标仍是现有 `ob-data-orch/web` 前端；生产迁移必须使用 Vue Router、真实 SFC、现有 `browserApi()` 和 Workbench 组件。

`src/` 中不再包含 `v-html`、原型 HTML 读取器或动态脚本注入。原始 `ob-data-orch-workbench-prototype.html` 保留为视觉和交互对照，不参与运行时渲染。

## 本地预览

```powershell
npm install
npm run dev
```

```powershell
npm run typecheck
npm run build
npm run preview
```

开发服务器默认使用 `http://localhost:5173`；预览服务器默认使用 `http://localhost:4173`。

## 开发交接前提

- 确认真实项目的目标分支和写权限。
- 先检查真实项目已有未提交改动，禁止 reset 或覆盖。
- 按 `IMPLEMENTATION.md` 分阶段迁移，并逐项完成 `ACCEPTANCE.md`。
- 原型中的数据均为安全合成状态，不连接真实数据库或执行工具。
