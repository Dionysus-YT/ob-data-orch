import type { Page } from '@playwright/test'
import type { SecurityPolicy } from '../../src/api/securityPolicy'

export function securityPolicy(): SecurityPolicy { return { capturedAt: '2026-10-24T01:00:00Z', loadedAt: '2026-10-24T00:00:00Z', source: 'STARTUP_CONFIGURATION', builtInRedaction: true, credentialFormat: 'credential-aes-gcm-v1', credentialKeyId: 'synthetic-key-v1', rootKeyStatus: 'LOADED', rootKeySource: 'LOCAL_PROTECTED_FILE', sensitiveCommandView: 'NOT_AVAILABLE', productionConfirmation: 'TASK_SUBMISSION_RULES', auditRetentionDays: null, auditAutomaticCleanup: false, realExecutionEnabled: true, normalImportExecutionEnabled: true, directLoadProbeEnabled: false, directLoadExecutionEnabled: false } }
export type SecurityMode = 'ready' | 'denied' | 'error' | 'invalid'
export async function securityFixture(page: Page, initial: SecurityMode = 'ready') {
  let mode = initial
  const calls: string[] = []
  await page.route('**/api/v1/settings/security', async route => {
    calls.push(route.request().method())
    if (mode === 'denied' || mode === 'error') return route.fulfill({ status: mode === 'denied' ? 404 : 503, json: { code: mode === 'denied' ? 'NOT_FOUND' : 'SECURITY_POLICY_UNAVAILABLE', message: 'synthetic-only', retryable: mode === 'error' } })
    return route.fulfill({ json: { item: mode === 'invalid' ? { ...securityPolicy(), auditAutomaticCleanup: true } : securityPolicy() } })
  })
  return { calls, setMode: (value: SecurityMode) => { mode = value } }
}
