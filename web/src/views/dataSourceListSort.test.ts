import { describe, expect, it } from 'vitest'

import type { DataSourceSummary } from '@/api/browser'
import { sortDataSources } from './dataSourceListSort'

function source(id: string, input: Partial<DataSourceSummary> = {}): DataSourceSummary {
  return { id, displayName: id, environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2883, clusterName: 'cluster', tenantName: 'tenant', state: 'DISABLED', revision: 1, credentialRevision: 1, ...input }
}

describe('数据源列表排序', () => {
  it('端口使用数值排序并保持输入数组不变', () => {
    const input = [source('large', { port: 28830 }), source('small', { port: 2883 })]
    expect(sortDataSources(input, 'port', 'asc').map((item) => item.id)).toEqual(['small', 'large'])
    expect(input.map((item) => item.id)).toEqual(['large', 'small'])
  })

  it('环境按开发到生产的风险等级排序', () => {
    const input = [source('prod', { environment: 'PRODUCTION' }), source('dev', { environment: 'DEVELOPMENT' }), source('stage', { environment: 'STAGING' })]
    expect(sortDataSources(input, 'environment', 'asc').map((item) => item.id)).toEqual(['dev', 'stage', 'prod'])
  })

  it('连接状态升序优先呈现需要处理的失败与失效状态', () => {
    const input = [source('ok', { lastTestStatus: 'SUCCEEDED' }), source('untested'), source('failed', { lastTestStatus: 'FAILED' }), source('stale', { lastTestStatus: 'INVALIDATED' })]
    expect(sortDataSources(input, 'connection', 'asc').map((item) => item.id)).toEqual(['failed', 'stale', 'untested', 'ok'])
  })
})
