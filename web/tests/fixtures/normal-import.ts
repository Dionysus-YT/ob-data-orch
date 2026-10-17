import type { DataSourceSummary, ExecutionNodeSummary } from '../../src/api/browser'

export const normalImportSources: DataSourceSummary[] = [
  { id: 'synthetic-import-mysql', displayName: '合成 MySQL 导入目标', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: 'synthetic.example.invalid', port: 2883, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user', defaultDatabase: 'synthetic_database', state: 'ENABLED', revision: 1, credentialRevision: 1, lastTestStatus: 'SUCCEEDED', lastTestedAt: '2026-10-15T00:00:00Z', sysCredentialState: 'UNAVAILABLE' },
  { id: 'synthetic-import-oracle', displayName: '合成 Oracle 导入目标', environment: 'STAGING', connectionKind: 'ODP', compatibilityMode: 'ORACLE', host: 'synthetic.example.invalid', port: 2883, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user', defaultDatabase: 'SYNTHETIC_SCHEMA', state: 'ENABLED', revision: 2, credentialRevision: 1, lastTestStatus: 'SUCCEEDED', sysCredentialState: 'AVAILABLE' },
  { id: 'synthetic-import-invalidated', displayName: '连接测试已失效', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: 'synthetic.example.invalid', port: 2883, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user', state: 'ENABLED', revision: 2, credentialRevision: 1, lastTestStatus: 'INVALIDATED' },
]

export const normalImportNode: ExecutionNodeSummary = {
  id: 'synthetic-import-node', displayName: '合成导入节点', platform: 'WINDOWS_AMD64', managementState: 'ENABLED',
  agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: '2026-10-15T00:00:00Z',
  environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: true, unavailableReasons: [],
  revision: 1, updatedAt: '2026-10-15T00:00:00Z',
}
