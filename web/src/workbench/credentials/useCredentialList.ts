import { computed, onScopeDispose, ref } from 'vue'
import { browserApi, storageCredentialErrorMessage, type ApiError, type StorageCredentialListItem } from '@/api/browser'
import { filterStorageCredentials } from './storageCredentialList'

export type CredentialApi = Pick<ReturnType<typeof browserApi>, 'listStorageCredentials' | 'createStorageCredential' | 'rotateStorageCredential' | 'deleteStorageCredential'>

// 授权列表与写事务锁的唯一所有者；密钥输入由编辑能力独立短时持有。
export function useCredentialList(createApi: (signal: AbortSignal) => CredentialApi = browserApi) {
  const controller = new AbortController()
  const api = createApi(controller.signal)
  const credentials = ref<StorageCredentialListItem[]>([])
  const loading = ref(true)
  const loadFailure = ref('')
  const feedback = ref('')
  const notice = ref('')
  const keyword = ref('')
  const providerFilter = ref('')
  const actionID = ref('')
  const writeBusy = ref(false)
  const accessDenied = ref(false)
  const visibleCredentials = computed(() => filterStorageCredentials(credentials.value, { keyword: keyword.value, provider: providerFilter.value }))
  let disposed = false
  let readVersion = 0
  let transaction = 0
  const active = () => !disposed
  onScopeDispose(() => { disposed = true; readVersion++; controller.abort() })

  async function loadCredentials() {
    if (!active() || writeBusy.value) return
    const version = ++readVersion
    const valid = () => active() && version === readVersion
    loading.value = true
    loadFailure.value = ''
    try {
      const result = await api.listStorageCredentials()
      if (!valid()) return
      credentials.value = result
      accessDenied.value = false
    } catch (error) {
      if (!valid()) return
      // 授权失败后，仅成功的授权读取能重新开放输入；临时故障不能解除阻断。
      if ([401, 403].includes((error as Partial<ApiError>).status ?? 0)) accessDenied.value = true
      if (accessDenied.value) credentials.value = []
      loadFailure.value = storageCredentialErrorMessage(error, '无法加载存储凭据，请稍后重试。')
    } finally {
      if (valid()) loading.value = false
    }
  }
  function beginWrite(id: string) {
    if (!active() || writeBusy.value || accessDenied.value) return
    const version = ++transaction
    readVersion++
    loading.value = false
    writeBusy.value = true
    actionID.value = id
    feedback.value = notice.value = ''
    return {
      valid: () => active() && version === transaction,
      finish() { if (active() && version === transaction) { writeBusy.value = false; actionID.value = '' } },
    }
  }
  return { api, active, credentials, loading, loadFailure, feedback, notice, keyword, providerFilter, actionID, writeBusy, accessDenied, visibleCredentials, loadCredentials, beginWrite }
}
