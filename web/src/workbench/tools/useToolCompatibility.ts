import { onScopeDispose, ref, shallowRef } from 'vue'
import type { ToolCompatibility, ToolCompatibilityApi } from '@/api/toolCompatibility'

/** 本域唯一持有工具事实与读取会话；旧会话和撤权响应不能恢复已撤下的目录。 */
export function useToolCompatibility(api: ToolCompatibilityApi) {
  const baseline = shallowRef<ToolCompatibility>(), loading = ref(false), failure = ref('')
  let generation = 0, disposed = false, controller: AbortController | undefined
  function reset() { generation++; controller?.abort(); controller = undefined; baseline.value = undefined; loading.value = false; failure.value = '' }
  async function read() {
    if (disposed || loading.value) return
    reset(); const current = generation, request = new AbortController(); controller = request; loading.value = true
    try { const value = await api.getToolCompatibility(request.signal); if (!disposed && current === generation) baseline.value = value }
    catch (error) {
      if (!disposed && current === generation) {
        const status = error && typeof error === 'object' && 'status' in error ? Number(error.status) : 0
        failure.value = [401, 403, 404].includes(status) ? '需要有效登录及系统管理员权限才能读取工具状态。' : '无法读取工具状态，请重试；读取成功前状态保持未知。'
      }
    } finally { if (!disposed && current === generation) { loading.value = false; controller = undefined } }
  }
  onScopeDispose(() => { disposed = true; reset() })
  return { baseline, loading, failure, read, reset }
}
