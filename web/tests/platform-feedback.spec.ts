import { expect, test, type Page } from '@playwright/test'
import { mockFacts } from './fixtures/facts'

const time = '2026-10-10T00:00:00Z'
const node = {
  id: 'synthetic-feedback-node', displayName: '合成反馈节点', platform: 'WINDOWS_AMD64',
  managementState: 'DISABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE',
  lastHeartbeatAt: time, environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE',
  acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED'], revision: 1, updatedAt: time, createdAt: time,
  allowedRoots: ['/E:/synthetic-exports'], toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java',
}

async function navigate(page: Page, path: string) {
  await page.evaluate(async path => {
    const modulePath = '/src/router/index.ts'
    const { router } = await import(/* @vite-ignore */ modulePath)
    await router.push(path)
  }, path)
}

test('节点启用短时确认，预检查边界保留且读取不重放成功', async ({ page }, info) => {
  await mockFacts(page)
  let current = { ...node }
  await page.route('**/api/v1/execution-nodes/synthetic-feedback-node', route => route.fulfill({ json: { item: current } }))
  await page.route('**/api/v1/execution-nodes/synthetic-feedback-node:enable', route => {
    current = { ...current, managementState: 'ENABLED', acceptsNewTasks: true, unavailableReasons: [], revision: 2 }
    return route.fulfill({ json: { item: current } })
  })
  await page.goto('/nodes/synthetic-feedback-node')
  await page.getByRole('button', { name: '启用节点', exact: true }).click()
  const notice = page.locator('.ant-message').getByRole('status').filter({ hasText: '节点已启用。' })
  await expect(notice).toBeVisible()
  await expect(page.locator('.node-detail .ant-alert-success')).toHaveCount(0)
  await expect(page.getByText('具体任务仍需核验数据源、路径、对象和参数。', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('node-operation-message.png'), fullPage: true })
  await expect(notice).toBeHidden({ timeout: 5500 })
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('具体任务仍需核验数据源、路径、对象和参数。', { exact: true })).toBeVisible()
  await expect(notice).toHaveCount(0)
})

test('注册码复制失败持久警示，成功短时确认且关闭清除', async ({ page }) => {
  await mockFacts(page)
  await page.addInitScript(() => {
    Reflect.set(window, 'syntheticCopyDenied', true)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {
      writeText: async () => { if (Reflect.get(window, 'syntheticCopyDenied')) throw new Error('synthetic-denied') },
    } })
  })
  await page.route('**/api/v1/execution-nodes/synthetic-feedback-node', route => route.fulfill({ json: { item: { ...node, agentAssociationStatus: 'PENDING', heartbeatStatus: 'NEVER_CONNECTED', lastHeartbeatAt: null, environmentStatus: 'NOT_CHECKED', capacityStatus: 'UNKNOWN', unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED'] } } }))
  await page.route('**/api/v1/execution-nodes/synthetic-feedback-node:enrollments', route => route.fulfill({ status: 201, json: {
    requestId: 'synthetic-request', enrollmentId: 'synthetic-enrollment', nodeId: node.id,
    enrollmentMaterial: 'synthetic-only-material', expiresAt: '2099-01-01T00:00:00Z', displayedOnce: true,
  } }))
  await page.goto('/nodes/synthetic-feedback-node')
  await page.getByRole('button', { name: '生成一次性注册码', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '复制注册码', exact: true }).click()
  const error = dialog.locator('.ant-alert-error').filter({ hasText: '注册码复制失败，请手动选择复制。' })
  await expect(error).toBeVisible()
  await page.waitForTimeout(3200)
  await expect(error).toBeVisible()
  await page.evaluate(() => Reflect.set(window, 'syntheticCopyDenied', false))
  await dialog.getByRole('button', { name: '复制注册码', exact: true }).click()
  await expect(error).toHaveCount(0)
  const message = page.locator('.ant-message')
  await expect(message.getByText('注册码已复制。', { exact: true })).toBeVisible()
  await expect(message).not.toContainText('synthetic-only-material')
  await navigate(page, '/settings')
  await expect(message.getByText('注册码已复制。', { exact: true })).toHaveCount(0)
})

test('模板重复成功替换同一提示，离开页面不保留旧成功', async ({ page }) => {
  await mockFacts(page)
  let revision = 1
  const template = { id: 'synthetic-template', displayName: '合成初始模板', revision, capabilityVersion: 'synthetic-capability', configFingerprint: 'synthetic-fingerprint', createdAt: time, updatedAt: time }
  await page.route('**/api/v1/export-config-templates', route => route.fulfill({ json: { items: [template] } }))
  await page.route('**/api/v1/export-config-templates/synthetic-template', route => route.fulfill({ json: { revision: ++revision } }))
  await page.goto('/templates')
  for (const name of ['合成新模板', '合成初始模板', '合成新模板']) {
    await page.getByRole('button', { name: '改名', exact: true }).click()
    await page.getByRole('textbox', { name: '模板名称', exact: true }).fill(name)
    await page.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.ant-message').getByText(`模板「${name}」已改名。`, { exact: true })).toBeVisible()
    await expect(page.locator('.ant-message-notice')).toHaveCount(1)
  }
  await navigate(page, '/settings')
  await expect(page.locator('.ant-message-notice')).toHaveCount(0)
})

for (const [path, endpoint, title] of [
  ['/tasks', '/api/v1/tasks', '无法加载任务列表'],
  ['/templates', '/api/v1/export-config-templates', '操作未完成'],
  ['/settings/storage-credentials', '/api/v1/storage-credentials', '无法加载存储凭据'],
]) test(`${title}使用持久 Ant Alert`, async ({ page }) => {
  await mockFacts(page)
  await page.route(`**${endpoint}*`, route => route.fulfill({ status: 503, json: { code: 'SYNTHETIC_UNAVAILABLE' } }))
  await page.goto(path!)
  const alert = page.locator('.ant-alert-error').filter({ hasText: title! })
  await expect(alert).toBeVisible()
  await page.waitForTimeout(3200)
  await expect(alert).toBeVisible()
  await expect(page.locator('.feedback-error, .feedback-notice, .storage-credential-feedback')).toHaveCount(0)
  await expect(page.locator('.ant-message-notice')).toHaveCount(0)
})

test('数据源解析重复成功可感知，关闭编辑会话清除短时提示', async ({ page }) => {
  await page.route('**/api/v1/**', route => route.abort())
  await page.goto('/data-sources?uiFixture=data-sources')
  await page.getByRole('button', { name: 'Production finance reporting', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '编辑数据源', exact: true })
  for (let attempt = 0; attempt < 2; attempt++) {
    const connection = ['mysql', '-h', '192.0.2.20', '-P', '2881', '-u', 'synthetic@synthetic', '-D', 'synthetic_db', '-p'].join(' ')
    await drawer.locator('#source-parser-input').fill(connection)
    await drawer.getByRole('button', { name: '智能解析', exact: true }).click()
    await expect(page.locator('.ant-message').getByText('连接串已解析到表单，尚未保存。', { exact: true })).toBeVisible()
    await expect(page.locator('.ant-message-notice')).toHaveCount(1)
    await expect(drawer.locator('.ant-alert-success')).toHaveCount(0)
  }
  await drawer.getByRole('button', { name: '取消', exact: true }).click()
  await page.getByRole('button', { name: '放弃更改', exact: true }).click()
  await expect(drawer).toBeHidden()
  await expect(page.locator('.ant-message-notice')).toHaveCount(0)
})
