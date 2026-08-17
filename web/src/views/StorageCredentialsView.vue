<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Plus, RefreshCw, RotateCw, Trash2 } from '@lucide/vue'

import { browserApi, storageCredentialErrorMessage, type StorageCredentialListItem, type StorageCredentialProvider, type StorageCredentialWrite } from '@/api/browser'
import WorkbenchButton from '@/components/WorkbenchButton.vue'
import WorkbenchFormField from '@/components/WorkbenchFormField.vue'
import { filterStorageCredentials, storageCredentialProviders, validateStorageCredentialWrite, type StorageCredentialFieldErrors } from './storageCredentialList'

const api = browserApi()
const credentials = ref<StorageCredentialListItem[]>([])
const loading = ref(true)
const loadFailure = ref('')
const feedback = ref('')
const notice = ref('')
const keyword = ref('')
const providerFilter = ref('')
const actionID = ref('')

const editorOpen = ref(false)
const editingID = ref<string | null>(null)
const formName = ref('')
const formProvider = ref<StorageCredentialProvider>('OSS')
const formAccessKey = ref('')
const formSecretKey = ref('')
const formErrors = ref<StorageCredentialFieldErrors>({})
const saving = ref(false)

const pendingDelete = ref<StorageCredentialListItem>()

const visibleCredentials = computed(() => filterStorageCredentials(credentials.value, { keyword: keyword.value, provider: providerFilter.value }))

onMounted(() => {
  void loadCredentials()
})

async function loadCredentials() {
  loading.value = true
  await refreshCredentials()
  loading.value = false
}

async function refreshCredentials() {
  loadFailure.value = ''
  try {
    credentials.value = await api.listStorageCredentials()
  } catch (error) {
    loadFailure.value = storageCredentialErrorMessage(error, '无法加载存储凭据，请稍后重试。')
  }
}

function resetForm() {
  formName.value = ''
  formProvider.value = 'OSS'
  formAccessKey.value = ''
  formSecretKey.value = ''
  formErrors.value = {}
}

function openCreate() {
  editingID.value = null
  resetForm()
  editorOpen.value = true
}

function openRotate(credential: StorageCredentialListItem) {
  editingID.value = credential.id
  formName.value = credential.displayName
  formProvider.value = credential.provider
  formAccessKey.value = ''
  formSecretKey.value = ''
  formErrors.value = {}
  editorOpen.value = true
}

function closeEditor() {
  editorOpen.value = false
  editingID.value = null
  resetForm()
}

async function submitForm() {
  const input: StorageCredentialWrite = {
    displayName: formName.value,
    provider: formProvider.value,
    accessKey: formAccessKey.value,
    secretKey: formSecretKey.value,
  }
  formErrors.value = validateStorageCredentialWrite(input)
  if (Object.keys(formErrors.value).length > 0) return
  saving.value = true
  feedback.value = ''
  notice.value = ''
  const target = editingID.value ? credentials.value.find((item) => item.id === editingID.value) : undefined
  try {
    if (target) {
      // 轮换使用 If-Match 乐观锁与幂等键；成功后以新修订替换当前行。
      const rotated = await api.rotateStorageCredential(target.id, target.revision, input)
      actionID.value = target.id
      credentials.value = credentials.value.map((item) => item.id === rotated.id ? rotated : item)
      notice.value = `凭据「${rotated.displayName}」已轮换为修订 ${rotated.currentRevision}；历史任务快照不受影响。`
    } else {
      const created = await api.createStorageCredential(input)
      credentials.value = [...credentials.value, created]
      notice.value = `凭据「${created.displayName}」已创建；密钥只以加密信封保存，任何读取响应都不会回显。`
    }
    closeEditor()
  } catch (error) {
    feedback.value = storageCredentialErrorMessage(error, target ? '凭据轮换失败。' : '凭据创建失败。')
  } finally {
    saving.value = false
    actionID.value = ''
  }
}

function requestDelete(credential: StorageCredentialListItem) {
  pendingDelete.value = credential
}

async function confirmDelete() {
  const target = pendingDelete.value
  if (!target) return
  actionID.value = target.id
  feedback.value = ''
  notice.value = ''
  try {
    await api.deleteStorageCredential(target.id, target.revision)
    credentials.value = credentials.value.filter((item) => item.id !== target.id)
    notice.value = `凭据「${target.displayName}」已删除；已冻结任务的历史快照只保留引用标识。`
  } catch (error) {
    feedback.value = storageCredentialErrorMessage(error, '凭据删除失败。')
  } finally {
    actionID.value = ''
    pendingDelete.value = undefined
  }
}

function updatedAtLabel(value: string) {
  const time = new Date(value)
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString()
}
</script>

<template>
  <div class="storage-credential-page">
    <header class="storage-credential-header">
      <div>
        <h1>存储凭据</h1>
        <p>管理对象存储（OSS/S3/COS/OBS）的 AccessKey/SecretKey。密钥只在创建或轮换请求体内短暂存在，保存后仅以加密信封落库；任何列表、详情、任务快照或日志都不会回显密钥。</p>
      </div>
      <WorkbenchButton variant="primary" :disabled="editorOpen" @click="openCreate"><template #icon><Plus :size="15" aria-hidden="true" /></template>新增凭据</WorkbenchButton>
    </header>

    <section class="storage-credential-filter" aria-label="存储凭据筛选">
      <label><span class="visually-hidden">关键字</span><input v-model.trim="keyword" placeholder="按名称或标识搜索" /></label>
      <label><span class="visually-hidden">提供方</span><select v-model="providerFilter" aria-label="提供方"><option value="">全部提供方</option><option v-for="provider in storageCredentialProviders" :key="provider" :value="provider">{{ provider }}</option></select></label>
      <WorkbenchButton :disabled="loading" @click="loadCredentials"><template #icon><RefreshCw :size="14" :class="{ 'is-spinning': loading }" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</WorkbenchButton>
    </section>

    <p v-if="feedback" class="storage-credential-feedback is-error" role="alert">{{ feedback }}</p>
    <p v-else-if="notice" class="storage-credential-feedback is-notice" role="status">{{ notice }}</p>

    <section v-if="editorOpen" class="content-card storage-credential-editor" :aria-label="editingID ? '轮换存储凭据' : '新增存储凭据'">
      <h2>{{ editingID ? '轮换凭据' : '新增凭据' }}</h2>
      <p class="section-hint">{{ editingID ? '同时轮换 AccessKey 与 SecretKey 并递增修订；提交后旧修订立即失效，任务需重新保存草稿引用新修订。' : '凭据由当前主体独有；provider 决定其可绑定的输出类型。密钥不会进入日志、命令或快照。' }}</p>
      <div class="storage-credential-form">
        <WorkbenchFormField label="凭据名称" required :error="formErrors.displayName">
          <input v-model.trim="formName" class="tree-input" placeholder="例如 生产 OSS 只读账号" autocomplete="off" />
        </WorkbenchFormField>
        <WorkbenchFormField label="提供方" required :error="formErrors.provider">
          <select v-model="formProvider" class="tree-input tree-select" :disabled="Boolean(editingID)"><option v-for="provider in storageCredentialProviders" :key="provider" :value="provider">{{ provider }}</option></select>
        </WorkbenchFormField>
        <WorkbenchFormField label="AccessKey" required :error="formErrors.accessKey" helper="只写入加密信封；创建/轮换成功后本页立即清空输入。">
          <input v-model="formAccessKey" type="password" class="tree-input" autocomplete="new-password" />
        </WorkbenchFormField>
        <WorkbenchFormField label="SecretKey" required :error="formErrors.secretKey" helper="与 AccessKey 分别加密为独立信封（AAD 绑定凭据标识与修订）。">
          <input v-model="formSecretKey" type="password" class="tree-input" autocomplete="new-password" />
        </WorkbenchFormField>
      </div>
      <div class="storage-credential-editor-actions">
        <WorkbenchButton variant="primary" :disabled="saving" @click="submitForm">{{ saving ? '提交中…' : editingID ? '确认轮换' : '创建' }}</WorkbenchButton>
        <WorkbenchButton :disabled="saving" @click="closeEditor">取消</WorkbenchButton>
      </div>
    </section>

    <section class="content-card table-card">
      <table>
        <thead><tr><th>凭据</th><th>提供方</th><th>当前修订</th><th>更新时间</th><th class="storage-credential-actions">操作</th></tr></thead>
        <tbody v-if="loading"><tr><td colspan="5" class="storage-credential-message">正在加载存储凭据…</td></tr></tbody>
        <tbody v-else-if="loadFailure"><tr><td colspan="5" class="storage-credential-message is-error"><strong>无法加载存储凭据</strong><span>{{ loadFailure }}</span><WorkbenchButton @click="loadCredentials">重试</WorkbenchButton></td></tr></tbody>
        <tbody v-else-if="credentials.length === 0"><tr><td colspan="5" class="storage-credential-message"><strong>暂无存储凭据</strong><span>新增后可在导出向导的对象存储输出中选择使用。</span></td></tr></tbody>
        <tbody v-else-if="visibleCredentials.length === 0"><tr><td colspan="5" class="storage-credential-message"><strong>没有符合当前筛选条件的凭据</strong><span>请调整筛选条件。</span></td></tr></tbody>
        <tbody v-else>
          <tr v-for="credential in visibleCredentials" :key="credential.id">
            <td class="storage-credential-name"><div class="cell-stack"><strong :title="credential.displayName">{{ credential.displayName }}</strong><small>{{ credential.id }}</small></div></td>
            <td>{{ credential.provider }}</td>
            <td>{{ credential.currentRevision }}</td>
            <td>{{ updatedAtLabel(credential.updatedAt) }}</td>
            <td class="storage-credential-row-actions">
              <button type="button" class="row-action" :disabled="Boolean(actionID) || editorOpen" @click="openRotate(credential)"><RotateCw :size="13" aria-hidden="true" />轮换</button>
              <button type="button" class="row-action is-danger" :disabled="Boolean(actionID) || editorOpen" @click="requestDelete(credential)"><Trash2 :size="13" aria-hidden="true" />删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="pendingDelete" class="content-card storage-credential-delete" role="alertdialog" :aria-label="`删除存储凭据：${pendingDelete.displayName}`">
      <h2>删除凭据「{{ pendingDelete.displayName }}」？</h2>
      <p>删除后凭据及其全部加密信封修订将被物理移除，无法恢复。已提交任务的历史快照只保留引用标识，不会因此改写或回填密钥。</p>
      <div class="storage-credential-delete-actions">
        <WorkbenchButton variant="danger" :disabled="Boolean(actionID)" @click="confirmDelete">确认删除</WorkbenchButton>
        <WorkbenchButton :disabled="Boolean(actionID)" @click="pendingDelete = undefined">取消</WorkbenchButton>
      </div>
    </section>

    <p class="section-hint">对象存储输出的任务提交仍由存储专用预检查门禁阻断；凭据创建成功不代表对象存储可用或任务可执行。</p>
  </div>
</template>

<style scoped>
.storage-credential-page {
  color: var(--color-text-primary);
}

.storage-credential-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-6);
  margin-bottom: var(--space-6);
}

.storage-credential-header h1 {
  margin: 0;
  font-size: var(--text-page-title-size);
  font-weight: var(--font-weight-semibold);
  letter-spacing: -.01em;
  line-height: var(--text-page-title-line-height);
}

.storage-credential-header p {
  margin: var(--space-1) 0 0;
  color: var(--color-text-secondary);
  font-size: var(--text-body-size);
  line-height: var(--text-body-line-height);
}

.storage-credential-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  padding: 0 0 var(--space-4);
  border-bottom: 1px solid var(--color-border-default);
}

.storage-credential-filter label {
  min-width: 160px;
  flex: 0 1 260px;
}

.storage-credential-filter input,
.storage-credential-filter select,
.storage-credential-form input,
.storage-credential-form select {
  width: 100%;
  height: var(--size-control);
  padding: 0 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-control);
  color: #2b3037;
  background: var(--color-bg-surface);
  font-size: 13px;
}

.storage-credential-filter input:focus,
.storage-credential-filter select:focus,
.storage-credential-form input:focus,
.storage-credential-form select:focus {
  border-color: var(--color-primary);
  outline: 2px solid rgb(37 103 185 / 14%);
  outline-offset: 0;
}

.storage-credential-feedback {
  margin: var(--space-3) 0 0;
  padding: var(--space-2) var(--space-3);
  border-left: 3px solid #8fb0dc;
  color: #526174;
  background: #f4f7fb;
  font-size: 13px;
  line-height: 20px;
}

.storage-credential-feedback.is-error {
  border-left-color: #d45a52;
  color: #9f2f28;
  background: #fff5f4;
}

.storage-credential-editor,
.storage-credential-delete {
  margin: var(--space-4) 0 0;
  padding: var(--space-4);
}

.storage-credential-editor h2,
.storage-credential-delete h2 {
  margin: 0;
  font-size: var(--text-section-title-size);
  font-weight: var(--font-weight-semibold);
  line-height: var(--text-section-title-line-height);
}

.storage-credential-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
  margin-top: var(--space-3);
}

.storage-credential-editor-actions,
.storage-credential-delete-actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-3);
}

.storage-credential-delete p {
  margin: var(--space-2) 0 0;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.storage-credential-actions {
  width: 16%;
  text-align: right !important;
}

.storage-credential-name strong {
  overflow: hidden;
  color: #1e2329;
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.storage-credential-name small {
  overflow: hidden;
  color: var(--color-text-tertiary);
  font-size: 11px;
  line-height: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-stack {
  display: grid;
  align-content: center;
  gap: 1px;
}

.storage-credential-row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-1);
  white-space: nowrap;
}

.row-action {
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0 var(--space-1);
  border: 0;
  border-radius: 4px;
  color: #245d9e;
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  font-weight: 500;
  line-height: 30px;
  cursor: pointer;
}

.row-action.is-danger { color: #b42318; }

.row-action:hover:not(:disabled) { background: var(--color-primary-soft); }
.row-action.is-danger:hover:not(:disabled) { background: #fff1f0; }

.row-action:disabled {
  color: #99a3b0;
  cursor: not-allowed;
}

.storage-credential-message {
  height: 160px !important;
  color: var(--color-text-secondary) !important;
  text-align: center !important;
}

.storage-credential-message strong,
.storage-credential-message span {
  display: block;
}

.storage-credential-message strong {
  margin-bottom: 4px;
  color: #2b394d;
  font-size: 14px;
}

.storage-credential-message span {
  margin-bottom: 12px;
  color: var(--color-text-tertiary);
  font-size: 12px;
}

.storage-credential-message.is-error strong {
  color: #a6352c;
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 760px) {
  .storage-credential-header {
    align-items: stretch;
    flex-direction: column;
    gap: 14px;
  }

  .storage-credential-header :deep(.workbench-button) {
    align-self: flex-start;
  }

  .storage-credential-form {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
