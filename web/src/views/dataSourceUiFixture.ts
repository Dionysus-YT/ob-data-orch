import type { DataSourceSummary } from '@/api/browser'

const dataSourceUiFixtureParameter = 'uiFixture'
const dataSourceUiFixtureValue = 'data-sources'

// 开发样本仅用于冻结视觉状态的同态比对；全部端点和身份均为合成值，不进入真实数据源、任务或连接测试路径。
export const DATA_SOURCE_UI_FIXTURES: readonly DataSourceSummary[] = [
  {
    id: 'ui-fixture-production-finance-reporting', displayName: 'Production finance reporting', environment: 'PRODUCTION', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '192.0.2.18', port: 2883, clusterName: 'obp-east-01', tenantName: 'analytics', username: 'finance_reporter', sysCredentialState: 'AVAILABLE', defaultDatabase: 'finance_reporting', state: 'ENABLED', revision: 101, credentialRevision: 201, lastTestStatus: 'SUCCEEDED', lastTestedAt: '2026-09-01T10:32:45.000Z',
    lifecycleEligibility: { enable: { allowed: false, reasonCode: 'ALREADY_ENABLED', reason: '数据源当前已启用。' }, disable: { allowed: true }, delete: { allowed: false, reasonCode: 'UNFINISHED_TASKS_EXIST', reason: '存在未完成任务，请等待任务结束后删除。' }, archive: { allowed: true, referenceCount: 3 } },
  },
  { id: 'ui-fixture-customer-analytics-source', displayName: 'Customer analytics source', environment: 'PRODUCTION', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '192.0.2.18', port: 2883, clusterName: 'obp-east-02', tenantName: 'analytics', username: 'analytics_reader', sysCredentialState: 'AVAILABLE', defaultDatabase: 'customer_analytics', state: 'ENABLED', revision: 102, credentialRevision: 202, lastTestStatus: 'SUCCEEDED', lastTestedAt: '2026-09-01T09:15:21.000Z' },
  { id: 'ui-fixture-data-mart-sales', displayName: 'Data mart – sales', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '192.0.2.18', port: 2883, clusterName: 'obp-test-01', tenantName: 'dwh', username: 'dwh_reader', sysCredentialState: 'UNAVAILABLE', defaultDatabase: 'sales_mart', state: 'ENABLED', revision: 103, credentialRevision: 203, lastTestStatus: 'INVALIDATED', lastTestedAt: '2026-08-20T17:45:12.000Z' },
  { id: 'ui-fixture-internal-reporting-db', displayName: 'Internal reporting db', environment: 'STAGING', connectionKind: 'ODP', compatibilityMode: 'ORACLE', host: '192.0.2.18', port: 2883, clusterName: 'obp-pre-01', tenantName: 'reporting', username: 'reporting_reader', sysCredentialState: 'AVAILABLE', state: 'ENABLED', revision: 104, credentialRevision: 204, lastTestStatus: 'UNKNOWN', lastTestedAt: '2026-09-18T11:05:33.000Z' },
  { id: 'ui-fixture-legacy-finance-archive', displayName: 'Legacy finance archive', environment: 'PRODUCTION', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '192.0.2.18', port: 2883, clusterName: 'obp-east-03', tenantName: 'finance', username: 'finance_archive', sysCredentialState: 'AVAILABLE', defaultDatabase: 'finance_archive', state: 'DISABLED', revision: 105, credentialRevision: 205, lastTestStatus: 'FAILED', lastTestedAt: '2026-09-01T09:42:17.000Z' },
  { id: 'ui-fixture-data-lake-external-source', displayName: 'Data lake external source', environment: 'PRODUCTION', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '192.0.2.18', port: 2883, clusterName: 'datalake-01', tenantName: 'analytics', username: 'lake_reader', sysCredentialState: 'UNAVAILABLE', defaultDatabase: 'lake_external', state: 'ENABLED', revision: 106, credentialRevision: 206, lastTestStatus: 'PENDING' },
  { id: 'ui-fixture-dev-sandbox', displayName: 'Dev sandbox', environment: 'DEVELOPMENT', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '192.0.2.18', port: 2883, clusterName: 'obp-dev-01', tenantName: 'sandbox', username: 'sandbox_user', sysCredentialState: 'UNAVAILABLE', defaultDatabase: 'sandbox', state: 'DISABLED', revision: 107, credentialRevision: 207 },
]

// 双重开关必须同时满足；重复参数也按歧义输入失败关闭，避免意外进入演示状态。
export function isDataSourceUiFixtureEnabled(search: string, isDevelopment: boolean): boolean {
  if (!isDevelopment) return false
  const values = new URLSearchParams(search).getAll(dataSourceUiFixtureParameter)
  return values.length === 1 && values[0] === dataSourceUiFixtureValue
}

// 每次返回副本，避免页面内的临时操作污染固定样本或后续加载结果。
export function dataSourceUiFixtureForSearch(search: string, isDevelopment = import.meta.env.DEV): DataSourceSummary[] | undefined {
  if (!isDataSourceUiFixtureEnabled(search, isDevelopment)) return undefined
  return DATA_SOURCE_UI_FIXTURES.map((source) => ({ ...source, lifecycleEligibility: source.lifecycleEligibility ? { ...source.lifecycleEligibility } : undefined }))
}
