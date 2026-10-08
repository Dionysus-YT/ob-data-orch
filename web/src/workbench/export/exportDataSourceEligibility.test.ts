import { describe, expect, it } from 'vitest'

import type { DataSourceSummary } from '@/api/browser'
import { isExportEligibleDataSource } from './exportDataSourceEligibility'

const baseSource: DataSourceSummary = {
  id: 'source-1', displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2881,
  clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user', state: 'ENABLED', revision: 1, credentialRevision: 1,
}

describe('导出数据源候选', () => {
  it('只接受已启用且当前配置已有成功测试事实的数据源', () => {
    expect(isExportEligibleDataSource({ ...baseSource, lastTestStatus: 'SUCCEEDED' })).toBe(true)
    expect(isExportEligibleDataSource({ ...baseSource, lastTestStatus: 'FAILED' })).toBe(false)
    expect(isExportEligibleDataSource({ ...baseSource, state: 'DISABLED', lastTestStatus: 'SUCCEEDED' })).toBe(false)
    expect(isExportEligibleDataSource(baseSource)).toBe(false)
  })
})
