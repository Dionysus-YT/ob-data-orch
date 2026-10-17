import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { NormalImportApi, NormalImportConfig, NormalImportDraft } from '@/api/normalImport'
import type { NormalImportForm } from './normalImportForm'
import { hydrateNormalImport, normalImportConfigSignature } from './normalImportDraftInput'

// 草稿能力独占读取、保存与修订；迟到保存只更新本会话元数据，不覆盖继续编辑的表单。
export function useNormalImportDraft(deps: {
  api: NormalImportApi; form: NormalImportForm; input: Readonly<Ref<NormalImportConfig>>
  bindRoute(id: string): Promise<unknown>
}) {
  const currentDraft = ref<NormalImportDraft | null>(null)
  const loadingDraft = ref(false), savingDraft = ref(false), bindingDraftRoute = ref(false)
  const draftFailure = ref(''), draftLoadFailure = ref(''), operationNotice = ref('')
  const formSignature = computed(() => JSON.stringify(deps.form))
  const savedSignature = ref(formSignature.value)
  const draftDirty = computed(() => savedSignature.value !== formSignature.value)
  const draftNeedsRebind = ref(false)
  let disposed = false, epoch = 0, formVersion = 0, bindingVersion = 0
  watch(formSignature, () => { formVersion++; operationNotice.value = '' }, { flush: 'sync' })

  function resetDraft() {
    epoch++; currentDraft.value = null; loadingDraft.value = false; savingDraft.value = false
    bindingVersion++; draftNeedsRebind.value = false
    draftFailure.value = ''; draftLoadFailure.value = ''; bindingDraftRoute.value = false; operationNotice.value = ''; savedSignature.value = formSignature.value
  }
  function invalidateDraftBinding() { if (currentDraft.value) { bindingVersion++; draftNeedsRebind.value = true } }
  async function loadDraft(id: string) {
    if (disposed) return
    const request = ++epoch, version = formVersion
    loadingDraft.value = true; draftFailure.value = ''; draftLoadFailure.value = ''; currentDraft.value = null
    try {
      const draft = await deps.api.getNormalImportDraft(id)
      if (disposed || request !== epoch) return
      if (draft.id !== id) throw new Error('草稿读取绑定无效')
      if (version !== formVersion) { draftFailure.value = draftLoadFailure.value = '读取期间配置已变化，请重新打开草稿。'; return }
      hydrateNormalImport(deps.form, draft.config)
      currentDraft.value = draft; savedSignature.value = formSignature.value
    } catch {
      if (!disposed && request === epoch) draftFailure.value = draftLoadFailure.value = '无法读取普通导入草稿。请检查权限或重新打开。'
    } finally { if (!disposed && request === epoch) loadingDraft.value = false }
  }
  async function saveDraft(): Promise<NormalImportDraft | null> {
    if (disposed || loadingDraft.value || savingDraft.value || !deps.input.value.dataSourceId) return null
    const request = epoch, version = formVersion, binding = bindingVersion, signature = formSignature.value
    const input: NormalImportConfig = JSON.parse(JSON.stringify(deps.input.value)), previous = currentDraft.value
    savingDraft.value = true; draftFailure.value = ''; operationNotice.value = ''
    try {
      const draft = previous ? await deps.api.updateNormalImportDraft(previous, input) : await deps.api.createNormalImportDraft(input)
      if (disposed || request !== epoch) return null
      if (normalImportConfigSignature(draft.config) !== normalImportConfigSignature(input)) throw new Error('草稿活动配置绑定无效')
      if (previous && (draft.id !== previous.id || draft.revision <= previous.revision)) throw new Error('草稿修订无效')
      currentDraft.value = draft
      savedSignature.value = signature
      draftNeedsRebind.value = binding !== bindingVersion
      if (!previous) {
        bindingDraftRoute.value = true
        try { await deps.bindRoute(draft.id) }
        finally { if (!disposed && request === epoch) bindingDraftRoute.value = false }
      }
      if (disposed || request !== epoch || version !== formVersion || binding !== bindingVersion) return null
      operationNotice.value = '普通导入草稿已保存'
      return draft
    } catch {
      if (!disposed && request === epoch) draftFailure.value = '草稿保存失败或修订已变化，请重新读取后重试。'
      return null
    } finally { if (!disposed && request === epoch) savingDraft.value = false }
  }
  async function ensureSaved() {
    if (disposed || loadingDraft.value || savingDraft.value) return null
    return currentDraft.value && !draftDirty.value && !draftNeedsRebind.value ? currentDraft.value : saveDraft()
  }
  onScopeDispose(() => { disposed = true; resetDraft() })
  return { currentDraft, loadingDraft, savingDraft, bindingDraftRoute, draftDirty, draftNeedsRebind, draftFailure, draftLoadFailure, operationNotice, formSignature, loadDraft, saveDraft, resetDraft, ensureSaved, invalidateDraftBinding }
}
export type NormalImportDraftState = ReturnType<typeof useNormalImportDraft>
