export interface LogFilter { readonly from: string; readonly to: string; readonly taskId: string; readonly source: string; readonly keyword: string; readonly limit: number }
export interface LogItem { readonly token: string; readonly taskId: string; readonly source: string; readonly streamId: string; readonly epoch: number; readonly sequence: number; readonly kind: 'LOG' | 'GAP' | 'TRUNCATED'; readonly message: string; readonly integrityCode: string; readonly receivedAt: string }
export interface LogPage { readonly items: readonly LogItem[]; readonly nextCursor: string; readonly pageCursor: string; readonly snapshotToken: string; readonly snapshotAt: string; readonly sources: readonly string[]; readonly availability: 'RETAINED' | 'EXPIRED' | 'NOT_COLLECTED'; readonly expired: boolean; readonly incomplete: boolean; readonly truncated: boolean; readonly scannedRecords: number; readonly scannedBytes: number }
export interface LogContext { readonly items: readonly LogItem[]; readonly selectedSequence: number; readonly snapshotAt: string; readonly incomplete: boolean }
export interface LogQueryApi {
  queryTaskLogs(filter: LogFilter, cursor?: string, signal?: AbortSignal): Promise<LogPage>
  getLogContext(token: string, signal?: AbortSignal): Promise<LogContext>
  copyLogContext(token: string, signal?: AbortSignal): Promise<string>
  downloadTaskLogs(token: string, fullTask: boolean, signal?: AbortSignal): Promise<Blob>
}
const sources = ['OBDUMPER_STDOUT', 'OBDUMPER_STDERR', 'OBDUMPER_FILE', 'OBLOADER_STDOUT', 'OBLOADER_STDERR']
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '日志响应无法校验，请重新查询。', retryable: false } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).length !== keys.length || keys.some(key => !Object.hasOwn(value, key))) invalid()
  return value as Record<string, unknown>
}
function string(value: unknown, max = 8192): string { if (typeof value !== 'string' || value.length > max) invalid(); return value }
function timestamp(value: unknown): string { const s = string(value, 40); if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(s) || !Number.isFinite(Date.parse(s)) || new Date(s).toISOString().slice(0, 19) !== s.slice(0, 19)) invalid(); return s }
function integer(value: unknown, max: number, min = 0): number { if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < min || value > max) invalid(); return value }
function boolean(value: unknown): boolean { if (typeof value !== 'boolean') invalid(); return value }
function item(value: unknown, context: boolean): LogItem {
  const r = object(value, ['token', 'taskId', 'source', 'streamId', 'epoch', 'sequence', 'kind', 'message', 'integrityCode', 'receivedAt'])
  const token = string(r.token), taskId = string(r.taskId, 200), source = string(r.source, 32), streamId = string(r.streamId, 64)
  if ((!context && !token) || !/^[\p{L}\p{N}_.-]+$/u.test(taskId) || !sources.includes(source) || !/^[a-f0-9]{64}$/.test(streamId) || !['LOG', 'GAP', 'TRUNCATED'].includes(String(r.kind))) invalid()
  return { token, taskId, source, streamId, epoch: integer(r.epoch, Number.MAX_SAFE_INTEGER, 1), sequence: integer(r.sequence, Number.MAX_SAFE_INTEGER, 1), kind: r.kind as LogItem['kind'], message: string(r.message, 262144), integrityCode: string(r.integrityCode, 128), receivedAt: timestamp(r.receivedAt) }
}
export function parseLogPage(value: unknown): LogPage {
  const r = object(value, ['items', 'nextCursor', 'pageCursor', 'snapshotToken', 'snapshotAt', 'sources', 'availability', 'expired', 'incomplete', 'truncated', 'scannedRecords', 'scannedBytes'])
  if (!Array.isArray(r.items) || r.items.length > 500 || !Array.isArray(r.sources) || r.sources.length > sources.length || new Set(r.sources).size !== r.sources.length || r.sources.some(s => typeof s !== 'string' || !sources.includes(s)) || !['RETAINED', 'EXPIRED', 'NOT_COLLECTED'].includes(String(r.availability))) invalid()
  const result: LogPage = { items: r.items.map(v => item(v, false)), nextCursor: string(r.nextCursor), pageCursor: string(r.pageCursor), snapshotToken: string(r.snapshotToken), snapshotAt: timestamp(r.snapshotAt), sources: r.sources as string[], availability: r.availability as LogPage['availability'], expired: boolean(r.expired), incomplete: boolean(r.incomplete), truncated: boolean(r.truncated), scannedRecords: integer(r.scannedRecords, 50000), scannedBytes: integer(r.scannedBytes, 64 * 1024 * 1024) }
  if (!result.pageCursor || !result.snapshotToken || result.truncated && result.nextCursor || result.items.length > result.scannedRecords || result.availability !== 'RETAINED' && result.items.length) invalid()
  if (new Set(result.items.map(i => `${i.streamId}:${i.sequence}`)).size !== result.items.length) invalid()
  return result
}
export function parseLogContext(value: unknown): LogContext {
  const r = object(value, ['items', 'selectedSequence', 'snapshotAt', 'incomplete'])
  if (!Array.isArray(r.items) || !r.items.length || r.items.length > 101) invalid()
  const items = r.items.map(v => item(v, true)), selectedSequence = integer(r.selectedSequence, Number.MAX_SAFE_INTEGER, 1), first = items[0]!
  if (!items.some(i => i.sequence === selectedSequence) || items.some((i, n) => i.streamId !== first.streamId || i.epoch !== first.epoch || i.taskId !== first.taskId || i.source !== first.source || Math.abs(i.sequence - selectedSequence) > 50 || n > 0 && i.sequence <= items[n - 1]!.sequence)) invalid()
  return { items, selectedSequence, snapshotAt: timestamp(r.snapshotAt), incomplete: boolean(r.incomplete) }
}
export function createLogQueryApi(transport: { read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>; write(path: string, body: unknown, signal?: AbortSignal): Promise<Record<string, unknown>>; download(path: string, body: unknown, signal?: AbortSignal): Promise<Blob> }): LogQueryApi {
  return {
    async queryTaskLogs(filter, cursor, signal) { const q = new URLSearchParams({ ...filter, limit: String(filter.limit), ...(cursor ? { cursor } : {}) }); return parseLogPage((await transport.read(`/api/v1/logs/tasks?${q}`, signal)).item) },
    async getLogContext(token, signal) { return parseLogContext((await transport.read(`/api/v1/logs/task-context?${new URLSearchParams({ token })}`, signal)).item) },
    async copyLogContext(token, signal) { const r = await transport.write('/api/v1/logs/task-context:copy', { token }, signal); return string(r.text, 28 * 1024 * 1024) },
    downloadTaskLogs: (token, fullTask, signal) => transport.download('/api/v1/logs/tasks:download', { token, fullTask }, signal),
  }
}
