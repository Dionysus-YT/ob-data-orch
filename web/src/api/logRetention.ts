export interface LogRetentionPolicy {
  readonly retentionDays: number
  readonly revision: number
  readonly source: 'PLATFORM_SETTING' | 'DEPLOYMENT_CONFIG' | 'BUILT_IN_DEFAULT'
  readonly modifiedBy?: string
  readonly modifiedAt?: string
  readonly applicationMode: 'NEXT_BATCH'
  readonly restartRequired: false
}
export interface LogRetentionWrite { readonly retentionDays: number; readonly reason: string; readonly confirmed: boolean }
export interface LogRetentionReceipt { readonly revision: number; readonly changed: boolean; readonly replayed: boolean }
export interface LogRetentionApi {
  getLogRetention(signal?: AbortSignal): Promise<LogRetentionPolicy>
  changeLogRetention(revision: number, payload: LogRetentionWrite, key: string, signal?: AbortSignal): Promise<LogRetentionReceipt>
}
interface Transport {
  read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>
  write(path: string, body: LogRetentionWrite, revision: number, key: string, signal?: AbortSignal): Promise<Record<string, unknown>>
}
const path = '/api/v1/settings/log-retention'
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '普通日志策略响应无法校验，请核对版本后重试。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).some(key => !keys.includes(key))) invalid()
  return value as Record<string, unknown>
}
function revision(value: unknown): number { return typeof value === 'number' && Number.isSafeInteger(value) && value > 0 && value < Number.MAX_SAFE_INTEGER ? value : invalid() }
function safeText(value: unknown, max: number): string { return typeof value === 'string' && value.length > 0 && value.length <= max && !/[\p{Cc}]/u.test(value) ? value : invalid() }

/** 只接收普通日志策略的完整安全投影；损坏响应不能恢复为默认可编辑状态。 */
export function parseLogRetention(value: unknown): LogRetentionPolicy {
  const row = object(value, ['retentionDays', 'revision', 'source', 'modifiedBy', 'modifiedAt', 'applicationMode', 'restartRequired'])
  if (typeof row.retentionDays !== 'number' || !Number.isInteger(row.retentionDays) || row.retentionDays < 7 || row.retentionDays > 365 || !['PLATFORM_SETTING', 'DEPLOYMENT_CONFIG', 'BUILT_IN_DEFAULT'].includes(String(row.source)) || row.applicationMode !== 'NEXT_BATCH' || row.restartRequired !== false) invalid()
  const modified = row.source === 'PLATFORM_SETTING'
  if (modified !== (row.modifiedBy !== undefined) || modified !== (row.modifiedAt !== undefined) || row.source === 'BUILT_IN_DEFAULT' && row.retentionDays !== 30) invalid()
  let metadata: Pick<LogRetentionPolicy, 'modifiedBy' | 'modifiedAt'> = {}
  if (modified) {
    const actor = safeText(row.modifiedBy, 200), at = safeText(row.modifiedAt, 64)
    if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(at) || !Number.isFinite(Date.parse(at)) || /[\s/\\?#]/u.test(actor)) invalid()
    metadata = { modifiedBy: actor, modifiedAt: at }
  }
  return { retentionDays: row.retentionDays, revision: revision(row.revision), source: row.source as LogRetentionPolicy['source'], applicationMode: 'NEXT_BATCH', restartRequired: false, ...metadata }
}

/** 复用既有浏览器认证、CSRF、版本和幂等传输，不接受客户端身份或配置来源。 */
export function createLogRetentionApi(transport: Transport): LogRetentionApi {
  return {
    async getLogRetention(signal) { const row = object(await transport.read(path, signal), ['requestId', 'item']); return parseLogRetention(row.item) },
    async changeLogRetention(currentRevision, payload, key, signal) {
      revision(currentRevision); safeText(key, 128)
      const body = { retentionDays: payload.retentionDays, reason: payload.reason.trim(), confirmed: payload.confirmed }
      if (!Number.isInteger(body.retentionDays) || body.retentionDays < 7 || body.retentionDays > 365 || typeof body.confirmed !== 'boolean' || Array.from(safeText(body.reason, 400)).length > 200) invalid()
      const row = object(await transport.write(path, body, currentRevision, key, signal), ['requestId', 'item'])
      const item = object(row.item, ['revision', 'changed', 'replayed'])
      if (typeof item.changed !== 'boolean' || typeof item.replayed !== 'boolean' || revision(item.revision) !== currentRevision + (item.changed ? 1 : 0)) invalid()
      return { revision: item.revision as number, changed: item.changed, replayed: item.replayed }
    },
  }
}
