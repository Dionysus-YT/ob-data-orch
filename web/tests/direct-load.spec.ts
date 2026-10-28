import { chromium, expect, test, type Page } from '@playwright/test'
import { readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { normalImportNode, normalImportSources } from './fixtures/normal-import'
import type { DirectLoadConfig } from '../src/api/directLoad'

test.use({ actionTimeout: 15000 })
const metadata = JSON.parse(await readFile(new URL('../../internal/parammeta/direct/obloader-4.3.5-direct-v1.json', import.meta.url), 'utf8'))
const source = { ...normalImportSources[0]!, id: 'synthetic-direct-source', displayName: '合成旁路目标' }
const observerSource = { ...source, id: 'synthetic-observer-source', displayName: '合成旁路直连目标', connectionKind: 'OBSERVER_DIRECT', port: 2881, clusterName: '' }
const node = { ...normalImportNode, id: 'synthetic-direct-node', displayName: '合成旁路节点' }
const checks = ['TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'DATABASE_CONNECTIVITY', 'RPC_CONNECTIVITY', 'DIRECT_LOAD_VERSION', 'WRITE_ACCESS', 'DIRECT_LOAD_STRUCTURE', 'DIRECT_LOAD_CONFIG']
function completeConfig(): DirectLoadConfig { return { dataSourceId: source.id, nodeId: node.id, database: 'synthetic_database', scenario: 'PRIVATE_ODP', rpcPort: 2885, table: 'synthetic_table', filePath: 'E:\\synthetic-direct', format: 'CSV', externalData: false, columnSeparator: '', thread: '', parallel: '', replaceData: false, truncateTable: false, applicability: { singleTable:true,wholeTableCommit:true,noCheckpoint:true,structureLimits:true } } }
async function mockDirect(page: Page, saved = false, enabled = false, derivation?: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH', checkMode: 'UNKNOWN' | 'FAILED' | 'EXPIRED' = 'UNKNOWN', initialConfig = completeConfig()) {
  let config = initialConfig, revision = 1
  const writes: { path: string; method: string; body: Record<string, unknown> }[] = [], errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const envelope = () => ({ id:'direct-draft',revision,configVersion:'direct-load-v1',config,configFingerprint:'a'.repeat(64),metadataVersion:metadata.metadataVersion,capabilityVersion:metadata.capabilityVersion,executionEnabled:enabled,rpcPortSource:'MANUAL',...(derivation?{sourceTaskId:'synthetic-parent',sourceDerivation:derivation,parametersLocked:derivation==='RERUN_FROM_SCRATCH'}:{}) })
  await page.route('**/api/v1/**', async route => {
    const request=route.request(), path=new URL(request.url()).pathname, method=request.method()
    if (method !== 'GET') { expect(request.headers()['x-csrf-token']).toBeTruthy();expect(request.headers()['idempotency-key']).toBeTruthy();writes.push({path,method,body:request.postDataJSON()});if(path.includes('/direct-draft')) expect(request.headers()['if-match']).toBe(`"rev-${revision}"`) }
    if(path==='/api/v1/data-sources')return route.fulfill({json:{items:[source,observerSource]}})
    if(path==='/api/v1/execution-nodes')return route.fulfill({json:{items:[node]}})
    if(path==='/api/v1/direct-load-parameter-metadata')return route.fulfill({json:metadata})
    if(path==='/api/v1/direct-load-drafts' && method==='POST'){config=request.postDataJSON().config;saved=true;return route.fulfill({status:201,json:envelope()})}
    if(path==='/api/v1/direct-load-drafts/direct-draft' && saved){if(method==='PATCH'){config=request.postDataJSON().config;revision++}return route.fulfill({json:envelope()})}
    if(path.endsWith(':preview-command'))return route.fulfill({json:{redactedCommand:'obloader --direct --csv --table synthetic_table --rpc-port 2885 -p ******',configFingerprint:'a'.repeat(64),executionEnabled:enabled}})
    if(path.endsWith(':precheck'))return route.fulfill({status:202,json:{id:'direct-check',checkKind:'DIRECT_LOAD_PREFLIGHT'}})
    if (path === '/api/v1/direct-load-prechecks/direct-check') {
      const passed = enabled || checkMode === 'EXPIRED'
      return route.fulfill({ json: {
        id: 'direct-check', draftId: 'direct-draft', draftRevision: revision, configFingerprint: 'a'.repeat(64),
        checkKind: 'DIRECT_LOAD_PREFLIGHT', status: passed ? 'SUCCEEDED' : 'FAILED', integrityStatus: 'COMPLETE',
        validUntil: checkMode === 'EXPIRED' ? '2000-01-01T00:00:00Z' : new Date(Date.now() + 60000).toISOString(), executionEnabled: enabled,
        results: checks.map(check => ({ check, status: passed ? 'PASSED' : checkMode, evidenceCode: passed ? 'DIRECT_LOAD_VERIFIED' : checkMode === 'FAILED' ? 'DIRECT_LOAD_REJECTED' : 'DIRECT_LOAD_PROBE_NOT_CONFIGURED' })),
        ...(passed ? { directLoadConfigFingerprint: 'd'.repeat(64), inputManifest: { fileCount: 1, totalBytes: 1, digest: 'b'.repeat(64), files: [{ path: 'synthetic_table.csv', kind: 'DATA', object: 'synthetic_table', size: 1, modified: '1', sha256: 'c'.repeat(64) }] } } : {}),
      } })
    }
    if(path.endsWith(':submit'))return route.fulfill({status:201,json:{id:'synthetic-direct-task'}})
    return route.fulfill({status:404,json:{}})
  })
  return {writes,errors}
}
async function select(page:Page,label:string,option:string){const control=page.getByRole('combobox',{name:new RegExp('^(?:\\* )?'+label+'$')});const select=page.locator('.ant-select').filter({has:control});if(await control.isDisabled()){await expect(select).toContainText(option);return}await select.click();await page.locator('.ant-select-dropdown:visible .ant-select-item-option-content').getByText(option,{exact:true}).click();await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)}
async function next(page:Page,name:string){await page.getByRole('button',{name:`下一步：${name}`,exact:true}).click()}
async function chooseSource(page: Page) { await page.getByRole('radiogroup', { name: '目标数据源', exact: true }).getByRole('radio', { name: /^合成旁路目标/ }).check() }
async function switchConnectionSource(page: Page, observer: boolean) { const radio=page.getByRole('radiogroup', { name: '目标数据源', exact: true }).getByRole('radio', { name: observer ? /^合成旁路直连目标/ : /^合成旁路目标/ }); await radio.click(); const dialog=page.getByRole('dialog',{name:'更换目标数据源？',exact:true}); await expect(dialog).toBeVisible(); await dialog.getByRole('button',{name:'更换并清除关联配置',exact:true}).click(); await expect(dialog).toHaveCount(0); await expect(radio).toBeChecked() }
async function acceptApplicability(page: Page) { await next(page, '导入目标'); await page.getByRole('dialog', { name: '确认旁路导入适用条件', exact: true }).getByRole('button', { name: '已了解，继续配置', exact: true }).click() }

for (const width of [1920, 1440, 1280]) test(`修复后替换意图可配置且经第五步检查-${width}`, async ({ page }, info) => {
  const { writes, errors } = await mockDirect(page, true, true)
  await page.setViewportSize({ width, height: width === 1280 ? 720 : 1080 })
  await page.goto('/imports/direct/new?draft=direct-draft&step=4')
  await page.getByRole('button', { name: /并发与高风险行为/ }).click()
  const replace = page.getByRole('checkbox', { name: '替换数据（--replace-data）', exact: true })
  await expect(replace).toBeEnabled(); await replace.check()
  await expect(replace).toBeChecked()
  await expect(page.getByText('已请求替换数据，需重新预检查并确认当前配置的写入影响。', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath(`replace-intent-${width}.png`), animations: 'disabled' })
  await next(page, '预检查与提交')
  await expect(page.getByText('当前预检查通过', { exact: true })).toBeVisible()
  expect(writes.some(row => row.method === 'PATCH' && (row.body.config as DirectLoadConfig)?.replaceData === true)).toBe(true)
  expect(errors).toEqual([])
})

test('修复后结构拒绝显示具体原因和修正动作', async ({ page }, info) => {
  const { writes, errors } = await mockDirect(page, true, true)
  await page.route('**/api/v1/direct-load-prechecks/direct-check', route => route.fulfill({ json: {
    id: 'direct-check', draftId: 'direct-draft', draftRevision: 1 + writes.filter(row => row.method === 'PATCH').length, configFingerprint: 'a'.repeat(64),
    checkKind: 'DIRECT_LOAD_PREFLIGHT', status: 'FAILED', integrityStatus: 'COMPLETE', validUntil: new Date(Date.now() + 60000).toISOString(), executionEnabled: true,
    directLoadConfigFingerprint: 'd'.repeat(64),
    results: checks.map(check => ({ check, status: check === 'DIRECT_LOAD_STRUCTURE' ? 'FAILED' : 'PASSED', evidenceCode: check === 'DIRECT_LOAD_STRUCTURE' ? 'DIRECT_LOAD_REPLACE_LOB_BLOCKED' : 'DIRECT_LOAD_VERIFIED' })),
    inputManifest: { fileCount: 1, totalBytes: 1, digest: 'b'.repeat(64), files: [{ path: 'synthetic_table.csv', kind: 'DATA', object: 'synthetic_table', size: 1, modified: '1', sha256: 'c'.repeat(64) }] },
  } }))
  await page.goto('/imports/direct/new?draft=direct-draft&step=5')
  const row = page.locator('.wizard-precheck-table tr').filter({ hasText: '结构与替换条件' })
  await expect(row).toContainText('目标表非空且有 LOB 列')
  await expect(row).toContainText('请关闭“替换数据”并重新检查')
  await expect(page.getByRole('button', { name: '提交并启动旁路导入', exact: true })).toBeDisabled()
  await row.scrollIntoViewIfNeeded()
  await page.screenshot({ path: info.outputPath('structure-specific-reason.png'), animations: 'disabled' })
  expect(errors).toEqual([])
})

test('修复后页面断网重试保持同一请求且名称冻结', async ({ page }) => {
  await mockDirect(page, true, true)
  const keys: string[] = [], bodies: unknown[] = []
  await page.route('**/api/v1/direct-load-drafts/direct-draft:submit', route => {
    keys.push(route.request().headers()['idempotency-key']!); bodies.push(route.request().postDataJSON())
    return keys.length === 1 ? route.abort('connectionreset') : route.fulfill({ status: 201, json: { id: 'synthetic-original-task' } })
  })
  await page.goto('/imports/direct/new?draft=direct-draft&step=5')
  await page.getByRole('button', { name: '提交并启动旁路导入', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '确认提交旁路导入', exact: true })
  await dialog.getByRole('textbox', { name: '任务名称', exact: true }).fill('合成重试任务')
  await dialog.getByRole('checkbox').check()
  await dialog.getByRole('button', { name: '确认整体提交并启动', exact: true }).click()
  await expect(dialog.getByRole('textbox', { name: '任务名称', exact: true })).toBeDisabled()
  await dialog.getByRole('button', { name: '确认整体提交并启动', exact: true }).click()
  await expect(page).toHaveURL(/synthetic-original-task/)
  expect(keys).toHaveLength(2); expect(keys[0]).toBe(keys[1]); expect(bodies[0]).toEqual(bodies[1])
})

test('本地部署只提供两种连接场景，目录过滤部署外入口和说明', async ({ page }, info) => {
  const { errors } = await mockDirect(page)
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/imports/direct/new')
  await chooseSource(page)
  await expect(page.getByRole('combobox', { name: /^(?:\* )?连接场景$/ })).toBeDisabled()
  await select(page, '连接场景', '通过 ODP 连接')
  await page.screenshot({ path: info.outputPath('direct-local-scenarios.png') })
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '全部参数', exact: true }).click()
  const catalog = page.getByRole('dialog', { name: '旁路导入全部参数', exact: true })
  await expect(catalog).toContainText('共 102 个 CLI 参数')
  await expect(catalog).not.toContainText(/云|Cloud|public-cloud|3307/)
  const search = catalog.getByRole('textbox', { name: '搜索参数、含义或旁路适用状态', exact: true })
  await search.fill('public-cloud')
  await expect(catalog.getByText('--public-cloud', { exact: true })).toHaveCount(0)
  await search.fill('rpc-port')
  await expect(catalog).toContainText('私有 ODP 默认 2885，OBServer 默认 2882')
  await page.screenshot({ path: info.outputPath('direct-local-rpc-catalog.png') })
  expect(errors).toEqual([])
})

test('历史部署场景不能继续、预检查或提交，重新选择前保留原端口', async ({ page }) => {
  const config = completeConfig(); config.scenario = 'CLOUD_ODP'; config.rpcPort = 3307
  const { writes } = await mockDirect(page, true, true, undefined, 'UNKNOWN', config)
  await page.goto('/imports/direct/new?draft=direct-draft&step=1')
  await expect(page.getByRole('spinbutton', { name: 'RPC 端口', exact: true })).toHaveValue('3307')
  await next(page, '导入目标')
  await expect(page.getByText('当前配置不适用于本地部署，请重新选择连接场景。', { exact: true })).toBeVisible()
  await expect(page.getByRole('dialog', { name: '确认旁路导入适用条件', exact: true })).toHaveCount(0)
  await page.goto('/imports/direct/new?draft=direct-draft&step=5')
  await expect(page.getByText('配置尚未完整，不能预检查', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '提交并启动旁路导入', exact: true })).toBeDisabled()
  expect(writes).toEqual([])
})

test('入口确认可取消、仅明确继续时记录，筛选与RPC默认值不授予执行资格', async ({ page }, info) => {
  const { writes, errors } = await mockDirect(page)
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/imports/direct/new')
  await page.getByRole('textbox', { name: '按名称筛选数据源', exact: true }).fill('不存在的合成来源')
  await expect(page.getByText('当前筛选无可选数据源', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: '按名称筛选数据源', exact: true }).fill('合成旁路')
  await select(page, '环境', '生产')
  await expect(page.getByText('当前筛选无可选数据源', { exact: true })).toBeVisible()
  await select(page, '环境', '测试')
  await chooseSource(page)
  await select(page, '连接场景', '通过 ODP 连接')
  const rpc = page.getByRole('spinbutton', { name: 'RPC 端口', exact: true })
  await expect(rpc).toHaveValue('2885')
  await rpc.fill('12885')
  await switchConnectionSource(page, true)
  await expect(rpc).toHaveValue('12885')
  await rpc.fill('')
  await switchConnectionSource(page, false)
  await expect(rpc).toHaveValue('2885')
  await switchConnectionSource(page, true)
  await expect(rpc).toHaveValue('2882')
  await switchConnectionSource(page, false)
  await expect(rpc).toHaveValue('2885')
  await page.screenshot({ path: info.outputPath('direct-source-default-rpc.png') })
  await next(page, '导入目标')
  const dialog = page.getByRole('dialog', { name: '确认旁路导入适用条件', exact: true })
  await expect(dialog.getByRole('button', { name: '返回配置', exact: true })).toBeFocused()
  for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
    await page.setViewportSize(viewport)
    await expect(dialog.getByRole('button', { name: '返回配置', exact: true })).toBeInViewport()
    await expect(dialog.getByRole('button', { name: '已了解，继续配置', exact: true })).toBeInViewport()
    await page.screenshot({ path: info.outputPath(`direct-applicability-${viewport.width}.png`) })
  }
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('button', { name: '下一步：导入目标', exact: true })).toBeFocused()
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  await expect(page).toHaveURL(/draft=direct-draft/)
  expect((writes.find(write => write.method === 'POST')!.body.config as DirectLoadConfig).applicability).toEqual({ singleTable: false, wholeTableCommit: false, noCheckpoint: false, structureLimits: false })
  await acceptApplicability(page)
  await expect(page.getByRole('heading', { name: '导入目标', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '上一步', exact: true }).click()
  await next(page, '导入目标')
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('heading', { name: '导入目标', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  await expect(page.locator('.page-heading')).toContainText('草稿已保存')
  expect((writes.find(write => write.method === 'PATCH')!.body.config as DirectLoadConfig).applicability).toEqual({ singleTable: true, wholeTableCommit: true, noCheckpoint: true, structureLimits: true })
  expect(writes.some(write => /:(precheck|submit)$/.test(write.path))).toBe(false)
  expect(errors).toEqual([])
})

test('旧最终步链接归一到第五步，缺少入口确认时不能预检查或提交', async ({ page }) => {
  const config = completeConfig(); config.applicability.singleTable = false
  const { writes } = await mockDirect(page, true, true, undefined, 'UNKNOWN', config)
  await page.goto('/imports/direct/new?draft=direct-draft&step=6')
  await expect(page.locator('[aria-current="step"]')).toContainText('预检查与提交')
  await expect(page.getByText('配置尚未完整，不能预检查', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '提交并启动旁路导入', exact: true })).toBeDisabled()
  expect(writes).toEqual([])
})

test('旁路水平进度按当前输入更新，跳步不补齐配置，节点与来源事实使用统一显示', async ({ page }) => {
  await mockDirect(page)
  await page.goto('/imports/direct/new')
  const rail = page.getByLabel('任务创建步骤', { exact: true })
  await expect(rail.locator('.ant-steps-item')).toHaveCount(5)
  await expect(rail.locator('[aria-current="step"]')).toContainText('当前步骤 · 0%')
  await expect(page.getByRole('checkbox')).toHaveCount(0)
  await page.getByRole('button', { name: '全部参数', exact: true }).click()
  await page.getByRole('textbox', { name: '搜索参数、含义或旁路适用状态', exact: true }).fill('file-path')
  await page.getByRole('button', { name: '定位步骤 3', exact: true }).click()
  await expect(rail.locator('[aria-current="step"]')).toContainText('文件与格式')
  await expect(rail.getByLabel('返回第 1 步：数据源与连接', { exact: true })).toContainText('待完善')
  await rail.getByLabel('返回第 1 步：数据源与连接', { exact: true }).click()
  await chooseSource(page)
  const selectedFacts=page.locator('.wizard-source-facts').filter({hasText:'合成旁路目标'})
  await expect(selectedFacts).toContainText('测试')
  await expect(selectedFacts).not.toContainText('TEST')
  await expect(selectedFacts).not.toContainText('T00:')
  await expect(rail.locator('[aria-current="step"]')).toContainText('当前步骤 · 75%')
  await select(page, '连接场景', '通过 ODP 连接')
  await expect(page.getByRole('spinbutton', { name: 'RPC 端口', exact: true })).toHaveValue('2885')
  await acceptApplicability(page)
  await page.getByRole('textbox', { name: '唯一目标表', exact: true }).fill('synthetic_table')
  await next(page, '文件与格式')
  await select(page, '执行节点', '合成旁路节点 · Windows · x86_64')
  await expect(rail.locator('[aria-current="step"]')).toContainText('当前步骤 · 33%')
  await page.getByRole('textbox', { name: '文件或目录路径', exact: true }).fill('E:\\synthetic-direct')
  await select(page, '文件格式', 'CSV')
  await expect(rail.locator('[aria-current="step"]')).toContainText('当前步骤 · 100%')
  await rail.getByLabel('返回第 2 步：导入目标', { exact: true }).click()
  await expect(page.getByRole('textbox', { name: '目标数据库 / Schema', exact: true })).toHaveValue(source.defaultDatabase!)
})

for (const mode of ['UNKNOWN', 'FAILED', 'EXPIRED'] as const) test(`旁路 ${mode} 总览、摘要及同类操作规格`, async ({ page }, info) => {
  await mockDirect(page, true, false, undefined, mode)
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/imports/direct/new?draft=direct-draft&step=6')
  const message = mode === 'EXPIRED' ? '旁路预检查已失效，请重新检查' : mode === 'FAILED' ? '旁路预检查未通过，提交已阻断' : '旁路预检查存在无法确认项，提交已阻断'
  await expect(page.getByText(message, { exact: true })).toBeVisible()
  await expect(page.locator('.ant-alert').filter({ hasText: message })).toHaveClass(new RegExp(mode === 'EXPIRED' ? 'ant-alert-warning' : 'ant-alert-error'))
  await expect(page.locator('[aria-current="step"]')).toContainText('当前步骤 · 0%')
  await expect(page.getByRole('button', { name: '提交并启动旁路导入', exact: true })).toBeDisabled()
  if (mode === 'FAILED') expect(await page.getByRole('button', { name: '检查配置', exact: true }).first().evaluate(element => element.getBoundingClientRect().height)).toBe(27)
  else await expect(page.getByRole('button', { name: '检查配置', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '查看执行节点', exact: true })).toHaveCount(mode === 'UNKNOWN' ? 1 : 0)
  if (mode === 'UNKNOWN') await expect(page.getByText('DIRECT_LOAD_PROBE_NOT_CONFIGURED', { exact: true })).toHaveCount(1)
  expect(await page.locator('.footer-actions').evaluate(element => getComputedStyle(element).columnGap)).toBe('8px')
  await page.screenshot({ path: info.outputPath(`direct-${mode.toLowerCase()}-review.png`) })
  await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
  const summary = page.getByRole('dialog', { name: '任务摘要', exact: true })
  const metrics = await summary.evaluate(element => {
    const body = element.querySelector('.ant-drawer-body')!, label = element.querySelector('dt')!, heading = element.querySelector('.wizard-summary-section h3')!
    return { inset: label.getBoundingClientRect().left - body.getBoundingClientRect().left, font: getComputedStyle(heading).fontSize, line: getComputedStyle(heading).lineHeight, weight: getComputedStyle(heading).fontWeight }
  })
  expect(metrics).toEqual({ inset: 24, font: '16px', line: '24px', weight: '600' })
  await expect(summary).toContainText(message)
  await page.screenshot({ path: info.outputPath(`direct-${mode.toLowerCase()}-summary.png`) })
})

test('旁路探针未配置只提供当前节点入口，不引导修改业务参数或放开提交', async ({ page }) => {
  const { writes } = await mockDirect(page, true)
  await page.goto('/imports/direct/new?draft=direct-draft&step=5')
  await expect(page.getByText('无法确认', { exact: true })).toHaveCount(9)
  await expect(page.getByText('DIRECT_LOAD_PROBE_NOT_CONFIGURED', { exact: true })).toHaveCount(1)
  await expect(page.getByRole('button', { name: '检查配置', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '提交并启动旁路导入', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '查看执行节点', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes\/synthetic-direct-node$/)
  expect(writes.some(row => row.path.endsWith(':submit'))).toBe(false)
})

test('旁路五步配置、保存恢复、自动检查与独立门禁闭环',async({page},info)=>{
  const {writes,errors}=await mockDirect(page)
  await page.setViewportSize({width:1920,height:1080});await page.goto('/imports/direct/new')
  await expect(page.locator('.wizard-ant-step-rail .ant-steps-item')).toHaveCount(5)
  await next(page,'导入目标');await expect(page.getByRole('dialog',{name:'确认旁路导入适用条件',exact:true})).toHaveCount(0)
  await chooseSource(page);await select(page,'连接场景','通过 ODP 连接')
  await expect(page.getByRole('spinbutton',{name:'RPC 端口',exact:true})).toHaveValue('2885')
  expect(writes.some(row => row.path.endsWith(':precheck'))).toBe(false);await acceptApplicability(page)
  await page.getByRole('textbox',{name:'唯一目标表',exact:true}).fill('synthetic_table');await next(page,'文件与格式')
  await select(page,'执行节点','合成旁路节点 · Windows · x86_64');await page.getByRole('textbox',{name:'文件或目录路径',exact:true}).fill('E:\\synthetic-direct')
  await select(page,'文件格式','CSV')
  await page.getByRole('button',{name:'保存草稿',exact:true}).click();await expect(page).toHaveURL(/draft=direct-draft/);await expect(page.locator('.page-heading')).toContainText('草稿已保存')
  await next(page,'执行参数');await next(page,'预检查与提交')
  await expect(page.getByRole('region',{name:'完整脱敏命令'}).locator('pre')).toContainText('--direct')
  await expect(page.getByText('无法确认',{exact:true})).toHaveCount(9)
  await expect(page.getByRole('button',{name:'提交并启动旁路导入',exact:true})).toBeDisabled()
  expect(writes.filter(item=>item.path.endsWith(':precheck'))).toHaveLength(1)
  await page.screenshot({path:info.outputPath('direct-load-1920-review.png'),fullPage:false})
  await page.getByRole('button',{name:'查看任务摘要',exact:true}).click();await expect(page.getByRole('heading',{name:'任务摘要',exact:true})).toBeVisible();await page.screenshot({path:info.outputPath('direct-load-summary.png')});await page.keyboard.press('Escape');await expect(page.getByRole('heading',{name:'任务摘要',exact:true})).not.toBeVisible();await expect(page.getByRole('button',{name:'查看任务摘要',exact:true})).toBeFocused()
  await page.getByRole('button',{name:'全部参数',exact:true}).click();await expect(page.getByText(/共 102 个 CLI 参数/)).toBeVisible();await page.getByRole('textbox',{name:'搜索参数、含义或旁路适用状态',exact:true}).fill('retry');await expect(page.getByText('--retry',{exact:true})).toBeVisible();await expect(page.getByText('不适用 / 不生成',{exact:true})).toBeVisible();await page.keyboard.press('Escape')
  await page.getByLabel('返回第 2 步：导入目标',{exact:true}).click();await expect(page.getByRole('textbox',{name:'唯一目标表',exact:true})).toHaveValue('synthetic_table')
  await page.getByRole('textbox',{name:'唯一目标表',exact:true}).fill('unsaved_table');await page.getByRole('link',{name:'首页',exact:true}).click();await expect(page.getByRole('dialog',{name:'离开旁路导入配置？',exact:true})).toBeVisible();await page.getByRole('button',{name:'继续配置',exact:true}).click();await expect(page.getByRole('textbox',{name:'唯一目标表',exact:true})).toHaveValue('unsaved_table')
  page.once('dialog',dialog=>dialog.accept());await page.reload();await expect(page.getByRole('textbox',{name:'唯一目标表',exact:true})).toHaveValue('synthetic_table')
  expect(writes.some(item=>item.path.endsWith(':submit'))).toBe(false);expect(errors).toEqual([])
})

for(const viewport of [{width:1920,height:1080},{width:1440,height:1024},{width:1280,height:720}])test(`旁路 ${viewport.width} 五步工作面与键盘弹层`,async({page},info)=>{
  const {errors}=await mockDirect(page,true);await page.setViewportSize(viewport);await page.goto('/imports/direct/new?draft=direct-draft&step=1')
  await expect(page.getByRole('radiogroup',{name:'目标数据源',exact:true})).toBeVisible()
  // 低视口沿用工作面正常滚动；定位后仍须整项可见且不被固定底栏遮挡。
  const selectedSource=page.locator('.wizard-source-choice').filter({has:page.getByRole('radio',{name:/^合成旁路目标/})})
  await selectedSource.evaluate(element => element.scrollIntoView({ block: 'center' }))
  await expect(selectedSource).toBeInViewport({ ratio: 1 })
  const sourceBounds = await selectedSource.boundingBox(), footerBounds = await page.locator('.wizard-footer').boundingBox()
  expect(sourceBounds!.y + sourceBounds!.height).toBeLessThanOrEqual(footerBounds!.y)
  await page.screenshot({path:info.outputPath(`direct-load-${viewport.width}-source.png`)})
  await next(page,'导入目标')
  await expect(page.getByRole('textbox',{name:'唯一目标表',exact:true})).toHaveValue('synthetic_table');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.screenshot({path:info.outputPath(`direct-load-${viewport.width}-target.png`)})
  await next(page,'文件与格式');await expect(page.getByRole('textbox',{name:'文件或目录路径',exact:true})).toHaveValue('E:\\synthetic-direct');await page.screenshot({path:info.outputPath(`direct-load-${viewport.width}-files.png`)})
  await next(page,'执行参数');await page.getByRole('button',{name:'查看任务摘要',exact:true}).focus();await page.keyboard.press('Enter');await expect(page.getByRole('heading',{name:'任务摘要',exact:true})).toBeFocused();await page.keyboard.press('Escape');await expect(page.getByRole('button',{name:'查看任务摘要',exact:true})).toBeFocused()
  await next(page,'预检查与提交');await expect(page.getByText('无法确认',{exact:true})).toHaveCount(9);await expect(page.getByRole('button',{name:'提交并启动旁路导入',exact:true})).toBeDisabled();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:info.outputPath(`direct-load-${viewport.width}-review.png`)})
  expect(errors).toEqual([])
})

test('本地格式仅开放CSV，历史未支持格式不静默转换且不能继续',async({page})=>{
  const config = completeConfig(); config.format = 'SQL'
  await mockDirect(page,true,false,undefined,'UNKNOWN',config);await page.goto('/imports/direct/new?draft=direct-draft&step=3');await next(page,'执行参数');await expect(page.getByText('当前连接场景尚未确认此格式的旁路适用性。',{exact:true})).toBeVisible();await select(page,'文件格式','CSV');await next(page,'执行参数');await expect(page.getByRole('heading',{name:'执行参数',exact:true})).toBeVisible()
})

test('旁路无权或读取失败不展示旧草稿，也不开放保存',async({page})=>{
  const writes:string[]=[];await page.route('**/api/v1/**',route=>{if(route.request().method()!=='GET')writes.push(route.request().url());return route.fulfill({status:403,json:{}})});await page.goto('/imports/direct/new?draft=unreadable&step=4');await expect(page.getByText('无法读取旁路导入草稿，请检查权限后重试。',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'保存草稿',exact:true})).toBeDisabled();await expect(page.getByRole('textbox',{name:'唯一目标表',exact:true})).toHaveCount(0);expect(writes).toEqual([])
})

test('合成独立执行开关开放后仍须任务名称和最终风险确认', async ({ page }, info) => {
  const { writes, errors } = await mockDirect(page, true, true)
  await page.goto('/imports/direct/new?draft=direct-draft&step=6')
  const submit = page.getByRole('button', { name: '提交并启动旁路导入', exact: true })
  await expect(submit).toBeEnabled()
  await expect(page.getByText('当前预检查通过', { exact: true })).toBeVisible()
  await submit.click()
  const confirmation = page.getByRole('dialog', { name: '确认提交旁路导入', exact: true })
  await expect(confirmation).toBeVisible()
  const confirm = confirmation.getByRole('button', { name: '确认整体提交并启动', exact: true })
  await confirm.click()
  expect(writes.filter(item => item.path.endsWith(':submit'))).toHaveLength(0)
  await confirmation.getByRole('checkbox').check()
  await confirm.click()
  await expect(confirmation.getByText('请填写 1～128 字的任务名称。', { exact: true })).toBeVisible()
  await confirmation.getByRole('textbox', { name: '任务名称', exact: true }).fill('合成旁路风险确认任务')
  await page.screenshot({ path: info.outputPath('direct-load-submit-confirmation.png') })
  await confirm.click()
  await expect(page).toHaveURL(/\/tasks\/synthetic-direct-task$/)
  const submissions = writes.filter(item => item.path.endsWith(':submit'))
  expect(submissions).toHaveLength(1)
  expect(submissions[0]!.body).toEqual({ precheckId: 'direct-check', taskName: '合成旁路风险确认任务', confirmedConfigFingerprint: 'a'.repeat(64) })
  expect(errors).toEqual([])
})

test('旁路从头执行只读复验，基于原配置新建可改参', async ({ page }) => {
  const locked = await mockDirect(page, true, false, 'RERUN_FROM_SCRATCH')
  await page.goto('/imports/direct/new?draft=direct-draft&step=2')
  await expect(page.getByRole('heading', { name: '预检查、完整命令与风险确认', exact: true })).toBeVisible()
  await expect(page.getByText('无法确认', { exact: true })).toHaveCount(9)
  await expect(page.getByRole('button', { name: '上一步', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: '保存草稿', exact: true })).toBeDisabled()
  await expect(page.getByRole('combobox')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '检查配置', exact: true })).toHaveCount(0)
  for (const write of locked.writes.filter(item => item.method === 'PATCH')) expect(write.body.config).toEqual(completeConfig())
  await page.unroute('**/api/v1/**')
  const editable = await mockDirect(page, true, false, 'REBUILD_FROM_CONFIG')
  await page.goto('/imports/direct/new?draft=direct-draft&step=2')
  await page.getByRole('textbox', { name: '唯一目标表', exact: true }).fill('synthetic_new_table')
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  await expect(page.locator('.page-heading')).toContainText('草稿已保存')
  expect(editable.writes.filter(item => item.method === 'PATCH').at(-1)?.body.config).toMatchObject({ table: 'synthetic_new_table' })
})

test('旁路原生 200% 缩放下五步、摘要与门禁操作可达', async ({ browserName }, info) => {
  test.setTimeout(180000)
  expect(browserName).toBe('chromium')
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url))
  const context = await chromium.launchPersistentContext('', {
    channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, baseURL: 'http://127.0.0.1:15174',
    args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage()
    const config = completeConfig(); config.applicability.singleTable = false
    await mockDirect(page, true, false, undefined, 'UNKNOWN', config)
    await page.goto('/imports/direct/new?draft=direct-draft&step=1')
    await expect(page.getByRole('spinbutton', { name: 'RPC 端口', exact: true })).toHaveValue('2885')
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(options: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const target = (await api.tabs.query({})).find(tab => tab.url.includes('127.0.0.1:15174'))
      if (!target) throw new Error('隔离测试标签不存在')
      await api.tabs.setZoom(target.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await next(page, '导入目标')
    const applicability = page.getByRole('dialog', { name: '确认旁路导入适用条件', exact: true })
    await expect(applicability.getByRole('button', { name: '返回配置', exact: true })).toBeInViewport()
    await expect(applicability.getByRole('button', { name: '已了解，继续配置', exact: true })).toBeInViewport()
    const actionBounds = await applicability.getByRole('button', { name: '已了解，继续配置', exact: true }).boundingBox()
    const viewport = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }))
    expect(actionBounds!.x + actionBounds!.width).toBeLessThanOrEqual(viewport.width)
    expect(actionBounds!.y + actionBounds!.height).toBeLessThanOrEqual(viewport.height)
    // 原生缩放下沿用无裁切的可视窗口采集，避免截图助手按 CSS 尺寸截掉右半弹窗。
    const modalSession = await context.newCDPSession(page)
    const modalCapture = await modalSession.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('direct-load-200-percent-applicability.png'), Buffer.from(modalCapture.data, 'base64'))
    await applicability.getByRole('button', { name: '已了解，继续配置', exact: true }).click()
    await next(page, '文件与格式')
    await next(page, '执行参数')
    await next(page, '预检查与提交')
    await expect(page.getByText('无法确认', { exact: true })).toHaveCount(9)
    const summaryButton = page.getByRole('button', { name: '查看任务摘要', exact: true })
    await summaryButton.click()
    const summary = page.getByRole('dialog', { name: '任务摘要', exact: true })
    await expect(summary).toBeVisible()
    for (let index = 0; index < 12; index++) { await page.keyboard.press('Tab'); expect(await summary.evaluate(element => element.contains(document.activeElement))).toBe(true) }
    await page.keyboard.press('Escape')
    await expect(summaryButton).toBeFocused()
    const command = page.getByRole('region', { name: '完整脱敏命令', exact: true })
    await command.scrollIntoViewIfNeeded()
    await expect(command.getByRole('heading', { name: '完整命令（敏感信息已脱敏）', exact: true })).toBeInViewport()
    const commandBounds = await command.boundingBox(), footerBounds = await page.locator('.wizard-footer').boundingBox()
    expect(commandBounds!.y + commandBounds!.height).toBeLessThanOrEqual(footerBounds!.y)
    const submit = page.getByRole('button', { name: '提交并启动旁路导入', exact: true })
    await submit.scrollIntoViewIfNeeded()
    await expect(submit).toBeInViewport()
    await expect(submit).toBeDisabled()
    const session = await context.newCDPSession(page)
    const capture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('direct-load-native-200.png'), Buffer.from(capture.data, 'base64'))
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  } finally { await context.close() }
})
