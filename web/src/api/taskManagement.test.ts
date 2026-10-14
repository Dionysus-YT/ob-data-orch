import { describe, expect, it } from 'vitest'
import { createBrowserApi } from './browser'
import { parseTaskExecutionFacts } from './taskExecutionFacts'

describe('任务管理响应与写请求边界', () => {
  it('执行事实拒绝隐藏路径、未知摘要和无效数值，不透传原始证据', () => {
    expect(parseTaskExecutionFacts({ rawPayload: 'hidden', command: 'hidden', outputLocation: { kind: 'UNAVAILABLE', redaction: 'UNAVAILABLE' } })).toEqual({ outputLocation: { kind: 'UNAVAILABLE', redaction: 'UNAVAILABLE' } })
    for (const invalid of [{ outputLocation: { kind: 'LOCAL', redaction: 'KIND_ONLY', value: '/hidden' } }, { plannedActualMatch: 'PENDING' }, { elapsedMs: -1 }, { exitCode: 1.5 }, { cancelAvailable: 'true' }, { outputLocation: { kind: 'UNAVAILABLE', redaction: 'FULL' } }]) expect(() => parseTaskExecutionFacts(invalid)).toThrow()
  })
  it('草稿列表白名单裁剪配置，删除携带 CSRF、版本和幂等键', async () => {
    const calls: { path: string; init?: RequestInit }[] = []
    const draft = { id: 'synthetic-draft', type: 'OBDUMPER_EXPORT', displayName: '合成草稿', dataSourceId: 'source', nodeId: 'node', revision: 2, createdAt: '2026-10-14T00:00:00Z', updatedAt: '2026-10-14T00:00:00Z', availableActions: ['DELETE'], config: { credential: 'hidden' }, path: '/hidden' }
    const api = createBrowserApi({ csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-idempotency', fetcher: async (path, init) => { calls.push({ path: String(path), init }); return Response.json(init?.method === 'DELETE' ? { deleted: true } : { items: [draft], totalPages: 1 }) } })
    const page = await api.listExportDrafts(10, 'orders', 'synthetic-cursor'); expect(page.items[0]).not.toHaveProperty('config'); expect(page.items[0]).not.toHaveProperty('path')
    await api.deleteExportDraft(draft.id, draft.revision)
    expect(calls[0]?.path).toBe('/api/v1/export-drafts?limit=10&keyword=orders&cursor=synthetic-cursor')
    expect(calls[1]?.init?.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf', 'If-Match': '"rev-2"', 'Idempotency-Key': 'synthetic-idempotency' })
  })
})
