import { writeFile } from 'node:fs/promises'
import { mysqlExportCases } from './cases.mjs'

const operations = {
  SUCCEEDED: '创建草稿 → 校验预览/指纹 → 七项通过 → 提交 → 真实终态 → 清单与完整内容',
  API_REJECTED: '创建请求得到 400/422；预检查、提交、工具执行均为零',
  PRECHECK_FAILED: '完整七项报告 → 指定项失败 → 独立项保留 → 提交被拒绝',
  TASK_FAILED: '七项通过 → 真实启动 → 非零退出和指定 SQL 错误日志；不得错判成功',
  OBSERVED: '记录工具终态与实际数据/文件行为；不外推为推荐组合或 PASS',
}
const workflows = {
  UI: '五步正常导航、真实对象目录、保存、自动预检、确认提交、详情终态及内容',
  INVALIDATION: '保存后编辑线程；修订+1/指纹变化；旧预检绑定不匹配、旧提交拒绝；新预检通过',
  IDEMPOTENCY: '创建/预检/提交同键同请求重放得到同 ID；同键不同载荷 409；乐观锁拒绝旧修订',
  EXPIRY: '等待真实 validUntil，不改数据库/系统时间；过期提交 409/422',
  SUBMIT_REPLAY: '同提交幂等键重复请求只返回同一任务；终态、内容和源配置冻结一致',
  CANCEL_QUEUED: '观察 WAITING_SCHEDULE 且未启动；取消为 CANCELLED；重复取消回执稳定',
  CANCEL_RUNNING: '观察真实 RUNNING/进程证据；取消受控进程树；终态 CANCELLED、无待核对',
  CHECKPOINT_NEGATIVE: '真实失败且无 dump.ckpt 的任务继续请求被拒绝',
  CHECKPOINT_POSITIVE: '真实失败/检查点/原完整预检；继续任务继承快照并追加 --retry；不重复数据且父子关系正确',
  DERIVE: '真实失败任务派生；按原配置新建可编辑并重新预检；从头重跑保持指纹，改配置提交被拒绝',
  TEMPLATE: '成功任务保存→列表→改名→创建草稿→更换专用输出→新预检/执行→删除本次模板',
  LOGS: '真实运行期间 SSE；来源内序号、断线续读、REST 分页、两层脱敏；下载未实现单列阻断',
  SECRET_CLEANUP: '核对真实密码及其可恢复编码不进入 argv/响应/快照/日志；对应 execution 安全文件与原始日志清理',
  AGENT_RECOVERY: '独占本轮任务窗口；保留身份/TLS重启 Agent；记录租约、序号、重复投递和单进程结果',
  AUTHORIZATION: '未登录/缺失 CSRF/随机对象失败关闭；第二个已授权身份验证跨主体无权对象 404',
  ACCESSIBILITY: '1920/1440/1280/720 视口；键盘、焦点还原、无整页横向溢出、唯一摘要和常驻完整命令',
  FORMAT_GATE: '普通向导只提供 CSV/CUT/SQL；历史格式不作为当前新建入口，不启动门控格式',
  DRAFT_ROUNDTRIP: '页面保存读取开关与 TIME/TIMESTAMP 格式，刷新后修订及指纹保持；命令参数仍存在',
}
const cell = value => String(value).replaceAll('|', '\\|').replaceAll('\n', '<br>')
const lines = ['# MySQL 本地导出端到端测试矩阵', '', `共 ${mysqlExportCases.length} 个独立场景。用例定义由 scripts/export-e2e/cases.mjs 驱动；此文件由 matrix.mjs 生成。真实结果单独记录，矩阵存在不表示已通过。`, '', '通用前提：用户授权的非生产 test-mysql、Windows AMD64 当前 Agent/控制面、独立 owner 夹具库和允许根目录下专用输出。CSV/CUT/SQL 参数通过真实 API→唯一生成器→七项预检→Agent→官方工具→结果验证。每个场景串行，危险 SQL 负例止于草稿接口。默认日志行为单独测试，其余参数场景使用独立日志目录。', '', '| 用例 | 分类/场景 | 输入变化 | 执行与预期 | 条件 |', '| --- | --- | --- | --- | --- |']
for (const test of mysqlExportCases) {
  const patch = structuredClone(test.patch)
  if (patch.query?.length > 220) patch.query = `${patch.query.slice(0, 80)}…（${patch.query.length} 字符合法 SELECT）`
  const expected = test.workflow ? workflows[test.workflow] : operations[test.expected]
  lines.push(`| ${test.id} | ${cell(`${test.area}：${test.title}`)} | ${cell(JSON.stringify(patch))} | ${cell(expected + (test.failedCheck ? `；${test.failedCheck} 失败` : ''))} | ${cell(test.requires?.join(', ') ?? '通用前提')} |`)
}
lines.push('', '判定：PASS 必须完成该项全部断言；FAIL 是已执行且不符合预期；BLOCKED 为缺少具体前提；PARTIAL 为只有部分断言完成；OBSERVED 只记录未定版语义。阻断和部分完成不计入通过数。真实检查点、sys 正例、跨主体身份和活跃租约恢复缺失时保留明确条件，禁止合成证据顶替。', '')
await writeFile('docs/03-technical/evidence/export-mysql-e2e-cases.md', lines.join('\n'))
process.stdout.write(JSON.stringify({ matrixCases: mysqlExportCases.length }) + '\n')
