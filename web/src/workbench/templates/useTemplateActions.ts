import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { taskDetailErrorMessage, type ApiError, type ConfigTemplateItem } from '@/api/browser'
import { templateNameError } from './templateRules'
import type { useTemplateCatalog } from './useTemplateCatalog'

// 改名和删除分别绑定具体修订，取消、撤权与重开都撤销上一代反馈。
export function useTemplateActions(catalog: ReturnType<typeof useTemplateCatalog>) {
  const pendingDelete = shallowRef<ConfigTemplateItem>()
  const renameTarget = shallowRef<ConfigTemplateItem>()
  const renameID = computed(() => renameTarget.value?.id ?? '')
  const renameValue = ref(''), renameError = ref(''), renameFailure = ref(''), deleteFailure = ref('')
  const renameConflict = ref(false), deleteConflict = ref(false)
  let renameVersion = 0, deleteVersion = 0
  function cancelRename() { renameVersion++; renameTarget.value = undefined; renameValue.value = renameError.value = renameFailure.value = ''; renameConflict.value = false }
  function cancelDelete() { deleteVersion++; pendingDelete.value = undefined; deleteFailure.value = ''; deleteConflict.value = false }
  onScopeDispose(() => { cancelRename(); cancelDelete() })
  watch([catalog.accessDenied, catalog.templates], () => {
    if (renameTarget.value && !catalog.allowed(renameTarget.value, 'RENAME')) cancelRename()
    if (pendingDelete.value && !catalog.allowed(pendingDelete.value, 'DELETE')) cancelDelete()
  }, { flush: 'sync' })
  function startRename(template: ConfigTemplateItem) {
    if (catalog.actionBusy.value || !catalog.allowed(template, 'RENAME')) return
    cancelDelete(); cancelRename()
    renameTarget.value = template
    renameValue.value = template.displayName
    catalog.notice.value = ''
  }
  function requestDelete(template: ConfigTemplateItem) {
    if (catalog.actionBusy.value || !catalog.allowed(template, 'DELETE')) return
    cancelRename(); cancelDelete()
    pendingDelete.value = template
    catalog.notice.value = ''
  }
  function updateRenameValue(value: string) { if (!catalog.actionBusy.value && !renameConflict.value) { renameValue.value = value; renameError.value = '' } }
  function validateRename() { renameError.value = templateNameError(renameValue.value) }
  async function confirmRename(template = renameTarget.value) {
    const target = renameTarget.value
    if (!target || template?.id !== target.id || !catalog.allowed(target, 'RENAME') || catalog.actionBusy.value || renameConflict.value) return
    const displayName = renameValue.value.trim()
    renameError.value = templateNameError(displayName)
    if (renameError.value) return
    if (displayName === target.displayName) { cancelRename(); return }
    const finish = catalog.beginAction(target.id)
    if (!finish) return
    const version = renameVersion
    const valid = () => catalog.active() && version === renameVersion
    renameFailure.value = ''
    try {
      const revision = await catalog.api.renameConfigTemplate(target.id, target.revision, displayName)
      if (!valid()) return
      cancelRename()
      catalog.templates.value = catalog.templates.value.map(item => item.id === target.id ? { ...item, displayName, revision } : item)
      catalog.notice.value = '模板「' + displayName + '」已改名。'
    } catch (error) {
      if (!valid()) return
      catalog.denied(error)
      if (!valid()) return
      renameConflict.value = [409, 412].includes((error as Partial<ApiError>).status ?? 0)
      renameFailure.value = renameConflict.value ? '模板已被修改，请刷新列表后重新改名。' : taskDetailErrorMessage(error, '模板改名未完成，请重试。')
    } finally { finish() }
  }
  async function confirmDelete() {
    const target = pendingDelete.value
    if (!target || !catalog.allowed(target, 'DELETE') || deleteConflict.value) return
    const finish = catalog.beginAction(target.id)
    if (!finish) return
    const version = deleteVersion
    const valid = () => catalog.active() && version === deleteVersion
    deleteFailure.value = ''
    try {
      await catalog.api.deleteConfigTemplate(target.id, target.revision)
      if (!valid()) return
      cancelDelete()
      catalog.templates.value = catalog.templates.value.filter(item => item.id !== target.id)
      catalog.notice.value = '模板「' + target.displayName + '」已删除。'
    } catch (error) {
      if (!valid()) return
      catalog.denied(error)
      if (!valid()) return
      deleteConflict.value = [409, 412].includes((error as Partial<ApiError>).status ?? 0)
      deleteFailure.value = deleteConflict.value ? '模板已变更，请刷新列表后重新确认删除。' : taskDetailErrorMessage(error, '模板删除未完成，请重试。')
    } finally { finish() }
  }
  return { pendingDelete, renameTarget, renameID, renameValue, renameError, renameFailure, deleteFailure, renameConflict, deleteConflict, startRename, confirmRename, requestDelete, confirmDelete, cancelRename, cancelDelete, updateRenameValue, validateRename }
}
