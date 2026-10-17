import { computed, onMounted, onScopeDispose, ref, watch } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { createNormalImportForm } from './normalImportForm'
import { normalImportSteps, normalImportTitles } from './normalImportOptions'
import { useNormalImportParameters } from './useNormalImportParameters'
import { useNormalImportReferences, type NormalImportReferenceApi } from './useNormalImportReferences'
import type { NormalImportApi } from '@/api/normalImport'
import { normalImportDraftInput } from './normalImportDraftInput'
import { useNormalImportDraft } from './useNormalImportDraft'
import { useNormalImportReview } from './useNormalImportReview'
import { useNormalImportReviewAutomation } from './useNormalImportReviewAutomation'

// 入口只装配能力与导航，持久化、检查和提交资格由各自的唯一状态所有者提供。
export function useNormalImportWizard(deps: {
  api: NormalImportApi & NormalImportReferenceApi
  route: Pick<RouteLocationNormalizedLoaded, 'query'>
  router: Pick<Router, 'replace'>
  fieldPrefix: string
}) {
  const form = createNormalImportForm()
  const references = useNormalImportReferences(form, deps.api)
  const parameters = useNormalImportParameters(form, references)
  const input = computed(() => normalImportDraftInput(parameters.configuration.value))
  const draft = useNormalImportDraft({ api: deps.api, form, input, bindRoute: id => deps.router.replace({ query: { ...deps.route.query, draft: id } }) })
  watch(parameters.configurationSignature, draft.invalidateDraftBinding, { flush: 'sync' })
  const hasInputs = draft.draftDirty
  const review = useNormalImportReview({
    api: deps.api, draft, binding: computed(() => draft.formSignature.value + parameters.configurationSignature.value),
    complete: computed(() => !parameters.issues.value.length),
    filesReady: computed(() => !parameters.issues.value.some(issue => issue.step <= normalImportSteps.input)),
    risky: computed(() => parameters.riskDescriptions.value.length > 0 || references.selectedSource.value?.environment === 'PRODUCTION'),
    navigateTask: id => deps.router.replace({ name: 'task-detail', params: { id } }),
  })
  const attemptedStep = ref(0)
  const navigating = ref(false)
  let disposed = false
  const activeStep = computed(() => {
    const value = Number(deps.route.query.step ?? 1)
    return Number.isInteger(value) && value >= normalImportSteps.source && value <= normalImportSteps.review ? value : normalImportSteps.source
  })
  const automation = useNormalImportReviewAutomation({
    active: computed(() => activeStep.value === normalImportSteps.review),
    binding: computed(() => draft.formSignature.value + parameters.configurationSignature.value),
    ready: computed(() => !parameters.issues.value.length && !draft.loadingDraft.value && !references.loadingSources.value && !references.loadingNodes.value && !draft.draftLoadFailure.value),
    busy: review.reviewBusy, hasCommand: computed(() => !!review.commandPreview.value), hasCheck: computed(() => !!review.activePrecheck.value),
    prepareCommand: review.previewCommand, startCheck: () => review.startCheck(false),
  })
  const unsupportedDraft = computed(() => deps.route.query.draft !== undefined && typeof deps.route.query.draft !== 'string')
  const currentStepIssues = computed(() => parameters.issues.value.filter(issue => issue.step === activeStep.value))
  const currentStepError = computed(() => currentStepIssues.value[0]?.message ?? '')
  const completedSteps = computed(() => Object.values(normalImportSteps).filter(step => step < activeStep.value && !parameters.issues.value.some(issue => issue.step === step)))
  // 前四步只表达输入完整度；最后一步只使用当前完整预检查的通过项。
  const stepProgressPercent = computed(() => {
    if (activeStep.value === normalImportSteps.review) return Math.round((review.activePrecheck.value?.results.filter(item => item.status === 'PASSED').length ?? 0) / 6 * 100)
    const required = activeStep.value === normalImportSteps.source ? ['dataSourceId', 'database']
      : activeStep.value === normalImportSteps.input ? ['nodeId', 'filePath', 'content', ...(parameters.traits.value.structuredData ? ['format'] : [])]
        : activeStep.value === normalImportSteps.objects ? ['scope', ...(form.scope === 'SPECIFIC' ? ['objects'] : [])] : []
    const invalid = new Set(currentStepIssues.value.map(issue => issue.field))
    const fields = new Set([...required, ...invalid])
    return fields.size ? Math.round((fields.size - invalid.size) * 100 / fields.size) : 100
  })
  const footerLabel = computed(() => activeStep.value === normalImportSteps.execution ? '查看预检查与命令' : `下一步：${normalImportTitles[activeStep.value] ?? ''}`)

  async function moveToStep(step: number) {
    if (disposed || navigating.value || !Number.isInteger(step) || step < normalImportSteps.source || step > normalImportSteps.review || step === activeStep.value) return
    navigating.value = true
    try { await deps.router.replace({ query: { ...deps.route.query, step: String(step) } }) }
    finally { if (!disposed) navigating.value = false }
  }
  function nextStep() {
    if (disposed || navigating.value || activeStep.value >= normalImportSteps.review || unsupportedDraft.value) return
    attemptedStep.value = activeStep.value
    if (!currentStepError.value) void moveToStep(activeStep.value + 1)
  }
  function previousStep() { if (activeStep.value > 1) void moveToStep(activeStep.value - 1) }
  function fieldError(field: string, step = activeStep.value) {
    return attemptedStep.value === step ? parameters.issues.value.find(issue => issue.step === step && issue.field === field)?.message : undefined
  }

  watch(activeStep, () => { attemptedStep.value = 0; parameters.cancelSourceChange(); parameters.cancelScopeChange(); parameters.cancelContentChange() }, { flush: 'sync' })
  // 路由变更立即撤销旧会话，自己的首次保存只绑定 URL；步骤导航保留唯一表单。
  async function openDraftSession() {
    if (draft.bindingDraftRoute.value && deps.route.query.draft === draft.currentDraft.value?.id) return
    review.resetReview()
    parameters.resetParameters()
    references.resetReferences()
    draft.resetDraft()
    attemptedStep.value = 0
    const id = deps.route.query.draft
    if (typeof id === 'string' && id) await draft.loadDraft(id)
    if (disposed || deps.route.query.draft !== id) return
    void references.loadSources(); void references.loadNodes()
  }
  watch(() => deps.route.query.draft, () => { void openDraftSession() }, { flush: 'sync' })
  onMounted(() => { void openDraftSession() })
  onScopeDispose(() => { disposed = true })
  return {
    form, ...references, ...parameters, ...draft, ...review, ...automation, hasInputs, fieldPrefix: deps.fieldPrefix, titles: normalImportTitles, steps: normalImportSteps,
    activeStep, attemptedStep, navigating, unsupportedDraft, currentStepIssues, currentStepError, completedSteps, stepProgressPercent,
    footerLabel, moveToStep, nextStep, previousStep, fieldError,
  }
}

export type NormalImportWizard = ReturnType<typeof useNormalImportWizard>
