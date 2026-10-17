import { computed, onScopeDispose, ref } from 'vue'
import { dataSourceErrorMessage, executionNodeErrorMessage, type BrowserApi, type DataSourceSummary, type ExecutionNodeSummary } from '@/api/browser'
import type { NormalImportForm } from './normalImportForm'
import { isNormalImportSource } from './normalImportRules'

export type NormalImportReferenceApi = Pick<BrowserApi, 'listDataSources' | 'listExecutionNodes'>

// 引用读取只使用已存在的授权列表，不借用导出候选资格或发起任何连接探测。
export function useNormalImportReferences(form: NormalImportForm, api: NormalImportReferenceApi) {
  const sources = ref<DataSourceSummary[]>([])
  const nodes = ref<ExecutionNodeSummary[]>([])
  const loadingSources = ref(false)
  const loadingNodes = ref(false)
  const sourceFailure = ref('')
  const nodeFailure = ref('')
  const sourceKeyword = ref('')
  const sourceEnvironment = ref('')
  let disposed = false
  let sourceEpoch = 0
  let nodeEpoch = 0

  const eligibleSources = computed(() => sources.value.filter(isNormalImportSource))
  const visibleSources = computed(() => eligibleSources.value.filter(source =>
    (!sourceEnvironment.value || source.environment === sourceEnvironment.value)
    && source.displayName.toLocaleLowerCase().includes(sourceKeyword.value.trim().toLocaleLowerCase()),
  ))
  const selectedSource = computed(() => !sourceFailure.value ? eligibleSources.value.find(source => source.id === form.dataSourceId) : undefined)
  const selectedNode = computed(() => !nodeFailure.value ? nodes.value.find(node => node.id === form.nodeId) : undefined)

  async function loadSources() {
    if (disposed) return
    const epoch = ++sourceEpoch
    loadingSources.value = true
    sourceFailure.value = ''
    try {
      const items = await api.listDataSources()
      if (disposed || epoch !== sourceEpoch) return
      sources.value = items
      if (form.dataSourceId && !eligibleSources.value.some(source => source.id === form.dataSourceId)) form.dataSourceId = ''
    } catch (error) {
      if (disposed || epoch !== sourceEpoch) return
      sources.value = []
      sourceFailure.value = dataSourceErrorMessage(error, '无法读取已授权数据源，请重试。')
    } finally {
      if (!disposed && epoch === sourceEpoch) loadingSources.value = false
    }
  }

  async function loadNodes() {
    if (disposed) return
    const epoch = ++nodeEpoch
    loadingNodes.value = true
    nodeFailure.value = ''
    try {
      const items = await api.listExecutionNodes()
      if (disposed || epoch !== nodeEpoch) return
      nodes.value = items
      if (form.nodeId && !items.some(node => node.id === form.nodeId && node.acceptsNewTasks)) form.nodeId = ''
    } catch (error) {
      if (disposed || epoch !== nodeEpoch) return
      nodes.value = []
      nodeFailure.value = executionNodeErrorMessage(error, '无法读取已授权执行节点，请重试。')
    } finally {
      if (!disposed && epoch === nodeEpoch) loadingNodes.value = false
    }
  }

  // 会话代在发起新读取前递增，A → B → A 也不能接纳第一代响应或 finally。
  function resetReferences() {
    sourceEpoch++
    nodeEpoch++
    sources.value = []
    nodes.value = []
    sourceFailure.value = ''
    nodeFailure.value = ''
    sourceKeyword.value = ''
    sourceEnvironment.value = ''
    loadingSources.value = false
    loadingNodes.value = false
  }

  onScopeDispose(() => { disposed = true; resetReferences() })
  return {
    sources, nodes, loadingSources, loadingNodes, sourceFailure, nodeFailure, sourceKeyword, sourceEnvironment,
    eligibleSources, visibleSources, selectedSource, selectedNode, loadSources, loadNodes, resetReferences,
  }
}

export type NormalImportReferences = ReturnType<typeof useNormalImportReferences>
