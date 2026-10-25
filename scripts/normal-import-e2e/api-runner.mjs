// 只对现有本机 HTTPS 运行普通业务 API；全部连接与写入范围来自本轮所有权日记。
import { readFile, writeFile, mkdir, rename } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFile as executeFile } from 'node:child_process'
import { promisify } from 'node:util'
import { ApiClient } from './api-client.mjs'
import { run } from './browser.mjs'
import { lifecycle } from './lifecycle.mjs'
import { runGates } from './api-gates.mjs'
import { runRepair } from './api-repair.mjs'
import { closeout } from './api-closeout.mjs'
import { runRemaining } from './api-remaining.mjs'
import { runCapabilities, runCapabilityNegatives, runCapabilityRetry } from './api-capabilities.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const work = path.join(root, 'artifacts/normal-import-full-20261020')
const retest = path.join(root, 'artifacts/normal-import-retest-20261019')
const args = process.argv.slice(2)
if (args.length !== 1 || !['--auth-check', '--execute','--gates','--repair','--closeout','--remaining','--remaining-closeout','--capabilities','--capabilities-closeout','--capabilities-negatives','--capabilities-retry'].includes(args[0])) throw new Error('API_RUN_MODE_REQUIRED')
const readJSON = async file => JSON.parse((await readFile(file, 'utf8')).replace(/^\uFEFF/, ''))
async function save(file, value) {
  const temporary = `${file}.${process.pid}.tmp`
  await writeFile(temporary, JSON.stringify(value, null, 2), { flag: 'wx', mode: 0o600 })
  await rename(temporary, file)
}
const require = (condition, code) => { if (!condition) throw new Error(code) }
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
let client
const execFile = promisify(executeFile)
const python = 'C:/Users/Dionysus/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'

async function fixture(action, input, destination) {
  let result
  try {
    result = await execFile(path.join(work, action === 'import-parameters-repair' ? 'fixture-repair.exe' : 'fixture.exe'), ['-action', action, '-database', input.database, '-run-id', input.runId, '-python', python], { cwd: root, windowsHide: true, maxBuffer: 4 * 1024 * 1024 })
  } catch { throw new Error('INDEPENDENT_FIXTURE_READ_UNAVAILABLE') }
  const content = JSON.parse(result.stdout)
  require(content.ownerVerified === true, 'FIXTURE_OWNERSHIP_INVALID')
  if (destination) await save(destination, content)
  return content
}

async function collect(script, extra = []) {
  try {
    const result = await execFile(python, [path.join(root,'scripts/normal-import-e2e',script), ...extra], { cwd: root, windowsHide: true, maxBuffer: 4 * 1024 * 1024 })
    console.log(result.stdout.trim())
  } catch { throw new Error('INDEPENDENT_RESULT_COLLECTION_FAILED') }
}

async function accepted(route, method, body, revision) {
  const response = await client.request(route, method, body, revision)
  require(response.status >= 200 && response.status < 300, response.body.error?.code ?? response.body.code ?? 'API_REJECTED')
  return response.body.item ?? response.body
}

// 每个定义分别验证真实 API 的活动参数、未知/类型错误输入和所有者草稿清理，不启动任务。
async function parameterContracts(input) {
  const matrix = await readJSON(path.join(root, 'internal/importdomain/testdata/normal-parameter-matrix.json'))
  const fields = {
    host:['host'], user:['user'], database:['database'], port:['port'], password:['password'], 'no-sys':['noSys'], yes:['yes'],
    'file-path':['filePath'], 'log-path':['logPath'], 'ctl-path':['mapping','ctlPath'], 'file-suffix':['fileSuffix'], 'file-encoding':['fileEncoding'], 'file-regular-expression':['fileRegex'],
    all:['scope'], 'exclude-table':['excludeTables'], ddl:['content'], csv:['format'], cut:['format'], sql:['format'],
    'include-column-names':['mapping','includeColumns'], 'exclude-column-names':['mapping','excludeColumns'], 'exclude-data-types':['mapping','excludeDataTypes'],
    'column-quote':['parser','columnQuote'], 'column-separator':['parser','columnSeparator'], 'column-splitter':['parser','columnSplitter'], 'line-separator':['parser','lineSeparator'], 'escape-character':['parser','escapeCharacter'], 'null-string':['parser','nullString'], 'empty-string':['parser','emptyString'],
    'compression-algo':['parser','compressionAlgo'], 'compression-level':['parser','compressionLevel'], 'date-format':['parser','dateFormat'], 'default-date':['parser','defaultDate'], 'skip-header':['parser','skipHeader'], 'skip-footer':['parser','skipFooter'], 'ignore-escape':['parser','ignoreEscape'], 'trail-delimiter':['parser','trailDelimiter'], 'with-trim':['parser','withTrim'], compress:['parser','compress'], 'auto-column-mapping':['mapping','kind'],
    'max-discards':['execution','maxDiscards'], 'max-errors':['execution','maxErrors'], thread:['execution','thread'], batch:['execution','batch'], 'block-size':['execution','blockSize'], 'max-tps':['execution','maxTps'], 'max-wait-timeout':['execution','maxWaitTimeout'], strict:['execution','strict'], rw:['execution','rw'], mem:['execution','memory'],
    'nls-date-format':['execution','nlsDateFormat'], 'nls-timestamp-format':['execution','nlsTimestampFormat'], 'nls-timestamp-tz-format':['execution','nlsTimestampTzFormat'], 'compat-mode':['compatMysql'], 'replace-data':['replaceData'], 'replace-object':['replaceObject'], 'truncate-table':['cleanup'], 'delete-from-table':['cleanup'],
  }
  for (const name of ['table','table-group','view','trigger','obj-user','role','sequence','synonym','type','type-body','package','package-body','function','procedure']) fields[name] = ['objects',name]
  const state = { done: false, scope: 'REAL_API_CONTRACT_ONLY', records: [], startedAt: new Date().toISOString() }
  const output = path.join(work, 'api-parameter-contracts.json')
  for (const item of matrix.parameters) {
    const name = item.name.slice(2)
    const record = { name: item.name, mode: item.mode, outcome: 'FAIL' }
    let draft
    try {
      // 合成数据库名只用于 Oracle 预览，不触发探针或用户对象写入。
      const config = { dataSourceId: item.mode === 'ORACLE' ? '55208ffb-2171-4e69-bb97-5af53c2f0d81' : input.sourceId, nodeId: input.nodeId,
        database: item.mode === 'ORACLE' ? 'NIVT_SYNTHETIC_SCHEMA' : input.database, filePath: `${input.inputRoot}\\quoted`, content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', parser: {}, mapping: { kind: 'POSITION' }, execution: {}, cleanup: 'NONE', ...structuredClone(item.patch) }
      // 领域测试使用合成绝对路径；真实 API 预览也绑定本轮受控目录，不复用其他路径。
      if (config.logPath) config.logPath = `${input.inputRoot}\\logs-api-contract`
      if (config.mapping?.ctlPath) config.mapping.ctlPath = `${input.inputRoot}\\synthetic.ctl`
      const created = await client.request('/api/v1/normal-import-drafts', 'POST', { configVersion: 'normal-import-v1', config })
      require(created.status === 201, 'API_PARAMETER_CONFIG_REJECTED')
      draft = created.body
      record.draftId = draft.id
      await save(output, { ...state, current: record })
      const preview = await accepted(`/api/v1/normal-import-drafts/${draft.id}:preview-command`, 'POST', {}, draft.revision)
      const expected = item.name === '--password' ? '-p ******' : item.name === '--strict' ? '--strict=false' : item.name
      require(preview.configFingerprint === draft.configFingerprint && preview.redactedCommand.includes(expected) && preview.redactedCommand.includes('******'), 'API_PARAMETER_PROJECTION_MISSING')
      record.previewMatched = true
      const invalid = structuredClone(config)
      const field = fields[name]
      require(field, 'API_PARAMETER_TEST_FIELD_MISSING')
      if (field.length === 1) invalid[field[0]] = ['invalid-type']
      else { invalid[field[0]] ??= {}; invalid[field[0]][field[1]] = ['invalid-type'] }
      const rejected = await client.request('/api/v1/normal-import-drafts', 'POST', { configVersion: 'normal-import-v1', config: invalid })
      if (rejected.status === 201) {
        // 错误输入意外保存时也只清理本次回执的对象，并保持该参数失败。
        const bad = rejected.body
        await client.request(`/api/v1/normal-import-drafts/${bad.id}`, 'DELETE', {}, bad.revision)
      }
      require([400,422].includes(rejected.status), 'API_PARAMETER_INVALID_TYPE_ACCEPTED')
      Object.assign(record, { outcome: 'PASS', rejectionStatus: rejected.status, rejectionCode: rejected.body.error?.code ?? rejected.body.code })
    } catch (error) { record.code = /^[A-Z_0-9]+$/.test(error.message) ? error.message : 'API_PARAMETER_TEST_UNAVAILABLE' }
    finally {
      if (draft) {
        const deleted = await client.request(`/api/v1/normal-import-drafts/${draft.id}`, 'DELETE', {}, draft.revision)
        record.cleanupStatus = deleted.status
        if (![200,204].includes(deleted.status)) { record.outcome = 'FAIL'; record.code = 'API_DRAFT_CLEANUP_UNCONFIRMED' }
      }
    }
    state.records.push(record)
    await save(output, state)
    console.log(`${item.name}: ${record.outcome}`)
  }
  state.done = true
  state.finishedAt = new Date().toISOString()
  await save(output, state)
  return state
}

async function main() {
  await mkdir(work, { recursive: true })
  client = new ApiClient(await readFile(path.join(root, 'var/dev/control-plane/data/control-plane-ca.pem')))
  if (args[0] === '--auth-check') {
    const checks = [{ name: 'HTTPS_HEALTH', status: await client.health(), expected: 200 }]
    for (const [name, route, method, body] of [
      ['SESSION_AUTH', '/api/v1/session', 'GET'], ['DRAFT_READ_AUTH', '/api/v1/normal-import-drafts', 'GET'],
      ['DRAFT_WRITE_AUTH', '/api/v1/normal-import-drafts', 'POST', { configVersion: 'normal-import-v1', config: {} }],
    ]) { const result = await client.request(route, method, body); checks.push({ name, status: result.status, expected: 401, code: result.body.error?.code ?? result.body.code }) }
    await save(path.join(work, 'api-auth-boundary.json'), { scope: 'LIVE_HTTPS_NO_SESSION', checks, passed: checks.every(row => row.status === row.expected), functionalParameterTestsRun: false })
    console.log(JSON.stringify({ checks, functionalParameterTestsRun: false }))
    return
  }
  const chunks = []
  let length = 0
  for await (const chunk of process.stdin) { length += chunk.length; require(length <= 1024, 'LOGIN_INPUT_INVALID'); chunks.push(chunk) }
  const password = Buffer.concat(chunks)
  chunks.forEach(chunk => chunk.fill(0))
  await client.login(password)
  console.log('API 登录成功；会话和 CSRF 仅保留在本进程内存。')
  if(args[0]==='--capabilities') { await runCapabilities({ client, root, save, accepted, readJSON }); return }
  if(args[0]==='--capabilities-negatives') { await runCapabilityNegatives({ client, root, save, accepted, readJSON }); return }
  if(args[0]==='--capabilities-retry') { await runCapabilityRetry({ client, root, save, accepted, readJSON }); return }
  if(args[0]==='--capabilities-closeout') {
    const work=path.join(root,'artifacts/normal-import-capabilities-20261020'),owned=await readJSON(path.join(work,'run.json'))
    const oracleSource=await accepted(`/api/v1/data-sources/${owned.oracleSourceId}`)
    owned.oracleDatabase=oracleSource.defaultDatabase||oracleSource.username.toUpperCase()
    const journals=await Promise.all(['mysql-api.json','oracle-api.json','retry-mysql-api.json','retry-oracle-api.json','negative-api.json','negative-retry-api.json'].map(name=>readJSON(path.join(work,name))))
    await closeout(owned,{client,accepted,save,work,readJSON,require,journals,refreshEnvironment:true});return
  }
  if(args[0]==='--remaining') { await runRemaining({ client, root, save, accepted, readJSON }); return }
  if(args[0]==='--remaining-closeout') {
    const work = path.join(root,'artifacts/normal-import-remaining-20261020')
    const owned = await readJSON(path.join(work,'run.json'))
    require(/^[a-f0-9]{32}$/.test(owned.runId) && owned.inputRoot === path.join(root,'ob-data',`nivt-normal-20261019-${owned.runId.slice(0,12)}`),'CLOSEOUT_OWNER_INVALID')
    await closeout(owned,{ client, accepted, save, work, readJSON, require, journals:[await readJSON(path.join(work,'oracle-api.json'))] }); return
  }
  const input = await readJSON(path.join(retest, 'run.json'))
  const expectedRoot = path.join(root, 'ob-data', `nivt-normal-20261019-${input.runId.slice(0,12)}`)
  require(/^[a-f0-9]{32}$/.test(input.runId) && input.inputRoot === expectedRoot && input.database.endsWith(input.runId.slice(0,12)), 'FIXTURE_OWNERSHIP_INVALID')
  if(args[0]==='--gates') { await runGates(input,client,save,work); return }
  if(args[0]==='--repair') { await runRepair(input, { client, save, work, readJSON, require, accepted, fixture, collect, parameterContracts }); return }
  if(args[0]==='--closeout') { await closeout(input, { client, accepted, save, work, readJSON, require }); return }
  // 回执存在就停止，避免重复提交已写入的固定夹具；断点续跑须先逐项核对历史终态。
  for (const file of [path.join(retest,'browser-initial.json'), path.join(work,'browser-parameters.json')]) {
    try { await readFile(file); throw new Error('EXISTING_API_BATCH_REQUIRES_RECONCILIATION') }
    catch (error) { if (error.code !== 'ENOENT') throw error }
  }
  for (const [id, mode] of [[input.sourceId,'MYSQL'],['55208ffb-2171-4e69-bb97-5af53c2f0d81','ORACLE']]) {
    const source = await accepted(`/api/v1/data-sources/${id}`)
    require(source.environment === 'TEST' && source.connectionKind === 'ODP' && source.compatibilityMode === mode && source.state === 'ENABLED', 'DATA_SOURCE_OUTSIDE_TEST_SCOPE')
  }
  const seeded = await fixture('import-parameters-inspect', input)
  require(seeded.records.length === 28 && seeded.records.every(row => row.beforeMatched) && seeded.excludedTableUnchanged, 'FIXTURE_SEED_STATE_CHANGED')
  const contracts = await parameterContracts(input)
  require(contracts.done && contracts.records.length === 72 && contracts.records.every(row => row.outcome === 'PASS'), 'API_PARAMETER_CONTRACT_FAILED')
  const before = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
  require(before.platform === 'WINDOWS_AMD64', 'NODE_PLATFORM_MISMATCH')
  require(before.taskSummary && before.taskSummary.running === 0 && before.taskSummary.waiting === 0 && before.taskSummary.reconciling === 0, 'NODE_HAS_EXISTING_TASKS')
  const requested = await accepted(`/api/v1/execution-nodes/${input.nodeId}:environment-check`, 'POST', undefined, before.revision)
  const checkId = requested.environmentCheck?.checkId
  require(checkId, 'NODE_ENVIRONMENT_BINDING_MISSING')
  const until = Date.now() + 120000
  let node
  while (Date.now() < until) {
    node = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
    require(node.environmentCheck?.checkId === checkId, 'NODE_ENVIRONMENT_BINDING_CHANGED')
    if (node.environmentCheck?.status === 'PASSED') break
    require(node.environmentCheck?.status !== 'FAILED', 'NODE_ENVIRONMENT_CHECK_FAILED')
    await sleep(1500)
  }
  require(node?.environmentCheck?.status === 'PASSED', 'NODE_ENVIRONMENT_CHECK_TIMEOUT')
  await save(path.join(work, 'api-node-readiness.json'), { nodeId: input.nodeId, revision: node.revision, environmentStatus: node.environmentCheck.status, factsRevision: node.environmentCheck.factsRevision })
  const runtime = file => {
    let reported = 0
    return { request: client.request.bind(client), publish: async state => {
      await save(file, state)
      while (reported < state.records.length) { const row = state.records[reported++]; console.log(`${row.id}: ${row.outcome}`) }
    } }
  }
  const initial = await run(input, runtime(path.join(retest,'browser-initial.json')))
  require(initial.done && !initial.stopped && initial.records.every(row => ['PASS','CONTENT_PENDING','FAILURE_CONTENT_PENDING'].includes(row.outcome)), 'REAL_RETEST_BATCH_FAILED')
  const success = initial.records.find(row => row.id === 'NI-E2E-13')
  const failure = initial.records.find(row => row.id === 'NI-E2E-14')
  const life = await lifecycle({ ...input, successTaskId: success?.taskId, failedTaskId: failure?.taskId, oldPrecheckId: success?.precheckId }, runtime(path.join(retest,'lifecycle-initial.json')))
  await save(path.join(retest,'supplemental.json'), { discardLimitFailure: initial.records.find(row => row.id === 'NI-E2E-20')?.duplicateLog === true })
  await fixture('import-inspect', input, path.join(retest,'content.json'))
  await collect('collect.py', ['normal-import-retest-20261019'])
  require(life.done && life.records.every(row => row.outcome === 'PASS'), 'REAL_LIFECYCLE_BATCH_FAILED')
  const verifiedRetest = await readJSON(path.join(retest,'verified-results.json'))
  require(verifiedRetest.records.length === 23 && verifiedRetest.records.every(row => row.outcome === 'PASS'), 'REAL_RETEST_CONTENT_FAILED')
  const parameterCases = await readJSON(path.join(input.inputRoot,'parameter-cases.json'))
  const parameters = await run({ ...input, parameterCases }, runtime(path.join(work,'browser-parameters.json')))
  await fixture('import-parameters-inspect', input, path.join(work,'content-after.json'))
  require(parameters.done && !parameters.stopped && parameters.records.length === 28, 'REAL_PARAMETER_BATCH_INCOMPLETE')
  await collect('collect-parameters.py')
  const verifiedParameters = await readJSON(path.join(work,'verified-results.json'))
  require(verifiedParameters.records.length === 28 && verifiedParameters.records.every(row => row.outcome === 'PASS'), 'REAL_PARAMETER_BATCH_FAILED')
  await save(path.join(work,'combined-real-results.json'), { runId: input.runId, records: [...verifiedRetest.records, ...verifiedParameters.records] })
  console.log('API 批次、独立完整行集/结构、持久事件与秘密审计均通过；未执行能力仍保留阻断。')
}

try { await main() }
catch (error) { console.error(/^[A-Z_0-9]+$/.test(error.message) ? error.message : 'API_TEST_UNAVAILABLE'); process.exitCode = 1 }
finally { await client?.close() }
