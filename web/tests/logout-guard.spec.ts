import { chromium, expect, test, type Page, type Route } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import { writeFile } from 'node:fs/promises'
import { normalImportNode, normalImportSources } from './fixtures/normal-import'

test.use({ actionTimeout: 15000, viewport: { width: 1920, height: 1080 } })
const health = new WeakMap<Page, { pageErrors: string[]; consoleErrors: string[] }>()
test.afterEach(async ({ page }, info) => {
  const observed = health.get(page)
  if (!observed) return
  expect(observed?.pageErrors).toEqual([])
  expect(observed?.consoleErrors).toEqual([])
  await expect(page.locator('vite-error-overlay')).toHaveCount(0)
  await info.attach('browser-environment', { body: JSON.stringify({ browser: page.context().browser()?.version(), viewport: page.viewportSize(), url: page.url() }), contentType: 'application/json' })
})

async function fixture(page: Page, response: (route: Route) => Promise<void> = route => route.fulfill({ status: 204 })) {
  let posts = 0
  const nativeDialogs: string[] = [], pageErrors: string[] = []
  const consoleErrors: string[] = []
  health.set(page, { pageErrors, consoleErrors })
  page.on('pageerror', error => pageErrors.push(error.message))
  page.on('console', message => {
    // 失败用例主动返回 503 / 401 或断网；只排除对应注销请求的浏览器资源错误。
    if (message.type() === 'error' && !(message.location().url.endsWith('/logout') && /Failed to load resource/.test(message.text()))) consoleErrors.push(`${message.location().url}: ${message.text()}`)
  })
  page.on('dialog', dialog => { nativeDialogs.push(dialog.type()); void dialog.dismiss() })
  await page.route('**/*', route => new URL(route.request().url()).hostname === '127.0.0.1' ? route.continue() : route.abort())
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/storage-credentials') return route.fulfill({ json: { items: [] } })
    return route.fulfill({ status: 404, json: {} })
  })
  await page.route('**/logout', route => {
    expect(route.request().method()).toBe('POST')
    expect(route.request().headers()['x-csrf-token']).toBe('local-mvp-csrf-v1')
    posts++
    return response(route)
  })
  await page.route('**/login', route => route.fulfill({ contentType: 'text/html; charset=utf-8', body: '<!doctype html><meta charset="UTF-8"><title>合成登录页</title><h1>合成登录页</h1>' }))
  await page.route('**/favicon.ico', route => route.fulfill({ status: 204 }))
  return { posts: () => posts, nativeDialogs, pageErrors }
}

async function dirtyWizard(page: Page, kind: 'normal' | 'export' = 'normal') {
  await page.goto(kind === 'normal' ? '/imports/normal/new' : '/exports/new')
  await expect(page.getByRole('heading', { name: kind === 'normal' ? '新建普通导入任务' : '新建导出任务', exact: true })).toBeVisible()
  await page.getByRole('radio', { name: /合成 MySQL 导入目标/ }).check()
  await expect(page.getByText('草稿有未保存更改', { exact: true })).toBeVisible()
}

for (const kind of ['normal', 'export'] as const) {
  test(`${kind} 退出复用离开确认，取消保留输入，确认后只注销一次且无原生二次提示`, async ({ page }, info) => {
    const observed = await fixture(page)
    await dirtyWizard(page, kind)
    await page.getByRole('link', { name: '退出登录', exact: true }).click()
    const leave = page.getByRole('dialog', { name: kind === 'normal' ? '离开普通导入配置？' : '离开导出配置？', exact: true })
    await expect(leave).toBeVisible()
    await expect(leave.getByRole('button', { name: '继续配置', exact: true })).toBeFocused()
    expect(observed.posts()).toBe(0)
    await page.screenshot({ path: info.outputPath(`${kind}-before-logout-confirm.png`) })
    await leave.getByRole('button', { name: '继续配置', exact: true }).click()
    await expect(leave).not.toBeVisible()
    await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
    expect(observed.posts()).toBe(0)
    await page.getByRole('link', { name: '任务中心', exact: true }).click()
    await expect(leave).toBeVisible()
    expect(observed.posts()).toBe(0)
    await leave.getByRole('button', { name: '继续配置', exact: true }).click()
    await page.getByRole('link', { name: '退出登录', exact: true }).click()
    await leave.getByRole('button', { name: '放弃配置并离开', exact: true }).click()
    await expect(page.getByRole('heading', { name: '合成登录页', exact: true })).toBeVisible()
    expect(observed.posts()).toBe(1)
    expect(observed.nativeDialogs).toEqual([])
    expect(observed.pageErrors).toEqual([])
  })
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`注销失败保留输入与原生刷新保护 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const observed = await fixture(page, route => route.fulfill({ status: 503 }))
    await page.setViewportSize(viewport)
    await dirtyWizard(page)
    await page.getByRole('link', { name: '退出登录', exact: true }).click()
    await page.getByRole('dialog', { name: '离开普通导入配置？', exact: true }).getByRole('button', { name: '放弃配置并离开', exact: true }).click()
    const failure = page.getByRole('dialog', { name: '退出登录未完成', exact: true })
    await expect(failure.getByRole('alert')).toContainText('暂无法确认退出登录结果')
    await expect(failure.getByRole('button', { name: '留在当前页', exact: true })).toBeFocused()
    await page.screenshot({ path: info.outputPath('logout-error.png') })
    await failure.getByRole('button', { name: '留在当前页', exact: true }).click()
    await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
    expect(observed.posts()).toBe(1)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.evaluate(() => { setTimeout(() => window.location.reload(), 0) })
    await expect.poll(() => observed.nativeDialogs).toEqual(['beforeunload'])
    await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
    expect(observed.pageErrors).toEqual([])
  })
}

test('干净页面直接退出，未触发业务确认或原生提示', async ({ page }) => {
  const observed = await fixture(page)
  await page.goto('/imports/normal/new')
  await page.getByRole('link', { name: '退出登录', exact: true }).click()
  await expect(page).toHaveURL(/\/login$/)
  expect(observed.posts()).toBe(1)
  expect(observed.nativeDialogs).toEqual([])
  expect(observed.pageErrors).toEqual([])
})

test('缺少 CSRF 不发送退出请求且保留刷新保护', async ({ page }) => {
  const observed = await fixture(page)
  await dirtyWizard(page)
  await page.evaluate(() => document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.remove())
  await page.getByRole('link', { name: '退出登录', exact: true }).click()
  await page.getByRole('dialog', { name: '离开普通导入配置？', exact: true }).getByRole('button', { name: '放弃配置并离开', exact: true }).click()
  const failure = page.getByRole('dialog', { name: '退出登录未完成', exact: true })
  await expect(failure).toContainText('未发送退出请求')
  expect(observed.posts()).toBe(0)
  await failure.getByRole('button', { name: '留在当前页', exact: true }).click()
  await page.evaluate(() => { setTimeout(() => window.location.reload(), 0) })
  await expect.poll(() => observed.nativeDialogs).toEqual(['beforeunload'])
})

test('注销请求期间重复触发不重发，会话已失效时正常进入登录页', async ({ page }) => {
  let release!: () => void
  const waiting = new Promise<void>(resolve => { release = resolve })
  const observed = await fixture(page, async route => { await waiting; await route.fulfill({ status: 401 }) })
  await dirtyWizard(page)
  const logout = page.getByRole('link', { name: '退出登录', exact: true })
  await logout.click()
  await page.getByRole('dialog', { name: '离开普通导入配置？', exact: true }).getByRole('button', { name: '放弃配置并离开', exact: true }).click()
  await expect.poll(observed.posts).toBe(1)
  await expect(logout).toHaveAttribute('aria-busy', 'true')
  await logout.dispatchEvent('click')
  expect(observed.posts()).toBe(1)
  release()
  await expect(page).toHaveURL(/\/login$/)
  expect(observed.posts()).toBe(1)
  expect(observed.nativeDialogs).toEqual([])
  expect(observed.pageErrors).toEqual([])
})

test('网络失败可以重试，重试沿用当前编辑会话且不会重复弹出原生确认', async ({ page }) => {
  let attempts = 0
  const observed = await fixture(page, route => ++attempts === 1 ? route.abort('failed') : route.fulfill({ status: 204 }))
  await dirtyWizard(page)
  await page.getByRole('link', { name: '退出登录', exact: true }).click()
  const leave = page.getByRole('dialog', { name: '离开普通导入配置？', exact: true })
  await leave.getByRole('button', { name: '放弃配置并离开', exact: true }).click()
  const failure = page.getByRole('dialog', { name: '退出登录未完成', exact: true })
  await expect(failure.getByRole('alert')).toBeVisible()
  await failure.getByRole('button', { name: '重试退出', exact: true }).click()
  await expect(leave).toBeVisible()
  expect(observed.posts()).toBe(1)
  await leave.getByRole('button', { name: '放弃配置并离开', exact: true }).click()
  await expect(page.getByRole('heading', { name: '合成登录页', exact: true })).toBeVisible()
  expect(observed.posts()).toBe(2)
  expect(observed.nativeDialogs).toEqual([])
})

test('数据源标准页保留外壳与控件，干净会话直接退出', async ({ page }, info) => {
  const observed = await fixture(page)
  await page.goto('/data-sources?uiFixture=data-sources')
  await expect(page.getByRole('heading', { name: '数据源管理', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '新建数据源', exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('source-standard-1920.png') })
  await page.getByRole('link', { name: '退出登录', exact: true }).click()
  await expect(page.getByRole('heading', { name: '合成登录页', exact: true })).toBeVisible()
  expect(observed.posts()).toBe(1)
  expect(observed.nativeDialogs).toEqual([])
})

test('退出失败弹窗在原生 200% 缩放下可读、键盘可达且返回触发点', async ({ browserName }, info) => {
  test.setTimeout(180000)
  expect(browserName).toBe('chromium')
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext('', {
    channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, baseURL: 'http://127.0.0.1:15174',
    args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage()
    const observed = await fixture(page, route => route.fulfill({ status: 503 }))
    await dirtyWizard(page)
    const ratio = await page.evaluate(() => devicePixelRatio)
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(options: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const target = (await api.tabs.query({})).find(tab => tab.url.includes('127.0.0.1:15174'))
      if (!target) throw new Error('隔离测试标签不存在')
      await api.tabs.setZoom(target.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(ratio * 2)
    const logout = page.getByRole('link', { name: '退出登录', exact: true })
    await logout.click()
    await page.getByRole('dialog', { name: '离开普通导入配置？', exact: true }).getByRole('button', { name: '放弃配置并离开', exact: true }).click()
    const failure = page.getByRole('dialog', { name: '退出登录未完成', exact: true })
    await expect(failure.getByRole('button', { name: '留在当前页', exact: true })).toBeInViewport()
    await expect(failure.getByRole('button', { name: '重试退出', exact: true })).toBeInViewport()
    for (let index = 0; index < 10; index++) {
      await page.keyboard.press('Tab')
      expect(await failure.evaluate(element => element.contains(document.activeElement))).toBe(true)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const session = await context.newCDPSession(page)
    const capture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('logout-error-native-200.png'), Buffer.from(capture.data, 'base64'))
    await page.keyboard.press('Escape')
    await expect(failure).not.toBeVisible()
    await expect(logout).toBeFocused()
    await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
    expect(observed.posts()).toBe(1)
    expect(observed.nativeDialogs).toEqual([])
    expect(health.get(page)?.pageErrors).toEqual([])
    expect(health.get(page)?.consoleErrors).toEqual([])
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await info.attach('browser-environment', { body: JSON.stringify({ browser: context.browser()?.version(), nativeZoom: 2, url: page.url() }), contentType: 'application/json' })
  } finally { await context.close() }
})
