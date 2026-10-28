import assert from 'node:assert/strict'
import test from 'node:test'
import { waitForDevelopmentHealth } from '../dev-readiness.mjs'

function fixture(overrides = {}) {
  let elapsed = 0
  return {
    now: () => elapsed,
    sleep: async ms => { elapsed += ms },
    exited: () => null,
    probe: async () => { if (elapsed < 30000) throw new Error('健康连接失败（ECONNREFUSED）') },
    ...overrides,
  }
}

test('初始化超过旧的十秒窗口仍可就绪', async () => {
  const options = fixture()
  await waitForDevelopmentHealth(options)
  assert.equal(options.now(), 30000)
})

test('持续失败在总时限退出并说明取消可能来自清理', async () => {
  const options = fixture({ probe: async () => { throw new Error('健康响应无效（HTTP 503）') } })
  await assert.rejects(waitForDevelopmentHealth(options), /120 秒.*HTTP 503.*context canceled/)
  assert.equal(options.now(), 120000)
})

test('单次检查预算受剩余总时限约束', async () => {
  const budgets = []
  const options = fixture({ timeoutMs: 600, probe: async budget => { budgets.push(budget); throw new Error('健康检查超时') } })
  await assert.rejects(waitForDevelopmentHealth(options), /健康检查超时/)
  assert.deepEqual(budgets, [600, 350, 100])
  assert.equal(options.now(), 600)
})

test('子进程提前退出或用户取消立即终止等待', async () => {
  for (const exit of [1, 0, 'SIGTERM']) {
    await assert.rejects(waitForDevelopmentHealth(fixture({ exited: () => exit })), /就绪前退出/)
  }
  await assert.rejects(waitForDevelopmentHealth(fixture({ canceled: () => true })), /启动已取消/)
})
