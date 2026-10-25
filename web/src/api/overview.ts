import { parseTaskProgressEvidence, type TaskProgress } from './taskProgress'

export type OverviewRange = '24h' | '7d'
export type OverviewSection<T> = { readonly status: 'READY'; readonly data: T } | { readonly status: 'UNAVAILABLE' | 'RESTRICTED' }
export interface OverviewMetrics { readonly running: number; readonly waiting: number; readonly todaySucceeded: number; readonly todayFailed: number }
export interface OverviewBucket { readonly from: string; readonly to: string; readonly total: number; readonly succeeded: number; readonly failed: number }
export interface OverviewTask {
  readonly id: string; readonly displayName: string; readonly type: 'OBDUMPER_EXPORT' | 'OBLOADER_NORMAL_IMPORT' | 'OBLOADER_DIRECT_LOAD'
  readonly state: 'WAITING_SCHEDULE' | 'STARTING' | 'RUNNING' | 'CANCELLING' | 'SUCCEEDED' | 'FAILED' | 'CANCELLED'
  readonly reconciliationRequired: boolean; readonly stageEvidence: 'UNAVAILABLE' | 'RELIABLE'; readonly progressEvidence: 'UNAVAILABLE' | 'PERCENT'
  readonly progress?: TaskProgress; readonly startedAt?: string; readonly updatedAt: string
}
export interface OverviewNode {
  readonly id: string; readonly displayName: string; readonly managementState: 'ENABLED' | 'DISABLED' | 'MAINTENANCE'
  readonly heartbeatStatus: 'ONLINE' | 'OFFLINE' | 'NEVER_CONNECTED'; readonly environmentStatus: 'NORMAL' | 'ABNORMAL' | 'UNCONFIRMED' | 'EXPIRED' | 'NOT_CHECKED'
  readonly capacityStatus: 'AVAILABLE' | 'BUSY' | 'UNKNOWN'; readonly acceptsNewTasks: boolean; readonly unavailableReasons: readonly string[]
  readonly lastHeartbeatAt: string | null; readonly capacityTotal?: number; readonly capacityUsed?: number
  readonly observedAt?: string; readonly cpuUsagePercent?: number; readonly memoryUsagePercent?: number; readonly updatedAt: string
}
export interface OverviewNodes { readonly total: number; readonly unavailable: number; readonly items: readonly OverviewNode[] }
export interface OverviewAttentionItem {
  readonly kind: 'TASK_FAILED' | 'NODE_UNAVAILABLE'; readonly objectId: string; readonly displayName: string
  readonly occurredAt: string; readonly source: 'TASK_STATE' | 'NODE_STATE'; readonly reconciliationRequired: boolean
}
export interface OverviewAttention {
  readonly from: string; readonly to: string; readonly coverage: 'TASK_FACTS_ONLY' | 'TASK_AND_NODE_FACTS'; readonly items: readonly OverviewAttentionItem[]
}
export interface OverviewSnapshot {
  readonly snapshotAt: string; readonly timeZone: 'UTC'; readonly timeZoneSource: 'BUILTIN_UTC'; readonly range: OverviewRange
  readonly todayFrom: string; readonly todayTo: string; readonly refreshIntervalSeconds: number; readonly staleAfterSeconds: number; readonly summary: string
  readonly metrics: OverviewSection<OverviewMetrics>; readonly trend: OverviewSection<readonly OverviewBucket[]>
  readonly nodes: OverviewSection<OverviewNodes>; readonly myTasks: OverviewSection<readonly OverviewTask[]>; readonly attention: OverviewSection<OverviewAttention>
}
export interface OverviewApi { getOverview(range: OverviewRange): Promise<OverviewSnapshot> }
interface Transport { read(path: string): Promise<Record<string, unknown>> }

function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '首页快照无法校验，请刷新；若仍失败，请核对网页与控制面版本。', retryable: false, conflict: false, fieldErrors: [] } }
function record(value: unknown): Record<string, unknown> { return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : invalid() }
function known(item: Record<string, unknown>, keys: readonly string[]) { if (Object.keys(item).some(key => !keys.includes(key))) invalid() }
function text(value: unknown, max = 1024): string { return typeof value === 'string' && value.length > 0 && value.length <= max && !Array.from(value).some(character => character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) ? value : invalid() }
function identifier(value: unknown): string { const result = text(value, 256); return /[\s/\\?#]/.test(result) ? invalid() : result }
function instant(value: unknown): string { const result = text(value, 64); return /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/.test(result) && Number.isFinite(Date.parse(result)) ? result : invalid() }
function count(value: unknown): number { return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : invalid() }
function boolean(value: unknown): boolean { return typeof value === 'boolean' ? value : invalid() }
function choice<T extends string>(value: unknown, values: readonly T[]): T { return values.includes(value as T) ? value as T : invalid() }
function list<T>(value: unknown, limit: number, parse: (item: unknown) => T): readonly T[] { return Array.isArray(value) && value.length <= limit ? value.map(parse) : invalid() }
function unique<T>(items: readonly T[], key: (item: T) => string): readonly T[] { if (new Set(items.map(key)).size !== items.length) invalid(); return items }
function section<T>(value: unknown, parse: (value: unknown) => T): OverviewSection<T> {
  const item = record(value); known(item, ['status', 'data'])
  if (item.status === 'READY') return { status: 'READY', data: parse(item.data) }
  const status = choice(item.status, ['UNAVAILABLE', 'RESTRICTED'] as const)
  if (item.data !== undefined) invalid()
  return { status }
}
function metrics(value: unknown): OverviewMetrics {
  const item = record(value); known(item, ['running', 'waiting', 'todaySucceeded', 'todayFailed'])
  return { running: count(item.running), waiting: count(item.waiting), todaySucceeded: count(item.todaySucceeded), todayFailed: count(item.todayFailed) }
}
function bucket(value: unknown): OverviewBucket {
  const item = record(value); known(item, ['from', 'to', 'total', 'succeeded', 'failed'])
  const result = { from: instant(item.from), to: instant(item.to), total: count(item.total), succeeded: count(item.succeeded), failed: count(item.failed) }
  if (Date.parse(result.from) >= Date.parse(result.to) || result.succeeded + result.failed > result.total) invalid()
  return result
}
function task(value: unknown): OverviewTask {
  const item = record(value); known(item, ['id', 'displayName', 'type', 'state', 'reconciliationRequired', 'stageEvidence', 'progressEvidence', 'progress', 'startedAt', 'updatedAt'])
  const type = choice(item.type, ['OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT', 'OBLOADER_DIRECT_LOAD'] as const)
  return { id: identifier(item.id), displayName: text(item.displayName), type,
    state: choice(item.state, ['WAITING_SCHEDULE', 'STARTING', 'RUNNING', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED'] as const),
    reconciliationRequired: boolean(item.reconciliationRequired), updatedAt: instant(item.updatedAt),
    ...(item.startedAt === undefined ? {} : { startedAt: instant(item.startedAt) }), ...parseTaskProgressEvidence(item, type, invalid) }
}
function node(value: unknown): OverviewNode {
  const item = record(value); known(item, ['id', 'displayName', 'managementState', 'heartbeatStatus', 'environmentStatus', 'capacityStatus', 'acceptsNewTasks', 'unavailableReasons', 'lastHeartbeatAt', 'capacityTotal', 'capacityUsed', 'observedAt', 'cpuUsagePercent', 'memoryUsagePercent', 'updatedAt'])
  const reasons = list(item.unavailableReasons, 32, value => { const code = text(value, 80); return /^[A-Z0-9_]+$/.test(code) ? code : invalid() })
  const result: OverviewNode = { id: identifier(item.id), displayName: text(item.displayName),
    managementState: choice(item.managementState, ['ENABLED', 'DISABLED', 'MAINTENANCE'] as const),
    heartbeatStatus: choice(item.heartbeatStatus, ['ONLINE', 'OFFLINE', 'NEVER_CONNECTED'] as const),
    environmentStatus: choice(item.environmentStatus, ['NORMAL', 'ABNORMAL', 'UNCONFIRMED', 'EXPIRED', 'NOT_CHECKED'] as const),
    capacityStatus: choice(item.capacityStatus, ['AVAILABLE', 'BUSY', 'UNKNOWN'] as const), acceptsNewTasks: boolean(item.acceptsNewTasks), unavailableReasons: reasons,
    lastHeartbeatAt: item.lastHeartbeatAt === null ? null : instant(item.lastHeartbeatAt), updatedAt: instant(item.updatedAt),
    ...(item.observedAt === undefined ? {} : { observedAt: instant(item.observedAt) }),
    ...(item.capacityTotal === undefined ? {} : { capacityTotal: count(item.capacityTotal) }),
    ...(item.capacityUsed === undefined ? {} : { capacityUsed: count(item.capacityUsed) }),
    ...(item.cpuUsagePercent === undefined ? {} : { cpuUsagePercent: count(item.cpuUsagePercent) }),
    ...(item.memoryUsagePercent === undefined ? {} : { memoryUsagePercent: count(item.memoryUsagePercent) }) }
  if ((result.capacityTotal === undefined) !== (result.capacityUsed === undefined) ||
    result.cpuUsagePercent !== undefined && result.cpuUsagePercent > 100 || result.memoryUsagePercent !== undefined && result.memoryUsagePercent > 100 ||
    !result.observedAt && (result.cpuUsagePercent !== undefined || result.memoryUsagePercent !== undefined) ||
    result.acceptsNewTasks && (result.managementState !== 'ENABLED' || result.heartbeatStatus !== 'ONLINE' || result.environmentStatus !== 'NORMAL' || result.capacityStatus !== 'AVAILABLE' || reasons.length > 0)) invalid()
  return result
}
function nodes(value: unknown): OverviewNodes {
  const item = record(value); known(item, ['total', 'unavailable', 'items'])
  const result = { total: count(item.total), unavailable: count(item.unavailable), items: unique(list(item.items, 5, node), node => node.id) }
  if (result.total < result.items.length || result.unavailable > result.total || result.total === 0) invalid()
  return result
}
function attention(value: unknown): OverviewAttention {
  const item = record(value); known(item, ['from', 'to', 'coverage', 'items'])
  const result = { from: instant(item.from), to: instant(item.to), coverage: choice(item.coverage, ['TASK_FACTS_ONLY', 'TASK_AND_NODE_FACTS'] as const),
    items: unique(list(item.items, 5, raw => {
      const event = record(raw); known(event, ['kind', 'objectId', 'displayName', 'occurredAt', 'source', 'reconciliationRequired'])
      const kind = choice(event.kind, ['TASK_FAILED', 'NODE_UNAVAILABLE'] as const)
      const source = choice(event.source, ['TASK_STATE', 'NODE_STATE'] as const)
      if ((kind === 'TASK_FAILED') !== (source === 'TASK_STATE')) invalid()
      return { kind, source, objectId: identifier(event.objectId), displayName: text(event.displayName), occurredAt: instant(event.occurredAt), reconciliationRequired: boolean(event.reconciliationRequired) }
    }), event => `${event.kind}:${event.objectId}`) }
  if (Date.parse(result.from) >= Date.parse(result.to)) invalid()
  return result
}

/** 严格解码快照；未知字段、矛盾时间、隐含旧数据或未知事实都不能降级为正常。 */
export function parseOverviewSnapshot(value: unknown, requestedRange: OverviewRange): OverviewSnapshot {
  const item = record(value)
  known(item, ['snapshotAt', 'timeZone', 'timeZoneSource', 'range', 'todayFrom', 'todayTo', 'refreshIntervalSeconds', 'staleAfterSeconds', 'summary', 'metrics', 'trend', 'nodes', 'myTasks', 'attention'])
  const result: OverviewSnapshot = { snapshotAt: instant(item.snapshotAt), timeZone: choice(item.timeZone, ['UTC']), timeZoneSource: choice(item.timeZoneSource, ['BUILTIN_UTC']),
    range: choice(item.range, ['24h', '7d']), todayFrom: instant(item.todayFrom), todayTo: instant(item.todayTo),
    refreshIntervalSeconds: count(item.refreshIntervalSeconds), staleAfterSeconds: count(item.staleAfterSeconds), summary: text(item.summary),
    metrics: section(item.metrics, metrics), trend: section(item.trend, raw => list(raw, requestedRange === '24h' ? 25 : 7, bucket)),
    nodes: section(item.nodes, nodes), myTasks: section(item.myTasks, raw => unique(list(raw, 5, task), item => item.id)), attention: section(item.attention, attention) }
  const at = Date.parse(result.snapshotAt)
  if ([result.metrics, result.trend, result.nodes, result.myTasks, result.attention].every(section => section.status !== 'READY')) invalid()
  if (result.range !== requestedRange || result.refreshIntervalSeconds !== 30 || result.staleAfterSeconds !== 90 ||
    Date.parse(result.todayFrom) > at || Date.parse(result.todayTo) <= at || Date.parse(result.todayTo) - Date.parse(result.todayFrom) !== 86400000) invalid()
  if (result.trend.status === 'READY') {
    const buckets = result.trend.data
    if (!buckets.length || Date.parse(buckets.at(-1)!.to) !== at || buckets.some((item, index) => index > 0 && Date.parse(item.from) !== Date.parse(buckets[index - 1]!.to))) invalid()
    if (requestedRange === '24h' && at - Date.parse(buckets[0]!.from) !== 86400000) invalid()
  }
  return result
}

/** 使用既有浏览器安全传输；此接口始终只读，不接受身份、路径或执行参数。 */
export function createOverviewApi(transport: Transport): OverviewApi {
  return { async getOverview(range) { const body = await transport.read(`/api/v1/overview?range=${range}`); return parseOverviewSnapshot(body.item, range) } }
}
