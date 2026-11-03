import { expect, test, type Page } from '@playwright/test'
import { normalImportNode, normalImportSources } from './fixtures/normal-import'

const account = (page: Page) => page.getByRole('button', { name: /^(当前用户 |账户菜单)/ })
const passwordDialog = (page: Page) => page.getByRole('dialog', { name: '修改密码', exact: true })

async function fixture(page: Page, result = 'success', username = 'admin') {
  await page.context().grantPermissions(['local-network-access'], { origin: 'http://127.0.0.1:15174' })
  const errors: string[] = [], writes: { csrf: string | undefined; current: boolean; next: boolean }[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => {
    if (message.type() === 'error' && !/\/api\/v1\/session\/password$/.test(message.location().url)) errors.push(message.text())
  })
  await page.route('**/*', route => new URL(route.request().url()).hostname === '127.0.0.1' ? route.continue() : route.abort())
  // 镜像认证 Handler 的 HTML 注入，直接渲染已核实用户名，不依赖前端写死账户。
  await page.route('**/data-sources?uiFixture=data-sources', async route => {
    const response = await route.fetch()
    await route.fulfill({ response, body: (await response.text()).replaceAll('local-mvp-username-v1', username) })
  })
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/session') return route.fulfill({ json: { username } })
    if (path === '/api/v1/data-sources') return route.fulfill({ json: { items: normalImportSources } })
    if (path === '/api/v1/execution-nodes') return route.fulfill({ json: { items: [normalImportNode] } })
    if (path === '/api/v1/storage-credentials') return route.fulfill({ json: { items: [] } })
    if (path === '/api/v1/session/password') {
      const body = new URLSearchParams(route.request().postData() ?? '')
      writes.push({ csrf: route.request().headers()['x-csrf-token'], current: body.get('currentPassword') === 'synthetic-current', next: body.get('newPassword') === 'synthetic-new-password' })
      if (result === 'network') return route.abort('failed')
      if (result === 'wrong') return route.fulfill({ status: 400, json: { error: { code: 'CURRENT_PASSWORD_INVALID' } } })
      if (result === 'unavailable') return route.fulfill({ status: 503, json: { error: { code: 'PASSWORD_CHANGE_UNAVAILABLE' } } })
      if (result === 'expired') return route.fulfill({ status: 401, json: { error: { code: 'UNAUTHENTICATED' } } })
      return route.fulfill({ status: 204 })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  await page.route('**/login', route => route.fulfill({ contentType: 'text/html; charset=utf-8', body: '<!doctype html><meta charset="UTF-8"><title>合成登录页</title><h1>合成登录页</h1>' }))
  await page.route('**/favicon.ico', route => route.fulfill({ status: 204 }))
  return { errors, writes }
}
async function openPassword(page: Page) {
  // 全局外壳可早于异步路由工作区挂载；先等目标页就绪，避免路由完成时关闭刚打开的菜单。
  await expect(page.locator('#main-workspace').getByRole('heading', { level: 1 })).toBeVisible()
  await account(page).click()
  await page.getByRole('menuitem', { name: '修改密码', exact: true }).click()
  await expect(passwordDialog(page)).toBeVisible()
}
async function fillPassword(page: Page) {
  const dialog = passwordDialog(page)
  await dialog.getByLabel('当前密码', { exact: true }).fill('synthetic-current')
  await dialog.getByLabel('新密码', { exact: true }).fill('synthetic-new-password')
  await dialog.getByLabel('确认新密码', { exact: true }).fill('synthetic-new-password')
}

for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]) {
  test(`账户菜单、改密取消与焦点 ${viewport.width}×${viewport.height}`, async ({ page }, info) => {
    const model = await fixture(page)
    await page.setViewportSize(viewport)
    await page.goto('/data-sources?uiFixture=data-sources')
    await expect(page).toHaveTitle('LDP · 数据源管理')
    await expect(page.getByRole('heading', { name: '数据源管理', exact: true })).toBeVisible()
    await expect(account(page)).toHaveText('admin')
    await account(page).focus()
    await page.keyboard.press('ArrowDown')
    await expect(page.getByRole('menu', { name: '账户操作' })).toBeVisible()
    await expect(page.getByRole('menuitem')).toHaveText(['修改密码', '退出登录'])
    await expect(page.getByRole('menuitem', { name: '修改密码', exact: true })).toBeFocused()
    await page.keyboard.press('ArrowDown')
    await expect(page.getByRole('menuitem', { name: '退出登录', exact: true })).toBeFocused()
    await page.keyboard.press('Home')
    await expect(page.getByRole('menuitem', { name: '修改密码', exact: true })).toBeFocused()
    await page.keyboard.press('End')
    await expect(page.getByRole('menuitem', { name: '退出登录', exact: true })).toBeFocused()
    await page.screenshot({ path: info.outputPath(`account-menu-${viewport.width}.png`) })
    await page.keyboard.press('Escape')
    await expect(account(page)).toBeFocused()
    await openPassword(page)
    await expect(passwordDialog(page).getByLabel('当前密码', { exact: true })).toBeFocused()
    await fillPassword(page)
    await page.screenshot({ path: info.outputPath(`password-dialog-${viewport.width}.png`) })
    for (let i = 0; i < 8; i++) { await page.keyboard.press('Tab'); expect(await passwordDialog(page).evaluate(element => element.contains(document.activeElement))).toBe(true) }
    await page.keyboard.press('Escape')
    await expect(passwordDialog(page)).not.toBeVisible()
    await expect(account(page)).toBeFocused()
    await openPassword(page)
    await expect(passwordDialog(page).getByLabel('当前密码', { exact: true })).toHaveValue('')
    await expect(passwordDialog(page).getByLabel('新密码', { exact: true })).toHaveValue('')
    await passwordDialog(page).getByRole('button', { name: '取消', exact: true }).click()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(model.writes).toHaveLength(0)
    expect(model.errors).toEqual([])
    await expect(page.locator('vite-error-overlay')).toHaveCount(0)
    await info.attach('browser-environment', { body: JSON.stringify({ browser: page.context().browser()?.version(), viewport, url: page.url() }), contentType: 'application/json' })
  })
}

test('字段校验不发送请求；保存使用当前会话并返回登录页', async ({ page }) => {
  const model = await fixture(page)
  await page.goto('/data-sources?uiFixture=data-sources')
  await openPassword(page)
  const dialog = passwordDialog(page)
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  await expect(dialog.getByText('请输入当前密码', { exact: true })).toBeVisible()
  await fillPassword(page)
  await dialog.getByLabel('确认新密码', { exact: true }).fill('synthetic-mismatch')
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  await expect(dialog.getByText('两次输入的新密码不一致', { exact: true })).toBeVisible()
  expect(model.writes).toHaveLength(0)
  await dialog.getByLabel('确认新密码', { exact: true }).fill('synthetic-new-password')
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page).toHaveURL(/\/login$/)
  expect(model.writes).toEqual([{ csrf: 'local-mvp-csrf-v1', current: true, next: true }])
  expect(model.errors).toEqual([])
})

for (const result of ['wrong', 'unavailable', 'network', 'expired']) test(`修改密码失败保持明确反馈 ${result}`, async ({ page }, info) => {
  const model = await fixture(page, result)
  await page.goto('/data-sources?uiFixture=data-sources')
  await openPassword(page)
  await fillPassword(page)
  const dialog = passwordDialog(page)
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  if (result === 'wrong') { await expect(dialog.getByText('当前密码不正确', { exact: true })).toBeVisible(); await expect(dialog.getByLabel('当前密码', { exact: true })).toHaveValue('') }
  else await expect(dialog.getByRole('alert').filter({ hasText: result === 'unavailable' ? '密码未能保存' : result === 'expired' ? '登录已失效' : '修改结果暂无法确认' })).toBeVisible()
  if (result === 'network' || result === 'expired') await expect(dialog.getByLabel('新密码', { exact: true })).toHaveValue('')
  await expect(page).toHaveURL(/data-sources/)
  await page.screenshot({ path: info.outputPath(`password-${result}.png`) })
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  await openPassword(page)
  await expect(passwordDialog(page).getByLabel('新密码', { exact: true })).toHaveValue('')
  expect(model.writes).toHaveLength(1)
  expect(model.errors).toEqual([])
})

test('保存期间重复点击只发送一次，卸载后的迟到成功不跳转新页面', async ({ page }) => {
  const model = await fixture(page)
  let release!: () => void
  const waiting = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/session/password', async route => { await waiting; await route.fulfill({ status: 204 }).catch(() => undefined) })
  await page.goto('/data-sources?uiFixture=data-sources')
  await openPassword(page)
  await fillPassword(page)
  const requests: string[] = []
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/session/password') requests.push(request.method()) })
  const save = passwordDialog(page).getByRole('button', { name: '确定', exact: true })
  await save.click()
  await expect.poll(() => requests.length).toBe(1)
  await expect(save).toBeDisabled()
  await save.dispatchEvent('click')
  await expect(passwordDialog(page).getByRole('button', { name: '取消', exact: true })).toBeDisabled()
  expect(requests).toEqual(['POST'])
  await page.reload()
  release()
  await expect(page.getByRole('heading', { name: '数据源管理', exact: true })).toBeVisible()
  await expect(passwordDialog(page)).toHaveCount(0)
  await expect(page).toHaveURL(/data-sources/)
  expect(model.errors).toEqual([])
})

test('改密沿用未保存离开许可，取消不发请求且保留业务输入', async ({ page }) => {
  const model = await fixture(page)
  await page.goto('/imports/normal/new')
  await page.getByRole('radio', { name: /合成 MySQL 导入目标/ }).check()
  await openPassword(page)
  await fillPassword(page)
  await passwordDialog(page).getByRole('button', { name: '确定', exact: true }).click()
  const leave = page.getByRole('dialog', { name: '离开普通导入配置？', exact: true })
  await expect(leave).toBeVisible()
  expect(model.writes).toHaveLength(0)
  await leave.getByRole('button', { name: '继续配置', exact: true }).click()
  await expect(passwordDialog(page)).toBeVisible()
  await passwordDialog(page).getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.getByRole('radio', { name: /合成 MySQL 导入目标/ })).toBeChecked()
  expect(model.errors).toEqual([])
})

test('安全令牌缺失不改密，长用户名和窄屏保留可见入口', async ({ page }) => {
  const username = 'synthetic-long-account-'.repeat(8), model = await fixture(page, 'success', username)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/data-sources?uiFixture=data-sources')
  await expect(account(page)).toHaveAccessibleName(`当前用户 ${username}`)
  await expect(account(page)).toBeInViewport()
  await openPassword(page)
  await fillPassword(page)
  await page.evaluate(() => document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.remove())
  await passwordDialog(page).getByRole('button', { name: '确定', exact: true }).click()
  await expect(passwordDialog(page).getByRole('alert').filter({ hasText: '页面安全校验失效' })).toBeVisible()
  expect(model.writes).toHaveLength(0)
  expect(model.errors).toEqual([])
})

test('按 OCP 参考视口呈现窄密码弹窗', async ({ page }, info) => {
  const model = await fixture(page)
  await page.setViewportSize({ width: 909, height: 869 })
  await page.goto('/data-sources?uiFixture=data-sources')
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await account(page).click()
  await page.screenshot({ path: info.outputPath('ocp-menu-reference-909.png') })
  await page.getByRole('menuitem', { name: '修改密码', exact: true }).click()
  const dialog = passwordDialog(page)
  // Ant 将 dialog 角色赋给全屏容器，宽度契约属于内部的弹窗正文。
  await expect(dialog.getByRole('document')).toHaveCSS('width', '400px')
  await expect(dialog.getByPlaceholder('请输入', { exact: true })).toBeVisible()
  await expect(dialog.getByPlaceholder('请输入密码', { exact: true })).toBeVisible()
  await expect(dialog.getByPlaceholder('请再次输入新密码', { exact: true })).toBeVisible()
  await page.screenshot({ path: info.outputPath('ocp-password-reference-909.png') })
  expect(model.errors).toEqual([])
})
