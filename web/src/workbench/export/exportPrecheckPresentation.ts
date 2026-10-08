import type { PrecheckResult } from '@/api/browser'

// EX-I6：预检查清单按输出类型二选一——本地冻结六项或对象存储六项（含两项存储检查）。
export const fixedPrecheckChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] as const
export const storagePrecheckChecks = ['STORAGE_CONNECTIVITY', 'STORAGE_AUTH'] as const

const failedResultDetails: Record<string, string> = {
  DATABASE_CONNECTION_FAILED: 'Agent 使用任务短时凭据未能建立数据库连接。请核对数据源网络、ODP 地址和端口、租户用户名及密码后重新测试数据源。',
  OBJECT_NOT_ACCESSIBLE: '数据库连接已完成，但当前账号无法访问所选表。可能是读取权限不足，或对象不存在、不可见；请核对库表名、兼容模式和对象授权。',
  TOOL_RUNTIME_INVALID: '节点上的 Java、OB Loader/Dumper 或受控本机运行时不满足启动要求。请在执行节点页面完成环境检查并修复后重试。',
  OUTPUT_PATH_NOT_WRITABLE: '导出路径或已填写的日志路径不在节点允许根目录内，或者最近已有父目录不可写。请核对路径格式、允许根目录和文件系统权限。',
  OUTPUT_PATH_NOT_EMPTY: '导出目录已有内容。请改用空目录，或确认覆盖风险后启用“跳过导出目录是否为空的检查”。',
  OUTPUT_SPACE_INSUFFICIENT: '目标文件系统的可用空间低于本任务要求的 1 GiB。请释放空间或改用空间充足的导出路径。',
  STORAGE_ENDPOINT_UNREACHABLE: 'Agent 未能与对象存储端点建立网络连接。请核对 Endpoint、节点出站网络与端口后重试。',
  STORAGE_CREDENTIAL_REJECTED: '对象存储凭据未被存储服务接受。请核对凭据是否有效、是否具备目标 Bucket 权限。',
}

export type PrecheckCheckName = (typeof fixedPrecheckChecks)[number] | (typeof storagePrecheckChecks)[number]

export function precheckCheckLabel(check: PrecheckCheckName) {
  return {
    DATABASE_CONNECTIVITY: '数据源连接',
    OBJECT_ACCESS: '所选表可读取',
    TOOL_ENVIRONMENT: '工具环境',
    OUTPUT_PATH: '输出路径',
    OUTPUT_EMPTY: '输出目录空性',
    AVAILABLE_SPACE: '可用空间',
    STORAGE_CONNECTIVITY: '存储端点连通性',
    STORAGE_AUTH: '存储凭据有效性',
  }[check]
}

export function precheckResultLabel(result: PrecheckResult | undefined, waitingForAgent: boolean) {
  if (!result) return waitingForAgent ? '等待所选 Agent 领取' : '尚未执行'
  if (result.evidenceCode === 'OBJECT_ACCESS_UNAVAILABLE') return '未完成'
  if (result.evidenceCode === 'DATABASE_CONNECTION_UNAVAILABLE') return '未完成'
  if (result.evidenceCode === 'STORAGE_CONNECTIVITY_UNAVAILABLE') return '未完成（探测未授权）'
  if (result.evidenceCode === 'STORAGE_AUTH_UNAVAILABLE') return '未完成（探测未授权）'
  if (result.evidenceCode === 'OUTPUT_EMPTY_CHECK_SKIPPED') return '已按配置跳过'
  return { PASSED: '通过', FAILED: '未通过', UNKNOWN: '未完成' }[result.status]
}

export function precheckResultDetail(result: PrecheckResult | undefined) {
  if (!result) return ''
  if (result.status === 'FAILED') {
    return failedResultDetails[result.evidenceCode] ?? 'Agent 未能通过此项固定检查；请依据原因码核对当前草稿和执行节点配置。'
  }
  if (result.check === 'OBJECT_ACCESS' && result.status === 'PASSED') return '已确认当前账号可读取所选表的元数据，并可完成不返回业务行的读取验证。'
  if (result.evidenceCode === 'DATABASE_CONNECTION_UNAVAILABLE') return 'Agent 未取得可验证的数据库连接结果，因此不能确认数据库侧条件。'
  if (result.evidenceCode === 'OBJECT_ACCESS_UNAVAILABLE') return '对象访问检查未得到可验证结论；这不是权限不足或对象存在性的证据。请确认节点和数据库状态后重试。'
  if (result.evidenceCode === 'TOOL_RUNTIME_UNAVAILABLE') return 'Agent 暂时无法确认受控工具运行时；请确认节点在线后重新执行预检查。'
  if (result.evidenceCode === 'OUTPUT_PATH_UNAVAILABLE') return 'Agent 暂时无法确认导出路径或日志路径；请检查节点状态和文件系统后重新执行预检查。'
  if (result.evidenceCode === 'OUTPUT_SPACE_UNAVAILABLE') return 'Agent 暂时无法读取本地临时分块目录所在文件系统的可用空间；请填写 --tmp-path 或检查节点状态后重新执行预检查。'
  if (result.evidenceCode === 'OUTPUT_EMPTY_CHECK_SKIPPED') return '已启用 --skip-check-dir；仅跳过导出目录是否为空的检查，路径可写性和可用空间仍会验证。'
  if (result.evidenceCode === 'STORAGE_CONNECTIVITY_UNAVAILABLE') return '存储端点连通性探测尚未授权（真实网络探测归 EX-V1 排期）；结果保持未完成，任务不能据此提交。'
  if (result.evidenceCode === 'STORAGE_AUTH_UNAVAILABLE') return '存储凭据有效性探测尚未授权（真实凭据探测归 EX-V1 排期）；结果保持未完成，任务不能据此提交。'
  if (result.check === 'STORAGE_CONNECTIVITY' && result.status === 'PASSED') return 'Agent 已与对象存储端点建立受控网络连接；这不等同于存储服务或凭据可用。'
  if (result.check === 'STORAGE_AUTH' && result.status === 'PASSED') return '对象存储凭据已被存储服务接受；这不等同于目标 Bucket 可写。'
  return ''
}

export function precheckResultBlocksSubmission(result: PrecheckResult | undefined) {
  return Boolean(result && result.status !== 'PASSED')
}
