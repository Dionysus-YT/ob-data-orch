export interface ApiError {
  readonly status: number
  readonly code: string
  readonly message: string
  readonly retryable: boolean
  readonly conflict: boolean
}

export interface DataSourceSummary {
  readonly id: string
  readonly displayName: string
  readonly environment: string
  readonly connectionKind: string
  readonly compatibilityMode: string
  readonly host: string
  readonly port: number
  readonly defaultDatabase?: string
  readonly state: string
  readonly revision: number
}

export interface DataSourceWrite {
  readonly displayName: string
  readonly environment: string
  readonly connectionKind: 'ODP'
  readonly compatibilityMode: 'MYSQL'
  readonly host: string
  readonly port: number
  readonly username: string
  readonly defaultDatabase?: string
  readonly password: string
}

export interface DataSourceConnectionTest {
  readonly status: 'SUCCEEDED' | 'FAILED' | 'PENDING' | 'UNAVAILABLE'
  readonly code: string
  readonly testedAt?: string
}

export interface ExportDraftInput {
  readonly dataSourceId: string
  readonly nodeId: string
  readonly database: string
  readonly table: string
  readonly format: 'CSV'
  readonly filePath: string
}

export interface ExportDraft {
  readonly id: string
  readonly dataSourceId: string
  readonly nodeId: string
  readonly revision: number
  readonly config: ExportDraftInput
  readonly configFingerprint: string
}

export interface CommandPreview {
  readonly command: string
  readonly configFingerprint: string
}

export interface Precheck {
  readonly id: string
  readonly draftId: string
  readonly draftRevision: number
  readonly configFingerprint: string
  readonly nodeId: string
  readonly status: string
  readonly integrityStatus: string
  readonly validUntil: string
}

export interface TaskDetail {
  readonly id: string
  readonly dataSourceId: string
  readonly nodeId: string
  readonly precheckId: string
  readonly configFingerprint: string
  readonly toolVersion: string
  readonly metadataVersion: string
  readonly capabilityVersion: string
  readonly plannedCommand: string
  readonly state: string
  readonly executionId?: string
  readonly submittedAt: string
  readonly realExecutionEnabled: false
}

export interface TaskLog {
  readonly sourceSeq: number
  readonly kind: string
  readonly message: string
  readonly integrityCode: string
  readonly receivedAt: string
}

export interface BrowserApi {
  listDataSources(): Promise<DataSourceSummary[]>
  createDataSource(input: DataSourceWrite): Promise<string>
  testDataSourceConnection(dataSourceId: string): Promise<DataSourceConnectionTest>
  createExportDraft(input: ExportDraftInput): Promise<string>
  getExportDraft(draftId: string): Promise<ExportDraft>
  updateExportDraft(draft: ExportDraft): Promise<ExportDraft>
  previewExportCommand(draft: ExportDraft): Promise<CommandPreview>
  startPrecheck(draft: ExportDraft): Promise<string>
  getPrecheck(precheckId: string): Promise<Precheck>
  submitExportDraft(draft: ExportDraft, precheckId: string): Promise<string>
  getTask(taskId: string): Promise<TaskDetail>
  getTaskLogs(taskId: string): Promise<TaskLog[]>
}

export type FetchLike = typeof fetch

interface BrowserApiOptions {
  readonly fetcher: FetchLike
  readonly csrfToken: () => string | undefined
  readonly idempotencyKey: () => string
}

const unsafeCommandPattern = /(?:--(?:user|password)(?:\s+|=)(?!["']?\*{3,}["']?(?:\s|$))|\b(?:username|password)\s*[:=]\s*\S+)/i
const unsafeLogPattern = /(?:--(?:user|password)(?:\s+|=)\S+|\b(?:username|password)\s*[:=]\s*\S+)/i

export function createBrowserApi(options: BrowserApiOptions): BrowserApi {
  return {
    async listDataSources() {
      const body = await request(options, '/api/v1/data-sources', { method: 'GET' })
      return listOf(body, 'items').map(parseDataSourceSummary)
    },
    async createDataSource(input) {
      const body = await request(options, '/api/v1/data-sources', writeRequest(options, input))
      return requiredString(body, 'id')
    },
    async testDataSourceConnection(dataSourceId) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}:test-connection`, writeRequest(options, {}, undefined, 'POST', false))
      const status = requiredString(body, 'status')
      if (status !== 'SUCCEEDED' && status !== 'FAILED' && status !== 'PENDING' && status !== 'UNAVAILABLE') {
        throw localError('RESPONSE_INVALID', '控制面返回了无效连接测试状态。')
      }
      return { status, code: requiredString(body, 'code'), testedAt: optionalString(body, 'testedAt') }
    },
    async createExportDraft(input) {
      const body = await request(options, '/api/v1/export-drafts', writeRequest(options, input))
      return requiredString(body, 'id')
    },
    async getExportDraft(draftId) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draftId)}`, { method: 'GET' })
      return parseExportDraft(requiredObject(body, 'item'))
    },
    async updateExportDraft(draft) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draft.id)}`, writeRequest(options, draft.config, draft.revision, 'PATCH'))
      return parseExportDraft(requiredObject(body, 'item'))
    },
    async previewExportCommand(draft) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draft.id)}:preview-command`, writeRequest(options, {}, draft.revision))
      const command = requiredString(body, 'command')
      if (unsafeCommandPattern.test(command)) {
        throw localError('UNSAFE_COMMAND_RESPONSE', '命令预览未通过本地脱敏校验，已拒绝展示。')
      }
      return { command, configFingerprint: requiredString(body, 'configFingerprint') }
    },
    async startPrecheck(draft) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draft.id)}:precheck`, writeRequest(options, {}, draft.revision, 'POST', true))
      return requiredString(body, 'id')
    },
    async getPrecheck(precheckId) {
      const body = await request(options, `/api/v1/prechecks/${encodeURIComponent(precheckId)}`, { method: 'GET' })
      return parsePrecheck(requiredObject(body, 'item'))
    },
    async submitExportDraft(draft, precheckId) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draft.id)}:submit`, writeRequest(options, { precheckId }, draft.revision, 'POST', true))
      return requiredString(body, 'id')
    },
    async getTask(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}`, { method: 'GET' })
      return parseTaskDetail(requiredObject(body, 'item'))
    },
    async getTaskLogs(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}/logs`, { method: 'GET' })
      return listOf(body, 'items').map(parseTaskLog)
    },
  }
}

export function browserApi(): BrowserApi {
  return createBrowserApi({
    fetcher: fetch,
    csrfToken: csrfTokenFromDocument,
    idempotencyKey: newIdempotencyKey,
  })
}

function writeRequest(options: BrowserApiOptions, body: unknown, revision?: number, method = 'POST', includeIdempotency = revision === undefined): RequestInit {
  const csrfToken = options.csrfToken()
  if (!csrfToken) {
    throw localError('CSRF_TOKEN_UNAVAILABLE', '当前环境未提供浏览器 CSRF 令牌，已拒绝写操作。')
  }
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-CSRF-Token': csrfToken,
  }
  if (revision !== undefined) {
    headers['If-Match'] = `"rev-${revision}"`
  }
  if (includeIdempotency) {
    headers['Idempotency-Key'] = options.idempotencyKey()
  }
  return { method, headers, body: JSON.stringify(body) }
}

async function request(options: BrowserApiOptions, path: string, init: RequestInit): Promise<Record<string, unknown>> {
  let response: Response
  try {
    response = await options.fetcher(path, { ...init, credentials: 'same-origin', cache: 'no-store' })
  } catch {
    throw localError('NETWORK_UNAVAILABLE', '无法连接控制面，请检查当前环境后重试。', true)
  }
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    throw toApiError(response.status, asRecord(body))
  }
  return asRecord(body)
}

function parseDataSourceSummary(value: unknown): DataSourceSummary {
  const source = asRecord(value)
  return {
    id: requiredString(source, 'id'),
    displayName: requiredString(source, 'displayName'),
    environment: requiredString(source, 'environment'),
    connectionKind: requiredString(source, 'connectionKind'),
    compatibilityMode: requiredString(source, 'compatibilityMode'),
    host: requiredString(source, 'host'),
    port: requiredNumber(source, 'port'),
    defaultDatabase: optionalString(source, 'defaultDatabase'),
    state: requiredString(source, 'state'),
    revision: requiredNumber(source, 'revision'),
  }
}

function parseExportDraft(value: Record<string, unknown>): ExportDraft {
  return {
    id: requiredString(value, 'id'),
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    revision: requiredNumber(value, 'revision'),
    config: parseDraftInput(requiredObject(value, 'config')),
    configFingerprint: requiredString(value, 'configFingerprint'),
  }
}

function parseDraftInput(value: Record<string, unknown>): ExportDraftInput {
  const format = requiredString(value, 'format')
  if (format !== 'CSV') {
    throw localError('UNSUPPORTED_DRAFT_FORMAT', '当前页面只支持 CSV 草稿。')
  }
  return {
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    database: requiredString(value, 'database'),
    table: requiredString(value, 'table'),
    format,
    filePath: requiredString(value, 'filePath'),
  }
}

function parsePrecheck(value: Record<string, unknown>): Precheck {
  return {
    id: requiredString(value, 'id'),
    draftId: requiredString(value, 'draftId'),
    draftRevision: requiredNumber(value, 'draftRevision'),
    configFingerprint: requiredString(value, 'configFingerprint'),
    nodeId: requiredString(value, 'nodeId'),
    status: requiredString(value, 'status'),
    integrityStatus: requiredString(value, 'integrityStatus'),
    validUntil: requiredString(value, 'validUntil'),
  }
}

function parseTaskDetail(value: Record<string, unknown>): TaskDetail {
  const plannedCommand = requiredString(value, 'plannedCommand')
  if (unsafeCommandPattern.test(plannedCommand)) {
    throw localError('UNSAFE_COMMAND_RESPONSE', '任务命令未通过本地脱敏校验，已拒绝展示。')
  }
  if (value.realExecutionEnabled !== false) {
    throw localError('REAL_EXECUTION_STATE_INVALID', '控制面未声明真实执行关闭，已拒绝展示任务状态。')
  }
  return {
    id: requiredString(value, 'id'),
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    precheckId: requiredString(value, 'precheckId'),
    configFingerprint: requiredString(value, 'configFingerprint'),
    toolVersion: requiredString(value, 'toolVersion'),
    metadataVersion: requiredString(value, 'metadataVersion'),
    capabilityVersion: requiredString(value, 'capabilityVersion'),
    plannedCommand,
    state: requiredString(value, 'state'),
    executionId: optionalString(value, 'executionId'),
    submittedAt: requiredString(value, 'submittedAt'),
    realExecutionEnabled: false,
  }
}

function parseTaskLog(value: unknown): TaskLog {
  const record = asRecord(value)
  const message = requiredString(record, 'message')
  return {
    sourceSeq: requiredNumber(record, 'sourceSeq'),
    kind: requiredString(record, 'kind'),
    message: unsafeLogPattern.test(message) ? '日志因本地安全策略被隐藏。' : message,
    integrityCode: requiredString(record, 'integrityCode'),
    receivedAt: requiredString(record, 'receivedAt'),
  }
}

function csrfTokenFromDocument(): string | undefined {
  const value = document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.getAttribute('content')?.trim()
  return value || undefined
}

function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

function toApiError(status: number, value: Record<string, unknown>): ApiError {
  const code = optionalString(value, 'code') ?? 'REQUEST_FAILED'
  const message = optionalString(value, 'message') ?? '请求未能完成。'
  return { status, code, message, retryable: value.retryable === true, conflict: status === 409 || status === 412 }
}

function localError(code: string, message: string, retryable = false): ApiError {
  return { status: 0, code, message, retryable, conflict: false }
}

function asRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效响应。')
  }
  return value as Record<string, unknown>
}

function listOf(value: Record<string, unknown>, key: string): unknown[] {
  const list = value[key]
  if (!Array.isArray(list)) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效集合。')
  }
  return list
}

function requiredObject(value: Record<string, unknown>, key: string): Record<string, unknown> {
  return asRecord(value[key])
}

function requiredString(value: Record<string, unknown>, key: string): string {
  const item = value[key]
  if (typeof item !== 'string' || !item.trim()) {
    throw localError('RESPONSE_INVALID', '控制面返回了缺失字段。')
  }
  return item
}

function optionalString(value: Record<string, unknown>, key: string): string | undefined {
  const item = value[key]
  return typeof item === 'string' && item.trim() ? item : undefined
}

function requiredNumber(value: Record<string, unknown>, key: string): number {
  const item = value[key]
  if (typeof item !== 'number' || !Number.isFinite(item)) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效数值。')
  }
  return item
}
