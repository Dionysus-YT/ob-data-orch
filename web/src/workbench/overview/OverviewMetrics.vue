<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Tooltip as ATooltip } from 'ant-design-vue'
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { OverviewSnapshot } from '@/api/overview'
import { overviewMetricLabels, overviewMetricLink } from './overviewPresentation'

const props = defineProps<{ snapshot: OverviewSnapshot; busy: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const router = useRouter()
// 四项指标沿用原精确口径；插画仅帮助辨识，不推断平台健康或任务资格。
const cards = [
  { key: 'running', artwork: 'overview-metric-art--running', scope: '当前状态' },
  { key: 'waiting', artwork: 'overview-metric-art--waiting', scope: '当前状态' },
  { key: 'todaySucceeded', artwork: 'overview-metric-art--succeeded', scope: 'UTC 今日' },
  { key: 'todayFailed', artwork: 'overview-metric-art--failed', scope: 'UTC 今日' },
] as const
const metrics = computed(() => props.snapshot.metrics.status === 'READY' ? props.snapshot.metrics.data : null)
</script>

<template>
  <section class="overview-metrics" aria-labelledby="overview-status-title">
    <h2 id="overview-status-title" class="orch-sr-only">运行状态</h2>
    <div class="overview-metric-grid">
      <div v-for="card in cards" :key="card.key" class="content-card overview-metric-card">
        <ATooltip :title="card.key.startsWith('today') ? '按 UTC 自然日及任务终态时间统计' : undefined"><span class="overview-metric-label" :tabindex="card.key.startsWith('today') ? 0 : undefined">{{ overviewMetricLabels[card.key] }}</span></ATooltip>
        <span class="overview-metric-art" :class="card.artwork" aria-hidden="true" />
        <div class="overview-metric-result">
          <AButton v-if="metrics" type="link" class="overview-metric-value" :aria-label="`${overviewMetricLabels[card.key]} ${metrics[card.key]} 个，查看任务`" @click="router.push(overviewMetricLink(card.key, snapshot))"><strong>{{ metrics[card.key] }}</strong></AButton>
          <strong v-else class="overview-metric-unknown">{{ snapshot.metrics.status === 'RESTRICTED' ? '未授权' : '未知' }}</strong>
          <span v-if="metrics" class="overview-meta">个</span>
        </div>
        <span class="overview-metric-scope">{{ card.scope }}</span>
      </div>
    </div>
    <AAlert v-if="snapshot.metrics.status === 'RESTRICTED'" class="overview-section-alert" type="info" show-icon message="当前没有可查看的任务指标，请核对任务授权范围。" />
    <AAlert v-else-if="!metrics" class="overview-section-alert" type="warning" show-icon message="任务指标暂不可用，未以零值代替。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试指标</AButton></template></AAlert>
  </section>
</template>

<style scoped>
.overview-metrics { min-inline-size: 0; }
.overview-metric-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--ob-foundation-space-4); }
.overview-metric-card { display: grid; grid-template-columns: 88px minmax(0, 1fr); align-items: center; gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); min-inline-size: 0; padding: var(--ob-foundation-space-6); box-shadow: var(--ob-foundation-surface-shadow); }
.overview-metric-label { grid-column: 2; color: var(--ob-color-text); font-weight: var(--ob-product-typography-weight); }
/* 四象限透明图集保留同一视角和材质，不在 CSS 中重画立体插画。 */
.overview-metric-art { grid-column: 1; grid-row: 1 / 4; inline-size: 104px; block-size: 104px; margin-inline-start: -8px; align-self: start; margin-block-start: -16px; background-image: url('./assets/task-metrics-isometric.png'); background-size: 200% 200%; background-repeat: no-repeat; }
.overview-metric-art--running { background-position: left top; }
.overview-metric-art--waiting { background-position: right top; }
.overview-metric-art--succeeded { background-position: left bottom; }
.overview-metric-art--failed { background-position: right bottom; }
.overview-metric-result { display: flex; grid-column: 2; align-items: baseline; gap: var(--ob-foundation-space-2); min-inline-size: 0; }
.overview-metric-value { height: auto; padding: 0; font-size: calc(var(--ob-product-typography-title-size) * 1.5); font-weight: var(--ob-product-typography-weight); line-height: 1.25; font-variant-numeric: tabular-nums; }
.overview-metric-unknown { color: var(--ob-color-secondary); font-size: var(--ob-product-typography-title-size); }
.overview-metric-scope { grid-column: 2; color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
@container overview (max-width: 899px) {
  .overview-metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .overview-metric-card { padding: var(--ob-foundation-space-4); }
}
@container overview (max-width: 419px) { .overview-metric-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
