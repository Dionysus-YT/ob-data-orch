<script setup lang="ts">
import { Button as AButton, Checkbox as ACheckbox, Tooltip as ATooltip } from 'ant-design-vue'
import { computed, ref } from 'vue'
import type { OverviewBucket, OverviewRange } from '@/api/overview'
import { overviewBucketLabel, overviewRangeLabels, overviewTime } from './overviewPresentation'
import { overviewTrendHit, overviewTrendMaximum, overviewTrendPath, overviewTrendSeries, overviewTrendX, type OverviewTrendSeries } from './overviewTrend'

const props = defineProps<{ buckets: readonly OverviewBucket[]; range: OverviewRange; shown: readonly OverviewTrendSeries[] }>()
const emit = defineEmits<{ 'update:shown': [readonly OverviewTrendSeries[]]; analyze: [OverviewBucket] }>()
const active = ref<number | null>(null)
const maximum = computed(() => overviewTrendMaximum(props.buckets))
const visible = computed(() => overviewTrendSeries.filter(item => props.shown.includes(item.key)))
const indexes = computed(() => [...new Set([0, Math.floor((props.buckets.length - 1) / 3), Math.floor((props.buckets.length - 1) * 2 / 3), props.buckets.length - 1])].filter(index => index >= 0))
const bucket = computed(() => active.value === null ? undefined : props.buckets[active.value])
const chartLabel = computed(() => `任务结果趋势，${overviewRangeLabels[props.range]}，共 ${props.buckets.reduce((sum, item) => sum + item.total, 0)} 个终态任务`)
const y = (count: number) => 204 - count * 196 / maximum.value
function toggle(key: OverviewTrendSeries, checked: boolean) {
  emit('update:shown', checked ? [...props.shown, key] : props.shown.filter(item => item !== key))
}
</script>

<template>
  <div class="overview-trend-visual">
    <div class="overview-trend-plot">
      <div class="overview-trend-axis" aria-hidden="true"><span v-for="fraction in [1, 0.5, 0]" :key="fraction">{{ Math.round(maximum * fraction) }}</span></div>
      <div class="overview-trend-canvas">
        <svg class="overview-trend-chart" viewBox="0 0 760 212" role="img" :aria-label="chartLabel" :data-bucket-count="buckets.length">
          <line v-for="fraction in [0, 0.5, 1]" :key="fraction" x1="8" x2="752" :y1="y(maximum * fraction)" :y2="y(maximum * fraction)" class="trend-grid" />
          <line v-for="index in indexes" :key="index" :x1="overviewTrendX(buckets, index)" :x2="overviewTrendX(buckets, index)" y1="8" y2="204" class="trend-grid trend-grid-vertical" />
          <line v-if="active !== null && bucket" :x1="overviewTrendX(buckets, active)" :x2="overviewTrendX(buckets, active)" y1="8" y2="204" class="trend-guide" />
          <g v-for="series in visible" :key="series.key" :class="series.className">
            <path :d="overviewTrendPath(buckets, series.key, maximum)" class="trend-line" />
            <circle v-if="active !== null && bucket" :cx="overviewTrendX(buckets, active)" :cy="y(bucket[series.key])" r="4" class="trend-point" />
          </g>
        </svg>
        <!-- 命中区由 Ant Button 承担键盘、焦点和点击；SVG 始终只绘制数据。 -->
        <div class="overview-trend-hit-layer" aria-label="按时间段下钻">
          <ATooltip v-for="(item, index) in buckets" :key="item.from" :trigger="['hover', 'focus']">
            <template #title>
              <div class="overview-trend-tooltip"><strong>{{ overviewBucketLabel(item, range) }} UTC</strong><span v-for="series in overviewTrendSeries" :key="series.key">{{ series.label }}<b>{{ item[series.key] }}</b></span><span>点击查看该时段任务</span></div>
            </template>
            <AButton type="text" class="overview-trend-hit" :style="overviewTrendHit(buckets, index)" :aria-label="`分析 ${overviewTime(item.from)} 至 ${overviewTime(item.to)} UTC 的任务`" @mouseenter="active = index" @mouseleave="active = null" @focus="active = index" @blur="active = null" @click="emit('analyze', item)"><span class="orch-sr-only">分析该时段任务</span></AButton>
          </ATooltip>
        </div>
      </div>
      <div class="overview-trend-ticks" aria-hidden="true"><span v-for="index in indexes" :key="index">{{ overviewBucketLabel(buckets[index]!, range).slice(0, range === '24h' ? 11 : 5) }}</span></div>
    </div>
    <div class="overview-trend-legend" aria-label="趋势系列">
      <ACheckbox v-for="series in overviewTrendSeries" :key="series.key" :checked="shown.includes(series.key)" @change="toggle(series.key, $event.target.checked ?? false)"><span :class="series.className">{{ series.label }}</span></ACheckbox>
    </div>
    <p v-if="!visible.length" class="overview-meta" role="status">所有系列已隐藏，统计范围保持不变。请选择要展示的系列。</p>
  </div>
</template>

<style scoped>
.overview-trend-plot { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--ob-foundation-space-2); min-inline-size: 0; padding-top: var(--ob-foundation-space-2); }
.overview-trend-axis { display: flex; flex-direction: column; justify-content: space-between; padding-block: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); text-align: end; }
.overview-trend-canvas { position: relative; min-inline-size: 0; }
.overview-trend-chart { display: block; width: 100%; height: auto; aspect-ratio: 760 / 212; overflow: visible; }
.overview-trend-ticks { display: flex; grid-column: 2; justify-content: space-between; gap: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.trend-grid { stroke: var(--ob-color-border); stroke-width: 1; vector-effect: non-scaling-stroke; }
.trend-grid-vertical { stroke-dasharray: 3 4; }
.trend-guide { stroke: var(--ob-color-secondary); stroke-dasharray: 3 3; vector-effect: non-scaling-stroke; }
.trend-total { color: var(--ob-color-primary); }
.trend-succeeded { color: var(--ob-color-success); }
.trend-failed { color: var(--ob-color-danger); }
.trend-line { fill: none; stroke: currentColor; stroke-width: 1.5; vector-effect: non-scaling-stroke; }
.trend-point { fill: var(--ob-color-surface); stroke: currentColor; stroke-width: 2; vector-effect: non-scaling-stroke; }
.overview-trend-hit-layer { position: absolute; inset: 0; }
.overview-trend-hit { position: absolute; inset-block: 0; min-inline-size: 0; block-size: 100%; padding: 0; }
.overview-trend-legend { display: flex; flex-wrap: wrap; justify-content: center; gap: var(--ob-foundation-space-3); margin-top: var(--ob-foundation-space-4); }
.overview-trend-tooltip { display: grid; gap: var(--ob-foundation-space-1); min-inline-size: 160px; }
.overview-trend-tooltip > span { display: flex; justify-content: space-between; gap: var(--ob-foundation-space-4); }
</style>
