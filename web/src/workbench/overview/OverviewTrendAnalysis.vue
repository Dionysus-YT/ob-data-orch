<script setup lang="ts">
import { LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, Empty as AEmpty, RadioButton as ARadioButton, RadioGroup as ARadioGroup, Select as ASelect, Skeleton as ASkeleton, Table as ATable, Tag as ATag } from 'ant-design-vue'
import { computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { TaskListItem } from '@/api/browser'
import { taskStateBadge, taskStateText, taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import { overviewBucketLabel, overviewTime } from './overviewPresentation'
import { overviewTableComponents } from './overviewTable'
import type { OverviewTrendSession } from './overviewTrend'
import { useOverviewAnalysis } from './useOverviewAnalysis'

const props = defineProps<{ session: OverviewTrendSession }>()
const emit = defineEmits<{ 'authorization-lost': [] }>()
const router = useRouter()
const { selectedFrom, state, query, valid, list } = useOverviewAnalysis(props.session)
const { tasks, totalPages, pageIndex, nextCursor, loading, refreshing, restricted, failure } = list
const busy = computed(() => loading.value || refreshing.value)
const options = computed(() => [{ value: '', label: '全部时间段' }, ...props.session.buckets.map(bucket => ({ value: bucket.from, label: overviewBucketLabel(bucket, props.session.range) }))])
const tableComponents = overviewTableComponents('趋势下钻任务列表')
const columns = [{ title: '任务', key: 'task', width: '32%' }, { title: '状态', key: 'state' }, { title: '数据源 / 节点', key: 'objects' }, { title: '终态时间（UTC）', key: 'finishedAt' }]
const taskOf = (record: Record<string, unknown>) => record as unknown as TaskListItem
// 新读取已经确认授权失效时通知快照所有者清空全页，不能只清弹窗后留下旧指标。
watch(restricted, value => { if (value) emit('authorization-lost') }, { flush: 'sync' })
</script>

<template>
  <div class="overview-drilldown">
    <div class="overview-drilldown-controls">
      <label class="overview-drilldown-window">时间段<ASelect v-model:value="selectedFrom" aria-label="分析时间段" :options="options" /></label>
      <ARadioGroup v-model:value="state" button-style="solid" role="radiogroup" aria-label="分析任务状态"><ARadioButton value="">全部终态</ARadioButton><ARadioButton value="SUCCEEDED">成功</ARadioButton><ARadioButton value="FAILED">失败</ARadioButton><ARadioButton value="CANCELLED">已取消</ARadioButton></ARadioGroup>
      <AButton :loading="busy" :disabled="busy || !valid" @click="list.refresh"><template #icon><ReloadOutlined aria-hidden="true" /></template>刷新列表</AButton>
    </div>
    <p v-if="query" class="overview-meta">终态时间 {{ overviewTime(String(query.finishedFrom)) }} — {{ overviewTime(String(query.finishedTo)) }} UTC（含起点、不含终点）</p>
    <AAlert v-if="!valid" type="warning" show-icon message="分析窗口无效，请关闭后从当前趋势重新打开。" />
    <AAlert v-else-if="failure" type="error" show-icon :message="failure"><template #action><AButton :disabled="busy" @click="list.refresh">重试任务列表</AButton></template></AAlert>
    <ASkeleton v-if="loading" active :paragraph="{ rows: 4 }" aria-label="正在读取分析任务" />
    <ATable v-else-if="!restricted && tasks.length" :components="tableComponents" :data-source="[...tasks]" :columns="columns" row-key="id" :pagination="false" :scroll="{ x: 760 }" :loading="refreshing">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'task'"><AButton type="link" class="overview-object-link" :href="router.resolve({ name: 'task-detail', params: { id: taskOf(record).id } }).href" target="_blank" rel="noopener noreferrer" :aria-label="`查看 ${taskOf(record).displayName} 任务详情（新标签页）`">{{ taskOf(record).displayName }}</AButton><p class="overview-meta">{{ taskTypeText(taskOf(record).type) }} · {{ taskOf(record).id }}</p></template>
        <template v-else-if="column.key === 'state'"><ABadge :status="taskStateBadge(taskOf(record).state, taskOf(record).reconciliationRequired)" :text="taskStateText(taskOf(record).state)" /><ATag v-if="taskOf(record).reconciliationRequired" color="warning">状态核对中</ATag></template>
        <template v-else-if="column.key === 'objects'"><span>{{ taskOf(record).dataSourceName || '历史名称未记录' }}{{ taskOf(record).dataSourceDeleted ? '（已删除）' : '' }}</span><p class="overview-meta">{{ taskOf(record).nodeName || '历史节点名称未记录' }}{{ taskOf(record).nodeDeleted ? '（已删除）' : '' }}</p></template>
        <span v-else-if="column.key === 'finishedAt'" class="overview-meta">{{ overviewTime(taskOf(record).finishedAt) }}</span>
      </template>
    </ATable>
    <AEmpty v-else-if="valid && !failure && !restricted" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="该时段与状态下暂无可查看的任务" />
    <footer v-if="valid && !failure" class="overview-drilldown-pagination"><span class="overview-meta">当前授权范围 · 每页 10 条</span><div><AButton class="orch-icon-action" aria-label="上一页分析任务" :disabled="busy || pageIndex === 0" @click="list.goToPreviousPage"><LeftOutlined aria-hidden="true" /></AButton><span role="status" aria-label="分析任务分页">第 {{ pageIndex + 1 }} 页<template v-if="totalPages"> / {{ totalPages }} 页</template></span><AButton class="orch-icon-action" aria-label="下一页分析任务" :disabled="busy || !nextCursor" @click="list.goToNextPage"><RightOutlined aria-hidden="true" /></AButton></div></footer>
  </div>
</template>

<style scoped>
.overview-drilldown { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--ob-foundation-space-4); min-inline-size: 0; }
.overview-drilldown-controls { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-3); }
.overview-drilldown-window { display: flex; align-items: center; gap: var(--ob-foundation-space-2); min-inline-size: 0; }
.overview-drilldown-window > * { inline-size: 260px; max-inline-size: 100%; }
.overview-drilldown-pagination { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--ob-foundation-space-3); }
.overview-drilldown-pagination > div { display: flex; align-items: center; gap: var(--ob-foundation-space-3); }
@media (max-width: 700px) { .overview-drilldown-window { inline-size: 100%; } }
</style>
