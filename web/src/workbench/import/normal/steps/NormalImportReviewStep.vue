<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Badge, Button, Spin, Table, type TableColumnsType } from 'ant-design-vue'
import { importPrecheckItems, normalImportTitles } from '../normalImportOptions'
import { normalImportPrecheckPresentation, normalImportPrecheckFixAction } from '../normalImportPrecheckPresentation'
import type { NormalImportReviewModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportReviewModel }>()
const { fieldPrefix, issues, moveToStep, goToNode, rerunLocked, activePrecheck, discoveredFiles, checkExpired, commandPreview, previewingCommand, copyingCommand, copyCommand, copyFailure, precheckOverview } = toRefs(props.model)
const rows = computed(() => importPrecheckItems.map(item => {
  const check = activePrecheck.value ?? (item.check === 'INPUT_FILES' ? discoveredFiles.value : null)
  const result = check?.results.find(result => result.check === item.check)
  const groupTitle = ['TOOL_ENVIRONMENT', 'NODE_RESOURCE'].includes(item.check) ? '执行环境' : ['INPUT_FILES', 'INPUT_MAPPING'].includes(item.check) ? '输入与映射' : '数据库与对象'
  const currentEvidence = check && !checkExpired.value && !['PENDING', 'LEASED', 'INVALIDATED', 'EXPIRED'].includes(check.status) ? result?.evidenceCode : undefined
  return { ...item, result, groupTitle, currentEvidence, fixAction: normalImportPrecheckFixAction(check, result, checkExpired.value), ...normalImportPrecheckPresentation(check, result, checkExpired.value) }
}))
const needsNodeSetup = computed(() => rows.value.some(row => row.currentEvidence === 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED'))
const columns = computed<TableColumnsType>(() => [{ title: '预检查项', key: 'name', width: 180 }, { title: '状态', key: 'status', width: 155 }, { title: '检查结果', key: 'description' }, ...(rerunLocked.value ? [] : [{ title: '操作', key: 'action', width: 160 }])])
</script>

<template>
  <section class="form-section normal-import-section">
    <h2>预检查与提交确认</h2>
    <Alert v-if="issues.length" type="warning" show-icon :message="`本页配置有 ${issues.length} 项待补齐`">
      <template #description><ul class="normal-import-issue-list"><li v-for="issue in issues" :key="issue.step + '-' + issue.field"><span v-if="rerunLocked">{{ normalImportTitles[issue.step - 1] }}：{{ issue.message }}</span><Button v-else type="link" @click="moveToStep(issue.step)">{{ normalImportTitles[issue.step - 1] }}：{{ issue.message }}</Button></li></ul></template>
    </Alert>
    <section class="precheck-list" aria-label="普通导入任务预检查">
      <Alert :id="fieldPrefix + '-precheck-status'" :type="precheckOverview.type" show-icon :message="precheckOverview.message" :description="needsNodeSetup ? undefined : precheckOverview.description">
        <template v-if="needsNodeSetup" #description>{{ precheckOverview.description }} 当前节点尚未启用数据库检查。<code>NORMAL_IMPORT_PROBE_NOT_CONFIGURED</code></template>
        <template v-if="needsNodeSetup" #action><Button type="link" @click="goToNode">查看执行节点</Button></template>
      </Alert>
      <Table class="wizard-precheck-table" :columns="columns" :data-source="rows" row-key="name" :pagination="false" :scroll="{ x: 740 }" size="small">
        <template #bodyCell="{ column, record }">
          <div v-if="column.key === 'name'" class="wizard-precheck-identity"><strong>{{ record.name }}</strong><small class="wizard-precheck-category">{{ record.groupTitle }}</small></div>
          <Badge v-else-if="column.key === 'status'" :status="record.status" :text="record.text" />
          <div v-else-if="column.key === 'description'" class="wizard-precheck-result"><span>{{ record.detail || record.description }}</span><small v-if="record.currentEvidence && record.result.status !== 'PASSED' && record.currentEvidence !== 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED'" class="wizard-precheck-reason">{{ record.text === '前置阻断，未执行' ? '依赖原因码' : '原因码' }}：<code>{{ record.currentEvidence }}</code></small></div>
          <template v-else-if="column.key === 'action'"><Button v-if="record.fixAction && !(needsNodeSetup && record.fixAction.target === 'NODE')" type="link" size="small" @click="record.fixAction.target === 'NODE' ? goToNode() : moveToStep(record.fixAction.step)">{{ record.fixAction.label }}</Button><span v-else>—</span></template>
        </template>
      </Table>
    </section>
    <section class="wizard-command" aria-label="完整脱敏命令">
      <div class="wizard-section-heading"><h3>完整命令（敏感信息已脱敏）</h3><Button :loading="copyingCommand" :disabled="!commandPreview" @click="copyCommand">复制脱敏命令</Button></div>
      <Spin v-if="previewingCommand" tip="正在生成命令预览…"><span class="wizard-loading-space" /></Spin>
      <pre v-else-if="commandPreview"><code>{{ commandPreview.redactedCommand }}</code></pre>
      <p v-else :id="fieldPrefix + '-command-reason'">命令预览尚未准备好。</p>
      <p v-if="commandPreview">密码占位已脱敏，复制的命令仅供核对，不能直接执行。</p>
      <Alert v-if="copyFailure" type="error" show-icon :message="copyFailure" />
    </section>
  </section>
</template>
