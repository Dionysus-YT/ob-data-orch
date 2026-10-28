import { chromium, expect, test, type Page } from '@playwright/test'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { schedulingFixture, schedulingPolicy } from './fixtures/scheduling-policy'
import { timingFixture } from './fixtures/scheduling-timing'
import { mockFacts } from './fixtures/facts'

const number = (page: Page) => page.getByRole('spinbutton', { name: '全平台最大并发' })
async function details(page: Page) {
  const toggle = page.getByRole('region', { name: '全平台执行并发', exact: true }).getByRole('button', { name: /配置详情$/ })
  if (await toggle.getAttribute('aria-expanded') !== 'true') await toggle.click()
}
const review = (page: Page) => page.getByRole('dialog', { name: '核对平台并发上限' })
async function edit(page: Page, maximum = '1') {
  await page.getByRole('button', { name: '编辑平台并发策略' }).click(); await expect(number(page)).toBeFocused()
  await number(page).fill(maximum); await page.getByRole('textbox', { name: '变更原因' }).fill('合成平台并发上限调整')
  await page.getByRole('button', { name: '核对变更', exact: true }).click()
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) test(`调度策略读取、降低确认与保存 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  const model = await schedulingFixture(page); await page.setViewportSize(viewport); await page.goto('/settings/scheduling')
  await expect(number(page)).toHaveValue('8'); await details(page); await expect(page.getByText('3 / 2', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath(`scheduling-read-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
  await edit(page); await expect(review(page).getByText('8 个执行 → 1 个执行', { exact: true })).toBeVisible()
  await expect(review(page).getByRole('button', { name: '确认保存' })).toBeDisabled(); expect(model.writes).toHaveLength(0)
  await review(page).getByRole('checkbox').check(); await expect(review(page).getByRole('button', { name: '确认保存' })).toBeEnabled()
  await page.screenshot({ path: info.outputPath(`scheduling-confirm-${viewport.width}.png`), fullPage: false, animations: 'disabled' })
  await review(page).getByRole('button', { name: '确认保存' }).click()
  await expect(number(page)).toHaveValue('1'); await details(page); await expect(page.getByText('3 / 2', { exact: true })).toBeVisible()
  await expect(page.getByText('调度策略已保存，从后续新执行领取生效。', { exact: true })).toBeVisible()
  expect(model.writes[0]?.headers['if-match']).toBe('"rev-1"'); expect(model.writes[0]?.headers['idempotency-key']).toBeTruthy(); expect(model.writes[0]?.headers['x-csrf-token']).toBeTruthy()
  expect(model.writes[0]?.body).toEqual({ maxConcurrentExecutions: 1, reason: '合成平台并发上限调整', confirmed: true })
  await page.reload(); await expect(number(page)).toHaveValue('1'); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([]); await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath(`scheduling-saved-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
  await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
})

test('升级未配置不伪造默认值，首次设置也要求收紧确认', async ({ page }, info) => {
  await schedulingFixture(page, 'unconfigured'); await page.goto('/settings/scheduling'); await expect(number(page)).toHaveValue(''); await expect(number(page)).toHaveAttribute('readonly', '')
  await page.getByRole('button', { name: '编辑平台并发策略' }).click(); await expect(number(page)).toHaveValue(''); await number(page).fill('4')
  await page.getByRole('button', { name: '核对变更' }).click(); await expect(page.getByText('请输入 1–200 字的变更原因，不要填写密码或密钥。')).toBeVisible()
  await page.getByRole('textbox', { name: '变更原因' }).fill('合成首次上限'); await expect(page.getByText('请输入 1–200 字的变更原因，不要填写密码或密钥。')).toHaveCount(0)
  await page.getByRole('button', { name: '核对变更' }).click(); await expect(review(page).getByText('尚未配置 → 4 个执行', { exact: true })).toBeVisible()
  await expect(review(page).getByRole('button', { name: '确认保存' })).toBeDisabled(); await page.screenshot({ path: info.outputPath('scheduling-first-confirm.png'), animations: 'disabled' })
})

for (const mode of ['denied', 'error', 'invalid'] as const) test(`调度策略 ${mode} 失败关闭`, async ({ page }, info) => {
  await schedulingFixture(page, mode); await page.goto('/settings/scheduling')
  await expect(page.getByRole('alert').filter({ hasText: mode === 'denied' ? '系统管理员权限' : '无法读取当前调度策略' })).toBeVisible()
  await expect(page.getByRole('button', { name: '编辑平台并发策略' })).toHaveCount(0); await expect(number(page)).toHaveCount(0)
  await page.screenshot({ path: info.outputPath(`scheduling-${mode}.png`), fullPage: true, animations: 'disabled' })
})

test('系统管理权限不泄露全局数量', async ({ page }, info) => {
  await schedulingFixture(page, 'restricted'); await page.goto('/settings/scheduling')
  await details(page); await expect(page.getByText('无全局任务读取权限，数量不返回', { exact: true })).toBeVisible(); await expect(page.getByText('3 / 2', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '编辑平台并发策略' })).toBeVisible(); await page.screenshot({ path: info.outputPath('scheduling-restricted.png'), fullPage: true })
})

for (const mode of ['write-error', 'conflict', 'replay'] as const) test(`调度策略 ${mode} 保留可信状态`, async ({ page }, info) => {
  const model = await schedulingFixture(page, mode); await page.goto('/settings/scheduling'); await edit(page, '16'); await review(page).getByRole('button', { name: '确认保存' }).click()
  if (mode === 'replay') { await expect(number(page)).toHaveValue('12'); await expect(page.getByText('已确认原请求的保存回执，当前策略以重新读取结果为准。')).toBeVisible() }
  else if (mode === 'conflict') { await expect(page.getByRole('alert').filter({ hasText: '未覆盖当前配置' })).toBeVisible(); await expect(page.getByRole('button', { name: '编辑平台并发策略' })).toHaveCount(0) }
  else {
    await details(page); await expect(page.getByRole('cell').filter({ hasText: '最近读取上限' })).toContainText('当前值待核对')
    const key = model.writes[0]?.headers['idempotency-key']; await page.screenshot({ path: info.outputPath('scheduling-unconfirmed.png'), fullPage: false, animations: 'disabled' })
    model.setMode('ready'); await page.getByRole('button', { name: '核对变更' }).click(); await review(page).getByRole('button', { name: '确认保存' }).click()
    await expect(number(page)).toHaveValue('16'); expect(model.writes[1]?.headers['idempotency-key']).toBe(key)
  }
  await page.screenshot({ path: info.outputPath(`scheduling-${mode}.png`), fullPage: false, animations: 'disabled' })
})

test('键盘编辑及未保存输入的离开保护', async ({ page }, info) => {
  await schedulingFixture(page); await page.goto('/settings/scheduling'); await page.getByRole('button', { name: '编辑平台并发策略' }).focus(); await page.keyboard.press('Enter')
  await expect(number(page)).toBeFocused(); await page.keyboard.press('ControlOrMeta+A'); await page.keyboard.type('16'); await page.keyboard.press('Tab')
  await expect(page.getByRole('textbox', { name: '变更原因' })).toBeFocused(); await page.keyboard.type('synthetic scheduling change')
  const leaveForSecurity = async () => {
    const target = page.getByRole('tab', { name: '安全策略', exact: true })
    await target.focus(); await page.keyboard.press('Enter')
  }
  await leaveForSecurity()
  const dialog = page.getByRole('dialog', { name: '放弃未保存的调度设置？' }); await expect(dialog).toBeVisible(); await page.screenshot({ path: info.outputPath('scheduling-discard.png'), animations: 'disabled' })
  await dialog.getByRole('button', { name: '继续编辑' }).click(); await expect(page).toHaveURL('/settings/scheduling'); await expect(page.getByRole('textbox', { name: '变更原因' })).toHaveValue('synthetic scheduling change')
  await leaveForSecurity(); await dialog.getByRole('button', { name: '放弃并离开' }).click(); await expect(page).toHaveURL('/settings/security')
})

test('策略读取中不展示猜测值', async ({ page }, info) => {
  await timingFixture(page)
  let release!: () => void; const pending = new Promise<void>(yes => { release = yes })
  await page.route('**/api/v1/settings/scheduling', async route => { await pending; await route.fulfill({ json: { item: schedulingPolicy() } }) })
  await page.goto('/settings/scheduling'); await expect(page.getByRole('region', { name: '全平台执行并发', exact: true }).getByRole('status')).toContainText('正在读取当前策略'); await expect(number(page)).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('scheduling-loading.png'), fullPage: true }); release(); await expect(number(page)).toHaveValue('8')
})

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) test(`已授权任务详情显示固定等待原因且不暴露数量 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
  const duplicateKeys: string[] = []; page.on('console', message => { if (message.text().includes('Duplicate keys')) duplicateKeys.push(message.text()) })
  await page.setViewportSize(viewport)
  await mockFacts(page); let reason = 'PLATFORM_CONCURRENCY_LIMIT'
  await page.route('**/api/v1/tasks/synthetic-task/execution', route => route.fulfill({ json: { item: { state: 'WAITING_SCHEDULE', schedulingWaitReason: reason, reconciliationRequired: false, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', updatedAt: '2026-10-24T00:00:00Z' } } }))
  await page.goto('/tasks/synthetic-task'); await expect(page.getByRole('cell').filter({ hasText: '平台并发上限暂时阻止新增执行' })).toBeVisible()
  await page.screenshot({ path: info.outputPath(`task-scheduling-wait-${viewport.width}.png`), fullPage: true })
  reason = 'SCHEDULING_POLICY_UNAVAILABLE'; await page.getByRole('button', { name: '刷新', exact: true }).click(); await expect(page.getByRole('cell').filter({ hasText: '调度策略暂不可用' })).toBeVisible()
  expect(duplicateKeys).toEqual([]); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('调度策略原生 200% 下确认保存可达', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium', '原生缩放使用 Chromium 测试扩展'); test.setTimeout(120000)
  const artifactRoot = fileURLToPath(new URL('../../artifacts/', import.meta.url)), profile = await mkdtemp(`${artifactRoot}scheduling-zoom-`), extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage(); await schedulingFixture(page); await page.goto('http://127.0.0.1:15174/settings/scheduling')
    await expect(number(page)).toHaveValue('8'); const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => { const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome; const tab = (await api.tabs.query({})).find(item => item.url.includes('15174')); if (!tab) throw new Error('隔离测试页面不存在'); await api.tabs.setZoom(tab.id, 2) })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    await edit(page); await review(page).getByRole('checkbox').check(); const save = review(page).getByRole('button', { name: '确认保存' }); await save.focus(); await expect(save).toBeInViewport()
    await review(page).evaluate(async element => { await Promise.allSettled(element.getAnimations({ subtree: true }).filter(animation => Number.isFinite(animation.effect?.getComputedTiming().endTime)).map(animation => animation.finished)) })
    const cdp = await context.newCDPSession(page), capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }); await writeFile(info.outputPath('scheduling-native-200.png'), Buffer.from(capture.data, 'base64')); await cdp.detach()
    await page.keyboard.press('Enter'); await expect(number(page)).toHaveValue('1'); await expect(page.getByRole('button', { name: '编辑平台并发策略' })).toBeFocused(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally { const browser = context.browser(); if (browser) await browser.close(); else await context.close(); await removeProfile(artifactRoot, profile) }
})
async function removeProfile(artifactRoot: string, profile: string) { if (!resolve(profile).startsWith(`${resolve(artifactRoot)}${sep}`)) throw new Error('临时目录超出测试产物范围'); await rm(profile, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 }) }
