import { computed, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import type { LocationQuery, LocationQueryRaw, RouteLocationRaw } from 'vue-router'
import { browserApi, taskDetailErrorMessage, taskListPageSizes, type BrowserApi, type ExportDraftListItem, type TaskListPageSize } from '@/api/browser'
import { isTaskAuthorizationError } from './taskDetailSession'

type DraftApi = Pick<BrowserApi, 'listExportDrafts' | 'deleteExportDraft'>
const cursorMemory = new Map<string, { stack: (string | undefined)[]; index: number }>()

// 草稿列表与删除的唯一状态所有者；条件会话隔离读取、确认、写反馈和 finally，不复制完整草稿配置。
export function useTaskDraftList(query: Readonly<Ref<LocationQuery>>, replaceQuery: (query: LocationQueryRaw) => unknown, navigate: (location: RouteLocationRaw) => unknown, createApi: (signal: AbortSignal) => DraftApi = browserApi) {
  const keyword = computed(() => typeof query.value.draftKeyword === 'string' ? query.value.draftKeyword : '')
  const limit = computed<TaskListPageSize>(() => taskListPageSizes.includes(Number(query.value.draftLimit) as TaskListPageSize) ? Number(query.value.draftLimit) as TaskListPageSize : 10)
  const key = computed(() => JSON.stringify([keyword.value, limit.value]))
  const items = shallowRef<readonly ExportDraftListItem[]>([])
  const totalPages = ref(0), pageIndex = ref(0), loading = ref(false), refreshing = ref(false), restricted = ref(false)
  const failure = ref(''), notice = ref(''), busyId = ref('')
  const nextCursor = ref<string>(), confirmation = shallowRef<ExportDraftListItem>()
  let controller = new AbortController(), api = createApi(controller.signal), stack: (string | undefined)[] = [undefined]
  let disposed = false, version = 0, pendingRead: Promise<void> | undefined
  const remember = () => { cursorMemory.delete(key.value); cursorMemory.set(key.value, { stack: [...stack], index: pageIndex.value }); if (cursorMemory.size > 20) cursorMemory.delete(cursorMemory.keys().next().value!) }
  watch(key, () => {
    version++; controller.abort(); controller = new AbortController(); api = createApi(controller.signal); pendingRead = undefined
    const saved = cursorMemory.get(key.value); stack = saved ? [...saved.stack] : [undefined]; pageIndex.value = saved?.index ?? 0
    items.value = []; nextCursor.value = undefined; totalPages.value = 0; failure.value = notice.value = busyId.value = ''; confirmation.value = undefined; restricted.value = false
    void load()
  }, { immediate: true, flush: 'sync' })
  onScopeDispose(() => { disposed = true; version++; controller.abort(); notice.value = ''; confirmation.value = undefined })
  function load(preserve = false): Promise<void> {
    if (disposed || busyId.value) return Promise.resolve()
    if (pendingRead) return pendingRead
    const current = ++version, currentApi = api
    loading.value = !preserve; refreshing.value = preserve
    const promise = (async () => {
      try {
        const page = await currentApi.listExportDrafts(limit.value, keyword.value, stack[pageIndex.value])
        if (disposed || current !== version) return
        items.value = page.items; totalPages.value = page.totalPages; nextCursor.value = page.nextCursor; failure.value = ''; restricted.value = false; remember()
      } catch (error) {
        if (disposed || current !== version) return
        failure.value = taskDetailErrorMessage(error, '草稿列表暂时不可读取。')
        if (isTaskAuthorizationError(error)) { restricted.value = true; items.value = []; totalPages.value = 0; nextCursor.value = undefined; confirmation.value = undefined; notice.value = '' }
      } finally { if (!disposed && current === version) { loading.value = refreshing.value = false; pendingRead = undefined } }
    })()
    pendingRead = promise; return promise
  }
  function updateFilters(patch: { keyword?: string; limit?: TaskListPageSize }) {
    const next: LocationQueryRaw = { ...query.value, tab: 'drafts', draftKeyword: patch.keyword ?? keyword.value, draftLimit: patch.limit ?? limit.value }
    if (!next.draftKeyword) delete next.draftKeyword
    if (Number(next.draftLimit) === 10) delete next.draftLimit
    void replaceQuery(next)
  }
  function next() { if (loading.value || refreshing.value || busyId.value || !nextCursor.value) return; stack = [...stack.slice(0, pageIndex.value + 1), nextCursor.value]; pageIndex.value++; void load() }
  function previous() { if (loading.value || refreshing.value || busyId.value || !pageIndex.value) return; pageIndex.value--; void load() }
  function open(item: ExportDraftListItem, submit = false) { if (busyId.value || !item.availableActions.includes(submit ? 'SUBMIT' : 'CONTINUE_EDIT')) return; void navigate({ path: '/exports/new', query: { draft: item.id, step: submit || item.sourceDerivation === 'RERUN_FROM_SCRATCH' ? '5' : '1' } }) }
  function requestDelete(item: ExportDraftListItem) { if (!busyId.value && item.availableActions.includes('DELETE')) { failure.value = notice.value = ''; confirmation.value = item } }
  function dismiss() { if (!busyId.value) confirmation.value = undefined }
  async function confirmDelete() {
    const item = confirmation.value; if (!item || busyId.value) return
    const current = ++version, currentApi = api; pendingRead = undefined; busyId.value = item.id; loading.value = refreshing.value = false
    try {
      await currentApi.deleteExportDraft(item.id, item.revision)
      if (disposed || current !== version) return
      confirmation.value = undefined; notice.value = '草稿已删除。'
    } catch (error) { if (!disposed && current === version) failure.value = taskDetailErrorMessage(error, '删除草稿未完成。') }
    finally { if (!disposed && current === version) { busyId.value = ''; await load(true); if (!disposed && version === current + 1 && !items.value.length && pageIndex.value > 0) { pageIndex.value--; await load() } } }
  }
  return { keyword, limit, items, totalPages, pageIndex, nextCursor, loading, refreshing, restricted, failure, notice, busyId, confirmation, load, updateFilters, next, previous, open, requestDelete, dismiss, confirmDelete }
}
