<script setup lang="ts">
import { CheckCircleOutlined, LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Select as ASelect, SelectOption as ASelectOption, Skeleton as ASkeleton, Tag as ATag, Tooltip as ATooltip, TypographyText as ATypographyText, type TableColumnsType } from 'ant-design-vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { taskListPageSizes, type TaskListAction, type TaskListItem, type TaskListPageSize } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { environmentTagTheme, managementTheme } from '@/platform/theme'
import TaskListFilters from '@/workbench/tasks/TaskListFilters.vue'
import TaskActionDialogs from '@/workbench/tasks/TaskActionDialogs.vue'
import TaskActionMenu from '@/workbench/tasks/TaskActionMenu.vue'
import TaskDraftList from '@/workbench/tasks/TaskDraftList.vue'
import { Tabs as ATabs, TabPane as ATabPane } from 'ant-design-vue'
import { taskHistoricalName, taskMoreActions, taskNeedsReconciliation, taskRowPrimary, taskStageLabel, taskStateBadgeStatus, taskStateLabel, taskTypeLabel } from '@/workbench/tasks/taskListPresentation'
import { hasTaskListConditions } from '@/workbench/tasks/taskListQuery'
import { taskDateTimeText, taskDurationText, taskEnvironmentLabel } from '@/workbench/tasks/taskStatePresentation'
import { useTaskList } from '@/workbench/tasks/useTaskList'
import { useTaskListActions } from '@/workbench/tasks/useTaskListActions'

const route = useRoute()
const router = useRouter()
const activeView = computed(() => route.query.tab === 'drafts' ? 'drafts' : 'submitted')
const replaceDraftQuery = (query: import('vue-router').LocationQueryRaw) => router.replace({ query })
const navigateDraft = (location: import('vue-router').RouteLocationRaw) => router.push(location)
function switchView(key: string | number) { const query = { ...route.query }; if (key === 'drafts') query.tab = 'drafts'; else delete query.tab; void router.replace({ query }) }
const list = useTaskList(computed(() => route.query), query => router.replace({ query }), { enabled: computed(() => activeView.value === 'submitted') })
const { filters, tasks, totalPages, pageIndex, nextCursor, loading, refreshing, refreshCompleted, restricted, failure, dateRangeError, lastLoadedAt, filterOptions } = list
const actions = useTaskListActions({ navigate: location => router.push(location), beginMutation: list.beginMutation, endMutation: list.endMutation })
const actionModel = reactive(actions)
const { busyTaskId, failure: actionFailure, notice } = actions
const { MessageContext } = useOperationMessage(notice)
const hasConditions = computed(() => hasTaskListConditions(filters.value))

// 运行中耗时按秒更新显示；只改变本地展示，不触发读取。
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined
onMounted(() => { clock = setInterval(() => { now.value = Date.now() }, 1000) })
onBeforeUnmount(() => clearInterval(clock))
// 路由对象变化时关闭旧确认，旧任务的确认不能在新条件下执行。
watch(() => route.fullPath, () => { actions.invalidate() }, { flush: 'sync' })
watch(restricted, value => { if (value) actions.invalidate() }, { flush: 'sync' })

const middleCell = () => ({ style: { verticalAlign: 'middle' } })
const columns: TableColumnsType<TaskListItem> = [
  { key: 'task', title: '任务', width: 230, customCell: middleCell },
  { key: 'type', title: '类型', width: 80, align: 'center', customCell: middleCell },
  { key: 'source', title: '数据源 / 对象', width: 190, customCell: middleCell },
  { key: 'state', title: '状态 / 阶段', width: 130, align: 'center', customCell: middleCell },
  { key: 'progress', title: '进度', width: 90, align: 'center', customCell: middleCell },
  { key: 'node', title: '执行节点', width: 140, customCell: middleCell },
  { key: 'creator', title: '创建人', width: 110, customCell: middleCell },
  { key: 'time', title: '时间', width: 170, align: 'center', customCell: middleCell },
  { key: 'actions', title: '操作', width: 110, align: 'center', customCell: middleCell },
]

function openPrimary(task: TaskListItem) {
  void router.push({ name: 'task-detail', params: { id: task.id }, query: { tab: 'overview' } })
}
function rowAction(task: TaskListItem, action: string) { actions.request(task, action as TaskListAction) }
</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <MessageContext />
    <div class="orch-ui task-workspace">
      <header class="orch-page-header task-heading">
        <div class="orch-page-title"><h1>任务中心</h1><span class="orch-context-label">导入 / 导出</span></div>
      </header>
      <p class="task-intro">定位授权范围内的任务并进入详情排障，列表不展开命令、完整参数或日志。</p>

      <AAlert v-if="failure && tasks.length" class="orch-feedback" type="error" message="刷新未完成" :description="`${failure} 当前仍展示上一次加载的任务。`" show-icon><template #action><AButton @click="list.refresh()">重试</AButton></template></AAlert>
      <AAlert v-if="actionFailure" class="orch-feedback" type="error" message="操作未完成" :description="actionFailure" show-icon />

      <ATabs :active-key="activeView" :destroy-inactive-tab-pane="true" @change="switchView">
        <ATabPane key="submitted" tab="已提交任务">
          <section v-if="activeView === 'submitted'" class="orch-management" aria-label="任务列表">
            <div class="orch-table-meta">
              <h2>任务列表</h2>
              <div class="task-page-actions">
                <span v-if="lastLoadedAt" class="task-updated">最后更新：{{ lastLoadedAt }}</span>
                <AButton type="text" :loading="refreshing" :disabled="loading || refreshing" :aria-busy="refreshing" aria-label="刷新" @click="list.refresh()"><template #icon><CheckCircleOutlined v-if="refreshCompleted" aria-hidden="true" /><ReloadOutlined v-else aria-hidden="true" /></template>{{ refreshing ? '刷新中' : refreshCompleted ? '已刷新' : '刷新' }}</AButton>
                <TaskActionMenu v-if="!restricted" :actions="['CREATE_EXPORT', 'CREATE_NORMAL_IMPORT', 'CREATE_DIRECT_LOAD']" label="新建任务" :session-key="route.fullPath" creation @select="action => action === 'CREATE_EXPORT' ? router.push('/exports/new') : action === 'CREATE_NORMAL_IMPORT' && router.push('/imports/normal/new')" />
              </div>
            </div>

            <TaskListFilters :filters="filters" :options="filterOptions" :disabled="restricted" :date-range-error="dateRangeError" @update="list.updateFilters" @clear="list.clearFilters" @expand="list.loadFilterOptions" />

            <ASkeleton v-if="loading && !tasks.length" active :paragraph="{ rows: 6 }" class="task-loading" role="status" aria-label="正在读取授权任务" />
            <AAlert v-else-if="failure && !tasks.length" class="task-state" :type="restricted ? 'warning' : 'error'" :message="restricted ? '当前身份无权查看任务' : '无法加载任务列表'" :description="restricted ? '请联系管理员确认任务查看权限。' : failure" show-icon><template #action><AButton v-if="!restricted" @click="list.refresh()">重试</AButton></template></AAlert>
            <template v-else-if="!dateRangeError">
              <OrchOperationalTable :rows="tasks" :columns="columns" :loading="loading || refreshing" row-key="id" :minimum-width="1250" :label="hasConditions ? '符合当前筛选条件的授权任务' : '当前授权范围内的任务'">
                <template #bodyCell="{ record: task, column }">
                  <template v-if="column.key === 'task'">
                    <ATooltip :title="task.displayName"><RouterLink class="task-name" :to="{ name: 'task-detail', params: { id: task.id } }">{{ task.displayName }}</RouterLink></ATooltip>
                    <span class="task-secondary"><ATypographyText class="task-id" :copyable="{ text: task.id, tooltip: true }">{{ task.id }}</ATypographyText><ConfigProvider v-if="task.environment" :theme="environmentTagTheme(task.environment)"><ATag :bordered="false">{{ taskEnvironmentLabel(task.environment) }}</ATag></ConfigProvider><span class="task-inline-type">{{ taskTypeLabel(task.type) }}</span></span>
                  </template>
                  <template v-else-if="column.key === 'type'"><span class="task-type-cell">{{ taskTypeLabel(task.type) }}</span></template>
                  <template v-else-if="column.key === 'source'">
                    <span class="task-primary">{{ taskHistoricalName(task.dataSourceName, task.dataSourceId, task.dataSourceDeleted) }}</span>
                    <ATooltip v-if="task.objectSummary" :title="task.objectSummary"><span class="task-secondary task-ellipsis">{{ task.objectSummary }}</span></ATooltip>
                  </template>
                  <template v-else-if="column.key === 'state'">
                    <ABadge :status="taskStateBadgeStatus(task)" :text="taskStateLabel(task)" />
                    <span v-if="taskStageLabel()" class="task-secondary">{{ taskStageLabel() }}</span>
                    <ATag v-if="taskNeedsReconciliation(task)" color="warning" class="task-flag">状态核对中</ATag>
                    <ATag v-else-if="task.cancelRequested && task.state !== 'CANCELLING'" class="task-flag">取消请求中</ATag>
                  </template>
                  <template v-else-if="column.key === 'progress'"><ATooltip title="工具未提供可靠进度"><span class="task-muted" aria-label="工具未提供可靠进度">—</span></ATooltip></template>
                  <template v-else-if="column.key === 'node'"><span class="task-primary">{{ taskHistoricalName(task.nodeName, task.nodeId, task.nodeDeleted) }}</span></template>
                  <template v-else-if="column.key === 'creator'"><span class="task-primary">{{ task.creatorName || '—' }}<template v-if="task.ownedByCurrentUser">（我）</template></span></template>
                  <template v-else-if="column.key === 'time'">
                    <template v-if="task.startedAt"><span class="task-primary">开始 {{ taskDateTimeText(task.startedAt) }}</span><span class="task-secondary">耗时 {{ taskDurationText(task.startedAt, task.finishedAt, now) || '—' }}</span></template>
                    <span v-else class="task-primary">提交 {{ taskDateTimeText(task.submittedAt) }}</span>
                  </template>
                  <template v-else-if="column.key === 'actions'">
                    <div class="task-row-actions">
                      <AButton type="link" size="small" :disabled="busyTaskId === task.id" @click="openPrimary(task)">{{ taskRowPrimary(task) === 'VIEW_ERROR' ? '查看错误' : '查看详情' }}</AButton>
                      <TaskActionMenu v-if="taskMoreActions(task).length" :actions="taskMoreActions(task)" :label="`${task.displayName} 的更多操作`" :busy="Boolean(busyTaskId)" @select="action => rowAction(task, action)" />
                    </div>
                  </template>
                </template>
                <template #empty>
                  <EmptyState v-if="hasConditions" title="没有符合条件的任务" description="调整或清除筛选条件后重试。" action="清除筛选" @action="list.clearFilters()" />
                  <EmptyState v-else title="尚无可展示任务" description="当前授权范围内没有已提交任务。"><RouterLink v-slot="{ href, navigate }" to="/exports/new" custom><AButton :href="href" type="primary" @click="navigate">新建导出任务</AButton></RouterLink></EmptyState>
                </template>
              </OrchOperationalTable>
              <footer class="orch-table-footer task-footer">
                <span>{{ totalPages === 0 ? '共 0 页' : `共 ${totalPages} 页` }}</span>
                <nav class="task-pagination" aria-label="任务列表分页">
                  <!-- 包裹 label 使 Ant Select 内部 combobox 获得可访问名称。 -->
                  <label><span class="orch-sr-only">每页显示</span><ASelect :value="filters.limit" aria-label="每页显示" class="task-page-size" :disabled="loading || refreshing" @change="value => list.changePageSize(Number(value) as TaskListPageSize)">
                    <ASelectOption v-for="size in taskListPageSizes" :key="size" :value="size">{{ size }} 条/页</ASelectOption>
                  </ASelect></label>
                  <AButton type="text" class="orch-icon-action" aria-label="上一页" :disabled="loading || refreshing || pageIndex === 0" @click="list.goToPreviousPage()"><template #icon><LeftOutlined class="product-icon" aria-hidden="true" /></template></AButton>
                  <span aria-live="polite">第 {{ pageIndex + 1 }} 页</span>
                  <AButton type="text" class="orch-icon-action" aria-label="下一页" :disabled="loading || refreshing || !nextCursor" @click="list.goToNextPage()"><template #icon><RightOutlined class="product-icon" aria-hidden="true" /></template></AButton>
                </nav>
              </footer>
            </template>
          </section>
        </ATabPane>
        <ATabPane key="drafts" tab="我的草稿"><TaskDraftList v-if="activeView === 'drafts'" :query="route.query" :replace-query="replaceDraftQuery" :navigate="navigateDraft" /></ATabPane>
      </ATabs>
    </div>

    <TaskActionDialogs :model="actionModel" />
  </ConfigProvider>
</template>

<style scoped>
.task-workspace { min-width: 0; }
.task-intro { margin: 0 0 var(--ob-foundation-space-4); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.task-page-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; align-items: center; gap: var(--ob-foundation-space-2); }
.task-updated { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.task-loading, .task-state { margin: var(--ob-foundation-space-6); }
.task-name { display: block; max-width: 100%; overflow: hidden; color: var(--ob-color-primary); font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.task-primary { display: block; min-inline-size: 0; overflow-wrap: anywhere; }
.task-secondary { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-1); margin-top: var(--ob-foundation-space-1); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); line-height: var(--ob-component-table-metadata-line-height); overflow-wrap: anywhere; }
.task-ellipsis { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.task-id { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.task-secondary :deep(.ant-tag) { margin: 0; }
.task-inline-type { display: none; }
.task-flag { display: block; width: fit-content; margin: var(--ob-foundation-space-1) auto 0; }
.task-muted { color: var(--ob-color-muted); }
.task-row-actions { display: flex; justify-content: center; align-items: center; gap: var(--ob-foundation-space-1); }
.task-footer { justify-content: space-between; }
.task-pagination { display: flex; align-items: center; gap: var(--ob-foundation-space-2); margin-left: auto; }
.task-page-size { width: 110px; }
.task-confirm-facts { display: grid; gap: var(--ob-foundation-space-1); margin: 0 0 var(--ob-foundation-space-3); }
.task-confirm-facts div { display: grid; grid-template-columns: 88px minmax(0, 1fr); }
.task-confirm-facts dt { color: var(--ob-color-secondary); }
.task-confirm-facts dd { margin: 0; overflow-wrap: anywhere; }
.task-confirm-note { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.task-template-name { display: grid; gap: var(--ob-foundation-space-1); }
/* 工作区收窄时类型并入任务次行；表格内部横滚，状态与操作列保持可达。 */
@container orch-workspace (max-width: 1120px) { .task-inline-type { display: inline; } .task-type-cell { color: var(--ob-color-muted); } }
@media (max-width: 620px) { .task-footer { align-items: flex-start; flex-direction: column; } }
</style>
