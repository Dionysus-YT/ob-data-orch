import { expect, test } from '@playwright/test'
import { mockFacts } from './fixtures/facts'
import { overviewFixture } from './fixtures/overview'

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`首页不依赖其他页面加载，导航后按需打开导出 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    await page.setViewportSize(viewport)
    await mockFacts(page)
    await page.route('**/api/v1/overview?*', route => route.fulfill({ json: { item: overviewFixture() } }))
    const errors: string[] = [], requestedViews: string[] = [], prematureRequests: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    page.on('request', request => {
      const path = new URL(request.url()).pathname
      if (path.startsWith('/src/views/')) requestedViews.push(path)
    })
    // 未进入的页面即使不可下载，也不能阻塞登录后的首页；只拦源码，不伪造模块成功。
    await page.route('**/src/views/**', route => {
      if (new URL(route.request().url()).pathname === '/src/views/HomeView.vue') return route.continue()
      prematureRequests.push(route.request().url())
      return route.abort()
    })
    await page.goto('/')
    await expect(page).toHaveTitle('LDP · 首页')
    await expect(page.getByRole('heading', { name: '首页', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '今日失败任务 2 个，查看任务' })).toBeVisible()
    expect(prematureRequests).toEqual([])
    expect(requestedViews).toEqual(['/src/views/HomeView.vue'])
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await page.screenshot({ path: info.outputPath(`startup-home-${viewport.width}.png`), fullPage: false })

    await page.unroute('**/src/views/**')
    await page.getByRole('link', { name: '导出任务', exact: true }).click()
    await expect(page).toHaveURL(/\/exports\/new$/)
    await expect(page.getByRole('heading', { name: '新建导出任务', exact: true })).toBeVisible()
    await expect(page.getByRole('radio').first()).toBeVisible()
    expect(requestedViews).toContain('/src/views/ExportWizardView.vue')
    await page.getByRole('radio').first().check()
    await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
    await expect(page.getByRole('radiogroup', { name: '导出内容', exact: true })).toBeVisible()
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    expect(errors).toEqual([])
    await page.screenshot({ path: info.outputPath(`startup-export-${viewport.width}.png`), fullPage: false })
  })
}
