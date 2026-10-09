import { expect, test, type Page, type Route } from '@playwright/test'
import { mockFacts } from './fixtures/facts'

const time = '2026-10-09T00:00:00Z'
function node(id: string, associated = false) {
  return { id, displayName: `合成节点 ${id}`, platform: 'WINDOWS_AMD64', managementState: 'DISABLED', agentAssociationStatus: associated ? 'ASSOCIATED' : 'PENDING', heartbeatStatus: associated ? 'ONLINE' : 'NEVER_CONNECTED', lastHeartbeatAt: associated ? time : null, environmentStatus: associated ? 'EXPIRED' : 'NOT_CHECKED', capacityStatus: associated ? 'AVAILABLE' : 'UNKNOWN', acceptsNewTasks: false, unavailableReasons: [associated ? 'ENVIRONMENT_EXPIRED' : 'AGENT_ASSOCIATION_REQUIRED'], revision: 1, updatedAt: time, createdAt: time, allowedRoots: ['/E:/synthetic-exports'], toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java' }
}
function material(id: string) { return { requestId: 'synthetic-request', enrollmentId: `synthetic-enrollment-${id}`, nodeId: id, enrollmentMaterial: 'synthetic-only-material', expiresAt: '2099-01-01T00:00:00Z', displayedOnce: true } }
async function navigate(page: Page, path: string) {
  // 真实路由复用实例，不通过刷新或组件 key 掩盖旧回调问题。
  await page.evaluate(async path => {
    const modulePath = '/src/router/index.ts'
    const { router } = await import(/* @vite-ignore */ modulePath)
    await router.push(path)
  }, path)
}
async function reads(page: Page, associated = false) {
  await mockFacts(page)
  await page.route('**/api/v1/execution-nodes/*', route => {
    const id = new URL(route.request().url()).pathname.split('/').pop()!
    return route.fulfill({ json: { item: node(id, associated) } })
  })
}

test('节点详情同实例隔离迟到刷新及签发，关闭与路由切换清除材料', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  await reads(page); await page.goto('/nodes/A'); await expect(page.getByRole('heading', { name: '合成节点 A' })).toBeVisible()
  await page.locator('.node-detail-header').evaluate(element => element.setAttribute('data-instance', 'retained'))
  const held: Route[] = []
  await page.route('**/api/v1/execution-nodes/A', route => { held.push(route) })
  await page.getByRole('button', { name: '刷新节点详情' }).click(); await expect.poll(() => held.length).toBe(1)
  await navigate(page, '/nodes/B'); await expect(page.getByRole('heading', { name: '合成节点 B' })).toBeVisible()
  await held[0]!.fulfill({ json: { item: node('A') } }).catch(() => {})
  await expect(page.locator('.node-detail-header')).toHaveAttribute('data-instance', 'retained'); await expect(page.getByRole('heading', { name: '合成节点 B' })).toBeVisible()
  const enrollments: Route[] = []; await page.route('**/api/v1/execution-nodes/*:enrollments', route => { enrollments.push(route) })
  await page.getByRole('button', { name: '生成一次性注册码' }).click(); await expect.poll(() => enrollments.length).toBe(1)
  await page.getByRole('button', { name: '关闭并清除' }).click()
  await enrollments[0]!.fulfill({ status: 201, json: material('B') })
  await expect(page.getByRole('textbox', { name: '一次性 Agent 注册码' })).toHaveCount(0)
  await page.getByRole('button', { name: '生成一次性注册码' }).click(); await expect.poll(() => enrollments.length).toBe(2)
  await enrollments[1]!.fulfill({ status: 201, json: material('B') }); await expect(page.getByRole('textbox', { name: '一次性 Agent 注册码' })).toHaveValue(/^obdo-r1\./)
  await navigate(page, '/nodes/C'); await expect(page.getByRole('heading', { name: '合成节点 C' })).toBeVisible(); await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(await page.evaluate(() => Object.keys(localStorage).some(key => /enrollment|registration/i.test(key)))).toBe(false)
  await page.screenshot({ path: info.outputPath('node-detail-route-isolation.png'), fullPage: true }); expect(errors).toEqual([])
})

test('节点编辑 A → B → 新建保持实例，迟到读取与编辑响应不能覆盖新表单', async ({ page }, info) => {
  await reads(page); await page.goto('/nodes/A/edit'); await expect(page.getByLabel('节点名称')).toHaveValue('合成节点 A')
  await page.locator('.node-form-heading').evaluate(element => element.setAttribute('data-instance', 'retained'))
  const writes: Route[] = []; await page.route('**/api/v1/execution-nodes/A', route => {
    if (route.request().method() === 'PATCH') writes.push(route)
    else return route.fulfill({ json: { item: node('A') } })
  })
  await page.getByRole('button', { name: '保存修改' }).click(); await expect.poll(() => writes.length).toBe(1)
  await navigate(page, '/nodes/B/edit'); await expect(page.getByLabel('节点名称')).toHaveValue('合成节点 B')
  await writes[0]!.fulfill({ json: { item: { ...node('A'), revision: 2 } } }).catch(() => {})
  await expect(page.getByLabel('节点名称')).toHaveValue('合成节点 B'); await expect(page.locator('.node-form-heading')).toHaveAttribute('data-instance', 'retained')
  const held: Route[] = []; await page.route('**/api/v1/execution-nodes/C', route => { held.push(route) })
  await navigate(page, '/nodes/C/edit'); await expect.poll(() => held.length).toBe(1)
  await navigate(page, '/nodes/new'); await expect(page.getByRole('heading', { name: '注册执行节点' })).toBeVisible()
  await expect(page.locator('.node-form-heading')).toHaveAttribute('data-instance', 'retained'); await held[0]!.fulfill({ json: { item: node('C') } }).catch(() => {})
  await expect(page.getByLabel('节点名称')).toHaveValue(''); await expect(page.getByLabel('导出数据目录白名单')).toHaveValue('')
  await page.setViewportSize({ width: 640, height: 720 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('node-form-mode-reset.png'), fullPage: true })
})

test('新建防重，离开登记页后迟到成功不得触发导航', async ({ page }) => {
  await reads(page); const posts: Route[] = []
  await page.route('**/api/v1/execution-nodes', route => { if (route.request().method() === 'POST') posts.push(route); else return route.fulfill({ json: { items: [] } }) })
  await page.goto('/nodes/new'); await page.getByLabel('节点名称').fill('合成新建节点'); await page.getByLabel('OB Loader/Dumper 安装目录').fill('E:/synthetic-tools'); await page.getByLabel('工具专用 Java 8 路径').fill('C:/synthetic-java'); await page.getByLabel('导出数据目录白名单').fill('/E:/synthetic-exports')
  await page.getByRole('button', { name: '保存并继续 Agent 关联' }).click(); await expect.poll(() => posts.length).toBe(1)
  await expect(page.getByLabel('节点名称')).toBeDisabled()
  await navigate(page, '/nodes/B/edit'); await expect(page.getByLabel('节点名称')).toHaveValue('合成节点 B')
  await posts[0]!.fulfill({ status: 201, json: { id: 'synthetic-created' } }).catch(() => {})
  await expect(page).toHaveURL(/\/nodes\/B\/edit$/); await expect(page.getByLabel('节点名称')).toHaveValue('合成节点 B'); expect(posts).toHaveLength(1)
})

test('环境操作旧响应不写新节点，单次延迟刷新在离开页面时取消', async ({ page }) => {
  await reads(page, true); await page.clock.install(); const counts = { A: 0, B: 0 }
  await page.route('**/api/v1/execution-nodes/*', route => {
    const id = new URL(route.request().url()).pathname.split('/').pop()! as 'A' | 'B'
    counts[id]++; return route.fulfill({ json: { item: node(id, true) } })
  })
  const operations: Route[] = []; await page.route('**/api/v1/execution-nodes/*:environment-check', route => { operations.push(route) })
  await page.goto('/nodes/A'); await page.getByRole('button', { name: '检查节点环境' }).click(); await expect.poll(() => operations.length).toBe(1)
  await navigate(page, '/nodes/B'); await expect(page.getByRole('heading', { name: '合成节点 B' })).toBeVisible()
  await operations[0]!.fulfill({ json: { item: { ...node('A', true), revision: 2 } } }).catch(() => {})
  await expect(page.getByRole('heading', { name: '合成节点 B' })).toBeVisible(); await page.getByRole('button', { name: '检查节点环境' }).click(); await expect.poll(() => operations.length).toBe(2)
  await operations[1]!.fulfill({ json: { item: { ...node('B', true), revision: 2 } } }); await expect(page.getByText('环境检查已请求，等待 Agent 回传固定运行时结果。')).toBeVisible()
  await navigate(page, '/nodes'); await page.clock.runFor(3000); expect(counts).toEqual({ A: 1, B: 1 })
})
