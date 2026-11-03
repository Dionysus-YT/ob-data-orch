import { describe, expect, it, vi } from 'vitest'
import { createBrowserApi } from './browser'

const summary = { id: 'template', taskType: 'OBDUMPER_EXPORT', displayName: '合成模板', revision: 1, capabilityVersion: 'synthetic', configFingerprint: 'synthetic', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }
function setup(body: unknown) {
  const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify(body), { status: 200 }))
  const key = vi.fn(() => 'synthetic-fallback-key')
  return { fetcher, key, api: createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf', idempotencyKey: key }) }
}
describe('模板接口的安全摘要与写入身份', () => {
  it('缺少动作事实保持只读，配置原文不进入摘要', async () => {
    const { api } = setup({ items: [{ ...summary, config: { filePath: 'synthetic-path' } }] })
    const values = await api.listConfigTemplates()
    expect(values[0]?.availableActions).toEqual([])
    expect(values[0]).not.toHaveProperty('config')
  })
  it.each([{ availableActions: ['RUN_TOOL'] }, { availableActions: ['RENAME', 'RENAME'] }])('未知或重复的操作资格失败关闭：%j', async ({ availableActions }) => {
    const { api } = setup({ items: [{ ...summary, availableActions }] })
    await expect(api.listConfigTemplates()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('草稿创建和模板保存显式沿用调用者的原请求键', async () => {
    const draft = setup({ templateId: 'template', taskType: 'OBDUMPER_EXPORT', draftId: 'created' })
    await draft.api.createTaskDraftFromTemplate('template', 'source', 'node', 'OBDUMPER_EXPORT', 'synthetic-original-key')
    expect(new Headers(draft.fetcher.mock.calls[0]?.[1]?.headers).get('Idempotency-Key')).toBe('synthetic-original-key')
    expect(draft.key).not.toHaveBeenCalled()
    const save = setup({ sourceTaskId: 'task', id: 'saved' })
    await save.api.saveTaskTemplate('task', '合成模板', 'synthetic-save-key')
    expect(new Headers(save.fetcher.mock.calls[0]?.[1]?.headers).get('Idempotency-Key')).toBe('synthetic-save-key')
    expect(save.key).not.toHaveBeenCalled()
  })
  it('不匹配的草稿类型不能被正常结果接纳', async () => {
    const { api } = setup({ templateId: 'template', taskType: 'OBLOADER_NORMAL_IMPORT', draftId: 'created' })
    await expect(api.createTaskDraftFromTemplate('template', 'source', 'node', 'OBDUMPER_EXPORT')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
