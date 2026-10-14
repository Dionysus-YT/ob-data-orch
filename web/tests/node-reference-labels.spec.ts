import { expect, test, type Page } from '@playwright/test'
import type { ExecutionNodeCandidate } from '../src/api/browser'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
import { mockFacts } from './fixtures/facts'

const source = DATA_SOURCE_UI_FIXTURES[0]!
const replacements: ExecutionNodeCandidate[] = [
  { id: 'synthetic-new-windows-node', displayName: '合成新节点', platform: 'WINDOWS_AMD64' },
  { id: 'synthetic-new-linux-node', displayName: '合成新节点', platform: 'LINUX_AMD64' },
  { id: 'synthetic-new-arm-node', displayName: '合成新节点', platform: 'LINUX_ARM64' },
]
const labels = ['合成新节点 · Windows · x86_64', '合成新节点 · Linux · x86_64', '合成新节点 · Linux · aarch64']

async function openChoices(page: Page, surface: string) {
  if (surface === 'sources') {
    const drawer = page.getByRole('dialog', { name: '编辑数据源', exact: true })
    await expect(drawer).toBeVisible()
    const select = drawer.getByRole('combobox', { name: /执行节点/ })
    await expect(select).toBeEnabled()
    await select.focus()
  } else if (surface === 'export') {
    const select = page.getByRole('combobox', { name: /选择执行节点/ })
    await expect(select).toBeEnabled()
    await select.focus()
  } else {
    await page.getByRole('button', { name: '用模板新建草稿', exact: true }).click()
    const select = page.getByRole('combobox', { name: /^执行节点/ })
    await expect(select).toBeEnabled()
    await select.focus()
  }
  await page.keyboard.press('ArrowDown')
}

for (const surface of ['sources', 'export', 'templates']) {
  test(`${surface} 节点改名动态更新，重新登记重读候选并统一平台展示`, async ({ page }, info) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
    let nodes: ExecutionNodeCandidate[] = [{ id: 'synthetic-old-node', displayName: '合成旧节点', platform: 'WINDOWS_AMD64' }]
    await mockFacts(page)
    await page.route('**/api/v1/execution-nodes?eligibleFor=*', route => route.fulfill({ json: { items: nodes } }))
    await page.route(`**/api/v1/data-sources/${source.id}`, route => route.fulfill({ json: { item: source } }))
    await page.route('**/api/v1/export-config-templates', route => route.fulfill({ json: { items: [{
      id: 'synthetic-template', displayName: '合成模板', revision: 1, capabilityVersion: 'synthetic-capability',
      configFingerprint: 'synthetic-fingerprint', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
    }] } }))
    const path = surface === 'sources' ? `/data-sources?edit=${source.id}&test=1` : surface === 'export' ? '/exports/new?step=2' : '/templates'
    await page.goto(path)
    await openChoices(page, surface)
    const originalLabel = '合成旧节点 · Windows · x86_64'
    await page.locator('.ant-select-dropdown:visible').getByText(originalLabel, { exact: true }).click()

    // 同一节点仅改登记名称；无需重载页面，重新展开即更新选项与当前选择的标签。
    nodes = [{ ...nodes[0]!, displayName: '合成重命名节点' }]
    await openChoices(page, surface)
    const renamedLabel = '合成重命名节点 · Windows · x86_64'
    await expect(page.locator('.ant-select-dropdown:visible').getByText(renamedLabel, { exact: true })).toBeVisible()
    await expect(page.locator('.ant-select-selection-item').filter({ hasText: renamedLabel })).toBeVisible()
    await page.locator('.ant-select-dropdown:visible').getByText(renamedLabel, { exact: true }).click()

    // 重新登记使用独立标识；展开时不能保留已删除节点的名称，也不能按同名自动改绑。
    nodes = replacements
    await openChoices(page, surface)
    const dropdown = page.locator('.ant-select-dropdown:visible')
    for (const label of labels) await expect(dropdown.getByText(label, { exact: true })).toBeVisible()
    await expect(dropdown).not.toContainText('合成旧节点')
    await expect(dropdown).not.toContainText('合成重命名节点')
    await expect(dropdown).not.toContainText('WINDOWS_AMD64')
    await expect(dropdown).not.toContainText('LINUX_AMD64')
    await expect(dropdown).not.toContainText('LINUX_ARM64')
    await dropdown.getByText(labels[2]!, { exact: true }).click()
    const selection = page.locator('.ant-select-selection-item').filter({ hasText: labels[2]! })
    await expect(selection).toBeVisible()
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await page.screenshot({ path: info.outputPath(`${surface}-node-labels.png`), fullPage: false })
    await page.reload()
    await openChoices(page, surface)
    for (const label of labels) await expect(page.locator('.ant-select-dropdown:visible').getByText(label, { exact: true })).toBeVisible()
    expect(errors).toEqual([])
  })
}
