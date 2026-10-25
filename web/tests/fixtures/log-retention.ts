import type { Page } from '@playwright/test'
import type { LogRetentionPolicy, LogRetentionWrite } from '../../src/api/logRetention'
export type RetentionMode = 'ready' | 'denied' | 'error' | 'invalid' | 'write-error' | 'conflict' | 'replay' | 'refresh-error'
export const retentionPolicy = (): LogRetentionPolicy => ({ retentionDays: 30, revision: 1, source: 'BUILT_IN_DEFAULT', applicationMode: 'NEXT_BATCH', restartRequired: false })
export async function logRetentionFixture(page: Page, initial: RetentionMode = 'ready') {
  let mode = initial, policy = retentionPolicy()
  const writes: { body: LogRetentionWrite; headers: Record<string, string> }[] = []
  await page.route('**/api/v1/settings/log-retention', async route => {
    const request = route.request()
    const fail = (status: number, code: string) => route.fulfill({ status, json: { code, message: 'synthetic-only', retryable: status >= 500 } })
    if (mode === 'denied') return fail(404, 'NOT_FOUND')
    if (mode === 'error') return fail(503, 'LOG_RETENTION_UNAVAILABLE')
    if (request.method() === 'PATCH') {
      const body = request.postDataJSON() as LogRetentionWrite
      writes.push({ body, headers: request.headers() })
      if (mode === 'write-error') return fail(503, 'LOG_RETENTION_UNAVAILABLE')
      if (mode === 'conflict') return fail(412, 'REVISION_CONFLICT')
      const previous = policy.revision
      policy = { ...policy, retentionDays: mode === 'replay' ? 120 : body.retentionDays, revision: previous + (mode === 'replay' ? 2 : 1), source: 'PLATFORM_SETTING', modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-23T14:00:00Z' }
      return route.fulfill({ json: { item: { revision: previous + 1, changed: true, replayed: mode === 'replay' } } })
    }
    if (mode === 'refresh-error' && policy.revision > 1) return fail(503, 'LOG_RETENTION_UNAVAILABLE')
    return route.fulfill({ json: { item: mode === 'invalid' ? { ...policy, retentionDays: 0 } : policy } })
  })
  return { writes, setMode: (value: RetentionMode) => { mode = value } }
}
