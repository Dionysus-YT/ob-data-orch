import { onScopeDispose, ref, shallowRef, watch } from 'vue'
import type { AccessControlApi, PermissionCandidate, PermissionScopeType } from '@/api/accessControl'
import { permissionDenied, permissionError } from './permissionRules'

/** 每个范围选择器独立绑定编辑代、范围类型与搜索，候选不跨用户或请求合并。 */
export function usePermissionCandidates(api: AccessControlApi, binding: () => number, type: () => PermissionScopeType, onDenied: () => void) {
  const items = shallowRef<readonly PermissionCandidate[]>([]); const loading = ref(false); const failure = ref(''); const nextCursor = ref<string>(); const search = ref(''); const loaded = ref(false)
  let disposed = false; let generation = 0; let request: AbortController | undefined; let timer: ReturnType<typeof setTimeout> | undefined
  function reset() { generation++; request?.abort(); request = undefined; if (timer) clearTimeout(timer); timer = undefined; items.value = []; nextCursor.value = undefined; loading.value = false; failure.value = ''; loaded.value = false; search.value = '' }
  async function read(more = false) {
    if (disposed || loading.value || more && (!nextCursor.value || items.value.length >= 500)) return
    const expected = ++generation; const session = binding(); const scope = type(); const controller = new AbortController(); request = controller; loading.value = true; failure.value = ''
    const valid = () => !disposed && expected === generation && session === binding() && scope === type()
    try {
      const result = await api.getPermissionCandidates(scope, search.value, more ? nextCursor.value : undefined, controller.signal)
      if (!valid()) return
      if (more && result.items.some(row => items.value.some(previous => previous.id === row.id))) { failure.value = '候选分页发生变化，请重新搜索。'; return }
      items.value = more ? [...items.value, ...result.items] : result.items; nextCursor.value = result.nextCursor; loaded.value = true
    } catch (error) { if (!valid()) return; if (permissionDenied(error)) { reset(); onDenied() } else failure.value = permissionError(error, '无法读取授权候选，请重试。') }
    finally { if (valid()) { loading.value = false; request = undefined } }
  }
  function begin() { if (!loaded.value) return read() }
  function setSearch(value: string) { if (disposed) return; generation++; request?.abort(); request = undefined; if (timer) clearTimeout(timer); search.value = value; items.value = []; nextCursor.value = undefined; loaded.value = false; failure.value = ''; loading.value = true; timer = setTimeout(() => { timer = undefined; loading.value = false; void read() }, 200) }
  watch(() => [binding(), type()], reset, { flush: 'sync' })
  onScopeDispose(() => { disposed = true; reset() })
  return { items, loading, failure, nextCursor, search, loaded, reset, begin, read, setSearch }
}
