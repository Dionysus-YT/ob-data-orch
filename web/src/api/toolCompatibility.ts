export interface ToolCatalogVersion {
  readonly metadataVersion: string
  readonly usage: 'TASK_CONFIGURATION' | 'HISTORICAL_REPLAY'
}
export interface ToolModule {
  readonly taskType: 'OBDUMPER_EXPORT' | 'OBLOADER_NORMAL_IMPORT' | 'OBLOADER_DIRECT_LOAD'
  readonly metadataSelection: 'TASK_CONFIGURATION' | 'FIXED'
  readonly catalogs: readonly ToolCatalogVersion[]
}
export interface ToolBaseline {
  readonly tool: 'OBDUMPER' | 'OBLOADER'
  readonly supportedVersions: readonly string[]
  readonly defaultTargetVersion: string
  readonly modules: readonly ToolModule[]
}
export interface ToolCompatibility {
  readonly capturedAt: string
  readonly loadedAt: string
  readonly source: 'BUILT_IN_CATALOGS'
  readonly tools: readonly ToolBaseline[]
}
export interface ToolCompatibilityApi { getToolCompatibility(signal?: AbortSignal): Promise<ToolCompatibility> }

function invalid(): never { throw { status: 502, code: 'RESPONSE_INVALID', message: '工具状态响应无法校验，请重新读取。', retryable: false, conflict: false, fieldErrors: [] } }
function object(value: unknown, keys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value) || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Object.keys(value).length !== keys.length || keys.some(key => !Object.hasOwn(value, key))) invalid()
  return value as Record<string, unknown>
}
function timestamp(value: unknown): value is string {
  return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) && Number.isFinite(Date.parse(value)) && new Date(value).toISOString().slice(0, 19) === value.slice(0, 19)
}
function module(value: unknown, type: ToolModule['taskType']): ToolModule {
  const row = object(value, ['taskType', 'metadataSelection', 'catalogs']), exported = type === 'OBDUMPER_EXPORT'
  if (row.taskType !== type || row.metadataSelection !== (exported ? 'TASK_CONFIGURATION' : 'FIXED') || !Array.isArray(row.catalogs) || row.catalogs.length < 1 || row.catalogs.length > (exported ? 32 : 1)) invalid()
  const pattern = exported ? /^obdumper-4\.3\.5-slice-v[1-9]\d*(?:-[a-z0-9]+)*$/ : new RegExp(`^obloader-4\\.3\\.5-${type === 'OBLOADER_NORMAL_IMPORT' ? 'normal' : 'direct'}-v[1-9]\\d*$`)
  const catalogs: ToolCatalogVersion[] = row.catalogs.map(value => {
    const item = object(value, ['metadataVersion', 'usage'])
    if (typeof item.metadataVersion !== 'string' || item.metadataVersion.length > 128 || !pattern.test(item.metadataVersion) || !['TASK_CONFIGURATION', 'HISTORICAL_REPLAY'].includes(String(item.usage)) || (!exported && item.usage !== 'TASK_CONFIGURATION')) invalid()
    return { metadataVersion: item.metadataVersion, usage: item.usage as ToolCatalogVersion['usage'] }
  })
  if (new Set(catalogs.map(item => item.metadataVersion)).size !== catalogs.length || !catalogs.some(item => item.usage === 'TASK_CONFIGURATION')) invalid()
  return { taskType: type, metadataSelection: exported ? 'TASK_CONFIGURATION' : 'FIXED', catalogs }
}

/** 工具、模块与历史目录分别校验；不接受节点安装结论、未知工具或猜测的全局活动版本。 */
export function parseToolCompatibility(value: unknown): ToolCompatibility {
  const row = object(value, ['capturedAt', 'loadedAt', 'source', 'tools'])
  if (!timestamp(row.capturedAt) || !timestamp(row.loadedAt) || Date.parse(row.loadedAt) > Date.parse(row.capturedAt) || row.source !== 'BUILT_IN_CATALOGS' || !Array.isArray(row.tools) || row.tools.length !== 2) invalid()
  const tools: ToolBaseline[] = row.tools.map((value, i) => {
    const tool = object(value, ['tool', 'supportedVersions', 'defaultTargetVersion', 'modules']), name = i === 0 ? 'OBDUMPER' : 'OBLOADER'
    if (tool.tool !== name || !Array.isArray(tool.supportedVersions) || tool.supportedVersions.length !== 1 || tool.supportedVersions[0] !== '4.3.5-RELEASE' || tool.defaultTargetVersion !== tool.supportedVersions[0] || !Array.isArray(tool.modules) || tool.modules.length !== (i === 0 ? 1 : 2)) invalid()
    return { tool: name, supportedVersions: [...tool.supportedVersions] as string[], defaultTargetVersion: tool.defaultTargetVersion as string, modules: i === 0 ? [module(tool.modules[0], 'OBDUMPER_EXPORT')] : [module(tool.modules[0], 'OBLOADER_NORMAL_IMPORT'), module(tool.modules[1], 'OBLOADER_DIRECT_LOAD')] }
  })
  return { capturedAt: row.capturedAt, loadedAt: row.loadedAt, source: 'BUILT_IN_CATALOGS', tools }
}

/** 仅访问固定只读端点，不提供工具安装、元数据激活或任意配置传输。 */
export function createToolCompatibilityApi(transport: { read(path: string, signal?: AbortSignal): Promise<Record<string, unknown>> }): ToolCompatibilityApi {
  return { async getToolCompatibility(signal) {
    const response = await transport.read('/api/v1/settings/tools', signal)
    if (!response || ![Object.prototype, null].includes(Object.getPrototypeOf(response)) || Object.keys(response).some(key => !['requestId', 'item'].includes(key)) || !Object.hasOwn(response, 'item')) invalid()
    return parseToolCompatibility(response.item)
  } }
}
