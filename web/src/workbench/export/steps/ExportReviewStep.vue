<script setup lang="ts">
import { computed, toRefs } from 'vue'
import type { ExportReviewStepModel } from '../exportStepModels'
import { Alert as AAlert, Spin as ASpin, Button as AButton, Table as ATable, Badge as ABadge, type TableColumnsType } from 'ant-design-vue'
import { precheckCheckLabel, precheckFixAction, precheckResultDetail, precheckResultLabel, precheckResultBlocksSubmission, precheckResultBlocked, type PrecheckRow } from '../exportPrecheckPresentation'

const props = defineProps<{ model: ExportReviewStepModel }>()
const { fieldPrefix, activePrecheck, draftDirty, draftLoadFailure, loadCreatedDraft, currentDraft, displayedDraftConfig, storageOutput, precheckWaitingForResults, precheckGroups, precheckOverview, precheckExpired, commandPreview, copyCommand, previewingCommand, commandPreviewFailure, loadCommandPreview, copyFailure, submissionFailure, moveToStep } = toRefs(props.model)
type PrecheckTableRow = PrecheckRow & { groupTitle: string; label: string; fixAction: ReturnType<typeof precheckFixAction> }
const parameterChecks = computed(() => Boolean(displayedDraftConfig.value?.filterConfig?.weakRead || displayedDraftConfig.value?.filterConfig?.enableHiddenPk || displayedDraftConfig.value?.ddlBehavior?.addExtraMessage))
// 表格只展开现有展示分组，不复制报告状态或推断检查依赖。
const tableRows = computed<PrecheckTableRow[]>(() => precheckGroups.value.flatMap(group => group.rows.map(row => ({ ...row, groupTitle: group.title, label: precheckCheckLabel(row.check, storageOutput.value, displayedDraftConfig.value?.objectScope.scopeKind, parameterChecks.value), fixAction: precheckFixAction(row) }))))
const hasTemporalFormats = computed(() => Object.values(displayedDraftConfig.value?.dataFormat?.timestampFormats ?? {}).some(value => typeof value === 'string' && value.length > 0))
const resultsStale = computed(() => precheckExpired.value || ['INVALIDATED', 'EXPIRED'].includes(activePrecheck.value?.status ?? ''))
const columns: TableColumnsType<PrecheckTableRow> = [
  { title: '预检查项', key: 'check', width: 180 },
  { title: '状态', key: 'status', width: 155 },
  { title: '检查结果', key: 'result' },
  { title: '操作', key: 'action', width: 160 },
]
</script>

<template>
  <section class="form-section">
    <h2>预检查与提交确认</h2>
    <section class="precheck-list" aria-label="预检查结果">
      <p class="section-hint">进入本页后自动检查；独立项目继续执行，依赖项说明阻断原因。修正配置并保存后会重新检查。</p>
      <AAlert v-if="hasTemporalFormats" type="info" show-icon message="日期时间格式已完成参数校验；实际序列化结果需要导出时确认。" />
      <AAlert :type="precheckOverview.type" show-icon :message="precheckOverview.message" :description="precheckOverview.description">
        <template #action>
          <AButton v-if="draftDirty || !currentDraft && !draftLoadFailure" type="link" @click="moveToStep(4)">返回执行与输出</AButton>
          <AButton v-else-if="draftLoadFailure" type="link" @click="loadCreatedDraft()">重新读取草稿</AButton>
          <AButton v-else-if="commandPreviewFailure" type="link" @click="loadCommandPreview()">重新生成命令</AButton>
        </template>
      </AAlert>
      <AAlert v-if="displayedDraftConfig?.objectScope.scopeKind === 'QUERY_RESULT'" type="info" show-icon message="结果集预检查只确认连接" description="对象访问项是数据库级连接投影；不会执行查询 SQL，也不能证明被引用对象、语法和权限有效。这些由 OBDUMPER 执行时确认。" />
      <ATable class="wizard-precheck-table" :columns="columns" :data-source="tableRows" row-key="check" :pagination="false" :scroll="{ x: 740 }" size="small">
        <template #bodyCell="{ column, record: row }">
          <div v-if="column.key === 'check'" class="wizard-precheck-identity">
            <strong>{{ row.label }}</strong>
            <small class="wizard-precheck-category">{{ row.groupTitle }}</small>
          </div>
          <ABadge v-else-if="column.key === 'status'" :status="resultsStale ? 'warning' : precheckWaitingForResults ? 'processing' : !row.result ? 'default' : row.result.status === 'PASSED' ? 'success' : row.result.status === 'FAILED' ? 'error' : 'warning'" :text="resultsStale ? '已失效' : !row.result && activePrecheck?.status === 'FAILED' ? '未返回结果' : precheckResultLabel(row.result, precheckWaitingForResults)" />
          <div v-else-if="column.key === 'result'" class="wizard-precheck-result">
            <span>{{ precheckResultDetail(row.result, storageOutput, displayedDraftConfig?.objectScope.scopeKind) || '—' }}</span>
            <small v-if="precheckResultBlocksSubmission(row.result)" class="wizard-precheck-reason">{{ precheckResultBlocked(row.result) ? '依赖原因码' : '原因码' }}：<code>{{ row.result?.evidenceCode }}</code></small>
          </div>
          <template v-else-if="column.key === 'action'">
            <AButton v-if="precheckResultBlocksSubmission(row.result)" type="link" size="small" @click="moveToStep(row.fixAction.step)">{{ row.fixAction.label }}</AButton>
            <span v-else>—</span>
          </template>
        </template>
      </ATable>
    </section>
    <section class="wizard-command" :aria-labelledby="`${fieldPrefix}-command-heading`">
      <div class="wizard-section-heading">
        <h3 :id="`${fieldPrefix}-command-heading`">完整命令（敏感信息已脱敏）</h3>
        <AButton :disabled="!commandPreview" @click="copyCommand">复制脱敏命令</AButton>
      </div>
      <ASpin v-if="previewingCommand" tip="正在生成命令预览…"><span class="wizard-loading-space" /></ASpin>
      <pre v-else-if="commandPreview"><code>{{ commandPreview.command }}</code></pre>
      <p v-else>命令预览尚未准备好。</p>
      <p v-if="commandPreview">密码占位已脱敏，复制的命令仅供核对，不能直接执行。</p>
      <p v-if="commandPreview">草稿版本 rev-{{ currentDraft?.revision }} · 配置指纹 {{ commandPreview.configFingerprint }}</p>
      <AAlert v-if="copyFailure" type="error" show-icon :message="copyFailure" />
    </section>
    <AAlert v-if="submissionFailure" type="error" show-icon :message="submissionFailure" />
  </section>
</template>
