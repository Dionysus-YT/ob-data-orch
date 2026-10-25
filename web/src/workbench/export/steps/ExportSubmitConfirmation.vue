<script setup lang="ts">
import { nextTick, ref, toRefs, watch } from 'vue'
import type { ExportSubmitConfirmationModel } from '../exportStepModels'
import { Modal as AModal, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Alert as AAlert, Button as AButton } from 'ant-design-vue'
import { activateModal } from '@/components/modalFocus'

const props = defineProps<{ model: ExportSubmitConfirmationModel }>()
const { submitConfirmationOpen, submitting, closeSubmitConfirmation, submitCancelID, confirmSubmitTask, displayedSource, draftScopeSummary, displayedDraftConfig, dropObject, removeNewline, submissionFailure } = toRefs(props.model)

const cancelButton = ref<{ $el: HTMLButtonElement }>()
let invoker: HTMLElement | null = null
watch(submitConfirmationOpen, open => { if (open) invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null }, { flush: 'sync' })
// 复用产品焦点栈；打开先落在安全返回，关闭和卸载同时释放，忙碌时禁止 Escape 提交之外的退出。
watch([submitConfirmationOpen, cancelButton], async ([open], _previous, cleanup) => {
  if (!open) return
  let disposed = false, release: (() => void) | undefined
  cleanup(() => { disposed = true; release?.() })
  await nextTick()
  const button = cancelButton.value?.$el, root = button?.closest<HTMLElement>('[role="dialog"]')
  if (!disposed && button && root) release = activateModal(root, button, () => { if (!submitting.value) closeSubmitConfirmation.value() }, invoker)
}, { flush: 'post' })
</script>

<template>
  <AModal :open="submitConfirmationOpen" title="确认提交导出任务" :closable="!submitting" :mask-closable="!submitting" :keyboard="!submitting" @cancel="closeSubmitConfirmation">
    <div class="export-submit-facts">
      <p>提交后配置冻结，所选 Agent 领取任务并启动 OBDUMPER；已提交任务不能原地修改参数。</p>
      <ADescriptions class="wizard-submit-facts" size="small" :column="1"><ADescriptionsItem label="数据源">{{ displayedSource?.displayName ?? '当前不可用' }}</ADescriptionsItem><ADescriptionsItem label="导出范围">{{ draftScopeSummary }}</ADescriptionsItem><ADescriptionsItem label="输出位置">{{ displayedDraftConfig?.outputConfig.filePath ?? '未读取' }}</ADescriptionsItem></ADescriptions>
      <AAlert v-if="displayedSource?.environment === 'PRODUCTION' || dropObject || removeNewline" type="warning" show-icon message="请确认生产环境与高风险参数的影响。" :description="[displayedSource?.environment === 'PRODUCTION' ? '生产数据源' : '', dropObject ? 'DDL 包含前置 DROP' : '', removeNewline ? '删除导出数据中的换行' : ''].filter(Boolean).join('；')" />
      <AAlert v-if="submissionFailure" type="error" show-icon :message="submissionFailure" />
    </div>
    <template #footer><AButton :id="submitCancelID" ref="cancelButton" :disabled="submitting" @click="closeSubmitConfirmation">返回检查</AButton><AButton type="primary" :loading="submitting" @click="confirmSubmitTask">确认提交并启动导出</AButton></template>
  </AModal>
</template>
