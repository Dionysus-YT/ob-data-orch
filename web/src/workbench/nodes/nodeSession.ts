import { onScopeDispose, watch, type Ref } from 'vue'
import { browserApi } from '@/api/browser'

export type NodeApi = Pick<ReturnType<typeof browserApi>, 'listExecutionNodes' | 'getExecutionNode' | 'createExecutionNode' | 'updateExecutionNode' | 'enableExecutionNode' | 'requestExecutionNodeEnvironmentCheck' | 'deleteOrArchiveExecutionNode' | 'issueExecutionNodeEnrollment'>
export type NodeApiFactory = (signal: AbortSignal) => NodeApi

// 节点页会话绑定路由身份（表单同时绑定模式）；回到同一 ID 也不能复活旧操作。
// Abort 只取消浏览器等待，不能撤销已到达控制面的写操作；提交资格仍由服务端复验。
export function useNodeSession(key: Readonly<Ref<string>>, createApi: NodeApiFactory = browserApi) {
  let disposed = false
  let version = 0
  let controller = new AbortController()
  let api = createApi(controller.signal)
  const resets = new Set<() => void>()
  watch(key, () => {
    version++
    controller.abort()
    controller = new AbortController()
    api = createApi(controller.signal)
    for (const reset of resets) reset()
  }, { flush: 'sync' })
  onScopeDispose(() => {
    disposed = true
    version++
    controller.abort()
    for (const reset of resets) reset()
    resets.clear()
  })
  return {
    get api() { return api },
    active: () => !disposed,
    capture() { const captured = version; return () => !disposed && captured === version },
    onReset(reset: () => void) { resets.add(reset) },
  }
}

export type NodeSession = ReturnType<typeof useNodeSession>
