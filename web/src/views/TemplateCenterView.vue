<script setup lang="ts">
import { Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { FileAddOutlined, EditOutlined, ReloadOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import EmptyState from '@/components/EmptyState.vue'
import { useTemplateCatalog } from '@/workbench/templates/useTemplateCatalog'
import { useTemplateActions } from '@/workbench/templates/useTemplateActions'
import { useTemplateDraft } from '@/workbench/templates/useTemplateDraft'

const columns = [{ key: 'c0', title: '模板名称' }, { key: 'c1', title: '能力版本' }, { key: 'c2', title: '来源任务' }, { key: 'c3', title: '更新时间' }, { key: 'c4', title: '操作', width: '26%' }]
const router = useRouter()
const catalog = useTemplateCatalog()
const { templates, loading, loadFailure, notice, actionBusy, refresh } = catalog
const { pendingDelete, renameID, renameValue, startRename, confirmRename, requestDelete, confirmDelete, cancelRename, cancelDelete } = useTemplateActions(catalog)
const { draftSourceID, draftNodeID, sourcesLoading, createDraftFailure, eligibleSources, nodes, loadChoices, createDraft } = useTemplateDraft(catalog,
  id => router.push({ path: '/exports/new', query: { draft: id, step: '1' } }),
)
onMounted(() => { void refresh() })
function dateTime(value: string) {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString()
}
</script>

<template>
  <section class="page-heading task-heading"><div><h1>模板中心</h1><p>模板只复用三类任务的显式、非敏感配置；由模板创建的草稿必须重新选择数据源与执行节点、重新绑定存储凭据并重新完成预检查与风险确认。</p></div></section>
  <section class="filter-bar"><AButton :disabled="loading" @click="refresh"><template #icon><ReloadOutlined class="product-icon" :spin="loading" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</AButton></section>
  <p v-if="loadFailure" class="feedback feedback-error" role="alert">{{ loadFailure }}</p>
  <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
  <section class="content-card table-card">
    <OrchOperationalTable :rows="templates" :columns="columns" row-key="id" label="模板列表" :loading="loading">
      <template #bodyCell="{ record: template, column }">
        <div v-if="column.key === 'c0'" class="template-name"><div v-if="renameID !== template.id" class="cell-stack"><strong :title="template.displayName">{{ template.displayName }}</strong><small>{{ template.id }}</small></div><div v-else class="template-rename"><AInput v-model:value.trim="renameValue" aria-label="模板名称" /><AButton :disabled="Boolean(actionBusy)" type="primary" @click="confirmRename(template)">保存</AButton><AButton :disabled="Boolean(actionBusy)" @click="cancelRename">取消</AButton></div></div>
        <div v-else-if="column.key === 'c1'"><code>{{ template.capabilityVersion }}</code></div>
        <div v-else-if="column.key === 'c2'">{{ template.sourceTaskId || '手工创建' }}</div>
        <div v-else-if="column.key === 'c3'">{{ dateTime(template.updatedAt) }}</div>
        <div v-else-if="column.key === 'c4'" class="template-row-actions"><AButton :disabled="Boolean(actionBusy)" type="text" class="row-action" @click="startRename(template)"><template #icon><EditOutlined class="product-icon" aria-hidden="true" /></template>改名</AButton><AButton :disabled="Boolean(actionBusy)" type="text" class="row-action" @click="loadChoices(); createDraft(template)"><template #icon><FileAddOutlined class="product-icon" aria-hidden="true" /></template>用模板新建草稿</AButton><AButton :disabled="Boolean(actionBusy)" type="text" danger class="row-action is-danger" @click="requestDelete(template)"><template #icon><DeleteOutlined class="product-icon" aria-hidden="true" /></template>删除</AButton></div>
      </template>
      <template #empty><div v-if="loading">正在加载模板…</div><div v-else-if="templates.length === 0"><EmptyState title="尚无可展示模板" description="模板只从成功任务保存；列表不会显示凭据、节点、预检查、命令、日志或结果。" /></div></template>
    </OrchOperationalTable>
  </section>
  <section v-if="templates.length > 0" class="content-card template-draft-choices"><h2>由模板创建草稿</h2><p class="section-hint">选择数据源与执行节点后点击“用模板新建草稿”；模板不复制凭据、节点、预检查或风险确认。</p><p v-if="createDraftFailure" class="feedback feedback-error" role="alert">{{ createDraftFailure }}</p><div class="template-choice-row"><label>数据源<ASelect v-model:value="draftSourceID" aria-label="数据源" :disabled="sourcesLoading"><ASelectOption value="" disabled>请选择数据源</ASelectOption><ASelectOption v-for="source in eligibleSources" :key="source.id" :value="source.id">{{ source.displayName }}</ASelectOption></ASelect></label><label>执行节点<ASelect v-model:value="draftNodeID" aria-label="执行节点" :disabled="sourcesLoading"><ASelectOption value="" disabled>请选择执行节点</ASelectOption><ASelectOption v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</ASelectOption></ASelect></label></div></section>
  <p class="section-hint">使用模板一定创建新的可编辑草稿，并重新选择/确认数据源、节点、路径、权限与预检查。手工新建模板暂未提供，模板只从成功任务保存。</p>
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
