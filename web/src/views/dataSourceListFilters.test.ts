import { describe, expect, it } from 'vitest'
import type { DataSourceSummary } from '@/api/browser'
import { filterDataSources, type DataSourceListFilters } from './dataSourceListFilters'

const sources: DataSourceSummary[] = [
  { id: 'source-1', displayName: '未测试源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'UNKNOWN', host: '10.0.0.1', port: 2881, state: 'DISABLED', revision: 1, credentialRevision: 1 },
  { id: 'source-2', displayName: '可连接源', environment: 'PRODUCTION', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '10.0.0.2', port: 2881, state: 'ENABLED', revision: 1, credentialRevision: 1, lastTestStatus: 'SUCCEEDED' },
  { id: 'source-3', displayName: '失败源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'ORACLE', host: '10.0.0.3', port: 2881, state: 'ENABLED', revision: 1, credentialRevision: 1, lastTestStatus: 'FAILED' },
]

const allFilters: DataSourceListFilters = { keyword: '', environment: '', compatibilityMode: '', connectionStatus: '', state: '' }

describe('数据源列表筛选', () => {
  it('按连接状态筛选，并将缺少受控测试结果的数据源归为未测试', () => {
    expect(filterDataSources(sources, { ...allFilters, connectionStatus: 'UNTESTED' }).map((source) => source.id)).toEqual(['source-1'])
    expect(filterDataSources(sources, { ...allFilters, connectionStatus: 'SUCCEEDED' }).map((source) => source.id)).toEqual(['source-2'])
    expect(filterDataSources(sources, { ...allFilters, connectionStatus: 'FAILED' }).map((source) => source.id)).toEqual(['source-3'])
  })

  it('与既有关键字、环境、兼容模式和启用状态筛选叠加', () => {
    expect(filterDataSources(sources, { ...allFilters, keyword: '源', environment: 'TEST', compatibilityMode: 'ORACLE', connectionStatus: 'FAILED', state: 'ENABLED' }).map((source) => source.id)).toEqual(['source-3'])
  })
})
