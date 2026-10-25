import { describe, expect, it, vi } from 'vitest'
import { createBrowserApi, type FetchLike } from './browser'

function fixture(body: unknown) {
  const fetcher = vi.fn<FetchLike>(async () => Response.json(body))
  return { fetcher, api: createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-idempotency-key' }) }
}
const time = '2026-10-19T00:00:00Z'
const draft = { id: 'normal-draft', type: 'OBLOADER_NORMAL_IMPORT', displayName: '合成普通导入', revision: 1, dataSourceId: 'synthetic-source', nodeId: '', createdAt: time, updatedAt: time, availableActions: ['CONTINUE_EDIT', 'DELETE'] }
const template = { id: 'normal-template', taskType: 'OBLOADER_NORMAL_IMPORT', displayName: '合成普通导入模板', capabilityVersion: 'import-odp-local-v1', configFingerprint: 'a'.repeat(64), revision: 1, createdAt: time, updatedAt: time }

describe('任务复用的类型与响应边界', () => {
  it('统一草稿保留三类类型，旧导出读取拒绝导入与未知类型', async () => {
    const f = fixture({ items: [draft, { ...draft, id: 'export-draft', type: 'OBDUMPER_EXPORT' }, { ...draft, id:'direct-draft',type:'OBLOADER_DIRECT_LOAD' }], totalPages: 1, nextCursor: '' })
    expect((await f.api.listTaskDrafts(10)).items.map(item => item.type)).toEqual(['OBLOADER_NORMAL_IMPORT', 'OBDUMPER_EXPORT', 'OBLOADER_DIRECT_LOAD'])
    expect(String(f.fetcher.mock.calls[0]![0])).toContain('/api/v1/task-drafts?limit=10')
    await expect(fixture({ items: [draft], totalPages: 1 }).api.listExportDrafts(10)).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    await expect(fixture({ items: [{ ...draft, type: 'UNKNOWN' }], totalPages: 1 }).api.listTaskDrafts(10)).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('旁路任务派生沿用原事务，响应必须绑定来源、类型与操作', async () => {
    const response={draftId:'direct-derived',sourceTaskId:'direct-parent',derivation:'RERUN_FROM_SCRATCH',taskType:'OBLOADER_DIRECT_LOAD'}
    const f=fixture(response);await expect(f.api.rebuildTaskDraft('direct-parent','RERUN_FROM_SCRATCH')).resolves.toEqual(response)
    expect(String(f.fetcher.mock.calls[0]![0])).toBe('/api/v1/tasks/direct-parent:rebuild-draft')
    for(const invalid of [{...response,sourceTaskId:'other-parent'},{...response,derivation:'REBUILD_FROM_CONFIG'},{...response,taskType:'FUTURE_IMPORT'}])await expect(fixture(invalid).api.rebuildTaskDraft('direct-parent','RERUN_FROM_SCRATCH')).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('模板列表要求显式已支持类型，响应白名单不持有完整配置', async () => {
    const f = fixture({ items: [{ ...template, config: { synthetic: 'excluded' } }] })
    const items = await f.api.listConfigTemplates()
    expect(items[0]).toMatchObject({ taskType: 'OBLOADER_NORMAL_IMPORT' }); expect(items[0]).not.toHaveProperty('config')
    await expect(fixture({ items: [{ ...template, taskType: 'OBLOADER_DIRECT_LOAD' }] }).api.listConfigTemplates()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('模板派生响应必须匹配模板及类型，绑定与安全头不被路由改写', async () => {
    const f = fixture({ draftId: 'normal-created', templateId: 'normal-template', taskType: 'OBLOADER_NORMAL_IMPORT' })
    await expect(f.api.createTaskDraftFromTemplate('normal-template', 'source', 'node', 'OBLOADER_NORMAL_IMPORT')).resolves.toEqual({ draftId: 'normal-created', taskType: 'OBLOADER_NORMAL_IMPORT' })
    const [url, init] = f.fetcher.mock.calls[0]!
    expect(String(url)).toContain('/api/v1/config-templates/normal-template:create-draft')
    expect(JSON.parse(String(init?.body))).toEqual({ dataSourceId: 'source', nodeId: 'node' })
    const headers = new Headers(init?.headers)
    expect(headers.get('X-CSRF-Token')).toBe('synthetic-csrf'); expect(headers.get('Idempotency-Key')).toBeTruthy()
    await expect(fixture({ draftId: 'wrong', templateId: 'normal-template', taskType: 'OBDUMPER_EXPORT' }).api.createTaskDraftFromTemplate('normal-template', 'source', 'node', 'OBLOADER_NORMAL_IMPORT')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
