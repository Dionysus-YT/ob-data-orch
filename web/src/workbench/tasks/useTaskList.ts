import { computed, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import { browserApi, taskDetailErrorMessage, type ApiError, type BrowserApi, type TaskFilterOptions, type TaskListItem, type TaskListPageSize } from '@/api/browser'
import { isActiveTaskState, taskDateTimeText } from './taskStatePresentation'
import { emptyTaskListRouteState, parseTaskListRoute, taskListApiFilter, taskListConditionKey, taskListDateRangeError, taskListRouteQuery, type TaskListRouteState } from './taskListQuery'

export type TaskListApi = Pick<BrowserApi, 'listTasks' | 'listTaskFilterOptions'>
export type TaskListApiFactory = (signal: AbortSignal) => TaskListApi

/** 自动刷新间隔为产品固定值（task-center P1 第 5 节），不暴露给用户。 */
export const taskListAutoRefreshMs = 10_000

// 游标栈按条件签名缓存于模块内：路由内返回列表可恢复页码；整页刷新后回到首页，不伪造页码。
const cursorMemory = new Map<string, { readonly stack: readonly (string | undefined)[]; readonly index: number }>()
const cursorMemoryLimit = 20

function rememberCursor(key: string, stack: readonly (string | undefined)[], index: number) {
  cursorMemory.delete(key)
  cursorMemory.set(key, { stack, index })
  while (cursorMemory.size > cursorMemoryLimit) cursorMemory.delete(cursorMemory.keys().next().value!)
}

/** 仅供测试重置模块级游标缓存。 */
export function resetTaskListCursorMemory() { cursorMemory.clear() }

export interface TaskListOptions {
  readonly enabled?: Readonly<Ref<boolean>>
  readonly createApi?: TaskListApiFactory
  readonly autoRefreshMs?: number
}

// 任务列表的唯一状态所有者：筛选来自路由 query，读取、分页、自动刷新与写事务互斥都在这里。
// 路由条件变化立即使旧读取失效；迟到、重复或卸载后的响应不能覆盖当前结果。
export function useTaskList(routeQuery: Readonly<Ref<LocationQuery>>, replaceQuery: (query: LocationQueryRaw) => unknown, options: TaskListOptions = {}) {
  const createApi = options.createApi ?? browserApi
  const autoRefreshMs = options.autoRefreshMs ?? taskListAutoRefreshMs
  const filters = computed<TaskListRouteState>(() => parseTaskListRoute(routeQuery.value))
  const conditionKey = computed(() => taskListConditionKey(filters.value))
  const tasks = shallowRef<readonly TaskListItem[]>([])
  const totalPages = ref(0)
  const pageIndex = ref(0)
  const nextCursor = ref<string>()
  const loading = ref(false)
  const refreshing = ref(false)
  const refreshCompleted = ref(false)
  const restricted = ref(false)
  const failure = ref('')
  const dateRangeError = ref('')
  const lastLoadedAt = ref('')
  const filterOptions = shallowRef<TaskFilterOptions | null>(null)
  let cursorStack: (string | undefined)[] = [undefined]
  let disposed = false
  let readVersion = 0
  let readController: AbortController | undefined
  let optionsController: AbortController | undefined
  let mutations = 0
  let refreshTimer: ReturnType<typeof setTimeout> | undefined

  const hasActiveTasks = computed(() => tasks.value.some(task => isActiveTaskState(task.state)))
  const visible = () => typeof document === 'undefined' || document.visibilityState !== 'hidden'

  function stopTimer() { clearTimeout(refreshTimer); refreshTimer = undefined }

  // 只有当前页存在非终态任务、页面可见且没有授权失效时才安排下一次读取。
  function scheduleRefresh() {
    stopTimer()
    if (disposed || options.enabled?.value === false || restricted.value || !hasActiveTasks.value || !visible() || autoRefreshMs <= 0) return
    const version = readVersion
    refreshTimer = setTimeout(() => {
      refreshTimer = undefined
      if (disposed || version !== readVersion) return
      if (mutations || loading.value || refreshing.value) { scheduleRefresh(); return }
      void load('auto')
    }, autoRefreshMs)
  }

  async function load(mode: 'initial' | 'manual' | 'auto' | 'page') {
    if (disposed || options.enabled?.value === false) return
    stopTimer()
    dateRangeError.value = taskListDateRangeError(filters.value)
    if (dateRangeError.value) {
      readVersion++
      readController?.abort()
      tasks.value = []
      totalPages.value = 0
      nextCursor.value = undefined
      loading.value = refreshing.value = refreshCompleted.value = false
      return
    }
    const version = ++readVersion
    readController?.abort()
    const controller = new AbortController()
    readController = controller
    const valid = () => !disposed && version === readVersion
    const preserve = mode === 'manual' || mode === 'auto'
    loading.value = !preserve
    refreshing.value = mode === 'manual'
    refreshCompleted.value = false
    if (mode !== 'auto') failure.value = ''
    const key = conditionKey.value
    try {
      const page = await createApi(controller.signal).listTasks(cursorStack[pageIndex.value], filters.value.limit, taskListApiFilter(filters.value), controller.signal)
      if (!valid()) return
      tasks.value = page.items
      totalPages.value = page.totalPages
      nextCursor.value = page.nextCursor || undefined
      restricted.value = false
      failure.value = ''
      lastLoadedAt.value = taskDateTimeText(new Date().toISOString())
      refreshCompleted.value = mode === 'manual'
      rememberCursor(key, cursorStack, pageIndex.value)
    } catch (error) {
      if (!valid()) return
      const status = (error as Partial<ApiError>).status
      restricted.value = status === 401 || status === 403
      // 授权失效或首次读取失败时不保留旧事实；刷新失败保留可信旧行。
      if (restricted.value || !preserve) {
        tasks.value = []
        totalPages.value = 0
        nextCursor.value = undefined
      }
      failure.value = taskDetailErrorMessage(error, '任务列表暂时无法读取。')
    } finally {
      if (valid()) {
        loading.value = refreshing.value = false
        scheduleRefresh()
      }
    }
  }

  async function loadFilterOptions() {
    optionsController?.abort()
    const controller = new AbortController()
    optionsController = controller
    try {
      const options = await createApi(controller.signal).listTaskFilterOptions(controller.signal)
      if (!disposed && optionsController === controller) filterOptions.value = options
    } catch {
      // 候选读取失败不阻断列表；展开筛选只显示已有条件，不推断候选。
      if (!disposed && optionsController === controller) filterOptions.value = null
    }
  }

  // 条件变化：清空旧事实与游标，恢复同条件的已访问页（路由内返回），否则回到首页。
  watch(conditionKey, key => {
    const remembered = cursorMemory.get(key)
    cursorStack = remembered ? [...remembered.stack] : [undefined]
    pageIndex.value = remembered ? remembered.index : 0
    tasks.value = []
    totalPages.value = 0
    nextCursor.value = undefined
    refreshCompleted.value = false
    void load('initial')
  }, { immediate: true, flush: 'sync' })

  if (options.enabled) watch(options.enabled, enabled => {
    readVersion++; readController?.abort(); optionsController?.abort(); stopTimer(); loading.value = refreshing.value = false
    if (enabled) void load('initial')
  }, { flush: 'sync' })

  function onVisibilityChange() {
    if (!visible()) { stopTimer(); return }
    if (hasActiveTasks.value && !loading.value && !refreshing.value && !mutations) void load('auto')
  }
  if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibilityChange)

  onScopeDispose(() => {
    disposed = true
    readVersion++
    stopTimer()
    readController?.abort()
    optionsController?.abort()
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibilityChange)
  })

  function goToNextPage() {
    const cursor = nextCursor.value
    if (loading.value || refreshing.value || !cursor) return
    cursorStack = [...cursorStack.slice(0, pageIndex.value + 1), cursor]
    pageIndex.value += 1
    void load('page')
  }

  function goToPreviousPage() {
    if (loading.value || refreshing.value || pageIndex.value === 0) return
    pageIndex.value -= 1
    void load('page')
  }

  function updateFilters(patch: Partial<TaskListRouteState>) {
    void replaceQuery(taskListRouteQuery({ ...filters.value, ...patch }))
  }

  function changePageSize(limit: TaskListPageSize) { updateFilters({ limit }) }

  function clearFilters() { void replaceQuery(taskListRouteQuery({ ...emptyTaskListRouteState, limit: filters.value.limit })) }

  function refresh() {
    if (loading.value || refreshing.value || mutations) return
    void load('manual')
  }

  // 写事务期间使在途读取失效并暂停自动刷新，结束后由调用方决定是否重新读取。
  function beginMutation() { mutations++; readVersion++; readController?.abort(); loading.value = refreshing.value = false; stopTimer() }
  function endMutation(reload: boolean) {
    mutations = Math.max(0, mutations - 1)
    if (disposed) return
    // 写后复读保留可信旧行，不把它表达成用户手动刷新。
    if (reload) void load('auto')
    else scheduleRefresh()
  }

  return {
    filters, tasks, totalPages, pageIndex, nextCursor, loading, refreshing, refreshCompleted, restricted, failure,
    dateRangeError, lastLoadedAt, filterOptions, hasActiveTasks,
    refresh, goToNextPage, goToPreviousPage, updateFilters, changePageSize, clearFilters, loadFilterOptions, beginMutation, endMutation,
  }
}
