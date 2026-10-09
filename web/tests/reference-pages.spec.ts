import { expect, test, type Page } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { mockFacts } from './fixtures/facts'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
import type { GeneralizedExportConfig } from '../src/api/browser'

async function chooseDatabase(page: Page, name = 'finance_reporting') {
  await page.getByRole('button', { name: '展开数据库列表' }).click()
  await page.getByText(name, { exact: true }).last().click()
}

function exportSelect(page: Page, label: string) {
  return page.locator('.ant-select').filter({ has: page.getByRole('combobox', { name: label === '单个文件上限' ? /单个文件拆分阈值/ : label }) })
}

async function chooseExportOption(page: Page, label: string, option: string) {
  await exportSelect(page, label).click()
  await page.locator('.ant-select-dropdown:visible').getByText(option, { exact: true }).click()
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
}

function catalogGroups(names: Partial<Record<'TABLE' | 'VIEW' | 'FUNCTION' | 'PROCEDURE' | 'SEQUENCE', string[]>>) {
  return (['TABLE', 'VIEW', 'FUNCTION', 'PROCEDURE', 'SEQUENCE'] as const).map((objectType) => ({ objectType, objects: names[objectType] ?? [], truncated: false, unavailable: false }))
}

async function mockExportCatalog(page: Page, names: Partial<Record<'TABLE' | 'VIEW' | 'FUNCTION' | 'PROCEDURE' | 'SEQUENCE', string[]>>) {
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    await route.fulfill({ json: { item: {
      id: 'synthetic-catalog', status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
      ...input, objects: input.objectType === 'DATABASE' ? ['finance_reporting'] : [],
      groups: input.objectType === 'ALL' ? catalogGroups(names) : [],
      truncated: false, validUntil: '2099-01-01T00:00:00Z',
    } } })
  })
}

async function selectSyntheticTable(page: Page) {
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  const expand = candidates.getByRole('button', { name: '展开表分类' })
  if (await expand.count()) await expand.click()
  await candidates.getByRole('checkbox', { name: 'synthetic_table' }).check()
}


test('数据源迁移保留分页、编辑焦点和独立 Save/Test', async ({ page }, info) => {
  const unexpected: string[] = []
  page.on('pageerror', (error) => unexpected.push(error.message))
  await page.route('**/api/v1/**', (route) => route.abort())
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/data-sources?uiFixture=data-sources')
  await expect(page.getByRole('heading', { name: '数据源管理' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: '数据源分页' })).toBeVisible()
  await expect(page.getByRole('button', { name: '上一页', exact: true })).toBeDisabled()
  await page.screenshot({ path: info.outputPath('data-sources.png'), fullPage: true })
  await page.getByRole('button', { name: '新建数据源', exact: true }).click()
  await page.getByRole('menuitem', { name: 'OceanBase MySQL', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '新建数据源', exact: true })
  await expect(drawer).toBeVisible()
  await drawer.getByRole('button', { name: '测试连接', exact: true }).click()
  await expect(drawer.getByRole('button', { name: '开始测试', exact: true })).toBeDisabled()
  await expect(drawer.getByText('请先保存配置，再独立测试连接。', { exact: true })).toBeVisible()
  await drawer.getByRole('textbox', { name: '主机 IP/域名' }).fill('synthetic.example.invalid')
  await page.keyboard.press('Escape')
  const confirm = page.getByRole('dialog', { name: '放弃未保存的更改？' })
  await expect(confirm).toBeVisible()
  await expect(confirm.getByRole('button', { name: '继续编辑' })).toBeFocused()
  await confirm.getByRole('button', { name: '继续编辑' }).click()
  await expect(drawer).toBeVisible()
  await expect(drawer.getByRole('textbox', { name: '主机 IP/域名' })).toHaveValue('synthetic.example.invalid')
  await page.screenshot({ path: info.outputPath('source-editor.png'), fullPage: true })
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '放弃更改', exact: true }).click()
  await expect(drawer).not.toBeVisible()
  expect(unexpected).toEqual([])
})

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`四代表页在 ${viewport.width}×${viewport.height} 保持可达和无整页横滚`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    await page.setViewportSize(viewport)
    await mockFacts(page)
    for (const [path, heading] of [['/data-sources?uiFixture=data-sources', '数据源管理'], ['/exports/new', '新建导出任务'], ['/', '首页'], ['/tasks/synthetic-task', '任务详情']]) {
      await page.goto(path!)
      await expect(page.getByRole('heading', { name: heading!, exact: true })).toBeVisible()
      if (path === '/tasks/synthetic-task') {
        await expect(page.getByRole('heading', { name: '冻结配置', exact: true })).toBeVisible()
        await expect(page.getByText('synthetic.fixture', { exact: true })).toBeVisible()
        await expect(page.getByText('obdumper --csv', { exact: true })).toBeVisible()
        await expect(page.getByRole('textbox')).toHaveCount(0)
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await page.screenshot({ path: info.outputPath(`${heading}.png`), fullPage: true })
    }
    expect(errors).toEqual([])
  })
}

test('导出单选、步骤和摘要不改变草稿语义', async ({ page }, info) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table', 'synthetic_archive'] })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await expect(page.getByRole('heading', { name: '导出内容', level: 3, exact: true })).toBeVisible()
  const progress = await page.locator('.export-ant-step-rail').boundingBox()
  const workspace = await page.locator('.orch-task-workspace').boundingBox()
  expect(progress && workspace && progress.y + progress.height <= workspace.y).toBeTruthy()
  await page.screenshot({ path: info.outputPath('export-content.png'), fullPage: true })
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await expect(page.getByRole('heading', { name: '导出对象', exact: true })).toHaveCount(0)
  await expect(page.getByText('选择数据库后显示导出对象')).toBeVisible()
  await chooseDatabase(page)
  await expect(page.getByRole('heading', { name: '导出对象', exact: true })).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: '已选 0 项' })).toBeVisible()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await candidates.getByRole('button', { name: '展开表分类' }).click()
  await candidates.getByRole('checkbox', { name: '选择全部可见表' }).check()
  await expect(page.getByRole('status').filter({ hasText: '已选 2 项' })).toBeVisible()
  await page.getByRole('textbox', { name: '搜索候选对象' }).fill('archive')
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_archive' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_table' })).toHaveCount(0)
  await candidates.getByRole('checkbox', { name: 'synthetic_archive' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_archive' })).not.toBeChecked()
  await page.getByRole('textbox', { name: '搜索候选对象' }).fill('')
  await page.getByRole('textbox', { name: '搜索已选对象' }).fill('missing')
  await expect(page.getByText('没有匹配的已选对象')).toBeVisible()
  await page.getByRole('textbox', { name: '搜索已选对象' }).fill('')
  await page.getByRole('radiogroup', { name: '导出范围' }).getByRole('radio', { name: '整库导出' }).check()
  await expect(page.getByRole('heading', { name: '导出对象', exact: true })).toHaveCount(0)
  await page.getByRole('radiogroup', { name: '导出范围' }).getByRole('radio', { name: '部分导出' }).check()
  await expect(page.getByRole('button', { name: '移除已选对象 synthetic_table' })).toBeVisible()
  await page.getByRole('button', { name: '清空', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_table' })).not.toBeChecked()
  await expect(page.getByRole('button', { name: '下一步：选择数据格式' })).toBeDisabled()
  await expect(page.getByText('请至少选择一个导出对象。')).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'synthetic_table' }).check()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await page.screenshot({ path: info.outputPath('export-objects.png'), fullPage: true })
  await page.locator('.export-object-workspace').screenshot({ path: info.outputPath('export-object-selector.png') })
  await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '任务摘要' })
  await expect(drawer).toBeVisible()
  await expect(drawer.getByText('Production finance reporting')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(drawer).not.toBeVisible()
  await expect(page.getByRole('button', { name: '查看任务摘要', exact: true })).toBeFocused()
  await page.getByRole('button', { name: /选择数据源 已完成/ }).click()
  await expect(page.getByRole('heading', { name: '筛选数据源' })).toBeVisible()
  await expect(page.getByRole('radio', { name: /Production finance reporting/ })).toBeChecked()
})

test('部分导出从节点元数据加载对象并清除跨库旧选择', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  await mockFacts(page)
  const observed: Array<{ database: string; nodeId: string; objectType: string }> = []
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { database: string; nodeId: string; objectType: string; keyword: string }
    observed.push({ database: input.database, nodeId: input.nodeId, objectType: input.objectType })
    await route.fulfill({ json: { item: {
      id: `catalog-${observed.length}`, status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
      nodeId: input.nodeId, database: input.database, objectType: input.objectType, keyword: input.keyword,
      objects: input.objectType === 'DATABASE' ? ['finance_reporting', 'other_db'] : [],
      groups: input.objectType === 'ALL' ? catalogGroups({ TABLE: input.database === 'finance_reporting' ? ['orders', 'users'] : ['fresh_table'] }) : [],
      truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('button', { name: '表（2）' })).toBeVisible()
  await candidates.getByRole('button', { name: '展开表分类' }).click()
  await expect(candidates.getByRole('checkbox', { name: 'orders' })).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'orders' }).check()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await page.getByRole('button', { name: '清空', exact: true }).click()
  await expect(candidates.getByRole('checkbox', { name: 'orders' })).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'orders' }).check()
  await chooseDatabase(page, 'other_db')
  await expect(candidates.getByRole('button', { name: '表（1）' })).toBeVisible()
  await candidates.getByRole('button', { name: '展开表分类' }).click()
  await expect(candidates.getByRole('checkbox', { name: 'fresh_table' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'orders' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
  expect(observed.filter(({ objectType }) => objectType === 'ALL').map(({ database }) => database)).toEqual(['finance_reporting', 'other_db'])
  await candidates.getByRole('checkbox', { name: 'fresh_table' }).check()
  await page.getByRole('button', { name: '下一步：选择数据格式' }).click()
  await page.getByRole('button', { name: '下一步：执行与输出' }).click()
  await expect(page.getByRole('button', { name: '更换节点' })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '执行节点', exact: true })).toHaveCount(0)
  const nodeFacts = page.getByRole('group', { name: '执行节点信息' })
  await expect(nodeFacts.getByText('合成执行节点', { exact: true })).toBeVisible()
  await expect(nodeFacts.getByText('平台 · WINDOWS_AMD64', { exact: true })).toBeVisible()
  await expect(page).toHaveURL(/step=4/)
  await expect(page).toHaveTitle(/OB Data Orch/)
  await expect(page.locator('vite-error-overlay')).toHaveCount(0)
  for (const width of [1440, 925, 720]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(nodeFacts.getByRole('button', { name: '更换节点' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath(`export-output-node-${width}.png`), fullPage: false })
  }
  await page.getByRole('button', { name: '更换节点' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  expect(errors).toEqual([])
})

test('步骤 2 刷新恢复同会话选择并重新读取目录，数据源修订变化清除旧数据库', async ({ page }) => {
  await mockFacts(page)
  let sourceRevision = 101
  const catalogRequests: string[] = []
  await page.route('**/api/v1/data-sources', (route) => route.fulfill({ json: {
    items: DATA_SOURCE_UI_FIXTURES.map((source) => source.id === 'ui-fixture-production-finance-reporting' ? { ...source, revision: sourceRevision } : source),
    total: 7, nextCursor: '',
  } }))
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    catalogRequests.push(input.objectType)
    await route.fulfill({ json: { item: {
      id: `catalog-${catalogRequests.length}`, status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
      ...input, objects: input.objectType === 'DATABASE' ? ['finance_reporting'] : [],
      groups: input.objectType === 'ALL' ? catalogGroups({ TABLE: ['synthetic_table'] }) : [],
      truncated: false, validUntil: '2099-01-01T00:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await chooseDatabase(page)
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('button', { name: '表（1）' })).toBeVisible()
  await candidates.getByRole('checkbox', { name: '选择全部可见表' }).check()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()

  await page.reload()
  await expect(page.locator('.export-field-body .ant-select').nth(1)).toContainText('finance_reporting')
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '表（1）' })).toBeVisible()
  await expect.poll(() => catalogRequests.filter((type) => type === 'DATABASE').length).toBeGreaterThanOrEqual(2)
  await expect.poll(() => catalogRequests.filter((type) => type === 'ALL').length).toBeGreaterThanOrEqual(2)

  await page.getByRole('button', { name: '下一步：选择数据格式' }).click()
  await page.reload()
  await expect(page.getByRole('heading', { name: '数据文件设置', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '下一步：执行与输出' }).click()
  await expect(page.getByRole('heading', { name: '输出设置' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: '输出设置' })).toBeVisible()
  await page.getByRole('button', { name: '上一步' }).click()
  await page.getByRole('button', { name: '上一步' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()

  sourceRevision = 102
  await page.reload()
  await expect(page.locator('.export-field-body .ant-select').nth(1)).toContainText('请选择数据库')
  await expect(page.getByText('选择数据库后显示导出对象')).toBeVisible()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toHaveCount(0)

  await chooseDatabase(page)
  await page.evaluate(() => {
    const state = window.history.state
    window.history.replaceState({ ...state, obDataOrchExportStep2Recovery: { ...state.obDataOrchExportStep2Recovery, sessionFingerprint: 'different-synthetic-session' } }, '')
  })
  await page.reload()
  await page.getByRole('button', { name: '上一步' }).click()
  await expect(page.getByRole('radio').first()).not.toBeChecked()
})

test('结果集 SQL 编辑器输入随步骤 2 刷新恢复', async ({ page }, info) => {
  const pageErrors: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))
  await mockFacts(page)
  await mockExportCatalog(page, {})
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.getByText('按结果集导出', { exact: true }).click()
  await chooseDatabase(page)

  const editor = page.locator('.sql-query-editor .monaco-editor')
  await expect(editor).toBeVisible({ timeout: 30000 })
  await editor.locator('.view-line').first().click()
  await page.keyboard.insertText('SELECT id FROM synthetic_table')
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql)).toBe('SELECT id FROM synthetic_table')
  await page.getByRole('button', { name: '撤销 SQL 编辑' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql)).not.toBe('SELECT id FROM synthetic_table')
  await page.getByRole('button', { name: '重做 SQL 编辑' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql)).toBe('SELECT id FROM synthetic_table')

  await page.reload()
  await expect(editor).toBeVisible({ timeout: 30000 })
  await expect(editor.locator('.view-line').first()).toContainText('SELECT id FROM synthetic_table')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('query-result-editor.png'), fullPage: true })
  expect(pageErrors).toEqual([])
})

test('结果集 SQL 工具栏提供本地编辑操作', async ({ page }, info) => {
  const pageErrors: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))
  await mockFacts(page)
  await mockExportCatalog(page, {})
  await page.setViewportSize({ width: 925, height: 884 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.getByText('按结果集导出', { exact: true }).click()
  await chooseDatabase(page)

  const editor = page.locator('.sql-query-editor .monaco-editor')
  await expect(editor).toBeVisible({ timeout: 30000 })
  await editor.locator('.view-line').first().click()
  await page.keyboard.insertText('select id, name from synthetic_table where id = 1')
  await page.getByRole('button', { name: '格式化 SQL' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toContain('\n')
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toMatch(/^SELECT/)
  await expect(page.getByText('已格式化 SQL。')).toBeVisible()

  const editorContainer = page.locator('.sql-query-editor')
  const closedHeight = await editorContainer.evaluate((element) => element.getBoundingClientRect().height)
  await expect(page.getByRole('toolbar', { name: 'SQL 编辑操作' }).getByRole('button', { name: /查找/ })).toHaveCount(1)
  await page.getByRole('button', { name: '查找与替换 SQL' }).click()
  await expect(editor.locator('.find-widget')).toBeVisible()
  await expect(editor.getByRole('textbox', { name: 'Replace', exact: true })).toBeVisible()
  await expect.poll(() => editorContainer.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(closedHeight + 64)
  await expect.poll(() => page.evaluate(async () => { await document.fonts.ready; return document.fonts.check('16px codicon') })).toBe(true)
  const findInput = editor.getByRole('textbox', { name: 'Find', exact: true })
  await findInput.fill('中文字段')
  await expect(findInput).toHaveValue('中文字段')
  await editor.locator('.find-widget').screenshot({ path: info.outputPath('query-result-find.png') })
  await page.locator('.sql-query-editor-viewport').screenshot({ path: info.outputPath('query-result-find-layout.png') })
  await page.keyboard.press('Escape')
  await expect.poll(() => editorContainer.evaluate((element) => element.getBoundingClientRect().height)).toBe(closedHeight)

  await editor.locator('.view-line').first().click()
  await page.keyboard.press('Control+A')
  await page.getByRole('button', { name: '转换 SQL 大小写' }).click()
  await page.getByRole('menuitem', { name: '全部小写' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toMatch(/^select/)

  const beforeIndent = await page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)
  await page.getByRole('button', { name: '调整 SQL 缩进' }).click()
  await page.getByRole('menuitem', { name: '添加缩进' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).not.toBe(beforeIndent)
  await page.getByRole('button', { name: '调整 SQL 缩进' }).click()
  await page.getByRole('menuitem', { name: '删除缩进' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toBe(beforeIndent)

  await page.getByRole('button', { name: '调整 SQL 注释' }).click()
  await page.getByRole('menuitem', { name: '添加注释' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toContain('--')
  await expect(page.getByRole('button', { name: '下一步：选择数据格式' })).toBeDisabled()
  await page.getByRole('button', { name: '调整 SQL 注释' }).click()
  await page.getByRole('menuitem', { name: '删除注释' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).not.toContain('--')
  await expect(page.getByRole('button', { name: '下一步：选择数据格式' })).toBeEnabled()

  await editor.locator('.view-line').first().click()
  await page.keyboard.press('Control+A')
  await page.keyboard.insertText('select * from synthetic_table where name in ')
  await page.getByRole('button', { name: '转换 SQL 大小写' }).click()
  await page.getByRole('menuitem', { name: '全部大写' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toBe('SELECT * FROM SYNTHETIC_TABLE WHERE NAME IN ')
  await expect(page.getByText('已转换全文。')).toBeVisible()
  await page.getByRole('button', { name: 'IN 值转换' }).click()
  await page.getByRole('textbox', { name: '待转换的 IN 值' }).fill("O'Reilly\nParis")
  await expect(page.getByText("('O''Reilly', 'Paris')", { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '插入到 SQL' }).click()
  await expect.poll(() => page.evaluate(() => window.history.state?.obDataOrchExportStep2Recovery?.querySql as string)).toBe("SELECT * FROM SYNTHETIC_TABLE WHERE NAME IN ('O''Reilly', 'Paris')")
  await page.getByRole('button', { name: 'IN 值转换' }).click()
  await page.getByRole('textbox', { name: '待转换的 IN 值' }).fill('1\nnot-a-number')
  await page.getByRole('radio', { name: '数值' }).check()
  await page.getByRole('button', { name: '插入到 SQL' }).click()
  await expect(page.getByText('数值类型只能包含整数、小数或科学计数法。')).toBeVisible()
  await page.getByRole('button', { name: '取消' }).last().click()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  expect(pageErrors).toEqual([])
  await page.getByRole('toolbar', { name: 'SQL 编辑操作' }).screenshot({ path: info.outputPath('query-result-toolbar-icons.png') })
  await page.screenshot({ path: info.outputPath('query-result-toolbar.png'), fullPage: true })
})

test('旧控制面拒绝数据库目录时提示版本不匹配且下拉保留手动入口', async ({ page }) => {
  await mockFacts(page)
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    if (input.objectType === 'DATABASE') {
      await route.fulfill({ status: 422, json: { code: 'EXPORT_OBJECT_CATALOG_FIELDS_INVALID', message: '对象查询条件无效' } })
      return
    }
    await route.fulfill({ json: { item: {
      id: 'catalog-objects', status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
      nodeId: input.nodeId, database: input.database, objectType: input.objectType, keyword: input.keyword,
      objects: [], groups: input.objectType === 'ALL' ? catalogGroups({ TABLE: ['orders'] }) : [], truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await expect(page.getByText('目录查询条件与当前控制面版本不兼容。请更新控制面与 Agent 后重新加载。')).toBeVisible()
  await page.getByRole('button', { name: '展开数据库列表' }).click()
  await expect(page.getByText('手动输入其他数据库 / Schema…')).toBeVisible()
})

test('导出对象按五类切换并保留跨分类选择', async ({ page }) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'], VIEW: ['synthetic_view'], FUNCTION: ['synthetic_function'] })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await chooseDatabase(page)
  await page.getByText('仅导出结构', { exact: true }).click()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('button', { name: '展开视图分类' })).toBeEnabled()
  await expect(candidates.getByRole('button', { name: '展开函数分类' })).toBeEnabled()
  await expect(candidates.getByRole('button', { name: '展开存储过程分类' })).toBeEnabled()
  await expect(candidates.getByRole('button', { name: '展开序列分类' })).toBeEnabled()
  await candidates.getByRole('button', { name: '展开表分类' }).click()
  await candidates.getByRole('checkbox', { name: 'synthetic_table' }).check()
  await candidates.getByRole('button', { name: '展开视图分类' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '收起视图分类' })).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'synthetic_view' }).check()
  await expect(page.getByRole('heading', { name: '已选 2 项' })).toBeVisible()
  await expect(page.getByRole('region', { name: '已选导出对象' }).getByText('表（1）')).toBeVisible()
  await expect(page.getByRole('region', { name: '已选导出对象' }).getByText('视图（1）')).toBeVisible()
  await page.getByRole('button', { name: '收起已选视图' }).click()
  await expect(page.getByRole('button', { name: '移除已选对象 synthetic_view' })).toHaveCount(0)
  await page.getByRole('button', { name: '展开已选视图' }).click()
  await page.getByRole('button', { name: '移除已选对象 synthetic_view' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await candidates.getByRole('button', { name: '展开函数分类' }).click()
  await candidates.getByRole('checkbox', { name: 'synthetic_function' }).check()
  await expect(page.getByRole('region', { name: '已选导出对象' }).getByText('函数（1）')).toBeVisible()
  await expect(page.getByRole('heading', { name: '已选 2 项' })).toBeVisible()
})

test('Ant 步骤显示当前进度，分类方框直接勾选且图标尺寸一致', async ({ page }, info) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'], VIEW: ['synthetic_view'], FUNCTION: ['synthetic_function'], PROCEDURE: ['synthetic_procedure'], SEQUENCE: ['synthetic_sequence'] })
  await page.setViewportSize({ width: 925, height: 884 })
  await page.goto('/exports/new')
  const rail = page.locator('.export-ant-step-rail')
  await expect(rail).toHaveClass(/ant-steps-with-progress/)
  await expect(rail.getByText('当前步骤 · 0%')).toBeVisible()
  await page.getByRole('radio').first().check()
  await expect(rail.getByText('当前步骤 · 100%')).toBeVisible()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await expect(rail.locator('.ant-steps-item').first()).toHaveClass(/ant-steps-item-finish/)
  const [progressRing, currentTitle] = await Promise.all([
    rail.locator('.ant-steps-item-process .ant-progress').boundingBox(),
    rail.locator('.ant-steps-item-process .ant-steps-item-title').boundingBox(),
  ])
  expect(progressRing && currentTitle && progressRing.x + progressRing.width < currentTitle.x).toBe(true)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await chooseDatabase(page)
  await page.getByText('仅导出结构', { exact: true }).click()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('button', { name: '视图（1）' })).toBeVisible()
  const viewCheckbox = candidates.getByRole('checkbox', { name: '选择全部视图' })
  await viewCheckbox.check()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '展开视图分类' })).toBeVisible()
  await expect(rail.getByText('当前步骤 · 100%')).toBeVisible()
  const iconSizes = await candidates.locator('.export-object-tree-category:not(.export-object-selected-category) > .export-object-kind-icon').evaluateAll((icons) => icons.map((icon) => {
    const rect = icon.getBoundingClientRect()
    return { width: rect.width, height: rect.height, fontSize: getComputedStyle(icon).fontSize }
  }))
  expect(iconSizes).toHaveLength(5)
  expect(iconSizes.every((size) => size.width === 16 && size.height === 16 && size.fontSize === '14px')).toBe(true)
  await page.screenshot({ path: info.outputPath('export-step-progress-and-object-icons.png'), fullPage: true })
  await viewCheckbox.uncheck()
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '展开视图分类' })).toBeVisible()
})

test('对象选择超过 100 项时框尺寸固定且内部滚动，分类图标沿用 ODC 语义', async ({ page }, info) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: Array.from({ length: 100 }, (_, index) => `table_${index}`), VIEW: ['view_100'] })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await chooseDatabase(page)
  await page.getByText('仅导出结构', { exact: true }).click()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  const selected = page.getByRole('region', { name: '已选导出对象' })
  const paneHeights = () => page.locator('.export-object-pane').evaluateAll((items) => items.map((item) => item.getBoundingClientRect().height))
  const initialHeights = await paneHeights()
  await expect(page.locator('.export-format-panel, .export-option-hint')).toHaveCount(0)
  await candidates.getByRole('button', { name: '展开表分类' }).click()
  await candidates.getByRole('checkbox', { name: '选择全部可见表' }).check()
  await expect(page.getByRole('heading', { name: '已选 100 项' })).toBeVisible()
  await expect.poll(paneHeights).toEqual(initialHeights)
  for (const label of ['候选对象滚动区', '已选对象滚动区']) {
    const body = page.getByRole('region', { name: label, exact: true })
    await expect.poll(() => body.evaluate((item) => item.scrollHeight > item.clientHeight)).toBe(true)
    await body.focus()
    await page.keyboard.press('End')
    await expect.poll(() => body.evaluate((item) => item.scrollTop)).toBeGreaterThan(0)
    expect(await body.evaluate((item) => {
      const pane = item.parentElement!
      const header = pane.querySelector('.export-object-pane-heading')!
      return Math.abs(header.getBoundingClientRect().top - pane.getBoundingClientRect().top)
    })).toBeLessThan(1)
  }
  await candidates.getByRole('button', { name: '展开视图分类' }).click()
  await candidates.getByRole('checkbox', { name: 'view_100' }).check()
  await expect(page.getByRole('heading', { name: '已选 101 项' })).toBeVisible()
  await expect(candidates.locator('.export-object-kind-glyph')).toContainText(['fₓ', 'Pₓ', '¹²³'])
  await expect(page.getByText('手动添加候选对象', { exact: true })).toHaveCount(0)
  await selected.getByRole('button', { name: '移除已选对象 table_99', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 100 项' })).toBeVisible()
  await expect.poll(paneHeights).toEqual(initialHeights)
  await selected.getByRole('textbox', { name: '搜索已选对象' }).fill('view_100')
  await expect(selected.getByRole('button', { name: /^移除已选对象/ })).toHaveCount(1)
  await expect.poll(paneHeights).toEqual(initialHeights)
  await selected.getByRole('textbox', { name: '搜索已选对象' }).clear()
  await page.locator('.export-object-workspace').screenshot({ path: info.outputPath('objects-fixed-scroll.png') })
  await page.setViewportSize({ width: 720, height: 900 })
  await expect.poll(paneHeights).toEqual(initialHeights)
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.locator('.export-object-workspace').screenshot({ path: info.outputPath('objects-fixed-scroll-narrow.png') })
  await selected.getByRole('button', { name: '清空', exact: true }).click()
  await expect(selected.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
  await expect.poll(paneHeights).toEqual(initialHeights)
})

test('五万对象完整加载，虚拟列表控制 DOM，尾部搜索和全选使用完整集合', async ({ page }, info) => {
  await mockFacts(page)
  const types = ['TABLE', 'VIEW', 'FUNCTION', 'PROCEDURE', 'SEQUENCE'] as const
  const names = Object.fromEntries(types.map(type => [type, Array.from({ length: 10000 }, (_, index) => `${type}_${index}`)]))
  let requests = 0
  await page.route('**/api/v1/data-sources/*:search-export-objects', async route => {
    const input = route.request().postDataJSON() as { objectType: string }
    requests++
    await route.fulfill({ json: { item: { id: 'large-catalog', status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting', ...input,
      objects: input.objectType === 'DATABASE' ? ['finance_reporting'] : input.objectType === 'ALL' ? [] : names[input.objectType],
      groups: input.objectType === 'ALL' ? catalogGroups(names) : [], truncated: false, validUntil: '2099-01-01T00:00:00Z' } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  const started = Date.now()
  await chooseDatabase(page)
  await page.getByText('仅导出结构', { exact: true }).click()
  await expect(page.getByRole('heading', { name: '选择对象(50000)' })).toBeVisible()
  const loadMs = Date.now() - started
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await candidates.getByRole('button', { name: '展开表分类' }).click()
  await expect.poll(() => candidates.locator('.ant-list-item').count()).toBeLessThan(40)
  const candidateRowsWhenExpanded = await candidates.locator('.ant-list-item').count()
  const scroll = page.getByRole('region', { name: '候选对象滚动区', exact: true })
  await scroll.evaluate(element => { element.scrollTop = element.scrollHeight - element.clientHeight - 120 })
  await expect(candidates.getByRole('checkbox', { name: 'TABLE_9999', exact: true })).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'TABLE_9999', exact: true }).check()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await page.getByRole('textbox', { name: '搜索候选对象' }).fill('TABLE_9999')
  await expect(candidates.getByRole('checkbox', { name: 'TABLE_9999', exact: true })).toBeChecked()
  await page.getByRole('button', { name: '搜索或刷新对象' }).click()
  await expect.poll(() => requests).toBe(3)
  await page.getByRole('textbox', { name: '搜索候选对象' }).clear()
  const selectionStarted = Date.now()
  await scroll.evaluate(element => { element.scrollTop = 0 })
  await candidates.getByRole('checkbox', { name: '选择全部可见表', exact: true }).check()
  await expect(page.getByRole('heading', { name: '已选 10000 项' })).toBeVisible()
  const selectMs = Date.now() - selectionStarted
  const selected = page.getByRole('region', { name: '已选导出对象' })
  await expect.poll(() => selected.locator('.ant-list-item').count()).toBeLessThan(40)
  await selected.getByRole('textbox', { name: '搜索已选对象' }).fill('TABLE_9999')
  await selected.getByRole('button', { name: '移除已选对象 TABLE_9999', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 9999 项' })).toBeVisible()
  await selected.getByRole('textbox', { name: '搜索已选对象' }).clear()
  await page.getByRole('button', { name: '下一步：选择数据格式' }).click()
  await page.getByRole('button', { name: '上一步', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 9999 项' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: '已选 9999 项' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '选择对象(50000)' })).toBeVisible()
  const batchSelectionStarted = Date.now()
  await candidates.getByRole('checkbox', { name: '选择全部可见表', exact: true }).check()
  for (const label of ['视图', '函数', '存储过程', '序列']) await candidates.getByRole('checkbox', { name: `选择全部${label}`, exact: true }).check()
  await expect(page.getByRole('heading', { name: '已选 50000 项' })).toBeVisible()
  const batchSelectMs = Date.now() - batchSelectionStarted
  const selectedScroll = page.getByRole('region', { name: '已选对象滚动区', exact: true })
  await selectedScroll.evaluate(element => { element.scrollTop = element.scrollHeight })
  await expect(selected.getByRole('button', { name: '移除已选对象 SEQUENCE_9999', exact: true })).toBeVisible()
  await selected.getByRole('button', { name: '移除已选对象 SEQUENCE_9999', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 49999 项' })).toBeVisible()
  await expect.poll(() => selected.locator('.ant-list-item').count()).toBeLessThan(40)
  await page.setViewportSize({ width: 720, height: 900 })
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  const metricsPath = info.outputPath('catalog-performance.json')
  await writeFile(metricsPath, JSON.stringify({ objects: 50000, selected: 49999, loadMs, selectMs, batchSelectMs, candidateRowsWhenExpanded, renderedCandidateRows: await candidates.locator('.ant-list-item').count(), renderedSelectedRows: await selected.locator('.ant-list-item').count() }, null, 2))
  await info.attach('catalog-performance', { path: metricsPath, contentType: 'application/json' })
  await page.screenshot({ path: info.outputPath('large-catalog.png'), fullPage: true })
})

test('五类对象从固定元数据目录加载并可同时勾选', async ({ page }) => {
  await mockFacts(page)
  const names: Record<string, string> = { TABLE: 'table_fixture', VIEW: 'view_fixture', FUNCTION: 'fn_fixture', PROCEDURE: 'proc_fixture', SEQUENCE: 'seq_fixture' }
  const labels: Record<string, string> = { TABLE: '表', VIEW: '视图', FUNCTION: '函数', PROCEDURE: '存储过程', SEQUENCE: '序列' }
  const requested: string[] = []
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    requested.push(input.objectType)
    await route.fulfill({ json: { item: {
      id: `catalog-${requested.length}`, status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
      nodeId: input.nodeId, database: input.database, objectType: input.objectType, keyword: input.keyword,
      objects: input.objectType === 'DATABASE' ? ['finance_reporting'] : input.objectType === 'ALL' ? [] : [names[input.objectType]],
      groups: input.objectType === 'ALL' ? catalogGroups({ TABLE: [names.TABLE], VIEW: [names.VIEW], FUNCTION: [names.FUNCTION], PROCEDURE: [names.PROCEDURE], SEQUENCE: [names.SEQUENCE] }) : [],
      truncated: false, validUntil: '2099-01-01T00:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await chooseDatabase(page)
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect.poll(() => requested.filter((type) => type !== 'DATABASE').length).toBe(1)
  await expect(page.getByRole('heading', { name: '选择对象(1)' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '视图（1）' })).toHaveCount(0)
  await page.getByText('导出结构和数据', { exact: true }).click()
  await expect(candidates.getByRole('button', { name: '视图（1）' })).toBeVisible()
  await page.getByText('仅导出数据', { exact: true }).click()
  await expect(candidates.getByRole('button', { name: '视图（1）' })).toHaveCount(0)
  await page.getByText('仅导出结构', { exact: true }).click()
  for (const type of ['TABLE', 'VIEW', 'FUNCTION', 'PROCEDURE', 'SEQUENCE']) {
    await candidates.getByRole('button', { name: `展开${labels[type]}分类` }).click()
    await expect(candidates.getByRole('checkbox', { name: names[type] })).toBeVisible()
    await candidates.getByRole('checkbox', { name: names[type] }).check()
  }
  await expect(page.getByRole('heading', { name: '已选 5 项' })).toBeVisible()
  await expect(page.getByRole('radiogroup', { name: '导出内容' }).getByRole('radio', { name: '仅导出结构' })).toBeChecked()
  await page.getByText('导出结构和数据', { exact: true }).click()
  await expect(page.getByRole('radiogroup', { name: '导出内容' }).getByRole('radio', { name: '导出结构和数据' })).toBeChecked()
  await expect(page.getByRole('heading', { name: '已选 5 项' })).toBeVisible()
  await page.getByText('仅导出数据', { exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '展开表分类' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '展开序列分类' })).toHaveCount(0)
  await page.getByText('仅导出结构', { exact: true }).click()
  await expect(page.getByRole('heading', { name: '已选 5 项' })).toBeVisible()
  expect(requested.filter((type) => type !== 'DATABASE')).toEqual(['ALL'])
  await candidates.getByRole('button', { name: '展开序列分类' }).click()
  await page.getByRole('textbox', { name: '搜索候选对象' }).fill('seq_fixture')
  await page.getByRole('button', { name: '搜索或刷新对象' }).click()
  await expect.poll(() => requested.filter((type) => type === 'SEQUENCE').length).toBe(1)
  await page.getByRole('textbox', { name: '搜索候选对象' }).fill('')
  await expect(candidates.getByRole('checkbox', { name: 'seq_fixture' })).toBeVisible()
})

test('对象目录接口返回 404 时提示版本或授权且不提供手动候选', async ({ page }, info) => {
  await mockFacts(page)
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    await route.fulfill({ status: 404, json: { error: { code: 'NOT_FOUND', message: '数据源不存在或当前身份无权访问。' } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await expect(page.getByRole('heading', { name: '导出内容', level: 3, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: '导出内容与对象', level: 2, exact: true })).toHaveCount(0)
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await expect(page.getByRole('region', { name: '选择导出对象' }).getByRole('alert').filter({ hasText: '对象目录接口不可用' })).toBeVisible()
  for (const width of [1440, 720]) {
    await page.setViewportSize({ width, height: 1024 })
    const dropdown = await page.locator('.export-database-select .ant-select').boundingBox()
    const manual = await page.getByRole('button', { name: '手动输入数据库', exact: true }).boundingBox()
    expect(dropdown && manual && manual.x >= dropdown.x + dropdown.width).toBeTruthy()
    expect(Math.abs(manual!.y - dropdown!.y)).toBeLessThan(2)
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath(`step2-manual-right-${width}.png`) })
  }
  await page.getByRole('button', { name: '手动输入数据库' }).click()
  await page.getByRole('textbox', { name: '手动输入数据库 / Schema' }).fill('manual_schema')
  await page.getByRole('button', { name: '使用此名称' }).click()
  await expect(page.locator('.export-field-body .ant-select').nth(1)).toContainText('manual_schema')
  await expect(page.getByText('手动添加候选对象', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
})

test('对象查询未被节点领取后停止轮询并提示重试', async ({ page }) => {
  await mockFacts(page)
  let requests = 0
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    if (input.objectType === 'DATABASE') {
      await route.fulfill({ json: { item: {
        id: 'database-catalog', status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
        ...input, objects: ['finance_reporting'], truncated: false, validUntil: '2026-09-30T12:00:00Z',
      } } })
      return
    }
    requests++
    await route.fulfill({ status: 202, json: { item: {
      id: 'catalog-pending', status: 'PENDING', dataSourceId: 'ui-fixture-production-finance-reporting',
      ...input, objects: [], truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.route('**/api/v1/export-object-catalog-queries/catalog-pending', async (route) => {
    await route.fulfill({ json: { item: {
      id: 'catalog-pending', status: 'EXPIRED', dataSourceId: 'ui-fixture-production-finance-reporting',
      nodeId: 'synthetic-node', database: 'finance_reporting', objectType: 'ALL', keyword: '',
      objects: [], truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await expect(page.getByRole('alert').filter({ hasText: '执行节点未及时完成对象查询' })).toBeVisible()
  await expect(page.getByText('手动添加候选对象', { exact: true })).toHaveCount(0)
  await expect.poll(() => requests).toBe(1)
})

test('导出列包含与排除保持互斥，空配置不能推进', async ({ page }) => {
  await mockFacts(page)
  await page.goto('/exports/new?step=2')
  await expect(page.getByRole('textbox', { name: '排除表' })).toHaveCount(0)
  await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
  await expect(page.getByRole('textbox', { name: '排除表' })).toBeVisible()
  const include = page.getByRole('textbox', { name: /包含列/ })
  const exclude = page.getByRole('textbox', { name: /排除列/ })
  await include.fill('synthetic_column')
  await expect(exclude).toBeDisabled()
  await include.fill('')
  await exclude.fill('synthetic_excluded_column')
  await expect(include).toBeDisabled()
  await expect(page.getByRole('button', { name: /^下一步：/ })).toBeDisabled()
  await expect(page.getByText('请选择已启用且基础连接测试成功的数据源。').first()).toBeVisible()
})

for (const mode of ['MYSQL', 'ORACLE'] as const) {
  test(`导出勾选参数按内容格式与租户显隐并保留 DDL 提交值 ${mode}`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await mockFacts(page)
    await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
    const source = { ...DATA_SOURCE_UI_FIXTURES[0]!, compatibilityMode: mode }
    await page.route('**/api/v1/data-sources', (route) => route.fulfill({ json: { items: [source], total: 1, nextCursor: '' } }))
    let saved: GeneralizedExportConfig | undefined
    await page.route('**/api/v1/export-drafts**', async (route) => {
      if (route.request().method() === 'POST' && new URL(route.request().url()).pathname === '/api/v1/export-drafts') {
        saved = (route.request().postDataJSON() as { config: GeneralizedExportConfig }).config
        await route.fulfill({ json: { id: 'synthetic-checkbox-draft' } })
      } else if (route.request().url().includes(':preview-command')) {
        await route.fulfill({ json: { command: 'obdumper --ddl --sql', configFingerprint: 'synthetic-checkbox-fingerprint' } })
      } else {
        await route.fulfill({ json: { item: { id: 'synthetic-checkbox-draft', dataSourceId: source.id, nodeId: 'synthetic-node', revision: 1, configVersion: 'v6', config: { config: saved }, configFingerprint: 'synthetic-checkbox-fingerprint' } } })
      }
    })
    await page.goto('/exports/new')
    await page.getByRole('radio').first().check()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.locator('.export-field-body .ant-select').first().click()
    await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
    await chooseDatabase(page)
    await page.getByRole('radio', { name: '整库导出' }).check()
    await expect(page.getByRole('checkbox', { name: /备副本读取/ })).toHaveCount(0)
    await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
    await expect(page.getByRole('checkbox', { name: /隐藏主键加速|排除生成列/ })).toHaveCount(0)
    await expect(page.getByRole('checkbox', { name: /前置 DROP|保留 Schema|使用原生建表语句/ })).toHaveCount(0)
    await page.getByRole('radiogroup', { name: '导出内容' }).getByText('仅导出结构', { exact: true }).click()
    await expect(page.getByRole('checkbox', { name: /备副本读取|隐藏主键加速|附加表定义信息|排除生成列/ })).toHaveCount(0)
    await page.getByRole('button', { name: /^下一步：/ }).click()
    const options = page.getByRole('region', { name: '其他选项' })
    for (const name of [/前置 DROP/, /保留 Schema/, /使用原生建表语句/]) await options.getByRole('checkbox', { name }).check()
    await expect(options.getByText(/可能造成数据丢失/)).toBeVisible()
    await expect(page.getByRole('combobox', { name: '数据格式' })).toHaveCount(0)
    await expect(options.getByRole('checkbox', { name: '附加表定义信息' })).toBeDisabled()
    await expect(options.getByRole('checkbox', { name: /备副本读取|隐藏主键加速|排除生成列/ })).toHaveCount(0)
    await expect(options.getByRole('checkbox')).toHaveCount(4)
    await expect(page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ })).toHaveCount(0)
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
    await options.screenshot({ path: info.outputPath('ddl-only-checkboxes.png') })
    await page.getByRole('button', { name: '上一步', exact: true }).click()
    await expect(page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ })).not.toContainText('已配置')
    await page.getByRole('radiogroup', { name: '导出内容' }).getByText('导出结构和数据', { exact: true }).click()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    for (const format of ['CSV', 'CUT', 'SQL']) {
      await chooseExportOption(page, '数据格式', `${format} 格式`)
      for (const name of [/前置 DROP/, /保留 Schema/, /使用原生建表语句/]) await expect(options.getByRole('checkbox', { name })).toBeChecked()
      for (const name of [/备副本读取/, /隐藏主键加速/, /附加表定义信息/]) await expect(options.getByRole('checkbox', { name })).toBeDisabled()
      await expect(options.getByRole('checkbox', { name: /排除生成列/ })).toBeEnabled()
      const zeroDate = options.getByRole('checkbox', { name: /保留零日期时间/ })
      if (mode === 'MYSQL' && format !== 'SQL') await expect(zeroDate).toBeDisabled()
      else await expect(zeroDate).toHaveCount(0)
      await expect(options.getByRole('checkbox', { name: '包含列名表头' })).toHaveCount(format === 'CSV' ? 1 : 0)
      await expect(options.getByRole('checkbox', { name: /行尾分隔符处理|移除回车换行/ })).toHaveCount(format === 'CUT' ? 2 : 0)
      await expect(options.getByRole('checkbox', { name: /去除首尾空格/ })).toHaveCount(format !== 'SQL' ? 1 : 0)
    }
    await page.getByRole('button', { name: '上一步', exact: true }).click()
    await page.getByRole('radiogroup', { name: '导出内容' }).getByText('仅导出数据', { exact: true }).click()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await expect(options.getByRole('checkbox', { name: /前置 DROP|保留 Schema|使用原生建表语句/ })).toHaveCount(0)
    await page.getByRole('button', { name: '上一步', exact: true }).click()
    await page.getByRole('radiogroup', { name: '导出内容' }).getByText('导出结构和数据', { exact: true }).click()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    for (const name of [/前置 DROP/, /保留 Schema/, /使用原生建表语句/]) await options.getByRole('checkbox', { name }).check()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.getByRole('button', { name: '上一步', exact: true }).click()
    await options.getByRole('checkbox', { name: /排除生成列/ }).check()
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
    await options.screenshot({ path: info.outputPath('all-checkboxes.png') })
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic-checkboxes')
    await page.getByRole('button', { name: '创建草稿并进入预检查' }).click()
    await expect.poll(() => saved?.ddlBehavior).toEqual({ dropObject: true, retainSchema: true, compactSchema: true })
    expect(saved?.dataFormat?.formatKind).toBe('SQL')
    expect(saved?.filterConfig?.excludeVirtualColumns).toBe(true)
    expect(JSON.stringify(saved)).not.toMatch(/preserveZero|weakRead|enableHiddenPk|addExtraMessage/)
    expect(errors).toEqual([])
  })
}

test('其他选项按四类展示，说明分行且不联动勾选', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await page.getByRole('radio', { name: '整库导出' }).check()
  await page.getByRole('radiogroup', { name: '导出内容' }).getByText('导出结构和数据', { exact: true }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  const options = page.getByRole('region', { name: '其他选项' })
  await expect(options.locator('.export-format-panel')).toHaveCount(1)
  await expect(options.locator('.export-option-group .export-format-panel')).toHaveCount(0)
  for (const group of ['结构导出', '数据读取', '字段与文本处理', '文件组织']) {
    await expect(options.getByRole('heading', { name: group, exact: true })).toBeVisible()
    await expect(options.getByRole('region', { name: `${group}选项` })).toBeVisible()
  }
  const names = new Map([
    ['前置 DROP', '--drop-object'], ['保留 Schema 前缀', '--retain-schema'], ['使用原生建表语句', '--compact-schema'],
    ['附加表定义信息', '--add-extra-message'], ['备副本读取', '--weak-read'], ['一致性快照', '--snapshot'],
    ['隐藏主键加速', '--enable-hidden-pk'], ['排除生成列', '--exclude-virtual-columns'], ['保留零日期时间', '--preserve-zero-datetime'],
    ['包含列名表头', '--skip-header'], ['去除首尾空格', '--with-trim'], ['行尾分隔符处理', '--trail-delimiter'],
    ['移除回车换行', '--remove-newline'], ['扁平输出目录', '--no-nested-dir'], ['保留空文件', '--retain-empty-files'],
  ])
  const checked = new Set<string>()
  for (const format of ['CSV', 'CUT', 'SQL']) {
    await chooseExportOption(page, '数据格式', `${format} 格式`)
    for (const [label, parameter] of names) {
      const control = options.getByRole('checkbox', { name: label, exact: true })
      if (!await control.count() || checked.has(label)) continue
      const original = await control.isChecked()
      const hint = options.getByRole('button', { name: `${label}参数说明`, exact: true })
      await hint.hover()
      const tooltip = page.getByRole('tooltip').filter({ visible: true })
      await expect(tooltip).toContainText(parameter)
      await expect(tooltip).toContainText('含义：')
      const rows = await tooltip.locator('.export-option-hint-content > div').evaluateAll((items) => items.map((item) => item.getBoundingClientRect().y))
      expect(rows.length === 2 && rows[1]! > rows[0]!).toBe(true)
      await hint.click()
      expect(await control.isChecked()).toBe(original)
      await hint.blur()
      await page.mouse.move(12, 80)
      await expect(tooltip).toBeHidden()
      checked.add(label)
    }
  }
  expect([...checked].sort()).toEqual([...names.keys()].sort())
  await chooseExportOption(page, '数据格式', 'CUT 格式')
  await options.scrollIntoViewIfNeeded()
  await options.screenshot({ path: info.outputPath('other-options-groups.png') })
  await page.setViewportSize({ width: 720, height: 900 })
  const disabledHint = options.getByRole('button', { name: '附加表定义信息参数说明' })
  await disabledHint.focus()
  const tooltip = page.getByRole('tooltip').filter({ visible: true })
  await expect(tooltip).toContainText('--add-extra-message')
  await expect(tooltip).toContainText('sys 租户权限')
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('other-options-tooltip-narrow.png') })
  expect(errors).toEqual([])
})

for (const mode of ['MYSQL', 'ORACLE'] as const) {
  test(`其他步骤沿用第三步参数说明且不误触控件 ${mode}`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await mockFacts(page)
    await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
    await page.route('**/api/v1/data-sources', (route) => route.fulfill({ json: { items: [{ ...DATA_SOURCE_UI_FIXTURES[0]!, compatibilityMode: mode }], total: 1, nextCursor: '' } }))
    await page.setViewportSize({ width: 1440, height: 1024 })
    const checkHint = async (label: string, parameter: string) => {
      const hint = page.getByRole('button', { name: `${label}参数说明`, exact: true })
      await page.mouse.move(200, 20)
      await hint.focus()
      const tooltip = page.getByRole('tooltip').filter({ hasText: `参数：${parameter}` }).filter({ visible: true })
      await expect(tooltip).toContainText(parameter)
      await expect(tooltip).toContainText('含义：')
      const rows = await tooltip.locator('.export-option-hint-content > div').evaluateAll((items) => items.map((item) => item.getBoundingClientRect().y))
      expect(rows.length === 2 && rows[1]! > rows[0]!).toBe(true)
      await hint.click()
      await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
      await hint.blur()
      await page.mouse.move(200, 20)
      await expect(tooltip).toBeHidden()
    }
    await page.goto('/exports/new')
    await expect(page.locator('.export-option-hint')).toHaveCount(0)
    await expect(page.locator('.export-format-panel')).toHaveCount(0)
    await expect(page.getByRole('radio').first()).not.toBeChecked()
    await page.getByRole('radio').first().check()
    await page.screenshot({ path: info.outputPath('source-baseline.png') })
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await expect(page.getByRole('button', { name: '读取执行节点参数说明', exact: true })).toHaveCount(0)
    await expect(page.getByRole('combobox', { name: '读取执行节点', exact: true }).locator('xpath=ancestor::*[contains(@class, "export-format-panel")]')).toHaveCount(0)
    await expect(page.locator('h3 .export-option-hint')).toHaveCount(0)
    await page.locator('.export-field-body .ant-select').first().click()
    await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
    await chooseDatabase(page)
    await expect(page.getByRole('button', { name: '数据库 / Schema参数说明', exact: true })).toHaveCount(0)
    await expect(page.locator('.export-format-panel')).toHaveCount(0)
    await page.getByText('导出结构和数据', { exact: true }).click()
    await page.getByRole('radio', { name: '整库导出' }).check()
    await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
    await checkHint('序列策略', '--sequence-policy')
    await expect(page.locator('.export-advanced-form .ant-select')).toHaveClass(/ant-select-disabled/)
    await checkHint('条件筛选', '--where')
    await expect(page.getByRole('textbox', { name: '条件筛选', exact: true })).toBeDisabled()
    await page.getByRole('textbox', { name: '排除表', exact: true }).fill('synthetic_excluded')
    await checkHint('排除表', '--exclude-table')
    await expect(page.getByRole('textbox', { name: '排除表', exact: true })).toHaveValue('synthetic_excluded')
    await page.screenshot({ path: info.outputPath('objects-baseline.png') })
    await page.setViewportSize({ width: 720, height: 900 })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath('objects-baseline-narrow.png') })
    await page.setViewportSize({ width: 1440, height: 1024 })
    await page.getByText('按结果集导出', { exact: true }).click()
    await expect(page.getByRole('button', { name: '查询 SQL参数说明', exact: true })).toHaveCount(0)
    await expect(page.locator('.export-format-panel')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '查询结果条数限制参数说明', exact: true })).toHaveCount(0)
    await expect(page.getByRole('textbox', { name: '查询结果条数限制', exact: true }).locator('xpath=ancestor::*[contains(@class, "export-format-panel")]')).toHaveCount(0)
    await expect(page.getByRole('textbox', { name: '查询结果条数限制', exact: true })).toHaveValue('1000')
    await page.getByText('导出结构和数据', { exact: true }).click()
    await page.getByRole('radio', { name: '整库导出' }).check()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.getByRole('textbox', { name: '导出路径', exact: true }).fill('/E:/exports/synthetic-baseline')
    await page.getByRole('textbox', { name: '日志路径', exact: true }).fill('/E:/logs/synthetic-baseline')
    await checkHint('日志路径', '--log-path')
    await checkHint('跳过导出目录空性检查', '--skip-check-dir')
    await expect(page.getByRole('checkbox', { name: '跳过导出目录空性检查', exact: true })).not.toBeChecked()
    await page.getByRole('checkbox', { name: '跳过导出目录空性检查', exact: true }).check()
    await expect(page.getByText('可能覆盖同名文件；路径可写性与可用空间仍会检查。')).toBeVisible()
    const advanced = page.getByRole('button', { name: /高级设置 · 执行限制与资源/ })
    await advanced.click()
    await page.getByRole('textbox', { name: '导出线程', exact: true }).fill('4')
    await checkHint('导出线程', '--thread')
    if (mode === 'ORACLE') await checkHint('游标抓取行数（Oracle）', '--fetch-size')
    else await expect(page.getByRole('button', { name: '游标抓取行数（Oracle）参数说明', exact: true })).toHaveCount(0)
    await advanced.click()
    await advanced.click()
    await expect(page.getByRole('textbox', { name: '导出线程', exact: true })).toHaveValue('4')
    await page.getByRole('radio', { name: 'OSS', exact: true }).check()
    await expect(page.getByRole('button', { name: /^(执行节点|输出目的地|Bucket|对象路径|Endpoint|Region|存储凭据)参数说明$/ })).toHaveCount(0)
    await expect(page.getByRole('textbox', { name: 'Bucket', exact: true }).locator('xpath=ancestor::*[contains(@class, "export-format-panel")]')).toHaveCount(0)
    await checkHint('本地临时分块目录', '--tmp-path')
    await expect(page.getByRole('checkbox', { name: '跳过导出目录空性检查', exact: true })).toHaveCount(0)
    await page.getByRole('radio', { name: '本地路径', exact: true }).check()
    await expect(page.getByRole('textbox', { name: '日志路径', exact: true })).toHaveValue('/E:/logs/synthetic-baseline')
    await page.setViewportSize({ width: 720, height: 900 })
    await page.getByRole('button', { name: '导出路径参数说明', exact: true }).focus()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath('output-baseline-narrow.png') })
    expect(errors).toEqual([])
  })

  test(`第三步各区域参数说明分行且保留控件交互 ${mode}`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
    await mockFacts(page)
    await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
    const source = { ...DATA_SOURCE_UI_FIXTURES[0]!, compatibilityMode: mode }
    await page.route('**/api/v1/data-sources', (route) => route.fulfill({ json: { items: [source], total: 1, nextCursor: '' } }))
    await page.setViewportSize({ width: 1440, height: 1024 })
    await page.goto('/exports/new')
    await page.getByRole('radio').first().check()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.locator('.export-field-body .ant-select').first().click()
    await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
    await chooseDatabase(page)
    await page.getByRole('radio', { name: '整库导出' }).check()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await expect(page.getByRole('button', { name: '闪回 SCN参数说明' })).toHaveCount(0)
    const advanced = page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ })
    await advanced.click()
    await page.getByRole('checkbox', { name: '启用压缩', exact: true }).check()
    const hints: Array<[string, string]> = [
      ['数据格式', '--csv / --cut / --sql'], ['文件编码', '--file-encoding'], ['字段分隔符', '--column-separator'],
      ['字段分隔符', '--column-splitter'], ['文本识别符', '--column-quote'], ['换行符号', '--line-separator'],
      ['包围模式', '--column-quote-mode'], ['NULL 表示', '--null-string'], ['转义字符', '--escape-character'],
      ['闪回 SCN', '--flashback-scn'], ['闪回时间点', '--flashback-timestamp'],
      ['DATETIME 值格式', '--datetime-value-format'], ['DATE 值格式', '--date-value-format'],
      ['文件拆分方式', '--block-size'], ['单个文件拆分阈值', '--block-size'],
      ['启用压缩', '--compress'], ['压缩算法', '--compression-algo'], ['压缩等级', '--compression-level'],
    ]
    const checked = new Set<string>()
    for (const format of ['CSV', 'CUT', 'SQL']) {
      await chooseExportOption(page, '数据格式', `${format} 格式`)
      for (const [label, parameter] of hints) {
        const key = `${label}:${parameter}`
        if (checked.has(key) || label === '字段分隔符' && parameter !== (format === 'CSV' ? '--column-separator' : '--column-splitter')) continue
        const hint = page.getByRole('button', { name: `${label}参数说明`, exact: true })
        if (!await hint.count()) continue
        await hint.hover()
        const tooltip = page.getByRole('tooltip').filter({ visible: true })
        await expect(tooltip).toContainText(parameter)
        await expect(tooltip).toContainText('含义：')
        const rows = await tooltip.locator('.export-option-hint-content > div').evaluateAll((items) => items.map((item) => item.getBoundingClientRect().y))
        expect(rows.length === 2 && rows[1]! > rows[0]!).toBe(true)
        await hint.click()
        await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
        await expect(page.getByRole('checkbox', { name: '启用压缩', exact: true })).toBeChecked()
        await hint.blur()
        await page.mouse.move(12, 80)
        await expect(tooltip).toBeHidden()
        checked.add(key)
      }
    }
    expect(checked.size).toBe(mode === 'MYSQL' ? 17 : 16)
    if (mode === 'MYSQL') await expect(page.getByRole('button', { name: '闪回时间点参数说明' })).toHaveCount(0)
    await chooseExportOption(page, '压缩算法', 'gzip')
    await expect(page.getByRole('button', { name: '压缩等级参数说明' })).toHaveCount(0)
    await chooseExportOption(page, '数据格式', 'CSV 格式')
    await page.setViewportSize({ width: 720, height: 900 })
    await page.getByRole('button', { name: '字段分隔符参数说明', exact: true }).focus()
    const tooltip = page.getByRole('tooltip').filter({ visible: true })
    await expect(tooltip).toContainText('--column-separator')
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath('format-hint-narrow.png') })
    expect(errors).toEqual([])
  })
}

test('筛选入口保留适用原因，快照与 SCN 在第三步互斥并保留提交值', async ({ page }, info) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'], VIEW: ['synthetic_view'] })
  let saved: GeneralizedExportConfig | undefined
  await page.route('**/api/v1/export-drafts**', async (route) => {
    if (route.request().method() === 'POST' && new URL(route.request().url()).pathname === '/api/v1/export-drafts') {
      saved = (route.request().postDataJSON() as { config: GeneralizedExportConfig }).config
      await route.fulfill({ json: { id: 'synthetic-consistency-draft' } })
    } else if (route.request().url().includes(':preview-command')) {
      await route.fulfill({ json: { command: 'obdumper --csv', configFingerprint: 'synthetic-consistency-fingerprint' } })
    } else {
      await route.fulfill({ json: { item: { id: 'synthetic-consistency-draft', dataSourceId: DATA_SOURCE_UI_FIXTURES[0]!.id, nodeId: 'synthetic-node', revision: 1, configVersion: 'v6', config: { config: saved }, configFingerprint: 'synthetic-consistency-fingerprint' } } })
    }
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await page.getByRole('radiogroup', { name: '导出内容' }).getByText('导出结构和数据', { exact: true }).click()
  await page.getByRole('radio', { name: '整库导出' }).check()
  await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
  const where = page.getByRole('textbox', { name: '条件筛选' })
  const partition = page.getByRole('textbox', { name: '分区筛选' })
  await expect(where).toBeVisible()
  await expect(where).toBeDisabled()
  await expect(partition).toBeDisabled()
  await expect(page.getByText('仅指定表范围可用；整库及包含其他类型的对象范围不适用。')).toHaveCount(2)
  await expect(page.getByRole('heading', { name: '读取一致性' })).toHaveCount(0)
  await expect(page.getByRole('checkbox', { name: /一致性快照/ })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: '闪回 SCN' })).toHaveCount(0)
  await page.getByRole('radio', { name: '部分导出' }).check()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await candidates.getByRole('checkbox', { name: '选择全部可见表' }).check()
  await expect(where).toBeEnabled()
  await expect(partition).toBeEnabled()
  await where.fill('id > 100')
  await partition.fill('p0,p2')
  await candidates.getByRole('checkbox', { name: '选择全部视图' }).check()
  await expect(where).toBeDisabled()
  await expect(where).toHaveValue('')
  await expect(partition).toBeDisabled()
  await expect(partition).toHaveValue('')
  await candidates.getByRole('checkbox', { name: '选择全部视图' }).uncheck()
  await where.fill('id > 100')
  await partition.fill('p0,p2')
  await page.getByRole('button', { name: /^下一步：/ }).click()
  const snapshot = page.getByRole('region', { name: '其他选项' }).getByRole('checkbox', { name: /一致性快照/ })
  await snapshot.check()
  const advanced = page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ })
  await advanced.click()
  const scn = page.getByRole('textbox', { name: '闪回 SCN' })
  await expect(scn).toBeDisabled()
  await expect(advanced).not.toContainText('已配置')
  await snapshot.uncheck()
  await scn.fill('invalid')
  await expect(snapshot).toBeDisabled()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page).toHaveURL(/step=3/)
  await expect(page.locator('.export-step-error').filter({ hasText: '闪回 SCN 必须是正整数。' })).toBeVisible()
  await expect(page.getByText('闪回 SCN 必须是正整数。').first()).toBeVisible()
  await scn.fill('100')
  await expect(advanced).toContainText('已配置 1 项')
  await expect(page.getByRole('button', { name: /^下一步：/ })).toBeEnabled()
  await page.locator('.export-advanced').screenshot({ path: info.outputPath('flashback-advanced.png') })
  await page.getByRole('button', { name: '上一步', exact: true }).click()
  await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
  await expect(where).toHaveValue('id > 100')
  await expect(partition).toHaveValue('p0,p2')
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(advanced).toContainText('已配置 1 项')
  await advanced.click()
  await expect(scn).toHaveValue('100')
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic-consistency')
  await page.getByRole('button', { name: '创建草稿并进入预检查' }).click()
  await expect.poll(() => saved?.filterConfig).toMatchObject({ flashbackScn: 100, where: 'id > 100', partition: 'p0,p2' })
  expect(saved?.filterConfig?.snapshot).toBeUndefined()
  expect(saved?.filterConfig?.flashbackTimestamp).toBeUndefined()
})

test('导出参数按租户显隐，切换 MySQL 清除 Oracle 专属活动值', async ({ page }) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
  const base = DATA_SOURCE_UI_FIXTURES[0]!
  await page.route('**/api/v1/data-sources', (route) => route.fulfill({ json: { items: [base, { ...base, id: 'synthetic-oracle-source', displayName: '合成 Oracle 数据源', compatibilityMode: 'ORACLE' }], total: 2, nextCursor: '' } }))
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    const sourceID = new URL(route.request().url()).pathname.split('/').at(-1)!.split(':')[0]
    await route.fulfill({ json: { item: { id: 'synthetic-catalog', status: 'SUCCEEDED', dataSourceId: sourceID, ...input, objects: input.objectType === 'DATABASE' ? ['finance_reporting'] : [], groups: input.objectType === 'ALL' ? catalogGroups({ TABLE: ['synthetic_table'] }) : [], truncated: false, validUntil: '2099-01-01T00:00:00Z' } } })
  })
  let saved: GeneralizedExportConfig | undefined
  await page.route('**/api/v1/export-drafts', async (route) => {
    saved = (route.request().postDataJSON() as { config: GeneralizedExportConfig }).config
    await route.fulfill({ json: { id: 'synthetic-mode-draft' } })
  })
  await page.route('**/api/v1/export-drafts/synthetic-mode-draft', (route) => route.fulfill({ json: { item: { id: 'synthetic-mode-draft', dataSourceId: base.id, nodeId: 'synthetic-node', revision: 1, configVersion: 'v6', config: { config: saved }, configFingerprint: 'synthetic-mode-fingerprint' } } }))
  await page.route('**/api/v1/export-drafts/synthetic-mode-draft:preview-command', (route) => route.fulfill({ json: { command: 'obdumper --csv', configFingerprint: 'synthetic-mode-fingerprint' } }))
  await page.goto('/exports/new')
  await page.getByRole('radio', { name: /合成 Oracle 数据源/ }).check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await page.getByRole('radio', { name: '整库导出' }).check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  await page.getByRole('textbox', { name: '闪回时间点' }).fill('2026-10-08 00:00:00')
  await expect(page.getByRole('checkbox', { name: /一致性快照/ })).toBeDisabled()
  await expect(page.getByRole('combobox', { name: 'DATETIME 值格式' })).toHaveCount(0)
  await expect(page.getByRole('checkbox', { name: /保留零日期时间/ })).toHaveCount(0)
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /高级设置 · 执行限制与资源/ }).click()
  await page.getByRole('textbox', { name: '游标抓取行数' }).fill('256')
  for (let index = 0; index < 3; index++) await page.getByRole('button', { name: '上一步', exact: true }).click()
  await page.getByRole('radio', { name: /Production finance reporting/ }).check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('radio', { name: '指定时间（Oracle）' })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: '闪回时间点' })).toHaveCount(0)
  await expect(page.getByRole('radiogroup', { name: '读取一致性' })).toHaveCount(0)
  await chooseDatabase(page)
  await page.getByRole('radio', { name: '整库导出' }).check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  await expect(page.getByRole('textbox', { name: '闪回时间点' })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: 'DATETIME 值格式' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: /保留零日期时间/ })).toBeDisabled()
  await expect(page.getByText(/Oracle 日期时间值格式尚待验证/)).toHaveCount(0)
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('textbox', { name: '游标抓取行数' })).toHaveCount(0)
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic-mode')
  await page.getByRole('button', { name: '创建草稿并进入预检查' }).click()
  await expect.poll(() => Boolean(saved)).toBe(true)
  await expect(page.locator('.configuration-summary')).toContainText('Production finance reporting')
  expect(saved?.filterConfig?.flashbackTimestamp).toBeUndefined()
  expect(saved?.performanceConfig?.fetchSize).toBeUndefined()
})

test('导出普通入口限制格式，执行页仅展开一个高级设置', async ({ page }, info) => {
  await mockFacts(page)
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new?step=3')
  const formats = page.getByRole('combobox', { name: '数据格式' })
  await expect(formats).toBeVisible()
  await expect(page.locator('.export-format-grid .ant-select-selection-item').first()).toHaveText('CSV 格式')
  await exportSelect(page, '数据格式').click()
  await expect(page.locator('.ant-select-dropdown:visible .ant-select-item-option')).toHaveCount(3)
  await page.getByRole('heading', { name: '数据文件设置' }).click()
  await expect(page.getByRole('heading', { name: '数据文件设置' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '其他选项' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: '包含列名表头' })).toBeChecked()
  await page.getByRole('checkbox', { name: '包含列名表头' }).uncheck()
  await expect(page.getByRole('checkbox', { name: '包含列名表头' })).not.toBeChecked()
  await expect(page.getByRole('checkbox', { name: /一致性快照/ })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '转义字符' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: /去除首尾空格/ })).toBeVisible()
  await expect(page.getByRole('combobox', { name: /单个文件拆分阈值/ })).toHaveCount(0)
  await expect(page.getByRole('checkbox', { name: /启用压缩/ })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: '文件编码' })).toBeVisible()
  await expect(page.getByRole('region', { name: '其他选项' }).getByRole('combobox')).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'CSV 常用选项' }).getByRole('combobox', { name: '字段分隔符' })).toBeVisible()
  await expect(page.getByRole('region', { name: '其他选项' }).getByRole('checkbox', { name: /扁平输出目录/ })).toBeVisible()
  await expect(page.getByRole('region', { name: '其他选项' }).getByRole('checkbox', { name: /保留空文件/ })).toBeVisible()
  await expect(page.getByRole('region', { name: '合成格式示意' })).toHaveCount(0)
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
  await page.getByRole('region', { name: '其他选项' }).screenshot({ path: info.outputPath('export-other-options.png') })
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  await expect(page.getByRole('combobox', { name: /单个文件拆分阈值/ })).toBeVisible()
  await chooseExportOption(page, '单个文件上限', '512 MB')
  await expect(exportSelect(page, '单个文件上限').locator('.ant-select-selection-item')).toHaveText('512 MB')
  await exportSelect(page, '文件编码').click()
  for (const label of ['UTF8', 'UTF16', 'UTF32', 'ISO-8859-1', 'ASCII', 'GB2312', 'GBK', 'GB18030']) {
    await expect(page.locator('.ant-select-dropdown:visible').getByText(label, { exact: true })).toBeVisible()
  }
  await page.locator('.ant-select-dropdown:visible .rc-virtual-list-holder').evaluate((element) => { element.scrollTop = element.scrollHeight })
  await expect(page.locator('.ant-select-dropdown:visible').getByText('BIG5', { exact: true })).toBeVisible()
  await page.getByRole('heading', { name: '数据文件设置' }).click()
  await chooseExportOption(page, '文件编码', 'GBK')
  await expect(exportSelect(page, '文件编码').locator('.ant-select-selection-item')).toHaveText('GBK')
  await chooseExportOption(page, '字段分隔符', '竖线 |')
  await expect(exportSelect(page, '字段分隔符').locator('.ant-select-selection-item')).toHaveText('竖线 |')
  await chooseExportOption(page, '字段分隔符', '自定义…')
  await page.getByRole('textbox', { name: '字段分隔符自定义值' }).fill('::')
  await expect(page.getByRole('textbox', { name: '字段分隔符自定义值' })).toHaveValue('::')
  await expect(page.getByText('请输入单个分隔字符。')).toBeVisible()
  await page.getByRole('textbox', { name: '字段分隔符自定义值' }).fill(':')
  await expect(page.getByText('请输入单个分隔字符。')).toHaveCount(0)
  await chooseExportOption(page, '字段分隔符', '英文逗号（默认）')
  await expect(page.getByRole('textbox', { name: '字段分隔符自定义值' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('export-format-csv.png'), fullPage: true })
  await chooseExportOption(page, '数据格式', 'CUT 格式')
  await expect(page.getByRole('heading', { name: '其他选项' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: '包含列名表头' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ })).toBeVisible()
  await page.screenshot({ path: info.outputPath('export-format.png'), fullPage: true })
  await page.goto('/exports/new?step=4')
  await expect(page.getByRole('button', { name: /高级设置 · 执行限制与资源/ })).toHaveCount(1)
  await expect(page.getByRole('textbox', { name: '导出路径' })).toBeVisible()
  await expect(page.getByText('日志路径（可选）', { exact: true })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '日志路径' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '日志路径' })).toHaveValue('')
  await page.getByRole('checkbox', { name: /跳过导出目录空性检查/ }).check()
  await expect(page.getByText('可能覆盖同名文件；路径可写性与可用空间仍会检查。')).toBeVisible()
  await expect(page.getByRole('button', { name: /高级设置 · 执行限制与资源/ })).not.toContainText('已配置')
  await page.screenshot({ path: info.outputPath('export-output.png'), fullPage: true })
  await page.getByRole('button', { name: '创建草稿并进入预检查' }).click()
  await expect(page.getByText('请先选择一个完成基础连接测试的数据源。').first()).toBeVisible()
})

test('导出高级设置使用 Ant 展开反馈并按格式和输出类型显隐', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  async function captureAdvanced(name: string) {
    const panel = page.locator('.export-advanced')
    await panel.scrollIntoViewIfNeeded()
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
    await page.mouse.move(16, 80)
    await panel.screenshot({ path: info.outputPath(name) })
  }
  await mockFacts(page)
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new?step=2')
  await page.getByRole('radiogroup', { name: '导出内容' }).getByText('仅导出结构', { exact: true }).click()
  const objectAdvanced = page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ })
  await expect(objectAdvanced).toHaveAttribute('aria-expanded', 'false')
  await objectAdvanced.focus()
  await page.keyboard.press('Enter')
  await expect(objectAdvanced).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByRole('checkbox', { name: /前置 DROP/ })).toHaveCount(0)

  await page.goto('/exports/new?step=3')
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  const compressionAlgo = page.getByRole('combobox', { name: '压缩算法' })
  await expect(compressionAlgo).toHaveCount(0)
  await page.getByRole('checkbox', { name: /启用压缩/ }).check()
  await expect(compressionAlgo).toBeEnabled()
  await chooseExportOption(page, '数据格式', 'CUT 格式')
  await expect(page.getByRole('combobox', { name: '字段分隔符' })).toBeVisible()
  await chooseExportOption(page, '字段分隔符', '自定义…')
  await page.getByRole('textbox', { name: '字段分隔符自定义值' }).fill('||')
  await expect(page.getByRole('textbox', { name: '字段分隔符自定义值' })).toHaveValue('||')
  await expect(page.getByRole('checkbox', { name: /行尾分隔符处理/ })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: /移除回车换行/ })).toBeVisible()
  await page.getByRole('checkbox', { name: /移除回车换行/ }).check()
  await expect(page.getByText('删除换行会改变导出数据中的文本内容，请确认符合交付要求。')).toBeVisible()
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  await expect(page.getByRole('checkbox', { name: /行尾分隔符处理/ })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '字段分隔符' })).toBeVisible()
  await chooseExportOption(page, '数据格式', 'SQL 格式')
  await expect(page.getByRole('combobox', { name: '字段分隔符' })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: '换行符号' })).toBeVisible()
  await expect(page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: '序列化' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '日期时间' })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: 'NULL 替换' })).toHaveCount(0)

  await page.goto('/exports/new?step=4')
  const executionAdvanced = page.getByRole('button', { name: /高级设置 · 执行限制与资源/ })
  await expect(executionAdvanced).toHaveAttribute('aria-expanded', 'false')
  await captureAdvanced('advanced-collapsed.png')
  await executionAdvanced.click()
  await expect(executionAdvanced).toHaveAttribute('aria-expanded', 'true')
  await page.getByRole('textbox', { name: '导出线程' }).fill('4')
  await expect(executionAdvanced).toContainText('已配置 1 项')
  await captureAdvanced('advanced-expanded.png')
  await executionAdvanced.click()
  await expect(page.getByRole('textbox', { name: '导出线程' })).toBeHidden()
  await executionAdvanced.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('textbox', { name: '导出线程' })).toHaveValue('4')
  await page.setViewportSize({ width: 720, height: 900 })
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await captureAdvanced('advanced-narrow.png')
  await page.getByRole('radiogroup', { name: '输出类型' }).getByRole('radio', { name: 'S3' }).check()
  await expect(page.getByRole('textbox', { name: 'Bucket' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '日志路径' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: /跳过导出目录空性检查/ })).toHaveCount(0)
  await expect(page.getByText(/未取得通过证据时不能提交/)).toBeVisible()
  expect(errors).toEqual([])
})

test('导出格式在 200% 缩放下仍可填写且无整页横滚', async ({ page }, info) => {
  await mockFacts(page)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/exports/new?step=3')
  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  await expect(page.getByRole('combobox', { name: '字段分隔符' })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '换行符号' })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '文件编码' })).toBeVisible()
  await expect(page.getByRole('button', { name: '下一步：执行与输出' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('export-format-zoom-200.png'), fullPage: true })
})

test('导出对象双栏在 200% 缩放下按顺序堆叠', async ({ page }, info) => {
  await mockFacts(page)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await chooseDatabase(page)
  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  const [entry, selected] = await Promise.all([
    page.getByRole('region', { name: '选择导出对象' }).boundingBox(),
    page.getByRole('region', { name: '已选导出对象' }).boundingBox(),
  ])
  expect(entry).not.toBeNull()
  expect(selected).not.toBeNull()
  expect(selected!.y).toBeGreaterThanOrEqual(entry!.y + entry!.height)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('export-objects-zoom-200.png'), fullPage: true })
})

test('导出草稿修改后沿用原标识，预检查通过后仍需提交确认', async ({ page }) => {
  test.setTimeout(45000)
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
  let created = 0
  let updated = 0
  let submitted = 0
  let revision = 1
  let stalePrecheck = true
  let input: Record<string, unknown> | undefined
  await page.route('**/api/v1/export-drafts**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/export-drafts' && request.method() === 'POST') {
      created += 1
      input = request.postDataJSON() as Record<string, unknown>
      await route.fulfill({ json: { id: 'synthetic-draft' } })
      return
    }
    if (path === '/api/v1/export-drafts/synthetic-draft' && request.method() === 'PATCH') {
      updated += 1
      revision += 1
      input = request.postDataJSON() as Record<string, unknown>
    }
    if (path.endsWith(':preview-command')) {
      await route.fulfill({ json: { command: 'obdumper --csv -p ******', configFingerprint: `synthetic-fingerprint-${revision}` } })
      return
    }
    if (path.endsWith(':precheck')) {
      await route.fulfill({ json: { id: 'synthetic-precheck' } })
      return
    }
    if (path.endsWith(':submit')) {
      submitted += 1
      await route.fulfill({ json: { id: 'synthetic-task' } })
      return
    }
    await route.fulfill({ json: { item: { id: 'synthetic-draft', dataSourceId: input?.dataSourceId, nodeId: input?.nodeId, revision, configVersion: 'v6', config: { config: input?.config }, configFingerprint: `synthetic-fingerprint-${revision}` } } })
  })
  await page.route('**/api/v1/prechecks/**', async (route) => {
    const checks = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE']
    await route.fulfill({ json: { item: { id: 'synthetic-precheck', draftId: 'synthetic-draft', draftRevision: stalePrecheck ? revision - 1 : revision, configFingerprint: `synthetic-fingerprint-${revision}`, nodeId: 'synthetic-node', status: 'SUCCEEDED', integrityStatus: 'COMPLETE', validUntil: '2026-10-01T00:00:00Z', results: checks.map((check) => ({ check, status: 'PASSED', evidenceCode: 'SYNTHETIC_PASSED' })) } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await selectSyntheticTable(page)
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('combobox', { name: '执行节点' })).toHaveCount(0)
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic')
  await page.getByRole('button', { name: '保存草稿' }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/changed')
  await expect(page.getByText('草稿有未保存更改')).toBeVisible()
  await page.getByRole('button', { name: '保存草稿' }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  expect(created).toBe(1)
  expect(updated).toBe(1)
  await page.reload()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '导出路径' })).toHaveValue('/E:/exports/changed')
  expect(created).toBe(1)
  expect(updated).toBe(1)
  await page.getByRole('button', { name: '保存并进入预检查' }).click()
  await expect(page.getByRole('heading', { name: '参数预检查与完整命令' })).toBeVisible()
  await page.reload()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '执行预检查' }).click()
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeDisabled()
  stalePrecheck = false
  await page.getByRole('button', { name: '重新执行预检查' }).click()
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: '提交并启动导出', exact: true }).click()
  const confirmation = page.getByRole('dialog', { name: '确认提交导出任务' })
  await expect(confirmation).toBeVisible()
  await expect(confirmation.getByRole('button', { name: '返回检查' })).toBeFocused()
  expect(submitted).toBe(0)
  await confirmation.getByRole('button', { name: '返回检查' }).click()
  await expect(confirmation).not.toBeVisible()
  await page.getByRole('button', { name: '提交并启动导出', exact: true }).click()
  await confirmation.getByRole('button', { name: '确认提交并启动导出' }).click()
  await expect.poll(() => submitted).toBe(1)
})

test('更换执行节点后创建新草稿以遵守绑定不可变契约', async ({ page }) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
  await page.route('**/api/v1/execution-nodes**', (route) => route.fulfill({ json: { items: [
    { id: 'synthetic-node', displayName: '合成执行节点', platform: 'WINDOWS_AMD64' },
    { id: 'synthetic-node-2', displayName: '合成执行节点 2', platform: 'WINDOWS_AMD64' },
  ] } }))
  let created = 0
  let updated = 0
  let input: Record<string, unknown> | undefined
  await page.route('**/api/v1/export-drafts**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/export-drafts' && request.method() === 'POST') {
      created += 1
      input = request.postDataJSON() as Record<string, unknown>
      await route.fulfill({ json: { id: `synthetic-draft-${created}` } })
      return
    }
    if (request.method() === 'PATCH') updated += 1
    if (path.endsWith(':preview-command')) {
      await route.fulfill({ json: { command: 'obdumper --csv -p ******', configFingerprint: 'synthetic-fingerprint' } })
      return
    }
    await route.fulfill({ json: { item: { id: `synthetic-draft-${created}`, dataSourceId: input?.dataSourceId, nodeId: input?.nodeId, revision: 1, configVersion: 'v6', config: { config: input?.config }, configFingerprint: 'synthetic-fingerprint' } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64', { exact: true }).last().click()
  await chooseDatabase(page)
  await selectSyntheticTable(page)
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic')
  await page.getByRole('button', { name: '保存草稿' }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '更换节点' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 2 · WINDOWS_AMD64').last().click()
  await expect(page.getByText('草稿尚未创建')).toBeVisible()
  await selectSyntheticTable(page)
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: '保存草稿' }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  expect(created).toBe(2)
  expect(updated).toBe(0)
})

test('卸数整改保留空格分隔符、整库排除表、自动压缩与文件组织并显示摘要', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
  let saved: { dataSourceId: string; nodeId: string; config: GeneralizedExportConfig } | undefined
  await page.route('**/api/v1/export-drafts**', async (route) => {
    if (route.request().method() === 'POST' && new URL(route.request().url()).pathname === '/api/v1/export-drafts') {
      saved = route.request().postDataJSON()
      await route.fulfill({ json: { id: 'format-audit-draft' } })
    } else if (route.request().url().includes(':preview-command')) {
      await route.fulfill({ json: { command: 'obdumper --csv', configFingerprint: 'synthetic-format-fingerprint' } })
    } else {
      await route.fulfill({ json: { item: { id: 'format-audit-draft', dataSourceId: saved?.dataSourceId, nodeId: saved?.nodeId, revision: 1, configVersion: 'v6', config: { config: saved?.config }, configFingerprint: 'synthetic-format-fingerprint' } } })
    }
  })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await page.getByRole('radio', { name: '整库导出' }).check()
  await expect(page.getByRole('radiogroup', { name: '读取一致性' })).toHaveCount(0)
  await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
  await page.getByRole('textbox', { name: '排除表' }).fill('synthetic_excluded')
  await expect(page.getByRole('button', { name: /高级设置.*已配置 1 项/ })).toBeVisible()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('heading', { name: '数据文件设置' })).toBeVisible()
  await expect(page.getByRole('radiogroup', { name: '读取一致性' })).toHaveCount(0)
  await page.getByRole('region', { name: '其他选项' }).getByRole('checkbox', { name: /一致性快照/ }).check()
  await chooseExportOption(page, '字段分隔符', '自定义…')
  await page.getByRole('textbox', { name: '字段分隔符自定义值' }).fill(' ')
  await expect(page.getByRole('textbox', { name: '字段分隔符自定义值' })).toHaveValue(' ')
  await page.getByRole('textbox', { name: 'NULL 替换' }).fill(' NULL ')
  await expect(page.getByRole('combobox', { name: 'DATE 值格式' })).toHaveCount(0)
  await expect(page.getByRole('checkbox', { name: /启用压缩/ })).toHaveCount(0)
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  await chooseExportOption(page, 'DATE 值格式', 'yyyyMMdd')
  await page.getByRole('checkbox', { name: /启用压缩/ }).check()
  await expect(exportSelect(page, '压缩算法').locator('.ant-select-selection-item')).toHaveText('自动（zstd）')
  await page.getByRole('radio', { name: '按行数（ROW）' }).check()
  await chooseExportOption(page, '单个文件上限', '自定义…')
  await page.getByRole('textbox', { name: '单个文件上限自定义值' }).fill('256')
  await expect(page.getByRole('radio', { name: '按行数（ROW）' })).toBeChecked()
  await page.getByRole('checkbox', { name: /保留空文件/ }).check()
  await page.getByRole('checkbox', { name: /扁平输出目录/ }).check()
  await expect(page.getByRole('region', { name: '合成格式示意' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ })).toContainText('已配置 3 项')
  await page.screenshot({ path: info.outputPath('export-format-audit.png'), fullPage: true })
  await page.setViewportSize({ width: 720, height: 900 })
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('export-format-audit-narrow.png'), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('radiogroup', { name: '输出类型' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: /保留空文件/ })).toHaveCount(0)
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic-format')
  await page.getByRole('button', { name: /高级设置 · 执行限制与资源/ }).click()
  await page.getByRole('textbox', { name: '导出总量上限' }).fill('1048576')
  await expect(page.getByText('已设置导出总量限制，输出可能不包含全部所选数据。')).toBeVisible()
  await page.screenshot({ path: info.outputPath('export-output-audit.png'), fullPage: true })
  await page.getByRole('button', { name: '创建草稿并进入预检查' }).click()
  await expect.poll(() => saved?.config.objectScope.excludeTables).toEqual(['synthetic_excluded'])
  expect(saved?.config.dataFormat?.csvOptions).toMatchObject({ columnSeparator: ' ', nullString: ' NULL ' })
  expect(saved?.config.outputConfig).toMatchObject({ compress: true, compressionAlgo: 'zstd', retainEmptyFiles: true, noNestedDir: true, maxFileSize: 1048576 })
  expect(saved?.config.performanceConfig).toMatchObject({ blockSize: '256ROW' })
  expect(saved?.config.filterConfig?.snapshot).toBe(true)
  await expect(page.locator('.configuration-summary')).toContainText('synthetic_excluded')
  await expect(page.locator('.configuration-summary')).toContainText('256 行')
  await expect(page.locator('.configuration-summary')).toContainText('1048576 Byte')
  await expect(page.locator('.configuration-summary .export-option-hint, .configuration-summary .export-format-panel')).toHaveCount(0)
  expect(errors).toEqual([])
})

test('结果集不显示一致性控制且切换 SQL 后提示格式参数变化', async ({ page }) => {
  await mockFacts(page)
  await mockExportCatalog(page, { TABLE: ['synthetic_table'] })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await chooseDatabase(page)
  await page.getByRole('radiogroup', { name: '导出内容' }).getByText('按结果集导出', { exact: true }).click()
  await page.locator('.sql-query-editor .view-line').first().click()
  await page.keyboard.insertText('SELECT id FROM synthetic_table')
  await expect(page.getByRole('radiogroup', { name: '读取一致性' })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: '自定义查询', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('checkbox', { name: /一致性快照/ })).toHaveCount(0)
  await page.getByRole('textbox', { name: 'NULL 替换' }).fill('NULL')
  await chooseExportOption(page, '数据格式', 'SQL 格式')
  await expect(page.getByRole('alert').filter({ hasText: '已切换为 SQL' })).toContainText('NULL 表示')
  await expect(page.getByRole('textbox', { name: 'NULL 替换' })).toHaveCount(0)
  await expect(page.getByRole('combobox', { name: '包围模式' })).toHaveCount(0)
  await expect(page.getByRole('region', { name: '合成格式示意' })).toHaveCount(0)
  await expect(page.getByRole('region', { name: '其他选项' }).getByRole('checkbox', { name: /备副本读取|隐藏主键加速|附加表定义信息|排除生成列/ })).toHaveCount(0)
  await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
  await expect(page.getByRole('textbox', { name: '闪回 SCN' })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: '闪回时间点' })).toHaveCount(0)
})
