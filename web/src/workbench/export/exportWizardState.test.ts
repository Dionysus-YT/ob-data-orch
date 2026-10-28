import { afterEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, nextTick, reactive, ref, watch } from 'vue'
import type { BrowserApi, ExecutionNodeCandidate, ExportDraft, ExportObjectCatalogQuery, Precheck } from '@/api/browser'
import type { ExportRuntime } from './exportRuntime'
import { DATA_SOURCE_UI_FIXTURES } from '@/workbench/sources/dataSourceUiFixture'
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
  const route = reactive({ query: {} as Record<string, string> })
  const runtime = {
    api, route, router: { push: vi.fn(), replace: vi.fn(async (target: { query: Record<string, string> }) => { route.query = target.query }) }, activeStep: computed(() => step.value), moveToStep: vi.fn(),
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
  state.form.contentKind.value = 'DATA_ONLY'
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

it('保留 Schema 的正常交互清除原生路径冲突，恢复旧值不静默改写', async () => {
  const state = await setup()
  state.form.contentKind.value = 'DDL_ONLY'
  await nextTick()
  state.form.compactSchema.value = true
  state.form.retainSchema.value = true
  await nextTick()
  expect(state.form.compactSchema.value).toBe(false)
  expect(state.parameters.draftInput.value?.config.ddlBehavior?.compactSchema).toBe(false)
  state.form.hydratingDerivedDraft.value = true
  state.form.retainSchema.value = false
  state.form.compactSchema.value = true
  await nextTick()
  state.form.retainSchema.value = true
  await nextTick()
  expect(state.form.compactSchema.value).toBe(true)
  state.form.hydratingDerivedDraft.value = false
  expect(state.parameters.draftInput.value).toBeUndefined()
})

describe('导出向导单一状态与参数边界', () => {
  it('仅手动选择节点也是未保存输入，回到空表单后不误报丢失配置', async () => {
    const s = await setup()
    s.form.hydratingDerivedDraft.value = true
    s.form.resetForm()
    await nextTick()
    s.form.hydratingDerivedDraft.value = false
    s.lifecycle.clearDraftState()
    expect(s.lifecycle.hasInputs.value).toBe(false)
    s.form.selectedNodeID.value = 'synthetic-node'
    expect(s.lifecycle.hasInputs.value).toBe(true)
    expect(s.lifecycle.draftDirty.value).toBe(true)
    s.form.selectedNodeID.value = ''
    expect(s.lifecycle.draftDirty.value).toBe(false)
  })
  it('更换来源先确认，取消不清输入或检查，确认后沿用既有清值规则', async () => {
    const s = await setup(), other = { ...s.source, id: 'synthetic-other-source', displayName: '合成备用数据源' }
    s.references.sources.value.push(other)
    s.saved()
    s.parameters.selectSource(other.id)
    s.parameters.selectSource('synthetic-missing-source')
    expect(s.parameters.pendingSourceId.value).toBe(other.id)
    expect(s.form.selectedDataSourceID.value).toBe(s.source.id)
    expect(s.form.database.value).toBe('synthetic_database')
    expect(s.lifecycle.activePrecheck.value).not.toBeNull()
    s.parameters.cancelSourceChange()
    expect(s.parameters.pendingSourceId.value).toBeNull()
    expect(s.form.selectedObjectsByType.TABLE).toEqual(['synthetic_table'])
    s.parameters.selectSource(other.id)
    s.parameters.confirmSourceChange()
    await nextTick()
    expect(s.form.selectedDataSourceID.value).toBe(other.id)
    expect(s.form.database.value).toBe('')
    expect(s.form.contentKind.value).toBe('')
    expect(s.form.selectedObjectsByType.TABLE).toEqual([])
    expect(s.lifecycle.activePrecheck.value).toBeNull()
  })

  it('来源候选失效与草稿回填撤销旧确认，不能确认到失效候选或旧会话', async () => {
    const s = await setup(), other = { ...s.source, id: 'synthetic-other-source' }
    s.references.sources.value.push(other)
    s.parameters.selectSource(other.id)
    s.references.sources.value = [s.source]
    s.parameters.confirmSourceChange()
    expect(s.form.selectedDataSourceID.value).toBe(s.source.id)
    s.references.sources.value.push(other)
    s.parameters.selectSource(other.id)
    s.form.hydratingDerivedDraft.value = true
    expect(s.parameters.pendingSourceId.value).toBeNull()
    s.parameters.confirmSourceChange()
    expect(s.form.selectedDataSourceID.value).toBe(s.source.id)
  })

  it('新表单和重置不默认选择导出内容，未选择只能保存编辑草稿', async () => {
    const initial = createExportForm()
    expect(initial.contentKind.value).toBe('')
    initial.contentKind.value = 'DDL_ONLY'
    initial.resetForm()
    expect(initial.contentKind.value).toBe('')
    const s = await setup()
    s.form.contentKind.value = ''
    await nextTick()
    expect(s.parameters.exportMode.value).toBe('')
    expect(s.parameters.dataOptionsActive.value).toBe(false)
    expect(s.parameters.extraMessageSupported.value).toBe(false)
    expect(s.parameters.draftInput.value).toBeUndefined()
    expect(s.parameters.saveDraftInput.value).toMatchObject({ draftState: 'EDITING', config: { contentSelection: { contentKind: '' } } })
    s.form.selectedDataSourceID.value = ''
    await nextTick()
    expect(s.form.contentKind.value).toBe('')
  })

  it('专家参数可保存和回填，失去内容或 sys 资格时清除', async () => {
    const s = await setup()
    s.references.sources.value = [{ ...s.source, sysCredentialState: 'AVAILABLE' }]
    s.form.scopeKind.value = 'ALL'
    s.form.contentKind.value = 'DDL_AND_DATA'
    await nextTick()
    s.form.weakRead.value = true
    s.form.enableHiddenPk.value = true
    s.form.addExtraMessage.value = true
    s.form.sequencePolicy.value = 'RESTART'
    s.form.timeValueFormat.value = 'HH:mm:ss'
    s.form.timestampValueFormat.value = 'yyyy-MM-dd HH:mm:ss'
    s.form.preserveZeroDatetime.value = true
    const input = s.parameters.draftInput.value!
    expect(input.config.filterConfig).toMatchObject({ weakRead: true, enableHiddenPk: true })
    expect(input.config.ddlBehavior).toMatchObject({ addExtraMessage: true, sequencePolicy: 'RESTART' })
    expect(input.config.dataFormat?.timestampFormats).toMatchObject({ timeValueFormat: 'HH:mm:ss', preserveZeroDatetime: true })
    s.form.hydratingDerivedDraft.value = true
    populateFormFromDraft({ ...s.form, ...s.catalog }, { ...s.draft, ...input })
    await nextTick()
    s.form.hydratingDerivedDraft.value = false
    expect(s.parameters.draftInput.value).toEqual(input)
    s.references.sources.value = [{ ...s.source, sysCredentialState: 'UNAVAILABLE' }]
    await nextTick()
    expect(s.form.addExtraMessage.value).toBe(false)
    expect(s.form.weakRead.value).toBe(true)
    s.form.contentKind.value = 'DDL_ONLY'
    await nextTick()
    expect(s.form.weakRead.value).toBe(false)
    expect(s.form.enableHiddenPk.value).toBe(false)
    expect(s.form.preserveZeroDatetime.value).toBe(false)
    expect(s.form.sequencePolicy.value).toBe('RESTART')
  })

  it('租户和格式切换只清除不适用日期参数，Oracle SQL 保留 NLS', async () => {
    const s = await setup()
    s.form.dateValueFormat.value = 'yyyy-MM-dd'
    s.form.timeValueFormat.value = 'HH:mm:ss'
    s.form.preserveZeroDatetime.value = true
    s.references.sources.value = [{ ...s.source, compatibilityMode: 'ORACLE' }]
    await nextTick()
    expect(s.form.dateValueFormat.value).toBe('yyyy-MM-dd')
    expect(s.form.timeValueFormat.value).toBe('')
    expect(s.form.preserveZeroDatetime.value).toBe(false)
    s.form.timestampTzValueFormat.value = 'yyyy-MM-dd HH:mm:ss'
    s.form.nlsDateFormat.value = 'YYYY-MM-DD HH24:MI:SS'
    s.form.formatKind.value = 'SQL'
    await nextTick()
    expect(s.form.dateValueFormat.value).toBe('')
    expect(s.form.timestampTzValueFormat.value).toBe('')
    expect(s.parameters.draftInput.value?.config.dataFormat?.timestampFormats).toEqual({ nlsDateFormat: 'YYYY-MM-DD HH24:MI:SS' })
  })

  it('每个新增参数编辑立即撤销旧预览、预检查和提交确认', async () => {
    const s = await setup()
    const changes = [() => { s.form.weakRead.value = true }, () => { s.form.enableHiddenPk.value = true }, () => { s.form.addExtraMessage.value = true }, () => { s.form.sequencePolicy.value = 'PRESERVE' }, () => { s.form.preserveZeroDatetime.value = true }, () => { s.form.timestampValueFormat.value = 'yyyy-MM-dd' }, () => { s.form.nlsDateFormat.value = 'YYYY-MM-DD' }]
    for (const change of changes) {
      s.saved()
      s.lifecycle.submitConfirmationOpen.value = true
      change()
      expect(s.lifecycle.commandPreview.value).toBeNull()
      expect(s.lifecycle.activePrecheck.value).toBeNull()
      expect(s.lifecycle.submitConfirmationOpen.value).toBe(false)
    }
  })

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
  it('进入第五步自动检查一次，返回保留结果且手动重试防重', async () => {
    const start = vi.fn(async () => 'synthetic-precheck')
    const poll = vi.fn()
    const preview = vi.fn()
    const s = await setup({ startPrecheck: start, getPrecheck: poll, previewExportCommand: preview })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    preview.mockResolvedValue(s.lifecycle.commandPreview.value)
    poll.mockResolvedValue({ ...s.passed, status: 'FAILED', results: s.passed.results.map(result => result.check === 'OUTPUT_PATH' ? { ...result, status: 'FAILED', evidenceCode: 'OUTPUT_PATH_NOT_WRITABLE' } : result) })
    expect(start).not.toHaveBeenCalled()
    s.step.value = 5
    await vi.waitFor(() => expect(s.lifecycle.activePrecheck.value?.status).toBe('FAILED'))
    expect(start).toHaveBeenCalledOnce()
    expect(s.lifecycle.precheckOverview.value.description).toContain('1 项未通过')
    expect(s.lifecycle.canSubmit.value).toBe(false)
    s.step.value = 4
    await nextTick()
    s.step.value = 5
    await nextTick()
    expect(start).toHaveBeenCalledOnce()
    poll.mockResolvedValue(s.passed)
    await Promise.all([s.lifecycle.startPrecheck(), s.lifecycle.startPrecheck()])
    expect(start).toHaveBeenCalledTimes(2)
    expect(s.lifecycle.canSubmit.value).toBe(true)
  })

  it('等待草稿和预览准备就绪，自动请求失败后不循环重试', async () => {
    const response = deferred<Awaited<ReturnType<BrowserApi['previewExportCommand']>>>()
    const start = vi.fn().mockRejectedValue(new Error('synthetic-request-failed'))
    const s = await setup({ previewExportCommand: () => response.promise, startPrecheck: start })
    s.lifecycle.currentDraft.value = s.draft
    s.step.value = 5
    const previewing = s.lifecycle.loadCommandPreview()
    await nextTick()
    expect(start).not.toHaveBeenCalled()
    response.resolve({ command: 'synthetic-preview', configFingerprint: s.draft.configFingerprint })
    await previewing
    await vi.waitFor(() => expect(s.lifecycle.precheckFailure.value).not.toBe(''))
    await nextTick()
    expect(start).toHaveBeenCalledOnce()
    expect(s.lifecycle.canStartPrecheck.value).toBe(true)
  })

  it('检查期间配置变化拒绝迟到结果，保存后仅为新版本自动检查', async () => {
    const old = deferred<Precheck>()
    const start = vi.fn().mockResolvedValueOnce('synthetic-old-check').mockResolvedValue('synthetic-new-check')
    const poll = vi.fn().mockReturnValueOnce(old.promise)
    const preview = vi.fn()
    const update = vi.fn()
    const s = await setup({ startPrecheck: start, getPrecheck: poll, previewExportCommand: preview, updateExportDraft: update })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    preview.mockImplementation(async (draft: ExportDraft) => ({ command: 'synthetic-preview', configFingerprint: draft.configFingerprint }))
    s.step.value = 5
    await vi.waitFor(() => expect(poll).toHaveBeenCalledOnce())
    s.form.filePath.value = '/E:/synthetic-new-output'
    old.resolve({ ...s.passed, id: 'synthetic-old-check' })
    await nextTick()
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    expect(s.lifecycle.canSubmit.value).toBe(false)
    const revised = { ...s.draft, revision: 2, configFingerprint: 'synthetic-new-fingerprint', config: s.parameters.draftInput.value!.config }
    update.mockResolvedValue(revised)
    poll.mockResolvedValue({ ...s.passed, id: 'synthetic-new-check', draftRevision: 2, configFingerprint: revised.configFingerprint })
    await s.lifecycle.createDraft(false)
    await vi.waitFor(() => expect(s.lifecycle.canSubmit.value).toBe(true))
    expect(start).toHaveBeenCalledTimes(2)
    expect((s.lifecycle.activePrecheck.value as Precheck | null)?.id).toBe('synthetic-new-check')
  })

  it('第五步自动检查卸载后不接纳迟到结果', async () => {
    const response = deferred<Precheck>()
    const poll = vi.fn(() => response.promise)
    const preview = vi.fn()
    const s = await setup({ startPrecheck: async () => 'synthetic-precheck', getPrecheck: poll, previewExportCommand: preview })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    preview.mockResolvedValue(s.lifecycle.commandPreview.value)
    s.step.value = 5
    await vi.waitFor(() => expect(poll).toHaveBeenCalledOnce())
    s.scope.stop()
    response.resolve(s.passed)
    await Promise.resolve()
    expect(s.lifecycle.activePrecheck.value).toBeNull()
  })

  it('节点 A 到 B 再回 A 后拒绝旧自动检查的迟到标识', async () => {
    const response = deferred<string>()
    const start = vi.fn(() => response.promise)
    const poll = vi.fn()
    const preview = vi.fn()
    const s = await setup({ startPrecheck: start, getPrecheck: poll, previewExportCommand: preview })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    preview.mockResolvedValue(s.lifecycle.commandPreview.value)
    s.step.value = 5
    await vi.waitFor(() => expect(start).toHaveBeenCalledOnce())
    s.form.selectedNodeID.value = 'synthetic-node-b'
    s.form.selectedNodeID.value = 'synthetic-node'
    response.resolve('synthetic-old-check')
    await nextTick()
    expect(s.lifecycle.precheckID.value).toBe('')
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    expect(s.lifecycle.currentDraft.value).toBeNull()
    expect(poll).not.toHaveBeenCalled()
  })

  it('预检查读取返回其他标识时阻断并保留安全错误', async () => {
    const preview = vi.fn()
    const poll = vi.fn()
    const s = await setup({ startPrecheck: async () => 'synthetic-current-check', getPrecheck: poll, previewExportCommand: preview })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    preview.mockResolvedValue(s.lifecycle.commandPreview.value)
    poll.mockResolvedValue(s.passed)
    s.step.value = 5
    await vi.waitFor(() => expect(s.lifecycle.precheckFailure.value).not.toBe(''))
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    expect(s.lifecycle.canSubmit.value).toBe(false)
  })

  it('节点等待期间读取失败可恢复同一检查，不再次发起探测', async () => {
    vi.useFakeTimers()
    const start = vi.fn(async () => 'synthetic-precheck')
    const poll = vi.fn()
    const preview = vi.fn()
    const s = await setup({ startPrecheck: start, getPrecheck: poll, previewExportCommand: preview })
    s.saved()
    s.lifecycle.activePrecheck.value = null
    preview.mockResolvedValue(s.lifecycle.commandPreview.value)
    poll.mockResolvedValueOnce({ ...s.passed, status: 'PENDING' }).mockRejectedValueOnce(new Error('synthetic-read-failed')).mockResolvedValue(s.passed)
    s.step.value = 5
    await vi.waitFor(() => expect(poll).toHaveBeenCalledOnce())
    await vi.advanceTimersByTimeAsync(1000)
    expect(s.lifecycle.precheckFailure.value).not.toBe('')
    expect(s.lifecycle.precheckWaitingForResults.value).toBe(true)
    expect(s.lifecycle.precheckRunning.value).toBe(false)
    expect(s.lifecycle.canStartPrecheck.value).toBe(true)
    await s.lifecycle.startPrecheck()
    expect(start).toHaveBeenCalledOnce()
    expect(s.lifecycle.canSubmit.value).toBe(true)
  })

  it('缺项或到达有效期的通过结果仍阻断提交，不自动重探', async () => {
    vi.useFakeTimers()
    const s = await setup()
    s.saved()
    s.lifecycle.activePrecheck.value = { ...s.passed, results: s.passed.results.slice(0, 1) }
    expect(s.lifecycle.precheckRows.value).toHaveLength(7)
    expect(s.lifecycle.canSubmit.value).toBe(false)
    s.lifecycle.activePrecheck.value = { ...s.passed, validUntil: new Date(Date.now() + 1000).toISOString() }
    expect(s.lifecycle.canSubmit.value).toBe(true)
    await vi.advanceTimersByTimeAsync(1001)
    expect(s.lifecycle.canSubmit.value).toBe(false)
    expect(s.lifecycle.precheckOverview.value.message).toContain('已过期')
  })

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
    expect(s.lifecycle.operationNotice.value).toBe('')
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

  it('同一节点改名只更新当前名称，保留草稿绑定和预检查', async () => {
    const read = vi.fn(async (): Promise<ExecutionNodeCandidate[]> => [{ id: 'synthetic-node', displayName: '合成新登记名称', platform: 'WINDOWS_AMD64' }])
    const s = await setup({ listExportNodeCandidates: read })
    s.saved()
    const draft = s.lifecycle.currentDraft.value
    const precheck = s.lifecycle.activePrecheck.value
    await s.references.loadNodeCandidates()
    await nextTick()
    expect(s.references.selectedNode.value?.displayName).toBe('合成新登记名称')
    expect(s.lifecycle.displayedNode.value?.displayName).toBe('合成新登记名称')
    expect(s.form.selectedNodeID.value).toBe('synthetic-node')
    expect(s.lifecycle.currentDraft.value).toBe(draft)
    expect(s.lifecycle.activePrecheck.value).toBe(precheck)
  })

  it('候选重排不改变已选节点或检查，新建空选择不自动填首项', async () => {
    const nodes: ExecutionNodeCandidate[] = [{ id: 'synthetic-other', displayName: '合成在线节点', platform: 'WINDOWS_AMD64' }, { id: 'synthetic-node', displayName: '合成原节点', platform: 'WINDOWS_AMD64' }]
    const s = await setup({ listExportNodeCandidates: vi.fn().mockResolvedValue(nodes) }); s.saved()
    const precheck = s.lifecycle.activePrecheck.value
    await s.references.loadNodeCandidates(); expect(s.form.selectedNodeID.value).toBe('synthetic-node'); expect(s.lifecycle.activePrecheck.value).toBe(precheck)
    s.form.selectedNodeID.value = ''; await s.references.loadNodeCandidates(); expect(s.form.selectedNodeID.value).toBe('')
  })

  it('候选策略读取失败撤下旧选项，保留绑定并在复读成功后恢复', async () => {
    const node: ExecutionNodeCandidate = { id: 'synthetic-node', displayName: '合成原节点', platform: 'WINDOWS_AMD64' }
    const read = vi.fn().mockResolvedValueOnce([node]).mockRejectedValueOnce({ status: 503, code: 'EXECUTION_NODE_QUERY_UNAVAILABLE' }).mockResolvedValueOnce([node])
    const s = await setup({ listExportNodeCandidates: read }); await s.references.loadNodeCandidates(); await s.references.loadNodeCandidates(); await nextTick()
    expect(s.references.nodes.value).toEqual([]); expect(s.references.selectedNode.value).toBeUndefined(); expect(s.form.selectedNodeID.value).toBe('synthetic-node'); expect(s.references.nodeLoadFailure.value).toBeTruthy()
    await s.references.loadNodeCandidates(); expect(s.references.selectedNode.value?.id).toBe('synthetic-node')
  })

  it('原节点删除后清除绑定，后续展开不自动改派到唯一新节点', async () => {
    const read = vi.fn(async (): Promise<ExecutionNodeCandidate[]> => [{ id: 'synthetic-replacement', displayName: '合成节点', platform: 'WINDOWS_AMD64' }])
    const s = await setup({ listExportNodeCandidates: read })
    s.saved()
    await s.references.loadNodeCandidates()
    await nextTick()
    expect(s.form.selectedNodeID.value).toBe('')
    expect(s.lifecycle.currentDraft.value).toBeNull()
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    await s.references.loadNodeCandidates()
    expect(s.form.selectedNodeID.value).toBe('')
  })

  it('节点名称拒绝乱序与卸载后的响应', async () => {
    const old = deferred<ExecutionNodeCandidate[]>()
    const newer = deferred<ExecutionNodeCandidate[]>()
    const late = deferred<ExecutionNodeCandidate[]>()
    const s = await setup({ listExportNodeCandidates: vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(newer.promise).mockReturnValueOnce(late.promise) })
    const first = s.references.loadNodeCandidates()
    const second = s.references.loadNodeCandidates()
    newer.resolve([{ id: 'synthetic-node', displayName: '合成最新名称', platform: 'WINDOWS_AMD64' }])
    await second
    old.resolve([{ id: 'synthetic-node', displayName: '合成旧名称', platform: 'WINDOWS_AMD64' }])
    await first
    expect(s.references.selectedNode.value?.displayName).toBe('合成最新名称')
    const third = s.references.loadNodeCandidates()
    s.scope.stop()
    late.resolve([{ id: 'synthetic-node', displayName: '合成迟到名称', platform: 'WINDOWS_AMD64' }])
    await third
    expect(s.references.nodes.value[0]?.displayName).toBe('合成最新名称')
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


describe('导出操作反馈的事实与会话', () => {
  it('仅选数据源即可保存并恢复；补齐后更新同一草稿且保存不触发预检查', async () => {
    let stored: ExportDraft
    const preview = vi.fn(), precheck = vi.fn()
    const s = await setup({ createExportDraft: async input => { stored = { ...input, id: 'synthetic-draft', revision: 1, configFingerprint: 'synthetic-fingerprint' }; return stored.id }, getExportDraft: async () => structuredClone(stored), updateExportDraft: async draft => { stored = { ...draft, revision: draft.revision + 1 }; return stored }, previewExportCommand: preview, startPrecheck: precheck })
    s.form.selectedNodeID.value = ''
    s.form.database.value = ''
    s.form.filePath.value = ''
    await nextTick()
    await s.lifecycle.createDraft(false)
    expect(s.lifecycle.currentDraft.value?.draftState).toBe('EDITING')
    expect(s.lifecycle.draftDirty.value).toBe(false)
    expect(s.runtime.route.query.draft).toBe('synthetic-draft')
    expect(s.lifecycle.operationNotice.value).toBe('草稿已保存。')
    await s.lifecycle.loadDerivedDraft('synthetic-draft')
    expect(s.form.selectedNodeID.value).toBe('')
    expect(s.form.database.value).toBe('')
    expect(s.lifecycle.derivedDraftBindingLocked.value).toBe(false)
    s.form.selectedNodeID.value = 'synthetic-node'
    s.form.database.value = 'synthetic_database'
    s.form.scopeKind.value = 'ALL'
    s.form.filePath.value = '/E:/synthetic-output'
    await nextTick()
    await s.lifecycle.createDraft(false)
    expect(s.lifecycle.currentDraft.value).toMatchObject({ id: 'synthetic-draft', revision: 2, draftState: 'READY' })
    expect(s.lifecycle.draftDirty.value).toBe(false)
    expect(preview).not.toHaveBeenCalled()
    expect(precheck).not.toHaveBeenCalled()
  })

  it('读取 A → B → A 时拒绝第一次 A 的迟到成功', async () => {
    const first = deferred<ExportDraft>(), second = deferred<ExportDraft>(), third = deferred<ExportDraft>()
    const read = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise)
    const s = await setup({ getExportDraft: read })
    const a1 = s.lifecycle.loadDerivedDraft('a', false), b = s.lifecycle.loadDerivedDraft('b', false), a2 = s.lifecycle.loadDerivedDraft('a', false)
    third.resolve({ ...s.draft, id: 'a', revision: 3 }); await a2
    first.resolve({ ...s.draft, id: 'a', revision: 1 }); second.resolve({ ...s.draft, id: 'b', revision: 2 }); await Promise.all([a1, b])
    expect(s.lifecycle.currentDraft.value).toMatchObject({ id: 'a', revision: 3 })
    expect(s.lifecycle.draftLoadFailure.value).toBe('')
  })

  it('创建回读身份错误不能标记保存成功或绑定 URL', async () => {
    const s = await setup({ createExportDraft: async () => 'requested', getExportDraft: async () => ({ ...s.draft, id: 'foreign' }) })
    await s.lifecycle.createDraft(false)
    expect(s.lifecycle.currentDraft.value).toBeNull()
    expect(s.lifecycle.operationNotice.value).toBe('')
    expect(s.runtime.router.replace).not.toHaveBeenCalled()
    expect(s.lifecycle.draftFailure.value).not.toBe('')
  })

  it('有效保存只确认一次，预览或预检查结论仍属于持久结果', async () => {
    const s = await setup({
      updateExportDraft: async draft => ({ ...draft, revision: 2 }),
      previewExportCommand: async draft => ({ command: 'synthetic-preview', configFingerprint: draft.configFingerprint }),
    })
    s.saved()
    await s.lifecycle.createDraft(false)
    expect(s.lifecycle.operationNotice.value).toBe('草稿已保存。')
    expect(s.lifecycle.draftNotice.value).toBe('')
    expect(s.lifecycle.activePrecheck.value).toBeNull()
    expect(s.lifecycle.canSubmit.value).toBe(false)
  })

  it('复制成功与失败分开，失败不会被表达为成功', async () => {
    const copy = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('synthetic-denied'))
    vi.stubGlobal('navigator', { clipboard: { writeText: copy } })
    const s = await setup()
    s.saved()
    await s.lifecycle.copyCommand()
    expect(s.lifecycle.copyNotice.value).toBe('脱敏命令已复制。')
    expect(s.lifecycle.copyFailure.value).toBe('')
    await s.lifecycle.copyCommand()
    expect(s.lifecycle.copyNotice.value).toBe('')
    expect(s.lifecycle.copyFailure.value).toContain('复制失败')
  })

  for (const invalidation of ['edit', 'binding', 'unmount'] as const) it('复制在 ' + invalidation + ' 后完成不能恢复旧反馈', async () => {
    const copied = deferred<void>()
    vi.stubGlobal('navigator', { clipboard: { writeText: () => copied.promise } })
    const s = await setup()
    s.saved()
    const copying = s.lifecycle.copyCommand()
    if (invalidation === 'edit') s.form.filePath.value = '/E:/synthetic-new-output'
    else if (invalidation === 'binding') s.form.selectedNodeID.value = 'synthetic-other-node'
    else s.scope.stop()
    copied.resolve(undefined)
    await copying
    expect(s.lifecycle.copyNotice.value).toBe('')
    expect(s.lifecycle.copyFailure.value).toBe('')
  })

  it('旧复制失败不能覆盖新复制成功', async () => {
    const old = deferred<void>()
    const copy = vi.fn().mockReturnValueOnce(old.promise).mockResolvedValueOnce(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText: copy } })
    const s = await setup()
    s.saved()
    const copying = s.lifecycle.copyCommand()
    await s.lifecycle.copyCommand()
    old.reject(new Error('synthetic-old-denied'))
    await copying
    expect(s.lifecycle.copyNotice.value).toBe('脱敏命令已复制。')
    expect(s.lifecycle.copyFailure.value).toBe('')
  })
})
