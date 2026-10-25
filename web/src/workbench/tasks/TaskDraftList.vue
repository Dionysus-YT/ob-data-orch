<script setup lang="ts">
import { LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, Input as AInput, Select as ASelect, Skeleton as ASkeleton, Tooltip as ATooltip, type TableColumnsType } from 'ant-design-vue'
import { onScopeDispose, ref, toRef, watch } from 'vue'
import type { LocationQuery, LocationQueryRaw, RouteLocationRaw } from 'vue-router'
import { taskListPageSizes, type TaskDraftListItem, type TaskListPageSize } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import TaskActionMenu from './TaskActionMenu.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { taskDateTimeText, taskTypeText } from './taskStatePresentation'
import { useTaskDraftList } from './useTaskDraftList'

const props = defineProps<{ query: LocationQuery; replaceQuery: (query: LocationQueryRaw) => unknown; navigate: (location: RouteLocationRaw) => unknown }>()
const drafts = useTaskDraftList(toRef(props, 'query'), props.replaceQuery, props.navigate)
const { items, totalPages, pageIndex, nextCursor, loading, refreshing, restricted, failure, busyId, confirmation } = drafts
const { MessageContext } = useOperationMessage(drafts.notice)
const keywordInput = ref(drafts.keyword.value)
let debounce: ReturnType<typeof setTimeout> | undefined
watch(drafts.keyword, value => { keywordInput.value = value }, { flush: 'sync' })
function updateKeyword(value: string) { clearTimeout(debounce); keywordInput.value = value; debounce = setTimeout(() => drafts.updateFilters({ keyword: value.trim() }), 300) }
onScopeDispose(() => clearTimeout(debounce))
const columns: TableColumnsType<TaskDraftListItem> = [{ key: 'draft', title: '草稿', width: 280 }, { key: 'source', title: '数据源 / 执行节点', width: 240 }, { key: 'state', title: '状态', width: 80, align: 'center' }, { key: 'time', title: '时间', width: 180 }, { key: 'actions', title: '操作', width: 130, align: 'center' }]
</script>

<template>
  <MessageContext />
  <section class="orch-management" aria-label="我的草稿">
    <div class="orch-table-meta draft-meta"><h2>我的草稿</h2><div class="draft-actions"><AButton type="text" :loading="refreshing" :disabled="loading || Boolean(busyId)" @click="drafts.load(true)"><template #icon><ReloadOutlined /></template>刷新草稿</AButton><TaskActionMenu :actions="['CREATE_EXPORT', 'CREATE_NORMAL_IMPORT', 'CREATE_DIRECT_LOAD']" label="新建任务" creation @select="action => navigate(action === 'CREATE_DIRECT_LOAD' ? '/imports/direct/new' : action === 'CREATE_NORMAL_IMPORT' ? '/imports/normal/new' : '/exports/new')" /></div></div>
    <div class="draft-filters"><AInput :value="keywordInput" aria-label="草稿名称 / ID 关键字" placeholder="草稿名称 / ID" allow-clear :maxlength="128" class="draft-keyword" @update:value="value => updateKeyword(String(value))" /><p>只显示当前账号的未提交草稿；提交执行将进入向导重新预检查。</p></div>
    <AAlert v-if="failure" class="orch-feedback" :type="restricted ? 'warning' : 'error'" :message="restricted ? '当前身份无法读取草稿' : '操作未完成'" :description="failure" show-icon><template #action><AButton v-if="!restricted" @click="drafts.load(true)">重试</AButton></template></AAlert>
    <ASkeleton v-if="loading && !items.length" active :paragraph="{ rows: 5 }" class="draft-loading" />
    <template v-else-if="!restricted">
      <OrchOperationalTable :rows="items" :columns="columns" row-key="id" :minimum-width="910" label="当前账号的未提交草稿" :loading="loading || refreshing">
        <template #bodyCell="{ record: item, column }">
          <template v-if="column.key === 'draft'"><ATooltip :title="item.displayName"><AButton type="link" class="draft-name" :disabled="!item.availableActions.includes('CONTINUE_EDIT')" @click="drafts.open(item)">{{ item.displayName }}</AButton></ATooltip><span class="draft-secondary"><code>{{ item.id }}</code> · {{ taskTypeText(item.type) }} · 修订 {{ item.revision }}</span><span v-if="item.sourceTaskId" class="draft-secondary">来源任务 {{ item.sourceTaskId }}</span></template>
          <template v-else-if="column.key === 'source'"><span>{{ item.dataSourceId }}</span><span class="draft-secondary">{{ item.nodeId || '未选择执行节点' }}</span></template>
          <template v-else-if="column.key === 'state'"><ABadge status="default" text="草稿" /></template>
          <template v-else-if="column.key === 'time'"><span>更新 {{ taskDateTimeText(item.updatedAt) }}</span><span class="draft-secondary">创建 {{ taskDateTimeText(item.createdAt) }}</span></template>
          <template v-else-if="column.key === 'actions'"><div class="draft-actions"><AButton v-if="item.availableActions.includes('CONTINUE_EDIT')" type="link" size="small" :disabled="Boolean(busyId)" @click="drafts.open(item)">继续编辑</AButton><TaskActionMenu :actions="item.availableActions.filter(action => action !== 'CONTINUE_EDIT')" :label="`${item.displayName} 草稿的更多操作`" :busy="Boolean(busyId)" @select="action => { if (action === 'SUBMIT') drafts.open(item, true); if (action === 'DELETE') drafts.requestDelete(item) }" /></div></template>
        </template>
        <template #empty><EmptyState :title="drafts.keyword.value ? '没有符合条件的草稿' : '尚无未提交草稿'" :description="drafts.keyword.value ? '调整关键字后重试。' : '在导出或普通导入向导保存后，可在这里继续编辑。'" /></template>
      </OrchOperationalTable>
      <footer class="orch-table-footer draft-footer"><span>共 {{ totalPages }} 页</span><nav class="draft-actions" aria-label="草稿列表分页"><label><span class="orch-sr-only">草稿每页显示</span><ASelect :value="drafts.limit.value" class="draft-page-size" :options="taskListPageSizes.map(value => ({ value, label: `${value} 条/页` }))" :disabled="loading || refreshing || Boolean(busyId)" aria-label="草稿每页显示" @change="value => drafts.updateFilters({ limit: Number(value) as TaskListPageSize })" /></label><AButton type="text" aria-label="草稿上一页" :disabled="loading || refreshing || Boolean(busyId) || !pageIndex" @click="drafts.previous()"><template #icon><LeftOutlined /></template></AButton><span>第 {{ pageIndex + 1 }} 页</span><AButton type="text" aria-label="草稿下一页" :disabled="loading || refreshing || Boolean(busyId) || !nextCursor" @click="drafts.next()"><template #icon><RightOutlined /></template></AButton></nav></footer>
    </template>
  </section>
  <OrchDangerConfirm :open="Boolean(confirmation)" title="删除草稿" confirm-label="删除草稿" destructive :busy="Boolean(busyId)" @confirm="drafts.confirmDelete()" @cancel="drafts.dismiss()"><p>确认删除 {{ confirmation?.displayName }}（{{ confirmation?.id }}）？</p><p>删除后不再提供继续编辑或提交入口；历史预检查与审计引用保留。已提交任务和正在执行预检查的草稿不能删除。</p><AAlert v-if="failure" type="error" message="删除未完成" :description="failure" show-icon /></OrchDangerConfirm>
</template>

<style scoped>
.draft-actions, .draft-filters { display: flex; align-items: center; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.draft-meta { flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.draft-filters { flex-wrap: wrap; padding: 0 var(--ob-foundation-space-4) var(--ob-foundation-space-4); }
.draft-filters p { margin: 0; color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.draft-keyword { width: 260px; max-width: 100%; }
.draft-name { display: block; max-width: 100%; height: auto; padding: 0; overflow: hidden; text-overflow: ellipsis; text-align: left; }
.draft-secondary { display: block; margin-top: var(--ob-foundation-space-1); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); overflow-wrap: anywhere; }
.draft-footer { justify-content: space-between; }
.draft-page-size { width: 110px; }
.draft-loading { padding: var(--ob-foundation-space-6); }
@media (max-width: 620px) { .draft-footer { flex-direction: column; align-items: flex-start; } }
</style>
