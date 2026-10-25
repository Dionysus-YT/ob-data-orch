import { chromium, expect, test, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { schedulingFixture } from './fixtures/scheduling-policy'
import { timingFixture, timingPolicy } from './fixtures/scheduling-timing'

const region = (page: Page) => page.getByRole('region', { name: '超时与检查间隔', exact: true })
const review = (page: Page) => page.getByRole('dialog', { name: '核对调度时间策略', exact: true })
async function prepare(page: Page, mode: Parameters<typeof timingFixture>[1] = 'ready') { await schedulingFixture(page); return timingFixture(page, mode) }
async function edit(page: Page) {
  await region(page).getByRole('button', { name: '编辑时间策略' }).click()
  await expect(region(page).getByRole('spinbutton', { name: '节点心跳超时' })).toBeFocused()
  await region(page).getByRole('spinbutton', { name: '节点心跳超时' }).fill('60')
  await region(page).getByRole('spinbutton', { name: '任务启动超时' }).fill('30')
  await region(page).getByRole('spinbutton', { name: '领取检查间隔' }).fill('5')
  await region(page).getByRole('textbox', { name: '时间策略变更原因' }).fill('合成调度时间策略调整')
  await region(page).getByRole('button', { name: '核对时间变更' }).click()
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) test(`时间策略实际读取、确认保存与复读 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  const model = await prepare(page); await page.setViewportSize(viewport); await page.goto('/settings/scheduling')
  await expect(region(page).getByRole('cell').filter({ hasText: '节点心跳超时' })).toContainText('120 秒')
  await expect(region(page).getByText('未配置额外启动期限', { exact: true })).toBeVisible(); await expect(region(page).getByText('未配置额外检查间隔', { exact: true })).toBeVisible()
  await expect(region(page).getByRole('spinbutton')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath(`timing-read-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
  await edit(page); await expect(review(page).getByText('120 秒 → 60 秒', { exact: true })).toBeVisible(); await expect(review(page).getByText('未配置 → 30 秒', { exact: true })).toBeVisible()
  const save = review(page).getByRole('button', { name: '确认保存', exact: true }); await expect(save).toBeDisabled(); expect(model.writes).toHaveLength(0)
  await review(page).getByRole('checkbox').check(); await expect(save).toBeEnabled(); await page.screenshot({ path: info.outputPath(`timing-confirm-${viewport.width}.png`), animations: 'disabled' })
  await save.click(); await expect(region(page).getByRole('cell').filter({ hasText: '节点心跳超时' })).toContainText('60 秒')
  await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toBeFocused(); await expect(page.getByText('调度时间策略已保存，按各项生效范围执行。', { exact: true })).toBeVisible()
  expect(model.writes[0]?.body).toEqual({ heartbeatTimeoutSeconds: 60, startupTimeoutSeconds: 30, pollingIntervalSeconds: 5, reason: '合成调度时间策略调整', confirmed: true })
  expect(model.writes[0]?.headers['if-match']).toBe('"rev-1"'); expect(model.writes[0]?.headers['idempotency-key']).toBeTruthy(); expect(model.writes[0]?.headers['x-csrf-token']).toBeTruthy()
  await expect(page.getByText('8 个执行', { exact: true })).toBeVisible(); await page.reload()
  await expect(region(page).getByRole('cell').filter({ hasText: '任务启动期限' })).toContainText('30 秒'); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([]); await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath(`timing-saved-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
  await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
})

test('时间字段保留真实空覆盖，字段错误修正后清除', async ({ page }, info) => {
  await prepare(page); await page.goto('/settings/scheduling'); await region(page).getByRole('button', { name: '编辑时间策略' }).click()
  await expect(region(page).getByRole('spinbutton', { name: '任务启动超时' })).toHaveValue(''); await expect(region(page).getByRole('spinbutton', { name: '领取检查间隔' })).toHaveValue('')
  await region(page).getByRole('spinbutton', { name: '任务启动超时' }).fill('30'); await region(page).getByRole('button', { name: '核对时间变更' }).click()
  const error = region(page).getByText('请输入 1–200 字的变更原因，不要填写密码或密钥。', { exact: true }); await expect(error).toBeVisible()
  await page.screenshot({ path: info.outputPath('timing-field-error.png'), fullPage: true, animations: 'disabled' })
  await region(page).getByRole('textbox', { name: '时间策略变更原因' }).fill('合成首次期限'); await expect(error).toHaveCount(0)
  await region(page).getByRole('button', { name: '核对时间变更' }).click(); await expect(review(page).getByRole('button', { name: '确认保存' })).toBeDisabled()
})

test('已配置的期限不能清空，提高单一心跳阈值不要求收紧勾选', async ({ page }) => {
  await prepare(page, 'configured'); await page.goto('/settings/scheduling'); await region(page).getByRole('button', { name: '编辑时间策略' }).click()
  await region(page).getByRole('spinbutton', { name: '任务启动超时' }).fill(''); await region(page).getByRole('textbox', { name: '时间策略变更原因' }).fill('合成保留已配置期限')
  await region(page).getByRole('button', { name: '核对时间变更' }).click(); await expect(region(page).getByText('已配置的策略不能通过清空重置。')).toBeVisible(); await expect(review(page)).toHaveCount(0)
  await region(page).getByRole('spinbutton', { name: '任务启动超时' }).fill('60'); await region(page).getByRole('spinbutton', { name: '节点心跳超时' }).fill('240')
  await region(page).getByRole('button', { name: '核对时间变更' }).click(); await expect(review(page).getByRole('checkbox')).toHaveCount(0); await expect(review(page).getByRole('button', { name: '确认保存' })).toBeEnabled()
})

for (const mode of ['denied', 'error', 'invalid'] as const) test(`时间策略 ${mode} 失败关闭且不改变并发域`, async ({ page }, info) => {
  await prepare(page, mode); await page.goto('/settings/scheduling')
  await expect(region(page).getByRole('alert').filter({ hasText: mode === 'denied' ? '系统管理员权限' : '无法读取调度时间策略' })).toBeVisible()
  await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toHaveCount(0); await expect(region(page).getByRole('cell')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '编辑平台并发策略' })).toBeVisible(); await page.screenshot({ path: info.outputPath(`timing-${mode}.png`), fullPage: true, animations: 'disabled' })
})

test('时间策略加载中不以控制面常量补成已读取事实', async ({ page }, info) => {
  await schedulingFixture(page); let release!: () => void; const pending = new Promise<void>(yes => { release = yes })
  await page.route('**/api/v1/settings/scheduling/timing', async route => { await pending; await route.fulfill({ json: { item: timingPolicy() } }) })
  await page.goto('/settings/scheduling'); await expect(region(page).getByRole('status')).toContainText('正在读取调度时间策略'); await expect(region(page).getByRole('cell')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('timing-loading.png'), fullPage: true }); release(); await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toBeVisible()
})

for (const mode of ['write-error', 'conflict', 'replay', 'read-after-write-error'] as const) test(`时间策略 ${mode} 保留可信状态`, async ({ page }, info) => {
  const model = await prepare(page, mode); await page.goto('/settings/scheduling'); await edit(page); await review(page).getByRole('checkbox').check(); await review(page).getByRole('button', { name: '确认保存' }).click()
  if (mode === 'write-error') {
    await expect(region(page).getByRole('cell').filter({ hasText: '最近读取的心跳超时' })).toContainText('当前值待核对')
    const key = model.writes[0]?.headers['idempotency-key']; await region(page).screenshot({ path: info.outputPath('timing-unconfirmed.png'), animations: 'disabled' })
    model.setMode('ready'); await region(page).getByRole('button', { name: '核对时间变更' }).click(); await review(page).getByRole('checkbox').check(); await review(page).getByRole('button', { name: '确认保存' }).click()
    await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toBeVisible(); expect(model.writes[1]?.headers['idempotency-key']).toBe(key)
  } else if (mode === 'conflict') {
    await expect(region(page).getByRole('alert').filter({ hasText: '未覆盖当前配置' })).toBeVisible(); await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toHaveCount(0)
  } else if (mode === 'replay') {
    await expect(region(page).getByRole('cell').filter({ hasText: '节点心跳超时' })).toContainText('240 秒'); await expect(page.getByText('已确认原时间设置回执，实际策略以重新读取结果为准。')).toBeVisible()
  } else {
    await expect(region(page).getByRole('alert').filter({ hasText: '回执已确认' })).toBeVisible(); await expect(region(page).getByRole('cell')).toHaveCount(0)
  }
  await page.screenshot({ path: info.outputPath(`timing-${mode}.png`), fullPage: true, animations: 'disabled' })
})

test('时间策略键盘进入与未保存离开保护', async ({ page }, info) => {
  await prepare(page); await page.setViewportSize({ width: 1440, height: 1024 }); await page.goto('/settings/scheduling')
  await region(page).getByRole('button', { name: '编辑时间策略' }).focus(); await page.keyboard.press('Enter'); await expect(region(page).getByRole('spinbutton', { name: '节点心跳超时' })).toBeFocused()
  await page.keyboard.press('ControlOrMeta+A'); await page.keyboard.type('180'); await page.keyboard.press('Tab'); await expect(region(page).getByRole('spinbutton', { name: '任务启动超时' })).toBeFocused()
  const security = page.getByRole('region', { name: '系统管理分组', exact: true }).getByRole('link', { name: '安全策略', exact: true })
  await security.click(); const dialog = page.getByRole('dialog', { name: '放弃未保存的时间设置？' }); await expect(dialog).toBeVisible(); await page.screenshot({ path: info.outputPath('timing-discard.png'), animations: 'disabled' })
  await dialog.getByRole('button', { name: '继续编辑' }).click(); await expect(page).toHaveURL('/settings/scheduling'); await expect(region(page).getByRole('spinbutton', { name: '节点心跳超时' })).toHaveValue('180')
  await security.click(); await dialog.getByRole('button', { name: '放弃并离开' }).click(); await expect(page).toHaveURL('/settings/security')
})

test('时间与候选策略原生 200% 下可滚动核对并键盘保存', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium', '原生缩放使用 Chromium 测试扩展'); test.setTimeout(120000)
  const artifactRoot = resolve(info.outputDir); await mkdir(artifactRoot, { recursive: true }); const profile = await mkdtemp(resolve(artifactRoot, 'timing-zoom-')), extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage(); await prepare(page); await page.goto('http://127.0.0.1:15174/settings/scheduling')
    await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toBeVisible(); const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => { const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome; const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在'); await api.tabs.setZoom(tab.id, 2) })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2); await edit(page)
    await review(page).evaluate(async element => { await Promise.allSettled(element.getAnimations({ subtree: true }).filter(animation => Number.isFinite(animation.effect?.getComputedTiming().endTime)).map(animation => animation.finished)) })
    const cdp = await context.newCDPSession(page)
    let capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('timing-native-200-top.png'), Buffer.from(capture.data, 'base64'))
    await review(page).getByRole('checkbox').check(); await expect(review(page).getByRole('checkbox')).toBeChecked()
    const save = review(page).getByRole('button', { name: '确认保存' }); await expect(save).toBeEnabled(); await save.focus(); await expect(save).toBeFocused(); await expect(save).toBeInViewport()
    await review(page).evaluate(async element => { await Promise.allSettled(element.getAnimations({ subtree: true }).filter(animation => Number.isFinite(animation.effect?.getComputedTiming().endTime)).map(animation => animation.finished)) })
    capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('timing-native-200-save.png'), Buffer.from(capture.data, 'base64')); await cdp.detach()
    await page.keyboard.press('Enter'); await expect(region(page).getByRole('cell').filter({ hasText: '节点心跳超时' })).toContainText('60 秒'); await expect(region(page).getByRole('button', { name: '编辑时间策略' })).toBeFocused()
    const candidates = page.getByRole('region', { name: '候选节点顺序', exact: true })
    await candidates.getByRole('button', { name: '编辑候选顺序' }).click(); await candidates.locator('.ant-select').click(); await page.getByTitle('在线节点优先', { exact: true }).click()
    await candidates.getByRole('textbox', { name: '候选顺序变更原因' }).fill('合成原生缩放候选验证'); await candidates.getByRole('button', { name: '核对候选顺序' }).click()
    const candidateReview = page.getByRole('dialog', { name: '核对候选顺序', exact: true }), candidateSave = candidateReview.getByRole('button', { name: '确认保存' })
    await candidateReview.getByRole('checkbox').check(); await expect(candidateReview.getByRole('checkbox')).toBeChecked(); await expect(candidateSave).toBeEnabled(); await candidateSave.focus(); await expect(candidateSave).toBeInViewport()
    await candidateReview.evaluate(async element => { await Promise.allSettled(element.getAnimations({ subtree: true }).filter(animation => Number.isFinite(animation.effect?.getComputedTiming().endTime)).map(animation => animation.finished)) })
    const candidateCDP = await context.newCDPSession(page), candidateCapture = await candidateCDP.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('candidate-native-200-save.png'), Buffer.from(candidateCapture.data, 'base64')); await candidateCDP.detach()
    await page.keyboard.press('Enter'); await expect(candidates.getByRole('cell').filter({ hasText: '当前候选顺序' })).toContainText('在线节点优先'); await expect(candidates.getByRole('button', { name: '编辑候选顺序' })).toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally { const browser = context.browser(); if (browser) await browser.close(); else await context.close(); await removeProfile(artifactRoot, profile) }
})
async function removeProfile(root: string, profile: string) { if (!resolve(profile).startsWith(`${resolve(root)}${sep}`)) throw new Error('临时目录超出本轮产物范围'); await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 }) }
