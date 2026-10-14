import { onScopeDispose, ref, watch, type Ref } from 'vue'
import { taskDetailErrorMessage, type TaskOperation } from '@/api/browser'
import { isTaskAuthorizationError, type TaskDetailSession } from './taskDetailSession'

// 操作记录按详情会话绑定；加载更多防重，旧页和卸载后的响应不能覆盖当前任务。
export function useTaskOperations(session: Readonly<Ref<TaskDetailSession | null>>, onRestricted: () => void = () => {}) {
  const operations = ref<readonly TaskOperation[]>([])
  const nextCursor = ref<string>()
  const failure = ref('')
  const loading = ref(false)
  const loaded = ref(false)
  let pending: { binding: TaskDetailSession; promise: Promise<void> } | undefined
  let disposed = false
  watch(session, () => { operations.value = []; nextCursor.value = undefined; failure.value = ''; loading.value = loaded.value = false; pending = undefined }, { flush: 'sync' })
  onScopeDispose(() => { disposed = true })
  function load(more = false): Promise<void> {
    const binding = session.value
    if (!binding || disposed || !binding.active() || more && !nextCursor.value) return Promise.resolve()
    if (pending?.binding === binding) return pending.promise
    loading.value = true
    const cursor = more ? nextCursor.value : undefined
    const promise = (async () => {
      try {
        if (!binding.api.getTaskOperations) throw new Error('当前服务尚未提供任务操作记录。')
        const page = await binding.api.getTaskOperations(binding.id, cursor)
        if (disposed || session.value !== binding || !binding.active()) return
        const existing = more ? operations.value : []
        const known = new Set(existing.map(item => item.id))
        operations.value = [...existing, ...page.items.filter(item => { if (known.has(item.id)) return false; known.add(item.id); return true })]
        nextCursor.value = page.nextCursor
        failure.value = ''
        loaded.value = true
      } catch (error) {
        if (!disposed && session.value === binding && binding.active()) { if (isTaskAuthorizationError(error)) onRestricted(); else failure.value = taskDetailErrorMessage(error, '操作记录暂时不可读取。') }
      } finally {
        if (session.value === binding && !disposed && binding.active()) loading.value = false
        if (pending?.binding === binding) pending = undefined
      }
    })()
    pending = { binding, promise }
    return promise
  }
  return { operations, nextCursor, failure, loading, loaded, load }
}
