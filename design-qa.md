# Export Wizard 设计 QA

**Findings**

- [P1] 无法完成高保真像素级对照
  Location: E01–E06 全流程。
  Evidence: 设计真值为 `docs/02-design/assets/high-fidelity/export-v1-wizard.html`，但内置浏览器的安全策略拒绝捕获该本地 `file://` 设计资产；实现页 `http://127.0.0.1:5173/exports/new` 可正常渲染和交互，但本次会话的浏览器截图请求超时，未生成可持久引用的实现截图。
  Impact: 无法把同一视口、同一状态的设计图和实现截图放入同一对照输入，不能声称通过高保真视觉验收。
  Fix: 提供可由浏览器捕获的设计截图或 Figma 节点后，以 E01 默认态、E02 选择态、E03 筛选态、E04 格式态、E05 输出态和 E06 命令态在同一 Desktop 视口重新对照。

**Open Questions**

- 设计资产的本地文件访问限制是本次 QA 唯一阻塞项；不应为绕过它启动另一个浏览器表面或把设计文件临时托管为新服务。

**Implementation Checklist**

- [x] E01 使用数据源选择表，保留已授权和基础测试成功的真实门禁。
- [x] E02、E04 使用统一三列选择面板；新建格式仅展示 CSV、CUT、SQL。
- [x] E03 聚合对象、查询、条件、分区、闪回与一致性筛选，并保持现有互斥校验。
- [x] E05 保持节点、本地输出路径与能力门控；不添加未实现的对象存储提交能力。
- [x] E06 保持控制面生成的脱敏命令、预检查与提交路径，并提供复制操作。
- [x] 浏览器 DOM 验证 E01–E06 的标题、核心控件、稳定步骤条、摘要区及固定操作条。

**Follow-up Polish**

- 在可对照的设计截图可用后，再检查字体、行高、间距、颜色令牌、图标和拷贝的精确差异。

## Comparison metadata

- Source visual truth path: `docs/02-design/assets/high-fidelity/export-v1-wizard.html`
- Implementation URL: `http://127.0.0.1:5173/exports/new`
- Implementation screenshot path: unavailable (浏览器截图超时)
- Source screenshot path: unavailable (本地 `file://` 设计资产被浏览器安全策略阻止)
- Viewport and density normalization: unavailable; 未形成可比较的同视口图像
- State exercised: E01 默认态与数据源选择后流转；E02 内容选择；E03 对象与筛选；E04 格式；E05 输出与节点；E06 预检查与最终命令。
- Full-view comparison evidence: unavailable; blocked before comparison input can be created.
- Focused-region comparison evidence: unavailable; blocked before comparison input can be created.
- Primary interactions verified: 数据源选择、E01 → E02 → E03 正常流转；E03 筛选控件存在；E04 三种格式存在；E05 节点和路径控件存在；E06 预检查与最终命令区域存在。
- Console errors checked: not available through当前浏览器会话。

**final result: blocked**
