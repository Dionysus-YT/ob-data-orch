import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import { isNavigationFailure } from 'vue-router'
import type { NormalImportApi, NormalImportCheck, NormalImportCommand, NormalImportDraft } from '@/api/normalImport'
import type { NormalImportDraftState } from './useNormalImportDraft'
import { normalImportBlockingSummary } from './normalImportPrecheckPresentation'

// 预览、检查与提交消费冻结草稿；每次配置或会话变化立即撤销旧结果和确认。
export function useNormalImportReview(deps: {
  api: NormalImportApi; draft: Pick<NormalImportDraftState, 'currentDraft' | 'draftDirty' | 'draftNeedsRebind' | 'savingDraft' | 'loadingDraft' | 'ensureSaved'>; binding: Readonly<Ref<string>>; complete: Readonly<Ref<boolean>>
  filesReady: Readonly<Ref<boolean>>; risky: Readonly<Ref<boolean>>; navigateTask(id: string): Promise<unknown>
}) {
  const commandPreview = ref<NormalImportCommand | null>(null), activePrecheck = ref<NormalImportCheck | null>(null), discoveredFiles = ref<NormalImportCheck | null>(null)
  const previewingCommand = ref(false), checking = ref(false), submitting = ref(false), copyingCommand = ref(false)
  const reviewFailure = ref(''), copyNotice = ref(''), copyFailure = ref(''), submitConfirmationOpen = ref(false), checkExpired = ref(false)
  const taskNameInput = ref(''), taskNameError = ref('')
  const submissionUncertain = ref(false), submittedTaskID = ref('')
  const openingSubmittedTask = ref(false)
  // 未知结果只能恢复同一提交意图；名称、风险确认、证明与键一起冻结，不读取重试时的输入。
  let submissionIntent: { draft: NormalImportDraft; precheckId: string; name?: string; confirmedFingerprint?: string; key: string } | undefined
  const submissionLocked = computed(() => submitting.value || submissionUncertain.value || !!submittedTaskID.value)
  let disposed = false, epoch = 0, preparingDraftEpoch: number | undefined, pollTimer: ReturnType<typeof setTimeout> | undefined, expiryTimer: ReturnType<typeof setTimeout> | undefined
  function stopTimers() { clearTimeout(pollTimer); clearTimeout(expiryTimer); pollTimer = undefined; expiryTimer = undefined }
  function clearFrozenFacts() {
    stopTimers(); commandPreview.value = null; activePrecheck.value = null; discoveredFiles.value = null
    copyNotice.value = copyFailure.value = ''; submitConfirmationOpen.value = false; checkExpired.value = false
  }
  function resetReview() {
    epoch++; preparingDraftEpoch = undefined; clearFrozenFacts()
    previewingCommand.value = false; checking.value = false; submitting.value = false; copyingCommand.value = false
    reviewFailure.value = ''
    taskNameInput.value = ''; taskNameError.value = ''
    submissionIntent = undefined; submissionUncertain.value = false; submittedTaskID.value = disposed ? '' : deps.draft.currentDraft.value?.submittedTaskId ?? ''
    openingSubmittedTask.value = false
  }
  watch(deps.binding, resetReview, { flush: 'sync' })
  watch(() => [deps.draft.currentDraft.value?.id, deps.draft.currentDraft.value?.revision, deps.draft.currentDraft.value?.configFingerprint, deps.draft.currentDraft.value?.submittedTaskId], () => {
    // 自动保存是当前事务的准备步骤；其他保存则立即撤销旧修订的展示、轮询与请求资格。
    if (preparingDraftEpoch === epoch) clearFrozenFacts()
    else resetReview()
    submittedTaskID.value = deps.draft.currentDraft.value?.submittedTaskId ?? ''
  }, { flush: 'sync' })
  async function prepareDraft(request: number) {
    preparingDraftEpoch = request
    try { return await deps.draft.ensureSaved() }
    finally { if (preparingDraftEpoch === request) preparingDraftEpoch = undefined }
  }
  const reviewBusy = computed(() => previewingCommand.value || checking.value || submissionLocked.value || deps.draft.savingDraft.value || deps.draft.loadingDraft.value)
  const canPreview = computed(() => deps.complete.value && !reviewBusy.value)
  const canDiscoverFiles = computed(() => deps.filesReady.value && !reviewBusy.value)
  const canStartPrecheck = computed(() => deps.complete.value && !reviewBusy.value)
  const canSubmit = computed(() => {
    const check = activePrecheck.value, draft = deps.draft.currentDraft.value
    if (disposed || submitting.value || submittedTaskID.value) return false
    // 证明过期不妨碍按原键核对已经发生的提交；服务端仍复验当前权限。
    if (submissionUncertain.value && submissionIntent) return draft?.id === submissionIntent.draft.id && draft.revision === submissionIntent.draft.revision && draft.configFingerprint === submissionIntent.draft.configFingerprint
    return !reviewBusy.value && deps.complete.value && !deps.draft.draftDirty.value && !deps.draft.draftNeedsRebind.value && !checkExpired.value && !!draft && !!check && check.executionEnabled && draft.executionEnabled
      && check.checkKind === 'NORMAL_IMPORT_PREFLIGHT' && check.status === 'SUCCEEDED' && check.integrityStatus === 'COMPLETE' && !!check.inputManifest
      && check.draftId === draft.id && check.draftRevision === draft.revision && check.configFingerprint === draft.configFingerprint && Date.parse(check.validUntil) > Date.now()
  })
  const precheckOverview = computed<{ type: 'info' | 'success' | 'warning' | 'error'; message: string; description: string }>(() => {
    const check = activePrecheck.value
    if (submittedTaskID.value) return { type: reviewFailure.value ? 'warning' : 'success', message: '任务已提交', description: reviewFailure.value || '可查看已创建任务；再次打开不会重复提交。' }
    if (submissionUncertain.value) return { type: 'warning', message: '提交结果尚未确认', description: '请核对原提交结果；核对期间保留原配置，请勿另建相同任务。' }
    if (!deps.complete.value) return { type: 'info', message: '草稿尚未完成配置', description: '请补齐前四步；配置与命令准备就绪后自动检查。' }
    if (deps.draft.draftDirty.value || deps.draft.draftNeedsRebind.value) return { type: 'warning', message: '配置已变化，原检查结果已失效', description: '请保存当前配置后重新检查。' }
    if (reviewFailure.value) return { type: 'error', message: '无法完成当前操作', description: reviewFailure.value }
    if (check && ['PENDING', 'LEASED'].includes(check.status)) return { type: 'info', message: check.status === 'LEASED' ? '执行节点正在检查' : '等待执行节点领取预检查', description: '检查结果返回后将在下方展示；预检查不会启动导入。' }
    if (reviewBusy.value) return { type: 'info', message: '正在准备预检查', description: '当前配置与命令准备就绪后自动检查。' }
    if (checkExpired.value || check?.status === 'EXPIRED') return { type: 'warning', message: '预检查结果已过期', description: '请重新检查，取得当前有效的结果后再提交。' }
    if (check?.status === 'INVALIDATED') return { type: 'warning', message: '预检查结果已失效', description: '请核对当前配置并重新检查。' }
    if (check?.status === 'FAILED') return { type: 'error', message: '预检查未通过', description: `${normalImportBlockingSummary(check) || '检查结果不完整'}。请按对应项目修正后重新检查。` }
    if (canSubmit.value) return { type: 'success', message: '预检查通过，可以提交', description: '当前配置的必需检查全部通过；提交后才会启动导入。' }
    if (check?.status === 'SUCCEEDED' && (!check.executionEnabled || !deps.draft.currentDraft.value?.executionEnabled)) return { type: 'warning', message: '预检查通过，执行暂未开放', description: '检查通过不代表允许启动；当前执行门禁仍阻断提交。' }
    if (check) return { type: 'warning', message: '预检查结果无法用于当前配置', description: '结果缺失、绑定不一致或命令已变化；请重新检查。' }
    return { type: 'info', message: '等待自动预检查', description: '准备就绪后自动开始；检查结束后可手动重新检查。' }
  })
  async function previewCommand() {
    if (!canPreview.value) return
    const request = epoch; previewingCommand.value = true; reviewFailure.value = ''
    try {
      const draft = await prepareDraft(request); if (!draft || disposed || request !== epoch) return
      const command = await deps.api.previewNormalImportCommand(draft)
      if (disposed || request !== epoch || deps.draft.draftDirty.value) return
      if (command.configFingerprint !== draft.configFingerprint) throw new Error('命令绑定失效')
      commandPreview.value = command
    } catch { if (!disposed && request === epoch) reviewFailure.value = '命令预览失败。请重新保存配置后重试。' }
    finally { if (!disposed && request === epoch) previewingCommand.value = false }
  }
  async function startCheck(filesOnly = false) {
    if (filesOnly ? !canDiscoverFiles.value : !canStartPrecheck.value) return
    const request = ++epoch; stopTimers(); checking.value = true; reviewFailure.value = ''; submitConfirmationOpen.value = false
    copyingCommand.value = false; copyNotice.value = copyFailure.value = ''
    activePrecheck.value = null; checkExpired.value = false
    if (filesOnly) discoveredFiles.value = null
    try {
      const draft = await prepareDraft(request); if (!draft || disposed || request !== epoch) return
      const id = await deps.api.startNormalImportCheck(draft, filesOnly)
      if (disposed || request !== epoch) return
      const poll = async () => {
        try {
          const check = await deps.api.getNormalImportCheck(id)
          if (disposed || request !== epoch) return
          if (check.id !== id || check.draftId !== draft.id || check.draftRevision !== draft.revision || check.configFingerprint !== draft.configFingerprint || check.checkKind !== (filesOnly ? 'NORMAL_IMPORT_FILES' : 'NORMAL_IMPORT_PREFLIGHT')) throw new Error('检查绑定失效')
          if (filesOnly) discoveredFiles.value = check; else activePrecheck.value = check
          const remaining = Date.parse(check.validUntil) - Date.now()
          if (remaining <= 0) { checking.value = false; checkExpired.value = true; return }
          if (check.status === 'PENDING' || check.status === 'LEASED') pollTimer = setTimeout(() => void poll(), Math.min(1500, remaining))
          else {
            checking.value = false
            expiryTimer = setTimeout(() => { if (!disposed && request === epoch) { checkExpired.value = true; if (!submissionLocked.value) submitConfirmationOpen.value = false } }, Math.min(remaining, 2147483647))
          }
        } catch { if (!disposed && request === epoch) { checking.value = false; reviewFailure.value = '无法读取检查结果，请重新发起检查。' } }
      }
      await poll()
    } catch { if (!disposed && request === epoch) { checking.value = false; reviewFailure.value = '检查发起失败。请确认节点可用并重新保存配置。' } }
    finally { if (!disposed && request === epoch && !pollTimer) checking.value = false }
  }
  async function copyCommand() {
    const command = commandPreview.value
    if (!command || disposed || copyingCommand.value) return
    const request = epoch; copyingCommand.value = true; copyNotice.value = copyFailure.value = ''
    try { await navigator.clipboard.writeText(command.redactedCommand); if (!disposed && request === epoch) copyNotice.value = '脱敏命令已复制。' }
    catch { if (!disposed && request === epoch) copyFailure.value = '复制失败，请检查浏览器剪贴板权限或手动选择复制。' }
    finally { if (!disposed && request === epoch) copyingCommand.value = false }
  }
  async function submitTask(confirmed = false) {
    if (!canSubmit.value || !activePrecheck.value || !deps.draft.currentDraft.value) return
    if (!confirmed) { taskNameInput.value = submissionIntent?.name ?? (submissionIntent ? '' : deps.draft.currentDraft.value.config.taskName ?? ''); taskNameError.value = ''; submitConfirmationOpen.value = true; return }
    if (!submitConfirmationOpen.value) return
    const taskName = submissionIntent?.name ?? (submissionIntent ? '' : taskNameInput.value.trim())
    if (Array.from(taskName).length > 200 || /\p{Cc}/u.test(taskName)) { taskNameError.value = '任务名称最多 200 个字符，且不能包含控制字符。'; return }
    taskNameError.value = ''
    const request = epoch, draft = deps.draft.currentDraft.value, check = activePrecheck.value
    if (!submissionIntent) {
      try { submissionIntent = { draft: JSON.parse(JSON.stringify(draft)), precheckId: check.id, name: taskName || undefined, confirmedFingerprint: deps.risky.value ? draft.configFingerprint : undefined, key: crypto.randomUUID() } }
      catch { reviewFailure.value = '无法生成安全提交标识，请刷新后重试。'; return }
    }
    const intent = submissionIntent
    submitting.value = true; reviewFailure.value = ''
    try {
      const id = await deps.api.submitNormalImport(intent.draft, intent.precheckId, intent.confirmedFingerprint, intent.name, intent.key)
      if (!disposed && request === epoch) {
        submittedTaskID.value = id; submissionUncertain.value = false; submitConfirmationOpen.value = false
        await openSubmittedTask()
      }
    } catch (error) {
      if (disposed || request !== epoch) return
      let rejection = error
      // 丢失回执或证明冲突后读取已授权的冻结证明；只接受同一提交绑定的任务标识。
      try {
        const observed = await deps.api.getNormalImportCheck(intent.precheckId)
        if (disposed || request !== epoch) return
        if (observed.id === intent.precheckId && observed.draftId === intent.draft.id && observed.draftRevision === intent.draft.revision && observed.configFingerprint === intent.draft.configFingerprint && observed.submittedTaskId) {
          submittedTaskID.value = observed.submittedTaskId; submissionUncertain.value = false; submitConfirmationOpen.value = false
          await openSubmittedTask(); return
        }
      } catch (readError) {
        if (readError && typeof readError === 'object' && 'status' in readError && [401, 403, 404].includes(Number(readError.status))) rejection = readError
      }
      if (disposed || request !== epoch) return
      const status = rejection && typeof rejection === 'object' && 'status' in rejection ? Number(rejection.status) : 0
      if ([400, 401, 403, 404, 409, 412, 422].includes(status)) {
        resetReview()
        reviewFailure.value = '提交被拒绝，请核对权限和当前配置后重新预检查。'
      } else {
        submissionUncertain.value = true
        reviewFailure.value = '提交结果尚未确认。核对将复用原提交，请勿另建相同任务。'
      }
    }
    finally { if (!disposed && request === epoch) submitting.value = false }
  }
  async function openSubmittedTask() {
    if (disposed || !submittedTaskID.value || openingSubmittedTask.value) return
    const request = epoch, id = submittedTaskID.value
    openingSubmittedTask.value = true
    try { if (isNavigationFailure(await deps.navigateTask(id))) throw new Error('任务导航未完成。') }
    catch { if (!disposed && request === epoch && submittedTaskID.value === id) reviewFailure.value = '任务已提交，页面跳转未完成，请点击“查看已创建任务”重试。' }
    finally { if (!disposed && request === epoch) openingSubmittedTask.value = false }
  }
  onScopeDispose(() => { disposed = true; resetReview() })
  return { commandPreview, activePrecheck, discoveredFiles, previewingCommand, checking, submitting, copyingCommand, reviewFailure, copyNotice, copyFailure, precheckOverview, submitConfirmationOpen, taskNameInput, taskNameError, checkExpired, reviewBusy, submissionUncertain, submittedTaskID, submissionLocked, openingSubmittedTask, openSubmittedTask, canPreview, canDiscoverFiles, canStartPrecheck, canSubmit, previewCommand, startCheck, copyCommand, submitTask, resetReview }
}
