import { describe, expect, it } from 'vitest'
import type { OverviewBucket } from '@/api/overview'
import { overviewTrendAreaPath, overviewTrendHit, overviewTrendPath, overviewTrendQuery, overviewTrendScale, overviewTrendX, overviewTrendY, type OverviewTrendSession } from './overviewTrend'

const buckets: readonly OverviewBucket[] = [
  { from: '2026-10-28T00:30:00Z', to: '2026-10-28T01:00:00Z', total: 1, succeeded: 0, failed: 1 },
  { from: '2026-10-28T01:00:00Z', to: '2026-10-28T02:00:00Z', total: 2, succeeded: 1, failed: 0 },
  { from: '2026-10-28T02:00:00Z', to: '2026-10-28T02:30:00Z', total: 0, succeeded: 0, failed: 0 },
]
const session: OverviewTrendSession = { id: 1, mode: 'analysis', range: '24h', snapshotAt: buckets.at(-1)!.to, buckets }

describe('首页趋势真实分桶与下钻边界', () => {
  it('部分小时桶按真实时间位置连线，空窗口和单个桶不生成额外图点', () => {
    expect([0, 1, 2].map(index => overviewTrendX(buckets, index))).toEqual([8, 256, 752])
    expect(overviewTrendPath(buckets, 'total', 2)).toMatch(/^M8,106 C.*256,8 C.*752,204$/)
    expect(overviewTrendPath([], 'total', 2)).toBe('')
    expect(overviewTrendAreaPath([], 2)).toBe('')
    expect(overviewTrendPath([buckets[0]!], 'total', 2)).toBe('M380,106')
    expect(overviewTrendX([buckets[0]!], 0)).toBe(380)
  })
  it('整数刻度为小值、尖峰和大值保留顶部空间，所有刻度与纵坐标一致', () => {
    for (const count of [0, 1, 2, 4, 5, 6, 9, 38, 1000000]) {
      const { maximum, ticks } = overviewTrendScale([{ ...buckets[0]!, total: count, succeeded: 0, failed: 0 }])
      expect(maximum).toBeGreaterThan(count)
      expect(ticks[0]).toBe(maximum)
      expect(ticks.at(-1)).toBe(0)
      expect(ticks.every(Number.isInteger)).toBe(true)
      expect(overviewTrendY(count, maximum)).toBeGreaterThan(8)
      expect(overviewTrendY(0, maximum)).toBe(204)
      expect(ticks.every((tick, index) => !index || tick < ticks[index - 1]!)).toBe(true)
    }
    expect(overviewTrendScale([{ ...buckets[0]!, total: 38 }])).toEqual({ maximum: 50, ticks: [50, 40, 30, 20, 10, 0] })
  })
  it('曲线经过真实端点，不越过相邻计数，零平台保持为零', () => {
    const cases = [[0, 38, 0, 0, 0], [0, 0, 38, 0, 0], [1, 2, 3, 5, 8], [8, 5, 3, 2, 1], [1, 15, 2, 17, 0], [0, 1, 0, 1, 0]]
    const offsets = [0, 1800000, 5400000, 14400000, 25200000, 28800000]
    for (const values of cases) {
      const facts = values.map((total, index) => ({ ...buckets[0]!, from: new Date(Date.parse(buckets[0]!.from) + offsets[index]!).toISOString(), to: new Date(Date.parse(buckets[0]!.from) + offsets[index + 1]!).toISOString(), total, succeeded: 0, failed: 0 }))
      const maximum = overviewTrendScale(facts).maximum
      const parts = [...overviewTrendPath(facts, 'total', maximum).matchAll(/[MC]([^MC]*)/g)].map(match => match[1]!.trim().split(/[\s,]+/).map(Number))
      expect(parts).toHaveLength(values.length)
      for (let index = 1; index < parts.length; index++) {
        const previous = parts[index - 1]!, start = previous.slice(-2), segment = parts[index]!
        const [x1, y1, x2, y2, x, y] = segment as [number, number, number, number, number, number]
        expect(x).toBeCloseTo(overviewTrendX(facts, index), 3)
        expect(y).toBeCloseTo(overviewTrendY(values[index]!, maximum), 3)
        expect(x1).toBeGreaterThan(start[0]!)
        expect(x2).toBeLessThan(x)
        expect(y1).toBe(start[1])
        expect(y2).toBe(y)
        for (let sample = 0; sample <= 20; sample++) {
          const t = sample / 20, u = 1 - t
          const actual = u ** 3 * start[1]! + 3 * u ** 2 * t * y1 + 3 * u * t ** 2 * y2 + t ** 3 * y
          expect(actual).toBeGreaterThanOrEqual(Math.min(start[1]!, y) - 0.0001)
          expect(actual).toBeLessThanOrEqual(Math.max(start[1]!, y) + 0.0001)
          if (values[index - 1] === 0 && values[index] === 0) expect(actual).toBeCloseTo(204, 8)
        }
      }
      expect(overviewTrendAreaPath(facts, maximum)).toBe(`${overviewTrendPath(facts, 'total', maximum)} L752,204 L8,204 Z`)
    }
  })
  it('分项和总数使用同一过渡进度，桶间不会出现分项合计超过总数', () => {
    const facts = [[38, 31, 7], [3, 0, 1], [8, 4, 3]].map(([total, succeeded, failed], index) => ({ ...buckets[index]!, total: total!, succeeded: succeeded!, failed: failed! }))
    const maximum = overviewTrendScale(facts).maximum
    const curves = ['total', 'succeeded', 'failed'].map(key => [...overviewTrendPath(facts, key as 'total' | 'succeeded' | 'failed', maximum).matchAll(/[MC]([^MC]*)/g)].map(match => match[1]!.trim().split(/[\s,]+/).map(Number)))
    for (let index = 1; index < facts.length; index++) {
      for (let sample = 0; sample <= 20; sample++) {
        const t = sample / 20, u = 1 - t
        const counts = curves.map(parts => {
          const startY = parts[index - 1]!.at(-1)!, segment = parts[index]!
          const y = u ** 3 * startY + 3 * u ** 2 * t * segment[1]! + 3 * u * t ** 2 * segment[3]! + t ** 3 * segment[5]!
          return (204 - y) * maximum / 196
        })
        expect(counts[1]! + counts[2]!).toBeLessThanOrEqual(counts[0]! + 0.0001)
      }
    }
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
