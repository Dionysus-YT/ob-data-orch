import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { DirectLoadApi, DirectLoadConfig, DirectLoadDraft } from '@/api/directLoad'
import { directLoadSignature, hydrateDirectLoad } from './directLoadForm'

/** 草稿是修订、读写和脏状态的唯一所有者；保存响应不覆盖仍在编辑的表单。 */
export function useDirectLoadDraft(deps: { api: DirectLoadApi; form: DirectLoadConfig; input: Readonly<Ref<DirectLoadConfig>>; bindRoute(id: string): Promise<unknown> }) {
  const currentDraft = ref<DirectLoadDraft>(), loadingDraft = ref(false), savingDraft = ref(false), bindingDraftRoute = ref(false)
  const draftFailure = ref(''), draftLoadFailure = ref(''), operationNotice = ref(''), needsRebind = ref(false)
  const formSignature = computed(() => JSON.stringify(deps.form)), savedSignature = ref(formSignature.value)
  const draftDirty = computed(() => formSignature.value !== savedSignature.value)
  const rerunLocked = computed(() => currentDraft.value?.parametersLocked === true || currentDraft.value?.sourceDerivation === 'RERUN_FROM_SCRATCH')
  let disposed = false, epoch = 0, formVersion = 0
  watch(formSignature, () => { formVersion++; needsRebind.value = true; operationNotice.value = '' }, { flush: 'sync' })
  function resetDraft() { epoch++; currentDraft.value = undefined; loadingDraft.value = savingDraft.value = bindingDraftRoute.value = needsRebind.value = false; draftFailure.value = draftLoadFailure.value = operationNotice.value = ''; savedSignature.value = formSignature.value }
  async function loadDraft(id: string) {
    const requested = ++epoch, version = formVersion; loadingDraft.value = true; draftFailure.value = draftLoadFailure.value = ''; currentDraft.value = undefined
    try { const draft = await deps.api.getDirectLoadDraft(id); if (disposed || requested !== epoch) return; if (draft.id !== id || version !== formVersion) throw new Error(); hydrateDirectLoad(deps.form, draft.config); currentDraft.value = draft; savedSignature.value = formSignature.value; needsRebind.value = false }
    catch { if (!disposed && requested === epoch) draftFailure.value = draftLoadFailure.value = '无法读取旁路导入草稿，请检查权限后重试。' }
    finally { if (!disposed && requested === epoch) loadingDraft.value = false }
  }
  async function saveDraft(): Promise<DirectLoadDraft | undefined> {
    if (disposed || loadingDraft.value || savingDraft.value || draftLoadFailure.value || !deps.form.dataSourceId) return
    if (rerunLocked.value && currentDraft.value && directLoadSignature(currentDraft.value.config) !== directLoadSignature(deps.input.value)) { draftFailure.value = '从头重新执行保留原配置；修改参数请基于原配置新建。'; return }
    const requested = epoch, version = formVersion, signature = formSignature.value, config = JSON.parse(JSON.stringify(deps.input.value)) as DirectLoadConfig, previous = currentDraft.value
    savingDraft.value = true; draftFailure.value = operationNotice.value = ''
    try {
      const draft = previous ? await deps.api.updateDirectLoadDraft(previous, config) : await deps.api.createDirectLoadDraft(config)
      if (disposed || requested !== epoch) return
      if (directLoadSignature(draft.config) !== directLoadSignature(config) || previous && (draft.id !== previous.id || draft.revision <= previous.revision || draft.sourceTaskId !== previous.sourceTaskId || draft.sourceDerivation !== previous.sourceDerivation || !!draft.parametersLocked !== !!previous.parametersLocked)) throw new Error()
      currentDraft.value = draft; savedSignature.value = signature; needsRebind.value = version !== formVersion
      if (!previous) { bindingDraftRoute.value = true; try { await deps.bindRoute(draft.id) } finally { if (!disposed && requested === epoch) bindingDraftRoute.value = false } }
      if (disposed || requested !== epoch || version !== formVersion) return
      operationNotice.value = '旁路导入草稿已保存'; return draft
    } catch { if (!disposed && requested === epoch) draftFailure.value = '草稿保存失败或修订已变化，请重新读取后重试。' }
    finally { if (!disposed && requested === epoch) savingDraft.value = false }
  }
  async function ensureSaved() { return currentDraft.value && !draftDirty.value && !needsRebind.value ? currentDraft.value : saveDraft() }
  function invalidateBinding() { needsRebind.value = true }
  onScopeDispose(() => { disposed = true; resetDraft() })
  return { currentDraft, loadingDraft, savingDraft, bindingDraftRoute, draftFailure, draftLoadFailure, operationNotice, formSignature, draftDirty, rerunLocked, loadDraft, saveDraft, resetDraft, ensureSaved, invalidateBinding }
}
export type DirectLoadDraftState = ReturnType<typeof useDirectLoadDraft>
