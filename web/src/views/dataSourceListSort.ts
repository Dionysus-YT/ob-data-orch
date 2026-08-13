import type { DataSourceSummary } from '@/api/browser'

export type DataSourceSortKey = 'name' | 'environment' | 'host' | 'port' | 'tenant' | 'mode' | 'connection' | 'state'
export type SortDirection = 'asc' | 'desc'

const environmentRank: Record<string, number> = { DEVELOPMENT: 0, TEST: 1, STAGING: 2, PRODUCTION: 3 }
const connectionRank: Record<string, number> = { FAILED: 0, INVALIDATED: 1, EXPIRED: 2, UNKNOWN: 3, UNAVAILABLE: 4, UNTESTED: 5, PENDING: 6, SUCCEEDED: 7 }
const collator = new Intl.Collator('zh-CN', { numeric: true, sensitivity: 'base' })

export function sortDataSources(sources: readonly DataSourceSummary[], key: DataSourceSortKey, direction: SortDirection) {
  const factor = direction === 'asc' ? 1 : -1
  return [...sources].sort((left, right) => {
    const result = compareDataSource(left, right, key)
    if (result !== 0) return result * factor
    return collator.compare(left.id, right.id)
  })
}

function compareDataSource(left: DataSourceSummary, right: DataSourceSummary, key: DataSourceSortKey) {
  switch (key) {
    case 'name': return collator.compare(left.displayName, right.displayName)
    case 'environment': return compareNumber(environmentRank[left.environment] ?? 99, environmentRank[right.environment] ?? 99)
    case 'host': return collator.compare(left.host, right.host)
    case 'port': return compareNumber(left.port, right.port)
    case 'tenant': return collator.compare(left.tenantName, right.tenantName)
    case 'mode': return collator.compare(left.compatibilityMode, right.compatibilityMode)
    case 'connection': return compareNumber(connectionRank[left.lastTestStatus ?? 'UNTESTED'] ?? 99, connectionRank[right.lastTestStatus ?? 'UNTESTED'] ?? 99)
    case 'state': return compareNumber(left.state === 'ENABLED' ? 0 : 1, right.state === 'ENABLED' ? 0 : 1)
  }
}

function compareNumber(left: number, right: number) {
  return left === right ? 0 : left < right ? -1 : 1
}
