import type { DataSourceSummary } from '@/api/browser'

export function isExportEligibleDataSource(source: DataSourceSummary): boolean {
  return source.state === 'ENABLED' && source.lastTestStatus === 'SUCCEEDED'
}
