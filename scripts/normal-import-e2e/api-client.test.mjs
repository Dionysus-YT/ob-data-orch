import test from 'node:test'
import assert from 'node:assert/strict'
import { ApiClient, encodeLogin } from './api-client.mjs'
import { run } from './browser.mjs'
import { lifecycle } from './lifecycle.mjs'

test('登录表单按 UTF8 编码保留特殊字符且缓冲区可清零', () => {
  const password = Buffer.from('synthetic &=+%中文')
  const form = encodeLogin(password)
  assert.equal(new URLSearchParams(form.toString()).get('password'), password.toString())
  password.fill(0)
  form.fill(0)
  assert.ok(password.every(value => value === 0))
  assert.ok(form.every(value => value === 0))
})

test('没有 CA 或密码时失败关闭', async () => {
  assert.throws(() => new ApiClient(Buffer.alloc(0)), /TLS_CA_REQUIRED/)
  assert.throws(() => encodeLogin(Buffer.alloc(0)), /LOGIN_INPUT_INVALID/)
  const client = new ApiClient(Buffer.from('synthetic-ca'))
  try {
    const password = Buffer.alloc(1025, 65)
    await assert.rejects(client.login(password), /LOGIN_INPUT_INVALID/)
    assert.ok(password.every(value => value === 0))
  } finally { await client.close() }
})

test('API 传输不能改写为其他来源或机器认证入口', async () => {
  const client = new ApiClient(Buffer.from('synthetic-ca'))
  try {
    for (const path of ['https://synthetic.invalid/', '//synthetic.invalid/', '/agent/v1/executions']) {
      await assert.rejects(client.request(path), /API_SCOPE_INVALID/)
    }
  } finally { await client.close() }
})

test('原执行用例可由直接 API 传输驱动且不依赖浏览器全局对象', async () => {
  const requests = []
  const input = { runId: 'a'.repeat(32), inputRoot: 'E:\\synthetic', database: 'synthetic_db', sourceId: 'synthetic-source', nodeId: 'synthetic-node', ids: ['NI-E2E-09'] }
  const runtime = { request: async (...args) => { requests.push(args); return { status: 400, body: { code: 'REQUEST_INVALID' } } } }
  const state = await run(input, runtime)
  assert.equal(state.done, true)
  assert.equal(state.records.length, 1)
  assert.equal(state.records[0].outcome, 'PASS')
  assert.equal(requests[0][0], '/api/v1/normal-import-drafts')
  assert.equal(requests[0][1], 'POST')
  assert.equal(requests[0][2].config.direct, true)
})

test('生命周期用例也由直接 API 传输驱动', async () => {
  const requests = []
  const state = await lifecycle({ runId: 'a'.repeat(32), ids: ['NI-E2E-16'], successTaskId: 'synthetic-task' }, {
    request: async (...args) => { requests.push(args); return { status: 422, body: { code: 'TASK_ACTION_REJECTED' } } },
  })
  assert.equal(state.done, true)
  assert.equal(state.records[0].outcome, 'PASS')
  assert.equal(requests[0][0], '/api/v1/tasks/synthetic-task:rebuild-draft')
  assert.equal(requests[0][5], true)
})
