// 使用正常登录、Cookie 和 CSRF 验证现役 API；未满足预检查时只验证拒绝提交，不制造执行证明。
import { readFile, writeFile, rename } from 'node:fs/promises'
import { randomUUID } from 'node:crypto'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { ApiClient } from '../normal-import-e2e/api-client.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const runDirectory = process.argv[2]
if (process.argv.length !== 3 || !/^artifacts[\\/]direct-load-e2e-[0-9]{8}-[a-f0-9]{12}$/.test(runDirectory ?? '')) throw new Error('RUN_SCOPE_REQUIRED')
const work = path.join(root, runDirectory)
const owned = JSON.parse(await readFile(path.join(work, 'run.json'), 'utf8'))
if (!/^[a-f0-9]{32}$/.test(owned.runId) || !/^odo_e2e_[0-9]{8}_[a-f0-9]{12}$/.test(owned.database) || !owned.database.endsWith(owned.runId.slice(0, 12)) || !work.endsWith(owned.runId.slice(0, 12))) throw new Error('RUN_OWNER_INVALID')
// 新批次才可启动；失败后保留回执，通过单独核对决定后续动作，不能重放提交。
await writeFile(path.join(work, 'api-started.json'), JSON.stringify({ startedAt: new Date().toISOString() }), { flag: 'wx' })
const client = new ApiClient(await readFile(path.join(root, 'var/dev/control-plane/data/control-plane-ca.pem')))
const execute = promisify(execFile)
const state = { scope: 'LIVE_AUTHENTICATED_API', runId: owned.runId, records: [], drafts: [], checks: [], tasksSubmitted: 0, realImportVerified: false }
let createdFixture = false
let activeCheck = false
const requireFact = (value, code) => { if (!value) throw new Error(code) }
const safeCode = error => /^[A-Z_0-9]+$/.test(error?.message ?? '') ? error.message : 'DIRECT_API_TEST_UNAVAILABLE'
async function save(name, value) {
  const file = path.join(work, name), temporary = `${file}.${process.pid}.tmp`
  await writeFile(temporary, JSON.stringify(value, null, 2), { flag: 'wx' })
  await rename(temporary, file)
}
async function checkpoint() { await save('api-results.json', state) }
async function record(id, operation) {
  try { const facts = await operation(); state.records.push({ id, outcome: 'PASS', ...facts }) }
  catch (error) { state.records.push({ id, outcome: 'FAIL', code: safeCode(error) }) }
  await checkpoint()
}
async function request(route, method = 'GET', body, revision, key, csrf) {
  return client.request(route, method, body, revision, key, csrf)
}
async function accepted(route, method = 'GET', body, revision, key) {
  const response = await request(route, method, body, revision, key)
  requireFact(response.status >= 200 && response.status < 300, response.body.error?.code ?? response.body.code ?? 'API_REJECTED')
  return response.body
}
async function fixture(action, receipt = action) {
  let result
  try {
    result = await execute(path.join(root, 'artifacts/direct-load-e2e-tools/fixture.exe'), ['-action', action, '-database', owned.database, '-run-id', owned.runId, '-python', 'C:/Users/Dionysus/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'], { cwd: root, windowsHide: true, timeout: 120000, maxBuffer: 4 * 1024 * 1024 })
  } catch { throw new Error('DIRECT_FIXTURE_OPERATION_UNAVAILABLE') }
  const value = JSON.parse(result.stdout)
  if (action !== 'direct-probe') requireFact(value.ownerVerified === true, 'FIXTURE_OWNERSHIP_INVALID')
  await save(`${receipt}.json`, value)
  return value
}
const base = '/api/v1/direct-load-drafts'
const config = { dataSourceId: 'fe776904-5ed8-485d-8623-f53909b4d9fc', nodeId: '7ebf034b-7874-48e8-8222-9c030baa7446', database: owned.database, scenario: 'PRIVATE_ODP', rpcPort: 2885, table: 'dlvt_basic', filePath: path.join(root, 'ob-data', `dlvt-direct-${owned.runId.slice(0, 12)}`, 'dlvt_basic'), format: 'CSV', externalData: true, columnSeparator: '', thread: '', parallel: '', replaceData: false, truncateTable: false, applicability: { singleTable: true, wholeTableCommit: true, noCheckpoint: true, structureLimits: true } }
async function create(value = config, key) {
  const response = await request(base, 'POST', { configVersion: 'direct-load-v1', config: value }, undefined, key)
  if (response.status === 201) { state.drafts.push({ id: response.body.id, revision: response.body.revision }); await checkpoint() }
  requireFact(response.status === 201, response.body.error?.code ?? response.body.code ?? 'DRAFT_CREATE_REJECTED')
  return response.body
}
async function rejectConfig(id, patch, expected = [400, 422]) {
  await record(id, async () => {
    const response = await request(base, 'POST', { configVersion: 'direct-load-v1', config: { ...structuredClone(config), ...patch } })
    if (response.status === 201) { state.drafts.push({ id: response.body.id, revision: response.body.revision }); await checkpoint() }
    requireFact(expected.includes(response.status), 'INVALID_CONFIG_ACCEPTED')
    return { status: response.status, code: response.body.error?.code ?? response.body.code }
  })
}
async function check(draft, filesOnly, receipt) {
  const action = filesOnly ? 'discover-files' : 'precheck'
  const started = await accepted(`${base}/${draft.id}:${action}`, 'POST', {}, draft.revision)
  state.checks.push({ id: started.id, action }); activeCheck = true; await checkpoint()
  const until = Date.now() + 120000
  while (Date.now() < until) {
    const value = await accepted(`/api/v1/direct-load-prechecks/${started.id}`)
    if (!['PENDING', 'LEASED'].includes(value.status)) {
      activeCheck = false
      await save(`${receipt ?? action}.json`, value)
      requireFact(value.results.length === (filesOnly ? 1 : 9), 'CHECK_PROJECTION_INVALID')
      return value
    }
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error('CHECK_NOT_TERMINAL')
}
try {
  // 密码仅从匿名 stdin 接收；登录完成立即清零，不写入任何回执。
  const chunks = []; let size = 0
  try {
    for await (const chunk of process.stdin) { chunks.push(chunk); size += chunk.length; requireFact(size <= 1024, 'LOGIN_INPUT_INVALID') }
    const password = Buffer.concat(chunks)
    chunks.forEach(chunk => chunk.fill(0))
    await client.login(password)
  } finally { chunks.forEach(chunk => chunk.fill(0)) }
  state.authenticated = true; await checkpoint()
  console.log('API_LOGIN_SUCCEEDED')
  await record('METADATA', async () => {
    const value = await accepted('/api/v1/direct-load-parameter-metadata')
    requireFact(value.definitions?.length === 103 && value.metadataVersion === 'obloader-4.3.5-direct-v1' && value.capabilityVersion === 'direct-load-local-v1', 'METADATA_CONTRACT_INVALID')
    return { definitions: value.definitions.length }
  })
  await record('LIST', async () => { const value = await accepted(base); state.executionEnabled = value.executionEnabled; requireFact(Array.isArray(value.items), 'DRAFT_LIST_INVALID'); return { executionEnabled: value.executionEnabled } })
  const source = (await accepted(`/api/v1/data-sources/${config.dataSourceId}`)).item
  requireFact(source.environment === 'TEST' && source.state === 'ENABLED' && source.connectionKind === 'ODP', 'TEST_SOURCE_SCOPE_CHANGED')
  state.sourceReadiness = { hasCluster: Boolean(source.clusterName), hasTenant: Boolean(source.tenantName), kind: source.connectionKind, lastTestStatus: source.lastTestStatus }
  await record('CSRF_REQUIRED', async () => { const value = await request(base, 'POST', { configVersion: 'direct-load-v1', config }, undefined, randomUUID(), false); requireFact(value.status === 401, 'CSRF_NOT_ENFORCED'); return { status: value.status } })
  await record('UNKNOWN_OBJECT_HIDDEN', async () => { const value = await request(`${base}/${randomUUID()}`); requireFact(value.status === 404, 'UNKNOWN_OBJECT_NOT_HIDDEN'); return { status: value.status } })
  for (const [id, patch, expected] of [
    ['CLOUD_REJECTED', { scenario: 'CLOUD_ODP' }, [400, 404, 422]],
    ['OBSERVER_SOURCE_MISMATCH', { scenario: 'OBSERVER_DIRECT', rpcPort: 2882 }, [404]],
    ['SQL_REJECTED', { format: 'SQL' }], ['PARQUET_REJECTED', { format: 'PARQUET' }], ['ORC_REJECTED', { format: 'ORC' }],
    ['RPC_RANGE', { rpcPort: 65536 }], ['RPC_TYPE', { rpcPort: '2885' }], ['UNKNOWN_FIELD', { arbitraryCommand: 'synthetic' }],
    ['THREAD_TYPE', { thread: 2 }], ['THREAD_RANGE', { thread: '-1' }], ['PARALLEL_RANGE', { parallel: '0' }],
    ['TABLE_INJECTION', { table: 'dlvt_basic;synthetic' }],
  ]) await rejectConfig(id, patch, expected)
  const completeConfig = structuredClone(config)
  if (!state.sourceReadiness.hasCluster || !state.sourceReadiness.hasTenant) {
    await record('COMPLETE_CONFIG_SOURCE_BINDING', async () => {
      const value = await request(base, 'POST', { configVersion: 'direct-load-v1', config })
      if (value.status === 201) { state.drafts.push({ id: value.body.id, revision: value.body.revision }); await checkpoint() }
      requireFact(value.status === 422 && value.body.code === 'DIRECT_LOAD_CONFIG_INVALID', 'INCOMPLETE_SOURCE_BINDING_ACCEPTED')
      return { outcome: 'BLOCKED', status: value.status, code: value.body.code, reason: 'REGISTERED_ODP_CLUSTER_OR_TENANT_MISSING' }
    })
    // 未完成草稿验证继续进行，不能伪造集群名以获得完整配置资格。
    config.applicability = { singleTable: false, wholeTableCommit: false, noCheckpoint: false, structureLimits: false }
  }
  const key = randomUUID(), draft = await create(config, key)
  await record('CREATE_READ', async () => { const value = await accepted(`${base}/${draft.id}`); requireFact(value.id === draft.id && value.configFingerprint === draft.configFingerprint && value.rpcPortSource === 'MANUAL', 'DRAFT_READ_MISMATCH'); return {} })
  await record('CREATE_IDEMPOTENT', async () => { const value = await request(base, 'POST', { configVersion: 'direct-load-v1', config }, undefined, key); requireFact(value.status === 201 && value.body.id === draft.id, 'CREATE_REPLAY_INVALID'); return {} })
  await record('IDEMPOTENCY_CONFLICT', async () => { const value = await request(base, 'POST', { configVersion: 'direct-load-v1', config: { ...config, thread: '3' } }, undefined, key); requireFact(value.status === 409 && value.body.code === 'IDEMPOTENCY_CONFLICT', 'IDEMPOTENCY_CONFLICT_NOT_ENFORCED'); return { status: value.status, code: value.body.code } })
  await record('REVISION_REQUIRED', async () => { const value = await request(`${base}/${draft.id}`, 'PATCH', { configVersion: 'direct-load-v1', config }); requireFact(value.status === 428, 'REVISION_NOT_ENFORCED'); return { status: value.status } })
  await record('REVISION_STALE', async () => { const value = await request(`${base}/${draft.id}`, 'PATCH', { configVersion: 'direct-load-v1', config }, draft.revision + 99); requireFact(value.status === 412, 'REVISION_NOT_ENFORCED'); return { status: value.status } })
  await record('PREVIEW', async () => {
    if (!state.sourceReadiness.hasCluster || !state.sourceReadiness.hasTenant) {
      const value = await request(`${base}/${draft.id}:preview-command`, 'POST', {}, draft.revision)
      requireFact(value.status === 422 && value.body.code === 'DIRECT_LOAD_CONFIG_INVALID', 'INCOMPLETE_PREVIEW_ACCEPTED')
      return { outcome: 'BLOCKED', status: value.status, code: value.body.code, reason: 'REGISTERED_ODP_CLUSTER_OR_TENANT_MISSING' }
    }
    const value = await accepted(`${base}/${draft.id}:preview-command`, 'POST', {}, draft.revision); requireFact(value.configFingerprint === draft.configFingerprint && /(?:^|\s)--direct(?:\s|$)/.test(value.redactedCommand) && value.redactedCommand.includes('******') && !value.redactedCommand.includes('--cloud'), 'PREVIEW_INVALID'); return { fingerprintMatched: true, directFlag: true, maskedSecret: true }
  })
  await record('PATCH', async () => { const value = await accepted(`${base}/${draft.id}`, 'PATCH', { configVersion: 'direct-load-v1', config: { ...config, thread: '2', parallel: '2' } }, draft.revision); requireFact(value.revision > draft.revision && value.configFingerprint !== draft.configFingerprint, 'PATCH_BINDING_INVALID'); Object.assign(draft, value); Object.assign(state.drafts.find(item => item.id === draft.id), { revision: draft.revision }); return {} })
  await record('FIXTURE_CREATE', async () => { await fixture('direct-create'); createdFixture = true; return { ownerVerified: true } })
  if (createdFixture) {
    await fixture('direct-inspect', 'fixture-before'); await fixture('direct-probe')
    await record('FILE_DISCOVERY_API', async () => { const value = await check(draft, true); requireFact(value.status === 'SUCCEEDED' && value.results[0]?.status === 'PASSED' && value.inputManifest?.fileCount === 1, 'SINGLE_TABLE_FILES_UNVERIFIED'); return { status: value.status, results: value.results, inputFiles: value.inputManifest.fileCount } })
    const fixtureRoot = path.dirname(config.filePath)
    for (const [id, patch, expected, count] of [
      ['FILES_SHARDS', { table: 'dlvt_shards', filePath: path.join(fixtureRoot, 'dlvt_shards') }, 'SUCCEEDED', 2],
      ['FILES_SINGLE_FILE', { filePath: path.join(config.filePath, 'dlvt_basic.csv') }, 'SUCCEEDED', 1],
      ['FILES_MULTI_TABLE_REJECTED', { filePath: path.join(fixtureRoot, 'multi') }, 'FAILED'],
      ['FILES_EMPTY_REJECTED', { filePath: path.join(fixtureRoot, 'empty') }, 'FAILED'],
      ['FILES_MISSING_REJECTED', { filePath: path.join(fixtureRoot, 'absent') }, 'FAILED'],
      ['FILES_OUTSIDE_ROOT_REJECTED', { filePath: path.join(root, 'scripts', 'direct-load-e2e') }, 'FAILED'],
    ]) {
      if (activeCheck) break
      await record(id, async () => {
        const variant = await create({ ...config, ...patch })
        const value = await check(variant, true, id.toLowerCase())
        requireFact(value.status === expected && value.results[0]?.status === (expected === 'SUCCEEDED' ? 'PASSED' : 'FAILED'), 'FILE_DISCOVERY_EXPECTATION_MISMATCH')
        requireFact(count === undefined ? !value.inputManifest : value.inputManifest?.fileCount === count, 'FILE_MANIFEST_EXPECTATION_MISMATCH')
        return { status: value.status, results: value.results, inputFiles: value.inputManifest?.fileCount ?? null }
      })
    }
    if (!activeCheck) await record('PREFLIGHT_API', async () => {
      if (!state.sourceReadiness.hasCluster || !state.sourceReadiness.hasTenant) {
        const value = await request(`${base}/${draft.id}`, 'PATCH', { configVersion: 'direct-load-v1', config: completeConfig }, draft.revision)
        requireFact(value.status === 422, 'INCOMPLETE_SOURCE_BINDING_ACCEPTED')
        const rejected = await request(`${base}/${draft.id}:precheck`, 'POST', {}, draft.revision)
        requireFact(rejected.status === 422 && rejected.body.code === 'DIRECT_LOAD_CONFIG_INVALID', 'INCOMPLETE_PREFLIGHT_ACCEPTED')
        return { outcome: 'BLOCKED', status: rejected.status, code: rejected.body.code, reason: 'COMPLETE_CONFIGURATION_UNAVAILABLE' }
      }
      const value = await check(draft, false); state.precheck = { id: value.id, status: value.status, results: value.results }; return { status: value.status, results: value.results }
    })
    // 不伪造预检查事实；只有拒绝路径使用合成 precheckId，绝不提交可能成功的写入。
    await record('UNPROVEN_SUBMISSION_REJECTED', async () => { const value = await request(`${base}/${draft.id}:submit`, 'POST', { precheckId: randomUUID(), taskName: `direct-api-${owned.runId.slice(0, 12)}`, confirmedConfigFingerprint: draft.configFingerprint }, draft.revision); const code = value.body.error?.code ?? value.body.code; requireFact(value.status === 422 && ['DIRECT_LOAD_EXECUTION_GATED', 'PRECHECK_REQUIRED'].includes(code), 'UNPROVEN_SUBMISSION_ACCEPTED'); return { status: value.status, code } })
  }
  state.realExecutionOutcome = 'BLOCKED_PENDING_VALID_PREFLIGHT_AND_EXECUTION_ADMISSION'
} catch (error) { state.error = safeCode(error); process.exitCode = 1 }
finally {
  if (state.authenticated && !activeCheck) {
    for (const draft of state.drafts) {
      try { const response = await request(`${base}/${draft.id}`, 'DELETE', {}, draft.revision); draft.cleanupStatus = response.status; if (response.status !== 200) process.exitCode = 1 }
      catch { draft.cleanupCode = 'DRAFT_CLEANUP_UNAVAILABLE'; process.exitCode = 1 }
    }
  }
  if (createdFixture && !activeCheck) {
    try { await fixture('direct-inspect', 'fixture-after'); await fixture('direct-cleanup'); state.fixtureCleaned = true }
    catch { state.cleanupCode = 'FIXTURE_CLEANUP_UNAVAILABLE'; process.exitCode = 1 }
  }
  if (activeCheck) state.cleanupDeferred = 'ACTIVE_CHECK_REQUIRES_TERMINAL_RECONCILIATION'
  await client.close()
  state.finishedAt = new Date().toISOString()
  state.apiPassed = !state.error && state.records.every(row => row.outcome === 'PASS') && state.drafts.every(row => row.cleanupStatus === 200)
  state.failedChecks = state.records.filter(row => row.outcome === 'FAIL').length
  state.blockedChecks = state.records.filter(row => row.outcome === 'BLOCKED').length
  if (!state.apiPassed) process.exitCode = 1
  await checkpoint()
  console.log(JSON.stringify({ apiPassed: state.apiPassed, records: state.records.length, tasksSubmitted: state.tasksSubmitted, realImportVerified: false, error: state.error }))
}
