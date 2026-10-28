import { computed, onMounted, onScopeDispose, ref, watch } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import type { DirectLoadApi, DirectLoadConfig } from '@/api/directLoad'
import { applyDirectLoadScenario, createDirectLoadForm, directLoadInput, hydrateDirectLoad } from './directLoadForm'
import { directLoadInputProgress, directLoadIssues, directLoadReviewStep, directLoadScenarioForSource, directLoadTitles } from './directLoadRules'
import { directLoadPrecheckProgress } from './directLoadPrecheckPresentation'
import { useDirectLoadReferences, type DirectLoadReferenceApi } from './useDirectLoadReferences'
import { useDirectLoadDraft } from './useDirectLoadDraft'
import { useDirectLoadReview } from './useDirectLoadReview'

/** 只组合独立能力与五步导航；入口阅读确认不授予预检查或执行资格。 */
export function useDirectLoadWizard(deps: { api: DirectLoadApi & DirectLoadReferenceApi; route: Pick<RouteLocationNormalizedLoaded, 'query'>; router: Pick<Router, 'replace' | 'push'> }) {
  const form = createDirectLoadForm(), references = useDirectLoadReferences(form, deps.api)
  const input = computed(() => directLoadInput(form))
  const draft = useDirectLoadDraft({ api: deps.api, form, input, bindRoute: id => deps.router.replace({ query: { ...deps.route.query, draft: id } }) })
  const issues = computed(() => directLoadIssues(input.value, references.selectedSource.value, references.selectedNode.value))
  const binding = computed(() => JSON.stringify([input.value, references.selectedSource.value?.revision, references.selectedSource.value?.credentialRevision, references.selectedSource.value?.environment, references.selectedNode.value?.revision, references.selectedNode.value?.platform]))
  watch(binding, draft.invalidateBinding, { flush: 'sync' })
  const review = useDirectLoadReview({ api: deps.api, draft, binding, ready: computed(() => !issues.value.length && !draft.loadingDraft.value), navigateTask: id => deps.router.replace({ name: 'task-detail', params: { id } }) })
  const attemptedStep = ref(0), navigating = ref(false), pendingSourceId = ref<string | null>(null)
  const applicabilityConfirmationOpen = ref(false)
  let disposed = false, session = 0, lastAttempt = '', applicabilityBinding = ''
  // 历史六步链接只将最终步归一到当前最终步，不修改已保存配置或冻结证据。
  const activeStep = computed(() => { const step = Number(deps.route.query.step ?? 1); return draft.rerunLocked.value || step === 6 ? directLoadReviewStep : Number.isInteger(step) && step >= 1 && step <= directLoadReviewStep ? step : 1 })
  const invalidDraftLink = computed(() => deps.route.query.draft !== undefined && (typeof deps.route.query.draft !== 'string' || !deps.route.query.draft))
  const currentIssues = computed(() => issues.value.filter(issue => issue.step === activeStep.value))
  const completedSteps = computed(() => [1, 2, 3, 4].filter(step => step < activeStep.value && !issues.value.some(issue => issue.step === step)))
  const stepProgressPercent = computed(() => activeStep.value === directLoadReviewStep
    ? directLoadPrecheckProgress(review.activePrecheck.value, review.checkExpired.value)
    : directLoadInputProgress(activeStep.value, input.value, issues.value))
  function fieldError(field: string) { return attemptedStep.value === activeStep.value ? currentIssues.value.find(issue => issue.field === field)?.message : undefined }
  async function moveToStep(step: number) {
    if (disposed || navigating.value || review.submitting.value || review.submissionUncertain.value || draft.rerunLocked.value && step !== directLoadReviewStep || !Number.isInteger(step) || step < 1 || step > directLoadReviewStep) return
    navigating.value = true; try { await deps.router.replace({ query: { ...deps.route.query, step: String(step) } }) } finally { if (!disposed) navigating.value = false }
  }
  function nextStep() {
    if (disposed || invalidDraftLink.value || draft.loadingDraft.value || draft.draftLoadFailure.value || navigating.value || applicabilityConfirmationOpen.value || activeStep.value >= directLoadReviewStep) return
    attemptedStep.value = activeStep.value
    if (activeStep.value === 1 && !currentIssues.value.some(issue => issue.field !== 'applicability') && currentIssues.value.some(issue => issue.field === 'applicability')) {
      applicabilityBinding = binding.value; applicabilityConfirmationOpen.value = true; return
    }
    if (!currentIssues.value.length) void moveToStep(activeStep.value + 1)
  }
  function cancelApplicabilityConfirmation() { applicabilityConfirmationOpen.value = false; applicabilityBinding = '' }
  function confirmApplicability() {
    if (disposed || !applicabilityConfirmationOpen.value || applicabilityBinding !== binding.value || activeStep.value !== 1 || draft.loadingDraft.value || currentIssues.value.some(issue => issue.field !== 'applicability')) return
    cancelApplicabilityConfirmation()
    // 只有当前会话的明确确认才同时记录四项阅读确认；最终提交仍另行绑定实际风险。
    form.applicability = { singleTable: true, wholeTableCommit: true, noCheckpoint: true, structureLimits: true }
    nextStep()
  }
  function selectScenario(scenario: DirectLoadConfig['scenario']) { if (!disposed && !draft.rerunLocked.value && !draft.loadingDraft.value && scenario === directLoadScenarioForSource(references.selectedSource.value)) applyDirectLoadScenario(form, scenario) }
  function previousStep() { if (activeStep.value > 1) void moveToStep(activeStep.value - 1) }
  function applySource(id: string) { const source = references.sources.value.find(source => source.id === id); form.dataSourceId = id; applyDirectLoadScenario(form, directLoadScenarioForSource(source)); form.database = source?.defaultDatabase ?? ''; form.table = ''; form.replaceData = form.truncateTable = false; pendingSourceId.value = null }
  function selectSource(id: string) { if (draft.rerunLocked.value || id === form.dataSourceId) return; if (form.dataSourceId && (form.database || form.table || form.replaceData || form.truncateTable)) pendingSourceId.value = id; else applySource(id) }
  function confirmSourceChange() { if (!draft.rerunLocked.value && pendingSourceId.value !== null) applySource(pendingSourceId.value) }
  function cancelSourceChange() { pendingSourceId.value = null }
  watch(activeStep, () => { attemptedStep.value = 0; cancelSourceChange(); cancelApplicabilityConfirmation(); lastAttempt = ''; if (activeStep.value !== directLoadReviewStep) review.resetReview() }, { flush: 'sync' })
  watch(binding, () => { lastAttempt = ''; cancelApplicabilityConfirmation() }, { flush: 'sync' })
  watch([activeStep, binding, review.canReview], () => {
    if (activeStep.value !== directLoadReviewStep || !review.canReview.value || invalidDraftLink.value || draft.draftLoadFailure.value || lastAttempt === binding.value) return
    lastAttempt = binding.value; void review.prepareReview()
  }, { flush: 'post' })
  async function openSession() {
    if (draft.bindingDraftRoute.value && deps.route.query.draft === draft.currentDraft.value?.id) return
    const requested = ++session; cancelApplicabilityConfirmation(); review.resetReview(); references.resetReferences(); hydrateDirectLoad(form, createDirectLoadForm()); draft.resetDraft(); cancelSourceChange(); lastAttempt = ''; attemptedStep.value = 0
    const id = deps.route.query.draft
    if (typeof id === 'string' && id) await draft.loadDraft(id)
    if (disposed || requested !== session || deps.route.query.draft !== id) return
    void references.loadSources(); void references.loadNodes()
  }
  watch(() => deps.route.query.draft, () => { void openSession() }, { flush: 'sync' })
  onMounted(() => { void openSession(); void references.loadMetadata() })
  onScopeDispose(() => { disposed = true; session++; cancelApplicabilityConfirmation() })
  function goToNode() {
    if (disposed) return
    const node = references.selectedNode.value
    return deps.router.push(node ? { name: 'node-detail', params: { id: node.id } } : { name: 'nodes' })
  }
  return { form, ...references, ...draft, ...review, titles: directLoadTitles, reviewStep: directLoadReviewStep, input, issues, activeStep, attemptedStep, navigating, pendingSourceId, applicabilityConfirmationOpen, invalidDraftLink, currentIssues, completedSteps, stepProgressPercent, fieldError, moveToStep, nextStep, previousStep, selectSource, selectScenario, confirmSourceChange, cancelSourceChange, confirmApplicability, cancelApplicabilityConfirmation, goToNode, goToSources: () => deps.router.push('/data-sources'), retryDraft: openSession }
}
export type DirectLoadWizard = ReturnType<typeof useDirectLoadWizard>
