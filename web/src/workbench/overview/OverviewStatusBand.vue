<script setup lang="ts">
import { Alert as AAlert, Button as AButton } from 'ant-design-vue'
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { OverviewMetrics, OverviewSnapshot } from '@/api/overview'
import { overviewMetricLabels, overviewMetricLink } from './overviewPresentation'

const props = defineProps<{ snapshot: OverviewSnapshot; busy: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const router = useRouter()
// 四项任务指标固定顺序；只有 READY 分区才有数量，未知/未授权不以 0 代替。
const metricKeys: readonly (keyof OverviewMetrics)[] = ['running', 'waiting', 'todaySucceeded', 'todayFailed']
const metrics = computed(() => props.snapshot.metrics.status === 'READY' ? props.snapshot.metrics.data : null)
const nodes = computed(() => props.snapshot.nodes.status === 'READY' ? props.snapshot.nodes.data : null)
// 节点格只使用派生的“不可接收”结论；范围不可见时不输出任何计数。
const nodeFallback = computed(() => props.snapshot.nodes.status === 'RESTRICTED' ? '受限' : '未知')
</script>

<template>
  <section class="content-card overview-status" aria-labelledby="overview-status-title">
    <h2 id="overview-status-title" class="orch-sr-only">运行状态</h2>
    <div class="overview-status-cells">
      <div v-for="key in metricKeys" :key="key" class="overview-status-cell">
        <span class="overview-status-label">{{ overviewMetricLabels[key] }}</span>
        <AButton v-if="metrics" type="link" class="overview-status-value" :aria-label="`${overviewMetricLabels[key]} ${metrics[key]} 个，查看任务`" @click="router.push(overviewMetricLink(key, snapshot))"><strong>{{ metrics[key] }}</strong></AButton>
        <strong v-else class="overview-status-unknown">{{ snapshot.metrics.status === 'RESTRICTED' ? '未授权' : '未知' }}</strong>
        <small>{{ key.startsWith('today') ? 'UTC 自然日 · 按终态时间' : '当前精确状态' }}</small>
      </div>
      <div class="overview-status-cell">
        <span class="overview-status-label">不可接收节点</span>
        <AButton v-if="nodes" type="link" class="overview-status-value" :aria-label="`不可接收节点 ${nodes.unavailable} 个，可见 ${nodes.total} 个，查看执行节点`" @click="router.push({ name: 'nodes' })"><strong>{{ nodes.unavailable }}</strong><span class="overview-status-total"> / {{ nodes.total }}</span></AButton>
        <strong v-else class="overview-status-unknown">{{ nodeFallback }}</strong>
        <small>可见执行节点 · 当前事实</small>
      </div>
    </div>
    <AAlert v-if="snapshot.metrics.status === 'RESTRICTED'" class="overview-section-alert" type="info" show-icon message="当前没有可查看的任务指标，请核对任务授权范围。" />
    <AAlert v-else-if="!metrics" class="overview-section-alert" type="warning" show-icon message="任务指标暂不可用，未以零值代替。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试指标</AButton></template></AAlert>
  </section>
</template>

<style scoped>
.overview-status { min-inline-size: 0; padding: var(--ob-foundation-space-4) var(--ob-foundation-space-6); }
.overview-status-cells { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); }
.overview-status-cell { display: grid; justify-items: start; align-content: start; min-inline-size: 0; padding-inline: var(--ob-foundation-space-6); border-inline-start: 1px solid var(--ob-color-border); }
.overview-status-cell:first-child { padding-inline-start: 0; border-inline-start: 0; }
.overview-status-label { color: var(--ob-color-secondary); }
.overview-status-cell strong { font-size: var(--ob-product-typography-title-size); font-weight: var(--ob-product-typography-weight); line-height: var(--ob-product-typography-title-line-height); }
.overview-status-value { height: auto; padding-inline: 0; }
.overview-status-total { color: var(--ob-color-secondary); }
.overview-status-unknown { color: var(--ob-color-secondary); }
.overview-status-cell small { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); overflow-wrap: anywhere; }
@container overview (max-width: 899px) {
  .overview-status { padding: var(--ob-foundation-space-4); }
  .overview-status-cells { grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr)); gap: var(--ob-foundation-space-4); }
  .overview-status-cell, .overview-status-cell:first-child { padding-inline: 0; border-inline-start: 0; }
}
</style>
