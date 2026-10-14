import { expect, test, type Page, type Route } from '@playwright/test'
import type { ExecutionNodeDetail, NodeRuntimeInspection } from '../src/api/browser'
import { mockFacts } from './fixtures/facts'

const sampledAt = '2026-10-12T00:00:00Z'
const gib = 1024 ** 3

function fullInspection() {
  const kinds = ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME', 'CONNECTOR', 'WORKSPACE', 'LOG_QUEUE', 'TOOL_LOG', 'DIRECT_LOAD_CONFIG', 'RESOURCES', 'RUNTIME_LAYOUT'] as const
  return {
    checkId: 'synthetic-environment-check', runtimeConfigurationDigest: 'a'.repeat(64), observedAt: sampledAt, versionProbeAllowed: true,
    items: kinds.map(kind => ({ kind, status: 'PASSED' as const, code: 'READY', required: ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'CONNECTOR', 'WORKSPACE', 'LOG_QUEUE', 'RESOURCES', 'RUNTIME_LAYOUT'].includes(kind), path: kind === 'RESOURCES' || kind === 'RUNTIME_LAYOUT' ? undefined : 'E:\\synthetic-runtime\\' + kind, version: kind === 'JAVA_RUNTIME' ? '1.8.0_491' : kind === 'OBDUMPER_RUNTIME' || kind === 'OBLOADER_RUNTIME' ? '4.3.5-RELEASE' : undefined, exists: kind === 'RESOURCES' ? undefined : true, readable: kind === 'WORKSPACE' || kind === 'LOG_QUEUE' ? true : undefined, writable: kind === 'WORKSPACE' || kind === 'LOG_QUEUE' ? true : undefined })),
    directLoad: { status: 'READ' as const, taskTimeoutMs: 0, heartbeatTimeoutMs: 60000000, heartbeatIntervalMs: 30000000, hasUnrecognizedKeys: true, fingerprint: 'b'.repeat(64) },
  }
}

function detail(id = 'synthetic-detail', overrides: Partial<ExecutionNodeDetail> = {}): ExecutionNodeDetail {
  return {
    id, displayName: '合成运行节点 ' + id, platform: 'WINDOWS_AMD64', managementState: 'ENABLED',
    agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: sampledAt,
    environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: true, unavailableReasons: [],
    revision: 1, updatedAt: sampledAt, createdAt: sampledAt,
    toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java\\bin\\java.exe', allowedRoots: ['E:\\synthetic-exports', 'F:\\synthetic-exports'],
    agentFacts: { os: 'windows', arch: 'amd64', agentVersion: 'synthetic-v1', bootId: 'synthetic-boot', observedAt: sampledAt, capacityTotal: 8, capacityUsed: 3, cpuUsagePercent: 25, memoryUsagePercent: 75, memoryTotalBytes: 32 * gib, memoryAvailableBytes: 8 * gib },
    dataRootUsages: [{ root: 'E:\\synthetic-exports', totalBytes: 400 * gib, availableBytes: 100 * gib }, { root: 'F:\\synthetic-exports', totalBytes: 100 * gib, availableBytes: 5 * gib }],
    executionQuota: { totalGiB: 20, reservedGiB: 5, usedGiB: 10 },
    taskSummary: { scope: 'MINE', running: 2, waiting: 3, reconciling: 1 },
    budgetPolicy: { systemReservedBytes: 2 * gib, taskOverheadBytes: 0 }, defaultJvmMemory: '4G',
    environmentCheck: { checkId: 'synthetic-environment-check', status: 'PASSED', code: 'TOOL_RUNTIME_READY', factsRevision: 3, requestedAt: '2026-10-11T23:58:00Z', completedAt: '2026-10-11T23:59:00Z' },
    agentDiagnostics: {
      agentId: 'synthetic-agent', protocolVersion: 'agent-v1', factsRevision: 3, startedAt: '2026-10-11T23:00:00Z',
      expectedRuntimeConfigurationDigest: 'a'.repeat(64), runtimeConfigurationDigest: 'a'.repeat(64),
      runtimeVersions: { java: { version: '1.8.0_491', source: 'JAVA_RELEASE' }, obloader: { version: '4.3.5-RELEASE', source: 'JAR_MANIFEST' }, obdumper: { version: '4.3.5-RELEASE', source: 'JAR_POM' } },
      logQueue: { pendingBatches: 2, pendingGaps: 1, pendingBytes: 2048, lastUploadResult: 'FAILED', lastUploadAt: '2026-10-11T23:59:00Z' },
    },
    activeExecutions: { scope: 'READABLE', items: [{ taskId: 'synthetic-task', executionId: 'synthetic-execution', leaseId: 'synthetic-lease', leaseEpoch: 2, lastEventSeq: 3 }] },
    ...overrides,
  }
}

async function navigate(page: Page, path: string) {
  // 使用真实路由复用同一详情实例，验证 Tab 与旧响应的会话边界。
  await page.evaluate(async target => {
    const modulePath = '/src/router/index.ts'
    const { router } = await import(/* @vite-ignore */ modulePath)
    await router.push(target)
  }, path)
}

for (const scope of ['MINE', 'ALL'] as const) {
  test('默认概览显示 ' + scope + ' 真实资源与任务范围，Tab 切换只消费同一份详情', async ({ page }, info) => {
    await mockFacts(page)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    let reads = 0
    const current = detail('synthetic-detail', { taskSummary: { scope, running: 2, waiting: 3, reconciling: 1 } })
    await page.route('**/api/v1/execution-nodes/synthetic-detail', route => { reads++; return route.fulfill({ json: { item: current } }) })
    await page.setViewportSize({ width: 1440, height: 1024 })
    await page.goto('/nodes/synthetic-detail')
    await expect(page).toHaveTitle('OB Data Orch · 执行节点详情')
    const actions = page.getByRole('group', { name: '节点操作', exact: true })
    await expect(actions.getByRole('link', { name: '返回执行节点', exact: true })).toHaveAttribute('href', '/nodes')
    await expect(actions.getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveAttribute('href', '/ob-data-orch-agent-windows-x86_64.zip')
    await expect(page.getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveCount(1)
    const overview = page.getByRole('tab', { name: '运行概览', exact: true })
    await expect(overview).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('region', { name: '节点运行状态' }).locator('[data-state-category]')).toHaveCount(4)
    await expect(page.getByRole('heading', { name: '工具与内存配置', exact: true })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: '环境检查', exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: '日志上传队列', exact: true })).toHaveCount(0)
    for (const reading of ['CPU 25.0%', '内存 75.0%', '磁盘 95.0%', '执行额度占用 75.0%']) await expect(page.locator('[aria-label="' + reading + '"]')).toBeVisible()
    await expect(page.getByRole('heading', { name: scope === 'ALL' ? '全部任务' : '我的任务', exact: true })).toBeVisible()
    for (const summary of ['运行中 2', '等待中 3', '核对中 1']) await expect(page.getByText(summary, { exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: '查看任务 →', exact: true })).toHaveAttribute('href', '/tasks?nodeId=synthetic-detail&scope=' + scope)
    await expect(page.getByRole('button', { name: '启用节点', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '检查节点环境', exact: true })).toHaveCount(0)
    await page.locator('[aria-label="磁盘 95.0%"]').focus()
    await expect(page.getByRole('tooltip')).toHaveText('95.0 GiB / 100.0 GiB')
    await overview.focus()
    await expect(page.getByRole('tooltip')).toBeHidden()
    if (scope === 'MINE') await page.screenshot({ path: info.outputPath('node-detail-overview-1440.png'), fullPage: true })
    await overview.press('ArrowRight')
    const configuration = page.getByRole('tab', { name: '环境与配置', exact: true })
    await expect(configuration).toBeFocused()
    await configuration.press('Enter')
    await expect(configuration).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('heading', { name: '工具与内存配置', exact: true })).toBeVisible()
    const configurationPanel = page.getByRole('tabpanel', { name: '环境与配置', exact: true })
    for (const repeated of ['CPU 架构', '事实采样时间', '任务槽位快照', '上报平台与登记不同']) await expect(configurationPanel.getByText(repeated, { exact: true })).toHaveCount(0)
    const directories = configurationPanel.getByRole('list', { name: '导出目录', exact: true })
    for (const root of current.allowedRoots) await expect(directories.getByText(root, { exact: true })).toBeVisible()
    await expect(configurationPanel.getByRole('heading', { name: '导出目录', exact: true })).toHaveCount(0)
    await expect(page.getByText('系统预留内存', { exact: true })).toBeVisible()
    await expect(page.getByText('2 GiB', { exact: true })).toBeVisible()
    await expect(page.getByText('自动', { exact: true })).toBeVisible()
    await expect(page.getByText('4 GiB', { exact: true })).toBeVisible()
    await expect(page.getByText(current.javaPath, { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: '导出数据目录与空间', exact: true })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: '环境检查记录', exact: true })).toHaveCount(0)
    await expect(page.getByText('检查绑定事实版本', { exact: true })).toHaveCount(0)
    await expect(page.getByText('1.8.0_491（待检查）', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: '日志上传队列', exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: '环境检查', exact: true })).toHaveCount(0)
    await expect(actions.getByRole('link', { name: '下载 Agent 包', exact: true })).toBeVisible()
    await expect(configurationPanel.getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveCount(0)
    if (scope === 'MINE') await page.screenshot({ path: info.outputPath('node-detail-configuration-1440.png'), fullPage: true })
    await configuration.press('Home')
    await expect(overview).toBeFocused()
    await expect(overview).toHaveAttribute('aria-selected', 'true')
    await overview.press('End')
    await expect(configuration).toBeFocused()
    await expect(configuration).toHaveAttribute('aria-selected', 'true')
    await configuration.press('ArrowLeft')
    await expect(overview).toBeFocused()
    await expect(page.getByRole('heading', { name: '执行额度', exact: true })).toBeVisible()
    expect(reads).toBe(1)
    expect(errors).toEqual([])
  })
}

test('未知快照和统计不填零，真实零值仍可见，刷新提供原位反馈', async ({ page }) => {
  await mockFacts(page)
  let current = detail('synthetic-detail', { agentFacts: undefined, dataRootUsages: undefined, executionQuota: undefined, taskSummary: undefined, budgetPolicy: undefined, defaultJvmMemory: undefined })
  const held: Route[] = []
  let hold = false
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => { if (hold) { held.push(route); return } return route.fulfill({ json: { item: current } }) })
  await page.goto('/nodes/synthetic-detail')
  await expect(page.locator('[aria-label="CPU 尚未采集"]')).toBeVisible()
  await expect(page.locator('[aria-label="执行额度未知"]')).toBeVisible()
  for (const state of ['运行中 —', '等待中 —', '核对中 —']) await expect(page.getByText(state, { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  await expect(page.getByText('系统预留内存', { exact: true }).locator('..')).toHaveText('系统预留内存尚未取得')
  await expect(page.getByText('每任务额外预留', { exact: true }).locator('..')).toHaveText('每任务额外预留尚未取得')
  await expect(page.getByText('自动', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '导出数据目录与空间', exact: true })).toHaveCount(0)
  const directories = page.getByRole('list', { name: '导出目录', exact: true })
  await expect(directories.getByRole('listitem')).toHaveCount(current.allowedRoots.length)
  for (const root of current.allowedRoots) await expect(directories.getByText(root, { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '运行概览', exact: true }).click()
  const facts = detail().agentFacts!
  current = detail('synthetic-detail', {
    agentFacts: { ...facts, cpuUsagePercent: 0, memoryUsagePercent: 0, memoryAvailableBytes: facts.memoryTotalBytes },
    dataRootUsages: [{ root: 'E:\\synthetic-exports', totalBytes: 100 * gib, availableBytes: 100 * gib }],
    executionQuota: { totalGiB: 20, usedGiB: 0, reservedGiB: 0 }, taskSummary: { scope: 'MINE', running: 0, waiting: 0, reconciling: 0 },
  })
  hold = true
  const refresh = page.getByRole('button', { name: '刷新节点详情' })
  await refresh.click()
  await expect.poll(() => held.length).toBe(1)
  await expect(refresh).toHaveText('刷新中')
  await expect(refresh).toBeDisabled()
  await expect(page.locator('[aria-label="执行额度未知"]')).toBeVisible()
  await held[0]!.fulfill({ json: { item: current } })
  await expect(refresh).toHaveText('已刷新')
  for (const reading of ['CPU 0.0%', '内存 0.0%', '磁盘 0.0%', '执行额度占用 0.0%']) await expect(page.locator('[aria-label="' + reading + '"]')).toBeVisible()
  for (const state of ['运行中 0', '等待中 0', '核对中 0']) await expect(page.getByText(state, { exact: true })).toBeVisible()
})

test('平台差异保留实际上报，配置目录原样展示且不混入旧采样目录与空间', async ({ page }) => {
  await mockFacts(page)
  const facts = detail().agentFacts!
  const current = detail('synthetic-detail', {
    agentFacts: { ...facts, os: 'LINUX', arch: 'ARM64' }, environmentStatus: 'ABNORMAL', acceptsNewTasks: false,
    unavailableReasons: ['ENVIRONMENT_CHECK_ABNORMAL'],
    dataRootUsages: [{ root: 'F:\\synthetic-exports', totalBytes: 100 * gib, availableBytes: 5 * gib }, { root: 'G:\\synthetic-old-root', totalBytes: 100 * gib, availableBytes: 1 * gib }],
  })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  await page.goto('/nodes/synthetic-detail')
  await expect(page.locator('.node-detail-title')).toContainText('Windows · x86_64')
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  await expect(page.getByText('实际上报平台', { exact: true })).toBeVisible()
  await expect(page.getByText('Linux · aarch64（与登记不同）', { exact: true })).toBeVisible()
  const directories = page.getByRole('list', { name: '导出目录', exact: true })
  for (const root of current.allowedRoots) await expect(directories.getByText(root, { exact: true })).toBeVisible()
  await expect(directories.locator('..').locator('.node-environment-note')).toHaveCount(0)
  await expect(directories).not.toContainText('GiB')
  const disks = page.getByRole('list', { name: '磁盘大小', exact: true })
  await expect(disks.getByRole('listitem')).toHaveCount(current.allowedRoots.length)
  await expect(disks.getByRole('listitem').nth(0)).toHaveText('E:\\synthetic-exports · 尚未取得')
  await expect(disks.getByRole('listitem').nth(1)).toHaveText('F:\\synthetic-exports · 100.0 GiB')
  await expect(page.getByRole('heading', { name: '导出数据目录与空间', exact: true })).toHaveCount(0)
  await expect(page.getByText('G:\\synthetic-old-root', { exact: true })).toHaveCount(0)
})

test('日志队列只在配置页展示，删除的活动明细不返回，未知与真实零分开', async ({ page }) => {
  await mockFacts(page)
  let current = detail()
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  await page.goto('/nodes/synthetic-detail')
  await expect(page.getByRole('link', { name: '任务 synthetic-task →', exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '日志上传队列', exact: true })).toHaveCount(0)
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  const queue = page.locator('section[aria-labelledby="node-log-queue-title"]')
  await expect(queue.getByText('2 份', { exact: true })).toBeVisible()
  await expect(queue.getByText('有 1 处日志缺口等待上传确认。', { exact: true })).toBeVisible()
  await expect(queue.getByText('2.0 KB', { exact: true })).toBeVisible()
  await expect(queue.getByText('上传失败，等待重试', { exact: true })).toBeVisible()
  current = detail('synthetic-detail', { agentDiagnostics: undefined, environmentCheck: undefined, activeExecutions: undefined })
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('尚无可读取的 Agent 执行快照。', { exact: true })).toHaveCount(0)
  await expect(queue.getByText('Agent 尚未提供日志上传信息。', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: '任务 synthetic-task →', exact: true })).toHaveCount(0)
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  await expect(page.getByText('尚未取得版本', { exact: true })).toHaveCount(3)
  await expect(page.getByText('控制面尚未提供检查记录。', { exact: true })).toHaveCount(0)
  current = detail('synthetic-detail', { activeExecutions: { scope: 'READABLE', items: [] }, agentDiagnostics: { ...detail().agentDiagnostics!, logQueue: { pendingBatches: 0, pendingGaps: 0, pendingBytes: 0, lastUploadResult: 'UNKNOWN' } } })
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('当前读取范围内没有活动执行。', { exact: true })).toHaveCount(0)
  await expect(queue.getByText('0 份', { exact: true })).toBeVisible()
  await expect(queue.getByText('0 B', { exact: true })).toBeVisible()
})

test('离线保留快照，管理与 Agent 状态独立，服务错误不清空可信事实', async ({ page }, info) => {
  await mockFacts(page)
  let failed = false
  const current = detail('synthetic-detail', { heartbeatStatus: 'OFFLINE', acceptsNewTasks: false, unavailableReasons: ['HEARTBEAT_OFFLINE', 'EXECUTION_BUDGET_UNKNOWN'] })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => failed ? route.fulfill({ status: 503, json: { code: 'TEMPORARY', message: '合成读取失败' } }) : route.fulfill({ json: { item: current } }))
  await page.goto('/nodes/synthetic-detail')
  await expect(page.locator('.node-facts [data-state-category="management"]')).toContainText('已启用')
  await expect(page.locator('.node-facts [data-state-category="agent"]')).toContainText('离线')
  await expect(page.locator('.node-resources')).toHaveClass(/is-stale/)
  await expect(page.getByText('当前展示离线前的最后可信快照', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '检查节点环境', exact: true })).toHaveCount(0)
  await expect(page.getByText('执行内存额度尚不可核对', { exact: true })).toBeVisible()
  await expect(page.locator('.ant-collapse, details')).toHaveCount(0)
  const refreshedAt = await page.locator('.node-detail-freshness').textContent()
  failed = true
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('刷新未完成', { exact: true })).toBeVisible()
  await expect(page.locator('[aria-label="CPU 25.0%"]')).toBeVisible()
  await expect(page.locator('.node-detail-freshness')).toHaveText(refreshedAt!)
  await page.screenshot({ path: info.outputPath('node-detail-offline.png'), fullPage: true })
})

test('环境检查覆盖旧正常状态且防重，回传通过后仍需单独启用', async ({ page }) => {
  await mockFacts(page)
  await page.clock.install()
  let current = detail('synthetic-detail', { environmentStatus: 'EXPIRED', acceptsNewTasks: false, unavailableReasons: ['ENVIRONMENT_CHECK_EXPIRED'] })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  const checks: Route[] = []
  await page.route('**/api/v1/execution-nodes/synthetic-detail:environment-check', route => { checks.push(route) })
  let enables = 0
  await page.route('**/api/v1/execution-nodes/synthetic-detail:enable', route => {
    enables++
    current = { ...current, managementState: 'ENABLED', acceptsNewTasks: true, unavailableReasons: [], revision: 4 }
    return route.fulfill({ json: { item: current } })
  })
  await page.goto('/nodes/synthetic-detail')
  await expect(page.getByText('环境检查期间节点保持禁用', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '检查节点环境', exact: true }).click()
  await expect.poll(() => checks.length).toBe(1)
  await expect(page.getByRole('button', { name: '刷新节点详情' })).toBeDisabled()
  current = { ...current, managementState: 'DISABLED', environmentStatus: 'NORMAL', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], revision: 2 }
  await checks[0]!.fulfill({ json: { item: current } })
  await expect(page.locator('.node-facts [data-state-category="environment"]')).toContainText('检查中')
  await expect(page.getByRole('button', { name: '环境检查中', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: '启用节点', exact: true })).toHaveCount(0)
  current = { ...current, unavailableReasons: ['NODE_DISABLED'], revision: 3 }
  await page.clock.runFor(2500)
  await expect(page.getByRole('button', { name: '启用节点', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '启用节点', exact: true }).click()
  await expect(page.getByText('当前可接收新任务', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '启用节点', exact: true })).toHaveCount(0)
  expect(checks).toHaveLength(1)
  expect(enables).toBe(1)
})

test('同实例 A → B → A 重置 Tab，迟到刷新与授权失效不能恢复旧配置和反馈', async ({ page }) => {
  await mockFacts(page)
  let holdNext = false
  let restricted = false
  const held: Route[] = []
  await page.route('**/api/v1/execution-nodes/*', route => {
    const id = new URL(route.request().url()).pathname.split('/').pop()!
    if (holdNext) { holdNext = false; held.push(route); return }
    if (restricted) return route.fulfill({ status: 403, json: { code: 'FORBIDDEN', message: '无权限' } })
    return route.fulfill({ json: { item: detail(id) } })
  })
  await page.goto('/nodes/A')
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  await page.locator('.node-detail-header').evaluate(element => element.setAttribute('data-instance', 'retained'))
  holdNext = true
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect.poll(() => held.length).toBe(1)
  await navigate(page, '/nodes/B')
  await expect(page.getByRole('heading', { name: '合成运行节点 B', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: '运行概览', exact: true })).toHaveAttribute('aria-selected', 'true')
  await navigate(page, '/nodes/A')
  await expect(page.getByRole('heading', { name: '合成运行节点 A', exact: true })).toBeVisible()
  await held[0]!.fulfill({ json: { item: detail('A', { displayName: '合成旧响应', revision: 99 }) } }).catch(() => {})
  await expect(page.locator('.node-detail-header')).toHaveAttribute('data-instance', 'retained')
  await expect(page.getByRole('heading', { name: '合成旧响应', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '刷新节点详情' })).toHaveText('刷新')
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  restricted = true
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('无法打开执行节点', { exact: true })).toBeVisible()
  await expect(page.getByRole('tab')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '工具与内存配置', exact: true })).toHaveCount(0)
  await expect(page.locator('.node-detail-freshness')).toHaveCount(0)
  restricted = false
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.getByRole('tab', { name: '运行概览', exact: true })).toHaveAttribute('aria-selected', 'true')
})

test('概览与配置在桌面、窄屏和 200% 缩放下保留长值与操作', async ({ page }, info) => {
  await mockFacts(page)
  const longPath = 'E:\\synthetic-exports\\' + 'synthetic-long-directory-'.repeat(10)
  const current = detail('synthetic-detail', { displayName: '合成长名称执行节点'.repeat(6), toolHome: longPath, javaPath: longPath + '\\java.exe', allowedRoots: [longPath] })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  await page.goto('/nodes/synthetic-detail')
  for (const [width, height] of [[1920, 1080], [1440, 1024], [1280, 720], [640, 720], [320, 720]]) {
    await page.setViewportSize({ width: width!, height: height! })
    for (const tab of ['运行概览', '环境与配置']) {
      await page.getByRole('tab', { name: tab, exact: true }).click()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), String(width) + 'px ' + tab).toBe(true)
      if (tab === '环境与配置') await expect(page.getByRole('list', { name: '导出目录', exact: true }).getByText(longPath, { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '刷新节点详情' })).toBeVisible()
      await expect(page.getByRole('group', { name: '节点操作', exact: true }).getByRole('link', { name: '返回执行节点', exact: true })).toBeVisible()
      await expect(page.getByRole('group', { name: '节点操作', exact: true }).getByRole('link', { name: '下载 Agent 包', exact: true })).toBeVisible()
    }
    if (width === 1280 || width === 640) await page.screenshot({ path: info.outputPath('node-detail-configuration-' + width + '.png'), fullPage: true })
  }
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.evaluate(() => { document.body.style.zoom = '2' })
  for (const tab of ['运行概览', '环境与配置']) {
    await page.getByRole('tab', { name: tab, exact: true }).click()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), '200% ' + tab).toBe(true)
  }
  await page.screenshot({ path: info.outputPath('node-detail-200-percent.png'), fullPage: true })
})

test('核心环境与配置直观展示，检查移到概览，技术字段及已移除模块不展示', async ({ page }, info) => {
  await mockFacts(page)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  let current = detail('synthetic-detail', {
    agentDiagnostics: { ...detail().agentDiagnostics!, host: { hostname: 'synthetic-host', addressPrefixes: ['192.0.2.0/24', '2001:db8::/64'], osName: 'Synthetic Windows', osVersion: '10.0.26000', cpuModel: 'Synthetic CPU Model', logicalCpus: 8, physicalCores: 4 } },
    environmentCheck: { ...detail().environmentCheck!, source: 'SUBJECT', inspection: fullInspection() },
    recentTasks: { scope: 'READABLE', items: [{ id: 'synthetic-recent-task', type: 'OBDUMPER_EXPORT', state: 'SUCCEEDED', reconciliationRequired: false, submittedAt: sampledAt, startedAt: sampledAt, finishedAt: sampledAt }] },
    activity: { checkSource: 'SUBJECT', events: [{ id: 'synthetic-agent-event', action: 'AGENT_ENROLLED', source: 'AGENT', result: 'SUCCEEDED', occurredAt: sampledAt }], operations: [{ id: 'synthetic-node-operation', action: 'EXECUTION_NODE_ENVIRONMENT_CHECK_REQUESTED', source: 'SUBJECT', result: 'SUCCEEDED', occurredAt: sampledAt }] },
  })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/nodes/synthetic-detail')
  await expect(page.getByRole('link', { name: 'synthetic-recent-task →', exact: true })).toHaveCount(0)
  await expect(page.getByText('Agent 完成关联 · 完成', { exact: true })).toHaveCount(0)
  const inspection = page.locator('.node-inspection-items')
  await expect(page.getByRole('heading', { name: '环境检查', exact: true })).toBeVisible()
  await expect(inspection.locator('li')).toHaveCount(10)
  await expect(inspection.getByText('Java 环境', { exact: true })).toBeVisible()
  for (const heading of ['活动执行与租约', '近期任务', '最近节点事件', 'Direct Load 配置摘要', '环境检查记录', '逐项环境检查']) await expect(page.getByRole('heading', { name: heading, exact: true })).toHaveCount(0)
  await expect(page.locator('.ant-collapse, details')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('node-core-overview-1440.png'), fullPage: true })
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  for (const text of ['synthetic-host', 'Synthetic Windows · 10.0.26000', '1.8.0_491']) await expect(page.getByText(text, { exact: true })).toBeVisible()
  const environment = page.getByRole('region', { name: '执行环境', exact: true })
  const tools = page.getByRole('region', { name: '工具与内存配置', exact: true })
  await expect(environment.getByText('处理器型号', { exact: true }).locator('..')).toHaveText('处理器型号Synthetic CPU Model')
  await expect(environment.locator('.node-definition > div')).toHaveCount(9)
  await expect(tools.locator('.node-definition > div')).toHaveCount(9)
  await expect(environment.getByText('CPU 物理核', { exact: true }).locator('..')).toHaveText('CPU 物理核4 核')
  await expect(environment.getByText('CPU 逻辑核', { exact: true }).locator('..')).toHaveText('CPU 逻辑核8 核')
  await expect(environment.getByText('内存大小', { exact: true }).locator('..')).toHaveText('内存大小32.0 GiB')
  const disks = environment.getByRole('list', { name: '磁盘大小', exact: true })
  await expect(disks.getByRole('listitem')).toHaveText(['E:\\synthetic-exports · 400.0 GiB', 'F:\\synthetic-exports · 100.0 GiB'])
  for (const text of ['192.0.2.0/24、2001:db8::/64', '协议版本', '当前事实版本', '登记配置摘要', 'Agent 配置摘要', '检查标识', '检查绑定事实版本', '用户操作', '请求环境检查 · 完成']) await expect(page.getByText(text, { exact: true })).toHaveCount(0)
  const leftBounds = await environment.locator('.node-definition').boundingBox()
  const rightBounds = await tools.locator('.node-definition').boundingBox()
  expect(leftBounds).not.toBeNull()
  expect(rightBounds).not.toBeNull()
  expect(Math.abs(leftBounds!.y + leftBounds!.height - rightBounds!.y - rightBounds!.height)).toBeLessThan(2)
  await expect(page.getByRole('heading', { name: '日志上传队列', exact: true })).toBeVisible()
  await expect(inspection).toHaveCount(0)
  await expect(page.locator('.ant-collapse, details')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '导出数据目录与空间', exact: true })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('node-core-configuration-1440.png'), fullPage: true })
  for (const [width, height] of [[1280, 720], [640, 720], [320, 720]]) {
    await page.setViewportSize({ width: width!, height: height! })
    for (const tab of ['运行概览', '环境与配置']) {
      await page.getByRole('tab', { name: tab, exact: true }).click()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), String(width) + ' ' + tab).toBe(true)
    }
  }
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.evaluate(() => { document.body.style.zoom = '2' })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), '核心配置 200%').toBe(true)
  await page.evaluate(() => { document.body.style.zoom = '1' })
  const previous = fullInspection()
  current = { ...current, environmentCheck: { ...current.environmentCheck!, checkId: 'synthetic-new-check', status: 'PENDING', completedAt: undefined, inspection: previous } }
  await page.getByRole('tab', { name: '运行概览', exact: true }).click()
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('正在展示上次检查结果', { exact: true })).toBeVisible()
  current = {
    ...current, environmentCheck: { ...current.environmentCheck!, inspection: undefined },
    agentDiagnostics: { ...current.agentDiagnostics!, host: { ...current.agentDiagnostics!.host!, physicalCores: undefined } },
    dataRootUsages: [{ root: current.allowedRoots[0]!, totalBytes: 100 * gib, availableBytes: 50 * gib }, { root: current.allowedRoots[1]!, totalBytes: 100 * gib, availableBytes: 50 * gib }],
  }
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  await expect(environment.getByText('CPU 物理核', { exact: true }).locator('..')).toHaveText('CPU 物理核尚未取得')
  await expect(environment.getByText('CPU 逻辑核', { exact: true }).locator('..')).toHaveText('CPU 逻辑核8 核')
  await expect(disks.getByRole('listitem')).toHaveText(['E:\\synthetic-exports · 100.0 GiB', 'F:\\synthetic-exports · 100.0 GiB'])
  await expect(disks).not.toContainText('200.0 GiB')
  current = { ...current, agentFacts: undefined, dataRootUsages: undefined, agentDiagnostics: { ...current.agentDiagnostics!, host: undefined } }
  await page.getByRole('tab', { name: '运行概览', exact: true }).click()
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('正在等待 Agent 返回检查结果；可稍后点击「刷新」查看。', { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: '环境与配置', exact: true }).click()
  await expect(page.getByText('Agent 尚未上报', { exact: true })).toBeVisible()
  await expect(environment.locator('.node-definition > div')).toHaveCount(9)
  await expect(environment.getByText('处理器型号', { exact: true }).locator('..')).toHaveText('处理器型号尚未取得')
  for (const label of ['CPU 物理核', 'CPU 逻辑核', '内存大小']) await expect(environment.getByText(label, { exact: true }).locator('..')).toHaveText(label + '尚未取得')
  await expect(disks.getByRole('listitem')).toHaveText(['E:\\synthetic-exports · 尚未取得', 'F:\\synthetic-exports · 尚未取得'])
  expect(errors).toEqual([])
})

test('环境检查按必需项说明阻断，版本与权限有处理建议，可选项不冒充阻断原因', async ({ page }, info) => {
  await mockFacts(page)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  const baseline = fullInspection()
  const inspection: NodeRuntimeInspection = { ...baseline, items: baseline.items.map(item => item.kind === 'JAVA_RUNTIME' ? { ...item, status: 'FAILED', code: 'VERSION_UNSUPPORTED', version: '17.0.1' } : item.kind === 'WORKSPACE' ? { ...item, status: 'FAILED', code: 'NOT_WRITABLE', writable: false } : item.kind === 'OBLOADER_RUNTIME' ? { ...item, status: 'UNKNOWN', code: 'VERSION_PROBE_TIMEOUT', version: undefined } : item) }
  const current = detail('synthetic-detail', { managementState: 'DISABLED', environmentStatus: 'ABNORMAL', acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED', 'TOOL_RUNTIME_INVALID'], environmentCheck: { ...detail().environmentCheck!, status: 'FAILED', code: 'TOOL_RUNTIME_INVALID', inspection } })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  await page.goto('/nodes/synthetic-detail')
  await expect(page).toHaveURL(/\/nodes\/synthetic-detail$/)
  const blocked = page.locator('.node-eligibility-alert')
  await expect(blocked).toContainText('Java 环境、工作目录未通过')
  await expect(blocked).not.toContainText('导入工具')
  const rows = page.locator('.node-inspection-items li')
  const java = rows.filter({ has: page.getByText('Java 环境', { exact: true }) })
  const workspace = rows.filter({ has: page.getByText('工作目录', { exact: true }) })
  const loader = rows.filter({ hasText: '导入工具（可选）' })
  await expect(java).toContainText('检测到版本 17.0.1，当前要求 Java 8。')
  await expect(java).toContainText('在「编辑节点」中选择 Java 8 的程序位置')
  await expect(java.getByText('未通过', { exact: true })).toBeVisible()
  await expect(workspace).toContainText('创建或清理临时文件')
  await expect(workspace).toContainText('写入和删除权限')
  await expect(loader.getByText('未确认', { exact: true })).toBeVisible()
  await expect(loader).toContainText('可选项，不影响当前导出环境检查。')
  await expect(page.getByText('E:\\synthetic-runtime\\JAVA_RUNTIME', { exact: true })).toHaveCount(0)
  await expect(page.getByText('VERSION_UNSUPPORTED', { exact: true })).toHaveCount(0)
  await expect(page.locator('.ant-collapse, details')).toHaveCount(0)
  await page.setViewportSize({ width: 1440, height: 1024 })
  await blocked.screenshot({ path: info.outputPath('environment-blockers-1440.png') })
  await page.locator('section[aria-labelledby="node-inspection-title"]').screenshot({ path: info.outputPath('environment-check-errors-1440.png') })
  await page.setViewportSize({ width: 640, height: 720 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true)
  await expect(workspace).toContainText('写入和删除权限')
  await page.screenshot({ path: info.outputPath('environment-check-errors-640.png'), fullPage: true })
  await page.evaluate(() => { document.body.style.zoom = '2' })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true)
  await expect(workspace).toContainText('写入和删除权限')
  expect(errors).toEqual([])
})

test('未执行版本查询、超时和无有效输出分别解释，旧检查明细不作为当前失败原因', async ({ page }, info) => {
  await mockFacts(page)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  const baseline = fullInspection()
  const inspection: NodeRuntimeInspection = { ...baseline, versionProbeAllowed: false, items: baseline.items.map(item => ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'OBLOADER_RUNTIME'].includes(item.kind) ? { ...item, status: 'UNKNOWN', code: 'VERSION_PROBE_NOT_REQUESTED', version: undefined } : item) }
  let current = detail('synthetic-detail', { managementState: 'DISABLED', environmentStatus: 'ABNORMAL', acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED', 'TOOL_RUNTIME_UNAVAILABLE'], environmentCheck: { ...detail().environmentCheck!, status: 'FAILED', code: 'TOOL_RUNTIME_UNAVAILABLE', inspection } })
  await page.route('**/api/v1/execution-nodes/synthetic-detail', route => route.fulfill({ json: { item: current } }))
  await page.goto('/nodes/synthetic-detail')
  const blocked = page.locator('.node-eligibility-alert')
  await expect(blocked).toContainText('环境检查结果待确认：Java 环境、导出工具未取得结果')
  await expect(blocked).not.toContainText('暂无法核验')
  const rows = page.locator('.node-inspection-items li')
  const java = rows.filter({ has: page.getByText('Java 环境', { exact: true }) })
  const dumper = rows.filter({ has: page.getByText('导出工具', { exact: true }) })
  const loader = rows.filter({ hasText: '导入工具（可选）' })
  await expect(java).toContainText('本次没有执行版本检查，尚不能确认实际版本。')
  await expect(java).toContainText('若刚检查过，请确认控制面与 Agent 已同步更新后重试')
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.locator('section[aria-labelledby="node-inspection-title"]').screenshot({ path: info.outputPath('environment-check-unrequested-1440.png') })
  const queried: NodeRuntimeInspection = { ...inspection, versionProbeAllowed: true, items: inspection.items.map(item => item.kind === 'JAVA_RUNTIME' ? { ...item, code: 'VERSION_UNAVAILABLE' } : item.kind === 'OBDUMPER_RUNTIME' ? { ...item, code: 'VERSION_PROBE_TIMEOUT' } : item.kind === 'OBLOADER_RUNTIME' ? { ...item, code: 'VERSION_OUTPUT_INVALID' } : item) }
  current = { ...current, environmentCheck: { ...current.environmentCheck!, inspection: queried } }
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(dumper).toContainText('版本查询超过等待时间，尚未取得结果。')
  await expect(loader).toContainText('没有可确认的唯一版本号')
  await expect(java).not.toContainText('没有执行版本检查')
  current = { ...current, unavailableReasons: ['NODE_DISABLED', 'ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { ...current.environmentCheck!, checkId: 'synthetic-next-check', status: 'PENDING', code: undefined, completedAt: undefined } }
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('正在展示上次检查结果', { exact: true })).toBeVisible()
  await expect(blocked).toContainText('正在等待 Agent 返回结果')
  await expect(blocked).not.toContainText('Java 环境、导出工具未取得结果')
  current = { ...current, environmentCheck: { ...current.environmentCheck!, status: 'FAILED', code: 'TOOL_RUNTIME_UNAVAILABLE', inspection: undefined }, unavailableReasons: ['NODE_DISABLED', 'TOOL_RUNTIME_UNAVAILABLE'] }
  await page.getByRole('button', { name: '刷新节点详情' }).click()
  await expect(page.getByText('本次检查没有返回逐项结果。请确认控制面与 Agent 已同步更新后重新检查。', { exact: true })).toBeVisible()
  expect(errors).toEqual([])
})
