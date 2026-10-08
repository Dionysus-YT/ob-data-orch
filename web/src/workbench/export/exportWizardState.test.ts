import { afterEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, nextTick, reactive, ref, watch } from 'vue'
import type { BrowserApi, ExportDraft, ExportObjectCatalogQuery, Precheck } from '@/api/browser'
import type { ExportRuntime } from './exportRuntime'
import { DATA_SOURCE_UI_FIXTURES } from '@/views/dataSourceUiFixture'
import { createExportForm } from './exportForm'
import { populateFormFromDraft } from './exportFormMapping'
import { useExportReferences } from './useExportReferences'
import { useExportParameters } from './useExportParameters'
import { useExportCatalog } from './useExportCatalog'
import { useExportDraftLifecycle } from './useExportDraftLifecycle'
import { fixedPrecheckChecks } from './exportPrecheckPresentation'
import { useExportRecovery } from './useExportRecovery'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers(); vi.unstubAllGlobals() })

async function setup(overrides: Partial<BrowserApi> = {}) {
  const scope = effectScope()
  scopes.push(scope)
  const source = { ...DATA_SOURCE_UI_FIXTURES[0]! }
  const step = ref(2)
  const api = { ...overrides } as BrowserApi
  const runtime = {
    api, route: reactive({ query: {} }), router: { push: vi.fn() }, activeStep: computed(() => step.value), moveToStep: vi.fn(),
  } as unknown as ExportRuntime
  const state = scope.run(() => {
    const form = createExportForm()
    const references = useExportReferences({ ...form, api })
    const parameters = useExportParameters({ ...form, ...references })
    const catalog = useExportCatalog({ ...form, ...references, ...runtime })
    const lifecycle = useExportDraftLifecycle({ ...form, ...references, ...parameters, ...runtime, hydrateDraft: draft => populateFormFromDraft({ ...form, ...catalog }, draft) })
    return { form, references, parameters, catalog, lifecycle }
  })!
  state.form.hydratingDerivedDraft.value = true
  state.references.sources.value = [source]
  state.references.nodes.value = [{ id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64' }]
  state.form.selectedDataSourceID.value = source.id
  state.form.selectedNodeID.value = 'synthetic-node'
  state.form.database.value = 'synthetic_database'
  state.form.selectedObjectsByType.TABLE = ['synthetic_table']
  state.form.filePath.value = '/E:/synthetic-output'
  await nextTick()
  state.form.hydratingDerivedDraft.value = false
  const input = state.parameters.draftInput.value!
  expect(input).toBeDefined()
  const draft: ExportDraft = { ...input, id: 'synthetic-draft', revision: 1, configFingerprint: 'synthetic-fingerprint' }
  const passed: Precheck = {
    id: 'synthetic-precheck', draftId: draft.id, draftRevision: draft.revision, nodeId: draft.nodeId, configFingerprint: draft.configFingerprint,
    status: 'SUCCEEDED', integrityStatus: 'COMPLETE', validUntil: '2099-01-01T00:00:00Z',
    results: fixedPrecheckChecks.map(check => ({ check, status: 'PASSED', evidenceCode: 'SYNTHETIC_PASSED' })),
  }
  function saved() {
    state.lifecycle.currentDraft.value = draft
    state.lifecycle.createdDraftID.value = draft.id
    state.lifecycle.commandPreview.value = { command: 'synthetic-preview', configFingerprint: draft.configFingerprint }
    state.lifecycle.activePrecheck.value = passed
  }
  return { ...state, source, step, scope, runtime, draft, passed, saved }
}

describe('导出向导单一状态与参数边界', () => {
  it('内容模式往返保留合法的跨类型对象，格式切换只清不适用字段', async () => {
    const s = await setup()
    s.form.contentKind.value = 'DDL_AND_DATA'
    s.form.selectedObjectsByType.VIEW = ['synthetic_view']
    await nextTick()
    s.form.columnSeparator.value = '|'
    s.form.fileEncoding.value = 'UTF-8'
    s.form.formatKind.value = 'CUT'
    expect(s.form.columnSeparator.value).toBe('')
    expect(s.form.fileEncoding.value).toBe('UTF-8')
    s.form.contentKind.value = 'DATA_ONLY'
    await nextTick()
    expect(s.form.applicableSelectedObjects.value.map(o => o.type)).toEqual(['TABLE'])
    s.form.contentKind.value = 'DDL_AND_DATA'
    await nextTick()
    expect(s.form.applicableSelectedObjects.value.map(o => o.type)).toEqual(['TABLE', 'VIEW'])
  })

  it('数据库和节点切换清除旧范围，租户变化清除未验证时间参数', async () => {
    const s = await setup()
    s.form.database.value = 'synthetic_other'
    await nextTick()
    expect(s.form.selectedObjectsByType.TABLE).toEqual([])
    s.form.selectedObjectsByType.TABLE = ['synthetic_other_table']
    s.form.selectedNodeID.value = 'synthetic-other-node'
    await nextTick()
    expect(s.form.selectedObjectsByType.TABLE).toEqual([])
    s.references.sources.value = [{ ...s.source, compatibilityMode: 'ORACLE' }]
    await nextTick()
    s.form.flashbackTimestamp.value = '2026-10-08 00:00:00'
    s.form.fetchSize.value = '1000'
    s.references.sources.value = [{ ...s.source, compatibilityMode: 'MYSQL' }]
    await nextTick()
    expect(s.form.flashbackTimestamp.value).toBe('')
    expect(s.form.fetchSize.value).toBe('')
  })

  it('快照和闪回互斥，混合对象取消表级筛选资格', async () => {
    const s = await setup()
    s.form.snapshot.value = true
    s.form.flashbackScn.value = '100'
    expect(s.parameters.draftInput.value).toBeUndefined()
    s.form.contentKind.value = 'DDL_AND_DATA'
    s.form.where.value = 'id > 1'
    s.form.partition.value = 'synthetic_partition'
    s.form.selectedObjectsByType.VIEW = ['synthetic_view']
    await nextTick()
    expect(s.form.where.value).toBe('')
    expect(s.form.partition.value).toBe('')
  })
})

describe('草稿、预检查与提交竞态', () => {
  it('恢复读取期间的编辑不会被迟到快照覆盖', async () => {
    const response = deferred<ExportDraft>()
    const s = await setup({ getExportDraft: () => response.promise })
    const restoring = s.lifecycle.loadDerivedDraft('synthetic-draft', false)
    s.form.filePath.value = '/E:/synthetic-current-input'
    response.resolve(s.draft)
    await restoring
    expect(s.form.filePath.value).toBe('/E:/synthetic-current-input')
    expect(s.lifecycle.currentDraft.value).toBeNull()
    expect(s.lifecycle.loadingDraft.value).toBe(false)
  })
  it('字段编辑立即失效旧证据与确认，绑定切换不更新旧草稿', async () => {
    const s = await setup()
    s.saved()
    expect(s.lifecycle.canSubmit.value).toBe(true)
    s.lifecycle.submitTask()
    s.form.filePath.value = '/E:/synthetic-changed'
    expect(s.lifecycle.draftDirty.value).toBe(true)
    expect(s.lifecycle.commandPreview.value).toBeNull()
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    expect(s.lifecycle.submitConfirmationOpen.value).toBe(false)
    s.form.selectedNodeID.value = 'synthetic-other-node'
    expect(s.lifecycle.currentDraft.value).toBeNull()
  })

  it('保存期间编辑只接纳新修订，不解锁旧配置或自动前进', async () => {
    const response = deferred<ExportDraft>()
    const update = vi.fn(() => response.promise)
    const preview = vi.fn()
    const s = await setup({ updateExportDraft: update, previewExportCommand: preview })
    s.saved()
    const saving = s.lifecycle.createDraft()
    s.form.filePath.value = '/E:/synthetic-edited-during-save'
    response.resolve({ ...s.draft, revision: 2 })
    await saving
    expect(s.lifecycle.currentDraft.value?.revision).toBe(2)
    expect(s.lifecycle.draftDirty.value).toBe(true)
    expect(preview).not.toHaveBeenCalled()
    expect(s.runtime.moveToStep).not.toHaveBeenCalled()
  })

  for (const stage of ['preview', 'start'] as const) it(`预检查 ${stage} 期间编辑拒绝迟到响应`, async () => {
    const previewResponse = deferred<Awaited<ReturnType<BrowserApi['previewExportCommand']>>>()
    const startResponse = deferred<string>()
    const start = vi.fn(() => startResponse.promise)
    const poll = vi.fn()
    const s = await setup({ previewExportCommand: () => previewResponse.promise, startPrecheck: start, getPrecheck: poll })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    const checking = s.lifecycle.startPrecheck()
    if (stage === 'start') {
      previewResponse.resolve(s.lifecycle.commandPreview.value!)
      await Promise.resolve()
      expect(start).toHaveBeenCalledOnce()
    }
    s.form.filePath.value = '/E:/synthetic-changed-during-precheck'
    previewResponse.resolve({ command: 'synthetic-preview', configFingerprint: s.draft.configFingerprint })
    startResponse.resolve('synthetic-late-precheck')
    await checking
    expect(s.lifecycle.precheckID.value).toBe('')
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    expect(s.lifecycle.startingPrecheck.value).toBe(false)
    expect(poll).not.toHaveBeenCalled()
    if (stage === 'preview') expect(start).not.toHaveBeenCalled()
  })

  it('同修订预览以最新请求为准，迟到错误不能覆盖成功', async () => {
    const old = deferred<Awaited<ReturnType<BrowserApi['previewExportCommand']>>>()
    const newer = deferred<Awaited<ReturnType<BrowserApi['previewExportCommand']>>>()
    const s = await setup({ previewExportCommand: vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(newer.promise) })
    s.saved()
    const first = s.lifecycle.loadCommandPreview()
    const second = s.lifecycle.loadCommandPreview()
    newer.resolve({ command: 'synthetic-new', configFingerprint: s.draft.configFingerprint })
    await second
    old.reject(new Error('synthetic-old-error'))
    await first
    expect(s.lifecycle.commandPreview.value?.command).toBe('synthetic-new')
    expect(s.lifecycle.commandPreviewFailure.value).toBe('')
    expect(s.lifecycle.previewingCommand.value).toBe(false)
  })

  it('提交防重且卸载后迟到成功不跳转', async () => {
    const response = deferred<string>()
    const submit = vi.fn(() => response.promise)
    const s = await setup({ submitExportDraft: submit })
    s.saved()
    const first = s.lifecycle.confirmSubmitTask()
    const second = s.lifecycle.confirmSubmitTask()
    expect(submit).toHaveBeenCalledOnce()
    s.scope.stop()
    response.resolve('synthetic-task')
    await Promise.all([first, second])
    expect(s.runtime.router.push).not.toHaveBeenCalled()
  })

  it('预检查绑定修订、节点、指纹和完整性，任一不符都阻断', async () => {
    const s = await setup()
    for (const change of [{ draftRevision: 2 }, { nodeId: 'synthetic-other' }, { configFingerprint: 'synthetic-other' }, { integrityStatus: 'GAP' }]) {
      s.saved()
      s.lifecycle.activePrecheck.value = { ...s.passed, ...change } as Precheck
      expect(s.lifecycle.canSubmit.value).toBe(false)
    }
  })
})

describe('会话绑定的刷新恢复', () => {
  it('回填期间卸载后不发布草稿或请求预览', async () => {
    const response = deferred<ExportDraft>()
    const preview = vi.fn()
    const s = await setup({ getExportDraft: () => response.promise, previewExportCommand: preview })
    s.scope.run(() => watch(s.form.filePath, () => s.scope.stop(), { flush: 'sync' }))
    const loading = s.lifecycle.loadDerivedDraft(s.draft.id, false)
    response.resolve({ ...s.draft, config: { ...s.draft.config, outputConfig: { ...s.draft.config.outputConfig, filePath: '/E:/synthetic-restored' } } })
    await loading
    expect(s.lifecycle.currentDraft.value).toBeNull()
    expect(preview).not.toHaveBeenCalled()
  })

  function historyFixture(state: Record<string, unknown> = {}) {
    const history = { state, replaceState: vi.fn((next: Record<string, unknown>) => { history.state = next }) }
    vi.stubGlobal('window', { history })
    return history
  }

  it('保存后只在历史项登记引用，切换绑定删除旧引用', async () => {
    const history = historyFixture()
    const s = await setup()
    const recovery = s.scope.run(() => useExportRecovery({ ...s.form, ...s.references, ...s.catalog, ...s.lifecycle, ...s.runtime }))!
    expect(await recovery.restoreAfterReferences('synthetic-session-fingerprint')).toBe('')
    s.saved()
    await nextTick()
    expect(history.state.obDataOrchExportSavedDraftRecovery).toEqual({ version: 1, sessionFingerprint: 'synthetic-session-fingerprint', draftId: s.draft.id })
    const saved = JSON.stringify(history.state.obDataOrchExportSavedDraftRecovery)
    expect(saved).not.toContain('config')
    expect(saved).not.toContain('precheck')
    s.form.selectedNodeID.value = 'synthetic-other-node'
    await nextTick()
    expect(history.state.obDataOrchExportSavedDraftRecovery).toBeUndefined()
  })

  for (const fingerprint of ['synthetic-session-fingerprint', 'synthetic-other-session', '']) it(`按会话核对已保存引用 ${fingerprint || '无会话'}`, async () => {
    historyFixture({ obDataOrchExportSavedDraftRecovery: { version: 1, sessionFingerprint: 'synthetic-session-fingerprint', draftId: 'synthetic-draft' } })
    const s = await setup()
    const recovery = s.scope.run(() => useExportRecovery({ ...s.form, ...s.references, ...s.catalog, ...s.lifecycle, ...s.runtime }))!
    expect(await recovery.restoreAfterReferences(fingerprint)).toBe(fingerprint === 'synthetic-session-fingerprint' ? 'synthetic-draft' : '')
    expect(s.lifecycle.activePrecheck.value).toBeNull()
  })
})

describe('引用数据与目录请求生命周期', () => {
  it('迟到批量目录不能覆盖新的分类刷新结果', async () => {
    const batch = deferred<ExportObjectCatalogQuery>()
    const category = deferred<ExportObjectCatalogQuery>()
    const s = await setup({ searchExportObjects: vi.fn().mockReturnValueOnce(batch.promise).mockReturnValueOnce(category.promise) })
    const query = { id: 'synthetic-query', dataSourceId: s.source.id, nodeId: 'synthetic-node', database: 'synthetic_database', keyword: '', status: 'SUCCEEDED', truncated: false, validUntil: '2099-01-01T00:00:00Z' } as ExportObjectCatalogQuery
    const first = s.catalog.loadAllCatalogs()
    const second = s.catalog.loadCatalog('TABLE', '', true)
    category.resolve({ ...query, objectType: 'TABLE', objects: ['synthetic_latest_table'], groups: [] })
    await second
    batch.resolve({ ...query, objectType: 'ALL', objects: [], groups: [{ objectType: 'TABLE', objects: ['synthetic_old_table'], truncated: false, unavailable: false }] })
    await first
    expect(s.catalog.catalogByType.TABLE.names).toContain('synthetic_latest_table')
    expect(s.catalog.catalogByType.TABLE.names).not.toContain('synthetic_old_table')
    expect(s.catalog.catalogByType.TABLE.failure).toBe('')
    expect(s.catalog.catalogByType.TABLE.loading).toBe(false)
  })

  it('引用列表拒绝乱序与卸载后的结果', async () => {
    const old = deferred<typeof DATA_SOURCE_UI_FIXTURES>()
    const newer = deferred<typeof DATA_SOURCE_UI_FIXTURES>()
    const late = deferred<typeof DATA_SOURCE_UI_FIXTURES>()
    const s = await setup({ listDataSources: vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(newer.promise).mockReturnValueOnce(late.promise) })
    const first = s.references.loadSources()
    const second = s.references.loadSources()
    newer.resolve([{ ...s.source, displayName: '合成最新名称' }])
    await second
    old.resolve([{ ...s.source, displayName: '合成旧名称' }])
    await first
    expect(s.references.sources.value[0]?.displayName).toBe('合成最新名称')
    const third = s.references.loadSources()
    s.scope.stop()
    late.resolve([])
    await third
    expect(s.references.sources.value).toHaveLength(1)
  })

  it('切库后的迟到 PENDING 不再建立轮询或填入旧目录', async () => {
    vi.useFakeTimers()
    const response = deferred<ExportObjectCatalogQuery>()
    const poll = vi.fn()
    const s = await setup({ searchExportObjects: () => response.promise, getExportObjectCatalogQuery: poll })
    const loading = s.catalog.loadAllCatalogs()
    s.form.database.value = 'synthetic_other'
    response.resolve({ id: 'synthetic-query', status: 'PENDING' } as ExportObjectCatalogQuery)
    await loading
    s.scope.stop()
    await vi.runAllTimersAsync()
    expect(poll).not.toHaveBeenCalled()
    expect(s.catalog.catalogByType.TABLE.names).toEqual([])
  })

  it('卸载时唤醒目录轮询等待并停止后续请求', async () => {
    vi.useFakeTimers()
    const query = { id: 'synthetic-query', dataSourceId: DATA_SOURCE_UI_FIXTURES[0]!.id, nodeId: 'synthetic-node', database: 'synthetic_database', objectType: 'ALL', keyword: '', status: 'PENDING' } as ExportObjectCatalogQuery
    const poll = vi.fn()
    const s = await setup({ searchExportObjects: async () => query, getExportObjectCatalogQuery: poll })
    const loading = s.catalog.loadAllCatalogs()
    await Promise.resolve()
    s.scope.stop()
    await loading
    await vi.runAllTimersAsync()
    expect(poll).not.toHaveBeenCalled()
  })
})
