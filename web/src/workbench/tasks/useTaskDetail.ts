import { onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import { browserApi, taskDetailErrorMessage, type TaskCommandEvidence, type TaskExecution, type TaskOverview, type TaskSnapshot } from '@/api/browser'
import type { TaskDetailApi, TaskDetailSession } from './taskDetailSession'
import { useTaskLogs } from './useTaskLogs'

// 详情读取是概览、冻结证据和执行事实的唯一所有者；每个会话最多一个刷新链和计时器。
export function useTaskDetail(taskID: Readonly<Ref<string>>, createApi: (signal: AbortSignal) => TaskDetailApi = browserApi) {
  const session = shallowRef<TaskDetailSession | null>(null)
  const overview = ref<TaskOverview | null>(null)
  const execution = ref<TaskExecution | null>(null)
  const snapshot = ref<TaskSnapshot | null>(null)
  const commandEvidence = ref<TaskCommandEvidence | null>(null)
  const loading = ref(true)
  const failure = ref('')
  const executionFailure = ref('')
  const snapshotFailure = ref('')
  const commandFailure = ref('')
  const taskLogs = useTaskLogs(session, execution)
  let release = () => {}
  let cycle: { binding: TaskDetailSession; pending: Map<string, Promise<void>>; refreshing?: Promise<void>; timer?: ReturnType<typeof setTimeout> } | undefined

  watch(taskID, (id) => {
    release()
    overview.value = null
    execution.value = null
    snapshot.value = null
    commandEvidence.value = null
    failure.value = executionFailure.value = snapshotFailure.value = commandFailure.value = ''
    loading.value = Boolean(id)
    if (!id) {
      session.value = null
      cycle = undefined
      failure.value = '未指定任务。'
      return
    }
    const controller = new AbortController()
    let active = true
    const binding: TaskDetailSession = { id, api: createApi(controller.signal), active: () => active }
    const current = { binding, pending: new Map<string, Promise<void>>() } as NonNullable<typeof cycle>
    release = () => {
      active = false
      clearTimeout(current.timer)
      controller.abort()
    }
    cycle = current
    session.value = binding
    void refresh()
  }, { immediate: true, flush: 'sync' })
  onScopeDispose(() => { release() })

  function read<T>(key: string, request: (api: TaskDetailApi, id: string) => Promise<T>, accept: (item: T) => void, errorState: Ref<string>, fallback: string): Promise<void> {
    const current = cycle
    if (!current?.binding.active()) return Promise.resolve()
    const existing = current.pending.get(key)
    if (existing) return existing
    const { binding } = current
    const pending = (async () => {
      try {
        const item = await request(binding.api, binding.id)
        if (!binding.active()) return
        accept(item)
        errorState.value = ''
      } catch (error) {
        if (binding.active()) errorState.value = taskDetailErrorMessage(error, fallback)
      } finally {
        current.pending.delete(key)
      }
    })()
    current.pending.set(key, pending)
    return pending
  }

  function loadExecution() {
    return read('execution', (api, id) => api.getTaskExecution(id), item => { execution.value = item }, executionFailure, '执行状态暂时不可读取。')
  }
  function loadSnapshot() {
    if (snapshot.value) return Promise.resolve()
    return read('snapshot', (api, id) => api.getTaskSnapshot(id), item => { snapshot.value = item }, snapshotFailure, '冻结配置暂时不可读取。')
  }
  function loadCommandEvidence() {
    if (commandEvidence.value) return Promise.resolve()
    return read('command', (api, id) => api.getTaskCommandEvidence(id), item => { commandEvidence.value = item }, commandFailure, '命令证据暂时不可读取。')
  }

  function refresh(): Promise<void> {
    const current = cycle
    if (!current?.binding.active()) return Promise.resolve()
    if (current.refreshing) return current.refreshing
    clearTimeout(current.timer)
    current.timer = undefined
    loading.value = overview.value === null
    failure.value = ''
    const pending = (async () => {
      try {
        if (!overview.value) {
          await read('overview', (api, id) => api.getTaskOverview(id), item => { overview.value = item }, failure, '无法读取任务概览。')
        }
        if (!current.binding.active() || !overview.value) return
        await Promise.all([loadSnapshot(), loadCommandEvidence(), loadExecution(), taskLogs.loadLogs()])
        if (current.binding.active()) taskLogs.ensureLogStream()
      } finally {
        if (current.binding.active()) {
          loading.value = false
          current.refreshing = undefined
          // 失败概览保留手动重试；已载入的任务继续按原两秒周期核对事实。
          if (overview.value) current.timer = setTimeout(() => { void refresh() }, 2000)
        }
      }
    })()
    current.refreshing = pending
    return pending
  }

  return { session, overview, execution, snapshot, commandEvidence, loading, failure, executionFailure, snapshotFailure, commandFailure, refresh, loadExecution, loadSnapshot, loadCommandEvidence, ...taskLogs }
}
