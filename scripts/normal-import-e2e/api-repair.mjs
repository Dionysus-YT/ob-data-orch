// 只修复已取证的三个失败夹具；保留首轮失败，任何断点都须先核对回执再续跑。
import { copyFile, mkdir } from 'node:fs/promises'
import { constants } from 'node:fs'
import path from 'node:path'
import { run } from './browser.mjs'

export async function runRepair(input, d) {
  const { client, save, work, readJSON, require, accepted, fixture, collect, parameterContracts } = d
  const original = await readJSON(path.join(work, 'verified-results.json'))
  require(original.counts.PASS === 25 && original.counts.FAIL === 3, 'REPAIR_INITIAL_STATE_CHANGED')
  require(original.records.filter(row => row.outcome === 'FAIL').map(row => row.id).sort().join(',') === 'NI-PARAM-compat,NI-PARAM-ignore,NI-PARAM-trail', 'REPAIR_FAILURE_SET_CHANGED')
  const source = await accepted(`/api/v1/data-sources/${input.sourceId}`)
  require(source.environment === 'TEST' && source.compatibilityMode === 'MYSQL' && source.state === 'ENABLED' && source.connectionKind === 'ODP', 'REPAIR_SOURCE_SCOPE_CHANGED')
  let node = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
  require(node.platform === 'WINDOWS_AMD64' && node.taskSummary.running === 0 && node.taskSummary.waiting === 0 && node.taskSummary.reconciling === 0, 'REPAIR_NODE_NOT_IDLE')
  const request = await accepted(`/api/v1/execution-nodes/${input.nodeId}:environment-check`, 'POST', undefined, node.revision)
  const checkId = request.environmentCheck?.checkId
  require(checkId, 'REPAIR_ENVIRONMENT_BINDING_MISSING')
  for (let n = 0; n < 80; n++) {
    node = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
    require(node.environmentCheck?.checkId === checkId, 'REPAIR_ENVIRONMENT_BINDING_CHANGED')
    if (node.environmentCheck.status === 'PASSED') break
    require(node.environmentCheck.status !== 'FAILED', 'REPAIR_ENVIRONMENT_FAILED')
    await new Promise(resolve => setTimeout(resolve, 1500))
  }
  require(node.environmentCheck.status === 'PASSED', 'REPAIR_ENVIRONMENT_TIMEOUT')
  await save(path.join(work, 'api-repair-readiness.json'), { nodeId: node.id, revision: node.revision, checkId, status: node.environmentCheck.status, factsRevision: node.environmentCheck.factsRevision })
  const archive = path.join(work, 'initial-failures')
  await mkdir(archive)
  for (const name of ['browser-parameters.json','verified-results.json','content-after.json','api-parameter-contracts.json']) await copyFile(path.join(work,name), path.join(archive,name), constants.COPYFILE_EXCL)
  const all = await readJSON(path.join(input.inputRoot, 'parameter-cases.json'))
  const positives = ['ignore','trail','compat'].map(key => structuredClone(all.find(row => row.id === `NI-PARAM-${key}`)))
  for (const row of positives.slice(0,2)) row.input.config.parser.lineSeparator = '\\n'
  positives[2].input.config.compatMysqlVersion = 'mysql80'
  const negatives = ['native','explicit_crlf'].map(key => {
    const row = structuredClone(positives[1]); row.id = `NI-PARAM-cut_${key}`; row.title = 'CUT LF 输入与 Windows 换行规则不符时阻断'; row.failureCheck = 'INPUT_MAPPING'
    row.names = ['--cut']; row.input.config.logPath += `-${key}`
    if (key === 'native') delete row.input.config.parser.lineSeparator
    else row.input.config.parser.lineSeparator = '\\r\\n'
    return row
  })
  const extras = [['fivesix','mysql56'],['fiveseven','mysql57']].map(([key,version]) => {
    const row = structuredClone(positives[2]); row.id = `NI-PARAM-compat_${key}`; row.input.config.compatMysqlVersion = version; row.input.config.replaceObject = true; row.input.config.logPath += `-${key}`; return row
  })
  const contracts = await parameterContracts(input)
  require(contracts.records.length === 72 && contracts.records.every(row => row.outcome === 'PASS'), 'REPAIR_API_CONTRACT_FAILED')
  const boundaries = []
  for (const version of [undefined, 'mysql']) {
    const config = { ...positives[2].input.config, dataSourceId: input.sourceId, nodeId: input.nodeId, database: input.database }
    delete config.compatMysqlVersion
    if (version) config.compatMysqlVersion = version
    const response = await client.request('/api/v1/normal-import-drafts', 'POST', { configVersion: 'normal-import-v1', config })
    const boundary = { name: version ? 'INVALID_MYSQL_VERSION' : 'LEGACY_VERSION_REQUIRED', createStatus: response.status, outcome: 'FAIL' }
    if (!version) {
      require(response.status === 201, 'LEGACY_DRAFT_NOT_EDITABLE')
      const draft = response.body
      for (const action of ['preview-command','precheck']) {
        const rejected = await client.request(`/api/v1/normal-import-drafts/${draft.id}:${action}`, 'POST', {}, draft.revision)
        require(rejected.status === 422, 'LEGACY_VERSION_EXECUTION_ALLOWED'); boundary[action] = rejected.status
      }
      require((await client.request(`/api/v1/normal-import-drafts/${draft.id}`, 'DELETE', {}, draft.revision)).status === 200, 'REPAIR_DRAFT_CLEANUP_FAILED')
    } else require([400,422].includes(response.status), 'INVALID_MYSQL_VERSION_ACCEPTED')
    boundary.outcome = 'PASS'; boundaries.push(boundary)
  }
  await save(path.join(work,'api-repair-contract-boundaries.json'), { records: boundaries })
  await save(path.join(work, 'repair-reset-receipt.json'), await fixture('import-parameters-repair', input))
  let reported = 0
  const state = await run({ ...input, parameterCases: [...negatives,...positives,...extras] }, { request: client.request.bind(client), publish: async value => {
    await save(path.join(work,'api-repair-batch.json'), value)
    while (reported < value.records.length) { const row = value.records[reported++]; console.log(`${row.id}: ${row.outcome}`) }
  } })
  require(state.done && !state.stopped && state.records.length === 7, 'REPAIR_BATCH_INCOMPLETE')
  await fixture('import-parameters-inspect', input, path.join(work,'content-after.json'))
  const old = await readJSON(path.join(archive,'browser-parameters.json'))
  for (const row of positives) {
    const repaired = state.records.find(value => value.id === row.id)
    require(repaired.outcome === 'CONTENT_PENDING', 'REPAIR_EXECUTION_FAILED')
    old.records[old.records.findIndex(value => value.id === row.id)] = repaired
  }
  await save(path.join(work,'browser-parameters.json'), old)
  await collect('collect-parameters.py')
  await collect('collect-repair.py')
  const verified = await readJSON(path.join(work,'verified-results.json'))
  require(verified.counts.PASS === 28 && !verified.counts.FAIL, 'REPAIR_CONTENT_NOT_VERIFIED')
  // 无任务的负例草稿只按本次回执和当前修订清理，不扫描其他业务数据。
  for (const row of state.records.filter(value => !value.taskId && value.draftId)) {
    const draft = await accepted(`/api/v1/normal-import-drafts/${row.draftId}`)
    require((await client.request(`/api/v1/normal-import-drafts/${draft.id}`, 'DELETE', {}, draft.revision)).status === 200, 'REPAIR_DRAFT_CLEANUP_FAILED')
  }
  console.log('失败三项已独立复验；MySQL 三个官方版本及 CUT 两个拒绝反例均已核对。')
}
