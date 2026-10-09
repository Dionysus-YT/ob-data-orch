import { onScopeDispose, ref, watch } from 'vue'
import { storageCredentialErrorMessage, type StorageCredentialListItem, type StorageCredentialProvider, type StorageCredentialWrite } from '@/api/browser'
import { validateStorageCredentialWrite, type StorageCredentialFieldErrors } from './storageCredentialList'
import type { useCredentialList } from './useCredentialList'

export function useCredentialEditor(list: ReturnType<typeof useCredentialList>) {
  const editorOpen = ref(false)
  const editingID = ref<string | null>(null)
  const formName = ref('')
  const formProvider = ref<StorageCredentialProvider>('OSS')
  const formAccessKey = ref('')
  const formSecretKey = ref('')
  const formErrors = ref<StorageCredentialFieldErrors>({})
  const saving = ref(false)
  let editorVersion = 0
  let target: StorageCredentialListItem | undefined
  onScopeDispose(closeEditor)
  watch(list.accessDenied, denied => { if (denied) closeEditor() }, { flush: 'sync' })
  function resetForm() {
    formName.value = formAccessKey.value = formSecretKey.value = ''
    formProvider.value = 'OSS'
    formErrors.value = {}
  }
  function openCreate() {
    if (!list.active() || list.writeBusy.value || list.accessDenied.value) return
    closeEditor()
    editorOpen.value = true
  }
  function openRotate(credential: StorageCredentialListItem) {
    if (!list.active() || list.writeBusy.value || list.accessDenied.value) return
    closeEditor()
    target = credential
    editingID.value = credential.id
    formName.value = credential.displayName
    formProvider.value = credential.provider
    editorOpen.value = true
  }
  function closeEditor() {
    editorVersion++
    editorOpen.value = saving.value = false
    editingID.value = null
    target = undefined
    resetForm()
  }
  async function submitForm() {
    if (!list.active() || !editorOpen.value || list.writeBusy.value) return
    const input = { displayName: formName.value, provider: formProvider.value, accessKey: formAccessKey.value, secretKey: formSecretKey.value } satisfies StorageCredentialWrite
    formErrors.value = validateStorageCredentialWrite(input)
    if (Object.keys(formErrors.value).length) return
    const current = target
    const transaction = list.beginWrite(current?.id ?? '')
    if (!transaction) return
    const version = editorVersion
    const valid = () => transaction.valid() && version === editorVersion && editorOpen.value
    saving.value = true
    try {
      // 捕获打开时的修订，保留 If-Match/幂等；刷新不能静默升级用户正在轮换的修订。
      const result = current
        ? await list.api.rotateStorageCredential(current.id, current.revision, input)
        : await list.api.createStorageCredential(input)
      if (!valid()) return
      list.credentials.value = current
        ? list.credentials.value.map(item => item.id === result.id ? result : item)
        : [...list.credentials.value, result]
      list.notice.value = current
        ? `凭据「${result.displayName}」已轮换为修订 ${result.currentRevision}；历史任务快照不受影响。`
        : `凭据「${result.displayName}」已创建；密钥只以加密信封保存，任何读取响应都不会回显。`
      closeEditor()
    } catch (error) {
      if (valid()) list.feedback.value = storageCredentialErrorMessage(error, current ? '凭据轮换失败。' : '凭据创建失败。')
    } finally {
      // 请求体仅在事务栈内持有；失败时保留当前表单便于重试，关闭/卸载另行清空。
      input.accessKey = input.secretKey = ''
      if (valid()) saving.value = false
      transaction.finish()
    }
  }
  return { editorOpen, editingID, formName, formProvider, formAccessKey, formSecretKey, formErrors, saving, openCreate, openRotate, closeEditor, submitForm }
}
