import { expect, test, type Page } from '@playwright/test'
import { normalImportNode, normalImportSources } from './fixtures/normal-import'
import type { NormalImportConfig, NormalImportDraft } from '../src/api/normalImport'

test.use({ actionTimeout: 15000 })
async function selectOption(page: Page, label: string, option: string) {
  await page.locator('.ant-select').filter({ has: page.getByRole('combobox', { name: new RegExp('^(?:\\* )?' + label + '(?: .+参数说明)?$') }) }).click()
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option-content').getByText(option, { exact: true }).click()
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
}

test('普通导入保存恢复、第五步统一文件检查、脱敏命令和失败关闭预检查形成闭环', async ({ page }, info) => {
  const errors: string[] = [], writes: { method: string; path: string; revision?: string; config?: Record<string, unknown> }[] = []
  page.on('pageerror', error => errors.push(error.message))
  let draft: NormalImportDraft | undefined
  const envelope = () => ({ ...draft, metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1' })
  await page.route('**/api/v1/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname, method = request.method()
    if (method !== 'GET') {
      expect(request.headers()['x-csrf-token']).toBeTruthy(); expect(request.headers()['idempotency-key']).toBeTruthy()
      writes.push({ method, path, revision: request.headers()['if-match'], config: request.postDataJSON()?.config })
    }
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/normal-import-drafts' && method === 'POST') {
      draft = { id: 'synthetic-normal-draft', revision: 1, configVersion: 'normal-import-v1', config: request.postDataJSON().config as NormalImportConfig, configFingerprint: 'a'.repeat(64), executionEnabled: false }
      return route.fulfill({ status: 201, json: envelope() })
    }
    if (path === '/api/v1/normal-import-drafts/synthetic-normal-draft') {
      if (method === 'PATCH') {
        expect(request.headers()['if-match']).toBe(`"rev-${draft!.revision}"`)
        draft = { ...draft!, revision: draft!.revision + 1, config: request.postDataJSON().config as NormalImportConfig }
      }
      return route.fulfill({ json: envelope() })
    }
    if (path.endsWith(':precheck')) return route.fulfill({ status: 202, json: { id: 'full-check' } })
    if (path.endsWith(':preview-command')) return route.fulfill({ json: { redactedCommand: `obloader --host 127.0.0.1 --port 2883 --database synthetic_database --file-path E:\\synthetic-import --csv --all -p ******`, configFingerprint: draft!.configFingerprint, executionEnabled: false } })
    if (path.startsWith('/api/v1/normal-import-prechecks/')) {
      const checks = ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING']
      return route.fulfill({ json: { id: 'full-check', draftId: draft!.id, draftRevision: draft!.revision, configFingerprint: draft!.configFingerprint, checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'FAILED', integrityStatus: 'COMPLETE', validUntil: new Date(Date.now() + 60000).toISOString(), executionEnabled: false,
        results: checks.map(check => ({ check, status: ['WRITE_ACCESS', 'INPUT_MAPPING'].includes(check) ? 'UNKNOWN' : 'PASSED', evidenceCode: ['WRITE_ACCESS', 'INPUT_MAPPING'].includes(check) ? 'IMPORT_UNAVAILABLE' : 'SYNTHETIC_OK' })),
        inputManifest: { files: [{ path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 12, modified: '1790000000000000000', sha256: 'b'.repeat(64) }], fileCount: 1, totalBytes: 12, digest: 'c'.repeat(64) } } })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  await page.goto('/imports/normal/new')
  await page.getByRole('radio', { name: /合成 MySQL 导入目标/ }).check()
  await page.getByRole('button', { name: '下一步：导入文件与格式', exact: true }).click()
  await selectOption(page, '执行节点', '合成导入节点 · Windows · x86_64')
  await page.getByRole('textbox', { name: '文件或目录路径', exact: true }).fill('E:\\synthetic-import')
  await page.getByRole('radiogroup', { name: '导入内容', exact: true }).getByText('仅导入数据', { exact: true }).click()
  await selectOption(page, '数据格式', 'CSV')
  await expect(page.getByRole('button', { name: '发现输入文件', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  await expect(page).toHaveURL(/draft=synthetic-normal-draft/)
  await page.getByRole('button', { name: '下一步：对象与映射', exact: true }).click()
  await page.getByRole('radio', { name: '全部对象', exact: true }).check()
  await page.getByRole('button', { name: '下一步：执行参数', exact: true }).click()
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  await expect(page.getByRole('textbox', { name: '任务名称', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '生成命令预览', exact: true })).toHaveCount(0)
  await expect(page.getByRole('region', { name: '完整脱敏命令', exact: true }).locator('pre')).toContainText('-p ******')
  await expect(page.getByRole('button', { name: '复制脱敏命令', exact: true })).toBeEnabled()
  await expect(page.getByText('无法确认', { exact: true })).toHaveCount(2)
  await expect(page.getByText('通过', { exact: true })).toHaveCount(4)
  await expect(page.getByRole('button', { name: '提交并启动导入', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
  await expect(page.getByText('提交时可选填写，留空自动生成', { exact: true })).toBeVisible()
  await expect(page.getByText('服务端草稿已保存，刷新后可恢复当前活动配置。', { exact: true })).toBeVisible()
  await expect(page.getByText('存在无法确认项，提交已阻断', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('heading', { name: '任务摘要', exact: true })).not.toBeVisible()
  await page.getByRole('region', { name: '完整脱敏命令', exact: true }).scrollIntoViewIfNeeded()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('normal-import-frozen-review.png'), fullPage: false })
  expect(writes.filter(write => write.path.endsWith(':precheck'))).toHaveLength(1)
  expect(writes.filter(write => write.path.endsWith(':discover-files'))).toHaveLength(0)
  await page.locator('.wizard-ant-step-rail .ant-steps-item').nth(1).click()
  await page.getByRole('textbox', { name: '文件或目录路径', exact: true }).fill('E:\\unsaved-import')
  await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
  await expect(page.getByText('当前更改尚未保存；刷新将恢复上次保存的配置。', { exact: true })).toBeVisible()
  await expect(page.getByText('尚未执行完整预检查', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('heading', { name: '任务摘要', exact: true })).not.toBeVisible()
  const restoredDraft = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/normal-import-drafts/synthetic-normal-draft')
  page.once('dialog', dialog => dialog.accept()); await page.reload(); await restoredDraft
  await expect(page.getByText('正在读取普通导入草稿…', { exact: true })).not.toBeVisible()
  await expect(page.getByRole('textbox', { name: '文件或目录路径', exact: true })).toHaveValue('E:\\synthetic-import')
  await expect(page.locator('.page-heading')).toContainText('草稿已保存')
  expect(writes.filter(write => write.method === 'POST' && write.path === '/api/v1/normal-import-drafts')).toHaveLength(1)
  for (const write of writes.filter(write => write.config)) {
    expect(write.config).not.toHaveProperty('credentialRevision'); expect(write.config).not.toHaveProperty('platform'); expect(write.config).not.toHaveProperty('formats'); expect(write.config).not.toHaveProperty('direct')
  }
  expect(writes.some(write => write.path.endsWith(':submit'))).toBe(false)
  expect(errors).toEqual([])
})

test('普通导入恢复失败保留明确错误并阻断旧草稿写入', async ({ page }) => {
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    return route.fulfill({ status: 404, json: {} })
  })
  await page.goto('/imports/normal/new?draft=unreadable-draft')
  await expect(page.getByRole('alert').filter({ hasText: '无法读取普通导入草稿' })).toBeVisible()
  await expect(page.getByRole('button', { name: '保存草稿', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: '下一步：导入文件与格式', exact: true })).toBeDisabled()
  await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toHaveCount(0)
})

for (const taskName of ['', '合成自定义导入名称']) {
  test(`普通导入自动准备后提交弹窗${taskName ? '使用自定义名称' : '允许留空自动命名'}`, async ({ page }, info) => {
    await page.setViewportSize(taskName ? { width: 1920, height: 1080 } : { width: 712, height: 464 })
    let revision = 1, commands = 0, checks = 0
    let submitted: Record<string, unknown> | undefined
    let config: NormalImportConfig = { dataSourceId: normalImportSources[0]!.id, nodeId: normalImportNode.id, database: 'synthetic_database', filePath: 'E:\\synthetic-import', content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', objects: {}, parser: {}, execution: {}, cleanup: 'NONE', mapping: { kind: 'POSITION' } }
    await page.route('**/api/v1/**', route => {
      const request = route.request(), path = new URL(request.url()).pathname
      if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
      if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
      if (path === '/api/v1/normal-import-drafts/submit-draft') {
        if (request.method() === 'PATCH') { revision++; config = request.postDataJSON().config }
        return route.fulfill({ json: { id: 'submit-draft', revision, configVersion: 'normal-import-v1', config, configFingerprint: 'a'.repeat(64), metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1', executionEnabled: true } })
      }
      if (path.endsWith(':preview-command')) { commands++; return route.fulfill({ json: { redactedCommand: 'obloader --no-sys --csv -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: true } }) }
      if (path.endsWith(':precheck')) { checks++; return route.fulfill({ status: 202, json: { id: 'submit-check' } }) }
      if (path === '/api/v1/normal-import-prechecks/submit-check') return route.fulfill({ json: {
        id: 'submit-check', draftId: 'submit-draft', draftRevision: revision, configFingerprint: 'a'.repeat(64), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'SUCCEEDED', integrityStatus: 'COMPLETE', executionEnabled: true, validUntil: new Date(Date.now() + 60000).toISOString(),
        results: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(check => ({ check, status: 'PASSED', evidenceCode: check === 'DATABASE_CONNECTIVITY' ? 'DATABASE_CONNECTED' : check === 'TOOL_ENVIRONMENT' ? 'TOOL_RUNTIME_READY' : check === 'NODE_RESOURCE' ? 'NODE_RESOURCE_READY' : 'IMPORT_VERIFIED' })),
        inputManifest: { files: [{ path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 12, modified: '1790000000000000000', sha256: 'b'.repeat(64) }], fileCount: 1, totalBytes: 12, digest: 'c'.repeat(64) },
      } })
      if (path.endsWith(':submit')) { submitted = request.postDataJSON(); return route.fulfill({ status: 201, json: { id: 'synthetic-import-task' } }) }
      return route.fulfill({ status: 404, json: {} })
    })
    await page.goto('/imports/normal/new?draft=submit-draft&step=5')
    await expect(page.getByText('通过', { exact: true })).toHaveCount(6)
    await expect(page.getByRole('region', { name: '完整脱敏命令', exact: true }).locator('pre')).toContainText('obloader')
    await expect(page.getByRole('button', { name: '生成命令预览', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: '提交并启动导入', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '确认提交普通导入任务', exact: true })
    await expect(dialog).toBeVisible(); expect(submitted).toBeUndefined()
    const cancel = dialog.getByRole('button', { name: '返回检查', exact: true })
    await expect(cancel).toBeFocused()
    // 打开后不主动滚动或聚焦补救，安全返回必须完整位于当前视口。
    await expect.poll(() => cancel.evaluate(element => {
      const box = element.getBoundingClientRect()
      return box.top >= 0 && box.bottom <= innerHeight && box.left >= 0 && box.right <= innerWidth
    })).toBe(true)
    const input = dialog.getByRole('textbox', { name: '任务名称（可选）', exact: true })
    await expect(input).toHaveValue(''); await input.fill(taskName ? `  ${taskName}  ` : '')
    await expect(dialog).toContainText('留空按目标数据库、文件格式、UTC 时间与任务标识自动生成')
    expect(commands).toBe(1); expect(checks).toBe(1)
    await page.screenshot({ path: info.outputPath('normal-import-submit-name.png'), fullPage: true })
    await dialog.getByRole('button', { name: '确认提交并启动导入', exact: true }).click()
    await expect(page).toHaveURL(/\/tasks\/synthetic-import-task/)
    expect(submitted).toEqual(taskName ? { precheckId: 'submit-check', taskName } : { precheckId: 'submit-check' })
    expect(commands).toBe(1); expect(checks).toBe(1)
  })
}

test('普通导入自动检查工具前置阻断，手动重检区分未启用检查', async ({ page }, info) => {
  let revision = 1, checksStarted = 0
  let config: NormalImportConfig = { dataSourceId: normalImportSources[0]!.id, nodeId: normalImportNode.id, database: 'synthetic_database', filePath: 'E:\\synthetic-import', content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', objects: {}, parser: {}, execution: {}, cleanup: 'NONE', taskName: '合成工具检查任务', mapping: { kind: 'POSITION' } }
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/v1/**', route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/normal-import-drafts/synthetic-tool-draft') {
      if (request.method() === 'PATCH') { revision++; config = request.postDataJSON().config }
      return route.fulfill({ json: { id: 'synthetic-tool-draft', revision, configVersion: 'normal-import-v1', config, configFingerprint: 'a'.repeat(64), metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1', executionEnabled: false } })
    }
    if (path.endsWith(':precheck')) { checksStarted++; return route.fulfill({ status: 202, json: { id: `tool-check-${checksStarted}` } }) }
    if (path.endsWith(':preview-command')) return route.fulfill({ json: { redactedCommand: 'obloader --no-sys -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: false } })
    if (path.startsWith('/api/v1/normal-import-prechecks/')) {
      const failedTool = path.endsWith('tool-check-1')
      return route.fulfill({ json: {
        id: path.split('/').at(-1), draftId: 'synthetic-tool-draft', draftRevision: revision, configFingerprint: 'a'.repeat(64), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'FAILED', integrityStatus: 'COMPLETE', executionEnabled: false, validUntil: new Date(Date.now() + 60000).toISOString(),
        results: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(name => {
          if (name === 'TOOL_ENVIRONMENT') return { check: name, status: failedTool ? 'FAILED' : 'PASSED', evidenceCode: failedTool ? 'TOOL_RUNTIME_INVALID' : 'TOOL_RUNTIME_READY' }
          if (['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'INPUT_MAPPING'].includes(name)) return { check: name, status: 'UNKNOWN', evidenceCode: failedTool ? 'PRECHECK_BLOCKED_BY_TOOL' : name === 'DATABASE_CONNECTIVITY' ? 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED' : 'PRECHECK_BLOCKED_BY_DATABASE' }
          return { check: name, status: 'PASSED', evidenceCode: 'SYNTHETIC_OK' }
        }),
        inputManifest: { files: [{ path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 12, modified: '1790000000000000000', sha256: 'b'.repeat(64) }], fileCount: 1, totalBytes: 12, digest: 'c'.repeat(64) },
      } })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  await page.goto('/imports/normal/new?draft=synthetic-tool-draft&step=5')
  const table = page.getByRole('region', { name: '普通导入任务预检查', exact: true })
  await expect(table.getByText('前置阻断，未执行', { exact: true })).toHaveCount(3)
  await expect(table.getByText('未通过', { exact: true })).toHaveCount(1)
  await expect(table.getByText('Java、OBLOADER 或本机依赖布局不符合要求，请在执行节点中检查环境。', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('normal-import-tool-blocked.png'), fullPage: true })
  expect(checksStarted).toBe(1)
  await page.getByRole('button', { name: '重新预检查', exact: true }).click()
  await expect(table.getByText('未启用', { exact: true })).toHaveCount(1)
  await expect(table.getByText('前置阻断，未执行', { exact: true })).toHaveCount(2)
  await expect(table.getByText('通过', { exact: true })).toHaveCount(3)
  await expect(table.getByText('当前节点尚未启用普通导入数据库连接检查，请更新 Agent 并核对本机只读 JDBC 检查配置。', { exact: true })).toBeVisible()
  await expect(table.getByText('未通过', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '提交并启动导入', exact: true })).toBeDisabled()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('normal-import-unconfigured.png'), fullPage: true })
  expect(errors).toEqual([])
})

test('普通导入低堆阻断提交，调整配置后重新检查清除旧失败', async ({ page }, info) => {
  await page.setViewportSize({ width: 1440, height: 1024 })
  let revision = 1, checksStarted = 0, submitted = false
  let config: NormalImportConfig = { dataSourceId: normalImportSources[0]!.id, nodeId: normalImportNode.id, database: 'synthetic_database', filePath: 'E:\\synthetic-import', content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', objects: {}, parser: {}, execution: { memory: '512M' }, cleanup: 'NONE', mapping: { kind: 'POSITION' } }
  const fingerprint = () => (config.execution.memory === '512M' ? 'a' : 'b').repeat(64)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/v1/**', route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/normal-import-drafts/heap-draft') {
      if (request.method() === 'PATCH') { revision++; config = request.postDataJSON().config }
      return route.fulfill({ json: { id: 'heap-draft', revision, configVersion: 'normal-import-v1', config, configFingerprint: fingerprint(), metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1', executionEnabled: true } })
    }
    if (path.endsWith(':submit')) { submitted = true; return route.fulfill({ status: 409, json: {} }) }
    if (path.endsWith(':precheck')) { checksStarted++; return route.fulfill({ status: 202, json: { id: `heap-check-${checksStarted}` } }) }
    if (path.endsWith(':preview-command')) return route.fulfill({ json: { redactedCommand: 'obloader --no-sys -p ******', configFingerprint: fingerprint(), executionEnabled: true } })
    if (path.startsWith('/api/v1/normal-import-prechecks/')) {
      const lowHeap = config.execution.memory === '512M'
      return route.fulfill({ json: {
        id: path.split('/').at(-1), draftId: 'heap-draft', draftRevision: revision, configFingerprint: fingerprint(), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: lowHeap ? 'FAILED' : 'SUCCEEDED', integrityStatus: 'COMPLETE', executionEnabled: true, validUntil: new Date(Date.now() + 60000).toISOString(),
        results: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(check => ({ check, status: lowHeap && check === 'NODE_RESOURCE' ? 'FAILED' : 'PASSED', evidenceCode: lowHeap && check === 'NODE_RESOURCE' ? 'IMPORT_JVM_HEAP_TOO_SMALL' : 'SYNTHETIC_OK' })),
        inputManifest: { files: [{ path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 12, modified: '1790000000000000000', sha256: 'b'.repeat(64) }], fileCount: 1, totalBytes: 12, digest: 'c'.repeat(64) },
      } })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  await page.goto('/imports/normal/new?draft=heap-draft&step=5')
  const table = page.getByRole('region', { name: '普通导入任务预检查', exact: true })
  await expect(table.getByText('未通过', { exact: true })).toHaveCount(1)
  await expect(table).toContainText('JVM 堆至少 700 MiB')
  await expect(page.getByRole('button', { name: '提交并启动导入', exact: true })).toBeDisabled()
  await page.screenshot({ path: info.outputPath('normal-import-heap-blocked.png'), fullPage: false })
  await page.locator('.wizard-ant-step-rail .ant-steps-item').nth(3).click()
  await page.getByText('高级设置 · 资源、会话与数据行为', { exact: true }).click()
  await page.getByRole('textbox', { name: 'JVM 内存', exact: true }).fill('1G')
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  await expect(table.getByText('通过', { exact: true })).toHaveCount(6)
  await expect(table).not.toContainText('JVM 堆至少 700 MiB')
  await expect(page.getByRole('button', { name: '提交并启动导入', exact: true })).toBeEnabled()
  expect(checksStarted).toBe(2); expect(submitted).toBe(false); expect(errors).toEqual([])
})

test('普通导入逐项解释权限拒绝、定义未识别和元信息读取失败', async ({ page }, info) => {
  let revision = 1, started = 0
  let config: NormalImportConfig = { dataSourceId: normalImportSources[0]!.id, nodeId: normalImportNode.id, database: 'synthetic_database', filePath: 'E:\\synthetic-import', content: 'DDL_AND_DATA', format: 'CSV', scope: 'ALL', objects: {}, parser: {}, execution: {}, cleanup: 'NONE', mapping: { kind: 'POSITION' } }
  const cases = [['UNKNOWN', 'IMPORT_WRITE_DDL_UNSUPPORTED', 'IMPORT_INPUT_DEFINITION_UNSUPPORTED'], ['FAILED', 'IMPORT_WRITE_DROP_REQUIRED', 'IMPORT_COLUMN_COUNT_MISMATCH'], ['UNKNOWN', 'IMPORT_WRITE_GRANTS_UNAVAILABLE', 'IMPORT_TARGET_METADATA_UNAVAILABLE']] as const
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/v1/**', route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/normal-import-drafts/reason-draft') {
      if (request.method() === 'PATCH') { revision++; config = request.postDataJSON().config }
      return route.fulfill({ json: { id: 'reason-draft', revision, configVersion: 'normal-import-v1', config, configFingerprint: 'a'.repeat(64), metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1', executionEnabled: false } })
    }
    if (path.endsWith(':preview-command')) return route.fulfill({ json: { redactedCommand: 'obloader --no-sys --ddl --csv -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: false } })
    if (path.endsWith(':precheck')) { started++; return route.fulfill({ status: 202, json: { id: `reason-check-${started}` } }) }
    if (path.startsWith('/api/v1/normal-import-prechecks/')) {
      const sample = cases[started - 1]
      return route.fulfill({ json: { id: `reason-check-${started}`, draftId: 'reason-draft', draftRevision: revision, configFingerprint: 'a'.repeat(64), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: sample ? 'FAILED' : 'SUCCEEDED', integrityStatus: 'COMPLETE', validUntil: new Date(Date.now() + 60000).toISOString(), executionEnabled: false,
        results: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(check => ({ check, status: sample && ['WRITE_ACCESS', 'INPUT_MAPPING'].includes(check) ? sample[0] : 'PASSED', evidenceCode: sample && check === 'WRITE_ACCESS' ? sample[1] : sample && check === 'INPUT_MAPPING' ? sample[2] : 'SYNTHETIC_OK' })),
        inputManifest: { files: [{ path: 'synthetic-schema.sql', kind: 'DDL', object: 'synthetic', size: 20, modified: '1790000000000000000', sha256: 'b'.repeat(64) }, { path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 12, modified: '1790000000000000000', sha256: 'b'.repeat(64) }], fileCount: 2, totalBytes: 32, digest: 'c'.repeat(64) },
      } })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  await page.goto('/imports/normal/new?draft=reason-draft&step=5')
  const table = page.getByRole('region', { name: '普通导入任务预检查', exact: true })
  await expect(table.getByText('无法确认', { exact: true })).toHaveCount(2)
  await expect(table).toContainText('输入包含尚不能完整核算权限的 DDL'); await expect(table).toContainText('同名建表定义未被完整识别')
  await page.getByRole('button', { name: '重新预检查', exact: true }).click()
  await expect(table.getByText('未通过', { exact: true })).toHaveCount(2)
  await expect(table).toContainText('DROP 权限'); await expect(table).toContainText('首条数据记录的列数')
  await page.screenshot({ path: info.outputPath('normal-import-specific-reasons.png'), fullPage: true })
  await page.getByRole('button', { name: '重新预检查', exact: true }).click()
  await expect(table.getByText('无法确认', { exact: true })).toHaveCount(2)
  await expect(table).toContainText('读取当前账号的授权信息失败'); await expect(table).toContainText('读取目标表列元信息失败')
  await page.getByRole('button', { name: '重新预检查', exact: true }).click()
  await expect(table.getByText('通过', { exact: true })).toHaveCount(6)
  await expect(table.getByText('无法确认', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '提交并启动导入', exact: true })).toBeDisabled()
  expect(errors).toEqual([])
})
