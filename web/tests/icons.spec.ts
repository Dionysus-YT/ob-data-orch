import { expect, test } from '@playwright/test'

test('产品图标统一尺寸、装饰语义与菜单对齐，窄屏导航可操作', async ({ page }, info) => {
  await page.route('**/api/v1/**', route => route.abort())
  await page.goto('/data-sources?uiFixture=data-sources')
  await expect(page.getByRole('table')).toBeVisible()
  await expect(page.locator('.nav-icon')).toHaveCount(11)
  const sizes = await page.locator('.product-icon').evaluateAll(elements => elements.map(element => ({
    size: getComputedStyle(element).fontSize,
    hidden: element.getAttribute('aria-hidden'),
    svg: Boolean(element.querySelector('svg[data-icon]')),
  })))
  expect(sizes.length).toBeGreaterThan(20)
  expect(sizes.every(icon => ['14px', '16px'].includes(icon.size) && icon.hidden === 'true' && icon.svg)).toBe(true)
  const create = page.getByRole('button', { name: '新建数据源', exact: true })
  expect((await create.boundingBox())!.height).toBe(36)
  await page.getByRole('button', { name: 'Production finance reporting 的操作' }).click()
  await expect(page.getByRole('menuitem', { name: '编辑', exact: true })).toBeVisible()
  await expect(page.getByRole('menuitem').locator('.product-icon')).toHaveCount(2)
  await page.screenshot({ path: info.outputPath('ant-product-icons.png'), fullPage: true })
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: '打开导航', exact: true }).click()
  await expect(page.getByRole('complementary', { name: '主导航' })).toBeVisible()
  await expect(page.getByRole('button', { name: '关闭导航', exact: true }).first()).toBeVisible()
  await page.getByRole('complementary', { name: '主导航' }).press('Escape')
  await expect(page.getByRole('button', { name: '打开导航', exact: true })).toBeFocused()
})
