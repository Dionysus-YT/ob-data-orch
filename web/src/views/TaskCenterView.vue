<script setup lang="ts">
import { UnorderedListOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
const columns = [{"key":"c0","title":"任务"},{"key":"c1","title":"类型"},{"key":"c2","title":"数据源 / 对象"},{"key":"c3","title":"状态 / 阶段"},{"key":"c4","title":"进度"},{"key":"c5","title":"执行节点"},{"key":"c6","title":"创建人"},{"key":"c7","title":"时间"},{"key":"c8","title":"操作"}]

import { Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
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

function changePageSize(value: unknown) {
  const selected = Number(value)
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
      <AButton :disabled="loading" @click="loadCurrentPage">刷新</AButton>
      <RouterLink v-slot="{ href, navigate }" to="/exports/new" custom><AButton :href="href" @click="navigate">新建导出</AButton></RouterLink>
      <RouterLink v-slot="{ href, navigate }" to="/imports/normal/new" custom><AButton :href="href" @click="navigate">新建普通导入</AButton></RouterLink>
      <RouterLink v-slot="{ href, navigate }" to="/imports/direct/new" custom><AButton :href="href" type="primary" @click="navigate">新建旁路导入</AButton></RouterLink>
    </div>
  </section>

  <section class="filter-bar task-filter-bar" aria-label="任务筛选">
    <label>任务名称 / ID 关键字<AInput aria-label="任务名称 / ID 关键字" disabled placeholder="筛选暂不可用" /></label>
    <label>任务类型<ASelect default-value="全部" aria-label="任务类型" disabled><ASelectOption value="全部">全部</ASelectOption></ASelect></label>
    <label>任务状态<ASelect default-value="全部" aria-label="任务状态" disabled><ASelectOption value="全部">全部</ASelectOption></ASelect></label>
    <label>创建时间<AInput aria-label="创建时间" disabled placeholder="开始日期 ～ 结束日期" /></label>
    <AButton disabled>展开筛选</AButton>
    <AButton disabled type="primary">查询</AButton>
  </section>

  <section class="scope-bar"><span>当前范围：本人创建或已按数据源明确授权的任务</span><span>当前仅支持分页浏览，搜索和筛选暂不可用。</span></section>

  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }} <AButton type="text" @click="loadCurrentPage">重试</AButton></p>
  <div v-if="loading && tasks.length === 0" class="content-card loading-state" role="status">正在读取授权任务…</div>

  <section v-else-if="!failure || tasks.length > 0" class="content-card table-card">
    <OrchOperationalTable :rows="tasks" :columns="columns" row-key="id" label="任务列表" :loading="loading">
      <template #bodyCell="{ record: task, column }">
        <div v-if="column.key === 'c0'"><strong>导出任务</strong><small>{{ task.id }}</small></div>
        <div v-else-if="column.key === 'c1'">{{ taskTypeLabel(task.type) }}</div>
        <div v-else-if="column.key === 'c2'"><strong>{{ task.dataSourceId }}</strong><small>{{ task.objectSummary || '对象摘要不可用' }}</small></div>
        <div v-else-if="column.key === 'c3'"><strong><span class="status-dot" :class="taskStateClass(task)" />{{ taskStateLabel(task) }}</strong><small>{{ taskStageLabel(task) }}</small><small v-if="taskNeedsReconciliation(task)" class="task-reconciliation">状态核对中</small></div>
        <div v-else-if="column.key === 'c4'">{{ taskProgressLabel() }}</div>
        <div v-else-if="column.key === 'c5'">{{ task.nodeId }}</div>
        <div v-else-if="column.key === 'c6'">{{ task.ownedByCurrentUser ? '我' : '已授权任务' }}</div>
        <div v-else-if="column.key === 'c7'"><strong>{{ task.startedAt ? '开始' : '提交' }}：{{ dateTime(task.startedAt || task.submittedAt) }}</strong><small>更新：{{ dateTime(task.updatedAt) }}</small></div>
        <div v-else-if="column.key === 'c8'"><RouterLink class="link-button" :to="{ name: 'task-detail', params: { id: task.id } }">查看详情</RouterLink></div>
      </template>
      <template #empty><div v-if="tasks.length === 0"><div class="empty-state"><div class="empty-mark"><UnorderedListOutlined class="product-icon" aria-hidden="true" /></div><h2>尚无可展示任务</h2><p>当前授权范围内没有已提交任务。</p><div class="task-create-actions"><RouterLink v-slot="{ href, navigate }" to="/exports/new" custom><AButton :href="href" type="primary" @click="navigate">创建首个导出任务</AButton></RouterLink></div></div></div></template>
    </OrchOperationalTable>
    <div class="task-pagination" aria-label="任务列表分页">
      <span aria-live="polite">{{ totalPages === 0 ? '暂无结果，共 0 页' : `第 ${pageIndex + 1} 页，共 ${totalPages} 页` }}</span>
      <label>每页显示
        <ASelect aria-label="每页显示" :value="pageSize" :disabled="loading" @change="changePageSize">
          <ASelectOption v-for="size in taskListPageSizes" :key="size" :value="size">{{ size }} 条</ASelectOption>
        </ASelect>
      </label>
      <div>
        <AButton :disabled="loading || pageIndex === 0" @click="goToPreviousPage">上一页</AButton>
        <AButton :disabled="loading || !nextCursor" @click="goToNextPage">下一页</AButton>
      </div>
    </div>
  </section>
  <p class="section-hint">当前列表只包含已提交任务。未知状态或需要重新核对的事实显示“状态核对中”；没有可靠工具证据时不展示百分比、ETA 或推测阶段。</p>
</template>

<style scoped>
.task-pagination { display: flex; align-items: center; justify-content: flex-end; gap: var(--ob-foundation-space-3); padding: var(--ob-foundation-space-3) 0 2px; color: var(--ob-color-secondary); font-size: var(--ob-component-field-label-size); }
.task-pagination label, .task-pagination > div { display: flex; align-items: center; gap: var(--ob-foundation-space-2); }
@media (max-width: 620px) { .task-pagination { align-items: flex-start; flex-direction: column; } }
</style>
