import { onScopeDispose, ref, shallowRef } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { browserApi, taskDetailErrorMessage, type BrowserApi, type TaskListAction, type TaskListItem } from '@/api/browser'
import { derivedDraftRoute, derivedTaskRoute } from './taskDerivationRoutes'
import { withTemplateWriteDeadline } from '@/workbench/templates/templateWrites'
import { templateNameError as validateTemplateName, templateWriteUncertain } from '@/workbench/templates/templateRules'

export type TaskListActionApi = Pick<BrowserApi, 'cancelTask' | 'rebuildTaskDraft' | 'resumeTaskFromCheckpoint' | 'saveTaskTemplate' | 'getTaskCommandEvidence'>

/** 需要确认框的操作；复制与查看日志直接执行。 */
export type TaskConfirmKind = Extract<TaskListAction, 'CANCEL' | 'RERUN_FROM_SCRATCH' | 'CHECKPOINT_RESUME' | 'SAVE_TEMPLATE' | 'REBUILD_FROM_CONFIG'>

export interface TaskListActionDependencies {
  readonly navigate: (location: RouteLocationRaw) => unknown
  readonly beginMutation: () => void | ((reload: boolean) => void)
  readonly endMutation: (reload: boolean) => void
  readonly copyText?: (value: string) => Promise<void>
  readonly createApi?: (signal: AbortSignal) => TaskListActionApi
}

type TaskActionOutcome = { readonly kind: 'NOTICE'; readonly text: string } | { readonly kind: 'NAVIGATE'; readonly location: RouteLocationRaw }

// 列表行写操作的唯一所有者：一次只允许一个写事务；卸载后迟到响应不导航、不发布反馈。
// 资格只用于展示，服务端在每个写接口内复验。
export function useTaskListActions(deps: TaskListActionDependencies) {
  let controller = new AbortController()
  let api = (deps.createApi ?? browserApi)(controller.signal)
  const copyText = deps.copyText ?? (value => navigator.clipboard.writeText(value))
  const busyTaskId = ref('')
  const failure = ref('')
  const notice = ref('')
  const confirmation = shallowRef<{ readonly kind: TaskConfirmKind; readonly task: TaskListItem } | null>(null)
  const templateName = ref('')
  const templateNameError = ref('')
  const templateResultUnknown = ref(false)
  let templateRequest: { taskId: string; name: string; key: string } | undefined
  let disposed = false
  let sequence = 0
  onScopeDispose(() => { disposed = true; sequence++; controller.abort() })

  function clearFeedback() { failure.value = ''; notice.value = '' }
  // 切换对象或列表条件立即撤销旧事务的展示与导航资格，已接受的服务端写入不被宣称取消。
  function invalidate() {
    sequence++
    controller.abort()
    controller = new AbortController()
    api = (deps.createApi ?? browserApi)(controller.signal)
    busyTaskId.value = ''
    confirmation.value = null
    templateName.value = ''
    templateNameError.value = ''
    templateResultUnknown.value = false
    templateRequest = undefined
    clearFeedback()
  }

  function request(task: TaskListItem, action: TaskListAction) {
    if (busyTaskId.value || !task.availableActions.includes(action)) return
    clearFeedback()
    if (action === 'VIEW_LOGS') { void deps.navigate({ name: 'task-detail', params: { id: task.id }, query: { tab: 'logs' } }); return }
    if (action === 'COPY_COMMAND') { void copyCommand(task); return }
    templateName.value = ''
    templateNameError.value = ''
    templateResultUnknown.value = false
    templateRequest = undefined
    confirmation.value = { kind: action, task }
  }

  function dismiss() { if (!busyTaskId.value) { confirmation.value = null; templateName.value = ''; templateResultUnknown.value = false; templateRequest = undefined; clearFeedback() } }
  function updateTemplateName(value: string) { if (templateResultUnknown.value) return; templateName.value = value; templateNameError.value = ''; templateRequest = undefined }

  async function run(task: TaskListItem, fallback: string, reload: boolean, operation: () => Promise<TaskActionOutcome>) {
    const kind = confirmation.value?.kind
    clearFeedback()
    const current = ++sequence
    const accepts = () => !disposed && current === sequence
    busyTaskId.value = task.id
    const finishMutation = deps.beginMutation()
    let succeeded = false
    try {
      const result = await operation()
      if (!accepts()) return
      succeeded = true
      confirmation.value = null
      if (kind === 'SAVE_TEMPLATE') { templateName.value = ''; templateResultUnknown.value = false; templateRequest = undefined }
      if (result.kind === 'NOTICE') notice.value = result.text
      else await deps.navigate(result.location)
    } catch (error) {
      if (accepts()) {
        templateResultUnknown.value = kind === 'SAVE_TEMPLATE' && (templateResultUnknown.value || templateWriteUncertain(error))
        failure.value = templateResultUnknown.value ? '保存结果待确认。可保持原请求重试，或刷新模板中心核对。' : taskDetailErrorMessage(error, fallback)
      }
    } finally {
      if (accepts()) busyTaskId.value = ''
      // 写操作可能已被服务端接受；无论成功与否都重读列表，以服务端投影为准。
      // 详情提供会话绑定的结束句柄，旧 finally 不能释放新任务的写事务暂停。
      if (typeof finishMutation === 'function') finishMutation(accepts() && (reload || !succeeded))
      else deps.endMutation(accepts() && (reload || !succeeded))
    }
  }

  function confirm() {
    const target = confirmation.value
    if (!target || busyTaskId.value) return
    const { kind, task } = target
    if (kind === 'CANCEL') {
      return run(task, '取消请求未完成。', true, async () => { await api.cancelTask(task.id); return { kind: 'NOTICE', text: '已提交取消请求。' } })
    }
    if (kind === 'REBUILD_FROM_CONFIG' || kind === 'RERUN_FROM_SCRATCH') {
      return run(task, kind === 'RERUN_FROM_SCRATCH' ? '从头重新执行发起失败。' : '基于原配置新建发起失败。', false, async () => {
        const derived = await api.rebuildTaskDraft(task.id, kind)
        if ((derived.taskType ?? 'OBDUMPER_EXPORT') !== task.type) throw new Error('派生草稿类型与来源任务不一致。')
        return { kind: 'NAVIGATE', location: derivedDraftRoute(derived, task.type) }
      })
    }
    if (kind === 'CHECKPOINT_RESUME') {
      return run(task, '从检查点继续发起失败。', false, async () => ({ kind: 'NAVIGATE', location: derivedTaskRoute((await api.resumeTaskFromCheckpoint(task.id)).id) }))
    }
    const name = templateName.value.trim()
    templateNameError.value = validateTemplateName(name)
    if (templateNameError.value) return
    if (!templateRequest || templateRequest.taskId !== task.id || templateRequest.name !== name) templateRequest = { taskId: task.id, name, key: crypto.randomUUID() }
    const key = templateRequest.key
    return run(task, '保存模板失败。', false, async () => { await withTemplateWriteDeadline(() => api.saveTaskTemplate(task.id, name, key), controller.signal); return { kind: 'NOTICE', text: `模板「${name}」已保存。` } })
  }

  async function copyCommand(task: TaskListItem) {
    const current = ++sequence
    busyTaskId.value = task.id
    try {
      const evidence = await api.getTaskCommandEvidence(task.id)
      if (disposed || current !== sequence) return
      await copyText(evidence.command)
      if (!disposed && current === sequence) notice.value = '已复制脱敏命令。'
    } catch (error) {
      if (!disposed && current === sequence) failure.value = taskDetailErrorMessage(error, '复制脱敏命令失败。')
    } finally {
      if (!disposed && current === sequence) busyTaskId.value = ''
    }
  }

  return { busyTaskId, failure, notice, confirmation, templateName, templateNameError, templateResultUnknown, updateTemplateName, request, confirm, dismiss, clearFeedback, invalidate }
}
