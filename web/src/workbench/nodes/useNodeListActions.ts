import { ref, watch } from 'vue'
import { executionNodeErrorMessage, type ExecutionNodeSummary } from '@/api/browser'
import { nodePrimaryAction } from './executionNodePresentation'
import type { useNodeList } from './useNodeList'

export function useNodeListActions(list: ReturnType<typeof useNodeList>) {
  const { session } = list
  const actionFailure = ref('')
  const notice = ref('')
  const actionBusy = ref('')
  const deletionTarget = ref<ExecutionNodeSummary>()
  let refreshTimer: ReturnType<typeof setTimeout> | undefined
  session.onReset(() => { clearTimeout(refreshTimer); deletionTarget.value = undefined; actionFailure.value = notice.value = actionBusy.value = '' })
  watch(list.restricted, restricted => { if (restricted) { deletionTarget.value = undefined; notice.value = '' } }, { flush: 'sync' })

  async function runPrimaryAction(node: ExecutionNodeSummary, action: 'environment-check' | 'enable') {
    if (!session.active() || actionBusy.value || action !== nodePrimaryAction(node)) return
    const active = session.capture()
    clearTimeout(refreshTimer)
    list.beginMutation()
    actionBusy.value = node.id
    actionFailure.value = notice.value = ''
    let refreshAfterFailure = false
    try {
      const updated = action === 'enable'
        ? await session.api.enableExecutionNode(node.id, node.revision)
        : await session.api.requestExecutionNodeEnvironmentCheck(node.id, node.revision)
      if (!active()) return
      list.acceptNode(updated)
      notice.value = action === 'enable'
        ? `节点“${updated.displayName}”已启用。`
        : `已请求“${updated.displayName}”检查环境。`
      if (action === 'environment-check') refreshTimer = setTimeout(() => { if (active()) void list.loadNodes(true) }, 2500)
    } catch (error) {
      if (!active()) return
      actionFailure.value = executionNodeErrorMessage(error, action === 'enable' ? '执行节点启用失败。' : '环境检查请求失败。')
      refreshAfterFailure = true
    } finally {
      if (active()) { list.endMutation(); actionBusy.value = '' }
    }
    if (active() && refreshAfterFailure) await list.loadNodes(true)
  }
  async function deleteNode() {
    const target = deletionTarget.value
    if (!session.active() || list.restricted.value || !target || actionBusy.value) return
    const active = session.capture()
    clearTimeout(refreshTimer)
    list.beginMutation()
    actionBusy.value = target.id
    actionFailure.value = notice.value = ''
    try {
      const result = await session.api.deleteExecutionNode(target.id, target.revision)
      if (!active()) return
      notice.value = result.agentAccessRevoked
        ? `节点“${target.displayName}”已永久删除；Agent 身份和未使用注册码已撤销。`
        : `节点“${target.displayName}”已永久删除。`
    } catch (error) {
      if (active()) actionFailure.value = executionNodeErrorMessage(error, '执行节点永久删除失败。')
    } finally {
      if (active()) { list.endMutation(); deletionTarget.value = undefined; actionBusy.value = '' }
    }
    if (active()) await list.loadNodes(true)
  }
  return { actionFailure, notice, actionBusy, deletionTarget, runPrimaryAction, deleteNode }
}
