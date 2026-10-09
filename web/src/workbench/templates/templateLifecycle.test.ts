import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { DataSourceSummary, ExportConfigTemplateItem } from '@/api/browser'
import { useTemplateCatalog } from './useTemplateCatalog'
import { useTemplateActions } from './useTemplateActions'
import { useTemplateDraft } from './useTemplateDraft'
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
const item = (id = 'synthetic-template'): ExportConfigTemplateItem => ({ id, displayName: id, revision: 1, capabilityVersion: 'synthetic-capability', configFingerprint: 'synthetic-fingerprint', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' })
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function setup() {
  const api = { listExportConfigTemplates: vi.fn(async () => [item()]), renameExportConfigTemplate: vi.fn(async () => 2), deleteExportConfigTemplate: vi.fn(async () => {}), listDataSources: vi.fn(async (): Promise<DataSourceSummary[]> => []), listExportNodeCandidates: vi.fn(async () => []), createDraftFromTemplate: vi.fn(async () => 'synthetic-draft') }
  const navigate = vi.fn(async () => {}); const scope = effectScope(); scopes.push(scope)
  const value = scope.run(() => { const catalog = useTemplateCatalog(() => api); return { catalog, actions: useTemplateActions(catalog), draft: useTemplateDraft(catalog, navigate) } })!
  return { ...value, api, navigate, scope }
}
describe('模板事实、编辑与派生生命周期', () => {
  it.each([401, 403])('列表 %s 同步清除改名和删除会话，授权恢复不复活旧操作', async status => {
    const { catalog, actions, draft, api } = setup(); await catalog.refresh()
    actions.startRename(item()); actions.renameValue.value = '合成未保存改名'; actions.requestDelete(item())
    api.listExportConfigTemplates.mockRejectedValueOnce({ status }); await catalog.refresh()
    expect(catalog.templates.value).toEqual([]); expect(actions.renameID.value).toBe(''); expect(actions.renameValue.value).toBe(''); expect(actions.pendingDelete.value).toBeUndefined()
    const attemptStaleActions = async () => {
      actions.startRename(item()); actions.requestDelete(item()); await actions.confirmRename(item()); await actions.confirmDelete()
      draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'; await draft.createDraft(item())
      expect(actions.renameID.value).toBe(''); expect(actions.renameValue.value).toBe(''); expect(actions.pendingDelete.value).toBeUndefined()
      expect(api.renameExportConfigTemplate).not.toHaveBeenCalled(); expect(api.deleteExportConfigTemplate).not.toHaveBeenCalled(); expect(api.createDraftFromTemplate).not.toHaveBeenCalled()
    }
    await attemptStaleActions()
    api.listExportConfigTemplates.mockRejectedValueOnce({ status: 503 }); await catalog.refresh(); await attemptStaleActions()
    await catalog.refresh(); await actions.confirmRename(item()); await actions.confirmDelete()
    expect(catalog.accessDenied.value).toBe(false); expect(actions.renameValue.value).toBe(''); expect(actions.pendingDelete.value).toBeUndefined()
    expect(api.renameExportConfigTemplate).not.toHaveBeenCalled(); expect(api.deleteExportConfigTemplate).not.toHaveBeenCalled()
    actions.startRename(catalog.templates.value[0]!); actions.renameValue.value = '合成新名'; await actions.confirmRename(item())
    expect(api.renameExportConfigTemplate).toHaveBeenCalledExactlyOnceWith(item().id, 1, '合成新名')
    actions.requestDelete(catalog.templates.value[0]!); await actions.confirmDelete()
    expect(api.deleteExportConfigTemplate).toHaveBeenCalledExactlyOnceWith(item().id, 2)
  })
  it('普通列表故障保留输入和确认，过期授权拒绝不能清除最新会话', async () => {
    const { catalog, actions, api } = setup(); await catalog.refresh(); actions.startRename(item()); actions.renameValue.value = '合成未保存改名'; actions.requestDelete(item())
    api.listExportConfigTemplates.mockRejectedValueOnce({ status: 503 }); await catalog.refresh()
    expect(actions.renameValue.value).toBe('合成未保存改名'); expect(actions.pendingDelete.value).toEqual(item())
    const old = deferred<ExportConfigTemplateItem[]>(); api.listExportConfigTemplates.mockImplementationOnce(() => old.promise)
    const pending = catalog.refresh(); await catalog.refresh(); old.reject({ status: 401 }); await pending
    expect(catalog.accessDenied.value).toBe(false); expect(actions.renameValue.value).toBe('合成未保存改名'); expect(actions.pendingDelete.value).toEqual(item()); expect(catalog.templates.value).toEqual([item()])
  })
  it('最新列表胜出，写入前失效旧读取；改名裁剪且保持修订', async () => {
    const { catalog, actions, api } = setup(); await catalog.refresh(); const read = deferred<ExportConfigTemplateItem[]>(); api.listExportConfigTemplates.mockImplementationOnce(() => read.promise)
    const pending = catalog.refresh(); actions.startRename(item()); actions.renameValue.value = ' 合成新名 '; await actions.confirmRename(item()); read.resolve([item()]); await pending
    expect(catalog.templates.value[0]?.revision).toBe(2); expect(catalog.templates.value[0]?.displayName).toBe('合成新名'); expect(api.renameExportConfigTemplate).toHaveBeenCalledWith(item().id, 1, '合成新名')
  })
  it('引用加载只有一个在途请求，空成功可缓存，半份失败不能发布并可重试', async () => {
    const { draft, api } = setup(); const read = deferred<DataSourceSummary[]>(); api.listDataSources.mockImplementationOnce(() => read.promise)
    const pending = draft.loadChoices(); const duplicate = draft.loadChoices(); expect(api.listDataSources).toHaveBeenCalledTimes(1); read.reject({ status: 503 }); await Promise.all([pending, duplicate]); expect(draft.nodes.value).toHaveLength(0)
    await draft.loadChoices(); await draft.loadChoices(); expect(api.listDataSources).toHaveBeenCalledTimes(2); expect(draft.sourcesLoading.value).toBe(false)
  })
  it('未绑定对象不创建；草稿方法防重，捕获源和节点并保持导航回调', async () => {
    const { catalog, draft, api, navigate } = setup(); await draft.createDraft(item()); expect(api.createDraftFromTemplate).not.toHaveBeenCalled()
    draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'; const write = deferred<string>(); api.createDraftFromTemplate.mockImplementationOnce(() => write.promise)
    const pending = draft.createDraft(item()); await draft.createDraft(item()); expect(api.createDraftFromTemplate).toHaveBeenCalledTimes(1); draft.draftNodeID.value = 'synthetic-other'; write.resolve('synthetic-created'); await pending
    expect(api.createDraftFromTemplate).toHaveBeenCalledWith(item().id, 'synthetic-source', 'synthetic-node'); expect(navigate).toHaveBeenCalledWith('synthetic-created'); expect(catalog.actionBusy.value).toBe('')
  })
  it('改名与删除共享方法锁，取消后的迟到反馈不得进入新输入', async () => {
    const { catalog, actions, api } = setup(); await catalog.refresh(); const write = deferred<number>(); api.renameExportConfigTemplate.mockImplementationOnce(() => write.promise)
    actions.startRename(item()); actions.renameValue.value = '合成新名'; const pending = actions.confirmRename(item()); await actions.confirmRename(item()); actions.requestDelete(item()); await actions.confirmDelete(); expect(api.renameExportConfigTemplate).toHaveBeenCalledTimes(1); expect(api.deleteExportConfigTemplate).not.toHaveBeenCalled()
    actions.cancelRename(); write.reject({ status: 409 }); await pending; expect(catalog.loadFailure.value).toBe(''); actions.requestDelete(item()); await actions.confirmDelete(); expect(catalog.templates.value).toHaveLength(0)
  })
  it.each(['read', 'choices', 'draft', 'rename', 'delete'] as const)('卸载后 %s 不回写、不导航', async kind => {
    const { catalog, actions, draft, api, navigate, scope } = setup(); await catalog.refresh()
    const response = deferred<ExportConfigTemplateItem[]>(); const choices = deferred<DataSourceSummary[]>(); const write = deferred<string>(); const rename = deferred<number>(); const deletion = deferred<void>(); let pending: Promise<void>
    if (kind === 'read') { api.listExportConfigTemplates.mockImplementationOnce(() => response.promise); pending = catalog.refresh() }
    else if (kind === 'choices') { api.listDataSources.mockImplementationOnce(() => choices.promise); pending = draft.loadChoices() }
    else if (kind === 'draft') { api.createDraftFromTemplate.mockImplementationOnce(() => write.promise); draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'; pending = draft.createDraft(item()) }
    else if (kind === 'rename') { api.renameExportConfigTemplate.mockImplementationOnce(() => rename.promise); actions.startRename(item()); actions.renameValue.value = '合成新名'; pending = actions.confirmRename(item()) }
    else { api.deleteExportConfigTemplate.mockImplementationOnce(() => deletion.promise); actions.requestDelete(item()); pending = actions.confirmDelete() }
    scope.stop(); const previous = catalog.templates.value
    if (kind === 'read') response.resolve([]); else if (kind === 'choices') choices.reject({ status: 503 }); else if (kind === 'draft') write.resolve('synthetic-late'); else if (kind === 'rename') rename.resolve(2); else deletion.reject({ status: 409 })
    await pending; expect(catalog.templates.value).toBe(previous); expect(catalog.notice.value).toBe(''); expect(catalog.loadFailure.value).toBe(''); expect(draft.createDraftFailure.value).toBe(''); expect(navigate).not.toHaveBeenCalled()
  })
})
