import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import type { TaskFilterOptions, TaskListFilter, TaskListItem, TaskListPage } from '@/api/browser'
import { parseTaskListRoute, taskListApiFilter, taskListConditionChips, taskListDateRangeError, taskListRouteQuery } from './taskListQuery'
import { resetTaskListCursorMemory, useTaskList, type TaskListApi } from './useTaskList'
import { useTaskListActions, type TaskListActionApi } from './useTaskListActions'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const time = '2026-01-01T00:00:00Z'
function task(id: string, overrides: Partial<TaskListItem> = {}): TaskListItem {
  return {
    id, type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', state: 'SUCCEEDED', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE',
    reconciliationRequired: false, nodeId: 'node-1', ownedByCurrentUser: true, submittedAt: time, updatedAt: time,
    displayName: id, dataSourceDeleted: false, nodeDeleted: false, cancelRequested: false, availableActions: [], ...overrides,
  }
}
const page = (items: TaskListItem[], nextCursor?: string): TaskListPage => ({ items, nextCursor, totalPages: 2 })
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers(); resetTaskListCursorMemory() })
async function settle() { for (let i = 0; i < 20; i++) await Promise.resolve() }

function setup(listTasks: TaskListApi['listTasks'], query: LocationQuery = {}) {
  const route = ref<LocationQuery>(query)
  const replaced: LocationQueryRaw[] = []
  const api: TaskListApi = { listTasks: vi.fn(listTasks), listTaskFilterOptions: vi.fn(async (): Promise<TaskFilterOptions> => ({ dataSources: [], nodes: [], creators: [], environments: [], creatorFilterAvailable: false })) }
  const scope = effectScope()
  scopes.push(scope)
  const list = scope.run(() => useTaskList(route, next => { replaced.push(next); route.value = next as LocationQuery }, { createApi: () => api, autoRefreshMs: 10_000 }))!
  return { route, replaced, api, list, scope }
}

describe('任务列表路由条件', () => {
  it('解析丢弃未知值，规范化输出稳定且默认值不入 URL', () => {
    const state = parseTaskListRoute({ state: 'FAILED,DRAFT,RUNNING,FAILED', type: 'FUTURE_IMPORT', environment: 'LOCAL', limit: '30', createdFrom: '2026-13-40', keyword: '  orders ' })
    expect(state.states).toEqual(['FAILED', 'RUNNING'])
    expect(state.types).toEqual([])
    expect(state.environment).toBe('')
    expect(state.limit).toBe(10)
    expect(state.createdFrom).toBe('')
    expect(taskListRouteQuery(state)).toEqual({ keyword: 'orders', state: 'FAILED,RUNNING' })
  })

  it('结束日期包含当天，倒置区间不发请求', () => {
    const state = parseTaskListRoute({ createdFrom: '2026-10-01', createdTo: '2026-10-01' })
    const filter: TaskListFilter = taskListApiFilter(state)
    expect(Date.parse(filter.createdTo!) - Date.parse(filter.createdFrom!)).toBe(24 * 3600 * 1000)
    expect(taskListDateRangeError(parseTaskListRoute({ createdFrom: '2026-10-02', createdTo: '2026-10-01' }))).not.toBe('')
  })

  it('节点页带入的条件出现在摘要中且可单独移除', () => {
    const chips = taskListConditionChips(parseTaskListRoute({ nodeId: 'node-1', scope: 'MINE' }), (_kind, id) => `名称-${id}`)
    expect(chips.map(chip => chip.label)).toEqual(['执行节点：名称-node-1', '范围：本人创建'])
    expect(chips[0]!.clear).toEqual({ nodeId: '' })
  })
})

describe('任务列表读取生命周期', () => {
  it('条件变化使迟到响应失效，只接纳最新条件', async () => {
    const first = deferred<TaskListPage>()
    const s = setup(async (_cursor, _limit, filter) => filter?.keyword === 'a' ? first.promise : page([task('task-b')]), { keyword: 'a' })
    s.route.value = { keyword: 'b' }
    await settle()
    expect(s.list.tasks.value.map(item => item.id)).toEqual(['task-b'])
    first.resolve(page([task('late-a')]))
    await settle()
    expect(s.list.tasks.value.map(item => item.id)).toEqual(['task-b'])
  })

  it('翻页使用游标栈，条件变化回到首页，A → B → A 恢复已访问页', async () => {
    const calls: Array<{ cursor?: string; keyword?: string }> = []
    const s = setup(async (cursor, _limit, filter) => { calls.push({ cursor, keyword: filter?.keyword }); return page([task(`t-${cursor ?? 'first'}`)], 'next-1') }, { keyword: 'a' })
    await settle()
    s.list.goToNextPage()
    await settle()
    expect(s.list.pageIndex.value).toBe(1)
    expect(calls.at(-1)).toEqual({ cursor: 'next-1', keyword: 'a' })
    s.list.updateFilters({ keyword: 'b' })
    await settle()
    expect(s.list.pageIndex.value).toBe(0)
    expect(calls.at(-1)).toEqual({ cursor: undefined, keyword: 'b' })
    s.list.updateFilters({ keyword: 'a' })
    await settle()
    expect(s.list.pageIndex.value).toBe(1)
    expect(calls.at(-1)).toEqual({ cursor: 'next-1', keyword: 'a' })
  })

  it('改变每页条数写入路由并回到首页', async () => {
    const s = setup(async () => page([task('t')], 'next'))
    await settle()
    s.list.goToNextPage()
    await settle()
    s.list.changePageSize(20)
    await settle()
    expect(s.replaced.at(-1)).toEqual({ limit: '20' })
    expect(s.list.pageIndex.value).toBe(0)
  })

  it('刷新失败保留可信旧行；授权失效清空并停止自动刷新', async () => {
    vi.useFakeTimers()
    let mode: 'ok' | 'fail' | 'forbidden' = 'ok'
    const s = setup(async () => {
      if (mode === 'fail') throw Object.assign(new Error('x'), { status: 503 })
      if (mode === 'forbidden') throw Object.assign(new Error('x'), { status: 403 })
      return page([task('running', { state: 'RUNNING' })])
    })
    await settle()
    mode = 'fail'
    s.list.refresh()
    await settle()
    expect(s.list.tasks.value).toHaveLength(1)
    expect(s.list.failure.value).not.toBe('')
    mode = 'forbidden'
    await vi.advanceTimersByTimeAsync(10_000)
    await settle()
    expect(s.list.tasks.value).toHaveLength(0)
    expect(s.list.restricted.value).toBe(true)
    const callsAfterForbidden = vi.mocked(s.api.listTasks).mock.calls.length
    await vi.advanceTimersByTimeAsync(30_000)
    expect(vi.mocked(s.api.listTasks).mock.calls.length).toBe(callsAfterForbidden)
  })

  it('只有存在非终态任务时自动刷新，卸载后不再请求', async () => {
    vi.useFakeTimers()
    let state = 'RUNNING'
    const s = setup(async () => page([task('t', { state })]))
    await settle()
    expect(s.api.listTasks).toHaveBeenCalledTimes(1)
    state = 'SUCCEEDED'
    await vi.advanceTimersByTimeAsync(10_000)
    await settle()
    expect(s.api.listTasks).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(s.api.listTasks).toHaveBeenCalledTimes(2)
    state = 'RUNNING'
    s.list.refresh()
    await settle()
    s.scope.stop()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(s.api.listTasks).toHaveBeenCalledTimes(3)
  })

  it('写事务期间在途读取失效，结束后复读', async () => {
    const held = deferred<TaskListPage>()
    let calls = 0
    const s = setup(async () => { calls++; return calls === 2 ? held.promise : page([task(`t-${calls}`)]) })
    await settle()
    s.list.refresh()
    s.list.beginMutation()
    held.resolve(page([task('stale')]))
    await settle()
    expect(s.list.tasks.value.map(item => item.id)).toEqual(['t-1'])
    s.list.endMutation(true)
    await settle()
    expect(s.list.tasks.value.map(item => item.id)).toEqual(['t-3'])
  })
})

describe('任务列表行操作', () => {
  function actionSetup(overrides: Partial<TaskListActionApi> = {}) {
    const api: TaskListActionApi = {
      cancelTask: vi.fn(async (taskId: string) => ({ taskId, state: 'CANCELLING', cancelRequested: true })),
      rebuildTaskDraft: vi.fn(async (taskId: string, derivation: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH') => ({ draftId: `draft-${taskId}`, sourceTaskId: taskId, derivation })),
      resumeTaskFromCheckpoint: vi.fn(async (taskId: string) => ({ id: `resume-${taskId}`, parentTaskId: taskId, derivationKind: 'CHECKPOINT_RESUME' as const })),
      saveTaskTemplate: vi.fn(async () => 'template-1'),
      getTaskCommandEvidence: vi.fn(async () => ({ kind: 'PLANNED' as const, command: 'obdumper -p ******', redaction: 'PASSWORD_ONLY' as const })),
      ...overrides,
    }
    const navigate = vi.fn()
    const begin = vi.fn()
    const end = vi.fn()
    const copied: string[] = []
    const scope = effectScope()
    scopes.push(scope)
    const actions = scope.run(() => useTaskListActions({ navigate, beginMutation: begin, endMutation: end, copyText: async value => { copied.push(value) }, createApi: () => api }))!
    return { api, navigate, begin, end, copied, actions, scope }
  }

  it('没有服务端资格的操作不会打开确认或发请求', () => {
    const s = actionSetup()
    s.actions.request(task('t'), 'CANCEL')
    expect(s.actions.confirmation.value).toBeNull()
  })

  it('取消经确认后提交，期间防重，结束后复读列表', async () => {
    const held = deferred<{ taskId: string; state: string; cancelRequested: boolean }>()
    const s = actionSetup({ cancelTask: vi.fn(() => held.promise) })
    const target = task('t', { state: 'RUNNING', availableActions: ['CANCEL'] })
    s.actions.request(target, 'CANCEL')
    expect(s.actions.confirmation.value?.kind).toBe('CANCEL')
    void s.actions.confirm()
    void s.actions.confirm()
    expect(s.api.cancelTask).toHaveBeenCalledTimes(1)
    expect(s.begin).toHaveBeenCalledTimes(1)
    held.resolve({ taskId: 't', state: 'CANCELLING', cancelRequested: true })
    await settle()
    expect(s.actions.notice.value).toBe('已提交取消请求。')
    expect(s.end).toHaveBeenCalledWith(true)
    expect(s.actions.confirmation.value).toBeNull()
  })

  it('派生成功跳转到共享目标，失败保留确认并复读', async () => {
    const s = actionSetup()
    s.actions.request(task('t', { state: 'FAILED', availableActions: ['RERUN_FROM_SCRATCH'] }), 'RERUN_FROM_SCRATCH')
    await s.actions.confirm()
    expect(s.navigate).toHaveBeenCalledWith({ path: '/exports/new', query: { draft: 'draft-t', step: '5' } })
    const failing = actionSetup({ resumeTaskFromCheckpoint: vi.fn(async () => { throw new Error('x') }) })
    failing.actions.request(task('t', { state: 'FAILED', availableActions: ['CHECKPOINT_RESUME'] }), 'CHECKPOINT_RESUME')
    await failing.actions.confirm()
    expect(failing.navigate).not.toHaveBeenCalled()
    expect(failing.actions.failure.value).not.toBe('')
    expect(failing.actions.confirmation.value?.kind).toBe('CHECKPOINT_RESUME')
    expect(failing.end).toHaveBeenCalledWith(true)
  })

  it('卸载后迟到写响应不导航、不发布反馈', async () => {
    const held = deferred<{ draftId: string; sourceTaskId: string; derivation: 'REBUILD_FROM_CONFIG' }>()
    const s = actionSetup({ rebuildTaskDraft: vi.fn(() => held.promise) })
    s.actions.request(task('t', { state: 'FAILED', availableActions: ['REBUILD_FROM_CONFIG'] }), 'REBUILD_FROM_CONFIG')
    void s.actions.confirm()
    s.scope.stop()
    held.resolve({ draftId: 'd', sourceTaskId: 't', derivation: 'REBUILD_FROM_CONFIG' })
    await settle()
    expect(s.navigate).not.toHaveBeenCalled()
    expect(s.actions.notice.value).toBe('')
  })

  it('检查点继续成功导航到新任务，字符串路径不作为提示文本', async () => {
    const s = actionSetup({ resumeTaskFromCheckpoint: vi.fn(async () => ({ id: 'new-task', parentTaskId: 't', derivationKind: 'CHECKPOINT_RESUME' as const })) })
    s.actions.request(task('t', { state: 'FAILED', availableActions: ['CHECKPOINT_RESUME'] }), 'CHECKPOINT_RESUME')
    await s.actions.confirm()
    expect(s.navigate).toHaveBeenCalledWith('/tasks/new-task')
    expect(s.actions.notice.value).toBe('')
  })

  it('复制只复制服务端脱敏命令；保存模板需要名称', async () => {
    const s = actionSetup()
    s.actions.request(task('t', { availableActions: ['COPY_COMMAND'] }), 'COPY_COMMAND')
    await settle()
    expect(s.copied).toEqual(['obdumper -p ******'])
    expect(s.actions.notice.value).toBe('已复制脱敏命令。')
    s.actions.request(task('t', { state: 'SUCCEEDED', availableActions: ['SAVE_TEMPLATE'] }), 'SAVE_TEMPLATE')
    await s.actions.confirm()
    expect(s.api.saveTaskTemplate).not.toHaveBeenCalled()
    s.actions.templateName.value = ' 合成模板 '
    await s.actions.confirm()
    expect(s.api.saveTaskTemplate).toHaveBeenCalledWith('t', '合成模板')
  })
})
