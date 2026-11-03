import { expect, test, type Page } from '@playwright/test'
import { mockFacts } from './fixtures/facts'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
import type { ExportObjectType, GeneralizedExportConfig } from '../src/api/browser'

const categories: ReadonlyArray<[ExportObjectType, string]> = [
  ['TABLE', '表'], ['VIEW', '视图'], ['FUNCTION', '函数'], ['PROCEDURE', '存储过程'],
  ['PACKAGE', '程序包'], ['TRIGGER', '触发器'], ['SYNONYM', '同义词'], ['PUBLIC_SYNONYM', '公共同义词'], ['TYPE', '类型'], ['SEQUENCE', '序列'],
]
const additions = categories.slice(4, 9)
const objectName = (type: ExportObjectType) => type === 'SYNONYM' || type === 'PUBLIC_SYNONYM' ? 'SYNTHETIC_ALIAS' : `SYNTHETIC_${type}`

async function oracleFixture(page: Page, legacy = false) {
  await mockFacts(page)
  const source = { ...DATA_SOURCE_UI_FIXTURES[0]!, compatibilityMode: 'ORACLE', defaultDatabase: undefined }
  await page.route('**/api/v1/data-sources', route => route.fulfill({ json: { items: [source], total: 1, nextCursor: '' } }))
  const requested: string[] = []
  await page.route('**/api/v1/data-sources/*:search-export-objects', async route => {
    const input = route.request().postDataJSON() as { objectType: 'DATABASE' | 'ALL' | ExportObjectType; keyword: string }
    requested.push(input.objectType)
    const types = legacy ? categories.filter(([type]) => !additions.some(([added]) => added === type)) : categories
    await route.fulfill({ json: { item: {
      id: 'synthetic-oracle-catalog', dataSourceId: source.id, ...input,
      objects: input.objectType === 'DATABASE' ? ['SYNTHETIC_SCHEMA'] : input.objectType === 'ALL' ? [] : [objectName(input.objectType)],
      groups: input.objectType === 'ALL' ? types.map(([objectType]) => ({ objectType, objects: [objectName(objectType)], truncated: false, unavailable: false })) : [],
      truncated: false, status: 'SUCCEEDED', validUntil: '2099-01-01T00:00:00Z',
    } } })
  })
  let saved: GeneralizedExportConfig | undefined
  let draftState: 'EDITING' | 'READY' | undefined
  await page.route('**/api/v1/export-drafts**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/export-drafts' && route.request().method() === 'POST') {
      const input = route.request().postDataJSON() as { config: GeneralizedExportConfig; draftState?: 'EDITING' | 'READY' }
      saved = input.config
      draftState = input.draftState
      await route.fulfill({ json: { id: 'synthetic-oracle-draft' } })
    } else if (path.endsWith(':preview-command')) {
      await route.fulfill({ json: { command: 'obdumper --ddl --package SYNTHETIC_PACKAGE', configFingerprint: 'synthetic-oracle-fingerprint' } })
    } else {
      await route.fulfill({ json: { item: {
        id: 'synthetic-oracle-draft', dataSourceId: source.id, nodeId: 'synthetic-node', revision: 1, configVersion: 'v6', draftState,
        capabilityVersion: draftState === 'EDITING' ? 'export-draft-edit-v1' : 'export-odp-ddl-csv-v1',
        metadataVersion: draftState === 'EDITING' ? 'obdumper-4.3.5-slice-v11-expert-parameters' : 'obdumper-4.3.5-slice-v13-oracle-objects',
        config: { draftState, config: saved }, configFingerprint: 'synthetic-oracle-fingerprint',
      } } })
    }
  })
  return { requested, saved: () => saved }
}

async function openObjects(page: Page) {
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.getByText('仅导出结构', { exact: true }).click()
  await page.locator('.export-node-select .ant-select').click()
  await page.locator('.ant-select-dropdown:visible').getByText('合成执行节点 · Windows · x86_64', { exact: true }).click()
  await page.getByRole('button', { name: '展开数据库列表' }).click()
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option-content').getByText('SYNTHETIC_SCHEMA', { exact: true }).click()
  await expect(page).toHaveURL(/exports\/new\?step=2/)
  await expect(page).toHaveTitle(/.+/)
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`Oracle 十类对象选择、内容切换与草稿恢复 ${viewport.width}`, async ({ page }, info) => {
    await page.setViewportSize(viewport)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error' || message.type() === 'warning') errors.push(message.text()) })
    const fixture = await oracleFixture(page)
    await openObjects(page)
    const candidates = page.getByRole('region', { name: '选择导出对象', exact: true })
    for (const [, label] of categories) await expect(candidates.getByRole('checkbox', { name: new RegExp(`^选择全部(?:可见)?${label}$`) })).toBeVisible()
    await expect(page.getByRole('heading', { name: '选择对象(10)' })).toBeVisible()
    for (const [, label] of [categories[0]!, ...additions]) await candidates.getByRole('checkbox', { name: new RegExp(`^选择全部(?:可见)?${label}$`) }).check()
    await expect(page.getByRole('heading', { name: '已选 6 项' })).toBeVisible()
    await page.evaluate(() => {
      window.scrollTo(0, 0)
      document.querySelectorAll('.export-object-scroll').forEach(element => { element.scrollTop = 0 })
    })
    await page.screenshot({ path: info.outputPath('oracle-ten-categories.png'), animations: 'disabled' })
    await page.locator('.export-object-workspace').evaluate(element => { element.scrollIntoView({ block: 'center', behavior: 'instant' }) })
    await candidates.getByRole('region', { name: '候选对象滚动区', exact: true }).evaluate(element => { element.scrollTop = element.scrollHeight })
    await page.locator('.export-object-workspace').screenshot({ path: info.outputPath('oracle-object-selector.png'), animations: 'disabled' })
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.getByText('仅导出数据', { exact: true }).click()
    await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
    for (const [, label] of additions) await expect(candidates.getByRole('checkbox', { name: new RegExp(`^选择全部(?:可见)?${label}$`) })).toHaveCount(0)
    await page.getByText('导出结构和数据', { exact: true }).click()
    await expect(page.getByRole('heading', { name: '已选 6 项' })).toBeVisible()
    await candidates.getByRole('button', { name: '展开程序包分类', exact: true }).click()
    await expect(candidates.getByRole('checkbox', { name: 'SYNTHETIC_PACKAGE', exact: true })).toBeChecked()
    await page.getByRole('textbox', { name: '搜索候选对象' }).fill('PACKAGE')
    await page.getByRole('button', { name: '搜索或刷新对象' }).click()
    await expect.poll(() => fixture.requested.filter(type => type === 'PACKAGE').length).toBe(1)
    await expect(candidates.getByRole('checkbox', { name: 'SYNTHETIC_PACKAGE', exact: true })).toBeChecked()
    await page.getByRole('button', { name: '保存草稿', exact: true }).click()
    await expect.poll(() => fixture.saved()?.objectScope.expressions?.length).toBe(6)
    expect(fixture.saved()?.objectScope.objectTypes).toEqual(['TABLE', 'PACKAGE', 'TRIGGER', 'SYNONYM', 'PUBLIC_SYNONYM', 'TYPE'])
    await expect(page).toHaveURL(/draft=synthetic-oracle-draft/)
    await page.reload()
    await expect(page.getByRole('heading', { name: '已选 6 项' })).toBeVisible()
    await page.getByRole('button', { name: '清空已选公共同义词', exact: true }).click()
    await expect(page.getByRole('heading', { name: '已选 5 项' })).toBeVisible()
    await expect(page.getByLabel('已选同义词列表', { exact: true }).getByText('SYNTHETIC_ALIAS', { exact: true })).toBeVisible()
    await candidates.getByRole('button', { name: '展开类型分类', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(candidates.getByRole('button', { name: '收起类型分类', exact: true })).toBeFocused()
    await expect(candidates.getByRole('checkbox', { name: 'SYNTHETIC_TYPE', exact: true })).toBeChecked()
    expect(errors).toEqual([])
  })
}

test('Oracle 旧 Agent 缺少新增分类时提示更新，不显示为零对象', async ({ page }) => {
  await oracleFixture(page, true)
  await openObjects(page)
  const candidates = page.getByRole('region', { name: '选择导出对象', exact: true })
  await expect(candidates.getByRole('button', { name: '程序包（加载失败）', exact: true })).toBeVisible()
  await candidates.getByRole('button', { name: '展开程序包分类', exact: true }).click()
  await expect(candidates.getByText('执行节点返回的对象分类不完整，请更新 Agent 后重试。')).toBeVisible()
  await expect(page.getByRole('button', { name: '下一步：选择数据格式' })).toBeDisabled()
})
