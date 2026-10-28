import { expect, test, type Page } from '@playwright/test'
import { logRetentionFixture } from './fixtures/log-retention'
import { schedulingFixture } from './fixtures/scheduling-policy'
import { timingFixture } from './fixtures/scheduling-timing'
import { candidateFixture } from './fixtures/scheduling-candidates'
import { securityFixture } from './fixtures/security-policy'
import { toolsFixture } from './fixtures/tool-compatibility'

const modules = [
  { path: '/settings/platform', title: '常规设置', section: '保留策略', state: '普通日志保留天数' },
  { path: '/settings/scheduling', title: '调度策略', section: '并发约束', state: '全平台最大并发' },
  { path: '/settings/security', title: '安全策略', section: '安全基线', state: '安全策略当前为只读状态' },
  { path: '/settings/tools', title: '工具与兼容', section: 'OBDUMPER', state: '工具与兼容当前为只读状态' },
] as const

async function prepare(page: Page) {
  const retention = await logRetentionFixture(page), scheduling = await schedulingFixture(page), timing = await timingFixture(page), candidates = await candidateFixture(page)
  await securityFixture(page); await toolsFixture(page)
  return { retention, scheduling, timing, candidates }
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`平台设置四页签、独立配置边界与响应式 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const errors: string[] = [], requests: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.route('**/api/v1/**', route => { requests.push(route.request().url()); return route.abort() })
    await prepare(page)
    await page.setViewportSize(viewport)
    for (const module of modules) {
      await page.goto(module.path)
      await expect(page).toHaveTitle(/OB Data Orch/)
      await expect(page.getByRole('heading', { name: '平台设置', exact: true, level: 1 })).toBeVisible()
      await expect(page.getByRole('heading', { name: module.title, exact: true })).toBeVisible()
      await expect(page.getByRole('tab')).toHaveCount(4)
      await expect(page.getByRole('tab', { name: module.title, exact: true })).toHaveAttribute('aria-selected', 'true')
      await expect(page.getByRole('tabpanel')).toHaveCount(1)
      await expect(page.getByRole('heading', { name: module.section, exact: true })).toBeVisible()
      if (module.path === '/settings/platform') {
        await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('30')
        await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveAttribute('readonly', '')
      } else if (module.path === '/settings/scheduling') {
        await expect(page.getByRole('spinbutton', { name: '全平台最大并发' })).toHaveValue('8')
        await expect(page.getByRole('radio', { name: '保持现有顺序', exact: true })).toBeChecked()
        await expect(page.getByRole('radio', { name: '保持现有顺序', exact: true })).toBeDisabled()
      } else await expect(page.getByRole('alert').filter({ hasText: module.state })).toBeVisible()
      await expect(page.getByRole('button', { name: /配置详情$/ })).toHaveCount(module.path === '/settings/scheduling' ? 3 : 1)
      for (const toggle of await page.getByRole('button', { name: /配置详情$/ }).all()) await expect(toggle).toHaveAttribute('aria-expanded', 'false')
      for (const input of await page.getByRole('spinbutton').all()) await expect(input).toHaveAttribute('readonly', '')
      await expect(page.getByRole('textbox')).toHaveCount(0)
      await expect(page.getByRole('combobox')).toHaveCount(0)
      await expect(page.getByRole('switch')).toHaveCount(0)
      await expect(page.getByRole('button', { name: ['/settings/platform', '/settings/scheduling'].includes(module.path) ? /保存|立即清理|上传|升级/ : /保存|编辑|立即清理|上传|升级/ })).toHaveCount(0)
      await expect(page.locator('.settings-main')).toHaveCount(1)
      await expect(page.locator('.settings-aside')).toHaveCount(0)
      await expect(page.locator('vite-error-overlay')).toHaveCount(0)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      if (viewport.width > 1280) {
        const group = page.getByRole('region', { name: '系统管理分组', exact: true })
        await expect(group.locator('[aria-current="page"]')).toHaveCount(1)
        await expect(group.getByRole('link')).toHaveCount(3)
        await expect(group.getByRole('link', { name: '平台设置', exact: true })).toHaveAttribute('aria-current', 'page')
        await expect(group.getByRole('link', { name: /调度策略|安全策略|工具与兼容/ })).toHaveCount(0)
      }
      await page.screenshot({ path: info.outputPath(`${module.title}-${viewport.width}.png`), fullPage: false })
      await page.screenshot({ path: info.outputPath(`${module.title}-${viewport.width}-full.png`), fullPage: true })
      await page.reload()
      await expect(page.getByRole('heading', { name: module.title, exact: true })).toBeVisible()
      await expect(page.getByRole('tab', { name: module.title, exact: true })).toHaveAttribute('aria-selected', 'true')
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

test('滚动后切换设置页签回到标题，锚点限定于当前正文', async ({ page }) => {
  await prepare(page)
  await page.setViewportSize({ width: 1440, height: 720 })
  await page.goto('/settings/security')
  await page.getByRole('heading', { name: '执行能力与审计', exact: true }).scrollIntoViewIfNeeded()
  await expect.poll(() => page.evaluate(() => scrollY)).toBeGreaterThan(0)
  await page.getByRole('tab', { name: '调度策略', exact: true }).click()
  await expect(page.getByRole('heading', { name: '调度策略', exact: true })).toBeInViewport()
  await page.goto('/settings/platform#%E0%A4%A')
  await expect(page.getByRole('heading', { name: '平台设置', exact: true })).toBeInViewport()
  await page.goto('/settings/platform#product-navigation')
  await expect(page.getByRole('heading', { name: '平台设置', exact: true })).toBeInViewport()
})

test('页签切换受离开确认保护，取消切换保持路由、选中项和日志输入', async ({ page }) => {
  await prepare(page); await page.goto('/settings/platform')
  await page.getByRole('button', { name: '编辑普通日志策略', exact: true }).click()
  await page.getByRole('spinbutton', { name: '普通日志保留天数' }).fill('60')
  await page.getByRole('textbox', { name: '变更原因', exact: true }).fill('合成页签离开验证')
  await page.getByRole('tab', { name: '安全策略', exact: true }).click()
  const leave = page.getByRole('dialog', { name: '放弃未保存的普通日志设置？', exact: true })
  await expect(leave).toBeVisible()
  await expect(page.getByRole('tab', { name: '常规设置', exact: true })).toHaveAttribute('aria-selected', 'true')
  await leave.getByRole('button', { name: '继续编辑', exact: true }).click()
  await expect(page).toHaveURL('/settings/platform')
  await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('60')
  await page.getByRole('tab', { name: '安全策略', exact: true }).click()
  await leave.getByRole('button', { name: '放弃并离开', exact: true }).click()
  await expect(page).toHaveURL('/settings/security')
  await expect(page.getByRole('tab', { name: '安全策略', exact: true })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('tab', { name: '常规设置', exact: true }).click()
  await expect(page.getByRole('textbox')).toHaveCount(0)
  await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('30')
})

test('保存中的页签切换不卸载写会话，保存并复读后允许切换', async ({ page }) => {
  await prepare(page)
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/settings/log-retention', async route => {
    if (route.request().method() === 'PATCH') await pending
    await route.fallback()
  })
  try {
    await page.goto('/settings/platform')
    await page.getByRole('button', { name: '编辑普通日志策略', exact: true }).click()
    await page.getByRole('spinbutton', { name: '普通日志保留天数' }).fill('60')
    await page.getByRole('textbox', { name: '变更原因', exact: true }).fill('合成保存中切换验证')
    await page.getByRole('button', { name: '核对变更', exact: true }).click()
    const request = page.waitForRequest(request => request.method() === 'PATCH')
    await page.getByRole('dialog', { name: '核对普通日志保留策略' }).getByRole('button', { name: '确认保存', exact: true }).click()
    await request
    // 保存确认弹层仍持有焦点；直接派发页签事件验证路由入口同样拒绝离开。
    await page.getByRole('tab', { name: '工具与兼容', exact: true }).dispatchEvent('click')
    await expect(page).toHaveURL('/settings/platform')
    await expect(page.getByRole('tab', { name: '常规设置', exact: true })).toHaveAttribute('aria-selected', 'true')
    release()
    await expect(page.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('60')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await page.getByRole('tab', { name: '工具与兼容', exact: true }).click()
    await expect(page).toHaveURL('/settings/tools')
    await expect(page.getByRole('heading', { name: 'OBDUMPER', exact: true })).toBeVisible()
  } finally { release() }
})

test('键盘切换页签与浏览器前进后退保持唯一活动配置域', async ({ page }) => {
  await prepare(page); await page.goto('/settings/platform')
  const scheduling = page.getByRole('tab', { name: '调度策略', exact: true })
  await scheduling.focus(); await page.keyboard.press('Enter')
  await expect(page).toHaveURL('/settings/scheduling')
  await expect(scheduling).toBeFocused()
  await page.getByRole('tab', { name: '安全策略', exact: true }).click()
  await expect(page).toHaveURL('/settings/security')
  await page.goBack(); await expect(page).toHaveURL('/settings/scheduling'); await expect(scheduling).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('heading', { name: '全平台执行并发', exact: true })).toBeVisible()
  await page.goForward(); await expect(page.getByRole('tab', { name: '安全策略', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('heading', { name: '全平台执行并发', exact: true })).toHaveCount(0)
})

test('分组表单默认锁定，取消恢复生效值，各组不串改或发送写入', async ({ page }, info) => {
  const models = await prepare(page); await page.setViewportSize({ width: 1920, height: 1080 }); await page.goto('/settings/platform')
  const days = page.getByRole('spinbutton', { name: '普通日志保留天数', exact: true })
  await expect(days).toHaveValue('30'); await expect(days).toHaveAttribute('readonly', '')
  await days.focus(); await page.keyboard.press('ArrowUp'); await expect(days).toHaveValue('30')
  await page.getByRole('button', { name: '编辑普通日志策略', exact: true }).click()
  await expect(days).not.toHaveAttribute('readonly', ''); await days.fill('60')
  await page.getByRole('textbox', { name: '变更原因', exact: true }).fill('合成取消验证')
  await page.getByRole('button', { name: '取消编辑', exact: true }).click()
  await expect(days).toHaveValue('30'); await expect(days).toHaveAttribute('readonly', ''); await expect(page.getByRole('textbox')).toHaveCount(0)
  await page.getByRole('tab', { name: '调度策略', exact: true }).click()
  const candidates = page.getByRole('region', { name: '候选节点顺序', exact: true })
  await expect(candidates.getByRole('radio', { name: '保持现有顺序', exact: true })).toBeChecked()
  await expect(candidates.getByRole('radio', { name: '在线节点优先', exact: true })).toBeDisabled()
  await candidates.getByRole('button', { name: '编辑候选顺序', exact: true }).click()
  await candidates.getByRole('radio', { name: '在线节点优先', exact: true }).check()
  await expect(page.getByRole('spinbutton', { name: '全平台最大并发', exact: true })).toHaveValue('8')
  await expect(page.getByRole('spinbutton', { name: '节点心跳超时', exact: true })).toHaveValue('120')
  await expect(page.getByRole('spinbutton', { name: '任务启动超时', exact: true })).toHaveValue('')
  await candidates.getByRole('button', { name: '取消候选编辑', exact: true }).click()
  await expect(candidates.getByRole('radio', { name: '保持现有顺序', exact: true })).toBeChecked()
  await expect(candidates.getByRole('radio', { name: '保持现有顺序', exact: true })).toBeDisabled()
  for (const model of Object.values(models)) expect(model.writes).toHaveLength(0)
  await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: info.outputPath('settings-form-cancel.png'), fullPage: true })
})

test('刷新撤权撤下表单和已展开详情，恢复读取后不重放旧详情', async ({ page }) => {
  const { retention } = await prepare(page); await page.goto('/settings/platform')
  const section = page.getByRole('region', { name: '普通任务日志', exact: true }), details = section.getByRole('button', { name: /配置详情$/ })
  await expect(details).toHaveAttribute('aria-expanded', 'false'); await details.click()
  await expect(section.getByText('内置默认', { exact: true })).toBeVisible()
  retention.setMode('denied'); await section.getByRole('button', { name: '重新读取', exact: true }).click()
  await expect(section.getByRole('alert')).toContainText('系统管理员权限')
  await expect(section.getByRole('spinbutton')).toHaveCount(0); await expect(details).toHaveCount(0)
  await expect(section.getByText('内置默认', { exact: true })).toHaveCount(0)
  retention.setMode('ready'); await section.getByRole('button', { name: '重新读取', exact: true }).click()
  await expect(section.getByRole('spinbutton', { name: '普通日志保留天数' })).toHaveValue('30')
  await expect(details).toHaveAttribute('aria-expanded', 'false'); expect(retention.writes).toHaveLength(0)
})
