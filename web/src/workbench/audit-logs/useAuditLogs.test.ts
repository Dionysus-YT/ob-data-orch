import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuditItem, AuditLogApi, AuditPage } from '@/api/auditLogs'
import { useLogPresets } from '@/workbench/log-query/useLogPresets'
import { auditFilterErrors, defaultAuditFilter, useAuditLogs } from './useAuditLogs'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers() })
function row(id = 'event-a'): AuditItem { return { token: `token-${id}`, id, actorType: 'SUBJECT', actorId: 'actor-1', action: 'DATA_SOURCE_UPDATED', objectType: 'DATA_SOURCE', objectId: 'source-1', result: 'SUCCEEDED', requestId: 'request-1', occurredAt: '2026-10-25T01:00:00Z', fields: [], detailsOmitted: true } }
function page(id = 'event-a', cursor = 'page-a'): AuditPage { return { items: [row(id)], pageCursor: cursor, nextCursor: 'next-b', snapshotToken: 'snapshot-a', snapshotAt: '2026-10-25T01:00:01Z', scannedRecords: 1, scannedBytes: 400, truncated: false } }
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (v: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup() {
 const api: AuditLogApi = { getAuditAccess: vi.fn(async () => true), queryAuditLogs: vi.fn(async () => page()), getAuditDetail: vi.fn(async () => row()), downloadAuditLogs: vi.fn(async () => new Blob(['已脱敏'])) }, scope = effectScope(); scopes.push(scope)
 const save = vi.fn(), denied = vi.fn(), state = scope.run(() => useAuditLogs(api, { save, denied }))!; return { api, state, save, denied, scope }
}
describe('审计会话与撤权', () => {
 it('首次不宽查，分页使用同一快照并在方法入口防重', async () => {
  const { state, api } = setup(); expect(api.queryAuditLogs).not.toHaveBeenCalled(); const pending = deferred<AuditPage>(); vi.mocked(api.queryAuditLogs).mockReturnValueOnce(pending.promise)
  const first = state.run(); await state.run(); expect(api.queryAuditLogs).toHaveBeenCalledTimes(1); pending.resolve(page()); await first
  await state.run('next'); expect(vi.mocked(api.queryAuditLogs).mock.calls[1]?.[1]).toBe('next-b'); expect(state.history.value).toEqual(['page-a']); await state.run('previous'); expect(vi.mocked(api.queryAuditLogs).mock.calls[2]?.[1]).toBe('page-a')
 })
 it('A → B → A 的旧查询成功和 finally 不能恢复结果或覆盖忙碌状态', async () => {
  const { state, api } = setup(), a = deferred<AuditPage>(), b = deferred<AuditPage>(); vi.mocked(api.queryAuditLogs).mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)
  const first = state.run(); state.filters.actorId = 'actor-b'; state.filters.actorId = ''; const second = state.run(); a.resolve(page('old-event')); await first; expect(state.page.value).toBeUndefined(); expect(state.loading.value).toBe(true); b.resolve(page('current-event')); await second; expect(state.page.value?.items[0]?.id).toBe('current-event')
 })
 it('详情切换、关闭及不匹配的响应不能恢复旧对象', async () => {
  const { state, api } = setup(); await state.run(); const first = deferred<AuditItem>(), second = deferred<AuditItem>(); vi.mocked(api.getAuditDetail).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
  const a = state.openDetail(row()), b = state.openDetail(row('event-b')); first.resolve(row()); await a; expect(state.detail.value).toBeUndefined(); expect(state.detailLoading.value).toBe(true); second.resolve(row('event-b')); await b; expect(state.detail.value?.id).toBe('event-b')
  vi.mocked(api.getAuditDetail).mockResolvedValueOnce(row('wrong-event')); await state.openDetail(row()); expect(state.page.value).toBeUndefined(); expect(state.detail.value).toBeUndefined()
 })
 it.each([401, 404, 409, 503])('读取失败 %s 撤下旧事实并保留筛选', async status => {
  const { state, api, denied } = setup(); state.filters.actorId = 'actor-1'; await state.run(); vi.mocked(api.queryAuditLogs).mockRejectedValueOnce({ status }); await state.run('refresh'); expect(state.page.value).toBeUndefined(); expect(state.filters.actorId).toBe('actor-1'); expect(state.failure.value).not.toBe(''); expect(denied).toHaveBeenCalledTimes([401, 404].includes(status) ? 1 : 0)
 })
 it('下载确认后防重，切换和卸载丢弃迟到文件与成功提示', async () => {
  const { state, api, save, scope } = setup(); await state.run(); state.requestDownload(); const pending = deferred<Blob>(); vi.mocked(api.downloadAuditLogs).mockReturnValueOnce(pending.promise)
  const first = state.download(); await state.download(); expect(api.downloadAuditLogs).toHaveBeenCalledTimes(1); state.filters.objectId = 'source-b'; pending.resolve(new Blob(['old'])); await first; expect(save).not.toHaveBeenCalled(); expect(state.notice.value).toBe('')
  await state.run(); state.requestDownload(); const late = deferred<Blob>(); vi.mocked(api.downloadAuditLogs).mockReturnValueOnce(late.promise); const second = state.download(); scope.stop(); late.resolve(new Blob(['late'])); await second; expect(save).not.toHaveBeenCalled(); expect(state.page.value).toBeUndefined()
 })
 it('详情关闭及卸载丢弃迟到失败', async () => {
  const { state, api, scope } = setup(); await state.run(); const pending = deferred<AuditItem>(); vi.mocked(api.getAuditDetail).mockReturnValueOnce(pending.promise); const work = state.openDetail(row()); state.closeDetail(); scope.stop(); pending.reject({ status: 404 }); await work; expect(state.detailFailure.value).toBe(''); expect(state.detail.value).toBeUndefined()
 })
 it('失效和截断结果不能打开下载，输入校验覆盖时间、标识与秘密探测关键字', async () => {
  const { state, api } = setup(); vi.mocked(api.queryAuditLogs).mockResolvedValueOnce({ ...page(), truncated: true, nextCursor: '' }); await state.run(); state.requestDownload(); expect(state.downloadOpen.value).toBe(false)
  const f = defaultAuditFilter(); expect(auditFilterErrors({ ...f, from: f.to }).time).toBeTruthy(); expect(auditFilterErrors({ ...f, actorId: '*' }).actorId).toBeTruthy(); expect(auditFilterErrors({ ...f, keyword: '\nsecret' }).keyword).toBeTruthy()
 })
 it('审计入口资格迟到或撤销不能重新显示预设', async () => {
  const scope = effectScope(); scopes.push(scope); const a = deferred<boolean>(), b = deferred<boolean>(), getAuditAccess = vi.fn().mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise), state = scope.run(() => useLogPresets({ getAuditAccess }))!
  const first = state.load(), second = state.load(); a.resolve(true); await first; expect(state.allowed.value).toBe(false); b.resolve(true); await second; expect(state.allowed.value).toBe(true); state.revoke(); expect(state.allowed.value).toBe(false); expect(state.failure.value).not.toBe(''); scope.stop()
 })
})
