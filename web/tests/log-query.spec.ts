import { chromium, expect, test, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { logQueryFixture, type LogMode } from './fixtures/log-query'
import { monitorNativeWindowErrors } from './fixtures/native-window-errors'

const verifyNative = new WeakMap<Page, () => Promise<void>>()
test.beforeEach(async ({ page }) => {
  verifyNative.set(page, await monitorNativeWindowErrors(page))
})
test.afterEach(async ({ page }) => {
  await verifyNative.get(page)?.()
})

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`日志查询分页、同源上下文与键盘 ${viewport.width}`, async ({ page }, info) => {
    const model = await logQueryFixture(page), errors: string[] = []; page.on('pageerror', e => errors.push(e.message)); await page.setViewportSize(viewport); await page.goto('/logs')
    await expect(page.getByRole('heading', { name: '日志中心', exact: true })).toBeVisible(); expect(model.queries).toHaveLength(0)
    await expect(page.getByText('选择时间与条件后查询任务日志', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '查询', exact: true }).focus(); await page.keyboard.press('Enter')
    await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toBeVisible(); expect(model.queries).toHaveLength(1)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); expect(await page.evaluate(() => (window as unknown as { syntheticUnsafe?: boolean }).syntheticUnsafe)).toBeUndefined()
    await page.screenshot({ path: info.outputPath('logs-query.png'), fullPage: true })
    await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(page.getByText('第 2 页', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '上一页', exact: true }).click(); await expect(page.getByText('第 1 页', { exact: true })).toBeVisible()
    expect(model.queries.at(-1)?.searchParams.get('cursor')).toBe('page-1')
    const context = page.getByRole('button', { name: '查看上下文', exact: true }); await context.focus(); await page.keyboard.press('Enter')
    const drawer = page.getByRole('dialog', { name: '同来源日志上下文' }); await expect(drawer).toBeVisible(); await expect(drawer.getByRole('heading', { name: '同来源日志上下文' })).toBeFocused(); await expect(drawer.getByRole('list', { name: '同来源上下文记录' }).getByRole('listitem')).toHaveCount(3)
    const sourceDetails = drawer.getByRole('button', { name: /来源详情$/ })
    await expect(sourceDetails).toHaveAttribute('aria-expanded', 'false'); await sourceDetails.click()
    await expect(drawer.getByText('未取得，记录按平台接收时间展示', { exact: true })).toBeVisible()
    await expect(drawer.getByText('来源代次', { exact: true })).toBeVisible(); await sourceDetails.click()
    await page.keyboard.press('Shift+Tab'); expect(await drawer.evaluate(el => el.contains(document.activeElement))).toBe(true)
    await page.screenshot({ path: info.outputPath('logs-context.png'), fullPage: true }); await page.keyboard.press('Escape'); await expect(drawer).toBeHidden(); await expect(context).toBeFocused()
    await page.getByRole('textbox', { name: '脱敏文本关键字', exact: true }).fill('合成'); await expect(page.getByText('筛选条件已修改，当前仍为上一次查询结果；请点击查询应用条件。', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '下载当前结果' })).toBeDisabled()
    await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.locator('mark').first()).toHaveText('合成'); expect(errors).toEqual([])
  })
}
for (const [mode, message] of [['empty', '当前条件没有匹配日志'], ['not-collected', '授权范围内尚无保留日志采集证据'], ['expired', '匹配范围的保留日志已清理'], ['truncated', '已达到 50000 条或 64 MiB 扫描限制，结果已截断，请缩小条件。'], ['gap', '本次扫描发现日志缺口或截断，已按采集事实保留标识。'], ['denied', '当前身份无法访问这些任务日志，请检查登录及任务权限。'], ['error', '日志查询未完成，条件与上次结果已保留，请重试。'], ['invalid', '日志查询未完成，条件与上次结果已保留，请重试。']] as const) {
  test(`日志查询状态 ${mode}`, async ({ page }, info) => {
    await logQueryFixture(page, mode as LogMode); await page.goto('/logs'); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByText(message, { exact: true })).toBeVisible()
    if (mode === 'truncated') { await expect(page.getByRole('button', { name: '下一页' })).toBeDisabled(); await expect(page.getByRole('button', { name: '下载当前结果' })).toBeDisabled() }
    await page.screenshot({ path: info.outputPath(`logs-${mode}.png`), fullPage: true })
  })
}
test('日志下载确认、完整任务范围与超限重试', async ({ page }, info) => {
  const model = await logQueryFixture(page); await page.goto('/logs?taskId=synthetic-task-1'); await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toBeVisible(); expect(model.queries).toHaveLength(1)
  await page.getByRole('button', { name: '下载当前结果', exact: true }).click(); const modal = page.getByRole('dialog', { name: '下载脱敏任务日志' }); await expect(modal).toBeVisible(); await page.screenshot({ path: info.outputPath('logs-download.png'), fullPage: true })
  const downloaded = page.waitForEvent('download'); await modal.getByRole('button', { name: '确认下载', exact: true }).click(); expect((await downloaded).suggestedFilename()).toBe('task-logs.log'); expect(model.actions[0]?.body).toEqual({ token: 'snapshot-1', fullTask: false }); expect(model.actions[0]?.csrf).toBeTruthy()
  await expect(modal).toBeHidden(); await page.getByRole('button', { name: '下载完整任务日志', exact: true }).click(); await expect(modal.getByText('下载此任务在快照水位前的全部保留日志，忽略当前时间、来源和关键字筛选。', { exact: true })).toBeVisible()
  model.failDownload(); await modal.getByRole('button', { name: '确认下载', exact: true }).click(); await expect(modal.getByText('下载超过 100000 条或 100 MiB，请缩小条件。', { exact: true })).toBeVisible(); expect(model.actions.at(-1)?.body.fullTask).toBe(true)
  await modal.getByRole('button', { name: '取消', exact: true }).click(); await page.getByRole('textbox', { name: '精确任务 ID', exact: true }).fill('changed-task'); await expect(page.getByRole('button', { name: '下载当前结果' })).toBeDisabled()
})

test('撤权清除旧结果且失败保留查询条件', async ({ page }) => {
  const model = await logQueryFixture(page); await page.goto('/logs'); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toBeVisible()
  model.setMode('error'); await page.getByRole('button', { name: '刷新日志快照' }).click(); await expect(page.getByText('日志查询未完成，条件与上次结果已保留，请重试。', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toBeVisible()
  model.setMode('denied'); await page.getByRole('button', { name: '刷新日志快照' }).click(); await expect(page.getByText('当前身份无法访问这些任务日志，请检查登录及任务权限。', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toHaveCount(0)
})

test('日志中心原生 200% 可查询与查看上下文', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium'); test.setTimeout(120000)
  const root = resolve(info.outputDir); await mkdir(root, { recursive: true }); const profile = await mkdtemp(resolve(root, 'logs-zoom-')), extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage()
    const verifyWindowErrors = await monitorNativeWindowErrors(page)
    await logQueryFixture(page); await page.goto('http://127.0.0.1:15174/logs')
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => { const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome; const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在'); await api.tabs.setZoom(tab.id, 2) })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    const query = page.getByRole('button', { name: '查询', exact: true }); await query.focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('button', { name: '查看上下文', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const cdp = await context.newCDPSession(page); let capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('logs-native-200-query.png'), Buffer.from(capture.data, 'base64'))
    await page.getByRole('button', { name: '查看上下文', exact: true }).focus(); await page.keyboard.press('Enter'); const drawer = page.getByRole('dialog', { name: '同来源日志上下文' }); await expect(drawer).toBeVisible(); await expect(drawer.getByRole('list', { name: '同来源上下文记录' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('logs-native-200-context.png'), Buffer.from(capture.data, 'base64')); await page.keyboard.press('Escape')
    const next = page.getByRole('button', { name: '下一页' }); await next.focus(); await page.keyboard.press('Enter'); await expect(page.getByText('第 2 页', { exact: true })).toBeVisible(); capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('logs-native-200-pagination.png'), Buffer.from(capture.data, 'base64')); await cdp.detach()
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })) }), contentType: 'application/json' })
    await verifyWindowErrors()
  } finally {
    await context.close()
    await removeLogsProfile(root, profile)
  }
})
async function removeLogsProfile(root: string, profile: string) {
  // 仅删除本测试新建并核对边界的临时配置目录，不能触及用户浏览器状态。
  if (!resolve(profile).startsWith(`${resolve(root)}${sep}`)) throw new Error('临时目录越界')
  await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 })
}
