import { computed, onScopeDispose, reactive, ref, shallowRef, watch } from 'vue'
import type { SchedulingTimingApi, SchedulingTimingPolicy, SchedulingTimingWrite } from '@/api/schedulingTiming'

interface TimingForm { heartbeatTimeoutSeconds: number | null; startupTimeoutSeconds: number | null; pollingIntervalSeconds: number | null; reason: string }
interface Review { readonly session: number; readonly before: SchedulingTimingPolicy; readonly payload: SchedulingTimingWrite; readonly key: string }
type Field = keyof TimingForm
const fields: Field[] = ['heartbeatTimeoutSeconds', 'startupTimeoutSeconds', 'pollingIntervalSeconds', 'reason']
function errorCode(error: unknown) { return error && typeof error === 'object' && 'code' in error ? String(error.code) : '' }
function denied(error: unknown) { return error && typeof error === 'object' && 'status' in error && [401, 403, 404].includes(Number(error.status)) }
function bounded(value: number | null, min: number, max: number) { return value !== null && Number.isInteger(value) && value >= min && value <= max }

/** 时间策略独立持有表单、冻结确认与写事务，不复制并发策略或 Agent 网络配置。 */
export function useSchedulingTiming(api: SchedulingTimingApi, options: { onSaved: (message: string) => void; clearNotice: () => void; newKey?: () => string }) {
  const policy = shallowRef<SchedulingTimingPolicy>(), loading = ref(false), saving = ref(false), editing = ref(false), stale = ref(true), unconfirmed = ref(false), forbidden = ref(false), failure = ref('')
  const form = reactive<TimingForm>({ heartbeatTimeoutSeconds: null, startupTimeoutSeconds: null, pollingIntervalSeconds: null, reason: '' })
  const errors = reactive<Record<Field, string>>({ heartbeatTimeoutSeconds: '', startupTimeoutSeconds: '', pollingIntervalSeconds: '', reason: '' }), confirmed = ref(false), review = shallowRef<Review>()
  let session = 0, disposed = false, reader: AbortController | undefined, writer: AbortController | undefined, retry: { fingerprint: string; key: string } | undefined
  const current = (expected: number) => !disposed && expected === session
  const changed = computed(() => Boolean(policy.value && (form.heartbeatTimeoutSeconds !== policy.value.heartbeatTimeoutSeconds || form.startupTimeoutSeconds !== policy.value.startupTimeoutSeconds || form.pollingIntervalSeconds !== policy.value.pollingIntervalSeconds)))
  const dirty = computed(() => editing.value && (changed.value || Boolean(form.reason.trim())))
  const canEdit = computed(() => Boolean(policy.value && !loading.value && !saving.value && !stale.value && !forbidden.value))
  function issue(field: Field): string {
    if (field === 'reason') { const text = form.reason.trim(); return text && Array.from(text).length <= 200 && !/[\p{Cc}]/u.test(text) && !/(?:password|secret|token|access[_-]?key)\s*[:=]|-----BEGIN/i.test(text) ? '' : '请输入 1–200 字的变更原因，不要填写密码或密钥。' }
    const value = form[field]
    if (field === 'heartbeatTimeoutSeconds') return bounded(value, 60, 3600) || value !== null && policy.value?.heartbeatTimeoutSource === 'CONTROL_PLANE' && value === policy.value.heartbeatTimeoutSeconds ? '' : '请输入 60–3600 的整数秒数。'
    if (value === null) return policy.value?.[field] === null ? '' : '已配置的策略不能通过清空重置。'
    return bounded(value, field === 'startupTimeoutSeconds' ? 15 : 1, field === 'startupTimeoutSeconds' ? 3600 : 60) ? '' : field === 'startupTimeoutSeconds' ? '请输入 15–3600 的整数秒数。' : '请输入 1–60 的整数秒数。'
  }
  watch(form, () => { if (!editing.value || saving.value) return; review.value = undefined; confirmed.value = false; for (const field of fields) if (errors[field]) errors[field] = issue(field) }, { flush: 'sync' })
  function clearForm() { editing.value = false; form.heartbeatTimeoutSeconds = null; form.startupTimeoutSeconds = null; form.pollingIntervalSeconds = null; form.reason = ''; for (const field of fields) errors[field] = ''; review.value = undefined; confirmed.value = false }
  function invalidate() { session++; reader?.abort(); writer?.abort(); reader = undefined; writer = undefined; loading.value = false; saving.value = false; retry = undefined; clearForm(); options.clearNotice() }
  async function read() {
    if (disposed || loading.value || saving.value || editing.value) return
    invalidate(); const expected = session, controller = new AbortController(); reader = controller
    loading.value = true; stale.value = true; forbidden.value = false; failure.value = ''
    try { const value = await api.getSchedulingTiming(controller.signal); if (current(expected)) { policy.value = value; stale.value = false; unconfirmed.value = false } }
    catch (error) { if (!current(expected)) return; policy.value = undefined; forbidden.value = Boolean(denied(error)); failure.value = forbidden.value ? '需要有效登录及系统管理员权限才能查看或修改调度时间策略。' : '无法读取调度时间策略，请重试；读取成功前不能编辑。' }
    finally { if (current(expected)) { loading.value = false; reader = undefined } }
  }
  function edit() { if (!canEdit.value || editing.value || !policy.value) return; invalidate(); failure.value = ''; editing.value = true; form.heartbeatTimeoutSeconds = policy.value.heartbeatTimeoutSeconds; form.startupTimeoutSeconds = policy.value.startupTimeoutSeconds; form.pollingIntervalSeconds = policy.value.pollingIntervalSeconds }
  function cancel() { if (disposed || saving.value) return; invalidate(); if (unconfirmed.value) { stale.value = true; failure.value = '上次时间策略保存结果尚未核对，请刷新当前策略。' } else failure.value = '' }
  function reviewChange() {
    if (!canEdit.value || !editing.value || review.value || !policy.value || !changed.value) return
    options.clearNotice(); failure.value = ''; for (const field of fields) errors[field] = issue(field)
    if (fields.some(field => errors[field])) return
    const before = policy.value
    const requiresConfirmation = form.heartbeatTimeoutSeconds! < before.heartbeatTimeoutSeconds || form.startupTimeoutSeconds !== null && (before.startupTimeoutSeconds === null || form.startupTimeoutSeconds < before.startupTimeoutSeconds) || form.pollingIntervalSeconds !== before.pollingIntervalSeconds
    const payload: SchedulingTimingWrite = Object.freeze({ heartbeatTimeoutSeconds: form.heartbeatTimeoutSeconds!, startupTimeoutSeconds: form.startupTimeoutSeconds, pollingIntervalSeconds: form.pollingIntervalSeconds, reason: form.reason.trim(), confirmed: requiresConfirmation })
    const fingerprint = JSON.stringify([before.revision, payload])
    if (retry?.fingerprint !== fingerprint) retry = { fingerprint, key: options.newKey?.() ?? crypto.randomUUID() }
    confirmed.value = false; review.value = { session, before, payload, key: retry.key }
  }
  function cancelReview() { if (!saving.value) { review.value = undefined; confirmed.value = false } }
  async function save() {
    const intent = review.value
    if (!intent || !current(intent.session) || saving.value || !editing.value || !canEdit.value || intent.before.revision !== policy.value?.revision || intent.payload.heartbeatTimeoutSeconds !== form.heartbeatTimeoutSeconds || intent.payload.startupTimeoutSeconds !== form.startupTimeoutSeconds || intent.payload.pollingIntervalSeconds !== form.pollingIntervalSeconds || intent.payload.reason !== form.reason.trim() || intent.payload.confirmed && !confirmed.value) return
    saving.value = true; failure.value = ''; options.clearNotice(); const controller = new AbortController(); writer = controller; let receiptConfirmed = false
    try {
      const result = await api.changeSchedulingTiming(intent.before.revision, intent.payload, intent.key, controller.signal)
      if (!current(intent.session)) return
      receiptConfirmed = true; clearForm(); retry = undefined; stale.value = true; policy.value = undefined
      const value = await api.getSchedulingTiming(controller.signal)
      if (!current(intent.session)) return
      policy.value = value; stale.value = false; unconfirmed.value = false
      options.onSaved(result.replayed ? '已确认原时间设置回执，实际策略以重新读取结果为准。' : result.changed ? '调度时间策略已保存，按各项生效范围执行。' : '调度时间策略没有变化。')
    } catch (error) {
      if (!current(intent.session)) return
      review.value = undefined; confirmed.value = false
      if (denied(error)) { clearForm(); policy.value = undefined; stale.value = true; forbidden.value = true; retry = undefined; failure.value = '登录或权限已失效，请重新登录并核对调度时间策略。' }
      else if (receiptConfirmed) failure.value = '时间设置回执已确认，但当前策略读取失败。请刷新并核对实际生效值。'
      else if (['REVISION_CONFLICT', 'IDEMPOTENCY_CONFLICT', 'SCHEDULING_TIMING_CONFIRMATION_REQUIRED'].includes(errorCode(error))) { clearForm(); stale.value = true; retry = undefined; failure.value = '策略、启动基线或请求版本已变化，未覆盖当前配置。请刷新后比较。' }
      else if (errorCode(error) === 'SCHEDULING_TIMING_FIELDS_INVALID') { errors.reason = '请核对时间范围及原因，不要填写密码或密钥。'; failure.value = '时间设置未保存，请修正字段后重试。' }
      else { unconfirmed.value = true; failure.value = '未确认时间设置保存成功。请核对输入后重试；相同内容会复用原请求标识。' }
    } finally { if (current(intent.session)) { saving.value = false; writer = undefined } }
  }
  function reset() { invalidate(); policy.value = undefined; stale.value = true; unconfirmed.value = false; forbidden.value = false; failure.value = '' }
  onScopeDispose(() => { disposed = true; reset() })
  return { policy, loading, saving, editing, stale, unconfirmed, forbidden, failure, form, errors, confirmed, review, changed, dirty, canEdit, read, edit, cancel, reviewChange, cancelReview, save, reset }
}
