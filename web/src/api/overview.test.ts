import { describe, expect, it, vi } from 'vitest'
import { createOverviewApi, parseOverviewSnapshot } from './overview'
import { overviewFixture } from '../../tests/fixtures/overview'

const at = new Date('2026-10-23T08:30:00Z')

describe('首页响应契约', () => {
  it('接受两种时间窗口、可靠进度和受限分区', () => {
    for (const period of ['24h', '7d'] as const) expect(parseOverviewSnapshot(overviewFixture(at, period), period).range).toBe(period)
    const value = { ...overviewFixture(at), nodes: { status: 'RESTRICTED' }, trend: { status: 'UNAVAILABLE' } }
    expect(parseOverviewSnapshot(value, '24h')).toMatchObject({ nodes: { status: 'RESTRICTED' }, trend: { status: 'UNAVAILABLE' } })
  })

  it.each([
    ['未知字段', (value: Record<string, unknown>) => { value.secret = 'synthetic-only' }],
    ['错误范围', (value: Record<string, unknown>) => { value.range = '7d' }],
    ['错误时区', (value: Record<string, unknown>) => { value.timeZone = 'Asia/Shanghai' }],
    ['非法时间', (value: Record<string, unknown>) => { value.snapshotAt = 'not-an-instant' }],
    ['未知刷新策略', (value: Record<string, unknown>) => { value.staleAfterSeconds = 0 }],
    ['受限携带数据', (value: Record<string, unknown>) => { value.nodes = { status: 'RESTRICTED', data: { total: 3 } } }],
    ['负数计数', (value: Record<string, unknown>) => { value.metrics = { status: 'READY', data: { running: -1, waiting: 0, todaySucceeded: 0, todayFailed: 0 } } }],
    ['今日边界错位', (value: Record<string, unknown>) => { value.todayTo = value.todayFrom }],
  ])('%s 不得被降级为正常', (_name, mutate) => {
    const value = structuredClone(overviewFixture(at)) as unknown as Record<string, unknown>
    mutate(value)
    expect(() => parseOverviewSnapshot(value, '24h')).toThrow()
  })

  it('拒绝重复任务、无效进度、矛盾节点与分桶缺口', () => {
    const base = overviewFixture(at)
    if (base.myTasks.status !== 'READY' || base.nodes.status !== 'READY' || base.trend.status !== 'READY') throw new Error('合成夹具缺失')
    const changes = [
      { myTasks: { status: 'READY', data: [base.myTasks.data[0], base.myTasks.data[0]] } },
      { myTasks: { status: 'READY', data: [{ ...base.myTasks.data[1], type: 'OBLOADER_DIRECT_LOAD' }] } },
      { nodes: { status: 'READY', data: { ...base.nodes.data, items: [{ ...base.nodes.data.items[0], acceptsNewTasks: true }] } } },
      { trend: { status: 'READY', data: base.trend.data.slice(0, -1) } },
      { trend: { status: 'READY', data: base.trend.data.map((bucket, index) => index === 1 ? { ...bucket, from: '2026-10-22T09:01:00Z' } : bucket) } },
      { trend: { status: 'READY', data: base.trend.data.map((bucket, index) => index === 0 ? { ...bucket, total: 0, failed: 1 } : bucket) } },
    ]
    for (const change of changes) expect(() => parseOverviewSnapshot({ ...base, ...change }, '24h')).toThrow()
  })

  it('全部分区未知的成功响应不得覆盖最后可信快照', () => {
    const value = { ...overviewFixture(at), metrics: { status: 'UNAVAILABLE' }, trend: { status: 'UNAVAILABLE' }, nodes: { status: 'RESTRICTED' }, myTasks: { status: 'UNAVAILABLE' }, attention: { status: 'UNAVAILABLE' } }
    expect(() => parseOverviewSnapshot(value, '24h')).toThrow()
  })

  it('只读传输不接收主体、节点路径或执行参数', async () => {
    const read = vi.fn(async () => ({ item: overviewFixture(at, '7d') }))
    const result = await createOverviewApi({ read }).getOverview('7d')
    expect(read).toHaveBeenCalledWith('/api/v1/overview?range=7d')
    expect(result.range).toBe('7d')
  })
})
