import type { DataSourceSummary } from '@/api/browser'

export type ConnectionStatusFilter = '' | 'UNTESTED' | 'SUCCEEDED' | 'FAILED' | 'PENDING' | 'UNAVAILABLE'

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
    const keywordMatched = !normalizedKeyword || [source.displayName, source.host].some((item) => item.toLowerCase().includes(normalizedKeyword))
    const connectionStatus = source.lastTestStatus ?? 'UNTESTED'
    return keywordMatched && (!filters.environment || source.environment === filters.environment) && (!filters.compatibilityMode || source.compatibilityMode === filters.compatibilityMode) && (!filters.connectionStatus || connectionStatus === filters.connectionStatus) && (!filters.state || source.state === filters.state)
  })
}
