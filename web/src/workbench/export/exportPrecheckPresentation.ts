import type { PrecheckResult } from '@/api/browser'

// 当前固定清单包含静态资源资格；存储输出还要求本地辅助目录与独立写权限证据。
export const fixedPrecheckChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] as const
export const storagePrecheckChecks = ['STORAGE_CONNECTIVITY', 'STORAGE_AUTH', 'STORAGE_WRITE'] as const
const storageOutputChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'OUTPUT_PATH', 'AVAILABLE_SPACE', ...storagePrecheckChecks] as const

// 待检查和缺项结果也使用当前输出类型的固定清单，不能以已返回项冒充完整报告。
export function precheckChecksForOutput(storageOutput: boolean) {
  return storageOutput ? storageOutputChecks : fixedPrecheckChecks
}

const blockedResultDetails: Record<string, string> = {
  PRECHECK_BLOCKED_BY_TOOL: '工具环境未通过，未执行本项。请先检查所选节点的 Java 和 OB Loader/Dumper 环境。',
  PRECHECK_BLOCKED_BY_DATABASE: '数据源连接未通过，未执行对象与参数访问检查。请先修正数据源连接。',
  PRECHECK_BLOCKED_BY_STORAGE_CONNECTIVITY: '存储端点连通性未通过，未执行凭据有效性检查。请先核对 Endpoint 和节点出站网络。',
  PRECHECK_BLOCKED_BY_STORAGE_AUTH: '存储凭据有效性未通过，未执行写权限检查。请先取得存储认证检查证据。',
  PRECHECK_BLOCKED_BY_OUTPUT_PATH: '无法安全定位导出目录或临时分块目录，未执行本项。请先核对路径和节点允许根目录。',
}

const failedResultDetails: Record<string, string> = {
  DATABASE_CONNECTION_FAILED: 'Agent 使用任务短时凭据未能建立数据库连接。请核对数据源网络、ODP 地址和端口、租户用户名及密码后重新测试数据源。',
  OBJECT_NOT_ACCESSIBLE: '数据库连接已完成，但当前账号无法访问所选表。可能是读取权限不足，或对象不存在、不可见；请核对库表名、兼容模式和对象授权。',
  TOOL_RUNTIME_INVALID: '节点上的 Java、OB Loader/Dumper 或受控本机运行时不满足启动要求。请在执行节点页面完成环境检查并修复后重试。',
  NODE_RESOURCE_INSUFFICIENT: '本任务 JVM 堆与堆外开销超过节点总执行额度。请减小 JVM 内存，或选择额度足够的节点；本项不预留实时资源。',
  OBJECT_SELECTION_REJECTED: '所选对象、列或分区的存在性与访问检查未通过。请核对表权限、列名与分区名大小写；复合分区表需要指定子分区。',
  OUTPUT_PATH_NOT_WRITABLE: '导出目录、临时分块目录或已填写的日志目录不在节点允许根目录内，或者最近已有父目录不可写。请核对路径格式、允许根目录和文件系统权限。',
  OUTPUT_PATH_NOT_EMPTY: '导出目录已有内容。请改用空目录，或确认覆盖风险后启用“跳过导出目录是否为空的检查”。',
  OUTPUT_SPACE_INSUFFICIENT: '目标文件系统的可用空间低于本任务要求的 1 GiB。请释放空间或改用空间充足的导出路径。',
  STORAGE_ENDPOINT_UNREACHABLE: 'Agent 未能与对象存储端点建立网络连接。请核对 Endpoint、节点出站网络与端口后重试。',
  STORAGE_CREDENTIAL_REJECTED: '对象存储凭据未被存储服务接受。请核对凭据是否有效、是否具备目标 Bucket 权限。',
  STORAGE_WRITE_REJECTED: '未通过目标 Bucket 与对象前缀的受控写权限检查。请核对存储凭据授权后重新检查。',
}

export type PrecheckCheckName = (typeof fixedPrecheckChecks)[number] | (typeof storagePrecheckChecks)[number]

export interface PrecheckRow {
  check: PrecheckCheckName
  result: PrecheckResult | undefined
}

// 分组只派生现有结果，不持有检查状态或引入第二份清单。
export function groupPrecheckRows(rows: readonly PrecheckRow[]) {
  const groups: { key: string; title: string; checks: readonly PrecheckCheckName[] }[] = [
    { key: 'environment', title: '执行环境', checks: ['TOOL_ENVIRONMENT', 'NODE_RESOURCE'] },
    { key: 'output', title: '输出与空间', checks: ['OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] },
    { key: 'database', title: '数据库与对象', checks: ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS'] },
    { key: 'storage', title: '对象存储', checks: storagePrecheckChecks },
  ]
  return groups.map(group => ({ ...group, rows: group.checks.flatMap(check => rows.filter(row => row.check === check)) })).filter(group => group.rows.length > 0)
}

export function precheckResultBlocked(result: PrecheckResult | undefined) {
  return Boolean(result?.status === 'UNKNOWN' && blockedResultDetails[result.evidenceCode])
}

export function precheckBlockingSummary(rows: readonly PrecheckRow[]) {
  const failed = rows.filter(row => row.result?.status === 'FAILED').length
  const blocked = rows.filter(row => precheckResultBlocked(row.result)).length
  const unknown = rows.filter(row => !row.result || row.result.status === 'UNKNOWN' && !precheckResultBlocked(row.result)).length
  return [failed && `${failed} 项未通过`, blocked && `${blocked} 项因前置条件未执行`, unknown && `${unknown} 项无法确认`].filter(Boolean).join('，')
}

export function precheckFixAction(row: PrecheckRow) {
  if (row.result?.evidenceCode === 'PRECHECK_BLOCKED_BY_TOOL') return { step: 2, label: '检查执行节点' }
  if (row.result?.evidenceCode === 'PRECHECK_BLOCKED_BY_DATABASE') return { step: 1, label: '检查数据源' }
  if (row.result?.evidenceCode === 'PARAMETER_REQUIREMENTS_REJECTED') return { step: 3, label: '修改读取与结构选项' }
  if (row.result?.evidenceCode === 'OBJECT_SELECTION_REJECTED') return { step: 2, label: '修改列与分区筛选' }
  if (row.check === 'DATABASE_CONNECTIVITY') return { step: 1, label: '检查数据源' }
  if (row.check === 'OBJECT_ACCESS') return { step: 2, label: '修改导出对象' }
  if (row.check === 'TOOL_ENVIRONMENT') return { step: 2, label: '检查执行节点' }
  return { step: 4, label: '修改执行与输出' }
}

export function precheckCheckLabel(check: PrecheckCheckName, storageOutput = false, scopeKind = 'SPECIFIED', parameterChecks = false) {
  if (check === 'OUTPUT_PATH' && storageOutput) return '本地辅助目录'
  if (check === 'OBJECT_ACCESS' && scopeKind === 'QUERY_RESULT') return '结果集访问（连接级）'
  if (check === 'OBJECT_ACCESS' && scopeKind === 'ALL' && !parameterChecks) return '全部对象访问（连接级）'
  return {
    DATABASE_CONNECTIVITY: '数据源连接',
    OBJECT_ACCESS: '对象与参数访问',
    TOOL_ENVIRONMENT: '工具环境',
    NODE_RESOURCE: '节点静态资源额度',
    OUTPUT_PATH: '输出路径',
    OUTPUT_EMPTY: '输出目录空性',
    AVAILABLE_SPACE: '可用空间',
    STORAGE_CONNECTIVITY: '存储端点连通性',
    STORAGE_AUTH: '存储凭据有效性',
    STORAGE_WRITE: '存储写权限',
  }[check]
}

export function precheckResultLabel(result: PrecheckResult | undefined, waitingForAgent: boolean) {
  if (!result) return waitingForAgent ? '等待检查结果' : '未开始'
  if (precheckResultBlocked(result)) return '前置阻断，未执行'
  if (result.status === 'PASSED' && result.evidenceCode === 'OUTPUT_EMPTY_CHECK_SKIPPED') return '按配置跳过'
  return { PASSED: '通过', FAILED: '未通过', UNKNOWN: '无法确认' }[result.status]
}

export function precheckResultDetail(result: PrecheckResult | undefined, storageOutput = false, scopeKind = 'SPECIFIED') {
  if (precheckResultBlocked(result)) return blockedResultDetails[result!.evidenceCode]!
  if (result?.evidenceCode === 'PARAMETER_REQUIREMENTS_VERIFIED') return 'Agent 已复核当前参数所需的副本读取、隐藏主键或 sys 视图访问条件，并检查已指定表的列与分区筛选；仅代表本次冻结配置的检查事实。'
  if (result?.evidenceCode === 'PARAMETER_REQUIREMENTS_REJECTED') return '当前参数所需的读取或 sys 视图权限未通过；请核对账号、数据库版本与表结构，或关闭对应参数后重新预检查。'
  if (!result) return ''
  if (result.status === 'FAILED') {
    return failedResultDetails[result.evidenceCode] ?? 'Agent 未能通过此项固定检查；请依据原因码核对当前草稿和执行节点配置。'
  }
  if (result.check === 'OBJECT_ACCESS' && result.status === 'PASSED') {
    if (scopeKind === 'QUERY_RESULT') return '已确认数据库连接；预检查不执行查询 SQL，实际语法、对象和权限由导出时确认。'
    if (scopeKind === 'ALL') return '已确认数据库连接；此项按数据库级可达性投影，实际对象由导出时确认。'
    if (result.evidenceCode === 'OBJECT_SELECTION_VERIFIED') return '已核对所选表的列名与分区名，并完成固定元数据及零行业务读取验证；未执行筛选表达式。'
    return '已核对所选对象的固定元数据；表对象还进行了不返回业务行的读取验证。'
  }
  if (result.evidenceCode === 'DATABASE_CONNECTION_UNAVAILABLE') return 'Agent 未取得可验证的数据库连接结果，因此不能确认数据库侧条件。'
  if (result.evidenceCode === 'OBJECT_ACCESS_UNAVAILABLE') return '对象访问检查未得到可验证结论；这不是权限不足或对象存在性的证据。请确认节点和数据库状态后重试。'
  if (result.evidenceCode === 'TOOL_RUNTIME_UNAVAILABLE') return 'Agent 暂时无法确认受控工具运行时；请确认节点在线后重新执行预检查。'
  if (result.evidenceCode === 'OUTPUT_PATH_UNAVAILABLE') return storageOutput ? '无法确认本地临时分块目录或日志目录；请明确填写临时目录并检查节点文件系统。' : 'Agent 暂时无法确认导出路径或日志路径；请检查节点状态和文件系统后重新执行预检查。'
  if (result.evidenceCode === 'NODE_RESOURCE_UNAVAILABLE') return '未取得可验证的 JVM 静态资源结论；节点内存事实、额度政策或本次探针可能不可用。请检查节点资源信息后重试。'
  if (result.evidenceCode === 'OUTPUT_SPACE_UNAVAILABLE') return storageOutput ? 'Agent 无法确认临时分块目录所在文件系统的可用空间；请填写临时目录或检查节点文件系统后重新检查。' : 'Agent 无法确认导出目录所在文件系统的可用空间；请检查路径和节点文件系统后重新检查。'
  if (result.evidenceCode === 'OUTPUT_EMPTY_CHECK_SKIPPED') return '已启用 --skip-check-dir；仅跳过导出目录是否为空的检查，路径可写性和可用空间仍会验证。'
  if (result.evidenceCode === 'STORAGE_CONNECTIVITY_UNAVAILABLE') return '未取得存储端点连通性结论；可能是探测未启用、Endpoint 缺失或探测暂时不可用。请核对节点探测授权与存储配置。'
  if (result.evidenceCode === 'STORAGE_AUTH_UNAVAILABLE') return '未取得存储凭据有效性证据；当前默认探测器未实现云厂商签名认证，真实验证仍待接入。此项会阻断提交。'
  if (result.evidenceCode === 'STORAGE_WRITE_UNAVAILABLE') return '未取得目标 Bucket 与对象前缀的写权限证据；不能用端点连通或认证通过代替。真实验证仍待接入，此项会阻断提交。'
  if (result.evidenceCode === 'STORAGE_WRITE_CLEANUP_UNCONFIRMED') return '受控写权限探测的清理未获确认，无法判定本次检查成功。请复核探测残留后重新检查。'
  if (result.check === 'NODE_RESOURCE' && result.status === 'PASSED') return 'JVM 堆与堆外开销符合节点总执行额度；尚未预留资源，正式执行领取时仍会复核实时可用量与并发占用。'
  if (result.check === 'OUTPUT_PATH' && result.status === 'PASSED' && storageOutput) return '已核对本地临时分块目录及已填写的日志目录的安全边界与父目录可写性；此项不验证 Bucket 权限。'
  if (result.check === 'STORAGE_CONNECTIVITY' && result.status === 'PASSED') return 'Agent 已与对象存储端点建立受控网络连接；这不等同于存储服务或凭据可用。'
  if (result.check === 'STORAGE_AUTH' && result.status === 'PASSED') return '对象存储凭据已被存储服务接受；这不等同于目标 Bucket 可写。'
  if (result.check === 'STORAGE_WRITE' && result.status === 'PASSED') return '已取得目标 Bucket 与冻结对象前缀的受控写权限证据；不代表导出结果完整性或远端对象清单已验证。'
  return ''
}

export function precheckResultBlocksSubmission(result: PrecheckResult | undefined) {
  return Boolean(result && result.status !== 'PASSED')
}
