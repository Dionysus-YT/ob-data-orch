import { computed, ref } from 'vue'
import { executionNodeErrorMessage, type ApiError, type ExecutionNodeSummary } from '@/api/browser'
import { useNodeSession, type NodeApiFactory } from './nodeSession'

// 列表接口返回当前身份的完整授权范围；筛选只处理已返回事实，不扩展对象可见性。
export function useNodeList(createApi?: NodeApiFactory) {
  const session = useNodeSession(ref('list'), createApi)
  const nodes = ref<ExecutionNodeSummary[]>([])
  const keyword = ref('')
  const management = ref('')
  const heartbeat = ref('')
  const environment = ref('')
  const acceptance = ref('')
  const loading = ref(true)
  const refreshing = ref(false)
  const restricted = ref(false)
  const failure = ref('')
  const lastLoadedAt = ref('')
  let readVersion = 0
  let mutations = 0
  const hasFilters = computed(() => Boolean(keyword.value || management.value || heartbeat.value || environment.value || acceptance.value))
  const visibleNodes = computed(() => {
    const query = keyword.value.trim().toLocaleLowerCase()
    return nodes.value.filter(node =>
      (!query || node.displayName.toLocaleLowerCase().includes(query) || node.id.toLocaleLowerCase().includes(query)) &&
      (!management.value || node.managementState === management.value) &&
      (!heartbeat.value || node.heartbeatStatus === heartbeat.value) &&
      (!environment.value || node.environmentStatus === environment.value) &&
      (!acceptance.value || String(node.acceptsNewTasks) === acceptance.value))
  })
  function invalidateRead() { readVersion++; loading.value = refreshing.value = false }
  session.onReset(invalidateRead)
  // 写事务开始时使旧列表读取失效，避免读取的旧修订覆盖操作结果。
  function beginMutation() { invalidateRead(); mutations++ }
  function endMutation() { mutations-- }
  function acceptNode(updated: ExecutionNodeSummary) { nodes.value = nodes.value.map(current => current.id === updated.id ? updated : current) }
  async function loadNodes(preserve = false) {
    if (!session.active() || mutations) return
    const active = session.capture()
    const version = ++readVersion
    const valid = () => active() && version === readVersion
    loading.value = !preserve
    refreshing.value = preserve
    failure.value = ''
    try {
      const result = await session.api.listExecutionNodes()
      if (!valid()) return
      nodes.value = result
      restricted.value = false
      lastLoadedAt.value = new Date().toLocaleString('zh-CN')
    } catch (error) {
      if (!valid()) return
      const status = (error as Partial<ApiError>).status
      restricted.value = status === 401 || status === 403
      if (!preserve || restricted.value) nodes.value = []
      failure.value = executionNodeErrorMessage(error, '无法读取执行节点，请稍后重试。')
    } finally {
      if (valid()) loading.value = refreshing.value = false
    }
  }
  function clearFilters() { keyword.value = management.value = heartbeat.value = environment.value = acceptance.value = '' }
  return { session, nodes, keyword, management, heartbeat, environment, acceptance, loading, refreshing, restricted, failure, lastLoadedAt, hasFilters, visibleNodes, loadNodes, clearFilters, beginMutation, endMutation, acceptNode }
}
