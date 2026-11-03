import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { ConfigTemplateItem, DataSourceSummary } from '@/api/browser'
import { DATA_SOURCE_UI_FIXTURES } from '@/workbench/sources/dataSourceUiFixture'
import { templateAllows, templateNameError } from './templateRules'
import { useTemplateCatalog } from './useTemplateCatalog'
import { useTemplateDraft } from './useTemplateDraft'
import { useTemplateActions } from './useTemplateActions'

const item = (index = 1): ConfigTemplateItem => ({ id: 'template-' + index, taskType: index % 2 ? 'OBDUMPER_EXPORT' : 'OBLOADER_NORMAL_IMPORT', displayName: '合成模板 ' + index, revision: 1, capabilityVersion: 'synthetic-capability', configFingerprint: 'synthetic-fingerprint', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', availableActions: ['CREATE_DRAFT', 'RENAME', 'DELETE'] })
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function setup() {
  const api = {
    listConfigTemplates: vi.fn(async () => [item()]),
    renameConfigTemplate: vi.fn(async () => 2), deleteConfigTemplate: vi.fn(async () => {}),
    listDataSources: vi.fn(async (): Promise<DataSourceSummary[]> => [{ ...DATA_SOURCE_UI_FIXTURES[0]!, id: 'source' }]),
    listExportNodeCandidates: vi.fn(async () => [{ id: 'node', displayName: '合成节点', platform: 'WINDOWS_AMD64' as const }]),
    listExecutionNodes: vi.fn(async () => []),
    createTaskDraftFromTemplate: vi.fn(async () => ({ draftId: 'created', taskType: 'OBDUMPER_EXPORT' as const })),
  }
  const navigate = vi.fn(async () => {})
  const scope = effectScope(); scopes.push(scope)
  const model = scope.run(() => { const catalog = useTemplateCatalog(() => api); return { catalog, draft: useTemplateDraft(catalog, navigate), actions: useTemplateActions(catalog) } })!
  model.catalog.templates.value = [item()]
  async function bind() { model.draft.selectTemplate(model.catalog.templates.value[0]!); await model.draft.loadChoices(); model.draft.draftSourceID.value = 'source'; model.draft.draftNodeID.value = 'node' }
  return { ...model, api, navigate, bind }
}

describe('模板安全边界与结果恢复', () => {
  it('名称按 UTF-8 字节检查，缺少操作资格不能从可见性推导', () => {
    expect(templateNameError(' ')).toContain('请输入')
    expect(templateNameError('中'.repeat(85) + 'a')).toBe('')
    expect(templateNameError('中'.repeat(85) + 'ab')).toContain('过长')
    expect(templateAllows({ ...item(), availableActions: undefined }, 'CREATE_DRAFT')).toBe(false)
    expect(templateAllows({ ...item(), availableActions: ['RENAME'] }, 'CREATE_DRAFT')).toBe(false)
  })
  it('完整集合的本地分页保留原顺序，筛选和删除收敛合法页码', async () => {
    const { catalog, api } = setup()
    api.listConfigTemplates.mockResolvedValue(Array.from({ length: 21 }, (_, index) => item(index + 1)))
    await catalog.refresh()
    catalog.page.value = 3
    expect(catalog.rows.value.map(value => value.id)).toEqual(['template-21'])
    catalog.typeFilter.value = 'OBLOADER_NORMAL_IMPORT'
    expect(catalog.page.value).toBe(1); expect(catalog.total.value).toBe(10)
    catalog.keyword.value = ' 20 '
    expect(catalog.rows.value.map(value => value.id)).toEqual(['template-20'])
    catalog.clearFilters(); catalog.page.value = 3
    catalog.templates.value = catalog.templates.value.slice(0, 20)
    expect(catalog.page.value).toBe(2)
    expect(catalog.templates.value[0]?.id).toBe('template-1')
  })
  it('响应丢失后冻结创建入口，显式重试沿用原键和绑定', async () => {
    const { draft, api, navigate, bind } = setup(); await bind()
    api.createTaskDraftFromTemplate.mockRejectedValueOnce({ status: 503 })
    await draft.createDraft()
    expect(draft.resultUnknown.value).toBe(true); expect(draft.canCreate.value).toBe(false)
    await draft.createDraft()
    expect(api.createTaskDraftFromTemplate).toHaveBeenCalledTimes(1)
    await draft.retryCreate()
    const calls = api.createTaskDraftFromTemplate.mock.calls as unknown as unknown[][]
    expect(calls[0]).toEqual(calls[1])
    expect(calls[0]?.[4]).toMatch(/^[a-f0-9-]{36}$/)
    expect(navigate).toHaveBeenCalledExactlyOnceWith('created', 'OBDUMPER_EXPORT')
  })
  it('创建已确认但导航失败时只重新打开，不再次创建', async () => {
    const { draft, api, navigate, bind } = setup(); await bind()
    navigate.mockRejectedValueOnce(new Error('合成导航失败'))
    await draft.createDraft()
    expect(draft.createdDraft.value?.draftId).toBe('created')
    expect(draft.createDraftFailure.value).toContain('草稿已创建')
    await draft.openCreatedDraft(); await draft.createDraft()
    expect(api.createTaskDraftFromTemplate).toHaveBeenCalledTimes(1)
    expect(navigate).toHaveBeenCalledTimes(2)
  })
  it('绑定 A → B → A 后旧写入不导航，取消后旧响应不能恢复抽屉', async () => {
    const { draft, api, navigate, bind } = setup(); await bind()
    let resolve!: (result: { draftId: string; taskType: 'OBDUMPER_EXPORT' }) => void
    api.createTaskDraftFromTemplate.mockImplementationOnce(() => new Promise(yes => { resolve = yes }))
    const pending = draft.createDraft()
    draft.draftNodeID.value = 'other'; draft.draftNodeID.value = 'node'
    resolve({ draftId: 'old-created', taskType: 'OBDUMPER_EXPORT' }); await pending
    expect(navigate).not.toHaveBeenCalled()
    draft.clearSelection()
    expect(draft.createdDraft.value).toBeUndefined()
  })
  it('改名冲突保留输入，不能重复覆盖；刷新撤销旧版本会话', async () => {
    const { actions, catalog, api } = setup()
    actions.startRename(item()); actions.renameValue.value = '合成新名称'
    api.renameConfigTemplate.mockRejectedValueOnce({ status: 412 })
    await actions.confirmRename()
    expect(actions.renameValue.value).toBe('合成新名称'); expect(actions.renameConflict.value).toBe(true)
    await actions.confirmRename()
    expect(api.renameConfigTemplate).toHaveBeenCalledTimes(1)
    api.listConfigTemplates.mockResolvedValue([{ ...item(), revision: 2 }])
    await catalog.refresh()
    expect(actions.renameTarget.value).toBeUndefined()
  })
  it('同一模板撤销使用权限时关闭使用会话，但不错误撤销其管理资格', async () => {
    const { catalog, draft, actions, bind } = setup(); await bind()
    catalog.templates.value = [{ ...item(), availableActions: ['RENAME', 'DELETE'] }]
    expect(draft.selectedTemplate.value).toBeUndefined()
    draft.selectTemplate(catalog.templates.value[0]!)
    expect(draft.selectedTemplate.value).toBeUndefined()
    actions.startRename(catalog.templates.value[0]!)
    expect(actions.renameID.value).toBe(item().id)
  })
  it('撤权后旧 finally 不释放重新授权后的新事务', async () => {
    const { catalog } = setup()
    const first = catalog.beginAction('first')
    catalog.denied({ status: 403 }); await catalog.refresh()
    const second = catalog.beginAction('second')
    if (!first || !second) throw new Error('合成事务未建立')
    first(); expect(catalog.actionBusy.value).toBe('second')
    second(); expect(catalog.actionBusy.value).toBe('')
  })
  it('旧目录成功响应不能解除后来发生的撤权', async () => {
    const { catalog, api } = setup()
    let complete!: (items: ConfigTemplateItem[]) => void
    api.listConfigTemplates.mockImplementationOnce(() => new Promise(resolve => { complete = resolve }))
    const pending = catalog.refresh()
    catalog.denied({ status: 403 })
    complete([item()]); await pending
    expect(catalog.accessDenied.value).toBe(true)
    expect(catalog.templates.value).toEqual([])
    expect(catalog.loading.value).toBe(false)
    await catalog.refresh()
    expect(catalog.accessDenied.value).toBe(false)
  })
  it('未决创建的后续拒绝不证明原请求失败，也不开放新键创建', async () => {
    const { draft, api, bind } = setup(); await bind()
    api.createTaskDraftFromTemplate.mockRejectedValueOnce({ status: 503 })
    await draft.createDraft()
    api.createTaskDraftFromTemplate.mockRejectedValueOnce({ status: 422 })
    await draft.retryCreate()
    expect(draft.resultUnknown.value).toBe(true)
    await draft.createDraft()
    expect(api.createTaskDraftFromTemplate).toHaveBeenCalledTimes(2)
    const calls = api.createTaskDraftFromTemplate.mock.calls as unknown as unknown[][]
    expect(calls[0]).toEqual(calls[1])
  })

})
