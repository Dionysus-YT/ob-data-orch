<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Badge, Button, Table, type TableColumnsType } from 'ant-design-vue'
import { importPrecheckItems, normalImportTitles } from '../normalImportOptions'
import { normalImportPrecheckPresentation } from '../normalImportPrecheckPresentation'
import type { NormalImportReviewModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportReviewModel }>()
const { fieldPrefix, issues, moveToStep, activePrecheck, discoveredFiles, checkExpired, commandPreview, previewingCommand, copyingCommand, copyCommand } = toRefs(props.model)
const rows = computed(() => importPrecheckItems.map(item => {
  const check = activePrecheck.value ?? (item.check === 'INPUT_FILES' ? discoveredFiles.value : null)
  const result = check?.results.find(result => result.check === item.check)
  return { ...item, ...normalImportPrecheckPresentation(check, result, checkExpired.value) }
}))
const columns: TableColumnsType = [{ title: '预检查项', key: 'name', dataIndex: 'name', width: 150 }, { title: '状态', key: 'status', width: 120 }, { title: '检查内容', key: 'description', dataIndex: 'description' }, { title: '操作', key: 'action', width: 100 }]
</script>

<template>
  <section class="form-section normal-import-section">
    <h2>预检查与完整命令</h2>
    <Alert v-if="issues.length" type="warning" show-icon :message="`本页配置有 ${issues.length} 项待补齐`">
      <template #description><ul class="normal-import-issue-list"><li v-for="issue in issues" :key="issue.step + '-' + issue.field"><Button type="link" @click="moveToStep(issue.step)">{{ normalImportTitles[issue.step - 1] }}：{{ issue.message }}</Button></li></ul></template>
    </Alert>
    <Alert type="info" show-icon :message="activePrecheck?.executionEnabled ? '提交要求当前完整预检查全部通过' : '普通导入执行暂未开放'" :description="activePrecheck?.executionEnabled ? '进入本步后自动生成命令并预检查。所有检查通过且当前配置与检查一致时，才能提交。' : '进入本步后自动生成命令并进行只读预检查。当前尚不能启动 OBLOADER；各项检查的结果与原因显示在下表。'" />
    <section aria-label="普通导入任务预检查">
      <Table :columns="columns" :data-source="rows" row-key="name" :pagination="false" :scroll="{ x: 600 }" size="small">
        <template #title><span class="orch-sr-only">普通导入任务预检查</span></template>
        <template #bodyCell="{ column, record }">
          <Badge v-if="column.key === 'status'" :status="record.status" :text="record.text" />
          <template v-else-if="column.key === 'description'"><span>{{ record.description }}</span><p v-if="record.detail" class="section-hint normal-import-check-detail">{{ record.detail }}</p></template>
          <Button v-else-if="column.key === 'action'" type="link" @click="moveToStep(record.step)">查看配置</Button>
        </template>
      </Table>
    </section>
    <section class="normal-import-command" aria-label="完整脱敏命令">
      <div class="normal-import-section-heading"><h3>完整命令（脱敏）</h3><Button :loading="copyingCommand" :disabled="!commandPreview" @click="copyCommand">复制脱敏命令</Button></div>
      <pre v-if="commandPreview">{{ commandPreview.redactedCommand }}</pre>
      <p v-else :id="fieldPrefix + '-command-reason'">{{ previewingCommand ? '正在自动生成完整脱敏命令…' : '配置完整后自动生成完整脱敏命令并开始预检查。' }}</p>
    </section>
  </section>
</template>
