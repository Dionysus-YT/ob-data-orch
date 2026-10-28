import { chromium, expect, test, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { toolCompatibility, toolsFixture, type ToolMode } from './fixtures/tool-compatibility'

async function expandCatalogs(page: Page) {
  await expect(page.getByRole('heading', { name: 'OBLOADER', exact: true })).toBeVisible()
  for (const toggle of await page.getByRole('button', { name: /参数目录$/ }).all()) {
    if (await toggle.getAttribute('aria-expanded') !== 'true') await toggle.click()
  }
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`工具目录分组、只读边界与键盘刷新 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const model = await toolsFixture(page), errors: string[] = []; page.on('pageerror', error => errors.push(error.message)); page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.setViewportSize(viewport); await page.goto('/settings/tools')
    await expect(page).toHaveTitle(/OB Data Orch/); await expect(page.getByRole('heading', { name: '工具与兼容', exact: true })).toBeVisible(); await expect(page.getByRole('heading', { name: 'OBDUMPER', exact: true })).toBeVisible()
    await expect(page.getByText('按任务配置选择，不设全平台单一活动版本', { exact: true })).toBeVisible()
    await expandCatalogs(page); await expect(page.getByText('obloader-4.3.5-normal-v1', { exact: true })).toBeVisible(); await expect(page.getByText('obloader-4.3.5-direct-v1', { exact: true })).toBeVisible()
    const historical = page.locator('.ant-descriptions-item').filter({ has: page.getByText('仅历史重放', { exact: true }) }); await expect(historical.getByText('obdumper-4.3.5-slice-v6', { exact: true })).toBeVisible(); await expect(historical.getByText('obdumper-4.3.5-slice-v8-object-selection', { exact: true })).toBeVisible()
    const first = await page.getByText('obdumper-4.3.5-slice-v5', { exact: true }).boundingBox(), second = await page.getByText('obdumper-4.3.5-slice-v7', { exact: true }).boundingBox()
    expect(first).not.toBeNull(); expect(second).not.toBeNull(); expect(second!.y).toBeGreaterThanOrEqual(first!.y + first!.height)
    await expect(page.getByRole('textbox')).toHaveCount(0); await expect(page.getByRole('switch')).toHaveCount(0); await expect(page.getByRole('combobox')).toHaveCount(0); await expect(page.getByRole('button', { name: /保存|安装|激活|上传|升级/ })).toHaveCount(0)
    const refresh = page.getByRole('button', { name: '刷新工具状态' }); await refresh.focus(); await page.keyboard.press('Enter'); await expect.poll(() => model.calls.length).toBe(2)
    await expect(page.getByRole('heading', { name: 'OBDUMPER', exact: true })).toBeVisible(); await expect(refresh).not.toHaveClass(/ant-btn-loading/); await expect(refresh).toBeFocused(); expect(model.calls).toEqual(['GET', 'GET']); expect(errors).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath(`tools-${viewport.width}.png`), fullPage: true })
    await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
  })
}

for (const mode of ['denied', 'error', 'invalid'] satisfies ToolMode[]) test(`工具状态 ${mode} 不显示猜测基线`, async ({ page }, info) => {
  await toolsFixture(page, mode); await page.goto('/settings/tools')
  await expect(page.getByRole('alert').filter({ hasText: mode === 'denied' ? '系统管理员' : '保持未知' })).toBeVisible()
  await expect(page.locator('.ant-descriptions')).toHaveCount(0); await expect(page.getByRole('heading', { name: 'OBDUMPER', exact: true })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath(`tools-${mode}.png`), fullPage: true })
})

test('工具刷新撤权后清除目录并可恢复', async ({ page }) => {
  const model = await toolsFixture(page); await page.goto('/settings/tools'); await expandCatalogs(page); await expect(page.getByText('obloader-4.3.5-normal-v1', { exact: true })).toBeVisible()
  model.setMode('denied'); await page.getByRole('button', { name: '刷新工具状态' }).click(); await expect(page.getByRole('alert').filter({ hasText: '系统管理员' })).toBeVisible(); await expect(page.locator('.ant-descriptions')).toHaveCount(0)
  model.setMode('ready'); await page.getByRole('button', { name: '刷新工具状态' }).click(); await expandCatalogs(page); await expect(page.getByText('obloader-4.3.5-normal-v1', { exact: true })).toBeVisible(); expect(model.calls).toEqual(['GET', 'GET', 'GET'])
})

test('工具状态加载期间不显示旧版静态目标', async ({ page }, info) => {
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/settings/tools', async route => { await pending; await route.fulfill({ json: { item: toolCompatibility() } }) })
  try {
    await page.goto('/settings/tools'); await expect(page.getByRole('status').filter({ hasText: '正在读取工具状态' })).toBeVisible(); await expect(page.locator('.ant-descriptions')).toHaveCount(0)
    await page.screenshot({ path: info.outputPath('tools-loading.png'), fullPage: true }); release(); await expect(page.getByRole('heading', { name: 'OBDUMPER', exact: true })).toBeVisible()
  } finally { release() }
})

test('工具状态原生 200% 下可阅读并键盘刷新', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium', '原生缩放使用 Chromium 测试扩展'); test.setTimeout(120000)
  const root = resolve(info.outputDir); await mkdir(root, { recursive: true }); const profile = await mkdtemp(resolve(root, 'tools-zoom-')), extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage(), model = await toolsFixture(page)
    await page.goto('http://127.0.0.1:15174/settings/tools'); await expect(page.getByRole('heading', { name: 'OBDUMPER', exact: true })).toBeVisible()
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => { const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome; const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在'); await api.tabs.setZoom(tab.id, 2) })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    const cdp = await context.newCDPSession(page)
    let capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('tools-native-200-top.png'), Buffer.from(capture.data, 'base64'))
    await expandCatalogs(page); await page.getByText('obdumper-4.3.5-slice-v11-expert-parameters', { exact: true }).scrollIntoViewIfNeeded()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('tools-native-200-catalogs.png'), Buffer.from(capture.data, 'base64'))
    const refresh = page.getByRole('button', { name: '刷新工具状态' }); await refresh.focus(); await expect(refresh).toBeInViewport(); await page.keyboard.press('Enter'); await expect.poll(() => model.calls.length).toBe(2); await expect(refresh).not.toHaveClass(/ant-btn-loading/); await expect(refresh).toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('tools-native-200-refresh.png'), Buffer.from(capture.data, 'base64')); await cdp.detach()
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally { await context.close(); await removeToolsProfile(root, profile) }
})

async function removeToolsProfile(root: string, profile: string) {
  // 只清理本测试创建的配置目录，核对绝对边界后删除，不能触及用户浏览器配置。
  if (!resolve(profile).startsWith(`${resolve(root)}${sep}`)) throw new Error('临时目录超出本轮产物范围')
  await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 })
}
