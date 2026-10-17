import type { ExecutionNodeDetail, ExecutionNodeSummary } from '@/api/browser'
import { nodeCPUArchitectureOptions, nodeOperatingSystemOptions, nodePlatformClassification } from './nodePlatformRules'
import { inspectionBlockingSummary } from './nodeInspectionPresentation'

// nodePlatformLabel 将机器平台枚举转换为显示名称，不改变注册协议与安装包目标。
export function nodePlatformLabel(platform: ExecutionNodeSummary['platform']): string {
  return `${nodeOperatingSystemLabel(platform)} · ${nodeCPUArchitectureLabel(platform)}`
}

// nodeOperatingSystemLabel 展示已验证平台值对应的操作系统分类。
export function nodeOperatingSystemLabel(platform: ExecutionNodeSummary['platform']): string {
  const { operatingSystem } = nodePlatformClassification(platform)
  return nodeOperatingSystemOptions.find(option => option.value === operatingSystem)!.label
}

// nodeCPUArchitectureLabel 展示已验证平台值对应的 CPU 架构名称。
export function nodeCPUArchitectureLabel(platform: ExecutionNodeSummary['platform']): string {
  const { cpuArchitecture } = nodePlatformClassification(platform)
  return nodeCPUArchitectureOptions.find(option => option.value === cpuArchitecture)!.label
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
  if (status === 'NORMAL') return '检查通过'
  if (status === 'ABNORMAL') return '检查异常'
  if (status === 'UNCONFIRMED') return '待核验'
  return status === 'EXPIRED' ? '检查已失效' : '未检查'
}

export function nodeCapacityLabel(status: ExecutionNodeSummary['capacityStatus']): string {
  if (status === 'AVAILABLE') return '有空位'
  return status === 'BUSY' ? '已满' : '未知'
}

// 各类别独立映射服务端事实；通用接收资格不能扩大为某类导入/导出模块已经可用。
export function nodeCategoryStates(node: ExecutionNodeSummary): { key: string; label: string; status: 'success' | 'error' | 'warning' | 'default' | 'processing'; text: string }[] {
  const authorizationRequired = node.unavailableReasons.includes('ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED')
  const checking = !authorizationRequired && node.unavailableReasons.includes('ENVIRONMENT_CHECK_IN_PROGRESS')
  return [
    { key: 'management', label: '管理', status: node.managementState === 'ENABLED' ? 'success' : node.managementState === 'MAINTENANCE' ? 'warning' : 'default', text: nodeManagementLabel(node.managementState) },
    { key: 'agent', label: 'Agent', status: node.agentAssociationStatus === 'PENDING' ? 'default' : node.heartbeatStatus === 'ONLINE' ? 'success' : node.heartbeatStatus === 'OFFLINE' ? 'error' : 'default', text: node.agentAssociationStatus === 'PENDING' ? nodeAssociationLabel(node.agentAssociationStatus) : nodeHeartbeatLabel(node.heartbeatStatus) },
    { key: 'environment', label: '运行环境', status: checking ? 'processing' : node.environmentStatus === 'NORMAL' ? 'success' : node.environmentStatus === 'ABNORMAL' ? 'error' : node.environmentStatus === 'EXPIRED' || node.environmentStatus === 'UNCONFIRMED' || authorizationRequired ? 'warning' : 'default', text: authorizationRequired ? '待授权' : checking ? '检查中' : nodeEnvironmentLabel(node.environmentStatus) },
    { key: 'capacity', label: '任务槽位', status: node.capacityStatus === 'AVAILABLE' ? 'success' : node.capacityStatus === 'BUSY' ? 'warning' : 'default', text: nodeCapacityLabel(node.capacityStatus) },
  ]
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

export function nodeUnavailableReasonLabel(reason: string, context?: Pick<ExecutionNodeDetail, 'environmentStatus' | 'environmentCheck'>): string {
  if (!reason) return '控制面未提供具体阻断原因'
  const check = context?.environmentCheck
  // 只用当前异常或待核验清单解释阻断，避免旧结果或可选项异常被误认为本次原因。
  if (['TOOL_RUNTIME_INVALID', 'TOOL_RUNTIME_UNAVAILABLE', 'ENVIRONMENT_CHECK_ABNORMAL'].includes(reason) && (context?.environmentStatus === 'ABNORMAL' || context?.environmentStatus === 'UNCONFIRMED') && check?.status === 'FAILED' && check.inspection?.checkId === check.checkId) {
    const summary = check.inspection && inspectionBlockingSummary(check.inspection)
    if (summary) return summary
  }
  const labels: Record<string, string> = {
    AGENT_ASSOCIATION_REQUIRED: '需要完成 Agent 关联',
    HEARTBEAT_REQUIRED: '尚未收到 Agent 心跳',
    HEARTBEAT_OFFLINE: 'Agent 当前离线',
    ENVIRONMENT_CHECK_REQUIRED: '尚未完成环境检查，请在 Agent 在线后点击「检查节点环境」。',
    ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED: '尚未授权 Agent 自动复验环境，请点击「授权并检查环境」。授权仅覆盖固定本机检查和版本查询。',
    ENVIRONMENT_CHECK_IN_PROGRESS: '环境检查进行中，正在等待 Agent 返回结果。',
    ENVIRONMENT_CHECK_ABNORMAL: '环境检查未通过，请查看节点详情中的检查项并按提示处理。',
    TOOL_RUNTIME_INVALID: '环境检查有未通过的必需项，请查看节点详情中的检查结果。',
    TOOL_RUNTIME_UNAVAILABLE: '环境检查结果不完整，部分必需项没有取得结果；请查看节点详情中的检查提示。',
    ENVIRONMENT_CHECK_EXPIRED: '环境或配置已变化，上次检查结果已失效，请重新检查。',
    RUNTIME_CONFIGURATION_MISMATCH: '等待 Agent 空闲后同步配置并重新检查环境',
    CAPACITY_UNKNOWN: '任务容量尚未取得',
    CAPACITY_BUSY: '当前任务容量已满',
    EXECUTION_BUDGET_UNKNOWN: '执行内存额度尚不可核对',
    EXECUTION_BUDGET_EXHAUSTED: '执行内存额度已满',
    NODE_DISABLED: '节点尚未启用；环境检查通过后请点击「启用节点」。',
    NODE_MAINTENANCE: '节点处于维护中',
  }
  return labels[reason] ?? '控制面返回了未识别的阻断原因，请在节点详情中核对'
}

// nodeCanRequestEnvironmentCheck 统一主动复验资格；待授权允许替换尚未启动版本查询的自动检查。
export function nodeCanRequestEnvironmentCheck(node: ExecutionNodeSummary): boolean {
  return node.agentAssociationStatus === 'ASSOCIATED' && node.heartbeatStatus === 'ONLINE'
    && !node.unavailableReasons.includes('RUNTIME_CONFIGURATION_MISMATCH')
    && (!node.unavailableReasons.includes('ENVIRONMENT_CHECK_IN_PROGRESS') || node.unavailableReasons.includes('ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED'))
}

export function nodePrimaryAction(node: ExecutionNodeSummary): 'environment-check' | 'enable' | undefined {
  if (!nodeCanRequestEnvironmentCheck(node)) return undefined
  if (node.unavailableReasons.includes('ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED')) return 'environment-check'
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
