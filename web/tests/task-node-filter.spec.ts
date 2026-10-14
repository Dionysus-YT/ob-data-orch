import { expect, test, type Page, type Route } from '@playwright/test'

const time = '2026-01-01T00:00:00Z'
function task(id: string, nodeId: string) { return { id, type: 'OBDUMPER_EXPORT', dataSourceId: 'synthetic-source', objectSummary: `synthetic.${id}`, state: 'RUNNING', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: false, nodeId, ownedByCurrentUser: true, submittedAt: time, updatedAt: time, displayName: id, dataSourceDeleted: false, nodeDeleted: false, cancelRequested: false, availableActions: [] } }
async function navigate(page: Page, path: string) {
  await page.evaluate(async path => { const modulePath = '/src/router/index.ts'; const { router } = await import(/* @vite-ignore */ modulePath); await router.push(path) }, path)
}

test('同实例节点和范围切换隔离迟到请求；授权失败清空任务；卸载后忽略响应', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  // 故意忽略取消信号，证明生命周期代次校验独立于浏览器是否成功中止请求。
  await page.addInitScript(() => { const original = window.fetch.bind(window); window.fetch = (input, init) => original(input, { ...init, signal: undefined }) })
  const held: Route[] = []
  const seen: string[] = []
  let reject = false
  await page.route('**/api/v1/tasks?*', async route => {
    const url = new URL(route.request().url()); const node = url.searchParams.get('nodeId')!; seen.push(`${node}:${url.searchParams.get('scope')}`)
    if (node === 'A' || node === 'C') { held.push(route); return }
    if (reject) { await route.fulfill({ status: 403, json: { code: 'FORBIDDEN', message: '合成权限已撤销' } }); return }
    await route.fulfill({ json: { items: [task('task-B', 'B')], totalPages: 1 } })
  })
  await page.goto('/tasks?nodeId=A&scope=MINE')
  await expect.poll(() => held.length).toBe(1)
  await page.locator('.task-heading').evaluate(element => element.setAttribute('data-instance-evidence', 'same'))
  await navigate(page, '/tasks?nodeId=B&scope=ALL')
  await expect(page.getByRole('link', { name: 'task-B', exact: true })).toBeVisible()
  await expect(page.locator('.task-heading')).toHaveAttribute('data-instance-evidence', 'same')
  await held.shift()!.fulfill({ json: { items: [task('late-A', 'A')], totalPages: 2 } })
  await expect(page.getByRole('link', { name: 'late-A', exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'task-B', exact: true })).toBeVisible()
  expect(seen).toEqual(['A:MINE', 'B:ALL'])
  reject = true
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await expect(page.getByRole('link', { name: 'task-B', exact: true })).toHaveCount(0)
  await expect(page.locator('.ant-alert-warning')).toBeVisible()
  await navigate(page, '/tasks?nodeId=C&scope=MINE')
  await expect.poll(() => held.length).toBe(1)
  await navigate(page, '/imports/normal/new')
  await held.shift()!.fulfill({ json: { items: [task('late-C', 'C')], totalPages: 1 } })
  await expect(page.getByRole('heading', { name: '任务中心', exact: true })).toHaveCount(0)
  expect(errors).toEqual([])
})
