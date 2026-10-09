import { computed, ref, watch, type Ref } from 'vue'
import { executionNodeErrorMessage, type ApiError, type ExecutionNodeDetail } from '@/api/browser'
import { nodePrimaryAction } from './executionNodePresentation'
import { useNodeSession, type NodeApiFactory } from './nodeSession'

// 详情是节点事实的唯一所有者；读取与版本写入共同管理事实失效及一次延迟复读。
export function useNodeDetail(nodeID: Readonly<Ref<string>>, createApi?: NodeApiFactory) {
  const session = useNodeSession(nodeID, createApi)
  const node = ref<ExecutionNodeDetail>()
  const loading = ref(true)
  const refreshing = ref(false)
  const failure = ref('')
  const actionFailure = ref('')
  const actionNotice = ref('')
  const actionBusy = ref(false)
  const primaryAction = computed(() => node.value ? nodePrimaryAction(node.value) : undefined)
  let readVersion = 0
  let refreshTimer: ReturnType<typeof setTimeout> | undefined

  function invalidateRead() {
    readVersion++
    loading.value = refreshing.value = false
    clearTimeout(refreshTimer)
  }
  session.onReset(() => {
    invalidateRead()
    node.value = undefined
    failure.value = actionFailure.value = actionNotice.value = ''
    actionBusy.value = false
  })
  watch(nodeID, () => { void loadNode() }, { immediate: true, flush: 'sync' })

  async function loadNode(preserve = false) {
    if (!session.active() || actionBusy.value) return
    const id = nodeID.value
    if (!id) { failure.value = '未指定执行节点。'; loading.value = false; return }
    const active = session.capture()
    const version = ++readVersion
    const current = () => active() && version === readVersion
    loading.value = !preserve
    refreshing.value = preserve
    failure.value = ''
    try {
      const result = await session.api.getExecutionNode(id)
      if (current()) node.value = result
    } catch (error) {
      if (!current()) return
      if ([401, 403, 404].includes((error as Partial<ApiError>).status ?? 0)) node.value = undefined
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
        ? '节点已启用；提交具体任务前仍需任务级预检查。'
        : '环境检查已请求，等待 Agent 回传固定运行时结果。'
      if (action === 'environment-check') refreshTimer = setTimeout(() => { if (active()) void loadNode(true) }, 2500)
    } catch (error) {
      if (!active()) return
      actionFailure.value = executionNodeErrorMessage(error, action === 'enable' ? '执行节点启用失败。' : '环境检查请求失败。')
      refreshAfterFailure = true
    } finally {
      if (active()) actionBusy.value = false
    }
    if (active() && refreshAfterFailure) await loadNode(true)
  }
  return { session, node, loading, refreshing, failure, actionFailure, actionNotice, actionBusy, primaryAction, loadNode, performPrimaryAction }
}
