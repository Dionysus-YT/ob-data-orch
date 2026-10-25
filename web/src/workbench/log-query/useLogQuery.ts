import { computed, onScopeDispose, reactive, ref, shallowRef, watch } from 'vue'
import type { LogContext, LogFilter, LogItem, LogPage, LogQueryApi } from '@/api/logQuery'

export function defaultLogFilter(now = Date.now()): LogFilter { return { from: new Date(now - 3600000).toISOString(), to: new Date(now).toISOString(), taskId: '', source: '', keyword: '', limit: 200 } }
function logFilterIssue(f: LogFilter): { field: 'time' | 'task' | 'keyword'; message: string } {
  if (!f.from || !f.to || !Number.isFinite(Date.parse(f.from)) || !Number.isFinite(Date.parse(f.to)) || Date.parse(f.from) >= Date.parse(f.to)) return { field: 'time', message: '请选择有效的起止时间。' }
  if (!f.taskId && Date.parse(f.to) - Date.parse(f.from) > 86400000) return { field: 'time', message: '跨任务查询最多 24 小时；查询更长时间请填写精确任务 ID。' }
  if (f.taskId && !/^[\p{L}\p{N}_.-]{1,200}$/u.test(f.taskId)) return { field: 'task', message: '请填写有效的精确任务 ID。' }
  if (new TextEncoder().encode(f.keyword).length > 256 || [...f.keyword].some(c => c.codePointAt(0)! < 32 || c.codePointAt(0) === 127)) return { field: 'keyword', message: '关键字过长或包含控制字符，请缩短后重试。' }
  return { field: 'time', message: '' }
}
export function logFilterError(f: LogFilter): string { return logFilterIssue(f).message }

/** 查询、快照和抽屉分别绑定代次；输入、路由和卸载均使旧异步结果失去回写资格。 */
export function useLogQuery(api: LogQueryApi, effects: { copy(text: string): Promise<void>; save(file: Blob): void }) {
  const filters = reactive({ ...defaultLogFilter() }), applied = shallowRef<LogFilter>(), page = shallowRef<LogPage>(), loading = ref(false), failure = ref(''), fieldError = ref(''), notice = ref(''), history = ref<string[]>([])
  const context = shallowRef<LogContext>(), selected = shallowRef<LogItem>(), contextOpen = ref(false), contextLoading = ref(false), contextFailure = ref(''), copying = ref(false)
  const downloadOpen = ref(false), downloading = ref(false), downloadFullTask = ref(false), actionFailure = ref('')
  const fieldOwner = ref('time'), relativeTime = ref(true)
  const timeError = computed(() => fieldOwner.value === 'time' ? fieldError.value : ''), taskError = computed(() => fieldOwner.value === 'task' ? fieldError.value : ''), keywordError = computed(() => fieldOwner.value === 'keyword' ? fieldError.value : '')
  let generation = 0, detailGeneration = 0, actionGeneration = 0, disposed = false, applying = false
  let request: AbortController | undefined, detailRequest: AbortController | undefined, actionRequest: AbortController | undefined
  const dirty = computed(() => !applied.value || JSON.stringify(filters) !== JSON.stringify(applied.value)), canDownload = computed(() => !!page.value && !dirty.value && !loading.value && !page.value.truncated && !downloading.value)
  function clearActions() { actionGeneration++; actionRequest?.abort(); actionRequest = undefined; downloadOpen.value = false; downloading.value = false; copying.value = false; actionFailure.value = ''; notice.value = '' }
  function closeContext() { detailGeneration++; detailRequest?.abort(); detailRequest = undefined; contextOpen.value = false; selected.value = undefined; context.value = undefined; contextLoading.value = false; contextFailure.value = ''; clearActions() }
  function invalidate() { generation++; request?.abort(); request = undefined; loading.value = false; fieldError.value = ''; closeContext() }
  function clearResults() { page.value = undefined; applied.value = undefined; history.value = [] }
  watch(filters, () => { if (!applying) { invalidate(); failure.value = '' } }, { flush: 'sync' })
  function reset(taskId = '', from = '', to = '') { invalidate(); clearResults(); Object.assign(filters, defaultLogFilter(), { taskId }, from && to ? { from, to } : {}); relativeTime.value = !(from && to); failure.value = '' }
  function setRange(from: string, to: string) { relativeTime.value = false; Object.assign(filters, { from, to }) }
  function handleError(error: unknown, fallback: string, target = failure) {
    const e = error && typeof error === 'object' ? error as { status?: number; code?: string; requestId?: string } : {}
    if ([401, 403, 404].includes(e.status ?? 0)) { invalidate(); clearResults(); failure.value = '当前身份无法访问这些任务日志，请检查登录及任务权限。'; return }
    if (e.code === 'LOG_QUERY_SNAPSHOT_INVALID') { closeContext(); clearResults(); failure.value = '日志快照已失效，请保留条件并重新查询。'; return }
    target.value = e.code === 'LOG_DOWNLOAD_LIMIT' ? '下载超过 100000 条或 100 MiB，请缩小条件。' : `${fallback}${e.requestId ? ` 请求标识：${e.requestId}` : ''}`
  }
  async function run(mode: 'new' | 'refresh' | 'next' | 'previous' = 'new') {
    if (disposed || loading.value) return
    const filter = { ...(mode === 'new' ? filters : applied.value ?? filters) }
    if ((mode === 'new' || mode === 'refresh') && relativeTime.value) { const latest = defaultLogFilter(); Object.assign(filter, { from: latest.from, to: latest.to }) }
    const issue = logFilterIssue(filter); fieldError.value = issue.message; fieldOwner.value = issue.field; if (fieldError.value) return
    if (mode === 'next' && !page.value?.nextCursor || mode === 'previous' && !history.value.length) return
    const previous = [...history.value], cursor = mode === 'next' ? page.value!.nextCursor : mode === 'previous' ? previous.at(-1) : undefined, oldPage = page.value
    invalidate(); const current = generation, controller = new AbortController(); request = controller; loading.value = true; failure.value = ''
    try {
      const result = await api.queryTaskLogs(filter, cursor, controller.signal)
      if (disposed || current !== generation) return
      // 相对时间只在成功提交新快照时同步到表单，失败不会留下虚假的未应用条件。
      applying = true
      try { Object.assign(filters, { from: filter.from, to: filter.to }) } finally { applying = false }
      page.value = result; applied.value = filter
      history.value = mode === 'next' ? [...previous, oldPage!.pageCursor] : mode === 'previous' ? previous.slice(0, -1) : []
      if (mode === 'refresh') notice.value = '已读取新的日志快照。'
    } catch (error) { if (!disposed && current === generation) handleError(error, '日志查询未完成，条件与上次结果已保留，请重试。') }
    finally { if (!disposed && current === generation) { loading.value = false; request = undefined } }
  }
  async function openContext(item: LogItem) {
    if (disposed || loading.value || dirty.value) return
    closeContext(); contextOpen.value = true; selected.value = item; contextLoading.value = true
    const current = detailGeneration, parent = generation, controller = new AbortController(); detailRequest = controller
    try {
      const result = await api.getLogContext(item.token, controller.signal)
      if (disposed || current !== detailGeneration || parent !== generation) return
      if (result.selectedSequence !== item.sequence || result.items.some(row => row.streamId !== item.streamId || row.epoch !== item.epoch || row.source !== item.source || row.taskId !== item.taskId)) throw { code: 'RESPONSE_INVALID' }
      context.value = result
    }
    catch (error) { if (!disposed && current === detailGeneration && parent === generation) handleError(error, '同来源上下文暂时无法读取，请重试。', contextFailure) }
    finally { if (!disposed && current === detailGeneration && parent === generation) { contextLoading.value = false; detailRequest = undefined } }
  }
  async function copyContext(single = false) {
    if (disposed || copying.value || !selected.value || !context.value || dirty.value) return
    clearActions(); const current = actionGeneration, detail = detailGeneration, controller = new AbortController(); actionRequest = controller; copying.value = true
    const active = () => !disposed && current === actionGeneration && detail === detailGeneration
    try {
      const text = single ? selected.value.message : await api.copyLogContext(selected.value.token, controller.signal)
      if (!active()) return
      await effects.copy(text)
      if (active()) notice.value = single ? '已复制当前脱敏消息。' : '已复制同来源脱敏上下文。'
    } catch (error) { if (active()) handleError(error, '复制未完成，请重试。', actionFailure) }
    finally { if (active()) { copying.value = false; actionRequest = undefined } }
  }
  function requestDownload(fullTask = false) { if (!canDownload.value || fullTask && !applied.value?.taskId) return; closeContext(); downloadFullTask.value = fullTask; downloadOpen.value = true }
  async function download() {
    if (disposed || downloading.value || !downloadOpen.value || !page.value || dirty.value) return
    const token = page.value.snapshotToken, full = downloadFullTask.value; clearActions(); downloadOpen.value = true; downloading.value = true
    const current = actionGeneration, parent = generation, controller = new AbortController(); actionRequest = controller
    try {
      const file = await api.downloadTaskLogs(token, full, controller.signal)
      if (disposed || current !== actionGeneration || parent !== generation) return
      effects.save(file); downloadOpen.value = false; notice.value = '已生成脱敏日志文件并发起下载。'
    } catch (error) { if (!disposed && current === actionGeneration && parent === generation) handleError(error, '日志下载未完成，请保留范围后重试。', actionFailure) }
    finally { if (!disposed && current === actionGeneration && parent === generation) { downloading.value = false; actionRequest = undefined } }
  }
  onScopeDispose(() => { disposed = true; invalidate(); clearResults() })
  return { filters, applied, page, loading, failure, fieldError, timeError, taskError, keywordError, relativeTime, notice, history, dirty, canDownload, context, selected, contextOpen, contextLoading, contextFailure, copying, downloadOpen, downloading, downloadFullTask, actionFailure, run, reset, setRange, openContext, closeContext, copyContext, requestDownload, download, cancelDownload: clearActions }
}
