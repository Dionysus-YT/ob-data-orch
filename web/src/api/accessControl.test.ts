import { describe, expect, it, vi } from 'vitest'
import { createAccessControlApi, parsePermissionProfile } from './accessControl'

const profile = () => ({ subject: { id: 'user-1', login: 'synthetic-user', displayName: '合成用户', accountStatus: 'ACTIVE' }, assignment: { roles: ['ROLE_BASIC_USER'], capabilities: [], scopes: [{ type: 'NODE_USE', objectId: 'node-1' }] }, revision: 1, initialAdministrator: false, editable: true, scopeObjects: [] })
describe('权限 API 严格投影与安全传输', () => {
  it.each([
    (row: ReturnType<typeof profile>) => ({ ...row, extra: true }),
    (row: ReturnType<typeof profile>) => ({ ...row, revision: 0 }),
    (row: ReturnType<typeof profile>) => ({ ...row, subject: { ...row.subject, id: '../other' } }),
    (row: ReturnType<typeof profile>) => ({ ...row, assignment: { ...row.assignment, roles: ['ROLE_BASIC_USER', 'ROLE_BASIC_USER'] } }),
    (row: ReturnType<typeof profile>) => ({ ...row, scopeObjects: [{ type: 'NODE_USE', objectId: 'foreign', displayName: '合成', available: true }] }),
    (row: ReturnType<typeof profile>) => ({ ...row, modifiedAt: 'not-a-time' }),
  ])('未知字段或不可信投影失败关闭 %#', change => { expect(() => parsePermissionProfile(change(profile()), 'user-1')).toThrow() })
  it('目标绑定和分页元信息不能自相矛盾，候选必须属于请求类型', async () => {
    expect(() => parsePermissionProfile(profile(), 'other')).toThrow()
    const read = vi.fn(async () => ({ currentSubjectId: 'user-1', directorySource: 'EXISTING_AUTH_SUBJECTS', directoryStatus: 'PROJECTED', timeZone: 'UTC', observedAt: '2026-10-23T00:00:00Z', items: [profile()], hasMore: false })); const api = createAccessControlApi({ read, write: vi.fn() }); await expect(api.getPermissionSubjects({})).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    read.mockResolvedValue({ items: [{ id: 'source-1', displayName: '合成', kind: 'DATA_SOURCE', available: true }], hasMore: false } as never); await expect(api.getPermissionCandidates('NODE_USE')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    read.mockResolvedValue({ items: [], hasMore: true, nextCursor: 'synthetic-cursor' } as never); await expect(api.getPermissionCandidates('NODE_USE')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('完整白名单载荷携带版本、幂等键和取消信号，不传操作者', async () => {
    const write = vi.fn(async () => ({ item: { revision: 2, changed: true, replayed: false } })); const api = createAccessControlApi({ read: vi.fn(), write }), controller = new AbortController()
    await api.changePermissionProfile('user-1', 1, { roles: [], capabilities: [], scopes: [], reason: '𠮷'.repeat(200), confirmed: true, actor: 'untrusted' } as never, 'synthetic-key', controller.signal)
    expect(write.mock.calls[0]).toEqual(['/api/v1/access-control/subjects/user-1', { roles: [], capabilities: [], scopes: [], reason: '𠮷'.repeat(200), confirmed: true }, 1, 'synthetic-key', controller.signal])
    write.mockResolvedValue({ item: { revision: 9, changed: true, replayed: false } }); await expect(api.changePermissionProfile('user-1', 1, { roles: [], capabilities: [], scopes: [], reason: '调整', confirmed: true }, 'synthetic-key')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
