import { ref } from 'vue'
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
  session.onReset(() => { clearTimeout(refreshTimer); deletionTarget.value = undefined })

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
        ? `节点“${updated.displayName}”已启用；具体任务仍需预检查。`
        : `已请求“${updated.displayName}”检查环境，等待 Agent 回传。`
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
  async function deleteOrArchiveNode() {
    const target = deletionTarget.value
    if (!session.active() || !target || actionBusy.value) return
    const active = session.capture()
    clearTimeout(refreshTimer)
    list.beginMutation()
    actionBusy.value = target.id
    actionFailure.value = notice.value = ''
    try {
      const result = await session.api.deleteOrArchiveExecutionNode(target.id, target.revision)
      if (!active()) return
      notice.value = result.outcome === 'DELETED'
        ? `节点“${target.displayName}”已删除。`
        : result.agentAccessRevoked
          ? `节点“${target.displayName}”已归档；Agent 身份和未使用注册码已撤销。`
          : `节点“${target.displayName}”已归档。`
    } catch (error) {
      if (active()) actionFailure.value = executionNodeErrorMessage(error, '执行节点删除或归档失败。')
    } finally {
      if (active()) { list.endMutation(); deletionTarget.value = undefined; actionBusy.value = '' }
    }
    if (active()) await list.loadNodes(true)
  }
  return { actionFailure, notice, actionBusy, deletionTarget, runPrimaryAction, deleteOrArchiveNode }
}
