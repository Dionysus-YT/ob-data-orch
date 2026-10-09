import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { Router } from 'vue-router'
import { taskDetailErrorMessage, type TaskExecution } from '@/api/browser'
import type { TaskDetailSession } from './taskDetailSession'

// 操作只持有当前会话的输入与忙状态；过期写操作不能跳转、清空新输入或写入新任务反馈。
export function useTaskActions(session: Readonly<Ref<TaskDetailSession | null>>, router: Pick<Router, 'push'>, execution: Readonly<Ref<TaskExecution | null>>) {
  const derivationFailure = ref('')
  const derivationBusy = ref('')
  const noticeTemplate = ref('')
  const templateNameInput = ref('')
  const savingTemplate = ref(false)
  // EX-I8：按钮只消费失败终态与 Agent 检查点事实；实际资格仍由服务端复验。
  const checkpointResumeAvailable = computed(() => execution.value?.state === 'FAILED' && execution.value?.resultSummary?.checkpointPresent === true)
  let disposed = false
  watch(session, () => {
    derivationFailure.value = derivationBusy.value = noticeTemplate.value = templateNameInput.value = ''
    savingTemplate.value = false
  }, { flush: 'sync' })
  onScopeDispose(() => { disposed = true })
  const accepts = (binding: TaskDetailSession) => !disposed && binding.active() && session.value === binding

  async function perform(kind: string, fallback: string, operation: (binding: TaskDetailSession) => Promise<void>) {
    const binding = session.value
    if (!binding || !accepts(binding) || derivationBusy.value || savingTemplate.value) return
    if (kind === 'SAVE_TEMPLATE') savingTemplate.value = true
    else derivationBusy.value = kind
    derivationFailure.value = ''
    try {
      await operation(binding)
    } catch (error) {
      if (accepts(binding)) derivationFailure.value = taskDetailErrorMessage(error, fallback)
    } finally {
      if (accepts(binding)) {
        derivationBusy.value = ''
        savingTemplate.value = false
      }
    }
  }

  function rebuildFromTask(derivation: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH') {
    return perform(derivation, derivation === 'RERUN_FROM_SCRATCH' ? '从头重新执行发起失败。' : '基于原配置新建发起失败。', async binding => {
      const derived = await binding.api.rebuildTaskDraft(binding.id, derivation)
      if (accepts(binding)) await router.push({ path: '/exports/new', query: { draft: derived.draftId, step: derivation === 'RERUN_FROM_SCRATCH' ? '5' : '1' } })
    })
  }
  function resumeFromCheckpoint() {
    return perform('CHECKPOINT_RESUME', '从检查点继续发起失败。', async binding => {
      const derived = await binding.api.resumeTaskFromCheckpoint(binding.id)
      if (accepts(binding)) await router.push(`/tasks/${encodeURIComponent(derived.id)}`)
    })
  }
  function saveAsTemplate() {
    const displayName = templateNameInput.value.trim()
    if (!displayName) return Promise.resolve()
    return perform('SAVE_TEMPLATE', '保存模板失败。', async binding => {
      await binding.api.saveTaskTemplate(binding.id, displayName)
      if (!accepts(binding)) return
      templateNameInput.value = ''
      noticeTemplate.value = '模板已保存；模板不复制凭据、节点、预检查或风险确认。'
    })
  }

  return { derivationFailure, derivationBusy, noticeTemplate, templateNameInput, savingTemplate, checkpointResumeAvailable, rebuildFromTask, resumeFromCheckpoint, saveAsTemplate }
}
