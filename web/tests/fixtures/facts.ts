import { DATA_SOURCE_UI_FIXTURES } from '../../src/views/dataSourceUiFixture'

export async function mockFacts(page: import('@playwright/test').Page) {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const fixedTime = '2026-09-16T00:00:00Z'
    let body: object = { items: [] }
    if (path === '/api/v1/data-sources') body = { items: DATA_SOURCE_UI_FIXTURES, total: 7, nextCursor: '' }
    else if (path === '/api/v1/execution-nodes') body = { items: [{ id: 'synthetic-node', displayName: '合成执行节点', platform: 'WINDOWS_AMD64' }] }
    else if (path === '/api/v1/tasks/synthetic-task') body = { item: { id: 'synthetic-task', type: 'OBDUMPER_EXPORT', precheckId: 'synthetic-precheck', state: 'FAILED', dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', submittedAt: fixedTime } }
    else if (path.endsWith('/execution')) body = { item: { state: 'FAILED', executionId: 'synthetic-execution', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', startedAt: fixedTime, finishedAt: fixedTime, updatedAt: fixedTime, reconciliationRequired: false } }
    else if (path.endsWith('/snapshot')) body = { item: { type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', metadataVersion: 'synthetic-metadata', capabilityVersion: 'synthetic-capability', dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', database: 'synthetic', table: 'fixture', format: 'CSV', filePath: 'synthetic-output', precheckId: 'synthetic-precheck', toolVersion: '4.3.5', configFingerprint: 'synthetic-fingerprint', objectSummary: 'synthetic.fixture' } }
    else if (path.endsWith('/command-evidence')) body = { item: { command: 'obdumper --csv', kind: 'PLANNED', redaction: 'PASSWORD_ONLY', configFingerprint: 'synthetic-fingerprint' } }
    else if (path.endsWith('/logs')) body = { items: [{ sourceSeq: 1, receivedAt: fixedTime, kind: 'STDERR', integrityCode: 'COMPLETE', message: '合成失败证据；未启动真实工具。' }], integrity: 'COMPLETE' }
    await route.fulfill({ json: body })
  })
}
