import { computed, watch, onMounted, onScopeDispose } from 'vue'
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
  const references = useExportReferences({ ...form, ...runtime })
  const parameters = useExportParameters({ ...form, ...references })
  const catalog = useExportCatalog({ ...form, ...references, activeStep, ...runtime })
  const lifecycle = useExportDraftLifecycle({ ...form, ...references, ...parameters, ...runtime, moveToStep, hydrateDraft: (draft) => populateFormFromDraft({ ...form, ...catalog }, draft) })
  const recovery = useExportRecovery({ ...form, ...references, ...catalog, ...lifecycle, activeStep, ...runtime })
  const {
    storageOutput, blockingPrecheckRows, canSubmit, currentDraft, attemptedStep, selectedSource,
    selectedNode, contentInputMessage, objectInputMessage, objectOptionsMessage, ordinaryFormat,
    formatOptionsMessage, draftInput, creatingDraft, scopeKind, enteredObjectCount, database, filePath,
    precheckRows, startingPrecheck, precheckRunning, precheckID, activePrecheck, draftValidationMessage,
    draftDirty, createDraft } = { ...form, ...references, ...parameters, ...lifecycle,
  }
  const titles = ['选择数据源', '导出内容与对象', '选择数据格式', '执行与输出', '预检查与命令']
  const footerBaselineNote = computed(() => {
    if (activeStep.value === 5) {
      if (storageOutput.value && blockingPrecheckRows.value.some((row) => row.check === 'STORAGE_CONNECTIVITY' || row.check === 'STORAGE_AUTH')) {
        return '对象存储输出需要存储端点连通性与凭据有效性两项预检查通过后才能提交；这两项探测尚未授权（归 EX-V1 排期），当前保持未完成。'
      }
      return canSubmit.value ? '预检查已通过。提交后，所选 Agent 将领取已冻结的导出任务并启动 OBDUMPER。' : (currentDraft.value ? '草稿已保存；请先完成当前版本的固定预检查，提交后才会启动 OBDUMPER。' : '尚未读取草稿；请返回上一步完成固定字段并创建草稿。')
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
    return precheckRows.value.length > 0 ? Math.round(precheckRows.value.filter((row) => row.result).length * 100 / precheckRows.value.length) : 0
  })

  const footerLabel = computed(() => {
    if (activeStep.value === 4) return creatingDraft.value ? '正在保存草稿…' : currentDraft.value ? '保存并进入预检查' : '创建草稿并进入预检查'
    if (activeStep.value === 5) {
      if (startingPrecheck.value) return '正在发起预检查…'
      if (precheckRunning.value) return '预检查进行中…'
      if (precheckID.value && !activePrecheck.value) return '重新读取预检查'
      return activePrecheck.value ? '重新执行预检查' : '执行预检查'
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

  watch(activeStep, () => { attemptedStep.value = 0 })

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
    if (!canAdvance.value) {
      attemptedStep.value = activeStep.value
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
  let disposed = false
  onScopeDispose(() => {
    disposed = true
    lifecycle.stopPrecheckPolling()
    catalog.stopCatalogQuery()
    catalog.stopDatabaseCatalogQuery()
  })
  async function initializeWizard() {
    const fingerprint = recovery.currentSessionFingerprint()
    await Promise.all([references.loadSources(), references.loadNodeCandidates(), references.loadStorageCredentials()])
    if (disposed) return
    const derivedDraftID = typeof route.query.draft === 'string' ? route.query.draft : ''
    if (derivedDraftID) await lifecycle.loadDerivedDraft(derivedDraftID)
    else if (!references.sourceLoadFailure.value && !references.nodeLoadFailure.value) {
      const savedDraftID = await recovery.restoreAfterReferences(await fingerprint)
      if (!disposed && savedDraftID) await lifecycle.loadDerivedDraft(savedDraftID, false)
    }
  }
  return {
    ...form, ...references, ...parameters, ...catalog, ...lifecycle, ...runtime, activeStep, titles,
    footerBaselineNote, canAdvance, stepProgressPercent, footerLabel, currentStepError,
    outputPathPlaceholder, moveToStep, previousStep, nextStep,
  }
}

export type ExportWizard = ReturnType<typeof useExportWizard>
