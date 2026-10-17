<script setup lang="ts">
import { toRefs } from 'vue'
import { Alert, Button, Form, FormItem, Input, Modal } from 'ant-design-vue'
import type { NormalImportSubmitModel } from '../normalImportStepModels'
const props = defineProps<{ model: NormalImportSubmitModel }>()
const { submitConfirmationOpen, submitting, taskNameInput, taskNameError, submitTask, fieldPrefix, reviewFailure, riskDescriptions, selectedSource } = toRefs(props.model)
</script>

<template>
  <Modal :open="submitConfirmationOpen" title="确认提交普通导入任务" :closable="!submitting" :mask-closable="!submitting" :keyboard="!submitting" @cancel="submitConfirmationOpen = false">
    <p>提交将按冻结配置写入目标数据库。任务名称可自定义；留空按目标数据库、文件格式、UTC 时间与任务标识自动生成。</p>
    <Form layout="vertical"><FormItem label="任务名称（可选）" :html-for="fieldPrefix + '-submit-name'" :validate-status="taskNameError ? 'error' : undefined" :help="taskNameError"><Input :id="fieldPrefix + '-submit-name'" v-model:value="taskNameInput" aria-label="任务名称（可选）" :disabled="submitting" placeholder="留空自动生成" /></FormItem></Form>
    <Alert v-if="riskDescriptions.length" type="warning" show-icon message="请确认本次导入的写入与结构影响" :description="riskDescriptions.join('；')" />
    <Alert v-if="selectedSource?.environment === 'PRODUCTION'" type="warning" show-icon message="本次导入将写入生产环境，请确认目标与配置。" />
    <Alert v-if="reviewFailure" type="error" show-icon :message="reviewFailure" />
    <template #footer><Button :disabled="submitting" @click="submitConfirmationOpen = false">返回检查</Button><Button type="primary" :loading="submitting" @click="submitTask(true)">确认提交并启动导入</Button></template>
  </Modal>
</template>
