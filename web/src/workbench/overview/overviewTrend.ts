import type { LocationQuery } from 'vue-router'
import type { OverviewBucket, OverviewRange } from '@/api/overview'

/** 趋势只复用已有终态计数；曲线仅修饰连线，不生成平滑计数、成功率或预测。 */
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

/** 使用整数刻度并为峰值留出顶部空间；切换图例不改变同一快照的比例尺。 */
export function overviewTrendScale(buckets: readonly OverviewBucket[]): { maximum: number; ticks: readonly number[] } {
  const largest = Math.max(0, ...buckets.map(bucket => bucket.total))
  const target = Math.max(1, largest / 4), magnitude = 10 ** Math.floor(Math.log10(target))
  const step = magnitude * [1, 2, 3, 5, 10].find(value => value * magnitude >= target)!
  const maximum = Math.max(2, Math.ceil((largest + Math.max(1, largest * 0.12)) / step) * step)
  return { maximum, ticks: Array.from({ length: Math.round(maximum / step) + 1 }, (_, index) => maximum - index * step) }
}

/** 绘图、刻度和悬停标记共用纵坐标映射，避免视觉位置与真实读数分离。 */
export function overviewTrendY(count: number, maximum: number): number { return 204 - count * 196 / maximum }

/** 相邻图点的中线划分命中区，窄屏及部分桶也不会让按钮重叠选错时段。 */
export function overviewTrendHit(buckets: readonly OverviewBucket[], index: number): { left: string; width: string } {
  const center = overviewTrendX(buckets, index)
  const left = index ? (overviewTrendX(buckets, index - 1) + center) / 2 : 0
  const right = index < buckets.length - 1 ? (center + overviewTrendX(buckets, index + 1)) / 2 : 760
  return { left: `${left / 760 * 100}%`, width: `${(right - left) / 760 * 100}%` }
}

/** 连线经过每个真实桶；各系列共用区间过渡，保持端点范围、零平台及总数与分项的大小关系。 */
export function overviewTrendPath(buckets: readonly OverviewBucket[], key: OverviewTrendSeries, maximum: number): string {
  const points = buckets.map((bucket, index) => ({ x: overviewTrendX(buckets, index), y: overviewTrendY(bucket[key], maximum) }))
  if (!points.length) return ''
  const coordinate = (value: number) => Math.round(value * 10000) / 10000
  let path = `M${coordinate(points[0]!.x)},${coordinate(points[0]!.y)}`
  for (let index = 1; index < points.length; index++) {
    const previous = points[index - 1]!, point = points[index]!, third = (point.x - previous.x) / 3
    // 水平端点切线让各段平滑衔接；相同过渡权重避免成功、失败与总数在桶间产生倒挂。
    path += ` C${coordinate(previous.x + third)},${coordinate(previous.y)} ${coordinate(point.x - third)},${coordinate(point.y)} ${coordinate(point.x)},${coordinate(point.y)}`
  }
  return path
}

/** 主系列只添加装饰性浅底，不参与计数、面积统计或任务下钻。 */
export function overviewTrendAreaPath(buckets: readonly OverviewBucket[], maximum: number): string {
  if (!buckets.length) return ''
  return `${overviewTrendPath(buckets, 'total', maximum)} L${overviewTrendX(buckets, buckets.length - 1)},204 L${overviewTrendX(buckets, 0)},204 Z`
}

/** 下钻只接受当前会话中的窗口和三类已有终态，保持服务端半开区间与授权读取。 */
export function overviewTrendQuery(session: OverviewTrendSession, selectedFrom: string, state: string): LocationQuery | null {
  const bucket = selectedFrom ? session.buckets.find(item => item.from === selectedFrom) : undefined
  if (selectedFrom && !bucket || !['', 'SUCCEEDED', 'FAILED', 'CANCELLED'].includes(state)) return null
  const from = bucket?.from ?? session.buckets[0]?.from, to = bucket?.to ?? session.buckets.at(-1)?.to
  if (!from || !to || !(Date.parse(from) < Date.parse(to))) return null
  return { finishedFrom: from, finishedTo: to, ...(state ? { state } : {}) }
}
