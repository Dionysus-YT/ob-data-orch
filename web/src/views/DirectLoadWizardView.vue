<script setup lang="ts">
import { nextTick, onBeforeUnmount, reactive, ref, toRef, useId } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { Alert, Button, Checkbox, FormItem, Input, Tag } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { useLogoutGuard } from '@/composables/useLogoutGuard'
import WizardFrame from '@/components/WizardFrame.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { useDirectLoadWizard } from '@/workbench/import/direct-load/useDirectLoadWizard'
import DirectLoadConfigurationStep from '@/workbench/import/direct-load/steps/DirectLoadConfigurationStep.vue'
import DirectLoadReviewStep from '@/workbench/import/direct-load/steps/DirectLoadReviewStep.vue'
import DirectLoadSummary from '@/workbench/import/direct-load/steps/DirectLoadSummary.vue'
import DirectLoadParameterCatalog from '@/workbench/import/direct-load/components/DirectLoadParameterCatalog.vue'
import '@/workbench/import/direct-load/directLoadWizard.css'

const requests = new AbortController(), route = useRoute(), fieldPrefix = useId()
const wizard = reactive(useDirectLoadWizard({ api: browserApi(requests.signal), route, router: useRouter() }))
const { MessageContext } = useOperationMessage([{ content: toRef(wizard, 'operationNotice') }, { content: toRef(wizard, 'copyNotice') }])
const frame = ref<InstanceType<typeof WizardFrame>>(), catalogOpen = ref(false), confirmLeave = ref(false)
let resolveLeave: ((leave: boolean) => void) | undefined
function guardLeave() { if (!wizard.draftDirty) return true; resolveLeave?.(false); confirmLeave.value = true; return new Promise<boolean>(resolve => { resolveLeave = resolve }) }
function finishLeave(leave: boolean) { confirmLeave.value = false; const resolve = resolveLeave; resolveLeave = undefined; resolve?.(leave) }
const logoutCommitted = useLogoutGuard(guardLeave)
function beforeUnload(event: BeforeUnloadEvent) { if (wizard.draftDirty && !logoutCommitted.value) { event.preventDefault(); event.returnValue = '' } }
onBeforeRouteLeave(guardLeave)
onBeforeRouteUpdate((to, from) => !(wizard.bindingDraftRoute && to.query.draft === wizard.currentDraft?.id) && to.query.draft !== from.query.draft ? guardLeave() : true)
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => { requests.abort(); finishLeave(false); window.removeEventListener('beforeunload', beforeUnload) })
async function advance() { wizard.nextStep(); await nextTick(); const root = frame.value?.$el as HTMLElement | undefined; const error = root?.querySelector<HTMLElement>('.ant-form-item-has-error'); if (error && !requests.signal.aborted) { error.scrollIntoView({ block: 'center' }); error.querySelector<HTMLElement>('input:not([disabled]),button')?.focus({ preventScroll: true }) } }
</script>

<template>
  <MessageContext />
  <section class="page-heading"><div><h1>新建旁路导入任务</h1></div><Tag :color="wizard.draftDirty ? 'warning' : wizard.currentDraft ? 'success' : 'default'">{{ wizard.draftDirty ? '草稿有未保存更改' : wizard.currentDraft ? '草稿已保存' : '草稿尚未创建' }}</Tag></section>
  <WizardFrame ref="frame" kind="direct" :step-titles="wizard.titles" :active-step="wizard.activeStep" :progress-percent="wizard.stepProgressPercent" :completed-steps="wizard.completedSteps" :navigation-locked="wizard.rerunLocked" @step-change="wizard.moveToStep">
    <template #actions><Button @click="catalogOpen = true">全部参数</Button><Button :loading="wizard.savingDraft" :disabled="wizard.rerunLocked || !wizard.selectedSource || wizard.loadingDraft || !!wizard.draftLoadFailure || wizard.busy" @click="wizard.saveDraft">保存草稿</Button></template>
    <template #default>
      <Alert v-if="wizard.invalidDraftLink" type="error" show-icon message="旁路导入草稿链接无效，请重新打开。" />
      <Alert v-if="wizard.draftFailure" type="error" show-icon :message="wizard.draftFailure"><template v-if="wizard.draftLoadFailure" #action><Button type="link" @click="wizard.retryDraft">重试</Button></template></Alert>
      <Alert v-if="wizard.loadingDraft" type="info" show-icon message="正在读取旁路导入草稿…" />
      <Alert v-if="wizard.rerunLocked" type="info" show-icon message="从头重新执行保留原配置，并重新完成所有检查。" />
      <template v-if="!wizard.loadingDraft && !wizard.draftLoadFailure && !wizard.invalidDraftLink"><DirectLoadConfigurationStep v-if="wizard.activeStep < wizard.reviewStep" :model="wizard" :field-prefix="fieldPrefix" /><DirectLoadReviewStep v-else :model="wizard" /></template>
    </template>
    <template #summary><DirectLoadSummary :model="wizard" /></template>
    <template #footer><footer class="wizard-footer"><Button :disabled="wizard.activeStep === 1 || wizard.rerunLocked || wizard.navigating" @click="wizard.previousStep">上一步</Button><span class="footer-grow" /><Button v-if="wizard.activeStep < wizard.reviewStep" type="primary" :disabled="wizard.loadingDraft || !!wizard.draftLoadFailure || wizard.invalidDraftLink || wizard.loadingSources && wizard.activeStep === 1 || wizard.loadingNodes && wizard.activeStep === 3" :loading="wizard.navigating" @click="advance">下一步：{{ wizard.titles[wizard.activeStep] }}</Button><div v-else class="footer-actions"><Button :disabled="!wizard.canReview" :loading="wizard.checking || wizard.previewing" @click="wizard.prepareReview()">重新预检查</Button><Button type="primary" :disabled="!wizard.canSubmit" :loading="wizard.submitting" @click="wizard.openSubmit">提交并启动旁路导入</Button></div></footer></template>
  </WizardFrame>
  <DirectLoadParameterCatalog :open="catalogOpen" :model="wizard" @close="catalogOpen = false" @locate="wizard.moveToStep" />
  <OrchDangerConfirm :open="wizard.applicabilityConfirmationOpen" title="确认旁路导入适用条件" confirm-label="已了解，继续配置" cancel-label="返回配置" @cancel="wizard.cancelApplicabilityConfirmation" @confirm="wizard.confirmApplicability"><p>继续配置前，请了解以下边界：</p><ul><li>仅导入一个明确目标表，同表可以有多个文件分片。</li><li>按表粒度整体提交，不能按普通会话或事务解释。</li><li>失败需从头执行，不支持检查点继续或任务级重试。</li><li>RPC、版本与表结构仍须预检查；BIT/二进制类型、虚拟生成列、索引及 LOB 存在适用限制。</li></ul><p>本次确认只记录已了解适用边界；最终提交仍须核对实际目标、结构证据和当前风险。</p></OrchDangerConfirm>
  <OrchDangerConfirm :open="confirmLeave" title="离开旁路导入配置？" confirm-label="放弃配置并离开" cancel-label="继续配置" @cancel="finishLeave(false)" @confirm="finishLeave(true)"><p>当前未保存输入将丢失。页面内步骤切换保留输入。</p></OrchDangerConfirm>
  <OrchDangerConfirm :open="wizard.pendingSourceId !== null" title="更换目标数据源？" confirm-label="更换并清除关联配置" cancel-label="保留当前配置" @cancel="wizard.cancelSourceChange" @confirm="wizard.confirmSourceChange"><p>将回填新数据源默认数据库，清除目标表与高风险数据行为，撤销旧命令、检查及风险确认。</p></OrchDangerConfirm>
  <OrchDangerConfirm :open="wizard.confirmSubmit" title="确认提交旁路导入" confirm-label="确认整体提交并启动" cancel-label="返回检查" :busy="wizard.submitting" @cancel="wizard.closeSubmit" @confirm="wizard.submitTask"><p>目标：{{ wizard.selectedSource?.displayName }} / {{ wizard.form.database }} / {{ wizard.form.table }}；节点：{{ wizard.selectedNode?.displayName }}。</p><FormItem label="任务名称" required :validate-status="wizard.nameError ? 'error' : undefined" :help="wizard.nameError"><Input v-model:value="wizard.taskName" :disabled="wizard.submissionUncertain || wizard.submitting" :maxlength="128" aria-label="任务名称" /></FormItem><Checkbox v-model:checked="wizard.riskAccepted">已确认单表整体提交、失败从头执行、结构与唯一索引限制、资源影响{{ wizard.form.truncateTable ? '、清空原数据' : '' }}{{ wizard.form.scenario === 'OBSERVER_DIRECT' ? '、直连跨机事务风险' : '' }}。</Checkbox><Alert v-if="!wizard.riskAccepted" type="warning" show-icon message="必须确认以上实际范围的风险后才能提交。" /></OrchDangerConfirm>
</template>
