import { computed, ref, watch, type Ref } from 'vue'
import { executionNodeErrorMessage, type ApiError, type ExecutionNodeDetail } from '@/api/browser'
import { nodePrimaryAction } from './executionNodePresentation'
import { useNodeSession, type NodeApiFactory } from './nodeSession'

// 详情是节点事实的唯一所有者；读取与版本写入共同管理事实失效及有界的检查复读。
export function useNodeDetail(nodeID: Readonly<Ref<string>>, createApi?: NodeApiFactory) {
  const session = useNodeSession(nodeID, createApi)
  const node = ref<ExecutionNodeDetail>()
  const loading = ref(true)
  const refreshing = ref(false)
  const refreshCompleted = ref(false)
  const lastLoadedAt = ref('')
  const failure = ref('')
  const actionFailure = ref('')
  const actionNotice = ref('')
  const actionBusy = ref(false)
  const primaryAction = computed(() => node.value ? nodePrimaryAction(node.value) : undefined)
  let readVersion = 0
  let refreshTimer: ReturnType<typeof setTimeout> | undefined
  let inspectionReadDeadline = 0

  function scheduleInspectionRead(active: () => boolean, firstRead = false) {
    clearTimeout(refreshTimer)
    const pending = node.value?.environmentCheck?.status === 'PENDING' || node.value?.unavailableReasons.includes('ENVIRONMENT_CHECK_IN_PROGRESS')
    if ((!firstRead && !pending) || node.value?.heartbeatStatus !== 'ONLINE' || Date.now() >= inspectionReadDeadline) { inspectionReadDeadline = 0; return }
    // 实际版本查询晚于首次复读时继续串行读取，最多两分钟；路由/授权/卸载会使捕获会话失效。
    refreshTimer = setTimeout(() => { if (active()) void loadNode(true, true) }, 2500)
  }

  function invalidateRead() {
    readVersion++
    loading.value = refreshing.value = refreshCompleted.value = false
    clearTimeout(refreshTimer)
    inspectionReadDeadline = 0
  }
  session.onReset(() => {
    invalidateRead()
    node.value = undefined
    failure.value = actionFailure.value = actionNotice.value = ''
    lastLoadedAt.value = ''
    actionBusy.value = false
  })
  watch(nodeID, () => { void loadNode() }, { immediate: true, flush: 'sync' })

  async function loadNode(preserve = false, automatic = false) {
    if (!session.active() || actionBusy.value || (preserve && (loading.value || refreshing.value))) return
    const id = nodeID.value
    if (!id) { failure.value = '未指定执行节点。'; loading.value = false; return }
    const active = session.capture()
    const version = ++readVersion
    const current = () => active() && version === readVersion
    clearTimeout(refreshTimer)
    loading.value = !preserve
    refreshing.value = preserve
    refreshCompleted.value = false
    failure.value = ''
    try {
      const result = await session.api.getExecutionNode(id)
      if (!current()) return
      node.value = result
      lastLoadedAt.value = new Date().toLocaleString('zh-CN')
      refreshCompleted.value = preserve && !automatic
      scheduleInspectionRead(active)
    } catch (error) {
      if (!current()) return
      inspectionReadDeadline = 0
      if ([401, 403, 404].includes((error as Partial<ApiError>).status ?? 0)) { node.value = undefined; actionNotice.value = ''; lastLoadedAt.value = '' }
      failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
    } finally {
      if (current()) loading.value = refreshing.value = false
    }
  }

  async function performPrimaryAction() {
    const current = node.value
    const action = primaryAction.value
    if (!session.active() || !current || !action || actionBusy.value) return
    const active = session.capture()
    invalidateRead()
    actionBusy.value = true
    actionFailure.value = actionNotice.value = ''
    let refreshAfterFailure = false
    try {
      const result = action === 'enable'
        ? await session.api.enableExecutionNode(current.id, current.revision)
        : await session.api.requestExecutionNodeEnvironmentCheck(current.id, current.revision)
      if (!active()) return
      node.value = result
      actionNotice.value = action === 'enable'
        ? '节点已启用。'
        : '已请求环境检查。'
      if (action === 'environment-check') { inspectionReadDeadline = Date.now() + 120000; scheduleInspectionRead(active, true) }
    } catch (error) {
      if (!active()) return
      actionFailure.value = executionNodeErrorMessage(error, action === 'enable' ? '执行节点启用失败。' : '环境检查请求失败。')
      refreshAfterFailure = true
    } finally {
      if (active()) actionBusy.value = false
    }
    if (active() && refreshAfterFailure) await loadNode(true)
  }
  return { session, node, loading, refreshing, refreshCompleted, lastLoadedAt, failure, actionFailure, actionNotice, actionBusy, primaryAction, loadNode, performPrimaryAction }
}
