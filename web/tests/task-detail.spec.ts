import { expect, test, type Page, type Route } from '@playwright/test'
import { mockFacts } from './fixtures/facts'

const time = '2026-01-01T00:00:00Z'
const overview = (id: string, state = 'RUNNING') => ({ id, type: 'OBDUMPER_EXPORT', dataSourceId: `source-${id}`, nodeId: `node-${id}`, precheckId: `precheck-${id}`, submittedAt: time, derivationKind: 'REBUILD_FROM_CONFIG', parentTaskId: id === 'A' ? 'B' : 'A', presentation: { id, displayName: id, type: 'OBDUMPER_EXPORT', dataSourceId: `source-${id}`, nodeId: `node-${id}`, dataSourceDeleted: false, nodeDeleted: false, state, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', ownedByCurrentUser: true, reconciliationRequired: false, cancelRequested: false, submittedAt: time, updatedAt: time, availableActions: ['COPY_COMMAND', 'VIEW_LOGS', ...(state === 'SUCCEEDED' ? ['SAVE_TEMPLATE', 'REBUILD_FROM_CONFIG'] : state === 'FAILED' ? ['REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH', 'CHECKPOINT_RESUME'] : ['CANCEL'])] } })
const execution = (state = 'RUNNING', checkpointPresent = false) => ({ state, executionId: 'synthetic-execution', reconciliationRequired: false, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', updatedAt: time, resultSummary: state === 'SUCCEEDED' ? { result: 'VERIFIED', fileCount: 1, totalBytes: 1024, files: [{ path: 'synthetic.csv', size: 1024 }], checkpointPresent, observedAt: time } : state === 'FAILED' ? { result: 'FAILED', fileCount: 0, totalBytes: 0, files: [], checkpointPresent, observedAt: time } : undefined })
const log = (message: string) => ({ sourceSeq: 1, receivedAt: time, kind: 'STDOUT', integrityCode: 'COMPLETE', message })

async function mockTask(page: Page, getState: (id: string) => string = () => 'RUNNING') {
  await mockFacts(page)
  await page.route('**/api/v1/tasks/**', async route => {
    const path = new URL(route.request().url()).pathname
    const id = path.split('/')[4]!
    let json: object = { item: overview(id, getState(id)) }
    if (path.endsWith('/execution')) json = { item: execution(getState(id), true) }
    if (path.endsWith('/snapshot')) json = { item: { type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: `source-${id}`, nodeId: `node-${id}`, precheckId: `precheck-${id}`, objectSummary: `synthetic.${id}`, format: 'CSV', configFingerprint: id, toolVersion: '4.3.5', metadataVersion: 'synthetic', capabilityVersion: 'synthetic' } }
    if (path.endsWith('/command-evidence')) json = { item: { kind: 'PLANNED', command: `synthetic-command-${id}`, redaction: 'PASSWORD_ONLY' } }
    if (path.endsWith('/logs')) json = { items: [log(`persisted-${id}`)], integrity: 'COMPLETE', lastReliableCursor: `cursor-${id}` }
    if (path.endsWith('/operations')) json = { items: [], nextCursor: null }
    await route.fulfill({ json })
  })
}

async function fakeStreams(page: Page) {
  await page.addInitScript(() => {
    // 合成 EventSource 保留关闭后的回调，用于模拟浏览器队列中已经迟到的事件。
    class SyntheticEventSource extends EventTarget {
      static streams: SyntheticEventSource[] = []
      closed = false
      onerror?: () => void
      constructor(public url: string) { super(); SyntheticEventSource.streams.push(this) }
      close() { this.closed = true }
    }
    Object.assign(window, { EventSource: SyntheticEventSource, taskStreams: SyntheticEventSource.streams })
  })
}

async function emit(page: Page, index: number, message: string) {
  await page.evaluate(({ index, message, time }) => {
    const streams = (window as unknown as { taskStreams: EventTarget[] }).taskStreams
    streams[index]!.dispatchEvent(new MessageEvent('log', { data: JSON.stringify({ sourceSeq: 2, receivedAt: time, kind: 'STDOUT', integrityCode: 'COMPLETE', message }), lastEventId: `cursor-${message}` }))
  }, { index, message, time })
}
async function streamFacts(page: Page) {
  return page.evaluate(() => (window as unknown as { taskStreams: Array<{ url: string; closed: boolean }> }).taskStreams.map(({ url, closed }) => ({ url, closed })))
}
async function navigateTask(page: Page, id: string) {
  // 调用真实路由器，只变更参数；不刷新页面，不通过 key 强制销毁实例掩盖隔离问题。
  await page.evaluate(async id => {
    const modulePath = '/src/router/index.ts'
    const { router } = await import(/* @vite-ignore */ modulePath)
    await router.push(`/tasks/${id}`)
  }, id)
}

test('任务详情同实例切换隔离迟到读取和日志，刷新不叠加轮询，卸载关闭流', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  let state = 'RUNNING'
  await mockTask(page, () => state)
  await fakeStreams(page)
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/tasks/A?tab=logs')
  await expect(page.getByRole('heading', { name: 'A', exact: true })).toBeVisible()
  await expect.poll(async () => (await streamFacts(page)).length).toBe(1)
  await page.locator('.page-heading').evaluate(element => { element.setAttribute('data-instance-evidence', 'same-instance') })
  const counts = { A: 0, B: 0 }
  let delayA = true
  const held: Route[] = []
  async function release() {
    await Promise.all(held.map(route => route.fulfill({ json: { item: execution('FAILED') } }).catch(() => {})))
  }
  await page.route('**/api/v1/tasks/*/execution', async route => {
    const id = new URL(route.request().url()).pathname.split('/')[4] as 'A' | 'B'
    counts[id]++
    if (id === 'A' && delayA) {
      // 暂存拦截请求而不阻塞处理器退出，失败时测试框架仍可正常卸载。
      held.push(route)
    } else await route.fulfill({ json: { item: execution(state) } })
  })
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect.poll(() => counts.A).toBe(1)
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  expect(counts.A).toBe(1)
  // 使用页面内来源任务链接，Vue Router 复用当前详情实例。
  await page.locator('.task-derivation-note').getByRole('link', { name: 'B', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'B', exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '执行日志' }).click()
  await expect(page.locator('.page-heading')).toHaveAttribute('data-instance-evidence', 'same-instance')
  await expect.poll(async () => (await streamFacts(page)).length).toBe(2)
  expect((await streamFacts(page))[0]!.closed).toBe(true)
  await emit(page, 0, 'late-A')
  await release()
  delayA = false
  await expect(page.locator('.task-log-list')).toContainText('persisted-B')
  await expect(page.locator('.task-log-list')).not.toContainText('persisted-A')
  await expect(page.locator('.task-log-list')).not.toContainText('late-A')
  await page.getByRole('tab', { name: '命令', exact: true }).click()
  await expect(page.getByText('synthetic-command-B', { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '运行概览' }).click()
  await expect(page.getByText('synthetic.B', { exact: true })).toBeVisible()
  await expect(page.getByText('运行中', { exact: true }).first()).toBeVisible()
  await page.getByRole('tab', { name: '执行日志' }).click()
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  expect((await streamFacts(page)).length).toBe(2)
  await page.evaluate(() => {
    const streams = (window as unknown as { taskStreams: Array<{ onerror?: () => void }> }).taskStreams
    streams[1]!.onerror?.()
  })
  await expect(page.getByText(/实时连接中断/)).toBeVisible()
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect.poll(async () => (await streamFacts(page)).length).toBe(3)
  await emit(page, 1, 'late-disconnected-B')
  await emit(page, 2, 'live-B')
  await expect(page.locator('.task-log-list')).toContainText('live-B')
  await expect(page.locator('.task-log-list')).not.toContainText('late-disconnected-B')
  state = 'SUCCEEDED'
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByRole('button', { name: '保存为模板', exact: true })).toBeVisible()
  expect((await streamFacts(page))[2]!.closed).toBe(true)
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({ path: info.outputPath('task-B-desktop.png'), fullPage: false })
  await page.setViewportSize({ width: 1280, height: 720 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('task-B-1280.png'), fullPage: true })
  await page.getByRole('link', { name: '返回任务中心' }).click()
  await expect(page).toHaveURL(/\/tasks$/)
  const before = counts.B
  await page.waitForTimeout(2200)
  expect(counts.B).toBe(before)
  expect(await page.locator('vite-error-overlay').count()).toBe(0)
  expect(errors).toEqual([])
})

test('A 初始概览迟到和卸载中的读取不覆盖 B 或重新建立后台生命周期', async ({ page }) => {
  await mockTask(page)
  await fakeStreams(page)
  const held: Route[] = []
  async function release() {
    await Promise.all(held.map(route => route.fulfill({ json: { item: overview('A') } }).catch(() => {})))
  }
  let requested = false
  await page.route('**/api/v1/tasks/A', async route => {
    requested = true
    held.push(route)
  })
  await page.goto('/tasks/A')
  await expect.poll(() => requested).toBe(true)
  await navigateTask(page, 'B')
  await expect(page.getByRole('heading', { name: 'B', exact: true })).toBeVisible()
  await release()
  await expect(page.getByRole('heading', { name: 'A', exact: true })).toHaveCount(0)
  expect((await streamFacts(page)).map(s => s.url)).toEqual(['/api/v1/tasks/B/logs/stream?after=cursor-B'])
  await page.getByRole('link', { name: '返回任务中心' }).click()
  expect((await streamFacts(page))[0]!.closed).toBe(true)
})

for (const [label, suffix, step] of [
  ['基于原配置新建', 'rebuild-draft', '1'],
  ['从头重新执行', 'rebuild-draft', '5'],
  ['从检查点继续', 'resume-checkpoint', ''],
] as const) {
  test(`任务派生 ${label} 保留来源、CSRF、幂等与原导航`, async ({ page }) => {
    await mockTask(page, () => 'FAILED')
    let writes = 0
    await page.route(`**/api/v1/tasks/A:${suffix}`, async route => {
      writes++
      expect(route.request().headers()['x-csrf-token']).toBe('local-mvp-csrf-v1')
      expect(route.request().headers()['idempotency-key']).toBeTruthy()
      const json = suffix === 'rebuild-draft'
        ? { draftId: 'synthetic-derived', sourceTaskId: 'A', derivation: step === '1' ? 'REBUILD_FROM_CONFIG' : 'RERUN_FROM_SCRATCH' }
        : { id: 'synthetic-resumed', parentTaskId: 'A', derivationKind: 'CHECKPOINT_RESUME' }
      if (suffix === 'rebuild-draft') expect(route.request().postDataJSON()).toEqual({ derivation: json.derivation })
      await route.fulfill({ json })
    })
    await page.goto('/tasks/A')
    if (label === '从检查点继续') await page.getByRole('button', { name: label, exact: true }).click()
    else { await page.getByRole('button', { name: '更多操作' }).click(); await page.getByRole('menuitem', { name: label, exact: true }).click() }
    expect(writes).toBe(0)
    await page.getByRole('dialog', { name: label, exact: true }).getByRole('button', { name: label, exact: true }).click()
    await expect(page).toHaveURL(step ? new RegExp(`/exports/new\\?draft=synthetic-derived&step=${step}`) : /\/tasks\/synthetic-resumed$/)
    expect(writes).toBe(1)
  })
}

test('成功任务保存模板，切换任务后旧派生写响应不得强制导航', async ({ page }) => {
  await mockTask(page, () => 'SUCCEEDED')
  let writes = 0
  await page.route('**/api/v1/tasks/A:save-template', async route => {
    writes++
    expect(route.request().postDataJSON()).toEqual({ displayName: '合成模板' })
    await route.fulfill({ json: { id: 'synthetic-template', sourceTaskId: 'A' } })
  })
  await page.goto('/tasks/A')
  await page.getByRole('button', { name: '保存为模板', exact: true }).click()
  await page.getByRole('textbox', { name: '模板名称' }).fill(' 合成模板 ')
  await page.getByRole('dialog', { name: '保存为模板' }).getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.ant-message').getByText('模板「合成模板」已保存。', { exact: true })).toBeVisible()
  await expect(page.locator('.task-failure-actions .ant-alert-success')).toHaveCount(0)
  await expect(page.getByRole('dialog', { name: '保存为模板' })).toHaveCount(0)
  expect(writes).toBe(1)

  await mockTask(page, () => 'FAILED')
  await navigateTask(page, 'B')
  const held: Route[] = []
  async function release() {
    await Promise.all(held.map(route => route.fulfill({ json: { draftId: 'late-derived', sourceTaskId: 'B', derivation: 'REBUILD_FROM_CONFIG' } }).catch(() => {})))
  }
  let requested = false
  await page.route('**/api/v1/tasks/B:rebuild-draft', async route => {
    requested = true
    held.push(route)
  })
  await page.getByRole('button', { name: '更多操作' }).click()
  await page.getByRole('menuitem', { name: '基于原配置新建', exact: true }).click()
  await page.getByRole('dialog', { name: '基于原配置新建' }).getByRole('button', { name: '基于原配置新建', exact: true }).click()
  await expect.poll(() => requested).toBe(true)
  await navigateTask(page, 'A')
  await expect(page.getByRole('heading', { name: 'A', exact: true })).toBeVisible()
  await release()
  await page.waitForTimeout(250)
  await expect(page).toHaveURL(/\/tasks\/A$/)
  await expect(page.getByRole('alert').filter({ hasText: /发起失败/ })).toHaveCount(0)
})

test('五个 Tab 使用同一事实，日志过滤错误定位、过期、操作记录分页与远端结果不冒充文件核验', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  await mockTask(page, () => 'SUCCEEDED')
  let logReads = 0, operationReads = 0
  await page.route('**/api/v1/tasks/A/execution', route => route.fulfill({ json: { item: { ...execution('SUCCEEDED'), plannedActualMatch: 'MATCHED', outputLocation: { kind: 'S3', redaction: 'KIND_ONLY' } } } }))
  await page.route('**/api/v1/tasks/A/logs*', route => { logReads++; return route.fulfill({ json: logReads > 1 ? { items: [], integrity: 'EXPIRED' } : { items: [log('normal'), { ...log('[ERROR] synthetic error'), sourceSeq: 2, kind: 'STDERR' }], integrity: 'COMPLETE' } }) })
  await page.route('**/api/v1/tasks/A/operations*', route => { operationReads++; const cursor = new URL(route.request().url()).searchParams.get('cursor'); return route.fulfill({ json: { items: [{ id: cursor ? 'operation-2' : 'operation-1', actorType: 'SUBJECT', actorName: '合成用户', action: cursor ? 'TASK_TEMPLATE_SAVED' : 'TASK_SUBMITTED', result: 'SUCCEEDED', occurredAt: time }], nextCursor: cursor ? null : 'operation-cursor' } }) })
  await page.goto('/tasks/A')
  await expect(page.getByRole('tab')).toHaveCount(5)
  await expect(page.getByText('远端对象未逐项核验', { exact: true })).toBeVisible()
  await expect(page.getByText('仅创建者可见', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: /展开文件清单/ })).toHaveCount(0)
  await page.getByRole('tab', { name: '配置快照' }).click()
  await expect(page).toHaveURL(/tab=snapshot/)
  for (const label of ['1 · 数据源与执行节点', '2 · 导出对象与内容', '3 · 数据格式', '4 · 输出与性能', '5 · 预检查与提交']) await expect(page.getByText(label, { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '命令', exact: true }).click(); await expect(page.getByText('完整实际启动命令与启动目录未保存', { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '执行日志' }).click()
  await page.getByRole('textbox', { name: '日志关键字' }).fill('error'); await expect(page.locator('.task-log-record')).toHaveCount(1)
  await page.getByRole('button', { name: '跳转首条错误' }).click(); await expect(page.locator('[data-log-error="true"]')).toBeFocused()
  await page.getByRole('button', { name: '暂停自动滚动' }).click(); await expect(page.getByRole('button', { name: '恢复自动滚动' })).toHaveAttribute('aria-pressed', 'true')
  await page.getByRole('button', { name: '刷新日志' }).click(); await expect(page.getByText('日志已过保留期', { exact: true })).toBeVisible(); await expect(page.locator('.task-log-record')).toHaveCount(0)
  await page.getByRole('tab', { name: '操作记录' }).click(); await expect(page.getByText('提交任务', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '加载更多' }).click(); await expect(page.getByText('保存模板', { exact: true })).toBeVisible(); expect(operationReads).toBe(2)
  await page.getByRole('tab', { name: '运行概览' }).click(); await expect(page.getByText('远端对象未逐项核验', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('task-detail-overview.png'), fullPage: true })
  await page.getByRole('tab', { name: '配置快照' }).click(); await page.screenshot({ path: info.outputPath('task-detail-snapshot.png'), fullPage: true })
  expect(errors).toEqual([])
})
