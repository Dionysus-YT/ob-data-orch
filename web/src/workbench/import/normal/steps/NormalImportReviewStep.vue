<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Badge, Button, Spin, Table, type TableColumnsType } from 'ant-design-vue'
import { importPrecheckItems, normalImportTitles } from '../normalImportOptions'
import { normalImportPrecheckPresentation, normalImportPrecheckFixAction } from '../normalImportPrecheckPresentation'
import type { NormalImportReviewModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportReviewModel }>()
const { fieldPrefix, issues, moveToStep, rerunLocked, activePrecheck, discoveredFiles, checkExpired, commandPreview, previewingCommand, copyingCommand, copyCommand, copyFailure, precheckOverview } = toRefs(props.model)
const rows = computed(() => importPrecheckItems.map(item => {
  const check = activePrecheck.value ?? (item.check === 'INPUT_FILES' ? discoveredFiles.value : null)
  const result = check?.results.find(result => result.check === item.check)
  const groupTitle = ['TOOL_ENVIRONMENT', 'NODE_RESOURCE'].includes(item.check) ? '执行环境' : ['INPUT_FILES', 'INPUT_MAPPING'].includes(item.check) ? '输入与映射' : '数据库与对象'
  return { ...item, result, groupTitle, fixAction: normalImportPrecheckFixAction(item.check, result), ...normalImportPrecheckPresentation(check, result, checkExpired.value) }
}))
const columns = computed<TableColumnsType>(() => [{ title: '预检查项', key: 'name', width: 180 }, { title: '状态', key: 'status', width: 155 }, { title: '检查结果', key: 'description' }, ...(rerunLocked.value ? [] : [{ title: '操作', key: 'action', width: 160 }])])
</script>

<template>
  <section class="form-section normal-import-section">
    <h2>预检查与提交确认</h2>
    <Alert v-if="issues.length" type="warning" show-icon :message="`本页配置有 ${issues.length} 项待补齐`">
      <template #description><ul class="normal-import-issue-list"><li v-for="issue in issues" :key="issue.step + '-' + issue.field"><span v-if="rerunLocked">{{ normalImportTitles[issue.step - 1] }}：{{ issue.message }}</span><Button v-else type="link" @click="moveToStep(issue.step)">{{ normalImportTitles[issue.step - 1] }}：{{ issue.message }}</Button></li></ul></template>
    </Alert>
    <section class="precheck-list" aria-label="普通导入任务预检查">
      <p class="section-hint">{{ rerunLocked ? '本次保留原配置并重新预检查；需修改配置时，请从来源任务选择“基于原配置新建”。' : '进入本页后自动检查；独立项目继续执行，依赖项说明阻断原因。修正配置并保存后会重新检查。' }}</p>
      <Alert :type="precheckOverview.type" show-icon :message="precheckOverview.message" :description="precheckOverview.description" />
      <Table class="wizard-precheck-table" :columns="columns" :data-source="rows" row-key="name" :pagination="false" :scroll="{ x: 740 }" size="small">
        <template #bodyCell="{ column, record }">
          <div v-if="column.key === 'name'" class="wizard-precheck-identity"><strong>{{ record.name }}</strong><small class="wizard-precheck-category">{{ record.groupTitle }}</small></div>
          <Badge v-else-if="column.key === 'status'" :status="record.status" :text="record.text" />
          <div v-else-if="column.key === 'description'" class="wizard-precheck-result"><span>{{ record.detail || record.description }}</span><small v-if="record.result && record.result.status !== 'PASSED' && record.status !== 'processing'" class="wizard-precheck-reason">{{ record.text === '前置阻断，未执行' ? '依赖原因码' : '原因码' }}：<code>{{ record.result.evidenceCode }}</code></small></div>
          <template v-else-if="column.key === 'action'"><Button v-if="record.status === 'error' || record.status === 'warning'" type="link" size="small" @click="moveToStep(record.fixAction.step)">{{ record.fixAction.label }}</Button><span v-else>—</span></template>
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
