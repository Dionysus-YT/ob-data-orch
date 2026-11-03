import { computed, onScopeDispose, ref, watch } from 'vue'
import { browserApi, taskDetailErrorMessage, type ApiError, type ConfigTemplateItem, type TemplateAction } from '@/api/browser'
import { templateAllows } from './templateRules'

export type TemplateApi = Pick<ReturnType<typeof browserApi>, 'listConfigTemplates' | 'renameConfigTemplate' | 'deleteConfigTemplate' | 'listDataSources' | 'listExportNodeCandidates' | 'listExecutionNodes' | 'createTaskDraftFromTemplate'>

// 目录是模板摘要的唯一所有者，浏览条件只派生当前完整授权集合，不复制业务事实。
export function useTemplateCatalog(createApi: (signal: AbortSignal) => TemplateApi = browserApi) {
  const controller = new AbortController()
  const api = createApi(controller.signal)
  const templates = ref<ConfigTemplateItem[]>([])
  const loading = ref(true), hasLoaded = ref(false), accessDenied = ref(false)
  const loadFailure = ref(''), notice = ref(''), actionBusy = ref('')
  const keyword = ref(''), typeFilter = ref(''), page = ref(1)
  const filtered = computed(() => {
    const query = keyword.value.trim().toLocaleLowerCase()
    return templates.value.filter(item => (!query || item.displayName.toLocaleLowerCase().includes(query)) && (!typeFilter.value || item.taskType === typeFilter.value))
  })
  const total = computed(() => filtered.value.length)
  const rows = computed(() => filtered.value.slice((page.value - 1) * 10, page.value * 10))
  const hasFilters = computed(() => Boolean(keyword.value || typeFilter.value))
  watch([keyword, typeFilter], () => { page.value = 1 }, { flush: 'sync' })
  watch(total, count => { page.value = Math.min(page.value, Math.max(1, Math.ceil(count / 10))) }, { flush: 'sync' })
  let disposed = false, readVersion = 0, actionVersion = 0
  const active = () => !disposed
  onScopeDispose(() => { disposed = true; readVersion++; actionVersion++; controller.abort() })
  function denied(error: unknown) {
    if (![401, 403].includes((error as Partial<ApiError>)?.status ?? 0)) return
    readVersion++
    loading.value = false
    accessDenied.value = true
    templates.value = []
    hasLoaded.value = false
    notice.value = ''
    actionVersion++
    actionBusy.value = ''
  }
  async function refresh() {
    if (!active() || actionBusy.value) return
    const version = ++readVersion
    const valid = () => active() && version === readVersion
    loading.value = true
    loadFailure.value = ''
    try {
      const result = await api.listConfigTemplates()
      if (!valid()) return
      templates.value = result
      accessDenied.value = false
      hasLoaded.value = true
    } catch (error) {
      if (!valid()) return
      denied(error)
      loadFailure.value = taskDetailErrorMessage(error, '无法加载模板，请稍后重试。')
    } finally { if (valid()) loading.value = false }
  }
  function allowed(item: ConfigTemplateItem, action: TemplateAction) {
    const current = templates.value.find(value => value.id === item.id && value.revision === item.revision && value.taskType === item.taskType)
    return active() && !accessDenied.value && Boolean(current && templateAllows(current, action))
  }
  function beginAction(id: string): false | (() => void) {
    if (!active() || actionBusy.value || accessDenied.value) return false
    readVersion++
    loading.value = false
    actionBusy.value = id
    notice.value = loadFailure.value = ''
    const version = ++actionVersion
    // 旧事务的 finally 不能释放撤权后重新建立的新事务。
    return () => { if (active() && version === actionVersion) actionBusy.value = '' }
  }
  function clearFilters() { keyword.value = typeFilter.value = ''; page.value = 1 }
  return { api, signal: controller.signal, active, templates, loading, hasLoaded, loadFailure, notice, actionBusy, accessDenied, keyword, typeFilter, page, rows, total, hasFilters, clearFilters, refresh, beginAction, allowed, denied }
}
