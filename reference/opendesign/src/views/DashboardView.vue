<script setup lang="ts">
import { ref } from 'vue'
import { fixtureTasks } from '../data/fixtures'
import type { SummaryMetric, ViewKey } from '../types'
import AppIcon from '../components/AppIcon.vue'
import DemoNote from '../components/DemoNote.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import StatusSummary from '../components/StatusSummary.vue'

const emit = defineEmits<{
  navigate: [view: ViewKey]
  openTask: [taskId: string]
}>()

const refreshing = ref(false)
const metrics: SummaryMetric[] = [
  { key: 'attention', label: '需要关注', value: 1, note: '失败任务，等待定位原因', progress: 16, tone: 'danger' },
  { key: 'running', label: '正在运行', value: 1, note: '导出任务已被节点领取', progress: 20, tone: 'info' },
  { key: 'nodes', label: '可调度节点', value: 1, note: '1 / 2 个节点满足调度门禁', progress: 50, tone: 'success' },
]

function refresh(): void {
  refreshing.value = true
  window.setTimeout(() => {
    refreshing.value = false
  }, 500)
}
</script>

<template>
  <div class="page dashboard-page" :aria-busy="refreshing">
    <PageHeader title="运行概览" description="在授权范围内快速识别任务异常、执行状态和节点可用性。" data-id="dashboard-title">
      <template #actions>
        <button class="btn" type="button" :disabled="refreshing" :aria-busy="refreshing" data-od-id="refresh-dashboard" @click="refresh">
          <AppIcon name="refresh" />{{ refreshing ? '正在刷新' : '刷新快照' }}
        </button>
        <button class="btn btn-primary" type="button" data-od-id="new-export-cta" @click="emit('navigate', 'wizard')"><AppIcon name="upload" />新建导出任务</button>
      </template>
    </PageHeader>

    <DemoNote text="以下为合成演示状态，仅用于验证信息层级，不代表真实运行事实。" />
    <div class="scope-strip"><span>授权范围：<strong>当前身份可查看的对象</strong></span><span>快照时间：<strong class="mono">2026-08-24 10:32:18 CST</strong></span><span>证据口径：<strong>演示数据</strong></span></div>
    <StatusSummary :metrics="metrics" label="演示状态摘要" />

    <div class="dashboard-grid">
      <section class="section-block" data-od-id="task-trend-section">
        <div class="section-heading"><h2>近 7 日任务活动</h2><span>演示数据 · 任务数</span></div>
        <div class="trend-chart">
          <svg viewBox="0 0 720 190" role="img" aria-label="近七日演示任务活动趋势，从 2 个任务变化到 6 个任务">
            <line class="chart-grid" x1="38" y1="26" x2="704" y2="26" /><line class="chart-grid" x1="38" y1="82" x2="704" y2="82" /><line class="chart-grid" x1="38" y1="138" x2="704" y2="138" />
            <path class="chart-area" d="M42 126 L150 102 L258 114 L366 70 L474 90 L582 46 L690 58 L690 150 L42 150 Z" /><path class="chart-line" d="M42 126 L150 102 L258 114 L366 70 L474 90 L582 46 L690 58" />
            <g><circle class="chart-point" cx="42" cy="126" r="4" /><circle class="chart-point" cx="150" cy="102" r="4" /><circle class="chart-point" cx="258" cy="114" r="4" /><circle class="chart-point" cx="366" cy="70" r="4" /><circle class="chart-point" cx="474" cy="90" r="4" /><circle class="chart-point" cx="582" cy="46" r="4" /><circle class="chart-point" cx="690" cy="58" r="4" /></g>
            <g class="chart-labels" text-anchor="middle"><text x="42" y="176">周一</text><text x="150" y="176">周二</text><text x="258" y="176">周三</text><text x="366" y="176">周四</text><text x="474" y="176">周五</text><text x="582" y="176">周六</text><text x="690" y="176">周日</text></g>
          </svg>
        </div>
      </section>
      <section class="section-block" data-od-id="attention-section">
        <div class="section-heading"><h2>待处理事项</h2><span>3 项演示</span></div>
        <div class="attention-list">
          <article class="attention-item tone-danger"><span class="attention-mark"><AppIcon name="alert" /></span><div><strong>导出任务执行失败</strong><p>节点报告输出目录不可写，需要核对路径权限。</p></div><button class="text-link" type="button" @click="emit('openTask', 'demo-task-040')">查看</button></article>
          <article class="attention-item tone-warning"><span class="attention-mark"><AppIcon name="clock" /></span><div><strong>1 份草稿预检查已失效</strong><p>配置在预检查通过后被修改，需要重新运行。</p></div><button class="text-link" type="button" @click="emit('navigate', 'wizard')">处理</button></article>
          <article class="attention-item tone-warning"><span class="attention-mark"><AppIcon name="server" /></span><div><strong>节点环境检查待复核</strong><p>win-agent-02 的心跳与工具状态已过期，当前不可调度。</p></div><button class="text-link" type="button" @click="emit('navigate', 'nodes')">查看</button></article>
        </div>
      </section>
    </div>

    <section class="table-section" data-od-id="recent-tasks-section">
      <div class="section-heading"><h2>最近任务</h2><span>合成任务记录</span></div>
      <div class="table-wrap recent-task-table"><table><caption class="sr-only">最近演示任务列表</caption><thead><tr><th>任务</th><th>状态</th><th>数据源</th><th class="hide-compact">执行节点</th><th>更新时间</th><th><span class="sr-only">操作</span></th></tr></thead>
        <tbody><tr v-for="task in fixtureTasks.slice(0, 3)" :key="task.id"><td><span class="cell-primary">{{ task.title }}</span><span class="cell-meta mono">{{ task.id }} · OBDUMPER</span></td><td><StatusBadge :label="task.status" :tone="task.tone" /></td><td><span class="truncate">{{ task.source }}</span></td><td class="hide-compact mono">{{ task.node }}</td><td class="mono">{{ task.updatedAt.slice(11) }}</td><td><button class="text-link" type="button" @click="emit('openTask', task.id)">详情</button></td></tr></tbody>
      </table></div>
    </section>
  </div>
</template>
