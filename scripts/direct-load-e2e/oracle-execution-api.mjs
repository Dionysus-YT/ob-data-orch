// Oracle 修复后的真实 API 执行；所有写入仅面向本轮 Schema 所有权表，结果另行独立核对。
import { readFile, writeFile, rename } from 'node:fs/promises'
import { execFile as runFile } from 'node:child_process'
import { promisify } from 'node:util'
import { randomUUID } from 'node:crypto'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { ApiClient } from '../normal-import-e2e/api-client.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..'), directory = process.argv[2]
const replaceSmoke = process.argv[3] === '--replace-smoke'
const smoke = process.argv[3] === '--smoke' || replaceSmoke
if (![3,4].includes(process.argv.length) || process.argv.length === 4 && !smoke || !/^artifacts[\\/]direct-load-e2e-[0-9]{8}-[a-f0-9]{12}$/.test(directory ?? '')) throw new Error('RUN_SCOPE_REQUIRED')
const work = path.join(root, directory), owned = JSON.parse(await readFile(path.join(work, 'run.json'), 'utf8'))
if (!/^[a-f0-9]{32}$/.test(owned.runId) || !new RegExp(`^odo_e2e_[0-9]{8}_${owned.runId.slice(0,12)}$`).test(owned.database) || !work.endsWith(owned.runId.slice(0,12))) throw new Error('RUN_OWNER_INVALID')
await writeFile(path.join(work, 'oracle-execution-started.json'), JSON.stringify({ startedAt: new Date().toISOString() }), { flag: 'wx' })
const execute = promisify(runFile), helper = path.join(root, 'artifacts/direct-load-oracle-tools/fixture-oracle.exe')
const python = 'C:/Users/Dionysus/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'
const fixtureActions={'direct-create':'direct-oracle-execution-create','direct-inspect':'direct-oracle-execution-inspect','direct-cleanup':'direct-oracle-execution-cleanup','direct-source-request':'direct-oracle-source-request','audit':'direct-oracle-audit'}
const helperArgs = action => ['-action', fixtureActions[action]??action, '-database', owned.database, '-run-id', owned.runId, '-python', python]
const client = new ApiClient(await readFile(path.join(root, 'var/dev/control-plane/data/control-plane-ca.pem')))
const state = { scope: 'LIVE_ORACLE_DIRECT_EXECUTION_API', runId: owned.runId, records: [], drafts: [], tasks: [], prechecks: [] }
let fixtureCreated = false, activeTask = false, activeCheck = false, database = '', identity = '', originalBefore
const originalSourceId='55208ffb-2171-4e69-bb97-5af53c2f0d81'
const sourceFacts=source=>Object.fromEntries(['id','revision','credentialRevision','connectionKind','compatibilityMode','state','port'].map(key=>[key,source[key]]))
const requireFact = (condition, code) => { if (!condition) throw new Error(code) }
const safeCode = error => /^[A-Z_0-9]+$/.test(error?.message ?? '') ? error.message : 'DIRECT_STANDALONE_UNAVAILABLE'
async function save(name, value) { const destination = path.join(work, name), temporary = `${destination}.${process.pid}.tmp`; await writeFile(temporary, JSON.stringify(value, null, 2), { flag: 'wx' }); await rename(temporary, destination) }
const checkpoint = () => save('oracle-execution-api.json', state)
async function accepted(route, method='GET', body, revision, key) { const response = await client.request(route, method, body, revision, key); requireFact(response.status >= 200 && response.status < 300, response.body.code ?? response.body.error?.code ?? 'API_REJECTED'); return response.body.item ?? response.body }
async function poll(route, terminal, timeout=180000) { const until=Date.now()+timeout; while(Date.now()<until) { const value=await accepted(route); if(terminal(value)) return value; await new Promise(resolve=>setTimeout(resolve,750)) } throw new Error('POLL_TIMEOUT') }
async function fixture(action, receipt=action) { let result; try { result=await execute(helper,helperArgs(action),{cwd:root,windowsHide:true,maxBuffer:4*1024*1024,timeout:120000}) } catch { throw new Error('FIXTURE_OPERATION_UNAVAILABLE') } const value=JSON.parse(result.stdout); requireFact(value.ownerVerified===true,'FIXTURE_OWNER_INVALID'); await save(`${receipt}.json`,value); return value }
async function record(id, action) { const row={id,startedAt:new Date().toISOString()}; state.current=row; await checkpoint(); try {Object.assign(row,{outcome:'PASS'},await action(row))}catch(error){Object.assign(row,{outcome:'FAIL',code:safeCode(error)})} row.finishedAt=new Date().toISOString(); state.records.push(row);delete state.current;await checkpoint();console.log(`${id}: ${row.outcome}`);return row }
const base='/api/v1/direct-load-drafts', nodeId='7ebf034b-7874-48e8-8222-9c030baa7446'
const inputRoot=path.join(root,'ob-data',`dlvt-oracle-${owned.runId.slice(0,12)}`)
const tableName=table=>`DLVO_${owned.runId.slice(0,12).toUpperCase()}_${table.replace('dlvt_','').toUpperCase()}`
function config(table, patch={}) {return {dataSourceId:state.sourceId,nodeId,database,scenario:'OBSERVER_DIRECT',rpcPort:2882,table:tableName(table),filePath:path.join(inputRoot,table.replace('dlvt_','')),format:'CSV',externalData:true,columnSeparator:'',thread:'1',parallel:'1',replaceData:false,truncateTable:false,applicability:{singleTable:true,wholeTableCommit:true,noCheckpoint:true,structureLimits:true},...patch}}
async function draft(value) { const item=await accepted(base,'POST',{configVersion:'direct-load-v1',config:value});state.drafts.push({id:item.id,revision:item.revision});await checkpoint();return item }
async function precheck(draft,row) { const started=await accepted(`${base}/${draft.id}:precheck`,'POST',{},draft.revision);row.precheckId=started.id;state.prechecks.push(started.id);activeCheck=true;await checkpoint();const value=await poll(`/api/v1/direct-load-prechecks/${started.id}`,v=>!['PENDING','LEASED'].includes(v.status));activeCheck=false;await save(`precheck-${row.id}.json`,value);row.checks=value.results;row.precheckStatus=value.status;return value }
async function runImport(id,table,patch={},first=false) {return record(id,async row=>{
  const item=await draft(config(table,patch));row.draftId=item.id
  const preview=await accepted(`${base}/${item.id}:preview-command`,'POST',{},item.revision)
  requireFact(preview.configFingerprint===item.configFingerprint && preview.redactedCommand.includes('--tenant') && preview.redactedCommand.includes(identity) && !preview.redactedCommand.includes('--cluster') && preview.redactedCommand.includes('******'),'DIRECT_COMMAND_MAPPING_INVALID')
  row.fingerprint=preview.configFingerprint
  const check=await precheck(item,row)
  requireFact(check.status==='SUCCEEDED' && check.integrityStatus==='COMPLETE' && check.results.length===9 && check.results.every(value=>value.status==='PASSED') && check.inputManifest && check.directLoadConfigFingerprint,'DIRECT_PREFLIGHT_NOT_PASSED')
  if(first) {
    const rejected=await client.request(`${base}/${item.id}:submit`,'POST',{precheckId:check.id,taskName:'synthetic-confirmation-test'},item.revision)
    requireFact(rejected.status===422 && rejected.body.code==='DIRECT_LOAD_CONFIRMATION_REQUIRED','FINAL_CONFIRMATION_NOT_ENFORCED')
    row.confirmationRejected=true
  }
  const body={precheckId:check.id,taskName:`direct-api-${id}-${owned.runId.slice(0,12)}`,confirmedConfigFingerprint:item.configFingerprint},key=randomUUID()
  row.submitIdempotencyKey=key;state.unresolvedSubmit=true;await checkpoint()
  const submitted=await accepted(`${base}/${item.id}:submit`,'POST',body,item.revision,key)
  row.taskId=submitted.id;state.tasks.push(submitted.id);state.unresolvedSubmit=false;activeTask=true;state.drafts.find(d=>d.id===item.id).submitted=true;await checkpoint()
  if(first) {const replay=await accepted(`${base}/${item.id}:submit`,'POST',body,item.revision,key);requireFact(replay.id===submitted.id && replay.replayed===true,'SUBMISSION_REPLAY_CHANGED_TASK');row.submissionReplayed=true
    const observed=await accepted(`/api/v1/direct-load-prechecks/${check.id}`);requireFact(observed.submittedTaskId===submitted.id,'SUBMITTED_TASK_LOOKUP_MISSING');row.submittedTaskRecovered=true
    state.unresolvedSubmit=true;await checkpoint();const crossKey=await client.request(`${base}/${item.id}:submit`,'POST',body,item.revision,randomUUID());requireFact(crossKey.status===409&&crossKey.body.code==='DIRECT_LOAD_SUBMISSION_CONFLICT','CROSS_KEY_DUPLICATED_TASK');state.unresolvedSubmit=false;row.crossKeyRejected=true;await checkpoint()
    const conflict=await client.request(`${base}/${item.id}:submit`,'POST',{...body,taskName:body.taskName+'-changed'},item.revision,randomUUID());requireFact(conflict.status===409&&conflict.body.code==='DIRECT_LOAD_SUBMISSION_CONFLICT','CONSUMED_PROOF_CHANGED_BODY_ACCEPTED');row.changedBodyRejected=true
  }
  const execution=await poll(`/api/v1/tasks/${submitted.id}/execution`,v=>['SUCCEEDED','FAILED','CANCELLED'].includes(v.state));activeTask=false
  const command=await accepted(`/api/v1/tasks/${submitted.id}/command-evidence`),snapshot=await accepted(`/api/v1/tasks/${submitted.id}/snapshot`),logs=await accepted(`/api/v1/tasks/${submitted.id}/logs`)
  row.execution={state:execution.state,exitCode:execution.exitCode,plannedActualMatch:execution.plannedActualMatch,reconciliationRequired:execution.reconciliationRequired,resultSummary:execution.resultSummary}
  row.commandMatched=command.command===preview.redactedCommand;row.snapshotMatched=snapshot.configFingerprint===item.configFingerprint && snapshot.groups?.length===6
  row.sameTableCommitObserved=logs.items?.some(log=>log.message.includes(`Load task on table "${tableName(table)}" is committed successfully!`))===true
  // 持久化日志前独立检查当前 TEST 秘密，探针只返回布尔结果，原文和可恢复编码均不得留证。
  const logBytes=Buffer.from(JSON.stringify(logs));let audit
  try { audit=await new Promise((resolve,reject)=>{const process=runFile(helper,helperArgs('audit'),{cwd:root,windowsHide:true,maxBuffer:1024},(error,output)=>error?reject(new Error('LOG_AUDIT_UNAVAILABLE')):resolve(JSON.parse(output)));process.stdin.end(logBytes)}) } finally {logBytes.fill(0)}
  requireFact(audit.plaintextOrRecoverableEncodingPresent===false,'LOG_SECRET_DETECTED')
  await save(`task-${id}.json`,{execution,command,snapshot,logs,secretAudit:audit})
  // execution API 不返回内部 result_summary；其旁路与提交事实由收尾只读数据库核验，不读取不存在的响应字段。
  requireFact(execution.state==='SUCCEEDED' && execution.exitCode===0 && execution.plannedActualMatch==='MATCHED' && !execution.reconciliationRequired && row.commandMatched && row.snapshotMatched && row.sameTableCommitObserved,'DIRECT_EXECUTION_NOT_VERIFIED')
  return {inputFiles:check.inputManifest.fileCount,targetVerification:'PENDING_INDEPENDENT_READ'}
})}
try {
  const chunks=[];let length=0
  try {for await(const chunk of process.stdin){chunks.push(chunk);length+=chunk.length;requireFact(length<=1024,'LOGIN_INPUT_INVALID')} const password=Buffer.concat(chunks);chunks.forEach(chunk=>chunk.fill(0));await client.login(password)}finally{chunks.forEach(chunk=>chunk.fill(0))}
  state.authenticated=true;await checkpoint()
  originalBefore=sourceFacts(await accepted(`/api/v1/data-sources/${originalSourceId}`));await save('original-source-before.json',originalBefore)
  state.unresolvedFixtureCreate=true;await checkpoint();await fixture('direct-create','fixture-create');fixtureCreated=true;state.unresolvedFixtureCreate=false;await checkpoint();await fixture('direct-inspect','fixture-before')
  // 请求字节只在匿名子进程管道和本次 HTTPS 请求内存中存在，绝不写文件或转换成长期 JS 字符串。
  let sourceBytes
  try {const output=await execute(helper,helperArgs('direct-source-request'),{cwd:root,windowsHide:true,encoding:'buffer',maxBuffer:16384});sourceBytes=output.stdout;const created=await client.createDataSourceWithJSONBytes(sourceBytes);requireFact(created.status===201,'DIRECT_SOURCE_CREATE_REJECTED');state.sourceId=created.body.id;await checkpoint()}finally{sourceBytes?.fill(0)}
  let source=await accepted(`/api/v1/data-sources/${state.sourceId}`)
  requireFact(source.connectionKind==='OBSERVER_DIRECT' && source.compatibilityMode==='ORACLE' && source.state==='DISABLED' && source.displayName===`direct-oracle-api-${owned.runId.slice(0,12)}`,'DIRECT_SOURCE_IDENTITY_INVALID')
  requireFact(/^[A-Z_][A-Z0-9_$#]*$/.test(source.username)&&!source.clusterName&&source.tenantName,'ORACLE_SOURCE_IDENTITY_INVALID');database=source.username;identity=source.username+'@'+source.tenantName
  await record('SOURCE_CONNECTION_TEST',async row=>{const started=await accepted(`/api/v1/data-sources/${source.id}:test-connection`,'POST',{nodeId},source.revision);row.connectionTestId=started.id;const result=await poll(`/api/v1/data-source-connection-tests/${started.id}`,v=>!['PENDING','LEASED'].includes(v.status));await save('source-connection-test.json',result);requireFact(result.status==='SUCCEEDED' && result.verificationSource==='AGENT_JDBC','DIRECT_SOURCE_CONNECTION_FAILED');return {status:result.status,verificationSource:result.verificationSource}})
  requireFact(state.records.at(-1).outcome==='PASS','DIRECT_SOURCE_CONNECTION_FAILED')
  source=await accepted(`/api/v1/data-sources/${state.sourceId}`);await accepted(`/api/v1/data-sources/${source.id}:enable`,'POST',{},source.revision)
  state.sourceEnabled=true;await checkpoint()
  const basic=await runImport('BASIC','dlvt_basic',replaceSmoke?{replaceData:true}:{},true)
  requireFact(basic.outcome==='PASS','BASIC_PATH_FAILED')
  for(const [id,table,patch] of (smoke ? [] : [ ['SHARDS','dlvt_shards',{}],['SEPARATOR','dlvt_separator',{columnSeparator:'|'}],['INCREMENTAL','dlvt_incremental',{}],['TRUNCATE','dlvt_truncate',{truncateTable:true}],['THREAD','dlvt_thread',{thread:'2'}],['PARALLEL','dlvt_parallel',{parallel:'2'}] ])) {
    await runImport(id,table,patch)
    if(activeTask || activeCheck || state.unresolvedSubmit) break
  }
  if(!activeTask && !activeCheck && !state.unresolvedSubmit) for(const [id,table,patch,failedCheck] of [ ['RAW_BLOCKED','dlvt_raw',{},'DIRECT_LOAD_STRUCTURE'],['INDEXED_REPLACE_BLOCKED','dlvt_replace',{replaceData:true},'DIRECT_LOAD_STRUCTURE'],['LOB_REPLACE_BLOCKED','dlvt_lob',{replaceData:true},'DIRECT_LOAD_STRUCTURE'] ]) await record(id,async row=>{const item=await draft(config(table,patch));const check=await precheck(item,row);requireFact(check.status==='FAILED' && check.results.some(value=>value.check===failedCheck && value.status==='FAILED'),'UNVERIFIED_STRUCTURE_ACCEPTED');const response=await client.request(`${base}/${item.id}:submit`,'POST',{precheckId:check.id,taskName:`negative-${id}`,confirmedConfigFingerprint:item.configFingerprint},item.revision);requireFact(response.status===422 && response.body.code==='PRECHECK_REQUIRED','FAILED_PREFLIGHT_SUBMITTED');return {submissionStatus:response.status,code:response.body.code}})
  if(!activeTask && !activeCheck && !state.unresolvedSubmit) await record('CONFIG_CHANGE_INVALIDATES_PROOF',async row=>{const item=await draft(config('dlvt_basic'));const check=await precheck(item,row);requireFact(check.status==='SUCCEEDED','STALE_BASELINE_NOT_VERIFIED');const updated=await accepted(`${base}/${item.id}`,'PATCH',{configVersion:'direct-load-v1',config:config('dlvt_basic',{parallel:'2'})},item.revision);state.drafts.find(d=>d.id===item.id).revision=updated.revision;const old=await accepted(`/api/v1/direct-load-prechecks/${check.id}`);requireFact(old.status==='INVALIDATED','OLD_PROOF_STILL_VALID');const rejected=await client.request(`${base}/${item.id}:submit`,'POST',{precheckId:check.id,taskName:'stale-proof',confirmedConfigFingerprint:updated.configFingerprint},updated.revision);requireFact(rejected.status===422 && rejected.body.code==='PRECHECK_REQUIRED','STALE_PROOF_SUBMITTED');return {status:old.status,submissionStatus:rejected.status}})
}catch(error){state.error=safeCode(error);process.exitCode=1}
finally {
  if(state.authenticated && !activeTask && !activeCheck && !state.unresolvedSubmit && !state.unresolvedFixtureCreate){
    if(fixtureCreated) try {await fixture('direct-inspect','fixture-after')}catch{state.targetReadError='INDEPENDENT_TARGET_READ_UNAVAILABLE';process.exitCode=1}
    for(const item of state.drafts)if(!item.submitted)try{const value=await client.request(`${base}/${item.id}`,'DELETE',{},item.revision);item.cleanupStatus=value.status}catch{item.cleanupStatus='UNAVAILABLE'}
    if(state.sourceId)try{let source=await accepted(`/api/v1/data-sources/${state.sourceId}`);requireFact(source.displayName===`direct-oracle-api-${owned.runId.slice(0,12)}`,'SOURCE_OWNER_INVALID');if(source.state==='ENABLED'){await accepted(`/api/v1/data-sources/${source.id}:disable`,'POST',{},source.revision);source=await accepted(`/api/v1/data-sources/${source.id}`)}const outcome=state.tasks.length?await accepted(`/api/v1/data-sources/${source.id}:archive`,'POST',{},source.revision):await accepted(`/api/v1/data-sources/${source.id}`,'DELETE',{},source.revision);state.sourceCleanup=outcome.outcome}catch(error){state.sourceCleanupError=safeCode(error);process.exitCode=1}
    if(fixtureCreated)try{const cleaned=await fixture('direct-cleanup','fixture-cleanup');requireFact(cleaned.tablesAbsent&&cleaned.inputAbsent,'ORACLE_CLEANUP_UNCONFIRMED');state.fixtureCleaned=true}catch{state.fixtureCleanupError='FIXTURE_CLEANUP_UNAVAILABLE';process.exitCode=1}
    if(originalBefore)try{const after=sourceFacts(await accepted(`/api/v1/data-sources/${originalSourceId}`));await save('original-source-after.json',after);state.originalSourceUnchanged=JSON.stringify(after)===JSON.stringify(originalBefore);requireFact(state.originalSourceUnchanged,'ORACLE_ORIGINAL_SOURCE_CHANGED')}catch{state.originalSourceReadError='ORACLE_ORIGINAL_SOURCE_UNAVAILABLE';process.exitCode=1}
  }else state.cleanupDeferred='UNRESOLVED_CHECK_OR_TASK_REQUIRES_RECONCILIATION'
  await client.close();state.finishedAt=new Date().toISOString();state.apiPassed=!state.error && state.records.every(row=>row.outcome==='PASS') && !['targetReadError','sourceCleanupError','fixtureCleanupError','originalSourceReadError','cleanupDeferred'].some(key=>state[key]);state.independentTargetVerification='REQUIRED_BEFORE_E2E_PASS';await checkpoint();console.log(JSON.stringify({apiPassed:state.apiPassed,records:state.records.length,tasks:state.tasks.length,error:state.error,cleanupDeferred:state.cleanupDeferred}));if(!state.apiPassed)process.exitCode=1
}
