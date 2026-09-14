import type { DataSourceSummary } from '@/api/browser'

export type ConnectionStatusFilter = '' | 'UNTESTED' | 'TESTING' | 'SUCCEEDED' | 'FAILED' | 'UNKNOWN' | 'EXPIRED' | 'INVALIDATED' | 'UNAVAILABLE'

export type DataSourceListFilters = {
  readonly keyword: string
  readonly environment: string
  readonly compatibilityMode: string
  readonly connectionStatus: ConnectionStatusFilter
  readonly state: string
}

export function filterDataSources(sources: readonly DataSourceSummary[], filters: DataSourceListFilters) {
  const normalizedKeyword = filters.keyword.toLowerCase()
  return sources.filter((source) => {
    const keywordMatched = !normalizedKeyword || [source.displayName, source.host, source.clusterName, source.tenantName].some((item) => item.toLowerCase().includes(normalizedKeyword))
    const connectionStatus = source.lastTestStatus ?? 'UNTESTED'
    const connectionMatched = !filters.connectionStatus
      || (filters.connectionStatus === 'TESTING' ? connectionStatus === 'PENDING' || connectionStatus === 'LEASED' : connectionStatus === filters.connectionStatus)
    return keywordMatched && (!filters.environment || source.environment === filters.environment) && (!filters.compatibilityMode || source.compatibilityMode === filters.compatibilityMode) && connectionMatched && (!filters.state || source.state === filters.state)
  })
}
