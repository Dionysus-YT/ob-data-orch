import { ref, computed, watch, nextTick, onScopeDispose } from 'vue'
import { type ExportDraft, type CommandPreview, type Precheck, exportDraftErrorMessage } from '@/api/browser'
import { groupPrecheckRows, precheckBlockingSummary, precheckChecksForOutput, precheckResultBlocksSubmission } from './exportPrecheckPresentation'
import type { ExportForm } from './exportForm'
import type { ExportReferences } from './useExportReferences'
import type { ExportRuntime } from './exportRuntime'
import type { ExportParameters } from './useExportParameters'

// useExportDraftLifecycle 管理草稿、命令预览、预检查和提交事务。
export function useExportDraftLifecycle(deps: ExportForm & Pick<ExportReferences, 'sources' | 'nodes' | 'selectedSourceRevision'> & Pick<ExportRuntime, 'activeStep' | 'route' | 'api' | 'moveToStep' | 'router'> & Pick<ExportParameters, 'draftInput'> & { hydrateDraft(draft: ExportDraft): void }) {
  const {
    fields, selectedDataSourceID, selectedNodeID, hydratingDerivedDraft, sources, nodes,
    selectedSourceRevision, activeStep, route, api, moveToStep, router, draftInput,
  } = deps
  const draftFailure = ref('')
  const draftNotice = ref('')
  const operationNotice = ref('')

  const creatingDraft = ref(false)
  const createdDraftID = ref('')

  const currentDraft = ref<ExportDraft | null>(null)
  const draftDirty = ref(false)

  const loadingDraft = ref(false)
  const draftLoadFailure = ref('')

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
  let formVersion = 0
  let disposed = false
  let draftReadEpoch = 0
  let previewEpoch = 0
  let precheckStartEpoch = 0
  let autoPrecheckVersion = -1
  let precheckExpiryTimer: ReturnType<typeof setTimeout> | undefined
  const precheckExpired = ref(false)
  let lastRecovery: { draftID: string; derived: boolean } | undefined
  onScopeDispose(() => {
    disposed = true
    stopPrecheckPolling()
    if (precheckExpiryTimer !== undefined) clearTimeout(precheckExpiryTimer)
  })

  const attemptedStep = ref(0)
  const copyNotice = ref('')
  const copyFailure = ref('')
  let copyEpoch = 0

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
    if (config.objectScope.scopeKind === 'QUERY_RESULT') return `按结果集导出 · ${config.dataFormat?.formatKind ?? 'CSV'}`
    if (content === 'DDL_ONLY') return '仅 DDL'
    if (content === 'DDL_AND_DATA') return `DDL + 数据 · ${config.dataFormat?.formatKind ?? 'CSV'}`
    // EX-I4：仅数据按实际数据格式投影。
    return config.dataFormat?.formatKind === 'CUT' ? '仅数据 · CUT' : config.dataFormat?.formatKind === 'SQL' ? '仅数据 · SQL' : '仅数据 · CSV'
  })

  const draftDeliverySummary = computed(() => {
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
    return Boolean(draft && precheck && commandPreview.value
      && !draftDirty.value && !precheckExpired.value && !precheckFailure.value && !loadingDraft.value
      && precheck.status === 'SUCCEEDED' && precheck.integrityStatus === 'COMPLETE'
      && precheck.draftId === draft.id && precheck.draftRevision === draft.revision
      && precheck.configFingerprint === draft.configFingerprint && precheck.nodeId === draft.nodeId
      && commandPreview.value.configFingerprint === draft.configFingerprint
      && precheckRows.value.every(row => row.result?.status === 'PASSED'))
  })
  const canSubmit = computed(() => precheckReadyForSubmission.value && !submitting.value && !startingPrecheck.value && !precheckRunning.value)
  const canStartPrecheck = computed(() => Boolean(currentDraft.value && commandPreview.value && !draftDirty.value
    && !loadingDraft.value && !creatingDraft.value && !startingPrecheck.value && !precheckRunning.value && !submitting.value))

  const precheckOverview = computed<{ type: 'info' | 'success' | 'warning' | 'error'; message: string; description: string }>(() => {
    if (draftDirty.value) return { type: 'warning', message: '配置已变化，原检查结果已失效', description: '请返回第四步保存当前配置；再次进入本页时会自动检查。' }
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

  const derivedDraftBindingLocked = computed(() => currentDraft.value !== null && typeof route.query.draft === 'string' && route.query.draft === currentDraft.value.id)

  let precheckPollTimer: ReturnType<typeof setTimeout> | undefined

  let precheckPollResolve: (() => void) | undefined

  let precheckPollVersion = 0

  // 自动检查只为当前表单版本尝试一次；失败、重进页面或轮询更新不会形成重复探测。
  // 保存和回填完成前不发起事务，手动重试继续复用同一入口和防重边界。
  watch([activeStep, currentDraft, commandPreview, draftDirty, creatingDraft, loadingDraft, previewingCommand, hydratingDerivedDraft], () => {
    if (disposed || activeStep.value !== 5 || hydratingDerivedDraft.value || previewingCommand.value || !canStartPrecheck.value
      || activePrecheck.value || precheckID.value || autoPrecheckVersion === formVersion) return
    autoPrecheckVersion = formVersion
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

  watch([selectedSourceRevision, ...Object.values(fields)], () => {
    if (hydratingDerivedDraft.value) return
    // 任一配置变化都会清除旧预览和预检查；已有草稿保留标识，待保存时更新。
    invalidateDraftState()
  }, { deep: true, flush: 'sync' })

  // 数据源与节点绑定不可更新；新绑定必须创建新草稿。
  watch([selectedDataSourceID, selectedNodeID], () => {
    if (hydratingDerivedDraft.value) return
    const draft = currentDraft.value
    if (draft && (draft.dataSourceId !== selectedDataSourceID.value || draft.nodeId !== selectedNodeID.value)) clearDraftState()
  }, { flush: 'sync' })

  // 派生入口与同会话保存恢复共用回填；只有派生入口要求 query.draft 仍匹配。
  async function loadDerivedDraft(draftID: string, derived = true) {
    if (disposed) return
    clearDraftState()
    lastRecovery = { draftID, derived }
    const epoch = ++draftReadEpoch
    const requestedVersion = formVersion
    loadingDraft.value = true
    draftLoadFailure.value = ''
    try {
      const draft = await api.getExportDraft(draftID)
      if (disposed || epoch !== draftReadEpoch) return
      if (formVersion !== requestedVersion) {
        draftNotice.value = '读取草稿期间配置已有变化，未覆盖当前输入。'
        return
      }
      if (derived && (typeof route.query.draft !== 'string' || route.query.draft !== draftID)) return
      hydratingDerivedDraft.value = true
      try {
        deps.hydrateDraft(draft)
        // Vue 默认在下一轮刷新 watcher；保持 hydration 标记直到本轮 watcher 全部跳过。
        await nextTick()
      } finally {
        hydratingDerivedDraft.value = false
      }
      if (disposed || epoch !== draftReadEpoch) return
      createdDraftID.value = draftID
      currentDraft.value = draft
      draftDirty.value = false
      draftNotice.value = derived && draft.sourceTaskId
        ? '已加载来源任务派生的草稿；基于原配置新建可以修改参数，从头重新执行不允许修改参数（提交时由服务端复验）。'
        : '已从服务端恢复最近保存的导出草稿；命令重新生成，预检查需要重新执行。'
      await loadCommandPreview(draft)
    } catch (error) {
      if (!disposed && epoch === draftReadEpoch) draftLoadFailure.value = exportDraftErrorMessage(error, '无法读取导出草稿。')
    } finally {
      if (!disposed && epoch === draftReadEpoch) loadingDraft.value = false
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

  async function createDraft(advance = true) {
    const input = draftInput.value
    if (disposed || !input || creatingDraft.value || submitting.value) return
    creatingDraft.value = true
    // EX-I8：已加载的派生草稿在步骤 5 保存时走更新路径，保留来源任务标记；
    // 全新草稿仍走创建路径。
    const existing = currentDraft.value
    if (existing) invalidateDraftState()
    else clearDraftState()
    const savedVersion = formVersion
    try {
      if (existing) {
        const updated = await api.updateExportDraft({ ...existing, dataSourceId: input.dataSourceId, nodeId: input.nodeId, config: input.config })
        if (disposed) return
        if (updated.dataSourceId !== selectedDataSourceID.value || updated.nodeId !== selectedNodeID.value) {
          clearDraftState()
          draftNotice.value = '保存期间更换了数据源或执行节点；请为当前绑定创建新草稿。'
          return
        }
        createdDraftID.value = updated.id
        currentDraft.value = updated
        draftDirty.value = formVersion !== savedVersion
        draftNotice.value = draftDirty.value ? '保存期间配置又有变化；请再次保存当前配置。' : ''
        if (draftDirty.value) return
        operationNotice.value = '草稿已保存。'
        if (advance) moveToStep(5)
        await loadCommandPreview(updated)
        return
      }
      const draftID = await api.createExportDraft(input)
      if (disposed) return
      if (input.dataSourceId !== selectedDataSourceID.value || input.nodeId !== selectedNodeID.value) {
        clearDraftState()
        draftNotice.value = '保存期间更换了数据源或执行节点；请为当前绑定创建新草稿。'
        return
      }
      createdDraftID.value = draftID
      if (formVersion === savedVersion) operationNotice.value = '导出草稿已创建。'
      draftNotice.value = ''
      await loadCreatedDraft(draftID, savedVersion)
      if (advance && currentDraft.value && !draftDirty.value) moveToStep(5)
    } catch (error) {
      if (disposed) return
      draftFailure.value = exportDraftErrorMessage(error, '无法创建导出草稿，请检查当前配置后重试。')
    } finally {
      if (!disposed) creatingDraft.value = false
    }
  }

  function clearDraftState() {
    lastRecovery = undefined
    draftReadEpoch++
    previewEpoch++
    precheckStartEpoch++
    autoPrecheckVersion = -1
    loadingDraft.value = false
    previewingCommand.value = false
    startingPrecheck.value = false
    stopPrecheckPolling()
    createdDraftID.value = ''
    currentDraft.value = null
    draftDirty.value = false
    draftFailure.value = ''
    draftLoadFailure.value = ''
    draftNotice.value = ''
    operationNotice.value = ''
    commandPreview.value = null
    commandPreviewFailure.value = ''
    copyEpoch++
    copyNotice.value = copyFailure.value = ''
    activePrecheck.value = null
    precheckID.value = ''
    precheckFailure.value = ''
    submissionFailure.value = ''
    submitConfirmationOpen.value = false
  }

  // invalidateDraftState 清除旧预览与预检查，同时保留草稿标识；后续保存更新同一草稿，
  // 派生草稿因此不会丢失来源关系，普通草稿也不会在每次编辑后生成孤立副本。
  function invalidateDraftState() {
    formVersion += 1
    const draft = currentDraft.value
    clearDraftState()
    if (!draft) return
    createdDraftID.value = draft.id
    currentDraft.value = draft
    draftDirty.value = true
    draftNotice.value = '当前配置有未保存更改；保存后需重新生成命令预览与预检查。'
  }

  async function loadCreatedDraft(draftID = createdDraftID.value, savedVersion?: number) {
    if (disposed) return
    if (!draftID) {
      if (lastRecovery) await loadDerivedDraft(lastRecovery.draftID, lastRecovery.derived)
      return
    }
    const epoch = ++draftReadEpoch
    loadingDraft.value = true
    draftLoadFailure.value = ''
    commandPreview.value = null
    commandPreviewFailure.value = ''
    try {
      const draft = await api.getExportDraft(draftID)
      if (disposed || epoch !== draftReadEpoch || createdDraftID.value !== draftID) return
      if (draft.dataSourceId !== selectedDataSourceID.value || draft.nodeId !== selectedNodeID.value) {
        clearDraftState()
        draftNotice.value = '读取草稿期间更换了数据源或执行节点；请为当前绑定创建新草稿。'
        return
      }
      currentDraft.value = draft
      draftDirty.value = savedVersion !== undefined && formVersion !== savedVersion
      draftNotice.value = draftDirty.value ? '保存期间配置又有变化；请再次保存当前配置。' : ''
      if (draftDirty.value) return
      await loadCommandPreview(draft)
    } catch (error) {
      if (disposed || epoch !== draftReadEpoch || createdDraftID.value !== draftID) return
      currentDraft.value = null
      draftLoadFailure.value = exportDraftErrorMessage(error, '无法读取刚创建的导出草稿，请重试。')
    } finally {
      if (!disposed && epoch === draftReadEpoch && createdDraftID.value === draftID) loadingDraft.value = false
    }
  }

  async function loadCommandPreview(draft = currentDraft.value) {
    if (disposed || !draft || draftDirty.value) return
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
    autoPrecheckVersion = formVersion
    const epoch = ++precheckStartEpoch
    const version = formVersion
    const isCurrent = () => !disposed && epoch === precheckStartEpoch && version === formVersion && !draftDirty.value
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
    submitting.value = true
    submissionFailure.value = ''
    try {
      const taskID = await api.submitExportDraft(currentDraft.value, activePrecheck.value.id)
      if (disposed) return
      submitConfirmationOpen.value = false
      await router.push(`/tasks/${encodeURIComponent(taskID)}`)
    } catch (error) {
      if (!disposed) submissionFailure.value = exportDraftErrorMessage(error, '任务未能提交；请重新读取预检查后重试。')
    } finally {
      if (!disposed) submitting.value = false
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
