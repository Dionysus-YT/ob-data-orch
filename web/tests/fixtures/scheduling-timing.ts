import type { Page } from '@playwright/test'
import type { SchedulingTimingPolicy, SchedulingTimingWrite } from '../../src/api/schedulingTiming'

export function timingPolicy(heartbeat = 120, startup: number | null = null, polling: number | null = null, revision = 1): SchedulingTimingPolicy {
  const configured = heartbeat !== 120 || startup !== null || polling !== null
  return { heartbeatTimeoutSeconds: heartbeat, heartbeatTimeoutSource: heartbeat === 120 ? 'CONTROL_PLANE' : 'PLATFORM_SETTING', startupTimeoutSeconds: startup, pollingIntervalSeconds: polling, candidateStrategy: 'EXPLICIT_SELECTION', revision, applicationMode: 'HEARTBEAT_NOW_STARTUP_NEXT_CLAIM_POLL_NEXT_CHECK', restartRequired: false, ...(configured ? { modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T02:00:00Z' } : {}) }
}
export type TimingMode = 'ready' | 'configured' | 'denied' | 'error' | 'invalid' | 'write-error' | 'conflict' | 'replay' | 'read-after-write-error'
export async function timingFixture(page: Page, initial: TimingMode = 'ready') {
  let mode = initial, policy = initial === 'configured' ? timingPolicy(180, 60, 5, 2) : timingPolicy(), changed = false
  const writes: { body: SchedulingTimingWrite; headers: Record<string, string> }[] = []
  await page.route('**/api/v1/settings/scheduling/timing', async route => {
    const request = route.request(), fail = (status: number, code: string) => route.fulfill({ status, json: { code, message: 'synthetic-only', retryable: status >= 500 } })
    if (mode === 'denied') return fail(404, 'NOT_FOUND')
    if (mode === 'error' || mode === 'read-after-write-error' && changed) return fail(503, 'SCHEDULING_TIMING_UNAVAILABLE')
    if (request.method() === 'PATCH') {
      const body = request.postDataJSON() as SchedulingTimingWrite; writes.push({ body, headers: request.headers() })
      if (mode === 'write-error') return fail(503, 'SCHEDULING_TIMING_UNAVAILABLE')
      if (mode === 'conflict') return fail(412, 'REVISION_CONFLICT')
      const revision = policy.revision
      policy = mode === 'replay' ? timingPolicy(240, 120, 10, revision + 2) : timingPolicy(body.heartbeatTimeoutSeconds, body.startupTimeoutSeconds, body.pollingIntervalSeconds, revision + 1)
      changed = true
      return route.fulfill({ json: { item: { revision: revision + 1, changed: true, replayed: mode === 'replay' } } })
    }
    return route.fulfill({ json: { item: mode === 'invalid' ? { ...policy, startupTimeoutSeconds: 0 } : policy } })
  })
  return { writes, setMode: (value: TimingMode) => { mode = value } }
}
