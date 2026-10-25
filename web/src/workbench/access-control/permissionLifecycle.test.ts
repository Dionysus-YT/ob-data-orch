import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AccessControlApi, PermissionPage, PermissionProfile } from '@/api/accessControl'
import { permissionFormError, permissionReasonError, scopeRoleAllowed } from './permissionRules'
import { usePermissionEditor } from './usePermissionEditor'
import { usePermissionList } from './usePermissionList'
import { usePermissionCandidates } from './usePermissionCandidates'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers() })
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function profile(id = 'user-a', editable = true): PermissionProfile { return { subject: { id, login: id, displayName: `合成用户 ${id}`, accountStatus: 'ACTIVE' }, assignment: { roles: ['ROLE_BASIC_USER'], capabilities: [], scopes: [{ type: 'DATA_SOURCE_USE', objectId: 'source-1' }, { type: 'NODE_USE', objectId: 'node-1' }] }, revision: 1, editable, initialAdministrator: false, scopeObjects: [{ type: 'DATA_SOURCE_USE', objectId: 'source-1', displayName: '合成数据源', environment: 'TEST', available: true }] } }
function api(): AccessControlApi { return { getPermissionSubjects: vi.fn(), getPermissionProfile: vi.fn(async id => profile(id)), getPermissionCandidates: vi.fn(async () => ({ items: [], hasMore: false })), changePermissionProfile: vi.fn(async () => ({ revision: 2, changed: true, replayed: false })) } }
function setup(port = api()) { const scope = effectScope(); scopes.push(scope); const onSaved = vi.fn(), onDenied = vi.fn(); const model = scope.run(() => usePermissionEditor(port, { canManage: () => true, currentSubjectId: () => 'admin', onSaved, onDenied, newKey: () => 'synthetic-stable-key' }))!; return { port, model, onSaved, onDenied, scope } }
function edit(model: ReturnType<typeof setup>['model']) { model.setRoles(['ROLE_BASIC_USER', 'ROLE_NODE_ADMIN']); model.form.reason = '调整合成职责' }
const page = (id = 'user-a', actor = 'admin'): PermissionPage => ({ currentSubjectId: actor, directorySource: 'EXISTING_AUTH_SUBJECTS', directoryStatus: 'PROJECTED', timeZone: 'UTC', observedAt: '2026-10-23T00:00:00Z', items: [profile(id)], hasMore: false })

describe('权限编辑事务与会话', () => {
  it('审计能力可为其他 ACTIVE 主体显式授予并进入风险确认，自身仍只读', async () => {
    const { model } = setup(); await model.openSubject('user-a'); model.setCapabilities(['CAP_AUDIT_LOG']); model.form.reason = '授予合成审计职责'; model.startReview(); expect(model.configError.value).toBe(''); expect(model.form.capabilities).toEqual(['CAP_AUDIT_LOG']); expect(model.diff.value?.risk).toBe(true); expect(model.review.value).toBeTruthy()
  })

  it('生产对象新增进入独立风险提示和带环境的确认名称', async () => {
    const { model } = setup(); await model.openSubject('user-a'); model.setRoles(['ROLE_BASIC_USER', 'ROLE_DATA_SOURCE_ADMIN']); model.rememberObjects(model.session.value, [{ type: 'DATA_SOURCE_MANAGE', objectId: 'production-source', displayName: '合成生产数据源', environment: 'PRODUCTION', available: true }]); model.setScopes('DATA_SOURCE_MANAGE', ['production-source']); expect(model.diff.value?.productionRisk).toBe(true); expect(model.scopeName('DATA_SOURCE_MANAGE', 'production-source')).toBe('合成生产数据源 · 生产'); model.setScopes('DATA_SOURCE_MANAGE', []); expect(model.diff.value?.productionRisk).toBe(false)
  })
  it('自身只读，独立能力和节点运行权限不允许新增，角色撤销清理依赖', async () => {
    const { model, port } = setup(); await model.openSubject('admin'); model.setRoles([]); model.startReview(); expect(model.readOnly.value).toBe(true); expect(port.changePermissionProfile).not.toHaveBeenCalled()
    await model.openSubject('user-a'); model.setCapabilities(['CAP_EXPERT_CONFIG']); model.setScopes('NODE_OPERATE', ['node-1']); expect(model.form.capabilities).toEqual([]); expect(model.form.scopes.some(scope => scope.type === 'NODE_OPERATE')).toBe(false)
    model.setRoles([]); expect(model.form.scopes).toEqual([]); expect(model.diff.value?.removedScopes).toHaveLength(2)
  })
  it('A → B → A 的迟到成功、错误和 finally 均不覆盖新会话', async () => {
    const { port, model, onDenied } = setup(); const a1 = deferred<PermissionProfile>(), b = deferred<PermissionProfile>(), a2 = deferred<PermissionProfile>()
    vi.mocked(port.getPermissionProfile).mockImplementationOnce(() => a1.promise).mockImplementationOnce(() => b.promise).mockImplementationOnce(() => a2.promise)
    const first = model.openSubject('user-a'), second = model.openSubject('user-b'), third = model.openSubject('user-a')
    a1.resolve({ ...profile(), revision: 9 }); b.reject({ status: 404 }); await first; await second
    expect(model.loading.value).toBe(true); expect(model.profile.value).toBeUndefined(); expect(onDenied).not.toHaveBeenCalled()
    a2.resolve(profile()); await third; expect(model.profile.value?.revision).toBe(1)
  })
  it('未确认不写入，重复确认只发一次，确认载荷与版本绑定', async () => {
    const { port, model, onSaved } = setup(); await model.openSubject('user-a'); edit(model); model.startReview(); expect(port.changePermissionProfile).not.toHaveBeenCalled()
    const write = deferred<{ revision: number; changed: boolean; replayed: boolean }>(); vi.mocked(port.changePermissionProfile).mockReturnValue(write.promise)
    const first = model.submitReview(); await model.submitReview(); model.setRoles([]); expect(port.changePermissionProfile).toHaveBeenCalledTimes(1); expect(model.form.roles).toContain('ROLE_NODE_ADMIN')
    expect(vi.mocked(port.changePermissionProfile).mock.calls[0]?.slice(0, 4)).toEqual(['user-a', 1, { roles: ['ROLE_BASIC_USER', 'ROLE_NODE_ADMIN'], capabilities: [], scopes: [{ type: 'DATA_SOURCE_USE', objectId: 'source-1' }, { type: 'NODE_USE', objectId: 'node-1' }], reason: '调整合成职责', confirmed: true }, 'synthetic-stable-key'])
    write.resolve({ revision: 2, changed: true, replayed: false }); await first; expect(onSaved).toHaveBeenCalledWith({ revision: 2, changed: true, replayed: false }); expect(model.open.value).toBe(false)
  })
  it('不确定失败保留草稿与幂等键，版本冲突阻断覆盖', async () => {
    const { port, model } = setup(); await model.openSubject('user-a'); edit(model); const key = vi.fn(() => 'unused')
    vi.mocked(port.changePermissionProfile).mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce({ status: 412 })
    model.startReview(); const previous = model.review.value?.key; await model.submitReview(); expect(model.dirty.value).toBe(true); model.startReview(); expect(model.review.value?.key).toBe(previous)
    await model.submitReview(); expect(model.stale.value).toBe(true); expect(model.readOnly.value).toBe(true); model.startReview(); expect(model.review.value).toBeUndefined(); expect(key).not.toHaveBeenCalled()
    model.refresh(); expect(model.discardOpen.value).toBe(true); model.cancelDiscard(); expect(model.form.reason).toBe('调整合成职责')
  })
  it('权限拒绝清空草稿，卸载或重置后的写回执不重放成功', async () => {
    const denied = setup(); await denied.model.openSubject('user-a'); edit(denied.model); denied.model.startReview(); vi.mocked(denied.port.changePermissionProfile).mockRejectedValue({ status: 403 }); await denied.model.submitReview(); expect(denied.onDenied).toHaveBeenCalledOnce(); expect(denied.model.form.roles).toEqual([])
    for (const stop of [false, true]) { const { model, port, scope, onSaved } = setup(); await model.openSubject('user-a'); edit(model); model.startReview(); const write = deferred<{ revision: number; changed: boolean; replayed: boolean }>(); vi.mocked(port.changePermissionProfile).mockReturnValue(write.promise); const saving = model.submitReview(); if (stop) scope.stop(); else model.reset(); write.resolve({ revision: 2, changed: true, replayed: false }); await saving; expect(onSaved).not.toHaveBeenCalled(); expect(model.profile.value).toBeUndefined() }
  })
  it('关闭和切换需显式放弃草稿，停用账号仅可收缩', async () => {
    const { model, port } = setup(); vi.mocked(port.getPermissionProfile).mockResolvedValue({ ...profile(), subject: { ...profile().subject, accountStatus: 'DISABLED' } }); await model.openSubject('user-a'); model.setRoles(['ROLE_BASIC_USER', 'ROLE_OPERATOR']); expect(model.form.roles).toEqual(['ROLE_BASIC_USER']); model.setRoles([]); model.close(); expect(model.open.value).toBe(true); model.cancelDiscard(); expect(model.dirty.value).toBe(true); await model.openSubject('user-b'); await model.confirmDiscard(); expect(model.subjectId.value).toBe('user-b')
  })
})

describe('权限目录、候选与规则', () => {
  it('分页失败仍对应原来的可信页，不把旧行计为下一页', async () => {
    const port = api(), scope = effectScope(); scopes.push(scope); const model = scope.run(() => usePermissionList(port, { onDenied: vi.fn(), onIdentityChanged: vi.fn() }))!
    vi.mocked(port.getPermissionSubjects).mockResolvedValueOnce({ ...page(), hasMore: true, nextCursor: 'synthetic-next' }).mockRejectedValueOnce({ status: 503 })
    await model.read(); await model.next(); expect(model.pageNumber.value).toBe(1); expect(model.query.value.cursor).toBeUndefined(); expect(model.items.value[0]?.subject.id).toBe('user-a'); expect(model.granted.value).toBe(false)
  })
  it('确认放弃再重读失败时不恢复已放弃的草稿', async () => {
    const { port, model } = setup(); await model.openSubject('user-a'); edit(model); model.refresh(); vi.mocked(port.getPermissionProfile).mockRejectedValueOnce({ status: 503 }); await model.confirmDiscard(); expect(model.form.reason).toBe(''); expect(model.form.roles).toEqual(['ROLE_BASIC_USER']); expect(model.stale.value).toBe(true)
  })
  it('最新筛选接管列表，迟到拒绝不清空新结果，当前拒绝不保留数量', async () => {
    const port = api(), scope = effectScope(); scopes.push(scope); const denied = vi.fn(); const model = scope.run(() => usePermissionList(port, { onDenied: denied, onIdentityChanged: vi.fn() }))!
    const old = deferred<PermissionPage>(), next = deferred<PermissionPage>(); vi.mocked(port.getPermissionSubjects).mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise)
    const first = model.read(); model.form.search = '最新'; const second = model.apply(); old.reject({ status: 404 }); await first; expect(model.loading.value).toBe(true); expect(denied).not.toHaveBeenCalled(); next.resolve(page('latest')); await second; expect(model.items.value[0]?.subject.id).toBe('latest')
    vi.mocked(port.getPermissionSubjects).mockRejectedValue({ status: 404 }); await model.read(); expect(model.items.value).toEqual([]); expect(model.metadata.value).toBeUndefined(); expect(model.granted.value).toBe(false)
  })
  it('候选绑定编辑代，搜索去抖，卸载后不请求或回放拒绝', async () => {
    vi.useFakeTimers(); const port = api(), scope = effectScope(), binding = ref(1); scopes.push(scope); const denied = vi.fn(); const model = scope.run(() => usePermissionCandidates(port, () => binding.value, () => 'NODE_USE', denied))!
    const old = deferred<{ items: []; hasMore: false }>(); vi.mocked(port.getPermissionCandidates).mockReturnValueOnce(old.promise); const first = model.begin(); binding.value = 2; old.reject({ status: 404 }); await first; expect(denied).not.toHaveBeenCalled()
    model.setSearch('a'); model.setSearch('b'); await vi.advanceTimersByTimeAsync(200); expect(port.getPermissionCandidates).toHaveBeenCalledTimes(2); expect(vi.mocked(port.getPermissionCandidates).mock.calls[1]?.slice(0, 2)).toEqual(['NODE_USE', 'b']); scope.stop(); model.setSearch('after'); await vi.advanceTimersByTimeAsync(500); expect(port.getPermissionCandidates).toHaveBeenCalledTimes(2)
  })
  it('收缩策略、潜伏授权与秘密输入在确认前拒绝', () => {
    expect(scopeRoleAllowed(['ROLE_OPERATOR'], 'DATA_SOURCE_USE')).toBe(false); expect(scopeRoleAllowed(['ROLE_DATA_SOURCE_ADMIN'], 'NODE_USE')).toBe(true)
    expect(permissionReasonError('token=synthetic')).not.toBe(''); expect(permissionReasonError('𠮷'.repeat(200))).toBe(''); expect(permissionReasonError('𠮷'.repeat(201))).not.toBe('')
    expect(permissionFormError({ ...profile().assignment, roles: ['ROLE_BASIC_USER'], capabilities: ['CAP_EXPERT_CONFIG'], scopes: [], reason: '调整' }, profile())).not.toBe('')
  })
})
