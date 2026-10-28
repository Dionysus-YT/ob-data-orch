<script setup lang="ts">
import { nextTick, onBeforeUnmount, reactive, ref, toRef, toRefs, useId, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { Alert as AAlert, Button as AButton, Tag as ATag } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import '@/workbench/export/exportWizard.css'
import WizardFrame from '@/components/WizardFrame.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import { useExportWizard } from '@/workbench/export/useExportWizard'
import ExportSourceStep from '@/workbench/export/steps/ExportSourceStep.vue'
import ExportObjectsStep from '@/workbench/export/steps/ExportObjectsStep.vue'
import ExportFormatStep from '@/workbench/export/steps/ExportFormatStep.vue'
import ExportOutputStep from '@/workbench/export/steps/ExportOutputStep.vue'
import ExportReviewStep from '@/workbench/export/steps/ExportReviewStep.vue'
import ExportSummary from '@/workbench/export/steps/ExportSummary.vue'
import ExportSubmitConfirmation from '@/workbench/export/steps/ExportSubmitConfirmation.vue'
import ExportParameterCatalog from '@/workbench/export/components/ExportParameterCatalog.vue'

const requests = new AbortController()
onBeforeUnmount(() => requests.abort())
const route = useRoute()
const wizard = reactive(useExportWizard({ api: browserApi(requests.signal), route, router: useRouter(), fieldPrefix: useId(), submitCancelID: useId() }))
const { MessageContext } = useOperationMessage([{ content: toRef(wizard, 'operationNotice') }, { content: toRef(wizard, 'copyNotice') }])
const { draftDirty, createdDraftID, activeStep, stepProgressPercent, moveToStep, creatingDraft, createDraft, attemptedStep, currentStepAlert, previousStep, loadingSources, loadingNodes, canAdvance, nextStep, footerLabel, canStartPrecheck, submitting, startPrecheck, canSubmit, submitTask } = toRefs(wizard)
const catalogOpen = ref(false), confirmLeave = ref(false)
const frame = ref<InstanceType<typeof WizardFrame>>()
let focusGeneration = 0
watch(() => [wizard.activeStep, route.query.draft, wizard.formVersion], () => { focusGeneration++ }, { flush: 'sync' })
async function advance() {
  const request = ++focusGeneration
  nextStep.value()
  await nextTick()
  if (wizard.attemptedStep !== wizard.activeStep || !wizard.currentStepError) return
  const root = frame.value?.$el as HTMLElement | undefined
  const errors = Array.from(root?.querySelectorAll<HTMLElement>('.ant-form-item-has-error') ?? [])
  const error = errors.find(item => item.querySelector('.ant-form-item-explain-error')?.textContent?.trim() === wizard.currentStepError) ?? errors[0]
  if (!error) return
  // 等待 Ant 高级区展开后再定位，旧步骤、旧草稿和卸载的回调不能抢回焦点。
  await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
  await Promise.allSettled((error.closest('.ant-collapse-content')?.getAnimations({ subtree: true }) ?? []).map(animation => animation.finished))
  await nextTick()
  if (requests.signal.aborted || request !== focusGeneration || !error.isConnected) return
  error.scrollIntoView({ block: 'center', behavior: 'instant' })
  error.querySelector<HTMLElement>('input:not([disabled]):not([type="hidden"]), textarea:not([disabled])')?.focus({ preventScroll: true })
}
let resolveLeave: ((leave: boolean) => void) | undefined
watch(() => route.query.draft, () => { catalogOpen.value = false }, { flush: 'sync' })
function guardLeave() {
  if (!wizard.draftDirty || wizard.submittedTaskID) return true
  resolveLeave?.(false); confirmLeave.value = true
  return new Promise<boolean>(resolve => { resolveLeave = resolve })
}
function finishLeave(leave: boolean) {
  confirmLeave.value = false
  const resolve = resolveLeave; resolveLeave = undefined; resolve?.(leave)
}
const logoutCommitted = useLogoutGuard(guardLeave)
function beforeUnload(event: BeforeUnloadEvent) { if (wizard.draftDirty && !wizard.submittedTaskID && !logoutCommitted.value) { event.preventDefault(); event.returnValue = '' } }
onBeforeRouteLeave(guardLeave)
onBeforeRouteUpdate((to, from) => !(wizard.bindingDraftRoute && to.query.draft === wizard.currentDraft?.id) && to.query.draft !== from.query.draft ? guardLeave() : true)
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => { focusGeneration++; finishLeave(false); window.removeEventListener('beforeunload', beforeUnload) })
</script>

<template>
  <MessageContext />
  <section class="page-heading">
    <div>
      <h1>新建导出任务</h1>
    </div>
    <ATag :color="draftDirty ? 'warning' : createdDraftID ? 'success' : 'default'">{{ draftDirty ? '草稿有未保存更改' : createdDraftID ? '草稿已保存' : '草稿尚未创建' }}</ATag>
  </section>
  <WizardFrame ref="frame" kind="export" :active-step="activeStep" :progress-percent="stepProgressPercent" :completed-steps="wizard.completedSteps" class="export-builder" @step-change="moveToStep">
    <template #actions><AButton @click="catalogOpen = true">全部参数</AButton><AButton :loading="creatingDraft" :disabled="!wizard.canSaveDraft" @click="createDraft(false)">保存草稿</AButton></template>
    <template #default>
      <AAlert v-if="wizard.draftRouteInvalid" type="warning" show-icon message="导出草稿链接无效，请重新打开。" />
      <AAlert v-if="wizard.draftFailure" type="error" show-icon :message="wizard.draftFailure" />
      <AAlert v-if="wizard.draftLoadFailure" type="error" show-icon :message="wizard.draftLoadFailure"><template #action><AButton type="link" @click="wizard.loadCreatedDraft()">重试</AButton></template></AAlert>
      <AAlert v-if="wizard.draftSessionLoading && (wizard.loadingDraft || activeStep !== 1)" type="info" show-icon message="正在准备导出配置…" />
      <AAlert v-if="wizard.draftNotice && activeStep !== 5" type="info" show-icon :message="wizard.draftNotice" />
      <AAlert v-if="attemptedStep === activeStep && currentStepAlert" class="export-step-error" type="error" show-icon :message="currentStepAlert" />
      <template v-if="(!wizard.draftSessionLoading || activeStep === 1 && !route.query.draft) && !wizard.draftLoadFailure && !wizard.draftRouteInvalid">
        <ExportSourceStep v-if="activeStep === 1" :model="wizard" />

        <ExportObjectsStep v-else-if="activeStep === 2" :model="wizard" />

        <ExportFormatStep v-else-if="activeStep === 3" :model="wizard" />

        <ExportOutputStep v-else-if="activeStep === 4" :model="wizard" />

        <ExportReviewStep v-else :model="wizard" />
      </template>
    </template>

    <template #summary>
      <ExportSummary :model="wizard" />
    </template>

    <template #footer>
      <footer class="wizard-footer">
        <AButton :disabled="activeStep === 1 || creatingDraft" @click="previousStep">上一步</AButton>
        <span class="footer-grow" />
        <AButton v-if="activeStep < 5" :loading="creatingDraft" :disabled="wizard.draftSessionLoading || wizard.draftRouteInvalid || !!wizard.draftLoadFailure || loadingSources || loadingNodes && activeStep === 4 || activeStep === 2 && !canAdvance" type="primary" @click="advance">{{ footerLabel }}</AButton>
        <div v-else class="footer-actions">
          <AButton :disabled="!canStartPrecheck" @click="startPrecheck">{{ footerLabel }}</AButton>
          <AButton :disabled="!canSubmit" type="primary" @click="submitTask">{{ submitting ? '正在提交任务…' : '提交并启动导出' }}</AButton>
        </div>
      </footer>
    </template>
  </WizardFrame>
  <ExportSubmitConfirmation :model="wizard" />
  <ExportParameterCatalog :open="catalogOpen" :model="wizard" @close="catalogOpen = false" />
  <OrchDangerConfirm :open="confirmLeave" title="离开导出配置？" confirm-label="放弃配置并离开" cancel-label="继续配置" @cancel="finishLeave(false)" @confirm="finishLeave(true)"><p>当前配置尚有未保存更改，离开后无法恢复。页面内步骤切换会保留配置。</p></OrchDangerConfirm>
  <OrchDangerConfirm :open="wizard.pendingSourceId !== null" title="更换导出数据源？" confirm-label="更换并清除关联配置" cancel-label="保留当前配置" @cancel="wizard.cancelSourceChange" @confirm="wizard.confirmSourceChange"><p>更换后会清除数据库、对象选择、查询、导出内容及关联参数；原命令与预检查失效。新数据源的导出内容需重新手动选择。</p></OrchDangerConfirm>
</template>
