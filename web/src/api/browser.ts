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
	if (apiError.code === 'DATA_SOURCE_DELETE_INELIGIBLE' || apiError.code === 'DATA_SOURCE_ARCHIVE_INELIGIBLE') return apiError.message || '数据源生命周期资格已变化，请刷新后重新比较。'
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '数据源不存在或当前身份无权访问。'
  if (apiError.status === 409 || apiError.status === 412 || apiError.conflict) return '数据源已发生变化，请刷新后重新比较。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查当前环境后重试。'
  return apiError.message || fallback
}

export function exportCatalogErrorMessage(error: unknown): string {
  const apiError = error as Partial<ApiError>
  if (apiError.code === 'CSRF_TOKEN_UNAVAILABLE' || apiError.status === 401) return '登录或页面安全校验已失效，请刷新后重试。'
  if (apiError.code === 'EXPORT_OBJECT_CATALOG_FIELDS_INVALID') return '目录查询条件与当前控制面版本不兼容。请更新控制面与 Agent 后重新加载。'
  if (apiError.status === 404) return '对象目录接口不可用，或当前身份无权读取所选数据源和节点。请确认控制面已更新并检查授权；仍可手动添加对象。'
  if (apiError.status === 409 || apiError.status === 412) return '数据源配置已变化，请刷新页面后重新选择。'
  if (apiError.code === 'EXPORT_OBJECT_CATALOG_NODE_UNAVAILABLE') return '执行节点当前无法接收对象查询。请检查 Agent 心跳和节点状态；也可手动添加对象。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查服务状态后重试。'
  return apiError.message || '无法读取对象元数据，可重试或手动添加。'
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

// storageCredentialErrorMessage 为存储凭据读写失败保留安全错误语义：
// 越权与不存在统一为 404，版本冲突与幂等冲突要求刷新后重试。
export function storageCredentialErrorMessage(error: unknown, fallback: string): string {
  const apiError = error as Partial<ApiError>
  if (apiError.code === 'CSRF_TOKEN_UNAVAILABLE') return '当前页面未获得请求安全令牌，已拒绝写操作。请刷新页面后重试。'
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '存储凭据不存在或当前身份无权访问。'
  if (apiError.status === 409 || apiError.status === 412 || apiError.conflict) return '存储凭据已发生变化，请刷新后重新比较。'
  if (apiError.code === 'STORAGE_CREDENTIAL_PROVIDER_MISMATCH') return '所选存储凭据与输出类型不一致。'
  if (apiError.code === 'STORAGE_CREDENTIAL_REVISION_STALE') return '存储凭据已轮换，请重新保存草稿。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查当前环境后重试。'
  return apiError.message || fallback
}

interface DataSourceProjection {
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
  // sysCredentialState 只表达数据源是否配置了可用的 sys 凭据（AVAILABLE/UNAVAILABLE），
  // 不下发账号或任何秘密（参考 ODC 数据源高级设置）。
  readonly sysCredentialState?: string
  readonly lastTestStatus?: string
  readonly lastTestedAt?: string
	readonly lifecycleEligibility?: DataSourceLifecycleEligibility
}

export interface DataSourceLifecycleActionEligibility {
	readonly allowed: boolean
	readonly reasonCode?: string
	readonly reason?: string
	readonly referenceCount?: number
}

// DataSourceLifecycleEligibility 只能来自服务端当前投影；页面不得从状态或引用数量自行推断。
export interface DataSourceLifecycleEligibility {
	readonly enable: DataSourceLifecycleActionEligibility
	readonly disable: DataSourceLifecycleActionEligibility
	readonly delete: DataSourceLifecycleActionEligibility
	readonly archive: DataSourceLifecycleActionEligibility
}

export interface DataSourceSummary extends DataSourceProjection {
  readonly username: string
}

export interface DataSourceDetail extends DataSourceProjection {
  readonly username?: string
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
  // 可选的 sys 凭据（参考 ODC 数据源高级设置）：账号与密码必须同时提供或同时留空。
  readonly sysUser?: string
  readonly sysPassword?: string
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
  // 可选的 sys 凭据（参考 ODC 数据源高级设置）：缺省保持现状，两者同空表示清除，两者同非空表示设置/轮换。
  readonly sysUser?: string
  readonly sysPassword?: string
}

export type DataSourceConnectionTestStatus = 'PENDING' | 'LEASED' | 'SUCCEEDED' | 'FAILED' | 'UNKNOWN' | 'EXPIRED' | 'INVALIDATED'

export type DataSourceConnectionTestVerificationSource = 'G2_SYNTHETIC' | 'AGENT_JDBC'

export type DataSourceConnectionTestSysVerificationStatus = 'NOT_CONFIGURED' | 'SUCCEEDED' | 'FAILED' | 'UNKNOWN'

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
  // 可选的 sys 凭据验证事实（参考 ODC 的 sys 账号验证）：与数据库结果相互独立。
  readonly sysCredentialConfigured: boolean
  readonly sysVerificationStatus?: DataSourceConnectionTestSysVerificationStatus
  readonly sysResultCode?: string
}

export interface ExportObjectCatalogQuery {
  readonly id: string
  readonly status: DataSourceConnectionTestStatus
  readonly dataSourceId: string
  readonly nodeId: string
  readonly database: string
  readonly objectType: 'DATABASE' | ExportObjectType
  readonly keyword: string
  readonly objects: readonly string[]
  readonly truncated: boolean
  readonly validUntil: string
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

export type ExportScopeKind = 'ALL' | 'SPECIFIED'
export type ExportObjectType = 'TABLE' | 'VIEW'
export type ExportContentKind = 'DATA_ONLY' | 'DDL_ONLY' | 'DDL_AND_DATA'
// EX-I4/EX-I5：数据格式单选；CSV/CUT/POS/SQL 为文本格式，PARQUET/ORC/AVRO 为结构化格式（2026-08-07 接入）。
export type ExportDataFormatKind = 'CSV' | 'CUT' | 'POS' | 'SQL' | 'PARQUET' | 'ORC' | 'AVRO'

// StructuredFormat 判断是否结构化格式：压缩与序列化选项不适用。
export function isStructuredFormat(formatKind: ExportDataFormatKind): boolean {
  return formatKind === 'PARQUET' || formatKind === 'ORC' || formatKind === 'AVRO'
}

export interface ExportObjectExpression {
  readonly schema?: string
  readonly name: string
}

export interface ExportObjectScope {
  readonly database: string
  readonly scopeKind: ExportScopeKind
  readonly objectTypes?: readonly ExportObjectType[]
  readonly expressions?: readonly ExportObjectExpression[]
  readonly excludeTables?: readonly string[]
}

export type CsvQuoteMode = 'all' | 'all_not_null' | 'minimal' | 'non_numeric' | 'none'
export type CompressionAlgo = 'zstd' | 'zlib' | 'gzip' | 'snappy'

// CsvOptions 是 EX-I3 启用的 CSV 序列化选项集合；
// EX-I4 起其中的共享文本选项（转义字符、行分隔符、NULL 替换、文件编码、去除空格）
// 随服务端 FORMAT_IN 规则在 CUT/SQL 格式下同样活动；columnSplitter 为 CUT 专属列分隔字符串。
export interface CsvOptions {
  readonly skipHeader?: boolean
  readonly columnSeparator?: string
  readonly columnQuote?: string
  readonly columnQuoteMode?: CsvQuoteMode
  readonly escapeCharacter?: string
  readonly lineSeparator?: string
  readonly nullString?: string
  readonly fileEncoding?: string
  readonly withTrim?: boolean
  readonly columnSplitter?: string
}

// CutOptions 是 EX-I4 启用的 CUT 序列化选项集合；仅数据格式为 CUT 时活动。
export interface CutOptions {
  readonly trailDelimiter?: boolean
  readonly removeNewline?: boolean
}

export interface FilterOptions {
  readonly querySql?: string
  // EX-I7 条件筛选（2026-08-11 实测定版）：--where，与 querySql 互斥。
  readonly where?: string
  // --partition 与 querySql 互斥，--exclude-data-types 为数据内容筛选；
  // enableHiddenPk 保留用于既有草稿解码，浏览器在专用预检查完成前不得构造或提交。
  readonly partition?: string
  readonly excludeDataTypes?: readonly string[]
  readonly enableHiddenPk?: boolean
  readonly includeColumnNames?: readonly string[]
  readonly excludeColumnNames?: readonly string[]
  readonly excludeVirtualColumns?: boolean
  readonly flashbackScn?: number
  readonly flashbackTimestamp?: string
  // EX-I7 一致性（2026-08-11 实测定版）：--snapshot 一致性快照、--weak-read 备库弱读。
  readonly snapshot?: boolean
  readonly weakRead?: boolean
}

// TimestampFormatsOptions 保留九个 OpenAPI 已解码字段；浏览器当前仅向 MySQL CSV/CUT 数据导出发送
// dateValueFormat 与 datetimeValueFormat，其余字段在兼容性验证完成前必须保持关闭。
export interface TimestampFormatsOptions {
  readonly dateValueFormat?: string
  readonly timeValueFormat?: string
  readonly datetimeValueFormat?: string
  readonly timestampValueFormat?: string
  readonly timestampTzValueFormat?: string
  readonly timestampLtzValueFormat?: string
  readonly nlsDateFormat?: string
  readonly nlsTimestampFormat?: string
  readonly nlsTimestampTzFormat?: string
}

export interface PerformanceOptions {
  readonly thread?: number
  readonly pageSize?: number
  readonly parallelMacro?: number
  readonly fetchSize?: number
  readonly jvmMemory?: string
  // EX-I7 文件拆分（2026-08-10）：--block-size（数字 MB 或数字+MB/ROW 后缀），显式传值已受控实测。
  readonly blockSize?: string
  // EX-I7 保存点续跑（2026-08-11 实测定版）：--retry，无保存点时工具失败关闭。
  readonly retry?: boolean
}

// EX-I6 对象存储（2026-08-07）：输出目标类型；对象存储要求受控 URI（凭据走执行槽位，不进 URI）。
export type ExportOutputKind = 'LOCAL' | 'OSS' | 'S3' | 'COS' | 'OBS'

// EX-I6 存储凭据槽位（2026-08-14）：对象存储凭据类型与安全投影。
export type StorageCredentialProvider = 'OSS' | 'S3' | 'COS' | 'OBS'

export interface StorageCredentialListItem {
  readonly id: string
  readonly displayName: string
  readonly provider: StorageCredentialProvider
  readonly currentRevision: number
  readonly revision: number
  readonly updatedAt: string
}

// StorageCredentialWrite 只在创建/轮换请求体内承载密钥明文；
// 密钥绝不进入草稿、命令、日志或任何读取响应。
export interface StorageCredentialWrite {
  readonly displayName: string
  readonly provider: StorageCredentialProvider
  readonly accessKey: string
  readonly secretKey: string
}

// StorageCredentialReference 是草稿输出配置中的对象存储凭据引用；
// 只引用标识与修订，不携带任何密钥材料。
export interface StorageCredentialReference {
  readonly storageCredentialId: string
  readonly revision: number
}

// 前置 DROP 与保留 Schema 仅 DDL 内容时携带，紧凑 Schema 同样仅 DDL 内容；
// addExtraMessage 保留用于既有草稿解码，需完成 sys 权限预检查前浏览器保持关闭。
export interface DDLBehaviorOptions {
  readonly dropObject?: boolean
  readonly retainSchema?: boolean
  readonly compactSchema?: boolean
  // 附加对象信息的浏览器提交仍由 sys 权限预检查门禁，当前不构造该字段。
  readonly addExtraMessage?: boolean
}

export interface GeneralizedExportConfig {
  readonly objectScope: ExportObjectScope
  readonly contentSelection: { readonly contentKind: ExportContentKind }
  readonly dataFormat?: {
    readonly formatKind: ExportDataFormatKind
    readonly csvOptions?: CsvOptions
    readonly cutOptions?: CutOptions
    readonly timestampFormats?: TimestampFormatsOptions
  }
  readonly outputConfig: {
    readonly outputKind: ExportOutputKind
    readonly filePath: string
    readonly logPath?: string
    readonly skipCheckDir?: boolean
    readonly noNestedDir?: boolean
    readonly maxFileSize?: number
    readonly retainEmptyFiles?: boolean
    readonly compress?: boolean
    readonly compressionAlgo?: CompressionAlgo
    // EX-I7 压缩等级（2026-08-10）：--compression-level，官方按算法分范围（zstd 1-22、zlib -1~9；gzip/snappy 不支持）。
    readonly compressionLevel?: number
    // EX-I4 POS 定版：控制文件目录（--ctl-path），仅 POS 格式使用。
    readonly controlFilePath?: string
    // EX-I6 对象存储：Multipart 本地临时分块目录（--tmp-path）。
    readonly tmpPath?: string
    // EX-I6 存储凭据槽位（2026-08-14）：对象存储输出的凭据引用；可缺省（依赖 Hadoop 标准配置链），LOCAL 输出不携带。
    readonly storageCredential?: StorageCredentialReference
  }
  readonly filterConfig?: FilterOptions
  readonly performanceConfig?: PerformanceOptions
  // EX-I7 DDL 行为（2026-08-10）：仅 DDL 内容时携带。
  readonly ddlBehavior?: DDLBehaviorOptions
}

// ExportDraftInput 是向导提交的当前泛化草稿请求；v5 扁平形态仅由历史兼容路径使用。
export interface ExportDraftInput {
  readonly configVersion: 'v6'
  readonly dataSourceId: string
  readonly nodeId: string
  readonly config: GeneralizedExportConfig
}

export interface ExportDraft {
  readonly id: string
  readonly dataSourceId: string
  readonly nodeId: string
  readonly revision: number
  readonly configVersion: 'v5' | 'v6'
  readonly config: GeneralizedExportConfig
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
	readonly check: 'DATABASE_CONNECTIVITY' | 'OBJECT_ACCESS' | 'TOOL_ENVIRONMENT' | 'OUTPUT_PATH' | 'OUTPUT_EMPTY' | 'AVAILABLE_SPACE' | 'STORAGE_CONNECTIVITY' | 'STORAGE_AUTH'
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
  // EX-I8 派生任务关系：基于原配置新建/从头重新执行/从检查点继续的来源任务与派生方式（原始任务缺省）。
  readonly parentTaskId?: string
  readonly derivationKind?: string
}

// TaskDerivationKind 是 EX-I8 三种受控派生方式的浏览器枚举。
export type TaskDerivationKind = 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH' | 'CHECKPOINT_RESUME'

export interface TaskDerivedDraft {
  readonly draftId: string
  readonly sourceTaskId: string
  readonly derivation: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH'
}

export interface TaskDerivedResume {
  readonly id: string
  readonly parentTaskId: string
  readonly derivationKind: 'CHECKPOINT_RESUME'
}

// ExportConfigTemplateItem 是模板列表的安全投影；配置 JSON 不进入浏览器。
export interface ExportConfigTemplateItem {
  readonly id: string
  readonly displayName: string
  readonly capabilityVersion: string
  readonly configFingerprint: string
  readonly sourceTaskId?: string
  readonly revision: number
  readonly createdAt: string
  readonly updatedAt: string
}

export interface TaskSnapshot {
  readonly type: 'OBDUMPER_EXPORT'
  readonly snapshotVersion: 'v1' | 'v2'
  readonly dataSourceId: string
  readonly nodeId: string
  readonly precheckId: string
  readonly objectSummary?: string
  readonly format: 'CSV' | 'CUT' | 'SQL' | 'POS' | 'PARQUET' | 'ORC' | 'AVRO' | 'DDL' | 'DDL_CSV'
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
  // EX-I8 结果与失败事实：Agent 观察到的文件/字节/检查点事实（未执行或未上报时缺省）。
  readonly resultSummary?: TaskResultSummary
}

export interface TaskResultSummary {
  readonly result: 'VERIFIED' | 'FAILED'
  readonly fileCount: number
  readonly totalBytes: number
  readonly files: readonly { readonly path: string; readonly size: number }[]
  readonly checkpointPresent: boolean
  readonly observedAt: string
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
  listDataSourcePage(query: Record<string, string>): Promise<{ items: DataSourceSummary[]; nextCursor: string; total: number }>
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
  getDataSource(dataSourceId: string): Promise<DataSourceDetail>
  createDataSource(input: DataSourceWrite): Promise<string>
  updateDataSource(dataSourceId: string, revision: number, input: DataSourceUpdate): Promise<DataSourceDetail>
  changeDataSourceState(dataSourceId: string, revision: number, targetState: 'ENABLED' | 'DISABLED'): Promise<DataSourceStateChange>
	deleteDataSource(dataSourceId: string, revision: number): Promise<DataSourceDeletionResult>
	archiveDataSource(dataSourceId: string, revision: number): Promise<DataSourceDeletionResult>
  startDataSourceConnectionTest(dataSourceId: string, revision: number, nodeId: string): Promise<DataSourceConnectionTestRequest>
  getDataSourceConnectionTest(connectionTestId: string): Promise<DataSourceConnectionTest>
  searchExportObjects(dataSourceId: string, revision: number, input: { nodeId: string; database: string; objectType: 'DATABASE' | ExportObjectType; keyword: string }): Promise<ExportObjectCatalogQuery>
  getExportObjectCatalogQuery(queryId: string): Promise<ExportObjectCatalogQuery>
  createExportDraft(input: ExportDraftInput): Promise<string>
  getExportDraft(draftId: string): Promise<ExportDraft>
  updateExportDraft(draft: ExportDraft): Promise<ExportDraft>
  previewExportCommand(draft: ExportDraft): Promise<CommandPreview>
  startPrecheck(draft: ExportDraft): Promise<string>
  getPrecheck(precheckId: string): Promise<Precheck>
  submitExportDraft(draft: ExportDraft, precheckId: string): Promise<string>
  listStorageCredentials(): Promise<StorageCredentialListItem[]>
  createStorageCredential(input: StorageCredentialWrite): Promise<StorageCredentialListItem>
  rotateStorageCredential(storageCredentialId: string, revision: number, input: StorageCredentialWrite): Promise<StorageCredentialListItem>
  deleteStorageCredential(storageCredentialId: string, revision: number): Promise<void>
  listTasks(cursor?: string, limit?: TaskListPageSize): Promise<TaskListPage>
  getTaskOverview(taskId: string): Promise<TaskOverview>
  // EX-I8 派生操作：从失败任务重建可编辑草稿（基于原配置新建/从头重新执行），
  // 或从失败导出任务创建检查点继续任务（继承原快照并追加 --retry）。
  rebuildTaskDraft(taskId: string, derivation: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH'): Promise<TaskDerivedDraft>
  resumeTaskFromCheckpoint(taskId: string): Promise<TaskDerivedResume>
  // EX-I8 模板复用：列表/改名/删除、从成功任务保存模板、由模板创建草稿。
  listExportConfigTemplates(): Promise<ExportConfigTemplateItem[]>
  saveTaskTemplate(taskId: string, displayName: string): Promise<string>
  renameExportConfigTemplate(templateId: string, revision: number, displayName: string): Promise<number>
  deleteExportConfigTemplate(templateId: string, revision: number): Promise<void>
  createDraftFromTemplate(templateId: string, dataSourceId: string, nodeId: string): Promise<string>
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
    async listDataSourcePage(query) {
      const body = await request(options, `/api/v1/data-sources?${new URLSearchParams({ ...query, limit: '10' })}`, { method: 'GET' })
      const total = body.total
      if (typeof total !== 'number' || !Number.isSafeInteger(total) || total < 0 || typeof body.nextCursor !== 'string') throw new Error('数据源分页响应无效')
      return { items: listOf(body, 'items').map(parseDataSourceSummary), nextCursor: body.nextCursor, total }
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
      return parseDataSourceDetail(requiredObject(body, 'item'))
    },
    async createDataSource(input) {
      const body = await request(options, '/api/v1/data-sources', writeRequest(options, input))
      return requiredString(body, 'id')
    },
    async updateDataSource(dataSourceId, revision, input) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}`, writeRequest(options, input, revision, 'PATCH', false))
      return parseDataSourceDetail(requiredObject(body, 'item'))
    },
    async changeDataSourceState(dataSourceId, revision, targetState) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}:${targetState === 'ENABLED' ? 'enable' : 'disable'}`, writeRequest(options, {}, revision, 'POST', false))
      const state = requiredString(body, 'state')
      if (state !== 'ENABLED' && state !== 'DISABLED') {
        throw localError('RESPONSE_INVALID', '控制面返回了无效数据源状态。')
      }
      return { state, revision: requiredNumber(body, 'revision') }
    },
	async deleteDataSource(dataSourceId, revision) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}`, writeRequest(options, {}, revision, 'DELETE', false))
      const outcome = requiredString(body, 'outcome')
		if (outcome !== 'DELETED') {
        throw localError('RESPONSE_INVALID', '控制面返回了无效数据源删除结果。')
      }
		return { outcome, revision: requiredNumber(body, 'revision') }
	},
	async archiveDataSource(dataSourceId, revision) {
		const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}:archive`, writeRequest(options, {}, revision, 'POST', false))
		if (requiredString(body, 'outcome') !== 'ARCHIVED') {
			throw localError('RESPONSE_INVALID', '控制面返回了无效数据源归档结果。')
		}
		return { outcome: 'ARCHIVED', revision: requiredNumber(body, 'revision') }
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
    async searchExportObjects(dataSourceId, revision, input) {
      const body = await request(options, `/api/v1/data-sources/${encodeURIComponent(dataSourceId)}:search-export-objects`, writeRequest(options, input, revision, 'POST', true))
      return parseExportObjectCatalogQuery(requiredObject(body, 'item'))
    },
    async getExportObjectCatalogQuery(queryId) {
      const body = await request(options, `/api/v1/export-object-catalog-queries/${encodeURIComponent(queryId)}`, { method: 'GET' })
      return parseExportObjectCatalogQuery(requiredObject(body, 'item'))
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
      // 草稿更新必须提交完整 v6 写请求；服务端按 configVersion 路由并重新失败关闭校验。
      const input: ExportDraftInput = { configVersion: 'v6', dataSourceId: draft.dataSourceId, nodeId: draft.nodeId, config: draft.config }
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draft.id)}`, writeRequest(options, input, draft.revision, 'PATCH'))
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
    async listStorageCredentials() {
      const body = await request(options, '/api/v1/storage-credentials', { method: 'GET' })
      return listOf(body, 'items').map(parseStorageCredentialListItem)
    },
    async createStorageCredential(input) {
      const body = await request(options, '/api/v1/storage-credentials', writeRequest(options, input))
      return parseStorageCredentialListItem(requiredObject(body, 'item'))
    },
    async rotateStorageCredential(storageCredentialId, revision, input) {
      // 轮换同时要求 If-Match 乐观锁与幂等键；服务端轮换后返回新修订的安全投影。
      const body = await request(options, `/api/v1/storage-credentials/${encodeURIComponent(storageCredentialId)}:rotate`, writeRequest(options, input, revision, 'POST', true))
      return parseStorageCredentialListItem(requiredObject(body, 'item'))
    },
    async deleteStorageCredential(storageCredentialId, revision) {
      await request(options, `/api/v1/storage-credentials/${encodeURIComponent(storageCredentialId)}`, writeRequest(options, {}, revision, 'DELETE', false))
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
    async rebuildTaskDraft(taskId, derivation) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}:rebuild-draft`, writeRequest(options, { derivation }))
      const draftId = requiredString(body, 'draftId')
      const sourceTaskId = requiredString(body, 'sourceTaskId')
      const responded = requiredString(body, 'derivation')
      if (responded !== derivation || sourceTaskId !== taskId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与请求不一致的派生草稿。')
      }
      return { draftId, sourceTaskId, derivation }
    },
    async resumeTaskFromCheckpoint(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}:resume-checkpoint`, writeEmptyPost(options))
      const id = requiredString(body, 'id')
      if (requiredString(body, 'parentTaskId') !== taskId || requiredString(body, 'derivationKind') !== 'CHECKPOINT_RESUME') {
        throw localError('RESPONSE_INVALID', '控制面返回了与请求不一致的继续任务。')
      }
      return { id, parentTaskId: taskId, derivationKind: 'CHECKPOINT_RESUME' }
    },
    async listExportConfigTemplates() {
      const body = await request(options, '/api/v1/export-config-templates', { method: 'GET' })
      return listOf(body, 'items').map(parseExportConfigTemplateItem)
    },
    async saveTaskTemplate(taskId, displayName) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}:save-template`, writeRequest(options, { displayName }))
      const id = requiredString(body, 'id')
      if (requiredString(body, 'sourceTaskId') !== taskId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与请求不一致的模板。')
      }
      return id
    },
    async renameExportConfigTemplate(templateId, revision, displayName) {
      const body = await request(options, `/api/v1/export-config-templates/${encodeURIComponent(templateId)}`, writeRequest(options, { displayName }, revision, 'PATCH', false))
      return requiredNumber(body, 'revision')
    },
    async deleteExportConfigTemplate(templateId, revision) {
      await request(options, `/api/v1/export-config-templates/${encodeURIComponent(templateId)}`, writeRequest(options, {}, revision, 'DELETE', false))
    },
    async createDraftFromTemplate(templateId, dataSourceId, nodeId) {
      const body = await request(options, `/api/v1/export-config-templates/${encodeURIComponent(templateId)}:create-draft`, writeRequest(options, { dataSourceId, nodeId }))
      const draftId = requiredString(body, 'draftId')
      if (requiredString(body, 'templateId') !== templateId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与请求不一致的草稿。')
      }
      return draftId
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
    fetcher: async (input, init) => {
      const response = await fetch(input, init)
      if (response.status === 401) window.location.assign('/login')
      return response
    },
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

function parseDataSourceProjection(source: Record<string, unknown>): DataSourceProjection {
  return {
    id: requiredString(source, 'id'),
    displayName: requiredString(source, 'displayName'),
    environment: requiredString(source, 'environment'),
    connectionKind: requiredString(source, 'connectionKind'),
    compatibilityMode: requiredString(source, 'compatibilityMode'),
    host: requiredString(source, 'host'),
    port: requiredNumber(source, 'port'),
    clusterName: requiredStringAllowEmpty(source, 'clusterName'),
    tenantName: requiredString(source, 'tenantName'),
    defaultDatabase: optionalString(source, 'defaultDatabase'),
    state: requiredString(source, 'state'),
    revision: requiredNumber(source, 'revision'),
    credentialRevision: requiredNumber(source, 'credentialRevision'),
    sysCredentialState: optionalString(source, 'sysCredentialState'),
		lastTestStatus: optionalString(source, 'lastTestStatus'),
		lastTestedAt: optionalString(source, 'lastTestedAt'),
		lifecycleEligibility: parseDataSourceLifecycleEligibility(source['lifecycleEligibility']),
	}
}

function parseDataSourceLifecycleEligibility(value: unknown): DataSourceLifecycleEligibility | undefined {
	if (value === undefined || value === null) return undefined
	const eligibility = asRecord(value)
	return {
		enable: parseDataSourceLifecycleActionEligibility(requiredObject(eligibility, 'enable')),
		disable: parseDataSourceLifecycleActionEligibility(requiredObject(eligibility, 'disable')),
		delete: parseDataSourceLifecycleActionEligibility(requiredObject(eligibility, 'delete')),
		archive: parseDataSourceLifecycleActionEligibility(requiredObject(eligibility, 'archive')),
	}
}

function parseDataSourceLifecycleActionEligibility(value: Record<string, unknown>): DataSourceLifecycleActionEligibility {
	const referenceCount = optionalNumber(value, 'referenceCount')
	if (referenceCount !== undefined && (!Number.isSafeInteger(referenceCount) || referenceCount < 1)) {
		throw localError('RESPONSE_INVALID', '控制面返回了无效数据源生命周期引用数量。')
	}
	return { allowed: requiredBoolean(value, 'allowed'), reasonCode: optionalString(value, 'reasonCode'), reason: optionalString(value, 'reason'), referenceCount }
}

function parseDataSourceSummary(value: unknown): DataSourceSummary {
  const source = asRecord(value)
  return { ...parseDataSourceProjection(source), username: requiredString(source, 'username') }
}

function parseDataSourceDetail(value: unknown): DataSourceDetail {
  const source = asRecord(value)
  return { ...parseDataSourceProjection(source), username: optionalString(source, 'username') }
}

function parseDataSourceConnectionTestRequest(value: Record<string, unknown>): DataSourceConnectionTestRequest {
  return {
    id: requiredString(value, 'id'),
    status: parseDataSourceConnectionTestStatus(value),
    nodeId: requiredString(value, 'nodeId'),
  }
}

function parseExportObjectCatalogQuery(value: Record<string, unknown>): ExportObjectCatalogQuery {
  const status = parseDataSourceConnectionTestStatus(value)
  const objectType = requiredString(value, 'objectType')
  if (objectType !== 'DATABASE' && objectType !== 'TABLE' && objectType !== 'VIEW') throw localError('RESPONSE_INVALID', '控制面返回了无效对象类型。')
  const database = objectType === 'DATABASE' ? requiredStringAllowEmpty(value, 'database') : requiredString(value, 'database')
  if (objectType === 'DATABASE' && database !== '') throw localError('RESPONSE_INVALID', '控制面返回了无效数据库查询范围。')
  const objects = requiredStringList(value, 'objects')
  if (objects.length > 100 || objects.some((name) => !name || name.length > 256 || /[*,\r\n\0]/.test(name))) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效对象目录。')
  }
  if (status !== 'SUCCEEDED' && objects.length > 0) throw localError('RESPONSE_INVALID', '未完成的对象查询包含对象名称。')
  return {
    id: requiredString(value, 'id'), status, dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'), database, objectType,
    keyword: optionalString(value, 'keyword') ?? '', objects, truncated: requiredBoolean(value, 'truncated'),
    validUntil: requiredString(value, 'validUntil'),
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
    sysCredentialConfigured: requiredBoolean(value, 'sysCredentialConfigured'),
    sysVerificationStatus: optionalString(value, 'sysVerificationStatus') as DataSourceConnectionTestSysVerificationStatus | undefined,
    sysResultCode: optionalString(value, 'sysResultCode'),
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

// parseStorageCredentialListItem 按白名单解析存储凭据安全投影：
// provider 只接受四种受控类型，修订必须为正整数，密钥字段绝不进入页面状态。
function parseStorageCredentialListItem(value: unknown): StorageCredentialListItem {
  const item = asRecord(value)
  const provider = requiredString(item, 'provider')
  if (provider !== 'OSS' && provider !== 'S3' && provider !== 'COS' && provider !== 'OBS') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效存储凭据提供方。')
  }
  const currentRevision = requiredNumber(item, 'currentRevision')
  const revision = requiredNumber(item, 'revision')
  if (!Number.isSafeInteger(currentRevision) || currentRevision < 1 || !Number.isSafeInteger(revision) || revision < 1 || revision > currentRevision) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效存储凭据修订。')
  }
  return {
    id: requiredString(item, 'id'),
    displayName: requiredString(item, 'displayName'),
    provider,
    currentRevision,
    revision,
    updatedAt: requiredString(item, 'updatedAt'),
  }
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
  const configVersion = requiredString(value, 'configVersion')
  if (configVersion !== 'v5' && configVersion !== 'v6') {
    throw localError('RESPONSE_INVALID', '控制面返回了未知的草稿配置版本。')
  }
  return {
    id: requiredString(value, 'id'),
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    revision: requiredNumber(value, 'revision'),
    configVersion,
    config: parseDraftInput(requiredObject(value, 'config')),
    configFingerprint: requiredString(value, 'configFingerprint'),
  }
}

// parseDraftInput 解析 v6 标准文档：嵌套泛化配置位于 config 键内，
// 顶层扁平投影键（database/scopeKind/table/contentKind/format/filePath 等）只用于展示，不参与输入重建。
// Go 对 nil 切片序列化为 null，可选数组字段的 null 与缺省等价。
function parseDraftInput(value: Record<string, unknown>): GeneralizedExportConfig {
  const nested = requiredObject(value, 'config')
  const scope = requiredObject(nested, 'objectScope')
  const scopeKind = requiredString(scope, 'scopeKind')
  if (scopeKind !== 'ALL' && scopeKind !== 'SPECIFIED') {
    throw localError('RESPONSE_INVALID', '控制面返回了未知的对象范围。')
  }
  const expressionsRaw = scope['expressions']
  const expressions: ExportObjectExpression[] = []
  if (expressionsRaw !== undefined && expressionsRaw !== null) {
    if (!Array.isArray(expressionsRaw)) throw localError('RESPONSE_INVALID', '控制面返回了无效对象表达式。')
    for (const item of expressionsRaw) {
      if (typeof item !== 'object' || item === null) throw localError('RESPONSE_INVALID', '控制面返回了无效对象表达式。')
      const expression = item as Record<string, unknown>
      const name = expression['name']
      const schema = expression['schema']
      if (typeof name !== 'string' || !name) throw localError('RESPONSE_INVALID', '控制面返回了无效对象名称。')
      if (schema !== undefined && typeof schema !== 'string') throw localError('RESPONSE_INVALID', '控制面返回了无效对象 Schema。')
      expressions.push(schema === undefined ? { name } : { schema, name })
    }
  }
  const objectTypesRaw = scope['objectTypes']
  const objectTypes: ExportObjectType[] = []
  if (objectTypesRaw !== undefined && objectTypesRaw !== null) {
    if (!Array.isArray(objectTypesRaw)) throw localError('RESPONSE_INVALID', '控制面返回了无效对象类型。')
    for (const item of objectTypesRaw) {
      if (item !== 'TABLE' && item !== 'VIEW') throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的对象类型。')
      objectTypes.push(item)
    }
  }
  const excludeTablesRaw = scope['excludeTables']
  const excludeTables: string[] = []
  if (excludeTablesRaw !== undefined && excludeTablesRaw !== null) {
    if (!Array.isArray(excludeTablesRaw) || excludeTablesRaw.some((item) => typeof item !== 'string')) {
      throw localError('RESPONSE_INVALID', '控制面返回了无效排除表清单。')
    }
    excludeTables.push(...(excludeTablesRaw as string[]))
  }
  const contentSelection = requiredObject(nested, 'contentSelection')
  const contentKind = requiredString(contentSelection, 'contentKind')
  if (contentKind !== 'DATA_ONLY' && contentKind !== 'DDL_ONLY' && contentKind !== 'DDL_AND_DATA') {
    throw localError('RESPONSE_INVALID', '控制面返回了未知的导出内容。')
  }
  let dataFormat: { readonly formatKind: ExportDataFormatKind; readonly csvOptions?: CsvOptions; readonly cutOptions?: CutOptions } | undefined
  const dataFormatRaw = nested['dataFormat']
  if (dataFormatRaw !== undefined && dataFormatRaw !== null) {
    const dataFormatObject = requiredObject(nested, 'dataFormat')
    const formatKind = dataFormatObject['formatKind']
    // 仅 DDL 场景服务端会把 formatKind 序列化为空字符串，等价于未选择数据格式。
    if (formatKind !== undefined && formatKind !== null && formatKind !== '') {
      if (formatKind !== 'CSV' && formatKind !== 'CUT' && formatKind !== 'POS' && formatKind !== 'SQL' && formatKind !== 'PARQUET' && formatKind !== 'ORC' && formatKind !== 'AVRO') throw localError('UNSUPPORTED_DRAFT_FORMAT', '当前页面不支持该数据格式。')
      dataFormat = { formatKind, csvOptions: parseCsvOptions(dataFormatObject), cutOptions: parseCutOptions(dataFormatObject) }
    }
  }
  const outputConfig = requiredObject(nested, 'outputConfig')
  const outputKind = requiredString(outputConfig, 'outputKind')
  // EX-I6：接受本地路径与受控对象存储输出类型；未知输出目标失败关闭。
  if (outputKind !== 'LOCAL' && outputKind !== 'OSS' && outputKind !== 'S3' && outputKind !== 'COS' && outputKind !== 'OBS') {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的输出目标。')
  }
  return {
    objectScope: {
      database: requiredString(scope, 'database'),
      scopeKind,
      objectTypes: objectTypes.length > 0 ? objectTypes : undefined,
      expressions: expressions.length > 0 ? expressions : undefined,
      excludeTables: excludeTables.length > 0 ? excludeTables : undefined,
    },
    contentSelection: { contentKind },
    dataFormat,
    outputConfig: {
      outputKind,
      filePath: requiredString(outputConfig, 'filePath'),
      logPath: optionalDraftString(outputConfig, 'logPath'),
      skipCheckDir: optionalDraftBoolean(outputConfig, 'skipCheckDir'),
      noNestedDir: optionalBoolean(outputConfig, 'noNestedDir'),
      maxFileSize: optionalNumber(outputConfig, 'maxFileSize'),
      retainEmptyFiles: optionalBoolean(outputConfig, 'retainEmptyFiles'),
      compress: optionalBoolean(outputConfig, 'compress'),
      compressionAlgo: parseCompressionAlgo(outputConfig),
      // EX-I4 POS 定版：控制文件目录（--ctl-path），仅 POS 格式携带。
      controlFilePath: optionalDraftString(outputConfig, 'controlFilePath'),
      // EX-I6 对象存储：Multipart 本地临时分块目录（--tmp-path）。
      tmpPath: optionalDraftString(outputConfig, 'tmpPath'),
      // EX-I6 存储凭据槽位：对象存储输出的凭据引用往返；LOCAL 输出不携带。
      storageCredential: parseStorageCredentialReference(outputConfig),
    },
    filterConfig: parseFilterConfig(nested),
    performanceConfig: parsePerformanceConfig(nested),
  }
}

// parseCsvOptions 解析 CSV 序列化选项；缺省、null 或空值视为未设置。
function parseCsvOptions(dataFormat: Record<string, unknown>): CsvOptions | undefined {
  const raw = dataFormat['csvOptions']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const quoteMode = options['columnQuoteMode']
  if (quoteMode !== undefined && quoteMode !== null && quoteMode !== '' && (quoteMode !== 'all' && quoteMode !== 'all_not_null' && quoteMode !== 'minimal' && quoteMode !== 'non_numeric' && quoteMode !== 'none')) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效的 CSV 包围模式。')
  }
  return {
    skipHeader: optionalBoolean(options, 'skipHeader'),
    columnSeparator: optionalOptionText(options, 'columnSeparator'),
    columnQuote: optionalOptionText(options, 'columnQuote'),
    columnQuoteMode: (quoteMode as CsvQuoteMode | undefined) ?? undefined,
    escapeCharacter: optionalOptionText(options, 'escapeCharacter'),
    lineSeparator: optionalOptionText(options, 'lineSeparator'),
    nullString: optionalOptionText(options, 'nullString'),
    fileEncoding: optionalOptionText(options, 'fileEncoding'),
    withTrim: optionalBoolean(options, 'withTrim'),
    // EX-I4 POS 定版：CUT 列分隔字符串（--column-splitter）随 CUT 格式往返。
    columnSplitter: optionalOptionText(options, 'columnSplitter'),
  }
}

// parseCutOptions 解析 CUT 序列化选项；缺省、null 或全空值视为未设置。
function parseCutOptions(dataFormat: Record<string, unknown>): CutOptions | undefined {
  const raw = dataFormat['cutOptions']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const trailDelimiter = optionalBoolean(options, 'trailDelimiter')
  const removeNewline = optionalBoolean(options, 'removeNewline')
  if (trailDelimiter === undefined && removeNewline === undefined) return undefined
  return {
    trailDelimiter: trailDelimiter || undefined,
    removeNewline: removeNewline || undefined,
  }
}

// parseCompressionAlgo 校验压缩算法枚举；缺省视为未设置。
function parseCompressionAlgo(outputConfig: Record<string, unknown>): CompressionAlgo | undefined {
  const raw = outputConfig['compressionAlgo']
  if (raw === undefined || raw === null || raw === '') return undefined
  if (raw !== 'zstd' && raw !== 'zlib' && raw !== 'gzip' && raw !== 'snappy') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效的压缩算法。')
  }
  return raw
}

// parseStorageCredentialReference 解析草稿输出配置中的存储凭据引用；缺省或 null 视为未绑定。
// 只接受非空标识与正修订，绝不接受或回显密钥字段。
function parseStorageCredentialReference(outputConfig: Record<string, unknown>): StorageCredentialReference | undefined {
  const raw = outputConfig['storageCredential']
  if (raw === undefined || raw === null) return undefined
  const reference = asRecord(raw)
  const storageCredentialId = requiredString(reference, 'storageCredentialId')
  const revision = requiredNumber(reference, 'revision')
  if (!Number.isSafeInteger(revision) || revision < 1) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效存储凭据引用。')
  }
  return { storageCredentialId, revision }
}

// parseFilterConfig 解析 EX-I3 筛选选项；缺省或 null 视为未设置。
function parseFilterConfig(nested: Record<string, unknown>): FilterOptions | undefined {
  const raw = nested['filterConfig']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const querySql = optionalOptionText(options, 'querySql')
  const includeColumnNames = optionalStringList(options, 'includeColumnNames')
  const excludeColumnNames = optionalStringList(options, 'excludeColumnNames')
  if (querySql === undefined && includeColumnNames === undefined && excludeColumnNames === undefined && options['excludeVirtualColumns'] === undefined && options['flashbackScn'] === undefined && options['flashbackTimestamp'] === undefined) {
    return undefined
  }
  return {
    querySql,
    includeColumnNames,
    excludeColumnNames,
    excludeVirtualColumns: optionalBoolean(options, 'excludeVirtualColumns'),
    flashbackScn: optionalNumber(options, 'flashbackScn'),
    flashbackTimestamp: optionalOptionText(options, 'flashbackTimestamp'),
  }
}

// parsePerformanceConfig 解析 EX-I3 资源选项；缺省或 null 视为未设置。
function parsePerformanceConfig(nested: Record<string, unknown>): PerformanceOptions | undefined {
  const raw = nested['performanceConfig']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const thread = optionalNumber(options, 'thread')
  const pageSize = optionalNumber(options, 'pageSize')
  const parallelMacro = optionalNumber(options, 'parallelMacro')
  const fetchSize = optionalNumber(options, 'fetchSize')
  const jvmMemory = optionalOptionText(options, 'jvmMemory')
  if (thread === undefined && pageSize === undefined && parallelMacro === undefined && fetchSize === undefined && jvmMemory === undefined) {
    return undefined
  }
  return { thread, pageSize, parallelMacro, fetchSize, jvmMemory }
}

function optionalBoolean(value: Record<string, unknown>, field: string): boolean | undefined {
  const raw = value[field]
  if (raw === undefined || raw === null) return undefined
  if (typeof raw !== 'boolean') throw localError('RESPONSE_INVALID', `控制面返回了无效字段：${field}。`)
  return raw
}

function optionalNumber(value: Record<string, unknown>, field: string): number | undefined {
  const raw = value[field]
  if (raw === undefined || raw === null) return undefined
  if (typeof raw !== 'number' || !Number.isSafeInteger(raw)) throw localError('RESPONSE_INVALID', `控制面返回了无效字段：${field}。`)
  return raw
}

function optionalOptionText(value: Record<string, unknown>, field: string): string | undefined {
  const raw = value[field]
  if (raw === undefined || raw === null || raw === '') return undefined
  if (typeof raw !== 'string') throw localError('RESPONSE_INVALID', `控制面返回了无效字段：${field}。`)
  return raw
}

function optionalStringList(value: Record<string, unknown>, field: string): readonly string[] | undefined {
  const raw = value[field]
  if (raw === undefined || raw === null) return undefined
  if (!Array.isArray(raw) || raw.some((entry) => typeof entry !== 'string' || !entry.trim())) {
    throw localError('RESPONSE_INVALID', `控制面返回了无效字段：${field}。`)
  }
  return raw as readonly string[]
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
  // EX-I6：预检查清单按输出类型二选一——本地冻结六项或对象存储六项（含两项存储检查）。
  const localChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] as const
  const storageChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'AVAILABLE_SPACE', 'STORAGE_CONNECTIVITY', 'STORAGE_AUTH'] as const
  const knownChecks = [...localChecks, ...storageChecks] as const
  if (!Array.isArray(raw) || raw.length !== localChecks.length) throw localError('RESPONSE_INVALID', '控制面返回了无效预检查结果。')
  const results = raw.map((item) => {
    const result = asRecord(item)
    const check = requiredString(result, 'check')
    const status = requiredString(result, 'status')
    if (!(knownChecks as readonly string[]).includes(check)) {
      throw localError('RESPONSE_INVALID', '控制面返回了未知预检查项。')
    }
    if (status !== 'PASSED' && status !== 'FAILED' && status !== 'UNKNOWN') {
      throw localError('RESPONSE_INVALID', '控制面返回了无效预检查状态。')
    }
    return { check, status, evidenceCode: requiredString(result, 'evidenceCode') }
  })
  if (new Set(results.map((result) => result.check)).size !== results.length) {
    throw localError('RESPONSE_INVALID', '控制面返回了重复的预检查项。')
  }
  const orderedChecks = results.map((result) => result.check).join(',')
  if (orderedChecks !== localChecks.join(',') && orderedChecks !== storageChecks.join(',')) {
    throw localError('RESPONSE_INVALID', '控制面返回了未知顺序的预检查项。')
  }
  return results as readonly PrecheckResult[]
}

// parseExportConfigTemplateItem 按白名单解析模板列表投影：配置 JSON 不进入页面状态。
function parseExportConfigTemplateItem(value: unknown): ExportConfigTemplateItem {
  const item = asRecord(value)
  return {
    id: requiredString(item, 'id'),
    displayName: requiredString(item, 'displayName'),
    capabilityVersion: requiredString(item, 'capabilityVersion'),
    configFingerprint: requiredString(item, 'configFingerprint'),
    sourceTaskId: optionalString(item, 'sourceTaskId'),
    revision: requiredNumber(item, 'revision'),
    createdAt: requiredString(item, 'createdAt'),
    updatedAt: requiredString(item, 'updatedAt'),
  }
}

function parseTaskOverview(value: Record<string, unknown>): TaskOverview {
  const type = requiredString(value, 'type')
  if (type !== 'OBDUMPER_EXPORT') throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务类型。')
  const parentTaskId = optionalString(value, 'parentTaskId')
  const derivationKind = optionalString(value, 'derivationKind')
  if (derivationKind !== undefined && derivationKind !== 'REBUILD_FROM_CONFIG' && derivationKind !== 'RERUN_FROM_SCRATCH' && derivationKind !== 'CHECKPOINT_RESUME') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效的派生方式。')
  }
  if ((parentTaskId === undefined) !== (derivationKind === undefined)) {
    throw localError('RESPONSE_INVALID', '控制面返回了不一致的派生关系。')
  }
  return {
    id: requiredString(value, 'id'),
    type,
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'),
    precheckId: requiredString(value, 'precheckId'),
    submittedAt: requiredString(value, 'submittedAt'),
    ...(parentTaskId && derivationKind ? { parentTaskId, derivationKind } : {}),
  }
}

function parseTaskSnapshot(value: Record<string, unknown>): TaskSnapshot {
  const type = requiredString(value, 'type')
  const snapshotVersion = requiredString(value, 'snapshotVersion')
  const format = requiredString(value, 'format')
  if (type !== 'OBDUMPER_EXPORT' || (snapshotVersion !== 'v1' && snapshotVersion !== 'v2') || (format !== 'CSV' && format !== 'CUT' && format !== 'SQL' && format !== 'POS' && format !== 'PARQUET' && format !== 'ORC' && format !== 'AVRO' && format !== 'DDL' && format !== 'DDL_CSV')) {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务快照。')
  }
  return {
    type,
    snapshotVersion,
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
    resultSummary: parseTaskResultSummary(value['resultSummary']),
  }
}

// containsResultPathControlBreak 检测结果文件相对路径中的 NUL、回车与换行。
function containsResultPathControlBreak(value: string): boolean {
  for (const character of value) {
    if (character === '\u0000' || character === '\r' || character === '\n') return true
  }
  return false
}

// parseTaskResultSummary 按白名单解析结果摘要：result 枚举、非负整数与受限相对路径清单；
// 缺省（未执行/未上报）返回 undefined，越界响应失败关闭。
function parseTaskResultSummary(value: unknown): TaskResultSummary | undefined {
  if (value === undefined || value === null) return undefined
  const summary = asRecord(value)
  const result = requiredString(summary, 'result')
  if (result !== 'VERIFIED' && result !== 'FAILED') throw localError('RESPONSE_INVALID', '控制面返回了无效结果结论。')
  const fileCount = requiredNumber(summary, 'fileCount')
  const totalBytes = requiredNumber(summary, 'totalBytes')
  if (!Number.isSafeInteger(fileCount) || fileCount < 0 || !Number.isSafeInteger(totalBytes) || totalBytes < 0) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效结果数量。')
  }
  const rawFiles = summary['files']
  if (!Array.isArray(rawFiles) || rawFiles.length > 100) throw localError('RESPONSE_INVALID', '控制面返回了无效结果文件清单。')
  const files = rawFiles.map((item) => {
    const file = asRecord(item)
    const path = requiredString(file, 'path')
    const size = requiredNumber(file, 'size')
    if (!path || path.length > 512 || containsResultPathControlBreak(path) || path.startsWith('/') || path.includes('../') || path.endsWith('/..') || path.includes('..\\')) {
      throw localError('RESPONSE_INVALID', '控制面返回了无效结果文件路径。')
    }
    if (!Number.isSafeInteger(size) || size < 0) throw localError('RESPONSE_INVALID', '控制面返回了无效结果文件大小。')
    return { path, size }
  })
  return {
    result,
    fileCount,
    totalBytes,
    files,
    checkpointPresent: requiredBoolean(summary, 'checkpointPresent'),
    observedAt: requiredString(summary, 'observedAt'),
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

function requiredStringAllowEmpty(value: Record<string, unknown>, key: string): string {
  const item = value[key]
  if (typeof item !== 'string') {
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
