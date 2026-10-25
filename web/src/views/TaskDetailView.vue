<script setup lang="ts">
import { ReloadOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Dropdown as ADropdown, Empty as AEmpty, Input as AInput, Menu as AMenu, MenuItem as AMenuItem, Select as ASelect, Skeleton as ASkeleton, Tabs as ATabs, TabPane as ATabPane, Tag as ATag, Timeline as ATimeline, TimelineItem as ATimelineItem, TypographyText as ATypographyText } from 'ant-design-vue'
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { TaskListAction } from '@/api/browser'
import { TypographyParagraph as ATypographyParagraph } from 'ant-design-vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { navigateAntTabs } from '@/composables/antTabKeyboard'
import { environmentTagTheme } from '@/platform/theme'
import TaskActionDialogs from '@/workbench/tasks/TaskActionDialogs.vue'
import TaskActionMenu from '@/workbench/tasks/TaskActionMenu.vue'
import TaskProgress from '@/workbench/tasks/TaskProgress.vue'
import { taskProgressPresentation } from '@/workbench/tasks/taskProgressPresentation'
import { formatBytes } from '@/workbench/tasks/taskDetailPresentation'
import { taskDetailLocation, taskDetailPrimary, taskLogIsError, taskLogSource, taskOperationLabel, taskOperationResult, taskResultText, taskSnapshotGroups } from '@/workbench/tasks/taskDetailRules'
import { taskFailureSummary } from '@/workbench/tasks/taskFailurePresentation'
import { taskActionLabels, taskHistoricalName, taskMoreActions } from '@/workbench/tasks/taskListPresentation'
import { taskDateTimeText as dateTime, taskDurationText, taskEnvironmentLabel, taskStateBadge, taskStateText, taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import { useTaskDetail } from '@/workbench/tasks/useTaskDetail'
import { useTaskListActions } from '@/workbench/tasks/useTaskListActions'
import { useTaskOperations } from '@/workbench/tasks/useTaskOperations'
import { taskLogCenterRoute } from '@/workbench/tasks/taskLogCenterRoute'

const route = useRoute()
const router = useRouter()
const detail = useTaskDetail(computed(() => typeof route.params.id === 'string' ? route.params.id : ''))
const { session, overview, execution, snapshot, commandEvidence, logs, loading, refreshing, refreshDone, failure, executionFailure, snapshotFailure, commandFailure, logFailure, logsExpired, logStreamStatus } = detail
const actions = useTaskListActions({ navigate: location => router.push(location), beginMutation: detail.beginMutation, endMutation: detail.endMutation })
const actionModel = reactive(actions)
const { MessageContext } = useOperationMessage(actions.notice)
const operations = reactive(useTaskOperations(session, detail.restrict))
const task = computed(() => overview.value?.presentation)
const title = computed(() => task.value?.displayName || snapshot.value?.objectSummary || overview.value?.id || '任务详情')
const state = computed(() => execution.value?.state ?? task.value?.state)
const primary = computed(() => taskDetailPrimary(task.value))
const more = computed(() => task.value ? taskMoreActions(task.value).filter(action => action !== primary.value && action !== 'VIEW_LOGS') : [])
const activeTab = computed(() => ['overview', 'snapshot', 'command', 'logs', 'operations'].includes(String(route.query.tab)) ? String(route.query.tab) : 'overview')
const snapshotGroups = computed(() => snapshot.value ? taskSnapshotGroups(snapshot.value) : [])
const derivationLabel = computed(() => ({ REBUILD_FROM_CONFIG: '基于原配置新建', RERUN_FROM_SCRATCH: '从头重新执行', CHECKPOINT_RESUME: '从检查点继续' } as Record<string, string>)[overview.value?.derivationKind ?? ''] ?? '')
const safeFailure = computed(() => execution.value?.errorSummary || taskFailureSummary(state.value, logs.value))
const output = computed(() => execution.value?.outputLocation)
const normalImport = computed(() => overview.value?.type === 'OBLOADER_NORMAL_IMPORT')
const directLoad = computed(() => overview.value?.type === 'OBLOADER_DIRECT_LOAD')
const location = computed(() => taskDetailLocation(overview.value?.type, snapshot.value ?? undefined, output.value))
const remoteResult = computed(() => Boolean(output.value && ['OSS', 'S3', 'COS', 'OBS'].includes(output.value.kind)))
const result = computed(() => execution.value?.resultSummary)
const progress = computed(() => taskProgressPresentation(execution.value?.progress, state.value, execution.value?.reconciliationRequired, overview.value?.type))
const filesExpanded = ref(false)
const keyword = ref('')
const source = ref<string>()
const paused = ref(false)
const logSurface = ref<HTMLElement>()
const filteredLogs = computed(() => logs.value.filter(record => (!source.value || record.kind === source.value) && (!keyword.value || record.message.toLocaleLowerCase().includes(keyword.value.toLocaleLowerCase()))))
const firstError = computed(() => logs.value.findIndex(taskLogIsError))
const gap = computed(() => logs.value.some(record => record.integrityCode !== 'COMPLETE'))
const sourceOptions = ['STDOUT', 'STDERR', 'TOOL_FILE', 'AGENT_EVENT'].map(value => ({ value, label: taskLogSource(value) }))
const returnTo = computed(() => { const back = router.options.history.state.back; return typeof back === 'string' && (back === '/tasks' || back.startsWith('/tasks?')) ? back : '/tasks' })
function setTab(value: string | number) { return router.replace({ query: { ...route.query, tab: String(value) } }) }
function request(action: string) { if (task.value) actions.request(task.value, action as TaskListAction) }
function latest() { const surface = logSurface.value; if (surface) surface.scrollTop = surface.scrollHeight }
async function showError() {
  keyword.value = ''; source.value = undefined
  await setTab('logs')
  await nextTick()
  // 缺少明确 ERROR 行时只聚焦日志区，不把 stderr 或未知消息冒充首条错误。
  const row = logSurface.value?.querySelector<HTMLElement>('[data-log-error="true"]')
  ;(row ?? logSurface.value)?.focus()
  row?.scrollIntoView({ block: 'nearest' })
}
watch(session, () => { actions.invalidate(); keyword.value = ''; source.value = undefined; paused.value = false; filesExpanded.value = false }, { flush: 'sync' })
watch([session, activeTab], () => { if (activeTab.value === 'operations' && !operations.loaded) void operations.load() }, { immediate: true })
watch([() => logs.value.length, activeTab], async () => { await nextTick(); if (!paused.value && activeTab.value === 'logs') latest() }, { flush: 'post' })
</script>

<template>
  <MessageContext />
  <div class="orch-ui task-detail-workspace">
    <header class="orch-page-header page-heading task-object-header">
      <div class="task-object-heading">
        <h1>{{ title }}</h1>
        <div v-if="overview" class="task-object-meta">
          <ATypographyText :copyable="{ text: overview.id }" class="task-object-id">{{ overview.id }}</ATypographyText><span>{{ taskTypeText(overview.type) }}</span>
          <ConfigProvider v-if="task?.environment" :theme="environmentTagTheme(task.environment)"><ATag :bordered="false">{{ taskEnvironmentLabel(task.environment) }}</ATag></ConfigProvider>
          <ABadge :status="taskStateBadge(state, execution?.reconciliationRequired)" :text="taskStateText(state)" />
          <ATag v-if="execution?.reconciliationRequired" color="warning">状态核对中</ATag>
          <span v-if="overview.parentTaskId" class="task-derivation-note">由 <RouterLink :to="`/tasks/${encodeURIComponent(overview.parentTaskId)}`">{{ overview.parentTaskId }}</RouterLink> {{ derivationLabel }}</span>
          <ADropdown v-if="overview.derivedTaskCount" :trigger="['click']"><AButton type="link">已派生 {{ overview.derivedTaskCount }} 个任务</AButton><template #overlay><AMenu><AMenuItem v-for="child in overview.derivedTasks" :key="child.id"><RouterLink :to="`/tasks/${encodeURIComponent(child.id)}`">{{ child.displayName }} · {{ taskStateText(child.state) }}</RouterLink></AMenuItem><AMenuItem v-if="overview.derivedTaskCount > (overview.derivedTasks?.length ?? 0)" disabled>展示最近 100 个授权任务</AMenuItem></AMenu></template></ADropdown>
        </div>
      </div>
      <div class="task-object-actions">
        <RouterLink v-slot="{ href, navigate }" :to="returnTo" custom><AButton type="text" :href="href" @click="navigate">返回任务中心</AButton></RouterLink>
        <AButton type="text" :loading="refreshing" :disabled="loading || refreshing || Boolean(actionModel.busyTaskId)" :aria-busy="refreshing" aria-label="刷新" @click="detail.refresh(true)"><template #icon><ReloadOutlined /></template>{{ refreshing ? '刷新中…' : refreshDone ? '已刷新' : '刷新' }}</AButton>
        <AButton v-if="primary" :type="primary === 'CANCEL' ? 'default' : 'primary'" :danger="primary === 'CANCEL'" :loading="Boolean(actionModel.busyTaskId)" :disabled="Boolean(actionModel.busyTaskId)" @click="request(primary)">{{ taskActionLabels[primary] }}</AButton>
        <TaskActionMenu v-if="more.length" :actions="more" label="更多操作" :session-key="overview?.id" :busy="Boolean(actionModel.busyTaskId)" @select="request" />
      </div>
    </header>
    <AAlert v-if="failure" class="orch-feedback" type="error" message="无法加载任务详情" :description="failure" show-icon><template #action><AButton @click="detail.refresh()">重试</AButton></template></AAlert>
    <ASkeleton v-else-if="loading" active :paragraph="{ rows: 8 }" aria-label="正在读取任务详情" />
    <template v-else-if="overview">
      <AAlert v-if="execution?.plannedActualMatch === 'MISMATCHED'" class="orch-feedback" type="error" message="计划与实际命令摘要不一致" description="结果不能视为成功，请核对命令与执行证据。" show-icon><template #action><AButton @click="setTab('command')">核对命令</AButton></template></AAlert>
      <AAlert v-else-if="state === 'FAILED'" class="orch-feedback" type="error" :message="execution?.errorCode ? `任务失败 · ${execution.errorCode}` : '任务失败'" :description="safeFailure" show-icon><template #action><AButton @click="showError">查看错误日志</AButton></template></AAlert>
      <AAlert v-else-if="execution?.reconciliationRequired" class="orch-feedback" type="warning" message="状态核对中" :description="`最后可信状态：${taskStateText(state)}；最后核对：${dateTime(execution.updatedAt)}。`" show-icon />
      <AAlert v-else-if="state === 'CANCELLING'" class="orch-feedback" type="info" message="正在取消任务" :description="`取消截止时间：${dateTime(execution?.cancelDeadline)}。取消不回滚已经写入的文件或数据。`" show-icon />
      <AAlert v-if="actionModel.failure && !actionModel.confirmation" class="orch-feedback" type="error" message="操作未完成" :description="actionModel.failure" show-icon />
      <section class="orch-management task-detail-surface" aria-label="任务详情工作面">
        <ATabs :active-key="activeTab" @change="setTab" @keydown="navigateAntTabs">
          <ATabPane key="overview" tab="运行概览">
            <AAlert v-if="executionFailure" type="error" message="无法读取执行事实" :description="executionFailure" show-icon><template #action><AButton @click="detail.loadExecution()">重试</AButton></template></AAlert>
            <template v-else>
              <ADescriptions title="执行" :column="{ xs: 1, sm: 2, xl: 3 }" size="small">
                <ADescriptionsItem key="state" label="任务状态">{{ taskStateText(state) }}</ADescriptionsItem><ADescriptionsItem key="stage" label="当前阶段">{{ progress.phase || '—' }}</ADescriptionsItem><ADescriptionsItem key="progress" label="执行进度"><TaskProgress :progress="execution?.progress" :state="state" :reconciliation="execution?.reconciliationRequired" :type="overview.type" /></ADescriptionsItem>
                <ADescriptionsItem v-if="state === 'WAITING_SCHEDULE' && execution?.schedulingWaitReason" key="scheduling-wait" label="等待原因">{{ execution.schedulingWaitReason === 'PLATFORM_CONCURRENCY_LIMIT' ? '平台并发上限暂时阻止新增执行' : '调度策略暂不可用' }}；恢复领取仍需满足节点、预算及预检查条件。</ADescriptionsItem>
                <ADescriptionsItem key="submitted" label="提交时间">{{ dateTime(overview.submittedAt) }}</ADescriptionsItem><ADescriptionsItem key="started" label="开始时间">{{ dateTime(execution?.startedAt) }}</ADescriptionsItem><ADescriptionsItem key="finished" label="结束时间">{{ dateTime(execution?.finishedAt) }}</ADescriptionsItem>
                <ADescriptionsItem key="duration" label="耗时">{{ taskDurationText(execution?.startedAt, execution?.finishedAt) || '—' }}</ADescriptionsItem><ADescriptionsItem key="updated" label="最后核对">{{ dateTime(execution?.updatedAt) }}</ADescriptionsItem><ADescriptionsItem key="execution" label="执行编号"><code>{{ execution?.executionId || '—' }}</code></ADescriptionsItem>
              </ADescriptions>
              <ADescriptions title="对象与位置" :column="{ xs: 1, sm: 2, xl: 3 }" size="small">
                <ADescriptionsItem :label="location.dataSourceLabel">{{ task ? taskHistoricalName(task.dataSourceName, task.dataSourceId, task.dataSourceDeleted) : overview.dataSourceId }}</ADescriptionsItem><ADescriptionsItem label="执行节点">{{ task ? taskHistoricalName(task.nodeName, task.nodeId, task.nodeDeleted) : overview.nodeId }}</ADescriptionsItem><ADescriptionsItem :label="location.objectLabel">{{ task?.objectSummary || snapshot?.objectSummary || '—' }}</ADescriptionsItem>
                <!-- 末项由 Ant 按当前列数补齐，避免固定 span 在窄屏或断点切换时越界。 -->
                <ADescriptionsItem :label="location.kindLabel">{{ location.kind }}</ADescriptionsItem><ADescriptionsItem :label="location.locationLabel"><code>{{ location.value }}</code></ADescriptionsItem>
              </ADescriptions>
              <ADescriptions title="结果" :column="{ xs: 1, sm: 2, xl: 3 }" size="small">
                <ADescriptionsItem label="结果结论">{{ taskResultText(result, remoteResult, overview.type) }}</ADescriptionsItem>
                <template v-if="normalImport"><ADescriptionsItem label="退出码">{{ execution?.exitCode ?? '—' }}</ADescriptionsItem><ADescriptionsItem label="load.ckpt">尚无可靠证据</ADescriptionsItem></template>
                <template v-else-if="directLoad"><ADescriptionsItem label="退出码">{{ execution?.exitCode ?? '—' }}</ADescriptionsItem><ADescriptionsItem label="恢复能力">旁路不支持检查点恢复</ADescriptionsItem></template>
                <template v-else><ADescriptionsItem label="文件数">{{ result && !remoteResult ? result.fileCount : '—' }}</ADescriptionsItem><ADescriptionsItem label="字节数">{{ result && !remoteResult ? formatBytes(result.totalBytes) : '—' }}</ADescriptionsItem><ADescriptionsItem label="dump.ckpt">{{ result ? result.checkpointPresent ? '存在' : '不存在' : '—' }}</ADescriptionsItem></template>
                <ADescriptionsItem label="观察时间">{{ dateTime(result?.observedAt) }}</ADescriptionsItem>
              </ADescriptions>
              <p v-if="normalImport" class="task-section-note">完成证据不代表目标数据库内容已逐行核验；检查点恢复尚未开放。</p>
              <p v-else-if="directLoad" class="task-section-note">旁路结果须结合独立提交与工具完成证据核验；不会生成导出文件或 load.ckpt。</p>
              <template v-if="!normalImport && !directLoad && result?.files.length && !remoteResult"><AButton type="link" :aria-expanded="filesExpanded" @click="filesExpanded = !filesExpanded">{{ filesExpanded ? '收起文件清单' : `展开文件清单（${result.files.length}）` }}</AButton><ul v-if="filesExpanded" class="task-result-files"><li v-for="file in result.files" :key="file.path"><code>{{ file.path }}</code><span>{{ formatBytes(file.size) }}</span></li></ul></template>
            </template>
          </ATabPane>
          <ATabPane key="snapshot" tab="配置快照">
            <AAlert v-if="snapshotFailure" type="error" message="无法读取冻结配置" :description="snapshotFailure" show-icon><template #action><AButton @click="detail.loadSnapshot()">重试</AButton></template></AAlert>
            <template v-else-if="snapshot">
              <p class="task-section-note">提交时的生效值保持冻结。历史快照未记录字段来源，统一标记“来源未记录”。</p>
              <ADescriptions v-for="group in snapshotGroups" :key="group.key" :title="group.label" :column="{ xs: 1, sm: 2 }" size="small"><ADescriptionsItem v-for="field in group.fields" :key="field.key" :label="field.label"><div class="task-snapshot-field"><ATypographyParagraph class="task-snapshot-value" :content="field.value" :ellipsis="{ rows: 3, expandable: true, symbol: '展开' }" /><ATag class="task-origin">来源未记录</ATag></div></ADescriptionsItem></ADescriptions>
              <ADescriptions title="冻结证据" :column="{ xs: 1, sm: 2 }" size="small"><ADescriptionsItem label="工具版本">{{ snapshot.toolVersion }}</ADescriptionsItem><ADescriptionsItem label="参数元数据版本">{{ snapshot.metadataVersion }}</ADescriptionsItem><ADescriptionsItem label="能力版本">{{ snapshot.capabilityVersion }}</ADescriptionsItem><ADescriptionsItem label="配置指纹"><code>{{ snapshot.configFingerprint }}</code></ADescriptionsItem></ADescriptions>
            </template>
          </ATabPane>
          <ATabPane key="command" tab="命令">
            <AAlert v-if="commandFailure" type="error" message="无法读取命令证据" :description="commandFailure" show-icon><template #action><AButton @click="detail.loadCommandEvidence()">重试</AButton></template></AAlert>
            <template v-else-if="commandEvidence"><div class="task-section-heading"><h2>计划执行命令</h2><AButton v-if="task?.availableActions.includes('COPY_COMMAND')" :loading="Boolean(actionModel.busyTaskId)" @click="request('COPY_COMMAND')">复制脱敏命令</AButton></div><pre class="task-evidence"><code>{{ commandEvidence.command }}</code></pre><p class="task-section-note">密码已脱敏，秘密由任务级安全文件提供。</p></template>
            <ADescriptions title="实际启动证据" :column="{ xs: 1, sm: 2 }" size="small"><ADescriptionsItem label="与计划摘要核对">{{ execution?.plannedActualMatch === 'MATCHED' ? '一致' : execution?.plannedActualMatch === 'MISMATCHED' ? '不一致' : '尚无核对证据' }}</ADescriptionsItem><ADescriptionsItem label="启动时间">{{ dateTime(execution?.startedAt) }}</ADescriptionsItem><ADescriptionsItem label="执行节点">{{ task?.nodeName || overview.nodeId }}</ADescriptionsItem><ADescriptionsItem label="退出码">{{ execution?.exitCode ?? '—' }}</ADescriptionsItem></ADescriptions>
            <AAlert type="info" message="完整实际启动命令与启动目录未保存" description="当前只展示 Agent 上报的摘要核对事实，不能由计划命令反推实际命令。" show-icon />
          </ATabPane>
          <ATabPane key="logs" tab="执行日志">
            <p v-if="overview"><RouterLink :to="taskLogCenterRoute(overview.id, overview.submittedAt)">在日志中心查询与下载</RouterLink></p>
            <div class="task-log-toolbar"><AInput v-model:value="keyword" allow-clear placeholder="过滤已加载日志" aria-label="日志关键字" class="task-log-keyword" /><ASelect v-model:value="source" :options="sourceOptions" allow-clear placeholder="全部来源" aria-label="日志来源" class="task-log-source" /><AButton :disabled="firstError < 0" @click="showError">跳转首条错误</AButton><AButton :aria-pressed="paused" @click="paused = !paused">{{ paused ? '恢复自动滚动' : '暂停自动滚动' }}</AButton><AButton @click="latest">跳到最新</AButton><AButton @click="detail.loadLogs()">刷新日志</AButton><ABadge :status="logStreamStatus === 'connected' ? 'processing' : logStreamStatus === 'interrupted' ? 'warning' : 'default'" :text="logStreamStatus === 'connected' ? '实时连接已连接' : logStreamStatus === 'interrupted' ? '实时连接中断' : '实时连接未开启'" /></div>
            <AAlert v-if="logsExpired" type="info" message="日志已过保留期" description="普通执行日志已清理；任务快照、结果和操作记录仍可查看。" show-icon />
            <AAlert v-else-if="logFailure" type="error" message="无法读取执行日志" :description="logFailure" show-icon />
            <AAlert v-if="gap && !logsExpired" type="warning" message="日志完整性需要核对" description="部分记录存在缺口或完整性未知；已加载内容保留，不能视为完整执行证据。" show-icon />
            <div v-if="!logsExpired" ref="logSurface" class="task-log-list task-evidence" tabindex="0" aria-label="已脱敏执行日志"><AEmpty v-if="!filteredLogs.length" :description="logs.length ? '没有符合条件的已加载日志' : '尚未收到日志'" /><div v-for="(record, index) in filteredLogs" :key="`${record.sourceSeq}-${record.receivedAt}-${record.kind}-${index}`" class="task-log-record" :data-log-error="taskLogIsError(record)" tabindex="-1"><time>{{ dateTime(record.receivedAt) }}</time><span>{{ taskLogSource(record.kind) }}</span><code><strong v-if="taskLogIsError(record)" class="task-log-error">错误 · </strong>{{ record.message }}</code></div></div>
          </ATabPane>
          <ATabPane key="operations" tab="操作记录">
            <AAlert v-if="operations.failure" type="error" message="无法读取操作记录" :description="operations.failure" show-icon><template #action><AButton @click="operations.load()">重试</AButton></template></AAlert>
            <ASkeleton v-else-if="operations.loading && !operations.operations.length" active />
            <AEmpty v-else-if="!operations.operations.length" description="暂无操作记录" />
            <ATimeline v-else class="task-operations"><ATimelineItem v-for="operation in operations.operations" :key="operation.id" :color="operation.result === 'FAILED' || operation.result === 'DENIED' ? 'red' : 'blue'"><div class="task-operation-heading"><strong>{{ taskOperationLabel(operation.action) }}</strong><span>{{ taskOperationResult(operation.result) }}</span></div><p>{{ operation.actorName }} · {{ dateTime(operation.occurredAt) }}</p></ATimelineItem></ATimeline>
            <AButton v-if="operations.nextCursor" :loading="operations.loading" @click="operations.load(true)">加载更多</AButton>
          </ATabPane>
        </ATabs>
      </section>
    </template>
  </div>
  <TaskActionDialogs :model="actionModel" />
</template>

<style scoped>
.task-detail-workspace, .task-object-heading { min-width: 0; }
.task-object-header { display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: var(--ob-foundation-space-4); }
.task-object-heading { flex: 1 1 320px; }
.task-object-heading h1 { margin: 0; font-size: var(--ob-product-typography-title-size); line-height: var(--ob-product-typography-title-line-height); overflow-wrap: anywhere; }
.task-object-meta, .task-object-actions, .task-log-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.task-object-meta { margin-top: var(--ob-foundation-space-2); color: var(--ob-color-secondary); }
.task-object-meta :deep(.ant-tag) { margin: 0; }
.task-object-id { min-width: 0; overflow-wrap: anywhere; font-family: var(--ob-foundation-font-mono); font-size: var(--ob-component-table-metadata-size); }
.task-object-actions { justify-content: flex-end; flex: none; }
.task-detail-surface { padding: 0 var(--ob-foundation-space-6) var(--ob-foundation-space-6); }
.task-detail-surface :deep(.ant-descriptions) { margin-bottom: var(--ob-foundation-space-6); }
.task-detail-surface :deep(.ant-descriptions-item-content) { overflow-wrap: anywhere; min-width: 0; }
.task-section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--ob-foundation-space-3); }
.task-section-heading h2 { font-size: var(--ob-product-typography-section-size); margin: 0; }
.task-section-note, .task-operations p { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.task-origin { margin-inline: var(--ob-foundation-space-2) 0; }
.task-snapshot-field { min-width: 0; width: 100%; }
.task-snapshot-value { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.task-evidence { max-width: 100%; padding: var(--ob-foundation-space-4); overflow: auto; background: var(--ob-color-subtle); font-family: var(--ob-foundation-font-mono); border: 1px solid var(--ob-color-border); border-radius: var(--ob-component-control-radius); }
.task-log-toolbar { margin-bottom: var(--ob-foundation-space-4); }
.task-log-keyword { width: 220px; }
.task-log-source { width: 150px; }
.task-log-list { max-height: min(560px, 60vh); margin-top: var(--ob-foundation-space-3); }
.task-log-record { display: grid; grid-template-columns: 170px 90px minmax(0, 1fr); gap: var(--ob-foundation-space-3); padding-block: var(--ob-foundation-space-1); font-size: var(--ob-component-table-metadata-size); }
.task-log-record code { white-space: pre-wrap; overflow-wrap: anywhere; }
.task-log-error { color: var(--ob-color-danger); }
.task-result-files { padding-left: var(--ob-foundation-space-4); }
.task-result-files li, .task-operation-heading { display: flex; justify-content: space-between; align-items: baseline; gap: var(--ob-foundation-space-3); }
.task-result-files code { overflow-wrap: anywhere; }
.task-operations { padding-top: var(--ob-foundation-space-3); }
@media (max-width: 1100px) { .task-object-header { flex-direction: column; } .task-object-heading { flex: none; width: 100%; } .task-object-actions { justify-content: flex-start; } }
@media (max-width: 620px) { .task-detail-surface { padding-inline: var(--ob-foundation-space-3); } .task-log-record { grid-template-columns: 1fr; gap: var(--ob-foundation-space-1); } .task-log-keyword { width: 100%; } }
</style>
