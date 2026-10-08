<script setup lang="ts">
import { toRefs, watch, onBeforeUnmount } from 'vue'
import type { ExportSubmitConfirmationModel } from '../exportStepModels'
import { Modal as AModal, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Alert as AAlert, Button as AButton } from 'ant-design-vue'

const props = defineProps<{ model: ExportSubmitConfirmationModel }>()
const { submitConfirmationOpen, submitting, closeSubmitConfirmation, submitCancelID, confirmSubmitTask, displayedSource, draftScopeSummary, displayedDraftConfig, dropObject, removeNewline, submissionFailure } = toRefs(props.model)

let focusTimer: ReturnType<typeof setTimeout> | undefined
watch(submitConfirmationOpen, (open) => {
  if (focusTimer) clearTimeout(focusTimer)
  if (open) focusTimer = setTimeout(() => document.getElementById(submitCancelID.value)?.focus(), 280)
})
onBeforeUnmount(() => { if (focusTimer) clearTimeout(focusTimer) })
</script>

<template>
  <AModal :open="submitConfirmationOpen" title="确认提交导出任务" :closable="!submitting" :mask-closable="!submitting" :keyboard="!submitting" @cancel="closeSubmitConfirmation">
    <div class="export-submit-facts">
      <p>提交后配置冻结，所选 Agent 领取任务并启动 OBDUMPER；已提交任务不能原地修改参数。</p>
      <ADescriptions class="export-facts" size="small" :column="1"><ADescriptionsItem label="数据源">{{ displayedSource?.displayName ?? '当前不可用' }}</ADescriptionsItem><ADescriptionsItem label="导出范围">{{ draftScopeSummary }}</ADescriptionsItem><ADescriptionsItem label="输出位置">{{ displayedDraftConfig?.outputConfig.filePath ?? '未读取' }}</ADescriptionsItem></ADescriptions>
      <AAlert v-if="displayedSource?.environment === 'PRODUCTION' || dropObject || removeNewline" type="warning" show-icon message="请确认生产环境与高风险参数的影响。" :description="[displayedSource?.environment === 'PRODUCTION' ? '生产数据源' : '', dropObject ? 'DDL 包含前置 DROP' : '', removeNewline ? '删除导出数据中的换行' : ''].filter(Boolean).join('；')" />
      <AAlert v-if="submissionFailure" type="error" show-icon :message="submissionFailure" />
    </div>
    <template #footer><AButton :id="submitCancelID" :disabled="submitting" @click="closeSubmitConfirmation">返回检查</AButton><AButton type="primary" :loading="submitting" @click="confirmSubmitTask">确认提交并启动导出</AButton></template>
  </AModal>
</template>
