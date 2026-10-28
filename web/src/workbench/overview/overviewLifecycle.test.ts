import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { OverviewSnapshot } from '@/api/overview'
import { overviewFixture } from '../../../tests/fixtures/overview'
import { useOverview } from './useOverview'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers() })
async function settle() { for (let index = 0; index < 25; index++) await Promise.resolve() }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup(read: (range: '24h' | '7d') => Promise<OverviewSnapshot>, autoRefreshMs = 0) {
  const scope = effectScope(); scopes.push(scope)
  const signals: AbortSignal[] = []
  const getOverview = vi.fn(read)
  const state = scope.run(() => useOverview({ createApi: signal => { signals.push(signal); return { getOverview } }, autoRefreshMs }))!
  return { scope, state, getOverview, signals }
}

describe('首页快照生命周期', () => {
  it('下钻入口确认失权时同步清全页快照、停止计时并隔离在途刷新', async () => {
    vi.useFakeTimers()
    const model = setup(async () => overviewFixture(), 1000); await settle()
    const pending = deferred<OverviewSnapshot>(); model.getOverview.mockImplementationOnce(() => pending.promise)
    void model.state.refresh(); model.state.invalidateAuthorization()
    expect(model.state.snapshot.value).toBeNull(); expect(model.state.restricted.value).toBe(true)
    expect(model.signals.at(-1)?.aborted).toBe(true)
    pending.resolve(overviewFixture()); await settle(); await vi.advanceTimersByTimeAsync(5000)
    expect(model.state.snapshot.value).toBeNull(); expect(model.getOverview).toHaveBeenCalledTimes(2)
  })
  it('方法入口防重，刷新失败保留整份快照且部分结果不混入旧分区', async () => {
    const pending = deferred<OverviewSnapshot>()
    const first = overviewFixture()
    const model = setup(async () => first)
    await settle()
    model.getOverview.mockImplementationOnce(() => pending.promise)
    void model.state.refresh(); void model.state.refresh()
    expect(model.getOverview).toHaveBeenCalledTimes(2)
    pending.reject({ code: 'NETWORK_UNAVAILABLE' }); await settle()
    expect(model.state.snapshot.value).toBe(first)
    expect(model.state.failure.value).not.toBe('')
    const partial: OverviewSnapshot = { ...overviewFixture(), trend: { status: 'UNAVAILABLE' } }
    model.getOverview.mockResolvedValueOnce(partial)
    await model.state.refresh()
    expect(model.state.snapshot.value?.trend.status).toBe('UNAVAILABLE')
    expect(model.state.partial.value).toBe(true)
  })

  it('A → B → A 隔离迟到成功、失败与 finally，不恢复旧快照', async () => {
    const first = deferred<OverviewSnapshot>(), second = deferred<OverviewSnapshot>(), third = deferred<OverviewSnapshot>()
    const model = setup(() => first.promise)
    model.getOverview.mockImplementationOnce(() => second.promise)
    model.state.range.value = '7d'
    model.getOverview.mockImplementationOnce(() => third.promise)
    model.state.range.value = '24h'
    first.resolve(overviewFixture()); second.reject(new Error('synthetic late failure')); await settle()
    expect(model.signals[0]?.aborted).toBe(true); expect(model.signals[1]?.aborted).toBe(true)
    expect(model.state.snapshot.value).toBeNull(); expect(model.state.loading.value).toBe(true); expect(model.state.failure.value).toBe('')
    const latest = overviewFixture()
    third.resolve(latest); await settle()
    expect(model.state.snapshot.value).toBe(latest); expect(model.state.loading.value).toBe(false)
  })

  it.each([401, 403])('授权失效 %s 立即清引用并停止自动读取', async status => {
    vi.useFakeTimers()
    const model = setup(async () => overviewFixture(), 1000)
    await settle()
    model.getOverview.mockRejectedValueOnce({ status })
    await model.state.refresh()
    expect(model.state.snapshot.value).toBeNull(); expect(model.state.restricted.value).toBe(true)
    await vi.advanceTimersByTimeAsync(5000)
    expect(model.getOverview).toHaveBeenCalledTimes(2)
  })

  it('自动刷新串行、到期标记旧快照且卸载拒绝迟到响应', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-23T08:30:00Z'))
    const initial = overviewFixture()
    const pending = deferred<OverviewSnapshot>()
    const model = setup(async () => initial, 1000)
    await settle()
    model.getOverview.mockImplementation(() => pending.promise)
    await vi.advanceTimersByTimeAsync(95000)
    expect(model.getOverview).toHaveBeenCalledTimes(2)
    expect(model.state.stale.value).toBe(true)
    model.scope.stop(); pending.resolve(overviewFixture()); await settle()
    expect(model.signals.at(-1)?.aborted).toBe(true)
    expect(model.state.snapshot.value).toBeNull(); expect(vi.getTimerCount()).toBe(0)
  })
})
