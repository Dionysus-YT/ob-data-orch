import { expect, test, type Page } from '@playwright/test'
import { mockFacts } from './fixtures/facts'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
import type { ExportDraftInput } from '../src/api/browser'

async function openExpertForm(page: Page, mode: 'MYSQL' | 'ORACLE', sys: 'AVAILABLE' | 'UNAVAILABLE' = 'AVAILABLE') {
  await mockFacts(page)
  const source = { ...DATA_SOURCE_UI_FIXTURES[0]!, compatibilityMode: mode, sysCredentialState: sys }
  await page.route('**/api/v1/data-sources', route => route.fulfill({ json: { items: [source], total: 1, nextCursor: '' } }))
  await page.route('**/api/v1/data-sources/*:search-export-objects', async route => {
    const input = route.request().postDataJSON() as { nodeId: string; database: string; objectType: string; keyword: string }
    await route.fulfill({ json: { item: { id: 'synthetic-catalog', status: 'SUCCEEDED', dataSourceId: source.id, ...input, objects: input.objectType === 'DATABASE' ? ['finance_reporting'] : [], groups: [], truncated: false, validUntil: '2099-01-01T00:00:00Z' } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('radiogroup', { name: '导出内容' }).getByText('导出结构和数据', { exact: true }).click()
  await page.locator('.export-node-select .ant-select').click()
  await page.getByText('合成执行节点 · Windows · x86_64').last().click()
  await page.getByRole('button', { name: '展开数据库列表' }).click()
  await page.getByText('finance_reporting', { exact: true }).last().click()
  await page.getByRole('radio', { name: '整库导出' }).check()
  return source
}

for (const mode of ['MYSQL', 'ORACLE'] as const) {
  test(`专家参数接入并保存全部适用日期字段 ${mode}`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.setViewportSize({ width: 1440, height: 1024 })
    const source = await openExpertForm(page, mode)
    let saved: ExportDraftInput | undefined
    await page.route('**/api/v1/export-drafts**', async route => {
      if (route.request().method() === 'POST' && new URL(route.request().url()).pathname === '/api/v1/export-drafts') {
        saved = route.request().postDataJSON() as ExportDraftInput
        await route.fulfill({ json: { id: 'synthetic-expert-draft' } })
      } else if (route.request().url().endsWith(':preview-command')) {
        await route.fulfill({ json: { command: 'obdumper --sequence-policy restart --weak-read --enable-hidden-pk --add-extra-message --sys-user ******', configFingerprint: 'synthetic-expert-fingerprint' } })
      } else if (route.request().url().endsWith(':precheck')) {
        await route.fulfill({ json: { id: 'synthetic-expert-check' } })
      } else {
        await route.fulfill({ json: { item: { id: 'synthetic-expert-draft', dataSourceId: source.id, nodeId: 'synthetic-node', configVersion: 'v6', revision: 1, config: { config: saved?.config }, configFingerprint: 'synthetic-expert-fingerprint' } } })
      }
    })
    await page.route('**/api/v1/prechecks/**', route => route.fulfill({ json: { item: { id: 'synthetic-expert-check', draftId: 'synthetic-expert-draft', draftRevision: 1, nodeId: 'synthetic-node', configFingerprint: 'synthetic-expert-fingerprint', status: 'SUCCEEDED', integrityStatus: 'COMPLETE', validUntil: '2099-01-01T00:00:00Z', results: ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE'].map(check => ({ check, status: 'PASSED', evidenceCode: 'SYNTHETIC_OK' })) } } }))
    await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
    await page.locator('.ant-select').filter({ has: page.getByRole('combobox', { name: /序列策略/ }) }).click()
    await page.locator('.ant-select-dropdown:visible').getByText('使用初始值（restart）', { exact: true }).click()
    await page.getByRole('button', { name: /^下一步：/ }).click()
    for (const name of ['备副本读取', '隐藏主键加速', '附加表定义信息']) await page.getByRole('checkbox', { name, exact: true }).check()
    if (mode === 'MYSQL') await page.getByRole('checkbox', { name: '保留零日期时间', exact: true }).check()
    await page.getByRole('button', { name: /高级设置 · 读取一致性与文件设置/ }).click()
    await page.getByRole('textbox', { name: 'TIMESTAMP 值格式', exact: true }).fill('yyyy-MM-dd HH:mm:ss')
    if (mode === 'MYSQL') {
      await page.getByRole('textbox', { name: 'TIME 值格式', exact: true }).fill('HH:mm:ss')
      await expect(page.getByRole('textbox', { name: 'NLS DATE 格式', exact: true })).toHaveCount(0)
    } else {
      for (const name of ['TIMESTAMP TZ 值格式', 'TIMESTAMP LTZ 值格式']) await page.getByRole('textbox', { name, exact: true }).fill('yyyy-MM-dd HH:mm:ss')
      for (const name of ['NLS DATE 格式', 'NLS TIMESTAMP 格式', 'NLS TIMESTAMP TZ 格式']) await page.getByRole('textbox', { name, exact: true }).fill('YYYY-MM-DD HH24:MI:SS')
      await expect(page.getByRole('textbox', { name: 'TIME 值格式', exact: true })).toHaveCount(0)
    }
    await page.screenshot({ path: info.outputPath('expert-formats.png'), fullPage: true })
    await page.setViewportSize({ width: 720, height: 900 })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.getByRole('textbox', { name: 'TIMESTAMP 值格式', exact: true }).focus()
    await expect(page.getByRole('textbox', { name: 'TIMESTAMP 值格式', exact: true })).toBeFocused()
    await page.screenshot({ path: info.outputPath('expert-formats-narrow.png'), fullPage: true })
    await page.getByRole('button', { name: /^下一步：/ }).click()
    await page.getByRole('textbox', { name: '导出路径', exact: true }).fill('/E:/tmp/synthetic-expert-output')
    await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
    await expect.poll(() => saved?.config.ddlBehavior).toEqual({ addExtraMessage: true, sequencePolicy: 'RESTART' })
    expect(saved?.config.filterConfig).toMatchObject({ weakRead: true, enableHiddenPk: true })
    const formats = saved?.config.dataFormat?.timestampFormats
    expect(formats?.timestampValueFormat).toBe('yyyy-MM-dd HH:mm:ss')
    if (mode === 'MYSQL') expect(formats).toMatchObject({ timeValueFormat: 'HH:mm:ss', preserveZeroDatetime: true })
    else expect(formats).toMatchObject({ timestampTzValueFormat: 'yyyy-MM-dd HH:mm:ss', timestampLtzValueFormat: 'yyyy-MM-dd HH:mm:ss', nlsDateFormat: 'YYYY-MM-DD HH24:MI:SS', nlsTimestampFormat: 'YYYY-MM-DD HH24:MI:SS', nlsTimestampTzFormat: 'YYYY-MM-DD HH24:MI:SS' })
    await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
    await expect(page.getByRole('dialog', { name: '任务摘要', exact: true })).toContainText('/E:/tmp/synthetic-expert-output')
    expect(errors).toEqual([])
  })
}

test('附加表定义信息在没有 sys 凭据时保留原因并禁用', async ({ page }) => {
  await openExpertForm(page, 'MYSQL', 'UNAVAILABLE')
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await expect(page.getByRole('checkbox', { name: '附加表定义信息', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '附加表定义信息参数说明' }).focus()
  await expect(page.getByRole('tooltip').filter({ visible: true })).toContainText('sys')
  for (const name of ['备副本读取', '隐藏主键加速', '保留零日期时间']) await expect(page.getByRole('checkbox', { name, exact: true })).toBeEnabled()
})
