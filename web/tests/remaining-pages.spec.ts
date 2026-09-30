import { expect, test } from '@playwright/test'

test('其余现役页面保持可读、禁用边界与统一控件高度', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', e => errors.push(e.message))
  await page.route('**/api/v1/**', route => route.fulfill({ json: { items: [], totalPages: 0 } }))
  await page.setViewportSize({ width: 1280, height: 720 })
  for (const [path, heading] of [
    ['/tasks', '任务中心'], ['/nodes', '执行节点'], ['/nodes/new', '注册执行节点'],
    ['/templates', '模板中心'], ['/settings/storage-credentials', '存储凭据'],
    ['/logs', '日志中心'], ['/settings', '系统设置'], ['/settings/access-control', '权限配置'],
    ['/imports/normal/new', '新建普通导入任务'], ['/imports/direct/new', '新建旁路导入任务'],
  ]) {
    await page.goto(path!)
    await expect(page.getByRole('heading', { name: heading!, exact: true }).first()).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), path).toBe(true)
    await page.screenshot({ path: info.outputPath(`${heading}.png`), fullPage: true })
  }
  expect(errors).toEqual([])
})

test('任务游标和页大小沿用服务端请求，不切换为 Table 客户端分页', async ({ page }) => {
  const requests: URL[] = []
  await page.route('**/api/v1/tasks?*', route => {
    const url = new URL(route.request().url()); requests.push(url)
    return route.fulfill({ json: { items: [], totalPages: 2, nextCursor: url.searchParams.has('cursor') ? '' : 'synthetic-next' } })
  })
  await page.goto('/tasks')
  await page.getByRole('button', { name: '下一页', exact: true }).click()
  await expect.poll(() => requests.at(-1)?.searchParams.get('cursor')).toBe('synthetic-next')
  await page.getByRole('combobox', { name: '每页显示' }).focus()
  await page.keyboard.press('ArrowDown')
  await page.getByText('20 条', { exact: true }).click()
  await expect.poll(() => requests.at(-1)?.searchParams.get('limit')).toBe('20')
  expect(requests.at(-1)?.searchParams.has('cursor')).toBe(false)
})

test('存储凭据表单的可访问标签、失败保留与成功清理秘密', async ({ page }) => {
  let writes = 0
  await page.route('**/api/v1/storage-credentials', route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { items: [] } })
    writes++
    return writes === 1 ? route.fulfill({ status: 409, json: { code: 'REVISION_CONFLICT' } }) : route.fulfill({ json: { item: { id: 'synthetic-credential', displayName: '合成凭据', provider: 'OSS', revision: 1, currentRevision: 1, updatedAt: '2026-09-16T00:00:00Z' } } })
  })
  await page.goto('/settings/storage-credentials')
  await page.getByRole('button', { name: '新增凭据', exact: true }).click()
  await page.getByLabel('凭据名称', { exact: true }).fill('合成凭据')
  await page.getByLabel('AccessKey', { exact: true }).fill('synthetic-access-fixture')
  await page.getByLabel('SecretKey', { exact: true }).fill('synthetic-secret-fixture')
  await page.getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.getByRole('alert').filter({ hasText: '存储凭据已发生变化' })).toBeVisible()
  await expect(page.getByLabel('SecretKey', { exact: true })).toHaveValue('synthetic-secret-fixture')
  await page.getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.getByRole('button', { name: '轮换', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '轮换', exact: true }).click()
  await expect(page.getByLabel('AccessKey', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('SecretKey', { exact: true })).toHaveValue('')
})
