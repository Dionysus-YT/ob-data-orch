import { expect, test, type Page } from '@playwright/test'
import { permissionFixture } from './fixtures/access-control'

const group = (page: Page) => page.getByRole('region', { name: '系统管理分组', exact: true })
const trigger = (page: Page) => group(page).getByRole('button', { name: '系统管理', exact: true })
const submenu = (page: Page) => page.getByRole('menu', { name: '系统管理子菜单', exact: true })

async function prepare(page: Page) {
  await permissionFixture(page)
  await page.route('**/api/v1/storage-credentials', route => route.fulfill({ json: { items: [] } }))
}

async function choose(page: Page, label: string, collapsed: boolean) {
  if (collapsed) {
    await trigger(page).click()
    await expect(submenu(page)).toBeVisible()
    await expect(submenu(page).getByRole('menuitem')).toHaveCount(3)
    await submenu(page).getByRole('menuitem', { name: label, exact: true }).click()
    await expect(submenu(page)).toHaveCount(0)
  } else await group(page).getByRole('link', { name: label, exact: true }).click()
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`系统管理父子导航、独立页面和深链接 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.setViewportSize(viewport)
    await prepare(page)
    await page.goto('/settings/access-control')
    await expect(page).toHaveTitle(/OB Data Orch/)
    await expect(page.getByRole('heading', { name: '权限配置', exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: '返回系统设置', exact: true })).toHaveCount(0)
    const collapsed = viewport.width === 1280
    if (collapsed) {
      await expect(group(page).getByRole('link')).toHaveCount(0)
      await expect(trigger(page)).toHaveClass(/active/)
      await trigger(page).click()
      await expect(submenu(page).getByRole('menuitem', { name: '权限配置', exact: true })).toHaveAttribute('aria-current', 'page')
      await expect(page.locator('.ant-dropdown:visible')).toHaveCSS('opacity', '1')
      await page.screenshot({ path: info.outputPath(`system-menu-${viewport.width}.png`), fullPage: false })
      await page.keyboard.press('Escape')
      await expect(trigger(page)).toBeFocused()
      await expect(page.locator('.ant-dropdown:visible')).toHaveCount(0)
    } else {
      await expect(group(page).getByRole('link')).toHaveCount(3)
      await expect(group(page).locator('[aria-current="page"]')).toHaveCount(1)
      await trigger(page).click()
      await expect(trigger(page)).toHaveAttribute('aria-expanded', 'false')
      await expect(group(page).getByRole('link', { name: '权限配置', exact: true })).not.toBeVisible()
      await expect(page).toHaveURL(/\/settings\/access-control$/)
      await trigger(page).click()
    }
    await page.screenshot({ path: info.outputPath(`system-navigation-${viewport.width}.png`), fullPage: false })
    for (const [label, path] of [['平台设置', '/settings/platform'], ['存储凭据', '/settings/storage-credentials'], ['权限配置', '/settings/access-control']] as const) {
      await choose(page, label, collapsed)
      await expect(page).toHaveURL(new RegExp(`${path}$`))
      await expect(page.getByRole('heading', { name: label, exact: true })).toBeVisible()
      if (!collapsed) await expect(group(page).getByRole('link', { name: label, exact: true })).toHaveAttribute('aria-current', 'page')
    }
    await page.reload()
    await expect(page.getByRole('heading', { name: '权限配置', exact: true })).toBeVisible()
    await expect(trigger(page)).toHaveAttribute('aria-expanded', String(!collapsed))
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    expect(errors).toEqual([])
    await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
  })
}

test('系统管理键盘路径与布局变化撤销旧弹层', async ({ page }) => {
  await prepare(page)
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/settings/access-control')
  await trigger(page).focus()
  await page.keyboard.press('Space')
  await expect(trigger(page)).toHaveAttribute('aria-expanded', 'false')
  await page.keyboard.press('ArrowRight')
  await expect(group(page).getByRole('link', { name: '权限配置', exact: true })).toBeFocused()
  await page.keyboard.press('ArrowLeft')
  await expect(trigger(page)).toBeFocused()
  await page.keyboard.press('Enter')
  await page.keyboard.press('Tab')
  await expect(group(page).getByRole('link', { name: '平台设置', exact: true })).toBeFocused()
  await page.setViewportSize({ width: 1280, height: 720 })
  await trigger(page).focus()
  await page.keyboard.press('ArrowDown')
  await expect(submenu(page).getByRole('menuitem', { name: '权限配置', exact: true })).toBeFocused()
  await page.keyboard.press('End')
  await expect(submenu(page).getByRole('menuitem', { name: '存储凭据', exact: true })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/settings\/storage-credentials$/)
  await expect(submenu(page)).toHaveCount(0)
  await expect(trigger(page)).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(submenu(page)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(submenu(page)).toHaveCount(0)
  await expect(trigger(page)).toBeFocused()
  await trigger(page).click()
  await page.setViewportSize({ width: 1440, height: 1024 })
  await expect(submenu(page)).toHaveCount(0)
  await expect(group(page).getByRole('link', { name: '存储凭据', exact: true })).toHaveAttribute('aria-current', 'page')
})

test('移动导航内保留系统管理层级，子页面切换关闭导航', async ({ page }, info) => {
  await prepare(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/settings/access-control')
  await page.getByRole('button', { name: '打开导航', exact: true }).click()
  await expect(group(page)).toBeVisible()
  await expect(group(page).getByRole('link', { name: '权限配置', exact: true })).toBeFocused()
  await page.screenshot({ path: info.outputPath('system-navigation-mobile.png'), fullPage: false })
  await group(page).getByRole('link', { name: '存储凭据', exact: true }).click()
  await expect(page.getByRole('heading', { name: '存储凭据', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '打开导航', exact: true })).toBeFocused()
  await page.getByRole('button', { name: '打开导航', exact: true }).click()
  await expect(group(page).getByRole('link', { name: '存储凭据', exact: true })).toBeFocused()
  await trigger(page).click()
  await page.getByRole('button', { name: '关闭导航', exact: true }).first().click()
  await page.getByRole('button', { name: '打开导航', exact: true }).click()
  await expect(trigger(page)).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(group(page).getByRole('link', { name: '存储凭据', exact: true })).toBeFocused()
})
