import { chromium, expect, test } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { securityFixture, type SecurityMode } from './fixtures/security-policy'

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`安全状态只读事实与键盘刷新 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const model = await securityFixture(page), errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
    await page.setViewportSize(viewport); await page.goto('/settings/security')
    await expect(page.getByText('synthetic-key-v1', { exact: true })).toBeVisible()
    await expect(page.getByText('未配置期限，当前不自动清理；普通任务日志期限不适用于审计。', { exact: true })).toBeVisible()
    await expect(page.getByRole('textbox')).toHaveCount(0); await expect(page.getByRole('switch')).toHaveCount(0); await expect(page.getByRole('button', { name: /保存|修改密钥|删除/ })).toHaveCount(0)
    const refresh = page.getByRole('button', { name: '刷新安全状态' }); await refresh.focus(); await page.keyboard.press('Enter'); await expect.poll(() => model.calls.length).toBe(2)
    await expect(page.getByText('synthetic-key-v1', { exact: true })).toBeVisible(); await expect(refresh).toBeFocused(); expect(model.calls).toEqual(['GET', 'GET']); expect(errors).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath(`security-${viewport.width}.png`), fullPage: true })
    await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
  })
}

for (const mode of ['denied', 'error', 'invalid'] satisfies SecurityMode[]) test(`安全状态 ${mode} 不显示猜测事实`, async ({ page }, info) => {
  await securityFixture(page, mode); await page.goto('/settings/security')
  await expect(page.getByRole('alert').filter({ hasText: mode === 'denied' ? '系统管理员' : '保持未知' })).toBeVisible()
  await expect(page.getByText('synthetic-key-v1', { exact: true })).toHaveCount(0); await expect(page.locator('.ant-descriptions')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath(`security-${mode}.png`), fullPage: true })
})

test('刷新撤权清除已读事实并可恢复', async ({ page }) => {
  const model = await securityFixture(page); await page.goto('/settings/security'); await expect(page.getByText('synthetic-key-v1', { exact: true })).toBeVisible()
  model.setMode('denied'); await page.getByRole('button', { name: '刷新安全状态' }).click(); await expect(page.getByRole('alert').filter({ hasText: '系统管理员' })).toBeVisible(); await expect(page.locator('.ant-descriptions')).toHaveCount(0)
  model.setMode('ready'); await page.getByRole('button', { name: '刷新安全状态' }).click(); await expect(page.getByText('synthetic-key-v1', { exact: true })).toBeVisible(); expect(model.calls).toEqual(['GET', 'GET', 'GET'])
})

test('安全状态原生 200% 下可阅读并键盘刷新', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium', '原生缩放使用 Chromium 测试扩展'); test.setTimeout(120000)
  const root = resolve(info.outputDir); await mkdir(root, { recursive: true }); const profile = await mkdtemp(resolve(root, 'security-zoom-')), extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage(), model = await securityFixture(page)
    await page.goto('http://127.0.0.1:15174/settings/security'); await expect(page.getByText('synthetic-key-v1', { exact: true })).toBeVisible()
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => { const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome; const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在'); await api.tabs.setZoom(tab.id, 2) })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    const cdp = await context.newCDPSession(page)
    let capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('security-native-200-top.png'), Buffer.from(capture.data, 'base64'))
    const refresh = page.getByRole('button', { name: '刷新安全状态' }); await refresh.focus(); await expect(refresh).toBeInViewport(); await page.keyboard.press('Enter'); await expect.poll(() => model.calls.length).toBe(2); await expect(refresh).not.toHaveClass(/ant-btn-loading/); await expect(refresh).toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('security-native-200-refresh.png'), Buffer.from(capture.data, 'base64')); await cdp.detach()
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally {
    await context.close()
    await removeSecurityProfile(root, profile)
  }
})

async function removeSecurityProfile(root: string, profile: string) {
  // 只清理本测试在产物目录中创建的浏览器配置，不允许越界删除。
  if (!resolve(profile).startsWith(`${resolve(root)}${sep}`)) throw new Error('临时目录超出本轮产物范围')
  await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 })
}
