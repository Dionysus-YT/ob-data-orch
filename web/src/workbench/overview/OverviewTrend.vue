<script setup lang="ts">
import { FullscreenOutlined, LineChartOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Button as AButton, Empty as AEmpty, Modal as AModal, RadioButton as ARadioButton, RadioGroup as ARadioGroup, Tooltip as ATooltip } from 'ant-design-vue'
import { computed, nextTick, ref, shallowRef, useId, watch } from 'vue'
import type { OverviewBucket, OverviewRange, OverviewSection } from '@/api/overview'
import { activateModal } from '@/components/modalFocus'
import OverviewTrendAnalysis from './OverviewTrendAnalysis.vue'
import OverviewTrendPlot from './OverviewTrendPlot.vue'
import { overviewRangeLabels, overviewTime } from './overviewPresentation'
import type { OverviewTrendSeries, OverviewTrendSession } from './overviewTrend'

const props = defineProps<{ section: OverviewSection<readonly OverviewBucket[]>; range: OverviewRange; snapshotRange: OverviewRange; snapshotAt: string; busy: boolean }>()
const emit = defineEmits<{ 'update:range': [OverviewRange]; retry: []; 'authorization-lost': [] }>()
const shown = ref<readonly OverviewTrendSeries[]>(['total', 'succeeded', 'failed'])
const buckets = computed(() => props.section.status === 'READY' ? props.section.data : [])
const hasResults = computed(() => buckets.value.some(bucket => bucket.total > 0))
const dialog = shallowRef<OverviewTrendSession | null>(null)
const dialogTitle = ref('任务结果趋势 · 放大')
const heading = ref<HTMLElement>()
const headingId = useId()
let sequence = 0
let invoker: HTMLElement | null = null

function closeDialog() { dialog.value = null }
function openDialog(mode: 'chart' | 'analysis', bucket?: OverviewBucket) {
  if (props.section.status !== 'READY' || !hasResults.value || props.range !== props.snapshotRange) return
  if (!dialog.value) invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const source = dialog.value ?? { range: props.snapshotRange, snapshotAt: props.snapshotAt, buckets: buckets.value }
  dialogTitle.value = mode === 'chart' ? '任务结果趋势 · 放大' : '任务结果分析'
  // 弹窗捕获同一图表快照；切换视图或重开创建新代，分析子树同步卸载旧读取。
  dialog.value = { id: ++sequence, mode, range: source.range, snapshotAt: source.snapshotAt, buckets: source.buckets, selectedFrom: bucket?.from }
}
function selectRange(value: unknown) { if (value === '24h' || value === '7d') emit('update:range', value) }
function loseAuthorization() { closeDialog(); emit('authorization-lost') }
// 范围和授权绑定变化立即关闭旧会话；同范围自动刷新不移动弹窗中正在分析的桶。
watch([() => props.range, () => props.snapshotRange, () => props.section.status], closeDialog, { flush: 'sync' })
watch(dialog, async (session, _previous, cleanup) => {
  if (!session) return
  let disposed = false
  let release: (() => void) | undefined
  cleanup(() => { disposed = true; release?.() })
  await nextTick()
  const element = heading.value, root = element?.closest<HTMLElement>('[role="dialog"]')
  if (!disposed && root && element) release = activateModal(root, element, closeDialog, invoker)
}, { flush: 'post' })
</script>

<template>
  <section class="content-card overview-section" aria-labelledby="overview-trend-title">
    <div class="overview-section-heading">
      <div><h2 id="overview-trend-title">任务结果趋势</h2><p>按任务进入终态的时间统计 · UTC</p></div>
      <div class="overview-trend-controls">
        <ARadioGroup :value="range" button-style="solid" role="radiogroup" aria-label="任务结果趋势时间范围" @update:value="selectRange"><ARadioButton value="24h">{{ overviewRangeLabels['24h'] }}</ARadioButton><ARadioButton value="7d">{{ overviewRangeLabels['7d'] }}</ARadioButton></ARadioGroup>
        <AButton type="text" :disabled="!hasResults || range !== snapshotRange" @click="openDialog('analysis')"><template #icon><LineChartOutlined aria-hidden="true" /></template>下钻分析</AButton>
        <ATooltip title="放大"><AButton type="text" class="orch-icon-action" aria-label="放大任务结果趋势" :disabled="!hasResults || range !== snapshotRange" @click="openDialog('chart')"><FullscreenOutlined aria-hidden="true" /></AButton></ATooltip>
      </div>
    </div>
    <template v-if="section.status === 'READY'">
      <AEmpty v-if="!hasResults" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="该时间范围内暂无终态任务" />
      <template v-else>
        <OverviewTrendPlot v-model:shown="shown" :buckets="buckets" :range="snapshotRange" @analyze="openDialog('analysis', $event)" />
        <p class="overview-meta overview-trend-window">{{ overviewTime(buckets[0]?.from) }} — {{ overviewTime(buckets.at(-1)?.to) }} UTC · 总数包含成功、失败和已取消</p>
      </template>
    </template>
    <AEmpty v-else-if="section.status === 'RESTRICTED'" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有可查看的任务趋势，请核对任务授权范围。" />
    <AAlert v-else type="warning" show-icon message="任务趋势暂不可用，未绘制零值趋势。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试趋势</AButton></template></AAlert>
  </section>
  <!-- 保留 Ant 弹层外壳，由本组件同步卸载业务子树；快速关闭也不重建 Portal 锚点。 -->
  <AModal :open="dialog !== null" width="min(1280px, calc(100vw - 48px))" :style="{ top: '24px' }" :footer="null" :focus-trigger-after-close="true" :aria-labelledby="headingId" @cancel="closeDialog">
    <template #title><h2 :id="headingId" ref="heading" tabindex="-1" class="overview-trend-dialog-title">{{ dialogTitle }}</h2></template>
    <div class="overview-trend-dialog-body">
      <template v-if="dialog">
        <div class="overview-trend-dialog-context"><span class="overview-meta">{{ overviewRangeLabels[dialog.range] }} · 图表快照 {{ overviewTime(dialog.snapshotAt) }} UTC</span><AButton v-if="dialog.mode === 'chart'" @click="openDialog('analysis')">分析该范围任务</AButton></div>
        <template v-if="dialog.mode === 'chart'"><OverviewTrendPlot v-model:shown="shown" :buckets="dialog.buckets" :range="dialog.range" @analyze="openDialog('analysis', $event)" /><p class="overview-meta overview-trend-window">总数包含成功、失败和已取消；选择时间点查看该时段任务。</p></template>
        <OverviewTrendAnalysis v-else :key="dialog.id" :session="dialog" @authorization-lost="loseAuthorization" />
      </template>
    </div>
  </AModal>
</template>

<style scoped>
.overview-trend-controls { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: var(--ob-foundation-space-2); }
.overview-trend-window { margin-top: var(--ob-foundation-space-4); }
.overview-trend-dialog-title { margin: 0; font-size: var(--ob-component-overlay-title-size); line-height: var(--ob-component-overlay-title-line-height); }
.overview-trend-dialog-body { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--ob-foundation-space-4); max-block-size: calc(100dvh - 160px); min-inline-size: 0; overflow: auto; padding: var(--ob-foundation-space-1); }
.overview-trend-dialog-context { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--ob-foundation-space-3); }
</style>
