<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Space as ASpace, Tooltip as ATooltip } from 'ant-design-vue'
import { computed } from 'vue'
import type { ExecutionNodeDetail } from '@/api/browser'
import NodeExecutionQuota from './NodeExecutionQuota.vue'
import NodeResourceUsage from './NodeResourceUsage.vue'
import { nodeTimeLabel } from './executionNodePresentation'
import { nodeDiskUsage } from './nodeResourcePresentation'
import NodeInspectionDetails from './NodeInspectionDetails.vue'
import { inspectionEmptyMessage } from './nodeInspectionPresentation'

// 概览只消费详情所有者的授权事实；不补读、不持有副本，也不从资源占用推导任务资格。
const props = defineProps<{ node: ExecutionNodeDetail }>()
const diskUsage = computed(() => nodeDiskUsage(props.node.dataRootUsages))
</script>

<template>
  <div class="node-runtime-overview">
    <div class="node-runtime-resources">
      <section aria-labelledby="node-resource-title">
        <h2 id="node-resource-title">资源使用率</h2>
        <NodeResourceUsage :node="node" :disk-usage="diskUsage" expanded />
        <p class="node-runtime-note">资源采样：{{ nodeTimeLabel(node.agentFacts?.observedAt) }}</p>
        <p class="node-runtime-note">磁盘取已配置目录所在磁盘的最高使用率。</p>
      </section>
      <section class="node-runtime-quota" aria-labelledby="node-quota-title">
        <h2 id="node-quota-title">执行额度</h2>
        <NodeExecutionQuota :quota="node.executionQuota" />
        <p class="node-runtime-note">预留与已用均为任务分配，独立于主机资源使用率。</p>
      </section>
    </div>
    <AAlert v-if="node.heartbeatStatus === 'OFFLINE'" class="node-runtime-stale" type="warning" message="当前展示离线前的最后可信快照" description="资源与额度不能视为实时值；节点离线不代表运行任务已结束。" show-icon />
    <section class="node-runtime-tasks" aria-labelledby="node-task-title">
      <div class="node-runtime-task-heading">
        <h2 id="node-task-title">{{ node.taskSummary?.scope === 'ALL' ? '全部任务' : node.taskSummary?.scope === 'MINE' ? '我的任务' : '任务统计' }}</h2>
        <RouterLink :to="{ path: '/tasks', query: { nodeId: node.id, scope: node.taskSummary?.scope ?? 'MINE' } }">查看任务 →</RouterLink>
      </div>
      <ASpace :size="24" wrap>
        <ATooltip :title="`包含启动中、运行中和取消中的${node.taskSummary?.scope === 'ALL' ? '全部' : '本人'}任务`"><ABadge :status="node.taskSummary?.running ? 'processing' : 'default'" :text="`运行中 ${node.taskSummary?.running ?? '—'}`" /></ATooltip>
        <ABadge :status="node.taskSummary?.waiting ? 'warning' : 'default'" :text="`等待中 ${node.taskSummary?.waiting ?? '—'}`" />
        <ABadge :status="node.taskSummary?.reconciling ? 'warning' : 'default'" :text="`核对中 ${node.taskSummary?.reconciling ?? '—'}`" />
      </ASpace>
      <p v-if="!node.taskSummary" class="node-runtime-note">尚无当前身份可读取的任务统计。</p>
    </section>
    <section class="node-runtime-tasks" aria-labelledby="node-inspection-title">
      <h2 id="node-inspection-title">环境检查</h2>
      <NodeInspectionDetails v-if="node.environmentCheck?.inspection" :inspection="node.environmentCheck.inspection" :current-check-id="node.environmentCheck.checkId" />
      <p v-else class="node-runtime-note">{{ inspectionEmptyMessage(node.environmentCheck) }}</p>
    </section>
  </div>
</template>

<style scoped>
.node-runtime-overview { min-width: 0; }
.node-runtime-resources { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); gap: var(--ob-foundation-space-6); padding-block: var(--ob-foundation-space-4) var(--ob-foundation-space-6); }
.node-runtime-resources section { min-width: 0; }
.node-runtime-overview h2 { margin: 0 0 var(--ob-foundation-space-4); font-size: var(--ob-product-typography-section-size); }
.node-runtime-quota { padding-left: var(--ob-foundation-space-6); border-left: 1px solid var(--ob-color-border); }
.node-runtime-note { margin: var(--ob-foundation-space-3) 0 0; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); line-height: 1.6; overflow-wrap: anywhere; }
.node-runtime-stale { margin-bottom: var(--ob-foundation-space-4); }
.node-runtime-tasks { padding-block: var(--ob-foundation-space-4); border-top: 1px solid var(--ob-color-border); }
.node-runtime-task-heading { display: flex; justify-content: space-between; align-items: baseline; flex-wrap: wrap; gap: var(--ob-foundation-space-3); }
.node-runtime-task-heading a { color: var(--ob-color-primary); font-size: var(--ob-component-field-label-size); }
@media (max-width: 720px) { .node-runtime-resources { grid-template-columns: 1fr; }.node-runtime-quota { padding: var(--ob-foundation-space-4) 0 0; border-left: 0; border-top: 1px solid var(--ob-color-border); } }
</style>
