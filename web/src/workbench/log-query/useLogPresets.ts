import { onScopeDispose, ref } from 'vue'
import type { AuditLogApi } from '@/api/auditLogs'

/** 只持有当前审计入口资格；不缓存角色或把入口资格当作事件读取授权。 */
export function useLogPresets(api: Pick<AuditLogApi, 'getAuditAccess'>) {
 const allowed = ref(false), failure = ref(''), loading = ref(false)
 let generation = 0, disposed = false, request: AbortController | undefined
 function revoke() { generation++; request?.abort(); request = undefined; allowed.value = false; loading.value = false; failure.value = '审计访问资格已失效，旧结果已清除。' }
 async function load() {
  const current = ++generation, controller = new AbortController(); request?.abort(); request = controller; loading.value = true; allowed.value = false; failure.value = ''
  try { const result = await api.getAuditAccess(controller.signal); if (!disposed && current === generation) allowed.value = result }
  catch { if (!disposed && current === generation) failure.value = '审计入口资格暂时无法确认，请重试。' }
  finally { if (!disposed && current === generation) { loading.value = false; request = undefined } }
 }
 onScopeDispose(() => { disposed = true; generation++; request?.abort(); allowed.value = false; failure.value = '' })
 return { allowed, failure, loading, load, revoke }
}
