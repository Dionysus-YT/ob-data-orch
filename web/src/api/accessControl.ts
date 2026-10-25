export const permissionRoles = ['ROLE_BASIC_USER', 'ROLE_OPERATOR', 'ROLE_DATA_SOURCE_ADMIN', 'ROLE_NODE_ADMIN', 'ROLE_SYSTEM_ADMIN'] as const
export const permissionCapabilities = ['CAP_PRODUCTION_TASK', 'CAP_EXPERT_CONFIG', 'CAP_SENSITIVE_COMMAND', 'CAP_AUDIT_LOG', 'CAP_GLOBAL_SCHEDULING_LOG'] as const
export const permissionScopeTypes = ['DATA_SOURCE_USE', 'DATA_SOURCE_MANAGE', 'NODE_USE', 'NODE_OPERATE', 'NODE_MANAGE', 'TASK_OPERATE_BY_DATA_SOURCE', 'TEMPLATE_USE', 'TEMPLATE_MANAGE'] as const
export type PermissionRole = typeof permissionRoles[number]
export type PermissionCapability = typeof permissionCapabilities[number]
export type PermissionScopeType = typeof permissionScopeTypes[number]
export interface PermissionScope { readonly type: PermissionScopeType; readonly objectId: string }
export interface PermissionAssignment { readonly roles: readonly PermissionRole[]; readonly capabilities: readonly PermissionCapability[]; readonly scopes: readonly PermissionScope[] }
export interface PermissionScopeObject extends PermissionScope { readonly displayName: string; readonly environment?: string; readonly available: boolean }
export interface PermissionProfile {
  readonly subject: { readonly id: string; readonly login: string; readonly displayName: string; readonly accountStatus: 'ACTIVE' | 'DISABLED' | 'UNKNOWN' }
  readonly assignment: PermissionAssignment; readonly revision: number; readonly modifiedBy?: string; readonly modifiedAt?: string
  readonly initialAdministrator: boolean; readonly editable: boolean; readonly scopeObjects: readonly PermissionScopeObject[]
}
export interface PermissionFilters { readonly search?: string; readonly role?: PermissionRole; readonly status?: 'ACTIVE' | 'DISABLED' | 'UNKNOWN'; readonly capability?: PermissionCapability; readonly limit?: number; readonly cursor?: string }
export interface PermissionPage { readonly currentSubjectId: string; readonly directorySource: 'EXISTING_AUTH_SUBJECTS'; readonly directoryStatus: 'PROJECTED'; readonly timeZone: 'UTC'; readonly observedAt: string; readonly items: readonly PermissionProfile[]; readonly hasMore: boolean; readonly nextCursor?: string }
export interface PermissionCandidate { readonly id: string; readonly displayName: string; readonly kind: 'DATA_SOURCE' | 'NODE' | 'TEMPLATE'; readonly environment?: string; readonly available: boolean }
export interface PermissionCandidatePage { readonly items: readonly PermissionCandidate[]; readonly hasMore: boolean; readonly nextCursor?: string }
export interface PermissionWrite extends PermissionAssignment { readonly reason: string; readonly confirmed: boolean }
export interface PermissionChangeResult { readonly revision: number; readonly changed: boolean; readonly replayed: boolean }
export interface AccessControlApi {
  getPermissionSubjects(filters: PermissionFilters, signal?: AbortSignal): Promise<PermissionPage>
  getPermissionProfile(subjectId: string, signal?: AbortSignal): Promise<PermissionProfile>
  getPermissionCandidates(type: PermissionScopeType, search?: string, cursor?: string, signal?: AbortSignal): Promise<PermissionCandidatePage>
  changePermissionProfile(subjectId: string, revision: number, payload: PermissionWrite, idempotencyKey: string, signal?: AbortSignal): Promise<PermissionChangeResult>
}
interface Transport { read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>; write(path: string, body: PermissionWrite, revision: number, key: string, signal?: AbortSignal): Promise<Record<string, unknown>> }

function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '权限数据无法校验，请刷新并核对网页与控制面版本。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown): Record<string, unknown> { return value && typeof value === 'object' && !Array.isArray(value) && [Object.prototype, null].includes(Object.getPrototypeOf(value)) ? value as Record<string, unknown> : invalid() }
function known(value: Record<string, unknown>, keys: readonly string[]) { if (Object.keys(value).some(key => !keys.includes(key))) invalid() }
function text(value: unknown, max = 256, empty = false): string { return typeof value === 'string' && (empty || value.length > 0) && value.length <= max && !Array.from(value).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) >= 127 && char.charCodeAt(0) <= 159) ? value : invalid() }
function id(value: unknown): string { const result = text(value, 200); return Array.from(result).some(char => /\s/.test(char) || '/\\?#*[]'.includes(char)) ? invalid() : result }
function bool(value: unknown): boolean { return typeof value === 'boolean' ? value : invalid() }
function revision(value: unknown): number { return typeof value === 'number' && Number.isSafeInteger(value) && value > 0 ? value : invalid() }
function choice<T extends string>(value: unknown, choices: readonly T[]): T { return choices.includes(value as T) ? value as T : invalid() }
function array<T>(value: unknown, limit: number, parse: (item: unknown) => T, key: (item: T) => string): readonly T[] { if (!Array.isArray(value) || value.length > limit) return invalid(); const result = value.map(parse); return new Set(result.map(key)).size === result.length ? result : invalid() }
function instant(value: unknown): string { const result = text(value, 64); return /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(result) && Number.isFinite(Date.parse(result)) ? result : invalid() }
export function permissionScopeKey(value: PermissionScope): string { return `${value.type}:${value.objectId}` }
function scope(value: unknown): PermissionScope { const row = object(value); known(row, ['type', 'objectId']); return { type: choice(row.type, permissionScopeTypes), objectId: id(row.objectId) } }
function assignment(value: unknown): PermissionAssignment {
  const row = object(value); known(row, ['roles', 'capabilities', 'scopes'])
  return { roles: array(row.roles, 5, value => choice(value, permissionRoles), value => value), capabilities: array(row.capabilities, 5, value => choice(value, permissionCapabilities), value => value), scopes: array(row.scopes, 200, scope, permissionScopeKey) }
}
function scopeObject(value: unknown): PermissionScopeObject { const row = object(value); known(row, ['type', 'objectId', 'displayName', 'environment', 'available']); return { type: choice(row.type, permissionScopeTypes), objectId: id(row.objectId), displayName: text(row.displayName), available: bool(row.available), ...(row.environment === undefined ? {} : { environment: text(row.environment, 40) }) } }

/** 对完整主体版本严格解码，未知字段、重复范围或不属于配置的对象摘要均拒绝。 */
export function parsePermissionProfile(value: unknown, requestedId?: string): PermissionProfile {
  const row = object(value); known(row, ['subject', 'assignment', 'revision', 'modifiedBy', 'modifiedAt', 'initialAdministrator', 'editable', 'scopeObjects'])
  const subject = object(row.subject); known(subject, ['id', 'login', 'displayName', 'accountStatus'])
  const result: PermissionProfile = { subject: { id: id(subject.id), login: text(subject.login), displayName: text(subject.displayName, 256, true), accountStatus: choice(subject.accountStatus, ['ACTIVE', 'DISABLED', 'UNKNOWN']) }, assignment: assignment(row.assignment), revision: revision(row.revision), initialAdministrator: bool(row.initialAdministrator), editable: bool(row.editable), scopeObjects: array(row.scopeObjects, 200, scopeObject, permissionScopeKey), ...(row.modifiedBy === undefined ? {} : { modifiedBy: id(row.modifiedBy) }), ...(row.modifiedAt === undefined ? {} : { modifiedAt: instant(row.modifiedAt) }) }
  if (requestedId !== undefined && result.subject.id !== requestedId || result.scopeObjects.some(item => !result.assignment.scopes.some(scope => permissionScopeKey(scope) === permissionScopeKey(item)))) invalid()
  return result
}
function pagination(row: Record<string, unknown>, size: number) { const hasMore = bool(row.hasMore); const nextCursor = row.nextCursor === undefined ? undefined : text(row.nextCursor, 2048); if (hasMore !== (nextCursor !== undefined) || hasMore && size === 0) invalid(); return { hasMore, ...(nextCursor ? { nextCursor } : {}) } }
function candidate(value: unknown): PermissionCandidate { const row = object(value); known(row, ['id', 'displayName', 'kind', 'environment', 'available']); return { id: id(row.id), displayName: text(row.displayName), kind: choice(row.kind, ['DATA_SOURCE', 'NODE', 'TEMPLATE']), available: bool(row.available), ...(row.environment === undefined ? {} : { environment: text(row.environment, 40) }) } }

/** 所有读写经既有浏览器安全传输；调用方只能提交业务意图，不能提交操作者或身份。 */
export function createAccessControlApi(transport: Transport): AccessControlApi {
  return {
    async getPermissionSubjects(filters, signal) {
      const query = new URLSearchParams()
      for (const [key, value] of Object.entries(filters)) if (value !== undefined && value !== '') query.set(key, String(value))
      const row = await transport.read(`/api/v1/access-control/subjects${query.size ? `?${query}` : ''}`, signal)
      known(row, ['requestId', 'currentSubjectId', 'directorySource', 'directoryStatus', 'timeZone', 'observedAt', 'items', 'hasMore', 'nextCursor'])
      const currentSubjectId = id(row.currentSubjectId)
      const items = array(row.items, 50, value => parsePermissionProfile(value), value => value.subject.id)
      if (items.some(item => item.editable !== (item.subject.id !== currentSubjectId))) invalid()
      return { currentSubjectId, directorySource: choice(row.directorySource, ['EXISTING_AUTH_SUBJECTS']), directoryStatus: choice(row.directoryStatus, ['PROJECTED']), timeZone: choice(row.timeZone, ['UTC']), observedAt: instant(row.observedAt), items, ...pagination(row, items.length) }
    },
    async getPermissionProfile(subjectId, signal) { const row = await transport.read(`/api/v1/access-control/subjects/${encodeURIComponent(id(subjectId))}`, signal); known(row, ['requestId', 'item']); return parsePermissionProfile(row.item, subjectId) },
    async getPermissionCandidates(type, search = '', cursor, signal) {
      choice(type, permissionScopeTypes)
      const query = new URLSearchParams({ type, limit: '50' }); if (search) query.set('search', search); if (cursor) query.set('cursor', cursor)
      const row = await transport.read(`/api/v1/access-control/candidates?${query}`, signal); known(row, ['requestId', 'items', 'hasMore', 'nextCursor'])
      const items = array(row.items, 50, candidate, value => value.id)
      const kind = type.startsWith('TEMPLATE_') ? 'TEMPLATE' : type.startsWith('NODE_') ? 'NODE' : 'DATA_SOURCE'
      if (items.some(item => item.kind !== kind)) invalid()
      return { items, ...pagination(row, items.length) }
    },
    async changePermissionProfile(subjectId, currentRevision, payload, key, signal) {
      id(subjectId); revision(currentRevision); text(key, 128)
      const config = assignment({ roles: payload.roles, capabilities: payload.capabilities, scopes: payload.scopes })
      const reason = text(payload.reason, 400)
      if (Array.from(reason).length > 200) invalid()
      const row = await transport.write(`/api/v1/access-control/subjects/${encodeURIComponent(subjectId)}`, { roles: [...config.roles], capabilities: [...config.capabilities], scopes: config.scopes.map(value => ({ type: value.type, objectId: value.objectId })), reason, confirmed: bool(payload.confirmed) }, currentRevision, key, signal)
      known(row, ['requestId', 'item']); const result = object(row.item); known(result, ['revision', 'changed', 'replayed'])
      const parsed = { revision: revision(result.revision), changed: bool(result.changed), replayed: bool(result.replayed) }
      if (parsed.revision !== currentRevision + (parsed.changed ? 1 : 0)) invalid()
      return parsed
    },
  }
}
