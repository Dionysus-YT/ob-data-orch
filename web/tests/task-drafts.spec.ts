import { expect, test, type Page } from '@playwright/test'
import { mockFacts } from './fixtures/facts'
import { monitorNativeWindowErrors } from './fixtures/native-window-errors'

const verifyNative = new WeakMap<Page, () => Promise<void>>()
test.beforeEach(async ({ page }) => { verifyNative.set(page, await monitorNativeWindowErrors(page)) })
test.afterEach(async ({ page }) => { await verifyNative.get(page)?.() })

const time = '2026-10-14T00:00:00Z'
const draft = (id: string, availableActions = ['CONTINUE_EDIT', 'SUBMIT', 'DELETE']) => ({ id, type: 'OBDUMPER_EXPORT', displayName: `synthetic.${id}`, dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', revision: 2, createdAt: time, updatedAt: time, availableActions })

test('我的草稿保持关键字和游标分页，入口分别进入编辑及最终复验，删除经确认并携带写门禁', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  await mockFacts(page)
  const reads: URL[] = []; let deleted = false, writes = 0
  await page.route('**/api/v1/task-drafts?*', route => { const url = new URL(route.request().url()); reads.push(url); return route.fulfill({ json: { items: deleted ? [] : url.searchParams.has('cursor') ? [draft('second')] : [draft('first'), draft('leased', ['CONTINUE_EDIT', 'SUBMIT'])], totalPages: deleted ? 0 : 2, nextCursor: url.searchParams.has('cursor') || deleted ? null : 'draft-cursor' } }) })
  await page.route('**/api/v1/export-drafts/first', route => { if (route.request().method() !== 'DELETE') return route.fallback(); writes++; const headers = route.request().headers(); expect(headers['x-csrf-token']).toBe('local-mvp-csrf-v1'); expect(headers['if-match']).toBe('"rev-2"'); expect(headers['idempotency-key']).toBeTruthy(); deleted = true; return route.fulfill({ json: { deleted: true } }) })
  await page.goto('/tasks?tab=drafts')
  await expect(page.getByText('synthetic.first', { exact: true }).first()).toBeVisible()
  await page.getByRole('textbox', { name: '草稿名称 / ID 关键字' }).fill('orders'); await expect(page).toHaveURL(/draftKeyword=orders/); await expect.poll(() => reads.at(-1)?.searchParams.get('keyword')).toBe('orders')
  await page.getByRole('button', { name: '草稿下一页' }).click(); await expect(page.getByText('synthetic.second', { exact: true }).first()).toBeVisible(); expect(reads.at(-1)?.searchParams.get('cursor')).toBe('draft-cursor')
  await page.getByRole('button', { name: '草稿上一页' }).click()
  await page.getByRole('button', { name: 'synthetic.leased 草稿的更多操作' }).click(); await expect(page.getByRole('menuitem', { name: '提交执行', exact: true })).toBeVisible(); await expect(page.getByRole('menuitem', { name: '删除草稿', exact: true })).toHaveCount(0); await page.keyboard.press('Escape'); await expect(page.getByRole('menuitem', { name: '提交执行', exact: true })).toHaveCount(0)
  for (const [width, height] of [[1440, 1024], [1280, 720], [390, 844]]) { await page.setViewportSize({ width: width!, height: height! }); await page.screenshot({ path: info.outputPath(`draft-list-${width}.png`), fullPage: true }); await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), { message: String(width) }).toBe(true) }
  await page.setViewportSize({ width: 1440, height: 1024 })
  await page.getByRole('button', { name: 'synthetic.first 草稿的更多操作' }).click(); await page.getByRole('menuitem', { name: '删除草稿', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '删除草稿' }); expect(writes).toBe(0); await dialog.getByRole('button', { name: '取消', exact: true }).click(); expect(writes).toBe(0)
  await page.getByRole('button', { name: 'synthetic.first 草稿的更多操作' }).click(); await page.getByRole('menuitem', { name: '删除草稿', exact: true }).click(); await page.getByRole('dialog', { name: '删除草稿' }).getByRole('button', { name: '删除草稿', exact: true }).click()
  await expect(page.locator('.ant-message').getByText('草稿已删除。', { exact: true })).toBeVisible(); await expect(page.getByText('没有符合条件的草稿', { exact: true })).toBeVisible(); expect(writes).toBe(1)
  await page.screenshot({ path: info.outputPath('draft-filtered-empty.png'), fullPage: false })
  expect(errors).toEqual([])
})

test('草稿操作使用已有向导重新读取，不直接提交任务或启动工具', async ({ page }) => {
  await mockFacts(page)
  await page.route('**/api/v1/task-drafts?*', route => route.fulfill({ json: { items: [draft('editable')], nextCursor: null, totalPages: 1 } }))
  await page.goto('/tasks?tab=drafts'); await page.getByRole('button', { name: '继续编辑', exact: true }).click(); await expect(page).toHaveURL(/exports\/new\?draft=editable&step=1/)
  await page.goto('/tasks?tab=drafts'); await page.getByRole('button', { name: 'synthetic.editable 草稿的更多操作' }).click(); await page.getByRole('menuitem', { name: '提交执行', exact: true }).click(); await expect(page).toHaveURL(/exports\/new\?draft=editable&step=5/)
})

test('任务操作菜单方向键遍历三类入口，Home End 定位，Escape 关闭并归还焦点', async ({ page }) => {
  await mockFacts(page)
  await page.route('**/api/v1/tasks?*', route => route.fulfill({ json: { items: [], nextCursor: null, totalPages: 0 } }))
  await page.goto('/tasks')
  const trigger = page.getByRole('button', { name: '新建任务', exact: true }); await trigger.focus(); await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('menuitem', { name: '导出', exact: true })).toBeFocused()
  await page.keyboard.press('End'); await expect(page.getByRole('menuitem', { name: '旁路导入', exact: true })).toBeFocused()
  await page.keyboard.press('ArrowUp'); await expect(page.getByRole('menuitem', { name: '普通导入', exact: true })).toBeFocused()
  await page.keyboard.press('Home'); await expect(page.getByRole('menuitem', { name: '导出', exact: true })).toBeFocused()
  await page.keyboard.press('Escape'); await expect(page.getByRole('menuitem', { name: '导出', exact: true })).toHaveCount(0); await expect(trigger).toBeFocused()
})
