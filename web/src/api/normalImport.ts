// 普通导入使用独立 DTO，不接收导出快照、旁路参数或客户端运行事实。
export interface NormalImportConfig {
  dataSourceId: string; nodeId: string; database: string; filePath: string
  content: '' | 'DDL_ONLY' | 'DATA_ONLY' | 'DDL_AND_DATA'; format?: '' | 'CSV' | 'CUT' | 'SQL'; scope: '' | 'ALL' | 'SPECIFIC'
  fileSuffix?: string; fileEncoding?: string; fileRegex?: string; logPath?: string; excludeTables?: string; taskName?: string
  objects?: Record<string, string>; parser: Record<string, string | boolean>; execution: Record<string, string>
  mapping?: { kind: 'POSITION' | 'CONTROL' | 'AUTO' | 'EXPLICIT'; ctlPath?: string; includeColumns?: string; excludeColumns?: string; excludeDataTypes?: string }
  compatMysql?: boolean; replaceData?: boolean; replaceObject?: boolean; cleanup: 'NONE' | 'TRUNCATE' | 'DELETE'
}
export interface NormalImportDraft {
  id: string; revision: number; configVersion: 'normal-import-v1'; config: NormalImportConfig; configFingerprint: string; executionEnabled: boolean
}
export interface NormalImportManifest {
  metadataDigest?: string
  fileCount: number; totalBytes: number; digest: string
  files: { path: string; kind: 'DATA' | 'DDL'; object: string; size: number; modified: string; sha256: string }[]
}
export interface NormalImportCheck {
  id: string; draftId: string; draftRevision: number; configFingerprint: string
  checkKind: 'NORMAL_IMPORT_FILES' | 'NORMAL_IMPORT_PREFLIGHT'
  status: 'PENDING' | 'LEASED' | 'SUCCEEDED' | 'FAILED' | 'INVALIDATED' | 'EXPIRED'
  integrityStatus: string; validUntil: string; executionEnabled: boolean
  results: { check: string; status: 'PASSED' | 'FAILED' | 'UNKNOWN'; evidenceCode: string }[]
  inputManifest?: NormalImportManifest
}
export interface NormalImportCommand { redactedCommand: string; configFingerprint: string; executionEnabled: boolean }
export interface NormalImportApi {
  createNormalImportDraft(config: NormalImportConfig): Promise<NormalImportDraft>
  updateNormalImportDraft(draft: NormalImportDraft, config: NormalImportConfig): Promise<NormalImportDraft>
  getNormalImportDraft(id: string): Promise<NormalImportDraft>
  listNormalImportDrafts(): Promise<{ id: string; revision: number; updatedAt: string }[]>
  deleteNormalImportDraft(draft: NormalImportDraft): Promise<void>
  previewNormalImportCommand(draft: NormalImportDraft): Promise<NormalImportCommand>
  startNormalImportCheck(draft: NormalImportDraft, filesOnly: boolean): Promise<string>
  getNormalImportCheck(id: string): Promise<NormalImportCheck>
  submitNormalImport(draft: NormalImportDraft, checkId: string, confirmedConfigFingerprint?: string, taskName?: string): Promise<string>
}

type ObjectValue = Record<string, unknown>
interface Transport {
  read(path: string): Promise<ObjectValue>
  write(path: string, body: unknown, revision?: number, method?: string): Promise<ObjectValue>
}
const base = '/api/v1/normal-import-drafts'
const digest = /^[a-f0-9]{64}$/
function invalid(): never { throw new Error('普通导入响应无效，已停止展示或提交。') }
function object(value: unknown): ObjectValue { return value && typeof value === 'object' && !Array.isArray(value) ? value as ObjectValue : invalid() }
function string(value: unknown): string { return typeof value === 'string' && value.length <= 131072 ? value : invalid() }
function integer(value: unknown, min = 0): number { return typeof value === 'number' && Number.isSafeInteger(value) && value >= min ? value : invalid() }
function fingerprint(value: unknown): string { const result = string(value); return digest.test(result) ? result : invalid() }
function identifier(value: unknown): string { const result = string(value); return result && result.length <= 256 && !/[\s/\\?#]/.test(result) && !Array.from(result).some(character => character.charCodeAt(0) < 32) ? result : invalid() }
function known(value: ObjectValue, fields: readonly string[]) { if (Object.keys(value).some(key => !fields.includes(key))) invalid() }
const configKeys = ['dataSourceId', 'nodeId', 'database', 'filePath', 'fileSuffix', 'fileEncoding', 'fileRegex', 'logPath', 'content', 'format', 'scope', 'objects', 'excludeTables', 'parser', 'mapping', 'execution', 'compatMysql', 'replaceData', 'replaceObject', 'cleanup', 'taskName']
const parserStrings = ['columnSeparator', 'columnQuote', 'columnSplitter', 'lineSeparator', 'escapeCharacter', 'nullString', 'emptyString', 'compressionAlgo', 'compressionLevel', 'dateFormat', 'defaultDate']
const parserBooleans = ['skipHeader', 'skipFooter', 'ignoreEscape', 'trailDelimiter', 'withTrim', 'compress']
const executionKeys = ['maxDiscards', 'maxErrors', 'strict', 'thread', 'rw', 'batch', 'blockSize', 'maxTps', 'maxWaitTimeout', 'memory', 'nlsDateFormat', 'nlsTimestampFormat', 'nlsTimestampTzFormat']
function parseConfig(raw: unknown): NormalImportConfig {
  const config = object(raw); known(config, configKeys)
  for (const [key, value] of Object.entries(config)) {
    if (['objects', 'parser', 'mapping', 'execution'].includes(key)) continue
    if (['compatMysql', 'replaceData', 'replaceObject'].includes(key)) { if (typeof value !== 'boolean') invalid() }
    else string(value)
  }
  for (const key of ['dataSourceId', 'nodeId', 'database', 'filePath', 'scope', 'content', 'cleanup']) string(config[key])
  if (!['', 'DDL_ONLY', 'DATA_ONLY', 'DDL_AND_DATA'].includes(string(config.content)) || !['', 'CSV', 'CUT', 'SQL'].includes(string(config.format ?? '')) || !['', 'ALL', 'SPECIFIC'].includes(string(config.scope)) || !['NONE', 'TRUNCATE', 'DELETE'].includes(string(config.cleanup))) invalid()
  const parser = object(config.parser); known(parser, [...parserStrings, ...parserBooleans])
  for (const [key, value] of Object.entries(parser)) { if (parserBooleans.includes(key) ? typeof value !== 'boolean' : typeof value !== 'string') invalid() }
  const execution = object(config.execution); known(execution, executionKeys); Object.values(execution).forEach(string)
  if (config.objects) { const objects = object(config.objects); known(objects, ['table', 'table-group', 'view', 'trigger', 'obj-user', 'role', 'sequence', 'synonym', 'type', 'type-body', 'package', 'package-body', 'function', 'procedure']); Object.values(objects).forEach(string) }
  if (config.mapping) {
    const mapping = object(config.mapping); known(mapping, ['kind', 'ctlPath', 'includeColumns', 'excludeColumns', 'excludeDataTypes'])
    if (!['POSITION', 'CONTROL', 'AUTO', 'EXPLICIT'].includes(string(mapping.kind))) invalid()
    Object.values(mapping).forEach(string)
  }
  return config as unknown as NormalImportConfig
}
function parseDraft(raw: ObjectValue): NormalImportDraft {
  if (raw.configVersion !== 'normal-import-v1' || raw.metadataVersion !== 'obloader-4.3.5-normal-v1' || raw.capabilityVersion !== 'import-odp-local-v1' || typeof raw.executionEnabled !== 'boolean') invalid()
  return { id: identifier(raw.id), revision: integer(raw.revision, 1), configVersion: raw.configVersion, config: parseConfig(raw.config), configFingerprint: fingerprint(raw.configFingerprint), executionEnabled: raw.executionEnabled }
}
function parseManifest(raw: unknown): NormalImportManifest | undefined {
  if (raw === null || raw === undefined) return undefined
  const value = object(raw)
  const fileCount = integer(value.fileCount, 1), totalBytes = integer(value.totalBytes)
  if (fileCount > 1024 || !Array.isArray(value.files) || value.files.length !== fileCount) invalid()
  const paths = new Set<string>()
  const files = value.files.map(raw => {
    const file = object(raw), path = string(file.path)
    if (!path || path.length > 512 || path.startsWith('/') || /[\\:]/.test(path) || Array.from(path).some(character => character.charCodeAt(0) < 32) || path.split('/').some(part => !part || part === '.' || part === '..') || paths.has(path) || !['DATA', 'DDL'].includes(string(file.kind))) invalid()
    paths.add(path)
    const modified = string(file.modified); if (!/^-?[0-9]{1,19}$/.test(modified)) invalid()
    return { path, kind: file.kind as 'DATA' | 'DDL', object: string(file.object), size: integer(file.size), modified, sha256: fingerprint(file.sha256) }
  })
  if (files.reduce((sum, file) => sum + file.size, 0) !== totalBytes) invalid()
  return { fileCount, totalBytes, digest: fingerprint(value.digest), files, ...(value.metadataDigest === undefined ? {} : { metadataDigest: fingerprint(value.metadataDigest) }) }
}
function parseCheck(value: ObjectValue): NormalImportCheck {
  const kind = value.checkKind
  if (kind !== 'NORMAL_IMPORT_FILES' && kind !== 'NORMAL_IMPORT_PREFLIGHT' || !['PENDING', 'LEASED', 'SUCCEEDED', 'FAILED', 'INVALIDATED', 'EXPIRED'].includes(string(value.status)) || typeof value.executionEnabled !== 'boolean') invalid()
  const checks = kind === 'NORMAL_IMPORT_FILES' ? ['INPUT_FILES'] : ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING']
  if (!Array.isArray(value.results) || value.results.length !== checks.length) invalid()
  const results = value.results.map((raw, index) => {
    const result = object(raw)
    if (result.check !== checks[index] || !['PASSED', 'FAILED', 'UNKNOWN'].includes(string(result.status))) invalid()
    if (!validEvidence(checks[index]!, string(result.status), string(result.evidenceCode))) invalid()
    return { check: checks[index]!, status: result.status as 'PASSED' | 'FAILED' | 'UNKNOWN', evidenceCode: string(result.evidenceCode) }
  })
  const inputManifest = parseManifest(value.inputManifest)
  if (value.status === 'SUCCEEDED' && (value.integrityStatus !== 'COMPLETE' || results.some(item => item.status !== 'PASSED') || !inputManifest)) invalid()
  const validUntil = string(value.validUntil); if (!Number.isFinite(Date.parse(validUntil))) invalid()
  return { id: identifier(value.id), draftId: identifier(value.draftId), draftRevision: integer(value.draftRevision, 1), configFingerprint: fingerprint(value.configFingerprint), checkKind: kind, status: value.status as NormalImportCheck['status'], integrityStatus: string(value.integrityStatus), validUntil, executionEnabled: value.executionEnabled, results, inputManifest }
}

function validEvidence(check: string, status: string, code: string): boolean {
  if (code === 'SYNTHETIC_OK') return true
  const common: Record<string, Record<string, readonly string[]>> = {
    DATABASE_CONNECTIVITY: { PASSED: ['DATABASE_CONNECTED'], FAILED: ['DATABASE_CONNECTION_FAILED'], UNKNOWN: ['DATABASE_CONNECTION_UNAVAILABLE', 'PRECHECK_BLOCKED_BY_TOOL', 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED'] },
    TOOL_ENVIRONMENT: { PASSED: ['TOOL_RUNTIME_READY'], FAILED: ['TOOL_RUNTIME_INVALID'], UNKNOWN: ['TOOL_RUNTIME_UNAVAILABLE'] },
    NODE_RESOURCE: { PASSED: ['NODE_RESOURCE_READY'], FAILED: ['NODE_RESOURCE_INSUFFICIENT', 'IMPORT_JVM_HEAP_TOO_SMALL'], UNKNOWN: ['NODE_RESOURCE_UNAVAILABLE'] },
    WRITE_ACCESS: { PASSED: ['IMPORT_VERIFIED'], FAILED: ['IMPORT_REJECTED', 'IMPORT_WRITE_PERMISSION_REJECTED', 'IMPORT_WRITE_INSERT_REQUIRED', 'IMPORT_WRITE_UPDATE_REQUIRED', 'IMPORT_WRITE_DELETE_REQUIRED', 'IMPORT_WRITE_CREATE_REQUIRED', 'IMPORT_WRITE_ALTER_REQUIRED', 'IMPORT_WRITE_DROP_REQUIRED'], UNKNOWN: ['IMPORT_UNAVAILABLE', 'PRECHECK_BLOCKED_BY_TOOL', 'PRECHECK_BLOCKED_BY_DATABASE', 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED', 'IMPORT_WRITE_PERMISSION_UNAVAILABLE', 'IMPORT_WRITE_GRANTS_UNAVAILABLE', 'IMPORT_WRITE_GRANTS_UNSUPPORTED', 'IMPORT_WRITE_DDL_UNSUPPORTED', 'IMPORT_WRITE_INPUT_UNAVAILABLE', 'IMPORT_WRITE_SCHEMA_UNAVAILABLE', 'IMPORT_WRITE_CLEANUP_SCOPE_UNAVAILABLE'] },
    INPUT_MAPPING: { PASSED: ['IMPORT_VERIFIED'], FAILED: ['IMPORT_REJECTED', 'IMPORT_INPUT_MAPPING_MISMATCH', 'IMPORT_COLUMN_COUNT_MISMATCH', 'IMPORT_COLUMN_NAME_MISMATCH', 'IMPORT_REQUIRED_COLUMN_MISSING'], UNKNOWN: ['IMPORT_UNAVAILABLE', 'PRECHECK_BLOCKED_BY_TOOL', 'PRECHECK_BLOCKED_BY_DATABASE', 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED', 'IMPORT_INPUT_MAPPING_UNAVAILABLE', 'IMPORT_INPUT_MAPPING_UNSUPPORTED', 'IMPORT_TARGET_METADATA_UNAVAILABLE', 'IMPORT_TARGET_STRUCTURE_MISSING', 'IMPORT_INPUT_DEFINITION_UNSUPPORTED', 'IMPORT_AUTO_COLUMNS_UNSUPPORTED', 'IMPORT_INPUT_FILES_CHANGED'] },
  }
  const codes = common[check] ?? { PASSED: ['IMPORT_VERIFIED'], FAILED: ['IMPORT_REJECTED'], UNKNOWN: ['IMPORT_UNAVAILABLE', 'PRECHECK_BLOCKED_BY_TOOL', 'PRECHECK_BLOCKED_BY_DATABASE', ...(check === 'WRITE_ACCESS' || check === 'INPUT_MAPPING' ? ['NORMAL_IMPORT_PROBE_NOT_CONFIGURED'] : [])] }
  return codes[status]?.includes(code) ?? false
}

export function createNormalImportApi(transport: Transport): NormalImportApi {
  const path = (draft: Pick<NormalImportDraft, 'id'>) => `${base}/${encodeURIComponent(draft.id)}`
  return {
    async createNormalImportDraft(config) { return parseDraft(await transport.write(base, { configVersion: 'normal-import-v1', config })) },
    async updateNormalImportDraft(draft, config) { return parseDraft(await transport.write(path(draft), { configVersion: 'normal-import-v1', config }, draft.revision, 'PATCH')) },
    async getNormalImportDraft(id) { return parseDraft(await transport.read(path({ id }))) },
    async listNormalImportDrafts() {
      const response = await transport.read(base); if (!Array.isArray(response.items) || response.items.length > 100) invalid()
      return response.items.map(raw => { const item = object(raw); return { id: identifier(item.id), revision: integer(item.revision, 1), updatedAt: string(item.updatedAt) } })
    },
    async deleteNormalImportDraft(draft) { const response = await transport.write(path(draft), {}, draft.revision, 'DELETE'); if (response.deleted !== true) invalid() },
    async previewNormalImportCommand(draft) {
      const value = await transport.write(`${path(draft)}:preview-command`, {}, draft.revision)
      const command = string(value.redactedCommand)
      if (!command.startsWith('obloader ') || /(?:--(?:sys-)?password|(?:^|\s)-p)(?:\s+|=)(?!["']?\*{3,}["']?(?:\s|$))\S+|\bpassword\s*[:=]\s*\S+/i.test(command) || typeof value.executionEnabled !== 'boolean') invalid()
      return { redactedCommand: command, configFingerprint: fingerprint(value.configFingerprint), executionEnabled: value.executionEnabled }
    },
    async startNormalImportCheck(draft, filesOnly) { return identifier((await transport.write(`${path(draft)}:${filesOnly ? 'discover-files' : 'precheck'}`, {}, draft.revision)).id) },
    async getNormalImportCheck(id) { return parseCheck(await transport.read(`/api/v1/normal-import-prechecks/${encodeURIComponent(id)}`)) },
    async submitNormalImport(draft, precheckId, confirmedConfigFingerprint, taskName) { return identifier((await transport.write(`${path(draft)}:submit`, { precheckId, ...(confirmedConfigFingerprint ? { confirmedConfigFingerprint } : {}), ...(taskName?.trim() ? { taskName: taskName.trim() } : {}) }, draft.revision)).id) },
  }
}
