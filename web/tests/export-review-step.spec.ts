import { expect, test, type Page } from '@playwright/test'
import type { GeneralizedExportConfig, PrecheckResult } from '../src/api/browser'
import { precheckChecksForOutput } from '../src/workbench/export/exportPrecheckPresentation'
import { DATA_SOURCE_UI_FIXTURES } from '../src/workbench/sources/dataSourceUiFixture'
import { mockFacts } from './fixtures/facts'

// 全部请求由合成响应接管，隔离 Vite 无控制面代理，不解析真实凭据或连接数据库。
async function openReview(page: Page, storage = false) {
  await mockFacts(page)
  const source = DATA_SOURCE_UI_FIXTURES[0]!
  const state = { starts: 0, reads: 0, revision: 1, outcome: 'DIRECTORY_FAILED', pending: false, rejectNext: false, rejectReadAt: 0 }
  let config: GeneralizedExportConfig = {
    objectScope: { database: 'synthetic_database', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ objectType: 'TABLE', name: 'synthetic_table' }] },
    contentSelection: { contentKind: 'DATA_ONLY' },
    dataFormat: { formatKind: 'CSV', csvOptions: { columnSeparator: ',' } },
    outputConfig: { outputKind: storage ? 'OSS' : 'LOCAL', filePath: storage ? 'oss://synthetic-bucket/exports?endpoint=storage.synthetic.test' : '/E:/synthetic-output', ...(storage ? { tmpPath: '/E:/synthetic-tmp' } : {}) },
  }
  const fingerprint = () => `synthetic-review-fingerprint-${state.revision}`
  await page.route('**/api/v1/export-drafts/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith(':preview-command')) {
      await route.fulfill({ json: { command: 'obdumper --csv --table synthetic_table -p ****** --file-path /E:/synthetic-output --file-encoding UTF-8 --date-value-format yyyy-MM-dd --datetime-value-format "yyyy-MM-dd HH:mm:ss" --thread 8 --fetch-size 1000', configFingerprint: fingerprint() } })
    } else if (path.endsWith(':precheck')) {
      state.starts++
      if (state.rejectNext) {
        state.rejectNext = false
        await route.fulfill({ status: 503, json: { error: { code: 'SYNTHETIC_UNAVAILABLE', message: '合成请求失败' } } })
      } else await route.fulfill({ json: { id: `synthetic-review-check-${state.starts}` } })
    } else {
      if (route.request().method() === 'PATCH') {
        config = (route.request().postDataJSON() as { config: GeneralizedExportConfig }).config
        state.revision++
      }
      await route.fulfill({ json: { item: { id: 'synthetic-review-draft', dataSourceId: source.id, nodeId: 'synthetic-node', revision: state.revision, configVersion: 'v6', config: { config }, configFingerprint: fingerprint() } } })
    }
  })
  await page.route('**/api/v1/prechecks/**', async route => {
    state.reads++
    if (state.reads === state.rejectReadAt) {
      await route.fulfill({ status: 503, json: { error: { code: 'SYNTHETIC_UNAVAILABLE', message: '合成读取失败' } } })
      return
    }
    const results: PrecheckResult[] = precheckChecksForOutput(storage).map(check => {
      if (state.pending) return { check, status: 'UNKNOWN', evidenceCode: 'SYNTHETIC_OK' }
      if (state.outcome === 'RESOURCE_FAILED' && check === 'NODE_RESOURCE') return { check, status: 'FAILED', evidenceCode: 'NODE_RESOURCE_INSUFFICIENT' }
      if (state.outcome === 'SELECTION_FAILED' && check === 'OBJECT_ACCESS') return { check, status: 'FAILED', evidenceCode: 'OBJECT_SELECTION_REJECTED' }
      if (state.outcome === 'WRITE_UNKNOWN' && check === 'STORAGE_WRITE') return { check, status: 'UNKNOWN', evidenceCode: 'STORAGE_WRITE_UNAVAILABLE' }
      if (state.outcome === 'TOOL_FAILED') {
        if (check === 'TOOL_ENVIRONMENT') return { check, status: 'FAILED', evidenceCode: 'TOOL_RUNTIME_INVALID' }
        if (check === 'DATABASE_CONNECTIVITY' || check === 'OBJECT_ACCESS') return { check, status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_TOOL' }
      }
      if (state.outcome === 'DIRECTORY_FAILED' && !storage) {
        if (check === 'OUTPUT_PATH') return { check, status: 'FAILED', evidenceCode: 'OUTPUT_PATH_NOT_WRITABLE' }
        if (check === 'OUTPUT_EMPTY' || check === 'AVAILABLE_SPACE') return { check, status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_OUTPUT_PATH' }
      }
      if (state.outcome === 'STORAGE_FAILED') {
        if (check === 'STORAGE_CONNECTIVITY') return { check, status: 'FAILED', evidenceCode: 'STORAGE_ENDPOINT_UNREACHABLE' }
        if (check === 'STORAGE_AUTH') return { check, status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_STORAGE_CONNECTIVITY' }
        if (check === 'STORAGE_WRITE') return { check, status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_STORAGE_AUTH' }
      }
      return { check, status: 'PASSED', evidenceCode: 'SYNTHETIC_OK' }
    })
    await route.fulfill({ json: { item: {
      id: route.request().url().split('/').at(-1), draftId: 'synthetic-review-draft', draftRevision: state.revision,
      configFingerprint: fingerprint(), nodeId: 'synthetic-node', status: state.pending ? 'PENDING' : results.every(result => result.status === 'PASSED') ? 'SUCCEEDED' : 'FAILED',
      integrityStatus: 'COMPLETE', validUntil: '2099-01-01T00:00:00Z', results,
    } } })
  })
  return {
    state,
    savedConfig: () => config,
    async enter() { await page.goto('/exports/new?draft=synthetic-review-draft&step=5') },
  }
}

function captureErrors(page: Page) {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error' || message.type() === 'warning') errors.push(message.text()) })
  return errors
}

function reviewRows(page: Page, category: string) {
  return page.getByRole('table').getByRole('row').filter({ has: page.getByText(category, { exact: true }) })
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }, { width: 720, height: 900 }]) {
  test(`自动预检查明确独立结果和依赖阻断 ${viewport.width}`, async ({ page }, info) => {
    const errors = captureErrors(page)
    await page.setViewportSize(viewport)
    const fixture = await openReview(page)
    await fixture.enter()
    await expect(page).toHaveURL(/\/exports\/new\?.*step=5/)
    await expect(page).toHaveTitle(/OB Data Orch/)
    await expect(page.getByRole('heading', { name: '预检查与提交确认' })).toBeVisible()
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await expect(page.getByText('预检查未通过', { exact: true })).toHaveCount(1)
    await expect(page.locator('.precheck-list')).toContainText('1 项未通过，2 项因前置条件未执行')
    await expect(page.getByRole('region', { name: '预检查结果' }).getByRole('table')).toBeVisible()
    for (const name of ['预检查项', '状态', '检查结果', '操作']) await expect(page.getByRole('columnheader', { name, exact: true })).toBeVisible()
    await expect(reviewRows(page, '数据库与对象').getByText('通过', { exact: true })).toHaveCount(2)
    await expect(reviewRows(page, '输出与空间').getByText('前置阻断，未执行', { exact: true })).toHaveCount(2)
    await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeDisabled()
    await expect(page.getByText('当前步骤 · 57%', { exact: true })).toBeVisible()
    expect(fixture.state.starts).toBe(1)
    await expect(page.getByRole('button', { name: '任务配置摘要', exact: true })).toHaveCount(0)
    const summary = page.getByRole('button', { name: '查看任务摘要', exact: true })
    await expect(page.getByRole('dialog', { name: '任务摘要', exact: true })).toHaveCount(0)
    const command = page.getByRole('region', { name: '完整命令（敏感信息已脱敏）', exact: true })
    await expect(command).toBeVisible()
    await expect(page.getByRole('button', { name: '完整命令（敏感信息已脱敏）', exact: true })).toHaveCount(0)
    await expect(command.locator('pre')).toContainText('-p ******')
    await expect(command.locator('pre')).toContainText('--fetch-size 1000')
    await summary.focus()
    await page.keyboard.press('Enter')
    await expect(page.locator('.wizard-summary')).toContainText('/E:/synthetic-output')
    await expect(page.getByRole('dialog', { name: '任务摘要', exact: true })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(command.locator('pre')).toBeVisible()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await expect(summary).toBeFocused()
    await page.getByRole('heading', { name: '预检查与提交确认' }).click()
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.screenshot({ path: info.outputPath(`review-failed-${viewport.width}-viewport.png`), fullPage: false })
    await page.screenshot({ path: info.outputPath(`review-failed-${viewport.width}.png`), fullPage: true })
    await reviewRows(page, '数据库与对象').first().evaluate(element => element.scrollIntoView({ block: 'center' }))
    await page.screenshot({ path: info.outputPath(`review-results-${viewport.width}.png`), fullPage: false })
    await command.scrollIntoViewIfNeeded()
    await page.screenshot({ path: info.outputPath(`review-command-${viewport.width}.png`), fullPage: false })
    expect(errors).toEqual([])
  })
}

test('返回修改后保存自动检查新修订，成功结果可提交并保留确认', async ({ page }, info) => {
  const errors = captureErrors(page)
  const fixture = await openReview(page)
  await fixture.enter()
  await expect(page.getByText('预检查未通过', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '修改执行与输出', exact: true }).first().click()
  await expect(page).toHaveURL(/step=4/)
  await page.getByRole('textbox', { name: '导出路径', exact: true }).fill('/E:/synthetic-corrected-output')
  fixture.state.outcome = 'PASSED'
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  await expect(page.getByText('预检查通过，可以提交', { exact: true })).toBeVisible()
  expect(fixture.state.starts).toBe(2)
  expect(fixture.state.revision).toBe(2)
  await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
  await expect(page.locator('.wizard-summary')).toContainText('/E:/synthetic-corrected-output')
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '提交并启动导出', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '确认提交导出任务' })).toBeVisible()
  await expect(page.getByRole('button', { name: '返回检查', exact: true })).toBeFocused()
  await page.getByRole('button', { name: '返回检查', exact: true }).click()
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({ path: info.outputPath('review-passed.png'), fullPage: true })
  expect(errors).toEqual([])
})

test('输出字段错误只在字段下显示，定位焦点并保留修正后的预检查', async ({ page }, info) => {
  const api = await openReview(page)
  await api.enter()
  await page.getByRole('button', { name: '修改执行与输出', exact: true }).first().click()
  const path = page.getByRole('textbox', { name: '导出路径', exact: true })
  await path.fill('synthetic-relative-output')
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  const message = '导出路径必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。'
  await expect(page.getByText(message, { exact: true })).toHaveCount(1)
  await expect(page.locator('.ant-form-item-explain-error')).toHaveText(message)
  await expect(path).toBeFocused()
  await page.screenshot({ path: info.outputPath('export-path-field-error.png'), fullPage: true })
  api.state.outcome = 'PASSED'
  await path.fill('/E:/synthetic-corrected-output')
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeEnabled()
  expect(api.savedConfig().outputConfig.filePath).toBe('/E:/synthetic-corrected-output')
})

test('工具环境失败时数据库组显示未执行且定位到节点步骤', async ({ page }) => {
  const fixture = await openReview(page)
  fixture.state.outcome = 'TOOL_FAILED'
  await fixture.enter()
  await expect(page.getByText('预检查未通过', { exact: true })).toBeVisible()
  const database = reviewRows(page, '数据库与对象')
  await expect(database.getByText('前置阻断，未执行', { exact: true })).toHaveCount(2)
  await expect(database.getByText('工具环境未通过，未执行本项', { exact: false })).toHaveCount(2)
  await database.getByRole('button', { name: '检查执行节点', exact: true }).first().click()
  await expect(page).toHaveURL(/step=2/)
  expect(fixture.state.starts).toBe(1)
})

test('等待节点时占位结果保持待检查，返回第五步不重复发起', async ({ page }) => {
  const fixture = await openReview(page)
  fixture.state.pending = true
  await fixture.enter()
  await expect(page.getByText('等待执行节点领取预检查', { exact: true })).toBeVisible()
  await expect(page.getByText('等待检查结果', { exact: true })).toHaveCount(7)
  await expect(page.getByRole('button', { name: '预检查进行中…', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: '上一步', exact: true }).click()
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  fixture.state.pending = false
  fixture.state.outcome = 'PASSED'
  await expect(page.getByText('预检查通过，可以提交', { exact: true })).toBeVisible()
  expect(fixture.state.starts).toBe(1)
})

test('自动发起失败后等待手动重试，不循环发起', async ({ page }) => {
  const fixture = await openReview(page)
  fixture.state.rejectNext = true
  fixture.state.outcome = 'PASSED'
  await fixture.enter()
  await expect(page.getByText('预检查请求或结果读取失败', { exact: true })).toBeVisible()
  expect(fixture.state.starts).toBe(1)
  await page.getByRole('button', { name: '开始预检查', exact: true }).click()
  await expect(page.getByText('预检查通过，可以提交', { exact: true })).toBeVisible()
  expect(fixture.state.starts).toBe(2)
})

test('等待节点时读取失败可重新读取同一检查', async ({ page }) => {
  const fixture = await openReview(page)
  fixture.state.pending = true
  fixture.state.rejectReadAt = 2
  await fixture.enter()
  await expect(page.getByText('预检查请求或结果读取失败', { exact: true })).toBeVisible()
  await expect(page.getByText('等待检查结果', { exact: true })).toHaveCount(7)
  fixture.state.pending = false
  fixture.state.outcome = 'PASSED'
  await page.getByRole('button', { name: '重新读取预检查', exact: true }).click()
  await expect(page.getByText('预检查通过，可以提交', { exact: true })).toBeVisible()
  expect(fixture.state.starts).toBe(1)
})

test('对象存储使用正确清单并说明认证的端点依赖', async ({ page }) => {
  const errors = captureErrors(page)
  const fixture = await openReview(page, true)
  fixture.state.outcome = 'STORAGE_FAILED'
  await fixture.enter()
  await expect(page.getByText('预检查未通过', { exact: true })).toBeVisible()
  await expect(reviewRows(page, '对象存储').getByText('存储端点连通性未通过，未执行凭据有效性检查', { exact: false })).toHaveCount(1)
  await expect(page.getByText('输出目录空性', { exact: true })).toHaveCount(0)
  await expect(page.getByText('本地辅助目录', { exact: true })).toBeVisible()
  await expect(reviewRows(page, '对象存储').getByText('存储凭据有效性未通过，未执行写权限检查', { exact: false })).toHaveCount(1)
  await expect(reviewRows(page, '数据库与对象').getByText('通过', { exact: true })).toHaveCount(2)
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeDisabled()
  expect(errors).toEqual([])
})

test('资源额度不足仍保留数据库通过，列分区错误定位筛选步骤', async ({ page }) => {
  const fixture = await openReview(page)
  fixture.state.outcome = 'RESOURCE_FAILED'
  await fixture.enter()
  await expect(reviewRows(page, '数据库与对象').getByText('通过', { exact: true })).toHaveCount(2)
  await expect(reviewRows(page, '执行环境').filter({ has: page.getByText('节点静态资源额度', { exact: true }) })).toContainText('超过节点总执行额度')
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeDisabled()
  fixture.state.outcome = 'SELECTION_FAILED'
  await page.getByRole('button', { name: '重新检查', exact: true }).click()
  await expect(reviewRows(page, '数据库与对象').filter({ has: page.getByText('对象与参数访问', { exact: true }) })).toContainText('子分区')
  await page.getByRole('button', { name: '修改列与分区筛选', exact: true }).click()
  await expect(page).toHaveURL(/step=2/)
})

test('存储认证通过仍需要独立写权限证据', async ({ page }) => {
  const fixture = await openReview(page, true)
  fixture.state.outcome = 'WRITE_UNKNOWN'
  await fixture.enter()
  await expect(reviewRows(page, '对象存储').getByText('通过', { exact: true })).toHaveCount(2)
  await expect(reviewRows(page, '对象存储').filter({ has: page.getByText('存储写权限', { exact: true }) })).toContainText('不能用端点连通或认证通过代替')
  await expect(page.getByRole('button', { name: '提交并启动导出', exact: true })).toBeDisabled()
})

test('存储临时分块目录在常用区，日志与临时目录保存进统一摘要', async ({ page }) => {
  const fixture = await openReview(page, true)
  fixture.state.outcome = 'WRITE_UNKNOWN'
  await fixture.enter()
  await page.getByRole('button', { name: '修改执行与输出', exact: true }).first().click()
  await expect(page).toHaveURL(/step=4/)
  const temporary = page.getByRole('textbox', { name: '本地临时分块目录', exact: true })
  await expect(temporary).toBeVisible()
  await expect(temporary).toHaveAttribute('aria-required', 'true')
  await temporary.fill('/E:/tmp/synthetic-upload')
  await page.getByRole('textbox', { name: '日志路径', exact: true }).fill('/E:/tmp/synthetic-logs')
  fixture.state.outcome = 'PASSED'
  await page.getByRole('button', { name: '查看预检查与命令', exact: true }).click()
  await expect(page.getByText('预检查通过，可以提交', { exact: true })).toBeVisible()
  expect(fixture.savedConfig().outputConfig).toMatchObject({ tmpPath: '/E:/tmp/synthetic-upload', logPath: '/E:/tmp/synthetic-logs' })
  await page.getByRole('button', { name: '查看任务摘要', exact: true }).click()
  await expect(page.locator('.wizard-summary')).toContainText('/E:/tmp/synthetic-upload')
  await expect(page.locator('.wizard-summary')).toContainText('/E:/tmp/synthetic-logs')
})
