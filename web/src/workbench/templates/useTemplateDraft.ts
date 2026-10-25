import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { dataSourceErrorMessage, taskDetailErrorMessage, type ConfigTemplateItem, type DataSourceSummary, type ExecutionNodeCandidate, type SupportedTaskType } from '@/api/browser'
import { isExportEligibleDataSource } from '@/workbench/export/exportDataSourceEligibility'
import { isNormalImportSource } from '@/workbench/import/normal/normalImportRules'
import type { useTemplateCatalog } from './useTemplateCatalog'

// 选定模板后才建立绑定会话；类型、修订、权限或目录变化立即撤销旧候选和迟到导航。
export function useTemplateDraft(catalog: ReturnType<typeof useTemplateCatalog>, created: (id: string, type: SupportedTaskType) => Promise<unknown>) {
  const selectedTemplate = shallowRef<ConfigTemplateItem>()
  const draftSourceID = ref(''), draftNodeID = ref('')
  const sources = ref<DataSourceSummary[]>([]), nodes = ref<ExecutionNodeCandidate[]>([])
  const sourcesLoading = ref(false), createDraftFailure = ref('')
  const eligibleSources = computed(() => sources.value.filter(selectedTemplate.value?.taskType === 'OBLOADER_NORMAL_IMPORT' ? isNormalImportSource : isExportEligibleDataSource))
  const canCreate = computed(() => Boolean(selectedTemplate.value && !sourcesLoading.value && !catalog.actionBusy.value && !catalog.accessDenied.value && eligibleSources.value.some(item => item.id === draftSourceID.value) && nodes.value.some(item => item.id === draftNodeID.value)))
  let version = 0, loaded = false, pending: Promise<void> | undefined
  function clearSelection() {
    version++; pending = undefined; loaded = false
    selectedTemplate.value = undefined; draftSourceID.value = draftNodeID.value = ''
    sources.value = []; nodes.value = []; sourcesLoading.value = false; createDraftFailure.value = ''
  }
  watch([catalog.accessDenied, catalog.templates], () => {
    const selected = selectedTemplate.value
    if (catalog.accessDenied.value || selected && !catalog.templates.value.some(item => item.id === selected.id && item.revision === selected.revision && item.taskType === selected.taskType)) clearSelection()
  }, { flush: 'sync' })
  onScopeDispose(clearSelection)
  function selectTemplate(template: ConfigTemplateItem) {
    if (!catalog.active() || catalog.accessDenied.value || catalog.actionBusy.value) return
    if (selectedTemplate.value?.id === template.id && selectedTemplate.value.revision === template.revision && selectedTemplate.value.taskType === template.taskType) { void loadChoices(true); return }
    clearSelection(); selectedTemplate.value = template
    void loadChoices()
  }
  function loadChoices(refresh = false): Promise<void> {
    const template = selectedTemplate.value
    if (!template || !catalog.active() || catalog.accessDenied.value || catalog.actionBusy.value) return Promise.resolve()
    if (pending) return pending
    if (loaded && !refresh) return Promise.resolve()
    const current = ++version
    const active = () => catalog.active() && !catalog.accessDenied.value && current === version && selectedTemplate.value === template
    sourcesLoading.value = true; createDraftFailure.value = ''
    const request = (async () => {
      try {
        // 候选按模板类型读取；两份引用原子发布，不能把部分成功当成可提交绑定。
        const [sourceItems, nodeItems] = await Promise.all([
          catalog.api.listDataSources(),
          template.taskType === 'OBLOADER_NORMAL_IMPORT'
            ? catalog.api.listExecutionNodes().then(items => items.filter(item => item.acceptsNewTasks).map(({ id, displayName, platform }) => ({ id, displayName, platform })))
            : catalog.api.listExportNodeCandidates(),
        ])
        if (!active()) return
        sources.value = sourceItems; nodes.value = nodeItems
        if (!eligibleSources.value.some(source => source.id === draftSourceID.value)) draftSourceID.value = ''
        if (!nodes.value.some(node => node.id === draftNodeID.value)) draftNodeID.value = ''
        loaded = true
      } catch (error) {
        if (active()) {
          loaded = false; sources.value = []; nodes.value = []
          draftSourceID.value = draftNodeID.value = ''
          createDraftFailure.value = dataSourceErrorMessage(error, '无法加载数据源或执行节点。')
        }
      } finally { if (active()) { sourcesLoading.value = false; pending = undefined } }
    })()
    pending = request
    return request
  }
  function onNodeDropdownVisibleChange(open: boolean) { if (open) void loadChoices(true) }
  async function createDraft() {
    const template = selectedTemplate.value
    if (!template || !canCreate.value || !catalog.active()) return
    const sourceID = draftSourceID.value, nodeID = draftNodeID.value, current = version
    if (!catalog.beginAction(template.id)) return
    const active = () => catalog.active() && !catalog.accessDenied.value && current === version && selectedTemplate.value === template && draftSourceID.value === sourceID && draftNodeID.value === nodeID
    createDraftFailure.value = ''
    try {
      const result = await catalog.api.createTaskDraftFromTemplate(template.id, sourceID, nodeID, template.taskType)
      if (!active()) return
      catalog.notice.value = '草稿已创建。'
      await created(result.draftId, result.taskType)
    } catch (error) {
      if (active()) createDraftFailure.value = taskDetailErrorMessage(error, '由模板创建草稿失败。')
    } finally { catalog.endAction() }
  }
  return { selectedTemplate, draftSourceID, draftNodeID, sourcesLoading, createDraftFailure, eligibleSources, nodes, canCreate, selectTemplate, clearSelection, loadChoices, onNodeDropdownVisibleChange, createDraft }
}
