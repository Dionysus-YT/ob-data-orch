import { expect, test } from '@playwright/test'

const observedAt = '2026-10-09T00:00:00Z'
const gib = 2 ** 30
const node = {
  id: 'synthetic-online', displayName: '合成在线节点', platform: 'WINDOWS_AMD64', managementState: 'ENABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: observedAt, environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: true, unavailableReasons: [], revision: 1, updatedAt: observedAt,
  agentFacts: { os: 'windows', arch: 'amd64', agentVersion: 'synthetic', bootId: 'synthetic-boot', observedAt, capacityTotal: 1, capacityUsed: 0, cpuUsagePercent: 25, memoryUsagePercent: 50, memoryTotalBytes: 16 * gib, memoryAvailableBytes: 8 * gib },
}

test('预算表单读取和保存真实字节策略，清空恢复自动值', async ({ page }) => {
  let saved: Record<string, unknown> | undefined
  const item = { ...node, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt, acceptsNewTasks: false, managementState: 'DISABLED', unavailableReasons: ['NODE_DISABLED'], budgetPolicy: { systemReservedBytes: 2147483648, taskOverheadBytes: 1073741824 } }
  await page.route('**/api/v1/execution-nodes/synthetic-online', route => {
    if (route.request().method() === 'PATCH') { saved = route.request().postDataJSON(); return route.fulfill({ json: { item: { ...item, revision: 2, budgetPolicy: saved?.budgetPolicy } } }) }
    return route.fulfill({ json: { item } })
  })
  await page.goto('/nodes/synthetic-online/edit')
  const reserved = page.getByRole('spinbutton', { name: '系统与 Agent 预留（GiB）' })
  const overhead = page.getByRole('spinbutton', { name: '每任务堆外预算（GiB）' })
  await expect(reserved).toHaveValue('2.00')
  await expect(overhead).toHaveValue('1.00')
  await reserved.fill('')
  await overhead.fill('1.25')
  await overhead.blur()
  await page.getByRole('button', { name: '保存修改', exact: true }).click()
  await expect.poll(() => saved?.budgetPolicy).toEqual({ systemReservedBytes: 0, taskOverheadBytes: 1342177280 })
  await expect(reserved).toHaveValue('')
  await expect(overhead).toHaveValue('1.25')
})

for (const scope of ['MINE', 'ALL'] as const) {
  test(`服务端 ${scope} 额度与完整节点任务统计实际接入`, async ({ page }) => {
    const item = { ...node, executionQuota: { totalGiB: 20, usedGiB: 10, reservedGiB: 5 }, taskSummary: { scope, running: 3, waiting: 23, reconciling: 1 }, budgetPolicy: {}, defaultJvmMemory: '4G' }
    await page.route('**/api/v1/execution-nodes', route => route.fulfill({ json: { items: [item] } }))
    await page.route('**/api/v1/execution-nodes/*', route => route.fulfill({ json: { item: { ...item, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt } } }))
    await page.goto('/nodes')
    await expect(page.getByRole('columnheader', { name: scope === 'ALL' ? '全部任务' : '我的任务', exact: true })).toBeVisible()
    await expect(page.locator('[aria-label="执行额度占用 75.0%"]')).toBeVisible()
    await expect(page.getByRole('button', { name: '总量 20.0 GiB' })).toBeVisible()
    await expect(page.getByText('运行中 3', { exact: true })).toBeVisible()
    await expect(page.getByText('等待中 23', { exact: true })).toBeVisible()
    await expect(page.getByText('核对中 1', { exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: '查看任务', exact: true })).toHaveAttribute('href', `/tasks?nodeId=synthetic-online&scope=${scope}`)
  })
}

test('节点统一六列表格展示真实资源，未知额度不采用固定任务容量', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  const online = { ...node, agentFacts: { ...node.agentFacts, memoryUsagePercent: 75, memoryAvailableBytes: 4 * gib } }
  const offline = { ...node, id: 'synthetic-offline', displayName: '合成离线节点', heartbeatStatus: 'OFFLINE', acceptsNewTasks: false, unavailableReasons: ['HEARTBEAT_OFFLINE'] }
  await page.route('**/api/v1/execution-nodes', route => route.fulfill({ json: { items: [online, offline] } }))
  await page.route('**/api/v1/execution-nodes/*', route => {
    const summary = route.request().url().endsWith(offline.id) ? offline : online
    return route.fulfill({ json: { item: { ...summary, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt, dataRootUsages: [{ root: '/E:/synthetic', totalBytes: 400 * gib, availableBytes: 20 * gib }] } } })
  })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/nodes')
  await expect(page.getByRole('heading', { name: '执行节点', exact: true })).toBeVisible()
  const region = page.getByRole('region', { name: '当前已加载的授权执行节点' })
  await expect(region.getByRole('columnheader')).toHaveCount(6)
  await expect(region.getByRole('columnheader', { name: '节点状态', exact: true })).toBeVisible()
  await expect(region.getByRole('columnheader', { name: '任务可用性', exact: true })).toHaveCount(0)
  const row = region.getByRole('row').filter({ has: page.getByRole('link', { name: node.displayName }) })
  await expect(row.locator('[aria-label="CPU 25.0%"]')).toBeVisible()
  await expect(row.locator('[aria-label="内存 75.0%"]')).toBeVisible()
  await expect(row.locator('[aria-label="磁盘 95.0%"]')).toBeVisible()
  const colors = await row.locator('.node-resource .ant-progress-bg').evaluateAll(bars => bars.map(bar => getComputedStyle(bar).backgroundColor))
  expect(colors).toHaveLength(3)
  expect(new Set(colors).size).toBe(3)
  const [low, medium, high] = colors.map(color => color.match(/[\d.]+/g)!.map(Number))
  expect(low![2]!).toBeGreaterThan(Math.max(low![0]!, low![1]!))
  expect(medium![0]!).toBeGreaterThan(medium![1]!)
  expect(medium![1]!).toBeGreaterThan(medium![2]! * 1.5)
  expect(medium![1]!).toBeGreaterThan(medium![0]! * 0.7)
  expect(high![0]!).toBeGreaterThan(Math.max(high![1]!, high![2]!) * 2)
  const memory = row.locator('[aria-label="内存 75.0%"]')
  const disk = row.locator('[aria-label="磁盘 95.0%"]')
  await memory.hover()
  await expect(page.getByRole('tooltip').filter({ hasText: '12.0 GiB / 16.0 GiB' })).toHaveText('12.0 GiB / 16.0 GiB')
  await page.screenshot({ path: info.outputPath('node-memory-tooltip.png'), fullPage: true })
  await disk.hover()
  await expect(page.getByRole('tooltip').filter({ hasText: '380.0 GiB / 400.0 GiB' })).toHaveText('380.0 GiB / 400.0 GiB')
  await page.screenshot({ path: info.outputPath('node-disk-tooltip.png'), fullPage: true })
  await page.getByRole('heading', { name: '执行节点', exact: true }).hover()
  await memory.focus()
  await expect(page.getByRole('tooltip').filter({ hasText: '12.0 GiB / 16.0 GiB' })).toBeVisible()
  await memory.blur()
  await expect(row.getByText('采样：', { exact: false })).toHaveCount(0)
  await expect(page.getByText('最新刷新：', { exact: false })).toBeVisible()
  await expect(row.locator('[aria-label="执行额度未知"]')).toBeVisible()
  await expect(row.getByRole('button', { name: '已用 待核对' })).toBeVisible()
  await expect(row.locator('[data-state-category="management"]')).toHaveText('管理已启用')
  await expect(row.locator('[data-state-category="agent"]')).toHaveText('Agent在线')
  await expect(row.locator('[data-state-category="environment"]')).toHaveText('运行环境检查通过')
  await expect(row.locator('[data-state-category="capacity"]')).toHaveText('任务槽位有空位')
  await expect(region.getByText('导出', { exact: true })).toHaveCount(0)
  await expect(region.getByText('普通导入', { exact: true })).toHaveCount(0)
  await expect(region.getByText('旁路导入', { exact: true })).toHaveCount(0)
  await expect(region.getByText('可用', { exact: true })).toHaveCount(0)
  await expect(region.locator('.node-resources.is-stale')).toHaveCount(1)
  await expect(row.getByText('1 / 0', { exact: true })).toHaveCount(0)
  await expect(row.getByRole('link', { name: '查看任务' })).toHaveAttribute('href', '/tasks?nodeId=synthetic-online&scope=MINE')
  for (const width of [1920, 1440, 1280, 640, 320]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${width}px 页面不横滚`).toBe(true)
    await page.screenshot({ path: info.outputPath(`node-resources-${width}.png`), fullPage: true })
  }
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.evaluate(() => { document.body.style.zoom = '2' })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await expect(page.locator('vite-error-overlay')).toHaveCount(0)
  expect(errors).toEqual([])
})

test('资源容量提示保留耗尽零值与离线标记，缺失容量不从使用率推算', async ({ page }) => {
  const full = { ...node, id: 'synthetic-full', displayName: '合成耗尽节点', agentFacts: { ...node.agentFacts, memoryUsagePercent: 100, memoryAvailableBytes: 0 } }
  const unknown = { ...node, id: 'synthetic-unknown', displayName: '合成未知节点', heartbeatStatus: 'OFFLINE', acceptsNewTasks: false, unavailableReasons: ['HEARTBEAT_OFFLINE'], agentFacts: { ...node.agentFacts, memoryTotalBytes: undefined, memoryAvailableBytes: undefined } }
  await page.route('**/api/v1/execution-nodes', route => route.fulfill({ json: { items: [full, unknown] } }))
  await page.route('**/api/v1/execution-nodes/*', route => {
    const summary = route.request().url().endsWith(full.id) ? full : unknown
    return route.fulfill({ json: { item: { ...summary, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt, dataRootUsages: summary.id === full.id ? [{ root: '/E:/synthetic', totalBytes: 1024, availableBytes: 0 }] : [] } } })
  })
  await page.goto('/nodes')
  const fullRow = page.getByRole('row').filter({ has: page.getByRole('link', { name: full.displayName, exact: true }) })
  await fullRow.locator('[aria-label="内存 100.0%"]') .hover()
  await expect(page.getByRole('tooltip').filter({ hasText: '16.0 GiB / 16.0 GiB' })).toBeVisible()
  await fullRow.locator('[aria-label="磁盘 100.0%"]') .hover()
  await expect(page.getByRole('tooltip').filter({ hasText: '1.0 KiB / 1.0 KiB' })).toBeVisible()
  const unknownRow = page.getByRole('row').filter({ has: page.getByRole('link', { name: unknown.displayName, exact: true }) })
  await unknownRow.locator('[aria-label="内存 50.0%"]') .hover()
  const memoryTooltip = page.getByRole('tooltip').filter({ hasText: '容量尚未采集' })
  await expect(memoryTooltip).toBeVisible()
  await expect(memoryTooltip).not.toContainText('使用率')
  await expect(memoryTooltip).toContainText('离线前快照')
  await page.getByRole('heading', { name: '执行节点', exact: true }).hover()
  await expect(page.getByRole('tooltip')).toHaveCount(0)
  await unknownRow.locator('[aria-label="磁盘 尚未采集"]') .hover()
  await expect(page.getByRole('tooltip').filter({ hasText: '容量尚未采集' })).toHaveText('容量尚未采集离线前快照')
})

test('手动刷新展示忙态与完成反馈，快速成功仍可感知，失败保留上次快照', async ({ page }, info) => {
  let response: 'ok' | 'hold' | 'error' = 'ok'
  let reads = 0
  let complete!: () => void
  const pending = new Promise<void>(resolve => { complete = resolve })
  await page.route('**/api/v1/execution-nodes', async route => {
    reads++
    if (response === 'hold') await pending
    if (response === 'error') return route.fulfill({ status: 503, json: { code: 'TEMPORARY', message: '合成读取失败' } })
    return route.fulfill({ json: { items: [node] } })
  })
  await page.route('**/api/v1/execution-nodes/*', route => route.fulfill({ json: { item: { ...node, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt } } }))
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/nodes')
  const refresh = page.getByRole('button', { name: '刷新执行节点', exact: true })
  await expect(page.getByRole('link', { name: node.displayName, exact: true })).toBeVisible()
  await expect(refresh).toHaveText('刷新')
  response = 'hold'
  await refresh.click()
  await expect(refresh).toHaveText('刷新中')
  await expect(refresh).toHaveAttribute('aria-busy', 'true')
  await expect(refresh).toBeDisabled()
  await expect(page.locator('.orch-operational-table .ant-spin-spinning')).toBeVisible()
  await expect(page.getByRole('link', { name: node.displayName, exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('node-refresh-pending.png'), fullPage: true })
  complete()
  await expect(refresh).toHaveText('已刷新')
  await expect(refresh.locator('[role="img"]')).toHaveAttribute('aria-label', 'check-circle')
  await expect(refresh).toBeEnabled()
  await expect(page.locator('.node-refresh-completed')).toHaveText('已刷新')
  response = 'ok'
  await refresh.click()
  await expect.poll(() => reads).toBe(3)
  await expect(refresh).toHaveText('已刷新')
  const successTime = await page.getByText('最新刷新：', { exact: false }).innerText()
  await page.screenshot({ path: info.outputPath('node-refresh-completed.png'), fullPage: true })
  response = 'error'
  await refresh.click()
  await expect(page.getByText('刷新未完成', { exact: true })).toBeVisible()
  await expect(refresh).toHaveText('刷新')
  await expect(refresh).toHaveAttribute('aria-busy', 'false')
  await expect(page.locator('.node-refresh-completed')).toHaveCount(0)
  await expect(page.getByText('最新刷新：', { exact: false })).toHaveText(successTime)
  await expect(page.getByRole('link', { name: node.displayName, exact: true })).toBeVisible()
  expect(reads).toBe(4)
})

test('额度分段悬停及键盘提示，零值、全占用和未知状态保持可解释', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  await page.goto('/tests/fixtures/node-resources.html')
  await expect(page.getByRole('heading', { name: '节点额度合成验证' })).toBeVisible()
  await expect(page.locator('[aria-label="执行额度占用 75.0%"]')).toBeVisible()
  await expect(page.getByText('总体占用', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '总量 20.0 GiB' }).locator('strong')).toHaveText('20.0GiB')
  for (const [key, text] of [['used', '已用 10.0 GiB · 50.0%'], ['reserved', '预留 5.0 GiB · 25.0%'], ['free', '可用 5.0 GiB · 25.0%']]) {
    // 使用分段内的几何点，避免矩形边界的中心落在圆环内部。
    const position = key === 'used' ? { x: 79, y: 44 } : key === 'reserved' ? { x: 20, y: 70 } : { x: 20, y: 18 }
    await page.locator('.node-quota-ring').hover({ position })
    await expect(page.getByRole('tooltip').filter({ hasText: text })).toBeVisible()
    await page.getByRole('heading').hover()
    await expect(page.getByRole('tooltip')).toHaveCount(0)
  }
  await page.getByRole('button', { name: '预留 5.0 GiB' }).focus()
  await expect(page.getByRole('tooltip').filter({ hasText: '预留 5.0 GiB' })).toBeVisible()
  await page.getByRole('button', { name: '空额度', exact: true }).click()
  await expect(page.locator('[aria-label="执行额度占用 0.0%"]')).toBeVisible()
  await page.getByRole('button', { name: '已用 0.0 GiB' }).focus()
  await expect(page.getByRole('tooltip').filter({ hasText: '已用 0.0 GiB · 0.0%' })).toBeVisible()
  await page.getByRole('button', { name: '满额度', exact: true }).click()
  await expect(page.locator('[data-segment="used"]')).toBeVisible()
  await expect(page.locator('[aria-label="执行额度占用 100.0%"]')).toBeVisible()
  await page.getByRole('button', { name: '未知额度', exact: true }).click()
  await expect(page.locator('[aria-label="执行额度未知"]')).toBeVisible()
  await expect(page.locator('[data-segment]')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('quota-unknown.png') })
  await expect(page.locator('vite-error-overlay')).toHaveCount(0)
  expect(errors).toEqual([])
})

test('自动刷新保留抽屉会话的间隔，关闭和真正离开列表时正确停止', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  let reads = 0
  await page.clock.install({ time: new Date('2026-10-10T03:00:00Z') })
  await page.route('**/api/v1/execution-nodes', route => { reads++; return route.fulfill({ json: { items: [node] } }) })
  await page.route('**/api/v1/execution-nodes/*', route => route.fulfill({ json: { item: { ...node, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt } } }))
  await page.goto('/nodes')
  await expect(page.getByRole('link', { name: node.displayName, exact: true })).toBeVisible()
  await expect(page.locator('.node-auto-refresh')).toHaveText('自动刷新60 秒')
  const previousTime = await page.getByText('最新刷新：', { exact: false }).innerText()
  await page.clock.fastForward(60000)
  await expect.poll(() => reads).toBe(2)
  await expect(page.getByText('最新刷新：', { exact: false })).not.toHaveText(previousTime)
  await page.locator('.node-auto-refresh .ant-select-selector').click()
  await page.locator('.ant-select-dropdown:visible').getByText('30 秒', { exact: true }).click()
  await page.clock.fastForward(30000); await expect.poll(() => reads).toBe(3)
  await page.locator('.node-auto-refresh .ant-select-selector').click()
  await page.locator('.ant-select-dropdown:visible').getByText('关闭', { exact: true }).click()
  await page.clock.fastForward(300000); expect(reads).toBe(3)
  await page.locator('.node-auto-refresh .ant-select-selector').click()
  await page.locator('.ant-select-dropdown:visible').getByText('15 秒', { exact: true }).click()
  await page.clock.fastForward(15000); await expect.poll(() => reads).toBe(4)
  await page.getByRole('button', { name: `${node.displayName} 的操作`, exact: true }).click()
  await page.getByRole('menuitem', { name: '编辑', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes\/synthetic-online\/edit$/)
  // 打开抽屉仍属于原列表会话，周期刷新继续；进入详情才卸载列表并取消计时器。
  await page.clock.fastForward(300000); await expect.poll(() => reads).toBe(5)
  await page.getByRole('dialog', { name: '编辑执行节点', exact: true }).getByRole('button', { name: '取消', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes$/)
  await expect(page.locator('.node-auto-refresh')).toHaveText('自动刷新15 秒')
  await page.getByRole('link', { name: node.displayName, exact: true }).click()
  await expect(page).toHaveURL(/\/nodes\/synthetic-online$/)
  await page.clock.fastForward(300000); expect(reads).toBe(5)
  expect(errors).toEqual([])
})

test('三点菜单选择编辑和永久删除，取消不发送请求，运行拒绝保留节点，成功后移除配置', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  let deleted = false; let attempts = 0
  const item = { ...node, executionQuota: { totalGiB: 20, usedGiB: 10, reservedGiB: 5 }, taskSummary: { scope: 'ALL', running: 1, waiting: 0, reconciling: 0 } }
  await page.route('**/api/v1/execution-nodes', route => route.fulfill({ json: { items: deleted ? [] : [item] } }))
  await page.route('**/api/v1/execution-nodes/*', route => route.fulfill({ json: { item: { ...item, toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', allowedRoots: ['/E:/synthetic'], createdAt: observedAt } } }))
  await page.route('**/api/v1/execution-nodes/synthetic-online:delete', route => {
    attempts++
    expect(route.request().method()).toBe('POST')
    expect(route.request().headers()['if-match']).toBe('"rev-1"')
    if (attempts === 1) return route.fulfill({ status: 409, json: { code: 'EXECUTION_NODE_RUNNING_TASK', message: '节点仍有运行或待核对执行，暂不能永久删除' } })
    deleted = true
    return route.fulfill({ json: { id: node.id, outcome: 'DELETED', revision: 0, agentAccessRevoked: true } })
  })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/nodes')
  const actions = page.getByRole('button', { name: `${node.displayName} 的操作`, exact: true })
  await expect(actions).toBeVisible()
  await expect(actions).toHaveAttribute('aria-haspopup', 'menu')
  await expect(page.getByRole('button', { name: `编辑 ${node.displayName}` })).toHaveCount(0)
  await expect(page.getByText('总体占用', { exact: true })).toHaveCount(0)
  await expect(page.locator('.node-quota-values strong .node-quota-unit')).toHaveCount(3)
  await expect(page.getByRole('columnheader', { name: '执行额度', exact: true })).toHaveCSS('text-align', 'center')
  await page.screenshot({ path: info.outputPath('node-list-clean.png'), fullPage: true })
  await actions.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('menuitem', { name: '编辑', exact: true })).toBeVisible()
  await page.getByRole('menuitem', { name: '删除', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '永久删除执行节点' })
  await expect(dialog.getByText('历史任务及执行证据保留。', { exact: false })).toBeVisible()
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  expect(attempts).toBe(0)
  await actions.click()
  await page.getByRole('menuitem', { name: '删除', exact: true }).click()
  await dialog.getByRole('button', { name: '永久删除', exact: true }).click()
  await expect(page.getByText('操作未完成', { exact: true })).toBeVisible()
  await expect(page.getByText('节点仍有运行或待核对的执行，暂不能永久删除。', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: node.displayName, exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('node-list-adjusted.png'), fullPage: true })
  await actions.click()
  await page.getByRole('menuitem', { name: '删除', exact: true }).click()
  await dialog.getByRole('button', { name: '永久删除', exact: true }).click()
  await expect(page.getByText('尚无已登记执行节点')).toBeVisible()
  await expect(page.getByText('最新刷新：', { exact: false })).toBeVisible()
  await expect(page.getByText('已永久删除；Agent 身份和未使用注册码已撤销。', { exact: false })).toBeVisible()
  expect(attempts).toBe(2); expect(errors).toEqual([])
})
