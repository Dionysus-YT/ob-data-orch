// 旁路传输边界独立于普通导入；未知配置、证据和未脱敏命令均失败关闭。
// 旧场景与格式仅用于安全读取历史配置；活动校验与写入只允许当前本地部署范围。
export interface DirectLoadConfig {
  dataSourceId: string; nodeId: string; database: string; scenario: '' | 'CLOUD_ODP' | 'PRIVATE_ODP' | 'OBSERVER_DIRECT'
  rpcPort: number; table: string; filePath: string; format: '' | 'CSV' | 'SQL' | 'PARQUET' | 'ORC'
  externalData: boolean; columnSeparator: string; thread: string; parallel: string; replaceData: boolean; truncateTable: boolean
  applicability: { singleTable: boolean; wholeTableCommit: boolean; noCheckpoint: boolean; structureLimits: boolean }
}
export interface DirectLoadDraft {
  id: string; revision: number; configVersion: 'direct-load-v1'; config: DirectLoadConfig; configFingerprint: string; executionEnabled: boolean
  sourceTaskId?: string; sourceDerivation?: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH'; parametersLocked?: boolean
}
export interface DirectLoadCheck {
  id: string; draftId: string; draftRevision: number; configFingerprint: string; checkKind: 'DIRECT_LOAD_FILES' | 'DIRECT_LOAD_PREFLIGHT'
  status: 'PENDING' | 'LEASED' | 'SUCCEEDED' | 'FAILED' | 'INVALIDATED' | 'EXPIRED'; integrityStatus: string; validUntil: string; executionEnabled: boolean
  results: { check: string; status: 'PASSED' | 'FAILED' | 'UNKNOWN'; evidenceCode: string }[]
  directLoadConfigFingerprint?: string
  submittedTaskId?: string
  inputManifest?: { fileCount: number; totalBytes: number; digest: string; files: { path: string; object: string; size: number; modified: string; sha256: string }[] }
}
export interface DirectLoadCommand { redactedCommand: string; configFingerprint: string; executionEnabled: boolean }
export interface DirectLoadParameter { longName: string; supportState: string; category: string; sourcePolicy: string; description: string; disposition: string }
export interface DirectLoadApi {
  createDirectLoadDraft(config: DirectLoadConfig): Promise<DirectLoadDraft>
  updateDirectLoadDraft(draft: DirectLoadDraft, config: DirectLoadConfig): Promise<DirectLoadDraft>
  getDirectLoadDraft(id: string): Promise<DirectLoadDraft>
  deleteDirectLoadDraft(draft: Pick<DirectLoadDraft, 'id' | 'revision'>): Promise<void>
  previewDirectLoadCommand(draft: DirectLoadDraft): Promise<DirectLoadCommand>
  startDirectLoadCheck(draft: DirectLoadDraft, filesOnly: boolean): Promise<string>
  getDirectLoadCheck(id: string): Promise<DirectLoadCheck>
  submitDirectLoad(draft: DirectLoadDraft, precheckId: string, confirmedConfigFingerprint: string, taskName: string, idempotencyKey?: string): Promise<string>
  getDirectLoadParameters(): Promise<DirectLoadParameter[]>
}
type RecordValue = Record<string, unknown>
interface Transport { read(path: string): Promise<RecordValue>; write(path: string, body: unknown, revision?: number, method?: string, idempotencyKey?: string): Promise<RecordValue> }
const base = '/api/v1/direct-load-drafts'
function invalid(): never { throw Object.assign(new Error('旁路导入响应无法安全校验，已停止展示或提交。'), { code: 'RESPONSE_INVALID' }) }
function object(raw: unknown): RecordValue { return raw && typeof raw === 'object' && !Array.isArray(raw) ? raw as RecordValue : invalid() }
function string(raw: unknown, max = 2048): string { return typeof raw === 'string' && raw.length <= max && !Array.from(raw).some(char => char.charCodeAt(0) < 32 && !['\n', '\t', '\r'].includes(char)) ? raw : invalid() }
function integer(raw: unknown, min = 0): number { return typeof raw === 'number' && Number.isSafeInteger(raw) && raw >= min ? raw : invalid() }
function boolean(raw: unknown): boolean { return typeof raw === 'boolean' ? raw : invalid() }
function identifier(raw: unknown): string { const value = string(raw, 256); return /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/.test(value) ? value : invalid() }
function fingerprint(raw: unknown): string { const value = string(raw); return /^[a-f0-9]{64}$/.test(value) ? value : invalid() }
function known(raw: RecordValue, keys: readonly string[]) { if (Object.keys(raw).some(key => !keys.includes(key))) invalid() }
export function parseDirectLoadConfig(raw: unknown): DirectLoadConfig {
  const config = object(raw)
  const textKeys = ['dataSourceId', 'nodeId', 'database', 'scenario', 'table', 'filePath', 'format', 'columnSeparator', 'thread', 'parallel']
  const boolKeys = ['externalData', 'replaceData', 'truncateTable']
  known(config, [...textKeys, ...boolKeys, 'rpcPort', 'applicability'])
  for (const key of ['columnSeparator', 'thread', 'parallel']) if (config[key] === undefined) config[key] = ''
  for (const key of boolKeys) if (config[key] === undefined) config[key] = false
  for (const key of textKeys) string(config[key])
  for (const key of boolKeys) boolean(config[key])
  if (integer(config.rpcPort) > 65535 || !['', 'CLOUD_ODP', 'PRIVATE_ODP', 'OBSERVER_DIRECT'].includes(string(config.scenario)) || !['', 'CSV', 'SQL', 'PARQUET', 'ORC'].includes(string(config.format))) invalid()
  const applicability = object(config.applicability), keys = ['singleTable', 'wholeTableCommit', 'noCheckpoint', 'structureLimits']
  known(applicability, keys); for (const key of keys) boolean(applicability[key])
  return structuredClone(config) as unknown as DirectLoadConfig
}
function parseDraft(value: RecordValue): DirectLoadDraft {
  if (value.configVersion !== 'direct-load-v1' || value.metadataVersion !== 'obloader-4.3.5-direct-v1' || value.capabilityVersion !== 'direct-load-local-v1' || value.rpcPortSource !== 'MANUAL') invalid()
  const sourceTaskId = value.sourceTaskId ? identifier(value.sourceTaskId) : undefined
  const sourceDerivation = value.sourceDerivation || undefined
  if (Boolean(sourceTaskId) !== Boolean(sourceDerivation) || sourceDerivation !== undefined && !['REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH'].includes(string(sourceDerivation))) invalid()
  const parametersLocked = value.parametersLocked === undefined ? sourceDerivation === 'RERUN_FROM_SCRATCH' : boolean(value.parametersLocked)
  if (parametersLocked !== (sourceDerivation === 'RERUN_FROM_SCRATCH')) invalid()
  return { id: identifier(value.id), revision: integer(value.revision, 1), configVersion: 'direct-load-v1', config: parseDirectLoadConfig(value.config), configFingerprint: fingerprint(value.configFingerprint), executionEnabled: boolean(value.executionEnabled), parametersLocked, ...(sourceTaskId ? { sourceTaskId, sourceDerivation: sourceDerivation as DirectLoadDraft['sourceDerivation'] } : {}) }
}
export function createDirectLoadApi(transport: Transport): DirectLoadApi {
  const path = (draft: Pick<DirectLoadDraft, 'id'>) => `${base}/${encodeURIComponent(draft.id)}`
  return {
    async createDirectLoadDraft(config) { return parseDraft(await transport.write(base, { configVersion: 'direct-load-v1', config })) },
    async updateDirectLoadDraft(draft, config) { return parseDraft(await transport.write(path(draft), { configVersion: 'direct-load-v1', config }, draft.revision, 'PATCH')) },
    async getDirectLoadDraft(id) { return parseDraft(await transport.read(path({ id }))) },
    async deleteDirectLoadDraft(draft) { if ((await transport.write(path(draft), {}, draft.revision, 'DELETE')).deleted !== true) invalid() },
    async previewDirectLoadCommand(draft) {
      const value = await transport.write(`${path(draft)}:preview-command`, {}, draft.revision), command = string(value.redactedCommand, 131072)
      if (!command.startsWith('obloader ') || !/(?:^|\s)--direct(?:\s|$)/.test(command) || /(?:--(?:sys-)?password|(?:^|\s)-p)(?:\s+|=)(?!["']?\*{3,}["']?(?:\s|$))\S+|\bpassword\s*[:=]\s*\S+/i.test(command)) invalid()
      return { redactedCommand: command, configFingerprint: fingerprint(value.configFingerprint), executionEnabled: boolean(value.executionEnabled) }
    },
    async startDirectLoadCheck(draft, filesOnly) { return identifier((await transport.write(`${path(draft)}:${filesOnly ? 'discover-files' : 'precheck'}`, {}, draft.revision)).id) },
    async getDirectLoadCheck(id) {
      const value = await transport.read(`/api/v1/direct-load-prechecks/${encodeURIComponent(id)}`), kind = value.checkKind
      if (!['DIRECT_LOAD_FILES', 'DIRECT_LOAD_PREFLIGHT'].includes(string(kind)) || !['PENDING', 'LEASED', 'SUCCEEDED', 'FAILED', 'INVALIDATED', 'EXPIRED'].includes(string(value.status)) || !Array.isArray(value.results) || value.results.length > 16) invalid()
      const checks = kind === 'DIRECT_LOAD_FILES' ? ['INPUT_FILES'] : ['TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'DATABASE_CONNECTIVITY', 'RPC_CONNECTIVITY', 'DIRECT_LOAD_VERSION', 'WRITE_ACCESS', 'DIRECT_LOAD_STRUCTURE', 'DIRECT_LOAD_CONFIG']
      if (value.results.length !== checks.length) invalid()
      const results = value.results.map((raw, index) => {
        const item = object(raw), check = identifier(item.check), status = string(item.status), evidenceCode = identifier(item.evidenceCode)
        const allowed: Record<string, string[]> = { PASSED: ['DIRECT_LOAD_VERIFIED'], FAILED: ['DIRECT_LOAD_REJECTED'], UNKNOWN: ['DIRECT_LOAD_UNAVAILABLE', 'DIRECT_LOAD_PROBE_NOT_CONFIGURED', 'PRECHECK_BLOCKED_BY_TOOL', 'PRECHECK_BLOCKED_BY_DATABASE'] }
        if (check === 'DIRECT_LOAD_STRUCTURE') {
          allowed.FAILED!.push('DIRECT_LOAD_TABLE_NOT_FOUND', 'DIRECT_LOAD_BIT_BLOCKED', 'DIRECT_LOAD_BINARY_BLOCKED', 'DIRECT_LOAD_VIRTUAL_BLOCKED', 'DIRECT_LOAD_REPLACE_INDEX_BLOCKED', 'DIRECT_LOAD_REPLACE_LOB_BLOCKED')
          allowed.UNKNOWN!.push('DIRECT_LOAD_TABLE_FACT_UNAVAILABLE')
        }
        if (check !== checks[index] || !allowed[status]?.includes(evidenceCode)) invalid()
        return { check, status: status as 'PASSED' | 'FAILED' | 'UNKNOWN', evidenceCode }
      })
      const directLoadConfigFingerprint = value.directLoadConfigFingerprint === undefined ? undefined : fingerprint(value.directLoadConfigFingerprint)
      const hasConfigEvidence = kind === 'DIRECT_LOAD_PREFLIGHT' && results.some(result => result.check === 'DIRECT_LOAD_CONFIG' && result.status === 'PASSED')
      if (hasConfigEvidence !== Boolean(directLoadConfigFingerprint)) invalid()
      const validUntil = string(value.validUntil); if (!Number.isFinite(Date.parse(validUntil))) invalid()
      let inputManifest: DirectLoadCheck['inputManifest']
      if (value.inputManifest) {
        const manifest = object(value.inputManifest), fileCount = integer(manifest.fileCount, 1), totalBytes = integer(manifest.totalBytes), paths = new Set<string>()
        if (fileCount > 1024 || !Array.isArray(manifest.files) || manifest.files.length !== fileCount) invalid()
        const files = manifest.files.map(raw => { const file = object(raw), path = string(file.path, 512), modified = string(file.modified, 24); if (!path || path.startsWith('/') || /[\\:\r\n]/.test(path) || path.split('/').some(part => !part || part === '.' || part === '..') || paths.has(path) || file.kind !== 'DATA' || !/^-?[0-9]{1,19}$/.test(modified)) invalid(); paths.add(path); return { path, object: string(file.object, 256), size: integer(file.size), modified, sha256: fingerprint(file.sha256) } })
        if (files.reduce((sum, item) => sum + item.size, 0) !== totalBytes || new Set(files.map(item => item.object)).size !== 1 || files.some(item => !item.object)) invalid()
        inputManifest = { fileCount, totalBytes, digest: fingerprint(manifest.digest), files }
      }
      if (value.status === 'SUCCEEDED' && (value.integrityStatus !== 'COMPLETE' || !results.length || results.some(item => item.status !== 'PASSED') || !inputManifest)) invalid()
      const submittedTaskId = value.submittedTaskId === undefined ? undefined : identifier(value.submittedTaskId)
      if (submittedTaskId && kind !== 'DIRECT_LOAD_PREFLIGHT') invalid()
      return { id: identifier(value.id), draftId: identifier(value.draftId), draftRevision: integer(value.draftRevision, 1), configFingerprint: fingerprint(value.configFingerprint), checkKind: kind as DirectLoadCheck['checkKind'], status: value.status as DirectLoadCheck['status'], integrityStatus: string(value.integrityStatus), validUntil, executionEnabled: boolean(value.executionEnabled), results, inputManifest, ...(directLoadConfigFingerprint ? {directLoadConfigFingerprint} : {}), ...(submittedTaskId ? {submittedTaskId} : {}) }
    },
    async submitDirectLoad(draft, precheckId, confirmedConfigFingerprint, taskName, idempotencyKey) {
      if (idempotencyKey !== undefined && !/^[a-zA-Z0-9_-]{16,200}$/.test(idempotencyKey)) invalid()
      return identifier((await transport.write(`${path(draft)}:submit`, { precheckId, taskName: taskName.trim(), confirmedConfigFingerprint }, draft.revision, 'POST', idempotencyKey)).id)
    },
    async getDirectLoadParameters() {
      const value = await transport.read('/api/v1/direct-load-parameter-metadata')
      if (value.metadataVersion !== 'obloader-4.3.5-direct-v1' || value.capabilityVersion !== 'direct-load-local-v1' || !Array.isArray(value.definitions) || value.definitions.length !== 103) invalid()
      const names = new Set<string>()
      return value.definitions.map(raw => { const item = object(raw), name = string(item.longName, 128); if (!/^--[a-z][a-z0-9-]*$/.test(name) || names.has(name) || !['ENABLED', 'VALIDATION_GATED', 'UNSUPPORTED'].includes(string(item.supportState)) || !Array.isArray(item.officialEvidence) || item.officialEvidence.length < 4) invalid(); names.add(name); return { longName: name.slice(2), supportState: string(item.supportState, 128), category: string(item.category, 128), sourcePolicy: string(item.sourcePolicy, 128), description: string(item.officialEvidence[2]), disposition: string(item.officialEvidence[3]) } })
    },
  }
}
