import { expect, test } from '@playwright/test'
import { mockFacts } from './fixtures/facts'


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
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await expect(page.getByRole('heading', { name: '导出内容与对象', exact: true })).toBeVisible()
  const progress = await page.locator('.export-ant-step-rail').boundingBox()
  const workspace = await page.locator('.orch-task-workspace').boundingBox()
  expect(progress && workspace && progress.y + progress.height <= workspace.y).toBeTruthy()
  await page.screenshot({ path: info.outputPath('export-content.png'), fullPage: true })
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await expect(page.getByRole('heading', { name: '导出对象', exact: true })).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: '已选 0 / 100 项' })).toBeVisible()
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_table\nsynthetic_archive')
  await page.getByRole('button', { name: '添加并选中' }).click()
  await expect(page.getByRole('status').filter({ hasText: '已选 2 / 100 项' })).toBeVisible()
  await page.getByRole('textbox', { name: '搜索候选对象' }).fill('archive')
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_archive' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_table' })).toHaveCount(0)
  await candidates.getByRole('checkbox', { name: 'synthetic_archive' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_archive' })).toHaveCount(0)
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
  await expect(candidates.getByRole('checkbox', { name: 'synthetic_table' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '下一步：选择数据格式' })).toBeDisabled()
  await expect(page.getByText('请至少选择一个表。')).toBeVisible()
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_table')
  await page.getByRole('button', { name: '添加并选中' }).click()
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
  await page.getByRole('button', { name: /选择数据源 已访问/ }).click()
  await expect(page.getByRole('heading', { name: '选择已有数据源' })).toBeVisible()
  await expect(page.getByRole('radio', { name: /Production finance reporting/ })).toBeChecked()
})

test('部分导出从节点元数据加载对象并清除跨库旧选择', async ({ page }) => {
  await mockFacts(page)
  const observed: Array<{ database: string; nodeId: string; objectType: string }> = []
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    const input = route.request().postDataJSON() as { database: string; nodeId: string; objectType: string; keyword: string }
    observed.push({ database: input.database, nodeId: input.nodeId, objectType: input.objectType })
    await route.fulfill({ json: { item: {
      id: `catalog-${observed.length}`, status: 'SUCCEEDED', dataSourceId: 'ui-fixture-production-finance-reporting',
      nodeId: input.nodeId, database: input.database, objectType: input.objectType, keyword: input.keyword,
      objects: input.objectType === 'DATABASE' ? ['finance_reporting', 'other_db'] : input.database === 'finance_reporting' ? ['orders', 'users'] : ['fresh_table'],
      truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('checkbox', { name: 'orders' })).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'orders' }).check()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await page.getByRole('button', { name: '清空', exact: true }).click()
  await expect(candidates.getByRole('checkbox', { name: 'orders' })).toBeVisible()
  await candidates.getByRole('checkbox', { name: 'orders' }).check()
  await page.locator('.export-field-body .ant-select').nth(1).click()
  await page.getByText('other_db', { exact: true }).last().click()
  await expect(candidates.getByRole('checkbox', { name: 'fresh_table' })).toBeVisible()
  await expect(candidates.getByRole('checkbox', { name: 'orders' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
  expect(observed.filter(({ objectType }) => objectType === 'TABLE').map(({ database }) => database)).toEqual(['finance_reporting', 'other_db'])
  await candidates.getByRole('checkbox', { name: 'fresh_table' }).check()
  await page.getByRole('button', { name: '下一步：选择数据格式' }).click()
  await page.getByRole('button', { name: '下一步：执行与输出' }).click()
  await expect(page.getByRole('button', { name: '返回内容与对象修改节点' })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '执行节点', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '返回内容与对象修改节点' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
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
      objects: ['orders'], truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await expect(page.getByText('目录查询条件与当前控制面版本不兼容。请更新控制面与 Agent 后重新加载。')).toBeVisible()
  await page.locator('.export-field-body .ant-select').nth(1).click()
  await expect(page.getByText('手动输入其他数据库 / Schema…')).toBeVisible()
})

test('导出对象按分类切换并确认清除旧类型选择', async ({ page }) => {
  await mockFacts(page)
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.getByText('仅导出结构', { exact: true }).click()
  const candidates = page.getByRole('region', { name: '选择导出对象' })
  await expect(candidates.getByRole('button', { name: '展开视图分类' })).toBeEnabled()
  await expect(candidates.getByText('函数（未开放）')).toBeVisible()
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_table')
  await page.getByRole('button', { name: '添加并选中' }).click()
  await candidates.getByRole('button', { name: '展开视图分类' }).click()
  await expect(page.getByRole('dialog').getByText('切换到视图？')).toBeVisible()
  await page.getByRole('button', { name: '切换并清空' }).click()
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
  await expect(candidates.getByRole('button', { name: '收起视图分类' })).toBeVisible()
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_view')
  await page.getByRole('button', { name: '添加并选中' }).click()
  await expect(page.getByRole('region', { name: '已选导出对象' }).getByText('视图（1）')).toBeVisible()
  await page.getByRole('button', { name: '收起已选视图' }).click()
  await expect(page.getByRole('button', { name: '移除已选对象 synthetic_view' })).toHaveCount(0)
  await page.getByRole('button', { name: '展开已选视图' }).click()
  await page.getByRole('button', { name: '移除已选对象 synthetic_view' }).click()
  await expect(page.getByRole('heading', { name: '已选 0 项' })).toBeVisible()
})

test('对象目录接口返回 404 时提示版本或授权并允许手动选择', async ({ page }) => {
  await mockFacts(page)
  await page.route('**/api/v1/data-sources/*:search-export-objects', async (route) => {
    await route.fulfill({ status: 404, json: { error: { code: 'NOT_FOUND', message: '数据源不存在或当前身份无权访问。' } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await expect(page.getByRole('region', { name: '选择导出对象' }).getByRole('alert').filter({ hasText: '对象目录接口不可用' })).toBeVisible()
  await page.locator('.export-field-body .ant-select').nth(1).click()
  await page.getByText('手动输入其他数据库 / Schema…').click()
  await page.getByRole('textbox', { name: '手动输入数据库 / Schema' }).fill('manual_schema')
  await page.getByRole('button', { name: '使用此名称' }).click()
  await expect(page.locator('.export-field-body .ant-select').nth(1)).toContainText('manual_schema')
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('manual_table')
  await page.getByRole('button', { name: '添加并选中' }).click()
  await expect(page.getByRole('heading', { name: '已选 1 项' })).toBeVisible()
  await page.getByRole('button', { name: '移除已选对象 manual_table' }).click()
  await expect(page.getByRole('region', { name: '选择导出对象' }).getByRole('checkbox', { name: 'manual_table' })).toHaveCount(0)
})

test('对象查询未被节点领取后停止轮询并保留手动入口', async ({ page }) => {
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
      nodeId: 'synthetic-node', database: 'finance_reporting', objectType: 'TABLE', keyword: '',
      objects: [], truncated: false, validUntil: '2026-09-30T12:00:00Z',
    } } })
  })
  await page.goto('/exports/new')
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: '下一步：导出内容与对象' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 · WINDOWS_AMD64').last().click()
  await expect(page.getByRole('alert').filter({ hasText: '执行节点未及时领取或完成对象查询' })).toBeVisible()
  await expect(page.getByText('手动添加候选对象', { exact: true })).toBeVisible()
  expect(requests).toBe(1)
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

test('导出普通入口限制格式，执行页仅展开一个高级设置', async ({ page }, info) => {
  await mockFacts(page)
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.goto('/exports/new?step=3')
  const formats = page.getByRole('radiogroup', { name: '数据格式' })
  await expect(formats.getByRole('radio')).toHaveCount(3)
  await expect(formats.getByRole('radio', { name: 'CSV' })).toBeChecked()
  await expect(page.getByRole('heading', { name: '数据文件设置' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'CSV 设置' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: '包含列头' })).toBeChecked()
  await page.getByRole('checkbox', { name: '包含列头' }).uncheck()
  await expect(page.getByRole('checkbox', { name: '包含列头' })).not.toBeChecked()
  await expect(page.getByRole('checkbox', { name: '一致性快照' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '文件拆分' })).toBeVisible()
  await formats.getByRole('radio', { name: 'CUT' }).check()
  await expect(page.getByRole('heading', { name: 'CUT 设置' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: '包含列头' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /高级设置 · 序列化、日期时间与压缩/ })).toBeVisible()
  await page.screenshot({ path: info.outputPath('export-format.png'), fullPage: true })
  await page.goto('/exports/new?step=4')
  await expect(page.getByRole('button', { name: /高级设置 · 文件布局、性能与对象存储/ })).toHaveCount(1)
  await expect(page.getByRole('textbox', { name: '导出路径' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '日志路径' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('export-output.png'), fullPage: true })
  await page.getByRole('button', { name: '创建草稿并进入预检查' }).click()
  await expect(page.getByText('请先选择一个完成基础连接测试的数据源。').first()).toBeVisible()
})

test('导出高级设置使用 Ant 展开反馈并按格式和输出类型显隐', async ({ page }) => {
  await mockFacts(page)
  await page.goto('/exports/new?step=2')
  await page.getByRole('radiogroup', { name: '导出内容' }).getByText('仅导出结构', { exact: true }).click()
  await page.getByRole('button', { name: /高级设置 · DDL、对象与数据筛选/ }).click()
  await page.getByRole('checkbox', { name: /前置 DROP/ }).check()
  await expect(page.getByText(/可能造成数据丢失/)).toBeVisible()

  await page.goto('/exports/new?step=3')
  await page.getByRole('button', { name: /高级设置 · 序列化、日期时间与压缩/ }).click()
  const compressionAlgo = page.getByRole('combobox', { name: '压缩算法' })
  await expect(compressionAlgo).toBeDisabled()
  await page.getByRole('checkbox', { name: /启用压缩/ }).check()
  await expect(compressionAlgo).toBeEnabled()
  await page.getByRole('radiogroup', { name: '数据格式' }).getByRole('radio', { name: 'CUT' }).check()
  await expect(page.getByRole('textbox', { name: '列分隔字符串' })).toBeVisible()
  await page.getByRole('radiogroup', { name: '数据格式' }).getByRole('radio', { name: 'Insert SQL' }).check()
  await expect(page.getByRole('textbox', { name: '列分隔字符串' })).toHaveCount(0)

  await page.goto('/exports/new?step=4')
  await page.getByRole('button', { name: /高级设置 · 文件布局、性能与对象存储/ }).click()
  await page.getByRole('radiogroup', { name: '输出类型' }).getByRole('radio', { name: 'S3' }).check()
  await expect(page.getByRole('textbox', { name: 'Bucket' })).toBeVisible()
  await expect(page.getByText(/未取得通过证据时不能提交/)).toBeVisible()
})

test('导出格式在 200% 缩放下仍可填写且无整页横滚', async ({ page }, info) => {
  await mockFacts(page)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/exports/new?step=3')
  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  await expect(page.getByRole('textbox', { name: '列分隔符' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '行分隔符' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '文件编码' })).toBeVisible()
  await expect(page.getByRole('button', { name: '下一步：执行与输出' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('export-format-zoom-200.png'), fullPage: true })
})

test('导出对象双栏在 200% 缩放下按顺序堆叠', async ({ page }, info) => {
  await mockFacts(page)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/exports/new?step=2')
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
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_table')
  await page.getByRole('button', { name: '添加并选中' }).click()
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
  await page.getByRole('button', { name: '保存并进入预检查' }).click()
  await expect(page.getByRole('heading', { name: '参数预检查与完整命令' })).toBeVisible()
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
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_table')
  await page.getByRole('button', { name: '添加并选中' }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('textbox', { name: '导出路径' }).fill('/E:/exports/synthetic')
  await page.getByRole('button', { name: '保存草稿' }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '返回内容与对象修改节点' }).click()
  await page.locator('.export-field-body .ant-select').first().click()
  await page.getByText('合成执行节点 2 · WINDOWS_AMD64').last().click()
  await expect(page.getByText('草稿尚未创建')).toBeVisible()
  await page.getByText('手动添加候选对象', { exact: true }).click()
  await page.getByRole('textbox', { name: '添加候选对象' }).fill('synthetic_table')
  await page.getByRole('button', { name: '添加并选中' }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: /^下一步：/ }).click()
  await page.getByRole('button', { name: '保存草稿' }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
  expect(created).toBe(2)
  expect(updated).toBe(0)
})
