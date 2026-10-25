# 导出与普通导入任务进度 · 2026-10-19

用户确认按真实工具证据展示任务进度，并要求从中断处完成。本轮完成 Agent 采集、控制面摘要、任务列表及详情的源码与隔离合成验证；没有安装现役服务、升级运行库或新增真实导入导出。工作树基于 `0c5953c`，包含其他既有未提交改动，未提交、推送或创建 PR。

## 事实口径与链路

| 环节 | 现役源码入口 | 边界 |
| --- | --- | --- |
| 固定模板解析 | `internal/toolprogress` | 只识别 4.3.5 单行 INFO 模板；固定六种阶段、0–100% 与安全整数子任务数；拒绝未知工具、格式漂移、越界和异常控制字符 |
| Agent 采集 | `internal/agentexecution/worker.go`、`internal/logstream/progress.go` | 第一层脱敏后解析固定 stdout；事实参与原批次摘要、私有可靠队列及重放；stderr、文件与不完整记录不产生数值事实 |
| 控制面接收与持久化 | `internal/logstream/batch.go`、`persistent.go`、`internal/store/execution_progress.go` | 复算正文与结构化事实，第二层脱敏后再次核验；日志 fsync 后与索引在同一短事务更新单个摘要 |
| 授权读取 | `internal/controlplane/task_progress.go`、`server.go` | 列表与执行详情共用原授权范围、`no-store` 和安全投影；不扫描历史日志，不返回正文、路径、对象名、命令或秘密 |
| 页面展示 | `web/src/api/taskProgress.ts`、`workbench/tasks/taskProgressPresentation.ts`、`TaskProgress.vue` | 严格校验三种证据组合；列表与详情共用纯展示规则和 Ant Progress，不新增读取或订阅所有者 |

百分比描述工具数据子任务，保留最多两位小数，不能解释为行数、字节数、表数或数据库写入结果。0% 正常展示；只有阶段时显示阶段，无证据时显示“—”。100% 仍保留原执行状态并说明等待收尾与结果核验；成功由原终态和结果证据决定，成功行显示“已完成”，不补造历史百分比。

失败、取消、日志缺口、未知执行状态和状态核对中的已有事实标明“最后记录”。重复或旧序号不刷新值与时间；跨流、跨 epoch 不拼接；缺口、截断保留末次值及原观察时间，后续完整累计事实可恢复。终态补传只补日志，不改写末次进度。进度不参与租约、状态、修订、取消完成或结果判定。

## 模板依据与真实验证限制

上轮只读检查本机 `ob-loader-dumper-4.3.5-RELEASE.jar` 常量，取得 `Progress`、`DumpProgress` 的 `Finished Tasks` / `Running Tasks` / 两位小数 `Progress` 模板，以及工具的结构完成标记；导出另有文件合并计数模板，该计数不进入本次数值投影。该 JAR 的 SHA-256 为 `b43f88caafd2c62462cfdeff38a0fd7088b0195c5d064074f977bc0b363fec06`，本轮再次只读核对一致。

这只是固定版本模板的静态依据。上轮扫描既有本地日志未找到进度样本（`matched 0`），本轮没有为获取样本启动官方工具、解析真实凭据或操作用户数据库；实际运行中的 INFO 前缀、进度频率及两类任务完整链路仍需后续获授权验证。格式不符时继续未知，不能据合成夹具声称真实任务已展示进度。

## 协议与迁移

- `0033_execution_progress.sql` 仅增加可空、受 `json_valid` 约束的 `task_executions.progress_json`；内部保存固定版本和来源位置，API 只返回固定安全摘要。不回填历史，不改写旧迁移，不增加日志正文表或长事务。
- 升级时先更新控制面并完成迁移，再更新 Agent。新控制面接受省略 `Progress` 的旧批次，缺省字段不改变旧 JSON 或摘要；旧 Agent 没有新事实时仍显示未知。旧控制面的严格解码不接受新字段，不能先接入新 Agent，也不能删字段或重算已经入队的批次摘要绕过拒绝。
- OpenAPI 与浏览器共同限制“未知 / 仅阶段 / 百分比”三种互斥组合，阶段不得附带数值；数值必须带完整计数，未知字段和显式 `progress: null` 均拒绝。旧服务使用省略字段，无须放宽 null 兼容。

## 本轮续作复核

- 收紧模板空白，拒绝 CR、FF、VT 等控制字符；保留官方时长文本的既有识别边界，不凭合成样本猜测额外格式。
- 补充正文与结构化值篡改、重复/冲突、跨流/epoch/工具、截断、第二层秘密拦截、取消后尾传、无效摘要降级及核对状态负例。
- 补全 Progress 的 `aria-valuemin`、`aria-valuemax`、`aria-valuenow` 和业务口径说明；键盘聚焦可读 Tooltip，Escape 关闭并保留焦点。运行中 100% 使用 active，取消后 100% 使用 normal，均不标为成功。
- 未知执行状态的进度统一标末次记录，加载时尚无 state 不误判；复用唯一状态所有者，并以自有属性判定拒绝将 `constructor`、`__proto__` 等原型成员识别为业务状态。
- 窄屏截图发现现有列表的操作列会被局部横滚区域裁掉，归为 implementation debt；仅将现有操作列固定右侧，沿用 Ant 表格与原列模型，不修改全局 Token、Shell 或 Archetype。
- 使用真实 `0001–0032` 迁移建立临时数据库，插入合成历史执行后升级 `0033` 并幂等重跑；验证历史进度仍为 NULL、非法 JSON 被拒绝，摘要写入不改变状态、修订、结果和时间。

## 实际检查与证据

- `go test ./internal/toolprogress ./internal/logstream ./internal/agentexecution ./internal/store ./internal/controlplane` 及这些包的 `go vet` 通过；最后补充的 Store 摘要负例另以 `-count=1` 通过。
- `go test ./contracts/... ./internal/migrate/... -count=1` 通过，包含固定六字段/六阶段、精度、安全计数、证据互斥与实际 `0032 → 0033` 历史升级。
- `npm run test`：45 文件、545 项通过；末次状态边界修改后，进度/API/列表展示的 14 项定向单测通过。全量 `npm run lint`、`npm run tokens:check` 及末次定向 ESLint、`npm run build`（含类型检查）通过。生产构建仍有既有大包体警告。
- `audit:business` 与 `audit:platform` 均为 0 硬违规，分别保留 13 / 47 个 REVIEW。进度的 API 校验、纯展示和原会话事实各有唯一所有者，无新跨层业务状态或全局视觉规则。
- 最终 6 项进度 Chrome 用例通过，覆盖 1920×1080、1440×1024、1280×720 的导出列表/详情及普通导入详情、0%/100%、阶段/未知、末次值、Tooltip 键盘操作、列表/详情授权撤销清空，以及未知/原型成员状态。断言操作按钮初始完整可见、单元格在表格边界内且不遮挡进度。
- 操作列固定后，7 项列表回归通过；固定前的列表/详情 18 项也已通过。最后的状态边界修正由上述 14 项单测及 6 项进度浏览器复验覆盖，没有重复扩大完整业务回归。
- `pwsh -NoProfile -File scripts/check-secrets.ps1`、相关 Go 文件 `gofmt -l` 及 `git diff --check` 通过。

本轮截图位于 `artifacts/task-progress-2026-10-19/`，三档子目录含 `task-progress-list-*`、`task-progress-detail-*` 和 `task-progress-import-detail-*`，代表截图已实际打开目视复核。日志位于 `C:/Users/Dionysus/AppData/Local/Temp/ob-progress-resume-20261019/`；上轮模板检查脚本及中间/末次浏览器证据保留在 `C:/Users/Dionysus/AppData/Local/Temp/ob-progress-20261019/`，中间失败未计为通过。

浏览器使用仓库 Playwright 与已安装 Chrome `156.0.8078.36`，服务为无 API 代理的隔离 Vite，业务数据全部合成。最初受继承代理和临时 WebSocket 配置影响的启动失败已在测试进程范围内修正，不更改项目依赖、用户代理或现役服务。进度用例严格监听页面错误和控制台 error，通过时均为空；既有列表/详情回归仍有 Vite/Ant `ResizeObserver loop completed with undelivered notifications` 诊断，不能据此声称开发服务零诊断。未在本轮重新执行原生 200% 缩放验证。

末次隔离服务已退出，15179 监听已关闭；两个一次性测试配置和独立 Vite 缓存已删除。保留最终截图、测试结果与复核日志，未改写现役进程、身份或状态目录。

本轮仅源码和合成验证完成；现役控制面/Agent、现役 SQLite 与既有真实能力门禁没有升级。EX-V1、普通导入真实验证矩阵、`load.ckpt`、旁路导入、麒麟原生认证及 G3/G4 仍以任务地图和各自证据为准。
