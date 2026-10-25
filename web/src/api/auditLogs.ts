export const auditObjectTypes = ['TASK', 'TASK_EXECUTION', 'DATA_SOURCE', 'EXECUTION_NODE', 'AGENT', 'EXPORT_DRAFT', 'SUBJECT', 'PLATFORM_SETTING', 'STORAGE_CREDENTIAL', 'LOG_QUERY'] as const
export interface AuditFilter { from: string; to: string; actorId: string; objectType: string; objectId: string; action: string; result: string; keyword: string; limit: number }
export interface AuditField { readonly name: string; readonly value: string }
export interface AuditItem { readonly token: string; readonly id: string; readonly actorType: 'SUBJECT' | 'AGENT' | 'SYSTEM'; readonly actorId: string; readonly action: string; readonly objectType: string; readonly objectId: string; readonly result: 'SUCCEEDED' | 'FAILED' | 'DENIED'; readonly requestId: string; readonly occurredAt: string; readonly fields: readonly AuditField[]; readonly detailsOmitted: boolean }
export interface AuditPage { readonly items: readonly AuditItem[]; readonly nextCursor: string; readonly pageCursor: string; readonly snapshotToken: string; readonly snapshotAt: string; readonly scannedRecords: number; readonly scannedBytes: number; readonly truncated: boolean }
export interface AuditLogApi {
 getAuditAccess(signal?: AbortSignal): Promise<boolean>
 queryAuditLogs(filter: AuditFilter, cursor?: string, signal?: AbortSignal): Promise<AuditPage>
 getAuditDetail(token: string, signal?: AbortSignal): Promise<AuditItem>
 downloadAuditLogs(token: string, signal?: AbortSignal): Promise<Blob>
}
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '审计响应无法校验，请重新查询。', retryable: false } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> { if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).length !== keys.length || keys.some(key => !Object.hasOwn(value, key))) invalid(); return value as Record<string, unknown> }
function string(value: unknown, max = 8192): string { if (typeof value !== 'string' || value.length > max) invalid(); return value }
function bool(value: unknown): boolean { if (typeof value !== 'boolean') invalid(); return value }
function integer(value: unknown, max: number): number { if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0 || value > max) invalid(); return value }
function id(value: unknown, empty = false): string { const text = string(value, 200); if ((!empty || text) && !/^[\p{L}\p{N}_.:@-]+$/u.test(text)) invalid(); return text }
function time(value: unknown): string { const text = string(value, 40); if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(text) || !Number.isFinite(Date.parse(text)) || new Date(text).toISOString().slice(0, 19) !== text.slice(0, 19)) invalid(); return text }
export function parseAuditItem(value: unknown): AuditItem {
 const r = object(value, ['token', 'id', 'actorType', 'actorId', 'action', 'objectType', 'objectId', 'result', 'requestId', 'occurredAt', 'fields', 'detailsOmitted'])
 const token = string(r.token), action = string(r.action, 100), type = string(r.objectType, 100)
 if (!token || !['SUBJECT', 'AGENT', 'SYSTEM'].includes(String(r.actorType)) || !['SUCCEEDED', 'FAILED', 'DENIED'].includes(String(r.result)) || !/^[A-Z0-9_]+$/.test(action) || !auditObjectTypes.includes(type as typeof auditObjectTypes[number]) || !Array.isArray(r.fields) || r.fields.length > 8) invalid()
 const fields = r.fields.map(v => { const field = object(v, ['name', 'value']), name = string(field.name, 80), text = string(field.value, 16); if (['changed', 'confirmed', 'redacted', 'environmentInspectionAuthorized', 'storageConnectivityProbeAllowed'].includes(name) ? !['true', 'false'].includes(text) : !['revision', 'records', 'count'].includes(name) || !/^(0|[1-9]\d*)$/.test(text) || !Number.isSafeInteger(Number(text))) invalid(); return { name, value: text } })
 if (new Set(fields.map(f => f.name)).size !== fields.length) invalid()
 return { token, id: id(r.id), actorType: r.actorType as AuditItem['actorType'], actorId: id(r.actorId), action, objectType: type, objectId: id(r.objectId, true), result: r.result as AuditItem['result'], requestId: id(r.requestId), occurredAt: time(r.occurredAt), fields, detailsOmitted: bool(r.detailsOmitted) }
}
export function parseAuditPage(value: unknown): AuditPage {
 const r = object(value, ['items', 'nextCursor', 'pageCursor', 'snapshotToken', 'snapshotAt', 'scannedRecords', 'scannedBytes', 'truncated'])
 if (!Array.isArray(r.items) || r.items.length > 500) invalid()
 const out: AuditPage = { items: r.items.map(parseAuditItem), nextCursor: string(r.nextCursor), pageCursor: string(r.pageCursor), snapshotToken: string(r.snapshotToken), snapshotAt: time(r.snapshotAt), scannedRecords: integer(r.scannedRecords, 50000), scannedBytes: integer(r.scannedBytes, 64 * 1024 * 1024), truncated: bool(r.truncated) }
 if (!out.pageCursor || !out.snapshotToken || out.items.length > out.scannedRecords || out.truncated && out.nextCursor || new Set(out.items.map(i => i.id)).size !== out.items.length) invalid()
 return out
}
export function createAuditLogApi(transport: { read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>; download(path: string, body: unknown, signal?: AbortSignal): Promise<Blob> }): AuditLogApi {
 return {
  async getAuditAccess(signal) { return bool((await transport.read('/api/v1/logs/audit-access', signal)).allowed) },
  async queryAuditLogs(filter, cursor, signal) { const q = new URLSearchParams({ ...filter, limit: String(filter.limit), ...(cursor ? { cursor } : {}) }); return parseAuditPage((await transport.read(`/api/v1/logs/audit?${q}`, signal)).item) },
  async getAuditDetail(token, signal) { return parseAuditItem((await transport.read(`/api/v1/logs/audit-detail?${new URLSearchParams({ token })}`, signal)).item) },
  downloadAuditLogs: (token, signal) => transport.download('/api/v1/logs/audit:download', { token }, signal),
 }
}
