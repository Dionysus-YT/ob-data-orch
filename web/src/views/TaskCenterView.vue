<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

import { browserApi, taskDetailErrorMessage, taskListPageSizes, type TaskListItem, type TaskListPageSize } from '@/api/browser'
import { taskNeedsReconciliation, taskProgressLabel, taskStageLabel, taskStateClass, taskStateLabel, taskTypeLabel } from './taskListPresentation'

const api = browserApi()
const tasks = ref<readonly TaskListItem[]>([])
const nextCursor = ref<string>()
const totalPages = ref(0)
const pageSize = ref<TaskListPageSize>(10)
const pageCursors = ref<readonly (string | undefined)[]>([undefined])
const pageIndex = ref(0)
const loading = ref(true)
const failure = ref('')
const lastUpdated = ref('')
let stopped = false

onMounted(() => {
  loading.value = false
  void loadCurrentPage()
})
onBeforeUnmount(() => { stopped = true })

async function loadCurrentPage() {
  if (stopped || loading.value) return
  loading.value = true
  failure.value = ''
  try {
    const page = await api.listTasks(pageCursors.value[pageIndex.value], pageSize.value)
    if (stopped) return
    tasks.value = page.items
    nextCursor.value = page.nextCursor
    totalPages.value = page.totalPages
    lastUpdated.value = new Date().toISOString()
  } catch (error) {
    if (!stopped) failure.value = taskDetailErrorMessage(error, '任务列表暂时无法读取。')
  } finally {
    if (!stopped) loading.value = false
  }
}

function goToPreviousPage() {
  if (loading.value || pageIndex.value === 0) return
  pageIndex.value -= 1
  void loadCurrentPage()
}

function goToNextPage() {
  const cursor = nextCursor.value
  if (loading.value || !cursor) return
  pageCursors.value = [...pageCursors.value.slice(0, pageIndex.value + 1), cursor]
  pageIndex.value += 1
  void loadCurrentPage()
}

function changePageSize(event: Event) {
  const selected = Number((event.target as HTMLSelectElement).value)
  if (selected !== 10 && selected !== 20 && selected !== 50) return
  pageSize.value = selected
  pageCursors.value = [undefined]
  pageIndex.value = 0
  nextCursor.value = undefined
  totalPages.value = 0
  void loadCurrentPage()
}

function dateTime(value: string) {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? '时间未知' : parsed.toLocaleString()
}
</script>

<template>
  <section class="page-heading task-heading">
    <div><h1>任务中心</h1><p>当前接入授权范围内的导出任务读取。列表只用于定位与进入详情，不展开命令、完整参数、错误或日志。</p><small v-if="lastUpdated">最后更新：{{ dateTime(lastUpdated) }}</small></div>
    <div class="task-create-actions">
      <button type="button" class="button button-secondary" :disabled="loading" @click="loadCurrentPage">刷新</button>
      <RouterLink class="button button-secondary" to="/exports/new">新建导出</RouterLink>
      <RouterLink class="button button-secondary" to="/imports/normal/new">新建普通导入</RouterLink>
      <RouterLink class="button button-primary" to="/imports/direct/new">新建旁路导入</RouterLink>
    </div>
  </section>

  <section class="filter-bar task-filter-bar" aria-label="任务筛选">
    <label>任务名称 / ID 关键字<input disabled placeholder="当前切片暂未开放筛选" /></label>
    <label>任务类型<select disabled><option>全部</option></select></label>
    <label>任务状态<select disabled><option>全部</option></select></label>
    <label>创建时间<input disabled placeholder="开始日期 ～ 结束日期" /></label>
    <button type="button" class="button button-secondary" disabled>展开筛选</button>
    <button type="button" class="button button-primary" disabled>查询</button>
  </section>

  <section class="scope-bar"><span>当前范围：本人创建或已按数据源明确授权的任务</span><span>仅显示授权范围内的页数；筛选能力将在后续 F3 小步骤接入。</span></section>

  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }} <button type="button" class="link-button" @click="loadCurrentPage">重试</button></p>
  <div v-if="loading && tasks.length === 0" class="content-card loading-state" role="status">正在读取授权任务…</div>

  <section v-else-if="!failure || tasks.length > 0" class="content-card table-card">
    <table>
      <thead><tr><th>任务</th><th>类型</th><th>数据源 / 对象</th><th>状态 / 阶段</th><th>进度</th><th>执行节点</th><th>创建人</th><th>时间</th><th>操作</th></tr></thead>
      <tbody>
        <tr v-if="tasks.length === 0"><td colspan="9"><div class="empty-state"><div class="empty-mark">□</div><h2>尚无可展示任务</h2><p>当前授权范围内没有已提交任务。</p><div class="task-create-actions"><RouterLink class="button button-primary" to="/exports/new">创建首个导出任务</RouterLink></div></div></td></tr>
        <tr v-for="task in tasks" v-else :key="task.id">
          <td><strong>导出任务</strong><small>{{ task.id }}</small></td>
          <td>{{ taskTypeLabel(task.type) }}</td>
          <td><strong>{{ task.dataSourceId }}</strong><small>{{ task.objectSummary || '对象摘要不可用' }}</small></td>
          <td><strong><span class="status-dot" :class="taskStateClass(task)" />{{ taskStateLabel(task) }}</strong><small>{{ taskStageLabel(task) }}</small><small v-if="taskNeedsReconciliation(task)" class="task-reconciliation">状态核对中</small></td>
          <td>{{ taskProgressLabel() }}</td>
          <td>{{ task.nodeId }}</td>
          <td>{{ task.ownedByCurrentUser ? '我' : '已授权任务' }}</td>
          <td><strong>{{ task.startedAt ? '开始' : '提交' }}：{{ dateTime(task.startedAt || task.submittedAt) }}</strong><small>更新：{{ dateTime(task.updatedAt) }}</small></td>
          <td><RouterLink class="link-button" :to="{ name: 'task-detail', params: { id: task.id } }">查看详情</RouterLink></td>
        </tr>
      </tbody>
    </table>
    <div class="task-pagination" aria-label="任务列表分页">
      <span aria-live="polite">{{ totalPages === 0 ? '暂无结果，共 0 页' : `第 ${pageIndex + 1} 页，共 ${totalPages} 页` }}</span>
      <label>每页显示
        <select :value="pageSize" :disabled="loading" @change="changePageSize">
          <option v-for="size in taskListPageSizes" :key="size" :value="size">{{ size }} 条</option>
        </select>
      </label>
      <div>
        <button type="button" class="button button-secondary" :disabled="loading || pageIndex === 0" @click="goToPreviousPage">上一页</button>
        <button type="button" class="button button-secondary" :disabled="loading || !nextCursor" @click="goToNextPage">下一页</button>
      </div>
    </div>
  </section>
  <p class="section-hint">当前列表只包含已提交任务。未知状态或需要重新核对的事实显示“状态核对中”；没有可靠工具证据时不展示百分比、ETA 或推测阶段。</p>
</template>

<style scoped>
.task-pagination { display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding: 14px 0 2px; color: #607187; font-size: 13px; }
.task-pagination label, .task-pagination > div { display: flex; align-items: center; gap: 8px; }
.task-pagination select { min-width: 84px; height: 32px; padding: 4px 8px; border: 1px solid #dbe3ed; border-radius: 4px; color: #405066; background: #fff; }
@media (max-width: 620px) { .task-pagination { align-items: flex-start; flex-direction: column; } }
</style>
