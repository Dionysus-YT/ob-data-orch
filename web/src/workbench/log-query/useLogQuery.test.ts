import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { LogContext, LogPage, LogQueryApi } from '@/api/logQuery'
import { defaultLogFilter, logFilterError, useLogQuery } from './useLogQuery'
import { logHighlights } from './presentation'

const item = () => ({ token: 'record', taskId: 'task-1', source: 'OBDUMPER_STDOUT', streamId: 'a'.repeat(64), epoch: 1, sequence: 1, kind: 'LOG' as const, message: 'safe', integrityCode: '', receivedAt: '2026-10-25T00:00:00Z' })
function page(): LogPage { return { items: [item()], nextCursor: 'next', pageCursor: 'first', snapshotToken: 'snapshot', snapshotAt: '2026-10-25T00:00:01Z', sources: ['OBDUMPER_STDOUT'], availability: 'RETAINED', expired: false, incomplete: false, truncated: false, scannedRecords: 1, scannedBytes: 200 } }
function context(): LogContext { return { items: [item()], selectedSequence: 1, snapshotAt: '2026-10-25T00:00:01Z', incomplete: false } }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
const scopes: ReturnType<typeof effectScope>[] = []; afterEach(() => scopes.splice(0).forEach(scope => scope.stop()))
function setup() {
  const scope = effectScope(); scopes.push(scope)
  const api = { queryTaskLogs: vi.fn<LogQueryApi['queryTaskLogs']>().mockResolvedValue(page()), getLogContext: vi.fn<LogQueryApi['getLogContext']>().mockResolvedValue(context()), copyLogContext: vi.fn<LogQueryApi['copyLogContext']>().mockResolvedValue('safe context'), downloadTaskLogs: vi.fn<LogQueryApi['downloadTaskLogs']>().mockResolvedValue(new Blob(['safe'])) }
  const effects = { copy: vi.fn<(text: string) => Promise<void>>().mockResolvedValue(), save: vi.fn() }, state = scope.run(() => useLogQuery(api, effects))!
  return { scope, api, effects, state }
}
describe('日志查询会话与写事务边界', () => {
  it('首次不查询，方法防重并传递信号', async () => {
    const { state, api } = setup(), pending = deferred<LogPage>(); expect(api.queryTaskLogs).not.toHaveBeenCalled(); api.queryTaskLogs.mockReturnValueOnce(pending.promise)
    const first = state.run(); await state.run(); expect(api.queryTaskLogs).toHaveBeenCalledTimes(1); expect(api.queryTaskLogs.mock.calls[0]![2]).toBeInstanceOf(AbortSignal); pending.resolve(page()); await first; expect(state.page.value).toEqual(page())
  })
  it('输入变化、A B A 与迟到 finally 不恢复旧结果', async () => {
    const { state, api } = setup(), old = deferred<LogPage>(), current = deferred<LogPage>(); api.queryTaskLogs.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const first = state.run(); state.filters.taskId = 'task-b'; state.filters.taskId = ''; const second = state.run(); old.resolve(page()); await first; expect(state.loading.value).toBe(true); expect(state.page.value).toBeUndefined(); current.resolve(page()); await second; expect(state.loading.value).toBe(false)
  })
  it('下一页和上一页始终用服务端签发的同快照游标', async () => {
    const { state, api } = setup(); await state.run(); api.queryTaskLogs.mockResolvedValueOnce({ ...page(), pageCursor: 'second', nextCursor: '' }); await state.run('next'); expect(state.history.value).toEqual(['first']); await state.run('previous'); expect(api.queryTaskLogs.mock.calls.at(-1)![1]).toBe('first'); expect(state.history.value).toEqual([])
  })
  it('失败保留旧快照，撤权和失效游标清除旧事实', async () => {
    const { state, api } = setup(); await state.run(); api.queryTaskLogs.mockRejectedValueOnce({ status: 503 }); await state.run('refresh'); expect(state.page.value).toEqual(page()); expect(state.dirty.value).toBe(false); expect(state.loading.value).toBe(false)
    api.queryTaskLogs.mockRejectedValueOnce({ status: 404 }); await state.run('refresh'); expect(state.page.value).toBeUndefined(); expect(state.failure.value).toContain('权限')
    await state.run(); api.queryTaskLogs.mockRejectedValueOnce({ status: 409, code: 'LOG_QUERY_SNAPSHOT_INVALID' }); await state.run('next'); expect(state.page.value).toBeUndefined(); expect(state.history.value).toEqual([])
  })
  it('上下文关闭或切换后的迟到响应不恢复旧抽屉', async () => {
    const { state, api } = setup(); await state.run(); const old = deferred<LogContext>(); api.getLogContext.mockReturnValueOnce(old.promise); const pending = state.openContext(item()); state.closeContext(); await state.openContext(item()); old.resolve({ ...context(), selectedSequence: 99 }); await pending; expect(state.context.value?.selectedSequence).toBe(1)
  })
  it('当前上下文响应必须匹配所选任务、来源和序号', async () => {
    const { state, api } = setup(); await state.run(); api.getLogContext.mockResolvedValueOnce({ ...context(), items: [{ ...item(), taskId: 'other-task' }] }); await state.openContext(item()); expect(state.context.value).toBeUndefined(); expect(state.contextFailure.value).toContain('无法读取')
  })
  it('重复下载防重，改条件及卸载取消下载和成功提示', async () => {
    const { state, api, effects, scope } = setup(); await state.run(); state.requestDownload(); const old = deferred<Blob>(); api.downloadTaskLogs.mockReturnValueOnce(old.promise); const pending = state.download(); await state.download(); expect(api.downloadTaskLogs).toHaveBeenCalledTimes(1); state.filters.keyword = 'changed'; old.resolve(new Blob(['old'])); await pending; expect(effects.save).not.toHaveBeenCalled(); expect(state.notice.value).toBe('')
    await state.run(); state.requestDownload(); const late = deferred<Blob>(); api.downloadTaskLogs.mockReturnValueOnce(late.promise); const run = state.download(); scope.stop(); late.resolve(new Blob(['late'])); await run; expect(effects.save).not.toHaveBeenCalled()
  })
  it('批量复制先走审计接口，关闭后迟到内容不能进入剪贴板', async () => {
    const { state, api, effects } = setup(); await state.run(); await state.openContext(item()); const old = deferred<string>(); api.copyLogContext.mockReturnValueOnce(old.promise); const pending = state.copyContext(); state.closeContext(); old.resolve('late'); await pending; expect(effects.copy).not.toHaveBeenCalled(); expect(state.notice.value).toBe('')
    await state.openContext(item()); await state.copyContext(); expect(api.copyLogContext).toHaveBeenCalledTimes(2); expect(effects.copy).toHaveBeenCalledWith('safe context')
  })
  it('下载失败保留确认范围，截断页不提供下载', async () => {
    const { state, api } = setup(); await state.run(); state.requestDownload(); api.downloadTaskLogs.mockRejectedValueOnce({ code: 'LOG_DOWNLOAD_LIMIT' }); await state.download(); expect(state.downloadOpen.value).toBe(true); expect(state.actionFailure.value).toContain('100000')
    api.queryTaskLogs.mockResolvedValueOnce({ ...page(), truncated: true, nextCursor: '' }); await state.run(); expect(state.canDownload.value).toBe(false)
  })
})
describe('日志条件与安全文本呈现', () => {
  it('只允许有效时间，跨任务 24 小时与精确任务长范围', () => { const f = defaultLogFilter(); expect(logFilterError(f)).toBe(''); expect(logFilterError({ ...f, from: new Date(Date.parse(f.to) - 86400001).toISOString() })).toContain('24'); expect(logFilterError({ ...f, taskId: 'task-1', from: '2025-01-01T00:00:00Z' })).toBe(''); expect(logFilterError({ ...f, taskId: '../x' })).toContain('任务 ID') })
  it('高亮只是字面片段，不执行脚本或构造正则', () => { const input = '<script>a[b]+</script>'; expect(logHighlights(input, '[b]+')).toEqual([{ text: '<script>a', hit: false }, { text: '[b]+', hit: true }, { text: '</script>', hit: false }]); expect(logHighlights(input, '')).toEqual([{ text: input, hit: false }]) })
})
