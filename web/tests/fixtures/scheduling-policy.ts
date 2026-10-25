import type { Page } from '@playwright/test'
import type { SchedulingPolicy, SchedulingPolicyWrite } from '../../src/api/schedulingPolicy'
import { timingFixture } from './scheduling-timing'
import { candidateFixture } from './scheduling-candidates'
export function schedulingPolicy(maximum: number | null = 8, revision = 1): SchedulingPolicy {
  return { maxConcurrentExecutions: maximum, revision, source: maximum === null ? 'UNCONFIGURED' : 'PLATFORM_SETTING', applicationMode: 'NEXT_CLAIM', restartRequired: false, ...(maximum === null ? {} : { modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T00:00:00Z' }), usage: { visibility: 'AVAILABLE', heldExecutions: 3, runningExecutions: 2, observedAt: '2026-10-24T00:00:00Z' } }
}
export type SchedulingMode = 'ready' | 'unconfigured' | 'restricted' | 'denied' | 'error' | 'invalid' | 'write-error' | 'conflict' | 'replay'
export async function schedulingFixture(page: Page, initial: SchedulingMode = 'ready') {
  await timingFixture(page)
  await candidateFixture(page)
  let mode = initial, policy = schedulingPolicy(initial === 'unconfigured' ? null : 8)
  const writes: { body: SchedulingPolicyWrite; headers: Record<string, string> }[] = []
  await page.route('**/api/v1/settings/scheduling', async route => {
    const request = route.request(), fail = (status: number, code: string) => route.fulfill({ status, json: { code, message: 'synthetic-only', retryable: status >= 500 } })
    if (mode === 'denied') return fail(404, 'NOT_FOUND')
    if (mode === 'error') return fail(503, 'SCHEDULING_POLICY_UNAVAILABLE')
    if (request.method() === 'PATCH') {
      const body = request.postDataJSON() as SchedulingPolicyWrite
      writes.push({ body, headers: request.headers() })
      if (mode === 'write-error') return fail(503, 'SCHEDULING_POLICY_UNAVAILABLE')
      if (mode === 'conflict') return fail(412, 'REVISION_CONFLICT')
      const revision = policy.revision
      policy = schedulingPolicy(mode === 'replay' ? 12 : body.maxConcurrentExecutions, revision + (mode === 'replay' ? 2 : 1))
      return route.fulfill({ json: { item: { revision: revision + 1, changed: true, replayed: mode === 'replay' } } })
    }
    const item = mode === 'restricted' ? { ...policy, usage: { ...policy.usage, visibility: 'RESTRICTED', heldExecutions: null, runningExecutions: null } } : mode === 'invalid' ? { ...policy, maxConcurrentExecutions: 0 } : policy
    return route.fulfill({ json: { item } })
  })
  return { writes, setMode: (value: SchedulingMode) => { mode = value } }
}
