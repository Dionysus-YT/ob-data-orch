<script setup lang="ts">
import { computed, ref } from 'vue'
import { fixtureNodes } from '../data/fixtures'
import type { SummaryMetric, Tone } from '../types'
import AppIcon from '../components/AppIcon.vue'
import DemoNote from '../components/DemoNote.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import StatusSummary from '../components/StatusSummary.vue'

const emit = defineEmits<{ openNode: [nodeId: string]; notify: [message: string] }>()
const query = ref('')
const schedule = ref('')
const runtime = ref('')
const refreshing = ref(false)
const filtered = computed(() => {
  const term = query.value.trim().toLowerCase()
  return fixtureNodes.filter((node) => (!term || `${node.id} ${node.displayName} ${node.platform} ${node.tool}`.toLowerCase().includes(term)) && (!schedule.value || node.schedule === schedule.value) && (!runtime.value || node.runtime === runtime.value))
})
const metrics = computed<SummaryMetric[]>(() => {
  const schedulable = fixtureNodes.filter((node) => node.schedule === 'schedulable').length
  const blocked = fixtureNodes.length - schedulable
  const online = fixtureNodes.filter((node) => node.runtime === 'online').length
  return [
    { key: 'registered', label: '已登记', value: fixtureNodes.length, note: '控制面管理字段完整', progress: 100, tone: 'neutral' },
    { key: 'schedulable', label: '可调度', value: schedulable, note: '全部门禁均满足', progress: Math.round((schedulable / fixtureNodes.length) * 100), tone: 'success' },
    { key: 'blocked', label: '不可调度', value: blocked, note: '保留阻断原因', progress: Math.round((blocked / fixtureNodes.length) * 100), tone: 'warning' },
    { key: 'online', label: '心跳在线', value: online, note: '运行时事实仍有效', progress: Math.round((online / fixtureNodes.length) * 100), tone: 'info' },
  ]
})

function nodeTone(value: 'schedulable' | 'blocked'): Tone { return value === 'schedulable' ? 'success' : 'warning' }
function reset(): void { query.value = ''; schedule.value = ''; runtime.value = '' }
function refresh(): void { refreshing.value = true; window.setTimeout(() => { refreshing.value = false }, 450) }
</script>

<template>
  <div class="page page-wide" :aria-busy="refreshing">
    <PageHeader title="执行节点" description="区分控制面配置、Agent 关联、运行时心跳、工具环境与最终可调度状态，避免把已注册误解为可执行。" data-id="execution-node-title">
      <template #actions><button class="btn" type="button" :disabled="refreshing" :aria-busy="refreshing" @click="refresh"><AppIcon name="refresh" />{{ refreshing ? '正在刷新' : '刷新节点状态' }}</button><button class="btn btn-primary" type="button" @click="emit('notify', '注册节点仅模拟界面反馈，不会连接目标机器。')"><AppIcon name="plus" />注册执行节点</button></template>
    </PageHeader>
    <DemoNote text="以下为合成节点事实。任务历史保留其原执行节点，即使该节点当前已不可调度。" />
    <StatusSummary :metrics="metrics" label="执行节点状态摘要" />
    <section class="workspace-section">
      <div class="filter-bar node-center-toolbar"><label class="filter-field is-search"><span>搜索节点</span><AppIcon name="search" /><input v-model="query" placeholder="节点 ID、平台或工具版本" /></label><label class="filter-field"><span>调度状态</span><select v-model="schedule"><option value="">全部状态</option><option value="schedulable">可调度</option><option value="blocked">不可调度</option></select></label><label class="filter-field"><span>运行时状态</span><select v-model="runtime"><option value="">全部状态</option><option value="online">在线</option><option value="stale">心跳过期</option></select></label><button class="btn btn-text" type="button" @click="reset">重置</button></div>
      <div class="table-result-head"><div><h2>节点列表</h2><span aria-live="polite">{{ filtered.length }} 个节点</span></div><p>调度结论来自完整门禁，而不是单一在线状态。</p></div>
      <div class="table-wrap node-table"><table><caption class="sr-only">执行节点列表</caption><thead><tr><th>节点</th><th>调度状态</th><th>运行时</th><th>工具环境</th><th>任务容量</th><th>最后心跳</th><th><span class="sr-only">操作</span></th></tr></thead><tbody><tr v-for="nodeItem in filtered" :key="nodeItem.id"><td class="node-main-cell"><span class="cell-primary">{{ nodeItem.displayName }}</span><span class="cell-meta mono">{{ nodeItem.id }} · {{ nodeItem.platform }}</span></td><td class="node-status-cell"><StatusBadge :label="nodeItem.schedule === 'schedulable' ? '可调度' : '不可调度'" :tone="nodeTone(nodeItem.schedule)" /></td><td class="node-runtime-cell" data-label="运行时"><StatusBadge :label="nodeItem.runtime === 'online' ? '心跳在线' : '心跳过期'" :tone="nodeItem.runtime === 'online' ? 'success' : 'warning'" /></td><td class="node-tool-cell" data-label="工具环境"><span>{{ nodeItem.tool }}</span><span class="cell-meta">{{ nodeItem.java }}</span></td><td class="node-load-cell" data-label="任务容量"><span class="node-load mono">{{ nodeItem.activeTasks }} / {{ nodeItem.capacity }}</span></td><td class="node-heartbeat-cell mono" data-label="最后心跳">{{ nodeItem.lastHeartbeat }}</td><td class="node-action-cell"><button class="text-link" type="button" @click="emit('openNode', nodeItem.id)">详情</button></td></tr></tbody></table><div v-if="!filtered.length" class="empty-inline" role="status">没有符合当前筛选条件的执行节点。</div></div>
    </section>
  </div>
</template>
