// 只核对真实回环 HTTPS 的未登录边界，不创建会话、草稿或任务。
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { ApiClient } from '../normal-import-e2e/api-client.mjs'

const root = process.cwd()
const runDirectory = process.argv[2]
if (!/^artifacts[\\/]direct-load-e2e-[0-9]{8}-[a-f0-9]{12}$/.test(runDirectory ?? '')) throw new Error('RUN_SCOPE_REQUIRED')
const client = new ApiClient(await readFile(path.join(root, 'var/dev/control-plane/data/control-plane-ca.pem')))
const records = []
try {
  records.push({ id: 'HTTPS_HEALTH', status: await client.health(), expected: 200 })
  for (const [id, route, method, body] of [
    ['DIRECT_METADATA_AUTH', '/api/v1/direct-load-parameter-metadata', 'GET'],
    ['DIRECT_DRAFT_LIST_AUTH', '/api/v1/direct-load-drafts', 'GET'],
    ['DIRECT_CREATE_AUTH', '/api/v1/direct-load-drafts', 'POST', { configVersion: 'direct-load-v1', config: {} }],
    ['DIRECT_READ_AUTH', '/api/v1/direct-load-drafts/11111111-1111-4111-8111-111111111111', 'GET'],
    ['DIRECT_PRECHECK_AUTH', '/api/v1/direct-load-drafts/11111111-1111-4111-8111-111111111111:precheck', 'POST', {}],
    ['DIRECT_SUBMIT_AUTH', '/api/v1/direct-load-drafts/11111111-1111-4111-8111-111111111111:submit', 'POST', {}],
  ]) {
    const response = await client.request(route, method, body)
    records.push({ id, status: response.status, expected: 401, code: response.body.error?.code ?? response.body.code })
  }
  const passed = records.every(row => row.status === row.expected)
  await writeFile(path.join(root, runDirectory, 'auth-boundary.json'), JSON.stringify({ scope: 'LIVE_HTTPS_WITHOUT_AUTHENTICATION', passed, records, functionalTasksRun: 0 }, null, 2), { flag: 'wx' })
  console.log(JSON.stringify({ passed, checks: records.length, functionalTasksRun: 0 }))
  if (!passed) process.exitCode = 1
} catch {
  console.error('DIRECT_AUTH_BOUNDARY_UNAVAILABLE')
  process.exitCode = 1
} finally {
  await client.close()
}
