<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CopyPlus, Pencil, RefreshCw, Trash2 } from '@lucide/vue'

import { browserApi, dataSourceErrorMessage, exportDraftErrorMessage, taskDetailErrorMessage, type DataSourceSummary, type ExecutionNodeCandidate, type ExportConfigTemplateItem } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import WorkbenchButton from '@/components/WorkbenchButton.vue'
import { isExportEligibleDataSource } from './exportDataSourceEligibility'

const api = browserApi()
const router = useRouter()
const templates = ref<ExportConfigTemplateItem[]>([])
const loading = ref(true)
const loadFailure = ref('')
const notice = ref('')
const actionBusy = ref('')

const renameID = ref('')
const renameValue = ref('')
const draftSourceID = ref('')
const draftNodeID = ref('')
const sources = ref<DataSourceSummary[]>([])
const nodes = ref<ExecutionNodeCandidate[]>([])
const sourcesLoading = ref(false)
const createDraftFailure = ref('')

const eligibleSources = computed(() => sources.value.filter(isExportEligibleDataSource))

onMounted(() => {
  void refresh()
})

async function refresh() {
  loading.value = true
  loadFailure.value = ''
  try {
    templates.value = await api.listExportConfigTemplates()
  } catch (error) {
    loadFailure.value = taskDetailErrorMessage(error, '无法加载模板，请稍后重试。')
  } finally {
    loading.value = false
  }
}

function startRename(template: ExportConfigTemplateItem) {
  renameID.value = template.id
  renameValue.value = template.displayName
  notice.value = ''
}

async function confirmRename(template: ExportConfigTemplateItem) {
  const displayName = renameValue.value.trim()
  if (!displayName || displayName === template.displayName) {
    renameID.value = ''
    return
  }
  actionBusy.value = template.id
  try {
    const revision = await api.renameExportConfigTemplate(template.id, template.revision, displayName)
    templates.value = templates.value.map((item) => item.id === template.id ? { ...item, displayName, revision } : item)
    notice.value = `模板「${displayName}」已改名。`
  } catch (error) {
    notice.value = ''
    loadFailure.value = taskDetailErrorMessage(error, '模板改名失败。')
  } finally {
    actionBusy.value = ''
    renameID.value = ''
  }
}

async function confirmDelete(template: ExportConfigTemplateItem) {
  if (!window.confirm(`删除模板「${template.displayName}」？删除后不可恢复。`)) return
  actionBusy.value = template.id
  try {
    await api.deleteExportConfigTemplate(template.id, template.revision)
    templates.value = templates.value.filter((item) => item.id !== template.id)
    notice.value = `模板「${template.displayName}」已删除。`
  } catch (error) {
    notice.value = ''
    loadFailure.value = taskDetailErrorMessage(error, '模板删除失败。')
  } finally {
    actionBusy.value = ''
  }
}

async function loadChoices() {
  if (sources.value.length > 0 || nodes.value.length > 0) return
  sourcesLoading.value = true
  createDraftFailure.value = ''
  try {
    ;[sources.value, nodes.value] = await Promise.all([api.listDataSources(), api.listExportNodeCandidates()])
  } catch (error) {
    createDraftFailure.value = dataSourceErrorMessage(error, exportDraftErrorMessage(error, '无法加载数据源或执行节点。'))
  } finally {
    sourcesLoading.value = false
  }
}

async function createDraft(template: ExportConfigTemplateItem) {
  actionBusy.value = template.id
  createDraftFailure.value = ''
  if (!draftSourceID.value || !draftNodeID.value) {
    createDraftFailure.value = '请选择数据源与执行节点。'
    actionBusy.value = ''
    return
  }
  try {
    const draftId = await api.createDraftFromTemplate(template.id, draftSourceID.value, draftNodeID.value)
    notice.value = '草稿已创建；模板不复制凭据、预检查与风险确认，请重新完成预检查后再提交。'
    await router.push({ path: '/exports/new', query: { draft: draftId, step: '1' } })
  } catch (error) {
    createDraftFailure.value = exportDraftErrorMessage(error, '由模板创建草稿失败。')
  } finally {
    actionBusy.value = ''
  }
}

function dateTime(value: string) {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString()
}
</script>

<template>
  <section class="page-heading task-heading"><div><h1>模板中心</h1><p>模板只复用三类任务的显式、非敏感配置；由模板创建的草稿必须重新选择数据源与执行节点、重新绑定存储凭据并重新完成预检查与风险确认。</p></div></section>
  <section class="filter-bar"><WorkbenchButton :disabled="loading" @click="refresh"><template #icon><RefreshCw :size="14" :class="{ 'is-spinning': loading }" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</WorkbenchButton></section>
  <p v-if="loadFailure" class="feedback feedback-error" role="alert">{{ loadFailure }}</p>
  <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
  <section class="content-card table-card"><table><thead><tr><th>模板名称</th><th>能力版本</th><th>来源任务</th><th>更新时间</th><th class="template-actions">操作</th></tr></thead><tbody v-if="loading"><tr><td colspan="5" class="template-message">正在加载模板…</td></tr></tbody><tbody v-else-if="templates.length === 0"><tr><td colspan="5" class="template-message"><EmptyState title="尚无可展示模板" description="模板只从成功任务保存；列表不会显示凭据、节点、预检查、命令、日志或结果。" /></td></tr></tbody><tbody v-else><tr v-for="template in templates" :key="template.id"><td class="template-name"><div v-if="renameID !== template.id" class="cell-stack"><strong :title="template.displayName">{{ template.displayName }}</strong><small>{{ template.id }}</small></div><div v-else class="template-rename"><input v-model.trim="renameValue" class="tree-input" aria-label="模板名称" /><WorkbenchButton variant="primary" :disabled="Boolean(actionBusy)" @click="confirmRename(template)">保存</WorkbenchButton><WorkbenchButton :disabled="Boolean(actionBusy)" @click="renameID = ''">取消</WorkbenchButton></div></td><td><code>{{ template.capabilityVersion }}</code></td><td>{{ template.sourceTaskId || '手工创建' }}</td><td>{{ dateTime(template.updatedAt) }}</td><td class="template-row-actions"><button type="button" class="row-action" :disabled="Boolean(actionBusy)" @click="startRename(template)"><Pencil :size="13" aria-hidden="true" />改名</button><button type="button" class="row-action" :disabled="Boolean(actionBusy)" @click="loadChoices(); createDraft(template)"><CopyPlus :size="13" aria-hidden="true" />用模板新建草稿</button><button type="button" class="row-action is-danger" :disabled="Boolean(actionBusy)" @click="confirmDelete(template)"><Trash2 :size="13" aria-hidden="true" />删除</button></td></tr></tbody></table></section>
  <section v-if="templates.length > 0" class="content-card template-draft-choices"><h2>由模板创建草稿</h2><p class="section-hint">选择数据源与执行节点后点击“用模板新建草稿”；模板不复制凭据、节点、预检查或风险确认。</p><p v-if="createDraftFailure" class="feedback feedback-error" role="alert">{{ createDraftFailure }}</p><div class="template-choice-row"><label>数据源<select v-model="draftSourceID" class="tree-input tree-select" :disabled="sourcesLoading"><option value="" disabled>请选择数据源</option><option v-for="source in eligibleSources" :key="source.id" :value="source.id">{{ source.displayName }}</option></select></label><label>执行节点<select v-model="draftNodeID" class="tree-input tree-select" :disabled="sourcesLoading"><option value="" disabled>请选择执行节点</option><option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</option></select></label></div></section>
  <p class="section-hint">使用模板一定创建新的可编辑草稿，并重新选择/确认数据源、节点、路径、权限与预检查。手工新建模板暂未提供，模板只从成功任务保存。</p>
</template>

<style scoped>
.template-actions { width: 26%; text-align: right !important; }
.template-row-actions { display: flex; align-items: center; justify-content: flex-end; gap: var(--space-1); white-space: nowrap; }
.row-action { height: 30px; display: inline-flex; align-items: center; gap: var(--space-1); padding: 0 var(--space-1); border: 0; border-radius: 4px; color: #245d9e; background: transparent; font-family: inherit; font-size: 12px; font-weight: 500; line-height: 30px; cursor: pointer; }
.row-action.is-danger { color: #b42318; }
.row-action:hover:not(:disabled) { background: var(--color-primary-soft); }
.row-action.is-danger:hover:not(:disabled) { background: #fff1f0; }
.row-action:disabled { color: #99a3b0; cursor: not-allowed; }
.template-name strong { overflow: hidden; color: #1e2329; font-size: 14px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.template-name small { overflow: hidden; color: var(--color-text-tertiary); font-size: 11px; line-height: 17px; text-overflow: ellipsis; white-space: nowrap; }
.cell-stack { display: grid; align-content: center; gap: 1px; }
.template-rename { display: flex; gap: var(--space-1); align-items: center; }
.template-rename input { width: 200px; height: var(--size-control); padding: 0 8px; border: 1px solid var(--color-border-strong); border-radius: var(--radius-control); }
.template-message { height: 160px !important; text-align: center !important; }
.template-draft-choices { margin-top: var(--space-4); padding: var(--space-4); }
.template-choice-row { display: flex; flex-wrap: wrap; gap: var(--space-3); margin-top: var(--space-2); }
.template-choice-row label { display: grid; gap: var(--space-1); min-width: 240px; color: var(--color-text-secondary); font-size: 13px; }
.template-choice-row select { width: 100%; height: var(--size-control); padding: 0 8px; border: 1px solid var(--color-border-strong); border-radius: var(--radius-control); }
</style>
