<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { Button, Drawer, Step, Steps } from 'ant-design-vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'

const props = defineProps<{ kind: 'export' | 'normal' | 'direct'; activeStep?: number; progressPercent?: number; stepTitles?: readonly string[]; completedSteps?: readonly number[]; navigationLocked?: boolean }>()
const emit = defineEmits<{ stepChange: [step: number] }>()
const summaryOpen = ref(false)
const stepRail = ref<InstanceType<typeof Steps>>()
const summaryTitleId = useId()
const { heading: summaryHeading, afterOpenChange: afterSummaryOpenChange, captureInvoker: captureSummaryInvoker } = useAntDrawerDialog(summaryTitleId, () => { summaryOpen.value = false }, undefined, () => summaryOpen.value)
void summaryHeading
function openSummary() {
  captureSummaryInvoker()
  summaryOpen.value = true
}
function selectVisitedStep(index: number) {
  if (!props.navigationLocked && index + 1 < (props.activeStep ?? 1)) emit('stepChange', index + 1)
}
function completed(index: number) { return props.completedSteps?.includes(index + 1) ?? index + 1 < (props.activeStep ?? 1) }
// 窄屏只滚动步骤栏自身，使新进入的步骤可见，不带动正文或改变全局导航。
watch(() => props.activeStep, async () => {
  await nextTick()
  const rail = stepRail.value?.$el as HTMLElement | undefined
  const current = rail?.querySelector<HTMLElement>('[aria-current="step"]')
  if (!rail || !current) return
  const bounds = rail.getBoundingClientRect()
  const target = current.getBoundingClientRect()
  if (target.left < bounds.left) rail.scrollLeft += target.left - bounds.left
  else if (target.right > bounds.right) rail.scrollLeft += target.right - bounds.right
}, { immediate: true, flush: 'post' })

const steps = computed(() => {
  if (props.kind === 'export') return ['选择数据源', '导出内容与对象', '数据格式', '执行与输出', '预检查与命令']
  return props.stepTitles ?? []
})
</script>

<template>
  <div class="orch-task-builder orch-horizontal-builder">
    <Steps ref="stepRail" class="wizard-ant-step-rail" :class="{ 'export-ant-step-rail': kind === 'export' }" direction="horizontal" :responsive="false" :current="(activeStep ?? 1) - 1" :percent="progressPercent ?? 0" aria-label="任务创建步骤" @change="selectVisitedStep">
      <Step v-for="(step, index) in steps" :key="step" :title="step" :description="index + 1 === (activeStep ?? 1) ? `当前步骤 · ${progressPercent ?? 0}%` : index + 1 < (activeStep ?? 1) ? completed(index) ? '已配置' : '待完善' : '待配置'" :status="index + 1 === (activeStep ?? 1) ? 'process' : index + 1 < (activeStep ?? 1) && completed(index) ? 'finish' : 'wait'" :disabled="navigationLocked || index + 1 >= (activeStep ?? 1)" :aria-current="index + 1 === (activeStep ?? 1) ? 'step' : undefined" :aria-label="!navigationLocked && index + 1 < (activeStep ?? 1) ? `返回第 ${index + 1} 步：${step}` : undefined" />
    </Steps>
    <div class="orch-task-decision"><div class="orch-task-context"><slot name="actions" /><Button @click="openSummary">查看任务摘要</Button></div><section class="orch-task-workspace"><slot /></section></div>
    <div class="orch-task-actions"><slot name="footer"><footer class="wizard-footer"><Button disabled aria-describedby="import-disabled-reason">上一步</Button><span id="import-disabled-reason" class="wizard-baseline-note">当前导入流程尚未开放草稿保存与任务提交。</span><span class="footer-grow" /><Button type="primary" disabled aria-describedby="import-disabled-reason">下一步</Button></footer></slot></div>
    <Drawer :open="summaryOpen" width="min(100vw, var(--ob-product-inspector-width))" :body-style="{ padding: 0 }" root-class-name="orch-inspector" @close="summaryOpen = false" @after-open-change="afterSummaryOpenChange">
      <template #title><h2 :id="summaryTitleId" ref="summaryHeading" class="ant-drawer-title" tabindex="-1">任务摘要</h2></template>
      <div class="orch-task-summary"><slot name="summary" /></div>
    </Drawer>
  </div>
</template>

<style scoped>
.orch-horizontal-builder { grid-template-columns: minmax(0, 1fr); gap: var(--ob-foundation-space-4); }
.orch-horizontal-builder .orch-task-actions { grid-column: 1; }
.wizard-ant-step-rail { min-inline-size: 0; overflow-x: auto; padding: var(--ob-foundation-space-3) 0; }
.wizard-ant-step-rail :deep(.ant-steps-item) { min-inline-size: 180px; }
</style>
