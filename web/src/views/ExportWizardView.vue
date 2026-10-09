<script setup lang="ts">
import { onBeforeUnmount, reactive, toRefs, useId } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Alert as AAlert, Button as AButton, Tag as ATag } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import '@/workbench/export/exportWizard.css'
import WizardFrame from '@/components/WizardFrame.vue'
import { useExportWizard } from '@/workbench/export/useExportWizard'
import ExportSourceStep from '@/workbench/export/steps/ExportSourceStep.vue'
import ExportObjectsStep from '@/workbench/export/steps/ExportObjectsStep.vue'
import ExportFormatStep from '@/workbench/export/steps/ExportFormatStep.vue'
import ExportOutputStep from '@/workbench/export/steps/ExportOutputStep.vue'
import ExportReviewStep from '@/workbench/export/steps/ExportReviewStep.vue'
import ExportSummary from '@/workbench/export/steps/ExportSummary.vue'
import ExportSubmitConfirmation from '@/workbench/export/steps/ExportSubmitConfirmation.vue'

const requests = new AbortController()
onBeforeUnmount(() => requests.abort())
const wizard = reactive(useExportWizard({ api: browserApi(requests.signal), route: useRoute(), router: useRouter(), fieldPrefix: useId(), submitCancelID: useId() }))
const { draftDirty, createdDraftID, activeStep, stepProgressPercent, moveToStep, draftInput, creatingDraft, createDraft, attemptedStep, currentStepError, previousStep, footerBaselineNote, loadingSources, loadingNodes, canAdvance, nextStep, footerLabel, currentDraft, commandPreview, startingPrecheck, precheckRunning, submitting, startPrecheck, canSubmit, submitTask } = toRefs(wizard)
</script>

<template>
  <section class="page-heading">
    <div>
      <h1>新建导出任务</h1>
    </div>
    <ATag :color="draftDirty ? 'warning' : createdDraftID ? 'success' : 'default'">{{ draftDirty ? '草稿有未保存更改' : createdDraftID ? '草稿已保存' : '草稿尚未创建' }}</ATag>
  </section>
  <WizardFrame kind="export" :active-step="activeStep" :progress-percent="stepProgressPercent" class="export-builder" @step-change="moveToStep">
    <template #actions><AButton v-if="activeStep === 4 && draftInput" :loading="creatingDraft" @click="createDraft(false)">保存草稿</AButton></template>
    <template #default>
      <AAlert v-if="attemptedStep === activeStep && currentStepError" class="export-step-error" type="error" show-icon :message="currentStepError" />
      <ExportSourceStep v-if="activeStep === 1" :model="wizard" />

      <ExportObjectsStep v-else-if="activeStep === 2" :model="wizard" />

      <ExportFormatStep v-else-if="activeStep === 3" :model="wizard" />

      <ExportOutputStep v-else-if="activeStep === 4" :model="wizard" />

      <ExportReviewStep v-else :model="wizard" />
    </template>

    <template #summary>
      <ExportSummary :model="wizard" />
    </template>

    <template #footer>
      <footer class="wizard-footer">
        <AButton :disabled="activeStep === 1 || creatingDraft" @click="previousStep">上一步</AButton>
        <span class="wizard-baseline-note">{{ footerBaselineNote }}</span>
        <span class="footer-grow" />
        <AButton v-if="activeStep < 5" :loading="creatingDraft" :disabled="loadingSources || loadingNodes && activeStep === 4 || activeStep === 2 && !canAdvance" type="primary" @click="nextStep">{{ footerLabel }}</AButton>
        <div v-else class="footer-actions">
          <AButton :disabled="!currentDraft || draftDirty || !commandPreview || startingPrecheck || precheckRunning || submitting" @click="startPrecheck">{{ footerLabel }}</AButton>
          <AButton :disabled="!canSubmit" type="primary" @click="submitTask">{{ submitting ? '正在提交任务…' : '提交并启动导出' }}</AButton>
        </div>
      </footer>
    </template>
  </WizardFrame>
  <ExportSubmitConfirmation :model="wizard" />
</template>
