import type { LocationQuery } from 'vue-router'
import type { OverviewBucket, OverviewRange } from '@/api/overview'

/** 趋势只复用已有终态计数；总数包含已取消，不生成成功率、平滑值或预测。 */
export const overviewTrendSeries = [
  { key: 'total', label: '终态总数', state: '', className: 'trend-total' },
  { key: 'succeeded', label: '成功', state: 'SUCCEEDED', className: 'trend-succeeded' },
  { key: 'failed', label: '失败', state: 'FAILED', className: 'trend-failed' },
] as const

/** 图例系列与任务终态筛选保持同一业务语义。 */
export type OverviewTrendSeries = typeof overviewTrendSeries[number]['key']

/** 每次打开弹窗捕获独立只读会话；后台刷新不得移动正在分析的时间窗口。 */
export interface OverviewTrendSession {
  readonly id: number
  readonly mode: 'chart' | 'analysis'
  readonly range: OverviewRange
  readonly snapshotAt: string
  readonly buckets: readonly OverviewBucket[]
  readonly selectedFrom?: string
}

/** 按真实桶起点放置图点，首尾部分桶不被虚构成完整小时。 */
export function overviewTrendX(buckets: readonly OverviewBucket[], index: number): number {
  const from = Date.parse(buckets[0]?.from ?? ''), last = Date.parse(buckets.at(-1)?.from ?? '')
  return last > from ? 8 + (Date.parse(buckets[index]!.from) - from) * 744 / (last - from) : 380
}

/** 偶数上界让零值和中间刻度保持整数，不裁掉合法计数。 */
export function overviewTrendMaximum(buckets: readonly OverviewBucket[]): number {
  return Math.max(2, Math.ceil(Math.max(0, ...buckets.map(bucket => bucket.total)) / 2) * 2)
}

/** 相邻图点的中线划分命中区，窄屏及部分桶也不会让按钮重叠选错时段。 */
export function overviewTrendHit(buckets: readonly OverviewBucket[], index: number): { left: string; width: string } {
  const center = overviewTrendX(buckets, index)
  const left = index ? (overviewTrendX(buckets, index - 1) + center) / 2 : 0
  const right = index < buckets.length - 1 ? (center + overviewTrendX(buckets, index + 1)) / 2 : 760
  return { left: `${left / 760 * 100}%`, width: `${(right - left) / 760 * 100}%` }
}

/** 仅连接真实桶计数；不以曲线插值暗示不存在的中间任务数。 */
export function overviewTrendPath(buckets: readonly OverviewBucket[], key: OverviewTrendSeries, maximum: number): string {
  return buckets.map((bucket, index) => `${index ? 'L' : 'M'}${overviewTrendX(buckets, index)},${204 - bucket[key] * 196 / maximum}`).join(' ')
}

/** 下钻只接受当前会话中的窗口和三类已有终态，保持服务端半开区间与授权读取。 */
export function overviewTrendQuery(session: OverviewTrendSession, selectedFrom: string, state: string): LocationQuery | null {
  const bucket = selectedFrom ? session.buckets.find(item => item.from === selectedFrom) : undefined
  if (selectedFrom && !bucket || !['', 'SUCCEEDED', 'FAILED', 'CANCELLED'].includes(state)) return null
  const from = bucket?.from ?? session.buckets[0]?.from, to = bucket?.to ?? session.buckets.at(-1)?.to
  if (!from || !to || !(Date.parse(from) < Date.parse(to))) return null
  return { finishedFrom: from, finishedTo: to, ...(state ? { state } : {}) }
}
