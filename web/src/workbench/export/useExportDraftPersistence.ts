import { computed, isRef, nextTick, onScopeDispose, ref, watch, type Ref } from 'vue'
import { exportDraftErrorMessage, type ExportDraft, type ExportDraftInput } from '@/api/browser'
import type { ExportForm } from './exportForm'
import type { ExportRuntime } from './exportRuntime'
import type { ExportReferences } from './useExportReferences'

// 草稿能力独占读写、修订和 URL 绑定；表单继续编辑时仅接纳服务端新修订，不回填旧配置。
export function useExportDraftPersistence(deps: Pick<ExportForm, 'fields' | 'selectedDataSourceID' | 'selectedNodeID' | 'hydratingDerivedDraft'> & Pick<ExportReferences, 'selectedSourceRevision'> & Pick<ExportRuntime, 'api' | 'route' | 'router' | 'activeStep' | 'moveToStep'> & {
  draftInput: Readonly<Ref<ExportDraftInput | undefined>>; saveDraftInput?: Readonly<Ref<ExportDraftInput | undefined>>
  busy: Readonly<Ref<boolean>>; hydrateDraft(draft: ExportDraft): void; resetReview(): void
}) {
  const currentDraft = ref<ExportDraft | null>(null), createdDraftID = ref('')
  const creatingDraft = ref(false), loadingDraft = ref(false), bindingDraftRoute = ref(false)
  const draftFailure = ref(''), draftLoadFailure = ref(''), draftNotice = ref(''), operationNotice = ref('')
  const formVersion = ref(0), savedVersion = ref(0)
  const signature = () => JSON.stringify(Object.fromEntries(Object.entries(deps.fields).map(([key, value]) => [key, isRef(value) ? value.value : value])))
  const initialSignature = signature()
  const hasInputs = computed(() => !!currentDraft.value || signature() !== initialSignature)
  const draftDirty = computed(() => hasInputs.value && formVersion.value !== savedVersion.value)
  let disposed = false, session = 0, readEpoch = 0, bindingVersion = 0
  let lastRecovery: { id: string; routeBound: boolean } | undefined

  function invalidateDraftState() {
    formVersion.value++
    deps.resetReview(); operationNotice.value = ''; draftFailure.value = ''
    if (currentDraft.value) draftNotice.value = '当前配置有未保存更改；保存后需重新生成命令预览与预检查。'
  }
  watch([deps.selectedSourceRevision, ...Object.values(deps.fields)], () => {
    if (!deps.hydratingDerivedDraft.value) invalidateDraftState()
  }, { deep: true, flush: 'sync' })
  watch([deps.selectedDataSourceID, deps.selectedNodeID, deps.selectedSourceRevision], () => {
    if (deps.hydratingDerivedDraft.value) return
    bindingVersion++
    const draft = currentDraft.value
    if (draft && draft.draftState !== 'EDITING' && (draft.dataSourceId !== deps.selectedDataSourceID.value || draft.nodeId !== deps.selectedNodeID.value)) {
      clearDraftState()
    }
  }, { flush: 'sync' })

  function clearDraftState() {
    session++; readEpoch++; lastRecovery = undefined
    currentDraft.value = null; createdDraftID.value = ''; creatingDraft.value = loadingDraft.value = bindingDraftRoute.value = false
    draftFailure.value = draftLoadFailure.value = draftNotice.value = operationNotice.value = ''
    deps.resetReview()
  }

  async function loadDerivedDraft(id: string, routeBound = true) {
    if (disposed) return
    clearDraftState(); lastRecovery = { id, routeBound }
    const request = session, read = ++readEpoch, version = formVersion.value
    loadingDraft.value = true
    try {
      const draft = await deps.api.getExportDraft(id)
      if (disposed || request !== session || read !== readEpoch) return
      if (formVersion.value !== version) { draftLoadFailure.value = '读取草稿期间配置已有变化，请重新打开草稿。'; return }
      if (draft.id !== id || routeBound && deps.route.query.draft !== id) throw new Error('草稿读取绑定无效')
      deps.hydratingDerivedDraft.value = true
      try { deps.hydrateDraft(draft); await nextTick() }
      finally { if (!disposed && request === session) deps.hydratingDerivedDraft.value = false }
      if (disposed || request !== session || read !== readEpoch) return
      currentDraft.value = draft; createdDraftID.value = draft.id; savedVersion.value = formVersion.value
      draftNotice.value = draft.draftState === 'EDITING' ? '已恢复未完成草稿；补齐配置后才能预检查。' : draft.sourceTaskId ? '已加载来源任务派生的草稿；来源绑定及从头重执行限制仍由服务端复验。' : '已从服务端恢复最近保存的导出草稿；预检查需要重新执行。'
    } catch (error) {
      if (!disposed && request === session && read === readEpoch) draftLoadFailure.value = exportDraftErrorMessage(error, '无法读取导出草稿。')
    } finally { if (!disposed && request === session && read === readEpoch) loadingDraft.value = false }
  }

  async function bindRoute(id: string, request: number) {
    if (deps.route.query.draft === id) return
    bindingDraftRoute.value = true
    try { await deps.router.replace({ query: { ...deps.route.query, draft: id } }) }
    finally { if (!disposed && request === session) bindingDraftRoute.value = false }
  }

  async function createDraft(advance = true) {
    if (disposed || creatingDraft.value || loadingDraft.value || deps.busy.value) return
    const input = advance ? deps.draftInput.value : deps.saveDraftInput?.value ?? deps.draftInput.value
    if (!input) { draftFailure.value = '当前配置包含无法保存的数值或未开放格式，请修正后重试。'; return }
    const frozen: ExportDraftInput = JSON.parse(JSON.stringify(input))
    const previous = currentDraft.value, request = session, version = formVersion.value, binding = bindingVersion
    creatingDraft.value = true; draftFailure.value = draftNotice.value = operationNotice.value = ''; deps.resetReview()
    try {
      const createdID = previous ? undefined : await deps.api.createExportDraft(frozen)
      if (disposed || request !== session) return
      const draft = previous
        ? await deps.api.updateExportDraft({ ...previous, dataSourceId: frozen.dataSourceId, nodeId: frozen.nodeId, config: frozen.config, draftState: frozen.draftState ?? 'READY' })
        : await deps.api.getExportDraft(createdID!)
      if (disposed || request !== session) return
      if (previous && currentDraft.value?.id !== previous.id) return
      if ((!previous && draft.id !== createdID) || draft.dataSourceId !== frozen.dataSourceId || draft.nodeId !== frozen.nodeId || previous && (draft.id !== previous.id || draft.revision <= previous.revision) || (draft.draftState === 'EDITING') !== (frozen.draftState === 'EDITING') || configSignature(draft.config) !== configSignature(frozen.config)) throw new Error('草稿写入绑定无效')
      if (!previous && binding !== bindingVersion) { draftNotice.value = '保存期间更换了数据源或执行节点，请重新保存当前配置。'; return }
      currentDraft.value = draft; createdDraftID.value = draft.id; savedVersion.value = version
      if (binding !== bindingVersion || version !== formVersion.value) { draftNotice.value = '保存期间配置又有变化；请再次保存当前配置。'; return }
      await bindRoute(draft.id, request)
      if (disposed || request !== session || binding !== bindingVersion || version !== formVersion.value) return
      operationNotice.value = '草稿已保存。'
      if (advance) deps.moveToStep(5)
    } catch (error) {
      if (!disposed && request === session && binding === bindingVersion) draftFailure.value = exportDraftErrorMessage(error, '草稿保存失败或修订已变化，请重新读取后重试。')
    } finally { if (!disposed && request === session) creatingDraft.value = false }
  }

  async function loadCreatedDraft(id = createdDraftID.value) {
    if (id) await loadDerivedDraft(id, deps.route.query.draft === id)
    else if (lastRecovery) await loadDerivedDraft(lastRecovery.id, lastRecovery.routeBound)
  }

  onScopeDispose(() => { disposed = true; clearDraftState() })
  return { currentDraft, createdDraftID, creatingDraft, loadingDraft, bindingDraftRoute, draftFailure, draftLoadFailure, draftNotice, operationNotice, formVersion, hasInputs, draftDirty, createDraft, loadDerivedDraft, loadCreatedDraft, clearDraftState, invalidateDraftState }
}

// Go 的零值字段与缺省字段等价；比较活动配置时仍保留数值、非空字符串及数组顺序。
function configSignature(value: unknown): string | undefined {
  function canonical(item: unknown): unknown {
    if (item === undefined || item === null || item === false || item === '') return undefined
    if (Array.isArray(item)) { const items = item.map(canonical).filter(value => value !== undefined); return items.length ? items : undefined }
    if (typeof item === 'object') { const entries = Object.entries(item).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => [key, canonical(value)]).filter(([, value]) => value !== undefined); return entries.length ? Object.fromEntries(entries) : undefined }
    return item
  }
  return JSON.stringify(canonical(value))
}
