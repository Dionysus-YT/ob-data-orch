export interface ApiError {
  readonly status: number
  readonly code: string
  readonly message: string
  readonly retryable: boolean
  readonly conflict: boolean
  readonly fieldErrors: readonly ApiFieldError[]
}

export interface ApiFieldError {
  readonly field: string
  readonly code: string
  readonly message?: string
}

export function dataSourceErrorMessage(error: unknown, fallback: string): string {
  const apiError = error as Partial<ApiError>
  if (apiError.code === 'CSRF_TOKEN_UNAVAILABLE') return '当前页面未获得请求安全令牌，已拒绝写操作。请刷新页面后重试。'
  if (apiError.code === 'DATA_SOURCE_NAME_UNAVAILABLE') return '数据源名称不可用，请更换后重试。'
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '数据源不存在或当前身份无权访问。'
  if (apiError.status === 409 || apiError.status === 412 || apiError.conflict) return '数据源已发生变化，请刷新后重新比较。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查当前环境后重试。'
  return apiError.message || fallback
}

export function exportDraftErrorMessage(error: unknown, fallback: string): string {
  const apiError = error as Partial<ApiError>
  if (apiError.code === 'CSRF_TOKEN_UNAVAILABLE') return '当前页面未获得请求安全令牌，已拒绝创建草稿。请刷新页面后重试。'
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '所选数据源或执行节点不存在，或当前身份无权使用。'
  if (apiError.status === 409 || apiError.status === 412 || apiError.conflict) return '草稿配置已发生变化，请刷新后重新比较。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查当前环境后重试。'
  return apiError.message || fallback
}

// taskDetailErrorMessage 为任务详情读取失败保留安全错误语义，避免把响应契约错误误报为网络中断。
export function taskDetailErrorMessage(error: unknown, fallback: string): string {
  const apiError = error as Partial<ApiError>
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '任务不存在或当前身份无权访问。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查当前环境后重试。'
  return apiError.message || fallback
}

export function executionNodeErrorMessage(error: unknown, fallback: string): string {
  const apiError = error as Partial<ApiError>
  if (apiError.code === 'CSRF_TOKEN_UNAVAILABLE') return '当前页面未获得请求安全令牌，已拒绝写操作。请刷新页面后重试。'
  if (apiError.code === 'EXECUTION_NODE_NAME_UNAVAILABLE') return '执行节点名称不可用，请更换后重试。'
  if (apiError.code === 'EXECUTION_NODE_RUNNING_TASK') return '节点仍有运行任务，暂不能删除或归档。'
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '执行节点不存在或当前身份无权访问。'
  if (apiError.status === 409 || apiError.status === 412 || apiError.conflict) return '执行节点已发生变化，请刷新后重新比较。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查当前环境后重试。'
  return apiError.message || fallback
}

export interface DataSourceSummary {
  readonly id: string
  readonly displayName: string
  readonly environment: string
  readonly connectionKind: string
  readonly compatibilityMode: string
  readonly host: string
  readonly port: number
  readonly clusterName: string
  readonly tenantName: string
  readonly defaultDatabase?: string
  readonly state: string
  readonly revision: number
  readonly credentialRevision: number
  readonly lastTestStatus?: string
  readonly lastTestedAt?: string
}

export interface DataSourceWrite {
  readonly displayName: string
  readonly environment: 'DEVELOPMENT' | 'TEST' | 'STAGING' | 'PRODUCTION'
  readonly connectionKind: 'ODP'
  readonly compatibilityMode: 'MYSQL' | 'ORACLE'
  readonly host: string
  readonly port: number
  readonly clusterName: string
  readonly tenantName: string
  readonly username: string
  readonly defaultDatabase?: string
  readonly password: string
}

export interface DataSourceUpdate {
  readonly displayName?: string
  readonly environment?: 'DEVELOPMENT' | 'TEST' | 'STAGING' | 'PRODUCTION'
  readonly connectionKind?: 'ODP'
  readonly compatibilityMode?: 'MYSQL' | 'ORACLE'
  readonly host?: string
  readonly port?: number
  readonly clusterName?: string
  readonly tenantName?: string
  readonly username?: string
  readonly defaultDatabase?: string
  readonly password?: string
}

export type DataSourceConnectionTestStatus = 'PENDING' | 'LEASED' | 'SUCCEEDED' | 'FAILED' | 'UNKNOWN' | 'EXPIRED' | 'INVALIDATED'

export type DataSourceConnectionTestVerificationSource = 'G2_SYNTHETIC' | 'AGENT_JDBC'

export interface DataSourceConnectionTestRequest {
  readonly id: string
  readonly status: DataSourceConnectionTestStatus
  readonly nodeId: string
}

export interface DataSourceConnectionTest {
  readonly id: string
  readonly status: DataSourceConnectionTestStatus
  readonly nodeId: string
  readonly nodeDisplayName?: string
  readonly agentId?: string
  readonly factsRevision?: number
  readonly resultCode?: string
  readonly completedAt?: string
  readonly verificationSource: DataSourceConnectionTestVerificationSource
  readonly realConnectionVerified: boolean
}

export interface DataSourceStateChange {
  readonly state: 'ENABLED' | 'DISABLED'
  readonly revision: number
}

export interface DataSourceDeletionResult {
	readonly outcome: 'DELETED' | 'ARCHIVED'
	readonly revision: number
}

export interface ExecutionNodeDeletionResult {
  readonly outcome: 'DELETED' | 'ARCHIVED'
  readonly revision: number
  readonly agentAccessRevoked: boolean
}

export interface ExecutionNodeCandidate {
  readonly id: string
  readonly displayName: string
  readonly platform: ExecutionNodePlatform
}

export type ExecutionNodePlatform = 'WINDOWS_AMD64' | 'LINUX_AMD64' | 'LINUX_ARM64'

export type ExecutionNodeAgentAssociationStatus = 'PENDING' | 'ASSOCIATED'

export type ExecutionNodeHeartbeatStatus = 'NEVER_CONNECTED' | 'ONLINE' | 'OFFLINE'

export type ExecutionNodeEnvironmentStatus = 'NOT_CHECKED' | 'NORMAL' | 'ABNORMAL' | 'EXPIRED'

export type ExecutionNodeCapacityStatus = 'UNKNOWN' | 'AVAILABLE' | 'BUSY'

export interface ExecutionNodeAgentFacts {
  readonly os: string
  readonly arch: string
  readonly agentVersion: string
  readonly bootId: string
  readonly observedAt: string
  readonly capacityTotal: number
  readonly capacityUsed: number
  readonly cpuUsagePercent?: number
  readonly memoryUsagePercent?: number
}

export interface ExecutionNodeDataRootUsage {
  readonly root: string
  readonly totalBytes: number
  readonly availableBytes: number
}

export interface ExecutionNodeWrite {
  readonly displayName: string
  readonly platform: ExecutionNodePlatform
  readonly allowedRoots: readonly string[]
  readonly toolHome: string
  readonly javaPath: string
}

export interface ExecutionNodeSummary {
  readonly id: string
  readonly displayName: string
  readonly platform: ExecutionNodePlatform
  readonly managementState: 'ENABLED' | 'DISABLED' | 'MAINTENANCE'
  readonly agentAssociationStatus: ExecutionNodeAgentAssociationStatus
  readonly heartbeatStatus: ExecutionNodeHeartbeatStatus
  readonly lastHeartbeatAt: string | null
  readonly environmentStatus: ExecutionNodeEnvironmentStatus
  readonly capacityStatus: ExecutionNodeCapacityStatus
  readonly acceptsNewTasks: boolean
  readonly unavailableReasons: readonly string[]
  readonly agentFacts?: ExecutionNodeAgentFacts
  readonly revision: number
  readonly updatedAt: string
}

export interface ExecutionNodeDetail extends ExecutionNodeSummary {
  readonly allowedRoots: readonly string[]
  readonly toolHome: string
  readonly javaPath: string
  readonly dataRootUsages?: readonly ExecutionNodeDataRootUsage[]
  readonly createdAt: string
}

export interface ExecutionNodeEnrollment {
  readonly requestId: string
  readonly enrollmentId: string
  readonly nodeId: string
  readonly enrollmentMaterial: string
  readonly expiresAt: string
  readonly displayedOnce: true
}

export interface ExportDraftInput {
  readonly dataSourceId: string
  readonly nodeId: string
  readonly database: string
  readonly table: string
  readonly format: 'CSV'
  readonly filePath: string
  readonly logPath?: string
  readonly skipCheckDir?: boolean
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
	readonly results: readonly PrecheckResult[]
  readonly validUntil: string
}

export interface PrecheckResult {
	readonly check: 'DATABASE_CONNECTIVITY' | 'OBJECT_ACCESS' | 'TOOL_ENVIRONMENT' | 'OUTPUT_PATH' | 'OUTPUT_EMPTY' | 'AVAILABLE_SPACE'
	readonly status: 'PASSED' | 'FAILED' | 'UNKNOWN'
	readonly evidenceCode: string
}

export interface TaskOverview {
  readonly id: string
  readonly type: 'OBDUMPER_EXPORT'
  readonly dataSourceId: string
  readonly nodeId: string
  readonly precheckId: string
  readonly submittedAt: string
}

export interface TaskSnapshot {
  readonly type: 'OBDUMPER_EXPORT'
  readonly dataSourceId: string
  readonly nodeId: string
  readonly precheckId: string
  readonly objectSummary?: string
  readonly format: 'CSV'
  readonly configFingerprint: string
  readonly toolVersion: string
  readonly metadataVersion: string
  readonly capabilityVersion: string
}

export interface TaskCommandEvidence {
  readonly kind: 'PLANNED'
  readonly command: string
  readonly redaction: 'PASSWORD_ONLY'
}

export interface TaskExecution {
  readonly state: string
  readonly executionId?: string
  readonly reconciliationRequired: boolean
  readonly stageEvidence: 'UNAVAILABLE'
  readonly progressEvidence: 'UNAVAILABLE'
  readonly startedAt?: string
  readonly finishedAt?: string
  readonly updatedAt: string
}

export interface TaskListItem {
  readonly id: string
  readonly type: 'OBDUMPER_EXPORT'
  readonly dataSourceId: string
  readonly objectSummary?: string
  readonly state: string
  readonly stageEvidence: 'UNAVAILABLE'
  readonly progressEvidence: 'UNAVAILABLE'
  readonly reconciliationRequired: boolean
  readonly nodeId: string
  readonly ownedByCurrentUser: boolean
  readonly submittedAt: string
  readonly startedAt?: string
  readonly finishedAt?: string
  readonly updatedAt: string
}

export interface TaskListPage {
  readonly items: readonly TaskListItem[]
  readonly nextCursor?: string
  readonly totalPages: number
}

export const taskListPageSizes = [10, 20, 50] as const
export type TaskListPageSize = (typeof taskListPageSizes)[number]

export interface TaskLog {
  readonly sourceSeq: number
  readonly kind: string
  readonly message: string
  readonly integrityCode: string
  readonly receivedAt: string
}

export interface TaskLogPage {
  readonly items: readonly TaskLog[]
  readonly nextCursor?: string
  readonly lastReliableCursor?: string
  readonly integrity: string
}

export interface TaskLogStream {
  close(): void
}

export interface BrowserApi {
  listDataSources(): Promise<DataSourceSummary[]>
  listExecutionNodes(): Promise<ExecutionNodeSummary[]>
  listExportNodeCandidates(): Promise<ExecutionNodeCandidate[]>
  listDataSourceConnectionTestNodeCandidates(): Promise<ExecutionNodeCandidate[]>
  getExecutionNode(nodeId: string): Promise<ExecutionNodeDetail>
  createExecutionNode(input: ExecutionNodeWrite): Promise<string>
  updateExecutionNode(nodeId: string, revision: number, input: ExecutionNodeWrite): Promise<ExecutionNodeDetail>
  issueExecutionNodeEnrollment(nodeId: string): Promise<ExecutionNodeEnrollment>
  requestExecutionNodeEnvironmentCheck(nodeId: string, revision: number): Promise<ExecutionNodeDetail>
  enableExecutionNode(nodeId: string, revision: number): Promise<ExecutionNodeDetail>
  deleteOrArchiveExecutionNode(nodeId: string, revision: number): Promise<ExecutionNodeDeletionResult>
  getDataSource(dataSourceId: string): Promise<DataSourceSummary>
  createDataSource(input: DataSourceWrite): Promise<string>
  updateDataSource(dataSourceId: string, revision: number, input: DataSourceUpdate): Promise<DataSourceSummary>
  changeDataSourceState(dataSourceId: string, revision: number, targetState: 'ENABLED' | 'DISABLED'): Promise<DataSourceStateChange>
  deleteOrArchiveDataSource(dataSourceId: string, revision: number): Promise<DataSourceDeletionResult>
  startDataSourceConnectionTest(dataSourceId: string, revision: number, nodeId: string): Promise<DataSourceConnectionTestRequest>
  getDataSourceConnectionTest(connectionTestId: string): Promise<DataSourceConnectionTest>
  createExportDraft(input: ExportDraftInput): Promise<string>
  getExportDraft(draftId: string): Promise<ExportDraft>
  updateExportDraft(draft: ExportDraft): Promise<ExportDraft>
  previewExportCommand(draft: ExportDraft): Promise<CommandPreview>
  startPrecheck(draft: ExportDraft): Promise<string>
  getPrecheck(precheckId: string): Promise<Precheck>
  submitExportDraft(draft: ExportDraft, precheckId: string): Promise<string>
  listTasks(cursor?: string, limit?: TaskListPageSize): Promise<TaskListPage>
  getTaskOverview(taskId: string): Promise<TaskOverview>
  getTaskSnapshot(taskId: string): Promise<TaskSnapshot>
  getTaskCommandEvidence(taskId: string): Promise<TaskCommandEvidence>
  getTaskExecution(taskId: string): Promise<TaskExecution>
  getTaskLogs(taskId: string, cursor?: string, after?: string): Promise<TaskLogPage>
  streamTaskLogs(taskId: string, after: string | undefined, onRecord: (record: TaskLog, cursor: string | undefined) => void, onDisconnected: () => void): TaskLogStream
}

export type FetchLike = typeof fetch

interface BrowserApiOptions {
  readonly fetcher: FetchLike
  readonly csrfToken: () => string | undefined
  readonly idempotencyKey: () => string
}

const unsafeCommandPattern = /(?:--password|(?:^|\s)-p)(?:\s+|=)(?!["']?\*{3,}["']?(?:\s|$))\S+|\bpassword\s*[:=]\s*\S+/i
const unsafeLogPattern = /(?:--(?:user|password)(?:\s+|=)\S+|\b(?:username|password)\s*[:=]\s*\S+)/i

export function createBrowserApi(options: BrowserApiOptions): BrowserApi {
  return {
    async listDataSources() {
      const body = await request(options, '/api/v1/data-sources', { method: 'GET' })
      return listOf(body, 'items').map(parseDataSourceSummary)
    },
    async listExecutionNodes() {
      const body = await request(options, '/api/v1/execution-nodes', { method: 'GET' })
      return listOf(body, 'items').map(parseExecutionNodeSummary)
    },
    async listExportNodeCandidates() {
      const body = await request(options, '/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT', { method: 'GET' })
      return listOf(body, 'items').map(parseExecutionNodeCandidate)
    },
    async listDataSourceConnectionTestNodeCandidates() {
      const body = await request(options, '/api/v1/execution-nodes?eligibleFor=DATA_SOURCE_CONNECTION_TEST', { method: 'GET' })
      return listOf(body, 'items').map(parseExecutionNodeCandidate)
    },
    async getExecutionNode(nodeId) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}`, { method: 'GET' })
      return parseExecutionNodeDetail(requiredObject(body, 'item'))
    },
    async createExecutionNode(input) {
      const body = await request(options, '/api/v1/execution-nodes', writeRequest(options, input))
      return requiredString(body, 'id')
    },
    async updateExecutionNode(nodeId, revision, input) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}`, writeRequest(options, input, revision, 'PATCH', false))
      return parseExecutionNodeDetail(requiredObject(body, 'item'))
    },
    async issueExecutionNodeEnrollment(nodeId) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:enrollments`, writeEmptyPost(options))
      const enrollment = parseExecutionNodeEnrollment(body)
      if (enrollment.nodeId !== nodeId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与当前节点不一致的关联材料。')
      }
      return enrollment
    },
    async requestExecutionNodeEnvironmentCheck(nodeId, revision) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:environment-check`, writeVersionedEmptyPost(options, revision))
      return parseExecutionNodeDetail(requiredObject(body, 'item'))
    },
    async enableExecutionNode(nodeId, revision) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:enable`, writeVersionedEmptyPost(options, revision))
      return parseExecutionNodeDetail(requiredObject(body, 'item'))
    },
    async deleteOrArchiveExecutionNode(nodeId, revision) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}`, writeRequest(options, {}, revision, 'DELETE', false))
      const outcome = requiredString(body, 'outcome')
      if (outcome !== 'DELETED' && outcome !== 'ARCHIVED') {
        throw localError('RESPONSE_INVALID', '控制面返回了无效执行节点删除结果。')
      }
      return { outcome, revision: requiredNumber(body, 'revision'), agentAccessRevoked: requiredBoolean(body, 'agentAccessRevoked') }
    },
    async getDataSource(dataSourceId) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}`, { method: 'GET' })
      return parseDataSourceSummary(requiredObject(body, 'item'))
    },
    async createDataSource(input) {
      const body = await request(options, '/api/v1/data-sources', writeRequest(options, input))
      return requiredString(body, 'id')
    },
    async updateDataSource(dataSourceId, revision, input) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}`, writeRequest(options, input, revision, 'PATCH', false))
      return parseDataSourceSummary(requiredObject(body, 'item'))
    },
    async changeDataSourceState(dataSourceId, revision, targetState) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}:${targetState === 'ENABLED' ? 'enable' : 'disable'}`, writeRequest(options, {}, revision, 'POST', false))
      const state = requiredString(body, 'state')
      if (state !== 'ENABLED' && state !== 'DISABLED') {
        throw localError('RESPONSE_INVALID', '控制面返回了无效数据源状态。')
      }
      return { state, revision: requiredNumber(body, 'revision') }
    },
    async deleteOrArchiveDataSource(dataSourceId, revision) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}`, writeRequest(options, {}, revision, 'DELETE', false))
      const outcome = requiredString(body, 'outcome')
      if (outcome !== 'DELETED' && outcome !== 'ARCHIVED') {
        throw localError('RESPONSE_INVALID', '控制面返回了无效数据源删除结果。')
      }
      return { outcome, revision: requiredNumber(body, 'revision') }
    },
    async startDataSourceConnectionTest(dataSourceId, revision, nodeId) {
      if (!nodeId.trim()) {
        throw localError('DATA_SOURCE_CONNECTION_TEST_NODE_REQUIRED', '请选择执行节点后再测试连接。')
      }
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}:test-connection`, writeRequest(options, { nodeId }, revision, 'POST', true))
      const connectionTest = parseDataSourceConnectionTestRequest(requiredObject(body, 'item'))
      if (connectionTest.nodeId !== nodeId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与所选节点不一致的连接测试。')
      }
      return connectionTest
    },
    async getDataSourceConnectionTest(connectionTestId) {
      const body = await request(options, `/api/v1/data-source-connection-tests/${encodeURIComponent(connectionTestId)}`, { method: 'GET' })
      return parseDataSourceConnectionTest(requiredObject(body, 'item'))
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
        throw localError('UNSAFE_COMMAND_RESPONSE', '命令预览包含未隐藏的密码，已拒绝展示。')
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
    async listTasks(cursor, limit = 10) {
      const query = new URLSearchParams({ limit: String(limit) })
      if (cursor) query.set('cursor', cursor)
      const body = await request(options, `/api/v1/tasks?${query.toString()}`, { method: 'GET' })
      const totalPages = requiredNumber(body, 'totalPages')
      if (!Number.isSafeInteger(totalPages) || totalPages < 0) {
        throw localError('RESPONSE_INVALID', '控制面返回了无效任务总页数。')
      }
      return { items: listOf(body, 'items').map(parseTaskListItem), nextCursor: optionalString(body, 'nextCursor'), totalPages }
    },
    async getTaskOverview(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}`, { method: 'GET' })
      return parseTaskOverview(requiredObject(body, 'item'))
    },
    async getTaskSnapshot(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}/snapshot`, { method: 'GET' })
      return parseTaskSnapshot(requiredObject(body, 'item'))
    },
    async getTaskCommandEvidence(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}/command-evidence`, { method: 'GET' })
      return parseTaskCommandEvidence(requiredObject(body, 'item'))
    },
    async getTaskExecution(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}/execution`, { method: 'GET' })
      return parseTaskExecution(requiredObject(body, 'item'))
    },
    async getTaskLogs(taskId, cursor, after) {
      if (cursor && after) throw localError('CURSOR_INVALID', '日志游标无效。')
      const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : after ? `?after=${encodeURIComponent(after)}` : ''
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}/logs${query}`, { method: 'GET' })
      return {
        items: listOf(body, 'items').map(parseTaskLog),
        nextCursor: optionalString(body, 'nextCursor'),
        lastReliableCursor: optionalString(body, 'lastReliableCursor'),
        integrity: optionalString(body, 'integrity') ?? 'UNKNOWN',
      }
    },
    streamTaskLogs(taskId, after, onRecord, onDisconnected) {
      if (typeof EventSource === 'undefined') throw localError('LOG_STREAM_UNAVAILABLE', '当前浏览器不支持实时日志连接。')
      const query = after ? `?after=${encodeURIComponent(after)}` : ''
      const stream = new EventSource(`/api/v1/tasks/${encodeURIComponent(taskId)}/logs/stream${query}`)
      stream.addEventListener('log', (event) => {
        if (!(event instanceof MessageEvent)) return
        try {
          onRecord(parseTaskLog(JSON.parse(event.data)), event.lastEventId || undefined)
        } catch {
          onDisconnected()
        }
      })
      stream.onerror = () => { onDisconnected() }
      return { close: () => stream.close() }
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

// writeEmptyPost 保持关联材料签发的无请求体契约，同时保留 CSRF 和幂等保护。
function writeEmptyPost(options: BrowserApiOptions): RequestInit {
  const csrfToken = options.csrfToken()
  if (!csrfToken) {
    throw localError('CSRF_TOKEN_UNAVAILABLE', '当前环境未提供浏览器 CSRF 令牌，已拒绝写操作。')
  }
  return {
    method: 'POST',
    headers: {
      'X-CSRF-Token': csrfToken,
      'Idempotency-Key': options.idempotencyKey(),
    },
  }
}

// writeVersionedEmptyPost 用于不接收请求体的节点状态动作。
// 版本前置条件让环境检查或启用不会覆盖页面已经过时的节点事实。
function writeVersionedEmptyPost(options: BrowserApiOptions, revision: number): RequestInit {
  const csrfToken = options.csrfToken()
  if (!csrfToken) {
    throw localError('CSRF_TOKEN_UNAVAILABLE', '当前环境未提供浏览器 CSRF 令牌，已拒绝写操作。')
  }
  return {
    method: 'POST',
    headers: {
      'X-CSRF-Token': csrfToken,
      'If-Match': `"rev-${revision}"`,
    },
  }
}

async function request(options: BrowserApiOptions, path: string, init: RequestInit): Promise<Record<string, unknown>> {
  let response: Response
  try {
    const fetcher = options.fetcher
    response = await fetcher(path, { ...init, credentials: 'same-origin', cache: 'no-store' })
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
    clusterName: requiredString(source, 'clusterName'),
    tenantName: requiredString(source, 'tenantName'),
    defaultDatabase: optionalString(source, 'defaultDatabase'),
    state: requiredString(source, 'state'),
    revision: requiredNumber(source, 'revision'),
    credentialRevision: requiredNumber(source, 'credentialRevision'),
    lastTestStatus: optionalString(source, 'lastTestStatus'),
    lastTestedAt: optionalString(source, 'lastTestedAt'),
  }
}

function parseDataSourceConnectionTestRequest(value: Record<string, unknown>): DataSourceConnectionTestRequest {
  return {
    id: requiredString(value, 'id'),
    status: parseDataSourceConnectionTestStatus(value),
    nodeId: requiredString(value, 'nodeId'),
  }
}

function parseDataSourceConnectionTest(value: Record<string, unknown>): DataSourceConnectionTest {
  const status = parseDataSourceConnectionTestStatus(value)
  const verificationSource = parseDataSourceConnectionTestVerificationSource(value)
  const realConnectionVerified = requiredBoolean(value, 'realConnectionVerified')
  const resultCode = optionalString(value, 'resultCode') ?? optionalString(value, 'code')
  const completedAt = optionalString(value, 'completedAt')
  if (isTerminalDataSourceConnectionTestStatus(status) && (!resultCode || !completedAt)) {
    throw localError('RESPONSE_INVALID', '控制面返回的连接测试终态缺少结果证据。')
  }
  if (realConnectionVerified !== (verificationSource === 'AGENT_JDBC' && status === 'SUCCEEDED')) {
    throw localError('RESPONSE_INVALID', '控制面返回的连接测试验证标记不一致。')
  }
  return {
    id: requiredString(value, 'id'),
    status,
    nodeId: requiredString(value, 'nodeId'),
    nodeDisplayName: optionalString(value, 'nodeDisplayName'),
    agentId: optionalString(value, 'agentId'),
    factsRevision: optionalPositiveInteger(value, 'factsRevision') ?? optionalPositiveInteger(value, 'nodeFactsRevision'),
    resultCode,
    completedAt,
    verificationSource,
    realConnectionVerified,
  }
}

function parseDataSourceConnectionTestStatus(value: Record<string, unknown>): DataSourceConnectionTestStatus {
  const status = requiredString(value, 'status')
  if (status !== 'PENDING' && status !== 'LEASED' && status !== 'SUCCEEDED' && status !== 'FAILED' && status !== 'UNKNOWN' && status !== 'EXPIRED' && status !== 'INVALIDATED') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效连接测试状态。')
  }
  return status
}

function isTerminalDataSourceConnectionTestStatus(status: DataSourceConnectionTestStatus): boolean {
  return status === 'SUCCEEDED' || status === 'FAILED' || status === 'UNKNOWN' || status === 'EXPIRED' || status === 'INVALIDATED'
}

function parseDataSourceConnectionTestVerificationSource(value: Record<string, unknown>): DataSourceConnectionTestVerificationSource {
  const source = requiredString(value, 'verificationSource')
  if (source !== 'G2_SYNTHETIC' && source !== 'AGENT_JDBC') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效连接测试来源。')
  }
  return source
}

function parseExecutionNodeCandidate(value: unknown): ExecutionNodeCandidate {
  const node = asRecord(value)
  const platform = parseExecutionNodePlatform(node)
  return { id: requiredString(node, 'id'), displayName: requiredString(node, 'displayName'), platform }
}

function parseExecutionNodeSummary(value: unknown): ExecutionNodeSummary {
  const node = asRecord(value)
  const managementState = requiredString(node, 'managementState')
  const agentAssociationStatus = parseExecutionNodeAgentAssociationStatus(node)
  const heartbeatStatus = parseExecutionNodeHeartbeatStatus(node)
  const lastHeartbeatAt = nullableString(node, 'lastHeartbeatAt')
  const environmentStatus = parseExecutionNodeEnvironmentStatus(node)
  const capacityStatus = parseExecutionNodeCapacityStatus(node)
  const acceptsNewTasks = node.acceptsNewTasks
  const unavailableReasons = requiredStringList(node, 'unavailableReasons')
  const agentFacts = parseExecutionNodeAgentFacts(node.agentFacts)
  if (managementState !== 'ENABLED' && managementState !== 'DISABLED' && managementState !== 'MAINTENANCE') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点管理状态。')
  }
  if (typeof acceptsNewTasks !== 'boolean') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点可用性结论。')
  }
  if (agentAssociationStatus === 'PENDING' && (heartbeatStatus !== 'NEVER_CONNECTED' || lastHeartbeatAt !== null || agentFacts)) {
    throw localError('RESPONSE_INVALID', '控制面返回了不一致的 Agent 关联事实。')
  }
  if (heartbeatStatus === 'NEVER_CONNECTED' && lastHeartbeatAt !== null) {
    throw localError('RESPONSE_INVALID', '控制面返回了不一致的节点心跳事实。')
  }
  if (acceptsNewTasks && (managementState !== 'ENABLED' || agentAssociationStatus !== 'ASSOCIATED' || heartbeatStatus !== 'ONLINE' || environmentStatus !== 'NORMAL' || capacityStatus !== 'AVAILABLE' || unavailableReasons.length > 0)) {
    throw localError('RESPONSE_INVALID', '控制面返回了不安全的节点事实状态。')
  }
  return {
    id: requiredString(node, 'id'),
    displayName: requiredString(node, 'displayName'),
    platform: parseExecutionNodePlatform(node),
    managementState,
    agentAssociationStatus,
    heartbeatStatus,
    lastHeartbeatAt,
    environmentStatus,
    capacityStatus,
    acceptsNewTasks,
    unavailableReasons,
    ...(agentFacts ? { agentFacts } : {}),
    revision: requiredNumber(node, 'revision'),
    updatedAt: requiredString(node, 'updatedAt'),
  }
}

function parseExecutionNodeAgentAssociationStatus(value: Record<string, unknown>): ExecutionNodeAgentAssociationStatus {
  const status = requiredString(value, 'agentAssociationStatus')
  if (status !== 'PENDING' && status !== 'ASSOCIATED') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效 Agent 关联状态。')
  }
  return status
}

function parseExecutionNodeHeartbeatStatus(value: Record<string, unknown>): ExecutionNodeHeartbeatStatus {
  const status = requiredString(value, 'heartbeatStatus')
  if (status !== 'NEVER_CONNECTED' && status !== 'ONLINE' && status !== 'OFFLINE') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点心跳状态。')
  }
  return status
}

function parseExecutionNodeEnvironmentStatus(value: Record<string, unknown>): ExecutionNodeEnvironmentStatus {
  const status = requiredString(value, 'environmentStatus')
  if (status !== 'NOT_CHECKED' && status !== 'NORMAL' && status !== 'ABNORMAL' && status !== 'EXPIRED') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点环境状态。')
  }
  return status
}

function parseExecutionNodeCapacityStatus(value: Record<string, unknown>): ExecutionNodeCapacityStatus {
  const status = requiredString(value, 'capacityStatus')
  if (status !== 'UNKNOWN' && status !== 'AVAILABLE' && status !== 'BUSY') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点容量状态。')
  }
  return status
}

function parseExecutionNodeAgentFacts(value: unknown): ExecutionNodeAgentFacts | undefined {
  if (value === undefined || value === null) return undefined
  const facts = asRecord(value)
  const capacityTotal = requiredNonNegativeInteger(facts, 'capacityTotal')
  const capacityUsed = requiredNonNegativeInteger(facts, 'capacityUsed')
  if (capacityUsed > capacityTotal) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点容量快照。')
  }
  return {
    os: requiredString(facts, 'os'),
    arch: requiredString(facts, 'arch'),
    agentVersion: requiredString(facts, 'agentVersion'),
    bootId: requiredString(facts, 'bootId'),
    observedAt: requiredString(facts, 'observedAt'),
    capacityTotal,
    capacityUsed,
    cpuUsagePercent: optionalPercentage(facts, 'cpuUsagePercent'),
    memoryUsagePercent: optionalPercentage(facts, 'memoryUsagePercent'),
  }
}

function parseExecutionNodeDetail(value: Record<string, unknown>): ExecutionNodeDetail {
  const summary = parseExecutionNodeSummary(value)
  return {
    ...summary,
    allowedRoots: requiredStringList(value, 'allowedRoots'),
    toolHome: requiredString(value, 'toolHome'),
    javaPath: requiredString(value, 'javaPath'),
    dataRootUsages: optionalExecutionNodeDataRootUsages(value, 'dataRootUsages'),
    createdAt: requiredString(value, 'createdAt'),
  }
}

function optionalExecutionNodeDataRootUsages(value: Record<string, unknown>, field: string): readonly ExecutionNodeDataRootUsage[] | undefined {
  const raw = value[field]
  if (raw === undefined) return undefined
  if (!Array.isArray(raw)) throw localError('RESPONSE_INVALID', '控制面返回了无效数据目录空间采样。')
  return raw.map((item) => {
    const usage = asRecord(item)
    const totalBytes = requiredNumber(usage, 'totalBytes')
    const availableBytes = requiredNumber(usage, 'availableBytes')
    if (!Number.isSafeInteger(totalBytes) || !Number.isSafeInteger(availableBytes) || totalBytes < 0 || availableBytes < 0 || availableBytes > totalBytes) {
      throw localError('RESPONSE_INVALID', '控制面返回了无效数据目录空间采样。')
    }
    return { root: requiredString(usage, 'root'), totalBytes, availableBytes }
  })
}

function parseExecutionNodeEnrollment(value: Record<string, unknown>): ExecutionNodeEnrollment {
  if (value.displayedOnce !== true) {
    throw localError('RESPONSE_INVALID', '控制面未确认关联材料只能展示一次，已拒绝保留。')
  }
  return {
    requestId: requiredString(value, 'requestId'),
    enrollmentId: requiredString(value, 'enrollmentId'),
    nodeId: requiredString(value, 'nodeId'),
    enrollmentMaterial: requiredString(value, 'enrollmentMaterial'),
    expiresAt: requiredString(value, 'expiresAt'),
    displayedOnce: true,
  }
}

function parseExecutionNodePlatform(value: Record<string, unknown>): ExecutionNodePlatform {
  const platform = requiredString(value, 'platform')
  if (platform !== 'WINDOWS_AMD64' && platform !== 'LINUX_AMD64' && platform !== 'LINUX_ARM64') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效执行节点平台。')
  }
  return platform
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
    logPath: optionalDraftString(value, 'logPath'),
    skipCheckDir: optionalDraftBoolean(value, 'skipCheckDir'),
  }
}

function optionalDraftString(value: Record<string, unknown>, field: string): string {
  const raw = value[field]
  if (raw === undefined) return ''
  if (typeof raw !== 'string') throw localError('RESPONSE_INVALID', `控制面返回了无效字段：${field}。`)
  return raw
}

function optionalDraftBoolean(value: Record<string, unknown>, field: string): boolean {
  const raw = value[field]
  if (raw === undefined) return false
  if (typeof raw !== 'boolean') throw localError('RESPONSE_INVALID', `控制面返回了无效字段：${field}。`)
  return raw
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
    results: requiredPrecheckResults(value, 'results'),
    validUntil: requiredString(value, 'validUntil'),
  }
}

function requiredPrecheckResults(value: Record<string, unknown>, field: string): readonly PrecheckResult[] {
  const raw = value[field]
  const fixedChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] as const
  if (!Array.isArray(raw) || raw.length !== fixedChecks.length) throw localError('RESPONSE_INVALID', '控制面返回了无效预检查结果。')
  const results = raw.map((item) => {
    const result = asRecord(item)
    const check = requiredString(result, 'check')
    const status = requiredString(result, 'status')
    if (!fixedChecks.includes(check as typeof fixedChecks[number])) {
      throw localError('RESPONSE_INVALID', '控制面返回了未知预检查项。')
    }
    if (status !== 'PASSED' && status !== 'FAILED' && status !== 'UNKNOWN') {
      throw localError('RESPONSE_INVALID', '控制面返回了无效预检查状态。')
    }
    return { check, status, evidenceCode: requiredString(result, 'evidenceCode') }
  })
  if (new Set(results.map((result) => result.check)).size !== fixedChecks.length) {
    throw localError('RESPONSE_INVALID', '控制面返回了重复或缺失的预检查项。')
  }
  return results as readonly PrecheckResult[]
}

function parseTaskOverview(value: Record<string, unknown>): TaskOverview {
  const type = requiredString(value, 'type')
  if (type !== 'OBDUMPER_EXPORT') throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务类型。')
  return {
    id: requiredString(value, 'id'),
    type,
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    precheckId: requiredString(value, 'precheckId'),
    submittedAt: requiredString(value, 'submittedAt'),
  }
}

function parseTaskSnapshot(value: Record<string, unknown>): TaskSnapshot {
  const type = requiredString(value, 'type')
  const format = requiredString(value, 'format')
  if (type !== 'OBDUMPER_EXPORT' || format !== 'CSV') {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务快照。')
  }
  return {
    type,
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    precheckId: requiredString(value, 'precheckId'),
    objectSummary: optionalString(value, 'objectSummary'),
    format,
    configFingerprint: requiredString(value, 'configFingerprint'),
    toolVersion: requiredString(value, 'toolVersion'),
    metadataVersion: requiredString(value, 'metadataVersion'),
    capabilityVersion: requiredString(value, 'capabilityVersion'),
  }
}

function parseTaskCommandEvidence(value: Record<string, unknown>): TaskCommandEvidence {
  const kind = requiredString(value, 'kind')
  const redaction = requiredString(value, 'redaction')
  const command = requiredString(value, 'command')
  if (kind !== 'PLANNED' || redaction !== 'PASSWORD_ONLY') {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的命令证据。')
  }
  if (unsafeCommandPattern.test(command)) {
    throw localError('UNSAFE_COMMAND_RESPONSE', '任务命令包含未隐藏的密码，已拒绝展示。')
  }
  return { kind, command, redaction }
}

function parseTaskExecution(value: Record<string, unknown>): TaskExecution {
  const stageEvidence = requiredString(value, 'stageEvidence')
  const progressEvidence = requiredString(value, 'progressEvidence')
  if (stageEvidence !== 'UNAVAILABLE' || progressEvidence !== 'UNAVAILABLE') {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的执行证据。')
  }
  return {
    state: requiredString(value, 'state'),
    executionId: optionalString(value, 'executionId'),
    reconciliationRequired: requiredBoolean(value, 'reconciliationRequired'),
    stageEvidence,
    progressEvidence,
    startedAt: optionalString(value, 'startedAt'),
    finishedAt: optionalString(value, 'finishedAt'),
    updatedAt: requiredString(value, 'updatedAt'),
  }
}

function parseTaskListItem(value: unknown): TaskListItem {
  const item = asRecord(value)
  const type = requiredString(item, 'type')
  const stageEvidence = requiredString(item, 'stageEvidence')
  const progressEvidence = requiredString(item, 'progressEvidence')
  if (type !== 'OBDUMPER_EXPORT' || stageEvidence !== 'UNAVAILABLE' || progressEvidence !== 'UNAVAILABLE') {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务列表证据。')
  }
  return {
    id: requiredString(item, 'id'),
    type,
    dataSourceId: requiredString(item, 'dataSourceId'),
    objectSummary: optionalString(item, 'objectSummary'),
    state: requiredString(item, 'state'),
    stageEvidence,
    progressEvidence,
    reconciliationRequired: requiredBoolean(item, 'reconciliationRequired'),
    nodeId: requiredString(item, 'nodeId'),
    ownedByCurrentUser: requiredBoolean(item, 'ownedByCurrentUser'),
    submittedAt: requiredString(item, 'submittedAt'),
    startedAt: optionalString(item, 'startedAt'),
    finishedAt: optionalString(item, 'finishedAt'),
    updatedAt: requiredString(item, 'updatedAt'),
  }
}

function parseTaskLog(value: unknown): TaskLog {
  const record = asRecord(value)
  const message = record.message
  if (typeof message !== 'string') {
    throw localError('RESPONSE_INVALID', '控制面返回了缺失字段。')
  }
  return {
    sourceSeq: requiredNumber(record, 'sourceSeq'),
    kind: requiredString(record, 'kind'),
    message: unsafeLogPattern.test(message) ? '日志因本地安全策略被隐藏。' : message,
    integrityCode: optionalString(record, 'integrityCode') ?? 'NONE',
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
  return { status, code, message, retryable: value.retryable === true, conflict: status === 409 || status === 412, fieldErrors: parseApiFieldErrors(value.fieldErrors) }
}

function localError(code: string, message: string, retryable = false): ApiError {
  return { status: 0, code, message, retryable, conflict: false, fieldErrors: [] }
}

function parseApiFieldErrors(value: unknown): readonly ApiFieldError[] {
  if (!Array.isArray(value)) return []
  const errors: ApiFieldError[] = []
  for (const item of value.slice(0, 100)) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) continue
    const record = item as Record<string, unknown>
    const field = optionalString(record, 'field')
    const code = optionalString(record, 'code')
    if (!field || !code) continue
    errors.push({ field, code, message: optionalString(record, 'message') })
  }
  return errors
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

function nullableString(value: Record<string, unknown>, key: string): string | null {
  const item = value[key]
  if (item === undefined || item === null) return null
  if (typeof item !== 'string' || !item.trim()) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效文本字段。')
  }
  return item
}

function requiredNumber(value: Record<string, unknown>, key: string): number {
  const item = value[key]
  if (typeof item !== 'number' || !Number.isFinite(item)) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效数值。')
  }
  return item
}

function requiredBoolean(value: Record<string, unknown>, key: string): boolean {
  const item = value[key]
  if (typeof item !== 'boolean') {
    throw localError('RESPONSE_INVALID', `控制面返回的 ${key} 无效。`)
  }
  return item
}

function optionalPositiveInteger(value: Record<string, unknown>, key: string): number | undefined {
  const item = value[key]
  if (item === undefined || item === null) return undefined
  if (typeof item !== 'number' || !Number.isSafeInteger(item) || item <= 0) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效版本号。')
  }
  return item
}

function requiredNonNegativeInteger(value: Record<string, unknown>, key: string): number {
  const item = requiredNumber(value, key)
  if (!Number.isInteger(item) || item < 0) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点容量。')
  }
  return item
}

function optionalPercentage(value: Record<string, unknown>, key: string): number | undefined {
  const item = value[key]
  if (item === undefined || item === null) return undefined
  if (typeof item !== 'number' || !Number.isFinite(item) || item < 0 || item > 100) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效节点资源快照。')
  }
  return item
}

function requiredStringList(value: Record<string, unknown>, key: string): readonly string[] {
  const item = value[key]
  if (!Array.isArray(item) || item.some((entry) => typeof entry !== 'string' || !entry.trim())) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效字符串列表。')
  }
  return item as string[]
}
