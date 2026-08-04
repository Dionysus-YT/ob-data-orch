<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import { browserApi, taskDetailErrorMessage, type TaskCommandEvidence, type TaskExecution, type TaskLog, type TaskOverview, type TaskSnapshot } from '@/api/browser'
import { taskFailureSummary } from './taskFailurePresentation'
import { createTaskLogStreamLifecycle } from './taskLogStreamLifecycle'

const api = browserApi()
const route = useRoute()
const taskID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const overview = ref<TaskOverview | null>(null)
const execution = ref<TaskExecution | null>(null)
const snapshot = ref<TaskSnapshot | null>(null)
const commandEvidence = ref<TaskCommandEvidence | null>(null)
const logs = ref<readonly TaskLog[]>([])
const loading = ref(true)
const failure = ref('')
const executionFailure = ref('')
const snapshotFailure = ref('')
const commandFailure = ref('')
const logFailure = ref('')
const logCursor = ref<string | undefined>()
const logStreamStatus = ref<'idle' | 'connected' | 'interrupted'>('idle')
const failureSummary = computed(() => taskFailureSummary(execution.value?.state, logs.value))
let pollTimer: ReturnType<typeof setTimeout> | undefined
const logStreamLifecycle = createTaskLogStreamLifecycle()
let stopped = false

onMounted(() => { void refresh() })
onBeforeUnmount(() => {
  stopped = true
  if (pollTimer !== undefined) clearTimeout(pollTimer)
  logStreamLifecycle.close()
})

async function refresh() {
  if (!taskID.value || stopped) return
  loading.value = overview.value === null
  failure.value = ''
  if (overview.value === null) {
    try {
      const item = await api.getTaskOverview(taskID.value)
      if (stopped) return
      overview.value = item
    } catch (error) {
      if (!stopped) failure.value = taskDetailErrorMessage(error, '无法读取任务概览。')
      return
    }
  }
  await Promise.all([
    loadSnapshot(),
    loadCommandEvidence(),
    loadExecution(),
    loadLogs(),
  ])
  if (!stopped) {
    loading.value = false
    pollTimer = setTimeout(() => { void refresh() }, 2000)
  }
}

async function loadExecution() {
  if (!taskID.value || stopped) return
  try {
    const item = await api.getTaskExecution(taskID.value)
    if (stopped) return
    execution.value = item
    executionFailure.value = ''
  } catch (error) {
    if (!stopped) executionFailure.value = taskDetailErrorMessage(error, '执行状态暂时不可读取。')
  }
}

async function loadSnapshot() {
  if (!taskID.value || stopped || snapshot.value !== null) return
  try {
    const item = await api.getTaskSnapshot(taskID.value)
    if (stopped) return
    snapshot.value = item
    snapshotFailure.value = ''
  } catch (error) {
    if (!stopped) snapshotFailure.value = taskDetailErrorMessage(error, '冻结配置暂时不可读取。')
  }
}

async function loadCommandEvidence() {
  if (!taskID.value || stopped || commandEvidence.value !== null) return
  try {
    const item = await api.getTaskCommandEvidence(taskID.value)
    if (stopped) return
    commandEvidence.value = item
    commandFailure.value = ''
  } catch (error) {
    if (!stopped) commandFailure.value = taskDetailErrorMessage(error, '命令证据暂时不可读取。')
  }
}

async function loadLogs() {
  if (!taskID.value || stopped) return
  try {
    const page = await api.getTaskLogs(taskID.value, undefined, logCursor.value)
    if (stopped) return
    appendLogs(page.items)
    logCursor.value = page.lastReliableCursor ?? logCursor.value
    logFailure.value = ''
    ensureLogStream()
  } catch (error) {
    if (!stopped) logFailure.value = taskDetailErrorMessage(error, '日志暂时不可读取。')
  }
}

function ensureLogStream() {
  if (stopped || logStreamLifecycle.active() || !taskID.value || (execution.value?.state !== 'STARTING' && execution.value?.state !== 'RUNNING')) return
  try {
    const opened = logStreamLifecycle.open((onDisconnected) => api.streamTaskLogs(taskID.value, logCursor.value, (record, cursor) => {
      if (stopped) return
      appendLogs([record])
      logCursor.value = cursor ?? logCursor.value
      logStreamStatus.value = 'connected'
    }, onDisconnected), () => {
      if (!stopped) logStreamStatus.value = 'interrupted'
    })
    if (opened) logStreamStatus.value = 'connected'
  } catch {
    logStreamStatus.value = 'interrupted'
  }
}

function appendLogs(records: readonly TaskLog[]) {
  const known = new Set(logs.value.map((record) => `${record.sourceSeq}|${record.receivedAt}|${record.kind}|${record.message}`))
  const appended = records.filter((record) => {
    const key = `${record.sourceSeq}|${record.receivedAt}|${record.kind}|${record.message}`
    if (known.has(key)) return false
    known.add(key)
    return true
  })
  if (appended.length > 0) logs.value = [...logs.value, ...appended]
}

function stateLabel(state?: string, reconciliationRequired = false) {
  if (reconciliationRequired) return '状态核对中'
  return {
    WAITING_SCHEDULE: '等待 Agent 领取',
    STARTING: '正在启动',
    RUNNING: '正在运行',
    SUCCEEDED: '导出成功',
    FAILED: '导出失败',
  }[state ?? ''] ?? '状态核对中'
}

function stateClass(state?: string) {
  return state === 'SUCCEEDED' ? 'success' : state === 'FAILED' ? 'danger' : 'neutral'
}

function executionStateLabel() {
  return stateLabel(execution.value?.state, execution.value?.reconciliationRequired)
}

function executionEmptyMessage() {
  if (execution.value?.state === 'WAITING_SCHEDULE') return '任务正等待所选 Agent 领取。'
  return 'Agent 启动 OBDUMPER 后，标准输出和错误输出会显示在这里。'
}

function dateTime(value?: string) {
  if (!value) return '尚未发生'
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? '时间未知' : parsed.toLocaleString()
}
</script>

<template>
  <section class="page-heading"><div><h1>任务详情</h1><p>任务参数在提交时冻结。页面每两秒刷新执行状态与已脱敏日志，不伪造进度或 ETA。</p></div><RouterLink class="button button-secondary" to="/tasks">返回任务中心</RouterLink></section>
  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }} <button type="button" class="link-button" @click="refresh">重试</button></p>
  <div v-else-if="loading" class="content-card loading-state" role="status">正在读取任务概览…</div>
  <template v-else-if="overview">
    <section class="content-card task-run-summary">
      <div><span class="detail-kicker">任务 ID</span><h2>{{ overview.id }}</h2><p><span class="status-dot" :class="stateClass(execution?.state)" />{{ executionStateLabel() }}</p></div>
      <div class="task-run-mode">{{ overview.type === 'OBDUMPER_EXPORT' ? '单表 CSV 导出任务' : '任务类型核对中' }}</div>
    </section>
    <section v-if="failureSummary" class="content-card task-failure-card" role="alert"><h2>失败原因</h2><p>{{ failureSummary }}</p><a class="link-button" href="#execution-logs">查看错误日志</a></section>
    <section class="module-sections task-detail-sections">
      <article class="content-card section-placeholder task-detail-card"><h2>运行概览</h2><p v-if="executionFailure" class="feedback feedback-error" role="alert">{{ executionFailure }} <button type="button" class="link-button" @click="loadExecution">重试</button></p><dl v-else-if="execution" class="summary-definition"><div><dt>任务状态</dt><dd>{{ executionStateLabel() }}</dd></div><div><dt>执行编号</dt><dd>{{ execution.executionId || '等待 Agent 领取' }}</dd></div><div><dt>提交时间</dt><dd>{{ dateTime(overview.submittedAt) }}</dd></div><div><dt>开始时间</dt><dd>{{ dateTime(execution.startedAt) }}</dd></div><div><dt>结束时间</dt><dd>{{ dateTime(execution.finishedAt) }}</dd></div><div><dt>最后核对</dt><dd>{{ dateTime(execution.updatedAt) }}</dd></div><div><dt>执行节点</dt><dd>{{ overview.nodeId }}</dd></div><div><dt>阶段与进度</dt><dd>当前没有可验证的工具阶段或进度映射。</dd></div></dl><p v-else class="section-hint">正在读取执行事实…</p></article>
      <article class="content-card section-placeholder task-detail-card"><h2>冻结配置</h2><p v-if="snapshotFailure" class="feedback feedback-error" role="alert">{{ snapshotFailure }} <button type="button" class="link-button" @click="loadSnapshot">重试</button></p><dl v-else-if="snapshot" class="summary-definition"><div><dt>数据源</dt><dd>{{ snapshot.dataSourceId }}</dd></div><div><dt>导出对象</dt><dd>{{ snapshot.objectSummary || '对象摘要不可用' }}</dd></div><div><dt>输出格式</dt><dd>{{ snapshot.format }}</dd></div><div><dt>预检查</dt><dd>{{ snapshot.precheckId }}</dd></div><div><dt>工具版本</dt><dd>{{ snapshot.toolVersion }}</dd></div><div><dt>配置指纹</dt><dd>{{ snapshot.configFingerprint }}</dd></div></dl><p v-else class="section-hint">正在读取冻结配置…</p></article>
    </section>
    <section class="content-card task-command-card"><h2>计划命令（默认脱敏）</h2><p v-if="commandFailure" class="feedback feedback-error" role="alert">{{ commandFailure }} <button type="button" class="link-button" @click="loadCommandEvidence">重试</button></p><template v-else-if="commandEvidence"><pre><code>{{ commandEvidence.command }}</code></pre><p>`-p ******` 仅用于展示；Agent 通过任务级官方安全文件读取密码，密码不进入启动参数或日志。</p></template><p v-else class="section-hint">正在读取脱敏命令证据…</p></section>
    <section id="execution-logs" class="content-card task-log-card"><div class="task-log-heading"><div><h2>执行日志</h2><p>只显示当前任务已完成双层脱敏的持久日志。</p><p v-if="logStreamStatus === 'connected'" class="section-hint">实时增量已连接；断线后将从最后可靠游标继续读取。</p><p v-else-if="logStreamStatus === 'interrupted'" class="section-hint">实时连接中断，已保留已加载日志；页面会按最后可靠游标重新读取。</p></div><button type="button" class="button button-secondary" @click="refresh">立即刷新</button></div><p v-if="logFailure" class="feedback feedback-error" role="alert">{{ logFailure }}</p><div v-else-if="logs.length === 0" class="empty-state"><div class="empty-mark">□</div><h2>尚未收到日志</h2><p>{{ executionEmptyMessage() }}</p></div><div v-else class="task-log-list"><div v-for="record in logs" :key="`${record.sourceSeq}-${record.receivedAt}-${record.kind}-${record.message}`" class="task-log-record"><time>{{ dateTime(record.receivedAt) }}</time><span>{{ record.kind }}</span><code>{{ record.message }}</code></div></div></section>
    <p class="section-hint">当前详情仅展示可授权读取的概览、执行事实、最小冻结快照、脱敏计划命令和本任务日志；路径、凭据、原始执行载荷与原始错误不在本页下发。</p>
  </template>
</template>
