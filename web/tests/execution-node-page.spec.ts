import { expect, test } from '@playwright/test'

const fixedTime = '2026-09-16T00:00:00Z'
const pendingNode = {
  id: 'synthetic-node', displayName: '合成执行节点', platform: 'WINDOWS_AMD64',
  managementState: 'DISABLED', agentAssociationStatus: 'PENDING', heartbeatStatus: 'NEVER_CONNECTED',
  lastHeartbeatAt: null, environmentStatus: 'NOT_CHECKED', capacityStatus: 'UNKNOWN',
  acceptsNewTasks: false, unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED'],
  revision: 1, updatedAt: fixedTime,
}
const detail = {
  ...pendingNode, allowedRoots: ['/E:/synthetic-exports'], toolHome: 'E:\\synthetic-tools',
  javaPath: 'C:\\synthetic-java\\bin\\java.exe', createdAt: fixedTime,
}

test('执行节点列表保留可信事实，并区分刷新错误与权限错误', async ({ page }, info) => {
  let response: 'ok' | 'error' | 'restricted' = 'ok'
  await page.route('**/api/v1/execution-nodes', route => {
    if (response === 'error') return route.fulfill({ status: 503, json: { code: 'TEMPORARY', message: '合成读取失败' } })
    if (response === 'restricted') return route.fulfill({ status: 403, json: { code: 'FORBIDDEN', message: '无权限' } })
    return route.fulfill({ json: { items: [pendingNode] } })
  })
  await page.route('**/api/v1/execution-nodes/synthetic-node', route => route.fulfill({ json: { item: detail } }))
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/nodes')
  await expect(page.getByRole('link', { name: '合成执行节点' })).toBeVisible()
  await expect(page.getByText('需要完成 Agent 关联')).toBeVisible()
  await expect(page.getByRole('combobox', { name: '管理状态' })).toBeVisible()
  await page.getByRole('button', { name: '合成执行节点 的操作' }).click()
  await page.getByRole('menuitem', { name: '编辑节点' }).click()
  await expect(page).toHaveURL(/\/nodes\/synthetic-node\/edit$/)
  await page.goBack()
  await expect(page.getByRole('link', { name: '合成执行节点' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('node-list-1280.png'), fullPage: true })
  for (const width of [1440, 1920]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${width}px`).toBe(true)
  }
  await page.setViewportSize({ width: 640, height: 720 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), '640px').toBe(true)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.getByRole('textbox', { name: '搜索节点名称或标识' }).fill('不存在')
  await expect(page.getByText('没有匹配的执行节点')).toBeVisible()
  await page.getByRole('region', { name: '当前已加载的授权执行节点' }).getByRole('button', { name: '清除筛选' }).click()
  response = 'error'
  await page.getByRole('button', { name: '刷新执行节点' }).click()
  await expect(page.getByText('当前仍展示上一次加载的节点')).toBeVisible()
  await expect(page.getByRole('link', { name: '合成执行节点' })).toBeVisible()
  response = 'restricted'
  await page.getByRole('button', { name: '刷新执行节点' }).click()
  await expect(page.getByText('当前身份无权访问执行节点')).toBeVisible()
  await expect(page.getByRole('link', { name: '合成执行节点' })).toHaveCount(0)
})

test('节点表单校验、一次性注册码展示与关闭清理', async ({ page }, info) => {
  let created = false
  await page.route('**/api/v1/execution-nodes', route => {
    if (route.request().method() === 'POST') { created = true; return route.fulfill({ status: 201, json: { id: pendingNode.id } }) }
    return route.fulfill({ json: { items: [] } })
  })
  await page.route('**/api/v1/execution-nodes/synthetic-node', route => route.fulfill({ json: { item: detail } }))
  await page.route('**/api/v1/execution-nodes/synthetic-node:enrollments', route => route.fulfill({ status: 201, json: {
    requestId: 'synthetic-request', enrollmentId: 'synthetic-enrollment', nodeId: pendingNode.id,
    enrollmentMaterial: 'synthetic-one-time-material', expiresAt: '2026-10-01T00:00:00Z', displayedOnce: true,
  } }))
  await page.goto('/nodes/new')
  await page.getByRole('button', { name: '保存并继续 Agent 关联' }).click()
  await expect(page.getByText('请检查以下字段')).toBeVisible()
  expect(created).toBe(false)
  await page.screenshot({ path: info.outputPath('node-form-validation.png'), fullPage: true })
  await page.getByLabel('节点名称').fill('合成执行节点')
  await page.getByLabel('OB Loader/Dumper 安装目录').fill('E:\\synthetic-tools')
  await page.getByLabel('工具专用 Java 8 路径').fill('C:\\synthetic-java\\bin\\java.exe')
  await page.getByLabel('导出数据目录白名单').fill('/E:/synthetic-exports')
  await page.getByRole('button', { name: '保存并继续 Agent 关联' }).click()
  await expect(page).toHaveURL(/\/nodes\/synthetic-node\?registration=created/)
  await expect(page.getByRole('heading', { name: '合成执行节点' })).toBeVisible()
  await page.screenshot({ path: info.outputPath('node-detail.png'), fullPage: true })
  await page.getByRole('button', { name: '生成一次性注册码' }).click()
  const dialog = page.getByRole('dialog', { name: '一次性 Agent 注册码' })
  await expect(dialog.getByLabel('一次性 Agent 注册码')).toHaveValue(/^obdo-r1\./)
  await dialog.getByRole('button', { name: '关闭并清除' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByLabel('一次性 Agent 注册码')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '生成一次性注册码' })).toBeFocused()
  await page.setViewportSize({ width: 640, height: 720 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.goto('/nodes/synthetic-node/edit')
  await expect(page.getByRole('heading', { name: '编辑执行节点' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
