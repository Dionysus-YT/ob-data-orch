<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { useRoute } from 'vue-router'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { Alert as AAlert, Button as AButton, ConfigProvider, DatePicker, Empty as AEmpty, Form as AForm, FormItem as AFormItem, Input as AInput, Modal as AModal, Select as ASelect, SelectOption as ASelectOption, Space as ASpace, Tag as ATag, type TableColumnsType } from 'ant-design-vue'
import { LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons-vue'
import { browserApi } from '@/api/browser'
import type { LogItem } from '@/api/logQuery'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { managementTheme } from '@/platform/theme'
import { useLogQuery } from '@/workbench/log-query/useLogQuery'
import { logHighlights, logMessagePreview, logSourceLabel, logTime } from '@/workbench/log-query/presentation'
import LogContextDrawer from '@/workbench/log-query/LogContextDrawer.vue'
import '@/workbench/log-query/logQuery.css'

dayjs.extend(utc)
const ARangePicker = DatePicker.RangePicker, route = useRoute()
const state = useLogQuery(browserApi(), { copy: text => navigator.clipboard.writeText(text), save: file => {
  const url = URL.createObjectURL(file), link = document.createElement('a'); link.href = url; link.download = 'task-logs.log'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 0)
} }), model = reactive(state)
const { MessageContext } = useOperationMessage(state.notice)
watch(() => route.fullPath, () => {
  const taskId = typeof route.query.taskId === 'string' ? route.query.taskId : '', from = typeof route.query.from === 'string' ? route.query.from : '', to = typeof route.query.to === 'string' ? route.query.to : ''
  state.reset(taskId, from, to); if (taskId) void state.run()
}, { immediate: true, flush: 'sync' })
const range = computed(() => model.filters.from && model.filters.to ? [dayjs.utc(model.filters.from), dayjs.utc(model.filters.to)] as [dayjs.Dayjs, dayjs.Dayjs] : undefined)
function updateRange(value: unknown) { const values = Array.isArray(value) ? value : []; state.setRange(dayjs.isDayjs(values[0]) ? values[0].toISOString() : '', dayjs.isDayjs(values[1]) ? values[1].toISOString() : '') }
const columns: TableColumnsType<LogItem> = [{ key: 'time', title: '接收时间', width: 192 }, { key: 'task', title: '任务 / 来源', width: 210 }, { key: 'message', title: '脱敏消息', width: 500 }, { key: 'actions', title: '操作', width: 112, fixed: 'right' }]
const emptyMessage = computed(() => !model.page ? '选择时间与条件后查询任务日志' : model.page.availability === 'EXPIRED' ? '匹配范围的保留日志已清理' : model.page.availability === 'NOT_COLLECTED' ? '授权范围内尚无保留日志采集证据' : '当前条件没有匹配日志')
</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <MessageContext />
    <div class="orch-ui log-query-workspace">
      <p class="orch-context-label">查询有权任务的已脱敏日志，按接收时间倒序展示。任务实时日志可在任务详情中查看。</p>
      <section class="orch-management" aria-label="任务日志查询">
        <AForm name="log-query" :model="model.filters" layout="vertical" class="log-query-filters" @finish="state.run()">
          <AFormItem label="接收时间范围（UTC）" :validate-status="model.timeError ? 'error' : undefined" :help="model.timeError" class="log-query-range">
            <ARangePicker class="log-query-picker" :value="range" :show-time="true" format="YYYY-MM-DD HH:mm:ss" :allow-clear="true" :placeholder="['开始时间', '结束时间']" @change="updateRange" />
          </AFormItem>
          <AFormItem name="taskId" label="精确任务 ID" :validate-status="model.taskError ? 'error' : undefined" :help="model.taskError"><AInput v-model:value="model.filters.taskId" aria-label="精确任务 ID" :maxlength="200" allow-clear placeholder="可选，仅查询指定任务" /></AFormItem>
          <AFormItem name="source" label="原始来源"><ASelect v-model:value="model.filters.source" aria-label="原始来源"><ASelectOption value="">全部已接入来源</ASelectOption><ASelectOption v-for="source in model.page?.sources ?? []" :key="source" :value="source">{{ logSourceLabel[source] }}</ASelectOption></ASelect></AFormItem>
          <AFormItem name="keyword" label="脱敏文本关键字" :validate-status="model.keywordError ? 'error' : undefined" :help="model.keywordError"><AInput v-model:value="model.filters.keyword" aria-label="脱敏文本关键字" :maxlength="256" allow-clear placeholder="普通文本包含" /></AFormItem>
          <ASpace class="log-query-submit"><AButton type="primary" html-type="submit" :loading="model.loading">查询</AButton><AButton @click="state.reset()">重置</AButton></ASpace>
        </AForm>
        <AAlert v-if="model.failure" class="orch-feedback" type="error" show-icon :message="model.failure" />
        <AAlert v-if="model.actionFailure && !model.downloadOpen && !model.contextOpen" class="orch-feedback" type="error" show-icon :message="model.actionFailure" />
        <AAlert v-if="model.dirty && model.page" class="orch-feedback" type="info" show-icon message="筛选条件已修改，当前仍为上一次查询结果；请点击查询应用条件。" />
        <div class="orch-table-meta log-query-meta">
          <div><h2>任务日志</h2><span class="orch-context-label">{{ model.page ? `本页 ${model.page.items.length} 条 · 快照 ${logTime(model.page.snapshotAt)}` : '默认最近 1 小时，首次进入不会自动跨任务查询' }}</span></div>
          <ASpace wrap><AButton :disabled="!model.page || model.dirty || model.loading" aria-label="刷新日志快照" @click="state.run('refresh')"><template #icon><ReloadOutlined /></template>刷新</AButton><AButton :disabled="!model.canDownload" @click="state.requestDownload()">下载当前结果</AButton><AButton v-if="model.applied?.taskId" :disabled="!model.canDownload" @click="state.requestDownload(true)">下载完整任务日志</AButton></ASpace>
        </div>
        <p v-if="model.applied" class="log-query-summary">{{ logTime(model.applied.from) }} 至 {{ logTime(model.applied.to) }} · {{ model.applied.taskId || '全部有权任务' }} · {{ logSourceLabel[model.applied.source] || '全部来源' }}<template v-if="model.applied.keyword"> · 包含“{{ model.applied.keyword }}”</template> · 已脱敏</p>
        <AAlert v-if="model.page?.truncated" class="orch-feedback" type="warning" show-icon message="已达到 50000 条或 64 MiB 扫描限制，结果已截断，请缩小条件。" />
        <AAlert v-if="model.page?.expired" class="orch-feedback" type="warning" show-icon message="授权范围内存在已按保留策略清理的日志；当前结果不包含已清理内容。" />
        <AAlert v-if="model.page?.incomplete" class="orch-feedback" type="warning" show-icon message="本次扫描发现日志缺口或截断，已按采集事实保留标识。" />
        <OrchOperationalTable :columns="columns" :rows="model.page?.items ?? []" row-key="token" label="脱敏任务日志结果" :loading="model.loading" :minimum-width="850">
          <template #empty><AEmpty :description="emptyMessage" /></template>
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'time'"><time class="log-query-time">{{ logTime(record.receivedAt) }}</time><span class="log-query-secondary">来源时间未取得 · 未标级别</span></template>
            <template v-else-if="column.key === 'task'"><RouterLink class="log-query-task" :to="{ name: 'task-detail', params: { id: record.taskId }, query: { tab: 'logs' } }">{{ record.taskId }}</RouterLink><span class="log-query-secondary">{{ logSourceLabel[record.source] }} · #{{ record.sequence }}</span></template>
            <template v-else-if="column.key === 'message'"><ATag v-if="record.kind !== 'LOG'" color="warning">{{ record.kind === 'GAP' ? '采集缺口' : '已截断' }}{{ record.integrityCode ? ` · ${record.integrityCode}` : '' }}</ATag><div class="log-query-message"><template v-for="(part, index) in logHighlights(logMessagePreview(record.message), model.applied?.keyword ?? '')" :key="index"><mark v-if="part.hit">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></div></template>
            <template v-else-if="column.key === 'actions'"><AButton type="link" :disabled="model.dirty || model.loading" @click="state.openContext(record)">查看上下文</AButton></template>
          </template>
        </OrchOperationalTable>
        <footer v-if="model.page" class="orch-table-footer log-query-footer"><span class="orch-context-label">已扫描 {{ model.page.scannedRecords }} 条 · {{ model.page.truncated ? '请收窄条件' : '数量仅代表已加载结果' }}</span><nav aria-label="日志结果分页"><AButton type="text" aria-label="上一页" :disabled="model.loading || model.dirty || !model.history.length" @click="state.run('previous')"><template #icon><LeftOutlined /></template></AButton><span>第 {{ model.history.length + 1 }} 页</span><AButton type="text" aria-label="下一页" :disabled="model.loading || model.dirty || !model.page.nextCursor" @click="state.run('next')"><template #icon><RightOutlined /></template></AButton></nav></footer>
      </section>
      <p class="log-query-footnote">当前接入 OBDUMPER / OBLOADER 的受控工具输出。采集完整性以实际缺口和保留事实为准；跨来源时间相近不代表因果关系。节点、调度日志将在各自采集与权限接入后提供查询。</p>
    </div>
    <LogContextDrawer :model="model" />
    <AModal :open="model.downloadOpen" title="下载脱敏任务日志" ok-text="确认下载" cancel-text="取消" :confirm-loading="model.downloading" :closable="!model.downloading" :mask-closable="!model.downloading" :keyboard="!model.downloading" @ok="state.download" @cancel="state.cancelDownload">
      <p>{{ model.downloadFullTask ? '下载此任务在快照水位前的全部保留日志，忽略当前时间、来源和关键字筛选。' : '下载本次快照中匹配当前条件的全部脱敏结果，范围不限于本页。' }}</p>
      <p class="log-query-task">任务：{{ model.applied?.taskId || '全部有权任务' }}<br>快照：{{ model.page ? logTime(model.page.snapshotAt) : '' }}<br>已扫描 {{ model.page?.scannedRecords ?? 0 }} 条，完整匹配数量在生成时确定。</p>
      <p v-if="!model.downloadFullTask && model.applied">{{ logTime(model.applied.from) }} 至 {{ logTime(model.applied.to) }} · {{ logSourceLabel[model.applied.source] || '全部来源' }} · 关键字：{{ model.applied.keyword || '无' }}</p>
      <AAlert type="info" show-icon message="UTF-8 脱敏文本，最多 100000 条或 100 MiB。下载生成成功或失败均留审计记录。" />
      <AAlert v-if="model.page?.expired || model.page?.incomplete" type="warning" show-icon message="存在已清理内容或采集缺口，下载不代表原始日志完整。" />
      <AAlert v-if="model.actionFailure" type="error" show-icon :message="model.actionFailure" />
    </AModal>
  </ConfigProvider>
</template>
