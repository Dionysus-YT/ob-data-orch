import type { SchedulingPolicyReceipt } from './schedulingPolicy'

export type CandidateOrder = 'EXISTING' | 'ONLINE_FIRST'
export interface SchedulingCandidatesPolicy {
  readonly order: CandidateOrder
  readonly revision: number
  readonly modifiedBy?: string
  readonly modifiedAt?: string
  readonly selectionMode: 'EXPLICIT_SELECTION'
  readonly applicationMode: 'NEXT_CANDIDATE_READ'
  readonly restartRequired: false
}
export interface SchedulingCandidatesWrite { readonly order: CandidateOrder; readonly reason: string; readonly confirmed: boolean }
export interface SchedulingCandidatesApi {
  getSchedulingCandidates(signal?: AbortSignal): Promise<SchedulingCandidatesPolicy>
  changeSchedulingCandidates(revision: number, payload: SchedulingCandidatesWrite, key: string, signal?: AbortSignal): Promise<SchedulingPolicyReceipt>
}
interface Transport {
  read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>>
  write(path: string, body: SchedulingCandidatesWrite, revision: number, key: string, signal?: AbortSignal): Promise<Record<string, unknown>>
}
const path = '/api/v1/settings/scheduling/candidates'
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '候选顺序响应无法校验，请重新读取。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).some(key => !keys.includes(key))) invalid()
  return value as Record<string, unknown>
}
function revision(value: unknown) { return typeof value === 'number' && Number.isSafeInteger(value) && value >= 1 && value < Number.MAX_SAFE_INTEGER ? value : invalid() }
function order(value: unknown): CandidateOrder { return value === 'EXISTING' || value === 'ONLINE_FIRST' ? value : invalid() }

/** 候选配置只含内置排序枚举，拒绝自动选中模式、不完整元信息与未知字段。 */
export function parseSchedulingCandidates(value: unknown): SchedulingCandidatesPolicy {
  const row = object(value, ['order', 'revision', 'modifiedBy', 'modifiedAt', 'selectionMode', 'applicationMode', 'restartRequired'])
  const currentRevision = revision(row.revision), currentOrder = order(row.order)
  if (row.selectionMode !== 'EXPLICIT_SELECTION' || row.applicationMode !== 'NEXT_CANDIDATE_READ' || row.restartRequired !== false) invalid()
  if (currentRevision === 1) {
    if (currentOrder !== 'EXISTING' || row.modifiedBy !== undefined || row.modifiedAt !== undefined) invalid()
    return { order: currentOrder, revision: currentRevision, selectionMode: 'EXPLICIT_SELECTION', applicationMode: 'NEXT_CANDIDATE_READ', restartRequired: false }
  }
  if (typeof row.modifiedBy !== 'string' || !row.modifiedBy || row.modifiedBy.length > 200 || /[\s/\\?#\p{Cc}]/u.test(row.modifiedBy) || typeof row.modifiedAt !== 'string' || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(row.modifiedAt) || !Number.isFinite(Date.parse(row.modifiedAt))) invalid()
  return { order: currentOrder, revision: currentRevision, modifiedBy: row.modifiedBy, modifiedAt: row.modifiedAt, selectionMode: 'EXPLICIT_SELECTION', applicationMode: 'NEXT_CANDIDATE_READ', restartRequired: false }
}

/** 复用浏览器安全传输，白名单写入，旧回执由领域会话复读当前策略。 */
export function createSchedulingCandidatesApi(transport: Transport): SchedulingCandidatesApi {
  return {
    async getSchedulingCandidates(signal) { return parseSchedulingCandidates(object(await transport.read(path, signal), ['requestId', 'item']).item) },
    async changeSchedulingCandidates(currentRevision, payload, key, signal) {
      revision(currentRevision)
      if (currentRevision >= Number.MAX_SAFE_INTEGER - 1 || !key || key.length > 128 || /[\p{Cc}]/u.test(key) || typeof payload.reason !== 'string' || typeof payload.confirmed !== 'boolean') invalid()
      const reason = payload.reason.trim()
      if (!reason || Array.from(reason).length > 200 || /[\p{Cc}]/u.test(reason)) invalid()
      const body: SchedulingCandidatesWrite = { order: order(payload.order), reason, confirmed: payload.confirmed }
      const item = object(object(await transport.write(path, body, currentRevision, key, signal), ['requestId', 'item']).item, ['revision', 'changed', 'replayed'])
      if (typeof item.changed !== 'boolean' || typeof item.replayed !== 'boolean' || revision(item.revision) !== currentRevision + (item.changed ? 1 : 0)) invalid()
      return { revision: item.revision as number, changed: item.changed, replayed: item.replayed }
    },
  }
}
