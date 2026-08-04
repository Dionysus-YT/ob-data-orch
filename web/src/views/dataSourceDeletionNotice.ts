import type { DataSourceDeletionResult } from '@/api/browser'

export function dataSourceDeletionNotice(result: DataSourceDeletionResult): string {
  return result.outcome === 'DELETED'
    ? '数据源已物理删除，可使用原名称新建。'
    : '数据源已归档，因历史引用仍保留名称。'
}
