<script setup lang="ts">
import { nextTick, onBeforeUnmount, reactive, ref, toRef, useId, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { Alert, Button, Tag } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import WizardFrame from '@/components/WizardFrame.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { useNormalImportWizard } from '@/workbench/import/normal/useNormalImportWizard'
import NormalImportSourceStep from '@/workbench/import/normal/steps/NormalImportSourceStep.vue'
import NormalImportInputStep from '@/workbench/import/normal/steps/NormalImportInputStep.vue'
import NormalImportObjectsStep from '@/workbench/import/normal/steps/NormalImportObjectsStep.vue'
import NormalImportExecutionStep from '@/workbench/import/normal/steps/NormalImportExecutionStep.vue'
import NormalImportReviewStep from '@/workbench/import/normal/steps/NormalImportReviewStep.vue'
import NormalImportSubmitConfirmation from '@/workbench/import/normal/steps/NormalImportSubmitConfirmation.vue'
import NormalImportSummary from '@/workbench/import/normal/steps/NormalImportSummary.vue'
import NormalImportParameterCatalog from '@/workbench/import/normal/components/NormalImportParameterCatalog.vue'
import '@/workbench/import/normal/normalImportWizard.css'

const requests = new AbortController()
onBeforeUnmount(() => requests.abort())
const route = useRoute()
const wizard = reactive(useNormalImportWizard({ api: browserApi(requests.signal), route, router: useRouter(), fieldPrefix: useId() }))
const { MessageContext } = useOperationMessage([{ content: toRef(wizard, 'operationNotice') }, { content: toRef(wizard, 'copyNotice') }])
const frame = ref<InstanceType<typeof WizardFrame>>()
const catalogOpen = ref(false)
const confirmLeave = ref(false)
let resolveLeave: ((leave: boolean) => void) | undefined
let focusGeneration = 0
watch(() => [wizard.activeStep, route.query.draft, wizard.formSignature], () => { focusGeneration++ }, { flush: 'sync' })
async function advance() {
  const focusRequest = ++focusGeneration
  wizard.nextStep()
  await nextTick()
  if (wizard.attemptedStep !== wizard.activeStep || !wizard.currentStepError) return
  // 长表单校验失败后定位当前工作面的首个错误，键盘用户可直接修正输入。
  const root = frame.value?.$el as HTMLElement | undefined
  const errors = Array.from(root?.querySelectorAll<HTMLElement>('.ant-form-item-has-error') ?? [])
  const error = errors.find(item => item.querySelector('.ant-form-item-explain-error')?.textContent?.trim() === wizard.currentStepError) ?? errors[0]
  if (!error) return
  // 等待原生 Collapse 的展开动画和字段反馈布局完成，避免把仍在高度动画中的字段滚到视口外。
  await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
  const expansion = error.closest('.ant-collapse-content')
  await Promise.allSettled((expansion?.getAnimations({ subtree: true }) ?? []).map(animation => animation.finished))
  await nextTick()
  if (requests.signal.aborted || focusRequest !== focusGeneration || !error.isConnected) return
  error.scrollIntoView({ block: 'center', behavior: 'instant' })
  error?.querySelector<HTMLInputElement>('input:not([disabled]):not([type="hidden"]), textarea:not([disabled])')?.focus({ preventScroll: true })
}
function guardLeave() {
  if (!wizard.hasInputs) return true
  resolveLeave?.(false)
  confirmLeave.value = true
  return new Promise<boolean>(resolve => { resolveLeave = resolve })
}
function finishLeave(leave: boolean) {
  confirmLeave.value = false
  const resolve = resolveLeave
  resolveLeave = undefined
  resolve?.(leave)
}
const logoutCommitted = useLogoutGuard(guardLeave)
function beforeUnload(event: BeforeUnloadEvent) { if (wizard.hasInputs && !logoutCommitted.value) { event.preventDefault(); event.returnValue = '' } }
onBeforeRouteLeave(guardLeave)
onBeforeRouteUpdate((to, from) => !(wizard.bindingDraftRoute && to.query.draft === wizard.currentDraft?.id) && to.query.draft !== from.query.draft ? guardLeave() : true)
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => { focusGeneration++; finishLeave(false); window.removeEventListener('beforeunload', beforeUnload) })
</script>

<template>
  <MessageContext />
  <section class="page-heading"><h1>新建普通导入任务</h1><Tag :color="wizard.draftDirty ? 'warning' : wizard.currentDraft ? 'success' : 'default'">{{ wizard.draftDirty ? '草稿有未保存更改' : wizard.currentDraft ? '草稿已保存' : '草稿尚未创建' }}</Tag></section>
  <WizardFrame ref="frame" kind="normal" :active-step="wizard.activeStep" :step-titles="wizard.titles" :progress-percent="wizard.stepProgressPercent" :completed-steps="wizard.completedSteps" :navigation-locked="wizard.rerunLocked" class="normal-import-builder" @step-change="wizard.moveToStep">
    <template #actions><Button @click="catalogOpen = true">全部参数</Button><Button :loading="wizard.savingDraft" :disabled="wizard.rerunLocked || !wizard.selectedSource || wizard.loadingDraft || !!wizard.draftLoadFailure || wizard.reviewBusy" @click="wizard.saveDraft">保存草稿</Button></template>
    <template #default>
      <Alert v-if="wizard.rerunLocked" type="info" show-icon message="从头重新执行：保留原配置并重新预检查。"><template #description>如需调整参数，请返回<RouterLink :to="`/tasks/${encodeURIComponent(wizard.currentDraft?.sourceTaskId ?? '')}`">来源任务</RouterLink>，选择“基于原配置新建”。</template></Alert>
      <Alert v-if="wizard.unsupportedDraft" type="warning" show-icon message="普通导入草稿链接无效，请重新打开。" />
      <Alert v-if="wizard.draftFailure" type="error" show-icon :message="wizard.draftFailure"><template v-if="wizard.draftLoadFailure" #action><Button type="link" :loading="wizard.loadingDraft" @click="wizard.retryDraft">重试</Button></template></Alert>
      <Alert v-if="wizard.reviewFailure && wizard.activeStep !== wizard.steps.review" type="error" show-icon :message="wizard.reviewFailure" />
      <Alert v-if="wizard.loadingDraft" type="info" show-icon message="正在读取普通导入草稿…" />
      <template v-if="!wizard.loadingDraft && !wizard.draftLoadFailure && !wizard.unsupportedDraft">
        <NormalImportSourceStep v-if="wizard.activeStep === wizard.steps.source" :model="wizard" />
        <NormalImportInputStep v-else-if="wizard.activeStep === wizard.steps.input" :model="wizard" />
        <NormalImportObjectsStep v-else-if="wizard.activeStep === wizard.steps.objects" :model="wizard" />
        <NormalImportExecutionStep v-else-if="wizard.activeStep === wizard.steps.execution" :model="wizard" />
        <NormalImportReviewStep v-else :model="wizard" />
      </template>
    </template>
    <template #summary><NormalImportSummary :model="wizard" /></template>
    <template #footer>
      <footer class="wizard-footer">
        <Button :disabled="wizard.rerunLocked || wizard.activeStep === 1 || wizard.navigating" @click="wizard.previousStep">上一步</Button>
        <span class="footer-grow" />
        <Button v-if="wizard.activeStep < wizard.steps.review" type="primary" :loading="wizard.navigating" :disabled="wizard.loadingDraft || wizard.unsupportedDraft || !!wizard.draftLoadFailure || wizard.loadingSources && wizard.activeStep === wizard.steps.source || wizard.loadingNodes && wizard.activeStep === wizard.steps.input" @click="advance">{{ wizard.footerLabel }}</Button>
        <div v-else class="footer-actions"><Button :loading="wizard.checking || wizard.previewingCommand" :disabled="!wizard.canStartPrecheck" @click="wizard.retryReview">重新预检查</Button><Button type="primary" :loading="wizard.submitting" :disabled="!wizard.canSubmit" :aria-describedby="wizard.fieldPrefix + '-precheck-status'" @click="wizard.submitTask()">提交并启动导入</Button></div>
      </footer>
    </template>
  </WizardFrame>
  <NormalImportParameterCatalog :open="catalogOpen" :model="wizard" @close="catalogOpen = false" />
  <OrchDangerConfirm :open="confirmLeave" title="离开普通导入配置？" confirm-label="放弃配置并离开" cancel-label="继续配置" @cancel="finishLeave(false)" @confirm="finishLeave(true)"><p>当前配置尚未保存，离开后无法恢复。页面内步骤切换会保留配置。</p></OrchDangerConfirm>
  <NormalImportSubmitConfirmation :model="wizard" />
  <OrchDangerConfirm :open="wizard.pendingSourceId !== null" title="更换目标数据源？" confirm-label="更换并清除关联配置" cancel-label="保留当前配置" @cancel="wizard.cancelSourceChange" @confirm="wizard.confirmSourceChange"><p>更换后会回填新数据源的默认数据库，并清除对象、排除表、Oracle 会话格式和高风险数据行为。各数据格式的暂存输入会保留，原风险说明确认失效。</p></OrchDangerConfirm>
  <OrchDangerConfirm :open="wizard.pendingScope !== null" title="改为全部对象？" confirm-label="清除指定对象并切换" cancel-label="保留指定对象" @cancel="wizard.cancelScopeChange" @confirm="wizard.confirmScopeChange"><p>将清除已填写的指定对象表达式。全部对象与具体对象不会同时生效，风险范围将重新计算。</p></OrchDangerConfirm>
  <OrchDangerConfirm :open="wizard.pendingContent !== null" title="更换导入内容？" confirm-label="清除不适用配置并切换" cancel-label="保留当前内容" @cancel="wizard.cancelContentChange" @confirm="wizard.confirmContentChange"><p>将清除新内容不适用的对象定义、兼容选项与高风险数据行为。各格式输入继续暂存，非活动参数不参与当前配置，原风险说明确认失效。</p></OrchDangerConfirm>
</template>
