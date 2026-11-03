<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Dropdown as ADropdown, Input as AInput, Menu as AMenu, MenuItem as AMenuItem, Select as ASelect, SelectOption as ASelectOption, Tag as ATag, Tooltip as ATooltip } from 'ant-design-vue'
import { onMounted, reactive, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { DeleteOutlined, EditOutlined, FileAddOutlined, LeftOutlined, MoreOutlined, ReloadOutlined, RightOutlined, SearchOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import EmptyState from '@/components/EmptyState.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import { useTemplateCatalog } from '@/workbench/templates/useTemplateCatalog'
import { useTemplateActions } from '@/workbench/templates/useTemplateActions'
import { useTemplateDraft } from '@/workbench/templates/useTemplateDraft'
import TemplateDraftDrawer from '@/workbench/templates/TemplateDraftDrawer.vue'
import TemplateRenameDialog from '@/workbench/templates/TemplateRenameDialog.vue'
import type { ConfigTemplateItem } from '@/api/browser'

const columns = [{ key: 'name', title: '模板名称', width: '34%' }, { key: 'type', title: '任务类型', className: 'template-type' }, { key: 'source', title: '来源任务', width: '30%' }, { key: 'updated', title: '更新时间', className: 'template-time' }, { key: 'actions', title: '操作', width: 160 }]
const router = useRouter()
const catalog = useTemplateCatalog()
const actions = useTemplateActions(catalog)
const draft = useTemplateDraft(catalog, async (id, type) => {
  const failure = await router.push({ path: type === 'OBLOADER_NORMAL_IMPORT' ? '/imports/normal/new' : '/exports/new', query: { draft: id, step: '1' } })
  if (failure) throw new Error('草稿导航未完成')
})
const state = reactive(catalog), editor = reactive(actions), binding = reactive(draft)
const { MessageContext } = useOperationMessage(catalog.notice)
function closeDraft() { if (!state.actionBusy) draft.clearSelection() }
function useTemplate(item: ConfigTemplateItem) { actions.cancelRename(); actions.cancelDelete(); draft.selectTemplate(item) }
function rename(item: ConfigTemplateItem) { closeDraft(); actions.startRename(item) }
function remove(item: ConfigTemplateItem) { closeDraft(); actions.requestDelete(item) }
function refreshEditors() { actions.cancelRename(); actions.cancelDelete(); void catalog.refresh() }
function dateTime(value: string) { const date = new Date(value); return Number.isNaN(date.valueOf()) ? '—' : date.toLocaleString('zh-CN', { hour12: false }) }
watch([catalog.keyword, catalog.typeFilter, catalog.page], () => { actions.cancelRename(); actions.cancelDelete(); draft.clearSelection() })
onMounted(() => { void catalog.refresh() })
</script>

<template>
  <MessageContext />
  <header class="orch-page-header"><div class="orch-page-title"><h1>模板中心</h1></div></header>
  <section class="orch-management template-workspace" aria-label="模板工作面">
    <div class="orch-table-meta"><h2>模板列表</h2><div class="orch-page-actions"><AButton :disabled="state.loading || Boolean(state.actionBusy)" :aria-busy="state.loading" @click="state.refresh"><template #icon><ReloadOutlined class="product-icon" :spin="state.loading" aria-hidden="true" /></template>{{ state.loading ? '刷新中…' : '刷新' }}</AButton></div></div>
    <div class="orch-toolbar template-filters">
      <label class="template-filter-name"><span>模板名称</span><AInput v-model:value="state.keyword" aria-label="搜索模板名称" placeholder="搜索模板名称" allow-clear :disabled="Boolean(state.actionBusy) || state.accessDenied"><template #prefix><SearchOutlined class="product-icon" aria-hidden="true" /></template></AInput></label>
      <div class="template-filter-type"><label for="template-type-filter">任务类型</label><ASelect id="template-type-filter" v-model:value="state.typeFilter" :disabled="Boolean(state.actionBusy) || state.accessDenied"><ASelectOption value="">全部类型</ASelectOption><ASelectOption value="OBDUMPER_EXPORT">导出</ASelectOption><ASelectOption value="OBLOADER_NORMAL_IMPORT">普通导入</ASelectOption></ASelect></div>
      <AButton v-if="state.hasFilters" type="text" @click="state.clearFilters">清除筛选</AButton>
    </div>
    <AAlert v-if="state.loadFailure" class="orch-feedback" type="error" :message="state.accessDenied ? '无法访问模板' : '模板读取未完成'" :description="state.loadFailure" show-icon><template #action><AButton :disabled="state.loading" @click="state.refresh">重试</AButton></template></AAlert>
    <OrchOperationalTable v-if="!state.accessDenied && (state.loading || state.hasLoaded)" :rows="state.rows" :columns="columns" row-key="id" label="模板列表" :loading="state.loading">
      <template #bodyCell="{ record: item, column }">
        <div v-if="column.key === 'name'" class="template-name"><ATooltip :title="item.displayName" :trigger="['hover', 'focus']"><strong tabindex="0">{{ item.displayName }}</strong></ATooltip><span class="template-compact">{{ taskTypeText(item.taskType) }} · {{ dateTime(item.updatedAt) }}</span></div>
        <ATag v-else-if="column.key === 'type'">{{ taskTypeText(item.taskType) }}</ATag>
        <ATooltip v-else-if="column.key === 'source' && item.sourceTaskId" :title="item.sourceTaskId" :trigger="['hover', 'focus']"><RouterLink class="template-source" :to="'/tasks/' + encodeURIComponent(item.sourceTaskId)">{{ item.sourceTaskId }}</RouterLink></ATooltip>
        <span v-else-if="column.key === 'source'">—</span>
        <ATooltip v-else-if="column.key === 'updated'" :title="item.updatedAt" :trigger="['hover', 'focus']"><span tabindex="0">{{ dateTime(item.updatedAt) }}</span></ATooltip>
        <div v-else-if="column.key === 'actions'" class="template-row-actions">
          <AButton v-if="state.allowed(item, 'CREATE_DRAFT')" type="text" class="row-action" :disabled="Boolean(state.actionBusy)" :aria-label="'使用模板 ' + item.displayName" @click="useTemplate(item)"><template #icon><FileAddOutlined class="product-icon" aria-hidden="true" /></template>使用模板</AButton>
          <ADropdown v-if="state.allowed(item, 'RENAME') || state.allowed(item, 'DELETE')" :trigger="['click']"><AButton type="text" :disabled="Boolean(state.actionBusy)" :aria-label="'更多操作 ' + item.displayName" class="orch-icon-action"><template #icon><MoreOutlined class="product-icon" aria-hidden="true" /></template></AButton><template #overlay><AMenu><AMenuItem v-if="state.allowed(item, 'RENAME')" @click="rename(item)"><EditOutlined class="product-icon" aria-hidden="true" /> 改名</AMenuItem><AMenuItem v-if="state.allowed(item, 'DELETE')" danger @click="remove(item)"><DeleteOutlined class="product-icon" aria-hidden="true" /> 删除</AMenuItem></AMenu></template></ADropdown>
          <span v-if="!item.availableActions?.length" class="template-readonly">仅可查看</span>
        </div>
      </template>
      <template #empty><span v-if="state.loading">正在加载模板…</span><EmptyState v-else-if="state.hasFilters" title="没有匹配的模板" description=""><AButton @click="state.clearFilters">清除筛选</AButton></EmptyState><EmptyState v-else title="暂无模板" description="在成功任务中保存模板后，可在这里复用。"><AButton @click="router.push('/tasks')">前往任务中心</AButton></EmptyState></template>
    </OrchOperationalTable>
    <footer v-if="state.hasLoaded && !state.accessDenied" class="orch-table-footer"><span>共 {{ state.total }} 条</span><nav class="source-pagination" aria-label="模板分页"><span>10 条/页</span><AButton type="text" class="orch-icon-action" aria-label="上一页" :disabled="state.page <= 1 || state.loading || Boolean(state.actionBusy)" @click="state.page--"><template #icon><LeftOutlined class="product-icon" aria-hidden="true" /></template></AButton><span aria-live="polite">第 {{ state.page }} 页</span><AButton type="text" class="orch-icon-action" aria-label="下一页" :disabled="state.page * 10 >= state.total || state.loading || Boolean(state.actionBusy)" @click="state.page++"><template #icon><RightOutlined class="product-icon" aria-hidden="true" /></template></AButton></nav></footer>
  </section>
  <TemplateDraftDrawer :model="binding" :busy="Boolean(state.actionBusy)" @close="closeDraft" @inspect-drafts="router.push('/tasks?tab=drafts')" />
  <TemplateRenameDialog :model="editor" :busy="Boolean(state.actionBusy)" @refresh="refreshEditors" />
  <OrchDangerConfirm :open="Boolean(editor.pendingDelete)" :title="'删除模板：' + (editor.pendingDelete?.displayName ?? '')" confirm-label="删除" destructive :confirm-disabled="editor.deleteConflict" :busy="Boolean(state.actionBusy)" @confirm="editor.confirmDelete()" @cancel="editor.cancelDelete()"><p>删除后无法再使用此模板；已创建的草稿和任务不受影响。</p><AAlert v-if="editor.deleteFailure" type="error" message="删除未完成" :description="editor.deleteFailure" show-icon><template v-if="editor.deleteConflict" #action><AButton @click="refreshEditors">刷新列表</AButton></template></AAlert></OrchDangerConfirm>
</template>

<style scoped>
.template-workspace { container: orch-workspace / inline-size; min-inline-size: 0; }
.template-filters { align-items: flex-end; }
.template-filters > :is(label, .template-filter-type) { display: grid; gap: var(--ob-foundation-space-1); min-inline-size: 0; }
.template-filter-name { flex: 1; max-inline-size: 360px; }
.template-filter-type { min-inline-size: 160px; }
.template-name { display: grid; min-inline-size: 0; }
.template-name strong, .template-source { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.template-row-actions { display: flex; align-items: center; justify-content: flex-end; white-space: nowrap; }
.template-readonly, .template-compact { color: var(--ob-color-secondary); }
.template-compact { display: none; font-size: var(--ob-component-table-metadata-size); overflow-wrap: anywhere; }
.template-source { font-family: var(--ob-foundation-font-mono); }
@container orch-workspace (max-width: 1120px) {
  .template-compact { display: block; }
  :deep(.template-type), :deep(.template-time) { display: none; }
}
@media (max-width: 640px) {
  .template-filters > :is(label, .template-filter-type) { flex: 1 1 100%; max-inline-size: none; }
}
</style>
