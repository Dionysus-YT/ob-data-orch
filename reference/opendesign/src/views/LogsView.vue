<script setup lang="ts">
import { computed, ref } from 'vue'
import { fixtureLogs, fixtureNodes } from '../data/fixtures'
import type { SummaryMetric, Tone } from '../types'
import AppIcon from '../components/AppIcon.vue'
import DemoNote from '../components/DemoNote.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import StatusSummary from '../components/StatusSummary.vue'

const query = ref('')
const node = ref('')
const level = ref('')
const range = ref('24h')
const refreshing = ref(false)
const metrics = computed<SummaryMetric[]>(() => ['INFO', 'WARN', 'ERROR'].map((value) => {
  const count = fixtureLogs.filter((log) => log.level === value).length
  return { key: value, label: value === 'INFO' ? '信息' : value === 'WARN' ? '警告' : '错误', value: count, note: value === 'INFO' ? '节点已上报的正常事实' : value === 'WARN' ? '需要复核的门禁事实' : '任务阻断或明确失败', progress: Math.round((count / fixtureLogs.length) * 100), tone: value === 'INFO' ? 'info' : value === 'WARN' ? 'warning' : 'danger' }
}))
const filtered = computed(() => {
  const term = query.value.trim().toLowerCase()
  return fixtureLogs.filter((log) => {
    const withinRange = range.value === 'all' || range.value === '7d' || log.time >= '2026-08-24 09:30:00'
    return withinRange && (!term || `${log.title} ${log.detail} ${log.node} ${log.source}`.toLowerCase().includes(term)) && (!node.value || log.node === node.value) && (!level.value || log.level === level.value)
  })
})
function levelTone(value: string): Tone { return value === 'ERROR' ? 'danger' : value === 'WARN' ? 'warning' : 'info' }
function reset(): void { query.value = ''; node.value = ''; level.value = ''; range.value = '24h' }
function refresh(): void { refreshing.value = true; window.setTimeout(() => { refreshing.value = false }, 450) }
</script>

<template>
  <div class="page page-wide" :aria-busy="refreshing">
    <PageHeader title="日志中心" description="汇总执行节点最近事件，保留来源、节点和时间范围，便于从节点详情继续排查。" data-id="log-center-title"><template #actions><button class="btn" type="button" :disabled="refreshing" :aria-busy="refreshing" @click="refresh"><AppIcon name="refresh" />{{ refreshing ? '正在刷新' : '刷新日志' }}</button></template></PageHeader>
    <DemoNote text="以下为合成节点事件，不读取目标机器文件，也不提供远程终端或跨时间全文检索。" />
    <StatusSummary :metrics="metrics" label="节点事件级别摘要" />
    <section class="workspace-section"><div class="filter-bar log-center-toolbar"><label class="filter-field is-search"><span>搜索日志</span><AppIcon name="search" /><input v-model="query" placeholder="事件内容、节点或来源" /></label><label class="filter-field"><span>执行节点</span><select v-model="node"><option value="">全部节点</option><option v-for="item in fixtureNodes" :key="item.id" :value="item.id">{{ item.id }}</option></select></label><label class="filter-field"><span>级别</span><select v-model="level"><option value="">全部级别</option><option value="INFO">INFO</option><option value="WARN">WARN</option><option value="ERROR">ERROR</option></select></label><label class="filter-field"><span>时间范围</span><select v-model="range"><option value="24h">最近 24 小时</option><option value="7d">最近 7 天</option><option value="all">全部演示记录</option></select></label><button class="btn btn-text" type="button" @click="reset">重置</button></div>
      <div class="table-result-head"><div><h2>节点事件</h2><span aria-live="polite">{{ filtered.length }} 条</span></div><p>完整运维日志仍由实际日志系统负责。</p></div>
      <div class="table-wrap log-center-table"><table><caption class="sr-only">执行节点事件日志</caption><thead><tr><th>时间</th><th>级别</th><th>执行节点</th><th>来源</th><th>事件</th></tr></thead><tbody><tr v-for="log in filtered" :key="log.id" :class="{ 'row-danger': log.level === 'ERROR' }"><td class="log-time-cell mono">{{ log.time }}</td><td class="log-level-cell"><StatusBadge :label="log.level" :tone="levelTone(log.level)" /></td><td class="log-node-cell mono" data-label="执行节点">{{ log.node }}</td><td class="log-source-cell mono" data-label="来源">{{ log.source }}</td><td class="log-message-cell" data-label="事件"><span class="cell-primary">{{ log.title }}</span><span class="cell-meta">{{ log.detail }}</span></td></tr></tbody></table><div v-if="!filtered.length" class="empty-inline" role="status">没有符合当前筛选条件的节点事件。</div></div>
    </section>
  </div>
</template>
