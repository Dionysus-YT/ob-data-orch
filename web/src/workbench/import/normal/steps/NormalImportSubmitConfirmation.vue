<script setup lang="ts">
import { computed, nextTick, ref, toRefs, watch } from 'vue'
import { Alert, Button, Descriptions, DescriptionsItem, Form, FormItem, Input, Modal } from 'ant-design-vue'
import type { NormalImportSubmitModel } from '../normalImportStepModels'
import { activateModal } from '@/components/modalFocus'
const props = defineProps<{ model: NormalImportSubmitModel }>()
const { submitConfirmationOpen, submitting, taskNameInput, taskNameError, submitTask, fieldPrefix, reviewFailure, riskDescriptions, selectedSource, selectedNode, currentDraft } = toRefs(props.model)
const config = computed(() => currentDraft.value?.config)
const scope = computed(() => !config.value ? '未读取' : config.value.scope === 'ALL' ? '全部对象' : Object.entries(config.value.objects ?? {}).map(([type, value]) => `${type}：${value}`).join('；'))
const cancelButton = ref<{ $el: HTMLButtonElement }>()
let invoker: HTMLElement | null = null
watch(submitConfirmationOpen, open => { if (open) invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null }, { flush: 'sync' })
// 确认事实读取冻结草稿，使用现有焦点栈把安全返回设为初始焦点并恢复触发器。
watch([submitConfirmationOpen, cancelButton], async ([open], _previous, cleanup) => {
  if (!open) return
  let disposed = false, release: (() => void) | undefined
  cleanup(() => { disposed = true; release?.() })
  await nextTick()
  const button = cancelButton.value?.$el, root = button?.closest<HTMLElement>('[role="dialog"]')
  if (!disposed && button && root) release = activateModal(root, button, () => { if (!submitting.value) submitConfirmationOpen.value = false }, invoker)
}, { flush: 'post' })
</script>

<template>
  <Modal :open="submitConfirmationOpen" title="确认提交普通导入任务" :closable="!submitting" :mask-closable="!submitting" :keyboard="!submitting" @cancel="submitConfirmationOpen = false">
    <div class="orch-confirmation-body">
      <p>提交将按冻结配置写入目标数据库。任务名称可自定义；留空按目标数据库、文件格式、UTC 时间与任务标识自动生成。</p>
      <Descriptions class="wizard-submit-facts" size="small" :column="1">
        <DescriptionsItem label="目标数据源">{{ selectedSource?.displayName ?? '当前不可用' }}</DescriptionsItem>
        <DescriptionsItem label="目标数据库 / Schema"><code>{{ config?.database ?? '未读取' }}</code></DescriptionsItem>
        <DescriptionsItem label="对象范围">{{ scope }}</DescriptionsItem>
        <DescriptionsItem label="执行节点">{{ selectedNode?.displayName ?? '当前不可用' }}</DescriptionsItem>
        <DescriptionsItem label="输入路径"><code>{{ config?.filePath ?? '未读取' }}</code></DescriptionsItem>
      </Descriptions>
      <Form layout="vertical"><FormItem label="任务名称（可选）" :html-for="fieldPrefix + '-submit-name'" :validate-status="taskNameError ? 'error' : undefined" :help="taskNameError"><Input :id="fieldPrefix + '-submit-name'" v-model:value="taskNameInput" aria-label="任务名称（可选）" :disabled="submitting" placeholder="留空自动生成" /></FormItem></Form>
      <Alert v-if="riskDescriptions.length" type="warning" show-icon message="请确认本次导入的写入与结构影响" :description="riskDescriptions.join('；')" />
      <Alert v-if="selectedSource?.environment === 'PRODUCTION'" type="warning" show-icon message="本次导入将写入生产环境，请确认目标与配置。" />
      <Alert v-if="reviewFailure" type="error" show-icon :message="reviewFailure" />
    </div>
    <template #footer><Button ref="cancelButton" :disabled="submitting" @click="submitConfirmationOpen = false">返回检查</Button><Button type="primary" :loading="submitting" @click="submitTask(true)">确认提交并启动导入</Button></template>
  </Modal>
</template>
