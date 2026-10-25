import type { SchedulingPolicyReceipt } from './schedulingPolicy'

export interface SchedulingTimingPolicy {
  readonly heartbeatTimeoutSeconds: number
  readonly heartbeatTimeoutSource: 'CONTROL_PLANE' | 'PLATFORM_SETTING'
  readonly startupTimeoutSeconds: number | null
  readonly pollingIntervalSeconds: number | null
  readonly candidateStrategy: 'EXPLICIT_SELECTION'
  readonly revision: number
  readonly modifiedBy?: string
  readonly modifiedAt?: string
  readonly applicationMode: 'HEARTBEAT_NOW_STARTUP_NEXT_CLAIM_POLL_NEXT_CHECK'
  readonly restartRequired: false
}
export interface SchedulingTimingWrite {
  readonly heartbeatTimeoutSeconds: number
  readonly startupTimeoutSeconds: number | null
  readonly pollingIntervalSeconds: number | null
  readonly reason: string
  readonly confirmed: boolean
}
export interface SchedulingTimingApi {
  getSchedulingTiming(signal?: AbortSignal): Promise<SchedulingTimingPolicy>
  changeSchedulingTiming(revision: number, payload: SchedulingTimingWrite, key: string, signal?: AbortSignal): Promise<SchedulingPolicyReceipt>
}
interface Transport {
  read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>
  write(path: string, body: SchedulingTimingWrite, revision: number, key: string, signal?: AbortSignal): Promise<Record<string, unknown>>
}
const path = '/api/v1/settings/scheduling/timing'
const mode = 'HEARTBEAT_NOW_STARTUP_NEXT_CLAIM_POLL_NEXT_CHECK'
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '调度时间策略响应无法校验，请重新读取。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).some(key => !keys.includes(key))) invalid()
  return value as Record<string, unknown>
}
function integer(value: unknown, minimum: number, maximum: number): number { return typeof value === 'number' && Number.isSafeInteger(value) && value >= minimum && value <= maximum ? value : invalid() }
function nullableSeconds(value: unknown, minimum: number, maximum: number): number | null { return value === null ? null : integer(value, minimum, maximum) }
function safeText(value: unknown, maximum: number): string { return typeof value === 'string' && value.length > 0 && value.length <= maximum && !/[\p{Cc}]/u.test(value) ? value : invalid() }

/** 时间来源与空覆盖均严格校验，未配置不能补成默认启动期限或 Agent 周期。 */
export function parseSchedulingTiming(value: unknown): SchedulingTimingPolicy {
  const row = object(value, ['heartbeatTimeoutSeconds', 'heartbeatTimeoutSource', 'startupTimeoutSeconds', 'pollingIntervalSeconds', 'candidateStrategy', 'revision', 'modifiedBy', 'modifiedAt', 'applicationMode', 'restartRequired'])
  if (!['CONTROL_PLANE', 'PLATFORM_SETTING'].includes(String(row.heartbeatTimeoutSource)) || row.candidateStrategy !== 'EXPLICIT_SELECTION' || row.applicationMode !== mode || row.restartRequired !== false) invalid()
  const heartbeat = integer(row.heartbeatTimeoutSeconds, row.heartbeatTimeoutSource === 'PLATFORM_SETTING' ? 60 : 1, row.heartbeatTimeoutSource === 'PLATFORM_SETTING' ? 3600 : 86400)
  const startup = nullableSeconds(row.startupTimeoutSeconds, 15, 3600), polling = nullableSeconds(row.pollingIntervalSeconds, 1, 60)
  const configured = row.heartbeatTimeoutSource === 'PLATFORM_SETTING' || startup !== null || polling !== null
  if (configured !== (row.modifiedBy !== undefined) || configured !== (row.modifiedAt !== undefined)) invalid()
  let metadata: Pick<SchedulingTimingPolicy, 'modifiedBy' | 'modifiedAt'> = {}
  if (configured) {
    const actor = safeText(row.modifiedBy, 200), stamp = safeText(row.modifiedAt, 64)
    if (/[\s/\\?#]/u.test(actor) || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(stamp) || !Number.isFinite(Date.parse(stamp))) invalid()
    metadata = { modifiedBy: actor, modifiedAt: stamp }
  }
  return { heartbeatTimeoutSeconds: heartbeat, heartbeatTimeoutSource: row.heartbeatTimeoutSource as SchedulingTimingPolicy['heartbeatTimeoutSource'], startupTimeoutSeconds: startup, pollingIntervalSeconds: polling, candidateStrategy: 'EXPLICIT_SELECTION', revision: integer(row.revision, 1, Number.MAX_SAFE_INTEGER - 1), applicationMode: mode, restartRequired: false, ...metadata }
}

/** 复用既有浏览器安全传输；确认回执后由会话所有者重新读取当前事实。 */
export function createSchedulingTimingApi(transport: Transport): SchedulingTimingApi {
  return {
    async getSchedulingTiming(signal) { const row = object(await transport.read(path, signal), ['requestId', 'item']); return parseSchedulingTiming(row.item) },
    async changeSchedulingTiming(currentRevision, payload, key, signal) {
      integer(currentRevision, 1, Number.MAX_SAFE_INTEGER - 2); safeText(key, 128)
      const body: SchedulingTimingWrite = { heartbeatTimeoutSeconds: integer(payload.heartbeatTimeoutSeconds, 1, 86400), startupTimeoutSeconds: nullableSeconds(payload.startupTimeoutSeconds, 15, 3600), pollingIntervalSeconds: nullableSeconds(payload.pollingIntervalSeconds, 1, 60), reason: safeText(payload.reason.trim(), 400), confirmed: payload.confirmed }
      if (typeof body.confirmed !== 'boolean' || Array.from(body.reason).length > 200) invalid()
      const row = object(await transport.write(path, body, currentRevision, key, signal), ['requestId', 'item']), item = object(row.item, ['revision', 'changed', 'replayed'])
      if (typeof item.changed !== 'boolean' || typeof item.replayed !== 'boolean' || integer(item.revision, 1, Number.MAX_SAFE_INTEGER - 1) !== currentRevision + (item.changed ? 1 : 0)) invalid()
      return { revision: item.revision as number, changed: item.changed, replayed: item.replayed }
    },
  }
}
