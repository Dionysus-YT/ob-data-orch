import { computed, onScopeDispose, reactive, ref, shallowRef } from 'vue'
import type { AccessControlApi, PermissionCapability, PermissionFilters, PermissionPage, PermissionProfile, PermissionRole } from '@/api/accessControl'
import { permissionDenied, permissionError } from './permissionRules'

/** 用户目录是列表事实、筛选、分页和管理资格的唯一所有者；失败时只保留只读旧事实。 */
export function usePermissionList(api: AccessControlApi, options: { onDenied: () => void; onIdentityChanged: () => void }) {
  const items = shallowRef<readonly PermissionProfile[]>([])
  const metadata = shallowRef<PermissionPage>()
  const form = reactive({ search: '', role: '' as PermissionRole | '', status: '' as 'ACTIVE' | 'DISABLED' | 'UNKNOWN' | '', capability: '' as PermissionCapability | '' })
  const query = shallowRef<PermissionFilters>({ limit: 10 })
  const loading = ref(false); const failure = ref(''); const granted = ref(false)
  const previousCursors = ref<string[]>([])
  let disposed = false; let generation = 0; let request: AbortController | undefined
  const currentSubjectId = computed(() => metadata.value?.currentSubjectId ?? '')
  const pageNumber = computed(() => previousCursors.value.length + 1)
  const canPrevious = computed(() => previousCursors.value.length > 0 && !loading.value)
  const canNext = computed(() => Boolean(metadata.value?.hasMore) && !loading.value)
  const hasFilters = computed(() => Boolean(form.search || form.role || form.status || form.capability))

  function invalidate(message = '当前身份没有权限配置访问资格，或接口尚未接入。') { generation++; request?.abort(); request = undefined; items.value = []; metadata.value = undefined; granted.value = false; loading.value = false; failure.value = message; previousCursors.value = []; options.onDenied() }
  async function read(replace = false) {
    if (disposed || loading.value && !replace) return false
    const expected = ++generation; request?.abort(); const controller = new AbortController(); request = controller
    loading.value = true; granted.value = false; failure.value = ''
    try {
      const result = await api.getPermissionSubjects(query.value, controller.signal)
      if (disposed || expected !== generation) return false
      if (metadata.value && metadata.value.currentSubjectId !== result.currentSubjectId) options.onIdentityChanged()
      items.value = result.items; metadata.value = result; granted.value = true
      return true
    } catch (error) {
      if (disposed || expected !== generation) return false
      if (permissionDenied(error)) invalidate(permissionError(error, '权限目录无法读取。'))
      else failure.value = permissionError(error, '无法读取用户权限目录，最后可信资料仅供只读查看。')
      return false
    } finally { if (!disposed && expected === generation) { loading.value = false; request = undefined } }
  }
  function apply() { query.value = { limit: 10, ...(form.search.trim() ? { search: form.search.trim() } : {}), ...(form.role ? { role: form.role } : {}), ...(form.status ? { status: form.status } : {}), ...(form.capability ? { capability: form.capability } : {}) }; previousCursors.value = []; return read(true) }
  function clear() { Object.assign(form, { search: '', role: '', status: '', capability: '' }); return apply() }
  async function move(cursor: string | undefined, history: string[]) { const originalQuery = query.value, originalHistory = previousCursors.value, originalMetadata = metadata.value; const requested = { ...query.value, cursor }; query.value = requested; previousCursors.value = history; if (!await read() && query.value === requested && metadata.value === originalMetadata) { query.value = originalQuery; previousCursors.value = originalHistory } }
  function next() { if (!canNext.value || !metadata.value?.nextCursor) return; return move(metadata.value.nextCursor, [...previousCursors.value, query.value.cursor ?? '']) }
  function previous() { if (!canPrevious.value) return; return move(previousCursors.value.at(-1) || undefined, previousCursors.value.slice(0, -1)) }
  onScopeDispose(() => { disposed = true; generation++; request?.abort(); request = undefined; items.value = []; metadata.value = undefined; granted.value = false })
  return { items, metadata, form, query, loading, failure, granted, currentSubjectId, pageNumber, canPrevious, canNext, hasFilters, read, apply, clear, next, previous, invalidate }
}
