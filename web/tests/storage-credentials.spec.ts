import { chromium, expect, test, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mockFacts } from './fixtures/facts'

async function prepare(page: Page) {
  await mockFacts(page)
  await page.route('**/api/v1/storage-credentials', route => route.request().method() === 'GET'
    ? route.fulfill({ json: { items: [{ id: 'synthetic-credential', displayName: '合成长名称存储凭据用于检查列表与抽屉的布局', provider: 'OSS', revision: 1, currentRevision: 1, updatedAt: '2026-10-01T00:00:00Z' }] } })
    : route.fulfill({ status: 503, json: { code: 'SYNTHETIC_UNAVAILABLE' } }))
}

async function checkEditor(page: Page, screenshot: (name: string) => Promise<void>) {
  await expect(page.getByRole('heading', { name: '存储凭据', exact: true })).toBeVisible()
  const workspace = page.getByRole('region', { name: '存储凭据工作面', exact: true })
  const create = workspace.getByRole('button', { name: '新增凭据', exact: true })
  await create.focus(); await page.keyboard.press('Enter')
  const drawer = page.getByRole('dialog', { name: '新增凭据', exact: true })
  await expect(drawer.getByRole('heading', { name: '新增凭据', exact: true })).toBeFocused()
  await drawer.getByRole('button', { name: '创建', exact: true }).click()
  await expect(drawer.getByLabel('AccessKey', { exact: true })).toHaveAttribute('aria-invalid', 'true')
  await drawer.getByLabel('凭据名称', { exact: true }).fill('合成新增凭据')
  await drawer.getByLabel('AccessKey', { exact: true }).fill('synthetic-access-fixture')
  await drawer.getByLabel('SecretKey', { exact: true }).fill('synthetic-secret-fixture')
  await drawer.getByRole('button', { name: '创建', exact: true }).click()
  await expect(drawer.getByRole('alert').filter({ hasText: '操作未完成' })).toBeVisible()
  await expect(drawer.getByLabel('SecretKey', { exact: true })).toHaveValue('synthetic-secret-fixture')
  await expect(drawer.locator('.ant-form-item-has-error')).toHaveCount(0)
  await page.keyboard.press('Shift+Tab'); expect(await drawer.evaluate(el => el.contains(document.activeElement))).toBe(true)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await screenshot('credential-editor-retry')
  await page.keyboard.press('Escape'); await expect(drawer).toBeHidden(); await expect(create).toBeFocused()
  await create.click(); await expect(drawer).toBeVisible()
  await expect(drawer.getByRole('alert').filter({ hasText: '操作未完成' })).toHaveCount(0)
  await expect(drawer.getByLabel('AccessKey', { exact: true })).toHaveValue('')
  await expect(drawer.getByLabel('SecretKey', { exact: true })).toHaveValue('')
  await drawer.getByRole('button', { name: '取消', exact: true }).click(); await expect(drawer).toBeHidden(); await expect(create).toBeFocused()
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`凭据单一工作面与抽屉会话 ${viewport.width}`, async ({ page }, info) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
    await prepare(page); await page.setViewportSize(viewport); await page.goto('/settings/storage-credentials')
    await expect(page).toHaveTitle(/OB Data Orch/)
    await expect(page.getByRole('region', { name: '存储凭据列表', exact: true })).toBeVisible()
    await expect(page.locator('.orch-management')).toHaveCount(1)
    await page.screenshot({ path: info.outputPath('credential-list.png'), fullPage: true })
    await checkEditor(page, async name => { await page.screenshot({ path: info.outputPath(`${name}.png`), fullPage: true }) })
    await page.getByRole('textbox', { name: '关键字', exact: true }).fill('synthetic-no-match')
    await expect(page.getByText('没有符合当前筛选条件的凭据', { exact: true })).toBeVisible()
    await page.getByRole('textbox', { name: '关键字', exact: true }).fill('')
    await expect(page.getByRole('button', { name: '轮换', exact: true })).toBeVisible()
    await expect(page.locator('vite-error-overlay')).toHaveCount(0); expect(errors).toEqual([])
  })
}

test('凭据抽屉原生 200% 缩放与键盘关闭清理', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium'); test.setTimeout(120000)
  const root = resolve(info.outputDir); await mkdir(root, { recursive: true })
  const profile = await mkdtemp(resolve(root, 'credentials-zoom-')), extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage()
    await prepare(page); await page.goto('http://127.0.0.1:15174/settings/storage-credentials')
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在')
      await api.tabs.setZoom(tab.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    const cdp = await context.newCDPSession(page)
    await checkEditor(page, async name => {
      const capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
      await writeFile(info.outputPath(`${name}-native-200.png`), Buffer.from(capture.data, 'base64'))
    })
    await cdp.detach()
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally {
    await context.close()
    await removeCredentialsProfile(root, profile)
  }
})

async function removeCredentialsProfile(root: string, profile: string) {
  // 只清理本测试新建且已核对边界的临时配置目录。
  if (!resolve(profile).startsWith(`${resolve(root)}${sep}`)) throw new Error('临时目录越界')
  await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 })
}
