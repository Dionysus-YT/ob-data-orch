import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { TaskListItem, TaskListPage } from '@/api/browser'
import { resetTaskListCursorMemory, type TaskListApi } from '@/workbench/tasks/useTaskList'
import { overviewFixture } from '../../../tests/fixtures/overview'
import type { OverviewTrendSession } from './overviewTrend'
import { useOverviewAnalysis } from './useOverviewAnalysis'

const snapshot = overviewFixture(new Date('2026-10-28T08:30:00Z'))
const buckets = snapshot.trend.status === 'READY' ? snapshot.trend.data : []
const session: OverviewTrendSession = { id: 1, mode: 'analysis', range: '24h', snapshotAt: snapshot.snapshotAt, buckets }
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); resetTaskListCursorMemory() })
async function settle() { for (let index = 0; index < 30; index++) await Promise.resolve() }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function task(id: string): TaskListItem {
  return { id, displayName: id, type: 'OBDUMPER_EXPORT', state: 'FAILED', dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: false, ownedByCurrentUser: true, submittedAt: snapshot.snapshotAt, updatedAt: snapshot.snapshotAt, dataSourceDeleted: false, nodeDeleted: false, cancelRequested: false, availableActions: [] }
}
function page(id: string, nextCursor?: string): TaskListPage { return { items: [task(id)], totalPages: 2, nextCursor } }
function setup(read: TaskListApi['listTasks'], input = session) {
  const scope = effectScope(); scopes.push(scope)
  const signals: AbortSignal[] = [], listTasks = vi.fn(read)
  const model = scope.run(() => useOverviewAnalysis(input, signal => { signals.push(signal); return { listTasks, listTaskFilterOptions: vi.fn() } }))!
  return { model, scope, listTasks, signals }
}

describe('首页分析弹窗与既有任务读取绑定', () => {
  it('只按捕获窗口读取，状态和时间段变化收窄服务端筛选且不加载无关候选', async () => {
    const value = setup(async () => page('current'))
    await settle()
    expect(value.listTasks.mock.calls[0]?.[2]).toMatchObject({ finishedFrom: buckets[0]!.from, finishedTo: buckets.at(-1)!.to })
    value.model.state.value = 'FAILED'; value.model.selectedFrom.value = buckets[2]!.from; await settle()
    expect(value.listTasks.mock.calls.at(-1)?.[2]).toMatchObject({ states: ['FAILED'], finishedFrom: buckets[2]!.from, finishedTo: buckets[2]!.to })
  })
  it('A → B → A 的迟到成功和错误不恢复旧行、错误或忙状态', async () => {
    const first = deferred<TaskListPage>(), second = deferred<TaskListPage>(), third = deferred<TaskListPage>()
    const value = setup(() => first.promise)
    value.listTasks.mockImplementationOnce(() => second.promise); value.model.state.value = 'FAILED'
    value.listTasks.mockImplementationOnce(() => third.promise); value.model.state.value = ''
    third.resolve(page('latest')); await settle()
    first.resolve(page('late-a')); second.reject({ status: 503 }); await settle()
    expect(value.model.list.tasks.value.map(item => item.id)).toEqual(['latest'])
    expect(value.model.list.failure.value).toBe(''); expect(value.model.list.loading.value).toBe(false)
    expect(value.signals[0]?.aborted).toBe(true); expect(value.signals[1]?.aborted).toBe(true)
  })
  it('关闭卸载取消等待，迟到结果不能补回已关闭弹窗', async () => {
    const pending = deferred<TaskListPage>(), value = setup(() => pending.promise)
    value.scope.stop(); pending.resolve(page('late')); await settle()
    expect(value.signals[0]?.aborted).toBe(true); expect(value.model.list.tasks.value).toEqual([])
  })
  it('非法窗口不退化为无筛选列表，也不继续旧请求', async () => {
    const pending = deferred<TaskListPage>(), value = setup(() => pending.promise)
    value.model.selectedFrom.value = 'synthetic-forged-window'; await settle()
    expect(value.model.valid.value).toBe(false); expect(value.listTasks).toHaveBeenCalledTimes(1)
    expect(value.signals[0]?.aborted).toBe(true)
    pending.resolve(page('late')); await settle(); expect(value.model.list.tasks.value).toEqual([])
  })
  it.each([401, 403])('失权 %s 清除旧任务，不把权限失败表达为空结果', async status => {
    const value = setup(async () => page('visible')); await settle()
    value.listTasks.mockRejectedValueOnce({ status }); value.model.list.refresh(); await settle()
    expect(value.model.list.restricted.value).toBe(true); expect(value.model.list.tasks.value).toEqual([])
    expect(value.model.list.failure.value).not.toBe('')
  })
  it('游标分页继续绑定同一状态和时间桶，重复翻页只创建一次读取', async () => {
    const pending = deferred<TaskListPage>(), value = setup(async () => page('first', 'synthetic-next'))
    await settle(); value.listTasks.mockImplementationOnce(() => pending.promise)
    value.model.list.goToNextPage(); value.model.list.goToNextPage()
    expect(value.listTasks).toHaveBeenCalledTimes(2)
    expect(value.listTasks.mock.calls.at(-1)?.[0]).toBe('synthetic-next')
    expect(value.listTasks.mock.calls.at(-1)?.[2]?.finishedFrom).toBe(buckets[0]!.from)
    pending.resolve(page('second')); await settle(); expect(value.model.list.pageIndex.value).toBe(1)
  })
})
