<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { FileAddOutlined, EditOutlined, ReloadOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import EmptyState from '@/components/EmptyState.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useTemplateCatalog } from '@/workbench/templates/useTemplateCatalog'
import { useTemplateActions } from '@/workbench/templates/useTemplateActions'
import { useTemplateDraft } from '@/workbench/templates/useTemplateDraft'
import { taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import { nodePlatformLabel } from '@/workbench/nodes/executionNodePresentation'

const columns = [{ key: 'c0', title: '模板名称' }, { key: 'c1', title: '能力版本' }, { key: 'c2', title: '来源任务' }, { key: 'c3', title: '更新时间' }, { key: 'c4', title: '操作', width: '26%' }]
const router = useRouter()
const catalog = useTemplateCatalog()
const { templates, loading, loadFailure, notice, actionBusy, refresh } = catalog
const { MessageContext } = useOperationMessage(notice)
const { pendingDelete, renameID, renameValue, startRename, confirmRename, requestDelete, confirmDelete, cancelRename, cancelDelete } = useTemplateActions(catalog)
const { selectedTemplate, draftSourceID, draftNodeID, sourcesLoading, createDraftFailure, eligibleSources, nodes, canCreate, selectTemplate, loadChoices, onNodeDropdownVisibleChange, createDraft } = useTemplateDraft(catalog,
  (id, type) => router.push({ path: type === 'OBLOADER_NORMAL_IMPORT' ? '/imports/normal/new' : '/exports/new', query: { draft: id, step: '1' } }),
)
onMounted(() => { void refresh() })
function dateTime(value: string) {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString()
}
</script>

<template>
  <MessageContext />
  <header class="orch-page-header"><div class="orch-page-title"><h1>模板中心</h1></div></header>
  <section class="orch-management" aria-label="模板工作面">
    <div class="orch-table-meta"><h2>模板列表</h2><div class="orch-page-actions"><AButton :disabled="loading" @click="refresh"><template #icon><ReloadOutlined class="product-icon" :spin="loading" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</AButton></div></div>
    <AAlert v-if="loadFailure" class="orch-feedback" type="error" message="操作未完成" :description="loadFailure" show-icon />
    <OrchOperationalTable :rows="templates" :columns="columns" row-key="id" label="模板列表" :loading="loading">
      <template #bodyCell="{ record: template, column }">
        <div v-if="column.key === 'c0'" class="template-name"><div v-if="renameID !== template.id" class="cell-stack"><strong :title="template.displayName">{{ template.displayName }}</strong><small>{{ taskTypeText(template.taskType) }} · {{ template.id }}</small></div><div v-else class="template-rename"><AInput v-model:value.trim="renameValue" aria-label="模板名称" /><AButton :disabled="Boolean(actionBusy)" type="primary" @click="confirmRename(template)">保存</AButton><AButton :disabled="Boolean(actionBusy)" @click="cancelRename">取消</AButton></div></div>
        <div v-else-if="column.key === 'c1'"><code>{{ template.capabilityVersion }}</code></div>
        <div v-else-if="column.key === 'c2'">{{ template.sourceTaskId || '手工创建' }}</div>
        <div v-else-if="column.key === 'c3'">{{ dateTime(template.updatedAt) }}</div>
        <div v-else-if="column.key === 'c4'" class="template-row-actions"><AButton :disabled="Boolean(actionBusy)" type="text" class="row-action" @click="startRename(template)"><template #icon><EditOutlined class="product-icon" aria-hidden="true" /></template>改名</AButton><AButton :disabled="Boolean(actionBusy)" type="text" class="row-action" @click="selectTemplate(template)"><template #icon><FileAddOutlined class="product-icon" aria-hidden="true" /></template>用模板新建草稿</AButton><AButton :disabled="Boolean(actionBusy)" type="text" danger class="row-action is-danger" @click="requestDelete(template)"><template #icon><DeleteOutlined class="product-icon" aria-hidden="true" /></template>删除</AButton></div>
      </template>
      <template #empty><div v-if="loading">正在加载模板…</div><div v-else-if="templates.length === 0"><EmptyState title="尚无可展示模板" description="在成功任务的详情页保存模板。" /></div></template>
    </OrchOperationalTable>
  </section>
  <section v-if="selectedTemplate" class="content-card template-draft-choices" aria-label="由模板创建草稿">
    <h2>由模板创建草稿：{{ selectedTemplate.displayName }}</h2>
    <p v-if="selectedTemplate.taskType === 'OBDUMPER_EXPORT'" class="section-hint">对象存储凭据须在导出向导中重新绑定。</p>
    <AAlert v-if="createDraftFailure" class="orch-feedback" type="error" message="无法创建草稿" :description="createDraftFailure" show-icon />
    <div class="template-choice-row">
      <label>数据源<ASelect v-model:value="draftSourceID" aria-label="数据源" :loading="sourcesLoading" :disabled="sourcesLoading || Boolean(actionBusy)"><ASelectOption value="" disabled>请选择数据源</ASelectOption><ASelectOption v-for="source in eligibleSources" :key="source.id" :value="source.id">{{ source.displayName }}</ASelectOption></ASelect></label>
      <label>执行节点<ASelect v-model:value="draftNodeID" aria-label="执行节点" :loading="sourcesLoading" :disabled="sourcesLoading && !nodes.length || Boolean(actionBusy)" @dropdown-visible-change="onNodeDropdownVisibleChange"><ASelectOption value="" disabled>请选择执行节点</ASelectOption><ASelectOption v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ nodePlatformLabel(node.platform) }}</ASelectOption></ASelect></label>
      <AButton :loading="sourcesLoading" :disabled="Boolean(actionBusy)" @click="loadChoices(true)">刷新候选</AButton>
      <AButton type="primary" :loading="Boolean(actionBusy)" :disabled="!canCreate" @click="createDraft">创建草稿</AButton>
    </div>
    <p v-if="!sourcesLoading && !createDraftFailure && (!eligibleSources.length || !nodes.length)" class="section-hint">当前授权范围内没有可用于该类型的完整绑定，请检查数据源与节点状态后刷新候选。</p>
  </section>
  <OrchDangerConfirm :open="Boolean(pendingDelete)" :title="`删除模板：${pendingDelete?.displayName ?? ''}`" confirm-label="删除" destructive :busy="Boolean(actionBusy)" @confirm="confirmDelete" @cancel="cancelDelete"><p>删除后不可恢复；已创建的任务不受影响。</p></OrchDangerConfirm>
</template>

<style scoped>
.template-row-actions { display: flex; align-items: center; justify-content: flex-end; gap: var(--ob-foundation-space-1); white-space: nowrap; }
.template-name strong { overflow: hidden; color: var(--ob-color-text); font-size: var(--ob-component-control-font-size); font-weight: var(--ob-product-typography-weight); text-overflow: ellipsis; white-space: nowrap; }
.template-name small { overflow: hidden; color: var(--ob-color-muted); font-size: var(--ob-component-field-helper-size); line-height: 17px; text-overflow: ellipsis; white-space: nowrap; }
.cell-stack { display: grid; align-content: center; gap: 1px; }
.template-rename { display: flex; gap: var(--ob-foundation-space-1); align-items: center; }
.template-draft-choices { margin-top: var(--ob-foundation-space-4); padding: var(--ob-foundation-space-4); }
.template-choice-row { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-3); margin-top: var(--ob-foundation-space-2); }
.template-choice-row label { display: grid; gap: var(--ob-foundation-space-1); min-width: 240px; color: var(--ob-color-form-secondary); font-size: var(--ob-component-field-label-size); }
</style>
