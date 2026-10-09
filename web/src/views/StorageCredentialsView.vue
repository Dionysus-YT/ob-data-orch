<script setup lang="ts">
import { Form as AForm, FormItem as AFormItem, Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
import { onMounted } from 'vue'
import { PlusOutlined, ReloadOutlined, SyncOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { storageCredentialProviders } from '@/workbench/credentials/storageCredentialList'
import { useCredentialList } from '@/workbench/credentials/useCredentialList'
import { useCredentialEditor } from '@/workbench/credentials/useCredentialEditor'
import { useCredentialDeletion } from '@/workbench/credentials/useCredentialDeletion'

const columns = [{ key: 'c0', title: '凭据' }, { key: 'c1', title: '提供方' }, { key: 'c2', title: '当前修订' }, { key: 'c3', title: '更新时间' }, { key: 'c4', title: '操作', width: '16%' }]
const list = useCredentialList()
const { credentials, loading, loadFailure, feedback, notice, keyword, providerFilter, actionID, visibleCredentials, loadCredentials } = list
const { editorOpen, editingID, formName, formProvider, formAccessKey, formSecretKey, formErrors, saving, openCreate, openRotate, closeEditor, submitForm } = useCredentialEditor(list)
const { pendingDelete, requestDelete, confirmDelete, cancelDelete } = useCredentialDeletion(list)
onMounted(() => { void loadCredentials() })
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
      <AButton :disabled="editorOpen" type="primary" @click="openCreate"><template #icon><PlusOutlined class="product-icon" aria-hidden="true" /></template>新增凭据</AButton>
    </header>

    <section class="storage-credential-filter" aria-label="存储凭据筛选">
      <label><span class="visually-hidden">关键字</span><AInput v-model:value.trim="keyword" aria-label="关键字" placeholder="按名称或标识搜索" /></label>
      <label><span class="visually-hidden">提供方</span><ASelect v-model:value="providerFilter" aria-label="提供方"><ASelectOption value="">全部提供方</ASelectOption><ASelectOption v-for="provider in storageCredentialProviders" :key="provider" :value="provider">{{ provider }}</ASelectOption></ASelect></label>
      <AButton :disabled="loading" @click="loadCredentials"><template #icon><ReloadOutlined class="product-icon" :spin="loading" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</AButton>
    </section>

    <p v-if="feedback" class="storage-credential-feedback is-error" role="alert">{{ feedback }}</p>
    <p v-else-if="notice" class="storage-credential-feedback is-notice" role="status">{{ notice }}</p>

    <section v-if="editorOpen" class="content-card storage-credential-editor" :aria-label="editingID ? '轮换存储凭据' : '新增存储凭据'">
      <h2>{{ editingID ? '轮换凭据' : '新增凭据' }}</h2>
      <p class="section-hint">{{ editingID ? '同时轮换 AccessKey 与 SecretKey 并递增修订；提交后旧修订立即失效，任务需重新保存草稿引用新修订。' : '凭据由当前主体独有；provider 决定其可绑定的输出类型。密钥不会进入日志、命令或快照。' }}</p>
      <AForm :model="{ formName, formProvider, formAccessKey, formSecretKey }" :disabled="saving" layout="vertical" class="storage-credential-form" @submit.prevent="submitForm">
        <AFormItem label="凭据名称" name="displayName" html-for="storage-credential-name" required :validate-status="formErrors.displayName ? 'error' : undefined" :help="formErrors.displayName">
          <AInput id="storage-credential-name" v-model:value.trim="formName" :aria-invalid="Boolean(formErrors.displayName)" placeholder="例如 生产 OSS 只读账号" autocomplete="off" />
        </AFormItem>
        <AFormItem label="提供方" name="provider" html-for="storage-credential-provider" required :validate-status="formErrors.provider ? 'error' : undefined" :help="formErrors.provider">
          <ASelect id="storage-credential-provider" v-model:value="formProvider" :aria-invalid="Boolean(formErrors.provider)" :disabled="Boolean(editingID)"><ASelectOption v-for="provider in storageCredentialProviders" :key="provider" :value="provider">{{ provider }}</ASelectOption></ASelect>
        </AFormItem>
        <AFormItem label="AccessKey" name="accessKey" html-for="storage-credential-access-key" required :validate-status="formErrors.accessKey ? 'error' : undefined" :help="formErrors.accessKey" extra="只写入加密信封；创建/轮换成功后本页立即清空输入。">
          <AInput id="storage-credential-access-key" v-model:value="formAccessKey" :aria-invalid="Boolean(formErrors.accessKey)" type="password" autocomplete="new-password" />
        </AFormItem>
        <AFormItem label="SecretKey" name="secretKey" html-for="storage-credential-secret-key" required :validate-status="formErrors.secretKey ? 'error' : undefined" :help="formErrors.secretKey" extra="与 AccessKey 分别加密为独立信封（AAD 绑定凭据标识与修订）。">
          <AInput id="storage-credential-secret-key" v-model:value="formSecretKey" :aria-invalid="Boolean(formErrors.secretKey)" type="password" autocomplete="new-password" />
        </AFormItem>
      </AForm>
      <div class="storage-credential-editor-actions">
        <AButton :disabled="saving" type="primary" @click="submitForm">{{ saving ? '提交中…' : editingID ? '确认轮换' : '创建' }}</AButton>
        <AButton :disabled="saving" @click="closeEditor">取消</AButton>
      </div>
    </section>

    <section class="content-card table-card">
      <OrchOperationalTable :rows="visibleCredentials" :columns="columns" row-key="id" label="存储凭据列表" :loading="loading">
        <template #bodyCell="{ record: credential, column }">
          <div v-if="column.key === 'c0'" class="storage-credential-name"><div class="cell-stack"><strong :title="credential.displayName">{{ credential.displayName }}</strong><small>{{ credential.id }}</small></div></div>
          <div v-else-if="column.key === 'c1'">{{ credential.provider }}</div>
          <div v-else-if="column.key === 'c2'">{{ credential.currentRevision }}</div>
          <div v-else-if="column.key === 'c3'">{{ updatedAtLabel(credential.updatedAt) }}</div>
          <div v-else-if="column.key === 'c4'" class="storage-credential-row-actions">
            <AButton :disabled="Boolean(actionID) || editorOpen" type="text" class="row-action" @click="openRotate(credential)"><template #icon><SyncOutlined class="product-icon" aria-hidden="true" /></template>轮换</AButton>
            <AButton :disabled="Boolean(actionID) || editorOpen" type="text" danger class="row-action is-danger" @click="requestDelete(credential)"><template #icon><DeleteOutlined class="product-icon" aria-hidden="true" /></template>删除</AButton>
          </div>
        </template>
        <template #empty><div v-if="loading">正在加载存储凭据…</div><div v-else-if="loadFailure"><strong>无法加载存储凭据</strong><span>{{ loadFailure }}</span><AButton @click="loadCredentials">重试</AButton></div><div v-else-if="credentials.length === 0"><strong>暂无存储凭据</strong><span>新增后可在导出向导的对象存储输出中选择使用。</span></div><div v-else-if="visibleCredentials.length === 0"><strong>没有符合当前筛选条件的凭据</strong><span>请调整筛选条件。</span></div></template>
      </OrchOperationalTable>
    </section>

    <OrchDangerConfirm :open="Boolean(pendingDelete)" :title="`删除存储凭据：${pendingDelete?.displayName ?? ''}`" confirm-label="确认删除" destructive :busy="Boolean(actionID)" @confirm="confirmDelete" @cancel="cancelDelete">
      <p>删除后凭据及其全部加密信封修订将被物理移除，无法恢复。已提交任务的历史快照只保留引用标识，不会因此改写或回填密钥。</p>
    </OrchDangerConfirm>

    <p class="section-hint">对象存储输出的任务提交仍由存储专用预检查门禁阻断；凭据创建成功不代表对象存储可用或任务可执行。</p>
  </div>
</template>

<style scoped>
.storage-credential-page {
  color: var(--ob-color-form-text);
}

.storage-credential-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ob-foundation-space-6);
  margin-bottom: var(--ob-foundation-space-6);
}

.storage-credential-header h1 {
  margin: 0;
  font-size: var(--ob-product-typography-title-size);
  font-weight: var(--ob-product-typography-weight);
  letter-spacing: 0;
  line-height: var(--ob-product-typography-title-line-height);
}

.storage-credential-header p {
  margin: var(--ob-foundation-space-1) 0 0;
  color: var(--ob-color-form-secondary);
  font-size: var(--ob-component-control-font-size);
  line-height: var(--ob-component-control-line-height);
}

.storage-credential-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ob-foundation-space-2);
  padding: 0 0 var(--ob-foundation-space-4);
  border-bottom: 1px solid var(--ob-color-border);
}

.storage-credential-filter label {
  min-width: 160px;
  flex: 0 1 260px;
}

.storage-credential-feedback {
  margin: var(--ob-foundation-space-3) 0 0;
  padding: var(--ob-foundation-space-2) var(--ob-foundation-space-3);
  border-left: 3px solid var(--ob-color-border-strong);
  color: var(--ob-color-secondary);
  background: var(--ob-color-page);
  font-size: var(--ob-component-field-label-size);
  line-height: 20px;
}

.storage-credential-feedback.is-error {
  border-left-color: var(--ob-color-danger);
  color: var(--ob-color-danger);
  background: var(--ob-foundation-danger-surface);
}

.storage-credential-editor {
  margin: var(--ob-foundation-space-4) 0 0;
  padding: var(--ob-foundation-space-4);
}

.storage-credential-editor h2 {
  margin: 0;
  font-size: var(--ob-product-typography-section-size);
  font-weight: var(--ob-product-typography-weight);
  line-height: var(--ob-component-overlay-title-line-height);
}

.storage-credential-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--ob-foundation-space-3);
  margin-top: var(--ob-foundation-space-3);
}

.storage-credential-editor-actions {
  display: flex;
  gap: var(--ob-foundation-space-2);
  margin-top: var(--ob-foundation-space-3);
}

.storage-credential-name strong {
  overflow: hidden;
  color: var(--ob-color-text);
  font-size: var(--ob-component-control-font-size);
  font-weight: var(--ob-product-typography-weight);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.storage-credential-name small {
  overflow: hidden;
  color: var(--ob-color-muted);
  font-size: var(--ob-component-field-helper-size);
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
  gap: var(--ob-foundation-space-1);
  white-space: nowrap;
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
    gap: var(--ob-foundation-space-3);
  }

  .storage-credential-form {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
