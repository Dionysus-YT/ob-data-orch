export interface SecurityPolicy {
  readonly capturedAt: string
  readonly loadedAt: string
  readonly source: 'STARTUP_CONFIGURATION'
  readonly builtInRedaction: true
  readonly credentialFormat: 'credential-aes-gcm-v1'
  readonly credentialKeyId: string
  readonly rootKeyStatus: 'LOADED'
  readonly rootKeySource: 'LOCAL_PROTECTED_FILE'
  readonly sensitiveCommandView: 'NOT_AVAILABLE'
  readonly productionConfirmation: 'TASK_SUBMISSION_RULES'
  readonly auditRetentionDays: null
  readonly auditAutomaticCleanup: false
  readonly realExecutionEnabled: boolean
  readonly normalImportExecutionEnabled: boolean
  readonly directLoadProbeEnabled: boolean
  readonly directLoadExecutionEnabled: boolean
}

export interface SecurityPolicyApi { getSecurityPolicy(signal?: AbortSignal): Promise<SecurityPolicy> }
const fields = ['capturedAt', 'loadedAt', 'source', 'builtInRedaction', 'credentialFormat', 'credentialKeyId', 'rootKeyStatus', 'rootKeySource', 'sensitiveCommandView', 'productionConfirmation', 'auditRetentionDays', 'auditAutomaticCleanup', 'realExecutionEnabled', 'normalImportExecutionEnabled', 'directLoadProbeEnabled', 'directLoadExecutionEnabled'] as const
function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '安全状态响应无法校验，请重新读取。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).some(key => !keys.includes(key))) invalid()
  return value as Record<string, unknown>
}
function timestamp(value: unknown): value is string {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) || !Number.isFinite(Date.parse(value))) return false
  // Date.parse 会归一不存在的日期，必须回查日期与秒，避免把坏载入时间当成事实。
  return new Date(value).toISOString().slice(0, 19) === value.slice(0, 19)
}

/** 安全页只接收非秘密白名单；未知保护状态、审计清理或矛盾门禁不得显示为已生效。 */
export function parseSecurityPolicy(value: unknown): SecurityPolicy {
  const row = object(value, fields)
  if (fields.some(field => !Object.hasOwn(row, field)) || !timestamp(row.capturedAt) || !timestamp(row.loadedAt) || Date.parse(row.loadedAt) > Date.parse(row.capturedAt) || row.source !== 'STARTUP_CONFIGURATION' || row.builtInRedaction !== true || row.credentialFormat !== 'credential-aes-gcm-v1' || row.rootKeyStatus !== 'LOADED' || row.rootKeySource !== 'LOCAL_PROTECTED_FILE' || typeof row.credentialKeyId !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(row.credentialKeyId) || row.sensitiveCommandView !== 'NOT_AVAILABLE' || row.productionConfirmation !== 'TASK_SUBMISSION_RULES' || row.auditRetentionDays !== null || row.auditAutomaticCleanup !== false) invalid()
  if (['realExecutionEnabled', 'normalImportExecutionEnabled', 'directLoadProbeEnabled', 'directLoadExecutionEnabled'].some(key => typeof row[key] !== 'boolean') || (!row.realExecutionEnabled && (row.normalImportExecutionEnabled || row.directLoadProbeEnabled || row.directLoadExecutionEnabled)) || (row.directLoadExecutionEnabled && !row.directLoadProbeEnabled)) invalid()
  return Object.fromEntries(fields.map(field => [field, row[field]])) as unknown as SecurityPolicy
}

/** 仅读取独立安全投影，不提供任意部署配置或修改入口。 */
export function createSecurityPolicyApi(transport: { read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>> }): SecurityPolicyApi {
  return { async getSecurityPolicy(signal) { return parseSecurityPolicy(object(await transport.read('/api/v1/settings/security', signal), ['requestId', 'item']).item) } }
}
