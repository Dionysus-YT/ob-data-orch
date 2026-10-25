import type { Page } from '@playwright/test'
import type { ToolCompatibility } from '../../src/api/toolCompatibility'

export function toolCompatibility(): ToolCompatibility {
  return { capturedAt: '2026-10-24T01:00:00Z', loadedAt: '2026-10-24T00:00:00Z', source: 'BUILT_IN_CATALOGS', tools: [
    { tool: 'OBDUMPER', supportedVersions: ['4.3.5-RELEASE'], defaultTargetVersion: '4.3.5-RELEASE', modules: [{ taskType: 'OBDUMPER_EXPORT', metadataSelection: 'TASK_CONFIGURATION', catalogs: [
      { metadataVersion: 'obdumper-4.3.5-slice-v5', usage: 'TASK_CONFIGURATION' }, { metadataVersion: 'obdumper-4.3.5-slice-v6', usage: 'HISTORICAL_REPLAY' }, { metadataVersion: 'obdumper-4.3.5-slice-v7', usage: 'TASK_CONFIGURATION' }, { metadataVersion: 'obdumper-4.3.5-slice-v8-object-selection', usage: 'HISTORICAL_REPLAY' }, { metadataVersion: 'obdumper-4.3.5-slice-v9-combined-object-selection', usage: 'TASK_CONFIGURATION' }, { metadataVersion: 'obdumper-4.3.5-slice-v10-ddl-text-formats', usage: 'TASK_CONFIGURATION' }, { metadataVersion: 'obdumper-4.3.5-slice-v11-expert-parameters', usage: 'TASK_CONFIGURATION' },
    ] }] },
    { tool: 'OBLOADER', supportedVersions: ['4.3.5-RELEASE'], defaultTargetVersion: '4.3.5-RELEASE', modules: [{ taskType: 'OBLOADER_NORMAL_IMPORT', metadataSelection: 'FIXED', catalogs: [{ metadataVersion: 'obloader-4.3.5-normal-v1', usage: 'TASK_CONFIGURATION' }] }, { taskType: 'OBLOADER_DIRECT_LOAD', metadataSelection: 'FIXED', catalogs: [{ metadataVersion: 'obloader-4.3.5-direct-v1', usage: 'TASK_CONFIGURATION' }] }] },
  ] }
}
export type ToolMode = 'ready' | 'denied' | 'error' | 'invalid'
export async function toolsFixture(page: Page, initial: ToolMode = 'ready') {
  let mode = initial
  const calls: string[] = []
  await page.route('**/api/v1/settings/tools', async route => {
    calls.push(route.request().method())
    if (mode === 'denied' || mode === 'error') return route.fulfill({ status: mode === 'denied' ? 404 : 503, json: { code: mode === 'denied' ? 'NOT_FOUND' : 'TOOL_COMPATIBILITY_UNAVAILABLE', message: 'synthetic-only', retryable: mode === 'error' } })
    return route.fulfill({ json: { item: mode === 'invalid' ? { ...toolCompatibility(), installedVersion: 'synthetic-guessed-version' } : toolCompatibility() } })
  })
  return { calls, setMode: (value: ToolMode) => { mode = value } }
}
