import type { DataSourceDetail, DataSourceSummary, DataSourceUpdate, DataSourceWrite } from '@/api/browser'

export const environments = [{ value: 'DEVELOPMENT', label: '开发' }, { value: 'TEST', label: '测试' }, { value: 'STAGING', label: '预生产' }, { value: 'PRODUCTION', label: '生产' }] as const
export function environmentLabel(value: string) { return environments.find((entry) => entry.value === value)?.label ?? '未知环境' }

export function connectionFact(source: Pick<DataSourceSummary, 'lastTestStatus' | 'lastTestedAt'>) {
  const status = source.lastTestStatus
  if (status === 'PENDING' || status === 'LEASED') return { label: '测试中', tone: 'running' as const, detail: status === 'PENDING' ? '等待执行节点' : '执行节点已领取', icon: 'none' as const }
  // 列表摘要没有真实连接验证来源，只表达历史测试结果，不渲染为真实资格已通过。
  if (status === 'SUCCEEDED') return { label: '最近测试成功', tone: 'neutral' as const, detail: formatVerifiedTime(source.lastTestedAt), icon: 'check' as const }
  if (status === 'FAILED') return { label: '连接失败', tone: 'danger' as const, detail: formatVerifiedTime(source.lastTestedAt), icon: 'none' as const }
  if (status === 'INVALIDATED' || status === 'EXPIRED') return { label: status === 'INVALIDATED' ? '结果已失效' : '结果已过期', tone: 'warning' as const, detail: source.lastTestedAt ? formatVerifiedTime(source.lastTestedAt) : '需要重新测试', icon: 'none' as const }
  if (status === 'UNKNOWN') return { label: '状态待确认', tone: 'neutral' as const, detail: source.lastTestedAt ? formatVerifiedTime(source.lastTestedAt) : '没有可验证的结论', icon: 'unknown' as const }
  return { label: '未测试', tone: 'neutral' as const, detail: '尚无测试记录', icon: 'none' as const }
}

// 列表使用单行短状态；成功仍只表示测试记录，未知状态不得降级为未测试或可用资格。
export function connectionListFact(source: Pick<DataSourceSummary, 'lastTestStatus'>) {
  const status = source.lastTestStatus
  // 绿色仅表达这条测试记录成功，与已启用共用视觉样式，但不派生生命周期或真实验证资格。
  const tone = status === 'SUCCEEDED' ? 'success' as const : connectionFact(source).tone
  if (!status) return { label: '未测试', tone }
  const labels: Record<string, string> = { PENDING: '测试中', LEASED: '测试中', SUCCEEDED: '测试成功', FAILED: '连接失败', INVALIDATED: '已失效', EXPIRED: '已过期', UNKNOWN: '待确认' }
  return { label: labels[status] ?? '待确认', tone }
}

export function formatVerifiedTime(value?: string) {
  if (!value || !Number.isFinite(new Date(value).getTime())) return '验证时间未提供'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value))
}

export type SourceForm = { -readonly [K in keyof DataSourceWrite]: DataSourceWrite[K] }
export function blankSourceForm(): SourceForm { return { displayName: '', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '', port: 2883, clusterName: '', tenantName: '', username: '', defaultDatabase: '', password: '', sysUser: '', sysPassword: '' } }

// 仅比较 P3 允许变更的字段；凭据留空保留，显式清除 sys 凭据使用既有 API 空值语义。
export function sourceUpdate(form: SourceForm, source: DataSourceDetail, clearSys: boolean): DataSourceUpdate {
  const changed: { -readonly [K in keyof DataSourceUpdate]?: DataSourceUpdate[K] } = {}
  for (const key of ['displayName', 'host', 'clusterName', 'tenantName', 'username'] as const) {
    if (form[key].trim() !== (source[key] ?? '')) changed[key] = form[key].trim()
  }
  if (form.environment !== source.environment) changed.environment = form.environment
  if (form.compatibilityMode !== source.compatibilityMode) changed.compatibilityMode = form.compatibilityMode
  if (form.port !== source.port) changed.port = form.port
  const database = form.compatibilityMode === 'MYSQL' ? form.defaultDatabase ?? '' : ''
  if (database !== (source.defaultDatabase ?? '')) changed.defaultDatabase = database
  if (form.password) changed.password = form.password
  if (clearSys) { changed.sysUser = ''; changed.sysPassword = '' }
  else if (form.sysUser?.trim() && form.sysPassword) { changed.sysUser = form.sysUser.trim(); changed.sysPassword = form.sysPassword }
  return changed
}
export function invalidatesConnection(update: DataSourceUpdate) { return Object.keys(update).some((key) => key !== 'displayName' && key !== 'environment') }
