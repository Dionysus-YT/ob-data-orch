import type { Page } from '@playwright/test'
import type { PermissionProfile, PermissionRole, PermissionScopeType, PermissionWrite } from '../../src/api/accessControl'
import { retentionPolicy } from './log-retention'
import { schedulingPolicy } from './scheduling-policy'
import { timingPolicy } from './scheduling-timing'
import { candidatePolicy } from './scheduling-candidates'
import { securityPolicy } from './security-policy'
import { toolCompatibility } from './tool-compatibility'

export function permissionProfile(id = 'user-a'): PermissionProfile {
  return { subject: { id, login: id === 'admin' ? 'admin' : `synthetic-${id}`, displayName: id === 'admin' ? '本机管理员' : id === 'disabled-user' ? '已停用合成用户' : '合成业务用户', accountStatus: id === 'disabled-user' ? 'DISABLED' : 'ACTIVE' }, assignment: { roles: id === 'admin' ? ['ROLE_BASIC_USER', 'ROLE_OPERATOR', 'ROLE_DATA_SOURCE_ADMIN', 'ROLE_NODE_ADMIN', 'ROLE_SYSTEM_ADMIN'] : ['ROLE_BASIC_USER'], capabilities: [], scopes: [{ type: 'DATA_SOURCE_USE', objectId: 'source-1' }, { type: 'NODE_USE', objectId: 'node-1' }] }, revision: 1, modifiedBy: 'admin', modifiedAt: '2026-10-23T04:00:00Z', initialAdministrator: id === 'admin', editable: id !== 'admin', scopeObjects: [{ type: 'DATA_SOURCE_USE', objectId: 'source-1', displayName: '合成测试数据源', environment: 'TEST', available: true }, { type: 'NODE_USE', objectId: 'node-1', displayName: '合成 Windows 节点', available: true }] }
}
export type PermissionMode = 'ready' | 'empty' | 'error' | 'denied' | 'conflict' | 'write-error' | 'long' | 'replay'
export async function permissionFixture(page: Page, initial: PermissionMode = 'ready') {
  let mode = initial; let user = permissionProfile(); const writes: { path: string; body: PermissionWrite; headers: Record<string, string> }[] = []; const calls: string[] = []
  await page.route('**/api/v1/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname; calls.push(`${request.method()} ${path}${url.search}`)
    const fail = (status: number, code: string) => route.fulfill({ status, json: { error: { code, message: 'synthetic-only', retryable: status >= 500 } } })
    if (path === '/api/v1/settings/scheduling') return route.fulfill({ json: { item: schedulingPolicy() } })
    if (path === '/api/v1/settings/scheduling/timing') return route.fulfill({ json: { item: timingPolicy() } })
    if (path === '/api/v1/settings/scheduling/candidates') return route.fulfill({ json: { item: candidatePolicy() } })
    if (path === '/api/v1/settings/security') return route.fulfill({ json: { item: securityPolicy() } })
    if (path === '/api/v1/settings/tools') return route.fulfill({ json: { item: toolCompatibility() } })
    if (path === '/api/v1/settings/log-retention') return route.fulfill({ json: { item: retentionPolicy() } })
    if (!path.startsWith('/api/v1/access-control/')) return route.fulfill({ json: { items: [] } })
    if (mode === 'denied') return fail(404, 'NOT_FOUND')
    if (mode === 'error') return fail(503, 'PERMISSIONS_UNAVAILABLE')
    if (path === '/api/v1/access-control/subjects') {
      let items = [permissionProfile('admin'), user, permissionProfile('disabled-user')]
      if (mode === 'long') items = items.map(item => ({ ...item, subject: { ...item.subject, displayName: '合成超长权限配置用户名称'.repeat(5), login: 'synthetic-account-'.repeat(10) } }))
      const search = url.searchParams.get('search'), role = url.searchParams.get('role') as PermissionRole | null, status = url.searchParams.get('status')
      if (search) items = items.filter(item => `${item.subject.login} ${item.subject.displayName}`.includes(search))
      if (role) items = items.filter(item => item.assignment.roles.includes(role))
      if (status) items = items.filter(item => item.subject.accountStatus === status)
      if (mode === 'empty') items = []
      return route.fulfill({ json: { requestId: 'synthetic-list', currentSubjectId: 'admin', directorySource: 'EXISTING_AUTH_SUBJECTS', directoryStatus: 'PROJECTED', timeZone: 'UTC', observedAt: '2026-10-23T04:00:00Z', items, hasMore: false } })
    }
    if (path === '/api/v1/access-control/candidates') {
      const type = url.searchParams.get('type') as PermissionScopeType; const kind = type.startsWith('NODE') ? 'NODE' : type.startsWith('TEMPLATE') ? 'TEMPLATE' : 'DATA_SOURCE'
      return route.fulfill({ json: { requestId: 'synthetic-candidates', items: [{ id: kind === 'NODE' ? 'node-1' : kind === 'TEMPLATE' ? 'template-1' : 'source-1', kind, displayName: kind === 'NODE' ? '合成 Windows 节点' : kind === 'TEMPLATE' ? '合成 CSV 模板' : '合成测试数据源', available: true, ...(kind === 'DATA_SOURCE' ? { environment: 'TEST' } : {}) }], hasMore: false } })
    }
    const id = decodeURIComponent(path.split('/').at(-1) ?? '')
    if (request.method() === 'PATCH') {
      writes.push({ path, body: request.postDataJSON() as PermissionWrite, headers: request.headers() })
      if (mode === 'conflict') return fail(412, 'REVISION_CONFLICT')
      if (mode === 'write-error') return fail(503, 'PERMISSIONS_UNAVAILABLE')
      if (mode === 'replay') { user = { ...user, revision: 3, assignment: { roles: [], capabilities: [], scopes: [] }, scopeObjects: [] }; return route.fulfill({ json: { requestId: 'synthetic-old-receipt', item: { revision: 2, changed: true, replayed: true } } }) }
      const body = request.postDataJSON() as PermissionWrite
      user = { ...user, assignment: { roles: body.roles, capabilities: body.capabilities, scopes: body.scopes }, revision: user.revision + 1, scopeObjects: user.scopeObjects.filter(item => body.scopes.some(scope => scope.type === item.type && scope.objectId === item.objectId)) }
      return route.fulfill({ json: { requestId: 'synthetic-save', item: { revision: user.revision, changed: true, replayed: false } } })
    }
    return route.fulfill({ json: { requestId: 'synthetic-detail', item: id === 'user-a' ? user : permissionProfile(id) } })
  })
  return { writes, calls, setMode: (value: PermissionMode) => { mode = value } }
}
