import { ref, computed, watch, onScopeDispose } from 'vue'
import { type ExportDraft, type CommandPreview, type Precheck, exportDraftErrorMessage } from '@/api/browser'
import { groupPrecheckRows, precheckBlockingSummary, precheckChecksForOutput, precheckResultBlocksSubmission } from './exportPrecheckPresentation'
import type { ExportForm } from './exportForm'
import type { ExportReferences } from './useExportReferences'
import type { ExportRuntime } from './exportRuntime'
import type { ExportParameters } from './useExportParameters'
import { useExportDraftPersistence } from './useExportDraftPersistence'

// useExportDraftLifecycle 管理草稿、命令预览、预检查和提交事务。
export function useExportDraftLifecycle(deps: ExportForm & Pick<ExportReferences, 'sources' | 'nodes' | 'selectedSourceRevision'> & Pick<ExportRuntime, 'activeStep' | 'route' | 'api' | 'moveToStep' | 'router'> & Pick<ExportParameters, 'draftInput'> & Partial<Pick<ExportParameters, 'saveDraftInput'>> & { hydrateDraft(draft: ExportDraft): void }) {
  const {
    hydratingDerivedDraft, selectedDataSourceID, selectedNodeID, sources, nodes, activeStep, route, api, router,
  } = deps
  const commandPreview = ref<CommandPreview | null>(null)
  const previewingCommand = ref(false)

  const commandPreviewFailure = ref('')
  const activePrecheck = ref<Precheck | null>(null)

  const precheckID = ref('')
  const startingPrecheck = ref(false)

  const precheckFailure = ref('')
  const submitting = ref(false)

  const submissionFailure = ref('')
  const submitConfirmationOpen = ref(false)

  // 保存请求期间若表单继续变化，旧响应只能更新草稿版本，不能解锁预检查或提交。
  let disposed = false
  let previewEpoch = 0
  let precheckStartEpoch = 0
  let autoPrecheckVersion = -1
  let precheckExpiryTimer: ReturnType<typeof setTimeout> | undefined
  const precheckExpired = ref(false)
  onScopeDispose(() => {
    disposed = true
    stopPrecheckPolling()
    if (precheckExpiryTimer !== undefined) clearTimeout(precheckExpiryTimer)
  })

  const attemptedStep = ref(0)
  const copyNotice = ref('')
  const copyFailure = ref('')
  let copyEpoch = 0
  let submissionEpoch = 0, autoPreviewVersion = -1
  const submittedTaskID = ref('')
  const persistence = useExportDraftPersistence({ ...deps, resetReview,
    busy: computed(() => submitting.value || startingPrecheck.value || precheckRunning.value || previewingCommand.value),
  })
  const { currentDraft, createdDraftID, creatingDraft, loadingDraft, draftFailure, draftLoadFailure, draftNotice, operationNotice, draftDirty, formVersion, createDraft, loadDerivedDraft, loadCreatedDraft, clearDraftState, invalidateDraftState } = persistence

  function resetReview() {
    previewEpoch++; precheckStartEpoch++; copyEpoch++; submissionEpoch++
    autoPrecheckVersion = autoPreviewVersion = -1
    previewingCommand.value = startingPrecheck.value = submitting.value = false
    stopPrecheckPolling()
    commandPreview.value = null; activePrecheck.value = null; precheckID.value = ''
    commandPreviewFailure.value = copyNotice.value = copyFailure.value = precheckFailure.value = submissionFailure.value = ''
    submitConfirmationOpen.value = false; submittedTaskID.value = ''
  }

  const displayedDraftConfig = computed(() => currentDraft.value?.config)
  const draftScopeSummary = computed(() => {
    const config = displayedDraftConfig.value
    if (!config) return ''
    const scope = config.objectScope
    if (scope.scopeKind === 'ALL') return `${scope.database} · 全部对象${scope.excludeTables?.length ? `；排除表：${scope.excludeTables.join('、')}` : ''}`
    if (scope.scopeKind === 'QUERY_RESULT') return `${scope.database} · 查询结果集（最多 ${config.filterConfig?.queryResultLimit ?? 0} 条）`
    const names = (scope.expressions ?? []).map((expression) => expression.name)
    return `${scope.database} · ${names.length} 个对象：${names.join('、')}`
  })

  const draftContentLabel = computed(() => {
    const config = displayedDraftConfig.value
    if (!config) return ''
    const content = config.contentSelection.contentKind
    if (!content) return '未选择'
    if (config.objectScope.scopeKind === 'QUERY_RESULT') return `按结果集导出 · ${config.dataFormat?.formatKind ?? 'CSV'}`
    if (content === 'DDL_ONLY') return '仅 DDL'
    if (content === 'DDL_AND_DATA') return `DDL + 数据 · ${config.dataFormat?.formatKind ?? 'CSV'}`
    // EX-I4：仅数据按实际数据格式投影。
    return config.dataFormat?.formatKind === 'CUT' ? '仅数据 · CUT' : config.dataFormat?.formatKind === 'SQL' ? '仅数据 · SQL' : '仅数据 · CSV'
  })

  const draftDeliverySummary = computed(() => {
    if (!displayedDraftConfig.value?.contentSelection.contentKind) return '未选择导出内容'
    const format = displayedDraftConfig.value?.dataFormat
    if (!format) return '仅结构，无数据文件格式'
    const options = format.csvOptions
    return [format.formatKind, `编码：${options?.fileEncoding || 'UTF-8（工具默认）'}`, `换行：${options?.lineSeparator || '继承执行节点默认'}`, ...(format.formatKind === 'CSV' || format.formatKind === 'CUT' ? [`NULL 表示：${JSON.stringify(options?.nullString || '\\N')}`] : [])].join(' · ')
  })

  const draftFileSummary = computed(() => {
    const config = displayedDraftConfig.value
    if (!config) return '尚未读取草稿'
    const output = config.outputConfig
    const block = config.performanceConfig?.blockSize
    return [`单文件拆分：${block ? block.endsWith('ROW') ? block.replace('ROW', '') + ' 行' : block.replace('MB', '') + ' MB' : '工具默认'}`, `总量上限：${output.maxFileSize ? output.maxFileSize + ' Byte（可能不包含全部数据）' : '未设置'}`, output.noNestedDir ? '扁平目录' : '工具默认目录层级', output.retainEmptyFiles ? '保留空结果文件' : '工具默认空文件规则', output.compress ? `压缩：${output.compressionAlgo || 'zstd'}${output.compressionLevel !== undefined ? ' · 等级 ' + output.compressionLevel : ''}` : '不压缩'].join('；')
  })

  const displayedSource = computed(() => {
    const sourceID = currentDraft.value?.dataSourceId ?? selectedDataSourceID.value
    return sources.value.find((source) => source.id === sourceID)
  })

  const displayedNode = computed(() => {
    const nodeID = currentDraft.value?.nodeId ?? selectedNodeID.value
    return nodes.value.find((node) => node.id === nodeID)
  })

  const precheckWaitingForResults = computed(() => activePrecheck.value?.status === 'PENDING' || activePrecheck.value?.status === 'LEASED' || (Boolean(precheckID.value) && !activePrecheck.value))
  // 读取失败只暂停本页轮询，不把已接受的检查当成结束；重试须读取同一标识，不能再次发起探测。
  const precheckRunning = computed(() => precheckWaitingForResults.value && !precheckFailure.value)
  const precheckRows = computed(() => {
    const checks = precheckChecksForOutput(Boolean(displayedDraftConfig.value && displayedDraftConfig.value.outputConfig.outputKind !== 'LOCAL'))
    return checks.map((check) => ({
      check,
      // Agent 原子提交完整报告；等待领取和检查中返回的占位 UNKNOWN 不作为已完成事实展示。
      result: activePrecheck.value?.status === 'PENDING' || activePrecheck.value?.status === 'LEASED' ? undefined : activePrecheck.value?.results.find((result) => result.check === check),
    }))
  })

  const blockingPrecheckRows = computed(() => precheckRows.value.filter((row) => precheckResultBlocksSubmission(row.result)))
  const precheckGroups = computed(() => groupPrecheckRows(precheckRows.value))

  // 预检查和脱敏命令必须绑定当前草稿版本、指纹与节点；旧证据不能解锁提交。
  const precheckReadyForSubmission = computed(() => {
    const draft = currentDraft.value
    const precheck = activePrecheck.value
    return Boolean(draft && draft.draftState !== 'EDITING' && precheck && commandPreview.value
      && !draftDirty.value && !precheckExpired.value && !precheckFailure.value && !loadingDraft.value
      && precheck.status === 'SUCCEEDED' && precheck.integrityStatus === 'COMPLETE'
      && precheck.draftId === draft.id && precheck.draftRevision === draft.revision
      && precheck.configFingerprint === draft.configFingerprint && precheck.nodeId === draft.nodeId
      && commandPreview.value.configFingerprint === draft.configFingerprint
      && precheckRows.value.every(row => row.result?.status === 'PASSED'))
  })
  const canSubmit = computed(() => precheckReadyForSubmission.value && !submitting.value && !startingPrecheck.value && !precheckRunning.value)
  const canStartPrecheck = computed(() => Boolean(currentDraft.value && currentDraft.value.draftState !== 'EDITING' && commandPreview.value && !draftDirty.value
    && !loadingDraft.value && !creatingDraft.value && !startingPrecheck.value && !precheckRunning.value && !submitting.value))

  const precheckOverview = computed<{ type: 'info' | 'success' | 'warning' | 'error'; message: string; description: string }>(() => {
    if (draftDirty.value) return { type: 'warning', message: '配置已变化，原检查结果已失效', description: '请保存当前配置；补齐必填项后将自动准备预检查。' }
    if (currentDraft.value?.draftState === 'EDITING') return { type: 'info', message: '草稿尚未完成配置', description: '已保存当前输入；补齐前四步后才能生成命令并预检查。' }
    if (draftLoadFailure.value) return { type: 'error', message: '无法读取草稿', description: draftLoadFailure.value }
    if (!currentDraft.value) return { type: 'info', message: loadingDraft.value ? '正在读取草稿' : '尚未准备好预检查', description: '请完成前四步并保存配置；草稿和命令准备就绪后自动检查。' }
    if (commandPreviewFailure.value) return { type: 'error', message: '无法准备预检查', description: commandPreviewFailure.value }
    if (precheckFailure.value) return { type: 'error', message: '预检查请求或结果读取失败', description: precheckFailure.value }
    if (precheckRunning.value) return { type: 'info', message: activePrecheck.value?.status === 'LEASED' ? '执行节点正在检查' : '等待执行节点领取预检查', description: '检查结果返回后将在下方展示；预检查不会启动导出。' }
    if (startingPrecheck.value || creatingDraft.value || loadingDraft.value || previewingCommand.value) return { type: 'info', message: '正在准备预检查', description: '当前配置与命令准备就绪后自动检查。' }
    if (precheckExpired.value || activePrecheck.value?.status === 'EXPIRED') return { type: 'warning', message: '预检查结果已过期', description: '请重新检查，取得当前有效的结果后再提交。' }
    if (activePrecheck.value?.status === 'INVALIDATED') return { type: 'warning', message: '预检查结果已失效', description: '请核对当前配置并重新检查。' }
    if (activePrecheck.value?.status === 'FAILED') return { type: 'error', message: '预检查未通过', description: `${precheckBlockingSummary(precheckRows.value) || '检查结果不完整'}。请按对应项目修正后重新检查。` }
    if (precheckReadyForSubmission.value) return { type: 'success', message: '预检查通过，可以提交', description: '当前配置的必需检查全部通过；提交后才会启动导出。' }
    if (activePrecheck.value) return { type: 'warning', message: '预检查结果无法用于当前配置', description: '结果缺失、绑定不一致或命令已变化；请重新检查。' }
    return { type: 'info', message: '等待自动预检查', description: '准备就绪后自动开始；检查结束后可手动重新检查。' }
  })

  const derivedDraftBindingLocked = computed(() => !!currentDraft.value?.sourceTaskId && typeof route.query.draft === 'string' && route.query.draft === currentDraft.value.id)

  let precheckPollTimer: ReturnType<typeof setTimeout> | undefined

  let precheckPollResolve: (() => void) | undefined

  let precheckPollVersion = 0

  // 保存仅持久化输入；进入第五步才准备命令，每个版本自动尝试一次，失败保留显式重试。
  watch([activeStep, currentDraft, draftDirty, creatingDraft, loadingDraft, previewingCommand, hydratingDerivedDraft], () => {
    const draft = currentDraft.value
    if (disposed || activeStep.value !== 5 || !draft || draft.draftState === 'EDITING' || draftDirty.value || creatingDraft.value || loadingDraft.value || previewingCommand.value || hydratingDerivedDraft.value || commandPreview.value || autoPreviewVersion === formVersion.value) return
    autoPreviewVersion = formVersion.value
    void loadCommandPreview(draft)
  }, { immediate: true, flush: 'post' })

  // 自动检查只为当前表单版本尝试一次；失败、重进页面或轮询更新不会形成重复探测。
  // 保存和回填完成前不发起事务，手动重试继续复用同一入口和防重边界。
  watch([activeStep, currentDraft, commandPreview, draftDirty, creatingDraft, loadingDraft, previewingCommand, hydratingDerivedDraft], () => {
    if (disposed || activeStep.value !== 5 || hydratingDerivedDraft.value || previewingCommand.value || !canStartPrecheck.value
      || activePrecheck.value || precheckID.value || autoPrecheckVersion === formVersion.value) return
    autoPrecheckVersion = formVersion.value
    void startPrecheck()
  }, { immediate: true, flush: 'post' })

  // 有效期到达时只使本地提交资格失效，不伪造服务端检查状态或自动发起新探测。
  watch(activePrecheck, refreshPrecheckExpiry, { flush: 'sync' })
  function refreshPrecheckExpiry() {
    if (precheckExpiryTimer !== undefined) clearTimeout(precheckExpiryTimer)
    precheckExpiryTimer = undefined
    const result = activePrecheck.value
    const remaining = Date.parse(result?.validUntil ?? '') - Date.now()
    precheckExpired.value = result?.status === 'SUCCEEDED' && (!Number.isFinite(remaining) || remaining <= 0)
    if (!disposed && result?.status === 'SUCCEEDED' && !precheckExpired.value) {
      precheckExpiryTimer = setTimeout(refreshPrecheckExpiry, Math.min(remaining, 2_147_483_647))
    }
  }

  async function copyCommand() {
    const preview = commandPreview.value
    if (disposed || !preview) return
    const epoch = ++copyEpoch
    const version = previewEpoch
    const valid = () => !disposed && epoch === copyEpoch && version === previewEpoch && commandPreview.value === preview
    copyNotice.value = copyFailure.value = ''
    // 剪贴板回调不能在预览变化、切换草稿或卸载后恢复旧成功或错误。
    try {
      await navigator.clipboard.writeText(preview.command)
      if (valid()) copyNotice.value = '脱敏命令已复制。'
    } catch {
      if (valid()) copyFailure.value = '复制失败，请检查浏览器剪贴板权限或手动选择复制。'
    }
  }

  async function loadCommandPreview(draft = currentDraft.value) {
    if (disposed || !draft || draft.draftState === 'EDITING' || draftDirty.value) return
    const epoch = ++previewEpoch
    previewingCommand.value = true
    commandPreview.value = null
    copyNotice.value = copyFailure.value = ''
    commandPreviewFailure.value = ''
    try {
      const preview = await api.previewExportCommand(draft)
      if (disposed || epoch !== previewEpoch || draftDirty.value || currentDraft.value?.id !== draft.id || currentDraft.value.revision !== draft.revision) return
      commandPreview.value = preview
    } catch (error) {
      if (disposed || epoch !== previewEpoch || draftDirty.value || currentDraft.value?.id !== draft.id || currentDraft.value.revision !== draft.revision) return
      commandPreviewFailure.value = exportDraftErrorMessage(error, '无法生成命令预览，请重新读取草稿后重试。')
    } finally {
      if (!disposed && epoch === previewEpoch && currentDraft.value?.id === draft.id && currentDraft.value.revision === draft.revision) previewingCommand.value = false
    }
  }

  async function startPrecheck() {
    if (disposed || !canStartPrecheck.value || !currentDraft.value) return
    autoPrecheckVersion = formVersion.value
    const epoch = ++precheckStartEpoch
    const version = formVersion.value
    const isCurrent = () => !disposed && epoch === precheckStartEpoch && version === formVersion.value && !draftDirty.value
    startingPrecheck.value = true
    precheckFailure.value = ''
    try {
      if (precheckID.value && precheckWaitingForResults.value) {
        await pollPrecheck(precheckID.value)
        return
      }
      stopPrecheckPolling()
      activePrecheck.value = null
      precheckID.value = ''
      let draft = currentDraft.value
      let draftRefreshed = false
      // Agent 重启或固定运行时变更会推进节点事实版本；先重算指纹，避免用户返回上一步手工重建同一份草稿。
      const currentPreview = await api.previewExportCommand(draft)
      if (!isCurrent()) return
      if (currentPreview.configFingerprint !== draft.configFingerprint) {
        draft = await api.updateExportDraft(draft)
        if (!isCurrent()) return
        currentDraft.value = draft
        commandPreview.value = null
        await loadCommandPreview(draft)
        if (!isCurrent() || !commandPreview.value) return
        draftRefreshed = true
      }
      const id = await api.startPrecheck(draft)
      if (!isCurrent()) return
      precheckID.value = id
      startingPrecheck.value = false
      draftNotice.value = draftRefreshed
        ? '执行节点运行事实已变化；已按原有字段刷新草稿并排队预检查。Agent 将只校验固定连接、对象、工具、输出路径和空间；不会启动 OBDUMPER。'
        : '预检查已排队，所选 Agent 将只校验固定连接、对象、工具、输出路径和空间；不会启动 OBDUMPER。'
      await pollPrecheck(id)
    } catch (error) {
      if (isCurrent()) precheckFailure.value = exportDraftErrorMessage(error, '无法发起或读取预检查。若请求已被接受，可点击“重新读取预检查”继续查询。')
    } finally {
      if (!disposed && epoch === precheckStartEpoch) startingPrecheck.value = false
    }
  }

  function submitTask() {
    if (!canSubmit.value) return
    submissionFailure.value = ''
    submitConfirmationOpen.value = true
  }

  function closeSubmitConfirmation() {
    if (!submitting.value) submitConfirmationOpen.value = false
  }

  async function confirmSubmitTask() {
    if (disposed || !currentDraft.value || !activePrecheck.value || !canSubmit.value) return
    const request = ++submissionEpoch, version = formVersion.value
    submitting.value = true
    submissionFailure.value = ''
    try {
      const taskID = await api.submitExportDraft(currentDraft.value, activePrecheck.value.id)
      if (disposed || request !== submissionEpoch || version !== formVersion.value) return
      submitConfirmationOpen.value = false
      submittedTaskID.value = taskID
      await router.push(`/tasks/${encodeURIComponent(taskID)}`)
    } catch (error) {
      if (!disposed && request === submissionEpoch) submissionFailure.value = exportDraftErrorMessage(error, '任务未能提交；请重新读取预检查后重试。')
    } finally {
      if (!disposed && request === submissionEpoch) submitting.value = false
    }
  }

  async function pollPrecheck(id: string) {
    const pollVersion = ++precheckPollVersion
    while (!disposed && pollVersion === precheckPollVersion) {
      const result = await api.getPrecheck(id)
      if (disposed || pollVersion !== precheckPollVersion) return
      if (result.id !== id) throw new Error('预检查回执与当前请求不匹配。')
      activePrecheck.value = result
      if (isTerminalPrecheckStatus(result.status)) {
        draftNotice.value = precheckNotice(result)
        return
      }
      await waitForPrecheckPoll(pollVersion)
    }
  }

  function waitForPrecheckPoll(pollVersion: number): Promise<void> {
    return new Promise((resolve) => {
      precheckPollResolve = () => {
        precheckPollResolve = undefined
        resolve()
      }
      precheckPollTimer = setTimeout(() => {
        precheckPollTimer = undefined
        if (pollVersion === precheckPollVersion) precheckPollResolve?.()
      }, 1000)
    })
  }

  function stopPrecheckPolling() {
    precheckPollVersion += 1
    if (precheckPollTimer !== undefined) {
      clearTimeout(precheckPollTimer)
      precheckPollTimer = undefined
    }
    precheckPollResolve?.()
  }

  function isTerminalPrecheckStatus(status: string) {
    return status === 'SUCCEEDED' || status === 'FAILED' || status === 'EXPIRED' || status === 'INVALIDATED'
  }

  function precheckNotice(precheck: Precheck) {
    if (precheck.status === 'SUCCEEDED') return '预检查已通过；结果仍需与当前配置和有效期匹配，提交后 Agent 才会启动 OBDUMPER。'
    if (precheck.status === 'FAILED') return '预检查未通过；请按结果修正配置并保存后重新检查。'
    if (precheck.status === 'EXPIRED') return '预检查在 Agent 完成前已过期；请重新发起。'
    return '预检查状态已失效；请核对配置后重新检查。'
  }

  return {
    ...persistence, submittedTaskID,
    draftFailure, draftNotice, operationNotice, creatingDraft, createdDraftID, currentDraft, draftDirty, loadingDraft,
    draftLoadFailure, commandPreview, previewingCommand, commandPreviewFailure, activePrecheck, precheckID,
    startingPrecheck, precheckFailure, submitting, submissionFailure, submitConfirmationOpen,
    attemptedStep, copyNotice, copyFailure, displayedDraftConfig, draftScopeSummary, draftContentLabel,
    draftDeliverySummary, draftFileSummary, displayedSource, displayedNode, precheckRunning, precheckWaitingForResults, precheckRows,
    blockingPrecheckRows, precheckGroups, precheckOverview, precheckExpired, canStartPrecheck, canSubmit, derivedDraftBindingLocked, loadDerivedDraft, copyCommand, createDraft,
    clearDraftState, invalidateDraftState, loadCreatedDraft, loadCommandPreview, startPrecheck, submitTask,
    closeSubmitConfirmation, confirmSubmitTask, pollPrecheck, waitForPrecheckPoll, stopPrecheckPolling,
    isTerminalPrecheckStatus, precheckNotice,
  }
}

export type ExportLifecycle = ReturnType<typeof useExportDraftLifecycle>
