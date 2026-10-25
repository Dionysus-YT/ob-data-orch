import { invalidNodeResponse as invalid, readNodeResponse } from './nodeResponseValidation'

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
  readonly items: readonly { readonly id: string; readonly type: 'OBDUMPER_EXPORT' | 'OBLOADER_NORMAL_IMPORT' | 'OBLOADER_DIRECT_LOAD'; readonly state: string; readonly reconciliationRequired: boolean; readonly submittedAt: string; readonly startedAt?: string; readonly finishedAt?: string }[]
}

const fieldLabels: Readonly<Record<string, string>> = {
  hostname: '主机名', osName: '操作系统名称', osVersion: '操作系统版本', cpuModel: 'CPU 型号', logicalCpus: '逻辑 CPU 数', physicalCores: '物理核心数', addressPrefixes: '网段摘要',
  checkId: '检查标识', runtimeConfigurationDigest: '运行配置摘要', observedAt: '采样时间', versionProbeAllowed: '版本查询许可', items: '明细列表',
  kind: '检查项类型', status: '状态', code: '结果码', required: '必需项标记', path: '检查路径', version: '版本',
  exists: '文件存在标记', readable: '可读标记', writable: '可写标记', executable: '可执行标记',
  taskTimeoutMs: '任务超时', heartbeatTimeoutMs: '心跳超时', heartbeatIntervalMs: '心跳间隔', hasUnrecognizedKeys: '未识别配置项标记', fingerprint: '配置指纹',
  events: '事件列表', operations: '操作列表', checkSource: '检查来源', action: '操作类型', source: '操作来源', result: '操作结果', occurredAt: '操作时间',
  scope: '读取范围', type: '任务类型', state: '任务状态', reconciliationRequired: '状态核对标记', submittedAt: '任务提交时间', startedAt: '开始时间', finishedAt: '完成时间', id: '记录标识',
}

function label(field: string): string { return fieldLabels[field] ?? '字段' }
function record(value: unknown): Record<string, unknown> { if (!value || typeof value !== 'object' || Array.isArray(value)) invalid('返回的数据结构不是有效记录。'); return value as Record<string, unknown> }
function text(item: Record<string, unknown>, field: string, maximum = 256): string { const value = item[field]; if (typeof value !== 'string' || !value || value !== value.trim() || value.length > maximum || Array.from(value).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) invalid(`${label(field)}缺失，或文本格式不正确。`); return value }
function optionalText(item: Record<string, unknown>, field: string, maximum = 256): string | undefined { return item[field] === undefined ? undefined : text(item, field, maximum) }
function number(item: Record<string, unknown>, field: string): number { const value = item[field]; if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) invalid(`${label(field)}缺失，或不是精度可靠的非负整数。`); return value }
function bool(item: Record<string, unknown>, field: string): boolean { if (typeof item[field] !== 'boolean') invalid(`${label(field)}缺失，或不是有效的是非标记。`); return item[field] }
function id(item: Record<string, unknown>, field: string): string { const value = text(item, field); if (!/^[0-9A-Za-z][0-9A-Za-z_-]{0,255}$/.test(value)) invalid(`${label(field)}的格式不正确。`); return value }
function time(item: Record<string, unknown>, field: string): string { const value = text(item, field); if (!/^\d{4}-\d{2}-\d{2}T/.test(value) || !Number.isFinite(Date.parse(value))) invalid(`${label(field)}不是有效的时间。`); return value }
function digest(item: Record<string, unknown>, field: string): string { const value = text(item, field); if (!/^[a-f0-9]{64}$/.test(value)) invalid(`${label(field)}的格式不正确。`); return value }
function array(item: Record<string, unknown>, field: string, maximum: number): unknown[] { const values = item[field]; if (!Array.isArray(values) || values.length > maximum) invalid(`${label(field)}缺失，或超过允许的 ${maximum} 条记录。`); return values }

export function parseNodeHostInventory(value: unknown): NodeHostInventory | undefined {
  if (value === undefined) return undefined
  return readNodeResponse('host', () => {
    const item = record(value)
    const logicalCpus = number(item, 'logicalCpus')
    if (logicalCpus < 1 || logicalCpus > 65536) invalid('逻辑 CPU 数不在支持范围内。')
    const physicalCores = item.physicalCores === undefined ? undefined : number(item, 'physicalCores')
    if (physicalCores !== undefined && (physicalCores < 1 || physicalCores > 65536)) invalid('物理核心数不在支持范围内。')
    const addressPrefixes = array(item, 'addressPrefixes', 32).map(value => {
      if (typeof value !== 'string' || !(/^(?:\d{1,3}\.){3}0\/24$/.test(value) || /^[0-9a-f:]+::\/64$/.test(value))) invalid('网段摘要格式不正确，不能展示未经确认的地址。')
      if (value.endsWith('/24') && value.slice(0, -3).split('.').some(part => Number(part) > 255)) invalid('网段摘要含有超出范围的地址。')
      return value
    })
    return { hostname: optionalText(item, 'hostname', 253), addressPrefixes, osName: optionalText(item, 'osName', 128), osVersion: optionalText(item, 'osVersion', 128), cpuModel: optionalText(item, 'cpuModel'), logicalCpus, physicalCores }
  })
}

export function parseNodeRuntimeInspection(value: unknown): NodeRuntimeInspection | undefined {
  if (value === undefined) return undefined
  return readNodeResponse('inspection', () => {
    const report = record(value)
    const versionProbeAllowed = bool(report, 'versionProbeAllowed')
    const seen = new Set<string>()
    const items = array(report, 'items', 10).map(value => {
      const item = record(value)
      const kind = text(item, 'kind') as NodeInspectionItem['kind']
      if (!nodeInspectionKinds.includes(kind)) invalid('存在当前页面尚未支持的检查项类型。')
      if (seen.has(kind)) invalid('同一检查项重复出现，无法确认完整检查结果。')
      seen.add(kind)
      const status = text(item, 'status') as NodeInspectionItem['status']
      if (!['PASSED', 'FAILED', 'UNKNOWN', 'NOT_APPLICABLE'].includes(status)) invalid('存在当前页面尚未支持的检查状态。')
      const code = text(item, 'code')
      if (!['READY', 'MISSING', 'NOT_READABLE', 'NOT_WRITABLE', 'NOT_EXECUTABLE', 'VERSION_UNSUPPORTED', 'VERSION_UNAVAILABLE', 'VERSION_PROBE_NOT_REQUESTED', 'VERSION_PROBE_TIMEOUT', 'VERSION_OUTPUT_INVALID', 'CONFIG_INVALID', 'NOT_CONFIGURED', 'SAMPLING_UNAVAILABLE'].includes(code)) invalid('存在当前页面尚未支持的检查结果码。')
      const required = bool(item, 'required')
      if (required !== ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'CONNECTOR', 'WORKSPACE', 'LOG_QUEUE', 'RESOURCES', 'RUNTIME_LAYOUT'].includes(kind)) invalid('必需检查项的标记与约定不一致。')
      const path = optionalText(item, 'path', 2048)
      if (path && !/^(?:[A-Za-z]:[\\/]|\/[^/])/.test(path)) invalid('检查路径不是受支持的本机绝对路径。')
      const version = optionalText(item, 'version', 64)
      if (version && (!versionProbeAllowed || !['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME'].includes(kind) || !/^[0-9][0-9A-Za-z._+-]{0,63}$/.test(version))) invalid('版本信息与查询许可或检查项不一致，或版本格式不正确。')
      const facts = { exists: item.exists === undefined ? undefined : bool(item, 'exists'), readable: item.readable === undefined ? undefined : bool(item, 'readable'), writable: item.writable === undefined ? undefined : bool(item, 'writable'), executable: item.executable === undefined ? undefined : bool(item, 'executable') }
      if (status === 'PASSED' && (code !== 'READY' || Object.values(facts).includes(false) || (['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME'].includes(kind) && !version))) invalid('检查标记为通过，但结果码、访问事实或版本信息不能支持该结论。')
      if (status === 'PASSED' && version && (kind === 'JAVA_RUNTIME' ? !/^1\.8\.0_[0-9]+(?:[-+][0-9A-Za-z._+-]+)?$/.test(version) : !['4.3.5', '4.3.5-RELEASE'].includes(version))) invalid('检查标记为通过，但上报版本不符合 Java 8 或工具 4.3.5 基线。')
      return { kind, status, code, required, path, version, ...facts }
    })
    if (seen.size !== nodeInspectionKinds.length) invalid('十项固定检查未全部返回，不能确认完整检查结果。')
    const config = record(report.directLoad)
    const status = text(config, 'status') as NodeRuntimeInspection['directLoad']['status']
    if (!['READ', 'MISSING', 'UNAVAILABLE', 'INVALID'].includes(status)) invalid('旁路配置的读取状态尚未支持。')
    const getInteger = (field: string) => config[field] === undefined ? undefined : number(config, field)
    const directLoad = { status, taskTimeoutMs: getInteger('taskTimeoutMs'), heartbeatTimeoutMs: getInteger('heartbeatTimeoutMs'), heartbeatIntervalMs: getInteger('heartbeatIntervalMs'), hasUnrecognizedKeys: bool(config, 'hasUnrecognizedKeys'), fingerprint: config.fingerprint === undefined ? undefined : digest(config, 'fingerprint') }
    if (status === 'READ' ? !directLoad.fingerprint : directLoad.fingerprint !== undefined || [directLoad.taskTimeoutMs, directLoad.heartbeatTimeoutMs, directLoad.heartbeatIntervalMs].some(value => value !== undefined)) invalid('旁路配置的读取状态与配置指纹或超时信息不一致。')
    return { checkId: id(report, 'checkId'), runtimeConfigurationDigest: digest(report, 'runtimeConfigurationDigest'), observedAt: time(report, 'observedAt'), versionProbeAllowed, items, directLoad }
  })
}

const actions = ['EXECUTION_NODE_CREATED', 'EXECUTION_NODE_UPDATED', 'EXECUTION_NODE_ENABLED', 'EXECUTION_NODE_ARCHIVED', 'EXECUTION_NODE_DELETED', 'AGENT_ENROLLMENT_ISSUED', 'AGENT_ENROLLED', 'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED', 'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUEUED', 'EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETED']
export function parseNodeActivity(value: unknown): NodeActivity | undefined {
  if (value === undefined) return undefined
  return readNodeResponse('activity', () => {
    const item = record(value)
    const parseItems = (field: string) => array(item, field, 5).map(value => {
      const item = record(value)
      const action = text(item, 'action')
      const source = text(item, 'source') as NodeActivityItem['source']
      const result = text(item, 'result') as NodeActivityItem['result']
      if (!actions.includes(action)) invalid('存在当前页面尚未支持的节点操作类型。')
      if (!['SUBJECT', 'AGENT', 'SYSTEM'].includes(source) || !['SUCCEEDED', 'FAILED', 'DENIED'].includes(result)) invalid('操作来源或结果不在支持范围内。')
      return { id: id(item, 'id'), action, source, result, occurredAt: time(item, 'occurredAt') }
    })
    const checkSource = text(item, 'checkSource') as NodeActivity['checkSource']
    if (!['SUBJECT', 'AGENT', 'SYSTEM', 'UNKNOWN'].includes(checkSource)) invalid('检查来源不在支持范围内。')
    return { events: parseItems('events'), operations: parseItems('operations'), checkSource }
  })
}

export function parseNodeRecentTasks(value: unknown): NodeRecentTasks | undefined {
  if (value === undefined) return undefined
  return readNodeResponse('recentTasks', () => {
    const item = record(value)
    const scope = text(item, 'scope') as NodeRecentTasks['scope']
    if (scope !== 'READABLE' && scope !== 'ALL') invalid('任务读取范围不在支持范围内，不能确认可展示的任务。')
    const items = array(item, 'items', 5).map((value): NodeRecentTasks['items'][number] => {
      const item = record(value)
      const state = text(item, 'state')
      const type = text(item, 'type')
      if (type !== 'OBDUMPER_EXPORT' && type !== 'OBLOADER_NORMAL_IMPORT' && type !== 'OBLOADER_DIRECT_LOAD') invalid('存在当前页面尚未支持的任务类型。')
      if (!['WAITING_SCHEDULE', 'STARTING', 'RUNNING', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED'].includes(state)) invalid('存在当前页面尚未支持的任务状态。')
      return { id: id(item, 'id'), type, state, reconciliationRequired: bool(item, 'reconciliationRequired'), submittedAt: time(item, 'submittedAt'), startedAt: item.startedAt === undefined ? undefined : time(item, 'startedAt'), finishedAt: item.finishedAt === undefined ? undefined : time(item, 'finishedAt') }
    })
    return { scope, items }
  })
}
