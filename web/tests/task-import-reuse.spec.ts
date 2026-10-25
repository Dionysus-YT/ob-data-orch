import { expect, test, type Page } from '@playwright/test'
import { normalImportNode, normalImportSources } from './fixtures/normal-import'
import type { NormalImportConfig, NormalImportDraft } from '../src/api/normalImport'

test.use({ actionTimeout: 15000 })
const time = '2026-10-19T00:00:00Z'
const taskType = 'OBLOADER_NORMAL_IMPORT'
const config: NormalImportConfig = { dataSourceId: normalImportSources[0]!.id, nodeId: normalImportNode.id, database: 'synthetic_database', filePath: 'E:\\synthetic-import', content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', objects: {}, parser: {}, execution: {}, cleanup: 'NONE', mapping: { kind: 'POSITION' } }

async function fixture(page: Page, state = 'SUCCEEDED') {
  const drafts = new Map<string, NormalImportDraft>()
  const writes: { path: string; method: string }[] = []
  let savedTemplate = false, checks = 0
  function create(id: string, derivation?: 'REBUILD_FROM_CONFIG' | 'RERUN_FROM_SCRATCH') {
    const item: NormalImportDraft = { id, revision: 1, configVersion: 'normal-import-v1', config: structuredClone(config), configFingerprint: 'a'.repeat(64), executionEnabled: false, ...(derivation ? { sourceTaskId: 'normal-task', sourceDerivation: derivation } : {}) }
    drafts.set(id, item); return item
  }
  const presentation = { id: 'normal-task', type: taskType, displayName: '合成普通导入任务', dataSourceId: config.dataSourceId, nodeId: config.nodeId, state, dataSourceDeleted: false, nodeDeleted: false, ownedByCurrentUser: true, reconciliationRequired: false, cancelRequested: false, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', submittedAt: time, updatedAt: time, availableActions: ['COPY_COMMAND', 'VIEW_LOGS', 'REBUILD_FROM_CONFIG', ...(state === 'SUCCEEDED' ? ['SAVE_TEMPLATE'] : ['RERUN_FROM_SCRATCH'])] }
  await page.route('**/api/v1/**', route => {
    const request = route.request(), path = new URL(request.url()).pathname, method = request.method()
    if (method !== 'GET') { expect(request.headers()['x-csrf-token']).toBeTruthy(); writes.push({ path, method }) }
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/tasks/filter-options') return route.fulfill({ json: { dataSources: [], nodes: [], creators: [], environments: [], creatorFilterAvailable: false } })
    if (path === '/api/v1/tasks') return route.fulfill({ json: { items: [presentation], totalPages: 1 } })
    if (path === '/api/v1/tasks/normal-task') return route.fulfill({ json: { item: { id: 'normal-task', type: taskType, dataSourceId: config.dataSourceId, nodeId: config.nodeId, precheckId: 'old-check', submittedAt: time, presentation } } })
    if (path.endsWith('/execution')) return route.fulfill({ json: { item: { state, reconciliationRequired: false, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', updatedAt: time } } })
    if (path.endsWith('/snapshot')) return route.fulfill({ json: { item: { type: taskType, snapshotVersion: 'normal-import-v1', dataSourceId: config.dataSourceId, nodeId: config.nodeId, precheckId: 'old-check', format: 'CSV', configFingerprint: 'a'.repeat(64), toolVersion: '4.3.5-RELEASE', metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1' } } })
    if (path.endsWith('/command-evidence')) return route.fulfill({ json: { item: { kind: 'PLANNED', command: 'obloader --csv -p ******', redaction: 'PASSWORD_ONLY' } } })
    if (path.endsWith('/logs') || path.endsWith('/operations')) return route.fulfill({ json: { items: [], integrity: 'COMPLETE' } })
    if (path.endsWith(':save-template')) { savedTemplate = true; return route.fulfill({ status: 201, json: { id: 'normal-template', sourceTaskId: 'normal-task' } }) }
    if (path === '/api/v1/config-templates') return route.fulfill({ json: { items: savedTemplate ? [{ id: 'normal-template', taskType, displayName: '合成普通导入模板', revision: 1, capabilityVersion: 'import-odp-local-v1', configFingerprint: 'a'.repeat(64), sourceTaskId: 'normal-task', createdAt: time, updatedAt: time }] : [] } })
    if (path === '/api/v1/config-templates/normal-template:create-draft') {
      expect(request.postDataJSON()).toEqual({ dataSourceId: config.dataSourceId, nodeId: config.nodeId })
      create('template-normal'); return route.fulfill({ status: 201, json: { draftId: 'template-normal', templateId: 'normal-template', taskType } })
    }
    if (path.endsWith(':rebuild-draft')) {
      const derivation = request.postDataJSON().derivation
      create('derived-normal', derivation)
      return route.fulfill({ status: 201, json: { draftId: 'derived-normal', sourceTaskId: 'normal-task', derivation, taskType } })
    }
    if (path === '/api/v1/task-drafts') return route.fulfill({ json: { items: [...drafts.values()].map(item => ({ id: item.id, type: taskType, displayName: config.database, dataSourceId: config.dataSourceId, nodeId: config.nodeId, revision: item.revision, sourceTaskId: item.sourceTaskId, sourceDerivation: item.sourceDerivation, createdAt: time, updatedAt: time, availableActions: ['CONTINUE_EDIT', 'DELETE'] })), totalPages: drafts.size ? 1 : 0, nextCursor: '' } })
    if (path === '/api/v1/normal-import-drafts' && method === 'POST') {
      const item = create('saved-normal'); item.config = request.postDataJSON().config
      return route.fulfill({ status: 201, json: { ...item, metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1' } })
    }
    if (path.startsWith('/api/v1/normal-import-drafts/') && !path.includes(':')) {
      const id = path.split('/').at(-1)!, item = drafts.get(id)
      if (!item) return route.fulfill({ status: 404, json: {} })
      if (method === 'DELETE') { expect(request.headers()['if-match']).toBe(`"rev-${item.revision}"`); drafts.delete(id); return route.fulfill({ json: { deleted: true } }) }
      if (method === 'PATCH') { item.revision++; item.config = request.postDataJSON().config }
      return route.fulfill({ json: { ...item, metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1' } })
    }
    if (path.endsWith(':preview-command')) return route.fulfill({ json: { redactedCommand: 'obloader --csv -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: false } })
    if (path.endsWith(':precheck')) { checks++; return route.fulfill({ status: 202, json: { id: 'new-check' } }) }
    if (path === '/api/v1/normal-import-prechecks/new-check') return route.fulfill({ json: { id: 'new-check', draftId: 'derived-normal', draftRevision: drafts.get('derived-normal')!.revision, configFingerprint: 'a'.repeat(64), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'FAILED', integrityStatus: 'COMPLETE', executionEnabled: false, validUntil: new Date(Date.now() + 60000).toISOString(), results: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(check => ({ check, status: 'UNKNOWN', evidenceCode: check === 'TOOL_ENVIRONMENT' ? 'TOOL_RUNTIME_UNAVAILABLE' : check === 'NODE_RESOURCE' ? 'NODE_RESOURCE_UNAVAILABLE' : check === 'DATABASE_CONNECTIVITY' ? 'DATABASE_CONNECTION_UNAVAILABLE' : 'IMPORT_UNAVAILABLE' })) } })
    return route.fulfill({ status: 404, json: {} })
  })
  return { drafts, writes, checks: () => checks, create }
}

test('普通导入保存离开后从统一草稿恢复并按正确版本删除', async ({ page }, info) => {
  const f = await fixture(page)
  await page.goto('/imports/normal/new')
  await page.getByRole('radio', { name: /合成 MySQL 导入目标/ }).check()
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  await expect(page).toHaveURL(/draft=saved-normal/)
  await page.getByRole('link', { name: '任务中心', exact: true }).click()
  await page.getByRole('tab', { name: '我的草稿', exact: true }).click()
  await expect(page.getByRole('region', { name: '我的草稿', exact: true })).toContainText('普通导入')
  await page.getByRole('button', { name: '继续编辑', exact: true }).click()
  await expect(page).toHaveURL(/\/imports\/normal\/new\?draft=saved-normal&step=1/)
  await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
  await page.getByRole('link', { name: '任务中心', exact: true }).click()
  await page.getByRole('tab', { name: '我的草稿', exact: true }).click()
  await page.screenshot({ path: info.outputPath('import-unified-drafts.png'), fullPage: true })
  await page.getByRole('button', { name: /草稿的更多操作/ }).click()
  await page.getByRole('menuitem', { name: '删除草稿', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '删除草稿', exact: true }).click()
  await expect(page.getByText('尚无未提交草稿', { exact: true })).toBeVisible()
  expect(f.drafts.size).toBe(0)
})

test('普通导入成功任务保存模板，首次正常绑定后进入普通导入向导', async ({ page }, info) => {
  const f = await fixture(page)
  await page.goto('/tasks/normal-task')
  await page.getByRole('button', { name: '保存为模板', exact: true }).click()
  await page.getByRole('textbox', { name: '模板名称', exact: true }).fill('合成普通导入模板')
  await page.getByRole('dialog').getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.getByRole('link', { name: '模板中心', exact: true }).click()
  await page.getByRole('button', { name: '用模板新建草稿', exact: true }).click()
  await expect(page.getByRole('button', { name: '创建草稿', exact: true })).toBeDisabled()
  const source = page.getByRole('combobox', { name: /^数据源/ })
  await expect(source).toBeEnabled(); await source.focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText(normalImportSources[0]!.displayName, { exact: true }).click()
  const node = page.getByRole('combobox', { name: /^执行节点/ })
  await node.focus(); await page.keyboard.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible').getByText('合成导入节点 · Windows · x86_64', { exact: true }).click()
  await expect(page.getByText('请选择数据源与执行节点。', { exact: true })).toHaveCount(0)
  await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '创建草稿', exact: true })).toBeEnabled()
  await page.screenshot({ path: info.outputPath('import-template-bound.png'), fullPage: true })
  await page.getByRole('button', { name: '创建草稿', exact: true }).click()
  await expect(page).toHaveURL(/\/imports\/normal\/new\?draft=template-normal&step=1/)
  await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
  expect(f.writes.some(item => item.path.startsWith('/api/v1/export-drafts'))).toBe(false)
})

for (const derivation of ['REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH'] as const) {
  test(`普通导入 ${derivation} 保持类型并重新取得预检查`, async ({ page }, info) => {
    const f = await fixture(page, 'FAILED')
    await page.goto('/tasks/normal-task')
    const label = derivation === 'RERUN_FROM_SCRATCH' ? '从头重新执行' : '基于原配置新建'
    if (derivation === 'REBUILD_FROM_CONFIG') await page.getByRole('button', { name: label, exact: true }).click()
    else { await page.getByRole('button', { name: '更多操作', exact: true }).click(); await page.getByRole('menuitem', { name: label, exact: true }).click() }
    await page.getByRole('dialog').getByRole('button', { name: label, exact: true }).click()
    await expect(page).toHaveURL(new RegExp(`/imports/normal/new\\?draft=derived-normal&step=${derivation === 'REBUILD_FROM_CONFIG' ? 1 : 5}`))
    if (derivation === 'RERUN_FROM_SCRATCH') {
      await expect(page.getByText('从头重新执行：保留原配置并重新预检查。', { exact: true })).toBeVisible()
      await expect.poll(f.checks).toBe(1)
      await expect(page.getByRole('button', { name: '上一步', exact: true })).toBeDisabled()
      await expect(page.getByRole('button', { name: '保存草稿', exact: true })).toBeDisabled()
      await expect(page.locator('.wizard-ant-step-rail .ant-steps-item').first()).toHaveClass(/ant-steps-item-disabled/)
      await expect(page.getByRole('button', { name: /修改执行参数|修改输入文件|修改对象与映射|检查执行节点|检查数据源/ })).toHaveCount(0)
      await expect(page.getByRole('region', { name: '普通导入任务预检查', exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '提交并启动导入', exact: true })).toBeDisabled()
      await page.screenshot({ path: info.outputPath('import-rerun-new-precheck.png'), fullPage: true })
    } else await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
    expect(f.writes.some(item => item.path.endsWith(':submit') || item.path.endsWith(':resume-checkpoint'))).toBe(false)
  })
}

test('节点目录配置说明导入读取与导出写入，保持原允许目录约束', async ({ page }, info) => {
  const f = await fixture(page)
  await page.goto('/nodes/new')
  const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
  await expect(drawer.getByLabel('任务数据目录白名单', { exact: true })).toBeVisible()
  await expect(drawer).toContainText('导出输出与普通导入输入均须位于允许目录内')
  await expect(drawer).not.toContainText('仅允许导出写入')
  await drawer.getByLabel('任务数据目录白名单', { exact: true }).scrollIntoViewIfNeeded()
  await page.screenshot({ path: info.outputPath('node-import-export-root-help.png'), fullPage: true })
  expect(f.writes).toEqual([])
})
