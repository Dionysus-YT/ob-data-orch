import { chromium, expect, test, type Page } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import { writeFile } from 'node:fs/promises'
import { mockFacts } from './fixtures/facts'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
import { normalImportNode, normalImportSources } from './fixtures/normal-import'
import { monitorNativeWindowErrors } from './fixtures/native-window-errors'

const diagnostics = new WeakMap<Page, () => Promise<void>>()
test.afterEach(async ({ page }) => { await diagnostics.get(page)?.() })
const time = '2026-01-01T00:00:00Z'
const template = (index = 1) => ({ id: 'template-' + index, taskType: index % 2 ? 'OBDUMPER_EXPORT' : 'OBLOADER_NORMAL_IMPORT', displayName: '合成模板 ' + index, revision: 1, capabilityVersion: 'synthetic-capability', configFingerprint: 'synthetic-fingerprint', sourceTaskId: index === 1 ? undefined : 'synthetic-task-' + index, createdAt: time, updatedAt: time, availableActions: ['CREATE_DRAFT', 'RENAME', 'DELETE'] })
const source = DATA_SOURCE_UI_FIXTURES.find(item => item.state === 'ENABLED' && item.lastTestStatus === 'SUCCEEDED' && item.connectionKind === 'ODP')!

async function fixture(page: Page, items = [template()]) {
  const nativeErrors = await monitorNativeWindowErrors(page)
  const consoleErrors: string[] = [], warnings: string[] = []
  page.on('console', message => {
    // 仅识别隔离用例主动注入的 API HTTP 失败；资产、框架和业务脚本错误均保留。
    const expectedHttp = message.location().url.includes('/api/v1/') && message.text().startsWith('Failed to load resource:')
    if (message.type() === 'error' && !expectedHttp) consoleErrors.push(message.text())
    if (message.type() === 'warning') warnings.push(message.text())
  })
  diagnostics.set(page, async () => {
    await nativeErrors()
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    expect(consoleErrors, '模板页面出现非预期控制台错误').toEqual([])
    await test.info().attach('console-health', { body: JSON.stringify({ errors: consoleErrors, warnings }), contentType: 'application/json' })
  })
  await mockFacts(page)
  await page.route('**/api/v1/config-templates', route => route.fulfill({ json: { items } }))
  await page.route('**/api/v1/data-sources', route => route.fulfill({ json: { items: [source], total: 1, nextCursor: '' } }))
  await page.route('**/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT', route => route.fulfill({ json: { items: [{ id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64' }] } }))
}
async function useFirst(page: Page) { await page.getByRole('button', { name: '使用模板 合成模板 1', exact: true }).click(); await expect(page.getByRole('dialog', { name: '使用模板', exact: true })).toBeVisible() }
async function bind(page: Page) {
  const dialog = page.getByRole('dialog', { name: '使用模板', exact: true })
  await dialog.getByRole('combobox', { name: /数据源/ }).focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText(source.displayName, { exact: true }).last().click()
  await dialog.getByRole('combobox', { name: /执行节点/ }).focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText('合成节点 · Windows · x86_64', { exact: true }).last().click()
  await expect(dialog.getByRole('button', { name: '创建草稿', exact: true })).toBeEnabled()
}
async function more(page: Page, label: string) { await page.getByRole('button', { name: '更多操作 合成模板 1', exact: true }).click(); await page.getByRole('menuitem', { name: label, exact: true }).click() }

test('模板查找、类型、分页和缺失来源表达当前授权集合', async ({ page }, info) => {
  await fixture(page, Array.from({ length: 21 }, (_, index) => template(index + 1)))
  await page.goto('/templates')
  await expect(page).toHaveTitle(/模板中心/)
  await expect(page.getByRole('heading', { level: 1, name: '模板中心' })).toBeVisible()
  await expect(page.getByText('共 21 条', { exact: true })).toBeVisible()
  await expect(page.getByText('手工创建', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '下一页', exact: true }).click()
  await expect(page.getByText('第 2 页', { exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: '任务类型', exact: true }).focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText('普通导入', { exact: true }).last().click()
  await expect(page.getByText('共 10 条', { exact: true })).toBeVisible()
  await expect(page.getByText('第 1 页', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: '搜索模板名称', exact: true }).fill('模板 20')
  await expect(page.getByText('共 1 条', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('filtered.png'), fullPage: true })
  await page.getByRole('textbox', { name: '搜索模板名称', exact: true }).fill('不匹配')
  await expect(page.getByText('没有匹配的模板', { exact: true })).toBeVisible()
})

test('使用与管理资格分别裁剪，旧服务缺少资格保持只读', async ({ page }) => {
  const items = [{ ...template(1), availableActions: ['CREATE_DRAFT'] }, { ...template(2), availableActions: ['RENAME', 'DELETE'] }, { ...template(3), availableActions: [] }]
  await fixture(page, items); await page.goto('/templates')
  await expect(page.getByRole('button', { name: '使用模板 合成模板 1', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '更多操作 合成模板 1', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '使用模板 合成模板 2', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '更多操作 合成模板 2', exact: true }).click()
  await expect(page.getByRole('menuitem', { name: '改名', exact: true })).toBeVisible()
  await expect(page.getByText('仅可查看', { exact: true })).toBeVisible()
})

test('改名字节校验、失败保留与版本冲突不自动覆盖', async ({ page }, info) => {
  await fixture(page)
  let writes = 0
  await page.route('**/api/v1/config-templates/template-1', route => { writes++; return route.fulfill({ status: 412, json: { code: 'REVISION_CONFLICT' } }) })
  await page.goto('/templates'); await more(page, '改名')
  const dialog = page.getByRole('dialog', { name: '改名', exact: true })
  await dialog.getByRole('textbox', { name: '模板名称', exact: true }).fill('中'.repeat(86))
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByText('模板名称过长，请缩短后重试。', { exact: true })).toBeVisible()
  expect(writes).toBe(0)
  await dialog.getByRole('textbox', { name: '模板名称', exact: true }).fill('合成新名称')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByText('模板已变更', { exact: true })).toBeVisible()
  await expect(dialog.getByRole('textbox', { name: '模板名称', exact: true })).toHaveValue('合成新名称')
  await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeDisabled()
  await expect(dialog.getByRole('button', { name: '取消', exact: true })).toBeEnabled()
  await page.screenshot({ path: info.outputPath('rename-conflict.png'), fullPage: true })
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  expect(writes).toBe(1)
})

test('删除取消零请求，失败不移除，成功只移除模板', async ({ page }) => {
  await fixture(page); let deletes = 0
  await page.route('**/api/v1/config-templates/template-1', route => { deletes++; return deletes === 1 ? route.fulfill({ status: 503, json: { code: 'TEMPLATE_DELETE_UNAVAILABLE' } }) : route.fulfill({ status: 204 }) })
  await page.goto('/templates'); await more(page, '删除')
  await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click()
  expect(deletes).toBe(0)
  await more(page, '删除')
  await page.getByRole('dialog').getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByRole('dialog').getByText('删除未完成', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '模板列表' })).toContainText('合成模板 1')
  await page.getByRole('dialog').getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByText('暂无模板', { exact: true })).toBeVisible()
  expect(deletes).toBe(2)
})

test('创建结果未知冻结绑定，原键重试只进入同类型草稿', async ({ page }, info) => {
  await fixture(page); const writes: { key: string | undefined; body: unknown }[] = []
  await page.route('**/api/v1/config-templates/template-1:create-draft', route => {
    writes.push({ key: route.request().headers()['idempotency-key'], body: route.request().postDataJSON() })
    return writes.length === 1 ? route.fulfill({ status: 503, json: { code: 'TEMPLATE_DRAFT_CREATE_FAILED' } }) : route.fulfill({ json: { templateId: 'template-1', taskType: 'OBDUMPER_EXPORT', draftId: 'synthetic-created' } })
  })
  await page.goto('/templates'); await useFirst(page)
  await expect(page.getByRole('button', { name: '创建草稿', exact: true })).toBeDisabled()
  expect(writes).toHaveLength(0)
  await bind(page)
  await page.getByRole('button', { name: '创建草稿', exact: true }).click()
  await expect(page.getByRole('dialog').getByText('创建结果待确认', { exact: true })).toBeVisible()
  await expect(page.getByRole('combobox', { name: /数据源/ })).toBeDisabled()
  await page.screenshot({ path: info.outputPath('create-unknown.png'), fullPage: true })
  await page.getByRole('button', { name: '用原请求重试', exact: true }).click()
  await expect(page).toHaveURL(/\/exports\/new\?draft=synthetic-created&step=1$/)
  expect(writes).toHaveLength(2); expect(writes[0]).toEqual(writes[1])
})

test('抽屉键盘关闭回到原按钮，关闭不创建草稿', async ({ page }) => {
  await fixture(page); let writes = 0
  await page.route('**/api/v1/config-templates/*:create-draft', route => { writes++; return route.fulfill({ status: 500 }) })
  await page.goto('/templates'); await useFirst(page)
  for (let index = 0; index < 8; index++) { await page.keyboard.press('Tab'); expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.ant-drawer-content')))).toBe(true) }
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '使用模板 合成模板 1', exact: true })).toBeFocused()
  expect(writes).toBe(0)
})

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }, { width: 640, height: 720 }]) {
  test('模板列表与使用抽屉视口 ' + viewport.width, async ({ page }, info) => {
    await page.setViewportSize(viewport); await fixture(page, Array.from({ length: 12 }, (_, index) => template(index + 1)))
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
    await page.goto('/templates')
    await expect(page.getByRole('button', { name: '使用模板 合成模板 1', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath('list.png'), fullPage: false })
    await useFirst(page)
    await expect(page.getByRole('dialog').getByRole('button', { name: '创建草稿', exact: true })).toBeInViewport()
    await page.screenshot({ path: info.outputPath('drawer.png'), fullPage: false })
    expect(errors).toEqual([])
  })
}

test('模板列表和抽屉原生 200% 仍可达', async ({ browserName }, info) => {
  test.setTimeout(180000); expect(browserName).toBe('chromium')
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext('', { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', '--disable-extensions-except=' + extension, '--load-extension=' + extension] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage(); await fixture(page)
    await page.goto('http://127.0.0.1:15174/templates')
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const target = (await api.tabs.query({})).find(tab => tab.url.includes('15174/templates'))
      if (!target) throw new Error('隔离模板页面不存在')
      await api.tabs.setZoom(target.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await useFirst(page)
    await expect(page.getByRole('dialog').getByRole('button', { name: '创建草稿', exact: true })).toBeInViewport()
    const cdp = await context.newCDPSession(page)
    const capture = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('native-200.png'), Buffer.from(capture.data, 'base64'))
    await diagnostics.get(page)?.()
  } finally { await context.close() }
})

test('普通导入使用自身候选并进入独立向导', async ({ page }) => {
  await fixture(page, [template(2)])
  await page.route('**/api/v1/data-sources', route => route.fulfill({ json: { items: normalImportSources, total: normalImportSources.length, nextCursor: '' } }))
  await page.route('**/api/v1/execution-nodes', route => route.fulfill({ json: { items: [normalImportNode] } }))
  let writes = 0
  await page.route('**/api/v1/config-templates/template-2:create-draft', route => { writes++; expect(route.request().postDataJSON()).toEqual({ dataSourceId: normalImportSources[0]!.id, nodeId: normalImportNode.id }); return route.fulfill({ json: { templateId: 'template-2', taskType: 'OBLOADER_NORMAL_IMPORT', draftId: 'synthetic-normal-draft' } }) })
  await page.goto('/templates')
  await page.getByRole('button', { name: '使用模板 合成模板 2', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '使用模板', exact: true })
  await dialog.getByRole('combobox', { name: /数据源/ }).focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText(normalImportSources[0]!.displayName, { exact: true }).last().click()
  await dialog.getByRole('combobox', { name: /执行节点/ }).focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText('合成导入节点 · Windows · x86_64', { exact: true }).last().click()
  await dialog.getByRole('button', { name: '创建草稿', exact: true }).click()
  await expect(page).toHaveURL(/\/imports\/normal\/new\?draft=synthetic-normal-draft&step=1$/)
  expect(writes).toBe(1)
})

test('首次错误、可信空集合和撤权状态不混淆', async ({ page }) => {
  await fixture(page)
  let status = 503, items: ReturnType<typeof template>[] = []
  await page.route('**/api/v1/config-templates', route => route.fulfill({ status, json: status === 200 ? { items } : { code: 'SYNTHETIC_UNAVAILABLE' } }))
  await page.goto('/templates')
  await expect(page.getByText('模板读取未完成', { exact: true })).toBeVisible()
  await expect(page.getByText('暂无模板', { exact: true })).toHaveCount(0)
  status = 200
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(page.getByText('暂无模板', { exact: true })).toBeVisible()
  items = [template()]
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByRole('button', { name: '使用模板 合成模板 1', exact: true })).toBeVisible()
  status = 403
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByText('无法访问模板', { exact: true })).toBeVisible()
  await expect(page.getByText('暂无模板', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('region', { name: '模板列表', exact: true })).toHaveCount(0)
})

test('与数据源参考页比较工作面及控件密度', async ({ page }, info) => {
  await page.setViewportSize({ width: 1920, height: 1080 }); await fixture(page)
  await page.goto('/data-sources?uiFixture=data-sources')
  await expect(page.getByRole('heading', { level: 1, name: '数据源管理' })).toBeVisible()
  await page.screenshot({ path: info.outputPath('data-sources-reference.png'), fullPage: true })
  const reference = await page.locator('.ant-table-tbody tr').first().boundingBox()
  await page.goto('/templates')
  await expect(page.getByRole('button', { name: '使用模板 合成模板 1', exact: true })).toBeVisible()
  const rendered = await page.locator('.ant-table-tbody tr').first().boundingBox()
  await info.attach('row-density', { body: JSON.stringify({ reference, rendered }), contentType: 'application/json' })
  await page.screenshot({ path: info.outputPath('template-reference.png'), fullPage: true })
})

test('长中文名称与来源标识在桌面和窄屏可完整访问', async ({ page }, info) => {
  const displayName = '合成名称'.repeat(20), sourceTaskId = 'synthetic-' + 'source'.repeat(19)
  await fixture(page, [{ ...template(), displayName, sourceTaskId }])
  for (const width of [1280, 640]) {
    await page.setViewportSize({ width, height: 720 })
    await page.goto('/templates')
    const name = page.locator('.template-name strong')
    await expect(name).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await name.focus()
    await expect(page.getByRole('tooltip', { name: displayName, exact: true })).toHaveText(displayName)
    await page.getByRole('link', { name: sourceTaskId, exact: true }).focus()
    await expect(page.getByRole('tooltip', { name: sourceTaskId, exact: true })).toHaveText(sourceTaskId)
    await page.screenshot({ path: info.outputPath('long-values-' + width + '.png'), fullPage: true })
  }
})
