import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import type { SchedulingPolicyApi, SchedulingPolicy, SchedulingPolicyWrite } from '@/api/schedulingPolicy'

interface Review { readonly session: number; readonly revision: number; readonly beforeMaximum: number | null; readonly payload: SchedulingPolicyWrite; readonly key: string }
function code(error: unknown) { return error && typeof error === 'object' && 'code' in error ? String(error.code) : '' }
function denied(error: unknown) { return error && typeof error === 'object' && 'status' in error && [401, 403, 404].includes(Number(error.status)) }
function maximumIssue(value: number | null) { return value !== null && Number.isInteger(value) && value >= 1 && value <= 1024 ? '' : '请输入 1–1024 的整数。' }
function reasonIssue(value: string) { const text = value.trim(); return text && Array.from(text).length <= 200 && !/[\p{Cc}]/u.test(text) && !/(?:password|secret|token|access[_-]?key)\s*[:=]|-----BEGIN/i.test(text) ? '' : '请输入 1–200 字的变更原因，不要填写密码或密钥。' }

/** 调度策略的唯一读取、表单和写事务所有者；会话代隔离取消、路由变化与迟到响应。 */
export function useSchedulingPolicy(api: SchedulingPolicyApi, options: { onSaved: (text: string) => void; clearNotice: () => void; newKey?: () => string }) {
  const policy = shallowRef<SchedulingPolicy>(), loading = ref(false), saving = ref(false), editing = ref(false), stale = ref(false), failure = ref(''), forbidden = ref(false)
  const unconfirmed = ref(false)
  const maximum = ref<number | null>(null), reason = ref(''), maximumError = ref(''), reasonError = ref(''), confirmed = ref(false), review = shallowRef<Review>()
  let session = 0, disposed = false, reader: AbortController | undefined, writer: AbortController | undefined
  let retry: { fingerprint: string; key: string } | undefined
  const current = (expected: number) => !disposed && expected === session
  const changed = computed(() => Boolean(policy.value && maximum.value !== policy.value.maxConcurrentExecutions))
  const dirty = computed(() => editing.value && (changed.value || Boolean(reason.value.trim())))
  const canEdit = computed(() => Boolean(policy.value && !loading.value && !saving.value && !stale.value && !forbidden.value))
  watch([maximum, reason], () => {
    if (saving.value) return
    review.value = undefined; confirmed.value = false
    // 已经显示的字段错误随修正更新；首次进入编辑不提前报错。
    if (maximumError.value) maximumError.value = maximumIssue(maximum.value)
    if (reasonError.value) reasonError.value = reasonIssue(reason.value)
  }, { flush: 'sync' })

  function clearForm() { editing.value = false; maximum.value = null; reason.value = ''; maximumError.value = ''; reasonError.value = ''; confirmed.value = false; review.value = undefined }
  function invalidate() { session++; reader?.abort(); writer?.abort(); reader = undefined; writer = undefined; loading.value = false; saving.value = false; retry = undefined; clearForm(); options.clearNotice() }
  async function read() {
    if (disposed || loading.value || saving.value || editing.value) return
    invalidate()
    const expected = session, controller = new AbortController(); reader = controller
    loading.value = true; stale.value = true; failure.value = ''; forbidden.value = false
    try { const value = await api.getSchedulingPolicy(controller.signal); if (current(expected)) { policy.value = value; stale.value = false; unconfirmed.value = false } }
    catch (error) { if (!current(expected)) return; policy.value = undefined; forbidden.value = Boolean(denied(error)); failure.value = forbidden.value ? '需要有效登录及系统管理员权限才能查看或修改调度策略。' : '无法读取当前调度策略，请重试；读取成功前不能编辑。' }
    finally { if (current(expected)) { loading.value = false; reader = undefined } }
  }
  function edit() { if (!canEdit.value || editing.value || !policy.value) return; invalidate(); failure.value = ''; editing.value = true; maximum.value = policy.value.maxConcurrentExecutions }
  function cancel() { if (disposed || saving.value) return; invalidate(); if (unconfirmed.value) { stale.value = true; failure.value = '上次保存结果尚未核对，请重新读取当前策略。' } else failure.value = '' }
  function reviewChange() {
    if (!canEdit.value || !editing.value || review.value || !policy.value || !changed.value) return
    options.clearNotice(); failure.value = ''
    maximumError.value = maximumIssue(maximum.value)
    const text = reason.value.trim()
    reasonError.value = reasonIssue(text)
    if (maximumError.value || reasonError.value) return
    const payload: SchedulingPolicyWrite = { maxConcurrentExecutions: maximum.value!, reason: text, confirmed: policy.value.maxConcurrentExecutions === null || maximum.value! < policy.value.maxConcurrentExecutions }
    const fingerprint = JSON.stringify([policy.value.revision, payload])
    if (retry?.fingerprint !== fingerprint) retry = { fingerprint, key: options.newKey?.() ?? crypto.randomUUID() }
    confirmed.value = false
    review.value = { session, revision: policy.value.revision, beforeMaximum: policy.value.maxConcurrentExecutions, payload, key: retry.key }
  }
  function cancelReview() { if (!saving.value) { review.value = undefined; confirmed.value = false } }
  async function save() {
    const intent = review.value
    if (!intent || !current(intent.session) || saving.value || !editing.value || !canEdit.value || intent.revision !== policy.value?.revision || intent.payload.maxConcurrentExecutions !== maximum.value || intent.payload.reason !== reason.value.trim() || intent.payload.confirmed && !confirmed.value) return
    saving.value = true; failure.value = ''; options.clearNotice()
    const controller = new AbortController(); writer = controller
    let receiptConfirmed = false
    try {
      const result = await api.changeSchedulingPolicy(intent.revision, intent.payload, intent.key, controller.signal)
      if (!current(intent.session)) return
      receiptConfirmed = true
      clearForm(); retry = undefined; stale.value = true; policy.value = undefined
      const value = await api.getSchedulingPolicy(controller.signal)
      if (!current(intent.session)) return
      policy.value = value; stale.value = false; unconfirmed.value = false
      options.onSaved(result.replayed ? '已确认原请求的保存回执，当前策略以重新读取结果为准。' : result.changed ? '调度策略已保存，从后续新执行领取生效。' : '调度策略没有变化。')
    } catch (error) {
      if (!current(intent.session)) return
      review.value = undefined; confirmed.value = false
      if (denied(error)) { clearForm(); policy.value = undefined; stale.value = true; forbidden.value = true; retry = undefined; failure.value = '登录或权限已失效，请重新登录并核对当前策略。' }
      else if (receiptConfirmed) { failure.value = '保存回执已确认，但当前策略读取失败。请重新读取，核对实际生效值。' }
      else if (code(error) === 'REVISION_CONFLICT' || code(error) === 'IDEMPOTENCY_CONFLICT') { clearForm(); stale.value = true; retry = undefined; failure.value = '策略或请求版本已变化，未覆盖当前配置。请重新读取后比较。' }
      else if (code(error) === 'SCHEDULING_POLICY_FIELDS_INVALID') { reasonError.value = '请核对并发上限和原因；原因中不能包含密码、令牌或密钥。'; failure.value = '设置未保存，请修正字段后重试。' }
      else { unconfirmed.value = true; failure.value = '未确认保存成功。请核对输入后重试；相同内容会复用原请求标识。' }
    } finally { if (current(intent.session)) { saving.value = false; writer = undefined } }
  }
  function reset() { invalidate(); policy.value = undefined; stale.value = true; forbidden.value = false; unconfirmed.value = false; failure.value = '' }
  onScopeDispose(() => { disposed = true; reset() })
  return { policy, loading, saving, editing, stale, unconfirmed, failure, forbidden, maximum, reason, maximumError, reasonError, confirmed, review, changed, dirty, canEdit, read, edit, cancel, reviewChange, cancelReview, save, reset }
}
