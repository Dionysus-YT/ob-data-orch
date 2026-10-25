import { chromium, expect, test, type Page } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { permissionFixture } from './fixtures/access-control'

const drawer = (page: Page) => page.locator('.ant-drawer-content').filter({ has: page.getByRole('heading', { name: '配置权限', exact: true }) })
async function openEditor(page: Page) { await page.getByRole('button', { name: '配置 合成业务用户 的权限', exact: true }).click(); await expect(drawer(page)).toBeVisible(); await expect(drawer(page).getByRole('checkbox', { name: '普通用户', exact: true })).toBeChecked() }
async function prepareSave(page: Page) { await drawer(page).getByRole('checkbox', { name: '系统管理员', exact: true }).check(); await drawer(page).getByRole('textbox', { name: '变更原因', exact: true }).fill('调整合成测试用户职责'); await drawer(page).getByRole('button', { name: '比较并保存' }).click(); await expect(page.getByRole('dialog').filter({ has: page.getByText('确认权限变更', { exact: true }) })).toBeVisible() }

test('权限旧回执只提示原提交，重新读取当前版本且不恢复旧角色', async ({ page }) => {
  await permissionFixture(page, 'replay'); await page.goto('/settings/access-control'); await openEditor(page); await prepareSave(page); await page.getByRole('button', { name: '确认保存', exact: true }).click(); await expect(page.getByText('已确认原请求的保存回执，当前权限以重新读取结果为准。')).toBeVisible(); const row = page.getByRole('row').filter({ has: page.getByText('合成业务用户', { exact: true }) }); await expect(row.getByText('数据源 0 · 节点 0 · 任务来源 0 · 模板 0')).toBeVisible(); await expect(row.getByText('系统管理员', { exact: true })).toHaveCount(0)
})

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`权限列表、抽屉与风险确认 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message)); page.on('console', value => { if (value.type() === 'error') errors.push(value.text()) })
    await page.setViewportSize(viewport); const model = await permissionFixture(page); await page.goto('/settings/access-control'); await expect(page).toHaveTitle(/OB Data Orch/); await expect(page.getByRole('heading', { name: '权限配置', exact: true })).toBeVisible(); await expect(page.getByText('本页 3 位用户')).toBeVisible(); await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath(`permissions-list-${viewport.width}.png`), fullPage: true })
    await openEditor(page); await page.screenshot({ path: info.outputPath(`permissions-editor-${viewport.width}.png`), fullPage: false }); await drawer(page).getByRole('tab', { name: '对象范围' }).click(); await expect(drawer(page).getByRole('combobox', { name: '数据源使用', exact: true })).toBeVisible(); await page.screenshot({ path: info.outputPath(`permissions-scopes-${viewport.width}.png`), fullPage: false })
    await drawer(page).getByRole('tab', { name: '独立能力' }).click(); await expect(drawer(page).getByRole('checkbox', { name: '敏感命令', exact: true })).toBeDisabled(); await drawer(page).getByRole('tab', { name: '固定角色' }).click(); await prepareSave(page); expect(model.writes).toHaveLength(0); await expect(page.getByText('新增系统管理职责，请核对目标用户和全部权限差异。')).toBeVisible(); await page.screenshot({ path: info.outputPath(`permissions-review-${viewport.width}.png`), fullPage: false })
    await page.getByRole('button', { name: '确认保存', exact: true }).click(); await expect(page.getByText('权限已保存，后续请求按新授权校验。')).toBeVisible(); expect(model.writes).toHaveLength(1); expect(model.writes[0]?.headers['if-match']).toBe('"rev-1"'); expect(model.writes[0]?.headers['idempotency-key']).toBeTruthy(); expect(model.writes[0]?.headers['x-csrf-token']).toBeTruthy(); expect(model.writes[0]?.body.confirmed).toBe(true); expect(Object.keys(model.writes[0]?.body ?? {}).sort()).toEqual(['capabilities', 'confirmed', 'reason', 'roles', 'scopes']); await expect(page.getByRole('button', { name: '配置 合成业务用户 的权限', exact: true })).toBeFocused(); expect(errors).toEqual([]); await info.attach('browser-version', { body: page.context().browser()?.version() ?? 'unknown', contentType: 'text/plain' })
  })
}

test('自身只读、停用只能撤销、字段校验、未保存确认与角色依赖清理', async ({ page }, info) => {
  const model = await permissionFixture(page); await page.goto('/settings/access-control'); await page.getByRole('button', { name: '查看 本机管理员 的权限' }).click(); await expect(page.getByText('当前登录用户的权限只读，不能在此修改自身授权。')).toBeVisible(); await expect(page.getByRole('button', { name: '比较并保存' })).toHaveCount(0); await page.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: '配置 已停用合成用户 的权限' }).click(); await expect(drawer(page).getByRole('checkbox', { name: '系统管理员', exact: true })).toBeDisabled(); await drawer(page).getByRole('button', { name: '关闭', exact: true }).click()
  await openEditor(page); await drawer(page).getByRole('checkbox', { name: '普通用户', exact: true }).uncheck(); await drawer(page).getByRole('button', { name: '比较并保存' }).click(); await expect(page.getByText('请填写变更原因。')).toBeVisible(); await drawer(page).getByRole('textbox', { name: '变更原因', exact: true }).fill('撤销合成用户任务创建资格'); await drawer(page).getByRole('button', { name: '比较并保存' }).click(); await expect(page.getByText('数据源使用：合成测试数据源')).toBeVisible(); await expect(page.getByText('节点使用 / 连接诊断：合成 Windows 节点')).toBeVisible(); await page.getByRole('button', { name: '返回编辑', exact: true }).click(); await drawer(page).getByRole('button', { name: '关闭', exact: true }).click(); await expect(page.getByText('放弃未保存的权限修改？')).toBeVisible(); await page.getByRole('button', { name: '继续编辑', exact: true }).click(); expect(model.writes).toHaveLength(0); await page.screenshot({ path: info.outputPath('permissions-validation-and-draft.png'), fullPage: false })
  await drawer(page).getByRole('button', { name: '关闭', exact: true }).click(); await page.getByRole('button', { name: '放弃修改', exact: true }).click(); await expect(page.getByRole('button', { name: '配置 合成业务用户 的权限', exact: true })).toBeFocused()
})

for (const mode of ['conflict', 'write-error'] as const) test(`保存 ${mode} 保留草稿且不显示成功`, async ({ page }, info) => {
  await permissionFixture(page, mode); await page.goto('/settings/access-control'); await openEditor(page); await prepareSave(page); await page.getByRole('button', { name: '确认保存', exact: true }).click(); await expect(drawer(page).getByRole('textbox', { name: '变更原因', exact: true })).toHaveValue('调整合成测试用户职责'); await expect(page.getByText(mode === 'conflict' ? '权限版本已变化，请刷新详情后重新比较；未覆盖其他变更。' : '未确认权限保存成功，草稿已保留，可使用原请求重试。')).toBeVisible(); await expect(page.getByText('权限已保存，后续请求按新授权校验。')).toHaveCount(0); if (mode === 'conflict') await expect(page.getByRole('button', { name: '比较并保存' })).toHaveCount(0); await page.screenshot({ path: info.outputPath(`permissions-${mode}.png`), fullPage: false })
})

for (const mode of ['empty', 'error', 'denied', 'long'] as const) test(`权限目录 ${mode} 状态`, async ({ page }, info) => {
  await permissionFixture(page, mode); await page.setViewportSize({ width: 1280, height: 720 }); await page.goto('/settings/access-control'); await expect(page.getByRole('heading', { name: '权限配置', exact: true })).toBeVisible(); if (mode === 'empty') await expect(page.getByText('暂无已认证用户', { exact: true })).toBeVisible(); else if (mode === 'long') await expect(page.getByText('本页 3 位用户')).toBeVisible(); else { await expect(page.getByText('权限目录暂不可用', { exact: true })).toBeVisible(); await expect(page.getByText('本页 0 位用户')).toHaveCount(0) }; expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath(`permissions-${mode}.png`), fullPage: true })
})

test('搜索、候选目录与键盘关闭恢复焦点', async ({ page }) => {
  const model = await permissionFixture(page); await page.goto('/settings/access-control'); await page.getByRole('textbox', { name: '登录标识或显示名称' }).fill('不存在的合成用户'); await page.getByRole('button', { name: '查询', exact: true }).click(); await expect(page.getByText('没有匹配的用户', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '重置', exact: true }).click(); await openEditor(page); await drawer(page).getByRole('tab', { name: '对象范围' }).click(); await drawer(page).getByRole('combobox', { name: '数据源使用', exact: true }).click(); await expect(page.locator('.ant-select-dropdown:visible').getByText('合成测试数据源', { exact: true })).toBeVisible(); expect(model.calls.some(call => call.includes('/candidates?type=DATA_SOURCE_USE'))).toBe(true); await page.keyboard.press('Escape'); await page.keyboard.press('Escape'); await expect(page.getByRole('button', { name: '配置 合成业务用户 的权限', exact: true })).toBeFocused()
})

test('原生 200% 缩放中保存、取消和确认保持可见并可键盘操作', async ({ browserName }, info) => {
  test.skip(browserName !== 'chromium', '原生缩放使用 Chromium 扩展接口')
  test.setTimeout(120000)
  const profilePath = await mkdtemp(fileURLToPath(new URL('../../artifacts/permission-zoom-', import.meta.url)))
  const extension = fileURLToPath(new URL('./fixtures/zoom-extension', import.meta.url)); const context = await chromium.launchPersistentContext(profilePath, { channel: 'chromium', headless: true, viewport: null, deviceScaleFactor: undefined, args: ['--window-size=1440,1024', `--disable-extensions-except=${extension}`, `--load-extension=${extension}`] })
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker')
    const page = await context.newPage()
    await permissionFixture(page)
    await page.goto('http://127.0.0.1:15174/settings/access-control')
    await expect(page.getByText('本页 3 位用户')).toBeVisible()
    const before = await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio }))
    await worker.evaluate(async () => {
      const api = (globalThis as unknown as { chrome: { tabs: { query(value: object): Promise<{ id: number; url: string }[]>; setZoom(id: number, factor: number): Promise<void> } } }).chrome
      const target = (await api.tabs.query({})).find(tab => tab.url.includes('15174'))
      if (!target) throw new Error('隔离页面不存在')
      await api.tabs.setZoom(target.id, 2)
    })
    await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(before.ratio * 2)
    const navigation = page.getByRole('button', { name: '系统管理', exact: true })
    await navigation.focus()
    await page.keyboard.press('ArrowDown')
    const menu = page.getByRole('menu', { name: '系统管理子菜单', exact: true })
    await expect(menu).toBeInViewport()
    await expect(menu.getByRole('menuitem', { name: '权限配置', exact: true })).toBeFocused()
    await expect(page.locator('.ant-dropdown:visible')).toHaveCSS('opacity', '1')
    const navigationSession = await context.newCDPSession(page)
    const navigationCapture = await navigationSession.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('system-menu-native-200-percent.png'), Buffer.from(navigationCapture.data, 'base64'))
    await navigationSession.detach()
    await page.keyboard.press('Enter')
    await expect(menu).toHaveCount(0)
    await expect(navigation).toBeFocused()
    for (const [label, path] of [['平台设置', '/settings/platform'], ['调度策略', '/settings/scheduling'], ['安全策略', '/settings/security'], ['工具与兼容', '/settings/tools']] as const) {
      await navigation.focus()
      await page.keyboard.press('ArrowDown')
      await menu.getByRole('menuitem', { name: label, exact: true }).click()
      await expect(page).toHaveURL(new RegExp(`${path}$`))
      await expect(page.getByRole('heading', { name: label, exact: true })).toBeInViewport()
      await expect(page.getByRole('textbox')).toHaveCount(0)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await expect(page.locator('.ant-dropdown:visible')).toHaveCount(0)
      const settingsSession = await context.newCDPSession(page)
      const settingsCapture = await settingsSession.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
      await writeFile(info.outputPath(`${label}-native-200-percent.png`), Buffer.from(settingsCapture.data, 'base64'))
      await settingsSession.detach()
    }
    await navigation.focus()
    await page.keyboard.press('ArrowDown')
    await menu.getByRole('menuitem', { name: '权限配置', exact: true }).click()
    await expect(page.getByText('本页 3 位用户')).toBeVisible()
    await openEditor(page)
    await expect(drawer(page).getByRole('button', { name: '关闭', exact: true })).toBeInViewport()
    await expect(drawer(page).getByRole('button', { name: '比较并保存' })).toBeInViewport()
    await prepareSave(page)
    const confirm = page.getByRole('button', { name: '确认保存', exact: true })
    await expect(confirm).toBeInViewport()
    await confirm.focus()
    await page.keyboard.press('Enter')
    await expect(page.getByText('权限已保存，后续请求按新授权校验。')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await openEditor(page)
    const session = await context.newCDPSession(page)
    const capture = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
    await writeFile(info.outputPath('permissions-native-200-percent.png'), Buffer.from(capture.data, 'base64'))
    await session.detach()
    await info.attach('native-zoom', { body: JSON.stringify({ before, after: await page.evaluate(() => ({ width: innerWidth, ratio: devicePixelRatio })), browser: context.browser()?.version() }), contentType: 'application/json' })
  } finally {
    await test.step('关闭本例独占的隔离浏览器', async () => { const browser = context.browser(); if (browser) await browser.close(); else await context.close() })
    await test.step('清除本例浏览器临时目录', () => rm(profilePath, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 }))
  }
})
