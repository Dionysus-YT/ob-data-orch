import { expect, test, type Page } from '@playwright/test'
import { schedulingFixture } from './fixtures/scheduling-policy'
import { candidateFixture, type CandidateMode } from './fixtures/scheduling-candidates'
test.use({ actionTimeout: 15000 })
const region = (page: Page) => page.getByRole('region', { name: '候选节点顺序', exact: true })
async function details(page: Page) {
  const toggle = region(page).getByRole('button', { name: /配置详情$/ })
  if (await toggle.getAttribute('aria-expanded') !== 'true') await toggle.click()
}
const review = (page: Page) => page.getByRole('dialog', { name: '核对候选顺序', exact: true })
async function prepare(page: Page, mode: CandidateMode = 'ready') { await schedulingFixture(page); return candidateFixture(page, mode) }
async function edit(page: Page) {
  await region(page).getByRole('button', { name: '编辑候选顺序' }).click()
  await region(page).getByRole('radio', { name: '在线节点优先', exact: true }).check()
  await region(page).getByRole('textbox', { name: '候选顺序变更原因' }).fill('合成候选顺序变化'); await region(page).getByRole('button', { name: '核对候选顺序' }).click()
}
for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) test(`候选策略独立读取确认保存 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message)); const model = await prepare(page); await page.setViewportSize(viewport); await page.goto('/settings/scheduling')
  await expect(region(page).getByRole('radio', { name: '保持现有顺序', exact: true })).toBeChecked(); await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath(`candidate-read-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
  await edit(page); await expect(review(page).getByText('保持现有顺序 → 在线节点优先', { exact: true })).toBeVisible(); const save = review(page).getByRole('button', { name: '确认保存' }); await expect(save).toBeDisabled(); expect(model.writes).toHaveLength(0)
  await review(page).getByRole('checkbox').check(); await expect(save).toBeEnabled(); await page.screenshot({ path: info.outputPath(`candidate-confirm-${viewport.width}.png`), animations: 'disabled' }); await save.click()
  await expect(region(page).getByRole('radio', { name: '在线节点优先', exact: true })).toBeChecked(); await expect(region(page).getByRole('button', { name: '编辑候选顺序' })).toBeFocused()
  expect(model.writes[0]?.body).toEqual({ order: 'ONLINE_FIRST', reason: '合成候选顺序变化', confirmed: true }); expect(model.writes[0]?.headers['if-match']).toBe('"rev-1"'); expect(model.writes[0]?.headers['x-csrf-token']).toBeTruthy(); expect(model.writes[0]?.headers['idempotency-key']).toBeTruthy()
  await expect(page.getByRole('spinbutton', { name: '全平台最大并发' })).toHaveValue('8'); await page.getByRole('region', { name: '超时与检查间隔', exact: true }).getByRole('button', { name: /配置详情$/ }).click(); await expect(page.getByText('120 秒 · 控制面启动配置', { exact: true })).toBeVisible(); await page.reload(); await expect(region(page).getByRole('radio', { name: '在线节点优先', exact: true })).toBeChecked()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); expect(errors).toEqual([]); await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath(`candidate-saved-${viewport.width}.png`), fullPage: true, animations: 'disabled' })
  await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
})
for (const mode of ['denied', 'error', 'invalid'] as const) test(`候选策略 ${mode} 失败关闭且不影响其他域`, async ({ page }, info) => {
  await prepare(page, mode); await page.goto('/settings/scheduling'); await expect(region(page).getByRole('alert').filter({ hasText: mode === 'denied' ? '系统管理员权限' : '无法读取候选顺序' })).toBeVisible(); await expect(region(page).getByRole('button', { name: '编辑候选顺序' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '编辑时间策略' })).toBeVisible(); await region(page).scrollIntoViewIfNeeded(); await page.screenshot({ path: info.outputPath(`candidate-${mode}.png`), fullPage: true, animations: 'disabled' })
})
for (const mode of ['write-error', 'conflict', 'replay', 'read-after-write-error'] as const) test(`候选策略 ${mode} 不冒认当前值`, async ({ page }, info) => {
  const model = await prepare(page, mode); await page.goto('/settings/scheduling'); await edit(page); await review(page).getByRole('checkbox').check(); await review(page).getByRole('button', { name: '确认保存' }).click()
  if (mode === 'write-error') {
    await details(page); await expect(region(page).getByRole('cell').filter({ hasText: '最近读取的候选顺序' })).toContainText('当前值待核对'); const key = model.writes[0]?.headers['idempotency-key']; model.setMode('ready'); await region(page).getByRole('button', { name: '核对候选顺序' }).click(); await review(page).getByRole('checkbox').check(); await review(page).getByRole('button', { name: '确认保存' }).click(); await expect(region(page).getByRole('button', { name: '编辑候选顺序' })).toBeVisible(); expect(model.writes[1]?.headers['idempotency-key']).toBe(key)
  } else if (mode === 'conflict') { await expect(region(page).getByRole('alert').filter({ hasText: '未覆盖当前配置' })).toBeVisible(); await expect(region(page).getByRole('button', { name: '编辑候选顺序' })).toHaveCount(0) }
  else if (mode === 'replay') { await expect(region(page).getByRole('radio', { name: '保持现有顺序', exact: true })).toBeChecked(); await expect(page.getByText('已确认原候选顺序回执，当前策略以重新读取结果为准。')).toBeVisible() }
  else { await expect(region(page).getByRole('alert').filter({ hasText: '回执已确认' })).toBeVisible(); await expect(region(page).getByRole('cell')).toHaveCount(0); await expect(region(page).getByRole('radio')).toHaveCount(0) }
  await region(page).scrollIntoViewIfNeeded(); await page.screenshot({ path: info.outputPath(`candidate-${mode}.png`), fullPage: true, animations: 'disabled' })
})
test('候选策略字段错误修正与键盘未保存保护', async ({ page }, info) => {
  await prepare(page); await page.setViewportSize({ width: 1440, height: 1024 }); await page.goto('/settings/scheduling'); await region(page).getByRole('button', { name: '编辑候选顺序' }).focus(); await page.keyboard.press('Enter')
  const current = region(page).getByRole('radio', { name: '保持现有顺序', exact: true }); await expect(current).toBeFocused(); await current.press('ArrowRight'); await expect(region(page).getByRole('radio', { name: '在线节点优先', exact: true })).toBeChecked(); await region(page).getByRole('button', { name: '核对候选顺序' }).click(); await expect(region(page).getByText('请选择内置策略并填写 1–200 字原因，不要填写密码或密钥。')).toBeVisible()
  await region(page).getByRole('textbox', { name: '候选顺序变更原因' }).fill('合成未保存原因'); const security = page.getByRole('tab', { name: '安全策略', exact: true }); await security.click()
  const leave = page.getByRole('dialog', { name: '放弃未保存的候选顺序？' }); await expect(leave).toBeVisible(); await page.screenshot({ path: info.outputPath('candidate-discard.png'), animations: 'disabled' }); await leave.getByRole('button', { name: '继续编辑' }).click(); await expect(region(page).getByRole('textbox', { name: '候选顺序变更原因' })).toHaveValue('合成未保存原因')
  await security.click(); await leave.getByRole('button', { name: '放弃并离开' }).click(); await expect(page).toHaveURL('/settings/security')
})
