import type { DataSourceSummary, ExecutionNodeSummary } from '@/api/browser'
import type { DirectLoadConfig, DirectLoadParameter } from '@/api/directLoad'

function unsafeText(value: string) { return Array.from(value).some(char => { const code = char.charCodeAt(0); return code < 32 || code >= 127 && code <= 159 }) }
function singleIdentifier(value: string) { return new TextEncoder().encode(value).length <= 256 && !value.startsWith('-') && /^[\p{L}\p{Nd}_$-]+$/u.test(value) }

export const directLoadTitles = ['数据源与连接', '导入目标', '文件与格式', '执行参数', '预检查与提交'] as const
export const directLoadReviewStep = directLoadTitles.length
export const directLoadScenarioLabels = { PRIVATE_ODP: '通过 ODP 连接', OBSERVER_DIRECT: '直连 OBServer' }
export interface DirectLoadIssue { step: number; field: string; message: string }

/** 场景端口仅为可编辑的填写默认值，不代表实际监听、映射或连通证据。 */
export function directLoadDefaultRpcPort(scenario: DirectLoadConfig['scenario']): number { return scenario === 'PRIVATE_ODP' ? 2885 : scenario === 'OBSERVER_DIRECT' ? 2882 : 0 }

/** 本地部署只投影当前可见目录；保留官方研究基线，不把已确认的其他部署能力显示为本地可用。 */
export function directLoadLocalParameters(parameters: readonly DirectLoadParameter[]): DirectLoadParameter[] {
  return parameters.filter(item => item.longName !== 'public-cloud').map(item => {
    if (['sql', 'par', 'orc'].includes(item.longName)) return { ...item, supportState: 'VALIDATION_GATED', description: '当前本地部署尚未确认此格式的旁路适用性。', disposition: '待官方参数映射确认，不开放配置或生成。' }
    if (item.longName === 'rpc-port') return { ...item, description: '私有 ODP 默认 2885，OBServer 默认 2882；可修改，必须实际检查。', disposition: '必填；默认值不代表实际监听或连通事实。' }
    if (item.longName === 'tenant') return { ...item, description: 'ODP 与 OBServer 直连均从已登记数据源派生租户；固定 4.3.5 RPC 连接需要租户参数。', disposition: '由数据源派生，不能与登录租户不一致。' }
    return item
  })
}

/** 前四步百分比仅表达有效输入完整度；可选参数不合法时同样不能显示全部配置完成。 */
export function directLoadInputProgress(step: number, config: DirectLoadConfig, issues: readonly DirectLoadIssue[]) {
  const required = step === 1 ? ['dataSourceId', 'scenario', 'rpcPort', 'applicability']
    : step === 2 ? ['database', 'table'] : step === 3 ? ['nodeId', 'filePath', 'format'] : []
  const invalid = new Set(issues.filter(issue => issue.step === step).map(issue => issue.field))
  if (step === 1 && !Object.values(config.applicability).every(Boolean)) invalid.add('applicability')
  const fields = new Set([...required, ...invalid])
  return fields.size ? Math.round((fields.size - invalid.size) * 100 / fields.size) : 100
}

/** 候选资格属于旁路流程，不借用普通导入或导出资格。 */
export function isDirectLoadSource(source: DataSourceSummary) { return source.state === 'ENABLED' && source.lastTestStatus === 'SUCCEEDED' && ['ODP', 'OBSERVER_DIRECT'].includes(source.connectionKind) && ['MYSQL', 'ORACLE'].includes(source.compatibilityMode) }
/** 连接方式来自已登记源；部署为单机不改变租户，也不根据 SQL 端口猜测路径。 */
export function directLoadScenarioForSource(source?: Pick<DataSourceSummary, 'connectionKind'>): DirectLoadConfig['scenario'] { return source?.connectionKind === 'OBSERVER_DIRECT' ? 'OBSERVER_DIRECT' : source?.connectionKind === 'ODP' ? 'PRIVATE_ODP' : '' }
/** 本地两种场景仅开放已确认的 CSV；历史或未知场景不获得格式资格。 */
export function directLoadFormats(scenario: DirectLoadConfig['scenario']): string[] { return scenario === 'PRIVATE_ODP' || scenario === 'OBSERVER_DIRECT' ? ['CSV'] : [] }
/** 前四步仅校验输入，不把本地完整度冒充版本、RPC、文件或结构检查。 */
export function directLoadIssues(config: DirectLoadConfig, source?: DataSourceSummary, node?: ExecutionNodeSummary): DirectLoadIssue[] {
  const issues: DirectLoadIssue[] = [], add = (step: number, field: string, message: string) => issues.push({ step, field, message })
  if (!Object.values(config.applicability).every(Boolean)) add(1, 'applicability', '请返回数据源与连接，点击下一步确认旁路适用条件。')
  if (!source || source.id !== config.dataSourceId || !isDirectLoadSource(source)) add(1, 'dataSourceId', '请选择已授权、已启用且连接测试成功的数据源。')
  if (!config.scenario) add(1, 'scenario', '请选择实际连接场景。')
  else if (!(config.scenario in directLoadScenarioLabels)) add(1, 'scenario', '当前配置不适用于本地部署，请重新选择连接场景。')
  else if (source && config.scenario !== directLoadScenarioForSource(source)) add(1, 'scenario', '连接方式与当前数据源不一致，请重新应用数据源连接方式。')
  if (!singleIdentifier(config.database)) add(2, 'database', '请填写唯一目标数据库或 Schema，不含限定符或表达式。')
  if (!Number.isSafeInteger(config.rpcPort) || config.rpcPort < 1 || config.rpcPort > 65535) add(1, 'rpcPort', '请填写 1～65535 范围的实际 RPC 端口。')
  if (!node || !node.acceptsNewTasks || node.id !== config.nodeId) add(3, 'nodeId', '请选择可接收新任务的授权执行节点。')
  const absolute = node?.platform === 'WINDOWS_AMD64' ? /^[A-Za-z]:[\\/]/.test(config.filePath) && !/[<>"|?*:]/.test(config.filePath.slice(2)) : ['LINUX_AMD64', 'LINUX_ARM64'].includes(node?.platform ?? '') && config.filePath.startsWith('/') && !config.filePath.startsWith('//')
  if (!absolute || unsafeText(config.filePath)) add(3, 'filePath', '请填写与执行节点平台匹配的本地绝对路径。')
  if (!singleIdentifier(config.table)) add(2, 'table', '请填写一个明确目标表；不允许限定符、星号、多表或表达式。')
  if (!directLoadFormats(config.scenario).includes(config.format)) add(3, 'format', '当前连接场景尚未确认此格式的旁路适用性。')
  if (config.format === 'CSV' && (new TextEncoder().encode(config.columnSeparator).length > 32 || unsafeText(config.columnSeparator) || config.columnSeparator.startsWith('-'))) add(3, 'columnSeparator', 'CSV 列分隔符格式无效。')
  for (const field of ['thread', 'parallel'] as const) if (config[field] && (!/^[0-9]+$/.test(config[field]) || !Number.isSafeInteger(Number(config[field])) || Number(config[field]) < 1 || Number(config[field]) > 2147483647)) add(4, field, '并发必须为有效正整数，留空则沿用官方默认。')
  return issues
}
