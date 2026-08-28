<script setup lang="ts">
import { computed, ref } from 'vue'
import { fixtureTasks } from '../data/fixtures'
import type { SummaryMetric } from '../types'
import AppIcon from '../components/AppIcon.vue'
import DemoNote from '../components/DemoNote.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import StatusSummary from '../components/StatusSummary.vue'

const emit = defineEmits<{ openTask: [taskId: string]; openWizard: [] }>()
const query = ref('')
const status = ref('')
const type = ref('')
const source = ref('')
const node = ref('')
const refreshing = ref(false)

const sources = [...new Set(fixtureTasks.map((task) => task.source))]
const nodes = [...new Set(fixtureTasks.map((task) => task.node).filter((item) => item !== '未领取'))]
const filtered = computed(() => {
  const term = query.value.trim().toLowerCase()
  return fixtureTasks.filter((task) => {
    const text = `${task.title} ${task.id} ${task.source} ${task.node}`.toLowerCase()
    return (!term || text.includes(term)) && (!status.value || task.statusKey === status.value) && (!type.value || task.type === type.value) && (!source.value || task.source === source.value) && (!node.value || task.node === node.value)
  })
})
const metrics = computed<SummaryMetric[]>(() => ['running', 'waiting', 'failed', 'success'].map((key) => {
  const matches = fixtureTasks.filter((task) => task.statusKey === key)
  const seed = matches[0]
  return { key, label: seed?.status ?? key, value: matches.length, note: key === 'failed' ? '需要定位阻断原因' : key === 'waiting' ? '等待可用节点领取' : key === 'running' ? '节点已领取并开始执行' : '固定终态已确认', progress: Math.round((matches.length / fixtureTasks.length) * 100), tone: seed?.tone ?? 'neutral' }
}))

function reset(): void {
  query.value = ''
  status.value = ''
  type.value = ''
  source.value = ''
  node.value = ''
}

function refresh(): void {
  refreshing.value = true
  window.setTimeout(() => { refreshing.value = false }, 450)
}
</script>

<template>
  <div class="page page-wide" :aria-busy="refreshing">
    <PageHeader title="任务中心" description="跟踪任务从配置冻结、预检查到执行结果的可追溯事实。" data-id="task-center-title">
      <template #actions><button class="btn" type="button" :disabled="refreshing" :aria-busy="refreshing" @click="refresh"><AppIcon name="refresh" />{{ refreshing ? '正在刷新' : '刷新' }}</button><button class="btn btn-primary" type="button" @click="emit('openWizard')"><AppIcon name="upload" />新建导出任务</button></template>
    </PageHeader>
    <DemoNote text="以下为合成任务记录，用于验证状态筛选、列表层级和详情链路，不代表真实运行结果。" />
    <StatusSummary :metrics="metrics" label="任务状态摘要" />

    <section class="workspace-section">
      <div class="filter-bar task-center-toolbar">
        <label class="filter-field is-search"><span>搜索任务</span><AppIcon name="search" /><input v-model="query" placeholder="任务名称、ID、数据源或节点" /></label>
        <label class="filter-field"><span>运行状态</span><select v-model="status"><option value="">全部状态</option><option value="waiting">等待节点</option><option value="running">运行中</option><option value="success">已成功</option><option value="failed">已失败</option><option value="cancelled">已取消</option></select></label>
        <label class="filter-field"><span>任务类型</span><select v-model="type"><option value="">全部类型</option><option value="DATA_EXPORT">数据导出</option><option value="DDL_EXPORT">结构导出</option></select></label>
        <label class="filter-field"><span>数据源</span><select v-model="source"><option value="">全部数据源</option><option v-for="item in sources" :key="item" :value="item">{{ item }}</option></select></label>
        <label class="filter-field"><span>执行节点</span><select v-model="node"><option value="">全部节点</option><option v-for="item in nodes" :key="item" :value="item">{{ item }}</option></select></label>
        <button class="btn btn-text" type="button" @click="reset">重置</button>
      </div>
      <div class="table-result-head"><div><h2>任务列表</h2><span aria-live="polite">{{ filtered.length }} 项</span></div><p>状态以控制面和执行节点上报的最终事实为准。</p></div>
      <div class="table-wrap task-center-table"><table><caption class="sr-only">导出任务列表</caption><thead><tr><th>任务</th><th>状态</th><th>类型</th><th>数据源</th><th class="task-col-node">执行节点</th><th>当前阶段</th><th class="task-time-cell">最后更新</th><th><span class="sr-only">操作</span></th></tr></thead><tbody>
        <tr v-for="task in filtered" :key="task.id"><td class="task-main-cell"><span class="cell-primary">{{ task.title }}</span><span class="cell-meta mono">{{ task.id }}</span></td><td class="task-status-cell"><StatusBadge :label="task.status" :tone="task.tone" /></td><td class="task-type-cell" data-label="类型"><span>{{ task.typeLabel }}</span><span class="cell-meta mono">{{ task.format }}</span></td><td class="task-source-cell" data-label="数据源">{{ task.source }}</td><td class="task-col-node mono">{{ task.node }}</td><td class="task-stage-cell" data-label="当前阶段"><span class="task-stage">{{ task.stage }}</span><span class="mini-progress"><span :style="{ width: `${task.progress}%` }" /></span><span class="mono progress-value">{{ task.progress }}%</span></td><td class="task-time-cell mono">{{ task.updatedAt }}</td><td class="task-action-cell"><button class="text-link" type="button" @click="emit('openTask', task.id)">详情</button></td></tr>
      </tbody></table><div v-if="!filtered.length" class="empty-inline" role="status">没有符合当前筛选条件的合成任务。重置筛选后可查看全部记录。</div></div>
      <footer class="table-footer"><span>显示 {{ filtered.length }} 项合成记录</span><span>自动刷新未开启</span></footer>
    </section>
  </div>
</template>
