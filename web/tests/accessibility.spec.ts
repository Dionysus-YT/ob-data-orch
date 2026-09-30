import { chromium, expect, test } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import { writeFile } from 'node:fs/promises'
import { mockFacts } from './fixtures/facts'

test('原生 200% 浏览器缩放下四代表页与抽屉可达', async ({ browserName }, info) => {
  // 独立扩展浏览器包含冷启动、四页导航和原生截图，不能沿用单页用例预算。
  test.setTimeout(180000)
  expect(browserName).toBe('chromium')
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext('', {
    channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined,
    args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage()
    await mockFacts(page)
    await page.goto('http://127.0.0.1:15174/data-sources?uiFixture=data-sources')
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(options: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const tabs = await api.tabs.query({})
      const target = tabs.find(tab => tab.url.includes('127.0.0.1:15174'))
      if (!target) throw new Error('隔离测试标签不存在')
      await api.tabs.setZoom(target.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    const after = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    expect(Math.abs(after.width - before.width / 2)).toBeLessThanOrEqual(1)
    for (const path of ['/data-sources?uiFixture=data-sources', '/exports/new', '/', '/tasks/synthetic-task']) {
      await page.goto(`http://127.0.0.1:15174${path}`)
      await expect(page.locator('h1').first()).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), path).toBe(true)
    }
    await page.goto('http://127.0.0.1:15174/data-sources?uiFixture=data-sources')
    await page.getByRole('button', { name: '新建数据源', exact: true }).click()
    await page.getByRole('menuitem', { name: 'OceanBase MySQL', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '新建数据源', exact: true })
    await expect(dialog).toBeVisible()
    const bounds = await dialog.boundingBox()
    expect(bounds!.width).toBeLessThanOrEqual(after.width)
    await expect(dialog.getByRole('button', { name: '确定', exact: true })).toBeInViewport()
    // 浏览器缩放下 Playwright 的 fullPage 裁剪使用 CSS 宽度；原生截图保留实际整个窗口。
    const session = await context.newCDPSession(page)
    const capture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('native-200-percent.png'), Buffer.from(capture.data, 'base64'))
    await info.attach('native-browser-zoom', { body: JSON.stringify({ before, after }), contentType: 'application/json' })
  } finally { await context.close() }
})

test('抽屉 Tab 与 Shift+Tab 焦点约束、焦点可见、ESC 返回及减少动画', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.route('**/api/v1/**', route => route.abort())
  await page.goto('/data-sources?uiFixture=data-sources')
  await page.getByRole('button', { name: '新建数据源', exact: true }).click()
  await page.getByRole('menuitem', { name: 'OceanBase MySQL', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '新建数据源', exact: true })
  await expect(dialog).toBeVisible()
  for (const key of ['Tab', 'Shift+Tab']) {
    for (let i = 0; i < 24; i++) {
      await page.keyboard.press(key)
      expect(await dialog.evaluate(el => el.contains(document.activeElement))).toBe(true)
    }
  }
  await page.keyboard.press('Tab')
  expect(await page.evaluate(() => getComputedStyle(document.activeElement!).outlineStyle)).not.toBe('none')
  expect(await dialog.evaluate(el => getComputedStyle(el).animationDuration)).toBe('0s')
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  expect(await page.evaluate(() => document.activeElement?.tagName)).not.toBe('BODY')
})

test('权限拒绝隐藏数据、数量与新建操作', async ({ page }) => {
  await page.route('**/api/v1/**', route => route.fulfill({ status: 403, json: { code: 'FORBIDDEN' } }))
  await page.goto('/data-sources')
  await expect(page.getByRole('alert')).toContainText('当前身份无权访问数据源')
  await expect(page.getByRole('button', { name: '新建数据源', exact: true })).toHaveCount(0)
  await expect(page.getByRole('navigation', { name: '数据源分页' })).toHaveCount(0)
  await expect(page.getByRole('table')).toHaveCount(0)
})
