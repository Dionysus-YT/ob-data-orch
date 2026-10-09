import { expect, test, type Page, type Route } from '@playwright/test'
import { mockFacts } from './fixtures/facts'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
const time = '2026-01-01T00:00:00Z'
const credential = (revision = 1) => ({ id: 'synthetic-credential', displayName: '合成凭据', provider: 'OSS', revision, currentRevision: revision, updatedAt: time })
const template = { id: 'synthetic-template', displayName: '合成模板', revision: 1, capabilityVersion: 'synthetic-capability', configFingerprint: 'synthetic-fingerprint', createdAt: '2026-01-01T00:00:00Z', updatedAt: time }
async function navigate(page: Page, path: string) {
  await page.evaluate(async path => { const modulePath = '/src/router/index.ts'; const { router } = await import(/* @vite-ignore */ modulePath); await router.push(path) }, path)
}
async function fill(page: Page) { await page.getByLabel('凭据名称', { exact: true }).fill('合成凭据'); await page.getByLabel('AccessKey', { exact: true }).fill('synthetic-access'); await page.getByLabel('SecretKey', { exact: true }).fill('synthetic-secret') }

for (const status of [401, 403]) {
  test(`凭据列表 ${status} 后删除框关闭，授权恢复不恢复旧确认`, async ({ page }) => {
    await mockFacts(page); let delayRead = false; const held: Route[] = []; let writes = 0
    await page.route('**/api/v1/storage-credentials', route => delayRead ? void held.push(route) : route.fulfill({ json: { items: [credential()] } }))
    await page.route('**/api/v1/storage-credentials/synthetic-credential', route => { writes++; return route.fulfill({ status: 204 }) })
    await page.goto('/settings/storage-credentials'); await expect(page.getByRole('button', { name: '删除', exact: true })).toBeVisible()
    delayRead = true; await page.getByRole('button', { name: '刷新', exact: true }).click(); await expect.poll(() => held.length).toBe(1)
    // 构造刷新在途时已进入确认的竞态，不依赖加载遮罩的点击时机。
    await page.getByRole('button', { name: '删除', exact: true }).dispatchEvent('click')
    await expect(page.getByRole('dialog')).toContainText('合成凭据')
    await held[0]!.fulfill({ status, json: { code: 'SYNTHETIC_DENIED' } })
    await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByText('合成凭据', { exact: true })).toHaveCount(0)
    delayRead = false
    if (status === 401) { await expect(page).toHaveURL(/\/login$/); await page.goto('/settings/storage-credentials') }
    else await page.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(page.getByRole('region', { name: '存储凭据列表' })).toContainText('合成凭据'); await expect(page.getByRole('dialog')).toHaveCount(0); expect(writes).toBe(0)
  })
  test(`模板列表 ${status} 后清除改名与删除框，恢复授权后输入不复活`, async ({ page }) => {
    await mockFacts(page); let delayRead = false; const held: Route[] = []; let writes = 0
    await page.route('**/api/v1/export-config-templates', route => delayRead ? void held.push(route) : route.fulfill({ json: { items: [template] } }))
    await page.route('**/api/v1/export-config-templates/synthetic-template', route => { writes++; return route.fulfill({ status: 204 }) })
    await page.goto('/templates'); await page.getByRole('button', { name: '改名', exact: true }).click(); await page.getByRole('textbox', { name: '模板名称', exact: true }).fill('合成未保存改名')
    delayRead = true; await page.getByRole('button', { name: '刷新', exact: true }).click(); await expect.poll(() => held.length).toBe(1)
    await page.getByRole('button', { name: '删除', exact: true }).dispatchEvent('click'); await expect(page.getByRole('dialog')).toContainText('合成模板')
    await held[0]!.fulfill({ status, json: { code: 'SYNTHETIC_DENIED' } })
    await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByRole('textbox', { name: '模板名称', exact: true })).toHaveCount(0)
    delayRead = false
    if (status === 401) { await expect(page).toHaveURL(/\/login$/); await page.goto('/templates') }
    else await page.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(page.getByRole('region', { name: '模板列表' })).toContainText('合成模板')
    await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByRole('textbox', { name: '模板名称', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: '改名', exact: true }).click(); await expect(page.getByRole('textbox', { name: '模板名称', exact: true })).toHaveValue('合成模板'); expect(writes).toBe(0)
  })
}

test('凭据创建和轮换防重、版本及敏感输入清理，旧刷新不能覆盖新修订', async ({ page }, info) => {
  await mockFacts(page); let row = credential(); let delayRead = false; const held: Route[] = []; const rotations: Route[] = []
  await page.route('**/api/v1/storage-credentials', route => { if (delayRead) held.push(route); else return route.fulfill({ json: { items: [row] } }) })
  await page.route('**/api/v1/storage-credentials/*:rotate', route => { rotations.push(route) })
  await page.goto('/settings/storage-credentials'); await page.getByRole('button', { name: '轮换', exact: true }).click(); await fill(page)
  delayRead = true; await page.getByRole('button', { name: '刷新', exact: true }).click(); await expect.poll(() => held.length).toBe(1)
  await page.getByRole('button', { name: '确认轮换', exact: true }).click(); await expect.poll(() => rotations.length).toBe(1)
  const headers = rotations[0]!.request().headers(); expect(headers['if-match']).toBe('"rev-1"'); expect(Boolean(headers['idempotency-key'] && headers['x-csrf-token'])).toBe(true)
  const payload = rotations[0]!.request().postDataJSON(); expect(Boolean(payload.accessKey && payload.secretKey && payload.provider === 'OSS')).toBe(true)
  await expect(page.getByLabel('SecretKey', { exact: true })).toBeDisabled(); row = credential(2); await rotations[0]!.fulfill({ json: { item: row } })
  await expect(page.getByText('已轮换为修订 2', { exact: false })).toBeVisible(); await held[0]!.fulfill({ json: { items: [credential()] } })
  await expect(page.getByRole('region', { name: '存储凭据列表' })).toContainText('2'); expect(rotations).toHaveLength(1)
  await page.getByRole('button', { name: '轮换', exact: true }).click(); await expect(page.getByLabel('AccessKey', { exact: true })).toHaveValue(''); await expect(page.getByLabel('SecretKey', { exact: true })).toHaveValue(''); await page.getByRole('button', { name: '取消', exact: true }).click()
  await page.setViewportSize({ width: 640, height: 720 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('credential-clean-state.png'), fullPage: true })
})

test('凭据离开后迟到创建不回写新页面，取消及重新进入没有秘密残留', async ({ page }) => {
  await mockFacts(page); const writes: Route[] = []
  await page.route('**/api/v1/storage-credentials', route => { if (route.request().method() === 'POST') writes.push(route); else return route.fulfill({ json: { items: [] } }) })
  await page.goto('/settings/storage-credentials'); await page.getByRole('button', { name: '新增凭据', exact: true }).click(); await fill(page); await page.getByRole('button', { name: '取消', exact: true }).click()
  await page.getByRole('button', { name: '新增凭据', exact: true }).click(); await expect(page.getByLabel('SecretKey', { exact: true })).toHaveValue(''); await fill(page); await page.getByRole('button', { name: '创建', exact: true }).click(); await expect.poll(() => writes.length).toBe(1)
  await navigate(page, '/settings'); await writes[0]!.fulfill({ json: { item: credential() } }).catch(() => {})
  await navigate(page, '/settings/storage-credentials'); await expect(page.getByText('暂无存储凭据', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '新增凭据', exact: true }).click(); await expect(page.getByLabel('AccessKey', { exact: true })).toHaveValue(''); await expect(page.getByLabel('SecretKey', { exact: true })).toHaveValue('')
})

test('凭据删除重复操作阻断并保持版本与 API 安全头', async ({ page }) => {
  await mockFacts(page); let deleted = false; const held: Route[] = []
  await page.route('**/api/v1/storage-credentials', route => route.fulfill({ json: { items: deleted ? [] : [credential()] } }))
  await page.route('**/api/v1/storage-credentials/synthetic-credential', route => { held.push(route) })
  await page.goto('/settings/storage-credentials'); await page.getByRole('button', { name: '删除', exact: true }).click(); const dialog = page.getByRole('dialog'); await dialog.getByRole('button', { name: '确认删除', exact: true }).click(); await expect.poll(() => held.length).toBe(1)
  await expect(dialog.getByRole('button', { name: /确认删除/ })).toBeDisabled(); expect(held[0]!.request().headers()['if-match']).toBe('"rev-1"'); deleted = true; await held[0]!.fulfill({ status: 204 }); await expect(page.getByText('暂无存储凭据', { exact: true })).toBeVisible(); expect(held).toHaveLength(1)
})

test('模板改名、选择引用与新建草稿保持语义，离开后迟到成功不导航', async ({ page }, info) => {
  await mockFacts(page); const errors: string[] = []; page.on('pageerror', error => errors.push(error.message)); let displayName = '合成模板'; const drafts: Route[] = []
  await page.route('**/api/v1/export-config-templates', route => route.fulfill({ json: { items: [{ ...template, displayName }] } }))
  await page.route('**/api/v1/export-config-templates/synthetic-template', route => { displayName = route.request().postDataJSON().displayName; return route.fulfill({ json: { revision: 2 } }) })
  await page.route('**/api/v1/export-config-templates/*:create-draft', route => { drafts.push(route) })
  const source = DATA_SOURCE_UI_FIXTURES.find(item => item.state === 'ENABLED' && item.lastTestStatus === 'SUCCEEDED')!
  await page.route('**/api/v1/data-sources', route => route.fulfill({ json: { items: [source], total: 1, nextCursor: '' } }))
  await page.route('**/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT', route => route.fulfill({ json: { items: [{ id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64' }] } }))
  await page.goto('/templates'); await page.getByRole('button', { name: '改名', exact: true }).click(); await page.getByRole('textbox', { name: '模板名称', exact: true }).fill(' 合成改名 '); await page.getByRole('button', { name: '保存', exact: true }).click(); await expect(page.getByText('合成改名', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '用模板新建草稿', exact: true }).click(); await expect(page.getByText('请选择数据源与执行节点。', { exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: /^数据源/ }).focus(); await page.keyboard.press('ArrowDown'); await page.getByText(source.displayName, { exact: true }).last().click(); await page.getByRole('combobox', { name: /^执行节点/ }).focus(); await page.keyboard.press('ArrowDown'); await page.getByText('合成节点 · WINDOWS_AMD64', { exact: true }).click()
  await page.getByRole('button', { name: '用模板新建草稿', exact: true }).click(); await expect.poll(() => drafts.length).toBe(1)
  expect(drafts[0]!.request().postDataJSON()).toEqual({ dataSourceId: source.id, nodeId: 'synthetic-node' }); await navigate(page, '/settings'); await drafts[0]!.fulfill({ json: { draftId: 'synthetic-late', templateId: 'synthetic-template' } }).catch(() => {})
  await expect(page).toHaveURL(/\/settings$/); expect(drafts).toHaveLength(1); expect(errors).toEqual([])
  await navigate(page, '/templates'); await page.setViewportSize({ width: 640, height: 720 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('template-return.png'), fullPage: true })
})
