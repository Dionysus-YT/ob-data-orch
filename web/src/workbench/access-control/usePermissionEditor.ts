import { computed, onScopeDispose, reactive, ref, shallowRef } from 'vue'
import type { AccessControlApi, PermissionCapability, PermissionChangeResult, PermissionProfile, PermissionRole, PermissionScopeObject, PermissionScopeType, PermissionWrite } from '@/api/accessControl'
import { permissionScopeKey } from '@/api/accessControl'
import { assignmentFingerprint, canonicalAssignment, cleanRoleDependencies, formFingerprint, permissionDenied, permissionDiff, permissionError, permissionFormError, permissionReasonError, scopeAdditionGated, type PermissionForm } from './permissionRules'

interface Review { readonly session: number; readonly fingerprint: string; readonly subjectId: string; readonly revision: number; readonly payload: PermissionWrite; readonly key: string }
type DiscardIntent = { session: number; kind: 'close' | 'reload' } | { session: number; kind: 'open'; id: string }

/** 单用户编辑器拥有唯一草稿、读取代和写事务，身份/关闭/切换使迟到响应失效。 */
export function usePermissionEditor(api: AccessControlApi, options: { canManage: () => boolean; currentSubjectId: () => string; onSaved: (result: PermissionChangeResult) => void; onDenied: () => void; newKey?: () => string }) {
  const open = ref(false); const subjectId = ref(''); const session = ref(0); const profile = shallowRef<PermissionProfile>()
  const form = reactive<PermissionForm>({ roles: [], capabilities: [], scopes: [], reason: '' })
  const loading = ref(false); const saving = ref(false); const failure = ref(''); const stale = ref(false)
  const reasonError = ref(''); const configError = ref(''); const discardOpen = ref(false); const review = shallowRef<Review>()
  const selectedObjects = shallowRef<readonly PermissionScopeObject[]>([])
  let disposed = false; let reader: AbortController | undefined; let writer: AbortController | undefined; let writeToken: symbol | undefined; let pending: DiscardIntent | undefined
  let retry: { fingerprint: string; key: string; session: number } | undefined
  const readOnly = computed(() => !options.canManage() || !profile.value?.editable || profile.value.subject.id === options.currentSubjectId() || stale.value)
  const dirty = computed(() => Boolean(profile.value && (assignmentFingerprint(form) !== assignmentFingerprint(profile.value.assignment) || form.reason.trim())))
  const changed = computed(() => Boolean(profile.value && assignmentFingerprint(form) !== assignmentFingerprint(profile.value.assignment)))
  const diff = computed(() => { if (!profile.value) return undefined; const value = permissionDiff(profile.value.assignment, form); return { ...value, productionRisk: value.addedScopes.some(scope => selectedObjects.value.some(item => permissionScopeKey(item) === permissionScopeKey(scope) && ['PROD', 'PRODUCTION'].includes(item.environment ?? ''))) } })
  const writable = () => open.value && Boolean(profile.value) && !readOnly.value && !loading.value && !saving.value && !review.value
  const valid = (expected: number, id: string) => !disposed && session.value === expected && subjectId.value === id && open.value

  function reset() { session.value++; reader?.abort(); reader = undefined; writer?.abort(); profile.value = undefined; subjectId.value = ''; open.value = false; loading.value = false; failure.value = ''; stale.value = false; reasonError.value = ''; configError.value = ''; discardOpen.value = false; pending = undefined; review.value = undefined; retry = undefined; selectedObjects.value = []; Object.assign(form, { roles: [], capabilities: [], scopes: [], reason: '' }) }
  function fill(value: PermissionProfile) { profile.value = value; Object.assign(form, canonicalAssignment(value.assignment), { reason: '' }); selectedObjects.value = value.scopeObjects; reasonError.value = ''; configError.value = ''; stale.value = false; retry = undefined }
  async function load(id: string, newSession: boolean) {
    if (disposed || writeToken) return
    if (newSession) { reset(); open.value = true; subjectId.value = id } else { session.value++; reader?.abort(); review.value = undefined; retry = undefined }
    const expected = session.value; const controller = new AbortController(); reader = controller
    loading.value = true; failure.value = ''; stale.value = Boolean(profile.value)
    try { const result = await api.getPermissionProfile(id, controller.signal); if (valid(expected, id)) fill(result) }
    catch (error) { if (!valid(expected, id)) return; if (permissionDenied(error)) { reset(); options.onDenied() } else { stale.value = true; failure.value = permissionError(error, '无法读取当前权限，编辑已阻断。请重试。') } }
    finally { if (valid(expected, id)) { loading.value = false; reader = undefined } }
  }
  function openSubject(id: string) { if (disposed || writeToken || !options.canManage()) return; if (dirty.value) { pending = { session: session.value, kind: 'open', id }; discardOpen.value = true; return }; return load(id, true) }
  function close() { if (saving.value || writeToken) return; if (dirty.value) { pending = { session: session.value, kind: 'close' }; discardOpen.value = true } else reset() }
  function refresh() { if (!open.value || writeToken || loading.value) return; if (dirty.value) { pending = { session: session.value, kind: 'reload' }; discardOpen.value = true } else return load(subjectId.value, false) }
  function cancelDiscard() { if (!saving.value) { discardOpen.value = false; pending = undefined } }
  function confirmDiscard() { const intent = pending; if (!intent || intent.session !== session.value || writeToken) return; discardOpen.value = false; pending = undefined; if (intent.kind === 'open') return load(intent.id, true); if (intent.kind === 'reload') { if (profile.value) fill(profile.value); return load(subjectId.value, false) }; reset() }
  function restore() { if (writable() && profile.value) { fill(profile.value); failure.value = '' } }
  function setRoles(roles: readonly PermissionRole[]) { if (!writable() || !profile.value) return; if (profile.value.subject.accountStatus !== 'ACTIVE' && roles.some(role => !profile.value!.assignment.roles.includes(role))) return; form.roles = [...roles]; cleanRoleDependencies(form); configError.value = ''; retry = undefined }
  function setCapabilities(capabilities: readonly PermissionCapability[]) { if (!writable() || !profile.value || capabilities.some(capability => (profile.value!.subject.accountStatus !== 'ACTIVE' || capability !== 'CAP_AUDIT_LOG') && !profile.value!.assignment.capabilities.includes(capability))) return; form.capabilities = [...capabilities]; cleanRoleDependencies(form); if (!capabilities.includes('CAP_PRODUCTION_TASK')) form.scopes = form.scopes.filter(scope => !['DATA_SOURCE_USE', 'TASK_OPERATE_BY_DATA_SOURCE'].includes(scope.type) || !selectedObjects.value.some(item => item.type === scope.type && item.objectId === scope.objectId && ['PROD', 'PRODUCTION'].includes(item.environment ?? ''))); retry = undefined }
  function setScopes(type: PermissionScopeType, ids: readonly string[]) { if (!writable() || !profile.value) return; const scopes = ids.map(objectId => ({ type, objectId })); if ((scopeAdditionGated(type) || profile.value.subject.accountStatus !== 'ACTIVE') && scopes.some(scope => !profile.value!.assignment.scopes.some(item => permissionScopeKey(item) === permissionScopeKey(scope)))) return; form.scopes = [...form.scopes.filter(item => item.type !== type), ...scopes]; configError.value = ''; retry = undefined }
  function rememberObjects(binding: number, values: readonly PermissionScopeObject[]) { if (binding !== session.value || !open.value || disposed) return; const map = new Map(selectedObjects.value.map(item => [permissionScopeKey(item), item])); for (const value of values) map.set(permissionScopeKey(value), value); selectedObjects.value = [...map.values()] }
  function scopeName(type: PermissionScopeType, id: string) { const object = selectedObjects.value.find(item => item.type === type && item.objectId === id); return object ? `${object.displayName}${['PROD', 'PRODUCTION'].includes(object.environment ?? '') ? ' · 生产' : ''}` : id }
  function startReview() {
    if (!writable() || !profile.value || !changed.value) return
    reasonError.value = permissionReasonError(form.reason); configError.value = permissionFormError(form, profile.value)
    if (reasonError.value || configError.value) return
    const fingerprint = formFingerprint(form)
    try { if (!retry || retry.fingerprint !== fingerprint || retry.session !== session.value) retry = { fingerprint, key: (options.newKey ?? (() => crypto.randomUUID()))(), session: session.value } }
    catch { failure.value = '无法建立安全的请求标识，请刷新页面。'; return }
    review.value = { session: session.value, fingerprint, subjectId: subjectId.value, revision: profile.value.revision, payload: { ...canonicalAssignment(form), reason: form.reason.trim(), confirmed: true }, key: retry.key }
  }
  function cancelReview() { if (!saving.value) review.value = undefined }
  async function submitReview() {
    const intent = review.value
    if (!intent || writeToken || disposed || !valid(intent.session, intent.subjectId) || readOnly.value || intent.fingerprint !== formFingerprint(form)) return
    const token = Symbol('permission-write'); writeToken = token; saving.value = true; failure.value = ''; const controller = new AbortController(); writer = controller
    try {
      const result = await api.changePermissionProfile(intent.subjectId, intent.revision, intent.payload, intent.key, controller.signal)
      if (!valid(intent.session, intent.subjectId)) return
      if (!options.canManage()) { stale.value = true; failure.value = '管理资格需要重新读取，未将旧回执当作当前权限。'; review.value = undefined; return }
      reset(); options.onSaved(result)
    } catch (error) {
      if (!valid(intent.session, intent.subjectId)) return
      review.value = undefined
      if (permissionDenied(error)) { reset(); options.onDenied() }
      else { failure.value = permissionError(error, '未确认权限保存成功，草稿已保留，可使用原请求重试。'); if ((error as { status?: number })?.status === 412 || (error as { code?: string })?.code === 'PERMISSION_SUBJECT_INACTIVE') stale.value = true }
    } finally { if (writeToken === token) { writeToken = undefined; saving.value = false; writer = undefined } }
  }
  onScopeDispose(() => { disposed = true; reset() })
  return { open, subjectId, session, profile, form, loading, saving, failure, stale, reasonError, configError, discardOpen, review, selectedObjects, readOnly, dirty, changed, diff, openSubject, close, refresh, cancelDiscard, confirmDiscard, restore, reset, setRoles, setCapabilities, setScopes, rememberObjects, scopeName, startReview, cancelReview, submitReview }
}
export type PermissionEditor = ReturnType<typeof usePermissionEditor>
