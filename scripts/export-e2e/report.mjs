import {readFile,writeFile} from 'node:fs/promises'
import path from 'node:path'
import {mysqlExportCases} from './cases.mjs'

// 只投影已完成验证的固定安全字段，报告不包含连接串、原始命令、路径或秘密材料。
const root=path.resolve('artifacts/mysql-export-e2e')
const json=async file=>JSON.parse((await readFile(path.join(root,file),'utf8')).replace(/^\uFEFF/,''))
const verified=await json('verified-results.json'),owned=await json('owned-executions.json')
const api=await json('api-secret-audit.json'),native=await json('native-log-secret-audit.json')
const cleanup=await json('security-cleanup.json'),database=await json('fixture-cleanup.json'),output=await json('output-cleanup.json')
const runtime=await json('runtime-final.json'),binaries=await json('runtime-binaries.json'),coverage=await json('command-coverage.json')
const argv=await json('live-argv-audit.json')
if(verified.cases.length!==mysqlExportCases.length||mysqlExportCases.some(test=>!verified.cases.some(row=>row.id===test.id)))throw new Error('REPORT_CASES_INCOMPLETE')
if(owned.some(row=>!['SUCCEEDED','FAILED','CANCELLED'].includes(row.state)||row.reconciliationRequired))throw new Error('REPORT_UNFINISHED_TASKS')
if(!database.databaseAbsent||!output.directoryAbsent||runtime.ownedTemplatesRemaining!==0)throw new Error('REPORT_CLEANUP_INCOMPLETE')
if([api,native,argv].some(proof=>proof.status!=='AUDITED'||proof.plaintextOrRecoverableEncodingPresent!==false)||api.tasks!==owned.length||cleanup.executions!==owned.length||cleanup.securityRemaining!==0||cleanup.rawLogRemaining!==0)throw new Error('REPORT_SECRET_FACTS_INCOMPLETE')
if(runtime.healthStatus!==200||!runtime.acceptsNewTasks||runtime.environmentCheckStatus!=='PASSED'||runtime.activeExecutions!==0||binaries.liveAgentProcesses!==1||binaries.liveControlPlaneProcesses!==1||!binaries.bundleMatchesAgent)throw new Error('REPORT_RUNTIME_NOT_RESTORED')
if(coverage.records.some(row=>row.code||row.missing.length||!row.passwordPlaceholder||row.redaction!=='PASSWORD_ONLY'))throw new Error('REPORT_COMMAND_EVIDENCE_INCOMPLETE')
const counts=verified.cases.reduce((all,row)=>(all[row.outcome]=(all[row.outcome]??0)+1,all),{})
const contentCases=verified.cases.filter(row=>row.outcome==='PASS'&&row.contentVerified).length
const states=owned.reduce((all,row)=>(all[row.state]=(all[row.state]??0)+1,all),{})
const safeRows=mysqlExportCases.map(test=>{
  const row=verified.cases.find(value=>value.id===test.id)
  return {
    id:test.id,title:test.title,outcome:row.outcome,
    ...Object.fromEntries(['code','httpStatus','submitHttpStatus','submitStatus','taskId','state','errorCode','exitCode','rows','fileCount','bytes','dataFileCount','contentVerified','contentHash','tablesVerified','independentRunsCompared','sseEvents','reconnectedEvents','restRecords','restPages','tasksAudited','nativeLogsAudited','securityRemaining','rawLogRemaining'].filter(key=>row[key]!==undefined).map(key=>[key,row[key]])),
    ...(row.eventualExecution?{eventualExecution:row.eventualExecution}:{}),
  }
})
const sealed={date:'2026-10-14',scope:'AUTHORIZED_NONPRODUCTION_MYSQL_LOCAL_WINDOWS_AMD64',counts,caseCount:verified.cases.length,contentCases,ownedTasks:owned.length,taskStates:states,commandEvidenceTasks:coverage.records.length,apiSecretAudit:api,nativeSecretAudit:native,securityCleanup:{executions:cleanup.executions,securityRemaining:cleanup.securityRemaining,rawLogRemaining:cleanup.rawLogRemaining},fixtureCleanup:database,outputCleanup:output,runtime,binaries,cases:safeRows}
await writeFile('docs/03-technical/evidence/export-mysql-e2e-2026-10-14-results.json',JSON.stringify(sealed,null,2)+'\n')
const cell=value=>String(value).replaceAll('|','\\|').replaceAll('\n','<br>')
const lines=[
  '# MySQL 本地导出真实端到端验证（2026-10-14）','',
  `本轮 ${verified.cases.length} 项：${counts.PASS} PASS、${counts.FAIL} FAIL、${counts.BLOCKED} BLOCKED、${counts.PARTIAL} PARTIAL、${counts.OBSERVED} OBSERVED。全部场景已有明确结果，但导出模块整体不能声明全部通过。${contentCases} 个通过项实际核对完整行集或 DDL 内容；通过总数还包含接口拒绝、预检查、生命周期和界面断言。`,
  '',
  '## 授权、环境与证据边界','',
  '用户明确授权非生产 `test-mysql`、创建并清理专用夹具、本地导出及正常浏览器登录。实际源为 OceanBase 4.4.2.3 / MySQL 兼容模式 / ODP，使用 Windows AMD64、Java 8 和 OBDUMPER 4.3.5-RELEASE。只读取本轮固定合成表，不读取业务表；测试辅助程序不进入产品 JDBC 或 SQL 接口。Oracle、对象存储、历史 POS/Parquet/ORC/Avro 实际导出、Linux 及真实并发压测不在本轮。',
  '',
  `共创建 ${owned.length} 个真实任务（含修复前尝试及新目录复验）：${states.SUCCEEDED} SUCCEEDED、${states.FAILED} FAILED、${states.CANCELLED} CANCELLED；结束时无未完成任务或待核对 execution。这是任务次数，不是用例通过数。${coverage.records.length} 个代表性冻结命令已核对参数名，无预期参数遗漏；79 个目录定义的范围见[参数追踪](export-mysql-parameter-coverage.md)，不能把目录数量当作全部参数真实可用。`,
  '',
  '浏览器按用户指定使用内嵌会话，通过正常 API 执行草稿、唯一生成器、七项预检查、提交及 Agent。页面实际走完五步、真实对象目录、确认提交及成功详情。CSV/CUT/SQL 完整内容由独立校验器核对 NULL、空串、引用、Unicode、换行、重复行、列、字节和清单；压缩先解压再核对。多表和整库逐表校验，DDL 多对象逐类型核对，不能用单张 rich 表或退出码替代整库成功。',
  '',
  '## 仍失败的 12 项','',
  '| 场景 | 真实结果与影响 |','| --- | --- |',
  '| MX-001 默认日志目录 | 七项预检通过；工具先在输出目录建立 logs，随后因输出目录非空退出 1。其余场景使用独立日志目录，以继续验证各参数；默认配置缺陷仍存在。 |',
  '| MX-004 整库 | 原尝试超时；最终 FAILED / PROCESS_WAIT_FAILED / exit 1，12 个文件、27,541,197 B、无检查点。部分文件存在不能证明整库完成。 |',
  '| MX-005 整库排除 | FAILED / PROCESS_WAIT_FAILED；直接进程退出码为 0，10 个文件、1,065,487 B。完整成功证据未成立，根因尚未定位；不把此例写成非零退出或整库成功。 |',
  '| MX-037-CUT、MX-037-SQL 空表保留 | 退出 0 但结果文件数为 0；Agent 按 RESULT_VERIFICATION_FAILED 失败关闭。CSV 空集合的真实空结果文件已独立验证。 |',
  '| MX-062、MX-062-CUT 零日期保留 | 源端 CAST AS CHAR 确认零日期确实存在，参数已发射，文件值却为 NULL 标记；任务终态成功但内容断言失败。 |',
  '| MX-075 retain-schema | 工具终态成功，实际 DDL 缺少期望的 Schema 限定；内容断言失败。 |',
  '| MX-110-zstd、MX-150-zstd-1、MX-150-zstd-22 | FAILED / PROCESS_WAIT_FAILED / exit 1，压缩结果为零字节，根因尚未定位；不能声明 zstd 或其等级边界可用。 |',
  '| MX-110-snappy | exit 1；日志明确出现 UnsatisfiedLinkError，java.library.path 中缺少 snappyjava。当前包和节点组合不满足 native 依赖。 |',
  '',
  '## 阻断、部分验证与行为观察','',
  '- MX-072：该源固定隐藏主键列零行读取返回 1054，没有可读夹具，隐藏主键加速正例 BLOCKED；其资格负例不能替代正例。',
  '- MX-073：源未配置 sys 凭据，附加定义信息正例 BLOCKED；未配置 sys 的拒绝负例已通过。',
  '- MX-128：本轮真实失败任务均缺少有效 dump.ckpt，检查点继续正例 BLOCKED；无检查点继续的 422 负例已通过。',
  '- MX-133：当前真实 Worker 未提供活跃进程重新附着入口，Agent 崩溃后的监管、租约与重复投递取证 BLOCKED；本轮安装时的闲置重启不算恢复正例。',
  '- MX-131：真实 SSE 首次 5 条、断线续读 5 条且无重复，终态 SUCCEEDED；REST 78 条、1 页、PERSISTED_DOUBLE_REDACTED。日志下载已按 DEC-050 推迟，未观察到真实多页日志，整体 PARTIAL。',
  '- MX-134：未登录、缺少 CSRF 和随机不存在对象检查已失败关闭；缺第二个已授权身份，跨主体验证 PARTIAL。',
  '- MX-031：实际 trail-delimiter 去掉尾部分隔符，完整行内容核对后保留 OBSERVED；不据参数名字推断方向。MX-063：未开启 preserve-zero-datetime 时的零日期转换只记录 OBSERVED。',
  '',
  'TIME/TIMESTAMP、DATE/DATETIME 与 PRESERVE/RESTART 序列策略已有真实文件断言。序列先实际推进，PRESERVE 导出下一值 29，RESTART 导出 17。SCN 正例通过 API 按精确 int64 传输；浏览器数值解析只接受安全整数，不能从 API 结果外推完整 int64 SCN 的页面输入/恢复可用。',
  '',
  '## 测试中修复并复验的实现','',
  '1. 多行及超过 4096 字节的合法 SELECT 在草稿、领取、信封与启动间采用统一有界校验；原始 SELECT 60 KiB、包装后 64 KiB，其余参数保持既有限制和失败关闭。真实多行/长查询已通过。',
  '2. 空文本结果以真实 .csv/.dat/.sql 文件、受控进程和固定成功终态共同判断；零字节实际文件允许通过，零文件、仅 MANIFEST/检查点、DDL 与二进制反例继续拒绝。真实 CSV 空集合已通过。',
  '3. CSV NONE 引用模式必须显式提供转义字符；缺失组合在接口和页面拒绝，合法组合真实通过。',
  '4. 重复列筛选在服务端拒绝，防止预检查之后才由工具报错。',
  '5. PENDING 七项占位补齐 NODE_RESOURCE 固定 evidenceCode，严格浏览器解码不再中断自动预检查；页面完整自动报告已复验。',
  '6. 草稿读取 API 解码补齐读取开关、日期/NLS、压缩等级及 DDL/序列字段，避免 GET→回填→PATCH 静默清值；真实页面刷新后 rev-3、指纹及 weakRead/TIME/TIMESTAMP 参数保持。Oracle 与序列读取有合成正反例，不外推 Oracle 真实验证。',
  '',
  '固定夹具与脚本准备错误单独纠正后使用新目录复验：DDL_ONLY 不携带数据资源参数、默认 CSV 单引号、CRLF 字面转义、NONE 显式 escape、临时目录现行绑定，以及生命周期旧修订、正确 SSE 路径和派生空目录。max-file-size 按总量停止契约断言，32 个完整源行、33,225 B 对应 32,768 B 上限；block-size 独立断言拆分。历史检查保留原结论，但变更后的绑定不匹配、旧提交为 422；新检查通过。旧任务节点事实变化与本轮派生语义分开，用当前事实的真实失败种子验证冻结指纹。',
  '',
  '## 实际检查、秘密与清理','',
  '- Go 全包 go test 与 go vet 通过；前端针对草稿 API/DTO/向导的 152 项测试、typecheck 与改动文件 ESLint 通过。',
  '- E2E 校验器 10 项测试通过，含缺表、少行、未知对象及多对象 DDL 反例；脚本语法检查通过。',
  '- audit:business 为 0 硬违规、9 REVIEW；audit:wizards 为 0 硬违规、4 REVIEW。API 只拥有传输与严格解码，表单/目录/事务保持原唯一所有者；现有跨模块节点标签与模板资格复用保持既有边界，未按行数另建框架。',
  '- Secret scan 与 git diff --check 通过（换行转换提示不算检查失败）。',
  `- ${api.tasks} 个任务的公共快照、脱敏命令与持久日志，以及 ${native.files} 个本轮 native 日志文件，均未匹配真实密码明文、Base64 或 URL 编码；另现场核对 1 个运行中的 Java argv。原始响应只进入匿名 stdin 比对，不写入报告。`,
  `- ${cleanup.executions} 个 execution 的 security 与 tool-raw-log 均无残留。本次模板已删除，专用库按 owner 核验后 DROP 并确认不存在，专用输出目录按绝对路径和 owner 核验后删除；保留平台任务历史及脱敏断言/摘要。`,
  '- 清理首条混合脚本被策略拒绝后，拆分为只读路径核验及无 Force 的精确目录删除，两步成功；无越界清理。',
  `- 本机控制面健康 ${runtime.healthStatus}、Agent 心跳恢复、节点 ENABLED / NORMAL、环境检查 PASSED，活动 execution 为 ${runtime.activeExecutions}。当前构建 Agent 与内嵌 Windows Agent 包摘要一致；Java/工具及身份/TLS/状态目录继续复用。`,
  '',
  `Agent SHA-256：\`${binaries.agentHash}\`。控制面 SHA-256：\`${binaries.controlPlaneHash}\`。本机源码已构建并安装运行，不等于正式发布或其他目标认证。`,
  '',
  '## 逐项结果','',
  '用例输入及执行断言见[172 项矩阵](export-mysql-e2e-cases.md)，可机读的安全结果见[结果 JSON](export-mysql-e2e-2026-10-14-results.json)。完整回执、文件统计与截图保留在忽略的 artifacts/mysql-export-e2e；真实输出已清理，不能再次对旧目录运行内容校验，复验须创建新的 owner 夹具和运行目录。','',
  '| 用例 | 场景 | 结果 | 实际断言/限制 |','| --- | --- | --- | --- |',
]
for(const row of safeRows){
  const facts=[row.code,row.errorCode,row.httpStatus?`HTTP ${row.httpStatus}`:null,row.submitHttpStatus?`submit ${row.submitHttpStatus}`:null,row.submitStatus?`submit ${row.submitStatus}`:null,row.rows!==undefined?`${row.rows} 行`:null,row.fileCount!==undefined?`${row.fileCount} 文件 / ${row.bytes??0} B`:null,row.tablesVerified?`${row.tablesVerified} 表`:null,row.independentRunsCompared?`${row.independentRunsCompared} 次独立命令/内容一致`:null].filter(Boolean)
  lines.push(`| ${row.id} | ${cell(row.title)} | ${row.outcome} | ${cell(facts.join('；')||'该项完整断言见矩阵与结果 JSON')} |`)
}
lines.push('','本轮仅认证上表已经通过的具体组合；失败、阻断、部分验证与行为观察不会因页面可选或参数已发射而提升为已支持。','')
await writeFile('docs/03-technical/evidence/export-mysql-e2e-2026-10-14.md',lines.join('\n'))
process.stdout.write(JSON.stringify({caseCount:verified.cases.length,counts,contentCases,cleanupConfirmed:true})+'\n')
