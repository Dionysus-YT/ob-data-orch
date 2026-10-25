import type { Page } from '@playwright/test'
import type { AuditItem, AuditPage } from '../../src/api/auditLogs'
import { logQueryFixture } from './log-query'

export function auditItem(id = 'event-2'): AuditItem { return { token: `record-${id}`, id, actorType: 'SUBJECT', actorId: 'synthetic-actor', action: 'DATA_SOURCE_UPDATED', objectType: 'DATA_SOURCE', objectId: 'synthetic-source', result: 'SUCCEEDED', requestId: 'synthetic-request', occurredAt: '2026-10-25T01:00:00Z', fields: [{ name: 'changed', value: 'true' }, { name: 'revision', value: '2' }], detailsOmitted: true } }
export function auditPage(): AuditPage { return { items: [auditItem()], nextCursor: 'next-2', pageCursor: 'page-1', snapshotToken: 'snapshot-1', snapshotAt: '2026-10-25T01:00:01Z', scannedRecords: 1, scannedBytes: 500, truncated: false } }
export type AuditMode = 'ready' | 'denied' | 'error' | 'invalid' | 'empty' | 'truncated'
export async function auditLogFixture(page: Page, initial: AuditMode = 'ready') {
 const task = await logQueryFixture(page), queries: URL[] = [], details: string[] = [], downloads: { body: unknown; csrf: string | undefined }[] = []
 let mode = initial, allowed = true, detailDenied = false, occurredAt = new Date(Date.now() - 1800000).toISOString(), snapshotAt = new Date().toISOString()
 await page.route('**/api/v1/logs/**', async route => {
  const request = route.request(), url = new URL(request.url())
  if (url.pathname === '/api/v1/logs/audit-access') return route.fulfill({ json: { allowed } })
  if (url.pathname === '/api/v1/logs/audit') {
   queries.push(url)
   occurredAt = new Date((Date.parse(url.searchParams.get('from')!) + Date.parse(url.searchParams.get('to')!)) / 2).toISOString()
   if (!url.searchParams.has('cursor')) snapshotAt = new Date().toISOString()
   if (mode === 'denied' || mode === 'error') return route.fulfill({ status: mode === 'denied' ? 404 : 503, json: { code: 'SYNTHETIC_ERROR' } })
   const result = { ...auditPage(), snapshotAt, items: [{ ...auditItem(), occurredAt }] }
   if (url.searchParams.get('cursor') === 'next-2') { result.items = [{ ...auditItem('event-1'), occurredAt }]; result.nextCursor = ''; result.pageCursor = 'page-2'; result.scannedRecords = 2 }
   if (mode === 'empty') result.items = []
   if (mode === 'truncated') { result.truncated = true; result.nextCursor = ''; result.scannedRecords = 50000 }
   return route.fulfill({ json: { item: mode === 'invalid' ? { ...result, safeDiff: { password: 'synthetic-unsafe-diff' } } : result } })
  }
  if (url.pathname === '/api/v1/logs/audit-detail') { details.push(url.searchParams.get('token') ?? ''); if (detailDenied) return route.fulfill({ status: 404, json: { code: 'NOT_FOUND' } }); return route.fulfill({ json: { item: { ...auditItem(), occurredAt } } }) }
  if (url.pathname === '/api/v1/logs/audit:download') { downloads.push({ body: request.postDataJSON(), csrf: request.headers()['x-csrf-token'] }); return route.fulfill({ headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Content-Disposition': 'attachment; filename="audit-logs.log"' }, body: '平台审计事件 · 已脱敏\nDATA_SOURCE_UPDATED\n' }) }
  return route.fallback()
 })
 return { task, queries, details, downloads, setMode: (value: AuditMode) => { mode = value }, setAllowed: (value: boolean) => { allowed = value }, denyDetail: () => { detailDenied = true } }
}
