import { onScopeDispose, ref, watch } from 'vue'
import { storageCredentialErrorMessage, type StorageCredentialListItem } from '@/api/browser'
import type { useCredentialList } from './useCredentialList'

export function useCredentialDeletion(list: ReturnType<typeof useCredentialList>) {
  const pendingDelete = ref<StorageCredentialListItem>()
  let version = 0
  function cancelDelete() { version++; pendingDelete.value = undefined }
  onScopeDispose(cancelDelete)
  // 授权失效同步清除确认目标，避免列表清空后弹框仍展示旧对象。
  watch(list.accessDenied, denied => { if (denied) cancelDelete() }, { flush: 'sync' })
  function requestDelete(credential: StorageCredentialListItem) {
    if (!list.active() || list.writeBusy.value || list.accessDenied.value) return
    cancelDelete()
    pendingDelete.value = credential
  }
  async function confirmDelete() {
    const target = pendingDelete.value
    if (!target) return
    const transaction = list.beginWrite(target.id)
    if (!transaction) return
    const requested = version
    const valid = () => transaction.valid() && requested === version
    try {
      await list.api.deleteStorageCredential(target.id, target.revision)
      if (!valid()) return
      list.credentials.value = list.credentials.value.filter(item => item.id !== target.id)
      list.notice.value = `凭据「${target.displayName}」已删除；已冻结任务的历史快照只保留引用标识。`
    } catch (error) {
      if (valid()) list.feedback.value = storageCredentialErrorMessage(error, '凭据删除失败。')
    } finally {
      if (valid()) cancelDelete()
      transaction.finish()
    }
  }
  return { pendingDelete, requestDelete, confirmDelete, cancelDelete }
}
