<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { Button, Drawer, Step, Steps } from 'ant-design-vue'
import OrchTaskStepRail from './OrchTaskStepRail.vue'
import { product } from '@/platform/tokens'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'

const props = defineProps<{ kind: 'export' | 'normal' | 'direct'; activeStep?: number }>()
const emit = defineEmits<{ stepChange: [step: number] }>()
const summaryOpen = ref(false)
const summaryTitleId = useId()
const { heading: summaryHeading, afterOpenChange: afterSummaryOpenChange, captureInvoker: captureSummaryInvoker } = useAntDrawerDialog(summaryTitleId, () => { summaryOpen.value = false })
void summaryHeading
function openSummary() {
  captureSummaryInvoker()
  summaryOpen.value = true
}
function selectVisitedStep(index: number) {
  if (props.kind === 'export' && index + 1 < (props.activeStep ?? 1)) emit('stepChange', index + 1)
}

const steps = computed(() => {
  if (props.kind === 'export') return ['选择数据源', '导出内容与对象', '数据格式', '执行与输出', '预检查与命令']
  if (props.kind === 'normal') return ['选择数据源', '文件来源', '内容与格式', '对象与映射', '执行参数', '预检查与命令']
  return ['适用条件', '连接与版本', '单表文件', '表与格式', '执行参数', '预检查与命令']
})
</script>

<template>
  <div class="orch-task-builder" :class="{ 'orch-export-builder': kind === 'export' }">
    <Steps v-if="kind === 'export'" class="export-ant-step-rail" direction="horizontal" :responsive="false" size="small" :current="(activeStep ?? 1) - 1" aria-label="任务创建步骤" @change="selectVisitedStep">
      <Step v-for="(step, index) in steps" :key="step" :title="step" :description="index + 1 === (activeStep ?? 1) ? '当前步骤' : index + 1 < (activeStep ?? 1) ? '已访问' : '待配置'" :status="index + 1 === (activeStep ?? 1) ? 'process' : 'wait'" :disabled="index + 1 >= (activeStep ?? 1)" :aria-current="index + 1 === (activeStep ?? 1) ? 'step' : undefined" />
    </Steps>
    <OrchTaskStepRail v-else :steps="steps" :active-step="activeStep ?? 1" />
    <div class="orch-task-decision"><div class="orch-task-context"><slot name="actions" /><Button @click="openSummary">查看任务摘要</Button></div><section class="orch-task-workspace"><slot /></section></div>
    <div class="orch-task-actions"><slot name="footer"><footer class="wizard-footer"><Button disabled aria-describedby="import-disabled-reason">上一步</Button><span id="import-disabled-reason" class="wizard-baseline-note">当前导入流程尚未开放草稿保存与任务提交。</span><span class="footer-grow" /><Button type="primary" disabled aria-describedby="import-disabled-reason">下一步</Button></footer></slot></div>
    <Drawer :open="summaryOpen" :width="product.inspector.width" root-class-name="orch-inspector" @close="summaryOpen = false" @after-open-change="afterSummaryOpenChange">
      <template #title><h2 :id="summaryTitleId" ref="summaryHeading" class="ant-drawer-title" tabindex="-1">任务摘要</h2></template>
      <div class="orch-task-summary"><slot name="summary" /></div>
    </Drawer>
  </div>
</template>

<style scoped>
.orch-export-builder { grid-template-columns: minmax(0, 1fr); gap: var(--ob-foundation-space-4); }
.orch-export-builder .orch-task-actions { grid-column: 1; }
.export-ant-step-rail { min-inline-size: 0; overflow-x: auto; padding: var(--ob-foundation-space-3) 0; }
.export-ant-step-rail :deep(.ant-steps-item) { min-inline-size: 160px; }
</style>
