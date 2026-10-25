import { chromium, expect, test, type Page } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import { writeFile } from 'node:fs/promises'
import { overviewFixture } from './fixtures/overview'
import type { OverviewRange, OverviewSnapshot } from '../src/api/overview'

type Mode = 'ready' | 'empty' | 'partial' | 'error' | 'denied' | 'invalid' | 'stale' | 'long' | 'task-restricted'
async function fixture(page: Page, initial: Mode = 'ready') {
  let mode = initial, calls = 0
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
    if (path === '/api/v1/tasks') return route.fulfill({ json: { items: [], totalPages: 0 } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [] } })
    return route.fulfill({ json: { items: [] } })
  })
  return { setMode: (next: Mode) => { mode = next }, get calls() { return calls }, requests }
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
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await expect(page.getByText('UTC（平台内置）', { exact: false }).first()).toBeVisible()
    await expect(page.getByRole('checkbox', { name: '终态总数' })).toBeChecked()
    await page.getByRole('checkbox', { name: '成功', exact: true }).uncheck()
    await expect(page.locator('svg g.trend-succeeded')).toHaveCount(0)
    await page.getByRole('checkbox', { name: '成功', exact: true }).check()
    await page.getByText('最近 7 天', { exact: true }).click()
    await expect.poll(() => model.calls).toBe(2)
    await expect(page.locator('.overview-trend-chart g.trend-total circle')).toHaveCount(7)
    await page.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(page.getByRole('button', { name: '已刷新', exact: true })).toBeVisible()
    expect(model.requests.every(request => request.startsWith('GET '))).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath(`overview-${viewport.width}.png`), fullPage: true })
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

test('DEC-060 关注优先结构：分区顺序、节点状态格与异常原因来自同一快照', async ({ page }) => {
  await fixture(page)
  await page.goto('/')
  const headings = await page.locator('.overview-workspace h2').allTextContents()
  expect(headings).toEqual(['运行状态', '需要关注', '我的任务', '执行节点', '任务结果趋势'])
  const attention = page.getByRole('region', { name: '需要关注', exact: true })
  await expect(attention.getByText('当前授权范围内今日失败 2 个', { exact: false })).toBeVisible()
  await expect(attention.getByText('节点当前事实 · Agent 当前离线', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '执行节点', exact: true }).getByRole('table')).toBeVisible()
  await page.getByRole('button', { name: '不可接收节点 1 个，可见 2 个，查看执行节点' }).click()
  await expect(page).toHaveURL(/\/nodes$/)
})

test('节点范围受限时状态带不输出节点计数', async ({ page }) => {
  await fixture(page, 'empty')
  await page.goto('/')
  await expect(page.getByRole('region', { name: '运行状态' }).getByText('受限', { exact: true })).toBeVisible()
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

test('趋势图形不可交互，明细按同一桶与子系列下钻且键盘可以激活', async ({ page }) => {
  await fixture(page)
  await page.goto('/')
  const chart = page.locator('.overview-trend-chart')
  await expect(chart).toHaveAttribute('role', 'img')
  await expect(chart.locator('a, [tabindex], button')).toHaveCount(0)
  const disclosure = page.getByRole('button', { name: /按时间段查看与下钻/ })
  await disclosure.focus()
  await page.keyboard.press('Enter')
  const detail = page.locator('.overview-trend-detail')
  const point = detail.getByRole('button', { name: /成功 1 个，查看任务/ }).first()
  await expect(point).toBeVisible()
  const label = await point.getAttribute('aria-label')
  await point.focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/state=SUCCEEDED/)
  const from = label!.slice(0, 19).replace(' ', 'T')
  await expect.poll(() => decodeURIComponent(new URL(page.url()).searchParams.get('finishedFrom') ?? '')).toContain(from)
  await expect(page.getByText(/终态时间（UTC，含起点、不含终点）/)).toBeVisible()
})

test('趋势范围入口带入完整终态窗口且不附加状态', async ({ page }) => {
  await fixture(page)
  await page.goto('/')
  await page.getByRole('button', { name: '查看该范围全部终态任务' }).click()
  await expect(page).toHaveURL(/finishedFrom=/)
  await expect(page).not.toHaveURL(/state=/)
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
  await expect(page.locator('.overview-trend-chart g.trend-total circle')).toHaveCount(7)
  await expect(page.getByText(/所选时间范围尚未取得/)).toHaveCount(0)
})

test('首页原生 200% 缩放下刷新和趋势键盘操作可达', async ({ browserName }, info) => {
  test.setTimeout(180000)
  expect(browserName).toBe('chromium')
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext('', {
    channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined,
    args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage()
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
    await expect(page.locator('.overview-trend-chart g.trend-total circle')).toHaveCount(7)
    const series = page.getByRole('checkbox', { name: '成功', exact: true })
    await series.focus()
    await page.keyboard.press('Space')
    await expect(series).not.toBeChecked()
    await page.keyboard.press('Space')
    await expect(series).toBeChecked()
    await page.getByRole('heading', { name: '任务结果趋势', exact: true }).scrollIntoViewIfNeeded()
    const session = await context.newCDPSession(page)
    const capture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('overview-native-200-percent.png'), Buffer.from(capture.data, 'base64'))
    await info.attach('native-browser-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })) }), contentType: 'application/json' })
    await info.attach('browser-version', { body: context.browser()?.version() ?? 'unknown', contentType: 'text/plain' })
    expect(errors).toEqual([])
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
