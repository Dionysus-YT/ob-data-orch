<script setup lang="ts">
import { Alert as AAlert, Drawer as ADrawer, Empty as AEmpty, Form as AForm, FormItem as AFormItem, Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Skeleton as ASkeleton } from 'ant-design-vue'
import { onMounted, reactive } from 'vue'
import { PlusOutlined, ReloadOutlined, SyncOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import { product } from '@/platform/tokens'
import { storageCredentialProviders } from '@/workbench/credentials/storageCredentialList'
import { useCredentialList } from '@/workbench/credentials/useCredentialList'
import { useCredentialEditor } from '@/workbench/credentials/useCredentialEditor'
import { useCredentialDeletion } from '@/workbench/credentials/useCredentialDeletion'

const columns = [{ key: 'c0', title: '凭据' }, { key: 'c1', title: '提供方' }, { key: 'c2', title: '当前修订' }, { key: 'c3', title: '更新时间' }, { key: 'c4', title: '操作', width: '16%' }]
const list = useCredentialList()
const { credentials, loading, loadFailure, feedback, notice, keyword, providerFilter, actionID, visibleCredentials, loadCredentials } = list
const { MessageContext } = useOperationMessage(notice)
const { editorOpen, editingID, formName, formProvider, formAccessKey, formSecretKey, formErrors, saving, openCreate, openRotate, closeEditor, submitForm } = useCredentialEditor(list)
// 直接绑定编辑器的原 ref，Ant 校验立即读取当前值，不复制秘密或保留上一渲染的值。
const formModel = reactive({ displayName: formName, provider: formProvider, accessKey: formAccessKey, secretKey: formSecretKey })
const { pendingDelete, requestDelete, confirmDelete, cancelDelete } = useCredentialDeletion(list)
const { heading, afterOpenChange } = useAntDrawerDialog('storage-credential-editor-title', closeEditor, () => saving.value, () => editorOpen.value)
void heading
onMounted(() => { void loadCredentials() })
function updatedAtLabel(value: string) {
  const time = new Date(value)
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString()
}
</script>

<template>
  <div class="orch-ui">
    <MessageContext />
    <header class="orch-page-header"><div class="orch-page-title"><h1>存储凭据</h1></div></header>
    <section class="orch-management" aria-label="存储凭据工作面">
      <div class="orch-table-meta">
        <h2>存储凭据列表</h2>
        <div class="orch-page-actions">
          <AButton :disabled="loading" @click="loadCredentials"><template #icon><ReloadOutlined class="product-icon" :spin="loading" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</AButton>
          <AButton :disabled="editorOpen || Boolean(actionID)" type="primary" @click="openCreate"><template #icon><PlusOutlined class="product-icon" aria-hidden="true" /></template>新增凭据</AButton>
        </div>
      </div>
      <div class="orch-toolbar storage-credential-filters" role="search" aria-label="存储凭据筛选">
        <AInput v-model:value.trim="keyword" aria-label="关键字" placeholder="按名称或标识搜索" allow-clear />
        <ASelect v-model:value="providerFilter" aria-label="提供方"><ASelectOption value="">全部提供方</ASelectOption><ASelectOption v-for="provider in storageCredentialProviders" :key="provider" :value="provider">{{ provider }}</ASelectOption></ASelect>
      </div>

      <AAlert v-if="feedback && !editorOpen" class="orch-feedback" type="error" message="操作未完成" :description="feedback" show-icon />
      <AAlert v-if="loadFailure" class="orch-feedback" type="error" message="无法加载存储凭据" :description="loadFailure" show-icon><template #action><AButton @click="loadCredentials">重试</AButton></template></AAlert>

      <OrchOperationalTable v-if="visibleCredentials.length" :rows="visibleCredentials" :columns="columns" row-key="id" label="存储凭据列表" :loading="loading" :minimum-width="850">
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
      </OrchOperationalTable>
      <ASkeleton v-else-if="loading" active :paragraph="{ rows: 4 }" role="status" aria-label="正在读取存储凭据" />
      <AEmpty v-else-if="!loadFailure" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" :description="credentials.length ? '没有符合当前筛选条件的凭据' : '暂无存储凭据'" />
    </section>

    <ADrawer :open="editorOpen" :width="`min(${product.inspector.width}px, 100vw)`" :destroy-on-close="true" :closable="!saving" :mask-closable="!saving" :keyboard="!saving" root-class-name="orch-inspector" class="orch-ui" @close="closeEditor" @after-open-change="afterOpenChange">
      <template #title><h2 id="storage-credential-editor-title" ref="heading" class="ant-drawer-title" tabindex="-1">{{ editingID ? '轮换凭据' : '新增凭据' }}</h2></template>
      <AAlert v-if="feedback" type="error" message="操作未完成" :description="feedback" show-icon />
      <AAlert v-if="editingID" type="warning" show-icon message="轮换后，使用旧版本凭据的草稿需重新绑定。" />
      <AForm :model="formModel" :disabled="saving" layout="vertical" class="storage-credential-form" @finish="submitForm">
        <AFormItem label="凭据名称" name="displayName" html-for="storage-credential-name" required :validate-status="formErrors.displayName ? 'error' : undefined" :help="formErrors.displayName">
          <AInput id="storage-credential-name" v-model:value.trim="formName" :aria-invalid="Boolean(formErrors.displayName)" placeholder="例如 生产 OSS 只读账号" autocomplete="off" />
        </AFormItem>
        <AFormItem label="提供方" name="provider" html-for="storage-credential-provider" required :validate-status="formErrors.provider ? 'error' : undefined" :help="formErrors.provider">
          <ASelect id="storage-credential-provider" v-model:value="formProvider" :aria-invalid="Boolean(formErrors.provider)" :disabled="Boolean(editingID)"><ASelectOption v-for="provider in storageCredentialProviders" :key="provider" :value="provider">{{ provider }}</ASelectOption></ASelect>
        </AFormItem>
        <AFormItem label="AccessKey" name="accessKey" html-for="storage-credential-access-key" required :validate-status="formErrors.accessKey ? 'error' : undefined" :help="formErrors.accessKey">
          <AInput id="storage-credential-access-key" v-model:value="formAccessKey" :aria-invalid="Boolean(formErrors.accessKey)" type="password" autocomplete="new-password" />
        </AFormItem>
        <AFormItem label="SecretKey" name="secretKey" html-for="storage-credential-secret-key" required :validate-status="formErrors.secretKey ? 'error' : undefined" :help="formErrors.secretKey" extra="保存后不再显示，请妥善保管。">
          <AInput id="storage-credential-secret-key" v-model:value="formSecretKey" :aria-invalid="Boolean(formErrors.secretKey)" type="password" autocomplete="new-password" />
        </AFormItem>
      </AForm>
      <template #footer><div class="storage-credential-editor-actions"><AButton :disabled="saving" @click="closeEditor">取消</AButton><AButton :loading="saving" type="primary" @click="submitForm">{{ editingID ? '确认轮换' : '创建' }}</AButton></div></template>
    </ADrawer>

    <OrchDangerConfirm :open="Boolean(pendingDelete)" :title="`删除存储凭据：${pendingDelete?.displayName ?? ''}`" confirm-label="确认删除" destructive :busy="Boolean(actionID)" @confirm="confirmDelete" @cancel="cancelDelete">
      <p>将删除该凭据的所有版本，无法恢复。</p>
    </OrchDangerConfirm>
  </div>
</template>

<style scoped>
.storage-credential-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ob-foundation-space-2);
}

.storage-credential-filters > * {
  min-width: 160px;
  flex: 0 1 260px;
}

.storage-credential-form {
  margin-top: var(--ob-foundation-space-3);
}

.storage-credential-editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--ob-foundation-space-2);
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

</style>
