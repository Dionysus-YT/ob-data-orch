import { computed, onScopeDispose, reactive, ref, shallowRef, watch } from 'vue'
import type { CandidateOrder, SchedulingCandidatesApi, SchedulingCandidatesPolicy, SchedulingCandidatesWrite } from '@/api/schedulingCandidates'

export const candidateOrderNames: Record<CandidateOrder, string> = { EXISTING: '保持现有顺序', ONLINE_FIRST: '在线节点优先' }
interface Review { readonly session: number; readonly before: SchedulingCandidatesPolicy; readonly payload: SchedulingCandidatesWrite; readonly key: string }
function code(error: unknown) { return error && typeof error === 'object' && 'code' in error ? String(error.code) : '' }
function denied(error: unknown) { return error && typeof error === 'object' && 'status' in error && [401, 403, 404].includes(Number(error.status)) }

/** 候选顺序有独立事实、表单和写会话；改变顺序不持有或修改向导选择。 */
export function useSchedulingCandidates(api: SchedulingCandidatesApi, options: { onSaved: (text: string) => void; clearNotice: () => void; newKey?: () => string }) {
  const policy = shallowRef<SchedulingCandidatesPolicy>(), loading = ref(false), saving = ref(false), editing = ref(false), stale = ref(true), unconfirmed = ref(false), failure = ref('')
  const form = reactive<{ order: CandidateOrder; reason: string }>({ order: 'EXISTING', reason: '' }), fieldError = ref(''), confirmed = ref(false), review = shallowRef<Review>()
  let session = 0, disposed = false, reader: AbortController | undefined, writer: AbortController | undefined, retry: { fingerprint: string; key: string } | undefined
  const current = (expected: number) => !disposed && session === expected
  const changed = computed(() => Boolean(policy.value && form.order !== policy.value.order))
  const dirty = computed(() => editing.value && (changed.value || Boolean(form.reason.trim())))
  const canEdit = computed(() => Boolean(policy.value && !stale.value && !loading.value && !saving.value))
  function validReason() { const reason = form.reason.trim(); return Boolean(reason && Array.from(reason).length <= 200 && !/[\p{Cc}]/u.test(reason) && !/(?:password|secret|token|access[_-]?key)\s*[:=]|-----BEGIN/i.test(reason)) }
  function clearForm() { editing.value = false; form.order = 'EXISTING'; form.reason = ''; fieldError.value = ''; review.value = undefined; confirmed.value = false }
  function invalidate() { session++; reader?.abort(); writer?.abort(); reader = undefined; writer = undefined; loading.value = false; saving.value = false; retry = undefined; clearForm(); options.clearNotice() }
  watch(form, () => { if (saving.value) return; review.value = undefined; confirmed.value = false; if (fieldError.value && validReason()) fieldError.value = '' }, { flush: 'sync' })
  async function read() {
    if (disposed || loading.value || saving.value || editing.value) return
    invalidate(); const expected = session, controller = new AbortController(); reader = controller; loading.value = true; stale.value = true; failure.value = ''
    try { const value = await api.getSchedulingCandidates(controller.signal); if (current(expected)) { policy.value = value; stale.value = false; unconfirmed.value = false } }
    catch (error) { if (current(expected)) { policy.value = undefined; failure.value = denied(error) ? '需要有效登录及系统管理员权限才能读取候选顺序。' : '无法读取候选顺序，请重试；读取成功前不能编辑。' } }
    finally { if (current(expected)) { loading.value = false; reader = undefined } }
  }
  function edit() { if (!canEdit.value || editing.value || !policy.value) return; invalidate(); failure.value = ''; editing.value = true; form.order = policy.value.order }
  function cancel() { if (disposed || saving.value) return; invalidate(); if (unconfirmed.value) { stale.value = true; failure.value = '上次候选顺序保存结果尚未核对，请刷新当前策略。' } else failure.value = '' }
  function reviewChange() {
    if (!canEdit.value || !editing.value || !changed.value || review.value || !policy.value) return
    if (!Object.hasOwn(candidateOrderNames, form.order) || !validReason()) { fieldError.value = '请选择内置策略并填写 1–200 字原因，不要填写密码或密钥。'; return }
    options.clearNotice(); failure.value = ''; fieldError.value = ''
    const payload: SchedulingCandidatesWrite = Object.freeze({ order: form.order, reason: form.reason.trim(), confirmed: true }), fingerprint = JSON.stringify([policy.value.revision, payload])
    if (retry?.fingerprint !== fingerprint) retry = { fingerprint, key: options.newKey?.() ?? crypto.randomUUID() }
    review.value = Object.freeze({ session, before: policy.value, payload, key: retry.key }); confirmed.value = false
  }
  function cancelReview() { if (!saving.value) { review.value = undefined; confirmed.value = false } }
  async function save() {
    const intent = review.value
    if (!intent || !current(intent.session) || !canEdit.value || !editing.value || saving.value || !confirmed.value || intent.before.revision !== policy.value?.revision || intent.payload.order !== form.order || intent.payload.reason !== form.reason.trim()) return
    saving.value = true; failure.value = ''; options.clearNotice(); const controller = new AbortController(); writer = controller; let receiptConfirmed = false
    try {
      const receipt = await api.changeSchedulingCandidates(intent.before.revision, intent.payload, intent.key, controller.signal)
      if (!current(intent.session)) return
      receiptConfirmed = true; clearForm(); retry = undefined; stale.value = true; policy.value = undefined
      const value = await api.getSchedulingCandidates(controller.signal)
      if (!current(intent.session)) return
      policy.value = value; stale.value = false; unconfirmed.value = false
      options.onSaved(receipt.replayed ? '已确认原候选顺序回执，当前策略以重新读取结果为准。' : receipt.changed ? '候选顺序已保存，下次读取候选时生效。' : '候选顺序没有变化。')
    } catch (error) {
      if (!current(intent.session)) return
      review.value = undefined; confirmed.value = false
      if (denied(error)) { clearForm(); policy.value = undefined; stale.value = true; retry = undefined; failure.value = '登录或权限已失效，请重新登录并核对候选顺序。' }
      else if (receiptConfirmed) failure.value = '候选顺序回执已确认，但当前策略读取失败，请刷新核对。'
      else if (['REVISION_CONFLICT', 'IDEMPOTENCY_CONFLICT', 'CANDIDATE_ORDER_CONFIRMATION_REQUIRED'].includes(code(error))) { clearForm(); stale.value = true; retry = undefined; failure.value = '候选顺序或请求版本已变化，未覆盖当前配置，请刷新后比较。' }
      else if (code(error) === 'CANDIDATE_ORDER_FIELDS_INVALID') fieldError.value = '请核对策略及原因后重试。'
      else { unconfirmed.value = true; failure.value = '未确认候选顺序保存成功。请核对输入后重试，相同内容会复用原请求标识。' }
    } finally { if (current(intent.session)) { saving.value = false; writer = undefined } }
  }
  function reset() { invalidate(); policy.value = undefined; stale.value = true; unconfirmed.value = false; failure.value = '' }
  onScopeDispose(() => { disposed = true; reset() })
  return { policy, loading, saving, editing, stale, unconfirmed, failure, form, fieldError, confirmed, review, changed, dirty, canEdit, read, edit, cancel, reviewChange, cancelReview, save, reset }
}
