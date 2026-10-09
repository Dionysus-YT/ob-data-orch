import { onScopeDispose, ref } from 'vue'
import { browserApi, taskDetailErrorMessage, type ApiError, type ExportConfigTemplateItem } from '@/api/browser'

export type TemplateApi = Pick<ReturnType<typeof browserApi>, 'listExportConfigTemplates' | 'renameExportConfigTemplate' | 'deleteExportConfigTemplate' | 'listDataSources' | 'listExportNodeCandidates' | 'createDraftFromTemplate'>

export function useTemplateCatalog(createApi: (signal: AbortSignal) => TemplateApi = browserApi) {
  const controller = new AbortController()
  const api = createApi(controller.signal)
  const templates = ref<ExportConfigTemplateItem[]>([])
  const loading = ref(true)
  const loadFailure = ref('')
  const notice = ref('')
  const actionBusy = ref('')
  let disposed = false
  let readVersion = 0
  const active = () => !disposed
  onScopeDispose(() => { disposed = true; readVersion++; controller.abort() })
  async function refresh() {
    if (!active() || actionBusy.value) return
    const version = ++readVersion
    const valid = () => active() && version === readVersion
    loading.value = true
    loadFailure.value = ''
    try {
      const result = await api.listExportConfigTemplates()
      if (valid()) templates.value = result
    } catch (error) {
      if (!valid()) return
      if ([401, 403].includes((error as Partial<ApiError>).status ?? 0)) templates.value = []
      loadFailure.value = taskDetailErrorMessage(error, '无法加载模板，请稍后重试。')
    } finally {
      if (valid()) loading.value = false
    }
  }
  function beginAction(id: string) {
    if (!active() || actionBusy.value) return false
    readVersion++
    loading.value = false
    actionBusy.value = id
    return true
  }
  function endAction() { if (active()) actionBusy.value = '' }
  return { api, active, templates, loading, loadFailure, notice, actionBusy, refresh, beginAction, endAction }
}
