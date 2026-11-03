<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Button as AButton, Empty as AEmpty, Table as ATable, Tag as ATag } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import type { OverviewSection, OverviewTask } from '@/api/overview'
import TaskProgress from '@/workbench/tasks/TaskProgress.vue'
import { taskStateBadge, taskStateText, taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import { overviewTime } from './overviewPresentation'
import { overviewTableComponents } from './overviewTable'

defineProps<{ section: OverviewSection<readonly OverviewTask[]>; busy: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const router = useRouter()
const tableComponents = overviewTableComponents('我的重点任务')
const columns = [
  { title: '任务', key: 'name', width: '40%' }, { title: '状态', key: 'state' }, { title: '进度 / 阶段', key: 'progress' }, { title: '更新时间（UTC）', key: 'time' },
]
</script>

<template>
  <section class="content-card overview-section overview-fixed-panel overview-tasks-panel" aria-labelledby="overview-my-tasks-title">
    <div class="overview-section-heading">
      <h2 id="overview-my-tasks-title">我的任务</h2>
      <AButton type="link" class="overview-object-link" @click="router.push({ name: 'tasks', query: { scope: 'MINE' } })">查看我的全部任务</AButton>
    </div>
    <div class="overview-panel-body" role="region" aria-label="我的任务内容" tabindex="0">
      <ATable v-if="section.status === 'READY' && section.data.length" :components="tableComponents" :data-source="[...section.data]" :columns="columns" row-key="id" :pagination="false" :scroll="{ x: 600 }">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'"><AButton type="link" class="overview-object-link" :title="record.displayName" @click="router.push({ name: 'task-detail', params: { id: record.id } })">{{ record.displayName }}</AButton><p class="overview-meta">{{ taskTypeText(record.type) }} · <span class="overview-identifier">{{ record.id }}</span></p></template>
          <template v-else-if="column.key === 'state'"><ABadge :status="taskStateBadge(record.state, record.reconciliationRequired)" :text="taskStateText(record.state)" /><ATag v-if="record.reconciliationRequired" color="warning">状态核对中</ATag></template>
          <TaskProgress v-else-if="column.key === 'progress'" :progress="record.progress" :state="record.state" :type="record.type" :reconciliation="record.reconciliationRequired" time-zone="UTC" />
          <span v-else-if="column.key === 'time'" class="overview-meta">{{ overviewTime(record.updatedAt) }}</span>
        </template>
      </ATable>
      <div v-else-if="section.status === 'READY'" class="overview-empty"><AEmpty :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有本人创建的重点任务" /></div>
      <div v-else-if="section.status === 'RESTRICTED'" class="overview-empty"><AEmpty :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有可查看的本人任务，请核对基础角色与授权范围。" /></div>
      <AAlert v-else type="warning" show-icon message="我的任务暂不可用。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试我的任务</AButton></template></AAlert>
    </div>
  </section>
</template>
