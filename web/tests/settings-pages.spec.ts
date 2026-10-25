import { expect, test } from '@playwright/test'
import { logRetentionFixture } from './fixtures/log-retention'
import { schedulingFixture } from './fixtures/scheduling-policy'
import { securityFixture } from './fixtures/security-policy'
import { toolsFixture } from './fixtures/tool-compatibility'

const modules = [
  { path: '/settings/platform', title: '平台设置', section: '保留策略', state: '当前开放普通任务日志保留策略' },
  { path: '/settings/scheduling', title: '调度策略', section: '并发约束', state: '并发、时间与候选顺序分别编辑和保存' },
  { path: '/settings/security', title: '安全策略', section: '安全基线', state: '安全策略当前为只读状态' },
  { path: '/settings/tools', title: '工具与兼容', section: 'OBDUMPER', state: '工具与兼容当前为只读状态' },
] as const

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`独立设置页面只读边界与响应式 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const errors: string[] = [], requests: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.route('**/api/v1/**', route => { requests.push(route.request().url()); return route.abort() })
    await logRetentionFixture(page); await schedulingFixture(page); await securityFixture(page); await toolsFixture(page)
    await page.setViewportSize(viewport)
    for (const module of modules) {
      await page.goto(module.path)
      await expect(page).toHaveTitle(/OB Data Orch/)
      await expect(page.getByRole('heading', { name: module.title, exact: true })).toBeVisible()
      await expect(page.getByRole('heading', { name: module.section, exact: true })).toBeVisible()
      await expect(page.getByRole('alert').filter({ hasText: module.state })).toBeVisible()
      await expect(page.getByRole('textbox')).toHaveCount(0)
      await expect(page.getByRole('combobox')).toHaveCount(0)
      await expect(page.getByRole('switch')).toHaveCount(0)
      await expect(page.getByRole('button', { name: ['/settings/platform', '/settings/scheduling'].includes(module.path) ? /保存|立即清理|上传|升级/ : /保存|编辑|立即清理|上传|升级/ })).toHaveCount(0)
      await expect(page.locator('.settings-tabs, .settings-aside, .ant-tabs')).toHaveCount(0)
      await expect(page.locator('vite-error-overlay')).toHaveCount(0)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      if (viewport.width > 1280) {
        const group = page.getByRole('region', { name: '系统管理分组', exact: true })
        await expect(group.locator('[aria-current="page"]')).toHaveCount(1)
        await expect(group.getByRole('link', { name: module.title, exact: true })).toHaveAttribute('aria-current', 'page')
      }
      await page.screenshot({ path: info.outputPath(`${module.title}-${viewport.width}.png`), fullPage: true })
      await page.reload()
      await expect(page.getByRole('heading', { name: module.title, exact: true })).toBeVisible()
    }
    expect(requests).toEqual([])
    expect(errors).toEqual([])
    await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
  })
}

test('旧系统设置深链接保留查询与锚点并进入平台设置', async ({ page }) => {
  await logRetentionFixture(page); await schedulingFixture(page)
  await page.goto('/settings?source=legacy#platform-retention-title')
  await expect(page).toHaveURL('/settings/platform?source=legacy#platform-retention-title')
  await expect(page.getByRole('heading', { name: '平台设置', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: '系统设置', exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '保留策略', exact: true })).toBeInViewport()
})

test('滚动后切换独立设置页面回到标题，锚点限定于当前正文', async ({ page }) => {
  await logRetentionFixture(page); await schedulingFixture(page)
  await page.setViewportSize({ width: 1440, height: 720 })
  await page.goto('/settings/platform')
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
  expect(await page.evaluate(() => scrollY)).toBeGreaterThan(0)
  await page.getByRole('region', { name: '系统管理分组', exact: true }).getByRole('link', { name: '调度策略', exact: true }).click()
  await expect(page.getByRole('heading', { name: '调度策略', exact: true })).toBeInViewport()
  await page.goto('/settings/platform#%E0%A4%A')
  await expect(page.getByRole('heading', { name: '平台设置', exact: true })).toBeInViewport()
  await page.goto('/settings/platform#product-navigation')
  await expect(page.getByRole('heading', { name: '平台设置', exact: true })).toBeInViewport()
})
