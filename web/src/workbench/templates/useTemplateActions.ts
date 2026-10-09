import { onScopeDispose, ref, watch } from 'vue'
import { taskDetailErrorMessage, type ExportConfigTemplateItem } from '@/api/browser'
import type { useTemplateCatalog } from './useTemplateCatalog'

export function useTemplateActions(catalog: ReturnType<typeof useTemplateCatalog>) {
  const pendingDelete = ref<ExportConfigTemplateItem>()
  const renameID = ref('')
  const renameValue = ref('')
  let renameVersion = 0
  let deleteVersion = 0
  function cancelRename() { renameVersion++; renameID.value = renameValue.value = '' }
  function cancelDelete() { deleteVersion++; pendingDelete.value = undefined }
  onScopeDispose(() => { cancelRename(); cancelDelete() })
  // 目录授权失效时同步撤销两类编辑会话，成功重读也不恢复旧输入或目标。
  watch(catalog.accessDenied, denied => { if (denied) { cancelRename(); cancelDelete() } }, { flush: 'sync' })
  function startRename(template: ExportConfigTemplateItem) {
    if (!catalog.active() || catalog.actionBusy.value || catalog.accessDenied.value) return
    cancelRename()
    renameID.value = template.id
    renameValue.value = template.displayName
    catalog.notice.value = ''
  }
  function requestDelete(template: ExportConfigTemplateItem) {
    if (!catalog.active() || catalog.actionBusy.value || catalog.accessDenied.value) return
    cancelDelete()
    pendingDelete.value = template
  }
  async function confirmRename(template: ExportConfigTemplateItem) {
    if (renameID.value !== template.id || !catalog.active() || catalog.actionBusy.value) return
    const displayName = renameValue.value.trim()
    if (!displayName || displayName === template.displayName) { cancelRename(); return }
    if (!catalog.beginAction(template.id)) return
    const version = renameVersion
    const valid = () => catalog.active() && version === renameVersion
    try {
      const revision = await catalog.api.renameExportConfigTemplate(template.id, template.revision, displayName)
      if (!valid()) return
      catalog.templates.value = catalog.templates.value.map(item => item.id === template.id ? { ...item, displayName, revision } : item)
      catalog.notice.value = `模板「${displayName}」已改名。`
    } catch (error) {
      if (valid()) { catalog.notice.value = ''; catalog.loadFailure.value = taskDetailErrorMessage(error, '模板改名失败。') }
    } finally {
      if (valid()) cancelRename()
      catalog.endAction()
    }
  }
  async function confirmDelete() {
    const template = pendingDelete.value
    if (!template || !catalog.beginAction(template.id)) return
    const version = deleteVersion
    const valid = () => catalog.active() && version === deleteVersion
    try {
      await catalog.api.deleteExportConfigTemplate(template.id, template.revision)
      if (!valid()) return
      cancelDelete()
      catalog.templates.value = catalog.templates.value.filter(item => item.id !== template.id)
      catalog.notice.value = `模板「${template.displayName}」已删除。`
    } catch (error) {
      if (valid()) { catalog.notice.value = ''; catalog.loadFailure.value = taskDetailErrorMessage(error, '模板删除失败。') }
    } finally {
      catalog.endAction()
    }
  }
  return { pendingDelete, renameID, renameValue, startRename, confirmRename, requestDelete, confirmDelete, cancelRename, cancelDelete }
}
