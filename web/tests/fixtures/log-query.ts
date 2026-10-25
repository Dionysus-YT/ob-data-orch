import type { Page } from '@playwright/test'
import type { LogContext, LogItem, LogPage } from '../../src/api/logQuery'

export function logItem(sequence = 60): LogItem { return { token: `record-${sequence}`, taskId: 'synthetic-task-1', source: 'OBDUMPER_STDOUT', streamId: 'a'.repeat(64), epoch: 1, sequence, kind: 'LOG', message: `合成日志 ${sequence} password=******\n<script>window.syntheticUnsafe=true</script>\n保留换行与对象路径 /synthetic/output`, integrityCode: '', receivedAt: '2026-10-25T00:00:00Z' } }
export function logPage(): LogPage { return { items: [logItem()], nextCursor: 'next-2', pageCursor: 'page-1', snapshotToken: 'snapshot-1', snapshotAt: '2026-10-25T00:00:01Z', sources: ['OBDUMPER_STDOUT'], availability: 'RETAINED', expired: false, incomplete: false, truncated: false, scannedRecords: 1, scannedBytes: 256 } }
export function logContext(): LogContext { return { items: [59, 60, 61].map(n => ({ ...logItem(n), token: '' })), selectedSequence: 60, snapshotAt: '2026-10-25T00:00:01Z', incomplete: false } }
export type LogMode = 'ready' | 'denied' | 'error' | 'invalid' | 'empty' | 'not-collected' | 'expired' | 'truncated' | 'gap'
export async function logQueryFixture(page: Page, initial: LogMode = 'ready') {
  let mode = initial, downloadError = false
  const queries: URL[] = [], contexts: string[] = [], actions: { path: string; body: Record<string, unknown>; csrf: string | undefined }[] = []
  await page.route('**/api/v1/logs/**', async route => {
    const req = route.request(), url = new URL(req.url())
    if (url.pathname === '/api/v1/logs/audit-access') return route.fulfill({ json: { allowed: false } })
    if (url.pathname === '/api/v1/logs/tasks') {
      queries.push(url)
      if (mode === 'denied' || mode === 'error') return route.fulfill({ status: mode === 'denied' ? 404 : 503, json: { code: mode === 'denied' ? 'NOT_FOUND' : 'LOG_QUERY_UNAVAILABLE', message: 'synthetic-only' } })
      const base = logPage(), result = { ...base, items: [...base.items] }
      if (url.searchParams.has('cursor') && url.searchParams.get('cursor') === 'next-2') { result.items = [logItem(59)]; result.pageCursor = 'page-2'; result.nextCursor = ''; result.scannedRecords = 2 }
      if (['empty', 'not-collected', 'expired'].includes(mode)) { result.items = []; result.nextCursor = ''; result.availability = mode === 'expired' ? 'EXPIRED' : mode === 'not-collected' ? 'NOT_COLLECTED' : 'RETAINED'; result.expired = mode === 'expired' }
      if (mode === 'truncated') { result.truncated = true; result.nextCursor = ''; result.scannedRecords = 50000 }
      if (mode === 'gap') { result.incomplete = true; result.items[0] = { ...logItem(), kind: 'GAP', integrityCode: 'SPOOL_LOST', message: '合成采集缺口' } }
      return route.fulfill({ json: { item: mode === 'invalid' ? { ...result, guessedLevel: 'INFO' } : result } })
    }
    if (url.pathname === '/api/v1/logs/task-context') { contexts.push(url.searchParams.get('token') ?? ''); return route.fulfill({ json: { item: logContext() } }) }
    actions.push({ path: url.pathname, body: req.postDataJSON(), csrf: req.headers()['x-csrf-token'] })
    if (url.pathname.endsWith(':copy')) return route.fulfill({ json: { text: '合成上下文 password=******\n仅同一来源' } })
    if (downloadError) return route.fulfill({ status: 422, json: { code: 'LOG_DOWNLOAD_LIMIT', message: 'synthetic-only' } })
    return route.fulfill({ headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Content-Disposition': 'attachment; filename="task-logs.log"' }, body: '# 已脱敏\n合成日志 password=******\n' })
  })
  return { queries, contexts, actions, setMode: (value: LogMode) => { mode = value }, failDownload: () => { downloadError = true } }
}
