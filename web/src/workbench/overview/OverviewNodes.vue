<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Button as AButton, Empty as AEmpty, Table as ATable } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import type { OverviewNode, OverviewNodes, OverviewSection } from '@/api/overview'
import { nodeCapacityLabel, nodeEnvironmentLabel, nodeHeartbeatLabel, nodeManagementLabel, nodeUnavailableReasonLabel } from '@/workbench/nodes/executionNodePresentation'
import { overviewTime } from './overviewPresentation'
import { overviewTableComponents } from './overviewTable'

const props = defineProps<{ section: OverviewSection<OverviewNodes>; busy: boolean; currentTime: number }>()
const emit = defineEmits<{ retry: [] }>()
const router = useRouter()
const tableComponents = overviewTableComponents('执行节点摘要')
const columns = [
  { title: '节点', key: 'name', width: '34%' }, { title: '接收新任务', key: 'accepts' }, { title: '槽位', key: 'capacity' }, { title: '心跳 / 资源（UTC）', key: 'runtime' },
]
// Ant Table 插槽记录为宽类型；行数据只来自已严格解码的节点分区，此处收窄回原类型。
const nodeOf = (record: Record<string, unknown>) => record as unknown as OverviewNode
// 采样超过 90 秒或缺失时标记为过期/未知，只作为中性快照，不推断高负载。
const sampleExpired = (at?: string) => !at || props.currentTime - Date.parse(at) >= 90_000
</script>

<template>
  <section class="content-card overview-section" aria-labelledby="overview-nodes-title">
    <div class="overview-section-heading">
      <div class="overview-nodes-heading">
        <h2 id="overview-nodes-title">执行节点</h2>
        <template v-if="section.status === 'READY'">
          <span class="overview-meta">{{ section.data.total }} 个可见节点</span>
          <AButton type="link" class="overview-object-link" :aria-label="`不可接收节点 ${section.data.unavailable} 个，可见 ${section.data.total} 个，查看执行节点`" @click="router.push({ name: 'nodes' })"><ABadge :status="section.data.unavailable ? 'warning' : 'success'" :text="`${section.data.unavailable} 个不可接收`" /></AButton>
        </template>
        <ABadge v-else-if="section.status === 'RESTRICTED'" status="default" text="范围受限" />
      </div>
      <AButton type="link" class="overview-object-link" @click="router.push({ name: 'nodes' })">查看全部节点</AButton>
    </div>
    <ATable v-if="section.status === 'READY'" :components="tableComponents" :data-source="[...section.data.items]" :columns="columns" row-key="id" :pagination="false" :scroll="{ x: 560 }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <AButton type="link" class="overview-object-link" :title="nodeOf(record).displayName" @click="router.push({ name: 'node-detail', params: { id: nodeOf(record).id } })">{{ nodeOf(record).displayName }}</AButton>
          <p class="overview-meta">{{ nodeManagementLabel(nodeOf(record).managementState) }} · {{ nodeHeartbeatLabel(nodeOf(record).heartbeatStatus) }} · {{ nodeEnvironmentLabel(nodeOf(record).environmentStatus) }}</p>
        </template>
        <template v-else-if="column.key === 'accepts'">
          <ABadge :status="nodeOf(record).acceptsNewTasks ? 'success' : 'warning'" :text="nodeOf(record).acceptsNewTasks ? '可接收任务' : '不可接收任务'" />
          <!-- 全部原因直接以文本列出，不依赖悬停或额外聚焦元素。 -->
          <p v-if="nodeOf(record).unavailableReasons.length" class="overview-node-reason">{{ nodeOf(record).unavailableReasons.map(reason => nodeUnavailableReasonLabel(reason)).join('；') }}</p>
        </template>
        <span v-else-if="column.key === 'capacity'">{{ nodeCapacityLabel(nodeOf(record).capacityStatus) }}<span v-if="nodeOf(record).capacityTotal !== undefined" class="overview-meta">（{{ nodeOf(record).capacityUsed }} / {{ nodeOf(record).capacityTotal }}）</span></span>
        <template v-else-if="column.key === 'runtime'">
          <span class="overview-meta">心跳 {{ overviewTime(nodeOf(record).lastHeartbeatAt) }}</span>
          <p class="overview-meta">CPU {{ nodeOf(record).cpuUsagePercent === undefined ? '未采集' : `${nodeOf(record).cpuUsagePercent}%` }} · 内存 {{ nodeOf(record).memoryUsagePercent === undefined ? '未采集' : `${nodeOf(record).memoryUsagePercent}%` }}</p>
          <p class="overview-meta"><template v-if="nodeOf(record).observedAt">采样 {{ overviewTime(nodeOf(record).observedAt) }}</template><template v-if="sampleExpired(nodeOf(record).observedAt)">{{ nodeOf(record).observedAt ? ' · ' : '' }}采样已过期或未知</template></p>
        </template>
      </template>
    </ATable>
    <AEmpty v-else-if="section.status === 'RESTRICTED'" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有可查看的执行节点，请核对节点对象授权。" />
    <AAlert v-else type="warning" show-icon message="执行节点事实暂不可用。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试节点</AButton></template></AAlert>
  </section>
</template>

<style scoped>
.overview-nodes-heading { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); }
.overview-node-reason { margin: var(--ob-foundation-space-1) 0 0; color: var(--ob-color-warning); font-size: var(--ob-component-table-metadata-size); overflow-wrap: anywhere; }
</style>
