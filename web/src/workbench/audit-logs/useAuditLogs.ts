import { computed, onScopeDispose, reactive, ref, shallowRef, watch } from 'vue'
import { auditObjectTypes, type AuditFilter, type AuditItem, type AuditLogApi, type AuditPage } from '@/api/auditLogs'

export function defaultAuditFilter(now = Date.now()): AuditFilter { return { from: new Date(now - 3600000).toISOString(), to: new Date(now).toISOString(), actorId: '', objectType: '', objectId: '', action: '', result: '', keyword: '', limit: 200 } }
export function auditFilterErrors(f: AuditFilter): Record<string, string> {
 const errors: Record<string, string> = {}
 if (!Number.isFinite(Date.parse(f.from)) || !Number.isFinite(Date.parse(f.to)) || Date.parse(f.from) >= Date.parse(f.to) || Date.parse(f.to) - Date.parse(f.from) > 86400000) errors.time = '请选择有效时间范围，最多 24 小时。'
 for (const key of ['actorId', 'objectId'] as const) if (f[key] && !/^[\p{L}\p{N}_.:@-]{1,200}$/u.test(f[key])) errors[key] = '请填写有效的精确 ID。'
 if (f.action && !/^[A-Z0-9_]{1,100}$/.test(f.action)) errors.action = '请填写有效的事件代码。'
 if (f.objectType && !auditObjectTypes.includes(f.objectType as typeof auditObjectTypes[number])) errors.objectType = '请选择已接入的对象域。'
 if (new TextEncoder().encode(f.keyword).length > 256 || [...f.keyword].some(c => { const n = c.codePointAt(0)!; return n < 32 || n >= 127 && n <= 159 })) errors.keyword = '关键字最多 256 字节，不能含控制字符。'
 return errors
}

/** 唯一表单与快照所有者；查询、单事件和下载分别绑定代次，跨预设卸载即失效。 */
export function useAuditLogs(api: AuditLogApi, effects: { save(file: Blob): void; denied(): void }) {
 const filters = reactive(defaultAuditFilter()), applied = shallowRef<AuditFilter>(), page = shallowRef<AuditPage>(), history = ref<string[]>([]), loading = ref(false), failure = ref(''), notice = ref(''), errors = ref<Record<string, string>>({}), relativeTime = ref(true)
 const selected = shallowRef<AuditItem>(), detail = shallowRef<AuditItem>(), detailOpen = ref(false), detailLoading = ref(false), detailFailure = ref('')
 const downloadOpen = ref(false), downloading = ref(false), actionFailure = ref('')
 let generation = 0, detailGeneration = 0, actionGeneration = 0, disposed = false, applying = false
 let queryRequest: AbortController | undefined, detailRequest: AbortController | undefined, actionRequest: AbortController | undefined
 const dirty = computed(() => !applied.value || JSON.stringify(filters) !== JSON.stringify(applied.value))
 const canDownload = computed(() => !!page.value && !page.value.truncated && !dirty.value && !loading.value && !downloading.value)
 function cancelDownload() { actionGeneration++; actionRequest?.abort(); actionRequest = undefined; downloadOpen.value = false; downloading.value = false; actionFailure.value = ''; notice.value = '' }
 function closeDetail() { detailGeneration++; detailRequest?.abort(); detailRequest = undefined; detailOpen.value = false; detailLoading.value = false; detail.value = undefined; selected.value = undefined; detailFailure.value = ''; notice.value = '' }
 function invalidate() { generation++; queryRequest?.abort(); queryRequest = undefined; loading.value = false; errors.value = {}; closeDetail(); cancelDownload() }
 function clearResults() { page.value = undefined; history.value = [] }
 watch(filters, () => { if (!applying) { invalidate(); failure.value = '' } }, { flush: 'sync' })
 function reset() { invalidate(); applying = true; try { Object.assign(filters, defaultAuditFilter()) } finally { applying = false }; applied.value = undefined; relativeTime.value = true; clearResults(); failure.value = '' }
 function setRange(from: string, to: string) { relativeTime.value = false; Object.assign(filters, { from, to }) }
 function fail(error: unknown, target = failure) {
  const e = error && typeof error === 'object' ? error as { status?: number; code?: string } : {}
  // 无法确认当前授权或响应结构时撤下缓存；筛选保留，允许重新查询，旧结果不能充当资格。
  clearResults(); detail.value = undefined; selected.value = undefined; downloadOpen.value = false
  const denied = [401, 403, 404].includes(e.status ?? 0)
  target.value = denied ? '审计访问资格已失效，旧结果已清除。' : e.status === 409 ? '审计快照已失效，请重新查询。' : e.status === 422 ? '下载达到扫描或文件限制，请缩小条件后重新查询。' : '审计读取未完成，条件已保留，请重新查询。'
  if (denied) effects.denied()
 }
 async function run(mode: 'query' | 'refresh' | 'next' | 'previous' = 'query') {
  if (disposed || loading.value || mode !== 'query' && dirty.value) return
  if (mode === 'next' && !page.value?.nextCursor || mode === 'previous' && !history.value.length) return
  const old = page.value, previous = [...history.value]
  const filter: AuditFilter = { ...filters, ...(relativeTime.value && ['query', 'refresh'].includes(mode) ? { from: new Date(Date.now() - 3600000).toISOString(), to: new Date().toISOString() } : {}) }
  errors.value = auditFilterErrors(filter); if (Object.keys(errors.value).length) return
  invalidate(); const current = generation, controller = new AbortController(); queryRequest = controller; loading.value = true; failure.value = ''
  const cursor = mode === 'next' ? old!.nextCursor : mode === 'previous' ? previous.at(-1) : undefined
  try {
   const result = await api.queryAuditLogs(filter, cursor, controller.signal)
   if (disposed || current !== generation) return
   applying = true; try { Object.assign(filters, { from: filter.from, to: filter.to }) } finally { applying = false }
   page.value = result; applied.value = filter; history.value = mode === 'next' ? [...previous, old!.pageCursor] : mode === 'previous' ? previous.slice(0, -1) : []
   if (mode === 'refresh') notice.value = '已读取新的审计快照。'
  } catch (error) { if (!disposed && current === generation) fail(error) }
  finally { if (!disposed && current === generation) { loading.value = false; queryRequest = undefined } }
 }
 async function openDetail(item: AuditItem) {
  if (disposed || loading.value || dirty.value) return
  closeDetail(); cancelDownload(); selected.value = item; detailOpen.value = true; detailLoading.value = true
  const current = detailGeneration, parent = generation, controller = new AbortController(); detailRequest = controller
  try {
   const result = await api.getAuditDetail(item.token, controller.signal)
   if (disposed || current !== detailGeneration || parent !== generation) return
   if (result.id !== item.id || result.token !== item.token || result.objectType !== item.objectType || result.objectId !== item.objectId || result.actorId !== item.actorId) throw { code: 'RESPONSE_INVALID' }
   detail.value = result
  } catch (error) { if (!disposed && current === detailGeneration && parent === generation) fail(error, detailFailure) }
  finally { if (!disposed && current === detailGeneration && parent === generation) { detailLoading.value = false; detailRequest = undefined } }
 }
 function requestDownload() { if (!canDownload.value) return; closeDetail(); cancelDownload(); downloadOpen.value = true }
 async function download() {
  if (disposed || downloading.value || !downloadOpen.value || !canDownload.value || !page.value) return
  const token = page.value.snapshotToken; downloading.value = true; actionFailure.value = ''
  const current = ++actionGeneration, parent = generation, controller = new AbortController(); actionRequest = controller
  try { const file = await api.downloadAuditLogs(token, controller.signal); if (disposed || current !== actionGeneration || parent !== generation) return; effects.save(file); downloadOpen.value = false; notice.value = '已生成脱敏审计文件并发起下载。' }
  catch (error) { if (!disposed && current === actionGeneration && parent === generation) fail(error, actionFailure) }
  finally { if (!disposed && current === actionGeneration && parent === generation) { downloading.value = false; actionRequest = undefined } }
 }
 onScopeDispose(() => { disposed = true; invalidate(); clearResults(); applied.value = undefined; failure.value = '' })
 return { filters, applied, page, history, loading, failure, notice, errors, dirty, canDownload, selected, detail, detailOpen, detailLoading, detailFailure, downloadOpen, downloading, actionFailure, reset, setRange, run, openDetail, closeDetail, requestDownload, cancelDownload, download }
}
