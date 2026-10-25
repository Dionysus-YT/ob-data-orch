import { onScopeDispose, ref, shallowRef } from 'vue'
import type { SecurityPolicy, SecurityPolicyApi } from '@/api/securityPolicy'

/** 本域唯一持有只读事实与请求会话；刷新失败或撤权清除旧状态，不以缓存宣称安全。 */
export function useSecurityPolicy(api: SecurityPolicyApi) {
  const policy = shallowRef<SecurityPolicy>(), loading = ref(false), failure = ref('')
  let generation = 0, disposed = false, controller: AbortController | undefined
  function reset() { generation++; controller?.abort(); controller = undefined; policy.value = undefined; loading.value = false; failure.value = '' }
  async function read() {
    if (disposed || loading.value) return
    reset(); const current = generation, request = new AbortController(); controller = request; loading.value = true
    try { const value = await api.getSecurityPolicy(request.signal); if (!disposed && current === generation) policy.value = value }
    catch (error) {
      if (!disposed && current === generation) {
        const status = error && typeof error === 'object' && 'status' in error ? Number(error.status) : 0
        failure.value = [401, 403, 404].includes(status) ? '需要有效登录及系统管理员权限才能读取安全状态。' : '无法读取安全状态，请重试；读取成功前状态保持未知。'
      }
    } finally { if (!disposed && current === generation) { loading.value = false; controller = undefined } }
  }
  onScopeDispose(() => { disposed = true; reset() })
  return { policy, loading, failure, read, reset }
}
