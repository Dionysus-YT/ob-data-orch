import { describe, expect, it, vi } from 'vitest'
import { createAuditLogApi, parseAuditItem, parseAuditPage } from './auditLogs'

const row = () => ({ token: 'record-token', id: 'event-1', actorType: 'SUBJECT', actorId: 'user-1', action: 'DATA_SOURCE_UPDATED', objectType: 'DATA_SOURCE', objectId: 'source-1', result: 'SUCCEEDED', requestId: 'request-1', occurredAt: '2026-10-25T01:00:00Z', fields: [{ name: 'revision', value: '2' }], detailsOmitted: true })
const page = () => ({ items: [row()], nextCursor: '', pageCursor: 'page-token', snapshotToken: 'snapshot-token', snapshotAt: '2026-10-25T01:00:01Z', scannedRecords: 1, scannedBytes: 500, truncated: false })
describe('审计严格传输', () => {
 it('仅接受固定安全投影，不接收原始差异或错误类型', () => {
  expect(parseAuditPage(page()).items).toHaveLength(1)
  for (const value of [{ ...row(), safeDiff: { password: 'synthetic-only' } }, { ...row(), actorType: 'OTHER' }, { ...row(), objectType: 'UNREGISTERED' }, { ...row(), occurredAt: '2026-02-30T00:00:00Z' }, { ...row(), fields: [{ name: 'password', value: 'synthetic-only' }] }, { ...row(), fields: [{ name: 'revision', value: '-1' }] }, { ...row(), fields: [{ name: 'changed', value: 'synthetic-only' }] }, { ...row(), fields: [{ name: 'revision', value: '9007199254740992' }] }]) expect(() => parseAuditItem(value)).toThrow()
  for (const value of [{ ...page(), scannedRecords: 0 }, { ...page(), scannedRecords: 50001 }, { ...page(), items: [row(), row()] }, { ...page(), truncated: true, nextCursor: 'next' }]) expect(() => parseAuditPage(value)).toThrow()
 })
 it('固定路径、精确筛选与取消信号透传，不传操作者身份', async () => {
  const read = vi.fn<(path: string, signal?: AbortSignal) => Promise<Record<string, unknown>>>(async () => ({ item: page(), allowed: true })), download = vi.fn(async () => new Blob(['已脱敏'])), api = createAuditLogApi({ read, download }), controller = new AbortController()
  const f = { from: '2026-10-25T00:00:00Z', to: '2026-10-25T01:00:00Z', actorId: 'user-1', objectType: '', objectId: '', action: '', result: '', keyword: 'a&b', limit: 200 }
  expect(await api.getAuditAccess(controller.signal)).toBe(true); await api.queryAuditLogs(f, 'page-token', controller.signal)
  const path = String(read.mock.calls.at(-1)?.[0]), url = new URL(path, 'https://example.test'); expect(url.pathname).toBe('/api/v1/logs/audit'); expect(url.searchParams.get('keyword')).toBe('a&b'); expect(read.mock.calls.at(-1)?.[1]).toBe(controller.signal)
  await api.downloadAuditLogs('snapshot-token', controller.signal); expect(download).toHaveBeenCalledWith('/api/v1/logs/audit:download', { token: 'snapshot-token' }, controller.signal)
 })
})
