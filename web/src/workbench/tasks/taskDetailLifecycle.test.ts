import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import type { BrowserApi, TaskCommandEvidence, TaskExecution, TaskLog, TaskLogPage, TaskOverview, TaskSnapshot } from '@/api/browser'
import { useTaskDetail } from './useTaskDetail'
import { useTaskActions } from './useTaskActions'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const time = '2026-01-01T00:00:00Z'
const overview = (id: string): TaskOverview => ({ id, type: 'OBDUMPER_EXPORT', dataSourceId: `source-${id}`, nodeId: `node-${id}`, precheckId: `precheck-${id}`, submittedAt: time })
const execution = (id: string, state = 'RUNNING'): TaskExecution => ({ state, executionId: id, reconciliationRequired: false, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', updatedAt: time })
const snapshot = (id: string): TaskSnapshot => ({ type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: id, nodeId: id, precheckId: id, format: 'CSV', configFingerprint: id, toolVersion: '4.3.5', metadataVersion: id, capabilityVersion: id })
const command = (id: string): TaskCommandEvidence => ({ kind: 'PLANNED', command: id, redaction: 'PASSWORD_ONLY' })
const log = (message: string): TaskLog => ({ sourceSeq: 1, receivedAt: time, kind: 'STDOUT', integrityCode: 'COMPLETE', message })
const logPage = (id: string): TaskLogPage => ({ items: [log(id)], lastReliableCursor: `cursor-${id}`, integrity: 'COMPLETE' })
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers() })
async function settle() { for (let i = 0; i < 20; i++) await Promise.resolve() }

function setup(overrides: Partial<BrowserApi> = {}) {
  vi.useFakeTimers()
  const id = ref('A')
  const streams: Array<{ id: string; record: (record: TaskLog, cursor?: string) => void; disconnect: () => void; close: ReturnType<typeof vi.fn> }> = []
  const api = {
    getTaskOverview: vi.fn(async (id: string) => overview(id)),
    getTaskExecution: vi.fn(async (id: string) => execution(id)),
    getTaskSnapshot: vi.fn(async (id: string) => snapshot(id)),
    getTaskCommandEvidence: vi.fn(async (id: string) => command(id)),
    getTaskLogs: vi.fn(async (id: string) => logPage(id)),
    streamTaskLogs: vi.fn((id, _cursor, record, disconnect) => {
      const stream = { id, record, disconnect, close: vi.fn() }
      streams.push(stream)
      return stream
    }),
    rebuildTaskDraft: vi.fn(async (id, derivation) => ({ draftId: `draft-${id}`, sourceTaskId: id, derivation })),
    resumeTaskFromCheckpoint: vi.fn(async id => ({ id: `resume-${id}`, parentTaskId: id, derivationKind: 'CHECKPOINT_RESUME' as const })),
    saveTaskTemplate: vi.fn(async () => 'synthetic-template'),
    ...overrides,
  } satisfies Partial<BrowserApi>
  const signals: AbortSignal[] = []
  const createApi = vi.fn((signal: AbortSignal) => { signals.push(signal); return api as BrowserApi })
  const router = { push: vi.fn(async () => {}) }
  const scope = effectScope()
  scopes.push(scope)
  const state = scope.run(() => {
    const detail = useTaskDetail(id, createApi)
    return { ...detail, ...useTaskActions(detail.session, router, detail.execution) }
  })!
  return { id, state, api, scope, router, streams, signals }
}

describe('任务详情会话与读取隔离', () => {
  it('同一实例 A → B 立即清空事实、游标、输入和反馈，只接纳 B', async () => {
    const s = setup()
    await settle()
    s.state.templateNameInput.value = 'A 的输入'
    s.state.noticeTemplate.value = 'A 的反馈'
    s.id.value = 'B'
    expect(s.state.overview.value).toBeNull()
    expect(s.state.snapshot.value).toBeNull()
    expect(s.state.commandEvidence.value).toBeNull()
    expect(s.state.logs.value).toEqual([])
    expect(s.state.logCursor.value).toBeUndefined()
    expect(s.state.templateNameInput.value).toBe('')
    expect(s.state.noticeTemplate.value).toBe('')
    expect(s.signals[0]!.aborted).toBe(true)
    expect(s.streams[0]!.close).toHaveBeenCalledOnce()
    await settle()
    expect(s.state.overview.value?.id).toBe('B')
    expect(s.state.snapshot.value?.dataSourceId).toBe('B')
    expect(s.state.commandEvidence.value?.command).toBe('B')
    expect(s.state.logs.value.map(l => l.message)).toEqual(['B'])
    expect(s.api.getTaskLogs).toHaveBeenLastCalledWith('B', undefined, undefined)
  })

  it('A 概览迟到成功不得覆盖 B，也不得开始 A 的后续读取或轮询', async () => {
    const late = deferred<TaskOverview>()
    const s = setup({ getTaskOverview: vi.fn(id => id === 'A' ? late.promise : Promise.resolve(overview(id))) })
    s.id.value = 'B'
    await settle()
    late.resolve(overview('A'))
    await settle()
    expect(s.state.overview.value?.id).toBe('B')
    expect(s.api.getTaskSnapshot).toHaveBeenCalledExactlyOnceWith('B')
    expect(vi.getTimerCount()).toBe(1)
  })

  it.each(['execution', 'snapshot', 'command', 'logs'] as const)('A 的 %s 迟到结果、错误与 finally 均不影响 B', async kind => {
    const late = deferred<never>()
    const key = { execution: 'getTaskExecution', snapshot: 'getTaskSnapshot', command: 'getTaskCommandEvidence', logs: 'getTaskLogs' }[kind] as keyof BrowserApi
    const defaults = { execution, snapshot, command, logs: logPage }
    const s = setup({ [key]: vi.fn((id: string) => id === 'A' ? late.promise : Promise.resolve(defaults[kind](id))) })
    await settle()
    s.id.value = 'B'
    await settle()
    late.reject(new Error('合成迟到错误'))
    await settle()
    expect(s.state.overview.value?.id).toBe('B')
    expect([s.state.failure.value, s.state.executionFailure.value, s.state.snapshotFailure.value, s.state.commandFailure.value, s.state.logFailure.value]).toEqual(['', '', '', '', ''])
    expect(s.state.loading.value).toBe(false)
    expect(vi.getTimerCount()).toBe(1)
  })

  it('A 四类详情成功响应迟到，不能覆盖 B 的任一事实、游标或流', async () => {
    const e = deferred<TaskExecution>()
    const sn = deferred<TaskSnapshot>()
    const c = deferred<TaskCommandEvidence>()
    const l = deferred<TaskLogPage>()
    const s = setup({
      getTaskExecution: vi.fn(id => id === 'A' ? e.promise : Promise.resolve(execution(id))),
      getTaskSnapshot: vi.fn(id => id === 'A' ? sn.promise : Promise.resolve(snapshot(id))),
      getTaskCommandEvidence: vi.fn(id => id === 'A' ? c.promise : Promise.resolve(command(id))),
      getTaskLogs: vi.fn(id => id === 'A' ? l.promise : Promise.resolve(logPage(id))),
    })
    await settle()
    s.id.value = 'B'
    await settle()
    e.resolve(execution('A', 'FAILED'))
    sn.resolve(snapshot('A'))
    c.resolve(command('A'))
    l.resolve(logPage('A'))
    await settle()
    expect(s.state.execution.value?.executionId).toBe('B')
    expect(s.state.snapshot.value?.dataSourceId).toBe('B')
    expect(s.state.commandEvidence.value?.command).toBe('B')
    expect(s.state.logs.value.map(l => l.message)).toEqual(['B'])
    expect(s.state.logCursor.value).toBe('cursor-B')
    expect(s.streams.map(stream => stream.id)).toEqual(['B'])
  })

  it('A → B → A 不接纳第一次 A 的请求，即使 ID 相同', async () => {
    const late = deferred<TaskOverview>()
    let first = true
    const s = setup({ getTaskOverview: vi.fn(id => {
      if (first) { first = false; return late.promise }
      return Promise.resolve(overview(id))
    }) })
    s.id.value = 'B'
    s.id.value = 'A'
    await settle()
    late.resolve({ ...overview('A'), nodeId: 'stale-node' })
    await settle()
    expect(s.state.overview.value?.nodeId).toBe('node-A')
    expect(s.signals.filter(signal => signal.aborted)).toHaveLength(2)
  })

  it('概览失败结束 loading，重试可读取并只创建一个轮询链', async () => {
    const s = setup({ getTaskOverview: vi.fn().mockRejectedValueOnce(new Error('合成失败')).mockResolvedValue(overview('A')) })
    await settle()
    expect(s.state.loading.value).toBe(false)
    expect(s.state.failure.value).not.toBe('')
    expect(vi.getTimerCount()).toBe(0)
    await s.state.refresh()
    expect(s.state.failure.value).toBe('')
    expect(s.state.overview.value?.id).toBe('A')
    expect(vi.getTimerCount()).toBe(1)
  })

  it('重复刷新和单项重试复用在途请求；手动刷新不叠加轮询或流', async () => {
    const late = deferred<TaskExecution>()
    const s = setup({ getTaskExecution: vi.fn(() => late.promise) })
    await settle()
    void s.state.refresh()
    void s.state.refresh()
    void s.state.loadExecution()
    expect(s.api.getTaskExecution).toHaveBeenCalledTimes(1)
    late.resolve(execution('A'))
    await settle()
    await s.state.refresh()
    await s.state.refresh()
    expect(vi.getTimerCount()).toBe(1)
    expect(s.streams).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(2000)
    expect(s.api.getTaskExecution).toHaveBeenCalledTimes(4)
    expect(s.api.getTaskSnapshot).toHaveBeenCalledTimes(1)
    expect(s.streams).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(1)
  })

  it('卸载中止 HTTP，释放轮询与流，迟到结果不得再写状态', async () => {
    const late = deferred<TaskExecution>()
    const s = setup()
    await settle()
    const before = s.state.execution.value
    s.api.getTaskExecution = vi.fn(() => late.promise)
    void s.state.refresh()
    await settle()
    s.scope.stop()
    expect(s.signals[0]!.aborted).toBe(true)
    expect(s.streams[0]!.close).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
    late.resolve(execution('stale'))
    s.streams[0]!.record(log('stale log'))
    s.streams[0]!.disconnect()
    await settle()
    expect(s.state.execution.value).toEqual(before)
    expect(s.state.logs.value.map(l => l.message)).toEqual(['A'])
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('任务日志的会话与连接代隔离', () => {
  it('A 迟到事件与断开回调不能进入或关闭 B 的流', async () => {
    const s = setup()
    await settle()
    const old = s.streams[0]!
    s.id.value = 'B'
    await settle()
    old.record(log('late-A'), 'cursor-late-A')
    old.disconnect()
    expect(s.state.logs.value.map(l => l.message)).toEqual(['B'])
    expect(s.state.logCursor.value).toBe('cursor-B')
    expect(s.streams[1]!.close).not.toHaveBeenCalled()
    expect(s.state.logStreamStatus.value).toBe('connected')
  })

  it('断开后可靠游标重连，旧连接事件不能污染同任务新流，重复日志去重', async () => {
    const s = setup()
    await settle()
    const old = s.streams[0]!
    old.record(log('live-A'), 'cursor-live-A')
    old.disconnect()
    old.record(log('late-disconnected'))
    expect(s.state.logStreamStatus.value).toBe('interrupted')
    await s.state.refresh()
    expect(s.api.getTaskLogs).toHaveBeenLastCalledWith('A', undefined, 'cursor-live-A')
    old.record(log('late-replaced'))
    s.streams[1]!.record(log('live-A'), 'cursor-live-A')
    expect(s.state.logs.value.map(l => l.message)).toEqual(['A', 'live-A'])
    expect(s.streams).toHaveLength(2)
  })

  it('流已推进可靠游标时，迟到的 HTTP 补读不得回退游标', async () => {
    const s = setup()
    await settle()
    const late = deferred<TaskLogPage>()
    s.api.getTaskLogs = vi.fn(() => late.promise)
    void s.state.loadLogs()
    s.streams[0]!.record(log('new-live'), 'new-cursor')
    late.resolve({ ...logPage('old-http'), lastReliableCursor: 'old-cursor' })
    await settle()
    expect(s.state.logCursor.value).toBe('new-cursor')
  })

  it('终态关闭实时流，迟到事件拒绝，后续轮询不再建流', async () => {
    const s = setup()
    await settle()
    s.api.getTaskExecution = vi.fn(async () => execution('A', 'SUCCEEDED'))
    await s.state.refresh()
    expect(s.streams[0]!.close).toHaveBeenCalledOnce()
    s.streams[0]!.record(log('after-terminal'))
    await s.state.refresh()
    expect(s.state.logs.value.map(l => l.message)).toEqual(['A'])
    expect(s.state.logStreamStatus.value).toBe('idle')
    expect(s.streams).toHaveLength(1)
  })
})

describe('任务派生与模板操作', () => {
  it('检查点按钮只在失败且明确报告检查点时可用，不从缺省结果推断资格', async () => {
    const s = setup()
    await settle()
    expect(s.state.checkpointResumeAvailable.value).toBe(false)
    s.state.execution.value = execution('A', 'FAILED')
    expect(s.state.checkpointResumeAvailable.value).toBe(false)
    const resultSummary = { result: 'FAILED' as const, fileCount: 0, totalBytes: 0, files: [], checkpointPresent: true, observedAt: time }
    s.state.execution.value = { ...execution('A', 'FAILED'), resultSummary }
    expect(s.state.checkpointResumeAvailable.value).toBe(true)
    s.state.execution.value = { ...execution('A', 'SUCCEEDED'), resultSummary }
    expect(s.state.checkpointResumeAvailable.value).toBe(false)
  })
  it.each(['REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH'] as const)('%s 保留草稿来源及目标步骤，方法入口防重', async kind => {
    const late = deferred<Awaited<ReturnType<BrowserApi['rebuildTaskDraft']>>>()
    const s = setup({ rebuildTaskDraft: vi.fn(() => late.promise) })
    await settle()
    const operation = s.state.rebuildFromTask(kind)
    await s.state.rebuildFromTask(kind)
    expect(s.api.rebuildTaskDraft).toHaveBeenCalledExactlyOnceWith('A', kind)
    late.resolve({ draftId: 'derived-draft', sourceTaskId: 'A', derivation: kind })
    await operation
    expect(s.router.push).toHaveBeenCalledExactlyOnceWith({ path: '/exports/new', query: { draft: 'derived-draft', step: kind === 'RERUN_FROM_SCRATCH' ? '5' : '1' } })
    expect(s.state.derivationBusy.value).toBe('')
  })

  it('检查点继续保留新任务导航，模板保存裁剪输入且清空已完成输入', async () => {
    const s = setup()
    await settle()
    await s.state.resumeFromCheckpoint()
    expect(s.api.resumeTaskFromCheckpoint).toHaveBeenCalledExactlyOnceWith('A')
    expect(s.router.push).toHaveBeenLastCalledWith('/tasks/resume-A')
    s.state.templateNameInput.value = '  合成模板  '
    await s.state.saveAsTemplate()
    expect(s.api.saveTaskTemplate).toHaveBeenCalledExactlyOnceWith('A', '合成模板')
    expect(s.state.templateNameInput.value).toBe('')
    expect(s.state.noticeTemplate.value).toContain('模板已保存')
  })

  it.each(['rebuild', 'resume', 'template'] as const)('%s 在路由变化或卸载后完成不得跳转、提示或清空 B 输入', async kind => {
    const late = deferred<never>()
    const key = { rebuild: 'rebuildTaskDraft', resume: 'resumeTaskFromCheckpoint', template: 'saveTaskTemplate' }[kind]
    const s = setup({ [key]: vi.fn(() => late.promise) })
    await settle()
    s.state.templateNameInput.value = 'A 输入'
    const pending = kind === 'rebuild' ? s.state.rebuildFromTask('REBUILD_FROM_CONFIG') : kind === 'resume' ? s.state.resumeFromCheckpoint() : s.state.saveAsTemplate()
    s.id.value = 'B'
    await settle()
    s.state.templateNameInput.value = 'B 输入'
    late.resolve((kind === 'template' ? 'synthetic-template' : { draftId: 'late-draft', id: 'late-resume' }) as never)
    await pending
    expect(s.router.push).not.toHaveBeenCalled()
    expect(s.state.templateNameInput.value).toBe('B 输入')
    expect(s.state.noticeTemplate.value).toBe('')
    expect(s.state.derivationFailure.value).toBe('')
    s.scope.stop()
  })

  it('过期写操作的 catch/finally 不覆盖 B 的在途操作；卸载后写成功不导航', async () => {
    const a = deferred<never>()
    const b = deferred<never>()
    const s = setup({ rebuildTaskDraft: vi.fn(id => id === 'A' ? a.promise : b.promise) })
    await settle()
    const first = s.state.rebuildFromTask('REBUILD_FROM_CONFIG')
    s.id.value = 'B'
    await settle()
    const second = s.state.rebuildFromTask('RERUN_FROM_SCRATCH')
    a.reject(new Error('合成旧操作失败'))
    await first
    expect(s.state.derivationBusy.value).toBe('RERUN_FROM_SCRATCH')
    expect(s.state.derivationFailure.value).toBe('')
    s.scope.stop()
    b.resolve({ draftId: 'late' } as never)
    await second
    expect(s.router.push).not.toHaveBeenCalled()
  })
})
