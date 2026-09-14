/* 使用合成依赖验证桌面组件；拦截全部业务 API，禁止本检查连接真实数据库或创建任务。 */
const assert = require('node:assert/strict')
const path = require('node:path')
const fs = require('node:fs/promises')
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')

const baseURL = process.env.DESKTOP_UI_URL || 'http://127.0.0.1:5173'
const output = path.resolve(process.env.DESKTOP_UI_OUTPUT || 'tmp/desktop-ui-verification')
const viewports = [{ width: 1920, height: 1080 }, { width: 1440, height: 1024 }, { width: 1280, height: 720 }]

async function main() {
  await fs.mkdir(output, { recursive: true })
  const browser = await chromium.launch({ channel: 'chrome', headless: true })
  const page = await browser.newPage({ reducedMotion: 'reduce' })
  const errors = []
  const checks = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route(url => url.pathname.startsWith('/api/'), route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'SERVICE_UNAVAILABLE', message: '合成检查：服务暂不可用。', requestId: 'desktop-ui-synthetic' } }) }))
  const sourceURL = `${baseURL}/data-sources?uiFixture=data-sources`
  async function noOverflow(label) {
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${label}: 页面横向溢出`)
  }
  try {
    for (const viewport of viewports) {
      await page.setViewportSize(viewport)
      await page.goto(sourceURL)
      await page.getByRole('button', { name: 'Production finance reporting', exact: true }).waitFor()
      await noOverflow(`数据源 ${viewport.width}`)
      const stateColors = await page.locator('.orch-fact-table .orch-status').evaluateAll(elements => elements.map(element => ({
        text: element.textContent.trim(), color: getComputedStyle(element.querySelector('.orch-status-dot') || element.querySelector('svg')).color,
        dot: element.querySelector('.orch-status-dot') ? getComputedStyle(element.querySelector('.orch-status-dot')).backgroundColor : null,
        background: getComputedStyle(element).backgroundColor,
      })))
      assert(stateColors.length > 0)
      assert(stateColors.every(state => state.background === 'rgba(0, 0, 0, 0)'), '行内状态不使用实心底色')
      assert(stateColors.filter(state => state.text === '已启用').every(state => state.dot === 'rgb(14, 108, 64)'))
      assert(stateColors.filter(state => state.text === '已停用').every(state => state.dot === 'rgb(218, 30, 40)'))
      assert(stateColors.filter(state => ['未测试', '状态待确认', '最近测试成功'].includes(state.text)).every(state => (state.dot || state.color) === 'rgb(82, 82, 82)'), '未知和历史结果不能伪装成绿色资格')
      await page.screenshot({ path: path.join(output, `sources-${viewport.width}.png`), fullPage: true })
      const trigger = page.getByRole('button', { name: 'Production finance reporting 的操作', exact: true })
      await trigger.focus()
      await page.keyboard.press('ArrowUp')
      const unavailable = page.getByRole('menuitem', { name: '永久删除' })
      await unavailable.waitFor()
      assert(await unavailable.evaluate(element => element === document.activeElement), '向上键应定位末项')
      assert.equal(await unavailable.getAttribute('aria-disabled'), 'true')
      assert.match(await unavailable.innerText(), /历史任务引用/)
      await page.keyboard.press('Enter')
      assert.equal(await page.locator('dialog[open]').count(), 0, '禁用项不得执行操作')
      await page.keyboard.press('Home')
      assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), '编辑配置')
      await page.keyboard.press('End')
      await page.screenshot({ path: path.join(output, `menu-${viewport.width}.png`) })
      await page.keyboard.press('Escape')
      assert(await trigger.evaluate(element => element === document.activeElement), '菜单关闭应归还焦点')
      await page.getByRole('button', { name: '新建数据源', exact: true }).click()
      await page.getByRole('button', { name: '保存配置', exact: true }).click()
      assert(await page.locator('.orch-validation-summary').isVisible())
      await page.waitForFunction(() => getComputedStyle(document.querySelector('[aria-invalid="true"]')).borderColor === 'rgb(218, 30, 40)')
      assert.equal(await page.locator('[aria-invalid="true"]').first().evaluate(element => getComputedStyle(element).borderColor), 'rgb(218, 30, 40)')
      const saveBounds = await page.getByRole('button', { name: '保存配置', exact: true }).boundingBox()
      assert(saveBounds && saveBounds.y + saveBounds.height <= viewport.height, '保存按钮必须处于视口内')
      await page.screenshot({ path: path.join(output, `validation-${viewport.width}.png`) })
      await page.keyboard.press('Escape')
      assert.equal(await page.locator('dialog[open]').count(), 0)
      checks.push(`${viewport.width}x${viewport.height}: 表格、菜单、校验和底栏`)
    }

    // 校验必须在失焦时出现，仍然非法的输入不能清除错误；不提交配置。
    await page.goto(sourceURL)
    await page.getByRole('button', { name: '新建数据源', exact: true }).click()
    const port = page.getByRole('spinbutton', { name: /SQL 端口/ })
    await port.fill('70000')
    await port.press('Tab')
    assert.equal(await port.getAttribute('aria-invalid'), 'true')
    await port.fill('70001')
    assert.equal(await port.getAttribute('aria-invalid'), 'true')
    await port.fill('2883')
    assert.equal(await port.getAttribute('aria-invalid'), 'false')
    await page.keyboard.press('Escape')
    checks.push('端口失焦校验、无效输入保持错误、修正后恢复')

    // 隐藏的高级字段报错时展开并聚焦；只编辑合成数据，不发起真实连接测试。
    await page.goto(sourceURL)
    await page.getByRole('button', { name: 'Production finance reporting', exact: true }).click()
    await page.locator('.orch-advanced summary').click()
    await page.getByLabel('sys 账号', { exact: true }).fill('synthetic_reader')
    await page.locator('.orch-advanced summary').click()
    await page.getByRole('button', { name: '保存配置', exact: true }).click()
    assert.equal(await page.locator('.orch-advanced').getAttribute('open'), '')
    assert(await page.getByLabel('sys 密码', { exact: true }).evaluate(element => element === document.activeElement))
    await page.keyboard.press('Escape')
    assert.equal(await page.locator('dialog[open]').count(), 2)
    assert(await page.getByRole('button', { name: '继续编辑', exact: true }).evaluate(element => element === document.activeElement), '危险操作默认聚焦安全动作')
    await page.screenshot({ path: path.join(output, 'discard-confirmation.png') })
    await page.getByRole('button', { name: '继续编辑', exact: true }).click()
    assert.equal(await page.locator('dialog[open]').count(), 1)
    checks.push('高级字段定位与嵌套确认焦点')

    // 使用现有内存网关验证测试中和失败结果，全部业务 API 仍被拦截。
    await page.goto(sourceURL)
    await page.getByRole('button', { name: 'Production finance reporting', exact: true }).click()
    await page.getByRole('tab', { name: '连接测试', exact: true }).click()
    await page.getByRole('combobox', { name: /执行节点/ }).selectOption('visual-validation-runtime')
    await page.getByRole('button', { name: '测试连接', exact: true }).click()
    await page.locator('.connection-test-result.is-pending').waitFor()
    await page.screenshot({ path: path.join(output, 'connection-pending.png') })
    await page.locator('.connection-test-result.is-error').waitFor()
    assert(await page.getByText('不代表真实连接成功，不能据此启用数据源。', { exact: true }).isVisible())
    await page.getByText('查看验证记录', { exact: true }).click()
    assert(await page.getByText('DATABASE_TCP_TIMEOUT', { exact: true }).isVisible())
    await page.screenshot({ path: path.join(output, 'connection-failed.png') })
    checks.push('合成连接测试进行中、失败原因、验证记录展开和非真实结果标记')

    // 直接挂载生产组件，覆盖真实业务不容易稳定停留的异步状态。
    await page.goto(sourceURL)
    await page.getByRole('button', { name: '新建数据源', exact: true }).waitFor()
    await page.evaluate(async () => {
      // 使用应用实际加载的 Vue 模块 URL，避免依赖预构建版本参数造成两个运行时实例。
      const vueURL = performance.getEntriesByType('resource').map(entry => entry.name).find(name => new URL(name).pathname.endsWith('/deps/vue.js'))
      if (!vueURL) throw new Error('没有找到页面使用的 Vue 运行时')
      const { createApp, h, reactive } = await import(vueURL)
      const { default: WorkbenchButton } = await import('/src/components/WorkbenchButton.vue')
      const { default: OrchButton } = await import('/src/workbench/components/OrchButton.vue')
      const { default: OrchDialog } = await import('/src/workbench/components/OrchDialog.vue')
      const { default: WorkbenchAlertDialog } = await import('/src/components/WorkbenchAlertDialog.vue')
      const state = reactive({ busy: false, open: false, alertOpen: false, clicks: 0 })
      const host = document.createElement('div')
      document.getElementById('main-workspace').append(host)
      const app = createApp({ render: () => h('div', { class: 'orch-ui', style: 'display:flex;gap:16px;padding:24px' }, [
        h(WorkbenchButton, { busy: state.busy, variant: 'danger', onClick: () => state.clicks++ }, () => '确认变更'),
        h(OrchButton, { busy: state.busy, variant: 'primary', onClick: () => state.clicks++ }, () => '保存配置'),
        h(OrchDialog, { open: state.open, title: '合成检查确认', busy: state.busy, onCancel: () => { state.open = false } }, () => h('p', '需要核对的合成影响。'.repeat(180))),
        h(WorkbenchAlertDialog, { open: state.alertOpen, title: '合成告警确认', description: '需要核对的合成影响。'.repeat(180), confirmLabel: '确认更新', busy: state.busy, onCancel: () => { state.alertOpen = false } }),
      ]) })
      app.config.idPrefix = 'desktop-check'
      app.mount(host)
      window.desktopComponentCheck = { state, app }
    })
    const workbenchButton = page.getByRole('button', { name: '确认变更', exact: true })
    const orchButton = page.getByRole('button', { name: '保存配置', exact: true })
    await workbenchButton.hover()
    assert.equal(await workbenchButton.evaluate(element => getComputedStyle(element).backgroundColor), 'rgb(184, 25, 33)')
    const widths = [await workbenchButton.boundingBox(), await orchButton.boundingBox()]
    await page.evaluate(() => { window.desktopComponentCheck.state.busy = true })
    assert.equal(await workbenchButton.getAttribute('aria-busy'), 'true')
    assert(await workbenchButton.isDisabled() && await orchButton.isDisabled())
    assert.equal((await workbenchButton.boundingBox()).width, widths[0].width)
    assert.equal((await orchButton.boundingBox()).width, widths[1].width)
    // 原生 disabled 阻止真实点击；测试保留无障碍命名与稳定布局。
    await page.setViewportSize({ width: 1280, height: 600 })
    await page.evaluate(() => { window.desktopComponentCheck.state.open = true })
    await page.getByRole('dialog', { name: '合成检查确认' }).waitFor()
    await page.keyboard.press('Escape')
    assert(await page.getByRole('dialog', { name: '合成检查确认' }).isVisible(), '忙碌确认框不能被 Escape 关闭')
    const confirmBounds = await page.getByRole('button', { name: '确认', exact: true }).boundingBox()
    assert(confirmBounds.y + confirmBounds.height <= 600)
    assert(await page.locator('dialog[open] .orch-confirm-body').evaluate(element => element.scrollHeight > element.clientHeight))
    await page.screenshot({ path: path.join(output, 'dialog-1280x600.png') })
    await page.evaluate(() => { window.desktopComponentCheck.state.busy = false })
    await page.keyboard.press('Escape')
    await page.evaluate(() => { window.desktopComponentCheck.state.alertOpen = true })
    await page.getByRole('alertdialog').waitFor()
    const alertBounds = await page.getByRole('button', { name: '确认更新', exact: true }).boundingBox()
    assert(alertBounds.y + alertBounds.height <= 600)
    assert(await page.locator('.workbench-alert-dialog-body').evaluate(element => element.scrollHeight > element.clientHeight))
    checks.push('两套按钮加载尺寸、危险悬停、低高度确认框、忙碌关闭保护')

    const routes = ['/', '/exports/new', '/imports/normal/new', '/imports/direct/new', '/tasks', '/templates', '/nodes', '/logs', '/settings', '/settings/storage-credentials', '/settings/access-control']
    for (const viewport of viewports) {
      await page.setViewportSize(viewport)
      for (const route of routes) {
        await page.goto(`${baseURL}${route}`)
        await page.locator('.enterprise-shell').waitFor()
        await page.locator('h1').first().waitFor()
        await noOverflow(`${route} ${viewport.width}`)
      }
      checks.push(`${viewport.width}: 11 个模块的桌面布局及合成服务不可用状态`)
    }
    assert.deepEqual(errors, [])
    await fs.writeFile(path.join(output, 'result.json'), JSON.stringify({ checks, errors, realOperations: false }, null, 2))
    console.log(JSON.stringify({ checks, errors, output }, null, 2))
  } catch (error) {
    await page.screenshot({ path: path.join(output, 'failure.png'), fullPage: true })
    console.error({ pageErrors: errors, dialogs: await page.locator('dialog').evaluateAll(elements => elements.map(element => ({ open: element.open, labelledBy: element.getAttribute('aria-labelledby'), text: element.querySelector('h2')?.textContent }))) })
    throw error
  } finally { await browser.close() }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
