export interface SchedulingPolicy {
  readonly maxConcurrentExecutions: number | null
  readonly revision: number
  readonly source: 'PLATFORM_SETTING' | 'UNCONFIGURED'
  readonly modifiedBy?: string
  readonly modifiedAt?: string
  readonly usage: { readonly visibility: 'AVAILABLE' | 'RESTRICTED'; readonly heldExecutions: number | null; readonly runningExecutions: number | null; readonly observedAt: string }
  readonly applicationMode: 'NEXT_CLAIM'
  readonly restartRequired: false
}
export interface SchedulingPolicyWrite { readonly maxConcurrentExecutions: number; readonly reason: string; readonly confirmed: boolean }
export interface SchedulingPolicyReceipt { readonly revision: number; readonly changed: boolean; readonly replayed: boolean }
export interface SchedulingPolicyApi {
  getSchedulingPolicy(signal?: AbortSignal): Promise<SchedulingPolicy>
  changeSchedulingPolicy(revision: number, payload: SchedulingPolicyWrite, key: string, signal?: AbortSignal): Promise<SchedulingPolicyReceipt>
}
interface Transport {
  read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>
  write(path: string, body: SchedulingPolicyWrite, revision: number, key: string, signal?: AbortSignal): Promise<Record<string, unknown>>
}
const path = '/api/v1/settings/scheduling'
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '平台调度策略响应无法校验，请核对版本后重试。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).some(key => !keys.includes(key))) invalid()
  return value as Record<string, unknown>
}
function revision(value: unknown): number { return typeof value === 'number' && Number.isSafeInteger(value) && value > 0 && value < Number.MAX_SAFE_INTEGER ? value : invalid() }
function safeText(value: unknown, max: number): string { return typeof value === 'string' && value.length > 0 && value.length <= max && !/[\p{Cc}]/u.test(value) ? value : invalid() }

function instant(value: unknown): string { const text=safeText(value,64); return /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(text) && Number.isFinite(Date.parse(text)) ? text : invalid() }

/** 只接收平台调度策略的完整安全投影；损坏响应不能恢复为默认可编辑状态。 */
export function parseSchedulingPolicy(value: unknown): SchedulingPolicy {
  const row = object(value, ['maxConcurrentExecutions', 'revision', 'source', 'modifiedBy', 'modifiedAt', 'applicationMode', 'restartRequired', 'usage'])
  if (row.applicationMode !== 'NEXT_CLAIM' || row.restartRequired !== false || !['PLATFORM_SETTING','UNCONFIGURED'].includes(String(row.source))) invalid()
  const modified=row.source==='PLATFORM_SETTING'
  if (modified !== (row.maxConcurrentExecutions !== null) || modified !== (row.modifiedBy !== undefined) || modified !== (row.modifiedAt !== undefined)) invalid()
  if (modified && (typeof row.maxConcurrentExecutions!=='number' || !Number.isInteger(row.maxConcurrentExecutions) || row.maxConcurrentExecutions<1 || row.maxConcurrentExecutions>1024)) invalid()
  let metadata: Pick<SchedulingPolicy, 'modifiedBy' | 'modifiedAt'> = {}
  if (modified) {
    const actor = safeText(row.modifiedBy, 200), at = instant(row.modifiedAt)
    if (/[\s/\\?#]/u.test(actor)) invalid()
    metadata={ modifiedBy:actor,modifiedAt:at }
  }
  const usage=object(row.usage,['visibility','heldExecutions','runningExecutions','observedAt'])
  if (!['AVAILABLE','RESTRICTED'].includes(String(usage.visibility))) invalid()
  if (usage.visibility==='RESTRICTED') { if (usage.heldExecutions!==null || usage.runningExecutions!==null) invalid() }
  else { if (typeof usage.heldExecutions!=='number' || !Number.isSafeInteger(usage.heldExecutions) || usage.heldExecutions<0 || typeof usage.runningExecutions!=='number' || !Number.isSafeInteger(usage.runningExecutions) || usage.runningExecutions<0 || usage.runningExecutions>usage.heldExecutions) invalid() }
  return { maxConcurrentExecutions:row.maxConcurrentExecutions as number|null,revision:revision(row.revision),source:row.source as SchedulingPolicy['source'],applicationMode:'NEXT_CLAIM',restartRequired:false,usage:{visibility:usage.visibility as 'AVAILABLE'|'RESTRICTED',heldExecutions:usage.heldExecutions as number|null,runningExecutions:usage.runningExecutions as number|null,observedAt:instant(usage.observedAt)},...metadata }

}

/** 复用既有浏览器认证、CSRF、版本和幂等传输，不接受客户端身份或配置来源。 */
export function createSchedulingPolicyApi(transport: Transport): SchedulingPolicyApi {
  return {
    async getSchedulingPolicy(signal) { const row = object(await transport.read(path, signal), ['requestId', 'item']); return parseSchedulingPolicy(row.item) },
    async changeSchedulingPolicy(currentRevision, payload, key, signal) {
      revision(currentRevision); safeText(key, 128)
      const body = { maxConcurrentExecutions: payload.maxConcurrentExecutions, reason: payload.reason.trim(), confirmed: payload.confirmed }
      if (!Number.isInteger(body.maxConcurrentExecutions) || body.maxConcurrentExecutions < 1 || body.maxConcurrentExecutions > 1024 || typeof body.confirmed !== 'boolean' || Array.from(safeText(body.reason, 400)).length > 200) invalid()
      const row = object(await transport.write(path, body, currentRevision, key, signal), ['requestId', 'item'])
      const item = object(row.item, ['revision', 'changed', 'replayed'])
      if (typeof item.changed !== 'boolean' || typeof item.replayed !== 'boolean' || revision(item.revision) !== currentRevision + (item.changed ? 1 : 0)) invalid()
      return { revision: item.revision as number, changed: item.changed, replayed: item.replayed }
    },
  }
}
