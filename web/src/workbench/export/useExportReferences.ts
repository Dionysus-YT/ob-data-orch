import { ref, computed, onScopeDispose } from 'vue'
import { type DataSourceSummary, type ExecutionNodeCandidate, type StorageCredentialListItem, dataSourceErrorMessage, exportDraftErrorMessage, storageCredentialErrorMessage } from '@/api/browser'
import { isExportEligibleDataSource } from './exportDataSourceEligibility'
import type { ExportForm } from './exportForm'
import type { ExportRuntime } from './exportRuntime'

// useExportReferences 管理授权引用数据与加载状态。
export function useExportReferences(deps: Pick<ExportForm, 'selectedDataSourceID' | 'selectedNodeID' | 'storageCredentialID'> & Pick<ExportRuntime, 'api'>) {
  const { selectedDataSourceID, selectedNodeID, storageCredentialID, api } = deps
  const sources = ref<DataSourceSummary[]>([])
  const nodes = ref<ExecutionNodeCandidate[]>([])

  const loadingSources = ref(true)
  const loadingNodes = ref(true)

  const sourceLoadFailure = ref('')
  const nodeLoadFailure = ref('')

  const sourceKeyword = ref('')
  const sourceEnvironment = ref('')

  // EX-I6 存储凭据槽位（2026-08-14）：对象存储输出可选绑定主体拥有的凭据引用。
  // 只保存标识与当前修订；密钥永远由任务级安全槽位解析，不会进入页面状态或草稿。
  const storageCredentials = ref<StorageCredentialListItem[]>([])
  const loadingStorageCredentials = ref(false)

  const storageCredentialLoadFailure = ref('')
  const eligibleSources = computed(() => sources.value.filter(isExportEligibleDataSource))

  const visibleSources = computed(() => eligibleSources.value.filter((source) =>
    (!sourceEnvironment.value || source.environment === sourceEnvironment.value)
    && (!sourceKeyword.value.trim() || source.displayName.toLocaleLowerCase().includes(sourceKeyword.value.trim().toLocaleLowerCase())),
  ))

  const selectedSource = computed(() => eligibleSources.value.find((source) => source.id === selectedDataSourceID.value))
  const selectedSourceRevision = computed(() => selectedSource.value?.revision ?? 0)

  const selectedNode = computed(() => nodes.value.find((node) => node.id === selectedNodeID.value))

  let disposed = false
  const epochs = { sources: 0, nodes: 0, credentials: 0 }
  onScopeDispose(() => { disposed = true })

  async function loadSources() {
    if (disposed) return
    const epoch = ++epochs.sources
    loadingSources.value = true
    sourceLoadFailure.value = ''
    try {
      const items = await api.listDataSources()
      if (disposed || epoch !== epochs.sources) return
      sources.value = items
      if (!eligibleSources.value.some((source) => source.id === selectedDataSourceID.value)) selectedDataSourceID.value = ''
    } catch (error) {
      if (disposed || epoch !== epochs.sources) return
      sourceLoadFailure.value = dataSourceErrorMessage(error, '无法加载可选数据源，请稍后重试。')
    } finally {
      if (!disposed && epoch === epochs.sources) loadingSources.value = false
    }
  }

  async function loadNodeCandidates() {
    if (disposed) return
    const epoch = ++epochs.nodes
    loadingNodes.value = true
    nodeLoadFailure.value = ''
    try {
      const items = await api.listExportNodeCandidates()
      if (disposed || epoch !== epochs.nodes) return
      nodes.value = items
      if (!nodes.value.some((node) => node.id === selectedNodeID.value)) selectedNodeID.value = nodes.value.length === 1 ? nodes.value[0]!.id : ''
    } catch (error) {
      if (disposed || epoch !== epochs.nodes) return
      nodeLoadFailure.value = exportDraftErrorMessage(error, '无法加载可选执行节点，请稍后重试。')
    } finally {
      if (!disposed && epoch === epochs.nodes) loadingNodes.value = false
    }
  }

  async function loadStorageCredentials() {
    if (disposed) return
    const epoch = ++epochs.credentials
    loadingStorageCredentials.value = true
    storageCredentialLoadFailure.value = ''
    try {
      const items = await api.listStorageCredentials()
      if (disposed || epoch !== epochs.credentials) return
      storageCredentials.value = items
      // 当前选择已被删除或轮换出列表时清除引用，避免提交陈旧绑定。
      if (!storageCredentials.value.some((credential) => credential.id === storageCredentialID.value)) storageCredentialID.value = ''
    } catch (error) {
      if (disposed || epoch !== epochs.credentials) return
      storageCredentialLoadFailure.value = storageCredentialErrorMessage(error, '无法加载存储凭据，请稍后重试。')
    } finally {
      if (!disposed && epoch === epochs.credentials) loadingStorageCredentials.value = false
    }
  }

  return {
    sources, nodes, loadingSources, loadingNodes, sourceLoadFailure, nodeLoadFailure, sourceKeyword,
    sourceEnvironment, storageCredentials, loadingStorageCredentials, storageCredentialLoadFailure,
    eligibleSources, visibleSources, selectedSource, selectedSourceRevision, selectedNode, loadSources,
    loadNodeCandidates, loadStorageCredentials,
  }
}

export type ExportReferences = ReturnType<typeof useExportReferences>
