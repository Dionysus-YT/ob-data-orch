// 节点采集响应使用独立的严格传输解析，不持有页面状态或发起请求。
export interface NodeHostInventory {
  readonly hostname?: string
  readonly addressPrefixes: readonly string[]
  readonly osName?: string
  readonly osVersion?: string
  readonly cpuModel?: string
  readonly logicalCpus: number
  readonly physicalCores?: number
}

export const nodeInspectionKinds = ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME', 'CONNECTOR', 'WORKSPACE', 'LOG_QUEUE', 'TOOL_LOG', 'DIRECT_LOAD_CONFIG', 'RESOURCES', 'RUNTIME_LAYOUT'] as const
export interface NodeInspectionItem {
  readonly kind: typeof nodeInspectionKinds[number]
  readonly status: 'PASSED' | 'FAILED' | 'UNKNOWN' | 'NOT_APPLICABLE'
  readonly code: string
  readonly required: boolean
  readonly path?: string
  readonly version?: string
  readonly exists?: boolean
  readonly readable?: boolean
  readonly writable?: boolean
  readonly executable?: boolean
}

export interface NodeRuntimeInspection {
  readonly checkId: string
  readonly runtimeConfigurationDigest: string
  readonly observedAt: string
  readonly versionProbeAllowed: boolean
  readonly items: readonly NodeInspectionItem[]
  readonly directLoad: {
    readonly status: 'READ' | 'MISSING' | 'UNAVAILABLE' | 'INVALID'
    readonly taskTimeoutMs?: number
    readonly heartbeatTimeoutMs?: number
    readonly heartbeatIntervalMs?: number
    readonly hasUnrecognizedKeys: boolean
    readonly fingerprint?: string
  }
}

export interface NodeActivityItem {
  readonly id: string
  readonly action: string
  readonly source: 'SUBJECT' | 'AGENT' | 'SYSTEM'
  readonly result: 'SUCCEEDED' | 'FAILED' | 'DENIED'
  readonly occurredAt: string
}
export interface NodeActivity { readonly events: readonly NodeActivityItem[]; readonly operations: readonly NodeActivityItem[]; readonly checkSource: 'SUBJECT' | 'AGENT' | 'SYSTEM' | 'UNKNOWN' }
export interface NodeRecentTasks {
  readonly scope: 'READABLE' | 'ALL'
  readonly items: readonly { readonly id: string; readonly type: string; readonly state: string; readonly reconciliationRequired: boolean; readonly submittedAt: string; readonly startedAt?: string; readonly finishedAt?: string }[]
}

function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '控制面返回了无效节点采集事实。', retryable: false, conflict: false, fieldErrors: [] } }
function record(value: unknown): Record<string, unknown> { if (!value || typeof value !== 'object' || Array.isArray(value)) invalid(); return value as Record<string, unknown> }
function text(item: Record<string, unknown>, field: string, maximum = 256): string { const value = item[field]; if (typeof value !== 'string' || !value || value !== value.trim() || value.length > maximum || Array.from(value).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) invalid(); return value }
function optionalText(item: Record<string, unknown>, field: string, maximum = 256): string | undefined { return item[field] === undefined ? undefined : text(item, field, maximum) }
function number(item: Record<string, unknown>, field: string): number { const value = item[field]; if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) invalid(); return value }
function bool(item: Record<string, unknown>, field: string): boolean { if (typeof item[field] !== 'boolean') invalid(); return item[field] }
function id(item: Record<string, unknown>, field: string): string { const value = text(item, field); if (!/^[0-9A-Za-z][0-9A-Za-z_-]{0,255}$/.test(value)) invalid(); return value }
function time(item: Record<string, unknown>, field: string): string { const value = text(item, field); if (!/^\d{4}-\d{2}-\d{2}T/.test(value) || !Number.isFinite(Date.parse(value))) invalid(); return value }
function digest(item: Record<string, unknown>, field: string): string { const value = text(item, field); if (!/^[a-f0-9]{64}$/.test(value)) invalid(); return value }
function array(item: Record<string, unknown>, field: string, maximum: number): unknown[] { const values = item[field]; if (!Array.isArray(values) || values.length > maximum) invalid(); return values }

export function parseNodeHostInventory(value: unknown): NodeHostInventory | undefined {
  if (value === undefined) return undefined
  const item = record(value)
  const logicalCpus = number(item, 'logicalCpus')
  if (logicalCpus < 1 || logicalCpus > 65536) invalid()
  const physicalCores = item.physicalCores === undefined ? undefined : number(item, 'physicalCores')
  if (physicalCores !== undefined && (physicalCores < 1 || physicalCores > 65536)) invalid()
  const addressPrefixes = array(item, 'addressPrefixes', 32).map(value => {
    if (typeof value !== 'string' || !(/^(?:\d{1,3}\.){3}0\/24$/.test(value) || /^[0-9a-f:]+::\/64$/.test(value))) invalid()
    if (value.endsWith('/24') && value.slice(0, -3).split('.').some(part => Number(part) > 255)) invalid()
    return value
  })
  return { hostname: optionalText(item, 'hostname', 253), addressPrefixes, osName: optionalText(item, 'osName', 128), osVersion: optionalText(item, 'osVersion', 128), cpuModel: optionalText(item, 'cpuModel'), logicalCpus, physicalCores }
}

export function parseNodeRuntimeInspection(value: unknown): NodeRuntimeInspection | undefined {
  if (value === undefined) return undefined
  const report = record(value)
  const versionProbeAllowed = bool(report, 'versionProbeAllowed')
  const seen = new Set<string>()
  const items = array(report, 'items', 10).map(value => {
    const item = record(value)
    const kind = text(item, 'kind') as NodeInspectionItem['kind']
    if (!nodeInspectionKinds.includes(kind) || seen.has(kind)) invalid()
    seen.add(kind)
    const status = text(item, 'status') as NodeInspectionItem['status']
    if (!['PASSED', 'FAILED', 'UNKNOWN', 'NOT_APPLICABLE'].includes(status)) invalid()
    const code = text(item, 'code')
    if (!['READY', 'MISSING', 'NOT_READABLE', 'NOT_WRITABLE', 'NOT_EXECUTABLE', 'VERSION_UNSUPPORTED', 'VERSION_UNAVAILABLE', 'VERSION_PROBE_NOT_REQUESTED', 'VERSION_PROBE_TIMEOUT', 'VERSION_OUTPUT_INVALID', 'CONFIG_INVALID', 'NOT_CONFIGURED', 'SAMPLING_UNAVAILABLE'].includes(code)) invalid()
    const required = bool(item, 'required')
    if (required !== ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'CONNECTOR', 'WORKSPACE', 'LOG_QUEUE', 'RESOURCES', 'RUNTIME_LAYOUT'].includes(kind)) invalid()
    const path = optionalText(item, 'path', 2048)
    if (path && !/^(?:[A-Za-z]:[\\/]|\/[^/])/.test(path)) invalid()
    const version = optionalText(item, 'version', 64)
    if (version && (!versionProbeAllowed || !['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME'].includes(kind) || !/^[0-9][0-9A-Za-z._+-]{0,63}$/.test(version))) invalid()
    const facts = { exists: item.exists === undefined ? undefined : bool(item, 'exists'), readable: item.readable === undefined ? undefined : bool(item, 'readable'), writable: item.writable === undefined ? undefined : bool(item, 'writable'), executable: item.executable === undefined ? undefined : bool(item, 'executable') }
    if (status === 'PASSED' && (code !== 'READY' || Object.values(facts).includes(false) || (['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME'].includes(kind) && !version))) invalid()
    if (status === 'PASSED' && version && (kind === 'JAVA_RUNTIME' ? !/^1\.8\.0_[0-9]+(?:[-+][0-9A-Za-z._+-]+)?$/.test(version) : !['4.3.5', '4.3.5-RELEASE'].includes(version))) invalid()
    return { kind, status, code, required, path, version, ...facts }
  })
  if (seen.size !== nodeInspectionKinds.length) invalid()
  const config = record(report.directLoad)
  const status = text(config, 'status') as NodeRuntimeInspection['directLoad']['status']
  if (!['READ', 'MISSING', 'UNAVAILABLE', 'INVALID'].includes(status)) invalid()
  const getInteger = (field: string) => config[field] === undefined ? undefined : number(config, field)
  const directLoad = { status, taskTimeoutMs: getInteger('taskTimeoutMs'), heartbeatTimeoutMs: getInteger('heartbeatTimeoutMs'), heartbeatIntervalMs: getInteger('heartbeatIntervalMs'), hasUnrecognizedKeys: bool(config, 'hasUnrecognizedKeys'), fingerprint: config.fingerprint === undefined ? undefined : digest(config, 'fingerprint') }
  if (status === 'READ' ? !directLoad.fingerprint : directLoad.fingerprint !== undefined || [directLoad.taskTimeoutMs, directLoad.heartbeatTimeoutMs, directLoad.heartbeatIntervalMs].some(value => value !== undefined)) invalid()
  return { checkId: id(report, 'checkId'), runtimeConfigurationDigest: digest(report, 'runtimeConfigurationDigest'), observedAt: time(report, 'observedAt'), versionProbeAllowed, items, directLoad }
}

const actions = ['EXECUTION_NODE_CREATED', 'EXECUTION_NODE_UPDATED', 'EXECUTION_NODE_ENABLED', 'EXECUTION_NODE_ARCHIVED', 'EXECUTION_NODE_DELETED', 'AGENT_ENROLLMENT_ISSUED', 'AGENT_ENROLLED', 'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED', 'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED', 'EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETED']
export function parseNodeActivity(value: unknown): NodeActivity | undefined {
  if (value === undefined) return undefined
  const item = record(value)
  const parseItems = (field: string) => array(item, field, 5).map(value => {
    const item = record(value)
    const action = text(item, 'action')
    const source = text(item, 'source') as NodeActivityItem['source']
    const result = text(item, 'result') as NodeActivityItem['result']
    if (!actions.includes(action) || !['SUBJECT', 'AGENT', 'SYSTEM'].includes(source) || !['SUCCEEDED', 'FAILED', 'DENIED'].includes(result)) invalid()
    return { id: id(item, 'id'), action, source, result, occurredAt: time(item, 'occurredAt') }
  })
  const checkSource = text(item, 'checkSource') as NodeActivity['checkSource']
  if (!['SUBJECT', 'AGENT', 'SYSTEM', 'UNKNOWN'].includes(checkSource)) invalid()
  return { events: parseItems('events'), operations: parseItems('operations'), checkSource }
}

export function parseNodeRecentTasks(value: unknown): NodeRecentTasks | undefined {
  if (value === undefined) return undefined
  const item = record(value)
  const scope = text(item, 'scope') as NodeRecentTasks['scope']
  if (scope !== 'READABLE' && scope !== 'ALL') invalid()
  const items = array(item, 'items', 5).map(value => {
    const item = record(value)
    const state = text(item, 'state')
    if (!['WAITING_SCHEDULE', 'STARTING', 'RUNNING', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED'].includes(state) || text(item, 'type') !== 'OBDUMPER_EXPORT') invalid()
    return { id: id(item, 'id'), type: 'OBDUMPER_EXPORT', state, reconciliationRequired: bool(item, 'reconciliationRequired'), submittedAt: time(item, 'submittedAt'), startedAt: item.startedAt === undefined ? undefined : time(item, 'startedAt'), finishedAt: item.finishedAt === undefined ? undefined : time(item, 'finishedAt') }
  })
  return { scope, items }
}
