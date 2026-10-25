import { expect, test, type Page } from '@playwright/test'
import { mockFacts } from './fixtures/facts'

const time = '2026-10-19T00:00:00Z'
const percent = (phase: string, amount: number, incomplete = false) => ({ phase, percent: amount, completedTasks: 7, runningTasks: 3, updatedAt: time, incomplete })

async function fixture(page: Page) {
  await mockFacts(page)
  await page.addInitScript(() => {
    class SyntheticEventSource extends EventTarget { close() {} }
    Object.assign(window, { EventSource: SyntheticEventSource })
  })
  const types = ['OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT'] as const
  const items = [
    { id: 'export-progress', displayName: '合成导出进度', type: types[0], state: 'RUNNING', progress: percent('EXPORT_DATA', 63.25) },
    { id: 'import-zero', displayName: '合成导入零进度', type: types[1], state: 'RUNNING', progress: percent('IMPORT_DATA', 0) },
    { id: 'import-completing', displayName: '合成导入等待核验', type: types[1], state: 'RUNNING', progress: percent('IMPORT_DATA', 100) },
    { id: 'stage-only', displayName: '合成结构阶段', type: types[0], state: 'RUNNING', progress: { phase: 'EXPORT_SCHEMA_COMPLETE', updatedAt: time, incomplete: false } },
    { id: 'unknown', displayName: '合成未知进度', type: types[0], state: 'RUNNING', progress: undefined },
    { id: 'failed', displayName: '合成失败末次进度', type: types[1], state: 'FAILED', progress: percent('IMPORT_DATA', 42.5) },
    { id: 'gap', displayName: '合成缺口进度', type: types[0], state: 'RUNNING', progress: percent('EXPORT_DATA', 55, true) },
    { id: 'success', displayName: '合成任务完成', type: types[1], state: 'SUCCEEDED', progress: undefined },
    { id: 'cancelled', displayName: '合成取消末次进度', type: types[0], state: 'CANCELLED', progress: percent('EXPORT_DATA', 30) },
    { id: 'cancelled-complete', displayName: '合成取消百分百进度', type: types[0], state: 'CANCELLED', progress: percent('EXPORT_DATA', 100) },
  ].map(item => ({ ...item, dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', stageEvidence: item.progress ? 'RELIABLE' : 'UNAVAILABLE', progressEvidence: item.progress && 'percent' in item.progress ? 'PERCENT' : 'UNAVAILABLE', reconciliationRequired: false, ownedByCurrentUser: true, submittedAt: time, startedAt: time, updatedAt: time, dataSourceDeleted: false, nodeDeleted: false, cancelRequested: false, availableActions: ['COPY_COMMAND', 'VIEW_LOGS'] }))
  let denied = false
  await page.route('**/api/v1/tasks**', route => {
    const path = new URL(route.request().url()).pathname
    if (denied) return route.fulfill({ status: 403, json: {} })
    if (path === '/api/v1/tasks/filter-options') return route.fulfill({ json: { dataSources: [], nodes: [], creators: [], environments: [], creatorFilterAvailable: false } })
    if (path === '/api/v1/tasks') return route.fulfill({ json: { items, totalPages: 1 } })
    const item = items.find(item => item.id === path.split('/')[4])!
    if (path.endsWith('/execution')) return route.fulfill({ json: { item: { state: item.state, stageEvidence: item.stageEvidence, progressEvidence: item.progressEvidence, progress: item.progress, reconciliationRequired: false, startedAt: time, updatedAt: time } } })
    if (path.endsWith('/snapshot')) return route.fulfill({ json: { item: { type: item.type, snapshotVersion: item.type === types[0] ? 'v2' : 'normal-import-v1', dataSourceId: item.dataSourceId, nodeId: item.nodeId, precheckId: 'synthetic-check', configFingerprint: 'synthetic-fingerprint', toolVersion: '4.3.5-RELEASE', metadataVersion: 'synthetic', capabilityVersion: 'synthetic', format: 'CSV' } } })
    if (path.endsWith('/command-evidence')) return route.fulfill({ json: { item: { kind: 'PLANNED', command: 'synthetic-tool --password ******', redaction: 'PASSWORD_ONLY' } } })
    if (path.endsWith('/logs') || path.endsWith('/operations')) return route.fulfill({ json: { items: [], integrity: 'COMPLETE' } })
    return route.fulfill({ json: { item: { id: item.id, type: item.type, dataSourceId: item.dataSourceId, nodeId: item.nodeId, precheckId: 'synthetic-check', submittedAt: time, presentation: item } } })
  })
  return { items, deny: () => { denied = true } }
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`两类任务进度在 ${viewport.width} 展示可靠百分比、阶段、未知及终态，列表详情共用`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.setViewportSize(viewport)
    const f = await fixture(page)
    await page.goto('/tasks')
    const row = (name: string) => page.getByRole('row').filter({ has: page.getByRole('link', { name, exact: true }) })
    await expect(row('合成导出进度')).toContainText('63.25%')
    await expect(row('合成导入零进度')).toContainText('0%')
    await expect(row('合成导入零进度').getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0')
    await expect(row('合成导入零进度').getByRole('progressbar')).toHaveAttribute('aria-valuemin', '0')
    await expect(row('合成导入零进度').getByRole('progressbar')).toHaveAttribute('aria-valuemax', '100')
    await expect(row('合成导入等待核验')).toContainText('100%')
    await expect(row('合成导入等待核验')).toContainText('运行中')
    await expect(row('合成导入等待核验')).not.toContainText('已完成')
    await expect(row('合成导入等待核验').getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100')
    await expect(row('合成导入等待核验').getByRole('progressbar')).not.toHaveClass(/ant-progress-status-success/)
    await expect(row('合成结构阶段')).toContainText('结构导出完成')
    await expect(row('合成结构阶段').locator('.ant-progress')).toHaveCount(0)
    await expect(row('合成未知进度').locator('.ant-progress')).toHaveCount(0)
    await expect(row('合成失败末次进度')).toContainText('42.5%')
    await expect(row('合成失败末次进度')).toContainText('最后记录')
    await expect(row('合成缺口进度')).toContainText('最后记录')
    await expect(row('合成取消末次进度')).toContainText('30%')
    await expect(row('合成取消百分百进度')).toContainText('最后记录')
    await expect(row('合成取消百分百进度')).toContainText('已取消')
    await expect(row('合成取消百分百进度').getByRole('progressbar')).toHaveClass(/ant-progress-status-normal/)
    await expect(row('合成取消百分百进度').getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100')
    await expect(row('合成任务完成')).toContainText('已完成')
    await expect(row('合成任务完成').locator('.ant-progress')).toHaveCount(0)
    const firstRow = row('合成导出进度')
    await expect(firstRow.getByRole('button', { name: '查看详情', exact: true })).toBeInViewport({ ratio: 1 })
    const tableBounds = await page.getByRole('region', { name: '当前授权范围内的任务', exact: true }).boundingBox()
    const actionBounds = await firstRow.getByRole('cell').last().boundingBox()
    const progressBounds = await firstRow.locator('.task-progress').boundingBox()
    expect(actionBounds!.x + actionBounds!.width).toBeLessThanOrEqual(tableBounds!.x + tableBounds!.width + 1)
    expect(progressBounds!.x + progressBounds!.width).toBeLessThanOrEqual(actionBounds!.x)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath(`task-progress-list-${viewport.width}.png`), fullPage: true })
    f.items[0]!.progress = percent('EXPORT_DATA', 100)
    await page.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(row('合成导出进度')).toContainText('100%')
    await page.getByRole('link', { name: '合成导出进度', exact: true }).click()
    await expect(page).toHaveURL(/\/tasks\/export-progress$/)
    const surface = page.getByRole('region', { name: '任务详情工作面', exact: true })
    await expect(surface).toContainText('100%')
    await expect(surface).toContainText('导出数据')
    await expect(surface).toContainText('运行中')
    await expect(surface.locator('.task-progress')).toHaveAttribute('aria-label', /等待收尾与结果核验/)
    await expect(surface.getByRole('progressbar')).toHaveAttribute('aria-valuetext', /不表示记录数、字节数或数据库结果/)
    await surface.locator('.task-progress').focus()
    await expect(page.getByRole('tooltip')).toContainText('数据子任务')
    await page.keyboard.press('Escape')
    await expect(page.getByRole('tooltip')).toHaveCount(0)
    await expect(surface.locator('.task-progress')).toBeFocused()
    await page.mouse.move(0, 0)
    await page.screenshot({ path: info.outputPath(`task-progress-detail-${viewport.width}.png`), fullPage: true })
    await page.goto('/tasks/import-zero')
    await expect(surface).toContainText('导入数据')
    await expect(surface).toContainText('0%')
    await expect(surface.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0')
    await expect(surface).not.toContainText('文件数')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath(`task-progress-import-detail-${viewport.width}.png`), fullPage: true })
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    expect(errors).toEqual([])
  })
}

test('权限撤销后清除列表进度，不保留上一个授权会话的数值', async ({ page }) => {
  const f = await fixture(page)
  await page.goto('/tasks')
  await expect(page.getByText('63.25%', { exact: true })).toBeVisible()
  f.deny()
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByText('当前身份无权查看任务', { exact: true })).toBeVisible()
  await expect(page.getByText('63.25%', { exact: true })).toHaveCount(0)
  await expect(page.locator('.task-progress')).toHaveCount(0)
})

test('权限撤销后清除详情进度，不泄露旧执行事实', async ({ page }) => {
  const f = await fixture(page)
  await page.goto('/tasks/export-progress')
  await expect(page.getByText('63.25%', { exact: true })).toBeVisible()
  f.deny()
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByText('任务不存在或当前身份无权查看。', { exact: true })).toBeVisible()
  await expect(page.getByText('63.25%', { exact: true })).toHaveCount(0)
  await expect(page.locator('.task-progress')).toHaveCount(0)
})

test('未知执行状态只展示最后可信进度，不以活跃进度暗示正在执行', async ({ page }) => {
  const f = await fixture(page)
  f.items[0]!.state = 'UNKNOWN_STATE'
  await page.goto('/tasks')
  const row = page.getByRole('row').filter({ has: page.getByRole('link', { name: '合成导出进度', exact: true }) })
  await expect(row).toContainText('状态核对中')
  await expect(row).toContainText('最后记录')
  await expect(row.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '63.25')
  await expect(row.getByRole('progressbar')).toHaveClass(/ant-progress-status-normal/)
  for (const state of ['constructor', '__proto__']) {
    f.items[0]!.state = state
    await page.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(page.getByRole('button', { name: '刷新', exact: true })).toBeEnabled()
    await expect(row).toContainText('状态核对中')
    await expect(row).toContainText('最后记录')
    await expect(row.getByRole('progressbar')).toHaveClass(/ant-progress-status-normal/)
  }
})
