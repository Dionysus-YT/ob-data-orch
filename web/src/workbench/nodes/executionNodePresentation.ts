import type { ExecutionNodeSummary } from '@/api/browser'

export function nodePlatformLabel(platform: ExecutionNodeSummary['platform']): string {
  if (platform === 'WINDOWS_AMD64') return 'Windows AMD64'
  if (platform === 'LINUX_AMD64') return 'Kylin Linux AMD64'
  return 'Kylin Linux ARM64'
}

export function nodeManagementLabel(state: ExecutionNodeSummary['managementState']): string {
  if (state === 'ENABLED') return '已启用'
  return state === 'MAINTENANCE' ? '维护中' : '已禁用'
}

export function nodeAssociationLabel(status: ExecutionNodeSummary['agentAssociationStatus']): string {
  return status === 'ASSOCIATED' ? '已关联' : '待关联'
}

export function nodeHeartbeatLabel(status: ExecutionNodeSummary['heartbeatStatus']): string {
  if (status === 'ONLINE') return '在线'
  return status === 'OFFLINE' ? '离线' : '从未连接'
}

export function nodeEnvironmentLabel(status: ExecutionNodeSummary['environmentStatus']): string {
  if (status === 'NORMAL') return '工具已就绪'
  if (status === 'ABNORMAL') return '工具异常'
  return status === 'EXPIRED' ? '已过期' : '未检查'
}

export function nodeCapacityLabel(status: ExecutionNodeSummary['capacityStatus']): string {
  if (status === 'AVAILABLE') return '可用'
  return status === 'BUSY' ? '繁忙' : '未知'
}

export function nodeTimeLabel(value: string | null | undefined): string {
  if (!value) return '尚未收到'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '控制面未提供有效时间' : date.toLocaleString('zh-CN')
}

export function nodeCapacitySummary(node: ExecutionNodeSummary): string {
  if (!node.agentFacts) return node.capacityStatus === 'UNKNOWN' ? '尚未收到容量快照' : '控制面未提供容量快照'
  return `${node.agentFacts.capacityUsed} / ${node.agentFacts.capacityTotal}`
}

export function nodeUnavailableReasonLabel(reason: string): string {
  if (!reason) return '控制面未提供具体阻断原因'
  const labels: Record<string, string> = {
    AGENT_ASSOCIATION_REQUIRED: '需要完成 Agent 关联',
    HEARTBEAT_REQUIRED: '尚未收到 Agent 心跳',
    HEARTBEAT_OFFLINE: 'Agent 当前离线',
    ENVIRONMENT_CHECK_REQUIRED: '尚未取得环境检查事实',
    ENVIRONMENT_CHECK_IN_PROGRESS: '环境检查正在等待 Agent 回传',
    ENVIRONMENT_CHECK_ABNORMAL: '环境检查发现异常',
    TOOL_RUNTIME_INVALID: '工具运行时无效：请检查节点上的 OBDUMPER 工具目录与 Java',
    TOOL_RUNTIME_UNAVAILABLE: '工具运行时暂无法核验，请稍后重新检查环境',
    ENVIRONMENT_CHECK_EXPIRED: '环境检查事实已过期',
    RUNTIME_CONFIGURATION_MISMATCH: '等待 Agent 空闲后同步配置并重新检查环境',
    CAPACITY_UNKNOWN: '任务容量尚未取得',
    CAPACITY_BUSY: '当前任务容量已满',
    NODE_DISABLED: '节点已禁用',
    NODE_MAINTENANCE: '节点处于维护中',
  }
  return labels[reason] ?? '控制面返回了未识别的阻断原因，请在节点详情中核对'
}

export function nodePrimaryAction(node: ExecutionNodeSummary): 'environment-check' | 'enable' | undefined {
  if (node.agentAssociationStatus !== 'ASSOCIATED' || node.heartbeatStatus !== 'ONLINE') return undefined
  if (node.unavailableReasons.includes('ENVIRONMENT_CHECK_IN_PROGRESS') || node.unavailableReasons.includes('RUNTIME_CONFIGURATION_MISMATCH')) return undefined
  if (node.environmentStatus !== 'NORMAL') return 'environment-check'
  if (node.managementState === 'DISABLED' && node.capacityStatus === 'AVAILABLE') return 'enable'
  return undefined
}

export function percentageLabel(value: number | undefined) {
  return value === undefined ? '尚未采集' : `${value.toFixed(1)}%`
}

export function byteLabel(value: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index++ }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

export function bootIdSummary(value: string) {
  return value.length <= 20 ? value : `${value.slice(0, 12)}…${value.slice(-4)}`
}
