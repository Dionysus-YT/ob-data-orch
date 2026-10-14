import type { ExecutionNodeDetail, NodeInspectionItem, NodeRuntimeInspection, NodeActivityItem } from '@/api/browser'

export const inspectionLabels: Record<NodeInspectionItem['kind'], string> = { JAVA_RUNTIME: 'Java 环境', OBDUMPER_RUNTIME: '导出工具', OBLOADER_RUNTIME: '导入工具', CONNECTOR: '数据库连接组件', WORKSPACE: '工作目录', LOG_QUEUE: '日志缓存目录', TOOL_LOG: '工具日志目录', DIRECT_LOAD_CONFIG: '旁路导入配置', RESOURCES: '系统资源', RUNTIME_LAYOUT: '工具安装完整性' }
export const inspectionStatusLabels: Record<NodeInspectionItem['status'], string> = { PASSED: '通过', FAILED: '未通过', UNKNOWN: '未确认', NOT_APPLICABLE: '不适用' }

// 原因和建议只解释固定检查事实，不回显受控路径、原始结果码或进程错误，也不承诺重试一定通过。
export function inspectionItemGuidance(item: NodeInspectionItem): { reason: string; action: string } | undefined {
  if (item.status === 'PASSED') return undefined
  if (item.status === 'NOT_APPLICABLE' && !item.required) return { reason: '当前未配置这一可选项。', action: '无需处理。' }
  const name = inspectionLabels[item.kind]
  switch (item.code) {
    case 'MISSING':
      return { reason: `${name}所需的文件或目录不存在。`, action: item.kind === 'JAVA_RUNTIME' ? '在「编辑节点」中核对 Java 位置，确认已安装 Java 8 后重新检查。' : item.kind === 'OBDUMPER_RUNTIME' || item.kind === 'OBLOADER_RUNTIME' ? '核对「工具安装目录」是否指向完整的安装根目录，而非其中的脚本文件夹。' : item.kind === 'CONNECTOR' ? '请按安装说明补齐项目支持的数据库连接组件后重新检查。' : '确认 Agent 所需目录已创建且运行账号可访问后重新检查。' }
    case 'NOT_READABLE':
      return { reason: `Agent 无法读取${name}所需的文件或目录。`, action: '请检查 Agent 运行账号的读取权限，再重新检查。' }
    case 'NOT_WRITABLE':
      return { reason: `Agent 无法在${name}创建或清理临时文件。`, action: '请检查 Agent 运行账号的写入和删除权限，再重新检查。' }
    case 'NOT_EXECUTABLE':
      return { reason: `${name}的启动文件不可执行。`, action: '请核对安装文件完整性及 Agent 运行账号的执行权限，再重新检查。' }
    case 'VERSION_UNSUPPORTED': {
      if (!['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME'].includes(item.kind)) return { reason: '这项组件的版本没有通过校验。', action: '请按安装说明核对组件版本，再重新检查。' }
      const expected = item.kind === 'JAVA_RUNTIME' ? 'Java 8' : '4.3.5'
      return { reason: `${item.version ? `检测到版本 ${item.version}` : '检测到不支持的版本'}，当前要求 ${expected}。`, action: item.kind === 'JAVA_RUNTIME' ? '在「编辑节点」中选择 Java 8 的程序位置，再重新检查。' : '使用完整的官方 4.3.5 工具包，更新「工具安装目录」后重新检查。' }
    }
    case 'VERSION_PROBE_NOT_REQUESTED':
      return { reason: '本次没有执行版本检查，尚不能确认实际版本。', action: '点击「检查节点环境」；若刚检查过，请确认控制面与 Agent 已同步更新后重试。' }
    case 'VERSION_PROBE_TIMEOUT':
      return { reason: '版本查询超过等待时间，尚未取得结果。', action: '确认 Agent 与 Java 可正常运行后重新检查；若反复超时，请检查 Agent 运行日志。' }
    case 'VERSION_OUTPUT_INVALID':
      return { reason: '版本查询返回的信息中没有可确认的唯一版本号。', action: item.kind === 'JAVA_RUNTIME' ? '请核对 Java 8 安装是否完整，再重新检查。' : '请核对是否使用完整的官方 4.3.5 工具包，再重新检查。' }
    case 'VERSION_UNAVAILABLE':
      return { reason: '版本查询未能完成，尚不能确认实际版本。', action: '确认 Agent 在线后重新检查；若仍失败，请检查 Java 安装和 Agent 运行日志。' }
    case 'CONFIG_INVALID':
      return { reason: item.kind === 'DIRECT_LOAD_CONFIG' ? '旁路导入配置文件的格式或内容无法通过校验。' : '工具安装布局或必需组件没有通过校验。', action: item.kind === 'DIRECT_LOAD_CONFIG' ? '请按官方工具说明核对配置文件。' : '请按安装说明核对完整的 4.3.5 工具包、支持平台及所需组件，再重新检查。' }
    case 'NOT_CONFIGURED':
      return { reason: '尚未取得这项配置。', action: item.required ? '请补齐所需配置后重新检查。' : '需要使用该功能时，再按安装说明补齐配置。' }
    case 'SAMPLING_UNAVAILABLE':
      return { reason: item.kind === 'RESOURCES' ? 'Agent 尚未取得完整的系统资源信息。' : 'Agent 尚未完成这项核验。', action: '确认 Agent 正常运行并可访问所需文件后，重新检查。' }
    default:
      return { reason: '尚未取得可解释的检查结果。', action: '请重新检查；若仍无结果，请确认控制面与 Agent 已同步更新。' }
  }
}

// 顶部摘要只列必需项；可选项异常不能被描述成阻断当前导出环境。
export function inspectionBlockingSummary(inspection: NodeRuntimeInspection): string | undefined {
  const failed = inspection.items.filter(item => item.required && item.status === 'FAILED').map(item => inspectionLabels[item.kind])
  const unfinished = inspection.items.filter(item => item.required && item.status !== 'PASSED' && item.status !== 'FAILED').map(item => inspectionLabels[item.kind])
  if (!failed.length && !unfinished.length) return undefined
  const causes = [failed.length ? failed.join('、') + '未通过' : '', unfinished.length ? unfinished.join('、') + '未取得结果' : ''].filter(Boolean).join('；')
  return `环境检查${failed.length ? '未通过' : '结果待确认'}：${causes}。请按下方「环境检查」提示处理后重新检查。`
}

export function inspectionEmptyMessage(check: ExecutionNodeDetail['environmentCheck']): string {
  if (check?.status === 'PENDING') return '正在等待 Agent 返回检查结果；可稍后点击「刷新」查看。'
  if (check?.status === 'FAILED') return '本次检查没有返回逐项结果。请确认控制面与 Agent 已同步更新后重新检查。'
  if (check?.status === 'PASSED') return '上次检查通过，但未提供逐项明细；可重新检查以取得明细。'
  return '还没有环境检查结果。Agent 在线后可点击「检查节点环境」。'
}

export const activityLabels: Record<string, string> = { EXECUTION_NODE_CREATED: '注册节点', EXECUTION_NODE_UPDATED: '编辑节点', EXECUTION_NODE_ENABLED: '启用节点', EXECUTION_NODE_ARCHIVED: '历史归档', EXECUTION_NODE_DELETED: '删除节点', AGENT_ENROLLMENT_ISSUED: '签发关联材料', AGENT_ENROLLED: 'Agent 完成关联', EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED: '请求环境检查', EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED: '重新排队环境检查', EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETED: '环境检查回传' }

export function activitySourceLabel(source: NodeActivityItem['source'] | 'UNKNOWN' | undefined): string { return source === 'SUBJECT' ? '用户操作' : source === 'AGENT' ? 'Agent 上报' : source === 'SYSTEM' ? '系统' : '尚无来源记录' }
export function activityResultLabel(result: NodeActivityItem['result']): string { return result === 'SUCCEEDED' ? '完成' : result === 'FAILED' ? '失败' : '拒绝' }
