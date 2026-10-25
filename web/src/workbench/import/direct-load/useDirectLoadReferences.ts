import { computed, onScopeDispose, ref } from 'vue'
import type { BrowserApi, DataSourceSummary, ExecutionNodeSummary } from '@/api/browser'
import type { DirectLoadApi, DirectLoadConfig, DirectLoadParameter } from '@/api/directLoad'
import { directLoadLocalParameters, isDirectLoadSource } from './directLoadRules'

export type DirectLoadReferenceApi = Pick<BrowserApi, 'listDataSources' | 'listExecutionNodes'> & Pick<DirectLoadApi, 'getDirectLoadParameters'>
/** 授权引用和只读参数目录各有请求代；失败、刷新和卸载均撤销旧资格。 */
export function useDirectLoadReferences(form: DirectLoadConfig, api: DirectLoadReferenceApi) {
  const sources = ref<DataSourceSummary[]>([]), nodes = ref<ExecutionNodeSummary[]>([]), metadata = ref<DirectLoadParameter[]>([])
  const loadingSources = ref(false), loadingNodes = ref(false), loadingMetadata = ref(false)
  const sourceKeyword = ref(''), sourceEnvironment = ref('')
  const sourceFailure = ref(''), nodeFailure = ref(''), metadataFailure = ref('')
  let disposed = false, sourceEpoch = 0, nodeEpoch = 0, metadataEpoch = 0
  const eligibleSources = computed(() => sources.value.filter(isDirectLoadSource))
  const visibleSources = computed(() => { const keyword = sourceKeyword.value.trim().toLocaleLowerCase(); return eligibleSources.value.filter(source => (!sourceEnvironment.value || source.environment === sourceEnvironment.value) && (!keyword || source.displayName.toLocaleLowerCase().includes(keyword))) })
  const selectedSource = computed(() => !sourceFailure.value && !loadingSources.value ? eligibleSources.value.find(source => source.id === form.dataSourceId) : undefined)
  const selectedNode = computed(() => !nodeFailure.value && !loadingNodes.value ? nodes.value.find(node => node.id === form.nodeId && node.acceptsNewTasks) : undefined)
  async function loadSources() {
    if (disposed) return
    const epoch = ++sourceEpoch; loadingSources.value = true; sourceFailure.value = ''
    try { const items = await api.listDataSources(); if (!disposed && epoch === sourceEpoch) sources.value = items }
    catch { if (!disposed && epoch === sourceEpoch) { sources.value = []; sourceFailure.value = '无法读取授权数据源，请重试。' } }
    finally { if (!disposed && epoch === sourceEpoch) loadingSources.value = false }
  }
  async function loadNodes() {
    if (disposed) return
    const epoch = ++nodeEpoch; loadingNodes.value = true; nodeFailure.value = ''
    try { const items = await api.listExecutionNodes({ forTaskSelection: true }); if (!disposed && epoch === nodeEpoch) nodes.value = items }
    catch { if (!disposed && epoch === nodeEpoch) { nodes.value = []; nodeFailure.value = '无法读取授权执行节点，请重试。' } }
    finally { if (!disposed && epoch === nodeEpoch) loadingNodes.value = false }
  }
  async function loadMetadata() {
    if (disposed || loadingMetadata.value) return
    const epoch = ++metadataEpoch; loadingMetadata.value = true; metadataFailure.value = ''
    try { const items = await api.getDirectLoadParameters(); if (!disposed && epoch === metadataEpoch) metadata.value = directLoadLocalParameters(items) }
    catch { if (!disposed && epoch === metadataEpoch) { metadata.value = []; metadataFailure.value = '旁路参数目录无法安全读取，请重试。' } }
    finally { if (!disposed && epoch === metadataEpoch) loadingMetadata.value = false }
  }
  function resetReferences() { sourceEpoch++; nodeEpoch++; sources.value = []; nodes.value = []; sourceKeyword.value = sourceEnvironment.value = ''; sourceFailure.value = nodeFailure.value = ''; loadingSources.value = loadingNodes.value = false }
  onScopeDispose(() => { disposed = true; metadataEpoch++; resetReferences() })
  return { sources, nodes, metadata, sourceKeyword, sourceEnvironment, visibleSources, loadingSources, loadingNodes, loadingMetadata, sourceFailure, nodeFailure, metadataFailure, eligibleSources, selectedSource, selectedNode, loadSources, loadNodes, loadMetadata, resetReferences }
}
