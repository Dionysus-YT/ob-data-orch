<script setup lang="ts">
import { Button as AButton, Tooltip as ATooltip } from 'ant-design-vue'
import { computed, ref, useId } from 'vue'
import type { OverviewBucket, OverviewRange } from '@/api/overview'
import { overviewBucketLabel, overviewRangeLabels, overviewTime } from './overviewPresentation'
import { overviewTrendAreaPath, overviewTrendHit, overviewTrendPath, overviewTrendScale, overviewTrendSeries, overviewTrendX, overviewTrendY, type OverviewTrendSeries } from './overviewTrend'

const props = defineProps<{ buckets: readonly OverviewBucket[]; range: OverviewRange; shown: readonly OverviewTrendSeries[]; expanded?: boolean }>()
const emit = defineEmits<{ 'update:shown': [readonly OverviewTrendSeries[]]; analyze: [OverviewBucket] }>()
const hovered = ref<number | null>(null)
const focused = ref<number | null>(null)
// 鼠标离开后恢复键盘焦点读数；同时只打开一个时间桶的提示。
const active = computed(() => hovered.value ?? focused.value)
const scale = computed(() => overviewTrendScale(props.buckets))
const fillId = useId()
const visible = computed(() => overviewTrendSeries.filter(item => props.shown.includes(item.key)))
const indexes = computed(() => [...new Set([0, Math.floor((props.buckets.length - 1) / 3), Math.floor((props.buckets.length - 1) * 2 / 3), props.buckets.length - 1])].filter(index => index >= 0))
const bucket = computed(() => active.value === null ? undefined : props.buckets[active.value])
const chartLabel = computed(() => `任务结果趋势，${overviewRangeLabels[props.range]}，共 ${props.buckets.reduce((sum, item) => sum + item.total, 0)} 个终态任务`)
const y = (count: number) => overviewTrendY(count, scale.value.maximum)
function toggle(key: OverviewTrendSeries) {
  emit('update:shown', props.shown.includes(key) ? props.shown.filter(item => item !== key) : [...props.shown, key])
}
function focusBucket(index: number) {
  focused.value = index
  hovered.value = null
}
</script>

<template>
  <div class="overview-trend-visual" :class="{ 'is-expanded': expanded }">
    <div class="overview-trend-legend" role="group" aria-label="趋势系列">
      <AButton v-for="series in overviewTrendSeries" :key="series.key" type="text" size="small" class="overview-trend-series" :class="{ 'is-hidden': !shown.includes(series.key) }" :aria-pressed="shown.includes(series.key)" @click="toggle(series.key)"><span class="overview-trend-swatch" :class="series.className" aria-hidden="true" /><span>{{ series.label }}</span></AButton>
    </div>
    <div class="overview-trend-axis" aria-hidden="true"><span class="overview-trend-axis-measure">{{ scale.maximum }}</span><span v-for="tick in scale.ticks" :key="tick" class="overview-trend-axis-tick" :style="{ top: `${y(tick) / 212 * 100}%` }">{{ tick }}</span></div>
    <div class="overview-trend-canvas">
      <svg class="overview-trend-chart" viewBox="0 0 760 212" preserveAspectRatio="none" role="img" :aria-label="chartLabel" :data-bucket-count="buckets.length">
        <defs><linearGradient :id="fillId" class="trend-total" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stop-color="currentColor" stop-opacity="0.12" /><stop offset="100%" stop-color="currentColor" stop-opacity="0" /></linearGradient></defs>
        <path v-if="shown.includes('total')" :d="overviewTrendAreaPath(buckets, scale.maximum)" :fill="`url(#${fillId})`" class="trend-area" />
        <line v-for="tick in scale.ticks" :key="tick" x1="8" x2="752" :y1="y(tick)" :y2="y(tick)" class="trend-grid" :class="{ 'trend-baseline': tick === 0 }" />
        <line v-if="active !== null && bucket" :x1="overviewTrendX(buckets, active)" :x2="overviewTrendX(buckets, active)" y1="8" y2="204" class="trend-guide" />
        <g v-for="series in visible" :key="series.key" :class="series.className">
          <path :d="overviewTrendPath(buckets, series.key, scale.maximum)" class="trend-line" />
        </g>
      </svg>
      <!-- 标记使用 CSS 圆形，绘图区宽高独立变化时仍保留真实坐标与圆形比例。 -->
      <template v-if="active !== null && bucket"><span v-for="series in visible" :key="series.key" class="overview-trend-point" :class="series.className" :style="{ left: `${overviewTrendX(buckets, active) / 760 * 100}%`, top: `${y(bucket[series.key]) / 212 * 100}%` }" aria-hidden="true" /></template>
      <!-- 命中区由 Ant Button 承担键盘、焦点和点击；SVG 始终只绘制数据。 -->
      <div class="overview-trend-hit-layer" aria-label="按时间段下钻">
        <ATooltip v-for="(item, index) in buckets" :key="item.from" :open="active === index" :trigger="['hover', 'focus']" color="var(--ob-color-surface)" :overlay-inner-style="{ color: 'var(--ob-color-text)', padding: 'var(--ob-foundation-space-3)', border: '1px solid var(--ob-color-border)' }">
          <template #title>
            <div class="overview-trend-tooltip"><strong>{{ overviewBucketLabel(item, range) }} UTC</strong><span v-for="series in overviewTrendSeries" :key="series.key" class="overview-trend-reading"><span class="overview-trend-swatch" :class="series.className" aria-hidden="true" /><span>{{ series.label }}</span><b>{{ item[series.key] }}</b></span><small>点击查看该时段任务</small></div>
          </template>
          <AButton type="text" class="overview-trend-hit" :style="overviewTrendHit(buckets, index)" :aria-label="`分析 ${overviewTime(item.from)} 至 ${overviewTime(item.to)} UTC 的任务`" @mouseenter="hovered = index" @mousemove="hovered = index" @mouseleave="hovered = null" @focus="focusBucket(index)" @keydown="hovered = null" @blur="focused = null" @click="emit('analyze', item)"><span class="orch-sr-only">分析该时段任务</span></AButton>
        </ATooltip>
      </div>
    </div>
    <div class="overview-trend-ticks" aria-hidden="true"><span v-for="index in indexes" :key="index">{{ overviewBucketLabel(buckets[index]!, range).slice(0, range === '24h' ? 11 : 5) }}</span></div>
    <p class="overview-trend-window">{{ overviewTime(buckets[0]?.from) }} — {{ overviewTime(buckets.at(-1)?.to) }} UTC</p>
    <p v-if="!visible.length" class="overview-meta overview-trend-hidden" role="status">所有系列已隐藏，统计范围保持不变。请选择要展示的系列。</p>
  </div>
</template>

<style scoped>
.overview-trend-visual { display: grid; grid-template-columns: minmax(28px, max-content) minmax(0, 1fr); gap: var(--ob-foundation-space-2); min-inline-size: 0; }
.overview-trend-axis { position: relative; grid-column: 1; grid-row: 2; color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); text-align: end; font-variant-numeric: tabular-nums; }
.overview-trend-axis-measure { visibility: hidden; }
.overview-trend-axis-tick { position: absolute; inset-inline-end: 0; transform: translateY(-50%); }
.overview-trend-canvas { position: relative; grid-column: 2; grid-row: 2; block-size: 200px; min-inline-size: 0; }
.is-expanded .overview-trend-canvas { block-size: clamp(180px, 32dvh, 280px); }
.overview-trend-chart { display: block; width: 100%; height: 100%; overflow: visible; }
.overview-trend-ticks { display: flex; grid-column: 2; justify-content: space-between; gap: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); font-variant-numeric: tabular-nums; }
.overview-trend-ticks > span { white-space: nowrap; }
.trend-grid { stroke: var(--ob-color-border); stroke-opacity: 0.45; stroke-width: 1; vector-effect: non-scaling-stroke; }
.trend-baseline { stroke-opacity: 0.8; }
.trend-guide { stroke: var(--ob-color-secondary); stroke-opacity: 0.65; stroke-dasharray: 3 3; vector-effect: non-scaling-stroke; }
.trend-total { color: var(--ob-color-primary); }
.trend-succeeded { color: var(--ob-color-success); }
.trend-failed { color: var(--ob-color-danger); }
.trend-line { fill: none; stroke: currentColor; stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; stroke-opacity: 0.7; vector-effect: non-scaling-stroke; }
.trend-total .trend-line { stroke-width: 2.25; stroke-opacity: 1; }
.overview-trend-point { position: absolute; z-index: 1; inline-size: 8px; block-size: 8px; border: 1.5px solid currentColor; border-radius: 50%; background: var(--ob-color-surface); transform: translate(-50%, -50%); pointer-events: none; }
.overview-trend-hit-layer { position: absolute; inset: 0; }
.overview-trend-hit { position: absolute; inset-block: 0; min-inline-size: 0; block-size: 100%; padding: 0; }
.overview-trend-hit:hover, .overview-trend-hit:active { background: transparent; }
.overview-trend-legend { display: flex; grid-column: 2; flex-wrap: wrap; gap: var(--ob-foundation-space-1) var(--ob-foundation-space-2); }
.overview-trend-series { display: inline-flex; align-items: center; gap: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-control-font-size); }
.overview-trend-swatch { display: inline-block; flex: none; inline-size: 16px; border-block-start: 2px solid currentColor; }
.overview-trend-series.is-hidden { text-decoration: line-through; }
.overview-trend-series.is-hidden .overview-trend-swatch { color: var(--ob-color-secondary); opacity: 0.45; }
.overview-trend-tooltip { display: grid; gap: var(--ob-foundation-space-1); min-inline-size: 160px; }
.overview-trend-reading { display: grid; grid-template-columns: 16px minmax(0, 1fr) auto; align-items: center; gap: var(--ob-foundation-space-2); }
.overview-trend-reading b { font-variant-numeric: tabular-nums; }
.overview-trend-tooltip small { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.overview-trend-window { grid-column: 2; margin: var(--ob-foundation-space-1) 0 0; color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.overview-trend-hidden { grid-column: 2; }
@container overview (max-width: 480px) { .overview-trend-ticks > span:not(:first-child):not(:last-child) { display: none; } }
</style>
