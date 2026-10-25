import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { DirectLoadApi, DirectLoadCheck, DirectLoadCommand, DirectLoadDraft } from '@/api/directLoad'
import type { DirectLoadDraftState } from './useDirectLoadDraft'

/** 预览、检查、复制和提交仅接纳当前配置代；取消等待不代表撤销服务端操作。 */
export function useDirectLoadReview(deps: { api: DirectLoadApi; draft: DirectLoadDraftState; binding: Readonly<Ref<string>>; ready: Readonly<Ref<boolean>>; navigateTask(id: string): Promise<unknown> }) {
  const commandPreview = ref<DirectLoadCommand>(), activePrecheck = ref<DirectLoadCheck>(), discoveredFiles = ref<DirectLoadCheck>()
  const previewing = ref(false), checking = ref(false), submitting = ref(false), copying = ref(false)
  const reviewFailure = ref(''), copyFailure = ref(''), copyNotice = ref(''), confirmSubmit = ref(false), taskName = ref(''), nameError = ref(''), riskAccepted = ref(false)
  const submissionUncertain = ref(false), submittedTaskId = ref('')
  // 一次确认对应一个提交意图；响应丢失时保留原名称、证明和幂等键，不能换键重建任务。
  let submissionIntent: { key: string; name: string; draft: DirectLoadDraft; precheckId: string; fingerprint: string } | undefined
  const now = ref(Date.now()), checkExpired = computed(() => !!activePrecheck.value && Date.parse(activePrecheck.value.validUntil) <= now.value)
  let disposed = false, epoch = 0, timer: ReturnType<typeof setTimeout> | undefined, wake: (() => void) | undefined
  const ticker = setInterval(() => { now.value = Date.now() }, 1000)
  watch(taskName, () => { nameError.value = '' }, { flush: 'sync' })
  const busy = computed(() => previewing.value || checking.value || submitting.value || deps.draft.savingDraft.value)
  const canReview = computed(() => deps.ready.value && !busy.value && !submissionUncertain.value && !submittedTaskId.value && !deps.draft.loadingDraft.value && !deps.draft.draftLoadFailure.value)
  const canSubmit = computed(() => {
    const draft = deps.draft.currentDraft.value, check = activePrecheck.value, command = commandPreview.value
    return !!(deps.ready.value && draft && !deps.draft.draftDirty.value && !busy.value && !submittedTaskId.value && draft.executionEnabled && command?.executionEnabled && check?.executionEnabled && command.configFingerprint === draft.configFingerprint && check.draftId === draft.id && check.draftRevision === draft.revision && check.configFingerprint === draft.configFingerprint && check.status === 'SUCCEEDED' && check.integrityStatus === 'COMPLETE' && check.directLoadConfigFingerprint && (!checkExpired.value || submissionUncertain.value && submissionIntent?.precheckId === check.id))
  })
  function resetReview() { epoch++; clearTimeout(timer); wake?.(); wake = undefined; commandPreview.value = undefined; activePrecheck.value = discoveredFiles.value = undefined; previewing.value = checking.value = submitting.value = copying.value = false; reviewFailure.value = copyFailure.value = copyNotice.value = taskName.value = nameError.value = submittedTaskId.value = ''; confirmSubmit.value = riskAccepted.value = submissionUncertain.value = false; submissionIntent = undefined }
  watch(deps.binding, resetReview, { flush: 'sync' })
  async function prepareReview(filesOnly = false) {
    if (!canReview.value || disposed) return
    const requested = epoch, binding = deps.binding.value, active = () => !disposed && requested === epoch && binding === deps.binding.value
    previewing.value = true; reviewFailure.value = ''; activePrecheck.value = undefined; confirmSubmit.value = riskAccepted.value = false
    try {
      const draft = await deps.draft.ensureSaved(); if (!draft || !active()) return
      const preview = await deps.api.previewDirectLoadCommand(draft); if (!active()) return
      if (preview.configFingerprint !== draft.configFingerprint) throw new Error()
      commandPreview.value = preview; previewing.value = false; checking.value = true
      const id = await deps.api.startDirectLoadCheck(draft, filesOnly); if (!active()) return
      for (let attempt = 0; attempt < 120 && active(); attempt++) {
        const check = await deps.api.getDirectLoadCheck(id); if (!active()) return
        if (check.id !== id || check.draftId !== draft.id || check.draftRevision !== draft.revision || check.configFingerprint !== draft.configFingerprint || check.checkKind !== (filesOnly ? 'DIRECT_LOAD_FILES' : 'DIRECT_LOAD_PREFLIGHT')) throw new Error()
        if (filesOnly) discoveredFiles.value = check; else activePrecheck.value = check
        now.value = Date.now()
        if (check.status !== 'PENDING' && check.status !== 'LEASED') return
        await new Promise<void>(resolve => { wake = resolve; timer = setTimeout(() => { wake = undefined; resolve() }, 750) })
      }
      if (active()) reviewFailure.value = '检查仍未结束，请稍后重新读取或重试。'
    } catch { if (active()) reviewFailure.value = '旁路命令或预检查无法完成，请核对配置、权限和执行节点后重试。' }
    finally { if (active()) previewing.value = checking.value = false }
  }
  async function copyCommand() {
    if (disposed || copying.value || !commandPreview.value) return
    const requested = epoch, command = commandPreview.value; copying.value = true; copyFailure.value = copyNotice.value = ''
    try { await navigator.clipboard.writeText(command.redactedCommand); if (!disposed && requested === epoch && commandPreview.value === command) copyNotice.value = '脱敏命令已复制' }
    catch { if (!disposed && requested === epoch) copyFailure.value = '复制失败，请允许剪贴板访问或手动选择命令复制。' }
    finally { if (!disposed && requested === epoch) copying.value = false }
  }
  function openSubmit() { if (canSubmit.value) { taskName.value = submissionIntent?.name ?? ''; nameError.value = ''; riskAccepted.value = false; confirmSubmit.value = true } }
  function closeSubmit() { if (!submitting.value) { confirmSubmit.value = false; taskName.value = nameError.value = ''; riskAccepted.value = false } }
  async function submitTask() {
    if (!canSubmit.value || !confirmSubmit.value || !riskAccepted.value || disposed) return
    if (!taskName.value.trim() || taskName.value.trim().length > 128 || Array.from(taskName.value).some(char => char.charCodeAt(0) < 32)) { nameError.value = '请填写 1～128 字的任务名称。'; return }
    if (submissionIntent && taskName.value.trim() !== submissionIntent.name) { nameError.value = '上次提交结果尚未确认，请保持原任务名称重试并核对结果。'; return }
    const draft = deps.draft.currentDraft.value!, check = activePrecheck.value!, requested = epoch
    if (!submissionIntent) {
      try { submissionIntent = { key: crypto.randomUUID(), name: taskName.value.trim(), draft, precheckId: check.id, fingerprint: draft.configFingerprint } }
      catch { reviewFailure.value = '无法生成安全提交标识，请刷新后重试。'; return }
    }
    const intent = submissionIntent, active = () => !disposed && requested === epoch
    submitting.value = true; reviewFailure.value = ''
    async function accept(id: string) { if (!active()) return; submittedTaskId.value = id; submissionUncertain.value = false; confirmSubmit.value = false; await deps.navigateTask(id) }
    try { await accept(await deps.api.submitDirectLoad(intent.draft, intent.precheckId, intent.fingerprint, intent.name, intent.key)) }
    catch (error) {
      if (!active()) return
      if (submittedTaskId.value) { reviewFailure.value = '任务已提交，页面跳转未完成；请从任务中心查看该任务。'; return }
      submissionUncertain.value = true
      reviewFailure.value = '提交结果尚未确认；重试将复用原请求，请勿另建相同任务。也可到任务中心核对。'
      try {
        const observed = await deps.api.getDirectLoadCheck(intent.precheckId)
        if (!active()) return
        if (observed.id === intent.precheckId && observed.draftId === intent.draft.id && observed.draftRevision === intent.draft.revision && observed.configFingerprint === intent.fingerprint && observed.submittedTaskId) await accept(observed.submittedTaskId)
      } catch { if (active() && submittedTaskId.value) reviewFailure.value = '任务已提交，页面跳转未完成；请从任务中心查看该任务。' }
      // 明确拒绝且未找回原任务时撤销证明；网络/响应解析失败仍按结果不确定处理，保留原键。
      const rejected = error && typeof error === 'object' && 'status' in error && [400, 401, 403, 409, 412, 422].includes(Number(error.status))
      if (active() && !submittedTaskId.value && rejected) {
        submissionIntent = undefined; submissionUncertain.value = false; confirmSubmit.value = false; activePrecheck.value = undefined
        reviewFailure.value = '提交被拒绝；请核对权限、当前配置与检查结果，重新预检查后再提交。'
      }
    }
    finally { if (!disposed && requested === epoch) submitting.value = false }
  }
  onScopeDispose(() => { disposed = true; clearInterval(ticker); resetReview() })
  return { commandPreview, activePrecheck, discoveredFiles, previewing, checking, submitting, copying, reviewFailure, copyFailure, copyNotice, confirmSubmit, taskName, nameError, riskAccepted, submissionUncertain, submittedTaskId, checkExpired, busy, canReview, canSubmit, resetReview, prepareReview, copyCommand, openSubmit, closeSubmit, submitTask }
}
