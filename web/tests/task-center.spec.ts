import { expect, test, type Page } from '@playwright/test'
import { mockFacts } from './fixtures/facts'

// 全部数据为合成夹具；不连接控制面、Agent 或数据库。
const time = '2026-10-01T02:00:00Z'
function task(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id, type: 'OBDUMPER_EXPORT', dataSourceId: 'synthetic-source', objectSummary: 'synthetic_db.orders', state: 'SUCCEEDED',
    stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: false, nodeId: 'synthetic-node',
    ownedByCurrentUser: true, submittedAt: time, startedAt: time, finishedAt: '2026-10-01T02:05:00Z', updatedAt: time,
    displayName: `synthetic_db.${id}`, dataSourceName: '合成数据源', dataSourceDeleted: false, environment: 'PRODUCTION',
    nodeName: '合成节点', nodeDeleted: false, creatorName: '合成用户', cancelRequested: false, availableActions: ['COPY_COMMAND', 'VIEW_LOGS'],
    ...overrides,
  }
}
const options = { dataSources: [{ id: 'synthetic-source', label: '合成数据源' }], nodes: [{ id: 'synthetic-node', label: '合成节点' }], creators: [], environments: ['PRODUCTION'], creatorFilterAvailable: false }

async function mockTasks(page: Page, items: unknown[], record?: (url: URL) => void) {
  await page.route('**/api/v1/tasks/filter-options', route => route.fulfill({ json: { requestId: 'r', ...options } }))
  await page.route('**/api/v1/tasks?*', route => { const url = new URL(route.request().url()); record?.(url); return route.fulfill({ json: { requestId: 'r', items, nextCursor: null, totalPages: items.length ? 1 : 0 } }) })
}

test('中文预设日期范围一次更新两个边界，清除和非法路由保持字段反馈', async ({ page }, info) => {
  const requests: URL[] = []
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.clock.setFixedTime(new Date('2026-10-14T04:00:00Z'))
  await mockTasks(page, [task('orders')], url => requests.push(url))
  await page.goto('/tasks')
  const creation = page.locator('.task-filter-range').first()
  await creation.locator('input').first().click()
  const calendar = page.locator('.ant-picker-dropdown:visible')
  await expect(calendar.locator('.ant-picker-month-btn').first()).toContainText('月')
  await expect(calendar).not.toContainText('Oct')
  await page.screenshot({ path: info.outputPath('task-date-range-zh.png'), fullPage: true })
  await calendar.getByText('最近 7 天', { exact: true }).click()
  await expect(page).toHaveURL(/createdFrom=2026-10-08/)
  await expect(page).toHaveURL(/createdTo=2026-10-14/)
  await expect.poll(() => requests.at(-1)?.searchParams.has('createdFrom') && requests.at(-1)?.searchParams.has('createdTo')).toBe(true)
  await page.getByRole('button', { name: '展开筛选' }).click()
  const started = page.locator('.task-filter-range').last()
  await started.locator('input').first().click()
  await calendar.getByText('昨天', { exact: true }).click()
  await expect(page).toHaveURL(/startedFrom=2026-10-13/)
  await expect(page).toHaveURL(/startedTo=2026-10-13/)
  await page.getByRole('button', { name: '清除筛选' }).click()
  await expect(creation.locator('input').first()).toHaveValue('')
  await expect(started.locator('input').last()).toHaveValue('')
  await page.goto('/tasks?createdFrom=2026-10-15&createdTo=2026-10-14')
  await expect(page.locator('.ant-form-item-explain-error')).toContainText('开始日期晚于结束日期')
  expect(errors).toEqual([])
})

test('筛选写入路由并传给服务端，摘要可移除，返回详情后恢复条件', async ({ page }) => {
  const requests: URL[] = []
  await mockTasks(page, [task('orders')], url => requests.push(url))
  await page.route('**/api/v1/tasks/orders*', route => route.fulfill({ status: 404, json: { code: 'NOT_FOUND' } }))
  await page.goto('/tasks?nodeId=synthetic-node&scope=MINE')
  await expect(page.getByRole('link', { name: 'synthetic_db.orders', exact: true })).toBeVisible()
  await expect(page.getByText('执行节点：合成节点', { exact: true })).toBeVisible()
  await page.getByLabel('任务名称 / ID 关键字').fill('orders')
  await expect.poll(() => requests.at(-1)?.searchParams.get('keyword')).toBe('orders')
  await expect(page).toHaveURL(/keyword=orders/)
  expect(requests.at(-1)?.searchParams.get('nodeId')).toBe('synthetic-node')
  await page.getByRole('button', { name: '移除筛选 执行节点：合成节点' }).click()
  await expect.poll(() => requests.at(-1)?.searchParams.has('nodeId')).toBe(false)
  await page.getByRole('link', { name: 'synthetic_db.orders', exact: true }).click()
  await page.goBack()
  await expect(page).toHaveURL(/keyword=orders/)
  await expect(page.getByLabel('任务名称 / ID 关键字')).toHaveValue('orders')
  await page.getByRole('button', { name: '清除筛选' }).click()
  await expect(page).toHaveURL(/\/tasks\?scope=MINE$|\/tasks$/)
})

test('行操作只来自服务端资格；取消经危险确认并提交写请求', async ({ page }) => {
  const writes: string[] = []
  let state = 'RUNNING'
  await page.route('**/api/v1/tasks/filter-options', route => route.fulfill({ json: { requestId: 'r', ...options } }))
  await page.route('**/api/v1/tasks?*', route => route.fulfill({ json: { requestId: 'r', items: [task('running', { state, finishedAt: undefined, availableActions: state === 'RUNNING' ? ['COPY_COMMAND', 'VIEW_LOGS', 'CANCEL'] : ['COPY_COMMAND', 'VIEW_LOGS'] }), task('readonly', { state: 'FAILED', availableActions: ['COPY_COMMAND', 'VIEW_LOGS'] })], nextCursor: null, totalPages: 1 } }))
  await page.route('**/api/v1/tasks/running:cancel', route => { writes.push(route.request().headers()['idempotency-key'] ?? ''); state = 'CANCELLING'; return route.fulfill({ status: 202, json: { requestId: 'r', item: { taskId: 'running', state: 'CANCELLING', cancelRequested: true, replayed: false } } }) })
  await page.goto('/tasks')
  await page.getByRole('button', { name: 'synthetic_db.readonly 的更多操作' }).click()
  await expect(page.getByRole('menuitem', { name: '取消任务' })).toHaveCount(0)
  await expect(page.getByRole('menuitem', { name: '从头重新执行' })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: '查看错误' })).toBeVisible()
  await page.getByRole('button', { name: 'synthetic_db.running 的更多操作' }).click()
  await page.getByRole('menuitem', { name: '取消任务' }).click()
  const dialog = page.getByRole('dialog', { name: '取消任务' })
  await expect(dialog).toContainText('不回滚已写入的文件或数据')
  await expect(dialog).toContainText('合成验证')
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  expect(writes).toEqual([])
  await page.getByRole('button', { name: 'synthetic_db.running 的更多操作' }).click()
  await page.getByRole('menuitem', { name: '取消任务' }).click()
  await page.getByRole('dialog', { name: '取消任务' }).getByRole('button', { name: '确认取消' }).click()
  await expect(page.locator('.ant-message').getByText('已提交取消请求。')).toBeVisible()
  expect(writes).toHaveLength(1)
  expect(writes[0]!.length).toBeGreaterThanOrEqual(16)
  await expect(page.getByText('取消中', { exact: true })).toBeVisible()
})

test('历史名称、已删除对象、核对标识与空态表达', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  let items: unknown[] = [task('deleted', { dataSourceName: undefined, dataSourceDeleted: true, nodeDeleted: true, reconciliationRequired: true, state: 'RUNNING', environment: undefined })]
  await page.route('**/api/v1/tasks/filter-options', route => route.fulfill({ json: { requestId: 'r', ...options } }))
  await page.route('**/api/v1/tasks?*', route => route.fulfill({ json: { requestId: 'r', items, nextCursor: null, totalPages: items.length ? 1 : 0 } }))
  await page.goto('/tasks')
  await expect(page.getByText('synthetic-source（已删除）', { exact: true })).toBeVisible()
  await expect(page.getByText('合成节点（已删除）', { exact: true })).toBeVisible()
  await expect(page.getByText('状态核对中', { exact: true })).toBeVisible()
  await expect(page.getByText('暂无可靠阶段')).toHaveCount(0)
  items = []
  await page.goto('/tasks?state=FAILED')
  await expect(page.getByText('没有符合条件的任务', { exact: true })).toBeVisible()
  await page.goto('/tasks')
  await expect(page.getByText('尚无可展示任务', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '新建任务' }).click()
  await expect(page.getByRole('menuitem', { name: '普通导入', exact: true })).not.toHaveAttribute('aria-disabled', 'true')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menuitem', { name: '普通导入', exact: true })).toHaveCount(0)
  expect(errors).toEqual([])
})

test('普通导入类型可筛选且新建菜单进入独立向导', async ({ page }, info) => {
  const requests: URL[] = []
  await mockFacts(page)
  await mockTasks(page, [task('normal', { type: 'OBLOADER_NORMAL_IMPORT', state: 'RUNNING', finishedAt: undefined, availableActions: ['VIEW_LOGS', 'COPY_COMMAND', 'CANCEL'] })], url => requests.push(url))
  await page.goto('/tasks?type=OBLOADER_NORMAL_IMPORT')
  await expect(page.getByLabel('任务类型', { exact: true })).toContainText('普通导入')
  await expect(page.getByText('类型：普通导入', { exact: true })).toBeVisible()
  await expect.poll(() => requests.at(-1)?.searchParams.get('type')).toBe('OBLOADER_NORMAL_IMPORT')
  await page.getByRole('button', { name: 'synthetic_db.normal 的更多操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '取消任务', exact: true }).click()
  const cancellation = page.getByRole('dialog', { name: '取消任务', exact: true })
  await expect(cancellation).toContainText('OBLOADER')
  await expect(cancellation).not.toContainText('OBDUMPER')
  await cancellation.getByRole('button', { name: '取消', exact: true }).click()
  await expect(cancellation).toHaveCount(0)
  await page.getByRole('button', { name: '新建任务' }).click()
  await expect(page.getByRole('menuitem', { name: '旁路导入', exact: true })).not.toHaveAttribute('aria-disabled', 'true')
  await page.getByRole('menuitem', { name: '普通导入', exact: true }).click()
  await expect(page).toHaveURL(/\/imports\/normal\/new/)
  await expect(page.getByRole('heading', { name: '新建普通导入任务', exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('normal-import-enabled-entry.png'), fullPage: true })
})

test('仅终态任务不自动刷新，三档视口不产生整页横向滚动', async ({ page }, info) => {
  let reads = 0
  await mockTasks(page, [task('done')], () => { reads++ })
  await page.goto('/tasks')
  await expect(page.getByRole('link', { name: 'synthetic_db.done', exact: true })).toBeVisible()
  const initial = reads
  await page.waitForTimeout(11_000)
  expect(reads).toBe(initial)
  for (const [width, height] of [[1920, 1080], [1440, 1024], [1280, 720]]) {
    await page.setViewportSize({ width: width!, height: height! })
    await page.getByRole('button', { name: '展开筛选' }).or(page.getByRole('button', { name: '收起筛选' })).first().isVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${width}`).toBe(true)
    await page.screenshot({ path: info.outputPath(`task-center-${width}.png`), fullPage: true })
  }
})

test('任务写入授权失效后清除旧确认和任务事实，不重放成功', async ({ page }) => {
  await mockFacts(page)
  let denied = false
  await page.route('**/api/v1/tasks?*', route => route.fulfill(denied ? { status: 403, json: { code: 'TASK_SCOPE_DENIED', message: '无权读取', retryable: false } } : { json: { requestId: 'r', items: [task('restricted', { state: 'RUNNING', availableActions: ['CANCEL'] })], totalPages: 1, nextCursor: null } }))
  await page.route('**/api/v1/tasks/restricted:cancel', route => { denied = true; return route.fulfill({ status: 403, json: { code: 'TASK_SCOPE_DENIED', message: '无权操作', retryable: false } }) })
  await page.goto('/tasks'); await page.getByRole('button', { name: 'synthetic_db.restricted 的更多操作' }).click(); await page.getByRole('menuitem', { name: '取消任务', exact: true }).click()
  await page.getByRole('dialog', { name: '取消任务' }).getByRole('button', { name: '确认取消' }).click()
  await expect(page.getByText('当前身份无权查看任务', { exact: true })).toBeVisible()
  await expect(page.getByRole('dialog', { name: '取消任务' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'synthetic_db.restricted', exact: true })).toHaveCount(0)
  await expect(page.locator('.ant-message').getByText('已提交取消请求。')).toHaveCount(0)
})
