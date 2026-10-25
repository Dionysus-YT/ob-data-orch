<script setup lang="ts">
import { computed, ref } from 'vue'
import { Alert as AAlert, Button as AButton, Checkbox as ACheckbox, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Empty as AEmpty, RadioButton as ARadioButton, RadioGroup as ARadioGroup, Table as ATable } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import type { OverviewBucket, OverviewRange, OverviewSection } from '@/api/overview'
import { overviewBucketLabel, overviewRangeLabels, overviewTasksLink, overviewTime } from './overviewPresentation'

const props = defineProps<{ section: OverviewSection<readonly OverviewBucket[]>; range: OverviewRange; snapshotRange: OverviewRange; busy: boolean }>()
const emit = defineEmits<{ 'update:range': [OverviewRange]; retry: [] }>()
const router = useRouter()
const definitions = [
  { key: 'total', label: '终态总数', state: undefined, className: 'trend-total' },
  { key: 'succeeded', label: '成功', state: 'SUCCEEDED', className: 'trend-succeeded' },
  { key: 'failed', label: '失败', state: 'FAILED', className: 'trend-failed' },
] as const
type Series = typeof definitions[number]['key']
const shown = ref<readonly Series[]>(['total', 'succeeded', 'failed'])
const buckets = computed(() => props.section.status === 'READY' ? props.section.data : [])
const visible = computed(() => definitions.filter(item => shown.value.includes(item.key)))
const hasResults = computed(() => buckets.value.some(bucket => bucket.total > 0))
const maximum = computed(() => Math.max(2, Math.ceil(Math.max(0, ...buckets.value.map(bucket => bucket.total)) / 2) * 2))
// 宽而矮的绘图区：趋势置于页面末尾，只表达结果走势，数值以明细为准。
const x = (index: number) => 48 + index * 1112 / Math.max(1, buckets.value.length - 1)
const y = (value: number) => 132 - value * 112 / maximum.value
const path = (key: Series) => buckets.value.map((bucket, index) => `${index ? 'L' : 'M'}${x(index)},${y(bucket[key])}`).join(' ')
const tickIndexes = computed(() => [...new Set([0, Math.floor((buckets.value.length - 1) / 3), Math.floor((buckets.value.length - 1) * 2 / 3), buckets.value.length - 1])])
const tickLabel = (index: number) => overviewTime(buckets.value[index]!.from).slice(props.snapshotRange === '24h' ? 11 : 5, props.snapshotRange === '24h' ? 16 : 10)
const windowFrom = computed(() => buckets.value[0]?.from), windowTo = computed(() => buckets.value.at(-1)?.to)
const chartLabel = computed(() => `任务结果趋势，${overviewRangeLabels[props.snapshotRange]}，共 ${buckets.value.reduce((sum, bucket) => sum + bucket.total, 0)} 个终态任务；逐时间段数值与下钻见下方明细`)
// 明细表承担图点的全部下钻；图形本身不可交互，避免原生可点击元素绕过 Ant 控件语义。
const columns = [
  { title: '时间段（UTC）', key: 'bucket' },
  ...definitions.map(series => ({ title: series.label, key: series.key, align: 'right' as const })),
]
const detailLabel = (bucket: OverviewBucket, label: string, value: number) => `${overviewTime(bucket.from)} 至 ${overviewTime(bucket.to)} UTC（含起点、不含终点），${label} ${value} 个，查看任务`
function toggle(key: Series, checked: boolean) { shown.value = checked ? [...shown.value, key] : shown.value.filter(item => item !== key) }
function selectRange(value: unknown) { if (value === '24h' || value === '7d') emit('update:range', value) }
// Ant Table 插槽记录为宽类型；行数据只来自已严格解码的 buckets，此处收窄回原类型。
const bucketOf = (record: Record<string, unknown>) => record as unknown as OverviewBucket
const seriesOf = (key: unknown) => definitions.find(series => series.key === key)
</script>

<template>
  <section class="content-card overview-section" aria-labelledby="overview-trend-title">
    <div class="overview-section-heading">
      <div><h2 id="overview-trend-title">任务结果趋势</h2><p>按任务进入终态的时间统计 · UTC</p></div>
      <div class="overview-trend-controls">
        <div v-if="section.status === 'READY' && hasResults" class="overview-trend-legend" aria-label="趋势系列">
          <ACheckbox v-for="series in definitions" :key="series.key" :checked="shown.includes(series.key)" @change="toggle(series.key, $event.target.checked ?? false)">
            <span :class="series.className">{{ series.label }}</span>
          </ACheckbox>
        </div>
        <ARadioGroup :value="range" button-style="solid" role="radiogroup" aria-label="任务结果趋势时间范围" @update:value="selectRange">
          <ARadioButton value="24h">{{ overviewRangeLabels['24h'] }}</ARadioButton><ARadioButton value="7d">{{ overviewRangeLabels['7d'] }}</ARadioButton>
        </ARadioGroup>
      </div>
    </div>
    <template v-if="section.status === 'READY'">
      <AEmpty v-if="!hasResults" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="该时间范围内暂无终态任务" />
      <template v-else>
        <svg class="overview-trend-chart" viewBox="0 0 1200 166" role="img" :aria-label="chartLabel">
          <g v-for="fraction in [0, 0.5, 1]" :key="fraction">
            <line x1="48" x2="1160" :y1="y(maximum * fraction)" :y2="y(maximum * fraction)" class="trend-grid" />
            <text x="38" :y="y(maximum * fraction) + 4" text-anchor="end" class="trend-label">{{ Math.round(maximum * fraction) }}</text>
          </g>
          <g v-for="series in visible" :key="series.key" :class="series.className">
            <path :d="path(series.key)" class="trend-line" />
            <circle v-for="(bucket, index) in buckets" :key="bucket.from" :cx="x(index)" :cy="y(bucket[series.key])" r="4" class="trend-point" />
          </g>
          <text v-for="index in tickIndexes" :key="index" :x="x(index)" y="156" text-anchor="middle" class="trend-label">{{ tickLabel(index) }}</text>
        </svg>
        <p v-if="!visible.length" class="overview-meta" role="status">所有系列已隐藏，统计范围保持不变。请选择要展示的系列。</p>
        <div class="overview-trend-window">
          <span class="overview-meta">{{ overviewTime(windowFrom) }} — {{ overviewTime(windowTo) }} UTC · 总数包含成功、失败和已取消</span>
          <AButton type="link" class="overview-object-link" @click="router.push(overviewTasksLink(undefined, windowFrom, windowTo))">查看该范围全部终态任务</AButton>
        </div>
        <ACollapse ghost class="overview-trend-detail">
          <ACollapsePanel key="buckets" :header="`按时间段查看与下钻（${buckets.length} 个时间段）`">
            <ATable :data-source="[...buckets]" :columns="columns" row-key="from" :pagination="false" :scroll="{ x: 480 }" aria-label="任务结果趋势明细">
              <template #bodyCell="{ column, record }">
                <span v-if="column.key === 'bucket'" class="overview-meta">{{ overviewBucketLabel(bucketOf(record), snapshotRange) }}</span>
                <template v-else-if="seriesOf(column.key)">
                  <AButton v-if="bucketOf(record)[seriesOf(column.key)!.key] > 0" type="link" class="overview-count-link" :aria-label="detailLabel(bucketOf(record), seriesOf(column.key)!.label, bucketOf(record)[seriesOf(column.key)!.key])" @click="router.push(overviewTasksLink(seriesOf(column.key)!.state, bucketOf(record).from, bucketOf(record).to))">{{ bucketOf(record)[seriesOf(column.key)!.key] }}</AButton>
                  <span v-else class="overview-meta">0</span>
                </template>
              </template>
            </ATable>
            <p class="overview-meta">系列开关只影响图形显示，明细与下钻保持同一统计范围。</p>
          </ACollapsePanel>
        </ACollapse>
      </template>
    </template>
    <AEmpty v-else-if="section.status === 'RESTRICTED'" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有可查看的任务趋势，请核对任务授权范围。" />
    <AAlert v-else type="warning" show-icon message="任务趋势暂不可用，未绘制零值趋势。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试趋势</AButton></template></AAlert>
  </section>
</template>

<style scoped>
.overview-trend-controls { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); }
.overview-trend-legend { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-3); }
.overview-trend-chart { display: block; width: 100%; max-height: 200px; overflow: visible; }
.trend-grid { stroke: var(--ob-color-border); stroke-width: 1; }
.trend-label { fill: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.trend-total { color: var(--ob-color-secondary); }
.trend-succeeded { color: var(--ob-color-success); }
.trend-failed { color: var(--ob-color-danger); }
.trend-line { fill: none; stroke: currentColor; stroke-width: 2; }
.trend-point { fill: var(--ob-color-surface); stroke: currentColor; stroke-width: 2; }
.overview-trend-window { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); margin-top: var(--ob-foundation-space-2); }
.overview-trend-detail { margin-top: var(--ob-foundation-space-2); }
.overview-count-link { height: auto; padding-inline: 0; }
</style>
