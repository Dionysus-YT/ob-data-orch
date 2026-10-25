import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { DataSourceSummary, ExecutionNodeCandidate, ConfigTemplateItem } from '@/api/browser'
import { DATA_SOURCE_UI_FIXTURES } from '@/workbench/sources/dataSourceUiFixture'
import { useTemplateCatalog } from './useTemplateCatalog'
import { useTemplateActions } from './useTemplateActions'
import { useTemplateDraft } from './useTemplateDraft'
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
const item = (id = 'synthetic-template'): ConfigTemplateItem => ({ id, taskType: 'OBDUMPER_EXPORT', displayName: id, revision: 1, capabilityVersion: 'synthetic-capability', configFingerprint: 'synthetic-fingerprint', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' })
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function setup() {
  const api = { listConfigTemplates: vi.fn(async () => [item()]), renameConfigTemplate: vi.fn(async () => 2), deleteConfigTemplate: vi.fn(async () => {}), listDataSources: vi.fn(async (): Promise<DataSourceSummary[]> => [{ ...DATA_SOURCE_UI_FIXTURES[0]!, id: 'synthetic-source' }]), listExportNodeCandidates: vi.fn(async (): Promise<ExecutionNodeCandidate[]> => [{ id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64' }]), listExecutionNodes: vi.fn(async () => []), createTaskDraftFromTemplate: vi.fn(async () => ({ draftId: 'synthetic-draft', taskType: 'OBDUMPER_EXPORT' as const })) }
  const navigate = vi.fn(async () => {}); const scope = effectScope(); scopes.push(scope)
  const value = scope.run(() => { const catalog = useTemplateCatalog(() => api); return { catalog, actions: useTemplateActions(catalog), draft: useTemplateDraft(catalog, navigate) } })!
  value.draft.selectedTemplate.value = item()
  return { ...value, api, navigate, scope }
}
describe('模板事实、编辑与派生生命周期', () => {
  it('模板 A → B → A 候选只接纳当前代，首次选择即加载且不触发必填错误', async () => {
    const { draft, api } = setup()
    const old = deferred<DataSourceSummary[]>()
    api.listDataSources.mockReturnValueOnce(old.promise)
    draft.selectTemplate(item('A')); draft.selectTemplate(item('B')); draft.selectTemplate(item('A'))
    await draft.loadChoices()
    old.resolve([{ ...DATA_SOURCE_UI_FIXTURES[0]!, id: 'old-source' }])
    for (let index = 0; index < 12; index++) await Promise.resolve()
    expect(draft.eligibleSources.value.map(item => item.id)).toEqual(['synthetic-source'])
    expect(draft.selectedTemplate.value?.id).toBe('A'); expect(draft.createDraftFailure.value).toBe('')
    expect(draft.canCreate.value).toBe(false); expect(api.createTaskDraftFromTemplate).not.toHaveBeenCalled()
  })
  it('写入期间绑定变化后旧结果不导航，授权失效清除已选模板与候选', async () => {
    const { draft, api, navigate, catalog } = setup()
    await draft.loadChoices(); draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'
    const held = deferred<{ draftId: string; taskType: 'OBDUMPER_EXPORT' }>()
    api.createTaskDraftFromTemplate.mockReturnValueOnce(held.promise)
    const pending = draft.createDraft(); draft.draftNodeID.value = 'another-node'
    held.resolve({ draftId: 'late-draft', taskType: 'OBDUMPER_EXPORT' }); await pending
    expect(navigate).not.toHaveBeenCalled(); expect(catalog.notice.value).toBe('')
    catalog.accessDenied.value = true
    expect(draft.selectedTemplate.value).toBeUndefined(); expect(draft.nodes.value).toEqual([]); expect(draft.draftSourceID.value).toBe('')
  })
  it.each([401, 403])('列表 %s 同步清除改名和删除会话，授权恢复不复活旧操作', async status => {
    const { catalog, actions, draft, api } = setup(); await catalog.refresh()
    actions.startRename(item()); actions.renameValue.value = '合成未保存改名'; actions.requestDelete(item())
    api.listConfigTemplates.mockRejectedValueOnce({ status }); await catalog.refresh()
    expect(catalog.templates.value).toEqual([]); expect(actions.renameID.value).toBe(''); expect(actions.renameValue.value).toBe(''); expect(actions.pendingDelete.value).toBeUndefined()
    const attemptStaleActions = async () => {
      actions.startRename(item()); actions.requestDelete(item()); await actions.confirmRename(item()); await actions.confirmDelete()
      draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'; await draft.createDraft()
      expect(actions.renameID.value).toBe(''); expect(actions.renameValue.value).toBe(''); expect(actions.pendingDelete.value).toBeUndefined()
      expect(api.renameConfigTemplate).not.toHaveBeenCalled(); expect(api.deleteConfigTemplate).not.toHaveBeenCalled(); expect(api.createTaskDraftFromTemplate).not.toHaveBeenCalled()
    }
    await attemptStaleActions()
    api.listConfigTemplates.mockRejectedValueOnce({ status: 503 }); await catalog.refresh(); await attemptStaleActions()
    await catalog.refresh(); await actions.confirmRename(item()); await actions.confirmDelete()
    expect(catalog.accessDenied.value).toBe(false); expect(actions.renameValue.value).toBe(''); expect(actions.pendingDelete.value).toBeUndefined()
    expect(api.renameConfigTemplate).not.toHaveBeenCalled(); expect(api.deleteConfigTemplate).not.toHaveBeenCalled()
    actions.startRename(catalog.templates.value[0]!); actions.renameValue.value = '合成新名'; await actions.confirmRename(item())
    expect(api.renameConfigTemplate).toHaveBeenCalledExactlyOnceWith(item().id, 1, '合成新名')
    actions.requestDelete(catalog.templates.value[0]!); await actions.confirmDelete()
    expect(api.deleteConfigTemplate).toHaveBeenCalledExactlyOnceWith(item().id, 2)
  })
  it('普通列表故障保留输入和确认，过期授权拒绝不能清除最新会话', async () => {
    const { catalog, actions, api } = setup(); await catalog.refresh(); actions.startRename(item()); actions.renameValue.value = '合成未保存改名'; actions.requestDelete(item())
    api.listConfigTemplates.mockRejectedValueOnce({ status: 503 }); await catalog.refresh()
    expect(actions.renameValue.value).toBe('合成未保存改名'); expect(actions.pendingDelete.value).toEqual(item())
    const old = deferred<ConfigTemplateItem[]>(); api.listConfigTemplates.mockImplementationOnce(() => old.promise)
    const pending = catalog.refresh(); await catalog.refresh(); old.reject({ status: 401 }); await pending
    expect(catalog.accessDenied.value).toBe(false); expect(actions.renameValue.value).toBe('合成未保存改名'); expect(actions.pendingDelete.value).toEqual(item()); expect(catalog.templates.value).toEqual([item()])
  })
  it('最新列表胜出，写入前失效旧读取；改名裁剪且保持修订', async () => {
    const { catalog, actions, api } = setup(); await catalog.refresh(); const read = deferred<ConfigTemplateItem[]>(); api.listConfigTemplates.mockImplementationOnce(() => read.promise)
    const pending = catalog.refresh(); actions.startRename(item()); actions.renameValue.value = ' 合成新名 '; await actions.confirmRename(item()); read.resolve([item()]); await pending
    expect(catalog.templates.value[0]?.revision).toBe(2); expect(catalog.templates.value[0]?.displayName).toBe('合成新名'); expect(api.renameConfigTemplate).toHaveBeenCalledWith(item().id, 1, '合成新名')
  })
  it('引用加载只有一个在途请求，空成功可缓存，半份失败不能发布并可重试', async () => {
    const { draft, api } = setup(); const read = deferred<DataSourceSummary[]>(); api.listDataSources.mockImplementationOnce(() => read.promise)
    const pending = draft.loadChoices(); const duplicate = draft.loadChoices(); expect(api.listDataSources).toHaveBeenCalledTimes(1); read.reject({ status: 503 }); await Promise.all([pending, duplicate]); expect(draft.nodes.value).toHaveLength(0)
    await draft.loadChoices(); await draft.loadChoices(); expect(api.listDataSources).toHaveBeenCalledTimes(2); expect(draft.sourcesLoading.value).toBe(false)
  })

  it('强制刷新更新同一节点名称，删除后的同名新节点不替换旧选择', async () => {
    const { draft, api } = setup()
    const source = DATA_SOURCE_UI_FIXTURES[0]!
    const node: ExecutionNodeCandidate = { id: 'synthetic-node', displayName: '合成旧名称', platform: 'WINDOWS_AMD64' }
    api.listDataSources.mockResolvedValue([source])
    api.listExportNodeCandidates.mockResolvedValue([node])
    await draft.loadChoices()
    draft.draftSourceID.value = source.id
    draft.draftNodeID.value = node.id
    api.listExportNodeCandidates.mockResolvedValue([{ ...node, displayName: '合成新名称' }])
    await draft.loadChoices(true)
    expect(draft.nodes.value[0]?.displayName).toBe('合成新名称')
    expect(draft.draftNodeID.value).toBe(node.id)
    expect(draft.draftSourceID.value).toBe(source.id)
    api.listExportNodeCandidates.mockResolvedValue([{ ...node, id: 'synthetic-new-node' }])
    await draft.loadChoices(true)
    expect(draft.draftNodeID.value).toBe('')
    await draft.createDraft()
    expect(api.createTaskDraftFromTemplate).not.toHaveBeenCalled()
  })

  it('刷新候选期间不创建草稿，失败清除引用且可重新加载', async () => {
    const { draft, api } = setup()
    const source = DATA_SOURCE_UI_FIXTURES[0]!
    api.listDataSources.mockResolvedValue([source])
    api.listExportNodeCandidates.mockResolvedValue([{ id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64' }])
    await draft.loadChoices()
    draft.draftSourceID.value = source.id
    draft.draftNodeID.value = 'synthetic-node'
    const response = deferred<ExecutionNodeCandidate[]>()
    api.listExportNodeCandidates.mockReturnValueOnce(response.promise)
    const pending = draft.loadChoices(true)
    const duplicate = draft.loadChoices(true)
    await draft.createDraft()
    expect(api.createTaskDraftFromTemplate).not.toHaveBeenCalled()
    expect(api.listExportNodeCandidates).toHaveBeenCalledTimes(2)
    response.reject({ status: 403 })
    await Promise.all([pending, duplicate])
    expect(draft.nodes.value).toEqual([])
    expect(draft.eligibleSources.value).toEqual([])
    expect(draft.draftSourceID.value).toBe('')
    expect(draft.draftNodeID.value).toBe('')
    await draft.loadChoices()
    expect(draft.nodes.value).toHaveLength(1)
    expect(draft.draftNodeID.value).toBe('')
  })
  it('未绑定对象不创建；草稿方法防重，捕获源和节点并保持导航回调', async () => {
    const { catalog, draft, api, navigate } = setup(); await draft.createDraft(); expect(api.createTaskDraftFromTemplate).not.toHaveBeenCalled()
    await draft.loadChoices(); draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'; const write = deferred<{ draftId: string; taskType: 'OBDUMPER_EXPORT' }>(); api.createTaskDraftFromTemplate.mockImplementationOnce(() => write.promise)
    const pending = draft.createDraft(); await draft.createDraft(); expect(api.createTaskDraftFromTemplate).toHaveBeenCalledTimes(1); write.resolve({ draftId: 'synthetic-created', taskType: 'OBDUMPER_EXPORT' }); await pending
    expect(api.createTaskDraftFromTemplate).toHaveBeenCalledWith(item().id, 'synthetic-source', 'synthetic-node', 'OBDUMPER_EXPORT'); expect(navigate).toHaveBeenCalledWith('synthetic-created', 'OBDUMPER_EXPORT'); expect(catalog.actionBusy.value).toBe('')
  })
  it('改名与删除共享方法锁，取消后的迟到反馈不得进入新输入', async () => {
    const { catalog, actions, api } = setup(); await catalog.refresh(); const write = deferred<number>(); api.renameConfigTemplate.mockImplementationOnce(() => write.promise)
    actions.startRename(item()); actions.renameValue.value = '合成新名'; const pending = actions.confirmRename(item()); await actions.confirmRename(item()); actions.requestDelete(item()); await actions.confirmDelete(); expect(api.renameConfigTemplate).toHaveBeenCalledTimes(1); expect(api.deleteConfigTemplate).not.toHaveBeenCalled()
    actions.cancelRename(); write.reject({ status: 409 }); await pending; expect(catalog.loadFailure.value).toBe(''); actions.requestDelete(item()); await actions.confirmDelete(); expect(catalog.templates.value).toHaveLength(0)
  })
  it.each(['read', 'choices', 'draft', 'rename', 'delete'] as const)('卸载后 %s 不回写、不导航', async kind => {
    const { catalog, actions, draft, api, navigate, scope } = setup(); await catalog.refresh()
    const response = deferred<ConfigTemplateItem[]>(); const choices = deferred<DataSourceSummary[]>(); const write = deferred<{ draftId: string; taskType: 'OBDUMPER_EXPORT' }>(); const rename = deferred<number>(); const deletion = deferred<void>(); let pending: Promise<void>
    if (kind === 'read') { api.listConfigTemplates.mockImplementationOnce(() => response.promise); pending = catalog.refresh() }
    else if (kind === 'choices') { api.listDataSources.mockImplementationOnce(() => choices.promise); pending = draft.loadChoices() }
    else if (kind === 'draft') { await draft.loadChoices(); api.createTaskDraftFromTemplate.mockImplementationOnce(() => write.promise); draft.draftSourceID.value = 'synthetic-source'; draft.draftNodeID.value = 'synthetic-node'; pending = draft.createDraft() }
    else if (kind === 'rename') { api.renameConfigTemplate.mockImplementationOnce(() => rename.promise); actions.startRename(item()); actions.renameValue.value = '合成新名'; pending = actions.confirmRename(item()) }
    else { api.deleteConfigTemplate.mockImplementationOnce(() => deletion.promise); actions.requestDelete(item()); pending = actions.confirmDelete() }
    scope.stop(); const previous = catalog.templates.value
    if (kind === 'read') response.resolve([]); else if (kind === 'choices') choices.reject({ status: 503 }); else if (kind === 'draft') write.resolve({ draftId: 'synthetic-late', taskType: 'OBDUMPER_EXPORT' }); else if (kind === 'rename') rename.resolve(2); else deletion.reject({ status: 409 })
    await pending; expect(catalog.templates.value).toBe(previous); expect(catalog.notice.value).toBe(''); expect(catalog.loadFailure.value).toBe(''); expect(draft.createDraftFailure.value).toBe(''); expect(navigate).not.toHaveBeenCalled()
  })
})
