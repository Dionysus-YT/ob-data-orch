# 系统设置职责拆分验收 · 2026-10-23

按用户采纳的评估与 [DEC-062](../../01-product/decisions.md#dec-062-系统设置拆分为四个独立配置域)，将四分区复合页面替换为平台设置、调度策略、安全策略、工具与兼容四个独立页面，与权限配置、存储凭据共同组成系统管理六个子项。

本轮交付是本地导航、页面与职责边界拆分。配置当前值、来源、版本及修改接口尚未接入，不提供编辑、保存或秘密输入，不新增配置 API、表、发布流程或通用配置引擎。工具页 4.3.5 是产品目标基线，普通日志 7–365 天及缺省 30 天是已确认规则，均不冒充当前部署事实。

## 实现与版本

| 配置域 | 路由 | 唯一页面入口 |
|---|---|---|
| 平台设置 | `/settings/platform` | `PlatformSettingsView.vue` |
| 调度策略 | `/settings/scheduling` | `SchedulingPolicyView.vue` |
| 安全策略 | `/settings/security` | `SecurityPolicyView.vue` |
| 工具与兼容 | `/settings/tools` | `ToolCompatibilityView.vue` |

旧 `/settings` 保留查询与锚点后跳转平台设置；权限和凭据地址兼容。删除旧 `SystemSettingsView.vue`，移除仅被其引用的页签、侧栏和复合布局样式；四页沿用 Settings Archetype、`settings-main`、Ant Alert/Descriptions 和既有 Token。全局导航仍由 Product Shell/系统管理分组持有，四页没有共享业务表单或权限缓存。

进入定位由四页实际复用的 `usePageEntryPosition` 负责：默认回到标题，有效锚点只在当前正文内定位并避开固定顶栏；非法片段和其他区域 ID 按无锚点处理。它不改变其他业务路由的滚动行为，不持有业务配置或全局状态。

21 个相关源码、路由、测试和设计/业务规则的内容指纹为 `93abf4626bc2ae6ed395bf41cc2b8237cb45a627738ca4c5f47a3d0dfa6caec2`；详见下述 `source-manifest.json`，含未提交实现与旧页面删除记录，不能只用 HEAD 代表当前版本。此前[三项导航验收](system-management-navigation-2026-10-23.md)保持为历史版本，不追认为当前六项导航证据。

## 检查结果

环境为 Windows 本机隔离 Vite `http://127.0.0.1:15174`，只使用合成响应。Browser plugin not available，采用仓库 Playwright。Chrome `156.0.8078.48`；原生 200% 使用 Chromium `153.0.8010.12`，宽度/设备比例由 1424/1 变为 712/2。

| 检查 | 实际结果 |
|---|---|
| 初批页面与导航 | 9 项通过：四页三档、无读取/写入请求、无虚假编辑入口、六项导航、深链接刷新、键盘及移动导航 |
| 共享页面回归 | 8 项通过：权限三档抽屉/风险确认、数据源列表/编辑/未保存确认、四代表页三档和其余现役页面；原生 200% 一项首次失败，见修正说明 |
| 最终补验 | 7 项通过，0 跳过/失败/重试：四页三档、旧地址及锚点、滚动后切页与非法/越界锚点、键盘、原生 200% 四页切换及权限保存/确认 |
| 类型、Lint、构建 | `npm run typecheck`、变更 Vue/TS/测试文件 ESLint、最终 `npm run build` 通过 |
| 架构与样式 | 最终 `audit:business` 11 项规则测试、`audit:platform` 6 项规则测试通过，均 0 违规；`tokens:check` 通过 |
| 差异 | 相关文件 `git diff --check` 通过，旧复合页面已不存在且无源码引用 |

原生缩放首次失败由保留上一页滚动位置导致新页标题在视口外。补齐本页进入定位后，最终补验实测四页标题可见、无整页横向滚动，原权限保存与确认仍可达。失败批次保留在 `regression-results.json`，不将其记作全批通过；最终结果见 `final-results.json`。

## P0 八项视觉验收

| 条件 | 结论与依据 |
|---|---|
| 同类布局与对齐 | 通过；四页统一标题、单一工作面及平面字段分组，六个子项层级一致 |
| 控件尺寸、样式和状态 | 通过；沿用 Ant Alert/Descriptions、既有导航及 Token，无新基础控件 |
| 表格、表单、抽屉、弹窗 | 通过；新页无编辑表单/弹层，共享导航与样式变更已复查数据源、权限抽屉及确认框 |
| 动效与反馈 | 通过；不可编辑说明使用持久 Alert，菜单使用 Ant 动效，键盘关闭和焦点恢复实测 |
| 视觉密度 | 通过；四页内容按配置域分组，与数据源及其他参考页共享字体、间距和工作面语言，无内部卡片堆叠 |
| 1920×1080 | 通过；四页实际截图及代表截图目视核对 |
| 1440×1024、1280×720 | 通过；四页实际截图、代表截图目视核对，无整页横滚；补充移动导航与原生 200% |
| 私有样式与重复实现 | 通过；仅共享设置分组间距、分隔和锚点安全距，无新增 Token；简单页面分别拥有本域说明，进入定位有四个真实调用方 |

样式审计有 117 个 REVIEW、7 个运行时引用，未对无关信号作完成承诺；本轮设置布局选择器均有实际四页引用。业务审计有 21 个 REVIEW，其中 ProductSystemNavigation 的“0 个业务入口”由 Product Shell 实际装配解释，保持外壳归属。新的进入定位复用不持有业务状态，也不依赖具体 feature。

## 证据与限制

本机证据根目录为 `C:/Users/Dionysus/.codex/visualizations/2026/10/22/01a162e6-40a2-7d22-bbab-18e5a64f8261/settings-split/`：

- `initial/`：首批 9 项的页面三档、六项导航和移动导航截图；结果来自实际 Playwright 终端输出。
- `regression-results.json`、`regression/`：共享参考页回归及首次原生缩放失败的真实报告/截图/追踪。
- `final-results.json`、`final/`：最终 7 项补验、四页三档、原生 200% 四页及权限编辑截图。
- `platform-audit-final.txt`、`business-audit-final.txt`、`source-manifest.json`、`evidence-manifest.json`：最终审计、源码和证据指纹。

仍有既有 Vite ResizeObserver 诊断和构建大分块提示，不声明零诊断或性能优化完成。配置读写、真实生效值、分域版本/保存/确认/审计仍待各域技术接入；本轮没有安装或替换服务、修改现役数据库、读取真实配置/秘密或运行官方工具，不提升真实执行准入与上线状态。
