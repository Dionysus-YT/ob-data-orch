import { chromium, expect, test, type Page } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import { writeFile } from 'node:fs/promises'
import { overviewAnalysisTask, overviewFixture } from './fixtures/overview'
import type { OverviewRange, OverviewSnapshot } from '../src/api/overview'
import { monitorNativeWindowErrors } from './fixtures/native-window-errors'

const verifyNative = new WeakMap<Page, () => Promise<void>>()
test.beforeEach(async ({ page }) => { verifyNative.set(page, await monitorNativeWindowErrors(page)) })
test.afterEach(async ({ page }) => { await verifyNative.get(page)?.() })

type Mode = 'ready' | 'empty' | 'partial' | 'error' | 'denied' | 'invalid' | 'stale' | 'long' | 'task-restricted'
async function fixture(page: Page, initial: Mode = 'ready') {
  let mode = initial, calls = 0, taskMode: 'ready' | 'denied' | 'error' = 'ready'
  const requests: string[] = []
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url()), path = url.pathname
    requests.push(`${route.request().method()} ${path}${url.search}`)
    if (path === '/api/v1/overview') {
      calls++
      if (mode === 'error' || mode === 'denied') return route.fulfill({ status: mode === 'error' ? 503 : 403, json: { error: { code: mode === 'error' ? 'OVERVIEW_UNAVAILABLE' : 'OVERVIEW_SCOPE_DENIED', message: 'synthetic-only', retryable: mode === 'error' } } })
      const range = url.searchParams.get('range') as OverviewRange
      let item: OverviewSnapshot = overviewFixture(mode === 'stale' ? new Date(Date.now() - 100000) : new Date(), range)
      if (mode === 'empty') {
        item = { ...item, summary: '当前授权范围内暂无任务事实；执行节点范围不可见。', metrics: { status: 'READY', data: { running: 0, waiting: 0, todaySucceeded: 0, todayFailed: 0 } },
          trend: { status: 'READY', data: item.trend.status === 'READY' ? item.trend.data.map(bucket => ({ ...bucket, total: 0, succeeded: 0, failed: 0 })) : [] }, nodes: { status: 'RESTRICTED' },
          myTasks: { status: 'READY', data: [] }, attention: { status: 'READY', data: { from: new Date(Date.now() - 86400000).toISOString(), to: item.snapshotAt, coverage: 'TASK_FACTS_ONLY', items: [] } } }
      }
      if (mode === 'partial') item = { ...item, summary: '快照数据不完整，任务趋势和异常事实暂不可用。', trend: { status: 'UNAVAILABLE' }, attention: { status: 'UNAVAILABLE' } }
      if (mode === 'task-restricted') item = { ...item, summary: '当前任务范围不可见，仅展示已授权的节点事实。', metrics: { status: 'RESTRICTED' }, trend: { status: 'RESTRICTED' }, myTasks: { status: 'RESTRICTED' }, attention: { status: 'RESTRICTED' } }
      if (mode === 'long' && item.myTasks.status === 'READY' && item.nodes.status === 'READY') {
        item = { ...item, myTasks: { status: 'READY', data: item.myTasks.data.map(task => ({ ...task, displayName: '合成超长中文技术任务名称'.repeat(7), id: `synthetic-${'x'.repeat(100)}-${task.id}` })) },
          nodes: { ...item.nodes, data: { ...item.nodes.data, items: item.nodes.data.items.map(node => ({ ...node, displayName: '合成超长执行节点名称'.repeat(7) })) } } }
      }
      return route.fulfill({ json: { requestId: 'synthetic-overview-request', item: mode === 'invalid' ? { ...item, secret: 'synthetic-invalid-field' } : item } })
    }
    if (path === '/api/v1/tasks/filter-options') return route.fulfill({ json: { dataSources: [], nodes: [], creators: [], environments: [], creatorFilterAvailable: false } })
    if (path === '/api/v1/tasks') {
      if (taskMode !== 'ready') return route.fulfill({ status: taskMode === 'denied' ? 403 : 503, json: { error: { code: 'SYNTHETIC_LIST_UNAVAILABLE', message: 'synthetic-only' } } })
      if (!url.searchParams.get('finishedFrom')) return route.fulfill({ json: { items: [], totalPages: 0 } })
      const cursor = url.searchParams.get('cursor'), state = url.searchParams.get('state') || 'SUCCEEDED'
      return route.fulfill({ json: { items: [overviewAnalysisTask(cursor ? 'second-page' : 'first-page', state, url.searchParams.get('finishedFrom')!)], totalPages: 2, ...(cursor ? {} : { nextCursor: 'synthetic-next' }) } })
    }
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [] } })
    return route.fulfill({ json: { items: [] } })
  })
  return { setMode: (next: Mode) => { mode = next }, setTaskMode: (next: typeof taskMode) => { taskMode = next }, get calls() { return calls }, requests }
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`首页 ${viewport.width}×${viewport.height} 显示授权事实并完成范围、系列和刷新交互`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.setViewportSize(viewport)
    const model = await fixture(page)
    await page.goto('/')
    await expect(page).toHaveTitle(/OB Data Orch/)
    await expect(page.getByRole('heading', { name: '首页', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '今日失败任务 2 个，查看任务' })).toBeVisible()
    await expect(page.getByText('100%', { exact: true })).toBeVisible()
    await expect(page.getByRole('region', { name: '我的任务', exact: true }).getByRole('table').getByText('运行中', { exact: true })).toBeVisible()
    const nodeTable = page.getByRole('table', { name: '执行节点摘要', exact: true })
    await expect(nodeTable).toBeVisible()
    await expect(page.getByRole('table', { name: '我的重点任务', exact: true })).toBeVisible()
    expect(await nodeTable.evaluate(table => { const viewport = table.closest('.ant-table-content')!; return viewport.scrollWidth <= viewport.clientWidth }), '节点摘要的完整事实无需横向滚动').toBe(true)
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await expect(page.getByRole('status', { name: '快照范围与时间' }).getByText('UTC', { exact: true })).toBeVisible()
    await expect(page.getByRole('checkbox', { name: '终态总数' })).toBeChecked()
    await expect(page.locator('.overview-trend-chart circle')).toHaveCount(0)
    await page.locator('.overview-trend-hit').first().focus()
    const point = await page.locator('.overview-trend-chart circle').first().boundingBox()
    expect(Math.abs(point!.width - point!.height), '响应式图形保留圆形数据点比例').toBeLessThan(0.5)
    await page.getByRole('checkbox', { name: '成功', exact: true }).uncheck()
    await expect(page.locator('svg g.trend-succeeded')).toHaveCount(0)
    await page.getByRole('checkbox', { name: '成功', exact: true }).check()
    await page.getByText('最近 7 天', { exact: true }).click()
    await expect.poll(() => model.requests.some(request => request.includes('/api/v1/overview?range=7d'))).toBe(true)
    await expect(page.locator('.overview-trend-chart')).toHaveAttribute('data-bucket-count', '7')
    await expect(page.locator('.overview-trend-hit')).toHaveCount(7)
    await page.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(page.getByRole('button', { name: '已刷新', exact: true })).toBeVisible()
    expect(model.requests.every(request => request.startsWith('GET '))).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    // 先记录真实首屏；全页截图不能证明固定外壳中的首屏布局。
    await page.getByRole('heading', { name: '首页', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: info.outputPath(`overview-${viewport.width}.png`), fullPage: false })
    await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
    expect(errors).toEqual([])
  })
}

test('首页任务范围受限与读取失败分开表达，不显示可信零值', async ({ page }, info) => {
  await fixture(page, 'task-restricted'); await page.goto('/')
  await expect(page.getByText('当前没有可查看的本人任务，请核对基础角色与授权范围。')).toBeVisible()
  await expect(page.getByText('当前没有可查看的任务趋势，请核对任务授权范围。')).toBeVisible()
  await expect(page.getByText('当前没有可查看的异常事实，请核对任务授权范围。')).toBeVisible()
  await expect(page.getByRole('button', { name: /运行中任务 0 个/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '重试趋势', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '重试指标', exact: true })).toHaveCount(0)
  await expect(page.getByText('当前没有可查看的任务指标，请核对任务授权范围。')).toBeVisible()
  await page.screenshot({ path: info.outputPath('overview-task-restricted.png'), fullPage: true })
})

test('OCP 参考结构：四张指标卡、趋势与关注并列、通栏明细保留同一快照', async ({ page }) => {
  await fixture(page)
  await page.goto('/')
  const headings = await page.locator('.overview-workspace h2').allTextContents()
  expect(headings).toEqual(['运行状态', '任务结果趋势', '需要关注', '我的任务', '执行节点'])
  await expect(page.locator('.overview-metric-card')).toHaveCount(4)
  const trend = await page.getByRole('region', { name: '任务结果趋势', exact: true }).boundingBox()
  const attentionBox = await page.getByRole('region', { name: '需要关注', exact: true }).boundingBox()
  expect(trend!.y).toBe(attentionBox!.y)
  expect(attentionBox!.x).toBeGreaterThan(trend!.x + trend!.width)
  const tasks = await page.getByRole('region', { name: '我的任务', exact: true }).boundingBox()
  const nodes = await page.getByRole('region', { name: '执行节点', exact: true }).boundingBox()
  expect(tasks!.width).toBe(nodes!.width)
  expect(nodes!.y).toBeGreaterThan(tasks!.y)
  const attention = page.getByRole('region', { name: '需要关注', exact: true })
  await expect(attention.getByText('当前授权范围内今日失败 2 个', { exact: false })).toBeVisible()
  await expect(attention.getByText('Agent 当前离线', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '执行节点', exact: true }).getByRole('table')).toBeVisible()
  await page.getByRole('button', { name: '不可接收节点 1 个，可见 2 个，查看执行节点' }).click()
  await expect(page).toHaveURL(/\/nodes$/)
})

test('节点范围受限时节点区不输出计数，任务卡仍保留可信零值', async ({ page }) => {
  await fixture(page, 'empty')
  await page.goto('/')
  await expect(page.getByRole('region', { name: '执行节点', exact: true }).getByText('范围受限', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '运行中任务 0 个，查看任务' })).toBeVisible()
  await expect(page.getByRole('button', { name: /不可接收节点/ })).toHaveCount(0)
})

test('指标按精确状态和 UTC 终态时间下钻，条件可见且可移除', async ({ page }) => {
  const model = await fixture(page)
  await page.goto('/')
  await page.getByRole('button', { name: '今日失败任务 2 个，查看任务' }).click()
  await expect(page).toHaveURL(/state=FAILED/)
  await expect(page).toHaveURL(/finishedFrom=/)
  await expect(page.getByText(/终态时间（UTC，含起点、不含终点）/)).toBeVisible()
  await expect.poll(() => model.requests.some(value => value.includes('/api/v1/tasks?') && value.includes('finishedFrom=') && value.includes('finishedTo='))).toBe(true)
  await page.getByRole('button', { name: /移除筛选 终态时间/ }).click()
  await expect(page).not.toHaveURL(/finishedFrom=/)
})

test('趋势绘图与 Ant 命中区分离，键盘在独立弹窗按同一桶和状态下钻', async ({ page }, info) => {
  const model = await fixture(page)
  await page.goto('/')
  const chart = page.locator('.overview-trend-chart')
  await expect(chart).toHaveAttribute('role', 'img')
  await expect(chart.locator('a, [tabindex], button')).toHaveCount(0)
  const point = page.locator('.overview-trend-hit').first()
  const label = await point.getAttribute('aria-label')
  await point.focus()
  await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog', { name: '任务结果分析', exact: true })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('radiogroup', { name: '分析任务状态' }).getByText('失败', { exact: true }).click()
  const from = label!.slice(3, 22).replace(' ', 'T')
  await expect.poll(() => model.requests.some(request => { const url = new URL(request.split(' ')[1]!, 'http://synthetic.invalid'); return url.pathname === '/api/v1/tasks' && url.searchParams.get('state') === 'FAILED' && (url.searchParams.get('finishedFrom') ?? '').startsWith(from) })).toBe(true)
  await expect(dialog.getByRole('table', { name: '趋势下钻任务列表' })).toBeVisible()
  const link = dialog.getByRole('link', { name: /任务详情（新标签页）/ })
  await expect(link).toHaveAttribute('target', '_blank')
  await expect(link).toHaveAttribute('href', '/tasks/first-page')
  await dialog.getByRole('button', { name: '下一页分析任务' }).click()
  await expect(dialog.getByRole('status', { name: '分析任务分页' })).toHaveText('第 2 页 / 2 页')
  await expect(dialog.getByRole('link', { name: /second-page/ })).toBeVisible()
  await page.screenshot({ path: info.outputPath('overview-analysis-dialog.png'), fullPage: false })
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(point).toBeFocused()
  await expect(page).toHaveURL(/\/$/)
})

test('趋势范围分析弹窗带入完整终态窗口且不附加状态', async ({ page }) => {
  const model = await fixture(page)
  await page.goto('/')
  await page.getByRole('button', { name: '下钻分析', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '任务结果分析' })).toBeVisible()
  await expect.poll(() => model.requests.some(request => request.includes('/api/v1/tasks?') && request.includes('finishedFrom=') && request.includes('finishedTo=') && !request.includes('state='))).toBe(true)
  await expect(page).toHaveURL(/\/$/)
})

test('趋势放大是独立弹窗，图例同步、焦点被约束且关闭返回入口', async ({ page }, info) => {
  await fixture(page); await page.setViewportSize({ width: 1440, height: 1024 }); await page.goto('/')
  const trigger = page.getByRole('button', { name: '放大任务结果趋势', exact: true })
  await trigger.focus(); await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog', { name: '任务结果趋势 · 放大', exact: true })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('heading', { name: '任务结果趋势 · 放大' })).toBeFocused()
  await dialog.getByRole('checkbox', { name: '成功', exact: true }).uncheck()
  await expect(dialog.locator('g.trend-succeeded')).toHaveCount(0)
  await dialog.getByRole('checkbox', { name: '成功', exact: true }).check()
  for (let index = 0; index < 32; index++) { await page.keyboard.press('Tab'); expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true) }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('overview-expanded-dialog.png'), fullPage: false })
  await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await expect(trigger).toBeFocused()
  await expect(page.locator('.overview-trend-detail')).toHaveCount(0)
})

test('分析弹窗关闭重开后迟到任务不覆盖新会话', async ({ page }) => {
  await fixture(page)
  let release!: () => void, reads = 0
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/tasks?*', async route => {
    reads++
    if (reads === 1) { await pending; return route.fulfill({ json: { items: [overviewAnalysisTask('late-old')], totalPages: 1 } }) }
    return route.fulfill({ json: { items: [overviewAnalysisTask('latest-visible')], totalPages: 1 } })
  })
  await page.goto('/'); await page.getByRole('button', { name: '下钻分析', exact: true }).click()
  await expect(page.getByLabel('正在读取分析任务')).toBeVisible()
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.getByRole('button', { name: '下钻分析', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '任务结果分析', exact: true })
  await expect(dialog.getByRole('link', { name: /latest-visible/ })).toBeVisible()
  release()
  await expect(dialog.getByText(/late-old/)).toHaveCount(0)
  await expect(dialog.getByRole('link', { name: /latest-visible/ })).toBeVisible()
})

test('趋势弹窗快速开关与放大转分析保持单一窗口，时间筛选重置分页', async ({ page }) => {
  const model = await fixture(page), errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.goto('/')
  const trigger = page.getByRole('button', { name: '放大任务结果趋势', exact: true })
  for (let index = 0; index < 3; index++) {
    await trigger.focus(); await page.keyboard.press('Enter')
    await expect(page.getByRole('heading', { name: '任务结果趋势 · 放大' })).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(trigger).toBeFocused()
  }
  await trigger.click()
  const expanded = page.getByRole('dialog', { name: '任务结果趋势 · 放大', exact: true })
  const point = expanded.locator('.overview-trend-hit').nth(1)
  const label = await point.getAttribute('aria-label')
  await point.focus(); await page.keyboard.press('Enter')
  const analysis = page.getByRole('dialog', { name: '任务结果分析', exact: true })
  await expect(page.getByRole('dialog')).toHaveCount(1)
  await expect(analysis.getByRole('heading', { name: '任务结果分析' })).toBeFocused()
  await analysis.getByRole('radiogroup', { name: '分析任务状态' }).getByText('成功', { exact: true }).click()
  await expect(analysis.getByRole('table')).toBeVisible()
  const from = label!.slice(3, 22).replace(' ', 'T')
  await expect.poll(() => model.requests.some(request => { const url = new URL(request.split(' ')[1]!, 'http://synthetic.invalid'); return url.searchParams.get('state') === 'SUCCEEDED' && (url.searchParams.get('finishedFrom') ?? '').startsWith(from) })).toBe(true)
  await analysis.getByRole('button', { name: '下一页分析任务' }).click()
  await expect(analysis.getByRole('status', { name: '分析任务分页' })).toHaveText('第 2 页 / 2 页')
  await analysis.locator('.overview-drilldown-window .ant-select-selector').click()
  await page.getByTitle('全部时间段', { exact: true }).click()
  await expect(analysis.getByRole('status', { name: '分析任务分页' })).toHaveText('第 1 页 / 2 页')
  await expect(analysis.getByRole('link', { name: /first-page/ })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('table', { name: '趋势下钻任务列表', includeHidden: true })).toHaveCount(0)
  await expect(trigger).toBeFocused()
  expect(errors).toEqual([])
})

test('分析刷新失败保留可信行，授权失效同步清理全页快照与弹窗', async ({ page }) => {
  const model = await fixture(page); await page.goto('/')
  await page.getByRole('button', { name: '下钻分析', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '任务结果分析', exact: true })
  await expect(dialog.getByRole('table', { name: '趋势下钻任务列表' })).toBeVisible()
  model.setTaskMode('error'); await dialog.getByRole('button', { name: '刷新列表', exact: true }).click()
  await expect(dialog.getByRole('alert')).toBeVisible()
  await expect(dialog.getByRole('table', { name: '趋势下钻任务列表' })).toBeVisible()
  model.setTaskMode('denied'); await dialog.getByRole('button', { name: '重试任务列表', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: /运行中任务 2 个/ })).toHaveCount(0)
  await expect(page.getByText(/合成分析任务/)).toHaveCount(0)
  await expect(page.getByRole('button', { name: '刷新权限' })).toBeVisible()
})

test('分析可信空结果保留窗口和筛选，不挂空表或误报失败', async ({ page }, info) => {
  await fixture(page)
  await page.route('**/api/v1/tasks?*', route => route.fulfill({ json: { items: [], totalPages: 0 } }))
  await page.goto('/'); await page.getByRole('button', { name: '下钻分析', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '任务结果分析', exact: true })
  await expect(dialog.getByText('该时段与状态下暂无可查看的任务')).toBeVisible()
  await expect(dialog.getByRole('table')).toHaveCount(0)
  await expect(dialog.getByRole('alert')).toHaveCount(0)
  await expect(dialog.getByLabel('分析时间段')).toBeVisible()
  await page.screenshot({ path: info.outputPath('overview-analysis-empty.png'), fullPage: false })
})

test('非法终态窗口在原条件处说明且不发送任务读取', async ({ page }) => {
  const model = await fixture(page)
  await page.goto('/tasks?finishedFrom=2026-10-23T01:00:00Z&finishedTo=2026-10-23T01:00:00Z')
  await expect(page.getByText('终态时间的起点必须早于终点。', { exact: true })).toBeVisible()
  expect(model.requests.some(value => value.includes('/api/v1/tasks?'))).toBe(false)
  await page.getByRole('button', { name: /移除筛选 终态时间/ }).click()
  await expect(page).not.toHaveURL(/finishedFrom=/)
  await expect.poll(() => model.requests.some(value => value.includes('/api/v1/tasks?'))).toBe(true)
})

for (const mode of ['empty', 'partial', 'invalid', 'stale', 'denied'] as const) {
  test(`首页 ${mode} 状态不展示伪造事实`, async ({ page }, info) => {
    await fixture(page, mode)
    await page.goto('/')
    if (mode === 'empty') {
      await expect(page.getByRole('button', { name: '运行中任务 0 个，查看任务' })).toBeVisible()
      await expect(page.getByText('该时间范围内暂无终态任务')).toBeVisible()
      await expect(page.locator('.overview-trend-chart')).toHaveCount(0)
      await expect(page.getByText('当前没有可查看的执行节点，请核对节点对象授权。')).toBeVisible()
      await page.getByText('最近 7 天', { exact: true }).click()
      await expect(page.getByText('该时间范围内暂无终态任务')).toBeVisible()
    } else if (mode === 'partial') {
      await expect(page.getByText('快照数据不完整', { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
      await expect(page.getByRole('button', { name: '重试趋势', exact: true })).toBeVisible()
      await expect(page.getByText('最近异常事实暂不可用，不能据此认定没有异常。')).toBeVisible()
    } else if (mode === 'invalid') {
      await expect(page.getByText('首页快照无法校验，请核对网页与控制面版本后重试。')).toBeVisible()
      await expect(page.getByRole('heading', { name: '需要关注' })).toHaveCount(0)
      await expect(page.getByText('synthetic-invalid-field')).toHaveCount(0)
    } else if (mode === 'stale') {
      await expect(page.getByText('数据已过期', { exact: true })).toBeVisible()
      await expect(page.getByText('快照已过期，以下为最后可信记录。')).toBeVisible()
    } else {
      await expect(page.getByRole('button', { name: '刷新权限' })).toBeVisible()
      await expect(page.getByRole('heading', { name: '需要关注' })).toHaveCount(0)
      await expect(page.getByText('合成正常执行节点')).toHaveCount(0)
    }
    await page.screenshot({ path: info.outputPath(`overview-${mode}.png`), fullPage: true })
  })
}

test('刷新失败保留原快照，随后权限失效清除所有旧事实', async ({ page }) => {
  const model = await fixture(page)
  await page.goto('/')
  await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
  model.setMode('error')
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByText(/保留最后可信快照/)).toBeVisible()
  await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
  model.setMode('denied')
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.getByRole('heading', { name: '需要关注' })).toHaveCount(0)
  await expect(page.getByText('合成离线执行节点', { exact: true })).toHaveCount(0)
})

test('长中文与长标识在窄屏不撑破页面', async ({ page }, info) => {
  await page.setViewportSize({ width: 1280, height: 720 })
  await fixture(page, 'long')
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '我的任务' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('overview-long-1280.png'), fullPage: true })
})

test('范围读取期间保留整份旧快照并说明旧范围，重复刷新不创建并发请求', async ({ page }) => {
  await fixture(page)
  await page.goto('/')
  await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  let calls = 0
  await page.route('**/api/v1/overview?range=7d', async route => { calls++; await pending; await route.fulfill({ json: { item: overviewFixture(new Date(), '7d') } }) })
  await page.getByText('最近 7 天', { exact: true }).click()
  await expect(page.getByText('所选时间范围尚未取得，当前仍显示最近 24 小时的完整快照。')).toBeVisible()
  await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
  await expect(page.getByRole('button', { name: /刷新中/ })).toBeDisabled()
  expect(calls).toBe(1)
  release()
  await expect(page.locator('.overview-trend-chart')).toHaveAttribute('data-bucket-count', '7')
  await expect(page.getByText(/所选时间范围尚未取得/)).toHaveCount(0)
})

test('首页原生 200% 缩放下刷新和趋势键盘操作可达', async ({ browserName }, info) => {
  test.setTimeout(240000)
  expect(browserName).toBe('chromium')
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext('', {
    channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined,
    args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage()
    const verifyWindowErrors = await monitorNativeWindowErrors(page)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await fixture(page)
    // 跟随当前项目的隔离端口，避免与其他会话占用的默认端口耦合。
    const origin = new URL(String(info.project.use.baseURL ?? 'http://127.0.0.1:15174')).host
    await page.goto(`http://${origin}/`)
    await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async host => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(options: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const target = (await api.tabs.query({})).find(tab => tab.url.includes(host))
      if (!target) throw new Error('隔离测试标签不存在')
      await api.tabs.setZoom(target.id, 2)
    }, origin)
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const refresh = page.getByRole('button', { name: '刷新', exact: true })
    await refresh.focus()
    await page.keyboard.press('Enter')
    await expect(page.getByRole('button', { name: '已刷新', exact: true })).toBeInViewport()
    await page.getByText('最近 7 天', { exact: true }).click()
    await expect(page.locator('.overview-trend-chart')).toHaveAttribute('data-bucket-count', '7')
    const series = page.getByRole('checkbox', { name: '成功', exact: true })
    await series.focus()
    await page.keyboard.press('Space')
    await expect(series).not.toBeChecked()
    await page.keyboard.press('Space')
    await expect(series).toBeChecked()
    await page.getByRole('heading', { name: '任务结果趋势', exact: true }).scrollIntoViewIfNeeded()
    await page.locator('.overview-trend-hit').first().focus()
    const point = await page.locator('.overview-trend-chart circle').first().boundingBox()
    expect(Math.abs(point!.width - point!.height), '原生 200% 缩放不拉伸数据点').toBeLessThan(0.5)
    const session = await context.newCDPSession(page)
    const capture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('overview-native-200-percent.png'), Buffer.from(capture.data, 'base64'))
    const enlarge = page.getByRole('button', { name: '放大任务结果趋势' })
    await enlarge.focus(); await page.keyboard.press('Enter')
    const dialog = page.getByRole('dialog', { name: '任务结果趋势 · 放大', exact: true })
    await expect(dialog).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const expanded = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('overview-expanded-native-200-percent.png'), Buffer.from(expanded.data, 'base64'))
    await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await expect(enlarge).toBeFocused()
    const analyze = page.getByRole('button', { name: '下钻分析', exact: true })
    await analyze.focus(); await page.keyboard.press('Enter')
    const analysis = page.getByRole('dialog', { name: '任务结果分析', exact: true })
    await expect(analysis.getByRole('heading', { name: '任务结果分析' })).toBeFocused()
    const failed = analysis.getByRole('radio', { name: '失败', exact: true })
    await failed.focus(); await page.keyboard.press('Space')
    await expect(failed).toBeChecked()
    const next = analysis.getByRole('button', { name: '下一页分析任务' })
    await expect(next).toBeEnabled()
    await next.focus(); await expect(next).toBeInViewport({ ratio: 1 }); await page.keyboard.press('Enter')
    await expect(analysis.getByRole('status', { name: '分析任务分页' })).toHaveText('第 2 页 / 2 页')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await analysis.locator('.overview-trend-dialog-body').evaluate(element => element.scrollWidth <= element.clientWidth), '弹窗正文不能随宽表溢出，横向滚动只属于表格').toBe(true)
    await expect(analysis.getByRole('button', { name: '上一页分析任务' })).toBeInViewport({ ratio: 1 })
    await expect(next).toBeInViewport({ ratio: 1 })
    const tableScroll = analysis.locator('.ant-table-content')
    await tableScroll.hover(); await page.mouse.wheel(1000, 0)
    await expect.poll(() => tableScroll.evaluate(element => element.scrollLeft)).toBeGreaterThan(0)
    await expect(analysis.getByRole('columnheader', { name: '终态时间（UTC）', exact: true })).toBeInViewport({ ratio: 1 })
    await page.mouse.wheel(-1000, 0)
    await expect.poll(() => tableScroll.evaluate(element => element.scrollLeft)).toBe(0)
    const analysisCapture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('overview-analysis-native-200-percent.png'), Buffer.from(analysisCapture.data, 'base64'))
    await page.keyboard.press('Escape'); await expect(analysis).toHaveCount(0); await expect(analyze).toBeFocused()
    await info.attach('native-browser-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })) }), contentType: 'application/json' })
    await info.attach('browser-version', { body: context.browser()?.version() ?? 'unknown', contentType: 'text/plain' })
    expect(errors).toEqual([])
    await verifyWindowErrors()
  } finally { await context.close() }
})

test('初次加载保留骨架且不展示猜测指标', async ({ page }, info) => {
  await fixture(page)
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  let requests = 0
  await page.route('**/api/v1/overview?range=24h', async route => { requests++; await pending; await route.fulfill({ json: { item: overviewFixture() } }) })
  await page.goto('/')
  await expect(page.locator('.ant-skeleton')).toBeVisible()
  await expect(page.getByRole('button', { name: /刷新中/ })).toBeDisabled()
  await expect(page.getByRole('button', { name: /运行中任务 .*查看任务/ })).toHaveCount(0)
  await expect(page.locator('vite-error-overlay')).toHaveCount(0)
  expect(requests).toBe(1)
  await page.screenshot({ path: info.outputPath('overview-initial-loading.png'), fullPage: true })
  release()
  await expect(page.getByRole('button', { name: '运行中任务 2 个，查看任务' })).toBeVisible()
})

test('首页可信空任务与节点范围受限往返刷新，恢复事实后再次清除旧表', async ({ page }, info) => {
  const model = await fixture(page, 'empty')
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/')
  const tasks = page.getByRole('region', { name: '我的任务', exact: true }), nodes = page.getByRole('region', { name: '执行节点', exact: true })
  await expect(tasks.getByText('当前没有本人创建的重点任务', { exact: true })).toBeVisible()
  await expect(nodes.getByText('当前没有可查看的执行节点，请核对节点对象授权。', { exact: true })).toBeVisible()
  await expect(tasks.getByRole('table')).toHaveCount(0); await expect(nodes.getByRole('table')).toHaveCount(0)
  await expect(page.getByRole('button', { name: /不可接收节点 .* 个，可见 .* 个/ })).toHaveCount(0)
  // 按实际视口记录空态，避免全页捕获改变固定外壳和待测尺寸。
  await page.screenshot({ path: info.outputPath('overview-empty-real-viewport.png'), fullPage: false })
  await nodes.scrollIntoViewIfNeeded()
  await page.screenshot({ path: info.outputPath('overview-empty-nodes-real-viewport.png'), fullPage: false })
  model.setMode('ready')
  const refresh = page.getByRole('button', { name: /^(刷新|已刷新)$/ })
  await refresh.click()
  await expect(tasks.getByRole('table')).toBeVisible(); await expect(nodes.getByRole('table')).toBeVisible()
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({ path: info.outputPath('overview-ready-real-viewport.png'), fullPage: false })
  model.setMode('empty')
  await refresh.click()
  await expect(nodes.getByText('当前没有可查看的执行节点，请核对节点对象授权。', { exact: true })).toBeVisible()
  await expect(tasks.getByRole('table')).toHaveCount(0); await expect(nodes.getByRole('table')).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
