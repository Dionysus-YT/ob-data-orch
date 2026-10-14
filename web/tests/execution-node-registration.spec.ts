import { expect, test, type Page, type Route } from '@playwright/test'

const time = '2026-10-10T00:00:00Z'
function node(id: string) {
  return {
    id, displayName: '合成节点 ' + id, platform: 'WINDOWS_AMD64',
    managementState: 'DISABLED', agentAssociationStatus: 'PENDING', heartbeatStatus: 'NEVER_CONNECTED',
    lastHeartbeatAt: null, environmentStatus: 'NOT_CHECKED', capacityStatus: 'UNKNOWN',
    acceptsNewTasks: false, unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED'],
    revision: 1, updatedAt: time, createdAt: time, allowedRoots: ['/E:/synthetic-exports'],
    toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java/bin/java.exe',
  }
}
async function mockNodes(page: Page) {
  const state = {
    items: [node('A'), node('B')], reads: 0, listStatus: 200, writeStatus: 200,
    writes: [] as Record<string, unknown>[], holdUpdates: false, held: [] as Route[], enrollments: 0,
  }
  // 所有 API 均由合成响应接管；隔离端口页面不能向开发控制面发送请求。
  await page.route('**/api/v1/**', route => route.fulfill({ json: { items: [] } }))
  await page.route('**/api/v1/execution-nodes**', async route => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    if (path.endsWith(':enrollments')) {
      state.enrollments++
      return route.fulfill({ status: 201, json: { requestId: 'synthetic-request', enrollmentId: 'synthetic-enrollment', nodeId: 'C', enrollmentMaterial: 'synthetic-only-material', expiresAt: '2099-01-01T00:00:00Z', displayedOnce: true } })
    }
    if (path === '/api/v1/execution-nodes' && method === 'GET') {
      state.reads++
      return state.listStatus === 200
        ? route.fulfill({ json: { items: state.items } })
        : route.fulfill({ status: state.listStatus, json: { code: 'FORBIDDEN', message: '合成授权失效' } })
    }
    if (method === 'POST' || method === 'PATCH') {
      const input = route.request().postDataJSON() as Record<string, unknown>
      state.writes.push(input)
      if (state.holdUpdates && method === 'PATCH') { state.held.push(route); return }
      if (state.writeStatus !== 200) return route.fulfill({ status: state.writeStatus, json: {
        code: 'VALIDATION_FAILED', message: '合成校验失败', fieldErrors: [{ field: 'budgetPolicy', code: 'EXECUTION_NODE_BUDGET_INVALID', message: '合成预算校验失败' }],
      } })
      if (method === 'POST') {
        state.items.push({ ...node('C'), ...input } as ReturnType<typeof node>)
        return route.fulfill({ status: 201, json: { id: 'C' } })
      }
      const id = path.split('/').pop()!
      const updated = { ...node(id), ...input, revision: 2 } as ReturnType<typeof node>
      state.items = state.items.map(item => item.id === id ? updated : item)
      return route.fulfill({ json: { item: updated } })
    }
    return route.fulfill({ json: { item: state.items.find(item => item.id === path.split('/').pop()) } })
  })
  return state
}
async function requestNavigation(page: Page, path: string) {
  await page.evaluate(async target => {
    const modulePath = '/src/router/index.ts'
    const { router } = await import(/* @vite-ignore */ modulePath)
    // 确认前导航处于等待中，不能等待该 Promise 后才操作确认框。
    void router.push(target)
  }, path)
}
async function fillNewNode(page: Page) {
  await page.getByLabel('节点名称', { exact: true }).fill('合成新节点')
  await page.getByLabel('OB Loader/Dumper 安装目录', { exact: true }).fill('E:/synthetic-tools')
  await page.getByLabel('工具专用 Java 8 路径', { exact: true }).fill('C:/synthetic-java/bin/java.exe')
  await page.getByLabel('导出数据目录白名单', { exact: true }).fill('/E:/synthetic-exports')
}

async function selectNodeOption(page: Page, field: string, label: string) {
  const control = page.getByRole('combobox').and(page.getByLabel(field, { exact: true }))
  await control.focus()
  await control.press('ArrowDown')
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').getByText(label, { exact: true }).click()
}

for (const target of [
  { architecture: 'x86_64', platform: 'LINUX_AMD64', package: 'linux-x86_64' },
  { architecture: 'aarch64', platform: 'LINUX_ARM64', package: 'linux-aarch64' },
]) {
  test('按操作系统与 ' + target.architecture + ' 注册，保留协议值并匹配 Agent 包', async ({ page }) => {
    const state = await mockNodes(page); await page.goto('/nodes/new')
    const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
    await expect(drawer.locator('#node-tool-hint')).toContainText('bin\\windows\\obdumper.bat')
    await expect(drawer.locator('#node-java-hint')).toContainText('bin\\java.exe')
    const architectureControl = drawer.getByRole('combobox').and(drawer.getByLabel('CPU 架构', { exact: true }))
    await architectureControl.focus()
    await architectureControl.press('ArrowDown')
    await expect(page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'aarch64' })).toHaveClass(/ant-select-item-option-disabled/)
    await page.locator('.ant-select-dropdown:visible .ant-select-item-option').getByText('x86_64', { exact: true }).click()
    await selectNodeOption(page, '操作系统', 'Linux')
    await selectNodeOption(page, 'CPU 架构', target.architecture)
    await expect(drawer.locator('#node-tool-hint')).toContainText('bin/obdumper')
    await expect(drawer.locator('#node-java-hint')).toContainText(target.architecture + ' 架构一致')
    await expect(drawer.locator('#node-roots-hint')).toContainText('路径区分大小写')
    await fillNewNode(page)
    await drawer.getByLabel('OB Loader/Dumper 安装目录', { exact: true }).fill('/opt/synthetic-tools')
    await drawer.getByLabel('工具专用 Java 8 路径', { exact: true }).fill('/opt/synthetic-java/bin/java')
    await drawer.getByLabel('导出数据目录白名单', { exact: true }).fill('/synthetic-exports')
    await drawer.getByRole('button', { name: '保存并继续 Agent 关联' }).click()
    await expect(page).toHaveURL(/\/nodes\/C\?registration=created$/)
    expect(state.writes).toEqual([{
      displayName: '合成新节点', platform: target.platform, allowedRoots: ['/synthetic-exports'],
      toolHome: '/opt/synthetic-tools', javaPath: '/opt/synthetic-java/bin/java',
    }])
    await expect(page.locator('.node-detail-title')).toContainText('Linux · ' + target.architecture)
    await expect(page.getByRole('group', { name: '节点操作', exact: true }).getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveAttribute('href', '/ob-data-orch-agent-' + target.package + '.zip')
    await expect(page.getByRole('region', { name: '完成 Agent 关联', exact: true }).getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveCount(0)
    expect(state.enrollments).toBe(0)
    await expect(page.getByRole('region', { name: '完成 Agent 关联', exact: true })).toContainText('root 或 sudo 权限运行 start.sh')
    await expect(page.getByRole('region', { name: '完成 Agent 关联', exact: true })).toContainText(target.architecture + ' 架构匹配')
    await page.getByRole('button', { name: '生成一次性注册码' }).click()
    const enrollment = page.getByRole('dialog', { name: '一次性 Agent 注册码', exact: true })
    await expect(enrollment).toContainText('root 或 sudo 权限运行 start.sh')
    await expect(enrollment.locator('code')).toHaveText('./start.sh')
    await enrollment.getByRole('button', { name: '关闭并清除' }).click()
    await expect(enrollment).toHaveCount(0)
  })
}

test('两种平台帮助随选择切换，保留已填路径且不显示具体发行版名称', async ({ page }, info) => {
  await mockNodes(page); await page.goto('/nodes/new')
  await page.setViewportSize({ width: 1440, height: 1024 })
  const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
  await fillNewNode(page)
  await expect(drawer.locator('#node-tool-hint')).toContainText('bin\\windows\\obdumper.bat')
  await expect(drawer.locator('#node-java-hint')).toContainText('无需加引号')
  await expect(drawer.getByRole('group', { name: 'CPU 架构', exact: true })).toHaveAccessibleDescription(/使用 x86_64 架构/)
  await drawer.locator('.ant-drawer-body').evaluate(element => { element.scrollTop = 0 })
  await page.screenshot({ path: info.outputPath('node-help-windows.png') })
  await selectNodeOption(page, '操作系统', 'Linux')
  await selectNodeOption(page, 'CPU 架构', 'aarch64')
  await expect(drawer.locator('#node-architecture-hint')).toContainText('lscpu')
  await expect(drawer.locator('#node-tool-hint')).toContainText('bin/obloader')
  await expect(drawer.locator('#node-tool-hint')).not.toContainText('bin\\windows')
  await expect(drawer.locator('#node-java-hint')).toContainText('aarch64 架构一致')
  await expect(drawer.locator('#node-roots-hint')).toContainText('路径区分大小写')
  await expect(drawer.getByLabel('工具专用 Java 8 路径', { exact: true })).toHaveAttribute('placeholder', '/usr/lib/jvm/java-8/bin/java')
  await expect(drawer.getByLabel('OB Loader/Dumper 安装目录', { exact: true })).toHaveValue('E:/synthetic-tools')
  await expect(drawer.getByLabel('工具专用 Java 8 路径', { exact: true })).toHaveValue('C:/synthetic-java/bin/java.exe')
  await expect(drawer.getByLabel('导出数据目录白名单', { exact: true })).toHaveValue('/E:/synthetic-exports')
  expect((await drawer.locator('.ant-form-item-extra').allTextContents()).join(' ')).not.toMatch(/麒麟|Kylin|Ubuntu|CentOS/i)
  const architectureGroup = drawer.getByRole('group', { name: 'CPU 架构', exact: true })
  await expect(architectureGroup).toHaveAttribute('aria-describedby', 'node-architecture-hint')
  await expect(architectureGroup).toHaveAccessibleDescription(/lscpu 核对 x86_64 或 aarch64/)
  await expect(architectureGroup.getByRole('combobox')).toHaveAttribute('id', 'node-cpu-architecture')
  await drawer.locator('.ant-drawer-body').evaluate(element => { element.scrollTop = 0 })
  await page.screenshot({ path: info.outputPath('node-help-linux.png') })
  await selectNodeOption(page, '操作系统', 'Windows')
  await expect(drawer.locator('#node-tool-hint')).toContainText('bin\\windows\\obloader.bat')
  await expect(drawer.locator('#node-java-hint')).toContainText('bin\\java.exe')
  await expect(architectureGroup.locator('.ant-select')).toContainText('x86_64')
  await expect(drawer.getByLabel('OB Loader/Dumper 安装目录', { exact: true })).toHaveValue('E:/synthetic-tools')
})

test('注册与编辑抽屉保留列表筛选，关闭恢复触发焦点', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  await mockNodes(page); await page.setViewportSize({ width: 1920, height: 1080 }); await page.goto('/nodes')
  await page.getByRole('textbox', { name: '搜索节点名称或标识' }).fill('合成节点 A')
  const register = page.getByRole('link', { name: '注册节点', exact: true }).first()
  await register.click()
  const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
  await expect(drawer.getByRole('heading', { name: '注册执行节点', exact: true })).toBeFocused()
  await expect(page.locator('.node-workspace')).toBeVisible()
  await expect(drawer.getByLabel('系统与 Agent 预留（GiB）')).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('node-registration-1920.png'), fullPage: true })
  await drawer.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes$/)
  await expect(register).toBeFocused()
  await expect(page.getByRole('textbox', { name: '搜索节点名称或标识' })).toHaveValue('合成节点 A')
  await page.getByRole('button', { name: '合成节点 A 的操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '编辑', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '编辑执行节点', exact: true }).getByLabel('节点名称', { exact: true })).toHaveValue('合成节点 A')
  await page.keyboard.press('Escape')
  await expect(page).toHaveURL(/\/nodes$/)
  expect(await page.evaluate(() => document.getElementById('app')?.inert)).toBe(false)
  await expect(page.getByRole('button', { name: '合成节点 A 的操作', exact: true })).toBeFocused()
  expect(errors).toEqual([])
})

test('未保存关闭和切换节点分别确认，取消保留输入与当前会话', async ({ page }) => {
  const state = await mockNodes(page); await page.goto('/nodes/A/edit')
  const drawer = page.getByRole('dialog', { name: '编辑执行节点', exact: true })
  await drawer.getByLabel('节点名称', { exact: true }).fill('合成未保存')
  await drawer.getByRole('button', { name: '取消', exact: true }).click()
  const confirmation = page.getByRole('dialog', { name: '放弃未保存的节点配置？' })
  await expect(confirmation.getByRole('button', { name: '继续编辑' })).toBeFocused()
  await confirmation.getByRole('button', { name: '继续编辑' }).click()
  await expect(page).toHaveURL(/\/nodes\/A\/edit$/)
  await expect(drawer.getByLabel('节点名称', { exact: true })).toHaveValue('合成未保存')
  await requestNavigation(page, '/nodes/B/edit')
  await expect(confirmation).toBeVisible()
  await confirmation.getByRole('button', { name: '放弃修改' }).click()
  await expect(page).toHaveURL(/\/nodes\/B\/edit$/)
  await expect(drawer.getByLabel('节点名称', { exact: true })).toHaveValue('合成节点 B')
  expect(state.writes).toHaveLength(0)
})

test('校验定位、预算 DTO 与创建后独立签发沿用原注册流程', async ({ page }) => {
  const state = await mockNodes(page); await page.goto('/nodes/new')
  const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
  await drawer.getByRole('button', { name: '保存并继续 Agent 关联' }).click()
  await expect(drawer.getByText('请检查以下字段')).toBeVisible()
  await drawer.getByRole('link', { name: /^节点名称：/ }).click()
  await expect(drawer.getByLabel('节点名称', { exact: true })).toBeFocused()
  expect(state.writes).toHaveLength(0)
  await fillNewNode(page)
  await drawer.getByRole('button', { name: '执行内存预算（可选）' }).click()
  await drawer.getByLabel('系统与 Agent 预留（GiB）').fill('2')
  await drawer.getByLabel('每任务堆外预算（GiB）').fill('1.25')
  await drawer.getByRole('button', { name: '保存并继续 Agent 关联' }).click()
  await expect(page).toHaveURL(/\/nodes\/C\?registration=created$/)
  expect(state.writes).toEqual([{
    displayName: '合成新节点', platform: 'WINDOWS_AMD64', allowedRoots: ['/E:/synthetic-exports'],
    toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java/bin/java.exe',
    budgetPolicy: { systemReservedBytes: 2147483648, taskOverheadBytes: 1342177280 },
  }])
  await expect(page.getByRole('group', { name: '节点操作', exact: true }).getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveAttribute('href', '/ob-data-orch-agent-windows-x86_64.zip')
  await expect(page.getByRole('link', { name: '下载 Agent 包', exact: true })).toHaveCount(1)
  expect(state.enrollments).toBe(0)
  await page.getByRole('button', { name: '生成一次性注册码' }).click()
  const enrollment = page.getByRole('dialog', { name: '一次性 Agent 注册码' })
  await expect(enrollment.getByLabel('一次性 Agent 注册码')).toHaveValue(/^obdo-r1\./)
  await enrollment.getByRole('button', { name: '关闭并清除' }).click()
  await expect(page.getByLabel('一次性 Agent 注册码')).toHaveCount(0)
})

test('服务端预算错误展开可选区并定位真实输入，失败保留草稿', async ({ page }) => {
  const state = await mockNodes(page); state.writeStatus = 422; await page.goto('/nodes/new')
  await fillNewNode(page)
  const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
  await drawer.getByRole('button', { name: '保存并继续 Agent 关联' }).click()
  await expect(drawer.getByLabel('系统与 Agent 预留（GiB）')).toBeVisible()
  await drawer.getByRole('link', { name: /^执行内存预算：/ }).click()
  await expect(drawer.getByLabel('系统与 Agent 预留（GiB）')).toBeFocused()
  await expect(drawer.getByLabel('节点名称', { exact: true })).toHaveValue('合成新节点')
  await expect(drawer.getByText('请输入导出数据目录白名单', { exact: true })).toHaveCount(0)
  await expect(page).toHaveURL(/\/nodes\/new$/)
})

test('保存入口防重与离开阻断，列表刷新不叠加写事务，成功后更新列表', async ({ page }) => {
  await page.clock.install()
  const state = await mockNodes(page); state.holdUpdates = true; await page.goto('/nodes/A/edit')
  const drawer = page.getByRole('dialog', { name: '编辑执行节点', exact: true })
  await drawer.getByLabel('节点名称', { exact: true }).fill('合成已修改')
  await drawer.getByRole('button', { name: '保存修改' }).click()
  await expect.poll(() => state.held.length).toBe(1)
  await expect(drawer.getByLabel('节点名称', { exact: true })).toBeDisabled()
  await expect(drawer.getByRole('button', { name: '取消', exact: true })).toBeDisabled()
  const reads = state.reads; await page.clock.runFor(180000); expect(state.reads).toBe(reads)
  await requestNavigation(page, '/nodes/B/edit'); await expect(page).toHaveURL(/\/nodes\/A\/edit$/)
  const updated = { ...node('A'), displayName: '合成已修改', revision: 2 }
  state.items[0] = updated
  await state.held[0]!.fulfill({ json: { item: updated } })
  await expect(page.locator('.ant-message').getByText('节点配置已保存。', { exact: true })).toBeVisible()
  await expect(drawer.getByText('节点配置已保存。', { exact: true })).toHaveCount(0)
  await expect(drawer.getByText('修改后由 Agent 在空闲时同步配置，无需重新注册。', { exact: true })).toBeVisible()
  await expect.poll(() => state.reads).toBe(reads + 1)
  await drawer.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes$/)
  await expect(page.getByRole('link', { name: '合成已修改', exact: true })).toBeVisible()
  expect(state.writes).toHaveLength(1)
})

test('列表授权失效立即清空打开的编辑配置与提交资格', async ({ page }) => {
  await page.clock.install()
  const state = await mockNodes(page); await page.goto('/nodes/A/edit')
  const drawer = page.getByRole('dialog', { name: '编辑执行节点', exact: true })
  await drawer.getByLabel('节点名称', { exact: true }).fill('合成未保存')
  state.listStatus = 403; await page.clock.runFor(60000)
  await expect(drawer.getByText('节点已不可访问', { exact: true })).toBeVisible()
  await expect(drawer.getByLabel('节点名称', { exact: true })).toHaveCount(0)
  await expect(drawer.getByRole('button', { name: '保存修改' })).toBeDisabled()
  await drawer.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page).toHaveURL(/\/nodes$/)
  await expect(page.getByText('当前身份无权访问执行节点')).toBeVisible()
  expect(state.writes).toHaveLength(0)
})

test('注册抽屉在桌面、窄屏与 200% 缩放下独立滚动且底部操作可达', async ({ page }, info) => {
  await mockNodes(page); await page.goto('/nodes/new')
  const drawer = page.getByRole('dialog', { name: '注册执行节点', exact: true })
  for (const [width, height] of [[1920, 1080], [1440, 1024], [1280, 720], [640, 720], [320, 640]]) {
    await page.setViewportSize({ width: width!, height: height! })
    await expect(drawer.getByRole('button', { name: '保存并继续 Agent 关联' })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), String(width)).toBe(true)
    await page.screenshot({ path: info.outputPath('node-registration-' + width + '.png') })
  }
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  await expect(drawer.getByRole('button', { name: '保存并继续 Agent 关联' })).toBeInViewport()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: info.outputPath('node-registration-200-percent.png') })
})
