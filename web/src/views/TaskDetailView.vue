<script setup lang="ts">
import { FileTextOutlined } from '@ant-design/icons-vue'
import { Button as AButton, Input as AInput } from 'ant-design-vue'
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { taskFailureSummary } from '@/workbench/tasks/taskFailurePresentation'
import { stateLabel, stateClass, dateTime, formatBytes } from '@/workbench/tasks/taskDetailPresentation'
import { useTaskDetail } from '@/workbench/tasks/useTaskDetail'
import { useTaskActions } from '@/workbench/tasks/useTaskActions'

const route = useRoute()
const taskID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const {
  session, overview, execution, snapshot, commandEvidence, logs, loading, failure,
  executionFailure, snapshotFailure, commandFailure, logFailure, logStreamStatus,
  refresh, loadExecution, loadSnapshot, loadCommandEvidence,
} = useTaskDetail(taskID)
const {
  derivationFailure, derivationBusy, noticeTemplate, templateNameInput, savingTemplate,
  checkpointResumeAvailable, rebuildFromTask, resumeFromCheckpoint, saveAsTemplate,
} = useTaskActions(session, useRouter(), execution)
const failureSummary = computed(() => taskFailureSummary(execution.value?.state, logs.value))
const derivationLabel = computed(() => ({ REBUILD_FROM_CONFIG: '基于原配置新建', RERUN_FROM_SCRATCH: '从头重新执行', CHECKPOINT_RESUME: '从检查点继续' })[overview.value?.derivationKind ?? ''] ?? '')
function executionStateLabel() { return stateLabel(execution.value?.state, execution.value?.reconciliationRequired) }
function executionEmptyMessage() {
  return execution.value?.state === 'WAITING_SCHEDULE' ? '任务正等待所选 Agent 领取。' : 'Agent 启动 OBDUMPER 后，标准输出和错误输出会显示在这里。'
}
</script>

<template>
  <section class="page-heading"><div><h1>任务详情</h1><p>任务参数在提交时冻结。页面每两秒刷新执行状态与已脱敏日志，不伪造进度或 ETA。</p></div><RouterLink v-slot="{ href, navigate }" to="/tasks" custom><AButton :href="href" @click="navigate">返回任务中心</AButton></RouterLink></section>
  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }} <AButton type="text" @click="refresh">重试</AButton></p>
  <div v-else-if="loading" class="content-card loading-state" role="status">正在读取任务概览…</div>
  <template v-else-if="overview">
    <section class="content-card task-run-summary">
      <div><span class="detail-kicker">任务 ID</span><h2>{{ overview.id }}</h2><p><span class="status-dot" :class="stateClass(execution?.state)" />{{ executionStateLabel() }}</p></div>
      <div class="task-run-mode">{{ overview.type === 'OBDUMPER_EXPORT' ? '单表 CSV 导出任务' : '任务类型核对中' }}</div>
    </section>
    <section v-if="overview.derivationKind" class="content-card task-derivation-note"><h2>来源任务</h2><p>本任务由来源任务{{ derivationLabel }}派生：<RouterLink class="link-button" :to="`/tasks/${encodeURIComponent(overview.parentTaskId ?? '')}`">{{ overview.parentTaskId }}</RouterLink></p></section>
    <section v-if="failureSummary" class="content-card task-failure-card" role="alert"><h2>失败原因</h2><p>{{ failureSummary }}</p><a class="link-button" href="#execution-logs">查看错误日志</a></section>
    <section v-if="execution?.state === 'SUCCEEDED'" class="content-card task-failure-actions"><h2>成功任务操作</h2><p class="section-hint">保存模板只复制非敏感配置；模板不复制凭据、节点、预检查、风险确认、日志或结果。</p><p v-if="noticeTemplate" class="feedback feedback-notice" role="status">{{ noticeTemplate }}</p><p v-if="derivationFailure" class="feedback feedback-error" role="alert">{{ derivationFailure }}</p><div class="task-derivation-buttons"><AInput v-model:value.trim="templateNameInput" style="min-width: 240px" placeholder="模板名称" /><AButton :disabled="savingTemplate || !templateNameInput.trim()" @click="saveAsTemplate">{{ savingTemplate ? '正在保存…' : '保存为模板' }}</AButton></div></section>
    <section v-if="execution?.state === 'FAILED'" class="content-card task-failure-actions"><h2>失败任务操作</h2><p class="section-hint">所有派生操作都产生新草稿或新任务并保留来源关系；基于原配置新建可以修改参数，从头重新执行与从检查点继续不允许修改参数。派生操作不复制凭据、预检查结果、风险确认或日志。</p><p v-if="derivationFailure" class="feedback feedback-error" role="alert">{{ derivationFailure }}</p><div class="task-derivation-buttons"><AButton :disabled="Boolean(derivationBusy)" @click="rebuildFromTask('REBUILD_FROM_CONFIG')">{{ derivationBusy === 'REBUILD_FROM_CONFIG' ? '正在创建草稿…' : '基于原配置新建' }}</AButton><AButton :disabled="Boolean(derivationBusy)" @click="rebuildFromTask('RERUN_FROM_SCRATCH')">{{ derivationBusy === 'RERUN_FROM_SCRATCH' ? '正在发起…' : '从头重新执行' }}</AButton><AButton v-if="checkpointResumeAvailable" :disabled="Boolean(derivationBusy)" type="primary" @click="resumeFromCheckpoint">{{ derivationBusy === 'CHECKPOINT_RESUME' ? '正在创建继续任务…' : '从检查点继续（--retry）' }}</AButton><p v-else class="section-hint">从检查点继续不可用：任务不是失败终态，或 Agent 未确认输出目录存在可读 dump.ckpt。</p></div></section>
    <section class="module-sections task-detail-sections">
      <article class="content-card section-placeholder task-detail-card"><h2>运行概览</h2><p v-if="executionFailure" class="feedback feedback-error" role="alert">{{ executionFailure }} <AButton type="text" @click="loadExecution">重试</AButton></p><dl v-else-if="execution" class="summary-definition"><div><dt>任务状态</dt><dd>{{ executionStateLabel() }}</dd></div><div><dt>执行编号</dt><dd>{{ execution.executionId || '等待 Agent 领取' }}</dd></div><div><dt>提交时间</dt><dd>{{ dateTime(overview.submittedAt) }}</dd></div><div><dt>开始时间</dt><dd>{{ dateTime(execution.startedAt) }}</dd></div><div><dt>结束时间</dt><dd>{{ dateTime(execution.finishedAt) }}</dd></div><div><dt>最后核对</dt><dd>{{ dateTime(execution.updatedAt) }}</dd></div><div><dt>执行节点</dt><dd>{{ overview.nodeId }}</dd></div><div><dt>阶段与进度</dt><dd>当前没有可验证的工具阶段或进度映射。</dd></div></dl><p v-else class="section-hint">正在读取执行事实…</p></article>
      <article class="content-card task-detail-card"><h2>执行结果</h2><p v-if="executionFailure" class="section-hint">执行状态不可用，结果事实同样无法读取。</p><p v-else-if="!execution?.resultSummary" class="section-hint">任务尚未完成或 Agent 未上报结果事实；平台不推测文件数、字节数或检查点。</p><dl v-else class="summary-definition"><div><dt>结果结论</dt><dd>{{ execution.resultSummary.result === 'VERIFIED' ? '结果已核验（文件存在且非空）' : '结果未通过核验' }}</dd></div><div><dt>结果文件</dt><dd>{{ execution.resultSummary.fileCount }} 个 · {{ formatBytes(execution.resultSummary.totalBytes) }}</dd></div><div><dt>保存点</dt><dd>{{ execution.resultSummary.checkpointPresent ? '输出目录存在 dump.ckpt（失败任务可据此评估从检查点继续）' : '输出目录不存在 dump.ckpt' }}</dd></div><div><dt>观察时间</dt><dd>{{ dateTime(execution.resultSummary.observedAt) }}</dd></div></dl><details v-if="execution?.resultSummary?.files.length" class="task-result-files"><summary>结果文件清单（相对输出目录）</summary><ul><li v-for="file in execution.resultSummary.files" :key="file.path"><code>{{ file.path }}</code><span>{{ formatBytes(file.size) }}</span></li></ul></details></article>
      <article class="content-card section-placeholder task-detail-card"><h2>冻结配置</h2><p v-if="snapshotFailure" class="feedback feedback-error" role="alert">{{ snapshotFailure }} <AButton type="text" @click="loadSnapshot">重试</AButton></p><dl v-else-if="snapshot" class="summary-definition"><div><dt>数据源</dt><dd>{{ snapshot.dataSourceId }}</dd></div><div><dt>导出对象</dt><dd>{{ snapshot.objectSummary || '对象摘要不可用' }}</dd></div><div><dt>输出格式</dt><dd>{{ snapshot.format }}</dd></div><div><dt>预检查</dt><dd>{{ snapshot.precheckId }}</dd></div><div><dt>工具版本</dt><dd>{{ snapshot.toolVersion }}</dd></div><div><dt>配置指纹</dt><dd>{{ snapshot.configFingerprint }}</dd></div></dl><p v-else class="section-hint">正在读取冻结配置…</p></article>
    </section>
    <section class="content-card task-command-card"><h2>计划命令（默认脱敏）</h2><p v-if="commandFailure" class="feedback feedback-error" role="alert">{{ commandFailure }} <AButton type="text" @click="loadCommandEvidence">重试</AButton></p><template v-else-if="commandEvidence"><pre><code>{{ commandEvidence.command }}</code></pre><p>`-p ******` 仅用于展示；Agent 通过任务级官方安全文件读取密码，密码不进入启动参数或日志。</p></template><p v-else class="section-hint">正在读取脱敏命令证据…</p></section>
    <section id="execution-logs" class="content-card task-log-card"><div class="task-log-heading"><div><h2>执行日志</h2><p>只显示当前任务已完成双层脱敏的持久日志。</p><p v-if="logStreamStatus === 'connected'" class="section-hint">实时增量已连接；断线后将从最后可靠游标继续读取。</p><p v-else-if="logStreamStatus === 'interrupted'" class="section-hint">实时连接中断，已保留已加载日志；页面会按最后可靠游标重新读取。</p></div><AButton @click="refresh">立即刷新</AButton></div><p v-if="logFailure" class="feedback feedback-error" role="alert">{{ logFailure }}</p><div v-else-if="logs.length === 0" class="empty-state"><div class="empty-mark"><FileTextOutlined class="product-icon" aria-hidden="true" /></div><h2>尚未收到日志</h2><p>{{ executionEmptyMessage() }}</p></div><div v-else class="task-log-list"><div v-for="record in logs" :key="`${record.sourceSeq}-${record.receivedAt}-${record.kind}-${record.message}`" class="task-log-record"><time>{{ dateTime(record.receivedAt) }}</time><span>{{ record.kind }}</span><code>{{ record.message }}</code></div></div></section>
    <p class="section-hint">当前详情仅展示可授权读取的概览、执行事实、结果摘要、最小冻结快照、脱敏计划命令和本任务日志；路径、凭据、原始执行载荷与原始错误不在本页下发。</p>
  </template>
</template>

<style scoped>
.task-detail-card, .task-failure-actions, .task-derivation-note {
  min-width: 0;
  padding: var(--ob-foundation-space-6);
}
.task-failure-actions, .task-derivation-note { margin-bottom: var(--ob-foundation-space-4); }
.task-detail-card h2, .task-failure-actions h2, .task-derivation-note h2 { margin: 0 0 var(--ob-foundation-space-3); font-size: var(--ob-product-typography-section-size); }
.task-detail-card dd { overflow-wrap: anywhere; }
.task-derivation-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ob-foundation-space-2);
  margin-top: var(--ob-foundation-space-2);
}

.task-derivation-note p {
  margin: var(--ob-foundation-space-1) 0 0;
  color: var(--ob-color-form-secondary);
  font-size: var(--ob-component-field-label-size);
}

.task-result-files {
  margin-top: var(--ob-foundation-space-3);
  border-top: 1px solid var(--ob-color-border);
  padding-top: var(--ob-foundation-space-2);
}

.task-result-files summary {
  cursor: pointer;
  color: var(--ob-color-form-secondary);
  font-size: var(--ob-component-field-label-size);
}

.task-result-files ul {
  margin: var(--ob-foundation-space-2) 0 0;
  padding-left: var(--ob-foundation-space-4);
  list-style: none;
}

.task-result-files li {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--ob-foundation-space-3);
  padding: 2px 0;
  font-size: var(--ob-component-field-helper-size);
}

.task-result-files code {
  overflow: hidden;
  color: var(--ob-product-navigation-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-result-files span {
  flex: none;
  color: var(--ob-color-muted);
  font-variant-numeric: tabular-nums;
}
</style>
