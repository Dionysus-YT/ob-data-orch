import { describe, expect, it } from 'vitest'
import type { OverviewBucket } from '@/api/overview'
import { overviewTrendHit, overviewTrendMaximum, overviewTrendPath, overviewTrendQuery, overviewTrendX, type OverviewTrendSession } from './overviewTrend'

const buckets: readonly OverviewBucket[] = [
  { from: '2026-10-28T00:30:00Z', to: '2026-10-28T01:00:00Z', total: 1, succeeded: 0, failed: 1 },
  { from: '2026-10-28T01:00:00Z', to: '2026-10-28T02:00:00Z', total: 2, succeeded: 1, failed: 0 },
  { from: '2026-10-28T02:00:00Z', to: '2026-10-28T02:30:00Z', total: 0, succeeded: 0, failed: 0 },
]
const session: OverviewTrendSession = { id: 1, mode: 'analysis', range: '24h', snapshotAt: buckets.at(-1)!.to, buckets }

describe('首页趋势真实分桶与下钻边界', () => {
  it('部分小时桶按真实时间位置连线，不插值或生成额外计数', () => {
    expect([0, 1, 2].map(index => overviewTrendX(buckets, index))).toEqual([8, 256, 752])
    expect(overviewTrendMaximum(buckets)).toBe(2)
    expect(overviewTrendPath(buckets, 'total', 2)).toBe('M8,106 L256,8 L752,204')
    expect(overviewTrendPath(buckets, 'failed', 2)).not.toMatch(/[CQ]/)
    expect(overviewTrendMaximum([])).toBe(2)
    expect(overviewTrendX([buckets[0]!], 0)).toBe(380)
  })
  it('命中区连续且不重叠，包含首尾图点并覆盖绘图区', () => {
    const regions = buckets.map((_, index) => overviewTrendHit(buckets, index))
    expect(regions[0]!.left).toBe('0%')
    for (let index = 0; index < regions.length - 1; index++) {
      expect(parseFloat(regions[index]!.left) + parseFloat(regions[index]!.width)).toBeCloseTo(parseFloat(regions[index + 1]!.left))
    }
    expect(parseFloat(regions.at(-1)!.left) + parseFloat(regions.at(-1)!.width)).toBeCloseTo(100)
  })
  it('总数按完整终态窗口读取，成功失败保持同一桶的半开区间', () => {
    expect(overviewTrendQuery(session, '', '')).toEqual({ finishedFrom: buckets[0]!.from, finishedTo: buckets.at(-1)!.to })
    expect(overviewTrendQuery(session, buckets[1]!.from, 'FAILED')).toEqual({ finishedFrom: buckets[1]!.from, finishedTo: buckets[1]!.to, state: 'FAILED' })
    expect(overviewTrendQuery(session, buckets[1]!.from, 'CANCELLED')?.state).toBe('CANCELLED')
  })
  it('伪造桶、非终态、空窗口和倒置时间均失败关闭', () => {
    expect(overviewTrendQuery(session, '2026-10-28T00:00:00Z', '')).toBeNull()
    expect(overviewTrendQuery(session, '', 'RUNNING')).toBeNull()
    expect(overviewTrendQuery({ ...session, buckets: [] }, '', '')).toBeNull()
    expect(overviewTrendQuery({ ...session, buckets: [{ ...buckets[0]!, to: buckets[0]!.from }] }, '', '')).toBeNull()
  })
})
