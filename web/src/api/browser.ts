import { parseNodeHostInventory, parseNodeRuntimeInspection, parseNodeActivity, parseNodeRecentTasks, type NodeHostInventory, type NodeRuntimeInspection, type NodeActivity, type NodeRecentTasks } from './nodeRuntimeFacts'
import { parseTaskExecutionFacts } from './taskExecutionFacts'
import { parseTaskProgressEvidence, type TaskProgress } from './taskProgress'
export type { TaskProgress } from './taskProgress'
import { createNormalImportApi, type NormalImportApi } from './normalImport'
import { createDirectLoadApi, type DirectLoadApi } from './directLoad'
import { createOverviewApi, type OverviewApi } from './overview'
import { createAccessControlApi, type AccessControlApi } from './accessControl'
import { createLogRetentionApi, type LogRetentionApi } from './logRetention'
import { createSchedulingPolicyApi, type SchedulingPolicyApi } from './schedulingPolicy'
import { createSchedulingTimingApi, type SchedulingTimingApi } from './schedulingTiming'
import { createSchedulingCandidatesApi, type SchedulingCandidatesApi } from './schedulingCandidates'
import { createSecurityPolicyApi, type SecurityPolicyApi } from './securityPolicy'
import { createToolCompatibilityApi, type ToolCompatibilityApi } from './toolCompatibility'
import { createAuditLogApi, type AuditLogApi } from './auditLogs'
import { createLogQueryApi, type LogQueryApi } from './logQuery'
import { invalidNodeResponse, NodeResponseError, readNodeResponse } from './nodeResponseValidation'

export type { NodeHostInventory, NodeRuntimeInspection, NodeInspectionItem, NodeActivity, NodeActivityItem, NodeRecentTasks } from './nodeRuntimeFacts'

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
  if (apiError.status === 404) return '对象目录接口不可用，或当前身份无权读取所选数据源和节点。请确认控制面已更新并检查授权。'
  if (apiError.status === 409 || apiError.status === 412) return '数据源配置已变化，请刷新页面后重新选择。'
  if (apiError.code === 'EXPORT_OBJECT_CATALOG_NODE_UNAVAILABLE') return '执行节点当前无法接收对象查询。请检查 Agent 心跳和节点状态后重试。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查服务状态后重试。'
  return apiError.message || '无法读取对象元数据，请重试。'
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
  if (error instanceof NodeResponseError) return error.message
  const apiError = (error && typeof error === 'object' ? error : {}) as Partial<ApiError>
  if (apiError.code === 'CSRF_TOKEN_UNAVAILABLE') return '当前页面未获得请求安全令牌，已拒绝写操作。请刷新页面后重试。'
  if (apiError.code === 'EXECUTION_NODE_NAME_UNAVAILABLE') return '执行节点名称不可用，请更换后重试。'
  if (apiError.code === 'EXECUTION_NODE_RUNNING_TASK') return '节点仍有运行或待核对的执行，暂不能永久删除。'
  if (apiError.status === 401) return '登录状态或请求安全校验已失效，请刷新页面后重试。'
  if (apiError.status === 404) return '执行节点不存在或当前身份无权访问。'
  if (apiError.status === 403) return '当前登录身份无权完成此节点操作，请联系管理员核对权限。'
  if (apiError.status === 409 || apiError.status === 412 || apiError.conflict) return '执行节点已发生变化，请刷新并核对最新配置后再操作。'
  if (apiError.code === 'NETWORK_UNAVAILABLE') return '无法连接控制面，请检查控制面服务与网络连接后重试。'
  const messages: Readonly<Record<string, string>> = {
    RESPONSE_INVALID: '执行节点数据缺少必要信息或格式不兼容。请刷新页面；若仍出现，请联系管理员核对网页、控制面和 Agent 版本。',
    API_DEPENDENCY_NOT_CONFIGURED: '控制面尚未配置此节点操作所需的服务，请联系管理员检查执行节点管理配置。',
    EXECUTION_NODE_QUERY_UNAVAILABLE: '控制面暂时无法读取节点记录、执行额度或任务统计。请重试；若仍失败，请联系管理员检查控制面服务与本地数据库状态。',
    EXECUTION_NODE_CREATE_UNAVAILABLE: '控制面暂时无法确认节点创建结果，请先刷新节点列表核对，再决定是否重试。',
    EXECUTION_NODE_UPDATE_UNAVAILABLE: '控制面暂时无法确认节点保存结果，请先刷新并核对节点配置，再决定是否重试。',
    EXECUTION_NODE_DELETE_UNAVAILABLE: '控制面暂时无法确认节点删除结果，请先刷新节点列表核对，再决定是否重试。',
    EXECUTION_NODE_ENABLE_UNAVAILABLE: '控制面暂时无法确认节点启用结果，请先刷新并核对节点管理状态，再决定是否重试。',
    ENVIRONMENT_CHECK_REQUEST_UNAVAILABLE: '控制面暂时无法确认环境检查请求结果，请先刷新节点查看是否已进入检查中，再决定是否重试。',
    NODE_ENABLE_PRECONDITION_FAILED: '节点尚未满足启用条件，请确认 Agent 在线，并在节点详情完成环境检查后再启用。',
    AGENT_ENROLLMENT_NOT_CONFIGURED: '控制面尚未配置 Agent 关联服务，请联系管理员完成关联服务配置后重新生成注册码。',
    AGENT_ENROLLMENT_UNAVAILABLE: '注册码暂时无法签发，请关闭弹层并刷新节点后重试；若仍失败，请联系管理员检查 Agent 关联服务。',
    EXECUTION_NODE_FIELDS_INVALID: '节点配置未通过校验，请检查表单标出的字段后重新保存。',
    REVISION_REQUIRED: '当前页面缺少有效的节点版本，请刷新节点后重新操作。',
  }
  if (apiError.code && Object.hasOwn(messages, apiError.code)) return messages[apiError.code]!
  // 未识别错误使用当前动作的固定提示，不展示服务端或程序异常的原始消息。
  return `${fallback}请刷新节点核对结果；若仍失败，请联系管理员检查控制面服务。`
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
  readonly connectionKind: 'ODP' | 'OBSERVER_DIRECT'
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
  readonly connectionKind?: 'ODP' | 'OBSERVER_DIRECT'
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
  readonly objectType: 'DATABASE' | 'ALL' | ExportObjectType
  readonly keyword: string
  readonly objects: readonly string[]
  readonly groups: readonly ExportObjectCatalogGroup[]
  readonly truncated: boolean
  readonly validUntil: string
}

export interface ExportObjectCatalogGroup {
  readonly objectType: ExportObjectType
  readonly objects: readonly string[]
  readonly truncated: boolean
  readonly unavailable: boolean
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
  readonly outcome: 'DELETED'
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

export type ExecutionNodeEnvironmentStatus = 'NOT_CHECKED' | 'NORMAL' | 'ABNORMAL' | 'EXPIRED' | 'UNCONFIRMED'

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
  readonly memoryTotalBytes?: number
  readonly memoryAvailableBytes?: number
}

export interface ExecutionNodeDataRootUsage {
  readonly root: string
  readonly totalBytes: number
  readonly availableBytes: number
}

export interface ExecutionNodeWrite {
  readonly budgetPolicy?: { readonly systemReservedBytes?: number; readonly taskOverheadBytes?: number }
  readonly displayName: string
  readonly platform: ExecutionNodePlatform
  readonly allowedRoots: readonly string[]
  readonly toolHome: string
  readonly javaPath: string
}

export interface ExecutionNodeSummary {
  readonly executionQuota?: { readonly totalGiB: number; readonly reservedGiB: number; readonly usedGiB: number }
  readonly budgetPolicy?: ExecutionNodeWrite['budgetPolicy']
  readonly defaultJvmMemory?: string
  readonly taskSummary?: { readonly scope: 'MINE' | 'ALL'; readonly running: number; readonly waiting: number; readonly reconciling: number }
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
  readonly environmentCheck?: { readonly checkId?: string; readonly status: 'NOT_CHECKED' | 'PENDING' | 'PASSED' | 'FAILED'; readonly code?: string; readonly factsRevision: number; readonly requestedAt?: string; readonly completedAt?: string; readonly source?: 'SUBJECT' | 'AGENT' | 'SYSTEM' | 'UNKNOWN'; readonly inspection?: NodeRuntimeInspection }
  readonly recentTasks?: NodeRecentTasks
  readonly activity?: NodeActivity
  readonly agentDiagnostics?: ExecutionNodeAgentDiagnostics
  readonly activeExecutions?: { readonly scope: 'READABLE' | 'ALL'; readonly items: readonly ExecutionNodeActiveExecution[] }
  readonly allowedRoots: readonly string[]
  readonly toolHome: string
  readonly javaPath: string
  readonly dataRootUsages?: readonly ExecutionNodeDataRootUsage[]
  readonly createdAt: string
}

export interface InstalledRuntimeVersion {
  readonly version: string
  readonly source: 'JAVA_RELEASE' | 'JAR_MANIFEST' | 'JAR_POM'
}

export interface ExecutionNodeLogQueue {
  readonly pendingBatches: number
  readonly pendingGaps: number
  readonly pendingBytes: number
  readonly lastUploadResult: 'UNKNOWN' | 'SUCCEEDED' | 'FAILED'
  readonly lastUploadAt?: string
}

export interface ExecutionNodeAgentDiagnostics {
  readonly host?: NodeHostInventory
  readonly agentId: string
  readonly protocolVersion: string
  readonly factsRevision: number
  readonly startedAt?: string
  readonly runtimeConfigurationDigest?: string
  readonly expectedRuntimeConfigurationDigest: string
  readonly runtimeVersions?: { readonly java?: InstalledRuntimeVersion; readonly obloader?: InstalledRuntimeVersion; readonly obdumper?: InstalledRuntimeVersion }
  readonly logQueue?: ExecutionNodeLogQueue
}

export interface ExecutionNodeActiveExecution {
  readonly taskId: string
  readonly executionId: string
  readonly leaseId: string
  readonly leaseEpoch: number
  readonly lastEventSeq: number
}

export interface ExecutionNodeEnrollment {
  readonly requestId: string
  readonly enrollmentId: string
  readonly nodeId: string
  readonly enrollmentMaterial: string
  readonly expiresAt: string
  readonly displayedOnce: true
}

export type ExportScopeKind = 'ALL' | 'SPECIFIED' | 'QUERY_RESULT'
export type ExportObjectType = 'TABLE' | 'VIEW' | 'FUNCTION' | 'PROCEDURE' | 'SEQUENCE'
export type ExportContentKind = 'DATA_ONLY' | 'DDL_ONLY' | 'DDL_AND_DATA'
// EX-I4/EX-I5：数据格式单选；CSV/CUT/POS/SQL 为文本格式，PARQUET/ORC/AVRO 为结构化格式（2026-08-07 接入）。
export type ExportDataFormatKind = 'CSV' | 'CUT' | 'POS' | 'SQL' | 'PARQUET' | 'ORC' | 'AVRO'

// StructuredFormat 判断是否结构化格式：压缩与序列化选项不适用。
export function isStructuredFormat(formatKind: ExportDataFormatKind): boolean {
  return formatKind === 'PARQUET' || formatKind === 'ORC' || formatKind === 'AVRO'
}

export interface ExportObjectExpression {
  readonly objectType?: ExportObjectType
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
  readonly queryResultLimit?: number
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

// TimestampFormatsOptions 按租户与格式表达导出值格式和 Oracle NLS 会话格式。
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
  readonly preserveZeroDatetime?: boolean
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

// DDLBehaviorOptions 只随适用的对象 DDL 携带；附加信息依赖 sys 凭据与权限预检查。
export interface DDLBehaviorOptions {
  readonly dropObject?: boolean
  readonly retainSchema?: boolean
  readonly compactSchema?: boolean
  readonly addExtraMessage?: boolean
  readonly sequencePolicy?: 'PRESERVE' | 'RESTART'
}

export interface GeneralizedExportConfig {
  readonly objectScope: ExportObjectScope
  // 空内容仅表示未完成的编辑草稿，READY 写入仍须完整校验。
  readonly contentSelection: { readonly contentKind: ExportContentKind | '' }
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
    // 仅编辑草稿保存未组成 URI 的位置字段；完整配置不得携带。
    readonly storageLocationDraft?: { readonly bucket: string; readonly path: string; readonly endpoint: string; readonly region: string }
  }
  readonly filterConfig?: FilterOptions
  readonly performanceConfig?: PerformanceOptions
  // EX-I7 DDL 行为（2026-08-10）：仅 DDL 内容时携带。
  readonly ddlBehavior?: DDLBehaviorOptions
}

// ExportDraftInput 是向导提交的当前泛化草稿请求；v5 扁平形态仅由历史兼容路径使用。
export interface ExportDraftInput {
  readonly configVersion: 'v6'
  readonly draftState?: 'EDITING' | 'READY'
  readonly dataSourceId: string
  readonly nodeId: string
  readonly config: GeneralizedExportConfig
}

export interface ExportDraft {
  readonly draftState?: 'EDITING' | 'READY'
  readonly sourceTaskId?: string
  readonly sourceDerivation?: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH'
  readonly id: string
  readonly dataSourceId: string
  readonly nodeId: string
  readonly revision: number
  readonly configVersion: 'v5' | 'v6'
  readonly config: GeneralizedExportConfig
  readonly configFingerprint: string
}

export interface ExportDraftListItem {
  readonly id: string
  readonly type: 'OBDUMPER_EXPORT'
  readonly displayName: string
  readonly dataSourceId: string
  readonly nodeId: string
  readonly revision: number
  readonly sourceTaskId?: string
  readonly sourceDerivation?: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH'
  readonly createdAt: string
  readonly updatedAt: string
  readonly availableActions: readonly ('CONTINUE_EDIT' | 'SUBMIT' | 'DELETE')[]
}
export interface ExportDraftListPage { readonly items: readonly ExportDraftListItem[]; readonly nextCursor?: string; readonly totalPages: number }

export type SupportedTaskType = 'OBDUMPER_EXPORT' | 'OBLOADER_NORMAL_IMPORT' | 'OBLOADER_DIRECT_LOAD'
export interface TaskDraftListItem extends Omit<ExportDraftListItem, 'type'> { readonly type: SupportedTaskType }
export interface TaskDraftListPage { readonly items: readonly TaskDraftListItem[]; readonly nextCursor?: string; readonly totalPages: number }

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
	readonly check: 'DATABASE_CONNECTIVITY' | 'OBJECT_ACCESS' | 'TOOL_ENVIRONMENT' | 'NODE_RESOURCE' | 'OUTPUT_PATH' | 'OUTPUT_EMPTY' | 'AVAILABLE_SPACE' | 'STORAGE_CONNECTIVITY' | 'STORAGE_AUTH' | 'STORAGE_WRITE'
	readonly status: 'PASSED' | 'FAILED' | 'UNKNOWN'
	readonly evidenceCode: string
}

export interface TaskOverview {
  readonly derivedTaskCount?: number
  readonly derivedTasks?: readonly { readonly id: string; readonly displayName: string; readonly state: string }[]
  readonly presentation?: TaskListItem
  readonly id: string
  readonly type: SupportedTaskType
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
	readonly taskType?: SupportedTaskType
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

export interface ConfigTemplateItem extends ExportConfigTemplateItem { readonly taskType: Exclude<SupportedTaskType, 'OBLOADER_DIRECT_LOAD'> }

export interface TaskSnapshot {
  readonly groups?: readonly TaskSnapshotGroup[]
  readonly type: SupportedTaskType
  readonly snapshotVersion: 'v1' | 'v2' | 'normal-import-v1' | 'direct-load-v1'
  readonly dataSourceId: string
  readonly nodeId: string
  readonly precheckId: string
  readonly objectSummary?: string
  readonly format: 'CSV' | 'CUT' | 'SQL' | 'POS' | 'PARQUET' | 'ORC' | 'AVRO' | 'DDL' | 'DDL_CSV' | 'DDL_CUT' | 'DDL_SQL'
  readonly configFingerprint: string
  readonly toolVersion: string
  readonly metadataVersion: string
  readonly capabilityVersion: string
}

export interface TaskSnapshotGroup {
  readonly key: 'source' | 'objects' | 'format' | 'output' | 'review' | 'input' | 'execution' | 'applicability' | 'connection' | 'target'
  readonly label: string
  readonly fields: readonly { readonly key: string; readonly label: string; readonly value: string; readonly source: 'UNKNOWN' }[]
}

export interface TaskCommandEvidence {
  readonly kind: 'PLANNED'
  readonly command: string
  readonly redaction: 'PASSWORD_ONLY'
}

// TaskOperationPage 只承载当前任务范围的安全操作事实，不复制审计差异或原始事件载荷。
export interface TaskOperationPage {
  readonly items: readonly TaskOperation[]
  readonly nextCursor?: string
}
export interface TaskOperation {
  readonly id: string
  readonly actorType: 'SUBJECT' | 'AGENT' | 'SYSTEM'
  readonly actorName: string
  readonly action: string
  readonly result: 'SUCCEEDED' | 'FAILED' | 'DENIED' | 'RECORDED'
  readonly occurredAt: string
}

export interface TaskExecution {
  readonly schedulingWaitReason?: 'PLATFORM_CONCURRENCY_LIMIT' | 'SCHEDULING_POLICY_UNAVAILABLE'
  readonly cancelAvailable?: boolean
  readonly cancelRequested?: boolean
  readonly cancelDeadline?: string
  readonly exitCode?: number
  readonly elapsedMs?: number
  readonly errorCode?: string
  readonly errorSummary?: string
  readonly plannedActualMatch?: 'MATCHED' | 'MISMATCHED' | 'UNAVAILABLE'
  readonly outputLocation?: { readonly kind: 'LOCAL' | 'OSS' | 'S3' | 'COS' | 'OBS' | 'UNAVAILABLE'; readonly value?: string; readonly redaction: 'FULL' | 'KIND_ONLY' | 'UNAVAILABLE' }
  readonly state: string
  readonly executionId?: string
  readonly reconciliationRequired: boolean
  readonly stageEvidence: 'UNAVAILABLE' | 'RELIABLE'
  readonly progressEvidence: 'UNAVAILABLE' | 'PERCENT'
  readonly progress?: TaskProgress
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
  readonly type: SupportedTaskType
  readonly dataSourceId: string
  readonly objectSummary?: string
  readonly state: string
  readonly stageEvidence: 'UNAVAILABLE' | 'RELIABLE'
  readonly progressEvidence: 'UNAVAILABLE' | 'PERCENT'
  readonly progress?: TaskProgress
  readonly reconciliationRequired: boolean
  readonly nodeId: string
  readonly ownedByCurrentUser: boolean
  readonly submittedAt: string
  readonly startedAt?: string
  readonly finishedAt?: string
  readonly updatedAt: string
  // 任务中心展示事实（DEC-050）：名称来自提交时冻结的展示事实，空值表示历史名称不可用。
  readonly displayName: string
  readonly dataSourceName?: string
  readonly dataSourceDeleted: boolean
  readonly environment?: TaskEnvironment
  readonly nodeName?: string
  readonly nodeDeleted: boolean
  readonly creatorName?: string
  readonly cancelRequested: boolean
  // 只是展示资格；写操作由对应接口复验。
  readonly availableActions: readonly TaskListAction[]
}

export type TaskEnvironment = 'DEVELOPMENT' | 'TEST' | 'STAGING' | 'PRODUCTION'
export const taskListActions = ['COPY_COMMAND', 'VIEW_LOGS', 'CANCEL', 'REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH', 'CHECKPOINT_RESUME', 'SAVE_TEMPLATE'] as const
export type TaskListAction = (typeof taskListActions)[number]
export const taskListStates = ['WAITING_SCHEDULE', 'STARTING', 'RUNNING', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED'] as const
export type TaskListState = (typeof taskListStates)[number]

// TaskListFilter 只收窄服务端授权范围；未传字段表示不筛选。时间为 RFC 3339 半开区间。
export interface TaskListFilter {
  readonly nodeId?: string
  readonly scope?: 'MINE' | 'ALL'
  readonly keyword?: string
  readonly types?: readonly SupportedTaskType[]
  readonly states?: readonly TaskListState[]
  readonly createdFrom?: string
  readonly createdTo?: string
  readonly startedFrom?: string
  readonly startedTo?: string
  readonly finishedFrom?: string
  readonly finishedTo?: string
  readonly dataSourceId?: string
  readonly environment?: TaskEnvironment
  readonly creatorId?: string
}

export interface TaskFilterOption {
  readonly id: string
  readonly label: string
}

// TaskFilterOptions 只来自当前授权任务；非全局身份不获得创建人候选。
export interface TaskFilterOptions {
  readonly dataSources: readonly TaskFilterOption[]
  readonly nodes: readonly TaskFilterOption[]
  readonly creators: readonly TaskFilterOption[]
  readonly environments: readonly TaskEnvironment[]
  readonly creatorFilterAvailable: boolean
}

export interface TaskCancellation {
  readonly taskId: string
  readonly state: string
  readonly cancelRequested: boolean
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

export interface BrowserApi extends NormalImportApi, DirectLoadApi, OverviewApi, AccessControlApi, LogRetentionApi, SchedulingPolicyApi, SchedulingTimingApi, SchedulingCandidatesApi, SecurityPolicyApi, ToolCompatibilityApi, LogQueryApi, AuditLogApi {
  listDataSources(): Promise<DataSourceSummary[]>
  listDataSourcePage(query: Record<string, string>): Promise<{ items: DataSourceSummary[]; nextCursor: string; total: number }>
  listExecutionNodes(options?: { forTaskSelection: boolean }): Promise<ExecutionNodeSummary[]>
  listExportNodeCandidates(): Promise<ExecutionNodeCandidate[]>
  listDataSourceConnectionTestNodeCandidates(): Promise<ExecutionNodeCandidate[]>
  getExecutionNode(nodeId: string): Promise<ExecutionNodeDetail>
  createExecutionNode(input: ExecutionNodeWrite): Promise<string>
  updateExecutionNode(nodeId: string, revision: number, input: ExecutionNodeWrite): Promise<ExecutionNodeDetail>
  issueExecutionNodeEnrollment(nodeId: string): Promise<ExecutionNodeEnrollment>
  requestExecutionNodeEnvironmentCheck(nodeId: string, revision: number): Promise<ExecutionNodeDetail>
  enableExecutionNode(nodeId: string, revision: number): Promise<ExecutionNodeDetail>
  deleteExecutionNode(nodeId: string, revision: number): Promise<ExecutionNodeDeletionResult>
  getDataSource(dataSourceId: string): Promise<DataSourceDetail>
  createDataSource(input: DataSourceWrite): Promise<string>
  updateDataSource(dataSourceId: string, revision: number, input: DataSourceUpdate): Promise<DataSourceDetail>
  changeDataSourceState(dataSourceId: string, revision: number, targetState: 'ENABLED' | 'DISABLED'): Promise<DataSourceStateChange>
	deleteDataSource(dataSourceId: string, revision: number): Promise<DataSourceDeletionResult>
	archiveDataSource(dataSourceId: string, revision: number): Promise<DataSourceDeletionResult>
  startDataSourceConnectionTest(dataSourceId: string, revision: number, nodeId: string): Promise<DataSourceConnectionTestRequest>
  getDataSourceConnectionTest(connectionTestId: string): Promise<DataSourceConnectionTest>
  searchExportObjects(dataSourceId: string, revision: number, input: { nodeId: string; database: string; objectType: 'DATABASE' | 'ALL' | ExportObjectType; keyword: string }): Promise<ExportObjectCatalogQuery>
  getExportObjectCatalogQuery(queryId: string): Promise<ExportObjectCatalogQuery>
  createExportDraft(input: ExportDraftInput): Promise<string>
  listExportDrafts(limit: TaskListPageSize, keyword?: string, cursor?: string): Promise<ExportDraftListPage>
  listTaskDrafts(limit: TaskListPageSize, keyword?: string, cursor?: string): Promise<TaskDraftListPage>
  deleteExportDraft(draftId: string, revision: number): Promise<void>
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
  listTasks(cursor?: string, limit?: TaskListPageSize, filter?: TaskListFilter, signal?: AbortSignal): Promise<TaskListPage>
  listTaskFilterOptions(signal?: AbortSignal): Promise<TaskFilterOptions>
  cancelTask(taskId: string): Promise<TaskCancellation>
  getTaskOverview(taskId: string): Promise<TaskOverview>
  getTaskOperations(taskId: string, cursor?: string): Promise<TaskOperationPage>
  // EX-I8 派生操作：从失败任务重建可编辑草稿（基于原配置新建/从头重新执行），
  // 或从失败导出任务创建检查点继续任务（继承原快照并追加 --retry）。
  rebuildTaskDraft(taskId: string, derivation: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH'): Promise<TaskDerivedDraft>
  resumeTaskFromCheckpoint(taskId: string): Promise<TaskDerivedResume>
  // EX-I8 模板复用：列表/改名/删除、从成功任务保存模板、由模板创建草稿。
  listExportConfigTemplates(): Promise<ExportConfigTemplateItem[]>
  listConfigTemplates(): Promise<ConfigTemplateItem[]>
  renameConfigTemplate(templateId: string, revision: number, displayName: string): Promise<number>
  deleteConfigTemplate(templateId: string, revision: number): Promise<void>
  createTaskDraftFromTemplate(templateId: string, dataSourceId: string, nodeId: string, taskType: Exclude<SupportedTaskType, 'OBLOADER_DIRECT_LOAD'>): Promise<{ draftId: string; taskType: Exclude<SupportedTaskType, 'OBLOADER_DIRECT_LOAD'> }>
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
  const logTransport: Parameters<typeof createLogQueryApi>[0] = {
      read: (path, signal) => request(options, path, { method: 'GET', signal }),
      write: (path, body, signal) => request(options, path, { ...writeRequest(options, body, undefined, 'POST', false), signal }),
      download: async (path, body, signal) => {
        const init = writeRequest(options, body, undefined, 'POST', false)
        let response: Response
        try { response = await options.fetcher(path, { ...init, signal, credentials: 'same-origin', cache: 'no-store' }) } catch { throw localError('NETWORK_UNAVAILABLE', '日志下载未完成，请重试。', true) }
        if (!response.ok) throw toApiError(response.status, asRecord(await response.json().catch(() => ({}))))
        if (response.headers.get('Content-Type')?.toLowerCase() !== 'text/plain; charset=utf-8' || Number(response.headers.get('Content-Length')) > 100 * 1024 * 1024) throw localError('RESPONSE_INVALID', '日志下载响应无法校验。')
        const file = await response.blob()
        if (file.size > 100 * 1024 * 1024) throw localError('RESPONSE_INVALID', '日志下载超过大小限制。')
        return file
      },
    }
  return {
    ...createSchedulingCandidatesApi({
      read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }),
      write: (path, body, revision, key, signal) => request(options, path, { ...writeRequest(options, body, revision, 'PATCH', true, key), ...(signal ? { signal } : {}) }),
    }),
    ...createSchedulingTimingApi({
      read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }),
      write: (path, body, revision, key, signal) => request(options, path, { ...writeRequest(options, body, revision, 'PATCH', true, key), ...(signal ? { signal } : {}) }),
    }),
    ...createSchedulingPolicyApi({
      read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }),
      write: (path, body, revision, key, signal) => request(options, path, { ...writeRequest(options, body, revision, 'PATCH', true, key), ...(signal ? { signal } : {}) }),
    }),
    ...createLogRetentionApi({
      read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }),
      write: (path, body, revision, key, signal) => request(options, path, { ...writeRequest(options, body, revision, 'PATCH', true, key), ...(signal ? { signal } : {}) }),
    }),
    ...createSecurityPolicyApi({ read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }) }),
    ...createToolCompatibilityApi({ read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }) }),
    ...createLogQueryApi(logTransport),
    ...createAuditLogApi(logTransport),
    ...createOverviewApi({ read: path => request(options, path, { method: 'GET' }) }),
    ...createAccessControlApi({
      read: (path, signal) => request(options, path, { method: 'GET', ...(signal ? { signal } : {}) }),
      write: (path, body, revision, key, signal) => request(options, path, { ...writeRequest(options, body, revision, 'PATCH', true, key), ...(signal ? { signal } : {}) }),
    }),
    ...createDirectLoadApi({
      read: path => request(options, path, { method: 'GET' }),
      write: (path, body, revision, method = 'POST', idempotencyKey) => request(options, path, writeRequest(options, body, revision, method, true, idempotencyKey)),
    }),
    ...createNormalImportApi({
      read: path => request(options, path, { method: 'GET' }),
      write: (path, body, revision, method = 'POST') => request(options, path, writeRequest(options, body, revision, method, true)),
    }),
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
    async listExecutionNodes(selection) {
      const body = await request(options, selection?.forTaskSelection ? '/api/v1/execution-nodes?candidateOrder=platform' : '/api/v1/execution-nodes', { method: 'GET' })
      return readNodeResponse('list', () => listOf(body, 'items').map(parseExecutionNodeSummary))
    },
    async listExportNodeCandidates() {
      const body = await request(options, '/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT', { method: 'GET' })
      return readNodeResponse('list', () => listOf(body, 'items').map(parseExecutionNodeCandidate))
    },
    async listDataSourceConnectionTestNodeCandidates() {
      const body = await request(options, '/api/v1/execution-nodes?eligibleFor=DATA_SOURCE_CONNECTION_TEST', { method: 'GET' })
      return readNodeResponse('list', () => listOf(body, 'items').map(parseExecutionNodeCandidate))
    },
    async getExecutionNode(nodeId) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}`, { method: 'GET' })
      return readNodeResponse('configuration', () => parseExecutionNodeDetail(requiredObject(body, 'item')))
    },
    async createExecutionNode(input) {
      const body = await request(options, '/api/v1/execution-nodes', writeRequest(options, input))
      return readNodeResponse('creation', () => requiredString(body, 'id'), true)
    },
    async updateExecutionNode(nodeId, revision, input) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}`, writeRequest(options, input, revision, 'PATCH', false))
      return readNodeResponse('configuration', () => parseExecutionNodeDetail(requiredObject(body, 'item')), true)
    },
    async issueExecutionNodeEnrollment(nodeId) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:enrollments`, writeEmptyPost(options))
      return readNodeResponse('enrollment', () => {
        const enrollment = parseExecutionNodeEnrollment(body)
        if (enrollment.nodeId !== nodeId) invalidNodeResponse('注册材料与当前节点不一致。')
        return enrollment
      }, true)
    },
    async requestExecutionNodeEnvironmentCheck(nodeId, revision) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:environment-check`, writeVersionedEmptyPost(options, revision))
      return readNodeResponse('configuration', () => parseExecutionNodeDetail(requiredObject(body, 'item')), true)
    },
    async enableExecutionNode(nodeId, revision) {
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:enable`, writeVersionedEmptyPost(options, revision))
      return readNodeResponse('configuration', () => parseExecutionNodeDetail(requiredObject(body, 'item')), true)
    },
    async deleteExecutionNode(nodeId, revision) {
      // 独立动作端点让旧控制面返回 404，避免 HMR 后仍把永久删除执行成归档。
      const body = await request(options, `/api/v1/execution-nodes/${encodeURIComponent(nodeId)}:delete`, writeVersionedEmptyPost(options, revision))
      return readNodeResponse('deletion', () => {
        const outcome = requiredString(body, 'outcome')
        const revisionAfterDeletion = requiredNumber(body, 'revision')
        if (outcome !== 'DELETED' || revisionAfterDeletion !== 0) invalidNodeResponse('删除结果或删除后的版本不符合约定。')
        return { outcome: 'DELETED', revision: revisionAfterDeletion, agentAccessRevoked: requiredBoolean(body, 'agentAccessRevoked') }
      }, true)
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
    async listExportDrafts(limit, keyword, cursor) {
      const query = new URLSearchParams({ limit: String(limit) })
      if (keyword) query.set('keyword', keyword)
      if (cursor) query.set('cursor', cursor)
      const body = await request(options, `/api/v1/export-drafts?${query}`, { method: 'GET' })
      return { items: listOf(body, 'items').map(parseExportDraftListItem), nextCursor: optionalString(body, 'nextCursor'), totalPages: requiredNonNegativeInteger(body, 'totalPages') }
    },
    async listTaskDrafts(limit, keyword, cursor) {
      const query = new URLSearchParams({ limit: String(limit) })
      if (keyword) query.set('keyword', keyword)
      if (cursor) query.set('cursor', cursor)
      const body = await request(options, `/api/v1/task-drafts?${query}`, { method: 'GET' })
      return { items: listOf(body, 'items').map(parseTaskDraftListItem), nextCursor: optionalString(body, 'nextCursor'), totalPages: requiredNonNegativeInteger(body, 'totalPages') }
    },
    async deleteExportDraft(draftId, revision) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draftId)}`, writeRequest(options, {}, revision, 'DELETE', true))
      if (body['deleted'] !== true) throw localError('RESPONSE_INVALID', '草稿删除结果无效。')
    },
    async getExportDraft(draftId) {
      const body = await request(options, `/api/v1/export-drafts/${encodeURIComponent(draftId)}`, { method: 'GET' })
      return parseExportDraft(requiredObject(body, 'item'))
    },
    async updateExportDraft(draft) {
      // 草稿更新必须提交完整 v6 写请求；服务端按 configVersion 路由并重新失败关闭校验。
      const input: ExportDraftInput = { configVersion: 'v6', ...(draft.draftState ? { draftState: draft.draftState } : {}), dataSourceId: draft.dataSourceId, nodeId: draft.nodeId, config: draft.config }
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
    async listTasks(cursor, limit = 10, filter, signal) {
      const query = new URLSearchParams({ limit: String(limit) })
      if (cursor) query.set('cursor', cursor)
      if (filter?.nodeId) query.set('nodeId', filter.nodeId)
      if (filter?.scope) query.set('scope', filter.scope)
      if (filter?.keyword) query.set('keyword', filter.keyword)
      if (filter?.types?.length) query.set('type', filter.types.join(','))
      if (filter?.states?.length) query.set('state', filter.states.join(','))
      for (const key of ['createdFrom', 'createdTo', 'startedFrom', 'startedTo', 'finishedFrom', 'finishedTo', 'dataSourceId', 'environment', 'creatorId'] as const) {
        const value = filter?.[key]
        if (value) query.set(key, value)
      }
      const body = await request(options, `/api/v1/tasks?${query.toString()}`, { method: 'GET', signal })
      const totalPages = requiredNumber(body, 'totalPages')
      if (!Number.isSafeInteger(totalPages) || totalPages < 0) {
        throw localError('RESPONSE_INVALID', '控制面返回了无效任务总页数。')
      }
      return { items: listOf(body, 'items').map(parseTaskListItem), nextCursor: optionalString(body, 'nextCursor'), totalPages }
    },
    async listTaskFilterOptions(signal) {
      const body = await request(options, '/api/v1/tasks/filter-options', { method: 'GET', signal })
      const optionList = (name: string) => listOf(body, name).map(value => {
        const item = asRecord(value)
        return { id: requiredString(item, 'id'), label: typeof item.label === 'string' ? item.label : '' }
      })
      const environments = listOf(body, 'environments').filter((value): value is TaskEnvironment => typeof value === 'string' && taskEnvironments.includes(value as TaskEnvironment))
      return { dataSources: optionList('dataSources'), nodes: optionList('nodes'), creators: optionList('creators'), environments, creatorFilterAvailable: requiredBoolean(body, 'creatorFilterAvailable') }
    },
    async cancelTask(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}:cancel`, writeRequest(options, {}))
      const item = requiredObject(body, 'item')
      if (requiredString(item, 'taskId') !== taskId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与请求不一致的取消回执。')
      }
      return { taskId, state: requiredString(item, 'state'), cancelRequested: requiredBoolean(item, 'cancelRequested') }
    },
    async getTaskOverview(taskId) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}`, { method: 'GET' })
      return parseTaskOverview(requiredObject(body, 'item'))
    },
    async getTaskOperations(taskId, cursor) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}/operations${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`, { method: 'GET' })
      const items = listOf(body, 'items').map(value => {
        const item = requiredObject({ item: value }, 'item')
        const actorType = requiredString(item, 'actorType')
        const result = requiredString(item, 'result')
        if (!['SUBJECT', 'AGENT', 'SYSTEM'].includes(actorType) || !['SUCCEEDED', 'FAILED', 'DENIED', 'RECORDED'].includes(result)) throw localError('RESPONSE_INVALID', '操作记录不符合安全契约。')
        return { id: requiredString(item, 'id'), actorType: actorType as TaskOperation['actorType'], actorName: requiredString(item, 'actorName'), action: requiredString(item, 'action'), result: result as TaskOperation['result'], occurredAt: requiredString(item, 'occurredAt') }
      })
      return { items, nextCursor: optionalString(body, 'nextCursor') }
    },
    async rebuildTaskDraft(taskId, derivation) {
      const body = await request(options, `/api/v1/tasks/${encodeURIComponent(taskId)}:rebuild-draft`, writeRequest(options, { derivation }))
      const draftId = requiredString(body, 'draftId')
      const sourceTaskId = requiredString(body, 'sourceTaskId')
      const responded = requiredString(body, 'derivation')
      if (responded !== derivation || sourceTaskId !== taskId) {
        throw localError('RESPONSE_INVALID', '控制面返回了与请求不一致的派生草稿。')
      }
      const taskType = optionalString(body, 'taskType') ?? 'OBDUMPER_EXPORT'
      if (taskType !== 'OBDUMPER_EXPORT' && taskType !== 'OBLOADER_NORMAL_IMPORT' && taskType !== 'OBLOADER_DIRECT_LOAD') throw localError('RESPONSE_INVALID', '派生草稿类型无效。')
      return { draftId, sourceTaskId, derivation, taskType }
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
    async listConfigTemplates() {
      const body = await request(options, '/api/v1/config-templates', { method: 'GET' })
      return listOf(body, 'items').map(raw => {
        const value = asRecord(raw), taskType = value['taskType']
        if (taskType !== 'OBDUMPER_EXPORT' && taskType !== 'OBLOADER_NORMAL_IMPORT') throw localError('RESPONSE_INVALID', '模板类型尚未支持。')
        return { ...parseExportConfigTemplateItem(value), taskType }
      })
    },
    async renameConfigTemplate(templateId, revision, displayName) {
      const body = await request(options, `/api/v1/config-templates/${encodeURIComponent(templateId)}`, writeRequest(options, { displayName }, revision, 'PATCH', false))
      return requiredNumber(body, 'revision')
    },
    async deleteConfigTemplate(templateId, revision) {
      await request(options, `/api/v1/config-templates/${encodeURIComponent(templateId)}`, writeRequest(options, {}, revision, 'DELETE', false))
    },
    async createTaskDraftFromTemplate(templateId, dataSourceId, nodeId, taskType) {
      const body = await request(options, `/api/v1/config-templates/${encodeURIComponent(templateId)}:create-draft`, writeRequest(options, { dataSourceId, nodeId }))
      if (requiredString(body, 'templateId') !== templateId || requiredString(body, 'taskType') !== taskType) throw localError('RESPONSE_INVALID', '控制面返回了与模板类型不一致的草稿。')
      return { draftId: requiredString(body, 'draftId'), taskType }
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

// 可选取消信号仅控制当前页面的 HTTP 生命周期，不取消服务端已接受的业务操作。
export function browserApi(signal?: AbortSignal): BrowserApi {
  return createBrowserApi({
    fetcher: async (input, init) => {
      const response = await fetch(input, signal ? { ...init, signal } : init)
      if (response.status === 401) window.location.assign('/login')
      return response
    },
    csrfToken: csrfTokenFromDocument,
    idempotencyKey: newIdempotencyKey,
  })
}

function writeRequest(options: BrowserApiOptions, body: unknown, revision?: number, method = 'POST', includeIdempotency = revision === undefined, idempotencyKey?: string): RequestInit {
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
    headers['Idempotency-Key'] = idempotencyKey ?? options.idempotencyKey()
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
  if (objectType !== 'DATABASE' && objectType !== 'ALL' && objectType !== 'TABLE' && objectType !== 'VIEW' && objectType !== 'FUNCTION' && objectType !== 'PROCEDURE' && objectType !== 'SEQUENCE') throw localError('RESPONSE_INVALID', '控制面返回了无效对象类型。')
  const database = objectType === 'DATABASE' ? requiredStringAllowEmpty(value, 'database') : requiredString(value, 'database')
  if (objectType === 'DATABASE' && database !== '') throw localError('RESPONSE_INVALID', '控制面返回了无效数据库查询范围。')
  const objects = requiredStringList(value, 'objects')
  if ((objectType === 'DATABASE' && objects.length > 100) || new Set(objects).size !== objects.length || objects.some((name) => !name || name.length > 256 || /[*,\r\n\0]/.test(name))) {
    throw localError('RESPONSE_INVALID', '控制面返回了无效对象目录。')
  }
  if (status !== 'SUCCEEDED' && objects.length > 0) throw localError('RESPONSE_INVALID', '未完成的对象查询包含对象名称。')
  if (objectType !== 'DATABASE' && requiredBoolean(value, 'truncated')) throw localError('RESPONSE_INVALID', '对象目录不完整，请更新执行节点后重试。')
  const rawGroups = value.groups ?? []
  if (!Array.isArray(rawGroups)) throw localError('RESPONSE_INVALID', '控制面返回的分类目录无效。')
  const types: ExportObjectType[] = ['TABLE', 'VIEW', 'FUNCTION', 'PROCEDURE', 'SEQUENCE']
  const groups = rawGroups.map((raw, index): ExportObjectCatalogGroup => {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw localError('RESPONSE_INVALID', '控制面返回的分类目录无效。')
    const row = raw as Record<string, unknown>
    const type = requiredString(row, 'objectType')
    const names = requiredStringList(row, 'objects')
    const truncated = requiredBoolean(row, 'truncated')
    const unavailable = requiredBoolean(row, 'unavailable')
    if (truncated) throw localError('RESPONSE_INVALID', '对象目录不完整，请更新执行节点后重试。')
    if (type !== types[index] || new Set(names).size !== names.length || names.some((name) => !name || name.length > 256 || /[*,\r\n\0]/.test(name)) || (unavailable && (names.length > 0 || truncated))) throw localError('RESPONSE_INVALID', '控制面返回的分类目录无效。')
    return { objectType: type, objects: names, truncated, unavailable }
  })
  if ((objectType === 'ALL' && status === 'SUCCEEDED' && (groups.length !== 5 || objects.length > 0)) || (objectType !== 'ALL' && groups.length > 0) || (status !== 'SUCCEEDED' && groups.length > 0)) throw localError('RESPONSE_INVALID', '控制面返回的分类目录无效。')
  return {
    id: requiredString(value, 'id'), status, dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: requiredString(value, 'nodeId'), database, objectType,
    keyword: optionalString(value, 'keyword') ?? '', objects, groups, truncated: requiredBoolean(value, 'truncated'),
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
  return readNodeResponse('summary', () => {
    const node = asRecord(value)
    const platform = parseExecutionNodePlatform(node)
    return { id: requiredString(node, 'id'), displayName: requiredString(node, 'displayName'), platform }
  })
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
  return readNodeResponse('summary', () => {
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
      invalidNodeResponse('管理状态不在支持范围内。')
    }
    if (typeof acceptsNewTasks !== 'boolean') {
      invalidNodeResponse('是否接收新任务的结论缺失，或格式不正确。')
    }
    if (agentAssociationStatus === 'PENDING' && (heartbeatStatus !== 'NEVER_CONNECTED' || lastHeartbeatAt !== null || agentFacts)) {
      invalidNodeResponse('Agent 尚未关联，却同时返回心跳或资源采样。')
    }
    if (heartbeatStatus === 'NEVER_CONNECTED' && lastHeartbeatAt !== null) {
      invalidNodeResponse('节点标记为从未连接，却同时返回最近心跳时间。')
    }
    if (acceptsNewTasks && (managementState !== 'ENABLED' || agentAssociationStatus !== 'ASSOCIATED' || heartbeatStatus !== 'ONLINE' || environmentStatus !== 'NORMAL' || capacityStatus !== 'AVAILABLE' || unavailableReasons.length > 0)) {
      invalidNodeResponse('接收新任务的结论与管理、心跳、环境、槽位或阻断原因不一致。')
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
      ...(node.taskSummary !== undefined ? { taskSummary: parseNodeTaskSummary(node.taskSummary) } : {}),
      ...(node.executionQuota !== undefined ? { executionQuota: parseNodeExecutionQuota(node.executionQuota) } : {}),
      ...(node.budgetPolicy !== undefined ? { budgetPolicy: parseNodeBudgetPolicy(node.budgetPolicy) } : {}),
      ...(node.defaultJvmMemory !== undefined ? { defaultJvmMemory: requiredString(node, 'defaultJvmMemory') } : {}),
      revision: requiredNumber(node, 'revision'),
      updatedAt: requiredString(node, 'updatedAt'),
    }
  })
}

function parseNodeTaskSummary(value: unknown) {
  return readNodeResponse('taskSummary', () => {
    const summary = asRecord(value)
    const result = { running: requiredNumber(summary, 'running'), waiting: requiredNumber(summary, 'waiting'), reconciling: requiredNumber(summary, 'reconciling') }
    if (Object.values(result).some(count => !Number.isSafeInteger(count) || count < 0)) invalidNodeResponse('任务数量必须是精度可靠的非负整数。')
    if (summary.scope !== undefined && summary.scope !== 'MINE' && summary.scope !== 'ALL') invalidNodeResponse('任务统计范围不在支持范围内。')
    return { ...result, scope: summary.scope === 'ALL' ? 'ALL' as const : 'MINE' as const }
  })
}

function parseNodeExecutionQuota(value: unknown) {
  return readNodeResponse('quota', () => {
    const quota = asRecord(value)
    const result = { totalGiB: requiredNumber(quota, 'totalGiB'), reservedGiB: requiredNumber(quota, 'reservedGiB'), usedGiB: requiredNumber(quota, 'usedGiB') }
    if (![result.totalGiB, result.reservedGiB, result.usedGiB].every(Number.isFinite) || result.totalGiB <= 0 || result.reservedGiB < 0 || result.usedGiB < 0 || result.reservedGiB + result.usedGiB > result.totalGiB) invalidNodeResponse('额度总量必须大于零，且预留与已用量之和不能超过总量。')
    return result
  })
}

function parseNodeBudgetPolicy(value: unknown) {
  return readNodeResponse('budgetPolicy', () => {
    const policy = asRecord(value)
    const result = { systemReservedBytes: policy.systemReservedBytes === undefined ? 0 : requiredNonNegativeInteger(policy, 'systemReservedBytes'), taskOverheadBytes: policy.taskOverheadBytes === undefined ? 0 : requiredNonNegativeInteger(policy, 'taskOverheadBytes') }
    if (!Object.values(result).every(Number.isSafeInteger)) invalidNodeResponse('系统预留或每任务额外预留不是精度可靠的非负字节数。')
    return result
  })
}

function parseExecutionNodeAgentAssociationStatus(value: Record<string, unknown>): ExecutionNodeAgentAssociationStatus {
  return readNodeResponse('summary', () => {
    const status = requiredString(value, 'agentAssociationStatus')
    if (status !== 'PENDING' && status !== 'ASSOCIATED') {
      invalidNodeResponse('Agent 关联状态不在支持范围内。')
    }
    return status
  })
}

function parseExecutionNodeHeartbeatStatus(value: Record<string, unknown>): ExecutionNodeHeartbeatStatus {
  return readNodeResponse('summary', () => {
    const status = requiredString(value, 'heartbeatStatus')
    if (status !== 'NEVER_CONNECTED' && status !== 'ONLINE' && status !== 'OFFLINE') {
      invalidNodeResponse('Agent 心跳状态不在支持范围内。')
    }
    return status
  })
}

function parseExecutionNodeEnvironmentStatus(value: Record<string, unknown>): ExecutionNodeEnvironmentStatus {
  return readNodeResponse('summary', () => {
    const status = requiredString(value, 'environmentStatus')
    if (status !== 'NOT_CHECKED' && status !== 'NORMAL' && status !== 'ABNORMAL' && status !== 'EXPIRED' && status !== 'UNCONFIRMED') {
      invalidNodeResponse('环境状态不在支持范围内。')
    }
    return status
  })
}

function parseExecutionNodeCapacityStatus(value: Record<string, unknown>): ExecutionNodeCapacityStatus {
  return readNodeResponse('summary', () => {
    const status = requiredString(value, 'capacityStatus')
    if (status !== 'UNKNOWN' && status !== 'AVAILABLE' && status !== 'BUSY') {
      invalidNodeResponse('任务槽位状态不在支持范围内。')
    }
    return status
  })
}

function parseExecutionNodeAgentFacts(value: unknown): ExecutionNodeAgentFacts | undefined {
  return readNodeResponse('agentFacts', () => {
    if (value === undefined || value === null) return undefined
    const facts = asRecord(value)
    const capacityTotal = requiredNonNegativeInteger(facts, 'capacityTotal')
    const capacityUsed = requiredNonNegativeInteger(facts, 'capacityUsed')
    if (capacityUsed > capacityTotal) {
      invalidNodeResponse('已用任务槽位超过了总槽位数。')
    }
    const memoryTotalBytes = facts.memoryTotalBytes === undefined ? undefined : requiredNumber(facts, 'memoryTotalBytes')
    const memoryAvailableBytes = facts.memoryAvailableBytes === undefined ? undefined : requiredNumber(facts, 'memoryAvailableBytes')
    // 旧服务可省略整对采样；部分缺失、越界或精度不可靠时拒绝投影，不能推算容量。
    if ((memoryTotalBytes !== undefined || memoryAvailableBytes !== undefined) &&
      (typeof memoryTotalBytes !== 'number' || typeof memoryAvailableBytes !== 'number' ||
        !Number.isSafeInteger(memoryTotalBytes) || !Number.isSafeInteger(memoryAvailableBytes) ||
        memoryTotalBytes <= 0 || memoryAvailableBytes < 0 || memoryAvailableBytes > memoryTotalBytes)) {
      invalidNodeResponse('物理内存总量与可用量缺项、超出可靠范围，或可用量大于总量。')
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
      memoryTotalBytes,
      memoryAvailableBytes,
    }
  })
}

function parseExecutionNodeDetail(value: Record<string, unknown>): ExecutionNodeDetail {
  const summary = parseExecutionNodeSummary(value)
  return {
    ...summary,
    environmentCheck: parseNodeEnvironmentCheck(value.environmentCheck),
    recentTasks: parseNodeRecentTasks(value.recentTasks),
    activity: parseNodeActivity(value.activity),
    agentDiagnostics: parseNodeAgentDiagnostics(value.agentDiagnostics, summary.agentFacts?.observedAt),
    activeExecutions: parseNodeActiveExecutions(value.activeExecutions),
    allowedRoots: requiredStringList(value, 'allowedRoots'),
    toolHome: requiredString(value, 'toolHome'),
    javaPath: requiredString(value, 'javaPath'),
    dataRootUsages: optionalExecutionNodeDataRootUsages(value, 'dataRootUsages'),
    createdAt: requiredString(value, 'createdAt'),
  }
}

const nodeDiagnosticLabels: Readonly<Record<string, string>> = {
  factsRevision: '绑定事实版本', leaseEpoch: '执行租约版本', lastEventSeq: '最近事件序号',
  pendingBatches: '待上传日志批次数', pendingGaps: '待补传日志缺口数', pendingBytes: '待上传字节数',
  checkId: '检查标识', agentId: 'Agent 标识', taskId: '任务标识', executionId: '执行标识', leaseId: '租约标识',
  startedAt: 'Agent 启动时间', requestedAt: '检查请求时间', completedAt: '检查完成时间', lastUploadAt: '最近上传时间',
}

function nodeDiagnosticInteger(value: Record<string, unknown>, field: string, maximum = Number.MAX_SAFE_INTEGER): number {
  const number = value[field]
  if (typeof number !== 'number' || !Number.isSafeInteger(number) || number < 0 || number > maximum) invalidNodeResponse(`${nodeDiagnosticLabels[field] ?? '诊断计数'}缺失、为负数、超过上限或超出可靠整数范围。`)
  return number
}

function nodeDiagnosticTime(value: Record<string, unknown>, field: string): string | undefined {
  if (value[field] === undefined) return undefined
  const time = value[field]
  if (typeof time !== 'string' || !/^\d{4}-\d{2}-\d{2}T/.test(time) || !Number.isFinite(Date.parse(time))) invalidNodeResponse(`${nodeDiagnosticLabels[field] ?? '诊断时间'}的格式不正确。`)
  return time
}

function nodeDiagnosticID(value: Record<string, unknown>, field: string): string {
  const id = value[field]
  if (typeof id !== 'string' || !/^[0-9A-Za-z][0-9A-Za-z_-]{0,255}$/.test(id)) invalidNodeResponse(`${nodeDiagnosticLabels[field] ?? '诊断记录标识'}缺失，或格式不正确。`)
  return id
}

function parseNodeEnvironmentCheck(value: unknown): ExecutionNodeDetail['environmentCheck'] {
  return readNodeResponse('environmentCheck', () => {
    if (value === undefined) return undefined
    const check = asRecord(value)
    const status = requiredString(check, 'status')
    if (status !== 'NOT_CHECKED' && status !== 'PENDING' && status !== 'PASSED' && status !== 'FAILED') invalidNodeResponse('环境检查状态不在支持范围内。')
    const code = check.code === undefined ? undefined : requiredString(check, 'code')
    if (code !== undefined && !['TOOL_RUNTIME_READY', 'TOOL_RUNTIME_INVALID', 'TOOL_RUNTIME_UNAVAILABLE'].includes(code)) invalidNodeResponse('存在当前页面尚未支持的环境检查结果码。')
    const factsRevision = nodeDiagnosticInteger(check, 'factsRevision')
    const requestedAt = nodeDiagnosticTime(check, 'requestedAt')
    const completedAt = nodeDiagnosticTime(check, 'completedAt')
    if (completedAt && requestedAt && Date.parse(completedAt) < Date.parse(requestedAt)) invalidNodeResponse('检查完成时间早于请求时间。')
    if (status === 'PASSED' && (code !== 'TOOL_RUNTIME_READY' || factsRevision < 1 || !completedAt)) invalidNodeResponse('检查标记为通过，但缺少成功结果、绑定事实版本或完成时间。')
    const source = check.source === undefined ? undefined : requiredString(check, 'source') as 'SUBJECT' | 'AGENT' | 'SYSTEM' | 'UNKNOWN'
    if (source !== undefined && !['SUBJECT', 'AGENT', 'SYSTEM', 'UNKNOWN'].includes(source)) invalidNodeResponse('环境检查来源不在支持范围内。')
    return { checkId: check.checkId === undefined ? undefined : nodeDiagnosticID(check, 'checkId'), status, code, factsRevision, requestedAt, completedAt, source, inspection: parseNodeRuntimeInspection(check.inspection) }
  })
}

function parseInstalledRuntimeVersion(value: unknown, java: boolean): InstalledRuntimeVersion | undefined {
  return readNodeResponse('runtimeVersions', () => {
    if (value === undefined) return undefined
    const item = asRecord(value)
    const version = requiredString(item, 'version')
    const source = requiredString(item, 'source')
    if (!/^[0-9][0-9A-Za-z._+-]{0,63}$/.test(version) || (java ? source !== 'JAVA_RELEASE' : source !== 'JAR_MANIFEST' && source !== 'JAR_POM')) invalidNodeResponse('安装版本格式不正确，或版本来源与运行时类型不匹配。')
    return { version, source: source as InstalledRuntimeVersion['source'] }
  })
}

function parseNodeAgentDiagnostics(value: unknown, observedAt: string | undefined): ExecutionNodeAgentDiagnostics | undefined {
  return readNodeResponse('diagnostics', () => {
    if (value === undefined) return undefined
    const item = asRecord(value)
    const digest = (field: string, optional = false) => {
      if (optional && item[field] === undefined) return undefined
      const value = requiredString(item, field)
      if (!/^[a-f0-9]{64}$/.test(value)) invalidNodeResponse('运行配置摘要的格式不正确，无法核对配置是否一致。')
      return value
    }
    const versions = item.runtimeVersions === undefined ? undefined : asRecord(item.runtimeVersions)
    const startedAt = nodeDiagnosticTime(item, 'startedAt')
    if (startedAt && observedAt && Date.parse(startedAt) > Date.parse(observedAt)) invalidNodeResponse('Agent 启动时间晚于本次资源采样时间。')
    const protocolVersion = requiredString(item, 'protocolVersion')
    if (!/^[0-9A-Za-z._-]{1,64}$/.test(protocolVersion)) invalidNodeResponse('Agent 协议版本缺失或格式不正确。')
    return {
      agentId: nodeDiagnosticID(item, 'agentId'), protocolVersion, factsRevision: nodeDiagnosticInteger(item, 'factsRevision'), startedAt,
      host: parseNodeHostInventory(item.host),
      runtimeConfigurationDigest: digest('runtimeConfigurationDigest', true), expectedRuntimeConfigurationDigest: digest('expectedRuntimeConfigurationDigest')!,
      runtimeVersions: versions ? { java: parseInstalledRuntimeVersion(versions.java, true), obloader: parseInstalledRuntimeVersion(versions.obloader, false), obdumper: parseInstalledRuntimeVersion(versions.obdumper, false) } : undefined,
      logQueue: parseNodeLogQueue(item.logQueue, observedAt, startedAt),
    }
  })
}

function parseNodeLogQueue(value: unknown, observedAt: string | undefined, startedAt: string | undefined): ExecutionNodeLogQueue | undefined {
  return readNodeResponse('logQueue', () => {
    if (value === undefined) return undefined
    const item = asRecord(value)
    const lastUploadResult = requiredString(item, 'lastUploadResult')
    const lastUploadAt = nodeDiagnosticTime(item, 'lastUploadAt')
    if (lastUploadResult !== 'UNKNOWN' && lastUploadResult !== 'SUCCEEDED' && lastUploadResult !== 'FAILED') invalidNodeResponse('存在当前页面尚未支持的上传结果。')
    if (lastUploadResult === 'UNKNOWN' ? lastUploadAt !== undefined : lastUploadAt === undefined) invalidNodeResponse('上传结果与上传时间缺项或互相矛盾。')
    if (lastUploadAt && observedAt && Date.parse(lastUploadAt) > Date.parse(observedAt)) invalidNodeResponse('最近上传时间晚于本次资源采样时间。')
    if (lastUploadAt && startedAt && Date.parse(lastUploadAt) < Date.parse(startedAt)) invalidNodeResponse('最近上传时间早于当前 Agent 的启动时间。')
    return { pendingBatches: nodeDiagnosticInteger(item, 'pendingBatches', 2 ** 30), pendingGaps: nodeDiagnosticInteger(item, 'pendingGaps', 2 ** 30), pendingBytes: nodeDiagnosticInteger(item, 'pendingBytes', 2 ** 30 + 16 * 2 ** 20), lastUploadResult, lastUploadAt }
  })
}

function parseNodeActiveExecutions(value: unknown): ExecutionNodeDetail['activeExecutions'] {
  return readNodeResponse('activeExecutions', () => {
    if (value === undefined) return undefined
    const item = asRecord(value)
    const scope = requiredString(item, 'scope')
    if (scope !== 'READABLE' && scope !== 'ALL') invalidNodeResponse('活动任务的读取范围不在支持范围内。')
    if (!Array.isArray(item.items) || item.items.length > 1024) invalidNodeResponse('活动任务列表缺失，或超过允许的数量上限。')
    const seen = new Set<string>()
    const items = item.items.map(value => {
      const item = asRecord(value)
      const executionId = nodeDiagnosticID(item, 'executionId')
      const leaseEpoch = nodeDiagnosticInteger(item, 'leaseEpoch')
      if (seen.has(executionId)) invalidNodeResponse('同一活动任务重复出现，无法确认执行状态。')
      if (leaseEpoch < 1) invalidNodeResponse('活动任务的执行租约版本无效。')
      seen.add(executionId)
      return { taskId: nodeDiagnosticID(item, 'taskId'), executionId, leaseId: nodeDiagnosticID(item, 'leaseId'), leaseEpoch, lastEventSeq: nodeDiagnosticInteger(item, 'lastEventSeq') }
    })
    return { scope, items }
  })
}

function optionalExecutionNodeDataRootUsages(value: Record<string, unknown>, field: string): readonly ExecutionNodeDataRootUsage[] | undefined {
  return readNodeResponse('dataRootUsages', () => {
    const raw = value[field]
    if (raw === undefined) return undefined
    if (!Array.isArray(raw)) invalidNodeResponse('目录空间列表格式不正确，或容量不可靠、可用空间大于总量。')
    return raw.map((item) => {
      const usage = asRecord(item)
      const totalBytes = requiredNumber(usage, 'totalBytes')
      const availableBytes = requiredNumber(usage, 'availableBytes')
      if (!Number.isSafeInteger(totalBytes) || !Number.isSafeInteger(availableBytes) || totalBytes < 0 || availableBytes < 0 || availableBytes > totalBytes) {
        invalidNodeResponse('目录空间列表格式不正确，或容量不可靠、可用空间大于总量。')
      }
      return { root: requiredString(usage, 'root'), totalBytes, availableBytes }
    })
  })
}

function parseExecutionNodeEnrollment(value: Record<string, unknown>): ExecutionNodeEnrollment {
  return readNodeResponse('enrollment', () => {
    if (value.displayedOnce !== true) {
      invalidNodeResponse('控制面未确认注册材料只能展示一次，页面已拒绝保留。')
    }
    return {
      requestId: requiredString(value, 'requestId'),
      enrollmentId: requiredString(value, 'enrollmentId'),
      nodeId: requiredString(value, 'nodeId'),
      enrollmentMaterial: requiredString(value, 'enrollmentMaterial'),
      expiresAt: requiredString(value, 'expiresAt'),
      displayedOnce: true,
    }
  })
}

function parseExecutionNodePlatform(value: Record<string, unknown>): ExecutionNodePlatform {
  return readNodeResponse('platform', () => {
    const platform = requiredString(value, 'platform')
    if (platform !== 'WINDOWS_AMD64' && platform !== 'LINUX_AMD64' && platform !== 'LINUX_ARM64') {
      invalidNodeResponse('操作系统与 CPU 架构组合不在支持范围内。')
    }
    return platform
  })
}

function parseExportDraftListItem(raw: unknown): ExportDraftListItem {
  const item = parseTaskDraftListItem(raw)
  if (item.type !== 'OBDUMPER_EXPORT') throw localError('RESPONSE_INVALID', '导出草稿类型无效。')
  return { ...item, type: 'OBDUMPER_EXPORT' }
}

function parseTaskDraftListItem(raw: unknown): TaskDraftListItem {
  const value = asRecord(raw)
  const type = value['type']
  if (type !== 'OBDUMPER_EXPORT' && type !== 'OBLOADER_NORMAL_IMPORT' && type !== 'OBLOADER_DIRECT_LOAD') throw localError('RESPONSE_INVALID', '草稿类型尚未支持。')
  const availableActions = listOf(value, 'availableActions').map(action => {
    if (action !== 'CONTINUE_EDIT' && action !== 'SUBMIT' && action !== 'DELETE') throw localError('RESPONSE_INVALID', '草稿操作资格无效。')
    return action
  })
  const revision = requiredNumber(value, 'revision')
  if (!Number.isSafeInteger(revision) || revision < 1) throw localError('RESPONSE_INVALID', '草稿版本无效。')
  const sourceDerivation = optionalString(value, 'sourceDerivation')
  if (sourceDerivation !== undefined && sourceDerivation !== 'REBUILD_FROM_CONFIG' && sourceDerivation !== 'RERUN_FROM_SCRATCH') throw localError('RESPONSE_INVALID', '草稿来源无效。')
  return { id: requiredString(value, 'id'), type, displayName: requiredString(value, 'displayName'), dataSourceId: requiredString(value, 'dataSourceId'), nodeId: requiredStringAllowEmpty(value, 'nodeId'), revision, sourceTaskId: optionalString(value, 'sourceTaskId'), sourceDerivation, createdAt: requiredString(value, 'createdAt'), updatedAt: requiredString(value, 'updatedAt'), availableActions }
}

function parseExportDraft(value: Record<string, unknown>): ExportDraft {
  const configVersion = requiredString(value, 'configVersion')
  const draftState = value['draftState']
  if (draftState !== undefined && draftState !== 'EDITING' && draftState !== 'READY' || draftState === 'EDITING' && configVersion !== 'v6') throw localError('RESPONSE_INVALID', '导出草稿阶段无效。')
  if ((draftState === 'EDITING') !== (value['capabilityVersion'] === 'export-draft-edit-v1')) throw localError('RESPONSE_INVALID', '导出草稿能力标记不一致。')
  if (configVersion !== 'v5' && configVersion !== 'v6') {
    throw localError('RESPONSE_INVALID', '控制面返回了未知的草稿配置版本。')
  }
  const sourceTaskId = optionalString(value, 'sourceTaskId')
  const sourceDerivation = optionalString(value, 'sourceDerivation')
  if (sourceDerivation !== undefined && sourceDerivation !== 'REBUILD_FROM_CONFIG' && sourceDerivation !== 'RERUN_FROM_SCRATCH') throw localError('RESPONSE_INVALID', '草稿来源无效。')
  if ((sourceTaskId === undefined) !== (sourceDerivation === undefined)) throw localError('RESPONSE_INVALID', '草稿来源不一致。')
  return {
    ...(draftState ? { draftState } : {}),
    ...(sourceTaskId && sourceDerivation ? { sourceTaskId, sourceDerivation } : {}),
    id: requiredString(value, 'id'),
    dataSourceId: requiredString(value, 'dataSourceId'),
    nodeId: draftState === 'EDITING' ? draftEditingString(value, 'nodeId') : requiredString(value, 'nodeId'),
    revision: requiredNumber(value, 'revision'),
    configVersion,
    config: parseDraftInput(requiredObject(value, 'config'), draftState === 'EDITING'),
    configFingerprint: requiredString(value, 'configFingerprint'),
  }
}

// parseDraftInput 解析 v6 标准文档：嵌套泛化配置位于 config 键内，
// 顶层扁平投影键（database/scopeKind/table/contentKind/format/filePath 等）只用于展示，不参与输入重建。
// Go 对 nil 切片序列化为 null，可选数组字段的 null 与缺省等价。
function parseDraftInput(value: Record<string, unknown>, editing = false): GeneralizedExportConfig {
  if (editing && value['draftState'] !== 'EDITING') throw localError('RESPONSE_INVALID', '导出编辑草稿标记不一致。')
  const nested = requiredObject(value, 'config')
  const scope = requiredObject(nested, 'objectScope')
  const scopeKind = requiredString(scope, 'scopeKind')
  if (scopeKind !== 'ALL' && scopeKind !== 'SPECIFIED' && scopeKind !== 'QUERY_RESULT') {
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
      const objectType = expression['objectType']
      if (typeof name !== 'string' || !name) throw localError('RESPONSE_INVALID', '控制面返回了无效对象名称。')
      if (schema !== undefined && typeof schema !== 'string') throw localError('RESPONSE_INVALID', '控制面返回了无效对象 Schema。')
      if (objectType !== undefined && objectType !== 'TABLE' && objectType !== 'VIEW' && objectType !== 'FUNCTION' && objectType !== 'PROCEDURE' && objectType !== 'SEQUENCE') throw localError('RESPONSE_INVALID', '控制面返回了无效表达式类型。')
      expressions.push({ name, ...(schema === undefined ? {} : { schema }), ...(objectType === undefined ? {} : { objectType }) })
    }
  }
  const objectTypesRaw = scope['objectTypes']
  const objectTypes: ExportObjectType[] = []
  if (objectTypesRaw !== undefined && objectTypesRaw !== null) {
    if (!Array.isArray(objectTypesRaw)) throw localError('RESPONSE_INVALID', '控制面返回了无效对象类型。')
    for (const item of objectTypesRaw) {
      if (item !== 'TABLE' && item !== 'VIEW' && item !== 'FUNCTION' && item !== 'PROCEDURE' && item !== 'SEQUENCE') throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的对象类型。')
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
  const contentKind = editing ? draftEditingString(contentSelection, 'contentKind') : requiredString(contentSelection, 'contentKind')
  if (!(editing && contentKind === '') && contentKind !== 'DATA_ONLY' && contentKind !== 'DDL_ONLY' && contentKind !== 'DDL_AND_DATA') {
    throw localError('RESPONSE_INVALID', '控制面返回了未知的导出内容。')
  }
  let dataFormat: GeneralizedExportConfig['dataFormat']
  const dataFormatRaw = nested['dataFormat']
  if (dataFormatRaw !== undefined && dataFormatRaw !== null) {
    const dataFormatObject = requiredObject(nested, 'dataFormat')
    const formatKind = dataFormatObject['formatKind']
    // 仅 DDL 场景服务端会把 formatKind 序列化为空字符串，等价于未选择数据格式。
    if (formatKind !== undefined && formatKind !== null && formatKind !== '') {
      if (formatKind !== 'CSV' && formatKind !== 'CUT' && formatKind !== 'POS' && formatKind !== 'SQL' && formatKind !== 'PARQUET' && formatKind !== 'ORC' && formatKind !== 'AVRO') throw localError('UNSUPPORTED_DRAFT_FORMAT', '当前页面不支持该数据格式。')
      dataFormat = { formatKind, csvOptions: parseCsvOptions(dataFormatObject), cutOptions: parseCutOptions(dataFormatObject), timestampFormats: parseTimestampFormats(dataFormatObject) }
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
      database: editing ? draftEditingString(scope, 'database', true) : requiredString(scope, 'database'),
      scopeKind,
      objectTypes: objectTypes.length > 0 ? objectTypes : undefined,
      expressions: expressions.length > 0 ? expressions : undefined,
      excludeTables: excludeTables.length > 0 ? excludeTables : undefined,
    },
    contentSelection: { contentKind },
    dataFormat,
    outputConfig: {
      outputKind,
      filePath: editing ? draftEditingString(outputConfig, 'filePath') : requiredString(outputConfig, 'filePath'),
      logPath: optionalDraftString(outputConfig, 'logPath'),
      skipCheckDir: optionalDraftBoolean(outputConfig, 'skipCheckDir'),
      noNestedDir: optionalBoolean(outputConfig, 'noNestedDir'),
      maxFileSize: optionalNumber(outputConfig, 'maxFileSize'),
      retainEmptyFiles: optionalBoolean(outputConfig, 'retainEmptyFiles'),
      compress: optionalBoolean(outputConfig, 'compress'),
      compressionAlgo: parseCompressionAlgo(outputConfig),
      compressionLevel: optionalNumber(outputConfig, 'compressionLevel'),
      // EX-I4 POS 定版：控制文件目录（--ctl-path），仅 POS 格式携带。
      controlFilePath: optionalDraftString(outputConfig, 'controlFilePath'),
      // EX-I6 对象存储：Multipart 本地临时分块目录（--tmp-path）。
      tmpPath: optionalDraftString(outputConfig, 'tmpPath'),
      // EX-I6 存储凭据槽位：对象存储输出的凭据引用往返；LOCAL 输出不携带。
      storageCredential: parseStorageCredentialReference(outputConfig),
      storageLocationDraft: parseExportStorageLocationDraft(outputConfig, editing),
    },
    filterConfig: parseFilterConfig(nested),
    performanceConfig: parsePerformanceConfig(nested),
    ddlBehavior: parseDDLBehavior(nested),
  }
}

// 编辑草稿只允许明确的空字符串；类型错误不能被解释为未填写。
function draftEditingString(value: Record<string, unknown>, key: string, allowMissing = false): string {
  if (allowMissing && value[key] === undefined) return ''
  if (typeof value[key] !== 'string') throw localError('RESPONSE_INVALID', '导出编辑草稿字段类型无效。')
  return value[key]
}

function parseExportStorageLocationDraft(value: Record<string, unknown>, editing: boolean): GeneralizedExportConfig['outputConfig']['storageLocationDraft'] {
  if (value['storageLocationDraft'] === undefined || value['storageLocationDraft'] === null) return undefined
  if (!editing) throw localError('RESPONSE_INVALID', '完整导出配置包含未完成位置。')
  const location = requiredObject(value, 'storageLocationDraft')
  if (Object.keys(location).some(key => !['bucket', 'path', 'endpoint', 'region'].includes(key))) throw localError('RESPONSE_INVALID', '导出存储位置包含未知字段。')
  return { bucket: draftEditingString(location, 'bucket'), path: draftEditingString(location, 'path'), endpoint: draftEditingString(location, 'endpoint'), region: draftEditingString(location, 'region') }
}

// parseTimestampFormats 按固定字段读取值格式与 NLS 配置，禁止草稿恢复静默丢失已保存选项。
function parseTimestampFormats(dataFormat: Record<string, unknown>): TimestampFormatsOptions | undefined {
  const raw = dataFormat['timestampFormats']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const formats: TimestampFormatsOptions = {
    dateValueFormat: optionalOptionText(options, 'dateValueFormat'),
    timeValueFormat: optionalOptionText(options, 'timeValueFormat'),
    datetimeValueFormat: optionalOptionText(options, 'datetimeValueFormat'),
    timestampValueFormat: optionalOptionText(options, 'timestampValueFormat'),
    timestampTzValueFormat: optionalOptionText(options, 'timestampTzValueFormat'),
    timestampLtzValueFormat: optionalOptionText(options, 'timestampLtzValueFormat'),
    nlsDateFormat: optionalOptionText(options, 'nlsDateFormat'),
    nlsTimestampFormat: optionalOptionText(options, 'nlsTimestampFormat'),
    nlsTimestampTzFormat: optionalOptionText(options, 'nlsTimestampTzFormat'),
    preserveZeroDatetime: optionalBoolean(options, 'preserveZeroDatetime'),
  }
  return Object.values(formats).every(value => value === undefined) ? undefined : formats
}

// parseDDLBehavior 仅保留公开 DDL 参数；未知序列策略失败关闭，不读取内部 sys 槽位。
function parseDDLBehavior(nested: Record<string, unknown>): DDLBehaviorOptions | undefined {
  const raw = nested['ddlBehavior']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const policy = optionalOptionText(options, 'sequencePolicy')
  if (policy !== undefined && policy !== 'PRESERVE' && policy !== 'RESTART') throw localError('RESPONSE_INVALID', '控制面返回了无效的序列策略。')
  const ddl: DDLBehaviorOptions = { dropObject: optionalBoolean(options, 'dropObject'), retainSchema: optionalBoolean(options, 'retainSchema'), compactSchema: optionalBoolean(options, 'compactSchema'), addExtraMessage: optionalBoolean(options, 'addExtraMessage'), sequencePolicy: policy }
  return Object.values(ddl).every(value => value === undefined) ? undefined : ddl
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

// parseFilterConfig 按固定字段解析筛选及读取选项；缺省或 null 视为未设置。
function parseFilterConfig(nested: Record<string, unknown>): FilterOptions | undefined {
  const raw = nested['filterConfig']
  if (raw === undefined || raw === null) return undefined
  const options = asRecord(raw)
  const querySql = optionalOptionText(options, 'querySql')
  const queryResultLimit = optionalNumber(options, 'queryResultLimit')
  const includeColumnNames = optionalStringList(options, 'includeColumnNames')
  const excludeColumnNames = optionalStringList(options, 'excludeColumnNames')
  const filter: FilterOptions = {
    querySql,
    queryResultLimit,
    includeColumnNames,
    excludeColumnNames,
    excludeVirtualColumns: optionalBoolean(options, 'excludeVirtualColumns'),
    flashbackScn: optionalNumber(options, 'flashbackScn'),
    flashbackTimestamp: optionalOptionText(options, 'flashbackTimestamp'),
    where: optionalOptionText(options, 'where'),
    partition: optionalOptionText(options, 'partition'),
    excludeDataTypes: optionalStringList(options, 'excludeDataTypes'),
    snapshot: optionalBoolean(options, 'snapshot'),
    weakRead: optionalBoolean(options, 'weakRead'),
    enableHiddenPk: optionalBoolean(options, 'enableHiddenPk'),
  }
  return Object.values(filter).every(value => value === undefined) ? undefined : filter
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
  const blockSize = optionalOptionText(options, 'blockSize')
  if (thread === undefined && pageSize === undefined && parallelMacro === undefined && fetchSize === undefined && jvmMemory === undefined && blockSize === undefined) {
    return undefined
  }
  return { thread, pageSize, parallelMacro, fetchSize, jvmMemory, blockSize }
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
  // 当前报告严格使用七项或九项，历史六项只保留读取；提交资格由业务清单与服务端再次复核。
  const localChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] as const
  const storageChecks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'OUTPUT_PATH', 'AVAILABLE_SPACE', 'STORAGE_CONNECTIVITY', 'STORAGE_AUTH', 'STORAGE_WRITE'] as const
  const legacyLocal = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'] as const
  const legacyStorage = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'AVAILABLE_SPACE', 'STORAGE_CONNECTIVITY', 'STORAGE_AUTH'] as const
  const knownChecks = [...localChecks, ...storageChecks] as const
  if (!Array.isArray(raw) || ![6, 7, 9].includes(raw.length)) throw localError('RESPONSE_INVALID', '控制面返回了无效预检查结果。')
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
  if (![localChecks, storageChecks, legacyLocal, legacyStorage].some(checks => orderedChecks === checks.join(','))) {
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
  if (type !== 'OBDUMPER_EXPORT' && type !== 'OBLOADER_NORMAL_IMPORT' && type !== 'OBLOADER_DIRECT_LOAD') throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务类型。')
  const parentTaskId = optionalString(value, 'parentTaskId')
  const derivationKind = optionalString(value, 'derivationKind')
  if (derivationKind !== undefined && derivationKind !== 'REBUILD_FROM_CONFIG' && derivationKind !== 'RERUN_FROM_SCRATCH' && derivationKind !== 'CHECKPOINT_RESUME') {
    throw localError('RESPONSE_INVALID', '控制面返回了无效的派生方式。')
  }
  if ((parentTaskId === undefined) !== (derivationKind === undefined)) {
    throw localError('RESPONSE_INVALID', '控制面返回了不一致的派生关系。')
  }
  const presentation = value['presentation'] !== undefined ? parseTaskListItem(value['presentation']) : undefined
  const derivedTasks = value['derivedTasks'] === undefined ? undefined : listOf(value, 'derivedTasks').map(raw => { const item = asRecord(raw); return { id: requiredString(item, 'id'), displayName: requiredString(item, 'displayName'), state: requiredString(item, 'state') } })
  const derivedTaskCount = value['derivedTaskCount'] === undefined ? undefined : requiredNonNegativeInteger(value, 'derivedTaskCount')
  if (derivedTasks && (derivedTaskCount === undefined || derivedTaskCount < derivedTasks.length || derivedTasks.length > 100)) throw localError('RESPONSE_INVALID', '派生任务关系无效。')
  if (presentation && presentation.id !== requiredString(value, 'id')) throw localError('RESPONSE_INVALID', '任务展示事实与当前任务不一致。')
  return {
    ...(derivedTasks ? { derivedTasks, derivedTaskCount } : {}),
    ...(presentation ? { presentation } : {}),
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
  if (snapshotVersion !== 'v1' && snapshotVersion !== 'v2' && snapshotVersion !== 'normal-import-v1' && snapshotVersion !== 'direct-load-v1') throw localError('RESPONSE_INVALID', '冻结配置版本无效。')
  const format = requiredString(value, 'format')
  if ((type !== 'OBDUMPER_EXPORT' && type !== 'OBLOADER_NORMAL_IMPORT' && type !== 'OBLOADER_DIRECT_LOAD') || (type === 'OBDUMPER_EXPORT' ? snapshotVersion !== 'v1' && snapshotVersion !== 'v2' : type === 'OBLOADER_NORMAL_IMPORT' ? snapshotVersion !== 'normal-import-v1' : snapshotVersion !== 'direct-load-v1') || (format !== 'CSV' && format !== 'CUT' && format !== 'SQL' && format !== 'POS' && format !== 'PARQUET' && format !== 'ORC' && format !== 'AVRO' && format !== 'DDL' && format !== 'DDL_CSV' && format !== 'DDL_CUT' && format !== 'DDL_SQL') || type === 'OBLOADER_NORMAL_IMPORT' && !['CSV', 'CUT', 'SQL', 'DDL', 'DDL_CSV', 'DDL_CUT', 'DDL_SQL'].includes(format) || type === 'OBLOADER_DIRECT_LOAD' && !['CSV', 'SQL', 'PARQUET', 'ORC'].includes(format)) {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务快照。')
  }
  const groups = value['groups'] === undefined ? undefined : listOf(value, 'groups').map(raw => {
    const group = asRecord(raw)
    const key = requiredString(group, 'key')
    const allowedGroups = type === 'OBLOADER_DIRECT_LOAD' ? ['applicability', 'connection', 'input', 'target', 'execution', 'review'] : type === 'OBLOADER_NORMAL_IMPORT' ? ['source', 'input', 'objects', 'execution', 'review'] : ['source', 'objects', 'format', 'output', 'review']
    if (!allowedGroups.includes(key)) throw localError('RESPONSE_INVALID', '冻结配置分组无效。')
    const fields = listOf(group, 'fields').map(rawField => {
      const field = asRecord(rawField)
      if (field['source'] !== 'UNKNOWN') throw localError('RESPONSE_INVALID', '冻结配置来源尚未支持。')
      return { key: requiredString(field, 'key'), label: requiredString(field, 'label'), value: requiredString(field, 'value'), source: 'UNKNOWN' as const }
    })
    return { key: key as TaskSnapshotGroup['key'], label: requiredString(group, 'label'), fields }
  })
  if (groups && (groups.length !== (type === 'OBLOADER_DIRECT_LOAD' ? 6 : 5) || new Set(groups.map(group => group.key)).size !== (type === 'OBLOADER_DIRECT_LOAD' ? 6 : 5))) throw localError('RESPONSE_INVALID', '冻结配置分组不完整。')
  return {
    ...(groups ? { groups } : {}),
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
  return {
    ...parseTaskExecutionFacts(value),
    state: requiredString(value, 'state'),
    executionId: optionalString(value, 'executionId'),
    reconciliationRequired: requiredBoolean(value, 'reconciliationRequired'),
    ...parseTaskProgressEvidence(value, undefined, () => { throw localError('RESPONSE_INVALID', '控制面返回了无效任务进度。') }),
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
  if (type !== 'OBDUMPER_EXPORT' && type !== 'OBLOADER_NORMAL_IMPORT' && type !== 'OBLOADER_DIRECT_LOAD') {
    throw localError('RESPONSE_INVALID', '控制面返回了尚未支持的任务列表证据。')
  }
  return {
    id: requiredString(item, 'id'),
    type,
    dataSourceId: requiredString(item, 'dataSourceId'),
    objectSummary: optionalString(item, 'objectSummary'),
    state: requiredString(item, 'state'),
    ...parseTaskProgressEvidence(item, type, () => { throw localError('RESPONSE_INVALID', '控制面返回了无效任务进度。') }),
    reconciliationRequired: requiredBoolean(item, 'reconciliationRequired'),
    nodeId: requiredString(item, 'nodeId'),
    ownedByCurrentUser: requiredBoolean(item, 'ownedByCurrentUser'),
    submittedAt: requiredString(item, 'submittedAt'),
    startedAt: optionalString(item, 'startedAt'),
    finishedAt: optionalString(item, 'finishedAt'),
    updatedAt: requiredString(item, 'updatedAt'),
    displayName: requiredString(item, 'displayName'),
    dataSourceName: optionalString(item, 'dataSourceName'),
    dataSourceDeleted: requiredBoolean(item, 'dataSourceDeleted'),
    environment: optionalTaskEnvironment(item),
    nodeName: optionalString(item, 'nodeName'),
    nodeDeleted: requiredBoolean(item, 'nodeDeleted'),
    creatorName: optionalString(item, 'creatorName'),
    cancelRequested: requiredBoolean(item, 'cancelRequested'),
    // 未知操作丢弃而不是推断，前端只渲染已识别的服务端资格。
    availableActions: listOf(item, 'availableActions').filter((value): value is TaskListAction => typeof value === 'string' && (taskListActions as readonly string[]).includes(value) && (type !== 'OBLOADER_DIRECT_LOAD' || !['CHECKPOINT_RESUME', 'SAVE_TEMPLATE', 'CANCEL'].includes(value))),
  }
}

const taskEnvironments: readonly TaskEnvironment[] = ['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION']

function optionalTaskEnvironment(item: Record<string, unknown>): TaskEnvironment | undefined {
  const value = optionalString(item, 'environment')
  if (value === undefined) return undefined
  if (!taskEnvironments.includes(value as TaskEnvironment)) throw localError('RESPONSE_INVALID', '控制面返回了未知任务环境。')
  return value as TaskEnvironment
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
