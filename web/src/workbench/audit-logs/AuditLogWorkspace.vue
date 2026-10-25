<script setup lang="ts">
import { computed, reactive } from 'vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { Alert as AAlert, Button as AButton, DatePicker, Empty as AEmpty, Form as AForm, FormItem as AFormItem, Input as AInput, Modal as AModal, Select as ASelect, SelectOption as ASelectOption, Space as ASpace, Tag as ATag, type TableColumnsType } from 'ant-design-vue'
import { LeftOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons-vue'
import { browserApi } from '@/api/browser'
import { auditObjectTypes, type AuditItem } from '@/api/auditLogs'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useAuditLogs } from './useAuditLogs'
import { auditObjectLabels, auditResultLabels } from './presentation'
import { logTime } from '@/workbench/log-query/presentation'
import AuditDetailDrawer from './AuditDetailDrawer.vue'
import './auditLogs.css'
dayjs.extend(utc)
const emit = defineEmits<{ denied: [] }>(), ARangePicker = DatePicker.RangePicker
const state = useAuditLogs(browserApi(), { denied: () => emit('denied'), save: file => { const url = URL.createObjectURL(file), link = document.createElement('a'); link.href = url; link.download = 'audit-logs.log'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 0) } }), model = reactive(state)
const { MessageContext } = useOperationMessage(state.notice)
const range = computed(() => model.filters.from && model.filters.to ? [dayjs.utc(model.filters.from), dayjs.utc(model.filters.to)] as [dayjs.Dayjs, dayjs.Dayjs] : undefined)
function updateRange(value: unknown) { const values = Array.isArray(value) ? value : []; state.setRange(dayjs.isDayjs(values[0]) ? values[0].toISOString() : '', dayjs.isDayjs(values[1]) ? values[1].toISOString() : '') }
const columns: TableColumnsType<AuditItem> = [{ key: 'time', title: '记录时间', width: 185 }, { key: 'actor', title: '操作人', width: 180 }, { key: 'action', title: '操作 / 对象', width: 350 }, { key: 'result', title: '结果', width: 85 }, { key: 'actions', title: '操作', width: 112, fixed: 'right' }]
</script>
<template>
  <MessageContext />
  <p class="orch-context-label">查询当前有权对象的审计事件。查询、详情与下载均记录操作；查看业务详情或操作对象仍需相应授权。</p>
  <section class="orch-management" aria-label="审计日志查询">
    <AForm name="audit-query" :model="model.filters" layout="vertical" class="log-query-filters" @finish="state.run()">
      <AFormItem label="记录时间范围（UTC）" :validate-status="model.errors.time ? 'error' : undefined" :help="model.errors.time" class="log-query-range"><ARangePicker class="log-query-picker" :value="range" :show-time="true" format="YYYY-MM-DD HH:mm:ss" :allow-clear="true" :placeholder="['开始时间', '结束时间']" @change="updateRange" /></AFormItem>
      <AFormItem label="操作人 ID" :validate-status="model.errors.actorId ? 'error' : undefined" :help="model.errors.actorId"><AInput v-model:value="model.filters.actorId" aria-label="操作人 ID" :maxlength="200" allow-clear placeholder="可选，精确匹配" /></AFormItem>
      <AFormItem label="对象类型"><ASelect v-model:value="model.filters.objectType" aria-label="对象类型"><ASelectOption value="">全部有权对象类型</ASelectOption><ASelectOption v-for="type in auditObjectTypes" :key="type" :value="type">{{ auditObjectLabels[type] }}</ASelectOption></ASelect></AFormItem>
      <AFormItem label="对象 ID" :validate-status="model.errors.objectId ? 'error' : undefined" :help="model.errors.objectId"><AInput v-model:value="model.filters.objectId" aria-label="对象 ID" :maxlength="200" allow-clear placeholder="可选，精确匹配" /></AFormItem>
      <AFormItem label="事件类型" :validate-status="model.errors.action ? 'error' : undefined" :help="model.errors.action"><AInput v-model:value="model.filters.action" aria-label="事件类型" :maxlength="100" allow-clear placeholder="可选，例如 TASK_SUBMITTED" /></AFormItem>
      <AFormItem label="操作结果"><ASelect v-model:value="model.filters.result" aria-label="操作结果"><ASelectOption value="">全部结果</ASelectOption><ASelectOption v-for="(label, value) in auditResultLabels" :key="value" :value="value">{{ label }}</ASelectOption></ASelect></AFormItem>
      <AFormItem label="审计文本关键字" :validate-status="model.errors.keyword ? 'error' : undefined" :help="model.errors.keyword"><AInput v-model:value="model.filters.keyword" aria-label="审计文本关键字" :maxlength="256" allow-clear placeholder="普通文本包含" /></AFormItem>
      <ASpace class="log-query-submit"><AButton type="primary" html-type="submit" :loading="model.loading">查询</AButton><AButton @click="state.reset">重置</AButton></ASpace>
    </AForm>
    <AAlert v-if="model.failure" class="orch-feedback" type="error" show-icon :message="model.failure" />
    <AAlert v-if="model.actionFailure" class="orch-feedback" type="error" show-icon :message="model.actionFailure" />
    <AAlert v-if="model.dirty && model.page" class="orch-feedback" type="info" show-icon message="筛选条件已修改，当前仍为上一次查询结果；请点击查询应用条件。" />
    <div class="orch-table-meta log-query-meta"><div><h2>审计日志</h2><span class="orch-context-label">{{ model.page ? `本页 ${model.page.items.length} 条 · 快照 ${logTime(model.page.snapshotAt)}` : '默认最近 1 小时，首次进入不会自动查询' }}</span></div><ASpace wrap><AButton :disabled="model.dirty || model.loading" aria-label="刷新审计快照" @click="state.run('refresh')"><template #icon><ReloadOutlined /></template>刷新</AButton><AButton :disabled="!model.canDownload" @click="state.requestDownload">下载当前结果</AButton></ASpace></div>
    <p v-if="model.applied" class="log-query-summary">{{ logTime(model.applied.from) }} 至 {{ logTime(model.applied.to) }} · {{ model.applied.actorId || '全部有权操作人' }} · {{ auditObjectLabels[model.applied.objectType] || '全部有权对象类型' }} {{ model.applied.objectId }} · {{ model.applied.action || '全部事件' }} · {{ auditResultLabels[model.applied.result as keyof typeof auditResultLabels] || '全部结果' }} · {{ model.applied.keyword ? '已应用关键字' : '无关键字' }} · 已脱敏</p>
    <AAlert v-if="model.page?.truncated" class="orch-feedback" type="warning" show-icon message="已达到 50000 条或 64 MiB 扫描限制，请缩小条件；当前快照不能下载。" />
    <OrchOperationalTable :columns="columns" :rows="model.page?.items ?? []" row-key="id" label="审计日志结果" :loading="model.loading" :minimum-width="850">
      <template #empty><AEmpty :description="!model.page ? '选择时间与条件后查询审计日志' : '当前有权范围与条件内没有匹配事件'" /></template>
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'time'"><time class="log-query-time">{{ logTime(record.occurredAt) }}</time><span class="log-query-secondary">平台审计事件</span></template>
        <template v-else-if="column.key === 'actor'"><span class="log-query-task">{{ record.actorId }}</span><span class="log-query-secondary">{{ record.actorType }}</span></template>
        <template v-else-if="column.key === 'action'"><span class="log-query-task">{{ record.action }}</span><span class="log-query-secondary">{{ auditObjectLabels[record.objectType] }} · {{ record.objectId || '未关联具体对象' }}</span></template>
        <template v-else-if="column.key === 'result'"><ATag :color="record.result === 'SUCCEEDED' ? 'success' : record.result === 'FAILED' ? 'error' : 'warning'">{{ auditResultLabels[record.result as keyof typeof auditResultLabels] }}</ATag></template>
        <template v-else-if="column.key === 'actions'"><AButton type="link" :disabled="model.dirty || model.loading" @click="state.openDetail(record)">查看详情</AButton></template>
      </template>
    </OrchOperationalTable>
    <footer v-if="model.page" class="orch-table-footer log-query-footer"><span class="orch-context-label">已扫描 {{ model.page.scannedRecords }} 条 · 数量仅代表已加载结果</span><nav aria-label="审计结果分页"><AButton type="text" aria-label="上一页" :disabled="model.loading || model.dirty || !model.history.length" @click="state.run('previous')"><template #icon><LeftOutlined /></template></AButton><span>第 {{ model.history.length + 1 }} 页</span><AButton type="text" aria-label="下一页" :disabled="model.loading || model.dirty || !model.page.nextCursor" @click="state.run('next')"><template #icon><RightOutlined /></template></AButton></nav></footer>
  </section>
  <p class="log-query-footnote">当前可查任务、数据源、节点、导出草稿和平台配置等审计事件。部分事件类型与历史变更详情暂未接入；审计保留期限未配置，无法承诺历史记录完整。</p>
  <AuditDetailDrawer :model="model" />
  <AModal :open="model.downloadOpen" title="下载脱敏审计日志" ok-text="确认下载" cancel-text="取消" :confirm-loading="model.downloading" :closable="!model.downloading" :mask-closable="!model.downloading" :keyboard="!model.downloading" @ok="state.download" @cancel="state.cancelDownload">
    <p>下载本次快照中匹配条件的全部可展示记录，范围不限于本页。生成前再次校验当前权限；成功与失败均记录审计。</p>
    <p v-if="model.applied">{{ logTime(model.applied.from) }} 至 {{ logTime(model.applied.to) }} · {{ model.applied.actorId || '全部有权操作人' }} · {{ auditObjectLabels[model.applied.objectType] || '全部有权对象类型' }} {{ model.applied.objectId }}</p>
    <p>快照：{{ model.page ? logTime(model.page.snapshotAt) : '' }} · 本页 {{ model.page?.items.length ?? 0 }} 条，完整匹配数量在生成时确定。</p>
    <AAlert type="info" show-icon message="UTF-8 脱敏文本；扫描最多 50000 条或 64 MiB，文件最多 100000 条或 100 MiB。超过任一限制要求缩小条件，不返回部分文件。" />
  </AModal>
</template>
