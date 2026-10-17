import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { NormalImportApi, NormalImportCheck, NormalImportCommand } from '@/api/normalImport'
import type { NormalImportDraftState } from './useNormalImportDraft'

// 预览、检查与提交消费冻结草稿；每次配置或会话变化立即撤销旧结果和确认。
export function useNormalImportReview(deps: {
  api: NormalImportApi; draft: Pick<NormalImportDraftState, 'currentDraft' | 'draftDirty' | 'draftNeedsRebind' | 'savingDraft' | 'loadingDraft' | 'ensureSaved'>; binding: Readonly<Ref<string>>; complete: Readonly<Ref<boolean>>
  filesReady: Readonly<Ref<boolean>>; risky: Readonly<Ref<boolean>>; navigateTask(id: string): Promise<unknown>
}) {
  const commandPreview = ref<NormalImportCommand | null>(null), activePrecheck = ref<NormalImportCheck | null>(null), discoveredFiles = ref<NormalImportCheck | null>(null)
  const previewingCommand = ref(false), checking = ref(false), submitting = ref(false), copyingCommand = ref(false)
  const reviewFailure = ref(''), copyNotice = ref(''), submitConfirmationOpen = ref(false), checkExpired = ref(false)
  const taskNameInput = ref(''), taskNameError = ref('')
  let disposed = false, epoch = 0, preparingDraftEpoch: number | undefined, pollTimer: ReturnType<typeof setTimeout> | undefined, expiryTimer: ReturnType<typeof setTimeout> | undefined
  function stopTimers() { clearTimeout(pollTimer); clearTimeout(expiryTimer); pollTimer = undefined; expiryTimer = undefined }
  function clearFrozenFacts() {
    stopTimers(); commandPreview.value = null; activePrecheck.value = null; discoveredFiles.value = null
    copyNotice.value = ''; submitConfirmationOpen.value = false; checkExpired.value = false
  }
  function resetReview() {
    epoch++; preparingDraftEpoch = undefined; clearFrozenFacts()
    previewingCommand.value = false; checking.value = false; submitting.value = false; copyingCommand.value = false
    reviewFailure.value = ''
    taskNameInput.value = ''; taskNameError.value = ''
  }
  watch(deps.binding, resetReview, { flush: 'sync' })
  watch(() => [deps.draft.currentDraft.value?.id, deps.draft.currentDraft.value?.revision, deps.draft.currentDraft.value?.configFingerprint], () => {
    // 自动保存是当前事务的准备步骤；其他保存则立即撤销旧修订的展示、轮询与请求资格。
    if (preparingDraftEpoch === epoch) clearFrozenFacts()
    else resetReview()
  }, { flush: 'sync' })
  async function prepareDraft(request: number) {
    preparingDraftEpoch = request
    try { return await deps.draft.ensureSaved() }
    finally { if (preparingDraftEpoch === request) preparingDraftEpoch = undefined }
  }
  const reviewBusy = computed(() => previewingCommand.value || checking.value || submitting.value || deps.draft.savingDraft.value || deps.draft.loadingDraft.value)
  const canPreview = computed(() => deps.complete.value && !reviewBusy.value)
  const canDiscoverFiles = computed(() => deps.filesReady.value && !reviewBusy.value)
  const canStartPrecheck = computed(() => deps.complete.value && !reviewBusy.value)
  const canSubmit = computed(() => {
    const check = activePrecheck.value, draft = deps.draft.currentDraft.value
    return !reviewBusy.value && deps.complete.value && !deps.draft.draftDirty.value && !deps.draft.draftNeedsRebind.value && !checkExpired.value && !!draft && !!check && check.executionEnabled && draft.executionEnabled
      && check.checkKind === 'NORMAL_IMPORT_PREFLIGHT' && check.status === 'SUCCEEDED' && check.integrityStatus === 'COMPLETE' && !!check.inputManifest
      && check.draftId === draft.id && check.draftRevision === draft.revision && check.configFingerprint === draft.configFingerprint && Date.parse(check.validUntil) > Date.now()
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
            expiryTimer = setTimeout(() => { if (!disposed && request === epoch) { checkExpired.value = true; submitConfirmationOpen.value = false } }, Math.min(remaining, 2147483647))
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
    const request = epoch; copyingCommand.value = true; copyNotice.value = ''
    try { await navigator.clipboard.writeText(command.redactedCommand); if (!disposed && request === epoch) copyNotice.value = '脱敏命令已复制' }
    catch { if (!disposed && request === epoch) reviewFailure.value = '复制失败，请重试。' }
    finally { if (!disposed && request === epoch) copyingCommand.value = false }
  }
  async function submitTask(confirmed = false) {
    if (!canSubmit.value || !activePrecheck.value || !deps.draft.currentDraft.value) return
    if (!confirmed) { taskNameInput.value = deps.draft.currentDraft.value.config.taskName ?? ''; taskNameError.value = ''; submitConfirmationOpen.value = true; return }
    if (!submitConfirmationOpen.value) return
    const taskName = taskNameInput.value.trim()
    if (Array.from(taskName).length > 200 || /\p{Cc}/u.test(taskName)) { taskNameError.value = '任务名称最多 200 个字符，且不能包含控制字符。'; return }
    taskNameError.value = ''
    const request = epoch, draft = deps.draft.currentDraft.value, check = activePrecheck.value
    submitting.value = true; reviewFailure.value = ''
    try {
      const id = await deps.api.submitNormalImport(draft, check.id, deps.risky.value ? draft.configFingerprint : undefined, taskName || undefined)
      if (!disposed && request === epoch) { submitConfirmationOpen.value = false; await deps.navigateTask(id) }
    } catch { if (!disposed && request === epoch) reviewFailure.value = '提交失败。请重新核对权限、配置与预检查。' }
    finally { if (!disposed && request === epoch) submitting.value = false }
  }
  onScopeDispose(() => { disposed = true; resetReview() })
  return { commandPreview, activePrecheck, discoveredFiles, previewingCommand, checking, submitting, copyingCommand, reviewFailure, copyNotice, submitConfirmationOpen, taskNameInput, taskNameError, checkExpired, reviewBusy, canPreview, canDiscoverFiles, canStartPrecheck, canSubmit, previewCommand, startCheck, copyCommand, submitTask, resetReview }
}
