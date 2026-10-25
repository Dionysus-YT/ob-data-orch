<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Button as AButton, Empty as AEmpty, Table as ATable, Tag as ATag } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import type { OverviewSection, OverviewTask } from '@/api/overview'
import TaskProgress from '@/workbench/tasks/TaskProgress.vue'
import { taskStateBadge, taskStateText, taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import { overviewTime } from './overviewPresentation'

defineProps<{ section: OverviewSection<readonly OverviewTask[]>; busy: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const router = useRouter()
const columns = [
  { title: '任务', key: 'name', width: '40%' }, { title: '状态', key: 'state' }, { title: '进度 / 阶段', key: 'progress' }, { title: '更新时间（UTC）', key: 'time' },
]
</script>

<template>
  <section class="content-card overview-section" aria-labelledby="overview-my-tasks-title">
    <div class="overview-section-heading">
      <div><h2 id="overview-my-tasks-title">我的任务</h2><p>仅本人创建 · 优先展示最近 24 小时失败、正在执行和等待任务</p></div>
      <AButton type="link" class="overview-object-link" @click="router.push({ name: 'tasks', query: { scope: 'MINE' } })">查看我的全部任务</AButton>
    </div>
    <ATable v-if="section.status === 'READY'" :data-source="[...section.data]" :columns="columns" row-key="id" :pagination="false" :scroll="{ x: 600 }" aria-label="我的重点任务">
      <template #emptyText><AEmpty :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有本人创建的重点任务" /></template>
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'"><AButton type="link" class="overview-object-link" :title="record.displayName" @click="router.push({ name: 'task-detail', params: { id: record.id } })">{{ record.displayName }}</AButton><p class="overview-meta">{{ taskTypeText(record.type) }} · <span class="overview-identifier">{{ record.id }}</span></p></template>
        <template v-else-if="column.key === 'state'"><ABadge :status="taskStateBadge(record.state, record.reconciliationRequired)" :text="taskStateText(record.state)" /><ATag v-if="record.reconciliationRequired" color="warning">状态核对中</ATag></template>
        <TaskProgress v-else-if="column.key === 'progress'" :progress="record.progress" :state="record.state" :type="record.type" :reconciliation="record.reconciliationRequired" time-zone="UTC" />
        <span v-else-if="column.key === 'time'" class="overview-meta">{{ overviewTime(record.updatedAt) }}</span>
      </template>
    </ATable>
    <AEmpty v-else-if="section.status === 'RESTRICTED'" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有可查看的本人任务，请核对基础角色与授权范围。" />
    <AAlert v-else type="warning" show-icon message="我的任务暂不可用。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试我的任务</AButton></template></AAlert>
  </section>
</template>
