<script setup lang="ts">
import { Progress as AProgress, Tooltip as ATooltip } from 'ant-design-vue'
import { computed } from 'vue'
import type { ExecutionNodeSummary } from '@/api/browser'
import { resourceCapacity, resourceCapacityLabel, resourcePercentage, resourceUsageColor, type NodeDiskUsage } from './nodeResourcePresentation'

const props = defineProps<{ node: ExecutionNodeSummary; diskUsage?: NodeDiskUsage; expanded?: boolean }>()
const readings = computed(() => [
  { label: 'CPU', percent: resourcePercentage(props.node.agentFacts?.cpuUsagePercent), capacity: undefined },
  { label: '内存', percent: resourcePercentage(props.node.agentFacts?.memoryUsagePercent), capacity: resourceCapacityLabel(resourceCapacity(props.node.agentFacts?.memoryTotalBytes, props.node.agentFacts?.memoryAvailableBytes)) },
  { label: '磁盘', percent: resourcePercentage(props.diskUsage?.percent), capacity: resourceCapacityLabel(props.diskUsage) },
])
</script>

<template>
  <div class="node-resources" :class="{ 'is-stale': node.heartbeatStatus !== 'ONLINE', 'is-expanded': expanded }" :aria-label="`${node.displayName} 资源使用率`">
    <ATooltip v-for="reading in readings" :key="reading.label" :trigger="['hover', 'focus']">
      <template #title>
        <div v-if="reading.label !== 'CPU'">{{ reading.capacity ?? '容量尚未采集' }}</div>
        <div v-else>CPU 使用率：{{ reading.percent === undefined ? '尚未采集' : reading.percent.toFixed(1) + '%' }}</div>
        <div v-if="node.heartbeatStatus === 'OFFLINE'">离线前快照</div>
      </template>
      <div class="node-resource" tabindex="0" :aria-label="`${reading.label} ${reading.percent === undefined ? '尚未采集' : reading.percent.toFixed(1) + '%'}`">
        <span>{{ reading.label }}</span>
        <AProgress :percent="reading.percent ?? 0" :aria-hidden="reading.percent === undefined" :show-info="false" size="small" :stroke-color="resourceUsageColor(reading.percent)" trail-color="var(--ob-color-border)" />
        <span class="node-resource-value">{{ reading.percent === undefined ? '—' : reading.percent.toFixed(1) + '%' }}</span>
      </div>
    </ATooltip>
  </div>
</template>

<style scoped>
.node-resources { display: grid; gap: var(--ob-foundation-space-1); width: 100%; max-width: 240px; margin-inline: auto; }
.node-resource { display: grid; grid-template-columns: 30px minmax(48px, 1fr) 48px; align-items: center; gap: var(--ob-foundation-space-2); font-size: var(--ob-component-table-metadata-size); }
.node-resource-value { text-align: right; font-variant-numeric: tabular-nums; }
.node-resources.is-expanded { max-width: none; gap: var(--ob-foundation-space-4); }
.is-expanded .node-resource { grid-template-columns: 44px minmax(48px, 1fr) 60px; font-size: var(--ob-component-field-label-size); }
.is-stale .node-resource { opacity: .55; }
</style>
