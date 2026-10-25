import { computed, watch, onMounted, onScopeDispose, nextTick, ref } from 'vue'
import type { ExportRuntime } from './exportRuntime'
import { createExportForm } from './exportForm'
import { useExportReferences } from './useExportReferences'
import { useExportParameters } from './useExportParameters'
import { useExportCatalog } from './useExportCatalog'
import { useExportDraftLifecycle } from './useExportDraftLifecycle'
import { populateFormFromDraft } from './exportFormMapping'
import { useExportRecovery } from './useExportRecovery'

// 只组装导出能力和页面导航；状态在各能力的唯一所有者内创建。
export function useExportWizard(runtime: Omit<ExportRuntime, 'activeStep' | 'moveToStep'>) {
  const { route, router } = runtime
  const activeStep = computed(() => {
    const value = Number(route.query.step ?? '1')
    return Number.isInteger(value) && value >= 1 && value <= 5 ? value : 1
  })
  const form = createExportForm()
  const initializing = ref(true)
  const draftRouteInvalid = computed(() => route.query.draft !== undefined && (typeof route.query.draft !== 'string' || !route.query.draft.trim() || route.query.draft.length > 256 || [...route.query.draft].some(character => character.charCodeAt(0) < 32)))
  const references = useExportReferences({ ...form, ...runtime })
  const parameters = useExportParameters({ ...form, ...references })
  const catalog = useExportCatalog({ ...form, ...references, activeStep, ...runtime })
  const lifecycle = useExportDraftLifecycle({ ...form, ...references, ...parameters, ...runtime, activeStep, moveToStep, hydrateDraft: (draft) => populateFormFromDraft({ ...form, ...catalog }, draft) })
  const recovery = useExportRecovery({ ...form, ...references, ...catalog, ...lifecycle, activeStep, ...runtime })
  const {
    canSubmit, currentDraft, attemptedStep, selectedSource,
    selectedNode, contentInputMessage, objectInputMessage, objectOptionsMessage, ordinaryFormat,
    formatOptionsMessage, draftInput, creatingDraft, scopeKind, enteredObjectCount, database, filePath,
    precheckRows, startingPrecheck, precheckRunning, precheckWaitingForResults, precheckFailure, precheckID, activePrecheck, draftValidationMessage,
    draftDirty, createDraft } = { ...form, ...references, ...parameters, ...lifecycle,
  }
  const titles = ['选择数据源', '导出内容与对象', '选择数据格式', '执行与输出', '预检查与命令']
  const validationAttempt = ref(0)
  // 完成状态只来自本步骤所需的当前配置，不以参数目录导航或访问顺序作为依据。
  const completedSteps = computed(() => [
    Boolean(selectedSource.value),
    Boolean(selectedSource.value && selectedNode.value && !contentInputMessage.value && !objectInputMessage.value && !objectOptionsMessage.value),
    Boolean(selectedSource.value && ordinaryFormat.value && !formatOptionsMessage.value),
    Boolean(draftInput.value),
  ].flatMap((complete, index) => complete && index + 1 < activeStep.value ? [index + 1] : []))
  const footerBaselineNote = computed(() => {
    if (activeStep.value === 5) {
      return canSubmit.value ? '提交后，所选执行节点将启动导出。' : (currentDraft.value ? '当前配置的必需检查通过且结果有效后可提交。' : '请返回第四步保存配置。')
    }
    if (currentStepError.value && (attemptedStep.value === activeStep.value || activeStep.value === 2)) return currentStepError.value
    return activeStep.value === 4 ? '创建草稿时服务端会再次校验数据源和节点授权。' : '填写当前步骤后继续；进入下一步前会检查当前输入。'
  })

  const canAdvance = computed(() => {
    if (activeStep.value === 1) return Boolean(selectedSource.value)
    if (activeStep.value === 2) return Boolean(selectedSource.value && selectedNode.value) && !contentInputMessage.value && !objectInputMessage.value && !objectOptionsMessage.value
    if (activeStep.value === 3) return Boolean(selectedSource.value) && ordinaryFormat.value && !formatOptionsMessage.value
    if (activeStep.value === 4) return Boolean(draftInput.value) && !creatingDraft.value
    return Boolean(selectedSource.value) && activeStep.value < 4
  })

  // 步骤百分比只来自当前可见配置和已返回的预检查事实，不按等待时间推测进度。
  const stepProgressPercent = computed(() => {
    if (activeStep.value === 1) return selectedSource.value ? 100 : 0
    if (activeStep.value === 2) {
      const objectReady = (scopeKind.value === 'ALL' || scopeKind.value === 'QUERY_RESULT' || enteredObjectCount.value > 0) && !contentInputMessage.value && !objectInputMessage.value && !objectOptionsMessage.value
      const completed = Number(Boolean(selectedNode.value)) + Number(Boolean(database.value.trim())) + Number(Boolean(selectedSource.value) && objectReady)
      return Math.round(completed * 100 / 3)
    }
    if (activeStep.value === 3) return ordinaryFormat.value && !formatOptionsMessage.value ? 100 : 0
    if (activeStep.value === 4) return draftInput.value ? 100 : filePath.value.trim() ? 50 : 0
    if (canSubmit.value) return 100
    // 第五步只计已通过或按配置允许跳过的项目，失败和依赖阻断不能显示为全部完成。
    return precheckRows.value.length > 0 ? Math.round(precheckRows.value.filter((row) => row.result?.status === 'PASSED').length * 100 / precheckRows.value.length) : 0
  })

  const footerLabel = computed(() => {
    if (activeStep.value === 4) return '查看预检查与命令'
    if (activeStep.value === 5) {
      if (startingPrecheck.value) return precheckWaitingForResults.value ? '正在读取预检查…' : '正在发起预检查…'
      if (precheckFailure.value && precheckID.value) return '重新读取预检查'
      if (precheckRunning.value) return '预检查进行中…'
      if (precheckID.value && !activePrecheck.value) return '重新读取预检查'
      return activePrecheck.value ? '重新检查' : '开始预检查'
    }
    return `下一步：${titles[activeStep.value]}`
  })

  const currentStepError = computed(() => {
    if (activeStep.value <= 3 && !selectedSource.value) return '请选择已启用且基础连接测试成功的数据源。'
    if (activeStep.value === 2 && !selectedNode.value) return '请选择读取元数据并执行导出的节点。'
    if (activeStep.value === 2) return contentInputMessage.value || objectInputMessage.value || objectOptionsMessage.value
    if (activeStep.value === 3) return !ordinaryFormat.value ? draftValidationMessage.value : formatOptionsMessage.value
    if (activeStep.value === 4) return draftValidationMessage.value
    return ''
  })

  watch(activeStep, () => { attemptedStep.value = 0; parameters.cancelSourceChange() })

  function fieldError(field: string) {
    const result = parameters.draftValidation.value
    return attemptedStep.value === 4 && !result.valid && result.field === field ? result.message : undefined
  }

  const outputPathPlaceholder = computed(() => selectedNode.value?.platform === 'WINDOWS_AMD64'
    ? '例如 /E:/exports/daily'
    : '例如 /var/ob-data-orch/exports/daily')

  function moveToStep(step: number) {
    void router.replace({ query: { ...route.query, step: String(step) } })
  }

  function previousStep() {
    if (activeStep.value > 1) moveToStep(activeStep.value - 1)
  }

  function nextStep() {
    if (initializing.value || lifecycle.loadingDraft.value || draftRouteInvalid.value || lifecycle.draftLoadFailure.value) return
    if (!canAdvance.value) {
      attemptedStep.value = activeStep.value
      validationAttempt.value++
      return
    }
    if (activeStep.value === 4) {
      if (currentDraft.value && !draftDirty.value) {
        moveToStep(5)
        return
      }
      void createDraft()
      return
    }
    if (activeStep.value < 4 && canAdvance.value) moveToStep(activeStep.value + 1)
  }

  onMounted(() => { void initializeWizard() })
  let disposed = false, sessionGeneration = 0, referencesReady = false
  let fingerprint: Promise<string>
  onScopeDispose(() => {
    disposed = true; sessionGeneration++
    lifecycle.stopPrecheckPolling()
    catalog.stopCatalogQuery()
    catalog.stopDatabaseCatalogQuery()
  })
  async function initializeWizard() {
    try {
      fingerprint = recovery.currentSessionFingerprint()
      await Promise.all([references.loadSources(), references.loadNodeCandidates(), references.loadStorageCredentials()])
      if (disposed) return
      referencesReady = true
      await openDraftSession(false)
    } finally { if (!disposed) initializing.value = false }
  }

  // URL 草稿身份变化立即撤销旧事务；自身保存绑定 URL 与步骤切换都保留当前表单。
  watch(() => route.query.draft, () => {
    if (lifecycle.bindingDraftRoute.value && route.query.draft === lifecycle.currentDraft.value?.id) return
    void openDraftSession(true)
  }, { flush: 'sync' })

  async function openDraftSession(reset: boolean) {
    const request = ++sessionGeneration
    if (reset) {
      parameters.cancelSourceChange()
      lifecycle.clearDraftState()
      catalog.stopCatalogQuery(); catalog.stopDatabaseCatalogQuery()
      form.hydratingDerivedDraft.value = true
      try { form.resetForm(); await nextTick() }
      finally { if (!disposed && request === sessionGeneration) form.hydratingDerivedDraft.value = false }
    }
    if (disposed || request !== sessionGeneration || !referencesReady || draftRouteInvalid.value) return
    const id = typeof route.query.draft === 'string' ? route.query.draft : ''
    if (id) await lifecycle.loadDerivedDraft(id)
    else if (!reset && !references.sourceLoadFailure.value && !references.nodeLoadFailure.value) {
      const savedDraftID = await recovery.restoreAfterReferences(await fingerprint)
      if (!disposed && request === sessionGeneration && savedDraftID) await lifecycle.loadDerivedDraft(savedDraftID, false)
    }
  }

  const draftSessionLoading = computed(() => initializing.value || lifecycle.loadingDraft.value)
  const canSaveDraft = computed(() => !!references.selectedSource.value && !draftRouteInvalid.value && !draftSessionLoading.value && !lifecycle.creatingDraft.value && !lifecycle.submitting.value && !lifecycle.startingPrecheck.value && !lifecycle.precheckRunning.value && !lifecycle.previewingCommand.value && !lifecycle.draftLoadFailure.value)
  return {
    ...form, ...references, ...parameters, ...catalog, ...lifecycle, ...runtime, activeStep, titles,
    footerBaselineNote, canAdvance, stepProgressPercent, completedSteps, footerLabel, currentStepError, fieldError, validationAttempt, draftRouteInvalid, draftSessionLoading, canSaveDraft,
    outputPathPlaceholder, moveToStep, previousStep, nextStep,
  }
}

export type ExportWizard = ReturnType<typeof useExportWizard>
