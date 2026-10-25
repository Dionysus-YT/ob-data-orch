// 当前批次只使用已授权 TEST 双数据源；实际写入限于独立所有权夹具。
import { mkdir, writeFile, readFile } from 'node:fs/promises'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import path from 'node:path'
import { run } from './browser.mjs'
import { runGates } from './api-gates.mjs'

const execute = promisify(execFile)
const python = 'C:/Users/Dionysus/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'
const require = (value, code) => { if (!value) throw new Error(code) }

export async function runRemaining({ client, root, save, accepted, readJSON }) {
  const work = path.join(root,'artifacts/normal-import-remaining-20261020')
  const input = await readJSON(path.join(work,'run.json'))
  require(/^[a-f0-9]{32}$/.test(input.runId) && input.inputRoot === path.join(root,'ob-data',`nivt-normal-20261019-${input.runId.slice(0,12)}`) && input.database === `odo_e2e_20261020_${input.runId.slice(0,12)}`, 'REMAINING_OWNER_SCOPE_INVALID')
  for (const name of ['oracle-api.json','api-capability-prechecks.json']) {
    try { await readFile(path.join(work,name)); throw new Error('REMAINING_EXISTING_BATCH_REQUIRES_RECONCILIATION') } catch (error) { if(error.code !== 'ENOENT') throw error }
  }
  const mysql = await accepted(`/api/v1/data-sources/${input.sourceId}`)
  const oracle = await accepted(`/api/v1/data-sources/${input.oracleSourceId}`)
  for (const [source, mode] of [[mysql,'MYSQL'],[oracle,'ORACLE']]) require(source.environment==='TEST' && source.connectionKind==='ODP' && source.compatibilityMode===mode && source.state==='ENABLED','REMAINING_SOURCE_NOT_TEST')
  const schema = oracle.defaultDatabase || oracle.username?.toUpperCase()
  require(schema && /^[A-Za-z_][A-Za-z0-9_$#]{0,127}$/.test(schema),'REMAINING_SCHEMA_UNAVAILABLE')
  let node = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
  require(node.taskSummary?.running===0 && node.taskSummary?.waiting===0 && node.taskSummary?.reconciling===0,'REMAINING_NODE_NOT_IDLE')
  const requested = await accepted(`/api/v1/execution-nodes/${input.nodeId}:environment-check`,'POST',undefined,node.revision)
  for(let count=0;count<80;count++) {
    node=await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
    require(node.environmentCheck?.checkId===requested.environmentCheck?.checkId,'REMAINING_CHECK_BINDING_CHANGED')
    if(node.environmentCheck?.status==='PASSED') break
    require(node.environmentCheck?.status!=='FAILED','REMAINING_NODE_ENVIRONMENT_FAILED')
    await new Promise(resolve=>setTimeout(resolve,1500))
  }
  require(node.environmentCheck?.status==='PASSED','REMAINING_NODE_ENVIRONMENT_TIMEOUT')
  await save(path.join(work,'node-readiness.json'),{revision:node.revision,factsRevision:node.environmentCheck.factsRevision,status:node.environmentCheck.status})
  const fixture = async action => {
    let result
    try { result=await execute(path.join(work,'fixture.exe'),['-action',action,'-database',input.database,'-run-id',input.runId,'-python',python],{cwd:root,windowsHide:true,maxBuffer:1024*1024}) }
    catch { throw new Error('REMAINING_FIXED_FIXTURE_UNAVAILABLE') }
    const receipt=JSON.parse(result.stdout)
    await save(path.join(work,`${action}.json`),receipt)
    return receipt
  }
  // MySQL 使用旧固定创建器，但本轮是全新独立库/目录，不重放已清理的 run。
  require((await fixture('import-create')).status==='CREATED','REMAINING_MYSQL_OWNER_CREATE_FAILED')
  await mkdir(path.join(input.inputRoot,'parameters'))
  const scenarios = [
    ['nls_date','DATE','nlsDateFormat','--nls-date-format','DD/MM/YYYY HH24:MI:SS','20/10/2026 12:34:56'],
    ['nls_timestamp','TIMESTAMP','nlsTimestampFormat','--nls-timestamp-format','DD/MM/YYYY HH24:MI:SS.FF6','20/10/2026 12:34:56.123456'],
    ['nls_timezone','TZ','nlsTimestampTzFormat','--nls-timestamp-tz-format','DD/MM/YYYY HH24:MI:SS.FF6 TZH:TZM','20/10/2026 12:34:56.123456 +08:00'],
  ]
  const cases=[]
  for(const [key,suffix,field,name,format,value] of scenarios) {
    const table=`NIVR_${input.runId.slice(0,12).toUpperCase()}_${suffix}`
    const directory=path.join(input.inputRoot,'parameters',key)
    await mkdir(directory)
    await writeFile(path.join(directory,table+'.csv'),`"1","${value}"\n`,{flag:'wx'})
    cases.push({id:'NI-PARAM-'+key,title:'Oracle NLS 独立结果验证',names:[name],input:{configVersion:'normal-import-v1',config:{dataSourceId:input.oracleSourceId,nodeId:input.nodeId,database:schema,filePath:directory,logPath:path.join(input.inputRoot,'logs-parameter-'+key),content:'DATA_ONLY',format:'CSV',scope:'SPECIFIC',objects:{table},parser:{columnSeparator:',',columnQuote:'"'},mapping:{kind:'POSITION'},execution:{memory:'1G',thread:'1',strict:'true',maxErrors:'0',maxDiscards:'0',[field]:format},cleanup:'NONE'}}})
  }
  const before=await fixture('import-oracle-remaining-inspect')
  await save(path.join(work,'oracle-content-before.json'),before)
  require(before.ownerVerified && before.records.length===3 && before.records.every(row=>row.actualRows===0 && row.structureMatched),'REMAINING_ORACLE_SEED_CHANGED')
  let reported=0
  const batch=await run({...input,sourceId:input.oracleSourceId,database:schema,parameterCases:cases},{request:client.request.bind(client),publish:async state=>{
    await save(path.join(work,'oracle-api.json'),state)
    while(reported<state.records.length){const row=state.records[reported++];console.log(`${row.id}: ${row.outcome}`)}
  }})
  await save(path.join(work,'oracle-content-after.json'),await fixture('import-oracle-remaining-inspect'))
  require(batch.done && !batch.stopped && batch.records.length===3,'REMAINING_ORACLE_BATCH_INCOMPLETE')
  const gates=await runGates({...input,gateFilter:'remaining'},client,save,work)
  require(gates.done && !gates.stopped && gates.records.length===19,'REMAINING_GATE_BATCH_INCOMPLETE')
  // 负例草稿仅按本轮回执删除，任务与证据保留。
  for(const row of batch.records.filter(row=>row.draftId && !row.taskId)) {
    const draft=await accepted(`/api/v1/normal-import-drafts/${row.draftId}`)
    require((await client.request(`/api/v1/normal-import-drafts/${draft.id}`,'DELETE',{},draft.revision)).status===200,'REMAINING_DRAFT_CLEANUP_FAILED')
  }
  node=await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
  await save(path.join(work,'node-after.json'),{health:await client.health(),heartbeatStatus:node.heartbeatStatus,lastHeartbeatAt:node.lastHeartbeatAt,environmentStatus:node.environmentStatus,taskSummary:node.taskSummary,executionQuota:node.executionQuota,logQueue:node.agentDiagnostics?.logQueue})
  console.log('Oracle 三项与剩余十九项已按真实 API 完成尝试；成功结论等待独立行集、结构与持久事件核对。')
}
