import { onScopeDispose, ref, watch, type Ref } from 'vue'
import { taskDetailErrorMessage, type TaskExecution, type TaskLog } from '@/api/browser'
import type { TaskDetailSession } from './taskDetailSession'
import { createTaskLogStreamLifecycle } from './taskLogStreamLifecycle'

// 日志、可靠游标与单条实时流只由此能力持有；HTTP 补读与流事件均绑定当前任务会话。
export function useTaskLogs(session: Readonly<Ref<TaskDetailSession | null>>, execution: Readonly<Ref<TaskExecution | null>>) {
  const logs = ref<readonly TaskLog[]>([])
  const logCursor = ref<string>()
  const logFailure = ref('')
  const logStreamStatus = ref<'idle' | 'connected' | 'interrupted'>('idle')
  const lifecycle = createTaskLogStreamLifecycle()
  let pending: { binding: TaskDetailSession; promise: Promise<void> } | undefined
  let disposed = false

  watch(session, () => {
    lifecycle.close()
    pending = undefined
    logs.value = []
    logCursor.value = undefined
    logFailure.value = ''
    logStreamStatus.value = 'idle'
  }, { flush: 'sync' })
  watch(() => execution.value?.state, (state) => {
    if (state !== 'STARTING' && state !== 'RUNNING') {
      lifecycle.close()
      logStreamStatus.value = 'idle'
    }
  }, { flush: 'sync' })
  onScopeDispose(() => { disposed = true; lifecycle.close() })
  const accepts = (binding: TaskDetailSession) => !disposed && binding.active() && session.value === binding

  function loadLogs(): Promise<void> {
    const binding = session.value
    if (!binding || !accepts(binding)) return Promise.resolve()
    if (pending?.binding === binding) return pending.promise
    const cursor = logCursor.value
    const promise = (async () => {
      try {
        const page = await binding.api.getTaskLogs(binding.id, undefined, cursor)
        if (!accepts(binding)) return
        appendLogs(page.items)
        // 实时流可能已走到更晚游标；游标是不透明值，旧 HTTP 结果不能将它倒退。
        if (logCursor.value === cursor) logCursor.value = page.lastReliableCursor ?? logCursor.value
        logFailure.value = ''
        ensureLogStream()
      } catch (error) {
        if (accepts(binding)) logFailure.value = taskDetailErrorMessage(error, '日志暂时不可读取。')
      } finally {
        if (pending?.binding === binding) pending = undefined
      }
    })()
    pending = { binding, promise }
    return promise
  }

  function ensureLogStream() {
    const binding = session.value
    if (!binding || !accepts(binding) || lifecycle.active() || (execution.value?.state !== 'STARTING' && execution.value?.state !== 'RUNNING')) return
    try {
      const opened = lifecycle.open((onDisconnected, isCurrent) => binding.api.streamTaskLogs(binding.id, logCursor.value, (record, cursor) => {
        if (!accepts(binding) || !isCurrent()) return
        appendLogs([record])
        logCursor.value = cursor ?? logCursor.value
        logStreamStatus.value = 'connected'
      }, onDisconnected), () => {
        if (accepts(binding)) logStreamStatus.value = 'interrupted'
      })
      if (accepts(binding)) logStreamStatus.value = opened ? 'connected' : 'interrupted'
    } catch {
      if (accepts(binding)) logStreamStatus.value = 'interrupted'
    }
  }

  function appendLogs(records: readonly TaskLog[]) {
    const known = new Set(logs.value.map(keyOf))
    const appended = records.filter(record => {
      const key = keyOf(record)
      if (known.has(key)) return false
      known.add(key)
      return true
    })
    if (appended.length) logs.value = [...logs.value, ...appended]
  }
  function keyOf(record: TaskLog) { return `${record.sourceSeq}|${record.receivedAt}|${record.kind}|${record.message}` }

  return { logs, logCursor, logFailure, logStreamStatus, loadLogs, ensureLogStream }
}
