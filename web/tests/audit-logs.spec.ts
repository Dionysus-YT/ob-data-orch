import { chromium, expect, test } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { auditLogFixture, type AuditMode } from './fixtures/audit-logs'

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
 test(`审计预设查询、详情、分页与下载 ${viewport.width}`, async ({ page }, info) => {
  const fixture = await auditLogFixture(page), errors: string[] = []; page.on('pageerror', error => errors.push(error.message)); await page.setViewportSize(viewport); await page.goto('/logs?preset=audit')
  await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeVisible(); await expect(page.getByText('选择时间与条件后查询审计日志', { exact: true })).toBeVisible(); expect(fixture.queries).toHaveLength(0)
  await page.getByRole('button', { name: '查询', exact: true }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('button', { name: '查看详情', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('audit-query.png'), fullPage: true })
  const detailButton = page.getByRole('button', { name: '查看详情', exact: true }); await detailButton.focus(); await page.keyboard.press('Enter')
  const drawer = page.getByRole('dialog', { name: '审计事件详情' }); await expect(drawer).toBeVisible(); await expect(drawer.getByText('部分历史变更详情暂不展示，查询与下载范围相同。', { exact: true })).toBeVisible(); await expect(drawer.getByRole('heading', { name: '审计事件详情' })).toBeFocused()
  await page.screenshot({ path: info.outputPath('audit-detail.png'), fullPage: true }); await page.keyboard.press('Escape'); await expect(drawer).toBeHidden(); await expect(detailButton).toBeFocused()
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(page.getByText('第 2 页', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '上一页', exact: true }).click(); await expect(page.getByText('第 1 页', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '下载当前结果', exact: true }).click(); const modal = page.getByRole('dialog', { name: '下载脱敏审计日志' }); await expect(modal).toBeVisible(); await page.screenshot({ path: info.outputPath('audit-download-confirm.png'), fullPage: true })
  const download = page.waitForEvent('download'); await modal.getByRole('button', { name: '确认下载', exact: true }).click(); expect((await download).suggestedFilename()).toBe('audit-logs.log'); expect(fixture.downloads).toHaveLength(1); expect(fixture.downloads[0]?.csrf).toBe('local-mvp-csrf-v1'); expect(fixture.downloads[0]?.body).toEqual({ token: 'snapshot-1' }); expect(errors).toEqual([])
 })
}

for (const mode of ['empty', 'truncated', 'error', 'invalid', 'denied'] as AuditMode[]) {
 test(`审计边界 ${mode}`, async ({ page }, info) => {
  await auditLogFixture(page, mode); await page.goto('/logs?preset=audit'); await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeVisible(); await page.getByRole('button', { name: '查询', exact: true }).click()
  if (mode === 'empty') await expect(page.getByText('当前有权范围与条件内没有匹配事件', { exact: true })).toBeVisible()
  else if (mode === 'truncated') { await expect(page.getByText('已达到 50000 条或 64 MiB 扫描限制，请缩小条件；当前快照不能下载。', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '下载当前结果', exact: true })).toBeDisabled() }
  else { await expect(page.getByText(mode === 'denied' ? '审计访问资格已失效，旧结果已清除。' : '审计读取未完成，条件已保留，请重新查询。', { exact: true })).toBeVisible(); await expect(page.getByText('synthetic-source', { exact: true })).toBeHidden() }
  await expect(page.getByText('synthetic-unsafe-diff', { exact: true })).toBeHidden(); await page.screenshot({ path: info.outputPath(`audit-${mode}.png`), fullPage: true })
 })
}
test('管理员无显式能力时不显示审计预设，直接地址不发起审计查询', async ({ page }) => {
 const fixture = await auditLogFixture(page); fixture.setAllowed(false); await page.goto('/logs?preset=audit'); await expect(page.getByText('审计日志需单独授予“审计访问”能力，系统管理员角色不会自动获得。', { exact: true })).toBeVisible(); await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeHidden(); expect(fixture.queries).toHaveLength(0)
})
test('详情中撤权清除旧结果并关闭审计预设', async ({ page }) => {
 const fixture = await auditLogFixture(page); await page.goto('/logs?preset=audit'); await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeVisible(); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByRole('button', { name: '查看详情', exact: true })).toBeVisible(); fixture.denyDetail(); await page.getByRole('button', { name: '查看详情', exact: true }).click(); await expect(page.getByText('审计访问资格已失效，旧结果已清除。', { exact: true })).toBeVisible(); await expect(page.getByRole('dialog', { name: '审计事件详情' })).toBeHidden(); await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeHidden()
})
test('预设隔离与字段错误不触发查询', async ({ page }) => {
 const fixture = await auditLogFixture(page); await page.goto('/logs?preset=audit'); await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeVisible(); await page.getByRole('textbox', { name: '对象 ID', exact: true }).fill('*'); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByText('请填写有效的精确 ID。', { exact: true })).toBeVisible(); expect(fixture.queries).toHaveLength(0)
 await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByRole('button', { name: '查看详情', exact: true })).toBeVisible(); await page.getByRole('tab', { name: '任务日志', exact: true }).click(); await expect(page.getByText('选择时间与条件后查询任务日志', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toBeVisible(); expect(fixture.task.queries).toHaveLength(1)
 await page.getByRole('tab', { name: '审计日志', exact: true }).click(); await expect(page.getByText('选择时间与条件后查询审计日志', { exact: true })).toBeVisible(); expect(fixture.queries).toHaveLength(1)
})

test('审计原生 200% 缩放、键盘与分页', async ({ browserName }, info) => {
 test.skip(browserName !== 'chromium'); test.setTimeout(120000)
 const root = resolve(info.outputDir); await mkdir(root, { recursive: true }); const profile = await mkdtemp(resolve(root, 'audit-zoom-')), extension = fileURLToPath(new URL('./fixtures/zoom-extension/', import.meta.url))
 const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
 try {
  const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage(); await auditLogFixture(page); await page.goto('http://127.0.0.1:15174/logs?preset=audit'); await expect(page.getByRole('tab', { name: '审计日志', exact: true })).toBeVisible()
  const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
  await worker.evaluate(async () => { const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome; const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在'); await api.tabs.setZoom(tab.id, 2) })
  await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2); await page.getByRole('button', { name: '查询', exact: true }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('button', { name: '查看详情', exact: true })).toBeVisible(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const cdp = await context.newCDPSession(page); let capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('audit-native-200-query.png'), Buffer.from(capture.data, 'base64'))
  await page.getByRole('button', { name: '查看详情', exact: true }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('dialog', { name: '审计事件详情' })).toBeVisible(); capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('audit-native-200-detail.png'), Buffer.from(capture.data, 'base64')); await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '下一页', exact: true }).focus(); await page.keyboard.press('Enter'); await expect(page.getByText('第 2 页', { exact: true })).toBeVisible(); capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('audit-native-200-pagination.png'), Buffer.from(capture.data, 'base64')); await cdp.detach(); await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })) }), contentType: 'application/json' })
 } finally { await context.close(); await removeAuditProfile(root, profile) }
})
async function removeAuditProfile(root: string, profile: string) { if (!resolve(profile).startsWith(`${resolve(root)}${sep}`)) throw new Error('临时目录越界'); await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 }) }
