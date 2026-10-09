import { computed, onScopeDispose, ref } from 'vue'
import { dataSourceErrorMessage, exportDraftErrorMessage, type DataSourceSummary, type ExecutionNodeCandidate, type ExportConfigTemplateItem } from '@/api/browser'
import { isExportEligibleDataSource } from '@/workbench/export/exportDataSourceEligibility'
import type { useTemplateCatalog } from './useTemplateCatalog'

export function useTemplateDraft(catalog: ReturnType<typeof useTemplateCatalog>, created: (id: string) => Promise<unknown>) {
  const draftSourceID = ref('')
  const draftNodeID = ref('')
  const sources = ref<DataSourceSummary[]>([])
  const nodes = ref<ExecutionNodeCandidate[]>([])
  const sourcesLoading = ref(false)
  const createDraftFailure = ref('')
  const eligibleSources = computed(() => sources.value.filter(isExportEligibleDataSource))
  let pending: Promise<void> | undefined
  let loaded = false
  onScopeDispose(() => { draftSourceID.value = draftNodeID.value = '' })
  function loadChoices(): Promise<void> {
    if (!catalog.active() || loaded) return Promise.resolve()
    if (pending) return pending
    sourcesLoading.value = true
    createDraftFailure.value = ''
    pending = (async () => {
      try {
        // 两类引用原子发布；其中一项失败不得把半份列表当成已缓存结果。
        const [sourceItems, nodeItems] = await Promise.all([catalog.api.listDataSources(), catalog.api.listExportNodeCandidates()])
        if (!catalog.active()) return
        sources.value = sourceItems
        nodes.value = nodeItems
        loaded = true
      } catch (error) {
        if (catalog.active()) createDraftFailure.value = dataSourceErrorMessage(error, exportDraftErrorMessage(error, '无法加载数据源或执行节点。'))
      } finally {
        if (catalog.active()) sourcesLoading.value = false
        pending = undefined
      }
    })()
    return pending
  }
  async function createDraft(template: ExportConfigTemplateItem) {
    if (!catalog.active() || catalog.actionBusy.value) return
    createDraftFailure.value = ''
    const sourceID = draftSourceID.value
    const nodeID = draftNodeID.value
    if (!sourceID || !nodeID) { createDraftFailure.value = '请选择数据源与执行节点。'; return }
    if (!catalog.beginAction(template.id)) return
    try {
      const id = await catalog.api.createDraftFromTemplate(template.id, sourceID, nodeID)
      if (!catalog.active()) return
      catalog.notice.value = '草稿已创建；模板不复制凭据、预检查与风险确认，请重新完成预检查后再提交。'
      await created(id)
    } catch (error) {
      if (catalog.active()) createDraftFailure.value = exportDraftErrorMessage(error, '由模板创建草稿失败。')
    } finally {
      catalog.endAction()
    }
  }
  return { draftSourceID, draftNodeID, sourcesLoading, createDraftFailure, eligibleSources, nodes, loadChoices, createDraft }
}
