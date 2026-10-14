import type { ExecutionNodeDataRootUsage, ExecutionNodeSummary } from '@/api/browser'

export function nodeSnapshotKey(node: ExecutionNodeSummary): string {
  return JSON.stringify([node.id, node.revision, node.agentFacts?.bootId, node.agentFacts?.observedAt])
}

export function resourcePercentage(value: number | undefined): number | undefined {
  return value !== undefined && Number.isFinite(value) && value >= 0 && value <= 100 ? value : undefined
}

// 按 50% 与 80% 分为浅蓝、黄色、红色；黄色复用现有警告配色，仅用于视觉而非调度或任务资格判断。
export function resourceUsageColor(value: number | undefined): string {
  const percent = resourcePercentage(value)
  if (percent === undefined) return 'var(--ob-color-border)'
  if (percent >= 80) return 'var(--ob-color-danger)'
  if (percent >= 50) return 'color-mix(in srgb, var(--ob-foundation-warning-border) 70%, var(--ob-color-warning))'
  return 'var(--ob-color-primary-hover)'
}

export interface NodeResourceCapacity {
  totalBytes: number
  usedBytes: number
}

export interface NodeDiskUsage extends NodeResourceCapacity {
  percent: number
}

// 已用量只由同一采样的总量减可用量得到；未知、无效或精度不可靠时不填零。
export function resourceCapacity(totalBytes: number | undefined, availableBytes: number | undefined): NodeResourceCapacity | undefined {
  if (totalBytes === undefined || availableBytes === undefined || !Number.isSafeInteger(totalBytes) ||
    !Number.isSafeInteger(availableBytes) || totalBytes <= 0 || availableBytes < 0 || availableBytes > totalBytes) return undefined
  return { totalBytes, usedBytes: totalBytes - availableBytes }
}

// 总量决定两项共用的二进制单位，避免已用量和总量采用不同尺度或小容量被显示为零 GiB。
export function resourceCapacityLabel(capacity: NodeResourceCapacity | undefined): string | undefined {
  if (!capacity) return undefined
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  const index = Math.min(Math.floor(Math.log2(capacity.totalBytes) / 10), units.length - 1)
  const divisor = 1024 ** index
  const precision = index ? 1 : 0
  return `${(capacity.usedBytes / divisor).toFixed(precision)} ${units[index]} / ${(capacity.totalBytes / divisor).toFixed(precision)} ${units[index]}`
}

// 白名单目录可能落在同一磁盘，不累加容量；使用率与容量均来自使用率最高的同一份磁盘采样。
export function nodeDiskUsage(roots: readonly ExecutionNodeDataRootUsage[] | undefined): NodeDiskUsage | undefined {
  if (!roots?.length) return undefined
  let highest: NodeDiskUsage | undefined
  for (const root of roots) {
    const capacity = resourceCapacity(root.totalBytes, root.availableBytes)
    if (!capacity) return undefined
    const usage = { ...capacity, percent: capacity.usedBytes / capacity.totalBytes * 100 }
    if (!highest || usage.percent > highest.percent) highest = usage
  }
  return highest
}

export interface NodeExecutionQuota {
  totalGiB: number
  reservedGiB: number
  usedGiB: number
}

// 图表只接受完整预算事实；缺失、超额或非法数值不伪装成零占用。
export function executionQuotaSegments(quota: NodeExecutionQuota | undefined) {
  if (!quota || ![quota.totalGiB, quota.reservedGiB, quota.usedGiB].every(Number.isFinite) || quota.totalGiB <= 0 ||
    quota.reservedGiB < 0 || quota.usedGiB < 0 || quota.reservedGiB + quota.usedGiB > quota.totalGiB) return undefined
  const freeGiB = quota.totalGiB - quota.reservedGiB - quota.usedGiB
  return [
    { key: 'used', label: '已用', value: quota.usedGiB, percent: quota.usedGiB / quota.totalGiB * 100, meaning: '运行任务已计入的执行预算' },
    { key: 'reserved', label: '预留', value: quota.reservedGiB, percent: quota.reservedGiB / quota.totalGiB * 100, meaning: '已分配、尚未确认运行或结束的执行预算' },
    { key: 'free', label: '可用', value: freeGiB, percent: freeGiB / quota.totalGiB * 100, meaning: '尚未分配的执行预算' },
  ]
}

// 采用两段半圆处理 100% 占用，避免 SVG 完整圆弧首尾重合后消失。
export function quotaSegmentPath(startPercent: number, percent: number): string {
  if (percent <= 0) return ''
  const gap = percent === 100 ? 0 : Math.min(0.35, percent / 4)
  const start = (startPercent + gap) * Math.PI / 50 - Math.PI / 2
  const end = (startPercent + percent - gap) * Math.PI / 50 - Math.PI / 2
  const mid = (start + end) / 2
  const point = (radius: number, angle: number) => `${50 + radius * Math.cos(angle)},${50 + radius * Math.sin(angle)}`
  return `M${point(45, start)} A45,45 0 0,1 ${point(45, mid)} A45,45 0 0,1 ${point(45, end)} L${point(34, end)} A34,34 0 0,0 ${point(34, mid)} A34,34 0 0,0 ${point(34, start)} Z`
}
