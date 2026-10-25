import type { Page } from '@playwright/test'
import type { CandidateOrder, SchedulingCandidatesPolicy, SchedulingCandidatesWrite } from '../../src/api/schedulingCandidates'
export function candidatePolicy(order: CandidateOrder = 'EXISTING', revision = 1): SchedulingCandidatesPolicy { return { order, revision, selectionMode: 'EXPLICIT_SELECTION', applicationMode: 'NEXT_CANDIDATE_READ', restartRequired: false, ...(revision === 1 ? {} : { modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T03:00:00Z' }) } }
export type CandidateMode = 'ready' | 'denied' | 'error' | 'invalid' | 'write-error' | 'conflict' | 'replay' | 'read-after-write-error'
export async function candidateFixture(page: Page, initial: CandidateMode = 'ready') {
  let mode = initial, policy = candidatePolicy(), changed = false
  const writes: { body: SchedulingCandidatesWrite; headers: Record<string, string> }[] = []
  await page.route('**/api/v1/settings/scheduling/candidates', async route => {
    const request = route.request(), fail = (status: number, code: string) => route.fulfill({ status, json: { code, message: 'synthetic-only', retryable: status >= 500 } })
    if (mode === 'denied') return fail(404, 'NOT_FOUND')
    if (mode === 'error' || mode === 'read-after-write-error' && changed) return fail(503, 'CANDIDATE_ORDER_UNAVAILABLE')
    if (request.method() === 'PATCH') {
      const body = request.postDataJSON() as SchedulingCandidatesWrite; writes.push({ body, headers: request.headers() })
      if (mode === 'write-error') return fail(503, 'CANDIDATE_ORDER_UNAVAILABLE')
      if (mode === 'conflict') return fail(412, 'REVISION_CONFLICT')
      const revision = policy.revision
      policy = mode === 'replay' ? candidatePolicy('EXISTING', revision + 2) : candidatePolicy(body.order, revision + 1); changed = true
      return route.fulfill({ json: { item: { revision: revision + 1, changed: true, replayed: mode === 'replay' } } })
    }
    return route.fulfill({ json: { item: mode === 'invalid' ? { ...policy, order: 'AUTO_SELECT' } : policy } })
  })
  return { writes, setMode: (value: CandidateMode) => { mode = value } }
}
