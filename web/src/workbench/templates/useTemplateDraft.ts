import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { dataSourceErrorMessage, taskDetailErrorMessage, type ConfigTemplateItem, type DataSourceSummary, type ExecutionNodeCandidate, type SupportedTaskType } from '@/api/browser'
import { isExportEligibleDataSource } from '@/workbench/export/exportDataSourceEligibility'
import { isNormalImportSource } from '@/workbench/import/normal/normalImportRules'
import { templateWriteUncertain } from './templateRules'
import { withTemplateWriteDeadline } from './templateWrites'
import type { useTemplateCatalog } from './useTemplateCatalog'

type CreatedDraft = { draftId: string; taskType: Exclude<SupportedTaskType, 'OBLOADER_DIRECT_LOAD'> }

// 绑定、候选与一次写入身份只有一个所有者；明确保存后的导航失败不会重新创建草稿。
export function useTemplateDraft(catalog: ReturnType<typeof useTemplateCatalog>, created: (id: string, type: SupportedTaskType) => Promise<unknown>) {
  const selectedTemplate = shallowRef<ConfigTemplateItem>()
  const draftSourceID = ref(''), draftNodeID = ref('')
  const sources = ref<DataSourceSummary[]>([]), nodes = ref<ExecutionNodeCandidate[]>([])
  const sourcesLoading = ref(false), createDraftFailure = ref(''), resultUnknown = ref(false)
  const createdDraft = shallowRef<CreatedDraft>()
  const eligibleSources = computed(() => sources.value.filter(selectedTemplate.value?.taskType === 'OBLOADER_NORMAL_IMPORT' ? isNormalImportSource : isExportEligibleDataSource))
  const canCreate = computed(() => Boolean(selectedTemplate.value && catalog.allowed(selectedTemplate.value, 'CREATE_DRAFT') && !sourcesLoading.value && !catalog.actionBusy.value && !resultUnknown.value && !createdDraft.value && eligibleSources.value.some(item => item.id === draftSourceID.value) && nodes.value.some(item => item.id === draftNodeID.value)))
  let selectionVersion = 0, readVersion = 0, bindingVersion = 0, loaded = false, pending: Promise<void> | undefined
  let intent: { key: string; template: ConfigTemplateItem; sourceID: string; nodeID: string; selection: number; binding: number } | undefined
  watch([draftSourceID, draftNodeID], () => { bindingVersion++ }, { flush: 'sync' })
  function clearSelection() {
    selectionVersion++; readVersion++; pending = undefined; loaded = false; intent = undefined
    selectedTemplate.value = undefined; draftSourceID.value = draftNodeID.value = ''
    sources.value = []; nodes.value = []; sourcesLoading.value = false; createDraftFailure.value = ''
    resultUnknown.value = false; createdDraft.value = undefined
  }
  watch([catalog.accessDenied, catalog.templates], () => {
    if (selectedTemplate.value && !catalog.allowed(selectedTemplate.value, 'CREATE_DRAFT')) clearSelection()
  }, { flush: 'sync' })
  onScopeDispose(clearSelection)
  function selectTemplate(template: ConfigTemplateItem) {
    if (!catalog.allowed(template, 'CREATE_DRAFT') || catalog.actionBusy.value) return
    clearSelection(); selectedTemplate.value = template
    void loadChoices()
  }
  function loadChoices(refresh = false): Promise<void> {
    const template = selectedTemplate.value
    if (!template || !catalog.allowed(template, 'CREATE_DRAFT') || catalog.actionBusy.value || resultUnknown.value || createdDraft.value) return Promise.resolve()
    if (pending) return pending
    if (loaded && !refresh) return Promise.resolve()
    const selection = selectionVersion, read = ++readVersion
    const active = () => catalog.active() && selection === selectionVersion && read === readVersion && catalog.allowed(template, 'CREATE_DRAFT')
    sourcesLoading.value = true; createDraftFailure.value = ''
    const request = (async () => {
      try {
        // 当前类型的两份引用必须一起发布，半份结果不能获得创建资格。
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
        if (!active()) return
        catalog.denied(error)
        if (!active()) return
        loaded = false; sources.value = []; nodes.value = []
        draftSourceID.value = draftNodeID.value = ''
        createDraftFailure.value = dataSourceErrorMessage(error, '无法加载数据源或执行节点。')
      } finally { if (active()) { sourcesLoading.value = false; pending = undefined } }
    })()
    pending = request
    return request
  }
  function onNodeDropdownVisibleChange(open: boolean) { if (open) void loadChoices(true) }
  function selectSource(value: string) { if (!catalog.actionBusy.value && !resultUnknown.value && !createdDraft.value) draftSourceID.value = value }
  function selectNode(value: string) { if (!catalog.actionBusy.value && !resultUnknown.value && !createdDraft.value) draftNodeID.value = value }
  const intentActive = () => Boolean(intent && catalog.active() && intent.selection === selectionVersion && intent.binding === bindingVersion && selectedTemplate.value === intent.template && catalog.allowed(intent.template, 'CREATE_DRAFT'))
  async function openCreatedDraft() {
    const result = createdDraft.value
    if (!result || !intentActive()) return
    createDraftFailure.value = ''
    try { await created(result.draftId, result.taskType) }
    catch { if (intentActive()) createDraftFailure.value = '草稿已创建，暂时无法打开。请重新打开草稿。' }
  }
  async function performCreate() {
    const request = intent
    if (!request || !intentActive()) return
    const finish = catalog.beginAction(request.template.id)
    if (!finish) return
    const active = () => intent === request && intentActive()
    const previouslyUnknown = resultUnknown.value
    createDraftFailure.value = ''
    try {
      const result = await withTemplateWriteDeadline(() => catalog.api.createTaskDraftFromTemplate(request.template.id, request.sourceID, request.nodeID, request.template.taskType, request.key), catalog.signal)
      if (!active()) return
      if (result.taskType !== request.template.taskType || !result.draftId) throw new Error('草稿响应无法核对')
      resultUnknown.value = false
      createdDraft.value = result
      await openCreatedDraft()
    } catch (error) {
      if (!active()) return
      catalog.denied(error)
      if (!active()) return
      resultUnknown.value = previouslyUnknown || templateWriteUncertain(error)
      createDraftFailure.value = resultUnknown.value ? '创建结果待确认。可用原请求重试，或前往“我的草稿”核对。' : taskDetailErrorMessage(error, '由模板创建草稿未完成。')
      if (!resultUnknown.value) intent = undefined
    } finally { finish() }
  }
  async function createDraft() {
    const template = selectedTemplate.value
    if (!template || !canCreate.value) return
    intent = { key: crypto.randomUUID(), template, sourceID: draftSourceID.value, nodeID: draftNodeID.value, selection: selectionVersion, binding: bindingVersion }
    await performCreate()
  }
  async function retryCreate() { if (resultUnknown.value && intentActive() && !catalog.actionBusy.value) await performCreate() }
  return { selectedTemplate, draftSourceID, draftNodeID, sourcesLoading, createDraftFailure, eligibleSources, nodes, canCreate, selectTemplate, clearSelection, loadChoices, onNodeDropdownVisibleChange, createDraft, resultUnknown, createdDraft, retryCreate, openCreatedDraft, selectSource, selectNode }
}
