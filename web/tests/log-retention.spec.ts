import { chromium, expect, test, type Page } from '@playwright/test'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { resolve, sep } from 'node:path'
import { logRetentionFixture, retentionPolicy } from './fixtures/log-retention'

async function details(page: Page) {
  const toggle = page.getByRole('region', { name: '普通任务日志', exact: true }).getByRole('button', { name: /配置详情$/ })
  if (await toggle.getAttribute('aria-expanded') !== 'true') await toggle.click()
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`普通日志期限实际读取、降低确认与保存 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
    const model = await logRetentionFixture(page); await page.setViewportSize(viewport); await page.goto('/settings/platform')
    const region = page.getByRole('region', { name: '普通任务日志', exact: true })
    await expect(region.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('30'); await details(page); await expect(region.getByText('内置默认', { exact: true })).toBeVisible()
    await expect(region.getByText('尚无平台修改记录', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '立即清理' })).toHaveCount(0)
    await page.screenshot({ path: info.outputPath(`retention-read-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: '编辑普通日志策略' }).click()
    await page.getByRole('spinbutton', { name: '普通日志保留天数' }).fill('7')
    await page.getByRole('button', { name: '核对变更', exact: true }).click(); await expect(page.getByText('请输入 1–200 字的变更原因，不要填写密码或密钥。')).toBeVisible()
    await page.getByRole('textbox', { name: '变更原因' }).fill('调整合成测试日志期限')
    await expect(page.getByText('请输入 1–200 字的变更原因，不要填写密码或密钥。')).toHaveCount(0)
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.screenshot({ path: info.outputPath(`retention-edit-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: '核对变更', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '核对普通日志保留策略' })
    await expect(dialog.getByText('30 天 → 7 天', { exact: true })).toBeVisible(); await expect(dialog.getByRole('button', { name: '确认保存', exact: true })).toBeDisabled()
    expect(model.writes).toHaveLength(0)
    await dialog.getByRole('checkbox', { name: '我已了解缩短期限的清理影响' }).check()
    await expect(dialog.getByRole('button', { name: '确认保存', exact: true })).toBeEnabled()
    await expect(page.locator('.ant-modal')).toHaveCSS('opacity', '1')
    await page.screenshot({ path: info.outputPath(`retention-confirm-${viewport.width}.png`), fullPage: false, animations: 'disabled' })
    await dialog.getByRole('button', { name: '确认保存', exact: true }).click()
    await expect(region.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('7'); await details(page); await expect(region.getByText('平台设置', { exact: true })).toBeVisible()
    await expect(page.getByText('普通日志策略已保存，从后续清理批次生效。', { exact: true })).toBeVisible()
    expect(model.writes).toHaveLength(1); expect(model.writes[0]?.headers['if-match']).toBe('"rev-1"'); expect(model.writes[0]?.headers['idempotency-key']).toBeTruthy(); expect(model.writes[0]?.headers['x-csrf-token']).toBeTruthy()
    expect(model.writes[0]?.body).toEqual({ retentionDays: 7, reason: '调整合成测试日志期限', confirmed: true })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.reload(); await expect(region.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('7'); expect(errors).toEqual([])
    await page.screenshot({ path: info.outputPath(`retention-saved-${viewport.width}.png`), fullPage: true })
    await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
  })
}

for (const mode of ['denied', 'error', 'invalid'] as const) test(`普通日志策略 ${mode} 失败关闭`, async ({ page }, info) => {
  await logRetentionFixture(page, mode); await page.goto('/settings/platform')
  await expect(page.getByRole('alert').filter({ hasText: mode === 'denied' ? '系统管理员权限' : '无法读取当前普通日志策略' })).toBeVisible()
  await expect(page.getByRole('button', { name: '编辑普通日志策略' })).toHaveCount(0); await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath(`retention-${mode}.png`), fullPage: true })
})

for (const mode of ['write-error', 'conflict', 'replay'] as const) test(`普通日志策略 ${mode} 的结果与重试`, async ({ page }, info) => {
  const model = await logRetentionFixture(page, mode); await page.goto('/settings/platform'); await page.getByRole('button', { name: '编辑普通日志策略' }).click()
  await page.getByRole('spinbutton', { name: '普通日志保留天数' }).fill('90'); await page.getByRole('textbox', { name: '变更原因' }).fill('合成延长期限')
  await page.getByRole('button', { name: '核对变更', exact: true }).click(); await page.getByRole('button', { name: '确认保存', exact: true }).click()
  if (mode === 'replay') { await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('120'); await expect(page.getByText('已确认原请求的保存回执，当前策略以重新读取结果为准。')).toBeVisible() }
  else if (mode === 'conflict') { await expect(page.getByRole('alert').filter({ hasText: '未覆盖当前配置' })).toBeVisible(); await expect(page.getByRole('button', { name: '编辑普通日志策略' })).toHaveCount(0) }
  else {
    await expect(page.getByRole('textbox', { name: '变更原因' })).toHaveValue('合成延长期限'); const firstKey = model.writes[0]?.headers['idempotency-key']
    const failure = page.getByRole('alert').filter({ hasText: '未确认保存成功' })
    await expect(failure).toBeVisible(); await failure.scrollIntoViewIfNeeded()
    await details(page); await expect(page.getByRole('cell').filter({ hasText: '最近读取期限' })).toContainText('当前值待核对')
    await page.screenshot({ path: info.outputPath('retention-write-unconfirmed.png'), fullPage: false, animations: 'disabled' })
    model.setMode('ready'); await page.getByRole('button', { name: '核对变更', exact: true }).click(); await page.getByRole('button', { name: '确认保存', exact: true }).click()
    await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('90'); expect(model.writes[1]?.headers['idempotency-key']).toBe(firstKey)
  }
  await page.screenshot({ path: info.outputPath(`retention-${mode}.png`), fullPage: true })
})

test('普通日志编辑可用键盘完成且取消清空原因', async ({ page }, info) => {
  const model = await logRetentionFixture(page); await page.goto('/settings/platform')
  await page.getByRole('button', { name: '编辑普通日志策略' }).focus(); await page.keyboard.press('Enter')
  await page.getByRole('spinbutton', { name: '普通日志保留天数' }).focus(); await page.keyboard.press('ControlOrMeta+A'); await page.keyboard.type('90')
  await page.keyboard.press('Tab'); await expect(page.getByRole('textbox', { name: '变更原因' })).toBeFocused(); await page.keyboard.type('synthetic retention review')
  await page.keyboard.press('Tab'); await page.keyboard.press('Enter'); await expect(page.getByRole('dialog', { name: '核对普通日志保留策略' })).toBeVisible()
  await expect.poll(() => page.evaluate(() => Boolean(document.activeElement?.closest('[role="dialog"]')))).toBe(true)
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0); await page.getByRole('button', { name: '取消编辑' }).click()
  await page.getByRole('button', { name: '编辑普通日志策略' }).click(); await expect(page.getByRole('textbox', { name: '变更原因' })).toHaveValue(''); expect(model.writes).toHaveLength(0)
  await page.screenshot({ path: info.outputPath('retention-keyboard.png'), fullPage: true })
})

test('读取中不显示猜测值，读取完成才可编辑', async ({ page }, info) => {
  let release!: () => void
  const pending = new Promise<void>(yes => { release = yes })
  await page.route('**/api/v1/settings/log-retention', async route => { await pending; await route.fulfill({ json: { item: retentionPolicy() } }) })
  await page.goto('/settings/platform'); await expect(page.getByRole('status')).toContainText('正在读取当前策略')
  await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveCount(0); await expect(page.getByRole('button', { name: '编辑普通日志策略' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('retention-loading.png'), fullPage: true })
  release(); await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('30')
})

test('保存后复读失败明确区分已确认回执与当前事实未知', async ({ page }, info) => {
  const model = await logRetentionFixture(page, 'refresh-error'); await page.goto('/settings/platform')
  await page.getByRole('button', { name: '编辑普通日志策略' }).click(); await page.getByRole('spinbutton', { name: '普通日志保留天数' }).fill('90')
  await page.getByRole('textbox', { name: '变更原因' }).fill('合成复读异常验证'); await page.getByRole('button', { name: '核对变更', exact: true }).click(); await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: '保存回执已确认' })).toBeVisible(); await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('retention-refresh-error.png'), fullPage: true })
  model.setMode('ready'); await page.getByRole('button', { name: '重新读取', exact: true }).click(); await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('90'); expect(model.writes).toHaveLength(1)
})

test('普通日志策略在原生 200% 缩放下可编辑、确认与保存', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium', '原生缩放使用 Chromium 测试扩展')
  test.setTimeout(120000)
  const artifactRoot = fileURLToPath(new URL('../../artifacts/', import.meta.url)), profilePath = await mkdtemp(`${artifactRoot}log-retention-zoom-`)
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext(profilePath, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker'), page = await context.newPage()
    await logRetentionFixture(page); await page.goto('http://127.0.0.1:15174/settings/platform')
    await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('30')
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const tab = (await api.tabs.query({})).find(item => item.url.includes('15174'))
      if (!tab) throw new Error('隔离测试页面不存在')
      await api.tabs.setZoom(tab.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    await page.getByRole('button', { name: '编辑普通日志策略' }).click()
    await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toBeFocused()
    await page.getByRole('spinbutton', { name: '普通日志保留天数' }).fill('7'); await page.getByRole('textbox', { name: '变更原因' }).fill('合成原生缩放验证')
    await page.getByRole('button', { name: '核对变更', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '核对普通日志保留策略' })
    await dialog.getByRole('checkbox', { name: '我已了解缩短期限的清理影响' }).check()
    const save = dialog.getByRole('button', { name: '确认保存', exact: true }); await save.focus(); await expect(save).toBeInViewport()
    await dialog.evaluate(async element => { await Promise.allSettled(element.getAnimations({ subtree: true }).filter(animation => Number.isFinite(animation.effect?.getComputedTiming().endTime)).map(animation => animation.finished)) })
    const cdp = await context.newCDPSession(page), capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('retention-confirm-native-200.png'), Buffer.from(capture.data, 'base64'))
    await page.keyboard.press('Enter'); await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('7')
    await expect(page.getByRole('button', { name: '编辑普通日志策略' })).toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    // 原生缩放时整页截图会按 CSS 宽度裁切，沿用浏览器可视区域取证。
    const savedCapture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('retention-saved-native-200.png'), Buffer.from(savedCapture.data, 'base64')); await cdp.detach()
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally {
    const browser = context.browser(); if (browser) await browser.close(); else await context.close()
    await removeTemporaryProfile(artifactRoot, profilePath)
  }
})

async function removeTemporaryProfile(artifactRoot: string, profilePath: string) {
  if (!resolve(profilePath).startsWith(`${resolve(artifactRoot)}${sep}`)) throw new Error('临时浏览器目录超出测试产物范围')
  await rm(profilePath, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 })
}
