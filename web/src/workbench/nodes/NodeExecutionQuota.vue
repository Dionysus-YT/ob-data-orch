<script setup lang="ts">
import { Tooltip as ATooltip } from 'ant-design-vue'
import { computed } from 'vue'
import { executionQuotaSegments, quotaSegmentPath, type NodeExecutionQuota } from './nodeResourcePresentation'

const props = defineProps<{ quota?: NodeExecutionQuota }>()
const segments = computed(() => {
  let start = 0
  return executionQuotaSegments(props.quota)?.map(segment => {
    const path = quotaSegmentPath(start, segment.percent)
    start += segment.percent
    return { ...segment, path, tooltip: `${segment.label} ${segment.value.toFixed(1)} GiB · ${segment.percent.toFixed(1)}%：${segment.meaning}` }
  })
})
const occupied = computed(() => segments.value ? segments.value[0]!.percent + segments.value[1]!.percent : undefined)
const entries = computed(() => [
  { key: 'total', label: '总量', value: segments.value ? props.quota?.totalGiB : undefined, tooltip: '用于任务分配的总执行预算' },
  { key: 'reserved', label: '预留', value: segments.value ? props.quota?.reservedGiB : undefined, tooltip: segments.value?.[1]?.tooltip },
  { key: 'used', label: '已用', value: segments.value ? props.quota?.usedGiB : undefined, tooltip: segments.value?.[0]?.tooltip },
])
</script>

<template>
  <div class="node-quota" :class="{ 'is-unknown': !segments }" :aria-label="occupied === undefined ? '执行额度未知' : `执行额度占用 ${occupied.toFixed(1)}%`">
    <div class="node-quota-ring">
      <svg v-if="!segments" viewBox="0 0 100 100" aria-hidden="true">
        <circle cx="50" cy="50" r="39.5" fill="none" stroke="var(--ob-color-border)" stroke-width="11" />
      </svg>
      <!-- Tooltip 位于 SVG 外，避免弹层继承 SVG 命名空间而无法渲染。每层只由实际分段接收指针。 -->
      <template v-for="segment in segments" :key="segment.key">
        <ATooltip v-if="segment.path" :title="segment.tooltip"><svg viewBox="0 0 100 100" aria-hidden="true"><path :d="segment.path" :class="`quota-${segment.key}`" :data-segment="segment.key" /></svg></ATooltip>
      </template>
      <div class="node-quota-center"><strong>{{ occupied === undefined ? '—' : occupied.toFixed(0) + '%' }}</strong><span v-if="occupied === undefined">待核对</span></div>
    </div>
    <div class="node-quota-values">
      <ATooltip v-for="entry in entries" :key="entry.key" :title="entry.value === undefined ? '尚无可核对的资源与分配事实' : entry.tooltip" :trigger="['hover', 'focus']">
        <button type="button" :class="`quota-${entry.key}`" :aria-label="`${entry.label} ${entry.value === undefined ? '待核对' : entry.value.toFixed(1) + ' GiB'}`"><span>{{ entry.label }}</span><strong>{{ entry.value === undefined ? '—' : entry.value.toFixed(1) }}<span v-if="entry.value !== undefined" class="node-quota-unit">GiB</span></strong></button>
      </ATooltip>
    </div>
  </div>
</template>

<style scoped>
.node-quota { display: grid; justify-items: center; gap: var(--ob-foundation-space-1); width: 100%; max-width: 190px; margin-inline: auto; }
.node-quota-ring { position: relative; width: 88px; height: 88px; }
.node-quota-ring svg { position: absolute; inset: 0; display: block; width: 100%; height: 100%; pointer-events: none; }
.node-quota-ring path { pointer-events: auto; }
.node-quota-center { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; pointer-events: none; }
.node-quota-center strong { color: var(--ob-color-text); font-size: var(--ob-foundation-space-4); font-weight: 500; }
.node-quota-center span, .node-quota-unit { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.node-quota-values { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); width: 100%; }
.node-quota-values button { display: grid; gap: var(--ob-foundation-space-1); min-width: 0; padding: 0 var(--ob-foundation-space-1); border: 0; background: transparent; color: var(--ob-color-text); font: inherit; font-size: var(--ob-component-table-metadata-size); cursor: help; }
.node-quota-values button span { color: var(--ob-color-secondary); }
.node-quota-values strong { font-weight: 500; font-variant-numeric: tabular-nums; white-space: nowrap; }
.node-quota-unit { margin-left: var(--ob-foundation-space-1); font-weight: 400; }
.quota-used { fill: var(--ob-color-primary); }
.quota-reserved { fill: var(--ob-color-warning); }
.quota-free { fill: var(--ob-color-border-strong); }
.node-quota-values .quota-used strong { color: var(--ob-color-primary); }
.node-quota-values .quota-reserved strong { color: var(--ob-color-warning); }
.is-unknown .node-quota-values strong { color: var(--ob-color-secondary); }
</style>
