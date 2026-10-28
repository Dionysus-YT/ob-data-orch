import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { browserApi, type ApiError } from '@/api/browser'
import type { OverviewApi, OverviewRange, OverviewSnapshot } from '@/api/overview'

export interface OverviewOptions { readonly createApi?: (signal: AbortSignal) => OverviewApi; readonly now?: () => number; readonly autoRefreshMs?: number }

/** 首页快照与读取会话的唯一所有者；范围切换立即失效旧请求，卸载和授权失效清除引用。 */
export function useOverview(options: OverviewOptions = {}) {
  const createApi = options.createApi ?? browserApi
  const now = options.now ?? Date.now
  const range = ref<OverviewRange>('24h')
  const snapshot = shallowRef<OverviewSnapshot | null>(null)
  const loading = ref(false), refreshing = ref(false), refreshCompleted = ref(false), restricted = ref(false)
  const failure = ref(''), currentTime = ref(now())
  let disposed = false, generation = 0
  let controller: AbortController | undefined
  let refreshTimer: ReturnType<typeof setTimeout> | undefined
  let clockTimer: ReturnType<typeof setTimeout> | undefined
  const stale = computed(() => snapshot.value !== null && currentTime.value - Date.parse(snapshot.value.snapshotAt) >= snapshot.value.staleAfterSeconds * 1000)
  const partial = computed(() => snapshot.value !== null && [snapshot.value.metrics, snapshot.value.trend, snapshot.value.nodes, snapshot.value.myTasks, snapshot.value.attention].some(section => section.status === 'UNAVAILABLE'))
  const visible = () => typeof document === 'undefined' || document.visibilityState !== 'hidden'

  function stopRefresh() { clearTimeout(refreshTimer); refreshTimer = undefined }
  function schedule() {
    stopRefresh()
    const delay = options.autoRefreshMs ?? (snapshot.value?.refreshIntervalSeconds ?? 30) * 1000
    if (disposed || restricted.value || !visible() || delay <= 0) return
    refreshTimer = setTimeout(() => { refreshTimer = undefined; void load('auto') }, delay)
  }
  function tick() {
    clearTimeout(clockTimer)
    currentTime.value = now()
    if (!disposed && visible()) clockTimer = setTimeout(tick, 1000)
  }
  function invalidate() {
    generation++
    controller?.abort()
    controller = undefined
    stopRefresh()
    loading.value = refreshing.value = false
  }
  // 下钻读取与首页共用浏览器身份；任一入口确认失权时同步清除全页快照和在途等待。
  function invalidateAuthorization(status: 401 | 403 = 403) {
    if (disposed) return
    invalidate()
    snapshot.value = null
    restricted.value = true
    refreshCompleted.value = false
    failure.value = status === 401 ? '登录状态已失效，请重新登录后查看首页。' : '当前身份没有可查看的首页范围，请核对账号与对象授权。'
  }
  async function load(mode: 'initial' | 'manual' | 'auto' = 'manual') {
    // 方法入口防重，按钮 disabled 仅是交互反馈。
    if (disposed || loading.value || refreshing.value) return
    stopRefresh()
    const version = ++generation, requestedRange = range.value
    const active = () => !disposed && version === generation
    controller = new AbortController()
    loading.value = snapshot.value === null
    refreshing.value = !loading.value
    refreshCompleted.value = false
    failure.value = ''
    try {
      const result = await createApi(controller.signal).getOverview(requestedRange)
      if (!active()) return
      snapshot.value = result
      restricted.value = false
      currentTime.value = now()
      refreshCompleted.value = mode === 'manual'
    } catch (error) {
      if (!active()) return
      const problem = error as Partial<ApiError>
      if (problem.status === 401 || problem.status === 403) {
        invalidateAuthorization(problem.status)
      } else {
        failure.value = problem.code === 'RESPONSE_INVALID' ? '首页快照无法校验，请核对网页与控制面版本后重试。' : '首页快照读取失败，请检查控制面状态后重试。'
      }
    } finally {
      if (active()) { loading.value = refreshing.value = false; controller = undefined; schedule() }
    }
  }
  watch(range, () => {
    invalidate()
    // 范围变更保留整份旧快照，页面显式标记旧范围；新结果到达后整体替换，不拼接分区。
    restricted.value = false
    failure.value = ''
    void load('initial')
  }, { immediate: true, flush: 'sync' })
  function onVisibility() { stopRefresh(); tick(); if (visible() && !restricted.value) void load('auto') }
  if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility)
  tick()
  onScopeDispose(() => {
    disposed = true
    invalidate()
    clearTimeout(clockTimer)
    snapshot.value = null
    failure.value = ''
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility)
  })
  return { range, snapshot, loading, refreshing, refreshCompleted, restricted, failure, stale, partial, currentTime, refresh: () => load('manual'), invalidateAuthorization }
}
